---
status: proposed
date: 2026-09-25
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev, prepare-commit-msg
informed: all mcplib consumers
---
# Rank Recommended Models by Use Case from Live Catalog Metadata

> **Revision 2 (2026-09-26), from [0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md).**
> Applied in place to this `proposed` record.
>
> * **O3:** OpenCode Go returns `RegionError` for four DeepSeek models outside
>   China, and `DataPolicyError` for `-contributor` models without training
>   consent. Both become eligibility rules (§3), and the Go sixes (§7) and
>   `StaticOpencodeGo` (§5) change.
> * **O1:** Zen's Anthropic- and Google-shaped routes read the key from vendor
>   headers, which `mcplib` does not send. Three of the Zen utility six use
>   those routes, so this decision now depends on that fix (*More Information
>   → Preconditions*).
> * **O5:** OpenCode's chat route sends `reasoning_effort` when the metadata
>   advertises it, and `mcplib`'s chat route sends nothing. §6 gains that
>   path.
> * **K1:** Kilo's runtime sending `reasoning: { effort }` is now confirmed
>   from source.
> * **X5:** a reasoning-enabled call can outlive the 30 s header timeout. This
>   is a new consequence.

## Context and Problem Statement

`0009-MADR-live-catalog-model-search.md` gives every configure flow a search over
the provider's full usable catalog. The six **recommended** models are still
what a user gets by pressing Enter. They are also what prepare-commit-msg's
non-interactive path installs without asking, as the primary model and the
fallbacks. How those six are chosen is the subject of this record.

The maintainer's statement of the problem: the library "supports git commit
message generation, 8b models without thinking do not do very well". The
request is to use the live Kilo and OpenCode catalogs, beyond the six-model
limit, with use-case-specific heuristics and algorithms that improve the
default selection.

All facts below were verified on 2026-09-25 against `mcplib` at `55e4b31` plus
the proposed 0009 changes, the live public catalogs, `kilocode` at `14e7546535`
(v7.7.12) and `opencode` at `0f549842ee`. The method is recorded under *More
Information → Evidence*.

### 1. How the recommended six are chosen today

`curateFromCatalog` (`llmprovider/models_catalog.go:328-391`) keeps static-catalog
hits in catalog order, then backfills from the listing in the lister's order:

| Provider | Backfill order | Where |
|---|---|---|
| Kilo | cheapest `pricing.completion` first | `discovery.go:598-607` |
| Hugging Face | highest `throughput` first | `discovery.go:468-481` |
| OpenCode Zen/Go | name heuristic `RankOpencodeModel` (nano > lite > flash > mini > haiku; free and codex demoted) | `models_catalog.go:233-260` |

Both metadata-driven orders favour small models: the cheapest and the fastest
models are the smallest. The static catalogs, which are also the offline
fallback, include 8B models: `meta-llama/llama-3.1-8b-instruct` in `StaticKilo`
and `meta-llama/Llama-3.1-8B-Instruct` in `StaticHuggingFace`
(`models_catalog.go:92-113`).

### 2. What that yields on today's data

The live curated sixes, via the in-tree `ListAvailableModels` against the
anonymous public listings on 2026-09-25:

* **Kilo:** `meta-llama/llama-3.1-8b-instruct, openai/gpt-oss-20b,
  amazon/nova-micro-v1, mistralai/mistral-nemo, openai/gpt-oss-120b,
  inclusionai/ling-3.0-flash-fin`. Under 0009 §1b's relaxed input rule the six
  becomes `kilo-auto/small, kilo-auto/efficient, kilo-auto/balanced,
  meta-llama/llama-3.1-8b-instruct, openai/gpt-oss-20b, rekaai/reka-edge`.
* **Hugging Face:** `openai/gpt-oss-20b, openai/gpt-oss-120b,
  meta-llama/Llama-3.1-8B-Instruct, deepseek-ai/DeepSeek-V4-Flash-0731,
  zai-org/GLM-5.2, openai/gpt-oss-safeguard-20b`.
* **OpenCode Zen and Go:** equal to `StaticOpencodeZen` and `StaticOpencodeGo`.

