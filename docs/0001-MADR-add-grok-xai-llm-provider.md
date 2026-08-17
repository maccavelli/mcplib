---
status: proposed
date: 2026-08-17
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev consumers
informed: mcplib contributors
---

# Adopt a Responses-API-Shaped Canonical Contract Across All `llmprovider` Providers, Including a New Grok Provider

> **Revision notes (both applied to this same `proposed` document, not superseding
> MADRs, since neither revision had been accepted at the time it was made):**
>
> 1. This MADR originally scoped a narrower decision (add Grok on the legacy Chat
>    Completions API, leave the existing three providers untouched), then was broadened
>    by explicit direction to make the Responses-API item/state shape canonical across
>    **all** providers in this package, not only the new one.
> 2. This revision adds a grounded assessment of Go-idiomaticity against Go 1.26.5
>    stdlib conventions and comparable OSS Go LLM-provider projects, per explicit
>    direction to prioritize idiomatic/canonical Go design for the provider "engines."
>    See "Idiomatic Go design precedent" below; the Decision Outcome's design-direction
>    subsection has been refined accordingly (sealed-interface item types, a
>    capability-resolver for reasoning gating, and explicit confirmation that the
>    package's existing opt-in-retry and typed-sentinel-error patterns are themselves
>    already idiomatic and are preserved, not replaced).

## Context and Problem Statement

`mcplib/llmprovider` is a small, dependency-free (`net/http` only) abstraction over LLM
backends, currently supporting three providers, each identified by a canonical string
constant in `llmprovider/constants.go:4-8`:

* `ProviderGemini = "gemini"` (Google Gemini, `llmprovider/gemini.go`)
* `ProviderOpenAI = "openai"` (OpenAI, `llmprovider/openai.go`)
* `ProviderClaude = "claude"` (Anthropic Claude, `llmprovider/claude.go`)

A fourth integration, Ollama, exists only in "listing" form
(`llmprovider/discovery.go:31-32`, `ValidateOllamaURL` at `discovery.go:230-249`) — it has
no `Provider` struct and is not reachable through `NewProvider`. This is documented
precedent for a deliberately partial integration, referenced below as an option.

There is currently **no support for xAI's Grok models** anywhere in this repository. A
case-insensitive search of the full tree for `grok`, `xai`, and `x\.ai` returns zero
matches — this is a greenfield addition, not a resumption of partial work.

The package's package doc (`llmprovider/provider.go:1-3`) states it is "used by both
mcp-server-magictools and mcp-server-magicdev." Both are external consumer services that
live in the same monorepo but in sibling modules not present in this checkout; this MADR
records the design for mcplib's own `llmprovider` package, not consumer wiring, and
cannot inspect those consumers' exact `Provider`-interface call sites directly. (An
earlier revision of this MADR mischaracterized this as a "one-way mirror" export
constraint, citing `README.md:1-4`. `git remote -v` shows `origin` is a public GitHub
repo (`github.com/maccavelli/mcplib`) that the local `main` branch tracks directly, and
the most recent commit is a normal `feat(...)` commit made straight on `main`, not a
"Sync from internal source ..." commit — this repo is not receive-only. The actual,
narrower limitation is simply that sibling consumer modules aren't checked out here.)

### Existing architecture (facts, not proposal)

**Core contract** (`llmprovider/provider.go:18-61`):

```go
type Provider interface {
    Name() string
    Generate(ctx context.Context, prompt string) (string, error)
}
```

Four optional capability interfaces, each satisfied by all three existing providers:

* `ToolProvider` — `GenerateWithTool(ctx, prompt string, tool Tool) (string, error)` (`provider.go:34-37`)
* `ModelDiscoverer` — `DiscoverModels(ctx) ([]string, error)` (`provider.go:41-44`)
* `ThinkingProvider` — `GenerateThinking(ctx, prompt string) (string, error)` (`provider.go:50-53`)
* `ThinkingToolProvider` — `GenerateWithToolThinking(ctx, prompt string, tool Tool) (string, error)` (`provider.go:58-61`)

Every method in this contract is **stateless and single-shot**: `(ctx, prompt string) ->
(string, error)`. There is no response identifier, no item/typed-output concept, and no
way to reference a prior turn other than the caller re-supplying the full prompt text
itself.

**Provider struct shape** is identical across `OpenAIProvider` (`openai.go:13-20`),
`ClaudeProvider` (`claude.go:14-21`), and `GeminiProvider` (`gemini.go:13-20`): `apiKey`,
`model`, `baseURL` (test/override injection point), `client *http.Client`, `maxTokens
int`, plus one provider-specific reasoning field (`reasoningEffort string` for OpenAI,
`thinkingBudget int` for Claude/Gemini).

Each provider already parses a **provider-specific typed-output shape internally** and
flattens it down to a single string before returning, discarding structure that its own
upstream API actually exposes:

* OpenAI: `choices[0].message.content` — a `tool_calls` array sits alongside `content` in
  the same message object (`openai.go:206-216`).
* Claude: `content` is an array of typed blocks (`"text"`, `"thinking"`, `"tool_use"`);
  the code explicitly concatenates only `"text"` blocks and has a regression test,
  `TestClaude_MultiBlockContent` (`provider_correctness_test.go:16-30`), guarding against
  naively reading `Content[0]` because a leading `"thinking"` block can have empty text.
