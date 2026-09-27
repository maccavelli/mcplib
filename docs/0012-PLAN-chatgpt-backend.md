---
status: proposed
date: 2026-09-27
associated-madr: "0012-MADR-conform-providers-to-reference-clients.md"
decision-makers: mcplib maintainers
---

# Implement 0012 §4 — The ChatGPT Backend

Associated MADR: [0012-MADR-conform-providers-to-reference-clients.md](0012-MADR-conform-providers-to-reference-clients.md)
(accepted 2026-09-27, revision 3). This is the fourth of that MADR's six
plans (§8).

This plan executes MADR §4 as amended in revision 3, and nothing else. If a
fact contradicts the MADR or this plan, **stop and prompt**. Add a dated
entry to §9 of this plan, amend the MADR when a decision or an asserted fact
changes, and only then continue.

## How this plan was proven

**Gate G-C ran first.** It used the Codex CLI's ChatGPT login on the
maintainer's machine, read-only: the access token was read and never
refreshed, so the CLI's refresh token was never used. Its results are in the
MADR's revision 3. Every phase below was then executed on 2026-09-27, in
scratch copies of `git archive 7ea0ad4` with the §2 and §3 plans applied:
* **Red.** Each phase's tests diff was applied, and each named test was seen
  to fail on the unfixed code. That includes the live tests, which fail
  against the real backend.
* **Green.** The fix diff was applied, and the full gate (§0.2) passed.
* **Mutants.** A guard that holds today, or a test of new API, was seen to
  fail on a named mutant.
* **Live.** The ChatGPT live tests ran through `mcplib` on the same
  read-only login.
* **Diffs.** Appendix B's diffs were generated mechanically from that proof.
  Applying all five plans' diffs in §8's order to a fresh `7ea0ad4` archive
  reproduces the proven tree byte for byte: 308 files, 0 mismatches.

## Goal

A ChatGPT session works. Today every ChatGPT generation fails with
`openai HTTP 400` and no message (Appendix A.C1). After this plan:
* **Requests** carry Codex's shape: `stream: true`, `store: false`,
  `include: ["reasoning.encrypted_content"]`, `prompt_cache_key` and
  `session-id`, with no `max_output_tokens`.
* **Streams** are read whatever their `Content-Type`, and their failures map
  onto the typed errors.
* **`Continue`** fails fast: with `store: false` there is nothing to chain
  from.
* **The listing** shows every model the account may use. It sends
  `mcplib`'s own release as `client_version`, which reveals `gpt-6-sol` and
  `gpt-6-luna`.
* **Errors** carry the backend's `{"detail"}` text and `resets_at`.
* **FedRAMP** accounts send `X-OpenAI-Fedramp: true`, as Codex does.

## Scope

**In scope:** MADR §4.1–§4.4 as amended: C1–C3 below.

**Out of scope:**
* Streaming for any other provider. §4.1 limits the reader to ChatGPT.
* A static ChatGPT catalog, which D11 removed (MADR revision 2).
* The `127.0.0.1` redirect, which is gated in `0012-PLAN-oauth-hygiene.md`
  phase O6.

## 0. Preconditions and conventions

### 0.1 Baseline

* `0012-PLAN-gateway-conventions.md` complete on `main`. On any other base
  the diffs need rebasing, which is a §9 deviation.
* `go test -count=1 ./...` passes before C1.

### 0.2 Gate (every phase)

1. `gofmt -l` prints nothing, and `golint -set_exit_status` passes, on each
   `.go` file the phase touched.
2. `go vet ./...` and `go vet -tags live_gateways ./llmprovider`.
3. `make lint`.
4. `go test -count=1 ./...` and `go test -race -count=1 ./llmprovider ./wizard`.

### 0.3 Red first

As in `0012-PLAN-item-fidelity.md` §0.3.

### 0.4 Credentials for live steps

* The ChatGPT live tests require `MCPLIB_LIVE_CHATGPT=1`, because every call
  spends the subscription. They also need a Codex CLI login at
  `$CODEX_HOME/auth.json` (default `~/.codex/auth.json`).
* `liveChatGPTSession` borrows that login read-only:
  * its refresh token is empty and its token URL unroutable, so it can
    never refresh;
  * it skips when the access token has under ten minutes left.

### 0.5 Commits

One `git commit --no-edit` per phase, after the gate. No push and no tag.

## Phase C0 — Start

1. Confirm §0.1.
2. Set this plan to `status: in-progress`.
3. Commit the documents only.

## Phase C1 — Codex's request shape and an event-stream reader (§4.1–§4.2)

**Today.**
* A ChatGPT session's body has `max_output_tokens`, no `stream` and no
  `store`, and the response is decoded as one JSON document (`openai.go`,
  `doGenerateItemsOnce`). Gate G-C: the backend rejects all three body
  choices.
* `Continue` chains `previous_response_id`, which has nothing to chain to.
* `TestOpenAI_ChatGPTSendsMaxOutputTokens` pins the defect. It answers the
  OAuth MADR's open question 1 the wrong way.

