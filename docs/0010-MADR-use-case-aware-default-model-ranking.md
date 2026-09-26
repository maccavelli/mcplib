---
status: proposed
date: 2026-09-26
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev, prepare-commit-msg
informed: all mcplib consumers
---
# Rank Recommended Models by Use Case from Live Catalog Metadata

> **Revision 3 (2026-09-26).** Applied in place to this `proposed` record, and
> grounded again after `0009-MADR-live-catalog-model-search.md` shipped
> (`mcplib` `5a1fc70`).
>
> **Code facts.** All `mcplib` citations are re-pinned to `5a1fc70`:
> * the 0009 refactor moved the Kilo price sort, the Hugging Face throughput
>   sort and `kiloCatalogEntry`;
> * the ranking now has a concrete hook: the per-provider `curate` argument of
>   `catalogFrom` (§2).
>
> **Reference facts.** Re-verified at `kilocode` `c267794785`, `opencode`
> `696f41bc8e`, `grok-build` `f0e3be11` and `codex` `25270df261`. Grok and Codex
> are new sources for this record. Corrections:
> * **OpenCode has no commit-message generator.** Revision 2's "both tools send
>   commit messages to a small … model" was true only of Kilo.
> * **OpenCode's cost/age selector is not its runtime behaviour.** It
>   (`CatalogV2.model.small`) is reached only from the `opencode debug v2`
>   command. OpenCode's runtime small model is a family-priority pick, and the
>   selector normalises its blend over the small subset, not the whole set.
> * **Revision 2's citation `opencode transform.ts:994-1002` is the wrong
>   path.** Those lines are the hard-coded OpenAI-compatible effort list; the
>   `reasoning_options` path is `reasoningVariants` (`:1717`).
> * **Kilo's small-task reasoning sends `{ enabled: true }` at the model's
>   default effort, not `{ effort }`** (`transform.ts:1678-1681`, moved from
>   1644-1647).
> * **Kilo's commit call streams within a 30 s total budget.** The timeout
>   consequence is restated accordingly.
>
> **New evidence.**
> * **None of Kilo, OpenCode, Grok or Codex routes short text work to a
>   non-reasoning model.** Kilo is the only one with a commit-message
>   generator. Codex runs its short utility calls on `gpt-5.6-luna` at `Low`
>   effort (Context §4).
> * **The capable profile no longer fixes `"medium"`.** Codex's catalog shows
>   rank does not imply effort, so the capable profile defers to the model's
>   own default (§1).
>
> **A gap in §3.** Kilo reports `created: 0` for every `kilo-auto/*` tier, and
> the age rule read that as 1970, excluding them. An epoch-zero or absent
> timestamp is now explicitly *unknown*.
>
> **Simulation re-run.** It now applies exactly the rules written in §3–§4,
> over the shipped `ListModelCatalog` output on 2026-09-26. It reproduces
> revision 2's tables unchanged (§7).
>
> **Wording fix (2026-09-26, while writing the PLAN).** §2 and *Confirmation*
> listed "id not covered" as a whole-provider fallback. Read literally, that
> would never rank Zen, Go or Hugging Face, because the metadata covers most
> but not all of their ids. The simulation behind §7 always treated an
> uncovered id as a candidate with unknown fields. The text now says so. The
> tables are unchanged.
>
> **Maintainer decisions (2026-09-26), on the PLAN.** Three decisions:
>
> * **`kilo-auto/*` tiers are not recommended under the utility profile.** A
>   user who wants one searches for it and adds it. Before this decision the
>   rules only kept them out of the live utility six. On a sparse catalog they
>   could still rank in, or arrive through the fill. §3 item 9 and §4's fill
>   rule now make the exclusion explicit, and it applies only to Kilo's utility
>   profile. Re-running the simulation with the rule leaves §7 unchanged.
> * **OpenCode's chat route looks up `reasoning_options` itself** (§6), rather
>   than taking them from the caller.
> * **§7's tables become a committed golden test.**
>
> **Revision 2 (2026-09-26), from [0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md).**
>
> * **O3:** OpenCode Go returns `RegionError` for four DeepSeek models outside
>   China, and `DataPolicyError` for `-contributor` models without training
>   consent. Both became eligibility rules (§3).
> * **O1:** Zen's Anthropic- and Google-shaped routes need vendor key headers.
>   That precondition is now met: 0009 §1c shipped in `90554f9`.
> * **O5:** OpenCode's chat route can send `reasoning_effort` (§6, corrected in
>   revision 3).
> * **K1:** Kilo's variant path sends `reasoning: { effort }` (§6).
> * **X5:** reasoning on a non-streaming call meets the 30 s header timeout
>   (Consequences).

## Context and Problem Statement

`0009-MADR-live-catalog-model-search.md`, now shipped, gives every configure
flow a search over the provider's full usable catalog. The six **recommended**
models are still what a user gets by pressing Enter. They are also what
prepare-commit-msg's non-interactive path installs without asking, as the
primary model and the fallbacks. How those six are chosen is the subject of
this record.

The maintainer's statement of the problem: the library "supports git commit
message generation, 8b models without thinking do not do very well". The
request is to use the live Kilo and OpenCode catalogs, beyond the six-model
limit, with use-case-specific heuristics and algorithms that improve the
default selection.

All facts below were verified on 2026-09-26 against:
* `mcplib` at `5a1fc70` (0009 complete);
* the live public catalogs;
* `kilocode` at `c267794785`, `opencode` at `696f41bc8e`, `grok-build` at
  `f0e3be11` and `codex` at `25270df261`.

The method is recorded under *More Information → Evidence*.

### 1. How the recommended six are chosen today

Each open-catalog provider's listing passes through `catalogFrom`
(`llmprovider/discovery.go:93-102`). It hands the usable ids to that provider's
`curate` function. Every curate function calls `curateFromCatalog`
(`llmprovider/models_catalog.go:328-390`), which keeps static-catalog hits in
catalog order, then backfills from the usable list in the fetch step's order:

