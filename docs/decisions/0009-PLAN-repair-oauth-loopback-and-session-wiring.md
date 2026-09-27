---
status: in-progress
date: 2026-09-27
associated-madr: 0009-MADR-repair-oauth-loopback-and-session-wiring.md
decision-makers: mcplib maintainers
---

<!-- markdownlint-disable MD013 MD024 MD033 MD036 MD060 -->

# PLAN 0009 — Repair OAuth loopback, paste-code, and Windows session wiring

Implements [0009-MADR-repair-oauth-loopback-and-session-wiring.md](0009-MADR-repair-oauth-loopback-and-session-wiring.md)
decisions D1–D11, closing findings F1–F12.

If execution discovers a fact that contradicts the MADR, **stop and amend the
MADR** before continuing. Do not smuggle a different architecture into a phase.

## Goal

On this Windows host, and in CI:

1. OpenAI browser login advertises `http://localhost:{1455|1457}/auth/callback`
   and the process accepts the callback on whichever of `127.0.0.1` / `::1` the
   browser dials. `net.Dial("tcp", "localhost:"+port)` succeeds against that
   listener here, where `localhost` is IPv6-first.
2. Grok `/callback` answers CORS preflight from `https://accounts.x.ai` with
   private-network permission. Redirect URI stays `http://127.0.0.1:{ephemeral}/callback`.
3. Wizard browser OAuth always races loopback against `Prompter.Input` paste-code.
   A missed callback is a prompt, not a ten-minute hang.
4. `OpenURL` cannot own the login wait. CombinedOutput is not used for it.
5. Every callback outcome (state mismatch, IdP `error`, missing `code`, write
   failure, success) completes the waiter with a result or an error.
6. A saved session used for generation is refreshable, or an explicit ChatGPT
   access-only token, never the fixture `chatgpt-access`.
7. Token-endpoint and refresh failures include a truncated, redacted body.
8. ChatGPT generate and ChatGPT `/models` send `originator: mcplib`.
9. After ChatGPT OAuth, the model menu is the live Codex catalog. There is no
   `StaticOpenAIChatGPT`. A listing failure does not offer Platform `gpt-4.1-*`.
10. `prepare-commit-msg` tests that touch `UserConfigDir` cannot write
    `%APPDATA%\prepare-commit-msg\oauth\`. `go test ./...` there does not
    observe `chatgpt-access` in the live store.

Device-code still works. No new `go.mod` require. No OS keyring. No Claude or
Gemini OAuth.

## Scope

### In scope (the only files any phase may touch)

**mcplib** (phases P1–P7), against HEAD `233999b`:

* `llmprovider/oauth_loopback.go`
* `llmprovider/oauth_loopback_test.go`
* `llmprovider/oauth_session.go`
* `llmprovider/oauth_session_test.go`
* `llmprovider/openai.go`
* `llmprovider/openai_chatgpt.go`
* `llmprovider/openai_chatgpt_test.go`
* `llmprovider/discovery.go`
* `llmprovider/discovery_test.go`
* `llmprovider/models_catalog.go`
* `llmprovider/models_catalog_test.go`
* `wizard/auth.go`
* `wizard/auth_test.go`
* `wizard/configure.go`
* `wizard/configure_test.go`
* `wizard/import.go`
* `wizard/import_test.go`

**prepare-commit-msg** (phase P8), sibling `../prepare-commit-msg`, after the
mcplib tag gate:

* `main_oauth_test.go`
* `internal/config/config.go`
* `internal/config/config_test.go`
* `internal/ui/setup.go` (only if `ValidateOAuth` / `Discover` wiring must
  pass HTTPClient or the new validator; do not rewrite the wizard)
* `internal/ui/setup_test.go`
* `go.mod`
* `go.sum`

P0 may also touch `docs/decisions/0009-MADR-repair-oauth-loopback-and-session-wiring.md`
and this file, and nothing else.

### Out of scope

* Claude, Gemini, Vertex, Bedrock, Foundry, gateway, Hugging Face, Kilo, or
  Ollama OAuth.
* Removing static-catalog fallbacks from API-key OpenAI or from any
  non-ChatGPT `DiscoverModels`. The dirty working tree already did that; it is
  **not** this plan.
* Changing the Prompter interface (no `context` on `Input`).
* Changing advertised OpenAI redirect host away from `localhost`, or Grok
  away from `127.0.0.1` ephemeral.
* Binding OpenAI `:0`.
* Wrapping `codex` / `grok` binaries. OS keyring. `golang.org/x/oauth2`.
* MagicDev, MagicTools, other fleet consumers besides prepare-commit-msg.
* Deleting the live `%APPDATA%\prepare-commit-msg\oauth\openai.json` from
  code (operator recovery, see Deferred).
* Omitting `max_output_tokens` on ChatGPT generate (capturing test pins
  today's value; live 400 on that field is open question 1).
* `git push`, and tags, unless the same turn explicitly says to.

## Starting state (do this before P1, not as a commit)

Baseline is **mcplib HEAD `233999b`**, not the dirty tree.

Uncommitted edits in `llmprovider/` and `wizard/` are a mixed draft of D1, D4,
D5, D9, and an **over-broad** catalog change. They are not a phase.

Before P1:

```text
git stash push -- llmprovider wizard
```

Do **not** use `-u` (that would stash this PLAN / MADR). Do **not** restore
that stash as a commit. Do **not** `git add` those files until the matching
phase has a recorded red run. The stash is a reference, not a source of truth.

If `git stash push` refuses because a path is untracked: the 19 modified
source files are tracked; stash them by path as above.

`prepare-commit-msg` is a separate repository. Do not edit it until P8.

## Verified baseline

Established 2026-09-16 against mcplib `233999b` and the sibling as present on
this host. Re-confirm the HEAD hash before P1.

| Fact | How it was verified |
| --- | --- |
| OpenAI loopback is `listenFirstAvailable("127.0.0.1", openaiLoopbackPorts)` and advertises `http://localhost:{port}/auth/callback` | `llmprovider/oauth_loopback.go` `browserListener` at HEAD |
| `LoginBrowserOAuth` calls `OpenURL` synchronously before `select` on `flowCtx` | HEAD `ignoreOAuthError(config.openURL(authorizeURL))` |
| State mismatch does not send on the result channel | HEAD `TestOAuthCallback_RejectsStateMismatch` fails the test if a result arrives |
| Missing `code` does not send; IdP `error` already does | HEAD `oauthCallbackHandler` |
| `OAuthFlowOptions.InputCode` exists and is raced when non-nil | HEAD `LoginBrowserOAuth`; no test sets it |
| Wizard never sets `InputCode` | `wizard/auth.go` `oauthFlowOptions` |
| Grok callback has no CORS / OPTIONS | HEAD `oauthCallbackHandler` is path-only GET handling |
| Token exchange / refresh errors are status-only | HEAD `exchangeOAuthCode`, `refreshOAuthSession` |
| ChatGPT generate does not set `originator` | HEAD `openai.go` `doGenerateItemsOnce` |
| `StaticOpenAIChatGPT` is `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.3-codex` | HEAD `models_catalog.go` |
| Wizard ChatGPT OAuth falls back to that slice | HEAD `wizard/configure.go` `discoverModels` |
| `ValidateOAuth` accepts any non-nil session | `prepare-commit-msg/internal/config/config.go:287` |
| `main_oauth_test.go` sets `HOME`/`XDG_CONFIG_HOME` only and saves `chatgpt-access` | sibling file lines 16–24 |
| Sibling OpenURL already `Start`s | `prepare-commit-msg/internal/ui/open.go:32` |
| Sibling `go-precheck.py` forbids replace and pseudo-versions | that file, lines 4–8 |
| Latest mcplib tag is `v1.5.0` | `git tag --list 'v1.*'` |
| Official Grok CORS is origin `https://accounts.x.ai`, GET, `allow_private_network(true)` | `grok-build/.../login.rs` `build_callback_router`; `config.rs` `PROD_ACCOUNTS_APP_ORIGINS` |
| `currentToken` treats zero `Expiry` as not expired | `oauth_session.go` `currentToken` |
| `Prompter.Input` has no `context` | `wizard/prompter.go` |

