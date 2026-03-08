package server

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"quotio-lite/internal/accounts"
	"quotio-lite/internal/config"
	"quotio-lite/internal/login"
	"quotio-lite/internal/managementusage"
	"quotio-lite/internal/probe"
	"quotio-lite/internal/proxyruntime"
)

type App struct {
	cfg      config.Config
	accounts accounts.Store
	login    login.Service
	probe    probe.Service
	usage    *managementusage.Service
	proxy    *proxyruntime.Manager

	mu                 sync.RWMutex
	lastProbe          map[string]probeSnapshot
	usageCachedAt      time.Time
	usageValidUntil    time.Time
	usageSnapshot      managementusage.Snapshot
	usageFetchInFlight chan struct{}
}

type probeSnapshot struct {
	Status  string
	At      string
	Message string
}

func New(cfg config.Config) *App {
	return &App{
		cfg:       cfg,
		accounts:  accounts.Store{AuthDir: cfg.AuthDir},
		login:     login.Service{AuthDir: cfg.AuthDir, CLIProxyPath: cfg.CLIProxyPath},
		probe:     probe.Service{AuthDir: cfg.AuthDir, CLIProxyPath: cfg.CLIProxyPath, Model: cfg.ProbeModel, RequestTimeout: cfg.ProbeTimeout},
		usage:     &managementusage.Service{AuthDir: cfg.AuthDir},
		proxy:     proxyruntime.NewManager(cfg.CLIProxyPath, cfg.AuthDir),
		lastProbe: map[string]probeSnapshot{},
	}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/meta", a.handleMeta)
	mux.HandleFunc("GET /api/accounts", a.handleAccounts)
	mux.HandleFunc("GET /api/accounts/{file}", a.handleAccountDetail)
	mux.HandleFunc("POST /api/accounts/login", a.handleLogin)
	mux.HandleFunc("DELETE /api/accounts/{file}", a.handleDeleteAccount)
	mux.HandleFunc("POST /api/accounts/{file}/probe", a.handleProbe)
	mux.HandleFunc("GET /api/proxy/status", a.handleProxyStatus)
	mux.HandleFunc("POST /api/proxy/start", a.handleProxyStart)
	mux.HandleFunc("POST /api/proxy/stop", a.handleProxyStop)
	mux.HandleFunc("POST /api/proxy/restart", a.handleProxyRestart)
	mux.HandleFunc("GET /api/proxy/credentials", a.handleProxyCredentials)
	mux.HandleFunc("POST /api/proxy/api-key/rotate", a.handleProxyRotateAPIKey)

	return withCORS(withJSON(mux))
}

