package wizard

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// minRevealRunes is the entry length below which Secret reveals nothing. Below
// it, four revealed runes would be most of a short or partially-pasted value.
const minRevealRunes = 8

// revealTail is how many trailing runes Secret shows while typing.
const revealTail = 4

// maskGlyphs is the fixed-width mask body, so the rendering does not encode the
// credential's length as it grows.
const maskGlyphs = 8

// TextPrompter is the zero-toolkit Prompter: stdlib plus golang.org/x/term,
// which every mcplib consumer with a wizard already depends on. Consumers with
// a UI toolkit implement Prompter over it instead.
type TextPrompter struct {
	// In is the input source. When it is an *os.File attached to a terminal,
	// Secret uses raw mode for live masking; otherwise it degrades to a plain
	// read. Accepting io.Reader rather than *os.File lets consumers inject a
	// scripted reader in their own tests.
	In  io.Reader
	Out io.Writer

	// reader is retained across prompts. A fresh bufio.Reader per call would
	// discard whatever the previous call buffered, silently losing input.
	reader *bufio.Reader

	// writeErr records the first failed write. A prompt whose output never
	// reached the user must not be treated as answered, so every method that
	// can return an error surfaces this rather than continuing blind.
	writeErr error
}

// NewTextPrompter returns a TextPrompter on stdin/stdout.
func NewTextPrompter() *TextPrompter {
	return &TextPrompter{In: os.Stdin, Out: os.Stdout}
}

var _ Prompter = (*TextPrompter)(nil)

func (p *TextPrompter) out() io.Writer {
	if p.Out == nil {
		return os.Stdout
	}
	return p.Out
}

// printf writes to the output, remembering the first failure. Subsequent
// writes are skipped: once the terminal is gone, further output is noise.
func (p *TextPrompter) printf(format string, args ...any) {
	if p.writeErr != nil {
		return
	}
	if _, err := fmt.Fprintf(p.out(), format, args...); err != nil {
		p.writeErr = err
	}
}

// flushErr returns and clears any recorded write failure.
func (p *TextPrompter) flushErr() error {
	err := p.writeErr
	p.writeErr = nil
	return err
}

func (p *TextPrompter) in() io.Reader {
	if p.In == nil {
		return os.Stdin
	}
	return p.In
}

// inTTY returns the input as a terminal file descriptor, if it is one. Raw-mode
// masking is only possible when it is.
func (p *TextPrompter) inTTY() (*os.File, bool) {
	f, ok := p.in().(*os.File)
	if !ok {
		return nil, false
	}
	return f, term.IsTerminal(int(f.Fd()))
}

// readLine reads one line. It returns io.EOF alongside any final partial line
// so callers can distinguish "the user pressed enter on an empty prompt" from
// "there is no more input" — without that distinction, a re-prompting loop
// never terminates.
func (p *TextPrompter) readLine() (string, error) {
	if p.reader == nil {
		p.reader = bufio.NewReader(p.in())
	}
	line, err := p.reader.ReadString('\n')
	trimmed := strings.TrimSpace(line)
	if errors.Is(err, io.EOF) {
		return trimmed, io.EOF
	}
	if err != nil {
		return "", err
	}
	return trimmed, nil
}

// Notify implements Prompter.
func (p *TextPrompter) Notify(level Level, format string, args ...any) {
	prefix := ""
	switch level {
	case LevelWarn:
		prefix = "warning: "
	case LevelError:
		prefix = "error: "
	case LevelInfo:
	}
	p.printf("%s%s\n", prefix, fmt.Sprintf(format, args...))
}

func (p *TextPrompter) renderChoices(title string, choices []Choice) {
	p.printf("\n%s\n", title)
	for i, c := range choices {
		if c.Detail != "" {
			p.printf("  %d) %s — %s\n", i+1, c.Label, c.Detail)
		} else {
			p.printf("  %d) %s\n", i+1, c.Label)
		}
	}
}

// Select implements Prompter.
func (p *TextPrompter) Select(title string, choices []Choice, defaultIdx int) (int, error) {
	if len(choices) == 0 {
		return 0, fmt.Errorf("wizard: Select called with no choices")
	}
	if defaultIdx < 0 || defaultIdx >= len(choices) {
		defaultIdx = 0
	}
	for {
		p.renderChoices(title, choices)
		p.printf("Select [1-%d] (default %d): ", len(choices), defaultIdx+1)
		line, err := p.readLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		exhausted := errors.Is(err, io.EOF)
		if line == "" {
			return defaultIdx, p.flushErr()
		}
		n, convErr := strconv.Atoi(line)
		if convErr == nil && n >= 1 && n <= len(choices) {
			return n - 1, p.flushErr()
		}
		if exhausted {
			return 0, fmt.Errorf("wizard: input exhausted while selecting")
		}
		p.Notify(LevelWarn, "enter a number between 1 and %d", len(choices))
	}
}