## Stability rule

Every mcplib phase ends with **all** of these, redirected to a phase log, and
branched on the command's own status — never `cmd | tail && git commit`:

```text
gofmt -l <files this phase staged>
go vet ./llmprovider ./wizard
go test ./llmprovider ./wizard
```

`make lint` runs at P7 (last mcplib code phase) and must be clean there. If a
phase's new names trip goconst, fix in that phase, do not disable the linter.

A new test is not trusted until it has been seen to fail. Each phase that
adds a gate: write the test first, run the named test, capture the FAIL line
in the phase log, then implement, then capture PASS. Do not dirty production
code to create the red run.

Each mcplib phase (P0–P7) ends with **one** `git commit --no-edit` in mcplib.
P8 commits in prepare-commit-msg only, after `python scripts/go-precheck.py`
on the files actually staged. No `-m`. No `git push`. No tag except the named
gate in Rollout, and only when the same turn says to tag.

No new `go.mod` require in either repository.

## Cross-cutting contracts

1. **C1 — ChatGPT-only catalog change.** D11 deletes `StaticOpenAIChatGPT` and
   stops substituting `StaticOpenAI` after ChatGPT OAuth. It does **not**
   change `DiscoverModels` fallbacks for Gemini, Claude, Grok, Hugging Face,
   Kilo, OpenCode, or API-key OpenAI, and it does **not** make wizard
   `Discover: false` skip those static catalogs. This is the contract most
   at risk: the dirty tree already ripped those fallbacks and rewrote
   `configure_test.go` to match. Restoring that blob would look green and
   still violate D11.

2. **C2 — Red tests first.** A phase that adds a gate without a recorded FAIL
   line has not executed.

3. **C3 — Hydra and RFC 8252 URIs are frozen.** OpenAI redirect stays
   `http://localhost:{1455|1457}/auth/callback`. Grok stays
   `http://127.0.0.1:{ephemeral}/callback`. Do not advertise `127.0.0.1` for
   OpenAI. Do not bind OpenAI `:0`.

4. **C4 — Device-code is unchanged except D8's error-body helper if the
   device token HTTP path can share it without protocol changes.** Existing
   device-code tests must still pass.

5. **C5 — 0008 host-lock tests are not weakened.** ChatGPT must not hit
   `api.openai.com`. Grok must not hit `cli-chat-proxy`. Do not use
   `WithBaseURL` to hide a default-host branch in those tests.

6. **C6 — Prompter is not changed.** `InputCode` wraps `Prompter.Input`.
   Cancellation is best-effort via `LoginBrowserOAuth`'s `inputCancel` after
   `select`. A leftover blocked `Input` goroutine until process exit is
   accepted. Do not add `context` to `Prompter`.

7. **C7 — CombinedOutput is forbidden for OpenURL.** mcplib starts `OpenURL`
   in a goroutine. prepare-commit-msg already `Start`s; P8 does not revert it.

8. **C8 — No test writes the live Windows profile.** Any test that calls
   `NewOAuthStore` / `OAuthDir` / `config.Save` must set `APPDATA` and
   `AppData` (same roaming path), plus `HOME`, `USERPROFILE`,
   `XDG_CONFIG_HOME`, and `LOCALAPPDATA`.

9. **C9 — No live secrets in logs or tests.** Token error bodies go through
   `logging.RedactString`. Tests use fixture strings, never the live JWT.

10. **C10 — Do not commit the dirty working tree as one blob.**

## Dependency and delivery order

```text
P0 docs
 → P1 loopback dual-stack + waiter + OpenURL (D1, D4, D5)
 → P2 Grok CORS (D2)
 → P3 token error bodies (D8)
 → P4 originator on generate (D9)
 → P5 wizard paste-code (D3)
 → P6 session validation + import exp (D7, F11)
 → P7 ChatGPT live catalog (D11)
 → Gate: tag mcplib v1.5.1 (Rollout; not a code phase)
 → P8 prepare-commit-msg isolation + ValidateOAuth (D6, D7 consumer)
 → Operator: live browser login + delete polluted openai.json
```

P2 depends on P1 only because both edit `oauth_loopback.go`. P3 may start
after P1 (different functions) but commit after P2 to keep the loopback file
from overlapping. P4 is independent of P2/P3 once P1 has landed; commit
order above is required anyway. P5 needs P1's `InputCode` race (already at
HEAD). P6 does not need P7. P8 needs the tag gate because
`scripts/go-precheck.py` rejects replace and pseudo-versions.

## Implementation Steps

### P0 — Commit the proposed pair (docs only)

Stage only:

* `docs/decisions/0009-MADR-repair-oauth-loopback-and-session-wiring.md`
* `docs/decisions/0009-PLAN-repair-oauth-loopback-and-session-wiring.md`

`git commit --no-edit`. No Go files.

### P1 — Dual-stack OpenAI loopback, complete the waiter, background OpenURL (D1, D4, D5; closes F1, F6; part of F10)

**Files:** `llmprovider/oauth_loopback.go`, `llmprovider/oauth_loopback_test.go`.

#### Red tests (write first, capture FAIL)

1. `TestListenLoopbackBothFamilies_LocalhostDials` — bind with the new helper
   on an ephemeral port (hold `127.0.0.1:0`, close, reuse that port).
   `net.DialTimeout("tcp", net.JoinHostPort("localhost", port), time.Second)`
   must succeed. Against HEAD this fails on this Windows host.

2. `TestListenLoopbackBothFamilies_SkipsPortWhenIPv4Busy` — hold IPv4 on the
   port, call the helper, expect error (not a v6-only listener on a busy v4
   port). D1: occupied family → next registered port, not a half bind.

3. `TestListenLoopbackBothFamilies_IPv4OnlyMissesIPv6Localhost` — skip unless
   `net.DefaultResolver.LookupIP(ctx, "ip", "localhost")` returns `::1` first.
   Bind **only** `tcp4 127.0.0.1` on the port. Dial `localhost:port` must fail
   or time out within 1s. This is the 2026-09-14 probe locked as a test.

4. `TestOAuthCallback_RejectsStateMismatch` — **rewrite in place.** Today it
   `t.Fatalf` if a result arrives. New contract: HTTP 400 **and** a result
   whose `err` contains `state mismatch`, received without waiting on
   `oauthBrowserTimeout`. Same rewrite for missing `code` in a new
   `TestOAuthCallback_MissingCodeCompletesWaiter`. IdP `error` already
   completes at HEAD; add `TestOAuthCallback_IdPErrorCompletesWaiter` so it
   cannot regress.

