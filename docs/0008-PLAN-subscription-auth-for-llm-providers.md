---
status: complete
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

**Phase 8 deviation (2026-09-13):** the `TokenStore` comment above originally
limited the requirement to browser/device/import. OpenAI `token_stdin` can also
produce `CredOAuth`; when it does, `TokenStore` is required and the access-only
session is saved before return. Static OpenAI and Grok token-stdin credentials
still require no store. This keeps Phase 10's persist-before-clearing contract
true without moving credential persistence into each consumer.

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

### Approved public flow boundary (2026-09-13)

The original Phase 5 text specified complete login flows but no callable API.
Phase 8 is in the separate `wizard` package and cannot invoke unexported
`llmprovider` functions. The maintainer approved this Phase 5 boundary:

```go
type OAuthFlowOptions struct {
    HTTPClient   *http.Client
    OpenURL      func(string) error
    InputCode    func(context.Context) (string, error)
    NotifyDevice func(verificationURL, userCode string)
    ClientID     string
    Issuer       string
}

func LoginBrowserOAuth(
    ctx context.Context,
    provider string,
    opts OAuthFlowOptions,
) (*OAuthSession, error)

func LoginDeviceOAuth(
    ctx context.Context,
    provider string,
    opts OAuthFlowOptions,
) (*OAuthSession, error)
```

Empty `ClientID` and `Issuer` use the locked provider defaults. Grok's
documented environment overrides are applied when the corresponding option is
empty. Private clock and sleep fields provide deterministic package tests.
Unsupported providers return an error. Phase 8 passes its prompter operations
through the callbacks and persists the returned session. This adds no files to
the phase and does not change the MADR's architecture.

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
     * **Approved Phase 8 correction (2026-09-13):** if either OpenAI path
       classifies the value as access-only OAuth, require `TokenStore` with the
       same error as browser/device/import, attach it to the session, and
       `Save` before returning. The original bullets omitted persistence.
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
~~`cmd /c start`~~ `rundll32.exe url.dll,FileProtocolHandler` (replaced by
the approved 2026-09-14 execution amendment below). Combined output discarded. Non-zero return is **not** fatal
(wizard still prints the URL). Import tests never invoke it. `open_test.go`
sets `openBrowser` to a recorder if a browser-path test is added; otherwise
the file can omit a test and `open.go` stays small enough that coverage
still clears 80% via setup tests that don't call it.

> **2026-09-14 execution amendment.** The first staged lint run after fixing
> the snapshot topology rejected all three variable-URL subprocess calls as
> G204. Validate an absolute HTTP(S) authorization URL and reject control
> characters before launching. macOS and Linux retain their direct non-shell
> commands. Windows uses `rundll32.exe url.dll,FileProtocolHandler` instead of
> the originally specified `cmd /c start`, removing the shell parsing boundary.
> Add an invalid-URL regression test and use narrowly justified G204 annotations
> only on the validated direct argument launches.

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

> **2026-09-13 execution amendment.** The original invocation above is still
> required, but its first Phase 10 run proved that the script's flat temporary
> snapshot made the staged `replace ... => ../mcplib` resolve to a missing
> directory. Before rerunning the gate, update `scripts/go-precheck.sh` to put
> the staged consumer under a same-named child of its temporary root and link
> the real sibling `mcplib` at the corresponding `../mcplib` location. This
> preserves the approved local-replace topology while the consumer files remain
> an isolated staged snapshot.

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
  `main_oauth_test.go`), `README.md`, and, by the approved 2026-09-13 Phase 10
  deviation, `scripts/go-precheck.sh`
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
| 2026-09-13 | 5 | Browser and device flows had no callable API, but Phase 8's separate `wizard` package must invoke them without modifying Phase 5 files | Add `OAuthFlowOptions`, `LoginBrowserOAuth`, and `LoginDeviceOAuth` in Phase 5, with private time seams and provider defaults. No MADR amendment | no additional file |
| 2026-09-13 | 8 | OpenAI `token_stdin` can return `CredOAuth`, but the plan required no `TokenStore` or save while Phase 10 clears `APIKey` and assumes every OAuth result was already persisted | Require `TokenStore` only when token-stdin classification produces OpenAI OAuth, then save the access-only session before return. Static OpenAI and Grok keys remain store-free. No MADR amendment: this enforces the accepted persistence boundary rather than changing it | no additional file |
| 2026-09-13 | 10 | The required staged `go-precheck.sh` run failed because its flat temporary snapshot could not resolve the approved sibling `../mcplib` replacement; the script itself was untouched when the failure was reproduced | Preserve the sibling topology inside the script's isolated temporary root and link the real local `mcplib` at the staged module's `../mcplib` path. No MADR amendment: authentication, persistence, and rollout decisions are unchanged | `scripts/go-precheck.sh` |
| 2026-09-14 | 10 | After the snapshot fix let the staged gate reach lint, G204 rejected every plan-prescribed browser subprocess that received a variable URL; the Windows `cmd /c start` route also exposed that URL to shell parsing | Validate absolute HTTP(S) URLs before launch; keep direct non-shell macOS/Linux launchers; replace Windows `cmd` with `rundll32.exe url.dll,FileProtocolHandler`; add invalid-URL coverage and narrow G204 annotations after validation. No MADR amendment: the browser handoff behavior and auth decisions are unchanged | no additional file (`internal/ui/open.go` and `open_test.go` were already in Phase 10) |

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

### Phase 4 — complete

The commit containing this entry adds the `AuthMethodID` constants,
`AuthMethod`, and copied `ProviderDescriptor.AuthMethods` lists. Only OpenAI
and Grok receive lists; `RequiresAPIKey` and every other descriptor remain
unchanged. It does not begin Phase 5 or amend the MADR.

Following the approved sequence, the empty compile scaffold was added first.
The non-OAuth test then passed before any method lists existed:

```text
=== RUN   TestDescriptors_NoOAuthOnOtherProviders
--- PASS: TestDescriptors_NoOAuthOnOtherProviders (0.00s)
PASS
ok  github.com/maccavelli/mcplib/llmprovider  0.645s
```

That expected-pass gate was proved in a scratch clone by assigning
`browser_oauth` to Claude. The verified mutation produced the intended complete
failure:

```text
=== RUN   TestDescriptors_NoOAuthOnOtherProviders
    descriptor_test.go:15: provider "claude" unexpectedly offers OAuth method "browser_oauth"
--- FAIL: TestDescriptors_NoOAuthOnOtherProviders (0.00s)
FAIL
FAIL  github.com/maccavelli/mcplib/llmprovider  0.599s
FAIL
```

The remaining tests were then added before populating the method lists. The
complete targeted run kept the non-OAuth gate green while the three new
requirements failed on their intended assertions:

