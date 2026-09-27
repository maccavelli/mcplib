---
status: proposed
date: 2026-09-16
decision-makers: mcplib maintainers
consulted: prepare-commit-msg
informed: none
---

<!-- markdownlint-disable MD013 MD024 MD033 MD036 MD060 -->

# Repair OAuth loopback, paste-code fallback, and Windows session wiring so ChatGPT and xAI browser login actually persist

## Context and Problem Statement

MADR 0008 shipped native browser / device-code / API-key authentication for
`openai` and `grok`. On this Windows laptop the shipped path does not do what
the user sees in the browser.

Reported behaviour (2026-09-14, interactive `prepare-commit-msg configure` on
this host, live binary `~/.global-git-hooks/prepare-commit-msg.exe` version
1.3.0 release, built 2026-09-14 14:03):

* **OpenAI / ChatGPT OAuth never yields a usable session.** After the browser
  login succeeds, the provider does not get a token, or the stored session is
  not one that generation can use. Every OpenAI OAuth attempt fails.
* **Grok device-code works.** A real JWT session is saved and is the current
  active provider.
* **Grok browser / "Sign in with xAI" does not.** The browser opens, the user
  authenticates successfully, and the token is not returned, stored, or used.
  This host is not headless.

0008's architecture (typed `TokenSource`, Codex host branch, Grok on
`api.x.ai`, `FileTokenStore` beside the consumer config) is not being reopened.
This record is the post-implementation debug pass: what the running code
actually does on Windows, where 0008's required wiring was never connected,
and what must change so browser login persists a session that generation can
spend.

### What was measured, not assumed

Probes were run on this host on 2026-09-14. Secrets are not recorded; only
lengths, prefixes, and non-secret fields.

**Loopback family mismatch (OpenAI).**
`llmprovider.browserListener` binds `127.0.0.1` and advertises
`http://localhost:{port}/auth/callback` (`oauth_loopback.go`). On this host:

* `localhost` resolves **AAAA `::1` first**, then A `127.0.0.1`.
* A listener bound to `127.0.0.1:1455` accepts `127.0.0.1:1455`.
* `CONNECT localhost:1455` **times out**.
* `CONNECT ::1:1455` **times out**.

So the Hydra redirect target the browser is given is not the socket the Go
server is holding. Device-code does not use this listener; that matches the
symptom split.

**Live consumer store.**
`%APPDATA%\prepare-commit-msg\` (the Windows `os.UserConfigDir` path
`prepare-commit-msg` actually uses):

| File | Observed |
| --- | --- |
| `config.json` | `active_provider=grok`; `grok.auth_kind=oauth`; `openai.auth_kind=oauth`; OpenAI model `gpt-5.4` (ChatGPT catalog) |
| `oauth/grok.json` | Real OIDC session: JWT access (prefix `eyJ0`, 786 bytes), refresh 86 bytes, `client_id` = `DefaultGrokOAuthClientID`, `token_url=https://auth.x.ai/oauth2/token`, expiry 2026-09-14 23:18:01 |
| `oauth/openai.json` | **Not a ChatGPT session.** Access is the 14-character fixture `chatgpt-access` (prefix `chat`, suffix `cess`, not a JWT, not `sk-`). `refresh` empty, `expiry` zero, `client_id` empty, `account_id` empty, `token_url` empty, `issuer=https://auth.openai.com` |

That OpenAI record is byte-for-byte the shape written by
`prepare-commit-msg/main_oauth_test.go`
`TestRunAnalyzer_OAuthDoesNotCallNewProviderWithAccessToken`
(`Access: "chatgpt-access"`, `Issuer: DefaultOpenAIIssuer`, no `ClientID`).
That test sets `HOME` and `XDG_CONFIG_HOME` only. A probe of `os.UserConfigDir`
on this host:

* default → `C:\Users\<user>\AppData\Roaming`
* with `HOME` and `XDG_CONFIG_HOME` redirected → **still**
  `C:\Users\<user>\AppData\Roaming`

Windows `UserConfigDir` reads `APPDATA`. The test writes the fixture into the
live store. `ValidateOAuth` treats any non-nil `Load` as success, so configure
and hook generation will accept it.

A real Codex session exists unused at `~/.codex/auth.json` (access 1757 bytes,
refresh 211, `account_id` 36). The import method would have produced a usable
`openai.json`. The live file is the test stub, not that import.

**Grok official CLI vs mcplib loopback.**
`grok-build` `xai-grok-shell/src/auth/oidc/login.rs`:

