---
status: accepted
date: 2026-09-26
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev, prepare-commit-msg
informed: all mcplib consumers
---
# Search Live Provider Catalogs for Primary and Fallback Model Selection

> **Revision notes (revision 2, 2026-09-25, applied in place to this same
> `proposed` document — same convention as `0001-MADR`, `0004-MADR` and
> `0008-MADR`).** Revision 1 (2026-09-24) was re-verified line by line against
> the tree at commit `55e4b31`, the live public catalogs, the provider API
> references, and the three consumer wizards. Every claim below now carries its
> evidence; the method is recorded under *More Information → Evidence*.
>
> **Facts corrected**
>
> * *"Health probing would grow with the catalog."* False. `probeGenerateHealth`
>   truncates its own input to `MaxListedModels` (`llmprovider/probe.go:17-20`),
>   so a widened `ListAvailableModels` would change *which* six models are
>   probed, not how many.
> * *"Every static catalog is asserted `<= MaxListedModels` in
>   `models_catalog_test.go`."* Partly false. Gemini, Grok and both OpenCode
>   catalogs are asserted there; Hugging Face and Kilo are asserted in
>   `huggingface_test.go:168` and `kilo_test.go:216`; `StaticOpenAI`,
>   `StaticClaude` and `StaticOpenAIChatGPT` have no count assertion.
> * *"Hugging Face: 317 offerings."* The 317 in `live_gateways_test.go:303-305`
>   counts *provider offerings*, not models. Measured on 2026-09-25: 139 models,
>   341 offerings, 94 usable. Kilo measured 394 models, 96 usable.
> * *"Three consumers implement `Prompter`."* Two do (`ptermPrompter` in
>   magictools and magicdev); prepare-commit-msg passes mcplib's own
>   `TextPrompter`.
> * *"On live failure the wizard notifies and uses static."* Only for Ollama.
>   Seven of the eight listers swallow HTTP and decode failures and return the
>   static catalog with a **nil** error, so the existing warning
>   (`wizard/configure.go:284-287`) never fires for them.
> * *"OpenAI `/v1/models` may carry a next page."* It does not: the official SDK
>   types it as a plain page with no cursor parameters. Speculative OpenAI
>   pagination is removed.
> * The `kilo-auto/*` example is a fixture case only. On 2026-09-25 the live
>   Kilo corpus contains **no** `kilo-auto/*` id at all (see Context §5).
>
> **Design gaps closed**
>
> * **One fetch, not two.** Revision 1 built a "recommended" and a
>   "searchable" corpus "from the same listing attempt" but specified two
>   exported functions, which would issue two GETs per configure run. Replaced
>   by one `ListModelCatalog*` call returning both views (§1).
> * **Live vs static is now observable** (`ModelCatalog.Live`), so search over
>   the six static ids is announced, not presented as a catalog search.
> * **Fallback `Confirm` placement** is specified: it follows a non-blank
>   search round only. Revision 1's wording would have added an unscripted
>   `Confirm` to the blank path, which `fakePrompter` rejects
>   (`wizard/fake_prompter_test.go:72-82`) and which would fail
>   `TestConfigureLLM_Fallbacks`.
> * **Filtering belongs to the listing, not the matcher.** Revision 1's
>   Confirmation asked `SearchModels` to drop a deny-listed id; it cannot see
>   Kilo's `mayTrainOnYourPrompts` flag, so that assertion moves to
>   `ListModelCatalog`.
> * **Reconfiguration keeps the prior choice.** A model picked by search is
>   outside the recommended six; without a rule, a re-run pressing Enter would
>   silently switch models (§4.3).
> * **`SearchModels` drops its `limit` parameter**; the wizard needs the total
>   match count to say "showing 20 of N".
> * **Subsequence matching runs on the id only**, not on the padded
>   `ModelLabel` text, whose annotations (`[★ Recommended: …]`) would make short
>   queries match nearly every curated id.
> * **Token separators include `:` and `~`**, which occur in live Kilo and
>   Ollama ids.
> * **Wizard fixture tests** must use a provider whose descriptor accepts a
>   base URL; Claude does not, so revision 1's `haiku`-against-a-fixture test
>   was not writable as specified.
> * **Pagination failure semantics and page bounds** are specified (§2).
>
> **Addendum (2026-09-25, found while writing
> `0009-PLAN-live-catalog-model-search.md`).** A reference implementation of §3,
> run against every Confirmation case, showed that the revision-2 case
> "`recommended` does not match via subsequence on a label" was wrong: the
> query is a plain *substring* of curated labels, so it matches by predicate 1.
> The case is replaced by `hspd`, which reaches a label only through
> subsequence. Also added: separator-only queries cannot match vacuously (§3);
> the ranking rule is stated as fuzzy-path only (globs keep input order); the
> fallback loop prompts nothing when nothing remains (§5), matching today; and
> §6 names `TestConfigureLLM_OffersEveryDescriptor`, whose stale endpoint line
> would become a search query.
>
> **Revision 3 (2026-09-25, maintainer direction: "fold in Kilo", and include
> Hugging Face).** The exact-`["text"]` input-modality rule, revision 2's
> related finding 1, is now part of this decision (§1b). It partially
> supersedes MADR 0003, which chose that rule for both gateways
> (`0003-MADR-add-gateway-llm-providers.md:806-807`, `:885-887`). The evidence,
> in Context §5, comes from Kilo's own client source and today's live catalogs.
> Kilo's client treats image input as an attachment capability, not an
> exclusion. Relaxing the rule to "input contains text" raises usable Kilo
> models from 96 to 284 and Hugging Face from 94 to 131.
>
> The maintainer also asked for an assessment of use-case-specific default
> ranking for commit-message generation. That is a separate decision about how
> the recommended six are chosen. It is recorded as out of scope here and
> proposed as [0010-MADR-use-case-aware-default-model-ranking.md](0010-MADR-use-case-aware-default-model-ranking.md). The training policy, the id deny lists, the output
> rule and the static catalogs are unchanged by this revision.
>
> **Revision 4 (2026-09-26), from [0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md).** This
> revision records four findings that bear on search. It changes no decision.
> They are *Related findings* 5-8: the retired ChatGPT catalog behind the
> ChatGPT short-circuit; the Zen per-route key headers, which search makes
> easier to reach; the OpenCode route table's gaps, which search exposes; and
> Kilo's anonymous `/models` retry, which §1's fetch step could adopt.
>
> **Revision 5 (2026-09-26, maintainer direction: "Fold into 0009").** The
> Zen/Go per-route key-header fix, related finding 6, becomes decision §1c.
> The evidence is Context §14: the server source, plus a live check with a
> bogus key. This partially supersedes MADR 0003's Bearer-only statement.

## Context and Problem Statement

`wizard.ConfigureLLM` (`wizard/configure.go:89-159`) is the canonical
configuration flow introduced by MADR 0004. After the user picks a provider and
a credential, it offers a **short, numbered menu** of models and, when
`Options.NeedFallbacks` is set, a **multi-select of the remainder**. Both menus
are drawn from the same list, and that list is deliberately capped at six.

For providers that publish large catalogs the six choices are too few, and
there is no way to search. The only way to reach any other model is to type
its exact id through the trailing `"Other (enter a model id)"` entry
(`otherModelLabel`, `wizard/configure.go:306-309`). Guessing a Hugging Face or
Kilo id is not a substitute for search.

All facts below were verified at commit `55e4b31` on 2026-09-25 unless a
different date is stated.

### 1. The menu is capped at six

`MaxListedModels = 6` (`llmprovider/models_catalog.go:9-11`) is "the hard cap
for configure-time model menus", intended to keep wizards short and avoid
"dumping dozens of unusable API IDs". The static catalogs are "the PRIMARY
source of truth for menus. Live list APIs are used only to confirm
availability" (`models_catalog.go:20-22`). `curateFromCatalog`
(`models_catalog.go:328-391`) keeps catalog hits in catalog order, backfills
from the rest of the list, and returns as soon as the output reaches the cap.

Count assertions exist for Gemini (`models_catalog_test.go:128`), Grok (`:192`),
OpenCode Zen/Go (`:340`), Hugging Face (`huggingface_test.go:168`) and Kilo
(`kilo_test.go:216`). `StaticOpenAI`, `StaticClaude` and `StaticOpenAIChatGPT`
have none; `StaticOpenAIChatGPT` is pinned to exactly three ids
(`models_catalog_test.go:213-218`).

