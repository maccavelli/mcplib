---
date: 2026-09-26
subject: llmprovider compatibility with the Kilo, OpenCode, Grok and Codex reference sources
examines: 0001-MADR-add-grok-xai-llm-provider.md, 0003-MADR-add-gateway-llm-providers.md, 0008-MADR-subscription-auth-for-llm-providers.md, 0009-MADR-live-catalog-model-search.md, 0010-MADR-use-case-aware-default-model-ranking.md
---
# Provider Source Compatibility Audit: Kilo, OpenCode, Grok and Codex

This report records findings. It decides nothing. Each finding names the
record it bears on and the kind of decision it needs. Accepted records it
contradicts are annotated with a pointer here, not rewritten.

## Scope and method

`mcplib`'s provider code was compared against the four reference
implementations of the services it talks to. The references are pinned
because their checkouts moved during the investigation.

| Reference | Repository | Commit | What it contains |
|---|---|---|---|
| Kilo | `kilocode` | `c267794785` (2026-09-26) | Kilo gateway client (`packages/kilo-gateway`), the Kilo CLI runtime (`packages/opencode`), gateway docs (`packages/kilo-docs`) |
| OpenCode | `opencode` | `696f41bc8e` (2026-09-25) | OpenCode client, and the Zen/Go **server** (`packages/console`) |
| Grok | `grok-build` | `f0e3be11` (2026-09-23) | xAI's Grok CLI (Rust): API client, OIDC login, model defaults |
| Codex | `codex` | `25270df261` (2026-09-26) | OpenAI's Codex CLI (Rust): ChatGPT backend client, OAuth, model catalog |

`mcplib` was read at `55e4b31`; the working tree differed only in `docs/`.
Line citations into other `docs/` records refer to their text at `55e4b31`.
The pointer notes this report added shift the later lines of those files.
Four read-only audits, one per reference, compared these areas, citing both
sides:
* endpoints and headers;
* request bodies and response parsing;
* errors and retries;
* OAuth and credential files;
* model catalogs.

Each finding carries a verification level:

* **L:** confirmed by a live request. Only anonymous requests with a bogus
  key were made, which spend nothing.
* **V:** confirmed by reading both sides' source line by line.
* **R:** reported by an audit with citations; representative citations
  spot-checked, not every one.

Classes follow the audits:

* **A:** compatibility risk. `mcplib` may send what the service rejects or
  mishandles, or mis-parse what it returns.
* **B:** a standard or capability worth adopting.
* **C:** harmless difference.

## Summary

The four audits produced 93 compared items. The findings that need action,
most severe first:

1. **OpenCode Zen/Go sends the key where two routes do not read it (A, L).**
   The Zen server reads the key from `x-api-key` on `/messages` and from
   `x-goog-api-key` on the Google route. `mcplib` sends only
   `Authorization: Bearer` on every route. Consequences:
   * every Anthropic-shaped and Google-shaped Zen/Go model fails with
     "Missing API key.";
   * that includes three of the six `StaticOpencodeZen` defaults;
   * a free model on those routes silently runs anonymously.
2. **Tool-call history is dropped in four request converters (A, V).**
   Consequences:
   * a multi-turn tool conversation loses the assistant's tool call;
   * gateways then discard the orphaned tool result.
3. **ChatGPT (Codex backend) requests differ from every request Codex sends
   (A, V).** Codex always sends `stream: true` and `store: false` and never
   sends `max_output_tokens`. `mcplib` does the opposite on all three. No
   live ChatGPT request has ever been made from `mcplib`.
4. **The ChatGPT catalog is retired (A, V).** Codex has removed all three
   `StaticOpenAIChatGPT` slugs.
5. **Importing a vendor CLI session shares a single-use refresh token (A, V).**
   Both Codex and Grok document refresh-token reuse detection. `mcplib`'s
   import copies the token and refreshes it independently.
6. **OpenCode Go gates some models by region and training consent (A, V).**
   `StaticOpencodeGo` includes a region-gated model.
7. **Grok's reasoning-effort table is wrong for grok-4.5 (A, V).**
8. **Error bodies are discarded, and quota exhaustion is retried (A, V/R).**
   This affects Kilo, Zen/Go and ChatGPT, and each publishes typed error
   bodies.