5. `TestLoginBrowserOAuth_OpenAICompletesCallbackAndExchange` — httptest IdP
   token endpoint; override `openaiLoopbackPorts` with two ephemeral ports
   (same injection 0008 used); `OpenURL` GETs `{redirect_uri}?code=&state=`.
   Session Access/Refresh set. `redirect_uri` form field has prefix
   `http://localhost:` and suffix `/auth/callback`. Against HEAD this fails
   on this host when `http.Get` uses `localhost` → `::1`.

6. `TestLoginBrowserOAuth_OpenURLDoesNotBlockWait` — `OpenURL` blocks on
   `ctx.Done()` for longer than the test budget; a second goroutine hits the
   callback immediately. Login must return success. Against HEAD, this hangs
   on `OpenURL`. Use a 2s test deadline.

Existing `TestListenFirstAvailable_*` stay. `listenFirstAvailable` remains for
those tests; production OpenAI no longer calls it.

#### Implementation (only after the FAIL lines)

* `browserListener` returns `[]net.Listener`.
* `listenOpenAILoopback(ports []int) ([]net.Listener, int, error)` tries 1455
  then 1457. Error text **must** contain `device-code`.
* `listenLoopbackBothFamilies(port int)`:
  * `net.Listen("tcp4", 127.0.0.1:port)` and `net.Listen("tcp6", [::1]:port)`.
  * If **either** listen fails with address-in-use (match
    `address already in use` **or** Windows `only one usage of each socket
    address`), close any success and return that error so the caller tries
    the next port.
  * If one family fails for another reason (no IPv6), keep the other family.
  * If both fail, join the errors.
* `serveCallbackListeners` runs one `http.Server` per listener,
  `ReadHeaderTimeout: 5s`, shutdown all on return.
* OpenAI redirect URI remains `http://localhost:{port}/auth/callback`.
* Grok still `net.Listen("tcp", "127.0.0.1:0")` wrapped as a one-element slice.
* `oauthCallbackHandler` sends on `result` (buffered, `select default`) for
  state mismatch, IdP `error`, missing `code`, write failure, and success.
* `if config.openURL != nil { go func() { ignoreOAuthError(config.openURL(authorizeURL)) }() }`
  **before** the `select`. Do not `CombinedOutput` here (there is no exec).

Do not add CORS in this phase.

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestListenLoopbackBothFamilies_|TestOAuthCallback_|TestLoginBrowserOAuth_OpenAI|TestLoginBrowserOAuth_Grok|TestListenFirstAvailable_|TestOpenAILoopbackPorts'
go test ./llmprovider ./wizard
```

Grok browser success test must still pass (regression). Commit.

### P2 — Grok CORS and private-network preflight (D2; closes F5)

**Files:** `llmprovider/oauth_loopback.go`, `llmprovider/oauth_loopback_test.go`.

#### Red tests

1. `TestOAuthCallback_GrokOptionsReturnsPrivateNetworkCORS` — handler for
   `/callback` as Grok. Request:

   ```text
   OPTIONS /callback
   Origin: https://accounts.x.ai
   Access-Control-Request-Method: GET
   Access-Control-Request-Private-Network: true
   ```

   Expect 204 (or 200), and **exact** headers:
   * `Access-Control-Allow-Origin: https://accounts.x.ai`
   * `Access-Control-Allow-Private-Network: true`
   * `Access-Control-Allow-Methods` contains `GET`

   A handler without those headers fails this test (HEAD).

2. `TestOAuthCallback_GrokGETIncludesCORS` — successful GET with
   `Origin: https://accounts.x.ai` still returns 200 HTML and the same three
   CORS headers. Does send the code on the result channel.

3. `TestOAuthCallback_GrokDoesNotWildcardOrigin` — `Origin: https://evil.example`
   → no `Access-Control-Allow-Origin` (or not `*` and not `evil.example`).
   OPTIONS still must not send on the result channel.

4. `TestOAuthCallback_OpenAIHasNoGrokCORS` — OpenAI `/auth/callback` OPTIONS
   does **not** advertise `https://accounts.x.ai`.

#### Implementation

* Constant `grokAccountsAppOrigin = "https://accounts.x.ai"`.
* `oauthCallbackHandler(path, state string, result chan<- oauthCallbackResult, corsOrigin string)`.
  Grok passes `grokAccountsAppOrigin`; OpenAI passes `""`.
* On OPTIONS: if `corsOrigin != ""`, write CORS headers when `Origin` equals
  that origin (byte-for-byte, no trailing slash), status 204, **do not**
  inspect `state`/`code`, **do not** send on `result`.
* On GET: if origin matches, set the same CORS headers on success and error
  responses.
* Do not use `*`. Do not add a module.

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestOAuthCallback_|TestLoginBrowserOAuth_Grok'
```

Commit.

### P3 — Token-endpoint errors include a redacted body (D8; closes F7)

**Files:** `llmprovider/oauth_loopback.go`, `llmprovider/oauth_session.go`,
their tests.

#### Red tests

1. `TestExchangeOAuthCode_IncludesRedactedBody` — token endpoint returns 400
   body `{"error":"invalid_grant","error_description":"eyJhbGciOiJub25lIn0.aaa.bbb"}`
   (JWT-shaped so `logging.Redact` has something to eat). Error string contains
   `400` and does **not** contain `eyJ`. It does contain `[REDACTED]` or the
   non-secret `invalid_grant`.

2. `TestRefreshOAuthSession_IncludesRedactedBody` — same for refresh 400.

3. `TestOAuthTokenError_CapsBody` — body of 4096 `'A'` bytes; error string
   length for the body portion ≤ 2048 plus redaction/truncation marker.

#### Implementation

Shared unexported helper, cap **2048** bytes:

```go
func oauthHTTPStatusError(op string, resp *http.Response) error
```

`io.ReadAll(io.LimitReader(resp.Body, 2048))`, close body, then
`logging.RedactString`, then
`fmt.Errorf("oauth: %s failed: %s: %s", op, resp.Status, redacted)`.
Use from `exchangeOAuthCode` and `refreshOAuthSession` on non-2xx / non-200.
Do not log the raw body.

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestExchangeOAuthCode_|TestRefreshOAuthSession_|TestOAuthTokenError_'
go test ./llmprovider
```

Commit.

### P4 — ChatGPT generate sets originator (D9; pins F9)

**Files:** `llmprovider/openai.go`, `llmprovider/openai_chatgpt.go`,
`llmprovider/openai_chatgpt_test.go`.

#### Red tests

1. `TestOpenAI_ChatGPTSetsOriginatorHeader` — capturing RoundTripper, ChatGPT
   session (`Issuer: DefaultOpenAIIssuer`), `Generate`. Assert
   `request.Header.Get("originator") == "mcplib"`. Also assert
   `ChatGPT-Account-Id` still set when AccountID is set (existing test stays).

2. `TestOpenAI_ChatGPTSendsMaxOutputTokens` — same capture, JSON body still
   contains `max_output_tokens`. This pins open question 1; do **not** omit
   the field.

3. Existing host-lock test: ChatGPT generate URL host is
   `chatgpt.com` / path `.../codex/responses`, not `api.openai.com`.

#### Implementation

In `openai_chatgpt.go`:

```go
openAIOriginatorHeader = "originator"
openAIOriginatorValue  = "mcplib"
```

In `doGenerateItemsOnce`, inside `if p.chatGPT`, set that header. Authorize
URL already uses the same value; switch it to the constants so they cannot
drift (`oauth_loopback.go` `buildAuthorizeURL`).

