package config

import (
	"path/filepath"
	"testing"
)

func TestLoadServiceMode(t *testing.T) {
	t.Setenv(envMode, ModeService)
	t.Setenv(envFrontendDistDir, "/tmp/frontend-dist")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Mode != ModeService {
		t.Fatalf("mode = %q", cfg.Mode)
	}
	if cfg.FrontendDistDir != "/tmp/frontend-dist" {
		t.Fatalf("frontend dist dir = %q", cfg.FrontendDistDir)
	}
}

func TestLoadDefaultsToDevModeAndLocalDist(t *testing.T) {
	t.Setenv(envMode, "")
	t.Setenv(envFrontendDistDir, "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Mode != ModeDev {
		t.Fatalf("mode = %q", cfg.Mode)
	}
	if filepath.Base(cfg.FrontendDistDir) != "dist" {
		t.Fatalf("unexpected frontend dist dir: %q", cfg.FrontendDistDir)
	}
}
