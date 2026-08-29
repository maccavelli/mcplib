---
status: accepted
date: 2026-08-29
parent-madr: 0003-MADR-add-gateway-llm-providers.md
decision-makers: mcplib maintainers
---

# Implementation Plan: Gateway LLM Providers on a Shared Chat Completions Primitive

> Paired with [0003-MADR-add-gateway-llm-providers.md](0003-MADR-add-gateway-llm-providers.md)
> (revision 4). Implements four new provider identifiers across three gateway families —
> OpenCode Zen, OpenCode Go, Hugging Face Inference Providers, and Kilo Gateway — on one
> shared Chat Completions primitive.

> **Revision notes (revision 2 of this plan, 2026-08-29).** Revision 1 covered only the two
> OpenCode gateways. This revision follows MADR revisions 3–4 and makes three changes:
> **(a)** Phase 2's Chat Completions artefacts are renamed to provider-neutral names and
> gain dual reasoning-field handling; **(b)** two new phases add Hugging Face and Kilo;
> **(c)** registration is folded into each provider's own phase instead of a separate late
> phase, so every phase now ends with one complete, usable provider. Revision 1's Phases 1
> and 3 are otherwise unchanged.

> **Revision notes (revision 3 of this plan, 2026-08-29).** Follows MADR revision 5: the
> build tag is renamed `gateway_live` → `live_gateways` to match the `live_<thing>`
> convention used across this codebase family; each gateway file gains a
> `wireShapesProbedOn` constant; and Phase 7's live tests are upgraded from liveness checks
> to **shape assertions**, which is what makes the date pin mean anything. Phases 1–6 are
> unchanged apart from the added constants.

## Table of Contents

