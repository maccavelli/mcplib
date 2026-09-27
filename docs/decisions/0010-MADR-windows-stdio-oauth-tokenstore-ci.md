---
status: proposed
date: 2026-09-20
decision-makers: mcplib maintainers
consulted: prepare-commit-msg
informed: all mcplib consumers
---

<!-- markdownlint-disable MD013 MD024 MD033 MD036 MD060 -->

# Close Windows stdio shutdown misclassification, finish the mcplib remainder of 0009, harden FileTokenStore, and make CI prove it on macOS, Linux, and Windows

## Context and Problem Statement

mcplib already runs `go test ./...` on `ubuntu-24.04`, `macos-15`, and
`windows-2025`. That matrix compiles the Windows-tagged self-update and token-store
files, and it is how 0005's native replace path was proven. It does not prove
that the library behaves correctly on Windows in production, and it does not
prove the remaining 0009 OAuth wiring.

A 2026-09-20 debugging pass on this tree, against the live sources cited in
the Evidence index, found four defects that share a host (Windows) and a
verification hole (CI that treats a green `go test` as platform parity):

1. **Stdio shutdown classification is POSIX-shaped.** Fleet servers call
   `IsExpectedShutdownErr` to ignore a benign IDE detach. On Windows the OS
   errors that actually arrive are `ERROR_BROKEN_PIPE` / `WSAECONNRESET`, whose
   messages are `The pipe has been ended.` and `An existing connection was
   forcibly closed by the remote host.` The classifier matches `syscall.EPIPE`
   (a synthetic Windows errno) and the English phrase `"broken pipe"`. Those
   never meet. A normal stdio shutdown on Windows is reported as an unexpected
   write failure.

2. **MADR 0009 is half in the tree.** Dual-stack OpenAI loopback (0009 D1),
   background `OpenURL` (D4), waiter completion (D5), ChatGPT `originator` on
   generate (D9), and the Codex `/models` listing (D11) are at HEAD.
   Grok CORS (D2), wizard paste-code (D3), token-endpoint error bodies (D8),
   and session validation plus import `exp` (D7, F11) are not. Browser login
   on this Windows host is still the 0009 problem statement.

3. **OAuth session files on Windows have no confidentiality control.**
   `chmod0600` is a no-op. `MkdirAll(..., 0o700)` is ignored. `selfupdate`
   already restricts a file to the current user with a protected DACL;
   `FileTokenStore` does not. Provider ids `CON` / `PRN` / `NUL` are legal.
   `Save` never `Sync`s before rename.

4. **CI vets, formats, and lints only on Linux; never races; never runs
   govulncheck; never cross-compiles the published GOOS/GOARCH set; and
   invokes bash scripts on the Windows matrix job with the runner default
   shell (PowerShell).** Windows-tagged production files are compiled by
   `go test` and never `go vet`'d. Several tests comment "run under -race"
   and CI never does.

0002's `paths` package, 0009 P8 (`prepare-commit-msg` APPDATA isolation and
consumer `ValidateOAuth`), TextPrompter raw-mode CR/LF, HFSC abort-on-log-
failure, diagnostic substring isolation, and self-update junction handling
are adjacent and **out of this decision**. This record is the four slices
above, in this repository.

0009 remains the OAuth-loopback decision record. This record does not
supersede it and does not edit it. It absorbs the unexecuted *mcplib*
remainder (0009 D2, D3, D7, D8, and confirmation of D11) so `oauth_loopback.go`
has one owner for the next commits, and it adds the shutdown, token-store,
and CI commitments 0009 never made. 0009 D6 / P8 stay in 0009; they live in
a sibling repository.

### What was measured, not assumed

**Windows `syscall.EPIPE` is not a pipe error.**
`GOOS=windows go doc syscall.EPIPE` on 2026-09-20 prints

```text
EPIPE Errno = APPLICATION_ERROR + iota
```

`APPLICATION_ERROR` is `1 << 29` in the Windows `syscall` package. The
constant exists so POSIX names compile; `errors.Is(err, syscall.EPIPE)` is
true only for that synthetic value. The Win32 codes that a closed stdout
pipe actually carries live in `golang.org/x/sys/windows`:

| Constant | Value | Documented English `Error()` |
| --- | --- | --- |
| `ERROR_BROKEN_PIPE` | 109 | `The pipe has been ended.` |
| `ERROR_NO_DATA` | 232 | `The pipe is being closed.` |
| `WSAECONNRESET` | 10054 | `An existing connection was forcibly closed by the remote host.` |
| `WSAECONNABORTED` | 10053 | `An established connection was aborted by the software in your host machine.` |

`syscall` on Windows does not export `ERROR_BROKEN_PIPE` or `WSAECONNRESET`
(`GOOS=windows go doc syscall.ERROR_BROKEN_PIPE` / `syscall.WSAECONNRESET`
returned `no symbol`). `x/sys/windows` does, and is already a direct
`go.mod` require (`golang.org/x/sys v0.47.0`).

**The production classifier and its test.**
`IsExpectedShutdownErr` (`stdio.go:185–208`) returns true for
`syscall.EPIPE`, `syscall.ECONNRESET`, and substring matches including
`"broken pipe"` and `"connection reset"`. It does not call
`errors.Is` against any `windows.ERROR_*` / `windows.WSAE*` value. The
fallback phrases do not occur in the four documented messages above.

`TestIsExpectedShutdownErr_Typed` (`transport_extra_test.go:45–58`) passes
`syscall.EPIPE` and `syscall.ECONNRESET` into that function. On Windows
those are the same synthetic constants the production `errors.Is` checks,
so the test is green on `windows-2025` and does not construct
`windows.ERROR_BROKEN_PIPE`. This is a check that has only been observed
passing against itself.

A wrapping `&os.PathError{Op: "write", Path: "stdout", Err: windows.ERROR_BROKEN_PIPE}`
is the shape `os`/`net` actually return. That value, run through HEAD
`IsExpectedShutdownErr`, is **[unverified in this pass as a live `go test`
FAIL line]** — the typed analysis plus the documented strings are the
evidence; the PLAN's first shutdown phase must capture the FAIL against
HEAD before changing `stdio.go`.

**0009 remainder, file by file, 2026-09-20 HEAD.**

