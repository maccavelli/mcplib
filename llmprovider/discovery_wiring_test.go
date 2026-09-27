package llmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// deadlineTransport records, for the first request of each method and path,
// how long its context had left (-1 without a deadline), then forwards it.
type deadlineTransport struct {
	mu   sync.Mutex
	seen map[string]time.Duration
}

func (d *deadlineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	left := time.Duration(-1)
	if dl, ok := r.Context().Deadline(); ok {
		left = time.Until(dl)
	}
	key := r.Method + " " + r.URL.Path
	d.mu.Lock()
	if d.seen == nil {
		d.seen = map[string]time.Duration{}
	}
	if _, ok := d.seen[key]; !ok {
		d.seen[key] = left
	}
	d.mu.Unlock()
	return http.DefaultTransport.RoundTrip(r)
}

func (d *deadlineTransport) left(key string) (time.Duration, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.seen[key]
	return v, ok
}

// discoverer is the DiscoverModels half of a provider.
type discoverer interface {
	DiscoverModels(context.Context) ([]string, error)
}

// TestDiscoverModels_ListingBounded pins MADR 0013 A5: every DiscoverModels
// listing runs under the 10 s bound ListModelCatalogWithSource applies, so a
// slow listing or metadata host cannot hold discovery past it.
func TestDiscoverModels_ListingBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	for _, tc := range []struct {
		name, listing string
		build         func(opts ...ProviderOption) (discoverer, error)
	}{
		{"claude", "GET /v1/models", func(o ...ProviderOption) (discoverer, error) { return NewClaude("k", "m", o...) }},
		{"gemini", "GET /models", func(o ...ProviderOption) (discoverer, error) {
			return NewGemini(context.Background(), "k", "m", o...)
		}},
		{"ollama", "GET /api/tags", func(o ...ProviderOption) (discoverer, error) { return NewOllama("", "m", o...) }},
		{"opencode", "GET /models", func(o ...ProviderOption) (discoverer, error) {
			return NewOpencode(ProviderOpencodeZen, "k", "glm-5.3-flash", o...)
		}},
		{"huggingface", "GET /models", func(o ...ProviderOption) (discoverer, error) { return NewHuggingFace("k", "m", o...) }},
		{"kilo", "GET /models", func(o ...ProviderOption) (discoverer, error) { return NewKilo("k", "m", o...) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &deadlineTransport{}
			p, err := tc.build(WithHTTPClient(&http.Client{Transport: rec}), WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			_, _ = p.DiscoverModels(context.Background())
			left, ok := rec.left(tc.listing)
			if !ok {
				t.Fatalf("no %s request was made", tc.listing)
			}
			switch {
			case left < 0:
				t.Errorf("%s ran without a deadline, want one within 10s", tc.listing)
			case left > 10*time.Second:
				t.Errorf("%s ran with %v left, want at most 10s", tc.listing, left.Round(time.Second))
			}
		})
	}
}

// getOnly answers GET with body and anything else (the health probes) with 500.
func getOnly(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDiscoverModels_HonoursRankingOptions pins MADR 0013 A4: with every
// probe failing, DiscoverModels returns exactly what ListModelCatalog
// recommends for the same profile and metadata URL, and never reads the
// environment's metadata URL when an option names one.
func TestDiscoverModels_HonoursRankingOptions(t *testing.T) {
	pinRankingNow(t, refNow)
	enableModelMetadata(t)
	envMeta, envHits := metadataServer(t, http.StatusInternalServerError, "")
	t.Setenv(envModelMetadataURL, envMeta.URL)

	kiloListing := `{"data":[` + strings.Join([]string{
		kiloRankEntry("a/flash-lite", "A Flash Lite", "0.0000001", "0.0000004", 20, true, ""),
		kiloRankEntry("b/flash", "B Flash", "0.0000003", "0.0000012", 10, true, `"terminalBench":{"overallScore":0.75}`),
		kiloRankEntry("c/pro", "C Pro", "0.000005", "0.000025", 40, true, `"terminalBench":{"overallScore":0.80}`),
		kiloRankEntry("d/mid", "D Mid", "0.000001", "0.000004", 60, true, `"preferredIndex":2`),
		kiloRankEntry("e/mini", "E Mini", "0.0000002", "0.0000008", 90, true, ""),
		kiloRankEntry("f/large", "F Large", "0.000002", "0.000008", 30, true, ""),
		kiloRankEntry("kilo-auto/efficient", "Auto Efficient", "-1", "-1", -1, true, `"preferredIndex":0`),
	}, ",") + `]}`
	zenIDs := []string{"glm-5.3-flash", "qwen3.8-flash", "kimi-k2.6", "gpt-6-luna", "hy3", "mimo-v2.6-flash", "deepseek-v4-pro"}
	zenMeta := `{` + mdSection("opencode",
		mdModel("glm-5.3-flash", "glm-flash", true, 0.15, 0.5, "2026-08-26"),
		mdModel("qwen3.8-flash", "qwen-flash", true, 0.2, 0.8, "2026-08-20"),
		mdModel("kimi-k2.6", "kimi", true, 0.6, 2.5, "2026-07-01"),
		mdModel("gpt-6-luna", "gpt-luna", true, 0.1, 0.5, "2026-09-22"),
		mdModel("hy3", "hy", true, 0.3, 1.2, "2026-08-01"),
		mdModel("mimo-v2.6-flash", "mimo", true, 0.14, 0.28, "2026-09-22"),
		mdModel("deepseek-v4-pro", "deepseek-pro", true, 2, 8, "2026-09-10"),
	) + `}`

	for _, tc := range []struct {
		provider, listing, meta string
		build                   func(opts ...ProviderOption) (discoverer, error)
	}{
		{ProviderKilo, kiloListing, "", func(o ...ProviderOption) (discoverer, error) { return NewKilo("k", "m", o...) }},
		{ProviderHuggingFace, hfRankListing, hfRankMetadata, func(o ...ProviderOption) (discoverer, error) {
			return NewHuggingFace("k", "m", o...)
		}},
		{ProviderOpencodeZen, zenStyleListing(zenIDs...), zenMeta, func(o ...ProviderOption) (discoverer, error) {
			return NewOpencode(ProviderOpencodeZen, "k", "glm-5.3-flash", o...)
		}},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			listing := getOnly(t, tc.listing)
			opts := []ProviderOption{WithBaseURL(listing.URL)}
			if tc.meta != "" {
				meta, _ := metadataServer(t, http.StatusOK, tc.meta)
				opts = append(opts, WithModelMetadataURL(meta.URL))
			}
			utility := listCatalog(context.Background(), t, tc.provider, opts...).Recommended
			opts = append(opts, WithModelProfile(ProfileCapable))
			want := listCatalog(context.Background(), t, tc.provider, opts...).Recommended
			if slices.Equal(want, utility) {
				t.Fatalf("fixture does not separate the profiles: both %v", want)
			}
			resetModelMetadataCache()
			p, err := tc.build(opts...)
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			got, err := p.DiscoverModels(context.Background())
			if err != nil {
				t.Fatalf("DiscoverModels: %v", err)
			}
			assertRanked(t, got, want)
			if n := envHits.Load(); n != 0 {
				t.Errorf("DiscoverModels fetched the environment's metadata URL %d times, want 0", n)
			}
		})
	}
}