```text
=== RUN   TestDescriptors_NoOAuthOnOtherProviders
--- PASS: TestDescriptors_NoOAuthOnOtherProviders (0.00s)
=== RUN   TestDescriptors_OpenAIAndGrokOfferOAuth
    descriptor_test.go:97: openai AuthMethods = []llmprovider.AuthMethod(nil), want []llmprovider.AuthMethod{llmprovider.AuthMethod{ID:"api_key", Label:"OpenAI API key", Detail:"Platform billing (`api.openai.com`)", Interactive:true, HeadlessOK:true}, llmprovider.AuthMethod{ID:"browser_oauth", Label:"Sign in with ChatGPT", Detail:"Plus/Pro/Business/Edu/Enterprise plan", Interactive:true, HeadlessOK:false}, llmprovider.AuthMethod{ID:"device_code", Label:"Sign in with ChatGPT (device code)", Detail:"", Interactive:true, HeadlessOK:true}, llmprovider.AuthMethod{ID:"token_stdin", Label:"Paste a ChatGPT access token or API key", Detail:"", Interactive:true, HeadlessOK:true}, llmprovider.AuthMethod{ID:"import_vendor_cli", Label:"Import ~/.codex/auth.json", Detail:"", Interactive:true, HeadlessOK:true}}
    descriptor_test.go:97: grok AuthMethods = []llmprovider.AuthMethod(nil), want []llmprovider.AuthMethod{llmprovider.AuthMethod{ID:"api_key", Label:"xAI API key", Detail:"console.x.ai billing (`api.x.ai`)", Interactive:true, HeadlessOK:true}, llmprovider.AuthMethod{ID:"browser_oauth", Label:"Sign in with xAI", Detail:"", Interactive:true, HeadlessOK:false}, llmprovider.AuthMethod{ID:"device_code", Label:"Sign in with xAI (device code)", Detail:"", Interactive:true, HeadlessOK:true}, llmprovider.AuthMethod{ID:"token_stdin", Label:"Paste an xAI API key", Detail:"", Interactive:true, HeadlessOK:true}, llmprovider.AuthMethod{ID:"import_vendor_cli", Label:"Import ~/.grok/auth.json", Detail:"", Interactive:true, HeadlessOK:true}}
--- FAIL: TestDescriptors_OpenAIAndGrokOfferOAuth (0.00s)
=== RUN   TestDescriptors_CoverEveryRegisteredProvider
    descriptor_test.go:131: descriptor "openai" does not offer required auth method "api_key"
    descriptor_test.go:131: descriptor "openai" does not offer required auth method "browser_oauth"
    descriptor_test.go:131: descriptor "grok" does not offer required auth method "api_key"
    descriptor_test.go:131: descriptor "grok" does not offer required auth method "browser_oauth"
--- FAIL: TestDescriptors_CoverEveryRegisteredProvider (0.00s)
=== RUN   TestDescriptors_DerivedFieldsMatchSource
--- PASS: TestDescriptors_DerivedFieldsMatchSource (0.00s)
=== RUN   TestDescriptors_StableOrderAndDefensiveCopy
    descriptor_test.go:192: Descriptors() returned no authentication methods
--- FAIL: TestDescriptors_StableOrderAndDefensiveCopy (0.00s)
=== RUN   TestDescriptors_NoStaleModels
--- PASS: TestDescriptors_NoStaleModels (0.00s)
=== RUN   TestDescriptors_EveryDescriptorIsConstructible
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/gemini
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/openai
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/claude
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/grok
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/opencode-zen
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/opencode-go
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/huggingface
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/kilo
=== RUN   TestDescriptors_EveryDescriptorIsConstructible/ollama
--- PASS: TestDescriptors_EveryDescriptorIsConstructible (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/gemini (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/openai (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/claude (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/grok (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/opencode-zen (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/opencode-go (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/huggingface (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/kilo (0.00s)
    --- PASS: TestDescriptors_EveryDescriptorIsConstructible/ollama (0.00s)
FAIL
FAIL  github.com/maccavelli/mcplib/llmprovider  0.589s
FAIL
```

After populating and defensively copying the exact ordered lists, the targeted
descriptor run passed. Per-file `golint` produced no output and the independent
phase gates were green:

```text
gofmt: exit 0, no output
go vet: exit 0, no output
make lint: exit 0
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
0 issues.
go test: exit 0
ok  github.com/maccavelli/mcplib/llmprovider  0.630s
ok  github.com/maccavelli/mcplib/wizard       0.853s
```

### Phase 5 — complete

The commit containing this entry adds the approved public browser/device login
boundary, S256 PKCE, OpenAI's registered-port loopback and Codex JSON device
protocol, and Grok's ephemeral loopback and RFC 8628 device protocol. Grok
discovery falls back to the locked production token/device endpoints, and its
documented issuer/client environment overrides apply only when explicit options
are empty. No live OAuth was performed, no dependency was added, and Phase 6
has not begun.

The Phase 5 tests were written first. Their initial complete targeted result was
red because none of the Phase 5 API or helpers existed:

```text
# github.com/maccavelli/mcplib/llmprovider [github.com/maccavelli/mcplib/llmprovider.test]
llmprovider/oauth_device_test.go:36:38: undefined: deviceAuthorizationGrantType
llmprovider/oauth_device_test.go:49:18: undefined: LoginDeviceOAuth
llmprovider/oauth_device_test.go:49:71: undefined: OAuthFlowOptions
llmprovider/oauth_device_test.go:94:12: undefined: LoginDeviceOAuth
llmprovider/oauth_device_test.go:94:65: undefined: OAuthFlowOptions
llmprovider/oauth_device_test.go:161:18: undefined: LoginDeviceOAuth
llmprovider/oauth_device_test.go:161:73: undefined: OAuthFlowOptions
llmprovider/oauth_loopback_test.go:26:25: undefined: listenFirstAvailable
llmprovider/oauth_loopback_test.go:44:22: undefined: listenFirstAvailable
llmprovider/oauth_loopback_test.go:58:24: undefined: openaiLoopbackPorts
llmprovider/oauth_loopback_test.go:58:24: too many errors
FAIL github.com/maccavelli/mcplib/llmprovider [build failed]
FAIL
```

After implementation, every new gate was proved in a scratch copy. The
verified mutations truncated the PKCE digest; reversed the OpenAI ports;
disabled second-port fallback and device-code guidance; changed the OpenAI
originator; accepted mismatched state; ignored Grok environment overrides;
changed the discovery fallback and Grok redirect host; admitted an unsupported
provider; changed the OpenAI device endpoint; ignored `slow_down`; and extended
the one-second device expiry. The complete test result was:

```text
=== RUN   TestGrokDevice_SlowDownIncreasesInterval
=== PAUSE TestGrokDevice_SlowDownIncreasesInterval
=== RUN   TestGrokDevice_StopsAtExpiry
=== PAUSE TestGrokDevice_StopsAtExpiry
=== RUN   TestOpenAIDevice_UsesCodexProtocol
=== PAUSE TestOpenAIDevice_UsesCodexProtocol
=== RUN   TestListenFirstAvailable_UsesSecondPortWhenFirstBusy
=== PAUSE TestListenFirstAvailable_UsesSecondPortWhenFirstBusy
=== RUN   TestListenFirstAvailable_ErrorsWhenAllBusy
=== PAUSE TestListenFirstAvailable_ErrorsWhenAllBusy
=== RUN   TestOpenAILoopbackPortsAre1455Then1457
=== PAUSE TestOpenAILoopbackPortsAre1455Then1457
=== RUN   TestBuildAuthorizeURL_OpenAIContract
=== PAUSE TestBuildAuthorizeURL_OpenAIContract
=== RUN   TestOAuthCallback_RejectsStateMismatch
=== PAUSE TestOAuthCallback_RejectsStateMismatch
=== RUN   TestResolveOAuthFlowConfig_GrokEnvironmentOverrides
    oauth_loopback_test.go:141: resolved issuer/client = ("https://auth.x.ai", "b1a00492-073a-47ea-816f-4c329264a828")
--- FAIL: TestResolveOAuthFlowConfig_GrokEnvironmentOverrides (0.00s)
=== RUN   TestOAuthEndpointsFor_GrokFallsBackAfterDiscoveryFailure
=== PAUSE TestOAuthEndpointsFor_GrokFallsBackAfterDiscoveryFailure
=== RUN   TestLoginBrowserOAuth_GrokCompletesCallbackAndExchange
=== PAUSE TestLoginBrowserOAuth_GrokCompletesCallbackAndExchange
=== RUN   TestOAuthLogin_RejectsUnsupportedProvider
=== PAUSE TestOAuthLogin_RejectsUnsupportedProvider
=== RUN   TestPKCE_ChallengeIsS256
=== PAUSE TestPKCE_ChallengeIsS256
=== CONT  TestGrokDevice_SlowDownIncreasesInterval
=== CONT  TestBuildAuthorizeURL_OpenAIContract
=== CONT  TestLoginBrowserOAuth_GrokCompletesCallbackAndExchange
=== CONT  TestOAuthLogin_RejectsUnsupportedProvider
    oauth_loopback_test.go:250: LoginBrowserOAuth() error = nil, want unsupported-provider error
--- FAIL: TestOAuthLogin_RejectsUnsupportedProvider (0.00s)
=== CONT  TestListenFirstAvailable_UsesSecondPortWhenFirstBusy
=== CONT  TestPKCE_ChallengeIsS256
=== NAME  TestBuildAuthorizeURL_OpenAIContract
    oauth_loopback_test.go:97: authorization query = map[client_id:[app_EMoamEEZ73f0CkXaXp7hrann] code_challenge:[challenge] code_challenge_method:[S256] codex_cli_simplified_flow:[true] id_token_add_organizations:[true] originator:[broken-client] redirect_uri:[http://localhost:1455/auth/callback] response_type:[code] scope:[openid profile email offline_access api.connectors.read api.connectors.invoke] state:[state]], want map[client_id:[app_EMoamEEZ73f0CkXaXp7hrann] code_challenge:[challenge] code_challenge_method:[S256] codex_cli_simplified_flow:[true] id_token_add_organizations:[true] originator:[mcplib] redirect_uri:[http://localhost:1455/auth/callback] response_type:[code] scope:[openid profile email offline_access api.connectors.read api.connectors.invoke] state:[state]]
--- FAIL: TestBuildAuthorizeURL_OpenAIContract (0.00s)
=== CONT  TestOAuthEndpointsFor_GrokFallsBackAfterDiscoveryFailure
=== CONT  TestOpenAIDevice_UsesCodexProtocol
=== NAME  TestPKCE_ChallengeIsS256
    oauth_pkce_test.go:28: challenge = "rQ", want independently computed S256 "rQoH2exAv8aY9ocQTAt3jGKHHJudYlN24PSuyyIrtOk"
--- FAIL: TestPKCE_ChallengeIsS256 (0.00s)
=== CONT  TestOAuthCallback_RejectsStateMismatch
=== NAME  TestOAuthEndpointsFor_GrokFallsBackAfterDiscoveryFailure
    oauth_loopback_test.go:164: fallback endpoints = llmprovider.oauthEndpoints{Authorization:"https://issuer.example/oauth2/authorize", Token:"https://issuer.example/oauth2/token", Device:"https://auth.x.ai/oauth2/device/code"}
--- FAIL: TestOAuthEndpointsFor_GrokFallsBackAfterDiscoveryFailure (0.00s)
=== CONT  TestOpenAILoopbackPortsAre1455Then1457
    oauth_loopback_test.go:59: openaiLoopbackPorts = [1457 1455], want [1455 1457]
--- FAIL: TestOpenAILoopbackPortsAre1455Then1457 (0.00s)
=== CONT  TestListenFirstAvailable_ErrorsWhenAllBusy
=== NAME  TestOAuthCallback_RejectsStateMismatch
    oauth_loopback_test.go:113: callback status = 200, want 400
--- FAIL: TestOAuthCallback_RejectsStateMismatch (0.00s)
=== NAME  TestListenFirstAvailable_ErrorsWhenAllBusy
    oauth_loopback_test.go:50: error = oauth: callback unavailable: listen tcp 127.0.0.1:62525: bind: address already in use, want device-code guidance
--- FAIL: TestListenFirstAvailable_ErrorsWhenAllBusy (0.00s)
=== NAME  TestListenFirstAvailable_UsesSecondPortWhenFirstBusy
    oauth_loopback_test.go:28: listenFirstAvailable() error = oauth: callback unavailable: listen tcp 127.0.0.1:62522: bind: address already in use
--- FAIL: TestListenFirstAvailable_UsesSecondPortWhenFirstBusy (0.00s)
=== CONT  TestGrokDevice_StopsAtExpiry
=== NAME  TestOpenAIDevice_UsesCodexProtocol
    oauth_device_test.go:172: LoginDeviceOAuth() error = oauth: OpenAI device-code request failed: 404 Not Found
--- FAIL: TestOpenAIDevice_UsesCodexProtocol (0.00s)
=== NAME  TestGrokDevice_SlowDownIncreasesInterval
    oauth_device_test.go:69: sleep sequence = [1s 1s], want [1s 6s]
=== NAME  TestGrokDevice_StopsAtExpiry
    oauth_device_test.go:105: token calls = 2, want at most 1 before expiry
--- FAIL: TestGrokDevice_StopsAtExpiry (0.00s)
--- FAIL: TestGrokDevice_SlowDownIncreasesInterval (0.00s)
=== NAME  TestLoginBrowserOAuth_GrokCompletesCallbackAndExchange
    oauth_loopback_test.go:240: redirect_uri = "http://localhost:62533/callback", want ephemeral 127.0.0.1 callback
--- FAIL: TestLoginBrowserOAuth_GrokCompletesCallbackAndExchange (0.01s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.697s
FAIL
```

The scratch copy was moved to Trash after the proof. The working tree was not
mutated. The first lint run found only Phase 5 implementation issues (explicit
best-effort/cleanup error handling, repeated form keys, and fallback control
flow); all were corrected within the approved Phase 5 source files. The final
independent phase gates were green. `git diff --check`, `gofmt -d`, per-file
`golint`, and `go vet ./...` exited zero with no output:

```text
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
0 issues.
ok  github.com/maccavelli/mcplib              3.059s
ok  github.com/maccavelli/mcplib/fastpath     0.678s
ok  github.com/maccavelli/mcplib/hfsc         1.256s
ok  github.com/maccavelli/mcplib/llmprovider  0.919s
ok  github.com/maccavelli/mcplib/logging      1.014s
ok  github.com/maccavelli/mcplib/schema       1.745s
ok  github.com/maccavelli/mcplib/selfupdate   2.411s
ok  github.com/maccavelli/mcplib/wizard       2.412s
ok  github.com/maccavelli/mcplib/llmprovider  1.682s
```

The final line is the race-enabled targeted Phase 5 run.

### Phase 6 — complete

The commit containing this entry makes OpenAI request authentication use a
`TokenSource` on every request while preserving the existing API-key
constructor. Static tokens use the Platform host; ChatGPT sessions use the
Codex backend, account and residency headers, and one refresh-driven retry
after HTTP 401. It also adds the ChatGPT catalog and makes ChatGPT model
listing a copied, HTTP-free result. `NewProviderWithSource` accepts OpenAI and
rejects other providers at this boundary; Grok support remains Phase 7. No
dependency was added and Phase 7 has not begun.