API-key OpenAI (`p.chatGPT == false`) must **not** send `originator`.

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestOpenAI_ChatGPT|TestBuildAuthorizeURL_OpenAIContract'
```

Commit.

### P5 — Wizard races paste-code (D3; closes F4)

**Files:** `wizard/auth.go`, `wizard/auth_test.go`, plus
`llmprovider/oauth_loopback_test.go` for `parseOAuthInput` / InputCode race.

#### Red tests

1. `TestParseOAuthInput_URLAndBareCode` in `oauth_loopback_test.go` (library
   already implements this; prove it). Cases: bare code; URL with matching
   state; URL with mismatched state → error; URL with `error=` → error;
   empty → error.

2. `TestLoginBrowserOAuth_InputCodeWinsBeforeLoopback` — `InputCode` returns
   `code=pasted-secret&state=...` immediately; `OpenURL` is a no-op that does
   not hit loopback. Session exchanges `pasted-secret`. Against a build that
   ignores `InputCode`, this hangs until timeout — use 2s ctx.

3. `TestConfigureLLM_BrowserOAuthSetsInputCode` — keep the existing
   `loginBrowserOAuth` stub in `TestConfigureLLM_BrowserAndDevicePersistSessions`.
   **Add** `if opts.InputCode == nil { t.Fatal(...) }`. Against HEAD this
   fails. Also assert some `seenNotify` entry contains `paste` (case
   insensitive) **and** that the authorize URL is notified even when
   `Options.OpenURL` is non-nil (pass a no-op OpenURL).

Do not add `context` to `Prompter`. `InputCode` is:

```go
func(ctx context.Context) (string, error) {
    return p.Input("Paste the redirected URL or authorization code if the browser does not return", "")
}
```

Notify **before** starting login, from `oauthFlowOptions`:

* `Open %s in your browser` for the URL happens inside the `OpenURL` wrapper
  (even when the consumer supplied `OpenURL`).
* A second `Notify` with the paste instruction, even when `OpenURL != nil`.

`OpenURL` failure remains non-fatal (`ignoreOAuthError` in the library).

Device-code path may leave `InputCode` set; `LoginDeviceOAuth` must ignore it
(HEAD does). Add a one-line assertion in the existing device stub that
`InputCode` may be non-nil without being called, or that calling it is not
required.

#### Verification

```text
go test ./wizard -count=1 -run 'TestConfigureLLM_Browser'
go test ./llmprovider -count=1 -run 'TestParseOAuthInput_|TestLoginBrowserOAuth_InputCode'
go test ./wizard ./llmprovider
```

Commit.

### P6 — Reject stub sessions; import JWT exp (D7, F11; closes F8, F11)

**Files:** `llmprovider/oauth_session.go`, `llmprovider/oauth_session_test.go`,
`wizard/auth.go`, `wizard/auth_test.go`, `wizard/import.go`,
`wizard/import_test.go`.

#### Red tests

1. `TestValidateOAuthSession_RejectsFixture` —
   `{Access: "chatgpt-access", Issuer: DefaultOpenAIIssuer}` → error.
   Empty Access → error.
   `{Access: "x", Refresh: ""}` with non-ChatGPT issuer → error.
   Refreshable `{Access, Refresh, ClientID, TokenURL}` → nil.
   Access-only ChatGPT: `Issuer=DefaultOpenAIIssuer`,
   `ClientID=DefaultOpenAIClientID`, `Refresh=""`, `Expiry` zero, Access a
   three-segment JWT-shaped string `eyJhbGciOiJub25lIn0.e30.x` → nil.
   Access-only with **non-zero** Expiry → error (D7: expiry zero on purpose).

2. `TestSaveOAuthCredential_RejectsFixture` — `saveOAuthCredential` with the
   fixture does not `Save` (memory store `saves==0`) and returns an error
   containing `chatgpt-access` **or** `stub` **or** `refresh`. Do not require
   a specific English sentence beyond that it fails.

3. `TestConfigureLLM_TokenStdinClassification` "openai access token" case:
   change the secret from `chatgpt-access` to a JWT-shaped access-only token.
   A new subtest `chatgpt-access fixture` expects configure error, saves 0.

4. `TestImportOpenAIAuth_SetsExpiryFromJWT` — vendor JSON access is a JWT
   with `exp` 2000000000; imported `Expiry` is that unix time (UTC). Access
   without `exp` stays zero and still imports if refresh is present.

#### Implementation

Exported:

```go
func ValidateOAuthSession(session *OAuthSession) error
```

Rules, in order:

1. `session == nil` or `strings.TrimSpace(session.Access) == ""` → error.
2. `session.Access == "chatgpt-access"` → error (the live fixture).
3. If `session.Refresh == ""`: allow only ChatGPT access-only:
   `strings.TrimRight(session.Issuer, "/") == DefaultOpenAIIssuer`,
   `session.ClientID == DefaultOpenAIClientID`, `session.Expiry.IsZero()`.
   Else error (`oauth: no refresh token`).
4. If `session.Refresh != ""`: require non-empty `ClientID` and `TokenURL`.

`saveOAuthCredential` calls `ValidateOAuthSession` **before** `Save`.
`keepExistingOAuth`, after the user confirms keep, validates and refuses to
keep a stub (return error, do not return the stub credential).

`accessOnlyOpenAISession` already sets `ClientID: DefaultOpenAIClientID` and
zero Expiry — it stays the access-only path.

`importOpenAIAuth`: after copying tokens, if access is a JWT, base64url-decode
payload, read numeric `exp`, set `Expiry` to `time.Unix(exp, 0).UTC()`.
Malformed JWT → leave Expiry zero (do not fail the import). Still requires
access **and** refresh as today.

Do not parse a `last_refresh` field; the vendor struct this repo reads has
none.

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestValidateOAuthSession_'
go test ./wizard -count=1 -run 'TestSaveOAuthCredential_|TestConfigureLLM_TokenStdin|TestImportOpenAI|TestConfigureLLM_Import'
```

Commit.

### P7 — ChatGPT live catalog (D11; closes F12)

**Files:** `llmprovider/discovery.go`, `llmprovider/discovery_test.go`,
`llmprovider/models_catalog.go`, `llmprovider/models_catalog_test.go`,
`llmprovider/openai.go` (**ChatGPT branch of `DiscoverModels` only**),
`wizard/configure.go`, `wizard/configure_test.go`, `wizard/auth_test.go`.

**Forbidden in this phase:** changing `DiscoverModels` fallback in
`claude.go`, `gemini.go`, `grok.go`, `huggingface.go`, `kilo.go`,
`opencode.go`. Changing wizard `Discover: false` for non-ChatGPT providers.

#### Red tests

1. `TestListAvailableModelsWithSource_ChatGPTListsCodexCatalog` — httptest
   returns mixed visibility. Assert order by `priority` ascending, then
   payload order: hidden omitted; `supported_in_api: false` omitted.
   Request path `/models`, query `client_version=0.0.0`, header
   `originator=mcplib`, `Authorization: Bearer …`, `ChatGPT-Account-Id` when
   set. Host is the httptest server (via `WithBaseURL`), **not**
   `api.openai.com`. Returned slice is a copy (mutating it does not affect
   the next call).

2. `TestListAvailableModelsWithSource_ChatGPTListingFailureIsError` — 502 →
   `err != nil` and `models == nil`. Must not equal `StaticOpenAI`.

