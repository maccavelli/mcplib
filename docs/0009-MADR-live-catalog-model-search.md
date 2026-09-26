---
status: proposed
date: 2026-09-24
decision-makers: mcplib maintainers
consulted: mcp-server-magictools, mcp-server-magicdev, prepare-commit-msg
informed: all mcplib consumers
---
# Search Live Provider Catalogs for Primary and Fallback Model Selection

## Context and Problem Statement

`wizard.ConfigureLLM` (`wizard/configure.go`) is the canonical configuration flow
introduced by MADR 0004. After the user picks a provider and a credential, it
offers a **short, numbered menu** of models and, when `Options.NeedFallbacks` is
set, a **multi-select of the remainder**. Both menus are drawn from the same
list. That list is deliberately tiny.

The presented choices are too limiting for providers that publish large catalogs,
and there is no way to search. A user who wants a model that is not one of the
six curated IDs has a single escape hatch: type the exact id by hand
(`otherModelLabel` at `wizard/configure.go`, the trailing `"Other (enter a model
id)"` entry). Guessing a Hugging Face or Kilo id is not a substitute for search.

### What the wizard actually offers today (verified in-tree)

**1. The menu is capped at six.** `MaxListedModels = 6`
(`llmprovider/models_catalog.go`) is documented as "the hard cap for
configure-time model menus" so wizards stay short and do not dump unusable API
IDs. Every static catalog is asserted `len(cat) <= MaxListedModels`
(`llmprovider/models_catalog_test.go`). `curateFromCatalog` returns as soon as
the output reaches that cap (`llmprovider/models_catalog.go`).

**2. Live listing is an availability filter on that six, not a catalog dump.**
`ListAvailableModels` (`llmprovider/discovery.go`) "fetches models from a
provider listing API when available, then curates them against the static
catalog so configure UIs never show huge unusable lists". Each lister builds a
full `available` slice of usable IDs, then throws all but six away:

| Lister | After the live GET | Then |
|---|---|---|
| `listGeminiModels` | usable `generateContent` text IDs | `curateFromCatalog(StaticGemini, …)` |
| `listOpenAIModels` | `isUsableOpenAIChatModel` hits | `curateFromCatalog(StaticOpenAI, …)` |
| `listClaudeModels` | `isUsableClaudeTextModel` hits | `curateFromCatalog(StaticClaude, …)` |
| `listGrokModels` | `isUsableGrokModel` hits | `curateFromCatalog(StaticGrok, …)` |
| `listOpencodeModels` | `isUsableOpencodeModel` hits | `curateFromCatalog(staticOpencodeCatalog, …)` |
| `listHuggingFaceModels` | text→text, live-provider, ranked by throughput | `curateFromCatalog(StaticHuggingFace, …, nil)` |
| `listKiloModels` | text→text, tools, not `mayTrainOnYourPrompts`, ranked by price | `curateFromCatalog(StaticKilo, …, nil)` |
| `listOllamaModels` | every installed name from `GET /api/tags` | **truncated in the lister itself** to `MaxListedModels` |

The live HTTP response is already in hand. The extra IDs are discarded before
the wizard ever sees them.

**3. The catalogs that hurt are the open gateways.** Hugging Face's router listing
was probed at **317 offerings** on 2026-08-29 (`llmprovider/live_gateways_test.go`,
`TestLive_HuggingFaceMetadataFields`). Kilo is documented in this package as a
**366-model open catalog** (`llmprovider/models_catalog.go` on `StaticKilo`;
`isUsableKiloModel` notes 25 of 366 excluded by the training-policy flag on the
same date). OpenCode Zen/Go are smaller but still larger than six, and Ollama
installs are machine-specific with no static catalog at all. A six-item menu
cannot represent those sources.

**4. Primary and fallback selection share that truncated list.**
`selectModel` (`wizard/configure.go`) builds `modelChoices` from `discoverModels`
plus the Other hatch and calls `Prompter.Select`. `selectFallbacks` removes the
primary and calls `Prompter.MultiSelect` on what is left. A fallback can only be
one of the other five curated IDs. There is no second look at the live catalog.

**5. Numbering already exists on the renderer that ships in this library.**
`TextPrompter.renderChoices` prints `  %d) %s` with 1-based indices
(`wizard/text_prompter.go`). `Select` accepts `1..N`; `MultiSelect` accepts a
comma-separated list of the same indices. Consumer `pterm` implementations of
`Prompter` already render their own numbered / navigable widgets. The missing
piece is the **corpus and the query**, not the digits.

