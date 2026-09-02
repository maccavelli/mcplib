package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testGitHubLimits() Limits {
	return Limits{
		ReleaseJSON: 4 << 10,
		ErrorBody:   256,
		Manifest:    1024,
		Executable:  4096,
	}
}

type gitHubEnv struct {
	src    *GitHubSource
	server *httptest.Server
	client *http.Client
}

func newGitHubEnv(t *testing.T, handler http.HandlerFunc, token string) *gitHubEnv {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	src, err := NewGitHubSource(GitHubOptions{
		Repository: Repository{Owner: "maccavelli", Name: "demo"},
		Client:     srv.Client(),
		APIBaseURL: base,
		UserAgent:  "demo/v1.0.0",
		Token:      token,
		Limits:     testGitHubLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &gitHubEnv{src: src, server: srv, client: srv.Client()}
}

func shaHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func validReleaseJSON(t *testing.T, tag string, assets ...githubAssetJSON) []byte {
	t.Helper()
	raw := githubReleaseJSON{
		ID:        11,
		TagName:   tag,
		HTMLURL:   "https://github.com/maccavelli/demo/releases/" + tag,
		Immutable: true,
		Assets:    assets,
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNewGitHubSource(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	base, _ := url.Parse("https://api.github.com")
	opts := GitHubOptions{
		Repository: Repository{Owner: "maccavelli", Name: "demo"},
		Client:     client,
		APIBaseURL: base,
		UserAgent:  "demo/v1.0.0",
		Limits:     DefaultLimits(),
	}
	src, err := NewGitHubSource(opts)
	if err != nil {
		t.Fatal(err)
	}
	if src.client == client {
		t.Fatal("NewGitHubSource must clone the client")
	}
	client.Timeout = 3 * time.Second
	if src.client.Timeout == client.Timeout {
		t.Fatal("cloned client mutated with caller")
	}
	if _, err := NewGitHubSource(GitHubOptions{Repository: opts.Repository, UserAgent: opts.UserAgent, Limits: opts.Limits}); err == nil {
		t.Fatal("accepted nil client")
	}
	bad := opts
	bad.UserAgent = "demo\r\nX-Injected: 1"
	if _, err := NewGitHubSource(bad); err == nil {
		t.Fatal("accepted control characters in user-agent")
	}
	slash := opts
	slash.Repository.Owner = "mac/cavelli"
	if _, err := NewGitHubSource(slash); err == nil {
		t.Fatal("accepted path separator in owner")
	}
	httpRemote, _ := url.Parse("http://example.com")
	insecure := opts
	insecure.APIBaseURL = httpRemote
	if _, err := NewGitHubSource(insecure); err == nil {
		t.Fatal("accepted non-loopback http")
	}
}

func TestGitHubTokenOrder(t *testing.T) {
	orig := lookupEnv
	t.Cleanup(func() { lookupEnv = orig })
	opts := GitHubOptions{
		Repository: Repository{Owner: "maccavelli", Name: "demo"},
		Client:     &http.Client{Timeout: time.Second},
		UserAgent:  "demo/v1.0.0",
		Limits:     DefaultLimits(),
	}
	lookupEnv = func(k string) string {
		switch k {
		case "GH_TOKEN":
			return "gh-from-env"
		case "GITHUB_TOKEN":
			return "github-from-env"
		default:
			return ""
		}
	}
	src, err := NewGitHubSource(opts)
	if err != nil {
		t.Fatal(err)
	}
	if src.token != "gh-from-env" {
		t.Fatalf("token = %q, want GH_TOKEN", src.token)
	}
	lookupEnv = func(k string) string {
		if k == "GITHUB_TOKEN" {
			return "github-from-env"
		}
		return ""
	}
	src, err = NewGitHubSource(opts)
	if err != nil {
		t.Fatal(err)
	}
	if src.token != "github-from-env" {
		t.Fatalf("token = %q, want GITHUB_TOKEN", src.token)
	}
	explicit := opts
	explicit.Token = "explicit"
	src, err = NewGitHubSource(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if src.token != "explicit" {
		t.Fatalf("token = %q, want explicit", src.token)
	}
}

func TestGitHubLatestRequiredHeadersAndReuse(t *testing.T) {
	var n int
	var last http.Header
	body := validReleaseJSON(t, "v1.2.3", githubAssetJSON{
		ID: 7, Name: "demo-linux-amd64", State: "uploaded", Size: 5,
		Digest: "sha256:" + shaHex([]byte("hello")),
	})
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		last = r.Header.Clone()
		if r.URL.Path != "/repos/maccavelli/demo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}, "secret-token")
	if _, err := env.src.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.src.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("requests = %d, want 2 on one client", n)
	}
	if got := last.Get("Accept"); got != gitHubAcceptJSON {
		t.Fatalf("Accept = %q", got)
	}
	if got := last.Get("X-GitHub-Api-Version"); got != gitHubAPIVersion {
		t.Fatalf("API version = %q", got)
	}
	if got := last.Get("User-Agent"); got != "demo/v1.0.0" {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := last.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestGitHubByTagAndRejects(t *testing.T) {
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/tags/v1.2.3"):
			_, _ = w.Write(validReleaseJSON(t, "v1.2.3", githubAssetJSON{
				ID: 1, Name: "demo-linux-amd64", State: "uploaded", Size: 1,
			}))
		case strings.HasSuffix(r.URL.Path, "/releases/tags/v0.0.1"):
			raw := githubReleaseJSON{ID: 2, TagName: "v0.0.1", Immutable: true, Draft: true}
			b, _ := json.Marshal(raw)
			_, _ = w.Write(b)
		case strings.HasSuffix(r.URL.Path, "/releases/tags/v0.0.2"):
			raw := githubReleaseJSON{ID: 3, TagName: "v0.0.2", Immutable: true, Prerelease: true}
			b, _ := json.Marshal(raw)
			_, _ = w.Write(b)
		case strings.HasSuffix(r.URL.Path, "/releases/tags/v0.0.3"):
			raw := githubReleaseJSON{ID: 4, TagName: "v0.0.3", Immutable: false}
			b, _ := json.Marshal(raw)
			_, _ = w.Write(b)
		case strings.HasSuffix(r.URL.Path, "/releases/tags/v0.0.4"):
			_, _ = w.Write([]byte(`{"id":5,"tag_name":"v0.0.4","immutable":true}{"id":6}`))
		default:
			http.NotFound(w, r)
		}
	}, "")
	if _, err := env.src.ByTag(context.Background(), "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.src.ByTag(context.Background(), "v0.0.1"); err == nil {
		t.Fatal("accepted draft")
	}
	if _, err := env.src.ByTag(context.Background(), "v0.0.2"); err == nil {
		t.Fatal("accepted prerelease")
	}
	_, err := env.src.ByTag(context.Background(), "v0.0.3")
	if !errors.Is(err, ErrMutableRelease) {
		t.Fatalf("mutable err = %v", err)
	}
	if _, err := env.src.ByTag(context.Background(), "v0.0.4"); err == nil {
		t.Fatal("accepted trailing json")
	}
	if _, err := env.src.ByTag(context.Background(), "../v1.2.3"); err == nil {
		t.Fatal("accepted traversal tag")
	}
}

func TestGitHubJSONOverflowAndMalformed(t *testing.T) {
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "latest") {
			_, _ = w.Write([]byte(strings.Repeat("a", 8<<10)))
			return
		}
		_, _ = w.Write([]byte(`{"id":`))
	}, "")
	if _, err := env.src.Latest(context.Background()); err == nil {
		t.Fatal("accepted oversized json")
	}
	if _, err := env.src.ByTag(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("accepted malformed json")
	}
}

