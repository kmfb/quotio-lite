package managementusage

import (
	"net/http"
	"testing"
	"time"
)

func TestPickWindows(t *testing.T) {
	five := &usageWindow{LimitWindowSeconds: 18000, UsedPercent: 28}
	weekly := &usageWindow{LimitWindowSeconds: 604800, UsedPercent: 11}
	limit := rateLimitWindow{
		PrimaryWindow:   five,
		SecondaryWindow: weekly,
	}

	gotFive, gotWeekly := pickWindows(limit)
	if gotFive != five {
		t.Fatalf("expected 5h window from primary")
	}
	if gotWeekly != weekly {
		t.Fatalf("expected weekly window from secondary")
	}
}

func TestToWindow(t *testing.T) {
	window := toWindow(&usageWindow{
		UsedPercent: 28,
		ResetAt:     1772559029,
	})

	if window.UsedPercent == nil || int(*window.UsedPercent) != 28 {
		t.Fatalf("unexpected used percent: %+v", window.UsedPercent)
	}
	if window.ResetAt != "2026-03-03T17:30:29Z" {
		t.Fatalf("unexpected resetAt: %s", window.ResetAt)
	}
}

func TestExtractErrorMessage(t *testing.T) {
	raw := []byte(`{"error":{"message":"usage_limit_reached"}}`)
	msg := extractErrorMessage(raw)
	if msg != "usage_limit_reached" {
		t.Fatalf("unexpected error message: %q", msg)
	}
}

func TestIsBackoffHTTPStatus(t *testing.T) {
	if !isBackoffHTTPStatus(http.StatusTooManyRequests) {
		t.Fatalf("429 should trigger backoff")
	}
	if !isBackoffHTTPStatus(http.StatusForbidden) {
		t.Fatalf("403 should trigger backoff")
	}
	if isBackoffHTTPStatus(http.StatusUnauthorized) {
		t.Fatalf("401 should not trigger backoff")
	}
}

func TestRandomDurationInRange(t *testing.T) {
	min := 3 * time.Minute
	max := 10 * time.Minute
	for i := 0; i < 20; i++ {
		got := randomDurationInRange(min, max)
		if got < min || got > max {
			t.Fatalf("duration out of range: %v", got)
		}
	}
}