* Gemini: `candidates[0].content.parts` is an array of parts, each either `text` or
  `functionCall` (`gemini.go:119-137`, `213-238`).

**Registration is a hardcoded switch, not a registry.** Adding a provider requires
touching exactly these places today:

1. `constants.go:4-8` — new `ProviderXxx` constant.
2. `provider.go:106-110` (`ProviderEnvVars`) — canonical env var name.
3. `provider.go:163-174` (`NewProvider` switch) — dispatch to the constructor.
4. `discovery.go:24-35` (`ListAvailableModels` switch) — dispatch to a `list<X>Models` func.
5. `models_catalog.go:87-98` (`StaticModels` switch) — a `Static<X>` catalog slice, plus an `isUsable<X>...` filter and `Rank<X>Model` function (pattern at `models_catalog.go:100+`).

**Config/options are functional options, not files.** `ProviderConfig`
(`options.go:24-36`) plus `WithHTTPClient`, `WithMaxTokens`, `WithBaseURL`,
`WithThinkingBudget`, `WithReasoningEffort` (`options.go:42-77`). Defaults:
60s-timeout `http.Client` (`options.go:11-21`), `MaxTokens: 8192` (`options.go:82-84`).
API keys are plain constructor string arguments — never read from env inside this
package; `ProviderEnvVars` only documents the expected variable name for callers.

**Error handling and retry** are shared across all three providers: identical typed
errors (`RateLimitError`, `ErrAuthFailure`, `ErrProviderUnavailable`,
`ErrInvalidRequest`, `provider.go:64-85`), a 1MB response cap applied before the status
check (e.g. `openai.go:101-103`), and a provider-agnostic `GenerateWithRetry`
(`provider.go:112-159`) that works purely against the `Provider` interface.

**Tests** are white-box, colocated in-package, and use `net/http/httptest` servers —
no mocks, no live-API integration tests
(`provider_test.go`, `provider_correctness_test.go`, `options_test.go`,
`models_catalog_test.go`, `gemini_test.go`, `thinking_test.go`).

### xAI Grok API facts (from `docs.x.ai`, verified 2026-08-17)

* **Two REST APIs exist.** The legacy **Chat Completions** API (`POST /v1/chat/completions`)
  is OpenAI-SDK-compatible: `messages` array, `choices[0].message.content`,
  `Authorization: Bearer $XAI_API_KEY`, base URL `https://api.x.ai/v1`
  (docs.x.ai/developers/model-capabilities/legacy/chat-completions). xAI's docs mark it
  "legacy" and steer new integrations to the newer, stateful **Responses** API
  (`POST /v1/responses`, `input` instead of `messages`, `output` array instead of
  `choices`, `previous_response_id` for stateful multi-turn, 30-day server-side storage)
  (docs.x.ai/developers/model-capabilities/text/comparison). xAI states Chat Completions
  "will not get new features" but does not state an end-of-life date.
* **Function/tool calling** on the Responses API uses `tools: [{"type":"function", "name":
  ..., "parameters": ...}]` and returns a `function_call` output item with `call_id`,
  `name`, and `arguments`; the caller executes it and replies with a
  `{"type":"function_call_output","call_id":...,"output":...}` input item
  (docs.x.ai/developers/tools/function-calling, docs.x.ai/developers/tools/advanced-usage).
  `tool_choice` supports `"auto"` / `"required"` / `"none"` / a forced
  `{"type":"function","name":"..."}`.
* **Reasoning is model-gated, not universal**, and the gating rules differ across model
  families (corroborated by xAI's own docs, a third-party provider adapter
  (`axl-sdk/axl` `xai.ts`), and a bug report against `grok-cli`,
  github.com/superagent-ai/grok-cli/issues/198):
  - `grok-3-mini` / `grok-3-mini-fast`: accept `reasoning_effort` with only `"low"` /
    `"high"` values.
  - `grok-4.5` / `grok-4.6`: accept `reasoning_effort` with `"low"`/`"medium"`/`"high"`
    (default)/`"xhigh"`, but reasoning **cannot be disabled** on these models.
  - `grok-3`, `grok-4`, `grok-4-fast-reasoning`, `grok-code-fast-1`: reason
    automatically and **return a 400 Bad Request if `reasoning_effort` is sent at all**.
  - Reasoning models across the board reject `presence_penalty`, `frequency_penalty`,
    and `stop` with a 400.
* **Auth/errors**: `401` for missing/invalid `Authorization: Bearer` header; `429` for
  rate limits, with documented per-model RPM/TPM tiers scaling with account spend
  (docs.x.ai/developers/rate-limits, docs.x.ai/developers/debugging). No `Retry-After`
  header is documented for xAI 429 responses. Model listing is `GET /v1/models` (Bearer
  auth, returns `data[].id` plus pricing/aliases).
* No streaming or batch usage is in scope for this decision — neither exists elsewhere
  in this package today, and this MADR does not change that.

### The Responses-API pattern is now cross-vendor, not xAI-specific (new fact, drives the scope change)

Independent research into each vendor's *current* API surface shows that three of the
four providers in scope (existing + Grok) have each shipped a near-identical successor
to their original stateless chat/generate endpoint, all sharing the same structural
ideas — a typed, polymorphic `output`/`steps` array instead of a flat message string,
and an opaque server-side identifier that lets a caller continue a conversation without
resending full history:

| Vendor | Legacy stateless endpoint | Newer Responses-style endpoint | Continuation parameter |
|---|---|---|---|
| OpenAI | `POST /v1/chat/completions` (`messages` → `choices[].message`) | `POST /v1/responses` (`input` → `output[]` typed items) | `previous_response_id` (developers.openai.com/api/docs/guides/migrate-to-responses) |
| xAI | `POST /v1/chat/completions` (same shape as OpenAI's legacy endpoint) | `POST /v1/responses` (same shape as OpenAI's new endpoint — xAI states its API is "compatible with OpenAI and Anthropic's SDKs") | `previous_response_id` (docs.x.ai/developers/model-capabilities/text/comparison) |
| Google Gemini | `POST /v1beta/models/{model}:generateContent` (`contents[]` → `candidates[].content.parts[]`) | **Interactions API** (`ai.google.dev/gemini-api/docs/interactions/interactions-overview`) — typed `steps` timeline, server-stored by default | `previous_interaction_id` |
| Anthropic Claude | `POST /v1/messages` (`messages[]` → `content[]` blocks) | **None.** Anthropic's own current docs (`platform.claude.com/docs/en/build-with-claude/working-with-messages`, `claude_api_primer`) state plainly: "The Messages API is stateless... You always send the full conversational history." No response-ID chaining, no server-managed history, no polymorphic output-item envelope beyond the existing `content` block-type array documented above. | *(none exists)* |

This is the pivotal fact for the broadened scope: **OpenAI, xAI, and Gemini have each
converged on the same item-based, optionally-stateful contract shape**, while Claude has
not and — based on currently published Anthropic documentation — has no such endpoint to
converge toward. Any canonical redesign of this package's `Provider` interface must
account for Claude as a permanent, not transitional, exception rather than assume every
provider will eventually catch up.

### Idiomatic Go design precedent (Go 1.26.5 stdlib facts + comparable OSS Go LLM projects)

Per explicit direction, this revision separately researched what "idiomatic, canonical
Go" means concretely for this redesign, rather than treating the item/state model as an
abstract data-modeling exercise. Two kinds of evidence were gathered: (a) what Go
1.24–1.26 actually shipped in the standard library (this module targets `go 1.26.5`,
`go.mod:3`, confirmed installed via `go version` → `go1.26.5 darwin/arm64`), and (b) how
comparable Go OSS projects that already solve "one interface, many LLM vendors" have
designed their public surface. Findings below are cited to primary sources; none of them
are adopted wholesale, they are used to ground specific refinements to the design
direction in the Decision Outcome.

**Go 1.23–1.26 standard library facts relevant to this package:**

* Range-over-func (`for v := range someFunc`) and the `iter` package (`iter.Seq[V]`,
  `iter.Seq2[K, V]`) shipped stable in Go 1.23 (August 2024) and are unchanged/mature by
  1.26 — `go.dev/doc/go1.26`, `go.dev/blog/range-functions`. This package's own
  `models_catalog.go` already imports `slices` (`models_catalog.go:5`) and uses
  `slices.Contains` (`models_catalog.go:107`), i.e., it already depends on the
  post-generics, iterator-era standard library; adopting `iter.Seq`/`iter.Seq2` at a
  package boundary would be consistent with, not a departure from, the module's current
  standard-library baseline.
* `encoding/json/v2` remains **experimental** in Go 1.26, gated behind
  `GOEXPERIMENT=jsonv2` (`go.dev/doc/go1.25`, unchanged per `go.dev/doc/go1.26`). The
  package's current plain `encoding/json` usage (all five provider/discovery files) is
  therefore still the canonical, non-experimental choice for Go 1.26.5 — this MADR does
  not recommend migrating to `encoding/json/v2`.
* Go 1.26 added: `new(expr)` (an expression, not just a type, as `new`'s operand — useful
  for inline pointer-to-optional-value construction), recursive/self-referential generic
  type constraints, and a rewritten `go fix` with "modernizer" analyzers
  (`go.dev/doc/go1.26`, `go.dev/blog/go1.26`). None of these are required by this
  decision, but `go fix` should be run as a normal implementation-plan step once new
  code lands, per the release notes' own guidance that modernizers "suggest safe fixes ...
  to take advantage of newer features of the language and standard library."
* `testing/synctest` graduated to stable in Go 1.25 (`go.dev/doc/go1.25`) and is
  available, non-experimental, in 1.26 — relevant if the implementation plan adds
  concurrency beyond the existing `probeGenerateHealth` fan-out/fan-in
  (`probe.go:13-60`, already a correct, leak-free `sync.WaitGroup` + buffered-channel
  pattern with per-goroutine `context.WithTimeout`).

**Comparable OSS Go LLM-provider projects surveyed** (all retrieved 2026-08-17; used as
corroborating design precedent, not as an authority to defer to over this package's own
constraints):

