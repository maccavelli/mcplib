package llmprovider

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Fixtures below are real bodies measured against the live gateways on
// 2026-08-28/29, not invented shapes.

// measured on OpenCode Zen, model big-pickle: reasoning arrives as
// message.reasoning_content.
const fixtureOpencodeReasoningContent = `{"id":"router-5a84a0cf","object":"chat.completion","created":1787968292,
"model":"big-pickle","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant",
"content":"Hi!","reasoning_content":"We need answer to user. They said \"say hi\".","tool_calls":null}}],
"usage":{"prompt_tokens":85,"completion_tokens":22,"total_tokens":107},"cost":"0"}`

// measured on Kilo Gateway, model kilo-auto/free: reasoning arrives as
// message.reasoning, plus a structured reasoning_details[] restatement.
const fixtureKiloReasoning = `{"id":"gen_01M16Q","object":"chat.completion","created":1788005835,
"model":"stepfun/step-3.7-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ALPHA",
"reasoning":"Got it, the user said to say ALPHA only.",
"reasoning_details":[{"type":"reasoning.text","text":"Got it, the user said to say ALPHA only."}]}}]}`

func TestDecodeChatCompletions_Message(t *testing.T) {
	body := `{"id":"cmpl-1","choices":[{"message":{"role":"assistant","content":"hello world"}}]}`
	resp, err := decodeChatCompletionsResponse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != "cmpl-1" {
		t.Errorf("ID = %q, want cmpl-1", resp.ID)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Output))
	}
	if _, ok := resp.Output[0].(MessageItem); !ok {
		t.Errorf("output[0] = %T, want MessageItem", resp.Output[0])
	}
	if got := resp.OutputText(); got != "hello world" {
		t.Errorf("OutputText() = %q", got)
	}
}

// TestDecodeChatCompletions_ReasoningFieldNames is the core of the shared
// decoder: two gateways spell reasoning differently and both must decode.
func TestDecodeChatCompletions_ReasoningFieldNames(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantReasoning string // "" means no ReasoningItem expected
		wantText      string
	}{
		{"reasoning_content only (OpenCode big-pickle, measured)",
			fixtureOpencodeReasoningContent,
			`We need answer to user. They said "say hi".`, "Hi!"},
		{"reasoning only (Kilo kilo-auto/free, measured)",
			fixtureKiloReasoning,
			"Got it, the user said to say ALPHA only.", "ALPHA"},
		{"both present: reasoning_content wins",
			`{"choices":[{"message":{"role":"assistant","content":"x","reasoning_content":"RC","reasoning":"R"}}]}`,
			"RC", "x"},
		{"reasoning_details only, no string field: not decoded",
			`{"choices":[{"message":{"role":"assistant","content":"x","reasoning_details":[{"type":"reasoning.text","text":"ignored"}]}}]}`,
			"", "x"},
		{"neither: absent reasoning is normal",
			`{"choices":[{"message":{"role":"assistant","content":"x"}}]}`,
			"", "x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := decodeChatCompletionsResponse(strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			var reasoning []ReasoningItem
			for _, it := range resp.Output {
				if r, ok := it.(ReasoningItem); ok {
					reasoning = append(reasoning, r)
				}
			}
			if tc.wantReasoning == "" {
				if len(reasoning) != 0 {
					t.Errorf("expected no ReasoningItem, got %d: %q", len(reasoning), reasoning[0].Text)
				}
			} else {
				if len(reasoning) != 1 {
					t.Fatalf("expected exactly 1 ReasoningItem, got %d", len(reasoning))
				}
				if reasoning[0].Text != tc.wantReasoning {
					t.Errorf("reasoning = %q, want %q", reasoning[0].Text, tc.wantReasoning)
				}
				// Reasoning precedes the message, matching the Responses API order.
				if _, ok := resp.Output[0].(ReasoningItem); !ok {
					t.Errorf("output[0] = %T, want ReasoningItem first", resp.Output[0])
				}
			}
			if got := resp.OutputText(); got != tc.wantText {
				t.Errorf("OutputText() = %q, want %q", got, tc.wantText)
			}
		})
	}
}

// TestDecodeChatCompletions_ToolCalls uses the tool_calls shape measured on Kilo.
func TestDecodeChatCompletions_ToolCalls(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[
	{"id":"chatcmpl-tool-8c37b719","type":"function","function":{"name":"get_weather","arguments":"{\"city\": \"Paris\"}"}}]}}]}`
	resp, err := decodeChatCompletionsResponse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Output))
	}
	fc, ok := resp.Output[0].(FunctionCallItem)
	if !ok {
		t.Fatalf("output[0] = %T, want FunctionCallItem", resp.Output[0])
	}
	if fc.CallID != "chatcmpl-tool-8c37b719" || fc.Name != "get_weather" {
		t.Errorf("call = %+v", fc)
	}
	if fc.Arguments != `{"city": "Paris"}` {
		t.Errorf("arguments = %q", fc.Arguments)
	}
}

func TestDecodeChatCompletions_Empty(t *testing.T) {
	if _, err := decodeChatCompletionsResponse(strings.NewReader(`{"choices":[]}`)); err == nil {
		t.Error("expected error for no choices")
	}
	empty := `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":null}}]}`
	if _, err := decodeChatCompletionsResponse(strings.NewReader(empty)); err == nil {
		t.Error("expected error when content, reasoning and tool_calls are all empty")
	}
	if _, err := decodeChatCompletionsResponse(strings.NewReader(`not json`)); err == nil {
		t.Error("expected error for malformed json")
	}
}

