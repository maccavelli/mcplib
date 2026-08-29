//go:build live_gateways

// Opt-in live tests against the real gateways. Excluded from the default build.
//
//	go test -tags live_gateways ./llmprovider/ -run Live -v
//
// Kilo tests use FREE models and send NO real API key: Kilo ignores a bogus
// bearer for free models (measured 200). They are rate-limited upstream, so 429
// and 400 SKIP rather than fail — these assert wire-format correctness, not
// gateway availability.
//
// OpenCode tests REQUIRE OPENCODE_API_KEY (plan deviation D3). Its free models
// answer 200 with NO Authorization header but 401 with a bogus one, and
// NewOpencode requires a non-empty key and always sends it — so a placeholder
// is strictly worse than none there.
//
// The Hugging Face test REQUIRES HF_TOKEN: HF reports is_free:false for all
// provider offerings, so no credential-free path exists (verified 2026-08-29).
//
// The shape assertions below are the point of this suite. A test that only
// checked "a response came back" would still pass after a gateway renamed a
// field, while the code silently degraded. Each failure is a DRIFT REPORT, not
// necessarily a bug: it means a wire shape changed after the date recorded in
// the relevant wireShapesProbedOn* constant.
package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// skipIfTransient converts upstream rate limiting and temporary model
// unavailability into a skip: these assert wire shapes, not uptime.
func skipIfTransient(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrInvalidRequest) {
		t.Skipf("gateway transient (free tier limits / model unavailable): %v", err)
	}
}

// opencodeKey returns a real OpenCode credential or skips. See deviation D3:
// OpenCode rejects a bogus bearer even for models that are free without one.
func opencodeKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("OPENCODE_API_KEY")
	if key == "" {
		t.Skip("OPENCODE_API_KEY unset: OpenCode returns 401 for a bogus key even on " +
			"free models, and NewOpencode always sends the key it is given (deviation D3)")
	}
	return key
}

func liveCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 90*time.Second)
}

// getJSON fetches a public gateway listing with NO Authorization header.
func getJSON(t *testing.T, url string, into any) int {
	t.Helper()
	ctx, cancel := liveCtx(t)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		t.Skipf("gateway unreachable: %v", err)
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode
	}
	if into != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(into); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return resp.StatusCode
}

func TestLive_OpencodeChatCompletions(t *testing.T) {
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewOpencode(ProviderOpencodeZen, opencodeKey(t), "hy3-free")
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("empty output (probed %s)", wireShapesProbedOnOpencode)
	}
}

func TestLive_OpencodeResponses(t *testing.T) {
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewOpencode(ProviderOpencodeZen, opencodeKey(t), "muse-spark-1.2-contributor-free")
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	if p.Route() != OpencodeRouteResponses {
		t.Fatalf("Route() = %q, want responses", p.Route())
	}
	resp, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: "Reply with only the word ALPHA"})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	var sawMessage bool
	for _, it := range resp.Output {
		if _, ok := it.(MessageItem); ok {
			sawMessage = true
		}
	}
	if !sawMessage {
		t.Errorf("no MessageItem in responses output (probed %s)", wireShapesProbedOnOpencode)
	}
}

// TestLive_OpencodeRouteStillEnforced is the measurement the entire 63+26-row
// route table rests on: routes are NOT interchangeable. If this fails, OpenCode
// has become a translating gateway and the table is no longer necessary.
func TestLive_OpencodeRouteStillEnforced(t *testing.T) {
	const model = "muse-spark-1.2-contributor-free"
	ctx, cancel := liveCtx(t)
	defer cancel()

	key := opencodeKey(t)
	onResponses, err := NewOpencode(ProviderOpencodeZen, key, model)
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	_, err = onResponses.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: "hi"})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("%s must still succeed on its documented /responses route: %v", model, err)
	}

	onChat, err := NewOpencode(ProviderOpencodeZen, key, model,
		WithOpencodeRoute(OpencodeRouteChatCompletions))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	if _, err := onChat.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: "hi"}); err == nil {
		t.Errorf("DRIFT (probed %s): %s now succeeds on /chat/completions. Routes were "+
			"measured as non-interchangeable; if that changed, the route table may no "+
			"longer be needed", wireShapesProbedOnOpencode, model)
	}
}

func TestLive_KiloChatCompletions(t *testing.T) {
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewKilo("unused-free-model", "kilo-auto/free")
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("empty output (probed %s)", wireShapesProbedOnKilo)
	}
}

// TestLive_KiloToolCall is the only end-to-end tool-calling coverage in this
// change: OpenCode's free models refuse tool requests and Hugging Face has no
// free tier.
func TestLive_KiloToolCall(t *testing.T) {
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewKilo("unused-free-model", "kilo-auto/free", WithMaxTokens(400))
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	tool := Tool{
		Name:        "get_weather",
		Description: "Get the weather for a city",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"city": map[string]any{"type": "string"}},
			"required":   []string{"city"},
		},
	}
	args, err := p.GenerateWithTool(ctx, "What is the weather in Paris? Use the tool.", tool)
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateWithTool: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		t.Fatalf("tool arguments %q are not valid JSON (probed %s): %v",
			args, wireShapesProbedOnKilo, err)
	}
}

