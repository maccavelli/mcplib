package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OAuthSession is declared here as a minimal stub so Phase 2's FileTokenStore
// can be tested in isolation. Phase 3 will extend this type with the full
// refresh + persist-before-use behaviour and the locked method set.
type OAuthSession struct {
	Provider   string
	Access     string
	Refresh    string
	Expiry     time.Time
	Issuer     string
	ClientID   string
	AccountID  string
	TokenURL   string
	Store      TokenStore
	HTTPClient *http.Client
}

// TokenStore interface (Phase 2 introduces this; Phase 3 adds methods that
// depend on it).
type TokenStore interface {
	Load(ctx context.Context, provider string) (*OAuthSession, error)
	Save(ctx context.Context, provider string, s *OAuthSession) error
	Delete(ctx context.Context, provider string) error
}

// fileRecord is the on-disk JSON shape.
type fileRecord struct {
	Provider  string    `json:"provider"`
	Access    string    `json:"access"`
	Refresh   string    `json:"refresh"`
	Expiry    time.Time `json:"expiry"`
	Issuer    string    `json:"issuer"`
	ClientID  string    `json:"client_id"`
	AccountID string    `json:"account_id"`
	TokenURL  string    `json:"token_url"`
}

// ErrInvalidProvider is returned when a provider id is empty or path-traverses.
var ErrInvalidProvider = errors.New("invalid provider id")

func validateProviderID(p string) error {
	if p == "" {
		return fmt.Errorf("%w: empty", ErrInvalidProvider)
	}
	if strings.ContainsAny(p, `/\`) || strings.Contains(p, "..") {
		return fmt.Errorf("%w: %q", ErrInvalidProvider, p)
	}
	return nil
}