| Project | Relevant design choice | Source |
|---|---|---|
| Google Agent Development Kit (Go) | `LLM` interface's streaming method returns `iter.Seq2[*LLMResponse, error]` — an iterator, not a channel, for SSE-backed model responses | engineeredintelligence.substack.com/p/why-its-an-iterator-not-a-channel |
| `amit-timalsina/pi-llm-go` | "Idiomatic Go: `iter.Seq2` streaming, sealed sum types, `errors.Is` sentinels, `context.Context` cancellation"; `Block`/`StreamEvent` are interfaces with unexported marker methods, type-switched exhaustively | pkg.go.dev/github.com/amit-timalsina/pi-llm-go |
| `nocturnium/llm-go-sdk` | Explicit design rule: "A bare provider does not retry... Retries, circuit breaking, and failover are **opt-in** and come from wrapping the provider with middleware" — retry is a decorator over the base client, not baked into it | github.com/nocturnium/llm-go-sdk |
| `mozilla-ai/any-llm-go` | "Errors are values, not exceptions. Every provider's SDK errors are normalized into typed sentinel errors (`ErrRateLimit`, `ErrAuthentication`, ...) that work with `errors.Is`/`errors.As`"; functional-options constructors | blog.mozilla.ai/.../now-in-go |
| `flexigpt/inference-go` | Per-provider `spec.ModelCapabilities` profiles plus a `ModelCapabilityResolver` that layers provider-level → provider-preset → model-preset → caller overrides, with unsupported features "safely dropped" and reported back via `Warnings` rather than sent to the API and rejected | github.com/flexigpt/inference-go |
| `tmc/langchaingo` | Registry/URI-based provider construction (`llms.Open("anthropic://...")`), modeled explicitly on `database/sql` — a heavier pattern than this package needs given its deliberate "no registry, hardcoded switch, SDK-free" design (already documented in Context above) | github.com/tmc/langchaingo (discussion #1282) |

**What this package already gets right, per this survey (a validation finding, not a
gap):** `llmprovider`'s existing choices — typed sentinel errors consumed via
`errors.Is`/`errors.As` (`provider.go:64-85`), functional options for construction
(`options.go`), and, critically, **`GenerateWithRetry` as a standalone function layered
over the `Provider` interface rather than retry logic embedded in each provider**
(`provider.go:112-159`) — are the same choices independently converged on by
`nocturnium/llm-go-sdk` and `mozilla-ai/any-llm-go`. This is strong corroboration that
the *current* error-handling and retry architecture is already idiomatic and canonical
for this problem domain and should be preserved unchanged through the Responses-shape
migration, not rewritten as part of it.

**Where a gap exists relative to this survey:** the package has no standard, reusable way
to represent "this is one of several possible typed things" (Claude's `content` block
types, OpenAI's `tool_calls` vs `content`, Gemini's `parts`) — today each provider
hand-decodes this into anonymous Go structs local to one file
(e.g. `openai.go:118-124`, `claude.go:126-131`, `gemini.go:119-127`). The sealed-interface
pattern pi-llm-go uses, and which is also the Go **standard library's own** long-standing
idiom for this exact situation (`go/ast`'s `Node`/`Decl`/`Stmt`/`Expr` interfaces, each
restricted to package-approved implementations via an unexported marker method such as
`exprNode()`), is the concrete, stdlib-precedented mechanism this MADR recommends for the
new canonical `Item` type — see the refined design direction below.

## Decision Drivers

* **Cross-vendor convergence, not a single-vendor quirk.** Three of the four
  providers in scope now natively expose the same item/state shape; designing Grok's
  integration around the older, "legacy"-labeled Chat Completions shape would mean
  building the new provider on a pattern the rest of the ecosystem (including this
  package's own OpenAI and, if adopted, Gemini integrations) is moving away from —
  inviting a second migration shortly after the first.
* **Avoid inventing a shape from scratch.** The Responses `output` item model
  (message / reasoning / function_call / function_call_output) formalizes distinctions
  the existing per-provider parsers already draw informally: OpenAI's `tool_calls`
  alongside `content`, Claude's typed `content` blocks (`"text"` vs `"thinking"` vs
  `"tool_use"` — see the `TestClaude_MultiBlockContent` regression above), and Gemini's
  `parts` (`text` vs `functionCall`). Canonicalizing on an item model is largely making
  explicit a distinction this package's code already has to reason about per provider.
* **Backward-compatibility blast radius.** `Provider.Generate(ctx, prompt string)
  (string, error)` is depended on by external consumers
  (mcp-server-magictools, mcp-server-magicdev) that this MADR cannot inspect directly,
  because those consumer modules live outside this checkout (see Context above). A
  canonical redesign that removes or changes the signature of `Generate`/
  `GenerateWithTool` is a breaking change for every consumer, not just an additive one
  for Grok.
* **Claude has no server-side statefulness to converge to.** Any canonical interface
  must remain implementable by a provider that is, and based on current Anthropic
  documentation will remain, purely stateless. The canonical shape's *item* structure
  (typed output) can still apply to Claude's existing typed content blocks; the
  canonical shape's *state* structure (response IDs / `previous_response_id`-equivalent
  chaining) cannot, and must degrade to caller-managed history replay for Claude without
  making that provider a second-class citizen the way Ollama is today.
* **Package's SDK-free constraint is unaffected.** All of OpenAI's, xAI's, Gemini's, and
  Claude's endpoints discussed here are documented, stable REST/JSON endpoints reachable
  with `net/http` — adopting the Responses shape does not require adopting a vendor SDK.
