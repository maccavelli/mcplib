package llmprovider

import (
	"net/http"
	"testing"
	"time"
)

// TestDefaultHTTPClient_Timeouts pins MADR 0012 §1.3: the references allow a
// generation 300 s to first byte, and a slow model must not be cut off at 30 s
// (0013 D6).
func TestDefaultHTTPClient_Timeouts(t *testing.T) {
	client := defaultHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout != 300*time.Second || client.Timeout != 330*time.Second {
		t.Fatalf("ResponseHeaderTimeout/Timeout = %s/%s, want 5m0s/5m30s",
			transport.ResponseHeaderTimeout, client.Timeout)
	}
}
