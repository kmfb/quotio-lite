package probe

import (
	"net/http"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "ok", status: http.StatusOK, body: "{}", want: "ok"},
		{name: "usage", status: http.StatusTooManyRequests, body: "usage_limit_reached", want: "usage_limited"},
		{name: "plan", status: http.StatusBadRequest, body: "plan_type free", want: "plan_mismatch"},
		{name: "expired", status: http.StatusUnauthorized, body: "token expired", want: "auth_expired"},
		{name: "disabled", status: http.StatusForbidden, body: "account disabled", want: "disabled"},
		{name: "network", status: http.StatusBadGateway, body: "", want: "network_error"},
		{name: "unknown", status: http.StatusBadRequest, body: "something else", want: "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.status, tc.body)
			if got != tc.want {
				t.Fatalf("classify(%d, %q) = %s; want %s", tc.status, tc.body, got, tc.want)
			}
		})
	}
}
