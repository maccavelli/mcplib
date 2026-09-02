package selfupdate

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	githubDigestPrefix = "sha256:"
	sha256HexLen       = 64
	maxChecksumLine    = 4096
)

func parseGitHubDigest(digest string) (string, error) {
	if !strings.HasPrefix(digest, githubDigestPrefix) {
		return "", fmt.Errorf("selfupdate: github digest %q is not sha256:<hex>: %w", digest, ErrIntegrity)
	}
	return parseHexDigest(digest[len(githubDigestPrefix):])
}

func parseHexDigest(digest string) (string, error) {
	if len(digest) != sha256HexLen {
		return "", fmt.Errorf("selfupdate: digest must be %d hex characters: %w", sha256HexLen, ErrIntegrity)
	}
	for i := 0; i < len(digest); i++ {
		c := digest[i]
		if !isHex(c) {
			return "", fmt.Errorf("selfupdate: digest must be %d hex characters: %w", sha256HexLen, ErrIntegrity)
		}
	}
	return strings.ToLower(digest), nil
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func parseSHA256SUMS(data []byte) (map[string]string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024), maxChecksumLine)
	entries := make(map[string]string)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimRight(scanner.Text(), "\r")
		if err := parseChecksumLine(line, entries); err != nil {
			return nil, fmt.Errorf("selfupdate: SHA256SUMS line %d: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("selfupdate: SHA256SUMS is malformed: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("selfupdate: SHA256SUMS has no entries: %w", ErrIntegrity)
	}
	return entries, nil
}

func parseChecksumLine(line string, entries map[string]string) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}
	digest, name, err := splitChecksumFields(line)
	if err != nil {
		return err
	}
	normalized, err := parseHexDigest(digest)
	if err != nil {
		return err
	}
	if err := validateChecksumName(name); err != nil {
		return err
	}
	if _, dup := entries[name]; dup {
		return fmt.Errorf("duplicate filename %q: %w", name, ErrIntegrity)
	}
	entries[name] = normalized
	return nil
}

func splitChecksumFields(line string) (digest, name string, err error) {
	fields := strings.FieldsFunc(line, unicode.IsSpace)
	if len(fields) != 2 {
		return "", "", fmt.Errorf("want exactly two fields: %w", ErrIntegrity)
	}
	digest = fields[0]
	name = strings.TrimPrefix(fields[1], "*")
	if name == "" {
		return "", "", fmt.Errorf("missing filename: %w", ErrIntegrity)
	}
	if strings.ContainsRune(name, '*') {
		return "", "", fmt.Errorf("filename %q has extra binary marker: %w", name, ErrIntegrity)
	}
	return digest, name, nil
}

func validateChecksumName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("filename %q is not a basename: %w", name, ErrIntegrity)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("filename %q is not a basename: %w", name, ErrIntegrity)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("filename %q is not a basename: %w", name, ErrIntegrity)
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("filename %q is not a basename: %w", name, ErrIntegrity)
	}
	return nil
}

func checksumFor(entries map[string]string, manifestName string) (string, error) {
	if err := validateChecksumName(manifestName); err != nil {
		return "", err
	}
	digest, ok := entries[manifestName]
	if !ok {
		return "", fmt.Errorf("selfupdate: SHA256SUMS has no entry for %s: %w", manifestName, ErrIntegrity)
	}
	return digest, nil
}
