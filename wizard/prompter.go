// Package wizard provides the canonical LLM provider configuration flow shared
// by every mcplib consumer that has a setup UI.
//
// It owns the DATA and the FLOW; consumers own the RENDERING. That split is
// deliberate: mcplib is consumed by twelve repositories, only three of which
// have a configuration wizard, and those three disagree about UI toolkits — two
// use pterm, one is deliberately plain. Moving pterm into mcplib would impose an
// interactive-terminal dependency on nine headless MCP servers that will never
// draw a menu.
//
// Consumers implement Prompter over whatever they already use. TextPrompter is
// provided for those with no toolkit at all.
package wizard

import "fmt"

// Level classifies a Notify message.
type Level int

// Notification levels.
const (
	LevelInfo Level = iota
	LevelWarn
	LevelError
)

// String implements fmt.Stringer.
func (l Level) String() string {
	switch l {
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return fmt.Sprintf("level(%d)", int(l))
	}
}

// Choice is one selectable option. Detail is an optional qualifier shown
// alongside the label, e.g. a provider's pricing model.
type Choice struct {
	Label  string
	Detail string
}

// Prompter is the rendering seam. A consumer implements it over its own UI
// toolkit; mcplib never imports one.
//
// The interface is deliberately small and stable: every method mirrors
// something all three existing wizards already do. It is public API in a
// library twelve repositories consume, so changing it later is a breaking
// change for all of them.
type Prompter interface {
	// Select asks the user to choose one option, returning its index.
	// defaultIdx is chosen when the user accepts the default.
	Select(title string, choices []Choice, defaultIdx int) (int, error)

	// MultiSelect asks the user to choose zero or more options, returning
	// their indices. preselected may be nil.
	MultiSelect(title string, choices []Choice, preselected []int) ([]int, error)

	// Confirm asks a yes/no question, returning def when the user accepts
	// the default.
	Confirm(question string, def bool) (bool, error)

	// Input reads a line of visible text, returning def when the user
	// accepts the default.
	Input(prompt, def string) (string, error)

	// Secret reads a credential without echoing it in full. Implementations
	// must never write the value to a log.
	Secret(prompt string) (string, error)

	// Notify reports progress or a problem. It must not be used to display a
	// credential; callers mask first.
	Notify(level Level, format string, args ...any)
}