**6. `Prompter` is a stable public seam.** MADR 0004 made `mcplib` own descriptors
and flow, and consumers own rendering. `wizard/prompter.go` states the interface
"is deliberately small and stable" and that "changing it later is a breaking
change for all of them." Twelve repositories consume this module; three implement
`Prompter`. A new method is a coordinated consumer change. Reusing `Input` +
`Select` / `MultiSelect` is not.

**7. Discovery is opt-in and degrades to static.** `Options.Discover` defaults
false (`wizard/configure.go`: "The zero value runs a full interactive
configuration over every known provider with no discovery"). When it is true,
`discoverModels` calls `ListAvailableModelsWithSource` with a 20s wizard timeout
wrapped around the lister's own 10s timeout, and on error or an empty result it
notifies and returns `d.StaticModels` (ChatGPT OAuth uses `StaticOpenAIChatGPT`
instead). ChatGPT OAuth never hits a list endpoint: `ListAvailableModelsWithSource`
short-circuits to those three IDs (`llmprovider/discovery.go`).

**8. Health probing must not grow with the catalog.** `probeGenerateHealth`
(`llmprovider/probe.go`) already caps candidates at `MaxListedModels` and fires
a real generate per candidate. Widening `ListAvailableModels` would turn a
six-probe into a hundreds-probe. Tests pin the current contract:
`TestListAvailableModels_OpencodeZen` requires `1..MaxListedModels`
(`llmprovider/discovery_test.go`).

**9. `mcplib` has no fuzzy-search dependency, and MADR 0004 forbade adding UI
stacks.** Direct requires in `go.mod` are `invopop/jsonschema`,
`modelcontextprotocol/go-sdk`, `golang.org/x/mod`, `golang.org/x/sys`,
`golang.org/x/term`. Catalogs in play are hundreds of strings, not millions.

### The requirement, restated against that evidence

A user configuring a provider must be able to **type a search string**, have it
run against the **provider's live usable catalog** (the `available` slice the
listers already compute), see **numbered matches**, and pick one. The same
interaction must be available when choosing **fallback / backup** models.
Matching must treat glob metacharacters as a first-class, cheaper path, and
must tolerate the inexact queries people actually type (`flash`, `llama 8b`,
`sonet`, `gpt-4*`).

The Other hatch stays. A live listing can still lag a brand-new id, which is
why `TestConfigureLLM_OtherModelEscapeHatch` exists.

## Decision Drivers

* **The live catalog is the search corpus.** Curated sixes remain the
  *recommended* list, not the only list a user can reach.
* **Usability filters stay on.** Embeddings, TTS, vision, dated Gemini
  previews, and Kilo's `mayTrainOnYourPrompts` models must not become
  searchable just because the cap comes off. The `isUsable*` functions and
  Kilo's listing-time policy check are the filter; search ranks what they
  pass.
* **`ListAvailableModels` keeps its six-item contract.** Probe, static-count
  tests, and any consumer that still treats the function as "the menu" depend
  on it. Search needs a sibling, not a silent widening.
* **`Prompter` does not grow.** Search has to land for all three wizards on an
  `mcplib` upgrade, without each consumer implementing a new method.
* **`mcplib` stays dependency-light.** A matcher over a few hundred IDs does
  not justify a new module.
* **Primary and fallbacks are the same interaction.** `NeedFallbacks` already
  exists; it must search, not multi-select leftovers of the recommended six.
* **Numbered selection is the existing TextPrompter contract.** Matches are
  `[]Choice` handed to `Select` / `MultiSelect`.
* **Blank input preserves the current happy path** as far as the `Prompter`
  script allows: an empty search query still offers the recommended six plus
  Other, so a user who wants the default still presses Enter.
* **Testability without a TTY.** `fakePrompter` already records every `Input`
  and `Select`. The matcher is a pure function of `(provider, ids, query)`.

## Considered Options

* Search corpus in `llmprovider` plus a search-then-select flow in `wizard`, `Prompter` unchanged
* Extend `Prompter` with `SearchSelect` / `SearchMultiSelect` for live typeahead
* Raise `MaxListedModels` or pass the full usable list into the existing `Select`
* Add a third-party fuzzy library (`sahilm/fuzzy` or equivalent)
* Widen `ListAvailableModels` itself to return the uncapped usable list
* Leave search to each consumer wizard

