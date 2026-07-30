package mcplib

import (
	"context"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer wraps the official Go SDK MCP Server to provide a standardized
// lifecycle manager with session tracking and buffered stdio transport.
// This is the canonical server wrapper used across the fleet.
type MCPServer struct {
	mcpServer *mcp.Server
	session   *mcp.ServerSession
}

// NewMCPServer initializes a new MCPServer instance with the designated name,
// version, and structured logger. It configures the underlying SDK server options.
func NewMCPServer(name, version string, logger *slog.Logger) *MCPServer {
	return &MCPServer{
		mcpServer: mcp.NewServer(
			&mcp.Implementation{Name: name, Version: version},
			&mcp.ServerOptions{Logger: logger},
		),
	}
}

// MCPServer returns the underlying SDK server instance for tool and resource registration.
func (s *MCPServer) MCPServer() *mcp.Server {
	return s.mcpServer
}

// Serve connects the IO transport via standard generic pipes and captures the session.
// It blocks until the session terminates or the context is cancelled.
func (s *MCPServer) Serve(ctx context.Context, stdout io.WriteCloser, reader io.ReadCloser) error {
	t := &mcp.IOTransport{
		Reader: reader,
		Writer: stdout,
	}
	session, err := s.mcpServer.Connect(ctx, t, nil)
	if err == nil {
		s.session = session
		return session.Wait()
	}
	return err
}

// Session returns the active MCP server session established during Serve.
// Returns nil if Serve has not been called or if connection failed.
func (s *MCPServer) Session() *mcp.ServerSession {
	return s.session
}
