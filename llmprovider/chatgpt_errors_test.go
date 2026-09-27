package llmprovider

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func classifyBody(provider string, status int, body string) error {
	return classifyHTTPError(provider, &http.Response{StatusCode: status, Header: http.Header{},
		Body: io.NopCloser(strings.NewReader(body))})
}

// TestClassify_DetailEnvelope: the ChatGPT backend answers a bad request with
// {"detail": ...} (gate G-C, 2026-09-27); that text is the message.
func TestClassify_DetailEnvelope(t *testing.T) {
	const detail = "The 'gpt-4.1-mini' model is not supported when using Codex with a ChatGPT account."
	err := classifyBody(ProviderOpenAI, http.StatusBadRequest, `{"detail":"`+detail+`"}`)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != detail || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v (message %q), want ErrInvalidRequest carrying the detail", err, apiErr.Message)
	}
}

// TestClassify_UsageLimitResetsAt: Codex reads resets_at (Unix seconds) from
// a usage_limit_reached body (codex-api/src/api_bridge.rs:186-206); with no
// Retry-After header it becomes RetryAfter.
func TestClassify_UsageLimitResetsAt(t *testing.T) {
	reset := time.Now().Add(time.Hour).Unix()
	err := classifyBody(ProviderOpenAI, http.StatusTooManyRequests,
		`{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","plan_type":"plus","resets_at":`+
			strconv.FormatInt(reset, 10)+`}}`)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, ErrQuotaExhausted) || !apiErr.Terminal {
		t.Fatalf("err = %v, want a terminal ErrQuotaExhausted", err)
	}
	if apiErr.RetryAfter < 59*time.Minute || apiErr.RetryAfter > time.Hour {
		t.Errorf("RetryAfter = %v, want about an hour", apiErr.RetryAfter)
	}
}