### 2. Every lister computes the full usable list, then discards all but six

`ListAvailableModelsWithSource` (`llmprovider/discovery.go:27-63`) dispatches
to one lister per provider. Each lister builds an `available` slice of ids its
usability filter passes, then discards everything beyond the curated six:

| Lister | Usable list built at | Order of that list | Then |
|---|---|---|---|
| `listGeminiModels` | `discovery.go:99-105`, `isUsableGeminiTextModel` | API order | `curateFromCatalog(StaticGemini, …, RankGeminiModel)` `:107` |
| `listOpenAIModels` | `:148-153`, `isUsableOpenAIChatModel` | API order | `curateFromCatalog(StaticOpenAI, …)` `:155` |
| `listClaudeModels` | `:197-202`, `isUsableClaudeTextModel` | API order (newest first) | `curateFromCatalog(StaticClaude, …)` `:207` |
| `listGrokModels` | `:310-315`, `isUsableGrokModel` | API order | `curateFromCatalog(StaticGrok, …)` `:317` |
| `listOpencodeModels` | `:368-373`, `isUsableOpencodeModel` | API order | `curateFromCatalog(staticOpencodeCatalog(gateway), …)` `:375` |
| `listHuggingFaceModels` | `:443-485`: input and output modality exactly `["text"]`, `isUsableHuggingFaceModel`, at least one `live` provider | throughput desc, then first-token latency asc | `curateFromCatalog(StaticHuggingFace, …, nil)` `:488` |
| `listKiloModels` | `:583-611`: modality exactly `["text"]`, not `mayTrainOnYourPrompts`, `tools` supported, `isUsableKiloModel` | completion price asc, `-1` last | `curateFromCatalog(StaticKilo, …, nil)` `:614` |
| `listOllamaModels` | `:245-248`: every installed name | `/api/tags` order | **truncated in the lister** to `MaxListedModels` `:249-251` |

The HTTP response is already in hand. The extra ids are discarded before the
wizard sees them. Kilo already has its fetch factored out
(`fetchKiloCatalog`, `discovery.go:523-550`), shared with
`KiloModelCapabilities`; the other seven listers inline theirs.

### 3. Seven of eight listers hide listing failures

Every lister except Ollama returns `StaticModels(provider), nil` on a request
build error, transport error, non-200 status, decode error or empty curation:
for example Gemini at `discovery.go:75`, `:81`, `:86`, `:96` and `:111`, and
Kilo at `:575` with an explicit `//nolint:nilerr` recording that this is "the
established contract". `listOllamaModels` alone returns errors (`:223`, `:228`,
`:233`, `:242`), because Ollama has no static catalog: `StaticModels` returns
`nil` for it (`models_catalog.go:187-191`).

The wizard's warning, `"could not list models for %s (%v); using the built-in
catalog"` (`wizard/configure.go:284-287`), therefore fires only for Ollama, for
a missing token source, or for a token error. For the other seven a failed
listing is indistinguishable from a live one. Today that is harmless because
both yield six ids. Once the corpus is searchable it is not: a search over six
static ids presented as a catalog search is misleading.

### 4. The catalogs that need search, measured

Public listings (no credential), fetched 2026-09-25, with the in-tree filters
applied (the Python replica of the filters and a Go program calling the
in-tree `ListAvailableModels` are described under *Evidence*):

| Provider | Models listed | Usable after in-tree filters | Live curated six equal to static? |
|---|---|---|---|
| OpenCode Zen | 81 | 80 | yes |
| OpenCode Go | 42 | 38 | yes |
| Hugging Face | 139 (341 provider offerings) | 94 | no |
| Kilo | 394 | 96 | no |

For comparison, the 2026-08-29 figures recorded in-tree are Kilo 366 models
with 25 dropped by the training policy (`models_catalog.go:105`, `:593`) and
Hugging Face "253 of 317 offerings" carrying all four metadata fields
(`live_gateways_test.go:303-305`). On 2026-09-25, 28 Kilo models carry
`mayTrainOnYourPrompts`; 12 of them are text→text and are removed by that
policy rather than by an earlier filter.

Ollama installs are machine-specific and there is no static catalog. The
first-party providers (Gemini, OpenAI, Anthropic, xAI) require a credential to
list and were not measured.

A six-item menu cannot represent roughly one hundred usable ids. A
hundred-row numbered menu is no better.

### 5. The exact-text input rule excludes the Kilo managed tiers and multimodal-input models

`StaticKilo` leads with four `kilo-auto/*` managed tiers, chosen for "the
strongest churn resistance available" (`models_catalog.go:101-113`). On
2026-09-25 the live listing reports:

| Id | `input_modalities` | `mayTrainOnYourPrompts` |
|---|---|---|
| `kilo-auto/small`, `kilo-auto/efficient`, `kilo-auto/balanced` | `["text","image"]` | false |
| `kilo-auto/frontier` | `["text","image","pdf"]` | false |
| `kilo-auto/free` | `["text"]` | **true** |

`listKiloModels` requires input modalities to be *exactly* `["text"]`
(`onlyText`, `discovery.go:383-384`, applied at `:584`) and drops the training
flag (`:587`). So every managed tier is excluded, and the in-tree
`ListAvailableModels(kilo)` returned, live, `meta-llama/llama-3.1-8b-instruct,
openai/gpt-oss-20b, amazon/nova-micro-v1, mistralai/mistral-nemo,
openai/gpt-oss-120b, inclusionai/ling-3.0-flash-fin`: none of `StaticKilo`'s
tiers. Hugging Face applies the same exact-`["text"]` input rule (`:444`).
MADR 0003 chose that rule deliberately, "dropping the 40 vision-language
models" (`0003-MADR-add-gateway-llm-providers.md:806-807`, `:885-887`,
`:1286-1289`).

Kilo's own client, at `kilocode` v7.7.12
(`packages/kilo-gateway/src/api/models.ts`), does not do that:

* The only listing-time filter is tool support: a model whose
  `supported_parameters` lacks `"tools"` is skipped (`:101-105`).
* Image input only sets a capability, `attachment: supportsImages` with
  `supportsImages = inputModalities.includes("image")` (`:281`, `:295`).
  A multimodal-input model is served for text like any other.
* `mayTrainOnYourPrompts` is passed through as metadata (`:304`). Kilo's clients
  label such models "May train" and hide them only under the opt-in
  `hide_prompt_training_models` setting
  (`packages/opencode/src/kilocode/provider/model-filter.ts:3-14`).
  mcplib's stricter exclusion is its own policy (`models_catalog.go:590-598`).

Measured on 2026-09-25 with the in-tree filters otherwise unchanged:

| Rule | Kilo usable | Hugging Face usable |
|---|---|---|
| input exactly `["text"]` (today) | 96 | 94 |
| input **contains** `"text"` | 284 | 131 |

Under the relaxed rule the live Kilo curated six become `kilo-auto/small,
kilo-auto/efficient, kilo-auto/balanced, meta-llama/llama-3.1-8b-instruct,
openai/gpt-oss-20b, rekaai/reka-edge`. `kilo-auto/frontier` is also usable.
`StaticHuggingFace`'s `zai-org/GLM-5.3-Flash` (input `["text","image"]`) becomes
usable. Other facts from the same listing:

* No live Kilo model lacks `"text"` input.
* No live Kilo model omits `supported_parameters`.
* Output sets other than exactly `["text"]` are `["text","image"]` (2),
  `["image","text"]` (9) and `["text","audio"]` (4). These are image- or
  audio-generation models.
* Ten models admitted by the relaxed rule are still removed by
  `isUsableKiloModel`'s id deny list (`-vl`, `omni`, `vision`), for example
  `qwen/qwen3-vl-8b-instruct` and `qwen/qwen3.8-omni-flash`.
* Two `StaticKilo` entries, `kilo-auto/free` and
  `nvidia/nemotron-3.5-lightning:free`, now carry `mayTrainOnYourPrompts: true`.
  The training policy excludes them from every live listing. The static fallback
  still offers them.

This matters here for two reasons. A glob such as `kilo-auto/*` finds nothing in
today's live Kilo corpus. And `ListModelCatalog`'s usable view inherits whatever
the filters admit, so search does not route around them. Revision 3 therefore
folds the input rule into this decision (§1b).

### 6. No lister paginates, and two providers paginate by default

