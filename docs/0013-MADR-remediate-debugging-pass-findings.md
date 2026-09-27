---
status: proposed
date: 2026-09-27
decision-makers: mcplib maintainers
consulted: prepare-commit-msg, mcp-server-magictools, mcp-server-magicdev
informed: all mcplib consumers
---
# Remediate the Defects Found by the Post-0010 Debugging Pass

> **Revision 2 (2026-09-27).** An assessment pass proved every finding before
> the plan was written. It changes the record as follows:
> * **Two new findings.** B9: Claude `GenerateThinking` gets HTTP 400 on
>   `claude-sonnet-5` and `claude-opus-4-8`. B10: two `StaticClaude` ids answer
>   404.
> * **Q1 gains an option (c)**, per-model wire shapes, now recommended.
>   Option (a), a 1,024-token budget, cannot work on Claude 4.7 and later.
> * **Q3's recommendation flips to (b).** Not every free Zen model is refused:
>   `space-bunny-free` answers 200.
> * **A1–A3 are latent.** No entry in the 2026-09-26 captures triggers them.
> * **B8 did not reproduce live.**
> * **C8's wording is corrected**, and **C10 no longer promises a test.**
> * **D5 and D6 are added**, both routed to 0012.
>
> [0013-PLAN-remediate-debugging-pass-findings.md](0013-PLAN-remediate-debugging-pass-findings.md)
> implements the recommended answers. Each of its tests was seen to fail on
> `ca29b81`, or on a named mutant, before its fix was proven to pass.

## Context and Problem Statement

[0009-MADR-live-catalog-model-search.md](0009-MADR-live-catalog-model-search.md)
(live catalog search) and
[0010-MADR-use-case-aware-default-model-ranking.md](0010-MADR-use-case-aware-default-model-ranking.md)
(use-case ranking, metadata, request-side reasoning) have landed on `main`. So
have part of
[0012-MADR-conform-providers-to-reference-clients.md](0012-MADR-conform-providers-to-reference-clients.md)
§1.4 (the OpenCode session header) and a `go fix` modernisation. Together
they add roughly 2,000 lines to `llmprovider` and `wizard`.

A debugging pass on 2026-09-26, at `ca29b81`, looked for bugs, gaps against
those MADRs, and missing or incomplete wiring. The question this record
decides is **which findings to fix, where, and in what order**.

### Method

* **Debugging pass (2026-09-26).** Three reviews ran in parallel, each
  read-only against the repository: ranking and metadata; provider request
  paths; the wizard and the matcher. Each copied the tree to a scratch
  directory and proved its claims with probe tests that were never committed.
* **Assessment pass (2026-09-27).** Every finding below was re-proven before
  the plan was written:
  * each fix's regression test was run on an archive of `ca29b81` and seen to
    fail with the message quoted in the plan;
  * a coverage profile of `./llmprovider` and `./wizard` confirmed each listed
    coverage gap;
  * live probes covered Anthropic, Gemini, OpenCode Zen and Go;
  * the full live suite ran on the proven fix tree.
* **Race detector.** `go test -race -count=1 ./llmprovider ./wizard` was clean
  at `ca29b81`, and after each plan phase.

Severity: **bug** is wrong behaviour; **gap** is behaviour missing or differing
from a MADR; **wiring** is an option or path that does not reach the code
meant to honour it; **nit** is low-impact.

### A. Ranking and metadata (0010)

A1–A3 are **latent**. The 2026-09-26 captures hold 394 Kilo, 82 Zen, 43 Go and
139 Hugging Face entries. None repeats an id, and no Kilo entry has a blank,
absent or non-finite price. The fixes therefore leave MADR 0010 §7's sixes
unchanged; the golden test passes after them.