| 0009 decision | HEAD |
| --- | --- |
| D1 dual-stack OpenAI loopback | Present. `listenLoopbackBothFamilies` in `oauth_loopback.go:274–300`; `browserListener` advertises `http://localhost:%d/auth/callback`; `TestListenLoopbackBothFamilies_LocalhostDials` exists. |
| D4 background `OpenURL` | Present. `oauth_loopback.go:109–113` launches `openURL` in a goroutine. |
| D5 waiter completion | Present. `oauthCallbackHandler` sends on `result` for state mismatch, IdP `error`, missing `code`, write failure, and success (`oauth_loopback.go:388–431`). `TestOAuthCallback_RejectsStateMismatch` expects the error. |
| D9 ChatGPT `originator` on generate | Present. `openai.go:168–169` sets `openAIOriginatorHeader` when `p.chatGPT`. Covered by `openai_chatgpt_test.go`. |
| D11 Codex `/models`, no `StaticOpenAIChatGPT` | Present. `ListAvailableModelsWithSource` branches on `isChatGPTTokenSource` to `listChatGPTModels` (`discovery.go:39–40`, `:123–157`) with `originator` and `client_version=0.0.0`. `StaticOpenAIChatGPT` has zero Go references. Wizard `discoverModels` (`configure.go:258–298`) returns nil on listing failure and does not substitute `StaticOpenAI`. |
| D2 Grok CORS | Absent. No `Access-Control-Allow-Origin`, no `Access-Control-Allow-Private-Network`, no OPTIONS branch. `oauthCallbackHandler` is `mux.HandleFunc(path, …)` with no method prefix, so an OPTIONS preflight is the same handler. Empty query `state` ≠ expected state → HTTP 400 **and** `oauth: callback state mismatch` on the result channel (`oauth_loopback.go:392–398`). A Chrome private-network preflight today can abort the login. |
| D3 paste-code | Absent. `wizard/auth.go:216–229` `oauthFlowOptions` sets `OpenURL` and `NotifyDevice` only. `InputCode` stays nil. Library `LoginBrowserOAuth` already races `InputCode` when set (`oauth_loopback.go:116–129`). |
| D8 error bodies | Absent. `exchangeOAuthCode` returns `oauth: token exchange failed: %s` with `resp.Status` (`oauth_loopback.go:481`). `refreshOAuthSession` returns `oauth: refresh failed: %s` (`oauth_session.go:155–159`). Neither reads the body. |
| D7 stub rejection | Absent in mcplib. There is no `ValidateOAuthSession`. `saveOAuthCredential` (`wizard/auth.go:232–247`) saves any non-nil session. `TestConfigureLLM_TokenStdinClassification` still treats access `chatgpt-access` as a successful OAuth save (`wizard/auth_test.go:134–139`). |
| F11 import `exp` | Absent. `importOpenAIAuth` (`wizard/import.go:78–94`) copies access, refresh, account id, issuer, client id, token URL, and leaves `Expiry` at zero. Grok import already parses `expires_at`. |
| D6 / P8 consumer isolation | Out of this repository (`prepare-commit-msg`). |

**Token store, 2026-09-20 HEAD.**

* `tokenstore_file_windows.go` is four lines: `chmod0600` returns nil.
* `tokenstore_file.go` `Save` writes the temp file, `Close`s, `Rename`s,
  then `chmod0600`. There is no `Sync`. `ctx` is unused.
* `validateProviderID` (`tokenstore.go:60–67`) rejects empty, `/\`, and
  `".."`. It accepts `CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`,
  `LPT1`–`LPT9`. Microsoft documents those names, and the same names with
  an extension (`NUL.txt`), as reserved device names.
* Unix 0600 is tested in `tokenstore_file_unix_test.go` (`//go:build unix`).
  There is no Windows ACL test, no reserved-name test, no overwrite test
  in tree (0009 cited a throwaway `TestFileTokenStore_OverwriteExisting`
  that was not committed).
* `selfupdate/cleanup_windows.go:122–170` `restrictToCurrentUser` already
  implements the DACL this store needs: current-user `GENERIC_ALL`,
  `PROTECTED_DACL_SECURITY_INFORMATION`, owner = current SID. It is
  unexported in package `selfupdate`. `llmprovider` must not import
  `selfupdate`.

**OAuth refresh persist, 2026-09-20 HEAD.**
`OAuthSession.Token` (`oauth_session.go:52–67`) refreshes, then `Save`s,
then `adopt`s only when `err == nil`. A successful IdP rotation followed
by a disk error leaves `s.Refresh` as the previous (now invalid) refresh
token. The next `Token()` retries with the dead refresh. This is in
`oauth_session.go`, which D8 already has to edit.

**CI, 2026-09-20 HEAD (`.github/workflows/ci.yml`).**

```yaml
strategy.matrix.os: [ubuntu-24.04, macos-15, windows-2025]
fail-fast: false
# every OS:
- go test ./...
# Linux only:
- go vet ./...
- test -z "$(gofmt -l .)"
- go mod tidy -diff
- golangci-lint v2.13.1 via make lint
- ./scripts/verify-selfupdate-release_test.sh
# every OS, no shell: key:
- ./scripts/refuse-existing-release_test.sh
- ./scripts/check-workflow-gh-repo.sh
```

There is no `-race`, no `govulncheck`, no `GOOS`/`GOARCH` matrix.
`make vuln` exists in the Makefile and is opt-in. MADR 0006 raised the
toolchain to 1.26.6 *because* govulncheck reported reachable stdlib
advisories, and then left `make vuln` off CI.

Native GOOS/GOARCH the matrix actually executes:

| Runner | What `go test` runs |
| --- | --- |
| ubuntu-24.04 | linux/amd64 |
| macos-15 | darwin/arm64 |
| windows-2025 | windows/amd64 |

`linux/arm64`, `darwin/amd64`, and `windows/arm64` are in the self-update
asset contract (0005) and are never compiled here.

GitHub-hosted Windows runners default `run:` to PowerShell.
`verify-selfupdate-release_test.sh` is already gated `if: runner.os == 'Linux'`.
The release-guard step is not. GitHub-hosted Windows images include Git Bash;
the contract is `shell: bash`, not an implicit association for `./file.sh`.

The Go race detector documents support for `linux/amd64`, `darwin/arm64`,
and `windows/amd64` — exactly this matrix. Wall-clock cost of `-race` on
`windows-2025` for this module is **[unverified]**; the PLAN captures it
on the first race job.

**x/sys is already a module requirement.** Token-store ACLs and the
Windows shutdown helper add no new `go.mod` require.

**Vendor trees, 2026-09-20, sibling checkouts (read-only).**
Pinned so this record does not float against `main`:

| Tree | HEAD | Subject date |
| --- | --- | --- |
| `grok-build` | `4247f661` | 2026-09-19 |
| `codex` | `ac192cd79` | 2026-09-06 |

These are the first-party CLIs 0008/0009 treated as the loopback and
session contracts. They confirm which 0010 decisions copy a vendor
behaviour, and which 0009 decisions are a measured Windows delta *from*
the vendor.

*Grok (`crates/codegen/xai-grok-login`, plus `xai-grok-shell-base`).*

* Loopback binds `127.0.0.1:{0}` (ephemeral; local-dev uses 56121) and
  advertises `http://127.0.0.1:{port}/callback`
  (`oidc/login.rs:376–387`). IPv4 literal. RFC 8252. Matches mcplib's
  Grok listener. CORS exists *because* the consent page is not that
  host: it is served from `accounts.x.ai` and `fetch`es the loopback.
