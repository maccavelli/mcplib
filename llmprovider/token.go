package llmprovider

import (
	"context"
	"time"
)

// TokenType identifies how a token authenticates a provider request.
type TokenType string

const (
	// TokenAPIKey identifies a provider API key.
	TokenAPIKey TokenType = "api_key"
	// TokenBearer identifies a bearer token.
	TokenBearer TokenType = "bearer"
)

// Token is an authentication value returned by a TokenSource.
type Token struct {
	Value  string
	Type   TokenType
	Expiry time.Time
	Header string
}

// TokenSource supplies authentication for a provider request.
type TokenSource interface {
	Token(ctx context.Context) (Token, error)
}

// StaticToken returns one fixed bearer token without expiry.
type StaticToken struct {
	Value  string
	Header string
}

// NewStaticToken constructs a static bearer-token source.
func NewStaticToken(value string) *StaticToken {
	return &StaticToken{Value: value}
}

// Token returns the static value and its configured authorization header.
func (s *StaticToken) Token(ctx context.Context) (Token, error) {
	header := s.Header
	if header == "" {
		header = "Authorization"
	}
	return Token{
		Value:  s.Value,
		Type:   TokenBearer,
		Header: header,
	}, nil
}
