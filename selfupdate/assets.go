package selfupdate

import (
	"fmt"
	"regexp"
)

const manifestAssetName = "SHA256SUMS"

var platformFieldRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]*$`)

type exactAssetSelector struct {
	platforms []Platform
	allowed   map[Platform]struct{}
}

// NewExactAssetSelector returns the canonical raw-binary selector. Each
// platform pair must be unique and use lowercase GOOS/GOARCH fields. The
// selector never interpolates OS or architecture strings that are not in
// this allow-list.
func NewExactAssetSelector(platforms []Platform) (AssetSelector, error) {
	if len(platforms) == 0 {
		return nil, fmt.Errorf("selfupdate: asset selector requires at least one platform")
	}
	allowed := make(map[Platform]struct{}, len(platforms))
	cloned := make([]Platform, 0, len(platforms))
	for _, p := range platforms {
		if err := validatePlatformFields(p); err != nil {
			return nil, err
		}
		if _, dup := allowed[p]; dup {
			return nil, fmt.Errorf("selfupdate: duplicate platform %s/%s", p.OS, p.Arch)
		}
		allowed[p] = struct{}{}
		cloned = append(cloned, p)
	}
	return &exactAssetSelector{platforms: cloned, allowed: allowed}, nil
}

func validatePlatformFields(p Platform) error {
	if !platformFieldRe.MatchString(p.OS) {
		return fmt.Errorf("selfupdate: invalid platform OS %q", p.OS)
	}
	if !platformFieldRe.MatchString(p.Arch) {
		return fmt.Errorf("selfupdate: invalid platform architecture %q", p.Arch)
	}
	return nil
}

func exactAssetName(product string, platform Platform) string {
	name := product + "-" + platform.OS + "-" + platform.Arch
	if platform.OS == "windows" {
		name += ".exe"
	}
	return name
}

// Select implements AssetSelector.
func (s *exactAssetSelector) Select(rel Release, product string, platform Platform) (Selection, error) {
	if err := validateProduct(product); err != nil {
		return Selection{}, err
	}
	if err := validatePlatformFields(platform); err != nil {
		return Selection{}, err
	}
	if _, ok := s.allowed[platform]; !ok {
		return Selection{}, fmt.Errorf("selfupdate: %s/%s is not in the product platform matrix: %w",
			platform.OS, platform.Arch, ErrUnsupportedPlatform)
	}
	want := exactAssetName(product, platform)
	var (
		binary    Asset
		manifest  Asset
		binaries  int
		manifests int
	)
	for _, asset := range rel.Assets {
		switch asset.Name {
		case want:
			binaries++
			if binaries > 1 {
				return Selection{}, fmt.Errorf("selfupdate: duplicate exact asset %q", want)
			}
			if err := validateAssetDigestSyntax(asset.Digest); err != nil {
				return Selection{}, err
			}
			binary = asset
		case manifestAssetName:
			manifests++
			if manifests > 1 {
				return Selection{}, fmt.Errorf("selfupdate: duplicate exact asset %q", manifestAssetName)
			}
			if err := validateAssetDigestSyntax(asset.Digest); err != nil {
				return Selection{}, err
			}
			manifest = asset
		}
	}
	if binaries == 0 {
		return Selection{}, fmt.Errorf("selfupdate: release %s has no exact asset %q", rel.Tag, want)
	}
	if manifests == 0 {
		return Selection{}, fmt.Errorf("selfupdate: release %s has no exact asset %q", rel.Tag, manifestAssetName)
	}
	return Selection{
		Binary:       binary,
		Manifest:     manifest,
		ManifestName: want,
	}, nil
}

func validateAssetDigestSyntax(digest string) error {
	if digest == "" {
		return nil
	}
	if _, err := parseGitHubDigest(digest); err != nil {
		return err
	}
	return nil
}
