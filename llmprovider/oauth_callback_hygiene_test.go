package llmprovider

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// lifeSciencesState is the state ChatGPT may return for an onboarding entry
// point (codex login/src/callback_params.rs:1, server.rs:366-374).
const lifeSciencesState = "expected-state.onboarding_entrypoint=life_sciences"

// TestOAuthCallback_AcceptsLifeSciencesSuffix: the loopback and a pasted
// callback URL both take the suffixed state.
func TestOAuthCallback_AcceptsLifeSciencesSuffix(t *testing.T) {
	result := make(chan oauthCallbackResult, 1)
	recorder := httptest.NewRecorder()
	oauthCallbackHandler("/callback", "expected-state", result, "").ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "http://127.0.0.1/callback?code=abc&state="+lifeSciencesState, http.NoBody))
	select {
	case got := <-result:
		if got.err != nil || got.code != "abc" {
			t.Fatalf("callback = %+v, want code abc", got)
		}
	default:
		t.Fatal("callback did not complete the waiter")
	}
	code, err := parseOAuthInput("http://localhost:1455/auth/callback?code=def&state="+lifeSciencesState, "expected-state")
	if err != nil || code != "def" {
		t.Fatalf("parseOAuthInput = %q, %v; want def", code, err)
	}
}

// TestOAuthCallback_OnlyTheExactSuffix: any other suffix is still a mismatch.
func TestOAuthCallback_OnlyTheExactSuffix(t *testing.T) {
	for _, state := range []string{"expected-state.onboarding_entrypoint=other", "expected-statex", "other" + lifeSciencesState[len("expected-state"):]} {
		assertCallbackCompletesWithError(t, "code=abc&state="+state, "state mismatch")
		if _, err := parseOAuthInput("http://localhost/cb?code=abc&state="+state, "expected-state"); err == nil ||
			!strings.Contains(err.Error(), "state mismatch") {
			t.Errorf("parseOAuthInput(state %q) = %v, want a state mismatch", state, err)
		}
	}
}

// TestOAuthCallback_SurfacesErrorDescription: the IdP's description reaches
// the user, as Codex prints it (server.rs:934-956).
func TestOAuthCallback_SurfacesErrorDescription(t *testing.T) {
	assertCallbackCompletesWithError(t, "state=expected-state&error=access_denied&error_description=Your+plan+is+inactive",
		"Your plan is inactive")
}

// TestOAuthCallback_MissingCodexEntitlement: Codex's entitlement refusal
// gets Codex's explanation (server.rs:934-944).
func TestOAuthCallback_MissingCodexEntitlement(t *testing.T) {
	assertCallbackCompletesWithError(t,
		"state=expected-state&error=access_denied&error_description=missing_codex_entitlement",
		"Codex is not enabled for your workspace")
	if _, err := parseOAuthInput("http://localhost/cb?state=expected-state&error=access_denied&error_description=missing_codex_entitlement",
		"expected-state"); err == nil || !strings.Contains(err.Error(), "Codex is not enabled for your workspace") {
		t.Fatalf("parseOAuthInput = %v, want the entitlement explanation", err)
	}
}
