// Package fastpath provides canonical middleware for routing MCP tool
// responses directly to the filesystem (Artifact Fast-Path) to bypass
// JSON-RPC payload limits.
package fastpath

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/maccavelli/mcplib"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// contained reports whether path is dir itself or lies beneath dir, using a
// real path-boundary check (filepath.Rel) rather than a string prefix. On
// Windows the comparison is case-insensitive.
func contained(path, dir string) bool {
	if dir == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
		dir = strings.ToLower(dir)
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func artifactPathFromInput(input any) string {
	inStr, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	var inMap map[string]any
	if err := json.Unmarshal(inStr, &inMap); err != nil {
		return ""
	}
	p, ok := inMap["artifact_path"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(p)
}

func sanitizeArtifactPath(artifactPath string) string {
	artifactPath = filepath.Clean(artifactPath)
	parent := filepath.Dir(artifactPath)
	if parent == "" {
		return artifactPath
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return artifactPath
	}
	return filepath.Join(resolved, filepath.Base(artifactPath))
}

func isSafeArtifactPath(artifactPath string) bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}
	tempDir := os.TempDir()
	sessionDir := filepath.Join(homeDir, ".gemini", "antigravity", "brain")
	return (homeDir != "" && contained(artifactPath, sessionDir)) || contained(artifactPath, tempDir)
}

func removeTempFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Debug("artifact fast-path: remove temp file", "path", path, "error", err)
	}
}

func writeArtifactAtomic(output any, artifactPath string) (*mcp.CallToolResult, bool) {
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o750); err != nil {
		slog.Warn("artifact fast-path mkdir failed, falling back to JSON-RPC", "path", artifactPath, "error", err)
		return nil, false
	}

	tmp, fErr := os.CreateTemp(filepath.Dir(artifactPath), ".artifact-*.tmp")
	if fErr != nil {
		slog.Warn("artifact fast-path temp create failed, falling back to JSON-RPC", "path", artifactPath, "error", fErr)
		return nil, false
	}

	tmpName := tmp.Name()
	encErr := json.NewEncoder(tmp).Encode(output)
	if encErr == nil {
		encErr = tmp.Sync()
	}
	closeErr := tmp.Close()

	switch {
	case encErr != nil:
		removeTempFile(tmpName)
		slog.Warn("artifact fast-path stream encode failed, falling back to JSON-RPC", "path", artifactPath, "error", encErr)
		return nil, false
	case closeErr != nil:
		removeTempFile(tmpName)
		slog.Warn("artifact fast-path temp close failed, falling back to JSON-RPC", "path", artifactPath, "error", closeErr)
		return nil, false
	default:
		if rErr := os.Rename(tmpName, artifactPath); rErr != nil {
			removeTempFile(tmpName)
			slog.Warn("artifact fast-path rename failed, falling back to JSON-RPC", "path", artifactPath, "error", rErr)
			return nil, false
		}
	}

	slog.Info("artifact fast-path os route successful via stream", "path", artifactPath)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Artifact written natively to: %s", artifactPath)},
		},
	}, true
}

func tryArtifactFastPath[Out any](input any, output Out, handlerErr error) (*mcp.CallToolResult, Out, bool) {
	if !mcplib.IsOrchestratorOwned() {
		return nil, output, false
	}

	artifactPath := artifactPathFromInput(input)
	if artifactPath == "" {
		return nil, output, false
	}

	artifactPath = sanitizeArtifactPath(artifactPath)
	if !isSafeArtifactPath(artifactPath) {
		slog.Warn("artifact fast-path write rejected: out of bounds", "path", artifactPath)
		return nil, output, false
	}

	if handlerErr != nil {
		return nil, output, false
	}

	res, ok := writeArtifactAtomic(output, artifactPath)
	if !ok {
		return nil, output, false
	}
	return res, *new(Out), true
}

// WithArtifactRouting provides middleware to automatically intercept and route payloads directly to disk.
//
// Written artifacts are owned by the caller/orchestrator (identified by the
// caller-supplied "artifact_path"); this middleware does not delete them, so the
// caller is responsible for their lifecycle/cleanup.
func WithArtifactRouting[In any, Out any]() func(func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(next func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
			res, output, err := next(ctx, req, input)
			if fastRes, fastOut, routed := tryArtifactFastPath(input, output, err); routed {
				return fastRes, fastOut, nil
			}
			return res, output, err
		}
	}
}