| Provider | Fetch-step order | Curate | Where |
|---|---|---|---|
| Kilo | cheapest `pricing.completion` first (`kiloUsable`) | `curateKilo` | `discovery.go:727-735`, `:745` |
| Hugging Face | highest `throughput` first (`fetchHuggingFaceUsable`) | `curateHuggingFace` | `discovery.go:602-614`, `:624` |
| OpenCode Zen/Go | listing order; backfill by the name heuristic `RankOpencodeModel` (nano > lite > flash > mini > haiku; free and codex demoted) | closure in `opencodeCatalog` | `discovery.go:475-484`, `models_catalog.go:233-260` |

Both metadata-driven orders favour small models: the cheapest and the fastest
models are the smallest. The static catalogs, which are also the offline
fallback, include 8B models: `meta-llama/llama-3.1-8b-instruct` in `StaticKilo`
and `meta-llama/Llama-3.1-8B-Instruct` in `StaticHuggingFace`
(`models_catalog.go:92-113`).

### 2. What that yields today

The shipped `ListModelCatalog`, called anonymously on 2026-09-26, recommends:

* **Kilo** (284 usable): `kilo-auto/small, kilo-auto/efficient,
  kilo-auto/balanced, meta-llama/llama-3.1-8b-instruct, openai/gpt-oss-20b,
  rekaai/reka-edge`.
* **Hugging Face** (131 usable): `openai/gpt-oss-20b, openai/gpt-oss-120b,
  meta-llama/Llama-3.1-8B-Instruct, zai-org/GLM-5.3-Flash,
  deepseek-ai/DeepSeek-V4-Flash-0731, zai-org/GLM-5.2`.
* **OpenCode Zen** (80 usable) and **OpenCode Go** (38 usable): equal to
  `StaticOpencodeZen` and `StaticOpencodeGo`. Go's six includes
  `deepseek-v4-flash`, which OpenCode Go refuses outside China (§3 item 8).

Against the criteria "no dense model of 14B or less, no model without
reasoning support, nothing the service refuses", three sixes fail:
* Kilo: the 8B Llama, and `rekaai/reka-edge`, which lacks `reasoning` in
  Kilo's `supported_parameters`;
* Hugging Face: the 8B Llama;
* Go: the region-gated DeepSeek model.

### 3. How the consumers use the six

* **prepare-commit-msg, interactive:** `wizard.ConfigureLLM` with
  `Discover: true` and `NeedFallbacks: true` (`internal/ui/setup.go:213-219`).
* **prepare-commit-msg, non-interactive (`--yes`):**
  `llmprovider.ListAvailableModels(...)`, then `model = models[0]` and
  fallbacks from the rest (`internal/ui/setup.go:134-145`, `:310-318`). The
  first recommended model *is* the installed default.
* **prepare-commit-msg, generation:** `llmprovider.GenerateWithRetry`
  (`main.go:26`), that is, plain `Generate`.
* **mcp-server-magictools:** two tiers, "Fast Tier LLM — Primary model for
  hydration & intelligence" and "Thinking Tier LLM — Dedicated model for deep
  reasoning" (`cmd/mcp-server-magictools/config.go:74-75`, `:127-179`). One
  commit-message ranking cannot serve both.
* **mcp-server-magicdev:** `ConfigureLLM` without fallbacks
  (`cmd/mcp-server-magicdev/configure.go:274-303`), and
  `llmprovider.ListAvailableModels` in `internal/integration/llm/client.go:77-79`.

The consumers pin `mcplib` v1.5.0 (prepare-commit-msg), v1.4.1 (magictools) and
v1.2.0 (magicdev). They adopt 0009 and this record on their next bump.

### 4. What the reference tools do for short text tasks

Of the four references, only Kilo has a commit-message generator. The other
three show their practice for comparable short tasks: titles, summaries and
classifiers.

**Kilo** (`kilocode` `c267794785`):

* **Model resolution.** Commit messages use the provider's *small model*, else
  the default model (`packages/opencode/src/kilocode/commit-message/generate.ts:24-33`).
  The docs confirm the small model is used "for session title generation,
  commit message generation, and prompt enhancement"
  (`packages/kilo-docs/pages/code-with-ai/agents/model-selection.md:18`).
* **The Kilo gateway's small model** is `kilo-auto/small`
  (`packages/opencode/src/kilocode/provider/provider.ts:297-300`). It routes to
  `google/gemma-4-26b-a4b-it` for paid accounts and to its `:free` variant for
  free accounts (`packages/kilo-docs/pages/gateway/models-and-providers.md:97-104`).
* **Reasoning.** Small tasks enable reasoning at the **model's default
  effort** when the model supports it: `return { reasoning: { enabled: true } }`
  (`packages/opencode/src/provider/transform.ts:1678-1681`). An explicit effort,
  `reasoning: { effort }`, is sent only when a variant is chosen (`:1398-1399`).
* **The commit call:**
  * it **streams** (`generate.ts:37`), with a **30 s** total timeout (`:156`,
    `:187`), temperature 0.3 (`:178`) and 3 retries (`:217`);
  * each file's diff is truncated at 4,000 characters, with no total cap
    (`packages/opencode/src/kilocode/commit-message/git-context.ts:124`,
    `:222-225`).
* **Picker and metadata.**
  * Model pickers order by the gateway's `preferredIndex`, surfaced as
    `recommendedIndex` (`packages/kilo-gateway/src/api/models.ts:298`).
  * `terminalBench` is displayed but never used for ordering.
  * The client stamps every gateway model's `release_date` as today (`:294`),
    and no client code reads `expiration_date`.
  * So age and expiry, which this record uses, are signals Kilo's own client
    does not exploit.

**OpenCode** (`opencode` `696f41bc8e`):

* **No commit-message generator** (a source search finds none). Its small
  model serves session titles.
* **Runtime small-model choice** is a family priority, newest first:
  `["gemini-flash", "gpt-nano", "claude-haiku"]`, with `["gpt-nano"]` alone for
  its own `opencode`/`opencode-go` providers
  (`packages/opencode/src/provider/provider.ts:1971-1980`, `:2048`). It does not
  check cost or age.
