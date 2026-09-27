package llmprovider

import (
	"errors"
	"strings"
	"testing"
)

// TestDecodeChat_LengthTextKeepsText: a text answer cut by the token limit is
// still returned, and says so in FinishReason.
func TestDecodeChat_LengthTextKeepsText(t *testing.T) {
	body := `{"id":"c1","choices":[{"finish_reason":"length","message":{"role":"assistant","content":"partial ans"}}]}`
	res, err := decodeChatCompletionsResponse(strings.NewReader(body))
	if err != nil || res.OutputText() != "partial ans" || res.FinishReason != finishReasonLength {
		t.Fatalf("decode = %+v/%v, want the text and FinishReason length", res, err)
	}
}

func TestIncompleteError_Reason(t *testing.T) {
	_, err := decodeResponsesAPIOutput(strings.NewReader(
		`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`))
	var incomplete *IncompleteError
	if !errors.As(err, &incomplete) || incomplete.Reason != "max_output_tokens" {
		t.Fatalf("error = %#v, want *IncompleteError with reason max_output_tokens", err)
	}
}