Against the criteria "no dense model of 14B or less, and no model without
reasoning support", both Kilo sixes and the Hugging Face six fail:

* the 8B Llama in each;
* `amazon/nova-micro-v1`, `mistralai/mistral-nemo` and `rekaai/reka-edge`, which
  lack `reasoning` in Kilo's `supported_parameters`.

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

### 4. What the reference tools do for commit messages

**Kilo** (`kilocode` v7.7.12):

* Commit messages use the provider's *small model*, else the default model
  (`packages/opencode/src/kilocode/commit-message/generate.ts:24-33`). The docs
  confirm the small model is "used for session title generation, commit message
  generation, and prompt enhancement"
  (`packages/kilo-docs/pages/code-with-ai/agents/model-selection.md:18`).
* For the Kilo gateway, the small model is `kilo-auto/small`
  (`packages/opencode/src/kilocode/provider/provider.ts:297-300`). It routes to
  `google/gemma-4-26b-a4b-it` for paid accounts
  (`kilo-docs/pages/gateway/models-and-providers.md:97-104`).
* Small-task requests to the Kilo gateway **enable reasoning** at the model's
  default effort when the model supports it:
  `return { reasoning: { enabled: true } }`
  (`packages/opencode/src/provider/transform.ts:1644-1647`).
* Kilo's model selectors sort by the gateway's `preferredIndex`.
  `terminalBench` scores are displayed, not used for ranking.

**OpenCode** (`opencode` at `0f549842ee`):

* The small model is chosen by family priority, newest first:
  `["gemini-flash", "gpt-nano", "claude-haiku"]`, with `["gpt-nano"]` for its own
  providers (`packages/opencode/src/provider/provider.ts:1971-1976`, `:2048`).
* Its newer catalog selector (`packages/core/src/catalog.ts:259-294`):
  * keeps active text-to-text models with `cost > 0` released within 18 months;
  * prefers ids, families or names matching
    `\b(nano|flash|lite|mini|haiku|small|fast)\b`;
  * orders by `0.8 × cost/maxCost + 0.2 × age/maxAge`.
* Metadata for its Zen and Go models comes from a models.dev-format
  `api.json`, fetched from `https://models.opencode.ai`
  (`packages/core/src/models-dev.ts:160`, `:176`). The Zen/Go `/models`
  endpoints return ids only.

Both tools send commit messages to a *small, recent, paid* model, and Kilo turns
reasoning on for it.

### 5. Metadata available but unused

**The Kilo listing** (394 models, all fields present unless noted):

* `supported_parameters`: `reasoning` on 273 models, `reasoning_effort` on 137.
* `terminalBench.overallScore` on 34 models; `preferredIndex` on 12.
* `context_length`: median 262,144; the smallest is 4,095.
* `created`.
* `expiration_date` on 26 models, one of them 3 days away
  (`deepseek/deepseek-v3.2`, 2026-09-28).
* `pricing.prompt` and `pricing.completion`; `isFree`; `mayTrainOnYourPrompts`.
* `autoRouting` on the five `kilo-auto/*` tiers.

mcplib decodes only `id`, the modalities, `pricing.completion`,
`supported_parameters` and `mayTrainOnYourPrompts` (`discovery.go:497-508`).

**`https://models.opencode.ai/api.json`** (models.dev format; models.dev is
MIT-licensed and documents the same format at `models.dev/api.json`):

* Keys `opencode` (111 models), `opencode-go` (41) and `huggingface` (78).
* Covers 79 of 81 live Zen ids, 40 of 42 live Go ids and 73 of 139 live
  Hugging Face ids.
* Fields: `reasoning`, `reasoning_options`, `tool_call`, `cost.input` and
  `cost.output`, `limit.context`, `release_date`, `status`
  (`alpha`/`beta`/`deprecated`), `family` and `modalities`.
* 4,929,526 bytes, or 488,743 with gzip, which Go's default transport requests
  automatically. Three fetches each took 0.20–0.23 s. Response headers:
  `cache-control: public, max-age=0, must-revalidate` and an `etag`.

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
* Kilo's thinking path sends `reasoning_effort`, default `"medium"`
  (`kilo.go:151-165`). Of Kilo's 273 reasoning-capable models, 136 advertise
  `reasoning` but not `reasoning_effort`. Kilo's own client sends the
  OpenRouter-style `reasoning` object instead. `kilo-auto/free`, the package's
  live-test target (`live_gateways_test.go:170`, `:220`), is one of the 136.

