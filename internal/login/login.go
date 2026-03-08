package login

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"quotio-lite/internal/accounts"
)

type Service struct {
	AuthDir      string
	CLIProxyPath string
}

type LoginMode string

const (
	LoginModeOAuth  LoginMode = "oauth"
	LoginModeDevice LoginMode = "device"
)

type Capabilities struct {
	Version                  string `json:"version"`
	SupportsCodexLogin       bool   `json:"supportsCodexLogin"`
	SupportsCodexDeviceLogin bool   `json:"supportsCodexDeviceLogin"`
	SupportsNoBrowser        bool   `json:"supportsNoBrowser"`
	SupportsIncognito        bool   `json:"supportsIncognito"`
}

type Result struct {
	File         string       `json:"file"`
	Mode         LoginMode    `json:"mode"`
	Capabilities Capabilities `json:"capabilities"`
}

func (s Service) LoginCodex(ctx context.Context, mode LoginMode) (Result, error) {
	if _, err := os.Stat(s.CLIProxyPath); err != nil {
		return Result{}, fmt.Errorf("cli proxy unavailable: %w", err)
	}

	capabilities, err := s.Capabilities(ctx)
	if err != nil {
		return Result{}, err
	}

	if mode == "" {
		mode = LoginModeOAuth
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

	args, err := buildLoginArgs(mode, cfgPath, capabilities)
	if err != nil {
		return Result{}, err
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

	return Result{File: latest, Mode: mode, Capabilities: capabilities}, nil
}

func (s Service) Capabilities(ctx context.Context) (Capabilities, error) {
	if _, err := os.Stat(s.CLIProxyPath); err != nil {
		return Capabilities{}, fmt.Errorf("cli proxy unavailable: %w", err)
	}

	helpCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(helpCtx, s.CLIProxyPath, "-h")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()

	text := out.String()
	caps := parseCapabilities(text)
	if text == "" {
		return Capabilities{}, fmt.Errorf("failed to inspect cli proxy capabilities: empty help output")
	}
	return caps, nil
}

func buildLoginArgs(mode LoginMode, cfgPath string, caps Capabilities) ([]string, error) {
	args := []string{"-config", cfgPath}

	switch mode {
	case "", LoginModeOAuth:
		if !caps.SupportsCodexLogin {
			return nil, fmt.Errorf("CLIProxyAPI does not support Codex OAuth login in this version")
		}
		args = append(args, "-codex-login")
	case LoginModeDevice:
		if !caps.SupportsCodexDeviceLogin {
			return nil, fmt.Errorf("CLIProxyAPI does not support Codex device login in this version")
		}
		args = append(args, "-codex-device-login")
	default:
		return nil, fmt.Errorf("unsupported login mode: %s", mode)
	}

	return args, nil
}

var versionPattern = regexp.MustCompile(`CLIProxyAPI Version:\s*([^,\s]+)`)

func parseCapabilities(help string) Capabilities {
	caps := Capabilities{
		SupportsCodexLogin:       strings.Contains(help, "-codex-login"),
		SupportsCodexDeviceLogin: strings.Contains(help, "-codex-device-login"),
		SupportsNoBrowser:        strings.Contains(help, "-no-browser"),
		SupportsIncognito:        strings.Contains(help, "-incognito"),
	}

	if matches := versionPattern.FindStringSubmatch(help); len(matches) == 2 {
		caps.Version = strings.TrimSpace(matches[1])
	}

	return caps
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