* Binds `127.0.0.1:{ephemeral}` and advertises `http://127.0.0.1:{port}/callback`
  (IPv4 literal — not subject to the `localhost`/`::1` bug).
* Serves CORS from `https://accounts.x.ai` with
  `allow_private_network(true)` (`accounts_app_cors_layer`).
* Races the loopback against **stdin paste** and prints
  `Paste the URL here if it doesn't connect:`.

mcplib Grok loopback uses the same `127.0.0.1` redirect URI but has **no CORS
headers, no OPTIONS handler, and no paste fallback**. `OAuthFlowOptions.InputCode`
exists (`oauth_loopback.go`) and is never set (`wizard/auth.go`
`oauthFlowOptions`). MADR 0008 required a `Prompter.Input` paste-code fallback.

**OpenURL wait.** `prepare-commit-msg/internal/ui/open.go` uses
`exec.Command(...).CombinedOutput()` including Windows
`rundll32.exe url.dll,FileProtocolHandler`. A redirected-stdio rundll32 launch
of a dead loopback URL on this host returned in **169 ms**, so a hang was
**not reproduced** with that probe. CombinedOutput is still the wrong primitive
(`LoginBrowserOAuth` calls `OpenURL` synchronously *before* the timeout select,
so a slow opener delays the wait and the 10-minute `flowCtx` does not cancel
it). Marked unverified as the cause of today's failures.

**Token-store overwrite.** A throwaway
`TestFileTokenStore_OverwriteExisting` on this host passed. First-save rename
is not the Grok-browser failure mode.

**Locked-in hang on bad callback.**
`TestOAuthCallback_RejectsStateMismatch` asserts that a state mismatch
**does not** send on the result channel. The waiter therefore sits until
`oauthBrowserTimeout` (10 minutes). That is the current contract, not an
accident the tests missed.

### Findings

1. **F1 — OpenAI loopback is IPv4-only while the advertised redirect host is
   `localhost`, which is IPv6-first on this Windows host.** Browser callback
   never reaches the server. Device-code does not use this path. Consequence:
   ChatGPT browser login can succeed at `auth.openai.com` and still return
   nothing to `LoginBrowserOAuth`.

2. **F2 — The working Grok path is device-code, not browser.** `oauth/grok.json`
   is a real OIDC session from 17:18. Grok browser login shares
   `LoginBrowserOAuth` with OpenAI. Consequence: the regression is in loopback
   completion (callback delivery + paste fallback + wait), not in
   `FileTokenStore` or Grok generate.

3. **F3 — `prepare-commit-msg` OAuth tests pollute the live Windows token
   store.** `TestRunAnalyzer_OAuthDoesNotCallNewProviderWithAccessToken` does
   not set `APPDATA`. `os.UserConfigDir` ignores `HOME`/`XDG_CONFIG_HOME` here.
   Live `oauth/openai.json` is the fixture `chatgpt-access`. Consequence:
   configure can mark `auth_kind=oauth` and generation sends a 14-character
   stub at `chatgpt.com`, then cannot refresh (`oauth: no refresh token`).

4. **F4 — Paste-code fallback is specified and unimplemented.** MADR 0008
   required `Prompter.Input` paste-code when loopback is unreachable. The
   library has `InputCode` and `parseOAuthInput`; the wizard never passes
   them. Grok CLI always races stdin paste. Consequence: a Windows IPv6 miss
   or a CORS-blocked Grok fetch has no recovery except waiting ten minutes.

5. **F5 — Grok loopback has none of the CORS / private-network headers the
   official CLI needed.** The accounts app origin is `https://accounts.x.ai`,
   not `auth.x.ai`. Chrome Local Network Access treats a public-origin fetch
   to `127.0.0.1` as a private-network request. Without
   `Access-Control-Allow-Origin` and `Access-Control-Allow-Private-Network`
   (and an OPTIONS handler), the browser can show an IdP success page while
   the CLI never sees `code`. This is **inferred** from the official CLI
   source, not captured as a HAR on this host.

6. **F6 — Failed callbacks do not finish the wait, and `OpenURL` is not under
   the login timeout.** State mismatch / missing code return HTTP 400 and
   leave the channel empty (test-locked). `OpenURL` runs to completion before
   `select` on `flowCtx`. Consequence: the process looks wedged after a
   "successful" browser login.

