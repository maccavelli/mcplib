package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

func (f *fakeProvider) GenerateItems(_ context.Context, _ ...Item) (*Response, error) {
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	return &Response{Output: []Item{MessageItem{Role: "assistant", Text: "ok"}}}, nil
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

func TestGenerateItemsWithRetry_RetriesThenSucceeds(t *testing.T) {
	f := &fakeProvider{errs: []error{ErrRateLimited, ErrProviderUnavailable}}
	resp, err := GenerateItemsWithRetry(context.Background(), f, []Item{MessageItem{Role: "user", Text: "hi"}}, 5, time.Nanosecond)
	if err != nil || resp.OutputText() != "ok" {
		t.Fatalf("expected success, got %v err=%v", resp, err)
	}
	if f.calls != 3 {
		t.Errorf("expected 3 calls, got %d", f.calls)
	}
}

func TestGenerateItemsWithRetry_NonRetryable(t *testing.T) {
	for _, e := range []error{ErrAuthFailure, ErrInvalidRequest} {
		f := &fakeProvider{errs: []error{fmt.Errorf("wrap: %w", e), fmt.Errorf("wrap: %w", e)}}
		_, err := GenerateItemsWithRetry(context.Background(), f, []Item{MessageItem{Role: "user", Text: "hi"}}, 5, time.Nanosecond)
		if !errors.Is(err, e) {
			t.Errorf("expected %v, got %v", e, err)
		}
		if f.calls != 1 {
			t.Errorf("non-retryable %v should be called once, got %d", e, f.calls)
		}
	}
}

func TestNewProvider(t *testing.T) {
	providers := []string{
		ProviderGemini, ProviderClaude, ProviderOpenAI, ProviderGrok,
		ProviderOpencodeZen, ProviderOpencodeGo, ProviderHuggingFace, ProviderKilo,
	}
	for _, p := range providers {
		prov, err := NewProvider(p, "test-key", "model-x")
		if err != nil {
			t.Errorf("NewProvider(%q) error: %v", p, err)
		}
		if prov.Name() != p {
			t.Errorf("NewProvider(%q).Name() = %q", p, prov.Name())
		}
	}

	_, err := NewProvider("unsupported-p", "k", "m")
	if err == nil {
		t.Error("expected error for unsupported provider")
	}
}

func TestRateLimitError_ErrorString(t *testing.T) {
	rl := &RateLimitError{RetryAfter: 5 * time.Second, Status: 429}
	s := rl.Error()
	if !errors.Is(rl, ErrRateLimited) {
		t.Error("must unwrap to ErrRateLimited")
	}
	if s == "" || fmt.Sprintf("%v", rl) == "" {
		t.Error("error string must not be empty")
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	d := parseRetryAfter(future)
	if d <= 0 || d > 35*time.Second {
		t.Errorf("parseRetryAfter(%q) = %v, expected ~30s", future, d)
	}
}

func TestGenerateWithRetry_ContextCancelled(t *testing.T) {
	f := &fakeProvider{errs: []error{ErrRateLimited}}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after short delay
	go func() {
		time.Sleep(2 * time.Millisecond)
		cancel()
	}()
	_, err := GenerateWithRetry(ctx, f, "prompt", 3, 50*time.Millisecond)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestGenerateItemsWithRetry_ContextCancelled(t *testing.T) {
	f := &fakeProvider{errs: []error{ErrRateLimited}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(2 * time.Millisecond)
		cancel()
	}()
	_, err := GenerateItemsWithRetry(ctx, f, []Item{MessageItem{Role: "user", Text: "hi"}}, 3, 50*time.Millisecond)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestProviderEnvVars_Opencode pins the single evidenced credential name: both
// gateways share OPENCODE_API_KEY (models.dev declares it for each; neither
// docs page names any other variable).
func TestProviderEnvVars_Opencode(t *testing.T) {
	for _, gw := range []string{ProviderOpencodeZen, ProviderOpencodeGo} {
		got, ok := ProviderEnvVars[gw]
		if !ok {
			t.Errorf("ProviderEnvVars missing %q", gw)
			continue
		}
		if got != "OPENCODE_API_KEY" {
			t.Errorf("ProviderEnvVars[%q] = %q, want OPENCODE_API_KEY", gw, got)
		}
	}
}

// TestNewProvider_OpencodeRoutesResolved verifies NewProvider resolves the wire
// format at construction, so a misroute is visible before any request is made.
func TestNewProvider_OpencodeRoutesResolved(t *testing.T) {
	prov, err := NewProvider(ProviderOpencodeZen, "k", "claude-sonnet-5")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	op, ok := prov.(*OpencodeProvider)
	if !ok {
		t.Fatalf("NewProvider returned %T, want *OpencodeProvider", prov)
	}
	if op.Route() != OpencodeRouteMessages {
		t.Errorf("Route() = %q, want %q", op.Route(), OpencodeRouteMessages)
	}
}
