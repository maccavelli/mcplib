package mcplib_test

import (
	"encoding/json"
	"testing"

	"github.com/maccavelli/mcplib"
)

// TestSocraticMachineResponse_Unmarshal verifies JSON unmarshaling of the
// SocraticMachineResponse struct including HFSC key propagation.
func TestSocraticMachineResponse_Unmarshal(t *testing.T) {
	raw := `{
		"decisions": [
			{"topic": "Safety Verdict", "decision": "SAFE", "rationale": "No race conditions detected."},
			{"topic": "Performance", "decision": "Acceptable", "rationale": "O(n) complexity preserved."}
		],
		"outstanding_questions": ["What about context cancellation in goroutines?"],
		"diagnostic_log_hfsc_key": "hfsc-1234567890"
	}`

	var resp mcplib.SocraticMachineResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(resp.Decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(resp.Decisions))
	}
	if resp.Decisions[0].Topic != "Safety Verdict" {
		t.Errorf("expected topic 'Safety Verdict', got %q", resp.Decisions[0].Topic)
	}
	if resp.Decisions[0].Decision != "SAFE" {
		t.Errorf("expected decision 'SAFE', got %q", resp.Decisions[0].Decision)
	}
	if resp.Decisions[0].Rationale != "No race conditions detected." {
		t.Errorf("unexpected rationale: %q", resp.Decisions[0].Rationale)
	}
	if len(resp.OutstandingQuestions) != 1 {
		t.Fatalf("expected 1 outstanding question, got %d", len(resp.OutstandingQuestions))
	}
	if resp.DiagnosticLogHFSCKey != "hfsc-1234567890" {
		t.Errorf("expected HFSC key 'hfsc-1234567890', got %q", resp.DiagnosticLogHFSCKey)
	}
}

// TestSocraticMachineResponse_EmptyDecisions verifies correct deserialization
// when the LLM returns no decisions or questions.
func TestSocraticMachineResponse_EmptyDecisions(t *testing.T) {
	raw := `{"decisions": [], "outstanding_questions": []}`

	var resp mcplib.SocraticMachineResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(resp.Decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(resp.Decisions))
	}
	if len(resp.OutstandingQuestions) != 0 {
		t.Errorf("expected 0 questions, got %d", len(resp.OutstandingQuestions))
	}
	if resp.DiagnosticLogHFSCKey != "" {
		t.Errorf("expected empty HFSC key, got %q", resp.DiagnosticLogHFSCKey)
	}
}

// TestSocraticMachineResponse_MarkdownStripping verifies the regex fence
// stripping handles markdown-wrapped JSON from LLMs with RLHF training.
func TestSocraticMachineResponse_MarkdownStripping(t *testing.T) {
	// Simulate LLM hallucinating markdown fences around JSON
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "json fence",
			input: "```json\n{\"decisions\":[{\"topic\":\"T\",\"decision\":\"D\",\"rationale\":\"R\"}],\"outstanding_questions\":[]}\n```",
		},
		{
			name:  "bare fence",
			input: "```\n{\"decisions\":[{\"topic\":\"T\",\"decision\":\"D\",\"rationale\":\"R\"}],\"outstanding_questions\":[]}\n```",
		},
		{
			name:  "no fence",
			input: "{\"decisions\":[{\"topic\":\"T\",\"decision\":\"D\",\"rationale\":\"R\"}],\"outstanding_questions\":[]}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use the same regex as the SocraticClient
			cleaned := tc.input
			if matches := mcplib.MarkdownFenceRegex().FindStringSubmatch(cleaned); len(matches) > 1 {
				cleaned = matches[1]
			}

			var resp mcplib.SocraticMachineResponse
			if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
				t.Fatalf("unmarshal failed for %q: %v", tc.name, err)
			}
			if len(resp.Decisions) != 1 || resp.Decisions[0].Topic != "T" {
				t.Errorf("unexpected parse result for %q: %+v", tc.name, resp)
			}
		})
	}
}

// TestSocraticClient_EnabledDefault verifies the circuit breaker starts disabled.
func TestSocraticClient_EnabledDefault(t *testing.T) {
	c := mcplib.NewSocraticClient("http://localhost:0/mcp", "test-server")
	if c.Enabled() {
		t.Error("expected Enabled() to be false before Start()")
	}
}

// TestSocraticClient_AnalyzeRequiresConnection verifies Analyze fails cleanly
// when the circuit breaker is open.
func TestSocraticClient_AnalyzeRequiresConnection(t *testing.T) {
	c := mcplib.NewSocraticClient("http://localhost:0/mcp", "test-server")
	_, err := c.Analyze(t.Context(), "test problem", 0)
	if err == nil {
		t.Fatal("expected error when circuit breaker is open")
	}
	if !contains(err.Error(), "circuit breaker") {
		t.Errorf("expected circuit breaker error, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstr(s, substr)
}

func searchSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