// TestLive_KiloReasoningSpelling pins the dual-name decoder: Kilo emits
// message.reasoning, OpenCode emits message.reasoning_content.
func TestLive_KiloReasoningSpelling(t *testing.T) {
	ctx, cancel := liveCtx(t)
	defer cancel()
	body := chatCompletionsBody("kilo-auto/free", 400,
		[]Item{MessageItem{Role: jsonRoleUser, Text: "Say ALPHA only"}}, chatCompletionsOpts{})
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", kiloBaseURL+"/chat/completions", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		t.Skipf("gateway unreachable: %v", err)
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		t.Skipf("gateway returned HTTP %d (free tier limits)", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message map[string]json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Choices) == 0 {
		t.Skip("no choices returned")
	}
	msg := decoded.Choices[0].Message
	if _, hasReasoning := msg[jsonKeyReasoning]; !hasReasoning {
		// Literal, not a constant: jsonKeyReasoningContent was dropped in plan
		// deviation D1 as unused, and reintroducing it for a build-tagged file
		// only would re-create the D2 `unused` problem.
		if _, hasContent := msg["reasoning_content"]; hasContent {
			t.Errorf("DRIFT (probed %s): Kilo now emits %q, not %q. The decoder handles "+
				"both, but the MADR's field-name table is stale",
				wireShapesProbedOnKilo, "reasoning_content", jsonKeyReasoning)
		}
		// Neither present is acceptable: not every model reasons.
	}
}

// TestLive_KiloSupportedParameters pins the metadata capability gating relies on.
func TestLive_KiloSupportedParameters(t *testing.T) {
	var listing struct {
		Data []kiloCatalogEntry `json:"data"`
	}
	if code := getJSON(t, kiloBaseURL+"/models", &listing); code != http.StatusOK {
		t.Skipf("models endpoint returned HTTP %d", code)
	}
	if len(listing.Data) == 0 {
		t.Fatalf("DRIFT (probed %s): empty catalog", wireShapesProbedOnKilo)
	}
	var withParams, withTools int
	for _, m := range listing.Data {
		if len(m.SupportedParameters) > 0 {
			withParams++
		}
		if contains(m.SupportedParameters, jsonKeyTools) {
			withTools++
		}
	}
	if withParams == 0 {
		t.Errorf("DRIFT (probed %s): no model publishes supported_parameters; "+
			"capability gating has nothing to gate on", wireShapesProbedOnKilo)
	}
	if withTools == 0 {
		t.Errorf("DRIFT (probed %s): no model lists %q in supported_parameters",
			wireShapesProbedOnKilo, jsonKeyTools)
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// TestLive_HuggingFaceMetadataFields pins the four fields metadata-driven
// curation depends on. Deliberately not universal: re-probed 2026-08-29, only
// 253 of 317 offerings carry all four, so asserting every offering would be
// flaky by construction.
func TestLive_HuggingFaceMetadataFields(t *testing.T) {
	var listing struct {
		Data []struct {
			ID           string `json:"id"`
			Architecture struct {
				OutputModalities []string `json:"output_modalities"`
			} `json:"architecture"`
			Providers []map[string]json.RawMessage `json:"providers"`
		} `json:"data"`
	}
	if code := getJSON(t, huggingFaceBaseURL+"/models", &listing); code != http.StatusOK {
		t.Skipf("models endpoint returned HTTP %d", code)
	}
	if len(listing.Data) == 0 {
		t.Fatalf("DRIFT (probed %s): empty catalog", wireShapesProbedOnHuggingFace)
	}
	for _, m := range listing.Data {
		if len(m.Architecture.OutputModalities) == 0 {
			t.Errorf("DRIFT (probed %s): %q has no architecture.output_modalities; "+
				"modality filtering would silently pass everything",
				wireShapesProbedOnHuggingFace, m.ID)
			break
		}
	}
	var ok int
	for _, m := range listing.Data {
		for _, pr := range m.Providers {
			_, a := pr["throughput"]
			_, b := pr["first_token_latency_ms"]
			_, c := pr["supports_tools"]
			if a && b && c {
				ok++
			}
		}
	}
	if ok == 0 {
		t.Errorf("DRIFT (probed %s): no offering publishes throughput + "+
			"first_token_latency_ms + supports_tools; metadata ranking is dead",
			wireShapesProbedOnHuggingFace)
	}
}

func TestLive_HuggingFaceChatCompletions(t *testing.T) {
	token := os.Getenv("HF_TOKEN")
	if token == "" {
		t.Skip("HF_TOKEN unset: Hugging Face reports is_free:false for all offerings, " +
			"so there is no credential-free path")
	}
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewHuggingFace(token, "openai/gpt-oss-20b")
	if err != nil {
		t.Fatalf("NewHuggingFace: %v", err)
	}
	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("empty output (probed %s)", wireShapesProbedOnHuggingFace)
	}
}

// TestLive_ListingsNeedNoCredential pins that all four catalogs are public.
// Discovery works before a key is configured, which the wizards rely on.
func TestLive_ListingsNeedNoCredential(t *testing.T) {
	for name, url := range map[string]string{
		ProviderOpencodeZen: opencodeZenBaseURL + "/models",
		ProviderOpencodeGo:  opencodeGoBaseURL + "/models",
		ProviderHuggingFace: huggingFaceBaseURL + "/models",
		ProviderKilo:        kiloBaseURL + "/models",
	} {
		t.Run(name, func(t *testing.T) {
			if code := getJSON(t, url, nil); code != http.StatusOK {
				t.Errorf("DRIFT: %s returned HTTP %d with no credential; discovery "+
					"before key configuration would break", url, code)
			}
		})
	}
}
