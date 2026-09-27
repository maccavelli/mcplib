---
status: proposed
date: 2026-09-20
associated-madr: 0010-MADR-windows-stdio-oauth-tokenstore-ci.md
decision-makers: mcplib maintainers
---

<!-- markdownlint-disable MD013 MD024 MD033 MD036 MD060 -->

# PLAN 0010 — Windows stdio shutdown, 0009 mcplib remainder, FileTokenStore, CI parity

Implements [0010-MADR-windows-stdio-oauth-tokenstore-ci.md](0010-MADR-windows-stdio-oauth-tokenstore-ci.md)
decisions D1–D18, closing findings F1–F18.

If execution discovers a fact that contradicts the MADR, **stop and amend the
MADR** before continuing. Do not smuggle a different architecture into a phase.

## Goal

When this plan is complete, all of the following are true in this repository
(no sibling repos, no tag, no push):

1. `IsExpectedShutdownErr(&os.PathError{Err: windows.ERROR_BROKEN_PIPE})` is
   true on `GOOS=windows`. The Windows-tagged test that asserts it was seen
   to FAIL against unmodified `stdio.go`, then PASS. `TestIsExpectedShutdownErr_Typed`
   still passes on every OS.
2. Grok `OPTIONS /callback` from `Origin: https://accounts.x.ai` returns 204
   (or 200) with the three CORS/PNA headers and does **not** send on the
   login waiter. OpenAI `/auth/callback` does not advertise that origin.
   Redirect URIs are unchanged: OpenAI `http://localhost:{1455|1457}/auth/callback`,
   Grok `http://127.0.0.1:{ephemeral}/callback`.
3. Wizard browser OAuth always sets `InputCode`. A notify contains
   `Paste the URL here if it doesn't connect:`. A scripted paste completing
   before loopback still exchanges the pasted code.
4. Token-endpoint and refresh HTTP errors include status plus a ≤2048-byte
   `logging.RedactString` body. A JWT-shaped fixture in the body appears as
   `[REDACTED]`.
5. `ValidateOAuthSession` rejects `chatgpt-access`. `saveOAuthCredential`
   does not `Save` that fixture. OpenAI vendor import sets `Expiry` from JWT
   `exp` when present.
6. `FileTokenStore.Save` `Sync`s before rename. `validateProviderID` rejects
   `CON` / `con` / `NUL` / `COM1` / `CON.json`. On Windows the session file
   has a protected DACL granting `GENERIC_ALL` to the current user.
7. A successful refresh that then fails `Save` still `adopt`s the new tokens
   in memory. That `Token()` call returns the save error; the next `Token()`
   in-process returns the new access without a second HTTP refresh.
8. `StaticOpenAIChatGPT` does not compile. ChatGPT listing against a capturing
   transport hits `{codex}/models?client_version=0.0.0` with `originator=mcplib`
   and a host that is not `api.openai.com`.
9. `.github/workflows/ci.yml` runs `go vet ./...` and `go test -race ./...`
   on ubuntu-24.04, macos-15, and windows-2025; runs pinned `govulncheck ./...`
   on Linux; sets `shell: bash` on the release-guard scripts; and cross-compiles
   linux/darwin/windows × amd64/arm64 plus Windows `go test -c` for
   `./selfupdate` and `./llmprovider`.

`go test ./...` and `go vet ./...` exit 0 on the execution host. `make lint`
is clean after the last Go phase. 0008 host-lock tests still pass. No new
`go.mod` require.

## Scope

### In scope (the only files any phase may touch)

**P0 (docs only):**

* `docs/decisions/0010-MADR-windows-stdio-oauth-tokenstore-ci.md`
* `docs/decisions/0010-PLAN-windows-stdio-oauth-tokenstore-ci.md`

**P1 — shutdown (D1, D2; closes F1, F2):**

* `stdio.go`
* `stdio_windows.go` (new, `//go:build windows`)
* `stdio_other.go` (new, `//go:build !windows`)
* `stdio_windows_test.go` (new, `//go:build windows`)
* `transport_extra_test.go` — **read-only except if `gofmt` of a sibling
  edit is required; do not change `TestIsExpectedShutdownErr_Typed`.**

**P2 — Grok CORS (D3; closes F4):**

* `llmprovider/oauth_loopback.go`
* `llmprovider/oauth_loopback_test.go`

**P3 — token error bodies + refresh adopt (D5, D9; closes F6, F18):**

* `llmprovider/oauth_loopback.go`
* `llmprovider/oauth_loopback_test.go`
* `llmprovider/oauth_session.go`
* `llmprovider/oauth_session_test.go`

**P4 — wizard paste-code (D4; closes F5):**

* `wizard/auth.go`
* `wizard/auth_test.go`
* `llmprovider/oauth_loopback_test.go` (InputCode race test only)

**P5 — session validation + import exp (D6, D7; closes F7, F8):**

* `llmprovider/oauth_validate.go` (new) **or** `llmprovider/oauth_session.go`
  if the validator is a few functions and a new file would be empty ceremony
