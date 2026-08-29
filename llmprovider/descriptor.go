package llmprovider

// ProviderDescriptor is the single source of truth for everything a
// configuration UI needs to know about a provider: what to call it, which
// environment variable holds its credential, whether it needs one at all, and
// which models to offer before a live listing is available.
//
// Fields that already exist elsewhere in this package — EnvVar from
// ProviderEnvVars, StaticModels from StaticModels() — are DERIVED here, not
// duplicated, so a change there cannot drift from what a wizard shows. That
// drift is the reason this type exists: three wizards previously kept their own
// provider lists, and none of them offered Grok.
type ProviderDescriptor struct {
	// ID is the canonical identifier accepted by NewProvider.
	ID string
	// Label is the human-readable name for a menu.
	Label string
	// EnvVar is the conventional environment variable for the credential.
	// Empty when RequiresAPIKey is false.
	EnvVar string
	// DefaultBaseURL is the endpoint used when none is configured. Empty when
	// the provider has no meaningful default to show a user.
	DefaultBaseURL string
	// SupportsBaseURL reports whether a caller may override the endpoint.
	SupportsBaseURL bool
	// IsLocal reports whether the provider runs on the user's machine.
	IsLocal bool
	// RequiresAPIKey reports whether a credential must be collected.
	RequiresAPIKey bool
	// StaticModels is the curated fallback catalog, used before or instead of
	// a live listing. May be empty for providers whose models are entirely
	// machine-specific.
	StaticModels []string
	// Notes is a short qualifier for a menu, e.g. pricing model. May be empty.
	Notes string
}

// descriptorSpecs holds only what is NOT derivable from existing package data.
// Order is menu order: remote providers first, local last.
var descriptorSpecs = []struct {
	id, label, defaultBaseURL, notes         string
	supportsBaseURL, isLocal, requiresAPIKey bool
}{
	{id: ProviderGemini, label: "Gemini (Google)", requiresAPIKey: true},
	{id: ProviderOpenAI, label: "OpenAI", requiresAPIKey: true},
	{id: ProviderClaude, label: "Claude (Anthropic)", requiresAPIKey: true},
	{id: ProviderGrok, label: "Grok (xAI)", requiresAPIKey: true},
	{
		id: ProviderOpencodeZen, label: "OpenCode Zen",
		defaultBaseURL: opencodeZenBaseURL, supportsBaseURL: true, requiresAPIKey: true,
		notes: "pay-as-you-go; free models available",
	},
	{
		id: ProviderOpencodeGo, label: "OpenCode Go",
		defaultBaseURL: opencodeGoBaseURL, supportsBaseURL: true, requiresAPIKey: true,
		notes: "subscription",
	},
	{
		id: ProviderHuggingFace, label: "Hugging Face",
		defaultBaseURL: huggingFaceBaseURL, supportsBaseURL: true, requiresAPIKey: true,
		notes: "monthly credits; no free tier",
	},
	{
		id: ProviderKilo, label: "Kilo Gateway",
		defaultBaseURL: kiloBaseURL, supportsBaseURL: true, requiresAPIKey: true,
		notes: "free models available",
	},
}

// Descriptors returns every provider a configuration UI may offer, in menu
// order. The returned slice and its StaticModels slices are copies: a caller
// mutating them cannot affect the next call.
//
// Adding a provider to this package means adding one spec entry beside its
// constant; every wizard built on Descriptors then offers it with no
// downstream edit. That property is asserted by
// TestDescriptors_CoverEveryRegisteredProvider.
func Descriptors() []ProviderDescriptor {
	out := make([]ProviderDescriptor, 0, len(descriptorSpecs))
	for _, s := range descriptorSpecs {
		d := ProviderDescriptor{
			ID:              s.id,
			Label:           s.label,
			DefaultBaseURL:  s.defaultBaseURL,
			SupportsBaseURL: s.supportsBaseURL,
			IsLocal:         s.isLocal,
			RequiresAPIKey:  s.requiresAPIKey,
			Notes:           s.notes,
			StaticModels:    StaticModels(s.id), // already returns a copy
		}
		if s.requiresAPIKey {
			d.EnvVar = ProviderEnvVars[s.id]
		}
		out = append(out, d)
	}
	return out
}

// DescriptorFor returns the descriptor for a canonical provider id.
func DescriptorFor(id string) (ProviderDescriptor, bool) {
	for _, d := range Descriptors() {
		if d.ID == id {
			return d, true
		}
	}
	return ProviderDescriptor{}, false
}
