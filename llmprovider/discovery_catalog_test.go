package llmprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// openAIStyleListing renders a {"data":[{"id":…}]} listing, the shape shared by
// OpenAI, Anthropic, xAI and the OpenCode gateways.
func openAIStyleListing(ids ...string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf(`{"id":%q}`, id))
	}
	return `{"data":[` + strings.Join(parts, ",") + `]}`
}

// ollamaTags renders an Ollama GET /api/tags body.
func ollamaTags(names ...string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf(`{"name":%q}`, n))
	}
	return `{"models":[` + strings.Join(parts, ",") + `]}`
}

// hfListing renders n text→text, live, tool-capable Hugging Face models named
// org/m-01…, with throughput descending so listing order is metadata order.
func hfListing(n int) string {
	parts := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		parts = append(parts, fmt.Sprintf(`{"id":"org/m-%02d","architecture":{"input_modalities":["text"],"output_modalities":["text"]},`+
			`"providers":[{"provider":"a","status":"live","supports_tools":true,"throughput":%d,"first_token_latency_ms":100}]}`, i, 1000-i))
	}
	return `{"object":"list","data":[` + strings.Join(parts, ",") + `]}`
}

// kiloListing renders n text→text, tool-capable, non-training Kilo models named
// org/m-01…, with completion price ascending so listing order is price order.
func kiloListing(n int) string {
	parts := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		parts = append(parts, fmt.Sprintf(`{"id":"org/m-%02d","architecture":{"input_modalities":["text"],"output_modalities":["text"]},`+
			`"pricing":{"completion":"0.00000%d"},"supported_parameters":["tools"],"mayTrainOnYourPrompts":false}`, i, i))
	}
	return `{"data":[` + strings.Join(parts, ",") + `]}`
}

const geminiCatalogFixture = `{"models":[
 {"name":"models/gemini-3.7-flash","supportedGenerationMethods":["generateContent"]},
 {"name":"models/gemini-2.5-flash","supportedGenerationMethods":["generateContent"]},
 {"name":"models/gemini-embedding-001","supportedGenerationMethods":["embedContent"]},
 {"name":"models/gemini-3.5-flash-preview-09-2026","supportedGenerationMethods":["generateContent"]}]}`

// catalogCase is one provider with a good listing fixture.
type catalogCase struct {
	provider, key, body string
}

func goodCatalogCases() []catalogCase {
	return []catalogCase{
		{ProviderGemini, "k", geminiCatalogFixture},
		{ProviderOpenAI, "k", openAIStyleListing("gpt-4.1-mini", "gpt-4o", "gpt-5.1")},
		{ProviderClaude, "k", openAIStyleListing("claude-haiku-4-5", "claude-sonnet-5")},
		{ProviderGrok, "k", openAIStyleListing("grok-4.6", "grok-3-mini")},
		{ProviderOpencodeZen, "k", opencodeListingFixture},
		{ProviderOpencodeGo, "k", opencodeListingFixture},
		{ProviderHuggingFace, "k", hfListingFixture},
		{ProviderKilo, "k", kiloListingFixture},
		{ProviderOllama, "", ollamaTags("llama3:latest", "mistral:7b")},
	}
}

