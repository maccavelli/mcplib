package selfupdate

import (
	"crypto/subtle"
	"fmt"
)

func verifyIntegrity(v Verification) error {
	if v.SHA256 == "" || v.ManifestSHA256 == "" {
		return fmt.Errorf("selfupdate: missing digest: %w", ErrIntegrity)
	}
	if !equalDigest(v.SHA256, v.ManifestSHA256) {
		return fmt.Errorf("selfupdate: staged digest does not match SHA256SUMS: %w", ErrIntegrity)
	}
	if v.GitHubSHA256 != "" && !equalDigest(v.SHA256, v.GitHubSHA256) {
		return fmt.Errorf("selfupdate: staged digest does not match github asset digest: %w", ErrIntegrity)
	}
	return nil
}

func verifyManifest(data []byte, manifestName, staged string) error {
	entries, err := parseSHA256SUMS(data)
	if err != nil {
		return err
	}
	want, err := checksumFor(entries, manifestName)
	if err != nil {
		return err
	}
	if !equalDigest(staged, want) {
		return fmt.Errorf("selfupdate: staged digest does not match SHA256SUMS: %w", ErrIntegrity)
	}
	return nil
}

func equalDigest(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