* **Status filtering.** At load, `alpha` models (unless experimental models
  are enabled) and `deprecated` models are removed; `beta` models are kept
  (`provider.ts:1693-1694`).
* **The experimental cost/age selector** (`packages/core/src/catalog.ts:244-286`,
  `SMALL_MODEL_RE` at `:294`) is a design reference, not runtime practice:
  * it keeps active text-to-text models with `cost > 0` released within 18
    months;
  * it prefers `\b(nano|flash|lite|mini|haiku|small|fast)\b`;
  * it orders by `0.8 × cost/maxCost + 0.2 × age/maxAge`, normalised over the
    small subset (`pick(items)`, `:268-283`);
  * its only caller is `packages/opencode/src/cli/cmd/debug/v2.ts:24`.
* **Metadata.** For Zen and Go models it comes from
  `https://models.opencode.ai/api.json` (`packages/core/src/models-dev.ts:160`,
  `:176`). The Zen/Go `/models` endpoints return ids only.

**Codex** (`codex` `25270df261`):

* **No commit-message generator** (a source search finds none).
* **Short utility calls use a small reasoning tier at `Low`:**
  * thread titles run on `gpt-5.6-luna` at `ReasoningEffort::Low`
    (`codex-rs/tui/src/app/thread_title.rs:30`, `:101`);
  * memory extraction runs on `gpt-5.6-luna` at `Low`
    (`codex-rs/model-provider/src/provider.rs:130`,
    `codex-rs/memories/write/src/lib.rs:80-82`).
* **Heavier background work steps up a tier and an effort level.** Memory
  consolidation runs on `gpt-5.6-terra` at `Medium` (`provider.rs:134`,
  `lib.rs:104-106`).
* **Default model and effort.**
  * The default is the highest-priority picker-visible model: `gpt-6-astra`
    (`codex-rs/models-manager/src/manager.rs:168-179`,
    `codex-rs/protocol/src/openai_models.rs:1022-1032`).
  * Its default effort is **low** (`codex-rs/models-manager/models.json:41`),
    while `gpt-6-sol`'s is medium (`:214`). **Rank does not imply effort.**
  * The catalog has no price field, and every model's lowest effort is `low`.

**Grok** (`grok-build` `f0e3be11`):

* **No commit-message generator.** The README has users pipe the staged diff
  to the main agent in headless mode (`crates/codegen/xai-grok-shell/README.md:763`).
* **Session titles** go to `grok-4.6`, the frontier model
  (`crates/codegen/xai-grok-models/default_models.json:2-5`). The request
  carries a 100-token cap and no effort
  (`crates/codegen/xai-grok-shell/src/session/helpers/session_summary.rs:229-231`).
* **Utility classifiers** use `low` effort where the model supports it
  (`crates/codegen/xai-grok-shell/src/util/config/resolve/auto_mode.rs:211-230`).
* **A non-reasoning model**, `grok-4-1-fast-non-reasoning`, is used only for
  64-token next-prompt ghost text
  (`crates/codegen/xai-grok-shell/src/util/config/resolve/prompt_suggest.rs:8`,
  `:211`).
* **The catalog has no price field.**

**Where they agree.** None of the four sends short text work to a
non-reasoning model. Grok's ghost text is the one exception, and it is
64-token completion, not diff summarisation. The two that run background text
on a cheaper tier (Codex, Kilo) pair a small reasoning-capable model with
`low` or the model's default effort.

### 5. Metadata available but unused

**The Kilo listing** (394 models on 2026-09-26, all fields present unless
noted):

* `supported_parameters`: `reasoning` on 273 models, `reasoning_effort` on 137.
* `terminalBench.overallScore` on 34 models; `preferredIndex` on 12, where
  `kilo-auto/efficient` is 0.
* `context_length`: median 262,144; the smallest is 4,095.
* `created`: `0` for all five `kilo-auto/*` tiers.
* `expiration_date` on 26 models.
* `pricing.prompt` and `pricing.completion`; `isFree`; `mayTrainOnYourPrompts`.
* `autoRouting` on the five `kilo-auto/*` tiers.

`mcplib` decodes only `id`, the modalities, `pricing.completion`,
`supported_parameters` and `mayTrainOnYourPrompts` (`kiloCatalogEntry`,
`discovery.go:630-641`).

**`https://models.opencode.ai/api.json`** (models.dev format; models.dev is
MIT-licensed and documents the same format at `models.dev/api.json`):

* Keys `opencode` (112 models on 2026-09-26), `opencode-go` (42) and
  `huggingface`.
* Covers 80 of 82 live Zen ids and 41 of 43 live Go ids.
* Fields: `reasoning`, `reasoning_options`, `tool_call`, `cost.input` and
  `cost.output`, `limit.context`, `release_date`, `status`
  (`alpha`/`beta`/`deprecated`), `family` and `modalities`.
* 4,932,611 bytes on 2026-09-26, or about 489 KB with gzip, which Go's default
  transport requests automatically. Fetches measured on 2026-09-25 took
  0.20–0.23 s. Response headers: `cache-control: public, max-age=0,
  must-revalidate` and an `etag`.

**The Hugging Face listing** carries, per provider offering: `pricing`,
`context_length`, `supports_tools`, `throughput` and `first_token_latency_ms`.
It has no reasoning flag.

### 6. The "thinking" half is not requested

* Plain `Generate` sends no reasoning parameter on any provider.
* All eight providers implement `ThinkingProvider.GenerateThinking`
  (`provider.go:50-53`).
* `WithReasoningEffort` and `WithThinkingBudget` exist (`options.go:71-86`).
* There is no retrying form of `GenerateThinking`. `GenerateWithRetry` takes a
  `Provider` and calls `Generate` (`provider.go:128`).
* **Kilo.** The thinking path sends `reasoning_effort`, default `"medium"`
  (`kilo.go:151-165`). Of Kilo's 273 reasoning-capable models, 136 advertise
  `reasoning` but not `reasoning_effort`. `kilo-auto/free`, the package's
  live-test target (`live_gateways_test.go:170`, `:220`), is one of the 136.
