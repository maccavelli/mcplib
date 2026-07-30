package logging

import (
	"fmt"
	"regexp"
	"strings"
)

// maxRedactBytes bounds the work (and stored size) for a single redaction call.
// A log line larger than this is almost always an accidental payload dump, so it
// is truncated with a marker before the regex passes run.
const maxRedactBytes = 256 * 1024

// This file is the single source of truth for secret redaction in mcplib.
// LogBuffer.Write, SanitizingWriter.Write, and the MCP log-notification path
// all funnel through Redact so the patterns can never drift apart.
//
// Patterns are split into three precompiled regexes by replacement style:
//   - reAuth / reKV preserve a human-readable label (capture group 1) and
//     redact only the credential, e.g. "Authorization: [REDACTED]".
//   - reToken redacts the whole match for self-identifying standalone secrets.
//
// Each is guarded by a cheap Match() so clean lines incur no allocation.
var (
	// reAuth matches "[Authorization:] (Bearer|Basic|Token) <credential>".
	// Group 1 is the optional "Authorization:" label to preserve. The greedy
	// credential class is what fixes the prior leftmost-match leak where the JWT
	// following "Bearer" was emitted in cleartext.
	reAuth = regexp.MustCompile(`(?i)((?:authorization\s*[:=]\s*)?)(?:bearer|basic|token)\s+[A-Za-z0-9._~+/=-]{8,}`)

	// reKV matches key=value / "key": "value" secret assignments. Group 1 is the
	// key name + separator (+ optional quote) to preserve; the value is redacted.
	reKV = regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|secret|api[_-]?key|access[_-]?key|client[_-]?secret|token|authorization)\b["']?\s*[:=]\s*["']?)[^"'\s,}]{4,}`)

	// reToken matches standalone, self-identifying secrets; the whole match is
	// redacted. Vendor prefixes stay case-sensitive (they are issued literals);
	// the legacy keyword prefixes are matched case-insensitively to preserve the
	// original behavior.
	reToken = regexp.MustCompile(strings.Join([]string{
		`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`,                      // JWT
		`\b(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA|ANVA)[0-9A-Z]{16}\b`,                 // AWS access key id
		`\bgh[posru]_[A-Za-z0-9]{20,}\b`,                                         // GitHub token
		`\bgithub_pat_[A-Za-z0-9_]{20,}\b`,                                       // GitHub fine-grained PAT
		`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`,                                       // Slack token
		`\b(?:sk|rk|pk)_(?:live|test)_[A-Za-z0-9]{16,}\b`,                        // Stripe key
		`\bAIza[0-9A-Za-z_-]{35}\b`,                                              // Google API key
		`-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP |ENCRYPTED )?PRIVATE KEY-----`, // PEM private key header
		`\b(?i:token_|sk_|key_|secret_)[A-Za-z0-9._/+=-]{6,}`,                    // legacy keyword prefixes (word-anchored, min length to cut false positives)
	}, "|"))
)

// Redact removes common secret formats from p, returning p unchanged (same
// backing array) when nothing matches. Safe for nil/empty input.
func Redact(p []byte) []byte {
	if len(p) > maxRedactBytes {
		// 3-index slice caps capacity so append allocates a fresh array and never
		// mutates the caller's buffer.
		marker := fmt.Sprintf("…[truncated %d bytes]\n", len(p)-maxRedactBytes)
		p = append(p[:maxRedactBytes:maxRedactBytes], marker...)
	}
	if reAuth.Match(p) {
		p = reAuth.ReplaceAll(p, []byte("${1}[REDACTED]"))
	}
	if reKV.Match(p) {
		p = reKV.ReplaceAll(p, []byte("${1}[REDACTED]"))
	}
	if reDSN.Match(p) {
		p = reDSN.ReplaceAll(p, []byte("${1}[REDACTED]@"))
	}
	if reToken.Match(p) {
		p = reToken.ReplaceAll(p, []byte("[REDACTED]"))
	}
	return p
}

// reDSN matches credentials embedded in connection strings / URIs
// (scheme://user:pass@host); the scheme is preserved, the user:pass redacted.
var reDSN = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^:@/\s]*:[^@/\s]+@`)

// RedactString is the string convenience wrapper around Redact.
func RedactString(s string) string {
	return string(Redact([]byte(s)))
}