9. **Non-streaming timeouts are one tenth of the references' (A, V/R).** The
   references allow 300 s for streamed calls. `mcplib` allows 30 s to
   response headers and 60 s in total.

One decision is confirmed by the Grok source: sending Grok OAuth sessions to
`api.x.ai` without `X-XAI-Token-Auth` (MADR 0008 §2). The OpenAI OAuth
constants and both device-code flows match their references.

## Findings

### Cross-provider

**X1. Tool-call and reasoning history dropped (A, V).**
* Four converters serialise only `MessageItem` and `FunctionCallOutputItem`:
  * `itemsToChatMessages` (`llmprovider/chatcompletions.go:27-49`);
  * `itemsToInput` (`grok.go:132-150`), used by OpenAI, Grok and the OpenCode
    Responses route;
  * `claudeItemsToMessages` (`claude.go:138-166`), used by Claude and the
    OpenCode messages route;
  * `geminiItemsToContents` (`gemini.go:133-167`), used by Gemini and the
    OpenCode Google route.
* A `FunctionCallItem` in the input is silently dropped, so a following
  `tool` message has no matching assistant `tool_calls`.
* Kilo's gateway "Removes tool result messages without matching tool calls"
  (`kilocode:packages/kilo-docs/pages/gateway/api-reference.md:237`, V).
* OpenAI and Grok recover through server-side `Continue`. Chat Completions
  providers (Kilo, OpenCode chat route, Hugging Face) cannot.
* Kilo's runtime also replays reasoning on assistant turns for interleaved
  models (`kilocode:packages/opencode/src/provider/transform.ts:310-326`, R).

**X9. Claude converter turns `system` into `assistant` (A, V; found while
designing the follow-up MADR, not by the audits).** `claudeItemsToMessages`
maps any role other than `user` to `assistant` (`llmprovider/claude.go:143-148`).
A `MessageItem{Role: "system"}` therefore becomes an assistant turn. The
Messages API has no `system` role in `messages`; it takes a top-level `system`
field. This affects Claude and the OpenCode messages route.

**X2. Error bodies discarded (A/B).**
`classifyHTTPError` maps on status alone (`llmprovider/http_helpers.go:90-108`,
V). All four services return typed bodies:

| Service | Body shape and types | Cited at |
|---|---|---|
| Kilo | `{error:{message,code,metadata}}`; codes `PAID_MODEL_AUTH_REQUIRED`, `PROMOTION_MODEL_LIMIT_REACHED` | `kilocode:packages/kilo-docs/pages/gateway/api-reference.md:340-347` (R) |
| OpenCode | `{type:"error",error:{type,message}}` | `opencode:packages/console/app/src/routes/zen/util/handler.ts:480-543` (R); seen live in L1 |
| Codex | `error.type` `usage_limit_reached` (with `resets_at`, `plan_type`) and `usage_not_included` | `codex:codex-rs/codex-api/src/api_bridge.rs:186`, `:217` (V) |
| xAI | nested and flat envelopes | `grok-build:crates/codegen/xai-grok-sampling-types/src/error.rs:605-629` (R) |

Status-only mapping has two effects:

* Different conditions collapse together. OpenCode 401 covers auth, credits,
  monthly limit and model-unavailable. 403 covers region and data-policy.
* Useful text is lost: for example Kilo's `metadata.buyCreditsUrl`.

**X3. Quota exhaustion retried (A).** `GenerateWithRetry` retries every 429,
capping `Retry-After` at 30 s (`llmprovider/provider.go:128-173`, V). Each
reference treats quota exhaustion as terminal:

* **Kilo** never retries `FreeUsageLimitError` or its own Kilo errors
  (`kilocode:packages/opencode/src/session/retry.ts:92`, `:106-110`, V).
* **Codex** never retries 429 (`retry_429: false`,
  `codex:codex-rs/model-provider-info/src/lib.rs:444`, V). Usage-limit errors
  are terminal (`codex-rs/protocol/src/error.rs:384-411`, R).
