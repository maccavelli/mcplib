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
	"sync"
	"testing"
	"time"
)

// chatGPTCapture records what a stubbed backend received.
type chatGPTCapture struct {
	mu     sync.Mutex
	calls  int
	header http.Header
	body   map[string]any
}

// chatGPTClient answers every request with reply and no Content-Type, as the
// ChatGPT backend sends its event stream (gate G-C, 2026-09-27).
func chatGPTClient(t *testing.T, reply string) (*http.Client, *chatGPTCapture) {
	t.Helper()
	c := &chatGPTCapture{}
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		c.mu.Lock()
		c.calls++
		c.header, c.body = r.Header.Clone(), body
		c.mu.Unlock()
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{},
			Body: io.NopCloser(strings.NewReader(reply)), Request: r}, nil
	})}, c
}

func chatGPTTestSession() *OAuthSession {
	return &OAuthSession{Issuer: DefaultOpenAIIssuer, Access: "sess", Refresh: "refresh", Expiry: time.Now().Add(time.Hour)}
}

func chatGPTFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestOpenAIChatGPT_SendsCodexRequest: the backend rejects a non-streaming
// request, one without store:false and one with max_output_tokens (gate G-C);
// Codex also sends include, prompt_cache_key and session-id.
func TestOpenAIChatGPT_SendsCodexRequest(t *testing.T) {
	client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
	p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra",
		WithHTTPClient(client), WithMaxTokens(321), WithSessionID("sess-42"))
	if err != nil {
		t.Fatal(err)
	}
	_, genErr := p.Generate(context.Background(), "hello")
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.body["stream"] != true || c.body["store"] != false || c.body["prompt_cache_key"] != "sess-42" {
		t.Errorf("stream/store/prompt_cache_key = %v/%v/%v, want true/false/sess-42",
			c.body["stream"], c.body["store"], c.body["prompt_cache_key"])
	}
	if include, _ := c.body["include"].([]any); !slices.Equal(include, []any{"reasoning.encrypted_content"}) {
		t.Errorf("include = %v", c.body["include"])
	}
	if v, ok := c.body["max_output_tokens"]; ok {
		t.Errorf("max_output_tokens = %v, want absent", v)
	}
	if c.header.Get("Accept") != "text/event-stream" || c.header.Get("session-id") != "sess-42" {
		t.Errorf("Accept = %q, session-id = %q", c.header.Get("Accept"), c.header.Get("session-id"))
	}
	if genErr != nil {
		t.Errorf("Generate: %v", genErr)
	}
}

// TestOpenAIChatGPT_DecodesRecordedStreams replays gate G-C's recordings
// (2026-09-27, gpt-6-astra; account identifiers redacted).
func TestOpenAIChatGPT_DecodesRecordedStreams(t *testing.T) {
	for _, tc := range []struct {
		fixture, id, text string
		call              *FunctionCallItem
	}{
		{"chatgpt-text.sse", "resp_01ae07221376cd90016ab96e72ca3c87d1a3c12bb5aaf6db92", "ok", nil},
		{"chatgpt-reasoning.sse", "resp_047ea2fee4cf7ab6016ab96e77a3f087d1ad70bca0c3b0cd8d", "391", nil},
		{"chatgpt-tool.sse", "resp_059e69aebb680141016ab96e74543487d1af68b2af975de82f", "",
			&FunctionCallItem{CallID: "call_SAcT3cGrAgagMgcNJEcgK62C", Name: "record", Arguments: `{"subject":"fix: probe"}`}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			client, _ := chatGPTClient(t, chatGPTFixture(t, tc.fixture))
			p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client))
			if err != nil {
				t.Fatal(err)
			}
			res, err := p.GenerateItems(context.Background(), MessageItem{Role: jsonRoleUser, Text: "hi"})
			if err != nil {
				t.Fatalf("GenerateItems: %v", err)
			}
			if res.ID != tc.id || res.OutputText() != tc.text {
				t.Errorf("ID = %q, text = %q; want %q, %q", res.ID, res.OutputText(), tc.id, tc.text)
			}
			var calls []FunctionCallItem
			for _, it := range res.Output {
				if fc, ok := it.(FunctionCallItem); ok {
					calls = append(calls, fc)
				}
			}
			if (tc.call == nil) != (len(calls) == 0) || (tc.call != nil && (len(calls) != 1 || calls[0] != *tc.call)) {
				t.Errorf("function calls = %+v, want %+v", calls, tc.call)
			}
		})
	}
}

