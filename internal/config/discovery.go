package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const envQuotioConfigPath = "QUOTIO_LITE_QUOTIO_CONFIG_PATH"

var (
	hostPattern      = regexp.MustCompile(`(?m)^\s*host:\s*"?([^"\n]+)"?\s*$`)
	portPattern      = regexp.MustCompile(`(?m)^\s*port:\s*"?([0-9]+)"?\s*$`)
	secretKeyPattern = regexp.MustCompile(`(?m)^\s*secret-key:\s*"?([^"\n]+)"?\s*$`)
)

type discoveredConfig struct {
	Host            string
	Port            int
	SecretKey       string
	SecretKeyHashed bool
	Path            string
	Found           bool
}

func discoverQuotioConfig(home string) discoveredConfig {
	for _, path := range configCandidates(home) {
		cfg, ok := parseConfigFile(path)
		if !ok {
			continue
		}
		cfg.Path = path
		cfg.Found = true
		return cfg
	}
	return discoveredConfig{}
}

func configCandidates(home string) []string {
	out := []string{}
	if v := strings.TrimSpace(os.Getenv(envQuotioConfigPath)); v != "" {
		out = append(out, v)
	}
	out = append(out,
		filepath.Join(home, "Library", "Application Support", "Quotio", "config.yaml"),
		filepath.Join(home, "Library", "Application Support", "CLIProxyAPI", "config.yaml"),
	)
	return deduplicate(out)
}

func parseConfigFile(path string) (discoveredConfig, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return discoveredConfig{}, false
	}
	text := string(raw)

	cfg := discoveredConfig{}
	if m := hostPattern.FindStringSubmatch(text); len(m) == 2 {
		cfg.Host = strings.TrimSpace(m[1])
	}
	if m := portPattern.FindStringSubmatch(text); len(m) == 2 {
		if port, convErr := strconv.Atoi(strings.TrimSpace(m[1])); convErr == nil && port > 0 && port <= 65535 {
			cfg.Port = port
		}
	}
	if m := secretKeyPattern.FindStringSubmatch(text); len(m) == 2 {
		cfg.SecretKey = strings.TrimSpace(m[1])
		cfg.SecretKeyHashed = looksLikeBcryptHash(cfg.SecretKey)
	}

	if cfg.Host == "" && cfg.Port == 0 && cfg.SecretKey == "" {
		return discoveredConfig{}, false
	}
	return cfg, true
}

func looksLikeBcryptHash(v string) bool {
	return strings.HasPrefix(v, "$2a$") || strings.HasPrefix(v, "$2b$") || strings.HasPrefix(v, "$2y$")
}

func deduplicate(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func formatBaseURL(host string, port int) string {
	h := strings.TrimSpace(host)
	if h == "" {
		h = defaultHost
	}
	p := port
	if p <= 0 || p > 65535 {
		p = defaultManagementPort
	}
	return fmt.Sprintf("http://%s:%d", h, p)
}
