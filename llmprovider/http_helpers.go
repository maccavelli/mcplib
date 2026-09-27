package llmprovider

import (
	"encoding/json"
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
		ID     string `json:"id"`
		Output []struct {
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
		} `json:"output"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}

	result := &Response{ID: raw.ID}
	for _, out := range raw.Output {
		switch out.Type {
		case itemTypeMessage:
			var sb strings.Builder
			for _, c := range out.Content {
				if c.Type == "output_text" || c.Type == jsonKeyText {
					sb.WriteString(c.Text)
				}
			}
			if text := sb.String(); text != "" {
				result.Output = append(result.Output, MessageItem{Role: jsonRoleAssistant, Text: text})
			}
		case itemTypeFunctionCall:
			result.Output = append(result.Output, FunctionCallItem{
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
			result.Output = append(result.Output, ReasoningItem{Text: sb.String()})
		}
	}

	return result, nil
}