## Decision Outcome

Chosen option: **"Search corpus in `llmprovider` plus a search-then-select flow
in `wizard`, `Prompter` unchanged"**, because it is the only option that puts
the live usable catalog behind a query, numbers the hits with the renderer
each consumer already has, covers primary and fallbacks, and leaves MADR 0004's
seams and `ListAvailableModels`'s six-item contract intact.

### 1. Uncapped usable listing — `ListUsableModels`

Add `ListUsableModels` / `ListUsableModelsWithSource` beside
`ListAvailableModels` in `llmprovider/discovery.go`. Same providers, same
auth, same 10s listing timeout, same degrade-to-`StaticModels` (and the same
ChatGPT OAuth short-circuit to `StaticOpenAIChatGPT`). The return value is
every ID the lister already classifies as usable, **not** passed through
`curateFromCatalog` and **not** truncated to `MaxListedModels`.

Refactor the eight listers so the HTTP fetch + usability filter is shared:

* `ListAvailableModels*` keeps calling `curateFromCatalog` (Ollama: truncate
  after the fetch) and stays `<= MaxListedModels`.
* `ListUsableModels*` returns the `available` slice the listers already build
  and currently discard.

Ollama is the one lister that truncates in place (`listOllamaModels`). The
truncation moves to the curated path only; the usable path returns every
installed name.

**Pagination.** This package never follows a next-page token: a search of
`llmprovider/discovery.go` for `has_more`, `nextPageToken`, `after_id`, and
`pageToken` is empty. Hugging Face, Kilo, OpenCode, Grok, and Ollama are
single-shot full dumps in the current code, so `ListUsableModels` is complete
for them on day one. Gemini and Anthropic list APIs are documented as
paginated; a single decode therefore searches an incomplete corpus for those
two. Following the provider's documented pagination belongs in
`ListUsableModels` (and in the shared fetch, so `ListAvailableModels` sees
the same pages when it curates). OpenAI's `/v1/models` has historically been
a single page; the implementation follows a next page when the decoded
envelope provides one and otherwise keeps the current single decode. This is
in scope because "search the live catalog" is false if the catalog is the
first page.

**Usability is unchanged.** `ListUsableModels` for Kilo still drops
`mayTrainOnYourPrompts` (policy, `listKiloModels`) and still requires
text→text plus `tools`. Hugging Face still requires text→text and at least
one live provider. OpenAI still uses `isUsableOpenAIChatModel`. Search does
not become a back door onto embeddings or training-on-prompts models.

### 2. Matcher — `SearchModels` in `llmprovider`

A pure function, no I/O:

```go
type ModelMatch struct {
    ID    string
    Label string
    Score int
}

func SearchModels(provider string, models []string, query string, limit int) []ModelMatch
```

`Label` is `ModelLabel(provider, id)` so a curated id keeps its annotation and
every other id degrades to the bare id (`models_catalog.go`: "a live listing
that outruns this table degrades to raw ids rather than hiding models").
`limit <= 0` means "no cap"; the wizard passes a display cap (see §4).

**Query dispatch (wildcard-optimised).** After trimming and lowercasing:

* Empty query: return no matches. The wizard treats empty as "show
  recommended", which is a flow decision, not a matcher decision.
* Query contains unescaped `*` or `?`: **glob path only**. Compile one
  case-insensitive regexp (`*` → `.*`, `?` → `.`, all other regexp
  metacharacters quoted). The whole id **and** its label are matched as flat
  strings, so `*` matches `/`. `path.Match` / `filepath.Match` are rejected
  for this corpus: Go's `*` does not match `/`, and ids such as
  `meta-llama/Llama-3.1-8B-Instruct` and `kilo-auto/balanced` would silently
  miss `*llama*` / `kilo-auto/*`. Glob does not run the fuzzy scorer.
* Otherwise: **fuzzy path** over the same id + label surface.

**Fuzzy scoring (stdlib, no new module).** Normalise by lowercasing and
treating `[-_/. ]+` as token separators so `llama 8b` and `llama-8b` and
`Llama-3.1-8B` share tokens.

A candidate matches if any of:

1. Case-insensitive substring of id or label.
2. Every query token is a prefix of some id/label token (order-independent).
3. Query characters appear in order as a subsequence of id or label
   (fzf-style).

Score, descending, with the usual bonuses: exact equality, substring, token
prefix, consecutive-run and word-boundary bonuses on the subsequence. Stable
sort by score desc, then shorter id, then lexicographic id. Candidates that
fail all three predicates are dropped, so a one-character subsequence does
not dump the catalog.

This is enough for the queries the requirement names. A separate Levenshtein
pass is not included: subsequence already accepts `sonet` → `sonnet` and
`gemni` → `gemini`, and a second edit-distance pass would dominate the glob
fast path the requirement asked to keep cheap.

### 3. Wizard flow — primary

Replace the body of `selectModel`. `Prompter` stays six methods.

1. Build two corpora from the same listing attempt:
   * **recommended** — today's `discoverModels` result (`ListAvailableModels`
     when `Options.Discover`, else static; ChatGPT OAuth static as today).
   * **searchable** — `ListUsableModelsWithSource` when `Options.Discover`,
     else the same static catalog. On live failure, notify (existing warning)
     and use static. Timeout and `WithBaseURL` match `discoverModels`.
