//go:build live_gateways

// Opt-in live tests against the real gateways. Excluded from the default build.
//
//	go test -tags live_gateways ./llmprovider/ -run Live -v
//
// Kilo tests REQUIRE KILO_API_KEY: since 2026-09-26 Kilo answers a placeholder
// bearer with 401, even on free models (0010 PLAN deviation). The free-model
// tests are rate-limited upstream, so rate limits, outages, quota and
// not-permitted refusals SKIP rather than fail; a 400 FAILS, since a wire
// regression answers with one (skipIfTransient; 0013 D5, MADR 0012 §1.1). These
// assert wire-format correctness, not gateway availability. The 0010 reasoning
// gate (TestLive_KiloReasoningShapes) uses a paid model and treats a 400 as
// DRIFT.
//
// OpenCode tests REQUIRE OPENCODE_API_KEY (plan deviation D3): a bogus key is
// answered 401. Without a key, NewOpencode sends the gateway's "public" token
// (MADR 0012 §1.7). The generation tests use paid OpenCode Go models: Zen's
// free tier refuses clients other than OpenCode (403 FreeTierError, typed as
// ErrNotPermitted; measured 2026-09-26/27; MADR 0013 D1).
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
	"slices"
	"strings"
	"testing"
	"time"
)

// skipIfTransient converts upstream rate limiting, outages and account-state
// refusals into a skip: these assert wire shapes, not uptime or quota. A 400
// (ErrInvalidRequest) is a failure, since a wire regression answers with one
// (MADR 0012 amendment, 0013 D5).
func skipIfTransient(t *testing.T, err error) {
	t.Helper()
	if liveTransient(err) {
		t.Skipf("gateway transient (rate limit / outage / quota / not permitted): %v", err)
	}
}

// liveTransient reports whether err is a class the live suite skips on.
func liveTransient(err error) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrProviderUnavailable) ||
		errors.Is(err, ErrQuotaExhausted) || errors.Is(err, ErrNotPermitted)
}