* **OpenCode**'s `FreeUsageLimitError` carries `retry-after` equal to the
  seconds until UTC midnight
  (`opencode:packages/console/app/src/routes/zen/util/ipRateLimiter.ts:39-40`,
  R).

So `mcplib` retries into an exhausted quota, spending the free per-IP budget.

**X4. Retry-After parsing (B).** `mcplib` parses integer seconds and HTTP-date
values (`llmprovider/provider.go:94-110`, V). It rejects fractional seconds.
The references also accept:
* `retry-after-ms` and fractional seconds (Kilo, `retry.ts:47-78`, R);
* an `x-should-retry: false` veto, with 525/526 terminal (xAI,
  `grok-build:crates/common/xai-circuit-breaker/src/retry_policy.rs:34-48`, R).

**X5. Timeouts (A for reasoning models).** `defaultHTTPClient` sets a 30 s
`ResponseHeaderTimeout` and a 60 s total (`llmprovider/options.go:11-21`, V).
Every reference streams with a 300 s idle or first-byte limit (R):
* Kilo: `kilocode:packages/opencode/src/kilocode/provider/provider.ts:25`;
* OpenCode: `opencode:packages/opencode/src/provider/provider.ts:1799-1800`;
* Grok: `grok-build:…/xai-grok-sampler/src/actor/request_task.rs:41`;
* Codex: `codex:codex-rs/model-provider-info/src/lib.rs:63`.

`mcplib` never streams, and a high-effort reasoning call can exceed 30 s
before headers arrive. The timeout is not an auth or invalid-request error,
so it is retried and billed again.

**X6. Client identification (B; A for OpenCode Go).**
* `mcplib` sends Go's default `User-Agent` and no client headers (V: grep over
  `llmprovider/*.go` finds no User-Agent).
* OpenCode Go's docs require a client-specific user agent and a stable
  `x-opencode-session` (`opencode:packages/web/src/content/docs/go.mdx:105-111`,
  R).
* Codex sends `originator`, a descriptive `User-Agent` and `session-id`, which
  keys prompt-cache affinity
  (`codex:codex-rs/login/src/auth/default_client.rs:152-179`,
  `codex-rs/core/src/client.rs:599`, R).
* Kilo sends `X-KILOCODE-EDITORNAME` and uses `X-KiloCode-TaskId` for
  prompt-cache keying (`kilocode:packages/kilo-gateway/src/headers.ts:85-88`,
  R).
* The CLI-specific headers `x-grok-*` and `X-XAI-Token-Auth` must **not** be
  imitated: the proxy version-gates on them.

**X7. Responses `status: incomplete` ignored (B/A).** The Responses decoder
does not read `status` or `incomplete_details` (`llmprovider/http_helpers.go:24-77`,
V). A reasoning run truncated at `max_output_tokens` then returns empty text
without an error. Grok and Codex treat it as an error (R).

**X8. Discovery health probes spend real generations (B; A for OpenCode Go).**
`DiscoverModels` generates against up to six models (`llmprovider/probe.go:10-40`,
V). No reference probes this way:
* they use listing and metadata instead;
* Grok validates keys with `GET /v1/api-key`
  (`grok-build:crates/codegen/xai-grok-login/src/api_key_probe.rs:82-84`, R);
* Go's docs expect "typical coding agent traffic"
  (`opencode:packages/web/src/content/docs/go.mdx:101-109`, R).

### OpenCode Zen and Go

**O1. Key header per route (A, L, V).**
* Zen `/messages` reads the key with
  `parseApiKey: (headers) => headers.get("x-api-key")`
  (`opencode:packages/console/app/src/routes/zen/v1/messages.ts:9`, V).
* The Google route reads `x-goog-api-key` (`…/zen/v1/models/[model].ts:9`, V).
* Chat and Responses read `authorization` (`…/zen/v1/chat/completions.ts:9`,
  V).
* The inference proxy picks the header the same way
  (`opencode:packages/console/app/src/lib/inference-proxy.ts:39-43`, V).
* `mcplib` sets only `Authorization: Bearer`, with the comment "The gateway
  accepts only Authorization: Bearer on every route — it ignores x-api-key
  and x-goog-api-key" (`llmprovider/opencode.go:258-262`, V).
