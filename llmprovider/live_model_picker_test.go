//go:build live_gateways

package llmprovider

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

// Candidate groups that share what the Kilo live tests need.
var (
	// kiloFreeCollecting are free models that require data collection
	// (every free text model did on 2026-09-27, gate G-K).
	kiloFreeCollecting = []string{"kilo-auto/free", "nvidia/nemotron-3.5-lightning:free", "poolside/laguna-s-2.1:free"}
	// kiloNonTraining are paid models that do not train on prompts, so they
	// answer with the default data_collection "deny".
	kiloNonTraining = []string{"deepseek/deepseek-v4.1-flash", "z-ai/glm-5.3-flash"}
)

// liveMetadata is models.opencode.ai's document, fetched once per run for the
// picker. It is read directly, so the providers under test still run without
// metadata (TestMain turns theirs off).
var liveMetadata = sync.OnceValues(func() (pickerDoc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultModelMetadataURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model picker: %s returned HTTP %d", defaultModelMetadataURL, resp.StatusCode)
	}
	return decodePickerDoc(resp.Body)
})

// liveModel picks, at run time, the first candidate the document lists as
// active for provider: OpenCode Zen, OpenCode Go or Kilo (MADR 0012 §3.4).
// Candidates share what the test needs (a route, a thinking shape, a
// data-collection policy). An unreachable document keeps the first
// candidate; no active candidate skips the test.
func liveModel(t *testing.T, provider string, candidates ...string) string {
	t.Helper()
	doc, err := liveMetadata()
	if err != nil {
		t.Logf("model metadata unreachable (%v); using %s", err, candidates[0])
		return candidates[0]
	}
	model, ok := firstActiveModel(doc, provider, candidates)
	if !ok {
		t.Skipf("no active %s model among %v (MADR 0012 §3.4)", provider, candidates)
	}
	t.Logf("picked %s from %v", model, candidates)
	return model
}

// TestLive_ModelPickerSkipsDeprecated: Go's glm-5 is deprecated in the
// 2026-09-27 document, so the picker takes the next candidate.
func TestLive_ModelPickerSkipsDeprecated(t *testing.T) {
	if got := liveModel(t, ProviderOpencodeGo, "glm-5", "glm-5.3-flash"); got != "glm-5.3-flash" {
		t.Fatalf("picked %q, want glm-5.3-flash", got)
	}
}

// TestLive_ModelPickerKilo: the document's kilo section drives Kilo's picks,
// and a model it does not list is passed over. It fails, rather than skips,
// when the section is missing.
func TestLive_ModelPickerKilo(t *testing.T) {
	doc, err := liveMetadata()
	if err != nil {
		t.Skipf("model metadata unreachable: %v", err)
	}
	want := kiloNonTraining[0]
	got, ok := firstActiveModel(doc, ProviderKilo, []string{"kilo-test/no-such-model", want})
	if !ok || got != want {
		t.Fatalf("picked %q (%t) from %d kilo models, want %s", got, ok, len(doc[ProviderKilo]), want)
	}
}
