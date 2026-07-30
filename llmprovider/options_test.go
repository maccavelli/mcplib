package llmprovider

import (
	"net/http"
	"testing"
	"time"
)

// TestApplyOptions_DefaultTimeout is the #5 regression: the default client must
// have a timeout (the old http.DefaultClient had none).
func TestApplyOptions_DefaultTimeout(t *testing.T) {
	cfg := ApplyOptions(nil)
	if cfg.HTTPClient == nil {
		t.Fatal("default HTTPClient is nil")
	}
	if cfg.HTTPClient.Timeout != 60*time.Second {
		t.Errorf("default timeout: got %v want 60s", cfg.HTTPClient.Timeout)
	}
}

func TestApplyOptions_WithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 5 * time.Second}
	cfg := ApplyOptions([]ProviderOption{WithHTTPClient(custom)})
	if cfg.HTTPClient != custom {
		t.Error("WithHTTPClient override not honored")
	}
}