2. `Input("Search models (blank for recommended)", "")`.
3. Empty query → `Select` on `modelChoices(recommended)` plus Other last.
   This is today's menu. Unscripted `fakePrompter.Input` already returns the
   default, so Select-only tests that pick a recommended index keep working.
4. Non-empty query → `SearchModels(provider, searchable, query, MaxSearchResults)`.
   `Select` on numbered matches, then **"Search again"**, then Other last.
   Choosing Search again loops to step 2. Choosing Other keeps the existing
   manual-id `Input`.
5. Zero matches → `Notify` that nothing matched, then the Search-again /
   Other menu so the user is not dead-ended.

`MaxSearchResults` is **20**. A numbered list longer than a screen is the
problem this MADR is solving; twenty hits plus two actions fits, and a
trailing notify ("showing 20 of N; refine the search") tells the user to
narrow. The matcher still ranks the whole corpus; the cap is display-only.

`Options.Discover` remains the network gate. Live search requires
`Discover: true`, which production wizards that already opt into live listing
pass today. The zero value still skips the network so tests and offline runs
search the static catalog. This MADR does not flip that default.

### 4. Wizard flow — fallbacks

Replace the body of `selectFallbacks` when `NeedFallbacks` is true. Same two
corpora, with the primary id and any already-chosen fallback ids removed
before matching.

1. `Input("Search fallback models (blank for recommended, or to skip)", "")`.
2. Empty query → today's `MultiSelect` on remaining recommended ids. An empty
   multi-select is still "no fallbacks". Unscripted Input + scripted
   MultiSelect keeps `TestConfigureLLM_Fallbacks` working.
3. Non-empty query → `SearchModels` on the remaining searchable corpus,
   `MultiSelect` on numbered matches (plus no Search-again row; a Confirm
   follows).
4. `Confirm("Add more fallbacks?", false)`. Yes loops to step 1 with already
   chosen ids excluded.

The Other hatch is not duplicated on the fallback step: a fallback the live
catalog does not list is a primary-model problem, and the user can re-run
configuration. If a fallback must be an unseen id, that is a later change;
this MADR does not add a second free-text hatch.

### 5. What does not change

* `Prompter`, `Choice`, `TextPrompter` numbering, `Result`, `Options` fields.
* `MaxListedModels`, `curateFromCatalog`, `probeGenerateHealth`.
* `ModelLabel`, static catalogs, deny lists, Kilo training policy.
* `otherModelLabel` as the last entry on the **primary** menu.
* Orchestrated processes still return `ErrOrchestrated` before any of this.

### Consequences

* Good, because Hugging Face, Kilo, OpenCode, and a well-stocked Ollama host
  become searchable instead of six guesses plus a typed id.
* Good, because the live GET the listers already perform is reused; search
  does not add a second HTTP surface.
* Good, because `ListAvailableModels` and probe stay six-item, so existing
  tests and any consumer menu that still calls that function do not become
  a 366-row dump.
* Good, because every `Prompter` implementation gains the feature on an
  `mcplib` bump: the flow uses `Input` + `Select` / `MultiSelect` only.
* Good, because glob queries skip fuzzy scoring and treat `/` as an ordinary
  character, which is what `kilo-auto/*` and `*llama*` need.
* Good, because the matcher is a pure function and can be made to fail on
  demand (glob that should miss, typo that should hit, deny-listed id that
  must not appear) without a TTY.
