// Package mcplib provides canonical shared primitives for MCP server infrastructure.
//
// This file provides NopReadCloser and NopWriteCloser adapters that wrap
// io.Reader and io.Writer with no-op Close methods to satisfy io.ReadCloser
// and io.WriteCloser interfaces respectively. These are used across the fleet
// for buffered stdio transport initialization.
package mcplib

import "io"

// NopReadCloser wraps an io.Reader to implement io.ReadCloser with a no-op Close.
type NopReadCloser struct{ io.Reader }

// Close implements the io.Closer interface by returning nil.
func (NopReadCloser) Close() error { return nil }

// NopWriteCloser wraps an io.Writer to implement io.WriteCloser with a no-op Close.
type NopWriteCloser struct{ io.Writer }

// Close implements the io.Closer interface by returning nil.
func (NopWriteCloser) Close() error { return nil }