A reasoning-capable default therefore may not reason when prepare-commit-msg
calls it.

### 7. Candidate-ranking simulation

A reference implementation of the ranking decided below, run over today's saved
listings, produces these sixes. The utility profile is the commit-message
default; the capable profile ranks by strength.

| Provider | utility | capable |
|---|---|---|
| Kilo | `deepseek/deepseek-v4.1-flash`, `z-ai/glm-5.3-flash`, `google/gemini-3.8-flash`, `google/gemini-3.6-flash`, `meta/muse-spark-1.2`, `thinkingmachines/inkling` | `openai/gpt-6-astra`, `anthropic/claude-fable-5.1`, `openai/gpt-5.6-sol`, `deepseek/deepseek-v4.1-flash`, `google/gemini-3.8-flash`, `x-ai/grok-4.6` |
| OpenCode Zen | `deepseek-v4.1-flash`, `qwen3.8-flash`, `glm-5.3-flash`, `deepseek-v4-flash`, `gemini-3.5-flash-lite`, `gemini-3.8-flash` | `claude-opus-5-5`, `gpt-6-sol`, `gpt-6-luna`, `grok-4.7`, `gpt-6-astra`, `muse-spark-1.3` |
| OpenCode Go | `mimo-v2.6-flash`, `qwen3.8-flash`, `glm-5.3-flash`, `gpt-6-luna`, `mimo-v2.6-pro`, `hy3` | `mimo-v2.6-pro`, `gpt-6-luna`, `grok-4.7`, `glm-5.3`, `grok-4.6`, `qwen3.8-max` |
| ~~OpenCode Go (revision 1)~~ | ~~`mimo-v2.6-flash`, `deepseek-v4.1-flash`, `qwen3.8-flash`, `glm-5.3-flash`, `deepseek-v4-flash`, `gpt-6-luna`~~ | ~~`mimo-v2.6-pro`, `gpt-6-luna`, `grok-4.7`, `muse-spark-1.3-contributor`, `glm-5.3`, `grok-4.6`~~ |
| Hugging Face | `deepseek-ai/DeepSeek-V4-Flash-0731`, `zai-org/GLM-5.3-Flash`, `deepseek-ai/DeepSeek-V4.1-Flash`, `thinkingmachines/Inkling-Small`, `stepfun-ai/Step-3.7-Flash`, `stepfun-ai/Step-3.5-Flash` | `zai-org/GLM-5.3`, `Qwen/Qwen3.8-27B`, `Qwen/Qwen3.8-2.4T-A95B`, `deepseek-ai/DeepSeek-V4-Pro-0813`, `moonshotai/Kimi-K3`, `thinkingmachines/Inkling` |

Every one of these eight sixes passes the criteria check. Today's three sixes
from §2 fail the same check, which is how the check was shown to be able to
fail.

## Decision Drivers

* **Commit messages are the primary use.** The default six must suit short,
  frequent, diff-to-text generation: reasoning-capable, recent, paid, and cheap
  enough to run on every commit. They must not include 8B-class models without
  reasoning.
* **Consumers differ.** A Thinking tier needs the strongest models, not the
  cheapest, so the use case must be selectable per call.
* **Use the provider's own metadata before name heuristics.** Where a catalog
  publishes quality, reasoning, price, context, age or expiry, use it.
* **Follow the reference implementations where they agree.** Kilo and OpenCode
  both route commit messages to a small, recent, paid model; OpenCode publishes
  the cost/age weighting; Kilo enables reasoning for small tasks.
* **Never fail configuration for want of metadata.** A missing, slow or changed
  metadata source degrades to today's ordering, silently and deterministically.
