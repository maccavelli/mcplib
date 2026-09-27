package llmprovider

import "fmt"

// IncompleteError is a response the service cut short: a Responses answer
// with status "incomplete", or a tool call truncated by the token limit (MADR
// 0012 §1.5). It matches ErrInvalidRequest, so the retry helpers do not repeat
// the same request.
type IncompleteError struct {
	// Reason is the service's reason, e.g. "max_output_tokens" or "length".
	Reason string
}

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("%v: response incomplete: %s", ErrInvalidRequest, e.Reason)
}

// Unwrap returns ErrInvalidRequest.
func (e *IncompleteError) Unwrap() error { return ErrInvalidRequest }

// finishReasonLength is the Chat Completions finish_reason for a token-limit
// cut.
const finishReasonLength = "length"