Following the phase's required sequencing, the first host-lock test was added
with the temporary Platform-only source constructor. It produced the intended
behavioral failure rather than only a compile failure:

```text
=== RUN   TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost
=== PAUSE TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost
=== CONT  TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost
    openai_chatgpt_test.go:39: request URL = https://api.openai.com/v1/responses, want chatgpt.com/backend-api/codex/responses
--- FAIL: TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.787s
FAIL
```

The remaining tests were then added before their APIs and catalog. Their
initial complete targeted result was red:

```text
# github.com/maccavelli/mcplib/llmprovider [github.com/maccavelli/mcplib/llmprovider.test]
llmprovider/discovery_test.go:109:17: undefined: ListAvailableModelsWithSource
llmprovider/discovery_test.go:118:32: undefined: StaticOpenAIChatGPT
llmprovider/discovery_test.go:119:44: undefined: StaticOpenAIChatGPT
llmprovider/discovery_test.go:122:16: undefined: ListAvailableModelsWithSource
llmprovider/models_catalog_test.go:215:19: undefined: StaticOpenAIChatGPT
llmprovider/models_catalog_test.go:216:49: undefined: StaticOpenAIChatGPT
llmprovider/openai_chatgpt_test.go:213:15: undefined: NewProviderWithSource
FAIL github.com/maccavelli/mcplib/llmprovider [build failed]
FAIL
```

Every new Phase 6 gate was proved after implementation in a scratch copy. The
verified mutations reversed static/ChatGPT hosts, suppressed account and
residency handling, disabled OAuth retry, admitted Claude to
`NewProviderWithSource`, forced ChatGPT listing through HTTP, and corrupted the
ChatGPT catalog. The complete combined result included these intended
failures. Opaque request-body function pointers in two `%v` request dumps are
shown as `<body>` and `<get-body>`; all behavior-bearing fields are preserved:

```text
=== RUN   TestListAvailableModelsWithSource_ChatGPTDoesNotHTTP
    discovery_test.go:100: ChatGPT model listing made an HTTP request
    discovery_test.go:119: models = [gpt-4.1-mini gpt-4.1-nano gpt-4o-mini gpt-4.1 gpt-4o o4-mini], want [gpt-broken gpt-5.4-mini gpt-5.3-codex]
--- FAIL: TestListAvailableModelsWithSource_ChatGPTDoesNotHTTP (0.00s)
=== RUN   TestStaticOpenAIChatGPTCatalog
    models_catalog_test.go:216: StaticOpenAIChatGPT = [gpt-broken gpt-5.4-mini gpt-5.3-codex], want [gpt-5.4 gpt-5.4-mini gpt-5.3-codex]
--- FAIL: TestStaticOpenAIChatGPTCatalog (0.00s)
=== RUN   TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost
=== PAUSE TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost
=== RUN   TestOpenAI_StaticKeyDoesNotHitChatGPTHost
=== PAUSE TestOpenAI_StaticKeyDoesNotHitChatGPTHost
=== RUN   TestOpenAI_ChatGPTSetsAccountHeader
=== PAUSE TestOpenAI_ChatGPTSetsAccountHeader
=== RUN   TestOpenAI_ChatGPTSetsResidencyHeader
=== PAUSE TestOpenAI_ChatGPTSetsResidencyHeader
=== RUN   TestOpenAI_OAuth401RetriesOnceAfterRefresh
=== PAUSE TestOpenAI_OAuth401RetriesOnceAfterRefresh
=== RUN   TestNewProviderWithSource_RejectsClaude
=== PAUSE TestNewProviderWithSource_RejectsClaude
=== RUN   TestNewProvider_APIKeyStillPlatform
=== PAUSE TestNewProvider_APIKeyStillPlatform
=== CONT  TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost
=== CONT  TestOpenAI_OAuth401RetriesOnceAfterRefresh
=== CONT  TestNewProviderWithSource_RejectsClaude
    openai_chatgpt_test.go:214: NewProviderWithSource() error = nil, want unsupported-provider error
--- FAIL: TestNewProviderWithSource_RejectsClaude (0.00s)
=== CONT  TestOpenAI_ChatGPTSetsAccountHeader
=== CONT  TestOpenAI_ChatGPTSetsResidencyHeader
=== CONT  TestNewProvider_APIKeyStillPlatform
=== RUN   TestOpenAI_ChatGPTSetsResidencyHeader/namespaced
=== CONT  TestOpenAI_StaticKeyDoesNotHitChatGPTHost
=== PAUSE TestOpenAI_ChatGPTSetsResidencyHeader/namespaced
=== RUN   TestOpenAI_ChatGPTSetsResidencyHeader/root_fallback
=== PAUSE TestOpenAI_ChatGPTSetsResidencyHeader/root_fallback
=== RUN   TestOpenAI_ChatGPTSetsResidencyHeader/namespaced_no_constraint_wins
=== PAUSE TestOpenAI_ChatGPTSetsResidencyHeader/namespaced_no_constraint_wins
=== CONT  TestOpenAI_ChatGPTSetsResidencyHeader/namespaced
=== CONT  TestOpenAI_ChatGPTSetsResidencyHeader/namespaced_no_constraint_wins
=== CONT  TestOpenAI_ChatGPTSetsResidencyHeader/root_fallback
=== NAME  TestOpenAI_OAuth401RetriesOnceAfterRefresh
    openai_chatgpt_test.go:203: Generate() error = Post "https://api.openai.com/v1/responses": unexpected request host
=== NAME  TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost
    openai_chatgpt_test.go:44: request URL = https://api.openai.com/v1/responses, want chatgpt.com/backend-api/codex/responses
--- FAIL: TestOpenAI_OAuth401RetriesOnceAfterRefresh (0.00s)
--- FAIL: TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost (0.00s)
=== NAME  TestOpenAI_ChatGPTSetsAccountHeader
    openai_chatgpt_test.go:91: ChatGPT-Account-Id = "", want acct_1
--- FAIL: TestOpenAI_ChatGPTSetsAccountHeader (0.00s)
=== NAME  TestNewProvider_APIKeyStillPlatform
    openai_chatgpt_test.go:234: request URL = &{POST https://chatgpt.com/backend-api/codex/responses HTTP/1.1 1 1 map[Authorization:[Bearer sk-test] Content-Type:[application/json]] {<body>} <get-body> 93 [] false chatgpt.com map[] map[] <nil> map[]   <nil> <nil> <nil>  {{}} <nil> [] map[]}, want api.openai.com/v1/responses
--- FAIL: TestNewProvider_APIKeyStillPlatform (0.00s)
=== NAME  TestOpenAI_ChatGPTSetsResidencyHeader/namespaced
    openai_chatgpt_test.go:145: residency header = "broken", want "eu"
=== NAME  TestOpenAI_ChatGPTSetsResidencyHeader/namespaced_no_constraint_wins
    openai_chatgpt_test.go:145: residency header = "broken", want ""
=== NAME  TestOpenAI_ChatGPTSetsResidencyHeader/root_fallback
    openai_chatgpt_test.go:145: residency header = "broken", want "us"
--- FAIL: TestOpenAI_ChatGPTSetsResidencyHeader (0.00s)
    --- FAIL: TestOpenAI_ChatGPTSetsResidencyHeader/namespaced (0.00s)
    --- FAIL: TestOpenAI_ChatGPTSetsResidencyHeader/namespaced_no_constraint_wins (0.00s)
    --- FAIL: TestOpenAI_ChatGPTSetsResidencyHeader/root_fallback (0.00s)
=== NAME  TestOpenAI_StaticKeyDoesNotHitChatGPTHost
    openai_chatgpt_test.go:64: request URL = &{POST https://chatgpt.com/backend-api/codex/responses HTTP/1.1 1 1 map[Authorization:[Bearer sk-test] Content-Type:[application/json]] {<body>} <get-body> 93 [] false chatgpt.com map[] map[] <nil> map[]   <nil> <nil> <nil>  {{}} <nil> [] map[]}, want api.openai.com/v1/responses
--- FAIL: TestOpenAI_StaticKeyDoesNotHitChatGPTHost (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.733s
FAIL
```

