---
status: accepted
date: 2026-08-28
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev consumers
informed: mcplib contributors
---

# Add OpenCode Zen/Go, Hugging Face and Kilo Gateway Providers on a Shared Chat Completions Primitive

> **Revision notes (revision 4, applied in place to this same `proposed` document,
> not a superseding MADR, since it has not been accepted — same convention as
> `0001-MADR`):**
>
> Revision 1 of this MADR chose a *delegating* provider that would construct and call
> the existing `OpenAIProvider` / `ClaudeProvider` / `GeminiProvider` structs with a
> `WithBaseURL` override. Revision 2 records live verification of the OpenCode gateway
> performed 2026-08-28 against `https://opencode.ai/zen/v1` and
> `https://opencode.ai/zen/go/v1`, plus a re-read of the provider sources. That
> verification **invalidated the delegation mechanism** (§"Why delegation to the
> existing provider structs cannot work") and **corrected eight factual claims** about
> the gateway (§"Corrections to revision 1"). The *shape* of the decision — one
> provider surface, multiple wire formats selected per model — survived verification
> and is strengthened by it; the *mechanism* changed from delegation to in-package
> decoder reuse. Original claims are struck through rather than deleted, so the record
> shows what was assumed and what was measured.
>
> **Revision 3 (2026-08-29)** broadens the scope from the two OpenCode gateways to a
> third provider, **Hugging Face Inference Providers**, by explicit direction. The
> broadening is not arbitrary bundling: live verification (§"Hugging Face Inference
> Providers facts") established that Hugging Face's router speaks **OpenAI Chat
> Completions** — precisely one of OpenCode's four routes. Shipping the two separately
> would either duplicate that wire format or force a rename refactor of OpenCode's chat
> helpers a release later. Revision 3 therefore promotes Chat Completions from an
> OpenCode implementation detail to a **shared package primitive** and renames the
> helpers accordingly. Everything revisions 1–2 decided about OpenCode stands unchanged;
> the Hugging Face facts section, the Hugging Face parts of the Decision Outcome, and
> three new option analyses are new.
>
> **Revision 5 (2026-08-29)** adds **wire-shape pinning** (§"Pinning what was probed"),
> by explicit direction, after a sibling repo surfaced the same class of risk at runtime.
> No decision from revisions 1–4 changes; this adds a mechanism that makes the *existing*
> decisions verifiable over time. It also strengthens the live tests from liveness checks
> into shape assertions, which is what makes the pin mean anything.
>
> **Revision 4 (2026-08-29)** broadens the scope once more, to a fourth provider —
> **Kilo Gateway** (the API behind the Kilo Code agent) — again by explicit direction.
> Live verification (§"Kilo Gateway facts") found that Kilo is *also* an OpenAI Chat
> Completions gateway, making it the **third consumer** of the shared primitive revision 3
> introduced and turning that decision from a two-caller convenience into the load-bearing
> structure of this change. Kilo did, however, force one substantive amendment: it returns
> reasoning under the OpenRouter field name `message.reasoning`, not OpenCode's
> `message.reasoning_content`, so the shared decoder must tolerate **both** names.
> Revisions 1–3 are otherwise unchanged.

## Context and Problem Statement

`mcplib/llmprovider` is an SDK-free (`net/http` only, `go 1.26.5` per `go.mod:3`)
abstraction over LLM backends. The package currently supports four concrete providers
reachable through `NewProvider` (`llmprovider/provider.go:213-226`):

* `ProviderGemini = "gemini"` (`llmprovider/constants.go:5`, `llmprovider/gemini.go`)
* `ProviderOpenAI = "openai"` (`llmprovider/constants.go:6`, `llmprovider/openai.go`)
* `ProviderClaude = "claude"` (`llmprovider/constants.go:7`, `llmprovider/claude.go`)
* `ProviderGrok = "grok"` (`llmprovider/constants.go:8`, `llmprovider/grok.go`)

A fifth integration, Ollama, exists only as listing support (`llmprovider/discovery.go:33-34`,
`ValidateOllamaURL` at `discovery.go:232-251`, `listOllamaModels` at `discovery.go:190-228`)
with no `Provider` struct.

The shared contract (`llmprovider/provider.go:19-24`) is:

```go
type Provider interface { Name() string; Generate(ctx context.Context, prompt string) (string, error) }
```

plus optional capability interfaces all four providers satisfy (`llmprovider/interface_test.go:7-66`):
`ToolProvider`, `ThinkingProvider`, `ThinkingToolProvider`, `ItemProvider` / `ItemToolProvider` /
`ItemThinkingProvider` / `ItemThinkingToolProvider` (`llmprovider/item.go:88-111`), `Continuer`
(`item.go:113-117`), and `ModelDiscoverer`. Construction is via functional options
(`llmprovider/options.go:23-89`) with `WithHTTPClient`, `WithMaxTokens`, `WithBaseURL`,
`WithThinkingBudget`, `WithReasoningEffort`; defaults are a 60s-timeout `http.Client`
(`llmprovider/options.go:11-21`) and `MaxTokens: 8192`.

Registration is a hardcoded switch, not a registry; adding a provider touches exactly five
places (`llmprovider/constants.go:3-9`, `llmprovider/provider.go:106-111` `ProviderEnvVars`,
`llmprovider/provider.go:213-226` `NewProvider`, `llmprovider/discovery.go:18-38`
`ListAvailableModels`, `llmprovider/models_catalog.go:97-110` `StaticModels` plus
`isUsable*`/`Rank*` filters). Error handling is uniform: typed sentinels
`ErrRateLimited`/`ErrAuthFailure`/`ErrProviderUnavailable`/`ErrInvalidRequest` plus
`RateLimitError` with `Retry-After` parsing (`llmprovider/provider.go:64-103`), a 1 MB
`io.LimitReader` applied *before* the status check, and opt-in retry decorators
`GenerateWithRetry` / `GenerateItemsWithRetry` (`llmprovider/provider.go:113-209`).

There is no support for OpenCode Zen, OpenCode Go, or Hugging Face. Tree searches for
`opencode`, `huggingface`, and `hf_` all return zero matches. All three are greenfield
additions. Baseline is green: `go test ./llmprovider/...` → `ok ... 0.623s`,
`go vet ./llmprovider/...` → clean.

This MADR covers **four new provider identifiers across three gateway families**. All are
*aggregators* — they resell many vendors' models behind one credential — a new category for
this package, whose existing four providers are all first-party vendor APIs. The three
families differ sharply, and that spread is what shapes the decision:

| | OpenCode Zen / Go | Hugging Face Inference Providers | Kilo Gateway |
|---|---|---|---|
| Wire formats reachable | **Four** | **One** (chat/completions) | **Three** (chat/completions, responses, messages) |
| Are formats interchangeable per model? | **No** — wrong route ⇒ HTTP 500 | n/a (one format) | **Yes** — the gateway translates; any model on any route |
| Route selection | Per-(gateway, model), hardcoded table | None needed | None needed — client picks its preferred format |
| Listing metadata | **None** (`owned_by` is a constant) | **Rich** (modalities, tools, throughput, latency, pricing) | **Richest** (`supported_parameters` per model, plus modalities, pricing, context, training policy) |
| Model catalog | Curated, ~64 + ~33 IDs | Open, 136 IDs / 317 offerings | Open, **366** IDs |
| Static catalog necessity | Primary source of truth | Fallback only | Fallback only |
| Credential-free model listing | Yes | Yes | Yes |
| Credential-free **generation** | Yes (free models) | **No** | **Yes** (20 free models, incl. tool calls) |
| Reasoning field on chat/completions | `reasoning_content` | undocumented | `reasoning` + `reasoning_details[]` |

All three intersect at exactly one point — the **Chat Completions** wire format — and that
intersection is why they are decided together rather than in sequence. Kilo makes the point
decisive: three of three new gateway families speak it, so it is the package's primitive,
not any one provider's detail.

### OpenCode Zen / Go facts — verified live 2026-08-28

Every claim below was measured against the running gateway, not inferred from prose. The
probe commands are reproducible and, where noted, need no credentials.

**Base URLs and auth.** Zen `https://opencode.ai/zen/v1`, Go `https://opencode.ai/zen/go/v1`.
Auth is `Authorization: Bearer <key>` and **only** that:

| Request | Result |
|---|---|
| `POST /zen/v1/responses`, no auth header | `401 {"type":"error","error":{"type":"AuthError","message":"Missing API key."}}` |
| same, `Authorization: Bearer sk-bogus-000` | `401 … "Invalid API key."` |
| same, `x-api-key: sk-bogus-000` (no Bearer) | `401 … "Missing API key."` |

The `x-api-key` result is decisive: the gateway does not read Anthropic-style or
Google-style key headers on any route.

**Environment variable.** Neither `https://opencode.ai/docs/zen/` nor
`https://opencode.ai/docs/go/` names an environment variable anywhere in its text
(`grep -oiE "OPENCODE[_A-Z]*API[_A-Z]*KEY"` over both pages → no match). The canonical
provider registry `models.dev/api.json`, which OpenCode itself publishes to, declares for
both gateways:

```
opencode     name="OpenCode Zen"  api=https://opencode.ai/zen/v1     env=['OPENCODE_API_KEY']
opencode-go  name="OpenCode Go"   api=https://opencode.ai/zen/go/v1  env=['OPENCODE_API_KEY']
```

`OPENCODE_API_KEY` is the only environment variable name evidenced, for both gateways.

**Model listing is unauthenticated.** `GET /zen/v1/models` and `GET /zen/go/v1/models`
both return `200` with **no** `Authorization` header, in OpenAI list shape:

```json
{"object":"list","data":[{"id":"claude-fable-5","object":"model","created":1787968159,"owned_by":"opencode"}, …]}
```

Zen currently lists 64 model IDs, Go 33. `owned_by` is the constant string `"opencode"`
for every entry — **the listing carries no routing, family, or capability information.**
Go's `net/http` with its default `User-Agent: Go-http-client/1.1` receives `200`
(verified with a real Go program), so the Cloudflare front is not User-Agent sensitive
for this endpoint.

**Per-model wire-format routing is real, enforced, and per-gateway.** Each gateway's docs
publish a model → endpoint table. Four routes exist:

| Route | Path | AI SDK package |
|---|---|---|
| Responses | `POST {base}/responses` | `@ai-sdk/openai` |
| Messages | `POST {base}/messages` | `@ai-sdk/anthropic` |
| Chat Completions | `POST {base}/chat/completions` | `@ai-sdk/openai-compatible` |
| Google | `POST {base}/models/<id>` | `@ai-sdk/google` |

Two independent measurements establish that these routes are **not interchangeable**:

1. *Free-model end-to-end cross-route test (no credentials needed).*
   `muse-spark-1.2-contributor-free` (docs: Responses) → `/responses` **200**,
   `/chat/completions` **500**, `/messages` **500**.
   `big-pickle` (docs: Chat Completions) → `/chat/completions` **200** with a real
   completion, `/responses` **500**.
2. *Validation-layer probe.* Model validation runs **before** auth: an unknown model with
   no auth header returns `{"error":{"type":"ModelError","message":"Model … is not supported"}}`,
   while a known model returns the `AuthError`. Most known models pass validation on most
   routes — so validation is *not* a route guard; the mismatch surfaces later as a **500**.

This refutes the reading that `/chat/completions` is a universal endpoint. It also
explains why `models.dev` lists `npm=@ai-sdk/openai-compatible` for both gateways: that
registry field is a single-value simplification and does **not** describe gateway dispatch.

**Routing differs between Zen and Go for the same model ID.** Definitive counterexample
from the two docs tables:

| Model ID | Zen route | Go route |
|---|---|---|
| `minimax-m3`, `minimax-m2.7`, `minimax-m2.5` | `/chat/completions` | `/messages` |

Other divergences: Go carries `glm-5.3`, `glm-5.3-flash`, `longcat-2.0`, `hy4-preview`,
`qwen3.8-max`, `qwen3.8-flash`, `mimo-v2.5-pro` which Zen does not; Zen carries the entire
GPT-5.x, Claude, and Gemini families which Go does not (Go's only GPT is `gpt-5.6-luna`).
**A route table keyed on model ID alone is therefore incorrect; it must be keyed on
(gateway, model).**

**The docs tables and the live listing disagree.** Zen's docs table lists `qwen3.7-max` and
`qwen3.7-plus`, but neither appears in the live `GET /zen/v1/models` response; the live
listing conversely includes `claude-sonnet-4`, `deepseek-v4-flash-free`, `kimi-k2.5`, and
`laguna-s-2.1-free`, which the docs table omits. The live listing is authoritative for
availability; the docs table is authoritative for routing. Both drift.

**Error semantics.**

* `429` carries **no** `Retry-After` header (verified by dumping response headers on a
  free-tier rate limit). `parseRetryAfter` will yield `0` and `GenerateWithRetry` falls
  through to its exponential-with-jitter path — correct behaviour, no change needed.
* Free-tier exhaustion returns `429` with `{"error":{"type":"FreeUsageLimitError", …}}`.
* Two distinct error envelope shapes are emitted. Gateway-level:
  `{"type":"error","error":{"type":"AuthError","message":…}}` (Anthropic-shaped, on all
  four routes). Upstream passthrough: `{"error":{"type":"server_error","message":"Error
  from provider (Console): …"}}` (no top-level `type`). The package classifies on HTTP
  status only and never decodes error bodies, so this asymmetry costs nothing today.

**Response shapes on the gateway match the native vendor shapes.**

* `/responses` returns a genuine OpenAI Responses envelope:
  `output[]` = `[{type:"reasoning", summary:[], encrypted_content:"…"}, {type:"message",
  content:[{type:"output_text", text:"ALPHA"}]}]`. The package's existing
  `decodeResponsesAPIOutput` (`http_helpers.go:23-76`) parses this correctly. Note the
  reasoning item carries `encrypted_content` with an **empty** `summary`, so the decoder
  yields `ReasoningItem{Text: ""}` — present but blank, matching how the package already
  treats summary-less reasoning.
* `/chat/completions` returns a standard OpenAI Chat Completions envelope with two
  non-standard additions: `message.reasoning_content` (a plain-text reasoning trace) and a
  top-level `cost` field. Example: `{"id":"router-…","object":"chat.completion","choices":
  [{"message":{"role":"assistant","content":"Hi!","reasoning_content":"We need answer …",
  "tool_calls":null}}],"usage":{…},"cost":"0"}`.
* The Google route accepts the native `:generateContent` suffix
  (`POST /zen/v1/models/gemini-3.7-flash:generateContent` reaches the auth layer exactly as
  the bare path does), so `GeminiProvider`'s URL construction
  (`gemini.go:204`) is shape-compatible with the gateway.

**Server-side conversation chaining does not work.** A `/responses` call returned
`id: resp_6a923bc83518b7c560bf443a`; an immediately following call passing that value as
`previous_response_id` returned **400**: `"referenced response not found or expired"`.
Chaining is therefore not something this MADR may assert.

**Free models are usable as unauthenticated integration-test targets.** `hy3-free`,
`nemotron-3.5-lightning-free`, `laguna-s-2.1-free`, and `big-pickle` returned real
completions from `/chat/completions` with no API key; `muse-spark-1.2-contributor-free`
did the same from `/responses`. They are rate-limited (`FreeUsageLimitError`) and
occasionally return `400 "Model is unavailable"`, so they belong behind a build tag, never
in the default `go test` path.

### Hugging Face Inference Providers facts — verified live 2026-08-29

Hugging Face **Inference Providers** is a routing proxy in front of 18 partner inference
providers (Cerebras, Groq, Together, Novita, Fireworks, DeepInfra, Baseten, Scaleway,
Nscale, Cohere, Z.ai, OVHcloud, Public AI, Featherless, fal, Replicate, WaveSpeed, and
HF's own `hf-inference`). Docs: `https://huggingface.co/docs/inference-providers/index`.
As with OpenCode, every claim below was measured, not inferred.

**Base URL and auth.** `https://router.huggingface.co/v1`. Auth is
`Authorization: Bearer hf_***`:

| Request | Result |
|---|---|
| `POST /v1/chat/completions`, `Authorization: Bearer hf_bogus000` | `401 {"error":"Invalid username or password."}`, plus `www-authenticate: Bearer realm="Authentication required"` |
| same, no auth header | `401`, `content-type: text/html` (the Hub's HTML shell, not JSON) |
| unknown model + bogus key | `401` — **auth is checked before model validation** |

Two consequences. First, unlike OpenCode there is **no credential-free model-validation
oracle**: HF resolves auth first, so the endpoint/model matrix cannot be probed without a
token. Second, the error envelope is `{"error": "<string>"}` — `error` is a **plain
string**, not the OpenAI `{"error":{"message":…,"type":…}}` object. The package classifies
on HTTP status alone and never decodes error bodies, so this costs nothing today, but any
future body-decoding must not assume the OpenAI shape.

**Environment variable.** `HF_TOKEN`, used uniformly across every official Python,
JavaScript, and cURL sample on the index page and the pricing page. No other name appears
in the Inference Providers documentation.

**Model listing is unauthenticated and richly annotated.** `GET /v1/models` returns `200`
with **no** credential (and also with a bogus one). Go's `net/http` retrieves it fine
(verified with a real Go program: `200`, 94,116 bytes, 136 models parsed). Shape:

```json
{"object":"list","data":[{
  "id":"zai-org/GLM-5.3-Flash","object":"model","created":1787640194,"owned_by":"zai-org",
  "architecture":{"input_modalities":["text"],"output_modalities":["text"]},
  "providers":[{"provider":"novita","status":"live","context_length":1048576,
    "pricing":{"input":0.075,"output":0.25},"is_free":false,"supports_tools":true,
    "supports_structured_output":false,"first_token_latency_ms":1341.8,
    "throughput":35.75,"is_model_author":false}, …]}]}
```

Measured census of that payload on 2026-08-29:

| Metric | Value |
|---|---|
| Models listed | 136 |
| `output_modalities` | `["text"]` for **all** 136 |
| `input_modalities` | `["text"]` for 96; `["image","text"]` for 40 (VLMs) |
| Provider offerings (model × provider) | 317, **all** `status:"live"` |
| Offerings with `supports_tools:true` | 220 |
| Offerings with `is_free:true` | **0** |
| Text→text models with ≥1 live tool-capable provider | 76 |
| Text→text models served by ≥2 providers | 58 of 96 |

**This is the decisive design fact for Hugging Face.** Every other provider in this package
ranks models with string heuristics — `RankGeminiModel` greps for `"flash"`, `RankGrokModel`
for `"mini-fast"` (`models_catalog.go:127-256`). Hugging Face publishes the actual figures
the heuristics are trying to approximate: measured `throughput` (tokens/sec),
`first_token_latency_ms`, `supports_tools`, `context_length`, and per-token `pricing`.
Discovery can be **data-driven** rather than guessed. Sample of the fastest live
text→text offerings:

| Model | Best provider | tok/s | TTFT ms | Tools | $/M out | Providers |
|---|---|---|---|---|---|---|
| `openai/gpt-oss-120b` | cerebras | 1105.6 | 226 | yes | 0.75 | 11 |
| `openai/gpt-oss-20b` | groq | 763.4 | 215 | yes | 0.50 | 7 |
| `zai-org/GLM-5.3` | together | 202.9 | 204.6 | yes | 4.40 | 4 |
| `meta-llama/Llama-3.1-8B-Instruct` | — | 149.6 | 608 | yes | 0.06 | 4 |
| `zai-org/GLM-5.3-Flash` | — | 144.0 | 373 | yes | 0.50 | 5 |
| `zai-org/GLM-5.2` | — | 115.5 | 254.6 | yes | 4.40 | 8 |

Query parameters on the listing are **ignored**: `/v1/models`, `/v1/models?provider=groq`,
and `/v1/models?supports_tools=true` all return the same 136 entries. Filtering is
therefore entirely client-side.

**Chat Completions is the documented surface, and it is fully featured.** The task
reference (`https://huggingface.co/docs/inference-providers/tasks/chat-completion`)
specifies `messages`, `max_tokens`, `tools`, `tool_choice` (including the forced form
`{"type":"function","function":{"name":…}}`), `response_format`, `temperature`, `top_p`,
`seed`, `stop`, `stream`, `tool_prompt`, and — significantly —

> **reasoning_effort** _string_ — Optional. Constrains effort on reasoning for models that
> support reasoning. … Common values: none, minimal, low, medium, high, xhigh. Support and
> defaults are provider and model-dependent.

The documented value set is an exact superset of this package's existing effort constants
(`effortLow`/`effortMedium`/`effortHigh`/`effortXHigh`, `constants.go:34-39`), so the
`ThinkingProvider` path maps onto a first-class documented parameter — unlike OpenCode's
chat route, which has none. The documented response is the standard envelope:
`choices[].message.{role,content,tool_calls[]}`, `id`, `model`, `usage`. Note the response
schema does **not** document a `reasoning_content` field, though individual upstream
providers may emit one.

**Model IDs carry an optional routing-policy suffix.** IDs are `<org>/<name>`, optionally
suffixed after a colon:

* `:fastest` — highest throughput (the **default** when no suffix is given)
* `:cheapest` — lowest price per output token
* `:preferred` — the caller's own provider preference order from HF settings
* `:<provider>` — pin one partner, e.g. `openai/gpt-oss-120b:groq`

Because provider selection happens **server-side**, this is a policy hint, not the
per-model wire-format decision OpenCode forces on the client. The suffix must nonetheless
be stripped before matching an ID against the listing, which returns bare `<org>/<name>`.

**A Responses endpoint exists but is undocumented and hazardous.**
`POST /v1/responses` is live and returns a genuine Responses envelope. With a bogus key it
answers **HTTP 200** with:

```json
{"id":"resp_e3bf…","object":"response","status":"failed","output":[],
 "error":{"code":"server_error","message":"401 \"Invalid username or password.\""},
 "usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}, …}
```

This is a trap for this package specifically: `classifyHTTPStatus` inspects only the status
code, so a `200` passes straight through to `decodeResponsesAPIOutput`, which finds an
empty `output[]` and returns a `*Response` with **no items and a nil error** — a silent
empty generation instead of an auth failure. Meanwhile the official index page states
plainly:

> **Note**: This OpenAI-compatible endpoint is currently available for chat completion
> tasks only.
> … **Chat Tasks Only**: Limited to conversational workloads

So `/v1/responses` is simultaneously undocumented, contradicted by the docs, and
error-unsafe under this package's classification model.

**No free tier usable for credential-free testing.** `is_free` is `false` for all 317
offerings. The pricing page grants monthly credits ($0.10 free, $2.00 PRO, $2.00/seat
Team/Enterprise) that require an account and token. Unlike OpenCode, Hugging Face
therefore admits **no unauthenticated live test**.

**Legacy `api-inference.huggingface.co` no longer resolves.** DNS lookup fails
(`nodename nor servname provided`), and `curl` exits 6. The old "Inference API
(serverless)" host is gone; `hf-inference` survives only as one partner *behind* the
router. There is no legacy endpoint to support or fall back to.

**Optional organization billing.** `X-HF-Bill-To: <org-or-resource-group>` attributes cost
to a Team/Enterprise org. Noted for completeness; not adopted (see Out of scope).

### Kilo Gateway facts — verified live 2026-08-29

Kilo Gateway is the inference API behind the **Kilo Code** agent (an open-source VS Code
coding agent). Docs: `https://kilo.ai/docs/gateway/api-reference`. As before, every claim
was measured.

**Base URL, and a host alias worth knowing.** The documented base is
**`https://api.kilo.ai/api/gateway`**. A second path, `https://kilo.ai/api/openrouter`,
also serves the gateway — and the legacy domain `kilocode.ai` issues a `308` redirect to
`kilo.ai`. The two catalogs are **byte-identical** (`cmp` over both `GET …/models`
responses: 411,089 bytes, identical), so `…/api/openrouter` is an alias retained for the
editor extension. This MADR uses the documented `api.kilo.ai/api/gateway` base and treats
the alias as an implementation detail not to be depended on.

**Auth and environment variable.** `Authorization: Bearer $KILO_API_KEY`; the API
reference states the header format verbatim as `"Authorization: Bearer $KILO_API_KEY"`.
The registry `models.dev/api.json` corroborates independently:

```
kilo  name="Kilo Gateway"  api=https://api.kilo.ai/api/gateway  env=['KILO_API_KEY']  #models=364
```

**Endpoints.** The API reference documents:

| Endpoint | Auth | Note |
|---|---|---|
| `POST /chat/completions` | required (paid models) | primary surface |
| `GET /models` | **none** | catalog |
| `GET /providers` | **none** | 23,618 bytes of partner metadata incl. `dataPolicy{training,retainsPrompts,canPublish}` |
| `POST /api/fim/completions` | required | fill-in-the-middle, Mistral models only |

Documented error codes are `400`, `401`, **`402` insufficient balance**, `403`, `429`,
`500`, `502` provider error, `503`. The `402` is new to this package — see Consequences.

**Kilo translates wire formats; it does not route by model.** `POST /responses` and
`POST /messages` are *undocumented* but live on the official host, and — decisively — the
**same model** succeeds on all three:

| Route | Request for `kilo-auto/free` | Result |
|---|---|---|
| `/chat/completions` | OpenAI chat body | `200`, `chat.completion` envelope, `content:"ALPHA"` |
| `/responses` | OpenAI Responses body | `200`, `output:[{type:"reasoning"},{type:"message"}]`, text `"ALPHA"` |
| `/messages` | Anthropic body | `200`, `content:[{type:"thinking"},{type:"text"}]` |

This is the **opposite** of OpenCode, where a model sent to the wrong route returns HTTP
500. Kilo is a format-translating gateway: the client picks the shape it likes and Kilo
adapts. **No route table is needed, and none should be built.**

**Free models generate without any credential.** `POST /chat/completions` with **no**
`Authorization` header and `model: "kilo-auto/free"` returned a real completion (routed
internally to `stepfun/step-3.7-flash`). Forced tool calling also works unauthenticated:

```json
"tool_calls":[{"id":"chatcmpl-tool-8c37b71980cc930c","type":"function",
  "function":{"name":"get_weather","arguments":"{\"city\": \"Paris\"}"}}]
```

`reasoning_effort: "low"` was likewise accepted (`200`). Twenty catalog entries carry
`isFree: true`; twelve of those are text-in/text-out **and** tool-capable. Kilo therefore
supports the same credential-free end-to-end testing OpenCode does — and, unlike OpenCode's
free tier, tool calls work.

**Errors are a structured, Kilo-specific envelope.** A paid or unknown model without a
valid key returns HTTP `401` with:

```json
{"error":{"code":"PAID_MODEL_AUTH_REQUIRED","message":"You need to sign in to use this model."},
 "error_type":"paid_model_auth_required"}
```

Note an unknown model ID is treated as a *paid* model, not as "model not found", so — as
with Hugging Face and unlike OpenCode — there is **no credential-free model-validation
oracle**.

**The catalog is the richest of the three gateways.** `GET /models` returns `200` with no
credential: 366 models in OpenRouter shape (`id`, `name`, `architecture`, `top_provider`,
`pricing`, `context_length`) plus four Kilo extensions — `supported_parameters[]`,
`isFree`, `mayTrainOnYourPrompts`, `preferredIndex`, and an `opencode{ai_sdk_provider,
family, prompt}` hint block. Census on 2026-08-29:

| Metric | Value |
|---|---|
| Models listed | 366 |
| Text-in / text-out | 145 |
| …of which tool-capable | 106 |
| `isFree: true` | 20 (12 text+tools) |
| `mayTrainOnYourPrompts: true` | 25 |
| `supported_parameters` ⊇ `tools` | 301 |
| … ⊇ `tool_choice` | 279 |
| … ⊇ `reasoning` | 244 |
| … ⊇ `reasoning_effort` | 105 |
| … ⊇ `response_format` | 294 |

`supported_parameters` is a **per-model list of accepted request parameters**, which is
strictly more informative than Hugging Face's boolean `supports_tools`: it says whether
`tool_choice` forcing, `reasoning`, `reasoning_effort` and `response_format` are each
accepted. Capability gating can therefore be exact rather than inferred.

**`kilo-auto/*` are Kilo-managed routing tiers**, not upstream models:
`kilo-auto/frontier`, `/balanced`, `/efficient`, `/small` and `/free`. All five are
tool-capable; `frontier`/`balanced`/`efficient` advertise a 1,000,000-token context and
variable pricing (`"prompt":"-1"`). Because Kilo maintains what they point at, they are the
most churn-resistant IDs in an otherwise open catalog.

**Reasoning field naming differs from OpenCode — this is the one substantive amendment
Kilo forces.** On `/chat/completions`, Kilo returns the OpenRouter convention:

```json
"message":{"role":"assistant","content":"ALPHA",
  "reasoning":"Got it, the user said to say ALPHA only…",
  "reasoning_details":[{"type":"reasoning.text","text":"…"}]}
```

That is `message.reasoning` (a plain string), **not** OpenCode's
`message.reasoning_content`. The shared decoder must accept both names.

**Kilo's `/responses` reasoning is in a non-standard place, which argues against using that
route.** Its reasoning item carries the text in `content[]` with
`type: "reasoning_text"` and leaves `summary: []`:

```json
{"id":"rs_tmp_…","type":"reasoning","status":"completed",
 "content":[{"type":"reasoning_text","text":"We need to respond with only the word ALPHA…"}],
 "summary":[],"format":"unknown"}
```

The package's `decodeResponsesAPIOutput` reads reasoning **only** from `summary[]`
(`http_helpers.go:64-71`), so this decodes to `ReasoningItem{Text: ""}` — the trace is
silently dropped. Fixing that would mean editing a decoder shared by OpenAI, Grok and
OpenCode's Responses route, widening the blast radius for a route Kilo does not document.

### Why delegation to the existing provider structs cannot work (OpenCode)

Revision 1's mechanism was to have the new provider construct an `OpenAIProvider`,
`ClaudeProvider`, or `GeminiProvider` with `WithBaseURL(gatewayBase)` and call it. The
auth headers of those structs are **hardcoded literals**, not configuration:

| Provider | Header set | Source |
|---|---|---|
| `OpenAIProvider` | `Authorization: Bearer …` | `openai.go:164` |
| `GrokProvider` | `Authorization: Bearer …` | `grok.go:183` |
| `ClaudeProvider` | `x-api-key: …` + `anthropic-version: 2023-06-01` | `claude.go:212-213` |
| `GeminiProvider` | `x-goog-api-key: …` | `gemini.go:211` |

The gateway accepts `Authorization: Bearer` and **rejects `x-api-key`** (measured above).
So delegating the Messages route to `ClaudeProvider` or the Google route to
`GeminiProvider` produces a guaranteed `401` on every call. There is no option, field, or
seam that changes those headers. Delegation is only viable for the Responses route, which
is the one route that needs no new code anyway.

Making it viable would require editing `claude.go` and `gemini.go` to accept an injectable
auth header — a change to two tested, shipping providers in service of a third. That is
option 3 below, and it is rejected.

### Corrections to revision 1

| # | Revision 1 claim | Verified fact |
|---|---|---|
| 1 | ~~Claude / Qwen / **MiniMax** all take the Anthropic `/messages` route~~ | MiniMax takes `/chat/completions` on **Zen** and `/messages` on **Go** |
| 2 | ~~`opencodeRoute(model string) routeKind`~~ | Must be keyed on **(gateway, model)** — routes diverge per gateway |
| 3 | ~~`ProviderEnvVars` documents `OPENCODE_API_KEY`, alias `OPENCODE_ZEN_API_KEY`~~ | `OPENCODE_API_KEY` only; the alias appears in no primary source and is struck |
| 4 | ~~`GET /v1/models` returns `{data:[{id, owned_by}]}` inventory~~ (implying usable metadata) | True in shape, but `owned_by` is constant `"opencode"`; the listing carries **no** route or capability data, and needs **no** API key |
| 5 | ~~Delegate to `ClaudeProvider` / `GeminiProvider`~~ | Impossible — hardcoded `x-api-key` / `x-goog-api-key` (see above) |
| 6 | ~~`Continuer` via `previous_response_id` where applicable~~ | Measured **400 "referenced response not found or expired"**; not implemented |
| 7 | ~~Zen/Go "do not document `Retry-After`"~~ | Measured: `429` responses carry no `Retry-After` header at all |
| 8 | ~~`StaticOpencodeZen` includes `qwen3.7-max`~~ | `qwen3.7-max` is absent from the live Zen listing; catalogs must be seeded from the live listing, not the docs table |
| 9 | ~~Unknown models default safely to `routeChatCompletions`~~ | A wrong route returns **500**, which maps to the *retryable* `ErrProviderUnavailable` — the failure is silent and expensive (see Consequences) |

## Decision Drivers

* **Reuse the logic where correctness risk actually lives.** The valuable, already-tested
  assets are the **response decoders** — `decodeResponsesAPIOutput` (`http_helpers.go:23-76`),
  `decodeClaudeResponse` (`claude.go:239-293`), `decodeGeminiResponse` (`gemini.go:239-291`)
  — and the status→sentinel classification switch. All three decoders are package-level
  functions, so a new file in the *same package* can call them directly. Request-body
  construction, by contrast, is short, per-route, and diverges from the native providers
  anyway (no `anthropic-version`, no `x-goog-api-key`).
* **Do not modify shipping providers to enable a new one.** `claude.go` and `gemini.go`
  are covered by `claude_items_test.go`, `gemini_items_test.go`, `thinking_test.go`, and
  `provider_correctness_test.go`. Widening their auth surface for a gateway they never
  talk to is regression risk paid by unrelated callers.
* **SDK-free constraint is non-negotiable** — `provider.go:1-3` states the package is
  dependency-free by design.
* **Full catalog coverage without per-model caller configuration** — callers pass a model
  ID and the provider selects the route, matching how the gateway itself dispatches.
* **Route-table drift must be survivable.** The table is docs-coupled, the docs disagree
  with the live listing, and a miss costs a retried 500. The design needs a caller-side
  escape hatch, not just a default.
* **Registration pattern continuity** — the five touch points above, preserving
  `ProviderEnvVars`, `NewProvider`, `ListAvailableModels`, `StaticModels`, and `Static*`
  conventions.
* **Testability with `httptest`** — existing tests are white-box `net/http/httptest`
  servers; `WithBaseURL` injection must let one test server observe route selection.
* **Chat Completions is now shared three ways, so it must be owned by the package, not by
  one provider.** It is one of OpenCode's four routes, the whole of Hugging Face's surface,
  and the whole of Kilo's documented surface. Whichever provider lands first, the converter
  and decoder belong in a provider-neutral file with provider-neutral names; naming them
  `opencode*` would guarantee a rename refactor when the second arrives, and a second one
  when the third does.
* **Prefer published metadata over string heuristics when a provider supplies it.** The
  package's `Rank*` functions grep model names because the vendors give nothing better.
  Hugging Face publishes measured throughput, first-token latency, tool support and
  pricing; Kilo publishes a per-model list of accepted request parameters and a
  training-on-prompts flag. Ignoring that in favour of another name-grep would be choosing
  a worse signal when a better one is free.
* **Never let a transport-level failure decode as success.** Hugging Face's
  `/v1/responses` returns HTTP 200 with `status:"failed"` on auth failure. Any endpoint
  adopted must fail loudly under status-only classification, or be rejected.

## Considered Options

**Mechanism (applies to all three providers):**

1. **Self-contained providers reusing the package's response decoders** (chosen)
2. Delegating provider that constructs and calls the existing provider structs (revision 1's choice)
3. Make the auth header injectable across all providers, then delegate
4. Thin OpenAI-compatible wrapper around `OpenAIProvider` with `BaseURL` override only
5. Listing-only integration (Ollama precedent)
6. Vendor SDK-based provider

**Hugging Face endpoint surface:**

7. **`/v1/chat/completions` only** (chosen)
8. `/v1/responses` only
9. Both, selected per model

**Hugging Face discovery:**

10. **Metadata-driven curation from `GET /v1/models`, static catalog as fallback** (chosen)
11. Static catalog primary, listing used only to confirm availability (the OpenCode approach)

**Kilo Gateway endpoint surface:**

12. **`/chat/completions` only** (chosen)
13. `/responses`, for the canonical item envelope
14. Multi-route like OpenCode, exposing all three formats via an option

## Decision Outcome

Chosen options: **1** (self-contained providers reusing the package's response decoders),
**7** (Hugging Face on `/v1/chat/completions` only), **10** (metadata-driven Hugging Face
discovery), and **12** (Kilo on `/chat/completions` only).

Option 1 delivers full Zen/Go catalog coverage, reuses precisely the logic that carries
correctness risk (decoding and error classification), and is the only mechanism that
neither breaks on the gateways' auth requirements nor modifies shipping providers. Options
7 and 10 follow from measured Hugging Face behaviour: its Responses endpoint is
undocumented and returns HTTP 200 on failure, while its listing publishes the very metrics
the package's rankers try to approximate. Option 12 follows from Kilo's: `/chat/completions`
is its only documented route, and its `/responses` route puts reasoning text where the
package's shared Responses decoder does not look.

**Four new provider identifiers**, all kebab-case per `constants.go:3-9`:

| Constant | Value | Env var | Base URL | Wire format used |
|---|---|---|---|---|
| `ProviderOpencodeZen` | `opencode-zen` | `OPENCODE_API_KEY` | `https://opencode.ai/zen/v1` | four, per (gateway, model) |
| `ProviderOpencodeGo` | `opencode-go` | `OPENCODE_API_KEY` | `https://opencode.ai/zen/go/v1` | four, per (gateway, model) |
| `ProviderHuggingFace` | `huggingface` | `HF_TOKEN` | `https://router.huggingface.co/v1` | Chat Completions |
| `ProviderKilo` | `kilo` | `KILO_API_KEY` | `https://api.kilo.ai/api/gateway` | Chat Completions |

> **Naming note (decided 2026-08-29).** The agent product is "Kilo Code"; the API is
> "Kilo Gateway", and `models.dev` registers it under the key `kilo`. The identifier is
> **`kilo`**, for registry parity — matching the choice already made for `huggingface`,
> whose value likewise follows `models.dev`. All derived names follow the identifier
> rather than the agent's name: `ProviderKilo`, `KiloProvider`, `NewKilo`, `kilo.go`,
> `StaticKilo`, `isUsableKiloModel`, `RankKiloModel`. The package uses one canonical name
> per provider with no aliases (`ProviderEnvVars` has a single value per key), so no
> `kilocode` alias is accepted. Recorded here because it is the kind of decision that is
> expensive to revisit after release.

### Shared: the Chat Completions primitive

Chat Completions is implemented **once**, in a provider-neutral file
(`llmprovider/chatcompletions.go`), with provider-neutral names — **not** the
`opencode`-prefixed names revision 2 proposed:

| Revision 2 name | Revision 3 name |
|---|---|
| ~~`opencodeItemsToChatMessages`~~ | `itemsToChatMessages` |
| ~~`decodeChatCompletionsResponse`~~ (in `opencode_chat.go`) | `decodeChatCompletionsResponse` (in `chatcompletions.go`) |

**Three** consumers share it: OpenCode's `OpencodeRouteChatCompletions`, Hugging Face's only
route, and Kilo's only route. The body builder is parameterised over the fields the
gateways differ on — Hugging Face and Kilo both send `reasoning_effort`, OpenCode does not
— so no provider carries a copy.

`decodeChatCompletionsResponse` maps `choices[0].message.content` → `MessageItem` and
`.tool_calls[]` → `FunctionCallItem`. For reasoning it must accept **both** vendor
spellings, because the two conventions were measured on different gateways:

| Field | Emitted by | Shape |
|---|---|---|
| `message.reasoning_content` | OpenCode (`big-pickle`) | string |
| `message.reasoning` | Kilo / OpenRouter convention | string |
| `message.reasoning_details[]` | Kilo | `[{type:"reasoning.text", text}]` |

The decoder reads `reasoning_content`, falling back to `reasoning`, and emits at most one
`ReasoningItem`. `reasoning_details[]` is **not** decoded: it is a structured restatement of
the same trace, and the canonical `ReasoningItem` carries a single `Text` field
(`item.go:44-51`), so parsing it would add a second source of truth for one value. Neither
field is documented by any of the three vendors, so both are treated as best-effort: absent
reasoning is normal, never an error.

### OpenCode Zen / Go

* **Two canonical identifiers, one implementation type.**
  `ProviderOpencodeZen = "opencode-zen"` and `ProviderOpencodeGo = "opencode-go"`
  (kebab-case, matching `constants.go:3-9`). `ProviderEnvVars` maps **both** to
  `"OPENCODE_API_KEY"` — the only evidenced name. `NewProvider` dispatches both to
  `NewOpencode(gateway, apiKey, model, opts...)` with the gateway-appropriate default base
  URL (`https://opencode.ai/zen/v1` / `https://opencode.ai/zen/go/v1`), overridable via
  `WithBaseURL` for tests.

* **One struct `OpencodeProvider` in `llmprovider/opencode.go`** holding `gateway`,
  `apiKey`, `model`, `baseURL`, `client`, `maxTokens`, `reasoningEffort`, `thinkingBudget`,
  and a resolved `route`. Every request sets `Authorization: Bearer <apiKey>` and
  `Content-Type: application/json` — one auth path for all four routes.

* **Route resolution keyed on (gateway, model).** A package-level
  `opencodeRoutes = map[string]map[string]opencodeRoute` seeded from the two docs tables,
  consulted as `opencodeRoutes[gateway][model]`. On a miss, a small ordered prefix
  heuristic applies *per gateway* (e.g. on Zen: `gpt-`/`grok-`/`muse-` → Responses,
  `claude-`/`qwen` → Messages, `gemini-` → Google, else Chat Completions; on Go the
  `minimax-` prefix maps to Messages, not Chat Completions). A new
  `WithOpencodeRoute(route)` option lets a caller override both, so a model added upstream
  before the table is updated is usable immediately rather than failing with an opaque 500.

* **Per-route request builders, shared decoders.** Each route builds its own body and
  reuses an existing decoder:

  | Route | Path | Body shape | Decoder |
  |---|---|---|---|
  | Responses | `{base}/responses` | `{model, input[], max_output_tokens, tools?, tool_choice?, reasoning?}` — same as `openai.go:120-152` | `decodeResponsesAPIOutput` (`http_helpers.go:23`) |
  | Messages | `{base}/messages` | `{model, messages[], max_tokens, tools?, tool_choice?, thinking?}` — same as `claude.go` | `decodeClaudeResponse` (`claude.go:239`) |
  | Google | `{base}/models/<id>:generateContent` | `{contents[], generationConfig, tools?}` — same as `gemini.go` | `decodeGeminiResponse` (`gemini.go:239`) |
  | Chat Completions | `{base}/chat/completions` | `{model, messages[], max_tokens, tools?, tool_choice?}` | `decodeChatCompletionsResponse` (**shared primitive**) |

  Only the last row is net-new wire logic, as revision 1 correctly anticipated — and as of
  revision 3 it is written once in `chatcompletions.go` and shared with Hugging Face rather
  than owned by OpenCode. OpenCode omits `reasoning_effort` on this route (it has no
  portable reasoning parameter across the DeepSeek/GLM/Kimi/MiniMax families routed here);
  Hugging Face sets it. That single field is the only divergence between the two callers.

* **Shared error classification.** The identical status→sentinel switch appears four times
  today (`openai.go:174-185`, `claude.go:223-234`, `gemini.go:223-234`, `grok.go:195-206`).
  Extract it once as `classifyHTTPStatus(provider string, resp *http.Response) error` in
  `http_helpers.go` and use it from all four `opencode.go` routes. Existing providers are
  **left untouched** in this change; adopting the helper there is a separate, optional
  follow-up.

* **Capabilities are route-dependent, and the type reflects that honestly.**
  `OpencodeProvider` implements `Provider`, `ToolProvider`, `ThinkingProvider`,
  `ThinkingToolProvider`, `ItemProvider`, `ItemToolProvider`, `ItemThinkingProvider`,
  `ItemThinkingToolProvider`, and `ModelDiscoverer`. It **does not** implement `Continuer`
  — chaining was measured to fail. Thinking is expressed per route: `reasoning.effort` on
  Responses, `thinking.budget_tokens` on Messages, `generationConfig.thinkingConfig` on
  Google, and — since Chat Completions has no portable reasoning parameter — a no-op that
  returns the plain generation on that route, documented as such.

* **Catalogs seeded from the live listing, not the docs table.**
  `StaticOpencodeZen` and `StaticOpencodeGo`, each `<= MaxListedModels` (= **6**,
  `models_catalog.go:11`), chosen from IDs confirmed present in
  `GET /zen/v1/models` / `GET /zen/go/v1/models` on 2026-08-28 and ranked for the
  low-latency git-hook use case that `RankGeminiModel`/`RankOpenAIModel` already optimise
  for. `StaticModels` and `ListAvailableModels` each gain two cases;
  `listOpencodeModels(ctx, gateway, cfg)` performs the `GET {base}/models` and feeds
  `curateFromCatalog` with `isUsableOpencodeModel` and `RankOpencodeModel`.
  `listOpencodeModels` sends the Bearer header when a key is present but **must not
  require one**, since the endpoint is public.

### Hugging Face Inference Providers

* **One identifier, one route, no route table.** `ProviderHuggingFace = "huggingface"`
  maps to `HF_TOKEN` in `ProviderEnvVars`; `NewProvider` dispatches to
  `NewHuggingFace(apiKey, model, opts...)` with default base URL
  `https://router.huggingface.co/v1`, overridable via `WithBaseURL`. A struct
  `HuggingFaceProvider` in `llmprovider/huggingface.go` holds `apiKey`, `model`,
  `baseURL`, `client`, `maxTokens`, `reasoningEffort`. Every request sets
  `Authorization: Bearer <token>` and posts to `{baseURL}/chat/completions` — no routing
  logic exists, and none is needed.

* **`/v1/responses` is deliberately not used.** It is undocumented, contradicted by the
  official index page ("available for chat completion tasks only"), and returns **HTTP 200
  with `status:"failed"`** on auth failure — which this package's status-only
  classification would decode as an empty, error-free success. A code comment records the
  measurement and the reason, so a future contributor does not "fix" the omission.

* **Body and decoding come from the shared primitive.** The request is
  `{model, messages[], max_tokens, tools?, tool_choice?, reasoning_effort?}` built by the
  shared builder; the response is parsed by `decodeChatCompletionsResponse`. Hugging Face
  contributes exactly one field the OpenCode chat route does not use:
  `reasoning_effort`.

* **`ThinkingProvider` maps to `reasoning_effort`, a documented parameter.** The value
  comes from `WithReasoningEffort` and defaults to `effortMedium`, reusing the package's
  existing constants (`constants.go:34-39`), whose value set the HF docs list verbatim
  (`none, minimal, low, medium, high, xhigh`). Because HF documents support as "provider
  and model-dependent", an ignored parameter is a normal outcome, not an error — the
  thinking path degrades to a plain generation rather than failing.

* **Tool calling uses the documented forced form.** `tools[]` carries
  `{type:"function", function:{name, description, parameters}}` and `tool_choice` is
  `{type:"function", function:{name}}`, both straight from the task reference. Only 220 of
  317 provider offerings advertise `supports_tools`, so model selection for tool use is
  gated on that flag from the listing (below) rather than assumed.

* **Discovery is metadata-driven — the one place this provider diverges from every other
  in the package.** `listHuggingFaceModels` fetches `GET {base}/models` (no credential
  required, though the Bearer header is sent when available), then:

  1. **Filters** to `architecture.output_modalities == ["text"]` **and**
     `input_modalities == ["text"]` — dropping the 40 vision-language models — keeping only
     models with at least one `providers[]` entry whose `status == "live"`.
  2. **Sorts** the survivors by measured `throughput` descending, tie-broken by
     `first_token_latency_ms` ascending — the metrics `RankGeminiModel` and
     `RankGrokModel` approximate with name-grepping, here as published figures.
  3. **Curates** via the existing `curateFromCatalog(StaticHuggingFace, sorted, isUsableHuggingFaceModel, nil)`.
     Passing `nil` for `rankFn` is deliberate and load-bearing: `curateFromCatalog` sorts
     the backfill only `if rankFn != nil` (`models_catalog.go:224-226`), so a nil ranker
     **preserves the metadata-derived order**. No change to `curateFromCatalog` is required.

  `RankHuggingFaceModel(string) int` is still exported for convention parity with the other
  four `Rank*` functions, but serves only as a weak name-based fallback for the path where
  the listing is unavailable and the static catalog is used.

* **Static catalog is a fallback, chosen for provider redundancy rather than taste.**
  `StaticHuggingFace` holds 6 IDs (`MaxListedModels`, `models_catalog.go:11`), every one
  confirmed present in `GET /v1/models` on 2026-08-29, text→text, tool-capable, and served
  by **at least four** partner providers — redundancy being the best available proxy for
  "will still exist next month" in an open catalog that churns:

  | ID | Providers | Best tok/s | $/M out |
  |---|---|---|---|
  | `openai/gpt-oss-20b` | 7 | 763.4 | 0.50 |
  | `openai/gpt-oss-120b` | 11 | 1105.6 | 0.75 |
  | `meta-llama/Llama-3.1-8B-Instruct` | 4 | 149.6 | 0.06 |
  | `zai-org/GLM-5.3-Flash` | 5 | 144.0 | 0.50 |
  | `deepseek-ai/DeepSeek-V4-Flash-0731` | 5 | 76.2 | — |
  | `zai-org/GLM-5.2` | 8 | 115.5 | 4.40 |

* **Model-ID policy suffixes pass through, but are stripped for matching.** A caller may
  write `openai/gpt-oss-120b:cheapest` or `…:groq`; the string is sent to the router
  verbatim, since selection is server-side. A helper splits at the **last** colon — model
  IDs contain `/` but the policy is always the final `:`-delimited segment — so that
  catalog matching and `isUsableHuggingFaceModel` see the bare `<org>/<name>`.

* **Capabilities.** `HuggingFaceProvider` implements `Provider`, `ToolProvider`,
  `ThinkingProvider`, `ThinkingToolProvider`, `ItemProvider`, `ItemToolProvider`,
  `ItemThinkingProvider`, `ItemThinkingToolProvider`, and `ModelDiscoverer`. It does
  **not** implement `Continuer`: Chat Completions is stateless by construction, the same
  reason `ClaudeProvider` omits it (`claude.go:12-17`).

### Kilo Gateway

* **One identifier, one route, no route table — despite three routes being available.**
  `ProviderKilo = "kilo"` maps to `KILO_API_KEY`; `NewProvider` dispatches to
  `NewKilo(apiKey, model, opts...)` with default base URL
  `https://api.kilo.ai/api/gateway`, overridable via `WithBaseURL`. A struct
  `KiloProvider` in `llmprovider/kilo.go` holds `apiKey`, `model`, `baseURL`,
  `client`, `maxTokens`, `reasoningEffort`. Every request sets `Authorization: Bearer` and
  posts to `{baseURL}/chat/completions`.

  Kilo *does* answer `/responses` and `/messages` with the same model, so a multi-route
  design is technically possible here in a way it is not for Hugging Face. It is declined:
  see option 14. The gateway translates formats, so a second route buys no model coverage
  — only a second code path to test.

* **Body and decoding come from the shared primitive**, making Kilo the third consumer.
  Kilo contributes the `message.reasoning` spelling the decoder must now tolerate; it sends
  `reasoning_effort` like Hugging Face.

* **Capability gating uses `supported_parameters`, the most precise signal available.**
  Kilo publishes a per-model list of accepted request parameters, so:
  * `tools` ∈ `supported_parameters` gates `GenerateWithTool` (301 of 366 models);
  * `tool_choice` ∈ list gates *forced* tool choice — 279 models — with a documented
    fallback to unforced `tools` for the 22 that accept `tools` but not `tool_choice`;
  * `reasoning_effort` ∈ list (105 models) gates sending that parameter on the thinking
    path; when absent the request omits it rather than risking a `400`.

  This is strictly better than Hugging Face's boolean `supports_tools` and than the
  string-heuristic gating every existing provider uses. It is **not** automatic, though:
  the catalog supplies the data, but a provider instance receives it only through an
  explicit `WithKiloCapabilities(...)` option. Populating it implicitly would mean either
  a hidden catalog fetch on the generation hot path or a lister mutating a provider it does
  not own; both were rejected. Omitting the option sends every parameter, which is what
  every other provider in this package already does. Whether gating is needed at all is
  open question 12.

* **Discovery mirrors the Hugging Face shape.** `listKiloModels` fetches
  `GET {base}/models` (no credential required; Bearer sent when available), filters to
  `architecture.output_modalities == ["text"]` and `input_modalities == ["text"]` (145 of
  366) with `tools` in `supported_parameters` (106), sorts by ascending
  `pricing.completion` — Kilo publishes price, not throughput, so cost is the available
  objective signal — and curates with
  `curateFromCatalog(StaticKilo, sorted, isUsableKiloModel, nil)`. The `nil` ranker
  again preserves the metadata-derived order (`models_catalog.go:224-226`).
  `pricing.completion` is a **string**, and is `"-1"` for the variable-priced `kilo-auto`
  tiers, so parsing must treat non-positive values as "unranked" and keep them in catalog
  order rather than sorting them to the front as if free.

* **`mayTrainOnYourPrompts` is surfaced, not silently ignored.** Twenty-five models declare
  it. `isUsableKiloModel` **excludes** them by default: a shared library used by commit
  hooks should not route a private diff to a model that trains on it without the caller
  saying so. This is a deliberate, documented policy choice, not a capability filter, and
  is the one place this package makes a judgement on the user's behalf — recorded here so
  it is reviewable.

* **Static catalog leans on the `kilo-auto/*` tiers.** `StaticKilo` holds 6 IDs
  (`MaxListedModels`), every one confirmed in `GET /models` on 2026-08-29. Five are
  Kilo-managed aliases, chosen because Kilo maintains what they point at — the strongest
  churn resistance available in a 366-model open catalog:

  | ID | Free | Tools | Context | Note |
  |---|---|---|---|---|
  | `kilo-auto/free` | yes | yes | 256,000 | zero-cost default; also the live-test target |
  | `kilo-auto/small` | no | yes | 262,144 | cheapest managed tier |
  | `kilo-auto/efficient` | no | yes | 1,000,000 | cost-optimised |
  | `kilo-auto/balanced` | no | yes | 1,000,000 | default quality tier |
  | `meta-llama/llama-3.1-8b-instruct` | no | yes | 131,072 | cheapest concrete model, $0.04/M out |
  | `nvidia/nemotron-3.5-lightning:free` | yes | yes | 1,000,000 | free fallback with `reasoning_effort` |

* **Capabilities.** `KiloProvider` implements the same nine interfaces as
  `HuggingFaceProvider` and, like it, does **not** implement `Continuer` — Chat Completions
  is stateless.

* **The host alias is not used.** `https://kilo.ai/api/openrouter` serves a byte-identical
  catalog but is undocumented and exists for the editor extension; the provider uses the
  documented `https://api.kilo.ai/api/gateway` only. A code comment records the alias so a
  future reader who encounters it in Kilo's own configs is not surprised.

### Pinning what was probed

Every decision in this MADR rests on wire shapes measured on **2026-08-28/29**: OpenCode's
63- and 26-row route tables, four static catalogs, the `reasoning_content` / `reasoning`
field split, Kilo's `supported_parameters` semantics, and Hugging Face's `throughput` /
`first_token_latency_ms` / `supports_tools` metadata. **None of it is version-negotiated.**
These are remote, continuously-deployed gateways: they can change shape under a running
binary with no signal to the client.

**A version pin is not available.** The sibling repo `magic-cli-remote` solves this for
*local* engines it spawns — `internal/provider/kilo/version.go` reads the engine version
from `GET /global/health` and compares it to a `KnownGoodVersion` constant. That works
because a local binary reports a version. The Kilo **gateway** does not: `/version`,
`/api/version` and `/health` all return `404`, and `/api/gateway/version` returns `405`
(probed 2026-08-29). The same is true of the OpenCode and Hugging Face gateways.

> **These are two different Kilo integrations and must not be conflated.**
> `magic-cli-remote` drives the local `kilo` CLI binary (`/opt/homebrew/bin/kilo`, v7.5.6
> at time of writing) as an HTTP subprocess engine. This MADR talks to the remote gateway
> at `api.kilo.ai/api/gateway`. `grep -rn 'api\.kilo\.ai' magic-cli-remote` returns
> **nothing** — there is no shared endpoint, no shared code, and no shared version. What
> transfers is the *pattern*, not the pin.

The pattern is well established in this codebase family, and already applied to **both**
vendors this MADR integrates:

| Repo / package | Pin | Policy |
|---|---|---|
| `magic-cli-remote/internal/provider/kilo` | `KnownGoodVersion = "7.4.23"` | warn per boot, never refuse |
| `magic-cli-remote/internal/provider/opencode` | `MinVersion = "1.18.0"` **and** `KnownGoodVersion = "1.18.21"` | hard floor **plus** a separate "what was verified" statement |

`opencode/version.go` states the distinction in terms worth reusing: `MinVersion` is a hard
floor below which the integration cannot work; `KnownGoodVersion` "is a statement about what
has been verified", and drifting off it "produces one warning per engine boot rather than an
outage". Build-tagged live suites (`live_kilo`, `live_opencode`, `live_grok`, `live_codex`,
`live_goose`) exist to re-validate after a drift.

**What this MADR adopts.** Since there is no version to pin, the pin is the **probe date**,
and the enforcement is **shape assertion** rather than version comparison:

* Each gateway file carries a `wireShapesProbedOn` constant (`"2026-08-28"` for the two
  OpenCode gateways, `"2026-08-29"` for Hugging Face and Kilo) with a doc comment naming
  what was measured and pointing at the live suite that re-validates it. It is
  documentation with a compiler-checked home, not runtime logic — there is nothing to
  compare it against at runtime.
* **The live tests become shape assertions, not liveness checks.** As specified in revision
  4 they asserted only "non-empty `OutputText()`", which would still pass if Hugging Face
  renamed `throughput`, Kilo renamed `reasoning`, or OpenCode moved a model between routes —
  the code would silently degrade to unsorted catalogs, dropped reasoning traces, or a
  retried 500. The suite must instead assert the specific fields each decision depends on.
* No runtime warning and no refusal. Unlike a local engine, there is no boot event to hang a
  warning off, and failing a request because a catalog field moved would turn a cosmetic
  drift into an outage.

**Build-tag naming.** Revision 4 specified a single `gateway_live` tag. The family
convention is `live_<provider>`. This MADR keeps **one** tag rather than four — the suites
are small and share fixtures — but renames it `live_gateways` to read as part of the family.
It is deliberately **not** `live_kilo`: that tag already means the local CLI engine in
`magic-cli-remote`, and reusing it across repos for a different surface is exactly the
conflation this section exists to prevent.

### Consequences

* Good, because the full Zen and Go catalogs are reachable through one `llmprovider`
  surface without callers knowing per-model gateway paths.
* Good, because the three battle-tested decoders and the 1 MB-limit / status-classification
  discipline are reused verbatim, and the only new decoder is the simplest of the four.
* Good, because no existing provider file is modified, so the existing test suite is a
  genuine regression net rather than something that had to be adjusted to accommodate this
  work.
* Good, because the change fits the five registration touch points exactly — no registry,
  no new dependencies, `go.mod` unchanged.
* Good, because free models make real end-to-end verification possible without
  credentials, behind a build tag.
* Neutral, because two provider names share one implementation type; justified by identical
  auth and logic differing only in default base URL and route table.
* **Bad, because a route-table miss fails as HTTP 500 → `ErrProviderUnavailable`, which
  `GenerateWithRetry` treats as retryable.** A misrouted model therefore burns the full
  retry budget and reports "provider unavailable" for what is really a table bug. Mitigated
  three ways: `WithOpencodeRoute` as a caller escape hatch; a table-driven unit test pinned
  to the published tables; and an explicit decision **not** to auto-fall-back to another
  route on 500, since silently changing wire format would convert a loud failure into a
  wrong answer.
* Bad, because the route table is docs-coupled and both gateways add models frequently;
  the table needs periodic reconciliation against the two docs pages.
* Bad, because `Continuer` is unavailable, so multi-turn callers must resend history. This
  matches `ClaudeProvider`, which is already stateless (`interface_test.go:56-59`), so no
  caller contract is broken.
* Bad, because tool-calling and reasoning behaviour on the Messages, Google, and Chat
  Completions routes could not be verified end-to-end — the free models that permit
  unauthenticated calls return `"Endpoint is unavailable"` for tool requests. These remain
  open questions (below) to be closed during implementation against a funded key.
* Good, because Chat Completions is written once and used twice; adding Hugging Face costs
  one struct, one lister, and one catalog rather than a second copy of the wire format.
* Good, because Hugging Face needs **no** route table, no `With*Route` escape hatch, and no
  misroute failure mode — its entire surface is one endpoint, so the largest maintenance
  liability in the OpenCode design simply does not exist for it.
* Good, because Hugging Face discovery degrades gracefully in three stages: live metadata →
  static catalog → error, and the first stage needs no credential.
* Good, because Hugging Face's `reasoning_effort` gives the `ThinkingProvider` path a
  documented parameter with the exact value set the package already models, so no new
  effort vocabulary is introduced.
* Neutral, because Hugging Face model IDs are `<org>/<name>[:policy]` rather than the bare
  slugs every other provider uses. Nothing in the package parses model IDs structurally, so
  this only affects the catalog-matching helper.
* **Bad, because Hugging Face admits no unauthenticated live test.** `is_free` is false for
  all 317 offerings, so unlike OpenCode there is no zero-credential end-to-end check; the
  Hugging Face live test must skip unless `HF_TOKEN` is set, and CI gains nothing from it.
* Bad, because Hugging Face's catalog is open and churns far faster than OpenCode's curated
  list — 136 models today, with partners adding and retiring continuously. Mitigated by
  making the static catalog a fallback rather than the source of truth, and by choosing its
  six entries on provider redundancy (≥4 partners each) rather than on preference.
* Good, because the probe date is recorded in code beside the logic it justifies, so a
  future reader can see *when* a route table or catalog was last true without archaeology.
* Good, because the live suite now fails on the drift it is meant to catch. Asserting
  liveness alone would have let a renamed metadata field pass while catalogs silently
  stopped being ranked.
* **Bad, because a date pin is strictly weaker than a version pin.** `magic-cli-remote` can
  warn on every engine boot; this package can only fail a test someone chooses to run.
  There is no runtime signal available — the gateways expose no version — so drift is
  detected on demand, not automatically. Running `-tags live_gateways` before a release is
  the only mitigation, and it is a process control, not a technical one.
* Bad, because the shape assertions couple the test suite to vendor field names that are
  undocumented on all three gateways (`reasoning_content`, `reasoning`, `throughput`,
  `supported_parameters`). That coupling is the point — it is what turns a silent
  degradation into a red test — but it will produce failures that are drift reports rather
  than bugs, and they must be read that way.
* Bad, because a live `/v1/responses` endpoint exists that this design refuses to use. A
  future contributor may reasonably wonder why; mitigated by recording the HTTP-200-on-
  failure measurement in a code comment at the point of the decision, not only here.
* Good, because Kilo makes the shared Chat Completions primitive pay for itself three times
  over: the fourth provider costs one struct, one lister and one catalog, with **zero** new
  wire logic. The marginal cost of the next OpenAI-compatible gateway is now near-flat.
* Good, because Kilo restores credential-free end-to-end testing that Hugging Face could not
  offer — and does it better than OpenCode, since Kilo's free tier serves **tool calls**,
  which OpenCode's free models refuse.
* Good, because `supported_parameters` lets capability gating be exact for the first time in
  this package: forced `tool_choice` and `reasoning_effort` are sent only to models that
  declare them, instead of being sent hopefully and failing at runtime.
* Neutral, because Kilo exposes three interchangeable wire formats and this design uses one.
  Nothing is lost — the gateway translates, so route choice affects only envelope shape, not
  model reach — but it is a capability deliberately left on the table.
* **Bad, because the shared decoder now carries two field names for one concept**
  (`reasoning_content` and `reasoning`), neither documented by any vendor. This is
  irreducible: they are different gateways' conventions for the same value. Mitigated by
  decoding both into one `ReasoningItem`, testing both spellings, and citing the gateway
  each was measured on.
* Bad, because Kilo's documented `402 Insufficient balance` has no dedicated sentinel.
  It falls to `ErrInvalidRequest` (non-retryable), which is the **correct** retry behaviour
  — retrying will not add balance — but produces a misleading "invalid request" message for
  what is really a billing state. Accepted rather than fixed: adding an `ErrPaymentRequired`
  sentinel changes a shared, exported error surface for one provider's benefit, which is out
  of proportion to this change and is noted as possible future work.
* Bad, because `isUsableKiloModel` excluding `mayTrainOnYourPrompts` models is a policy
  judgement, not a capability filter — the only one this package makes. It is documented in
  the function's comment and asserted by a test so it is visible and reversible.

### Confirmation

* `llmprovider/constants.go` defines `ProviderOpencodeZen` and `ProviderOpencodeGo`;
  `ProviderEnvVars` maps both to `OPENCODE_API_KEY`; `NewProvider` dispatches both with the
  correct default base URLs. Verified by `grep -n ProviderOpencode` and `TestNewProvider`.
* `llmprovider/opencode.go` implements `Provider`, `ToolProvider`, `ThinkingProvider`,
  `ThinkingToolProvider`, `ItemProvider`, `ItemToolProvider`, `ItemThinkingProvider`,
  `ItemThinkingToolProvider`, and `ModelDiscoverer`, with compile-time assertions added to
  `interface_test.go`. A comment records that `Continuer` is deliberately absent, citing the
  measured 400.
* `TestOpencodeRoute` is table-driven over **both** gateways and asserts the divergence
  explicitly: `("opencode-zen","minimax-m3") → chatCompletions` **and**
  `("opencode-go","minimax-m3") → messages`; plus `gpt-5.5`/`grok-4.6`/`muse-spark-1.2` →
  responses, `claude-opus-4-5`/`qwen3.6-plus` → messages, `gemini-3.7-flash` → google,
  `deepseek-v4-pro`/`glm-5.2`/`kimi-k3` → chatCompletions, and an unknown ID → the
  per-gateway prefix fallback.
* `TestOpencode_RoutePaths` uses one `httptest.Server` via `WithBaseURL` and asserts the
  request path per model: `/responses`, `/messages`, `/chat/completions`,
  `/models/<id>:generateContent`.
* `TestOpencode_KeyInHeader` asserts `Authorization: Bearer` is set on all four routes and
  that the key never appears in the URL; `TestOpencode_NoAnthropicHeaders` asserts
  `x-api-key` and `x-goog-api-key` are never sent.
* `TestOpencode_DecodeChatCompletions` covers content, `tool_calls`, and `reasoning_content`
  → `MessageItem` / `FunctionCallItem` / `ReasoningItem`.
* `TestOpencode_ErrorClassification` covers 429 (no `Retry-After` → `RateLimitError` with
  `RetryAfter == 0`), 401, 500, and 400 → the four sentinels.
* `TestOpencode_WithOpencodeRoute` asserts the override beats both table and heuristic.
* `llmprovider/discovery.go` handles both names via `listOpencodeModels`, with a test
  asserting it succeeds against a server that requires **no** `Authorization` header;
  `models_catalog.go` returns the curated statics, with `isUsableOpencodeModel` /
  `RankOpencodeModel` tests mirroring `models_catalog_test.go`.
* Optional `//go:build opencode_live` integration test hits `hy3-free` on
  `/chat/completions` and `muse-spark-1.2-contributor-free` on `/responses` with no API key.
  Excluded from the default `go test ./...`.
* `llmprovider/chatcompletions.go` exists with provider-neutral names
  (`itemsToChatMessages`, `decodeChatCompletionsResponse`) and **no** `opencode` or
  `huggingface` prefix on either; `grep -n 'func opencodeItemsToChatMessages'` returns
  nothing.
* `llmprovider/constants.go` defines `ProviderHuggingFace = "huggingface"`;
  `ProviderEnvVars["huggingface"] == "HF_TOKEN"`; `NewProvider` dispatches it to
  `NewHuggingFace` with default base URL `https://router.huggingface.co/v1`.
* `llmprovider/huggingface.go` implements the nine interfaces listed above and **not**
  `Continuer`, with compile-time assertions in `interface_test.go` and a negative assertion
  for `Continuer`.
* `TestHuggingFace_RequestShape` asserts, via `httptest` and `WithBaseURL`, that the path is
  `/chat/completions`, that `Authorization: Bearer` is set, and that `/v1/responses` is
  never requested.
* `TestHuggingFace_ReasoningEffort` asserts the request body carries
  `reasoning_effort: "medium"` by default on the thinking path and honours
  `WithReasoningEffort("xhigh")`.
* `TestHuggingFace_ToolCall` asserts the forced `tool_choice` shape and that
  `tool_calls[]` decodes to `FunctionCallItem` through the **shared** decoder.
* `TestListHuggingFaceModels_MetadataCuration` serves a fixture containing a
  vision-language model, a model whose only provider is not `live`, and three text models
  with differing `throughput`, then asserts: VLM and non-live models are dropped, and the
  survivors come back in descending-throughput order — proving the `nil` `rankFn` preserves
  metadata ordering.
* `TestListHuggingFaceModels_NoToken` asserts the lister succeeds against a server that
  rejects any request carrying an `Authorization` header.
* `TestStaticHuggingFace_Count` asserts `len(StaticHuggingFace) <= MaxListedModels`, and
  `TestHuggingFaceModelPolicySuffix` asserts `openai/gpt-oss-120b:groq` matches the catalog
  entry `openai/gpt-oss-120b` (split at the last colon, not the first).
* `llmprovider/constants.go` defines `ProviderKilo = "kilo"`;
  `ProviderEnvVars["kilo"] == "KILO_API_KEY"`; `NewProvider` dispatches it to
  `NewKilo` with default base URL `https://api.kilo.ai/api/gateway`.
* `llmprovider/kilo.go` implements the same nine interfaces as `HuggingFaceProvider`
  and **not** `Continuer`, with assertions in `interface_test.go`.
* `TestKilo_RequestShape` asserts path `/chat/completions`, `Authorization: Bearer`,
  and that `/responses` and `/messages` are never requested.
* `TestDecodeChatCompletions_ReasoningFieldNames` is table-driven over **both** spellings:
  a fixture using `reasoning_content` (OpenCode's measured `big-pickle` body) and one using
  `reasoning` plus `reasoning_details` (Kilo's measured body) each yield exactly one
  `ReasoningItem` with the expected text; a fixture with both present prefers
  `reasoning_content`; a fixture with neither yields no `ReasoningItem` and no error.
* `TestKilo_SupportedParameterGating` asserts that a model whose listing entry omits
  `tool_choice` produces a request with `tools` but **no** `tool_choice`, and that a model
  omitting `reasoning_effort` produces a thinking request without that field.
* `TestListKiloModels_MetadataCuration` serves a fixture with a vision model, a
  `mayTrainOnYourPrompts: true` model, a non-tool model, and three priced text models; it
  asserts the first three are dropped and the rest come back cheapest-first, with a
  `pricing.completion` of `"-1"` treated as unranked rather than free.
* `TestStaticKilo_Count` asserts `len(StaticKilo) <= MaxListedModels`.
* A `live_gateways` build-tagged suite hits `kilo-auto/free` on the real gateway with **no**
  API key, asserting both a plain generation and a forced tool call — the only live test in
  this change that covers tool calling end-to-end.
* Each gateway file declares `wireShapesProbedOn`, and its value matches the probe date
  cited in this MADR for that gateway.
* The `live_gateways` suite asserts **shapes, not just liveness** — each assertion pinned to
  the decision it protects:
  * OpenCode: `muse-spark-1.2-contributor-free` still succeeds on `/responses` **and** still
    fails on `/chat/completions` — the measurement the whole route table rests on;
  * Kilo: `choices[0].message.reasoning` is still the spelling emitted, and `GET /models`
    still publishes `supported_parameters` containing `tools`;
  * Hugging Face: `GET /v1/models` still publishes `throughput`,
    `first_token_latency_ms`, `supports_tools` and `architecture.output_modalities` —
    the four fields metadata-driven curation depends on;
  * all three: `GET /models` still answers `200` with **no** credential.
* A drift failure names the affected decision, so the suite reads as a drift report rather
  than an opaque assertion error.
* `go vet ./...`, `golangci-lint run -c .golangci.yml ./...`, and `go test ./...` pass.

## Pros and Cons of the Options

### 1. Self-contained providers reusing the package's response decoders (chosen)

* Good, because it covers all four routes on both gateways with one auth path and one type.
* Good, because it reuses all three existing decoders and the status-classification
  discipline — the parts that are hard to get right — while writing only the small,
  route-specific request bodies.
* Good, because it touches no existing provider file, so the current suite stays a
  regression net.
* Good, because `WithOpencodeRoute` gives callers a way around table drift.
* Bad, because the route table is docs-coupled and needs periodic reconciliation.
* Bad, because two logical providers share one implementation type.

### 2. Delegating provider calling the existing provider structs (revision 1's choice)

* Good, because it would have written the least new code, reusing whole providers rather
  than just their decoders.
* **Bad, because it does not work.** `ClaudeProvider` sends `x-api-key` (`claude.go:212`)
  and `GeminiProvider` sends `x-goog-api-key` (`gemini.go:211`); the gateway reads neither
  and returns `401 "Missing API key."` — measured. Only the Responses route could be
  delegated, and that route needs no new code regardless.
* Bad, because it would allocate a throwaway provider struct per call.

### 3. Make the auth header injectable across all providers, then delegate

* Good, because it would rescue option 2 and maximise reuse.
* Bad, because it edits `claude.go`, `gemini.go`, and `options.go` — three shipping files
  with existing coverage — so that a fourth, unrelated provider can borrow them.
* Bad, because an injectable auth header on a vendor-specific provider is a footgun: it
  makes `ClaudeProvider` look like a general Anthropic-protocol client when its body
  builder, `anthropic-version` pin, and error strings are all Anthropic-specific.
* Bad, because the delegated calls would still need per-route body adjustments, so the
  saving over option 1 is only the decoder wiring — which option 1 already gets for free by
  living in the same package.

### 4. Thin OpenAI-compatible wrapper around `OpenAIProvider` with `BaseURL` override only

* Good, because it is the smallest possible change and works immediately for the Responses
  family.
* **Bad, because `/chat/completions` is not a universal endpoint** — measured:
  `muse-spark-1.2-contributor-free` returns 500 there, `big-pickle` returns 500 on
  `/responses`. A single-endpoint wrapper silently restricts callers to one routing bucket
  and 500s on the rest, which is most of both catalogs.
* Bad, because it would leak gateway internals the `opencode/<id>` model ref exists to hide.

### 5. Listing-only integration (Ollama precedent)

* Good, because it is the smallest change with in-repo precedent (`discovery.go:33-34`).
* Bad, because it delivers no generation capability — the gateway would be discoverable but
  unusable for the package's primary purpose.
* Bad, because it is inconsistent with the four fully generative `Provider*` constants.

### 6. Vendor SDK-based provider

* Good, because it would delegate wire details upstream.
* Bad, because it violates the package's explicit SDK-free design (`provider.go:1-3`) and
  would add a dependency consumed by twelve binaries.
* Bad, because there is no Go SDK for these gateways; the AI SDK packages named in the docs
  are TypeScript.

### 7. Hugging Face on `/v1/chat/completions` only (chosen)

* Good, because it is the only Hugging Face surface the vendor actually documents, with a
  full published parameter and response schema.
* Good, because it reuses the shared Chat Completions primitive that OpenCode already
  needs, so Hugging Face's marginal wire cost is zero.
* Good, because `reasoning_effort` and forced `tool_choice` are both documented on it, so
  `ThinkingProvider` and `ToolProvider` map to specified behaviour rather than to a guess.
* Bad, because reasoning traces arrive only if an upstream provider volunteers a
  non-standard `reasoning_content`; `ReasoningItem` will usually be absent. Accepted: the
  package already treats reasoning items as optional.

### 8. Hugging Face on `/v1/responses` only

* Good, because the Responses envelope carries typed output items natively, which is the
  package's canonical shape, and `decodeResponsesAPIOutput` already parses it.
* **Bad, because it returns HTTP 200 with `status:"failed"` and an `error` object on auth
  failure** (measured). Under `classifyHTTPStatus`, which reads only the status code, that
  decodes to a `*Response` with empty `Output` and a **nil error** — a silent empty
  generation where an `ErrAuthFailure` belongs. Adopting it would require a body-inspecting
  error path that no other provider in the package has.
* Bad, because the official documentation states the OpenAI-compatible endpoint is "for
  chat completion tasks only" and the task reference documents no Responses surface. Building
  on an endpoint the vendor does not acknowledge invites silent breakage.

### 9. Hugging Face on both endpoints, selected per model

* Good, because it would in principle prefer the richer Responses envelope where supported.
* Bad, because it recreates the exact per-model routing burden that makes OpenCode the
  expensive provider — for a vendor that imposes none.
* Bad, because there is no signal to route on: `GET /v1/models` says nothing about
  Responses support, and the endpoint is undocumented, so the table would be pure guesswork.
* Bad, because it inherits the HTTP-200-on-failure hazard of option 8 for half the catalog.

### 10. Metadata-driven Hugging Face discovery (chosen)

* Good, because the listing publishes measured `throughput`, `first_token_latency_ms`,
  `supports_tools`, `context_length` and `pricing` — the ground truth that every other
  `Rank*` function in the package can only approximate by grepping model names.
* Good, because it needs no credential, so discovery works before a token is configured.
* Good, because it requires **no change** to `curateFromCatalog`: passing `nil` for
  `rankFn` preserves the caller's ordering (`models_catalog.go:224-226`).
* Good, because it drops the 40 vision-language models structurally, via
  `architecture.input_modalities`, rather than by denylisting substrings like `"vision"` and
  hoping the naming holds.
* Bad, because it introduces a second discovery idiom in a package that has one, so a
  reader must understand why Hugging Face differs. Mitigated by keeping the exported
  `RankHuggingFaceModel` for convention parity and documenting the divergence at the call
  site.

### 11. Static Hugging Face catalog primary, listing only to confirm availability

* Good, because it is uniform with the four existing providers and with OpenCode.
* Bad, because it discards published metrics in favour of name-grepping, choosing a worse
  signal when a better one is free.
* Bad, because Hugging Face's catalog is open and churns continuously; a six-entry
  hand-maintained list is a far poorer approximation of 136 live models than it is of
  OpenCode's curated ~64.
* Bad, because it cannot express `supports_tools`, so `GenerateWithTool` would be offered
  on models where 97 of 317 offerings do not support it.

### 12. Kilo on `/chat/completions` only (chosen)

* Good, because it is Kilo's only **documented** route, and the one its per-model
  `supported_parameters` metadata describes (`tools`, `tool_choice`, `reasoning_effort`,
  `response_format` are all Chat Completions parameters).
* Good, because it makes Kilo the third consumer of the shared primitive: zero new wire
  logic, one new struct.
* Good, because it sidesteps the `/responses` reasoning-placement gap entirely.
* Bad, because reasoning arrives as an undocumented `message.reasoning` string rather than
  as a typed item, so the decoder carries a second field name. Accepted — see Consequences.

### 13. Kilo on `/responses`, for the canonical item envelope

* Good, because the Responses envelope is the package's canonical item shape and
  `decodeResponsesAPIOutput` already parses it; reasoning would arrive as a typed item
  rather than an ad-hoc string field.
* **Bad, because Kilo puts reasoning text in `output[].content[].type=="reasoning_text"`
  and leaves `summary: []`** (measured), while `decodeResponsesAPIOutput` reads only
  `summary[]` (`http_helpers.go:64-71`). Adopting this route silently drops every reasoning
  trace, or forces an edit to a decoder shared by OpenAI, Grok and OpenCode — a wide blast
  radius for one provider.
* Bad, because the route is undocumented: the API reference lists `/chat/completions`,
  `/models`, `/providers` and FIM, and nothing else. Building on it invites silent breakage.
* Bad, because `supported_parameters` describes Chat Completions parameters, so capability
  gating would be describing a different endpoint than the one being called.

### 14. Kilo multi-route, exposing all three formats via an option

* Good, because Kilo genuinely translates: the same model answers on `/chat/completions`,
  `/responses` and `/messages` (measured), so unlike OpenCode a route option here would be
  safe rather than a 500 waiting to happen.
* **Bad, because it buys nothing.** With OpenCode, routing is mandatory — it is the only way
  to reach most of the catalog. With Kilo the gateway translates, so every model is
  reachable on the one route already chosen. A second path would add test surface and
  configuration for zero additional model coverage.
* Bad, because two of the three routes are undocumented, so the option would expose
  unsupported behaviour as a public API of this package.
* Bad, because it would tempt a future reader to generalise OpenCode's `WithOpencodeRoute`
  into a cross-provider concept, conflating two mechanisms that are opposites: OpenCode's
  route is a *correctness requirement*, Kilo's would be a *preference*.

## Open Questions

These could not be resolved without a funded API key and must be closed during
implementation, before the corresponding tests are claimed as passing.

**OpenCode Zen / Go** (needs `OPENCODE_API_KEY`):

1. Does the Messages route require or reject the `anthropic-version` header? Anthropic
   requires it natively; the gateway may inject it. Unreachable behind the auth check.
2. Do tool calls work on the Messages, Google, and Chat Completions routes, and is
   `tool_choice` forcing honoured? Free models return `"Endpoint is unavailable"` for tool
   requests, so this is untested.
3. Is reasoning/thinking accepted on the Messages and Google routes, and in what parameter
   shape does the gateway expect it?
4. Is chaining supported on any route with a funded key, or is the measured 400 universal?
   If a funded key does chain, `Continuer` can be added later without breaking the
   contract, since it is an optional interface.

**Hugging Face** (needs `HF_TOKEN`; note every question here is reachable with the free
$0.10 monthly credit, so these are cheaper to close than the OpenCode set):

5. Does the router return `429` on quota exhaustion, and does it send `Retry-After`? The
   pricing page documents credits but names no status code, and the free tier could not be
   exercised without a token. If `Retry-After` is absent, `RateLimitError.RetryAfter` is
   `0` and `GenerateWithRetry` falls back to jittered exponential backoff — the same
   outcome as OpenCode, so no code changes either way; the test fixture must simply assert
   the measured behaviour.
6. Is `reasoning_effort` accepted-and-ignored, or rejected with `400`, by a provider that
   does not support it? The docs say "support and defaults are provider and model-dependent"
   without specifying the failure mode. If some providers `400`, the thinking path needs a
   documented retry-without-reasoning fallback rather than a hard failure.
7. Does any router provider emit `reasoning_content` on `/v1/chat/completions`? The
   documented response schema omits it, but the shared decoder tolerates it. This only
   determines whether `ReasoningItem` is ever populated for Hugging Face — a
   documentation-accuracy question, not a correctness one.
8. Does a forced `tool_choice` succeed on a model whose listing entry reports
   `supports_tools: false`, or does it error? This decides whether `GenerateWithTool`
   should pre-check the flag and fail fast with `ErrInvalidRequest`, or simply attempt the
   call.

**Kilo Gateway** (needs `KILO_API_KEY`; note questions 9–10 are answerable **without** a
key, against `kilo-auto/free`, and should be closed during implementation rather than
deferred):

9. Does Kilo send `Retry-After` with `429`? Reachable free-of-charge by driving
   `kilo-auto/free` until the free tier limits. Determines only what the test fixture
   asserts, not the code path.
10. Does a paid model return the documented `402 Insufficient balance` with an exhausted
    balance, or the `401 PAID_MODEL_AUTH_REQUIRED` envelope seen with no credential at all?
    This decides whether the "insufficient balance" case is even reachable as a distinct
    status for this package, and therefore whether the `ErrPaymentRequired` future work
    noted in Consequences is worth doing.
11. Are `/responses` and `/messages` a supported surface Kilo intends to keep, or an
    artefact of its upstream proxying? Worth one question to `hi@kilo.ai` before relying on
    the *absence* of routing in any documentation this package ships. The decision does not
    depend on the answer — option 12 uses neither route — but the MADR's claim that Kilo
    "translates formats" should not be repeated as a durable guarantee without it.
12. **Is capability gating necessary at all?** Does a Kilo model whose
    `supported_parameters` omits `tool_choice` actually *reject* a forced `tool_choice`, or
    silently ignore it? Answerable with **no credential** against a free model that lacks
    the parameter. If it rejects, `WithKiloCapabilities` is load-bearing and should be
    documented as recommended; if it ignores, the option remains a no-cost refinement and
    the "capability gating can be exact" consequence above softens to "exact where it
    matters". No code changes either way.

## More Information

* **Primary sources — OpenCode (retrieved and, where marked ✓, executed 2026-08-28):**
  * `https://opencode.ai/docs/zen/` — endpoint table (64-row model → route mapping),
    pay-as-you-go pricing, `GET /zen/v1/models`, model ref `opencode/<id>`.
  * `https://opencode.ai/docs/go/` — base `https://opencode.ai/zen/go/v1`, its own 27-row
    endpoint table, `$10/month` subscription with 5h/weekly/monthly usage windows,
    model ref `opencode-go/<id>`.
  * `https://models.dev/api.json` ✓ — provider registry entries `opencode` and
    `opencode-go`: base URLs and `env=['OPENCODE_API_KEY']`.
  * Live gateway probes ✓ — auth header discrimination, unauthenticated `/models`,
    validation-before-auth ordering, free-model cross-route 200/500 matrix, absent
    `Retry-After` on 429, `/responses` and `/chat/completions` body shapes, and the
    `previous_response_id` 400.
* **Primary sources — Hugging Face (retrieved and, where marked ✓, executed 2026-08-29):**
  * `https://huggingface.co/docs/inference-providers/index` — router base URL
    `https://router.huggingface.co/v1`, `HF_TOKEN`, the 18-partner capability matrix, the
    `:fastest`/`:cheapest`/`:preferred`/`:<provider>` policy suffixes, and the explicit
    "available for chat completion tasks only" scoping of the OpenAI-compatible endpoint.
  * `https://huggingface.co/docs/inference-providers/tasks/chat-completion` — the full
    request/response schema, including `reasoning_effort` with values
    `none, minimal, low, medium, high, xhigh`, forced `tool_choice`, and the
    `choices[].message.tool_calls[]` shape.
  * `https://huggingface.co/docs/inference-providers/pricing` — monthly credit tiers
    ($0.10 free / $2.00 PRO / $2.00 per seat), routed-vs-custom-key billing, and the
    `X-HF-Bill-To` organization header.
  * Live router probes ✓ — unauthenticated `GET /v1/models` (136 models, 317 live
    offerings, full metadata census); `Authorization: Bearer` discrimination and the
    `{"error":"<string>"}` envelope; auth-before-model-validation ordering; the
    `/v1/responses` HTTP-200-with-`status:"failed"` measurement; query parameters on
    `/v1/models` being ignored; Go `net/http` retrieving the listing at 94,116 bytes; and
    DNS failure for the retired `api-inference.huggingface.co` host.
* **Primary sources — Kilo (retrieved and, where marked ✓, executed 2026-08-29):**
  * `https://kilo.ai/docs/gateway/api-reference` — base URL `https://api.kilo.ai/api/gateway`,
    `Authorization: Bearer $KILO_API_KEY`, the endpoint list (`POST /chat/completions`,
    `GET /models` and `GET /providers` both unauthenticated, `POST /api/fim/completions`),
    the request/response schema, and the documented error codes including
    `402 Insufficient balance`.
  * `https://kilo.ai/docs/ai-providers/kilocode` — the Kilo Code provider as configured in
    the agent; free models on registration, top-up at `app.kilo.ai/profile`.
  * `https://models.dev/api.json` ✓ — registry entry `kilo`: `name="Kilo Gateway"`,
    `api=https://api.kilo.ai/api/gateway`, `env=['KILO_API_KEY']`, 364 models.
  * Live gateway probes ✓ — unauthenticated `GET /models` (366 models, full
    `supported_parameters` census) and `GET /providers`; byte-identical catalogs from
    `api.kilo.ai/api/gateway` and the `kilo.ai/api/openrouter` alias, and the
    `kilocode.ai → kilo.ai` 308 redirect; credential-free generation **and forced tool
    calling** on `kilo-auto/free`; the same model succeeding on `/chat/completions`,
    `/responses` and `/messages`; the `message.reasoning` / `reasoning_details[]` shape;
    the `reasoning_text`-in-`content[]` placement on `/responses`; and the
    `PAID_MODEL_AUTH_REQUIRED` 401 envelope.
* **Codebase evidence (verified against the current checkout):**
  `provider.go:19-61` capability interfaces; `provider.go:64-103` sentinels and
  `RateLimitError`; `provider.go:106-111` `ProviderEnvVars`; `provider.go:213-226`
  `NewProvider`; `options.go:23-89` `ProviderConfig` and options;
  `http_helpers.go:23-76` `decodeResponsesAPIOutput`; `claude.go:239` `decodeClaudeResponse`;
  `gemini.go:239` `decodeGeminiResponse`; `models_catalog.go:11` `MaxListedModels = 6`;
  `models_catalog.go:182` `curateFromCatalog`; `probe.go:13` `probeGenerateHealth`;
  hardcoded auth headers at `openai.go:164`, `grok.go:183`, `claude.go:212-213`,
  `gemini.go:211`.
* **Prior MADRs:** `docs/0001-MADR-add-grok-xai-llm-provider.md` (canonical item/state
  scope, Grok reasoning gating, in-place revision convention) and its
  `0001-PLAN` (registration touch points); `docs/0002-MADR-xdg-compliant-user-paths.md`
  (shared `mcplib` package precedent).
* **Implementation plan** — written as `0003-PLAN-add-gateway-llm-providers.md`
  (revision 2, dated 2026-08-29) and **not yet approved**. No source edits may begin until
  it is. It enumerates: the exact `OpencodeRoute` values and the full per-gateway route
  table with its docs provenance; the `WithOpencodeRoute` option signature; the
  `classifyHTTPStatus` extraction; the shared `chatcompletions.go` primitive with its dual
  reasoning-field handling; the four OpenCode request builders; all four static catalogs
  with live-listing evidence for every ID; the three no-credential listers; Kilo's
  `supported_parameters` gating; the `live_gateways` shape-assertion suite and the
  `wireShapesProbedOn` constants; and a pre-committed resolution rule for each Open
  Question above.
* **Out of scope:** streaming (`iter.Seq2`), `encoding/json/v2`, batch/files/embeddings,
  pricing and usage-window logic, `Continuer` support, refactoring existing providers onto
  `classifyHTTPStatus`, and consumer migration in `mcp-server-magictools` /
  `mcp-server-magicdev`. Hugging Face specifically: the non-chat task APIs (text-to-image,
  embeddings, speech), the `X-HF-Bill-To` organization-billing header, custom-provider-key
  configuration, and any use of `/v1/responses`. Kilo specifically: the FIM endpoint
  (`POST /api/fim/completions`), `GET /providers` and its data-policy metadata beyond the
  `mayTrainOnYourPrompts` flag already used, the `kilo.ai/api/openrouter` alias host, any
  use of `/responses` or `/messages`, and an `ErrPaymentRequired` sentinel for `402`.
* **Filename (decided 2026-08-29).** Renamed from
  `0003-{MADR,PLAN}-add-opencode-zen-go-llm-provider.md` to
  `0003-{MADR,PLAN}-add-gateway-llm-providers.md`: the old slug named one of three gateway
  families. Both files were untracked at the time, so no history was rewritten; the plan's
  `parent-madr:` frontmatter and every cross-reference were updated with the move.