**Changes** (Appendix B.C1):
* **Body and headers.** For a ChatGPT session the body sends `stream: true`,
  `store: false`, `include: ["reasoning.encrypted_content"]` and
  `prompt_cache_key` (the provider's session id), and drops
  `max_output_tokens`. The headers add `Accept: text/event-stream` and
  `session-id` (Codex `codex-api/src/requests/headers.rs:8`).
* **Reader.** `readResponsesStream` (new, in `http_helpers.go`):
  * it reads `data:` payloads, taking the event type from the payload, never
    from `Content-Type`;
  * each `response.output_item.done` adds its item, and
    `response.created` and `response.completed` supply the id;
  * `response.failed` maps as Codex's `parse_failed_response` does;
  * `response.incomplete` is an `*IncompleteError`;
  * a stream that ends before `response.completed` is a retryable
    `ErrProviderUnavailable`.
* **Shared decoding.** `decodeResponsesAPIOutput`'s per-item decoding moves
  to `Response.appendOutput`, which both decoders share.
* **Stream failures.** They are an `*APIError` with `Status` 0 and no
  status sentinel.
* **`Continue`** returns `ErrInvalidRequest` for a ChatGPT session, without
  a request.
* **Test updates:**
  * the pin test becomes `TestOpenAI_ChatGPTOmitsMaxOutputTokens`;
  * the test helper reframes a JSON body as a stream when asked for one.
* **Fixtures.** Gate G-C's three recordings become testdata. Their
  account-derived `safety_identifier` and per-request `prompt_cache_key` are
  replaced with placeholders.

**Verification.**
* Red:
  * four unit tests, including the recorded streams and every failure path;
  * live `TestLive_ChatGPTGenerate`, which fails today with `openai HTTP
    400`.
* The API-key guard is proven by a mutant.
* Live after the fix:
  * a text call;
  * a forced tool call;
  * a tool round trip replayed with `store: false`.

## Phase C2 — The listing sends `mcplib`'s own release (§4.3, G-C item 7)

**Today.** The listing sends `client_version=0.0.0`, which hides `gpt-6-sol`
and `gpt-6-luna` (`minimal_client_version` 0.155.0).

**Changes** (Appendix B.C2):
* `chatgptClientVersion` reads `X.Y.Z` from `mcplib`'s module version. A
  pseudo-version gives the release it precedes.
* The listing sends that. A build with no release version sends `0.0.0`.
  The backend rejects `v1.5.0`, `1.5`, `(devel)` and anything over 32
  characters.

**Verification.**
* Red: `TestChatGPTListing_SendsMcplibVersion`, and live
  `TestLive_ChatGPTListingVersion`.
* The fallback guard is proven by two mutants.
* Live, the `1.5.0` listing holds every `0.0.0` model plus `gpt-6-sol` and
  `gpt-6-luna`, and each added model generates.

## Phase C3 — The error envelope, `resets_at` and FedRAMP (§4.4)

**Today.**
* **Error envelope.** The backend's `{"detail": …}` errors carry no message.
* **Usage limits.** `usage_limit_reached` ignores `resets_at`.
* **FedRAMP.** No header is sent, and the flag is not kept. It lives in the
  id token (Codex `login/src/token_data.rs`).

**Changes** (Appendix B.C3):
* **Errors:**
  * `parseAPIErrorBody` reads a top-level `detail`;
  * `resets_at` sets `RetryAfter` when no header does.
* **FedRAMP:**
  * `chatGPTFedRAMP` reads the claim at login;
  * `OAuthSession.FedRAMP` carries it through refresh, the token file
    (`fedramp`, omitted when false) and `wizard.Result.FedRAMP`;
  * generation and the listing send `X-OpenAI-Fedramp: true` for such a
    session.
* **Residency header.** Its source, OpenCode's Codex plugin, is recorded in
  a comment.

**Verification.**
* Red: three unit tests, and live `TestLive_ChatGPTErrorDetail`, whose
  unknown model yields an error with no message today.
* Guards proven by mutants:
  * a session without the claim sends no header;
  * the token store keeps the flag;
  * the wizard keeps it both ways.
* **No live FedRAMP run.** No FedRAMP account was available.

## Phase C4 — Records and close-out

1. Record each phase's result in §10.
2. Set this plan to `status: complete` once §7 holds.
3. Commit the documents only.

## 7. Acceptance criteria

* Every Appendix A red test fails before its fix and passes after it.
* Every mutant is killed.
* The gate passes after each phase.
* With `MCPLIB_LIVE_CHATGPT=1`, every ChatGPT live test passes.

## 8. Rollout and rollback

* **Behaviour change:** ChatGPT sessions go from always failing to working.
  `Continue` on a ChatGPT session now fails fast instead of failing at the
  server.
* **Additive API:** `OAuthSession.FedRAMP` and `wizard.Result.FedRAMP`. A
  consumer that persists `Result` should persist the new field.
* **Rollback:** revert the phase commit. Reverting C1 restores the broken
  ChatGPT path.

## 9. Deviation log

None yet.

## 10. Execution record

Not executed. The proof in Appendix A was made in scratch copies before
execution.

## Appendix A — Proof record (2026-09-27)

### A.C1 Phase C1 — Codex's request shape and an event-stream reader (§4.1–§4.2)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestOpenAIChatGPT_SendsCodexRequest` | unit | stream/store/prompt_cache_key = <nil>/<nil>/<nil>, want true/false/sess-42 |
| `TestOpenAIChatGPT_DecodesRecordedStreams` | unit | GenerateItems: invalid character 'e' looking for beginning of value |
| `TestOpenAIChatGPT_StreamFailures` | unit | err = invalid character 'd' looking for beginning of value, want llm: not permitted |
| `TestOpenAIChatGPT_ContinueIsInvalid` | unit | err = invalid character 'e' looking for beginning of value after 1 requests, want ErrInvalidRequest and none |
| `TestLive_ChatGPTGenerate` | live | Generate = "", llm: invalid request: openai HTTP 400 |

**Guards before the fix:**

* `TestOpenAIPlatform_KeepsJSONRequest`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 7 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `cg1-codex-shape-everywhere` | `TestOpenAIPlatform_KeepsJSONRequest` | killed | max_output_tokens = <nil>, want 321 |

### A.C2 Phase C2 — The listing sends `mcplib`'s own release (§4.3, G-C item 7)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestChatGPTListing_SendsMcplibVersion` | unit | client_version = "0.0.0", want "1.5.0" |
| `TestLive_ChatGPTListingVersion` | live | client_version sent = [0.0.0 0.0.0], want [0.0.0 1.5.0] |

**Guards before the fix:**

* `TestChatGPTListing_DevelBuildSendsFallback`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 3 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `cg2-raw-version` | `TestChatGPTListing_DevelBuildSendsFallback` | killed | client_version = "(devel)", want 0.0.0 |
| `cg2-two-part-accepted` | `TestChatGPTListing_DevelBuildSendsFallback` | killed | client_version = "1.5", want 0.0.0 |

### A.C3 Phase C3 — The error envelope, `resets_at` and FedRAMP (§4.4)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestClassify_DetailEnvelope` | unit | err = llm: invalid request: openai HTTP 400 (message ""), want ErrInvalidRequest carrying the detail |
| `TestClassify_UsageLimitResetsAt` | unit | RetryAfter = 0s, want about an hour |
| `TestOpenAIChatGPT_FedRAMPHeader` | unit | X-OpenAI-Fedramp = "" (present false), want true |
| `TestLive_ChatGPTErrorDetail` | live | err = llm: invalid request: openai HTTP 400, want ErrInvalidRequest carrying the backend's detail |

**Guards before the fix:**

* `TestOpenAIChatGPT_NoFedRAMPHeader`: passed on the unfixed code.
* `TestFileTokenStore_KeepsFedRAMP`: added with the fix.
* `TestConfigureLLM_KeepsFedRAMP`: added with the fix.

**Gate** (fix applied): all passed — per-file `golint` on 15 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `cg3-header-always` | `TestOpenAIChatGPT_NoFedRAMPHeader` | killed | X-OpenAI-Fedramp = "true", want absent |
| `cg3-store-drops-flag` | `TestFileTokenStore_KeepsFedRAMP` | killed | Load = &{Provider:openai Access:a Refresh:r Expiry:2026-09-27 17:49:48.498002 -0500 CDT Issuer:https://auth.openai.com ClientID:app_EMoamEEZ73f0CkXaXp7hrann AccountID: FedRAMP:false TokenURL: Store:0x538ed992d40 HTTPClie |
| `cg3-wizard-drops-flag` | `TestConfigureLLM_KeepsFedRAMP` | killed | Result.FedRAMP = false, want the kept session's true |
| `cg3-rebuild-drops-flag` | `TestConfigureLLM_KeepsFedRAMP` | killed | Result.FedRAMP = false, want the kept session's true |

### A.L Live runs on this plan's final tree (`-tags live_gateways`, 2026-09-27)

| Test | Result |
|---|---|
| `TestLive_ChatGPTErrorDetail` | PASS |
| `TestLive_ChatGPTListingVersion` | PASS |
| `TestLive_ChatGPTGenerate` | PASS |


Totals: 3 passed, 0 skipped, 0 failed.


## Appendix B — Diffs

Generated from the proof. Apply each phase's **Tests** diff, then its **Fix**
diff, with `git apply`, in order, on the §0.1 baseline.

### B.C1 Phase C1 — Codex's request shape and an event-stream reader (§4.1–§4.2)

**Tests** (`cg1-tests.diff`, 462 lines):

```diff
diff --git a/llmprovider/chatgpt_stream_test.go b/llmprovider/chatgpt_stream_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/chatgpt_stream_test.go
@@ -0,0 +1,230 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"io"
+	"net/http"
+	"os"
+	"slices"
+	"strings"
+	"sync"
+	"testing"
+	"time"
+)
+
+// chatGPTCapture records what a stubbed backend received.
+type chatGPTCapture struct {
+	mu     sync.Mutex
+	calls  int
+	header http.Header
+	body   map[string]any
+}
+
+// chatGPTClient answers every request with reply and no Content-Type, as the
+// ChatGPT backend sends its event stream (gate G-C, 2026-09-27).
+func chatGPTClient(t *testing.T, reply string) (*http.Client, *chatGPTCapture) {
+	t.Helper()
+	c := &chatGPTCapture{}
+	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		var body map[string]any
+		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
+			t.Errorf("decode request: %v", err)
+		}
+		c.mu.Lock()
+		c.calls++
+		c.header, c.body = r.Header.Clone(), body
+		c.mu.Unlock()
+		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{},
+			Body: io.NopCloser(strings.NewReader(reply)), Request: r}, nil
+	})}, c
+}
+
+func chatGPTTestSession() *OAuthSession {
+	return &OAuthSession{Issuer: DefaultOpenAIIssuer, Access: "sess", Refresh: "refresh", Expiry: time.Now().Add(time.Hour)}
+}
+
+func chatGPTFixture(t *testing.T, name string) string {
+	t.Helper()
+	raw, err := os.ReadFile("testdata/" + name)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return string(raw)
+}
+
+// TestOpenAIChatGPT_SendsCodexRequest: the backend rejects a non-streaming
+// request, one without store:false and one with max_output_tokens (gate G-C);
+// Codex also sends include, prompt_cache_key and session-id.
+func TestOpenAIChatGPT_SendsCodexRequest(t *testing.T) {
+	client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
+	p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra",
+		WithHTTPClient(client), WithMaxTokens(321), WithSessionID("sess-42"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	_, genErr := p.Generate(context.Background(), "hello")
+	c.mu.Lock()
+	defer c.mu.Unlock()
+	if c.body["stream"] != true || c.body["store"] != false || c.body["prompt_cache_key"] != "sess-42" {
+		t.Errorf("stream/store/prompt_cache_key = %v/%v/%v, want true/false/sess-42",
+			c.body["stream"], c.body["store"], c.body["prompt_cache_key"])
+	}
+	if include, _ := c.body["include"].([]any); !slices.Equal(include, []any{"reasoning.encrypted_content"}) {
+		t.Errorf("include = %v", c.body["include"])
+	}
+	if v, ok := c.body["max_output_tokens"]; ok {
+		t.Errorf("max_output_tokens = %v, want absent", v)
+	}
+	if c.header.Get("Accept") != "text/event-stream" || c.header.Get("session-id") != "sess-42" {
+		t.Errorf("Accept = %q, session-id = %q", c.header.Get("Accept"), c.header.Get("session-id"))
+	}
+	if genErr != nil {
+		t.Errorf("Generate: %v", genErr)
+	}
+}
+
+// TestOpenAIChatGPT_DecodesRecordedStreams replays gate G-C's recordings
+// (2026-09-27, gpt-6-astra; account identifiers redacted).
+func TestOpenAIChatGPT_DecodesRecordedStreams(t *testing.T) {
+	for _, tc := range []struct {
+		fixture, id, text string
+		call              *FunctionCallItem
+	}{
+		{"chatgpt-text.sse", "resp_01ae07221376cd90016ab96e72ca3c87d1a3c12bb5aaf6db92", "ok", nil},
+		{"chatgpt-reasoning.sse", "resp_047ea2fee4cf7ab6016ab96e77a3f087d1ad70bca0c3b0cd8d", "391", nil},
+		{"chatgpt-tool.sse", "resp_059e69aebb680141016ab96e74543487d1af68b2af975de82f", "",
+			&FunctionCallItem{CallID: "call_SAcT3cGrAgagMgcNJEcgK62C", Name: "record", Arguments: `{"subject":"fix: probe"}`}},
+	} {
+		t.Run(tc.fixture, func(t *testing.T) {
+			client, _ := chatGPTClient(t, chatGPTFixture(t, tc.fixture))
+			p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client))
+			if err != nil {
+				t.Fatal(err)
+			}
+			res, err := p.GenerateItems(context.Background(), MessageItem{Role: jsonRoleUser, Text: "hi"})
+			if err != nil {
+				t.Fatalf("GenerateItems: %v", err)
+			}
+			if res.ID != tc.id || res.OutputText() != tc.text {
+				t.Errorf("ID = %q, text = %q; want %q, %q", res.ID, res.OutputText(), tc.id, tc.text)
+			}
+			var calls []FunctionCallItem
+			for _, it := range res.Output {
+				if fc, ok := it.(FunctionCallItem); ok {
+					calls = append(calls, fc)
+				}
+			}
+			if (tc.call == nil) != (len(calls) == 0) || (tc.call != nil && (len(calls) != 1 || calls[0] != *tc.call)) {
+				t.Errorf("function calls = %+v, want %+v", calls, tc.call)
+			}
+		})
+	}
+}
+
+// sseEvents renders data-only events, as the backend frames them.
+func sseEvents(events ...string) string {
+	var b strings.Builder
+	for _, e := range events {
+		b.WriteString("data: " + e + "\n\n")
+	}
+	return b.String()
+}
+
+const sseCreated = `{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`
+
+func sseFailed(code string) string {
+	return `{"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"` + code +
+		`","message":"` + code + ` happened"}}}`
+}
+
+// TestOpenAIChatGPT_StreamFailures: response.failed codes map onto §1.1 as
+// Codex's parse_failed_response classifies them, response.incomplete onto
+// §1.5, and a stream that ends before response.completed is retryable.
+func TestOpenAIChatGPT_StreamFailures(t *testing.T) {
+	for _, tc := range []struct {
+		name, stream string
+		want         error
+		terminal     bool
+	}{
+		{"usage_not_included", sseEvents(sseCreated, sseFailed("usage_not_included")), ErrNotPermitted, true},
+		{"usage_limit_reached", sseEvents(sseCreated, sseFailed("usage_limit_reached")), ErrQuotaExhausted, true},
+		{"context_length_exceeded", sseEvents(sseCreated, sseFailed("context_length_exceeded")), ErrInvalidRequest, true},
+		{"rate_limit_exceeded", sseEvents(sseCreated, sseFailed("rate_limit_exceeded")), ErrRateLimited, false},
+		{"server_error", sseEvents(sseCreated, sseFailed("server_error")), ErrProviderUnavailable, false},
+		{"truncated", sseEvents(sseCreated), ErrProviderUnavailable, false},
+	} {
+		t.Run(tc.name, func(t *testing.T) {
+			client, _ := chatGPTClient(t, tc.stream)
+			p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client))
+			if err != nil {
+				t.Fatal(err)
+			}
+			_, err = p.Generate(context.Background(), "hi")
+			if !errors.Is(err, tc.want) {
+				t.Fatalf("err = %v, want %v", err, tc.want)
+			}
+			if !errors.Is(tc.want, ErrInvalidRequest) && errors.Is(err, ErrInvalidRequest) {
+				t.Errorf("err = %v also matches ErrInvalidRequest", err)
+			}
+			var apiErr *APIError
+			if errors.As(err, &apiErr) && apiErr.Terminal != tc.terminal {
+				t.Errorf("Terminal = %t, want %t", apiErr.Terminal, tc.terminal)
+			}
+		})
+	}
+	t.Run("incomplete", func(t *testing.T) {
+		client, _ := chatGPTClient(t, sseEvents(sseCreated,
+			`{"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`))
+		p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client))
+		if err != nil {
+			t.Fatal(err)
+		}
+		_, err = p.Generate(context.Background(), "hi")
+		var inc *IncompleteError
+		if !errors.As(err, &inc) || inc.Reason != "max_output_tokens" {
+			t.Fatalf("err = %v, want *IncompleteError max_output_tokens", err)
+		}
+	})
+}
+
+// TestOpenAIChatGPT_ContinueIsInvalid: with store:false there is no response
+// to chain from, so Continue fails without a request (MADR 0012 §4.2).
+func TestOpenAIChatGPT_ContinueIsInvalid(t *testing.T) {
+	client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
+	p, err := NewOpenAIWithSource(chatGPTTestSession(), "gpt-6-astra", WithHTTPClient(client))
+	if err != nil {
+		t.Fatal(err)
+	}
+	_, err = p.Continue(context.Background(), "resp_prev", MessageItem{Role: jsonRoleUser, Text: "more"})
+	c.mu.Lock()
+	defer c.mu.Unlock()
+	if !errors.Is(err, ErrInvalidRequest) || c.calls != 0 {
+		t.Fatalf("err = %v after %d requests, want ErrInvalidRequest and none", err, c.calls)
+	}
+}
+
+// TestOpenAIPlatform_KeepsJSONRequest: an API key keeps the platform shape:
+// max_output_tokens, no stream or store, a JSON response.
+func TestOpenAIPlatform_KeepsJSONRequest(t *testing.T) {
+	client, c := chatGPTClient(t, `{"id":"resp_p","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)
+	p, err := NewOpenAI("sk-test", "gpt-6-luna", WithHTTPClient(client), WithMaxTokens(321))
+	if err != nil {
+		t.Fatal(err)
+	}
+	out, err := p.Generate(context.Background(), "hi")
+	if err != nil || out != "ok" {
+		t.Fatalf("Generate = %q, %v", out, err)
+	}
+	c.mu.Lock()
+	defer c.mu.Unlock()
+	if got, _ := c.body["max_output_tokens"].(float64); got != 321 {
+		t.Errorf("max_output_tokens = %v, want 321", c.body["max_output_tokens"])
+	}
+	for _, k := range []string{"stream", "store", "include", "prompt_cache_key"} {
+		if v, ok := c.body[k]; ok {
+			t.Errorf("%s = %v, want absent", k, v)
+		}
+	}
+}
diff --git a/llmprovider/live_chatgpt_test.go b/llmprovider/live_chatgpt_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_chatgpt_test.go
@@ -0,0 +1,114 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"encoding/base64"
+	"encoding/json"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+	"time"
+)
+
+// liveChatGPTSession borrows the Codex CLI's ChatGPT login read-only. It
+// REQUIRES MCPLIB_LIVE_CHATGPT=1 (every call spends the subscription) and
+// $CODEX_HOME/auth.json (default ~/.codex). The session can never refresh:
+// its refresh token is empty and its token URL unroutable, so the CLI's
+// refresh token is never used or rotated (MADR 0012 §5). It skips when the
+// access token has under ten minutes left.
+func liveChatGPTSession(t *testing.T) *OAuthSession {
+	t.Helper()
+	if os.Getenv("MCPLIB_LIVE_CHATGPT") != "1" {
+		t.Skip("MCPLIB_LIVE_CHATGPT unset: live ChatGPT calls spend the subscription")
+	}
+	home := os.Getenv("CODEX_HOME")
+	if home == "" {
+		dir, err := os.UserHomeDir()
+		if err != nil {
+			t.Skip(err)
+		}
+		home = filepath.Join(dir, ".codex")
+	}
+	raw, err := os.ReadFile(filepath.Join(home, "auth.json"))
+	if err != nil {
+		t.Skipf("no Codex CLI login: %v", err)
+	}
+	var file struct {
+		Tokens struct {
+			Access    string `json:"access_token"`
+			AccountID string `json:"account_id"`
+		} `json:"tokens"`
+	}
+	if err := json.Unmarshal(raw, &file); err != nil || file.Tokens.Access == "" {
+		t.Skipf("Codex CLI auth.json has no ChatGPT access token (%v)", err)
+	}
+	parts := strings.Split(file.Tokens.Access, ".")
+	var claims struct {
+		Exp int64 `json:"exp"`
+	}
+	if len(parts) != 3 {
+		t.Skip("access token is not a JWT")
+	}
+	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
+	if err != nil || json.Unmarshal(payload, &claims) != nil {
+		t.Skip("access token claims unreadable")
+	}
+	expiry := time.Unix(claims.Exp, 0)
+	if time.Until(expiry) < 10*time.Minute {
+		t.Skip("Codex CLI access token expires within ten minutes; run codex to refresh it")
+	}
+	return &OAuthSession{
+		Provider:  ProviderOpenAI,
+		Access:    file.Tokens.Access,
+		Expiry:    expiry,
+		Issuer:    DefaultOpenAIIssuer,
+		ClientID:  DefaultOpenAIClientID,
+		AccountID: file.Tokens.AccountID,
+		TokenURL:  "http://127.0.0.1:1/never-refresh",
+	}
+}
+
+// TestLive_ChatGPTGenerate is gate G-C's end-to-end check through mcplib: a
+// text call, a forced tool call, and a tool round trip replayed with
+// store:false, on the backend's current default model.
+func TestLive_ChatGPTGenerate(t *testing.T) {
+	session := liveChatGPTSession(t)
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewOpenAIWithSource(session, "gpt-6-astra")
+	if err != nil {
+		t.Fatal(err)
+	}
+	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
+	skipIfTransient(t, err)
+	if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
+		t.Fatalf("Generate = %q, %v", out, err)
+	}
+	tool := Tool{Name: "get_weather", Description: "Get the weather for a city", Schema: map[string]any{
+		"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}, "required": []string{"city"}}}
+	res, err := p.GenerateItemsWithTool(ctx, tool, MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris?"})
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("GenerateItemsWithTool: %v", err)
+	}
+	var call *FunctionCallItem
+	for _, it := range res.Output {
+		if fc, ok := it.(FunctionCallItem); ok {
+			call = &fc
+		}
+	}
+	if call == nil || !strings.Contains(strings.ToLower(call.Arguments), "paris") {
+		t.Fatalf("tool call = %+v", call)
+	}
+	final, err := p.GenerateItems(ctx, MessageItem{Role: jsonRoleUser, Text: "What is the weather in Paris?"},
+		*call, FunctionCallOutputItem{CallID: call.CallID, Output: `{"forecast":"sunny, 21C"}`})
+	skipIfTransient(t, err)
+	if err != nil {
+		t.Fatalf("round trip: %v", err)
+	}
+	if text := strings.ToLower(final.OutputText()); !strings.Contains(text, "sunny") && !strings.Contains(text, "21") {
+		t.Errorf("reply %q does not use the tool result", final.OutputText())
+	}
+}
diff --git a/llmprovider/testdata/chatgpt-reasoning.sse b/llmprovider/testdata/chatgpt-reasoning.sse
new file mode 100644
--- /dev/null
+++ b/llmprovider/testdata/chatgpt-reasoning.sse
@@ -0,0 +1,27 @@
+event: response.created
+data: {"type":"response.created","response":{"id":"resp_047ea2fee4cf7ab6016ab96e77a3f087d1ad70bca0c3b0cd8d","object":"response","created_at":1790537335,"status":"in_progress","access_programs":{"cyber":"standard"},"background":false,"completed_at":null,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"low","mode":"standard","summary":"detailed"},"safety_identifier":"user-REDACTED","service_tier":"auto","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"auto","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":0}
+
+event: response.in_progress
+data: {"type":"response.in_progress","response":{"id":"resp_047ea2fee4cf7ab6016ab96e77a3f087d1ad70bca0c3b0cd8d","object":"response","created_at":1790537335,"status":"in_progress","access_programs":{"cyber":"standard"},"background":false,"completed_at":null,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"low","mode":"standard","summary":"detailed"},"safety_identifier":"user-REDACTED","service_tier":"auto","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"auto","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":1}
+
+event: response.output_item.added
+data: {"type":"response.output_item.added","item":{"id":"msg_047ea2fee4cf7ab6016ab96e78732487d19b60fdf30bbf9afb","type":"message","status":"in_progress","content":[],"phase":"final_answer","role":"assistant"},"output_index":0,"sequence_number":2}
+
+event: response.content_part.added
+data: {"type":"response.content_part.added","content_index":0,"item_id":"msg_047ea2fee4cf7ab6016ab96e78732487d19b60fdf30bbf9afb","output_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""},"sequence_number":3}
+
+event: response.output_text.delta
+data: {"type":"response.output_text.delta","content_index":0,"delta":"391","item_id":"msg_047ea2fee4cf7ab6016ab96e78732487d19b60fdf30bbf9afb","logprobs":[],"obfuscation":"CI6U1HaP5AFMl","output_index":0,"sequence_number":4}
+
+event: response.output_text.done
+data: {"type":"response.output_text.done","content_index":0,"item_id":"msg_047ea2fee4cf7ab6016ab96e78732487d19b60fdf30bbf9afb","logprobs":[],"output_index":0,"sequence_number":5,"text":"391"}
+
+event: response.content_part.done
+data: {"type":"response.content_part.done","content_index":0,"item_id":"msg_047ea2fee4cf7ab6016ab96e78732487d19b60fdf30bbf9afb","output_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":"391"},"sequence_number":6}
+
+event: response.output_item.done
+data: {"type":"response.output_item.done","item":{"id":"msg_047ea2fee4cf7ab6016ab96e78732487d19b60fdf30bbf9afb","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":"391"}],"phase":"final_answer","role":"assistant"},"output_index":0,"sequence_number":7}
+
+event: response.completed
+data: {"type":"response.completed","response":{"id":"resp_047ea2fee4cf7ab6016ab96e77a3f087d1ad70bca0c3b0cd8d","object":"response","created_at":1790537335,"status":"completed","access_programs":{"cyber":"standard"},"background":false,"completed_at":1790537336,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"low","mode":"standard","summary":"detailed"},"safety_identifier":"user-REDACTED","service_tier":"default","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"auto","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":{"attribution":{"items":{"msg_047ea2fee4cf7ab6016ab96e77b14887d1b722b65d0e9097d6":{"cache_write_tokens":0,"cached_tokens":0,"content":[{"cache_write_tokens":0,"cached_tokens":0,"input_tokens":17,"output_tokens":0}],"input_tokens":17,"output_tokens":0},"msg_047ea2fee4cf7ab6016ab96e78732487d19b60fdf30bbf9afb":{"cache_write_tokens":0,"cached_tokens":0,"content":[{"cache_write_tokens":0,"cached_tokens":0,"input_tokens":2,"output_tokens":5}],"input_tokens":2,"output_tokens":5}}},"input_tokens":19,"input_tokens_details":{"cache_write_tokens":0,"cached_tokens":0},"output_tokens":5,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":24},"user":null,"metadata":{}},"sequence_number":8}
+
diff --git a/llmprovider/testdata/chatgpt-text.sse b/llmprovider/testdata/chatgpt-text.sse
new file mode 100644
--- /dev/null
+++ b/llmprovider/testdata/chatgpt-text.sse
@@ -0,0 +1,27 @@
+event: response.created
+data: {"type":"response.created","response":{"id":"resp_01ae07221376cd90016ab96e72ca3c87d1a3c12bb5aaf6db92","object":"response","created_at":1790537330,"status":"in_progress","access_programs":{"cyber":"standard"},"background":false,"completed_at":null,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"medium","mode":"standard","summary":null},"safety_identifier":"user-REDACTED","service_tier":"auto","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"auto","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":0}
+
+event: response.in_progress
+data: {"type":"response.in_progress","response":{"id":"resp_01ae07221376cd90016ab96e72ca3c87d1a3c12bb5aaf6db92","object":"response","created_at":1790537330,"status":"in_progress","access_programs":{"cyber":"standard"},"background":false,"completed_at":null,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"medium","mode":"standard","summary":null},"safety_identifier":"user-REDACTED","service_tier":"auto","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"auto","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":1}
+
+event: response.output_item.added
+data: {"type":"response.output_item.added","item":{"id":"msg_01ae07221376cd90016ab96e73a98487d19fd3b1a140bc7a5a","type":"message","status":"in_progress","content":[],"phase":"final_answer","role":"assistant"},"output_index":0,"sequence_number":2}
+
+event: response.content_part.added
+data: {"type":"response.content_part.added","content_index":0,"item_id":"msg_01ae07221376cd90016ab96e73a98487d19fd3b1a140bc7a5a","output_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""},"sequence_number":3}
+
+event: response.output_text.delta
+data: {"type":"response.output_text.delta","content_index":0,"delta":"ok","item_id":"msg_01ae07221376cd90016ab96e73a98487d19fd3b1a140bc7a5a","logprobs":[],"obfuscation":"HEzrZkWilmQmyy","output_index":0,"sequence_number":4}
+
+event: response.output_text.done
+data: {"type":"response.output_text.done","content_index":0,"item_id":"msg_01ae07221376cd90016ab96e73a98487d19fd3b1a140bc7a5a","logprobs":[],"output_index":0,"sequence_number":5,"text":"ok"}
+
+event: response.content_part.done
+data: {"type":"response.content_part.done","content_index":0,"item_id":"msg_01ae07221376cd90016ab96e73a98487d19fd3b1a140bc7a5a","output_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":"ok"},"sequence_number":6}
+
+event: response.output_item.done
+data: {"type":"response.output_item.done","item":{"id":"msg_01ae07221376cd90016ab96e73a98487d19fd3b1a140bc7a5a","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":"ok"}],"phase":"final_answer","role":"assistant"},"output_index":0,"sequence_number":7}
+
+event: response.completed
+data: {"type":"response.completed","response":{"id":"resp_01ae07221376cd90016ab96e72ca3c87d1a3c12bb5aaf6db92","object":"response","created_at":1790537330,"status":"completed","access_programs":{"cyber":"standard"},"background":false,"completed_at":1790537331,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"medium","mode":"standard","summary":null},"safety_identifier":"user-REDACTED","service_tier":"default","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"auto","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":{"attribution":{"items":{"msg_01ae07221376cd90016ab96e72d88487d1ae43612874ffe640":{"cache_write_tokens":0,"cached_tokens":0,"content":[{"cache_write_tokens":0,"cached_tokens":0,"input_tokens":11,"output_tokens":0}],"input_tokens":11,"output_tokens":0},"msg_01ae07221376cd90016ab96e73a98487d19fd3b1a140bc7a5a":{"cache_write_tokens":0,"cached_tokens":0,"content":[{"cache_write_tokens":0,"cached_tokens":0,"input_tokens":2,"output_tokens":5}],"input_tokens":2,"output_tokens":5}}},"input_tokens":13,"input_tokens_details":{"cache_write_tokens":0,"cached_tokens":0},"output_tokens":5,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":18},"user":null,"metadata":{}},"sequence_number":8}
+
diff --git a/llmprovider/testdata/chatgpt-tool.sse b/llmprovider/testdata/chatgpt-tool.sse
new file mode 100644
--- /dev/null
+++ b/llmprovider/testdata/chatgpt-tool.sse
@@ -0,0 +1,39 @@
+event: response.created
+data: {"type":"response.created","response":{"id":"resp_059e69aebb680141016ab96e74543487d1af68b2af975de82f","object":"response","created_at":1790537332,"status":"in_progress","access_programs":{"cyber":"standard"},"background":false,"completed_at":null,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"medium","mode":"standard","summary":null},"safety_identifier":"user-REDACTED","service_tier":"auto","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"required","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[{"type":"function","description":"Record a commit subject.","name":"record","output_schema":null,"parameters":{"additionalProperties":false,"properties":{"subject":{"type":"string"}},"required":["subject"],"type":"object"},"strict":true}],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":0}
+
+event: response.in_progress
+data: {"type":"response.in_progress","response":{"id":"resp_059e69aebb680141016ab96e74543487d1af68b2af975de82f","object":"response","created_at":1790537332,"status":"in_progress","access_programs":{"cyber":"standard"},"background":false,"completed_at":null,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"medium","mode":"standard","summary":null},"safety_identifier":"user-REDACTED","service_tier":"auto","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"required","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[{"type":"function","description":"Record a commit subject.","name":"record","output_schema":null,"parameters":{"additionalProperties":false,"properties":{"subject":{"type":"string"}},"required":["subject"],"type":"object"},"strict":true}],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":null,"user":null,"metadata":{}},"sequence_number":1}
+
+event: response.output_item.added
+data: {"type":"response.output_item.added","item":{"id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","type":"function_call","status":"in_progress","arguments":"","call_id":"call_SAcT3cGrAgagMgcNJEcgK62C","name":"record"},"output_index":0,"sequence_number":2}
+
+event: response.function_call_arguments.delta
+data: {"type":"response.function_call_arguments.delta","delta":"{\"","item_id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","obfuscation":"UBDgCPPxoY8iOg","output_index":0,"sequence_number":3}
+
+event: response.function_call_arguments.delta
+data: {"type":"response.function_call_arguments.delta","delta":"subject","item_id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","obfuscation":"BYE9GPACP","output_index":0,"sequence_number":4}
+
+event: response.function_call_arguments.delta
+data: {"type":"response.function_call_arguments.delta","delta":"\":\"","item_id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","obfuscation":"23beaLCby8DGK","output_index":0,"sequence_number":5}
+
+event: response.function_call_arguments.delta
+data: {"type":"response.function_call_arguments.delta","delta":"fix","item_id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","obfuscation":"aqCtP18h5njuB","output_index":0,"sequence_number":6}
+
+event: response.function_call_arguments.delta
+data: {"type":"response.function_call_arguments.delta","delta":":","item_id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","obfuscation":"BXN4GcnOwUA3R6o","output_index":0,"sequence_number":7}
+
+event: response.function_call_arguments.delta
+data: {"type":"response.function_call_arguments.delta","delta":" probe","item_id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","obfuscation":"Z06DhjiRxI","output_index":0,"sequence_number":8}
+
+event: response.function_call_arguments.delta
+data: {"type":"response.function_call_arguments.delta","delta":"\"}","item_id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","obfuscation":"zEIjRjjZY6KpWe","output_index":0,"sequence_number":9}
+
+event: response.function_call_arguments.done
+data: {"type":"response.function_call_arguments.done","arguments":"{\"subject\":\"fix: probe\"}","item_id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","output_index":0,"sequence_number":10}
+
+event: response.output_item.done
+data: {"type":"response.output_item.done","item":{"id":"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac","type":"function_call","status":"completed","arguments":"{\"subject\":\"fix: probe\"}","call_id":"call_SAcT3cGrAgagMgcNJEcgK62C","name":"record"},"output_index":0,"sequence_number":11}
+
+event: response.completed
+data: {"type":"response.completed","response":{"id":"resp_059e69aebb680141016ab96e74543487d1af68b2af975de82f","object":"response","created_at":1790537332,"status":"completed","access_programs":{"cyber":"standard"},"background":false,"completed_at":1790537334,"error":null,"frequency_penalty":0.0,"incomplete_details":null,"instructions":null,"max_output_tokens":null,"max_tool_calls":null,"model":"gpt-6-astra","moderation":null,"output":[],"parallel_tool_calls":true,"presence_penalty":0.0,"previous_response_id":null,"prompt_cache_key":"00000000-0000-0000-0000-000000000000","prompt_cache_retention":"24h","reasoning":{"context":"all_turns","effort":"medium","mode":"standard","summary":null},"safety_identifier":"user-REDACTED","service_tier":"default","store":false,"temperature":1.0,"text":{"format":{"type":"text"},"verbosity":"medium"},"tool_choice":"required","tool_usage":{"image_gen":{"input_tokens":0,"input_tokens_details":{"image_tokens":0,"text_tokens":0},"output_tokens":0,"output_tokens_details":{"image_tokens":0,"text_tokens":0},"total_tokens":0},"web_search":{"num_requests":0}},"tools":[{"type":"function","description":"Record a commit subject.","name":"record","output_schema":null,"parameters":{"additionalProperties":false,"properties":{"subject":{"type":"string"}},"required":["subject"],"type":"object"},"strict":true}],"top_logprobs":0,"top_p":0.98,"truncation":"disabled","usage":{"attribution":{"items":{"msg_059e69aebb680141016ab96e74658887d1989f387a83d9b7fe":{"cache_write_tokens":0,"cached_tokens":0,"content":[{"cache_write_tokens":0,"cached_tokens":0,"input_tokens":15,"output_tokens":0}],"input_tokens":15,"output_tokens":0},"fc_059e69aebb680141016ab96e75dfb887d19050d87a5ae16bac":{"cache_write_tokens":0,"cached_tokens":0,"input_tokens":2,"output_tokens":19}},"request_fields":{"tools":{"cache_write_tokens":0,"cached_tokens":0,"input_tokens":34,"output_tokens":0}}},"input_tokens":51,"input_tokens_details":{"cache_write_tokens":0,"cached_tokens":0},"output_tokens":19,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":70},"user":null,"metadata":{}},"sequence_number":12}
+
```

**Fix** (`cg1-fix.diff`, 422 lines):

```diff
diff --git a/llmprovider/api_error.go b/llmprovider/api_error.go
--- a/llmprovider/api_error.go
+++ b/llmprovider/api_error.go
@@ -49,7 +49,7 @@
 // existing errors.Is check still matches (§7).
 type APIError struct {
 	Provider   string
-	Status     int
+	Status     int           // 0 for a failure reported inside a 200 event stream
 	Type       string        // the service's error type or code, e.g. "FreeUsageLimitError"
 	Message    string        // the service's message, redacted and bounded to 512 bytes
 	RetryAfter time.Duration // from Retry-After, when present
@@ -59,7 +59,11 @@
 
 func (e *APIError) Error() string {
 	var b strings.Builder
-	fmt.Fprintf(&b, "%v: %s HTTP %d", e.sentinel, e.Provider, e.Status)
+	if e.Status == 0 {
+		fmt.Fprintf(&b, "%v: %s stream", e.sentinel, e.Provider)
+	} else {
+		fmt.Fprintf(&b, "%v: %s HTTP %d", e.sentinel, e.Provider, e.Status)
+	}
 	if e.Type != "" {
 		fmt.Fprintf(&b, " %s", e.Type)
 	}
@@ -70,8 +74,9 @@
 }
 
 // Unwrap returns the classified sentinel and the pre-0012 status sentinel.
+// A stream failure (Status 0) has no status sentinel.
 func (e *APIError) Unwrap() []error {
-	if legacy := statusSentinel(e.Status); !errors.Is(e.sentinel, legacy) {
+	if legacy := statusSentinel(e.Status); e.Status != 0 && !errors.Is(e.sentinel, legacy) {
 		return []error{e.sentinel, legacy}
 	}
 	return []error{e.sentinel}
@@ -122,6 +127,32 @@
 	}
 	if errors.Is(e.sentinel, ErrRateLimited) && !e.Terminal {
 		return &RateLimitError{RetryAfter: e.RetryAfter, Status: e.Status, Provider: provider, Message: e.Message}
+	}
+	return e
+}
+
+// streamFailure classifies a response.failed event, which arrives inside a 200
+// stream, as Codex's parse_failed_response does
+// (codex-api/src/sse/responses_error.rs): quota and entitlement codes are
+// terminal (§1.1), context_length_exceeded and invalid_prompt are invalid
+// requests, rate_limit_exceeded and slow_down are rate limits, and anything
+// else is retryable. The APIError's Status is 0: there is no HTTP status.
+func streamFailure(provider, code, errType, message string) error {
+	env := apiErrorEnvelope{msg: message}
+	for _, t := range []string{code, errType} {
+		if t != "" && !env.hasType(t) {
+			env.types = append(env.types, t)
+		}
+	}
+	e := &APIError{Provider: provider, Type: env.errType(), Message: boundMessage(logging.RedactString(message))}
+	switch {
+	case env.hasType("rate_limit_exceeded") || env.hasType("slow_down"):
+		return &RateLimitError{Provider: provider, Message: e.Message}
+	case env.hasType("context_length_exceeded") || env.hasType("invalid_prompt"):
+		e.Terminal, e.sentinel = true, ErrInvalidRequest
+	default:
+		// 500 stands in for "no status": the table's codes win, else retryable.
+		e.Terminal, e.sentinel = classifyAPIError(serviceOf(provider), http.StatusInternalServerError, env, nil)
 	}
 	return e
 }
diff --git a/llmprovider/http_helpers.go b/llmprovider/http_helpers.go
--- a/llmprovider/http_helpers.go
+++ b/llmprovider/http_helpers.go
@@ -1,7 +1,9 @@
 package llmprovider
 
 import (
+	"bufio"
 	"encoding/json"
+	"fmt"
 	"io"
 	"log/slog"
 	"net/http"
@@ -27,62 +29,160 @@
 		IncompleteDetails struct {
 			Reason string `json:"reason"`
 		} `json:"incomplete_details"`
-		Output []struct {
-			Type    string `json:"type"`
-			Content []struct {
-				Type string `json:"type"`
-				Text string `json:"text"`
-			} `json:"content"`
-			Summary []struct {
-				Type string `json:"type"`
-				Text string `json:"text"`
-			} `json:"summary"`
-			CallID    string `json:"call_id"`
-			Name      string `json:"name"`
-			Arguments string `json:"arguments"`
-		} `json:"output"`
+		Output []responsesOutputItem `json:"output"`
 	}
 	if err := json.NewDecoder(body).Decode(&raw); err != nil {
 		return nil, err
 	}
 	// A truncated answer is an error, never an empty success (MADR 0012 §1.5).
 	if raw.Status == "incomplete" {
-		reason := raw.IncompleteDetails.Reason
-		if reason == "" {
-			reason = "unspecified"
-		}
-		return nil, &IncompleteError{Reason: reason}
+		return nil, incompleteResponse(raw.IncompleteDetails.Reason)
 	}
 
 	result := &Response{ID: raw.ID}
 	for _, out := range raw.Output {
-		switch out.Type {
-		case itemTypeMessage:
-			var sb strings.Builder
-			for _, c := range out.Content {
-				if c.Type == "output_text" || c.Type == jsonKeyText {
-					sb.WriteString(c.Text)
-				}
+		result.appendOutput(out)
+	}
+	return result, nil
+}
+
+// incompleteResponse is the §1.5 error for an incomplete Responses answer.
+func incompleteResponse(reason string) error {
+	if reason == "" {
+		reason = "unspecified"
+	}
+	return &IncompleteError{Reason: reason}
+}
+
+// responsesOutputItem is one Responses API output entry.
+type responsesOutputItem struct {
+	Type    string `json:"type"`
+	Content []struct {
+		Type string `json:"type"`
+		Text string `json:"text"`
+	} `json:"content"`
+	Summary []struct {
+		Type string `json:"type"`
+		Text string `json:"text"`
+	} `json:"summary"`
+	CallID    string `json:"call_id"`
+	Name      string `json:"name"`
+	Arguments string `json:"arguments"`
+}
+
+// appendOutput adds the item one Responses output entry carries, if any.
+func (r *Response) appendOutput(out responsesOutputItem) {
+	switch out.Type {
+	case itemTypeMessage:
+		var sb strings.Builder
+		for _, c := range out.Content {
+			if c.Type == "output_text" || c.Type == jsonKeyText {
+				sb.WriteString(c.Text)
 			}
-			if text := sb.String(); text != "" {
-				result.Output = append(result.Output, MessageItem{Role: jsonRoleAssistant, Text: text})
+		}
+		if text := sb.String(); text != "" {
+			r.Output = append(r.Output, MessageItem{Role: jsonRoleAssistant, Text: text})
+		}
+	case itemTypeFunctionCall:
+		r.Output = append(r.Output, FunctionCallItem{
+			CallID:    out.CallID,
+			Name:      out.Name,
+			Arguments: out.Arguments,
+		})
+	case itemTypeReasoning:
+		var sb strings.Builder
+		for _, s := range out.Summary {
+			if s.Type == "summary_text" || s.Type == jsonKeyText {
+				sb.WriteString(s.Text)
 			}
-		case itemTypeFunctionCall:
-			result.Output = append(result.Output, FunctionCallItem{
-				CallID:    out.CallID,
-				Name:      out.Name,
-				Arguments: out.Arguments,
-			})
-		case itemTypeReasoning:
-			var sb strings.Builder
-			for _, s := range out.Summary {
-				if s.Type == "summary_text" || s.Type == jsonKeyText {
-					sb.WriteString(s.Text)
-				}
+		}
+		r.Output = append(r.Output, ReasoningItem{Text: sb.String()})
+	}
+}
+
+// responsesStreamLimit bounds one Responses event stream.
+const responsesStreamLimit = 16 << 20
+
+// responsesStreamEvent is the part of one Responses stream event the reader
+// uses. The type is read from the payload, not the SSE "event:" line.
+type responsesStreamEvent struct {
+	Type     string              `json:"type"`
+	Item     responsesOutputItem `json:"item"`
+	Response struct {
+		ID    string `json:"id"`
+		Error *struct {
+			Type    string `json:"type"`
+			Code    string `json:"code"`
+			Message string `json:"message"`
+		} `json:"error"`
+		IncompleteDetails struct {
+			Reason string `json:"reason"`
+		} `json:"incomplete_details"`
+	} `json:"response"`
+}
+
+// readResponsesStream reads a Responses API event stream into a Response, as
+// Codex does (codex-api/src/sse/responses.rs:343-450): each
+// response.output_item.done adds its item, and response.created and
+// response.completed carry the id. response.failed maps onto §1.1,
+// response.incomplete onto §1.5, and a stream that ends before
+// response.completed is retryable (MADR 0012 §4.1). It does not rely on
+// Content-Type, which the ChatGPT backend does not send.
+func readResponsesStream(provider string, body io.Reader) (*Response, error) {
+	scanner := bufio.NewScanner(io.LimitReader(body, responsesStreamLimit))
+	scanner.Buffer(make([]byte, 0, 64<<10), responsesStreamLimit)
+	result := &Response{}
+	var data strings.Builder
+	dispatch := func() (done bool, err error) {
+		payload := data.String()
+		data.Reset()
+		if payload == "" || payload == "[DONE]" {
+			return false, nil
+		}
+		var event responsesStreamEvent
+		if err := json.Unmarshal([]byte(payload), &event); err != nil {
+			return false, fmt.Errorf("%s: decode stream event: %w", provider, err)
+		}
+		switch event.Type {
+		case "response.created":
+			result.ID = event.Response.ID
+		case "response.output_item.done":
+			result.appendOutput(event.Item)
+		case "response.failed":
+			e := event.Response.Error
+			if e == nil {
+				return false, streamFailure(provider, "", "", "response.failed event received")
 			}
-			result.Output = append(result.Output, ReasoningItem{Text: sb.String()})
+			return false, streamFailure(provider, e.Code, e.Type, e.Message)
+		case "response.incomplete":
+			return false, incompleteResponse(event.Response.IncompleteDetails.Reason)
+		case "response.completed":
+			if event.Response.ID != "" {
+				result.ID = event.Response.ID
+			}
+			return true, nil
+		}
+		return false, nil
+	}
+	for scanner.Scan() {
+		line := scanner.Text()
+		switch {
+		case line == "":
+			if done, err := dispatch(); done || err != nil {
+				return result, err
+			}
+		case strings.HasPrefix(line, "data:"):
+			if data.Len() > 0 {
+				data.WriteByte('\n')
+			}
+			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
 		}
 	}
-
-	return result, nil
+	if err := scanner.Err(); err != nil {
+		return nil, fmt.Errorf("%w: %s: read stream: %w", ErrProviderUnavailable, provider, err)
+	}
+	if done, err := dispatch(); done || err != nil {
+		return result, err
+	}
+	return nil, fmt.Errorf("%w: %s: stream ended before response.completed", ErrProviderUnavailable, provider)
 }
diff --git a/llmprovider/openai.go b/llmprovider/openai.go
--- a/llmprovider/openai.go
+++ b/llmprovider/openai.go
@@ -103,8 +103,13 @@
 	return p.doGenerateItems(ctx, input, &tool, true, "")
 }
 
-// Continue sends items to the OpenAI Responses API, chaining from a previous response.
+// Continue sends items to the OpenAI Responses API, chaining from a previous
+// response. A ChatGPT session stores no responses (store:false), so it has
+// nothing to chain from and returns ErrInvalidRequest (MADR 0012 §4.2).
 func (p *OpenAIProvider) Continue(ctx context.Context, previousResponseID string, input ...Item) (*Response, error) {
+	if p.chatGPT {
+		return nil, fmt.Errorf("%w: openai: a ChatGPT session cannot continue a response; replay the items", ErrInvalidRequest)
+	}
 	return p.doGenerateItems(ctx, input, nil, false, previousResponseID)
 }
 
@@ -119,9 +124,19 @@
 
 func (p *OpenAIProvider) doGenerateItemsOnce(ctx context.Context, input []Item, tool *Tool, thinking bool, prevResponseID string) (*Response, error) {
 	body := map[string]any{
-		jsonKeyModel:        p.model,
-		jsonKeyInput:        itemsToInput(input),
-		"max_output_tokens": p.maxTokens,
+		jsonKeyModel: p.model,
+		jsonKeyInput: itemsToInput(input),
+	}
+	if p.chatGPT {
+		// The ChatGPT backend takes only Codex's shape: it rejects a
+		// non-streaming request, a missing store:false and max_output_tokens
+		// (gate G-C, 2026-09-27; codex core/src/client.rs:1007-1008).
+		body["stream"] = true
+		body["store"] = false
+		body["include"] = []string{"reasoning.encrypted_content"}
+		body["prompt_cache_key"] = p.identity.session
+	} else {
+		body["max_output_tokens"] = p.maxTokens
 	}
 
 	if tool != nil {
@@ -169,6 +184,8 @@
 	}
 	req.Header.Set(oauthAuthorizationHeader, "Bearer "+token.Value)
 	if p.chatGPT {
+		req.Header.Set("Accept", "text/event-stream")
+		req.Header.Set(openAISessionHeader, p.identity.session)
 		req.Header.Set(openAIOriginatorHeader, openAIOriginatorValue)
 		if accountID := openAIAccountID(p.src); accountID != "" {
 			req.Header.Set(openAIAccountHeader, accountID)
@@ -188,6 +205,9 @@
 
 	if err := classifyHTTPError(ProviderOpenAI, resp); err != nil {
 		return nil, err
+	}
+	if p.chatGPT {
+		return readResponsesStream(ProviderOpenAI, resp.Body)
 	}
 
 	return decodeResponsesAPIOutput(limitedBody)
diff --git a/llmprovider/openai_chatgpt.go b/llmprovider/openai_chatgpt.go
--- a/llmprovider/openai_chatgpt.go
+++ b/llmprovider/openai_chatgpt.go
@@ -13,6 +13,9 @@
 	openAIResidencyHeader  = "x-openai-internal-codex-residency"
 	openAIOriginatorHeader = "originator"
 	openAIOriginatorValue  = "mcplib"
+	// openAISessionHeader carries the conversation id, as Codex sends it
+	// (codex-api/src/requests/headers.rs:8).
+	openAISessionHeader = "session-id"
 )
 
 // NewOpenAIWithSource creates an OpenAI provider from a dynamic token source.
diff --git a/llmprovider/openai_chatgpt_test.go b/llmprovider/openai_chatgpt_test.go
--- a/llmprovider/openai_chatgpt_test.go
+++ b/llmprovider/openai_chatgpt_test.go
@@ -118,10 +118,10 @@
 	}
 }
 
-// TestOpenAI_ChatGPTSendsMaxOutputTokens pins today's ChatGPT request body:
-// max_output_tokens is still sent (MADR 0009 open question 1 decides later
-// whether the Codex backend wants it).
-func TestOpenAI_ChatGPTSendsMaxOutputTokens(t *testing.T) {
+// TestOpenAI_ChatGPTOmitsMaxOutputTokens answers MADR 0009 open question 1:
+// the ChatGPT backend rejects max_output_tokens (400 "Unsupported parameter",
+// gate G-C 2026-09-27), so a ChatGPT session never sends it.
+func TestOpenAI_ChatGPTOmitsMaxOutputTokens(t *testing.T) {
 	t.Parallel()
 
 	var body map[string]any
@@ -144,8 +144,8 @@
 	if _, err := provider.Generate(context.Background(), "hello"); err != nil {
 		t.Fatalf("Generate() error = %v", err)
 	}
-	if got, ok := body["max_output_tokens"].(float64); !ok || int(got) != 321 {
-		t.Fatalf("max_output_tokens = %v, want 321", body["max_output_tokens"])
+	if got, ok := body["max_output_tokens"]; ok {
+		t.Fatalf("max_output_tokens = %v, want absent", got)
 	}
 }
 
@@ -299,6 +299,9 @@
 }
 
 func openAITestHTTPResponse(request *http.Request, status int, body string) *http.Response {
+	if status == http.StatusOK && request != nil && request.Header.Get("Accept") == "text/event-stream" {
+		body = openAITestStream(body)
+	}
 	return &http.Response{
 		StatusCode: status,
 		Status:     http.StatusText(status),
@@ -317,3 +320,23 @@
 	}
 	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
 }
+
+// openAITestStream reframes a Responses JSON body as the event stream a
+// ChatGPT session reads (MADR 0012 §4.1): one output_item.done per output
+// item, then response.completed.
+func openAITestStream(body string) string {
+	var parsed struct {
+		ID     string            `json:"id"`
+		Output []json.RawMessage `json:"output"`
+	}
+	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
+		return body
+	}
+	var b strings.Builder
+	for _, item := range parsed.Output {
+		b.WriteString(`data: {"type":"response.output_item.done","item":` + string(item) + "}\n\n")
+	}
+	id, _ := json.Marshal(parsed.ID)
+	b.WriteString(`data: {"type":"response.completed","response":{"id":` + string(id) + "}}\n\n")
+	return b.String()
+}
```

### B.C2 Phase C2 — The listing sends `mcplib`'s own release (§4.3, G-C item 7)

**Tests** (`cg2-tests.diff`, 135 lines):

```diff
diff --git a/llmprovider/chatgpt_listing_version_test.go b/llmprovider/chatgpt_listing_version_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/chatgpt_listing_version_test.go
@@ -0,0 +1,66 @@
+package llmprovider
+
+import (
+	"context"
+	"net/http"
+	"net/http/httptest"
+	"testing"
+	"time"
+)
+
+// withMcplibVersion makes buildVersions report v as mcplib's version for one
+// (non-parallel) test.
+func withMcplibVersion(t *testing.T, v string) {
+	t.Helper()
+	saved := buildVersions
+	buildVersions = func() (string, string) { return v, v }
+	t.Cleanup(func() { buildVersions = saved })
+}
+
+// chatGPTListingVersion returns the client_version one ChatGPT listing sent.
+func chatGPTListingVersion(t *testing.T) string {
+	t.Helper()
+	var got string
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		got = r.URL.Query().Get("client_version")
+		_, _ = w.Write([]byte(`{"models":[{"slug":"gpt-6-astra","visibility":"list","priority":1}]}`))
+	}))
+	t.Cleanup(srv.Close)
+	session := &OAuthSession{Issuer: DefaultOpenAIIssuer, Access: "a", Expiry: time.Now().Add(time.Hour)}
+	if _, err := ListAvailableModelsWithSource(context.Background(), ProviderOpenAI, session,
+		WithHTTPClient(srv.Client()), WithBaseURL(srv.URL)); err != nil {
+		t.Fatalf("listing: %v", err)
+	}
+	return got
+}
+
+// TestChatGPTListing_SendsMcplibVersion: a release, a pseudo-version and an
+// +incompatible build send X.Y.Z (the backend answers 400 to "v1.5.0",
+// "1.5", "(devel)" and anything over 32 characters; measured 2026-09-27).
+func TestChatGPTListing_SendsMcplibVersion(t *testing.T) {
+	for _, tc := range []struct{ build, want string }{
+		{"v1.5.0", "1.5.0"},
+		{"v1.5.1-0.20260927120000-7ea0ad4abcde", "1.5.1"},
+		{"v2.0.0+incompatible", "2.0.0"},
+	} {
+		t.Run(tc.build, func(t *testing.T) {
+			withMcplibVersion(t, tc.build)
+			if got := chatGPTListingVersion(t); got != tc.want {
+				t.Errorf("client_version = %q, want %q", got, tc.want)
+			}
+		})
+	}
+}
+
+// TestChatGPTListing_DevelBuildSendsFallback: a build with no release version
+// sends 0.0.0, which the backend accepts.
+func TestChatGPTListing_DevelBuildSendsFallback(t *testing.T) {
+	for _, build := range []string{"(devel)", "", "1.5"} {
+		t.Run(build, func(t *testing.T) {
+			withMcplibVersion(t, build)
+			if got := chatGPTListingVersion(t); got != "0.0.0" {
+				t.Errorf("client_version = %q, want 0.0.0", got)
+			}
+		})
+	}
+}
diff --git a/llmprovider/live_chatgpt_listing_test.go b/llmprovider/live_chatgpt_listing_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_chatgpt_listing_test.go
@@ -0,0 +1,59 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"net/http"
+	"slices"
+	"strings"
+	"testing"
+)
+
+// TestLive_ChatGPTListingVersion is gate G-C item 7 through mcplib: a v1.5.0
+// build lists with client_version 1.5.0, and that listing holds every model
+// the 0.0.0 fallback lists (on 2026-09-27 it added gpt-6-sol and gpt-6-luna,
+// whose minimal_client_version is 0.155.0), and each added model generates.
+func TestLive_ChatGPTListingVersion(t *testing.T) {
+	session := liveChatGPTSession(t)
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	var sent []string
+	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
+		sent = append(sent, r.URL.Query().Get("client_version"))
+		return http.DefaultTransport.RoundTrip(r)
+	})}
+	list := func(build string) []string {
+		withMcplibVersion(t, build)
+		models, err := ListAvailableModelsWithSource(ctx, ProviderOpenAI, session, WithHTTPClient(client))
+		skipIfTransient(t, err)
+		if err != nil {
+			t.Fatalf("listing as %s: %v", build, err)
+		}
+		return models
+	}
+	fallback, release := list("(devel)"), list("v1.5.0")
+	if !slices.Equal(sent, []string{"0.0.0", "1.5.0"}) {
+		t.Fatalf("client_version sent = %v, want [0.0.0 1.5.0]", sent)
+	}
+	for _, id := range fallback {
+		if !slices.Contains(release, id) {
+			t.Errorf("%s listed for 0.0.0 but not for 1.5.0 (%v)", id, release)
+		}
+	}
+	t.Logf("0.0.0: %v; 1.5.0: %v", fallback, release)
+	// The models the release listing adds must be served.
+	for _, id := range release {
+		if slices.Contains(fallback, id) {
+			continue
+		}
+		p, err := NewOpenAIWithSource(session, id)
+		if err != nil {
+			t.Fatal(err)
+		}
+		out, err := p.Generate(ctx, "Reply with only the word ALPHA")
+		skipIfTransient(t, err)
+		if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
+			t.Errorf("%s: Generate = %q, %v", id, out, err)
+		}
+	}
+}
```

**Fix** (`cg2-fix.diff`, 51 lines):

```diff
diff --git a/llmprovider/discovery.go b/llmprovider/discovery.go
--- a/llmprovider/discovery.go
+++ b/llmprovider/discovery.go
@@ -8,16 +8,34 @@
 	"math"
 	"net/http"
 	"net/url"
+	"regexp"
 	"slices"
 	"strconv"
 	"strings"
 	"time"
 )
 
-// chatgptModelsClientVersion is required by GET .../codex/models. The Codex
-// backend rejects requests without this query parameter. 0.0.0 is accepted
-// and does not hide currently listed subscription models.
+// chatgptModelsClientVersion is the client_version GET .../codex/models is
+// sent when mcplib's build has no release version. The backend requires the
+// parameter; 0.0.0 is accepted but hides models whose minimal_client_version
+// is higher (gpt-6-sol and gpt-6-luna on 2026-09-27).
 const chatgptModelsClientVersion = "0.0.0"
+
+// chatgptVersionRE reads X.Y.Z from a module version: the only form the
+// backend accepts ("v1.5.0", "1.5" and "(devel)" answer 400, as does anything
+// over 32 characters; measured 2026-09-27). A pseudo-version names the
+// release it precedes.
+var chatgptVersionRE = regexp.MustCompile(`^v?(\d{1,9}\.\d{1,9}\.\d{1,9})(?:[-+].*)?$`)
+
+// chatgptClientVersion is the client_version for mcplib at version v: its
+// own release, never a Codex version string (MADR 0012 §4.3), else
+// chatgptModelsClientVersion.
+func chatgptClientVersion(v string) string {
+	if m := chatgptVersionRE.FindStringSubmatch(v); m != nil {
+		return m[1]
+	}
+	return chatgptModelsClientVersion
+}
 
 // Listing pagination (MADR 0009 §2): Gemini and Anthropic page their model
 // lists, so each fetch requests the maximum page size and follows at most
@@ -168,7 +186,8 @@
 		return ModelCatalog{}, fmt.Errorf("model listing: parse chatgpt models URL: %w", err)
 	}
 	query := endpoint.Query()
-	query.Set("client_version", chatgptModelsClientVersion)
+	mcplib, _ := buildVersions()
+	query.Set("client_version", chatgptClientVersion(mcplib))
 	endpoint.RawQuery = query.Encode()
 
 	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
```

### B.C3 Phase C3 — The error envelope, `resets_at` and FedRAMP (§4.4)

**Tests** (`cg3-tests.diff`, 145 lines):

```diff
diff --git a/llmprovider/chatgpt_errors_test.go b/llmprovider/chatgpt_errors_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/chatgpt_errors_test.go
@@ -0,0 +1,44 @@
+package llmprovider
+
+import (
+	"errors"
+	"io"
+	"net/http"
+	"strconv"
+	"strings"
+	"testing"
+	"time"
+)
+
+func classifyBody(provider string, status int, body string) error {
+	return classifyHTTPError(provider, &http.Response{StatusCode: status, Header: http.Header{},
+		Body: io.NopCloser(strings.NewReader(body))})
+}
+
+// TestClassify_DetailEnvelope: the ChatGPT backend answers a bad request with
+// {"detail": ...} (gate G-C, 2026-09-27); that text is the message.
+func TestClassify_DetailEnvelope(t *testing.T) {
+	const detail = "The 'gpt-4.1-mini' model is not supported when using Codex with a ChatGPT account."
+	err := classifyBody(ProviderOpenAI, http.StatusBadRequest, `{"detail":"`+detail+`"}`)
+	var apiErr *APIError
+	if !errors.As(err, &apiErr) || apiErr.Message != detail || !errors.Is(err, ErrInvalidRequest) {
+		t.Fatalf("err = %v (message %q), want ErrInvalidRequest carrying the detail", err, apiErr.Message)
+	}
+}
+
+// TestClassify_UsageLimitResetsAt: Codex reads resets_at (Unix seconds) from
+// a usage_limit_reached body (codex-api/src/api_bridge.rs:186-206); with no
+// Retry-After header it becomes RetryAfter.
+func TestClassify_UsageLimitResetsAt(t *testing.T) {
+	reset := time.Now().Add(time.Hour).Unix()
+	err := classifyBody(ProviderOpenAI, http.StatusTooManyRequests,
+		`{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","plan_type":"plus","resets_at":`+
+			strconv.FormatInt(reset, 10)+`}}`)
+	var apiErr *APIError
+	if !errors.As(err, &apiErr) || !errors.Is(err, ErrQuotaExhausted) || !apiErr.Terminal {
+		t.Fatalf("err = %v, want a terminal ErrQuotaExhausted", err)
+	}
+	if apiErr.RetryAfter < 59*time.Minute || apiErr.RetryAfter > time.Hour {
+		t.Errorf("RetryAfter = %v, want about an hour", apiErr.RetryAfter)
+	}
+}
diff --git a/llmprovider/chatgpt_fedramp_test.go b/llmprovider/chatgpt_fedramp_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/chatgpt_fedramp_test.go
@@ -0,0 +1,60 @@
+package llmprovider
+
+import (
+	"context"
+	"testing"
+	"time"
+)
+
+// chatGPTLoginSession is the session a ChatGPT login yields for an id token
+// with these auth claims.
+func chatGPTLoginSession(t *testing.T, auth map[string]any) *OAuthSession {
+	t.Helper()
+	session, err := oauthSessionFromResponse(
+		oauthFlowConfig{provider: ProviderOpenAI, issuer: DefaultOpenAIIssuer, clientID: DefaultOpenAIClientID, now: time.Now},
+		DefaultOpenAIIssuer+"/oauth/token",
+		oauthTokenResponse{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 3600,
+			IDToken: openAITestJWT(t, map[string]any{"https://api.openai.com/auth": auth})})
+	if err != nil {
+		t.Fatal(err)
+	}
+	return session
+}
+
+// fedrampHeader returns X-OpenAI-Fedramp as one ChatGPT call sent it.
+func fedrampHeader(t *testing.T, session *OAuthSession) (string, bool) {
+	t.Helper()
+	client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
+	p, err := NewOpenAIWithSource(session, "gpt-6-astra", WithHTTPClient(client))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, err := p.Generate(context.Background(), "hi"); err != nil {
+		t.Fatalf("Generate: %v", err)
+	}
+	c.mu.Lock()
+	defer c.mu.Unlock()
+	v, ok := c.header["X-Openai-Fedramp"]
+	if !ok {
+		return "", false
+	}
+	return v[0], true
+}
+
+// TestOpenAIChatGPT_FedRAMPHeader: an id token with chatgpt_account_is_fedramp
+// sends X-OpenAI-Fedramp: true, as Codex's bearer auth does
+// (model-provider/src/bearer_auth_provider.rs:43-45; login/src/token_data.rs).
+func TestOpenAIChatGPT_FedRAMPHeader(t *testing.T) {
+	session := chatGPTLoginSession(t, map[string]any{"chatgpt_account_id": "acct", "chatgpt_account_is_fedramp": true})
+	if v, ok := fedrampHeader(t, session); !ok || v != "true" {
+		t.Fatalf("X-OpenAI-Fedramp = %q (present %t), want true", v, ok)
+	}
+}
+
+// TestOpenAIChatGPT_NoFedRAMPHeader: without the claim, no header.
+func TestOpenAIChatGPT_NoFedRAMPHeader(t *testing.T) {
+	session := chatGPTLoginSession(t, map[string]any{"chatgpt_account_id": "acct"})
+	if v, ok := fedrampHeader(t, session); ok {
+		t.Fatalf("X-OpenAI-Fedramp = %q, want absent", v)
+	}
+}
diff --git a/llmprovider/live_chatgpt_errors_test.go b/llmprovider/live_chatgpt_errors_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_chatgpt_errors_test.go
@@ -0,0 +1,26 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"errors"
+	"strings"
+	"testing"
+)
+
+// TestLive_ChatGPTErrorDetail: the backend refuses a model outside the Codex
+// catalog with 400 {"detail": ...}; the error carries that text.
+func TestLive_ChatGPTErrorDetail(t *testing.T) {
+	session := liveChatGPTSession(t)
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	p, err := NewOpenAIWithSource(session, "gpt-4.1-mini")
+	if err != nil {
+		t.Fatal(err)
+	}
+	_, err = p.Generate(ctx, "hi")
+	var apiErr *APIError
+	if !errors.Is(err, ErrInvalidRequest) || !errors.As(err, &apiErr) || !strings.Contains(apiErr.Message, "not supported") {
+		t.Fatalf("err = %v, want ErrInvalidRequest carrying the backend's detail", err)
+	}
+}
```

**Fix** (`cg3-fix.diff`, 353 lines):

```diff
diff --git a/llmprovider/api_error.go b/llmprovider/api_error.go
--- a/llmprovider/api_error.go
+++ b/llmprovider/api_error.go
@@ -121,6 +121,12 @@
 		RetryAfter: retryAfterFrom(resp.Header),
 	}
 	e.Terminal, e.sentinel = classifyAPIError(serviceOf(provider), resp.StatusCode, envelope, body)
+	// A usage limit says when it resets, as Codex reads it (MADR 0012 §4.4).
+	if e.RetryAfter == 0 && envelope.resetsAt > 0 {
+		if d := time.Until(time.Unix(envelope.resetsAt, 0)); d > 0 {
+			e.RetryAfter = d
+		}
+	}
 	// x-should-retry: false is the service saying no retry can succeed (§1.2).
 	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("X-Should-Retry")), "false") {
 		e.Terminal = true
@@ -214,8 +220,9 @@
 // OpenAI/Codex {error:{type,code,message}},
 // xAI nested or flat {code,error}, Gemini {error:{code,message,status}}.
 type apiErrorEnvelope struct {
-	types []string // candidate classifications, most specific first
-	msg   string
+	types    []string // candidate classifications, most specific first
+	msg      string
+	resetsAt int64 // usage_limit_reached's reset time, Unix seconds, or 0
 }
 
 func (e apiErrorEnvelope) errType() string {
@@ -236,6 +243,7 @@
 		ErrorType string          `json:"error_type"`
 		Message   string          `json:"message"`
 		Error     json.RawMessage `json:"error"`
+		Detail    json.RawMessage `json:"detail"`
 	}
 	if json.Unmarshal(body, &top) != nil {
 		return apiErrorEnvelope{msg: strings.TrimSpace(string(body))}
@@ -249,22 +257,28 @@
 		}
 	}
 	var inner struct {
-		Type    string          `json:"type"`
-		Code    json.RawMessage `json:"code"`
-		Status  string          `json:"status"`
-		Message string          `json:"message"`
+		Type     string          `json:"type"`
+		Code     json.RawMessage `json:"code"`
+		Status   string          `json:"status"`
+		Message  string          `json:"message"`
+		ResetsAt int64           `json:"resets_at"`
 	}
 	var text string
 	switch {
 	case json.Unmarshal(top.Error, &inner) == nil:
 		add(jsonString(inner.Code), inner.Type, inner.Status)
 		env.msg = inner.Message
+		env.resetsAt = inner.ResetsAt
 	case json.Unmarshal(top.Error, &text) == nil:
 		env.msg = text
 	}
 	add(top.ErrorType, jsonString(top.Code), top.Type)
 	if env.msg == "" {
 		env.msg = top.Message
+	}
+	if env.msg == "" {
+		// The ChatGPT backend's {"detail": ...} (gate G-C, 2026-09-27).
+		env.msg = jsonString(top.Detail)
 	}
 	return env
 }
