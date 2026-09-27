package llmprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// vendorAuthFileLimit bounds a vendor CLI auth file.
const vendorAuthFileLimit = 1 << 20

// VendorCLISession is a TokenSource that borrows a vendor CLI's login
// read-through (MADR 0012 §5.1). Every Token call re-reads the CLI's auth
// file and returns its access token; it never refreshes. The refresh token
// stays the CLI's alone, because both vendors revoke a refresh-token family
// when one is used twice (grok-build xai-grok-login/src/oidc/refresh.rs:29-35;
// codex login/src/auth/manager.rs:1657-1690). An expired token is
// ErrAuthFailure telling the user to run the CLI, which refreshes it.
type VendorCLISession struct {
	// Provider is ProviderOpenAI (the Codex CLI's auth.json) or ProviderGrok
	// (the Grok CLI's auth.json).
	Provider string
	// Path is the CLI's auth file.
	Path string

	mu        sync.Mutex
	accountID string // the ChatGPT account id last read
	fedramp   bool   // the id token's chatgpt_account_is_fedramp, last read
}

// vendorCredential is what one read of a CLI auth file yields.
type vendorCredential struct {
	access    string
	expiry    time.Time
	accountID string
	fedramp   bool
}

// Token re-reads the auth file and returns its access token, or
// ErrAuthFailure when the file is unreadable, holds no token, or the token
// has expired.
func (s *VendorCLISession) Token(ctx context.Context) (Token, error) {
	if err := ctx.Err(); err != nil {
		return Token{}, err
	}
	cli, hint, parse := vendorCLI(s.Provider)
	if parse == nil {
		return Token{}, fmt.Errorf("%w: no vendor CLI session for provider %q", ErrInvalidRequest, s.Provider)
	}
	raw, err := readVendorAuthFile(s.Path)
	if err != nil {
		return Token{}, fmt.Errorf("%w: read the %s login: %w; %s", ErrAuthFailure, cli, err, hint)
	}
	cred, err := parse(raw)
	if err != nil {
		return Token{}, fmt.Errorf("%w: the %s login in %s: %w; %s", ErrAuthFailure, cli, s.Path, err, hint)
	}
	if !cred.expiry.IsZero() && !time.Now().Before(cred.expiry) {
		return Token{}, fmt.Errorf("%w: the %s login in %s expired at %s; %s",
			ErrAuthFailure, cli, s.Path, cred.expiry.Format(time.RFC3339), hint)
	}
	s.mu.Lock()
	s.accountID, s.fedramp = cred.accountID, cred.fedramp
	s.mu.Unlock()
	return Token{Value: cred.access, Type: TokenBearer, Expiry: cred.expiry, Header: oauthAuthorizationHeader}, nil
}

// vendorCLI names a provider's CLI, the advice for a stale login, and its
// auth-file parser; the parser is nil for a provider without one.
func vendorCLI(provider string) (cli, hint string, parse func([]byte) (vendorCredential, error)) {
	switch provider {
	case ProviderOpenAI:
		return "Codex CLI", "run codex to refresh it, or codex login", parseCodexAuth
	case ProviderGrok:
		return "Grok CLI", "run grok to refresh it, or grok login", parseGrokAuth
	default:
		return "", "", nil
	}
}

// readVendorAuthFile reads at most vendorAuthFileLimit bytes of path, opened
// within its directory.
func readVendorAuthFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { ignoreOAuthError(root.Close()) }()
	f, err := root.Open(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	defer func() { ignoreOAuthError(f.Close()) }()
	return io.ReadAll(io.LimitReader(f, vendorAuthFileLimit))
}

// parseCodexAuth reads ~/.codex/auth.json's ChatGPT tokens. Expiry is the
// access token's JWT exp, as Codex reads it (login/src/auth/manager.rs:3004-3026).
func parseCodexAuth(raw []byte) (vendorCredential, error) {
	var file struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
			IDToken     string `json:"id_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return vendorCredential{}, fmt.Errorf("decode: %w", err)
	}
	if file.Tokens.AccessToken == "" {
		return vendorCredential{}, fmt.Errorf("no ChatGPT access token")
	}
	return vendorCredential{
		access:    file.Tokens.AccessToken,
		expiry:    jwtExpiry(file.Tokens.AccessToken),
		accountID: file.Tokens.AccountID,
		fedramp:   chatGPTFedRAMP(file.Tokens.IDToken),
	}, nil
}

// grokAuthScope is the Grok CLI's key for its default login,
// "{issuer}::{client_id}" (xai-grok-login/src/config.rs:188-196, :241-256).
var grokAuthScope = strings.TrimRight(DefaultGrokOAuthIssuer, "/") + "::" + DefaultGrokOAuthClientID

// parseGrokAuth reads the Grok CLI's auth.json: exactly the default login's
// entry, never another scope. Expiry is the entry's expires_at, else the
// access token's JWT exp.
func parseGrokAuth(raw []byte) (vendorCredential, error) {
	var file map[string]struct {
		Key       string `json:"key"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return vendorCredential{}, fmt.Errorf("decode: %w", err)
	}
	entry, ok := file[grokAuthScope]
	if !ok || entry.Key == "" {
		return vendorCredential{}, fmt.Errorf("no %s login", grokAuthScope)
	}
	expiry := jwtExpiry(entry.Key)
	if entry.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, entry.ExpiresAt)
		if err != nil {
			return vendorCredential{}, fmt.Errorf("expires_at: %w", err)
		}
		expiry = parsed
	}
	return vendorCredential{access: entry.Key, expiry: expiry}, nil
}

// jwtExpiry reads a JWT's numeric exp claim, or returns the zero time for a
// token that is not a JWT or has none.
func jwtExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp *float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == nil {
		return time.Time{}
	}
	return time.Unix(int64(*claims.Exp), 0).UTC()
}

// vendorAccount returns the ChatGPT account id and FedRAMP flag a Codex CLI
// session last read.
func (s *VendorCLISession) vendorAccount() (accountID string, fedramp bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accountID, s.fedramp
}