* Good, because blank search restores today's recommended menu, so the
  default-Enter path survives.
* Neutral, because `Options.Discover` still has to be true for the corpus to
  be live. Callers that never set it search six static IDs. That is the
  existing zero-value contract, not a new limitation.
* Neutral, because match labels for non-curated ids are the bare id.
  `ModelLabel`'s table is not required to cover the open catalogs.
* Bad, because the primary flow grows an `Input` before `Select`. Scripts
  that feed the Other hatch's model id as the first `Input` will have that
  string consumed as a search query. `TestConfigureLLM_OtherModelEscapeHatch`
  is exactly that pattern (`inputs: []string{"my-custom-model"}`) and must
  send an empty search line first. Consumer tests that do the same need the
  same extra empty `Input`. Inserting "Search the catalog…" as a menu row
  was rejected so recommended indices stay stable; Other-by-index still
  shifts if a search-result menu is showing, which is unavoidable.
* Bad, because Gemini and Anthropic search are incomplete until listing
  pagination is implemented. Shipping search against the first page only
  would look like a working catalog and hide the rest. Pagination is
  therefore in this decision, not a follow-up.
* Bad, because a 20-hit cap can hide a still-relevant match; the truncated
  notify is the mitigation, and the user refines rather than scrolling.
* Bad, because subsequence matching is permissive on very short queries.
  The three predicates plus the display cap plus "refine the search" are
  the mitigation; a one-character query will still return twenty hits.

### Confirmation

Compliance is confirmed by tests that have been seen to fail on a broken
input, not only to pass on a good one:

* `ListAvailableModels` still returns `1..MaxListedModels` on the existing
  fixtures (`TestListAvailableModels_OpencodeZen` and siblings).
* `ListUsableModels` on the same Hugging Face / Kilo / OpenCode fixtures
  returns every usable id, including ones `curateFromCatalog` would drop,
  and still excludes deny-listed and `mayTrainOnYourPrompts` ids.
* `listOllamaModels`'s usable path returns more than six installed names
  when the fixture has more than six.
* Pagination: a Gemini or Anthropic fixture that splits usable ids across
  two pages returns both pages from `ListUsableModels`; a single-page
  fixture still works. A deliberately truncated matcher that stops after
  page one must fail that test before the follow-on implementation is
  trusted.
* `SearchModels`:
  * glob `kilo-auto/*` matches `kilo-auto/balanced` and does not run a
    fuzzy path (assert by a glob that would also fuzzy-match a non-glob
    id and checking it is absent).
  * glob `*llama*` matches `meta-llama/Llama-3.1-8B-Instruct` (the
    `path.Match` counterexample).
  * `flash` ranks `gemini-3.5-flash` above unrelated subsequence hits.
  * `llama 8b` matches `meta-llama/Llama-3.1-8B-Instruct`.
  * `sonet` matches `claude-sonnet-5`.
  * empty query returns nil.
  * a deny-listed id present in the input slice is absent from output.
* Wizard, via `fakePrompter`:
  * blank primary search + index 0 yields the first recommended model
    (existing Select-only tests).
  * query `haiku` then index 0 yields a haiku id from the searchable
    corpus, not only from the static six, when `Discover` is true against
    a fixture.
  * Other still returns a typed id; the script includes the empty search
    line.
  * fallbacks: blank search + `MultiSelect` of remaining recommended
    still matches `TestConfigureLLM_Fallbacks`; a fallback query plus
    multi-select records ids that were not in the recommended six; the
    primary id never appears in fallback matches.
* `Prompter` method set is unchanged (`var _ Prompter = (*TextPrompter)(nil)`
  and the fake). A new method would fail those assignments until every
  implementation grew, which this decision forbids.

## Pros and Cons of the Options

### Search corpus in `llmprovider` plus a search-then-select flow in `wizard`, `Prompter` unchanged

The chosen option. Matcher and uncapped listing live next to the catalogs
they search; the wizard owns the loop; numbering stays in the renderer.

* Good, because it meets live-catalog search, glob, fuzzy, numbered picks,
  and fallbacks without a breaking `Prompter` change.
* Good, because `ListAvailableModels` remains safe to call from probe and
  from any consumer that still wants a short menu.
* Good, because a glob compile-once / scan-once path stays in-process and
  allocation-light on a 366-row catalog.
* Neutral, because TextPrompter search is query-then-list, not filter-as-
  you-type. That is the cost of not growing `Prompter`.
