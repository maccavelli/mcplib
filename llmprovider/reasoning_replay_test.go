package llmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// replayServer serves a metadata document declaring interleaved for one Go
// model, and records the chat request body.
func replayServer(t *testing.T, interleaved string) (*httptest.Server, *map[string]any) {
	t.Helper()
	t.Setenv(envDisableModelMetadata, "0") // TestMain turns the fetch off
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api.json") {
			_, _ = w.Write([]byte(`{"opencode-go":{"models":{"kimi-k2.6":{"id":"kimi-k2.6"` + interleaved + `}}}}`))
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"sunny"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &body
}

func replayConversation() []Item {
	return []Item{
		MessageItem{Role: jsonRoleUser, Text: "weather?"},
		ReasoningItem{Text: "Call the tool."},
		FunctionCallItem{CallID: "call_1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
		FunctionCallOutputItem{CallID: "call_1", Output: "sunny"},
		MessageItem{Role: jsonRoleAssistant, Text: "It is sunny."},
		MessageItem{Role: jsonRoleUser, Text: "thanks"},
	}
}

// TestOpencodeChat_ReplaysInterleavedReasoning: the declared field is set on
// every assistant message, with the reasoning that preceded it or "" (as
// OpenCode's client does, transform.ts:321-349).
func TestOpencodeChat_ReplaysInterleavedReasoning(t *testing.T) {
	srv, body := replayServer(t, `,"interleaved":{"field":"reasoning_content"}`)
	p, err := NewOpencode(ProviderOpencodeGo, "k", "kimi-k2.6", WithBaseURL(srv.URL),
		WithModelMetadataURL(srv.URL+"/api.json"), WithOpencodeRoute(OpencodeRouteChatCompletions))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	if _, err := p.GenerateItems(context.Background(), replayConversation()...); err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	var assistants []any
	for _, m := range (*body)[jsonKeyMessages].([]any) {
		if msg := m.(map[string]any); msg[jsonKeyRole] == jsonRoleAssistant {
			assistants = append(assistants, msg["reasoning_content"])
		}
	}
	if len(assistants) != 2 || assistants[0] != "Call the tool." || assistants[1] != "" {
		t.Fatalf("assistant reasoning_content = %#v, want [\"Call the tool.\" \"\"]", assistants)
	}
}

// TestOpencodeChat_NoReplayWithoutInterleaved: a model the metadata does not
// mark interleaved gets no reasoning field.
func TestOpencodeChat_NoReplayWithoutInterleaved(t *testing.T) {
	srv, body := replayServer(t, "")
	p, err := NewOpencode(ProviderOpencodeGo, "k", "kimi-k2.6", WithBaseURL(srv.URL),
		WithModelMetadataURL(srv.URL+"/api.json"), WithOpencodeRoute(OpencodeRouteChatCompletions))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	if _, err := p.GenerateItems(context.Background(), replayConversation()...); err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	for _, m := range (*body)[jsonKeyMessages].([]any) {
		if _, ok := m.(map[string]any)["reasoning_content"]; ok {
			t.Fatalf("message %v carries reasoning_content for a non-interleaved model", m)
		}
	}
}
