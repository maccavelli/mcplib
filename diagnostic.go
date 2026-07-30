package mcplib

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DiagnosticInput defines the canonical input schema for the get_internal_logs tool.
type DiagnosticInput struct {
	SessionID string `json:"session_id"`
	MaxLines  int    `json:"max_lines"`
}

const (
	defaultDiagnosticMaxLines = 25
	defaultDiagnosticMaxCap   = 1000
)

// diagnosticConfig holds the resolved tunables for the get_internal_logs tool.
type diagnosticConfig struct {
	defaultMaxLines int // returned when the caller omits max_lines
	maxCap          int // upper clamp for max_lines
}

// DiagnosticOption configures RegisterDiagnosticTool. Options are additive, so
// existing 3-argument call sites remain source-compatible.
type DiagnosticOption func(*diagnosticConfig)

// WithDefaultMaxLines overrides the number of log lines returned when the caller
// omits max_lines (default 25). Non-positive values are ignored.
func WithDefaultMaxLines(n int) DiagnosticOption {
	return func(c *diagnosticConfig) { c.defaultMaxLines = n }
}

// WithMaxLineCap overrides the upper clamp applied to max_lines (default 1000).
// Non-positive values are ignored.
func WithMaxLineCap(n int) DiagnosticOption {
	return func(c *diagnosticConfig) { c.maxCap = n }
}

func resolveDiagnosticConfig(opts ...DiagnosticOption) diagnosticConfig {
	c := diagnosticConfig{
		defaultMaxLines: defaultDiagnosticMaxLines,
		maxCap:          defaultDiagnosticMaxCap,
	}
	for _, o := range opts {
		o(&c)
	}
	if c.defaultMaxLines <= 0 {
		c.defaultMaxLines = defaultDiagnosticMaxLines
	}
	if c.maxCap <= 0 {
		c.maxCap = defaultDiagnosticMaxCap
	}
	return c
}

// RegisterDiagnosticTool canonically binds the get_internal_logs tool to the provided server.
// It implements dynamic description templating using a zero-allocation strings.Builder
// to ensure the orchestrator's semantic index generates mathematically distinct vector
// embeddings, preventing routing collisions (STD-MCP-LOG-TELEMETRY-001).
func RegisterDiagnosticTool(s *mcp.Server, ringBuffer *LogBuffer, serverDomain string, opts ...DiagnosticOption) {
	if ringBuffer == nil {
		slog.Error("mcplib: RegisterDiagnosticTool requires a non-nil LogBuffer", "tool", "get_internal_logs")
		return
	}

	cfg := resolveDiagnosticConfig(opts...)

	var builder strings.Builder

	// Fallback/Graceful Degradation Check
	trimmedDomain := strings.TrimSpace(serverDomain)
	if trimmedDomain == "" {
		slog.Error("mcplib: empty serverDomain provided to RegisterDiagnosticTool", "tool", "get_internal_logs")
		trimmedDomain = "UNDEFINED [WARNING: INVALID CONTEXT]"
	} else if len(trimmedDomain) > 150 {
		slog.Error("mcplib: excessively long serverDomain provided to RegisterDiagnosticTool, truncating", "len", len(trimmedDomain))
		trimmedDomain = trimmedDomain[:150] + " [TRUNCATED]"
	}

	// Pre-allocate to prevent intermediate heap allocations during concatenation.
	builder.Grow(150 + len(trimmedDomain))

	builder.WriteString("[DIRECTIVE: ")
	builder.WriteString(strings.ToUpper(strings.ReplaceAll(trimmedDomain, " ", "_")))
	builder.WriteString(" TELEMETRY] Retrieves diagnostic logs natively from the ")
	builder.WriteString(trimmedDomain)
	builder.WriteString(" ecosystem memory buffer. ")
	builder.WriteString("[PROSE: Use this tool to retrieve internal system logs specifically for the ")
	builder.WriteString(trimmedDomain)
	builder.WriteString(" server when diagnosing failures or verifying execution telemetry.] ")
	builder.WriteString("Keywords: diagnostic, logs, telemetry, ")
	builder.WriteString(strings.ReplaceAll(strings.ToLower(trimmedDomain), " ", "-"))
	builder.WriteString(", system-buffer, debug-server")

	tool := &mcp.Tool{
		Name:        "get_internal_logs",
		Description: builder.String(),
		InputSchema: map[string]any{
			SchemaTypeKey: SchemaTypeObject,
			"properties": map[string]any{
				FieldSessionID: map[string]any{
					SchemaTypeKey: "string",
					"description": "Optional CSSA session correlation ID. If provided, filters logs to only include lines containing this ID, isolating concurrent orchestrator telemetry.",
				},
				"max_lines": map[string]any{
					SchemaTypeKey: "integer",
					"description": fmt.Sprintf("Optional number of recent log lines to retrieve (0-%d). Defaults to %d.", cfg.maxCap, cfg.defaultMaxLines),
					"default":     0,
				},
			},
		},
	}

	handleFunc := diagnosticHandler(ringBuffer, cfg)
	HardenedAddTool(s, tool, handleFunc)
}

// DiagnosticHandlerFunc returns the core execution logic for the get_internal_logs tool,
// bound to the provided ring buffer, using the default 25-line default and 1000-line cap.
// It handles session isolation and max_lines clamping.
func DiagnosticHandlerFunc(ringBuffer *LogBuffer) func(ctx context.Context, req *mcp.CallToolRequest, input DiagnosticInput) (*mcp.CallToolResult, any, error) {
	return diagnosticHandler(ringBuffer, resolveDiagnosticConfig())
}

// diagnosticHandler is the configurable core of the get_internal_logs tool.
func diagnosticHandler(ringBuffer *LogBuffer, cfg diagnosticConfig) func(ctx context.Context, req *mcp.CallToolRequest, input DiagnosticInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input DiagnosticInput) (*mcp.CallToolResult, any, error) {
		maxLines := input.MaxLines
		if maxLines <= 0 {
			maxLines = cfg.defaultMaxLines
		} else if maxLines > cfg.maxCap {
			maxLines = cfg.maxCap
		}

		rawLogs := ringBuffer.String()
		if rawLogs == "" || strings.TrimSpace(rawLogs) == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "[NO LOGS AVAILABLE]"}},
			}, nil, nil
		}

		// Phase 1 (Black Swan Mitigation): Concurrent Orchestrator Session Isolation.
		// Grep the global buffer for the exact SessionID if provided.
		if input.SessionID != "" {
			var filtered strings.Builder
			lines := strings.SplitSeq(rawLogs, "\n")
			for line := range lines {
				if strings.Contains(line, input.SessionID) {
					filtered.WriteString(line)
					filtered.WriteString("\n")
				}
			}
			rawLogs = filtered.String()
			if rawLogs == "" {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "[NO LOGS AVAILABLE FOR SESSION]"}},
				}, nil, nil
			}
		}

		output := TailLines(rawLogs, maxLines)
		if output == "" || strings.TrimSpace(output) == "" {
			output = "[NO LOGS AVAILABLE]"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: output}},
		}, nil, nil
	}
}
