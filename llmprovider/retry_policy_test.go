package llmprovider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// quotaError is a terminal OpenCode free-usage limit: a 429 that only the
// classified sentinel, not the status, marks as unretryable.
func quotaError() error {
	return classifyFixture("opencode-zen/responses", http.StatusTooManyRequests,
		`{"type":"error","error":{"type":"FreeUsageLimitError","message":"limit"}}`, nil)
}

// TestRetry_TerminalMakesOneCall: a terminal APIError stops the retry loop at
// once (MADR 0012 §1.2).
func TestRetry_TerminalMakesOneCall(t *testing.T) {
	f := &fakeProvider{errs: []error{quotaError(), quotaError(), quotaError()}}
	_, err := GenerateWithRetry(context.Background(), f, "p", 2, time.Millisecond)
	if f.calls != 1 || !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("calls = %d, err = %v; want 1 call and the quota error", f.calls, err)
	}
}

// TestRetry_RetryAfterAboveCapReturns: a server delay beyond the 30 s cap is
// returned to the caller, with its RetryAfter, instead of being slept on.
func TestRetry_RetryAfterAboveCapReturns(t *testing.T) {
	limited := &RateLimitError{RetryAfter: 120 * time.Second, Status: http.StatusTooManyRequests, Provider: "fake"}
	f := &fakeProvider{errs: []error{limited, limited}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	_, err := GenerateWithRetry(ctx, f, "p", 2, time.Millisecond)
	var rl *RateLimitError
	if f.calls != 1 || !errors.As(err, &rl) || rl.RetryAfter != 120*time.Second || time.Since(start) > time.Second {
		t.Fatalf("calls = %d, err = %v after %s; want 1 call returning the 120s retry-after at once",
			f.calls, err, time.Since(start))
	}
}

func TestParseRetryAfter_MillisAndFractional(t *testing.T) {
	for _, header := range []http.Header{
		{"Retry-After-Ms": {"1500"}},
		{"Retry-After": {"1.5"}},
	} {
		var rl *RateLimitError
		err := classifyFixture("kilo", http.StatusTooManyRequests, `{}`, header)
		if !errors.As(err, &rl) || rl.RetryAfter != 1500*time.Millisecond {
			t.Errorf("header %v: err = %v, want retry-after 1.5s", header, err)
		}
	}
}

// TestRetry_408IsRetried: a request timeout is transient (0013 B4).
func TestRetry_408IsRetried(t *testing.T) {
	timeout := classifyFixture("huggingface", http.StatusRequestTimeout, `{"error":"timeout"}`, nil)
	f := &fakeProvider{errs: []error{timeout}}
	out, err := GenerateWithRetry(context.Background(), f, "p", 2, time.Millisecond)
	if err != nil || out != "ok" || f.calls != 2 {
		t.Fatalf("calls = %d, out = %q, err = %v; want a retry that succeeds", f.calls, out, err)
	}
}

// TestRetry_ShouldRetryFalseIsTerminal: x-should-retry: false makes an
// otherwise retryable error terminal.
func TestRetry_ShouldRetryFalseIsTerminal(t *testing.T) {
	unavailable := classifyFixture("openai", http.StatusServiceUnavailable, `{}`, http.Header{"X-Should-Retry": {"false"}})
	f := &fakeProvider{errs: []error{unavailable, unavailable}}
	_, err := GenerateWithRetry(context.Background(), f, "p", 2, time.Millisecond)
	if f.calls != 1 || !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("calls = %d, err = %v; want 1 call", f.calls, err)
	}
}

// TestGenerateItemsWithRetry_UsesSharedLoop: the items helper follows the same
// policy as GenerateWithRetry (0013 B6).
func TestGenerateItemsWithRetry_UsesSharedLoop(t *testing.T) {
	f := &fakeProvider{errs: []error{quotaError(), quotaError()}}
	_, err := GenerateItemsWithRetry(context.Background(), f, nil, 2, time.Millisecond)
	if f.calls != 1 || !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("calls = %d, err = %v; want 1 call", f.calls, err)
	}
}

// TestRetry_NegativeRetriesMakesOneCall: retries below zero means one attempt,
// never zero attempts and a nil-wrapped error (0013 B5).
func TestRetry_NegativeRetriesMakesOneCall(t *testing.T) {
	f := &fakeProvider{errs: []error{ErrProviderUnavailable}}
	_, err := GenerateWithRetry(context.Background(), f, "p", -1, time.Millisecond)
	if f.calls != 1 || err == nil || strings.Contains(err.Error(), "%!w") {
		t.Fatalf("calls = %d, err = %v; want one attempt and its error", f.calls, err)
	}
}
