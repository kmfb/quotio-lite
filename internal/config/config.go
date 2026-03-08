package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ModeDev               = "dev"
	ModeService           = "service"
	defaultHost           = "127.0.0.1"
	defaultPort           = 18417
	defaultManagementPort = 8317
	defaultProbeModel     = "gpt-5.1-codex-mini"
	defaultProbeTimeout   = 50 * time.Second
	defaultFrontendDist   = "web/dist"
	envMode               = "QUOTIO_LITE_MODE"
	envHost               = "QUOTIO_LITE_HOST"
	envPort               = "QUOTIO_LITE_PORT"
	envAuthDir            = "QUOTIO_LITE_AUTH_DIR"
	envCLIProxyPath       = "QUOTIO_LITE_CLIPROXY_PATH"
	envProbeModel         = "QUOTIO_LITE_PROBE_MODEL"
	envProbeTimeoutSec    = "QUOTIO_LITE_PROBE_TIMEOUT_SEC"
	envFrontendDistDir    = "QUOTIO_LITE_FRONTEND_DIST_DIR"
)

type Config struct {
	Mode            string
	Host            string
	Port            int
	AuthDir         string
	CLIProxyPath    string
	ProbeModel      string
	ProbeTimeout    time.Duration
	FrontendDistDir string
}

func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home dir: %w", err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("resolve working dir: %w", err)
	}

	mode := strings.TrimSpace(getenvOr(envMode, ModeDev))
	if mode == "" {
		mode = ModeDev
	}
	if mode != ModeDev && mode != ModeService {
		return Config{}, fmt.Errorf("invalid %s=%q", envMode, mode)
	}

	cfg := Config{
		Mode:            mode,
		Host:            getenvOr(envHost, defaultHost),
		Port:            defaultPort,
		AuthDir:         getenvOr(envAuthDir, filepath.Join(home, ".cli-proxy-api")),
		CLIProxyPath:    getenvOr(envCLIProxyPath, filepath.Join(home, ".quotio-lite", "bin", "CLIProxyAPI")),
		ProbeModel:      getenvOr(envProbeModel, defaultProbeModel),
		ProbeTimeout:    defaultProbeTimeout,
		FrontendDistDir: getenvOr(envFrontendDistDir, filepath.Join(workingDir, defaultFrontendDist)),
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
