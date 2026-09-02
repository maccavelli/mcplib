package selfupdate

import (
	"errors"
	"strings"
	"testing"
)

const testDigest = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestParseSHA256SUMS(t *testing.T) {
	t.Run("gnu text and binary", func(t *testing.T) {
		body := testDigest + "  demo-linux-amd64\n" +
			strings.ToUpper(testDigest) + " *demo-windows-amd64.exe\n"
		got, err := parseSHA256SUMS([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if got["demo-linux-amd64"] != testDigest {
			t.Fatalf("text entry = %q", got["demo-linux-amd64"])
		}
		if got["demo-windows-amd64.exe"] != testDigest {
			t.Fatalf("binary entry = %q", got["demo-windows-amd64.exe"])
		}
	})

	t.Run("crlf blank and comment", func(t *testing.T) {
		body := "# generated\r\n\r\n" + testDigest + "  demo-linux-amd64\r\n"
		got, err := parseSHA256SUMS([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got["demo-linux-amd64"] != testDigest {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("duplicate entries", func(t *testing.T) {
		body := testDigest + "  demo-linux-amd64\n" + testDigest + "  demo-linux-amd64\n"
		if _, err := parseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted duplicate filename")
		}
	})

	t.Run("malformed hex", func(t *testing.T) {
		body := strings.Repeat("zz", 32) + "  demo-linux-amd64\n"
		if _, err := parseSHA256SUMS([]byte(body)); !errors.Is(err, ErrIntegrity) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("extra fields", func(t *testing.T) {
		body := testDigest + "  demo-linux-amd64 extra\n"
		if _, err := parseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted extra fields")
		}
	})

	t.Run("absolute unix path", func(t *testing.T) {
		body := testDigest + "  /tmp/demo-linux-amd64\n"
		if _, err := parseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted absolute path")
		}
	})

	t.Run("traversal", func(t *testing.T) {
		body := testDigest + "  ../demo-linux-amd64\n"
		if _, err := parseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted traversal")
		}
	})

	t.Run("nested path", func(t *testing.T) {
		body := testDigest + "  dir/demo-linux-amd64\n"
		if _, err := parseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted nested path")
		}
	})

	t.Run("overlong line", func(t *testing.T) {
		body := testDigest + "  " + strings.Repeat("a", maxChecksumLine) + "\n"
		if _, err := parseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted overlong line")
		}
	})

	t.Run("lookup", func(t *testing.T) {
		body := testDigest + "  demo-linux-amd64\n" +
			strings.Repeat("cd", 32) + "  demo-darwin-arm64\n"
		entries, err := parseSHA256SUMS([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		got, err := checksumFor(entries, "demo-linux-amd64")
		if err != nil {
			t.Fatal(err)
		}
		if got != testDigest {
			t.Fatalf("lookup = %q", got)
		}
		if _, err := checksumFor(entries, "missing"); err == nil {
			t.Fatal("lookup succeeded for missing name")
		}
	})
}

func TestParseGitHubDigest(t *testing.T) {
	got, err := parseGitHubDigest("sha256:" + strings.ToUpper(testDigest))
	if err != nil {
		t.Fatal(err)
	}
	if got != testDigest {
		t.Fatalf("normalized = %q", got)
	}
	if _, err := parseGitHubDigest("SHA256:" + testDigest); err == nil {
		t.Fatal("accepted uppercase prefix")
	}
	if _, err := parseGitHubDigest(testDigest); err == nil {
		t.Fatal("accepted bare hex")
	}
}
