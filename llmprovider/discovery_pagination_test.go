package llmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
)

// pagingServer answers each request with respond(query) and records every query.
type pagingServer struct {
	mu      sync.Mutex
	queries []url.Values
	srv     *httptest.Server
}

func newPagingServer(t *testing.T, respond func(q url.Values) (int, string)) *pagingServer {
	t.Helper()
	ps := &pagingServer{}
	ps.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ps.mu.Lock()
		ps.queries = append(ps.queries, q)
		ps.mu.Unlock()
		status, body := respond(q)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ps.srv.Close)
	return ps
}

func (ps *pagingServer) seen() []url.Values {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return slices.Clone(ps.queries)
}

func TestListModelCatalog_GeminiFollowsNextPageToken(t *testing.T) {
	ps := newPagingServer(t, func(q url.Values) (int, string) {
		if q.Get("pageToken") == "" {
			return http.StatusOK, `{"models":[{"name":"models/gemini-3.7-flash","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"p2"}`
		}
		return http.StatusOK, `{"models":[{"name":"models/gemini-2.5-flash","supportedGenerationMethods":["generateContent"]}]}`
	})
	cat, err := ListModelCatalog(context.Background(), ProviderGemini, "k", WithBaseURL(ps.srv.URL))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}
	if want := []string{"gemini-3.7-flash", "gemini-2.5-flash"}; !slices.Equal(cat.Usable, want) || !cat.Live {
		t.Errorf("catalog = %+v, want Live with Usable %v", cat, want)
	}
	seen := ps.seen()
	if len(seen) != 2 {
		t.Fatalf("requests = %d, want 2", len(seen))
	}
	for i, q := range seen {
		if q.Get("pageSize") != "1000" {
			t.Errorf("request %d pageSize = %q, want 1000", i, q.Get("pageSize"))
		}
	}
	if seen[1].Get("pageToken") != "p2" {
		t.Errorf("second request pageToken = %q, want p2", seen[1].Get("pageToken"))
	}
}

func TestListModelCatalog_ClaudeFollowsHasMore(t *testing.T) {
	ps := newPagingServer(t, func(q url.Values) (int, string) {
		if q.Get("after_id") == "" {
			return http.StatusOK, `{"data":[{"id":"claude-sonnet-5"}],"has_more":true,"last_id":"claude-sonnet-5"}`
		}
		return http.StatusOK, `{"data":[{"id":"claude-haiku-4-5"}],"has_more":false,"last_id":"claude-haiku-4-5"}`
	})
	cat, err := ListModelCatalog(context.Background(), ProviderClaude, "k", WithBaseURL(ps.srv.URL))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}
	if want := []string{"claude-sonnet-5", "claude-haiku-4-5"}; !slices.Equal(cat.Usable, want) || !cat.Live {
		t.Errorf("catalog = %+v, want Live with Usable %v", cat, want)
	}
	seen := ps.seen()
	if len(seen) != 2 {
		t.Fatalf("requests = %d, want 2", len(seen))
	}
	for i, q := range seen {
		if q.Get("limit") != "1000" {
			t.Errorf("request %d limit = %q, want 1000", i, q.Get("limit"))
		}
	}
	if seen[1].Get("after_id") != "claude-sonnet-5" {
		t.Errorf("second request after_id = %q, want claude-sonnet-5", seen[1].Get("after_id"))
	}
}

func TestListModelCatalog_GeminiSecondPageFailureDegrades(t *testing.T) {
	ps := newPagingServer(t, func(q url.Values) (int, string) {
		if q.Get("pageToken") == "" {
			return http.StatusOK, `{"models":[{"name":"models/gemini-3.7-flash","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"p2"}`
		}
		return http.StatusInternalServerError, ""
	})
	cat, err := ListModelCatalog(context.Background(), ProviderGemini, "k", WithBaseURL(ps.srv.URL))
	if err != nil {
		t.Fatalf("a failed page must degrade, not error: %v", err)
	}
	if cat.Live || !slices.Equal(cat.Usable, StaticGemini) {
		t.Errorf("catalog = %+v, want static (Live false)", cat)
	}
}

