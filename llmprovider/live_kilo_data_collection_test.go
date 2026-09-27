//go:build live_gateways

package llmprovider

import (
	"errors"
	"testing"
)

// TestLive_KiloDataCollectionDenied is gate G-K's assertion: with the default
// deny, a free model that requires data collection (kilo-auto/free) is refused
// with a terminal ErrNotPermitted carrying data_collection_required.
func TestLive_KiloDataCollectionDenied(t *testing.T) {
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewKilo(kiloKey(t), "kilo-auto/free")
	if err != nil {
		t.Fatalf("NewKilo: %v", err)
	}
	_, err = p.Generate(ctx, "Reply with only the word ALPHA")
	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrProviderUnavailable) {
		t.Skipf("transient: %v", err)
	}
	var apiErr *APIError
	if !errors.Is(err, ErrNotPermitted) || !errors.As(err, &apiErr) || apiErr.Type != "data_collection_required" {
		t.Fatalf("err = %v, want ErrNotPermitted with type data_collection_required", err)
	}
}
