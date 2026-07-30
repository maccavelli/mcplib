// Package mcplib provides shared primitives for the MCP server ecosystem.
//
// This module contains canonical implementations of cross-cutting concerns
// shared by all mcp-server-* projects: transport I/O adapters, shutdown
// error classification, orchestrator detection, lifecycle locking, and
// NopCloser adapters.
package mcplib

import "os"

// IsOrchestratorOwned reports whether the current process is managed by
// the magictools orchestrator. The orchestrator sets the environment
// variable MCP_ORCHESTRATOR_OWNED=true on every sub-server it spawns.
//
// This function is the canonical, shared implementation. Individual
// servers MUST NOT re-implement this check via Viper bindings or
// config-struct methods — the env var is set once at process spawn
// and never mutates at runtime.
func IsOrchestratorOwned() bool {
	return os.Getenv("MCP_ORCHESTRATOR_OWNED") == EnvValueTrue
}
