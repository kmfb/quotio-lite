package accountexpiry

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFetchDedupesByEmailAndHandlesMissingEmail(t *testing.T) {
	authDir := t.TempDir()
	writeCredentialFixture(t, authDir, "codex-a-team.json", `{"email":"person@example.com"}`)
	writeCredentialFixture(t, authDir, "codex-b-team.json", `{"email":"person@example.com"}`)
	writeCredentialFixture(t, authDir, "codex-c-team.json", `{"email":""}`)

	var mu sync.Mutex
	requestCount := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		if got := r.URL.Query().Get("email"); got != "person@example.com" {
			t.Fatalf("email query = %q", got)
		}
		if got := r.Header.Get("Referer"); got != browserReferer {
			t.Fatalf("referer = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != browserUserAgent {
			t.Fatalf("user-agent = %q", got)
		}

		return jsonResponse(http.StatusOK, `{"success":true,"data":{"days_remaining":21,"expire_date":"2026-03-30 07:17:08","join_date":"2026-02-28 07:17:08","order_id":"Team_1772263027264_372c87","status":"active"}}`), nil
	})}

	service := &Service{AuthDir: authDir, Endpoint: defaultEndpoint, HTTPClient: client}
	snapshot, err := service.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.Status != "ok" {
		t.Fatalf("snapshot status = %q", snapshot.Status)
	}
	if len(snapshot.ByFile) != 3 {
		t.Fatalf("snapshot size = %d", len(snapshot.ByFile))
	}
	if snapshot.ByFile["codex-c-team.json"].Status != "missing_email" {
		t.Fatalf("missing-email status = %q", snapshot.ByFile["codex-c-team.json"].Status)
	}
	if got := snapshot.ByFile["codex-a-team.json"].Status; got != "active" {
		t.Fatalf("status = %q", got)
	}
	if got := snapshot.ByFile["codex-a-team.json"].DaysRemaining; got == nil || *got != 21 {
		t.Fatalf("days remaining = %+v", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if requestCount != 1 {
		t.Fatalf("requestCount = %d", requestCount)
	}
}

func TestFetchHandlesSuccessFalseGracefully(t *testing.T) {
	authDir := t.TempDir()
	writeCredentialFixture(t, authDir, "codex-a-team.json", `{"email":"person@example.com"}`)

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"success":false,"message":"not found"}`), nil
	})}

	service := &Service{AuthDir: authDir, Endpoint: defaultEndpoint, HTTPClient: client}
	snapshot, err := service.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if snapshot.Status != "unavailable" {
		t.Fatalf("snapshot status = %q", snapshot.Status)
	}
	if got := snapshot.ByFile["codex-a-team.json"].Message; got != "not found" {
		t.Fatalf("message = %q", got)
	}
}

func writeCredentialFixture(t *testing.T, authDir, file, contents string) {
	t.Helper()
	fullPath := filepath.Join(authDir, file)
	if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