func TestItemsToChatMessages(t *testing.T) {
	msgs := itemsToChatMessages([]Item{
		MessageItem{Text: "no role"},
		MessageItem{Role: jsonRoleAssistant, Text: "assistant text"},
		FunctionCallOutputItem{CallID: "call_1", Output: `{"ok":true}`},
		FunctionCallItem{CallID: "c", Name: "n", Arguments: "{}"}, // unhandled: skipped
	})
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0][jsonKeyRole] != jsonRoleUser {
		t.Errorf("empty role should default to user, got %v", msgs[0][jsonKeyRole])
	}
	if msgs[1][jsonKeyRole] != jsonRoleAssistant {
		t.Errorf("assistant role not preserved: %v", msgs[1][jsonKeyRole])
	}
	if msgs[2][jsonKeyRole] != jsonRoleTool || msgs[2]["tool_call_id"] != "call_1" {
		t.Errorf("tool result message = %v", msgs[2])
	}
}

func TestChatCompletionsBody(t *testing.T) {
	tool := &Tool{Name: "get_weather", Description: "Get weather", Schema: map[string]any{"type": "object"}}
	input := []Item{MessageItem{Role: jsonRoleUser, Text: "hi"}}

	t.Run("no tool, no reasoning", func(t *testing.T) {
		b := chatCompletionsBody("big-pickle", 128, input, chatCompletionsOpts{})
		if _, ok := b[jsonKeyTools]; ok {
			t.Error("tools must be absent")
		}
		if _, ok := b[jsonKeyToolChoice]; ok {
			t.Error("tool_choice must be absent")
		}
		if _, ok := b[jsonKeyReasoningEffort]; ok {
			t.Error("reasoning_effort must be absent")
		}
		if b[jsonKeyMaxTokens] != 128 || b[jsonKeyModel] != "big-pickle" {
			t.Errorf("body = %v", b)
		}
	})

	t.Run("tool without ForceTool omits tool_choice", func(t *testing.T) {
		b := chatCompletionsBody("kilo-auto/free", 1, input, chatCompletionsOpts{Tool: tool})
		if _, ok := b[jsonKeyTools]; !ok {
			t.Error("tools must be present")
		}
		if _, ok := b[jsonKeyToolChoice]; ok {
			t.Error("tool_choice must be absent when ForceTool is false")
		}
	})

	t.Run("tool with ForceTool sends both", func(t *testing.T) {
		b := chatCompletionsBody("openai/gpt-oss-20b", 1, input, chatCompletionsOpts{Tool: tool, ForceTool: true})
		if _, ok := b[jsonKeyTools]; !ok {
			t.Error("tools must be present")
		}
		tc, ok := b[jsonKeyToolChoice].(map[string]any)
		if !ok {
			t.Fatalf("tool_choice = %v", b[jsonKeyToolChoice])
		}
		fn, ok := tc[jsonKeyFunction].(map[string]any)
		if !ok || fn[jsonKeyName] != "get_weather" {
			t.Errorf("tool_choice.function = %v", tc[jsonKeyFunction])
		}
	})

	t.Run("reasoning effort passthrough", func(t *testing.T) {
		b := chatCompletionsBody("deepseek-v4-pro", 1, input, chatCompletionsOpts{ReasoningEffort: effortXHigh})
		if b[jsonKeyReasoningEffort] != effortXHigh {
			t.Errorf("reasoning_effort = %v, want %q", b[jsonKeyReasoningEffort], effortXHigh)
		}
	})
}

// TestClassifyHTTPStatus pins the status -> sentinel mapping shared by every
// gateway provider, including Kilo's documented 402.
func TestClassifyHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		retryAfter string
		wantErr    error
		wantNil    bool
	}{
		{"200 is success", http.StatusOK, "", nil, true},
		{"429 without Retry-After", http.StatusTooManyRequests, "", ErrRateLimited, false},
		{"429 with Retry-After", http.StatusTooManyRequests, "7", ErrRateLimited, false},
		{"401", http.StatusUnauthorized, "", ErrAuthFailure, false},
		{"403", http.StatusForbidden, "", ErrAuthFailure, false},
		{"402 insufficient balance is non-retryable", http.StatusPaymentRequired, "", ErrInvalidRequest, false},
		{"400", http.StatusBadRequest, "", ErrInvalidRequest, false},
		{"404", http.StatusNotFound, "", ErrInvalidRequest, false},
		{"500", http.StatusInternalServerError, "", ErrProviderUnavailable, false},
		{"503", http.StatusServiceUnavailable, "", ErrProviderUnavailable, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status, Header: http.Header{}}
			if tc.retryAfter != "" {
				resp.Header.Set("Retry-After", tc.retryAfter)
			}
			err := classifyHTTPStatus("gw/route", resp)
			if tc.wantNil {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want wrapping %v", err, tc.wantErr)
			}
			// Every branch names the provider/route so a failure is
			// attributable from the error alone, including 429 since
			// RateLimitError gained a Provider field.
			if !strings.Contains(err.Error(), "gw/route") {
				t.Errorf("error must name the provider/route: %v", err)
			}
			if tc.status == http.StatusTooManyRequests {
				var rl *RateLimitError
				if !errors.As(err, &rl) {
					t.Fatalf("429 must yield *RateLimitError, got %T", err)
				}
				want := time.Duration(0)
				if tc.retryAfter == "7" {
					want = 7 * time.Second
				}
				if rl.RetryAfter != want {
					t.Errorf("RetryAfter = %v, want %v", rl.RetryAfter, want)
				}
			}
		})
	}
}