7. **F7 — Token exchange and refresh errors drop the response body.**
   `exchangeOAuthCode` / `refreshOAuthSession` return `oauth: token exchange
   failed: 400 Bad Request` with no IdP text. Grok CLI's
   `OidcError::TokenExchangeHttp` includes `{status} — {body}`. Consequence:
   even when the callback arrives, a reject is undiagnosable.

8. **F8 — `ValidateOAuth` / wizard save accept any non-nil session.** A
   missing refresh token, zero expiry, empty `client_id`, or 14-character
   access value still pass. Consequence: the polluted fixture is a
   "configured" ChatGPT provider.

9. **F9 — ChatGPT generate does not send the `originator` request header
   Codex and OpenCode send on every Codex-backend call.** Authorize-URL
   `originator=mcplib` is set; the `POST .../codex/responses` header is not.
   Codex's `default_client` always inserts `originator`. Whether the backend
   401s without it is **[unverified]** until a live ChatGPT session exists.

10. **F10 — Coverage never drove OpenAI loopback to completion, never bound
    `localhost`, never sent OPTIONS, and never isolated Windows `APPDATA`.**
    The only `LoginBrowserOAuth` success test is Grok, and it calls the
    callback with `http.Get` on the `127.0.0.1` redirect URI — it cannot see
    F1 or F5.

11. **F11 — OpenAI vendor import omits expiry.** `importOpenAIAuth` copies
    access/refresh/account_id but leaves `Expiry` zero, so `Token()` will not
    refresh until a 401. Secondary; the live store was not produced by import.

12. **F12 — ChatGPT generate rejected the frozen 5.x catalog; the live Codex
    `/models` list does not include those slugs.** A live generate on this host
    (2026-09-14) returned HTTP 400 "model is not supported when using Codex
    with a ChatGPT account" for `gpt-5.4`, `gpt-5.4-mini`, and `gpt-5.3-codex`.
    `GET https://chatgpt.com/backend-api/codex/models?client_version=0.0.0`
    returned visibility=`list` slugs in priority order starting at
    `gpt-6-astra`. Consequence: after ChatGPT OAuth, configure must not offer
    `StaticOpenAIChatGPT` or Platform `gpt-4.1-*`.

## Decision Drivers

* Browser login that shows an IdP success page must either persist a
  refreshable session or fail fast with an error the user can act on.
  Hanging until a 10-minute timeout is not a result.
* Device-code must keep working; it is the proven path on this host.
* The Codex Hydra redirect URI remain `http://localhost:{1455|1457}/auth/callback`.
  Changing the advertised host is an allow-list break, not a Windows fix.
* Grok redirect URI remain `http://127.0.0.1:{ephemeral}/callback` (RFC 8252).
* Tests that touch `UserConfigDir` / `OAuthDir` must not be able to write the
  live Windows profile. A fixture string in `%APPDATA%` is a product defect.
* Paste-code is part of 0008, not a new feature. Wire it.
* Do not weaken 0008's host-lock tests (ChatGPT must not hit
  `api.openai.com`; Grok must not hit `cli-chat-proxy`).
* No new Go module. No OS keyring. No Claude/Gemini OAuth.

## Considered Options

* **A — Repair loopback (dual-stack OpenAI, Grok CORS/PNA), wire paste-code,
  isolate Windows token-store tests, reject stub sessions** (chosen)
* **B — Document "Windows users must use device-code"**
* **C — Drop native browser login; import `~/.codex/auth.json` /
  `~/.grok/auth.json` only**
* **D — Shell out to `codex login` / `grok login`**

## Decision Outcome

Chosen option: **A**. 0008 already committed to native browser PKCE. The
failures are implementation and test-isolation defects on Windows (and
missing 0008 wiring), not a reason to abandon the grant. Device-code stays
as the headless path, not as a Windows consolation prize.

### The decisions

1. **D1 — OpenAI loopback listens on both `127.0.0.1` and `::1` for ports
   1455 then 1457.** The advertised redirect URI stays
   `http://localhost:{port}/auth/callback`. If either family is occupied, fall
   through to the next registered port; if both ports fail on both families,
   error text still contains `device-code`. Do not bind `:0`. Do not advertise
   `127.0.0.1` for OpenAI.

2. **D2 — Grok loopback answers CORS preflight and GET from
   `https://accounts.x.ai` with private-network permission.** OPTIONS and GET
   on `/callback` send `Access-Control-Allow-Origin: https://accounts.x.ai`,
   `Access-Control-Allow-Private-Network: true`, and the GET methods the
   official CLI allows. Do not wildcard origin. Redirect URI stays
   `http://127.0.0.1:{ephemeral}/callback`.