* `llmprovider/oauth_validate_test.go` (new)
* `wizard/auth.go`
* `wizard/auth_test.go`
* `wizard/import.go`
* `wizard/import_test.go`

**P6 — FileTokenStore (D10–D13; closes F9–F12):**

* `llmprovider/tokenstore.go`
* `llmprovider/tokenstore_file.go`
* `llmprovider/tokenstore_file_windows.go`
* `llmprovider/tokenstore_acl_windows.go` (new, optional sibling of the
  chmod helper)
* `llmprovider/tokenstore_test.go`
* `llmprovider/tokenstore_file_windows_test.go` (new, `//go:build windows`)
* `llmprovider/tokenstore_file_unix_test.go` — unix 0600 test stays;
  do not delete it

**P7 — confirm ChatGPT catalog (D8; closes the D11-confirmation half of F3):**

* `llmprovider/discovery_test.go`
* `wizard/configure_test.go` only if a ChatGPT-OAuth listing-failure case
  is missing; do not rewrite `discoverModels` unless a new test proves a
  D11 regression

**P8 — CI (D14–D18; closes F13–F17):**

* `.github/workflows/ci.yml`

P7 `make lint` may touch nothing if clean; if a new exported name from P5
trips goconst/revive, fix the name in that earlier phase's files, not by
disabling a linter.

### Out of scope

* `prepare-commit-msg` (0009 D6 / P8: APPDATA isolation, consumer
  `ValidateOAuth`).
* `paths/` (0002).
* TextPrompter raw-mode CR/LF, HFSC abort-on-log-failure, diagnostic
  `Contains` isolation, self-update junction vs `ModeSymlink`.
* Extracting `restrictToCurrentUser` into a shared package.
* Widening `grokOAuthScopes` with Grok's `conversations:*` /
  `workspaces:*` (MADR open question 7).
* Reverting 0009 D1 dual-stack OpenAI loopback to Codex's IPv4-only bind.
* Changing advertised OpenAI redirect host away from `localhost`, or Grok
  away from `127.0.0.1` ephemeral. Binding OpenAI `:0`.
* Adding `context` to `Prompter`. OS keyring. Claude/Gemini OAuth.
* New `go.mod` require. `git push`. Tags. Live browser login (0009 product
  confirmation). Deleting the operator's live `openai.json`.

## Stability rule

Every **code** phase (P1–P7) ends with these, **redirected to a phase log
in the session scratchpad (outside the repository)**, then branched on the
command's own status — never `cmd | tail && git commit`:

```text
gofmt -w <files this phase staged>
test -z "$(gofmt -l <files this phase staged>)"   # or gofmt -l and assert empty
go vet ./<packages this phase touched>
go test ./<packages this phase touched>
```

Packages by phase: P1 `mcplib` (module root); P2–P3 `./llmprovider`;
P4 `./wizard` and `./llmprovider`; P5 `./llmprovider ./wizard`;
P6 `./llmprovider`; P7 `./llmprovider ./wizard`.

`make lint` runs at **P7** (last Go-source phase) and must be clean there.
If a phase's new names trip goconst, fix in that phase.

P8 ends with a YAML-valid `ci.yml` and `python`/`ruby`/`actionlint` if
present on PATH; if actionlint is absent, skip it and say so in the phase
log. Do not add actionlint as a dependency.

**Red tests first.** A phase that adds a gate: write the test, run the
named test against production code still at the previous phase, capture the
FAIL line in the phase log, then implement, then capture PASS. Do not dirty
production code to create the red run. A search-and-replace that matched
nothing is not a red run.

**Windows-tagged tests** (`stdio_windows_test.go`,
`tokenstore_file_windows_test.go`) execute only when `runtime.GOOS=="windows"`.
`GOOS=windows go test` on Linux builds a Windows binary and does **not**
run it. For P1 and P6 ACL tests:

* If the execution host is Windows native: `go test` as written.
* If the execution host is Linux/WSL: `GOOS=windows go test -c -o <scratch>/x.exe ./<pkg>`
  must exit 0 (compile proof), and the FAIL/PASS of the tagged test must
  be captured from a process whose `runtime.GOOS` is `windows` (native
  shell or `windows-2025`). Do not treat a Linux skip as PASS. Do not use
  Wine.

Each phase ends with **one** `git commit --no-edit` in this repository.
Stage only that phase's files. No `-m` / `--message` / `-F`. No `git push`.
No tag. No `git commit --amend` except the secret-redaction case in the
global rules (does not apply here).

Phase logs live in the session scratchpad, named
`<scratch>/0010-P<n>.log`. Redirect: `cmd > log 2>&1; echo $?`. Read the
log; do not pipe the gate into `tail`.

## Cross-cutting contracts

1. **C1 — ChatGPT catalog is confirmation-only.** D8 forbids rewriting
   `discoverModels` or deleting static catalogs for API-key OpenAI / Gemini /
   Claude / Grok / gateways. Existing
   `TestListAvailableModelsWithSource_ChatGPTListsCodexCatalog` and
   `TestListAvailableModelsWithSource_ChatGPTListingFailureIsError` stay.
   This is the contract most at risk: a "cleanup" of wizard fallbacks would
   look green and violate 0009 D11 / 0010 D8.

