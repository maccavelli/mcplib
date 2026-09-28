---
status: in-progress
date: 2026-09-28
associated-madr: "0012-MADR-conform-providers-to-reference-clients.md"
decision-makers: mcplib maintainers
---

# Implement 0012 §5 — OAuth Session Hygiene

Associated MADR: [0012-MADR-conform-providers-to-reference-clients.md](0012-MADR-conform-providers-to-reference-clients.md)
(accepted 2026-09-27, revision 3). This is the fifth of that MADR's six plans
(§8).

This plan executes MADR §5 as amended in revision 3, and nothing else. If a
fact contradicts the MADR or this plan, **stop and prompt**. Add a dated
entry to §9 of this plan, amend the MADR when a decision or an asserted fact
changes, and only then continue.

O2 omits §5.2's `principal_type` and `principal_id`, which the owner
withdrew on 2026-09-27 (MADR revision 3). Phase O6 is gated on one owner-run
live ChatGPT browser login (MADR revision 2 decision).

## How this plan was proven

Every phase below was executed on 2026-09-27, in scratch copies of
`git archive 7ea0ad4` with the §2–§4 plans applied:
* **Red.** Each phase's tests diff was applied, and each named test was seen
  to fail on the unfixed code.
* **Green.** The fix diff was applied, and the full gate (§0.2) passed.
* **Mutants.** A guard that holds today, or a test of new API, was seen to
  fail on a named mutant.
* **Live.** Both vendor CLI logins on the maintainer's machine were used
  read-through. Their files were read, never written, and no refresh was
  sent. Nothing was revoked.
* **Diffs.** Appendix B's diffs were generated mechanically from that proof.
  Applying all five plans' diffs in §8's order to a fresh `7ea0ad4` archive
  reproduces the proven tree byte for byte: 308 files, 0 mismatches.

The reference facts cite `codex` `25270df261` and `grok-build` `f0e3be11`.

## Goal

* **No shared refresh tokens.** Using a vendor CLI's login copies no token.
  `mcplib` reads the CLI's file on every request and never refreshes it. The
  CLI keeps its refresh token, and both vendors revoke a refresh-token
  family on reuse (§5.1).
* **Refreshes that cannot trip reuse detection.** `mcplib`'s own sessions:
  * reload the store before refreshing;
  * use Codex's JSON body on OpenAI;
  * fail terminally on a dead refresh token;
  * retry transient failures;
  * expire from the JWT;
  * refresh five minutes early (§5.2).
* **A safer login (§5.3):**
  * Codex's `life_sciences` state suffix is accepted;
  * the IdP's description, and Codex's entitlement message, reach the user;
  * a malicious Grok device-code response is refused before the user sees
    it;
  * `RevokeOAuthSession` ends a session at its issuer;
  * OpenAI redirects to `127.0.0.1` (gated).
* **`CODEX_ACCESS_TOKEN`** is no longer offered as a ChatGPT bearer (§5.4).

## Scope

**In scope:** MADR §5.1–§5.4 as amended: O1–O6 below.

**Out of scope:**
* §5.2's principal fields, withdrawn by the owner (MADR revision 3).
* Codex personal access tokens (§5.4).
* The OAuth MADR's dual-stack `localhost` listener, which stays.

## 0. Preconditions and conventions

### 0.1 Baseline

* `0012-PLAN-chatgpt-backend.md` complete on `main`. C3's FedRAMP plumbing
  is part of the session state this plan changes. On any other base the
  diffs need rebasing, which is a §9 deviation.
* `go test -count=1 ./...` passes before O1.

### 0.2 Gate (every phase)

1. `gofmt -l` prints nothing, and `golint -set_exit_status` passes, on each
   `.go` file the phase touched.
2. `go vet ./...` and `go vet -tags live_gateways ./llmprovider`.
3. `make lint`.
4. `go test -count=1 ./...` and `go test -race -count=1 ./llmprovider ./wizard`.

### 0.3 Red first

As in `0012-PLAN-item-fidelity.md` §0.3.

### 0.4 Credentials for live steps

| Test | Needs |
|---|---|
| `TestLive_VendorCLISession/openai` | `MCPLIB_LIVE_CHATGPT=1` and a Codex CLI login |
| `TestLive_VendorCLISession/grok` | `MCPLIB_LIVE_GROK_CLI=1` and a Grok CLI login |
| `TestLive_GrokDiscoveryPublishesRevocation` | nothing; it only reads a public document |
| `TestLive_ChatGPTBrowserLogin` (O6) | `MCPLIB_LIVE_BROWSER_LOGIN=1` and a person at a browser |

The vendor tests skip when the CLI's token has expired.

### 0.5 Commits

One `git commit --no-edit` per phase, after the gate. No push and no tag.

## Phase O0 — Start

1. Confirm §0.1.
2. Set this plan to `status: in-progress`.
3. Commit the documents only.

## Phase O1 — Vendor CLI logins are read-through (§5.1)

**Today.**
* **Refresh token copied.** `wizard/import.go` copies the CLI's refresh
  token into `mcplib`'s store and refreshes it independently. The first
  refresh by either side revokes the other's family.
* **Wrong scope.** Grok's entry is picked from a Go map, so iteration order
  decides which one.
* **Ignored path.** `GROK_AUTH_PATH` is ignored.

**Changes** (Appendix B.O1):
* **`llmprovider.VendorCLISession{Provider, Path}`**, a new `TokenSource`:
  * `Token` re-reads the file each call, opened within its directory
    (`os.OpenRoot`);
  * Codex: `tokens.access_token`, with expiry from its JWT `exp`, plus
    `account_id` and the id token's FedRAMP claim;
  * Grok: exactly the `"{issuer}::{client_id}"` entry for the default
    login, with expiry from `expires_at`, else the JWT;
  * an expired or missing token is `ErrAuthFailure` that tells the user to
    run the CLI.
* **ChatGPT mode.** An OpenAI provider on a Codex `VendorCLISession` is in
  ChatGPT mode, with the file's account id and FedRAMP flag.
* **Wizard:**
  * the import method becomes `CredVendorCLI`, and `Result.VendorAuthPath`
    names the file;
  * no token is copied or saved, and no `TokenStore` is needed;
  * path resolution follows each CLI: `$CODEX_HOME`, else `~/.codex`; for
    Grok, `$GROK_AUTH_PATH`, else `$GROK_HOME`, else `~/.grok`;
  * the menu labels become "Use the … CLI login".
* **Test updates.** The import tests of the removed functions are replaced,
  and the label and TokenStore-requirement tests are updated.

**Verification.**
* Red: two wizard tests show a copied token and a saved session, and an
  ignored `GROK_AUTH_PATH`.
* Mutants prove `VendorCLISession`'s rules:
  * re-read;
  * expiry;
  * the exact Grok scope (50 iterations defeat map order);
  * ChatGPT mode.
* Live, both CLI logins generate read-through.

## Phase O2 — Refresh of `mcplib`-owned sessions (§5.2)

**Today.**
* **Stale token resent.** `OAuthSession.Token` refreshes without looking at
  its store, so a token another process rotated is resent.
* **Wrong encoding.** The OpenAI refresh is form-encoded; Codex sends JSON.
* **No failure classes.** A dead refresh token is a plain error, so retries
  resend it, and a transient failure is not retried.
* **Expiry and window.** Expiry ignores the JWT, and the window is two
  minutes, where both references use five.

**Changes** (Appendix B.O2):
* **Store reload.** `reloadOrRefresh` loads the store first. A different
  stored refresh token is adopted. It is refreshed only if its own access
  token is due, and then with the stored token.
* **Request body.** `newRefreshRequest` sends JSON for the OpenAI issuer
  (`login/src/oauth/client.rs:80-110`) and a form otherwise.
* **Failures:**
  * `refreshFailure` wraps `ErrAuthFailure` for a 401 or for
    `invalid_grant`, `invalid_client`, `refresh_token_expired`,
    `refresh_token_reused` or `refresh_token_invalidated`;
  * transport errors, 429 and 5xx are retried: 3 attempts, 200 ms then
    400 ms.
* **Expiry.** `tokenExpiry` uses the JWT `exp`, else `expires_in`, at login
  and at refresh.
* **Window.** `oauthRefreshSkew` is 5 minutes.
* **Test updates:**
  * `TestOAuthSession_SkewsTwoMinutes` becomes `…SkewsFiveMinutes`;
  * the 401-retry test reads the JSON body.

**Verification.**
* Red: seven unit tests.
* Guards proven by mutants:
  * Grok stays form-encoded;
  * an unchanged store still refreshes;
  * a plain 400 is neither retried nor terminal.
* **No live run.** A live refresh needs an `mcplib`-owned session, which
  only a browser login creates. No CLI login's token is ever refreshed.

## Phase O3 — Callback and device-code hardening (§5.3)

**Today.**
* **Suffixed state.** A callback whose state carries Codex's
  `.onboarding_entrypoint=life_sciences` suffix is a state mismatch.
* **Lost description.** An authorize error reports only its code.
* **Unvalidated device codes.** A Grok device-code response is shown to the
  user unvalidated.

**Changes** (Appendix B.O3):
* **State.** `callbackStateMatches` accepts the expected state, or it plus
  exactly that suffix (`server.rs:366-374`).
* **Errors:**
  * `callbackError` carries `error_description`, stripped of control
    characters and bounded to 300 bytes;
  * Codex's entitlement case (`access_denied` with
    `missing_codex_entitlement`) gets Codex's explanation
    (`server.rs:934-956`);
  * state is checked before the error in both the loopback and a pasted
    URL.
* **Device codes.** `validateGrokDeviceCode` requires a `user_code` of ASCII
  letters, digits and `-`, and `https` verification URIs (or `http` to
  `localhost` or `127.0.0.1`) with no control characters
  (`device_code.rs:148-155`, `:474-487`).

**Verification.**
* Red: four unit tests.
* Guards proven by mutants:
  * only the exact suffix passes;
  * `https`, loopback `http` and lowercase codes still pass.

## Phase O4 — `RevokeOAuthSession` (§5.3)

**Today.** `mcplib` cannot revoke a session.

**Changes** (Appendix B.O4):
* **`RevokeOAuthSession(ctx, *OAuthSession) error`** revokes the refresh
  token, else the access token. It never touches a `TokenStore`.
* **OpenAI:** Codex's JSON body `{token, token_type_hint, client_id}` at the
  token URL's origin with path `/oauth/revoke`. `client_id` is sent only
  for a refresh token (`revoke.rs:31-53`, `:134-150`).
* **Grok:** an RFC 7009 form to the discovery document's
  `revocation_endpoint`. Without one, the error wraps `errors.ErrUnsupported`.
* **Discovery.** `oauthEndpoints` decodes `revocation_endpoint`.
* The doc comment forbids passing a vendor CLI's session.

**Verification.**
* Nothing is red, because the function is new. Four mutants prove it.
* Live, `TestLive_GrokDiscoveryPublishesRevocation` reads xAI's document:
  `https://auth.x.ai/oauth2/revoke`. **No real session was revoked.**

## Phase O5 — Withdraw `CODEX_ACCESS_TOKEN` (§5.4)

**Today.** `token_stdin` offers `CODEX_ACCESS_TOKEN` as a ChatGPT bearer. In
Codex it is a personal access token or an agent-identity JWT
(`login/src/auth/access_token.rs:1-14`).

**Changes** (Appendix B.O5):
* The environment branch is removed; pasting a token remains.
* `ValidateOAuthSession`'s comment no longer names the variable.
* `TestConfigureLLM_TokenStdinUsesCodexEnvironment` is removed with the
  behaviour it tested.

**Verification.**
* Red: `TestConfigureLLM_IgnoresCodexAccessTokenEnv`.
* The pasted-token guard is proven by a mutant.

## Phase O6 — The `127.0.0.1` redirect (§5.3), gated

**Gate.** Run this phase's code only after the owner has run
`TestLive_ChatGPTBrowserLogin` from this phase's tree, and it has passed:

```sh
MCPLIB_LIVE_BROWSER_LOGIN=1 go test -count=1 -tags live_gateways \
  -run '^TestLive_ChatGPTBrowserLogin$' -v ./llmprovider
```

The test:
1. prints the authorize URL;
2. waits up to five minutes for the ChatGPT login to redirect to
   `127.0.0.1`;
3. generates once;
4. revokes the new session with `RevokeOAuthSession`.

Record its output in §10. If it fails, stop and prompt: the redirect stays
`localhost`.

**Changes** (Appendix B.O6):
* `browserListener` returns `http://127.0.0.1:{port}/auth/callback` for
  OpenAI, as Codex does (`server.rs:193`). The dual-stack listener stays.
* The exchange test's `redirect_uri` assertion follows.

**Verification.**
* Red: `TestBrowserListener_OpenAIRedirectsToLoopbackIP`.
* The Grok redirect guard is proven by a mutant.
* **Not run.** The owner's browser login is the live proof, and it has not
  run.

## Phase O7 — Records and close-out

1. Record each phase's result in §10.
2. Set this plan to `status: complete` once §7 holds. Until O6's gate runs,
   the plan is `in-progress`.
3. Commit the documents only.

## Phase O8 — Windows path expectations in `TestVendorAuthPath` (amendment, 2026-09-28)

Added after execution; see §9, 2026-09-28.

**Found.** CI's `validate (windows-2025)` job has failed on every push since
O1 reached `origin` (run `36356998336` on `a3a04af`, through run
`36429177833` on `a27da70`). The macOS and Linux jobs pass:

```text
--- FAIL: TestVendorAuthPath/codex_home
    import_test.go:32: vendorAuthPath = "\\x\\codex\\auth.json", <nil>; want "/x/codex/auth.json"
--- FAIL: TestVendorAuthPath/grok_home
    import_test.go:32: vendorAuthPath = "\\x\\grok\\auth.json", <nil>; want "/x/grok/auth.json"
```

**Cause: the test, not the code.**
* `vendorAuthPath` (`wizard/import.go`) joins `$CODEX_HOME` or `$GROK_HOME`
  with `auth.json` using `filepath.Join`. On Windows that gives
  `\x\codex\auth.json`, which is what each CLI resolves there, so it is
  correct, and MADR §5.1 is unaffected.
* O1's test hard-coded the Unix form for those two cases. It could pass only
  on macOS and Linux. The default-home cases already used `filepath.Join`,
  and they pass on Windows.
* The §0.2 gate ran only on macOS, so no phase exercised Windows.

**Change** (test only; no library code changes):

```diff
diff --git a/wizard/import_test.go b/wizard/import_test.go
--- a/wizard/import_test.go
+++ b/wizard/import_test.go
@@ -19,11 +19,11 @@ func TestVendorAuthPath(t *testing.T) {
 		env            map[string]string
 		want           string
 	}{
-		{"codex home", llmprovider.ProviderOpenAI, map[string]string{"CODEX_HOME": "/x/codex"}, "/x/codex/auth.json"},
+		{"codex home", llmprovider.ProviderOpenAI, map[string]string{"CODEX_HOME": "/x/codex"}, filepath.Join("/x/codex", "auth.json")},
 		{"codex default", llmprovider.ProviderOpenAI, nil, filepath.Join(home, ".codex", "auth.json")},
 		{"grok auth path", llmprovider.ProviderGrok,
 			map[string]string{"GROK_AUTH_PATH": "/y/login.json", "GROK_HOME": "/x/grok"}, "/y/login.json"},
-		{"grok home", llmprovider.ProviderGrok, map[string]string{"GROK_HOME": "/x/grok"}, "/x/grok/auth.json"},
+		{"grok home", llmprovider.ProviderGrok, map[string]string{"GROK_HOME": "/x/grok"}, filepath.Join("/x/grok", "auth.json")},
 		{"grok default", llmprovider.ProviderGrok, nil, filepath.Join(home, ".grok", "auth.json")},
 	} {
 		t.Run(tc.name, func(t *testing.T) {
```

`GROK_AUTH_PATH` keeps its literal expectation, because `vendorAuthPath`
returns that path unchanged.

**Proof** (2026-09-28, on `a27da70`):
* **Windows** (Go 1.26.6, `windows/amd64`, in a temporary clone):
  * **Red.** Base fails exactly as CI does, on `codex_home` and `grok_home`.
  * **Green.** With the change, all five cases pass.
  * **Mutant.** `vendorAuthPath` changed to `dir + "/auth.json"` fails the
    changed test: `vendorAuthPath = "/x/codex/auth.json", <nil>; want
    "\\x\\codex\\auth.json"`. On macOS and Linux this mutant is equivalent
    (the separator is `/`), so only a Windows run can kill it.
  * **Full suite.** `wizard` and every other package pass except
    `selfupdate`. There, `TestNativeReplaceRunningCopy` fails on the laptop
    with `replace target: Access is denied.` It passes on CI's Windows
    runner, so it is recorded as an open observation and is out of scope
    here (§9).
* **macOS** (a scratch clone): the §0.2 gate passes, and `gofmt` and
  `golint` pass on `wizard/import_test.go`.

**Steps.**
1. Confirm `main` is at `a27da70`, or a descendant that has not changed
   `wizard/import_test.go`.
2. Apply the diff above with `git apply`, and check the diff is exactly this
   one.
