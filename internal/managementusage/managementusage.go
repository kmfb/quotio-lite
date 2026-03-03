package managementusage

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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	codexUsageEndpoint   = "https://chatgpt.com/backend-api/wham/usage"
	codexQuotaUserAgent  = "codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal"
	window5HSeconds      = 5 * 60 * 60
	windowWeeklySeconds  = 7 * 24 * 60 * 60
	defaultFetchTimeout  = 10 * time.Second
	defaultMaxConcurrent = 4
	minBackoffDuration   = 3 * time.Minute
	maxBackoffDuration   = 10 * time.Minute
)

type Window struct {
	UsedPercent *float64
	ResetAt     string
}

type AccountUsage struct {
	PlanType string
	Window5H Window
	Weekly   Window
	Status   string
	Message  string
}

type Snapshot struct {
	Status  string
	Message string
	ByFile  map[string]AccountUsage
}

type Service struct {
	AuthDir string
	Timeout time.Duration

	mu               sync.Mutex
	backoffUntil     time.Time
	backoffReason    string
	lastGoodSnapshot map[string]AccountUsage
}

type credentialFile struct {
	AccessToken interface{} `json:"access_token"`
	AccountID   interface{} `json:"account_id"`
}

type usageResponse struct {
	PlanType  string          `json:"plan_type"`
	RateLimit rateLimitWindow `json:"rate_limit"`
}

type rateLimitWindow struct {
	Allowed         *bool        `json:"allowed"`
	PrimaryWindow   *usageWindow `json:"primary_window"`
	SecondaryWindow *usageWindow `json:"secondary_window"`
}

type usageWindow struct {
	UsedPercent        interface{} `json:"used_percent"`
	LimitWindowSeconds interface{} `json:"limit_window_seconds"`
	ResetAt            interface{} `json:"reset_at"`
}

func (s *Service) Fetch(ctx context.Context) (Snapshot, error) {
	if snapshot, active := s.snapshotFromBackoff(); active {
		return snapshot, nil
	}

	authDir := strings.TrimSpace(s.AuthDir)
	if authDir == "" {
		return Snapshot{}, errors.New("auth dir not configured")
	}

	entries, err := os.ReadDir(authDir)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read auth dir: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if isCandidateFile(name) {
			files = append(files, name)
		}
	}

	snapshot := Snapshot{
		Status: "ok",
		ByFile: map[string]AccountUsage{},
	}
	if len(files) == 0 {
		return snapshot, nil
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultFetchTimeout
	}
	client := &http.Client{Timeout: timeout}

	sem := make(chan struct{}, defaultMaxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var okCount int
	var shouldBackoff bool
	var backoffReason string

	for _, file := range files {
		file := file
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				snapshot.ByFile[file] = AccountUsage{
					Status:  "unavailable",
					Message: "quota request canceled",
				}
				mu.Unlock()
				return
			}
			defer func() { <-sem }()

			usage, triggerBackoff, reason := s.fetchOne(ctx, client, filepath.Join(authDir, file))
			mu.Lock()
			snapshot.ByFile[file] = usage
			if usage.Status == "ok" || usage.Status == "no_access" {
				okCount++
			}
			if triggerBackoff && !shouldBackoff {
				shouldBackoff = true
				backoffReason = reason
			}
			mu.Unlock()
		}()
	}

	wg.Wait()
	if shouldBackoff {
		return s.activateBackoff(backoffReason), nil
	}
	if okCount == 0 {
		snapshot.Status = "unavailable"
		snapshot.Message = "failed to fetch quota for all accounts"
	}
	s.updateLastGoodSnapshot(snapshot.ByFile)
	return snapshot, nil
}

