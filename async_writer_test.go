package mcplib

import (
	"io"
	"sync"
	"testing"
)

// TestAsyncWriter_WriteCloseRace is the #1 regression: Write racing Close must
// never panic with "send on closed channel". Run under -race.
func TestAsyncWriter_WriteCloseRace(t *testing.T) {
	aw := NewAsyncWriter(io.Discard, 8)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			for range 200 {
				_, _ = aw.Write([]byte("log line\n"))
			}
		})
	}

	closed := make(chan struct{})
	go func() {
		_ = aw.Close()
		close(closed)
	}()

	wg.Wait()
	<-closed

	// Close is idempotent.
	if err := aw.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
	// Writes after close are dropped gracefully (no panic, no error).
	if _, err := aw.Write([]byte("after close\n")); err != nil {
		t.Fatalf("Write after Close returned error: %v", err)
	}
}