* `TestOpencode_KeyInHeader` forbids the vendor headers
  (`llmprovider/opencode_test.go:107-127`, V).

**Live confirmation (L1, 2026-09-26, key `sk-bogus-000`):**

| Request | Response |
|---|---|
| `POST /zen/v1/messages`, `Authorization: Bearer …` | `{"type":"error","error":{"type":"AuthError","message":"Missing API key."}}` |
| `POST /zen/v1/messages`, `x-api-key: …` | `… "Invalid API key."` |
| `POST /zen/v1/models/gemini-3.7-flash:generateContent`, `Authorization: Bearer …` | `… "Missing API key."` |
| same, `x-goog-api-key: …` | `… "Invalid API key."` |

MADR 0003's evidence for the Bearer-only rule was a probe of `/responses` only
(`0003-MADR-add-gateway-llm-providers.md:126-137`). That probe is correct for
that route and does not generalise.

Affected models:
* on Zen, every model with `provider.npm` `@ai-sdk/anthropic` or
  `@ai-sdk/google` in `api.json`;
* on Go, the Anthropic-shaped models;
* in `StaticOpencodeZen`: `claude-haiku-4-5`, `gemini-3.7-flash` and
  `gemini-3.5-flash-lite`.

**O2. Routes can come from `api.json` (B; A for new models).**
* OpenCode chooses each model's wire format as
  `model.provider?.npm ?? provider.npm ?? "@ai-sdk/openai-compatible"`
  (`opencode:packages/opencode/src/provider/provider.ts:1274-1278`, R).
* The server rejects a model sent to the wrong format, with a 500 or a 401
  `ModelError` (`…/zen/util/handler.ts:215`, `:554-560`, R).
* `mcplib` uses a hand table plus a prefix heuristic
  (`llmprovider/opencode_route.go:91-218`).
* Against the current `api.json`, the Zen table has no disagreements for
  listed models. It is missing 18 active models, and its heuristic misroutes
  `qwen3.8-max` to messages (R).
* MADR 0003's statement that the models.dev npm field "does **not** describe
  gateway dispatch" (`0003-MADR…:188-191`) overlooks the per-model override
  (V: `api.json` gives `gemini-3.7-flash` → `@ai-sdk/google`,
  `qwen3.8-flash` → `@ai-sdk/anthropic`).
* `jev-*` models use a fifth route, `systemone`, which `api.json` does not
  list (R).
* Go's qwen models conflict: `api.json` implies chat_completions, while the Go
  docs and `mcplib` say messages. The server truth is in secret configuration.
  This needs a live probe.

**O3. Go region and training-consent gates (A, V).** On Go (`modelList: "lite"`):
* `deepseek-v4.1-flash`, `deepseek-flash`, `deepseek-v4-flash` and
  `deepseek-v4-pro` throw `RegionError` unless the workspace regions include
  `cn` (`opencode:…/zen/util/handler.ts:162-171`, V).
* The default regions add `cn` only for requests from China
  (`opencode:packages/console/core/src/workspace.ts:88-89`, V).
* `muse-spark-1.3-contributor` and `muse-spark-1.2-contributor` throw
  `DataPolicyError` without training consent
  (`…/zen/util/trainingConsent.ts`, `handler.ts:147-152`, V).
* `StaticOpencodeGo` includes `deepseek-v4-flash`
  (`llmprovider/models_catalog.go:79-86`), so non-CN Go users get a 403 on a
  default.

**O4. Keyless use (B).** The server treats the key `"public"` as anonymous
(`…/zen/util/handler.ts:103-104`, R). OpenCode's client uses it for free
models. `mcplib` rejects an empty key (`llmprovider/opencode.go:45-47`, V).

**O5. Request conventions (B).** These need a live check per model family (R):
* **Responses route:** `store:false`, `prompt_cache_key` and
  `include:["reasoning.encrypted_content"]`
  (`opencode:packages/opencode/src/provider/transform.ts:1235-1243`,
  `:1380-1384`).
