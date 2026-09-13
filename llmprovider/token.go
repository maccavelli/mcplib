package llmprovider

import (
	"context"
	"time"
)

type TokenType string

const (
	TokenAPIKey TokenType = "api_key"
	TokenBearer TokenType = "bearer"
)

type Token struct {
	Value  string
	Type   TokenType
	Expiry time.Time
	Header string
}

type TokenSource interface {
	Token(ctx context.Context) (Token, error)
}

type StaticToken struct {
	Value  string
	Header string
}

func NewStaticToken(value string) *StaticToken {
	return &StaticToken{Value: value}
}

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
