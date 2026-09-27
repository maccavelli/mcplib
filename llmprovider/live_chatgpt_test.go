//go:build live_gateways

package llmprovider

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// liveChatGPTSession borrows the Codex CLI's ChatGPT login read-only. It
// REQUIRES MCPLIB_LIVE_CHATGPT=1 (every call spends the subscription) and
// $CODEX_HOME/auth.json (default ~/.codex). The session can never refresh:
// its refresh token is empty and its token URL unroutable, so the CLI's
// refresh token is never used or rotated (MADR 0012 §5). It skips when the
// access token has under ten minutes left.
func liveChatGPTSession(t *testing.T) *OAuthSession {
	t.Helper()
	if os.Getenv("MCPLIB_LIVE_CHATGPT") != "1" {
		t.Skip("MCPLIB_LIVE_CHATGPT unset: live ChatGPT calls spend the subscription")
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		dir, err := os.UserHomeDir()
		if err != nil {
			t.Skip(err)
		}
		home = filepath.Join(dir, ".codex")
	}
	raw, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		t.Skipf("no Codex CLI login: %v", err)
	}
	var file struct {
		Tokens struct {
			Access    string `json:"access_token"`
			AccountID string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &file); err != nil || file.Tokens.Access == "" {
		t.Skipf("Codex CLI auth.json has no ChatGPT access token (%v)", err)
	}
	parts := strings.Split(file.Tokens.Access, ".")
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if len(parts) != 3 {
		t.Skip("access token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &claims) != nil {
		t.Skip("access token claims unreadable")
	}
	expiry := time.Unix(claims.Exp, 0)
	if time.Until(expiry) < 10*time.Minute {
		t.Skip("Codex CLI access token expires within ten minutes; run codex to refresh it")
	}
	return &OAuthSession{
		Provider:  ProviderOpenAI,
		Access:    file.Tokens.Access,
		Expiry:    expiry,
		Issuer:    DefaultOpenAIIssuer,
		ClientID:  DefaultOpenAIClientID,
		AccountID: file.Tokens.AccountID,
		TokenURL:  "http://127.0.0.1:1/never-refresh",
	}
}

// TestLive_ChatGPTGenerate is gate G-C's end-to-end check through mcplib: a
// text call, a forced tool call, and a tool round trip replayed with
// store:false, on the backend's current default model.
func TestLive_ChatGPTGenerate(t *testing.T) {
	session := liveChatGPTSession(t)
	ctx, cancel := liveCtx(t)
	defer cancel()
	p, err := NewOpenAIWithSource(session, "gpt-6-astra")
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
	skipIfTransient(t, err)
	if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
		t.Fatalf("Generate = %q, %v", out, err)
	}
	tool := Tool{Name: "get_weather", Description: "Get the weather for a city", Schema: map[string]any{
		"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}, "required": []string{"city"}}}
	res, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris?"})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("GenerateItemsWithTool: %v", err)
	}
	var call *FunctionCallItem
	for _, it := range res.Output {
		if fc, ok := it.(FunctionCallItem); ok {
			call = &fc
		}
	}
	if call == nil || !strings.Contains(strings.ToLower(call.Arguments), "paris") {
		t.Fatalf("tool call = %+v", call)
	}
	final, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris?"},
		*call, FunctionCallOutputItem{CallID: call.CallID, Output: `{"forecast":"sunny, 21C"}`})
	skipIfTransient(t, err)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if text := strings.ToLower(final.OutputText()); !strings.Contains(text, "sunny") && !strings.Contains(text, "21") {
		t.Errorf("reply %q does not use the tool result", final.OutputText())
	}
}