2. **C2 — Red tests first.** A phase without a recorded FAIL line for each
   new gate has not executed.

3. **C3 — Hydra and RFC 8252 URIs stay frozen.** OpenAI
   `http://localhost:{1455|1457}/auth/callback`. Grok
   `http://127.0.0.1:{ephemeral}/callback`. Dual-stack OpenAI listen stays
   (0009 D1, Codex still IPv4-only). Do not bind OpenAI `:0`.

4. **C4 — Device-code tests still pass.** `TestGrokDevice_*`,
   `TestOpenAIDevice_*`, wizard device subtest. `InputCode` may be non-nil;
   `LoginDeviceOAuth` must not require it.

5. **C5 — 0008 host-lock tests are not weakened.** ChatGPT generate/listing
   must not hit `api.openai.com`. Grok must not hit `cli-chat-proxy`.

6. **C6 — `Prompter` is unchanged.** No `context` on `Input`. Cancellation
   of paste is best-effort via existing `inputCancel`.

7. **C7 — No live secrets in tests or logs.** JWT-shaped fixtures only.
   Token error bodies go through `logging.RedactString`.

8. **C8 — No new `go.mod` require.** `golang.org/x/sys` is already there.
   `llmprovider` does not import `selfupdate`. Duplicate the ACL recipe.

9. **C9 — Do not delete `TestOAuthSession_RefreshPersistsBeforeReturn`.**
   D9 changes adopt-on-save-failure. Keep the assertion that `Save` was
   called with the new tokens; change the in-memory Access/Refresh
   expectation to the adopted values. Deleting the test to make D9 green
   is a violation.

10. **C10 — `TestIsExpectedShutdownErr_Typed` stays.** It remains the
    POSIX-constant test. The Windows-tagged test is additive.

11. **C11 — Wildcard CORS origin is forbidden.** Exact
    `https://accounts.x.ai`. Not `*` and not `auth.x.ai`.

The contract most likely to be dropped under time pressure is **C1**
(rewriting wizard discovery while "just adding a test") and **C9**
(deleting the persist-before-return test because it "contradicts D9").
Both would look like cleanup.

## Dependency and delivery order

```text
P0 docs commit (this pair only)
 → P1 shutdown classifier + Windows FAIL-then-PASS (D1, D2)
 → P2 Grok CORS (D3)                          [oauth_loopback.go]
 → P3 error bodies + refresh adopt (D5, D9)   [oauth_loopback.go, oauth_session.go]
 → P4 wizard paste-code (D4)                  [wizard/auth.go; loopback test]
 → P5 ValidateOAuthSession + import exp (D6, D7)
 → P6 FileTokenStore Sync / reserved / ACL (D10–D13)
 → P7 catalog confirmation tests (D8)
 → P8 CI (D14–D18)
```

P2 before P3 because both edit `oauth_loopback.go`. P4 after P2 so CORS
tests exist before paste races loopback. P5 after P4 because both edit
`wizard/auth.go`. P6 is independent of P2–P5 once P0 is done; it still
commits after P5 to keep the log linear. P7 is tests-only on catalog.
P8 last so CI proves the tests P1–P7 added.

P1 is independent and goes first so a Windows pipe-error FAIL is on disk
before later phases lengthen the suite.

## Implementation Steps

### P0 — Commit the proposed pair (docs only)

Stage only the two 0010 files. `git commit --no-edit`. No Go, no CI.

If they are already committed, skip and record that in the execution log.

### P1 — Windows shutdown classifier (D1, D2; closes F1, F2)

**Files:** `stdio.go`, new `stdio_windows.go`, new `stdio_other.go`, new
`stdio_windows_test.go`. Do not edit `TestIsExpectedShutdownErr_Typed`.

#### Red tests (write first, capture FAIL on Windows GOOS)

`stdio_windows_test.go`:

```go
//go:build windows

func TestIsExpectedShutdownErr_Windows(t *testing.T) {
    // PathError wrapping ERROR_BROKEN_PIPE
    // bare ERROR_NO_DATA
    // net.OpError wrapping WSAECONNRESET
    // bare WSAECONNABORTED
    // negative: windows.ERROR_INVALID_FUNCTION (Error() has none of the
    // existing fallback phrases)
}
```

Use `golang.org/x/sys/windows` constants. Construct:

```go
&os.PathError{Op: "write", Path: "stdout", Err: windows.ERROR_BROKEN_PIPE}
windows.ERROR_NO_DATA
&net.OpError{Op: "write", Net: "tcp", Err: windows.WSAECONNRESET}
windows.WSAECONNABORTED
windows.ERROR_INVALID_FUNCTION  // want false
```

Run, with `runtime.GOOS == "windows"`:

```text
go test . -count=1 -run 'TestIsExpectedShutdownErr_Windows'
```

