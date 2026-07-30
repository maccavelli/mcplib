package mcplib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// nopReadCloser wraps an io.Reader with a no-op Close for test use.
type nopReadCloser struct{ io.Reader }

func (nopReadCloser) Close() error { return nil }

func TestNewStdioPipeline(t *testing.T) {
	stdin := nopReadCloser{strings.NewReader("hello")}
	var stdout bytes.Buffer
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	pipeline := NewStdioPipeline(stdin, &stdout, cancel)

	if pipeline.Reader == nil {
		t.Fatal("expected non-nil Reader")
	}
	if pipeline.Writer == nil {
		t.Fatal("expected non-nil Writer")
	}
	if pipeline.bufferedOut == nil {
		t.Fatal("expected non-nil bufferedOut")
	}
}

func TestStdioPipeline_WriteAndFlush(t *testing.T) {
	var stdout bytes.Buffer
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	pipeline := NewStdioPipeline(nopReadCloser{strings.NewReader("")}, &stdout, cancel)

	msg := []byte("{\"jsonrpc\":\"2.0\",\"id\":1}\n")
	n, err := pipeline.Writer.Write(msg)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write returned %d, want %d", n, len(msg))
	}

	// AutoFlusher should have already flushed to stdout
	if stdout.Len() == 0 {
		t.Error("expected auto-flush to propagate data to stdout")
	}

	// Final flush should succeed
	if err := pipeline.Flush(); err != nil {
		t.Errorf("Flush failed: %v", err)
	}
}

func TestEOFDetector_CancelsOnEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Reader that returns EOF immediately
	r := strings.NewReader("")
	detector := buildStdioEOFDetector(r, nopReadCloser{r}, cancel)

	buf := make([]byte, 10)
	_, err := detector.Read(buf)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF, got %v", err)
	}

	// Context should be cancelled
	select {
	case <-ctx.Done():
		// expected
	default:
		t.Error("expected context to be cancelled after EOF")
	}
}

func TestEOFDetector_CancelsOnlyOnce(t *testing.T) {
	cancelCount := 0
	cancel := func() { cancelCount++ }

	r := &multiEOFReader{eofCount: 3}
	detector := buildStdioEOFDetector(r, nopReadCloser{r}, cancel)

	buf := make([]byte, 10)
	for range 3 {
		detector.Read(buf)
	}

	if cancelCount != 1 {
		t.Errorf("expected cancel called once, got %d", cancelCount)
	}
}

func TestEOFDetector_CloseUnblocksRead(t *testing.T) {
	// Verify that Close() calls the underlying stdin Close()
	closeTracker := &closeTrackingReader{}
	detector := buildStdioEOFDetector(closeTracker, closeTracker, func() {})
	if err := detector.Close(); err != nil {
		t.Errorf("expected nil from Close(), got %v", err)
	}
	if !closeTracker.closed {
		t.Error("expected Close() to close the underlying stdin reader")
	}
}

func TestEOFDetector_ReadsData(t *testing.T) {
	data := "hello world"
	r := strings.NewReader(data)
	detector := buildStdioEOFDetector(r, nopReadCloser{r}, func() {})

	buf := make([]byte, 64)
	n, _ := detector.Read(buf)
	if string(buf[:n]) != data {
		t.Errorf("expected %q, got %q", data, string(buf[:n]))
	}
}

func TestAutoFlusher_FlushesOnWrite(t *testing.T) {
	fb := &flushTracker{}
	pipeline := &StdioPipeline{}
	af := &autoFlusher{w: fb, mu: &pipeline.writeMu}

	af.Write([]byte("test\n"))

	if fb.flushCount != 1 {
		t.Errorf("expected 1 flush, got %d", fb.flushCount)
	}
}

func TestAutoFlusher_Close(t *testing.T) {
	pipeline := &StdioPipeline{}
	af := &autoFlusher{w: &bytes.Buffer{}, mu: &pipeline.writeMu}
	if err := af.Close(); err != nil {
		t.Errorf("expected nil from Close(), got %v", err)
	}
}

func TestAutoFlusher_NonFlusher(t *testing.T) {
	// Writer that doesn't implement Flush — should not panic
	var buf bytes.Buffer
	pipeline := &StdioPipeline{}
	af := &autoFlusher{w: &buf, mu: &pipeline.writeMu}

	n, err := af.Write([]byte("data"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 4 {
		t.Errorf("expected n=4, got %d", n)
	}
}

func TestIsExpectedShutdownErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"io.EOF", io.EOF, true},
		{"wrapped EOF", fmt.Errorf("wrap: %w", io.EOF), true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"broken pipe", fmt.Errorf("write: broken pipe"), true},
		{"connection reset", fmt.Errorf("read: connection reset by peer"), true},
		{"use of closed", fmt.Errorf("use of closed network connection"), true},
		{"file already closed", fmt.Errorf("file already closed"), true},
		{"bad file descriptor", fmt.Errorf("bad file descriptor"), true},
		{"client is closing", fmt.Errorf("client is closing"), true},
		{"connection closed", fmt.Errorf("connection closed"), true},
		{"random error", fmt.Errorf("something went wrong"), false},
		{"uppercase EOF", fmt.Errorf("got EOF from reader"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsExpectedShutdownErr(tt.err)
			if got != tt.expected {
				t.Errorf("IsExpectedShutdownErr(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

// --- test helpers ---

type multiEOFReader struct {
	eofCount int
	calls    int
}

func (r *multiEOFReader) Read(p []byte) (int, error) {
	r.calls++
	return 0, io.EOF
}

// closeTrackingReader tracks whether Close() was called.
type closeTrackingReader struct {
	closed bool
}

func (c *closeTrackingReader) Read(p []byte) (int, error) { return 0, io.EOF }
func (c *closeTrackingReader) Close() error               { c.closed = true; return nil }

type flushTracker struct {
	bytes.Buffer
	flushCount int
}

func (f *flushTracker) Flush() error {
	f.flushCount++
	return nil
}
