package llmprovider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// GeminiProvider's wire is the Interactions API, POST {base}/interactions
// (MADR 0014 §1). Every shape below was measured against gemini-3.7-flash on
// 2026-09-27; OpenCode's google route keeps generateContent (gemini.go).
const (
	interactionsPath              = "/interactions"
	interactionStepThought        = "thought"
	interactionStepModelOutput    = "model_output"
	interactionStepUserInput      = "user_input"
	interactionStepFunctionCall   = "function_call"
	interactionStepFunctionResult = "function_result"
	interactionSummariesAuto      = "auto"
)

// interactionTextStep is a user_input or model_output step holding one text.
func interactionTextStep(stepType, text string) map[string]any {
	return map[string]any{jsonKeyType: stepType, jsonKeyContent: []map[string]any{{jsonKeyType: jsonKeyText, jsonKeyText: text}}}
}

// interactionsInput converts items to Interactions steps. System items go to
// system_instruction instead (systemPrompt). A function call is preceded by
// the thought step whose signature Gemini requires to replay it (a replayed
// call without one is refused with 400), carrying the item's Signature or,
// for a call Gemini did not issue, the placeholder it accepts. A result is
// keyed by call_id and named after its call.
func interactionsInput(items []Item) []map[string]any {
	names := map[string]string{}
	for _, item := range items {
		if call, ok := item.(FunctionCallItem); ok {
			names[call.CallID] = call.Name
		}
	}
	var steps []map[string]any
	for _, item := range items {
		switch v := item.(type) {
		case MessageItem:
			switch v.Role {
			case jsonRoleSystem:
				continue
			case "", jsonRoleUser:
				steps = append(steps, interactionTextStep(interactionStepUserInput, v.Text))
			default:
				steps = append(steps, interactionTextStep(interactionStepModelOutput, v.Text))
			}
		case FunctionCallItem:
			signature := v.Signature
			if signature == "" {
				signature = geminiSkipThoughtSignature
			}
			steps = append(steps,
				map[string]any{jsonKeyType: interactionStepThought, "signature": signature},
				map[string]any{jsonKeyType: interactionStepFunctionCall, "id": v.CallID, jsonKeyName: v.Name,
					jsonKeyArguments: toolArguments(v.Arguments)})
		case FunctionCallOutputItem:
			name := names[v.CallID]
			if name == "" {
				name = v.CallID
			}
			steps = append(steps, map[string]any{jsonKeyType: interactionStepFunctionResult, jsonKeyCallID: v.CallID,
				jsonKeyName: name, "result": v.Output})
		}
	}
	return steps
}

// geminiThinkingLevel maps an effort to thinking_level: low, medium or high,
// with xhigh sent as high; anything else is omitted, leaving the model's
// default. gemini-2.5-flash-lite refuses medium and fails on low (measured
// 2026-09-27), so it gets high or nothing.
func geminiThinkingLevel(model, effort string) string {
	level := strings.ToLower(effort)
	switch level {
	case effortXHigh:
		level = effortHigh
	case effortLow, effortMedium, effortHigh:
	default:
		return ""
	}
	if strings.HasPrefix(strings.ToLower(model), "gemini-2.5-flash-lite") && level != effortHigh {
		return ""
	}
	return level
}

// interactionsBody builds one Interactions request (MADR 0014 §1). The API has
// no thinking budget, so WithThinkingBudget does not apply here.
func (p *GeminiProvider) interactionsBody(input []Item, tool *Tool, thinking bool, prevInteractionID string) map[string]any {
	gen := map[string]any{"max_output_tokens": p.maxTokens}
	if thinking {
		// Without it a thought step carries only its signature.
		gen["thinking_summaries"] = interactionSummariesAuto
		if level := geminiThinkingLevel(p.model, p.reasoningEffort); level != "" {
			gen["thinking_level"] = level
		}
	}
	body := map[string]any{
		jsonKeyModel:        p.model,
		jsonKeyInput:        interactionsInput(input),
		"store":             p.store,
		"generation_config": gen,
	}
	if system := systemPrompt(input); system != "" {
		body["system_instruction"] = system
	}
	if tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			jsonKeyType:        jsonKeyFunction,
			jsonKeyName:        tool.Name,
			jsonKeyDescription: tool.Description,
			jsonKeyParameters:  tool.Schema,
		}}
		// tool_choice belongs in generation_config; top level is refused.
		gen["tool_choice"] = map[string]any{"allowed_tools": map[string]any{"mode": "any", "tools": []string{tool.Name}}}
	}
	if prevInteractionID != "" {
		body["previous_interaction_id"] = prevInteractionID
	}
	return body
}

// decodeInteraction maps an Interaction to a Response: a thought step's
// summary becomes a ReasoningItem and its signature the Signature of the
// calls after it; model_output text becomes a MessageItem; function_call a
// FunctionCallItem. incomplete is an *IncompleteError (MADR 0012 §1.5);
// failed and cancelled are retryable failures.
func decodeInteraction(body io.Reader) (*Response, error) {
	var raw struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Steps  []struct {
			Type      string `json:"type"`
			Signature string `json:"signature"`
			Summary   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"summary"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"steps"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("gemini: decode interaction: %w", err)
	}
	switch raw.Status {
	case "completed", "requires_action":
	case statusIncomplete:
		// An Interaction gives no reason (measured at max_output_tokens).
		return nil, &IncompleteError{Reason: statusIncomplete}
	default:
		return nil, fmt.Errorf("%w: gemini interaction %s", ErrProviderUnavailable, raw.Status)
	}
	result := &Response{ID: raw.ID}
	signature := ""
	for _, step := range raw.Steps {
		switch step.Type {
		case interactionStepThought:
			signature = step.Signature
			var sb strings.Builder
			for _, s := range step.Summary {
				sb.WriteString(s.Text)
			}
			if sb.Len() > 0 {
				result.Output = append(result.Output, ReasoningItem{Text: sb.String()})
			}
		case interactionStepModelOutput:
			var sb strings.Builder
			for _, c := range step.Content {
				if c.Type == jsonKeyText {
					sb.WriteString(c.Text)
				}
			}
			if sb.Len() > 0 {
				result.Output = append(result.Output, MessageItem{Role: jsonRoleAssistant, Text: sb.String()})
			}
		case interactionStepFunctionCall:
			// Compact, as the generateContent decoder returns arguments.
			args := "{}"
			if a := bytes.TrimSpace(step.Arguments); len(a) > 0 && string(a) != "null" {
				var buf bytes.Buffer
				if err := json.Compact(&buf, a); err != nil {
					return nil, fmt.Errorf("gemini: function_call arguments: %w", err)
				}
				args = buf.String()
			}
			result.Output = append(result.Output, FunctionCallItem{CallID: step.ID, Name: step.Name,
				Arguments: args, Signature: signature})
		}
	}
	if len(result.Output) == 0 {
		return nil, errors.New("gemini returned no content")
	}
	return result, nil
}