The combined mutation made the retry test fail on the intentionally reversed
host before it could exercise the retry assertion. The host mutation was
therefore restored in the same scratch copy while retry remained disabled; the
isolated complete result then failed on the first 401 as intended:

```text
=== RUN   TestOpenAI_OAuth401RetriesOnceAfterRefresh
=== PAUSE TestOpenAI_OAuth401RetriesOnceAfterRefresh
=== CONT  TestOpenAI_OAuth401RetriesOnceAfterRefresh
    openai_chatgpt_test.go:203: Generate() error = llm: authentication failed: openai HTTP 401
--- FAIL: TestOpenAI_OAuth401RetriesOnceAfterRefresh (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.635s
FAIL
```

The scratch copy was moved to Trash after the proof. The working tree was not
mutated. `git diff --check`, `go vet ./...`, repository lint, the full suite,
and the race-enabled targeted Phase 6 suite were green:

```text
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
0 issues.
ok  github.com/maccavelli/mcplib              1.452s
ok  github.com/maccavelli/mcplib/fastpath     1.342s
ok  github.com/maccavelli/mcplib/hfsc         0.668s
ok  github.com/maccavelli/mcplib/llmprovider  1.215s
ok  github.com/maccavelli/mcplib/logging      1.136s
ok  github.com/maccavelli/mcplib/schema       1.567s
ok  github.com/maccavelli/mcplib/selfupdate   2.909s
ok  github.com/maccavelli/mcplib/wizard       1.898s
ok  github.com/maccavelli/mcplib/llmprovider  1.312s
```

The final line is the race-enabled targeted Phase 6 run.

### Phase 7 — complete

The commit containing this entry changes Grok request authentication from a
stored API-key string to a `TokenSource`, while preserving `NewGrok`'s empty
static-key rejection. Both static keys and OAuth sessions use
`https://api.x.ai/v1`; every request acquires the current token and sends only
the standard bearer authorization header. OAuth sessions refresh and retry
once after HTTP 401, while static keys do not retry. Grok model discovery and
health probes retain the same source rather than extracting a credential.

Following the phase's required sequencing, the first host-lock test used the
temporary test-only CLI-proxy constructor. It produced the intended behavioral
failure:

```text
=== RUN   TestGrok_SessionUsesAPIXAIHost
=== PAUSE TestGrok_SessionUsesAPIXAIHost
=== CONT  TestGrok_SessionUsesAPIXAIHost
    grok_oauth_test.go:38: request URL = https://cli-chat-proxy.grok.com/v1/responses, want api.x.ai/v1/responses
--- FAIL: TestGrok_SessionUsesAPIXAIHost (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.744s
FAIL
```

The complete initial targeted run, after adding the remaining tests but before
adding Grok construction from a source, was red. The legacy empty-key behavior
already passed; the later scratch mutation separately proves the new
`StaticToken` branch of that combined gate.

```text
=== RUN   TestGrok_SessionUsesAPIXAIHost
=== PAUSE TestGrok_SessionUsesAPIXAIHost
=== RUN   TestGrok_SessionOmitsCLITokenAuthHeader
=== PAUSE TestGrok_SessionOmitsCLITokenAuthHeader
=== RUN   TestGrok_EmptyStaticKeyStillRejected
=== PAUSE TestGrok_EmptyStaticKeyStillRejected
=== RUN   TestGrok_TokenSourceCalledPerRequest
=== PAUSE TestGrok_TokenSourceCalledPerRequest
=== RUN   TestGrok_OAuth401RetriesOnceAfterRefresh
=== PAUSE TestGrok_OAuth401RetriesOnceAfterRefresh
=== CONT  TestGrok_SessionUsesAPIXAIHost
=== CONT  TestGrok_TokenSourceCalledPerRequest
=== NAME  TestGrok_SessionUsesAPIXAIHost
    grok_oauth_test.go:34: NewProviderWithSource() error = provider "grok" does not accept TokenSource
--- FAIL: TestGrok_SessionUsesAPIXAIHost (0.00s)
=== CONT  TestGrok_EmptyStaticKeyStillRejected
--- PASS: TestGrok_EmptyStaticKeyStillRejected (0.00s)
=== CONT  TestGrok_SessionOmitsCLITokenAuthHeader
    grok_oauth_test.go:68: NewProviderWithSource() error = provider "grok" does not accept TokenSource
--- FAIL: TestGrok_SessionOmitsCLITokenAuthHeader (0.00s)
=== NAME  TestGrok_TokenSourceCalledPerRequest
    grok_oauth_test.go:97: NewProviderWithSource() error = provider "grok" does not accept TokenSource
--- FAIL: TestGrok_TokenSourceCalledPerRequest (0.00s)
=== CONT  TestGrok_OAuth401RetriesOnceAfterRefresh
    grok_oauth_test.go:159: NewProviderWithSource() error = provider "grok" does not accept TokenSource
--- FAIL: TestGrok_OAuth401RetriesOnceAfterRefresh (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.668s
FAIL
```

The first post-implementation build caught a missing `time` import in the
planned `grok.go` change. It was corrected before any gate was treated as
green:

```text
# github.com/maccavelli/mcplib/llmprovider [github.com/maccavelli/mcplib/llmprovider.test]
llmprovider/grok.go:280:19: undefined: time
FAIL github.com/maccavelli/mcplib/llmprovider [build failed]
FAIL
```

Every new Phase 7 gate was then proved against deliberate breakage in scratch
copies. The verified combined mutations selected the forbidden CLI proxy,
added `X-XAI-Token-Auth`, admitted an empty `StaticToken`, and replaced each
acquired credential with the first token. The first attempted mutation did not
compile because it left `token` unused, so it established nothing; the scratch
mutation was corrected and inspected before this complete behavioral result:

