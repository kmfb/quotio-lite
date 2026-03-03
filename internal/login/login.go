package login

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"quotio-lite/internal/accounts"
)

type Service struct {
	AuthDir      string
	CLIProxyPath string
}

type Result struct {
	File string `json:"file"`
}

func (s Service) LoginCodex(ctx context.Context, incognito bool) (Result, error) {
	if _, err := os.Stat(s.CLIProxyPath); err != nil {
		return Result{}, fmt.Errorf("cli proxy unavailable: %w", err)
	}

	before, err := (accounts.Store{AuthDir: s.AuthDir}).Snapshot()
	if err != nil {
		return Result{}, err
	}

	cfgPath, cfgDir, err := s.writeTempConfig()
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(cfgDir)

	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()

	args := []string{"-config", cfgPath, "-codex-login"}
	if incognito {
		args = append(args, "-incognito")
	}

	cmd := exec.CommandContext(ctx, s.CLIProxyPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("codex login failed: %w (%s)", err, out.String())
	}

	after, err := (accounts.Store{AuthDir: s.AuthDir}).Snapshot()
	if err != nil {
		return Result{}, err
	}

	latest := accounts.DetectLatestChanged(before, after)
	if latest == "" {
		return Result{}, fmt.Errorf("login finished but no credential file changed")
	}

	return Result{File: latest}, nil
}

func (s Service) writeTempConfig() (string, string, error) {
	tmpDir, err := os.MkdirTemp("", "quotio-lite-login-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}

	cfgPath := filepath.Join(tmpDir, "config.yaml")
	content := fmt.Sprintf(`host: "127.0.0.1"
port: 18418
auth-dir: %q
api-keys:
  - "quotio-lite-login"
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
`, s.AuthDir)

	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		return "", "", fmt.Errorf("write temp config: %w", err)
	}
	return cfgPath, tmpDir, nil
}