* CORS is a frozen contract. `PROD_ACCOUNTS_APP_ORIGINS =
  ["https://accounts.x.ai"]` (`config.rs:124–131`), locked by
  `allowed_accounts_app_origins_are_frozen` (`config.rs:411–417`) with
  the comment that the consent page delivers the code via
  `fetch(..., cors)` and removing an origin breaks already-installed
  CLIs. The callback router is
  `.route("/callback", get(handle_callback)).layer(cors)` where
  `cors = accounts_app_cors_layer(Method::GET).allow_private_network(true)`
  (`oidc/login.rs:112–119`). `accounts_app_cors_layer` allow-lists that
  origin and that method (`config.rs:133–147`). Axum's `CorsLayer`
  answers OPTIONS; the GET handler never sees a preflight. That is the
  opposite of mcplib HEAD, where `HandleFunc(path, …)` with no method
  prefix treats OPTIONS as a callback with empty state (F4).
* Paste is a first-class race, timeout 600 s (same 10 minutes as
  `oauthBrowserTimeout`). Module docs: Path A loopback, Path B stdin
  paste, "essential for remote VMs where the browser runs on a different
  machine" (`oidc/login.rs:1`, `:26–28`, `:355–357`). CLI prompt:
  `Paste the URL here if it doesn't connect:` (`oidc/login.rs:431`).
  Parser accepts a full `http://127.0.0.1:{port}/callback?code=&state=`
  URL, a bare code (state skipped on empty), and an `error=` URL
  (`parse_pasted_input`, matrix test `oidc/login.rs:643–676`). Matches
  mcplib `parseOAuthInput`; the missing piece is wiring `InputCode` (F5).
* Token-endpoint failures carry the body:
  `OIDC token exchange failed: HTTP {status} — {body}` and the refresh
  twin (`oidc/protocol.rs:35–38`, `exchange_code` at `:400–406` reads
  `resp.text()`). Grounds D5.
* Session files are owner-only **and** `fsync`'d. `write_store_to`
  (`storage.rs:210–237`) opens via `open_secure_file`, flushes, then
  `sync_all()`, then `ensure_owner_only_permissions`. The helper
  (`xai-grok-shell-base/src/util/secure_file.rs:1–10`, `:86–172`)
  documents Unix 0o600 / Windows ACL granting access only to the current
  user, `GENERIC_ALL`, `PROTECTED_DACL_SECURITY_INFORMATION`, no
  inherited ACEs. Same recipe as `selfupdate.restrictToCurrentUser`
  (Grok does not also `SetOwner`; selfupdate does). Grounds D10 and D11.
  Grok's comment: "The data is stored in plaintext; OS file permissions
  are the only protection" — no keyring, matching 0008/0009.
* `webbrowser::open` failure is logged and the flow continues
  (`oidc/login.rs:408–423`). Matches 0009 D4 / D3 `OpenURL` non-fatal.
* OAuth2 default scopes at this SHA include
  `conversations:read|write` and `workspaces:read|write` on top of
  mcplib's `grokOAuthScopes` (`config.rs:14–26` vs
  `llmprovider/oauth_loopback.go:25`). **Out of this record.** 0008
  froze the six-scope string; widening it is a separate decision.

*Codex (`codex-rs/login`, `codex-rs/codex-api`).*

* Hydra redirect is `http://localhost:{1455|1457}/auth/callback`
  (`login/src/server.rs:59–62`, `:176`). Authorize query includes
  `originator`, `codex_cli_simplified_flow=true`,
  `id_token_add_organizations=true`, and the same scope string mcplib
  already sends (`server.rs:584–611` vs `oauth_loopback.go:24`).
* Bind is **IPv4-only**: `Server::http("127.0.0.1:{port}")`, then the
  same on 1457 (`server.rs:637–694`). E2E hits
  `http://127.0.0.1:{port}/auth/callback` while the advertised host is
  `localhost` (`login/tests/suite/login_server_e2e.rs:25–26`, `:155–161`).
  This is 0009 F1's vendor shape. Dual-stack (already at mcplib HEAD,
  0009 D1) is a Windows-measured delta from Codex, not a Codex copy.
  0010 does not revert it.
* Every default Codex HTTP client inserts `originator` (`DEFAULT_ORIGINATOR
  = "codex_cli_rs"`) plus optional
  `x-openai-internal-codex-residency` (`auth/default_client.rs:40–42`,
  `:335–350`). Authorize URL also sets `originator` (`server.rs:601`).
  Grounds 0009 D9 / 0010 D8. mcplib's value remains `mcplib`.
* ChatGPT models listing is `GET {chatgpt backend}/models` with
  `client_version=` (`codex-api/src/endpoint/models.rs:31–43`; tests
  pin `https://chatgpt.com/backend-api/codex/models`). Grounds D8.
* No paste-code race in `codex-rs/login`. Recovery is the 1457 fallback
  port and a `/cancel` probe of a previous instance. Paste-code (D4)
  follows Grok, not Codex.
* `FileAuthStorage::save` sets unix `0o600` and `flush`s; there is no
  Windows ACL and no `sync_all` (`login/src/auth/storage.rs:206–222`).
  Codex offers a keyring backend; 0008/0009 already rejected a keyring
  in mcplib. D10/D11 follow Grok's file ACL + fsync, which is also what
  mcplib `selfupdate` already does.

### Findings

1. **F1 — `IsExpectedShutdownErr` cannot match a real Windows broken pipe.**
   Production checks `syscall.EPIPE` / `"broken pipe"`; the OS delivers
   `ERROR_BROKEN_PIPE` / `The pipe has been ended.` Consequence: every
   consumer that uses this classifier on Windows treats IDE detach as a
   hard write error.

2. **F2 — The shutdown test cannot fail on Windows for F1.**
   `TestIsExpectedShutdownErr_Typed` feeds the function its own POSIX
   constants. Consequence: `windows-2025` CI stays green for a classifier
   that is wrong in production. A new check is not trusted until it has
   been seen to fail against `windows.ERROR_BROKEN_PIPE` on HEAD.

3. **F3 — 0009 D1, D4, D5, D9, and D11 are already in this tree.**
   Consequence: this record must not rewrite loopback, `OpenURL`, waiter
   completion, generate `originator`, or the ChatGPT catalog. It confirms
   D11 with tests and implements the remainder.

4. **F4 — Grok CORS is unimplemented, and OPTIONS is harmful.**
   `oauthCallbackHandler` has no CORS headers. Because the mux pattern has
   no method prefix, OPTIONS is classified as a callback with a missing
   state and completes the waiter with `state mismatch`. Consequence: a
   private-network preflight from `https://accounts.x.ai` can fail the
   login; a missed GET still has no paste-code escape hatch (F5).

5. **F5 — Wizard never sets `InputCode`.**
   Library support exists; `oauthFlowOptions` does not use it. Consequence:
   a missed loopback is a 10-minute timeout. This is 0008's specified
   paste-code path, still unwired.

6. **F6 — Token-endpoint and refresh errors are status-only.**
   `400 Bad Request` with no IdP body. Consequence: a rejected exchange is
   undiagnosable, which is 0009 F7 still open.

7. **F7 — `saveOAuthCredential` accepts the `chatgpt-access` fixture.**
   Empty refresh, empty client id, 14-character access all save. The
   wizard test locks that in. Consequence: configure reports OAuth success
   for a session generation cannot spend (`oauth: no refresh token`).

