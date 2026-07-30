package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type fakeProvider struct {
	calls int
	errs  []error // per-call result; nil (or past end) = success
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Generate(_ context.Context, _ string) (string, error) {
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return "", f.errs[i]
	}
	return "ok", nil
}

// TestGenerateWithRetry_NonRetryable is the #7 regression: auth/invalid errors
// must stop after a single attempt.
func TestGenerateWithRetry_NonRetryable(t *testing.T) {
	for _, e := range []error{ErrAuthFailure, ErrInvalidRequest} {
		f := &fakeProvider{errs: []error{fmt.Errorf("wrap: %w", e), fmt.Errorf("wrap: %w", e)}}
		_, err := GenerateWithRetry(context.Background(), f, "p", 5, time.Nanosecond)
		if !errors.Is(err, e) {
			t.Errorf("expected %v, got %v", e, err)
		}
		if f.calls != 1 {
			t.Errorf("non-retryable %v should be called once, got %d", e, f.calls)
		}
	}
}

func TestGenerateWithRetry_RetriesThenSucceeds(t *testing.T) {
	f := &fakeProvider{errs: []error{ErrRateLimited, ErrProviderUnavailable}}
	out, err := GenerateWithRetry(context.Background(), f, "p", 5, time.Nanosecond)
	if err != nil || out != "ok" {
		t.Fatalf("expected success, got %q err=%v", out, err)
	}
	if f.calls != 3 {
		t.Errorf("expected 3 calls (2 retryable fails + 1 ok), got %d", f.calls)
	}
}

// TestGenerateWithRetry_SubNanosNoPanic guards the rand.N(0) latent panic.
func TestGenerateWithRetry_SubNanosNoPanic(t *testing.T) {
	f := &fakeProvider{errs: []error{ErrRateLimited}}
	if _, err := GenerateWithRetry(context.Background(), f, "p", 2, time.Nanosecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
