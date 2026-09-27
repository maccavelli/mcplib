package llmprovider

import (
	"context"
	"testing"
)

// TestGrok_SendsToolDescription: the tool's description reaches xAI, as the
// OpenAI and OpenCode Responses paths already send it.
func TestGrok_SendsToolDescription(t *testing.T) {
	var body map[string]any
	srv := captureServer(t, &body, `{"id":"r","output":[{"type":"function_call","call_id":"c","name":"get_weather","arguments":"{}"}]}`)
	p, err := NewGrok("k", "grok-4.6", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	tool := Tool{Name: "get_weather", Description: "Get the weather for a city", Schema: map[string]any{"type": "object"}}
	if _, err := p.GenerateWithTool(context.Background(), "weather?", tool); err != nil {
		t.Fatal(err)
	}
	tools, _ := body[jsonKeyTools].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)[jsonKeyDescription] != tool.Description {
		t.Fatalf("tools = %v, want the description", body[jsonKeyTools])
	}
}
