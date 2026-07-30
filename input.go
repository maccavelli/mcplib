package mcplib

// UniversalBaseInput defines optional telemetry correlation fields shared across
// MCP sub-server tool inputs. Embed in handler argument structs so schema
// reflection marks session_id as optional (pointer + omitempty).
type UniversalBaseInput struct {
	SessionID *string `json:"session_id,omitempty,omitzero" jsonschema:"Optional CSSA correlation ID"`
}

// optionalTelemetryFields are never auto-promoted to required during magictools
// schema sync (Gap S1 repair). Handlers treat these as optional even when the
// upstream JSON Schema omits an explicit required array.
var optionalTelemetryFields = map[string]struct{}{
	FieldSessionID:  {},
	FieldProjectID:  {},
	"artifact_path": {},
	"context":       {},
}

// IsOptionalTelemetryField reports whether a schema property name should be
// excluded from sync-time required-field promotion.
func IsOptionalTelemetryField(name string) bool {
	_, ok := optionalTelemetryFields[name]
	return ok
}