// sseEvents renders data-only events, as the backend frames them.
func sseEvents(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("data: " + e + "\n\n")
	}
	return b.String()
}

const sseCreated = `{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`

func sseFailed(code string) string {
	return `{"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"` + code +
		`","message":"` + code + ` happened"}}}`
}

// TestOpenAIChatGPT_StreamFailures: response.failed codes map onto §1.1 as
// Codex's parse_failed_response classifies them, response.incomplete onto
// §1.5, and a stream that ends before response.completed is retryable.
func TestOpenAIChatGPT_StreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name, stream string
		want         error
		terminal     bool
	}{
		{"usage_not_included", sseEvents(sseCreated, sseFailed("usage_not_included")), ErrNotPermitted, true},
		{"usage_limit_reached", sseEvents(sseCreated, sseFailed("usage_limit_reached")), ErrQuotaExhausted, true},
		{"context_length_exceeded", sseEvents(sseCreated, sseFailed("context_length_exceeded")), ErrInvalidRequest, true},
		{"rate_limit_exceeded", sseEvents(sseCreated, sseFailed("rate_limit_exceeded")), ErrRateLimited, false},
		{"server_error", sseEvents(sseCreated, sseFailed("server_error")), ErrProviderUnavailable, false},
		{"truncated", sseEvents(sseCreated), ErrProviderUnavailable, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := chatGPTClient(t, tc.stream)
			p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client))
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Generate(context.Background(), "hi")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !errors.Is(tc.want, ErrInvalidRequest) && errors.Is(err, ErrInvalidRequest) {
				t.Errorf("err = %v also matches ErrInvalidRequest", err)
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.Terminal != tc.terminal {
				t.Errorf("Terminal = %t, want %t", apiErr.Terminal, tc.terminal)
			}
		})
	}
	t.Run("incomplete", func(t *testing.T) {
		client, _ := chatGPTClient(t, sseEvents(sseCreated,
			`{"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`))
		p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client))
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Generate(context.Background(), "hi")
		var inc *IncompleteError
		if !errors.As(err, &inc) || inc.Reason != "max_output_tokens" {
			t.Fatalf("err = %v, want *IncompleteError max_output_tokens", err)
		}
	})
}

// TestOpenAIChatGPT_ContinueIsInvalid: with store:false there is no response
// to chain from, so Continue fails without a request (MADR 0012 §4.2).
func TestOpenAIChatGPT_ContinueIsInvalid(t *testing.T) {
	client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
	p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Continue(context.Background(), "resp_prev", MessageItem{Role: jsonRoleUser, Text: "more"})
	c.mu.Lock()
	defer c.mu.Unlock()
	if !errors.Is(err, ErrInvalidRequest) || c.calls != 0 {
		t.Fatalf("err = %v after %d requests, want ErrInvalidRequest and none", err, c.calls)
	}
}

// TestOpenAIPlatform_KeepsJSONRequest: an API key keeps the platform shape:
// max_output_tokens, no stream or store, a JSON response.
func TestOpenAIPlatform_KeepsJSONRequest(t *testing.T) {
	client, c := chatGPTClient(t, `{"id":"resp_p","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)
	p, err := NewOpenAI("sk-test", "gpt-6-luna", WithHTTPClient(client), WithMaxTokens(321))
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Generate(context.Background(), "hi")
	if err != nil || out != "ok" {
		t.Fatalf("Generate = %q, %v", out, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if got, _ := c.body["max_output_tokens"].(float64); got != 321 {
		t.Errorf("max_output_tokens = %v, want 321", c.body["max_output_tokens"])
	}
	for _, k := range []string{"stream", "store", "include", "prompt_cache_key"} {
		if v, ok := c.body[k]; ok {
			t.Errorf("%s = %v, want absent", k, v)
		}
	}
}