// kiloKey returns a real Kilo credential or skips: Kilo rejects a placeholder
// bearer with 401 (0010 PLAN deviation, 2026-09-26).
func kiloKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("KILO_API_KEY")
	if key == "" {
		t.Skip("KILO_API_KEY unset: Kilo returns 401 for a placeholder key, even on free models")
	}
	return key
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
	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t),
		liveModel(t, ProviderOpencodeGo, "hy3", "glm-5.3-flash", "kimi-k2.6"))
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
	p, err := NewOpencode(ProviderOpencodeGo, opencodeKey(t),
		liveModel(t, ProviderOpencodeGo, "gpt-6-luna", "grok-4.6"))
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
	model := liveModel(t, ProviderOpencodeGo, "gpt-6-luna", "grok-4.6")
	ctx, cancel := liveCtx(t)
	defer cancel()

	key := opencodeKey(t)
	onResponses, err := NewOpencode(ProviderOpencodeGo, key, model)
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	_, err = onResponses.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: "hi"})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("%s must still succeed on its documented /responses route: %v", model, err)
	}

	onChat, err := NewOpencode(ProviderOpencodeGo, key, model,
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
	p, err := NewKilo(kiloKey(t), liveModel(t, ProviderKilo, kiloFreeCollecting...), WithKiloDataCollection(true))
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
	p, err := NewKilo(kiloKey(t), liveModel(t, ProviderKilo, kiloFreeCollecting...), WithMaxTokens(400),
		WithKiloDataCollection(true))
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
	body := chatCompletionsBody(liveModel(t, ProviderKilo, kiloFreeCollecting...), 400,
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

// TestLive_OpencodeKeyHeaderPerRoute pins MADR 0009 §1c against the live Zen
// server with a bogus key, which spends nothing: a route that does not read the
// header answers "Missing API key.", and the route's own header reaches key
// validation ("Invalid API key."). Measured 2026-09-26.
func TestLive_OpencodeKeyHeaderPerRoute(t *testing.T) {
	const bogus = "sk-bogus-000"
	routes := []struct{ name, path, body, right string }{
		{"messages", "/messages", `{"model":"claude-haiku-4-5","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, "x-api-key"},
		{"google", "/models/gemini-3.7-flash:generateContent", `{"contents":[{"parts":[{"text":"hi"}]}]}`, "x-goog-api-key"},
	}
	for _, rt := range routes {
		t.Run(rt.name, func(t *testing.T) {
			for _, tc := range []struct{ header, value, want string }{
				{"Authorization", "Bearer " + bogus, "Missing API key."},
				{rt.right, bogus, "Invalid API key."},
			} {
				status, body := postLive(t, opencodeZenBaseURL+rt.path, rt.body, tc.header, tc.value)
				if status != http.StatusUnauthorized {
					t.Skipf("%s via %s returned HTTP %d, not 401", rt.name, tc.header, status)
				}
				if !strings.Contains(body, tc.want) {
					t.Errorf("DRIFT: %s with the key in %s: want %q, got %s", rt.name, tc.header, tc.want, body)
				}
			}
		})
	}
}

// postLive sends a JSON POST with one key header and returns the status and a
// bounded body. A transport failure skips, like the suite's other requests.
func postLive(t *testing.T, url, body, header, value string) (int, string) {
	t.Helper()
	ctx, cancel := liveCtx(t)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(header, value)
	resp, err := defaultHTTPClient().Do(req)
	if err != nil {
		t.Skipf("gateway unreachable: %v", err)
	}
	defer closeResponseBody(resp)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(raw)
}

// TestLive_ModelMetadataDocument checks the shape MADR 0010 §2 depends on:
// models.opencode.ai still publishes the three sections, and a known Zen
// model still carries its reasoning flag.
func TestLive_ModelMetadataDocument(t *testing.T) {
	enableModelMetadata(t)
	ctx, cancel := liveCtx(t)
	defer cancel()
	doc, err := loadModelMetadata(ctx, ApplyOptions(nil))
	if err != nil {
		t.Skipf("metadata unreachable: %v", err)
	}
	for _, key := range []string{metadataKeyZen, metadataKeyGo, metadataKeyHF} {
		if len(doc[key]) == 0 {
			t.Errorf("DRIFT: %s no longer publishes section %q", defaultModelMetadataURL, key)
		}
	}
	m, ok := doc[metadataKeyZen]["glm-5.3-flash"]
	if !ok || m.Reasoning == nil || !*m.Reasoning {
		t.Errorf("DRIFT: %s/glm-5.3-flash reasoning = %v (present %v), want true", metadataKeyZen, m.Reasoning, ok)
	}
}

// TestLive_OpencodeChatReasoningEffort is MADR 0010 §6's OpenCode gate (as
// amended 2026-09-26): on OpenCode Go, for each chat-routed utility model whose
// reasoning_options list "low" (glm-flash and Hy families), the gateway accepts
// reasoning_effort "low". It also proves the x-opencode-session header: Go
// answers 400 MissingSessionID without it.
func TestLive_OpencodeChatReasoningEffort(t *testing.T) {
	key := opencodeKey(t)
	enableModelMetadata(t)
	for _, candidate := range []string{"glm-5.3-flash", "hy3"} {
		t.Run(candidate, func(t *testing.T) {
			model := liveModel(t, ProviderOpencodeGo, candidate)
			ctx, cancel := liveCtx(t)
			defer cancel()
			doc, err := loadModelMetadata(ctx, ApplyOptions(nil))
			if err != nil {
				t.Skipf("metadata unreachable: %v", err)
			}
			if !slices.Contains(doc.reasoningEfforts(ProviderOpencodeGo, model), effortLow) {
				t.Fatalf("DRIFT: %s reasoning_options no longer list %q", model, effortLow)
			}
			p, err := NewOpencode(ProviderOpencodeGo, key, model, WithReasoningEffort(effortLow), WithMaxTokens(400))
			if err != nil {
				t.Fatalf("NewOpencode: %v", err)
			}
			if p.Route() != OpencodeRouteChatCompletions {
				t.Fatalf("DRIFT: %s no longer routes to chat_completions (%s)", model, p.Route())
			}
			out, err := p.GenerateThinking(ctx, "Reply with only the word ALPHA")
			if errors.Is(err, ErrRateLimited) {
				t.Skipf("gateway transient: %v", err)
			}
			if err != nil {
				t.Fatalf("DRIFT (probed %s): gateway rejected reasoning_effort on %s: %v", wireShapesProbedOnOpencode, model, err)
			}
			if strings.TrimSpace(out) == "" {
				t.Errorf("empty output from %s", model)
			}
		})
	}
}

// TestLive_KiloReasoningShapes is MADR 0010 §6's gate (as amended 2026-09-26):
// on deepseek/deepseek-v4.1-flash, Kilo's first utility default, the gateway
// must accept both reasoning shapes and return reasoning. A 400 is a DRIFT
// failure here, not a skip, so skipIfTransient is deliberately not used. The
// gateway does not validate effort values, so this proves acceptance and that
// reasoning is on, not that {"effort":"low"} changes the effort.
func TestLive_KiloReasoningShapes(t *testing.T) {
	key := kiloKey(t)
	for _, tc := range []struct{ name, effort string }{{"enabled", ""}, {"effort low", effortLow}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := liveCtx(t)
			defer cancel()
			p, err := NewKilo(key, liveModel(t, ProviderKilo, kiloNonTraining...), WithMaxTokens(400),
				WithReasoningEffort(tc.effort))
			if err != nil {
				t.Fatalf("NewKilo: %v", err)
			}
			resp, err := p.GenerateItemsThinking(ctx, MessageItem{Role: jsonRoleUser, Text: "Reply with only the word ALPHA"})
			if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrProviderUnavailable) {
				t.Skipf("gateway transient: %v", err)
			}
			if err != nil {
				t.Fatalf("DRIFT (probed %s): gateway rejected reasoning shape %q: %v", wireShapesProbedOnKilo, tc.name, err)
			}
			sawReasoning := false
			for _, item := range resp.Output {
				if _, ok := item.(ReasoningItem); ok {
					sawReasoning = true
				}
			}
			if !sawReasoning {
				t.Errorf("DRIFT (probed %s): no reasoning returned for shape %q", wireShapesProbedOnKilo, tc.name)
			}
		})
	}
}
