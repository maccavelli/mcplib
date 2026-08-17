package llmprovider

import (
	"context"
	"iter"
	"strings"
)

// Item is a sealed interface representing a single typed element in an LLM
// response or input. Only types defined in this package may implement Item;
// the unexported marker method itemKind() prevents external implementations.
//
// Callers switch on concrete types: MessageItem, FunctionCallItem,
// FunctionCallOutputItem, ReasoningItem.
type Item interface {
	itemKind() string // sealed marker — unexported, prevents external implementation
}

// MessageItem represents a text message from a role (typically "assistant").
type MessageItem struct {
	Role string // "user", "assistant", "system"
	Text string
}

func (MessageItem) itemKind() string { return itemTypeMessage }

// FunctionCallItem represents the model requesting a function/tool call.
type FunctionCallItem struct {
	CallID    string // provider-issued call identifier
	Name      string // function name
	Arguments string // JSON-encoded arguments
}

func (FunctionCallItem) itemKind() string { return itemTypeFunctionCall }

// FunctionCallOutputItem represents the caller's response to a function call.
type FunctionCallOutputItem struct {
	CallID string // matches FunctionCallItem.CallID
	Output string // the function's return value (typically JSON)
}

func (FunctionCallOutputItem) itemKind() string { return itemTypeFunctionCallOutput }

// ReasoningItem represents a model's internal reasoning/thinking trace.
// Not all providers or models emit this; when absent, the Output slice
// simply contains no ReasoningItem values.
type ReasoningItem struct {
	Text string
}

func (ReasoningItem) itemKind() string { return itemTypeReasoning }

// Response is the canonical result of an item-based generation call.
type Response struct {
	// ID is the provider-issued response identifier, used for server-side
	// conversation chaining (OpenAI/xAI: response ID, Gemini: interaction ID).
	// Empty for providers that do not support server-side state (Claude).
	ID string

	// Output contains the typed items returned by the model.
	Output []Item
}

// OutputText returns the concatenation of all MessageItem texts in Output,
// mirroring the OpenAI/xAI SDK's output_text convenience accessor.
// Returns "" if no MessageItem is present.
func (r *Response) OutputText() string {
	var sb strings.Builder
	for _, item := range r.Output {
		if m, ok := item.(MessageItem); ok {
			sb.WriteString(m.Text)
		}
	}
	return sb.String()
}

// Items returns an iterator over r.Output for use with range-over-func.
func (r *Response) Items() iter.Seq[Item] {
	return func(yield func(Item) bool) {
		for _, item := range r.Output {
			if !yield(item) {
				return
			}
		}
	}
}

// ItemProvider is an optional interface for providers that support the
// canonical item-based generation contract.
type ItemProvider interface {
	Provider
	GenerateItems(ctx context.Context, input ...Item) (*Response, error)
}

// ItemToolProvider extends ItemProvider with tool-aware generation.
type ItemToolProvider interface {
	ItemProvider
	GenerateItemsWithTool(ctx context.Context, tool Tool, input ...Item) (*Response, error)
}

// ItemThinkingProvider extends ItemProvider with extended thinking/reasoning.
type ItemThinkingProvider interface {
	ItemProvider
	GenerateItemsThinking(ctx context.Context, input ...Item) (*Response, error)
}

// ItemThinkingToolProvider combines tool calling and extended thinking.
type ItemThinkingToolProvider interface {
	ItemToolProvider
	GenerateItemsWithToolThinking(ctx context.Context, tool Tool, input ...Item) (*Response, error)
}

// Continuer is an optional interface for providers that support server-side
// conversation chaining via a previous response ID.
type Continuer interface {
	Continue(ctx context.Context, previousResponseID string, input ...Item) (*Response, error)
}
