package llmprovider

import (
	"context"
	"strings"
	"sync"
	"time"
)

// probeGenerateHealth runs a tiny generate against each candidate and returns
// those that respond successfully, preserving preferred order.
// At most MaxListedModels candidates are probed. Failures are skipped.
func probeGenerateHealth(ctx context.Context, preferred []string, generate func(ctx context.Context, modelID string) (string, error)) []string {
	if len(preferred) == 0 {
		return nil
	}
	candidates := preferred
	if len(candidates) > MaxListedModels {
		candidates = candidates[:MaxListedModels]
	}

	type result struct {
		id    string
		ok    bool
		index int
	}
	ch := make(chan result, len(candidates))
	var wg sync.WaitGroup

	for i, id := range candidates {
		wg.Add(1)
		go func(idx int, modelID string) {
			defer wg.Done()
			tCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			out, err := generate(tCtx, modelID)
			ok := err == nil && strings.Contains(strings.ToLower(out), "hello")
			ch <- result{id: modelID, ok: ok, index: idx}
		}(i, id)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	healthy := make(map[string]struct{})
	for r := range ch {
		if r.ok {
			healthy[strings.ToLower(r.id)] = struct{}{}
		}
	}

	var out []string
	for _, id := range candidates {
		if _, ok := healthy[strings.ToLower(id)]; ok {
			out = append(out, id)
		}
	}
	return out
}