* **Test-pattern continuity.** The existing `httptest`-based, no-mock test pattern must
  remain viable against whichever shape is chosen; a canonical item model is more
  test-fixture work per provider than the flat-string model, but is still plain
  JSON-over-HTTP and fits `httptest.NewServer` the same way today's tests do.
* **Idiomatic, canonical Go for this module's actual standard-library baseline
  (`go 1.26.5`), not a generic best-practices checklist.** Per explicit direction, the
  redesign should follow (a) what Go 1.23–1.26 shipped in the standard library —
  sealed-interface polymorphism (the `go/ast` idiom), `iter.Seq`/`iter.Seq2` for
  sequence-shaped data, plain `encoding/json` (since `encoding/json/v2` is still
  experimental in 1.26) — and (b) design choices independently converged on by
  comparable Go LLM-provider OSS projects surveyed above, while explicitly preserving
  the choices this package already has right (typed sentinel errors, opt-in retry as a
  decorator, functional options) rather than discarding them for novelty's sake.

## Considered Options

* Canonicalize the `Provider` contract on a Responses-API-shaped item/state model across all four providers (existing three, migrated, plus a new Grok provider built natively on this shape)
* Add Grok only, on xAI's native Responses API, without changing the shared `Provider` interface (translate Grok's item-based output back down to today's flat `Generate`/`GenerateWithTool` strings)
* Add Grok only, on the legacy Chat Completions API, following the existing `OpenAIProvider` template unchanged (the MADR's original, narrower scope)
* Listing-only integration for Grok (Ollama precedent): no `Provider` struct, `ListAvailableModels` support only

## Decision Outcome

Chosen option: "Canonicalize the `Provider` contract on a Responses-API-shaped
item/state model across all four providers," because it is the only option that treats
the cross-vendor convergence documented above as an architectural signal rather than an
implementation detail to be worked around, and it directly satisfies the scope
expansion requested for this MADR — the Responses API shape becomes canonical **across
the provider interfaces**, not a one-off adaptation layer bolted onto Grok alone.

### What "canonical" means for this package (design direction; exact Go signatures are implementation-plan territory)

* A new typed **item model** replaces the flat `string` used today for both input and
  output: at minimum a `MessageItem` (role + text), `FunctionCallItem` (name,
  arguments, call ID), and `FunctionCallOutputItem` (call ID, output), mirroring the
  `type` discriminants already present in OpenAI's/xAI's `output[]`, Gemini's
  Interactions `steps[]`, and — read back out of the existing per-provider Go structs
  documented above — the distinctions Claude's and Gemini's current code already parses
  by hand. **Per the idiomatic-Go research above, `Item` should be a sealed interface**
  (an unexported marker method, e.g. `itemType() string` or similar, satisfied only by
  types defined inside `llmprovider`), the same restricted-polymorphism idiom the Go
  standard library itself uses for `go/ast`'s `Node`/`Decl`/`Stmt`/`Expr` family, and
  independently converged on by `pi-llm-go`'s `Block`/`StreamEvent` sum types (see
  survey table above) — not a string-discriminator-plus-`any` shape, which is the
  pattern this package currently uses informally and inconsistently across
  `openai.go`/`claude.go`/`gemini.go`'s ad hoc anonymous decode structs. A sealed
  interface lets callers `switch` exhaustively on item type with compiler-checked
  coverage, and prevents callers outside the package from fabricating invalid item
  values.
* A new **`Response` result type** replaces the bare `string` returned by `Generate`
  today: at minimum an opaque provider-issued `ID` (empty for providers that do not
  support one) and an `Output []Item` slice, with a convenience accessor equivalent to
  the OpenAI/xAI SDK's `output_text` for callers that only want the final text. Per the
  Go 1.23+ `iter` package being this module's actual standard-library baseline (already
  depended on transitively via `slices.Contains`, `models_catalog.go:107`), `Response`
  should also expose an `All() iter.Seq[Item]` (or similar) accessor alongside the plain
  `Output []Item` field — a slice-returning method plus an iterator-returning method is
  the exact pattern the standard library itself now follows for `slices`/`maps`
  (`slices.Values`, `maps.All`, etc.), and keeps this design forward-compatible with a
  future streaming addition (e.g. `iter.Seq2[Item, error]`, matching Google's Go ADK
  `LLM.GenerateContent` streaming shape from the survey above) without requiring a
  second breaking change to `Response` if streaming is ever added — noting again that
  streaming itself remains explicitly out of scope for this decision.
