package logging

import (
	"strings"
	"testing"
)

// TestRedact_SecretClasses verifies each hardened pattern redacts its secret
// class while leaving the surrounding text intact.
func TestRedact_SecretClasses(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		secret string // substring that must be gone
	}{
		{"aws access key id", "key=AKIAIOSFODNN7EXAMPLE end", "AKIAIOSFODNN7EXAMPLE"},
		{"bare jwt", "tok eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcDEF123 end", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcDEF123"},
		{"github token", "gh ghp_0123456789abcdefghijABCDEFGHIJ end", "ghp_0123456789abcdefghijABCDEFGHIJ"},
		{"github pat", "x github_pat_0123456789abcdefghij end", "github_pat_0123456789abcdefghij"},
		{"slack token", "s xoxb-2222-3333-abcdefghij end", "xoxb-2222-3333-abcdefghij"},
		{"stripe live", "p sk_live_0123456789abcdef0123 end", "sk_live_0123456789abcdef0123"},
		{"google api key", "g AIzaSyA0123456789abcdefghijklmnopqrstuv end", "AIzaSyA0123456789abcdefghijklmnopqrstuv"},
		{"pem header", "-----BEGIN RSA PRIVATE KEY----- rest", "BEGIN RSA PRIVATE KEY"},
		{"password kv", `cfg password=hunter2longvalue end`, "hunter2longvalue"},
		{"api_key json", `{"api_key":"abcd1234efgh"}`, "abcd1234efgh"},
		{"legacy token_", "connecting with token_abc123xyz end", "token_abc123xyz"},
		{"legacy short secret_", "had secret_xyz456 here", "secret_xyz456"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := RedactString(tc.in)
			if strings.Contains(out, tc.secret) {
				t.Errorf("secret %q not redacted: %q", tc.secret, out)
			}
			if !strings.Contains(out, "[REDACTED]") {
				t.Errorf("expected [REDACTED] marker, got %q", out)
			}
		})
	}
}

// TestRedact_Refinements covers the follow-up additions: DSN credentials, and
// confirms schemeless Authorization / X-Api-Key are caught by the key=value pass.
func TestRedact_Refinements(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		secret string
		keep   string // substring that must remain (readability)
	}{
		{"postgres dsn", "dsn postgres://admin:s3cr3tpw@db.host:5432/app end", "s3cr3tpw", "postgres://"},
		{"redis dsn", "redis://:hunter2pass@cache:6379 ok", "hunter2pass", "redis://"},
		{"schemeless authorization", "Authorization: ab39f0c2deadbeef0011 done", "ab39f0c2deadbeef0011", "Authorization"},
		{"x-api-key header", `X-Api-Key: abcd1234efghijk done`, "abcd1234efghijk", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := RedactString(tc.in)
			if strings.Contains(out, tc.secret) {
				t.Errorf("secret %q not redacted: %q", tc.secret, out)
			}
			if !strings.Contains(out, "[REDACTED]") {
				t.Errorf("expected [REDACTED], got %q", out)
			}
			if tc.keep != "" && !strings.Contains(out, tc.keep) {
				t.Errorf("expected %q preserved, got %q", tc.keep, out)
			}
		})
	}
}

// TestRedact_SizeCap confirms an oversized input is truncated with a marker
// (bounding regex work + stored size) and the caller's buffer is not mutated.
func TestRedact_SizeCap(t *testing.T) {
	in := make([]byte, maxRedactBytes+5000)
	for i := range in {
		in[i] = 'a'
	}
	orig := append([]byte(nil), in...)
	out := Redact(in)
	if len(out) >= len(in) {
		t.Errorf("expected truncation, got len %d (input %d)", len(out), len(in))
	}
	if !strings.Contains(string(out), "truncated 5000 bytes") {
		t.Errorf("expected truncation marker, got tail %q", string(out[len(out)-40:]))
	}
	if string(in) != string(orig) {
		t.Error("Redact must not mutate the caller's buffer")
	}
}

// TestRedact_NoOverRedaction confirms the word-anchored, min-length legacy prefix
// no longer clobbers short benign identifiers.
func TestRedact_NoOverRedaction(t *testing.T) {
	for _, benign := range []string{"secret_id", "token_v2", "next_secret_in_line", "key_map"} {
		if out := RedactString("value " + benign + " here"); !strings.Contains(out, benign) {
			t.Errorf("benign identifier %q was over-redacted: %q", benign, out)
		}
	}
}

// TestRedact_BearerLeak is the B2 regression: the JWT after "Bearer" used to be
// emitted in cleartext because leftmost-match consumed only "Bearer".
func TestRedact_BearerLeak(t *testing.T) {
	in := "Authorization: Bearer eyJhbGciOi.eyJzdWIi.sigABC123 done"
	out := RedactString(in)
	for _, leak := range []string{"eyJhbGciOi", "eyJzdWIi", "sigABC123"} {
		if strings.Contains(out, leak) {
			t.Errorf("JWT segment %q leaked: %q", leak, out)
		}
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("expected redaction, got %q", out)
	}
	// The "Authorization:" label is preserved for readability.
	if !strings.Contains(out, "Authorization:") {
		t.Errorf("expected Authorization label preserved, got %q", out)
	}
}

// TestRedact_PreservesBenign verifies clean text is returned byte-for-byte
// (same backing array) so logs stay readable and the io.Writer contract holds.
func TestRedact_PreservesBenign(t *testing.T) {
	in := []byte("connecting with the database and finishing up")
	out := Redact(in)
	if string(out) != string(in) {
		t.Errorf("benign text altered: %q", out)
	}
	if &out[0] != &in[0] {
		t.Error("benign input should be returned unmodified (same backing array)")
	}
}