3. **D3 — Wizard always races loopback against paste-code.**
   `oauthFlowOptions` sets `InputCode` to a `Prompter.Input` that accepts a
   callback URL or a bare code (`parseOAuthInput` already does this). Notify
   the authorize URL **and** "paste the redirected URL here if the browser
   does not return" even when `OpenURL` is non-nil. `OpenURL` failure remains
   non-fatal.

4. **D4 — `OpenURL` must not own the login wait.** Launch the browser without
   waiting on redirected stdio (`Start` / equivalent). The 10-minute timeout
   select starts immediately. CombinedOutput is forbidden for this helper.

5. **D5 — Every callback outcome completes the waiter.** State mismatch,
   IdP `error`, missing `code`, and write failures send on the result
   channel. `TestOAuthCallback_RejectsStateMismatch` is rewritten to expect
   an error result, not a silent hang.

6. **D6 — No test may write `FileTokenStore` through the real Windows
   `UserConfigDir`.** `prepare-commit-msg` tests that call `NewOAuthStore` /
   `OAuthDir` / `config.Save` set `APPDATA` (and keep `HOME` /
   `USERPROFILE` / `XDG_CONFIG_HOME`). Add a test that fails if
   `os.UserConfigDir` is unchanged after the isolation helper runs on
   Windows.

7. **D7 — A saved OAuth session used for generation must be refreshable or
   an explicit access-only ChatGPT token, never a stub.** `ValidateOAuth`
   (and wizard `saveOAuthCredential`) reject empty access, reject
   `chatgpt-access`-class fixtures, and reject `Kind=oauth` with empty
   refresh **unless** the documented OpenAI `token_stdin` /
   `CODEX_ACCESS_TOKEN` access-only path set `Expiry` zero on purpose.
   Browser and device-code results must have non-empty `Refresh`,
   `ClientID`, and `TokenURL`.

8. **D8 — Token-endpoint failures include a truncated, redacted body** in
   the returned error (status + `logging.Redact` of the body, capped). Same
   for refresh.

9. **D9 — ChatGPT-mode generate sets `originator` on the Codex request** to
   the same `mcplib` value already used on the authorize URL. Keep
   `ChatGPT-Account-Id` and residency behaviour. Prove with a capturing
   RoundTripper. Live confirmation that the backend requires it is plan
   work once D1–D7 produce a real session (F9).

10. **D10 — Tests that must exist before this is closed:** OpenAI
    `LoginBrowserOAuth` loopback success (httptest IdP, real listener);
    `localhost` connect succeeds against the OpenAI listener on a host where
    `localhost` is `::1`; Grok OPTIONS `/callback` returns the D2 headers;
    wizard browser path invokes `InputCode`; `ValidateOAuth` rejects the
    live fixture shape; Windows isolation helper (D6). Deliberate red runs
    required.

11. **D11 — After ChatGPT OAuth, model listing is the Codex `/models`
    endpoint and nothing else.** `ListAvailableModelsWithSource` for an
    OpenAI ChatGPT session calls
    `{DefaultOpenAIChatGPTBaseURL}/models?client_version=0.0.0` with the
    session bearer, `originator=mcplib`, and `ChatGPT-Account-Id` when
    known. It never calls `api.openai.com`. Visibility other than `list`
    (or empty) is dropped; `supported_in_api=false` is dropped. A listing
    failure or empty usable set is an error; the wizard then prompts for a
    model id. Delete `StaticOpenAIChatGPT`. Do not substitute `StaticOpenAI`.
    API-key OpenAI and every other provider keep their existing static-catalog
    fallback. `DiscoverModels` for non-ChatGPT providers is unchanged.

### Consequences

* Good, because OpenAI browser login can complete on Windows IPv6-first
  `localhost` without changing the Hydra allow-list.
* Good, because Grok browser login matches the CORS contract the official
  CLI already had to add for `accounts.x.ai`.
* Good, because paste-code makes a missed callback a recoverable prompt
  instead of a 10-minute hang, including on this non-headless laptop.
* Good, because the live `chatgpt-access` class of store pollution becomes
  a failing test rather than a configured provider.
* Neutral, because device-code is unchanged except that it still must pass.
* Neutral, because import of `~/.codex/auth.json` remains available and is
  not the primary fix; F11 (import JWT `exp`) is in scope because D7
  validates the same OpenAI session type import saves.