```text
=== RUN   TestGrok_SessionUsesAPIXAIHost
=== PAUSE TestGrok_SessionUsesAPIXAIHost
=== RUN   TestGrok_SessionOmitsCLITokenAuthHeader
=== PAUSE TestGrok_SessionOmitsCLITokenAuthHeader
=== RUN   TestGrok_EmptyStaticKeyStillRejected
=== PAUSE TestGrok_EmptyStaticKeyStillRejected
=== RUN   TestGrok_TokenSourceCalledPerRequest
=== PAUSE TestGrok_TokenSourceCalledPerRequest
=== RUN   TestGrok_OAuth401RetriesOnceAfterRefresh
=== PAUSE TestGrok_OAuth401RetriesOnceAfterRefresh
=== CONT  TestGrok_SessionUsesAPIXAIHost
=== CONT  TestGrok_TokenSourceCalledPerRequest
=== CONT  TestGrok_EmptyStaticKeyStillRejected
=== CONT  TestGrok_SessionOmitsCLITokenAuthHeader
=== CONT  TestGrok_OAuth401RetriesOnceAfterRefresh
=== NAME  TestGrok_EmptyStaticKeyStillRejected
    grok_oauth_test.go:82: NewProviderWithSource() error = nil, want empty-static-key error
--- FAIL: TestGrok_EmptyStaticKeyStillRejected (0.00s)
=== NAME  TestGrok_OAuth401RetriesOnceAfterRefresh
    grok_oauth_test.go:162: Generate() error = Post "https://cli-chat-proxy.grok.com/v1/responses": unexpected request host
=== NAME  TestGrok_SessionUsesAPIXAIHost
    grok_oauth_test.go:44: request URL = https://cli-chat-proxy.grok.com/v1/responses, want api.x.ai/v1/responses
--- FAIL: TestGrok_OAuth401RetriesOnceAfterRefresh (0.00s)
--- FAIL: TestGrok_SessionUsesAPIXAIHost (0.00s)
=== NAME  TestGrok_TokenSourceCalledPerRequest
    grok_oauth_test.go:106: authorizations/calls = [Bearer first-token Bearer first-token]/2, want [Bearer first-token Bearer second-token]/2
--- FAIL: TestGrok_TokenSourceCalledPerRequest (0.00s)
=== NAME  TestGrok_SessionOmitsCLITokenAuthHeader
    grok_oauth_test.go:54: request unexpectedly contains X-Xai-Token-Auth
--- FAIL: TestGrok_SessionOmitsCLITokenAuthHeader (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.598s
FAIL
```

Because the broken host masked the OAuth retry assertion, the host and token
replacement were restored in the same scratch copy while retry remained
disabled. The isolated test then failed on the unretried 401 as intended:

```text
=== RUN   TestGrok_OAuth401RetriesOnceAfterRefresh
=== PAUSE TestGrok_OAuth401RetriesOnceAfterRefresh
=== CONT  TestGrok_OAuth401RetriesOnceAfterRefresh
    grok_oauth_test.go:162: Generate() error = llm: authentication failed: grok HTTP 401
--- FAIL: TestGrok_OAuth401RetriesOnceAfterRefresh (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.661s
FAIL
```

The locked static-key no-retry behavior has its own gate. A second inspected
scratch mutation incorrectly allowed static credentials to enter the retry
path; the gate observed both requests:

```text
=== RUN   TestGrok_Static401DoesNotRetry
=== PAUSE TestGrok_Static401DoesNotRetry
=== CONT  TestGrok_Static401DoesNotRetry
    grok_oauth_test.go:185: generate calls = 2, want 1
--- FAIL: TestGrok_Static401DoesNotRetry (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/llmprovider 0.592s
FAIL
```

Both scratch copies were moved to Trash after the proofs. The working tree was
not mutated by any negative test.

The final staged-file `gofmt -d`, per-file `golint`, `go vet ./...`, and
`git diff --cached --check` gates exited zero with no output. Repository lint,
the full suite, and the race-enabled targeted Phase 7 suite were green:

```text
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
0 issues.
ok  github.com/maccavelli/mcplib              (cached)
ok  github.com/maccavelli/mcplib/fastpath     (cached)
ok  github.com/maccavelli/mcplib/hfsc         (cached)
ok  github.com/maccavelli/mcplib/llmprovider  0.707s
ok  github.com/maccavelli/mcplib/logging      (cached)
ok  github.com/maccavelli/mcplib/schema       (cached)
ok  github.com/maccavelli/mcplib/selfupdate   (cached)
ok  github.com/maccavelli/mcplib/wizard       (cached)
ok  github.com/maccavelli/mcplib/llmprovider  1.665s
```

The final line is the race-enabled targeted Phase 7 run.

### Phase 8 — complete

The commit containing this entry extends the renderer-agnostic wizard with
credential kinds, provider auth-method selection, the orchestrated-process
guard, browser and device OAuth wiring, token-stdin classification, dynamic
token-source discovery, and masked vendor-session import. OAuth results keep
`APIKey` empty and expose the session fields needed by consumers. Browser,
device, imported, and—following the approved deviation—OpenAI token-stdin
OAuth sessions are saved through the supplied `TokenStore`. Providers without
auth methods retain their existing prompt scripts, and Ollama returns
`CredNone`. No dependency was added.

The complete Phase 8 test set was added before the new result/options fields or
auth helpers. Its initial targeted run was compile-red:

```text
# github.com/maccavelli/mcplib/wizard [github.com/maccavelli/mcplib/wizard.test]
wizard/auth_test.go:16:58: unknown field Orchestrated in struct literal of type Options
wizard/auth_test.go:17:21: undefined: ErrOrchestrated
wizard/auth_test.go:35:4: unknown field Kind in struct literal of type Result
wizard/auth_test.go:35:18: undefined: CredOAuth
wizard/auth_test.go:36:4: unknown field AccessToken in struct literal of type Result
wizard/auth_test.go:37:4: unknown field RefreshToken in struct literal of type Result
wizard/auth_test.go:38:4: unknown field TokenExpiry in struct literal of type Result
wizard/auth_test.go:39:4: unknown field Issuer in struct literal of type Result
wizard/auth_test.go:40:4: unknown field ClientID in struct literal of type Result
wizard/auth_test.go:41:4: unknown field AccountID in struct literal of type Result
wizard/auth_test.go:41:4: too many errors
FAIL github.com/maccavelli/mcplib/wizard [build failed]
FAIL
```

After implementation, every new behavioral gate was proved against inspected
mutations in a scratch copy. The combined mutations bypassed explicit
orchestration, copied OAuth access into `APIKey`, mislabeled static credentials,
added an auth menu to Claude, changed the missing-store contract, broke all
token-stdin classifications, ignored `CODEX_ACCESS_TOKEN`, skipped OAuth saves,
selected Grok's API-key scope, and copied OpenAI's platform key. The complete
targeted result was:

```text
=== RUN   TestConfigureLLM_OrchestratedReturnsErr
    auth_test.go:18: ConfigureLLM() error = select provider: fakePrompter: unexpected Select("Choose an LLM provider:"), want ErrOrchestrated
--- FAIL: TestConfigureLLM_OrchestratedReturnsErr (0.00s)
=== RUN   TestConfigureLLM_ChatGPTResultDoesNotPopulateAPIKey
    auth_test.go:50: APIKey/Kind = "existing-access-abcd"/"oauth", want empty/"oauth"
--- FAIL: TestConfigureLLM_ChatGPTResultDoesNotPopulateAPIKey (0.00s)
=== RUN   TestConfigureLLM_APIKeyKindUnchanged
=== RUN   TestConfigureLLM_APIKeyKindUnchanged/gemini
    auth_test.go:78: Kind/APIKey = ""/"sk-super-secret-key-1234", want "api_key"/key
=== RUN   TestConfigureLLM_APIKeyKindUnchanged/openai
    auth_test.go:78: Kind/APIKey = ""/"sk-super-secret-key-1234", want "api_key"/key
--- FAIL: TestConfigureLLM_APIKeyKindUnchanged (0.00s)
    --- FAIL: TestConfigureLLM_APIKeyKindUnchanged/gemini (0.00s)
    --- FAIL: TestConfigureLLM_APIKeyKindUnchanged/openai (0.00s)
=== RUN   TestConfigureLLM_DoesNotOfferClaudeOAuth
    auth_test.go:91: ConfigureLLM() error = select model: fakePrompter: unexpected Select("Choose a Claude (Anthropic) model:")
--- FAIL: TestConfigureLLM_DoesNotOfferClaudeOAuth (0.00s)
=== RUN   TestConfigureLLM_OAuthMethodsRequireTokenStore
    auth_test.go:111: auth index 1 error = wizard: broken store error
--- FAIL: TestConfigureLLM_OAuthMethodsRequireTokenStore (0.00s)
=== RUN   TestConfigureLLM_TokenStdinClassification
=== RUN   TestConfigureLLM_TokenStdinClassification/openai_platform_key
    auth_test.go:159: result credential = "oauth"/"sk-platform"/"sk-platform"
=== RUN   TestConfigureLLM_TokenStdinClassification/openai_access_token
    auth_test.go:159: result credential = "api_key"/"chatgpt-access"/""
=== RUN   TestConfigureLLM_TokenStdinClassification/grok_always_api_key
    auth_test.go:159: result credential = "oauth"/"xai-key"/"xai-key"
--- FAIL: TestConfigureLLM_TokenStdinClassification (0.00s)
    --- FAIL: TestConfigureLLM_TokenStdinClassification/openai_platform_key (0.00s)
    --- FAIL: TestConfigureLLM_TokenStdinClassification/openai_access_token (0.00s)
    --- FAIL: TestConfigureLLM_TokenStdinClassification/grok_always_api_key (0.00s)
=== RUN   TestConfigureLLM_TokenStdinUsesCodexEnvironment
    auth_test.go:181: ConfigureLLM() error = enter credential: fakePrompter: unexpected Secret("Paste your OpenAI credential")
--- FAIL: TestConfigureLLM_TokenStdinUsesCodexEnvironment (0.00s)
=== RUN   TestConfigureLLM_BrowserAndDevicePersistSessions
=== RUN   TestConfigureLLM_BrowserAndDevicePersistSessions/browser
    auth_test.go:229: Kind/saves/session = "oauth"/0/<nil>
=== RUN   TestConfigureLLM_BrowserAndDevicePersistSessions/device
    auth_test.go:229: Kind/saves/session = "oauth"/0/<nil>
--- FAIL: TestConfigureLLM_BrowserAndDevicePersistSessions (0.00s)
    --- FAIL: TestConfigureLLM_BrowserAndDevicePersistSessions/browser (0.00s)
    --- FAIL: TestConfigureLLM_BrowserAndDevicePersistSessions/device (0.00s)
=== RUN   TestConfigureLLM_LocalProviderSkipsKey
--- PASS: TestConfigureLLM_LocalProviderSkipsKey (0.01s)
=== RUN   TestImportGrok_SkipsAPIKeyScope
    import_test.go:42: imported access/refresh = "xai-MUST-SKIP"/""
--- FAIL: TestImportGrok_SkipsAPIKeyScope (0.00s)
=== RUN   TestImportOpenAI_IgnoresPlatformKeyInAuthJSON
    import_test.go:75: imported access/refresh = "sk-MUST-IGNORE"/"rt-chatgpt"
--- FAIL: TestImportOpenAI_IgnoresPlatformKeyInAuthJSON (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/wizard 0.554s
FAIL
```

The combined mutation correctly left the local-provider kind intact. Changing
only the no-credential result to `CredAPIKey` then proved that adjusted legacy
gate independently:

```text
=== RUN   TestConfigureLLM_LocalProviderSkipsKey
    configure_test.go:116: Kind = "api_key", want CredNone for a local provider
--- FAIL: TestConfigureLLM_LocalProviderSkipsKey (0.01s)
FAIL
FAIL github.com/maccavelli/mcplib/wizard 0.636s
FAIL
```

Execution then exposed the documented plan contradiction: OpenAI token-stdin
could create `CredOAuth` without saving it, while Phase 10 clears `APIKey` and
requires the wizard to have saved every OAuth result. The maintainer approved
requiring a store and saving only when token-stdin classifies the value as
OpenAI OAuth. Tests added before that correction produced:

```text
=== RUN   TestConfigureLLM_TokenStdinClassification
=== RUN   TestConfigureLLM_TokenStdinClassification/openai_platform_key
=== RUN   TestConfigureLLM_TokenStdinClassification/openai_access_token
    auth_test.go:165: TokenStore saves = 0, want 1
=== RUN   TestConfigureLLM_TokenStdinClassification/grok_always_api_key
--- FAIL: TestConfigureLLM_TokenStdinClassification (0.00s)
    --- PASS: TestConfigureLLM_TokenStdinClassification/openai_platform_key (0.00s)
    --- FAIL: TestConfigureLLM_TokenStdinClassification/openai_access_token (0.00s)
    --- PASS: TestConfigureLLM_TokenStdinClassification/grok_always_api_key (0.00s)
=== RUN   TestConfigureLLM_TokenStdinOAuthRequiresTokenStore
    auth_test.go:179: ConfigureLLM() error = select model: fakePrompter: unexpected Select("Choose a OpenAI model:")
--- FAIL: TestConfigureLLM_TokenStdinOAuthRequiresTokenStore (0.00s)
=== RUN   TestConfigureLLM_TokenStdinUsesCodexEnvironment
    auth_test.go:204: result credential = "oauth"/"codex-access-abcd"; Secret calls = 0; TokenStore saves = 0
--- FAIL: TestConfigureLLM_TokenStdinUsesCodexEnvironment (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/wizard 0.744s
FAIL
```

After the correction passed, a fresh inspected scratch mutation returned the
access-only session without checking or saving its store. The same complete
targeted result failed again on the intended assertions:

```text
=== RUN   TestConfigureLLM_TokenStdinClassification
=== RUN   TestConfigureLLM_TokenStdinClassification/openai_platform_key
=== RUN   TestConfigureLLM_TokenStdinClassification/openai_access_token
    auth_test.go:165: TokenStore saves = 0, want 1
=== RUN   TestConfigureLLM_TokenStdinClassification/grok_always_api_key
--- FAIL: TestConfigureLLM_TokenStdinClassification (0.00s)
    --- PASS: TestConfigureLLM_TokenStdinClassification/openai_platform_key (0.00s)
    --- FAIL: TestConfigureLLM_TokenStdinClassification/openai_access_token (0.00s)
    --- PASS: TestConfigureLLM_TokenStdinClassification/grok_always_api_key (0.00s)
=== RUN   TestConfigureLLM_TokenStdinOAuthRequiresTokenStore
    auth_test.go:179: ConfigureLLM() error = select model: fakePrompter: unexpected Select("Choose a OpenAI model:")
--- FAIL: TestConfigureLLM_TokenStdinOAuthRequiresTokenStore (0.00s)
=== RUN   TestConfigureLLM_TokenStdinUsesCodexEnvironment
    auth_test.go:204: result credential = "oauth"/"codex-access-abcd"; Secret calls = 0; TokenStore saves = 0
--- FAIL: TestConfigureLLM_TokenStdinUsesCodexEnvironment (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/wizard 0.609s
FAIL
```

The masked-confirmation and persistence behavior of the wizard import branch
was also proved in a fresh scratch copy by displaying the raw access token and
skipping the save:

```text
=== RUN   TestConfigureLLM_ImportConfirmsAndPersistsSession
    import_test.go:79: TokenStore saves/session = 0/<nil>
    import_test.go:81: displayed text contains raw credential "sess-grok"
--- FAIL: TestConfigureLLM_ImportConfirmsAndPersistsSession (0.00s)
FAIL
FAIL github.com/maccavelli/mcplib/wizard 0.644s
FAIL
```

