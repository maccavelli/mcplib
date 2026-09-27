package wizard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	mcplib "github.com/maccavelli/mcplib"
	"github.com/maccavelli/mcplib/llmprovider"
	"github.com/maccavelli/mcplib/logging"
)

// CredentialKind identifies the credential represented by a Result.
type CredentialKind string

const (
	// CredNone means the provider does not require a credential.
	CredNone CredentialKind = ""
	// CredAPIKey means APIKey contains a static provider key.
	CredAPIKey CredentialKind = "api_key"
	// CredOAuth means the OAuth fields contain a refreshable or access-only session.
	CredOAuth CredentialKind = "oauth"
)

// ErrOrchestrated reports that an orchestrator-owned process must use the LLM backplane.
var ErrOrchestrated = errors.New("wizard: orchestrated process uses the LLM backplane, not provider OAuth")

var (
	loginBrowserOAuth = llmprovider.LoginBrowserOAuth
	loginDeviceOAuth  = llmprovider.LoginDeviceOAuth
)

type resolvedCredential struct {
	kind    CredentialKind
	apiKey  string
	session *llmprovider.OAuthSession
	source  llmprovider.TokenSource
}

func orchestrated(o Options) bool {
	if o.Orchestrated != nil {
		return *o.Orchestrated
	}
	return mcplib.IsOrchestratorOwned()
}

func resolveCredential(
	ctx context.Context,
	p Prompter,
	d llmprovider.ProviderDescriptor,
	o Options,
) (resolvedCredential, error) {
	if len(d.AuthMethods) == 0 {
		key, err := resolveAPIKey(p, d, o)
		if err != nil {
			return resolvedCredential{}, err
		}
		kind := CredNone
		if d.RequiresAPIKey {
			kind = CredAPIKey
		}
		return staticCredential(kind, key), nil
	}

	choices := make([]Choice, 0, len(d.AuthMethods))
	for _, method := range d.AuthMethods {
		choices = append(choices, Choice{Label: method.Label, Detail: method.Detail})
	}
	idx, err := p.Select(fmt.Sprintf("Choose how to authenticate with %s:", d.Label), choices, 0)
	if err != nil {
		return resolvedCredential{}, fmt.Errorf("select authentication method: %w", err)
	}
	method := d.AuthMethods[idx].ID
	if method == llmprovider.AuthAPIKey {
		key, keyErr := resolveAPIKey(p, d, o)
		if keyErr != nil {
			return resolvedCredential{}, keyErr
		}
		return staticCredential(CredAPIKey, key), nil
	}
	if method == llmprovider.AuthTokenStdin {
		return resolveTokenStdin(ctx, p, d, o)
	}
	if o.TokenStore == nil {
		return resolvedCredential{}, errors.New("wizard: TokenStore is required for OAuth")
	}

	switch method {
	case llmprovider.AuthBrowserOAuth:
		if session, keep, keepErr := keepExistingOAuth(p, d, o); keepErr != nil {
			return resolvedCredential{}, keepErr
		} else if keep {
			return oauthCredential(session), nil
		}
		flow, drain := browserFlowOptions(p, o)
		session, loginErr := loginBrowserOAuth(ctx, d.ID, flow)
		drain()
		if loginErr != nil {
			return resolvedCredential{}, loginErr
		}
		return saveOAuthCredential(ctx, o.TokenStore, d.ID, session)
	case llmprovider.AuthDeviceCode:
		session, loginErr := loginDeviceOAuth(ctx, d.ID, oauthFlowOptions(p))
		if loginErr != nil {
			return resolvedCredential{}, loginErr
		}
		return saveOAuthCredential(ctx, o.TokenStore, d.ID, session)
	case llmprovider.AuthImportVendorCLI:
		session, importErr := importVendorSession(d.ID, o)
		if importErr != nil {
			return resolvedCredential{}, importErr
		}
		use, confirmErr := p.Confirm(
			fmt.Sprintf("Import the existing session (%s)?", logging.MaskSecret(session.Access)), true)
		if confirmErr != nil {
			return resolvedCredential{}, confirmErr
		}
		if !use {
			return resolvedCredential{}, errors.New("wizard: vendor session import declined")
		}
		return saveOAuthCredential(ctx, o.TokenStore, d.ID, session)
	default:
		return resolvedCredential{}, fmt.Errorf("wizard: unsupported authentication method %q", method)
	}
}

func staticCredential(kind CredentialKind, key string) resolvedCredential {
	return resolvedCredential{
		kind:   kind,
		apiKey: key,
		source: llmprovider.NewStaticToken(key),
	}
}

func oauthCredential(session *llmprovider.OAuthSession) resolvedCredential {
	return resolvedCredential{kind: CredOAuth, session: session, source: session}
}

func keepExistingOAuth(
	p Prompter,
	d llmprovider.ProviderDescriptor,
	o Options,
) (*llmprovider.OAuthSession, bool, error) {
	if o.Existing.Kind != CredOAuth || o.Existing.Provider != d.ID || o.Existing.AccessToken == "" {
		return nil, false, nil
	}
	keep, err := p.Confirm(
		fmt.Sprintf("Keep the existing session (%s)?", logging.MaskSecret(o.Existing.AccessToken)), true)
	if err != nil {
		return nil, false, err
	}
	if !keep {
		return nil, false, nil
	}
	session := &llmprovider.OAuthSession{
		Provider:  d.ID,
		Access:    o.Existing.AccessToken,
		Refresh:   o.Existing.RefreshToken,
		Expiry:    o.Existing.TokenExpiry,
		Issuer:    o.Existing.Issuer,
		ClientID:  o.Existing.ClientID,
		AccountID: o.Existing.AccountID,
		Store:     o.TokenStore,
	}
	if err := llmprovider.ValidateOAuthSession(session); err != nil {
		return nil, false, fmt.Errorf("wizard: the saved %s session cannot be kept (%w); sign in again", d.Label, err)
	}
	return session, true, nil
}

