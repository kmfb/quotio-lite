package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"quotio-lite/internal/accounts"
)

type Service struct {
	AuthDir        string
	CLIProxyPath   string
	Model          string
	RequestTimeout time.Duration
}

type Result struct {
	HTTPStatus     int    `json:"httpStatus"`
	LatencyMS      int64  `json:"latencyMs"`
	Classification string `json:"classification"`
	RawSnippet     string `json:"rawSnippet"`
}

func (s Service) Run(ctx context.Context, file string) (Result, error) {
	if s.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.RequestTimeout)
		defer cancel()
	}

	if _, err := os.Stat(s.CLIProxyPath); err != nil {
		return Result{}, fmt.Errorf("cli proxy unavailable: %w", err)
	}

	srcPath, err := accounts.ResolveCredentialPath(s.AuthDir, file)
	if err != nil {
		return Result{}, err
	}

	tempDir, err := os.MkdirTemp("", "quotio-lite-probe-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	dstPath := filepath.Join(tempDir, file)
	if err := copyFile(srcPath, dstPath); err != nil {
		return Result{}, err
	}

	port, err := freeLocalPort()
	if err != nil {
		return Result{}, err
	}
	apiKey := "quotio-lite-probe-key"

	cfgPath, err := writeProbeConfig(tempDir, port, apiKey)
	if err != nil {
		return Result{}, err
	}

	cmd := exec.Command(s.CLIProxyPath, "-config", cfgPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start cli proxy for probe: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitCtx, waitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer waitCancel()
	if err := waitForServer(waitCtx, baseURL); err != nil {
		return Result{}, fmt.Errorf("probe server startup timeout: %w (%s)", err, truncate(out.String(), 300))
	}

	httpClient := &http.Client{Timeout: 25 * time.Second}
	started := time.Now()

	status, body, err := runModelsCheck(httpClient, baseURL, apiKey)
	if err != nil {
		return Result{
			HTTPStatus:     0,
			LatencyMS:      time.Since(started).Milliseconds(),
			Classification: "network_error",
			RawSnippet:     truncate(err.Error(), 300),
		}, nil
	}
	if status != http.StatusOK {
		return Result{
			HTTPStatus:     status,
			LatencyMS:      time.Since(started).Milliseconds(),
			Classification: classify(status, body),
			RawSnippet:     truncate(body, 300),
		}, nil
	}

	status, body, err = runChatCheck(httpClient, baseURL, apiKey, s.Model)
	if err != nil {
		return Result{
			HTTPStatus:     0,
			LatencyMS:      time.Since(started).Milliseconds(),
			Classification: "network_error",
			RawSnippet:     truncate(err.Error(), 300),
		}, nil
	}

	classification := classify(status, body)
	if status == http.StatusOK {
		classification = "ok"
	}

	return Result{
		HTTPStatus:     status,
		LatencyMS:      time.Since(started).Milliseconds(),
		Classification: classification,
		RawSnippet:     truncate(body, 300),
	}, nil
}

func runModelsCheck(client *http.Client, baseURL, apiKey string) (int, string, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(body), nil
}

func runChatCheck(client *http.Client, baseURL, apiKey, model string) (int, string, error) {
	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{{
			"role":    "user",
			"content": "ping",
		}},
		"max_tokens": 8,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(body), nil
}

func classify(status int, body string) string {
	if status == http.StatusOK {
		return "ok"
	}

	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, "usage_limit_reached"), strings.Contains(lower, "quota"), strings.Contains(lower, "rate limit"):
		return "usage_limited"
	case strings.Contains(lower, "plan_type"), strings.Contains(lower, "free plan"), strings.Contains(lower, "plan mismatch"):
		return "plan_mismatch"
	case strings.Contains(lower, "expired"), strings.Contains(lower, "invalid_grant"), strings.Contains(lower, "invalid token"), strings.Contains(lower, "unauthorized"):
		return "auth_expired"
	case strings.Contains(lower, "disabled"):
		return "disabled"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "auth_expired"
	case status == http.StatusBadGateway || status == http.StatusGatewayTimeout || status == http.StatusServiceUnavailable:
		return "network_error"
	default:
		return "unknown"
	}
}

func waitForServer(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get(baseURL + "/")
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

func freeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate local port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func writeProbeConfig(authDir string, port int, apiKey string) (string, error) {
	cfgPath := filepath.Join(authDir, "config.yaml")
	content := fmt.Sprintf(`host: "127.0.0.1"
port: %d
auth-dir: %q
api-keys:
  - %q
remote-management:
  allow-remote: false
debug: false
logging-to-file: false
routing:
  strategy: "round-robin"
quota-exceeded:
  switch-project: true
  switch-preview-model: true
request-retry: 1
max-retry-interval: 5
`, port, authDir, apiKey)

	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write probe config: %w", err)
	}
	return cfgPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source credential: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create probe credential copy: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy credential: %w", err)
	}
	return nil
}

func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n]
}
