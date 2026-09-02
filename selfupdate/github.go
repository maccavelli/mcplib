package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	gitHubAPIVersion  = "2026-03-10"
	gitHubAcceptJSON  = "application/vnd.github+json"
	gitHubAcceptAsset = "application/octet-stream"
	defaultAPIOrigin  = "https://api.github.com"
	maxRedirects      = 10
)

var (
	lookupEnv = os.Getenv
	timeNow   = time.Now
)

// GitHubSource is the canonical GitHub Releases implementation of ReleaseSource.
type GitHubSource struct {
	repo      Repository
	client    *http.Client
	apiBase   *url.URL
	userAgent string
	token     string
	limits    Limits
	now       func() time.Time
}

// NewGitHubSource validates options, clones the supplied client, and resolves
// the token once. It never mutates the caller's client or URL.
func NewGitHubSource(opts GitHubOptions) (*GitHubSource, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("selfupdate: github client is required")
	}
	if err := opts.Limits.valid(); err != nil {
		return nil, err
	}
	if err := validateGitHubName("owner", opts.Repository.Owner); err != nil {
		return nil, err
	}
	if err := validateGitHubName("repository", opts.Repository.Name); err != nil {
		return nil, err
	}
	if err := validateUserAgent(opts.UserAgent); err != nil {
		return nil, err
	}
	base, err := normalizeAPIBase(opts.APIBaseURL)
	if err != nil {
		return nil, err
	}
	cloned := *opts.Client
	src := &GitHubSource{
		repo:      opts.Repository,
		client:    &cloned,
		apiBase:   base,
		userAgent: opts.UserAgent,
		token:     resolveToken(opts.Token),
		limits:    opts.Limits,
		now:       timeNow,
	}
	src.client.CheckRedirect = src.checkRedirect
	return src, nil
}

func validateGitHubName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("selfupdate: github %s is required", kind)
	}
	if strings.ContainsAny(name, `/\:?`) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("selfupdate: github %s contains illegal characters", kind)
	}
	return nil
}

func validateUserAgent(ua string) error {
	if strings.TrimSpace(ua) == "" {
		return fmt.Errorf("selfupdate: user-agent is required")
	}
	if strings.IndexFunc(ua, unicode.IsControl) >= 0 {
		return fmt.Errorf("selfupdate: user-agent contains illegal characters")
	}
	return nil
}

func resolveToken(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := lookupEnv("GH_TOKEN"); v != "" {
		return v
	}
	return lookupEnv("GITHUB_TOKEN")
}

func normalizeAPIBase(raw *url.URL) (*url.URL, error) {
	var base *url.URL
	if raw == nil {
		parsed, err := url.Parse(defaultAPIOrigin)
		if err != nil {
			return nil, err
		}
		base = parsed
	} else {
		cloned := *raw
		base = &cloned
	}
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("selfupdate: github API base must not include user, query, or fragment")
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("selfupdate: github API base is incomplete")
	}
	if !isLoopbackHost(base.Hostname()) && !strings.EqualFold(base.Scheme, "https") {
		return nil, fmt.Errorf("selfupdate: github API base must be https")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	return base, nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *GitHubSource) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("selfupdate: too many redirects")
	}
	if !sameOrigin(req.URL, s.apiBase) {
		req.Header.Del("Authorization")
	}
	return nil
}

func sameOrigin(u, base *url.URL) bool {
	return strings.EqualFold(u.Scheme, base.Scheme) && strings.EqualFold(u.Host, base.Host)
}

func (s *GitHubSource) apiURL(elem ...string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(s.apiBase.String(), "/"))
	for _, e := range elem {
		b.WriteByte('/')
		b.WriteString(url.PathEscape(e))
	}
	return b.String()
}

// Latest implements ReleaseSource.
func (s *GitHubSource) Latest(ctx context.Context) (Release, error) {
	return s.getRelease(ctx, s.apiURL("repos", s.repo.Owner, s.repo.Name, "releases", "latest"))
}

// ByTag implements ReleaseSource.
func (s *GitHubSource) ByTag(ctx context.Context, tag string) (Release, error) {
	if tag == "" || strings.ContainsAny(tag, `/\:`) || strings.IndexFunc(tag, unicode.IsControl) >= 0 {
		return Release{}, fmt.Errorf("selfupdate: invalid release tag %q", tag)
	}
	return s.getRelease(ctx, s.apiURL("repos", s.repo.Owner, s.repo.Name, "releases", "tags", tag))
}

type githubReleaseJSON struct {
	ID         int64             `json:"id"`
	TagName    string            `json:"tag_name"`
	HTMLURL    string            `json:"html_url"`
	Draft      bool              `json:"draft"`
	Prerelease bool              `json:"prerelease"`
	Immutable  bool              `json:"immutable"`
	Assets     []githubAssetJSON `json:"assets"`
}

type githubAssetJSON struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

