package llmprovider

import (
	"context"
	"testing"
)

func TestStaticToken_ReturnsBearer(t *testing.T) {
	tok, err := NewStaticToken("sk-test").Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.Type != TokenBearer {
		t.Errorf("Type = %q, want %q", tok.Type, TokenBearer)
	}
	if tok.Header != "Authorization" {
		t.Errorf("Header = %q, want %q", tok.Header, "Authorization")
	}
	if tok.Value != "sk-test" {
		t.Errorf("Value = %q, want %q", tok.Value, "sk-test")
	}
	if !tok.Expiry.IsZero() {
		t.Errorf("Expiry = %v, want zero", tok.Expiry)
	}
}

func TestStaticToken_EmptyValueStillReturnsToken(t *testing.T) {
	_, err := NewStaticToken("").Token(context.Background())
	if err != nil {
		t.Errorf("Token() with empty value returned error %v; want nil", err)
	}
}

func TestStaticToken_CustomHeader(t *testing.T) {
	s := &StaticToken{Value: "x", Header: "X-Custom"}
	tok, err := s.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.Header != "X-Custom" {
		t.Errorf("Header = %q, want %q", tok.Header, "X-Custom")
	}
}
