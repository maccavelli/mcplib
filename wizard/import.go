package wizard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maccavelli/mcplib/llmprovider"
)

const grokOAuthRefreshEndpoint = "https://auth.x.ai/oauth2/token"

type openAIAuthFile struct {
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

type grokAuthFileEntry struct {
	Key          string `json:"key"`
	AuthMode     string `json:"auth_mode"`
	Refresh      string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
	OIDCIssuer   string `json:"oidc_issuer"`
	OIDCClientID string `json:"oidc_client_id"`
}

func importVendorSession(provider string, o Options) (*llmprovider.OAuthSession, error) {
	dir, err := vendorAuthDir(provider, o)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("wizard: open %s vendor session directory: %w", provider, err)
	}
	data, readErr := root.ReadFile("auth.json")
	closeErr := root.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("wizard: read %s vendor session: %w", provider, errors.Join(readErr, closeErr))
	}
	switch provider {
	case llmprovider.ProviderOpenAI:
		return importOpenAIAuth(data)
	case llmprovider.ProviderGrok:
		return importGrokAuth(data)
	default:
		return nil, fmt.Errorf("wizard: provider %q has no vendor session import", provider)
	}
}

func vendorAuthDir(provider string, o Options) (string, error) {
	var envName, defaultDir string
	switch provider {
	case llmprovider.ProviderOpenAI:
		envName, defaultDir = "CODEX_HOME", ".codex"
	case llmprovider.ProviderGrok:
		envName, defaultDir = "GROK_HOME", ".grok"
	default:
		return "", fmt.Errorf("wizard: provider %q has no vendor auth path", provider)
	}
	if dir := o.lookupEnv()(envName); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("wizard: resolve home directory: %w", err)
	}
	return filepath.Join(home, defaultDir), nil
}

func importOpenAIAuth(data []byte) (*llmprovider.OAuthSession, error) {
	var auth openAIAuthFile
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("wizard: decode OpenAI vendor session: %w", err)
	}
	if auth.Tokens.AccessToken == "" || auth.Tokens.RefreshToken == "" {
		return nil, errors.New("wizard: OpenAI vendor session requires access and refresh tokens")
	}
	return &llmprovider.OAuthSession{
		Provider:  llmprovider.ProviderOpenAI,
		Access:    auth.Tokens.AccessToken,
		Refresh:   auth.Tokens.RefreshToken,
		Issuer:    llmprovider.DefaultOpenAIIssuer,
		ClientID:  llmprovider.DefaultOpenAIClientID,
		AccountID: auth.Tokens.AccountID,
		TokenURL:  llmprovider.DefaultOpenAIIssuer + "/oauth/token",
	}, nil
}

func importGrokAuth(data []byte) (*llmprovider.OAuthSession, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("wizard: decode Grok vendor session: %w", err)
	}
	for scope, raw := range entries {
		if scope == "xai::api_key" {
			continue
		}
		var entry grokAuthFileEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("wizard: decode Grok vendor session entry: %w", err)
		}
		if entry.AuthMode == "api_key" ||
			strings.TrimRight(entry.OIDCIssuer, "/") != llmprovider.DefaultGrokOAuthIssuer {
			continue
		}
		if entry.Key == "" {
			return nil, errors.New("wizard: Grok vendor session has no access token")
		}
		var expiry time.Time
		if entry.ExpiresAt != "" {
			parsed, err := time.Parse(time.RFC3339, entry.ExpiresAt)
			if err != nil {
				return nil, fmt.Errorf("wizard: parse Grok vendor session expiry: %w", err)
			}
			expiry = parsed
		}
		clientID := entry.OIDCClientID
		if clientID == "" {
			clientID = llmprovider.DefaultGrokOAuthClientID
		}
		return &llmprovider.OAuthSession{
			Provider: llmprovider.ProviderGrok,
			Access:   entry.Key,
			Refresh:  entry.Refresh,
			Expiry:   expiry,
			Issuer:   llmprovider.DefaultGrokOAuthIssuer,
			ClientID: clientID,
			TokenURL: grokOAuthRefreshEndpoint,
		}, nil
	}
	return nil, errors.New("wizard: no Grok OAuth session found in vendor auth file")
}
