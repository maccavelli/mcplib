package llmprovider

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

type pkceCodes struct {
	verifier  string
	challenge string
}

func newPKCE() (pkceCodes, error) {
	verifier, err := randomBase64URL(32)
	if err != nil {
		return pkceCodes{}, fmt.Errorf("oauth: generate PKCE verifier: %w", err)
	}
	digest := sha256.Sum256([]byte(verifier))
	return pkceCodes{
		verifier:  verifier,
		challenge: base64.RawURLEncoding.EncodeToString(digest[:]),
	}, nil
}

func randomBase64URL(byteCount int) (string, error) {
	random := make([]byte, byteCount)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}