diff --git a/llmprovider/chatgpt_fedramp_store_test.go b/llmprovider/chatgpt_fedramp_store_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/chatgpt_fedramp_store_test.go
@@ -0,0 +1,26 @@
+package llmprovider
+
+import (
+	"context"
+	"path/filepath"
+	"testing"
+	"time"
+)
+
+// TestFileTokenStore_KeepsFedRAMP: the flag survives a save and a load, so a
+// reloaded session still sends the header.
+func TestFileTokenStore_KeepsFedRAMP(t *testing.T) {
+	store, err := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	in := &OAuthSession{Provider: ProviderOpenAI, Access: "a", Refresh: "r", Expiry: time.Now().Add(time.Hour),
+		Issuer: DefaultOpenAIIssuer, ClientID: DefaultOpenAIClientID, FedRAMP: true}
+	if err := store.Save(context.Background(), ProviderOpenAI, in); err != nil {
+		t.Fatal(err)
+	}
+	out, err := store.Load(context.Background(), ProviderOpenAI)
+	if err != nil || !out.FedRAMP {
+		t.Fatalf("Load = %+v, %v; want FedRAMP", out, err)
+	}
+}
diff --git a/llmprovider/discovery.go b/llmprovider/discovery.go
--- a/llmprovider/discovery.go
+++ b/llmprovider/discovery.go
@@ -199,6 +199,9 @@
 	req.Header.Set(openAIOriginatorHeader, openAIOriginatorValue)
 	if accountID := openAIAccountID(src); accountID != "" {
 		req.Header.Set(openAIAccountHeader, accountID)
+	}
+	if openAIFedRAMP(src) {
+		req.Header.Set(openAIFedRAMPHeader, "true")
 	}
 
 	resp, err := cfg.HTTPClient.Do(req)