3. Run the §0.2 gate on macOS, and `go test -count=1 ./wizard` on a Windows
   host.
4. Commit with `git commit --no-edit`.
5. After the owner pushes, confirm CI's `validate (windows-2025)` job passes
   on that commit. Record it in §10, and set this plan to `status: complete`.

## 7. Acceptance criteria

* Every Appendix A red test fails before its fix and passes after it.
* Every mutant is killed.
* The gate passes after each phase.
* The vendor-CLI live tests pass with both opt-ins.
* The MADR's §5 confirmations hold:
  * a changed store refresh token means no refresh request;
  * a rewritten vendor file returns the new token, with no token endpoint
    contacted;
  * the exact Grok scope is chosen in all 50 iterations.
* O6's owner-run login passes before O6 lands.
* Amended 2026-09-28 (O8): CI passes on all three runners (Linux, macOS and
  Windows), not only the macOS gate.

## 8. Rollout and rollback

* **Behaviour changes:**
  * **Vendor CLI imports.** Consumers holding an imported session keep
    working until its refresh fails. A consumer that re-runs the wizard gets
    `CredVendorCLI` and must persist `VendorAuthPath`, then construct
    `llmprovider.VendorCLISession` from it. This is the consumer change the
    MADR's consequences name.
  * **Refresh.** Terminal refresh failures are now `ErrAuthFailure`, so
    retry loops stop.
* **Additive API:** `VendorCLISession`, `CredVendorCLI`,
  `Result.VendorAuthPath` and `RevokeOAuthSession`.
* **Rollback:** revert the phase commit. O1 is the one with a consumer-side
  change; reverting it restores the copy-and-refresh import.

## 9. Deviation log

* **2026-09-28: `TestVendorAuthPath` fails on Windows CI.**
  * **Found:** CI's `validate (windows-2025)` job has failed on every push
    since O1 reached `origin`: two cases expected `/`-separated paths.
  * **Decision:** fix the test's expectations with `filepath.Join`. The code
    is correct and stays unchanged.
  * **Scope added:** Phase O8, `wizard/import_test.go` only.
  * **Status:** returned from `complete` to `in-progress` until O8 lands and
    CI's Windows job passes.
  * **Not addressed:** `TestNativeReplaceRunningCopy` (`selfupdate`) fails
    with `Access is denied.` on the owner's Windows laptop, but passes on
    CI's Windows runner. It is not part of this plan. It needs its own
    investigation if it is to be pursued.

## 10. Execution record

Executed on `main`, 2026-09-27, after `0012-PLAN-circuit-breaker-test.md`
(`05a1fcf`). Each phase applied Appendix B with `git apply`, taken from this
document, and checked equal to the proven diff.

* **O1**, commit `93bc781`: 2 red tests failed as required; 0 base guards passed; gate passed; 5/5 mutants killed.
* **O2**, commit `73e2b41`: 7 red tests failed as required; 3 base guards passed; gate passed; 4/4 mutants killed.
* **O3**, commit `46bd8c7`: 4 red tests failed as required; 2 base guards passed; gate passed; 3/3 mutants killed.
* **O4**, commit `45fabc0`: nothing red (see Appendix A); 0 base guards passed; gate passed; 4/4 mutants killed.
* **O5**, commit `85e20ab`: 1 red test failed as required; 1 base guard passed; gate passed; 1/1 mutants killed.
* **O6**, commit `a5f2460`: 1 red test failed as required; 1 base guard passed; gate passed; 1/1 mutants killed.

**Live** (`-tags live_gateways`, this plan's tests, on the executed tree): 4 passed, 0 skipped, 0 failed.

**O6 gate** (owner, 2026-09-27): `TestLive_ChatGPTBrowserLogin`, run from O6's tree (O5 plus Appendix B.O6), passed in 78.65 s: the owner signed in through `http://127.0.0.1:1455/auth/callback`, the new session generated once, and `RevokeOAuthSession` revoked it. O6 then landed as recorded above.

## Appendix A — Proof record (2026-09-27)

### A.O1 Phase O1 — Vendor CLI logins are read-through (§5.1)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestConfigureLLM_VendorLoginReadsThrough` | unit | Kind/access/refresh/saves = "oauth"/"sess-grok-cli"/"rt-grok-cli"/1, want vendor_cli, no tokens, no save |
| `TestConfigureLLM_VendorLoginHonoursGrokAuthPath` | unit | ConfigureLLM() error = wizard: read grok vendor session: openat auth.json: no such file or directory |

**Guards before the fix:**

* `TestVendorCLISession_ReadsThrough`: added with the fix.
* `TestVendorCLISession_ExpiredIsAuthFailure`: added with the fix.
* `TestVendorCLISession_GrokExactScope`: added with the fix.
* `TestVendorCLISession_OpenAIIsChatGPT`: added with the fix.
* `TestVendorAuthPath`: added with the fix.

**Gate** (fix applied): all passed — per-file `golint` on 12 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `oa1-expiry-ignored` | `TestVendorCLISession_ExpiredIsAuthFailure` | killed | err = <nil>, want ErrAuthFailure advising to run codex |
| `oa1-grok-first-entry` | `TestVendorCLISession_GrokExactScope` | killed | Token = "xai-api-key", <nil>; want grok-default |
| `oa1-not-chatgpt` | `TestVendorCLISession_OpenAIIsChatGPT` | killed | Generate: invalid character 'e' looking for beginning of value |
| `oa1-account-not-recorded` | `TestVendorCLISession_ReadsThrough` | killed | account = "", want "acct_1" |
| `oa1-auth-path-ignored` | `TestVendorAuthPath` | killed | vendorAuthPath = "/x/grok/auth.json", <nil>; want "/y/login.json" |

### A.O2 Phase O2 — Refresh of `mcplib`-owned sessions (§5.2)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestOAuthRefresh_ReloadsStoreFirst` | unit | Token = "a-refreshed", <nil> after 1 refreshes (refresh "rt-new") |
| `TestOAuthRefresh_StoredTokenIsTheOneRefreshed` | unit | Token = "a-refreshed", <nil> after 1 requests sending "" |
| `TestOAuthRefresh_OpenAISendsJSON` | unit | Content-Type "application/x-www-form-urlencoded", body map[] |
| `TestOAuthRefresh_TerminalFailuresAreAuthFailure` | unit | err = oauth: refresh failed: 400 Bad Request: {"error":"invalid_client"} after 1 requests, want ErrAuthFailure after 1 |
| `TestOAuthRefresh_RetriesTransientFailures` | unit | Token = "", oauth: refresh failed: 503 Service Unavailable:  after 1 requests |
| `TestOAuthRefresh_ExpiryFromJWT` | unit | refresh Expiry = 2026-09-27 17:50:19.221149 -0500 CDT m=+3600.003637210, <nil> |
| `TestOAuthRefresh_FiveMinuteWindow` | unit | four minutes left: Token = "a-old", <nil> after 0 |

**Guards before the fix:**

* `TestOAuthRefresh_GrokStaysForm`: passed on the unfixed code.
* `TestOAuthRefresh_SameStoredTokenStillRefreshes`: passed on the unfixed code.
* `TestOAuthRefresh_BadRequestIsNotRetried`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 5 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `oa2-json-for-all` | `TestOAuthRefresh_GrokStaysForm` | killed | Content-Type "application/json", grant ""; want the form grant |
| `oa2-trust-stored-token` | `TestOAuthRefresh_SameStoredTokenStillRefreshes` | killed | Token = "", <nil> after 0 refreshes; want a-refreshed after 1 |
| `oa2-retry-4xx` | `TestOAuthRefresh_BadRequestIsNotRetried` | killed | err = oauth: refresh failed: 400 Bad Request: {"error":"invalid_request"} after 3 requests, want one plain failure |
| `oa2-400-is-auth-failure` | `TestOAuthRefresh_BadRequestIsNotRetried` | killed | err = llm: authentication failed: oauth: refresh failed: 400 Bad Request: {"error":"invalid_request"} after 1 requests, want one plain failure |

### A.O3 Phase O3 — Callback and device-code hardening (§5.3)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestOAuthCallback_AcceptsLifeSciencesSuffix` | unit | callback = {code: err:0x4e5d1acdab0}, want code abc |
| `TestOAuthCallback_SurfacesErrorDescription` | unit | callback result = llmprovider.oauthCallbackResult{code:"", err:(*errors.errorString)(0x55b48c413ac0)}, want an error containing "Your plan is inactive" |
| `TestOAuthCallback_MissingCodexEntitlement` | unit | callback result = llmprovider.oauthCallbackResult{code:"", err:(*errors.errorString)(0x625f2418bb00)}, want an error containing "Codex is not enabled for your workspace" |
| `TestGrokDevice_RejectsUnsafeResponses` | unit | err = <nil>, notified = true |

**Guards before the fix:**

* `TestOAuthCallback_OnlyTheExactSuffix`: passed on the unfixed code.
* `TestGrokDevice_AcceptsHTTPSAndLoopback`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 4 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `oa3-any-suffix` | `TestOAuthCallback_OnlyTheExactSuffix` | killed | callback status = 200, want 400 |
| `oa3-https-only` | `TestGrokDevice_AcceptsHTTPSAndLoopback` | killed | device map[device_code:dc expires_in:60 interval:1 user_code:ABCD-EFGH verification_uri:http://localhost:22255/device]: err = oauth: Grok device-code response has an invalid verification URI, notified = false; want succe |
| `oa3-uppercase-only` | `TestGrokDevice_AcceptsHTTPSAndLoopback` | killed | device map[device_code:dc expires_in:60 interval:1 user_code:ABCD-efgh-1234 verification_uri:https://accounts.x.ai/device verification_uri_complete:https://accounts.x.ai/device?user_code=ABCD-efgh-1234]: err = oauth: Gro |

### A.O4 Phase O4 — `RevokeOAuthSession` (§5.3)

**Red:** none. Every test of this phase names API the fix adds, so each is proven by a mutant instead (below).

**Guards before the fix:**

* `TestRevokeOAuthSession_OpenAIRevokesRefreshToken`: added with the fix.
* `TestRevokeOAuthSession_OpenAIAccessOnly`: added with the fix.
* `TestRevokeOAuthSession_GrokUsesDiscovery`: added with the fix.
* `TestRevokeOAuthSession_GrokWithoutEndpoint`: added with the fix.
* `TestLive_GrokDiscoveryPublishesRevocation`: added with the fix.

**Gate** (fix applied): all passed — per-file `golint` on 5 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `oa4-prefer-access` | `TestRevokeOAuthSession_OpenAIRevokesRefreshToken` | killed | err = <nil>, request /oauth/revoke application/json map[token:a token_type_hint:access_token]; want JSON map[client_id:app_EMoamEEZ73f0CkXaXp7hrann token:rt token_type_hint:refresh_token] at /oauth/revoke |
| `oa4-client-id-always` | `TestRevokeOAuthSession_OpenAIAccessOnly` | killed | err = <nil>, body map[client_id:app_EMoamEEZ73f0CkXaXp7hrann token:a token_type_hint:access_token]; want map[token:a token_type_hint:access_token] |
| `oa4-grok-guesses-endpoint` | `TestRevokeOAuthSession_GrokWithoutEndpoint` | killed | err = <nil> after request "/oauth2/revoke"; want ErrUnsupported and none |
| `oa4-grok-sends-json` | `TestRevokeOAuthSession_GrokUsesDiscovery` | killed | err = oauth: revoke failed: 401 Unauthorized: { |

### A.O5 Phase O5 — Withdraw `CODEX_ACCESS_TOKEN` (§5.4)

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestConfigureLLM_IgnoresCodexAccessTokenEnv` | unit | ConfigureLLM() error = fakePrompter: unexpected Confirm("Use CODEX_ACCESS_TOKEN from the environment (••••••••oken)?") |

**Guards before the fix:**

* `TestConfigureLLM_TokenStdinStillAcceptsChatGPTToken`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 4 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `oa5-paste-always-api-key` | `TestConfigureLLM_TokenStdinStillAcceptsChatGPTToken` | killed | ConfigureLLM() error = select model: fakePrompter: unexpected Select("Choose a OpenAI model:") |

### A.O6 Phase O6 — The `127.0.0.1` redirect (§5.3), gated

**Red** (tests diff applied, fix not): each test failed as required.

| Test | Kind | First failure message |
|---|---|---|
| `TestBrowserListener_OpenAIRedirectsToLoopbackIP` | unit | redirect = "http://localhost:1455/auth/callback", path = "/auth/callback" |

**Guards before the fix:**

* `TestBrowserListener_GrokRedirectUnchanged`: passed on the unfixed code.

**Gate** (fix applied): all passed — per-file `golint` on 4 files, `gofmt -l`, `go vet ./...`, `go vet -tags live_gateways ./llmprovider`, `make lint`, `go test -count=1 ./...`, `go test -race -count=1 ./llmprovider ./wizard`.

**Mutants** (each applied alone to a scratch copy of the green tree):

| Mutant | Test | Result | Failure message |
|---|---|---|---|
| `oa6-grok-localhost` | `TestBrowserListener_GrokRedirectUnchanged` | killed | redirect = "http://localhost:62439/callback", path = "/callback" |

### A.L Live runs on this plan's final tree (`-tags live_gateways`, 2026-09-27)

| Test | Result |
|---|---|
| `TestLive_ChatGPTBrowserLogin` | SKIP |
| `TestLive_GrokDiscoveryPublishesRevocation` | PASS |
| `TestLive_VendorCLISession` | PASS |
| `TestLive_VendorCLISession/openai` | PASS |
| `TestLive_VendorCLISession/grok` | PASS |

* Skip reason: MCPLIB_LIVE_BROWSER_LOGIN unset: this needs a person to sign in in a browser

Totals: 4 passed, 1 skipped, 0 failed.


## Appendix B — Diffs

Generated from the proof. Apply each phase's **Tests** diff, then its **Fix**
diff, with `git apply`, in order, on the §0.1 baseline.

### B.O1 Phase O1 — Vendor CLI logins are read-through (§5.1)

**Tests** (`oa1-tests.diff`, 90 lines):

```diff
diff --git a/wizard/vendor_cli_test.go b/wizard/vendor_cli_test.go
new file mode 100644
--- /dev/null
+++ b/wizard/vendor_cli_test.go
@@ -0,0 +1,85 @@
+package wizard
+
+import (
+	"context"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/maccavelli/mcplib/llmprovider"
+)
+
+const grokCLILogin = `{
+  "https://auth.x.ai::b1a00492-073a-47ea-816f-4c329264a828": {
+    "key": "sess-grok-cli",
+    "auth_mode": "oidc",
+    "refresh_token": "rt-grok-cli",
+    "expires_at": "2099-01-01T00:00:00Z",
+    "oidc_issuer": "https://auth.x.ai",
+    "oidc_client_id": "b1a00492-073a-47ea-816f-4c329264a828"
+  }
+}`
+
+func writeGrokCLILogin(t *testing.T, dir string) string {
+	t.Helper()
+	path := filepath.Join(dir, "auth.json")
+	if err := os.WriteFile(path, []byte(grokCLILogin), 0o600); err != nil {
+		t.Fatal(err)
+	}
+	return path
+}
+
+// TestConfigureLLM_VendorLoginReadsThrough: using the Grok CLI's login copies
+// no token and saves nothing; the CLI keeps its refresh token to itself
+// (MADR 0012 §5.1).
+func TestConfigureLLM_VendorLoginReadsThrough(t *testing.T) {
+	dir := t.TempDir()
+	writeGrokCLILogin(t, dir)
+	store := newMemoryTokenStore()
+	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), 4, 0}, confirms: []bool{true}}
+	res, err := ConfigureLLM(context.Background(), f, Options{
+		TokenStore: store,
+		LookupEnv: func(name string) string {
+			if name == "GROK_HOME" {
+				return dir
+			}
+			return ""
+		},
+	})
+	if err != nil {
+		t.Fatalf("ConfigureLLM() error = %v", err)
+	}
+	if string(res.Kind) != "vendor_cli" || res.AccessToken != "" || res.RefreshToken != "" || store.saves != 0 {
+		t.Fatalf("Kind/access/refresh/saves = %q/%q/%q/%d, want vendor_cli, no tokens, no save",
+			res.Kind, res.AccessToken, res.RefreshToken, store.saves)
+	}
+	assertTextMasksSecret(t, f.allText, "sess-grok-cli")
+}
+
+// TestConfigureLLM_VendorLoginHonoursGrokAuthPath: a non-empty
+// GROK_AUTH_PATH wins over GROK_HOME, as the Grok CLI resolves it
+// (xai-grok-login/src/storage.rs:45-55).
+func TestConfigureLLM_VendorLoginHonoursGrokAuthPath(t *testing.T) {
+	path := writeGrokCLILogin(t, t.TempDir())
+	emptyHome := t.TempDir()
+	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderGrok), 4, 0}, confirms: []bool{true}}
+	_, err := ConfigureLLM(context.Background(), f, Options{
+		TokenStore: newMemoryTokenStore(),
+		LookupEnv: func(name string) string {
+			switch name {
+			case "GROK_AUTH_PATH":
+				return path
+			case "GROK_HOME":
+				return emptyHome
+			}
+			return ""
+		},
+	})
+	if err != nil {
+		t.Fatalf("ConfigureLLM() error = %v", err)
+	}
+	if len(f.seenConfirm) == 0 || !strings.Contains(f.seenConfirm[0], path) {
+		t.Fatalf("confirm prompts = %q, want one naming %s", f.seenConfirm, path)
+	}
+}
```

**Fix** (`oa1-fix.diff`, 908 lines):

```diff
diff --git a/llmprovider/descriptor.go b/llmprovider/descriptor.go
--- a/llmprovider/descriptor.go
+++ b/llmprovider/descriptor.go
@@ -100,7 +100,7 @@
 			},
 			{
 				ID:          AuthImportVendorCLI,
-				Label:       "Import ~/.codex/auth.json",
+				Label:       "Use the Codex CLI login (~/.codex/auth.json)",
 				Interactive: true,
 				HeadlessOK:  true,
 			},
