package selfupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestTerminalConfirmerRequiresTTY(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	c := NewTerminalConfirmer(r, io.Discard)
	_, err = c.Confirm(context.Background(), Prompt{Product: "demo", Current: "v1", Target: "v2", Operation: OperationUpgrade})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestTerminalConfirmerYesNo(t *testing.T) {
	orig := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = orig })

	t.Run("yes", func(t *testing.T) {
		in, out := pipeFile(t, "yes\n")
		var buf strings.Builder
		c := NewTerminalConfirmer(in, &buf)
		ok, err := c.Confirm(context.Background(), Prompt{Product: "demo", Current: "v1.0.0", Target: "v1.1.0", Operation: OperationUpgrade})
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if !strings.Contains(buf.String(), "upgrade") {
			t.Fatalf("prompt = %q", buf.String())
		}
		_ = out
	})
	t.Run("no", func(t *testing.T) {
		in, _ := pipeFile(t, "n\n")
		c := NewTerminalConfirmer(in, io.Discard)
		ok, err := c.Confirm(context.Background(), Prompt{Product: "demo", Current: "v1.0.0", Target: "v1.1.0", Operation: OperationUpgrade})
		if err != nil || ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
}

func pipeFile(t *testing.T, input string) (in *os.File, w *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return r, w
}
