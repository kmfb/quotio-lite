package login

import (
	"strings"
	"testing"
)

func TestParseCapabilities(t *testing.T) {
	help := `CLIProxyAPI Version: 6.8.47, Commit: abcdef, BuiltAt: 2026-03-07T12:42:28Z
Usage of /tmp/CLIProxyAPI
  -codex-device-login
    Login to Codex using device code flow
  -codex-login
    Login to Codex using OAuth
  -no-browser
    Don't open browser automatically for OAuth
`

	caps := parseCapabilities(help)
	if caps.Version != "6.8.47" {
		t.Fatalf("expected version 6.8.47, got %q", caps.Version)
	}
	if !caps.SupportsCodexLogin {
		t.Fatal("expected SupportsCodexLogin=true")
	}
	if !caps.SupportsCodexDeviceLogin {
		t.Fatal("expected SupportsCodexDeviceLogin=true")
	}
	if !caps.SupportsNoBrowser {
		t.Fatal("expected SupportsNoBrowser=true")
	}
	if caps.SupportsIncognito {
		t.Fatal("expected SupportsIncognito=false")
	}
}

func TestBuildLoginArgsDefaultsToOAuthWithoutIncognito(t *testing.T) {
	caps := Capabilities{SupportsCodexLogin: true, SupportsIncognito: false}
	args, err := buildLoginArgs("", "/tmp/config.yaml", caps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-codex-login") {
		t.Fatalf("expected -codex-login in args: %v", args)
	}
	if strings.Contains(joined, "-incognito") {
		t.Fatalf("did not expect -incognito in args: %v", args)
	}
}

func TestBuildLoginArgsDeviceModeRequiresCapability(t *testing.T) {
	_, err := buildLoginArgs(LoginModeDevice, "/tmp/config.yaml", Capabilities{})
	if err == nil {
		t.Fatal("expected error when device login capability is missing")
	}

	args, err := buildLoginArgs(LoginModeDevice, "/tmp/config.yaml", Capabilities{SupportsCodexDeviceLogin: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-codex-device-login") {
		t.Fatalf("expected -codex-device-login in args: %v", args)
	}
}