@@ -136,7 +136,7 @@
 			},
 			{
 				ID:          AuthImportVendorCLI,
-				Label:       "Import ~/.grok/auth.json",
+				Label:       "Use the Grok CLI login (~/.grok/auth.json)",
 				Interactive: true,
 				HeadlessOK:  true,
 			},
diff --git a/llmprovider/descriptor_test.go b/llmprovider/descriptor_test.go
--- a/llmprovider/descriptor_test.go
+++ b/llmprovider/descriptor_test.go
@@ -49,7 +49,7 @@
 			},
 			{
 				ID:          AuthImportVendorCLI,
-				Label:       "Import ~/.codex/auth.json",
+				Label:       "Use the Codex CLI login (~/.codex/auth.json)",
 				Interactive: true,
 				HeadlessOK:  true,
 			},
@@ -81,7 +81,7 @@
 			},
 			{
 				ID:          AuthImportVendorCLI,
-				Label:       "Import ~/.grok/auth.json",
+				Label:       "Use the Grok CLI login (~/.grok/auth.json)",
 				Interactive: true,
 				HeadlessOK:  true,
 			},
diff --git a/llmprovider/live_vendor_session_test.go b/llmprovider/live_vendor_session_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_vendor_session_test.go
@@ -0,0 +1,60 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"errors"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+)
+
+// liveVendorSession returns a read-through session on a real CLI login, or
+// skips. It never writes the file and never refreshes (MADR 0012 §5.1).
+func liveVendorSession(t *testing.T, provider, optIn, homeEnv, dir string) *VendorCLISession {
+	t.Helper()
+	if os.Getenv(optIn) != "1" {
+		t.Skipf("%s unset: this spends the CLI's subscription", optIn)
+	}
+	home := os.Getenv(homeEnv)
+	if home == "" {
+		userHome, err := os.UserHomeDir()
+		if err != nil {
+			t.Skip(err)
+		}
+		home = filepath.Join(userHome, dir)
+	}
+	path := filepath.Join(home, "auth.json")
+	if provider == ProviderGrok && os.Getenv("GROK_AUTH_PATH") != "" {
+		path = os.Getenv("GROK_AUTH_PATH")
+	}
+	s := &VendorCLISession{Provider: provider, Path: path}
+	if _, err := s.Token(t.Context()); errors.Is(err, ErrAuthFailure) {
+		t.Skipf("no live %s CLI login: %v", provider, err)
+	}
+	return s
+}
+
+// TestLive_VendorCLISession generates through each CLI's own login.
+func TestLive_VendorCLISession(t *testing.T) {
+	for _, tc := range []struct{ provider, optIn, homeEnv, dir, model string }{
+		{ProviderOpenAI, "MCPLIB_LIVE_CHATGPT", "CODEX_HOME", ".codex", "gpt-6-astra"},
+		{ProviderGrok, "MCPLIB_LIVE_GROK_CLI", "GROK_HOME", ".grok", "grok-4.6"},
+	} {
+		t.Run(tc.provider, func(t *testing.T) {
+			s := liveVendorSession(t, tc.provider, tc.optIn, tc.homeEnv, tc.dir)
+			ctx, cancel := liveCtx(t)
+			defer cancel()
+			p, err := NewProviderWithSource(tc.provider, s, tc.model)
+			if err != nil {
+				t.Fatal(err)
+			}
+			out, err := p.Generate(ctx, "Reply with only the word ALPHA")
+			skipIfTransient(t, err)
+			if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
+				t.Fatalf("Generate = %q, %v", out, err)
+			}
+		})
+	}
+}
diff --git a/llmprovider/openai_chatgpt.go b/llmprovider/openai_chatgpt.go
--- a/llmprovider/openai_chatgpt.go
+++ b/llmprovider/openai_chatgpt.go
@@ -48,11 +48,18 @@
 }
 
 func isChatGPTTokenSource(src TokenSource) bool {
+	if vendor, ok := src.(*VendorCLISession); ok {
+		return vendor.Provider == ProviderOpenAI
+	}
 	session, ok := src.(*OAuthSession)
 	return ok && session.ChatGPT()
 }
 
 func openAIAccountID(src TokenSource) string {
+	if vendor, ok := src.(*VendorCLISession); ok {
+		accountID, _ := vendor.vendorAccount()
+		return accountID
+	}
 	session, ok := src.(*OAuthSession)
 	if !ok {
 		return ""
@@ -64,6 +71,10 @@
 
 // openAIFedRAMP reports whether src is a FedRAMP ChatGPT session.
 func openAIFedRAMP(src TokenSource) bool {
+	if vendor, ok := src.(*VendorCLISession); ok {
+		_, fedramp := vendor.vendorAccount()
+		return fedramp
+	}
 	session, ok := src.(*OAuthSession)
 	if !ok {
 		return false
diff --git a/llmprovider/vendor_session.go b/llmprovider/vendor_session.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/vendor_session.go
@@ -0,0 +1,184 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/base64"
+	"encoding/json"
+	"fmt"
+	"io"
+	"os"
+	"path/filepath"
+	"strings"
+	"sync"
+	"time"
+)
+
+// vendorAuthFileLimit bounds a vendor CLI auth file.
+const vendorAuthFileLimit = 1 << 20
+
+// VendorCLISession is a TokenSource that borrows a vendor CLI's login
+// read-through (MADR 0012 §5.1). Every Token call re-reads the CLI's auth
+// file and returns its access token; it never refreshes. The refresh token
+// stays the CLI's alone, because both vendors revoke a refresh-token family
+// when one is used twice (grok-build xai-grok-login/src/oidc/refresh.rs:29-35;
+// codex login/src/auth/manager.rs:1657-1690). An expired token is
+// ErrAuthFailure telling the user to run the CLI, which refreshes it.
+type VendorCLISession struct {
+	// Provider is ProviderOpenAI (the Codex CLI's auth.json) or ProviderGrok
+	// (the Grok CLI's auth.json).
+	Provider string
+	// Path is the CLI's auth file.
+	Path string
+
+	mu        sync.Mutex
+	accountID string // the ChatGPT account id last read
+	fedramp   bool   // the id token's chatgpt_account_is_fedramp, last read
+}
+
+// vendorCredential is what one read of a CLI auth file yields.
+type vendorCredential struct {
+	access    string
+	expiry    time.Time
+	accountID string
+	fedramp   bool
+}
+
+// Token re-reads the auth file and returns its access token, or
+// ErrAuthFailure when the file is unreadable, holds no token, or the token
+// has expired.
+func (s *VendorCLISession) Token(ctx context.Context) (Token, error) {
+	if err := ctx.Err(); err != nil {
+		return Token{}, err
+	}
+	cli, hint, parse := vendorCLI(s.Provider)
+	if parse == nil {
+		return Token{}, fmt.Errorf("%w: no vendor CLI session for provider %q", ErrInvalidRequest, s.Provider)
+	}
+	raw, err := readVendorAuthFile(s.Path)
+	if err != nil {
+		return Token{}, fmt.Errorf("%w: read the %s login: %w; %s", ErrAuthFailure, cli, err, hint)
+	}
+	cred, err := parse(raw)
+	if err != nil {
+		return Token{}, fmt.Errorf("%w: the %s login in %s: %w; %s", ErrAuthFailure, cli, s.Path, err, hint)
+	}
+	if !cred.expiry.IsZero() && !time.Now().Before(cred.expiry) {
+		return Token{}, fmt.Errorf("%w: the %s login in %s expired at %s; %s",
+			ErrAuthFailure, cli, s.Path, cred.expiry.Format(time.RFC3339), hint)
+	}
+	s.mu.Lock()
+	s.accountID, s.fedramp = cred.accountID, cred.fedramp
+	s.mu.Unlock()
+	return Token{Value: cred.access, Type: TokenBearer, Expiry: cred.expiry, Header: oauthAuthorizationHeader}, nil
+}
+
+// vendorCLI names a provider's CLI, the advice for a stale login, and its
+// auth-file parser; the parser is nil for a provider without one.
+func vendorCLI(provider string) (cli, hint string, parse func([]byte) (vendorCredential, error)) {
+	switch provider {
+	case ProviderOpenAI:
+		return "Codex CLI", "run codex to refresh it, or codex login", parseCodexAuth
+	case ProviderGrok:
+		return "Grok CLI", "run grok to refresh it, or grok login", parseGrokAuth
+	default:
+		return "", "", nil
+	}
+}
+
+// readVendorAuthFile reads at most vendorAuthFileLimit bytes of path, opened
+// within its directory.
+func readVendorAuthFile(path string) ([]byte, error) {
+	root, err := os.OpenRoot(filepath.Dir(path))
+	if err != nil {
+		return nil, err
+	}
+	defer func() { ignoreOAuthError(root.Close()) }()
+	f, err := root.Open(filepath.Base(path))
+	if err != nil {
+		return nil, err
+	}
+	defer func() { ignoreOAuthError(f.Close()) }()
+	return io.ReadAll(io.LimitReader(f, vendorAuthFileLimit))
+}
+
+// parseCodexAuth reads ~/.codex/auth.json's ChatGPT tokens. Expiry is the
+// access token's JWT exp, as Codex reads it (login/src/auth/manager.rs:3004-3026).
+func parseCodexAuth(raw []byte) (vendorCredential, error) {
+	var file struct {
+		Tokens struct {
+			AccessToken string `json:"access_token"`
+			AccountID   string `json:"account_id"`
+			IDToken     string `json:"id_token"`
+		} `json:"tokens"`
+	}
+	if err := json.Unmarshal(raw, &file); err != nil {
+		return vendorCredential{}, fmt.Errorf("decode: %w", err)
+	}
+	if file.Tokens.AccessToken == "" {
+		return vendorCredential{}, fmt.Errorf("no ChatGPT access token")
+	}
+	return vendorCredential{
+		access:    file.Tokens.AccessToken,
+		expiry:    jwtExpiry(file.Tokens.AccessToken),
+		accountID: file.Tokens.AccountID,
+		fedramp:   chatGPTFedRAMP(file.Tokens.IDToken),
+	}, nil
+}
+
+// grokAuthScope is the Grok CLI's key for its default login,
+// "{issuer}::{client_id}" (xai-grok-login/src/config.rs:188-196, :241-256).
+var grokAuthScope = strings.TrimRight(DefaultGrokOAuthIssuer, "/") + "::" + DefaultGrokOAuthClientID
+
+// parseGrokAuth reads the Grok CLI's auth.json: exactly the default login's
+// entry, never another scope. Expiry is the entry's expires_at, else the
+// access token's JWT exp.
+func parseGrokAuth(raw []byte) (vendorCredential, error) {
+	var file map[string]struct {
+		Key       string `json:"key"`
+		ExpiresAt string `json:"expires_at"`
+	}
+	if err := json.Unmarshal(raw, &file); err != nil {
+		return vendorCredential{}, fmt.Errorf("decode: %w", err)
+	}
+	entry, ok := file[grokAuthScope]
+	if !ok || entry.Key == "" {
+		return vendorCredential{}, fmt.Errorf("no %s login", grokAuthScope)
+	}
+	expiry := jwtExpiry(entry.Key)
+	if entry.ExpiresAt != "" {
+		parsed, err := time.Parse(time.RFC3339, entry.ExpiresAt)
+		if err != nil {
+			return vendorCredential{}, fmt.Errorf("expires_at: %w", err)
+		}
+		expiry = parsed
+	}
+	return vendorCredential{access: entry.Key, expiry: expiry}, nil
+}
+
+// jwtExpiry reads a JWT's numeric exp claim, or returns the zero time for a
+// token that is not a JWT or has none.
+func jwtExpiry(token string) time.Time {
+	parts := strings.Split(token, ".")
+	if len(parts) != 3 {
+		return time.Time{}
+	}
+	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
+	if err != nil {
+		return time.Time{}
+	}
+	var claims struct {
+		Exp *float64 `json:"exp"`
+	}
+	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == nil {
+		return time.Time{}
+	}
+	return time.Unix(int64(*claims.Exp), 0).UTC()
+}
+
+// vendorAccount returns the ChatGPT account id and FedRAMP flag a Codex CLI
+// session last read.
+func (s *VendorCLISession) vendorAccount() (accountID string, fedramp bool) {
+	s.mu.Lock()
+	defer s.mu.Unlock()
+	return s.accountID, s.fedramp
+}
diff --git a/llmprovider/vendor_session_test.go b/llmprovider/vendor_session_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/vendor_session_test.go
@@ -0,0 +1,93 @@
+package llmprovider
+
+import (
+	"context"
+	"errors"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+	"time"
+)
+
+func writeVendorFile(t *testing.T, path, content string) {
+	t.Helper()
+	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func codexAuthJSON(t *testing.T, exp time.Time, accountID string) (string, string) {
+	t.Helper()
+	access := openAITestJWT(t, map[string]any{"exp": exp.Unix()})
+	return `{"tokens":{"access_token":"` + access + `","refresh_token":"rt-cli","account_id":"` + accountID + `"}}`, access
+}
+
+// TestVendorCLISession_ReadsThrough: each Token call re-reads the file, so a
+// refresh the CLI writes is picked up with no request from mcplib.
+func TestVendorCLISession_ReadsThrough(t *testing.T) {
+	path := filepath.Join(t.TempDir(), "auth.json")
+	s := &VendorCLISession{Provider: ProviderOpenAI, Path: path}
+	for _, account := range []string{"acct_1", "acct_2"} {
+		content, access := codexAuthJSON(t, time.Now().Add(time.Hour), account)
+		writeVendorFile(t, path, content)
+		tok, err := s.Token(context.Background())
+		if err != nil || tok.Value != access {
+			t.Fatalf("Token = %q, %v; want the file's current token", tok.Value, err)
+		}
+		if got, _ := s.vendorAccount(); got != account {
+			t.Fatalf("account = %q, want %q", got, account)
+		}
+	}
+}
+
+// TestVendorCLISession_ExpiredIsAuthFailure: an expired token is never
+// refreshed here; the error says to run the CLI.
+func TestVendorCLISession_ExpiredIsAuthFailure(t *testing.T) {
+	path := filepath.Join(t.TempDir(), "auth.json")
+	content, _ := codexAuthJSON(t, time.Now().Add(-time.Minute), "acct")
+	writeVendorFile(t, path, content)
+	_, err := (&VendorCLISession{Provider: ProviderOpenAI, Path: path}).Token(context.Background())
+	if !errors.Is(err, ErrAuthFailure) || !strings.Contains(err.Error(), "run codex") {
+		t.Fatalf("err = %v, want ErrAuthFailure advising to run codex", err)
+	}
+}
+
+// TestVendorCLISession_GrokExactScope: the default login's scope is chosen
+// every time, never another entry of the map (50 runs defeat map order).
+func TestVendorCLISession_GrokExactScope(t *testing.T) {
+	path := filepath.Join(t.TempDir(), "auth.json")
+	writeVendorFile(t, path, `{
+  "xai::api_key": {"key": "xai-api-key", "auth_mode": "api_key"},
+  "https://auth.x.ai::other-client": {"key": "grok-other", "auth_mode": "oidc", "expires_at": "2099-01-01T00:00:00Z"},
+  "https://auth.x.ai::b1a00492-073a-47ea-816f-4c329264a828": {"key": "grok-default", "auth_mode": "oidc", "expires_at": "2099-01-01T00:00:00Z"}
+}`)
+	s := &VendorCLISession{Provider: ProviderGrok, Path: path}
+	for range 50 {
+		tok, err := s.Token(context.Background())
+		if err != nil || tok.Value != "grok-default" {
+			t.Fatalf("Token = %q, %v; want grok-default", tok.Value, err)
+		}
+	}
+}
+
+// TestVendorCLISession_OpenAIIsChatGPT: a Codex CLI login puts OpenAI in
+// ChatGPT mode, with the file's account id.
+func TestVendorCLISession_OpenAIIsChatGPT(t *testing.T) {
+	path := filepath.Join(t.TempDir(), "auth.json")
+	content, _ := codexAuthJSON(t, time.Now().Add(time.Hour), "acct_cli")
+	writeVendorFile(t, path, content)
+	client, c := chatGPTClient(t, chatGPTFixture(t, "chatgpt-text.sse"))
+	p, err := NewOpenAIWithSource(&VendorCLISession{Provider: ProviderOpenAI, Path: path}, "gpt-6-astra", WithHTTPClient(client))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, err := p.Generate(context.Background(), "hi"); err != nil {
+		t.Fatalf("Generate: %v", err)
+	}
+	c.mu.Lock()
+	defer c.mu.Unlock()
+	if c.body["stream"] != true || c.header.Get(openAIAccountHeader) != "acct_cli" {
+		t.Fatalf("stream = %v, account = %q; want ChatGPT mode for acct_cli", c.body["stream"], c.header.Get(openAIAccountHeader))
+	}
+}
diff --git a/wizard/auth.go b/wizard/auth.go
--- a/wizard/auth.go
+++ b/wizard/auth.go
@@ -22,6 +22,10 @@
 	CredAPIKey CredentialKind = "api_key"
 	// CredOAuth means the OAuth fields contain a refreshable or access-only session.
 	CredOAuth CredentialKind = "oauth"
+	// CredVendorCLI means VendorAuthPath names a vendor CLI's auth file, read on
+	// every request through llmprovider.VendorCLISession; the CLI keeps it
+	// fresh and mcplib holds no token (MADR 0012 §5.1).
+	CredVendorCLI CredentialKind = "vendor_cli"
 )
 
 // ErrOrchestrated reports that an orchestrator-owned process must use the LLM backplane.
@@ -33,10 +37,11 @@
 )
 
 type resolvedCredential struct {
-	kind    CredentialKind
-	apiKey  string
-	session *llmprovider.OAuthSession
-	source  llmprovider.TokenSource
+	kind       CredentialKind
+	apiKey     string
+	session    *llmprovider.OAuthSession
+	source     llmprovider.TokenSource
+	vendorPath string
 }
 
 func orchestrated(o Options) bool {
@@ -83,6 +88,9 @@
 	if method == llmprovider.AuthTokenStdin {
 		return resolveTokenStdin(ctx, p, d, o)
 	}
+	if method == llmprovider.AuthImportVendorCLI {
+		return resolveVendorCLI(ctx, p, d, o)
+	}
 	if o.TokenStore == nil {
 		return resolvedCredential{}, errors.New("wizard: TokenStore is required for OAuth")
 	}
@@ -107,23 +115,38 @@
 			return resolvedCredential{}, loginErr
 		}
 		return saveOAuthCredential(ctx, o.TokenStore, d.ID, session)
-	case llmprovider.AuthImportVendorCLI:
-		session, importErr := importVendorSession(d.ID, o)
-		if importErr != nil {
-			return resolvedCredential{}, importErr
-		}
-		use, confirmErr := p.Confirm(
-			fmt.Sprintf("Import the existing session (%s)?", logging.MaskSecret(session.Access)), true)
-		if confirmErr != nil {
-			return resolvedCredential{}, confirmErr
-		}
-		if !use {
-			return resolvedCredential{}, errors.New("wizard: vendor session import declined")
-		}
-		return saveOAuthCredential(ctx, o.TokenStore, d.ID, session)
 	default:
 		return resolvedCredential{}, fmt.Errorf("wizard: unsupported authentication method %q", method)
 	}
+}
+
+// resolveVendorCLI uses a vendor CLI's login read-through: it checks the
+// file holds a live token, asks, and returns the path, never the tokens
+// (MADR 0012 §5.1). No TokenStore is needed.
+func resolveVendorCLI(
+	ctx context.Context,
+	p Prompter,
+	d llmprovider.ProviderDescriptor,
+	o Options,
+) (resolvedCredential, error) {
+	path, err := vendorAuthPath(d.ID, o)
+	if err != nil {
+		return resolvedCredential{}, err
+	}
+	session := &llmprovider.VendorCLISession{Provider: d.ID, Path: path}
+	token, err := session.Token(ctx)
+	if err != nil {
+		return resolvedCredential{}, err
+	}
+	use, err := p.Confirm(fmt.Sprintf("Use the %s CLI login in %s (%s)?", d.Label, path,
+		logging.MaskSecret(token.Value)), true)
+	if err != nil {
+		return resolvedCredential{}, err
+	}
+	if !use {
+		return resolvedCredential{}, errors.New("wizard: vendor CLI login declined")
+	}
+	return resolvedCredential{kind: CredVendorCLI, source: session, vendorPath: path}, nil
 }
 
 func staticCredential(kind CredentialKind, key string) resolvedCredential {
diff --git a/wizard/auth_test.go b/wizard/auth_test.go
--- a/wizard/auth_test.go
+++ b/wizard/auth_test.go
@@ -106,7 +106,7 @@
 }
 
 func TestConfigureLLM_OAuthMethodsRequireTokenStore(t *testing.T) {
-	for _, authIdx := range []int{1, 2, 4} {
+	for _, authIdx := range []int{1, 2} {
 		f := &fakePrompter{
 			t:       t,
 			selects: []int{providerIdx(t, llmprovider.ProviderOpenAI), authIdx},
diff --git a/wizard/configure.go b/wizard/configure.go
--- a/wizard/configure.go
+++ b/wizard/configure.go
@@ -34,6 +34,9 @@
 	Model        string
 	BaseURL      string
 	Fallbacks    []string
+	// VendorAuthPath is the vendor CLI auth file a CredVendorCLI result reads
+	// through; consumers persist it and build llmprovider.VendorCLISession.
+	VendorAuthPath string
 }
 
 // Options controls the flow. The zero value runs a full interactive
@@ -133,6 +136,7 @@
 	}
 	res.Kind = credential.kind
 	res.APIKey = credential.apiKey
+	res.VendorAuthPath = credential.vendorPath
 	if credential.session != nil {
 		res.AccessToken = credential.session.Access
 		res.RefreshToken = credential.session.Refresh
@@ -278,7 +282,7 @@
 	source llmprovider.TokenSource,
 	o Options,
 ) llmprovider.ModelCatalog {
-	chatGPT := res.Kind == CredOAuth && d.ID == llmprovider.ProviderOpenAI
+	chatGPT := (res.Kind == CredOAuth || res.Kind == CredVendorCLI) && d.ID == llmprovider.ProviderOpenAI
 	// A ChatGPT session lists only from the Codex backend (MADR 0009 D11):
 	// the Platform catalog is not available to it, so there is no fallback.
 	static := d.StaticModels
diff --git a/wizard/import.go b/wizard/import.go
--- a/wizard/import.go
+++ b/wizard/import.go
@@ -1,163 +1,38 @@
 package wizard
 
 import (
-	"encoding/base64"
-	"encoding/json"
-	"errors"
 	"fmt"
 	"os"
 	"path/filepath"
-	"strings"
-	"time"
 
 	"github.com/maccavelli/mcplib/llmprovider"
 )
 
-const grokOAuthRefreshEndpoint = "https://auth.x.ai/oauth2/token"
-
-type openAIAuthFile struct {
-	Tokens struct {
-		AccessToken  string `json:"access_token"`
-		RefreshToken string `json:"refresh_token"`
-		AccountID    string `json:"account_id"`
-	} `json:"tokens"`
-}
-
-type grokAuthFileEntry struct {
-	Key          string `json:"key"`
-	AuthMode     string `json:"auth_mode"`
-	Refresh      string `json:"refresh_token"`
-	ExpiresAt    string `json:"expires_at"`
-	OIDCIssuer   string `json:"oidc_issuer"`
-	OIDCClientID string `json:"oidc_client_id"`
-}
-
-func importVendorSession(provider string, o Options) (*llmprovider.OAuthSession, error) {
-	dir, err := vendorAuthDir(provider, o)
-	if err != nil {
-		return nil, err
-	}
-	root, err := os.OpenRoot(dir)
-	if err != nil {
-		return nil, fmt.Errorf("wizard: open %s vendor session directory: %w", provider, err)
-	}
-	data, readErr := root.ReadFile("auth.json")
-	closeErr := root.Close()
-	if readErr != nil || closeErr != nil {
-		return nil, fmt.Errorf("wizard: read %s vendor session: %w", provider, errors.Join(readErr, closeErr))
-	}
+// vendorAuthPath resolves a vendor CLI's auth file as the CLI does (MADR 0012
+// §5.1). Codex: $CODEX_HOME/auth.json, else ~/.codex/auth.json. Grok: a
+// non-empty $GROK_AUTH_PATH verbatim, else $GROK_HOME/auth.json, else
+// ~/.grok/auth.json (grok-build xai-grok-login/src/storage.rs:45-55,
+// xai-dirs/src/lib.rs:43-58).
+func vendorAuthPath(provider string, o Options) (string, error) {
+	env := o.lookupEnv()
+	var homeEnv, defaultDir string
 	switch provider {
 	case llmprovider.ProviderOpenAI:
-		return importOpenAIAuth(data)
+		homeEnv, defaultDir = "CODEX_HOME", ".codex"
 	case llmprovider.ProviderGrok:
-		return importGrokAuth(data)
+		if path := env("GROK_AUTH_PATH"); path != "" {
+			return path, nil
+		}
+		homeEnv, defaultDir = "GROK_HOME", ".grok"
 	default:
-		return nil, fmt.Errorf("wizard: provider %q has no vendor session import", provider)
+		return "", fmt.Errorf("wizard: provider %q has no vendor CLI login", provider)
 	}
-}
-
-func vendorAuthDir(provider string, o Options) (string, error) {
-	var envName, defaultDir string
-	switch provider {
-	case llmprovider.ProviderOpenAI:
-		envName, defaultDir = "CODEX_HOME", ".codex"
-	case llmprovider.ProviderGrok:
-		envName, defaultDir = "GROK_HOME", ".grok"
-	default:
-		return "", fmt.Errorf("wizard: provider %q has no vendor auth path", provider)
-	}
-	if dir := o.lookupEnv()(envName); dir != "" {
-		return dir, nil
+	if dir := env(homeEnv); dir != "" {
+		return filepath.Join(dir, "auth.json"), nil
 	}
 	home, err := os.UserHomeDir()
 	if err != nil {
 		return "", fmt.Errorf("wizard: resolve home directory: %w", err)
 	}
-	return filepath.Join(home, defaultDir), nil
+	return filepath.Join(home, defaultDir, "auth.json"), nil
 }
-
-func importOpenAIAuth(data []byte) (*llmprovider.OAuthSession, error) {
-	var auth openAIAuthFile
-	if err := json.Unmarshal(data, &auth); err != nil {
-		return nil, fmt.Errorf("wizard: decode OpenAI vendor session: %w", err)
-	}
-	if auth.Tokens.AccessToken == "" || auth.Tokens.RefreshToken == "" {
-		return nil, errors.New("wizard: OpenAI vendor session requires access and refresh tokens")
-	}
-	return &llmprovider.OAuthSession{
-		Provider:  llmprovider.ProviderOpenAI,
-		Access:    auth.Tokens.AccessToken,
-		Refresh:   auth.Tokens.RefreshToken,
-		Expiry:    jwtExpiry(auth.Tokens.AccessToken),
-		Issuer:    llmprovider.DefaultOpenAIIssuer,
-		ClientID:  llmprovider.DefaultOpenAIClientID,
-		AccountID: auth.Tokens.AccountID,
-		TokenURL:  llmprovider.DefaultOpenAIIssuer + "/oauth/token",
-	}, nil
-}
-
-// jwtExpiry reads a JWT access token's numeric exp claim (MADR 0009 F11). A
-// token that is not a JWT, or has no exp, gives the zero time; the session then
-// refreshes on its first rejection instead.
-func jwtExpiry(token string) time.Time {
-	parts := strings.Split(token, ".")
-	if len(parts) != 3 {
-		return time.Time{}
-	}
-	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
-	if err != nil {
-		return time.Time{}
-	}
-	var claims struct {
-		Exp *float64 `json:"exp"`
-	}
-	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == nil {
-		return time.Time{}
-	}
-	return time.Unix(int64(*claims.Exp), 0).UTC()
-}
-
-func importGrokAuth(data []byte) (*llmprovider.OAuthSession, error) {
-	var entries map[string]json.RawMessage
-	if err := json.Unmarshal(data, &entries); err != nil {
-		return nil, fmt.Errorf("wizard: decode Grok vendor session: %w", err)
-	}
-	for scope, raw := range entries {
-		if scope == "xai::api_key" {
-			continue
-		}
-		var entry grokAuthFileEntry
-		if err := json.Unmarshal(raw, &entry); err != nil {
-			return nil, fmt.Errorf("wizard: decode Grok vendor session entry: %w", err)
-		}
-		if entry.AuthMode == "api_key" ||
-			strings.TrimRight(entry.OIDCIssuer, "/") != llmprovider.DefaultGrokOAuthIssuer {
-			continue
-		}
-		if entry.Key == "" {
-			return nil, errors.New("wizard: Grok vendor session has no access token")
-		}
-		var expiry time.Time
-		if entry.ExpiresAt != "" {
-			parsed, err := time.Parse(time.RFC3339, entry.ExpiresAt)
-			if err != nil {
-				return nil, fmt.Errorf("wizard: parse Grok vendor session expiry: %w", err)
-			}
-			expiry = parsed
-		}
-		clientID := entry.OIDCClientID
-		if clientID == "" {
-			clientID = llmprovider.DefaultGrokOAuthClientID
-		}
-		return &llmprovider.OAuthSession{
-			Provider: llmprovider.ProviderGrok,
-			Access:   entry.Key,
-			Refresh:  entry.Refresh,
-			Expiry:   expiry,
-			Issuer:   llmprovider.DefaultGrokOAuthIssuer,
-			ClientID: clientID,
-			TokenURL: grokOAuthRefreshEndpoint,
-		}, nil
-	}
-	return nil, errors.New("wizard: no Grok OAuth session found in vendor auth file")
-}
diff --git a/wizard/import_test.go b/wizard/import_test.go
--- a/wizard/import_test.go
+++ b/wizard/import_test.go
@@ -1,136 +1,36 @@
 package wizard
 
 import (
-	"context"
-	"encoding/base64"
 	"os"
 	"path/filepath"
 	"testing"
-	"time"
 
 	"github.com/maccavelli/mcplib/llmprovider"
 )
 
-const grokVendorAuthFixture = `{
-  "xai::api_key": {
-    "key": "xai-MUST-SKIP",
-    "auth_mode": "api_key"
-  },
-  "https://auth.x.ai::b1a00492-073a-47ea-816f-4c329264a828": {
-    "key": "sess-grok",
-    "auth_mode": "oidc",
-    "refresh_token": "rt-grok",
-    "expires_at": "2099-01-01T00:00:00Z",
-    "oidc_issuer": "https://auth.x.ai",
-    "oidc_client_id": "b1a00492-073a-47ea-816f-4c329264a828"
-  }
-}`
-
-// TestImportOpenAIAuth_SetsExpiryFromJWT: an imported ChatGPT session takes
-// its expiry from the access token's exp claim, so it refreshes before the
-// backend rejects it (F11). A token without one still imports.
-func TestImportOpenAIAuth_SetsExpiryFromJWT(t *testing.T) {
-	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":2000000000}`))
-	withExp := `{"tokens":{"access_token":"eyJhbGciOiJub25lIn0.` + payload + `.sig","refresh_token":"rt-chatgpt"}}`
-	session, err := importOpenAIAuth([]byte(withExp))
+// TestVendorAuthPath pins each CLI's own resolution (MADR 0012 §5.1).
+func TestVendorAuthPath(t *testing.T) {
+	home, err := os.UserHomeDir()
 	if err != nil {
-		t.Fatalf("importOpenAIAuth() error = %v", err)
+		t.Skip(err)
 	}
-	if want := time.Unix(2000000000, 0).UTC(); !session.Expiry.Equal(want) || session.Expiry.Location() != time.UTC {
-		t.Fatalf("Expiry = %v, want %v", session.Expiry, want)
-	}
-
-	session, err = importOpenAIAuth([]byte(`{"tokens":{"access_token":"at-chatgpt","refresh_token":"rt-chatgpt"}}`))
-	if err != nil || !session.Expiry.IsZero() {
-		t.Fatalf("no-exp import: Expiry = %v, err = %v; want zero and no error", session.Expiry, err)
+	for _, tc := range []struct {
+		name, provider string
+		env            map[string]string
+		want           string
+	}{
+		{"codex home", llmprovider.ProviderOpenAI, map[string]string{"CODEX_HOME": "/x/codex"}, "/x/codex/auth.json"},
+		{"codex default", llmprovider.ProviderOpenAI, nil, filepath.Join(home, ".codex", "auth.json")},
+		{"grok auth path", llmprovider.ProviderGrok,
+			map[string]string{"GROK_AUTH_PATH": "/y/login.json", "GROK_HOME": "/x/grok"}, "/y/login.json"},
+		{"grok home", llmprovider.ProviderGrok, map[string]string{"GROK_HOME": "/x/grok"}, "/x/grok/auth.json"},
+		{"grok default", llmprovider.ProviderGrok, nil, filepath.Join(home, ".grok", "auth.json")},
+	} {
+		t.Run(tc.name, func(t *testing.T) {
+			got, err := vendorAuthPath(tc.provider, Options{LookupEnv: func(name string) string { return tc.env[name] }})
+			if err != nil || got != tc.want {
+				t.Fatalf("vendorAuthPath = %q, %v; want %q", got, err, tc.want)
+			}
+		})
 	}
 }
-
-func TestImportGrok_SkipsAPIKeyScope(t *testing.T) {
-	dir := t.TempDir()
-	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(grokVendorAuthFixture), 0o600); err != nil {
-		t.Fatalf("write fixture: %v", err)
-	}
-	session, err := importVendorSession(llmprovider.ProviderGrok, Options{
-		LookupEnv: func(name string) string {
-			if name == "GROK_HOME" {
-				return dir
-			}
-			return ""
-		},
-	})
-	if err != nil {
-		t.Fatalf("importVendorSession() error = %v", err)
-	}
-	if session.Access != "sess-grok" || session.Refresh != "rt-grok" || session.Access == "xai-MUST-SKIP" {
-		t.Fatalf("imported access/refresh = %q/%q", session.Access, session.Refresh)
-	}
-	if session.Provider != llmprovider.ProviderGrok || session.Issuer != llmprovider.DefaultGrokOAuthIssuer ||
-		session.ClientID != llmprovider.DefaultGrokOAuthClientID || session.Expiry.IsZero() {
-		t.Fatalf("imported Grok metadata = %#v", session)
-	}
-}
-
-func TestConfigureLLM_ImportConfirmsAndPersistsSession(t *testing.T) {
-	dir := t.TempDir()
-	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(grokVendorAuthFixture), 0o600); err != nil {
-		t.Fatalf("write fixture: %v", err)
-	}
-	store := newMemoryTokenStore()
-	f := &fakePrompter{
-		t:        t,
-		selects:  []int{providerIdx(t, llmprovider.ProviderGrok), 4, 0},
-		confirms: []bool{true},
-	}
-	res, err := ConfigureLLM(context.Background(), f, Options{
-		TokenStore: store,
-		LookupEnv: func(name string) string {
-			if name == "GROK_HOME" {
-				return dir
-			}
-			return ""
-		},
-	})
-	if err != nil {
-		t.Fatalf("ConfigureLLM() error = %v", err)
-	}
-	if res.Kind != CredOAuth || res.APIKey != "" || res.AccessToken != "sess-grok" {
-		t.Errorf("result credential = %q/%q/%q", res.Kind, res.APIKey, res.AccessToken)
-	}
-	if store.saves != 1 || store.sessions[llmprovider.ProviderGrok] == nil {
-		t.Errorf("TokenStore saves/session = %d/%v", store.saves, store.sessions[llmprovider.ProviderGrok])
-	}
-	assertTextMasksSecret(t, f.allText, "sess-grok")
-}
-
-func TestImportOpenAI_IgnoresPlatformKeyInAuthJSON(t *testing.T) {
-	dir := t.TempDir()
-	fixture := `{
-  "OPENAI_API_KEY": "sk-MUST-IGNORE",
-  "tokens": {
-    "access_token": "at-chatgpt",
-    "refresh_token": "rt-chatgpt",
-    "account_id": "acct_test"
-  }
-}`
-	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(fixture), 0o600); err != nil {
-		t.Fatalf("write fixture: %v", err)
-	}
-	session, err := importVendorSession(llmprovider.ProviderOpenAI, Options{
-		LookupEnv: func(name string) string {
-			if name == "CODEX_HOME" {
-				return dir
-			}
-			return ""
-		},
-	})
-	if err != nil {
-		t.Fatalf("importVendorSession() error = %v", err)
-	}
-	if session.Access != "at-chatgpt" || session.Refresh != "rt-chatgpt" || session.Access == "sk-MUST-IGNORE" {
-		t.Fatalf("imported access/refresh = %q/%q", session.Access, session.Refresh)
-	}
-	if session.AccountID != "acct_test" || session.TokenURL != llmprovider.DefaultOpenAIIssuer+"/oauth/token" {
-		t.Fatalf("imported OpenAI metadata = %#v", session)
-	}
-}
```

### B.O2 Phase O2 — Refresh of `mcplib`-owned sessions (§5.2)

**Tests** (`oa2-tests.diff`, 234 lines):

```diff
diff --git a/llmprovider/oauth_refresh_test.go b/llmprovider/oauth_refresh_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/oauth_refresh_test.go
@@ -0,0 +1,229 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"io"
+	"net/http"
+	"net/http/httptest"
+	"path/filepath"
+	"strings"
+	"sync/atomic"
+	"testing"
+	"time"
+)
+
+// refreshServer answers the n-th refresh request (from 1) with reply(n) and
+// counts the requests.
+func refreshServer(t *testing.T, reply func(n int32, w http.ResponseWriter, r *http.Request)) (*httptest.Server, *atomic.Int32) {
+	t.Helper()
+	var calls atomic.Int32
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		reply(calls.Add(1), w, r)
+	}))
+	t.Cleanup(srv.Close)
+	return srv, &calls
+}
+
+func refreshOK(w http.ResponseWriter, access string) {
+	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": access, "refresh_token": "rt-new", "expires_in": 3600})
+}
+
+func expiredSession(srv *httptest.Server, issuer string) *OAuthSession {
+	return &OAuthSession{Provider: ProviderOpenAI, Access: "a-old", Refresh: "rt-old", Expiry: time.Now().Add(-time.Minute),
+		Issuer: issuer, ClientID: "client-test", TokenURL: srv.URL, HTTPClient: srv.Client()}
+}
+
+// TestOAuthRefresh_ReloadsStoreFirst: a store another process already
+// refreshed is adopted, and the stale refresh token is never sent.
+func TestOAuthRefresh_ReloadsStoreFirst(t *testing.T) {
+	srv, calls := refreshServer(t, func(_ int32, w http.ResponseWriter, _ *http.Request) { refreshOK(w, "a-refreshed") })
+	store, err := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if err := store.Save(context.Background(), ProviderOpenAI, &OAuthSession{Provider: ProviderOpenAI, Access: "a-stored",
+		Refresh: "rt-stored", Expiry: time.Now().Add(time.Hour), ClientID: "client-test", TokenURL: srv.URL}); err != nil {
+		t.Fatal(err)
+	}
+	session := expiredSession(srv, "")
+	session.Store = store
+	tok, err := session.Token(context.Background())
+	if err != nil || tok.Value != "a-stored" || calls.Load() != 0 || session.Refresh != "rt-stored" {
+		t.Fatalf("Token = %q, %v after %d refreshes (refresh %q); want the stored session and none",
+			tok.Value, err, calls.Load(), session.Refresh)
+	}
+}
+
+// TestOAuthRefresh_StoredTokenIsTheOneRefreshed: when the stored session is
+// due too, the refresh sends the stored refresh token, never the stale one.
+func TestOAuthRefresh_StoredTokenIsTheOneRefreshed(t *testing.T) {
+	var sent map[string]string
+	srv, calls := refreshServer(t, func(_ int32, w http.ResponseWriter, r *http.Request) {
+		_ = json.NewDecoder(r.Body).Decode(&sent)
+		refreshOK(w, "a-refreshed")
+	})
+	store, err := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if err := store.Save(context.Background(), ProviderOpenAI, &OAuthSession{Provider: ProviderOpenAI, Access: "a-stored",
+		Refresh: "rt-stored", Expiry: time.Now().Add(-time.Minute), Issuer: DefaultOpenAIIssuer, ClientID: "client-test",
+		TokenURL: srv.URL}); err != nil {
+		t.Fatal(err)
+	}
+	session := expiredSession(srv, DefaultOpenAIIssuer)
+	session.Store = store
+	tok, err := session.Token(context.Background())
+	if err != nil || tok.Value != "a-refreshed" || calls.Load() != 1 || sent["refresh_token"] != "rt-stored" {
+		t.Fatalf("Token = %q, %v after %d requests sending %q; want one refresh of rt-stored",
+			tok.Value, err, calls.Load(), sent["refresh_token"])
+	}
+}
+
+// TestOAuthRefresh_SameStoredTokenStillRefreshes: a store holding the same
+// refresh token changes nothing; the session refreshes.
+func TestOAuthRefresh_SameStoredTokenStillRefreshes(t *testing.T) {
+	srv, calls := refreshServer(t, func(_ int32, w http.ResponseWriter, _ *http.Request) { refreshOK(w, "a-refreshed") })
+	store, err := NewFileTokenStore(filepath.Join(t.TempDir(), "tokens"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	session := expiredSession(srv, "")
+	if err := store.Save(context.Background(), ProviderOpenAI, session); err != nil {
+		t.Fatal(err)
+	}
+	session.Store = store
+	tok, err := session.Token(context.Background())
+	if err != nil || tok.Value != "a-refreshed" || calls.Load() != 1 {
+		t.Fatalf("Token = %q, %v after %d refreshes; want a-refreshed after 1", tok.Value, err, calls.Load())
+	}
+}
+
+// TestOAuthRefresh_OpenAISendsJSON: the OpenAI issuer's refresh is a JSON
+// body, as Codex sends it.
+func TestOAuthRefresh_OpenAISendsJSON(t *testing.T) {
+	var contentType string
+	var body map[string]string
+	srv, _ := refreshServer(t, func(_ int32, w http.ResponseWriter, r *http.Request) {
+		contentType = r.Header.Get("Content-Type")
+		_ = json.NewDecoder(r.Body).Decode(&body)
+		refreshOK(w, "a-new")
+	})
+	if _, err := expiredSession(srv, DefaultOpenAIIssuer).Token(context.Background()); err != nil {
+		t.Fatalf("Token: %v", err)
+	}
+	if contentType != "application/json" || body["grant_type"] != "refresh_token" ||
+		body["refresh_token"] != "rt-old" || body["client_id"] != "client-test" {
+		t.Fatalf("Content-Type %q, body %v; want the JSON refresh grant", contentType, body)
+	}
+}
+
+// TestOAuthRefresh_GrokStaysForm: the Grok issuer's refresh stays
+// form-encoded, as the Grok CLI sends it.
+func TestOAuthRefresh_GrokStaysForm(t *testing.T) {
+	var contentType, grant string
+	srv, _ := refreshServer(t, func(_ int32, w http.ResponseWriter, r *http.Request) {
+		contentType = r.Header.Get("Content-Type")
+		if err := r.ParseForm(); err == nil {
+			grant = r.PostForm.Get("grant_type")
+		}
+		refreshOK(w, "a-new")
+	})
+	if _, err := expiredSession(srv, DefaultGrokOAuthIssuer).Token(context.Background()); err != nil {
+		t.Fatalf("Token: %v", err)
+	}
+	if contentType != "application/x-www-form-urlencoded" || grant != "refresh_token" {
+		t.Fatalf("Content-Type %q, grant %q; want the form grant", contentType, grant)
+	}
+}
+
+// TestOAuthRefresh_TerminalFailuresAreAuthFailure: a dead refresh token is
+// ErrAuthFailure after one request, so no retry loop resends it.
+func TestOAuthRefresh_TerminalFailuresAreAuthFailure(t *testing.T) {
+	cases := map[string]struct {
+		status int
+		body   string
+	}{
+		"invalid_grant":             {http.StatusBadRequest, `{"error":"invalid_grant"}`},
+		"invalid_client":            {http.StatusBadRequest, `{"error":"invalid_client"}`},
+		"refresh_token_expired":     {http.StatusBadRequest, `{"error":{"code":"refresh_token_expired"}}`},
+		"refresh_token_reused":      {http.StatusBadRequest, `{"error":{"code":"refresh_token_reused"}}`},
+		"refresh_token_invalidated": {http.StatusBadRequest, `{"error":"refresh_token_invalidated"}`},
+		"401":                       {http.StatusUnauthorized, `{"error":"unauthorized"}`},
+	}
+	for name, tc := range cases {
+		t.Run(name, func(t *testing.T) {
+			srv, calls := refreshServer(t, func(_ int32, w http.ResponseWriter, _ *http.Request) {
+				w.WriteHeader(tc.status)
+				_, _ = io.WriteString(w, tc.body)
+			})
+			_, err := expiredSession(srv, DefaultOpenAIIssuer).Token(context.Background())
+			if !errors.Is(err, ErrAuthFailure) || calls.Load() != 1 {
+				t.Fatalf("err = %v after %d requests, want ErrAuthFailure after 1", err, calls.Load())
+			}
+		})
+	}
+}
+
+// TestOAuthRefresh_RetriesTransientFailures: 503 twice, then success.
+func TestOAuthRefresh_RetriesTransientFailures(t *testing.T) {
+	srv, calls := refreshServer(t, func(n int32, w http.ResponseWriter, _ *http.Request) {
+		if n < 3 {
+			w.WriteHeader(http.StatusServiceUnavailable)
+			return
+		}
+		refreshOK(w, "a-third")
+	})
+	tok, err := expiredSession(srv, DefaultOpenAIIssuer).Token(context.Background())
+	if err != nil || tok.Value != "a-third" || calls.Load() != 3 {
+		t.Fatalf("Token = %q, %v after %d requests; want a-third after 3", tok.Value, err, calls.Load())
+	}
+}
+
+// TestOAuthRefresh_BadRequestIsNotRetried: a 400 that is not a known
+// terminal code fails after one request and is not an ErrAuthFailure.
+func TestOAuthRefresh_BadRequestIsNotRetried(t *testing.T) {
+	srv, calls := refreshServer(t, func(_ int32, w http.ResponseWriter, _ *http.Request) {
+		w.WriteHeader(http.StatusBadRequest)
+		_, _ = io.WriteString(w, `{"error":"invalid_request"}`)
+	})
+	_, err := expiredSession(srv, DefaultOpenAIIssuer).Token(context.Background())
+	if err == nil || errors.Is(err, ErrAuthFailure) || calls.Load() != 1 || !strings.Contains(err.Error(), "invalid_request") {
+		t.Fatalf("err = %v after %d requests, want one plain failure", err, calls.Load())
+	}
+}
+
+// TestOAuthRefresh_ExpiryFromJWT: the access token's exp wins over
+// expires_in, on refresh and on login.
+func TestOAuthRefresh_ExpiryFromJWT(t *testing.T) {
+	exp := time.Now().Add(10 * time.Minute).Truncate(time.Second)
+	access := openAITestJWT(t, map[string]any{"exp": exp.Unix()})
+	srv, _ := refreshServer(t, func(_ int32, w http.ResponseWriter, _ *http.Request) { refreshOK(w, access) })
+	session := expiredSession(srv, DefaultOpenAIIssuer)
+	tok, err := session.Token(context.Background())
+	if err != nil || !tok.Expiry.Equal(exp) {
+		t.Fatalf("refresh Expiry = %v, %v; want %v", tok.Expiry, err, exp)
+	}
+	login, err := oauthSessionFromResponse(oauthFlowConfig{provider: ProviderOpenAI, issuer: DefaultOpenAIIssuer, now: time.Now},
+		srv.URL, oauthTokenResponse{AccessToken: access, ExpiresIn: 3600})
+	if err != nil || !login.Expiry.Equal(exp) {
+		t.Fatalf("login Expiry = %v, %v; want %v", login.Expiry, err, exp)
+	}
+}
+
+// TestOAuthRefresh_FiveMinuteWindow: a token with four minutes left is
+// refreshed; one with six is not.
+func TestOAuthRefresh_FiveMinuteWindow(t *testing.T) {
+	srv, calls := refreshServer(t, func(_ int32, w http.ResponseWriter, _ *http.Request) { refreshOK(w, "a-new") })
+	session := expiredSession(srv, "")
+	session.Expiry = time.Now().Add(6 * time.Minute)
+	if tok, err := session.Token(context.Background()); err != nil || tok.Value != "a-old" {
+		t.Fatalf("six minutes left: Token = %q, %v; want a-old", tok.Value, err)
+	}
+	session.Expiry = time.Now().Add(4 * time.Minute)
+	if tok, err := session.Token(context.Background()); err != nil || tok.Value != "a-new" || calls.Load() != 1 {
+		t.Fatalf("four minutes left: Token = %q, %v after %d; want a-new after 1", tok.Value, err, calls.Load())
+	}
+}
```

**Fix** (`oa2-fix.diff`, 343 lines):

```diff
diff --git a/llmprovider/oauth_loopback.go b/llmprovider/oauth_loopback.go
--- a/llmprovider/oauth_loopback.go
+++ b/llmprovider/oauth_loopback.go
@@ -528,7 +528,7 @@
 		Provider:   config.provider,
 		Access:     payload.AccessToken,
 		Refresh:    payload.RefreshToken,
-		Expiry:     config.now().Add(time.Duration(expiresIn) * time.Second),
+		Expiry:     tokenExpiry(payload.AccessToken, expiresIn, config.now()),
 		Issuer:     config.issuer,
 		ClientID:   config.clientID,
 		AccountID:  chatGPTAccountID(payload.IDToken),
diff --git a/llmprovider/oauth_session.go b/llmprovider/oauth_session.go
--- a/llmprovider/oauth_session.go
+++ b/llmprovider/oauth_session.go
@@ -1,6 +1,7 @@
 package llmprovider
 
 import (
+	"bytes"
 	"context"
 	"encoding/json"
 	"errors"
@@ -8,6 +9,7 @@
 	"io"
 	"net/http"
 	"net/url"
+	"slices"
 	"strings"
 	"time"
 
@@ -15,12 +17,28 @@
 )
 
 const (
-	oauthRefreshSkew         = 2 * time.Minute
+	// oauthRefreshSkew refreshes an access token this long before it
+	// expires: five minutes, as Codex (login/src/auth/manager.rs:203-216) and
+	// the Grok CLI (xai-grok-login/src/model.rs:9) do.
+	oauthRefreshSkew         = 5 * time.Minute
 	oauthAuthorizationHeader = "Authorization"
 	// oauthErrorBodyLimit caps how much of a failed token response an error
 	// carries (MADR 0009 D8).
 	oauthErrorBodyLimit = 2048
+	// oauthRefreshAttempts and oauthRefreshBackoff retry a refresh that
+	// failed in transport, with 429 or with 5xx, as the Grok CLI does
+	// (xai-grok-login/src/oidc/protocol.rs:430-438).
+	oauthRefreshAttempts = 3
+	oauthRefreshBackoff  = 200 * time.Millisecond
 )
+
+// oauthTerminalRefreshCodes mean the refresh token is dead: Codex's permanent
+// failures (login/src/auth/manager.rs:1657-1690) and the Grok CLI's
+// (xai-grok-login/src/oidc/refresh.rs:21-27).
+var oauthTerminalRefreshCodes = []string{
+	"invalid_grant", "invalid_client",
+	"refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated",
+}
 
 // chatGPTAccessFixture is the stub access token a consumer test once wrote into
 // a live token store (MADR 0009 F3, F8).
@@ -98,10 +116,7 @@
 	state := s.refreshState()
 	s.mu.Unlock()
 
-	next, token, err := refreshOAuthSession(ctx, state)
-	if err == nil && state.store != nil {
-		err = state.store.Save(ctx, state.provider, next)
-	}
+	next, token, err := reloadOrRefresh(ctx, state)
 
 	s.mu.Lock()
 	if err == nil {
@@ -179,33 +194,40 @@
 }
 
 func refreshOAuthSession(ctx context.Context, state oauthSessionState) (*OAuthSession, Token, error) {
-	form := url.Values{
-		"grant_type":    {"refresh_token"},
-		"refresh_token": {state.refresh},
-		"client_id":     {state.clientID},
-	}
-	req, err := http.NewRequestWithContext(
-		ctx,
-		http.MethodPost,
-		refreshTokenURL(state),
-		strings.NewReader(form.Encode()),
-	)
+	var lastErr error
+	for attempt := range oauthRefreshAttempts {
+		if attempt > 0 {
+			if err := sleepWithContext(ctx, oauthRefreshBackoff<<(attempt-1)); err != nil {
+				return nil, Token{}, err
+			}
+		}
+		next, token, retry, err := refreshOAuthSessionOnce(ctx, state)
+		if !retry {
+			return next, token, err
+		}
+		lastErr = err
+	}
+	return nil, Token{}, lastErr
+}
+
+// refreshOAuthSessionOnce makes one refresh request. retry reports a
+// transport error, 429 or 5xx.
+func refreshOAuthSessionOnce(ctx context.Context, state oauthSessionState) (next *OAuthSession, token Token, retry bool, err error) {
+	req, err := newRefreshRequest(ctx, state)
 	if err != nil {
-		return nil, Token{}, fmt.Errorf("oauth: create refresh request: %w", err)
-	}
-	identityOf(ProviderConfig{}).setUserAgent(req)
-	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
-
+		return nil, Token{}, false, err
+	}
 	client := state.httpClient
 	if client == nil {
 		client = defaultHTTPClient()
 	}
 	resp, err := client.Do(req)
 	if err != nil {
-		return nil, Token{}, fmt.Errorf("oauth: refresh request: %w", err)
+		return nil, Token{}, true, fmt.Errorf("oauth: refresh request: %w", err)
 	}
 	if resp.StatusCode != http.StatusOK {
-		return nil, Token{}, oauthHTTPStatusError("refresh", resp)
+		retry = resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
+		return nil, Token{}, retry, refreshFailure(resp)
 	}
 
 	var payload oauthRefreshResponse
@@ -216,29 +238,24 @@
 		if closeErr != nil {
 			err = errors.Join(err, fmt.Errorf("oauth: close refresh response: %w", closeErr))
 		}
-		return nil, Token{}, err
+		return nil, Token{}, false, err
 	}
 	if closeErr != nil {
-		return nil, Token{}, fmt.Errorf("oauth: close refresh response: %w", closeErr)
+		return nil, Token{}, false, fmt.Errorf("oauth: close refresh response: %w", closeErr)
 	}
 	if payload.AccessToken == "" {
-		return nil, Token{}, errors.New("oauth: refresh response missing access token")
+		return nil, Token{}, false, errors.New("oauth: refresh response missing access token")
 	}
 
 	refresh := payload.RefreshToken
 	if refresh == "" {
 		refresh = state.refresh
 	}
-	expiresIn := payload.ExpiresIn
-	if expiresIn <= 0 {
-		expiresIn = 3600
-	}
-	expiry := time.Now().Add(time.Duration(expiresIn) * time.Second)
-	next := &OAuthSession{
+	next = &OAuthSession{
 		Provider:   state.provider,
 		Access:     payload.AccessToken,
 		Refresh:    refresh,
-		Expiry:     expiry,
+		Expiry:     tokenExpiry(payload.AccessToken, payload.ExpiresIn, time.Now()),
 		Issuer:     state.issuer,
 		ClientID:   state.clientID,
 		AccountID:  state.accountID,
@@ -252,7 +269,114 @@
 		Type:   TokenBearer,
 		Expiry: next.Expiry,
 		Header: oauthAuthorizationHeader,
-	}, nil
+	}, false, nil
+}
+
+// newRefreshRequest builds the refresh request: JSON for the OpenAI issuer,
+// as Codex sends it (login/src/oauth/client.rs:80-110), form-encoded
+// otherwise, as the Grok CLI sends it (xai-grok-login/src/oidc/protocol.rs:492-507).
+func newRefreshRequest(ctx context.Context, state oauthSessionState) (*http.Request, error) {
+	params := map[string]string{
+		"grant_type":    "refresh_token",
+		"refresh_token": state.refresh,
+		"client_id":     state.clientID,
+	}
+	contentType := "application/x-www-form-urlencoded"
+	var body io.Reader
+	if strings.TrimRight(state.issuer, "/") == DefaultOpenAIIssuer {
+		raw, err := json.Marshal(params)
+		if err != nil {
+			return nil, fmt.Errorf("oauth: encode refresh request: %w", err)
+		}
+		contentType, body = "application/json", bytes.NewReader(raw)
+	} else {
+		form := url.Values{}
+		for k, v := range params {
+			form.Set(k, v)
+		}
+		body = strings.NewReader(form.Encode())
+	}
+	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshTokenURL(state), body)
+	if err != nil {
+		return nil, fmt.Errorf("oauth: create refresh request: %w", err)
+	}
+	identityOf(ProviderConfig{}).setUserAgent(req)
+	req.Header.Set("Content-Type", contentType)
+	return req, nil
+}
+
+// refreshFailure reports a failed refresh response, closing its body. A 401
+// or a terminal error code wraps ErrAuthFailure: the refresh token is dead
+// and the user must sign in again.
+func refreshFailure(resp *http.Response) error {
+	body, readErr := io.ReadAll(io.LimitReader(resp.Body, oauthErrorBodyLimit))
+	ignoreOAuthError(resp.Body.Close())
+	resp.Body = io.NopCloser(bytes.NewReader(body))
+	err := oauthHTTPStatusError("refresh", resp)
+	if readErr != nil {
+		err = errors.Join(err, fmt.Errorf("oauth: read refresh response: %w", readErr))
+	}
+	if resp.StatusCode == http.StatusUnauthorized || slices.Contains(oauthTerminalRefreshCodes, refreshErrorCode(body)) {
+		return fmt.Errorf("%w: %w", ErrAuthFailure, err)
+	}
+	return err
+}
+
+// refreshErrorCode reads an OAuth error code: {"error": "code"},
+// {"error": {"code": "code"}} or {"code": "code"}, lower-cased.
+func refreshErrorCode(body []byte) string {
+	var top struct {
+		Error json.RawMessage `json:"error"`
+		Code  string          `json:"code"`
+	}
+	if json.Unmarshal(body, &top) != nil {
+		return ""
+	}
+	if code := jsonString(top.Error); code != "" {
+		return strings.ToLower(code)
+	}
+	var inner struct {
+		Code string `json:"code"`
+	}
+	if json.Unmarshal(top.Error, &inner) == nil && inner.Code != "" {
+		return strings.ToLower(inner.Code)
+	}
+	return strings.ToLower(top.Code)
+}
+
+// tokenExpiry is the access token's JWT exp when it has one, else now plus
+// expires_in (3600 s when absent), as Codex reads it (MADR 0012 §5.2).
+func tokenExpiry(access string, expiresIn int64, now time.Time) time.Time {
+	if exp := jwtExpiry(access); !exp.IsZero() {
+		return exp
+	}
+	if expiresIn <= 0 {
+		expiresIn = 3600
+	}
+	return now.Add(time.Duration(expiresIn) * time.Second)
+}
+
+// reloadOrRefresh re-reads the session from its store before refreshing:
+// another process sharing the store may already have rotated the refresh
+// token, and sending the old one again trips the issuer's reuse detection.
+// A stored session with a different refresh token is adopted, and refreshed
+// only if its own access token is due (MADR 0012 §5.2).
+func reloadOrRefresh(ctx context.Context, state oauthSessionState) (*OAuthSession, Token, error) {
+	if state.store != nil {
+		stored, err := state.store.Load(ctx, state.provider)
+		if err == nil && stored != nil && stored.Refresh != "" && stored.Refresh != state.refresh {
+			stored.Store, stored.HTTPClient = state.store, state.httpClient
+			if token, ok := stored.currentToken(); ok {
+				return stored, token, nil
+			}
+			state = stored.refreshState()
+		}
+	}
+	next, token, err := refreshOAuthSession(ctx, state)
+	if err == nil && state.store != nil {
+		err = state.store.Save(ctx, state.provider, next)
+	}
+	return next, token, err
 }
 
 func refreshTokenURL(state oauthSessionState) string {
diff --git a/llmprovider/oauth_session_test.go b/llmprovider/oauth_session_test.go
--- a/llmprovider/oauth_session_test.go
+++ b/llmprovider/oauth_session_test.go
@@ -249,7 +249,7 @@
 	}
 }
 
