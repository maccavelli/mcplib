---
status: proposed
date: 2026-09-26
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev, prepare-commit-msg
informed: all mcplib consumers
---
# Conform `llmprovider` to the Reference Clients of Kilo, OpenCode, Grok and Codex

## Context and Problem Statement

`mcplib` talks to four services whose owners publish their own clients:
* the Kilo gateway (`kilocode`);
* OpenCode Zen and Go (`opencode`, which also contains the Zen/Go **server**);
* the xAI API (`grok-build`, the Grok CLI);
* the ChatGPT Codex backend (`codex`, the Codex CLI).

[0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md)
compared `mcplib` at `55e4b31` with those clients, pinned at `c267794785`,
`696f41bc8e`, `f0e3be11` and `25270df261`. It found that `mcplib` diverges
from what the services expect in ways that break requests, waste quota, or
risk logging users out.

The maintainer directed that every candidate decision in that report become
one MADR. The exception is O1, the Zen/Go key header, which was folded into
`0009-MADR-live-catalog-model-search.md` §1c. This is that MADR.

Finding identifiers below (X1, O2, K1, G2, C1, …) refer to the report, which
holds the evidence and marks each item **L** (live-confirmed), **V**
(source-confirmed on both sides) or **R** (reported, citation spot-checked).
The facts this decision depends on most are summarised here. All `mcplib`
paths are at `55e4b31`.

### 1. Shared request path

* **Errors.** `classifyHTTPStatus` maps on status alone
  (`llmprovider/http_helpers.go:78-108`). `openai.go`, `claude.go`, `gemini.go`
  and `grok.go` each duplicate that switch.

  The four services all return typed error bodies (X2, V/R):

  | Service | Types |
  |---|---|
  | OpenCode | `AuthError`, `CreditsError`, `MonthlyLimitError`, `UserLimitError` and `ModelError` on 401; `RegionError` and `DataPolicyError` on 403; `RateLimitError`, `FreeUsageLimitError`, `GoUsageLimitError` and `BlackUsageLimitError` on 429 with `retry-after` (`opencode:packages/console/app/src/routes/zen/util/handler.ts:480-532`, V) |
  | Kilo | `PAID_MODEL_AUTH_REQUIRED`, `PROMOTION_MODEL_LIMIT_REACHED` (`kilocode:packages/opencode/src/kilocode/kilo-errors.ts:5-6`, V) |
  | Codex | `usage_limit_reached`, `usage_not_included` (`codex:codex-rs/codex-api/src/api_bridge.rs:186`, `:217`, V) |
  | xAI | nested and flat envelopes (R) |

* **Retries.** `GenerateWithRetry` retries every error except
  `ErrAuthFailure` and `ErrInvalidRequest`. It caps `Retry-After` at 30 s
  (`llmprovider/provider.go:128-173`). Each reference treats quota exhaustion
  as terminal (X3, V):
  * Kilo: `kilocode:…/session/retry.ts:92`, `:109`;
  * Codex: `retry_429: false`, `codex:…/model-provider-info/src/lib.rs:444`.
* **Timeouts.** The shared client allows 30 s to response headers and 60 s in
  total (`llmprovider/options.go:11-21`). Every reference streams with a 300 s
  idle or first-byte limit (X5, R).
* **Identification.** `mcplib` sends Go's default `User-Agent` and no client
  or session header (X6, V). OpenCode Go's docs require a client-specific
  agent and a stable `x-opencode-session`. Codex and Kilo key prompt-cache
  affinity on a session header.
* **Truncation.** The Responses decoder ignores `status: "incomplete"`
  (`llmprovider/http_helpers.go:24-77`), so a truncated reasoning run returns
  empty text as success (X7, V).
* **Probes.** `DiscoverModels` spends one real generation per candidate
  (`llmprovider/probe.go:10-40`). No reference probes that way (X8, V/R).

### 2. Canonical items

The item contract is `MessageItem`, `FunctionCallItem`,
`FunctionCallOutputItem` and `ReasoningItem` (`llmprovider/item.go:20-51`).
Four converters drop `FunctionCallItem` (X1, V):
* `itemsToChatMessages`;
* `itemsToInput` (OpenAI, Grok, OpenCode Responses);
* `claudeItemsToMessages` (Claude, OpenCode messages);
* `geminiItemsToContents` (Gemini, OpenCode Google).

A tool result therefore arrives without its call. Kilo's gateway then deletes
the orphaned result (`kilocode:packages/kilo-docs/pages/gateway/api-reference.md:237`,
V).

`claudeItemsToMessages` also turns a `system` message into an `assistant` turn
(`llmprovider/claude.go:143-148`, X9, V).

### 3. Gateways

