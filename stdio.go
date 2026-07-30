// Package mcplib provides canonical shared primitives for MCP server infrastructure.
//
// This file implements STD-MCP-CORE-STDIO-001: Graceful Stdio Transport Pipeline.
// It provides the canonical buffered stdio transport stack for all Go-based MCP
// servers, ensuring real-close semantics, mutex-protected flushing, EOF detection,
// and final flush guarantees are applied uniformly across the fleet.
package mcplib

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"syscall"
)

// eofWordRe matches "eof" as a standalone word so a wrapped-as-string EOF
// ("got EOF from reader") is recognized while substrings like "typeof" are not.
var eofWordRe = regexp.MustCompile(`\beof\b`)

// DefaultBufferSize is the mandatory minimum buffer size for stdio transport
// streams per STD-MCP-CORE-STDIO-001 §2 (128KB).
const DefaultBufferSize = 128 * 1024

// StdioPipeline encapsulates the canonical buffered stdio transport required by
// STD-MCP-CORE-STDIO-001. It bundles a 128KB buffered reader with EOF detection
// and real-close semantics, a 128KB mutex-protected buffered writer with
// auto-flushing, and coordinated shutdown guarantees.
type StdioPipeline struct {
	// Reader is an io.ReadCloser wrapping the buffered stdin with EOF detection.
	// Close() on this reader will close the underlying stdin to unblock the SDK's
	// background read goroutine, allowing the process to fully terminate.
	// Pass this directly to mcp.IOTransport.Reader or mcplib.MCPServer.Serve().
	Reader io.ReadCloser

	// Writer is an io.WriteCloser wrapping the buffered stdout with auto-flushing.
	// All writes are serialized through writeMu to prevent torn JSON-RPC frames.
	// Pass this directly to mcp.IOTransport.Writer or mcplib.MCPServer.Serve().
	Writer io.WriteCloser

	// bufferedOut is retained for the final Flush() call before process exit.
	bufferedOut *bufio.Writer

	// writeMu serializes all writes to bufferedOut, including both autoFlusher
	// writes from the SDK and the final pipeline.Flush() on shutdown. This
	// prevents torn JSON-RPC frames when shutdown races with in-flight responses.
	writeMu sync.Mutex
}

// NewStdioPipeline constructs a standards-compliant stdio transport pipeline.
// stdin must be an io.ReadCloser (typically os.Stdin) — its Close() method is
// called during shutdown to unblock the SDK's background reader goroutine.
// stdout is the raw OS writer (typically the real os.Stdout captured before
// redirection). cancel is invoked when EOF is detected on stdin, triggering
// graceful shutdown.
func NewStdioPipeline(stdin io.ReadCloser, stdout io.Writer, cancel context.CancelFunc) *StdioPipeline {
	bufferedIn := bufio.NewReaderSize(stdin, DefaultBufferSize)
	bufferedOut := bufio.NewWriterSize(stdout, DefaultBufferSize)

	p := &StdioPipeline{
		bufferedOut: bufferedOut,
	}

	p.Reader = buildStdioEOFDetector(bufferedIn, stdin, cancel)
	p.Writer = &autoFlusher{w: bufferedOut, mu: &p.writeMu}

	return p
}

// Flush performs the mandatory final flush per STD-MCP-CORE-STDIO-001 §2.
// Must be called after the MCP SDK Run()/Connect() loop completes to ensure
// the final shutdown acknowledgment reaches the IDE before process exit.
// Serialized through writeMu to prevent torn writes if the SDK's write loop
// has not yet fully stopped.
func (p *StdioPipeline) Flush() error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.bufferedOut.Flush()
}

// NewEOFDetector wraps a reader with EOF detection that invokes cancel when
// the upstream pipe is closed. Close() is a no-op for backward compatibility
// with servers that construct their own transport instead of using NewStdioPipeline.
//
// Deprecated: Use NewStdioPipeline for production servers. NewStdioPipeline
// provides real-close semantics that unblock the SDK's background reader goroutine.
func NewEOFDetector(r io.Reader, cancel context.CancelFunc) io.ReadCloser {
	return &eofDetector{r: r, stdin: nil, cancel: cancel}
}

// NewAutoFlusher wraps a writer with immediate flushing after every Write call.
// This ensures zero artificial latency for JSON-RPC delivery over buffered pipes.
// Close() is a no-op to prevent the MCP SDK from closing os.Stdout.
//
// Deprecated: Use NewStdioPipeline for production servers. NewStdioPipeline
// provides mutex-protected flushing that prevents torn JSON-RPC frames.
func NewAutoFlusher(w io.Writer) io.WriteCloser {
	return &autoFlusher{w: w, mu: &sync.Mutex{}}
}

// buildStdioEOFDetector wraps a reader with EOF detection that invokes cancel when
// the upstream pipe is closed. Close() will close the underlying stdin to
// unblock the SDK's background read goroutine.
func buildStdioEOFDetector(r io.Reader, stdin io.Closer, cancel context.CancelFunc) io.ReadCloser {
	return &eofDetector{r: r, stdin: stdin, cancel: cancel}
}

// eofDetector monitors stdin for EOF and cancels the provided context.
type eofDetector struct {
	r      io.Reader
	stdin  io.Closer // raw os.Stdin handle for real close
	cancel context.CancelFunc
	once   sync.Once
}

func (e *eofDetector) Read(p []byte) (n int, err error) {
	n, err = e.r.Read(p)
	if errors.Is(err, io.EOF) {
		e.once.Do(func() {
			slog.Warn("stdin EOF detected; initiating graceful shutdown")
			e.cancel()
		})
	}
	return n, err
}

// Close closes the underlying stdin file descriptor to unblock the SDK's
// background reader goroutine. Without this, the goroutine permanently blocks
// on os.Stdin.Read(), preventing process exit and causing pipe corruption
// when the IDE spawns a replacement process.
func (e *eofDetector) Close() error {
	e.once.Do(func() {
		e.cancel()
	})
	if e.stdin != nil {
		return e.stdin.Close()
	}
	return nil
}

// flusher is satisfied by writers that support explicit buffer flushing
// (e.g. *bufio.Writer).
type flusher interface {
	Flush() error
}

// autoFlusher wraps an io.Writer and immediately flushes the underlying
// buffer after every Write call. All writes are serialized through mu to
// prevent torn JSON-RPC frames when pipeline.Flush() races with SDK writes.
type autoFlusher struct {
	w  io.Writer
	mu *sync.Mutex
}

func (a *autoFlusher) Write(p []byte) (n int, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	n, err = a.w.Write(p)
	if err != nil {
		return n, err
	}
	// Flush after every write so a frame is never left stranded in the buffer
	// (the SDK may not include the trailing newline in the same Write), and
	// surface flush failures (e.g. broken pipe) instead of silently swallowing them.
	if f, ok := a.w.(flusher); ok {
		if flushErr := f.Flush(); flushErr != nil {
			return n, flushErr
		}
	}
	return n, nil
}

// Close is a NopCloser: prevent the MCP SDK from closing os.Stdout.
func (a *autoFlusher) Close() error { return nil }

// IsExpectedShutdownErr reports whether err represents a benign shutdown
// condition such as EOF, broken pipe, or a closed connection. This is the
// canonical classifier shared across all MCP servers.
func IsExpectedShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if eofWordRe.MatchString(msg) {
		return true
	}
	for _, phrase := range []string{
		"broken pipe", "connection reset", "use of closed",
		"file already closed", "bad file descriptor",
		"client is closing", "connection closed",
	} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}