* **Chat route:** `reasoning_effort` when `api.json` advertises effort options
  (`transform.ts:994-1002`). `mcplib` deliberately sends none
  (`llmprovider/opencode.go:222-231`), so `GenerateThinking` on Zen/Go chat
  models does not reason.
* **Messages route:** adaptive thinking for Claude 4.7+ and MiniMax-M3
  (`transform.ts:657-684`, `:1293-1296`).

**O6. Live tests target deprecated models (B).** `hy3-free` and
`muse-spark-1.2-contributor-free` are `status: "deprecated"` in `api.json`
(`llmprovider/live_gateways_test.go:96`, `:113`, `:140`, R).

### Kilo

**K1. Reasoning parameter (B; A possible).**
* `mcplib` sends flat `reasoning_effort`, gated on that capability
  (`llmprovider/kilo.go:151-165`, V).
* Kilo's runtime sends `reasoning: { effort }` for the Kilo gateway
  (`kilocode:packages/opencode/src/provider/transform.ts:1398-1399`, V).
  For small tasks it sends `reasoning: { enabled: true }` (`:1678-1681`, R).
* `reasoning_effort` appears in the gateway's parameter list for only 137 of
  its 273 reasoning models (0010 Context §5).
* The gateway docs document neither field (R).

**K2. Data-collection opt-out not sent (B, policy gap).**
* Kilo injects `provider.data_collection: "deny"` when its privacy setting is
  on (`kilocode:packages/kilo-gateway/src/responses.ts:52-56`, V). A test pins
  the wire field (`test/responses.test.ts:108-116`, R).
* `mcplib` enforces its training policy only at listing time, so a request to
  a model it did not list (for example a typed id, `kilo-auto/free`, or a
  static fallback) carries no opt-out.
* The docs warn that some free models require data collection to be allowed
  (R). A live check is needed before this becomes a default.

**K3. `max_tokens` value (A conditional / B).**
* `mcplib` always sends 8192 (`llmprovider/options.go:116`, V).
* Kilo sends `min(model output limit, 32000)`, the limit taken from
  `top_provider.max_completion_tokens` in the listing (R).

**K4. Organization- and URL-scoped tokens (A for those tokens only).**
* Kilo tokens may carry a URL prefix ending in `:` that selects the host and
  organization
  (`kilocode:packages/kilo-gateway/src/auth/token.ts:9`, V).
* Organization scope uses `X-KILOCODE-ORGANIZATIONID` and
  `/api/organizations/{id}/models` (R).
* `mcplib` supports neither.

**K5. Listing (B).**
* Kilo retries `/models` anonymously on a 401 when not organization-scoped
  (`kilocode:packages/kilo-gateway/src/api/models.ts:242-245`, V).
* Kilo treats a missing `supported_parameters` as tool-capable
  (`api/models.ts:101-105`, V). No live model omits it today (0009 Context §5).
* `kilo-auto/balanced`, in `StaticKilo`, is not in the gateway docs' tier list
  (R).

**K6. Base path (C; record accuracy).**
* Kilo's own client sends chat and catalog requests to
  `api.kilo.ai/api/openrouter` (`kilocode:packages/kilo-gateway/src/api/constants.ts:25`,
  V).
* MADR 0003 calls that path "an alias retained for the editor extension"
  (`0003-MADR…:402-407`). It is the Kilo CLI's primary path.
* `mcplib` uses the documented `api.kilo.ai/api/gateway`, which is also
  correct, so no code change is needed.

**K7. Anonymous use (B).** Kilo uses the key `"anonymous"` for free models
(`kilocode:packages/kilo-gateway/src/api/constants.ts:49`, V). `mcplib`
rejects an empty key (`llmprovider/kilo.go:55-57`, R).

### Grok

**G1. Session routing and `X-XAI-Token-Auth` confirmed (C).**
* The Grok CLI adds `X-XAI-Token-Auth` only for cli-chat-proxy URLs
  (`grok-build:crates/codegen/xai-grok-shell/src/agent/proxy_headers.rs:21-27`,
  V).
* It sends session bearers to `api.x.ai` for image and voice calls without
  that header (R).
* This supports MADR 0008 §2. The CLI's own model turns for sessions use the
  proxy, so `mcplib`'s `api.x.ai` model route still rests on 0008's live probe.

