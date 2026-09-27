package wizard

import (
	"context"
	"testing"
	"time"

	"github.com/maccavelli/mcplib/llmprovider"
)

// TestConfigureLLM_KeepsFedRAMP: a kept ChatGPT session keeps its FedRAMP
// flag in the Result, which consumers persist and hand back as Existing.
func TestConfigureLLM_KeepsFedRAMP(t *testing.T) {
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderOpenAI), 1},
		confirms: []bool{true},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{
			Provider:     llmprovider.ProviderOpenAI,
			Kind:         CredOAuth,
			AccessToken:  "existing-access-abcd",
			RefreshToken: "existing-refresh",
			TokenExpiry:  time.Now().Add(time.Hour),
			Issuer:       llmprovider.DefaultOpenAIIssuer,
			ClientID:     llmprovider.DefaultOpenAIClientID,
			AccountID:    "acct_test",
			FedRAMP:      true,
			Model:        "kept-chatgpt-model",
		},
		TokenStore: newMemoryTokenStore(),
	})
	if err != nil {
		t.Fatalf("ConfigureLLM() error = %v", err)
	}
	if !res.FedRAMP {
		t.Fatal("Result.FedRAMP = false, want the kept session's true")
	}
}
