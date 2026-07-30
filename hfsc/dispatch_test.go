package hfsc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeLogSession struct {
	mu   sync.Mutex
	data []string
}

func (f *fakeLogSession) Log(_ context.Context, p *mcp.LoggingMessageParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var s string
	if raw, ok := p.Data.(json.RawMessage); ok {
		_ = json.Unmarshal(raw, &s)
	}
	f.data = append(f.data, s)
	return nil
}

func (f *fakeLogSession) records() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.data...)
}

func hasPrefix(recs []string, prefix string) bool {
	for _, r := range recs {
		if strings.HasPrefix(r, prefix) {
			return true
		}
	}
	return false
}

// errReader yields its data once, then returns a non-EOF error.
type errReader struct {
	data []byte
	done bool
}

func (e *errReader) Read(p []byte) (int, error) {
	if e.done {
		return 0, fmt.Errorf("boom")
	}
	e.done = true
	return copy(p, e.data), nil
}

// TestStreamHeavyPayload_StandaloneCap verifies the non-orchestrator fallback
// rejects an oversized payload instead of reading it unbounded into memory.
func TestStreamHeavyPayload_StandaloneCap(t *testing.T) {
	// session == nil forces the standalone branch regardless of orchestrator env.
	big := strings.NewReader(strings.Repeat("x", 17*1024*1024))
	_, err := StreamHeavyPayload(context.Background(), nil, "f.json", "proj", "model", big)
	if err == nil {
		t.Fatal("expected error for oversized standalone payload")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size-cap error, got %v", err)
	}
}

func TestGenerateSessionID(t *testing.T) {
	id, err := generateSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("session id length: got %d want 32 (%q)", len(id), id)
	}
}

// TestStream_AbortOnReadFault is part of #8: a mid-stream read fault must emit
// HFSC_ABORT (not HFSC_FINALIZE) so the peer stops waiting.
func TestStream_AbortOnReadFault(t *testing.T) {
	sess := &fakeLogSession{}
	ctx, cancel := context.WithCancel(context.Background())
	executeContinuousStream(ctx, cancel, sess, "sid", &errReader{data: []byte("hello")})

	recs := sess.records()
	if !hasPrefix(recs, "HFSC_ABORT|") {
		t.Errorf("expected HFSC_ABORT, got %v", recs)
	}
	if hasPrefix(recs, "HFSC_FINALIZE|") {
		t.Errorf("must not finalize a faulted stream, got %v", recs)
	}
}

// TestStream_AbortOnCancel is part of #8: a cancelled context terminates the
// goroutine promptly and emits HFSC_ABORT.
func TestStream_AbortOnCancel(t *testing.T) {
	sess := &fakeLogSession{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	executeContinuousStream(ctx, cancel, sess, "sid", strings.NewReader(strings.Repeat("x", 1<<16)))

	if !hasPrefix(sess.records(), "HFSC_ABORT|") {
		t.Errorf("expected HFSC_ABORT on cancellation, got %v", sess.records())
	}
}
