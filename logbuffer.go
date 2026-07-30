package mcplib

import (
	"bytes"
	"fmt"
	"sync"

	mcplogging "github.com/maccavelli/mcplib/logging"
)

const (
	// DefaultLogBufferLimit is the maximum size of the log buffer in bytes (2MB).
	DefaultLogBufferLimit = 2 * 1024 * 1024

	// DefaultLogTrimTarget is the target size after trimming in bytes (1MB).
	DefaultLogTrimTarget = 1 * 1024 * 1024
)

// LogBufferOption configures a LogBuffer.
type LogBufferOption func(*LogBuffer)

// WithBufferSize sets custom max size and trim target for the log buffer.
func WithBufferSize(maxSize, trimTarget int) LogBufferOption {
	return func(lb *LogBuffer) {
		lb.maxSize = maxSize
		lb.trimTarget = trimTarget
	}
}

// NewLogBuffer creates a hardened log buffer with secret redaction and
// ring-buffer trimming. Defaults to 2MB max / 1MB trim target.
func NewLogBuffer(opts ...LogBufferOption) *LogBuffer {
	lb := &LogBuffer{
		maxSize:    DefaultLogBufferLimit,
		trimTarget: DefaultLogTrimTarget,
	}
	for _, opt := range opts {
		opt(lb)
	}
	// Validate: trimTarget must be positive and strictly below maxSize, otherwise
	// the ring trim would index negatively (panic) or discard the whole buffer.
	if lb.maxSize <= 0 {
		lb.maxSize = DefaultLogBufferLimit
	}
	if lb.trimTarget <= 0 || lb.trimTarget >= lb.maxSize {
		lb.trimTarget = lb.maxSize / 2
	}
	return lb
}

// LogBuffer is a concurrency-safe, secret-redacting, size-bounded log buffer.
// It implements io.Writer and is used by MCP servers to capture slog output
// for the get_internal_logs tool.
//
// Write() always returns len(p), nil per the io.Writer contract.
// String() uses RLock for read-concurrency.
// Zero-value is usable (defaults applied on first Write).
type LogBuffer struct {
	mu         sync.RWMutex
	buf        bytes.Buffer
	maxSize    int
	trimTarget int
}

// Write implements io.Writer with secret redaction and ring-buffer trimming.
// Returns len(p), nil per the io.Writer contract regardless of redaction.
func (lb *LogBuffer) Write(p []byte) (int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Apply defaults for zero-value usage.
	if lb.maxSize == 0 {
		lb.maxSize = DefaultLogBufferLimit
		lb.trimTarget = DefaultLogTrimTarget
	}

	redacted := mcplogging.Redact(p)
	_, err := lb.buf.Write(redacted)
	if err != nil {
		return len(p), err
	}

	if lb.buf.Len() > lb.maxSize {
		data := lb.buf.Bytes()
		trimPoint := len(data) - lb.trimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		retained := data[trimPoint:]
		lb.buf.Reset()
		lb.buf.WriteString("[TRUNCATED]\n")
		if _, err := lb.buf.Write(retained); err != nil {
			return 0, fmt.Errorf("trim buffer: %w", err)
		}
	}

	return len(p), nil
}

// String returns the full buffer contents.
func (lb *LogBuffer) String() string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.buf.String()
}

// TailLines returns the last n lines of s using a zero-allocation backward scan.
// Trailing newlines are stripped before scanning to return content lines only.
func TailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if s != "" && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if s == "" {
		return ""
	}
	count := 0
	i := len(s)
	for i > 0 {
		i--
		if s[i] == '\n' {
			count++
			if count == n {
				return s[i+1:]
			}
		}
	}
	return s
}
