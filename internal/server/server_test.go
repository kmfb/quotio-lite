package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quotio-lite/internal/accountexpiry"
	"quotio-lite/internal/config"
)

func TestAccountsAPIIncludesExpiryMetadata(t *testing.T) {
	authDir := t.TempDir()
	writeAccountFixture(t, authDir, "codex-person@example.com-team.json", `{"email":"person@example.com","type":"team","account_id":"acct-123"}`)

	app := New(config.Config{AuthDir: authDir})
	app.expiry = &accountexpiry.Service{
		AuthDir:  authDir,
		Endpoint: "https://codexcn.com/api/check-expiry",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"success":true,"data":{"days_remaining":21,"expire_date":"2026-03-30 07:17:08","join_date":"2026-02-28 07:17:08","order_id":"Team_1772263027264_372c87","status":"active"}}`), nil
		})},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []struct {
			File   string `json:"file"`
			Email  string `json:"email"`
			Expiry struct {
				DaysRemaining *int   `json:"daysRemaining"`
				ExpireDate    string `json:"expireDate"`
				Status        string `json:"status"`
				OrderID       string `json:"orderId"`
			} `json:"expiry"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode accounts payload: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items len = %d", len(payload.Items))
	}
	if payload.Items[0].Expiry.DaysRemaining == nil || *payload.Items[0].Expiry.DaysRemaining != 21 {
		t.Fatalf("days remaining = %+v", payload.Items[0].Expiry.DaysRemaining)
	}
	if payload.Items[0].Expiry.Status != "active" {
		t.Fatalf("expiry status = %q", payload.Items[0].Expiry.Status)
	}
	if payload.Items[0].Expiry.ExpireDate == "" {
		t.Fatalf("expire date should not be empty")
	}
	if payload.Items[0].Expiry.OrderID == "" {
		t.Fatalf("order id should not be empty")
	}
}

func TestAccountDetailAPIHandlesMissingExpiryEmailGracefully(t *testing.T) {
	authDir := t.TempDir()
	writeAccountFixture(t, authDir, "codex-no-email-team.json", `{"email":"","type":"team","account_id":"acct-123"}`)

	app := New(config.Config{AuthDir: authDir})
	app.expiry = &accountexpiry.Service{AuthDir: authDir}

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/codex-no-email-team.json", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Expiry struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"expiry"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode detail payload: %v", err)
	}
	if payload.Expiry.Status != "missing_email" {
		t.Fatalf("expiry status = %q", payload.Expiry.Status)
	}
	if payload.Expiry.Message == "" {
		t.Fatalf("expiry message should not be empty")
	}
}

func writeAccountFixture(t *testing.T, authDir, file, contents string) {
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
