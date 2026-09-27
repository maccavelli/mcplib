package llmprovider

import "testing"

// TestKilo_DataCollectionAllowed: WithKiloDataCollection(true) sends no
// provider preference, as Kilo's client does without its privacy setting.
func TestKilo_DataCollectionAllowed(t *testing.T) {
	if provider, ok := kiloBody(t, WithKiloDataCollection(true))["provider"]; ok {
		t.Fatalf("provider = %v, want absent", provider)
	}
}