// serveBody starts a server that answers every request with body.
func serveBody(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListModelCatalog_RecommendedMatchesListAvailable(t *testing.T) {
	for _, tc := range goodCatalogCases() {
		t.Run(tc.provider, func(t *testing.T) {
			srv := serveBody(t, tc.body)
			cat, err := ListModelCatalog(context.Background(), tc.provider, tc.key, WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("ListModelCatalog: %v", err)
			}
			avail, err := ListAvailableModels(context.Background(), tc.provider, tc.key, WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("ListAvailableModels: %v", err)
			}
			if !slices.Equal(cat.Recommended, avail) {
				t.Errorf("Recommended = %v, ListAvailableModels = %v", cat.Recommended, avail)
			}
			if !cat.Live {
				t.Errorf("Live = false for a good fixture")
			}
		})
	}
}

func TestListModelCatalog_OpenAIRecommendedIsCurated(t *testing.T) {
	order := []string{"gpt-5.1", "o3", "gpt-4o", "o4-mini", "gpt-4.1", "gpt-4o-mini", "gpt-4.1-nano", "gpt-4.1-mini"}
	srv := serveBody(t, openAIStyleListing(order...))
	cat, err := ListModelCatalog(context.Background(), ProviderOpenAI, "k", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}
	if !slices.Equal(cat.Recommended, StaticOpenAI) {
		t.Errorf("Recommended = %v, want StaticOpenAI %v in catalog order", cat.Recommended, StaticOpenAI)
	}
	if !slices.Equal(cat.Usable, order) {
		t.Errorf("Usable = %v, want listing order %v", cat.Usable, order)
	}
	if !cat.Live {
		t.Error("Live = false")
	}
}

func TestListModelCatalog_UsableIsUncapped(t *testing.T) {
	zen := make([]string, 0, 8)
	for i := 1; i <= 8; i++ {
		zen = append(zen, fmt.Sprintf("zen-model-%02d", i))
	}
	for _, tc := range []catalogCase{
		{ProviderHuggingFace, "k", hfListing(8)},
		{ProviderKilo, "k", kiloListing(8)},
		{ProviderOpencodeZen, "k", openAIStyleListing(zen...)},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			srv := serveBody(t, tc.body)
			cat, err := ListModelCatalog(context.Background(), tc.provider, tc.key, WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("ListModelCatalog: %v", err)
			}
			if len(cat.Usable) != 8 {
				t.Errorf("len(Usable) = %d, want 8: %v", len(cat.Usable), cat.Usable)
			}
			if len(cat.Recommended) != MaxListedModels {
				t.Errorf("len(Recommended) = %d, want %d", len(cat.Recommended), MaxListedModels)
			}
		})
	}
}

func TestListModelCatalog_RecommendedSubsetOfUsable(t *testing.T) {
	for _, tc := range goodCatalogCases() {
		t.Run(tc.provider, func(t *testing.T) {
			srv := serveBody(t, tc.body)
			cat, err := ListModelCatalog(context.Background(), tc.provider, tc.key, WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("ListModelCatalog: %v", err)
			}
			for _, id := range cat.Recommended {
				if !slices.Contains(cat.Usable, id) {
					t.Errorf("Recommended id %q is not in Usable %v", id, cat.Usable)
				}
			}
		})
	}
}

func TestListModelCatalog_FiltersStillApply(t *testing.T) {
	for _, tc := range []struct {
		catalogCase
		absent []string
	}{
		{catalogCase{ProviderKilo, "k", kiloListingFixture}, []string{"org/trains", "org/no-tools"}},
		{catalogCase{ProviderHuggingFace, "k", hfListingFixture}, []string{"org/not-live"}},
		{catalogCase{ProviderOpencodeZen, "k", opencodeListingFixture}, []string{"deepseek-v4-flash-vision-exp"}},
		{catalogCase{ProviderGemini, "k", geminiCatalogFixture}, []string{"gemini-embedding-001", "gemini-3.5-flash-preview-09-2026"}},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			srv := serveBody(t, tc.body)
			cat, err := ListModelCatalog(context.Background(), tc.provider, tc.key, WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("ListModelCatalog: %v", err)
			}
			if !cat.Live {
				t.Fatalf("Live = false; the fixture must be read, not replaced by static")
			}
			for _, id := range tc.absent {
				if slices.Contains(cat.Usable, id) {
					t.Errorf("filtered id %q is in Usable %v", id, cat.Usable)
				}
			}
		})
	}
}

