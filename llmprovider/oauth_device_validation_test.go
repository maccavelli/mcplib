package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// grokDeviceLogin runs a Grok device login against a stub issuer whose
// device endpoint answers with device, and whose token endpoint succeeds at
// once. It reports the error and whether the user was shown anything.
func grokDeviceLogin(t *testing.T, device map[string]any) (notified bool, err error) {
	t.Helper()
	clock := newFakeOAuthClock()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, fmt.Sprintf(`{"device_authorization_endpoint":%q,"token_endpoint":%q}`, srv.URL+"/device", srv.URL+"/token"))
	})
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		raw, _ := json.Marshal(device)
		writeTestJSON(t, w, string(raw))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, `{"access_token":"grok-access","refresh_token":"grok-refresh","expires_in":3600}`)
	})
	_, err = LoginDeviceOAuth(context.Background(), ProviderGrok, OAuthFlowOptions{
		HTTPClient:   srv.Client(),
		ClientID:     "test-client",
		Issuer:       srv.URL,
		NotifyDevice: func(string, string) { notified = true },
		now:          clock.now,
		sleep:        clock.sleep,
	})
	return notified, err
}

func grokDevice(userCode, uri, complete string) map[string]any {
	d := map[string]any{"device_code": "dc", "user_code": userCode, "verification_uri": uri, "expires_in": 60, "interval": 1}
	if complete != "" {
		d["verification_uri_complete"] = complete
	}
	return d
}

// TestGrokDevice_RejectsUnsafeResponses: a malicious issuer cannot put
// control characters in front of the user or send them off https, as the
// Grok CLI guards (xai-grok-login/src/device_code.rs:148-155, :474-487).
func TestGrokDevice_RejectsUnsafeResponses(t *testing.T) {
	for name, device := range map[string]map[string]any{
		"control user_code": grokDevice("AB\x1b[2JCD", "https://accounts.x.ai/device", ""),
		"space user_code":   grokDevice("AB CD", "https://accounts.x.ai/device", ""),
		"javascript uri":    grokDevice("ABCD-EFGH", "javascript:alert(1)", ""),
		"plain http uri":    grokDevice("ABCD-EFGH", "http://evil.test/device", ""),
		"control in uri":    grokDevice("ABCD-EFGH", "https://accounts.x.ai/device\x07", ""),
		"bad complete uri":  grokDevice("ABCD-EFGH", "https://accounts.x.ai/device", "ftp://evil.test/device"),
		"http complete uri": grokDevice("ABCD-EFGH", "https://accounts.x.ai/device", "http://evil.test/device?user_code=ABCD-EFGH"),
	} {
		t.Run(name, func(t *testing.T) {
			notified, err := grokDeviceLogin(t, device)
			if err == nil || notified || !strings.Contains(err.Error(), "invalid") {
				t.Fatalf("err = %v, notified = %t; want an invalid-response error before any notice", err, notified)
			}
		})
	}
}

// TestGrokDevice_AcceptsHTTPSAndLoopback: https, and http to a loopback
// host, pass.
func TestGrokDevice_AcceptsHTTPSAndLoopback(t *testing.T) {
	for _, device := range []map[string]any{
		grokDevice("ABCD-efgh-1234", "https://accounts.x.ai/device", "https://accounts.x.ai/device?user_code=ABCD-efgh-1234"),
		grokDevice("ABCD-EFGH", "http://localhost:22255/device", ""),
		grokDevice("ABCD-EFGH", "http://127.0.0.1:22255/device", ""),
	} {
		if notified, err := grokDeviceLogin(t, device); err != nil || !notified {
			t.Fatalf("device %v: err = %v, notified = %t; want success", device, err, notified)
		}
	}
}