**G2. grok-4.5 does not offer `xhigh` (A, V).**
* The CLI's `default_models.json` lists efforts
  `[xhigh, high, medium, low]` for grok-4.6 but `[high, medium, low]` for
  grok-4.5 (`grok-build:crates/codegen/xai-grok-models/default_models.json`).
* `mcplib` allows `xhigh` for any `grok-4.5*` prefix
  (`llmprovider/grok_reasoning.go:25-62`, V). MADR 0001 states the same
  (`0001-MADR-add-grok-xai-llm-provider.md:151-152`).
* The CLI sends an effort only if the model's menu offers it (R).

**G3. Refresh-token reuse after import (A, V).**
* The Grok CLI documents that re-sending a rotated refresh token "trips the
  IdP's reuse detection and revokes a successor a sibling may hold"
  (`grok-build:crates/codegen/xai-grok-login/src/oidc/refresh.rs:32-35`).
* It writes rotated tokens to disk before any network I/O for the same reason
  (`…/xai-grok-login/src/manager.rs:750`).
* `mcplib`'s import copies the refresh token into its own store and refreshes
  independently (`wizard/import.go`). See also C4.

**G4. Refresh omits team principal (A for team logins, V).** The CLI sends
`principal_type` and `principal_id` on refresh when the login has them
(`…/xai-grok-login/src/oidc/protocol.rs:485-500`). `mcplib` sends neither
(`llmprovider/oauth_session.go:129-144`, R).

**G5. Import picks a scope nondeterministically (A edge, V).**
* `importGrokAuth` ranges over a Go map and returns the first non-API-key
  entry (`wizard/import.go:102-115`). Go map iteration order is random, so a
  file with more than one session scope imports an arbitrary one.
* The CLI keys the active scope as `"{issuer}::{client_id}"` (R).
* It also honours `GROK_AUTH_PATH` and marks some entries `auth_mode:
  external`, which it does not refresh itself (R).

**G6. Other Grok items (B/C, R):**
* Truncation (`status: incomplete`) is undetected (X7).
* A fixed `max_output_tokens` of 8192 is sent; the CLI leaves it to the
  server.
* `store` defaults to true on the API; the CLI sends `false` for ZDR
  compliance. `mcplib`'s `Continue` needs stored responses.
* The tool `description` is not sent.
* 403 means policy denial, not re-authentication.
* The model filter admits `grok-imagine-video-1.5` ("imagine" does not
  contain "image").
* Device-code `user_code` and verification-URI checks are missing.
* The CLI's default models are grok-4.6 then grok-4.5; `StaticGrok` leads with
  grok-3-mini-fast.

### Codex / ChatGPT backend

**C1. Streaming, `store` and `max_output_tokens` (A, V).**
* Codex's request is built with `store: false` and `stream: true`
  (`codex:codex-rs/core/src/client.rs:1007-1008`, V).
* `max_output_tokens` is not a field of its request type (R). OpenCode's Codex
  plugin strips it with the comment "Match codex cli" (R).
* `mcplib` sends `max_output_tokens`, sets no `stream` or `store`, and decodes
  one JSON body (`llmprovider/openai.go:119-123`, V).
* Whether the backend rejects non-streaming requests is not in the sources.
  MADR 0008 records that no live ChatGPT probe has been run
  (`0008-MADR-subscription-auth-for-llm-providers.md:780-783`, R).
* `Continue` (`previous_response_id`) is sent by Codex only over WebSocket.
  With `store: false` it has nothing to chain from over HTTP (R).