3. `TestStaticOpenAIChatGPTCatalog` at HEAD expects the frozen slice.
   **Delete this test** only after a replacement test
   `TestStaticOpenAIChatGPTRemoved` that does not compile if the identifier
   is reintroduced: a file-level check is unnecessary; grep in verification
   plus deleting the identifier is enough. Add
   `TestStaticModels_OpenAIIsPlatformCatalog` asserting `StaticModels("openai")`
   is `StaticOpenAI` (gpt-4.1-*), not the ChatGPT 5.x slice.

4. `TestConfigureLLM_ChatGPTListingFailurePromptsForModel` — OpenAI, auth
   method keep-existing OAuth (or stubbed browser that returns a ChatGPT
   session with refresh/client/tokenURL so P6 save accepts it),
   `Discover: true`, HTTPClient hitting 502. Script `inputs` with
   `manual-chatgpt`. Result model is that string. `seenSelect` model menu,
   if any, must not contain `gpt-5.4` or `gpt-4.1-mini`. Against HEAD this
   fails because `discoverModels` returns `StaticOpenAIChatGPT`.

5. `TestConfigureLLM_APIKeyKindUnchanged` / other existing wizard tests that
   use `Discover: false` must still get static catalogs for Claude/Gemini
   **without** new Input prompts. If a test starts failing because the menu
   disappeared, **the implementation overshot C1 — revert that part**.

#### Implementation

* `listChatGPTModels` as specified in D11. Constant
  `chatgptModelsClientVersion = "0.0.0"`.
* `ListAvailableModelsWithSource`: if provider is OpenAI **and**
  `isChatGPTTokenSource(src)`, call `listChatGPTModels` and return. Do not
  fall through to `listOpenAIModels`.
* Delete `StaticOpenAIChatGPT` and its test.
* `wizard/configure.go` `discoverModels`:
  * Keep `fallback := d.StaticModels` for non-ChatGPT.
  * If `res.Kind == CredOAuth && d.ID == ProviderOpenAI`, **do not** set
    fallback to `StaticOpenAIChatGPT` or `StaticOpenAI`; fallback is nil.
  * `Discover: false` returns that fallback (nil for ChatGPT OAuth → caller
    already prompts when `len(models)==0`).
  * Listing failure: notify **without** `using the built-in catalog` when
    fallback is nil; when fallback is non-nil, keep today's notify+fallback
    behaviour.
* Add `Options.HTTPClient` so tests (and P8) can inject a client for listing
  without `WithBaseURL` fighting ChatGPT host selection. Production nil →
  package default.
