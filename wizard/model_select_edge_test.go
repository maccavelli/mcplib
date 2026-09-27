package wizard

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/mcplib/llmprovider"
)

// TestConfigureLLM_FallbackPicksDeduped pins MADR 0013 C1: a repeated index
// in a fallback MultiSelect adds that model once.
func TestConfigureLLM_FallbackPicksDeduped(t *testing.T) {
	withEnv(t, nil)
	static := llmprovider.StaticModels(llmprovider.ProviderClaude)
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0}, secrets: []string{testKey},
		inputs: []string{"", ""}, multiSelects: [][]int{{0, 0, 1}},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{NeedFallbacks: true})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if want := []string{static[1], static[2]}; !slices.Equal(res.Fallbacks, want) {
		t.Errorf("Fallbacks = %q, want %q", res.Fallbacks, want)
	}
}

// TestMultiSelect_RepeatedIndexCountsOnce pins MADR 0013 C1 in TextPrompter.
func TestMultiSelect_RepeatedIndexCountsOnce(t *testing.T) {
	p, _ := pipePrompter(t, "1,1,2\n")
	got, err := p.MultiSelect("Pick any", []Choice{{Label: "a"}, {Label: "b"}, {Label: "c"}}, nil)
	if err != nil {
		t.Fatalf("MultiSelect: %v", err)
	}
	if !slices.Equal(got, []int{0, 1}) {
		t.Errorf("MultiSelect(1,1,2) = %v, want [0 1]", got)
	}
}

// TestConfigureLLM_OtherDefaultsOnlyToSameProvider pins MADR 0013 C4 (MADR
// 0009 §4.3): the Other and "No models found" prompts default to the saved
// model only when it belongs to the chosen provider.
func TestConfigureLLM_OtherDefaultsOnlyToSameProvider(t *testing.T) {
	withEnv(t, nil)
	saved := Result{Provider: llmprovider.ProviderClaude, Model: "claude-opus-5"}
	t.Run("other provider, Other", func(t *testing.T) {
		gemini := len(llmprovider.StaticModels(llmprovider.ProviderGemini))
		f := &fakePrompter{
			t: t, selects: []int{providerIdx(t, llmprovider.ProviderGemini), gemini},
			secrets: []string{testKey}, inputs: []string{""},
		}
		res, err := ConfigureLLM(context.Background(), f, Options{Existing: saved})
		if err == nil {
			t.Errorf("Enter at Other saved provider=%s model=%q, want an error (no default from another provider)",
				res.Provider, res.Model)
		}
	})
	t.Run("other provider, no models found", func(t *testing.T) {
		f := &fakePrompter{
			t: t, selects: []int{providerIdx(t, llmprovider.ProviderOllama)},
			inputs: []string{"http://127.0.0.1:1"}, confirms: []bool{false},
		}
		res, err := ConfigureLLM(context.Background(), f, Options{Existing: saved})
		if err == nil {
			t.Errorf("Enter at No models found saved provider=%s model=%q, want an error", res.Provider, res.Model)
		}
	})
	t.Run("same provider keeps its model", func(t *testing.T) {
		claude := len(llmprovider.StaticModels(llmprovider.ProviderClaude))
		f := &fakePrompter{
			t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), claude + 1},
			secrets: []string{testKey}, inputs: []string{""},
		}
		res, err := ConfigureLLM(context.Background(), f, Options{
			Existing: Result{Provider: llmprovider.ProviderClaude, Model: "my-model"},
		})
		if err != nil || res.Model != "my-model" {
			t.Errorf("Enter at Other: model=%q err=%v, want my-model", res.Model, err)
		}
	})
}

// TestConfigureLLM_FallbackExclusionIgnoresCase pins MADR 0013 C6: a primary
// typed in another case is not offered back as a fallback.
func TestConfigureLLM_FallbackExclusionIgnoresCase(t *testing.T) {
	withEnv(t, nil)
	static := llmprovider.StaticModels(llmprovider.ProviderClaude)
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), len(static)},
		secrets: []string{testKey}, inputs: []string{"", "Claude-Haiku-4-5", ""}, multiSelects: [][]int{{}},
	}
	if _, err := ConfigureLLM(context.Background(), f, Options{NeedFallbacks: true}); err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if n := len(f.seenMultiSelectItems[0]); n != len(static)-1 {
		t.Errorf("fallback menu has %d rows, want %d (claude-haiku-4-5 is the primary)", n, len(static)-1)
	}
}

// TestConfigureLLM_BlankModelIDRefused pins MADR 0013 C7 and covers
// model_select.go's empty-id branch: a blank or whitespace-only id is refused
// at Other and at "No models found".
func TestConfigureLLM_BlankModelIDRefused(t *testing.T) {
	withEnv(t, nil)
	claude := len(llmprovider.StaticModels(llmprovider.ProviderClaude))
	for _, id := range []string{"", "   "} {
		f := &fakePrompter{
			t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), claude},
			secrets: []string{testKey}, inputs: []string{"", id},
		}
		if res, err := ConfigureLLM(context.Background(), f, Options{}); err == nil {
			t.Errorf("Other with %q: model=%q, want an error", id, res.Model)
		}
	}
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOllama)},
		inputs: []string{"http://127.0.0.1:1", "   "}, confirms: []bool{false},
	}
	if res, err := ConfigureLLM(context.Background(), f, Options{}); err == nil {
		t.Errorf("No models found with blanks: model=%q, want an error", res.Model)
	}
}

