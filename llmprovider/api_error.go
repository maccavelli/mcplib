package llmprovider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/maccavelli/mcplib/logging"
)

// sentinelError is a sentinel that also matches a broader one, so a new class
// can be added without breaking an existing errors.Is check.
type sentinelError struct {
	msg    string
	parent error
}

func (e *sentinelError) Error() string { return e.msg }
func (e *sentinelError) Unwrap() error { return e.parent }

var (
	// ErrQuotaExhausted marks exhausted quota, credit or balance. Retrying
	// cannot succeed before the quota resets, so the APIError carrying it is
	// terminal. It also matches ErrRateLimited (MADR 0012 §1.1).
	ErrQuotaExhausted error = &sentinelError{msg: "llm: quota exhausted", parent: ErrRateLimited}
	// ErrNotPermitted marks a request the account may not make: a region,
	// data-policy, entitlement or free-tier restriction (MADR 0012 §1.1).
	ErrNotPermitted = errors.New("llm: not permitted")
)

const (
	// apiErrorBodyLimit bounds how much of an error body is read.
	apiErrorBodyLimit = 64 << 10
	// apiErrorMessageLimit bounds APIError.Message.
	apiErrorMessageLimit = 512
)

// APIError is a non-2xx response with the service's own error classification
// (MADR 0012 §1.1). It unwraps to the classified sentinel and, when that
// differs, to the sentinel the status alone mapped to before, so every
// existing errors.Is check still matches (§7).
type APIError struct {
	Provider   string
	Status     int
	Type       string        // the service's error type or code, e.g. "FreeUsageLimitError"
	Message    string        // the service's message, redacted and bounded to 512 bytes
	RetryAfter time.Duration // from Retry-After, when present
	Terminal   bool          // retrying cannot succeed
	sentinel   error
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%v: %s HTTP %d", e.sentinel, e.Provider, e.Status)
	if e.Type != "" {
		fmt.Fprintf(&b, " %s", e.Type)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	return b.String()
}

// Unwrap returns the classified sentinel and the pre-0012 status sentinel.
func (e *APIError) Unwrap() []error {
	if legacy := statusSentinel(e.Status); !errors.Is(e.sentinel, legacy) {
		return []error{e.sentinel, legacy}
	}
	return []error{e.sentinel}
}

// statusSentinel is the status-only mapping every provider used before MADR
// 0012.
func statusSentinel(status int) error {
	switch {
	case status == http.StatusTooManyRequests:
		return ErrRateLimited
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrAuthFailure
	case status >= http.StatusInternalServerError:
		return ErrProviderUnavailable
	default:
		return ErrInvalidRequest
	}
}

// classifyHTTPError maps a non-200 response to a typed error; it returns nil
// for 200. provider names the caller for the message: a multi-route gateway
// passes "gateway/route", so a misroute is diagnosable from the error alone.
// A plain 429 stays a *RateLimitError; every other status is an *APIError.
func classifyHTTPError(provider string, resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var body []byte
	if resp.Body != nil {
		var readErr error
		if body, readErr = io.ReadAll(io.LimitReader(resp.Body, apiErrorBodyLimit)); readErr != nil {
			slog.Debug("llmprovider: read error body", "provider", provider, "error", readErr)
		}
	}
	envelope := parseAPIErrorBody(body)
	e := &APIError{
		Provider:   provider,
		Status:     resp.StatusCode,
		Type:       envelope.errType(),
		Message:    boundMessage(logging.RedactString(envelope.message())),
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
	}
	e.Terminal, e.sentinel = classifyAPIError(serviceOf(provider), resp.StatusCode, envelope, body)
	if errors.Is(e.sentinel, ErrRateLimited) && !e.Terminal {
		return &RateLimitError{RetryAfter: e.RetryAfter, Status: e.Status, Provider: provider, Message: e.Message}
	}
	return e
}

// serviceOf reduces a provider label ("opencode-go/messages", "kilo") to the
// service whose error vocabulary applies.
func serviceOf(provider string) string {
	name, _, _ := strings.Cut(provider, "/")
	if strings.HasPrefix(name, serviceOpencode) {
		return serviceOpencode
	}
	return name
}

// serviceOpencode names both OpenCode gateways' shared error vocabulary.
const serviceOpencode = "opencode"

var (
	opencodeQuotaTypes     = []string{"FreeUsageLimitError", "GoUsageLimitError", "BlackUsageLimitError", "CreditsError", "MonthlyLimitError", "UserLimitError"}
	opencodeForbiddenTypes = []string{"RegionError", "DataPolicyError", "FreeTierError"}
	openAIQuotaTypes       = []string{"usage_limit_reached", "insufficient_quota"}
)

// classifyAPIError applies MADR 0012 §1.1's table (with its 2026-09-27 rows)
// and falls back to the status-only mapping.
func classifyAPIError(service string, status int, env apiErrorEnvelope, body []byte) (terminal bool, sentinel error) {
	has := func(types ...string) bool {
		return slices.ContainsFunc(types, env.hasType)
	}
	switch {
	case service == serviceOpencode && has(opencodeQuotaTypes...),
		service == ProviderKilo && (has("PROMOTION_MODEL_LIMIT_REACHED") || bytes.Contains(body, []byte("FreeUsageLimitError"))),
		service == ProviderOpenAI && has(openAIQuotaTypes...),
		status == http.StatusPaymentRequired:
		return true, ErrQuotaExhausted
	case service == serviceOpencode && has(opencodeForbiddenTypes...),
		service == ProviderOpenAI && has("usage_not_included"),
		service == ProviderKilo && status == http.StatusForbidden:
		return true, ErrNotPermitted
	case service == serviceOpencode && has("ModelError"),
		service == ProviderKilo && has("PAID_MODEL_AUTH_REQUIRED"):
		return true, ErrInvalidRequest
	case status == http.StatusTooManyRequests:
		return false, ErrRateLimited
	case status == http.StatusRequestTimeout:
		return false, ErrProviderUnavailable
	case status == 525 || status == 526:
		return true, ErrProviderUnavailable
	case status >= http.StatusInternalServerError:
		return false, ErrProviderUnavailable
	default:
		return true, statusSentinel(status)
	}
}

// apiErrorEnvelope is the union of the error bodies MADR 0012 §1.1 lists:
// OpenCode and Claude {type:"error",error:{type,message}}, Kilo
// {error:{code,message}} or {code}, OpenAI/Codex {error:{type,code,message}},
// xAI nested or flat {code,error}, Gemini {error:{code,message,status}}.
type apiErrorEnvelope struct {
	types []string // candidate classifications, most specific first
	msg   string
}

func (e apiErrorEnvelope) errType() string {
	if len(e.types) == 0 {
		return ""
	}
	return e.types[0]
}

func (e apiErrorEnvelope) message() string { return e.msg }

func (e apiErrorEnvelope) hasType(t string) bool { return slices.Contains(e.types, t) }

func parseAPIErrorBody(body []byte) apiErrorEnvelope {
	var top struct {
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &top) != nil {
		return apiErrorEnvelope{msg: strings.TrimSpace(string(body))}
	}
	var env apiErrorEnvelope
	add := func(values ...string) {
		for _, v := range values {
			if v != "" && v != "error" && !env.hasType(v) {
				env.types = append(env.types, v)
			}
		}
	}
	var inner struct {
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
		Status  string          `json:"status"`
		Message string          `json:"message"`
	}
	var text string
	switch {
	case json.Unmarshal(top.Error, &inner) == nil:
		add(jsonString(inner.Code), inner.Type, inner.Status)
		env.msg = inner.Message
	case json.Unmarshal(top.Error, &text) == nil:
		env.msg = text
	}
	add(jsonString(top.Code), top.Type)
	if env.msg == "" {
		env.msg = top.Message
	}
	return env
}

// jsonString returns a JSON string's value; numbers and null give "".
func jsonString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// boundMessage trims s to apiErrorMessageLimit bytes on a rune boundary.
func boundMessage(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= apiErrorMessageLimit {
		return s
	}
	cut := apiErrorMessageLimit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
