package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// interactionCapture records the requests a stub Interactions server received.
type interactionCapture struct {
	mu     sync.Mutex
	paths  []string
	bodies []map[string]any
}

func (c *interactionCapture) last() (string, map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.paths) == 0 {
		return "", nil
	}
	return c.paths[len(c.paths)-1], c.bodies[len(c.bodies)-1]
}

// interactionServer answers every request with reply and records it.
func interactionServer(t *testing.T, reply string) (*httptest.Server, *interactionCapture) {
	t.Helper()
	c := &interactionCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		c.mu.Lock()
		c.paths, c.bodies = append(c.paths, r.URL.Path), append(c.bodies, body)
		c.mu.Unlock()
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

const interactionText = `{"id":"v1_int_1","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"ok"}]}]}`

// stepTypes lists the input steps' types.
func stepTypes(body map[string]any) []string {
	var out []string
	steps, _ := body["input"].([]any)
	for _, s := range steps {
		out = append(out, fmt.Sprint(s.(map[string]any)["type"]))
	}
	return out
}

var weatherTool = Tool{Name: "get_weather", Description: "Weather for a city",
	Schema: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}}}

// TestGeminiInteractions_Request: the request shape measured on 2026-09-27
// (MADR 0014 §1): system_instruction, typed steps, a thought step with the
// call's signature before it, a result keyed by call_id, the tool and its
// forced choice in generation_config, and store false.
func TestGeminiInteractions_Request(t *testing.T) {
	srv, c := interactionServer(t, interactionText)
	p, err := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, genErr := p.GenerateItemsWithTool(context.Background(), weatherTool,
		MessageItem{Role: jsonRoleSystem, Text: "Be brief."},
		MessageItem{Role: jsonRoleUser, Text: "weather?"},
		FunctionCallItem{CallID: "call_1", Name: "get_weather", Arguments: `{"city":"Paris"}`, Signature: "sig-1"},
		FunctionCallOutputItem{CallID: "call_1", Output: "sunny"})
	path, body := c.last()
	if path != "/interactions" {
		t.Fatalf("path = %q, want /interactions", path)
	}
	if got := fmt.Sprint(stepTypes(body)); got != "[user_input thought function_call function_result]" {
		t.Errorf("steps = %s", got)
	}
	steps, _ := body["input"].([]any)
	if len(steps) == 4 {
		thought, call, result := steps[1].(map[string]any), steps[2].(map[string]any), steps[3].(map[string]any)
		if thought["signature"] != "sig-1" || call["id"] != "call_1" || fmt.Sprint(call["arguments"]) != "map[city:Paris]" ||
			result["call_id"] != "call_1" || result["name"] != "get_weather" || result["result"] != "sunny" {
			t.Errorf("thought %v, call %v, result %v", thought, call, result)
		}
	}
	gen, _ := body["generation_config"].(map[string]any)
	if body["system_instruction"] != "Be brief." || body["store"] != false || body["model"] != "gemini-3.7-flash" {
		t.Errorf("system_instruction %v, store %v, model %v", body["system_instruction"], body["store"], body["model"])
	}
	if fmt.Sprint(gen["tool_choice"]) != "map[allowed_tools:map[mode:any tools:[get_weather]]]" {
		t.Errorf("tool_choice = %v", gen["tool_choice"])
	}
	tools, _ := body["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["type"] != "function" || tools[0].(map[string]any)["name"] != "get_weather" {
		t.Errorf("tools = %v", body["tools"])
	}
	if genErr != nil {
		t.Errorf("GenerateItemsWithTool: %v", genErr)
	}
}

// TestGeminiInteractions_SyntheticCallCarriesPlaceholder: a call Gemini did not
// issue replays after a thought step with the placeholder signature.
func TestGeminiInteractions_SyntheticCallCarriesPlaceholder(t *testing.T) {
	srv, c := interactionServer(t, interactionText)
	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	_, _ = p.GenerateItems(context.Background(), MessageItem{Role: jsonRoleUser, Text: "q"},
		FunctionCallItem{CallID: "c", Name: "f", Arguments: "{}"}, FunctionCallOutputItem{CallID: "c", Output: "r"})
	_, body := c.last()
	steps, _ := body["input"].([]any)
	if len(steps) < 2 || steps[1].(map[string]any)["signature"] != geminiSkipThoughtSignature {
		t.Fatalf("steps = %v, want a placeholder thought before the call", steps)
	}
}