A search of `llmprovider/*.go` (non-test) for `has_more`, `nextPageToken`,
`after_id`, `pageToken` and `last_id` finds nothing. Each lister issues one GET
with no query string (`discovery.go:72`, `:123`, `:170`, `:221`, `:285`, `:341`,
`:400`, `:528`) and decodes one body.

| Provider | Listing | Documented pagination | Consequence of one GET |
|---|---|---|---|
| Gemini | `GET /v1beta/models` | `pageSize` (default **50**, max 1000), `pageToken`; response `nextPageToken`, "if this field is omitted, there are no more pages" | only the first 50 models are seen |
| Anthropic | `GET /v1/models` | `limit` (default **20**, 1–1000), `after_id`; response `has_more`, `last_id`; "more recently released models are listed first" | only the 20 newest models are seen; older curated ids such as `claude-3-5-haiku-latest` can fall off page 1 |
| OpenAI | `GET /v1/models` | none: the official Python SDK's `models.list()` takes no `limit`/`after` and returns a non-cursor `SyncPage` | complete |
| xAI | `GET /v1/models` | none documented; the official SDK's `list_language_models()` sends an empty request with no page token | complete as far as documented (an assumption, see *Evidence*) |
| OpenCode Zen/Go, Hugging Face, Kilo | `GET {base}/models` | none; live bodies carry only `data` (and `object`) at top level | complete (probed 2026-09-25) |
| Ollama | `GET /api/tags` | none documented | complete |

The Gemini and Anthropic truncation already affects `ListAvailableModels`
today: curation can only find catalog ids on the first page.

### 7. Primary and fallback selection share the truncated list

`ConfigureLLM` calls `discoverModels` once (`wizard/configure.go:132`). If the
result is empty (Ollama with nothing installed) it asks for a model id
directly (`:133-147`). Otherwise `selectModel` (`:311-334`) builds
`modelChoices(models)` plus Other, pre-selects `o.Existing.Model` if present
(regardless of `o.Existing.Provider`), and calls `Select`. Choosing Other calls
`Input("Model id", o.Existing.Model)`. `selectFallbacks` (`:337-358`) removes the
primary and calls `MultiSelect` on the rest with no preselection. A fallback
can only be one of the other five curated ids.

### 8. Numbering already exists in every renderer

`TextPrompter.renderChoices` prints `  %d) %s` with 1-based indices
(`wizard/text_prompter.go:128-137`). `Select` accepts `1..N` (`:140-168`),
`MultiSelect` a comma-separated list of the same (`:171-204`), and `readLine`
trims surrounding whitespace only (`:100-113`), so a query such as `llama 8b`
survives. On EOF, `Input` returns its default (`:231-247`).

Both pterm consumers render `"%d. %s"` entries through
`pterm.DefaultInteractiveSelect` / `DefaultInteractiveMultiselect` with
`WithMaxHeight(12)` (magictools `cmd/mcp-server-magictools/pterm_prompter.go:26`,
`:60-79`). pterm v0.12.83 enables type-to-filter on both widgets by default
(`Filter: true`, `interactive_select_printer.go:27`,
`interactive_multiselect_printer.go:27`). The missing piece is the **corpus and
the query**, not the digits.

### 9. `Prompter` is a stable public seam with three callers

`Prompter` has six methods: `Select`, `MultiSelect`, `Confirm`, `Input`,
`Secret`, `Notify` (`wizard/prompter.go:55-79`). The package documentation
calls it "deliberately small and stable … changing it later is a breaking
change" (`:50-54`). Compile-time assertions pin the implementations:
`text_prompter.go:52`, `text_prompter_test.go:202-203`,
`fake_prompter_test.go:115`, and `var _ wizard.Prompter = ptermPrompter{}` in
each pterm consumer.

On this workstation, 13 `go.mod` files require `mcplib` (7 at v1.2.0, 5 at
v1.4.1, 1 at v1.5.0). Three call `wizard.ConfigureLLM`, and all three pass
`Discover: true`:

| Consumer | `Prompter` | `NeedFallbacks` | mcplib pin |
|---|---|---|---|
| mcp-server-magictools (`config.go:497-505`) | own `ptermPrompter` | per tier | v1.4.1 |
| mcp-server-magicdev (`configure.go:274-303`) | own `ptermPrompter` | no | v1.2.0 |
| prepare-commit-msg (`internal/ui/setup.go:213-219`) | mcplib `TextPrompter` | yes | v1.5.0 |

prepare-commit-msg clamps fallbacks to `MaxFallbacks = 3`
(`internal/config/config.go:21`, `ClampFallbacks` `:165-180`); mcplib imposes no
limit.

### 10. Discovery is opt-in, bounded, and fixture-injectable only for some providers

`Options.Discover` defaults to false (`wizard/configure.go:48-50`; "the zero
value runs a full interactive configuration … with no discovery", `:35-36`).
When true, `discoverModels` (`:257-291`) wraps the call in
`DiscoverLimit` (default `20s`, `:15`) around the lister's own hard 10s
timeout (`discovery.go:30`). ChatGPT OAuth is special-cased twice: the wizard's
static fallback is `StaticOpenAIChatGPT` (`configure.go:265-268`), and
`ListAvailableModelsWithSource` short-circuits to it without HTTP
(`discovery.go:32-34`; pinned by `TestListAvailableModelsWithSource_ChatGPTDoesNotHTTP`).

The wizard forwards `WithBaseURL` only when `resolveBaseURL` produced one
(`configure.go:279-282`), and `resolveBaseURL` prompts only when
`d.SupportsBaseURL` (`:187`). Only OpenCode Zen, OpenCode Go, Hugging Face,
Kilo and Ollama set it (`llmprovider/descriptor.go:147-167`). A wizard test can
therefore point a live listing at an `httptest` fixture for those five
providers only, by scripting the endpoint `Input` first.

### 11. Health probing caps itself

`probeGenerateHealth` truncates candidates to `MaxListedModels` before firing
one generate per candidate (`llmprovider/probe.go:10-20`). It is called by each
provider's `DiscoverModels` with the curated listing (for example
`gemini.go:297-317`, `grok.go:239-250`, `openai.go:204-219`). Its cost does not
grow with the catalog. What it probes is whatever list it is handed, so its
input must remain the curated recommendation rather than an API-ordered
dump.

### 12. `mcplib` has no fuzzy-search dependency

Direct requirements in `go.mod:5-11` are `invopop/jsonschema`,
`modelcontextprotocol/go-sdk`, `golang.org/x/mod`, `golang.org/x/sys` and
`golang.org/x/term`. MADR 0004 rejected moving `pterm` into `mcplib` and made
"`mcplib` must stay dependency-light" a decision driver. The usable corpora in
§4 are about one hundred short strings; the longest live id is 56 characters.
No live id in the four public catalogs contains `*` or `?`. Their punctuation
is `-`, `.`, `/`, `:` and `~`.

### 13. How tests drive the flow

`fakePrompter` (`wizard/fake_prompter_test.go`) returns its default for an
unscripted `Input` (`:84-95`) but **errors** on an unscripted `Select`,
`MultiSelect` or `Confirm` (`:47-49`, `:64-66`, `:76-78`). Scripted inputs are
consumed in order. `TestConfigureLLM_OtherModelEscapeHatch`
(`configure_test.go:263-284`) scripts `inputs: {"my-custom-model"}`, so its first
`Input` is the manual id. `TestConfigureLLM_Fallbacks` (`:165-185`) scripts no
`Input` and no `Confirm`.

Consumers differ:

* magicdev's `recordingPrompter` returns the default for `Input`, `Confirm` and
  `Select` (`pterm_prompter_test.go:51-68`), so any added prompt with a blank
  default is transparent to it.
* prepare-commit-msg feeds `TextPrompter` a literal stdin script
  (`internal/ui/setup_test.go:48`, `:79`, `:119`, `:144`, `:194`, `:230`), one line
  per prompt. Any added prompt consumes the next line.

`TestConfigureLLM_EmptyDiscoveryFallsBackToStatic` (`configure_test.go:143-162`)
is described as discovering "against an unreachable base URL". It sets no base
URL, and Claude does not support one, so the listing is a real
`GET https://api.anthropic.com/v1/models` with a fake key. It passes because a
401 or a dial error both degrade to static. See *Related findings*.

### 14. Zen and Go read the key from a different header per route (revision 5)

The Zen/Go server takes the key from a route-specific header. At `opencode`
`696f41bc8e`, in `packages/console/app/src/`:

| Route | `parseApiKey` | Cited at |
|---|---|---|
| `/messages` (Zen and Go) | `headers.get("x-api-key")` | `routes/zen/v1/messages.ts:9`, `routes/zen/go/v1/messages.ts:9` |
| Google `…/models/{model}:generateContent` | `headers.get("x-goog-api-key")` | `routes/zen/v1/models/[model].ts:9` |
| `/chat/completions`, `/responses` | `headers.get("authorization")?.split(" ")[1]` | `routes/zen/v1/chat/completions.ts:9` |

The inference proxy picks the header the same way
(`lib/inference-proxy.ts:39-43`). No route falls back to another header.

`mcplib` sends `Authorization: Bearer` on every route. The comment reads:
"The gateway accepts only Authorization: Bearer on every route — it ignores
x-api-key and x-goog-api-key" (`llmprovider/opencode.go:258-262`).
`TestOpencode_KeyInHeader` forbids the vendor headers
(`llmprovider/opencode_test.go:107-140`). MADR 0003's evidence for this was a
probe of `POST /zen/v1/responses` only (`0003-MADR-add-gateway-llm-providers.md:126-137`,
at `55e4b31`).

A live check on 2026-09-26, with the bogus key `sk-bogus-000` and no other
credential:

| Request | Response |
|---|---|
| `/zen/v1/messages` with `Authorization: Bearer` | `401 AuthError "Missing API key."` |
| `/zen/v1/messages` with `x-api-key` | `401 AuthError "Invalid API key."` |
| Google route with `Authorization: Bearer` | `401 AuthError "Missing API key."` |
| Google route with `x-goog-api-key` | `401 AuthError "Invalid API key."` |

So every Anthropic- or Google-shaped Zen/Go model fails today with "Missing API
key.", including three `StaticOpencodeZen` defaults: `claude-haiku-4-5`,
`gemini-3.7-flash` and `gemini-3.5-flash-lite`. A free model on those routes
would run anonymously (`handler.ts:103-104`). Search (§3–§5) makes every such
model selectable, so the defect's reach grows with this decision
([0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md) O1).

### The requirement, restated against that evidence

A user configuring a provider must be able to **type a search string**, have it
run against the **provider's live usable catalog** (the list the listers
already compute), see **numbered matches**, and pick one. The same interaction
must be available when choosing **fallback** models. A query containing glob
metacharacters takes a cheaper, exact path. Other queries must tolerate the
inexact forms people actually type (`flash`, `llama 8b`, `sonet`). The user must
be told when the corpus is the built-in catalog rather than a live listing.

The Other hatch stays: a live listing can still lag a brand-new id, which is
why `TestConfigureLLM_OtherModelEscapeHatch` exists.

## Decision Drivers

* **The live catalog is the search corpus.** The curated six remain the
  *recommended* list, not the only list a user can reach.
* **Usability filters stay on.** Embeddings, TTS, image or audio *generation*,
  dated Gemini previews and Kilo's `mayTrainOnYourPrompts` models must not
  become searchable because the cap comes off. The listing filters decide what
  is searchable; search only ranks what they admit.
* **A text model is one that reads text and writes text** (revision 3). Also
  accepting images or PDFs does not stop a model serving a text prompt. Kilo's
  own client treats that as an attachment capability (Context §5).
* **`ListAvailableModels` keeps its contract.** Same signature, same
  `<= MaxListedModels` result, same degrade-to-static-with-nil-error behaviour.
  Its curated content changes only for Kilo and Hugging Face, and only through
  §1b. The probe paths and every existing test that does not assert the old
  input rule depend on it.
* **One listing per configure run.** Search must not add a second GET.
* **Truthful about the corpus.** A search over static ids is announced as
  such.
* **`Prompter` does not grow.** Search has to reach all three wizards on an
  `mcplib` upgrade without each consumer implementing a new method.
* **`mcplib` stays dependency-light.** A matcher over about a hundred ids does
  not justify a new module.
* **Primary and fallbacks are the same interaction.** `NeedFallbacks` must
  search, not multi-select the leftovers of the recommended six.
* **Blank input preserves today's menus** as far as a script allows, and a
  reconfiguration that presses Enter keeps the model it had.
* **Testable without a TTY or a network.** The matcher is a pure function; the
  flow runs under `fakePrompter`; listings run against `httptest` fixtures.

## Considered Options

* Model catalog listing in `llmprovider` plus a search-then-select flow in `wizard`, `Prompter` unchanged
* Extend `Prompter` with `SearchSelect` / `SearchMultiSelect` for live typeahead
* Raise `MaxListedModels` or pass the full usable list into the existing `Select`
* Add a third-party fuzzy library (`sahilm/fuzzy` or equivalent)
* Widen `ListAvailableModels` itself to return the uncapped usable list
* Leave search to each consumer wizard

## Decision Outcome

Chosen option: **"Model catalog listing in `llmprovider` plus a
search-then-select flow in `wizard`, `Prompter` unchanged"**. It is the only
option that puts the live usable catalog behind a query, fetches it once,
numbers the hits with the renderer each consumer already has, covers primary
and fallbacks, and leaves MADR 0004's seams and `ListAvailableModels`'s
contract intact.

### 1. One listing, two views: `ListModelCatalog`

Add to `llmprovider/discovery.go`:

```go
// ModelCatalog is the result of one listing attempt, viewed two ways.
type ModelCatalog struct {
    // Recommended is exactly what ListAvailableModels returns: at most
    // MaxListedModels ids, curated against the static catalog.
    Recommended []string
    // Usable is every id the provider's usability filters admit, in the
    // lister's order, uncapped. Equal to Recommended when Live is false.
    Usable []string
    // Live reports whether Usable came from the provider's listing. False
    // means the static catalog was substituted.
    Live bool
}

func ListModelCatalog(ctx context.Context, providerName, apiKey string, opts ...ProviderOption) (ModelCatalog, error)
func ListModelCatalogWithSource(ctx context.Context, providerName string, src TokenSource, opts ...ProviderOption) (ModelCatalog, error)
```

Contract:

* Same providers, token handling, 10s hard timeout and ChatGPT short-circuit
  as `ListAvailableModelsWithSource`. For ChatGPT OAuth,
  `Recommended == Usable == StaticOpenAIChatGPT` and `Live == false`.
* **Errors are unchanged.** Where a lister returns static with a nil error
  today, `ListModelCatalog` returns `{Recommended: static, Usable: static,
  Live: false}` with a nil error. Where it errors today (Ollama unreachable or
  malformed, unknown provider, unknown OpenCode gateway, missing token source,
  token failure), it returns the same error.
* `Live` is true iff the listing request(s) succeeded and their result was
  used rather than the static catalog. For the seven providers with a static
  catalog, a listing whose filtered result is empty substitutes static, as
  today, so `Live` is false. Ollama has no static catalog: a successful
  listing with nothing installed is `Live: true` with both views empty.
* **Invariant:** `len(Recommended) == 0` iff `len(Usable) == 0`, and every
  element of `Recommended` is in `Usable`, compared case-insensitively.
  `curateFromCatalog` returns only elements of its `available` input, and it
  is non-empty whenever that input has a usable element.
* `Usable` order is the lister's order (§2 of Context): API order for
  Gemini/OpenAI/Anthropic/xAI/OpenCode, metadata rank for Hugging Face and
  Kilo, `/api/tags` order for Ollama.

Implementation shape: each lister is split into a *fetch-and-filter* step
returning `(usable []string, live bool, err error)` and a *curate* step
(`curateFromCatalog` with the provider's catalog, usability function and rank,
or, for Ollama, truncation to `MaxListedModels`). Kilo's `fetchKiloCatalog` is
the precedent.

`ListAvailableModels` and `ListAvailableModelsWithSource` keep their signatures
and become `ListModelCatalog*(…).Recommended`. The unexported `list*Models`
functions called directly by each provider's `DiscoverModels` keep returning
the curated list, so `probeGenerateHealth`'s input is unchanged.

### 1b. Input modality: "contains text" for Kilo and Hugging Face (revision 3)

In the Kilo and Hugging Face fetch-and-filter steps, a model qualifies on
modality when its `architecture.input_modalities` **contains** `"text"` and its
`architecture.output_modalities` is **exactly** `["text"]`. This replaces the
exact-`["text"]` input rule that MADR 0003 chose, for these two providers only.

Unchanged:

* **Output stays exactly `["text"]`.** `wizard` configures text generation.
  Models that also output images or audio are generation-oriented (Context
  §5), and Kilo lists them through a separate path (`fetchKiloImageModels`,
  `models.ts:140-157`).