func TestListModelCatalog_OllamaSplit(t *testing.T) {
	names := []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8"}
	srv := serveBody(t, ollamaTags(names...))
	cat, err := ListModelCatalog(context.Background(), ProviderOllama, "", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}
	if !slices.Equal(cat.Usable, names) {
		t.Errorf("Usable = %v, want %v", cat.Usable, names)
	}
	if !slices.Equal(cat.Recommended, names[:MaxListedModels]) {
		t.Errorf("Recommended = %v, want %v", cat.Recommended, names[:MaxListedModels])
	}
	// Appending to Recommended must not write through into Usable.
	_ = append(cat.Recommended, "sentinel")
	if cat.Usable[MaxListedModels] != names[MaxListedModels] {
		t.Errorf("Usable[%d] = %q after appending to Recommended; the views alias", MaxListedModels, cat.Usable[MaxListedModels])
	}
}

func TestListModelCatalog_OllamaEmptyIsLive(t *testing.T) {
	srv := serveBody(t, `{"models":[]}`)
	cat, err := ListModelCatalog(context.Background(), ProviderOllama, "", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListModelCatalog: %v", err)
	}
	if !cat.Live || len(cat.Recommended) != 0 || len(cat.Usable) != 0 {
		t.Errorf("catalog = %+v, want Live with both views empty", cat)
	}
}

func TestListModelCatalog_LiveFlag(t *testing.T) {
	for _, p := range []string{
		ProviderGemini, ProviderOpenAI, ProviderClaude, ProviderGrok,
		ProviderOpencodeZen, ProviderOpencodeGo, ProviderHuggingFace, ProviderKilo,
	} {
		t.Run(p, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()
			cat, err := ListModelCatalog(context.Background(), p, "k", WithBaseURL(srv.URL))
			if err != nil {
				t.Fatalf("a failed listing must degrade, not error: %v", err)
			}
			if cat.Live {
				t.Error("Live = true for a failed listing")
			}
			static := StaticModels(p)
			if !slices.Equal(cat.Recommended, static) || !slices.Equal(cat.Usable, static) {
				t.Errorf("catalog = %+v, want both views = %v", cat, static)
			}
		})
	}
}

func TestListModelCatalogWithSource_ChatGPTDoesNotHTTP(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits++
		t.Error("ChatGPT model listing made an HTTP request")
	}))
	defer srv.Close()

	session := &OAuthSession{
		Issuer: DefaultOpenAIIssuer,
		Access: "session-access",
		Expiry: time.Now().Add(time.Hour),
	}
	cat, err := ListModelCatalogWithSource(context.Background(), ProviderOpenAI, session, WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("ListModelCatalogWithSource() error = %v", err)
	}
	if !reflect.DeepEqual(cat.Recommended, StaticOpenAIChatGPT) || !reflect.DeepEqual(cat.Usable, StaticOpenAIChatGPT) {
		t.Fatalf("catalog = %+v, want both views = %v", cat, StaticOpenAIChatGPT)
	}
	if cat.Live {
		t.Error("Live = true for the ChatGPT short-circuit")
	}
	cat.Recommended[0] = "mutated"
	again, err := ListModelCatalogWithSource(context.Background(), ProviderOpenAI, session)
	if err != nil {
		t.Fatalf("second call error = %v", err)
	}
	if hits != 0 || again.Recommended[0] == "mutated" {
		t.Fatalf("HTTP hits/second result = %d/%v", hits, again.Recommended)
	}
}

func TestListModelCatalog_Errors(t *testing.T) {
	if _, err := ListModelCatalog(context.Background(), "unsupported", "k"); err == nil {
		t.Error("unsupported provider: want an error")
	}
	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer notFound.Close()
	if _, err := ListModelCatalog(context.Background(), ProviderOllama, "", WithBaseURL(notFound.URL)); err == nil {
		t.Error("ollama 404: want an error")
	}
	if _, err := ListModelCatalogWithSource(context.Background(), ProviderGemini, nil); err == nil {
		t.Error("nil TokenSource: want an error")
	}
}