**C2. Catalog retired (A, V).**
* Codex's bundled `models-manager/models.json` lists `gpt-6-astra`,
  `gpt-6-sol`, `gpt-6-luna`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`,
  `gpt-5.5` and hidden entries.
* It does not list `gpt-5.4`, `gpt-5.4-mini` or `gpt-5.3-codex`, which make
  up `StaticOpenAIChatGPT` (`llmprovider/models_catalog.go:48-52`).
* Migration prompts map `gpt-5.4` → `gpt-6-sol` and `gpt-5.4-mini` →
  `gpt-6-luna` (R).
* A live listing exists: `GET {base}/models?client_version=…` (R). Choosing a
  `client_version` value is a policy question, because it presents as a Codex
  version.

**C3. Usage-limit errors (A, V).** Codex parses `usage_limit_reached` (with
`resets_at`) and `usage_not_included`, and never retries 429 (X2, X3). `mcplib`
reports a plan without Codex access as "rate limited" and retries it.

**C4. Refresh-token rotation (A, V/R).**
* Codex maps `refresh_token_reused` to a permanent failure
  (`codex:codex-rs/login/src/auth/manager.rs:1687`, V).
* Codex reloads storage before refreshing, so another process's rotation is
  picked up (R).
* `mcplib` never re-reads its store, and import shares Codex's refresh token
  (R).
* `mcplib` wraps permanent refresh failures in an untyped error that
  `GenerateWithRetry` retries (R).

**C5. OAuth details.**
* **Matches (C, V/R):** issuer, client id, scopes, ports 1455→1457, PKCE,
  code exchange and device flow.
* **Redirect host (B, V):** Codex now uses
  `http://127.0.0.1:<port>/auth/callback` (`codex:codex-rs/login/src/server.rs:193`,
  changed 2026-09-24). `mcplib` uses `localhost` (`llmprovider/oauth_loopback.go:258`).
* **Also absent from `mcplib` (B, R):**
  * the `life_sciences` state-suffix tolerance;
  * the `missing_codex_entitlement` message;
  * token revocation;
  * JWT-`exp`-based expiry with a 5-minute window;
  * JSON refresh bodies (`manager.rs:1637`: `TokenEncoding::Json`, V).

**C6. `CODEX_ACCESS_TOKEN` semantics (A, R).** In Codex this variable is
either a personal access token (`at-` prefix), hydrated through a `whoami`
call for the account id, or an agent-identity JWT sent with a different
scheme. `mcplib` treats it as a ChatGPT OAuth bearer with no account id
(`wizard/auth.go`). MADR 0008 names it as a headless source
(`0008-MADR…:145-146`, `:564`).

**C7. Residency header provenance (C, R).** `mcplib`'s
`x-openai-internal-codex-residency` logic comes from OpenCode's Codex plugin.
Codex does not read the `chatgpt_compute_residency` claim at all.

## Records affected

| Record | Status | Statement | Finding | Action taken |
|---|---|---|---|---|
| `0003-MADR-add-gateway-llm-providers.md` | accepted | Zen/Go auth is Bearer "on any route" (`:126-137`) | O1 (L): wrong for `/messages` and Google | Pointer note added |
| same | accepted | models.dev npm "does not describe gateway dispatch" (`:188-191`) | O2: the per-model `provider.npm` does | Pointer note added |
| same | accepted | `/api/openrouter` is an editor alias (`:402-407`) | K6: it is Kilo's primary path | Pointer note added |
| same | accepted | Zen 429 carries no `Retry-After` (`:212`) | X3: the server sets it for gateway-generated limits and strips it from upstream 429s; re-measure | Pointer note added |
| `0008-MADR-subscription-auth-for-llm-providers.md` | accepted | `CODEX_ACCESS_TOKEN` as a ChatGPT bearer (`:145-146`, `:564`) | C6 | Pointer note added |
| same | accepted | ChatGPT transport and catalog | C1, C2, C3, C5 | Pointer note added |
| same | accepted | Vendor-session import | G3, G5, C4 | Pointer note added |
| `0001-MADR-add-grok-xai-llm-provider.md` | proposed | grok-4.5 accepts `xhigh` (`:151-152`) | G2 | Pointer note added |
| `0009-MADR-live-catalog-model-search.md` | proposed | ChatGPT short-circuit catalog; Kilo listing fallbacks; Zen key headers | C2, K5, O1 | Amended (revision 4); O1 decided as §1c (revision 5) |
| `0010-MADR-use-case-aware-default-model-ranking.md` | proposed | Go defaults; Zen defaults; request-side reasoning | O1, O3, O5, K1, K2, X5 | Amended (revision 2) |

## Candidate decisions

