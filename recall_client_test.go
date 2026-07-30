package mcplib

import (
	"context"
	"sync"
	"testing"
)

// TestRecallClient_CloseIdempotent is the #3 regression: Close must be safe to
// call more than once (previously double-closed the telemetry shards → panic).
func TestRecallClient_CloseIdempotent(t *testing.T) {
	c := NewRecallClient("http://127.0.0.1:0", "test")
	c.Close()
	c.Close() // must not panic
}

// TestRecallClient_SaveCloseRace is the #2 regression: SaveToRecallWithNamespace
// racing Close must never panic on a closed channel. Run under -race.
func TestRecallClient_SaveCloseRace(t *testing.T) {
	c := NewRecallClient("http://127.0.0.1:0", "test")

	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for range 30 {
				_ = c.SaveToRecallWithNamespace(context.Background(), "sess", "proj", "ns", map[string]any{"k": n})
			}
		}(i)
	}

	go c.Close()
	wg.Wait()

	// Saves after Close return an error rather than panicking.
	if err := c.SaveToRecallWithNamespace(context.Background(), "s", "p", "ns", nil); err == nil {
		t.Error("expected error from SaveToRecallWithNamespace after Close")
	}
}