* **OpenCode routing.** OpenCode's client picks each model's wire format from
  `api.json` `provider.npm`
  (`opencode:packages/opencode/src/provider/provider.ts:1274-1278`). `mcplib`
  uses a hand table dated 2026-08-28. It is missing 18 active Zen models and
  misroutes `qwen3.8-max` (O2, R). `jev-*` models use a fifth route,
  `systemone`.
* **OpenCode Go gates.** Region- and consent-gated models (O3, V) are handled
  for defaults by
  `0010-MADR-use-case-aware-default-model-ranking.md` §3. At request time they
  surface only as an untyped 403 (X2).
* **Keyless use.** The server treats the key `"public"` as anonymous (O4, R).
  Kilo's client uses `"anonymous"` (K7, V). Both `NewOpencode` and `NewKilo`
  reject an empty key.
* **Kilo request conventions:**
  * Kilo's runtime sends `reasoning: { effort }` (K1, V). `mcplib` sends
    `reasoning_effort`; 0010 §6 already decides the change.
  * `provider.data_collection: "deny"` is Kilo's opt-out from providers that
    train on prompts (K2, V). `mcplib` never sends it, although its policy
    excludes such models at listing time.
  * `max_tokens` is fixed at 8192, where Kilo clamps to the listing's
    `max_completion_tokens` (K3, R).
  * URL-prefixed and organization tokens are unsupported (K4, V).
  * A 401 on `/models` falls back to the static list instead of retrying
    anonymously (K5, V).

### 4. ChatGPT backend

* **Transport.** Codex's `/responses` requests always carry `stream: true` and
  `store: false`, and never `max_output_tokens`
  (`codex:codex-rs/core/src/client.rs:1007-1008`, V). `mcplib` sends
  `max_output_tokens`, omits `stream` and `store`, and decodes one JSON body
  (`llmprovider/openai.go:118-123`, `:198`) (C1).
* **No live check has run.** No live ChatGPT request has ever been made from
  `mcplib`
  (`0008-MADR-subscription-auth-for-llm-providers.md:780-783`, at `55e4b31`).
* **Catalog.** `StaticOpenAIChatGPT` (`gpt-5.4`, `gpt-5.4-mini`,
  `gpt-5.3-codex`) is absent from Codex's catalog, which now offers the
  `gpt-6-*` and `gpt-5.6-*` families and `gpt-5.5` (C2, V). The backend
  publishes `GET {base}/models?client_version=…` (R).
* **Errors and headers.**
  * Usage-limit errors are typed and terminal in Codex (C3, V).
  * Codex sends `X-OpenAI-Fedramp: true` for FedRAMP accounts
    (`codex:codex-rs/model-provider/src/bearer_auth_provider.rs:43-45`, V).
  * Codex sends `originator`, a descriptive `User-Agent` and `session-id`
    (X6, R).

### 5. OAuth sessions

* **Shared refresh tokens.** Both vendors detect refresh-token reuse (G3, C4,
  V).
  * Grok: "Re-sending the RT then trips the IdP's reuse detection"
    (`grok-build:…/xai-grok-login/src/oidc/refresh.rs:35`).
  * Codex treats `refresh_token_reused` as permanent
    (`codex:…/login/src/auth/manager.rs:1687`).

  `mcplib`'s import copies the vendor's refresh token into its own store and
  refreshes independently (`wizard/import.go`). It never re-reads its store
  before refreshing (`llmprovider/oauth_session.go:27-68`).
* **Grok import.** It picks a scope from a Go map in random order
  (`wizard/import.go:102`, G5, V). It ignores `GROK_AUTH_PATH`, which the CLI
  honours (`grok-build:…/xai-grok-login/src/storage.rs:46-54`, V).
* **Refresh request.** It is form-encoded, and Codex uses JSON
  (`manager.rs:1637`, V). It omits Grok's `principal_type` and `principal_id`
  for team logins (G4, V). Its failures are untyped, so `GenerateWithRetry`
  retries them (C4, R).
* **Login.**
  * Codex now redirects to `http://127.0.0.1:<port>/auth/callback`; `mcplib`
    uses `localhost` (`llmprovider/oauth_loopback.go:258`, C5, V).
  * Codex tolerates a `life_sciences` state suffix
    (`codex:…/login/src/server.rs:366-373`, V).
  * Codex revokes tokens at `https://auth.openai.com/oauth/revoke`
    (`manager.rs:213`, V); `mcplib` has no revocation.
* **`CODEX_ACCESS_TOKEN`.** In Codex this is a personal access token (`at-`)
  or an agent-identity JWT
  (`codex:codex-rs/login/src/auth/access_token.rs:1-14`, V). `mcplib`'s wizard
  offers it as a ChatGPT OAuth bearer (`wizard/auth.go:173-181`, C6).