func (s *Service) fetchOne(ctx context.Context, client *http.Client, path string) (AccountUsage, bool, string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AccountUsage{Status: "unavailable", Message: fmt.Sprintf("read credential: %v", err)}, false, ""
	}

	var cred credentialFile
	if err := json.Unmarshal(raw, &cred); err != nil {
		return AccountUsage{Status: "unavailable", Message: fmt.Sprintf("decode credential: %v", err)}, false, ""
	}

	token := stringValue(cred.AccessToken)
	accountID := stringValue(cred.AccountID)
	if token == "" || accountID == "" {
		return AccountUsage{Status: "unavailable", Message: "missing access token or account id"}, false, ""
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, codexUsageEndpoint, nil)
	if err != nil {
		return AccountUsage{Status: "unavailable", Message: fmt.Sprintf("build usage request: %v", err)}, false, ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Chatgpt-Account-Id", accountID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", codexQuotaUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return AccountUsage{Status: "unavailable", Message: fmt.Sprintf("request quota: %v", err)}, false, ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return AccountUsage{Status: "unavailable", Message: fmt.Sprintf("read quota response: %v", err)}, false, ""
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := extractErrorMessage(body)
		if message == "" {
			message = "request failed"
		}
		return AccountUsage{
			Status:  "unavailable",
			Message: fmt.Sprintf("HTTP %d %s", resp.StatusCode, message),
		}, isBackoffHTTPStatus(resp.StatusCode), message
	}

	var payload usageResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return AccountUsage{Status: "unavailable", Message: fmt.Sprintf("decode quota response: %v", err)}, false, ""
	}

	fiveHour, weekly := pickWindows(payload.RateLimit)
	usage := AccountUsage{
		PlanType: normalizePlanType(payload.PlanType),
		Status:   "ok",
		Window5H: toWindow(fiveHour),
		Weekly:   toWindow(weekly),
	}

	if usage.Window5H.UsedPercent == nil && usage.Weekly.UsedPercent == nil {
		usage.Status = "unavailable"
		usage.Message = "no quota windows returned"
	}

	allowed := payload.RateLimit.Allowed
	if usage.PlanType == "free" || (allowed != nil && !*allowed) {
		usage.Status = "no_access"
		if usage.Message == "" {
			usage.Message = "this account has no codex access"
		}
	}

	return usage, false, ""
}

func (s *Service) snapshotFromBackoff() (Snapshot, bool) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backoffUntil.IsZero() {
		return Snapshot{}, false
	}
	if !now.Before(s.backoffUntil) {
		s.backoffUntil = time.Time{}
		s.backoffReason = ""
		return Snapshot{}, false
	}

	message := fmt.Sprintf(
		"quota fetch backoff active until %s: %s",
		s.backoffUntil.UTC().Format(time.RFC3339),
		s.backoffReason,
	)
	cached := cloneUsageMap(s.lastGoodSnapshot)
	if len(cached) > 0 {
		return Snapshot{
			Status:  "stale",
			Message: message,
			ByFile:  cached,
		}, true
	}
	return Snapshot{
		Status:  "unavailable",
		Message: message,
		ByFile:  map[string]AccountUsage{},
	}, true
}

func (s *Service) activateBackoff(reason string) Snapshot {
	if strings.TrimSpace(reason) == "" {
		reason = "rate limited by upstream"
	}
	until := time.Now().Add(randomDurationInRange(minBackoffDuration, maxBackoffDuration))

	s.mu.Lock()
	s.backoffUntil = until
	s.backoffReason = reason
	cached := cloneUsageMap(s.lastGoodSnapshot)
	s.mu.Unlock()

	message := fmt.Sprintf(
		"quota fetch backoff active until %s: %s",
		until.UTC().Format(time.RFC3339),
		reason,
	)
	if len(cached) > 0 {
		return Snapshot{
			Status:  "stale",
			Message: message,
			ByFile:  cached,
		}
	}
	return Snapshot{
		Status:  "unavailable",
		Message: message,
		ByFile:  map[string]AccountUsage{},
	}
}

func (s *Service) updateLastGoodSnapshot(byFile map[string]AccountUsage) {
	if len(byFile) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastGoodSnapshot == nil {
		s.lastGoodSnapshot = map[string]AccountUsage{}
	}
	for file, usage := range byFile {
		if usage.Status == "ok" || usage.Status == "no_access" {
			s.lastGoodSnapshot[file] = usage
		}
	}
}

func isBackoffHTTPStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusForbidden
}

