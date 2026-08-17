package llmprovider

import "strings"

// reasoningSupport enumerates gating tiers for Grok reasoning_effort.
type reasoningSupport int

const (
	// reasoningUnsupported: model reasons automatically, sending reasoning_effort
	// causes a 400 Bad Request. Applies to: grok-3, grok-4, grok-4-fast-reasoning,
	// grok-code-fast-1.
	reasoningUnsupported reasoningSupport = iota

	// reasoningLowHigh: model accepts reasoning_effort with only "low" or "high".
	// Applies to: grok-3-mini, grok-3-mini-fast.
	reasoningLowHigh

	// reasoningFull: model accepts reasoning_effort with "low"/"medium"/"high"/"xhigh".
	// Reasoning cannot be disabled. Applies to: grok-4.5, grok-4.6.
	reasoningFull
)

// grokReasoningSupport returns the reasoning-effort gating tier for a Grok model.
// Unknown models default to reasoningUnsupported (safest: omit the parameter).
func grokReasoningSupport(model string) reasoningSupport {
	sm := strings.ToLower(model)
	switch {
	// grok-3-mini family: low/high only
	case strings.HasPrefix(sm, "grok-3-mini"):
		return reasoningLowHigh
	// grok-4.5 / grok-4.6 family: full range, always-on reasoning
	case strings.HasPrefix(sm, "grok-4.5"), strings.HasPrefix(sm, "grok-4.6"):
		return reasoningFull
	// grok-3, grok-4, grok-4-fast-reasoning, grok-code-fast-1: unsupported
	default:
		return reasoningUnsupported
	}
}

// grokClampReasoningEffort returns the reasoning_effort value to include in the
// request body, or "" if the parameter must be omitted entirely.
func grokClampReasoningEffort(model, effort string) string {
	tier := grokReasoningSupport(model)
	switch tier {
	case reasoningUnsupported:
		return "" // must not send
	case reasoningLowHigh:
		switch strings.ToLower(effort) {
		case effortLow:
			return effortLow
		default:
			return effortHigh // clamp everything else to "high"
		}
	case reasoningFull:
		switch strings.ToLower(effort) {
		case effortLow, effortMedium, effortHigh, effortXHigh:
			return strings.ToLower(effort)
		default:
			return effortHigh // default for full-range models
		}
	}
	return ""
}
