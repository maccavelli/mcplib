package llmprovider

import (
	"context"
	"strings"
	"testing"
)

// thoughtSummaryResponse is generateContent's shape with includeThoughts
// (gemini-3.7-flash, 2026-09-27): the summary part carries thought: true, a
// boolean, and its text in "text".
const thoughtSummaryResponse = `{"candidates":[{"content":{"parts":[
{"thought":true,"text":"Multiply 17 by 23."},
{"text":"391","thoughtSignature":"sig-abc"}]}}]}`

// TestGeminiDecode_ThoughtSummaryPart: the thought part is reasoning, the
// answer is the only message, and the response decodes (MADR 0014 §3).
func TestGeminiDecode_ThoughtSummaryPart(t *testing.T) {
	res, err := decodeGeminiResponse(strings.NewReader(thoughtSummaryResponse))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Output) != 2 {
		t.Fatalf("output = %#v, want a ReasoningItem and a MessageItem", res.Output)
	}
	if r, ok := res.Output[0].(ReasoningItem); !ok || r.Text != "Multiply 17 by 23." {
		t.Errorf("output[0] = %#v, want the thought summary", res.Output[0])
	}
	if res.OutputText() != "391" {
		t.Errorf("OutputText = %q, want 391", res.OutputText())
	}
}

// geminiWireBodies returns the request body OpenCode's google route sends for
// one call; GeminiProvider speaks the Interactions API (MADR 0014 §1).
func geminiWireBodies(t *testing.T, thinking bool, items ...Item) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	var body map[string]any
	srv := captureServer(t, &body, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
	op, err := NewOpencode(ProviderOpencodeZen, "k", "gemini-3.7-flash", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]ItemThinkingProvider{"opencode-google": op} {
		body = nil
		if thinking {
			_, err = p.GenerateItemsThinking(context.Background(), items...)
		} else {
			_, err = p.(ItemProvider).GenerateItems(context.Background(), items...)
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[name] = body
	}
	return out
}

// TestGeminiThinking_AsksForThoughts: a thinking call sets includeThoughts,
// as OpenCode's client does (transform.ts:1280-1288).
func TestGeminiThinking_AsksForThoughts(t *testing.T) {
	for name, body := range geminiWireBodies(t, true, MessageItem{Role: jsonRoleUser, Text: "hi"}) {
		gen, _ := body["generationConfig"].(map[string]any)
		tc, _ := gen["thinkingConfig"].(map[string]any)
		if tc["includeThoughts"] != true {
			t.Errorf("%s: thinkingConfig = %v, want includeThoughts true", name, tc)
		}
	}
}

// TestGeminiThinking_NotAskedWithoutThinking: a plain call sends no
// thinkingConfig at all.
func TestGeminiThinking_NotAskedWithoutThinking(t *testing.T) {
	for name, body := range geminiWireBodies(t, false, MessageItem{Role: jsonRoleUser, Text: "hi"}) {
		gen, _ := body["generationConfig"].(map[string]any)
		if _, ok := gen["thinkingConfig"]; ok {
			t.Errorf("%s: generationConfig = %v, want no thinkingConfig", name, gen)
		}
	}
}

// TestGemini_SystemInstruction: system items go to systemInstruction and
// never become a model turn.
func TestGemini_SystemInstruction(t *testing.T) {
	bodies := geminiWireBodies(t, false,
		MessageItem{Role: jsonRoleSystem, Text: "Reply in French."},
		MessageItem{Role: jsonRoleSystem, Text: "Be brief."},
		MessageItem{Role: jsonRoleUser, Text: "hello"})
	for name, body := range bodies {
		si, _ := body["systemInstruction"].(map[string]any)
		parts, _ := si["parts"].([]any)
		if len(parts) != 1 || parts[0].(map[string]any)["text"] != "Reply in French.\n\nBe brief." {
			t.Errorf("%s: systemInstruction = %v", name, body["systemInstruction"])
		}
		contents, _ := body["contents"].([]any)
		if len(contents) != 1 || contents[0].(map[string]any)["role"] != "user" {
			t.Errorf("%s: contents = %v, want only the user turn", name, contents)
		}
	}
}

// TestGemini_NoSystemInstructionWithoutSystem: no system item, no field.
func TestGemini_NoSystemInstructionWithoutSystem(t *testing.T) {
	for name, body := range geminiWireBodies(t, false, MessageItem{Role: jsonRoleUser, Text: "hello"}) {
		if v, ok := body["systemInstruction"]; ok {
			t.Errorf("%s: systemInstruction = %v, want absent", name, v)
		}
	}
}