### 6. Grok

* **Effort menus.** The CLI offers `grok-4.6` efforts
  `xhigh/high/medium/low` and `grok-4.5` only `high/medium/low`
  (`grok-build:crates/codegen/xai-grok-models/default_models.json`, G2, V).
  `grokClampReasoningEffort` allows `xhigh` for any `grok-4.5*` or `grok-4.6*`
  prefix (`llmprovider/grok_reasoning.go:25-62`).
* **Catalog filter.** The CLI's default models are `grok-4.6` then `grok-4.5`.
  `isUsableGrokModel` admits `grok-imagine-video-1.5`, because "imagine" does
  not contain "image" (G6, R).
* **Tool definitions.** The tool `description` is not sent on the Grok path
  (G6, R).

## Decision Drivers

* **Conform to the service's own client where it is observable.** The
  reference clients are what the services are built and tested against.
* **Never impersonate.** Identify as `mcplib` or the consuming application,
  never as `codex_cli_rs`, `Kilo-Code`, `opencode`, or the Grok CLI. Never send
  a header whose purpose is to vouch for another client, such as
  `X-XAI-Token-Auth` or `x-grok-*`.
* **Fail fast on conditions a retry cannot fix.** Exhausted quota, missing
  entitlement, region or data-policy denial, and a revoked token are
  terminal. Retrying them spends quota and delays the real error.
* **Keep existing callers compiling.** Every current sentinel still matches
  with `errors.Is`. New error types wrap existing sentinels, and new options
  are additive.
* **Prove unverifiable behaviour live before shipping it.** Where the sources
  cannot say what a server accepts (non-streaming ChatGPT calls, `originator`
  values, Kilo `data_collection` on free models, the Go qwen route), a gated
  live characterization runs first. Its result amends this record.
* **Share one refresh token with no one.** A session `mcplib` refreshes must be
  one `mcplib` created.
* **Separable delivery.** The groups below are independent enough to ship as
  separate plans in any order, with the dependencies §9 states.

## Considered Options

* Conform to the reference clients across all four services, one decision with live-characterization gates and separate plans per area
* Fix only the source-verified class-A defects and defer every standard
* Adopt streaming everywhere and model every provider on its vendor SDK
* Record the findings and change nothing

## Decision Outcome

Chosen option: **"Conform to the reference clients across all four services,
one decision with live-characterization gates and separate plans per area"**.
It fixes every verified defect, adopts the conventions the services document
or their own clients follow, keeps the public API source-compatible, and
refuses to guess where the sources are silent.

### 1. Shared transport

**1.1 Typed API errors.** Add

```go
// APIError is a non-2xx response with the service's own error classification.
type APIError struct {
	Provider   string
	Status     int
	Type       string        // service error type or code, e.g. "FreeUsageLimitError"
	Message    string        // service message, bounded to 512 bytes
	RetryAfter time.Duration
	Terminal   bool          // true when retrying cannot succeed
	sentinel   error         // one of the Err* sentinels below
}

func (e *APIError) Error() string
func (e *APIError) Unwrap() error // returns sentinel
```

and two sentinels, `ErrQuotaExhausted` and `ErrNotPermitted`. A per-service
body parser, reading at most 64 KiB of the error body, fills `Type` and
`Message`:

| Service | Envelope |
|---|---|
| OpenCode | `{type:"error",error:{type,message}}` |
| Kilo | `{error:{code,message,metadata}}` or `{code}` |
| OpenAI/Codex | `{error:{type,code,message}}` |
| xAI | nested `{error:{…}}` or flat `{code,error}` |
| Claude, Gemini | their documented `error.message` |

Classification is below. Anything not listed keeps today's status mapping,
now with the message attached.

| Condition | Sentinel | Terminal |
|---|---|---|
| OpenCode `FreeUsageLimitError`, `GoUsageLimitError`, `BlackUsageLimitError`; Kilo `PROMOTION_MODEL_LIMIT_REACHED` or a body containing `FreeUsageLimitError`; Codex `usage_limit_reached`; OpenAI `insufficient_quota` | `ErrQuotaExhausted` (which also wraps `ErrRateLimited`) | yes |
| OpenCode `RegionError`, `DataPolicyError`; Codex `usage_not_included`; Kilo 403 | `ErrNotPermitted` | yes |
| OpenCode `CreditsError`, `MonthlyLimitError`, `UserLimitError`; Kilo 402 | `ErrQuotaExhausted` | yes |
| OpenCode `ModelError`; Kilo `PAID_MODEL_AUTH_REQUIRED` | `ErrInvalidRequest` | yes |
| 401 otherwise | `ErrAuthFailure` | yes |
| 429 otherwise | `ErrRateLimited` | no |
| 5xx, except 525/526 | `ErrProviderUnavailable` | no |
| 525, 526 | `ErrProviderUnavailable` | yes |

