package mcplib

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HardenedResourceHandler wraps an MCP resource handler with panic recovery
// to prevent unhandled errors from crashing the server process.
func HardenedResourceHandler(
	handler func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error),
) func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (res *mcp.ReadResourceResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("resource handler panic", "panic", r, "trace", string(debug.Stack()))
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}