* Bad, because dual-stack listen and CORS are new failure modes (permission
  prompts, firewall). Paste-code is the escape hatch; D3 is therefore not
  optional.
* Bad, because a live ChatGPT generate probe is still required to close F9.
  D9 ships the header Codex sends; it does not claim the backend was
  observed to require it.

### Confirmation

Compliance is confirmed by tests seen to fail on a deliberate wrong
implementation, plus one host probe:

* Bind OpenAI loopback, then `net.Dial("tcp", "localhost:"+port)` succeeds
  on this Windows host where `localhost` is `::1`. Temporarily binding
  only `127.0.0.1` makes that dial fail (the 2026-09-14 probe).
* Grok callback OPTIONS from origin `https://accounts.x.ai` returns 204/200
  with `Access-Control-Allow-Private-Network: true`. A handler without that
  header fails the test.
* Wizard browser OAuth with a scripted `InputCode` completing before
  loopback still `Save`s a session. A build that leaves `InputCode` nil
  fails.
* `TestOAuthCallback_RejectsStateMismatch` (renamed) receives an error on
  the channel within the test, not after a timeout.
* `go test` in `prepare-commit-msg` with a canary that reads the real
  `%APPDATA%\prepare-commit-msg\oauth\openai.json` access value must not
  observe `chatgpt-access` after the OAuth tests. Isolation helper test
  fails if `UserConfigDir` still equals the pre-test APPDATA.
* `ValidateOAuth` on a session with access `chatgpt-access` and empty
  refresh **fails**.
* ChatGPT generate capturing transport asserts `originator=mcplib`.
* ChatGPT `ListAvailableModelsWithSource` capturing transport asserts
  `GET …/models?client_version=0.0.0`, `originator=mcplib`, and a host that
  is not `api.openai.com`. Hidden slugs are omitted. A 502 returns an error
  and a nil slice, not `StaticOpenAI`.
* `StaticOpenAIChatGPT` does not compile. Wizard ChatGPT OAuth with a failed
  listing prompts for a model id and does not offer `gpt-5.4` or `gpt-4.1-mini`.
* Existing 0008 host-lock tests still pass (A1–A5, A11, A16, A17).
* `go test ./llmprovider ./wizard` and `prepare-commit-msg` `go test ./...`
  exit 0. No new `go.mod` require.

A live OpenAI browser login on this Windows laptop after D1–D7, producing
`oauth/openai.json` with JWT access, non-empty refresh, `client_id`, and
`account_id`, is the product confirmation. It is not a unit test.

## Pros and Cons of the Options

### A — Repair loopback, paste-code, isolation, validation (chosen)

* Good, because it attacks the measured Windows callback miss (F1) and the
  measured store pollution (F3), and it finishes 0008's paste-code (F4)
  and Grok's known CORS requirement (F5).
* Good, because device-code and API keys stay.
* Bad, because CORS and dual-stack are easy to implement incompletely;
  D10's red tests are the counter.

### B — Windows users must use device-code

* Good, because device-code already works here.
* Bad, because the user is on a laptop with a browser, selected browser
  login, and 0008 promised that path as the default. Documenting around a
  `localhost`/`::1` bug leaves OpenAI broken for every Windows consumer.

### C — Import-only vendor CLI files

* Good, because `~/.codex/auth.json` on this host is already a complete
  ChatGPT session.
* Bad, because 0008 rejected import-as-the-whole-design: MagicDev and
  prepare-commit-msg would still send people to another product's wizard.
  Import stays as one method.

### D — Shell out to official CLIs

* Good, because those CLIs already handle CORS, paste, and IPv6.
* Bad, because 0008 rejected wrapping `codex`/`grok` (PATH, version skew,
  nine headless servers). This debug pass does not reverse that.

## More Information

### Evidence index

