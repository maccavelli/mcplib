package mcplib

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHardenedResourceHandler_Success(t *testing.T) {
	handler := HardenedResourceHandler(func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{},
		}, nil
	})
	res, err := handler(context.Background(), &mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("expected nil error; got %v", err)
	}
	if res == nil {
		t.Fatal("expected valid result")
	}
}

func TestHardenedResourceHandler_PanicRecovery(t *testing.T) {
	handler := HardenedResourceHandler(func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		panic("resource panic")
	})
	_, err := handler(context.Background(), &mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected error from panic recovery")
	}
}
