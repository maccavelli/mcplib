---
status: proposed
date: 2026-09-12
associated-madr: "0008-MADR-subscription-auth-for-llm-providers.md"
decision-makers: mcplib maintainers
---

# Implement Browser, API-Key, and Headless Subscription Authentication for OpenAI and xAI Grok

Associated MADR: [0008-MADR-subscription-auth-for-llm-providers.md](0008-MADR-subscription-auth-for-llm-providers.md)
(accepted, revision 4, 2026-09-12).

This plan executes that MADR and nothing else. If execution discovers a fact that
contradicts the MADR, **stop and amend the MADR** before continuing. Do not
smuggle a different architecture into a phase.

> **Plan revision 2026-09-12.** Maintainer named **prepare-commit-msg** as the
> first consumer. Phases 1–9 stay in `mcplib`. Phase 10 is that hook only
> (sibling repo `../prepare-commit-msg`). MagicDev and MagicTools are still
> out of this plan.

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
    Provider  string
    Access    string
    Refresh   string
    Expiry    time.Time
    Issuer    string
    ClientID  string
    AccountID string // ChatGPT; empty for Grok
    TokenURL  string
    Store     TokenStore // optional; required to persist rotation
    mu        sync.Mutex
    inflight  *tokenFuture // unexported single-flight
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
* `WithBaseURL` still overrides the host in tests; ChatGPT-mode tests that
  pass a Platform host must fail (see Phase 6 negative test).

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

When orchestrated (nil pointer resolves true): `ConfigureLLM` returns
`ErrOrchestrated` without prompting. Callers that want a standalone wizard
inside tests set `Orchestrated` to `boolPtr(false)`.

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
| 2 | `TokenStore` + `FileTokenStore` `0600` | `tokenstore.go`, `tokenstore_file.go`, `tokenstore_file_unix.go`, `tokenstore_file_windows.go`, `tokenstore_test.go` | — |
| 3 | `OAuthSession` refresh + persist-before-use | `oauth_session.go`, `oauth_session_test.go` | — |
| 4 | `AuthMethod` on descriptors | — | `descriptor.go`, `descriptor_test.go` |
| 5 | PKCE, OpenAI loopback 1455/1457, Grok ephemeral loopback, device-code | `oauth_pkce.go`, `oauth_loopback.go`, `oauth_device.go`, `*_test.go` | `constants.go` (OAuth string constants) |
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

   `Load` of a missing file returns `(nil, nil)`, not an error.
   `Save` writes `{dir}/{provider}.json` via temp file + rename, then sets
   mode `0600` on Unix (`tokenstore_file_unix.go`). Windows: create with
   `os.OpenFile` and `syscall.O_CREAT` user-only as far as the stdlib allows;
   skip the `0600` assert in tests tagged `unix`.
   `NewFileTokenStore` `MkdirAll(dir, 0700)`.
   Reject `provider` containing `/`, `\`, or `..`.

2. **Red tests, in this order:**

   * `TestFileTokenStore_SaveMode0600` (unix): `stat.Mode().Perm() == 0600`.
     Implement `Save` **without** chmod first, run, **watch it fail**, then
     add chmod. This is the MADR confirmation "mode 0600".
   * `TestFileTokenStore_RoundTrip`
   * `TestFileTokenStore_LoadMissingIsNil`
   * `TestFileTokenStore_RejectsPathTraversal`

3. Phase green. Commit.

`Save` must not log `access` or `refresh`. A test
`TestFileTokenStore_DoesNotLogSecrets` is not required if the store never
calls `slog`; assert the file package imports no `log/slog`.

## Phase 3 — `OAuthSession` refresh

### Behaviour

* If `time.Until(Expiry) > 2*time.Minute` (and Access non-empty), return Access.
* Else POST `application/x-www-form-urlencoded` to `TokenURL`
  `grant_type=refresh_token&refresh_token=...&client_id=...`.
* On HTTP 200, parse `access_token`, optional `refresh_token` (keep old if
  absent), `expires_in` (default 3600s).
* If `Store != nil`, `Save` the **new** session. If `Save` fails, return that
  error and **do not** adopt the new access token in memory.
* Single-flight: concurrent `Token()` share one refresh.
* `ChatGPT()` is `strings.TrimRight(Issuer, "/") == DefaultOpenAIIssuer`.

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

### Red tests

* `TestDescriptors_OpenAIAndGrokOfferOAuth` — FAIL until methods exist.
* `TestDescriptors_NoOAuthOnOtherProviders` — range `Descriptors()`, if
  `ID` not in `{openai,grok}` then no method id is `browser_oauth` or
  `device_code`. Write this test **first** against current code; it should
  **PASS already** (empty methods). Keep it. Then add openai/grok methods
  and confirm it still passes.
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

* Bind `127.0.0.1:1455`. If `EADDRINUSE`, bind `127.0.0.1:1457`. If both fail,
  return an error that tells the caller to use device-code. **Do not** pick
  another port.
* Advertise redirect `http://localhost:{port}/auth/callback` (host
  `localhost`, not `127.0.0.1`, matching Codex `server.rs`).
