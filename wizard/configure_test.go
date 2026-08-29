package wizard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/mcplib/llmprovider"
)

// claudeIdx returns the index of a provider in the canonical descriptor order,
// so tests script menu positions without hard-coding them.
func providerIdx(t *testing.T, id string) int {
	t.Helper()
	for i, d := range llmprovider.Descriptors() {
		if d.ID == id {
			return i
		}
	}
	t.Fatalf("provider %q not in Descriptors()", id)
	return -1
}

func withEnv(t *testing.T, vals map[string]string) {
	t.Helper()
	orig := getenv
	getenv = func(k string) string { return vals[k] }
	t.Cleanup(func() { getenv = orig })
}

const testKey = "sk-super-secret-key-1234"

func TestConfigureLLM_EnvKeyPrecedence(t *testing.T) {
	withEnv(t, map[string]string{"CLAUDE_API_KEY": testKey})
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderClaude), 0},
		confirms: []bool{true}, // yes, use the env key
	}
	res, err := ConfigureLLM(context.Background(), f, Options{AllowEnv: true})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.APIKey != testKey {
		t.Errorf("APIKey = %q, want the env value", res.APIKey)
	}
	if len(f.seenSecret) != 0 {
		t.Errorf("Secret must not be prompted when the env key is accepted: %v", f.seenSecret)
	}
}

func TestConfigureLLM_KeepExisting(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderClaude), 0},
		confirms: []bool{true}, // keep existing
	}
	res, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{Provider: llmprovider.ProviderClaude, APIKey: "existing-key-abcd"},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.APIKey != "existing-key-abcd" {
		t.Errorf("APIKey = %q, want the existing key", res.APIKey)
	}
	if len(f.seenSecret) != 0 {
		t.Error("Secret must not be prompted when the existing key is kept")
	}
}

func TestConfigureLLM_PromptsWhenNothingAvailable(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t:       t,
		selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0},
		secrets: []string{testKey},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{AllowEnv: true})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.APIKey != testKey {
		t.Errorf("APIKey = %q", res.APIKey)
	}
	if len(f.seenSecret) != 1 {
		t.Errorf("Secret prompted %d times, want 1", len(f.seenSecret))
	}
}

// TestConfigureLLM_LocalProviderSkipsKey: Ollama needs no credential, which is
// why ProviderDescriptor has RequiresAPIKey.
func TestConfigureLLM_LocalProviderSkipsKey(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t:       t,
		selects: []int{providerIdx(t, llmprovider.ProviderOllama)},
		// Two inputs: the endpoint, then the manual model id (Ollama has no
		// static catalog, so the flow falls through to manual entry).
		inputs: []string{"http://localhost:11434", "llama3.2:latest"},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{AllowEnv: true})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.APIKey != "" {
		t.Errorf("APIKey = %q, want empty for a local provider", res.APIKey)
	}
	if len(f.seenSecret) != 0 {
		t.Error("Secret must never be prompted for a provider that needs no key")
	}
	// Ollama has no static catalog, so the flow must fall through to a manual
	// model entry rather than dead-ending.
	if res.Model != "llama3.2:latest" {
		t.Errorf("Model = %q, want the manually entered id", res.Model)
	}
}

// TestConfigureLLM_NoModelsAndNoneEnteredErrors: a Result with an empty Model
// cannot generate anything, so the flow must fail rather than return it.
func TestConfigureLLM_NoModelsAndNoneEnteredErrors(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t:       t,
		selects: []int{providerIdx(t, llmprovider.ProviderOllama)},
		inputs:  []string{"http://localhost:11434", ""},
	}
	if _, err := ConfigureLLM(context.Background(), f, Options{}); err == nil {
		t.Error("expected an error when no model is available and none is entered")
	}
}

func TestConfigureLLM_EmptyDiscoveryFallsBackToStatic(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t:       t,
		selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0},
		secrets: []string{testKey},
	}
	// Discover against an unreachable base URL: the listing fails, so the
	// static catalog must be offered instead of the wizard dead-ending.
	res, err := ConfigureLLM(context.Background(), f, Options{
		Discover:      true,
		DiscoverLimit: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	static := llmprovider.StaticModels(llmprovider.ProviderClaude)
	if len(static) == 0 || res.Model != static[0] {
		t.Errorf("Model = %q, want the first static model %v", res.Model, static)
	}
}

func TestConfigureLLM_Fallbacks(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t:            t,
		selects:      []int{providerIdx(t, llmprovider.ProviderClaude), 0},
		secrets:      []string{testKey},
		multiSelects: [][]int{{0, 1}},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{NeedFallbacks: true})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if len(res.Fallbacks) != 2 {
		t.Fatalf("Fallbacks = %v, want 2", res.Fallbacks)
	}
	for _, fb := range res.Fallbacks {
		if fb == res.Model {
			t.Errorf("fallbacks must exclude the primary model %q: %v", res.Model, res.Fallbacks)
		}
	}
}