func TestListModelCatalog_ClaudeSecondPageFailureDegrades(t *testing.T) {
	ps := newPagingServer(t, func(q url.Values) (int, string) {
		if q.Get("after_id") == "" {
			return http.StatusOK, `{"data":[{"id":"claude-sonnet-5"}],"has_more":true,"last_id":"claude-sonnet-5"}`
		}
		return http.StatusInternalServerError, ""
	})
	cat, err := ListModelCatalog(context.Background(), ProviderClaude, "k", WithBaseURL(ps.srv.URL))
	if err != nil {
		t.Fatalf("a failed page must degrade, not error: %v", err)
	}
	if cat.Live || !slices.Equal(cat.Usable, StaticClaude) {
		t.Errorf("catalog = %+v, want static (Live false)", cat)
	}
}

func TestListModelCatalog_PaginationIsBounded(t *testing.T) {
	// The bound is policy (MADR 0009 §2), so it is asserted as a literal: a
	// comparison against maxListingPages could not detect the bound changing.
	const wantPages = 10
	for _, tc := range []struct {
		provider, body string
		static         []string
	}{
		{ProviderGemini, `{"models":[{"name":"models/gemini-3.7-flash","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"again"}`, StaticGemini},
		{ProviderClaude, `{"data":[{"id":"claude-sonnet-5"}],"has_more":true,"last_id":"x"}`, StaticClaude},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			ps := newPagingServer(t, func(url.Values) (int, string) { return http.StatusOK, tc.body })
			cat, err := ListModelCatalog(context.Background(), tc.provider, "k", WithBaseURL(ps.srv.URL))
			if err != nil {
				t.Fatalf("ListModelCatalog: %v", err)
			}
			if n := len(ps.seen()); n != wantPages {
				t.Errorf("requests = %d, want %d", n, wantPages)
			}
			if cat.Live || !slices.Equal(cat.Usable, tc.static) {
				t.Errorf("catalog = %+v, want static (Live false)", cat)
			}
		})
	}
}

func TestListModelCatalog_ClaudeHasMoreWithoutLastIDDegrades(t *testing.T) {
	ps := newPagingServer(t, func(url.Values) (int, string) {
		return http.StatusOK, `{"data":[{"id":"claude-sonnet-5"}],"has_more":true}`
	})
	cat, err := ListModelCatalog(context.Background(), ProviderClaude, "k", WithBaseURL(ps.srv.URL))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}
	if n := len(ps.seen()); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
	if cat.Live || !slices.Equal(cat.Usable, StaticClaude) {
		t.Errorf("catalog = %+v, want static (Live false)", cat)
	}
}

func TestListModelCatalog_SinglePageRequestsMaxPageSize(t *testing.T) {
	gemini := newPagingServer(t, func(url.Values) (int, string) {
		return http.StatusOK, `{"models":[{"name":"models/gemini-3.7-flash","supportedGenerationMethods":["generateContent"]}]}`
	})
	if _, err := ListModelCatalog(context.Background(), ProviderGemini, "k", WithBaseURL(gemini.srv.URL)); err != nil {
		t.Fatalf("gemini: %v", err)
	}
	if seen := gemini.seen(); len(seen) != 1 || seen[0].Get("pageSize") != "1000" || seen[0].Has("pageToken") {
		t.Errorf("gemini queries = %v, want one request with pageSize=1000 and no pageToken", seen)
	}

	claude := newPagingServer(t, func(url.Values) (int, string) {
		return http.StatusOK, `{"data":[{"id":"claude-sonnet-5"}],"has_more":false}`
	})
	if _, err := ListModelCatalog(context.Background(), ProviderClaude, "k", WithBaseURL(claude.srv.URL)); err != nil {
		t.Fatalf("claude: %v", err)
	}
	if seen := claude.seen(); len(seen) != 1 || seen[0].Get("limit") != "1000" || seen[0].Has("after_id") {
		t.Errorf("claude queries = %v, want one request with limit=1000 and no after_id", seen)
	}
}