`classifyHTTPStatus` and the four duplicated switches are replaced by one
`classifyHTTPError(provider, resp)`.

**1.2 Retry policy.** `GenerateWithRetry`, `GenerateItemsWithRetry` and the
new `GenerateThinkingWithRetry` (0010 §6) stop immediately in three cases:
* on any `APIError` with `Terminal`;
* on a response header `x-should-retry: false`;
* when `RetryAfter` exceeds the 30 s backoff cap.

In the last case they return the error with its `RetryAfter`, so the caller
can reschedule. `parseRetryAfter` additionally accepts `retry-after-ms` and
fractional seconds.

**1.3 Timeouts.** The default generation client sets `ResponseHeaderTimeout`
to 300 s and `Timeout` to 330 s, matching the references' 300 s limit. Listing
keeps its 10 s context (`llmprovider/discovery.go:30`). `WithHTTPClient` still
overrides everything.

**1.4 Identification, without impersonation.**
* `WithClientInfo(name, version string)` sets the consuming application. The
  default name is `mcplib`, and the default version comes from
  `runtime/debug.ReadBuildInfo`.
* Every request sends
  `User-Agent: <name>/<version> (<GOOS>; <GOARCH>) mcplib/<mcplib version>`.
* `WithSessionID(id)` sets a conversation id. The default is a random UUID per
  provider instance.
* Per service:
  * **OpenCode:** `x-opencode-session: <id>`.
  * **Kilo:** `X-KILOCODE-EDITORNAME: <name>` and `X-KiloCode-TaskId: <id>`.
  * **ChatGPT:** `session-id: <id>` and `originator: <name>`, subject to gate
    G-C.
  * **xAI:** the `User-Agent` only.

No request sends `x-opencode-client`, `x-grok-*` or `X-XAI-Token-Auth`, or a
reference client's `User-Agent` or `originator` value.

**1.5 Truncation is an error.** The Responses decoder reads `status` and
`incomplete_details.reason`. The Chat Completions decoder reads
`finish_reason`. They return `*IncompleteError{Reason}`, which wraps
`ErrInvalidRequest` and is terminal, in two cases:
* `status: "incomplete"` on any output;
* `finish_reason: "length"` on a response whose output is a tool call.

A text response truncated by `length` still returns its text, and exposes the
reason through the new field `Response.FinishReason`.