* **Keep MADR 0009's contracts.** `ListAvailableModels*` keeps its signature and
  `≤ MaxListedModels` result. Search still covers the whole usable list.
  `Prompter` is untouched.
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
* follows the two reference tools' commit-message practice;
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
func (p ModelProfile) ReasoningEffort() string // "low" for utility, "medium" for capable
```

`wizard.Options` gains `Profile llmprovider.ModelProfile`. This is additive,
and its zero value is utility. The wizard forwards it with `WithModelProfile`.
Callers that pass nothing, including `ListAvailableModels` in prepare-commit-msg's
`--yes` path and in magicdev, get the utility profile.

### 2. Metadata sources

* **Kilo:** its own listing. `kiloCatalogEntry` additionally decodes:
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
* **Failure of any metadata source** (transport, non-200, decode, timeout, id
  not covered) means that provider's candidates are ranked by today's ordering
  (§1 of Context). No error or notice reaches the caller: the listing itself
  succeeded, and 0009's `Live` flag still describes the listing.
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
7. (revision 2) its id contains `-contributor`: OpenCode Go serves those only
   with training consent (`opencode:packages/console/app/src/routes/zen/util/trainingConsent.ts`),
   which the training policy rules out;
8. (revision 2) for OpenCode Go only, it is one of `deepseek-v4.1-flash`,
   `deepseek-flash`, `deepseek-v4-flash` or `deepseek-v4-pro`. These return
   `RegionError` unless the workspace allows region `cn`, which is added by
   default only for requests from China
   (`opencode:packages/console/app/src/routes/zen/util/handler.ts:162-171`,
   `packages/console/core/src/workspace.ts:88-89`, at `696f41bc8e`). They stay
   searchable.

An unknown field does not exclude. Variable-priced Kilo tiers (`"-1"`) have
unknown cost, not zero cost.

### 4. Ordering

Each candidate has:

* `signal`: whether it has a provider quality signal (Kilo `terminalBench` or
  `preferredIndex`);
* `small`: whether its id, family or name matches
  `\b(nano|flash|lite|mini|haiku|small|fast)\b` (OpenCode's `SMALL_MODEL_RE`);
* `blend`: `0.8 × cost/maxCost + 0.2 × age/maxAge` over the eligible set,
  where unknown cost or age counts as the maximum.

* **Utility:** candidates with a signal first, then small, then lower `blend`,
  then id.
* **Capable:** candidates with a signal first, then higher `terminalBench`,
  then lower `preferredIndex`, then not-small first, then newer, then higher
  cost, then id.
* **Diversity, both profiles:** at most two per group. The group is the
  vendor prefix before `/`, with a leading `~` removed, or the metadata
  `family` for unprefixed ids.
* **Result:** the first six. If fewer than six are eligible, the remaining
  places are filled by today's ordering over the rest of the usable list, so
  the recommended list is never shorter than it is today.

### 5. Static catalogs

`StaticKilo`, `StaticOpencodeZen`, `StaticOpencodeGo` and `StaticHuggingFace`
are replaced by the utility sixes from Context §7. The snapshot date goes in
each list's comment. The replacement:

* removes `kilo-auto/free` and `nvidia/nemotron-3.5-lightning:free`, which are
  training-flagged;
* removes the 8B entries.

The static lists remain the offline fallback for both profiles.
`modelLabels` keeps its existing entries. New static ids display as bare ids,
which is the documented degradation (`models_catalog.go:638-640`).

### 6. Request-side reasoning

* Add `GenerateThinkingWithRetry(ctx, p ThinkingProvider, prompt, retries, delay)`
  with exactly `GenerateWithRetry`'s backoff, jitter and error classification.
  The two share one loop.
* Document the utility recipe: construct the provider with
  `WithReasoningEffort(ProfileUtility.ReasoningEffort())` and call
  `GenerateThinkingWithRetry`.
* **Kilo thinking path:** when the model advertises `reasoning` but not
  `reasoning_effort` (or capabilities are unknown), send the unified
  `"reasoning": {"effort": <effort>}` object, which Kilo's client uses. When
  capabilities are known to include `reasoning_effort`, keep today's field.
  This is **gated on a live opt-in test** against `kilo-auto/free`, a
  reasoning-only model reachable without a key. If the gateway rejects the
  object or returns no reasoning, the change stops and this MADR is amended.
* **OpenCode chat route (revision 2):** today `mcplib` sends no reasoning
  parameter on the Zen/Go chat route (`llmprovider/opencode.go:222-231`). So
  `GenerateThinking` does not reason on the chat-routed utility defaults
  (`glm-5.3-flash`, `mimo-v2.6-flash`, `deepseek-v4.1-flash`). OpenCode's
  client sends `reasoning_effort` on that route
  (`opencode:packages/opencode/src/provider/transform.ts:994-1002`), driven by
  the model's `reasoning_options` in `api.json`. `mcplib` does the same when
  the metadata of §2 advertises an `effort` option. This is gated on a live
  check per model family, as for Kilo.
* prepare-commit-msg adopts the recipe on its next `mcplib` bump. That change
  is in that repository, not here.

### 7. What does not change

* `Prompter`, `Result`, `TextPrompter`, and 0009's search, `ModelCatalog` and
  pagination.
* `ListAvailableModels*` signatures and the `≤ MaxListedModels` bound. Their
  *content* for Kilo, Zen, Go and Hugging Face changes, which is the purpose of
  this decision.
* The usability filters, 0009 §1b, the id deny lists and the training policy.
* First-party provider curation and `StaticGemini`, `StaticOpenAI`,
  `StaticClaude`, `StaticGrok` and `StaticOpenAIChatGPT`.
* `probeGenerateHealth`. It probes whatever six the ranking produces.

### Consequences

* Good, because the default six for all four open catalogs become recent,
  reasoning-capable flash-class models (Context §7). The 8B and non-reasoning
  entries of Context §2 leave both the live and the static defaults.
* Good, because prepare-commit-msg's `--yes` path and magicdev's `ListModels`
  improve without any consumer change: they call `ListAvailableModels`, whose
  default profile is utility.
* Good, because magictools' Thinking tier can ask for `ProfileCapable` with one
  additive field.
* Good, because the Kilo signals (`terminalBench`, `preferredIndex`, reasoning,
  expiry) come from the listing already fetched, at no extra cost.
* Good, because every metadata failure degrades to exactly today's ordering.
* Good, because enabling reasoning becomes one documented option plus one
  retry helper, not per-consumer plumbing.
* Neutral, because ranking uses signals the providers publish, not measured
  commit-message quality. The criteria are enforced; "best" is not proven. See
  the evaluation-harness option.
* Neutral, because non-signal candidates rank by cost and age alone, so a
  cheap, new, weak model can outrank a strong one on Zen, Go and Hugging Face,
  where no quality signal exists.
* Bad, because a new external dependency, `models.opencode.ai`, joins the
  listing path for three providers. Mitigated by the fallback, the in-process
  cache, the override variable and the disable switch.
* Bad, because the recommended six drift daily with the providers' catalogs.
  That is inherent to "use the live catalog". The static lists pin the offline
  view to a dated snapshot.
* Bad, because changing Kilo's thinking wire field risks a request the gateway
  rejects. The live gate in §6 must pass before it ships.
* Bad, because (revision 2) enabling reasoning on a non-streaming call raises
  its latency. `mcplib`'s shared client allows 30 s to response headers
  (`llmprovider/options.go:11-21`). Every reference client streams, with a
  300 s limit (0011-REPORT X5). The utility recipe is safe only at low effort
  until timeouts or streaming change.
* Bad, because (revision 2) the Go eligibility rules of §3 items 7-8 copy two
  lists from OpenCode's server source. They can drift. A typed
  `RegionError`/`DataPolicyError` at request time (0011-REPORT X2) is the
  durable signal.
* Bad, because the diversity cap is coarse: vendor prefix or family. Three
  OpenAI models with distinct families can still appear, for example in the
  capable Zen six.

### Confirmation

Each gate must first be seen to fail against a planted defect:

* **Pure ranking tests** over fixed candidate fixtures with known metadata:
  * the utility six contains no dense model of 14B or less, no non-reasoning,
    free, training-flagged, expiring or preview id, and at most two per group;
  * the capable six orders `terminalBench` descending;
  * *planted failures:* cost-only ordering, a dropped reasoning filter, and a
    removed diversity cap each break a named assertion.
* **Fallback:** metadata fetch returns 500, times out, or omits an id. The
  result then equals today's ordering exactly, as asserted against
  `ListAvailableModels` output captured before the change on the same
  fixtures.
* **Isolation:** with `MCPLIB_DISABLE_MODELS_METADATA=1`, no request reaches the
  metadata URL. Every unit-test package that lists Zen, Go or Hugging Face sets
  it in `TestMain` or injects a fixture URL.
* **Profiles:** `WithModelProfile(ProfileCapable)` and the zero value produce
  the two orders on one fixture. `wizard.Options.Profile` reaches the listing.
* **Reasoning:**
  * `GenerateThinkingWithRetry` retries exactly as `GenerateWithRetry` does,
    and a planted no-retry defect fails.
  * The Kilo request body carries `reasoning.effort` for a reasoning-only
    model, and `reasoning_effort` when capabilities list it.
  * The live gate on `kilo-auto/free` passes.
* **Static catalogs:** the new lists satisfy the utility criteria. The existing
  count tests (`≤ MaxListedModels`) still hold.
* **Go gates (revision 2):**
  * a Go fixture containing `deepseek-v4-flash` and `muse-spark-1.3-contributor`
    never yields either in the recommended six, and both remain in search;
  * a Zen fixture keeps `deepseek-v4-flash` eligible, because the region gate
    is Go-only;
  * *planted failures:* dropping the Go gate breaks the first assertion;
    applying it to Zen breaks the second.
* **OpenCode chat reasoning (revision 2):** for a chat-routed model whose
  metadata advertises `effort`, the request body carries `reasoning_effort`;
  without the metadata it carries none.

## Pros and Cons of the Options

### Profile-based ranking from live catalog metadata, with metadata fetched live and a fallback to today's ordering

* Good, because it uses the signals the catalogs publish, and follows the two
  reference tools where they agree.
* Good, because a profile per call serves commit messages and reasoning tiers.
* Good, because a fallback to today's ordering bounds the downside.
* Bad, because it adds an external metadata dependency and more ranking code
  to maintain.

### Re-curate the static catalogs only

* Good, because it is a small, offline, reviewable diff.
* Bad, because the live curation keeps backfilling cheapest- and fastest-first,
  so live defaults regress to small models as catalogs change.
* Bad, because it ignores `terminalBench`, reasoning and expiry, which the
  listing already carries.

### One commit-message ranking for every consumer

* Good, because there is no new option.
* Bad, because magictools' Thinking tier would be offered the same cheap
  flash-class six. The maintainer rejected it.

### Delegate to each provider's own small model

* Good, because it matches Kilo and OpenCode exactly: `kilo-auto/small` and the
  `gpt-nano` family.
* Bad, because `kilo-auto/small` resolves to one 26B model with about 4B active
  parameters and no quality signal, close to the model class the maintainer
  reports doing poorly.
* Bad, because it yields one model, not six, and Go has no `gpt-nano` model.

### Metadata from a committed snapshot instead of a live fetch

* Good, because it works offline and needs no new runtime dependency.
* Bad, because it is stale between releases, when catalogs change daily
  (26 Kilo expiry dates). The maintainer chose the live fetch.

### Measure commit-message quality directly (an evaluation harness)

* Good, because it would rank by the actual task rather than proxies.
* Bad, because it spends money and tokens on every refresh, needs a labelled
  diff corpus, and still needs a runtime ranking to apply its results. It is
  recorded as possible future work.

## More Information

### Relationship to earlier decisions

* **0009-MADR-live-catalog-model-search.md** provides `ModelCatalog.Usable`,
  the search, and §1b's input rule. This decision changes only how
  `Recommended` is chosen for four providers, and adds the reasoning helper.
  It should be implemented after 0009.
* **0003-MADR-add-gateway-llm-providers.md** chose price-ascending (Kilo) and
  throughput-descending (Hugging Face) ordering ("Kilo publishes price, not
  throughput, so cost is the available objective signal",
  `0003-MADR-add-gateway-llm-providers.md:887-889`). This decision supersedes those two orderings for the
  recommended six, while keeping them as the fallback order. 0003 is annotated
  when this MADR is accepted.
* **0004-MADR-canonicalize-llm-provider-configuration.md**: `wizard.Options`
  gains one additive field. `Prompter` is unchanged.

### Preconditions (revision 2)

* **Zen per-route key headers ([0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md) O1).** Three of the Zen utility six
  (`qwen3.8-flash` via `@ai-sdk/anthropic`, and `gemini-3.5-flash-lite` and
  `gemini-3.8-flash` via `@ai-sdk/google`) use routes whose key header
  `mcplib` does not send today. That was confirmed live on 2026-09-26.
  Shipping this ranking before that fix would move users onto models that
  fail with "Missing API key.". The fix is a candidate transport decision
  in the report, not part of this record. **Resolved:** the maintainer
  folded the fix into `0009-MADR-live-catalog-model-search.md` as §1c
  (2026-09-26). This record is implemented after 0009, so the precondition
  holds by sequencing.
* **Kilo request conventions (report K2).** `provider.data_collection:
  "deny"` is how Kilo's own client asks the gateway not to route prompts to
  providers that train on them. It complements this record's training policy
  for commit diffs. It is a related candidate, not a precondition.

### Out of scope

* Metadata-driven ranking for Gemini, OpenAI, Anthropic, Grok and Ollama. Their
  static catalogs are hand-picked, and their listings expose little metadata.
  `api.json` covers some of them, which is a possible follow-up.
* The evaluation harness.
* Consumer changes: prepare-commit-msg adopting `GenerateThinkingWithRetry`,
  and magictools setting `ProfileCapable` for its Thinking tier.
* Revising the id deny lists, which still exclude ten `-vl`/`omni` Kilo models
  (0009 Context §5).
* A disk cache for metadata.

### Evidence

* **Code facts:** read from the cited files in `mcplib` (`55e4b31`),
  prepare-commit-msg, mcp-server-magictools and mcp-server-magicdev (current
  checkouts), `kilocode` (`14e7546535`) and `opencode` (`0f549842ee`). The
  `kilocode` and `opencode` facts were gathered by read-only searches and then
  spot-checked line by line: `generate.ts:24-33`, `provider.ts:297-300`,
  `transform.ts:1644-1647`, `model-selection.md:18`,
  `models-and-providers.md:97-104`, opencode `provider.ts:1971-1976`, `:2048`,
  `catalog.ts:259-294`, `models-dev.ts:160`, `:176`.
* **Live data, anonymous GETs saved on 2026-09-25:**
  `api.kilo.ai/api/gateway/models`, `opencode.ai/zen/v1/models`,
  `opencode.ai/zen/go/v1/models`, `router.huggingface.co/v1/models` and
  `models.opencode.ai/api.json`. Field censuses and coverage counts come from
  scratch Python over those bodies. Download size and time were measured with
  `curl`, three times plus once with gzip.
* **Today's sixes (§2):** a scratch Go module outside the repository (`replace`
  to this checkout) called the in-tree `ListAvailableModels`. The relaxed-rule
  Kilo six comes from the 0009 replica.
* **Simulation (§7):** a scratch Python reference of §3–§4, run over the saved
  bodies, checked every six against the criteria (dense ≤14B, reasoning,
  training). The same check applied to today's three sixes reported violations
  in all three, so it can fail.
* **Line citations into other `docs/` records** refer to their text at
  `55e4b31`.
* **models.dev licence:** MIT, per the `sst/models.dev` repository.
* **Pinned references:** the `kilocode` and `opencode` checkouts advanced during
  this investigation (to `c267794785` and `696f41bc8e`). Every citation into
  them is to the pinned commits above, re-verified with `git show <commit>:<path>`.

### Implementation notes (not a plan)

A PLAN sharing this number should, at minimum:

* sequence it after `0009-PLAN-live-catalog-model-search.md`;
* extend `kiloCatalogEntry` and add the metadata client with its injection and
  disable switches;
* add a pure ranking function and the profile option;
* wire `wizard.Options.Profile`;
* replace the four static catalogs;
* add `GenerateThinkingWithRetry` and the Kilo `reasoning` object, behind the
  live gate;
* annotate 0003;
* prove every new check fails on a planted defect.

No source changes accompany this proposed MADR.
