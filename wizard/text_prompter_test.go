package wizard

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// pipePrompter returns a TextPrompter reading from a pipe (never a TTY, so the
// Secret fallback path is exercised) and writing to a buffer.
func pipePrompter(t *testing.T, input string) (*TextPrompter, *bytes.Buffer) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = w.Close()
	t.Cleanup(func() { _ = r.Close() })
	var out bytes.Buffer
	return &TextPrompter{In: r, Out: &out}, &out
}

// TestRenderSecret_LiveReveal pins the decision that the tail is visible AS YOU
// TYPE, so a paste is confirmed the instant it lands.
func TestRenderSecret_LiveReveal(t *testing.T) {
	got := renderSecret([]rune("sk-abcdefghijkla75y"))
	if !strings.HasSuffix(got, "a75y") {
		t.Errorf("renderSecret = %q, want it to end in a75y", got)
	}
	for _, leak := range []string{"sk-", "abcdef"} {
		if strings.Contains(got, leak) {
			t.Errorf("renderSecret leaks %q: %q", leak, got)
		}
	}
}

// TestRenderSecret_BelowThreshold: a short or partial paste must not expose
// most of itself.
func TestRenderSecret_BelowThreshold(t *testing.T) {
	for n := 1; n < minRevealRunes; n++ {
		in := []rune(strings.Repeat("x", n))
		got := renderSecret(in)
		if strings.ContainsRune(got, 'x') {
			t.Errorf("renderSecret(%d runes) = %q must reveal nothing below %d", n, got, minRevealRunes)
		}
		if len([]rune(got)) != n {
			t.Errorf("renderSecret(%d runes) = %q, want %d glyphs so length feedback still works", n, got, n)
		}
	}
}

func TestRenderSecret_RuneSafe(t *testing.T) {
	got := renderSecret([]rune("prefix-日本語テスト"))
	if !utf8.ValidString(got) {
		t.Errorf("renderSecret produced invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "語テスト") {
		t.Errorf("renderSecret = %q, want the last four RUNES", got)
	}
}

// TestSecret_FallbackWhenNotATTY is the Git Bash / mintty guarantee: raw mode
// is unavailable there, and without a fallback those users cannot configure at
// all.
func TestSecret_FallbackWhenNotATTY(t *testing.T) {
	p, out := pipePrompter(t, "sk-my-secret-key\n")
	got, err := p.Secret("Enter API key")
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	if got != "sk-my-secret-key" {
		t.Errorf("Secret = %q", got)
	}
	if !strings.Contains(out.String(), "Enter API key") {
		t.Errorf("prompt not shown: %q", out.String())
	}
}

func TestSecret_EmptyReprompts(t *testing.T) {
	p, out := pipePrompter(t, "\n\nsk-eventually\n")
	got, err := p.Secret("Key")
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	if got != "sk-eventually" {
		t.Errorf("Secret = %q, want the first non-empty entry", got)
	}
	if !strings.Contains(out.String(), "empty value") {
		t.Errorf("expected an empty-value warning: %q", out.String())
	}
}

func TestSelect_DefaultOnEmpty(t *testing.T) {
	choices := []Choice{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	p, _ := pipePrompter(t, "\n")
	got, err := p.Select("Pick", choices, 1)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got != 1 {
		t.Errorf("Select = %d, want the default 1", got)
	}

	p2, _ := pipePrompter(t, "3\n")
	got, err = p2.Select("Pick", choices, 0)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got != 2 {
		t.Errorf("Select = %d, want 2 (1-based input 3)", got)
	}
}

func TestSelect_RepromptsOnBadInput(t *testing.T) {
	p, out := pipePrompter(t, "99\nx\n2\n")
	got, err := p.Select("Pick", []Choice{{Label: "a"}, {Label: "b"}}, 0)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got != 1 {
		t.Errorf("Select = %d, want 1", got)
	}
	if !strings.Contains(out.String(), "between 1 and 2") {
		t.Errorf("expected a range warning: %q", out.String())
	}
}

func TestMultiSelect_ParsesIndices(t *testing.T) {
	choices := []Choice{{Label: "a"}, {Label: "b"}, {Label: "c"}}

	p, _ := pipePrompter(t, "1,3\n")
	got, err := p.MultiSelect("Pick any", choices, nil)
	if err != nil {
		t.Fatalf("MultiSelect: %v", err)
	}
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("MultiSelect = %v, want [0 2]", got)
	}

	// Empty accepts the preselection.
	p2, _ := pipePrompter(t, "\n")
	got, err = p2.MultiSelect("Pick any", choices, []int{1})
	if err != nil {
		t.Fatalf("MultiSelect: %v", err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("MultiSelect = %v, want the preselection [1]", got)
	}

	// Unparseable re-prompts.
	p3, out := pipePrompter(t, "x\n2\n")
	got, err = p3.MultiSelect("Pick any", choices, nil)
	if err != nil {
		t.Fatalf("MultiSelect: %v", err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("MultiSelect = %v, want [1]", got)
	}
	if !strings.Contains(out.String(), "comma-separated") {
		t.Errorf("expected a format warning: %q", out.String())
	}
}

func TestConfirm(t *testing.T) {
	for _, tc := range []struct {
		in   string
		def  bool
		want bool
	}{
		{"\n", true, true}, {"\n", false, false},
		{"y\n", false, true}, {"yes\n", false, true},
		{"n\n", true, false}, {"anything\n", true, false},
	} {
		p, _ := pipePrompter(t, tc.in)
		got, err := p.Confirm("OK?", tc.def)
		if err != nil {
			t.Fatalf("Confirm: %v", err)
		}
		if got != tc.want {
			t.Errorf("Confirm(%q, def=%v) = %v, want %v", tc.in, tc.def, got, tc.want)
		}
	}
}

func TestInput_DefaultOnEmpty(t *testing.T) {
	p, _ := pipePrompter(t, "\n")
	got, err := p.Input("URL", "http://localhost:11434")
	if err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got != "http://localhost:11434" {
		t.Errorf("Input = %q, want the default", got)
	}
}

func TestTextPrompter_SatisfiesPrompter(t *testing.T) {
	var _ Prompter = (*TextPrompter)(nil)
	var _ Prompter = NewTextPrompter()
}
