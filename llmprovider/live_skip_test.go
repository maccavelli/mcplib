//go:build live_gateways

package llmprovider

import (
	"net/http"
	"testing"
)

// TestLiveTransient_SkipsOnlyTransientClasses pins 0013 D5: the live suite
// skips rate limits, outages and account-state refusals, and fails on a 400.
func TestLiveTransient_SkipsOnlyTransientClasses(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		skip bool
	}{
		{"429", classifyFixture("kilo", http.StatusTooManyRequests, `{}`, nil), true},
		{"500", classifyFixture("kilo", http.StatusInternalServerError, `{}`, nil), true},
		{"quota", classifyFixture("kilo", http.StatusPaymentRequired, `{}`, nil), true},
		{"free tier", classifyFixture("opencode-zen/responses", http.StatusForbidden,
			`{"type":"error","error":{"type":"FreeTierError","message":"no"}}`, nil), true},
		{"400", classifyFixture("kilo", http.StatusBadRequest, `{"error":{"message":"bad field"}}`, nil), false},
		{"401", classifyFixture("kilo", http.StatusUnauthorized, `{}`, nil), false},
		{"success", nil, false},
	} {
		if got := liveTransient(test.err); got != test.skip {
			t.Errorf("%s: liveTransient = %t, want %t (%v)", test.name, got, test.skip, test.err)
		}
	}
}
