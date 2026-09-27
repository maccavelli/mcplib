package llmprovider

import (
	"strings"
	"testing"
)

// TestItemFidelity_GeminiReplaysSignature: the thoughtSignature Gemini issues
// with a call is kept on the item and sent back with it.
func TestItemFidelity_GeminiReplaysSignature(t *testing.T) {
	res, err := decodeGeminiResponse(strings.NewReader(
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"sig-abc"}]}}]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	call, ok := res.Output[0].(FunctionCallItem)
	if !ok || call.Signature != "sig-abc" {
		t.Fatalf("item = %#v, want Signature sig-abc", res.Output[0])
	}
	contents := geminiItemsToContents([]Item{MessageItem{Role: jsonRoleUser, Text: "weather?"}, call,
		FunctionCallOutputItem{CallID: call.CallID, Output: "sunny"}})
	mustEqualJSON(t, contents[1], `{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"sig-abc"}]}`)
}