// MultiSelect implements Prompter. Input is a comma-separated list of indices;
// an empty line accepts the preselection.
func (p *TextPrompter) MultiSelect(title string, choices []Choice, preselected []int) ([]int, error) {
	if len(choices) == 0 {
		return nil, nil
	}
	for {
		p.renderChoices(title, choices)
		p.printf("Select any (comma-separated, e.g. 1,3; blank for none): ")
		line, err := p.readLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		exhausted := errors.Is(err, io.EOF)
		if line == "" {
			return preselected, p.flushErr()
		}
		var out []int
		ok := true
		for _, part := range strings.Split(line, ",") {
			n, convErr := strconv.Atoi(strings.TrimSpace(part))
			if convErr != nil || n < 1 || n > len(choices) {
				ok = false
				break
			}
			out = append(out, n-1)
		}
		if ok {
			return out, p.flushErr()
		}
		if exhausted {
			return nil, fmt.Errorf("wizard: input exhausted while selecting")
		}
		p.Notify(LevelWarn, "enter comma-separated numbers between 1 and %d", len(choices))
	}
}

// Confirm implements Prompter.
func (p *TextPrompter) Confirm(question string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	p.printf("%s %s: ", question, hint)
	line, err := p.readLine()
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if wErr := p.flushErr(); wErr != nil {
		return false, wErr
	}
	switch strings.ToLower(line) {
	case "":
		return def, nil
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// Input implements Prompter.
func (p *TextPrompter) Input(prompt, def string) (string, error) {
	if def != "" {
		p.printf("%s (default %s): ", prompt, def)
	} else {
		p.printf("%s: ", prompt)
	}
	line, err := p.readLine()
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if wErr := p.flushErr(); wErr != nil {
		return "", wErr
	}
	if line == "" {
		return def, nil
	}
	return line, nil
}

// renderSecret produces the masked view of an in-progress credential entry.
//
// The last four runes are revealed LIVE, so a paste is confirmed the instant it
// lands and the key is identifiable without pressing Enter. Below
// minRevealRunes nothing is revealed: on a short or partially-pasted value,
// four runes would be most of the secret.
//
// This is split from the terminal I/O so it is testable without a TTY.
func renderSecret(entered []rune) string {
	if len(entered) < minRevealRunes {
		return strings.Repeat("•", len(entered))
	}
	return strings.Repeat("•", maskGlyphs) + string(entered[len(entered)-revealTail:])
}

// Secret implements Prompter with live masking.
//
// Raw mode is REQUIRED for live masking and it fails on Git Bash / mintty. That
// fallback is not optional: without it those users cannot configure at all. On
// failure this prints the documented notice and falls back to a plain visible
// read — the behaviour prepare-commit-msg has today, preserved here as the
// shared baseline.
//
// The live tail is a deliberate exposure: four characters are on screen for the
// whole entry, visible in screen shares and recordings. See MADR 0004
// revision 2, "Accepted trade-off".
func (p *TextPrompter) Secret(prompt string) (string, error) {
	for {
		value, err := p.secretOnce(prompt)
		if value != "" {
			return value, nil
		}
		if err != nil {
			// Includes io.EOF: input is exhausted, so re-prompting would spin
			// forever. Report rather than loop.
			return "", fmt.Errorf("wizard: no value entered: %w", err)
		}
		p.Notify(LevelWarn, "empty value; please try again")
	}
}

func (p *TextPrompter) secretOnce(prompt string) (string, error) {
	f, isTTY := p.inTTY()
	if !isTTY {
		// Not a terminal (piped input, CI, an injected reader): read plainly.
		// Nothing is echoed by us, so there is no masking to do.
		p.printf("%s: ", prompt)
		return p.readLine()
	}
	fd := int(f.Fd())

	state, err := term.MakeRaw(fd)
	if err != nil {
		// Git Bash / mintty and similar: raw mode is unavailable. Falling
		// back keeps configuration possible; failing here would not.
		p.printf("(hidden input unavailable: %v — typing will be visible)\n", err)
		p.printf("%s: ", prompt)
		return p.readLine()
	}
	defer func() {
		// A terminal left in raw mode is unusable, so surface the failure
		// rather than discarding it.
		if rErr := term.Restore(fd, state); rErr != nil {
			p.Notify(LevelError, "failed to restore terminal mode: %v", rErr)
		}
	}()

	p.printf("%s: ", prompt)
	var entered []rune
	buf := make([]byte, 1)
	for {
		n, readErr := f.Read(buf)
		if readErr != nil || n == 0 {
			p.printf("\n")
			return string(entered), readErr
		}
		switch b := buf[0]; b {
		case '\r', '\n':
			p.printf("\n")
			return string(entered), nil
		case 3: // Ctrl-C
			p.printf("\n")
			return "", fmt.Errorf("wizard: cancelled")
		case 127, 8: // DEL, Backspace — remove one RUNE, not one byte
			if len(entered) > 0 {
				entered = entered[:len(entered)-1]
			}
		default:
			if b < 32 { // other control bytes, incl. escape sequences
				continue
			}
			entered = append(entered, rune(b))
		}
		// Redraw the whole line so the revealed tail updates as it moves.
		p.printf("\r\033[K%s: %s", prompt, renderSecret(entered))
	}
}
