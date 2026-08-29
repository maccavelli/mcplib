---
status: accepted
date: 2026-08-29
parent-madr: 0004-MADR-canonicalize-llm-provider-configuration.md
decision-makers: mcplib maintainers
---

# Implementation Plan: Canonicalize LLM Provider Configuration in `mcplib`

> Paired with [0004-MADR-canonicalize-llm-provider-configuration.md](0004-MADR-canonicalize-llm-provider-configuration.md)
> (revision 2). Moves provider descriptors, the configuration flow, and secret
> prompting into `mcplib` behind a renderer-agnostic `Prompter` interface, and adds an
> `OllamaProvider` so every descriptor maps to a constructible provider.

> **Prerequisite satisfied.** MADR 0004 was blocked on MADR/PLAN 0003. That work is
> **merged to `main`** (`50ac165`, 25 files, +3,813 lines, `make lint` 0 issues). This plan
> therefore opens with **eight** registered providers and can build `OllamaProvider` on the
> `chatcompletions.go` primitive 0003 introduced.

## Table of Contents

- [0. Notation and Conventions](#0-notation-and-conventions)
- [1. Baseline State (verified 2026-08-29, post-0003)](#1-baseline-state-verified-2026-08-29-post-0003)
- [2. Reuse Inventory](#2-reuse-inventory)
- [3. Authoritative Descriptor Data](#3-authoritative-descriptor-data)
- [4. Type and Signature Definitions](#4-type-and-signature-definitions)
- [5. Phase Sequencing Overview](#5-phase-sequencing-overview)
- [Phase 1 — `MaskSecret` in `mcplib/logging`](#phase-1--masksecret-in-mcpliblogging)
- [Phase 2 — `ProviderDescriptor` and `ModelLabel`](#phase-2--providerdescriptor-and-modellabel)
- [Phase 3 — `OllamaProvider`](#phase-3--ollamaprovider)
- [Phase 4 — `mcplib/wizard`: `Prompter` and `TextPrompter`](#phase-4--mcplibwizard-prompter-and-textprompter)
- [Phase 5 — `wizard.ConfigureLLM`](#phase-5--wizardconfigurellm)
- [Phase 6 — Migrate `prepare-commit-msg`](#phase-6--migrate-prepare-commit-msg)
- [Phase 7 — Migrate `mcp-server-magicdev`](#phase-7--migrate-mcp-server-magicdev)
- [Phase 8 — Migrate `mcp-server-magictools`](#phase-8--migrate-mcp-server-magictools)
- [9. Verification Commands](#9-verification-commands)
- [10. Acceptance Criteria](#10-acceptance-criteria)
- [11. Decisions Resolved by This Plan](#11-decisions-resolved-by-this-plan)
- [12. Risks and Mitigations](#12-risks-and-mitigations)
- [13. Out of Scope](#13-out-of-scope)
- [14. File Summary](#14-file-summary)
- [15. Deviation Log](#15-deviation-log)

---

## 0. Notation and Conventions

- **File references** use `path/file.go:L<start>` against the checkout snapshot of
  **2026-08-29**, post-0003-merge (`git log -1` → `50ac165`).
- `make test` / `make vet` / `make lint` are `mcplib`'s Makefile targets
  ([Makefile:16-17](../Makefile#L16-L17), [29-30](../Makefile#L29-L30),
  [32-38](../Makefile#L32-L38)). Consumer repos have their own; each phase names which.
- **"Phase green"** means, in the repo that phase touches: `gofmt -l` prints nothing, and
  `vet`, `lint` and `test` all exit 0.
- Each phase ends with **one commit in the repo it touches**. **No `git push`.**
- **Cross-repo phases (6–8) are gated on an `mcplib` release.** All three consumers pin
  `github.com/maccavelli/mcplib v1.0.1`. Until a version carrying Phases 1–5 is tagged and
  published, each consumer phase begins by adding a local `replace` directive
  (Step 6.0 pattern) so the work is verifiable, and ends by **leaving that directive in
  place with a TODO**. Removing it and pinning a real version is a maintainer action
  requiring a push and a tag — explicitly outside this plan.
- Code blocks are the **intended final source**, not sketches.

---

## 1. Baseline State (verified 2026-08-29, post-0003)

| Check | Result |
|---|---|
| `mcplib` on `main` at `50ac165` | `gofmt` clean, `vet` clean, `make lint` **0 issues**, all 6 packages `ok` |
| Registered providers | **8**: `gemini`, `openai`, `claude`, `grok`, `opencode-zen`, `opencode-go`, `huggingface`, `kilo` |
| Ollama status | **listing-only** — `discovery.go:42` `case "ollama"`, `listOllamaModels` at `discovery.go:199`, `ValidateOllamaURL` at `discovery.go:241`. No `Provider` struct, so `NewProvider("ollama", …)` fails. |
| `mcplib` direct dependencies | **2** — `invopop/jsonschema`, `modelcontextprotocol/go-sdk`. No UI dependency, no terminal I/O. |
| Consumers depending on `mcplib` | **12**; **3** have LLM wizards |

**Consumer wizard anchors, re-verified at these exact lines:**

| Repo | File | Lines | Symbols this plan replaces |
|---|---|---|---|
| `mcp-server-magictools` | `cmd/mcp-server-magictools/config.go` | 806 | `readHiddenSecret:565`, `maskKey:576`, `selectProviderForTier:601`, `resolveAPIKey:621`, `promptOllamaURL:676`, `selectModel:696` |
| `mcp-server-magicdev` | `cmd/mcp-server-magicdev/configure.go` | 610 | `maskKey:107`, `resolveAPIKey:118`, `readHiddenSecret:182`, `providerEnvVars:27` |
| `prepare-commit-msg` | `internal/ui/setup.go` | 548 | `promptProvider:71`, `resolveAPIKey:111`, `modelAnnotations:173`, `discoverModels:210`, `promptModel:227`, `promptFallbacks:284` |

**The drift these anchors encode** (MADR §"Measured drift"): three provider lists, three
env-var sources, two byte-identical `maskKey` copies, a 17-entry shadow model catalog, and
`gemini-2.0-flash` recommended by `magictools` while `models_catalog.go:26` documents it as
shut down. Grok has been in `mcplib` since MADR 0001 and appears in **none** of the three.

---

## 2. Reuse Inventory

Phases 1–5 add to `mcplib`; almost everything they need already exists.

| Need | Existing symbol | Location | Reusable? |
|---|---|---|---|
| Chat Completions request body | `chatCompletionsBody` | `llmprovider/chatcompletions.go:53` | **Yes** — `OllamaProvider` uses it verbatim |
| Chat Completions decoder | `decodeChatCompletionsResponse` | `llmprovider/chatcompletions.go:97` | **Yes** |
| Status → sentinel | `classifyHTTPStatus` | `llmprovider/http_helpers.go:90` | **Yes** |
| Catalog curation | `curateFromCatalog` | `llmprovider/models_catalog.go:320` | **Yes — unchanged** |
| Health probe | `probeGenerateHealth` | `llmprovider/probe.go:13` | **Yes** |
| Ollama model listing | `listOllamaModels` | `llmprovider/discovery.go:199` | **Yes — kept on `/api/tags`** (MADR decision 5) |
| Ollama reachability | `ValidateOllamaURL` | `llmprovider/discovery.go:241` | **Yes** |
| Static catalogs per provider | `StaticModels` | `llmprovider/models_catalog.go:162` | **Yes** — descriptors derive from it |
| Env var map | `ProviderEnvVars` | `llmprovider/provider.go:113` | **Yes** — descriptors derive from it |
| Secret redaction precedent | `Redact` / `RedactString` | `logging/redact.go:54,81` | Sibling; `MaskSecret` joins it |
| Body-capturing test server | `captureServer` | `llmprovider/thinking_test.go:14` | **Yes** |

**Net-new in `mcplib`:** `MaskSecret`, `ProviderDescriptor` + `Descriptors()` +
`ModelLabel`, `OllamaProvider`, and the `wizard` package (`Prompter`, `TextPrompter`,
`ConfigureLLM`). `OllamaProvider` contributes **no new wire logic**.

---

## 3. Authoritative Descriptor Data

### 3.1 The nine descriptors

Derived from data `llmprovider` already holds; `Label` and `Notes` are new.

| ID | Label | EnvVar | RequiresAPIKey | SupportsBaseURL | IsLocal | Notes |
|---|---|---|---|---|---|---|
| `gemini` | Gemini (Google) | `GEMINI_API_KEY` | yes | no | no | |
| `openai` | OpenAI | `OPENAI_API_KEY` | yes | no | no | |
| `claude` | Claude (Anthropic) | `CLAUDE_API_KEY` | yes | no | no | |
| `grok` | Grok (xAI) | `XAI_API_KEY` | yes | no | no | |
| `opencode-zen` | OpenCode Zen | `OPENCODE_API_KEY` | yes | yes | no | pay-as-you-go; free models available |
| `opencode-go` | OpenCode Go | `OPENCODE_API_KEY` | yes | yes | no | $10/month subscription |
| `huggingface` | Hugging Face | `HF_TOKEN` | yes | yes | no | monthly credits; no free tier |
| `kilo` | Kilo Gateway | `KILO_API_KEY` | yes | yes | no | free models available |
| `ollama` | Ollama (local) | *(none)* | **no** | **yes** | **yes** | runs on your machine |

`Descriptors()` returns them in exactly this order — remote-first, local last — so every
menu is identically ordered without each wizard choosing.

### 3.2 Ollama facts (verified live 2026-08-29 against a running v0.31.1)

| Probe | Result |
|---|---|
| `GET /api/version` | `200 {"version":"0.31.1"}` |
| `GET /v1/models` | `200`, OpenAI list shape — same as the 0003 gateways |
| `POST /v1/chat/completions`, **no auth header** | `200`, standard envelope, `content:"ALPHA"` |
| `message` keys returned | `content`, `role` |

From `https://docs.ollama.com/api/openai-compatibility`:

* base `http://localhost:11434/v1`;
* an API key is **"required but ignored"** → `RequiresAPIKey: false`, `NewOllama` accepts an
  empty key and sends no `Authorization` header;
* **`tool_choice` is NOT supported** → maps exactly onto `chatCompletionsOpts.ForceTool`,
  the flag 0003 added for Kilo. Ollama sets `ForceTool: false`;
* `reasoning_effort` **is** supported, values `none|low|medium|high|**max**`. Note `max`,
  not this package's `xhigh` (`constants.go:63`) — the provider clamps `xhigh` → `max`.

**Listing stays on the native `/api/tags`** (MADR decision 5): `listOllamaModels`
(`discovery.go:199`) and `ValidateOllamaURL` (`discovery.go:241`) are working, tested code
against Ollama's stable native API, and `/v1/models` is a compatibility shim. Ollama is
therefore the one provider whose lister does not resemble the others — deliberately.

### 3.3 Model display labels

`prepare-commit-msg`'s `modelAnnotations` (`setup.go:173-194`, 17 entries) becomes
`ModelLabel(provider, model) string` in `models_catalog.go`, beside the catalogs it
annotates, so a catalog edit and its label edit are one diff. Labels are **advisory**: an
unknown model returns its bare id, never an error, so a live listing that outruns the label
table degrades to raw ids rather than hiding models.

---

## 4. Type and Signature Definitions

### 4.1 `mcplib/logging` (Phase 1)

```go
// MaskSecret renders a credential for human identification: a fixed-width mask
// plus the last four runes, e.g. "••••••••a75y".
//
// This is the opposite intent to Redact, which hides a secret completely for
// logs. MaskSecret reveals a suffix ON PURPOSE so a person can tell which key
// they are looking at. Never use it on a value that will be logged.
//
// Fewer than minMaskedRunes (8) runes reveals nothing: on a short or partial
// value, four runes is most of the secret. Empty input returns "—".
// Rune-safe: counts and slices runes, never bytes.
func MaskSecret(s string) string
```

### 4.2 `llmprovider` (Phases 2–3)

```go
type ProviderDescriptor struct {
    ID              string
    Label           string
    EnvVar          string
    DefaultBaseURL  string
    SupportsBaseURL bool
    IsLocal         bool
    RequiresAPIKey  bool
    StaticModels    []string
    Notes           string
}

func Descriptors() []ProviderDescriptor
func DescriptorFor(id string) (ProviderDescriptor, bool)
func ModelLabel(provider, model string) string

const ProviderOllama = "ollama"

type OllamaProvider struct{ /* … */ }
func NewOllama(apiKey, model string, opts ...ProviderOption) (*OllamaProvider, error)
```

`ProviderEnvVars`, `StaticModels` and the `Provider*` constants stay exported and
unchanged; descriptors are **derived from** them, so no existing caller breaks.

### 4.3 `mcplib/wizard` (Phases 4–5)

```go
type Level int
const (LevelInfo Level = iota; LevelWarn; LevelError)

type Choice struct{ Label, Detail string }

type Prompter interface {
    Select(title string, choices []Choice, defaultIdx int) (int, error)
    MultiSelect(title string, choices []Choice, preselected []int) ([]int, error)
    Confirm(question string, def bool) (bool, error)
    Input(prompt, def string) (string, error)
    Secret(prompt string) (string, error)
    Notify(level Level, format string, args ...any)
}

type TextPrompter struct{ In *os.File; Out io.Writer }
func NewTextPrompter() *TextPrompter

type Result struct {
    Provider, APIKey, Model, BaseURL string
    Fallbacks                        []string
}

type Options struct {
    Providers     []string
    Existing      Result
    AllowEnv      bool
    Discover      bool
    DiscoverLimit time.Duration
    NeedFallbacks bool
}

func ConfigureLLM(ctx context.Context, p Prompter, o Options) (Result, error)
```

Six methods, not five: `MultiSelect` exists for the fallback step (MADR decision 4).

---

## 5. Phase Sequencing Overview

Eight phases. Phases 1–5 are `mcplib` and independently shippable; 6–8 are consumer
migrations, each in its own repo, in ascending order of difficulty.

| Phase | Repo | Deliverable | New files | Modified |
|---|---|---|---|---|
| 1 | mcplib | `MaskSecret` | `logging/mask.go`, `logging/mask_test.go` | — |
| 2 | mcplib | `ProviderDescriptor`, `Descriptors()`, `ModelLabel` | `llmprovider/descriptor.go`, `descriptor_test.go` | `models_catalog.go`, `models_catalog_test.go` |
| 3 | mcplib | `OllamaProvider` | `llmprovider/ollama.go`, `ollama_test.go` | `constants.go`, `provider.go`, `descriptor.go`, `discovery.go`, `models_catalog.go`, `interface_test.go`, `provider_test.go` |
| 4 | mcplib | `Prompter` + `TextPrompter` | `wizard/prompter.go`, `wizard/text_prompter.go`, `wizard/text_prompter_test.go` | `go.mod` (+`x/term`) |
| 5 | mcplib | `ConfigureLLM` | `wizard/configure.go`, `wizard/configure_test.go`, `wizard/fake_prompter_test.go` | — |
| 6 | prepare-commit-msg | Migrate `internal/ui/setup.go` | — | `setup.go`, `setup_test.go`, `go.mod` |
| 7 | mcp-server-magicdev | Migrate `configure.go` | `cmd/.../pterm_prompter.go` | `configure.go`, `go.mod` |
| 8 | mcp-server-magictools | Migrate `config.go`, wrap `ProviderSpec` | `cmd/.../pterm_prompter.go` | `config.go`, `internal/provider/catalog.go`, `go.mod` |

Dependencies: 1 and 2 are independent; 3 needs 2 (its descriptor); 4 is independent of 1–3;
5 needs 2 and 4. Each of 6–8 needs 1–5 and is independent of the other two — so a consumer
migration can stall without blocking the rest.

---

## Phase 1 — `MaskSecret` in `mcplib/logging`

**Goal:** one masking function, rune-safe, replacing two byte-sliced copies.

### Step 1.1 — `logging/mask.go` (new)

```go
package logging

import "strings"

// minMaskedRunes is the length below which MaskSecret reveals nothing. Four
// revealed runes out of six is most of a short secret; out of forty it is a
// fingerprint. Eight is the smallest length at which the tail is a minority.
const minMaskedRunes = 8

// maskWidth is the fixed number of mask glyphs. Fixed rather than proportional
// so the rendering does not leak the credential's length.
const maskWidth = 8

// MaskSecret renders a credential for human identification … (doc comment per §4.1)
func MaskSecret(s string) string {
    r := []rune(strings.TrimSpace(s))
    switch {
    case len(r) == 0:
        return "—"
    case len(r) < minMaskedRunes:
        return strings.Repeat("•", maskWidth)
    default:
        return strings.Repeat("•", maskWidth) + string(r[len(r)-4:])
    }
}
```

### Step 1.2 — `logging/mask_test.go` (new)

1. **`TestMaskSecret_RevealsLastFour`** — a 40-rune key ends with its last four runes and
   contains none of the preceding ones.
2. **`TestMaskSecret_ShortInputRevealsNothing`** — 1..7 runes reveal no input character.
3. **`TestMaskSecret_Empty`** — `""` and `"   "` → `"—"`.
4. **`TestMaskSecret_RuneSafe`** — the regression for the byte-sliced copies: input ending
   in multi-byte runes (`"key-日本語テスト"`) yields **valid UTF-8** (`utf8.ValidString`) and
   ends with exactly the last four *runes*, not bytes.
5. **`TestMaskSecret_FixedWidth`** — masks for a 20- and a 60-rune input have identical
   length, so the rendering does not leak length.
6. **`TestMaskSecret_IsNotRedact`** — documents the distinction: `MaskSecret` reveals a
   suffix, `RedactString` does not, asserted on the same input.

### Step 1.3 — Verification

```bash
cd mcplib && gofmt -l ./logging && make vet && make lint && make test
```

**Acceptance:** six tests pass; `logging` gains no import beyond `strings`. **Commit.**

---

## Phase 2 — `ProviderDescriptor` and `ModelLabel`

**Goal:** one record per provider, derived from existing data, so adding a provider to
`mcplib` makes it appear in every menu.

### Step 2.1 — `llmprovider/descriptor.go` (new)

`Descriptors()` returns the eight registered providers in the §3.1 order (Ollama is added
in Phase 3, with its descriptor). Each entry's `EnvVar` comes from `ProviderEnvVars` and
`StaticModels` from `StaticModels(id)`, so those maps remain the source of truth:

```go
// ProviderDescriptor is the single source of truth for everything a
// configuration UI needs to know about a provider. Fields that already exist
// elsewhere in this package (EnvVar, StaticModels) are derived, not duplicated,
// so a change there cannot drift from what a wizard shows.
type ProviderDescriptor struct { /* … §4.2 … */ }

// descriptorSpecs holds only what is NOT derivable from existing package data.
var descriptorSpecs = []struct {
    id, label, defaultBaseURL, notes string
    supportsBaseURL, isLocal, requiresAPIKey bool
}{
    {id: ProviderGemini, label: "Gemini (Google)", requiresAPIKey: true},
    // … one row per provider, in menu order …
}

func Descriptors() []ProviderDescriptor  // builds from descriptorSpecs + ProviderEnvVars + StaticModels
func DescriptorFor(id string) (ProviderDescriptor, bool)
```

### Step 2.2 — `ModelLabel` in `models_catalog.go`

Transcribed from `prepare-commit-msg`'s `modelAnnotations` (`setup.go:173-194`), extended
to the models `mcplib` actually ships. Unknown model → its bare id.

### Step 2.3 — `llmprovider/descriptor_test.go` (new)

1. **`TestDescriptors_CoverEveryRegisteredProvider`** — the load-bearing test. Every key of
   `ProviderEnvVars` has a descriptor, **and** every descriptor with
   `RequiresAPIKey` has an entry in `ProviderEnvVars`. This is what makes "add a provider,
   every wizard updates" true; without it the drift returns silently.
2. **`TestDescriptors_DerivedFieldsMatchSource`** — each descriptor's `EnvVar` equals
   `ProviderEnvVars[id]` and `StaticModels` equals `StaticModels(id)`.
3. **`TestDescriptors_StableOrder`** — two calls return the same order, and mutating the
   returned slice does not affect the next call (defensive copy).
4. **`TestDescriptorFor`** — hit and miss.
5. **`TestModelLabel`** — a known model returns a label containing its id; an unknown model
   returns exactly its id; an empty model returns `""`.
6. **`TestDescriptors_NoStaleModels`** — no descriptor's `StaticModels` contains
   `gemini-2.0-` or `gemini-1.5-`, the families `models_catalog.go:26` documents as shut
   down. This is the assertion that would have caught `magictools`'s `gemini-2.0-flash`.

### Step 2.4 — Verification

```bash
cd mcplib && gofmt -l ./llmprovider && make vet && make lint && make test
go test ./llmprovider/ -run 'TestDescriptor|TestModelLabel' -v
```

**Acceptance:** all six pass, `Descriptors()` returns **8** entries. **Commit.**

---

## Phase 3 — `OllamaProvider`

**Goal:** close the descriptor model's only sharp edge — every descriptor becomes
constructible.

### Step 3.1 — `llmprovider/constants.go`

Add `ProviderOllama = "ollama"` to the identifier block, replacing the bare `"ollama"`
string literal at `discovery.go:42`.

### Step 3.2 — `llmprovider/ollama.go` (new)

```go
const ollamaBaseURL = "http://localhost:11434"

// wireShapesProbedOnOllama — probe-pin, same pattern as the 0003 gateways.
// Measured against a running v0.31.1: GET /v1/models returns the OpenAI list
// shape and POST /v1/chat/completions answers 200 with NO Authorization header.
const wireShapesProbedOnOllama = "2026-08-29"

// OllamaProvider implements Provider against a local Ollama instance using its
// OpenAI-compatible endpoint, so it reuses this package's shared Chat
// Completions primitive rather than adding a fifth wire format.
//
// Three properties differ from every other provider here, all from Ollama's
// published compatibility notes and confirmed live:
//
//   - An API key is "required but ignored", so NewOllama accepts an empty key
//     and sends no Authorization header at all.
//   - tool_choice is NOT supported, so ForceTool is always false: tools are
//     offered, never forced. This is the same seam Kilo uses.
//   - reasoning_effort accepts none|low|medium|high|max — "max", not this
//     package's "xhigh", which is clamped.
//
// Generation uses {baseURL}/v1/chat/completions; model listing deliberately
// stays on the native /api/tags (listOllamaModels), which is stable, tested,
// and not a compatibility shim.
type OllamaProvider struct{ /* … */ }
```

`doGenerateItems` mirrors `huggingface.go` with three differences: no `Authorization`
header, `ForceTool: false`, and `clampOllamaEffort(effortXHigh) == "max"`.

### Step 3.3 — Registration and descriptor

`provider.go`: `NewProvider` gains `case ProviderOllama: return NewOllama(apiKey, model, opts...)`.
**`ProviderEnvVars` gains no entry** — Ollama needs no credential, and adding an empty
string would make `TestDescriptors_CoverEveryRegisteredProvider` meaningless.
`descriptor.go` gains the Ollama row with `RequiresAPIKey: false`, `IsLocal: true`,
`SupportsBaseURL: true`, `DefaultBaseURL: ollamaBaseURL`.
`discovery.go:42` switches from `case "ollama"` to `case ProviderOllama`.
`models_catalog.go`: `StaticModels(ProviderOllama)` returns `nil` — installed models are
machine-specific, so there is no meaningful static catalog, and `listOllamaModels` is the
only sensible source. `Descriptors()` therefore carries an empty `StaticModels` for it,
which `ConfigureLLM` handles (Phase 5).

### Step 3.4 — `llmprovider/ollama_test.go` (new)

1. **`TestOllama_NoAuthHeader`** — the handler **fails the test** if `Authorization` is
   present, for both empty and non-empty configured keys.
2. **`TestOllama_NeverForcesToolChoice`** — request body carries `tools` but **no**
   `tool_choice`, because Ollama does not support it.
3. **`TestOllama_ClampsXHighEffort`** — `WithReasoningEffort(effortXHigh)` sends
   `reasoning_effort: "max"`; `effortMedium` passes through unchanged.
4. **`TestOllama_Generate`** — decodes via the shared primitive.
5. **`TestOllama_EmptyKeyAccepted`** — `NewOllama("", "llama3.2")` succeeds, unlike every
   other provider constructor.
6. **`TestOllama_ErrorClassification`** — 429/500/400 → sentinels; the error names `ollama`.
7. **`TestOllama_NoContinuer`** — negative interface assertion.
8. **`TestDescriptors_EveryDescriptorIsConstructible`** *(in `descriptor_test.go`)* — the
   payoff: loop every descriptor, call `NewProvider(d.ID, key, model)` with `key` empty when
   `!d.RequiresAPIKey`, and assert none errors. This is what "no sharp edge" means, enforced.
9. **`TestWireShapesProbedOn`** — extend the existing table with `ollama`.

`interface_test.go` gains `*OllamaProvider` in the nine all-satisfied groups;
`provider_test.go` adds `ProviderOllama` to its provider slice — noting that its
`TestNewProvider` passes `"test-key"`, which Ollama accepts and ignores.

### Step 3.5 — Verification

```bash
cd mcplib && gofmt -l ./llmprovider && make vet && make lint && make test
go test ./llmprovider/ -run 'TestOllama|TestDescriptors' -v
```

**Acceptance:** all nine pass; `Descriptors()` returns **9**; every descriptor constructs.
**Commit.**

---

## Phase 4 — `mcplib/wizard`: `Prompter` and `TextPrompter`

**Goal:** the renderer seam, plus a zero-toolkit default. This is the phase that adds
`mcplib`'s **third** direct dependency.

### Step 4.1 — `wizard/prompter.go` (new)

The six-method interface from §4.3, plus `Level` and `Choice`. No UI toolkit imported —
this file's only import is `fmt` for the `Level` stringer.

### Step 4.2 — `wizard/text_prompter.go` (new)

`go get golang.org/x/term@v0.43.0` — the version all three consumers already pin, so no
consumer sees a version bump.

`Select` / `MultiSelect` / `Confirm` / `Input` are numbered-list prompts over
`bufio.Scanner`. `MultiSelect` accepts comma-separated indices and re-prompts on an
unparseable entry.

**`Secret` is the masked-entry path**, and carries the two behaviours the MADR fixed:

```go
// Secret reads a credential with live masking. The last four runes are revealed
// AS YOU TYPE, so a paste is confirmed the instant it lands and the key is
// identifiable without pressing Enter (MADR decision 3). Below
// minRevealRunes (8) nothing is revealed, so a short or partial paste does not
// expose most of itself.
//
// The exposure is deliberate and was chosen over an on-submit reveal: four
// characters are on screen for the whole entry, visible in screen shares and
// recordings. See MADR revision 2, "Accepted trade-off".
//
// Raw mode is REQUIRED for live masking, and it fails on Git Bash / mintty.
// That fallback is not optional: without it those users cannot configure at
// all. On failure this prints the documented notice and falls back to a plain
// visible read — the behaviour prepare-commit-msg has today
// (internal/ui/setup.go:161), preserved here as the shared baseline.
func (p *TextPrompter) Secret(prompt string) (string, error)
```

Handling required: `\r` and `\n` submit; backspace/DEL remove one **rune**; Ctrl-C returns
an error; a bracketed-paste escape sequence is consumed rather than echoed; empty entry
re-prompts once.

### Step 4.3 — `wizard/text_prompter_test.go` (new)

Raw mode needs a TTY, so tests drive the **rendering function** rather than the terminal:
`renderSecret(runes []rune) string` is separated from the terminal I/O for exactly this
reason.

1. **`TestRenderSecret_LiveReveal`** — 12 runes → mask + last four; the revealed suffix
   matches the input's last four runes.
2. **`TestRenderSecret_BelowThreshold`** — 1..7 runes reveal nothing.
3. **`TestRenderSecret_RuneSafe`** — multi-byte input renders valid UTF-8.
4. **`TestSecret_FallbackWhenNotATTY`** — with a non-TTY reader, `Secret` returns the line
   and emits the "input will be visible" notice. This is the Git Bash guarantee.
5. **`TestSecret_EmptyReprompts`** — an empty first line re-prompts rather than returning `""`.
6. **`TestMultiSelect_ParsesIndices`** — `"1,3"` selects those; `"x"` re-prompts;
   empty returns the preselection.
7. **`TestSelect_DefaultOnEmpty`** — empty input returns `defaultIdx`.
8. **`TestTextPrompter_SatisfiesPrompter`** — compile-time assertion.

### Step 4.4 — Verification

```bash
cd mcplib && gofmt -l ./wizard && make vet && make lint && make test
go mod tidy && git diff go.mod   # must show exactly one added dependency: golang.org/x/term
grep -rn 'pterm\|bubbletea' go.mod ; echo "^ must be empty"
```

**Acceptance:** eight tests pass; `go.mod` gains **exactly** `golang.org/x/term`; neither
`pterm` nor `bubbletea` appears anywhere. **Commit.**

---

## Phase 5 — `wizard.ConfigureLLM`

**Goal:** the canonical flow, testable with no TTY.

### Step 5.1 — `wizard/configure.go` (new)

The flow, which is the intersection of what all three wizards already do:

1. **Provider** — `Select` over `Descriptors()`, filtered by `Options.Providers` when set.
   Each `Choice` is `{Label: d.Label, Detail: d.Notes}`.
2. **Base URL** — when `d.SupportsBaseURL`, `Input` with `d.DefaultBaseURL` as the default.
   For `d.IsLocal`, validate with `llmprovider.ValidateOllamaURL` and re-prompt on failure.
3. **Key** — skipped entirely when `!d.RequiresAPIKey`. Otherwise precedence
   **env → existing → prompt**, the order `prepare-commit-msg` implements at
   `setup.go:111-170`:
   * env var set and `AllowEnv` → `Confirm("Use $HF_TOKEN?")`, showing
     `logging.MaskSecret(envValue)` so the user sees *which* key;
   * `Options.Existing.APIKey` non-empty → `Confirm("Keep existing key (••••••••a75y)?")`;
   * otherwise `Secret`.
4. **Model** — when `Discover`, `llmprovider.ListAvailableModels` under `DiscoverLimit`,
   falling back to `StaticModels` on error **or empty result** (the Ollama-with-no-models
   case). Labelled with `ModelLabel`. Then `Select`.
5. **Fallbacks** — when `NeedFallbacks`, `MultiSelect` over the remaining models.

`ConfigureLLM` **returns** a `Result`; it never writes config. Persistence stays per-app.

### Step 5.2 — `wizard/fake_prompter_test.go` (new)

A scripted `Prompter` recording every call and replaying queued answers, erroring on an
unexpected prompt. This is what makes the flow testable without a TTY — coverage all three
wizards lack today.

### Step 5.3 — `wizard/configure_test.go` (new)

1. **`TestConfigureLLM_EnvKeyPrecedence`** — env set + `AllowEnv` + confirm → env key used,
   `Secret` never called.
2. **`TestConfigureLLM_KeepExisting`** — declining env, accepting existing → existing key,
   `Secret` never called.
3. **`TestConfigureLLM_PromptsWhenNothingAvailable`** — neither → `Secret` called once.
4. **`TestConfigureLLM_LocalProviderSkipsKey`** — selecting `ollama` never calls `Secret`
   and returns an empty `APIKey`.
5. **`TestConfigureLLM_DiscoveryTimeoutFallsBackToStatic`** — a lister that blocks past
   `DiscoverLimit` yields the static catalog, not an error.
6. **`TestConfigureLLM_EmptyDiscoveryFallsBackToStatic`** — a lister returning zero models
   behaves the same. Guards the Ollama-with-no-models path.
7. **`TestConfigureLLM_Fallbacks`** — `NeedFallbacks` calls `MultiSelect` once and excludes
   the primary model from the result.
8. **`TestConfigureLLM_MaskedKeyNeverPrintsSecret`** — every string passed to `Notify` and
   `Confirm` is checked against the raw key: it must appear nowhere, and the masked form
   must appear at least once.
9. **`TestConfigureLLM_OffersEveryDescriptor`** — with no `Providers` filter, the provider
   `Select` receives exactly `len(Descriptors())` choices.

### Step 5.4 — Verification

```bash
cd mcplib && gofmt -l ./wizard && make vet && make lint && make test
go test ./wizard/ -v          # no TTY, no network
```

**Acceptance:** nine tests pass with no TTY and no network. **Commit.**

---

## Phase 6 — Migrate `prepare-commit-msg`

Chosen first: it has no `pterm`, so it exercises `TextPrompter` directly and needs no
`Prompter` implementation of its own.

### Step 6.0 — Local dependency (all consumer phases)

```
// go.mod
require github.com/maccavelli/mcplib v1.0.1
replace github.com/maccavelli/mcplib => ../mcplib   // TODO: remove once a release carrying wizard/ is tagged
```

The `replace` **stays** at the end of this plan. Removing it requires a tagged, pushed
`mcplib` release — a maintainer action outside this plan.

### Step 6.1 — `internal/ui/setup.go`

Delete `promptProvider` (`:71`), `modelAnnotations` (`:173-194`), `discoverModels` (`:210`),
`promptModel` (`:227`), `promptFallbacks` (`:284`), and the hidden-read body of
`resolveAPIKey` (`:111-170`). Replace with one `wizard.ConfigureLLM` call using
`wizard.NewTextPrompter()`, mapping `SetupOptions` onto `wizard.Options` and the returned
`Result` onto `config.Config` exactly as today.

**Grok and the four gateways and Ollama appear immediately**, with no per-provider code:
the menu comes from `Descriptors()`.

`--yes` non-interactive mode (`runSetupNonInteractive`) does **not** route through
`ConfigureLLM` — it takes flags, not prompts. It is left untouched beyond using
`DescriptorFor` to validate `--provider`, which now accepts all nine.

### Step 6.2 — Tests

- **`TestSetup_OffersEveryDescriptor`** — the drift guard, in **this** repo: the provider
  menu must offer exactly `llmprovider.Descriptors()`. Fails the build when `mcplib` adds a
  provider this wizard cannot reach — the Grok class of bug.
- Existing `setup_test.go` cases for non-interactive mode must still pass unmodified.

### Step 6.3 — Verification

```bash
cd prepare-commit-msg && gofmt -l . && go vet ./... && go test ./...
grep -rn 'modelAnnotations\|func promptModel\|func promptProvider' . ; echo "^ must be empty"
```

**Acceptance:** tests green; the deleted symbols are gone; the wizard offers nine
providers. **Commit in `prepare-commit-msg`.**

---

## Phase 7 — Migrate `mcp-server-magicdev`

### Step 7.1 — `cmd/mcp-server-magicdev/pterm_prompter.go` (new)

~75 lines implementing the six `Prompter` methods over `pterm`, preserving current styling:
`DefaultInteractiveSelect`, `DefaultInteractiveMultiselect`, `DefaultInteractiveConfirm`,
`DefaultInteractiveTextInput`, and — for `Secret` — **delegation to
`wizard.NewTextPrompter().Secret`**, because `pterm`'s `WithMask("*")` masks uniformly and
cannot reveal a tail. A compile-time `var _ wizard.Prompter = (*ptermPrompter)(nil)`.

### Step 7.2 — `configure.go`

Delete `providerEnvVars` (`:27`), `maskKey` (`:107`), ~~`readHiddenSecret` (`:182`)~~, and the
prompting body of `resolveAPIKey` (`:118`). Call `wizard.ConfigureLLM` with the pterm
prompter. Stored-key display becomes `logging.MaskSecret`.

Non-LLM steps — IDE setup, tokens, Jira — are untouched.

> **Amended 2026-08-29 (deviation D3).** `readHiddenSecret` is **not** deleted: five non-LLM
> callers depend on it (`setupJira:228`, `setupConfluence:283`, `setupGitlab:321`,
> `setupGithub:339`, `token.go:104`), so removing the symbol contradicts the "non-LLM steps
> are untouched" sentence in this same step. Its **body** is deleted instead — it becomes a
> thin delegate to `wizard.NewTextPrompter().Secret`. The duplicated raw-terminal handling
> is gone, no non-LLM caller changes, and those five prompts gain masked paste feedback.

### Step 7.3 — Tests and verification

`TestConfigure_OffersEveryDescriptor` as in Phase 6, plus the compile-time assertion.

```bash
cd mcp-server-magicdev && gofmt -l . && go vet ./... && go test ./...
grep -rn 'func maskKey\|providerEnvVars' . ; echo "^ must be empty"
```

> **Amended 2026-08-29 (deviation D5).** The phase also unblocks the runtime, which
> otherwise rejects six of the nine providers the migrated wizard offers.
> `internal/integration/llm_client.go` loses its three-provider switch in favour of
> `llmprovider.DescriptorFor` validation, and gains `llm.base_url` — added to
> `internal/config/registry.go` and to the valid-keys list in `internal/handler/tools.go` —
> threaded through `llmprovider.WithBaseURL`. Without this the wizard writes configurations
> the server cannot start on.

**Acceptance:** `maskKey` and the local env-var map are gone; `NewLLMClient` constructs
every descriptor id. **Commit in `magicdev`.**

---

## Phase 8 — Migrate `mcp-server-magictools`

Last: it carries the tier concept and the largest wizard.

### Step 8.1 — `internal/provider/catalog.go` — wrap, do not replace

Per MADR decision 1, `ProviderSpec` **keeps** what `mcplib` does not model and **derives**
the rest:

| Field | After |
|---|---|
| `Fast`, `Thinking`, `Embedding` | **kept** — tier concept, magictools-only |
| `StaticModels map[Tier][]string` | **kept for `TierEmbedding` only**; fast/thinking tiers come from `Descriptors()` |
| `Dimensions map[string]int` | **kept** — embedding-only |
| `ID`, `Label`, `EnvVar`, `IsLocal`, `SupportsBaseURL` | **derived** from `DescriptorFor(id)` |

This deletes the duplicated identity data **and** `gemini-2.0-flash`, since the fast-tier
list now comes from a catalog that knows it is shut down.

`ProviderVoyage` stays entirely local: it is an embedding provider `llmprovider` does not
model (§13).

### Step 8.2 — `cmd/mcp-server-magictools/pterm_prompter.go` (new)

As Phase 7. The two implementations are near-identical but live in their own repos; sharing
them would mean `mcplib` importing `pterm`, which is the thing this MADR exists to avoid.

### Step 8.3 — `config.go`

Delete `readHiddenSecret` (`:565`), `maskKey` (`:576`), `resolveAPIKey` (`:621`),
`promptOllamaURL` (`:676`), `selectModel` (`:696`), and `providerEnvVars` (`:31`), which
`resolveAPIKey` was its only reader of.

> **Amended 2026-08-29 (deviation D4).** `maskKey`'s three live callers are in the
> configuration **summary display** (`:525`, `:537`, `:548`), not the wizard; they are
> repointed at `logging.MaskSecret`. `config_extra_test.go:13-18` pins `maskKey`'s output
> and is added to this phase's scope: `maskKey("short")` revealed four of five characters,
> `logging.MaskSecret("short")` reveals none, so the assertions change with the behaviour. `selectProviderForTier` (`:601`) becomes
a thin filter: take `Descriptors()`, keep those whose `ProviderSpec` supports the tier, hand
them to `ConfigureLLM` via `Options.Providers`. `promptOllamaURL` disappears because
`ConfigureLLM` handles `SupportsBaseURL` and `IsLocal` generically.

`ConfigureLLM` is called **once per tier**; `mcplib` never learns what a tier is.

### Step 8.4 — Tests and verification

- **`TestConfigure_OffersEveryDescriptorPerTier`** — each tier's menu is a subset of
  `Descriptors()`, and the union across tiers covers every descriptor the specs mark
  supported.
- **`TestCatalog_NoShutDownModels`** — no tier list contains `gemini-2.0-` or `gemini-1.5-`.
  The direct regression for the bug this MADR was written around.

```bash
cd mcp-server-magictools && gofmt -l . && go vet ./... && go test ./...
grep -rn 'func maskKey\|func promptOllamaURL\|gemini-2.0-flash' . ; echo "^ must be empty"
```

> **Amended 2026-08-29 (deviation D6).** The phase also threads base URLs to the runtime.
> `IntelligenceEngine` gains `ThinkingAPIURL` (with YAML and patch plumbing), and
> `internal/llm/pool.go` passes `llmprovider.WithBaseURL` for both tiers when set. `APIURL`
> was previously written and never read; Step 8.1 makes that path reachable, so it is
> repaired here rather than left dormant.

**Acceptance:** all three greps empty; both tiers honour a configured endpoint.
**Commit in `magictools`.**

---

## 9. Verification Commands

```bash
# mcplib (phases 1-5)
cd mcplib
gofmt -l ./logging ./llmprovider ./wizard
make vet && make lint && make test
go test -race ./wizard/ ./llmprovider/
go mod tidy && git diff go.mod        # exactly one new dependency: golang.org/x/term
grep -rn 'pterm\|bubbletea' go.mod    # must be empty

# every descriptor is constructible
go test ./llmprovider/ -run TestDescriptors_EveryDescriptorIsConstructible -v

# consumers (phases 6-8), each in its own repo
gofmt -l . && go vet ./... && go test ./...

# the drift guards
grep -rn 'func maskKey' ~/gitrepos/go/mcp-server-magic{tools,dev}     # must be empty
grep -rn 'modelAnnotations' ~/gitrepos/go/prepare-commit-msg          # must be empty
grep -rn 'gemini-2.0-flash' ~/gitrepos/go/mcp-server-magictools       # must be empty
```

---

## 10. Acceptance Criteria

1. `gofmt -l` prints nothing in all four repos.
2. `vet`, `lint`, `test` exit 0 in all four repos.
3. `go test -race` passes for `mcplib/wizard` and `mcplib/llmprovider`.
4. **`mcplib` gains exactly one direct dependency**, `golang.org/x/term`, at the version all
   three consumers already pin. `pterm` and `bubbletea` appear in neither `mcplib`'s direct
   nor indirect requirements.
5. `Descriptors()` returns **9** entries and covers every key of `ProviderEnvVars`.
6. **`TestDescriptors_EveryDescriptorIsConstructible` passes** — no descriptor exists that
   `NewProvider` cannot build.
7. `OllamaProvider` sends **no** `Authorization` header, never sends `tool_choice`, and
   clamps `xhigh` → `max`.
8. `MaskSecret` is rune-safe on multi-byte input and reveals nothing below 8 runes.
9. `TextPrompter.Secret` reveals the last four runes **live**, and falls back to visible
   entry when raw mode is unavailable — the Git Bash guarantee.
10. `ConfigureLLM` is fully tested against a scripted `Prompter` with **no TTY and no
    network**, including that the raw key never reaches `Notify` or `Confirm`.
11. Each of the three wizards has an `OffersEveryDescriptor` test, so a provider added to
    `mcplib` and not surfaced **fails that repo's build**.
12. `func maskKey` exists in **neither** consumer; `modelAnnotations` and
    `providerEnvVars` are gone.
13. `gemini-2.0-flash` appears in no wizard, no catalog, and no test fixture.
14. The nine `mcplib` interface groups include `*OllamaProvider`; `Continuer` does not.
15. Each phase committed separately in the repo it touches; **no `git push`**.
16. The three consumer `replace` directives are present and carry the removal TODO.

---

## 11. Decisions Resolved by This Plan

| MADR decision | Resolved in |
|---|---|
| 1 — wrap `magictools`'s `ProviderSpec` | [Step 8.1](#step-81--internalprovidercataloggo--wrap-do-not-replace) |
| 2 — Ollama gets a real `Provider` | [Phase 3](#phase-3--ollamaprovider) |
| 3 — key tail revealed **live** | [Step 4.2](#step-42--wizardtext_prompterrgo-new), tests 1–2 in [4.3](#step-43--wizardtext_prompter_testgo-new) |
| 4 — `MultiSelect`, six methods | [§4.3](#43-mcplibwizard-phases-45), Step 4.3 test 6 |
| 5 — Ollama OpenAI-compatible, `/api/tags` listing | [§3.2](#32-ollama-facts-verified-live-2026-08-29-against-a-running-v0311), [Step 3.2](#step-32--llmproviderollamago-new) |
| 6 — 0003 before 0004 | Prerequisite; 0003 merged at `50ac165` |

Additional decisions this plan makes:

- **`ProviderEnvVars` gains no Ollama entry.** An empty-string value would make the
  descriptor-coverage test vacuous; `RequiresAPIKey: false` carries the meaning instead.
- **`StaticModels(ProviderOllama)` returns `nil`** — installed models are machine-specific,
  so `ConfigureLLM` must tolerate an empty catalog (Step 5.3 test 6).
- **`ConfigureLLM` returns a `Result`, never writes config** — persistence differs per app
  and unifying it is a separate decision.
- **`renderSecret` is split from terminal I/O** so live masking is testable without a TTY.
- **The two `pterm` prompters are duplicated across repos**, deliberately: sharing them
  would put `pterm` in `mcplib`.
- **`--yes` non-interactive mode bypasses `ConfigureLLM`** — it takes flags, not prompts.

---

## 12. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Raw-mode `Secret` breaks configuration on Windows terminals | **High** — it is why `prepare-commit-msg` has the fallback today | Users cannot configure at all | The fallback is a required baseline, not an afterthought; Step 4.3 test 4 asserts it with a non-TTY reader |
| Live tail reveal exposes 4 chars in screen shares | Certain, by design | Minor credential exposure | Accepted in MADR revision 2 with the trade-off stated; mitigated by the 8-rune threshold; reversible in one line |
| Consumers cannot build until `mcplib` is released | Certain | Phases 6–8 unmergeable as-is | `replace` directive with a TODO; the release is called out as a maintainer action, not silently assumed |
| `Prompter` proves wrong after consumers adopt it | Low | Breaking change to 12 repos | Six methods, all mirroring what the three wizards already do; `TextPrompter` exercises every one from day one |
| `magictools` tier wrap regresses embedding config | Medium | Embedding setup breaks | `ProviderSpec` keeps `TierEmbedding` models and `Dimensions` verbatim; only fast/thinking derive from descriptors |
| A wizard silently stops offering a new provider | **High** without a guard — it already happened with Grok | The bug this MADR exists to fix | `OffersEveryDescriptor` test in each of the three repos, so it fails the build rather than going unnoticed |
| Ollama's `/v1` shim changes shape | Low | Generation breaks | `wireShapesProbedOnOllama` records the probe date; listing stays on the stable native `/api/tags` |

---

## 13. Out of Scope

- **Config persistence.** `magictools`'s `internal/config` alone is 1,626 LOC with a watcher
  and patch system. `ConfigureLLM` returns a `Result`; each app persists as it does today.
- **Provider tiers** (fast/thinking/embedding) — a `magictools` concept; `mcplib` does not
  model them.
- **Embedding providers** (`voyage`) and embedding-model selection. `llmprovider` has no
  embedding abstraction; adding one is its own MADR. This holds even though Ollama exposes
  `/v1/embeddings` — the gap is the missing abstraction, not the endpoint.
- **Migrating `listOllamaModels` to `/v1/models`** — declined; §3.2.
- **Non-LLM wizard steps** — magicdev's IDE/token/Jira setup, magictools' backplane menu.
- **`bubbletea`**, and any opinion about it.
- **Tagging or pushing an `mcplib` release**, and removing the consumer `replace` directives.
- The nine consumers with no wizard — untouched.

---

## 14. File Summary

### New files (14)

| File | Phase | Purpose |
|---|---|---|
| `mcplib/logging/mask.go` | 1 | `MaskSecret` |
| `mcplib/logging/mask_test.go` | 1 | Six tests incl. the rune-safety regression |
| `mcplib/llmprovider/descriptor.go` | 2, 3 | `ProviderDescriptor`, `Descriptors()`, `DescriptorFor` |
| `mcplib/llmprovider/descriptor_test.go` | 2, 3 | Coverage, derivation, constructibility |
| `mcplib/llmprovider/ollama.go` | 3 | `OllamaProvider` on the shared chat primitive |
| `mcplib/llmprovider/ollama_test.go` | 3 | Seven Ollama tests |
| `mcplib/wizard/prompter.go` | 4 | Six-method interface, `Choice`, `Level` |
| `mcplib/wizard/text_prompter.go` | 4 | Zero-toolkit default incl. live-masked `Secret` |
| `mcplib/wizard/text_prompter_test.go` | 4 | Eight tests, no TTY |
| `mcplib/wizard/configure.go` | 5 | `ConfigureLLM` |
| `mcplib/wizard/fake_prompter_test.go` | 5 | Scripted `Prompter` |
| `mcplib/wizard/configure_test.go` | 5 | Nine flow tests |
| `mcp-server-magicdev/cmd/.../pterm_prompter.go` | 7 | `Prompter` over pterm |
| `mcp-server-magictools/cmd/.../pterm_prompter.go` | 8 | `Prompter` over pterm |

### Modified files (19)

| File | Phase | Change |
|---|---|---|
| `mcplib/llmprovider/constants.go` | 3 | +`ProviderOllama` |
| `mcplib/llmprovider/provider.go` | 3 | +`NewProvider` case (no `ProviderEnvVars` entry) |
| `mcplib/llmprovider/discovery.go` | 3 | `case "ollama"` → `case ProviderOllama` |
| `mcplib/llmprovider/models_catalog.go` | 2, 3 | +`ModelLabel`, +`StaticModels` Ollama case |
| `mcplib/llmprovider/models_catalog_test.go` | 2 | +`ModelLabel` tests |
| `mcplib/llmprovider/interface_test.go` | 3 | +`*OllamaProvider` in nine groups |
| `mcplib/llmprovider/provider_test.go` | 3 | +`ProviderOllama` in the provider slice |
| `mcplib/go.mod` | 4 | +`golang.org/x/term` (third direct dependency) |
| `prepare-commit-msg/internal/ui/setup.go` | 6 | −5 functions, −17-entry catalog, +`ConfigureLLM` |
| `mcp-server-magicdev/cmd/.../configure.go` | 7 | −`maskKey`, −`providerEnvVars`; `readHiddenSecret` delegates to `wizard` (D3) |
| `mcp-server-magictools/cmd/.../config.go` | 8 | −5 functions, `selectProviderForTier` becomes a filter |
| `mcp-server-magictools/internal/provider/catalog.go` | 8 | `ProviderSpec` wraps descriptors; tier/embedding data kept |
| `mcp-server-magictools/cmd/.../config_extra_test.go` | 8 | `maskKey` assertions become `logging.MaskSecret` (D4) |
| `mcp-server-magicdev/internal/integration/llm_client.go` | 7 | −three-provider switch; +`WithBaseURL` (D5) |
| `mcp-server-magicdev/internal/config/registry.go` | 7 | +`llm.base_url` (D5) |
| `mcp-server-magicdev/internal/handler/tools.go` | 7 | +`llm.base_url` in the valid-keys list (D5) |
| `mcp-server-magictools/internal/config/config.go` | 8 | +`ThinkingAPIURL` (D6) |
| `mcp-server-magictools/internal/config/patch_yaml.go` | 8 | +`thinking_api_url` patching (D6) |
| `mcp-server-magictools/internal/llm/pool.go` | 8 | +`WithBaseURL` on both tiers (D6) |

Plus three consumer `go.mod` files gaining a `replace` directive (Phases 6–8).

---

## 15. Deviation Log

Empty at time of writing. Per the repository workflow, any deviation discovered during
execution is recorded here — dated, naming what was found, the resolution chosen, and any
files added to a phase's scope — **before** the fix is executed. The original step is struck
through or annotated rather than rewritten.

Plan 0003 recorded four deviations (D1–D4) against a plan of comparable size; expect
similar here, particularly around raw-mode terminal handling in Phase 4.

| Date | Phase/Step | Finding | Resolution | Files added to scope |
|---|---|---|---|---|
| 2026-08-29 | **D1** — Phase 4, Step 4.2 | `TextPrompter.In` was typed `*os.File`. Phase 6 could not use it: `prepare-commit-msg` threads an `io.Reader` through `RunSetupWithOptions` so its tests can inject scripted input, and its existing code already handles the "not an `*os.File`" case. The signature was wrong for its own consumers. | `In` is now `io.Reader`. Raw-mode masking is used only when the input is genuinely a terminal (`inTTY`), degrading to a plain read otherwise — the behaviour non-TTY callers already received. All Phase 4 tests pass unchanged. | none |
| 2026-08-29 | **D2** — Phase 5, Step 5.2 | `ConfigureLLM` had two capability gaps against the wizard it was replacing, both caught by `prepare-commit-msg`'s existing tests during Phase 6. Its model menu had no manual-entry escape hatch, though the `promptModel` it replaced offered one; and its environment reader was hard-wired to `os.Getenv`, so a consumer that indirects `os.Getenv` for its own tests — as `prepare-commit-msg` does via `osGetenv` — could not drive the env-key branch. | The model menu gained a trailing `"Other (enter a model id)"` option, and `Options.LookupEnv` makes the environment reader injectable. Executed in `5510341`; this row backfills the record, which that commit references but never wrote. | none |
| 2026-08-29 | **D3** — Phase 7, Step 7.2 | Step 7.2 is self-contradictory as written: it orders the deletion of `readHiddenSecret` (`configure.go:182`) while also stating that the non-LLM steps are untouched. `readHiddenSecret` has five non-LLM callers — `setupJira:228`, `setupConfluence:283`, `setupGitlab:321`, `setupGithub:339` and `token.go:104` — so deleting the symbol breaks the build. Confirmed pre-existing: `configure.go` was unmodified in the working tree when this was found. | **Delete the implementation, keep the symbol.** `readHiddenSecret` becomes a thin delegate to `wizard.NewTextPrompter().Secret`. The duplicated raw-terminal handling this MADR exists to remove is gone, the five non-LLM callers compile unchanged, and those five credential prompts gain the masked-tail paste feedback. | none |
| 2026-08-29 | **D4** — Phase 8, Step 8.3 | Step 8.3 lists `maskKey` (`config.go:576`) for deletion but does not account for its callers or its test. Its three live call sites are in the **configuration summary display** (`:525`, `:537`, `:548`), not the wizard, and `config_extra_test.go:13-18` pins its exact output — a file absent from the plan's file list. The formats differ substantively, not cosmetically: `maskKey("short")` returns `"****hort"`, revealing four of five characters, where `logging.MaskSecret("short")` returns `"••••••••"` and reveals none. | Delete `maskKey`, repoint the three display sites at `logging.MaskSecret`, and rewrite the two `TestConfigHelpers` assertions to the new format. One masking implementation per binary, and short values stop leaking most of themselves. | `mcp-server-magictools/cmd/.../config_extra_test.go` |
| 2026-08-29 | **D5** — Phase 7, Step 7.2 | Phase 7 migrates the wizard but not the runtime that consumes its output. `internal/integration/llm_client.go:47-56` switches on the configured id and returns `unsupported llm provider: %s` for anything outside `gemini|openai|claude`, so six of the nine providers the migrated wizard offers would write a configuration the server rejects at start. There is also no `llm.base_url` key in `internal/config/registry.go`, so the `Result.BaseURL` that `ConfigureLLM` collects for Ollama and all four gateways has nowhere to be stored. Confirmed pre-existing: both files were unmodified when this was found. | Delete the switch. `llmprovider.NewProvider` is already the authority on valid ids, and `DescriptorFor` gives a clear up-front error, so a tenth provider added to `mcplib` works here with no edit — the drift this MADR exists to remove. Add `llm.base_url` to the registry and thread it through `llmprovider.WithBaseURL`. | `mcp-server-magicdev/internal/integration/llm_client.go`, `internal/config/registry.go`, `internal/handler/tools.go` |
| 2026-08-29 | **D6** — Phase 8, Step 8.3 | `internal/llm/pool.go:105` and `:113` construct both tiers with `WithHTTPClient` alone and never pass `WithBaseURL`. `Intelligence.APIURL` is written by the wizard (`config.go:141-143`) and persisted (`config.go:744`) but never read — a dead write that is currently harmless only because Ollama's spec sets `Fast: false`, making the fast-tier `promptOllamaURL` branch unreachable. Step 8.1 makes it reachable. `IntelligenceEngine` also has no `ThinkingAPIURL` field at all, so the thinking tier cannot store an endpoint. Confirmed pre-existing: `pool.go` and `config.go` were unmodified when this was found. | Add `ThinkingAPIURL` with its YAML and patch plumbing, and pass `llmprovider.WithBaseURL` for both tiers when set. This repairs the dead `APIURL` write rather than leaving it dormant behind a newly-reachable path. | `mcp-server-magictools/internal/config/config.go`, `internal/config/patch_yaml.go`, the tier patch structs, `internal/llm/pool.go` |
