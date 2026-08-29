---
status: proposed
date: 2026-08-29
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev, prepare-commit-msg
informed: all mcplib consumers
---

# Canonicalize LLM Provider Configuration — Descriptors, Flow and Prompting — in `mcplib`, Renderer-Agnostic

> **Revision notes (revision 2, 2026-08-29, applied in place to this `proposed` document —
> same convention as `0001-MADR` and `0003-MADR`).** Revision 1 shipped with five open
> questions. All are now answered by the maintainer, plus a sixth raised by the Ollama
> answer. Two answers went **against** this document's recommendation and are recorded as
> such below, with the trade-off each accepts stated rather than quietly dropped. One answer
> **grows scope**: `llmprovider` gains an Ollama `Provider` implementation. See
> "Decisions Resolved".

## Context and Problem Statement

`mcplib` is a shared library consumed by **12 repositories** (`grep -l maccavelli/mcplib */go.mod`):
`mcp-server-brainstorm`, `-duckduckgo`, `-evolve-plan`, `-filesystem`, `-go-modernizer`,
`-magicdev`, `-magicskills`, `-magictools`, `-recall`, `-sequential-thinking`,
`-socratic-thinker`, and `prepare-commit-msg`.

Three of those twelve ship an **interactive LLM configuration wizard**:

| Repo | Wizard entry point | Wizard LOC |
|---|---|---|
| `mcp-server-magictools` | `cmd/mcp-server-magictools/config.go` | 806 |
| `mcp-server-magicdev` | `cmd/mcp-server-magicdev/configure.go` | 610 |
| `prepare-commit-msg` | `internal/ui/setup.go` | 548 |

All three configure the *same* thing — an `llmprovider` provider, an API key, and a model —
against the *same* library. `mcplib/llmprovider` already owns the canonical provider
identifiers (`constants.go:3-9`), the environment-variable map (`provider.go:106-111`), the
curated model catalogs (`models_catalog.go:25-66`), live model listing
(`discovery.go:18-38`), and health probing (`probe.go:13-59`).

Yet each wizard re-implements provider selection, key entry, model selection and validation
independently — and **they have measurably drifted apart**.

### Measured drift (verified 2026-08-29)

**1. Provider coverage differs four ways, and every wizard is behind the library.**

| Source | Providers offered |
|---|---|
| `mcplib/llmprovider` (canonical) | `gemini`, `openai`, `claude`, **`grok`** — plus `ollama` listing-only |
| `prepare-commit-msg` `promptProvider` (`setup.go:71-102`) | `gemini`, `openai`, `claude` — **no Grok** |
| `mcp-server-magicdev` `providerEnvVars` (`configure.go:27-31`) | `gemini`, `openai`, `claude` — **no Grok** |
| `mcp-server-magictools` `internal/provider/catalog.go` | `gemini`, `claude`, `openai`, `voyage`, `ollama` — a **different** set, with its own `ProviderSpec` type |

Grok was added to `mcplib` by MADR 0001 and shipped. **Zero of the three wizards offer
it.** A capability the library has had through a full release is unreachable from every
user-facing configuration path.

**2. Three independent sources of truth for the environment-variable map.**
`mcplib` exports `ProviderEnvVars` (`provider.go:106-111`). `prepare-commit-msg` uses it
correctly (`setup.go:105-110`). `mcp-server-magicdev` declares a **local copy**
(`configure.go:27-31`) that omits Grok. `mcp-server-magictools` embeds the value a third
time as `ProviderSpec.EnvVar` per provider.

**3. Model catalogs are duplicated and stale — one wizard offers a shut-down model.**

* `mcp-server-magictools` `catalog.go` offers `gemini-2.0-flash` in its fast tier.
  `mcplib/models_catalog.go:26` states in terms that `gemini-2.0-*` and `1.5-*` **are shut
  down**, and `RankGeminiModel` (`models_catalog.go:257`) scores them **−2000** (`models_catalog.go:289`). The
  wizard actively recommends a model the library classifies as dead.
