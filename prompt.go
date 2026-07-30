package mcplib

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HardenedPromptHandler wraps an MCP prompt handler with panic recovery
// to prevent unhandled errors from crashing the server process.
func HardenedPromptHandler(
	handler func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error),
) func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (res *mcp.GetPromptResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("prompt handler panic", "panic", r, "trace", string(debug.Stack()))
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}
