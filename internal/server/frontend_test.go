package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quotio-lite/internal/config"
)

func TestHandlerServiceModeServesFrontendIndex(t *testing.T) {
	distDir := t.TempDir()
	writeFrontendFixture(t, distDir, "index.html", "<html>service shell</html>")
	writeFrontendFixture(t, distDir, filepath.Join("assets", "app.js"), "console.log('ok')")

	app := New(config.Config{Mode: config.ModeService, FrontendDistDir: distDir})
	handler := app.Handler()

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	assetRec := httptest.NewRecorder()
	handler.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK {
		t.Fatalf("asset status = %d", assetRec.Code)
	}
	if !strings.Contains(assetRec.Body.String(), "console.log") {
		t.Fatalf("expected built asset body, got %q", assetRec.Body.String())
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/accounts/demo.json", nil)
	pageRec := httptest.NewRecorder()
	handler.ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("page status = %d", pageRec.Code)
	}
	if !strings.Contains(pageRec.Body.String(), "service shell") {
		t.Fatalf("expected SPA fallback body, got %q", pageRec.Body.String())
	}
}

func TestHandlerServiceModePreservesAPIJSON(t *testing.T) {
	distDir := t.TempDir()
	writeFrontendFixture(t, distDir, "index.html", "<html>service shell</html>")

	app := New(config.Config{Mode: config.ModeService, FrontendDistDir: distDir})
	handler := app.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content type = %q", got)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if payload["mode"] != config.ModeService {
		t.Fatalf("mode = %v", payload["mode"])
	}
	if payload["frontendServing"] != true {
		t.Fatalf("frontendServing = %v", payload["frontendServing"])
	}
}

func TestHandlerDevModeDoesNotServeFrontendRoutes(t *testing.T) {
	app := New(config.Config{Mode: config.ModeDev})
	handler := app.Handler()

	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func writeFrontendFixture(t *testing.T, distDir, name, contents string) {
	t.Helper()
	fullPath := filepath.Join(distDir, name)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