- [0. Notation and Conventions](#0-notation-and-conventions)
- [1. Baseline State (verified 2026-08-29)](#1-baseline-state-verified-2026-08-29)
- [2. Reuse Inventory — What Already Exists](#2-reuse-inventory--what-already-exists)
- [3. Authoritative Gateway Data](#3-authoritative-gateway-data)
- [4. Type and Signature Definitions](#4-type-and-signature-definitions)
- [5. Phase Sequencing Overview](#5-phase-sequencing-overview)
- [Phase 1 — Constants, Route Table, Route Resolution](#phase-1--constants-route-table-route-resolution)
- [Phase 2 — Shared Primitives: Error Classifier + Chat Completions](#phase-2--shared-primitives-error-classifier--chat-completions)
- [Phase 3 — `OpencodeProvider` Core](#phase-3--opencodeprovider-core)
- [Phase 4 — OpenCode Discovery, Catalogs, Registration](#phase-4--opencode-discovery-catalogs-registration)
- [Phase 5 — Hugging Face Provider](#phase-5--hugging-face-provider)
- [Phase 6 — Kilo Gateway Provider](#phase-6--kilo-gateway-provider)
- [Phase 7 — Live Verification and Open Questions](#phase-7--live-verification-and-open-questions)
- [8. Verification Commands](#8-verification-commands)
- [9. Acceptance Criteria](#9-acceptance-criteria)
- [10. Decisions Resolved by This Plan](#10-decisions-resolved-by-this-plan)
- [11. Risks and Mitigations](#11-risks-and-mitigations)
- [12. Out of Scope](#12-out-of-scope)
- [13. File Summary](#13-file-summary)
- [14. Deviation Log](#14-deviation-log)

---

## 0. Notation and Conventions

- **File references** use `llmprovider/file.go:L<start>-L<end>` against the checkout
  snapshot of **2026-08-29** (`git log -1` → `86679bf ci: pin golangci-lint to v2.13.1`).
- `make test` = `go test ./...` ([Makefile:16-17](../Makefile#L16-L17)).
- `make vet` = `go vet ./...` ([Makefile:29-30](../Makefile#L29-L30)).
- `make lint` = `golangci-lint run -c .golangci.yml ./...` ([Makefile:32-38](../Makefile#L32-L38)).
- **"Phase green"** means, for the files that phase touches: `gofmt -l` prints nothing, and
  `make vet`, `make lint`, `make test` all exit 0.
- Each phase ends with **one commit**, made only after that phase is green, per the
  repository's commit-cadence rule. **No `git push` at any point in this plan.**
- **Gateway family** is OpenCode / Hugging Face / Kilo. **Provider identifier** is one of
  the four canonical strings. **Route** is one of OpenCode's four wire formats — the term
  applies to OpenCode only; Hugging Face and Kilo each use a single endpoint.
- Code blocks are the **intended final source**, not sketches. Where abbreviated, the block
  says so explicitly.

---

## 1. Baseline State (verified 2026-08-29)

| Check | Command | Result |
|---|---|---|
| Tests green | `go test ./llmprovider/...` | `ok github.com/maccavelli/mcplib/llmprovider 0.623s` |
| Vet clean | `go vet ./llmprovider/...` | no output |
| No prior work | `grep -ri 'opencode\|huggingface\|hf_\|kilo' .` | zero matches |
| No identifier collisions | `grep -c` for all 12 planned new symbols | zero matches for each |
| Go version | `go.mod:3` | `go 1.26.5` |
| Lint config | `.golangci.yml` | `version: "2"`, 20 linters incl. `goconst`, `gocritic`, `revive`, `gosec` |

`llmprovider/` is 4,744 lines across 28 files. The five registration touch points are
`constants.go:3-9`, `provider.go:106-111`, `provider.go:213-226`, `discovery.go:18-38`,
`models_catalog.go:97-110`.

**Exactly two existing tests enumerate the provider set** and must be extended:
`provider_test.go:95` and `models_catalog_test.go:197`. No other test hard-codes a provider
count.

---

## 2. Reuse Inventory — What Already Exists

The plan's central efficiency claim: the per-provider item→request converters are already
**package-level functions**, not methods, so new files in the same package call them
directly — no delegation, no struct construction.

| Need | Existing symbol | Location | Reusable as-is? |
|---|---|---|---|
| Items → Responses `input[]` | `itemsToInput` | `grok.go:120-138` | **Yes** — already shared by OpenAI + Grok |
| Items → Anthropic `messages[]` | `claudeItemsToMessages` | `claude.go:138-167` | **Yes** |
| Items → Google `contents[]` | `geminiItemsToContents` | `gemini.go:133-167` | **Yes** |
| Items → Chat Completions `messages[]` | — | — | **No — net new (Phase 2)** |
| Decode Responses envelope | `decodeResponsesAPIOutput` | `http_helpers.go:23-76` | **Yes** |
| Decode Anthropic envelope | `decodeClaudeResponse` | `claude.go:239-293` | **Yes** |
| Decode Google envelope | `decodeGeminiResponse` | `gemini.go:239-291` | **Yes** |
| Decode Chat Completions envelope | — | — | **No — net new (Phase 2)** |
| Status → sentinel classification | inline switch, duplicated ×4 | `openai.go:174-185`, `claude.go:223-234`, `gemini.go:223-234`, `grok.go:195-206` | **Extract to shared helper (Phase 2)** |
| Body close | `closeResponseBody` | `http_helpers.go:12-19` | **Yes** |
| Catalog curation | `curateFromCatalog` | `models_catalog.go:182-240` | **Yes — unchanged** |
| Health probe | `probeGenerateHealth` | `probe.go:13-59` | **Yes** |
| Retry-After parsing | `parseRetryAfter` | `provider.go:89-103` | **Yes** |
| **Test:** body-capturing httptest server | `captureServer(t, *map[string]any, respBody)` | `thinking_test.go:14-26` | **Yes** |
| **Test:** body-capturing server (variant) | `bodyCapture(t, respBody) (*httptest.Server, *map[string]any)` | `provider_correctness_test.go:34-45` | **Yes** |

**Net-new wire logic across the whole change is exactly two functions** — one item
converter and one decoder, both Chat Completions, both written once in Phase 2 and used by
three providers. Everything else is assembly.

### Decoder behaviour notes that affect this work

- `decodeClaudeResponse` hardcodes `Response{ID: ""}` (`claude.go:258`) and errors on empty
  `content`. Correct for us — no provider here implements `Continuer`.
- `decodeGeminiResponse` errors with `"gemini returned no content"` when
  `candidates[0].content.parts` is empty (`gemini.go:260-262`). The error text says "gemini"
  even on OpenCode's Google route; accepted, since it names the *wire format*. Documented in
  a code comment rather than reworded, to avoid touching `gemini.go`.
- `decodeResponsesAPIOutput` reads reasoning **only** from `summary[]`
  (`http_helpers.go:64-71`). OpenCode's Zen `/responses` returns `summary: []` with
  `encrypted_content`, so it yields `ReasoningItem{Text: ""}` — present but blank. Tests
  must expect that, not its absence. **This same limitation is why Kilo's `/responses` route
  is not used** (MADR option 13).
- `curateFromCatalog` sorts the backfill only `if rankFn != nil` (`models_catalog.go:224-226`).
  Passing `nil` therefore **preserves the caller's ordering** of `available`. Phases 5 and 6
  depend on this; no change to `curateFromCatalog` is required.

---

## 3. Authoritative Gateway Data

### 3.1 OpenCode — Zen route table (`https://opencode.ai/zen/v1`)

Extracted programmatically from `https://opencode.ai/docs/zen/` on 2026-08-28 (63 rows).

| Route | Model IDs |
|---|---|
| `responses` (25) | `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-5.5`, `gpt-5.5-pro`, `gpt-5.4`, `gpt-5.4-pro`, `gpt-5.4-mini`, `gpt-5.4-nano`, `gpt-5.3-codex`, `gpt-5.3-codex-spark`, `gpt-5.2`, `gpt-5.2-codex`, `gpt-5.1`, `gpt-5.1-codex`, `gpt-5.1-codex-max`, `gpt-5.1-codex-mini`, `gpt-5`, `gpt-5-codex`, `gpt-5-nano`, `grok-4.6`, `grok-4.5`, `grok-build-0.1`, `muse-spark-1.2`, `muse-spark-1.2-contributor-free` |
| `messages` (14) | `claude-fable-5`, `claude-opus-5`, `claude-opus-4-8`, `claude-opus-4-7`, `claude-opus-4-6`, `claude-opus-4-5`, `claude-sonnet-5`, `claude-sonnet-4-6`, `claude-sonnet-4-5`, `claude-haiku-4-5`, `qwen3.7-max`, `qwen3.7-plus`, `qwen3.6-plus`, `qwen3.5-plus` |
| `google` (6) | `gemini-3.7-flash`, `gemini-3.6-flash`, `gemini-3.5-flash`, `gemini-3.5-flash-lite`, `gemini-3.1-pro`, `gemini-3-flash` |
| `chat_completions` (18) | `deepseek-v4-pro`, `deepseek-v4-flash`, `minimax-m3`, `minimax-m2.7`, `minimax-m2.5`, `glm-5.2`, `glm-5.1`, `glm-5`, `kimi-k2.5`, `kimi-k2.6`, `kimi-k2.7-code`, `kimi-k3`, `big-pickle`, `mimo-v2.5-free`, `hy3-free`, `ling-3.0-flash-fin-free`, `nemotron-3-ultra-free`, `nemotron-3.5-lightning-free` |

### 3.2 OpenCode — Go route table (`https://opencode.ai/zen/go/v1`)

From `https://opencode.ai/docs/go/`, 2026-08-28 (26 rows).

| Route | Model IDs |
|---|---|
| `responses` (3) | `grok-4.6`, `gpt-5.6-luna`, `muse-spark-1.2-contributor` |
| `messages` (8) | `minimax-m3`, `minimax-m2.7`, `minimax-m2.5`, `qwen3.8-max`, `qwen3.8-flash`, `qwen3.7-max`, `qwen3.7-plus`, `qwen3.6-plus` |
| `google` (0) | *(none — Go carries no Gemini models)* |
| `chat_completions` (15) | `glm-5.3-flash`, `glm-5.3`, `glm-5.2`, `glm-5.1`, `kimi-k3`, `kimi-k2.7-code`, `kimi-k2.6`, `longcat-2.0`, `deepseek-v4-pro`, `deepseek-v4-flash`, `deepseek-v4-flash-vision-exp`, `mimo-v2.5`, `mimo-v2.5-pro`, `hy4-preview`, `hy3` |

**The `minimax-*` family is the divergence proof**: `chat_completions` on Zen, `messages`
on Go. Any implementation keyed on model ID alone is wrong.

### 3.3 OpenCode — table-vs-live reconciliation

Ten IDs are live but absent from the docs tables; each is resolved correctly by the prefix
heuristic in Step 1.3:

| Gateway | Model | Heuristic result |
|---|---|---|
| Zen | `claude-sonnet-4` | `messages` (prefix `claude-`) ✓ |
| Zen | `deepseek-v4-flash-free` | `chat_completions` (default) ✓ |
| Zen | `laguna-s-2.1-free` | `chat_completions` (default) ✓ — **empirically confirmed** 200 on `/chat/completions` |
| Go | `kimi-k2.5`, `glm-5`, `mimo-v2-pro`, `mimo-v2-omni`, `hy3-preview` | `chat_completions` (default) ✓ |
| Go | `qwen3.5-plus` | `messages` (prefix `qwen`, Go) ✓ |
| Go | `grok-4.5` | `responses` (prefix `grok-`) ✓ |

Conversely, Zen's docs table lists `qwen3.7-max` and `qwen3.7-plus`, which are **absent from
the live listing**. Keep them in the route table; **exclude them from `StaticOpencodeZen`**.

### 3.4 Hugging Face — listing metadata (`https://router.huggingface.co/v1`)

`GET /v1/models`, unauthenticated, 2026-08-29: 136 models, 94,116 bytes.

| Metric | Value |
|---|---|
| `output_modalities == ["text"]` | all 136 |
| `input_modalities == ["text"]` | 96 (the other 40 are `["image","text"]`) |
| Provider offerings (model × provider) | 317, **all** `status:"live"` |
| Offerings with `supports_tools:true` | 220 |
| Offerings with `is_free:true` | **0** |
| Text→text models with ≥1 live tool-capable provider | 76 |
| Text→text models served by ≥2 providers | 58 of 96 |

Per-offering fields: `provider`, `status`, `context_length`, `pricing{input,output}`,
`is_free`, `supports_tools`, `supports_structured_output`, `first_token_latency_ms`,
`throughput`, `is_model_author`. Query parameters on `/v1/models` are **ignored** —
filtering is entirely client-side.

### 3.5 Kilo — listing metadata (`https://api.kilo.ai/api/gateway`)

`GET /models`, unauthenticated, 2026-08-29: 366 models, 411,089 bytes (byte-identical to
the `kilo.ai/api/openrouter` alias).

| Metric | Value |
|---|---|
| Models listed | 366 |
| Text-in / text-out | 145 |
| …of which tool-capable | 106 |
| `isFree: true` | 20 (12 text + tools) |
| `mayTrainOnYourPrompts: true` | 25 |
| `supported_parameters` ⊇ `tools` | 301 |
| … ⊇ `tool_choice` | 279 |
| … ⊇ `reasoning` | 244 |
| … ⊇ `reasoning_effort` | 105 |
| … ⊇ `response_format` | 294 |

Per-model fields: OpenRouter shape (`id`, `name`, `architecture{input_modalities,
output_modalities,tokenizer}`, `top_provider`, `pricing{prompt,completion,…}` as **strings**,
`context_length`) plus Kilo extensions `supported_parameters[]`, `isFree`,
`mayTrainOnYourPrompts`, `preferredIndex`, `opencode{ai_sdk_provider,family,prompt}`.

**Two traps recorded here so implementation does not rediscover them:**

1. `pricing.completion` is a **string**, and is `"-1"` for the variable-priced
   `kilo-auto/{frontier,balanced,efficient}` tiers. A naive ascending sort puts `-1` before
   `0` and ranks the most expensive tiers as if they were free.
2. Kilo model IDs may **legitimately end in `:free`** (`tencent/hy3:free`,
   `minimax/minimax-m3:free`). Unlike Hugging Face, Kilo has **no** policy suffix, so
   **nothing may be stripped at the colon**. Applying Hugging Face's suffix logic to Kilo
   would corrupt 20 model IDs.

### 3.6 Verified behaviour all three families depend on

| Behaviour | OpenCode | Hugging Face | Kilo |
|---|---|---|---|
| Auth scheme | `Bearer` only; `x-api-key` rejected | `Bearer hf_***` | `Bearer $KILO_API_KEY` |
| Env var (evidenced) | `OPENCODE_API_KEY` | `HF_TOKEN` | `KILO_API_KEY` |
| Model listing needs credential | No | No | No |
| Credential-free generation | Yes (free models) | **No** | Yes (`kilo-auto/free`) |
| Credential-free **tool call** | No (`"Endpoint is unavailable"`) | No | **Yes** |
| Wrong endpoint | HTTP 500 | n/a | n/a (gateway translates) |
| `429` carries `Retry-After` | **No** (measured) | unknown (Q5) | unknown (Q9) |
| Chat reasoning field | `reasoning_content` | undocumented | `reasoning` + `reasoning_details[]` |
| Error envelope | `{"type":"error","error":{…}}` | `{"error":"<string>"}` | `{"error":{"code","message"},"error_type"}` |

---

## 4. Type and Signature Definitions

### 4.1 Shared — Chat Completions (Phase 2)

```go
type chatCompletionsOpts struct {
	Tool            *Tool  // nil = no tools
	ForceTool       bool   // false = send tools without tool_choice
	ReasoningEffort string // "" = omit reasoning_effort
}

func itemsToChatMessages(items []Item) []map[string]any
func chatCompletionsBody(model string, maxTokens int, input []Item, o chatCompletionsOpts) map[string]any
func decodeChatCompletionsResponse(body io.Reader) (*Response, error)
func classifyHTTPStatus(provider string, resp *http.Response) error
```

`ForceTool` is separable from `Tool` because Kilo gates forced `tool_choice` on
`supported_parameters` (§3.5): 301 models accept `tools` but only 279 accept `tool_choice`.

### 4.2 OpenCode (Phases 1, 3, 4)

```go
type OpencodeRoute string
const (
	OpencodeRouteResponses       OpencodeRoute = "responses"
	OpencodeRouteMessages        OpencodeRoute = "messages"
	OpencodeRouteChatCompletions OpencodeRoute = "chat_completions"
	OpencodeRouteGoogle          OpencodeRoute = "google"
)

type OpencodeProvider struct {
	gateway, apiKey, model, baseURL string
	client                          *http.Client
	maxTokens, thinkingBudget       int
	reasoningEffort                 string
	route                           OpencodeRoute // resolved once at construction
}

func NewOpencode(gateway, apiKey, model string, opts ...ProviderOption) (*OpencodeProvider, error)
func WithOpencodeRoute(route OpencodeRoute) ProviderOption
func (p *OpencodeProvider) Route() OpencodeRoute
```

### 4.3 Hugging Face (Phase 5)

```go
type HuggingFaceProvider struct {
	apiKey, model, baseURL string
	client                 *http.Client
	maxTokens              int
	reasoningEffort        string
}

func NewHuggingFace(apiKey, model string, opts ...ProviderOption) (*HuggingFaceProvider, error)
func splitHuggingFaceModelPolicy(id string) (base, policy string)
func isUsableHuggingFaceModel(id string) bool
func RankHuggingFaceModel(m string) int   // weak name-based fallback only
```

### 4.4 Kilo (Phase 6)

```go
type KiloProvider struct {
	apiKey, model, baseURL string
	client                 *http.Client
	maxTokens              int
	reasoningEffort        string
	// caps is the model's supported_parameters set. nil means "unknown".
	caps map[string]struct{}
}

func NewKilo(apiKey, model string, opts ...ProviderOption) (*KiloProvider, error)
func WithKiloCapabilities(params ...string) ProviderOption
func KiloModelCapabilities(ctx context.Context, apiKey, model string, opts ...ProviderOption) ([]string, error)
func isUsableKiloModel(id string) bool
func RankKiloModel(m string) int      // weak name-based fallback only
func kiloPriceRank(s string) float64
```

### 4.5 Naming constraint that is not negotiable

`Name()` **must** return the provider identifier, because `TestNewProvider`
(`provider_test.go:94-110`) asserts `NewProvider(p, …).Name() == p` for every registered
name, and this plan adds all four to that list. So `OpencodeProvider.Name()` returns
`p.gateway`, `HuggingFaceProvider.Name()` returns `ProviderHuggingFace`, and
`KiloProvider.Name()` returns `ProviderKilo`.

### 4.6 Interfaces

All four providers implement: `Provider`, `ToolProvider`, `ThinkingProvider`,
`ThinkingToolProvider`, `ItemProvider`, `ItemToolProvider`, `ItemThinkingProvider`,
`ItemThinkingToolProvider`, `ModelDiscoverer`.

**None implements `Continuer`** — OpenCode because chaining was measured to return
HTTP 400; Hugging Face and Kilo because Chat Completions is stateless by construction.

---

## 5. Phase Sequencing Overview

Seven phases. Each leaves the tree compiling and green, and ends in one commit. From
Phase 3 onward, each phase delivers one **complete, registered, usable** provider.

| Phase | Deliverable | New files | Modified files |
|---|---|---|---|
| 1 | Constants (all 4), `wireShapesProbedOn` pin, OpenCode route table + resolution + option | `opencode_route.go`, `opencode_route_test.go` | `constants.go`, `options.go` |
| 2 | Shared `classifyHTTPStatus` + Chat Completions primitive | `chatcompletions.go`, `chatcompletions_test.go` | `http_helpers.go` |
| 3 | `OpencodeProvider` core, all four routes | `opencode.go`, `opencode_test.go` | — |
| 4 | OpenCode discovery, catalogs, registration | — | `discovery.go`, `models_catalog.go`, `provider.go`, `opencode.go`, + 4 test files |
| 5 | Hugging Face provider, discovery, catalog, registration | `huggingface.go`, `huggingface_test.go` | `discovery.go`, `models_catalog.go`, `provider.go`, + 4 test files |
| 6 | Kilo provider, discovery, catalog, registration, `WithKiloCapabilities` | `kilo.go`, `kilo_test.go` | `discovery.go`, `models_catalog.go`, `provider.go`, `options.go`, + 4 test files |
| 7 | `live_gateways` shape-assertion suite, open questions, docs | `live_gateways_test.go` | MADR, this plan |

Dependencies: 1 and 2 are independent of each other; both precede 3. 3 precedes 4. 2
precedes 5 and 6 (they consume the shared primitive). 5 and 6 are independent of each other
and of 4. 7 is last.

---

## Phase 1 — Constants, Route Table, Route Resolution

**Goal:** all four provider constants, the OpenCode wire-shape pin, plus HTTP-free routing
logic that can be exhaustively unit-tested before any network code exists.

### Step 1.0 — the `wireShapesProbedOn` pin (pattern used by Phases 1, 5 and 6)

Every route table, catalog and decoder field name in this change was measured on a specific
date against a **remote, continuously-deployed** gateway. None is version-negotiated, and
none of the three gateways exposes a version endpoint to compare against — verified
2026-08-29: Kilo's `/version`, `/api/version` and `/health` all return `404` and
`/api/gateway/version` returns `405`. The probe date is therefore the only pin available,
and it is documentation with a compiler-checked home, not runtime logic. See MADR
§"Pinning what was probed" and the precedent in `magic-cli-remote/internal/provider/{kilo,opencode}/version.go`.

Each gateway file declares one constant beside its base URL. **All three files are
`package llmprovider`, so the identifiers must differ** — `wireShapesProbedOnOpencode`,
`wireShapesProbedOnHuggingFace`, `wireShapesProbedOnKilo`. The exact text below goes in
`opencode_route.go` in this phase; Phases 5 and 6 add theirs in Steps 5.1 and 6.1:

```go
// wireShapesProbedOnOpencode is the date every wire shape in this file was measured
// against the live gateway: the per-gateway route tables, which routes reject
// which models (a mismatch returns HTTP 500, not a typed error), and the
// response envelopes each route returns.
//
// The OpenCode gateways are remote and continuously deployed. They expose no
// version endpoint, so there is nothing to compare this against at runtime and
// nothing warns when it goes stale — unlike a local engine, which can report a
// version on boot (see magic-cli-remote/internal/provider/opencode/version.go).
// Re-validate with: go test -tags live_gateways ./llmprovider/ -run Live
const wireShapesProbedOnOpencode = "2026-08-28"
```

Phase 5 (`huggingface.go`) and Phase 6 (`kilo.go`) each declare their own with
`"2026-08-29"` and a doc comment naming what *that* file measured — for Hugging Face the
listing metadata fields and the `/v1/responses` HTTP-200-on-failure behaviour; for Kilo the
`reasoning` spelling, `supported_parameters`, and the string `"-1"` pricing.

> ~~**`unused` note.** … referenced from the `live_gateways` suite …~~
>
> **Amended 2026-08-29 — deviation D2.** That mitigation does **not** work:
> `.golangci.yml` configures no `build-tags`, so `//go:build live_gateways` files are never
> analysed and a reference from the tagged suite is invisible to `unused`. Instead each
> constant is referenced from a **normal, untagged** test — `TestWireShapesProbedOn`
> (Step 1.4) — which asserts every `wireShapesProbedOn*` parses as a `YYYY-MM-DD` date and
> is not in the future. `.golangci.yml` sets `run: tests: true`, so this counts as a real
> use, and unlike a `//nolint` it also catches a typo'd date. The `live_gateways` suite
> still includes the value in its failure messages, for self-documenting drift reports —
> that is now a readability benefit rather than the lint mitigation.

### Step 1.1

### Step 1.1 — `llmprovider/constants.go` (modify)

Extend the canonical identifier block at `constants.go:3-9`. All four land here so later
phases need not reopen this file:

```go
// Canonical provider identifiers.
const (
	ProviderGemini = "gemini"
	ProviderOpenAI = "openai"
	ProviderClaude = "claude"
	ProviderGrok   = "grok"
	// ProviderOpencodeZen is the OpenCode Zen gateway (pay-as-you-go).
	ProviderOpencodeZen = "opencode-zen"
	// ProviderOpencodeGo is the OpenCode Go gateway (subscription).
	ProviderOpencodeGo = "opencode-go"
	// ProviderHuggingFace is the Hugging Face Inference Providers router.
	ProviderHuggingFace = "huggingface"
	// ProviderKilo is the Kilo Gateway (the API behind the Kilo Code agent).
	// models.dev registers this gateway as "kilo"; this package uses the product
	// name a caller configuring it will recognise. See MADR revision 4.
	ProviderKilo = "kilo"
)
```

~~Add to the JSON field-name block (`constants.go:11-31`) seven new constants:
`jsonKeyToolCalls`, `jsonKeyToolChoice`, `jsonKeyReasoning`, `jsonKeyReasoningContent`,
`jsonKeyReasoningEffort`, `jsonKeyMaxTokens`, `jsonRoleTool`.~~

> **Amended 2026-08-29 — deviation D1.** Declaring these here fails the Phase 1 `make lint`
> gate: `unused` flags every one, because the plan's own code blocks first reference them in
> Phases 2, 3 and 6. Two of them are never referenced in **any** phase. Each constant is
> therefore declared in the phase that first uses it, still in `constants.go`:
>
> | Constant | Declared in | First use |
> |---|---|---|
> | `jsonRoleTool`, `jsonKeyMaxTokens`, `jsonKeyToolChoice`, `jsonKeyReasoningEffort` | **Step 2.1** | `chatcompletions.go` — `itemsToChatMessages` and `chatCompletionsBody` |
> | `jsonKeyReasoning` | **Step 3.1** | `opencode.go` `responsesBody` |
> | ~~`jsonKeyToolCalls`~~, ~~`jsonKeyReasoningContent`~~ | **dropped** | never — `decodeChatCompletionsResponse` reads those fields via struct tags, not constants |
>
> Existing entries `jsonKeyModel`, `jsonKeyMessages`, `jsonKeyContent`, `jsonKeyRole`,
> `jsonKeyTools`, `jsonKeyType`, `jsonKeyFunction`, `jsonKeyName`, `jsonKeyDescription`,
> `jsonKeyParameters`, `jsonRoleAssistant`, `jsonRoleUser`, `jsonKeyEnabled` are **reused,
> not duplicated**. Step 1.1 therefore adds only the four provider identifiers.

### Step 1.2 — `llmprovider/options.go` (modify)

One additive field on `ProviderConfig` (after `ReasoningEffort`, `options.go:35`) and one
option constructor. This is the only change to a shared config file in the whole plan; no
existing field, default, or behaviour changes.

```go
	// OpencodeRoute overrides the wire format the OpenCode gateway providers use
	// for the configured model. Empty means "resolve from the built-in route
	// table, then the per-gateway prefix heuristic". Set this when a model is
	// newer than the table. Ignored by all other providers.
	OpencodeRoute OpencodeRoute
```

```go
// WithOpencodeRoute pins the wire format used by the OpenCode Zen/Go providers,
// overriding the built-in route table. Use it when the gateway adds a model
// before this package's table is updated; sending a model to the wrong route
// fails with an opaque HTTP 500. Ignored by all other providers.
func WithOpencodeRoute(route OpencodeRoute) ProviderOption {
	return func(cfg *ProviderConfig) { cfg.OpencodeRoute = route }
}
```

`ApplyOptions` (`options.go:80-89`) needs **no** change: the zero value `""` is the correct
"unset" sentinel.

### Step 1.3 — `llmprovider/opencode_route.go` (new file)

```go
package llmprovider

import (
	"fmt"
	"strings"
)

// Default gateway base URLs. Both gateways share one credential and one auth
// scheme (Authorization: Bearer); they differ only in base URL, catalog, and
// per-model routing.
const (
	opencodeZenBaseURL = "https://opencode.ai/zen/v1"
	opencodeGoBaseURL  = "https://opencode.ai/zen/go/v1"
)

// OpencodeRoute identifies which wire format the OpenCode gateway expects for a
// given model. OpenCode Zen and Go are multi-protocol gateways: they do not
// normalize to a single request shape, and sending a model to the wrong route
// fails with an opaque HTTP 500 rather than a typed error.
type OpencodeRoute string

const (
	OpencodeRouteResponses       OpencodeRoute = "responses"
	OpencodeRouteMessages        OpencodeRoute = "messages"
	OpencodeRouteChatCompletions OpencodeRoute = "chat_completions"
	OpencodeRouteGoogle          OpencodeRoute = "google"
)

// path returns the request path suffix for the route. The Google route is
// model-scoped, so it takes the model id.
func (r OpencodeRoute) path(model string) string {
	switch r {
	case OpencodeRouteResponses:
		return "/responses"
	case OpencodeRouteMessages:
		return "/messages"
	case OpencodeRouteGoogle:
		return fmt.Sprintf("/models/%s:generateContent", model)
	case OpencodeRouteChatCompletions:
		return "/chat/completions"
	default:
		return "/chat/completions"
	}
}

// valid reports whether r is one of the four known routes.
func (r OpencodeRoute) valid() bool {
	switch r {
	case OpencodeRouteResponses, OpencodeRouteMessages,
		OpencodeRouteChatCompletions, OpencodeRouteGoogle:
		return true
	default:
		return false
	}
}

// opencodeBaseURL returns the default base URL for a gateway.
func opencodeBaseURL(gateway string) (string, error) {
	switch gateway {
	case ProviderOpencodeZen:
		return opencodeZenBaseURL, nil
	case ProviderOpencodeGo:
		return opencodeGoBaseURL, nil
	default:
		return "", fmt.Errorf("unsupported opencode gateway: %s", gateway)
	}
}

// opencodeRouteTable maps gateway -> model id -> wire format, transcribed from the
// published endpoint tables at https://opencode.ai/docs/zen/ and
// https://opencode.ai/docs/go/ (retrieved 2026-08-28).
//
// Routing is per-gateway, not per-model: the minimax-* family takes
// chat_completions on Zen and messages on Go. Reconcile this table against both
// docs pages when either gateway announces new models.
//
//nolint:goconst // model IDs are intentionally repeated across gateways and tests
var opencodeRouteTable = map[string]map[string]OpencodeRoute{
	ProviderOpencodeZen: { /* all 63 rows from §3.1, grouped by route, one comment per group */ },
	ProviderOpencodeGo:  { /* all 26 rows from §3.2, grouped by route, one comment per group */ },
}

// opencodeHeuristicRoute infers a route from the model id prefix when the table
// has no entry. Both gateways send gpt/grok/muse to responses and qwen to
// messages; they differ on minimax (Go only) and gemini/claude (Zen only).
// Chat Completions is the default because it is the largest bucket on both.
func opencodeHeuristicRoute(gateway, model string) OpencodeRoute {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "gpt-"), strings.HasPrefix(m, "grok-"),
		strings.HasPrefix(m, "muse-"):
		return OpencodeRouteResponses
	case strings.HasPrefix(m, "qwen"):
		return OpencodeRouteMessages
	}
	if gateway == ProviderOpencodeZen {
		switch {
		case strings.HasPrefix(m, "claude-"):
			return OpencodeRouteMessages
		case strings.HasPrefix(m, "gemini-"):
			return OpencodeRouteGoogle
		}
	}
	if gateway == ProviderOpencodeGo && strings.HasPrefix(m, "minimax-") {
		return OpencodeRouteMessages
	}
	return OpencodeRouteChatCompletions
}

// resolveOpencodeRoute picks the wire format for (gateway, model), honouring an
// explicit override first, then the published table, then the prefix heuristic.
func resolveOpencodeRoute(gateway, model string, override OpencodeRoute) (OpencodeRoute, error) {
	if override != "" {
		if !override.valid() {
			return "", fmt.Errorf("%w: unknown opencode route %q", ErrInvalidRequest, override)
		}
		return override, nil
	}
	if byModel, ok := opencodeRouteTable[gateway]; ok {
		if r, ok := byModel[strings.ToLower(strings.TrimSpace(model))]; ok {
			return r, nil
		}
	}
	return opencodeHeuristicRoute(gateway, model), nil
}
```

> The two table literals are elided above only for readability of this document. They are
> transcribed **in full** from §3.1 and §3.2 — 63 and 26 entries — grouped by route with a
> comment per group naming the AI SDK package, exactly as the docs tables present them.

### Step 1.4 — `llmprovider/opencode_route_test.go` (new file)

Table-driven, in the style of `models_catalog_test.go:158-179`:

1. **`TestOpencodeRoute_Table`** — one case per bucket per gateway, with the divergence as
   its own named case: `{zen, "minimax-m3", chatCompletions}` **and**
   `{go, "minimax-m3", messages}`. Plus zen `gpt-5.5`/`grok-4.6`/`muse-spark-1.2` →
   responses; zen `claude-opus-4-5`/`qwen3.6-plus` → messages; zen `gemini-3.7-flash` →
   google; zen `deepseek-v4-pro`/`glm-5.2`/`kimi-k3`/`big-pickle` → chat_completions; go
   `gpt-5.6-luna` → responses; go `glm-5.3` → chat_completions.
2. **`TestOpencodeRoute_Heuristic`** — the exact ten live-but-untabled IDs from §3.3 with
   their documented answers. This is the drift regression guard.
3. **`TestOpencodeRoute_Override`** — override beats table and heuristic; an invalid
   override returns an error wrapping `ErrInvalidRequest`.
4. **`TestOpencodeRoute_Path`** — `OpencodeRouteGoogle.path("gemini-3.7-flash")` ==
   `"/models/gemini-3.7-flash:generateContent"`; other three return fixed suffixes.
5. **`TestOpencodeBaseURL`** — both gateways return documented URLs; unknown → error.
6. **`TestOpencodeRouteTable_NoUnknownRoutes`** — every value in the table satisfies
   `r.valid()`, so a typo cannot ship.
7. **`TestProviderConstants_Distinct`** — the four new identifiers are pairwise distinct and
   distinct from the four existing ones (guards a copy-paste error in Step 1.1).
8. **`TestWireShapesProbedOn`** *(added by deviation D2)* — asserts every
   `wireShapesProbedOn*` constant parses as `YYYY-MM-DD` via `time.Parse` and is not in the
   future. This is what satisfies `unused` for those constants (a normal untagged test, so
   `run: tests: true` picks it up), and it catches a typo'd date. As Phases 5 and 6 add
   their constants, each is added to this test's table in the same commit.

### Step 1.5 — Verification

```bash
gofmt -l llmprovider/constants.go llmprovider/options.go llmprovider/opencode_route.go llmprovider/opencode_route_test.go
go vet ./... && go test ./llmprovider/... -run 'TestOpencodeRoute|TestOpencodeBaseURL|TestProviderConstants' -v
make lint && make test
```

**Acceptance:** all seven tests pass; no other test changes behaviour. **Commit.**

---

## Phase 2 — Shared Primitives: Error Classifier + Chat Completions

**Goal:** the two net-new wire functions plus the one extraction, written once for three
consumers.

### Step 2.1 — `llmprovider/http_helpers.go` (modify, additive)

**First, `constants.go` (deviation D1).** Add the two JSON-key constants this phase's
`chatcompletions.go` uses, to the field-name block:

```go
	jsonKeyMaxTokens       = "max_tokens"
	jsonKeyToolChoice      = "tool_choice"
	jsonKeyReasoningEffort = "reasoning_effort"
	jsonRoleTool           = "tool"
```

> **Corrected 2026-08-29.** The D1 entry first assigned `jsonKeyToolChoice` and
> `jsonKeyReasoningEffort` to Step 6.1. That was wrong: `chatCompletionsBody` in this phase
> uses both, so they must be declared here or Phase 2 does not compile. Phase 6 reuses them
> as capability keys in `supports()` but is not their first use. The resolution principle
> — declare at first use — is unchanged.

Then `http_helpers.go`: add `"fmt"` to the import block (`http_helpers.go:3-9`, which currently lacks it), then
append:

```go
// classifyHTTPStatus maps a non-200 response to the package's typed sentinels.
// It is the shared form of the switch duplicated in openai.go, claude.go,
// gemini.go and grok.go; those remain untouched, and adopting this helper there
// is deliberately out of scope. Returns nil for 200 OK.
//
// provider names the caller for the error string — for a multi-route gateway,
// pass "gateway/route" so a misroute is diagnosable from the error alone.
//
// Note: Kilo documents HTTP 402 "insufficient balance". It falls to
// ErrInvalidRequest, which is the correct retry behaviour (retrying will not add
// balance) even though the message reads as a client error. See MADR revision 4.
func classifyHTTPStatus(provider string, resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return &RateLimitError{
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Status:     resp.StatusCode,
		}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s HTTP %d", ErrAuthFailure, provider, resp.StatusCode)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: %s HTTP %d", ErrProviderUnavailable, provider, resp.StatusCode)
	default:
		return fmt.Errorf("%w: %s HTTP %d", ErrInvalidRequest, provider, resp.StatusCode)
	}
}
```

### Step 2.2 — `llmprovider/chatcompletions.go` (new file)

Provider-neutral by name and by content. **No `opencode`, `huggingface` or `kilo` prefix
appears in this file's identifiers.**

```go
package llmprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// chatCompletionsOpts carries the per-gateway variations of a Chat Completions
// request. Zero values omit the corresponding field entirely.
type chatCompletionsOpts struct {
	// Tool, when non-nil, is offered to the model.
	Tool *Tool
	// ForceTool sends tool_choice pinning Tool. Kilo gates tool_choice on the
	// model's supported_parameters, so it is separable from offering the tool.
	ForceTool bool
	// ReasoningEffort, when non-empty, is sent as reasoning_effort. OpenCode's
	// chat route has no portable reasoning parameter and always leaves this "".
	ReasoningEffort string
}

// itemsToChatMessages converts canonical items to OpenAI Chat Completions
// messages. Tool results become role:"tool" messages keyed by tool_call_id,
// which is the Chat Completions equivalent of the Responses API's
// function_call_output item.
func itemsToChatMessages(items []Item) []map[string]any {
	var messages []map[string]any
	for _, item := range items {
		switch v := item.(type) {
		case MessageItem:
			role := v.Role
			if role == "" {
				role = jsonRoleUser
			}
			messages = append(messages, map[string]any{
				jsonKeyRole: role, jsonKeyContent: v.Text,
			})
		case FunctionCallOutputItem:
			messages = append(messages, map[string]any{
				jsonKeyRole: jsonRoleTool, "tool_call_id": v.CallID,
				jsonKeyContent: v.Output,
			})
		}
	}
	return messages
}

// chatCompletionsBody builds an OpenAI Chat Completions request body shared by
// every gateway in this package that speaks the format.
func chatCompletionsBody(model string, maxTokens int, input []Item, o chatCompletionsOpts) map[string]any {
	body := map[string]any{
		jsonKeyModel:     model,
		jsonKeyMessages:  itemsToChatMessages(input),
		jsonKeyMaxTokens: maxTokens,
	}
	if o.Tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			jsonKeyType: jsonKeyFunction,
			jsonKeyFunction: map[string]any{
				jsonKeyName:        o.Tool.Name,
				jsonKeyDescription: o.Tool.Description,
				jsonKeyParameters:  o.Tool.Schema,
			},
		}}
		if o.ForceTool {
			body[jsonKeyToolChoice] = map[string]any{
				jsonKeyType:     jsonKeyFunction,
				jsonKeyFunction: map[string]any{jsonKeyName: o.Tool.Name},
			}
		}
	}
	if o.ReasoningEffort != "" {
		body[jsonKeyReasoningEffort] = o.ReasoningEffort
	}
	return body
}

// decodeChatCompletionsResponse decodes an OpenAI Chat Completions envelope into
// a canonical Response.
//
// Reasoning has two competing vendor spellings, both undocumented, both measured
// 2026-08-28/29:
//
//	message.reasoning_content  — OpenCode Zen (verified on big-pickle)
//	message.reasoning          — Kilo Gateway, the OpenRouter convention
//
// Both are accepted; reasoning_content wins when both are present. Kilo also
// sends message.reasoning_details[] ({type:"reasoning.text", text}), a structured
// restatement of the same trace; it is deliberately NOT decoded, because
// ReasoningItem carries a single Text field (item.go:44-51) and parsing both
// would create two sources of truth for one value.
//
// Absent reasoning is normal, never an error.
func decodeChatCompletionsResponse(body io.Reader) (*Response, error) {
	var raw struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("chat completions: response contained no choices")
	}

	msg := raw.Choices[0].Message
	// The response id is not a resumable conversation handle on any gateway in
	// this package, so it is carried for logging only.
	result := &Response{ID: raw.ID}

	reasoning := msg.ReasoningContent
	if reasoning == "" {
		reasoning = msg.Reasoning
	}
	if strings.TrimSpace(reasoning) != "" {
		result.Output = append(result.Output, ReasoningItem{Text: reasoning})
	}
	if msg.Content != "" {
		role := msg.Role
		if role == "" {
			role = jsonRoleAssistant
		}
		result.Output = append(result.Output, MessageItem{Role: role, Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		result.Output = append(result.Output, FunctionCallItem{
			CallID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
		})
	}

	if len(result.Output) == 0 {
		return nil, fmt.Errorf("chat completions: response contained no usable content")
	}
	return result, nil
}
```

### Step 2.3 — `llmprovider/chatcompletions_test.go` (new file)

Fixtures are **real measured bodies**, cited in comments.

1. **`TestDecodeChatCompletions_Message`** — plain content → one `MessageItem`;
   `OutputText()` matches; `ID` preserved.
2. **`TestDecodeChatCompletions_ReasoningFieldNames`** — table-driven, the core new test:

   | Case | Fixture source | Expect |
   |---|---|---|
   | `reasoning_content` only | OpenCode `big-pickle`, measured | 1 `ReasoningItem` with that text, then `MessageItem` |
   | `reasoning` only | Kilo `kilo-auto/free`, measured | 1 `ReasoningItem` with that text |
   | both present | synthetic | exactly 1 `ReasoningItem`, text from `reasoning_content` |
   | `reasoning_details` present, both strings absent | Kilo shape | **no** `ReasoningItem`, no error |
   | neither | synthetic | no `ReasoningItem`, no error |

3. **`TestDecodeChatCompletions_ToolCalls`** — Kilo's measured
   `{"id":"chatcmpl-tool-…","type":"function","function":{"name":"get_weather","arguments":"{\"city\": \"Paris\"}"}}`
   → `FunctionCallItem` with `CallID`/`Name`/`Arguments`.
4. **`TestDecodeChatCompletions_Empty`** — no choices → error; choices present but content,
   both reasoning fields, and tool_calls all empty → error.
5. **`TestItemsToChatMessages`** — `MessageItem` with empty role defaults to `user`;
   `FunctionCallOutputItem` → `{"role":"tool","tool_call_id":…,"content":…}`; unhandled
   item types are skipped without panicking.
6. **`TestChatCompletionsBody`** — table over `chatCompletionsOpts`: no tool → no
   `tools`/`tool_choice` keys; tool without `ForceTool` → `tools` present, `tool_choice`
   **absent**; tool with `ForceTool` → both; empty `ReasoningEffort` → key absent;
   `"xhigh"` → key present with that value.
7. **`TestClassifyHTTPStatus`** — 200 → nil; 429 without `Retry-After` → `*RateLimitError`,
   `errors.Is(…, ErrRateLimited)`, `RetryAfter == 0`; 429 **with** `Retry-After: 7` →
   `7*time.Second`; 401 and 403 → `ErrAuthFailure`; **402 → `ErrInvalidRequest`** (pinning
   the documented Kilo behaviour); 500 and 503 → `ErrProviderUnavailable`; 400 and 404 →
   `ErrInvalidRequest`.

### Step 2.4 — Verification

```bash
gofmt -l llmprovider/http_helpers.go llmprovider/chatcompletions.go llmprovider/chatcompletions_test.go
go vet ./... && go test ./llmprovider/... -run 'TestDecodeChatCompletions|TestItemsToChatMessages|TestChatCompletionsBody|TestClassifyHTTPStatus' -v
grep -rn 'opencodeItemsToChatMessages\|opencode_chat.go' llmprovider/ ; echo "^ must be empty"
make lint && make test
```

**Acceptance:** all seven tests pass, including all five reasoning-spelling cases; the
`grep` finds nothing. **Commit.**

---

## Phase 3 — `OpencodeProvider` Core

**Goal:** the four-route provider, reusing three existing converters, three existing
decoders, and the Phase 2 primitive.

### Step 3.1 — `llmprovider/opencode.go` (new file)

**(a) `constants.go` (deviation D1).** Add the constant this phase's `responsesBody` uses:

```go
	jsonKeyReasoning = "reasoning"
```

**(b) Type + doc comment** recording two measured facts:

```go
// OpencodeProvider implements Provider against the OpenCode Zen and OpenCode Go
// AI gateways. Both are multi-protocol: the gateway dispatches each model to one
// of four upstream wire formats and does NOT normalize them, so this provider
// selects the request shape, path and decoder per model via resolveOpencodeRoute.
// Sending a model to the wrong route fails with an opaque HTTP 500, which
// classifies as the retryable ErrProviderUnavailable — use WithOpencodeRoute to
// override the table when the gateway adds a model.
//
// OpencodeProvider does NOT implement Continuer. The gateway rejects
// previous_response_id with HTTP 400 "referenced response not found or expired"
// (measured 2026-08-28 against a response id seconds old). This is a property of
// the gateway, not a TODO. Callers must replay prior items on every call.
```

**(b) Constructor** — `apiKey` required (matching `NewClaude`/`NewGrok`); route resolved
once so a misroute is a construction-time fact:

```go
func NewOpencode(gateway, apiKey, model string, opts ...ProviderOption) (*OpencodeProvider, error) {
	defaultBase, err := opencodeBaseURL(gateway)
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, fmt.Errorf("opencode api key is required")
	}
	cfg := ApplyOptions(opts)
	baseURL := defaultBase
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	route, err := resolveOpencodeRoute(gateway, model, cfg.OpencodeRoute)
	if err != nil {
		return nil, err
	}
	return &OpencodeProvider{
		gateway: gateway, apiKey: apiKey, model: model, baseURL: baseURL,
		client: cfg.HTTPClient, maxTokens: cfg.MaxTokens,
		thinkingBudget: cfg.ThinkingBudget, reasoningEffort: cfg.ReasoningEffort,
		route: route,
	}, nil
}
```

**(c) `Name()` / `Route()`** — `Name()` returns `p.gateway` (§4.5).

**(d) String-shaped methods** — `Generate`, `GenerateThinking`, `GenerateWithTool`,
`GenerateWithToolThinking`, shaped exactly like `grok.go:47-92`; tool methods return the
first `FunctionCallItem`'s `Arguments` or `fmt.Errorf("opencode: no function call in response")`.

**(e) Item methods** — the four `GenerateItems*` variants delegating to
`doGenerateItems(ctx, input, tool, thinking)`. **No `prevResponseID` parameter**, because
`Continuer` is not implemented.

**(f) Per-route body builders** — three reuse existing converters; the fourth calls the
Phase 2 primitive:

```go
func (p *OpencodeProvider) responsesBody(input []Item, tool *Tool, thinking bool) map[string]any {
	body := map[string]any{
		jsonKeyModel:        p.model,
		jsonKeyInput:        itemsToInput(input), // reused from grok.go:120
		"max_output_tokens": p.maxTokens,
	}
	if tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			jsonKeyType: jsonKeyFunction, jsonKeyName: tool.Name,
			jsonKeyDescription: tool.Description, jsonKeyParameters: tool.Schema,
		}}
		body[jsonKeyToolChoice] = map[string]any{jsonKeyType: jsonKeyFunction, jsonKeyName: tool.Name}
	}
	if thinking {
		effort := p.reasoningEffort
		if effort == "" {
			effort = effortMedium
		}
		body[jsonKeyReasoning] = map[string]any{"effort": effort}
	}
	return body
}

func (p *OpencodeProvider) messagesBody(input []Item, tool *Tool, thinking bool) map[string]any {
	maxTokens := p.maxTokens
	body := map[string]any{
		jsonKeyModel:    p.model,
		jsonKeyMessages: claudeItemsToMessages(input), // reused from claude.go:138
	}
	if thinking {
		budget := p.thinkingBudget
		if budget <= 0 {
			budget = defaultClaudeThinkingBudget // claude.go:28
		}
		if maxTokens <= budget {
			maxTokens = budget + defaultClaudeThinkingBudget
		}
		body["thinking"] = map[string]any{jsonKeyType: jsonKeyEnabled, "budget_tokens": budget}
	}
	body[jsonKeyMaxTokens] = maxTokens
	if tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			jsonKeyName: tool.Name, jsonKeyDescription: tool.Description,
			"input_schema": tool.Schema,
		}}
		if thinking {
			// Extended thinking is incompatible with a forced tool_choice.
			body[jsonKeyToolChoice] = map[string]any{jsonKeyType: "auto"}
		} else {
			body[jsonKeyToolChoice] = map[string]any{jsonKeyType: "tool", jsonKeyName: tool.Name}
		}
	}
	return body
}

func (p *OpencodeProvider) googleBody(input []Item, tool *Tool, thinking bool) map[string]any {
	genCfg := map[string]any{"maxOutputTokens": p.maxTokens}
	if thinking {
		budget := p.thinkingBudget
		if budget <= 0 {
			budget = dynamicGeminiThinkingBudget // gemini.go:23
		}
		genCfg["thinkingConfig"] = map[string]any{"thinkingBudget": budget}
	}
	body := map[string]any{
		"contents":         geminiItemsToContents(input), // reused from gemini.go:133
		"generationConfig": genCfg,
	}
	if tool != nil {
		body[jsonKeyTools] = []map[string]any{{
			"functionDeclarations": []map[string]any{{
				jsonKeyName: tool.Name, jsonKeyDescription: tool.Description,
				jsonKeyParameters: tool.Schema,
			}},
		}}
		body["toolConfig"] = map[string]any{"functionCallingConfig": map[string]any{
			"mode": "ANY", "allowedFunctionNames": []string{tool.Name},
		}}
	}
	return body
}

// chatBody delegates to the shared primitive. OpenCode's chat route has no
// portable reasoning parameter across the DeepSeek/GLM/Kimi/MiniMax families
// routed there, so ReasoningEffort is deliberately left empty and the thinking
// path returns a plain generation. Asserted by TestOpencode_Thinking_PerRoute.
func (p *OpencodeProvider) chatBody(input []Item, tool *Tool) map[string]any {
	return chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
		Tool: tool, ForceTool: tool != nil,
	})
}
```

**(g) The single request driver:**

```go
func (p *OpencodeProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
	var body map[string]any
	switch p.route {
	case OpencodeRouteResponses:
		body = p.responsesBody(input, tool, thinking)
	case OpencodeRouteMessages:
		body = p.messagesBody(input, tool, thinking)
	case OpencodeRouteGoogle:
		body = p.googleBody(input, tool, thinking)
	case OpencodeRouteChatCompletions:
		body = p.chatBody(input, tool)
	default:
		return nil, fmt.Errorf("%w: unresolved opencode route for model %q", ErrInvalidRequest, p.model)
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("opencode: marshal request: %w", err)
	}

	url := p.baseURL + p.route.path(p.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// The gateway accepts only Authorization: Bearer on every route — it ignores
	// x-api-key and x-goog-api-key even for the Anthropic- and Google-shaped
	// routes (verified 2026-08-28). Key stays in a header, never the URL.
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// 1MB cap applied BEFORE the status check so error bodies are bounded too.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if err := classifyHTTPStatus(p.gateway+"/"+string(p.route), resp); err != nil {
		return nil, err
	}

	switch p.route {
	case OpencodeRouteResponses:
		return decodeResponsesAPIOutput(limitedBody) // http_helpers.go:23
	case OpencodeRouteMessages:
		return decodeClaudeResponse(limitedBody) // claude.go:239
	case OpencodeRouteGoogle:
		return decodeGeminiResponse(limitedBody) // gemini.go:239
	default:
		return decodeChatCompletionsResponse(limitedBody) // chatcompletions.go
	}
}
```

`DiscoverModels` is added in **Phase 4**, once `listOpencodeModels` exists.

### Step 3.2 — `llmprovider/opencode_test.go` (new file)

Reuses `captureServer` (`thinking_test.go:14`) for body assertions.

1. **`TestOpencode_RoutePaths`** — one server capturing `r.URL.Path`; the divergence
   asserted at the HTTP layer:

   | Gateway | Model | Expected path |
   |---|---|---|
   | zen | `gpt-5.5` | `/responses` |
   | zen | `claude-sonnet-5` | `/messages` |
   | zen | `gemini-3.7-flash` | `/models/gemini-3.7-flash:generateContent` |
   | zen | `deepseek-v4-pro` | `/chat/completions` |
   | zen | `minimax-m3` | `/chat/completions` |
   | go | `minimax-m3` | `/messages` |

2. **`TestOpencode_Generate_PerRoute`** — four subtests with matching fixtures: responses
   (measured Zen shape with `summary:[]` + `encrypted_content` → asserts `Output[0]` is
   `ReasoningItem` with `Text == ""`); messages (asserts `resp.ID == ""`); google;
   chat_completions (measured `big-pickle` body).
3. **`TestOpencode_KeyInHeader`** — all four routes: `Authorization == "Bearer test-key"`;
   no key in `r.URL.RawQuery`; and **`x-api-key` / `x-goog-api-key` absent** on the
   messages and google routes. Regression guard for MADR correction #5.
4. **`TestOpencode_ToolCall_PerRoute`** — forced tool call per route with per-route fixtures
   (`function_call` item; `tool_use` block; `functionCall` part; `tool_calls`).
5. **`TestOpencode_Thinking_PerRoute`** — captures and unmarshals the body: responses →
   `reasoning.effort`; messages → `thinking.budget_tokens` and `max_tokens > budget_tokens`;
   google → `generationConfig.thinkingConfig.thinkingBudget`; **chat_completions → no
   `reasoning_effort` key at all**.
6. **`TestOpencode_ErrorClassification`** — per route: 429 no `Retry-After` →
   `ErrRateLimited` with `RetryAfter == 0`; 401 → `ErrAuthFailure`; 500 →
   `ErrProviderUnavailable`; 400 → `ErrInvalidRequest`. Error string contains
   `opencode-zen/messages` etc.
7. **`TestOpencode_WithOpencodeRoute`** — override sends `gpt-5.5` to `/chat/completions`;
   `Route()` reports the override.
8. **`TestOpencode_ConstructorErrors`** — unknown gateway → error; empty key → error;
   invalid route override → error wrapping `ErrInvalidRequest`.
9. **`TestOpencode_NoContinuer`** — negative interface assertion with a message citing the
   measured 400.
10. **`TestOpencode_RetryOnRateLimit`** — `GenerateWithRetry` with 429-then-200 succeeds.

### Step 3.3 — Verification

```bash
gofmt -l llmprovider/opencode.go llmprovider/opencode_test.go
go vet ./... && go test ./llmprovider/... -run TestOpencode -v
make lint && make test
```

**Acceptance:** all ten tests pass, including both divergence assertions and the `Continuer`
negative. **Commit.**

---

## Phase 4 — OpenCode Discovery, Catalogs, Registration

### Step 4.1 — `llmprovider/discovery.go` (modify)

Add two cases to the `ListAvailableModels` switch (`discovery.go:24-37`), before `default`:

```go
	case ProviderOpencodeZen, ProviderOpencodeGo:
		return listOpencodeModels(ctx, strings.ToLower(providerName), apiKey, cfg)
```

Append the lister, following `listGrokModels` (`discovery.go:254-297`) with one deliberate
difference — the credential is optional:

```go
// listOpencodeModels fetches the gateway catalog and curates it. The OpenCode
// /models endpoint is PUBLIC — it answers 200 with no credentials (verified
// 2026-08-28) — so the Authorization header is sent only when a key is
// available, and an empty key is not an error.
//
// The listing carries no routing or capability metadata (every entry reports
// owned_by "opencode"), so route selection cannot be derived from it; see
// opencode_route.go.
func listOpencodeModels(ctx context.Context, gateway, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL, err := opencodeBaseURL(gateway)
	if err != nil {
		return nil, err
	}
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
	if err != nil {
		return StaticModels(gateway), nil
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(gateway), nil
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return StaticModels(gateway), nil
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(gateway), nil
	}
	var available []string
	for _, m := range result.Data {
		if isUsableOpencodeModel(m.ID) {
			available = append(available, m.ID)
		}
	}
	curated := curateFromCatalog(staticOpencodeCatalog(gateway), available,
		isUsableOpencodeModel, RankOpencodeModel)
	if len(curated) == 0 {
		return StaticModels(gateway), nil
	}
	return curated, nil
}
```

### Step 4.2 — `llmprovider/models_catalog.go` (modify)

Static catalogs — every ID confirmed present in the live listing on 2026-08-28.
`MaxListedModels` is **6** (`models_catalog.go:11`), so each list is exactly 6.

```go
	// StaticOpencodeZen: fast/cheap first across all four gateway routes.
	// Verified present in GET https://opencode.ai/zen/v1/models on 2026-08-28.
	// Deliberately excludes qwen3.7-max/qwen3.7-plus: they appear in the docs
	// endpoint table but NOT in the live listing.
	StaticOpencodeZen = []string{
		"gpt-5.4-nano",          // responses
		"gemini-3.5-flash-lite", // google
		"gpt-5.4-mini",          // responses
		"claude-haiku-4-5",      // messages
		"gemini-3.7-flash",      // google
		"kimi-k2.6",             // chat_completions
	}

	// StaticOpencodeGo: Go carries no Claude or Gemini models.
	// Verified present in GET https://opencode.ai/zen/go/v1/models on 2026-08-28.
	StaticOpencodeGo = []string{
		"glm-5.3-flash",     // chat_completions
		"qwen3.8-flash",     // messages
		"deepseek-v4-flash", // chat_completions
		"kimi-k2.6",         // chat_completions
		"gpt-5.6-luna",      // responses
		"grok-4.6",          // responses
	}
```

Two `StaticModels` cases (`models_catalog.go:97-110`), plus:

```go
// staticOpencodeCatalog returns the curation seed for a gateway (no copy; callers
// must not mutate).
func staticOpencodeCatalog(gateway string) []string {
	if gateway == ProviderOpencodeGo {
		return StaticOpencodeGo
	}
	return StaticOpencodeZen
}

// opencodeDenySubstrings reject non-text / non-production entries that appear in
// the gateway catalogs (e.g. deepseek-v4-flash-vision-exp, mimo-v2-omni,
// hy3-preview on OpenCode Go).
var opencodeDenySubstrings = []string{
	"vision", "image", "embed", "tts", "audio", "omni", "preview", "-exp",
}

// isUsableOpencodeModel filters gateway model IDs to production text models.
// Unlike the vendor filters it enforces no family prefix: the gateways
// deliberately aggregate many vendors under bare IDs.
func isUsableOpencodeModel(id string) bool {
	sm := strings.ToLower(strings.TrimSpace(id))
	if sm == "" {
		return false
	}
	for _, deny := range opencodeDenySubstrings {
		if strings.Contains(sm, deny) {
			return false
		}
	}
	return true
}

// RankOpencodeModel scores a gateway model for menu ordering. Higher is better.
// Prefers low-latency tiers for fast Git hook execution, penalizes heavy
// reasoning tiers, and demotes free models because the gateway rate-limits them
// aggressively (HTTP 429 FreeUsageLimitError).
func RankOpencodeModel(m string) int {
	sm := strings.ToLower(m)
	score := 0
	switch {
	case strings.Contains(sm, "nano"):
		score += 200
	case strings.Contains(sm, "lite"):
		score += 190
	case strings.Contains(sm, "flash"):
		score += 180
	case strings.Contains(sm, "mini"):
		score += 170
	case strings.Contains(sm, "haiku"):
		score += 160
	case strings.Contains(sm, "sonnet"):
		score += 90
	case strings.Contains(sm, "opus"), strings.Contains(sm, "-pro"),
		strings.Contains(sm, "-max"):
		score -= 300
	}
	if strings.HasSuffix(sm, "-free") {
		score -= 50 // free tier is rate-limited; usable but not a default
	}
	if strings.Contains(sm, "codex") {
		score -= 100 // code-completion specialisations, not general chat
	}
	return score
}
```

### Step 4.3 — `llmprovider/opencode.go` (modify — add `DiscoverModels`)

Mirrors `grok.go:213-233`, with one deliberate difference:

```go
// DiscoverModels returns curated gateway models, with a short health probe.
// Falls back to the static catalog. Each probe reconstructs the provider so the
// per-model route is resolved correctly — a single route cannot be assumed
// across candidates, and cloning p would send every candidate down the first
// model's wire format and 500 on most of them.
func (p *OpencodeProvider) DiscoverModels(ctx context.Context) ([]string, error) {
	listed, err := listOpencodeModels(ctx, p.gateway, p.apiKey, ProviderConfig{
		HTTPClient: p.client, BaseURL: p.baseURL,
	})
	if err != nil || len(listed) == 0 {
		listed = StaticModels(p.gateway)
	}
	healthy := probeGenerateHealth(ctx, listed, func(tCtx context.Context, modelID string) (string, error) {
		tp, err := NewOpencode(p.gateway, p.apiKey, modelID,
			WithHTTPClient(p.client), WithBaseURL(p.baseURL))
		if err != nil {
			return "", err
		}
		return tp.Generate(tCtx, "Respond with ONLY the word Hello")
	})
	if len(healthy) > 0 {
		return healthy, nil
	}
	return listed, nil
}
```

### Step 4.4 — Registration (`llmprovider/provider.go`)

`ProviderEnvVars` (`provider.go:106-111`) gains two entries; `NewProvider`
(`provider.go:213-226`) gains one case before `default`:

```go
	ProviderOpencodeZen: "OPENCODE_API_KEY",
	ProviderOpencodeGo:  "OPENCODE_API_KEY",
```

```go
	case ProviderOpencodeZen, ProviderOpencodeGo:
		return NewOpencode(name, apiKey, model, opts...)
```

### Step 4.5 — Tests (modify four existing test files)

- `discovery_test.go`: `TestListAvailableModels_OpencodeZen` / `_OpencodeGo` (real listing
  shape, curated result capped at `MaxListedModels`, denied ID absent);
  **`TestListOpencodeModels_NoAPIKey`** — handler **fails the test** if it sees an
  `Authorization` header, yet `ListAvailableModels(ctx, ProviderOpencodeZen, "", …)`
  succeeds; `TestListAvailableModels_OpencodeFallback` — 500 server yields statics.
- `models_catalog_test.go`: `TestIsUsableOpencodeModel`, `TestRankOpencodeModel`,
  `TestStaticOpencode_Count`; extend the provider slice at **line 197**.
- `provider_test.go`: extend the provider slice at **line 95** with both gateway names;
  `TestProviderEnvVars_Opencode`; `TestNewProvider_OpencodeRoutesResolved` (type-assert to
  `*OpencodeProvider`, check `Route()` for `claude-sonnet-5` is `OpencodeRouteMessages`).
- `interface_test.go`: add `*OpencodeProvider` to the **nine** groups satisfied by every
  provider (the eight capability groups plus `ModelDiscoverer`); update the
  file doc comment ("all four concrete provider implementations") and annotate the
  `Continuer` group (`interface_test.go:56-59`).

### Step 4.6 — Verification

```bash
gofmt -l llmprovider/discovery.go llmprovider/models_catalog.go llmprovider/opencode.go llmprovider/provider.go llmprovider/*_test.go
go vet ./... && go test ./llmprovider/... -run 'Opencode|TestNewProvider|TestStatic|TestProviderInterface' -v
make lint && make test
```

**Acceptance:** OpenCode is fully registered and usable end-to-end;
`TestListOpencodeModels_NoAPIKey` proves the no-credential path. **Commit.**

---

## Phase 5 — Hugging Face Provider

**Goal:** a complete, registered Hugging Face provider. One endpoint, no route table,
metadata-driven discovery.

### Step 5.1 — `llmprovider/huggingface.go` (new file)

```go
package llmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const huggingFaceBaseURL = "https://router.huggingface.co/v1"

// wireShapesProbedOnHuggingFace is the date this file's wire shapes were measured against
// the live router (Step 1.0 pattern): the /v1/models metadata field names that
// drive curation (throughput, first_token_latency_ms, supports_tools,
// architecture.output_modalities), and the /v1/responses HTTP-200-with-
// status:"failed" behaviour that is the reason this provider does not use it.
// Re-validate with: go test -tags live_gateways ./llmprovider/ -run Live
const wireShapesProbedOnHuggingFace = "2026-08-29"

// HuggingFaceProvider implements Provider against Hugging Face Inference
// Providers, a routing proxy in front of 18 partner inference providers.
//
// Only POST {base}/chat/completions is used. A POST {base}/responses endpoint
// also exists and returns a Responses envelope, but it is deliberately NOT used:
// it is undocumented, the official docs scope the OpenAI-compatible surface to
// "chat completion tasks only", and on auth failure it returns HTTP 200 with
// status:"failed" and the error in the body (measured 2026-08-29). Under this
// package's status-only classification that would decode as an empty, error-free
// success. Do not "fix" this omission without re-measuring.
//
// HuggingFaceProvider does NOT implement Continuer: Chat Completions is
// stateless, the same reason ClaudeProvider omits it (claude.go:12-17).
type HuggingFaceProvider struct {
	apiKey, model, baseURL string
	client                 *http.Client
	maxTokens              int
	reasoningEffort        string
}

func NewHuggingFace(apiKey, model string, opts ...ProviderOption) (*HuggingFaceProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("huggingface api key is required")
	}
	cfg := ApplyOptions(opts)
	baseURL := huggingFaceBaseURL
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	return &HuggingFaceProvider{
		apiKey: apiKey, model: model, baseURL: baseURL,
		client: cfg.HTTPClient, maxTokens: cfg.MaxTokens,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

func (p *HuggingFaceProvider) Name() string { return ProviderHuggingFace }
```

Then the standard method set — `Generate`, `GenerateThinking`, `GenerateWithTool`,
`GenerateWithToolThinking`, the four `GenerateItems*` variants — all delegating to:

```go
func (p *HuggingFaceProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
	effort := ""
	if thinking {
		effort = p.reasoningEffort
		if effort == "" {
			effort = effortMedium
		}
	}
	body := chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
		Tool: tool, ForceTool: tool != nil, ReasoningEffort: effort,
	})
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("huggingface: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)
	limitedBody := io.LimitReader(resp.Body, 1<<20)
	if err := classifyHTTPStatus(ProviderHuggingFace, resp); err != nil {
		return nil, err
	}
	return decodeChatCompletionsResponse(limitedBody)
}
```

Plus `DiscoverModels`, mirroring Step 4.3 but constructing `NewHuggingFace`.

`reasoning_effort` is documented as "provider and model-dependent", so an ignored parameter
is a normal outcome — the thinking path degrades to a plain generation rather than failing.
This is stated in the `GenerateThinking` doc comment.

### Step 5.2 — `llmprovider/discovery.go` (modify) — metadata-driven lister

Add `"math"` and `"slices"` to the import block, add the switch case, and append:

```go
// listHuggingFaceModels fetches the router catalog and curates it using the
// metadata Hugging Face publishes. The endpoint is PUBLIC (200 with no
// credential, verified 2026-08-29), so the Authorization header is optional.
//
// Unlike every other provider in this package, ranking here uses measured
// figures rather than name heuristics: the listing reports throughput
// (tokens/sec) and first_token_latency_ms per provider offering. The sorted
// order is handed to curateFromCatalog with a nil rankFn, which preserves it
// (models_catalog.go:224-226).
func listHuggingFaceModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := huggingFaceBaseURL
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
	if err != nil {
		return StaticModels(ProviderHuggingFace), nil
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(ProviderHuggingFace), nil
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return StaticModels(ProviderHuggingFace), nil
	}

	var result struct {
		Data []struct {
			ID           string `json:"id"`
			Architecture struct {
				InputModalities  []string `json:"input_modalities"`
				OutputModalities []string `json:"output_modalities"`
			} `json:"architecture"`
			Providers []struct {
				Status              string  `json:"status"`
				SupportsTools       bool    `json:"supports_tools"`
				Throughput          float64 `json:"throughput"`
				FirstTokenLatencyMs float64 `json:"first_token_latency_ms"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(ProviderHuggingFace), nil
	}

	type scored struct {
		id   string
		tps  float64
		ttft float64
	}
	var ranked []scored
	for _, m := range result.Data {
		if !onlyText(m.Architecture.InputModalities) || !onlyText(m.Architecture.OutputModalities) {
			continue
		}
		if !isUsableHuggingFaceModel(m.ID) {
			continue
		}
		best := scored{id: m.ID, ttft: math.MaxFloat64}
		live := false
		for _, pr := range m.Providers {
			if pr.Status != "live" {
				continue
			}
			live = true
			if pr.Throughput > best.tps {
				best.tps = pr.Throughput
			}
			if pr.FirstTokenLatencyMs > 0 && pr.FirstTokenLatencyMs < best.ttft {
				best.ttft = pr.FirstTokenLatencyMs
			}
		}
		if live {
			ranked = append(ranked, best)
		}
	}
	// Fastest first; ties broken by lowest time-to-first-token.
	slices.SortStableFunc(ranked, func(a, b scored) int {
		switch {
		case a.tps > b.tps:
			return -1
		case a.tps < b.tps:
			return 1
		case a.ttft < b.ttft:
			return -1
		case a.ttft > b.ttft:
			return 1
		}
		return 0
	})
	available := make([]string, 0, len(ranked))
	for _, r := range ranked {
		available = append(available, r.id)
	}

	// nil rankFn preserves the metadata-derived order above.
	curated := curateFromCatalog(StaticHuggingFace, available, isUsableHuggingFaceModel, nil)
	if len(curated) == 0 {
		return StaticModels(ProviderHuggingFace), nil
	}
	return curated, nil
}

// onlyText reports whether a modality list is exactly ["text"].
func onlyText(mods []string) bool { return len(mods) == 1 && mods[0] == "text" }
```

### Step 5.3 — `llmprovider/models_catalog.go` (modify)

```go
	// StaticHuggingFace: fallback only — discovery is metadata-driven. Every ID
	// confirmed present in GET https://router.huggingface.co/v1/models on
	// 2026-08-29, text->text, tool-capable, and served by >= 4 partner providers
	// (redundancy is the best available proxy for durability in an open catalog).
	StaticHuggingFace = []string{
		"openai/gpt-oss-20b",                 // 7 providers, 763 tok/s, $0.50/M out
		"openai/gpt-oss-120b",                // 11 providers, 1106 tok/s, $0.75/M out
		"meta-llama/Llama-3.1-8B-Instruct",   // 4 providers, cheapest at $0.06/M out
		"zai-org/GLM-5.3-Flash",              // 5 providers, 144 tok/s
		"deepseek-ai/DeepSeek-V4-Flash-0731", // 5 providers, all tool-capable
		"zai-org/GLM-5.2",                    // 8 providers
	}
```

Plus one `StaticModels` case, and:

```go
// splitHuggingFaceModelPolicy splits a router model id into its bare
// "<org>/<name>" and an optional provider-selection policy suffix
// (":fastest", ":cheapest", ":preferred", or a partner name like ":groq").
// The split is at the LAST colon, and only when the suffix contains no "/",
// because org and model names may not contain colons but the policy always
// follows one.
//
// NOTE: this is Hugging Face-specific. Kilo model ids may legitimately END in
// ":free" (tencent/hy3:free), so Kilo must NOT use this helper.
func splitHuggingFaceModelPolicy(id string) (base, policy string) {
	i := strings.LastIndex(id, ":")
	if i < 0 || strings.Contains(id[i+1:], "/") {
		return id, ""
	}
	return id[:i], id[i+1:]
}

// isUsableHuggingFaceModel filters router model IDs to production text models.
// Modality filtering happens in listHuggingFaceModels from published metadata;
// this catches the static-catalog path where no metadata is available.
func isUsableHuggingFaceModel(id string) bool {
	base, _ := splitHuggingFaceModelPolicy(strings.TrimSpace(id))
	sm := strings.ToLower(base)
	if sm == "" || !strings.Contains(sm, "/") {
		return false
	}
	for _, deny := range []string{"vision", "-vl-", "embed", "tts", "whisper", "flux", "stable-diffusion"} {
		if strings.Contains(sm, deny) {
			return false
		}
	}
	return true
}

// RankHuggingFaceModel is a weak, name-based fallback used only when the live
// listing is unavailable and the static catalog is in play. The primary ranking
// path uses published throughput and latency; see listHuggingFaceModels.
func RankHuggingFaceModel(m string) int {
	sm := strings.ToLower(m)
	score := 0
	switch {
	case strings.Contains(sm, "-20b"), strings.Contains(sm, "8b"), strings.Contains(sm, "flash"):
		score += 150
	case strings.Contains(sm, "-120b"), strings.Contains(sm, "70b"):
		score += 80
	}
	if strings.Contains(sm, "thinking") || strings.Contains(sm, "-r1") {
		score -= 100 // reasoning-heavy; slow for hook latency
	}
	return score
}
```

### Step 5.4 — Registration and tests

`provider.go`: `ProviderHuggingFace: "HF_TOKEN"` in `ProviderEnvVars`; a `NewProvider` case
returning `NewHuggingFace(apiKey, model, opts...)`. `discovery.go`: a
`case ProviderHuggingFace:` in `ListAvailableModels`.

`llmprovider/huggingface_test.go` (new):

1. **`TestHuggingFace_RequestShape`** — path is `/chat/completions`; `Authorization: Bearer`
   set; the test server fails if `/responses` is ever requested.
2. **`TestHuggingFace_ReasoningEffort`** — thinking path sends `reasoning_effort:"medium"`
   by default; `WithReasoningEffort("xhigh")` is honoured; the **non**-thinking path sends
   no `reasoning_effort` key.
3. **`TestHuggingFace_ToolCall`** — forced `tool_choice` shape; `tool_calls[]` decodes to
   `FunctionCallItem` through the shared decoder.
4. **`TestHuggingFace_ErrorClassification`** — 401/429/500/400 → the four sentinels, using
   the measured `{"error":"Invalid username or password."}` body to prove the decoder is
   never reached on error.
5. **`TestHuggingFace_NoContinuer`** — negative interface assertion.
6. **`TestSplitHuggingFaceModelPolicy`** — table: `openai/gpt-oss-120b` → (`…120b`, ``);
   `openai/gpt-oss-120b:groq` → (`…120b`, `groq`); `…:cheapest` → policy `cheapest`; a bare
   `org/name` with no colon is unchanged.

`discovery_test.go` (modify):

7. **`TestListHuggingFaceModels_MetadataCuration`** — fixture with (a) a VLM
   (`input_modalities:["image","text"]`), (b) a model whose only provider is
   `status:"queued"`, (c) three text models with throughputs 50/300/150 — asserts (a) and
   (b) are dropped and the rest are ordered 300, 150, 50, proving the `nil` rankFn preserves
   metadata order.
8. **`TestListHuggingFaceModels_NoToken`** — handler fails the test on any `Authorization`
   header; the call still succeeds.
9. **`TestListAvailableModels_HuggingFaceFallback`** — 500 server → statics.

`models_catalog_test.go`, `provider_test.go`, `interface_test.go` (modify): add
`ProviderHuggingFace` to both provider slices, add `*HuggingFaceProvider` to the eight
interface groups (nine), add `TestStaticHuggingFace_Count`.

### Step 5.5 — Verification

```bash
gofmt -l llmprovider/huggingface.go llmprovider/huggingface_test.go llmprovider/discovery.go llmprovider/models_catalog.go llmprovider/provider.go
go vet ./... && go test ./llmprovider/... -run 'HuggingFace' -v
make lint && make test
```

**Acceptance:** all nine Hugging Face tests pass; metadata ordering proven. **Commit.**

---

## Phase 6 — Kilo Gateway Provider

**Goal:** a complete, registered Kilo provider with exact capability gating from
`supported_parameters`.

### Step 6.1 — `llmprovider/kilo.go` (new file)

**No new constants.** This phase's `supports()` gating reuses `jsonKeyToolChoice` and
`jsonKeyReasoningEffort` as capability keys; both are declared in Step 2.1, their first use.

```go
const kiloBaseURL = "https://api.kilo.ai/api/gateway"

// wireShapesProbedOnKilo is the date this file's wire shapes were measured
// against the live gateway (Step 1.0 pattern): the message.reasoning spelling
// (not reasoning_content), supported_parameters as a per-model capability list,
// pricing.completion as a string with "-1" for variable-priced tiers, and
// /chat/completions answering free models with no credential.
// Re-validate with: go test -tags live_gateways ./llmprovider/ -run Live
const wireShapesProbedOnKilo = "2026-08-29"

// KiloProvider implements Provider against Kilo Gateway, the inference API
// behind the Kilo Code agent.
//
// Only POST {base}/chat/completions is used. Kilo also answers POST /responses
// and POST /messages with the SAME model — it is a format-translating gateway,
// unlike OpenCode where routes are per-model and a mismatch returns HTTP 500
// (verified 2026-08-29). Those two routes are undocumented, and Kilo's
// /responses places reasoning text in output[].content[].type=="reasoning_text"
// with summary:[], which decodeResponsesAPIOutput does not read
// (http_helpers.go:64-71), so it would silently drop every trace. Because the
// gateway translates, one route reaches the whole catalog; a second buys no
// model coverage. See MADR revision 4, options 13 and 14.
//
// https://kilo.ai/api/openrouter serves a byte-identical catalog but is an
// undocumented alias retained for the editor extension; it is not used here.
//
// KiloProvider does NOT implement Continuer: Chat Completions is stateless.
type KiloProvider struct {
	apiKey, model, baseURL string
	client                 *http.Client
	maxTokens              int
	reasoningEffort        string
	// caps is the model's supported_parameters set. nil means "unknown" — send
	// the standard request rather than guessing a model lacks a capability.
	caps map[string]struct{}
}
```

`NewKilo` mirrors `NewHuggingFace` (required key, `WithBaseURL` override) and leaves
`caps` nil. `Name()` returns `ProviderKilo`.

**Capability gating** — the one thing unique to this provider:

```go
// supports reports whether the model accepts a request parameter. When caps is
// unknown (nil), it returns true: the gateway is the authority, and refusing to
// send a parameter we merely cannot confirm would silently degrade requests.
func (p *KiloProvider) supports(param string) bool {
	if p.caps == nil {
		return true
	}
	_, ok := p.caps[param]
	return ok
}

func (p *KiloProvider) doGenerateItems(ctx context.Context, input []Item, tool *Tool, thinking bool) (*Response, error) {
	effort := ""
	if thinking && p.supports(jsonKeyReasoningEffort) {
		effort = p.reasoningEffort
		if effort == "" {
			effort = effortMedium
		}
	}
	body := chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
		Tool: tool,
		// 301 of 366 models accept "tools" but only 279 accept "tool_choice";
		// offering the tool unforced is strictly better than a 400.
		ForceTool:       tool != nil && p.supports(jsonKeyToolChoice),
		ReasoningEffort: effort,
	})
	// … marshal; POST {base}/chat/completions; Content-Type + Bearer;
	// defer closeResponseBody; 1MB io.LimitReader BEFORE the status check;
	// classifyHTTPStatus(ProviderKilo, resp); decodeChatCompletionsResponse.
	// Structure is identical to HuggingFaceProvider.doGenerateItems.
}
```

**How `caps` is populated.** Only by the caller, through a new option. Neither
`listKiloModels` (which returns `[]string`) nor `DiscoverModels` (which must not mutate the
receiver) can supply it, and fetching the catalog inside `doGenerateItems` would put a
hidden network round-trip on the hot path of a git hook. So:

```go
// WithKiloCapabilities declares the request parameters the configured Kilo model
// accepts, as published in that model's supported_parameters (GET {base}/models).
// It gates optional fields the model may reject: "tool_choice" for forced tool
// calls and "reasoning_effort" for the thinking path.
//
// Omit it and every parameter is sent — the gateway is the authority, and
// withholding a parameter we merely cannot confirm would silently degrade
// requests. Ignored by all other providers.
func WithKiloCapabilities(params ...string) ProviderOption {
	return func(cfg *ProviderConfig) { cfg.KiloCapabilities = params }
}
```

`ProviderConfig` gains one additive field alongside `OpencodeRoute` (same pattern as
Step 1.2):

```go
	// KiloCapabilities lists the request parameters the configured Kilo model
	// accepts (its supported_parameters). Empty means "unknown — send everything".
	KiloCapabilities []string
```

`NewKilo` converts it to the `caps` set, leaving `caps` nil when the slice is empty. To
obtain the values, a caller uses the catalog it already fetches; `ListAvailableModels`
returns ids only, so a package-level accessor is provided:

```go
// KiloModelCapabilities returns the supported_parameters published for one Kilo
// model, for use with WithKiloCapabilities. The catalog endpoint is public, so
// apiKey may be empty.
func KiloModelCapabilities(ctx context.Context, apiKey, model string, opts ...ProviderOption) ([]string, error)
```

**Whether gating is needed at all is unmeasured** — see open question 12. The default
(send everything) is the behaviour every other provider in this package already has, so an
unanswered question 12 costs nothing.

**`DiscoverModels`** mirrors Step 4.3 with `NewKilo`, and deliberately does **not** populate
`caps`: probing reconstructs a provider per candidate model, and silently attaching one
model's capabilities to another would be worse than sending everything.

### Step 6.2 — `llmprovider/discovery.go` (modify) — Kilo lister

Add `"strconv"` to the imports, add the switch case, and append:

```go
// listKiloModels fetches the Kilo catalog and curates it. The /models
// endpoint is PUBLIC (200 with no credential, verified 2026-08-29).
//
// Two documented traps are handled here:
//
//  1. pricing.completion is a STRING and is "-1" for the variable-priced
//     kilo-auto/{frontier,balanced,efficient} tiers. A naive ascending sort would
//     rank the most expensive tiers as cheaper than free. Non-parseable or
//     negative prices sort LAST, not first — see kiloPriceRank.
//  2. Kilo model ids may legitimately END in ":free" (tencent/hy3:free), so
//     nothing is stripped at the colon. splitHuggingFaceModelPolicy must not be
//     used here.
//
// Models flagged mayTrainOnYourPrompts are excluded. That is a POLICY decision,
// not a capability filter — see isUsableKiloModel's comment.
func listKiloModels(ctx context.Context, apiKey string, cfg ProviderConfig) ([]string, error) {
	baseURL := kiloBaseURL
	if cfg.BaseURL != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", http.NoBody)
	if err != nil {
		return StaticModels(ProviderKilo), nil
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return StaticModels(ProviderKilo), nil
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return StaticModels(ProviderKilo), nil
	}

	var result struct {
		Data []kiloCatalogEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return StaticModels(ProviderKilo), nil
	}

	type priced struct {
		id    string
		price float64
	}
	var ranked []priced
	for _, m := range result.Data {
		if !onlyText(m.Architecture.InputModalities) || !onlyText(m.Architecture.OutputModalities) {
			continue
		}
		if m.MayTrainOnYourPrompts { // POLICY — see isUsableKiloModel's comment
			continue
		}
		if !slices.Contains(m.SupportedParameters, jsonKeyTools) {
			continue
		}
		if !isUsableKiloModel(m.ID) {
			continue
		}
		ranked = append(ranked, priced{id: m.ID, price: kiloPriceRank(m.Pricing.Completion)})
	}
	// Cheapest first; "-1" and unparseable prices sort last via kiloPriceRank.
	slices.SortStableFunc(ranked, func(a, b priced) int {
		switch {
		case a.price < b.price:
			return -1
		case a.price > b.price:
			return 1
		}
		return 0
	})
	available := make([]string, 0, len(ranked))
	for _, r := range ranked {
		available = append(available, r.id)
	}

	// nil rankFn preserves the price ordering above.
	curated := curateFromCatalog(StaticKilo, available, isUsableKiloModel, nil)
	if len(curated) == 0 {
		return StaticModels(ProviderKilo), nil
	}
	return curated, nil
}

// kiloCatalogEntry is the subset of Kilo's OpenRouter-shaped catalog entry this
// package reads. Shared by listKiloModels and KiloModelCapabilities.
type kiloCatalogEntry struct {
	ID           string `json:"id"`
	Architecture struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Completion string `json:"completion"`
	} `json:"pricing"`
	SupportedParameters   []string `json:"supported_parameters"`
	MayTrainOnYourPrompts bool     `json:"mayTrainOnYourPrompts"`
}

// KiloModelCapabilities returns the supported_parameters published for one Kilo
// model, for use with WithKiloCapabilities. The catalog endpoint is public, so
// apiKey may be empty. Returns an error only when the catalog is unreachable or
// the model is absent — an empty list is a valid answer.
func KiloModelCapabilities(ctx context.Context, apiKey, model string, opts ...ProviderOption) ([]string, error) {
	// Same fetch as listKiloModels; returns the matching entry's
	// SupportedParameters, or fmt.Errorf("%w: kilo model %q not in catalog",
	// ErrInvalidRequest, model) when absent.
}

// kiloPriceRank parses Kilo's string pricing into a sortable value. A negative
// or unparseable price means "variable" (the kilo-auto tiers report "-1") and
// sorts last rather than first.
func kiloPriceRank(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v < 0 {
		return math.MaxFloat64
	}
	return v
}
```

### Step 6.3 — `llmprovider/models_catalog.go` (modify)

```go
	// StaticKilo: fallback only — discovery is metadata-driven. Every ID
	// confirmed present in GET https://api.kilo.ai/api/gateway/models on
	// 2026-08-29, all tool-capable. Five are kilo-auto/* managed tiers, chosen
	// because Kilo maintains what they point at — the strongest churn resistance
	// available in a 366-model open catalog.
	StaticKilo = []string{
		"kilo-auto/free",                     // free; also the live-test target
		"kilo-auto/small",                    // cheapest managed tier
		"kilo-auto/efficient",                // cost-optimised
		"kilo-auto/balanced",                 // default quality tier
		"meta-llama/llama-3.1-8b-instruct",   // cheapest concrete model, $0.04/M out
		"nvidia/nemotron-3.5-lightning:free", // free fallback, supports reasoning_effort
	}
```

Plus one `StaticModels` case, and:

```go
// isUsableKiloModel filters Kilo model IDs to production text models.
//
// POLICY, not capability: models flagged mayTrainOnYourPrompts are excluded by
// the lister (25 of 366 on 2026-08-29). A shared library used by commit hooks
// should not route a private diff to a model that trains on it without the
// caller asking. This is the only judgement this package makes on the user's
// behalf; it is asserted by TestIsUsableKiloModel_TrainingPolicy so it stays
// visible and reversible. The flag lives in the listing, so the check is in
// listKiloModels; this function handles the id-only static path.
func isUsableKiloModel(id string) bool {
	sm := strings.ToLower(strings.TrimSpace(id))
	// NOTE: no colon-splitting — ":free" is part of the id, not a policy suffix.
	if sm == "" || !strings.Contains(sm, "/") {
		return false
	}
	for _, deny := range []string{"vision", "-vl", "embed", "tts", "whisper", "omni", "image"} {
		if strings.Contains(sm, deny) {
			return false
		}
	}
	return true
}

// RankKiloModel is a weak, name-based fallback used only when the live
// listing is unavailable. The primary ranking path uses published pricing;
// see listKiloModels.
func RankKiloModel(m string) int {
	sm := strings.ToLower(m)
	score := 0
	if strings.HasPrefix(sm, "kilo-auto/") {
		score += 200 // managed tiers: stable ids, gateway-selected models
	}
	switch {
	case strings.Contains(sm, "flash"), strings.Contains(sm, "lightning"),
		strings.Contains(sm, "small"), strings.Contains(sm, "mini"):
		score += 100
	case strings.Contains(sm, "-pro"), strings.Contains(sm, "-max"),
		strings.Contains(sm, "frontier"):
		score -= 200
	}
	return score
}
```

### Step 6.4 — Registration and tests

`provider.go`: `ProviderKilo: "KILO_API_KEY"`; a `NewProvider` case.
`discovery.go`: a `case ProviderKilo:`.

`llmprovider/kilo_test.go` (new):

1. **`TestKilo_RequestShape`** — path `/chat/completions`; `Authorization: Bearer`; the
   server fails the test if `/responses` or `/messages` is requested.
2. **`TestKilo_SupportedParameterGating`** — table over `caps`:

   | `caps` | Expect in body |
   |---|---|
   | nil (unknown) | `tools` **and** `tool_choice` **and** `reasoning_effort` |
   | `{tools}` | `tools`, **no** `tool_choice`, **no** `reasoning_effort` |
   | `{tools, tool_choice}` | both, no `reasoning_effort` |
   | `{tools, tool_choice, reasoning_effort}` | all three |

3. **`TestKilo_Generate`** — the measured `kilo-auto/free` body (content + `reasoning`)
   → `ReasoningItem` then `MessageItem`, `OutputText()` correct.
4. **`TestKilo_ErrorClassification`** — 401 with the measured
   `{"error":{"code":"PAID_MODEL_AUTH_REQUIRED",…},"error_type":…}` body → `ErrAuthFailure`;
   **402 → `ErrInvalidRequest`**; 429/500/400 → their sentinels.
5. **`TestKilo_NoContinuer`** — negative interface assertion.
6. **`TestKiloPriceRank`** — `"0"` → 0; `"0.0000004"` → that value; `"-1"` →
   `math.MaxFloat64`; `""` and `"abc"` → `math.MaxFloat64`.
7. **`TestWithKiloCapabilities`** — `NewKilo(..., WithKiloCapabilities("tools"))` yields a
   provider whose `supports("tools")` is true and `supports("tool_choice")` is false;
   passing no option leaves `caps` nil so **both** are true. This is the test that would
   have caught `caps` being unreachable.
8. **`TestKiloModelCapabilities`** — against an `httptest` catalog fixture: a known model
   returns its `supported_parameters`; an absent model returns an error wrapping
   `ErrInvalidRequest`; the handler fails the test if it sees an `Authorization` header
   when `apiKey` is empty.

`discovery_test.go` (modify):

9. **`TestListKiloModels_MetadataCuration`** — fixture with a VLM, a
   `mayTrainOnYourPrompts:true` model, a model lacking `tools`, and three priced text models
   (`"0.000002"`, `"0.0000005"`, `"-1"`) — asserts the first three are dropped and the rest
   ordered cheapest-first with `"-1"` **last**.
10. **`TestListKiloModels_NoAPIKey`** — handler fails on any `Authorization` header; the
    call still succeeds.
11. **`TestIsUsableKiloModel_ColonFree`** — `tencent/hy3:free` and
   `nvidia/nemotron-3.5-lightning:free` are **usable** and unmodified, pinning trap #2.
12. **`TestIsUsableKiloModel_TrainingPolicy`** — documents and pins the
    `mayTrainOnYourPrompts` exclusion, asserting it is applied in `listKiloModels` (where
    the flag lives) and not in the id-only `isUsableKiloModel`.

`models_catalog_test.go`, `provider_test.go`, `interface_test.go` (modify): add
`ProviderKilo` to both provider slices, `*KiloProvider` to the nine interface
groups, `TestStaticKilo_Count`, `TestRankKiloModel`.

### Step 6.5 — Verification

```bash
gofmt -l llmprovider/kilo.go llmprovider/kilo_test.go llmprovider/discovery.go llmprovider/models_catalog.go llmprovider/provider.go
go vet ./... && go test ./llmprovider/... -run 'Kilo' -v
make lint && make test
```

**Acceptance:** all twelve Kilo tests pass, including both documented traps and the
`caps` reachability test. **Commit.**

---

## Phase 7 — Live Verification and Open Questions

### Step 7.1 — `llmprovider/live_gateways_test.go` (new file, build-tagged)

```go
//go:build live_gateways

// Opt-in live tests against the real gateways. Excluded from the default build.
//   go test -tags live_gateways ./llmprovider/ -run Live -v
//
// OpenCode and Kilo tests use FREE models and send NO real API key, so they need
// no credentials; they are rate-limited upstream, so 429 and 400 SKIP rather
// than fail — these assert wire-format correctness, not gateway availability.
//
// The Hugging Face test REQUIRES HF_TOKEN and skips when it is unset: HF reports
// is_free:false for all 317 provider offerings, so no credential-free path
// exists (verified 2026-08-29).
package llmprovider
```

These are **shape assertions**, not liveness checks. A test that only asserted "a response
came back" would still pass after Hugging Face renamed `throughput`, Kilo renamed
`reasoning`, or OpenCode moved a model between routes — while the code silently degraded to
unsorted catalogs, dropped reasoning traces, or a retried 500. Each row below pins a
specific measurement a decision rests on; a failure is a **drift report**, not a bug, and
each failure message must include the relevant `wireShapesProbedOn*` value so the report
says when the assertion was last true.

**All five were re-run by hand on 2026-08-29 and hold**, which is both the baseline and a
worked example of the procedure: OpenCode `/responses` `200` + `/chat/completions` `500` for
`muse-spark-1.2-contributor-free`; Kilo still emits `reasoning` (not `reasoning_content`)
and publishes `supported_parameters` on 366/366 models with `tools` on 301; Hugging Face
still publishes all four curation fields; and all four `GET …/models` still answer `200`
with no credential.

One caveat the re-run surfaced: the OpenCode free tier returns
`429 FreeUsageLimitError` under repeated use, which is exactly why the suite skips on 429
rather than failing.

| Test | Target | Credential | Asserts |
|---|---|---|---|
| `TestLive_OpencodeChatCompletions` | `hy3-free` on Zen | `OPENCODE_API_KEY` or skip (D3) | non-empty `OutputText()` |
| `TestLive_OpencodeResponses` | `muse-spark-1.2-contributor-free` | `OPENCODE_API_KEY` or skip (D3) | `MessageItem` present; `Route()` is `OpencodeRouteResponses` |
| `TestLive_KiloChatCompletions` | `kilo-auto/free` | none | non-empty `OutputText()` |
| `TestLive_KiloToolCall` | `kilo-auto/free` | none | **a `FunctionCallItem` with parseable JSON arguments** |
| `TestLive_HuggingFaceChatCompletions` | `openai/gpt-oss-20b` | `HF_TOKEN` or skip | non-empty `OutputText()` |
| **`TestLive_OpencodeRouteStillEnforced`** | `muse-spark-1.2-contributor-free` | `OPENCODE_API_KEY` or skip (D3) | `/responses` still `200` **and** `/chat/completions` still fails — the measurement the entire 63+26-row route table rests on |
| **`TestLive_KiloReasoningSpelling`** | `kilo-auto/free` | none | `choices[0].message.reasoning` is still the emitted spelling (not `reasoning_content`), pinning the dual-name decoder |
| **`TestLive_KiloSupportedParameters`** | `GET /models` | none | entries still publish `supported_parameters`, containing `tools` for the gated models |
| **`TestLive_HuggingFaceMetadataFields`** | `GET /v1/models` | none | the four curation fields are still **present in the schema**: `architecture.output_modalities` on every model, and `throughput` / `first_token_latency_ms` / `supports_tools` on **at least one live offering of at least one model**. Deliberately not universal — re-probed 2026-08-29, only **253 of 317** offerings carry all four, so asserting every offering would be flaky by construction |
| **`TestLive_ListingsNeedNoCredential`** | all four `GET …/models` | none | each still answers `200` with **no** `Authorization` header |

`TestLive_KiloToolCall` is the **only** end-to-end tool-calling coverage in this change —
OpenCode's free models refuse tool requests and Hugging Face has no free tier.

~~Because `NewOpencode` and `NewKilo` require a non-empty key, the credential-free tests
pass a placeholder string; free models ignore auth (measured).~~

> **Amended 2026-08-29 — deviation D3.** That is true of **Kilo only**. Measured:
>
> | Request | Result |
> |---|---|
> | OpenCode free model, **no** `Authorization` header | `200` |
> | OpenCode free model, **bogus** Bearer | `401 "Invalid API key."` |
> | Kilo free model, **bogus** Bearer | `200` |
>
> The original measurement of OpenCode's free models used **no header**; generalising it to
> "ignores auth" was wrong. A bogus key is strictly worse there than none, and
> `NewOpencode` requires a non-empty key and always sends it. The three OpenCode live tests
> therefore **skip unless `OPENCODE_API_KEY` is set**, matching the Hugging Face row.
> Kilo's four tests remain credential-free.

> **Known caveat, stated rather than worked around:** these depend on third-party services
> being up and on free-tier capacity. That is exactly why they are build-tagged and
> skip-on-limit — they add real-wire confidence when run by hand and contribute nothing to
> CI flakiness. They do **not** substitute for the `httptest` coverage in Phases 2–6, which
> is what gates the merge.

### Step 7.2 — Close the MADR's twelve open questions

| # | Question | Credential | Resolution rule, fixed **before** measuring |
|---|---|---|---|
| 1 | OpenCode `/messages` and `anthropic-version` | `OPENCODE_API_KEY` | If required → set the header for `OpencodeRouteMessages` only, plus an assertion in `TestOpencode_KeyInHeader` |
| 2 | OpenCode tool calls per route | `OPENCODE_API_KEY` | If a route fails → that route's `GenerateWithTool` returns a typed error naming the route, never silent empty text |
| 3 | OpenCode thinking on messages/google | `OPENCODE_API_KEY` | If unsupported → documented no-op, as chat_completions already is; add a test |
| 4 | OpenCode chaining with a funded key | `OPENCODE_API_KEY` | If it works → `Continuer` is a **follow-up** MADR, not this one; record the finding either way |
| 5 | HF `429` + `Retry-After` | `HF_TOKEN` | Fixture-only; no code change either way |
| 6 | HF `reasoning_effort` rejected vs ignored | `HF_TOKEN` | If some providers `400` → documented retry-without-reasoning fallback, not a hard failure |
| 7 | HF `reasoning_content` ever emitted | `HF_TOKEN` | Documentation accuracy only; decoder already tolerates it |
| 8 | HF forced `tool_choice` on a non-tool model | `HF_TOKEN` | If it errors → pre-check `supports_tools` and fail fast with `ErrInvalidRequest` |
| 9 | Kilo `429` + `Retry-After` | **none** | Fixture-only. **Answerable without a key — close it in this phase.** |
| 10 | Kilo `402` reachable as a distinct status | `KILO_API_KEY` | If unreachable → drop the `ErrPaymentRequired` future-work note |
| 11 | Kilo `/responses` + `/messages` supported or incidental | email `hi@kilo.ai` | Decision does not depend on it; if incidental, soften the MADR's "translates formats" claim to an observation |
| 12 | **Is Kilo capability gating necessary at all?** Does a model whose `supported_parameters` omits `tool_choice` actually reject a forced `tool_choice`, or ignore it? | **none** — pick any free model lacking `tool_choice` | If it **rejects** → `WithKiloCapabilities` is load-bearing; document it in the provider doc comment as recommended. If it **ignores** → the option stays as a no-cost refinement, and the MADR's "capability gating can be exact" consequence is downgraded to "can be exact where it matters". Either way no code changes; this decides documentation and emphasis only. **Answerable without a key — close it in this phase.** |

Anything that changes a *decision* rather than a step is a **deviation**: stop, present
evidence and resolutions, amend the MADR, then execute — per the repository workflow.

### Step 7.3 — Documentation

- Amend the MADR: set each open question to its measured answer, or record explicitly that
  it remains open because no funded key was available.
- Append any deviation to §14 of this plan.
- If `README.md` enumerates supported providers, add all four. *(Verify first with
  `grep -n -i 'grok\|provider' README.md`; do not assume it does.)*

### Step 7.4 — Verification

```bash
go build -tags live_gateways ./...                          # live file compiles
go test ./...                                               # live tests NOT run by default
go test -tags live_gateways ./llmprovider/ -run Live -v      # manual, network
make lint && make vet && make test
go mod tidy && git diff --exit-code go.mod go.sum           # must be unchanged
```

**Acceptance:** default `go test ./...` performs no network I/O; `go.mod`/`go.sum`
unchanged. **Commit.**

---

## 8. Verification Commands

```bash
# Format check — must print nothing
gofmt -l ./llmprovider

# Unit tests, all packages
make test          # go test ./...

# Vet
make vet           # go vet ./...

# Lint with the project config
make lint          # golangci-lint run -c .golangci.yml ./...

# Module tidiness — this change adds no dependency
go mod tidy && git diff --exit-code go.mod go.sum

# Race detector (probeGenerateHealth is concurrent)
go test -race ./llmprovider/... -run 'Opencode|HuggingFace|Kilo|ListAvailableModels'

# Existing providers must be untouched
git diff --stat main -- llmprovider/openai.go llmprovider/claude.go llmprovider/gemini.go llmprovider/grok.go

# Chat Completions must exist exactly once, provider-neutral
grep -rn 'func decodeChatCompletionsResponse\|func itemsToChatMessages' llmprovider/

# Opt-in live check (network; not part of any gate)
go test -tags live_gateways ./llmprovider/ -run Live -v
```

---

## 9. Acceptance Criteria

1. `gofmt -l ./llmprovider` prints nothing.
2. `make test`, `make vet`, `make lint` all exit 0.
3. `go test -race` passes for all new provider and discovery tests.
4. `go mod tidy` leaves `go.mod` and `go.sum` byte-identical — **no new dependency**.
5. All four identifiers appear at all five registration touch points:
   `grep -n 'ProviderOpencodeZen\|ProviderOpencodeGo\|ProviderHuggingFace\|ProviderKilo' llmprovider/constants.go llmprovider/provider.go llmprovider/discovery.go llmprovider/models_catalog.go`.
6. `TestNewProvider` passes with all **eight** provider names in its slice
   (`provider_test.go:95`), and `Name()` returns the identifier for each.
7. `TestProviderInterfaceSatisfaction` compiles with `*OpencodeProvider`,
   `*HuggingFaceProvider` and `*KiloProvider` in all **nine** groups satisfied by every
   provider (eight capability groups + `ModelDiscoverer`); all three
   `Test*_NoContinuer` negatives pass.
8. **Chat Completions exists exactly once.** The `grep` in §8 returns one hit each, both in
   `chatcompletions.go`; no identifier in that file carries an `opencode`, `huggingface` or
   `kilo` prefix.
9. `TestDecodeChatCompletions_ReasoningFieldNames` passes all five cases, covering both
   measured vendor spellings.
10. OpenCode's per-gateway route divergence is asserted at **both** layers:
    `TestOpencodeRoute_Table` and `TestOpencode_RoutePaths`.
11. `TestOpencode_KeyInHeader` proves `Authorization: Bearer` on all four routes and the
    **absence** of `x-api-key` / `x-goog-api-key`.
12. All three listers work with **no** credential: `TestListOpencodeModels_NoAPIKey`,
    `TestListHuggingFaceModels_NoToken`, `TestListKiloModels_NoAPIKey`.
13. `TestListHuggingFaceModels_MetadataCuration` proves the `nil` `rankFn` preserves
    metadata ordering; `TestListKiloModels_MetadataCuration` proves `"-1"` pricing sorts
    last.
14. `TestKilo_SupportedParameterGating` passes all four `caps` cases, including
    nil-means-send-everything, **and** `TestWithKiloCapabilities` proves `caps` is
    reachable from the public API — the option, not an internal side effect, is what
    populates it.
15. `TestIsUsableKiloModel_ColonFree` proves `:free` ids survive intact — Kilo never
    uses `splitHuggingFaceModelPolicy`.
16. Static catalogs: `StaticOpencodeZen`, `StaticOpencodeGo`, `StaticHuggingFace`,
    `StaticKilo` each contain exactly 6 IDs, all confirmed live on their measurement
    date; neither OpenCode list contains `qwen3.7-max` or `qwen3.7-plus`.
17. `classifyHTTPStatus` maps **402 → `ErrInvalidRequest`**, asserted in both
    `TestClassifyHTTPStatus` and `TestKilo_ErrorClassification`.
18. ~~No existing provider file is modified: the `git diff --stat` in §8 is empty.~~
    **Amended by deviation D4:** `openai.go`, `claude.go`, `gemini.go` and `grok.go` each
    gain **exactly one line** — naming themselves in the `RateLimitError` they construct.
    Their request building, decoding and classification switches are untouched;
    `git diff --stat` against `main` must show `2 +-` for each and nothing more.
19. Three **distinct** constants exist — `wireShapesProbedOnOpencode` (`"2026-08-28"`),
    `wireShapesProbedOnHuggingFace` and `wireShapesProbedOnKilo` (both `"2026-08-29"`) —
    distinct because all three files share `package llmprovider`. Each value matches the
    probe date the MADR cites for that gateway, and each is referenced from the
    untagged `TestWireShapesProbedOn` so `unused` does not flag it — **not** from the
    `live_gateways` suite, which golangci-lint does not analyse (deviation D2).
20. The `live_gateways` suite contains the five shape-assertion tests from Step 7.1, not
    liveness checks alone; `go build -tags live_gateways ./...` compiles and plain
    `go test ./...` still performs no network I/O.
21. Every phase was committed separately, and **no `git push` was performed**.

---

## 10. Decisions Resolved by This Plan

| MADR-deferred item | Resolved in |
|---|---|
| OpenCode route values, full per-gateway table with provenance | §3.1–3.3, Step 1.3 |
| `WithOpencodeRoute` signature | Step 1.2 |
| `classifyHTTPStatus` extraction | Step 2.1 |
| Shared Chat Completions primitive, provider-neutral | Step 2.2 |
| Dual reasoning field spellings | Step 2.2, test 2 in Step 2.3 |
| OpenCode four request builders | Step 3.1(f) |
| All four static catalogs with live-listing evidence | Steps 4.2, 5.3, 6.3 |
| Three no-credential listers | Steps 4.1, 5.2, 6.2 |
| HF metadata-driven curation via nil `rankFn` | Step 5.2 |
| Kilo `supported_parameters` gating | Step 6.1 |
| Build-tagged live tests | Step 7.1 |
| How each of the 12 open questions is answered | Step 7.2 |

Additional decisions this plan makes that the MADR left implicit:

- **`Name()` returns the provider identifier**, forced by `TestNewProvider`'s assertion.
- **OpenCode's `route` is resolved at construction**, exposed via `Route()`.
- **`chatCompletionsOpts.ForceTool` is separate from `Tool`**, because Kilo gates
  `tool_choice` independently of `tools`.
- **Kilo's `caps == nil` means "send everything"** — refusing to send a parameter we merely
  cannot confirm would silently degrade requests.
- **`DiscoverModels` reconstructs the provider per probe candidate** for OpenCode, because
  route varies by model.
- **`decodeGeminiResponse`'s "gemini" error text is accepted as-is** on OpenCode's Google
  route, to avoid touching `gemini.go`.
- **Registration moved into each provider's phase**, so each phase ships one usable provider.
- **`splitHuggingFaceModelPolicy` is HF-only**, with a comment forbidding its use on Kilo.

---

## 11. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| OpenCode route table drifts | **High** — docs and live listings already disagree | A new model 500s | Heuristic covers all 10 currently-untabled live IDs (§3.3); `WithOpencodeRoute` escape hatch; `TestOpencodeRoute_Heuristic` pins behaviour |
| OpenCode misroute 500 → retried | Medium | Wasted retry budget, misleading error | Error carries `gateway/route`; **no** auto-fallback (silently changing wire format turns a loud failure into a wrong answer) |
| A third reasoning field spelling appears | Medium | Traces silently dropped | Decoder handles both known names with a five-case test; adding a third is a one-line change plus a case |
| Kilo `"-1"` pricing mis-sorts | Medium | Most expensive tiers ranked as free | `kiloPriceRank` maps negative/unparseable to `MaxFloat64`; `TestKiloPriceRank` and the curation test pin it |
| Kilo `:free` ids corrupted by suffix-splitting | Medium | 20 model ids broken | `splitHuggingFaceModelPolicy` documented HF-only; `TestIsUsableKiloModel_ColonFree` pins it |
| HF/Kilo open catalogs churn | **High** | Static entries retired | Statics are fallback only; HF chosen on ≥4-provider redundancy, Kilo on `kilo-auto/*` managed tiers; `curateFromCatalog` intersects with the live listing |
| Free-model live tests flake | High | CI noise | Build-tagged out of the default build; skip on 429/400 |
| `goconst` trips on repeated model IDs | Medium | Lint failure | `//nolint:goconst` on the route-table var block, matching `models_catalog.go:24` |
| `options.go` change affects other providers | Low | Regression | Purely additive field; zero value is the sentinel; `ApplyOptions` unchanged; full suite is the gate |
| Kilo training-policy exclusion surprises a caller | Low | A wanted model is hidden | Documented in the function comment, pinned by a named test, and listed in §10 as a deliberate decision |
| Open questions resolve against a decision | Low | Rework | Resolution rules fixed in Step 7.2 **before** measuring; decision-level changes are documented deviations |

---

## 12. Out of Scope

- Streaming (`iter.Seq2`) for any provider.
- `encoding/json/v2`.
- Batch, files, or embeddings endpoints.
- Pricing, cost accounting, Kilo's `market_cost`, or HF's credit tiers.
- `Continuer` / conversation chaining — measured broken on OpenCode, structurally absent on
  the other two. Revisit only if question 4 says otherwise, in a follow-up MADR.
- **Refactoring `openai.go`, `claude.go`, `gemini.go`, `grok.go` onto `classifyHTTPStatus`.**
  The helper is introduced and used only by the new providers. Migrating the other four is a
  clean follow-up but would put four tested files in this change's blast radius for no
  functional gain. *(Still out of scope after deviation D4, which changes one line in each
  for provider attribution and leaves their classification switches intact.)*
- Making auth headers injectable on the existing providers (MADR option 3, rejected).
- An `ErrPaymentRequired` sentinel for Kilo's `402`.
- HF: non-chat task APIs, `X-HF-Bill-To`, custom-provider-key config, `/v1/responses`.
- Kilo: the FIM endpoint, `GET /providers` beyond the `mayTrainOnYourPrompts` flag, the
  `kilo.ai/api/openrouter` alias, `/responses` and `/messages`.
- Consumer migration in `mcp-server-magictools` / `mcp-server-magicdev`.
- Any `git push`.

---

## 13. File Summary

### New files (11)

| File | Phase | Purpose |
|---|---|---|
| `llmprovider/opencode_route.go` | 1 | Route type, base URLs, `wireShapesProbedOnOpencode`, per-gateway table, heuristic, resolver |
| `llmprovider/opencode_route_test.go` | 1 | Seven routing/constant tests incl. the divergence assertion |
| `llmprovider/chatcompletions.go` | 2 | **Shared** converter, body builder, decoder (3 consumers) |
| `llmprovider/chatcompletions_test.go` | 2 | Seven tests incl. five reasoning-spelling cases and `classifyHTTPStatus` |
| `llmprovider/opencode.go` | 3, 4 | `OpencodeProvider`, four body builders, request driver, `DiscoverModels` |
| `llmprovider/opencode_test.go` | 3 | Ten provider tests across all four routes |
| `llmprovider/huggingface.go` | 5 | `HuggingFaceProvider` on Chat Completions, `wireShapesProbedOnHuggingFace` |
| `llmprovider/huggingface_test.go` | 5 | Six provider tests |
| `llmprovider/kilo.go` | 6 | `KiloProvider`, `supported_parameters` gating, `KiloModelCapabilities`, `wireShapesProbedOnKilo` |
| `llmprovider/kilo_test.go` | 6 | Eight provider tests incl. the gating table and `caps` reachability |
| `llmprovider/live_gateways_test.go` | 7 | Build-tagged suite across all three families: 5 generation checks + 5 shape assertions (10 tests) |

### Modified files (10)

| File | Phases | Change |
|---|---|---|
| `llmprovider/constants.go` | 1, 2, 3, 6 | +4 provider constants (Phase 1); +5 JSON key constants, each declared in the phase that first uses it (deviation D1) |
| `llmprovider/options.go` | 1, 6 | +`ProviderConfig.OpencodeRoute`, +`WithOpencodeRoute`; +`ProviderConfig.KiloCapabilities`, +`WithKiloCapabilities` |
| `llmprovider/http_helpers.go` | 2 | +`classifyHTTPStatus`, +`fmt` import |
| `llmprovider/discovery.go` | 4, 5, 6 | +4 switch cases, +3 listers, +`onlyText`, +`kiloPriceRank`, +`math`/`slices`/`strconv` imports |
| `llmprovider/models_catalog.go` | 4, 5, 6 | +4 static catalogs, +4 `StaticModels` cases, +`staticOpencodeCatalog`, +3 `isUsable*`, +3 `Rank*`, +`splitHuggingFaceModelPolicy`, +deny lists |
| `llmprovider/provider.go` | 4, 5, 6 | +4 `ProviderEnvVars` entries, +3 `NewProvider` cases |
| `llmprovider/provider_test.go` | 4, 5, 6 | Provider slice 4→8 names, +4 tests |
| `llmprovider/models_catalog_test.go` | 4, 5, 6 | Provider slice 4→8 names, +9 catalog tests |
| `llmprovider/discovery_test.go` | 4, 5, 6 | +10 discovery tests |
| `llmprovider/interface_test.go` | 4, 5, 6 | +3 provider types in 8 groups, `Continuer` comment |

**Unmodified by design:** `openai.go`, `claude.go`, `gemini.go`, `grok.go`,
`grok_reasoning.go`, `item.go`, `probe.go`, `go.mod`, `go.sum`.

---

## 14. Deviation Log

Empty at time of writing. Per the repository workflow, any deviation discovered during
execution is recorded here — dated, naming what was found, the resolution chosen, and any
files added to a phase's scope — **before** the fix is executed. The original step is struck
through or annotated rather than rewritten.

| Date | Phase/Step | Finding | Resolution | Files added to scope |
|---|---|---|---|---|
| 2026-08-29 | **D1** — Phase 1, Step 1.1 | `make lint` fails the Phase 1 gate: `unused` flags all 7 new JSON-key constants, because the plan's own code blocks first use them in Phases 2/3/6. Two — `jsonKeyToolCalls`, `jsonKeyReasoningContent` — are referenced **0 times in any phase**; `decodeChatCompletionsResponse` reads those fields via struct tags. Confirmed introduced, not pre-existing: `make lint` passes on the stashed tree. | Declare each constant in the phase that first uses it (still `constants.go`): `jsonRoleTool`+`jsonKeyMaxTokens`→Step 2.1, `jsonKeyReasoning`→Step 3.1, `jsonKeyToolChoice`+`jsonKeyReasoningEffort`→Step 6.1. **Drop** the two dead constants. Chosen over `//nolint:unused` so the linter keeps working and the dead constants are removed rather than suppressed. | none — Steps 1.1, 2.1, 3.1, 6.1 amended |
| 2026-08-29 | **D4** — post-Phase 7, by explicit direction | `RateLimitError.Error()` carried no provider name, so a 429 from any of the eight providers read identically and was not attributable to a source. Found during Phase 2 and scoped out then, because the type is exported and shared by the four providers criterion 18 protects. | Added a `Provider` field, populated by `classifyHTTPStatus` and by each existing provider (one line each). `Error()` omits it when empty, reproducing the original message verbatim, so callers matching the old string are unaffected — asserted by `TestRateLimitError_ProviderAttribution`. Two tests that previously *exempted* 429 from the provider-name assertion were tightened into positive assertions. **Criterion 18 amended** from "not modified" to "exactly one line each". | `openai.go`, `claude.go`, `gemini.go`, `grok.go` (one line each) |
| 2026-08-29 | **D3** — Phase 7, Step 7.1 | Step 7.1 asserted OpenCode's free models "ignore auth", so the live tests could pass a placeholder key. False: measured, OpenCode returns `200` with **no** `Authorization` header but `401 "Invalid API key."` with a bogus one. The original measurement used no header and was wrongly generalised. `NewOpencode` requires a non-empty key and always sends it, so all three OpenCode live tests failed `401`, including the route-enforcement assertion that protects the 63+26-row table. Kilo is unaffected — it genuinely ignores a bogus key (`200`). | The three OpenCode live tests skip unless `OPENCODE_API_KEY` is set, matching the Hugging Face row. Chosen over relaxing `NewOpencode`'s non-empty-key contract (a design change diverging from `NewClaude`/`NewGrok`) and over bypassing the provider with raw HTTP (which would test the gateway rather than the integration). Accepted cost: route enforcement is only verified by someone holding an OpenCode key. | none — Step 7.1 amended |
| 2026-08-29 | **D2** — Phase 1, Step 1.0 | Step 1.0's stated fix for `unused` on `wireShapesProbedOn*` — "reference it from the `live_gateways` suite" — cannot work: `.golangci.yml` sets no `build-tags`, so `//go:build live_gateways` files are never analysed. Would have recurred in Phases 5 and 6. | Reference each constant from a new **untagged** `TestWireShapesProbedOn` (Step 1.4, test 8) that validates it parses as `YYYY-MM-DD` and is not in the future. Chosen over `//nolint` (leaves the date unverified), exporting (grows public API for 12 repos to satisfy a linter), and adding repo-wide `build-tags` (changes lint config as a side effect of this feature). | none — Steps 1.0, 1.4 amended |
