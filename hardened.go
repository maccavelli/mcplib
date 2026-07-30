package mcplib

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolConfig holds the configuration for HardenedAddTool middleware.
type ToolConfig struct {
	// SerializeCalls wraps the handler in a global sync.Mutex to prevent
	// concurrent tool execution within the same server process.
	SerializeCalls bool

	// LogStack includes debug.Stack() output in panic recovery log entries.
	LogStack bool

	// SchemaDeriveFunc is an optional callback invoked at registration time
	// to auto-derive tool.InputSchema via reflection. Keeping this as a
	// callback ensures mcplib has zero third-party dependencies — servers
	// bring their own jsonschema reflector.
	SchemaDeriveFunc func(*mcp.Tool)
}

// ToolOption configures optional middleware behaviors for HardenedAddTool.
type ToolOption func(*ToolConfig)

// WithSchemaDerival enables automatic InputSchema derivation at registration
// time via a user-provided callback. The callback should use a jsonschema
// reflector to populate tool.InputSchema from the generic input type.
//
// Example:
//
//	mcplib.WithSchemaDerival(func(t *mcp.Tool) {
//	    r := new(jsonschema.Reflector)
//	    r.ExpandedStruct = true
//	    sch := r.Reflect(new(MyInput))
//	    b, _ := json.Marshal(sch)
//	    var m map[string]any
//	    json.Unmarshal(b, &m)
//	    t.InputSchema = m
//	})
func WithSchemaDerival(fn func(*mcp.Tool)) ToolOption {
	return func(c *ToolConfig) {
		c.SchemaDeriveFunc = fn
	}
}

// WithSerializedCalls wraps the tool handler in a global sync.Mutex,
// preventing concurrent tool execution within the server process.
func WithSerializedCalls() ToolOption {
	return func(c *ToolConfig) {
		c.SerializeCalls = true
	}
}

// WithStackTrace includes debug.Stack() output in panic recovery logs.
func WithStackTrace() ToolOption {
	return func(c *ToolConfig) {
		c.LogStack = true
	}
}

// toolMutexes serializes tool execution per-tool when WithSerializedCalls is enabled.
var toolMutexes sync.Map

func getToolMutex(name string) *sync.Mutex {
	mu, _ := toolMutexes.LoadOrStore(name, &sync.Mutex{})
	mutex, ok := mu.(*sync.Mutex)
	if !ok {
		newMu := &sync.Mutex{}
		toolMutexes.Store(name, newMu)
		return newMu
	}
	return mutex
}

// HardenedAddTool registers an MCP tool with composable hardening middleware.
//
// Always-on layers:
//   - L1: Panic recovery — recovers panics and returns a structured error result
//   - L2: Nil-safety — initializes res.Content to empty slice if nil
//   - L3: Telemetry signal — injects __orchestrator_signal when orchestrator-owned
//
// Opt-in layers via ToolOption:
//   - L4: Schema derivation — WithSchemaDerival(fn) for auto-derived InputSchema
//   - L7: Call serialization — WithSerializedCalls() for global mutex
func HardenedAddTool[In any, Out any](
	s *mcp.Server,
	tool *mcp.Tool,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
	opts ...ToolOption,
) {
	mcp.AddTool(s, tool, InternalWrapHandler(tool, handler, opts...))
}