* Path `/auth/callback`: read `code`, `state`, `error`. State must match.
  Respond 200 text/html "You can close this window." (no secrets in HTML).
* Timeout 10 minutes.
* `OpenURL` is invoked with the authorize URL; a nil/erroring `OpenURL` does
  not fail the flow (MADR: print/notify the URL).
* Tests use `httptest` for the **token** exchange and a real loopback bind on
  1455 in `TestOpenAILoopback_Binds1455Then1457`: occupy 1455 with a dummy
  listener, assert the flow binds 1457. If the implementation binds a random
  port, this test fails.

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
* `TestOpenAILoopback_DoesNotUseRandomPort` — as above.

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

1. `TestOpenAI_ChatGPTSessionDoesNotHitPlatformHost`: `OAuthSession` with
   `Issuer: DefaultOpenAIIssuer`, `WithHTTPClient` capturing URL. Generate
   against httptest. Assert `req.URL.Host != "api.openai.com"` and path
   contains `/backend-api/codex/responses` when base URL is default.
   **Write this test first against current `OpenAIProvider` (which always
   uses `p.baseURL + "/responses"` with the Platform default). It MUST
   FAIL.** Then implement the branch.
2. `TestOpenAI_StaticKeyDoesNotHitChatGPTHost`: static key, default base,
   assert host `api.openai.com` and path `/v1/responses`.
3. `TestOpenAI_ChatGPTSetsAccountHeader`.
4. `TestNewProviderWithSource_RejectsClaude`: `NewProviderWithSource("claude",
   src, "x")` errors.
5. `TestNewProvider_APIKeyStillPlatform`: regression
   `NewProvider("openai", "sk-test", "gpt-4.1-mini")` hits Platform.

Phase green. Commit.

## Phase 7 — Grok `TokenSource`

### Construction

`NewGrok(apiKey, model, opts...)` unchanged empty-key error.
Internal field `apiKey string` becomes `src TokenSource`.
`doGenerateItems` (or the existing request helper) calls `src.Token(ctx)`
and sets `Authorization: Bearer`. Assert the request header map has **no**
`X-XAI-Token-Auth` (any case).

Default host remains `https://api.x.ai/v1`. Session does not change it.
A test that sets session + default host and spies the URL must equal
`https://api.x.ai/v1/responses` (or the `WithBaseURL` test server).

`NewProviderWithSource("grok", src, model, opts...)` works for both
`*StaticToken` and `*OAuthSession`.

### Red tests

* `TestGrok_SessionUsesAPIXAIHost`: FAIL if URL contains `cli-chat-proxy`.
  Write against a stub that "helpfully" uses the proxy — or write the
  assertion first on a temporary wrong `baseURL` in the test only (do not
  commit a wrong default). The committed implementation's default must
  be `api.x.ai`; the test locks it.
* `TestGrok_SessionOmitsCLITokenAuthHeader`: range `req.Header`; fail if
  any key equals `X-XAI-Token-Auth` ignoring case.
* `TestGrok_EmptyStaticKeyStillRejected`.

Phase green. Commit.

## Phase 8 — Wizard

### Flow (after provider select, before model select)

1. If orchestrated → return `ErrOrchestrated`.
2. `Select` among `d.AuthMethods` (if empty, today's `resolveAPIKey` only —
   all non-openai/grok providers).
3. Switch on `AuthMethodID`:

   * `api_key`: existing `resolveAPIKey`. `Kind=CredAPIKey`. Env vars
     unchanged (`OPENAI_API_KEY`, `XAI_API_KEY`). Also honour
     `CODEX_ACCESS_TOKEN` for openai `token_stdin` only, not here.
   * `browser_oauth`: require `TokenStore`. Run Phase 5 loopback. `Save`.
     Fill OAuth fields. `APIKey=""`. `Kind=CredOAuth`.
   * `device_code`: require `TokenStore`. Run Phase 5 device. `Notify` the
     verification URL and user code. `Save`. OAuth fields.
   * `token_stdin`: `Secret`. If the value looks like `sk-` / `xai-`, treat
     as API key. If openai and env `CODEX_ACCESS_TOKEN` / pasted JWT-like,
     treat as OAuth access-only (no refresh; `Expiry` zero; ChatGPT mode
     if openai). Grok paste is API key only (probe: session is JWT but we
     do not offer "paste access token" as a separate Grok method beyond
     import).
   * `import_vendor_cli`: see below.

4. Discover models: `ListAvailableModelsWithSource` using a `StaticToken` or
   `OAuthSession` matching `Kind`. ChatGPT therefore sees
   `StaticOpenAIChatGPT`.
5. Model + fallbacks unchanged.

### Import

**openai:** read `$CODEX_HOME/auth.json` default `~/.codex/auth.json`.
Require `tokens.access_token` and `tokens.refresh_token`. Account id from
`tokens.account_id` or id-token claims (`chatgpt_account_id`). Ignore
`OPENAI_API_KEY` in that file (that is Platform, not ChatGPT). Confirm with
`MaskSecret(access)`.

