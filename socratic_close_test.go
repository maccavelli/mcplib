package mcplib_test

import (
	"testing"

	"github.com/maccavelli/mcplib"
)

// TestSocraticClient_CloseIdempotent is the #3 regression: Close must be
// idempotent and leave the client DISCONNECTED.
func TestSocraticClient_CloseIdempotent(t *testing.T) {
	c := mcplib.NewSocraticClient("http://127.0.0.1:0", "test")
	c.Close()
	c.Close() // must not panic
	if got := c.State(); got != "DISCONNECTED" {
		t.Errorf("State after Close: got %q, want DISCONNECTED", got)
	}
}