* **Kilo still requires `"tools"`** in `supported_parameters`. Kilo's client
  treats a missing array as tool-capable. No live model omits the array, so
  staying strict costs nothing today, and an unknown capability stays excluded.
* **The training policy stays** (`mayTrainOnYourPrompts` excluded), for the
  reason `models_catalog.go:590-598` records: commit hooks send private diffs.
* **The id deny lists stay** (`isUsableKiloModel`, `isUsableHuggingFaceModel`).
  They still remove ten relaxed-eligible Kilo ids named `-vl`/`omni`/`vision`.
  Revising them belongs to the default-ranking decision, not this one.
* **The static catalogs stay.** `StaticKilo`'s two training-flagged entries and
  the 8B entries in `StaticKilo`/`StaticHuggingFace` are inputs to the proposed
  default-ranking decision (*More Information → Out of scope*).

### 1c. Zen/Go key header per route (revision 5)

The OpenCode provider sets exactly one key header per request, chosen by the
model's route:

| Route | Header |
|---|---|
| `messages` | `x-api-key: <key>` |
| `google` | `x-goog-api-key: <key>` |
| `responses`, `chat_completions` | `Authorization: Bearer <key>` |

This matches both the server (Context §14) and OpenCode's own client, which
uses each route's vendor SDK and header. The key still never enters the URL.

The rule lives in one unexported helper keyed on `OpencodeRoute`, so a route
added later must choose its header explicitly. The `/models` listing keeps
`Authorization: Bearer`, which that endpoint reads. `TestOpencode_KeyInHeader`
is reversed to assert the per-route header and the absence of the other two.

This partially supersedes MADR 0003's "Bearer on every route" statement
(`0003-MADR-add-gateway-llm-providers.md:126-137`, at `55e4b31`).

### 2. Pagination for Gemini and Anthropic

Pagination lives in the shared fetch, so `ListAvailableModels` benefits too.

* **Gemini:** request `pageSize=1000`; while the response carries a non-empty
  `nextPageToken`, request again with `pageToken`.
* **Anthropic:** request `limit=1000`; while `has_more` is true, request again
  with `after_id=<last_id>`.
* **Bound:** at most 10 pages per listing (10,000 models at the maximum page
  size), all inside the existing 10s listing timeout.
* **Failure:** if any page fails (transport, non-200, decode), or the page
  bound is reached with more pages pending, the whole listing is treated as
  failed and degrades exactly as a first-page failure does today (static,
  nil error, `Live: false`). A partial corpus is never presented as live.

OpenAI, xAI, OpenCode, Hugging Face, Kilo and Ollama keep one GET (Context §6).
If a provider later documents pagination, it is added in its fetch step under
the same bound and failure rules.

### 3. Matcher: `SearchModels` in `llmprovider`

A pure function with no I/O:

```go
type ModelMatch struct {
    ID    string
    Label string // ModelLabel(provider, ID)
    Score int    // higher is better; comparable only within one call
}

// SearchModels ranks models against query. It does not filter for
// usability: models is expected to be a ModelCatalog view.
func SearchModels(provider string, models []string, query string) []ModelMatch
```

* **Surface.** Each candidate is matched on its id and on
  `ModelLabel(provider, id)` (`models_catalog.go:685-691`), which is the bare id
  for anything outside the curated label table.
* **No filtering.** Whatever is in `models` is searchable. Usability is the
  listing's responsibility (§1), because the Kilo training flag is not
  recoverable from an id.
* **Normalisation.** Trim, then lowercase. Tokens split on runs of
  `[-_/.:~]` and whitespace, so `llama 8b`, `llama-8b` and `Llama-3.1-8B` share
  tokens and `llama3:latest` yields `llama3`, `latest`.
* **Empty query** returns nil. The wizard treats blank as "show recommended",
  which is a flow decision, not a matcher decision.
* **Glob path.** If the query contains `*` or `?`, it is compiled once into a
  case-insensitive, fully anchored regexp: `*` becomes `.*`, `?` becomes `.`,
  and every other character is `regexp.QuoteMeta`-escaped. It is matched
  against the whole id and the whole label, so `*` matches `/`. Fuzzy scoring
  does not run. There is no escape syntax, because no id contains `*` or `?`
  (Context §12). `path.Match` and `filepath.Match` are rejected: their `*` does
  not cross `/`, so `*llama*` would miss `meta-llama/Llama-3.1-8B-Instruct`.
  Glob matches share one score and keep input order.
* **Fuzzy path** (no `*` or `?`). A candidate matches if any of these holds:
  1. the query is a substring of the id or of the label;
  2. every query token is a prefix of some id token or label token, in any
     order;
  3. the query, with separators removed, is an in-order subsequence of the id
     with separators removed.

  Subsequence runs on the **id only**. Curated labels carry annotation text
  such as `[★ Recommended: high speed, low latency]` (`models_catalog.go:642-683`),
  and a subsequence over that text would admit almost any short query.
  Candidates meeting none of the three are dropped.
  Predicate 2 requires at least one query token and predicate 3 at least one
  non-separator character, so a query made only of separators cannot match
  vacuously.
* **Ranking (fuzzy path).** Scores are tiered so that a stronger kind of match always
  outranks a weaker one: exact id equality, then id prefix, then substring of
  the id, then substring of the label, then all-tokens-prefix, then
  subsequence. Within the subsequence tier, contiguous runs and matches at
  token starts score higher. Ties sort by shorter id, then lexicographic id.
  Input order is stable within equal keys.

A separate edit-distance pass is not included. Subsequence already accepts
`sonet` for `sonnet` and `gemni` for `gemini`, and edit distance would dominate
the cost of the glob path the requirement asks to keep cheap.

### 4. Wizard: primary model

`discoverModels` returns a `ModelCatalog`:

* `Discover == false`: `{Recommended: static, Usable: static, Live: false}`,
  where static is `d.StaticModels`, or `StaticOpenAIChatGPT` for ChatGPT OAuth
  (today's `configure.go:265-271`). No network, no notice.
* `Discover == true`: `ListModelCatalogWithSource` under the existing
  `DiscoverLimit` and `WithBaseURL` handling. On error, today's warning and the
  static catalog. On `Live == false` without an error, except for ChatGPT OAuth
  (which has no listing to fail), `Notify(LevelInfo, "live model listing for %s
  is unavailable; search covers the built-in catalog only", d.Label)`.

The empty-catalog path (`configure.go:133-147`) is unchanged and keys on
`len(Recommended) == 0`, which by the §1 invariant implies `Usable` is empty.

`selectModel` becomes a loop. `Prompter` is not changed.

1. `q := Input("Search models (blank for recommended)", "")`.
2. **Blank** shows today's menu: `Select("Choose a %s model:", …)` over
   `modelChoices(Recommended)`, then the current-model row when §4.3 applies,
   then Other last. The default is the index of `o.Existing.Model` if it is
   listed, else 0. Other keeps today's `Input("Model id", o.Existing.Model)`.
3. **Current model.** If `o.Existing.Provider == d.ID`, `o.Existing.Model` is
   non-empty, and it is not in `Recommended`, it is listed after the
   recommended ids, before Other, as
   `Choice{Label: ModelLabel(d.ID, id), Detail: "current"}`, and it is the
   default. Recommended indices therefore never move; Other moves down one
   row only when the current model is shown.
4. **Non-blank:** `m := SearchModels(d.ID, Usable, q)`.
   * No matches: `Notify(LevelWarn, "no %s models match %q", d.Label, q)` and
     return to step 1.
   * More than `maxSearchResults` (20) matches:
     `Notify(LevelInfo, "showing %d of %d matches; refine the search to narrow them", 20, len(m))`.
   * `Select("Choose a %s model:", …)` over the first 20 matches, then
     `"Search again"`, then Other. The default is 0. Search again returns to
     step 1. Other takes the manual-id path as in step 2.

Blank input ends the loop, and `TextPrompter.Input` returns its blank default
on EOF (Context §8), so a script that runs out cannot loop forever.

### 5. Wizard: fallbacks

When `NeedFallbacks` is true, `selectFallbacks` loops over the same catalog.
The primary and every fallback already chosen are excluded from both views
before each round. If both remaining views are empty, the loop ends without a
prompt, as today's `selectFallbacks` returns before prompting when nothing
remains (`configure.go:344-346`).

1. `q := Input("Search fallback models (blank for recommended)", "")`.
2. **Blank** runs today's step and ends the loop: if any recommended ids
   remain, `MultiSelect("Choose fallback models (optional):", …)` over them with
   no preselection. An empty selection adds nothing. **No `Confirm` follows.**
3. **Non-blank:** `SearchModels` over the remaining usable ids. No matches
   gives the same warning as §4 and returns to step 1. Otherwise, after the
   same truncation notice, `MultiSelect("Choose fallback models:", …)` over the
   first 20. Selected ids are appended in the order chosen.
4. After a non-blank round only, `Confirm("Search for more fallback models?",
   false)`. Yes returns to step 1; No ends the loop.

The Other hatch is not added to the fallback step. A fallback that the live
catalog does not list is rare. The user can re-run configuration, and a
consumer can accept free-text fallbacks itself (prepare-commit-msg already
takes `--fallback`). mcplib imposes no fallback count. Consumers that clamp,
such as prepare-commit-msg with `MaxFallbacks = 3`, keep doing so.

### 6. Prompt-sequence impact

| Scenario | Before | After |
|---|---|---|
| Pick recommended model *i* | `Select(i)` | `Input("")`, `Select(i)` |
| Pick Other | `Select(len)`, `Input(id)` | `Input("")`, `Select(len)`, `Input(id)` |
| Search and pick | not possible | `Input(q)`, `Select(j)` |
| Fallbacks, recommended | `MultiSelect(ix)` | `Input("")`, `MultiSelect(ix)` |
| Fallbacks, searched | not possible | `Input(q)`, `MultiSelect(ix)`, `Confirm(false)` |

Under `fakePrompter` and magicdev's `recordingPrompter`, an unscripted `Input`
returns `""`, so every "before" script except the Other hatch keeps working.
One passes by accident: `TestConfigureLLM_OffersEveryDescriptor`
(`configure_test.go:217-236`) scripts an endpoint `Input` for Gemini, which has
no endpoint prompt (`llmprovider/descriptor.go:72`). That leftover line would
become a search query, match nothing, and loop back to a blank search, so the
PLAN removes it. A
stdin-scripted `TextPrompter`, as in prepare-commit-msg's tests, needs one
blank line inserted before the model selection and, with `NeedFallbacks`, one
before the fallback selection.

### 7. What does not change

* `Prompter`, `Choice`, `Level`, `TextPrompter`'s numbering and parsing,
  `Result`, and every `Options` field.
* `ListAvailableModels*` signatures and results, `MaxListedModels`,
  `curateFromCatalog`, `probeGenerateHealth`, and each provider's
  `DiscoverModels`.
* `ModelLabel`, the static catalogs, every id deny filter, the output-modality
  rule, the Kilo tools rule and the Kilo training policy. The one filter change
  is §1b's input-modality rule for Kilo and Hugging Face.
* `otherModelLabel` as the last entry of every primary model menu.
* Orchestrated processes still return `ErrOrchestrated` before any prompt.

### Consequences

* Good, because Hugging Face (94 usable), Kilo (96), OpenCode Zen (80) and Go
  (38), and a well-stocked Ollama host become searchable instead of six
  entries plus a typed id.
* Good, because one GET (or one paginated sequence) serves both the
  recommended menu and the search corpus. Configure runs make no extra
  listing calls.
* Good, because Gemini and Anthropic listings become complete, which also fixes
  curation against catalog ids beyond page 1 in `ListAvailableModels` today.
* Good, because `ListAvailableModels` and the probe paths keep their six-item
  input, so no existing caller becomes a hundred-row menu.
* Good, because `Live` makes a static fallback visible for the first time.
  Today seven of eight listers hide it.
* Good, because every `Prompter` gains the feature on an `mcplib` bump: the flow
  uses `Input`, `Select`, `MultiSelect`, `Confirm` and `Notify` only.
* Good, because glob queries skip fuzzy scoring and treat `/` as an ordinary
  character, which is what `*llama*` and `vendor/*` need.
* Good, because a blank search reproduces today's menus, and a reconfiguration
  defaults to the model it already had.
* Good, because the matcher is pure and the listings are fixture-driven, so
  every new check can be made to fail on demand without a TTY or a network.
* Neutral, because `Options.Discover` still gates the network. A caller that
  leaves it false searches the static catalog. All three wizards set it
  (Context §9).
* Neutral, because non-curated matches display the bare id.
  `modelLabels` does not cover the open catalogs.
* Neutral, because pterm users filter twice: the search query, then pterm's own
  type-to-filter on the result menu (Context §8). The second filter is
  optional.
* Bad, because the primary flow gains an `Input` before `Select`. A script
  whose first `Input` is the Other hatch's model id has that id consumed as a
  search query: `TestConfigureLLM_OtherModelEscapeHatch` must script `""`
  first. prepare-commit-msg's stdin-scripted tests need a blank line before
  each model and fallback selection when it bumps to this release.
* Bad, because a stale script fails quietly. A line meant for `Select`, read
  as a query, runs a search rather than raising an error: `7` fuzzy-matches
  ids containing `7`. Consumer script updates are therefore required, not
  optional, on upgrade.
* Bad, because the 20-row display cap can hide a relevant match. The
  "showing 20 of N" notice and "Search again" are the mitigation.
* Bad, because subsequence matching is permissive on one- and two-character
  queries. Tiered ranking, the id-only restriction and the display cap limit
  the damage; a one-character query still yields up to 20 rows.
* Good, because §1c fixes authentication for every Anthropic- and
  Google-shaped Zen/Go model, including three `StaticOpencodeZen` defaults.
  They fail today with "Missing API key." (Context §14).
* Good, because §1b restores the four paid Kilo managed tiers, and
  `StaticHuggingFace`'s `zai-org/GLM-5.3-Flash`, to live menus and search. Usable
  Kilo rises from 96 to 284 and Hugging Face from 94 to 131 (Context §5).
* Neutral, because §1b changes the live curated six for Kilo and Hugging Face.
  That is the purpose of the change, but a user who re-runs configuration sees
  different recommendations for those two providers.
* Bad, because §1b fixes *which* models are admitted, not how the six are
  *ranked*. The relaxed Kilo six still include
  `meta-llama/llama-3.1-8b-instruct`, an 8B model without reasoning, because
  Kilo's backfill ranks by lowest price. Ranking for the commit-message use
  case is the proposed default-ranking decision.
* Bad, because §1b partially supersedes an accepted decision (MADR 0003's
  input filter). The two existing curation tests that pin the old rule must be
  rewritten, not merely extended.

### Confirmation

Compliance is confirmed by tests that have each been **seen to fail** on a
deliberately broken input (run against a scratch copy, never by dirtying the
tree) before they are relied on:

* **Contract unchanged.** Every existing `TestListAvailableModels_*`,
  `TestList*Models_*`, `TestCurateFromCatalog_*` and `probe_test.go` case passes
  unmodified, including `TestListAvailableModels_OpencodeZen`'s
  `1..MaxListedModels` bound (`discovery_test.go:291`).
* **Equivalence.** For every provider fixture, `ListModelCatalog(…).Recommended`
  equals `ListAvailableModels(…)`. Planted failure: a `Recommended` that skips
  curation.
* **Uncapped usable view.** Hugging Face, Kilo and OpenCode fixtures with more
  than `MaxListedModels` usable entries return all of them in `Usable`.
  Planted failure: truncating `Usable` to six.
* **Filters still apply.** A Kilo fixture entry with `mayTrainOnYourPrompts:
  true`, a non-text Hugging Face entry, and a Gemini `embedding-001` /
  `-preview-` entry are absent from `Usable`. Planted failure: bypassing the
  Kilo policy check.
* **Key header per route (§1c).**
  * For a model on each of the four routes, the request carries exactly the
    header §1c names, neither of the other two, and no key in the URL.
  * *Planted failure:* the helper returning `Authorization` for every route.
  * An opt-in live test (`live_gateways` build tag) repeats Context §14's
    bogus-key check. It pins "Missing API key." for the wrong header and
    "Invalid API key." for the right one on `/messages` and the Google
    route, so drift on the server is detected.
* **Input modality (§1b).** In the Kilo and Hugging Face fixtures:
  * a text+image-input, text-output entry (`org/vlm`) is admitted and ranked
    in its metadata order;
  * a text-input, text+image-output entry is rejected;
  * an audio-only-input entry is rejected.

  The two existing curation tests, `TestListHuggingFaceModels_MetadataCuration`
  and `TestListKiloModels_MetadataCuration`, which assert that `org/vlm` is
  dropped, are rewritten to the new rule with exact expected orders. Planted
  failure: restoring the exact-`["text"]` input check.
* **Ollama.** A `/api/tags` fixture with eight names yields eight in `Usable`
  and six in `Recommended`. Planted failure: truncating before the split.
* **Live flag.** A non-200 fixture yields `Live == false` with
  `Usable == Recommended == static` and a nil error; a good fixture yields
  `Live == true`. ChatGPT OAuth yields `Live == false` with no HTTP request.
* **Pagination.** A Gemini fixture splitting usable ids across two pages
  (`nextPageToken`) and an Anthropic fixture (`has_more`/`last_id` →
  `after_id`) each return both pages. A fixture whose second page is non-200
  degrades to static with `Live == false`. A fixture that never stops paging
  terminates at the bound. Planted failure: a fetch that stops after page one
  must fail the two-page tests.
* **`SearchModels`.**
  * glob `kilo-auto/*` over a fixture list matches `kilo-auto/balanced` and
    not `kilo-auto-legacy` or `meta-llama/kilo-auto`;
  * glob `*llama*` matches `meta-llama/Llama-3.1-8B-Instruct` (the `path.Match`
    counterexample);
  * glob `gpt-4*` matches `gpt-4.1-mini` and does not match `chatgpt-4o-latest`,
    which the substring predicate would admit (planted failure: an unanchored
    glob regexp);
  * `*llama*` returns results at all (planted failure: routing glob queries
    to the fuzzy path, where a literal `*` matches nothing);
  * `flash` ranks `gemini-3.5-flash` above any subsequence-only hit;
  * `llama 8b` matches `meta-llama/Llama-3.1-8B-Instruct`;
  * `sonet` matches `claude-sonnet-5`;
  * `hspd` does not match `claude-haiku-4-5`, although it is an in-order
    subsequence of that id's label (`…high speed…`) and of nothing in the id
    (planted failure: subsequence over labels);
  * `llama3 latest` matches `llama3:latest`;
  * empty and whitespace-only queries return nil;
  * equal-score ties order by shorter id, then lexicographically.
* **Wizard, via `fakePrompter`.**
  * Blank search then index 0 returns `Recommended[0]`; existing Select-only
    tests pass unmodified.
  * Against an OpenCode Zen fixture (endpoint scripted as the first `Input`) that
    lists a usable id absent from `StaticOpencodeZen`, a query matching it plus
    index 0 returns that id. Planted failure: searching `Recommended` instead
    of `Usable`.
  * Other still returns a typed id, with `""` scripted first.
  * No matches, then a blank search, reaches the recommended menu.
  * More than 20 matches renders exactly 20 match rows plus Search again and
    Other, and notifies the total.
  * `Existing{Provider: d.ID, Model: x}` with `x` outside `Recommended` renders
    `x` after the recommended rows with `Detail: "current"` as the default;
    with a different `Existing.Provider` it does not appear.
  * `Live == false` under `Discover: true` notifies once; `Discover: false`
    does not.
  * `TestConfigureLLM_Fallbacks` passes unmodified. A searched fallback round
    records ids outside the recommended six, excludes the primary, and asks
    `Confirm` exactly once per non-blank round.
* **Seam unchanged.** `var _ Prompter = (*TextPrompter)(nil)` and the fake's
  assertion still compile, with no new `Prompter` method.
* **No network in unit tests.** Every new wizard test that sets
  `Discover: true` uses an `httptest` fixture or the ChatGPT short-circuit.

## Pros and Cons of the Options

### Model catalog listing in `llmprovider` plus a search-then-select flow in `wizard`, `Prompter` unchanged

The chosen option. Listing and matching live next to the catalogs they serve,
the wizard owns the loop, and numbering stays in the renderer.

* Good, because it delivers live-catalog search, glob, fuzzy matching,
  numbered picks and fallbacks without a breaking `Prompter` change.
* Good, because one listing call feeds both views, and `ListAvailableModels`
  remains safe for the probe paths and any short-menu caller.
* Good, because the glob path compiles once and scans about a hundred ids in
  process.
* Neutral, because `TextPrompter` search is query-then-list, not filter-as-you-
  type. That is the cost of not growing `Prompter`.
* Bad, because scripted consumer tests need an extra blank line per selection
  step, and a stale script fails quietly rather than loudly.

### Extend `Prompter` with `SearchSelect` / `SearchMultiSelect` for live typeahead

Each consumer would implement a filter-as-you-type widget.

* Good, because pterm already ships type-to-filter selects (Context §8).
* Bad, because MADR 0004 made `Prompter` a deliberately small interface. A new
  method is a coordinated change in both pterm consumers before the feature
  exists anywhere, and `TextPrompter` would still need a query-then-list loop.
* Bad, because a widget does not fix the corpus. Without the uncapped listing,
  typeahead still filters six ids.
* Bad, because `TextPrompter` cannot do true typeahead without raw mode, and
  MADR 0004 records raw-mode failures on Git Bash / mintty.

### Raise `MaxListedModels` or pass the full usable list into the existing `Select`

* Good, because it is a small change, and pterm's built-in filter would make it
  usable for the two pterm consumers.
* Bad, because `TextPrompter`, prepare-commit-msg's renderer, would print about
  one hundred numbered rows: the failure mode `MaxListedModels` exists to prevent
  (`models_catalog.go:9-10`).
* Bad, because it gives `TextPrompter` neither glob nor fuzzy search.
* Bad, because raising the cap also changes curation and the probe input.
  `probeGenerateHealth` shares the constant (`probe.go:17-20`), and a larger cap
  means more real generate calls per `DiscoverModels`.

### Add a third-party fuzzy library

* Good, because a known scorer is less code to maintain.
* Bad, because MADR 0004's dependency-light driver still applies, and N is
  about a hundred short strings.
* Bad, because common Go fuzzy packages do not implement the glob path or a
  `*` that crosses `/`; that code is ours either way.
* Neutral, because a library can still be adopted later behind the same
  `SearchModels` signature if the stdlib matcher proves inadequate.

### Widen `ListAvailableModels` itself to return the uncapped usable list

* Good, because one function stays the listing API.
* Bad, because its documented purpose is to keep configure menus short
  (`discovery.go:16-18`), tests assert `<= MaxListedModels`, and the probe paths
  would then test the first six ids in API order instead of the curated six
  (`probe.go:17-20` truncates without ranking). That is a silent quality change
  for every caller.
* Bad, because a caller that kept passing its result to `Select` would dump the
  full catalog: option 3 by another name.

### Leave search to each consumer wizard

* Good, because `mcplib` would not change.
* Bad, because it recreates the drift MADR 0004 removed: three matchers, three
  corpus policies, and a new provider in `mcplib` not searchable until three
  downstream edits land. The Grok gap recorded in MADR 0004 shows those edits do
  not all happen.
* Bad, because no consumer can see the uncapped usable list; the listers
  discard it inside `mcplib`.

## More Information

### Relationship to earlier decisions

* **MADR 0004** owns the descriptor / flow / renderer split and the six-method
  `Prompter`. This MADR extends that flow and does not change the prompting
  architecture.
* **MADR 0003** introduced the open catalogs (OpenCode, Hugging Face, Kilo) and
  their metadata-driven ranking, which `Usable` preserves. A blank search
  still shows the 0003/0004 curated order. **§1b partially supersedes 0003**:
  its `input_modalities == ["text"]` filter for Hugging Face and Kilo
  (`0003-MADR-add-gateway-llm-providers.md:806-807`, `:885-887`) becomes "contains
  `text`". 0003's output filter, tools rule, training policy and ranking stand.
  0003 is annotated with a forward pointer when this MADR is accepted; its text
  is not rewritten. **§1c partially supersedes 0003's "Bearer on every route"**
  statement (`0003-MADR-add-gateway-llm-providers.md:126-137`, at `55e4b31`).
* **MADR 0008** added ChatGPT and Grok OAuth. ChatGPT remains the three-id
  `StaticOpenAIChatGPT` short-circuit; search over three ids is correct, just
  small. Grok OAuth lists from the same `api.x.ai/v1/models` endpoint as an API key.

### Related findings, not decided here

These were found while verifying this record. Each predates it and is left
unchanged by it. Each needs its own decision or fix.

1. ~~**Exact-text input modality excludes the Kilo managed tiers** (Context §5).
   Consequence of inaction: the live recommended Kilo menu and the live Kilo
   search corpus both omit every `kilo-auto/*` tier that `StaticKilo`
   recommends. Candidate fix: require output modality exactly `["text"]` and
   input modalities merely to *include* `"text"` for Kilo and Hugging Face. This
   widens the usable set and needs its own evidence.~~ **Decided in revision 3:
   folded into this MADR as §1b, with the evidence in Context §5.**
2. **`TestConfigureLLM_EmptyDiscoveryFallsBackToStatic` makes a real network
   call** to `api.anthropic.com` with a fake key (Context §13). Its comment is
   wrong and its result depends on the network. Candidate fix: drive it from a
   provider with `SupportsBaseURL` against a non-200 `httptest` fixture.
3. **Listing ignores proxy settings.** `defaultHTTPClient`
   (`llmprovider/options.go:11-21`) builds an `http.Transport` without
   `Proxy: http.ProxyFromEnvironment`, so `HTTPS_PROXY` is not honoured for
   listing or generation. It was observed while trying to trace the network
   call in item 2: a logging proxy set via `HTTPS_PROXY` received no
   connection.
4. **`Existing.Fallbacks` is never pre-selected** (`selectFallbacks` passes
   `nil`, `configure.go:347`), so reconfiguration forgets fallbacks unless they
   are re-picked. This MADR keeps that behaviour.
5. **The ChatGPT catalog is retired** ([0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md) C2).
   * `StaticOpenAIChatGPT`'s `gpt-5.4`, `gpt-5.4-mini` and `gpt-5.3-codex`
     are no longer in the Codex CLI's catalog. Under §1 they are both views
     of the ChatGPT short-circuit, so ChatGPT users would search only retired
     ids.
   * The Codex backend has a model listing,
     `GET {base}/models?client_version=…`. Adopting it is part of the
     report's ChatGPT-conformance candidate, and it would make the
     short-circuit a real listing.
6. ~~**Zen per-route key headers** (report O1, confirmed live).~~ **Decided in
   revision 5 as §1c.**
   * Every Anthropic- or Google-shaped Zen/Go model fails with "Missing API
     key." today.
   * Search makes all of them selectable, not only the three in
     `StaticOpencodeZen`.
   * Search does not create the defect but widens its reach. The fix belongs
     to the report's transport candidate.
7. **The OpenCode route table has gaps** (report O2).
   * 18 active Zen models are missing from the table, and the prefix
     heuristic misroutes `qwen3.8-max`.
   * Search lets users pick any listed id, so gaps in the table surface as
     request errors.
   * Deriving routes from `api.json` `provider.npm` is the report's
     recommendation.
8. **Kilo `/models` anonymous retry** (report K5). On a 401 with a key and no
   organization, Kilo's client retries `/models` without credentials
   (`kilocode:packages/kilo-gateway/src/api/models.ts:242-245`, at
   `c267794785`). `mcplib` substitutes the static catalog instead. §1's
   fetch step could adopt the retry, which would keep search live when a key
   is wrong.

### Out of scope

* Flipping `Options.Discover` to default true.
* Filter-as-you-type, in `TextPrompter` or anywhere else.
* Searching models the usability filters reject.
* Expanding `modelLabels` to cover open catalogs.
* A fallback count limit or a free-text fallback hatch in `mcplib`.
* Unifying how consumers persist `Result` (MADR 0004's out-of-scope item
  stands).
* Health-probing search hits. Probes stay on the recommended six.
* A public interactive TUI in `mcplib`.
* Updating the consumers. Each adopts this on its own `mcplib` bump, with the
  test-script changes in §6.
* **How the recommended six are ranked** (revision 3). That covers:
  * use-case profiles, for example commit messages versus a reasoning tier;
  * quality signals: Kilo `terminalBench` and `preferredIndex`, and reasoning
    support;
  * external metadata for OpenCode Zen/Go (`models.opencode.ai/api.json`);
  * cost/age weighting, family diversity, and expiry;
  * the composition of the static catalogs, including `StaticKilo`'s two
    training-flagged entries and the 8B entries.

  The maintainer asked for this assessment. It is a separate decision,
  proposed as [0010-MADR-use-case-aware-default-model-ranking.md](0010-MADR-use-case-aware-default-model-ranking.md), and builds on this one's `ModelCatalog.Usable`.

### Evidence

Commit `55e4b31`, 2026-09-25. All read-only. No repository file was modified
while gathering it (`git status --porcelain` empty before and after).

* **Code facts:** read from the cited files and lines. Pagination absence:
  `grep -n 'has_more\|nextPageToken\|after_id\|pageToken\|last_id'` over
  `llmprovider/*.go` excluding tests returned nothing. Callers of
  `probeGenerateHealth` and `ListAvailableModels*` were enumerated by
  repository-wide grep.
* **Baseline:** `go test -count=1 ./llmprovider/ ./wizard/` passed.
* **Live catalogs:** anonymous `GET` of the four public listings
  (`opencode.ai/zen/v1/models`, `opencode.ai/zen/go/v1/models`,
  `router.huggingface.co/v1/models`, `api.kilo.ai/api/gateway/models`). The
  in-tree filters were replicated in a scratch Python script to count usable
  ids, top-level keys and id punctuation. Separately, a scratch Go module
  (`replace` directive to this checkout, outside the repository) called the
  in-tree `ListAvailableModels` for the same four providers and printed the
  curated results quoted in Context §4–§5.
* **Provider references:** Gemini `models.list` (ai.google.dev API reference:
  `pageSize` default 50, max 1000, `nextPageToken`). Anthropic List Models
  (platform.claude.com API reference: `limit` default 20, range 1–1000,
  `after_id`, `has_more`, `last_id`, newest first). OpenAI: the official
  `openai-python` `Models.list()` signature (no pagination parameters; returns
  `SyncPage`); the REST reference page refused an automated fetch (HTTP 403).
  xAI: the official `xai-sdk-python` `list_language_models()` sends an empty
  request with no page token. The xAI REST reference page fetched did not
  document `/v1/models`, so "xAI is single-page" is an assumption consistent
  with the SDK and with current code. Ollama: `docs/api.md` documents no
  parameters for `/api/tags`.
* **Consumers:** the `go.mod` files and wizard sources of the three consumers
  (cited in Context §9 and §13). pterm defaults were read from the
  `pterm@v0.12.83` module source.
* **Network-call finding:** by code path. Claude's descriptor lacks
  `supportsBaseURL`, so `resolveBaseURL` returns `""` and
  `listClaudeModels` uses its built-in endpoint. Observation through a proxy
  was not possible (Related finding 3).

* **Line citations into other `docs/` records** refer to their text at
  `55e4b31`.
* **Revision 3 evidence (2026-09-25):**
  * **Kilo client behaviour:** read from `kilocode` v7.7.12 (commit
    `14e7546535`), `packages/kilo-gateway/src/api/models.ts:101-105`, `:281`,
    `:295`, `:304` and `packages/opencode/src/kilocode/provider/model-filter.ts:3-14`.
  * **Rule comparison:** the 96/284 and 94/131 counts, the relaxed curated six,
    the output-set census and the ten name-denied ids come from a scratch Python
    replica of the in-tree Kilo and Hugging Face filters over the anonymous
    live listings. The replica reproduces the existing fixture tests' expected
    results under today's rule.
  * **Training flags:** the `mayTrainOnYourPrompts` values of `StaticKilo`
    entries were read from the same listing.

### Implementation notes (not a plan)

A PLAN sharing this number (`0009-PLAN-live-catalog-model-search.md`) should,
at minimum, sequence:

1. The fetch/curate split per lister, `ModelCatalog` and `ListModelCatalog*`,
   with `ListAvailableModels*` rewritten on top, plus equivalence tests.
1b. The §1b input-modality rule for Kilo and Hugging Face, with the two
   existing curation tests rewritten and MADR 0003 annotated.
1c. The §1c per-route key header, with `TestOpencode_KeyInHeader` reversed
   and an opt-in live drift test.
2. Gemini and Anthropic pagination with the §2 bound and failure rules.
3. `SearchModels` and its tests.
4. `discoverModels`, `selectModel` and `selectFallbacks` rewritten per §4–§5,
   with `TestConfigureLLM_OtherModelEscapeHatch` updated for the leading `""`.
5. Documentation (README / wizard guide) and a release note stating the
   prompt-sequence change in §6.

Each new check must be proven to fail on a planted input before it is relied
on. No source changes accompany this proposed MADR.
