package llmprovider

import (
	"slices"
	"strings"
	"testing"
)

// matcherFixture is the shared SearchModels corpus. Expected results below were
// derived from a reference implementation of MADR 0009 §3 before this code was
// written (0009-PLAN Appendix C).
var matcherFixture = []string{
	"gemini-3.5-flash", "gemini-3.5-flash-lite", "gemini-2.5-pro",
	"claude-sonnet-5", "claude-haiku-4-5",
	"meta-llama/Llama-3.1-8B-Instruct", "meta-llama/kilo-auto",
	"kilo-auto/balanced", "kilo-auto/free", "kilo-auto/fast", "kilo-auto-legacy",
	"gpt-4.1-mini", "chatgpt-4o-latest", "gpt-5.4",
	"llama3:latest", "fireworks/llama-ash",
}

func matchIDs(ms []ModelMatch) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func matchScores(ms []ModelMatch) []int {
	out := make([]int, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Score)
	}
	return out
}

func assertIDs(t *testing.T, query string, got []ModelMatch, want []string) {
	t.Helper()
	if ids := matchIDs(got); !slices.Equal(ids, want) {
		t.Errorf("SearchModels(%q) ids = %v, want %v", query, ids, want)
	}
}

func TestSearchModels_GlobCrossesSlash(t *testing.T) {
	got := SearchModels("", matcherFixture, "kilo-auto/*")
	assertIDs(t, "kilo-auto/*", got, []string{"kilo-auto/balanced", "kilo-auto/free", "kilo-auto/fast"})
}

func TestSearchModels_GlobMatchesAcrossOrg(t *testing.T) {
	got := SearchModels("", matcherFixture, "*llama*")
	if !slices.Contains(matchIDs(got), "meta-llama/Llama-3.1-8B-Instruct") {
		t.Errorf("*llama* = %v, want it to contain meta-llama/Llama-3.1-8B-Instruct", matchIDs(got))
	}
}

func TestSearchModels_GlobIsAnchored(t *testing.T) {
	got := SearchModels("", matcherFixture, "gpt-4*")
	assertIDs(t, "gpt-4*", got, []string{"gpt-4.1-mini"})
}

func TestSearchModels_GlobKeepsInputOrder(t *testing.T) {
	got := SearchModels("", matcherFixture, "*")
	assertIDs(t, "*", got, matcherFixture)
	for _, m := range got {
		if m.Score != scoreGlob {
			t.Errorf("%s score = %d, want %d", m.ID, m.Score, scoreGlob)
		}
	}
}

func TestSearchModels_SubstringOutranksSubsequence(t *testing.T) {
	got := SearchModels("", matcherFixture, "flash")
	assertIDs(t, "flash", got, []string{"gemini-3.5-flash", "gemini-3.5-flash-lite", "fireworks/llama-ash"})
	if scores := matchScores(got); !slices.Equal(scores, []int{4000, 4000, 1020}) {
		t.Errorf("scores = %v, want [4000 4000 1020]", scores)
	}
}

func TestSearchModels_TokenPrefix(t *testing.T) {
	for query, want := range map[string]string{
		"llama 8b":      "meta-llama/Llama-3.1-8B-Instruct",
		"llama3 latest": "llama3:latest",
	} {
		got := SearchModels("", matcherFixture, query)
		assertIDs(t, query, got, []string{want})
		if scores := matchScores(got); !slices.Equal(scores, []int{scoreTokenPrefix}) {
			t.Errorf("%q scores = %v, want [%d]", query, scores, scoreTokenPrefix)
		}
	}
}

func TestSearchModels_Subsequence(t *testing.T) {
	got := SearchModels("", matcherFixture, "sonet")
	assertIDs(t, "sonet", got, []string{"claude-sonnet-5"})
	if scores := matchScores(got); !slices.Equal(scores, []int{1035}) {
		t.Errorf("scores = %v, want [1035]", scores)
	}
}

func TestSearchModels_NoSubsequenceOverLabels(t *testing.T) {
	// hspd is an in-order subsequence of the haiku label ("…high speed…") but of
	// nothing in any id, so it must match nothing.
	label := strings.ToLower(ModelLabel("", "claude-haiku-4-5"))
	if _, ok := subsequenceBonus(label, "hspd"); !ok {
		t.Fatalf("precondition: hspd must be a subsequence of the label %q", label)
	}
	if got := SearchModels("", matcherFixture, "hspd"); len(got) != 0 {
		t.Errorf("hspd = %v, want no matches", matchIDs(got))
	}
}

func TestSearchModels_EmptyQuery(t *testing.T) {
	for _, q := range []string{"", "   "} {
		if got := SearchModels("", matcherFixture, q); got != nil {
			t.Errorf("SearchModels(%q) = %v, want nil", q, got)
		}
	}
}

func TestSearchModels_TieBreak(t *testing.T) {
	got := SearchModels("", matcherFixture, "auto")
	assertIDs(t, "auto", got, []string{
		"kilo-auto/fast", "kilo-auto/free", "kilo-auto-legacy", "kilo-auto/balanced", "meta-llama/kilo-auto",
	})
}

func TestSearchModels_Tiers(t *testing.T) {
	got := SearchModels("", []string{"xgpt-5.4", "gpt-5.4-mini", "gpt-5.4"}, "gpt-5.4")
	assertIDs(t, "gpt-5.4", got, []string{"gpt-5.4", "gpt-5.4-mini", "xgpt-5.4"})
	if scores := matchScores(got); !slices.Equal(scores, []int{scoreExact, scoreIDPrefix, scoreIDSubstring}) {
		t.Errorf("scores = %v, want exact, prefix, substring", scores)
	}
}

func TestSearchModels_Dedupe(t *testing.T) {
	got := SearchModels("", []string{"a-b", "A-B", "a-b"}, "a")
	assertIDs(t, "a", got, []string{"a-b"})
}

func TestSearchModels_LabelFromModelLabel(t *testing.T) {
	got := SearchModels("", []string{"claude-haiku-4-5"}, "haiku")
	if len(got) != 1 || got[0].Label != ModelLabel("", "claude-haiku-4-5") {
		t.Errorf("got %+v, want one match labelled %q", got, ModelLabel("", "claude-haiku-4-5"))
	}
}

func TestSearchModels_SeparatorOnlyQuery(t *testing.T) {
	got := SearchModels("", []string{"ab", "a-b"}, "-")
	assertIDs(t, "-", got, []string{"a-b"})
}
