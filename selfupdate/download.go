package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("selfupdate: body limit must be positive")
	}
	data, err := io.ReadAll(&io.LimitedReader{R: r, N: limit + 1})
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("selfupdate: response exceeds %d-byte limit", limit)
	}
	return data, nil
}

func decodeJSON(data []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("selfupdate: malformed github json: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("selfupdate: trailing github json")
	}
	return nil
}

func sanitizeDiagnostic(s string, limit int64) string {
	if int64(len(s)) > limit {
		s = s[:limit]
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 32 || r == 127 || (r >= 0x80 && r <= 0x9f) {
			b.WriteByte('?')
			continue
		}
		if !utf8.ValidRune(r) {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func copyLimited(r io.Reader, w io.Writer, advertised, limit int64) (written int64, digest string, err error) {
	if advertised <= 0 {
		return 0, "", fmt.Errorf("selfupdate: advertised size must be positive: %w", ErrIntegrity)
	}
	if advertised > limit {
		return 0, "", fmt.Errorf("selfupdate: advertised size %d exceeds limit %d: %w", advertised, limit, ErrIntegrity)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), &io.LimitedReader{R: r, N: advertised + 1})
	if err != nil {
		return n, "", err
	}
	if n != advertised {
		if n > advertised {
			return n, "", fmt.Errorf("selfupdate: body longer than advertised size %d: %w", advertised, ErrIntegrity)
		}
		return n, "", fmt.Errorf("selfupdate: body shorter than advertised size %d: %w", advertised, ErrIntegrity)
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func downloadAsset(ctx context.Context, src ReleaseSource, rel Release, asset Asset, w io.Writer, limit int64) (digest string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	rc, err := src.OpenAsset(ctx, rel, asset)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, digest, err = copyLimited(rc, w, asset.Size, limit)
	if err != nil {
		return "", err
	}
	if asset.Digest != "" {
		want, derr := parseGitHubDigest(asset.Digest)
		if derr != nil {
			return "", derr
		}
		if digest != want {
			return "", fmt.Errorf("selfupdate: downloaded digest does not match github asset digest: %w", ErrIntegrity)
		}
	}
	return digest, nil
}