diff --git a/llmprovider/oauth_loopback.go b/llmprovider/oauth_loopback.go
--- a/llmprovider/oauth_loopback.go
+++ b/llmprovider/oauth_loopback.go
@@ -532,6 +532,7 @@
 		Issuer:     config.issuer,
 		ClientID:   config.clientID,
 		AccountID:  chatGPTAccountID(payload.IDToken),
+		FedRAMP:    chatGPTFedRAMP(payload.IDToken),
 		TokenURL:   tokenURL,
 		HTTPClient: config.httpClient,
 	}, nil
@@ -559,6 +560,26 @@
 		return claims.AccountID
 	}
 	return claims.Auth.AccountID
+}
+
+// chatGPTFedRAMP reports the id token's chatgpt_account_is_fedramp claim, as
+// Codex reads it (login/src/token_data.rs AuthClaims). A missing or
+// unreadable claim is false.
+func chatGPTFedRAMP(idToken string) bool {
+	parts := strings.Split(idToken, ".")
+	if len(parts) < 2 {
+		return false
+	}
+	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
+	if err != nil {
+		return false
+	}
+	var claims struct {
+		Auth struct {
+			FedRAMP bool `json:"chatgpt_account_is_fedramp"`
+		} `json:"https://api.openai.com/auth"`
+	}
+	return json.Unmarshal(payload, &claims) == nil && claims.Auth.FedRAMP
 }
 
 func decodeOAuthResponse(resp *http.Response, target any) error {
diff --git a/llmprovider/oauth_session.go b/llmprovider/oauth_session.go
--- a/llmprovider/oauth_session.go
+++ b/llmprovider/oauth_session.go
@@ -144,6 +144,7 @@
 	issuer     string
 	clientID   string
 	accountID  string
+	fedramp    bool
 	tokenURL   string
 	store      TokenStore
 	httpClient *http.Client
@@ -156,6 +157,7 @@
 		issuer:     s.Issuer,
 		clientID:   s.ClientID,
 		accountID:  s.AccountID,
+		fedramp:    s.FedRAMP,
 		tokenURL:   s.TokenURL,
 		store:      s.Store,
 		httpClient: s.HTTPClient,
@@ -170,6 +172,7 @@
 	s.Issuer = next.Issuer
 	s.ClientID = next.ClientID
 	s.AccountID = next.AccountID
+	s.FedRAMP = next.FedRAMP
 	s.TokenURL = next.TokenURL
 	s.Store = next.Store
 	s.HTTPClient = next.HTTPClient
@@ -239,6 +242,7 @@
 		Issuer:     state.issuer,
 		ClientID:   state.clientID,
 		AccountID:  state.accountID,
+		FedRAMP:    state.fedramp,
 		TokenURL:   state.tokenURL,
 		Store:      state.store,
 		HTTPClient: state.httpClient,
diff --git a/llmprovider/openai.go b/llmprovider/openai.go
--- a/llmprovider/openai.go
+++ b/llmprovider/openai.go
@@ -189,6 +189,9 @@
 		req.Header.Set(openAIOriginatorHeader, openAIOriginatorValue)
 		if accountID := openAIAccountID(p.src); accountID != "" {
 			req.Header.Set(openAIAccountHeader, accountID)
+		}
+		if openAIFedRAMP(p.src) {
+			req.Header.Set(openAIFedRAMPHeader, "true")
 		}
 		if residency := openAIResidency(token.Value); residency != "" {
 			req.Header.Set(openAIResidencyHeader, residency)
diff --git a/llmprovider/openai_chatgpt.go b/llmprovider/openai_chatgpt.go
--- a/llmprovider/openai_chatgpt.go
+++ b/llmprovider/openai_chatgpt.go
@@ -16,6 +16,9 @@
 	// openAISessionHeader carries the conversation id, as Codex sends it
 	// (codex-api/src/requests/headers.rs:8).
 	openAISessionHeader = "session-id"
+	// openAIFedRAMPHeader marks a FedRAMP account's requests, as Codex's
+	// bearer auth does (model-provider/src/bearer_auth_provider.rs:43-45).
+	openAIFedRAMPHeader = "X-OpenAI-Fedramp"
 )
 
 // NewOpenAIWithSource creates an OpenAI provider from a dynamic token source.
@@ -59,6 +62,17 @@
 	return session.AccountID
 }
 
