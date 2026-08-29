package llmprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// chatCompletionsOpts carries the per-gateway variations of a Chat Completions
// request. Zero values omit the corresponding field entirely.
type chatCompletionsOpts struct {
	// Tool, when non-nil, is offered to the model.
	Tool *Tool
	// ForceTool sends tool_choice pinning Tool. Kilo gates tool_choice on the
	// model's supported_parameters, so it is separable from offering the tool.
	ForceTool bool
	// ReasoningEffort, when non-empty, is sent as reasoning_effort. OpenCode's
	// chat route has no portable reasoning parameter and always leaves this "".
	ReasoningEffort string
}

// itemsToChatMessages converts canonical items to OpenAI Chat Completions
// messages. Tool results become role:"tool" messages keyed by tool_call_id,
// which is the Chat Completions equivalent of the Responses API's
// function_call_output item.
func itemsToChatMessages(items []Item) []map[string]any {
	var messages []map[string]any
	for _, item := range items {
		switch v := item.(type) {
		case MessageItem:
			role := v.Role
			if role == "" {
				role = jsonRoleUser
			}
			messages = append(messages, map[string]any{
				jsonKeyRole:    role,
				jsonKeyContent: v.Text,
			})
		case FunctionCallOutputItem:
			messages = append(messages, map[string]any{
				jsonKeyRole:    jsonRoleTool,
				"tool_call_id": v.CallID,
				jsonKeyContent: v.Output,
			})
		}
	}
	return messages
}

// chatCompletionsBody builds an OpenAI Chat Completions request body shared by
// every gateway in this package that speaks the format.
func chatCompletionsBody(model string, maxTokens int, input []Item, o chatCompletionsOpts) map[string]any {
	body := map[string]any{
		jsonKeyModel:     model,
		jsonKeyMessages:  itemsToChatMessages(input),
		jsonKeyMaxTokens: maxTokens,
	}
	if o.Tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			jsonKeyType: jsonKeyFunction,
			jsonKeyFunction: map[string]any{
				jsonKeyName:        o.Tool.Name,
				jsonKeyDescription: o.Tool.Description,
				jsonKeyParameters:  o.Tool.Schema,
			},
		}}
		if o.ForceTool {
			body[jsonKeyToolChoice] = map[string]any{
				jsonKeyType:     jsonKeyFunction,
				jsonKeyFunction: map[string]any{jsonKeyName: o.Tool.Name},
			}
		}
	}
	if o.ReasoningEffort != "" {
		body[jsonKeyReasoningEffort] = o.ReasoningEffort
	}
	return body
}

// decodeChatCompletionsResponse decodes an OpenAI Chat Completions envelope into
// a canonical Response.
//
// Reasoning has two competing vendor spellings, both undocumented, both measured
// 2026-08-28/29:
//
//	message.reasoning_content  — OpenCode Zen (verified on big-pickle)
//	message.reasoning          — Kilo Gateway, the OpenRouter convention
//
// Both are accepted; reasoning_content wins when both are present. Kilo also
// sends message.reasoning_details[] ({type:"reasoning.text", text}), a structured
// restatement of the same trace; it is deliberately NOT decoded, because
// ReasoningItem carries a single Text field (item.go:44-51) and parsing both
// would create two sources of truth for one value.
//
// Absent reasoning is normal, never an error.
func decodeChatCompletionsResponse(body io.Reader) (*Response, error) {
	var raw struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("chat completions: response contained no choices")
	}

	msg := raw.Choices[0].Message
	// The response id is not a resumable conversation handle on any gateway in
	// this package, so it is carried for logging only.
	result := &Response{ID: raw.ID}

	reasoning := msg.ReasoningContent
	if reasoning == "" {
		reasoning = msg.Reasoning
	}
	if strings.TrimSpace(reasoning) != "" {
		result.Output = append(result.Output, ReasoningItem{Text: reasoning})
	}
	if msg.Content != "" {
		role := msg.Role
		if role == "" {
			role = jsonRoleAssistant
		}
		result.Output = append(result.Output, MessageItem{Role: role, Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		result.Output = append(result.Output, FunctionCallItem{
			CallID:    tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	if len(result.Output) == 0 {
		return nil, fmt.Errorf("chat completions: response contained no usable content")
	}
	return result, nil
}