8. **F8 — OpenAI vendor import leaves `Expiry` zero.**
   `Token()` treats zero expiry as not-expired (`oauth_session.go`
   `currentToken`), so a near-dead imported JWT is spent until a 401.
   Grok import already parses expiry.

9. **F9 — Windows token files have no mode and no DACL restriction.**
   `chmod0600` is `return nil`. Session JSON (refresh tokens) inherits the
   directory ACL (user + Administrators + SYSTEM is typical).
   `restrictToCurrentUser` already exists next door in `selfupdate`.

10. **F10 — `FileTokenStore.Save` never `Sync`s.**
    A crash between `Write` and `Rename` can leave a truncated session
    file that `Load` will decode-fail or, worse, decode as partial JSON.
    Self-update already `Sync`s staging and receipts.

11. **F11 — `validateProviderID` allows Windows reserved device names.**
    `Save("CON")` / `Save("NUL")` can address the console device instead
    of a file in the store directory.

12. **F12 — Windows token-store behaviour has no test in tree.**
    Unix 0600 is locked. Overwrite, ACL, reserved names, and `Sync` are
    not. 0009's throwaway overwrite test was not committed.

13. **F13 — `go vet` (and lint/gofmt/tidy) run only on Linux.**
    `lock_windows.go`, `replace_windows.go`, `cleanup_windows.go`,
    `tokenstore_file_windows.go` compile on `windows-2025` and are never
    vetted.

14. **F14 — CI never runs `-race`.**
    `async_writer_test.go` and `recall_client_test.go` say to. The race
    detector supports every OS in the matrix.

15. **F15 — `govulncheck` is Makefile-only.**
    MADR 0006's reason for the 1.26.6 floor is not a CI gate.

16. **F16 — The release-guard shell tests run on Windows without `shell: bash`.**
    The sibling `verify-selfupdate-release_test.sh` step is already
    Linux-only, which shows the authors knew this class of problem.

17. **F17 — The published GOOS/GOARCH set is not compiled.**
    Asset names include `windows/arm64` and `linux/arm64`. CI never
    `GOOS=… GOARCH=… go build ./...`.

18. **F18 — A failed `Save` after a successful refresh drops the new tokens.**
    `Token()` adopts only when `Save` returns nil. Refresh-token rotation
    plus a transient disk error bricks the in-memory session. This is in
    the same function D8 has to touch.

## Decision Drivers

* A classifier that is green on `windows-2025` and wrong on a real pipe
  close is worse than no classifier: consumers trust it to keep shutdown
  quiet.
* 0009 already decided CORS, paste-code, error bodies, and stub rejection.
  Re-litigating those in a third OAuth MADR would split `oauth_loopback.go`
  across two proposed records. Absorbing the mcplib remainder here keeps
  one commit series and leaves 0009's P8 in the sibling repo.
* Refresh tokens on disk are as sensitive as API keys. Unix 0600 is the
  existing contract; Windows must have an equivalent, and the equivalent
  already exists in-tree as `restrictToCurrentUser`.
* `FileTokenStore` is the only persistence mcplib owns. Reserved device
  names and a missing `Sync` are defects in that persistence, not consumer
  bugs.
* CI that compiles Windows files and vets only Linux is how F1's test
  stayed green. The matrix OS list is the right skeleton; the gates on
  each OS are the missing half.
* No new Go module. `golang.org/x/sys` is already required. No OS keyring
  (0008 / 0009). No Claude/Gemini OAuth. No 0002 `paths` package.

## Considered Options

* **A — One mcplib MADR covering shutdown classification, the 0009 mcplib
  remainder, FileTokenStore Windows confidentiality, and CI parity**
  (chosen)
* **B — Four separate MADRs, one per slice**
* **C — Amend 0009 to add shutdown, token ACL, and CI**
* **D — CI-only until 0009 is fully executed, then a follow-up Windows MADR**
* **E — Document "Windows shutdown errors are noisy" and "use device-code
  on Windows"; leave the classifier and 0009 remainder**

## Decision Outcome

Chosen option: **A**.

The four slices share the Windows production path and the CI that has to
prove it. Splitting them (B) interleaves `oauth_loopback.go`,
`oauth_session.go`, `tokenstore_file.go`, and `.github/workflows/ci.yml`
across four numbers. Amending 0009 (C) would silently widen a record whose
problem statement is ChatGPT/Grok browser login. CI-only (D) would merge
gates that cannot yet fail on F1/F4/F5/F9. Documenting around the defects
(E) leaves every Windows consumer with a noisy detach and a hanging
browser login.

0009 is not superseded. After this record is executed, 0009's remaining
work in *this* repository is done; 0009 P8 (`prepare-commit-msg`) is still
0009's to run.

### The decisions

1. **D1 — `IsExpectedShutdownErr` recognises the Windows pipe and reset
   errors by typed `errors.Is`, via a build-tagged helper.**
   Untagged `stdio.go` keeps the POSIX `syscall.EPIPE` /
   `syscall.ECONNRESET` checks (they are correct on unix). A new
   `expectedOSShutdown(error) bool` is implemented in
   `stdio_windows.go` (`//go:build windows`) and `stdio_other.go`
   (`//go:build !windows`). The Windows implementation returns true for
   `windows.ERROR_BROKEN_PIPE`, `windows.ERROR_NO_DATA`,
   `windows.WSAECONNRESET`, and `windows.WSAECONNABORTED`, including when
   wrapped in `*os.PathError` / `*os.SyscallError` / `*net.OpError`
   (`errors.Is` already unwraps those). The unix/other implementation
   returns false; POSIX matching stays in the untagged function.
   Secondary string fallbacks `"pipe has been ended"` and `"pipe is being
   closed"` are added for wrapped errors that have lost their type. They
   are not the primary contract — English `FormatMessage` is locale-
   dependent. `golang.org/x/sys/windows` is imported only from the
   Windows-tagged file. Existing `"broken pipe"` / `"connection reset"`
   phrases stay.

2. **D2 — A Windows-tagged test constructs real Win32 errors and is seen
   to fail on HEAD before D1 is implemented.**
   `stdio_windows_test.go` (`//go:build windows`) asserts
   `IsExpectedShutdownErr` is true for:
   * `&os.PathError{Op: "write", Path: "stdout", Err: windows.ERROR_BROKEN_PIPE}`
   * `windows.ERROR_NO_DATA`
   * `&net.OpError{Err: windows.WSAECONNRESET}` (or `os.SyscallError`
     wrapping it)
   * `windows.WSAECONNABORTED`
   and false for an unrelated Win32 error (e.g. `windows.ERROR_ACCESS_DENIED`
   as a bare value, still subject to the existing `"use of closed"` string
   path — pick a code whose `Error()` contains none of the fallback
   phrases, such as `windows.ERROR_INVALID_FUNCTION`).
   The PLAN records the FAIL line against unmodified `stdio.go`, then the
   PASS line after D1. `TestIsExpectedShutdownErr_Typed` stays; it remains
   the POSIX-constant test.

