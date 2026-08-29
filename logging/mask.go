package logging

import "strings"

// minMaskedRunes is the length below which MaskSecret reveals nothing. Four
// revealed runes out of six is most of a short secret; out of forty it is a
// fingerprint. Eight is the smallest length at which the tail is a minority.
const minMaskedRunes = 8

// maskWidth is the fixed number of mask glyphs. Fixed rather than proportional
// so the rendering does not leak the credential's length.
const maskWidth = 8

// revealedRunes is how many trailing runes MaskSecret shows. Four is the
// convention used by Stripe, AWS and GitHub for stored-key display.
const revealedRunes = 4

// MaskSecret renders a credential for human identification: a fixed-width mask
// plus the last four runes, e.g. "••••••••a75y".
//
// This is the opposite intent to Redact, which hides a secret completely so a
// log line is safe to keep. MaskSecret reveals a suffix ON PURPOSE, so a person
// can tell which key they are looking at. Never use it on a value that will be
// written to a log; use Redact there.
//
// Inputs shorter than minMaskedRunes reveal nothing: on a short or partial
// value, four runes is most of the secret. Empty or whitespace-only input
// returns "—".
//
// Rune-safe: counts and slices runes, never bytes, so a multi-byte value cannot
// be split mid-rune into invalid UTF-8.
func MaskSecret(s string) string {
	r := []rune(strings.TrimSpace(s))
	switch {
	case len(r) == 0:
		return "—"
	case len(r) < minMaskedRunes:
		return strings.Repeat("•", maskWidth)
	default:
		return strings.Repeat("•", maskWidth) + string(r[len(r)-revealedRunes:])
	}
}