* Bad, because consumer tests that stuff the Other model id into the first
  `Input` need an extra empty line.

### Extend `Prompter` with `SearchSelect` / `SearchMultiSelect` for live typeahead

Each consumer would implement a filter-as-you-type widget. `pterm` could
look nicer; `TextPrompter` would still be a query-then-list loop internally.

* Good, because magictools / magicdev could reuse whatever interactive
  filter they already have.
* Bad, because MADR 0004 called `Prompter` a permanent, small interface in
  a library twelve repos consume; a new method is a coordinated three-repo
  change before the feature exists anywhere.
* Bad, because the corpus problem is not solved by a new widget. Without
  `ListUsableModels`, typeahead still filters six IDs.
* Bad, because `TextPrompter` cannot do true typeahead without raw mode on
  every platform, and MADR 0004 already recorded that raw mode fails on
  Git Bash / mintty.

### Raise `MaxListedModels` or pass the full usable list into the existing `Select`

* Good, because it is a small code change.
* Bad, because a 366-row numbered menu is the failure mode
  `MaxListedModels` was introduced to prevent
  (`models_catalog.go`: "avoids dumping dozens of unusable API IDs").
* Bad, because it provides neither glob nor fuzzy search.
* Bad, because `probeGenerateHealth` shares the cap and would generate
  against hundreds of models.

### Add a third-party fuzzy library

* Good, because a well-known scorer would be less to maintain.
* Bad, because MADR 0004's dependency-light driver still applies, and the
  N is hundreds of short strings.
* Bad, because popular Go fuzzy packages still would not implement the
  glob fast path or `/`-matching the catalog needs; that code is ours
  either way.
* Neutral, because adding a module later remains possible if the stdlib
  matcher proves inadequate; nothing in this decision seals the function
  body, only the signature and the glob-first dispatch.

### Widen `ListAvailableModels` itself to return the uncapped usable list

* Good, because one function stays the listing API.
* Bad, because the function's documented purpose is to keep configure UIs
  short (`discovery.go`), tests assert `len <= MaxListedModels`, and probe
  generates against the result. Changing the contract is a silent behaviour
  break for every caller.
* Bad, because a wizard that kept calling `Select` on that result would
  dump the full catalog, which is option 3 by another name.

### Leave search to each consumer wizard

* Good, because `mcplib` would not change.
* Bad, because it recreates the MADR 0004 drift: three matchers, three
  corpus policies, and a new provider in `mcplib` would not become
  searchable until three downstream edits land. The Grok gap was the
  existence proof that those edits do not all happen.

## More Information

### Relationship to earlier decisions

* **MADR 0004** owns the descriptor / flow / renderer split and the six-method
  `Prompter`. This MADR is an extension of that flow, not a new prompting
  architecture. Growing `Prompter` would contradict 0004; reusing it is the
  compliance path.
* **MADR 0003** introduced the open catalogs (OpenCode, Hugging Face, Kilo)
  and the metadata-driven ranking that `ListUsableModels` must preserve.
  Search ranks by query score; when the query is empty, the recommended six
  remain the 0003/0004 curated order (throughput for HF, price for Kilo,
  `Rank*` for the first-party providers).
* **MADR 0008** added ChatGPT / Grok OAuth. ChatGPT listing remains the
  three-id `StaticOpenAIChatGPT` short-circuit; search over three IDs is
  still correct, just small. Grok OAuth uses the same `api.x.ai/v1` listing
  as an API key.

### Out of scope

* Flipping `Options.Discover` to default true.
* Filter-as-you-type, even in `TextPrompter`.
* Searching models the usability filters reject (embeddings, TTS, vision,
  `mayTrainOnYourPrompts`).
* Expanding `modelLabels` to cover open catalogs.
* Unifying how consumers persist `Result` (MADR 0004 out of scope stands).
* Health-probing search hits. Probe stays on the recommended six.
* A public interactive TUI in `mcplib`.

### Implementation notes (not a plan)

A later PLAN sharing this number should, at minimum: split each lister into
fetch+filter vs curate; add pagination on Gemini and Anthropic list calls;
export `ListUsableModels*`; add `SearchModels` with the glob-first dispatch;
rewrite `selectModel` / `selectFallbacks`; update
`TestConfigureLLM_OtherModelEscapeHatch` for the extra empty `Input`; and
prove each new gate fails on planted input before relying on it. No source
changes accompany this proposed MADR.
