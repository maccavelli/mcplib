package llmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGrokDevice_SlowDownIncreasesInterval(t *testing.T) {
	t.Parallel()

	clock := newFakeOAuthClock()
	var tokenCalls int
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, fmt.Sprintf(`{"device_authorization_endpoint":%q,"token_endpoint":%q}`, srv.URL+"/device", srv.URL+"/token"))
	})
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, `{"device_code":"device-secret","user_code":"ABCD-EFGH","verification_uri":"https://example.test/activate","expires_in":60,"interval":1}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse token form: %v", err)
		}
		if r.PostForm.Get("grant_type") != deviceAuthorizationGrantType {
			t.Errorf("grant_type = %q", r.PostForm.Get("grant_type"))
		}
		tokenCalls++
		if tokenCalls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			writeTestJSON(t, w, `{"error":"slow_down"}`)
			return
		}
		writeTestJSON(t, w, `{"access_token":"grok-access","refresh_token":"grok-refresh","expires_in":3600}`)
	})

	var notifiedURL, notifiedCode string
	session, err := LoginDeviceOAuth(context.Background(), ProviderGrok, OAuthFlowOptions{
		HTTPClient: srv.Client(),
		ClientID:   "test-client",
		Issuer:     srv.URL,
		NotifyDevice: func(verificationURL, userCode string) {
			notifiedURL, notifiedCode = verificationURL, userCode
		},
		now:   clock.now,
		sleep: clock.sleep,
	})
	if err != nil {
		t.Fatalf("LoginDeviceOAuth() error = %v", err)
	}
	if session.Access != "grok-access" || session.TokenURL != srv.URL+"/token" {
		t.Fatalf("session = %#v", session)
	}
	if notifiedURL != "https://example.test/activate" || notifiedCode != "ABCD-EFGH" {
		t.Fatalf("notification = (%q, %q)", notifiedURL, notifiedCode)
	}
	if got, want := clock.sleeps(), []time.Duration{time.Second, 6 * time.Second}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sleep sequence = %v, want %v", got, want)
	}
}

func TestGrokDevice_StopsAtExpiry(t *testing.T) {
	t.Parallel()

	clock := newFakeOAuthClock()
	var tokenCalls int
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, fmt.Sprintf(`{"device_authorization_endpoint":%q,"token_endpoint":%q}`, srv.URL+"/device", srv.URL+"/token"))
	})
	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, `{"device_code":"device-secret","user_code":"ABCD-EFGH","verification_uri":"https://example.test/activate","expires_in":1,"interval":1}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		tokenCalls++
		w.WriteHeader(http.StatusBadRequest)
		writeTestJSON(t, w, `{"error":"authorization_pending"}`)
	})

	_, err := LoginDeviceOAuth(context.Background(), ProviderGrok, OAuthFlowOptions{
		HTTPClient: srv.Client(),
		ClientID:   "test-client",
		Issuer:     srv.URL,
		now:        clock.now,
		sleep:      clock.sleep,
	})
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("error = %v, want expiry error", err)
	}
	if tokenCalls > 1 {
		t.Fatalf("token calls = %d, want at most 1 before expiry", tokenCalls)
	}
}

func TestOpenAIDevice_UsesCodexProtocol(t *testing.T) {
	t.Parallel()

	clock := newFakeOAuthClock()
	var pollCalls int
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/api/accounts/deviceauth/usercode", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode user-code request: %v", err)
		}
		if body["client_id"] != "test-client" {
			t.Errorf("client_id = %q", body["client_id"])
		}
		writeTestJSON(t, w, `{"device_auth_id":"device-auth-id","user_code":"OPEN-AI","interval":"2"}`)
	})
	mux.HandleFunc("/api/accounts/deviceauth/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode poll request: %v", err)
		}
		if body["device_auth_id"] != "device-auth-id" || body["user_code"] != "OPEN-AI" {
			t.Errorf("poll body = %v", body)
		}
		pollCalls++
		if pollCalls == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeTestJSON(t, w, `{"authorization_code":"authorization-code","code_challenge":"challenge","code_verifier":"server-verifier"}`)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse exchange form: %v", err)
		}
		want := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {"authorization-code"},
			"client_id":     {"test-client"},
			"redirect_uri":  {srv.URL + "/deviceauth/callback"},
			"code_verifier": {"server-verifier"},
		}
		if !reflect.DeepEqual(r.PostForm, want) {
			t.Errorf("exchange form = %v, want %v", r.PostForm, want)
		}
		writeTestJSON(t, w, `{"access_token":"openai-access","refresh_token":"openai-refresh","expires_in":3600}`)
	})

	var notification [2]string
	session, err := LoginDeviceOAuth(context.Background(), ProviderOpenAI, OAuthFlowOptions{
		HTTPClient: srv.Client(),
		ClientID:   "test-client",
		Issuer:     srv.URL,
		NotifyDevice: func(verificationURL, userCode string) {
			notification = [2]string{verificationURL, userCode}
		},
		now:   clock.now,
		sleep: clock.sleep,
	})
	if err != nil {
		t.Fatalf("LoginDeviceOAuth() error = %v", err)
	}
	if notification != [2]string{srv.URL + "/codex/device", "OPEN-AI"} {
		t.Fatalf("notification = %v", notification)
	}
	if session.Access != "openai-access" || session.Issuer != srv.URL || session.Provider != ProviderOpenAI {
		t.Fatalf("session = %#v", session)
	}
	if got, want := clock.sleeps(), []time.Duration{2 * time.Second}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sleep sequence = %v, want %v", got, want)
	}
}

type fakeOAuthClock struct {
	mu        sync.Mutex
	current   time.Time
	durations []time.Duration
}

func newFakeOAuthClock() *fakeOAuthClock {
	return &fakeOAuthClock{current: time.Unix(1_700_000_000, 0)}
}

func (c *fakeOAuthClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *fakeOAuthClock) sleep(ctx context.Context, duration time.Duration) error {
	c.mu.Lock()
	c.current = c.current.Add(duration)
	c.durations = append(c.durations, duration)
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (c *fakeOAuthClock) sleeps() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.durations...)
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}
