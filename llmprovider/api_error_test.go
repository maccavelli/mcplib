package llmprovider

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestClassifyHTTPError_Table pins MADR 0012 §1.1's classification, with the
// rows its 2026-09-27 amendment added: sentinel, Terminal, Type, a bounded
// Message, and the pre-0012 status sentinel still matching (§7).
func TestClassifyHTTPError_Table(t *testing.T) {
	opencode := func(errType, msg string) string {
		return `{"type":"error","error":{"type":"` + errType + `","message":"` + msg + `"}}`
	}
	for _, test := range []struct {
		name     string
		provider string
		status   int
		body     string
		want     error
		terminal bool
		errType  string
		message  string
	}{
		{"opencode free usage limit", "opencode-zen/responses", 429, opencode("FreeUsageLimitError", "free limit reached"), ErrQuotaExhausted, true, "FreeUsageLimitError", "free limit reached"},
		{"opencode go usage limit", "opencode-go/chat_completions", 429, opencode("GoUsageLimitError", "go limit"), ErrQuotaExhausted, true, "GoUsageLimitError", "go limit"},
		{"opencode black usage limit", "opencode-zen/messages", 429, opencode("BlackUsageLimitError", "limit"), ErrQuotaExhausted, true, "BlackUsageLimitError", "limit"},
		{"opencode credits", "opencode-zen/responses", 401, opencode("CreditsError", "no credits"), ErrQuotaExhausted, true, "CreditsError", "no credits"},
		{"opencode monthly limit", "opencode-zen/responses", 401, opencode("MonthlyLimitError", "monthly"), ErrQuotaExhausted, true, "MonthlyLimitError", "monthly"},
		{"opencode user limit", "opencode-zen/responses", 401, opencode("UserLimitError", "user"), ErrQuotaExhausted, true, "UserLimitError", "user"},
		{"opencode region", "opencode-go/messages", 403, opencode("RegionError", "region"), ErrNotPermitted, true, "RegionError", "region"},
		{"opencode data policy", "opencode-go/messages", 403, opencode("DataPolicyError", "policy"), ErrNotPermitted, true, "DataPolicyError", "policy"},
		{"opencode free tier", "opencode-zen/chat_completions", 403, opencode("FreeTierError", "OpenCode's free tier can only be used from within OpenCode"), ErrNotPermitted, true, "FreeTierError", "free tier"},
		{"opencode model", "opencode-zen/responses", 401, opencode("ModelError", "unknown model"), ErrInvalidRequest, true, "ModelError", "unknown model"},
		{"opencode upstream 402", "opencode-zen/google", 402, `{"type":"error","error":{"type":"error","message":"Upstream request failed: Insufficient account funds"}}`, ErrQuotaExhausted, true, "", "Insufficient account funds"},
		{"kilo promotion limit", "kilo", 429, `{"error":{"code":"PROMOTION_MODEL_LIMIT_REACHED","message":"promo"}}`, ErrQuotaExhausted, true, "PROMOTION_MODEL_LIMIT_REACHED", "promo"},
		{"kilo free usage text", "kilo", 429, `{"error":{"message":"FreeUsageLimitError: try later"}}`, ErrQuotaExhausted, true, "", "try later"},
		{"kilo 402", "kilo", 402, `{"error":{"code":"insufficient_balance","message":"top up"}}`, ErrQuotaExhausted, true, "insufficient_balance", "top up"},
		{"kilo 403", "kilo", 403, `{"error":{"message":"denied"}}`, ErrNotPermitted, true, "", "denied"},
		{"kilo paid model auth", "kilo", 401, `{"code":"PAID_MODEL_AUTH_REQUIRED"}`, ErrInvalidRequest, true, "PAID_MODEL_AUTH_REQUIRED", ""},
		{"codex usage limit", "openai", 429, `{"error":{"type":"usage_limit_reached","message":"reached"}}`, ErrQuotaExhausted, true, "usage_limit_reached", "reached"},
		{"openai insufficient quota", "openai", 429, `{"error":{"type":"insufficient_quota","code":"insufficient_quota","message":"quota"}}`, ErrQuotaExhausted, true, "insufficient_quota", "quota"},
		{"codex usage not included", "openai", 403, `{"error":{"type":"usage_not_included","message":"plan"}}`, ErrNotPermitted, true, "usage_not_included", "plan"},
		{"401 otherwise", "grok", 401, `{"code":"unauthenticated","error":"bad key"}`, ErrAuthFailure, true, "unauthenticated", "bad key"},
		{"403 otherwise", "claude", 403, `{"type":"error","error":{"type":"permission_error","message":"no"}}`, ErrAuthFailure, true, "permission_error", "no"},
		{"408", "huggingface", 408, `{"error":"timeout"}`, ErrProviderUnavailable, false, "", "timeout"},
		{"500", "gemini", 500, `{"error":{"code":500,"message":"internal","status":"INTERNAL"}}`, ErrProviderUnavailable, false, "INTERNAL", "internal"},
		{"525", "kilo", 525, "", ErrProviderUnavailable, true, "", ""},
		{"526", "kilo", 526, "", ErrProviderUnavailable, true, "", ""},
		{"400 otherwise", "ollama", 400, `{"error":"model not found"}`, ErrInvalidRequest, true, "", "model not found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := classifyFixture(test.provider, test.status, test.body, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if legacy := statusSentinel(test.status); !errors.Is(err, legacy) {
				t.Errorf("error = %v no longer matches the pre-0012 sentinel %v", err, legacy)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error %T is not an *APIError", err)
			}
			if apiErr.Terminal != test.terminal || apiErr.Type != test.errType || apiErr.Status != test.status {
				t.Errorf("Terminal/Type/Status = %t/%q/%d, want %t/%q/%d",
					apiErr.Terminal, apiErr.Type, apiErr.Status, test.terminal, test.errType, test.status)
			}
			if !strings.Contains(apiErr.Message, test.message) || !strings.Contains(err.Error(), test.provider) {
				t.Errorf("error = %q, want the provider and the message %q", err, test.message)
			}
		})
	}
}

