package llmprovider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// closeResponseBody closes an HTTP response body, logging close failures at debug level.
func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		slog.Debug("llmprovider: close response body", "error", err)
	}
}

// decodeResponsesAPIOutput decodes a Responses API JSON body into a Response.
// Shared by providers using the Responses API envelope (OpenAI, Grok).
func decodeResponsesAPIOutput(body io.Reader) (*Response, error) {
	var raw struct {
		ID                string `json:"id"`
		Status            string `json:"status"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Output []responsesOutputItem `json:"output"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}
	// A truncated answer is an error, never an empty success (MADR 0012 §1.5).
	if raw.Status == "incomplete" {
		return nil, incompleteResponse(raw.IncompleteDetails.Reason)
	}

	result := &Response{ID: raw.ID}
	for _, out := range raw.Output {
		result.appendOutput(out)
	}
	return result, nil
}

// incompleteResponse is the §1.5 error for an incomplete Responses answer.
func incompleteResponse(reason string) error {
	if reason == "" {
		reason = "unspecified"
	}
	return &IncompleteError{Reason: reason}
}

// responsesOutputItem is one Responses API output entry.
type responsesOutputItem struct {
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Summary []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"summary"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// appendOutput adds the item one Responses output entry carries, if any.
func (r *Response) appendOutput(out responsesOutputItem) {
	switch out.Type {
	case itemTypeMessage:
		var sb strings.Builder
		for _, c := range out.Content {
			if c.Type == "output_text" || c.Type == jsonKeyText {
				sb.WriteString(c.Text)
			}
		}
		if text := sb.String(); text != "" {
			r.Output = append(r.Output, MessageItem{Role: jsonRoleAssistant, Text: text})
		}
	case itemTypeFunctionCall:
		r.Output = append(r.Output, FunctionCallItem{
			CallID:    out.CallID,
			Name:      out.Name,
			Arguments: out.Arguments,
		})
	case itemTypeReasoning:
		var sb strings.Builder
		for _, s := range out.Summary {
			if s.Type == "summary_text" || s.Type == jsonKeyText {
				sb.WriteString(s.Text)
			}
		}
		r.Output = append(r.Output, ReasoningItem{Text: sb.String()})
	}
}

// responsesStreamLimit bounds one Responses event stream.
const responsesStreamLimit = 16 << 20

// responsesStreamEvent is the part of one Responses stream event the reader
// uses. The type is read from the payload, not the SSE "event:" line.
type responsesStreamEvent struct {
	Type     string              `json:"type"`
	Item     responsesOutputItem `json:"item"`
	Response struct {
		ID    string `json:"id"`
		Error *struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	} `json:"response"`
}

// readResponsesStream reads a Responses API event stream into a Response, as
// Codex does (codex-api/src/sse/responses.rs:343-450): each
// response.output_item.done adds its item, and response.created and
// response.completed carry the id. response.failed maps onto §1.1,
// response.incomplete onto §1.5, and a stream that ends before
// response.completed is retryable (MADR 0012 §4.1). It does not rely on
// Content-Type, which the ChatGPT backend does not send.
func readResponsesStream(provider string, body io.Reader) (*Response, error) {
	scanner := bufio.NewScanner(io.LimitReader(body, responsesStreamLimit))
	scanner.Buffer(make([]byte, 0, 64<<10), responsesStreamLimit)
	result := &Response{}
	var data strings.Builder
	dispatch := func() (done bool, err error) {
		payload := data.String()
		data.Reset()
		if payload == "" || payload == "[DONE]" {
			return false, nil
		}
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return false, fmt.Errorf("%s: decode stream event: %w", provider, err)
		}
		switch event.Type {
		case "response.created":
			result.ID = event.Response.ID
		case "response.output_item.done":
			result.appendOutput(event.Item)
		case "response.failed":
			e := event.Response.Error
			if e == nil {
				return false, streamFailure(provider, "", "", "response.failed event received")
			}
			return false, streamFailure(provider, e.Code, e.Type, e.Message)
		case "response.incomplete":
			return false, incompleteResponse(event.Response.IncompleteDetails.Reason)
		case "response.completed":
			if event.Response.ID != "" {
				result.ID = event.Response.ID
			}
			return true, nil
		}
		return false, nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if done, err := dispatch(); done || err != nil {
				return result, err
			}
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %s: read stream: %w", ErrProviderUnavailable, provider, err)
	}
	if done, err := dispatch(); done || err != nil {
		return result, err
	}
	return nil, fmt.Errorf("%w: %s: stream ended before response.completed", ErrProviderUnavailable, provider)
}