| Claim | Source |
| --- | --- |
| OpenAI binds `127.0.0.1`, advertises `http://localhost:{port}/auth/callback` | `llmprovider/oauth_loopback.go` `browserListener` |
| This host `localhost` is `::1` then `127.0.0.1` | 2026-09-14 `Resolve-DnsName localhost` / `GetHostAddresses` |
| `localhost:1455` times out against an IPv4-only listener; `127.0.0.1:1455` succeeds | 2026-09-14 TcpClient probe while bound to `127.0.0.1:1455` |
| Live `openai.json` access is fixture `chatgpt-access`, no refresh/client_id | 2026-09-14 read of `%APPDATA%\prepare-commit-msg\oauth\openai.json` (prefix/suffix only) |
| That shape is written by `TestRunAnalyzer_OAuthDoesNotCallNewProviderWithAccessToken` | `prepare-commit-msg/main_oauth_test.go` lines 16–26 |
| Windows `UserConfigDir` ignores `HOME`/`XDG_CONFIG_HOME` | 2026-09-14 `os.UserConfigDir` probe |
| Live `grok.json` is a real JWT OIDC session | 2026-09-14 file shape (prefix `eyJ0`, refresh 86, default client id) |
| Grok CLI CORS + private-network + paste race | `grok-build` `login.rs` `build_callback_router`, `race_callback_and_stdin`; `config.rs` `PROD_ACCOUNTS_APP_ORIGINS` |
| `InputCode` never set by wizard | `wizard/auth.go` `oauthFlowOptions` |
| State mismatch does not complete the waiter | `oauth_loopback.go` `oauthCallbackHandler`; `TestOAuthCallback_RejectsStateMismatch` |
| `OpenURL` is `CombinedOutput` and runs before the timeout select | `internal/ui/open.go`; `LoginBrowserOAuth` |
| rundll32 CombinedOutput analogue returned in 169 ms on a dead URL | 2026-09-14 Process probe **[does not prove a real-browser hang]** |
| Codex always sends `originator` on HTTP | `codex-rs/login/src/auth/default_client.rs` `headers.insert("originator", ...)` |
| OpenCode sets `originator` on ChatGPT chat headers | `opencode/.../codex.ts` `output.headers.originator = "opencode"` |
| FileTokenStore overwrite works on this Windows host | 2026-09-14 `TestFileTokenStore_OverwriteExisting` PASS |
| `ValidateOAuth` only checks non-nil session + model | `prepare-commit-msg/internal/config/config.go` `ValidateOAuth` |
| Real Codex auth.json unused | 2026-09-14 `~/.codex/auth.json` token lengths |
| HEAD still IPv4-only OpenAI loopback; OpenURL synchronous | 2026-09-16 `git show HEAD:llmprovider/oauth_loopback.go` `listenFirstAvailable("127.0.0.1", openaiLoopbackPorts)` and `ignoreOAuthError(config.openURL(authorizeURL))` |
| Wizard still omits `InputCode` | 2026-09-16 `wizard/auth.go` `oauthFlowOptions` (unchanged vs HEAD) |
| `StaticOpenAIChatGPT` still in HEAD | 2026-09-16 `git show HEAD:llmprovider/models_catalog.go` |
| `prepare-commit-msg` OpenURL already `Start` | 2026-09-16 `internal/ui/open.go` `command.Start` |
| `main_oauth_test.go` still omits `APPDATA` | 2026-09-16 file lines 16–17 vs `config_test.go` `isolateHome` |
| Consumer precheck forbids replace / pseudo-version | 2026-09-16 `prepare-commit-msg/scripts/go-precheck.py` lines 4–8 |
| Next mcplib tag after v1.5.0 | 2026-09-16 `git tag --list 'v1.*'` |

### Related records

* [0008-MADR-subscription-auth-for-llm-providers.md](../0008-MADR-subscription-auth-for-llm-providers.md)
  (accepted) — the architecture this repair implements correctly.
* [0008-PLAN-subscription-auth-for-llm-providers.md](../0008-PLAN-subscription-auth-for-llm-providers.md)
  (complete) — Phase 5 loopback ports, Phase 8 paste-code, Phase 10
  `rundll32` OpenURL and `FileTokenStore`. Leftover: paste-code never
  wired; Windows `APPDATA` isolation missing from `main_oauth_test.go`;
  OpenAI listener family not specified.

### Open questions for the plan

1. Live ChatGPT generate after a real session exists: does omitting
   `originator` 401, and does `max_output_tokens` (always sent today;
   OpenCode clears it for Codex) 400? D9 ships the header; the plan pins
   both with a capturing test. The live probe is operator confirmation,
   not a unit test.
2. **Closed 2026-09-16.** `importOpenAIAuth` parses JWT `exp` from the
   access token when present (F11). Codex `last_refresh` is not in the
   vendor file this repo reads (`tokens.access_token` /
   `tokens.refresh_token` / `tokens.account_id` only).
3. After D6, delete or replace the polluted live `openai.json` as a
   documented recovery step, not as code.

## Amendment — 2026-09-14: keep ChatGPT browser login; dual-stack loopback

