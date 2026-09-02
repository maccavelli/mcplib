package selfupdate

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

var isTerminal = term.IsTerminal

type terminalConfirmer struct {
	in  *os.File
	out io.Writer
}

// NewTerminalConfirmer prompts on out and reads from in. A non-terminal input
// returns ErrConfirmationRequired instead of hanging.
func NewTerminalConfirmer(in *os.File, out io.Writer) Confirmer {
	return terminalConfirmer{in: in, out: out}
}

func (c terminalConfirmer) Confirm(ctx context.Context, p Prompt) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if c.in == nil {
		return false, fmt.Errorf("selfupdate: confirmation input is nil: %w", ErrConfirmationRequired)
	}
	if !isTerminal(int(c.in.Fd())) {
		return false, fmt.Errorf("selfupdate: pass --yes to apply without a TTY: %w", ErrConfirmationRequired)
	}
	if c.out == nil {
		return false, fmt.Errorf("selfupdate: confirmation output is nil")
	}
	prompt := fmt.Sprintf("selfupdate: %s %s from %s to %s? [y/N] ",
		sanitizeText(p.Operation.String()),
		sanitizeText(p.Product),
		sanitizeText(p.Current),
		sanitizeText(p.Target),
	)
	if _, err := io.WriteString(c.out, prompt); err != nil {
		return false, err
	}
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		s := bufio.NewScanner(c.in)
		if !s.Scan() {
			if err := s.Err(); err != nil {
				errCh <- err
				return
			}
			errCh <- io.EOF
			return
		}
		lineCh <- s.Text()
	}()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case err := <-errCh:
		return false, err
	case line := <-lineCh:
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		default:
			return false, nil
		}
	}
}
