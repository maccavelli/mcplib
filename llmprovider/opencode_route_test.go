package llmprovider

import (
	"errors"
	"testing"
	"time"
)

// TestOpencodeRoute_Table pins the published endpoint tables, including the
// per-gateway divergence that makes a model-only key wrong.
func TestOpencodeRoute_Table(t *testing.T) {
	tests := []struct {
		name    string
		gateway string
		model   string
		want    OpencodeRoute
	}{
		// The divergence proof: same model id, different route per gateway.
		{"zen minimax is chat", ProviderOpencodeZen, "minimax-m3", OpencodeRouteChatCompletions},
		{"go minimax is messages", ProviderOpencodeGo, "minimax-m3", OpencodeRouteMessages},

		{"zen gpt", ProviderOpencodeZen, "gpt-5.5", OpencodeRouteResponses},
		{"zen grok", ProviderOpencodeZen, "grok-4.6", OpencodeRouteResponses},
		{"zen muse", ProviderOpencodeZen, "muse-spark-1.2", OpencodeRouteResponses},
		{"zen claude", ProviderOpencodeZen, "claude-opus-4-5", OpencodeRouteMessages},
		{"zen qwen", ProviderOpencodeZen, "qwen3.6-plus", OpencodeRouteMessages},
		{"zen gemini", ProviderOpencodeZen, "gemini-3.7-flash", OpencodeRouteGoogle},
		{"zen deepseek", ProviderOpencodeZen, "deepseek-v4-pro", OpencodeRouteChatCompletions},
		{"zen glm", ProviderOpencodeZen, "glm-5.2", OpencodeRouteChatCompletions},
		{"zen kimi", ProviderOpencodeZen, "kimi-k3", OpencodeRouteChatCompletions},
		{"zen big-pickle", ProviderOpencodeZen, "big-pickle", OpencodeRouteChatCompletions},

		{"go gpt", ProviderOpencodeGo, "gpt-5.6-luna", OpencodeRouteResponses},
		{"go glm", ProviderOpencodeGo, "glm-5.3", OpencodeRouteChatCompletions},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveOpencodeRoute(tc.gateway, tc.model, "")
			if err != nil {
				t.Fatalf("resolveOpencodeRoute(%q,%q): %v", tc.gateway, tc.model, err)
			}
			if got != tc.want {
				t.Errorf("resolveOpencodeRoute(%q,%q) = %q, want %q", tc.gateway, tc.model, got, tc.want)
			}
		})
	}
}

// TestOpencodeRoute_Heuristic covers the ten model ids that are live on the
// gateways but absent from the published tables (plan §3.3). This is the
// regression guard for catalog drift: every one must still resolve correctly
// through the prefix heuristic.
func TestOpencodeRoute_Heuristic(t *testing.T) {
	tests := []struct {
		gateway string
		model   string
		want    OpencodeRoute
	}{
		{ProviderOpencodeZen, "claude-sonnet-4", OpencodeRouteMessages},
		{ProviderOpencodeZen, "deepseek-v4-flash-free", OpencodeRouteChatCompletions},
		{ProviderOpencodeZen, "laguna-s-2.1-free", OpencodeRouteChatCompletions},
		{ProviderOpencodeGo, "kimi-k2.5", OpencodeRouteChatCompletions},
		{ProviderOpencodeGo, "glm-5", OpencodeRouteChatCompletions},
		{ProviderOpencodeGo, "mimo-v2-pro", OpencodeRouteChatCompletions},
		{ProviderOpencodeGo, "mimo-v2-omni", OpencodeRouteChatCompletions},
		{ProviderOpencodeGo, "hy3-preview", OpencodeRouteChatCompletions},
		{ProviderOpencodeGo, "qwen3.5-plus", OpencodeRouteMessages},
		{ProviderOpencodeGo, "grok-4.5", OpencodeRouteResponses},
	}
	for _, tc := range tests {
		t.Run(tc.gateway+"/"+tc.model, func(t *testing.T) {
			// Guard the premise: these must NOT be in the table, or the test
			// is asserting table lookups rather than the heuristic.
			if _, tabled := opencodeRouteTable[tc.gateway][tc.model]; tabled {
				t.Fatalf("%q is in the route table for %q; move this case to TestOpencodeRoute_Table",
					tc.model, tc.gateway)
			}
			got := opencodeHeuristicRoute(tc.gateway, tc.model)
			if got != tc.want {
				t.Errorf("opencodeHeuristicRoute(%q,%q) = %q, want %q", tc.gateway, tc.model, got, tc.want)
			}
		})
	}
}

