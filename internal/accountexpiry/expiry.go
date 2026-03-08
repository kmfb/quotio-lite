package accountexpiry

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultEndpoint      = "https://codexcn.com/api/check-expiry"
	defaultFetchTimeout  = 10 * time.Second
	defaultMaxConcurrent = 3
	minBackoffDuration   = 10 * time.Minute
	maxBackoffDuration   = 30 * time.Minute

	browserReferer         = "https://codexcn.com/"
	browserUserAgent       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	browserSecCHUA         = `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`
	browserSecCHUAMobile   = "?0"
	browserSecCHUAPlatform = `"macOS"`
)

type AccountExpiry struct {
	DaysRemaining *int   `json:"daysRemaining"`
	ExpireDate    string `json:"expireDate"`
	JoinDate      string `json:"joinDate"`
	OrderID       string `json:"orderId"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

type Snapshot struct {
	Status  string
	Message string
	ByFile  map[string]AccountExpiry
}

type Service struct {
	AuthDir    string
	Endpoint   string
	Timeout    time.Duration
	HTTPClient *http.Client

	mu               sync.Mutex
	backoffUntil     time.Time
	backoffReason    string
	lastGoodSnapshot map[string]AccountExpiry
}

type credentialFile struct {
	Email interface{} `json:"email"`
}

type apiResponse struct {
	Success bool            `json:"success"`
	Data    apiResponseData `json:"data"`
	Message string          `json:"message"`
	Error   interface{}     `json:"error"`
}

type apiResponseData struct {
	DaysRemaining interface{} `json:"days_remaining"`
	Email         string      `json:"email"`
	ExpireDate    string      `json:"expire_date"`
	JoinDate      string      `json:"join_date"`
	OrderID       string      `json:"order_id"`
	Status        string      `json:"status"`
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

	snapshot := Snapshot{
		Status: "ok",
		ByFile: map[string]AccountExpiry{},
	}

	emailToFiles := map[string][]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		file := entry.Name()
		if !isCandidateFile(file) {
			continue
		}

		email, readErr := readEmailFromCredential(filepath.Join(authDir, file))
		if readErr != nil {
			snapshot.ByFile[file] = AccountExpiry{
				Status:  "unavailable",
				Message: readErr.Error(),
			}
			continue
		}
		if email == "" {
			snapshot.ByFile[file] = AccountExpiry{
				Status:  "missing_email",
				Message: "Missing email on credential",
			}
			continue
		}

		normalizedEmail := strings.ToLower(email)
		emailToFiles[normalizedEmail] = append(emailToFiles[normalizedEmail], file)
	}

	if len(snapshot.ByFile) == 0 && len(emailToFiles) == 0 {
		return snapshot, nil
	}

	client := s.HTTPClient
	if client == nil {
		timeout := s.Timeout
		if timeout <= 0 {
			timeout = defaultFetchTimeout
		}
		client = &http.Client{Timeout: timeout}
	}

	sem := make(chan struct{}, defaultMaxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var okCount int
	var shouldBackoff bool
	var backoffReason string

	for email, files := range emailToFiles {
		email := email
		files := append([]string(nil), files...)
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				for _, file := range files {
					snapshot.ByFile[file] = AccountExpiry{
						Status:  "unavailable",
						Message: "expiry request canceled",
					}
				}
				mu.Unlock()
				return
			}
			defer func() { <-sem }()

			expiry, triggerBackoff, reason := s.fetchOne(ctx, client, email)

			mu.Lock()
			for _, file := range files {
				snapshot.ByFile[file] = expiry
			}
			if expiry.Status != "unavailable" {
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
	if okCount == 0 && len(emailToFiles) > 0 {
		snapshot.Status = "unavailable"
		snapshot.Message = "failed to fetch expiry for all accounts"
	}
	s.updateLastGoodSnapshot(snapshot.ByFile)
	return snapshot, nil
}

func (s *Service) fetchOne(ctx context.Context, client *http.Client, email string) (AccountExpiry, bool, string) {
	endpoint := strings.TrimSpace(s.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return AccountExpiry{Status: "unavailable", Message: fmt.Sprintf("invalid expiry endpoint: %v", err)}, false, ""
	}
	query := endpointURL.Query()
	query.Set("email", email)
	endpointURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL.String(), nil)
	if err != nil {
		return AccountExpiry{Status: "unavailable", Message: fmt.Sprintf("build expiry request: %v", err)}, false, ""
	}
	req.Header.Set("Referer", browserReferer)
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("sec-ch-ua", browserSecCHUA)
	req.Header.Set("sec-ch-ua-mobile", browserSecCHUAMobile)
	req.Header.Set("sec-ch-ua-platform", browserSecCHUAPlatform)

	resp, err := client.Do(req)
	if err != nil {
		return AccountExpiry{Status: "unavailable", Message: fmt.Sprintf("request expiry: %v", err)}, false, ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return AccountExpiry{Status: "unavailable", Message: fmt.Sprintf("read expiry response: %v", err)}, false, ""
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := extractErrorMessage(body)
		if message == "" {
			message = "request failed"
		}
		return AccountExpiry{
			Status:  "unavailable",
			Message: fmt.Sprintf("HTTP %d %s", resp.StatusCode, message),
		}, isBackoffHTTPStatus(resp.StatusCode), message
	}

	var payload apiResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return AccountExpiry{Status: "unavailable", Message: fmt.Sprintf("decode expiry response: %v", err)}, false, ""
	}

	if !payload.Success {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = extractAPIError(payload.Error)
		}
		if message == "" {
			message = "upstream returned success=false"
		}
		return AccountExpiry{Status: "unavailable", Message: message}, false, ""
	}

	status := strings.TrimSpace(strings.ToLower(payload.Data.Status))
	if status == "" {
		status = "available"
	}

	return AccountExpiry{
		DaysRemaining: intPointer(payload.Data.DaysRemaining),
		ExpireDate:    strings.TrimSpace(payload.Data.ExpireDate),
		JoinDate:      strings.TrimSpace(payload.Data.JoinDate),
		OrderID:       strings.TrimSpace(payload.Data.OrderID),
		Status:        status,
	}, false, ""
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
		"expiry fetch backoff active until %s: %s",
		s.backoffUntil.UTC().Format(time.RFC3339),
		s.backoffReason,
	)
	cached := cloneExpiryMap(s.lastGoodSnapshot)
	if len(cached) > 0 {
		return Snapshot{Status: "stale", Message: message, ByFile: cached}, true
	}
	return Snapshot{Status: "unavailable", Message: message, ByFile: map[string]AccountExpiry{}}, true
}

func (s *Service) activateBackoff(reason string) Snapshot {
	if strings.TrimSpace(reason) == "" {
		reason = "rate limited by upstream"
	}
	until := time.Now().Add(randomDurationInRange(minBackoffDuration, maxBackoffDuration))

	s.mu.Lock()
	s.backoffUntil = until
	s.backoffReason = reason
	cached := cloneExpiryMap(s.lastGoodSnapshot)
	s.mu.Unlock()

	message := fmt.Sprintf(
		"expiry fetch backoff active until %s: %s",
		until.UTC().Format(time.RFC3339),
		reason,
	)
	if len(cached) > 0 {
		return Snapshot{Status: "stale", Message: message, ByFile: cached}
	}
	return Snapshot{Status: "unavailable", Message: message, ByFile: map[string]AccountExpiry{}}
}

func (s *Service) updateLastGoodSnapshot(byFile map[string]AccountExpiry) {
	if len(byFile) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastGoodSnapshot == nil {
		s.lastGoodSnapshot = map[string]AccountExpiry{}
	}
	for file, expiry := range byFile {
		if expiry.Status != "unavailable" {
			s.lastGoodSnapshot[file] = expiry
		}
	}
}

func isBackoffHTTPStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusForbidden || status >= http.StatusInternalServerError
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

func cloneExpiryMap(src map[string]AccountExpiry) map[string]AccountExpiry {
	if len(src) == 0 {
		return map[string]AccountExpiry{}
	}
	out := make(map[string]AccountExpiry, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func readEmailFromCredential(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read credential: %v", err)
	}

	var cred credentialFile
	if err := json.Unmarshal(raw, &cred); err != nil {
		return "", fmt.Errorf("decode credential: %v", err)
	}
	return stringValue(cred.Email), nil
}

func isCandidateFile(name string) bool {
	return strings.HasPrefix(name, "codex-") && strings.HasSuffix(name, ".json")
}

func extractErrorMessage(body []byte) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		return ""
	}

	var payload struct {
		Error interface{} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return truncateMessage(message)
	}
	if extracted := extractAPIError(payload.Error); extracted != "" {
		return extracted
	}
	return truncateMessage(message)
}

func extractAPIError(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}:
		for _, key := range []string{"message", "error", "detail"} {
			if nested, ok := typed[key]; ok {
				if message := extractAPIError(nested); message != "" {
					return message
				}
			}
		}
	}
	return ""
}

func truncateMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 160 {
		return message
	}
	return message[:157] + "..."
}

func intPointer(value interface{}) *int {
	switch typed := value.(type) {
	case float64:
		parsed := int(typed)
		return &parsed
	case float32:
		parsed := int(typed)
		return &parsed
	case int:
		parsed := typed
		return &parsed
	case int64:
		parsed := int(typed)
		return &parsed
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return nil
		}
		value := int(parsed)
		return &value
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return nil
		}
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}
