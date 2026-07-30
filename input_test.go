package mcplib

import "testing"

func TestIsOptionalTelemetryField(t *testing.T) {
	optional := []string{"session_id", "project_id", "artifact_path", "context"}
	for _, name := range optional {
		if !IsOptionalTelemetryField(name) {
			t.Errorf("IsOptionalTelemetryField(%q) = false; want true", name)
		}
	}
	if IsOptionalTelemetryField("namespace") {
		t.Error("IsOptionalTelemetryField(namespace) = true; want false")
	}
}
