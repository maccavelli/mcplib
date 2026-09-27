package llmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// routeServer serves a metadata document listing model under gateway's
// section, with provider.npm set when npm is not empty (listed=false serves a
// document without the model), and answers every generation route with "ok",
// recording the request paths.
func routeServer(t *testing.T, gateway, model, npm string, listed bool) (*httptest.Server, func() []string) {
	t.Helper()
	t.Setenv(envDisableModelMetadata, "0") // TestMain turns the fetch off
	var (
		mu    sync.Mutex
		paths []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api.json") {
			entry := `"other-model":{"id":"other-model"}`
			if listed {
				entry = `"` + model + `":{"id":"` + model + `"`
				if npm != "" {
					entry += `,"provider":{"npm":"` + npm + `"}`
				}
				entry += "}"
			}
			_, _ = w.Write([]byte(`{"` + modelMetadataKey(gateway) + `":{"models":{` + entry + `}}}`))
			return
		}
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
		case strings.HasSuffix(r.URL.Path, "/responses"):
			_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
		case strings.HasSuffix(r.URL.Path, ":generateContent"):
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
		default:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

// generatePath runs one Generate and returns the single request path it sent.
func generatePath(t *testing.T, srv *httptest.Server, paths func() []string, gateway, model string, opts ...ProviderOption) string {
	t.Helper()
	opts = append([]ProviderOption{WithBaseURL(srv.URL), WithModelMetadataURL(srv.URL + "/api.json")}, opts...)
	p, err := NewOpencode(gateway, "k", model, opts...)
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	if _, err := p.Generate(context.Background(), "hi"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := paths()
	if len(got) != 1 {
		t.Fatalf("requests = %v, want exactly one", got)
	}
	return got[0]
}

// TestOpencode_RoutesFromMetadata: the metadata's provider.npm picks the
// route, as OpenCode's client does (provider.ts:1274-1278), for a model the
// table does not know and for one it routes differently.
func TestOpencode_RoutesFromMetadata(t *testing.T) {
	tests := []struct {
		name, gateway, model, npm, want string
	}{
		{"anthropic", ProviderOpencodeGo, "brand-new-model", "@ai-sdk/anthropic", "/messages"},
		{"openai", ProviderOpencodeGo, "brand-new-model", "@ai-sdk/openai", "/responses"},
		{"google", ProviderOpencodeZen, "brand-new-model", "@ai-sdk/google", "/models/brand-new-model:generateContent"},
		{"compatible", ProviderOpencodeGo, "brand-new-model", "@ai-sdk/openai-compatible", "/chat/completions"},
		{"unset", ProviderOpencodeGo, "brand-new-model", "", "/chat/completions"},
		// The table says messages; the metadata wins.
		{"over table", ProviderOpencodeGo, "minimax-m3", "@ai-sdk/openai-compatible", "/chat/completions"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, paths := routeServer(t, tc.gateway, tc.model, tc.npm, true)
			if got := generatePath(t, srv, paths, tc.gateway, tc.model); got != tc.want {
				t.Errorf("path = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOpencode_RouteOverrideBeatsMetadata: WithOpencodeRoute wins over the
// metadata.
func TestOpencode_RouteOverrideBeatsMetadata(t *testing.T) {
	srv, paths := routeServer(t, ProviderOpencodeGo, "brand-new-model", "@ai-sdk/anthropic", true)
	got := generatePath(t, srv, paths, ProviderOpencodeGo, "brand-new-model",
		WithOpencodeRoute(OpencodeRouteChatCompletions))
	if got != "/chat/completions" {
		t.Errorf("path = %q, want the override's /chat/completions", got)
	}
}

// TestOpencode_RouteWithoutMetadataUsesTable: metadata that is disabled, or
// that does not list the model, leaves the table's route.
func TestOpencode_RouteWithoutMetadataUsesTable(t *testing.T) {
	t.Run("absent from document", func(t *testing.T) {
		srv, paths := routeServer(t, ProviderOpencodeGo, "minimax-m3", "", false)
		if got := generatePath(t, srv, paths, ProviderOpencodeGo, "minimax-m3"); got != "/messages" {
			t.Errorf("path = %q, want the table's /messages", got)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		srv, paths := routeServer(t, ProviderOpencodeGo, "minimax-m3", "@ai-sdk/openai-compatible", true)
		t.Setenv(envDisableModelMetadata, "1")
		if got := generatePath(t, srv, paths, ProviderOpencodeGo, "minimax-m3"); got != "/messages" {
			t.Errorf("path = %q, want the table's /messages", got)
		}
	})
}

// TestOpencodeRouteTable_MatchesMetadataSnapshot: every active Zen and Go
// model in testdata/opencode-routes.json (provider.npm per model, from
// models.opencode.ai/api.json on 2026-09-27, deprecated models omitted)
// resolves without metadata to the route its npm selects. It pins the table's
// refresh; the oracle below is written independently of the production map.
func TestOpencodeRouteTable_MatchesMetadataSnapshot(t *testing.T) {
	raw, err := os.ReadFile("testdata/opencode-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]map[string]string
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	oracle := map[string]OpencodeRoute{
		"@ai-sdk/openai":    OpencodeRouteResponses,
		"@ai-sdk/anthropic": OpencodeRouteMessages,
		"@ai-sdk/google":    OpencodeRouteGoogle,
	}
	gateways := map[string]string{"opencode": ProviderOpencodeZen, "opencode-go": ProviderOpencodeGo}
	for section, models := range snapshot {
		gateway, ok := gateways[section]
		if !ok || len(models) == 0 {
			t.Fatalf("snapshot section %q: unknown or empty", section)
		}
		for model, npm := range models {
			want, ok := oracle[npm]
			if !ok {
				want = OpencodeRouteChatCompletions
			}
			got, err := resolveOpencodeRoute(gateway, model, "")
			if err != nil || got != want {
				t.Errorf("%s %s (npm %q): route = %q, %v; want %q", gateway, model, npm, got, err, want)
			}
		}
	}
}

// TestIsUsableOpencodeModel_DeniesSystemone: jev-* models use OpenCode's
// "systemone" route, which has no encoder here (MADR 0012 §3.1).
func TestIsUsableOpencodeModel_DeniesSystemone(t *testing.T) {
	for _, id := range []string{"jev-1.13", "jev-1.13-free", "JEV-2"} {
		if isUsableOpencodeModel(id) {
			t.Errorf("isUsableOpencodeModel(%q) = true, want false", id)
		}
	}
	if !isUsableOpencodeModel("kimi-k2.6") {
		t.Error("isUsableOpencodeModel(kimi-k2.6) = false, want true")
	}
}