-func TestOAuthSession_SkewsTwoMinutes(t *testing.T) {
+func TestOAuthSession_SkewsFiveMinutes(t *testing.T) {
 	var calls atomic.Int32
 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
 		calls.Add(1)
@@ -278,7 +278,7 @@
 		t.Errorf("zero expiry token = %q, calls = %d; want current-access, 0", tok.Value, calls.Load())
 	}
 
-	session.Expiry = time.Now().Add(3 * time.Minute)
+	session.Expiry = time.Now().Add(6 * time.Minute)
 	tok, err = session.Token(context.Background())
 	if err != nil {
 		t.Fatalf("Token outside skew: %v", err)
diff --git a/llmprovider/openai_chatgpt_test.go b/llmprovider/openai_chatgpt_test.go
--- a/llmprovider/openai_chatgpt_test.go
+++ b/llmprovider/openai_chatgpt_test.go
@@ -7,7 +7,6 @@
 	"errors"
 	"io"
 	"net/http"
-	"net/url"
 	"reflect"
 	"strings"
 	"testing"
@@ -226,16 +225,17 @@
 			return openAITestHTTPResponse(request, http.StatusOK, openAITestResponse), nil
 		case "auth.test":
 			refreshCalls++
-			if err := request.ParseForm(); err != nil {
-				t.Errorf("parse refresh form: %v", err)
-			}
-			want := url.Values{
-				"client_id":     {DefaultOpenAIClientID},
-				"grant_type":    {"refresh_token"},
-				"refresh_token": {"old-refresh"},
-			}
-			if !reflect.DeepEqual(request.PostForm, want) {
-				t.Errorf("refresh form = %v, want %v", request.PostForm, want)
+			var got map[string]string
+			if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
+				t.Errorf("decode refresh body: %v", err)
+			}
+			want := map[string]string{
+				"client_id":     DefaultOpenAIClientID,
+				"grant_type":    "refresh_token",
+				"refresh_token": "old-refresh",
+			}
+			if !reflect.DeepEqual(got, want) {
+				t.Errorf("refresh body = %v, want the JSON grant %v (MADR 0012 §5.2)", got, want)
 			}
 			return openAITestHTTPResponse(request, http.StatusOK, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`), nil
 		default:
```

### B.O3 Phase O3 — Callback and device-code hardening (§5.3)

**Tests** (`oa3-tests.diff`, 158 lines):

```diff
diff --git a/llmprovider/oauth_callback_hygiene_test.go b/llmprovider/oauth_callback_hygiene_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/oauth_callback_hygiene_test.go
@@ -0,0 +1,63 @@
+package llmprovider
+
+import (
+	"net/http"
+	"net/http/httptest"
+	"strings"
+	"testing"
+)
+
+// lifeSciencesState is the state ChatGPT may return for an onboarding entry
+// point (codex login/src/callback_params.rs:1, server.rs:366-374).
+const lifeSciencesState = "expected-state.onboarding_entrypoint=life_sciences"
+
+// TestOAuthCallback_AcceptsLifeSciencesSuffix: the loopback and a pasted
+// callback URL both take the suffixed state.
+func TestOAuthCallback_AcceptsLifeSciencesSuffix(t *testing.T) {
+	result := make(chan oauthCallbackResult, 1)
+	recorder := httptest.NewRecorder()
+	oauthCallbackHandler("/callback", "expected-state", result, "").ServeHTTP(recorder, httptest.NewRequest(
+		http.MethodGet, "http://127.0.0.1/callback?code=abc&state="+lifeSciencesState, http.NoBody))
+	select {
+	case got := <-result:
+		if got.err != nil || got.code != "abc" {
+			t.Fatalf("callback = %+v, want code abc", got)
+		}
+	default:
+		t.Fatal("callback did not complete the waiter")
+	}
+	code, err := parseOAuthInput("http://localhost:1455/auth/callback?code=def&state="+lifeSciencesState, "expected-state")
+	if err != nil || code != "def" {
+		t.Fatalf("parseOAuthInput = %q, %v; want def", code, err)
+	}
+}
+
+// TestOAuthCallback_OnlyTheExactSuffix: any other suffix is still a mismatch.
+func TestOAuthCallback_OnlyTheExactSuffix(t *testing.T) {
+	for _, state := range []string{"expected-state.onboarding_entrypoint=other", "expected-statex", "other" + lifeSciencesState[len("expected-state"):]} {
+		assertCallbackCompletesWithError(t, "code=abc&state="+state, "state mismatch")
+		if _, err := parseOAuthInput("http://localhost/cb?code=abc&state="+state, "expected-state"); err == nil ||
+			!strings.Contains(err.Error(), "state mismatch") {
+			t.Errorf("parseOAuthInput(state %q) = %v, want a state mismatch", state, err)
+		}
+	}
+}
+
+// TestOAuthCallback_SurfacesErrorDescription: the IdP's description reaches
+// the user, as Codex prints it (server.rs:934-956).
+func TestOAuthCallback_SurfacesErrorDescription(t *testing.T) {
+	assertCallbackCompletesWithError(t, "state=expected-state&error=access_denied&error_description=Your+plan+is+inactive",
+		"Your plan is inactive")
+}
+
+// TestOAuthCallback_MissingCodexEntitlement: Codex's entitlement refusal
+// gets Codex's explanation (server.rs:934-944).
+func TestOAuthCallback_MissingCodexEntitlement(t *testing.T) {
+	assertCallbackCompletesWithError(t,
+		"state=expected-state&error=access_denied&error_description=missing_codex_entitlement",
+		"Codex is not enabled for your workspace")
+	if _, err := parseOAuthInput("http://localhost/cb?state=expected-state&error=access_denied&error_description=missing_codex_entitlement",
+		"expected-state"); err == nil || !strings.Contains(err.Error(), "Codex is not enabled for your workspace") {
+		t.Fatalf("parseOAuthInput = %v, want the entitlement explanation", err)
+	}
+}
diff --git a/llmprovider/oauth_device_validation_test.go b/llmprovider/oauth_device_validation_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/oauth_device_validation_test.go
@@ -0,0 +1,85 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"fmt"
+	"net/http"
+	"net/http/httptest"
+	"strings"
+	"testing"
+)
+
+// grokDeviceLogin runs a Grok device login against a stub issuer whose
+// device endpoint answers with device, and whose token endpoint succeeds at
+// once. It reports the error and whether the user was shown anything.
+func grokDeviceLogin(t *testing.T, device map[string]any) (notified bool, err error) {
+	t.Helper()
+	clock := newFakeOAuthClock()
+	mux := http.NewServeMux()
+	srv := httptest.NewServer(mux)
+	t.Cleanup(srv.Close)
+	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
+		writeTestJSON(t, w, fmt.Sprintf(`{"device_authorization_endpoint":%q,"token_endpoint":%q}`, srv.URL+"/device", srv.URL+"/token"))
+	})
+	mux.HandleFunc("/device", func(w http.ResponseWriter, _ *http.Request) {
+		raw, _ := json.Marshal(device)
+		writeTestJSON(t, w, string(raw))
+	})
+	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
+		writeTestJSON(t, w, `{"access_token":"grok-access","refresh_token":"grok-refresh","expires_in":3600}`)
+	})
+	_, err = LoginDeviceOAuth(context.Background(), ProviderGrok, OAuthFlowOptions{
+		HTTPClient:   srv.Client(),
+		ClientID:     "test-client",
+		Issuer:       srv.URL,
+		NotifyDevice: func(string, string) { notified = true },
+		now:          clock.now,
+		sleep:        clock.sleep,
+	})
+	return notified, err
+}
+
+func grokDevice(userCode, uri, complete string) map[string]any {
+	d := map[string]any{"device_code": "dc", "user_code": userCode, "verification_uri": uri, "expires_in": 60, "interval": 1}
+	if complete != "" {
+		d["verification_uri_complete"] = complete
+	}
+	return d
+}
+
+// TestGrokDevice_RejectsUnsafeResponses: a malicious issuer cannot put
+// control characters in front of the user or send them off https, as the
+// Grok CLI guards (xai-grok-login/src/device_code.rs:148-155, :474-487).
+func TestGrokDevice_RejectsUnsafeResponses(t *testing.T) {
+	for name, device := range map[string]map[string]any{
+		"control user_code": grokDevice("AB\x1b[2JCD", "https://accounts.x.ai/device", ""),
+		"space user_code":   grokDevice("AB CD", "https://accounts.x.ai/device", ""),
+		"javascript uri":    grokDevice("ABCD-EFGH", "javascript:alert(1)", ""),
+		"plain http uri":    grokDevice("ABCD-EFGH", "http://evil.test/device", ""),
+		"control in uri":    grokDevice("ABCD-EFGH", "https://accounts.x.ai/device\x07", ""),
+		"bad complete uri":  grokDevice("ABCD-EFGH", "https://accounts.x.ai/device", "ftp://evil.test/device"),
+		"http complete uri": grokDevice("ABCD-EFGH", "https://accounts.x.ai/device", "http://evil.test/device?user_code=ABCD-EFGH"),
+	} {
+		t.Run(name, func(t *testing.T) {
+			notified, err := grokDeviceLogin(t, device)
+			if err == nil || notified || !strings.Contains(err.Error(), "invalid") {
+				t.Fatalf("err = %v, notified = %t; want an invalid-response error before any notice", err, notified)
+			}
+		})
+	}
+}
+
+// TestGrokDevice_AcceptsHTTPSAndLoopback: https, and http to a loopback
+// host, pass.
+func TestGrokDevice_AcceptsHTTPSAndLoopback(t *testing.T) {
+	for _, device := range []map[string]any{
+		grokDevice("ABCD-efgh-1234", "https://accounts.x.ai/device", "https://accounts.x.ai/device?user_code=ABCD-efgh-1234"),
+		grokDevice("ABCD-EFGH", "http://localhost:22255/device", ""),
+		grokDevice("ABCD-EFGH", "http://127.0.0.1:22255/device", ""),
+	} {
+		if notified, err := grokDeviceLogin(t, device); err != nil || !notified {
+			t.Fatalf("device %v: err = %v, notified = %t; want success", device, err, notified)
+		}
+	}
+}
```

**Fix** (`oa3-fix.diff`, 163 lines):

```diff
diff --git a/llmprovider/oauth_device.go b/llmprovider/oauth_device.go
--- a/llmprovider/oauth_device.go
+++ b/llmprovider/oauth_device.go
@@ -12,6 +12,7 @@
 	"strconv"
 	"strings"
 	"time"
+	"unicode"
 )
 
 const (
@@ -169,6 +170,9 @@
 	if device.DeviceCode == "" || device.UserCode == "" || device.VerificationURI == "" {
 		return nil, errors.New("oauth: Grok device-code response is incomplete")
 	}
+	if err := validateGrokDeviceCode(device); err != nil {
+		return nil, err
+	}
 	verificationURL := device.VerificationURI
 	if device.VerificationURIComplete != "" {
 		verificationURL = device.VerificationURIComplete
@@ -220,6 +224,44 @@
 	}
 }
 
+// validateGrokDeviceCode refuses a device-code response a malicious issuer
+// could use against the user, as the Grok CLI does
+// (xai-grok-login/src/device_code.rs:148-155, :474-487): the user_code must
+// be letters, digits and '-', and each verification URI https, or http to a
+// loopback host, with no control characters.
+func validateGrokDeviceCode(device grokDeviceCode) error {
+	for _, r := range device.UserCode {
+		if r > unicode.MaxASCII || !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
+			return errors.New("oauth: Grok device-code response has an invalid user_code")
+		}
+	}
+	for _, uri := range []string{device.VerificationURI, device.VerificationURIComplete} {
+		if uri != "" && !safeVerificationURI(uri) {
+			return errors.New("oauth: Grok device-code response has an invalid verification URI")
+		}
+	}
+	return nil
+}
+
+func safeVerificationURI(uri string) bool {
+	if strings.IndexFunc(uri, unicode.IsControl) >= 0 {
+		return false
+	}
+	parsed, err := url.Parse(uri)
+	if err != nil {
+		return false
+	}
+	switch parsed.Scheme {
+	case "https":
+		return parsed.Host != ""
+	case "http":
+		host := parsed.Hostname()
+		return host == "localhost" || host == "127.0.0.1"
+	default:
+		return false
+	}
+}
+
 func durationFromSeconds(seconds float64, fallback time.Duration, floor bool) time.Duration {
 	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
 		return fallback
diff --git a/llmprovider/oauth_loopback.go b/llmprovider/oauth_loopback.go
--- a/llmprovider/oauth_loopback.go
+++ b/llmprovider/oauth_loopback.go
@@ -13,6 +13,7 @@
 	"os"
 	"strings"
 	"time"
+	"unicode"
 )
 
 const (
@@ -417,7 +418,7 @@
 			return
 		}
 		query := request.URL.Query()
-		if query.Get("state") != state {
+		if !callbackStateMatches(query.Get("state"), state) {
 			http.Error(w, "State mismatch", http.StatusBadRequest)
 			select {
 			case result <- oauthCallbackResult{err: errors.New("oauth: callback state mismatch")}:
@@ -425,10 +426,10 @@
 			}
 			return
 		}
-		if oauthError := query.Get("error"); oauthError != "" {
+		if authErr := callbackError(query); authErr != nil {
 			http.Error(w, "Authorization failed", http.StatusBadRequest)
 			select {
-			case result <- oauthCallbackResult{err: fmt.Errorf("oauth: authorization failed: %s", oauthError)}:
+			case result <- oauthCallbackResult{err: authErr}:
 			default:
 			}
 			return
@@ -459,6 +460,47 @@
 	return mux
 }
 
+// lifeSciencesStateSuffix is the onboarding marker ChatGPT may append to the
+// callback state (codex login/src/callback_params.rs:1).
+const lifeSciencesStateSuffix = ".onboarding_entrypoint=life_sciences"
+
+// callbackStateMatches accepts the expected state, or it followed by exactly
+// lifeSciencesStateSuffix (codex login/src/server.rs:366-374).
+func callbackStateMatches(received, expected string) bool {
+	return received == expected || received == expected+lifeSciencesStateSuffix
+}
+
+// callbackErrorMessageLimit bounds the IdP's error_description.
+const callbackErrorMessageLimit = 300
+
+// callbackError reports an authorize error from the callback query, or nil.
+// It carries error_description, and Codex's explanation for a workspace
+// without Codex (codex login/src/server.rs:934-956).
+func callbackError(query url.Values) error {
+	code := query.Get("error")
+	if code == "" {
+		return nil
+	}
+	description := strings.Map(func(r rune) rune {
+		if unicode.IsControl(r) {
+			return -1
+		}
+		return r
+	}, query.Get("error_description"))
+	if len(description) > callbackErrorMessageLimit {
+		description = description[:callbackErrorMessageLimit]
+	}
+	switch {
+	case code == "access_denied" && strings.Contains(strings.ToLower(description), "missing_codex_entitlement"):
+		return fmt.Errorf("oauth: authorization failed (%s): Codex is not enabled for your workspace; "+
+			"ask your workspace administrator for access to Codex", code)
+	case strings.TrimSpace(description) != "":
+		return fmt.Errorf("oauth: authorization failed (%s): %s", code, description)
+	default:
+		return fmt.Errorf("oauth: authorization failed: %s", code)
+	}
+}
+
 func parseOAuthInput(input, expectedState string) (string, error) {
 	input = strings.TrimSpace(input)
 	if input == "" {
@@ -467,11 +509,11 @@
 	parsed, err := url.Parse(input)
 	if err == nil && parsed.Scheme != "" {
 		query := parsed.Query()
-		if oauthError := query.Get("error"); oauthError != "" {
-			return "", fmt.Errorf("oauth: authorization failed: %s", oauthError)
-		}
-		if receivedState := query.Get("state"); receivedState != "" && receivedState != expectedState {
+		if receivedState := query.Get("state"); receivedState != "" && !callbackStateMatches(receivedState, expectedState) {
 			return "", errors.New("oauth: callback state mismatch")
+		}
+		if authErr := callbackError(query); authErr != nil {
+			return "", authErr
 		}
 		if code := query.Get("code"); code != "" {
 			return code, nil
```

### B.O4 Phase O4 — `RevokeOAuthSession` (§5.3)

**Tests:** no change in this part.

**Fix** (`oa4-fix.diff`, 281 lines):

```diff
diff --git a/llmprovider/live_oauth_revoke_test.go b/llmprovider/live_oauth_revoke_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_oauth_revoke_test.go
@@ -0,0 +1,24 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"net/http"
+	"testing"
+)
+
+// TestLive_GrokDiscoveryPublishesRevocation: xAI's discovery document names
+// a revocation endpoint (https://auth.x.ai/oauth2/revoke on 2026-09-27),
+// which RevokeOAuthSession uses. Read-only: nothing is revoked.
+func TestLive_GrokDiscoveryPublishesRevocation(t *testing.T) {
+	ctx, cancel := liveCtx(t)
+	defer cancel()
+	endpoints, err := oauthEndpointsFor(ctx, oauthFlowConfig{provider: ProviderGrok, issuer: DefaultGrokOAuthIssuer,
+		httpClient: http.DefaultClient})
+	if err != nil {
+		t.Skipf("discovery unreachable: %v", err)
+	}
+	if endpoints.Revocation != DefaultGrokOAuthIssuer+"/oauth2/revoke" {
+		t.Fatalf("revocation_endpoint = %q", endpoints.Revocation)
+	}
+}
diff --git a/llmprovider/oauth_loopback.go b/llmprovider/oauth_loopback.go
--- a/llmprovider/oauth_loopback.go
+++ b/llmprovider/oauth_loopback.go
@@ -57,6 +57,7 @@
 	Authorization string `json:"authorization_endpoint"`
 	Token         string `json:"token_endpoint"`
 	Device        string `json:"device_authorization_endpoint"`
+	Revocation    string `json:"revocation_endpoint"`
 }
 
 type oauthTokenResponse struct {
diff --git a/llmprovider/oauth_revoke.go b/llmprovider/oauth_revoke.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/oauth_revoke.go
@@ -0,0 +1,113 @@
+package llmprovider
+
+import (
+	"bytes"
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"net/http"
+	"net/url"
+	"strings"
+)
+
+// RevokeOAuthSession revokes an mcplib-owned session at its issuer, so a
+// logout ends the refresh-token family rather than only forgetting it (MADR
+// 0012 §5.3). It revokes the refresh token when there is one, else the access
+// token. It does not touch any TokenStore: the caller deletes its copy
+// whether or not revocation succeeds, as Codex does
+// (codex login/src/auth/revoke.rs:1-5). Never pass a vendor CLI's session:
+// revoking it would log the CLI out.
+//
+// OpenAI: a JSON request to {issuer}/oauth/revoke (revoke.rs:31-53, 104-114).
+// Grok: an RFC 7009 form request to the discovery document's
+// revocation_endpoint; without one the result wraps errors.ErrUnsupported.
+func RevokeOAuthSession(ctx context.Context, session *OAuthSession) error {
+	if session == nil {
+		return errors.New("oauth: revoke: nil session")
+	}
+	session.mu.Lock()
+	state := session.refreshState()
+	access := session.Access
+	session.mu.Unlock()
+
+	token, hint := state.refresh, oauthRefreshToken
+	if token == "" {
+		token, hint = access, "access_token"
+	}
+	if token == "" {
+		return errors.New("oauth: revoke: session holds no token")
+	}
+	client := state.httpClient
+	if client == nil {
+		client = defaultHTTPClient()
+	}
+
+	var req *http.Request
+	var err error
+	if strings.TrimRight(state.issuer, "/") == DefaultOpenAIIssuer {
+		req, err = openAIRevokeRequest(ctx, state, token, hint)
+	} else {
+		req, err = grokRevokeRequest(ctx, state, client, token, hint)
+	}
+	if err != nil {
+		return err
+	}
+	identityOf(ProviderConfig{}).setUserAgent(req)
+	resp, err := client.Do(req)
+	if err != nil {
+		return fmt.Errorf("oauth: revoke request: %w", err)
+	}
+	if resp.StatusCode != http.StatusOK {
+		return oauthHTTPStatusError("revoke", resp)
+	}
+	return resp.Body.Close()
+}
+
+// openAIRevokeRequest posts Codex's JSON body. The endpoint is the token
+// URL's origin with path /oauth/revoke, as Codex derives it from its refresh
+// URL (revoke.rs:134-150), else the issuer's.
+func openAIRevokeRequest(ctx context.Context, state oauthSessionState, token, hint string) (*http.Request, error) {
+	endpoint := DefaultOpenAIIssuer + "/oauth/revoke"
+	if state.tokenURL != "" {
+		u, err := url.Parse(state.tokenURL)
+		if err != nil {
+			return nil, fmt.Errorf("oauth: revoke: parse token URL: %w", err)
+		}
+		u.Path, u.RawQuery, u.Fragment = "/oauth/revoke", "", ""
+		endpoint = u.String()
+	}
+	body := map[string]string{"token": token, "token_type_hint": hint}
+	if hint == oauthRefreshToken {
+		body[oauthParamClientID] = state.clientID
+	}
+	raw, err := json.Marshal(body)
+	if err != nil {
+		return nil, fmt.Errorf("oauth: revoke: encode: %w", err)
+	}
+	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
+	if err != nil {
+		return nil, fmt.Errorf("oauth: revoke: create request: %w", err)
+	}
+	req.Header.Set("Content-Type", "application/json")
+	return req, nil
+}
+
+// grokRevokeRequest posts an RFC 7009 form to the issuer's discovered
+// revocation_endpoint (https://auth.x.ai/oauth2/revoke on 2026-09-27).
+func grokRevokeRequest(ctx context.Context, state oauthSessionState, client *http.Client, token, hint string) (*http.Request, error) {
+	endpoints, err := oauthEndpointsFor(ctx, oauthFlowConfig{provider: ProviderGrok, issuer: state.issuer, httpClient: client})
+	if err != nil {
+		return nil, err
+	}
+	if endpoints.Revocation == "" {
+		return nil, fmt.Errorf("oauth: revoke: %s publishes no revocation_endpoint: %w", state.issuer, errors.ErrUnsupported)
+	}
+	form := url.Values{"token": {token}, "token_type_hint": {hint}, oauthParamClientID: {state.clientID}}
+	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Revocation, strings.NewReader(form.Encode()))
+	if err != nil {
+		return nil, fmt.Errorf("oauth: revoke: create request: %w", err)
+	}
+	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
+	return req, nil
+}
diff --git a/llmprovider/oauth_revoke_test.go b/llmprovider/oauth_revoke_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/oauth_revoke_test.go
@@ -0,0 +1,93 @@
+package llmprovider
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"net/http"
+	"net/http/httptest"
+	"net/url"
+	"testing"
+)
+
+// revokeCapture records the revoke request a stub issuer received.
+type revokeCapture struct {
+	path, contentType string
+	json              map[string]string
+	form              url.Values
+}
+
+func revokeIssuer(t *testing.T, discovery bool) (*httptest.Server, *revokeCapture) {
+	t.Helper()
+	c := &revokeCapture{}
+	mux := http.NewServeMux()
+	srv := httptest.NewServer(mux)
+	t.Cleanup(srv.Close)
+	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
+		doc := fmt.Sprintf(`{"token_endpoint":%q}`, srv.URL+"/oauth2/token")
+		if discovery {
+			doc = fmt.Sprintf(`{"token_endpoint":%q,"revocation_endpoint":%q}`, srv.URL+"/oauth2/token", srv.URL+"/oauth2/revoke")
+		}
+		_, _ = w.Write([]byte(doc))
+	})
+	record := func(w http.ResponseWriter, r *http.Request) {
+		c.path, c.contentType = r.URL.Path, r.Header.Get("Content-Type")
+		if c.contentType == "application/json" {
+			_ = json.NewDecoder(r.Body).Decode(&c.json)
+		} else if err := r.ParseForm(); err == nil {
+			c.form = r.PostForm
+		}
+		w.WriteHeader(http.StatusOK)
+	}
+	mux.HandleFunc("/oauth/revoke", record)
+	mux.HandleFunc("/oauth2/revoke", record)
+	return srv, c
+}
+
+// TestRevokeOAuthSession_OpenAIRevokesRefreshToken: Codex's JSON body, at
+// the token URL's origin with path /oauth/revoke.
+func TestRevokeOAuthSession_OpenAIRevokesRefreshToken(t *testing.T) {
+	srv, c := revokeIssuer(t, false)
+	err := RevokeOAuthSession(context.Background(), &OAuthSession{Access: "a", Refresh: "rt", Issuer: DefaultOpenAIIssuer,
+		ClientID: DefaultOpenAIClientID, TokenURL: srv.URL + "/oauth/token", HTTPClient: srv.Client()})
+	want := map[string]string{"token": "rt", "token_type_hint": "refresh_token", "client_id": DefaultOpenAIClientID}
+	if err != nil || c.path != "/oauth/revoke" || c.contentType != "application/json" || fmt.Sprint(c.json) != fmt.Sprint(want) {
+		t.Fatalf("err = %v, request %s %s %v; want JSON %v at /oauth/revoke", err, c.path, c.contentType, c.json, want)
+	}
+}
+
+// TestRevokeOAuthSession_OpenAIAccessOnly: with no refresh token the access
+// token is revoked, and no client_id is sent (revoke.rs:31-53).
+func TestRevokeOAuthSession_OpenAIAccessOnly(t *testing.T) {
+	srv, c := revokeIssuer(t, false)
+	err := RevokeOAuthSession(context.Background(), &OAuthSession{Access: "a", Issuer: DefaultOpenAIIssuer,
+		ClientID: DefaultOpenAIClientID, TokenURL: srv.URL + "/oauth/token", HTTPClient: srv.Client()})
+	want := map[string]string{"token": "a", "token_type_hint": "access_token"}
+	if err != nil || fmt.Sprint(c.json) != fmt.Sprint(want) {
+		t.Fatalf("err = %v, body %v; want %v", err, c.json, want)
+	}
+}
+
+// TestRevokeOAuthSession_GrokUsesDiscovery: an RFC 7009 form to the
+// discovered revocation_endpoint.
+func TestRevokeOAuthSession_GrokUsesDiscovery(t *testing.T) {
+	srv, c := revokeIssuer(t, true)
+	err := RevokeOAuthSession(context.Background(), &OAuthSession{Access: "a", Refresh: "rt", Issuer: srv.URL,
+		ClientID: "grok-client", HTTPClient: srv.Client()})
+	if err != nil || c.path != "/oauth2/revoke" || c.form.Get("token") != "rt" ||
+		c.form.Get("token_type_hint") != "refresh_token" || c.form.Get("client_id") != "grok-client" {
+		t.Fatalf("err = %v, request %s %v; want the form at /oauth2/revoke", err, c.path, c.form)
+	}
+}
+
+// TestRevokeOAuthSession_GrokWithoutEndpoint: an issuer with no
+// revocation_endpoint is unsupported, and nothing is sent.
+func TestRevokeOAuthSession_GrokWithoutEndpoint(t *testing.T) {
+	srv, c := revokeIssuer(t, false)
+	err := RevokeOAuthSession(context.Background(), &OAuthSession{Access: "a", Refresh: "rt", Issuer: srv.URL,
+		ClientID: "grok-client", HTTPClient: srv.Client()})
+	if !errors.Is(err, errors.ErrUnsupported) || c.path != "" {
+		t.Fatalf("err = %v after request %q; want ErrUnsupported and none", err, c.path)
+	}
+}
diff --git a/llmprovider/oauth_session.go b/llmprovider/oauth_session.go
--- a/llmprovider/oauth_session.go
+++ b/llmprovider/oauth_session.go
@@ -30,6 +30,8 @@
 	// (xai-grok-login/src/oidc/protocol.rs:430-438).
 	oauthRefreshAttempts = 3
 	oauthRefreshBackoff  = 200 * time.Millisecond
+	// oauthRefreshToken is the refresh grant type, token field and hint.
+	oauthRefreshToken = "refresh_token"
 )
 
 // oauthTerminalRefreshCodes mean the refresh token is dead: Codex's permanent
@@ -277,9 +279,9 @@
 // otherwise, as the Grok CLI sends it (xai-grok-login/src/oidc/protocol.rs:492-507).
 func newRefreshRequest(ctx context.Context, state oauthSessionState) (*http.Request, error) {
 	params := map[string]string{
-		"grant_type":    "refresh_token",
-		"refresh_token": state.refresh,
-		"client_id":     state.clientID,
+		"grant_type":      oauthRefreshToken,
+		oauthRefreshToken: state.refresh,
+		"client_id":       state.clientID,
 	}
 	contentType := "application/x-www-form-urlencoded"
 	var body io.Reader
```

### B.O5 Phase O5 — Withdraw `CODEX_ACCESS_TOKEN` (§5.4)

**Tests** (`oa5-tests.diff`, 60 lines):

```diff
diff --git a/wizard/codex_access_token_test.go b/wizard/codex_access_token_test.go
new file mode 100644
--- /dev/null
+++ b/wizard/codex_access_token_test.go
@@ -0,0 +1,55 @@
+package wizard
+
+import (
+	"context"
+	"testing"
+
+	"github.com/maccavelli/mcplib/llmprovider"
+)
+
+// TestConfigureLLM_IgnoresCodexAccessTokenEnv: with CODEX_ACCESS_TOKEN set
+// and the environment allowed, token_stdin still asks for the credential;
+// the variable is never offered (codex login/src/auth/access_token.rs:1-14).
+func TestConfigureLLM_IgnoresCodexAccessTokenEnv(t *testing.T) {
+	f := &fakePrompter{
+		t:       t,
+		selects: []int{providerIdx(t, llmprovider.ProviderOpenAI), 3, 0},
+		secrets: []string{testKey},
+	}
+	res, err := ConfigureLLM(context.Background(), f, Options{
+		AllowEnv:   true,
+		TokenStore: newMemoryTokenStore(),
+		LookupEnv: func(name string) string {
+			if name == "CODEX_ACCESS_TOKEN" {
+				return "at-personal-access-token"
+			}
+			return ""
+		},
+	})
+	if err != nil {
+		t.Fatalf("ConfigureLLM() error = %v", err)
+	}
+	if len(f.seenConfirm) != 0 || len(f.seenSecret) != 1 || res.Kind != CredAPIKey || res.APIKey != testKey {
+		t.Fatalf("confirms %q, secrets %d, credential %q/%q; want the pasted key and no CODEX_ACCESS_TOKEN offer",
+			f.seenConfirm, len(f.seenSecret), res.Kind, res.APIKey)
+	}
+}
+
+// TestConfigureLLM_TokenStdinStillAcceptsChatGPTToken: a pasted ChatGPT
+// access token is still an access-only OAuth session.
+func TestConfigureLLM_TokenStdinStillAcceptsChatGPTToken(t *testing.T) {
+	store := newMemoryTokenStore()
+	f := &fakePrompter{
+		t:       t,
+		selects: []int{providerIdx(t, llmprovider.ProviderOpenAI), 3},
+		secrets: []string{"pasted-chatgpt-access"},
+		inputs:  []string{"chatgpt-model"},
+	}
+	res, err := ConfigureLLM(context.Background(), f, Options{TokenStore: store})
+	if err != nil {
+		t.Fatalf("ConfigureLLM() error = %v", err)
+	}
+	if res.Kind != CredOAuth || res.AccessToken != "pasted-chatgpt-access" || store.saves != 1 {
+		t.Fatalf("credential %q/%q, saves %d; want an access-only session", res.Kind, res.AccessToken, store.saves)
+	}
+}
```

**Fix** (`oa5-fix.diff`, 78 lines):

```diff
diff --git a/llmprovider/oauth_session.go b/llmprovider/oauth_session.go
--- a/llmprovider/oauth_session.go
+++ b/llmprovider/oauth_session.go
@@ -48,7 +48,7 @@
 
 // ValidateOAuthSession reports whether a session can be used for generation
 // (MADR 0009 D7). It must be refreshable, or the explicit access-only ChatGPT
-// token that token_stdin and CODEX_ACCESS_TOKEN produce, and never a stub. A
+// token that token_stdin produces, and never a stub. A
 // refreshable session needs a client id. Its token URL may be empty, because
 // the refresh derives it from the issuer.
 func ValidateOAuthSession(session *OAuthSession) error {
diff --git a/wizard/auth.go b/wizard/auth.go
--- a/wizard/auth.go
+++ b/wizard/auth.go
@@ -200,19 +200,8 @@
 	d llmprovider.ProviderDescriptor,
 	o Options,
 ) (resolvedCredential, error) {
-	if d.ID == llmprovider.ProviderOpenAI && o.AllowEnv {
-		if access := o.lookupEnv()("CODEX_ACCESS_TOKEN"); access != "" {
-			use, err := p.Confirm(
-				fmt.Sprintf("Use CODEX_ACCESS_TOKEN from the environment (%s)?", logging.MaskSecret(access)), true)
-			if err != nil {
-				return resolvedCredential{}, err
-			}
-			if use {
-				return saveAccessOnlyOpenAI(ctx, o, access)
-			}
-		}
-	}
-
+	// CODEX_ACCESS_TOKEN is not offered: in Codex it holds a personal access
+	// token or an agent-identity JWT, not a ChatGPT OAuth bearer (MADR 0012 §5.4).
 	value, err := p.Secret(fmt.Sprintf("Paste your %s credential", d.Label))
 	if err != nil {
 		return resolvedCredential{}, fmt.Errorf("enter credential: %w", err)
diff --git a/wizard/auth_test.go b/wizard/auth_test.go
--- a/wizard/auth_test.go
+++ b/wizard/auth_test.go
@@ -202,37 +202,6 @@
 	if err == nil || err.Error() != "wizard: TokenStore is required for OAuth" {
 		t.Fatalf("ConfigureLLM() error = %v", err)
 	}
-}
-
-func TestConfigureLLM_TokenStdinUsesCodexEnvironment(t *testing.T) {
-	store := newMemoryTokenStore()
-	f := &fakePrompter{
-		t:        t,
-		selects:  []int{providerIdx(t, llmprovider.ProviderOpenAI), 3},
-		confirms: []bool{true},
-		inputs:   []string{"chatgpt-model"},
-	}
-	res, err := ConfigureLLM(context.Background(), f, Options{
-		AllowEnv:   true,
-		TokenStore: store,
-		LookupEnv: func(name string) string {
-			if name == "CODEX_ACCESS_TOKEN" {
-				return "codex-access-abcd"
-			}
-			return ""
-		},
-	})
-	if err != nil {
-		t.Fatalf("ConfigureLLM() error = %v", err)
-	}
-	if res.Kind != CredOAuth || res.AccessToken != "codex-access-abcd" || res.Model != "chatgpt-model" ||
-		len(f.seenSecret) != 0 || store.saves != 1 {
-		t.Fatalf(
-			"result credential = %q/%q; model = %q; Secret calls = %d; TokenStore saves = %d",
-			res.Kind, res.AccessToken, res.Model, len(f.seenSecret), store.saves,
-		)
-	}
-	assertTextMasksSecret(t, f.allText, "codex-access-abcd")
 }
 
 func TestConfigureLLM_BrowserAndDevicePersistSessions(t *testing.T) {
```

### B.O6 Phase O6 — The `127.0.0.1` redirect (§5.3), gated

**Tests** (`oa6-tests.diff`, 89 lines):

```diff
diff --git a/llmprovider/live_oauth_login_test.go b/llmprovider/live_oauth_login_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/live_oauth_login_test.go
@@ -0,0 +1,46 @@
+//go:build live_gateways
+
+package llmprovider
+
+import (
+	"context"
+	"os"
+	"strings"
+	"testing"
+	"time"
+)
+
+// TestLive_ChatGPTBrowserLogin is the owner-run gate for the 127.0.0.1
+// redirect (MADR 0012 §5.3). It REQUIRES MCPLIB_LIVE_BROWSER_LOGIN=1 and a
+// person at a browser: it prints the authorize URL, waits up to five minutes
+// for the ChatGPT login to redirect back, generates once with the new
+// session, and then revokes it, so no mcplib session outlives the test.
+func TestLive_ChatGPTBrowserLogin(t *testing.T) {
+	if os.Getenv("MCPLIB_LIVE_BROWSER_LOGIN") != "1" {
+		t.Skip("MCPLIB_LIVE_BROWSER_LOGIN unset: this needs a person to sign in in a browser")
+	}
+	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
+	defer cancel()
+	session, err := LoginBrowserOAuth(ctx, ProviderOpenAI, OAuthFlowOptions{
+		OpenURL: func(u string) error {
+			t.Logf("open this URL and sign in: %s", u)
+			return nil
+		},
+	})
+	if err != nil {
+		t.Fatalf("LoginBrowserOAuth: %v", err)
+	}
+	defer func() {
+		if err := RevokeOAuthSession(context.Background(), session); err != nil {
+			t.Errorf("RevokeOAuthSession: %v", err)
+		}
+	}()
+	p, err := NewOpenAIWithSource(session, "gpt-6-astra")
+	if err != nil {
+		t.Fatal(err)
+	}
+	out, err := p.Generate(ctx, "Reply with only the word ALPHA")
+	if err != nil || !strings.Contains(strings.ToUpper(out), "ALPHA") {
+		t.Fatalf("Generate = %q, %v", out, err)
+	}
+}
diff --git a/llmprovider/oauth_redirect_test.go b/llmprovider/oauth_redirect_test.go
new file mode 100644
--- /dev/null
+++ b/llmprovider/oauth_redirect_test.go
@@ -0,0 +1,33 @@
+package llmprovider
+
+import (
+	"strings"
+	"testing"
+)
+
+// TestBrowserListener_OpenAIRedirectsToLoopbackIP: Codex, with the same
+// client id, redirects to 127.0.0.1 on 1455 or 1457 (codex
+// login/src/server.rs:193, :77-79).
+func TestBrowserListener_OpenAIRedirectsToLoopbackIP(t *testing.T) {
+	listeners, redirect, path, err := browserListener(ProviderOpenAI)
+	if err != nil {
+		t.Skipf("OpenAI loopback ports busy: %v", err)
+	}
+	defer closeCallbackListeners(listeners...)
+	if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/auth/callback") || path != "/auth/callback" {
+		t.Fatalf("redirect = %q, path = %q; want http://127.0.0.1:{port}/auth/callback", redirect, path)
+	}
+}
+
+// TestBrowserListener_GrokRedirectUnchanged: Grok keeps its ephemeral
+// 127.0.0.1 /callback.
+func TestBrowserListener_GrokRedirectUnchanged(t *testing.T) {
+	listeners, redirect, path, err := browserListener(ProviderGrok)
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer closeCallbackListeners(listeners...)
+	if !strings.HasPrefix(redirect, "http://127.0.0.1:") || !strings.HasSuffix(redirect, "/callback") || path != "/callback" {
+		t.Fatalf("redirect = %q, path = %q", redirect, path)
+	}
+}
```

**Fix** (`oa6-fix.diff`, 26 lines):

```diff
diff --git a/llmprovider/oauth_loopback.go b/llmprovider/oauth_loopback.go
--- a/llmprovider/oauth_loopback.go
+++ b/llmprovider/oauth_loopback.go
@@ -247,7 +247,9 @@
 		if err != nil {
 			return nil, "", "", err
 		}
-		return listeners, fmt.Sprintf("http://localhost:%d/auth/callback", port), "/auth/callback", nil
+		// 127.0.0.1, as Codex redirects with the same client id since
+		// 4b97832cfb (login/src/server.rs:193); MADR 0012 §5.3.
+		return listeners, fmt.Sprintf("http://127.0.0.1:%d/auth/callback", port), "/auth/callback", nil
 	}
 	listener, err := net.Listen("tcp", "127.0.0.1:0")
 	if err != nil {
diff --git a/llmprovider/oauth_loopback_test.go b/llmprovider/oauth_loopback_test.go
--- a/llmprovider/oauth_loopback_test.go
+++ b/llmprovider/oauth_loopback_test.go
@@ -282,7 +282,7 @@
 	if tokenForm.Get("code") != "browser-secret" || tokenForm.Get("code_verifier") == "" {
 		t.Fatalf("token form = %v", tokenForm)
 	}
-	if got := tokenForm.Get("redirect_uri"); !strings.HasPrefix(got, "http://localhost:") || !strings.HasSuffix(got, "/auth/callback") {
+	if got := tokenForm.Get("redirect_uri"); !strings.HasPrefix(got, "http://127.0.0.1:") || !strings.HasSuffix(got, "/auth/callback") {
 		t.Fatalf("redirect_uri = %q, want localhost OpenAI callback", got)
 	}
 }
```