| # | Severity | Where | Finding | Evidence | Status |
|---|---|---|---|---|---|
| A1 | bug (latent) | `llmprovider/model_ranking.go:203-237`, `discovery.go:722` | A listing that repeats an id recommends it twice. `kiloUsable` does not dedupe, and `rankRecommended` lets two copies through the per-group cap. The old `curateFromCatalog` deduped. `Usable` repeats it too. | `Recommended lists a/pro 2 times: [a/pro a/pro b/pro]`; `Usable lists a/pro 2 times` | confirmed |
| A2 | gap (latent) | `model_ranking.go:305-311` | `kiloPrice("")` is a *known* price of 0, so a Kilo entry with no `pricing` block is excluded as free. MADR 0010 §3 says an absent field never excludes. `model_ranking_test.go:320` pins the blank-is-0 rule. | `no pricing block: costKnown=true eligible=false` | confirmed |
| A3 | bug (latent) | `model_ranking.go:310-311`, blend at `:153-155` | A price of `"Infinity"` parses as known. `maxCost` becomes +Inf, that model's blend becomes `Inf/Inf = NaN`, and `cmp.Compare` orders NaN first, so the costliest model ranks cheapest. | `ranked = [z/infinite a/cheap b/mid]` | confirmed |
| A4 | wiring | `opencode.go:353-357`, `huggingface.go:173-177`, `kilo.go:217-221`; `options.go:45-48`, `:121-123` | Each open catalog's `DiscoverModels` lists with `ProviderConfig{HTTPClient, BaseURL}` only. `WithModelMetadataURL` given to `NewOpencode` is ignored there, `NewHuggingFace` keeps no metadata URL, and no provider keeps `ModelProfile`. Discovery therefore always ranks for utility, against whatever document the environment names. The profile half is the documented contract ("Ignored by provider constructors"), so fixing it changes that contract. | Kilo capable: `ranked = [b/flash d/mid c/pro …]` (utility order); HF and Zen: fallback order, `fetched the environment's metadata URL 1 times` | confirmed |
| A5 | wiring | `discovery.go:109-111`, `:67`; `model_metadata.go:229` | Only `ListModelCatalogWithSource` bounds listing to 10 s. The `DiscoverModels` listings of Claude, Gemini, Ollama, OpenCode, Hugging Face and Kilo run without a deadline. OpenAI and Grok list through `ListAvailableModelsWithSource` and are bounded (`openai.go:204-210`, `grok.go:239-245`). On the unbounded path `metadataCurate` waits on `<-meta` for as long as the HTTP client allows, against MADR 0010 §2's "under the same 10 s listing context". | 6 of 6: `GET … ran without a deadline` | confirmed |
| A6 | bug (perf) | `model_metadata.go:120-141`, `opencode.go:239-248` | Metadata failures are not cached (0010 PLAN §1.11 item 3). `chatReasoningEffort` looks the document up inside every chat-route thinking call, under the caller's context, so with the metadata host down every call and every retry waits on a fresh fetch. Reasoning is then silently dropped. When a cached document expires and the refetch fails, the stale copy is discarded too. | `3 thinking calls made 3 metadata fetches`; `metadata lookup ran without a deadline`; stale: `refresh failed: … HTTP 500` | confirmed |
| A7 | nit | `model_metadata.go:227-229` | The curate closure reads a one-shot channel, so a second call would block forever. `catalogFrom` calls it at most once today. | reading | confirmed (latent) |
| A8 | nit | `model_metadata.go:122-139` | No single-flight: concurrent cold-cache listings each fetch the 4.9 MB document. | reading | confirmed |
| A9 | nit | `discovery.go:791` | `KiloModelCapabilities` sets no timeout of its own. It relies on the caller's context or the client's 60 s. | reading | confirmed |
| A10 | nit | `model_ranking.go:37` | `alpha` matches anywhere in an id (for example `alphacode`). This follows MADR 0010 §3 item 6 literally. | reading | confirmed |

### B. Provider request paths (0010 §1, §6; 0012 §1.4)

| # | Severity | Where | Finding | Evidence | Status |
|---|---|---|---|---|---|
| B1 | gap | `claude.go:124-136`, `:177-181`; `opencode.go:178-187`, `:208-214`; `gemini.go:45-55` | MADR 0010 §6's utility recipe, `WithReasoningEffort("low")` + `GenerateThinkingWithRetry`, gives **low** reasoning only on effort-based wires. See "Where the recipe gives low" below. `options.go:33-35` still documents the effort as "OpenAI" only. | wire bodies below | confirmed |
| B2 | gap | `openai.go:25`, `huggingface.go:134`, `ollama.go:150`, `opencode.go:164`, `grok_reasoning.go:59` | MADR 0010 §1 says `ProfileCapable.ReasoningEffort() == ""` means "the model's own default". On the wire, `""` becomes `"medium"` on OpenAI, Hugging Face, Ollama and OpenCode responses, and `"high"` on Grok 4.5. Only Kilo sends the model-default `{"enabled": true}`. | probe: `openai {"reasoning":{"effort":"medium"}}`, `grok(grok-4.5) {"reasoning":{"effort":"high"}}` | confirmed |
| B3 | gap | `http_helpers.go:106` | Every 4xx other than 401, 403 and 429 becomes `ErrInvalidRequest`, and the body is discarded. The live gateways' explanations are lost: 402 "Insufficient account funds", 400 `MissingSessionID`, 400 "Model is unavailable", and the 400 of B9. A 403 `FreeTierError` is reported as `ErrAuthFailure`. | `HTTP 402: attempts=1 invalid=true`; live B9: `llm: invalid request: claude HTTP 400` | confirmed; owned by 0012 §1.1 |
| B4 | gap | `http_helpers.go:106` | HTTP 408 (request timeout) is terminal, so it is never retried. | `HTTP 408: attempts=1 invalid=true` | confirmed; owned by 0012 §1.2 |
| B5 | nit | `provider.go:191` | A negative `retries` makes no call and returns `failed after 0 attempts: %!w(<nil>)`. `GenerateItemsWithRetry` has the same flaw. | probe output | confirmed |
| B6 | nit | `provider.go:196` | `GenerateItemsWithRetry` keeps its own copy of the loop (0010 PLAN §1.11 item 5). 0012 §1.2 must edit both. | reading | confirmed; owned by 0012 §1.2 |
| B7 | gap | `discovery.go:502`; each `DiscoverModels` probe | The `GET /models` listing sends no `x-opencode-session`, and each health-probe provider draws a new session id. 0012 §1.4 says "every request"; only generation requests were in scope for the pull-forward. The listing works without the header today. | `ListAvailableModels last=GET /models x-opencode-session=""` | confirmed absent; impact suspected |
| B8 | gap | `gemini.go:23`, `:45-55` | Gemini counts thinking tokens against `maxOutputTokens` (8192). With the dynamic budget, a long task could end truncated. | Not reproduced: `gemini-3.7-flash`, dynamic budget, 8192 cap, commit-message prompt: 3 of 3 runs `STOP`, 333–415 thought tokens (2026-09-27) | suspected (long inputs only) |
| **B9** | **bug** | `claude.go:177-181`, `opencode.go:178-187`; `models_catalog.go:54-62` | **Claude `GenerateThinking` fails on current models.** Anthropic answers today's `thinking: {type: "enabled", budget_tokens: 4096}` on `claude-sonnet-5` and `claude-opus-4-8` with HTTP 400. These are two of `StaticClaude`'s six. The same request goes to Zen's messages route for Claude ids of 4.7 and later; that route could not be checked live (D2). | live, 2026-09-27: `"thinking.type.enabled" is not supported for this model. Use "thinking.type.adaptive" and "output_config.effort" to control thinking behavior.`; mcplib at `ca29b81`: `GenerateThinking: llm: invalid request: claude HTTP 400` on both models, both efforts | confirmed |
| **B10** | **gap** | `models_catalog.go:54-62` | Two `StaticClaude` ids are retired: `claude-3-5-haiku-latest` and `claude-sonnet-4-20250514`. The static catalog is what the wizard offers when the listing fails, so both are dead ends. | live, 2026-09-27: HTTP 404 `not_found_error` for both; the Models API lists neither | confirmed |

**Where the recipe gives low (B1)**, from wire bodies recorded against
`httptest` and live probes on 2026-09-27:

| Wire | Behaviour at `ca29b81` | Body |
|---|---|---|
| OpenCode responses, OpenAI, Grok 4.5 | honours low | `reasoning.effort:"low"` |
| OpenCode chat (when metadata lists it), Hugging Face, Ollama | honours low | `reasoning_effort:"low"` |
| Kilo | honours low | `reasoning:{"effort":"low"}` |
| Claude up to 4.6, OpenCode messages (qwen, older Claude) | ignores it | `thinking.budget_tokens:4096` |
| Claude 4.7 and later (`claude-sonnet-5`, `claude-opus-4-8`) | **request refused** (B9) | `thinking.type:"enabled"` → HTTP 400 |
| Gemini, OpenCode google | ignores it | `thinkingBudget:-1` (dynamic) |
| Grok 4 | sends no reasoning field | none |

Three of the Zen utility six use those budget routes: `qwen3.8-flash` on
messages, and `gemini-3.5-flash-lite` and `gemini-3.8-flash` on google. So does
Go's `qwen3.8-flash`. The metadata document gives all three an `effort`
reasoning option that includes `low`. OpenCode's own client sends Gemini that
effort as `thinkingConfig.thinkingLevel`
(`packages/opencode/src/provider/transform.ts:1789-1791` at `696f41bc8e`).

**What each budget wire accepts (live, 2026-09-27):**
* **Anthropic.**
  * `claude-haiku-4-5` takes a budget only. It refuses `adaptive` ("adaptive
    thinking is not supported on this model") and `output_config.effort`
    ("This model does not support the effort parameter.").
  * The budget minimum is 1024. 1023 is refused on every model: "budget_tokens:
    Input should be greater than or equal to 1024".
  * `claude-sonnet-4-6` accepts both forms.
  * `claude-sonnet-5` and `claude-opus-4-8` accept only `thinking: {type:
    "adaptive"}`, with or without `output_config: {effort: "low"}`. That
    includes a forced `tool_choice`.
* **Gemini.**
  * `thinkingLevel: "low"` is accepted on `gemini-3.5-flash-lite` and
    `gemini-3.7-flash`. It used 0 thought tokens, against 275 and 333–415
    with the dynamic budget.
  * Gemini 2.5 refuses `thinkingLevel` ("Thinking level is not supported for
    this model") and accepts `thinkingBudget: 1024`.
* **OpenCode Go messages (`qwen3.8-flash`).** Every shape is accepted: budget
  4096, budget 1024, adaptive with effort, effort alone, and none. Thinking
  length does not track the setting (143–520 characters between runs).

### C. Wizard and matcher (0009, 0010 §1)

| # | Severity | Where | Finding | Evidence | Status |
|---|---|---|---|---|---|
| C1 | bug | `wizard/text_prompter.go:171-204`, `wizard/model_select.go:182-192` | Repeating an index in a fallback MultiSelect duplicates the fallback. The parser keeps repeats, and `appendPicks` does not check what was already chosen. | `MultiSelect(1,1,2) = [0 0 1]`; `Fallbacks = ["claude-sonnet-5" "claude-sonnet-5" "claude-sonnet-4-6"]` | confirmed |
| C2 | wiring | `wizard/configure.go:14-16`, `:52-53`, `:281-285`; `discovery.go:67` | `Options.DiscoverLimit` can't raise the listing budget above the lister's hard 10 s, so the wizard's own 20 s default never applies. When the lister fails, the cause is swallowed: `ListModelCatalogWithSource` degrades with a nil error, and the user sees only the static-catalog notice. Surfacing the cause needs a new exported field, `ModelCatalog.Err`. | DiscoverLimit 30 s, server delay 12 s → `elapsed=10s`; notice without cause | confirmed |
| C3 | gap | `wizard/model_select.go:109-157` | `Existing.Fallbacks` is never preselected, so reconfiguring forgets them. Already recorded as 0009 "Related findings" item 4 and kept as is. | `Existing.Fallbacks=["claude-sonnet-5"] … Fallbacks=[]` | confirmed (known) |
| C4 | bug | `wizard/model_select.go:98-107`, `wizard/configure.go:143` | The Other prompt and the "No models found" prompt both default to `Existing.Model` whatever the saved provider was, so Enter saves another provider's model. MADR 0009 §4.3 limits "current" to the same provider. | `Enter at Other saved provider=gemini model="claude-opus-5"`; `… provider=ollama model="claude-opus-5"` | confirmed |
| C5 | nit | `wizard/model_select.go:71-75` | The default row matches `Existing.Model` in the recommended list without checking `Existing.Provider`. | reading | confirmed |
| C6 | nit | `wizard/model_select.go:159-178` | Excluding the primary is case-sensitive, while `SearchModels` dedupes case-insensitively. A primary typed as `Claude-Haiku-4-5` via Other is offered back as the fallback `claude-haiku-4-5`. | `fallback menu has 6 rows, want 5` | confirmed |
| C7 | nit | `wizard/model_select.go:103`, `wizard/configure.go:147` | Both manual prompts accept a whitespace-only id from any `Prompter` that doesn't trim. `TextPrompter` trims. | `Other with "   ": model="   "`; `No models found with blanks: model="   "` | confirmed |
| C8 | nit | `wizard/text_prompter.go:183-202` | The MultiSelect parser rejects `1,2,`, `1 2` and `1,,2`, then re-prompts. At end of input, an unparsable last line is an error and an empty last line accepts the preselection, as Enter does. | probe output | confirmed |
| C9 | nit | `llmprovider/model_matcher.go:44`, `:122-123`, `:140-143` | Any `?` forces the glob path, so `sonnet?` finds nothing. The label-substring tier matches curated annotation text, so `[` matches models whose ids contain no `[`. Label tokens keep their bracket (`[balanced`), so the token-prefix tier cannot reach a bracketed annotation word. | `"sonnet?" -> []`, `"[" -> [gpt-4.1 claude-sonnet-5]`; without the label tier, `balanced speed -> []` | confirmed |
| C10 | bug (edge) | `wizard/configure.go:272`, `llmprovider/oauth_session.go:71-74`, `wizard/auth.go:159` | The wizard treats any OpenAI OAuth credential as ChatGPT. The lister requires `Issuer == DefaultOpenAIIssuer`. `keepExistingOAuth` copies `Existing.Issuer` as is. So a consumer-saved config with an empty issuer would send the ChatGPT token to the API-key model listing, and the static notice would stay suppressed. Every session mcplib creates sets the issuer (`auth.go:211`, `import.go:90`, `oauth_loopback.go:163`), so only state persisted by a consumer can reach this. | the two predicates, by reading | suspected (runtime path not reproduced) |

### D. Live behaviour and operations

| # | Severity | Where | Finding | Evidence | Status |
|---|---|---|---|---|---|
| D1 | gap | Zen free models; `live_gateways_test.go:107-179` | OpenCode Zen's free tier refuses most third-party use, and mcplib reports the 403 as `ErrAuthFailure`, which points users at their key. `TestLive_OpencodeResponses` and `TestLive_OpencodeRouteStillEnforced` fail on it; `TestLive_OpencodeChatCompletions` skips (its `hy3-free` is no longer listed). **Not every free id is refused**, so a suffix rule cannot tell them apart. | Live, 2026-09-27, the 10 usable `-free` ids of the 2026-09-26 Zen listing: 8 answer `403 FreeTierError "OpenCode's free tier can only be used from within OpenCode"`; `space-bunny-free` answers 200 on Zen and on Go; `deepseek-v4-flash-free` answers `400 "Upstream request failed: Model is unavailable."` | confirmed |
| D2 | open | Zen paid models | Every paid Zen model and route answers `402 "Upstream request failed: Insufficient account funds"`. OpenCode's gateway source reports an empty workspace balance as a 401 `CreditsError` with a different message. It uses the "Upstream request failed:" prefix for provider-side errors (see D1). The failure is therefore likely on OpenCode's upstream side, not the account. Until it clears, the DeepSeek family's live reasoning check (0010 PLAN Phase 6) stays outstanding: it is region-gated on Go. | 2026-09-26: five models across four upstreams; 2026-09-27: `gemini-3.8-flash` (google) still 402 | unresolved |
| D3 | gap | `README.md` | The README documents none of 0010's API: profiles, `WithModelProfile`, `WithModelMetadataURL`, the `MCPLIB_*` variables, `GenerateThinkingWithRetry`, the `kilo-auto` exclusion, or the session header. This is 0010 PLAN Phase 7, still open. | a search finds none of these names | confirmed |
| D4 | nit | Kilo free-model live tests | `kilo-auto/free` now routes to `poolside/laguna-s-2.1:free` and is often rate-limited (429 skips in both live runs). | live runs | confirmed |
| **D5** | **gap** | `live_gateways_test.go:42-52` | `skipIfTransient` skips on every `ErrInvalidRequest`. A wire regression that the gateway answers with 400 therefore skips instead of failing, in every live test that uses it. Kilo's reasoning gate treats 400 as DRIFT separately. Telling "model unavailable" from "bad request" needs B3's typed errors. | reading; `hy3-free`'s 400 skipped on 2026-09-26 | confirmed; owned by 0012 §1.1 |
| **D6** | **nit** | `options.go:11-21`; live suite | One full live run failed `TestLive_OpencodeChatReasoningEffort/glm-5.3-flash` with `http2: timeout awaiting response headers`, the shared client's 30 s header limit. | 2 of 2 re-runs passed on the fix tree, and 2 of 2 on `ca29b81` | confirmed (transient); owned by 0012 §1.3 |

**Live suite at `ca29b81` (2026-09-26):**

| Result | Tests |
|---|---|
| PASS | OpenCode Go reasoning gate; Kilo reasoning gate; metadata document; key-header-per-route; listings without credentials; Hugging Face ×2; Kilo tool call, reasoning spelling and supported parameters |
| FAIL | `TestLive_OpencodeResponses`, `TestLive_OpencodeRouteStillEnforced` (D1) |
| SKIP | `TestLive_OpencodeChatCompletions` (D1), `TestLive_KiloChatCompletions` (429) |

**Live suite on the plan's proven fix tree (2026-09-27)**, including the
plan's new live tests:

| Result | Tests |
|---|---|
| PASS | every other OpenCode test, including the three moved to Go; metadata document; key-header-per-route; listings without credentials; Hugging Face ×2; Kilo supported parameters and reasoning gate; `TestLive_StaticClaudeServed` (4 of 4); `TestLive_ClaudeThinkingShapes` (6 of 6); `TestLive_GeminiThinkingShapes` (4 of 4); `TestLive_OpencodeMessagesThinking` |
| FAIL | `TestLive_OpencodeChatReasoningEffort/glm-5.3-flash`, once (D6; passed on re-run) |
| SKIP | `TestLive_KiloChatCompletions`, `TestLive_KiloToolCall`, `TestLive_KiloReasoningSpelling` (429, D4) |

### Coverage gaps (existing tests only)

A coverage profile at `ca29b81` shows zero hits on each of these blocks:
* `configure.go:293-296`: the listing-error warning;
* `configure.go:297-299`: a live listing with an empty recommended list. This
  branch is **observably equivalent** to falling through: both paths reach
  "No models found". A mutant that deletes it survives the whole wizard
  suite, so a test can reach the branch but cannot pin it.
* `model_select.go:72-74`: the default row;
* `model_select.go:103-105`, `:128-130`, `:139-141`: an empty Other id, a
  blank fallback round with nothing left, and a fallback search with no
  matches;
* `model_matcher.go:89-90`, `:122-123`: the glob `?` path and the
  label-substring tier.

## Decision Drivers

* **Protect commit-message defaults.** Wrong or duplicated recommendations
  (A1–A3) and silently dropped reasoning (A6, B1) hit prepare-commit-msg's
  `--yes` path directly.
* **A documented path must work.** `GenerateThinking` on a static catalog
  model (B9) and a static id that no longer exists (B10) fail users who did
  nothing unusual.
* **Options must do what they say.** Options that fail to reach the code
  meant to honour them (A4, A5, C2) are defects even when the default works.
* **Keep ownership clean.** 0012 already owns typed errors, retry rules,
  timeouts and identification (B3, B4, B6, B7, D5, D6). Duplicating them here
  would split one decision across two records.
* **No unproven fixes.** Every fix lands with a test that has been seen to
  fail, as in 0009 and 0010.
* **Follow the reference client where the wire is vendor-specific.** OpenCode's
  `transform.ts` already maps efforts per model family for Anthropic and
  Gemini.

## Considered Options

* Remediate in a 0013 PLAN, route transport items to 0012, and settle four design questions first
* Fold every finding into 0012
* Fix only the confirmed bugs; record everything else as accepted
* Do nothing now

## Decision Outcome

Chosen option: **"Remediate in a 0013 PLAN, route transport items to 0012, and
settle four design questions first"**. It fixes the defects that affect
defaults, requests and wiring now. It keeps each item in the record that
already owns it, and it forces an explicit answer where the finding is a
design gap rather than a slip.

### 1. Fixed by the 0013 PLAN

The plan is written for the recommended answers to Q1–Q4. It names the phases
each answer governs.

* **Ranking correctness (A1–A3).**
  * A1: `catalogFrom` dedupes usable ids, and `rankRecommended` skips an id
    already chosen.
  * A2: a blank price is unknown.
  * A3: an infinite price is unknown.
* **Discovery wiring (A4–A5).**
  * A4: the Kilo, OpenCode and Hugging Face providers keep `ModelProfile` and,
    where they rank with metadata, `ModelMetadataURL`. Their `DiscoverModels`
    passes both. The `ModelProfile` doc drops "Ignored by provider
    constructors".
  * A5: every `DiscoverModels` listing runs under `modelListingTimeout` (10 s),
    the bound `ListModelCatalogWithSource` already applies.
* **Metadata on the request path (A6).**
  * A failed fetch is remembered for one minute (`modelMetadataRetryAfter`).
  * A stale document is kept when a refresh fails.
  * The chat route's lookup waits at most 5 s (`metadataLookupTimeout`).
  * This supersedes 0010 PLAN §1.11 item 3.
* **Thinking shapes (B9, and B1 under Q1 (c); B2 under Q2 (a)).**
  * B9 needs no answer: Claude 4.7 and later get `thinking: {type:
    "adaptive"}`, plus `output_config.effort` when an effort is set.
  * Q1 (c) decides how "low" maps on the budget models.
  * Q2 (a) is documentation only.
* **Static catalog (B10).** The two retired ids leave `StaticClaude`.
* **Wizard (C1, C2 under Q4 (b), C4, C6, C7).**
  * C1: `appendPicks` skips an id already chosen, and the TextPrompter's
    MultiSelect counts a repeated index once.
  * C2: `ModelCatalog.Err` carries the listing failure, the wizard's notice
    names it, and `defaultDiscoverLimit` becomes 10 s. `DiscoverLimit` is
    documented as unable to extend the lister's bound.
  * C4: the Other and "No models found" prompts default to the saved model
    only for its own provider.
  * C6: fallback exclusion ignores case.
  * C7: a blank id is refused at both prompts.
* **Live tests (D1).** The three OpenCode generation tests move to paid
  OpenCode Go models. Zen's free-tier refusal becomes a typed error in 0012
  (Q3 (b)).
* **Tests.** The coverage gaps above get tests. Each new guard test that
  passes on `ca29b81` is proven by a named mutant.
* **Documentation.** The README gains two paragraphs for this record's
  behaviour. The README section for 0010's API (D3) stays with 0010 PLAN
  Phase 7.

### 2. Routed to 0012

* B3 (typed `APIError`). §1.1's table also needs OpenCode's 403
  `FreeTierError` and the 402 "Upstream request failed" error.
* B4 (408 is retryable) and B6 (one retry loop for all three helpers).
* B7 (the session header on every request, one id per logical session).
* D5 (live tests skip only the transient classes once errors are typed).
* D6 (the generation client's header timeout).

0012 gets a pointer bullet for these in the plan's records phase.

### 3. Four questions the reviewer must answer

**Q1 — B1: the utility recipe on budget-based routes.**
* **Options:**
  * (a) Map `WithReasoningEffort` onto a thinking budget on the messages and
    google wires, for example `low` → 1024. *Evidence against:* Claude 4.7
    and later refuse any budget (B9). Gemini 3.7 Flash still thought for 237
    tokens on a 1,024 budget.
  * (b) Document the recipe as effort-routes only, and have consumers set
    `WithThinkingBudget` themselves. B9 is still fixed.
  * (c) **Per-model wire shapes, as OpenCode's client chooses them.**
    * Claude 4.7 and later: adaptive thinking with `output_config.effort`,
      using OpenCode's version test
      (`transform.ts:657-666`). An unversioned `claude-` id counts as current,
      as there.
    * Other messages models: an enabled budget, with `low` → 1024, the
      documented minimum.
    * Gemini 3 and later: `low` → `thinkingLevel: "low"`.
    * Gemini 1.x and 2.x: `low` → a 1,024 budget, using OpenCode's
      `GEMINI_LEGACY_RE` (`transform.ts:521`).
    * An explicit `WithThinkingBudget` still wins where a budget applies.
      Other efforts keep today's budget.
* **Recommendation:** (c). It is the only option whose every shape was
  accepted live. It gives the two Gemini utility defaults no thinking at `low`,
  and it matches the reference client.

**Q2 — B2: what `""` means for the capable profile.**
* **Options:**
  * (a) Amend MADR 0010 §1: `""` means *each provider's documented default*.
    That is `medium` on effort APIs, `high` on Grok 4.5, `{"enabled": true}`
    on Kilo, adaptive with no effort on Claude 4.7 and later, a 4,096 budget
    on older Claude, and dynamic on Gemini.
  * (b) Change the providers so `""` sends no effort. Every existing
    `GenerateThinking` caller that relied on the implicit `medium` changes
    behaviour.
* **Recommendation:** (a). It describes today's behaviour honestly, and
  breaks no consumer.

**Q3 — D1: free OpenCode Zen models.**
* **Options:**
  * (a) Exclude Zen `-free` ids from the usable list. *Evidence against:*
    `space-bunny-free` works (200) on Zen and Go, so a suffix rule hides a
    working model. Free ids are already never recommended, because §3 of 0010
    excludes a known zero cost.
  * (b) Keep them searchable, and classify `FreeTierError` distinctly under
    0012 §1.1, so the error says why.
* **Either way:** the free-model live tests move to paid Go models.
* **Recommendation:** (b).

**Q4 — C2: the discovery budget.**
* **Options:**
  * (a) The lister's 10 s becomes a default that a caller deadline may extend
    up to `DiscoverLimit`.
  * (b) Keep 10 s fixed. Document `DiscoverLimit` as unable to extend it, and
    surface the failure's cause (`ModelCatalog.Err`).
* **Recommendation:** (b). It is simpler, and the metadata fetch then shares
  one known bound.

### 4. Accepted, recorded, not changed

* A7, A8, A9, A10: latent or low-impact. A10 follows the MADR text.
* B5: the documented contract is `retries ≥ 0`. The message is fixed with B6
  in 0012.
* B8: not reproduced on short inputs; stays *suspected* for long ones.
* C3: kept by 0009's decision.
* C5, C8, C9: behaviour as designed. C9 is noted for a future matcher
  revision.
* C10: stays *suspected*. It needs consumer-persisted state with an empty
  issuer, which no mcplib path creates. No test is added until it is
  reproduced.
* `configure.go:297-299`: kept. It is reachable, and the plan tests that it
  runs, but it is observably equivalent (coverage gaps).
* D2: external. The DeepSeek live check stays outstanding in 0010 PLAN
  Phase 6.
* D4: external.

### Consequences

* Good, because every confirmed bug that can change a recommended model, fail
  a thinking request or drop reasoning gets fixed, with a test that has been
  seen to fail.
* Good, because `DiscoverModels` honours the same options and bound as
  `ListModelCatalog`, so the two ways of listing cannot disagree.
* Good, because 0012 stays the single owner of transport semantics.
* Neutral, because `ModelCatalog` gains an exported field (`Err`). The
  addition is backward compatible.
* Neutral, because `StaticClaude` shrinks from six ids to four, and five
  wizard tests stop hard-coding six rows.
* Bad, because the Zen google and messages routes cannot be proven live
  while D2 stands. Their shapes are proven against `httptest` and on the
  upstream APIs directly.
* Bad, because B3's unhelpful 402, 403 and 400 messages persist until 0012
  §1.1 ships.

### Confirmation

* Each fixed finding has a regression test that fails on `ca29b81`, with the
  failure recorded, and passes after the fix. A new guard test that cannot
  fail on `ca29b81` is instead shown to fail on a named mutant.
* `go test -race -count=1 ./llmprovider ./wizard` and `go test -count=1 ./...`
  pass after every phase, as do `make lint` and `go vet` with and without
  `-tags live_gateways`.
* The live suite has no FAIL. It may skip only on 429 or an unset credential.
  A transport timeout is re-run once, and both runs are recorded (D6).
* 0012 carries the pointer bullet, and 0010's §1 and PLAN §1.11 item 3 carry
  amendment notes.

## Pros and Cons of the Options

### Remediate in a 0013 PLAN, route transport items to 0012, settle four questions first

* Good, because defects that affect defaults and requests are fixed now, in
  the smallest scope.
* Good, because ownership stays with the record that already decided each
  area.
* Bad, because the reviewer must answer Q1–Q4 before those parts of the PLAN
  run.

### Fold every finding into 0012

* Good, because it gives one record and one plan.
* Bad, because 0012 is large, is still proposed, and depends on gate G-C. The
  ranking, thinking and wizard bugs would wait on unrelated transport work.

### Fix only the confirmed bugs; record everything else as accepted

* Good, because it is the smallest change.
* Bad, because the wiring gaps (A4, A5, C2) would stay. Consumers configuring
  `WithModelMetadataURL`, a profile for `DiscoverModels`, or a longer
  `DiscoverLimit`, would keep being silently ignored.

### Do nothing now

* Good, because it costs nothing today.
* Bad, because `GenerateThinking` stays broken on two of the six static Claude
  models (B9), and A6 makes every thinking call wait on a dead metadata host.
  C1 and C4 let the wizard save wrong configurations, and two live tests stay
  red.

## More Information

### Relationship to other records

* **0009:** C1, C2, C4–C9 are defects or gaps in its wizard and matcher. C3
  is its "Related findings" item 4, left as it decided.
* **0010:** A1–A10, B1, B2 and B5 are in its ranking, metadata and
  request-side reasoning. A6's fix supersedes its PLAN §1.11 item 3, and Q2
  (a) amends its §1. Its PLAN stays `in-progress`: Phase 6's DeepSeek live
  check (D2) and Phase 7 (D3) are open.
* **0012:** it owns B3, B4, B6, B7, D5 and D6, and receives a pointer bullet.
  Its §1.4 session header was partly implemented early. B7 records what
  remains. Its §1.6 would stop `DiscoverModels` probing the open catalogs;
  A4 and A5 change the listing that path keeps.

### Evidence

* **Code:** `mcplib` at `ca29b81`. Line numbers above are at that commit.
* **Probe and regression tests** ran in throwaway copies of the tree, outside
  the repository, and were never committed. The quoted outputs are their
  failure messages. The plan carries the regression tests as diffs.
* **Live:** probes on 2026-09-26 and 2026-09-27 used `OPENCODE_API_KEY`,
  `KILO_API_KEY`, `HF_TOKEN`, `ANTHROPIC_API_KEY` and `GEMINI_API_KEY` from the
  environment. No credential value was read or recorded. OpenCode's edge
  blocks Python's default client (Cloudflare error 1010), so OpenCode probes
  used Go's HTTP client.
* **Reference sources:**
  * OpenCode's gateway billing and error paths at `opencode` `696f41bc8e`
    (`packages/console/app/src/routes/zen/util/handler.ts:480-502`,
    `:895-974`);
  * its effort mapping, at the same commit, in
    `packages/opencode/src/provider/transform.ts`: `:521` (`GEMINI_LEGACY_RE`),
    `:657-666` (`anthropicUsesModernAdaptiveThinking`), `:1789-1791` (Gemini
    `thinkingLevel`) and `:1852-1864` (`anthropicEffort`).

No source changes accompany this proposed MADR.
