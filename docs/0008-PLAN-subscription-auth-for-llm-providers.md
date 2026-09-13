---
status: in-progress
date: 2026-09-13
associated-madr: "0008-MADR-subscription-auth-for-llm-providers.md"
decision-makers: mcplib maintainers
---

# Implement Browser, API-Key, and Headless Subscription Authentication for OpenAI and xAI Grok

Associated MADR: [0008-MADR-subscription-auth-for-llm-providers.md](0008-MADR-subscription-auth-for-llm-providers.md)
(accepted, revision 5, 2026-09-12).

This plan executes that MADR and nothing else. If execution discovers a fact that
contradicts the MADR, **stop and amend the MADR** before continuing. Do not
smuggle a different architecture into a phase.

> **Plan revision 2026-09-12.** Maintainer named **prepare-commit-msg** as the
> first consumer. Phases 1–9 stay in `mcplib`. Phase 10 is that hook only
> (sibling repo `../prepare-commit-msg`). MagicDev and MagicTools are still
> out of this plan.
>
> **Plan revision 2026-09-13 (sweep).** Re-read this plan against `mcplib`
> `69271af`, `wizard/configure_test.go`, `wizard/text_prompter.go` (1-based
> Select), and `prepare-commit-msg` `internal/ui/setup.go` /
> `setup_test.go` / `main.go` / `config.go`. Corrections below are applied
> in place; they do not change the MADR. See §0.1.

## Goal

Standalone `mcplib` consumers can configure `openai` and `grok` with:

* an API key (today's path, unchanged billing host),
* a browser OAuth PKCE loopback,
* a headless path (RFC 8628 device-code, token stdin/env, or import of the
  official CLI credential file),

and then generate through `llmprovider` against the **correct** inference host
for that credential. Orchestrated sub-servers keep using `NewBackplaneClient`
and never hold ChatGPT/Grok refresh tokens.

**First consumer:** `prepare-commit-msg` (`configure` + hook generation) can
spend a ChatGPT or Grok subscription after Phase 10.

## Scope

**In:** `mcplib` packages `llmprovider`, `wizard`, and the README LLM section;
then `prepare-commit-msg` config, wizard wiring, and `NewProvider` construction
(Phase 10).

**Out:** `claude`, `gemini`, Vertex, Bedrock, Foundry, gateway OAuth, Ollama
changes, wrapping `codex`/`grok` binaries, OS keyring, Grok customer-IdP OIDC,
Grok `ExternalHelper` command, MagicTools backplane login, MagicDev,
`golang.org/x/oauth2`, vendor SDKs, `git push`. Unifying config schemas across
the fleet remains out (MADR 0004).

## 0. Notation and conventions

* File references are against `mcplib` at `69271af` plus the accepted MADR
  (`docs/0008-MADR-subscription-auth-for-llm-providers.md`). Do not treat
  uncommitted `README.md` edits as this plan's baseline.
* **Phase green** (this repo has no `make pre-add-check`): for every file the
  phase stages, `gofmt -l` prints nothing; `go vet` on the touched packages
  exits 0; `make lint` exits 0; `go test` on the touched packages exits 0.
  Redirect each of those to a log and branch on the command's own status —
  never `cmd | tail && git commit`.
* Each `mcplib` phase (1–9) ends with **one** `git commit --no-edit` in
  `mcplib`. Phase 10 commits in `prepare-commit-msg` only. No `-m`. No
  `git push`.
* **A new test is not trusted until it has been seen to fail.** Each phase
  that adds a gate: write the test first, run it, capture the FAIL line in
  the phase log, then implement, then capture PASS. Do not dirty production
  code to create the red run.
* No new `go.mod` require. PKCE, loopback, device-code, and refresh are
  stdlib `net/http` + `crypto/sha256` + `encoding/base64`.
* Names below are **the** exported API, not sketches. If a name must change,
  amend this plan before committing.
* `fakePrompter.Select` indices are **0-based**. `TextPrompter.Select` user
  input is **1-based** (`wizard/text_prompter.go:149`). mcplib wizard tests
  use fakePrompter. prepare-commit-msg setup tests type into TextPrompter.
  Do not mix the two.

### 0.1 Sweep findings (2026-09-13) — locked by this revision

These were ambiguous or wrong in the 2026-09-12 text. The body below matches
this list.

1. **`Options.Orchestrated == nil` means `mcplib.IsOrchestratorOwned()`, not
   true.** If nil meant true, every existing `ConfigureLLM` test would return
   `ErrOrchestrated`. Tests that need a standalone wizard leave it nil (CI is
   not orchestrated) or set `boolPtr(false)`.
2. **`prepare-commit-msg` already has an OpenAI interactive script**
   (`setup_test.go:107` `input := "2\ntest-key\n1\n…"`). After Phase 8 that
   input is wrong: `2` selects openai, then the **next** line is the new
   auth-method menu, so `"test-key"` is not a key. Phase 10 **must** change
   that string to `2\n1\ntest-key\n1\n…` (provider 2, auth method 1 =
   `api_key`). Gemini `1` and Claude `3` stay valid (no auth-method menu).
3. **`AuthKind` on disk is `"oauth"` or absent.** Do not write `"api_key"`;
   empty means API key so existing `config.json` files stay valid without a
   churn on next configure.
4. **`token_stdin` heuristic is exact, not "JWT-like".** openai: prefix `sk-`
   → API key, anything else → ChatGPT access-only session (no refresh).
   grok: always API key. `CODEX_ACCESS_TOKEN` is read only on openai
   `token_stdin` when `AllowEnv` is true, same Confirm+MaskSecret as an env
   API key.
5. **Keep-existing OAuth** is a `Confirm`, same as keep-existing key, using
   `MaskSecret(Existing.AccessToken)`. It runs only when
   `Existing.Kind == CredOAuth` and `Existing.Provider == d.ID`.
6. **Host-lock tests must not use `WithBaseURL`.** A capturing
   `http.RoundTripper` records `r.URL` and returns a canned 200 Responses
   body. `WithBaseURL` would hide the default-host branch.
7. **OpenAI loopback ports are injected.** Production calls
   `listenFirstAvailable("127.0.0.1", []int{1455, 1457})`. Tests pass two
   ephemeral ports so they do not fight a running Codex CLI on 1455.
8. **OAuth URL constants live in `llmprovider/oauth_constants.go`**, not
   `constants.go` (that file is JSON field names; stuffing URLs there trips
   goconst and mixes concerns).
9. **`FileTokenStore.Load` sets `session.Store = fs`** so a loaded session
   refreshes to the same directory without the caller remembering.
10. **`OAuthSession.HTTPClient`**: nil → `defaultHTTPClient()`. Tests assign
    the httptest client. Refresh POSTs use that client.
11. **Empty `TokenURL`**: ChatGPT issuer → `{issuer}/oauth/token`; otherwise
    `https://auth.x.ai/oauth2/token`.
12. **One 401 retry** on generate for `*OAuthSession` only: on
    `ErrAuthFailure`, set `Expiry` to the past, call `Token()` again, retry
    the HTTP once. Static keys do not retry 401 (today's behaviour).
13. **Phase 8 `configure_test.go`:** current tests select claude, gemini, or
    ollama only (`selects` second index is the **model**, not auth). They
    keep working. Any **new** openai/grok wizard test must queue an extra
    0-based auth-method index after the provider index.
14. **Phase 10 `go-precheck.sh`** is invoked with the files **actually
    staged**, not a guessed list. `open.go` is listed only after it exists.

## 1. Locked constants

These values are copied from the official CLIs inspected for the MADR. Changing
any of them is a MADR deviation.

### OpenAI / Codex

| Name | Value |
|---|---|
| `DefaultOpenAIIssuer` | `https://auth.openai.com` |
| `DefaultOpenAIClientID` | `app_EMoamEEZ73f0CkXaXp7hrann` |
| `DefaultOpenAIChatGPTBaseURL` | `https://chatgpt.com/backend-api/codex` |
| `DefaultOpenAIPlatformBaseURL` | `https://api.openai.com/v1` (already used by `NewOpenAI`) |
| Authorize path | `{issuer}/oauth/authorize` |
| Token path | `{issuer}/oauth/token` |
| Browser redirect | `http://localhost:{port}/auth/callback` |
| Loopback ports | **1455**, fallback **1457** (Hydra allow-list). Never a random port. |
| Authorize scopes | `openid profile email offline_access api.connectors.read api.connectors.invoke` |
| Authorize extras | `response_type=code`, `code_challenge_method=S256`, `id_token_add_organizations=true`, `codex_cli_simplified_flow=true`, `originator=mcplib` |
| Device user-code | `POST {issuer}/api/accounts/deviceauth/usercode` JSON `{"client_id":...}` |
| Device poll | `POST {issuer}/api/accounts/deviceauth/token` JSON `{"device_auth_id","user_code"}` until 200 or 15 minutes |
| Device verification URL shown to user | `{issuer}/codex/device` |
| Device code exchange redirect | `{issuer}/deviceauth/callback` |
| ChatGPT request URL | `{DefaultOpenAIChatGPTBaseURL}/responses` |
| ChatGPT extra headers | `Authorization: Bearer {access}`; `ChatGPT-Account-Id` when account id is known; `x-openai-internal-codex-residency` when the access JWT carries a residency other than `no_constraint` |
| Forbidden | sending a ChatGPT session to `api.openai.com`; sending a Platform key to the Codex host |

### xAI / Grok

| Name | Value |
|---|---|
| `DefaultGrokOAuthIssuer` | `https://auth.x.ai` |
| `DefaultGrokOAuthClientID` | `b1a00492-073a-47ea-816f-4c329264a828` |
| `DefaultGrokBaseURL` | `https://api.x.ai/v1` (already used by `NewGrok`; **session and key share this host**) |
| Token URL fallback | `https://auth.x.ai/oauth2/token` |
| Device URL fallback | `https://auth.x.ai/oauth2/device/code` |
| Discovery | `GET {issuer}/.well-known/openid-configuration`; on failure use the fallbacks |
| Browser loopback | `http://127.0.0.1:{ephemeral}/callback` (RFC 8252; random port is required) |
| Scopes | `openid profile email offline_access grok-cli:access api:access` |
| Device grant | `urn:ietf:params:oauth:grant-type:device_code` |
| Referrer / originator | `mcplib` (do **not** send `grok-build` or `opencode`) |
| Env override | `GROK_OAUTH2_CLIENT_ID`, `GROK_OAUTH2_ISSUER` (same names as the official CLI) |
| Forbidden | `https://cli-chat-proxy.grok.com`; header `X-XAI-Token-Auth` |

### Shared

| Name | Value |
|---|---|
| Token file mode | Unix `0600` after every write |
| Token file name | `{dir}/{provider}.json` where provider is `openai` or `grok` |
| Refresh skew | 2 minutes before `Expiry` |
| Single-flight | one in-flight refresh per `OAuthSession` |
| Persist-before-use | `TokenStore.Save` of the rotated refresh token **must succeed** before the new access token is returned |
| `originator` / `referrer` | `mcplib` |

## 2. Locked types

Place in `llmprovider` unless noted.

```go
package llmprovider

type TokenType string

const (
    TokenAPIKey TokenType = "api_key"
    TokenBearer TokenType = "bearer"
)

type Token struct {
    Value  string
    Type   TokenType
    Expiry time.Time // zero = unknown / non-expiring
    Header string    // always "Authorization" for openai and grok
}

type TokenSource interface {
    Token(ctx context.Context) (Token, error)
}

// StaticToken is today's API key. Token() returns it unchanged.
type StaticToken struct {
    Value  string
    Header string // default "Authorization"
}

func NewStaticToken(value string) *StaticToken

type OAuthSession struct {
    Provider   string
    Access     string
    Refresh    string
    Expiry     time.Time
    Issuer     string
    ClientID   string
    AccountID  string // ChatGPT; empty for Grok
    TokenURL   string // empty → default from issuer, see §0.1.11
    Store      TokenStore
    HTTPClient *http.Client // nil → defaultHTTPClient(); tests inject httptest
    mu         sync.Mutex
    inflight   *tokenFuture
}

func (s *OAuthSession) Token(ctx context.Context) (Token, error)
func (s *OAuthSession) ChatGPT() bool // Issuer trims to DefaultOpenAIIssuer

type TokenStore interface {
    Load(ctx context.Context, provider string) (*OAuthSession, error)
    Save(ctx context.Context, provider string, s *OAuthSession) error
    Delete(ctx context.Context, provider string) error
}

type FileTokenStore struct {
    Dir string
}

func NewFileTokenStore(dir string) (*FileTokenStore, error) // mkdir 0700

type AuthMethodID string

const (
    AuthAPIKey         AuthMethodID = "api_key"
    AuthBrowserOAuth   AuthMethodID = "browser_oauth"
    AuthDeviceCode     AuthMethodID = "device_code"
    AuthTokenStdin     AuthMethodID = "token_stdin"
    AuthImportVendorCLI AuthMethodID = "import_vendor_cli"
)

type AuthMethod struct {
    ID          AuthMethodID
    Label       string
    Detail      string
    Interactive bool
    HeadlessOK  bool
}
```

`ProviderDescriptor` gains `AuthMethods []AuthMethod`. `Descriptors()` copies
that slice. Only `openai` and `grok` get OAuth methods (table in the MADR).

```go
func NewProviderWithSource(name string, src TokenSource, model string, opts ...ProviderOption) (Provider, error)
```

Supported names: `openai`, `grok`. Any other name returns
`fmt.Errorf("provider %q does not accept TokenSource", name)`.

`NewProvider(name, apiKey, model, opts...)` stays. For openai/grok it becomes
`NewProviderWithSource(name, NewStaticToken(apiKey), model, opts...)`.
`NewGrok` still rejects a static empty key. `NewGrokWithSource` (unexported
construction via `NewProviderWithSource`) allows an `OAuthSession`.

OpenAI ChatGPT vs Platform is **not** a new provider id. Detection:

* `*StaticToken` → Platform, `DefaultOpenAIPlatformBaseURL`, `StaticOpenAI`.
* `*OAuthSession` with `ChatGPT()==true` → Codex host
  `DefaultOpenAIChatGPTBaseURL`, catalog `StaticOpenAIChatGPT`.
* `WithBaseURL` still overrides the host for decode/tool tests. **Default-host
  tests (A1, A2, A4) use a capturing RoundTripper and must not pass
  `WithBaseURL`.**

Grok: both sources use `DefaultGrokBaseURL`. Requests set
`Authorization: Bearer` from `TokenSource.Token` and **never**
`X-XAI-Token-Auth`.

### Wizard

```go
package wizard

type CredentialKind string

const (
    CredNone   CredentialKind = ""
    CredAPIKey CredentialKind = "api_key"
    CredOAuth  CredentialKind = "oauth"
)

type Result struct {
    Provider      string
    Kind          CredentialKind
    APIKey        string // set iff Kind == CredAPIKey
    AccessToken   string // set iff Kind == CredOAuth
    RefreshToken  string
    TokenExpiry   time.Time
    Issuer        string
    ClientID      string
    AccountID     string
    Model         string
    BaseURL       string
    Fallbacks     []string
}

type Options struct {
    // existing fields unchanged
    Providers, Existing, AllowEnv, Discover, DiscoverLimit, NeedFallbacks, LookupEnv
    TokenStore   llmprovider.TokenStore // required for browser/device/import
    OpenURL      func(string) error     // nil = log the URL only, still succeed
    Orchestrated *bool                  // nil => mcplib.IsOrchestratorOwned()
}
```

`orchestrated(o)` is: if `o.Orchestrated != nil`, use `*o.Orchestrated`; else
`mcplib.IsOrchestratorOwned()`. When that is true, `ConfigureLLM` returns
`ErrOrchestrated` with **zero** Prompter calls. Existing wizard tests leave
the field nil (the test process is not orchestrated). prepare-commit-msg sets
`boolPtr(false)` so a stray `MCP_ORCHESTRATOR_OWNED` cannot disable configure.

`ErrOrchestrated` is `var ErrOrchestrated = errors.New("wizard: orchestrated process uses the LLM backplane, not provider OAuth")`.

## 3. Catalog for ChatGPT mode

Add beside `StaticOpenAI` in `llmprovider/models_catalog.go`:

```go
StaticOpenAIChatGPT = []string{
    "gpt-5.4",
    "gpt-5.4-mini",
    "gpt-5.3-codex",
}
```

Sourced from Codex CLI docs (GPT-5.4, GPT-5.3-Codex) plus the mini sibling.
`StaticModels("openai")` **does not change** (Platform catalog). ChatGPT mode
calls `StaticOpenAIChatGPT` directly. Wizard "Other (enter a model id)" remains
the escape hatch. `ListAvailableModels("openai", chatgptAccessToken)` must
**not** hit `api.openai.com/v1/models`; it returns `StaticOpenAIChatGPT`
(copy). Live Platform listing stays for static keys.

Grok listing is unchanged: session bearer on `api.x.ai/v1/models` (probe: 200).

## 4. Phase sequencing

| Phase | Deliverable | New files | Modified files |
|---|---|---|---|
| 1 | `Token`, `TokenSource`, `StaticToken` | `llmprovider/token.go`, `token_test.go` | — |
| 2 | `TokenStore` + `FileTokenStore` `0600` | `tokenstore.go`, `tokenstore_file.go`, `tokenstore_file_unix.go`, `tokenstore_file_windows.go`, `tokenstore_test.go`; corrective pass adds `tokenstore_file_unix_test.go` | corrective pass also modifies `token.go`, `token_test.go`, and the Phase 2 files |
| 3 | `OAuthSession` refresh + persist-before-use | `oauth_constants.go` (initially `DefaultOpenAIIssuer` and the private xAI token URL fallback), `oauth_session.go`, `oauth_session_test.go` | `tokenstore.go` (add `mu`, `inflight`, and `tokenFuture` when they become used) |
| 4 | `AuthMethod` on descriptors | — | `descriptor.go`, `descriptor_test.go` |
| 5 | PKCE, OpenAI loopback 1455/1457, Grok ephemeral loopback, device-code | ~~`oauth_constants.go`,~~ `oauth_pkce.go`, `oauth_loopback.go`, `oauth_device.go`, `*_test.go` | `oauth_constants.go` (extend the Phase 3 file with the remaining locked constants) |
| 6 | OpenAI transport branch + ChatGPT catalog | `openai_chatgpt.go`, `openai_chatgpt_test.go` | `openai.go`, `provider.go`, `models_catalog.go`, `models_catalog_test.go`, `discovery.go`, `discovery_test.go` |
| 7 | Grok `TokenSource` on `api.x.ai` | `grok_oauth_test.go` | `grok.go`, `provider.go` |
| 8 | Wizard auth-method step, import, orchestrated guard | `wizard/auth.go`, `wizard/import.go`, `wizard/auth_test.go`, `wizard/import_test.go` | `wizard/configure.go`, `configure_test.go` |
| 9 | README | — | `README.md` |
| 10 | `prepare-commit-msg` adopts OAuth | — | see Phase 10 file list |

Dependencies: 1→2→3; 4 independent of 1–3; 5 needs 1 and 3; 6 needs 1, 3, 4, 5;
7 needs 1 and 3; 8 needs 2–7; 9 needs 8; **10 needs 1–9** and is a different
repository. Do not start Phase 10 until Phase 9 is committed in `mcplib`.

## Phase 1 — `TokenSource` / `StaticToken`

### Steps

1. Add `llmprovider/token.go` with the types in §2 (`Token`, `TokenType`,
   `TokenSource`, `StaticToken`, `NewStaticToken`). `NewStaticToken("")` is
   legal (OpenAI tests inject empty keys with `WithBaseURL`). `Token()`
   returns `Header: "Authorization"` when `StaticToken.Header` is empty.
2. **Red test first.** `TestStaticToken_ReturnsBearer` asserting
   `tok.Header == "Authorization"` and `tok.Type == TokenBearer`. Also
   `TestNewProvider_StillConstructsFromAPIKey` is **not** this phase.
3. `go test ./llmprovider -run TestStaticToken -count=1` — FAIL (file missing
   or assertion failing). Capture the FAIL line.
4. Implement. Re-run — PASS.
5. Phase green on `./llmprovider`. `git add` the two files. `git commit --no-edit`.

### Negative test (prove the instrument)

`TestStaticToken_EmptyValueStillReturnsToken` must fail if `Token()` returns
an error on empty value (that would break OpenAI's existing empty-key tests).
Write it to require `err == nil`.

## Phase 2 — `FileTokenStore` mode `0600`

### Steps

1. JSON on disk (one object, not a map):

```json
{
  "provider": "openai",
  "access": "...",
  "refresh": "...",
  "expiry": "2026-09-12T18:00:00Z",
  "issuer": "https://auth.openai.com",
  "client_id": "app_EMoamEEZ73f0CkXaXp7hrann",
  "account_id": "",
  "token_url": "https://auth.openai.com/oauth/token"
}
```

   Persist via an unexported `fileRecord` struct with those json tags — **not**
   by marshalling `OAuthSession` (that would try to serialize `Store` /
   `HTTPClient` / `mu`).
   `Load` of a missing file returns `(nil, nil)`, not an error.
   `Load` of a valid file returns `session` with `session.Store = fs`.
   `Save` writes `{dir}/{provider}.json` via temp file + rename, then sets
   mode `0600` on Unix (`tokenstore_file_unix.go`: `os.Chmod(path, 0o600)`
   after rename). Windows: `tokenstore_file_windows.go` `chmod0600` is a
   no-op; tests for mode use `//go:build unix`.
   `NewFileTokenStore` `MkdirAll(dir, 0o700)`.
   Reject `provider` that is empty or contains `/`, `\`, or `..`.

2. **Red tests, in this order:**

   * `TestFileTokenStore_SaveMode0600` (unix): `stat.Mode().Perm() == 0600`.
     ~~Implement `Save` **without** chmod first, run, **watch it fail**, then
     add chmod.~~ `os.CreateTemp` itself creates mode `0600`, so omitting the
     later chmod cannot prove this gate. The 2026-09-13 correction proves the
     test by changing the Unix helper to `0644` in a scratch clone and watching
     the assertion report `mode = 644, want 0600`. This is the MADR
     confirmation "mode 0600".
   * `TestFileTokenStore_RoundTrip`
   * `TestFileTokenStore_LoadMissingIsNil`
   * `TestFileTokenStore_RejectsPathTraversal`

3. ~~Phase green. Commit.~~ Commit `9836e94` landed even though the captured
   Phase 2 `gofmt` and lint checks were red. Complete the corrective pass below
   and land a corrective commit before Phase 3.

`Save` must not log `access` or `refresh`. A test
`TestFileTokenStore_DoesNotLogSecrets` is not required if the store never
calls `slog`; assert the file package imports no `log/slog`.

### Approved Phase 2 corrective pass (2026-09-13)

The maintainer approved this corrective pass after the Phase 1–2 audit. It does
not change the MADR's architecture.

1. Add the missing Go documentation required by the per-file `golint` gate to
   the Phase 1 and Phase 2 exported API.
2. Format `tokenstore.go`; check and propagate the `Close`, cleanup, and final
   chmod outcomes that `errcheck` reported.
3. Remove the unused Phase 3-only `mu`, `inflight`, and `tokenFuture` scaffold
   from Phase 2. Phase 3 adds them to `tokenstore.go` in the same commit that
   first uses them.
4. Move `TestFileTokenStore_SaveMode0600` to
   `tokenstore_file_unix_test.go` with `//go:build unix`; keep the other store
   tests platform-neutral.
5. Add a provider id containing `..` but no slash or backslash to
   `TestFileTokenStore_RejectsPathTraversal` so that rule is independently
   guarded.
6. Prove the adjusted tests in a scratch clone: use a `0644` Unix helper for
   the mode failure and remove only the `..` validation for the traversal
   failure. Assert both deliberate mutations landed before running the tests.
7. Run the corrected per-command phase gate plus per-file `golint`. Record the
   full output and commit only when every check is green.

Corrective scope: `docs/0008-PLAN-subscription-auth-for-llm-providers.md`,
`llmprovider/token.go`, `llmprovider/token_test.go`,
`llmprovider/tokenstore.go`, `llmprovider/tokenstore_file.go`,
`llmprovider/tokenstore_file_unix.go`,
`llmprovider/tokenstore_file_windows.go`, `llmprovider/tokenstore_test.go`, and
new `llmprovider/tokenstore_file_unix_test.go`.

## Phase 3 — `OAuthSession` refresh

### Approved constant-placement deviation (2026-09-13)

Phase 3's specified `ChatGPT()` API depends on `DefaultOpenAIIssuer`, but the
original phase table did not create `oauth_constants.go` until Phase 5. The
maintainer approved the recommended resolution: Phase 3 creates
`oauth_constants.go` with `DefaultOpenAIIssuer`, and Phase 5 extends that file
with the remaining locked constants. This preserves §0.1.8 and the exported API
without a temporary duplicate or literal. Phase 3 scope therefore adds
`llmprovider/oauth_constants.go`; the architectural decision is unchanged, so
the MADR needs no amendment.

The first amendment named only `DefaultOpenAIIssuer`. Implementation then
exposed the same ordering issue for §0.1.11's empty-`TokenURL` xAI fallback.
The maintainer approved adding private ~~`defaultGrokOAuthTokenURL`~~ to the
same Phase 3 file. The mandatory lint gate identified `Token` in that private
identifier as a G101 credential false positive, so a follow-up approval renamed
it to `defaultGrokOAuthRefreshURL` without suppressing the security check.
Phase 5 still owns the remaining locked OAuth constants.

### Behaviour

* If Access is non-empty and `Expiry` is zero: return Access (access-only
  import / `CODEX_ACCESS_TOKEN`; no refresh).
* If Access is non-empty and `time.Until(Expiry) > 2*time.Minute`: return Access.
* Else POST `application/x-www-form-urlencoded` to `tokenURL(s)` (see §0.1.11)
  `grant_type=refresh_token&refresh_token=...&client_id=...` using
  `s.HTTPClient` or `defaultHTTPClient()`.
* On HTTP 200, parse `access_token` (required), optional `refresh_token`
  (keep old if absent), `expires_in` (default 3600s).
* Build a **copy** of the session with the new tokens. If `Store != nil`,
  `Save` that copy **first**. Only if `Save` returns nil, assign the copy
  into `s`. If `Save` fails, `s.Refresh` is still the old value and
  `Token()` returns the save error.
* Missing refresh token when a refresh is required → error
  `oauth: no refresh token`.
* Single-flight under `s.mu`.
* `ChatGPT()` is `strings.TrimRight(s.Issuer, "/") == DefaultOpenAIIssuer`.

### Red tests

* `TestOAuthSession_RefreshPersistsBeforeReturn`: httptest token endpoint
  rotates refresh to `new-refresh`. A `TokenStore` whose `Save` returns an
  error must leave `session.Refresh == "old-refresh"` and `Token()` error.
  Write the test against an implementation that updates memory first — it
  **must fail** — then fix the order.
* `TestOAuthSession_SingleFlight`: two goroutines, token endpoint counter == 1.
* `TestOAuthSession_SkewsTwoMinutes`.
* `TestOAuthSession_ChatGPTDetectsIssuer`.

httptest only. No live network.

Phase green. Commit.

## Phase 4 — `AuthMethod` on descriptors

### Descriptor methods (exact labels)

**openai** (order):

1. `api_key` — "OpenAI API key" — Detail "Platform billing (`api.openai.com`)" — Interactive true, HeadlessOK true
2. `browser_oauth` — "Sign in with ChatGPT" — Detail "Plus/Pro/Business/Edu/Enterprise plan" — Interactive true, HeadlessOK false
3. `device_code` — "Sign in with ChatGPT (device code)" — HeadlessOK true
4. `token_stdin` — "Paste a ChatGPT access token or API key" — HeadlessOK true
5. `import_vendor_cli` — "Import ~/.codex/auth.json" — HeadlessOK true

**grok** (order):

1. `api_key` — "xAI API key" — Detail "console.x.ai billing (`api.x.ai`)"
2. `browser_oauth` — "Sign in with xAI"
3. `device_code` — "Sign in with xAI (device code)"
4. `token_stdin` — "Paste an xAI API key"
5. `import_vendor_cli` — "Import ~/.grok/auth.json"

Every other descriptor: `AuthMethods` nil or empty. `RequiresAPIKey` unchanged.

### Approved test-sequencing correction (2026-09-13)

The original expected-pass instruction for
`TestDescriptors_NoOAuthOnOtherProviders` assumed `AuthMethodID`, `AuthMethod`,
and `ProviderDescriptor.AuthMethods` already existed. They do not exist at the
Phase 3 boundary, so the test cannot compile against the untouched tree. The
maintainer approved this corrected Phase 4 sequence:

1. Add only the `AuthMethodID` constants, `AuthMethod`, and the empty
   `ProviderDescriptor.AuthMethods` field in `descriptor.go`.
2. Add `TestDescriptors_NoOAuthOnOtherProviders` and observe it pass while all
   descriptor method lists remain empty.
3. Prove that expected-pass test in a scratch clone by assigning
   `browser_oauth` to a non-OAuth provider and observing its intended failure.
4. Add the remaining Phase 4 tests and observe them fail before populating the
   OpenAI and Grok method lists.

This changes only sequencing. It adds no files, changes no descriptor contract,
and requires no MADR amendment.

### Red tests

* `TestDescriptors_OpenAIAndGrokOfferOAuth` — FAIL until methods exist.
* `TestDescriptors_NoOAuthOnOtherProviders` — range `Descriptors()`, if
  `ID` not in `{openai,grok}` then no method id is `browser_oauth` or
  `device_code`. ~~Write this test **first** against current code; it should
  **PASS already** (empty methods).~~ Add the compile scaffold first as
  specified by the approved sequencing correction, then observe the test pass.
  Keep it, prove it with the deliberate scratch mutation, then add openai/grok
  methods and confirm it still passes.
* Extend `TestDescriptors_CoverEveryRegisteredProvider` to require openai
  and grok each contain `api_key` and `browser_oauth`.
* Defensive copy: mutating `d.AuthMethods[0].Label` must not affect the next
  `Descriptors()` call.

Phase green. Commit.

## Phase 5 — PKCE, loopback, device-code

### PKCE (`oauth_pkce.go`)

S256: 32 random bytes, base64url no pad verifier; SHA-256 challenge, base64url
no pad. `TestPKCE_ChallengeIsS256` computes the challenge independently.

### OpenAI loopback (`oauth_loopback.go`)

```go
func listenFirstAvailable(host string, ports []int) (net.Listener, int, error)
```

Production: `listenFirstAvailable("127.0.0.1", []int{1455, 1457})`. If both
fail, error text must contain `device-code`. **Do not** fall through to
`:0`.

* Advertise redirect `http://localhost:{port}/auth/callback` (host
  `localhost`, not `127.0.0.1`, matching Codex `server.rs:176`).
* Path `/auth/callback`: read `code`, `state`, `error`. State must match.
  Respond 200 `text/html` `You can close this window.` (no code/token in HTML).
* Timeout 10 minutes (`context.WithTimeout`).
* `OpenURL` is invoked with the authorize URL; nil or erroring `OpenURL`
  does not fail the flow.

**Red test (deterministic, no fight with Codex on 1455):**

`TestListenFirstAvailable_UsesSecondPortWhenFirstBusy`: bind
`127.0.0.1:0` twice to get two free ports `p1,p2`; occupy `p1`; call
`listenFirstAvailable("127.0.0.1", []int{p1, p2})`; assert returned port
`== p2`. Then `TestListenFirstAvailable_ErrorsWhenAllBusy` occupying both.

Production wiring test: `TestOpenAILoopbackPortsAre1455Then1457` asserts
`openaiLoopbackPorts == []int{1455, 1457}` (a named package var). That is
the Hydra allow-list lock; it does not bind those ports.

### Grok loopback

* Bind `127.0.0.1:0`. Redirect `http://127.0.0.1:{port}/callback`.
* Same timeout, state, PKCE exchange against `{TokenURL}`.

### Device-code

**OpenAI:** implement the Codex JSON protocol in §1 (not RFC 8628). Poll on
403/404 until 15 minutes. Then exchange `authorization_code` at
`{issuer}/oauth/token` with redirect `{issuer}/deviceauth/callback`.

**Grok:** RFC 8628 against device + token URLs. Honour `interval`,
`authorization_pending`, `slow_down` (+5s), `expires_in` (default 300s).
Floor interval at 1s so `NaN` cannot busy-loop (OpenCode's lesson).

### Red tests

* `TestGrokDevice_SlowDownIncreasesInterval` — fake token endpoint returns
  `slow_down` once then success; assert sleep sequence. Inject a `sleep`
  func. If the implementation ignores `slow_down`, FAIL.
* `TestGrokDevice_StopsAtExpiry` — endpoint always `authorization_pending`;
  `expires_in=1`; must return error, not hang. Use fake clock/sleep.
* `TestListenFirstAvailable_UsesSecondPortWhenFirstBusy` — as above.
* `TestOpenAILoopbackPortsAre1455Then1457`.

No live OAuth in this phase.

Phase green. Commit.

## Phase 6 — OpenAI transport branch

### Construction

`NewOpenAI(apiKey, model, opts...)` → `NewOpenAIWithSource(NewStaticToken(apiKey), model, opts...)`.

`NewOpenAIWithSource` (exported):

* Static → `baseURL = DefaultOpenAIPlatformBaseURL` unless `WithBaseURL`.
* ChatGPT session → `baseURL = DefaultOpenAIChatGPTBaseURL` unless
  `WithBaseURL`.

`doGenerateItems` calls `src.Token(ctx)` per request, sets
`Authorization: Bearer {token.Value}`. If ChatGPT:

* URL is `{baseURL}/responses` (so default becomes
  `https://chatgpt.com/backend-api/codex/responses`).
* Set `ChatGPT-Account-Id` from `OAuthSession.AccountID` when non-empty.
* Parse residency from the access JWT (unverified payload, same claims path
  as OpenCode: `chatgpt_compute_residency` or
  `https://api.openai.com/auth`.chatgpt_compute_residency); if set and not
  `no_constraint`, set `x-openai-internal-codex-residency`.

`NewProviderWithSource("openai", src, model, opts...)` routes here.

### Listing

`ListAvailableModels` signature unchanged (API key / static).
Add `ListAvailableModelsWithSource(ctx, name string, src TokenSource, opts...)`.
The old function is:

```go
return ListAvailableModelsWithSource(ctx, name, NewStaticToken(apiKey), opts...)
```

For `openai` + ChatGPT session: return a copy of `StaticOpenAIChatGPT`; **do
not** HTTP. A test that points `httptest` at `api.openai.com` behaviour
fails if any request is made (`httptest` server that `t.Fatal`s on hit).

### Red tests (load-bearing)

1. `TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost`: capturing
   `http.RoundTripper` (no `WithBaseURL`). Construct via
   `NewOpenAIWithSource` with `&OAuthSession{Issuer: DefaultOpenAIIssuer,
   Access: "sess", Refresh: "r", Expiry: time.Now().Add(time.Hour)}` and
   `WithHTTPClient`. `Generate` once. Assert `captured.URL.Host == "chatgpt.com"`
   and path `== "/backend-api/codex/responses"`. RoundTripper returns this canned body (already used in
   `thinking_test.go` / `provider_correctness_test.go`):

   `{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`

   so Generate does not fail on decode.
   **Red run:** first implement `NewOpenAIWithSource` as
   `NewOpenAI(session.Access, …)` (Platform default). The test MUST FAIL
   (`Host == "api.openai.com"`). Then add the ChatGPT branch.
2. `TestOpenAI_StaticKeyDoesNotHitChatGPTHost`: `NewStaticToken("sk-test")`,
   same RoundTripper, no `WithBaseURL`. Assert host `api.openai.com` and
   path `/v1/responses`.
3. `TestOpenAI_ChatGPTSetsAccountHeader`: session `AccountID: "acct_1"`;
   assert header `ChatGPT-Account-Id` == `acct_1`.
4. `TestOpenAI_OAuth401RetriesOnceAfterRefresh`: first generate response
   401, refresh endpoint 200 with new access, second generate 200. Assert
   two generate requests and one refresh POST.
5. `TestNewProviderWithSource_RejectsClaude`:
   `NewProviderWithSource("claude", NewStaticToken("x"), "x")` errors.
6. `TestNewProvider_APIKeyStillPlatform`:
   `NewProvider("openai", "sk-test", "gpt-4.1-mini")` + RoundTripper →
   Platform host.

Phase green. Commit.

## Phase 7 — Grok `TokenSource`

### Construction

`NewGrok(apiKey, model, opts...)` unchanged empty-key error.
Internal field `apiKey string` becomes `src TokenSource`.
`doGenerateItems` (or the existing request helper) calls `src.Token(ctx)`
and sets `Authorization: Bearer`. Assert the request header map has **no**
`X-XAI-Token-Auth` (any case).

Default host remains `https://api.x.ai/v1`. Session does not change it.
Host-lock tests use the same capturing RoundTripper as Phase 6 (no
`WithBaseURL`). Path must be `/v1/responses`.

`NewProviderWithSource("grok", src, model, opts...)` works for both
`*StaticToken` and `*OAuthSession`.

### Red tests

* `TestGrok_SessionUsesAPIXAIHost`: capturing RoundTripper, session source,
  no `WithBaseURL`. Assert `Host == "api.x.ai"` and path `/v1/responses`,
  and `!strings.Contains(url, "cli-chat-proxy")`. **Red run:** temporarily
  construct with `baseURL: "https://cli-chat-proxy.grok.com/v1"` inside the
  test file only (a local helper that the production constructor must not
  use). Do not commit a production default of the proxy. The production
  constructor's default is `https://api.x.ai/v1`; the test locks it.
* `TestGrok_SessionOmitsCLITokenAuthHeader`: range `req.Header`; fail if
  any key equals `X-XAI-Token-Auth` ignoring case.
* `TestGrok_EmptyStaticKeyStillRejected`.

Phase green. Commit.

## Phase 8 — Wizard

### Flow (after provider select, before model select)

1. If `orchestrated(o)` → return `ErrOrchestrated` (no Prompter calls).
2. If `len(d.AuthMethods) == 0`: today's `resolveAPIKey` only; `Kind=CredAPIKey`
   when a key is collected, else `CredNone` (Ollama). **Do not** add a Select.
   Existing `configure_test.go` scripts stay valid.
3. Else `Select` among `d.AuthMethods` (0-based index in tests). Default index
   0 (`api_key`).
4. Switch on `AuthMethodID`:

   * `api_key`: existing `resolveAPIKey`. `Kind=CredAPIKey`. Env vars
     `OPENAI_API_KEY` / `XAI_API_KEY` only.
   * `browser_oauth`: if `TokenStore == nil`, error
     `wizard: TokenStore is required for OAuth`. If
     `Existing.Kind == CredOAuth` and `Existing.Provider == d.ID` and
     `Existing.AccessToken != ""`, `Confirm` keep existing
     (`MaskSecret(Existing.AccessToken)`); on yes, reuse Existing OAuth
     fields and skip loopback. Else run Phase 5 loopback, `Save`,
     `APIKey=""`, `Kind=CredOAuth`.
   * `device_code`: same TokenStore requirement. `Notify` verification URL
     + user code. Poll. `Save`.
   * `token_stdin`:
     * openai + `AllowEnv` + `LookupEnv("CODEX_ACCESS_TOKEN")` non-empty:
       Confirm to use it (masked). Yes → access-only OAuth (`Refresh=""`,
       `Expiry` zero, `Issuer=DefaultOpenAIIssuer`).
     * Else `Secret`. openai: if `strings.HasPrefix(value, "sk-")` then
       `Kind=CredAPIKey`; else access-only OAuth as above. grok: always
       `Kind=CredAPIKey`.
   * `import_vendor_cli`: TokenStore required. See Import. Confirm with
     `MaskSecret(access)` before Save.

4. Discover models: `ListAvailableModelsWithSource` using a `StaticToken` or
   `OAuthSession` matching `Kind`. ChatGPT therefore sees
   `StaticOpenAIChatGPT`.
5. Model + fallbacks unchanged.

### Import

Path helpers (use `o.LookupEnv`, fall back to `os.Getenv`):

* openai: `CODEX_HOME` if set, else `filepath.Join(home, ".codex", "auth.json")`
  where `home` is `os.UserHomeDir()`.
* grok: `GROK_HOME` if set, else `filepath.Join(home, ".grok", "auth.json")`.

**openai fixture shape** (ignore every other field):

```json
{
  "OPENAI_API_KEY": "sk-MUST-IGNORE",
  "tokens": {
    "access_token": "at-chatgpt",
    "refresh_token": "rt-chatgpt",
    "account_id": "acct_test"
  }
}
```

Require `tokens.access_token` and `tokens.refresh_token` as JSON strings.
Use `tokens.account_id` when present. **Ignore `OPENAI_API_KEY`.** Issuer
`DefaultOpenAIIssuer`, client id `DefaultOpenAIClientID`, token URL
`{issuer}/oauth/token`.

**grok fixture shape:**

```json
{
  "xai::api_key": {
    "key": "xai-MUST-SKIP",
    "auth_mode": "api_key"
  },
  "https://auth.x.ai::b1a00492-073a-47ea-816f-4c329264a828": {
    "key": "sess-grok",
    "auth_mode": "oidc",
    "refresh_token": "rt-grok",
    "expires_at": "2099-01-01T00:00:00Z",
    "oidc_issuer": "https://auth.x.ai",
    "oidc_client_id": "b1a00492-073a-47ea-816f-4c329264a828"
  }
}
```

Walk the map in iteration order after skipping `auth_mode == "api_key"` and
scope `xai::api_key`. Accept the first object whose `oidc_issuer` equals
`DefaultGrokOAuthIssuer` (trim `/`). Access = `key`. Token URL
`https://auth.x.ai/oauth2/token`. Parse `expires_at` as RFC3339.

Never log the file contents.

### Red tests (scripted `Prompter`, `httptest`, temp `TokenStore`)

* `TestConfigureLLM_OrchestratedReturnsErr`: `Orchestrated=boolPtr(true)` →
  `errors.Is(err, ErrOrchestrated)`, zero `Select` calls.
* `TestConfigureLLM_ChatGPTResultDoesNotPopulateAPIKey`: scripted select
  openai + browser; fake loopback by injecting an already-saved session
  (or a test hook `completeOAuth` if loopback is too heavy — prefer
  driving the real loopback against httptest authorize/token). Assert
  `res.APIKey == ""` and `res.Kind == CredOAuth`. **If the implementation
  copies access into APIKey, this test fails** — that is the billing-bug
  guard.
* `TestConfigureLLM_APIKeyKindUnchanged`: gemini (no extra select) and
  openai with selects `{providerIdx(openai), 0 /* api_key */, 0 /* model */}`
  set `Kind=CredAPIKey` and `APIKey`.
* `TestConfigureLLM_DoesNotOfferClaudeOAuth`: descriptors without methods
  skip the auth-method menu (one Select: provider).
* `TestImportGrok_SkipsAPIKeyScope`.
* `TestImportOpenAI_IgnoresPlatformKeyInAuthJSON`.

Phase green. Commit.

## Phase 9 — README

Update the LLM providers section:

* `NewProvider` still the API-key factory.
* `NewProviderWithSource` for openai/grok sessions.
* ChatGPT login uses `chatgpt.com/backend-api/codex`, not `api.openai.com`.
* Grok session uses `api.x.ai/v1`; no CLI proxy.
* `ConfigureLLM` may return `Kind=oauth` with empty `APIKey`.
* Orchestrated processes use the backplane, not this wizard.
* Claude/Gemini remain API-key.

No new dependency listed.

Phase green (`gofmt` N/A for md). Commit README. This is the last **mcplib**
source commit. Phase 10 is a different repository.

## Phase 10 — `prepare-commit-msg` is the first consumer

Repo: sibling `../prepare-commit-msg` (module
`github.com/maccavelli/prepare-commit-msg`, currently
`github.com/maccavelli/mcplib v1.4.1`). Standalone Git hook: never
orchestrated. Interactive configure already calls `wizard.ConfigureLLM`
(`internal/ui/setup.go:188`). Generation calls
`llmprovider.NewProvider(conf.ActiveProvider, apiKey, m)`
(`main.go:276`). Config is JSON, atomic, mode `0600`
(`internal/config/save_atomic.go`).

This phase does **not** put access/refresh tokens in `config.json`.
`config.json` already holds API keys at `0600`; rotating OAuth tokens go in
the MADR's `FileTokenStore` beside it so a refresh during `runAnalyzer`
does not rewrite the whole config and cannot leak into a copied
`config.json` snippet.

### 10.0 — `replace` so the unpublished mcplib API is visible

In `prepare-commit-msg/go.mod`, add:

```
replace github.com/maccavelli/mcplib => ../mcplib
```

Run `go mod tidy`. Leave the `replace` in place with a `// TODO: drop after
mcplib tag carrying Phases 1–9` comment. Removing it is a maintainer action
(tag + pin) and is **not** this phase. `git commit --no-edit` this `go.mod` /
`go.sum` change **together with the rest of Phase 10**, not as its own
commit, once the code compiles.

### 10.1 — Config schema

`internal/config/config.go` `ProviderConfig` gains **one** new field:

```go
AuthKind string `json:"auth_kind,omitempty"` // "" = API key (legacy); "oauth" = FileTokenStore session
```

No access token, no refresh token, no issuer in this struct. **Write
`AuthKind` only when it is `"oauth"`. Never write `"api_key"`.** Load:
`strings.EqualFold(pc.AuthKind, "oauth")` is OAuth; anything else is API key.

Add grok to `SupportedProviders` (today: gemini, openai, claude only —
`config.go:24-28`). OpenAI is already there. Grok must be, or
`ApplyDefaults` never templates a grok slot and OAuth grok is a second-class
path:

```go
var SupportedProviders = []string{
    llmprovider.ProviderGemini,
    llmprovider.ProviderOpenAI,
    llmprovider.ProviderClaude,
    llmprovider.ProviderGrok,
}
```

Do **not** add gateways here. Wizard can still persist a gateway if the user
picks one (`conf.Providers[res.Provider] = pc` already does).

`OAuthDir()` returns `filepath.Join(filepath.Dir(configPath), "oauth")`.
That is:

* macOS: `~/Library/Application Support/prepare-commit-msg/oauth/`
* Linux with `XDG_CONFIG_HOME`: `$XDG_CONFIG_HOME/prepare-commit-msg/oauth/`

`NewOAuthStore()` is `llmprovider.NewFileTokenStore` on that directory.

Keep `ValidateActive(provider, pc, apiKey string)` for the API-key path
(signature unchanged so existing tests compile).

Add:

```go
func IsOAuth(pc ProviderConfig) bool {
    return strings.EqualFold(pc.AuthKind, "oauth")
}

func ValidateOAuth(ctx context.Context, provider string, pc ProviderConfig, store llmprovider.TokenStore) error
```

`ValidateOAuth`: `store.Load` must return a non-nil session; `pc.Model`
non-empty. Error: `no OAuth session for provider %q; run 'prepare-commit-msg configure'`.

`runAnalyzer` calls `ValidateOAuth` when `IsOAuth(pc)`, else
`ResolveAPIKey` + `ValidateActive`.

`ResolveAPIKey` unchanged.

**Red tests** (`internal/config/config_test.go`):

* `TestSave_OAuthKindDoesNotWriteTokens`: `AuthKind=oauth`, `APIKey=""`;
  read saved `config.json`; fail if the bytes contain `"access_token"` or
  `"refresh_token"` (exact keys, not the substring `access` which would
  false-positive `active_provider`). Write this against an implementation
  that mistakenly stores tokens on `ProviderConfig` — it MUST fail — then
  keep tokens out of the struct.
* `TestValidateActive_OAuthWithoutKeyOK` with a temp `FileTokenStore` that
  has a session.
* `TestValidateActive_OAuthMissingSessionErrors`.
* `TestApplyDefaults_IncludesGrok`.
* Existing `TestConfig_SaveAndLoad` still passes (API-key file).

### 10.2 — Interactive configure

`runSetupInteractive` (`setup.go:168`):

1. Resolve config path; `store, err := llmprovider.NewFileTokenStore(oauthDir)`.
2. Seed `wizard.Result` as today for API key/model. If
   `pc.AuthKind == "oauth"`, set `Existing.Kind = wizard.CredOAuth` and
   load the session into `Existing.AccessToken` / `RefreshToken` / `Issuer` /
   `ClientID` / `AccountID` / `TokenExpiry` (for "keep existing?"). Do **not**
   put the access token in `Existing.APIKey`.
3. `wizard.Options`:
   * existing fields unchanged (`AllowEnv: !opts.NoEnv`, `Discover: true`,
     `DiscoverLimit: DiscoveryTimeout`, `NeedFallbacks: true`)
   * `TokenStore: store`
   * `OpenURL: openBrowser` (new helper; see 10.4)
   * `Orchestrated: boolPtr(false)` — this process is never a sub-server
4. After `ConfigureLLM`:
   * `Kind == CredAPIKey` or `CredNone`: `pc.AuthKind = ""`. `pc.APIKey =
     res.APIKey`. `store.Delete(ctx, res.Provider)` so a later generate
     cannot prefer a stale session.
   * `Kind == CredOAuth`: `pc.AuthKind = "oauth"`. **`pc.APIKey = ""` (do not
     copy `res.AccessToken`).** Wizard already `Save`d the TokenStore.
   * Model and fallbacks as today.
5. Drop the check `d.RequiresAPIKey && pc.APIKey == ""` for oauth results
   (`setup.go:208-210`). Replace with: if Kind is API key and descriptor
   requires a key and APIKey empty → same error; if Kind is oauth and
   `store.Load` is nil → error.

Existing interactive tests:

* Gemini `TestRunSetupInteractive_Success` input `1\ny\n…` — **unchanged**
  (no auth-method menu).
* Claude `TestRunSetupInteractive_FallbackMultiSelect` input `3\n…` —
  **unchanged**.
* **Must edit** `TestRunSetupInteractive_CoverageBranches` / `"OpenAI path"`
  (`setup_test.go:107`): today `2\ntest-key\n1\n\n\n\n\n\n`. After Phase 8
  that is wrong. Change to `2\n1\ntest-key\n1\n\n\n\n\n\n` (1-based:
  provider openai, auth `api_key`, secret, model 1, empty fallbacks, empty
  ops). Assert `AuthKind==""` and `APIKey=="test-key"` and
  `oauth/openai.json` does not exist.

**New tests** (1-based TextPrompter indices: openai=2, grok=4; auth
`api_key`=1, `import_vendor_cli`=5):

* `TestRunSetupInteractive_ImportGrokSession`: `t.Setenv("GROK_HOME", tmp)`;
  write the grok fixture from Phase 8 into
  `filepath.Join(tmp, "auth.json")`. Input `4\n5\ny\n1\n\n\n\n\n\n`
  (grok, import, confirm, model 1, rest default). Assert
  `AuthKind=="oauth"`, `APIKey==""`, `oauth/grok.json` exists, on Unix
  mode `0600`, `config.json` has no `"refresh"` / `"access"` keys. No
  network.
* `TestRunSetupInteractive_ChatGPTDoesNotCopyAccessIntoAPIKey`:
  `t.Setenv("CODEX_HOME", tmp)`; write the openai fixture. Input
  `2\n5\ny\n1\n\n\n\n\n\n`. Assert `APIKey==""`, `AuthKind=="oauth"`.

### 10.3 — Generation (`main.go` `runAnalyzer`)

Replace the `NewProvider(conf.ActiveProvider, apiKey, m)` loop body with a
helper `newActiveProvider(conf, pc, model) (llmprovider.Provider, error)`:

* `pc.AuthKind == "oauth"`: `sess, err := store.Load(ctx, conf.ActiveProvider)`;
  if sess nil, return the validate error. `sess.Store = store` so refresh
  persists. `llmprovider.NewProviderWithSource(conf.ActiveProvider, sess, m)`.
* Else: `apiKey := config.ResolveAPIKey(...)`; `ValidateActive` as today;
  `llmprovider.NewProvider(conf.ActiveProvider, apiKey, m)`.

Do **not** pass an oauth access token as the `apiKey` argument of
`NewProvider`.

**Red test** (`main_test.go` or new `main_oauth_test.go`):

* `TestRunAnalyzer_OAuthDoesNotCallNewProviderWithAccessToken`: this is
  hard if `NewProvider` is not injected. Inject at the same seam already
  used (`generateWithRetry` is already a package var). Add

  ```go
  var newProvider = llmprovider.NewProvider
  var newProviderWithSource = llmprovider.NewProviderWithSource
  ```

  in `main.go`, use them from `newActiveProvider`. Test assigns
  `newProvider` to a func that `t.Fatal`s if called, and
  `newProviderWithSource` to a stub that records `src`. Config
  `AuthKind=oauth` with a FileTokenStore session. Assert
  `newProvider` not called and `src` is `*llmprovider.OAuthSession`.
  **Write the test first against current `NewProvider(apiKey)` — it MUST
  fail** (oauth config would still go through NewProvider with empty key,
  or if someone copies access into APIKey, newProvider is called with the
  token). Then implement the helper.

Keep `generateWithRetry` as the generate seam. Auth failure still aborts
fallbacks (`main.go:286-288`).

### 10.4 — `openBrowser`

New `internal/ui/open.go`:

```go
var openBrowser = openBrowserDefault // tests may replace

func openBrowserDefault(url string) error
```

`darwin`: `exec.Command("open", url)`, `linux`: `xdg-open`, `windows`:
`cmd /c start`. Combined output discarded. Non-zero return is **not** fatal
(wizard still prints the URL). Import tests never invoke it. `open_test.go`
sets `openBrowser` to a recorder if a browser-path test is added; otherwise
the file can omit a test and `open.go` stays small enough that coverage
still clears 80% via setup tests that don't call it.

### 10.5 — Non-interactive `--yes`

Stays **API-key only**. If `opts.Yes` and the user would need OAuth, they
pass `--api-key`. Do not add `--device-code` in this phase. Document in
the prepare-commit-msg README configure section: subscription login is
interactive (`prepare-commit-msg configure` without `--yes`).

Existing `TestRunSetupNonInteractive` must still pass.

### 10.6 — README (prepare-commit-msg)

One short subsection: ChatGPT / Grok subscription via interactive
configure; tokens live under the config directory `oauth/`; `--yes` is
still API-key.

### Phase 10 green

In `prepare-commit-msg`:

```bash
LOG=/tmp/pcm-0008-phase10.log
{
  gofmt -l internal/config internal/ui main.go
  go vet ./...
  git add -- \
    go.mod go.sum README.md \
    internal/config/config.go internal/config/config_test.go \
    internal/ui/setup.go internal/ui/setup_test.go \
    internal/ui/open.go \
    main.go main_oauth_test.go
  ./scripts/go-precheck.sh
  go test -race ./...
} >"$LOG" 2>&1
GATE=$?
```

If `open_test.go` exists, add it to that `git add` list. Do not `git add -A`.
Do not `git add -u` (that would stage unrelated dirty files).

`gofmt -l` empty. Branch on `$GATE`, not on a pipeline. Then
`git commit --no-edit` **in prepare-commit-msg**. Do not commit mcplib.
Do not push.

`go-precheck.sh` with no args checks the staged `*.go` snapshot — that is
the repo's gate (`Makefile` `verify-staged`). Do not pass a guessed file
list that can omit a new test file.

Coverage floor is 80% (`Makefile` `COVERAGE_MIN`). If Phase 10 drops below,
add tests rather than lowering the floor.

## 5. Verification commands (every mcplib phase)

> **2026-09-13 correction:** The original command block below is retained as
> historical evidence but must not be used: its exit status is only the final
> `go test` status, so a preceding failure can be hidden.

```bash
LOG=/tmp/mcplib-0008-phaseN.log
{
  echo "== gofmt =="
  gofmt -l llmprovider wizard
  echo "== vet =="
  go vet ./llmprovider ./wizard
  echo "== lint =="
  make lint
  echo "== test =="
  go test ./llmprovider ./wizard
} >"$LOG" 2>&1
echo EXIT:$?
```

Use this replacement, which records and checks each command independently:

```bash
LOG_BASE=/tmp/mcplib-0008-phaseN
GATE=0

gofmt -l llmprovider wizard >"${LOG_BASE}.gofmt" 2>&1
GOFMT_STATUS=$?
if [ "$GOFMT_STATUS" -ne 0 ] || [ -s "${LOG_BASE}.gofmt" ]; then GATE=1; fi

go vet ./llmprovider ./wizard >"${LOG_BASE}.vet" 2>&1
VET_STATUS=$?
if [ "$VET_STATUS" -ne 0 ]; then GATE=1; fi

make lint >"${LOG_BASE}.lint" 2>&1
LINT_STATUS=$?
if [ "$LINT_STATUS" -ne 0 ]; then GATE=1; fi

go test ./llmprovider ./wizard >"${LOG_BASE}.test" 2>&1
TEST_STATUS=$?
if [ "$TEST_STATUS" -ne 0 ]; then GATE=1; fi

printf 'gofmt=%s vet=%s lint=%s test=%s gate=%s\n' \
  "$GOFMT_STATUS" "$VET_STATUS" "$LINT_STATUS" "$TEST_STATUS" "$GATE"
test "$GATE" -eq 0
```

`gofmt -l` must print **no paths**. Do not pipe `make lint` into `tail`
before inspecting its independently captured status.

Full-module `go test ./...` once after Phase 9.

## 6. Acceptance criteria

All must be true, each with a named test that was seen to fail:

| # | Criterion | Test |
|---|---|---|
| A1 | ChatGPT session does not call `api.openai.com` | `TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost` |
| A2 | Platform key does not call ChatGPT host | `TestOpenAI_StaticKeyDoesNotHitChatGPTHost` |
| A3 | `NewProvider("openai", "sk-…", …)` still Platform | `TestNewProvider_APIKeyStillPlatform` |
| A4 | Grok session host is `api.x.ai` | `TestGrok_SessionUsesAPIXAIHost` |
| A5 | Grok session has no `X-XAI-Token-Auth` | `TestGrok_SessionOmitsCLITokenAuthHeader` |
| A6 | `claude`/`gemini`/gateways have no OAuth methods | `TestDescriptors_NoOAuthOnOtherProviders` |
| A7 | Token files are `0600` | `TestFileTokenStore_SaveMode0600` |
| A8 | Refresh persists before adopting the new token | `TestOAuthSession_RefreshPersistsBeforeReturn` |
| A9 | Device-code honours `slow_down` and expiry | `TestGrokDevice_SlowDownIncreasesInterval`, `TestGrokDevice_StopsAtExpiry` |
| A10 | OpenAI loopback is 1455 then 1457 only | `TestOpenAILoopbackPortsAre1455Then1457`, `TestListenFirstAvailable_UsesSecondPortWhenFirstBusy` |
| A11 | OAuth `Result.APIKey` is empty | `TestConfigureLLM_ChatGPTResultDoesNotPopulateAPIKey` |
| A12 | Orchestrated wizard returns `ErrOrchestrated` | `TestConfigureLLM_OrchestratedReturnsErr` |
| A13 | `NewProviderWithSource` rejects non-openai/grok | `TestNewProviderWithSource_RejectsClaude` |
| A14 | `go.mod` require list is unchanged | `git diff go.mod` empty of `+require` after Phase 9 |
| A15 | `make lint` and `go test ./...` exit 0 | Phase 9 wrap-up |
| A16 | prepare-commit-msg `config.json` never contains oauth access/refresh | `TestSave_OAuthKindDoesNotWriteTokens` |
| A17 | Hook generate with `auth_kind=oauth` uses `NewProviderWithSource`, never `NewProvider` with the access token | `TestRunAnalyzer_OAuthDoesNotCallNewProviderWithAccessToken` |
| A18 | Importing `~/.codex/auth.json` leaves `APIKey` empty | `TestRunSetupInteractive_ChatGPTDoesNotCopyAccessIntoAPIKey` |
| A19 | Gemini/Claude interactive scripts still pass (no extra auth menu) | existing `TestRunSetupInteractive_Success`, `TestRunSetupInteractive_FallbackMultiSelect` |

## 7. Rollout and rollback

**Rollout.** After Phase 9, `prepare-commit-msg` Phase 10 uses a `replace`
to local `mcplib`. A later maintainer tag of mcplib drops the replace.
Users of the hook: existing `config.json` API keys keep working; ChatGPT/Grok
subscription requires a new interactive `prepare-commit-msg configure`.
MagicDev and MagicTools are not shipped in this plan.

**Rollback.** Revert Phase 10 in prepare-commit-msg first (hook returns to
API-key-only `NewProvider`). Then revert mcplib phases newest-first. API-key
constructors never change their signatures.

**Orchestrated fleet.** No rollout: `backplane.go` is not modified. Sub-servers
that already call `NewBackplaneClient` keep doing so.

## 8. Risks

| Risk | Mitigation in this plan |
|---|---|
| ChatGPT token sent to Platform (wrong bill) | A1 + A11; `APIKey` empty in OAuth results |
| Grok session sent to CLI proxy (426) | A4; default host unchanged |
| Impersonating Grok CLI | A5; referrer `mcplib` |
| OpenAI random loopback port (Hydra reject) | A10 |
| Refresh-token race (OpenCode stale disk) | A8 |
| ToS expansion to Claude/Gemini | A6 |
| Hanging device-code | A9 |
| Orchestrated OAuth | A12 |
| Hook copies ChatGPT token into `api_key` | A16, A17, A18 |

## 9. Out of scope (do not implement "while here")

* MagicDev and MagicTools consumer wiring.
* MagicTools orchestrator login.
* Putting OAuth tokens inside prepare-commit-msg `config.json`.
* Non-interactive (`--yes`) subscription login.
* `ExternalHelper` / customer OIDC for Grok.
* OS keyring `TokenStore`.
* Changing `StaticOpenAI` (Platform catalog).
* Live ChatGPT listing against the Codex backend.
* Sending `X-XAI-Token-Auth`.
* New Go module dependencies.

## 10. File summary (expected)

**New**

* `llmprovider/token.go`, `token_test.go`
* `llmprovider/tokenstore.go`, `tokenstore_file.go`, `tokenstore_file_unix.go`, `tokenstore_file_windows.go`, `tokenstore_test.go`
* `llmprovider/oauth_session.go`, `oauth_session_test.go`
* `llmprovider/oauth_pkce.go`, `oauth_loopback.go`, `oauth_device.go` and tests
* `llmprovider/openai_chatgpt.go`, `openai_chatgpt_test.go`
* `llmprovider/grok_oauth_test.go`
* `wizard/auth.go`, `wizard/import.go`, `wizard/auth_test.go`, `wizard/import_test.go`

**Modified**

* `llmprovider/descriptor.go`, `descriptor_test.go`
* `llmprovider/openai.go` (and existing openai tests)
* `llmprovider/grok.go` (and existing grok tests)
* `llmprovider/provider.go`
* `llmprovider/models_catalog.go`, `models_catalog_test.go`
* `llmprovider/discovery.go`, `discovery_test.go`
* `wizard/configure.go`, `configure_test.go`
* `README.md`

**prepare-commit-msg (Phase 10)**

* Modified: `go.mod`, `go.sum`, `internal/config/config.go`, `config_test.go`,
  `internal/ui/setup.go`, `setup_test.go`, `main.go`, `main_test.go` (or
  `main_oauth_test.go`), `README.md`
* New: `internal/ui/open.go` (and `open_test.go` if the helper needs a
  GOOS-stub test)

**Untouched in mcplib (assert in Phase 9 with `git diff --stat`)**

* `llmprovider/claude.go`, `gemini.go`, `ollama.go`, `huggingface.go`, `kilo.go`, `opencode.go`
* `backplane.go`, `orchestrator.go`
* `go.mod` require block

## 11. Deviation log

| Date | Phase | Finding | Decision | Files added to phase |
|---|---|---|---|---|
| 2026-09-12 | (plan) | Maintainer named prepare-commit-msg as first consumer | Add Phase 10; MagicDev/MagicTools stay out | Phase 10 in this plan |
| 2026-09-13 | (plan sweep) | Orchestrated-nil was specified backwards; OpenAI setup script would break; host tests used WithBaseURL; loopback tests would fight port 1455; AuthKind "api_key" would churn configs; token_stdin heuristic was fuzzy | Corrections in §0.1, applied in place | none (plan only) |
| 2026-09-13 | 1–2 audit | Phase 2 commit `9836e94` landed with a red format/lint log; the shared verification block masked intermediate failures; Unix mode test was platform-neutral; `..` validation lacked an isolated case; Phase 3-only scaffold made Phase 2 lint-red; Phase 1 and 2 exported APIs failed per-file `golint` | Stop before Phase 3. Run the approved Phase 2 corrective pass, replace the verification recipe, defer the unused scaffold to Phase 3, prove the adjusted gates in a scratch clone, and record exact results | `docs/0008-PLAN-subscription-auth-for-llm-providers.md`, `llmprovider/token.go`, `token_test.go`, `tokenstore.go`, `tokenstore_file.go`, `tokenstore_file_unix.go`, `tokenstore_file_windows.go`, `tokenstore_test.go`, new `tokenstore_file_unix_test.go`; Phase 3 adds `tokenstore.go` |
| 2026-09-13 | 3 | `ChatGPT()` requires exported `DefaultOpenAIIssuer`, but the phase table deferred its required `oauth_constants.go` file to Phase 5 | Create `oauth_constants.go` in Phase 3 with `DefaultOpenAIIssuer`; Phase 5 extends it with the remaining locked constants. No MADR amendment: the constant value, API, and file-placement decision are unchanged | new `llmprovider/oauth_constants.go` |
| 2026-09-13 | 3 | The first constant-placement correction omitted §0.1.11's xAI token URL fallback, which Phase 3 also needs and §0.1.8 requires in `oauth_constants.go` | Add private `defaultGrokOAuthTokenURL` beside `DefaultOpenAIIssuer` in Phase 3; Phase 5 still adds all other OAuth constants. No MADR amendment | no additional file |
| 2026-09-13 | 3 | `make lint` classified the approved private name `defaultGrokOAuthTokenURL` as a G101 hardcoded-credential finding because it contains `Token`; repeated `Authorization` literals also tripped `goconst` | Rename the private endpoint to `defaultGrokOAuthRefreshURL` and deduplicate the header with a private Phase 3 constant; do not suppress either lint rule. No MADR amendment | no additional file |
| 2026-09-13 | 4 | The expected-pass `TestDescriptors_NoOAuthOnOtherProviders` cannot compile at the Phase 3 boundary because the Phase 4 auth-method types and `AuthMethods` field do not yet exist | Add the empty compile scaffold first, observe the test pass, prove it with a non-OAuth-provider scratch mutation, then add the remaining red tests and implementation. No MADR amendment | no additional file |

## 12. Execution record

### Phase 1 — landed before execution-record update

Commit `11f1810` added `Token`, `TokenSource`, and `StaticToken`. Its captured
green run was:

```text
== gofmt ==
== vet ==
== lint ==
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
0 issues.
== test ==
ok  github.com/maccavelli/mcplib/llmprovider  0.989s
```

The required pre-implementation red output was not recorded. The 2026-09-13
audit later proved `TestStaticToken_ReturnsBearer` and
`TestStaticToken_EmptyValueStillReturnsToken` against deliberate breakage in a
scratch clone; both failed on their intended assertions.

### Phase 2 — corrective pass complete

Commit `9836e94` added the file token store, but its captured run was not green:

```text
== gofmt ==
llmprovider/tokenstore.go
== vet ==
== lint ==
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
llmprovider/tokenstore_file.go:85:17: Error return value of `os.Remove` is not checked (errcheck)
llmprovider/tokenstore_file.go:87:12: Error return value of `tmp.Close` is not checked (errcheck)
llmprovider/tokenstore_file.go:99:11: Error return value is not checked (errcheck)
llmprovider/tokenstore.go:17:1: File is not properly formatted (gofmt)
llmprovider/tokenstore.go:27:2: field mu is unused (unused)
llmprovider/tokenstore.go:28:2: field inflight is unused (unused)
llmprovider/tokenstore.go:32:6: type tokenFuture is unused (unused)
7 issues:
* errcheck: 3
* gofmt: 1
* unused: 3
make: *** [lint] Error 1
== test ==
ok  github.com/maccavelli/mcplib/llmprovider  0.853s
```

The required pre-implementation red output was not recorded. The 2026-09-13
audit subsequently observed all four Phase 2 test groups fail against deliberate
breakage in a scratch clone. That historical omission cannot be recreated as
contemporaneous evidence and the original commit was not rewritten.

The approved corrective pass added exported API documentation, removed the
Phase 3-only single-flight scaffold, made temporary-file cleanup and close
errors explicit, checked the final-file `chmod`, moved the mode assertion to a
Unix-tagged test file, and added `open..ai` as an isolated `..` traversal case.
No MADR amendment was needed because these corrections do not change an
architectural decision.

Before relying on the two adjusted checks, a scratch clone was deliberately
broken by changing the Unix permission helper to `0644` and removing only the
`..` rejection. The mutations were inspected before the complete targeted test
output was read:

```text
=== RUN   TestFileTokenStore_SaveMode0600
    tokenstore_file_unix_test.go:35: mode = 644, want 0600
--- FAIL: TestFileTokenStore_SaveMode0600 (0.00s)
=== RUN   TestFileTokenStore_RejectsPathTraversal
    tokenstore_test.go:72: Save("open..ai") returned nil error; want error
--- FAIL: TestFileTokenStore_RejectsPathTraversal (0.00s)
FAIL
FAIL  github.com/maccavelli/mcplib/llmprovider  0.647s
FAIL
```

The corrected targeted suite then passed all seven Phase 1–2 tests. Per-file
`golint` produced no output for each staged Go file. The corrected independent
gate results were:

```text
gofmt: exit 0, no output
go vet: exit 0, no output
make lint: exit 0
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
0 issues.
go test: exit 0
ok  github.com/maccavelli/mcplib/llmprovider  0.377s
ok  github.com/maccavelli/mcplib/wizard       1.134s
```

`GOOS=windows GOARCH=amd64 go test -c ./llmprovider` also exited zero, and
`go list` confirmed `tokenstore_file_unix_test.go` was absent from the Windows
test file set. The corrective commit containing this record restores Phase 2 to
a green boundary before Phase 3; it is not a new implementation phase.

### Phase 3 — complete

The commit containing this entry adds `OAuthSession.Token()` refresh,
persist-before-adoption, per-session single-flight, two-minute refresh skew,
issuer detection, and the two constants moved forward by the approved
deviations. It does not add live OAuth, alter `go.mod`, begin Phase 4, or amend
the MADR; the deviations changed implementation ordering and a private name,
not architecture.

The four tests were written first. Their initial complete result was red because
the Phase 3 API did not exist:

```text
# github.com/maccavelli/mcplib/llmprovider [github.com/maccavelli/mcplib/llmprovider.test]
llmprovider/oauth_session_test.go:72:20: session.Token undefined (type *OAuthSession has no field or method Token)
llmprovider/oauth_session_test.go:115:23: session.Token undefined (type *OAuthSession has no field or method Token)
llmprovider/oauth_session_test.go:175:22: session.Token undefined (type *OAuthSession has no field or method Token)
llmprovider/oauth_session_test.go:184:21: session.Token undefined (type *OAuthSession has no field or method Token)
llmprovider/oauth_session_test.go:193:21: session.Token undefined (type *OAuthSession has no field or method Token)
llmprovider/oauth_session_test.go:208:27: undefined: DefaultOpenAIIssuer
llmprovider/oauth_session_test.go:209:36: undefined: DefaultOpenAIIssuer
llmprovider/oauth_session_test.go:215:22: session.ChatGPT undefined (type *OAuthSession has no field or method ChatGPT)
FAIL  github.com/maccavelli/mcplib/llmprovider [build failed]
FAIL
```

After implementation, the test instrument was proved in a scratch clone. The
verified mutations adopted memory before a failing save, disabled the inflight
join, reduced the skew to one minute, and removed trailing-slash normalization.
The complete result showed each named test fail on its intended assertion:

```text
=== RUN   TestOAuthSession_RefreshPersistsBeforeReturn
    oauth_session_test.go:77: session.Access = "new-access", want old-access
    oauth_session_test.go:80: session.Refresh = "new-refresh", want old-refresh
--- FAIL: TestOAuthSession_RefreshPersistsBeforeReturn (0.00s)
=== RUN   TestOAuthSession_SingleFlight
    oauth_session_test.go:150: refresh request count = 2, want 1
--- FAIL: TestOAuthSession_SingleFlight (0.00s)
=== RUN   TestOAuthSession_SkewsTwoMinutes
    oauth_session_test.go:198: inside-skew token = "current-access", calls = 0; want refreshed-access, 1
--- FAIL: TestOAuthSession_SkewsTwoMinutes (0.00s)
=== RUN   TestOAuthSession_ChatGPTDetectsIssuer
=== RUN   TestOAuthSession_ChatGPTDetectsIssuer/exact
=== RUN   TestOAuthSession_ChatGPTDetectsIssuer/trailing_slash
    oauth_session_test.go:216: ChatGPT() = false, want true
=== RUN   TestOAuthSession_ChatGPTDetectsIssuer/different_issuer
=== RUN   TestOAuthSession_ChatGPTDetectsIssuer/empty
--- FAIL: TestOAuthSession_ChatGPTDetectsIssuer (0.00s)
    --- PASS: TestOAuthSession_ChatGPTDetectsIssuer/exact (0.00s)
    --- FAIL: TestOAuthSession_ChatGPTDetectsIssuer/trailing_slash (0.00s)
    --- PASS: TestOAuthSession_ChatGPTDetectsIssuer/different_issuer (0.00s)
    --- PASS: TestOAuthSession_ChatGPTDetectsIssuer/empty (0.00s)
FAIL
FAIL  github.com/maccavelli/mcplib/llmprovider  0.656s
FAIL
```

The first `make lint` run then exposed the private-name false positive recorded
in the deviation log plus repeated authorization-header literals. After the
approved rename and in-scope deduplication, all per-file `golint` checks
produced no output and the independent phase gates were green:

```text
gofmt: exit 0, no output
go vet: exit 0, no output
make lint: exit 0
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
0 issues.
go test: exit 0
ok  github.com/maccavelli/mcplib/llmprovider  0.703s
ok  github.com/maccavelli/mcplib/wizard       1.963s
go test -race ./llmprovider -run 'TestOAuthSession_' -count=1: exit 0
ok  github.com/maccavelli/mcplib/llmprovider  1.977s
```