// TestConfigureLLM_ListingErrorWarns covers configure.go's listing-error
// warning: Ollama unreachable with Discover set.
func TestConfigureLLM_ListingErrorWarns(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOllama)},
		inputs: []string{"http://127.0.0.1:1", "llama3"}, confirms: []bool{false},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{Discover: true})
	if err != nil || res.Model != "llama3" {
		t.Fatalf("ConfigureLLM: model=%q err=%v, want llama3", res.Model, err)
	}
	if n := countContaining(f.seenNotify, "could not list models for Ollama"); n != 1 {
		t.Errorf("listing warning seen %d times, want 1: %v", n, f.seenNotify)
	}
}

// TestConfigureLLM_OllamaEmptyListing covers configure.go's empty live
// listing: a reachable Ollama with nothing installed falls through to manual
// entry without a warning.
func TestConfigureLLM_OllamaEmptyListing(t *testing.T) {
	withEnv(t, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderOllama)}, inputs: []string{srv.URL, "llama3"}}
	res, err := ConfigureLLM(context.Background(), f, Options{Discover: true})
	if err != nil || res.Model != "llama3" {
		t.Fatalf("ConfigureLLM: model=%q err=%v, want llama3", res.Model, err)
	}
	if len(f.seenNotify) != 0 {
		t.Errorf("notices = %v, want none for an empty install", f.seenNotify)
	}
}

// TestConfigureLLM_DefaultRowIsExistingModel covers model_select.go's default
// row: a saved model in the recommended list is the menu default.
func TestConfigureLLM_DefaultRowIsExistingModel(t *testing.T) {
	withEnv(t, nil)
	static := llmprovider.StaticModels(llmprovider.ProviderClaude)
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 2}, secrets: []string{testKey}}
	res, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{Provider: llmprovider.ProviderClaude, Model: static[2]},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if f.seenSelectDefault[1] != 2 || res.Model != static[2] {
		t.Errorf("default = %d, model = %q; want 2 and %q", f.seenSelectDefault[1], res.Model, static[2])
	}
}

// TestConfigureLLM_BlankFallbackRoundWithNothingLeft covers model_select.go's
// blank round after search has taken every recommended model: it ends the
// loop without an empty MultiSelect.
func TestConfigureLLM_BlankFallbackRoundWithNothingLeft(t *testing.T) {
	withEnv(t, nil)
	srv := zenServer(t, http.StatusOK, zenListing(zenSearchIDs))
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOpencodeZen), 0}, secrets: []string{testKey},
		inputs:       []string{srv.URL, "", "flash", ""},
		multiSelects: [][]int{{0, 1, 2, 3, 4}}, confirms: []bool{true},
	}
	opts := zenOptions()
	opts.NeedFallbacks = true
	res, err := ConfigureLLM(context.Background(), f, opts)
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if len(res.Fallbacks) != 5 || len(f.seenMultiSelectItems) != 1 {
		t.Errorf("Fallbacks = %v after %d MultiSelects, want 5 after 1", res.Fallbacks, len(f.seenMultiSelectItems))
	}
}

// TestConfigureLLM_FallbackSearchNoMatches covers model_select.go's no-match
// fallback search: it warns and asks again.
func TestConfigureLLM_FallbackSearchNoMatches(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0}, secrets: []string{testKey},
		inputs: []string{"", "zzz-no-match", ""}, multiSelects: [][]int{{}},
	}
	if _, err := ConfigureLLM(context.Background(), f, Options{NeedFallbacks: true}); err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if n := countContaining(f.seenNotify, `match "zzz-no-match"`); n != 1 {
		t.Errorf("no-match warning seen %d times, want 1: %v", n, f.seenNotify)
	}
}

// TestConfigureLLM_ListingTokenFailureUsesStaticCatalog covers configure.go's
// listing-error branch for a provider that has a static catalog: a kept Grok
// session that cannot refresh fails the listing, so the wizard warns and
// offers the built-in catalog.
func TestConfigureLLM_ListingTokenFailureUsesStaticCatalog(t *testing.T) {
	withEnv(t, nil)
	static := llmprovider.StaticModels(llmprovider.ProviderGrok)
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), 1, 0}, confirms: []bool{true}}
	res, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{
			Provider:    llmprovider.ProviderGrok,
			Kind:        CredOAuth,
			AccessToken: "expired-access-abcd",
			TokenExpiry: time.Now().Add(-time.Hour),
		},
		TokenStore: newMemoryTokenStore(),
		Discover:   true,
	})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.Model != static[0] {
		t.Errorf("Model = %q, want the first built-in model %q", res.Model, static[0])
	}
	if n := countContaining(f.seenNotify, "no refresh token); using the built-in catalog"); n != 1 {
		t.Errorf("listing warning seen %d times, want 1: %v", n, f.seenNotify)
	}
}

// TestConfigureLLM_UnusableListingNotice covers the static-catalog notice with
// no cause: a listing that succeeds but offers no usable model degrades
// without a ModelCatalog.Err.
func TestConfigureLLM_UnusableListingNotice(t *testing.T) {
	withEnv(t, nil)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"not-a-claude-model"}],"has_more":false}`)),
			Request:    r,
		}, nil
	})}
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0}, secrets: []string{testKey}}
	if _, err := ConfigureLLM(context.Background(), f, Options{Discover: true, HTTPClient: client}); err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	want := "live model listing for Claude (Anthropic) is unavailable; search covers the built-in catalog only"
	if n := countContaining(f.seenNotify, want); n != 1 {
		t.Errorf("notice %q seen %d times, want 1: %v", want, n, f.seenNotify)
	}
}
