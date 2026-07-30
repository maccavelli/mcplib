package mcplib

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHardenedPromptHandler_Success(t *testing.T) {
	handler := HardenedPromptHandler(func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "test",
		}, nil
	})
	res, err := handler(context.Background(), &mcp.GetPromptRequest{})
	if err != nil {
		t.Fatalf("expected nil error; got %v", err)
	}
	if res == nil || res.Description != "test" {
		t.Fatal("expected valid result")
	}
}

func TestHardenedPromptHandler_PanicRecovery(t *testing.T) {
	handler := HardenedPromptHandler(func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		panic("prompt panic")
	})
	_, err := handler(context.Background(), &mcp.GetPromptRequest{})
	if err == nil {
		t.Fatal("expected error from panic recovery")
	}
}