func (a *App) handleMeta(w http.ResponseWriter, _ *http.Request) {
	_, authErr := os.Stat(a.cfg.AuthDir)
	_, cliErr := os.Stat(a.cfg.CLIProxyPath)
	proxyMeta := a.proxy.Meta()

	loginCapabilities, loginCapsErr := a.login.Capabilities(context.Background())

	response := map[string]interface{}{
		"version":                "v1.1",
		"host":                   a.cfg.Host,
		"port":                   a.cfg.Port,
		"authDir":                a.cfg.AuthDir,
		"cliProxyPath":           a.cfg.CLIProxyPath,
		"probeModel":             a.cfg.ProbeModel,
		"usageSource":            "chatgpt_wham_usage",
		"authDirAccessible":      authErr == nil,
		"cliProxyAccessible":     cliErr == nil,
		"proxyManagedConfigPath": proxyMeta.ManagedConfigPath,
		"proxyManagedStatePath":  proxyMeta.ManagedStatePath,
		"proxyDefaultPort":       proxyMeta.Port,
		"proxyHost":              proxyMeta.Host,
		"loginCapabilities":      loginCapabilities,
	}
	if loginCapsErr != nil {
		response["loginCapabilitiesError"] = loginCapsErr.Error()
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleAccounts(w http.ResponseWriter, r *http.Request) {
	records, err := a.accounts.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	a.mu.RLock()
	for i := range records {
		if s, ok := a.lastProbe[records[i].File]; ok {
			records[i].Status = s.Status
			records[i].LastProbeAt = s.At
			records[i].LastProbeMessage = s.Message
		}
	}
	a.mu.RUnlock()

	usageSnapshot := a.getUsageSnapshot(r.Context())
	for i := range records {
		applyUsageToRecord(&records[i], usageSnapshot)
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].Email == records[j].Email {
			return records[i].File < records[j].File
		}
		return records[i].Email < records[j].Email
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": records})
}

func (a *App) handleAccountDetail(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	detail, err := a.accounts.ReadDetail(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	a.mu.RLock()
	if s, ok := a.lastProbe[file]; ok {
		detail.Status = s.Status
		detail.LastProbeAt = s.At
		detail.LastProbeMessage = s.Message
	}
	a.mu.RUnlock()
	applyUsageToRecord(&detail.AccountRecord, a.getUsageSnapshot(r.Context()))

	writeJSON(w, http.StatusOK, detail)
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	result, err := a.login.LoginCodex(r.Context(), login.LoginMode(payload.Mode))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	detail, err := a.accounts.ReadDetail(result.File)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"file": result.File, "account": detail})
}

func (a *App) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if err := a.accounts.Delete(file); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	a.mu.Lock()
	delete(a.lastProbe, file)
	a.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleProbe(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	result, err := a.probe.Run(r.Context(), file)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	a.mu.Lock()
	a.lastProbe[file] = probeSnapshot{Status: result.Classification, At: now, Message: result.RawSnippet}
	a.mu.Unlock()

	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleProxyStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.proxy.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *App) handleProxyStart(w http.ResponseWriter, r *http.Request) {
	status, err := a.proxy.Start(r.Context())
	if err != nil {
		var conflictErr *proxyruntime.PortConflictError
		if errors.As(err, &conflictErr) {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":        conflictErr.Error(),
				"portConflict": conflictErr.Conflict,
			})
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *App) handleProxyStop(w http.ResponseWriter, r *http.Request) {
	status, err := a.proxy.Stop(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *App) handleProxyRestart(w http.ResponseWriter, r *http.Request) {
	status, err := a.proxy.Restart(r.Context())
	if err != nil {
		var conflictErr *proxyruntime.PortConflictError
		if errors.As(err, &conflictErr) {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":        conflictErr.Error(),
				"portConflict": conflictErr.Conflict,
			})
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *App) handleProxyCredentials(w http.ResponseWriter, r *http.Request) {
	result, err := a.proxy.Credentials(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleProxyRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	result, err := a.proxy.RotateAPIKey(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSONBody(r *http.Request, out interface{}) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(payload)
}

func (a *App) getUsageSnapshot(ctx context.Context) managementusage.Snapshot {
	const usageCacheTTL = 120 * time.Second
	const usageCacheJitterMax = 30 * time.Second

	a.mu.Lock()
	now := time.Now()
	cached := a.usageSnapshot
	validUntil := a.usageValidUntil
	if !validUntil.IsZero() && now.Before(validUntil) {
		a.mu.Unlock()
		return cached
	}

	if inFlight := a.usageFetchInFlight; inFlight != nil {
		a.mu.Unlock()
		select {
		case <-inFlight:
		case <-ctx.Done():
			a.mu.RLock()
			fallback := a.usageSnapshot
			a.mu.RUnlock()
			if len(fallback.ByFile) > 0 || fallback.Status != "" {
				return fallback
			}
			return managementusage.Snapshot{
				Status:  "unavailable",
				Message: ctx.Err().Error(),
			}
		}
		a.mu.RLock()
		defer a.mu.RUnlock()
		return a.usageSnapshot
	}

	inFlight := make(chan struct{})
	a.usageFetchInFlight = inFlight
	a.mu.Unlock()

	snapshot, err := a.usage.Fetch(ctx)
	if err != nil {
		snapshot = managementusage.Snapshot{
			Status:  "unavailable",
			Message: err.Error(),
		}
	}

	validFor := usageCacheTTL + randomDurationUpTo(usageCacheJitterMax)
	a.mu.Lock()
	a.usageSnapshot = snapshot
	a.usageCachedAt = time.Now()
	a.usageValidUntil = a.usageCachedAt.Add(validFor)
	close(inFlight)
	a.usageFetchInFlight = nil
	a.mu.Unlock()
	return snapshot
}

func randomDurationUpTo(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return time.Duration(binary.LittleEndian.Uint64(b[:]) % uint64(max+1))
}

func applyUsageToRecord(record *accounts.AccountRecord, snapshot managementusage.Snapshot) {
	usage := accounts.AccountUsage{
		Status:  "unavailable",
		Message: snapshot.Message,
	}

	if stats, ok := snapshot.ByFile[record.File]; ok {
		usage.PlanType = stats.PlanType
		usage.Status = stats.Status
		usage.Message = stats.Message
		usage.Window5H = accounts.UsageWindow{
			UsedPercent: stats.Window5H.UsedPercent,
			ResetAt:     stats.Window5H.ResetAt,
		}
		usage.Weekly = accounts.UsageWindow{
			UsedPercent: stats.Weekly.UsedPercent,
			ResetAt:     stats.Weekly.ResetAt,
		}
	}

	record.Usage = usage
}
