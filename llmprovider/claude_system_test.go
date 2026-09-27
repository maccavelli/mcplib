package llmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func systemConversation() []Item {
	return []Item{
		MessageItem{Role: "system", Text: "Always answer in French."},
		MessageItem{Role: "system", Text: "Be brief."},
		MessageItem{Role: jsonRoleUser, Text: "Say hello."},
	}
}

// TestClaude_SystemMessageIsTopLevel: system items are joined into the
// request's system field and never sent as an assistant turn (MADR 0012 §2).
func TestClaude_SystemMessageIsTopLevel(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Bonjour"}]}`))
	}))
	t.Cleanup(srv.Close)
	p, err := NewClaude("k", "claude-haiku-4-5", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClaude: %v", err)
	}
	if _, err := p.GenerateItems(context.Background(), systemConversation()...); err != nil {
		t.Fatalf("GenerateItems: %v", err)
	}
	if body["system"] != "Always answer in French.\n\nBe brief." {
		t.Errorf("system = %#v, want the two system items joined", body["system"])
	}
	mustEqualJSON(t, body[jsonKeyMessages], `[{"role":"user","content":"Say hello."}]`)
}

func TestOpencodeMessages_SystemMessageIsTopLevel(t *testing.T) {
	p, err := NewOpencode(ProviderOpencodeGo, "k", "qwen3.8-flash", WithOpencodeRoute(OpencodeRouteMessages))
	if err != nil {
		t.Fatalf("NewOpencode: %v", err)
	}
	body := p.messagesBody(systemConversation(), nil, false)
	if body["system"] != "Always answer in French.\n\nBe brief." {
		t.Errorf("system = %#v, want the two system items joined", body["system"])
	}
	mustEqualJSON(t, body[jsonKeyMessages], `[{"role":"user","content":"Say hello."}]`)
}