+// openAIFedRAMP reports whether src is a FedRAMP ChatGPT session.
+func openAIFedRAMP(src TokenSource) bool {
+	session, ok := src.(*OAuthSession)
+	if !ok {
+		return false
+	}
+	session.mu.Lock()
+	defer session.mu.Unlock()
+	return session.FedRAMP
+}
+
 func expireOpenAISession(src TokenSource) bool {
 	session, ok := src.(*OAuthSession)
 	if !ok {
@@ -70,6 +84,9 @@
 	return true
 }
 
+// openAIResidency reads chatgpt_compute_residency from the access token for
+// the residency header. Its source is OpenCode's Codex plugin
+// (plugin/openai/codex.ts:83, :426), not Codex (MADR 0012 §4.4).
 func openAIResidency(accessToken string) string {
 	parts := strings.Split(accessToken, ".")
 	if len(parts) != 3 {
diff --git a/llmprovider/tokenstore.go b/llmprovider/tokenstore.go
--- a/llmprovider/tokenstore.go
+++ b/llmprovider/tokenstore.go
@@ -21,6 +21,7 @@
 	Issuer     string
 	ClientID   string
 	AccountID  string
+	FedRAMP    bool // id-token chatgpt_account_is_fedramp: sends X-OpenAI-Fedramp (MADR 0012 §4.4)
 	TokenURL   string
 	Store      TokenStore
 	HTTPClient *http.Client
@@ -51,6 +52,7 @@
 	Issuer    string    `json:"issuer"`
 	ClientID  string    `json:"client_id"`
 	AccountID string    `json:"account_id"`
+	FedRAMP   bool      `json:"fedramp,omitempty"`
 	TokenURL  string    `json:"token_url"`
 }
 
diff --git a/llmprovider/tokenstore_file.go b/llmprovider/tokenstore_file.go
--- a/llmprovider/tokenstore_file.go
+++ b/llmprovider/tokenstore_file.go
@@ -54,6 +54,7 @@
 		Issuer:    rec.Issuer,
 		ClientID:  rec.ClientID,
 		AccountID: rec.AccountID,
+		FedRAMP:   rec.FedRAMP,
 		TokenURL:  rec.TokenURL,
 	}
 	s.Store = fs
@@ -76,6 +77,7 @@
 		Issuer:    s.Issuer,
 		ClientID:  s.ClientID,
 		AccountID: s.AccountID,
+		FedRAMP:   s.FedRAMP,
 		TokenURL:  s.TokenURL,
 	}
 	data, err := json.MarshalIndent(rec, "", "  ")
