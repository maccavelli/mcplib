package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTextReporterSanitizesControls(t *testing.T) {
	var buf bytes.Buffer
	r := NewTextReporter(&buf)
	err := r.Report(context.Background(), Event{
		Kind:    EventSelected,
		Product: "demo\nX",
		Current: "v1.0.0",
		Target:  "v1.1.0",
		Asset:   "demo-linux-amd64",
		Detail:  "ok\x1b[31mred\r\nmore",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\nX") || strings.ContainsRune(got, '\r') {
		t.Fatalf("unsanitized: %q", got)
	}
	if !strings.Contains(got, "selfupdate: selected") {
		t.Fatalf("%q", got)
	}
}

func TestTextReporterWriteError(t *testing.T) {
	r := NewTextReporter(failWriter{})
	if err := r.Report(context.Background(), Event{Kind: EventInstalling}); err == nil {
		t.Fatal("expected write error")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
