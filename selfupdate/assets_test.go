package selfupdate

import (
	"errors"
	"strings"
	"testing"
)

func testPlatforms() []Platform {
	return []Platform{
		{OS: "linux", Arch: "amd64"},
		{OS: "linux", Arch: "arm64"},
		{OS: "darwin", Arch: "amd64"},
		{OS: "darwin", Arch: "arm64"},
		{OS: "windows", Arch: "amd64"},
		{OS: "windows", Arch: "arm64"},
	}
}

func TestNewExactAssetSelector(t *testing.T) {
	if _, err := NewExactAssetSelector(nil); err == nil {
		t.Fatal("accepted empty platform list")
	}
	if _, err := NewExactAssetSelector([]Platform{{OS: "Linux", Arch: "amd64"}}); err == nil {
		t.Fatal("accepted uppercase OS")
	}
	if _, err := NewExactAssetSelector([]Platform{{OS: "linux", Arch: "AMD64"}}); err == nil {
		t.Fatal("accepted uppercase architecture")
	}
	if _, err := NewExactAssetSelector([]Platform{{OS: "linux-gnu", Arch: "amd64"}}); err == nil {
		t.Fatal("accepted hyphenated OS")
	}
	dups := []Platform{{OS: "linux", Arch: "amd64"}, {OS: "linux", Arch: "amd64"}}
	if _, err := NewExactAssetSelector(dups); err == nil {
		t.Fatal("accepted duplicate platform")
	}
	sel, err := NewExactAssetSelector(testPlatforms())
	if err != nil {
		t.Fatal(err)
	}
	if sel == nil {
		t.Fatal("nil selector")
	}
}

func TestExactAssetNameWindowsExtension(t *testing.T) {
	if got := exactAssetName("demo", Platform{OS: "linux", Arch: "amd64"}); got != "demo-linux-amd64" {
		t.Fatalf("linux = %q", got)
	}
	if got := exactAssetName("demo", Platform{OS: "windows", Arch: "amd64"}); got != "demo-windows-amd64.exe" {
		t.Fatalf("windows amd64 = %q", got)
	}
	if got := exactAssetName("demo", Platform{OS: "windows", Arch: "arm64"}); got != "demo-windows-arm64.exe" {
		t.Fatalf("windows arm64 = %q", got)
	}
	if got := exactAssetName("mcp-server-recall", Platform{OS: "darwin", Arch: "arm64"}); got != "mcp-server-recall-darwin-arm64" {
		t.Fatalf("hyphenated product = %q", got)
	}
}

func TestExactAssetSelectorSelect(t *testing.T) {
	sel, err := NewExactAssetSelector(testPlatforms())
	if err != nil {
		t.Fatal(err)
	}
	hex := strings.Repeat("ab", 32)
	digest := "sha256:" + hex
	release := Release{
		Tag: "v1.2.3",
		Assets: []Asset{
			{ID: 1, Name: "demo-linux-amd64", Digest: digest},
			{ID: 2, Name: "demo-linux-amd64-musl"},
			{ID: 3, Name: "demo-windows-amd64.exe", Digest: digest},
			{ID: 4, Name: "SHA256SUMS", Digest: digest},
			{ID: 5, Name: "SHA256SUMS-v1.2.3"},
			{ID: 6, Name: "mcp-server-recall-darwin-arm64"},
		},
	}

	t.Run("linux exact ignores prefix decoy", func(t *testing.T) {
		got, err := sel.Select(release, "demo", Platform{OS: "linux", Arch: "amd64"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Binary.Name != "demo-linux-amd64" || got.Binary.ID != 1 {
			t.Fatalf("binary = %+v", got.Binary)
		}
		if got.Manifest.Name != "SHA256SUMS" || got.ManifestName != "demo-linux-amd64" {
			t.Fatalf("manifest = %+v name=%s", got.Manifest, got.ManifestName)
		}
	})

	t.Run("windows extension placement", func(t *testing.T) {
		got, err := sel.Select(release, "demo", Platform{OS: "windows", Arch: "amd64"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Binary.Name != "demo-windows-amd64.exe" {
			t.Fatalf("binary = %q", got.Binary.Name)
		}
		if got.ManifestName != "demo-windows-amd64.exe" {
			t.Fatalf("manifest name = %q", got.ManifestName)
		}
	})

	t.Run("hyphenated product", func(t *testing.T) {
		got, err := sel.Select(release, "mcp-server-recall", Platform{OS: "darwin", Arch: "arm64"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Binary.Name != "mcp-server-recall-darwin-arm64" {
			t.Fatalf("binary = %q", got.Binary.Name)
		}
	})

	t.Run("duplicate exact assets", func(t *testing.T) {
		dup := release
		dup.Assets = append([]Asset{}, release.Assets...)
		dup.Assets = append(dup.Assets, Asset{ID: 99, Name: "demo-linux-amd64"})
		if _, err := sel.Select(dup, "demo", Platform{OS: "linux", Arch: "amd64"}); err == nil {
			t.Fatal("accepted duplicate exact asset")
		}
	})

	t.Run("missing manifest", func(t *testing.T) {
		missing := Release{Tag: "v1.0.0", Assets: []Asset{{Name: "demo-linux-amd64"}}}
		if _, err := sel.Select(missing, "demo", Platform{OS: "linux", Arch: "amd64"}); err == nil {
			t.Fatal("accepted missing SHA256SUMS")
		}
	})

	t.Run("duplicate manifest", func(t *testing.T) {
		dup := release
		dup.Assets = append([]Asset{}, release.Assets...)
		dup.Assets = append(dup.Assets, Asset{ID: 100, Name: "SHA256SUMS"})
		if _, err := sel.Select(dup, "demo", Platform{OS: "linux", Arch: "amd64"}); err == nil {
			t.Fatal("accepted duplicate SHA256SUMS")
		}
	})

	t.Run("unsupported platform", func(t *testing.T) {
		_, err := sel.Select(release, "demo", Platform{OS: "plan9", Arch: "amd64"})
		if !errors.Is(err, ErrUnsupportedPlatform) {
			t.Fatalf("err = %v, want ErrUnsupportedPlatform", err)
		}
	})

	t.Run("missing platform asset", func(t *testing.T) {
		if _, err := sel.Select(release, "demo", Platform{OS: "darwin", Arch: "amd64"}); err == nil {
			t.Fatal("accepted missing darwin/amd64 asset")
		}
	})

	t.Run("invalid product basename", func(t *testing.T) {
		if _, err := sel.Select(release, "../demo", Platform{OS: "linux", Arch: "amd64"}); err == nil {
			t.Fatal("accepted invalid product")
		}
	})

	t.Run("digest syntax", func(t *testing.T) {
		bad := release
		bad.Assets = []Asset{
			{Name: "demo-linux-amd64", Digest: "md5:deadbeef"},
			{Name: "SHA256SUMS"},
		}
		if _, err := sel.Select(bad, "demo", Platform{OS: "linux", Arch: "amd64"}); err == nil {
			t.Fatal("accepted malformed github digest")
		}
		short := release
		short.Assets = []Asset{
			{Name: "demo-linux-amd64", Digest: "sha256:abcd"},
			{Name: "SHA256SUMS"},
		}
		if _, err := sel.Select(short, "demo", Platform{OS: "linux", Arch: "amd64"}); err == nil {
			t.Fatal("accepted short github digest")
		}
	})
}