func TestGitHubRateLimit(t *testing.T) {
	fixed := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", fixed.Add(time.Minute).Unix()))
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("slow down"))
	}, "")
	env.src.now = func() time.Time { return fixed }
	_, err := env.src.Latest(context.Background())
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatal("missing ErrRateLimited")
	}
	if rl.StatusCode != 429 || rl.Remaining != 0 || rl.RetryAfter != 12*time.Second {
		t.Fatalf("%+v", rl)
	}
	if !rl.Reset.Equal(fixed.Add(time.Minute)) {
		t.Fatalf("reset = %s", rl.Reset)
	}
}

func TestGitHubRateLimitHTTPDateAndMalformedHeaders(t *testing.T) {
	fixed := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", fixed.Add(30*time.Second).Format(http.TimeFormat))
		w.Header().Set("X-RateLimit-Remaining", "nope")
		w.Header().Set("X-RateLimit-Reset", "nope")
		w.WriteHeader(http.StatusForbidden)
	}, "")
	env.src.now = func() time.Time { return fixed }
	_, err := env.src.Latest(context.Background())
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v", err)
	}
	if rl.RetryAfter != 30*time.Second || rl.Remaining != 0 || !rl.Reset.IsZero() {
		t.Fatalf("%+v", rl)
	}
}