// TestClassifyHTTPError_PlainRateLimit: a 429 with no quota classification
// stays a retryable *RateLimitError, now carrying the service's message.
func TestClassifyHTTPError_PlainRateLimit(t *testing.T) {
	err := classifyFixture("kilo", 429, `{"error":{"message":"slow down"}}`, http.Header{"Retry-After": {"7"}})
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != 7*time.Second || rl.Message != "slow down" {
		t.Fatalf("error = %#v, want a *RateLimitError with retry-after 7s and the message", err)
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatal("a plain 429 must stay a *RateLimitError, not an *APIError")
	}
}

func TestClassifyHTTPError_BoundsMessage(t *testing.T) {
	err := classifyFixture("kilo", 400, `{"error":{"message":"`+strings.Repeat("é", 600)+`"}}`, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || len(apiErr.Message) > apiErrorMessageLimit+len("…") {
		t.Fatalf("message is %d bytes, want at most %d", len(apiErr.Message), apiErrorMessageLimit+len("…"))
	}
}

func TestClassifyHTTPError_RedactsMessage(t *testing.T) {
	err := classifyFixture("openai", 400,
		`{"error":{"message":"bad token eyJhbGciOiJub25lIn0.eyJzdWIiOiJ4In0.sig"}}`, nil)
	if err == nil || strings.Contains(err.Error(), "eyJ") {
		t.Fatalf("error = %v, want the token redacted", err)
	}
}

// classifyFixture classifies a synthetic response and closes its body.
func classifyFixture(provider string, status int, body string, header http.Header) error {
	if header == nil {
		header = http.Header{}
	}
	resp := &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
	defer closeResponseBody(resp)
	return classifyHTTPError(provider, resp)
}
