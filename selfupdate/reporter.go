package selfupdate

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

type textReporter struct {
	w io.Writer
}

// NewTextReporter writes stable plain-text progress without ANSI escapes.
func NewTextReporter(w io.Writer) Reporter {
	return textReporter{w: w}
}

func (r textReporter) Report(_ context.Context, ev Event) error {
	if r.w == nil {
		return fmt.Errorf("selfupdate: reporter writer is nil")
	}
	line := formatEvent(ev)
	_, err := io.WriteString(r.w, line+"\n")
	return err
}

func formatEvent(ev Event) string {
	var b strings.Builder
	b.WriteString("selfupdate: ")
	b.WriteString(sanitizeText(ev.Kind.String()))
	if ev.Product != "" {
		b.WriteString(" product=")
		b.WriteString(sanitizeText(ev.Product))
	}
	if ev.Current != "" {
		b.WriteString(" current=")
		b.WriteString(sanitizeText(ev.Current))
	}
	if ev.Target != "" {
		b.WriteString(" target=")
		b.WriteString(sanitizeText(ev.Target))
	}
	if ev.Asset != "" {
		b.WriteString(" asset=")
		b.WriteString(sanitizeText(ev.Asset))
	}
	if ev.Bytes != 0 {
		b.WriteString(" bytes=")
		b.WriteString(strconv.FormatInt(ev.Bytes, 10))
	}
	if ev.Detail != "" {
		b.WriteString(" ")
		b.WriteString(sanitizeText(ev.Detail))
	}
	return b.String()
}

func sanitizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r < 32 || r == 127 || (r >= 0x80 && r <= 0x9f):
			b.WriteByte('?')
		case r == utf8.RuneError && size == 1:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
