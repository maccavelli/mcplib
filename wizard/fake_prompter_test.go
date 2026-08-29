package wizard

import (
	"fmt"
	"testing"
)

// fakePrompter is a scripted Prompter: it replays queued answers and records
// every prompt it was shown. This is what makes ConfigureLLM testable with no
// TTY — coverage none of the three existing wizards has today.
//
// An unexpected prompt is an error rather than a zero value, so a flow that
// asks something the test did not anticipate fails loudly.
type fakePrompter struct {
	t *testing.T

	selects      []int
	multiSelects [][]int
	confirms     []bool
	inputs       []string
	secrets      []string

	// Recorded for assertions.
	seenSelect      []string   // titles
	seenSelectItems [][]Choice // choices per Select call
	seenConfirm     []string
	seenInput       []string
	seenSecret      []string
	seenNotify      []string

	// allText accumulates every string the user could have seen, for the
	// "a credential must never be displayed" assertion.
	allText []string
}

func (f *fakePrompter) record(s string) { f.allText = append(f.allText, s) }

func (f *fakePrompter) Select(title string, choices []Choice, defaultIdx int) (int, error) {
	f.t.Helper()
	f.seenSelect = append(f.seenSelect, title)
	f.seenSelectItems = append(f.seenSelectItems, choices)
	f.record(title)
	for _, c := range choices {
		f.record(c.Label)
		f.record(c.Detail)
	}
	if len(f.selects) == 0 {
		return 0, fmt.Errorf("fakePrompter: unexpected Select(%q)", title)
	}
	v := f.selects[0]
	f.selects = f.selects[1:]
	if v < 0 || v >= len(choices) {
		return 0, fmt.Errorf("fakePrompter: scripted index %d out of range for %q", v, title)
	}
	return v, nil
}

func (f *fakePrompter) MultiSelect(title string, choices []Choice, preselected []int) ([]int, error) {
	f.t.Helper()
	f.record(title)
	for _, c := range choices {
		f.record(c.Label)
	}
	if len(f.multiSelects) == 0 {
		return nil, fmt.Errorf("fakePrompter: unexpected MultiSelect(%q)", title)
	}
	v := f.multiSelects[0]
	f.multiSelects = f.multiSelects[1:]
	return v, nil
}

func (f *fakePrompter) Confirm(question string, def bool) (bool, error) {
	f.t.Helper()
	f.seenConfirm = append(f.seenConfirm, question)
	f.record(question)
	if len(f.confirms) == 0 {
		return false, fmt.Errorf("fakePrompter: unexpected Confirm(%q)", question)
	}
	v := f.confirms[0]
	f.confirms = f.confirms[1:]
	return v, nil
}

func (f *fakePrompter) Input(prompt, def string) (string, error) {
	f.t.Helper()
	f.seenInput = append(f.seenInput, prompt)
	f.record(prompt)
	f.record(def)
	if len(f.inputs) == 0 {
		return def, nil // an un-scripted Input accepts the default
	}
	v := f.inputs[0]
	f.inputs = f.inputs[1:]
	return v, nil
}

func (f *fakePrompter) Secret(prompt string) (string, error) {
	f.t.Helper()
	f.seenSecret = append(f.seenSecret, prompt)
	f.record(prompt)
	if len(f.secrets) == 0 {
		return "", fmt.Errorf("fakePrompter: unexpected Secret(%q)", prompt)
	}
	v := f.secrets[0]
	f.secrets = f.secrets[1:]
	return v, nil
}

func (f *fakePrompter) Notify(level Level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	f.seenNotify = append(f.seenNotify, msg)
	f.record(msg)
}

var _ Prompter = (*fakePrompter)(nil)