func randomDurationInRange(min, max time.Duration) time.Duration {
	if min <= 0 && max <= 0 {
		return 0
	}
	if max <= min {
		return min
	}

	diff := max - min
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return min
	}
	offset := time.Duration(binary.LittleEndian.Uint64(b[:]) % uint64(diff+1))
	return min + offset
}

func cloneUsageMap(src map[string]AccountUsage) map[string]AccountUsage {
	if len(src) == 0 {
		return map[string]AccountUsage{}
	}
	out := make(map[string]AccountUsage, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func pickWindows(limit rateLimitWindow) (*usageWindow, *usageWindow) {
	var fiveHour *usageWindow
	var weekly *usageWindow

	candidates := []*usageWindow{limit.PrimaryWindow, limit.SecondaryWindow}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		seconds := intValue(candidate.LimitWindowSeconds)
		switch seconds {
		case window5HSeconds:
			if fiveHour == nil {
				fiveHour = candidate
			}
		case windowWeeklySeconds:
			if weekly == nil {
				weekly = candidate
			}
		}
	}

	if fiveHour == nil {
		fiveHour = limit.PrimaryWindow
	}
	if weekly == nil && limit.SecondaryWindow != fiveHour {
		weekly = limit.SecondaryWindow
	}
	return fiveHour, weekly
}

func toWindow(raw *usageWindow) Window {
	if raw == nil {
		return Window{}
	}
	return Window{
		UsedPercent: numberValue(raw.UsedPercent),
		ResetAt:     formatResetAt(raw.ResetAt),
	}
}

func numberValue(v interface{}) *float64 {
	switch t := v.(type) {
	case float64:
		value := t
		return &value
	case float32:
		value := float64(t)
		return &value
	case int:
		value := float64(t)
		return &value
	case int64:
		value := float64(t)
		return &value
	case json.Number:
		num, err := t.Float64()
		if err != nil {
			return nil
		}
		return &num
	case string:
		raw := strings.TrimSpace(t)
		if raw == "" {
			return nil
		}
		num, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil
		}
		return &num
	default:
		return nil
	}
}

func intValue(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case float32:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

func formatResetAt(v interface{}) string {
	switch t := v.(type) {
	case float64:
		return epochToRFC3339(t)
	case int64:
		return time.Unix(t, 0).UTC().Format(time.RFC3339)
	case int:
		return time.Unix(int64(t), 0).UTC().Format(time.RFC3339)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return ""
		}
		return time.Unix(n, 0).UTC().Format(time.RFC3339)
	case string:
		raw := strings.TrimSpace(t)
		if raw == "" {
			return ""
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts.UTC().Format(time.RFC3339)
		}
		if unix, err := strconv.ParseFloat(raw, 64); err == nil {
			return epochToRFC3339(unix)
		}
		return raw
	default:
		return ""
	}
}

func epochToRFC3339(raw float64) string {
	if raw <= 0 {
		return ""
	}
	if raw > 1e12 {
		raw = raw / 1000
	}
	return time.Unix(int64(raw), 0).UTC().Format(time.RFC3339)
}

func stringValue(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	default:
		return ""
	}
}

func normalizePlanType(v string) string {
	raw := strings.ToLower(strings.TrimSpace(v))
	switch raw {
	case "plus", "team", "free", "pro":
		return raw
	default:
		return raw
	}
}

func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if msg := nestedString(parsed, "error", "message"); msg != "" {
			return truncate(msg, 220)
		}
		if msg := stringValue(parsed["error"]); msg != "" {
			return truncate(msg, 220)
		}
		if msg := stringValue(parsed["message"]); msg != "" {
			return truncate(msg, 220)
		}
	}
	return truncate(strings.TrimSpace(string(body)), 220)
}

func nestedString(root map[string]interface{}, keys ...string) string {
	var current interface{} = root
	for _, key := range keys {
		obj, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = obj[key]
	}
	return stringValue(current)
}

func isCandidateFile(name string) bool {
	return strings.HasPrefix(name, "codex-") && strings.HasSuffix(name, ".json")
}

func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n]
}
