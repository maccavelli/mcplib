package llmprovider

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestPKCE_ChallengeIsS256(t *testing.T) {
	t.Parallel()

	pkce, err := newPKCE()
	if err != nil {
		t.Fatalf("newPKCE() error = %v", err)
	}

	verifierBytes, err := base64.RawURLEncoding.DecodeString(pkce.verifier)
	if err != nil {
		t.Fatalf("decode verifier: %v", err)
	}
	if len(verifierBytes) != 32 {
		t.Fatalf("verifier entropy = %d bytes, want 32", len(verifierBytes))
	}

	digest := sha256.Sum256([]byte(pkce.verifier))
	want := base64.RawURLEncoding.EncodeToString(digest[:])
	if pkce.challenge != want {
		t.Fatalf("challenge = %q, want independently computed S256 %q", pkce.challenge, want)
	}
}