* **OpenCode chat route.** It deliberately sends no reasoning parameter
  (`opencode.go:222-226`).

A reasoning-capable default therefore may not reason when prepare-commit-msg
calls it.

### 7. Candidate-ranking simulation

A reference implementation of exactly §3–§4, run on 2026-09-26 over the shipped
`ListModelCatalog` output with today's metadata, produces these sixes. The
utility profile is the commit-message default; the capable profile ranks by
strength.

| Provider | utility | capable |
|---|---|---|
| Kilo | `deepseek/deepseek-v4.1-flash`, `z-ai/glm-5.3-flash`, `google/gemini-3.8-flash`, `google/gemini-3.6-flash`, `meta/muse-spark-1.2`, `thinkingmachines/inkling` | `openai/gpt-6-astra`, `anthropic/claude-fable-5.1`, `openai/gpt-5.6-sol`, `deepseek/deepseek-v4.1-flash`, `google/gemini-3.8-flash`, `x-ai/grok-4.6` |
| OpenCode Zen | `deepseek-v4.1-flash`, `qwen3.8-flash`, `glm-5.3-flash`, `deepseek-v4-flash`, `gemini-3.5-flash-lite`, `gemini-3.8-flash` | `claude-opus-5-5`, `gpt-6-sol`, `gpt-6-luna`, `grok-4.7`, `gpt-6-astra`, `muse-spark-1.3` |
| OpenCode Go | `mimo-v2.6-flash`, `qwen3.8-flash`, `glm-5.3-flash`, `gpt-6-luna`, `mimo-v2.6-pro`, `hy3` | `mimo-v2.6-pro`, `gpt-6-luna`, `grok-4.7`, `glm-5.3`, `grok-4.6`, `qwen3.8-max` |
| ~~OpenCode Go (revision 1)~~ | ~~`mimo-v2.6-flash`, `deepseek-v4.1-flash`, `qwen3.8-flash`, `glm-5.3-flash`, `deepseek-v4-flash`, `gpt-6-luna`~~ | ~~`mimo-v2.6-pro`, `gpt-6-luna`, `grok-4.7`, `muse-spark-1.3-contributor`, `glm-5.3`, `grok-4.6`~~ |
| Hugging Face | `deepseek-ai/DeepSeek-V4-Flash-0731`, `zai-org/GLM-5.3-Flash`, `deepseek-ai/DeepSeek-V4.1-Flash`, `thinkingmachines/Inkling-Small`, `stepfun-ai/Step-3.7-Flash`, `stepfun-ai/Step-3.5-Flash` | `zai-org/GLM-5.3`, `Qwen/Qwen3.8-27B`, `Qwen/Qwen3.8-2.4T-A95B`, `deepseek-ai/DeepSeek-V4-Pro-0813`, `moonshotai/Kimi-K3`, `thinkingmachines/Inkling` |

* **Every six passes the criteria check.** Today's shipped sixes from §2 fail
  the same check, which shows the check can fail.
* **The paid `kilo-auto` tiers are eligible but do not reach the utility
  six.** Once §3's epoch rule applies, the Kilo eligible set rises from 189 to
  193 with `kilo-auto/small`, `/efficient`, `/balanced` and `/frontier`.
  * `kilo-auto/efficient` carries Kilo's quality signal (`preferredIndex` 0)
    but has unknown cost, so it ranks last among signal models.
  * `kilo-auto/small` has no signal.
  * Both remain one search away (0009).
  * §3 item 9 (maintainer decision, 2026-09-26) now excludes them from the
    utility profile outright. Re-running the simulation with that rule gives
    the same two Kilo sixes.
* **Revision 2's simulation reached the same sixes by a different path.** It
  had silently dropped every `kilo-auto` tier and grouped Hugging Face models
  by family. The rules as written give the same result.

## Decision Drivers

* **Commit messages are the primary use.** The default six must suit short,
  frequent, diff-to-text generation: reasoning-capable, recent, paid, and cheap
  enough to run on every commit. They must not include 8B-class models without
  reasoning.
* **Consumers differ.** A Thinking tier needs the strongest models, not the
  cheapest, so the use case must be selectable per call.
* **Use the provider's own metadata before name heuristics.** Where a catalog
  publishes quality, reasoning, price, context, age or expiry, use it.
* **Follow the references where they agree** (Context §4): short text work
  goes to a reasoning-capable model at low or default effort, never to a
  non-reasoning one; the effort for a strong model is that model's own default.
  The cost/age blend is adopted from OpenCode's selector as a design, knowing
  it is not OpenCode's runtime behaviour.
* **Never fail configuration for want of metadata.** A missing, slow or changed
  metadata source degrades to today's ordering, silently and deterministically.
* **Keep MADR 0009's contracts.** `ModelCatalog`, `ListModelCatalog*` and
  `ListAvailableModels*` keep their signatures and `≤ MaxListedModels` bound.
  Search still covers the whole usable list. `Prompter` is untouched.
* **Private diffs stay private.** The training-policy exclusion stands
  (`models_catalog.go:590-598`).
* **Testable offline.** Unit tests never reach models.opencode.ai. Ranking is a
  pure function of `(profile, candidates, metadata, now)`.

## Considered Options

* Profile-based ranking from live catalog metadata, with metadata fetched live and a fallback to today's ordering
* Re-curate the static catalogs only
* One commit-message ranking for every consumer
* Delegate to each provider's own small model (`kilo-auto/small`, the `gpt-nano` family)
* Metadata from a committed snapshot instead of a live fetch
* Measure commit-message quality directly (an evaluation harness) and rank by the measurements

## Decision Outcome

Chosen option: **"Profile-based ranking from live catalog metadata, with
metadata fetched live and a fallback to today's ordering"**. It is the only
option that:

* uses the signals the catalogs already publish;
* serves commit messages and reasoning tiers differently;
* matches the references' shared practice for short text work;
* degrades to today's behaviour when metadata is unavailable.

The maintainer chose the utility/capable profile pair, the live fetch with
fallback, and request-side reasoning on the mcplib side (2026-09-25).

