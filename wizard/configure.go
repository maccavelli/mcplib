package wizard

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/maccavelli/mcplib/llmprovider"
	"github.com/maccavelli/mcplib/logging"
)

// defaultDiscoverLimit bounds a live model listing so a slow or unreachable
// provider cannot stall a wizard indefinitely.
const defaultDiscoverLimit = 20 * time.Second

// Result is what ConfigureLLM produces. It is deliberately data, not config:
// each consumer persists it in its own schema. Unifying configuration storage
// across the three wizards is a separate decision (MADR 0004, Out of scope).
type Result struct {
	Provider     string
	Kind         CredentialKind
	APIKey       string
	AccessToken  string
	RefreshToken string
	TokenExpiry  time.Time
	Issuer       string
	ClientID     string
	AccountID    string
	Model        string
	BaseURL      string
	Fallbacks    []string
}

// Options controls the flow. The zero value runs a full interactive
// configuration over every known provider with no discovery.
type Options struct {
	// Providers restricts the menu to these ids. Empty offers every
	// descriptor, which is the behaviour that keeps a wizard current when
	// mcplib adds a provider.
	Providers []string
	// Existing pre-fills the flow, enabling "keep existing key?" and
	// defaulting the model selection.
	Existing Result
	// AllowEnv offers a credential found in the provider's environment
	// variable.
	AllowEnv bool
	// Discover queries the provider's live model listing. When false, or when
	// the listing fails or is empty, the static catalog is used. A ChatGPT
	// session has no static catalog, so the user is asked for a model id.
	Discover bool
	// DiscoverLimit bounds the listing call. Zero uses defaultDiscoverLimit.
	DiscoverLimit time.Duration
	// NeedFallbacks collects additional models after the primary.
	NeedFallbacks bool
	// Profile selects how the open catalogs (Kilo, OpenCode Zen and Go,
	// Hugging Face) rank the recommended models. The zero value,
	// llmprovider.ProfileUtility, suits short frequent tasks such as commit
	// messages; llmprovider.ProfileCapable suits reasoning-heavy tiers.
	Profile llmprovider.ModelProfile
	// LookupEnv reads an environment variable. Nil uses os.Getenv. Consumers
	// inject this to drive the flow deterministically in their own tests.
	LookupEnv func(string) string
	// TokenStore persists sessions created by browser, device-code, and import flows.
	TokenStore llmprovider.TokenStore
	// HTTPClient is used for live model listing. Nil uses the package default.
	HTTPClient *http.Client
	// OpenURL opens an OAuth authorization URL. Nil reports the URL through Prompter.
	OpenURL func(string) error
	// Orchestrated overrides process ownership detection when non-nil.
	Orchestrated *bool
}

// getenv is indirected for this package's own tests.
var getenv = os.Getenv

// lookupEnv resolves the environment reader for a run: the caller's when
// supplied, this package's otherwise.
func (o Options) lookupEnv() func(string) string {
	if o.LookupEnv != nil {
		return o.LookupEnv
	}
	return getenv
}

