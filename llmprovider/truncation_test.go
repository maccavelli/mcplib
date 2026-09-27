package llmprovider

import (
	"errors"
	"strings"
	"testing"
)

// TestDecodeResponses_IncompleteIsError: a Responses answer the service marked
// incomplete is an error naming the reason, never an empty success (MADR 0012
// §1.5).
func TestDecodeResponses_IncompleteIsError(t *testing.T) {
	body := `{"id":"r1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},
		"output":[{"type":"reasoning","summary":[]}]}`
	res, err := decodeResponsesAPIOutput(strings.NewReader(body))
	if err == nil || !errors.Is(err, ErrInvalidRequest) || !strings.Contains(err.Error(), "max_output_tokens") {
		t.Fatalf("decode = %+v/%v, want an ErrInvalidRequest naming max_output_tokens", res, err)
	}
}

// TestDecodeChat_LengthToolCallIsError: a tool call cut off by the token limit
// has unusable arguments, so it is an error.
func TestDecodeChat_LengthToolCallIsError(t *testing.T) {
	body := `{"id":"c1","choices":[{"finish_reason":"length","message":{"role":"assistant",
		"tool_calls":[{"id":"t1","function":{"name":"commit","arguments":"{\"subject\":\"fix: tru"}}]}}]}`
	res, err := decodeChatCompletionsResponse(strings.NewReader(body))
	if err == nil || !errors.Is(err, ErrInvalidRequest) || !strings.Contains(err.Error(), "length") {
		t.Fatalf("decode = %+v/%v, want an ErrInvalidRequest naming length", res, err)
	}
}
