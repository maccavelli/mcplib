package fastpath

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestContained is the #6 regression: containment must use a real path boundary,
// not a string prefix (which a sibling like "<dir>-evil" would satisfy).
func TestContained(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator)+"home", "u", "brain")
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join(dir, "a", "b.json"), true},
		{dir, true},
		{dir + "-evil", false},                 // sibling sharing the prefix
		{filepath.Join(dir, "..", "x"), false}, // traversal out
		{filepath.Join(string(filepath.Separator)+"home", "u", "other"), false},
	}
	for _, tc := range cases {
		if got := contained(filepath.Clean(tc.path), dir); got != tc.want {
			t.Errorf("contained(%q, %q) = %v, want %v", filepath.Clean(tc.path), dir, got, tc.want)
		}
	}
}

// TestArtifactRouting_AtomicWrite confirms a successful write lands the artifact
// and leaves no temp file behind.
func TestArtifactRouting_AtomicWrite(t *testing.T) {
	t.Setenv("MCP_ORCHESTRATOR_OWNED", "true")
	dir := t.TempDir() // under os.TempDir(), so it passes containment
	path := filepath.Join(dir, "out.json")

	type In struct {
		ArtifactPath string `json:"artifact_path"`
	}
	type Out struct {
		Data string `json:"data"`
	}
	next := func(_ context.Context, _ *mcp.CallToolRequest, _ In) (*mcp.CallToolResult, Out, error) {
		return &mcp.CallToolResult{}, Out{Data: "hello-artifact"}, nil
	}
	mw := WithArtifactRouting[In, Out]()(next)

	_, _, err := mw(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}}, In{ArtifactPath: path})
	if err != nil {
		t.Fatalf("middleware error: %v", err)
	}
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatalf("artifact not written: %v", rerr)
	}
	if !strings.Contains(string(b), "hello-artifact") {
		t.Errorf("unexpected artifact content: %s", b)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
