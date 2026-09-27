//go:build live_gateways

package llmprovider

import (
	"bytes"
	"os"
	"testing"
)

// TestLive_GrokToolDescription: xAI takes a tool with its description and
// calls it.
func TestLive_GrokToolDescription(t *testing.T) {
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY unset")
	}
	var sent []byte
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewGrok(key, "grok-4.6", WithHTTPClient(xaiRecordingClient(&sent)))
	if err != nil {
		t.Fatal(err)
	}
	tool := Tool{Name: "get_weather", Description: "Get the weather for a city", Schema: map[string]any{
		"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}, "required": []string{"city"}}}
	args, err := p.GenerateWithTool(ctx, "What is the weather in Paris?", tool)
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateWithTool: %v", err)
	}
	if !bytes.Contains(sent, []byte(`"description":"Get the weather for a city"`)) {
		t.Fatalf("request did not send the tool description: %s", sent)
	}
	if !bytes.Contains(bytes.ToLower([]byte(args)), []byte("paris")) {
		t.Errorf("arguments %q", args)
	}
}
