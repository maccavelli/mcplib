package llmprovider

import (
	"slices"
	"strings"
)

// The Grok CLI catalog's models (MADR 0012 §6).
const (
	grokModel46 = "grok-4.6"
	grokModel45 = "grok-4.5"
)

// grokEffortOrder ranks the efforts, lowest first.
var grokEffortOrder = []string{effortLow, effortMedium, effortHigh, effortXHigh}

// grokEffortMenu returns the reasoning efforts the Grok CLI offers a model,
// lowest first, or nil when reasoning_effort must be omitted (MADR 0012 §6).
// grok-4.6 and grok-4.5 are the CLI catalog's entries (grok-build f0e3be11:
// xai-grok-models/default_models.json); grok-4.6-build takes low and high
// (xai-grok-shell/src/agent/config_tests.rs:7958), as does grok-3-mini. Other
// models reason automatically and may reject the parameter.
func grokEffortMenu(model string) []string {
	sm := strings.ToLower(model)
	switch {
	case sm == grokModel46:
		return []string{effortLow, effortMedium, effortHigh, effortXHigh}
	case sm == grokModel45:
		return []string{effortLow, effortMedium, effortHigh}
	case sm == "grok-4.6-build", strings.HasPrefix(sm, "grok-3-mini"):
		return []string{effortLow, effortHigh}
	default:
		return nil
	}
}

// grokClampReasoningEffort returns the reasoning_effort value to include in the
// request body, or "" if the parameter must be omitted entirely. No effort
// sends high, every menu's CLI default. An effort off the menu clamps to the
// nearest lower one on it, else the menu's highest.
func grokClampReasoningEffort(model, effort string) string {
	menu := grokEffortMenu(model)
	if len(menu) == 0 {
		return ""
	}
	want := strings.ToLower(effort)
	if want == "" {
		return effortHigh
	}
	rank := slices.Index(grokEffortOrder, want)
	clamped := ""
	for _, e := range menu {
		if slices.Index(grokEffortOrder, e) <= rank {
			clamped = e
		}
	}
	if clamped == "" {
		return menu[len(menu)-1]
	}
	return clamped
}
