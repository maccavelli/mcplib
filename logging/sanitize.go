package logging

import (
	"io"
)

// SanitizingWriter wraps an io.Writer and redacts secrets before writing.
type SanitizingWriter struct {
	inner io.Writer
}

// NewSanitizingWriter creates a SanitizingWriter that redacts secrets from all
// output before forwarding to w.
func NewSanitizingWriter(w io.Writer) *SanitizingWriter {
	return &SanitizingWriter{inner: w}
}

// Write redacts secrets from p before forwarding to the inner writer.
func (sw *SanitizingWriter) Write(p []byte) (int, error) {
	_, err := sw.inner.Write(Redact(p))
	return len(p), err
}