// InternalWrapHandler returns the wrapped handler closure for direct use or
// coverage testing. It applies the same middleware stack as HardenedAddTool.
func InternalWrapHandler[In any, Out any](
	tool *mcp.Tool,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
	opts ...ToolOption,
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	var cfg ToolConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// L4: Schema derivation at registration time.
	if cfg.SchemaDeriveFunc != nil && tool.InputSchema == nil {
		cfg.SchemaDeriveFunc(tool)
	}

	// The divergence probe (below) is a per-call diagnostic: a second JSON decode
	// plus reflect.DeepEqual on every tool call. It is OFF by default to avoid
	// fleet-wide overhead; enable for a measurement window via
	// MCP_HARDENED_DECODE_PROBE=true. Read once at registration time.
	decodeProbe := os.Getenv("MCP_HARDENED_DECODE_PROBE") == EnvValueTrue

	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (res *mcp.CallToolResult, output Out, err error) {
		// L1: Panic recovery — outermost defer runs last.
		defer func() {
			if r := recover(); r != nil {
				if cfg.LogStack {
					slog.Error("panic recovered in tool handler",
						"tool", tool.Name,
						"panic", r,
						"trace", string(debug.Stack()),
					)
				} else {
					slog.Error("panic recovered in tool handler",
						"tool", tool.Name,
						"panic", r,
					)
				}
				res = &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{
							Text: fmt.Sprintf("Internal Error: Panic recovered in handler: %v", r),
						},
					},
				}
				err = nil
			}
		}()

		// L7: Call serialization (opt-in).
		if cfg.SerializeCalls {
			mu := getToolMutex(tool.Name)
			mu.Lock()
			defer mu.Unlock()
		}

		// Divergence probe (read-only, opt-in via MCP_HARDENED_DECODE_PROBE). The
		// go-sdk has already decoded Arguments into `input` case-SENSITIVELY and
		// with JSON-schema defaults applied; that value is always what the handler
		// receives below. Here we re-decode with stdlib encoding/json
		// (case-INsensitive, no defaults) ONLY to detect a mismatch, not to override
		// it — measuring whether any client relies on case-insensitive field matching.
		// The raw payload is deliberately not logged (it may contain secrets).
		if decodeProbe && req.Params != nil && len(req.Params.Arguments) > 0 {
			var probeInput In
			if err := json.Unmarshal(req.Params.Arguments, &probeInput); err != nil {
				slog.Warn("hardened probe: fallback unmarshal failed", "tool", tool.Name, "error", err)
			} else if !reflect.DeepEqual(probeInput, input) {
				slog.Warn("hardened probe: fallback decode diverged from SDK decode (using SDK value)", "tool", tool.Name)
			}
		}

		res, output, err = handler(ctx, req, input)

		// L2: Nil-safety — ensure Content is initialized.
		if err == nil && res != nil && res.Content == nil {
			res.Content = []mcp.Content{}
		}

		// L3: Telemetry signal injection (always-on when orchestrated).
		if IsOrchestratorOwned() && res != nil {
			success, confidence := ClassifySuccess(res)
			var intentContext string
			if req.Params != nil {
				intentContext = req.Params.Name
			}
			InjectSignal(res, OrchestratorSignal{
				Success:       success,
				Confidence:    confidence,
				IntentContext: intentContext,
				Category:      tool.Name,
			})
		}

		return res, output, err
	}
}

// HardenedAddSimpleTool adapts a simplified handler signature into the
// standard SDK ToolHandlerFunc and delegates to HardenedAddTool.
//
// Use this for servers whose handlers don't need access to the raw
// *mcp.CallToolRequest or don't construct *mcp.CallToolResult directly.
// On success, the Out value is returned as structured output via the SDK.
// On error, a CallToolResult{IsError: true} is constructed with the error message.
func HardenedAddSimpleTool[In any, Out any](
	s *mcp.Server,
	tool *mcp.Tool,
	handler func(context.Context, In) (Out, error),
	opts ...ToolOption,
) {
	adapted := func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		out, hErr := handler(ctx, input)
		if hErr != nil {
			var zero Out
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("execution failed: %v", hErr)}},
			}, zero, nil
		}

		outBytes, marshalErr := json.Marshal(out)
		if marshalErr != nil {
			var zero Out
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("serialization failed: %v", marshalErr)}},
			}, zero, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(outBytes)}},
		}, out, nil
	}
	HardenedAddTool(s, tool, adapted, opts...)
}