* A new **optional interface**, analogous to `ThinkingProvider`/`ToolProvider` today,
  for providers that support native server-side chaining — e.g. a
  `Continue(ctx, previousResponseID string, items ...Item) (*Response, error)` shape —
  implemented by OpenAI (via `/v1/responses`), xAI/Grok (via `/v1/responses`), and Gemini
  (via the Interactions API's `previous_interaction_id`). Claude does **not** implement
  this optional interface; it satisfies only the base, non-chaining item contract, with
  the package (or the caller) responsible for replaying prior items as `messages` on
  every call, exactly as Claude's Messages API already requires today. This mirrors how
  `ThinkingProvider`/`ThinkingToolProvider` are already optional per-provider
  capabilities (`provider.go:46-61`) rather than universal requirements — the precedent
  for "some providers implement more optional interfaces than others" already exists in
  this package, and is exactly the same small-interface, capability-via-type-assertion
  idiom the standard library uses for e.g. `http.Flusher`/`http.Hijacker` on top of
  `http.ResponseWriter`.
* Grok's `reasoning_effort` model-gating (see xAI facts above: rejected outright on
  `grok-4`/`grok-3`/`grok-code-fast-1`, restricted to `"low"`/`"high"` on
  `grok-3-mini`, richer on `grok-4.5`/`grok-4.6`) should be implemented as a **small,
  explicit per-provider capability-resolver function** (e.g.
  `grokReasoningSupport(model string) reasoningSupport`, returning an enum such as
  `reasoningUnsupported` / `reasoningGatedLowHigh` / `reasoningFull`), following the
  capability-resolver pattern surveyed in `flexigpt/inference-go` above, rather than
  scattering `strings.HasPrefix`/`strings.Contains` checks inline in the request-building
  code the way `models_catalog.go`'s existing `isUsable*`/`Rank*` functions do for model
  *filtering* (a different, already-acceptable use of that style — see
  `models_catalog.go:100-164` — but request-parameter validity is a correctness
  concern, not a menu-curation concern, and deserves a named, testable function with its
  own unit tests per gating tier, not inline string matching next to JSON body
  construction).
* Existing string-based methods (`Generate`, `GenerateWithTool`,
  `GenerateThinking`, `GenerateWithToolThinking`) are **not deleted** by this decision;
  see Consequences below on migration sequencing and consumer blast radius. Whether they
  become thin wrappers over the new item-based methods, are deprecated in place, or are
  removed on a longer timeline is implementation-plan and consumer-coordination work,
  not resolved here.
* The package's **existing idiomatic patterns are preserved unchanged** through this
  redesign, per the validation finding above: typed sentinel errors consumed via
  `errors.Is`/`errors.As` (`provider.go:64-85`), functional options
  (`options.go:38-77`), and — most importantly — `GenerateWithRetry` remaining a
  standalone function layered over the `Provider` interface rather than retry logic
  moving into individual providers (`provider.go:112-159`). No comparable OSS project
  surveyed contradicts these choices; two independently confirm them. This redesign is
  scoped to the request/response *shape*, not the error/retry/config *architecture*,
  which does not need to change.
* Grok is implemented **natively on the canonical shape from day one** — it is a new
  provider with no legacy callers depending on a flat-string `GrokProvider`, so it need
  not carry the transitional baggage that OpenAI/Claude/Gemini's migration requires.

### Consequences

* Good, because Grok launches already aligned with where OpenAI, xAI, and Gemini's own
  APIs are heading, avoiding a near-term second migration for the newest provider in the
  package.
* Good, because the item model is not a foreign import — it formalizes distinctions
  (`text` vs `thinking` vs `tool_use` for Claude; `text` vs `functionCall` for Gemini;
  `content` vs `tool_calls` for OpenAI) that every provider's current decode logic
  already has to handle internally; canonicalizing surfaces that structure to callers
  instead of collapsing it away.
* Good, because the five existing registration touch points (`constants.go`,
  `ProviderEnvVars`, `NewProvider` switch, `ListAvailableModels` switch, `StaticModels`)
  are unaffected by this decision — canonicalization changes what a provider returns,
  not how providers are named, keyed, or looked up.
* Good, because the sealed-interface `Item` design is not a speculative pattern being
  imposed on this codebase — it is the same idiom the Go standard library has used for
  `go/ast` for over a decade, and the same idiom `pi-llm-go` independently converged on
  for the identical problem (typed, provider-varying LLM response content). Choosing it
  here is following established Go convention, not inventing one.
* Good, because this redesign does not require touching the package's error-handling,
  retry, or configuration architecture — the survey of comparable OSS projects found
  two independent projects (`nocturnium/llm-go-sdk`, `mozilla-ai/any-llm-go`) that
  converged on the exact typed-sentinel-error and opt-in-retry-decorator design this
  package already has, meaning that part of the redesign work is zero, not merely small.