func TestGitHubSanitizedErrorDiagnostics(t *testing.T) {
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("nope\x1b[31msecret\nmore"))
	}, "")
	_, err := env.src.Latest(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "\x1b") || strings.Contains(err.Error(), "\n") {
		t.Fatalf("unsanitized error: %q", err.Error())
	}
}

func TestGitHubOpenAssetRedirectTokenPolicy(t *testing.T) {
	payload := []byte("hello")
	var foreignAuth string
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignAuth = r.Header.Get("Authorization")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(foreign.Close)

	var sameOriginAuth string
	var api *httptest.Server
	api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/maccavelli/demo/releases/assets/1":
			http.Redirect(w, r, foreign.URL+"/blob", http.StatusFound)
		case "/repos/maccavelli/demo/releases/assets/2":
			http.Redirect(w, r, api.URL+"/blob", http.StatusFound)
		case "/blob":
			sameOriginAuth = r.Header.Get("Authorization")
			_, _ = w.Write(payload)
		case "/repos/maccavelli/demo/releases/assets/3":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	base, _ := url.Parse(api.URL)
	src, err := NewGitHubSource(GitHubOptions{
		Repository: Repository{Owner: "maccavelli", Name: "demo"},
		Client:     api.Client(),
		APIBaseURL: base,
		UserAgent:  "demo/v1.0.0",
		Token:      "secret-token",
		Limits:     testGitHubLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rel := Release{ID: 11, Tag: "v1.0.0"}
	foreignAsset := Asset{ID: 1, Name: "demo-linux-amd64", State: "uploaded", Size: 5}
	rc, err := src.OpenAsset(context.Background(), rel, foreignAsset)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "hello" {
		t.Fatalf("foreign body = %q", got)
	}
	if foreignAuth != "" {
		t.Fatalf("authorization leaked to foreign host: %q", foreignAuth)
	}

	same := Asset{ID: 2, Name: "demo-linux-amd64", State: "uploaded", Size: 5}
	rc, err = src.OpenAsset(context.Background(), rel, same)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(rc)
	_ = rc.Close()
	if sameOriginAuth != "Bearer secret-token" {
		t.Fatalf("same-origin auth = %q", sameOriginAuth)
	}

	direct := Asset{ID: 3, Name: "demo-linux-amd64", State: "uploaded", Size: 5}
	rc, err = src.OpenAsset(context.Background(), rel, direct)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "hello" {
		t.Fatalf("200 body = %q", got)
	}
}

func TestGitHubCancellationAndTimeout(t *testing.T) {
	started := make(chan struct{})
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write(validReleaseJSON(t, "v1.0.0"))
	}, "")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	if _, err := env.src.Latest(ctx); err == nil {
		t.Fatal("expected cancellation")
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write(validReleaseJSON(t, "v1.0.0"))
	}))
	t.Cleanup(slow.Close)
	base, _ := url.Parse(slow.URL)
	client := slow.Client()
	client.Timeout = 50 * time.Millisecond
	src, err := NewGitHubSource(GitHubOptions{
		Repository: Repository{Owner: "maccavelli", Name: "demo"},
		Client:     client,
		APIBaseURL: base,
		UserAgent:  "demo/v1.0.0",
		Limits:     testGitHubLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Latest(context.Background()); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestGitHubIgnoresBrowserDownloadURL(t *testing.T) {
	var sawBrowser bool
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "browser") {
			sawBrowser = true
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/assets/9") {
			_, _ = w.Write([]byte("hello"))
			return
		}
		raw := map[string]any{
			"id": 11, "tag_name": "v1.0.0", "immutable": true,
			"assets": []map[string]any{{
				"id": 9, "name": "demo-linux-amd64", "state": "uploaded", "size": 5,
				"browser_download_url": "http://127.0.0.1/browser/demo-linux-amd64",
			}},
		}
		_ = json.NewEncoder(w).Encode(raw)
	}, "secret-token")
	rel, err := env.src.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rc, err := env.src.OpenAsset(context.Background(), rel, rel.Assets[0])
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if sawBrowser {
		t.Fatal("fetched metadata-provided browser URL")
	}
}