// TestGeminiInteractions_Response: a thought summary is reasoning, its
// signature rides on the calls after it, arguments are compact JSON, and the
// interaction id is the ID.
func TestGeminiInteractions_Response(t *testing.T) {
	srv, _ := interactionServer(t, `{"id":"v1_int_9","status":"requires_action","steps":[
{"type":"thought","signature":"sig-9","summary":[{"type":"text","text":"Look up the weather."}]},
{"type":"function_call","id":"call_7","name":"get_weather","arguments":{"city": "Paris"}},
{"type":"function_call","id":"call_8","name":"get_weather","arguments":{"city":"Rome"}}]}`)
	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	res, err := p.GenerateItemsWithTool(context.Background(), weatherTool, MessageItem{Role: jsonRoleUser, Text: "q"})
	if err != nil {
		t.Fatalf("GenerateItemsWithTool: %v", err)
	}
	want := []Item{
		ReasoningItem{Text: "Look up the weather."},
		FunctionCallItem{CallID: "call_7", Name: "get_weather", Arguments: `{"city":"Paris"}`, Signature: "sig-9"},
		FunctionCallItem{CallID: "call_8", Name: "get_weather", Arguments: `{"city":"Rome"}`, Signature: "sig-9"},
	}
	if res.ID != "v1_int_9" || fmt.Sprint(res.Output) != fmt.Sprint(want) {
		t.Fatalf("ID %q, output %+v; want v1_int_9, %+v", res.ID, res.Output, want)
	}
}

// TestGeminiInteractions_Thinking: summaries on every thinking call; an effort
// becomes thinking_level, xhigh as high; gemini-2.5-flash-lite takes only high.
func TestGeminiInteractions_Thinking(t *testing.T) {
	for _, tc := range []struct{ model, effort, want string }{
		{"gemini-3.7-flash", "", "<nil>"},
		{"gemini-3.7-flash", effortLow, "low"},
		{"gemini-3.7-flash", effortXHigh, "high"},
		{"gemini-2.5-flash-lite", effortLow, "<nil>"},
		{"gemini-2.5-flash-lite", effortHigh, "high"},
	} {
		srv, c := interactionServer(t, interactionText)
		p, _ := NewGemini(context.Background(), "k", tc.model, WithBaseURL(srv.URL), WithReasoningEffort(tc.effort))
		_, _ = p.GenerateThinking(context.Background(), "hi")
		_, body := c.last()
		gen, _ := body["generation_config"].(map[string]any)
		if gen["thinking_summaries"] != "auto" || fmt.Sprint(gen["thinking_level"]) != tc.want {
			t.Errorf("%s/%s: generation_config = %v, want summaries auto and level %s", tc.model, tc.effort, gen, tc.want)
		}
	}
}

// TestGeminiInteractions_PlainCallSendsNoExtras: no thinking, system or tool
// fields on a plain call.
func TestGeminiInteractions_PlainCallSendsNoExtras(t *testing.T) {
	srv, c := interactionServer(t, interactionText)
	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	_, _ = p.Generate(context.Background(), "hi")
	_, body := c.last()
	gen, _ := body["generation_config"].(map[string]any)
	for _, k := range []string{"thinking_summaries", "thinking_level", "tool_choice"} {
		if v, ok := gen[k]; ok {
			t.Errorf("generation_config.%s = %v, want absent", k, v)
		}
	}
	for _, k := range []string{"system_instruction", "tools", "previous_interaction_id"} {
		if v, ok := body[k]; ok {
			t.Errorf("%s = %v, want absent", k, v)
		}
	}
}

// TestGeminiInteractions_Status: incomplete is an *IncompleteError (as at
// max_output_tokens), failed is retryable.
func TestGeminiInteractions_Status(t *testing.T) {
	srv, _ := interactionServer(t, `{"id":"v1_x","status":"incomplete","steps":[{"type":"model_output","content":[{"type":"text","text":"1, 2, "}]}]}`)
	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	_, err := p.Generate(context.Background(), "count")
	var inc *IncompleteError
	if !errors.As(err, &inc) {
		t.Errorf("incomplete: err = %v, want *IncompleteError", err)
	}
	srv2, _ := interactionServer(t, `{"id":"v1_y","status":"failed","steps":[]}`)
	p2, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv2.URL))
	if _, err := p2.Generate(context.Background(), "hi"); !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("failed: err = %v, want ErrProviderUnavailable", err)
	}
}

// TestGemini_ContinueNeedsStore: without WithStore(true) nothing is stored, so
// Continue fails before any request.
func TestGemini_ContinueNeedsStore(t *testing.T) {
	srv, c := interactionServer(t, interactionText)
	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	_, err := p.Continue(context.Background(), "v1_prev", MessageItem{Role: jsonRoleUser, Text: "more"})
	if path, _ := c.last(); !errors.Is(err, ErrInvalidRequest) || path != "" {
		t.Fatalf("err = %v after request %q, want ErrInvalidRequest and none", err, path)
	}
}

// TestGemini_ContinueChainsWhenStored: with WithStore(true), Continue sends
// only the new items, previous_interaction_id and store true.
func TestGemini_ContinueChainsWhenStored(t *testing.T) {
	srv, c := interactionServer(t, interactionText)
	p, _ := NewGemini(context.Background(), "k", "gemini-3.7-flash", WithBaseURL(srv.URL), WithStore(true))
	res, err := p.Continue(context.Background(), "v1_prev", MessageItem{Role: jsonRoleUser, Text: "more"})
	path, body := c.last()
	if path != "/interactions" || body["previous_interaction_id"] != "v1_prev" || body["store"] != true ||
		fmt.Sprint(stepTypes(body)) != "[user_input]" {
		t.Fatalf("path %q, body %v", path, body)
	}
	if err != nil || res.ID != "v1_int_1" {
		t.Fatalf("Continue = %+v, %v", res, err)
	}
}