Against HEAD this **FAIL**s: `ERROR_BROKEN_PIPE classified false` (or the
test's own `t.Error` text). Capture that line in `0010-P1.log`.

On Linux/WSL: `GOOS=windows go test -c -o <scratch>/mcplib.test.exe .`
must exit 0. Then run the exe on Windows. A Linux `go test` skip is not
the red run.

#### Implementation (only after the FAIL line)

* `stdio_other.go`: `func expectedOSShutdown(error) bool { return false }`
* `stdio_windows.go`: `errors.Is` against `windows.ERROR_BROKEN_PIPE`,
  `windows.ERROR_NO_DATA`, `windows.WSAECONNRESET`,
  `windows.WSAECONNABORTED`.
* `stdio.go` `IsExpectedShutdownErr`: `|| expectedOSShutdown(err)` in the
  typed block; add phrases `"pipe has been ended"` and
  `"pipe is being closed"` to the existing substring list.
* Keep `syscall.EPIPE` / `syscall.ECONNRESET` and existing phrases.

#### Verification

```text
go test . -count=1 -run 'TestIsExpectedShutdownErr_'
go vet .
go test ./...
```

On Windows the new test PASSes. On Linux it is skipped (build tag) and
`TestIsExpectedShutdownErr_Typed` PASSes. Commit.

### P2 — Grok CORS and private-network preflight (D3; closes F4)

**Files:** `llmprovider/oauth_loopback.go`, `llmprovider/oauth_loopback_test.go`.

Vendor pin: grok-build `4247f661` `oidc/login.rs:112–119`, `config.rs:124–147`.

#### Red tests

1. `TestOAuthCallback_GrokOptionsReturnsPrivateNetworkCORS` — handler for
   Grok `/callback`. Request:

   ```text
   OPTIONS /callback
   Origin: https://accounts.x.ai
   Access-Control-Request-Method: GET
   Access-Control-Request-Private-Network: true
   ```

   Expect 204 (or 200) and **exact** headers:
   * `Access-Control-Allow-Origin: https://accounts.x.ai`
   * `Access-Control-Allow-Private-Network: true`
   * `Access-Control-Allow-Methods` contains `GET`

   Result channel must be empty (no send). Against HEAD: OPTIONS hits the
   GET logic, HTTP 400, `state mismatch` on the channel.

2. `TestOAuthCallback_GrokGETIncludesCORS` — GET
   `/callback?code=secret&state=expected` with
   `Origin: https://accounts.x.ai` → 200 HTML, same three headers, code
   on the channel.

3. `TestOAuthCallback_GrokDoesNotWildcardOrigin` — OPTIONS with
   `Origin: https://evil.example` → no `Access-Control-Allow-Origin` (not
   `*`, not `evil.example`). Channel empty.

4. `TestOAuthCallback_OpenAIHasNoGrokCORS` — OpenAI `/auth/callback`
   OPTIONS does not set `Access-Control-Allow-Origin` to
   `https://accounts.x.ai`.

Capture FAIL of (1) against HEAD.

#### Implementation

* Constant `grokAccountsAppOrigin = "https://accounts.x.ai"`.
* `oauthCallbackHandler(path, state string, result chan<- oauthCallbackResult, corsOrigin string)`.
  Grok `browserListener` path uses `grokAccountsAppOrigin`; OpenAI uses `""`.
* Register method-aware patterns or an OPTIONS branch **before** state/code
  inspection. OPTIONS never sends on `result`.
* On matching `Origin` (byte-for-byte, no trailing slash), set the three
  headers on OPTIONS and on GET success and error responses.
* No `*`. No new module.

`LoginBrowserOAuth_GrokCompletesCallbackAndExchange` must still pass
(regression: Grok URI stays `http://127.0.0.1:{port}/callback`).

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestOAuthCallback_|TestLoginBrowserOAuth_Grok|TestListenLoopbackBothFamilies_|TestOpenAILoopbackPorts'
go test ./llmprovider ./wizard
```

Commit.

### P3 — Token error bodies and refresh adopt (D5, D9; closes F6, F18)

**Files:** `llmprovider/oauth_loopback.go`, `oauth_loopback_test.go`,
`oauth_session.go`, `oauth_session_test.go`.

Vendor pin: grok-build `oidc/protocol.rs:35–38`.

#### Red tests

1. `TestExchangeOAuthCode_IncludesRedactedBody` — token endpoint 400 body
   `{"error":"invalid_grant","error_description":"eyJhbGciOiJub25lIn0.aaa.bbb"}`.
   Error contains `400`, contains `[REDACTED]` or `invalid_grant`, does
   **not** contain `eyJ`. Against HEAD: status-only, no body.

2. `TestRefreshOAuthSession_IncludesRedactedBody` — same for refresh 400.

3. `TestOAuthTokenError_CapsBody` — 4096 `A` bytes; the body portion of
   the error string is ≤ 2048 plus any redaction/truncation marker.

4. `TestOAuthSession_SaveFailureAdopts` — httptest refresh returns
   `new-access` / `new-refresh`; `Save` returns a sentinel. First
   `Token()` errors with that sentinel **and** `session.Access == "new-access"`.
   Second `Token()` (no new HTTP — close the server or count hits) returns
   `new-access` with nil error and HTTP hit count still 1.
   Against HEAD: `TestOAuthSession_RefreshPersistsBeforeReturn` currently
   asserts Access stays `old-access`. The new test FAILS. Do **not**
   delete that existing test (C9): after implement, update it so Save is
   still observed with new tokens, and Access/Refresh are the adopted
   values.

#### Implementation

```go
func oauthHTTPStatusError(op string, resp *http.Response) error
```

`io.ReadAll(io.LimitReader(resp.Body, 2048))`, close body,
`logging.RedactString`, then
`fmt.Errorf("oauth: %s failed: %s: %s", op, resp.Status, redacted)`.
Use from `exchangeOAuthCode` (non-2xx) and `refreshOAuthSession` (non-200).
Do not `slog` the raw body.

`OAuthSession.Token`: on successful refresh, `adopt(next)` **then**
`Save`. If `Save` fails, return `(token, saveErr)` with memory already
adopted. `slog.Error` the persist failure. Do not spend the token on the
failing call (return the error).

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestExchangeOAuthCode_|TestRefreshOAuthSession_|TestOAuthTokenError_|TestOAuthSession_'
go test ./llmprovider ./wizard
```

Commit.

### P4 — Wizard paste-code (D4; closes F5)

**Files:** `wizard/auth.go`, `wizard/auth_test.go`,
`llmprovider/oauth_loopback_test.go`.

Vendor pin: grok-build `oidc/login.rs:431` prompt string.

#### Red tests

1. In `TestConfigureLLM_BrowserAndDevicePersistSessions`, browser stub:
   `if opts.InputCode == nil { t.Fatal("InputCode is nil") }`.
   Against HEAD this FAILs. Device stub: `InputCode` may be non-nil;
   must not be invoked (device path does not call it). Assert a
   `seenNotify` entry contains `paste` (case-insensitive) **and** that
   the authorize URL is notified even when `Options.OpenURL` is a no-op
   (pass a no-op OpenURL on a subtest).

2. `TestLoginBrowserOAuth_InputCodeWinsBeforeLoopback` — `InputCode`
   returns `code=pasted-secret&state=...` immediately; `OpenURL` is a
   no-op that does not HTTP the loopback. Session exchanges
   `pasted-secret`. Use a 2s context. Against a build that ignores
   `InputCode`, this hangs until timeout — fail the test on ctx deadline.

`parseOAuthInput` already exists; add cases only if the matrix is missing
bare code / `error=` URL (Grok `parse_pasted_input_matrix`).

#### Implementation

`oauthFlowOptions`:

* Always set `InputCode` to
  `p.Input("Paste the URL here if it doesn't connect:", "")`
  (exact Grok wording).
* Wrap `OpenURL` so a `Notify` of the authorize URL **and** the paste
  line run even when the consumer supplied `OpenURL`. Then call the
  consumer hook. Failure of `OpenURL` stays non-fatal at
  `LoginBrowserOAuth` (`ignoreOAuthError`).

Do not add `context` to `Prompter`.

#### Verification

```text
go test ./wizard -count=1 -run 'TestConfigureLLM_Browser'
go test ./llmprovider -count=1 -run 'TestLoginBrowserOAuth_InputCode|TestParseOAuthInput_|TestLoginBrowserOAuth_Grok|TestGrokDevice_|TestOpenAIDevice_'
go test ./wizard ./llmprovider
```

Commit.

### P5 — ValidateOAuthSession and import exp (D6, D7; closes F7, F8)

**Files:** new `llmprovider/oauth_validate.go` + `_test.go` (preferred),
`wizard/auth.go`, `wizard/auth_test.go`, `wizard/import.go`,
`wizard/import_test.go`.

#### Red tests

1. `TestValidateOAuthSession_RejectsFixture` table:
   * `{Access: "chatgpt-access", Issuer: DefaultOpenAIIssuer}` → error
   * empty Access → error
   * `{Access: "x", Refresh: ""}` with Grok issuer → error
   * refreshable `{Access, Refresh, ClientID, TokenURL}` → nil
   * ChatGPT access-only: Issuer/ClientID defaults, Refresh `""`, Expiry
     zero, Access `eyJhbGciOiJub25lIn0.e30.x` → nil
   * same access-only with **non-zero** Expiry → error
   Against HEAD the function does not exist (compile fail) or, if written
   first as the gate, the chatgpt-access case is the FAIL once wired to
   `saveOAuthCredential`.

2. `TestSaveOAuthCredential_RejectsFixture` — memory store, session
   Access `chatgpt-access`. `saves==0`, error mentions `chatgpt-access`
   or `stub` or `refresh`.

3. Rewrite `TestConfigureLLM_TokenStdinClassification` `"openai access
   token"` to use the JWT-shaped access-only token. New subtest
   `chatgpt-access fixture` expects configure error, saves 0.

4. `TestImportOpenAIAuth_SetsExpiryFromJWT` — vendor JSON access is
   `eyJhbGciOiJub25lIn0.eyJleHAiOjIwMDAwMDAwMDB9.x` (payload
   `{"exp":2000000000}`). Imported `Expiry` is
   `time.Unix(2000000000, 0).UTC()`. Access without `exp` stays zero and
   still imports if refresh is present.
   Against HEAD: `Expiry` is zero for the exp JWT. FAIL.

Update `testOAuthSession` in `auth_test.go` to set `TokenURL` (D6 requires
it for browser/device saves). Do this in the same phase as the validator
so existing persist tests stay green after D6.

#### Implementation

Exported `func ValidateOAuthSession(*OAuthSession) error`. Rules exactly
MADR D6. `chatgpt-access` is an exact match (do not reject every 14-char
string). `saveOAuthCredential` calls it before `Save`.

`importOpenAIAuth`: if access is a three-segment JWT and payload JSON has
numeric `exp`, set `Expiry` to that unix time UTC. Unparsable `exp` →
leave zero. Still require access and refresh. Grok import untouched.

#### Verification

```text
go test ./llmprovider ./wizard -count=1 -run 'TestValidateOAuthSession_|TestSaveOAuthCredential_|TestConfigureLLM_TokenStdin|TestImportOpenAI|TestConfigureLLM_Browser'
go test ./llmprovider ./wizard
```

Commit.

### P6 — FileTokenStore Sync, reserved names, Windows ACL (D10–D13; closes F9–F12)

**Files:** `tokenstore.go`, `tokenstore_file.go`,
`tokenstore_file_windows.go`, optional `tokenstore_acl_windows.go`,
`tokenstore_test.go`, new `tokenstore_file_windows_test.go`. Unix 0600
test file stays.

Vendor pin: grok-build `secure_file.rs:86–172`; in-tree
`selfupdate/cleanup_windows.go:122–170`. Copy the recipe; do not import
`selfupdate`.

#### Red tests

1. `TestFileTokenStore_OverwriteExisting` — Save A, Save B, Load == B.
   Against HEAD this likely **PASSes** (os.Rename replaces). Still commit
   it; it was the 0009 throwaway that never landed. A pass on HEAD is
   allowed for this one case because it locks existing behaviour; say so
   in the phase log.

2. `TestFileTokenStore_RejectsReservedNames` — `CON`, `con`, `NUL`,
   `COM1`, `CON.json`, `COM1.txt`, `aux`, trailing-dot `foo.`, trailing
   space `foo `. All `errors.Is(..., ErrInvalidProvider)`. Against HEAD
   `Save("CON")` returns nil. FAIL.

3. `TestFileTokenStore_SaveSyncs` — optional if hard to observe: at
   least call Save and Load round-trip after adding `Sync`; if a fake
   is too heavy, skip a dedicated Sync unit test and keep the `Sync`
   error-path by injecting nothing — the call is in production code and
   `go test` exercises Save. Prefer a thin wrap only if it stays in
   `tokenstore_file.go` without a new global. Do not skip Sync in
   production to make tests easy.

4. Windows-tagged `TestFileTokenStore_RestrictsToCurrentUser` — after
   Save, `GetNamedSecurityInfo` on the file: owner SID is the current
   user **or** (if matching Grok, which does not SetOwner) an explicit
   `GENERIC_ALL` ACE for the current SID and the DACL is protected
   (`PROTECTED_DACL_SECURITY_INFORMATION`). Assert the recipe's
   invariants, not "Administrators is absent on a runner image".
   Against HEAD chmod is a no-op: FAIL if we assert a protected DACL
   bit / explicit ACE that default inheritance does not set.

Red run (2) and (4) against HEAD. (1) may already pass.

#### Implementation

* `validateProviderID`: after existing checks, case-insensitive match
  against `CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`;
  also if `filepath.Ext` stripped base matches that set (`CON.json`);
  reject trailing space and trailing `.` on the id. Same on every GOOS.
* `Save`: after successful write, `tmp.Sync()`, then `Close`, then
  `Rename`, then `chmod0600`. Sync error → failed Save, temp cleaned up.
* Windows `chmod0600`: `restrictToCurrentUser` copy (current-user
  `GENERIC_ALL`, `PROTECTED_DACL`, owner = current SID as in
  `selfupdate` — keep SetOwner; that is the in-tree recipe).
* `NewFileTokenStore`: after `MkdirAll`, call the same helper on `Dir`
  (Windows). Unix directory mode stays `0o700`.

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestFileTokenStore_'
go test ./llmprovider
```

Windows host additionally:

```text
go test ./llmprovider -count=1 -run 'TestFileTokenStore_RestrictsToCurrentUser'
```

Unix 0600 test still passes on unix. Commit.

### P7 — Confirm ChatGPT catalog (D8; closes F3 catalog half)

**Files:** `llmprovider/discovery_test.go`; `wizard/configure_test.go`
only if needed.

Do **not** rewrite `discoverModels` or `listChatGPTModels`.

#### Tests (add if missing; existing ones count)

Already at HEAD and must keep passing:

* `TestListAvailableModelsWithSource_ChatGPTListsCodexCatalog` —
  path `/models`, `client_version=0.0.0`, `originator=mcplib`,
  `ChatGPT-Account-Id`, hidden slugs dropped.
* `TestListAvailableModelsWithSource_ChatGPTListingFailureIsError` —
  502 → error, nil slice.

Add:

1. `TestListChatGPTModels_DefaultHostIsCodexNotPlatform` — custom
   `http.RoundTripper` that records `req.URL` and returns an error
   without dialing. `ListAvailableModelsWithSource` with a ChatGPT
   session, **no** `WithBaseURL`. Captured URL host is
   `chatgpt.com`, path contains `/backend-api/codex/models`, query has
   `client_version=0.0.0`, host is not `api.openai.com`.
   If this already exists, cite it and do not duplicate.

2. Wizard: ChatGPT OAuth + `Discover: true` + listing 502 → Select
   choices do not include `gpt-4.1-mini` or `gpt-5.4`; user can enter a
   model id (`Other` / Input). Reuse
   `TestConfigureLLM_EmptyDiscoveryPromptsForModel` shape with OpenAI
   OAuth session and TokenStore. Against HEAD this should already pass
   (`discoverModels` returns nil). If it passes on the first run, record
   PASS as confirmation, not as a missing red (D8 is confirm-only).

3. `grep` / compile: `StaticOpenAIChatGPT` has zero Go references.

#### Verification

```text
go test ./llmprovider -count=1 -run 'TestListAvailableModelsWithSource_ChatGPT|TestListChatGPTModels_'
go test ./wizard -count=1 -run 'TestConfigureLLM_EmptyDiscovery|TestConfigureLLM_ChatGPT'
go test ./llmprovider ./wizard
make lint
```

`make lint` must be 0 issues. Commit.

### P8 — CI parity (D14–D18; closes F13–F17)

**Files:** `.github/workflows/ci.yml` only.

Keep existing pins:
`actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1`,
`actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`.
Keep `fail-fast: false`. Keep `go-version-file: go.mod`.

#### Resulting workflow (normative)

`validate` job, same OS matrix:

1. checkout, setup-go, `go mod download` (unchanged)
2. `go test ./...` (unchanged, every OS)
3. **`go vet ./...` every OS** — move vet out of the Linux-only block.
   Leave `gofmt`, `go mod tidy -diff`, `golangci-lint` Linux-only.
4. **`go test -race ./...` every OS**, its own step so a race is not
   confused with a plain failure.
   Windows: if the first run fails with CGO/gcc missing, add a preceding
   step `choco install mingw -y --no-progress` and put mingw `bin` on
   `PATH`. Record that in the phase log. Do not drop Windows from `-race`
   without amending the MADR (D15).
5. Linux-only: gofmt, tidy-diff, golangci-lint v2.13.1, 
   `verify-selfupdate-release_test.sh` (unchanged).
6. Release-guard step: add `shell: bash`. Same two scripts. Runs on all
   three OS.

New job `vuln`, `runs-on: ubuntu-24.04`:

```yaml
- uses: actions/checkout@<same SHA>
- uses: actions/setup-go@<same SHA>
  with: { go-version-file: go.mod }
- run: go install golang.org/x/vuln/cmd/govulncheck@v1.7.0
- run: "$(go env GOPATH)/bin/govulncheck" ./...
```

Pin `v1.7.0` (MADR 0006). If that version refuses to run on Go 1.26.6,
stop, record the installer error, and amend the PLAN with the version
that runs; do not silently `@latest`.

New job `cross-compile`, `runs-on: ubuntu-24.04`, matrix:

```yaml
goos: [linux, darwin, windows]
goarch: [amd64, arm64]
```

Steps: checkout, setup-go, then

```text
GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} go build ./...
```

Additional step on that job (not in the GOOS matrix, or a second job
`cross-test-compile`):

```text
GOOS=windows GOARCH=amd64 go test -c -o $RUNNER_TEMP/selfupdate-windows-amd64.test.exe ./selfupdate
GOOS=windows GOARCH=amd64 go test -c -o $RUNNER_TEMP/llmprovider-windows-amd64.test.exe ./llmprovider
GOOS=windows GOARCH=arm64 go test -c -o $RUNNER_TEMP/selfupdate-windows-arm64.test.exe ./selfupdate
GOOS=windows GOARCH=arm64 go test -c -o $RUNNER_TEMP/llmprovider-windows-arm64.test.exe ./llmprovider
```

Do not upload those binaries as artifacts. Discard with the runner temp.

Do **not** set workflow-level `defaults.run.shell: bash`.

#### Verification

Inspect the staged YAML:

* `go vet ./...` has no `if: runner.os == 'Linux'` (or an equivalent
  step exists on every matrix OS).
* A step `go test -race ./...` exists on the matrix.
* A `vuln` job runs `govulncheck` with a version pin.
* Release-guard step has `shell: bash`.
* Cross-compile matrix is the six GOOS/GOARCH pairs plus Windows
  `go test -c` for the two packages.

`python` YAML parse if available; otherwise visual check of indentation.
Commit.

## Verification (whole plan)

After P8, on the execution host:

```text
gofmt -l stdio.go stdio_windows.go stdio_other.go stdio_windows_test.go \
  llmprovider/*.go wizard/*.go
go vet ./...
go test ./...
```

Linux additionally: `make lint`, `go mod tidy -diff`.

Windows additionally:

```text
go test . -count=1 -run 'TestIsExpectedShutdownErr_'
go test ./llmprovider -count=1 -run 'TestFileTokenStore_'
```

0008 host-lock tests in `./llmprovider` still pass (ChatGPT host, Grok
host). `StaticOpenAIChatGPT` still does not compile.

### Acceptance criteria (mapped to MADR Confirmation)

| # | Criterion | MADR |
| --- | --- | --- |
| A1 | Windows-tagged `TestIsExpectedShutdownErr_Windows` FAIL captured against HEAD, then PASS after P1. Typed POSIX test still passes. | D1, D2, Confirmation shutdown block |
| A2 | Grok OPTIONS from `accounts.x.ai` returns CORS/PNA headers and does not complete the waiter. Evil origin has no Allow-Origin. OpenAI OPTIONS has no Grok origin. | D3 |
| A3 | Wizard browser path sets `InputCode`; notify contains Grok paste sentence; InputCode-before-loopback test PASSes in 2s. Device tests still pass. | D4 |
| A4 | Exchange/refresh 400 with JWT-shaped body → error has status, `[REDACTED]`, no `eyJ`. 4096-byte body capped. | D5 |
| A5 | `chatgpt-access` rejected by `ValidateOAuthSession` and `saveOAuthCredential` (saves 0). JWT access-only stdin path still saves. | D6 |
| A6 | OpenAI import of JWT with `exp` 2000000000 sets that UTC expiry. | D7 |
| A7 | ChatGPT listing capturing transport: originator `mcplib`, Codex `/models?client_version=0.0.0`, host ≠ `api.openai.com`. 502 → error + nil. `StaticOpenAIChatGPT` absent. | D8 |
| A8 | Save-failure after refresh adopts new tokens; this `Token()` errors; next `Token()` does not hit HTTP. Existing persist test still asserts Save saw new tokens. | D9 |
| A9 | Overwrite test exists. Reserved names rejected. Windows DACL test PASSes on Windows GOOS. Unix 0600 test still unix-tagged and green. Save calls `Sync`. | D10–D13 |
| A10 | CI: vet every OS; `-race` every OS; govulncheck pinned Linux job; `shell: bash` on release-guard; 6-way `go build` plus Windows `go test -c`. | D14–D18 |
| A11 | `go test ./...` and `go vet ./...` exit 0. `make lint` clean after P7. No new `go.mod` require. 0008 host locks intact. | Confirmation whole-tree |

The criterion most likely to be quietly dropped is **A1's recorded FAIL
line** (shipping the Windows test only after the classifier, so it never
fails) and **A10's Windows `-race`** (leaving race Linux-only after a CGO
scare). Both are MADR D2 / D15. A phase log without the FAIL, or a YAML
that races only on Linux, is an incomplete phase.

## Rollout and Rollback

Rollout is merge of the eight phase commits (P0–P8) to `main` via the
normal PR path. This PLAN does not `git push` and does not tag. A
consumer bump (prepare-commit-msg 0009 P8) is a later record.

Rollback is `git revert` of the phase commit that misbehaved. P1–P7 are
library-compatible (additive classifier, additive CORS headers, stricter
Save validation). P5 can break a consumer that saved `chatgpt-access` on
purpose in tests — that is the point; those tests belong in 0009 P8.
P8 rollback is YAML-only.

## Deferred (named, so they are not mistaken for oversights)

* **0009 P8 / D6 consumer isolation** — lives in `prepare-commit-msg`
  (`APPDATA`, `ValidateOAuth`). This PLAN cannot close store pollution
  in a sibling module.
* **0002 `paths` package** — accepted, unimplemented; fleet adoption.
* **Grok extra OAuth2 scopes** (`conversations:*`, `workspaces:*`) —
  MADR open question 7; 0008 froze `grokOAuthScopes`.
* **Shared ACL helper** for `selfupdate` + `llmprovider` — two copies
  are the cost of not growing a package in this record.
* **TextPrompter CR/LF leftover, HFSC missing abort, diagnostic
  substring leak, self-update junction vs `ModeSymlink`** — adjacent
  Windows/hardening bugs from the 2026-09-20 pass; new numbers if
  pursued.
* **Live OpenAI/Grok browser login on the Windows laptop** — 0009
  product confirmation, not a unit test in this PLAN.
* **`git push` and tags** — explicit ask in the same turn, never this
  PLAN.
* **Windows `-race` wall-clock** — if `windows-2025` cannot finish
  `-race` inside a reasonable job budget, stop and amend D15; do not
  silently drop Windows from the race matrix.
