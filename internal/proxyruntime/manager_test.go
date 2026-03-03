package proxyruntime

import "testing"

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
