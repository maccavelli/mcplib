package mcplib

import (
	"strings"
	"testing"
)

func TestLogBuffer_Write(t *testing.T) {
	lb := new(LogBuffer)
	lb.Write([]byte("line1\nline2\nline3\n"))

	out := lb.String()
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line3") {
		t.Errorf("expected all lines, got: %s", out)
	}
}

func TestLogBuffer_Redaction(t *testing.T) {
	lb := NewLogBuffer()
	lb.Write([]byte("connecting with token_abc123xyz and sk_live_456\n"))
	out := lb.String()
	if strings.Contains(out, "token_abc123xyz") {
		t.Error("expected token to be redacted")
	}
	if strings.Contains(out, "sk_live_456") {
		t.Error("expected sk_ secret to be redacted")
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Error("expected [REDACTED] markers in output")
	}
	if !strings.Contains(out, "connecting with") {
		t.Error("expected non-secret text preserved")
	}
}

func TestLogBuffer_Trimming(t *testing.T) {
	lb := NewLogBuffer(WithBufferSize(1024, 512))
	// Write enough to exceed 1024 bytes
	line := strings.Repeat("x", 99) + "\n" // 100 bytes per line
	for range 15 {
		lb.Write([]byte(line))
	}
	out := lb.String()
	if len(out) > 1024 {
		t.Errorf("buffer should be trimmed, got %d bytes", len(out))
	}
	if len(out) < 512 {
		t.Errorf("buffer trimmed too aggressively, got %d bytes", len(out))
	}
}

func TestLogBuffer_IOWriterContract(t *testing.T) {
	lb := NewLogBuffer()
	input := []byte("secret_key_abcdef123 was rotated")
	n, err := lb.Write(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write() returned %d, want %d (len of original input per io.Writer contract)", n, len(input))
	}
}

func TestLogBuffer_ZeroValue(t *testing.T) {
	lb := &LogBuffer{} // zero-value, no constructor
	lb.Write([]byte("hello world\n"))
	out := lb.String()
	if out != "hello world\n" {
		t.Errorf("zero-value write failed, got: %q", out)
	}
	// Verify defaults were applied
	if lb.maxSize != DefaultLogBufferLimit {
		t.Errorf("expected maxSize=%d, got %d", DefaultLogBufferLimit, lb.maxSize)
	}
}

func TestLogBuffer_WithBufferSize(t *testing.T) {
	lb := NewLogBuffer(WithBufferSize(4096, 2048))
	if lb.maxSize != 4096 {
		t.Errorf("expected maxSize=4096, got %d", lb.maxSize)
	}
	if lb.trimTarget != 2048 {
		t.Errorf("expected trimTarget=2048, got %d", lb.trimTarget)
	}
}

func TestTailLines(t *testing.T) {
	s := "a\nb\nc\nd\ne"
	got := TailLines(s, 3)
	if got != "c\nd\ne" {
		t.Errorf("expected 'c\\nd\\ne', got: %s", got)
	}
}

func TestTailLines_TrailingNewline(t *testing.T) {
	s := "a\nb\nc\nd\ne\n"
	got := TailLines(s, 3)
	// Zero-alloc backward scan strips trailing newline, returns last 3 content lines.
	if got != "c\nd\ne" {
		t.Errorf("expected 'c\\nd\\ne', got: %q", got)
	}
}

func TestTailLines_Short(t *testing.T) {
	s := "a\nb"
	got := TailLines(s, 5)
	if got != s {
		t.Errorf("expected original string, got: %s", got)
	}
}

func TestTailLines_Empty(t *testing.T) {
	got := TailLines("", 5)
	if got != "" {
		t.Errorf("expected empty string, got: %q", got)
	}
}

func TestTailLines_OnlyNewline(t *testing.T) {
	got := TailLines("\n", 5)
	if got != "" {
		t.Errorf("expected empty string for bare newline, got: %q", got)
	}
}

func TestNopReadCloser_Close(t *testing.T) {
	nrc := NopReadCloser{Reader: strings.NewReader("test")}
	if err := nrc.Close(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestNopWriteCloser_Close(t *testing.T) {
	var sb strings.Builder
	nwc := NopWriteCloser{Writer: &sb}
	if err := nwc.Close(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestPagination_Apply(t *testing.T) {
	tests := []struct {
		name     string
		p        Pagination
		length   int
		expected [2]int
	}{
		{"empty", Pagination{}, 100, [2]int{0, 100}},
		{"offset", Pagination{Offset: 10}, 100, [2]int{10, 100}},
		{"limit", Pagination{Limit: 20}, 100, [2]int{0, 20}},
		{"offset_limit", Pagination{Offset: 10, Limit: 20}, 100, [2]int{10, 30}},
		{"out_of_bounds", Pagination{Offset: 200}, 100, [2]int{100, 100}},
		{"negative_offset", Pagination{Offset: -10}, 100, [2]int{0, 100}},
		{"extreme_limit", Pagination{Limit: 2000}, 100, [2]int{0, 100}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := tt.p.Apply(tt.length)
			if start != tt.expected[0] || end != tt.expected[1] {
				t.Errorf("got (%d,%d), want (%d,%d)", start, end, tt.expected[0], tt.expected[1])
			}
		})
	}
}
