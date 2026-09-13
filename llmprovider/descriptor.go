package llmprovider

// AuthMethodID identifies a provider authentication method.
type AuthMethodID string

const (
	// AuthAPIKey selects provider API-key authentication.
	AuthAPIKey AuthMethodID = "api_key"
	// AuthBrowserOAuth selects interactive browser OAuth.
	AuthBrowserOAuth AuthMethodID = "browser_oauth"
	// AuthDeviceCode selects a headless device-code flow.
	AuthDeviceCode AuthMethodID = "device_code"
	// AuthTokenStdin selects a credential pasted through standard input.
	AuthTokenStdin AuthMethodID = "token_stdin"
	// AuthImportVendorCLI imports a session from the provider's CLI.
	AuthImportVendorCLI AuthMethodID = "import_vendor_cli"
)

// AuthMethod describes one authentication path a provider offers.
type AuthMethod struct {
	ID          AuthMethodID
	Label       string
	Detail      string
	Interactive bool
	HeadlessOK  bool
}

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
	// AuthMethods lists supported authentication paths in menu order.
	AuthMethods []AuthMethod
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
	authMethods                              []AuthMethod
}{
	{id: ProviderGemini, label: "Gemini (Google)", requiresAPIKey: true},
	{
		id: ProviderOpenAI, label: "OpenAI", requiresAPIKey: true,
		authMethods: []AuthMethod{
			{
				ID:          AuthAPIKey,
				Label:       "OpenAI API key",
				Detail:      "Platform billing (`api.openai.com`)",
				Interactive: true,
				HeadlessOK:  true,
			},
			{
				ID:          AuthBrowserOAuth,
				Label:       "Sign in with ChatGPT",
				Detail:      "Plus/Pro/Business/Edu/Enterprise plan",
				Interactive: true,
			},
			{
				ID:          AuthDeviceCode,
				Label:       "Sign in with ChatGPT (device code)",
				Interactive: true,
				HeadlessOK:  true,
			},
			{
				ID:          AuthTokenStdin,
				Label:       "Paste a ChatGPT access token or API key",
				Interactive: true,
				HeadlessOK:  true,
			},
			{
				ID:          AuthImportVendorCLI,
				Label:       "Import ~/.codex/auth.json",
				Interactive: true,
				HeadlessOK:  true,
			},
		},
	},
	{id: ProviderClaude, label: "Claude (Anthropic)", requiresAPIKey: true},
	{
		id: ProviderGrok, label: "Grok (xAI)", requiresAPIKey: true,
		authMethods: []AuthMethod{
			{
				ID:          AuthAPIKey,
				Label:       "xAI API key",
				Detail:      "console.x.ai billing (`api.x.ai`)",
				Interactive: true,
				HeadlessOK:  true,
			},
			{
				ID:          AuthBrowserOAuth,
				Label:       "Sign in with xAI",
				Interactive: true,
			},
			{
				ID:          AuthDeviceCode,
				Label:       "Sign in with xAI (device code)",
				Interactive: true,
				HeadlessOK:  true,
			},
			{
				ID:          AuthTokenStdin,
				Label:       "Paste an xAI API key",
				Interactive: true,
				HeadlessOK:  true,
			},
			{
				ID:          AuthImportVendorCLI,
				Label:       "Import ~/.grok/auth.json",
				Interactive: true,
				HeadlessOK:  true,
			},
		},
	},
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
	{
		id: ProviderOllama, label: "Ollama (local)",
		defaultBaseURL: ollamaBaseURL, supportsBaseURL: true, isLocal: true,
		requiresAPIKey: false,
		notes:          "runs on your machine; no API key",
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
			AuthMethods:     append([]AuthMethod(nil), s.authMethods...),
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
