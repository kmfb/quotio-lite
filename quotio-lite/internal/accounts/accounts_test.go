package accounts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCredentialPathRejectsTraversal(t *testing.T) {
	authDir := t.TempDir()
	_, err := ResolveCredentialPath(authDir, "../evil.json")
	if err == nil {
		t.Fatalf("expected traversal path to fail")
	}
}

func TestResolveCredentialPathAcceptsValidFile(t *testing.T) {
	authDir := t.TempDir()
	file := "codex-user@example.com-team.json"
	full := filepath.Join(authDir, file)
	if err := os.WriteFile(full, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	resolved, err := ResolveCredentialPath(authDir, file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != full {
		t.Fatalf("resolved mismatch: got %s want %s", resolved, full)
	}
}