### 1. Profiles

```go
// ModelProfile selects how recommended models are ranked.
type ModelProfile int

const (
	// ProfileUtility (the zero value) ranks for short, frequent tasks such as
	// commit messages: reasoning-capable, recent, paid, cheap.
	ProfileUtility ModelProfile = iota
	// ProfileCapable ranks for reasoning-heavy tasks: strongest first.
	ProfileCapable
)

func WithModelProfile(p ModelProfile) ProviderOption

// ReasoningEffort is the recommended request effort for the profile: "low" for
// ProfileUtility, and "" for ProfileCapable, meaning the model's own default.
func (p ModelProfile) ReasoningEffort() string
```

* **`wizard.Options` gains `Profile llmprovider.ModelProfile`.** This is
  additive, and its zero value is utility. The wizard forwards it with
  `WithModelProfile`.
* **Callers that pass nothing get the utility profile.** That includes
  `ListAvailableModels` in prepare-commit-msg's `--yes` path and in magicdev.
* **Why `"low"` for utility:** Codex's title and extraction calls use `Low`,
  Grok's classifiers use `low` where supported, and `low` is the minimum effort
  in both first-party catalogs (Context §4).
* **Why the capable profile defers:** Codex's catalog shows the
  strongest-ranked model defaulting to `low` and the next to `medium`, so a
  fixed value would override vendor intent. Revision 2's `"medium"` is
  withdrawn.

### 2. Metadata sources and the ranking hook

* **Hook.** The ranking replaces each open-catalog provider's `curate` function
  as passed to `catalogFrom` (`discovery.go:93-102`): `curateKilo`,
  `curateHuggingFace`, and the `opencodeCatalog` closure.
  * The profile travels in `ProviderConfig`, set by `WithModelProfile`.
  * Metadata travels in the curate closure.
  * `catalogFrom`, `ModelCatalog`, the degrade-to-static contract and the
    `Live` flag are unchanged.
* **Kilo:** its own listing. The fetch step keeps each usable model's entry for
  the ranker, not just its id. `kiloCatalogEntry` additionally decodes:
  * `name`, `created`, `context_length`, `expiration_date`;
  * `pricing.prompt`, `preferredIndex`, `terminalBench.overallScore`.

  There is no extra request.
* **OpenCode Zen, OpenCode Go and Hugging Face:**
  `https://models.opencode.ai/api.json`, keys `opencode`, `opencode-go` and
  `huggingface`, joined on model id.
  * It is fetched concurrently with the listing, under the same 10 s listing
    context.
  * It is cached in-process for 10 minutes, so a configure run that lists
    several providers fetches it once.
  * It can be overridden with `WithModelMetadataURL(url)` or the environment
    variable `MCPLIB_MODELS_METADATA_URL`, and disabled with
    `MCPLIB_DISABLE_MODELS_METADATA=1`. This follows OpenCode's
    `OPENCODE_MODELS_URL` and `OPENCODE_DISABLE_MODELS_FETCH`.