**grok:** read `$GROK_HOME/auth.json` default `~/.grok/auth.json`. The file
is a map of scope → object. Pick the first entry with
`auth_mode` in `{oidc,oauth2}` (JSON may say `"oidc"`) and
`oidc_issuer` equal to `DefaultGrokOAuthIssuer` (or any issuer containing
`auth.x.ai`). Use `key` as access, `refresh_token`, `expires_at`,
`oidc_client_id`, `oidc_issuer`. Skip scope `xai::api_key`. Confirm with
`MaskSecret`.

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
* `TestConfigureLLM_APIKeyKindUnchanged`: gemini/openai api_key path still
  sets `Kind=CredAPIKey` and `APIKey`.
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
AuthKind string `json:"auth_kind,omitempty"` // "" or "api_key" (legacy) | "oauth"
```

No access token, no refresh token, no issuer in this struct. Legacy files
with only `api_key` keep working: empty `AuthKind` means API key.

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

`ValidateActive`:

* If `pc.AuthKind == "oauth"`: do **not** require `apiKey`. Require
  `OAuthStore.Load(ctx, provider)` returns a non-nil session, and `pc.Model`
  non-empty. Error text: `no OAuth session for provider %q; run 'prepare-commit-msg configure'`.
* Else: today's API-key check unchanged.

`ResolveAPIKey` unchanged (OAuth path does not call it for construction).

**Red tests** (`internal/config/config_test.go`):

* `TestSave_OAuthKindDoesNotWriteTokens`: `AuthKind=oauth`, `APIKey=""`;
  read the saved `config.json` as text; fail if it contains `"access"` or
  `"refresh_token"` as JSON keys. Write this against an implementation that
  mistakenly stores tokens on `ProviderConfig` — it MUST fail — then keep
  tokens out of the struct.
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
   * `Kind == CredAPIKey` or empty: `pc.AuthKind = "api_key"` (or `""` if we
     want byte-identical legacy — **use `"api_key"` when Kind is API key so
     the field is explicit**; empty still loads as API key). `pc.APIKey =
     res.APIKey`. `store.Delete` any leftover oauth file for that provider
     so a later generate cannot prefer a stale session.
   * `Kind == CredOAuth`: `pc.AuthKind = "oauth"`. **`pc.APIKey` stays empty
     (do not copy `res.AccessToken`).** Wizard already `Save`d the
     TokenStore. Fail if `res.APIKey != ""` in tests.
   * Model and fallbacks as today.
5. Drop the check `d.RequiresAPIKey && pc.APIKey == ""` for oauth results
   (`setup.go:208-210`). Replace with: if Kind is API key and descriptor
   requires a key and APIKey empty → same error; if Kind is oauth and
   `store.Load` is nil → error.

Existing interactive tests (`TestRunSetupInteractive_Success` gemini input
`1\ny\n…`, claude `3\n…`) **must keep passing**: gemini and claude have no
auth-method menu, so their scripted input is unchanged.

**New tests:**

* `TestRunSetupInteractive_OpenAIAPIKeyStillWorks`: select openai (menu
  index **2**), then auth method `api_key` (index **1**), then key, model,
  fallbacks, operational. Assert `AuthKind=="api_key"` and `APIKey` set and
  `oauth/openai.json` absent.
* `TestRunSetupInteractive_ImportGrokSession`: write a fixture
  `GROK_HOME/auth.json` (OIDC map shape from the MADR probe, **fake**
  tokens, `auth_mode: oidc`, issuer `https://auth.x.ai`). Select grok
  (index **4**), `import_vendor_cli`. Assert `AuthKind=="oauth"`,
  `APIKey==""`, `oauth/grok.json` exists mode `0600`, `config.json` has no
  refresh token. No network.
* `TestRunSetupInteractive_ChatGPTDoesNotCopyAccessIntoAPIKey`: fixture
  `CODEX_HOME/auth.json` with `tokens.access_token` / `refresh_token`.
  Select openai, import. Assert `APIKey==""`.

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
func openBrowser(url string) error
```

`darwin`: `exec.Command("open", url)`, `linux`: `xdg-open`, `windows`:
`cmd /c start`. Combined output discarded. Non-zero return is **not** fatal
(wizard still prints the URL). Tests never need a real browser: they use
import fixtures.

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
  ./scripts/go-precheck.sh internal/config/config.go internal/config/config_test.go internal/ui/setup.go internal/ui/setup_test.go internal/ui/open.go main.go main_test.go
  go test -race ./...
} >"$LOG" 2>&1
```

`gofmt -l` empty. `go-precheck.sh` exit 0. Tests exit 0. Then
`git commit --no-edit` **in prepare-commit-msg**. Do not commit mcplib in
this step. Do not push.

Coverage floor is 80% (`Makefile` `COVERAGE_MIN`). If Phase 10 drops below,
add tests rather than lowering the floor.

## 5. Verification commands (every mcplib phase)

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

`gofmt -l` must print **no paths**. Do not pipe `make lint` into `tail`
before inspecting `EXIT`.

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
| A10 | OpenAI loopback is 1455 then 1457 only | `TestOpenAILoopback_Binds1455Then1457` |
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
* `llmprovider/constants.go`
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

## 12. Execution record

Empty until the maintainer approves this plan and a phase lands.
