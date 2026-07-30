package mcplib

import (
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
)

type fakeFlushWriter struct {
	written  int
	flushes  int
	flushErr error
}

func (f *fakeFlushWriter) Write(p []byte) (int, error) { f.written += len(p); return len(p), nil }
func (f *fakeFlushWriter) Flush() error                { f.flushes++; return f.flushErr }

// TestAutoFlusher_FlushesWithoutNewline is the C regression: a write with no
// trailing newline must still flush (not stall in the buffer).
func TestAutoFlusher_FlushesWithoutNewline(t *testing.T) {
	w := &fakeFlushWriter{}
	a := NewAutoFlusher(w)
	if _, err := a.Write([]byte("partial frame no newline")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if w.flushes != 1 {
		t.Errorf("expected 1 flush, got %d", w.flushes)
	}
}

// TestAutoFlusher_FlushErrorPropagates is the C regression: a flush failure must
// surface from Write rather than being swallowed.
func TestAutoFlusher_FlushErrorPropagates(t *testing.T) {
	w := &fakeFlushWriter{flushErr: fmt.Errorf("broken pipe")}
	a := NewAutoFlusher(w)
	if _, err := a.Write([]byte("frame\n")); err == nil {
		t.Error("expected flush error to propagate from Write")
	}
}

// TestIsExpectedShutdownErr_Typed is the D regression: typed network/close errors
// are recognized, and "typeof" no longer false-positives via a bare "eof" match.
func TestIsExpectedShutdownErr_Typed(t *testing.T) {
	if !IsExpectedShutdownErr(net.ErrClosed) {
		t.Error("net.ErrClosed should be an expected shutdown error")
	}
	if !IsExpectedShutdownErr(syscall.EPIPE) {
		t.Error("EPIPE should be an expected shutdown error")
	}
	if !IsExpectedShutdownErr(syscall.ECONNRESET) {
		t.Error("ECONNRESET should be an expected shutdown error")
	}
	if IsExpectedShutdownErr(fmt.Errorf("typeof mismatch in handler")) {
		t.Error(`"typeof" must not be classified as a shutdown error`)
	}
}

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) { return 0, fmt.Errorf("write failed") }

// TestAsyncWriter_Dropped is the E regression: dropped writes are counted and
// observable via Dropped().
func TestAsyncWriter_Dropped(t *testing.T) {
	aw := NewAsyncWriter(errWriter{}, 4)
	for range 10 {
		_, _ = aw.Write([]byte("x"))
	}
	_ = aw.Close() // drains: each failing write increments dropped
	if aw.Dropped() == 0 {
		t.Error("expected Dropped() > 0 with a failing downstream writer")
	}
}

// TestLogBuffer_WithBufferSizeValidation is the F regression: an invalid
// trimTarget is clamped so a trim cannot panic or wipe the buffer.
func TestLogBuffer_WithBufferSizeValidation(t *testing.T) {
	lb := NewLogBuffer(WithBufferSize(100, 200)) // trimTarget >= maxSize: invalid
	if lb.trimTarget <= 0 || lb.trimTarget >= lb.maxSize {
		t.Errorf("trimTarget not clamped: maxSize=%d trimTarget=%d", lb.maxSize, lb.trimTarget)
	}
	// Force a trim — must not panic.
	lb.Write([]byte(strings.Repeat("a", 500)))
	if lb.String() == "" {
		t.Error("buffer unexpectedly empty after write")
	}
}
