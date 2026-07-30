package mcplib

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mcplogging "github.com/maccavelli/mcplib/logging"
)

// AsyncWriter provides non-blocking, buffered writes to an underlying io.Writer (usually Stderr).
// This is critical for bastion environments where blocking on SSH stderr can stall the main process.
type AsyncWriter struct {
	writer      io.Writer
	ch          chan []byte
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	dropped     atomic.Int64
	maxDuration time.Duration
	closed      atomic.Bool
}

// NewAsyncWriter creates a new AsyncWriter with the given channel capacity.
func NewAsyncWriter(w io.Writer, capacity int) *AsyncWriter {
	ctx, cancel := context.WithCancel(context.Background())
	aw := &AsyncWriter{
		writer:      w,
		ch:          make(chan []byte, capacity),
		ctx:         ctx,
		cancel:      cancel,
		maxDuration: 100 * time.Millisecond,
	}
	aw.wg.Add(1)
	go aw.worker(ctx)
	return aw
}

func (aw *AsyncWriter) worker(ctx context.Context) {
	defer aw.wg.Done()
	for {
		select {
		case <-ctx.Done():
			// Drain any buffered items best-effort, then exit. The channel is
			// never closed (see Close), so concurrent senders can never panic.
			for {
				select {
				case p := <-aw.ch:
					if _, err := aw.writer.Write(p); err != nil {
						aw.dropped.Add(1)
					}
				default:
					return
				}
			}
		case p := <-aw.ch:
			if _, err := aw.writer.Write(p); err != nil {
				aw.dropped.Add(1)
			}
		}
	}
}

// Write buffers data to the underlying channel or drops it if max duration is reached.
func (aw *AsyncWriter) Write(p []byte) (n int, err error) {
	if aw.closed.Load() {
		return len(p), nil
	}
	// Copy buffer to avoid race conditions with Caller-managed buffers
	data := make([]byte, len(p))
	copy(data, p)

	timer := time.NewTimer(aw.maxDuration)
	defer timer.Stop()

	select {
	case aw.ch <- data:
		return len(p), nil
	case <-timer.C:
		aw.dropped.Add(1)
		return len(p), nil // Dropping logs is better than blocking the main task on a bastion
	case <-aw.ctx.Done():
		// Shutdown in progress: drop gracefully rather than surfacing an error
		// to the slog handler chain.
		return len(p), nil
	}
}

// Close signals the worker (via context cancellation) to drain and finish, then
// waits for it. The data channel is intentionally NOT closed: doing so would
// race concurrent Write callers and panic with "send on closed channel".
func (aw *AsyncWriter) Close() error {
	if aw.closed.Swap(true) {
		return nil // idempotent: already closed
	}
	aw.cancel()  // Signal worker to drain buffered items and exit
	aw.wg.Wait() // Wait for worker to finish draining
	return nil
}

// Dropped returns the number of log writes dropped due to backpressure or
// downstream write errors over the writer's lifetime.
func (aw *AsyncWriter) Dropped() int64 {
	return aw.dropped.Load()
}

// OpenHardenedLogFile opens a file with a 50MB safety cap for Bastion environments.
// If the file exceeds 50MB, it is truncated to 0.
func OpenHardenedLogFile(path string) *os.File {
	const maxLogSize = 50 * 1024 * 1024 // 50MB
	//nolint:gosec // G703: path is server-controlled log location, not untrusted user input
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		if err := os.Truncate(path, 0); err != nil {
			_, _ = os.Stderr.WriteString("mcp-server: failed to truncate log file " + path + ": " + err.Error() + "\n") //nolint:errcheck // stderr fallback during log setup
		}
	}
	//nolint:gosec // G304: path is server-controlled log location from SetupStandardLogging
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return os.Stderr
	}
	return f
}

// SetupStandardLogging configures a non-blocking JSON logger for the bastion host.
// It ensures that no MCP server logs to Stdout, keeping stderr clean for JSON-RPC.
func SetupStandardLogging(serverName string, buffer io.Writer) func() {
	var writers []io.Writer

	logDir := filepath.Join(os.TempDir(), "mcp-server-"+serverName)
	if err := os.MkdirAll(logDir, 0700); err != nil {
		slog.Error("failed to create secure log directory", "dir", logDir, "error", err)
		logDir = os.TempDir()
	}
	localLogPath := filepath.Join(logDir, "mcp-subserver-"+serverName+".log")

	localLogFile := OpenHardenedLogFile(localLogPath)

	// On heavy load, we drop logs rather than stall the server connection.
	localAw := NewAsyncWriter(localLogFile, 1024)
	writers = append(writers, localAw)

	var globalLogFile *os.File
	var globalAw *AsyncWriter
	if envPath := os.Getenv("MCP_LOG_FILE"); envPath != "" {
		globalLogFile = OpenHardenedLogFile(envPath)
		globalAw = NewAsyncWriter(globalLogFile, 1024)
		writers = append(writers, globalAw)
	}

	if buffer != nil {
		writers = append(writers, buffer)
	}

	sw := io.MultiWriter(writers...)

	lvl := new(slog.LevelVar)
	lvl.Set(slog.LevelInfo) // Baseline default standalone protection

	if val := os.Getenv("ORCHESTRATOR_LOG_LEVEL"); val != "" {
		switch strings.ToUpper(val) {
		case "DEBUG":
			lvl.Set(slog.LevelDebug)
		case "INFO":
			lvl.Set(slog.LevelInfo)
		case "WARN", "WARNING":
			lvl.Set(slog.LevelWarn)
		case "ERROR", "CRITICAL":
			lvl.Set(slog.LevelError)
		}
	}

	format := os.Getenv("ORCHESTRATOR_LOG_FORMAT")
	if format == "" {
		format = "json"
	}

	var handler slog.Handler
	if strings.EqualFold(format, "text") {
		handler = slog.NewTextHandler(mcplogging.NewSanitizingWriter(sw), &slog.HandlerOptions{Level: lvl})
	} else {
		handler = slog.NewJSONHandler(mcplogging.NewSanitizingWriter(sw), &slog.HandlerOptions{Level: lvl})
	}

	slog.SetDefault(slog.New(handler).With("server", serverName))

	// SHUTDOWN ORDERING INVARIANT: This cleanup function closes the AsyncWriter
	// channels. Any component that writes to slog (e.g., RecallClient.Close()
	// telemetry flushes) MUST complete before this cleanup runs. Go's LIFO defer
	// stack naturally enforces this when SetupStandardLogging is deferred before
	// RecallClient initialization in main().
	return func() {
		_ = localAw.Close() //nolint:errcheck // shutdown cleanup; errors are non-actionable
		if localLogFile != nil && localLogFile != os.Stderr {
			_ = localLogFile.Close() //nolint:errcheck // shutdown cleanup; errors are non-actionable
		}
		if globalAw != nil {
			_ = globalAw.Close() //nolint:errcheck // shutdown cleanup; errors are non-actionable
		}
		if globalLogFile != nil && globalLogFile != os.Stderr {
			_ = globalLogFile.Close() //nolint:errcheck // shutdown cleanup; errors are non-actionable
		}
		// Surface silent backpressure loss. Written to stderr (not slog) since the
		// slog sink is being torn down here.
		dropped := localAw.Dropped()
		if globalAw != nil {
			dropped += globalAw.Dropped()
		}
		if dropped > 0 {
			fmt.Fprintf(os.Stderr, "mcp-server: %d log lines dropped (backpressure)\n", dropped)
		}
	}
}
