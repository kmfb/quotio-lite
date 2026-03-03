package managementusage

import "testing"

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