// TestConfigureLLM_MaskedKeyNeverPrintsSecret is the guarantee that a
// credential cannot leak through a prompt. Every string the user could have
// seen is checked against the raw key.
func TestConfigureLLM_MaskedKeyNeverPrintsSecret(t *testing.T) {
	withEnv(t, map[string]string{"CLAUDE_API_KEY": testKey})
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderClaude), 0},
		confirms: []bool{true},
	}
	if _, err := ConfigureLLM(context.Background(), f, Options{AllowEnv: true}); err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	var sawMask bool
	for _, s := range f.allText {
		if strings.Contains(s, testKey) {
			t.Errorf("a raw credential reached the user: %q", s)
		}
		if strings.Contains(s, "1234") && strings.Contains(s, "•") {
			sawMask = true
		}
	}
	if !sawMask {
		t.Error("expected the masked key to be shown so the user can identify it")
	}
}

// TestConfigureLLM_OffersEveryDescriptor: with no filter, the provider menu is
// exactly the canonical descriptor list. This is the property that keeps every
// wizard current when mcplib adds a provider.
func TestConfigureLLM_OffersEveryDescriptor(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t:       t,
		selects: []int{0, 0},
		secrets: []string{testKey},
		inputs:  []string{"http://localhost:11434"},
	}
	if _, err := ConfigureLLM(context.Background(), f, Options{}); err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if len(f.seenSelectItems) == 0 {
		t.Fatal("no Select was made")
	}
	got := len(f.seenSelectItems[0])
	want := len(llmprovider.Descriptors())
	if got != want {
		t.Errorf("provider menu offered %d choices, want all %d descriptors", got, want)
	}
}

func TestConfigureLLM_ProviderFilter(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{t: t, selects: []int{0, 0}, secrets: []string{testKey}}
	res, err := ConfigureLLM(context.Background(), f, Options{
		Providers: []string{llmprovider.ProviderGrok},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.Provider != llmprovider.ProviderGrok {
		t.Errorf("Provider = %q, want grok", res.Provider)
	}
	if n := len(f.seenSelectItems[0]); n != 1 {
		t.Errorf("filtered menu offered %d choices, want 1", n)
	}

	if _, err := ConfigureLLM(context.Background(), &fakePrompter{t: t},
		Options{Providers: []string{"nonexistent"}}); err == nil {
		t.Error("expected an error when no requested provider exists")
	}
}

// TestConfigureLLM_OtherModelEscapeHatch: a curated catalog and a live listing
// can both lag a newly released model, so the menu always ends with a manual
// entry. prepare-commit-msg's wizard had this before the migration.
func TestConfigureLLM_OtherModelEscapeHatch(t *testing.T) {
	withEnv(t, nil)
	static := llmprovider.StaticModels(llmprovider.ProviderClaude)
	f := &fakePrompter{
		t: t,
		// provider, then the trailing "Other" entry
		selects: []int{providerIdx(t, llmprovider.ProviderClaude), len(static)},
		secrets: []string{testKey},
		inputs:  []string{"my-custom-model"},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.Model != "my-custom-model" {
		t.Errorf("Model = %q, want the manually entered id", res.Model)
	}
	last := f.seenSelectItems[1][len(f.seenSelectItems[1])-1].Label
	if last != otherModelLabel {
		t.Errorf("model menu must end with %q, got %q", otherModelLabel, last)
	}
}

// TestConfigureLLM_InjectedLookupEnv: consumers drive the env-key branch
// deterministically in their own tests without touching the real environment.
func TestConfigureLLM_InjectedLookupEnv(t *testing.T) {
	withEnv(t, nil) // the package-level reader returns nothing
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderGemini), 0},
		confirms: []bool{true},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{
		AllowEnv:  true,
		LookupEnv: func(k string) string { return map[string]string{"GEMINI_API_KEY": testKey}[k] },
	})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.APIKey != testKey {
		t.Errorf("APIKey = %q, want the injected env value", res.APIKey)
	}
}
