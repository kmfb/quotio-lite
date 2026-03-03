package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	defaultHost           = "127.0.0.1"
	defaultPort           = 18417
	defaultManagementPort = 8317
	defaultProbeModel     = "gpt-5.1-codex-mini"
	defaultProbeTimeout   = 50 * time.Second
	envHost               = "QUOTIO_LITE_HOST"
	envPort               = "QUOTIO_LITE_PORT"
	envAuthDir            = "QUOTIO_LITE_AUTH_DIR"
	envCLIProxyPath       = "QUOTIO_LITE_CLIPROXY_PATH"
	envProbeModel         = "QUOTIO_LITE_PROBE_MODEL"
	envProbeTimeoutSec    = "QUOTIO_LITE_PROBE_TIMEOUT_SEC"
)

type Config struct {
	Host         string
	Port         int
	AuthDir      string
	CLIProxyPath string
	ProbeModel   string
	ProbeTimeout time.Duration
}

func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home dir: %w", err)
	}

	cfg := Config{
		Host:         getenvOr(envHost, defaultHost),
		Port:         defaultPort,
		AuthDir:      getenvOr(envAuthDir, filepath.Join(home, ".cli-proxy-api")),
		CLIProxyPath: getenvOr(envCLIProxyPath, filepath.Join(home, "Library", "Application Support", "Quotio", "CLIProxyAPI")),
		ProbeModel:   getenvOr(envProbeModel, defaultProbeModel),
		ProbeTimeout: defaultProbeTimeout,
	}

	if raw := os.Getenv(envPort); raw != "" {
		port, convErr := strconv.Atoi(raw)
		if convErr != nil || port <= 0 || port > 65535 {
			return Config{}, fmt.Errorf("invalid %s=%q", envPort, raw)
		}
		cfg.Port = port
	}

	if raw := os.Getenv(envProbeTimeoutSec); raw != "" {
		seconds, convErr := strconv.Atoi(raw)
		if convErr != nil || seconds <= 0 {
			return Config{}, fmt.Errorf("invalid %s=%q", envProbeTimeoutSec, raw)
		}
		cfg.ProbeTimeout = time.Duration(seconds) * time.Second
	}

	return cfg, nil
}

func getenvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
