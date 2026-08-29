package logging

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMaskSecret_RevealsLastFour(t *testing.T) {
	key := "sk-abcdefghijklmnopqrstuvwxyz0123456789a75y"
	got := MaskSecret(key)
	if !strings.HasSuffix(got, "a75y") {
		t.Errorf("MaskSecret(%q) = %q, want it to end in a75y", key, got)
	}
	// None of the preceding material may appear.
	for _, chunk := range []string{"sk-", "abcdef", "0123456789"} {
		if strings.Contains(got, chunk) {
			t.Errorf("mask leaks %q: %q", chunk, got)
		}
	}
}

func TestMaskSecret_ShortInputRevealsNothing(t *testing.T) {
	for _, in := range []string{"a", "ab", "abc", "abcd", "abcde", "abcdef", "abcdefg"} {
		got := MaskSecret(in)
		for _, c := range in {
			if strings.ContainsRune(got, c) {
				t.Errorf("MaskSecret(%q) = %q leaks %q; below %d runes nothing may be revealed",
					in, got, string(c), minMaskedRunes)
			}
		}
	}
}

func TestMaskSecret_Empty(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n"} {
		if got := MaskSecret(in); got != "—" {
			t.Errorf("MaskSecret(%q) = %q, want —", in, got)
		}
	}
}

// TestMaskSecret_RuneSafe is the regression for the byte-sliced maskKey copies
// this function replaces: key[len(key)-4:] splits a multi-byte rune and emits
// invalid UTF-8.
func TestMaskSecret_RuneSafe(t *testing.T) {
	for _, in := range []string{"key-日本語テスト", "prefix-émoji-🔑🔒🗝️🔐", "aaaaaaaa日本語テスト"} {
		got := MaskSecret(in)
		if !utf8.ValidString(got) {
			t.Errorf("MaskSecret(%q) produced invalid UTF-8: %q", in, got)
		}
		r := []rune(in)
		wantTail := string(r[len(r)-revealedRunes:])
		if !strings.HasSuffix(got, wantTail) {
			t.Errorf("MaskSecret(%q) = %q, want suffix %q (last %d RUNES, not bytes)",
				in, got, wantTail, revealedRunes)
		}
	}
}

// TestMaskSecret_FixedWidth: the mask must not encode the secret's length.
func TestMaskSecret_FixedWidth(t *testing.T) {
	short := MaskSecret(strings.Repeat("x", 20))
	long := MaskSecret(strings.Repeat("x", 60))
	if len([]rune(short)) != len([]rune(long)) {
		t.Errorf("mask width leaks length: %d vs %d runes", len([]rune(short)), len([]rune(long)))
	}
}

// TestMaskSecret_IsNotRedact documents the distinction between the two: Redact
// hides a secret for logs, MaskSecret reveals a suffix for identification.
func TestMaskSecret_IsNotRedact(t *testing.T) {
	const key = "sk_live_abcdefghijklmnop1234"
	masked := MaskSecret(key)
	redacted := RedactString(key)
	if !strings.HasSuffix(masked, "1234") {
		t.Errorf("MaskSecret must reveal the tail: %q", masked)
	}
	if strings.Contains(redacted, "1234") {
		t.Errorf("RedactString must not reveal the tail: %q", redacted)
	}
	if masked == redacted {
		t.Error("MaskSecret and RedactString must differ: they have opposite intent")
	}
}
