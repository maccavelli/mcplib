//go:build live_gateways

package llmprovider

import (
	"os"
	"strings"
	"testing"
)

// TestLive_GrokListingTextOnly: xAI's live listing yields no media model.
func TestLive_GrokListingTextOnly(t *testing.T) {
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY unset")
	}
	ctx, cancel := liveCtx(t)
	defer cancel()
	cat, err := ListModelCatalog(ctx, ProviderGrok, key)
	if err != nil || !cat.Live {
		t.Skipf("listing unavailable: live=%t err=%v", cat.Live, err)
	}
	for _, id := range cat.Usable {
		for _, media := range []string{"imagine", "video", "image", "voice", "tts", "stt"} {
			if strings.Contains(id, media) {
				t.Errorf("usable %q is a media model", id)
			}
		}
	}
	t.Logf("usable: %v", cat.Usable)
}
