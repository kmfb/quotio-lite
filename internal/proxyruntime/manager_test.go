package proxyruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quotio-lite/internal/accounts"
)

func TestMaskAPIKey(t *testing.T) {
	masked := maskAPIKey("sk-abcdefghijklmnopqrstuvwxyz123456")
	if masked != "sk-abcd********3456" {
		t.Fatalf("unexpected masked value: %q", masked)
	}
}

func TestParseLsofOutput(t *testing.T) {
	raw := []byte("COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME\nCLIProxyA 4321 tian   10u  IPv4 0x01      0t0  TCP 127.0.0.1:8317 (LISTEN)\n")
	pid, command := parseLsofOutput(raw)
	if pid != 4321 {
		t.Fatalf("unexpected pid: %d", pid)
	}
	if command != "CLIProxyA" {
		t.Fatalf("unexpected command: %q", command)
	}
}

func TestSelectProxyPoolFiltersToHealthyAccounts(t *testing.T) {
	window18 := 18.0
	window35 := 35.0
	window94 := 94.0
	window28 := 28.0
	window100 := 100.0

	records := []accounts.AccountRecord{
		{
			File:   "codex-fbbee47f-stardusted@163.com-team.json",
			Email:  "stardusted@163.com",
			Status: "ok",
			Usage: accounts.AccountUsage{
				Status:   "ok",
				Window5H: accounts.UsageWindow{UsedPercent: &window18},
				Weekly:   accounts.UsageWindow{UsedPercent: &window35},
			},
			LastProbeMessage: `{"id":"resp_ok"}`,
		},
		{
			File:   "codex-496d5237-stardusted01@163.com-team.json",
			Email:  "stardusted01@163.com",
			Status: "usage_limited",
			Usage: accounts.AccountUsage{
				Status:   "ok",
				Window5H: accounts.UsageWindow{UsedPercent: &window94},
				Weekly:   accounts.UsageWindow{UsedPercent: &window28},
			},
			LastProbeMessage: `{"error":{"code":"insufficient_quota"}}`,
		},
		{
			File:   "codex-01b53e73-stardusted@126.com-team.json",
			Email:  "stardusted@126.com",
			Status: "ok",
			Usage: accounts.AccountUsage{
				Status:   "ok",
				Window5H: accounts.UsageWindow{UsedPercent: &window18},
				Weekly:   accounts.UsageWindow{UsedPercent: &window100},
			},
			LastProbeMessage: `{"error":{"message":"status 403","code":"insufficient_quota"}}`,
		},
	}

	selected := selectProxyPool(records)
	if len(selected) != 1 {
		t.Fatalf("selected len = %d", len(selected))
	}
	if selected[0].Email != "stardusted@163.com" {
		t.Fatalf("selected email = %q", selected[0].Email)
	}
}

func TestSyncAccountsWritesManagedPoolAndConfig(t *testing.T) {
	authDir := t.TempDir()
	proxyDir := t.TempDir()

	goodFile := "codex-fbbee47f-stardusted@163.com-team.json"
	badFile := "codex-01b53e73-stardusted@126.com-team.json"
	if err := os.WriteFile(filepath.Join(authDir, goodFile), []byte(`{"email":"stardusted@163.com"}`), 0o600); err != nil {
		t.Fatalf("write good credential: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, badFile), []byte(`{"email":"stardusted@126.com"}`), 0o600); err != nil {
		t.Fatalf("write bad credential: %v", err)
	}

	window18 := 18.0
	window35 := 35.0
	window100 := 100.0
	manager := &Manager{
		authDir:      authDir,
		host:         DefaultHost,
		port:         DefaultPort,
		proxyDir:     proxyDir,
		configPath:   filepath.Join(proxyDir, "config.yaml"),
		statePath:    filepath.Join(proxyDir, "state.json"),
		poolDir:      filepath.Join(proxyDir, "authpool"),
		cliProxyPath: filepath.Join(proxyDir, "CLIProxyAPI"),
	}

	err := manager.SyncAccounts([]accounts.AccountRecord{
		{
			File:   goodFile,
			Email:  "stardusted@163.com",
			Status: "ok",
			Usage: accounts.AccountUsage{
				Status:   "ok",
				Window5H: accounts.UsageWindow{UsedPercent: &window18},
				Weekly:   accounts.UsageWindow{UsedPercent: &window35},
			},
		},
		{
			File:   badFile,
			Email:  "stardusted@126.com",
			Status: "ok",
			Usage: accounts.AccountUsage{
				Status:   "ok",
				Window5H: accounts.UsageWindow{UsedPercent: &window18},
				Weekly:   accounts.UsageWindow{UsedPercent: &window100},
			},
			LastProbeMessage: `{"error":{"code":"insufficient_quota"}}`,
		},
	})
	if err != nil {
		t.Fatalf("SyncAccounts: %v", err)
	}

	if !manager.usingManagedPool {
		t.Fatalf("expected managed pool to be enabled")
	}
	if len(manager.selectedFiles) != 1 || manager.selectedFiles[0] != goodFile {
		t.Fatalf("selectedFiles = %#v", manager.selectedFiles)
	}
	if _, err := os.Stat(filepath.Join(manager.poolDir, goodFile)); err != nil {
		t.Fatalf("good pooled file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.poolDir, badFile)); !os.IsNotExist(err) {
		t.Fatalf("bad pooled file should not exist, err=%v", err)
	}

	raw, err := os.ReadFile(manager.configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := string(raw); !containsAll(got, manager.poolDir, "strategy: \"round-robin\"") {
		t.Fatalf("config did not reference managed pool: %s", got)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
