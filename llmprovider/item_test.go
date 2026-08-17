package llmprovider

import "testing"

func TestResponse_OutputText(t *testing.T) {
	r := &Response{
		Output: []Item{
			ReasoningItem{Text: "thinking..."},
			MessageItem{Role: "assistant", Text: "hello "},
			FunctionCallItem{CallID: "c1", Name: "fn", Arguments: "{}"},
			MessageItem{Role: "assistant", Text: "world"},
		},
	}
	want := "hello world"
	if got := r.OutputText(); got != want {
		t.Errorf("OutputText() = %q, want %q", got, want)
	}
}

func TestResponse_OutputText_Empty(t *testing.T) {
	r := &Response{Output: []Item{}}
	if got := r.OutputText(); got != "" {
		t.Errorf("OutputText() = %q, want empty", got)
	}

	r2 := &Response{}
	if got := r2.OutputText(); got != "" {
		t.Errorf("OutputText() nil output = %q, want empty", got)
	}
}

func TestResponse_Items(t *testing.T) {
	items := []Item{
		MessageItem{Role: "user", Text: "hi"},
		ReasoningItem{Text: "think"},
		MessageItem{Role: "assistant", Text: "hello"},
	}
	r := &Response{Output: items}

	var collected []Item
	for item := range r.Items() {
		collected = append(collected, item)
	}
	if len(collected) != len(items) {
		t.Fatalf("Items() yielded %d items, want %d", len(collected), len(items))
	}
	for i, item := range collected {
		if item != items[i] {
			t.Errorf("Items()[%d] = %v, want %v", i, item, items[i])
		}
	}
}

func TestResponse_Items_EarlyBreak(t *testing.T) {
	items := []Item{
		MessageItem{Role: "user", Text: "1"},
		MessageItem{Role: "user", Text: "2"},
		MessageItem{Role: "user", Text: "3"},
	}
	r := &Response{Output: items}
	count := 0
	for range r.Items() {
		count++
		if count == 1 {
			break
		}
	}
	if count != 1 {
		t.Errorf("expected 1 iteration before break, got %d", count)
	}
}

// TestItemSealedInterface is a compile-time guarantee that all four concrete
// item types satisfy the sealed Item interface.
func TestItemSealedInterface(t *testing.T) {
	var _ Item = MessageItem{}
	var _ Item = FunctionCallItem{}
	var _ Item = FunctionCallOutputItem{}
	var _ Item = ReasoningItem{}

	if (MessageItem{}).itemKind() != "message" {
		t.Errorf("MessageItem.itemKind() = %q, want 'message'", (MessageItem{}).itemKind())
	}
	if (FunctionCallItem{}).itemKind() != "function_call" {
		t.Errorf("FunctionCallItem.itemKind() = %q, want 'function_call'", (FunctionCallItem{}).itemKind())
	}
	if (FunctionCallOutputItem{}).itemKind() != "function_call_output" {
		t.Errorf("FunctionCallOutputItem.itemKind() = %q, want 'function_call_output'", (FunctionCallOutputItem{}).itemKind())
	}
	if (ReasoningItem{}).itemKind() != "reasoning" {
		t.Errorf("ReasoningItem.itemKind() = %q, want 'reasoning'", (ReasoningItem{}).itemKind())
	}
}
