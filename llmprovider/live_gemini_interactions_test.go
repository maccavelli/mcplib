//go:build live_gateways

package llmprovider

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestLive_GeminiInteractions: GeminiProvider calls the Interactions API with
// store false and its system instruction is obeyed; with WithStore(true)
// Continue recalls an earlier turn (MADR 0014 §1-§2).
func TestLive_GeminiInteractions(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY unset")
	}
	var paths []string
	var bodies [][]byte
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && r.Body != nil {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			paths, bodies = append(paths, r.URL.Path), append(bodies, b)
			r.Body = io.NopCloser(bytes.NewReader(b))
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	ctx, cancel := liveCtx(t)
	defer cancel()
	plain, err := NewGemini(ctx, key, "gemini-3.7-flash", WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	res, err := plain.GenerateItems(ctx,
		MessageItem{Role: jsonRoleSystem, Text: "Whatever the user says, reply with only the word OMEGA."},
		MessageItem{Role: jsonRoleUser, Text: "Say hello."})
	skipIfTransient(t, err)
	if err != nil || !strings.Contains(strings.ToUpper(res.OutputText()), "OMEGA") {
		t.Fatalf("GenerateItems = %+v, %v; want the system instruction obeyed", res, err)
	}
	if len(paths) != 1 || !strings.HasSuffix(paths[0], "/interactions") || !bytes.Contains(bodies[0], []byte(`"store":false`)) ||
		!bytes.Contains(bodies[0], []byte(`"system_instruction"`)) {
		t.Fatalf("requests %v; want one /interactions call with store false and a system_instruction", paths)
	}

	stored, err := NewGemini(ctx, key, "gemini-3.7-flash", WithHTTPClient(client), WithStore(true))
	if err != nil {
		t.Fatal(err)
	}
	first, err := stored.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: "Remember the number 41. Reply with only OK."})
	skipIfTransient(t, err)
	if err != nil || first.ID == "" {
		t.Fatalf("first = %+v, %v", first, err)
	}
	next, err := stored.Continue(ctx, first.ID, MessageItem{Role: jsonRoleUser, Text: "Which number did I ask you to remember? Reply with only the number."})
	skipIfTransient(t, err)
	if err != nil || !strings.Contains(next.OutputText(), "41") {
		t.Fatalf("Continue = %+v, %v; want 41", next, err)
	}
}

// TestLive_StaticGeminiServed: every StaticGemini id answers on the
// Interactions API (measured 2026-09-27).
func TestLive_StaticGeminiServed(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY unset")
	}
	for _, model := range StaticGemini {
		t.Run(model, func(t *testing.T) {
			ctx, cancel := liveCtx(t)
			defer cancel()
			p, err := NewGemini(ctx, key, model)
			if err != nil {
				t.Fatal(err)
			}
			out, err := p.Generate(ctx, "Reply with only the word ALPHA")
			skipIfTransient(t, err)
			if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
				t.Fatalf("%s: Generate = %q, %v", model, out, err)
			}
		})
	}
}