diff --git a/wizard/auth.go b/wizard/auth.go
--- a/wizard/auth.go
+++ b/wizard/auth.go
@@ -162,6 +162,7 @@
 		Issuer:    o.Existing.Issuer,
 		ClientID:  o.Existing.ClientID,
 		AccountID: o.Existing.AccountID,
+		FedRAMP:   o.Existing.FedRAMP,
 		Store:     o.TokenStore,
 	}
 	if err := llmprovider.ValidateOAuthSession(session); err != nil {
diff --git a/wizard/configure.go b/wizard/configure.go
--- a/wizard/configure.go
+++ b/wizard/configure.go
@@ -30,6 +30,7 @@
 	Issuer       string
 	ClientID     string
 	AccountID    string
+	FedRAMP      bool
 	Model        string
 	BaseURL      string
 	Fallbacks    []string
@@ -139,6 +140,7 @@
 		res.Issuer = credential.session.Issuer
 		res.ClientID = credential.session.ClientID
 		res.AccountID = credential.session.AccountID
+		res.FedRAMP = credential.session.FedRAMP
 	}
 
 	cat := discoverModels(ctx, p, d, res, credential.source, o)
diff --git a/wizard/fedramp_test.go b/wizard/fedramp_test.go
new file mode 100644
--- /dev/null
+++ b/wizard/fedramp_test.go
@@ -0,0 +1,40 @@
+package wizard
+
+import (
+	"context"
+	"testing"
+	"time"
+
+	"github.com/maccavelli/mcplib/llmprovider"
+)
+
+// TestConfigureLLM_KeepsFedRAMP: a kept ChatGPT session keeps its FedRAMP
+// flag in the Result, which consumers persist and hand back as Existing.
+func TestConfigureLLM_KeepsFedRAMP(t *testing.T) {
+	f := &fakePrompter{
+		t:        t,
+		selects:  []int{providerIdx(t, llmprovider.ProviderOpenAI), 1},
+		confirms: []bool{true},
+	}
+	res, err := ConfigureLLM(context.Background(), f, Options{
+		Existing: Result{
+			Provider:     llmprovider.ProviderOpenAI,
+			Kind:         CredOAuth,
+			AccessToken:  "existing-access-abcd",
+			RefreshToken: "existing-refresh",
+			TokenExpiry:  time.Now().Add(time.Hour),
+			Issuer:       llmprovider.DefaultOpenAIIssuer,
+			ClientID:     llmprovider.DefaultOpenAIClientID,
+			AccountID:    "acct_test",
+			FedRAMP:      true,
+			Model:        "kept-chatgpt-model",
+		},
+		TokenStore: newMemoryTokenStore(),
+	})
+	if err != nil {
+		t.Fatalf("ConfigureLLM() error = %v", err)
+	}
+	if !res.FedRAMP {
+		t.Fatal("Result.FedRAMP = false, want the kept session's true")
+	}
+}
```

