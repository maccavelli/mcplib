package mcplib

import (
	"os"
	"testing"
)

func TestIsOrchestratorOwned_True(t *testing.T) {
	t.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	if !IsOrchestratorOwned() {
		t.Fatal("expected IsOrchestratorOwned to return true when MCP_ORCHESTRATOR_OWNED=true")
	}
}

func TestIsOrchestratorOwned_False(t *testing.T) {
	t.Setenv("MCP_ORCHESTRATOR_OWNED", "false")
	if IsOrchestratorOwned() {
		t.Fatal("expected IsOrchestratorOwned to return false when MCP_ORCHESTRATOR_OWNED=false")
	}
}

func TestIsOrchestratorOwned_Unset(t *testing.T) {
	os.Unsetenv("MCP_ORCHESTRATOR_OWNED")
	if IsOrchestratorOwned() {
		t.Fatal("expected IsOrchestratorOwned to return false when MCP_ORCHESTRATOR_OWNED is unset")
	}
}