The first repository lint run rejected the original environment-derived full
file read:

```text
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
wizard/import.go:39:15: G304: Potential file inclusion via variable (gosec)
    data, err := os.ReadFile(path)
                ^
1 issues:
* gosec: 1
make: *** [lint] Error 1
```

The import boundary now opens the selected vendor directory with `os.OpenRoot`
and reads only its fixed `auth.json` child. This resolves the path-escape risk
without suppressing lint. All scratch copies were moved to Trash after their
proofs, and the working tree was not mutated by the negative tests.

The final staged-file `gofmt -d`, per-file `golint`, `go vet ./...`, and
`git diff --cached --check` gates exited zero with no output. Repository lint,
the full suite, and the race-enabled wizard suite were green:

```text
/Users/<user>/go/bin/golangci-lint run -c .golangci.yml ./...
0 issues.
ok  github.com/maccavelli/mcplib              (cached)
ok  github.com/maccavelli/mcplib/fastpath     (cached)
ok  github.com/maccavelli/mcplib/hfsc         (cached)
ok  github.com/maccavelli/mcplib/llmprovider  (cached)
ok  github.com/maccavelli/mcplib/logging      (cached)
ok  github.com/maccavelli/mcplib/schema       (cached)
ok  github.com/maccavelli/mcplib/selfupdate   (cached)
ok  github.com/maccavelli/mcplib/wizard       0.500s
ok  github.com/maccavelli/mcplib/wizard       1.812s
```

The final line is the race-enabled Phase 8 wizard run.

### Phase 9 — complete

The commit containing this entry updates the README's LLM-provider guidance
for the APIs and behavior delivered by Phases 1–8. It keeps `NewProvider` as
the API-key factory, documents `NewProviderWithSource` for OpenAI and Grok,
locks the ChatGPT and Grok inference hosts, excludes the Grok CLI proxy,
explains the wizard's OAuth result shape and token-store boundary, directs
orchestrated processes to the backplane, and keeps Claude and Gemini
API-key-only. The pre-existing retry paragraph now distinguishes the single
OAuth refresh retry from static-key and other 4xx behavior. No dependency or
Go source file changed, and Phase 10 was not started.

Before the README edit, a fixed-string content audit covering every Phase 9
requirement was observed red:

```text
missing: NewProviderWithSource
missing: chatgpt.com/backend-api/codex
missing: api.openai.com
missing: api.x.ai/v1
missing: cli-chat-proxy
missing: Kind=oauth
missing: empty APIKey
missing: Claude and Gemini remain API-key-only
exit status 1
```

The same audit was rerun without weakening its patterns after the edit. Its
first post-edit result still reported `Claude and Gemini remain API-key-only`
because a Markdown source line break split the literal phrase. The README
formatting was corrected, and the unchanged audit then exited zero with no
output. `git diff --check` also exited zero with no output, and
`git diff --quiet -- go.mod go.sum` confirmed that no dependency file changed.
Go formatting, lint, vet, and tests were not rerun because this phase stages
only Markdown documentation and the approved phase explicitly marks `gofmt`
as not applicable.

### Phase 10 — complete

Consumer commit `3dafb46` implements subscription authentication in
`prepare-commit-msg`. The config schema persists only `auth_kind: "oauth"`;
API-key and unknown values normalize to the legacy empty representation, and
OAuth access and refresh tokens remain exclusively in the sibling `oauth/`
`FileTokenStore`. Interactive configure imports or creates OpenAI/Grok
sessions, deletes stale sessions when API-key auth is selected, and explicitly
marks this standalone hook as non-orchestrated. Non-interactive `--yes` remains
API-key-only. Generation constructs OAuth providers with a refreshable
`OAuthSession` through `NewProviderWithSource`, never by passing an access token
to `NewProvider`.

The initial test-first run was compile-red before the consumer interfaces
existed. Representative failures were:

```text
internal/config/config_test.go: unknown field AuthKind in ProviderConfig
internal/config/config_test.go: undefined: ValidateOAuth
internal/ui/setup_test.go: undefined: config.OAuthDir
main_oauth_test.go: undefined: config.NewOAuthStore
main_oauth_test.go: undefined: newProviderWithSource
FAIL github.com/maccavelli/prepare-commit-msg [build failed]
```

Every new or materially extended Phase 10 gate was then proved against an
inspected mutation in isolated copies; no negative test dirtied either real
working tree. The mutations produced the intended failures:

* adding `access_token` / `refresh_token` fields made
  `TestSave_OAuthKindDoesNotWriteTokens` print the leaked keys;
* removing Grok from `SupportedProviders` made
  `TestApplyDefaults_IncludesGrok` report the missing slot;
* making `IsOAuth` case-sensitive rejected `AuthKind="OAUTH"`; forcing
  `ValidateOAuth` to always fail or always pass separately broke the existing-
  session and missing-session gates;
* preserving `auth_kind: "api_key"` made
  `TestSave_NonOAuthAuthKindIsOmitted` print the incorrectly serialized value;
* skipping stale-session deletion left `oauth/openai.json` present after the
  API-key setup script;
* copying imported access tokens into `APIKey` broke both Grok and ChatGPT
  import tests, while explicit `access` / `refresh` config fields were caught
  independently by the serialized-config assertion;
* deleting the imported session made the Grok token-file existence check fail,
  and changing the copied token store to mode `0644` made its Unix permission
  assertion fail with `want 0600`;
* adding an authentication menu to Gemini and Claude broke both locked legacy
  interactive scripts;
* leaving OAuth on the non-interactive path made its new assertion report
  `auth kind: "oauth"`;
* routing OAuth generation through `NewProvider` with `session.Access` invoked
  the test's fatal static-provider seam;
* swallowing the browser command's non-zero exit broke the browser error-path
  test; and
* before URL validation existed, the unsafe-URL test reached the browser
  command and failed because it received an `open browser` error instead of the
  required `invalid browser URL` rejection.

The first coverage run was `79.5%` against the repository's `80.0%` floor.
The plan-authorized `open_test.go` command-result coverage raised the final
total to `80.7%`; the browser error assertion was also mutation-proved.

The first exact staged `go-precheck.sh` invocation exposed the approved
2026-09-13 deviation: its flat staged snapshot could not resolve the local
module replacement.

```text
github.com/maccavelli/mcplib@v1.4.1:
replacement directory ../mcplib does not exist
```

After the documented topology correction, the gate reached lint and exposed
the approved 2026-09-14 browser-launch deviation:

```text
internal/ui/open.go: G204: Subprocess launched with variable
3 issues:
* gosec: 3
```

The corrected launcher validates absolute HTTP(S) URLs, uses direct non-shell
commands on macOS/Linux, and uses
`rundll32.exe url.dll,FileProtocolHandler` on Windows. Narrow G204 annotations
apply only after that validation.

The final independently checked staged gate was fully green:

```text
bash=0 gofmt=0 vet=0 add=0 precheck=0 race=0 coverage=0 gate=0
total coverage: 80.7% (minimum 80.0%)
```

`git diff --cached --check` and the staged internal-identifier scan also
exited zero. The commit was created only in `prepare-commit-msg`, as required;
neither repository was pushed. This execution-record update remains an
uncommitted `mcplib` documentation change because Phase 10 explicitly says not
to commit `mcplib`.