* **Failure of the metadata source** (transport, non-200, decode, timeout,
  disabled, or a document without the provider's key) means that provider's
  candidates are ranked by today's curation (Context §1). No error or notice
  reaches the caller: the listing itself succeeded, and 0009's `Live` flag
  still describes the listing.
* **An id the metadata does not cover** is still a candidate, with every field
  unknown (§3). It is not a failure of the source: on 2026-09-26 the document
  covered 80 of 82 live Zen ids and 41 of 43 Go ids, and 78 Hugging Face
  entries against 131 usable router ids.
* **Gemini, OpenAI, Anthropic, Grok and Ollama** keep today's curation under
  both profiles. See *Out of scope*.

### 3. Eligibility for the recommended six

Search (0009) still covers every usable id. For the **recommended** six, a
candidate is excluded when a known field says:

1. it is not reasoning-capable (Kilo `supported_parameters` lacks `reasoning`;
   metadata `reasoning` is false);
2. it is free (price 0): rate-limited, and on Kilo often training-flagged;
3. it was released more than 18 months ago (Kilo `created`; metadata
   `release_date`);
4. its context is under 32,768 tokens;
5. its status is `deprecated` or `alpha`, or it expires within 30 days
   (Kilo `expiration_date`);
6. its id matches `preview|-exp\b|experimental|alpha` (case-insensitive);
7. its id contains `-contributor`: OpenCode Go serves those only with training
   consent (`opencode:packages/console/app/src/routes/zen/util/trainingConsent.ts:1-3`),
   which the training policy rules out;
8. for OpenCode Go only, it is one of `deepseek-v4.1-flash`, `deepseek-flash`,
   `deepseek-v4-flash` or `deepseek-v4-pro`. These return `RegionError` unless
   the workspace allows region `cn`, which is added by default only for
   requests from China
   (`opencode:packages/console/app/src/routes/zen/util/handler.ts:161-171`,
   `packages/console/core/src/workspace.ts:88-89`). They stay searchable.
9. for Kilo under `ProfileUtility` only, its id starts with `kilo-auto/`.
   Kilo's managed tiers route to a model Kilo chooses: `kilo-auto/small`
   resolves to one 26B model with about 4B active parameters, and on free
   accounts to its `:free` variant (Context §4). The maintainer ruled them out
   of the commit-message default (2026-09-26). They stay searchable, and they
   remain eligible under `ProfileCapable`.

**Unknown values never exclude:**
* an absent field;
* a Kilo `created` of `0` or less (revision 3: all five `kilo-auto/*` tiers
  report 0);
* a variable Kilo price (`"-1"`).

A variable price is *unknown* cost, not zero cost.

### 4. Ordering

Each candidate has:

* `signal`: whether it has a provider quality signal (Kilo `terminalBench` or
  `preferredIndex`);
* `small`: whether its id, family or name matches
  `\b(nano|flash|lite|mini|haiku|small|fast)\b` (OpenCode's `SMALL_MODEL_RE`);
* `blend`: `0.8 × cost/maxCost + 0.2 × age/maxAge` over the whole eligible set,
  where unknown cost or age counts as the maximum. OpenCode's selector
  normalises over its small subset only; normalising over the whole set keeps
  one scale for both tiers of the utility order.

* **Utility:** candidates with a signal first, then small, then lower `blend`,
  then id.
* **Capable:** candidates with a signal first, then higher `terminalBench`,
  then lower `preferredIndex`, then not-small first, then newer, then higher
  cost, then id.
* **Diversity, both profiles:** at most two per group. The group is the
  vendor prefix before `/`, with a leading `~` removed, or the metadata
  `family` for unprefixed ids.
* **Result:** the first six. If fewer than six are eligible, the remaining
  places are filled from today's curated order, then the rest of the usable
  list, so the recommended list is never shorter than it is today. The fill
  skips what §3 item 9 excludes, so under the utility profile a sparse Kilo
  catalog yields fewer than six rather than a `kilo-auto/*` tier.

### 5. Static catalogs

`StaticKilo`, `StaticOpencodeZen`, `StaticOpencodeGo` and `StaticHuggingFace`
are replaced by the utility sixes from Context §7. The snapshot date goes in
each list's comment. The replacement:

* removes `kilo-auto/free` and `nvidia/nemotron-3.5-lightning:free`, which are
  training-flagged;
* removes the 8B entries;
* removes the region-gated `deepseek-v4-flash` from Go.

The static lists remain the offline fallback for both profiles.
`modelLabels` keeps its existing entries. New static ids display as bare ids,
which is the documented degradation (`models_catalog.go:638-640`).

### 6. Request-side reasoning

* **Retry helper.** Add
  `GenerateThinkingWithRetry(ctx, p ThinkingProvider, prompt, retries, delay)`
  with exactly `GenerateWithRetry`'s backoff, jitter and error classification.
  The two share one loop. `0012-MADR-conform-providers-to-reference-clients.md`
  §1.2 extends that loop's terminal-error rules to all three helpers.
* **Utility recipe.** Construct the provider with
  `WithReasoningEffort(ProfileUtility.ReasoningEffort())` (`"low"`) and call
  `GenerateThinkingWithRetry`. For the capable profile, leave the effort unset.
* **Kilo thinking path.** Kilo's client uses two shapes (Context §4):
  * an explicit effort `reasoning: { effort }` on its variant path;
  * `reasoning: { enabled: true }`, the model's default effort, for small
    tasks.

  `mcplib` sends:
  * `{ "reasoning": { "effort": <effort> } }` when an effort is configured and
    the model advertises `reasoning` (or capabilities are unknown);
  * `{ "reasoning": { "enabled": true } }` when no effort is configured;
  * today's `reasoning_effort` only when capabilities are known to list it and
    not `reasoning`.

  The utility recipe's explicit `low` therefore differs from Kilo's own
  commit-message call, which uses the default effort, and matches Codex and
  Grok. It is **gated on a live opt-in test** against `kilo-auto/free`, a
  reasoning-only model reachable without a key. If the gateway rejects either
  shape or returns no reasoning, the change stops and this MADR is amended.
* **OpenCode chat route.**
  * **Today:** `mcplib` sends no reasoning parameter
    (`llmprovider/opencode.go:222-226`), so `GenerateThinking` does not reason
    on the chat-routed utility defaults (`glm-5.3-flash`, `mimo-v2.6-flash`,
    `deepseek-v4.1-flash`).
  * **OpenCode's client:** it builds per-model efforts from `api.json`
    `reasoning_options` (`reasoningVariants`,
    `opencode:packages/opencode/src/provider/transform.ts:1717`, selected at
    `provider.ts:1314`). It sends one only for a chosen variant, or the first
    variant for small tasks (`transform.ts:1391`, `:1412`).
  * **`mcplib` will:** send `reasoning_effort: <effort>` when an effort is
    configured and the model's `reasoning_options` lists that value, and send
    nothing otherwise. This is gated on a live check per model family, as for
    Kilo.
* **Consumer adoption.** prepare-commit-msg adopts the recipe on its next
  `mcplib` bump. That change is in that repository, not here.

### 7. What does not change

* `Prompter`, `Result`, `TextPrompter`, and 0009's search, `ModelCatalog`,
  `catalogFrom` and pagination.
* `ListAvailableModels*` and `ListModelCatalog*` signatures and the
  `≤ MaxListedModels` bound. Their *content* for Kilo, Zen, Go and Hugging Face
  changes, which is the purpose of this decision.
* The usability filters, 0009 §1b, the id deny lists and the training policy.
* First-party provider curation and `StaticGemini`, `StaticOpenAI`,
  `StaticClaude`, `StaticGrok` and `StaticOpenAIChatGPT`. Refreshing
  `StaticOpenAIChatGPT` belongs to 0012 §4.3.
* `probeGenerateHealth`. It probes whatever six the ranking produces.

### Consequences

* Good, because the default six for all four open catalogs become recent,
  reasoning-capable flash-class models (Context §7). The 8B, non-reasoning and
  region-gated entries of Context §2 leave both the live and the static
  defaults.
* Good, because prepare-commit-msg's `--yes` path and magicdev's `ListModels`
  improve without any consumer change: they call `ListAvailableModels`, whose
  default profile is utility.
* Good, because magictools' Thinking tier can ask for `ProfileCapable` with one
  additive field.
* Good, because the Kilo signals (`terminalBench`, `preferredIndex`, reasoning,
  expiry) come from the listing already fetched, at no extra cost. Kilo's own
  client uses none of them for ranking (Context §4).
* Good, because every metadata failure degrades to exactly today's curation.
* Good, because enabling reasoning becomes one documented option plus one
  retry helper, not per-consumer plumbing.
* Neutral, because ranking uses signals the providers publish, not measured
  commit-message quality. The criteria are enforced; "best" is not proven. See
  the evaluation-harness option.
* Neutral, because non-signal candidates rank by cost and age alone, so a
  cheap, new, weak model can outrank a strong one on Zen, Go and Hugging Face,
  where no quality signal exists.
* Neutral, because Kilo's managed tiers, which Kilo's docs recommend for
  background work, are never in the utility six (§3 item 9, a maintainer
  decision). This diverges from Kilo's own client, which sends commit messages
  to `kilo-auto/small`. The tiers remain one search away, and eligible under
  `ProfileCapable`.
* Bad, because a new external dependency, `models.opencode.ai`, joins the
  listing path for three providers. Mitigated by the fallback, the in-process
  cache, the override variable and the disable switch.
* Bad, because the recommended six drift daily with the providers' catalogs.
  That is inherent to "use the live catalog". The static lists pin the offline
  view to a dated snapshot.
* Bad, because changing Kilo's thinking wire fields risks a request the gateway
  rejects. The live gate in §6 must pass before it ships.
* Bad, because enabling reasoning on a non-streaming call raises its latency,
  and `mcplib`'s shared client allows 30 s to response headers
  (`llmprovider/options.go:11-21`). Kilo's own commit call fits in a 30 s
  *total* budget with reasoning on, but it **streams** (Context §4). A
  non-streaming call must finish all reasoning before the first header byte.
  The utility recipe is therefore safe at low effort. The general fix is 0012
  §1.3's 300 s timeouts.
* Bad, because the Go eligibility rules of §3 items 7–8 copy two lists from
  OpenCode's server source. They can drift. A typed
  `RegionError`/`DataPolicyError` at request time (0012 §1.1) is the durable
  signal.
* Bad, because the diversity cap is coarse: vendor prefix or family. Three
  OpenAI models with distinct families can still appear, for example in the
  capable Zen six.

### Confirmation

Each gate must first be seen to fail against a planted defect:

* **Pure ranking tests** over fixed candidate fixtures with known metadata:
  * the utility six contains no dense model of 14B or less, no non-reasoning,
    free, training-flagged, expiring, preview or Go-gated id, and at most two
    per group;
  * the capable six orders `terminalBench` descending;
  * a candidate with `created: 0` is eligible;
  * under the utility profile, Kilo never recommends a `kilo-auto/*` id,
    neither ranked nor filled. Under the capable profile, or on another
    provider, the same candidate is kept;
  * *planted failures:* cost-only ordering, a dropped reasoning filter, a
    removed diversity cap, and epoch-zero read as a date, each breaking a named
    assertion.
* **Fallback:** the metadata fetch returns 500, times out, fails to decode, is
  disabled, or lacks the provider's key. The result then equals that
  provider's curation function (`curateHuggingFace`, or the `opencodeCatalog`
  closure) applied to the same usable list. A covered and an uncovered id in
  one fixture are both ranked, the uncovered one with every field unknown.
* **Isolation:** with `MCPLIB_DISABLE_MODELS_METADATA=1`, no request reaches the
  metadata URL. Every unit-test package that lists Zen, Go or Hugging Face sets
  it in `TestMain` or injects a fixture URL.
* **Profiles:**
  * `WithModelProfile(ProfileCapable)` and the zero value produce the two
    orders on one fixture;
  * `wizard.Options.Profile` reaches the listing;
  * `ProfileUtility.ReasoningEffort() == "low"` and
    `ProfileCapable.ReasoningEffort() == ""`.
* **Reasoning:**
  * `GenerateThinkingWithRetry` retries exactly as `GenerateWithRetry` does,
    and a planted no-retry defect fails.
  * The Kilo request body carries `reasoning.effort` when an effort is set,
    `reasoning.enabled` when none is set, and `reasoning_effort` only when
    capabilities list it and not `reasoning`.
  * The live gate on `kilo-auto/free` passes for both shapes.
* **Static catalogs:** the new lists satisfy the utility criteria. The existing
  count tests (`≤ MaxListedModels`) still hold.
* **Go gates:**
  * a Go fixture containing `deepseek-v4-flash` and `muse-spark-1.3-contributor`
    never yields either in the recommended six, and both remain in search;
  * a Zen fixture keeps `deepseek-v4-flash` eligible, because the region gate
    is Go-only;
  * *planted failures:* dropping the Go gate breaks the first assertion;
    applying it to Zen breaks the second.
* **OpenCode chat reasoning:**
  * for a chat-routed model whose `reasoning_options` lists the configured
    effort, the body carries `reasoning_effort`;
  * with the effort unlisted, or no metadata, it carries none.

## Pros and Cons of the Options

### Profile-based ranking from live catalog metadata, with metadata fetched live and a fallback to today's ordering

* Good, because it uses the signals the catalogs publish, and matches the
  references' shared practice for short text work.
* Good, because a profile per call serves commit messages and reasoning tiers.
* Good, because a fallback to today's curation bounds the downside.
* Bad, because it adds an external metadata dependency and more ranking code
  to maintain.

### Re-curate the static catalogs only

* Good, because it is a small, offline, reviewable diff.
* Bad, because the live curation keeps backfilling cheapest- and fastest-first,
  so live defaults regress to small models as catalogs change. Today's Kilo and
  Hugging Face sixes already include an 8B model (Context §2).
* Bad, because it ignores `terminalBench`, reasoning and expiry, which the
  listing already carries.

### One commit-message ranking for every consumer

* Good, because there is no new option.
* Bad, because magictools' Thinking tier would be offered the same cheap
  flash-class six. The maintainer rejected it.

### Delegate to each provider's own small model

* Good, because it matches Kilo exactly (`kilo-auto/small`), and OpenCode's
  title model (the `gpt-nano` family).
* Bad, because `kilo-auto/small` resolves to one 26B model with about 4B active
  parameters, on free accounts a `:free` variant, and carries no quality
  signal. That is close to the model class the maintainer reports doing
  poorly.
* Bad, because it yields one model, not six. OpenCode's runtime rule finds no
  `gpt-nano` model on Go at all and falls back to the session model.

### Metadata from a committed snapshot instead of a live fetch

* Good, because it works offline and needs no new runtime dependency.
  OpenCode itself embeds a build-time snapshot as a fallback.
* Bad, because it is stale between releases, when catalogs change daily
  (26 Kilo expiry dates). The maintainer chose the live fetch.

### Measure commit-message quality directly (an evaluation harness)

* Good, because it would rank by the actual task rather than proxies.
* Bad, because it spends money and tokens on every refresh, needs a labelled
  diff corpus, and still needs a runtime ranking to apply its results. It is
  recorded as possible future work.

## More Information

### Relationship to earlier decisions

* **0009-MADR-live-catalog-model-search.md** (complete) provides
  `ModelCatalog.Usable`, the search, §1b's input rule and §1c's key headers.
  This decision changes only how `Recommended` is chosen for four providers,
  and adds the reasoning helper.
* **0012-MADR-conform-providers-to-reference-clients.md** (proposed):
  * reuses this record's metadata client (its §3.1) and its
    `GenerateThinkingWithRetry` (its §1.2);
  * its §1.3 timeouts relieve the latency consequence above;
  * its §1.1 typed errors carry the Go region and data-policy refusals.
* **0003-MADR-add-gateway-llm-providers.md** chose price-ascending (Kilo) and
  throughput-descending (Hugging Face) ordering ("Kilo publishes price, not
  throughput, so cost is the available objective signal",
  `0003-MADR-add-gateway-llm-providers.md:887-889` at `55e4b31`). This decision
  supersedes those two orderings for the recommended six, while keeping them as
  the fallback order. 0003 is annotated when this MADR is accepted.
* **0004-MADR-canonicalize-llm-provider-configuration.md**: `wizard.Options`
  gains one additive field. `Prompter` is unchanged.

### Preconditions

* **Zen per-route key headers** (0011-REPORT O1). **Met:** 0009 §1c shipped in
  `90554f9`, confirmed live by `TestLive_OpencodeKeyHeaderPerRoute`. Three of
  the Zen utility six use those routes: `qwen3.8-flash` via `@ai-sdk/anthropic`,
  and `gemini-3.5-flash-lite` and `gemini-3.8-flash` via `@ai-sdk/google`.
* **Kilo request conventions** (report K2). `provider.data_collection: "deny"`
  is how Kilo's client asks the gateway not to route prompts to providers that
  train on them. It is decided in 0012 §3.3, not here, and is not a
  precondition.

### Out of scope

* Metadata-driven ranking for Gemini, OpenAI, Anthropic, Grok and Ollama. Their
  static catalogs are hand-picked, and their listings expose little metadata.
  Codex's `priority`, `visibility` and `upgrade` fields (Context §4) are the
  obvious first-party signals for a follow-up.
* The evaluation harness.
* Consumer changes: prepare-commit-msg adopting `GenerateThinkingWithRetry`,
  and magictools setting `ProfileCapable` for its Thinking tier.
* Revising the id deny lists, which still exclude ten `-vl`/`omni` Kilo models
  (0009 Context §5).
* A disk cache, or an embedded snapshot, for metadata.

### Evidence

* **Code facts:**
  * `mcplib` at `5a1fc70`;
  * prepare-commit-msg, mcp-server-magictools and mcp-server-magicdev at their
    current checkouts (`8b06402`, `368ac6a`, `2338243`).
* **Reference facts:** read-only searches of `kilocode` (`c267794785`),
  `opencode` (`696f41bc8e`), `grok-build` (`f0e3be11`) and `codex`
  (`25270df261`). Their load-bearing lines were then opened with
  `git show <commit>:<path>` before use:
  * Kilo: `generate.ts:37`, `:156`, `:178`, `:208`, `:217`; `git-context.ts:124`,
    `:222-225`; `transform.ts:1678-1681`; `api/models.ts:294`, `:298`;
    `models-and-providers.md:85-87`;
  * OpenCode: `catalog.ts:244-286`, `debug/v2.ts:24`, `provider.ts:1314`,
    `:1693-1694`, `transform.ts:994-1002` (revision 2's misattributed lines),
    `:1391`, `:1412`, `:1717`;
  * Codex: `thread_title.rs:30`, `:101`; `provider.rs:124`, `:130`, `:134`;
    `memories/write/src/lib.rs:80-82`, `:104-106`; `models.json:41`, `:214`,
    `:385-386`;
  * Grok: `default_models.json:2-5`; `prompt_suggest.rs:8`, `:211`.

  Between revision 2's pins and these, only Kilo's `transform.ts` changed among
  the cited files: lines moved by +34, with no change of meaning.
* **Live data, anonymous GETs on 2026-09-26:**
  `api.kilo.ai/api/gateway/models`, `opencode.ai/zen/v1/models`,
  `opencode.ai/zen/go/v1/models`, `router.huggingface.co/v1/models` and
  `models.opencode.ai/api.json`. The 2026-09-25 bodies were kept for
  comparison.
* **Today's sixes (§2):** a scratch Go module outside the repository (`replace`
  to this checkout) called the shipped `ListModelCatalog` for the four open
  catalogs and dumped both views.
* **Simulation (§7):** a scratch Python implementation of §3–§4 as written,
  including the epoch rule and the fill rule, ranked those `Usable` lists with
  today's metadata, and checked every six against the criteria (dense ≤14B,
  reasoning, training, Go gates). The same check reports violations in the
  shipped sixes of §2, so it can fail.
* **Line citations into other `docs/` records** refer to their text at
  `55e4b31` unless stated.
* **models.dev licence:** MIT, per the `sst/models.dev` repository.

### Implementation notes (not a plan)

The plan is
[0010-PLAN-use-case-aware-default-model-ranking.md](0010-PLAN-use-case-aware-default-model-ranking.md).
It should, at minimum:

* add the profile to `ProviderConfig`, and a pure ranking function with the
  §3–§4 rules;
* swap the three open-catalog `curate` functions for profile-aware rankers
  that fall back to today's curation;
* keep Kilo entries through the fetch step, extend `kiloCatalogEntry`, and add
  the metadata client with its injection and disable switches;
* wire `wizard.Options.Profile`;
* replace the four static catalogs;
* add `GenerateThinkingWithRetry` and both Kilo reasoning shapes, behind the
  live gate;
* annotate 0003;
* prove every new check fails on a planted defect.

No source changes accompany this proposed MADR.