func resolveTokenStdin(
	ctx context.Context,
	p Prompter,
	d llmprovider.ProviderDescriptor,
	o Options,
) (resolvedCredential, error) {
	if d.ID == llmprovider.ProviderOpenAI && o.AllowEnv {
		if access := o.lookupEnv()("CODEX_ACCESS_TOKEN"); access != "" {
			use, err := p.Confirm(
				fmt.Sprintf("Use CODEX_ACCESS_TOKEN from the environment (%s)?", logging.MaskSecret(access)), true)
			if err != nil {
				return resolvedCredential{}, err
			}
			if use {
				return saveAccessOnlyOpenAI(ctx, o, access)
			}
		}
	}

	value, err := p.Secret(fmt.Sprintf("Paste your %s credential", d.Label))
	if err != nil {
		return resolvedCredential{}, fmt.Errorf("enter credential: %w", err)
	}
	if d.ID == llmprovider.ProviderOpenAI && !strings.HasPrefix(value, "sk-") {
		return saveAccessOnlyOpenAI(ctx, o, value)
	}
	return staticCredential(CredAPIKey, value), nil
}

func saveAccessOnlyOpenAI(ctx context.Context, o Options, access string) (resolvedCredential, error) {
	if o.TokenStore == nil {
		return resolvedCredential{}, errors.New("wizard: TokenStore is required for OAuth")
	}
	return saveOAuthCredential(
		ctx,
		o.TokenStore,
		llmprovider.ProviderOpenAI,
		accessOnlyOpenAISession(access),
	)
}

func accessOnlyOpenAISession(access string) *llmprovider.OAuthSession {
	return &llmprovider.OAuthSession{
		Provider: llmprovider.ProviderOpenAI,
		Access:   access,
		Issuer:   llmprovider.DefaultOpenAIIssuer,
		ClientID: llmprovider.DefaultOpenAIClientID,
	}
}

func oauthFlowOptions(p Prompter) llmprovider.OAuthFlowOptions {
	return llmprovider.OAuthFlowOptions{
		NotifyDevice: func(verificationURL, userCode string) {
			p.Notify(LevelInfo, "Open %s and enter code %s", verificationURL, userCode)
		},
	}
}

const pasteCodePrompt = "Paste the redirected URL or authorization code if the browser does not return"

// browserFlowOptions adds MADR 0009 D3's paste-code race to oauthFlowOptions.
// The authorize URL and the paste instruction are shown even when the consumer
// opens the browser itself. The returned drain must run once the login
// returns: it finishes a paste prompt the loopback overtook, so no later
// prompt reads alongside it.
func browserFlowOptions(p Prompter, o Options) (llmprovider.OAuthFlowOptions, func()) {
	paste := &pastePrompt{p: p, shown: make(chan struct{})}
	flow := oauthFlowOptions(p)
	flow.OpenURL = func(authorizeURL string) error {
		paste.show(authorizeURL)
		if o.OpenURL == nil {
			return nil
		}
		return o.OpenURL(authorizeURL)
	}
	flow.InputCode = paste.input
	return flow, paste.drain
}

// pastePrompt is the paste-code prompt that races the loopback. The login
// calls show and input on their own goroutines, so the prompter is used by
// at most one of them at a time: input waits for show, and at most one read
// is ever pending.
type pastePrompt struct {
	p     Prompter
	shown chan struct{} // closed once the URL and instruction are shown

	mu      sync.Mutex
	done    bool          // drain ran: nothing further may use the prompter
	pending chan struct{} // closed when the started read returns
}

func (s *pastePrompt) show(authorizeURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	s.p.Notify(LevelInfo, "Open %s in your browser", authorizeURL)
	s.p.Notify(LevelInfo, "If the browser does not return here, paste the redirected URL or the authorization code")
	close(s.shown)
}

func (s *pastePrompt) input(ctx context.Context) (string, error) {
	select {
	case <-s.shown:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	s.mu.Lock()
	if s.done || ctx.Err() != nil {
		s.mu.Unlock()
		return "", context.Canceled
	}
	pending := make(chan struct{})
	s.pending = pending
	s.mu.Unlock()
	defer close(pending)
	return s.p.Input(pasteCodePrompt, "")
}

func (s *pastePrompt) drain() {
	s.mu.Lock()
	s.done = true
	pending := s.pending
	s.mu.Unlock()
	if pending == nil {
		return
	}
	select {
	case <-pending:
	default:
		s.p.Notify(LevelInfo, "Browser sign-in finished; press Enter to continue")
		<-pending
	}
}

func saveOAuthCredential(
	ctx context.Context,
	store llmprovider.TokenStore,
	provider string,
	session *llmprovider.OAuthSession,
) (resolvedCredential, error) {
	if session == nil {
		return resolvedCredential{}, errors.New("wizard: OAuth login returned no session")
	}
	if err := llmprovider.ValidateOAuthSession(session); err != nil {
		return resolvedCredential{}, fmt.Errorf("wizard: refusing to save the OAuth session: %w", err)
	}
	session.Provider = provider
	session.Store = store
	if err := store.Save(ctx, provider, session); err != nil {
		return resolvedCredential{}, fmt.Errorf("wizard: save OAuth session: %w", err)
	}
	return oauthCredential(session), nil
}