// ConfigureLLM runs the canonical provider configuration flow: choose a
// provider, resolve a base URL, resolve a credential, choose a model, and
// optionally choose fallbacks.
//
// It renders nothing itself — every interaction goes through p — so a consumer
// keeps its own look and feel, and the flow is testable with a scripted
// Prompter and no TTY.
//
// It never writes configuration and never logs a credential: the key appears
// only in the returned Result, and anything shown to the user is masked with
// logging.MaskSecret.
func ConfigureLLM(ctx context.Context, p Prompter, o Options) (Result, error) {
	if orchestrated(o) {
		return Result{}, ErrOrchestrated
	}
	descriptors, err := selectableDescriptors(o.Providers)
	if err != nil {
		return Result{}, err
	}

	choices := make([]Choice, 0, len(descriptors))
	defaultIdx := 0
	for i, d := range descriptors {
		choices = append(choices, Choice{Label: d.Label, Detail: d.Notes})
		if d.ID == o.Existing.Provider {
			defaultIdx = i
		}
	}

	idx, err := p.Select("Choose an LLM provider:", choices, defaultIdx)
	if err != nil {
		return Result{}, fmt.Errorf("select provider: %w", err)
	}
	d := descriptors[idx]
	res := Result{Provider: d.ID}

	if res.BaseURL, err = resolveBaseURL(ctx, p, d, o); err != nil {
		return Result{}, err
	}
	credential, err := resolveCredential(ctx, p, d, o)
	if err != nil {
		return Result{}, err
	}
	res.Kind = credential.kind
	res.APIKey = credential.apiKey
	if credential.session != nil {
		res.AccessToken = credential.session.Access
		res.RefreshToken = credential.session.Refresh
		res.TokenExpiry = credential.session.Expiry
		res.Issuer = credential.session.Issuer
		res.ClientID = credential.session.ClientID
		res.AccountID = credential.session.AccountID
	}

	cat := discoverModels(ctx, p, d, res, credential.source, o)
	if len(cat.Recommended) == 0 {
		// Ollama with nothing installed, or a provider whose listing failed
		// and which has no static catalog. Let the user type an id rather
		// than dead-ending the wizard.
		manual, inputErr := p.Input("No models found; enter a model id", o.Existing.Model)
		if inputErr != nil {
			return Result{}, fmt.Errorf("enter model: %w", inputErr)
		}
		if manual == "" {
			// Returning Result{Model: ""} would hand the caller a
			// configuration that cannot generate anything.
			return Result{}, fmt.Errorf("wizard: no model available for %s and none entered", d.Label)
		}
		res.Model = manual
		return res, nil
	}

	if res.Model, err = selectModel(p, d, cat, o); err != nil {
		return Result{}, err
	}
	if o.NeedFallbacks {
		if res.Fallbacks, err = selectFallbacks(p, d, cat, res.Model); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}

// selectableDescriptors returns the descriptors a run may offer, preserving
// canonical menu order.
func selectableDescriptors(allow []string) ([]llmprovider.ProviderDescriptor, error) {
	all := llmprovider.Descriptors()
	if len(allow) == 0 {
		return all, nil
	}
	wanted := make(map[string]struct{}, len(allow))
	for _, id := range allow {
		wanted[id] = struct{}{}
	}
	var out []llmprovider.ProviderDescriptor
	for _, d := range all {
		if _, ok := wanted[d.ID]; ok {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("wizard: none of the requested providers exist: %v", allow)
	}
	return out, nil
}

// resolveBaseURL prompts for an endpoint when the provider supports one. A
// local provider is validated for reachability and re-prompted on failure.
func resolveBaseURL(ctx context.Context, p Prompter, d llmprovider.ProviderDescriptor, o Options) (string, error) {
	if !d.SupportsBaseURL {
		return "", nil
	}
	def := d.DefaultBaseURL
	if o.Existing.Provider == d.ID && o.Existing.BaseURL != "" {
		def = o.Existing.BaseURL
	}
	for {
		url, err := p.Input(fmt.Sprintf("%s endpoint", d.Label), def)
		if err != nil {
			return "", fmt.Errorf("enter base URL: %w", err)
		}
		if !d.IsLocal {
			return url, nil
		}
		vErr := llmprovider.ValidateOllamaURL(ctx, url)
		if vErr == nil {
			return url, nil
		}
		p.Notify(LevelWarn, "cannot reach %s: %v", url, vErr)
		again, err := p.Confirm("Try a different endpoint?", true)
		if err != nil {
			return "", err
		}
		if !again {
			return url, nil
		}
	}
}

// resolveAPIKey applies the precedence environment → existing → prompt. Any
// key shown to the user is masked; the raw value only ever reaches Result.
func resolveAPIKey(p Prompter, d llmprovider.ProviderDescriptor, o Options) (string, error) {
	if !d.RequiresAPIKey {
		return "", nil
	}

	if o.AllowEnv && d.EnvVar != "" {
		if envVal := o.lookupEnv()(d.EnvVar); envVal != "" {
			use, err := p.Confirm(
				fmt.Sprintf("Use %s from the environment (%s)?", d.EnvVar, logging.MaskSecret(envVal)), true)
			if err != nil {
				return "", err
			}
			if use {
				return envVal, nil
			}
		}
	}

	if o.Existing.Provider == d.ID && o.Existing.APIKey != "" {
		keep, err := p.Confirm(
			fmt.Sprintf("Keep the existing key (%s)?", logging.MaskSecret(o.Existing.APIKey)), true)
		if err != nil {
			return "", err
		}
		if keep {
			return o.Existing.APIKey, nil
		}
	}

	key, err := p.Secret(fmt.Sprintf("Enter your %s API key", d.Label))
	if err != nil {
		return "", fmt.Errorf("enter API key: %w", err)
	}
	return key, nil
}

// discoverModels returns the catalog to offer: the live listing when requested
// and available, otherwise the descriptor's static catalog in both views
// (MADR 0009 §4).
func discoverModels(
	ctx context.Context,
	p Prompter,
	d llmprovider.ProviderDescriptor,
	res Result,
	source llmprovider.TokenSource,
	o Options,
) llmprovider.ModelCatalog {
	chatGPT := res.Kind == CredOAuth && d.ID == llmprovider.ProviderOpenAI
	// A ChatGPT session lists only from the Codex backend (MADR 0009 D11):
	// the Platform catalog is not available to it, so there is no fallback.
	static := d.StaticModels
	if chatGPT {
		static = nil
	}
	fallback := llmprovider.ModelCatalog{Recommended: static, Usable: static}
	if !o.Discover {
		return fallback
	}
	limit := o.DiscoverLimit
	if limit <= 0 {
		limit = defaultDiscoverLimit
	}
	dCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	opts := []llmprovider.ProviderOption{llmprovider.WithModelProfile(o.Profile)}
	if res.BaseURL != "" {
		opts = append(opts, llmprovider.WithBaseURL(res.BaseURL))
	}
	if o.HTTPClient != nil {
		opts = append(opts, llmprovider.WithHTTPClient(o.HTTPClient))
	}
	cat, err := llmprovider.ListModelCatalogWithSource(dCtx, d.ID, source, opts...)
	if err != nil {
		if len(static) == 0 {
			p.Notify(LevelWarn, "could not list models for %s (%v)", d.Label, err)
			return fallback
		}
		p.Notify(LevelWarn, "could not list models for %s (%v); using the built-in catalog", d.Label, err)
		return fallback
	}
	if len(cat.Recommended) == 0 {
		return fallback
	}
	if !cat.Live && !chatGPT {
		p.Notify(LevelInfo, "live model listing for %s is unavailable; search covers the built-in catalog only", d.Label)
	}
	return cat
}

func modelChoices(provider string, models []string) []Choice {
	out := make([]Choice, 0, len(models))
	for _, m := range models {
		label := llmprovider.ModelLabel(provider, m)
		if label == m {
			out = append(out, Choice{Label: m})
			continue
		}
		out = append(out, Choice{Label: label})
	}
	return out
}