These groupings are suggestions for the maintainer. None is decided here.

1. **Gateway transport conformance (new MADR).**
   * ~~O1: per-route key headers. This is the most urgent item: three Zen
     defaults are broken today.~~ Decided in 0009 §1c on 2026-09-26, by
     maintainer direction.
   * O2: routes from `api.json`, with the table as fallback.
   * X2 and X3: typed error bodies and terminal quota errors.
   * X4: `Retry-After` forms.
   * X5: timeouts or streaming.
   * X6: client identification.
   * O4 and K7: public and anonymous keys.
   * X8: probe replacement.
2. **ChatGPT backend conformance (supersedes parts of 0008).**
   * C1: streaming, `store`, `max_output_tokens` and `Continue`.
   * C2: the catalog and live listing.
   * C3: usage-limit errors.
   * X6: `originator`, `User-Agent` and `session-id`.
   * **Precondition:** a live ChatGPT probe, which 0008 deferred.
3. **OAuth session hygiene (supersedes parts of 0008).**
   * G3, G5 and C4: import must not share a refresh token, for example by
     reading through the vendor file or by requiring a fresh login.
   * G4: the refresh principal.
   * C4: typed permanent refresh failures.
   * C5: the redirect host, revocation and JWT expiry.
   * C6: `CODEX_ACCESS_TOKEN`.
4. **Tool round-trip fidelity (bug-fix PLAN under 0001's canonical item
   contract).** X1: serialise `FunctionCallItem` in the Chat Completions,
   Grok and Claude converters.
5. **Grok reasoning and catalog (small PLAN under 0001).**
   * G2: per-model effort menus.
   * G6: the model filter and `StaticGrok` order.
6. **Kilo request conventions.** K1, K2 and K3 overlap 0010 §6. K4 is new
   surface. It is a new MADR, or part of candidate 1.

## Not verified

These need live probes or sources outside the four repositories:

* whether the ChatGPT backend accepts non-streaming requests, or requests
  without `store: false`;
* whether it still serves the retired slugs;
* whether `localhost` redirects stay allow-listed;
* which wire route OpenCode Go actually uses for qwen models (the server
  configuration is secret);
* how the OpenCode inference service behind `ConsoleMigration.inferenceUrl`
  behaves (it handles `oc_sk_` keys and is not in the repository);
* how the Kilo gateway treats `reasoning_effort` versus `reasoning`, an
  over-limit `max_tokens`, and `data_collection: "deny"` on free models;
* whether the Zen `FreeUsageLimitError` 429 carries `Retry-After` in practice
  (MADR 0003 measured none).

## Maintainer disposition (2026-09-26)

* **O1** is folded into `0009-MADR-live-catalog-model-search.md` as §1c.
* **Candidate decisions 1–6**, O1 excepted, are combined in one MADR:
  `0012-MADR-conform-providers-to-reference-clients.md`.

## Evidence

* **Audits:** four read-only audits of the pinned commits, each comparing
  `mcplib` with one reference and citing both sides.
* **My verification:** before this report was written, I opened the source
  lines supporting every finding marked V: O1, O3, G1, G2, G3, G5, C1, C2, C4
  (`refresh_token_reused`), C5 (redirect, JSON refresh), K1, K2, K4, K5, K6,
  K7, X1 and X3.
* **Live L1:** four `curl` requests from this workstation, 2026-09-26, with
  the bogus key `sk-bogus-000` and no other credential. Responses are quoted
  verbatim in O1.
* **Routes:** the per-model `provider.npm` values in O2 and O3 come from
  `https://models.opencode.ai/api.json`, fetched anonymously on 2026-09-25.
* **Scratch artefacts:** outside the repository, in the session scratchpad.
  No repository file was changed while gathering evidence.

## Corrections

* **2026-09-26, X4:** first written as "`mcplib` parses integer seconds only".
  `parseRetryAfter` also parses HTTP-date values (`llmprovider/provider.go:104-108`).
  The finding is corrected in place.
* **2026-09-26, X1:** first written as three converters. The audits did not
  examine Gemini's. `geminiItemsToContents` also drops `FunctionCallItem`, so
  the count is four.