func (s *GitHubSource) getRelease(ctx context.Context, rawURL string) (rel Release, err error) {
	req, err := s.newRequest(ctx, http.MethodGet, rawURL, gitHubAcceptJSON)
	if err != nil {
		return Release{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	body, err := readBounded(resp.Body, s.limits.ReleaseJSON)
	if err != nil {
		return Release{}, err
	}
	if err := s.mapStatus(resp, body); err != nil {
		return Release{}, err
	}
	var raw githubReleaseJSON
	if err := decodeJSON(body, &raw); err != nil {
		return Release{}, err
	}
	rel, err = mapRelease(raw, s.limits.Executable)
	if err != nil {
		return Release{}, err
	}
	if err := validateFetchedRelease(rel); err != nil {
		return Release{}, err
	}
	return rel, nil
}

func mapRelease(raw githubReleaseJSON, maxSize int64) (Release, error) {
	rel := Release{
		ID:         raw.ID,
		Tag:        raw.TagName,
		URL:        raw.HTMLURL,
		Draft:      raw.Draft,
		Prerelease: raw.Prerelease,
		Immutable:  raw.Immutable,
		Assets:     make([]Asset, 0, len(raw.Assets)),
	}
	for _, a := range raw.Assets {
		asset := Asset(a)
		if err := validateAssetMetadata(asset, maxSize); err != nil {
			return Release{}, err
		}
		rel.Assets = append(rel.Assets, asset)
	}
	return rel, nil
}

func validateFetchedRelease(rel Release) error {
	if rel.ID <= 0 || rel.Tag == "" {
		return fmt.Errorf("selfupdate: release metadata is incomplete")
	}
	if rel.Draft {
		return fmt.Errorf("selfupdate: release %s is a draft", rel.Tag)
	}
	if rel.Prerelease {
		return fmt.Errorf("selfupdate: release %s is a prerelease", rel.Tag)
	}
	if !rel.Immutable {
		return fmt.Errorf("selfupdate: release %s is not immutable: %w", rel.Tag, ErrMutableRelease)
	}
	return nil
}

func assetBelongsToRelease(rel Release, asset Asset) error {
	if len(rel.Assets) == 0 {
		return nil
	}
	for _, a := range rel.Assets {
		if a.ID == asset.ID {
			return nil
		}
	}
	return fmt.Errorf("selfupdate: asset %d is not part of release %s", asset.ID, rel.Tag)
}

func validateAssetMetadata(a Asset, maxSize int64) error {
	if a.ID <= 0 || a.Name == "" {
		return fmt.Errorf("selfupdate: asset metadata is incomplete")
	}
	if strings.ContainsAny(a.Name, `/\`) || strings.IndexFunc(a.Name, unicode.IsControl) >= 0 {
		return fmt.Errorf("selfupdate: asset name %q is not a basename", a.Name)
	}
	if a.State != "uploaded" {
		return fmt.Errorf("selfupdate: asset %s is not uploaded", a.Name)
	}
	if a.Size <= 0 {
		return fmt.Errorf("selfupdate: asset %s has non-positive size: %w", a.Name, ErrIntegrity)
	}
	if a.Size > maxSize {
		return fmt.Errorf("selfupdate: asset %s size %d exceeds limit %d: %w", a.Name, a.Size, maxSize, ErrIntegrity)
	}
	if a.Digest != "" {
		if _, err := parseGitHubDigest(a.Digest); err != nil {
			return err
		}
	}
	return nil
}

// OpenAsset implements ReleaseSource. The body is fetched from the asset API
// path derived from owner, repository, and asset ID.
func (s *GitHubSource) OpenAsset(ctx context.Context, rel Release, asset Asset) (io.ReadCloser, error) {
	if asset.ID <= 0 {
		return nil, fmt.Errorf("selfupdate: asset id is required")
	}
	if err := assetBelongsToRelease(rel, asset); err != nil {
		return nil, err
	}
	if err := validateAssetMetadata(asset, s.limits.Executable); err != nil {
		return nil, err
	}
	rawURL := s.apiURL("repos", s.repo.Owner, s.repo.Name, "releases", "assets", strconv.FormatInt(asset.ID, 10))
	req, err := s.newRequest(ctx, http.MethodGet, rawURL, gitHubAcceptAsset)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, readErr := readBounded(resp.Body, s.limits.ErrorBody)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, errors.Join(readErr, closeErr)
		}
		statusErr := s.mapStatus(resp, body)
		if closeErr != nil {
			return nil, errors.Join(statusErr, closeErr)
		}
		return nil, statusErr
	}
	return resp.Body, nil
}

func (s *GitHubSource) newRequest(ctx context.Context, method, rawURL, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", gitHubAPIVersion)
	req.Header.Set("User-Agent", s.userAgent)
	if s.token != "" && sameOrigin(req.URL, s.apiBase) {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	return req, nil
}

func (s *GitHubSource) mapStatus(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	if resp.StatusCode == http.StatusTooManyRequests || rateLimitedForbidden(resp) {
		return parseRateLimit(resp, s.now)
	}
	diag := sanitizeDiagnostic(string(body), s.limits.ErrorBody)
	return fmt.Errorf("selfupdate: github http %d: %s", resp.StatusCode, diag)
}

func rateLimitedForbidden(resp *http.Response) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	return resp.Header.Get("X-RateLimit-Remaining") == "0"
}

func parseRateLimit(resp *http.Response, now func() time.Time) error {
	if now == nil {
		now = timeNow
	}
	err := &RateLimitError{StatusCode: resp.StatusCode}
	if v := strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")); v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			err.Remaining = n
		}
	}
	if v := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")); v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			err.Reset = time.Unix(n, 0).UTC()
		}
	}
	if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
		if secs, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			if secs > 0 {
				err.RetryAfter = time.Duration(secs) * time.Second
			}
		} else if when, perr := http.ParseTime(v); perr == nil {
			d := when.Sub(now())
			if d > 0 {
				err.RetryAfter = d
			}
		}
	}
	return err
}
