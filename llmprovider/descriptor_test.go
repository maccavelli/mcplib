package llmprovider

import (
	"strings"
	"testing"
)

// TestDescriptors_CoverEveryRegisteredProvider is the load-bearing test of this
// whole design. If a provider can be constructed but has no descriptor, no
// wizard can offer it — which is exactly how Grok shipped in MADR 0001 and
// reached none of the three configuration wizards. Making that a build failure
// is the point.
func TestDescriptors_CoverEveryRegisteredProvider(t *testing.T) {
	described := make(map[string]ProviderDescriptor, len(Descriptors()))
	for _, d := range Descriptors() {
		described[d.ID] = d
	}

	for id := range ProviderEnvVars {
		if _, ok := described[id]; !ok {
			t.Errorf("provider %q is in ProviderEnvVars but has no descriptor: "+
				"no wizard can offer it", id)
		}
	}
	for id, d := range described {
		if !d.RequiresAPIKey {
			continue
		}
		if _, ok := ProviderEnvVars[id]; !ok {
			t.Errorf("descriptor %q requires an API key but has no ProviderEnvVars entry", id)
		}
	}
}

func TestDescriptors_DerivedFieldsMatchSource(t *testing.T) {
	for _, d := range Descriptors() {
		if d.RequiresAPIKey && d.EnvVar != ProviderEnvVars[d.ID] {
			t.Errorf("%s: EnvVar = %q, want %q from ProviderEnvVars",
				d.ID, d.EnvVar, ProviderEnvVars[d.ID])
		}
		want := StaticModels(d.ID)
		if len(d.StaticModels) != len(want) {
			t.Errorf("%s: StaticModels has %d entries, want %d from StaticModels()",
				d.ID, len(d.StaticModels), len(want))
			continue
		}
		for i := range want {
			if d.StaticModels[i] != want[i] {
				t.Errorf("%s: StaticModels[%d] = %q, want %q", d.ID, i, d.StaticModels[i], want[i])
			}
		}
	}
}

// TestDescriptors_StableOrderAndDefensiveCopy guards menu stability and ensures
// a caller cannot corrupt the catalog for everyone else.
func TestDescriptors_StableOrderAndDefensiveCopy(t *testing.T) {
	a, b := Descriptors(), Descriptors()
	if len(a) != len(b) {
		t.Fatalf("unstable length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("unstable order at %d: %q vs %q", i, a[i].ID, b[i].ID)
		}
	}

	a[0].Label = "MUTATED"
	if len(a[0].StaticModels) > 0 {
		a[0].StaticModels[0] = "MUTATED"
	}
	c := Descriptors()
	if c[0].Label == "MUTATED" {
		t.Error("Descriptors() must return a defensive copy: Label was mutated")
	}
	if len(c[0].StaticModels) > 0 && c[0].StaticModels[0] == "MUTATED" {
		t.Error("Descriptors() must return a defensive copy: StaticModels was mutated")
	}
}

func TestDescriptorFor(t *testing.T) {
	d, ok := DescriptorFor(ProviderKilo)
	if !ok {
		t.Fatal("DescriptorFor(kilo) not found")
	}
	if d.EnvVar != "KILO_API_KEY" || !d.SupportsBaseURL {
		t.Errorf("kilo descriptor = %+v", d)
	}
	if _, ok := DescriptorFor("nope"); ok {
		t.Error("DescriptorFor should miss on an unknown id")
	}
}

func TestModelLabel(t *testing.T) {
	if got := ModelLabel(ProviderClaude, "claude-haiku-4-5"); !strings.Contains(got, "Haiku") {
		t.Errorf("ModelLabel = %q, want a human label", got)
	}
	// Unknown models degrade to the bare id rather than disappearing.
	if got := ModelLabel(ProviderKilo, "some/unlisted-model"); got != "some/unlisted-model" {
		t.Errorf("unknown model label = %q, want the bare id", got)
	}
	if got := ModelLabel(ProviderGemini, ""); got != "" {
		t.Errorf("empty model label = %q, want empty", got)
	}
}

// TestDescriptors_NoStaleModels is the direct regression for the bug that
// motivated this MADR: mcp-server-magictools recommended gemini-2.0-flash while
// models_catalog.go documents the 2.0 and 1.5 families as shut down.
func TestDescriptors_NoStaleModels(t *testing.T) {
	for _, d := range Descriptors() {
		for _, m := range d.StaticModels {
			for _, dead := range []string{"gemini-2.0-", "gemini-1.5-"} {
				if strings.Contains(m, dead) {
					t.Errorf("%s offers %q, a shut-down model family (models_catalog.go:26)", d.ID, m)
				}
			}
		}
	}
}