* The same file's Claude fast tier is `claude-3-5-sonnet-latest`, `claude-3-5-haiku-latest`,
  `claude-3-opus-latest`; `mcplib`'s `StaticClaude` is `claude-haiku-4-5`,
  `claude-sonnet-5`, `claude-sonnet-4-6`, `claude-opus-4-8`, … — generations apart.
* `prepare-commit-msg` carries `modelAnnotations` (`setup.go:173-194`): **17 entries** that
  mirror `mcplib`'s 18 curated static models, adding display labels and `★ Recommended`
  markers. It has no Grok entries because it has no Grok.

**4. API-key entry behaves differently in all three, and none confirms a paste.**

| Repo | While entering | Stored-key display | Local `maskKey` |
|---|---|---|---|
| `magictools` | `*` per char (`pterm…WithMask("*")`, `config.go:568`) | `****a75y` | yes — `config.go:576-584` |
| `magicdev` | **nothing** (`term.ReadPassword`, `configure.go:185`) | `****a75y` | yes — `configure.go:107-115` (byte-identical to magictools') |
| `prepare-commit-msg` | **nothing** (`term.ReadPassword`, `setup.go:150`) | **nothing** — `"Existing key found in config."` | **none** |

The two `maskKey` implementations are byte-identical duplicates, and both slice **bytes**
(`key[len(key)-4:]`), so a non-ASCII paste splits a rune and prints mojibake.
`prepare-commit-msg` conversely has the **most robust** entry path — a documented fallback
for Git Bash / mintty where hidden reads fail (`setup.go:158-163`) and an empty-entry retry
— which the other two lack.

**5. The cost is about to triple.** MADR 0003 (`proposed`) adds four gateway providers:
`opencode-zen`, `opencode-go`, `huggingface`, `kilo`. Under the status quo that is
4 providers × 3 wizards = **12 independent edits**, in three repos, with three different UI
stacks, to expose one library change. The Grok precedent says they will not all happen.

### Existing architecture that constrains the solution

* **`mcplib` has exactly two direct dependencies** (`go.mod:5-8`): `invopop/jsonschema` and
  `modelcontextprotocol/go-sdk`. It has **zero UI dependencies** and does no terminal I/O:
  `input.go` is MCP tool-input schema helpers, `prompt.go` is MCP prompt-handler panic
  hardening, and the only `bufio`/stdin use is `stdio.go`, the JSON-RPC transport.
* **The consumers' UI stacks split 2:1.** `magictools` and `magicdev` both depend on
  `pterm v0.12.83` **and** `charmbracelet/bubbletea v1.3.10`. `prepare-commit-msg` depends
  on **neither** — only `golang.org/x/term v0.43.0` plus stdlib `fmt`.
* **Nine of the twelve consumers have no wizard at all** and would inherit any dependency
  `mcplib` adds.
* `mcplib/logging/redact.go:14-16` already declares itself "the single source of truth for
  secret redaction in mcplib", establishing the precedent that cross-cutting
  secret-handling belongs in this library.

This is the central design constraint: **canonicalizing "wizards and menus" cannot mean
moving `pterm` rendering into `mcplib`**, because that pushes an interactive-terminal
dependency onto nine headless MCP servers that will never draw a menu.

## Decision Drivers

* **One source of truth per fact.** Provider identity, env var, model catalog, display
  label and capability flags must exist once. The Grok gap and the `gemini-2.0-flash`
  recommendation are both direct consequences of having three.
* **Adding a provider to `mcplib` must make it appear in every wizard**, with no downstream
  edit. This is the acceptance test for the whole change.
* **`mcplib` must stay dependency-light.** Twelve binaries consume it; nine have no
  terminal UI. `pterm` + `bubbletea` is a large transitive surface to impose on them.
* **Consumers must keep their look and feel.** `magictools`/`magicdev` have invested in
  `pterm` styling; `prepare-commit-msg` is deliberately plain and scriptable. Canonicalizing
  must not force either to adopt the other's aesthetic.
* **Do not regress the best existing behaviour.** `prepare-commit-msg`'s Git Bash/mintty
  fallback is a real-world fix; any shared entry path must retain it or Windows users lose
  the ability to configure at all.
* **Testability without a TTY.** All three wizards are partly untested because they read
  real terminals. A canonical implementation should be driveable from a test.
* **Scope discipline.** `magictools` models concepts `mcplib` does not — provider *tiers*
  (fast/thinking/embedding) and *embedding* providers (`voyage`). Whether those move is a
  decision, not an assumption.

## Considered Options

1. **Renderer-agnostic canonicalization: `mcplib` owns descriptors, flow and validation
   behind a small `Prompter` interface, plus a zero-dependency default renderer** (chosen)
2. Full canonicalization including `pterm` rendering in `mcplib`
3. Data-only canonicalization — share descriptors and catalogs, leave the three wizards
4. A separate `mcplib/wizard` Go module with its own `go.mod`
5. Status quo — fix the drift in place, three times

## Decision Outcome

Chosen option: **1 — renderer-agnostic canonicalization**, because it is the only option
that makes "add a provider to `mcplib`, get it in every wizard" true while leaving
`mcplib`'s dependency set unchanged for the nine consumers that never prompt.

The change splits the wizard into three layers and assigns each an owner:

| Layer | Owner | Rationale |
|---|---|---|
| **Descriptors** — identity, env var, base URL, capabilities, display label, catalog | **`mcplib`** | Already 80% there; the drift is entirely here |
| **Flow** — select provider → resolve key → discover/select model → validate → return | **`mcplib`** | Identical intent in all three wizards today, three implementations |
| **Rendering** — how a menu, a confirm, a secret field look | **consumer**, via interface | The 2:1 `pterm`/plain split is a legitimate product difference |

### 1. `ProviderDescriptor` — one record per provider

A new `llmprovider/descriptor.go` promotes the scattered facts into one exported record,
derived from data the package already holds:

```go
// ProviderDescriptor is the single source of truth for everything a
// configuration UI needs to know about a provider.
type ProviderDescriptor struct {
    ID              string   // ProviderGemini, ProviderOpencodeZen, …
    Label           string   // "Gemini (Google)" — menu display
    EnvVar          string   // from ProviderEnvVars
    DefaultBaseURL  string   // "" when the provider has no override
    SupportsBaseURL bool     // Ollama, the OpenCode gateways, self-hosted
    IsLocal         bool     // Ollama — no API key required
    RequiresAPIKey  bool
    StaticModels    []string // from StaticModels(ID)
    Notes           string   // "free tier available", "subscription", …
}

func Descriptors() []ProviderDescriptor           // stable, menu-ordered
func DescriptorFor(id string) (ProviderDescriptor, bool)
```

`ProviderEnvVars`, `StaticModels` and the `Provider*` constants remain exported and
unchanged — descriptors are **derived from** them, so nothing existing breaks. Adding a
provider means adding one descriptor entry beside the constant, and every wizard updates.

**Model display labels move here too.** `prepare-commit-msg`'s 17-entry `modelAnnotations`
becomes `ModelLabel(provider, model) string` in `models_catalog.go`, beside the catalogs it
annotates, so a catalog edit and its label edit are the same diff.

### 2. `Prompter` — the renderer seam

A new package `mcplib/wizard` defines what a UI must provide. It is deliberately tiny and
UI-toolkit-free:

```go
type Choice struct{ Label, Detail string }

type Prompter interface {
    Select(title string, choices []Choice, defaultIdx int) (int, error)
    MultiSelect(title string, choices []Choice, preselected []int) ([]int, error)
    Confirm(question string, def bool) (bool, error)
    Input(prompt, def string) (string, error)
    Secret(prompt string) (string, error)   // masked entry
    Notify(level Level, format string, args ...any)
}
```

`magictools` and `magicdev` each implement it over `pterm` (~75 lines, keeping their exact
current styling). `prepare-commit-msg` uses the default.

`MultiSelect` exists for the fallback-model step (`prepare-commit-msg`'s `promptFallbacks`,
`setup.go:284`). It makes the interface six methods rather than five — a permanent
commitment in a library 12 repos consume, accepted for the better fallback UX. `pterm`
provides `DefaultInteractiveMultiselect`, so the two `pterm` consumers get it nearly free;
`TextPrompter` implements it as a numbered list accepting comma-separated indices.

### 3. `wizard.TextPrompter` — the zero-dependency default

`mcplib` ships one implementation built on stdlib plus `golang.org/x/term`, which
`prepare-commit-msg` already depends on. **`x/term` becomes `mcplib`'s third direct
dependency** — a single, small, `golang.org/x`-maintained package, versus `pterm` +
`bubbletea` + their transitive trees under option 2.

`TextPrompter.Secret` is where the masked-entry request lands:

* **the last 4 runes are revealed live, as you type or paste** — the field renders
  `••••••••a75y` and updates on every keystroke, so a paste is confirmed the instant it
  lands and the key is identifiable without pressing Enter;
* everything before the final 4 runes renders as `•`, so length is visible but the body is
  not;
* below a minimum length (fewer than 8 runes entered) **nothing** is revealed — otherwise a
  short or partial paste would expose most of itself;
* **`prepare-commit-msg`'s fallback chain is the required baseline**: raw-mode masked entry
  → on failure, the documented "hidden input unavailable — typing will be visible" path
  (`setup.go:158-163`), so Git Bash / mintty keeps working;
* empty entry re-prompts rather than accepting `""`.

**Accepted trade-off.** Revision 1 recommended revealing the tail only on submit, because
Stripe, AWS and GitHub all show last-4 on *stored* keys rather than during entry, and a
live tail puts 4 characters on screen for the whole entry — visible in screen shares,
recordings and over a shoulder. The maintainer chose live reveal for the stronger
immediate feedback. That is the decision; the exposure is recorded here so it is a known
cost rather than an oversight, and the minimum-length guard above is the mitigation. Moving
to on-submit later is a one-line change in `TextPrompter.Secret`.

### 4. `MaskSecret` — one masking function

In `mcplib/logging`, beside `Redact`, with the distinction stated in the doc comment:
`Redact` hides a secret completely for logs; `MaskSecret` reveals a suffix **on purpose**
for human identification. Rune-safe (fixing the byte-slicing bug in both existing copies),
with a short-input guard that reveals nothing below a threshold. Both `maskKey` copies are
deleted; `prepare-commit-msg` gains a stored-key display it never had.

### 5. `wizard.ConfigureLLM` — the canonical flow

```go
type Result struct {
    Provider, APIKey, Model, BaseURL string
    Fallbacks                        []string
}

type Options struct {
    Providers     []string      // nil = all descriptors
    Existing      Result         // pre-fill / "keep existing?"
    AllowEnv      bool           // offer a detected env var key
    Discover      bool           // live model listing vs static catalog
    DiscoverLimit time.Duration
    NeedFallbacks bool
}

func ConfigureLLM(ctx context.Context, p Prompter, o Options) (Result, error)
```

The flow is the intersection of what all three wizards already do: offer providers from
`Descriptors()`; resolve the key by precedence **env → existing config → prompt** (the
order `prepare-commit-msg` already implements at `setup.go:111-170`); list models via
`ListAvailableModels` with a timeout, falling back to `StaticModels`; label them with
`ModelLabel`; optionally probe with `probeGenerateHealth`; optionally collect fallbacks.

Because `Prompter` is an interface, the whole flow is testable with a scripted fake and no
TTY — closing the coverage gap all three wizards have today.

### 6. `OllamaProvider` — closing the descriptor's sharp edge

Every descriptor must map to a provider `NewProvider` can construct. Ollama was the one
exception — listing-only since inception (`discovery.go:33-34`), with no `Provider` struct.
Rather than special-case it or drop it from the menus (which would regress `magictools`,
the one app that offers it today), `llmprovider` gains a real `OllamaProvider`.

**Verified live against a running instance, 2026-08-29 (Ollama v0.31.1):**

| Probe | Result |
|---|---|
| `GET /api/version` | `200 {"version":"0.31.1"}` |
| `GET /v1/models` | `200`, OpenAI list shape — `data[].{id,object,created,owned_by}`, identical to the OpenCode/HF/Kilo listings |
| `POST /v1/chat/completions`, **no auth header** | `200`, standard envelope, `choices[0].message.content == "ALPHA"` |
| `message` keys returned | `content`, `role` — no reasoning field on the probed model |

So `OllamaProvider` is **a struct and a constructor, with no new wire logic**: it posts to
`{baseURL}/v1/chat/completions` and reuses MADR 0003's shared `chatCompletionsBody` and
`decodeChatCompletionsResponse`. Three details fall out of the published compatibility
notes (`https://docs.ollama.com/api/openai-compatibility`) and fit seams that already exist:

* **`tool_choice` is not supported.** This maps exactly onto the
  `chatCompletionsOpts.ForceTool` flag MADR 0003 introduced for Kilo — Ollama sets
  `ForceTool: false`, offering `tools` without forcing. No new mechanism.
* **`reasoning_effort` is supported**, with values `none|low|medium|high|max`. Note `max`,
  not the `xhigh` this package models (`constants.go:34-39`); the provider clamps `xhigh` to
  `max`.
* **An API key is "required but ignored."** `NewOllama` therefore does **not** require one,
  and `ProviderDescriptor.RequiresAPIKey` is `false` — the flag earns its place.

**The listing stays on the native `/api/tags`.** `listOllamaModels` (`discovery.go:190-228`)
and `ValidateOllamaURL` (`discovery.go:232-251`) are working, tested code against Ollama's
stable native API; `/v1/models` is a compatibility shim. Migrating them for cosmetic
symmetry would change working code for no functional gain. Ollama is therefore the one
provider whose lister does not look like the others, and that is deliberate.

**This section depends on MADR 0003 landing first** — see the sequencing decision below.

### Consequences

* Good, because adding a provider to `mcplib` makes it appear in every wizard with **no
  downstream edit** — the acceptance test for this MADR. Grok appears in all three
  immediately; MADR 0003's four gateways arrive for free instead of costing 12 edits.
* Good, because the four drift classes collapse to one source each: one provider list, one
  env-var map, one model catalog, one masking function.
* Good, because `magictools`'s `gemini-2.0-flash` recommendation disappears — the catalog
  it draws from is the one that knows the model is shut down.
* Good, because `mcplib` gains **one** small direct dependency (`x/term`), not two large
  ones, and the nine wizard-less consumers inherit nothing they cannot already reach.
* Good, because consumers keep their look and feel: `pterm` styling stays in the repos that
  chose it, behind ~60 lines of interface implementation each.
* Good, because the flow becomes testable without a TTY for the first time.
* Good, because the best existing behaviour is preserved rather than averaged away —
  `prepare-commit-msg`'s Git Bash fallback becomes the shared baseline, and
  `magictools`/`magicdev`'s masking becomes universal.
* Neutral, because three wizards shrink substantially while `mcplib` grows. Net LOC is
  roughly flat; the win is arity, not size.
* **Bad, because it is a coordinated four-repo change** — `mcplib` plus three consumers,
  each needing its own release and version bump. This is the real cost, and it is why the
  phasing below is additive-first: `mcplib` ships the new surface, then consumers migrate
  independently and at their own pace.
* Bad, because `Prompter` is a new public interface in a library used by 12 repos; changing
  it later is a breaking change. Mitigated by keeping it to five methods that mirror what
  all three wizards already do, and by shipping `TextPrompter` so the interface is exercised
  by a real implementation from day one.
* Bad, because `magictools`'s **tier** model (fast / thinking / embedding) and its
  **embedding providers** (`voyage`) have no `mcplib` equivalent. See "Scope boundaries".
* Good, because `OllamaProvider` closes the descriptor model's only sharp edge — every
  descriptor now maps to a constructible provider — and costs no new wire logic, since
  Ollama's OpenAI-compatible endpoint rides MADR 0003's shared primitive.
* Good, because Ollama's unsupported `tool_choice` needed no new mechanism: the
  `ForceTool` seam added for Kilo already covers it. Two independent providers now justify
  that flag.
* Neutral, because `Prompter` is six methods rather than five. `pterm` supplies
  `DefaultInteractiveMultiselect`, so the cost lands mostly on `TextPrompter`.
* **Bad, because the live-revealed key tail is on screen for the whole entry** — visible in
  screen shares, recordings and to anyone nearby, where an on-submit reveal would expose it
  only after the fact. Accepted deliberately for stronger paste feedback; mitigated by
  revealing nothing below 8 runes.
* **Bad, because this MADR now depends on MADR 0003.** `OllamaProvider` reuses
  `chatcompletions.go`, which 0003 introduces. 0004 cannot be executed first without either
  writing a throwaway chat decoder or dropping §6. The sequencing decision below accepts
  this.
* Bad, because a masked raw-mode reader is fiddly — bracketed paste, `\r` vs `\n`,
  backspace, Ctrl-C — and a bug there blocks configuration entirely. Mitigated by the
  mandatory fallback path and by the interface seam that lets it be tested headlessly.

### Scope boundaries

**In scope:** provider descriptors; model catalogs and display labels; the `Prompter`
interface (six methods); `TextPrompter`; `MaskSecret`; `ConfigureLLM` covering provider →
key → model → fallbacks; **a new `OllamaProvider` for text generation**; migration of all
three wizards.

**Out of scope, deliberately:**

* **Config persistence.** All three repos have their own schema, file location and atomic
  write (`magictools` `internal/config` is 1,626 LOC with a watcher and patch system;
  `prepare-commit-msg` has `save_atomic.go`). `ConfigureLLM` **returns a `Result`**; each
  app persists it as it already does. Unifying config storage is a separate, much larger
  decision.
* **Provider tiers** (fast/thinking/embedding). A `magictools` concept only. `ConfigureLLM`
  is called once per tier by that app; `mcplib` does not model tiers.
* **Embedding providers** (`voyage`) and embedding-model selection. `llmprovider` has no
  embedding abstraction; adding one is its own MADR. This holds even though Ollama's
  `/v1/embeddings` exists — the gap is the missing abstraction, not the missing endpoint.
* **Migrating `listOllamaModels` to `/v1/models`.** Considered and declined; see §6.
* **Non-LLM wizard steps** — magicdev's IDE/token/Jira setup, magictools' backplane menu.
* **`bubbletea`.** Neither `mcplib` nor the interface takes any position on it.
* The nine consumers with no wizard — untouched.

### Confirmation

* `grep -rn 'maskKey\|providerEnvVars\s*=' <all three repos>` returns nothing after
  migration; `grep -rn 'modelAnnotations'` returns nothing.
* Every wizard offers all providers in `llmprovider.Descriptors()` — asserted by a test in
  each consumer that compares its menu against the descriptor list, so the Grok class of
  drift **fails the build** rather than going unnoticed.
* `mcplib/go.mod` gains exactly one direct dependency, `golang.org/x/term`; `pterm` and
  `bubbletea` appear in neither `mcplib`'s direct nor indirect requirements.
* `gemini-2.0-flash` appears in no wizard.
* `wizard.ConfigureLLM` has table-driven tests over a scripted fake `Prompter` covering:
  env-key precedence, keep-existing, live-discovery success, discovery timeout → static
  fallback, and empty-key retry — with no TTY.
* `MaskSecret` is rune-safe: a multi-byte input yields valid UTF-8 and never splits a rune.
* `TextPrompter.Secret` falls back to visible entry when raw mode fails, asserted with a
  non-TTY reader.
* `OllamaProvider` implements `Provider`, `ToolProvider`, `ThinkingProvider` and
  `ModelDiscoverer`; `NewProvider(ProviderOllama, "", model)` succeeds **with an empty API
  key**, and a `httptest` test asserts the request carries `tools` but **no** `tool_choice`,
  and clamps `WithReasoningEffort("xhigh")` to `max`.
* `llmprovider.Descriptors()` has no entry that `NewProvider` cannot construct — asserted by
  a test that loops every descriptor.
* `TextPrompter.Secret` reveals the last 4 runes live once 8+ runes are entered, and
  **nothing** below that — asserted against a scripted rune stream, no TTY.
* `Prompter` has exactly six methods, and `TextPrompter` plus both `pterm` implementations
  satisfy it (compile-time assertions in each repo).
* `go test ./...` passes in all four repos; `mcplib` stays green for the nine untouched
  consumers.

## Pros and Cons of the Options

### 1. Renderer-agnostic canonicalization (chosen)

* Good, because it fixes every measured drift class at the source.
* Good, because it costs `mcplib` one small dependency instead of two large ones.
* Good, because consumers keep their UI identity, so migration is not also a redesign.
* Good, because the interface seam makes the flow testable headlessly.
* Bad, because it introduces a public interface that is expensive to change later.
* Bad, because it is a coordinated four-repo change.

### 2. Full canonicalization including `pterm` rendering in `mcplib`

* Good, because consumers would need almost no code — one call and done.
* Good, because the UI would be genuinely pixel-identical everywhere, not merely
  behaviourally identical.
* **Bad, because it forces `pterm` + `bubbletea` onto nine MCP servers that never draw a
  menu**, tripling `mcplib`'s direct dependency count for their benefit of zero.
* Bad, because it would force `prepare-commit-msg` — deliberately plain, a git hook that
  runs non-interactively in CI — to adopt an interactive TUI toolkit.
* Bad, because a library that owns rendering owns every future styling request from three
  apps with different tastes.

### 3. Data-only canonicalization (descriptors and catalogs, wizards stay)

* Good, because it is the smallest change and fixes drift classes 1–3 (providers, env vars,
  catalogs) — most of the measured damage.
* Good, because it needs no new interface and no new dependency.
* Bad, because it leaves three implementations of the same flow, so behaviour keeps
  diverging even when data does not — key-entry drift (class 4) is untouched, and the
  masked-entry request would still be built three times.
* Bad, because "add a provider, every wizard updates" only half-holds: the provider appears
  in the data but each wizard still decides independently whether to offer it.
* **Worth noting:** this is a strict subset of option 1 and a viable first phase if the
  four-repo coordination is unwelcome now.

### 4. Separate `mcplib/wizard` Go module with its own `go.mod`

* Good, because the UI dependency would be genuinely opt-in — the nine wizard-less
  consumers would not see `x/term` at all.
* Bad, because a multi-module repo adds real release friction: two version streams, a
  `replace` directive during development, and every cross-module change becoming a two-step
  release.
* Bad, because the dependency it isolates is one small `golang.org/x` package — the
  ceremony costs more than the thing it avoids.
* Reconsider only if the default prompter later needs a heavy toolkit.

### 5. Status quo — fix the drift three times

* Good, because it needs no coordination and no new API.
* Bad, because it has already been tried implicitly and failed: Grok shipped in MADR 0001
  and reached none of the three wizards; `magictools` still recommends a shut-down Gemini
  model; two byte-identical `maskKey` copies exist.
* Bad, because MADR 0003's four gateways would make the next divergence 12 edits wide.

## Decisions Resolved (2026-08-29)

Revision 1's five open questions, answered by the maintainer, plus a sixth the Ollama
answer raised. Two went against this document's recommendation; both are recorded with the
trade-off they accept.

| # | Question | Decision | Note |
|---|---|---|---|
| 1 | `magictools`'s `ProviderSpec`/tier catalog — replace or wrap? | **Wrap.** It keeps `Tier`, `Embedding` and `Dimensions`; `ID`/`Label`/`EnvVar`/`StaticModels` come from `Descriptors()`. | As recommended. Deletes the duplicated identity data and the `gemini-2.0-flash` recommendation at the source, without teaching `mcplib` about tiers. |
| 2 | Ollama — descriptor, or special case? | **Descriptor, and add a real `OllamaProvider` to `llmprovider`.** | **Scope grew.** Chosen over the narrower options so every descriptor maps to a constructible provider. Cheap in practice — see §6. |
| 3 | Reveal the key tail live, or on submit? | **Live while typing.** | **Against recommendation.** Accepts 4 characters on screen for the whole entry in exchange for instant paste confirmation. Mitigated by a minimum-length guard; reversible in one line. |
| 4 | `Prompter` — add `MultiSelect`, or loop `Select`? | **Add `MultiSelect`; six methods.** | **Against recommendation.** Accepts a larger permanent public interface for better fallback-selection UX. `pterm` supplies it natively. |
| 5 | Which Ollama API surface? | **OpenAI-compatible `/v1/chat/completions` for generation; keep the native `/api/tags` for listing.** | As recommended. Zero new wire logic; leaves working, tested listing code alone. |
| 6 | Order relative to MADR 0003? | **0003 first, then 0004.** | As recommended, and now effectively required: `OllamaProvider` consumes 0003's `chatcompletions.go`. |

**Consequent dependency:** this MADR is **blocked on MADR 0003 being accepted and executed**.
Both edit `constants.go`, `provider.go`, `models_catalog.go` and `discovery.go`, and §6
consumes 0003's shared Chat Completions primitive. 0004's descriptor table will therefore
open with **nine** providers — the four existing, 0003's four gateways, and Ollama.

## More Information

* **Codebase evidence (all verified 2026-08-29):**
  * `mcplib`: 12 dependent repos; `go.mod:5-8` two direct deps, no UI deps;
    `constants.go:3-9` provider identifiers; `provider.go:106-111` `ProviderEnvVars`;
    `models_catalog.go:25-66` static catalogs, `:26` the shut-down note, `:257`
    `RankGeminiModel` with its −2000 for 2.0/1.5 at `:289`; `discovery.go:18-38` `ListAvailableModels`;
    `probe.go:13-59` `probeGenerateHealth`; `logging/redact.go:14-16` the
    single-source-of-truth precedent; `input.go`/`prompt.go`/`stdio.go` confirming no
    terminal I/O exists today.
  * `mcp-server-magictools`: `cmd/.../config.go` 806 LOC, `:565-574` `readHiddenSecret`,
    `:576-584` `maskKey`, `:601` `selectProviderForTier`, `:696` `selectModel`;
    `internal/provider/catalog.go` 168 LOC with `ProviderSpec`, tiers, `voyage`, and
    `gemini-2.0-flash`; `internal/config/` 1,626 LOC.
  * `mcp-server-magicdev`: `cmd/.../configure.go` 610 LOC, `:27-31` local `providerEnvVars`,
    `:107-115` `maskKey`, `:182-196` `readHiddenSecret` via `term.ReadPassword`.
  * `prepare-commit-msg`: `internal/ui/setup.go` 548 LOC, `:30` the `readPassword` test
    seam, `:71-102` `promptProvider` (three providers), `:105-110` correct use of
    `ProviderEnvVars`, `:111-170` `resolveAPIKey` precedence and the Git Bash fallback,
    `:173-194` `modelAnnotations`, `:210` `discoverModels`, `:227` `promptModel`, `:284`
    `promptFallbacks`; `go.mod` `x/term` only, no `pterm`/`bubbletea`.
* **Ollama sources (verified live 2026-08-29 against a running v0.31.1 instance):**
  `https://docs.ollama.com/api/openai-compatibility` — base `http://localhost:11434/v1/`,
  endpoints `/v1/chat/completions`, `/v1/models`, `/v1/embeddings`, `/v1/responses`; API key
  "required but ignored"; `tools` and `reasoning_effort` supported, **`tool_choice` not
  supported**; reasoning values `none|low|medium|high|max`. Local probes ✓ — `/api/version`
  `200`, `/v1/models` in OpenAI list shape, and `/v1/chat/completions` returning `200` with
  a valid completion and **no** `Authorization` header.
* **Prior MADRs:** `0001` (Grok — the provider whose absence from all three wizards is this
  MADR's motivating evidence), `0002` (XDG paths — precedent for a shared `mcplib` package
  replacing per-repo copies), `0003` (four gateway providers — the change that makes this
  drift three times more expensive).
* **Sequencing.** This MADR is blocked on `0003-MADR-add-gateway-llm-providers.md` being
  accepted and executed; §6 consumes the `chatcompletions.go` primitive 0003 introduces.
* **Implementation plan** — to be written as
  `0004-PLAN-canonicalize-llm-provider-configuration.md` and approved before any source
  edits, per the repository workflow. It must phase the work additively: `mcplib` ships
  descriptors, `MaskSecret`, `Prompter`, `TextPrompter` and `ConfigureLLM` **without
  breaking any existing export**, then each consumer migrates in its own commit and release,
  in an order that lets the four-repo coordination fail safely at any point.