The owner asked whether **Sign in with ChatGPT** can be made to work, and if
so to fix it rather than remove the method. It can: OpenAI Hydra requires
`redirect_uri=http://localhost:{1455|1457}/auth/callback`, which this host
resolves as `::1` first, while the server only bound `127.0.0.1` (F1).
Listening on both `127.0.0.1` and `::1` for the same registered port keeps
the allow-list URI and accepts whichever family the browser dials. Go DNS
cannot substitute; the browser, not the hook, resolves `localhost`.

This amendment keeps option A and D1. It does **not** rip
`browser_oauth` off the OpenAI descriptor. Grok CORS/PNA (F5), paste-code
(F4), and APPDATA isolation (F3/D6) remain in the record and are not all
required to prove the ChatGPT callback path.

Shipped in this amendment's implementation:

* OpenAI loopback binds `tcp4 127.0.0.1` and `tcp6 ::1` on 1455 then 1457
* Callback state/code failures complete the waiter (F6)
* `OpenURL` is started in the background; prepare-commit-msg uses `Start`
  rather than `CombinedOutput` (D4)
* ChatGPT generate sets `originator: mcplib` (D9)

## Amendment — 2026-09-14: live ChatGPT model catalog after OAuth

0008 listed ChatGPT models from a static `gpt-5.4` / `gpt-5.4-mini` /
`gpt-5.3-codex` slice and forbade HTTP. A live generate on this host
(2026-09-14) returned HTTP 400 "model is not supported when using Codex
with a ChatGPT account" for all three. `GET
https://chatgpt.com/backend-api/codex/models?client_version=0.0.0` with the
stored ChatGPT session returned visibility=`list` slugs, in priority order:
`gpt-6-astra`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-5.5`.
Hidden entries (`gpt-reserve`, `codex-auto-review`) are omitted.

After ChatGPT OAuth, `ListAvailableModelsWithSource` calls that Codex
`/models` endpoint (never `api.openai.com`) and the wizard presents those
slugs. There is **no hardcoded ChatGPT model list**. A listing failure
does not substitute Platform `gpt-4.1-*` IDs or a frozen 5.x slice; the
menu is empty except "Other (enter a model id)".

This amendment is D11 / F12. It does **not** remove static-catalog
fallbacks for API-key OpenAI or for any other provider.

## Amendment — 2026-09-16: grounding against HEAD; working tree is not shipped

Re-read of `mcplib` HEAD `233999b` and sibling `prepare-commit-msg` on
2026-09-16. The 2026-09-14 "shipped in this amendment's implementation"
list is **working-tree only**. It is not in HEAD. Execution starts from
HEAD, not from the dirty blob.

HEAD still exhibits F1 (IPv4-only `listenFirstAvailable("127.0.0.1", …)`),
F4 (`wizard/auth.go` `oauthFlowOptions` omits `InputCode`), F5 (no CORS),
F6 (`TestOAuthCallback_RejectsStateMismatch` still requires a silent hang;
`LoginBrowserOAuth` still calls `OpenURL` synchronously), F7 (status-only
token errors), F8 (`ValidateOAuth` returns nil on any non-nil session),
F9 (no `originator` on generate), F11 (import `Expiry` zero), and F12
(`StaticOpenAIChatGPT` still compiled).

Measured on the sibling at the same time, not assumed from the 2026-09-14
text:

* `prepare-commit-msg/internal/ui/open.go` already uses `command.Start()`
  plus a Wait goroutine. D4's consumer helper is done there. D4's mcplib
  waiter is not.
* `config_test.go` `isolateHome` and `setup_test.go` `isolate` set `AppData`
  but `main_oauth_test.go` still sets only `HOME` / `XDG_CONFIG_HOME` and
  still writes `chatgpt-access` through `NewOAuthStore` (F3).
* `prepare-commit-msg` `scripts/go-precheck.py` rejects replace directives
  and pseudo-versions, so the consumer cannot pick up these mcplib commits
  until a tagged module version exists.

The uncommitted `llmprovider/` + `wizard/` diff also removes static
fallbacks from every `DiscoverModels` and from wizard `Discover: false`.
That is **out of D11**. D11 is ChatGPT-OAuth listing only.

## Amendment — 2026-09-27: `ba92db1` shipped the working-tree draft; `main` repaired

Commit `ba92db1` ("fix(oauth): repair loopback, paste fallback, and
ChatGPT session wiring") reached `origin/main` as a fast-forward. It
contains this pair and the 2026-09-16 working-tree draft that the PLAN's
C10 says must not be committed as one blob. The draft was applied with
unresolved conflict markers against `5a1fc70`:

* `llmprovider/discovery.go` holds 20 conflict blocks (60 marker lines,
  first at line 335). `wizard/configure_test.go` holds one (lines 244–247).
  At `ba92db1` and at `6e19cdf`, `go build ./...` fails with
  `llmprovider/discovery.go:335:1: syntax error: unexpected <<, expected }`,
  and `go test ./...` reports `llmprovider [build failed]` and
  `wizard [setup failed]`. The parent `ca29b81` has no markers.
* Each `<<<<<<< HEAD` side is the draft's pre-0009-refactor code. It calls
  `liveModels`, which no longer exists. Each `>>>>>>> 5a1fc70` side is the
  current code.
* The draft removed the static-catalog fallback from `DiscoverModels` in
  `claude.go`, `gemini.go`, `grok.go`, `huggingface.go`, `kilo.go` and
  `opencode.go`. It also rewrote the discovery and wizard tests for a wizard
  with no static menu. D11 and the PLAN's C1 reject both.
* It deleted `StaticOpenAIChatGPT` but left three references to it:
  `wizard/configure.go`, `llmprovider/discovery_catalog_test.go` and
  `wizard/model_select_test.go`.
* It added `Options.HTTPClient` without passing it to the listing. It also
  added a second `WithBaseURL` fed from `Existing.BaseURL`, which aims a new
  provider's listing at the previous provider's endpoint. `resolveBaseURL`
  already carries a same-provider base URL.

The decision is unchanged, and D11 stands as written. PLAN phase R1 restores
`main` to D11 and C1:

* Every conflict takes the current side, so `discovery.go` is `ca29b81` plus
  only the D11 Codex lister.
* The six `DiscoverModels` files and the rewritten wizard tests return to
  their `ca29b81` content.
* The wizard's half of P7 is finished: a ChatGPT session has no static
  catalog, and `Options.HTTPClient` is wired.

At `ba92db1`, these decisions have landed: D1, D4, D5 (state mismatch and
missing code), D9 and the `llmprovider` half of D11. D2, D3, D7, D8 and F11
have not. They remain PLAN work, as do P1's and P4's missing tests.

## Amendment — 2026-09-27: how D1 and D7 are confirmed

**D1's confirmation.** The first Confirmation bullet assumed a Go `localhost`
dial behaves like a browser's. It does not. On a macOS host where
`localhost` resolves to `::1` first, Go's dialer falls back to `127.0.0.1`
and reaches an IPv4-only listener. D1 is therefore confirmed with explicit
dials:
* an IPv4-only listener refuses `tcp6 [::1]`;
* the dual-stack helper accepts `tcp6 [::1]`.

**D7's refreshable-session rule.** It requires a `ClientID` but not a
`TokenURL`, because a kept session has none and the refresh derives it
from the issuer. Fresh browser and device-code results still carry
`TokenURL`, as D7 states. Both changes are in the PLAN's deviation log.

## Amendment — 2026-09-27: D3's race must leave one reader

D3 races the loopback against a paste prompt, and the PLAN's C6 accepted a
paste prompt left blocked after the loopback wins. With `TextPrompter`, that
leftover is not harmless:
* every read shares one unlocked `bufio.Reader`;
* the next wizard prompt then races it for the user's next line.

D3 now also requires the wizard to drain that prompt:
* The paste prompt starts only after the authorize URL and the paste
  instruction are shown.
* A prompt still waiting when the login returns is finished by one Enter,
  asked for as "Browser sign-in finished; press Enter to continue", before
  any other prompt.

`Prompter` is unchanged.

## Amendment — 2026-09-27: D1's premise and D7's `CODEX_ACCESS_TOKEN`

Both are decided in
[0012-MADR-conform-providers-to-reference-clients.md](../0012-MADR-conform-providers-to-reference-clients.md)
revision 2. The owner decided them on 2026-09-27.

* **D1.** The premise that Hydra requires a `localhost` redirect no longer
  holds. Codex switched to `http://127.0.0.1:{port}/auth/callback` on
  2026-09-24, with the same client id. `mcplib` follows it once a live login
  confirms the switch, and the dual-stack listener stays until then.
* **D7.** `CODEX_ACCESS_TOKEN` is withdrawn, because Codex treats it as a
  personal access token or an agent JWT. The access-only ChatGPT path stays
  for a token pasted on stdin only.