// TestOpencodeRoute_Override verifies an explicit route beats both the table
// and the heuristic, and that an unknown route is rejected.
func TestOpencodeRoute_Override(t *testing.T) {
	// gpt-5.5 is tabled as responses; the override must win.
	got, err := resolveOpencodeRoute(ProviderOpencodeZen, "gpt-5.5", OpencodeRouteChatCompletions)
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	if got != OpencodeRouteChatCompletions {
		t.Errorf("override = %q, want %q", got, OpencodeRouteChatCompletions)
	}

	if _, err := resolveOpencodeRoute(ProviderOpencodeZen, "gpt-5.5", OpencodeRoute("bogus")); err == nil {
		t.Error("expected error for invalid route override")
	} else if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("invalid override error = %v, want wrapping ErrInvalidRequest", err)
	}
}

// TestOpencodeRoute_Path pins the request path suffix per route.
func TestOpencodeRoute_Path(t *testing.T) {
	if got := OpencodeRouteGoogle.path("gemini-3.7-flash"); got != "/models/gemini-3.7-flash:generateContent" {
		t.Errorf("google path = %q", got)
	}
	if got := OpencodeRouteResponses.path("x"); got != "/responses" {
		t.Errorf("responses path = %q", got)
	}
	if got := OpencodeRouteMessages.path("x"); got != "/messages" {
		t.Errorf("messages path = %q", got)
	}
	if got := OpencodeRouteChatCompletions.path("x"); got != "/chat/completions" {
		t.Errorf("chat path = %q", got)
	}
}

// TestOpencodeBaseURL verifies the documented default base URLs.
func TestOpencodeBaseURL(t *testing.T) {
	zen, err := opencodeBaseURL(ProviderOpencodeZen)
	if err != nil || zen != "https://opencode.ai/zen/v1" {
		t.Errorf("zen base = %q, err = %v", zen, err)
	}
	goURL, err := opencodeBaseURL(ProviderOpencodeGo)
	if err != nil || goURL != "https://opencode.ai/zen/go/v1" {
		t.Errorf("go base = %q, err = %v", goURL, err)
	}
	if _, err := opencodeBaseURL("nope"); err == nil {
		t.Error("expected error for unknown gateway")
	}
}

// TestOpencodeRouteTable_NoUnknownRoutes ensures a typo in the table cannot ship.
func TestOpencodeRouteTable_NoUnknownRoutes(t *testing.T) {
	for gateway, byModel := range opencodeRouteTable {
		if len(byModel) == 0 {
			t.Errorf("route table for %q is empty", gateway)
		}
		for model, route := range byModel {
			if !route.valid() {
				t.Errorf("route table[%q][%q] = %q is not a known route", gateway, model, route)
			}
		}
	}
}

// TestProviderConstants_Distinct guards against a copy-paste error in the
// canonical identifier block.
func TestProviderConstants_Distinct(t *testing.T) {
	ids := []string{
		ProviderGemini, ProviderOpenAI, ProviderClaude, ProviderGrok,
		ProviderOpencodeZen, ProviderOpencodeGo, ProviderHuggingFace, ProviderKilo,
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			t.Error("empty provider identifier")
		}
		if _, dup := seen[id]; dup {
			t.Errorf("duplicate provider identifier: %q", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 8 {
		t.Errorf("expected 8 distinct provider identifiers, got %d", len(seen))
	}
}

// TestWireShapesProbedOn validates every gateway probe-date pin. Its second job
// is to give those constants a real, untagged use: golangci-lint analyses test
// files (run: tests: true) but not //go:build live_gateways files, so a
// reference from the live suite would not satisfy `unused`. See plan deviation
// D2. Phases 5 and 6 add their constants to this table.
func TestWireShapesProbedOn(t *testing.T) {
	pins := map[string]string{
		"opencode":    wireShapesProbedOnOpencode,
		"huggingface": wireShapesProbedOnHuggingFace,
		"kilo":        wireShapesProbedOnKilo,
	}
	for name, pin := range pins {
		t.Run(name, func(t *testing.T) {
			d, err := time.Parse(time.DateOnly, pin)
			if err != nil {
				t.Fatalf("wire-shape pin %q is not a YYYY-MM-DD date: %v", pin, err)
			}
			if d.After(time.Now()) {
				t.Errorf("wire-shape pin %q is in the future", pin)
			}
		})
	}
}