**1.6 Probes spend no generations on metered or quota-limited services.**
`DiscoverModels` for Kilo, OpenCode Zen/Go, Hugging Face and ChatGPT returns
the curated listing (0009's `Recommended`) without `probeGenerateHealth`.
Other providers keep today's behaviour. For Grok, key validation uses
`GET {base}/api-key` instead of a generation (`grok-build:…/xai-grok-login/src/api_key_probe.rs:82-84`,
R).

**1.7 Keyless free use.** `NewKilo` and `NewOpencode` accept an empty key and
then send the service's anonymous token: `anonymous` for Kilo, `public` for
Zen/Go. Paid models then fail with the typed error of §1.1. Descriptors keep
`RequiresAPIKey: true`, so the wizard is unchanged.

### 2. Canonical item fidelity

* **Tool calls.** Each of the four converters emits a `FunctionCallItem` in the
  provider's native assistant tool-call form:
  * Chat Completions: an `assistant` message with `tool_calls[{id, type:"function", function:{name, arguments}}]`;
  * Responses: a `{type:"function_call", call_id, name, arguments}` item;
  * Anthropic: an `assistant` message with a `tool_use` block
    `{id, name, input}`, where `input` is the arguments decoded as JSON;
  * Gemini: a `model` turn with `functionCall {name, args}`.

  Consecutive call items are grouped into one assistant turn where the
  provider requires it (Anthropic, Chat Completions).
* **Reasoning.** A `ReasoningItem` is replayed only where the reference client
  does: as `reasoning_content` on the preceding assistant message, for models
  whose `api.json` entry declares `interleaved.field: "reasoning_content"`
  (O5). Elsewhere it is omitted, as today.
* **System messages.** `claudeItemsToMessages` moves every `system`
  `MessageItem` into the request's top-level `system` field, joined in order,
  instead of emitting an assistant turn.

### 3. Gateways

**3.1 OpenCode routes from metadata.**
* The route for a Zen/Go model is derived from the per-model `provider.npm` in
  the metadata that 0010 §2 fetches:

  | `provider.npm` | Route |
  |---|---|
  | `@ai-sdk/openai` | responses |
  | `@ai-sdk/anthropic` | messages |
  | `@ai-sdk/google` | google |
  | anything else, or unset | chat_completions |

* `opencode_route.go`'s table remains the fallback when metadata is
  unavailable. It is refreshed from the same metadata at implementation time.
* An explicit `WithOpencodeRoute` still overrides both.
* `jev-*` (`systemone`) models are added to `isUsableOpencodeModel`'s deny
  list. The route is undocumented in the metadata and `mcplib` has no encoder
  for it.
* **Gate G-O:** a live probe decides which route OpenCode **Go** uses for qwen
  models, where the metadata and the Go docs disagree. Until it runs, the table
  keeps `messages` for Go qwen.

**3.2 OpenCode request conventions.**
* **Responses route:** sends `store: false` (OpenCode's client does:
  `opencode:packages/opencode/src/provider/transform.ts:1235-1243`, R).
* **Chat route:** sends `reasoning_effort` when the metadata advertises an
  effort option (0010 §6).
* **Messages route:** selects adaptive thinking for Claude 4.7+ and MiniMax-M3,
  as `transform.ts:657-684` and `:1293-1296` do (R), each under a live check.

**3.3 Kilo request conventions.**
* **Data collection:** sends `provider.data_collection: "deny"` by default.
  `WithKiloDataCollection(true)` opts out. **Gate G-K** first establishes what
  the gateway does with `deny` on a free model that requires collection. If it
  rejects, the error surfaces through §1.1 and the default stands.
* **Reasoning:** the `reasoning: { effort }` object per 0010 §6.
* **`max_tokens`:** clamped to the listing's `top_provider.max_completion_tokens`,
  else `max_completion_tokens`, when the listing is at hand (it is decoded by
  0010 §2).
* **URL-prefixed tokens:** a token matching
  `^(https?://[^:]+(?::\d+)?(?:/[^:]*)?):`
  (`kilocode:packages/kilo-gateway/src/auth/token.ts:9`) selects the base URL
  and path prefix; the whole token is still sent as the bearer.
* **Organizations:** an `/api/organizations/{id}` segment in that URL, or
  `WithKiloOrganization(id)`, adds `X-KILOCODE-ORGANIZATIONID` and lists
  `/api/organizations/{id}/models`.
* **`/models` 401:** a 401 with a key, when not organization-scoped, is retried
  once without credentials before falling back to static
  (`kilocode:packages/kilo-gateway/src/api/models.ts:242-245`).

**3.4 Live tests.** The opt-in live suite stops targeting models whose metadata
says `deprecated` (O6). It picks active free models from the metadata at run
time, with the current ids as a skip-if-absent fallback.

### 4. ChatGPT backend

This section supersedes MADR 0008's ChatGPT transport and catalog.

**Gate G-C (first, before any change here).** A maintainer-run, opt-in live
characterization with a real ChatGPT login. It records, for the current
default model, whether the backend:
1. accepts a non-streaming request;
2. accepts one without `store: false`;
3. accepts one with `max_output_tokens`;
4. accepts `originator: mcplib`;
5. accepts the `localhost` redirect;
6. still serves `gpt-5.4` and `gpt-5.4-mini`;
7. answers `GET /models` with `client_version` set to `mcplib`'s own version.

The results are recorded in this MADR's amendment. §4.1–§4.4 apply as written
unless a result contradicts them.

**4.1 Streaming.** ChatGPT-mode calls send `stream: true` with
`Accept: text/event-stream`. A new SSE reader collects
`response.output_item.done` items into the existing `Response` and takes the id
from `response.created` or `response.completed`. It maps `response.failed` and
`response.incomplete` onto §1.1 and §1.5. The reader lives in
`http_helpers.go` for reuse. Streaming is not extended to other providers by
this decision.

**4.2 Body.**
* Adds `store: false`, `include: ["reasoning.encrypted_content"]` and
  `prompt_cache_key: <session id>`.
* Omits `max_output_tokens`.
* `Continue` returns `ErrInvalidRequest` in ChatGPT mode: with `store: false`
  there is nothing to chain from over HTTP (C1).

**4.3 Catalog.**
* `StaticOpenAIChatGPT` becomes the Codex catalog's picker-visible models that
  gate G-C confirms are served, highest priority first. At `25270df261` those
  are `gpt-6-astra`, `gpt-6-sol`, `gpt-6-luna`, `gpt-5.6-sol`, `gpt-5.6-terra`,
  `gpt-5.6-luna` and `gpt-5.5`, capped at `MaxListedModels`.
* If G-C item 7 succeeds with `mcplib`'s own version, 0009's ChatGPT
  short-circuit becomes that live listing: `visibility == "list"`, sorted by
  `priority`. `mcplib` never sends a Codex version string.

**4.4 Headers and errors.**
* **Headers:** `originator` and `session-id` per §1.4; `X-OpenAI-Fedramp: true`
  when the id-token claim `chatgpt_account_is_fedramp` is set. The residency
  header stays, and its source (OpenCode's Codex plugin, not Codex) is
  recorded in a code comment.
* **Errors:** `usage_limit_reached` becomes `ErrQuotaExhausted` with
  `RetryAfter` derived from `resets_at`; `usage_not_included` becomes
  `ErrNotPermitted` (§1.1).

### 5. OAuth session hygiene

This section supersedes MADR 0008's vendor-session import and refresh
behaviour.

**5.1 Imported sessions are read-through, never refreshed by `mcplib`.**
* Importing from `~/.codex/auth.json` or the Grok CLI's `auth.json` no longer
  copies a refresh token.
* A new `llmprovider.VendorCLISession{Provider, Path}` implements
  `TokenSource`. It re-reads the vendor file on every `Token` call and returns
  its access token. When that token has expired, it returns `ErrAuthFailure`
  with "run `codex login` / `grok` to refresh", instead of refreshing.
* The wizard reports such a credential as a new kind, `CredVendorCLI`, with
  `Result.VendorAuthPath` set. Both changes are additive. Consumers persist
  the path, not tokens.
* Path resolution:
  * Codex: `$CODEX_HOME/auth.json`.
  * Grok: `$GROK_AUTH_PATH`, else `$GROK_HOME/auth.json`, else
    `~/.grok/auth.json`.
* The Grok entry chosen is exactly the key `"{issuer}::{client_id}"` for the
  default issuer and client, never a random map entry.
* Expiry comes from the file's own field when present, else from the access
  token's JWT `exp`.

**5.2 Refresh of `mcplib`-owned sessions.**
* Before refreshing, `OAuthSession.Token` re-loads the session from its
  `TokenStore`. If the stored refresh token differs from the in-memory one,
  it adopts the stored session and skips the refresh.
* Refresh bodies:
  * OpenAI: JSON (`TokenEncoding::Json`, `codex:…/manager.rs:1637`);
  * Grok: form-encoded, adding `principal_type` and `principal_id` when the
    login returned them. `fileRecord` gains both, as optional fields.
* Failures:
  * `invalid_grant`, `invalid_client`, `refresh_token_expired`,
    `refresh_token_reused` and `refresh_token_invalidated` are wrapped in
    `ErrAuthFailure` (terminal).
  * Transport errors and 5xx are retried up to three times inside `Token`.
* Expiry uses the access token's JWT `exp` when it parses, else
  `expires_in`. The refresh window becomes 5 minutes (Codex and Grok both use
  5).

**5.3 Login.**
* The OpenAI loopback redirect becomes `http://127.0.0.1:<port>/auth/callback`,
  on the same ports 1455 and 1457. This is subject to gate G-C item 5, whose
  result only confirms the choice, since Codex has moved.
* A callback `state` equal to the expected state plus Codex's `life_sciences`
  suffix is accepted.
* An authorize error surfaces its `error_description`. `missing_codex_entitlement`
  gets the message "this ChatGPT plan does not include Codex".
* Grok device-code responses are validated before use:
  * `user_code` must match `[A-Za-z0-9-]+`;
  * the verification URI must be `https`, or `http` to loopback.
* `llmprovider.RevokeOAuthSession(ctx, *OAuthSession)` posts to the issuer's
  revocation endpoint: `https://auth.openai.com/oauth/revoke` for OpenAI, and
  the discovery document's `revocation_endpoint` for Grok when present.

**5.4 `CODEX_ACCESS_TOKEN`.** The wizard's `token_stdin` method stops offering
`CODEX_ACCESS_TOKEN` as a ChatGPT OAuth bearer (`wizard/auth.go:173-181`).
Pasting an OAuth access token on stdin remains. Codex personal-access-token
support is out of scope.

### 6. Grok

* **Effort menus.** `grokClampReasoningEffort` uses exact per-model menus from
  the CLI's catalog:

  | Model | Efforts |
  |---|---|
  | `grok-4.6` | `xhigh`, `high`, `medium`, `low` |
  | `grok-4.5` | `high`, `medium`, `low` |
  | `grok-4.6-build` | `low`, `high` |
  | `grok-3-mini*` | `low`, `high` |
  | anything else | omitted |

  A requested effort not on the menu clamps to the nearest lower one, else
  the menu's highest.
* **Catalog filter.** `isUsableGrokModel` also rejects `imagine`, `video`,
  `voice`, `stt` and `tts`.
* **Static catalog.** `StaticGrok` leads with the CLI defaults `grok-4.6` and
  `grok-4.5`.
* **Tool descriptions.** Grok's tool definition sends `description`.
* **Storage.** `WithStore(bool)` is added for the Responses providers (OpenAI
  API-key mode and Grok). The default leaves the service default, which keeps
  `Continue` working. Callers under zero-data-retention set `false`.

### 7. What does not change

* `Prompter`, the wizard flow, and every exported function signature. The
  additions are new types, options, sentinels, a credential kind and a
  `Result` field.
* Every existing sentinel still matches the conditions it matched before, so
  `errors.Is(err, ErrRateLimited)` still holds for every 429.
* Grok sessions to `api.x.ai` without `X-XAI-Token-Auth` (0008 §2, confirmed
  by the report's G1). The OpenAI OAuth constants and device flows, which match.
* The Zen/Go key header, which is decided in 0009 §1c.

### 8. Plans

This MADR carries several PLANs, one per independent unit of work:

| Plan | Covers |
|---|---|
| `0012-PLAN-shared-transport.md` | §1 |
| `0012-PLAN-item-fidelity.md` | §2 |
| `0012-PLAN-gateway-conventions.md` | §3 (after 0009 and 0010) |
| `0012-PLAN-chatgpt-backend.md` | §4, opening with gate G-C |
| `0012-PLAN-oauth-hygiene.md` | §5 |
| `0012-PLAN-grok.md` | §6 |

### 9. Dependencies

* `0012-PLAN-shared-transport.md` first: §1.1's typed errors are the carrier
  for §3–§5's terminal conditions.
* §3.1 and §3.3 reuse 0010 §2's metadata client, so they follow 0010.
* §4 follows gate G-C.
* §2, §5 and §6 are independent of each other.

### Consequences

* Good, because every verified class-A item in the report gets a fix, except
  O1, which 0009 §1c fixes. That covers dropped tool calls, retired ChatGPT
  slugs, shared refresh tokens, grok-4.5 `xhigh`, futile retries, timeouts and
  silent truncation.
* Good, because quota, entitlement and policy failures become typed and
  terminal. Consumers can tell "wait until reset" from "change plan" from
  "retry".
* Good, because services see an honest, stable client identity and a session
  id, which is what Go's terms ask for. Prompt-cache affinity improves where
  services key on it.
* Good, because imported vendor sessions keep working as long as the vendor
  CLI keeps them fresh, and never revoke the CLI's own login.
* Good, because OpenCode routes follow the same metadata as OpenCode's client,
  so new models route correctly without an `mcplib` release.
* Neutral, because live gates G-C, G-K and G-O need a maintainer's credentials
  or network access, and their results may amend §3–§4.
* Bad, because the surface grows:
  * types: `APIError`, `IncompleteError`, `VendorCLISession`;
  * sentinels: `ErrQuotaExhausted`, `ErrNotPermitted`;
  * options: `WithClientInfo`, `WithSessionID`, `WithKiloDataCollection`,
    `WithKiloOrganization`, `WithStore`;
  * functions: `RevokeOAuthSession`;
  * wizard: `CredVendorCLI` and `Result.VendorAuthPath`.
* Bad, because consumers must persist and reload `CredVendorCLI` credentials,
  a change in each wizard consumer, before import works end to end.
* Bad, because the ChatGPT SSE reader is new parsing code on a path with no
  live test today. Gate G-C and a recorded-stream fixture are its only
  evidence until a live suite exists.
* Bad, because a 300 s header timeout lets a hung call block a caller with no
  deadline of its own for up to 330 s.
* Bad, because rejecting `jev-*` removes a family of Zen models from use
  rather than supporting the `systemone` route.

### Confirmation

Each check below must first be seen to fail against a planted defect (the
method of `0009-PLAN-live-catalog-model-search.md` Appendix B):

* **Errors (§1.1–§1.2):**
  * one fixture per row of the §1.1 table asserts the sentinel, `Terminal`,
    `Type` and a bounded `Message`;
  * `GenerateWithRetry` makes exactly one call for a terminal error, and for a
    `Retry-After` above the cap;
  * `retry-after-ms: 1500` and `retry-after: 1.5` parse to 1.5 s;
  * existing `errors.Is` assertions in the test suite pass unmodified.
* **Timeouts and identification (§1.3–§1.4):**
  * the default client's two timeouts equal the stated values;
  * each service's request carries exactly the §1.4 headers, and never the
    forbidden ones.
* **Truncation (§1.5):** an `incomplete` Responses fixture and a `length` +
  tool-call Chat fixture return `*IncompleteError`.
* **Item fidelity (§2):** a round trip `[user, FunctionCallItem,
  FunctionCallOutputItem]` through each of the four converters produces the
  native call form immediately before its result. A `system` item reaches
  Claude's top-level `system`.
* **OpenCode routes (§3.1):**
  * metadata with `@ai-sdk/anthropic` routes to messages;
  * metadata absent falls back to the table;
  * `WithOpencodeRoute` wins.
* **Refresh and import (§5.2, §5.1):**
  * **Store reload:** a store whose refresh token changed underneath results
    in no refresh request.
  * **Read-through import:** a `VendorCLISession` whose file is rewritten
    between calls returns the new token, and never contacts a token endpoint.
  * **Scope selection:** a Grok file with two scopes selects the exact key
    every time; the test runs 50 iterations to defeat map ordering.
* **Grok (§6):** `grok-4.5` + `xhigh` sends `high`; `grok-imagine-video-1.5`
  is not usable.
* **Gates:** each gate's transcript (with credentials redacted) is recorded in
  the owning plan, and any contradiction is amended here before its section
  ships.

## Pros and Cons of the Options

### Conform to the reference clients across all four services, one decision with live-characterization gates and separate plans per area

* Good, because it closes every verified defect and adopts the conventions the
  services' own clients use.
* Good, because the gates keep unverifiable behaviour from shipping on
  inference.
* Good, because separate plans let the transport fixes land first and the
  ChatGPT work wait for its gate.
* Bad, because it is the largest option and adds public surface.

### Fix only the source-verified class-A defects and defer every standard

* Good, because it is smaller and every change has direct evidence.
* Bad, because it leaves the conditions that cause repeated harm:
  * retries into exhausted quota;
  * an unidentified client on a service whose terms ask for identification;
  * silent truncation;
  * routes that drift from the metadata.

### Adopt streaming everywhere and model every provider on its vendor SDK

* Good, because it matches the references most closely and removes the
  timeout problem.
* Bad, because it rewrites every provider's transport. Streaming is proven
  necessary only for ChatGPT (C1). The timeout problem has a one-line
  alternative (§1.3).

### Record the findings and change nothing

* Good, because it costs nothing now.
* Bad, because tool calls stay dropped, imported sessions keep risking
  revocation, and ChatGPT stays unverified.

## More Information

### Relationship to earlier decisions

* **`0001-MADR-add-grok-xai-llm-provider.md`:** §2 completes its canonical item
  contract. §6 corrects its grok-4.5 effort statement (lines 151-152 at
  `55e4b31`).
* **`0003-MADR-add-gateway-llm-providers.md`:**
  * §1.1 replaces its status-only error mapping for the gateways;
  * §3.1 replaces its route table as the primary source;
  * the report's pointer note lists the statements it contradicts.
* **`0008-MADR-subscription-auth-for-llm-providers.md`:**
  * §4 supersedes its ChatGPT transport and catalog;
  * §5 supersedes its vendor-session import and refresh behaviour;
  * §5.4 withdraws its `CODEX_ACCESS_TOKEN` path.

  Its authentication-method set, OAuth constants, device flows and Grok routing
  stand. 0008 is annotated with a pointer to this record when it is accepted.
* **`0009-MADR-live-catalog-model-search.md`:**
  * §1c already decides O1;
  * §4.3 can turn its ChatGPT short-circuit into a live listing;
  * §3.3's anonymous `/models` retry fits its fetch step.
* **`0010-MADR-use-case-aware-default-model-ranking.md`:**
  * §3 reuses its metadata client;
  * its §6 request-side reasoning is referenced, not repeated;
  * its Go gates handle defaults, and §1.1's `ErrNotPermitted` handles
    requests.

### Out of scope

* Codex personal-access-token and agent-identity authentication.
* The Codex WebSocket transport and its `previous_response_id` chaining.
* Streaming for providers other than ChatGPT.
* An `mcplib` encoder for OpenCode's `systemone` route.
* Kilo organization administration beyond scoping requests and listings.
* Consumer changes, such as persisting `CredVendorCLI` or setting
  `WithClientInfo`, which each consumer adopts on its own bump.

### Evidence

* **Findings and verification levels:**
  [0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md).
* **Confirmed for this record:**
  * the four converters and the Claude `system` mapping (`llmprovider/grok.go:132-150`,
    `claude.go:138-166`, `gemini.go:133-167`, `chatcompletions.go:27-49`);
  * `parseRetryAfter`'s formats (`llmprovider/provider.go:94-110`);
  * the OpenCode error classes by status (`handler.ts:480-532`);
  * Codex's `life_sciences` suffix, `X-OpenAI-Fedramp` and revoke URL;
  * Kilo's two error codes;
  * the Codex `CODEX_ACCESS_TOKEN` classification;
  * the Grok CLI's `GROK_AUTH_PATH` resolution.
* **Line citations into other `docs/` records** refer to their text at
  `55e4b31`.
