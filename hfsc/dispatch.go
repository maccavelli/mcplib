// Package hfsc implements the High-Fidelity Smart Chunking selective dispatch
// middleware. It manages the Tier-2 (Extreme Payload) IPC transport, streaming
// geometrically massive payloads natively through session Log notifications
// to strictly bypass the OOM limits of standard JSON-RPC marshalling.
package hfsc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/maccavelli/mcplib"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// streamTimeout bounds the lifetime of a detached HFSC stream goroutine so a
// stalled peer or never-ending reader cannot leak it forever.
const streamTimeout = 10 * time.Minute

// maxAutoInterceptBytes is a hard ceiling for WithAutoIntercept. Beyond this the
// middleware refuses rather than risk OOM (the marshaled payload is already in
// memory — true streaming requires StreamHeavyPayload with a real io.Reader).
const maxAutoInterceptBytes = 64 * 1024 * 1024

const (
	// ReadSize is chosen so Base64 encoding remains efficiently bounded (3000 bytes -> 4000 base64 chars)
	ReadSize = 3000

	hfscLoggerName = "hfsc"
)

// LogSession defines the subset of mcp.ServerSession required for HFSC streaming.
type LogSession interface {
	Log(ctx context.Context, params *mcp.LoggingMessageParams) error
}

// SessionProvider interface to easily fetch the active LogSession.
type SessionProvider interface {
	Session() *mcp.ServerSession
}

// generateSessionID returns a cryptographically random hex session ID. It fails
// closed (returns an error) rather than emitting a predictable all-zero ID.
func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// marshalWireLog encodes a wire payload for HFSC log notifications.
func marshalWireLog(wire string) (json.RawMessage, error) {
	dataBytes, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(dataBytes), nil
}

// emitAbort best-effort notifies the peer that a stream terminated early so it
// stops waiting on HFSC_FINALIZE.
func emitAbort(ctx context.Context, session LogSession, sessionID string, index int) {
	wire := fmt.Sprintf("HFSC_ABORT|v2|%s|%d", sessionID, index)
	dataBytes, err := marshalWireLog(wire)
	if err != nil {
		slog.Debug("hfsc: marshal abort wire", "error", err, "session", sessionID)
		return
	}
	if err := session.Log(ctx, &mcp.LoggingMessageParams{Logger: hfscLoggerName, Level: "warning", Data: dataBytes}); err != nil {
		slog.Debug("hfsc: emit abort log", "error", err, "session", sessionID)
	}
}

// StreamHeavyPayload executes Tier-2 extreme scaling transfer protocol.
// It instantly returns an HFSC proxy intercept pointer to the Orchestrator
// and spins off an asynchronous stream converting the io.Reader into sequential
// Base64 Log chunks, bounding RAM footprint precisely to the chunk size.
func StreamHeavyPayload(
	ctx context.Context,
	session LogSession,
	filename string,
	projectID string,
	model string,
	reader io.Reader,
) (*mcp.CallToolResult, error) {
	// Gate 0: Standalone Mode check
	if !mcplib.IsOrchestratorOwned() || session == nil {
		// Standalone mode has no streaming path, so read into memory — but bound
		// it: an unbounded io.ReadAll here is the exact OOM this package exists to
		// avoid. Read one byte past the cap to detect (not silently truncate) overflow.
		const maxStandaloneBytes = 16 * 1024 * 1024
		b, err := io.ReadAll(io.LimitReader(reader, maxStandaloneBytes+1))
		if err != nil {
			return nil, fmt.Errorf("failed to read stream for standalone delivery: %w", err)
		}
		if len(b) > maxStandaloneBytes {
			return nil, fmt.Errorf("hfsc: standalone payload exceeds %d bytes; orchestrator streaming required", maxStandaloneBytes)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(b)},
			},
		}, nil
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("hfsc: failed to generate session id: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(filename))

	// Intercept Manifest Payload
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: fmt.Sprintf("Extreme-scale %s payload detected. Bypassing massive serialization limits via HFSC stream...", ext),
			},
		},
		Meta: map[string]any{
			"hfsc_stream":         true,
			mcplib.FieldSessionID: sessionID,
			"filename":            filename,
			mcplib.FieldProjectID: projectID,
			"model":               model,
		},
	}

	// Detach from the caller's context (the tool call returns before streaming
	// completes) but bound the lifetime so the goroutine cannot leak forever.
	asyncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), streamTimeout)
	go executeContinuousStream(asyncCtx, cancel, session, sessionID, reader)

	return result, nil
}

