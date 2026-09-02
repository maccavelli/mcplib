package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

type stubSource struct {
	body []byte
	err  error
}

func (s stubSource) Latest(context.Context) (Release, error) { return Release{}, nil }
func (s stubSource) ByTag(context.Context, string) (Release, error) {
	return Release{}, nil
}
func (s stubSource) OpenAsset(context.Context, Release, Asset) (io.ReadCloser, error) {
	if s.err != nil {
		return nil, s.err
	}
	return io.NopCloser(bytes.NewReader(s.body)), nil
}

func TestCopyLimited(t *testing.T) {
	payload := []byte("hello")
	want := shaHex(payload)
	var buf bytes.Buffer
	n, digest, err := copyLimited(bytes.NewReader(payload), &buf, 5, 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 || digest != want || buf.String() != "hello" {
		t.Fatalf("n=%d digest=%s body=%q", n, digest, buf.String())
	}
}

func TestCopyLimitedOverflowAndShort(t *testing.T) {
	if _, _, err := copyLimited(strings.NewReader("hello!!"), io.Discard, 5, 100); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("long body err = %v", err)
	}
	if _, _, err := copyLimited(strings.NewReader("hi"), io.Discard, 5, 100); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("short body err = %v", err)
	}
	if _, _, err := copyLimited(strings.NewReader("hello"), io.Discard, 5, 4); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("advertised over limit err = %v", err)
	}
}

func TestDownloadAssetDigestMismatch(t *testing.T) {
	payload := []byte("hello")
	src := stubSource{body: payload}
	asset := Asset{
		ID:     1,
		Name:   "demo-linux-amd64",
		State:  "uploaded",
		Size:   5,
		Digest: "sha256:" + strings.Repeat("ab", 32),
	}
	_, err := downloadAsset(context.Background(), src, Release{ID: 1}, asset, io.Discard, 100)
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadAssetSuccess(t *testing.T) {
	payload := []byte("hello")
	sum := sha256.Sum256(payload)
	src := stubSource{body: payload}
	asset := Asset{
		ID:     1,
		Name:   "demo-linux-amd64",
		State:  "uploaded",
		Size:   5,
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
	}
	var buf bytes.Buffer
	got, err := downloadAsset(context.Background(), src, Release{ID: 1}, asset, &buf, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != hex.EncodeToString(sum[:]) || buf.String() != "hello" {
		t.Fatalf("digest=%s body=%q", got, buf.String())
	}
}

func TestDownloadAssetOverflow(t *testing.T) {
	src := stubSource{body: []byte(strings.Repeat("x", 50))}
	asset := Asset{ID: 1, Name: "demo-linux-amd64", State: "uploaded", Size: 50}
	_, err := downloadAsset(context.Background(), src, Release{ID: 1}, asset, io.Discard, 16)
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v", err)
	}
}

func TestReadBoundedOverflow(t *testing.T) {
	_, err := readBounded(strings.NewReader(strings.Repeat("x", 8)), 4)
	if err == nil {
		t.Fatal("accepted overflow")
	}
}