* `OpenAIProvider.DiscoverModels`: on listing failure, if `p.chatGPT` return
  the error; else keep `StaticModels(ProviderOpenAI)`.

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestListAvailableModelsWithSource_ChatGPT|TestStaticModels|TestOpenAI_'
go test ./wizard -count=1
go test ./llmprovider ./wizard
gofmt -l llmprovider/*.go wizard/*.go
go vet ./llmprovider ./wizard
make lint
```

`rg StaticOpenAIChatGPT` prints nothing.

Commit. This is the last mcplib code phase.

### P8 — prepare-commit-msg isolation and ValidateOAuth (D6, D7 consumer; closes F3)

**Repo:** `../prepare-commit-msg`. **Blocked** until Rollout tags `v1.5.1`
and this phase's `go get` uses that tag (C: no replace, no pseudo-version).

**Files:** listed in Scope.

#### Red tests

1. `TestIsolateHome_RedirectsWindowsUserConfigDir` in `config_test.go` —
   `runtime.GOOS != "windows"` → skip. Record `os.UserConfigDir()` before
   `isolateHome`. After, `UserConfigDir` ≠ before and has prefix `tmp`.
   Against today's `isolateHome` this **passes** (it already sets `AppData`).
   Prove the test itself by running a scratch copy that omits `AppData` /
   `APPDATA` and observing FAIL, without committing that scratch.

2. `TestRedirectUserConfig_APPDATARequired` — the helper used by
   `main_oauth_test.go` must set both `APPDATA` and `AppData` to the same
   roaming path. A unit that reads `os.Getenv("APPDATA")` after isolate
   finds the temp path.

3. `TestValidateOAuth_RejectsChatGPTAccessFixture` — temp `FileTokenStore`,
   session Access `chatgpt-access`, empty refresh, default issuer.
   `ValidateOAuth` returns error. Against HEAD this fails (returns nil).

4. `TestRunAnalyzer_OAuthDoesNotCallNewProviderWithAccessToken` — must call
   the isolation helper **before** `NewOAuthStore`. Access token in the
   saved session must be a P6-valid refreshable or access-only token, **not**
   `chatgpt-access` (otherwise P6's validator, once consumed, fails the
   analyzer before the stubbed generate). After the test, a canary that
   reads the real `%APPDATA%\prepare-commit-msg\oauth\openai.json` (the
   pre-test path, captured before isolate) must not have been rewritten.
   Implementation: capture `os.UserConfigDir()` before isolate; after the
   test, if that directory still contains `prepare-commit-msg/oauth/openai.json`,
   its Access must not have become `chatgpt-access` **from this test**. The
   MADR canary is: after `go test` of the OAuth tests, live Access is not
   the fixture **written by those tests**. Isolation is what makes that true.

#### Implementation

* Expand `isolateHome` and `ui.isolate` to set, all to paths under
  `t.TempDir()`:

  ```text
  HOME, USERPROFILE, XDG_CONFIG_HOME,
  APPDATA, AppData,           # same roaming dir
  LOCALAPPDATA
  ```

* `main_oauth_test.go` calls that helper (duplicate the Setenv block, or
  export nothing from config.go — duplicate is required because the test is
  `package main`). Do not inject `userConfigDir` in this test; D6 wants the
  real `os.UserConfigDir` redirected via env.

* `ValidateOAuth`: after non-nil `Load`, call
  `llmprovider.ValidateOAuthSession(session)` and wrap the error with
  `run 'prepare-commit-msg configure'`.
  Update `TestValidateActive_OAuthWithoutKeyOK`: its session is currently
  `Access: "access-token"` with empty refresh and empty client ID — that
  must become a P6-valid refreshable session (`Refresh`, `ClientID`,
  `TokenURL` set) so the test still proves "OAuth without API key is OK".

* `go get github.com/maccavelli/mcplib@v1.5.1` then `go mod tidy`.
  `go.mod` must show `v1.5.1`, not a pseudo-version.

* `python scripts/go-precheck.py` on the staged list. No `replace`.

`internal/ui/open.go` is already `Start` — do not revert; no edit unless
gofmt.

Setup tests that run ChatGPT OAuth with `Discover: true` will now call
Codex `/models`. If a test hits the network or fails listing and then
dead-ends: inject `wizard.Options.HTTPClient` from `setup.go` only if a
test requires it; production may keep the default client. Prefer fixing
tests that already isolate and use API-key OpenAI (no ChatGPT listing).
Do not set `Discover: false` globally.

#### Verification

```text
python scripts/go-precheck.py main_oauth_test.go internal/config/config.go internal/config/config_test.go internal/ui/setup_test.go go.mod go.sum
go test ./...
```

On this Windows host, after the test run, live
`%APPDATA%\prepare-commit-msg\oauth\openai.json` Access is not
`chatgpt-access` **because of these tests**. If it is still the fixture from
before P8, that is operator recovery (Deferred), not a test failure, as long
as the canary inside the test never wrote it.

Commit in prepare-commit-msg only.

### R1 — Repair the `ba92db1` merge (added 2026-09-27; D11, C1, C10)

`ba92db1` committed the draft this plan's C10 forbids, with conflict
markers left in place. `main` does not build (see the MADR amendment of
2026-09-27). Owner approval: "proceed to 1. repair main", 2026-09-27. R1
runs before any other open phase, because every other phase's gate needs a
tree that builds.

**Baseline:** `6e19cdf` (`ba92db1` plus the 0013 docs). **Red:** at that
baseline `go build ./...` exits 1 with
`llmprovider/discovery.go:335:1: syntax error: unexpected <<, expected }`,
and `go vet ./...` also reports `wizard/configure_test.go:244:1: expected
operand, found '<<'`.

**Files:**

* `llmprovider/discovery.go`: resolve all 20 conflict blocks to the
  `5a1fc70` side. The result must equal `ca29b81` except for the D11 lister
  (`chatgptModelsClientVersion`, `chatGPTCatalogModel`, `listChatGPTModels`,
  `chatGPTCatalogModelListed`) and the one changed line in
  `ListModelCatalogWithSource`.
* `llmprovider/claude.go`, `gemini.go`, `grok.go`, `huggingface.go`,
  `kilo.go`, `opencode.go`, `wizard/configure_test.go`,
  `wizard/import_test.go`: restore their `ca29b81` content (C1).
  `catalogFrom` already substitutes the static catalog when a listing
  fails, so the `DiscoverModels` hunks cannot be told apart by any test.
  They are restored for conformance and so their doc comments are true.
* `llmprovider/openai.go`: in `DiscoverModels`, a listing failure returns
  the error for a ChatGPT session and falls back to
  `StaticModels(ProviderOpenAI)` otherwise (P7's last bullet).
* `wizard/configure.go`:
  * `static` is nil for a ChatGPT session.
  * The listing-failure notice drops "using the built-in catalog" when there
    is none.
  * `Options.HTTPClient` is passed with `WithHTTPClient`.
  * The `Existing.BaseURL` second `WithBaseURL` is removed.
  * The `Discover` doc comment describes both cases.
* Tests:
  * `llmprovider/discovery_test.go` is `ca29b81` with
    `…_ChatGPTDoesNotHTTP` replaced by `…_ChatGPTListsCodexCatalog` and
    `…_ChatGPTListingFailureIsError`.
  * `llmprovider/discovery_catalog_test.go`:
    `TestListModelCatalogWithSource_ChatGPTDoesNotHTTP` becomes
    `…_ChatGPTListsCodexCatalog`.
  * `wizard/model_select_test.go`: `TestConfigureLLM_ChatGPTNoStaticNotice`
    injects a Codex listing instead of reaching the network.
  * `wizard/auth_test.go` is `ca29b81` plus the three ChatGPT model-id edits
    and the new `TestConfigureLLM_ChatGPTListingFailurePromptsForModel`.
  * `llmprovider/probe_test.go` gains
    `TestOpenAIProvider_ChatGPTDiscoverModelsListingFailureIsError`.
  * `llmprovider/models_catalog_test.go` gains
    `TestStaticModels_OpenAIIsPlatformCatalog`.

**Mutants.** Each mutant puts back one behaviour R1 removes, and its named
test must fail at runtime:

* A ChatGPT static fallback.
* A notice that claims a built-in catalog.
* No static catalog for any provider.
* A static fallback in ChatGPT `DiscoverModels`.
* A listing failure that returns an error instead of degrading.
* Hidden Codex slugs kept.
* A ChatGPT listing failure that returns `StaticOpenAI`.
* A silent state mismatch.
* No originator on generate.
* ChatGPT `Recommended` and `Usable` sharing an array.
* The static notice fired for ChatGPT.
* `StaticModels(openai)` returning the old ChatGPT slice.

**Expected survivor.** An IPv4-only bind is expected to survive
`TestListenLoopbackBothFamilies_LocalhostDials` on macOS and Linux. Go's
dialer falls back from `::1` to `127.0.0.1`. That is acceptance criterion
A1's Windows-only negative case, still owed by P1.

**Verification.** On the repaired tree:
* `gofmt -l` on every changed file
* `golint -set_exit_status` per file
* `go build ./...`
* `go vet ./...`
* `go vet -tags live_gateways ./llmprovider`
* `make lint`
* `go test -count=1 ./...`
* `go test -race -count=1 ./llmprovider ./wizard`

No `.go` file may hold a conflict marker, and no file may reference
`StaticOpenAIChatGPT`. Commit with `git commit --no-edit`. Do not push.

**Not in R1:**
* P1's four missing tests: `…_IPv4OnlyMissesIPv6Localhost`,
  `…_OpenURLDoesNotBlockWait`, `…_MissingCodeCompletesWaiter`,
  `…_IdPErrorCompletesWaiter`.
* P4's `TestOpenAI_ChatGPTSendsMaxOutputTokens`.
* P2, P3, P5, P6 and P8.

## Verification (whole plan)

### Acceptance criteria (mapped to MADR Confirmation)

| # | Criterion | MADR |
| --- | --- | --- |
| A1 | Dual-stack OpenAI listener: `Dial localhost:port` succeeds on this host; IPv4-only bind makes that dial fail when `localhost` is `::1` first | D1, F1, Confirmation bullet 1 |
| A2 | Grok OPTIONS `/callback` from `https://accounts.x.ai` returns Allow-Private-Network true; without the header the new test fails | D2, F5 |
| A3 | Wizard browser OAuth sets `InputCode`; scripted InputCode before loopback Saves a session; `InputCode` nil fails the new test | D3, F4 |
| A4 | `TestOAuthCallback_RejectsStateMismatch` receives an error on the channel inside the test | D5, F6 |
| A5 | `OpenURL` blocking does not delay the timeout select | D4 |
| A6 | `go test` in prepare-commit-msg with isolation: live store is not written; helper test fails if `UserConfigDir` unchanged on Windows | D6, F3 |
| A7 | `ValidateOAuth` / `ValidateOAuthSession` on Access `chatgpt-access` empty refresh **fails** | D7, F8 |
| A8 | Token 400 error contains redacted body, not the raw JWT | D8, F7 |
| A9 | ChatGPT generate capturing transport asserts `originator=mcplib`; `max_output_tokens` still present | D9, F9 |
| A10 | ChatGPT `/models` capturing transport: path, `client_version=0.0.0`, not `api.openai.com`; 502 → error nil slice; `StaticOpenAIChatGPT` gone | D11, F12 |
| A11 | 0008 host-lock tests still pass (ChatGPT ↛ `api.openai.com`; Grok ↛ `cli-chat-proxy`) | Decision Drivers |
| A12 | `go test ./llmprovider ./wizard` exit 0; prepare-commit-msg `go test ./...` exit 0; no new `go.mod` require | Confirmation |
| A13 | Device-code tests still pass | Decision Drivers |

**Most likely to be quietly dropped under pressure: A1's IPv4-only negative
case, and C1 (not ripping every static fallback).** A1's positive dial can
pass on Linux CI where `localhost` is IPv4. The skip-guarded negative test
plus a recorded run on **this** Windows host are required. C1 fails if P7
touches `claude.go` / `gemini.go` `DiscoverModels`.

Live OpenAI browser login on this laptop producing `oauth/openai.json` with
JWT access, non-empty refresh, `client_id`, and `account_id` is product
confirmation. It is not a unit test and does not block merging P1–P8.

## Rollout and Rollback

**Order.**

1. P0–P7 in mcplib, one commit each, no push.
2. **Gate (needs an explicit "tag and push" in the same turn):** tag
   `v1.5.1` on the P7 commit once `go test ./...` is green. Do not move
   `v1.5.0`. `go list -m github.com/maccavelli/mcplib@v1.5.1` must resolve.
3. P8 in prepare-commit-msg: `go get github.com/maccavelli/mcplib@v1.5.1`.
4. Operator recovery: delete or replace
   `%APPDATA%\prepare-commit-msg\oauth\openai.json` if Access is still
   `chatgpt-access`. Then run `prepare-commit-msg configure` Sign in with
   ChatGPT.

**Rollback.** Revert the mcplib commits (or unpin prepare-commit-msg to
`v1.5.0`). Device-code and API keys remain the 0008 paths. Dual-stack /
CORS / paste-code / validation all revert together; do not leave P6
validation in the consumer against a library that lacks
`ValidateOAuthSession`.

## Deferred (named, so they are not mistaken for oversights)

* **Live originator / `max_output_tokens` probe (open question 1).** D9 and
  A9 ship and pin the header and the field. Whether the backend 401s without
  originator, or 400s on `max_output_tokens`, waits for a real ChatGPT
  session after P1–P7. Record the HTTP status in this PLAN's execution
  record; do not change generate in that same turn.
* **Deleting the polluted live `openai.json` (open question 3).** Operator
  step in Rollout, not code.
* **Fleet consumers other than prepare-commit-msg.** MagicDev / MagicTools
  wizards are out. They pick up `v1.5.1` when those repos bump, under their
  own plans.
* **HAR proof of Grok PNA.** F5 remains inferred from official CLI source.
  P2 locks the headers that CLI sends; a browser HAR is not required to
  close the phase.
* **Making `Prompter.Input` cancellable.** C6 accepts a leftover goroutine.
* **All-provider "no static fallback" wizard.** Explicitly rejected by D11 /
  C1. If wanted later, that is a new MADR.

## Deviation log

* **2026-09-27: phases landed outside the plan.** `ba92db1` combined P0's
  docs with source and tests in one commit. That breaks the bootstrap
  exception and C10. The commit carries parts of P1, P4 and P7 with no
  recorded red runs, the static-fallback removal that C1 forbids, and
  unresolved conflict markers, so `main` does not build. Resolution chosen
  by the owner: repair in place (R1), not revert. Reverting would also drop
  D1, D4, D5 and D9, which do work, and the history is already published.
  The MADR records this in its 2026-09-27 amendment.
* **2026-09-27 — P1 test 3 cannot work as written.**
  * Evidence, from a probe on the macOS host: `LookupIP("localhost")` returns
    `[::1 127.0.0.1]`, so the test's skip guard does not fire.
    `net.DialTimeout("tcp", "localhost:port")` then connects to an
    IPv4-only listener through `127.0.0.1`, because Go's dialer falls back
    across families. `net.DialTimeout("tcp6", "[::1]:port")` is refused.
  * The test as written would therefore always fail here. It also can't catch
    the IPv4-only-bind mutant, which survived R1.
  * Owner's resolution: explicit per-family dials. Test 3 asserts that an
    IPv4-only listener refuses `tcp6 [::1]`, which is the browser's first
    attempt. A new test, `TestListenLoopbackBothFamilies_AcceptsIPv6Loopback`,
    requires the helper to accept `tcp6 [::1]`. Both skip only when the host
    has no IPv6 loopback.
* **2026-09-27 — P6 rule 4 would refuse every kept session.**
  * Evidence: `wizard.Result` has no `TokenURL`, and `keepExistingOAuth`
    builds its session without one. `refreshTokenURL` derives the URL from
    the issuer (OpenAI) or uses the Grok default.
  * A literal rule 4 fails `TestConfigureLLM_ChatGPTResultDoesNotPopulateAPIKey`,
    `TestConfigureLLM_ChatGPTListingFailurePromptsForModel` and
    `TestConfigureLLM_ChatGPTNoStaticNotice`.
  * Separately, any keep-time D7 check refuses the no-refresh Grok session
    that 0013's `TestConfigureLLM_ListingTokenFailureUsesStaticCatalog`
    relies on.
  * Owner's resolution:
    * Rule 4 requires a non-empty `ClientID`; `TokenURL` may be empty.
    * The browser and device-code results must still carry `TokenURL`, as D7
      says, and a test pins that.
    * 0013's coverage test moves to a valid refreshable Grok session with
      `DiscoverLimit: time.Nanosecond`. Its refresh fails on the expired
      deadline before any network I/O, so the test still reaches the
      built-in-catalog warning.
* **2026-09-27 — P5's paste race steals the next line.**
  * Evidence: `TextPrompter` has no lock, and every read shares one
    `bufio.Reader` (`readLine`, `wizard/text_prompter.go:101-114`).
  * After a successful loopback login, the paste prompt's `Input` stays
    blocked in `ReadString`, and C6 accepts that leftover. The wizard's next
    prompt, the model menu, then reads the same reader concurrently. That
    is a data race, and the user's model choice can be swallowed.
  * Also, `LoginBrowserOAuth` runs `OpenURL` and `InputCode` on separate
    goroutines since D4, so the prompter would be written from two
    goroutines.
  * Owner's resolution: drain after success.
    * `InputCode` starts its prompt only after the `OpenURL` wrapper has
      shown the authorize URL and the paste instruction.
    * When the login returns while that prompt is still waiting, the wizard
      shows "Browser sign-in finished; press Enter to continue". It then
      waits for the pending read before its next prompt.
    * So only one read is ever pending. The browser path costs one Enter.
      `Prompter` is unchanged, as C6 requires.
  * New test: `TestConfigureLLM_BrowserLoopbackWinDrainsPastePrompt`.

## Execution record (2026-09-27)

Columns: phase, status, commit, red-test FAIL line, green-test PASS line,
what the plan predicted incorrectly.

| Phase | Status | Commit | Red | Green | Plan vs. actual |
| --- | --- | --- | --- | --- | --- |
| P0 | done, out of order | `ba92db1` | n/a (docs) | n/a | Committed together with source (C10) |
| P1 | done | `ba92db1` (code), then the P1 commit (tests) | The code landed first, so the new tests pass at HEAD. Each was proven by a mutant on a scratch copy: an IPv4-only bind fails `…_AcceptsIPv6Loopback` (`connect: connection refused`); an IPv6 twin fails `…_IPv4OnlyMissesIPv6Localhost` (`reached an IPv4-only listener`); a silent missing code and a silent IdP error each fail their test (`callback did not complete the waiter`); a synchronous OpenURL fails `…_OpenURLDoesNotBlockWait` (`waited on OpenURL instead of the callback`) | All 13 loopback tests pass; the gate passes | Test 3 was rewritten with explicit IPv6 dials (deviation log). The IPv4-only mutant that survived R1 is now caught |
| P2 | done | the P2 commit | The tests call the new four-argument `oauthCallbackHandler`, so at HEAD they fail to compile, which proves nothing. Each was proven by a mutant on a scratch copy: no private-network header (`…Allow-Private-Network = "", want true`); CORS on OPTIONS only (`GETIncludesCORS`: `…Allow-Origin = ""`); any origin allowed (`…for a foreign origin, want none`); OpenAI given the Grok origin; the preflight falls through to the state check (`preflight status = 400`); login passes no origin (the Grok login test's preflight fails) | The 4 named tests and both login tests pass; the gate passes | `callbackCORSOrigin` was added so the login's wiring could be tested, and the Grok login test now sends the accounts.x.ai preflight. An OPTIONS never completes the waiter, for either provider: at HEAD an OpenAI preflight fell into the state-mismatch path |
| P3 | done | the P3 commit | At HEAD: `error = "oauth: token exchange failed: 400 Bad Request", want the status and the body's invalid_grant`; the same for `refresh failed`; `error carries 0 body bytes, want exactly the first 2048` | All 3 pass; the gate passes | `oauthHTTPStatusError` lives in `oauth_session.go` and is used by both paths. Device-code polling is unchanged (C4 made sharing optional) |
| P5 | done | the P5 commit | At HEAD both wizard tests failed: `browser OAuth has no paste-code InputCode`. The library tests pass at HEAD and were proven by mutants: no state check (`= "pasted-code"/<nil>, want an error containing "state mismatch"`); `InputCode` ignored (`browser login timed out`); no drain (`Select("Choose a Grok (xAI) model:") started while the paste prompt was still reading`); no paste hint; URL hidden when the consumer opens the browser | Browser, device, stdin and import wizard tests pass; the gate passes, including `-race` | Drain after success, per the deviation log. The device flow gets no `InputCode` at all: `browserFlowOptions` adds it to the browser path only, so the plan's device-stub assertion is moot. The prompter's leftover goroutine (C6) can no longer outlive the login |
| P6 | done | the P6 commit | At HEAD:<br>• `saveOAuthCredential() error = <nil> with 1 saves`<br>• the stdin `chatgpt-access fixture` case saved the stub (`with 1 saves`)<br>• the keep test kept the stub (`error = <nil>`)<br>• `Expiry = 0001-01-01 …, want 2033-05-18 03:33:20 +0000 UTC`<br>`ValidateOAuthSession` is new API, so 12 mutants proved it and the pins instead, each killed. One is literal rule 4 (TokenURL required), which fails the derived-URL case, so the amendment is pinned | Every P6 test passes; 0013's moved coverage test passes; the gate passes | Rule 4 requires ClientID only, per the deviation log. Kept sessions are validated and a stub is refused with "sign in again". Both browser-login tests pin `TokenURL` and `ClientID` on fresh sessions. 0013's `TestConfigureLLM_ListingTokenFailureUsesStaticCatalog` now uses a refreshable Grok session with a 1 ns limit, and its mutant is still killed |
| P4 | done | `ba92db1` (originator), then the P4 commit (the pin) | The originator test was proven in R1 (no header gives `originator = "", want "mcplib"`). `TestOpenAI_ChatGPTSendsMaxOutputTokens` pins today's body, so it was proven by a mutant: dropping the field gives `max_output_tokens = <nil>, want 321` | All ChatGPT tests and `TestBuildAuthorizeURL_OpenAIContract` pass; the gate passes | None |
| P7 | done by R1 | `ba92db1`, then R1 | see R1 | see R1 | The llmprovider half landed in `ba92db1`; the wizard half and C1 only in R1 |
| R1 | done | the commit after `fc3e559` that adds this row | `6e19cdf`: `go build ./...` exit 1, `discovery.go:335:1: syntax error: unexpected <<` | Full gate PASS (below) | Added 2026-09-27. Two further stale `StaticOpenAIChatGPT` tests surfaced (`discovery_catalog_test.go`, `model_select_test.go`) and were rewritten in R1 |
| P8 | not started | | | | Blocked on the `v1.5.1` tag |

### R1 verification (2026-09-27)

R1 was first built and proven on a scratch archive of `fc3e559`. It was then
applied to the working tree, and the 17 changed files match the proven tree
byte for byte. The gate was then run in the working tree:

```text
gofmt -l <17 files>                            exit=0, no output
golint -set_exit_status <each file>            17 x exit=0
go build ./...                                 exit=0
go vet ./...                                   exit=0
go vet -tags live_gateways ./llmprovider       exit=0
make lint                                      exit=0
go test -count=1 ./...                         exit=0 (8 packages ok)
go test -race -count=1 ./llmprovider ./wizard  exit=0
```

Structure:
* No `.go` file holds a conflict marker.
* `StaticOpenAIChatGPT` has no references.
* `discovery.go` against `ca29b81` removes exactly one line:
  `return staticCatalog(slices.Clone(StaticOpenAIChatGPT)), nil`. Every other
  change adds the D11 lister.
* The eight restored files equal `ca29b81`.

Mutants (a mutant counts as killed only by a runtime `--- FAIL`):

| Mutant | Test | Result |
| --- | --- | --- |
| ChatGPT static fallback restored (`configure.go`) | `TestConfigureLLM_ChatGPTListingFailurePromptsForModel` | killed: `unexpected Select("Choose a OpenAI model:")` |
| same | `TestConfigureLLM_TokenStdinClassification` | killed: `oauth model = "gpt-4.1-mini", want chatgpt-model` |
| same | `TestConfigureLLM_ChatGPTResultDoesNotPopulateAPIKey` | killed: `unexpected Select("Choose a OpenAI model:")` |
| Failure notice claims a built-in catalog | `TestConfigureLLM_ChatGPTListingFailurePromptsForModel` | killed: `notice "…(model listing: chatgpt HTTP 502); using the built-in catalog" claims a built-in catalog…` |
| No static catalog for any provider (C1) | `TestConfigureLLM_PromptsWhenNothingAvailable` | killed: `wizard: no model available for Claude (Anthropic) and none entered` |
| ChatGPT `DiscoverModels` falls back to static | `TestOpenAIProvider_ChatGPTDiscoverModelsListingFailureIsError` | killed: `DiscoverModels() = [gpt-4.1-mini …]/<nil>, want nil/listing error` |
| Gemini listing failure returns an error (C1) | `TestListAvailableModels_Gemini` | killed: `fallback error: gemini: models endpoint returned HTTP 500` |
| Codex `hide` slugs kept | `TestListAvailableModelsWithSource_ChatGPTListsCodexCatalog` | killed |
| Codex 502 returns `StaticOpenAI` | `TestListAvailableModelsWithSource_ChatGPTListingFailureIsError` | killed: `error = nil, want listing failure` |
| Recommended and Usable share an array | `TestListModelCatalogWithSource_ChatGPTListsCodexCatalog` | killed: `Recommended and Usable share a backing array` |
| Static notice fired for ChatGPT | `TestConfigureLLM_ChatGPTNoStaticNotice` | killed |
| `StaticModels(openai)` returns the old ChatGPT slice | `TestStaticModels_OpenAIIsPlatformCatalog` | killed |
| State mismatch does not complete the waiter (D5) | `TestOAuthCallback_RejectsStateMismatch` | killed: `mismatched state did not complete the waiter` |
| No originator on generate (D9) | `TestOpenAI_ChatGPTSetsOriginatorHeader` | killed: `originator = "", want "mcplib"` |
| IPv4-only loopback bind (D1) | `TestListenLoopbackBothFamilies_LocalhostDials` | survived, as expected on macOS: A1's negative case needs the Windows host (P1) |
