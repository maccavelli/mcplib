package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyIntegrity(t *testing.T) {
	err := verifyIntegrity(Verification{
		SHA256:         testDigest,
		ManifestSHA256: testDigest,
		GitHubSHA256:   testDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = verifyIntegrity(Verification{
		SHA256:         testDigest,
		ManifestSHA256: strings.Repeat("ab", 32),
	})
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("manifest mismatch err = %v", err)
	}
	err = verifyIntegrity(Verification{
		SHA256:         testDigest,
		ManifestSHA256: testDigest,
		GitHubSHA256:   strings.Repeat("cd", 32),
	})
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("github mismatch err = %v", err)
	}
}

func TestVerifyManifestTestdata(t *testing.T) {
	valid, err := os.ReadFile(filepath.Join("testdata", "SHA256SUMS.valid"))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyManifest(valid, "demo-linux-amd64", testDigest); err != nil {
		t.Fatal(err)
	}
	if err := verifyManifest(valid, "demo-linux-amd64", strings.Repeat("ab", 32)); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("checksum mismatch err = %v", err)
	}
	invalid, err := os.ReadFile(filepath.Join("testdata", "SHA256SUMS.invalid"))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyManifest(invalid, "demo-linux-amd64", testDigest); err == nil {
		t.Fatal("accepted invalid testdata manifest")
	}
}
