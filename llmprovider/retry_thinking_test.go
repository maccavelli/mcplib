package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeThinker records which path was called. Generate always fails, so a
// retry helper that calls the plain path cannot pass.
type fakeThinker struct {
	plain, thinking int
	errs            []error // per thinking call; nil (or past end) = success
}

func (f *fakeThinker) Name() string { return "fake-thinker" }

func (f *fakeThinker) Generate(_ context.Context, _ string) (string, error) {
	f.plain++
	return "", errors.New("plain path called")
}

func (f *fakeThinker) GenerateThinking(_ context.Context, _ string) (string, error) {
	i := f.thinking
	f.thinking++
	if i < len(f.errs) && f.errs[i] != nil {
		return "", f.errs[i]
	}
	return "thought", nil
}

func TestGenerateThinkingWithRetry_RetriesThenSucceeds(t *testing.T) {
	f := &fakeThinker{errs: []error{ErrRateLimited, ErrProviderUnavailable}}
	out, err := GenerateThinkingWithRetry(context.Background(), f, "p", 5, time.Nanosecond)
	if err != nil || out != "thought" {
		t.Fatalf("got %q, %v; want \"thought\", nil", out, err)
	}
	if f.thinking != 3 || f.plain != 0 {
		t.Errorf("calls: thinking=%d plain=%d, want 3 and 0", f.thinking, f.plain)
	}
}

// TestGenerateThinkingWithRetry_NonRetryable mirrors GenerateWithRetry's #7
// regression: auth and invalid-request errors stop after one attempt.
func TestGenerateThinkingWithRetry_NonRetryable(t *testing.T) {
	for _, e := range []error{ErrAuthFailure, ErrInvalidRequest} {
		f := &fakeThinker{errs: []error{fmt.Errorf("wrap: %w", e), fmt.Errorf("wrap: %w", e)}}
		_, err := GenerateThinkingWithRetry(context.Background(), f, "p", 5, time.Nanosecond)
		if !errors.Is(err, e) {
			t.Errorf("expected %v, got %v", e, err)
		}
		if f.thinking != 1 {
			t.Errorf("non-retryable %v: thinking calls = %d, want 1", e, f.thinking)
		}
	}
}

func TestGenerateThinkingWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeThinker{errs: []error{ErrRateLimited}}
	_, err := GenerateThinkingWithRetry(ctx, f, "p", 3, 50*time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if f.thinking != 1 {
		t.Errorf("thinking calls = %d, want 1", f.thinking)
	}
}

func TestGenerateThinkingWithRetry_Exhausted(t *testing.T) {
	f := &fakeThinker{errs: []error{ErrRateLimited, ErrRateLimited, ErrRateLimited, ErrRateLimited}}
	_, err := GenerateThinkingWithRetry(context.Background(), f, "p", 3, time.Nanosecond)
	if err == nil || !strings.HasPrefix(err.Error(), "failed after 4 attempts:") {
		t.Fatalf("err = %v, want prefix %q", err, "failed after 4 attempts:")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("err = %v, want it to wrap ErrRateLimited", err)
	}
}