* Bad, because this is now a substantially larger change than the MADR's original
  scope: it requires migrating `OpenAIProvider` off Chat Completions onto `/v1/responses`
  and `GeminiProvider` off `generateContent` onto the Interactions API, in addition to
  building `GrokProvider`. That is three provider rewrites plus one new provider, not
  one new provider, and each rewrite carries its own regression risk against existing
  callers (e.g., the exact multi-block-content bug class that
  `TestClaude_MultiBlockContent` guards against must be re-verified for OpenAI's and
  Gemini's new output-item parsing, not assumed away).
* Bad, because `Provider.Generate(ctx, prompt string) (string, error)` and its three
  siblings are almost certainly depended on by mcp-server-magictools and
  mcp-server-magicdev in their current flat-string form; this MADR cannot verify those
  call sites because those consumer modules simply are not present in this checkout
  (they live in sibling repos/modules resolved via the monorepo's `go.work`, which is
  not part of this repo). Any implementation plan
  must either (a) keep the existing four methods as permanent, non-deprecated
  compatibility wrappers over the new item-based methods, or (b) explicitly coordinate a
  breaking-change rollout with both consumers before removing them. Silently changing
  `Generate`'s signature is out of scope for this decision and is not what "canonical"
  is intended to authorize.
* Bad, because Claude structurally cannot benefit from the "state" half of this
  decision — no `previous_response_id`-equivalent exists today, per Anthropic's own
  current documentation, and there is no announced roadmap item indicating one is
  coming. The canonicalization's value for Claude is limited to the item/output-typing
  half; treating the state half as "coming later for Claude too" would be an assumption
  this MADR explicitly does not make.
* Bad, because `reasoning_effort` model-gating for Grok (documented in the original
  research: 400 on `grok-4`/`grok-3`/`grok-code-fast-1`, required-and-restricted on
  `grok-3-mini`, richer options on `grok-4.5`/`grok-4.6`) is unchanged by this decision
  and remains new logic with no precedent in the existing three providers' reasoning
  fields, all of which apply their reasoning parameter unconditionally today. This
  revision narrows that gap by recommending a named capability-resolver function
  (design direction above) rather than leaving it fully open, but the function itself
  still needs to be designed, written, and unit-tested per reasoning tier in the
  implementation plan — it does not yet exist.
* Neutral, because xAI's Responses API and Chat Completions API are both currently
  supported (xAI has not announced an end-of-life date for Chat Completions), so
  technically the narrower "Grok on Chat Completions" option remained available; it is
  rejected here specifically because it does not fulfill the requested scope of making
  the Responses shape canonical across providers, not because Chat Completions is
  imminently unusable.
* Neutral, because the canonical provider identifier and env var naming remain open
  choices this MADR does not fix (as in the original scope): `ProviderGrok = "grok"`
  (product-name convention, matching `"gemini"`/`"claude"`) and
  `ProviderEnvVars[ProviderGrok] = "XAI_API_KEY"` (matching xAI's own documented env var
  name) are recommended defaults for the implementation plan to confirm, not
  architectural forks with real trade-offs.

### Confirmation

This decision will be considered correctly implemented when:

* A canonical item type and `Response` result type exist in `llmprovider` and are used
  by `GrokProvider`'s implementation of `Provider` and its optional interfaces, built
  against xAI's `POST /v1/responses`.
* `OpenAIProvider` and `GeminiProvider` are migrated to call `/v1/responses` and the
  Interactions API respectively, and their existing string-based methods continue to
  pass their current test suites (`provider_correctness_test.go`, `thinking_test.go`,
  `gemini_test.go`) unchanged in observable behavior, whether as compatibility wrappers
  or otherwise — i.e., no regression for existing consumers.
* `ClaudeProvider` implements the base item/output contract without the
  server-side-chaining optional interface, and a code comment or doc note records that
  this is a permanent (not TODO/transitional) limitation tied to Anthropic's current
  Messages API design, not an oversight.
* Unit tests using `httptest.NewServer` exist for all four providers' item-based
  request/response parsing, including a Grok-equivalent of
  `TestClaude_MultiBlockContent` verifying that reasoning/tool-call output items
  interleaved with message items are parsed correctly rather than assumed to be at a
  fixed index.
* `go vet`, the package's `golangci-lint` config, and `make -C scripts/go/mcplib test
  lint` pass with all changes included.
* The new `Item` type is a sealed interface (unexported marker method) with an
  exhaustive type switch used wherever a provider decodes output, not a
  string-discriminator-plus-`any` shape, and `Response` exposes both a plain
  `Output []Item` field and an `iter.Seq[Item]`-returning accessor.
* A named capability-resolver function exists for Grok's `reasoning_effort` gating
  (one of the three documented tiers: unsupported / low-high-only / full-range), with
  a unit test per tier confirming the parameter is included, restricted, or omitted as
  xAI's docs require, rather than the gating logic being inline string matching next to
  request-body construction.
* `GenerateWithRetry`, the typed sentinel errors (`ErrRateLimited`, `ErrAuthFailure`,
  `ErrProviderUnavailable`, `ErrInvalidRequest`), and the functional-options pattern
  are unchanged in behavior and location — confirming the redesign stayed scoped to
  request/response shape as intended, not the error/retry/config architecture.
* An implementation plan exists and is approved separately (per this repository's MADR
  workflow) before any of the above is implemented, sequencing the four
  provider changes and the compatibility-wrapper decision for existing consumers.

## Pros and Cons of the Options

### Canonicalize on the Responses-API item/state model across all four providers (chosen)

* Good, because it matches the direction all three convergent vendors have already
  taken, and lets Grok, OpenAI, and Gemini share one internal shape instead of three
  bespoke ones plus a fourth bespoke shape for Grok.
* Good, because it makes the previously-ad-hoc per-provider distinctions (thinking
  blocks, tool calls, function calls) into a first-class, uniformly-tested concept.
* Bad, because it is the largest option by implementation size: three provider
  migrations plus one new provider, versus one new provider alone.
* Bad, because it carries real breaking-change risk for external consumers this MADR
  cannot directly verify, requiring an explicit compatibility strategy before any code
  changes land.

### Grok only, on the Responses API, without changing the shared `Provider` interface

* Good, because it gets Grok onto xAI's modern, actively-developed endpoint (avoiding
  the "legacy" Chat Completions label) without touching OpenAI, Claude, or Gemini at
  all — zero blast radius for existing consumers.
* Good, because it is a strict subset of the chosen option's work and could serve as an
  incremental first phase toward it.
* Bad, because it does not fulfill the requested scope: the Responses shape would be
  Grok-specific plumbing translated back down to the old flat-string contract, not a
  canonical shape "across the provider interfaces." Any richer item data xAI returns
  (reasoning items, multiple output items) would be discarded at the translation
  boundary exactly as OpenAI's/Gemini's/Claude's richer structures are discarded today.
* Bad, because it defers, rather than resolves, the eventual redesign this MADR's
  broadened scope was asked to address — the package would still have three legacy-shape
  providers and one canonically-shaped-but-flattened provider.

### Grok only, on the legacy Chat Completions API (original scope of this MADR)

* Good, because it is the smallest, lowest-risk option, reusing the `OpenAIProvider`
  template near-verbatim.
* Bad, because it explicitly contradicts the requested scope expansion — it builds the
  newest provider in the package on the one API shape every convergent vendor is moving
  away from.
* Bad, because it commits to an endpoint xAI has explicitly deprioritized for new
  features, which would likely require a second migration once the canonical shape is
  adopted for the rest of the package.

### Listing-only integration (Ollama precedent)

* Good, because it is the smallest possible change and has direct precedent in this
  same package (`discovery.go:31-32`).
* Bad, because it does not deliver a usable Grok provider at all — no `Generate`, no
  tool calling, no way to actually run a Grok model through this package.
* Bad, because it is orthogonal to the scope-defining question (Responses-API
  canonicalization) this MADR now addresses, and would leave
  `llmprovider.NewProvider(llmprovider.ProviderGrok, ...)` unsupported, inconsistent with
  the other three `Provider*` constants.

## More Information

* Primary sources on xAI: `docs.x.ai/developers/rest-api-reference/inference/chat`,
  `docs.x.ai/developers/model-capabilities/legacy/chat-completions`,
  `docs.x.ai/developers/model-capabilities/text/comparison`,
  `docs.x.ai/developers/model-capabilities/text/reasoning`,
  `docs.x.ai/developers/tools/function-calling`, `docs.x.ai/developers/tools/overview`,
  `docs.x.ai/developers/rate-limits`, `docs.x.ai/developers/debugging`,
  `docs.x.ai/developers/rest-api-reference/inference/models`.
* Primary sources on OpenAI's Responses API:
  `developers.openai.com/api/docs/guides/migrate-to-responses`,
  `developers.openai.com/api/docs/guides/conversation-state`,
  `developers.openai.com/blog/responses-api`,
  `developers.openai.com/api/reference/resources/responses/`.
* Primary sources on Google Gemini's Interactions API:
  `ai.google.dev/gemini-api/docs/interactions/interactions-overview`,
  `ai.google.dev/gemini-api/docs/migrate-to-interactions`,
  `ai.google.dev/gemini-api/docs/caching` (Interactions-API version).
* Primary sources establishing Claude's Messages API has no Responses-style analog:
  `platform.claude.com/docs/en/build-with-claude/working-with-messages`,
  `platform.claude.com/docs/en/claude_api_primer` — both state the Messages API is fully
  stateless as of current documentation, with no response-ID or server-managed-history
  concept. All sources retrieved/verified 2026-08-17.
* Secondary/corroborating sources for the Grok `reasoning_effort` model-gating claim
  (used only to cross-check the primary xAI docs, not as an authority over them):
  `github.com/superagent-ai/grok-cli` issue #198 and commit `75efd70`, and the
  `axl-sdk/axl` `xai.ts` provider profile.
* Primary sources on this module's Go standard-library baseline:
  `go.dev/doc/go1.24`, `go.dev/doc/go1.25`, `go.dev/doc/go1.26`, `go.dev/blog/go1.26`,
  `go.dev/blog/range-functions`, plus the local `go.mod:3` (`go 1.26.5`) and `go version`
  output confirming the toolchain in use.
* Comparable Go OSS LLM-provider projects surveyed for idiomatic design precedent
  (retrieved 2026-08-17, used as corroborating design precedent only):
  `engineeredintelligence.substack.com` (Google ADK Go `iter.Seq2` streaming analysis),
  `pkg.go.dev/github.com/amit-timalsina/pi-llm-go`,
  `github.com/nocturnium/llm-go-sdk`, `blog.mozilla.ai` (`any-llm-go` release post),
  `github.com/flexigpt/inference-go`, `github.com/tmc/langchaingo` (discussion #1282).
* This MADR intentionally stops short of a full implementation plan. Per this
  repository's workflow, a paired `0001-PLAN-add-grok-xai-llm-provider.md` should be
  written and approved separately before any source changes are made. That plan must, at
  minimum: (1) define the exact Go item/`Response` types and optional-interface
  signatures sketched under "Decision Outcome" above, including the sealed-interface
  marker method's exact name/signature and the full enumerated set of `Item`
  implementations per provider; (2) decide the compatibility strategy for the four
  existing string-based methods against mcp-server-magictools/mcp-server-magicdev;
  (3) sequence the OpenAI and Gemini migrations relative to the new Grok provider;
  (4) resolve the canonical name/env-var strings flagged as "Neutral" consequences;
  (5) design the `reasoning_effort` capability-resolver function for Grok (exact enum
  values, exact model-to-tier mapping, and its unit tests) flagged as a "Bad"
  consequence; and (6) confirm whether/when an `iter.Seq[Item]`-returning accessor is
  added to `Response` alongside the plain slice field, per the forward-compatibility
  recommendation above.