// executeContinuousStream chunks the io.Reader locally without buffering the whole struct.
func executeContinuousStream(ctx context.Context, cancel context.CancelFunc, session LogSession, sessionID string, reader io.Reader) {
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("hfsc: stream goroutine panic recovered", "session", sessionID, "panic", r)
		}
	}()
	slog.Info("hfsc: extreme payload stream started", "session", sessionID)

	hasher := sha256.New()
	buf := make([]byte, ReadSize)
	index := 0

	for {
		// Honor shutdown/timeout between chunks so the goroutine terminates.
		select {
		case <-ctx.Done():
			slog.Warn("hfsc: stream cancelled before completion", "session", sessionID, "index", index, "error", ctx.Err())
			emitAbort(context.WithoutCancel(ctx), session, sessionID, index)
			return
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
			chunk := base64.StdEncoding.EncodeToString(buf[:n])

			wire := fmt.Sprintf("HFSC_STREAM|v2|%s|%d|%s", sessionID, index, chunk)
			dataBytes, marshalErr := marshalWireLog(wire)
			if marshalErr != nil {
				slog.Error("hfsc: marshal stream wire failed", "error", marshalErr, "session", sessionID, "index", index)
				emitAbort(ctx, session, sessionID, index)
				return
			}
			params := &mcp.LoggingMessageParams{
				Logger: hfscLoggerName,
				Level:  "info",
				Data:   dataBytes,
			}
			if logErr := session.Log(ctx, params); logErr != nil {
				slog.Error("hfsc: chunk log push failed, stream violently terminated", "error", logErr, "session", sessionID, "index", index)
				return
			}
			index++
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			slog.Error("hfsc: unrecoverable read fault inside extreme payload pipeline", "error", err, "session", sessionID)
			emitAbort(ctx, session, sessionID, index)
			return
		}
	}

	// Complete Stream Checksum Finalizer
	hashHex := hex.EncodeToString(hasher.Sum(nil))
	wire := fmt.Sprintf("HFSC_FINALIZE|v2|%s|%d|%s", sessionID, index, hashHex)
	dataBytes, err := marshalWireLog(wire)
	if err != nil {
		slog.Error("hfsc: marshal finalize wire failed", "error", err, "session", sessionID)
		return
	}
	params := &mcp.LoggingMessageParams{
		Logger: hfscLoggerName,
		Level:  "info",
		Data:   dataBytes,
	}

	if err := session.Log(ctx, params); err != nil {
		slog.Error("hfsc: failed to broadcast stream finalizer hash", "error", err, "session", sessionID)
	} else {
		slog.Info("hfsc: extreme stream successfully finalized natively", "session", sessionID, "total_chunks", index, "sha256", hashHex)
	}
}

// WithAutoIntercept provides middleware to automatically intercept and stream massive payloads.
//
// NOTE: this path json.Marshals the whole output to measure its size, so it is
// NOT a true OOM bypass — the full payload is materialized in memory before
// streaming. For genuinely unbounded payloads, call StreamHeavyPayload directly
// with a real io.Reader. A hard ceiling (maxAutoInterceptBytes) bounds the risk.
func WithAutoIntercept[In any, Out any](
	sp SessionProvider,
) func(func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(next func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
			res, output, err := next(ctx, req, input)
			if err != nil {
				return res, output, err
			}

			if mcplib.IsOrchestratorOwned() {
				if structBytes, marshalErr := json.Marshal(output); marshalErr == nil && len(structBytes) > 2000000 {
					if len(structBytes) > maxAutoInterceptBytes {
						slog.Error("auto-hfsc: payload exceeds hard ceiling; refusing", "size", len(structBytes), "max", maxAutoInterceptBytes)
						return nil, *new(Out), fmt.Errorf("hfsc: output %d bytes exceeds %d auto-intercept ceiling", len(structBytes), maxAutoInterceptBytes)
					}
					var logSess LogSession
					if sess := sp.Session(); sess != nil {
						logSess = sess
					}
					buffer := bytes.NewReader(structBytes)
					hfscRes, hfscErr := StreamHeavyPayload(ctx, logSess, "extreme_auto_intercept.json", req.Params.Name, "auto", buffer)
					if hfscErr == nil {
						slog.Info("auto-hfsc: natively intercepted extreme payload", "size", len(structBytes))
						return hfscRes, *new(Out), nil
					}
				}
			}

			return res, output, err
		}
	}
}
