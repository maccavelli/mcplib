package llmprovider

import (
	"context"
	"testing"
)

// TestWithStore_ResponsesProviders: store is absent unless WithStore sets it,
// on OpenAI API-key mode and Grok; a ChatGPT session always sends false.
func TestWithStore_ResponsesProviders(t *testing.T) {
	const reply = `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`
	store := func(t *testing.T, build func(url string) (Provider, error)) (any, bool) {
		t.Helper()
		var body map[string]any
		srv := captureServer(t, &body, reply)
		p, err := build(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Generate(context.Background(), "hi"); err != nil {
			t.Fatal(err)
		}
		v, ok := body["store"]
		return v, ok
	}
	for _, tc := range []struct {
		name  string
		opts  []ProviderOption
		want  any
		isSet bool
	}{
		{"default", nil, nil, false},
		{"false", []ProviderOption{WithStore(false)}, false, true},
		{"true", []ProviderOption{WithStore(true)}, true, true},
	} {
		t.Run("openai/"+tc.name, func(t *testing.T) {
			v, ok := store(t, func(url string) (Provider, error) {
				return NewOpenAI("sk-test", "gpt-6-luna", append([]ProviderOption{WithBaseURL(url)}, tc.opts...)...)
			})
			if ok != tc.isSet || v != tc.want {
				t.Errorf("store = %v (present %t), want %v (present %t)", v, ok, tc.want, tc.isSet)
			}
		})
		t.Run("grok/"+tc.name, func(t *testing.T) {
			v, ok := store(t, func(url string) (Provider, error) {
				return NewGrok("k", "grok-4.6", append([]ProviderOption{WithBaseURL(url)}, tc.opts...)...)
			})
			if ok != tc.isSet || v != tc.want {
				t.Errorf("store = %v (present %t), want %v (present %t)", v, ok, tc.want, tc.isSet)
			}
		})
	}
	t.Run("chatgpt ignores true", func(t *testing.T) {
		client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
		p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client), WithStore(true))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Generate(context.Background(), "hi"); err != nil {
			t.Fatal(err)
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.body["store"] != false {
			t.Errorf("store = %v, want false", c.body["store"])
		}
	})
}
