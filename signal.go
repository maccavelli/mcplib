package mcplib

import (
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OrchestratorSignal is the canonical telemetry payload structure injected
// into tool results when the process is orchestrator-managed. The orchestrator
// uses these signals for pipeline correlation, success tracking, and
// confidence scoring.
type OrchestratorSignal struct {
	Success       bool    `json:"success"`
	Confidence    float64 `json:"confidence"`
	IntentContext string  `json:"intent_context"`
	Category      string  `json:"category"`
}

// ClassifySuccess performs heuristic analysis on tool response content to
// determine execution success and confidence. It scans TextContent entries
// for known failure indicators.
func ClassifySuccess(res *mcp.CallToolResult) (success bool, confidence float64) {
	success = true
	confidence = 1.0

	for _, content := range res.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			msg := strings.ToLower(tc.Text)
			if strings.Contains(msg, "no matches found") ||
				strings.Contains(msg, "target not identified") ||
				strings.Contains(msg, "could not find") {
				success = false
				confidence = 0.5
				break
			}
		}
	}
	return success, confidence
}

// InjectSignal appends the __orchestrator_signal telemetry payload to the
// tool result's Content slice. This is a no-op if the process is not
// orchestrator-owned.
func InjectSignal(res *mcp.CallToolResult, signal OrchestratorSignal) {
	sigBytes, err := json.Marshal(map[string]any{"__orchestrator_signal": signal})
	if err != nil {
		return
	}
	res.Content = append(res.Content, &mcp.TextContent{
		Text: string(sigBytes),
	})
}