3. **D3 — Implement 0009 D2 in this tree: Grok CORS and private-network
   preflight.**
   Restate, do not change, 0009 D2:
   * Constant origin `https://accounts.x.ai` (no trailing slash, no
     wildcard, no `auth.x.ai`).
   * OPTIONS and GET on `/callback` send
     `Access-Control-Allow-Origin: https://accounts.x.ai`,
     `Access-Control-Allow-Private-Network: true`, and
     `Access-Control-Allow-Methods` containing `GET`, when `Origin` is
     that value byte-for-byte.
   * OPTIONS does not inspect `state`/`code` and does not send on the
     result channel. Status 204 (or 200).
   * A non-matching `Origin` gets no `Access-Control-Allow-Origin`.
   * OpenAI `/auth/callback` does not advertise the Grok origin.
   * Grok redirect URI remains `http://127.0.0.1:{ephemeral}/callback`.
   The handler signature gains an explicit CORS origin argument so OPTIONS
   cannot fall into the GET callback logic (closes F4's waiter abort).

4. **D4 — Implement 0009 D3 in this tree: wizard always races paste-code.**
   `oauthFlowOptions` sets `InputCode` to `Prompter.Input` with a prompt
   that tells the user to paste the redirected URL or authorization code
   if the browser does not return. `parseOAuthInput` stays the parser.
   Notify the authorize URL **and** the paste instruction even when the
   consumer supplied `OpenURL`. The paste notify text follows Grok CLI
   (`Paste the URL here if it doesn't connect:` at grok-build
   `oidc/login.rs:431`). `OpenURL` failure remains non-fatal
   (`ignoreOAuthError`). `Prompter` is unchanged (no `context` on `Input`).
   Device-code may see a non-nil `InputCode` and must ignore it
   (`LoginDeviceOAuth` already does).

5. **D5 — Implement 0009 D8 in this tree: token-endpoint and refresh
   errors include a truncated, redacted body.**
   Shared unexported helper, cap **2048** bytes, `logging.RedactString`,
   format `oauth: %s failed: %s: %s` (operation, status, redacted body).
   Used by `exchangeOAuthCode` and `refreshOAuthSession` on non-success.
   The raw body is never logged. Tests use fixture strings, never a live
   JWT; a JWT-shaped fixture in the body must appear as `[REDACTED]` in
   the error.

6. **D6 — Implement 0009 D7 in this tree (mcplib half): a saved session
   used for generation is refreshable, or an explicit ChatGPT access-only
   token, never a stub.**
   Exported `ValidateOAuthSession(*OAuthSession) error` in `llmprovider`:
   * empty Access → error
   * Access equal to `chatgpt-access` (and the same class: 14-character
     fixture with no refresh) → error
   * empty Refresh, for any issuer other than the documented ChatGPT
     access-only path → error
   * ChatGPT access-only: `Issuer == DefaultOpenAIIssuer`,
     `ClientID == DefaultOpenAIClientID`, empty Refresh, **zero** Expiry,
     Access a three-segment JWT-shaped string → nil
   * ChatGPT access-only with **non-zero** Expiry → error
   * Browser/device-code sessions require non-empty Refresh, ClientID, and
     TokenURL
   `saveOAuthCredential` calls it before `Save`. The wizard test that
   currently succeeds with `chatgpt-access` is rewritten to expect error
   and zero saves; a JWT-shaped access-only case replaces it as the
   success path. Consumer `ValidateOAuth` in `prepare-commit-msg` remains
   0009 P8.

7. **D7 — Implement 0009 F11: OpenAI vendor import sets `Expiry` from JWT
   `exp` when present.**
   `importOpenAIAuth` continues to require access and refresh. When the
   access token is a JWT with a numeric `exp` claim, `Expiry` is that
   unix time in UTC. Missing or unparsable `exp` leaves `Expiry` zero
   (refreshable session; `Token()` will refresh on 401). Grok import's
   `expires_at` parser stays.

8. **D8 — Confirm 0009 D11 at HEAD; do not re-implement the catalog.**
   `StaticOpenAIChatGPT` remains deleted. ChatGPT listing remains
   `listChatGPTModels` against `DefaultOpenAIChatGPTBaseURL` with
   `originator=mcplib` and `client_version=0.0.0`, and never
   `api.openai.com`. A listing failure or empty usable set remains an
   error that makes the wizard prompt for a model id, without
   substituting `StaticOpenAI`. API-key OpenAI and every other provider
   keep their existing static-catalog fallback. The PLAN adds or keeps
   capturing tests for those assertions and does not rewrite
   `discoverModels` unless a test proves a regression against D11.

9. **D9 — A successful refresh is adopted even when `Save` fails.**
   In `OAuthSession.Token`: on successful `refreshOAuthSession`, `adopt`
   the new session, then `Save`. If `Save` fails, the in-memory session
   already holds the new refresh (the only live credential after
   rotation); `Token()` returns the `Save` error so this call does not
   spend an unpersisted token. The next `Token()` in the same process
   sees `currentToken()` succeed. `slog` the persist failure. This
   changes persist-before-use for the crash window after a failed save;
   the alternative (leave the old refresh in memory) is a bricked
   session. Tests cover: save error after refresh → this call errors,
   subsequent `Token()` returns the new access without a second HTTP
   refresh.

10. **D10 — Windows `FileTokenStore` restricts the directory and the
    session file to the current user with a protected DACL.**
    `chmod0600` on Windows calls the same ACL recipe as
    `selfupdate.restrictToCurrentUser` (current-user `GENERIC_ALL`,
    owner = current SID, `PROTECTED_DACL_SECURITY_INFORMATION`).
    `NewFileTokenStore` applies it to `Dir` after `MkdirAll`. `Save`
    applies it to the final file after rename. The helper is duplicated
    in `llmprovider` (`tokenstore_file_windows.go` or a sibling
    `tokenstore_acl_windows.go`); `llmprovider` does not import
    `selfupdate`; `selfupdate` is not refactored in this record (extracting
    a shared package is a follow-up). Unix `chmod 0o600` / `0o700` stays.
    No OS keyring.

11. **D11 — `FileTokenStore.Save` `Sync`s the temp file before close and
    rename, on every GOOS.**
    After the write, `tmp.Sync()`, then `Close`, then `Rename`, then
    `chmod0600` / ACL. A `Sync` error is a failed `Save` (temp file
    cleaned up). Matches the self-update staging discipline.

12. **D12 — `validateProviderID` rejects Windows reserved device names,
    case-insensitively, including those names with an extension.**
    Reject `CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`,
    and the same with any suffix after a dot (`CON.json`). Comparison is
    case-insensitive and applies on every GOOS so a store copied onto
    Windows cannot be created from Unix. Trailing space and trailing dot
    on the provider id are rejected (Windows strips them). Existing
    `/\` and `..` rejects stay.

13. **D13 — Windows and GOOS-neutral tests lock D10–D12, and an overwrite
    test is committed.**
    * `TestFileTokenStore_OverwriteExisting` (all OS): Save twice, Load
      sees the second payload.
    * `TestFileTokenStore_RejectsReservedNames`: `CON`, `con`, `NUL`,
      `COM1`, `CON.json` all error with `ErrInvalidProvider`.
    * Windows-tagged: after Save, the file's DACL grants `GENERIC_ALL`
      to the current user and is protected (no inherited Administrators
      ACE, or an explicit assertion equivalent to what
      `GetNamedSecurityInfo` returns after `restrictToCurrentUser`).
    * The 0600 unix test stays unix-tagged.
    Each new gate is written first and seen to fail on HEAD.

14. **D14 — `go vet ./...` runs on every CI matrix OS, not only Linux.**
    `gofmt`, `go mod tidy -diff`, and `golangci-lint` stay Linux: they
    are GOOS-insensitive and lint is the expensive job. Vet is the
    GOOS-sensitive static check and is what Windows-tagged files currently
    skip.

15. **D15 — `go test -race ./...` runs on every CI matrix OS.**
    The race detector supports linux/amd64, darwin/arm64, and
    windows/amd64. This is a separate step from `go test ./...` so a race
    failure is distinguishable from a plain test failure. `make test`
    remains `go test ./...` (local inner loop stays fast). If
    `windows-2025` `-race` is impractically slow, that is a PLAN
    deviation — it is not pre-authorised here as "Linux and macOS only".

16. **D16 — CI grows a Linux `govulncheck ./...` job.**
    The version is pinned in `ci.yml` (same style as golangci-lint
    `v2.13.1`) and recorded in the PLAN. Non-zero exit fails the job.
    This is the 0006 gate that `make vuln` already describes and CI
    omitted.

17. **D17 — The release-guard script step sets `shell: bash`.**
    Both `./scripts/refuse-existing-release_test.sh` and
    `./scripts/check-workflow-gh-repo.sh` run under bash on all three
    matrix OS. GitHub-hosted Windows images provide Git Bash.
    `verify-selfupdate-release_test.sh` stays Linux-only (it needs
    `python3` and is already gated). Do not set a workflow-level
    `defaults.run.shell: bash`; `go test` on Windows keeps the runner
    default.

18. **D18 — CI grows a cross-compile job for the published GOOS/GOARCH
    set.**
    Linux runner, matrix `GOOS ∈ {linux,darwin,windows}` ×
    `GOARCH ∈ {amd64,arm64}`, `go build ./...`. Additionally, on that
    runner, `GOOS=windows GOARCH=amd64` and `GOOS=windows GOARCH=arm64`
    `go test -c` of `./selfupdate` and `./llmprovider` (output discarded)
    so Windows-tagged *tests* compile too. No execution of those binaries.

Host-lock tests from 0008 (ChatGPT must not hit `api.openai.com`; Grok
must not hit `cli-chat-proxy`) stay. No new `go.mod` require. No
`--clobber`. No `git push` or tag as part of this record.

### Consequences

* Good, because a Windows IDE detach becomes a classified shutdown
  again, and the test that says so is one that can fail.
* Good, because Grok browser login gets the CORS/PNA contract the
  official CLI needed, and OPTIONS stops aborting the waiter.
* Good, because a missed loopback is a paste prompt rather than a
  ten-minute hang.
* Good, because `chatgpt-access` stops being a successful configure
  result in this library.
* Good, because session files on Windows get a protected DACL matching
  the self-update receipt, reserved names cannot address devices, and
  `Save` is durable to `Sync`.
* Good, because a rotated refresh token survives a failed persist in
  the same process.
* Good, because `go vet` and `-race` run where the code runs, govulncheck
  is a merge gate, bash scripts have a bash shell, and the six published
  GOOS/GOARCH pairs compile.
* Neutral, because 0009 D1/D4/D5/D9/D11 are left in place; this record
  spends its OAuth budget on the remainder.
* Neutral, because unix 0600 and the existing POSIX shutdown phrases
  remain; Windows is additive.
* Bad, because duplicating `restrictToCurrentUser` into `llmprovider`
  creates two copies of the ACL recipe. A shared helper is the obvious
  cleanup and is explicitly deferred so this record does not grow a
  third package.
* Bad, because D9 weakens persist-before-use for the crash window after
  a failed `Save`. The in-process session is usable; a crash before the
  next successful `Save` still loses the rotated refresh. That is the
  remaining hole; an OS keyring is out of scope.
* Bad, because `-race` on three OS will lengthen CI. Distinguishing it
  as its own step (D15) keeps the signal.
* Bad, because 0009 P8 is still required before `prepare-commit-msg`
  tests stop writing `%APPDATA%\<product>\oauth\`. This record cannot
  close that from mcplib.

### Confirmation

Compliance is confirmed by tests seen to fail on a deliberate wrong
implementation, plus the CI configuration itself.

Shutdown (D1, D2):

```text
# on windows-2025 or GOOS=windows, against HEAD, before D1:
go test ./... -count=1 -run 'TestIsExpectedShutdownErr_Windows'
# expect FAIL: ERROR_BROKEN_PIPE classified false

# after D1:
go test ./... -count=1 -run 'TestIsExpectedShutdownErr_'
# expect PASS, including the POSIX Typed test
```

OAuth remainder (D3–D8):

```text
go test ./llmprovider -count=1 -run 'TestOAuthCallback_Grok|TestOAuthCallback_OpenAIHasNoGrokCORS'
# OPTIONS from Origin https://accounts.x.ai → 204/200, three CORS headers
# Origin https://evil.example → no Allow-Origin
# OpenAI OPTIONS → no accounts.x.ai

go test ./wizard -count=1 -run 'TestConfigureLLM_Browser'
# InputCode non-nil; a notify contains "paste"

go test ./llmprovider -count=1 -run 'TestExchangeOAuthCode_|TestRefreshOAuthSession_|TestOAuthTokenError_'
# 400 body with JWT-shaped text → error contains 400 and [REDACTED], not eyJ

go test ./wizard ./llmprovider -count=1 -run 'TestValidateOAuthSession_|TestSaveOAuthCredential_|TestConfigureLLM_TokenStdin|TestImportOpenAIAuth_'
# chatgpt-access rejected; JWT access-only accepted; import exp set

go test ./llmprovider -count=1 -run 'TestListAvailableModels|TestChatGPT'
# capturing transport: originator=mcplib, host ≠ api.openai.com
# StaticOpenAIChatGPT does not compile
```

Token store (D9–D13):

```text
go test ./llmprovider -count=1 -run 'TestFileTokenStore_|TestOAuthSession_SaveFailureAdopts'
# overwrite, reserved names, unix 0600 (unix), Windows DACL (windows)
```

CI (D14–D18): inspect `.github/workflows/ci.yml`:

* `go vet ./...` has no `if: runner.os == 'Linux'` (or has an equivalent
  step on every matrix OS).
* A step `go test -race ./...` exists on the matrix (every OS).
* A job runs `govulncheck ./...` with a pinned version.
* The release-guard step has `shell: bash`.
* A cross-compile job lists linux/darwin/windows × amd64/arm64 `go build ./...`
  and Windows `go test -c` for `./selfupdate` `./llmprovider`.

Whole-tree:

```text
go test ./...
go vet ./...
# Linux: gofmt -l empty, go mod tidy -diff, make lint
```

0008 host-lock tests still pass. No new `go.mod` require. A live OpenAI
and Grok browser login on the Windows laptop remains 0009's product
confirmation; this record's unit tests make that login *possible*.

## Pros and Cons of the Options

### A — One combined mcplib MADR (chosen)

* Good, because F1, F4–F12, and F13–F17 are the same class of defect
  (Windows production behaviour unproven by CI) and the file overlap is
  real (`oauth_session.go` is D5 and D9; `tokenstore_file.go` is D10–D13;
  `ci.yml` is the confirmation of all of it).
* Good, because 0009's unexecuted mcplib decisions keep their meaning
  (D3–D8 restate 0009 D2, D3, D7, D8, D11) without a second competing
  OAuth architecture.
* Bad, because the PLAN will be long. That is acceptable: the alternative
  is four interleaved plans touching the same files.

### B — Four separate MADRs

* Good, because a reviewer who only cares about CI could accept that
  slice alone.
* Bad, because `oauth_loopback.go` would sit in an OAuth MADR while CI
  claims to prove CORS tests that do not exist yet, and because shutdown
  tests on `windows-2025` are the CI story as much as the stdio story.
  Four numbers for one commit series is how remainder work gets stranded
  (see 0009 itself).

### C — Amend 0009

* Good, because D3–D8 *are* 0009.
* Bad, because 0009's problem statement is ChatGPT/Grok browser login on
  one laptop. Shutdown classification, FileTokenStore DACLs, govulncheck,
  and a GOARCH matrix are not that statement. Amending 0009 to include
  them would rewrite its rationale to look as if it had always been a
  Windows-hardening record. 0009 P8 also lives in another repository;
  widening 0009 here still would not close P8.

### D — CI first, 0009 later

* Good, because `-race` and `go vet` on Windows would start failing on
  whatever is already broken.
* Bad, because the F1 test does not exist yet, so a CI-only change
  cannot fail on F1. CORS, paste-code, and ACL similarly have no failing
  gate today. CI that cannot fail is the defect this record is closing.

### E — Document around the defects

* Good, because device-code already works on this host and Windows
  shutdown noise is "only" logs.
* Bad, because 0008 promised browser PKCE as the default, 0009 already
  rejected "Windows users must use device-code", and a classifier that
  lies is a fleet-wide behaviour change for every stdio server.

## More Information

### Evidence index

| Claim | Source |
| --- | --- |
| `IsExpectedShutdownErr` checks `syscall.EPIPE` / `ECONNRESET` and phrases `"broken pipe"`, `"connection reset"` | `stdio.go:185–208` |
| Typed test passes those same `syscall` constants | `transport_extra_test.go:45–58` |
| `GOOS=windows` `syscall.EPIPE` is `APPLICATION_ERROR + iota` | 2026-09-20 `GOOS=windows go doc syscall.EPIPE` |
| `syscall` on Windows has no `ERROR_BROKEN_PIPE` / `WSAECONNRESET` | 2026-09-20 `GOOS=windows go doc syscall.ERROR_BROKEN_PIPE` (no symbol) |
| Win32 values 109 / 232 / 10054 / 10053 and their English messages | Microsoft system error codes 0–499; Winsock `WSAECONNRESET` / `WSAECONNABORTED`; `golang.org/x/sys/windows` constants (already required at v0.47.0) |
| Dual-stack OpenAI loopback and background `OpenURL` are at HEAD | `oauth_loopback.go:109–113`, `:241–300` |
| Callback waiter already completes on state mismatch | `oauth_loopback.go:392–398`; `oauth_loopback_test.go` `TestOAuthCallback_RejectsStateMismatch` |
| ChatGPT generate sets `originator` | `openai.go:168–169`; `openai_chatgpt_test.go` |
| ChatGPT listing is Codex `/models` with `originator`; no `StaticOpenAIChatGPT` | `discovery.go:39–40`, `:123–157`; grep of `*.go` for `StaticOpenAIChatGPT` (zero hits) |
| Wizard listing failure does not substitute `StaticOpenAI` | `wizard/configure.go:258–298` |
| Grok CORS headers absent; OPTIONS shares the GET handler | `oauth_loopback.go:388–431` (`HandleFunc(path, …)`, no method, no `Access-Control-*`) |
| Wizard `oauthFlowOptions` omits `InputCode` | `wizard/auth.go:216–229` |
| Token exchange / refresh errors are status-only | `oauth_loopback.go:481`; `oauth_session.go:155–159` |
| `chatgpt-access` is a successful stdin-OAuth fixture | `wizard/auth_test.go:134–139` |
| `saveOAuthCredential` does not validate | `wizard/auth.go:232–247` |
| OpenAI import leaves `Expiry` zero | `wizard/import.go:78–94` |
| `chmod0600` on Windows is `return nil` | `llmprovider/tokenstore_file_windows.go` |
| `Save` has no `Sync` | `llmprovider/tokenstore_file.go:84–116` |
| `validateProviderID` rejects empty / `/\` / `..` only | `llmprovider/tokenstore.go:60–67` |
| Windows reserved device names, including with extension | Microsoft "Naming Files, Paths, and Namespaces" (`CON`, `PRN`, `AUX`, `NUL`, `COM1`–`9`, `LPT1`–`9`) |
| `restrictToCurrentUser` protected DACL recipe | `selfupdate/cleanup_windows.go:122–170` |
| Unix 0600 test is unix-tagged; no Windows ACL test | `tokenstore_file_unix_test.go` `//go:build unix` |
| Refresh adopts only after successful `Save` | `oauth_session.go:52–67` |
| Zero `Expiry` is treated as not-expired | `oauth_session.go` `currentToken` (`!s.Expiry.IsZero() && time.Until(s.Expiry) <= skew`) |
| CI matrix and Linux-only vet/lint | `.github/workflows/ci.yml:8–37` |
| Release-guard scripts unguarded by OS / `shell` | `.github/workflows/ci.yml:34–37` |
| `verify-selfupdate-release_test.sh` is Linux-only | `.github/workflows/ci.yml:30–32` |
| `make vuln` is opt-in, not CI | `Makefile` `vuln` target; README "Opt-in: `make vuln`" |
| 0006 raised Go to 1.26.6 because govulncheck reported reachable stdlib vulns | `docs/0006-MADR-raise-go-toolchain-floor-to-1-26-6.md` |
| Race detector supports this matrix's triples | Go race detector article: linux/amd64, darwin/arm64, windows/amd64 |
| Self-update asset contract includes windows/arm64, linux/arm64 | 0005 MADR/PLAN platform tables; `selfupdate/assets.go` `exactAssetName` |
| `x/sys` already required | `go.mod` `golang.org/x/sys v0.47.0` |
| GitHub Actions Windows default shell is PowerShell; bash is opt-in | GitHub Actions workflow syntax `jobs.<job_id>.steps[*].shell` |
| 0009 still `proposed`; P2–P7 unexecuted as a PLAN | `docs/decisions/0009-MADR-*.md` frontmatter; 0009 PLAN delivery order P2–P7 still listed as future work; HEAD sources above |
| HEAD `LoginBrowserOAuth` already races `InputCode` when set | `oauth_loopback.go:116–129` |
| grok-build HEAD used for vendor cites | `4247f661` (2026-09-19) |
| Codex HEAD used for vendor cites | `ac192cd79` (2026-09-06) |
| Grok loopback is `127.0.0.1:{ephemeral}/callback` | `grok-build` `xai-grok-login/src/oidc/login.rs:376–387` |
| Grok CORS origin is frozen `https://accounts.x.ai`; consent page `fetch`es loopback | `xai-grok-login/src/config.rs:124–147`, `:411–417` |
| Grok callback is GET + `CorsLayer.allow_private_network(true)`; OPTIONS is middleware | `xai-grok-login/src/oidc/login.rs:112–119` |
| Grok paste races loopback; 10-minute timeout; prompt `Paste the URL here if it doesn't connect:` | `oidc/login.rs:26–28`, `:355–357`, `:428–431`; parser matrix `:643–676` |
| Grok token/refresh errors include HTTP body | `oidc/protocol.rs:35–38`, `exchange_code` `:400–406` |
| Grok session write is `sync_all` + owner-only; Windows ACL = current-user `GENERIC_ALL` + protected DACL | `xai-grok-login/src/storage.rs:210–237`; `xai-grok-shell-base/src/util/secure_file.rs:1–10`, `:86–172` |
| Grok `webbrowser::open` failure is non-fatal | `oidc/login.rs:408–423` |
| Grok OAuth2 scopes add `conversations:*` and `workspaces:*` beyond mcplib | `xai-grok-login/src/config.rs:14–26` vs `llmprovider/oauth_loopback.go:25` — **out of this record** |
| Codex advertises `http://localhost:{1455\|1457}/auth/callback`, binds `127.0.0.1` only | `codex-rs/login/src/server.rs:59–62`, `:176`, `:637–694` |
| Codex authorize query includes `originator` and the same OpenAI scopes mcplib sends | `login/src/server.rs:584–611` |
| Codex default HTTP client always sets `originator` (`codex_cli_rs`) | `login/src/auth/default_client.rs:40–42`, `:335–350` |
| Codex models listing is `{chatgpt backend}/models?client_version=` | `codex-api/src/endpoint/models.rs:31–43`; `codex-api/src/api_bridge_tests.rs:508` |
| Codex login has no paste-code path | grep of `codex-rs/login` for paste/stdin (no OAuth paste race) |
| Codex file store is unix `0o600` + `flush`; no Windows ACL | `login/src/auth/storage.rs:206–222` |

### Related records

* [0009-MADR-repair-oauth-loopback-and-session-wiring.md](0009-MADR-repair-oauth-loopback-and-session-wiring.md)
  — source of D1–D11 for OAuth. This record implements the unexecuted
  mcplib subset (0009 D2, D3, D7, D8) and confirms D11. It does not
  supersede 0009. 0009 D6 / P8 remain in `prepare-commit-msg`.
* [0008-MADR-subscription-auth-for-llm-providers.md](../0008-MADR-subscription-auth-for-llm-providers.md)
  — native PKCE, `FileTokenStore`, host locks. Not reopened. Host-lock
  tests stay.
* [0005-MADR-canonicalize-cli-self-update-in-mcplib.md](../0005-MADR-canonicalize-cli-self-update-in-mcplib.md)
  — three-OS `go test` matrix, `restrictToCurrentUser`, asset GOOS/GOARCH
  set. This record extends that matrix's *gates* and reuses the ACL
  recipe without moving it.
* [0006-MADR-raise-go-toolchain-floor-to-1-26-6.md](../0006-MADR-raise-go-toolchain-floor-to-1-26-6.md)
  — govulncheck as the reason for the floor. This record puts it in CI.
* [0002-MADR-xdg-compliant-user-paths.md](../0002-MADR-xdg-compliant-user-paths.md)
  — accepted, unimplemented `paths` package. Out of scope here.
* [0007-MADR-restore-repository-context-in-the-reusable-release-workflow.md](../0007-MADR-restore-repository-context-in-the-reusable-release-workflow.md)
  — origin of `refuse-existing-release_test.sh` / `check-workflow-gh-repo.sh`.
  D17 gives those scripts a bash shell on Windows; it does not change
  their contract.

Vendor checkouts (sibling `gitrepos/`, not in this module):

* `grok-build` @ `4247f661` — CORS, paste race, token-error body, Windows
  ACL + `fsync` for session files.
* `codex` @ `ac192cd79` — Hydra `localhost:{1455\|1457}` URI, IPv4-only
  bind (0009 D1 is the Windows delta), `originator` on every default
  client, Codex `/models?client_version=`.

### Open questions for the plan

1. Wall-clock of `go test -race ./...` on `windows-2025` for this module.
   D15 requires it; a timeout or OOM is a deviation, not a silent
   Linux-only fallback.
2. Exact `govulncheck` version to pin next to golangci-lint v2.13.1.
   Check what `go install golang.org/x/vuln/cmd/govulncheck@latest`
   resolves to on the day the PLAN is executed; pin that or a reviewed
   older version, and record the choice.
3. Whether `GetNamedSecurityInfo` after `restrictToCurrentUser` on a
   GitHub-hosted `windows-2025` runner (which is not a personal
   workstation) still shows a protected DACL without unexpected inherited
   ACEs from the runner image. If the runner's default ACL shape differs,
   the Windows ACL test asserts the recipe's own invariants (owner SID =
   current user, protected bit set, an explicit `GENERIC_ALL` ACE for
   that SID) rather than "Administrators is absent".
4. `go test -c` output path on the Linux cross-compile job (`-o /dev/null`
   vs a temp file). Windows `GOOS` on Linux produces an `.exe`; the PLAN
   discards it and must not upload it as an artifact.
5. 0009 D11 confirmation: if a capturing test for `listChatGPTModels`
   already covers originator + host lock (`discovery_test.go:135`), the
   PLAN cites it rather than duplicating. If the wizard ChatGPT-OAuth
   "no StaticOpenAI on listing failure" case is missing, add it in the
   D8 phase.
6. Locale of Win32 `Error()` strings on `windows-2025`. D1's primary
   contract is `errors.Is`; the PLAN's negative test uses typed errors,
   not English phrases, so a non-en-US runner cannot false-pass D2.
7. Grok OAuth2 at `4247f661` requests four extra scopes
   (`conversations:read|write`, `workspaces:read|write`) that
   `grokOAuthScopes` omits. 0008 froze the six-scope string. This record
   does not widen it. A later MADR may, if generation or refresh starts
   failing for missing scope.

The PLAN is a separate file. It is not written until this MADR has been
reviewed. No source, test, or CI file is changed in the same commit as
this record.
