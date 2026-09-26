---
status: in-progress
date: 2026-09-26
associated-madr: "0009-MADR-live-catalog-model-search.md"
decision-makers: mcplib maintainers
---

# Implement Search Live Provider Catalogs for Primary and Fallback Model Selection

Associated MADR: [0009-MADR-live-catalog-model-search.md](0009-MADR-live-catalog-model-search.md)
(proposed, revision 3, 2026-09-25).

> **Plan revision 2026-09-25 (MADR revision 3).** The maintainer folded the
> Kilo and Hugging Face input-modality fix into 0009. Changes: new Phase 1b;
> Phase 0 annotates MADR 0003; Phase 1's filter test no longer asserts on
> `org/vlm`, because Phase 1b changes that rule; A1 names the two rewritten
> tests; A16 is added; §8 and §9 are updated. Default-model ranking for commit
> messages is a separate, proposed decision ([0010-MADR-use-case-aware-default-model-ranking.md](0010-MADR-use-case-aware-default-model-ranking.md))
> and is not in this plan.
>
> **Plan revision 2026-09-26 (MADR revision 5).** The maintainer folded the
> Zen/Go per-route key-header fix into 0009 (MADR §1c). Changes:
>
> * new Phase 0b, run immediately after Phase 0;
> * Phase 0's MADR 0003 note also cites §1c;
> * A17 is added;
> * Appendix B gains an optional `tags` field for build-tagged live tests,
>   re-proven.
>
> **Plan note 2026-09-26.**
> [0011-REPORT-provider-source-compatibility-audit.md](0011-REPORT-provider-source-compatibility-audit.md)
> adds MADR related findings 5-8. No step here changes:
>
> * the wizard tests use `httptest` fixtures, which the Zen key-header defect
>   (report O1) does not affect;
> * Phase 0's pointer note on MADR 0003 goes directly under the title, above
>   the audit note the report added;
> * line citations into other `docs/` records refer to their text at
>   `55e4b31`.

This plan executes that MADR and nothing else. If execution finds a fact that
contradicts the MADR or this plan, **stop and prompt**: record a dated entry in
§10, amend the MADR when a decision or an asserted fact changes, and only then
continue. Do not absorb a deviation silently, and do not widen scope.

## Goal

A user of `wizard.ConfigureLLM` can type a search string, run it against the
provider's live usable catalog, and pick a numbered match, for the primary
model and for fallbacks, through the existing six-method `Prompter`. Specifically:

* `llmprovider` exposes `ModelCatalog`, `ListModelCatalog` and
  `ListModelCatalogWithSource`: one listing, returning the curated six and the
  uncapped usable list, plus whether the list is live.
* Gemini and Anthropic listings follow pagination.
* `llmprovider.SearchModels` ranks ids with a glob path and a fuzzy path.
* `wizard` runs search-then-select for the primary model and the fallbacks.
* Kilo and Hugging Face admit models whose input *contains* text (MADR §1b).
* OpenCode Zen/Go send the key in the header each route reads: `x-api-key`,
  `x-goog-api-key` or `Authorization` (MADR §1c).
* `ListAvailableModels*`, `Prompter`, `Options`, `Result`, the static catalogs
  and every other usability filter behave exactly as they do today.

## Scope

**In scope:** `llmprovider/discovery.go`, the new `llmprovider/model_matcher.go`,
`wizard/configure.go`, the new `wizard/model_select.go`, their tests, the test
double `wizard/fake_prompter_test.go`, two existing wizard tests whose scripts
the new prompt changes, the two existing curation tests and fixtures that pin
the old input rule (`llmprovider/discovery_test.go`), `README.md`, a forward
pointer in MADR 0003, and this plan and its MADR.

**Out of scope:** everything in the MADR's *Out of scope* and *Related findings*
lists. That includes default-model ranking, the network call in `TestConfigureLLM_EmptyDiscoveryFallsBackToStatic`, proxy support in
`defaultHTTPClient`, and every consumer repository. See §8.

## 0. Baseline, conventions and proven tool behaviour

Every fact in this section was verified on 2026-09-25 against `mcplib` at
`55e4b31` with only the MADR modified. Appendix C records how.

### 0.1 Baseline

* `go test -count=1 ./...` exits 0 (8 packages `ok`).
* `make lint` exits 0 with `0 issues.` (golangci-lint 2.13.2, Go 1.26.6).
* `gofmt -l llmprovider wizard` prints nothing; `go vet ./llmprovider ./wizard`
  exits 0.
* `golint -set_exit_status` exits 0 on `llmprovider/discovery.go`,
  `llmprovider/models_catalog.go`, `llmprovider/discovery_test.go`,
  `wizard/configure.go`, `wizard/configure_test.go`,
  `wizard/fake_prompter_test.go` and `wizard/prompter.go`.
* Git hooks resolve to the global hooks directory
  (`git rev-parse --path-format=absolute --git-path hooks`), so
  `git commit --no-edit` takes its message from the `prepare-commit-msg` hook.

### 0.2 Tool behaviour this plan depends on (each proven, Appendix C)

1. **`golint` exits 0 on a finding unless given `-set_exit_status`.** On a file
   with an uncommented exported function, `golint bad.go` printed the finding
   and exited 0; `golint -set_exit_status bad.go` exited 1. Always pass the
   flag, one file per invocation. Passing files from two packages in one call
   prints `is in package wizard, not llmprovider` and still exits 0.
2. **`.golangci.yml` silently exempts any path matching**
   `(ui\.go|analyzer\.go|client\.go|tui\.go|login\.go|main\.go|search\.go|git\.go|config\.go|index\.go|internal\.go|logs\.go|kibana\.go)`
   from errcheck, gocognit, goconst, gocritic, gocyclo, gosec, ineffassign,
   nilerr, revive, staticcheck, unparam and unused. The fleet config, run over
   two identical files, reported the unchecked `os.Remove` in
   `model_matcher.go` and **nothing** in `model_search.go`. That is why the
   matcher lives in `model_matcher.go`. Every file this plan creates or edits
   was checked against the regex: none matches.
3. **`gofmt -l` exits 0 even when it lists a file.** The gate treats any output
   as failure.
4. **mcplib has no `scripts/go-precheck.sh` and no `pre-add-check` target**
   (`Makefile` targets: `test test-sum fmt vet lint tidy vuln help`). The
   global commit hook therefore runs `gofmt -l` and per-file `golint` on staged
   Go files. This plan's gate (Appendix A) is stricter and runs first.

### 0.3 Conventions

* `$SCRATCH` is the executing session's scratchpad directory. Nothing this plan
  generates outside the repository goes anywhere else, and no Python artefact
  is ever created inside the repository.
* **Phase gate** = Appendix A's `phase_gate.py` with the phase's exact file list
  and `--packages ./llmprovider ./wizard`. It runs gofmt, per-file golint, vet,
  `make lint` and `go test -count=1` as separate processes, logs each in full,
  and exits 0 only if all pass. Never pipe a gate into `tail` or `head`.
* **Seen to fail.** Every new test is either run red before its implementation
  (the FAIL line is copied into §11), or proven by a planted defect in a
  scratch copy with Appendix B's `mutate_and_test.py`. Most are both. Planted
  defects are never made in the working tree.
* **Indexing.** `fakePrompter.Select` scripts are 0-based. `TextPrompter` input
  is 1-based (`wizard/text_prompter.go:149`).
* One `git commit --no-edit` per phase, after the gate passes, staging only that
  phase's listed files. No `-m`, no `--amend`, no `git push`, no tag.
* The names, signatures, constants and strings in §1 are **the** implementation,
  not sketches. Changing one is a deviation (§10).

## 1. Locked design

### 1.1 Exported API (`llmprovider`)

```go
// ModelCatalog is the result of one model listing, viewed two ways.
type ModelCatalog struct {
	// Recommended is what ListAvailableModels returns: at most
	// MaxListedModels ids, curated against the static catalog.
	Recommended []string
	// Usable is every id the provider's usability filters admit, in listing
	// order, uncapped. It equals Recommended when Live is false.
	Usable []string
	// Live reports whether Usable came from the provider's listing rather
	// than the static catalog.
	Live bool
}

func ListModelCatalog(ctx context.Context, providerName, apiKey string, opts ...ProviderOption) (ModelCatalog, error)
func ListModelCatalogWithSource(ctx context.Context, providerName string, src TokenSource, opts ...ProviderOption) (ModelCatalog, error)

// ModelMatch is one ranked SearchModels result.
type ModelMatch struct {
	ID    string
	Label string // ModelLabel(provider, ID)
	Score int    // higher is better; comparable only within one call
}

func SearchModels(provider string, models []string, query string) []ModelMatch
```

`ListAvailableModels` and `ListAvailableModelsWithSource` keep their
signatures and doc comments. Their bodies become a call to the catalog function
that returns `.Recommended`.

### 1.2 Listing internals (`llmprovider/discovery.go`)

```go
const (
	maxListingPages     = 10
	geminiListPageSize  = "1000"
	claudeListPageLimit = "1000"
)

// catalogFrom applies the degrade-to-static contract every lister except
// Ollama has always had: a failed fetch, or one that yields no usable id,
// substitutes the static catalog.
func catalogFrom(usable []string, fetchErr error, static []string, curate func([]string) []string) ModelCatalog

// staticCatalog wraps a caller-owned copy of a static catalog.
func staticCatalog(static []string) ModelCatalog

// modelCatalogFor dispatches one provider's fetch and curation. The caller
// owns the timeout.
func modelCatalogFor(ctx context.Context, providerName, apiKey string, cfg ProviderConfig) (ModelCatalog, error)
```

`catalogFrom` body, locked:

```go
if fetchErr != nil || len(usable) == 0 {
	return staticCatalog(static)
}
recommended := curate(usable)
if len(recommended) == 0 {
	return staticCatalog(static)
}
return ModelCatalog{Recommended: recommended, Usable: usable, Live: true}
```

`staticCatalog` body, locked (one line, used as a mutation anchor):

```go
return ModelCatalog{Recommended: static, Usable: slices.Clone(static), Live: false}
```

Per-provider fetch-and-filter functions. Each returns the usable ids in listing
order and a non-nil error for request-build, transport, non-200 or decode
failure. Each contains the filtering code moved **unchanged** from today's
lister:

| Function | Moves from | Curation passed to `catalogFrom` |
|---|---|---|
| `fetchGeminiUsable(ctx, apiKey, cfg)` | `discovery.go:66-105` + pagination (§1.3) | `curateFromCatalog(StaticGemini, u, func(s string) bool { return isUsableGeminiTextModel(s, []string{methodGenerateContent}) }, RankGeminiModel)` |
| `fetchOpenAIUsable(ctx, apiKey, cfg)` | `:117-153` | `curateFromCatalog(StaticOpenAI, u, isUsableOpenAIChatModel, RankOpenAIModel)` |
| `fetchClaudeUsable(ctx, apiKey, cfg)` | `:164-202` + pagination (§1.3) | `curateFromCatalog(StaticClaude, u, isUsableClaudeTextModel, RankClaudeModel)` |
| `fetchGrokUsable(ctx, apiKey, cfg)` | `:279-315` | `curateFromCatalog(StaticGrok, u, isUsableGrokModel, RankGrokModel)` |
| `fetchOpencodeUsable(ctx, gateway, apiKey, cfg)` | `:332-373` | `curateFromCatalog(staticOpencodeCatalog(gateway), u, isUsableOpencodeModel, RankOpencodeModel)` |
| `fetchHuggingFaceUsable(ctx, apiKey, cfg)` | `:394-485` | `curateFromCatalog(StaticHuggingFace, u, isUsableHuggingFaceModel, nil)` |
| `kiloUsable(entries []kiloCatalogEntry) []string` over the existing `fetchKiloCatalog` | `:578-611` | `curateFromCatalog(StaticKilo, u, isUsableKiloModel, nil)` |
| `fetchOllamaNames(ctx, cfg)` | `:215-248` | none (§1.2 Ollama) |

`kiloUsable` keeps the line `if m.MayTrainOnYourPrompts { // POLICY — see isUsableKiloModel`
verbatim (mutation anchor). Every lister's static substitute stays
`StaticModels(provider)` (`StaticModels(gateway)` for OpenCode), which already
returns a fresh copy (`models_catalog.go:169-195`).

`modelCatalogFor` special cases, locked:

* **OpenCode:** call `opencodeBaseURL(gateway)` first. On error return
  `ModelCatalog{}, err` unchanged (today's `discovery.go:333-336`).
* **Ollama:** `fetchOllamaNames` errors are returned unchanged
  (`discovery.go:223`, `:228`, `:233`, `:242`). On success:

  ```go
  recommended := names
  if len(names) > MaxListedModels {
  	recommended = slices.Clone(names[:MaxListedModels])
  }
  return ModelCatalog{Recommended: recommended, Usable: names, Live: true}, nil
  ```
* **Unknown provider:** `fmt.Errorf("unsupported provider for model listing: %s", providerName)`,
  today's text.
* If `nilerr` reports `return catalogFrom(usable, err, …), nil`, annotate that
  line with `//nolint:nilerr // degrade-to-static contract (MADR 0009 §1)`,
  following the precedent at `discovery.go:574`. This is pre-approved and is
  not a deviation.

`ListModelCatalogWithSource` keeps today's order (`discovery.go:28-41`): apply
options, add the 10s timeout, ChatGPT short-circuit, nil-source error, token
acquisition, then `modelCatalogFor`. The ChatGPT short-circuit returns
`staticCatalog(slices.Clone(StaticOpenAIChatGPT)), nil`.

~~The unexported `list*Models` functions stay~~ The unexported `list*Models`
functions called by a `DiscoverModels` stay (2026-09-26 deviation, §10:
`listOpenAIModels` and `listGrokModels` had no such caller and are deleted),
because each provider's `DiscoverModels` calls them directly (`gemini.go:298`, `claude.go:298`,
`opencode.go:310`, `kilo.go:200`, `huggingface.go:174`, `ollama.go:195`). Each
becomes a three-line wrapper that calls `modelCatalogFor` and returns
`.Recommended` (or `nil, err`). `listOpencodeModels` keeps its `gateway`
parameter.

### 1.3 Pagination (Gemini and Anthropic only)

* **Gemini.** Build each request URL with `net/url`:
  `baseURL + "/models?" + url.Values{"pageSize": {geminiListPageSize}}`, plus
  `pageToken` when continuing. Rename today's local `url` variable
  (`discovery.go:72`) to `endpoint` so it no longer shadows the package. Add
  `NextPageToken string \`json:"nextPageToken"\`` to the decoded struct. The
  loop is `for page := 1; page <= maxListingPages; page++ {`; after decoding
  and filtering a page, `if result.NextPageToken == "" {` returns the
  accumulated usable ids.
* **Anthropic.** `baseURL + "/v1/models?" + url.Values{"limit": {claudeListPageLimit}}`,
  plus `after_id` when continuing. Add `HasMore bool \`json:"has_more"\`` and
  `LastID string \`json:"last_id"\``. Same loop header. After a page,
  `if !result.HasMore {` returns the accumulated ids. `if result.LastID == "" {`
  returns `fmt.Errorf("model listing: has_more without last_id")`.
* **Bound.** Falling out of the loop returns
  `fmt.Errorf("model listing: more than %d pages", maxListingPages)`.
* **Failure.** Any page's request-build, transport, non-200 or decode failure
  returns an error. `catalogFrom` turns every one of these into the static
  catalog with `Live == false` (MADR §2). Headers are unchanged:
  `x-goog-api-key`, `x-api-key`, `anthropic-version: 2023-06-01`. The API key
  never enters the query string: `TestGemini_KeyInHeaderNotURL`
  (`gemini_test.go:13-36`) rejects `key=` in a query, and the page parameters
  are `pageSize` and `pageToken`.

### 1.4 Matcher (`llmprovider/model_matcher.go`)

```go
const (
	scoreExact          = 6000
	scoreIDPrefix       = 5000
	scoreIDSubstring    = 4000
	scoreLabelSubstring = 3000
	scoreTokenPrefix    = 2000
	scoreSubsequence    = 1000
	scoreGlob           = 1
	maxSubsequenceBonus = 999
)
```

Algorithm, locked. Appendix C proves every test expectation in §5 against a
reference implementation of exactly this.

1. `q := strings.ToLower(strings.TrimSpace(query))`. If `q == ""`, return nil.
2. Walk `models` in order. Skip an id whose `strings.ToLower` form was already
   seen, using the line `if _, dup := seen[key]; dup {` (mutation anchor).
   `Label` is `ModelLabel(provider, id)`.
3. `if strings.ContainsAny(q, "*?") {` (mutation anchor): the **glob path**.
   Build the pattern rune by rune: `*` becomes `.*`, `?` becomes `.`, and
   anything else is `regexp.QuoteMeta(string(r))`. Then
   `pattern := "(?i)^" + b.String() + "$"` (mutation anchor). Compile with
   `regexp.Compile`; if that returns an error (impossible for quoted input),
   return nil. Keep, in input order, each id where the pattern matches the id or
   the label, with `Score: scoreGlob`. Return without sorting.
4. Otherwise the **fuzzy path**. Separators are `isModelSeparator(r) =
   strings.ContainsRune("-_/.:~", r) || unicode.IsSpace(r)`; tokens are
   `strings.FieldsFunc(strings.ToLower(s), isModelSeparator)`. With `lowerID`
   and `lowerLabel`, the first rule that holds sets the score:
   1. `lowerID == q` gives `scoreExact`;
   2. `strings.HasPrefix(lowerID, q)` gives `scoreIDPrefix`;
   3. `strings.Contains(lowerID, q)` gives `scoreIDSubstring`;
   4. `strings.Contains(lowerLabel, q)` gives `scoreLabelSubstring`;
   5. the query has at least one token, and every query token is a
      `strings.HasPrefix` of some id or label token, gives `scoreTokenPrefix`;
   6. `if bonus, ok := subsequenceBonus(lowerID, q); ok {` (mutation anchor)
      gives `scoreSubsequence + bonus`;
   7. otherwise the id is dropped.
5. `subsequenceBonus(lowerID, q)`: the needle is the runes of `q` that are not
   separators. An empty needle returns `0, false`. Greedy leftmost match over
   the runes of `lowerID`, with `prev := -2`. Each matched rune adds 10 when
   its index is `prev+1`, and 5 when it is index 0 or follows a separator. It
   returns `min(bonus, maxSubsequenceBonus), true` only if the whole needle
   matched.
6. Sort the fuzzy results, locked:

   ```go
   slices.SortStableFunc(out, func(a, b ModelMatch) int {
   	if c := cmp.Compare(b.Score, a.Score); c != 0 {
   		return c
   	}
   	if c := cmp.Compare(len(a.ID), len(b.ID)); c != 0 {
   		return c
   	}
   	return strings.Compare(a.ID, b.ID)
   })
   ```

`SearchModels` does no usability filtering (MADR §3).

### 1.5 Wizard (`wizard/model_select.go`, `wizard/configure.go`)

Constants in `model_select.go` (`otherModelLabel` moves here from
`configure.go:306-309`, comment included):

```go
const (
	otherModelLabel      = "Other (enter a model id)"
	searchAgainLabel     = "Search again"
	currentModelDetail   = "current"
	chooseModelTitle     = "Choose a %s model:"
	searchModelsPrompt   = "Search models (blank for recommended)"
	searchFallbackPrompt = "Search fallback models (blank for recommended)"
	chooseFallbacksTitle = "Choose fallback models (optional):"
	searchFallbacksTitle = "Choose fallback models:"
	moreFallbacksPrompt  = "Search for more fallback models?"
	maxSearchResults     = 20
)
```

Notices, locked text:

| When | Call |
|---|---|
| no match | `p.Notify(LevelWarn, "no %s models match %q", d.Label, q)` |
| more than 20 matches | `p.Notify(LevelInfo, "showing %d of %d matches; refine the search to narrow them", maxSearchResults, len(matches))` |
| listing substituted static | `p.Notify(LevelInfo, "live model listing for %s is unavailable; search covers the built-in catalog only", d.Label)` |
| listing error (unchanged) | `p.Notify(LevelWarn, "could not list models for %s (%v); using the built-in catalog", d.Label, err)` |

Signatures, locked:

```go
func discoverModels(ctx context.Context, p Prompter, d llmprovider.ProviderDescriptor, res Result, source llmprovider.TokenSource, o Options) llmprovider.ModelCatalog
func selectModel(p Prompter, d llmprovider.ProviderDescriptor, cat llmprovider.ModelCatalog, o Options) (string, error)
func selectRecommended(p Prompter, d llmprovider.ProviderDescriptor, models []string, o Options) (string, error)
func enterModelID(p Prompter, o Options) (string, error)
func selectFallbacks(p Prompter, d llmprovider.ProviderDescriptor, cat llmprovider.ModelCatalog, primary string) ([]string, error)
func excludedIDs(primary string, chosen []string) map[string]struct{}
func without(models []string, exclude map[string]struct{}) []string
func capMatches(p Prompter, matches []llmprovider.ModelMatch) []llmprovider.ModelMatch
func matchChoices(matches []llmprovider.ModelMatch) []Choice
```

`discoverModels` (in `configure.go`) returns:

* `chatGPT := res.Kind == CredOAuth && d.ID == llmprovider.ProviderOpenAI`
  (today's condition, `configure.go:266`). `static` is
  `slices.Clone(StaticOpenAIChatGPT)` when `chatGPT`, else `d.StaticModels`.
  `fallback := llmprovider.ModelCatalog{Recommended: static, Usable: static}`.
* `!o.Discover`: `fallback`, with no notice.
* Otherwise: today's timeout and `WithBaseURL` handling, then
  `ListModelCatalogWithSource`. On error, today's warning, then `fallback`. If
  `len(cat.Recommended) == 0`, return `fallback` (today's
  `err == nil && len(listed) == 0` path). Then
  `if !cat.Live && !chatGPT {` (mutation anchor) posts the static notice, and
  `cat` is returned.

`ConfigureLLM` changes only in its model section (`configure.go:132-157`):
`cat := discoverModels(…)`; the empty-catalog manual path keys on
`len(cat.Recommended) == 0`; `selectModel(p, d, cat, o)`; and
`selectFallbacks(p, d, cat, res.Model)`.

`selectModel` loop:

1. `q, err := p.Input(searchModelsPrompt, "")`; wrap the error as
   `"search models: %w"`; `q = strings.TrimSpace(q)`.
2. If `q == ""`, return `selectRecommended(p, d, cat.Recommended, o)`.
3. `matches := llmprovider.SearchModels(d.ID, cat.Usable, q)` (mutation
   anchor). If there are none, post the no-match notice and `continue`.
4. `shown := capMatches(p, matches)`. `Select(fmt.Sprintf(chooseModelTitle, d.Label), …, 0)`
   over `matchChoices(shown)`, then `{Label: searchAgainLabel}`, then
   `{Label: otherModelLabel}`. An index below `len(shown)` returns
   `shown[idx].ID`; `len(shown)` continues the loop; the last index returns
   `enterModelID(p, o)`.

`selectRecommended`: choices are `modelChoices(d.ID, models)`. The default is
the last index `i` with `models[i] == o.Existing.Model` (today's loop,
`configure.go:312-317`), and `listed` records whether one was found. Then
`if !listed && o.Existing.Provider == d.ID && o.Existing.Model != "" {`
(mutation anchor) appends
`Choice{Label: llmprovider.ModelLabel(d.ID, o.Existing.Model), Detail: currentModelDetail}`
and makes it the default. `otherModelLabel` always comes last. An index below
`len(models)` returns `models[idx]`; the current row returns
`o.Existing.Model`; the last index returns `enterModelID`.

`enterModelID`: today's code (`configure.go:323-331`):
`Input("Model id", o.Existing.Model)`, wrapped `"enter model: %w"`, and an empty
result returns `fmt.Errorf("wizard: no model entered")`.

`selectFallbacks` loop, with `var chosen []string` and
`exclude := excludedIDs(primary, chosen)` (mutation anchor) at the top of each
round:

1. `recs, usable := without(cat.Recommended, exclude), without(cat.Usable, exclude)`.
   If both are empty, return `chosen, nil` with no prompt.
2. `q := TrimSpace(Input(searchFallbackPrompt, ""))`, wrapped
   `"search fallback models: %w"`.
3. **Blank:** if `recs` is empty, return `chosen, nil`. Otherwise
   `MultiSelect(chooseFallbacksTitle, modelChoices(d.ID, recs), nil)`; if
   `chosen == nil`, set it to `[]string{}`; append `recs[i]` for each index
   `0 <= i < len(recs)`; then
   `return chosen, nil // a blank round ends the loop` (mutation anchor).
4. **Non-blank:** `matches := llmprovider.SearchModels(d.ID, usable, q)`. If
   there are none, post the no-match notice and `continue`. Otherwise
   `shown := capMatches(p, matches)`, then
   `MultiSelect(searchFallbacksTitle, matchChoices(shown), nil)`; set a nil
   `chosen` to `[]string{}`; append `shown[i].ID` for valid indices.
5. `more, err := p.Confirm(moreFallbacksPrompt, false)` (mutation anchor). If
   `!more`, return `chosen, nil`; otherwise `continue`.

Return shape, preserved from today (`configure.go:344-357`): nil when no
`MultiSelect` was shown, and a non-nil (possibly empty) slice once one was.

`capMatches` posts the cap notice when `len(matches) > maxSearchResults` and
returns `matches[:maxSearchResults]`; otherwise it returns `matches` unchanged.
`matchChoices` returns `Choice{Label: m.Label}` per match. No function mutates a
slice held by the catalog.

## 2. Phase sequencing

| Phase | Deliverable | New files | Modified files |
|---|---|---|---|
| 0 | MADR accepted, plan in progress, MADR 0003 annotated | — | `docs/0009-MADR-live-catalog-model-search.md`, `docs/0009-PLAN-live-catalog-model-search.md`, `docs/0003-MADR-add-gateway-llm-providers.md` |
| 0b | Zen/Go key header per route | — | `llmprovider/opencode.go`, `llmprovider/opencode_test.go`, `llmprovider/live_gateways_test.go` |
| 1 | `ModelCatalog`, `ListModelCatalog*`, lister split | `llmprovider/discovery_catalog_test.go` | `llmprovider/discovery.go` |
| 1b | Kilo/Hugging Face input contains text | — | `llmprovider/discovery.go`, `llmprovider/discovery_test.go`, `llmprovider/discovery_catalog_test.go` |
| 2 | Gemini and Anthropic pagination | `llmprovider/discovery_pagination_test.go` | `llmprovider/discovery.go` |
| 3 | `SearchModels` | `llmprovider/model_matcher.go`, `llmprovider/model_matcher_test.go` | — |
| 4 | Wizard search-then-select | `wizard/model_select.go`, `wizard/model_select_test.go` | `wizard/configure.go`, `wizard/configure_test.go`, `wizard/fake_prompter_test.go` |
| 5 | README and close-out | — | `README.md`, `docs/0009-PLAN-live-catalog-model-search.md` |

Dependencies: 1→1b→2; 3 is independent of 1–2; 4 needs 1, 1b and 3; 5 needs 4.
Phase 0b is independent of Phases 1–5. Execute in the order 0, 0b, 1, 1b, 2,
3, 4, 5.

## Phase 0 — Record acceptance

1. Set the MADR frontmatter to `status: accepted` and `date:` the execution
   date. Set this plan to `status: in-progress` with the same date.
2. In `docs/0003-MADR-add-gateway-llm-providers.md`, insert after the title
   line a blockquote:
   `> **Partially superseded (<execution date>) by [0009-MADR-live-catalog-model-search.md](0009-MADR-live-catalog-model-search.md) §1b and §1c:** the Hugging Face and Kilo filter `input_modalities == ["text"]` is replaced by "input contains `text`", and the Zen/Go "Bearer on every route" rule is replaced by a per-route key header. The output filter, tools rule, training policy and ranking below stand.`
   Change nothing else in 0003. Its status stays `accepted`.
3. ~~`git add docs/0009-MADR-live-catalog-model-search.md docs/0009-PLAN-live-catalog-model-search.md docs/0003-MADR-add-gateway-llm-providers.md`,
   then `git commit --no-edit`.~~ **Widened by the 2026-09-26 deviation (§10):**
   stage every pending `docs/` change, then `git commit --no-edit`. The files
   are Markdown only, so the phase gate does not apply.

## Phase 0b — Zen/Go key header per route (MADR §1c)

Runs right after Phase 0 because it fixes a live defect. It touches no file
that Phases 1–5 change.

### Steps

1. **Red test first.** In `llmprovider/opencode_test.go`, rewrite
   `TestOpencode_KeyInHeader` (`:107-140`) as a table:

   | model | fixture | key header | value |
   |---|---|---|---|
   | `gpt-5.5` | `fxOpencodeResponses` | `Authorization` | `Bearer test-key` |
   | `claude-sonnet-5` | `fxOpencodeMessages` | `x-api-key` | `test-key` |
   | `gemini-3.7-flash` | `fxOpencodeGoogle` | `x-goog-api-key` | `test-key` |
   | `deepseek-v4-pro` | `fxOpencodeChat` | `Authorization` | `Bearer test-key` |

   For each row the handler asserts:
   * the named header has the given value;
   * the other two of `Authorization`, `x-api-key` and `x-goog-api-key` are
     absent;
   * `r.URL.RawQuery == ""`.

   Replace the doc comment with: "TestOpencode_KeyInHeader pins MADR 0009 §1c:
   each Zen/Go route reads the key from its vendor's header, and no route
   reads the others." Then run
   `go test -count=1 -run '^TestOpencode_KeyInHeader$' ./llmprovider > "$SCRATCH/p0b-red.log" 2>&1`.
   It must fail on the `claude-sonnet-5` and `gemini-3.7-flash` subtests.
   Copy those `--- FAIL` lines to §11.
2. **Live drift test.** In `llmprovider/live_gateways_test.go` (build tag
   `live_gateways`), add `TestLive_OpencodeKeyHeaderPerRoute`:
   * It iterates this locked table:
     ```go
     routes := []struct{ name, path, body, right string }{
     	{"messages", "/messages", `{"model":"claude-haiku-4-5","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, "x-api-key"},
     	{"google", "/models/gemini-3.7-flash:generateContent", `{"contents":[{"parts":[{"text":"hi"}]}]}`, "x-goog-api-key"},
     }
     ```
   * For each route it POSTs to `opencodeZenBaseURL + path` twice with the key
     `sk-bogus-000`: once as `Authorization: Bearer sk-bogus-000`, once in the
     `right` header.
   * It requires HTTP 401 with a body containing `Missing API key.` for the
     first request and `Invalid API key.` for the second. On a failure it
     reports `DRIFT:` and the body.
   * It skips on a transport error or a non-401 status, as the file's other
     tests do (`live_gateways_test.go:47`, `:79`).
3. **Implement.** In `llmprovider/opencode.go`, add
   ```go
   // opencodeKeyHeader returns the header the Zen/Go server reads the key from
   // on route r. Each route parses only its vendor's header (MADR 0009 §1c).
   func opencodeKeyHeader(r OpencodeRoute, key string) (name, value string) {
   	switch r {
   	case OpencodeRouteMessages:
   		return "x-api-key", key
   	case OpencodeRouteGoogle:
   		return "x-goog-api-key", key
   	default:
   		return oauthAuthorizationHeader, "Bearer " + key
   	}
   }
   ```
   Replace `opencode.go:259-262` (the comment and the `Authorization` `Set`)
   with:
   ```go
   	// Each route reads the key from its vendor's header (MADR 0009 §1c); the
   	// key stays in a header, never the URL.
   	name, value := opencodeKeyHeader(p.route, p.apiKey)
   	req.Header.Set(name, value)
   ```
   `oauthAuthorizationHeader` is the existing constant (`oauth_session.go:16`).
   Update the file's doc comments that say "Bearer on every route".
4. Re-run step 1's command, which must pass. `go test -count=1 ./llmprovider`
   must pass. `go vet -tags live_gateways ./llmprovider` must exit 0, which
   compiles the live file.
5. **Live run (recorded).**
   `go test -count=1 -tags live_gateways -run '^TestLive_OpencodeKeyHeaderPerRoute$' -v ./llmprovider > "$SCRATCH/p0b-live.log" 2>&1`.
   Record the result in §11:
   * **PASS:** proceed.
   * **SKIP:** record why; proceed.
   * **FAIL:** a deviation (§10); stop.
6. Mutation proofs (below), then the phase gate on
   `llmprovider/opencode.go llmprovider/opencode_test.go llmprovider/live_gateways_test.go`
   **plus `llmprovider/opencode_route.go`** (2026-09-26 deviation, §10).
7. Stage ~~those three files~~ those four files. `git commit --no-edit`.

### Mutation proofs (`$SCRATCH/p0b-mutations.json`)

| name | file | old → new | run | tags | expect |
|---|---|---|---|---|---|
| `p0b-bearer-messages` | `llmprovider/opencode.go` | `return "x-api-key", key` → `return oauthAuthorizationHeader, "Bearer " + key` | `^TestOpencode_KeyInHeader$` | — | fail |
| `p0b-bearer-google` | same | `return "x-goog-api-key", key` → `return oauthAuthorizationHeader, "Bearer " + key` | `^TestOpencode_KeyInHeader$` | — | fail |
| `p0b-live-wrong-right` | `llmprovider/live_gateways_test.go` | `` `{"contents":[{"parts":[{"text":"hi"}]}]}`, "x-goog-api-key"}, `` → `` `{"contents":[{"parts":[{"text":"hi"}]}]}`, "x-api-key"}, `` | `^TestLive_OpencodeKeyHeaderPerRoute$` | `live_gateways` | fail |
| `p0b-control` | `llmprovider/opencode.go` | `// on route r. Each route parses only its vendor's header (MADR 0009 §1c).` → the same + ` (control)` | `^TestOpencode_KeyInHeader$` | — | pass |

`pkg` is `./llmprovider` for all rows.
* `p0b-live-wrong-right` needs network access. If it reports SKIP instead of
  FAIL, record that it could not be proven and why.
* The runner's `tags` field was added and re-proven on 2026-09-26 (Appendix B).

## Phase 1 — One listing, two views

### Steps

1. **Red tests first.** Create `llmprovider/discovery_catalog_test.go` with the
   tests below. Run
   `go test -count=1 -run 'TestListModelCatalog' ./llmprovider > "$SCRATCH/p1-red.log" 2>&1`.
   It must fail to compile (`undefined: ListModelCatalog`). Copy that line to
   §11.
2. Implement §1.1–§1.2 in `llmprovider/discovery.go`, with no pagination yet.
3. Re-run step 1's command, which must pass. Run
   `go test -count=1 ./llmprovider`, which must pass with **no existing test
   edited**.
4. Mutation proofs (below), then the phase gate on
   `llmprovider/discovery.go llmprovider/discovery_catalog_test.go`.
5. Stage exactly those two files. `git commit --no-edit`.

### Tests (`llmprovider/discovery_catalog_test.go`)

Test helpers local to this file: `openAIStyleListing(ids ...string) string`
returns `{"data":[{"id":…},…]}`; `ollamaTags(names ...string) string` returns
`{"models":[{"name":…},…]}`; `hfListing(n int)` and `kiloListing(n int)`
generate `n` text→text entries, live and tool-capable, named `org/m-01` onward,
with descending throughput and ascending price respectively. The existing
fixtures `opencodeListingFixture`, `hfListingFixture` and `kiloListingFixture`
(`discovery_test.go:274`, `:358`, `:441`) are reused.

| Test | Asserts |
|---|---|
| `TestListModelCatalog_RecommendedMatchesListAvailable` | For all nine providers against a good fixture: `slices.Equal(cat.Recommended, ListAvailableModels(...))` and `cat.Live`. |
| `TestListModelCatalog_OpenAIRecommendedIsCurated` | Fixture ids in order: `gpt-5.1, o3, gpt-4o, o4-mini, gpt-4.1, gpt-4o-mini, gpt-4.1-nano, gpt-4.1-mini`. `Recommended == StaticOpenAI` exactly (all six hit, catalog order), `Usable` equals the fixture order (8 ids), `Live`. |
| `TestListModelCatalog_UsableIsUncapped` | `hfListing(8)`, `kiloListing(8)`, and an OpenCode Zen fixture of 8 usable ids each give `len(Usable) == 8` and `len(Recommended) == MaxListedModels`. |
| `TestListModelCatalog_RecommendedSubsetOfUsable` | For every good fixture, each `Recommended` id appears in `Usable` (exact string). |
| `TestListModelCatalog_FiltersStillApply` | Absent from `Usable`: Kilo `org/trains`, `org/no-tools`; Hugging Face `org/not-live`; OpenCode `deepseek-v4-flash-vision-exp`; Gemini `gemini-embedding-001` (`embedContent`) and `gemini-3.5-flash-preview-09-2026`. |
| `TestListModelCatalog_OllamaSplit` | Eight names give `Usable` = all eight in order and `Recommended` = the first six. Then `_ = append(cat.Recommended, "sentinel")` must leave `cat.Usable[6]` unchanged. |
| `TestListModelCatalog_OllamaEmptyIsLive` | `{"models":[]}` gives a nil error, `Live`, and both views empty. |
| `TestListModelCatalog_LiveFlag` | For the eight providers with a static catalog, a 500 fixture gives a nil error, `!Live`, and `Recommended` and `Usable` both equal to `StaticModels(p)`. |
| `TestListModelCatalogWithSource_ChatGPTDoesNotHTTP` | Mirrors `discovery_test.go:96-129`: zero HTTP hits; both views equal `StaticOpenAIChatGPT`; `!Live`; mutating `Recommended[0]` does not change a second call's result. |
| `TestListModelCatalog_Errors` | Unsupported provider, Ollama 404 and a nil `TokenSource` each return a non-nil error. |

### Mutation proofs (Appendix B spec `$SCRATCH/p1-mutations.json`)

| name | file | old → new | run | expect |
|---|---|---|---|---|
| `p1-skip-curation` | `llmprovider/discovery.go` | `recommended := curate(usable)` → `recommended := usable[:min(len(usable), MaxListedModels)]` | `^TestListModelCatalog_OpenAIRecommendedIsCurated$` | fail |
| `p1-static-live` | same | `return ModelCatalog{Recommended: static, Usable: slices.Clone(static), Live: false}` → `…, Live: true}` | `^TestListModelCatalog_LiveFlag$` | fail |
| `p1-ollama-alias` | same | `recommended = slices.Clone(names[:MaxListedModels])` → `recommended = names[:MaxListedModels]` | `^TestListModelCatalog_OllamaSplit$` | fail |
| `p1-kilo-policy` | same | `if m.MayTrainOnYourPrompts { // POLICY — see isUsableKiloModel` → `if false { // POLICY — see isUsableKiloModel` | `^TestListModelCatalog_FiltersStillApply$` | fail |
| `p1-control` | same | the `catalogFrom` doc comment's first line → the same line + ` (control)` | `^TestListModelCatalog` | pass |

## Phase 1b — Input modality for Kilo and Hugging Face (MADR §1b)

### Steps

1. **Rewrite the pinned tests first** in `llmprovider/discovery_test.go`:
   * Append two entries to `hfListingFixture` (`:358`):
     ```json
     {"id":"org/painter","architecture":{"input_modalities":["text"],"output_modalities":["text","image"]},
      "providers":[{"provider":"a","status":"live","supports_tools":true,"throughput":500,"first_token_latency_ms":10}]},
     {"id":"org/listener","architecture":{"input_modalities":["audio"],"output_modalities":["text"]},
      "providers":[{"provider":"a","status":"live","supports_tools":true,"throughput":400,"first_token_latency_ms":10}]}
     ```
   * Append two entries to `kiloListingFixture` (`:441`):
     ```json
     {"id":"org/painter","architecture":{"input_modalities":["text"],"output_modalities":["text","image"]},
      "pricing":{"completion":"0.0000001"},"supported_parameters":["tools"],"mayTrainOnYourPrompts":false},
     {"id":"org/listener","architecture":{"input_modalities":["audio"],"output_modalities":["text"]},
      "pricing":{"completion":"0.0000001"},"supported_parameters":["tools"],"mayTrainOnYourPrompts":false}
     ```
     `org/painter` avoids the substring `image`, which `isUsableKiloModel`
     denies by id, so the rejection is proven to come from the output rule.
   * `TestListHuggingFaceModels_MetadataCuration` (`:374-401`): the `org/vlm`
     branch becomes an error when `org/vlm` is **absent**. Add `org/painter`
     and `org/listener` as must-be-absent. Set
     `want := []string{"org/vlm", "org/fast", "org/mid", "org/slow"}`.
   * `TestListKiloModels_MetadataCuration` (`:458-487`): drop the
     `case "org/vlm":` rejection. Add `org/painter` and `org/listener` to the
     rejected cases. Set
     `want := []string{"org/cheap", "org/vlm", "org/dear", "kilo-auto/variable"}`.
   * Update both fixtures' doc comments (`:355-357`, `:438-440`) to say the
     vision-language entry is admitted and the two new entries are rejected.
   * In `llmprovider/discovery_catalog_test.go` add
     `TestListModelCatalog_InputModalityContainsText`: for both providers,
     `org/vlm` is in `Usable`, and `org/painter` and `org/listener` are not.
2. **Red run.**
   `go test -count=1 -run 'MetadataCuration|InputModalityContainsText' ./llmprovider > "$SCRATCH/p1b-red.log" 2>&1`
   must fail on assertions (`org/vlm` is still dropped). Copy each `--- FAIL`
   line to §11.
3. **Implement.** In `llmprovider/discovery.go`, add beside `onlyText`:
   ```go
   // hasText reports whether a modality list includes "text". A model that also
   // accepts images or files still serves a text prompt (MADR 0009 §1b).
   func hasText(mods []string) bool { return slices.Contains(mods, jsonKeyText) }
   ```
   In the Hugging Face and Kilo fetch-and-filter steps, change the input half of
   the modality check from `!onlyText(m.Architecture.InputModalities)` to
   `!hasText(m.Architecture.InputModalities)`. The output half stays
   `!onlyText(m.Architecture.OutputModalities)`. Update the `listHuggingFaceModels`
   and `listKiloModels` doc comments that describe the filter.
4. Re-run step 2's command, which must pass. Then `go test -count=1 ./llmprovider`.
   Every other pre-existing test must pass unmodified.
5. **Live evidence (recorded, not gated).** Re-run the scratch program from
   Appendix C (a `replace` module calling `ListModelCatalog` anonymously for
   `kilo` and `huggingface`). Record in §11 whether `kilo-auto/small`,
   `kilo-auto/efficient` and `kilo-auto/balanced` appear in Kilo's `Usable`,
   and `zai-org/GLM-5.3-Flash` in Hugging Face's. Live catalogs drift, so a
   difference is a deviation to report (§10), not a reason to change the code.
6. Mutation proofs, then the phase gate on
   `llmprovider/discovery.go llmprovider/discovery_test.go llmprovider/discovery_catalog_test.go`.
7. Stage those three files. `git commit --no-edit`.

### Mutation proofs (`$SCRATCH/p1b-mutations.json`)

| name | old → new | run | expect |
|---|---|---|---|
| `p1b-exact-text-input` | `func hasText(mods []string) bool { return slices.Contains(mods, jsonKeyText) }` → `func hasText(mods []string) bool { return onlyText(mods) }` | `MetadataCuration\|InputModalityContainsText` | fail |
| `p1b-output-relaxed` | `func onlyText(mods []string) bool { return len(mods) == 1 && mods[0] == jsonKeyText }` → `func onlyText(mods []string) bool { return slices.Contains(mods, jsonKeyText) }` | `MetadataCuration\|InputModalityContainsText` | fail |
| `p1b-control` | the `hasText` doc comment's first line → the same line + ` (control)` | `MetadataCuration\|InputModalityContainsText` | pass |

All on `llmprovider/discovery.go`, `pkg` `./llmprovider`. After Phase 1b,
`onlyText` guards only the output half of both checks, so `p1b-output-relaxed`
admits `org/painter` (text+image output) and the tests must fail. The
`onlyText` anchor is today's line `discovery.go:384`, which this plan leaves
unchanged. The two modality checks are textually identical, which is why
neither is used as an anchor: the runner rejects any anchor that occurs more
than once.

## Phase 2 — Pagination

### Steps

1. **Red tests first.** Create `llmprovider/discovery_pagination_test.go`. Run
   `go test -count=1 -run 'TestListModelCatalog_(Gemini|Claude|Pagination|SinglePage)' ./llmprovider > "$SCRATCH/p2-red.log" 2>&1`.
   The two-page, second-page-failure, bound and page-size tests must **fail on
   assertions**: Phase 1 fetches one page and sends no page parameters. Copy
   each `--- FAIL` line to §11.
2. Implement §1.3.
3. Re-run step 1's command, which must pass, then `go test -count=1 ./llmprovider`.
4. Mutation proofs, then the phase gate on
   `llmprovider/discovery.go llmprovider/discovery_pagination_test.go`.
5. Stage those two files and commit.

### Tests (`llmprovider/discovery_pagination_test.go`)

Every handler counts its requests and records `r.URL.Query()`.

| Test | Fixture | Asserts |
|---|---|---|
| `TestListModelCatalog_GeminiFollowsNextPageToken` | page 1 (no `pageToken`): `models/gemini-3.7-flash`, `nextPageToken: "p2"`; page 2 (`pageToken=p2`): `models/gemini-2.5-flash`, no token | `Usable == [gemini-3.7-flash gemini-2.5-flash]`, `Live`, 2 requests, `pageSize=1000` on both |
| `TestListModelCatalog_ClaudeFollowsHasMore` | page 1: `claude-sonnet-5`, `has_more: true`, `last_id: "claude-sonnet-5"`; page 2 (`after_id=claude-sonnet-5`): `claude-haiku-4-5`, `has_more: false` | `Usable == [claude-sonnet-5 claude-haiku-4-5]`, `Live`, 2 requests, `limit=1000` on both |
| `TestListModelCatalog_GeminiSecondPageFailureDegrades` | page 1 good with token; page 2 returns 500 | nil error, `!Live`, `Usable == StaticGemini` |
| `TestListModelCatalog_ClaudeSecondPageFailureDegrades` | page 1 good with `has_more`; page 2 returns 500 | nil error, `!Live`, `Usable == StaticClaude` |
| `TestListModelCatalog_PaginationIsBounded` | subtests `gemini` (always `nextPageToken: "again"`) and `claude` (always `has_more: true, last_id: "x"`) | exactly ~~`maxListingPages`~~ a literal 10 (2026-09-26 deviation, §10) requests, `!Live`, static |
| `TestListModelCatalog_ClaudeHasMoreWithoutLastIDDegrades` | `has_more: true`, `last_id` absent | 1 request, `!Live`, static |
| `TestListModelCatalog_SinglePageRequestsMaxPageSize` | one page each, no continuation | exactly 1 request each; Gemini `pageSize=1000`, no `pageToken`; Claude `limit=1000`, no `after_id` |

### Mutation proofs (`$SCRATCH/p2-mutations.json`)

| name | old → new | run | expect |
|---|---|---|---|
| `p2-gemini-one-page` | `if result.NextPageToken == "" {` → `if true {` | `^TestListModelCatalog_GeminiFollowsNextPageToken$` | fail |
| `p2-claude-one-page` | `if !result.HasMore {` → `if true {` | `^TestListModelCatalog_ClaudeFollowsHasMore$` | fail |
| `p2-unbounded` | `maxListingPages     = 10` → `maxListingPages     = 1000` | `^TestListModelCatalog_PaginationIsBounded$` | fail |
| `p2-control` | `claudeListPageLimit = "1000"` → `claudeListPageLimit = "1000" // control` | `^TestListModelCatalog_` | pass |

All on `llmprovider/discovery.go`, `pkg` `./llmprovider`. The anchors include
the gofmt-aligned spacing of the §1.2 const block. If gofmt aligns it
differently, record the exact anchor used in §11; that is not a deviation.

## Phase 3 — Matcher

### Steps

1. **Red tests first.** Create `llmprovider/model_matcher_test.go`. `go test
   -count=1 -run TestSearchModels ./llmprovider` must fail to compile
   (`undefined: SearchModels`). Record the line in §11.
2. Implement §1.4 in `llmprovider/model_matcher.go`. Its package doc is
   unchanged; the file starts with a short comment naming MADR 0009 §3.
3. Re-run until it passes, then `go test -count=1 ./llmprovider`.
4. Mutation proofs, then the phase gate on
   `llmprovider/model_matcher.go llmprovider/model_matcher_test.go`.
5. Stage those two files and commit.

### Tests (`llmprovider/model_matcher_test.go`)

The shared fixture, in this order:

```go
var matcherFixture = []string{
	"gemini-3.5-flash", "gemini-3.5-flash-lite", "gemini-2.5-pro",
	"claude-sonnet-5", "claude-haiku-4-5",
	"meta-llama/Llama-3.1-8B-Instruct", "meta-llama/kilo-auto",
	"kilo-auto/balanced", "kilo-auto/free", "kilo-auto/fast", "kilo-auto-legacy",
	"gpt-4.1-mini", "chatgpt-4o-latest", "gpt-5.4",
	"llama3:latest", "fireworks/llama-ash",
}
```

Expected results, each derived from the reference implementation (Appendix C):

| Test | Query | Expected |
|---|---|---|
| `TestSearchModels_GlobCrossesSlash` | `kilo-auto/*` | ids exactly `[kilo-auto/balanced kilo-auto/free kilo-auto/fast]` |
| `TestSearchModels_GlobMatchesAcrossOrg` | `*llama*` | contains `meta-llama/Llama-3.1-8B-Instruct` |
| `TestSearchModels_GlobIsAnchored` | `gpt-4*` | ids exactly `[gpt-4.1-mini]` (no `chatgpt-4o-latest`) |
| `TestSearchModels_GlobKeepsInputOrder` | `*` | ids equal `matcherFixture`, every `Score == 1` |
| `TestSearchModels_SubstringOutranksSubsequence` | `flash` | ids exactly `[gemini-3.5-flash gemini-3.5-flash-lite fireworks/llama-ash]`, scores `4000, 4000, 1020` |
| `TestSearchModels_TokenPrefix` | `llama 8b`; `llama3 latest` | `[meta-llama/Llama-3.1-8B-Instruct]` score 2000; `[llama3:latest]` score 2000 |
| `TestSearchModels_Subsequence` | `sonet` | `[claude-sonnet-5]` score 1035 |
| `TestSearchModels_NoSubsequenceOverLabels` | `hspd` | empty. The test also asserts `hspd` would match `ModelLabel("", "claude-haiku-4-5")` by subsequence, so it fails if that label changes. |
| `TestSearchModels_EmptyQuery` | `""`, `"   "` | nil |
| `TestSearchModels_TieBreak` | `auto` | ids exactly `[kilo-auto/fast kilo-auto/free kilo-auto-legacy kilo-auto/balanced meta-llama/kilo-auto]` |
| `TestSearchModels_Tiers` | `gpt-5.4` over `[xgpt-5.4 gpt-5.4-mini gpt-5.4]` | `[gpt-5.4 gpt-5.4-mini xgpt-5.4]`, scores `6000, 5000, 4000` |
| `TestSearchModels_Dedupe` | `a` over `[a-b A-B a-b]` | ids exactly `[a-b]` |
| `TestSearchModels_LabelFromModelLabel` | `haiku` over `[claude-haiku-4-5]` | `Label == ModelLabel("", "claude-haiku-4-5")` |
| `TestSearchModels_SeparatorOnlyQuery` | `-` over `[ab a-b]` | ids exactly `[a-b]` (substring match only; no vacuous token or subsequence match) |

### Mutation proofs (`$SCRATCH/p3-mutations.json`)

| name | old → new | run | expect |
|---|---|---|---|
| `p3-glob-to-fuzzy` | `if strings.ContainsAny(q, "*?") {` → `if false {` | `^TestSearchModels_Glob` | fail |
| `p3-unanchored` | `pattern := "(?i)^" + b.String() + "$"` → `pattern := "(?i)" + b.String()` | `^TestSearchModels_GlobIsAnchored$` | fail |
| `p3-label-subsequence` | `if bonus, ok := subsequenceBonus(lowerID, q); ok {` → `if bonus, ok := subsequenceBonus(lowerLabel, q); ok {` | `^TestSearchModels_NoSubsequenceOverLabels$` | fail |
| `p3-no-length-tiebreak` | `if c := cmp.Compare(len(a.ID), len(b.ID)); c != 0 {` → `if c := 0; c != 0 {` | `^TestSearchModels_TieBreak$` | fail |
| `p3-no-dedupe` | `if _, dup := seen[key]; dup {` → `if _, dup := seen[key]; dup && false {` | `^TestSearchModels_Dedupe$` | fail |
| `p3-control` | `scoreGlob           = 1` → `scoreGlob           = 1 // control` | `^TestSearchModels_` | pass |

All on `llmprovider/model_matcher.go`, `pkg` `./llmprovider`.

## Phase 4 — Wizard

### Steps

1. **Test double.** In `wizard/fake_prompter_test.go`, add
   `seenSelectDefault []int`, appended in `Select`, and
   `seenMultiSelectItems [][]Choice`, appended in `MultiSelect`. No other
   behaviour changes.
2. **Existing scripts.** In `wizard/configure_test.go`:
   * `TestConfigureLLM_OtherModelEscapeHatch` (`:263-284`): change
     `inputs: []string{"my-custom-model"}` to
     `inputs: []string{"", "my-custom-model"}`. The first entry is the blank
     search.
   * `TestConfigureLLM_OffersEveryDescriptor` (`:217-236`): delete the line
     `inputs:  []string{"http://localhost:11434"},`. Provider index 0 is Gemini
     (`llmprovider/descriptor.go:72`), which has no endpoint prompt, so the
     line is already unused. Under the new flow it would become a search query
     that matches nothing (Appendix C).
3. **Red run.** Create `wizard/model_select_test.go`. Run
   `go test -count=1 ./wizard > "$SCRATCH/p4-red.log" 2>&1` and record the
   failures in §11. Expect a compile failure on the new constants. Before
   `model_select.go` exists, the step-2 edit alone also makes
   `TestConfigureLLM_OtherModelEscapeHatch` fail: today's flow reads `""` as the
   manual id and returns `wizard: no model entered`. To observe that assertion
   failure separately, run it before creating `model_select_test.go`.
4. Implement §1.5: `wizard/model_select.go` (new) and `wizard/configure.go`
   (`discoverModels`, the model section of `ConfigureLLM`, and removal of the
   old `selectModel`, `selectFallbacks` and `otherModelLabel`).
5. `go test -count=1 ./wizard` must pass, including every existing test not
   named in step 2, unmodified.
6. Mutation proofs, then the phase gate on
   `wizard/configure.go wizard/model_select.go wizard/model_select_test.go wizard/configure_test.go wizard/fake_prompter_test.go`.
7. Stage those five files and commit.

### Tests (`wizard/model_select_test.go`)

Fixtures:

* `zenSearchFixture` (OpenCode Zen listing), ids in order `gpt-5.4-nano,
  gemini-3.5-flash-lite, gpt-5.4-mini, claude-haiku-4-5, gemini-3.7-flash,
  kimi-k2.6, claude-sonnet-5, claude-opus-5`. The first six are
  `StaticOpencodeZen` (`models_catalog.go:68-75`), so `Recommended` is exactly
  those six in catalog order and `Usable` holds all eight.
* `zenManyFixture`: `m-01` through `m-25`.

A Zen run is scripted as `selects: {providerIdx(zen), …}`,
`inputs: {srv.URL, …}` (the endpoint prompt comes first, because Zen sets
`supportsBaseURL`, `descriptor.go:147`), and `secrets: {testKey}`, with
`Options{Discover: true, DiscoverLimit: 5 * time.Second}`. Claude runs use
`Discover: false` and its static catalog. No new test reaches the network.

| Test | Script (after provider, endpoint and key) | Asserts |
|---|---|---|
| `TestConfigureLLM_BlankSearchShowsRecommended` | Claude; no inputs; select 1 | `Model == StaticClaude[1]`; `seenInput` contains `searchModelsPrompt`; model menu has 7 rows, the last `otherModelLabel` |
| `TestConfigureLLM_SearchUsesLiveCorpus` | Zen; input `sonnet`; select 0 | `Model == "claude-sonnet-5"`, which is not in `StaticOpencodeZen`; menu rows `[ModelLabel(claude-sonnet-5), searchAgainLabel, otherModelLabel]` |
| `TestConfigureLLM_SearchNoMatchReturnsToSearch` | Claude; inputs `zzzz`, `""`; select 0 | `Model == StaticClaude[0]`; `seenNotify` contains `no Claude (Anthropic) models match "zzzz"` |
| `TestConfigureLLM_SearchAgain` | Claude; inputs `haiku`, `""`; selects 2, 1 | first menu `[claude-haiku-4-5, claude-3-5-haiku-latest, Search again, Other]` (by label); `Model == "claude-sonnet-5"` |
| `TestConfigureLLM_OtherFromSearchResults` | Claude; inputs `haiku`, `my-id`; select 3 | `Model == "my-id"` |
| `TestConfigureLLM_SearchResultsCapped` | Zen many; input `m-`; select 0 | 22 rows: `m-01`…`m-20`, Search again, Other; notice `showing 20 of 25 matches; refine the search to narrow them` exactly once |
| `TestConfigureLLM_CurrentModelListed` | Claude; `Existing{Provider: claude, Model: "claude-opus-5"}`; select 6 | row 6 is `{Label: "claude-opus-5", Detail: "current"}`; `seenSelectDefault[1] == 6`; row 7 is Other; `Model == "claude-opus-5"` |
| `TestConfigureLLM_CurrentModelOnlyForSameProvider` | Claude; `Existing{Provider: gemini, Model: "claude-opus-5"}`; select 0 | 7 rows; no `current` row; `seenSelectDefault[1] == 0` |
| `TestConfigureLLM_StaticCatalogNotice` | Zen against a 500 server; select 0 | `live model listing for OpenCode Zen is unavailable; search covers the built-in catalog only` exactly once; `Model == StaticOpencodeZen[0]` |
| `TestConfigureLLM_NoStaticNoticeWithoutDiscover` | Claude, `Discover: false` | no `seenNotify` entry contains `live model listing` |
| `TestConfigureLLM_ChatGPTNoStaticNotice` | construction copied from `auth_test.go:25-56` | no `live model listing` notice; `Model == StaticOpenAIChatGPT[0]` |
| `TestConfigureLLM_FallbackSearch` | Zen, `NeedFallbacks`; inputs `haiku`, `claude`; select 0; multi `{0,1}`; confirm false | `Model == "claude-haiku-4-5"`; the fallback menu is exactly `[claude-opus-5, ModelLabel(claude-sonnet-5)]`, so the primary is excluded; `Fallbacks == [claude-opus-5 claude-sonnet-5]`; `seenConfirm == [moreFallbacksPrompt]` |
| `TestConfigureLLM_FallbackSearchLoops` | as above but inputs `haiku`, `claude`, `""`; multis `{0}`, `{0}`; confirm true | `Fallbacks == [claude-opus-5 gpt-5.4-nano]`; the second menu has 5 rows, none `claude-haiku-4-5` or `claude-opus-5`; `len(seenConfirm) == 1` |
| `TestSelectFallbacks_ReturnShape` | direct calls | `{Rec: [a], Usable: [a]}` with primary `a` returns nil and prompts nothing; `{Rec: [a b], Usable: [a b]}` with primary `a` and multi `{}` returns a non-nil empty slice |

`TestConfigureLLM_Fallbacks` (`configure_test.go:165-185`) stays unmodified and
acts as the guard against a `Confirm` on the blank path, because `fakePrompter`
errors on an unscripted `Confirm` (`fake_prompter_test.go:76-78`).

### Mutation proofs (`$SCRATCH/p4-mutations.json`)

| name | file | old → new | run | expect |
|---|---|---|---|---|
| `p4-search-recommended` | `wizard/model_select.go` | `matches := llmprovider.SearchModels(d.ID, cat.Usable, q)` → `…cat.Recommended, q)` | `^TestConfigureLLM_SearchUsesLiveCorpus$` | fail |
| `p4-primary-not-excluded` | same | `exclude := excludedIDs(primary, chosen)` → `exclude := excludedIDs("", chosen)` | `^TestConfigureLLM_FallbackSearch$` | fail |
| `p4-no-confirm` | same | `more, err := p.Confirm(moreFallbacksPrompt, false)` → `more, err := false, error(nil)` | `^TestConfigureLLM_FallbackSearch$` | fail |
| `p4-confirm-on-blank` | same | `return chosen, nil // a blank round ends the loop` → `_, cerr := p.Confirm(moreFallbacksPrompt, false); return chosen, cerr // a blank round ends the loop` | `^TestConfigureLLM_Fallbacks$` | fail |
| `p4-current-any-provider` | same | `if !listed && o.Existing.Provider == d.ID && o.Existing.Model != "" {` → `if !listed && o.Existing.Model != "" {` | `^TestConfigureLLM_CurrentModelOnlyForSameProvider$` | fail |
| `p4-cap` | same | `maxSearchResults     = 20` → `maxSearchResults     = 25` | `^TestConfigureLLM_SearchResultsCapped$` | fail |
| `p4-no-static-notice` | `wizard/configure.go` | `if !cat.Live && !chatGPT {` → `if false {` | `^TestConfigureLLM_StaticCatalogNotice$` | fail |
| `p4-control` | `wizard/model_select.go` | `searchAgainLabel     = "Search again"` → `searchAgainLabel     = "Search again" // control` | `^TestConfigureLLM_` | pass |

`pkg` is `./wizard` for all. As in Phase 2, record the exact whitespace of the
const-block anchors in §11 if gofmt aligns them differently.

## Phase 5 — README and close-out

1. In `README.md`, after the paragraph ending "…not this standalone credential
   wizard." (`README.md:122-126`), insert:

   > With `Options.Discover` set, `ConfigureLLM` lists the provider's models
   > once and asks for a search before each model menu. A blank search shows
   > the curated recommendations, as before. A query searches every usable
   > model the provider lists: a glob such as `kilo-auto/*` or `*llama*`
   > matches whole ids, and other queries match loosely (`sonet`, `llama 8b`).
   > The same search is offered for fallbacks. `llmprovider.ListModelCatalog`
   > and `llmprovider.SearchModels` expose the listing and the matcher to
   > other callers. Scripts that drive the wizard need one extra (blank) line
   > before each model and fallback selection.
2. `go test -count=1 ./...` redirected to `$SCRATCH/p5-full.log`; branch on
   its own exit status.
3. `git diff --stat 55e4b31 -- go.mod go.sum llmprovider/models_catalog.go llmprovider/probe.go wizard/prompter.go wizard/text_prompter.go`
   must be empty (A13).
4. Fill in §11 (each phase's red lines, mutation summaries and gate summaries,
   verbatim). Set this plan to `status: complete` only if every §5 criterion
   holds.
5. Stage `README.md` and this plan, then `git commit --no-edit`. Do not push or
   tag; report the number of commits ahead of `origin/main`.

## 4. Verification commands

Per phase, after implementation (paths relative to the repository root):

```bash
python3 -B "$SCRATCH/mutate_and_test.py" --repo . --spec "$SCRATCH/pN-mutations.json" --workdir "$SCRATCH/pN-mut" > "$SCRATCH/pN-mut.log" 2>&1; MUT=$?
python3 -B "$SCRATCH/phase_gate.py" --repo . --logdir "$SCRATCH/pN-gate" --packages ./llmprovider ./wizard --files <phase files> > "$SCRATCH/pN-gate.log" 2>&1; GATE=$?
echo "mut=$MUT gate=$GATE"
```

Read both logs in full. Commit only when `MUT` and `GATE` are both 0. A
mutation reported `NOT OK` means either the test does not catch its defect or
the anchor is wrong. The runner exits early with `anchor occurs N times` if an
anchor is missing or ambiguous. Either case is a stop, not a retry.

## 5. Acceptance criteria

| # | Criterion | Evidence |
|---|---|---|
| A1 | `ListAvailableModels*` unchanged for every provider except Kilo and Hugging Face input modality | every pre-existing `llmprovider` test passes unmodified, except the two curation tests rewritten in Phase 1b; `TestListModelCatalog_RecommendedMatchesListAvailable` |
| A2 | `Recommended` is curated, `Usable` uncapped, from one fetch | `TestListModelCatalog_OpenAIRecommendedIsCurated`, `_UsableIsUncapped`, `p1-skip-curation` |
| A3 | Filters and Kilo policy still bind the searchable corpus | `TestListModelCatalog_FiltersStillApply`, `p1-kilo-policy` |
| A4 | Static fallback is observable (`Live`), and errors are unchanged | `TestListModelCatalog_LiveFlag`, `_Errors`, `p1-static-live` |
| A5 | Ollama split without aliasing | `TestListModelCatalog_OllamaSplit`, `p1-ollama-alias` |
| A6 | Gemini and Anthropic follow pagination, bounded, degrading on failure | Phase 2 tests; `p2-*` |
| A7 | Glob path is anchored, crosses `/`, keeps order, skips fuzzy | `TestSearchModels_Glob*`, `p3-glob-to-fuzzy`, `p3-unanchored` |
| A8 | Fuzzy ranking, tiers, tie-break, id-only subsequence, dedupe | Phase 3 tests; `p3-*` |
| A9 | Primary search uses `Usable`; blank shows the recommended menu; Other and Search again work | Phase 4 primary tests; `p4-search-recommended` |
| A10 | Reconfiguration keeps the current model for the same provider only | `TestConfigureLLM_CurrentModel*`, `p4-current-any-provider` |
| A11 | Fallback search excludes the primary; `Confirm` only after non-blank rounds; return shape unchanged | fallback tests; `TestConfigureLLM_Fallbacks` unmodified; `p4-primary-not-excluded`, `p4-no-confirm`, `p4-confirm-on-blank` |
| A12 | Display cap 20 with notice; static notice only when discovery was requested and not for ChatGPT | `TestConfigureLLM_SearchResultsCapped`, `_StaticCatalogNotice`, `_NoStaticNoticeWithoutDiscover`, `_ChatGPTNoStaticNotice`, `p4-cap`, `p4-no-static-notice` |
| A13 | `Prompter`, `TextPrompter`, static catalogs, probe and `go.mod` untouched | Phase 5 step 3 diff empty; `var _ Prompter` assertions compile |
| A14 | Every gate green; full module green | §11 gate summaries; `p5-full.log` exit 0 |
| A17 | Zen/Go send exactly one key header per route: `x-api-key` on messages, `x-goog-api-key` on google, `Authorization: Bearer` otherwise | `TestOpencode_KeyInHeader`; `TestLive_OpencodeKeyHeaderPerRoute`; `p0b-*` |
| A16 | Kilo and Hugging Face admit text-containing input, still require text-only output, and keep the training and tools rules | Phase 1b tests; `TestListModelCatalog_FiltersStillApply`; `p1b-*` |
| A15 | No new test reaches the network | code review of the new test files: every `Discover: true` test uses `httptest` or the ChatGPT short-circuit |

## 6. Rollout and rollback

**Rollout.** The maintainer tags the next minor release (`v1.6.0`; the last tag
is `v1.5.0`) after Phase 5. This plan does not tag or push. Consumers adopt the
release on their own bump (MADR out of scope). The release note must repeat the
README's last sentence. For prepare-commit-msg specifically: its stdin-scripted
tests (`internal/ui/setup_test.go:48`, `:79`, `:119`, `:144`, `:194`, `:230`)
need one blank line before each model selection, and before each fallback
selection because it sets `NeedFallbacks: true`. A stale line fails quietly: for
example `7`, meant for `Select`, becomes a search that matches
`gemini-3.7-flash` (Appendix C).

**Rollback.** Revert the phase commits newest first. No exported signature
changes and no persisted format changes, so a consumer that has not bumped is
unaffected, and one that has bumped returns to the six-item menus on reverting
its pin.

## 7. Risks

| Risk | Mitigation |
|---|---|
| Lister refactor changes curated output | A1: every existing listing test runs unmodified; the equivalence test covers all nine providers |
| New code silently excluded from lint by file name | §0.2 item 2: names checked against the regex; the gate runs `make lint` |
| `golint` findings pass unnoticed | §0.2 item 1: the gate uses `-set_exit_status`, one file per call |
| Pagination loops forever or overruns the 10s timeout | `maxListingPages`; bound test; the existing 10s context |
| Partial page sets presented as live | the second-page-failure tests |
| Scripted consumers misroute silently | README and release note; §6 names the affected prepare-commit-msg tests |
| A mutation proof passes for the wrong reason | the runner requires an exit status of non-zero **and** a `--- FAIL` line, and each phase includes a control mutation that must pass |

## 8. Out of scope (do not implement "while here")

* ~~The Kilo/Hugging Face exact-`["text"]` input-modality filter (MADR related
  finding 1).~~ Folded in as Phase 1b by MADR revision 3.
* Default-model ranking for commit messages: profiles, quality signals,
  external metadata, and the composition of the static catalogs. This is a
  separate, proposed decision.
* `TestConfigureLLM_EmptyDiscoveryFallsBackToStatic`'s live network call
  (related finding 2).
* `defaultHTTPClient` proxy support (related finding 3).
* Pre-selecting `Existing.Fallbacks` (related finding 4).
* Any change in prepare-commit-msg, mcp-server-magictools or
  mcp-server-magicdev.
* Changing `Options`, `Result`, `Prompter`, `TextPrompter`, `MaxListedModels`,
  `modelLabels` or any static catalog.
* New module dependencies.

## 9. File summary

**New:** `llmprovider/discovery_catalog_test.go`,
`llmprovider/discovery_pagination_test.go`, `llmprovider/model_matcher.go`,
`llmprovider/model_matcher_test.go`, `wizard/model_select.go`,
`wizard/model_select_test.go`.

**Modified:** `llmprovider/opencode.go`, `llmprovider/opencode_test.go`,
`llmprovider/live_gateways_test.go` (Phase 0b), `llmprovider/discovery.go`,
`llmprovider/discovery_test.go`
(Phase 1b fixtures and the two curation tests only),
`docs/0003-MADR-add-gateway-llm-providers.md` (forward pointer only),
`wizard/configure.go`,
`wizard/configure_test.go` (the two scripts in Phase 4 step 2 only),
`wizard/fake_prompter_test.go` (two recording fields), `README.md`,
`docs/0009-MADR-live-catalog-model-search.md` (status only),
`docs/0009-PLAN-live-catalog-model-search.md`.

**Must stay untouched** (checked in Phase 5 step 3): `go.mod`, `go.sum`,
`llmprovider/models_catalog.go`, `llmprovider/probe.go`, `wizard/prompter.go`,
`wizard/text_prompter.go`.

## 10. Deviation log

| Date | Phase | Finding | Decision | Files added to phase |
|---|---|---|---|---|
| 2026-09-26 | 0 | The 0009 MADR and PLAN link to records that are not committed: `0010-MADR-use-case-aware-default-model-ranking.md`, `0011-REPORT-provider-source-compatibility-audit.md` and `0012-MADR-conform-providers-to-reference-clients.md`. MADR 0003 already carries the 0011 audit note. Committing only the three planned files would leave broken links and pull the 0003 audit note into the commit anyway. | Maintainer chose "Commit all docs". Phase 0 commits every pending docs change. Only 0009 changes status (accepted); 0010 and 0012 stay `proposed` and nothing of them is implemented. No MADR amendment: no decision or asserted fact changes. | `docs/0001-MADR-add-grok-xai-llm-provider.md`, `docs/0008-MADR-subscription-auth-for-llm-providers.md` (audit notes only), `docs/0010-MADR-use-case-aware-default-model-ranking.md`, `docs/0011-REPORT-provider-source-compatibility-audit.md`, `docs/0012-MADR-conform-providers-to-reference-clients.md` |
| 2026-09-26 | 0b | `llmprovider/opencode_route.go:8-10` says both gateways use "one auth scheme (Authorization: Bearer)". §1c makes that false, and the file is not in Phase 0b's list. | Maintainer chose "Add file to phase": a comment-only edit pointing to `opencodeKeyHeader` and §1c, gated with the phase. No behaviour change and no MADR amendment. | `llmprovider/opencode_route.go` |
| 2026-09-26 | 1 | `make lint` failed: `func listOpenAIModels is unused` and `func listGrokModels is unused` (`discovery.go`). The plan said every `list*Models` wrapper stays, but its own caller list names no OpenAI or Grok caller. Those two were reached only through the old dispatch switch, which `modelCatalogFor` replaces. `openai.go:204` and `grok.go:239` call `ListAvailableModelsWithSource`. | Maintainer chose "Delete the two wrappers". Their logic lives in `fetchOpenAIUsable`/`curateOpenAI` and `fetchGrokUsable`/`curateGrok`, reached through `modelCatalogFor`. No behaviour change and no MADR amendment. | none (same file) |
| 2026-09-26 | 2 | Mutation `p2-unbounded` (`maxListingPages` 10 → 1000) reported NOT OK. `TestListModelCatalog_PaginationIsBounded` compared the request count against the production constant, so moving the bound moved the expectation (1000 == 1000). | Maintainer chose "Assert the literal bound": the test compares against `const wantPages = 10`, per MADR 0009 §2, independent of the production constant, and `p2-unbounded` is re-run. No MADR amendment. | none (same file) |

## 11. Execution record

Each phase appends: the red-run lines, the mutation runner's summary lines,
the gate summary line, and the commit SHA.

### Phase 0 — complete (2026-09-26)

* MADR 0009 set to `accepted`, and this plan to `in-progress`.
* MADR 0003 annotated with the §1b/§1c pointer, directly under its title.
* The commit scope was widened by the deviation above.
* Markdown only, so no phase gate applies.
* Commit: `754f1e4`.
* The plan's `docs/` changes are committed with each phase, so the deviation
  log and this record travel with the work they describe.

### Phase 0b — complete (2026-09-26)

**Red run:** `go test -count=1 -run '^TestOpencode_KeyInHeader$' ./llmprovider`, exit 1.

```
--- FAIL: TestOpencode_KeyInHeader/claude-sonnet-5 (0.00s)
    opencode_test.go:126: Authorization must not be sent on this route, got "Bearer test-key"
    opencode_test.go:124: x-api-key = "", want "test-key"
--- FAIL: TestOpencode_KeyInHeader/gemini-3.7-flash (0.00s)
    opencode_test.go:126: Authorization must not be sent on this route, got "Bearer test-key"
    opencode_test.go:124: x-goog-api-key = "", want "test-key"
```

**Green:**
* the same test passes;
* `go test -count=1 ./llmprovider` passes;
* `go vet -tags live_gateways ./llmprovider` exits 0.

**Live run:**
`go test -count=1 -tags live_gateways -run '^TestLive_OpencodeKeyHeaderPerRoute$' -v ./llmprovider`
passed, so no deviation.

```
--- PASS: TestLive_OpencodeKeyHeaderPerRoute (0.64s)
    --- PASS: TestLive_OpencodeKeyHeaderPerRoute/messages (0.40s)
    --- PASS: TestLive_OpencodeKeyHeaderPerRoute/google (0.24s)
```

**Mutation proofs:** `mut=0`.

```
p0b-bearer-messages: expect=fail exit=1 OK ['--- FAIL: TestOpencode_KeyInHeader (0.00s)']
p0b-bearer-google: expect=fail exit=1 OK ['--- FAIL: TestOpencode_KeyInHeader (0.00s)']
p0b-live-wrong-right: expect=fail exit=1 OK ['--- FAIL: TestLive_OpencodeKeyHeaderPerRoute (0.41s)']
p0b-control: expect=pass exit=0 OK []
4/4 behaved as expected
```

**Phase gate:** `gate=0`, `gate=PASS (8/8 checks)`. That covers gofmt, per-file
golint on all four files, vet, `make lint` and tests.

**Deviation:** `llmprovider/opencode_route.go` was added to the phase for a
comment-only fix (§10).

Commit: `90554f9`.

### Phase 1 — complete (2026-09-26)

**Red run:** `go test -count=1 -run 'TestListModelCatalog' ./llmprovider` failed
to compile, exit 1.

```
llmprovider/discovery_catalog_test.go:95:16: undefined: ListModelCatalog
```

**Green:**
* `go test -count=1 -run 'TestListModelCatalog' ./llmprovider`: `ok`;
* `go test -count=1 ./llmprovider ./wizard`: both `ok`, with no pre-existing
  test edited.

**Deviation:** the first gate run failed `make lint`: `listOpenAIModels` and
`listGrokModels` were unused. Both were deleted by maintainer decision (§10).

**Mutation proofs:** `mut=0`.

```
p1-skip-curation: expect=fail exit=1 OK ['--- FAIL: TestListModelCatalog_OpenAIRecommendedIsCurated (0.00s)']
p1-static-live: expect=fail exit=1 OK ['--- FAIL: TestListModelCatalog_LiveFlag (0.00s)']
p1-ollama-alias: expect=fail exit=1 OK ['--- FAIL: TestListModelCatalog_OllamaSplit (0.00s)']
p1-kilo-policy: expect=fail exit=1 OK ['--- FAIL: TestListModelCatalog_FiltersStillApply (0.00s)']
p1-control: expect=pass exit=0 OK []
5/5 behaved as expected
```

**Phase gate:** `gate=0`, `gate=PASS (6/6 checks)`. `make lint` reported
`0 issues.`, and the pre-approved `nilerr` annotation was not needed.

**Implementation notes, within §1.2:**
* `fetchOpenAIUsable`, `fetchGrokUsable` and `fetchOpencodeUsable` share an
  unexported `fetchDataIDs`/`filterIDs` pair for the identical
  `{"data":[{"id"}]}` fetch.
* The per-provider curation expressions are named `curateGemini`,
  `curateOpenAI`, `curateClaude`, `curateGrok`, `curateHuggingFace` and
  `curateKilo`. The OpenCode curation is a closure over the gateway.

Commit: `6aa41db`.

### Phase 1b — complete (2026-09-26)

**Red run:**
`go test -count=1 -run 'MetadataCuration|InputModalityContainsText' ./llmprovider`
exited 1. The first attempt failed to compile (`undefined: slices`) until
`discovery_test.go` imported `slices`, which the rewritten test needs. The
assertion failures:

```
--- FAIL: TestListModelCatalog_InputModalityContainsText/huggingface
    discovery_catalog_test.go:322: org/vlm (text+image in, text out) missing from Usable [org/fast org/mid org/slow]
--- FAIL: TestListModelCatalog_InputModalityContainsText/kilo
    discovery_catalog_test.go:322: org/vlm (text+image in, text out) missing from Usable [org/cheap org/dear kilo-auto/variable]
--- FAIL: TestListHuggingFaceModels_MetadataCuration
    discovery_test.go:407: got [org/fast org/mid org/slow], want [org/vlm org/fast org/mid org/slow]
--- FAIL: TestListKiloModels_MetadataCuration
    discovery_test.go:502: got [org/cheap org/dear kilo-auto/variable], want [org/cheap org/vlm org/dear kilo-auto/variable]
```

**Green:** the same command, and `go test -count=1 ./llmprovider ./wizard`,
both `ok`.

**Live evidence (recorded, not gated):** the scratch program called
`ListModelCatalog` anonymously.
* Kilo: `live=true usable=284 recommended=[kilo-auto/small kilo-auto/efficient
  kilo-auto/balanced meta-llama/llama-3.1-8b-instruct openai/gpt-oss-20b
  rekaai/reka-edge]`. `kilo-auto/small`, `/efficient` and `/balanced` are all
  in `Usable`.
* Hugging Face: `live=true usable=131`. `zai-org/GLM-5.3-Flash` is in `Usable`
  and in `Recommended`.

This matches MADR 0009 Context §5 exactly, so there is no deviation.

**Mutation proofs:** `mut=0`.

```
p1b-exact-text-input: expect=fail exit=1 OK [InputModalityContainsText, HuggingFace MetadataCuration, Kilo MetadataCuration]
p1b-output-relaxed: expect=fail exit=1 OK [InputModalityContainsText, HuggingFace MetadataCuration, Kilo MetadataCuration]
p1b-control: expect=pass exit=0 OK []
3/3 behaved as expected
```

**Phase gate:** `gate=0`, `gate=PASS (7/7 checks)`.

Commit: `a5c617d`.

### Phase 2 — complete (2026-09-26)

**Red run:**
`go test -count=1 -run 'TestListModelCatalog_(Gemini|Claude|Pagination|SinglePage)' ./llmprovider`,
exit 1.
* The first attempt failed to compile, because the bound test references
  `maxListingPages`. The locked §1.2 constant block was added first, with no
  pagination logic, so the red run could fail on assertions as the plan
  requires.
* The assertion failures (abridged; the full log is in the scratchpad):

```
--- FAIL: TestListModelCatalog_GeminiFollowsNextPageToken: requests = 1, want 2
--- FAIL: TestListModelCatalog_ClaudeFollowsHasMore: requests = 1, want 2
--- FAIL: TestListModelCatalog_GeminiSecondPageFailureDegrades: … Live:true}, want static (Live false)
--- FAIL: TestListModelCatalog_ClaudeSecondPageFailureDegrades: … Live:true}, want static (Live false)
--- FAIL: TestListModelCatalog_PaginationIsBounded/gemini: requests = 1, want maxListingPages (10)
--- FAIL: TestListModelCatalog_PaginationIsBounded/claude: requests = 1, want maxListingPages (10)
--- FAIL: TestListModelCatalog_ClaudeHasMoreWithoutLastIDDegrades: … Live:true}, want static (Live false)
--- FAIL: TestListModelCatalog_SinglePageRequestsMaxPageSize: gemini queries = [map[]] …; claude queries = [map[]] …
```

**Green:** the same command, and `go test -count=1 ./llmprovider ./wizard`,
both `ok`.

**Deviation:** the first mutation run reported
`p2-unbounded: expect=fail exit=0 NOT OK`. The bound test compared against the
production constant. By maintainer decision it now asserts the literal 10
(§10).

**Mutation proofs, after the fix:** `mut=0`.

```
p2-gemini-one-page: expect=fail exit=1 OK ['--- FAIL: TestListModelCatalog_GeminiFollowsNextPageToken (0.00s)']
p2-claude-one-page: expect=fail exit=1 OK ['--- FAIL: TestListModelCatalog_ClaudeFollowsHasMore (0.00s)']
p2-unbounded: expect=fail exit=1 OK ['--- FAIL: TestListModelCatalog_PaginationIsBounded (0.16s)']
p2-control: expect=pass exit=0 OK []
4/4 behaved as expected
```

**Phase gate:** `gate=0`, `gate=PASS (6/6 checks)`.

**Implementation note:** each page is fetched by `fetchGeminiPage` or
`fetchClaudePage`, so the `defer` that closes each response is not inside the
page loop.

Commit: `dcc1dcc`.

### Phase 3 — complete (2026-09-26)

**Red run:** `go test -count=1 -run TestSearchModels ./llmprovider` failed to
compile, exit 1:

```
llmprovider/model_matcher_test.go:21:20: undefined: ModelMatch
```

**Green:** all 14 `TestSearchModels_*` pass on the first implementation run,
including the reference-derived exact scores (`flash` → 4000, 4000, 1020;
`sonet` → 1035). The Go matcher therefore agrees with the Appendix C
reference. `go test -count=1 ./llmprovider ./wizard`: both `ok`.

**Mutation proofs:** `mut=0`.

```
p3-glob-to-fuzzy: expect=fail exit=1 OK [GlobCrossesSlash, GlobMatchesAcrossOrg, GlobIsAnchored, GlobKeepsInputOrder]
p3-unanchored: expect=fail exit=1 OK ['--- FAIL: TestSearchModels_GlobIsAnchored (0.00s)']
p3-label-subsequence: expect=fail exit=1 OK ['--- FAIL: TestSearchModels_NoSubsequenceOverLabels (0.00s)']
p3-no-length-tiebreak: expect=fail exit=1 OK ['--- FAIL: TestSearchModels_TieBreak (0.00s)']
p3-no-dedupe: expect=fail exit=1 OK ['--- FAIL: TestSearchModels_Dedupe (0.00s)']
p3-control: expect=pass exit=0 OK []
6/6 behaved as expected
```

**Phase gate:** `gate=0`, `gate=PASS (6/6 checks)`.

**Implementation note:** the unlocked helper names are `uniqueModelMatches`,
`globMatches`, `fuzzyModelScore`, `modelTokens` and `tokenPrefixMatch`. The
locked anchors of §1.4 are verbatim.

Commit: `22a0fbb`.

### Phase 4 — complete (2026-09-26)

**Step 1:** `fakePrompter` gained `seenSelectDefault` and
`seenMultiSelectItems`. `gofmt -w` realigned the struct, with no behaviour
change.

**Step 2, observed red:** with only the script change in place,
`go test -run '^TestConfigureLLM_OtherModelEscapeHatch$' ./wizard` exited 1:

```
--- FAIL: TestConfigureLLM_OtherModelEscapeHatch (0.00s)
    configure_test.go:275: ConfigureLLM: wizard: no model entered
```

This is today's flow reading the blank search line as the manual id, as
predicted.

**Step 3, red:** `go test -count=1 ./wizard` failed to compile on the new
constants, exit 1:

```
wizard/model_select_test.go:82:35: undefined: searchModelsPrompt
```

**Green:** `go test -count=1 ./wizard`: `ok`, with all new and pre-existing
tests passing. The only pre-existing test edits are the two step-2 script
changes, confirmed by `git diff -U0 wizard/configure_test.go`.
`go test -count=1 ./llmprovider ./wizard`: both `ok`.

**Mutation proofs:** `mut=0`.

```
p4-search-recommended: expect=fail exit=1 OK ['--- FAIL: TestConfigureLLM_SearchUsesLiveCorpus (0.00s)']
p4-primary-not-excluded: expect=fail exit=1 OK ['--- FAIL: TestConfigureLLM_FallbackSearch (0.00s)']
p4-no-confirm: expect=fail exit=1 OK ['--- FAIL: TestConfigureLLM_FallbackSearch (0.00s)']
p4-confirm-on-blank: expect=fail exit=1 OK ['--- FAIL: TestConfigureLLM_Fallbacks (0.00s)']
p4-current-any-provider: expect=fail exit=1 OK ['--- FAIL: TestConfigureLLM_CurrentModelOnlyForSameProvider (0.00s)']
p4-cap: expect=fail exit=1 OK ['--- FAIL: TestConfigureLLM_SearchResultsCapped (0.00s)']
p4-no-static-notice: expect=fail exit=1 OK ['--- FAIL: TestConfigureLLM_StaticCatalogNotice (0.00s)']
p4-control: expect=pass exit=0 OK []
8/8 behaved as expected
```

**Phase gate:** `gate=0`, `gate=PASS (9/9 checks)`.

**Implementation notes:**
* The unlocked helpers in `model_select.go` are `appendPicks`, which carries
  the nil-versus-empty return shape, and `matchIDs`.
* `otherModelLabel` and its comment moved from `configure.go` to
  `model_select.go`, as §1.5 specifies.

## Appendix A — `phase_gate.py`

Write to `$SCRATCH/phase_gate.py`. It was proven on 2026-09-25: it passed the
clean tree (6/6 checks), and failed (2/5 checks, exit 1) on a scratch copy with
an unformatted, uncommented exported function. On that copy `gofmt` exited 0
but printed the path, `golint` exited 1, and `make lint` exited 2.

```python
#!/usr/bin/env python3
"""Phase gate for 0009-PLAN: every check runs separately, logs in full, and decides alone.

Usage:
    phase_gate.py --repo PATH --logdir DIR --packages ./llmprovider ./wizard --files F1.go F2.go ...

Exit 0 only if every check passes. Each check's complete output is written to
DIR/<check>.log; the summary line names each check's own exit status.
"""
from __future__ import annotations

import argparse
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

TIMEOUT_SECONDS = 900


@dataclass
class Check:
    name: str
    argv: list[str]
    must_be_silent: bool = False  # gofmt -l reports by printing, not by status


def run(check: Check, repo: Path, logdir: Path) -> bool:
    proc = subprocess.run(check.argv, cwd=repo, capture_output=True, text=True,
                          timeout=TIMEOUT_SECONDS, check=False)
    output = proc.stdout + proc.stderr
    (logdir / f"{check.name}.log").write_text(f"$ {' '.join(check.argv)}\n{output}\nexit={proc.returncode}\n")
    ok = proc.returncode == 0 and not (check.must_be_silent and output.strip())
    print(f"{check.name}: exit={proc.returncode} {'PASS' if ok else 'FAIL'}")
    return ok


def build_checks(files: list[str], packages: list[str]) -> list[Check]:
    checks = [Check("gofmt", ["gofmt", "-l", *files], must_be_silent=True)]
    checks += [Check(f"golint-{Path(f).name}", ["golint", "-set_exit_status", f]) for f in files]
    checks += [
        Check("vet", ["go", "vet", *packages]),
        Check("lint", ["make", "lint"]),
        Check("test", ["go", "test", "-count=1", *packages]),
    ]
    return checks


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, required=True)
    parser.add_argument("--logdir", type=Path, required=True)
    parser.add_argument("--packages", nargs="+", required=True)
    parser.add_argument("--files", nargs="+", required=True)
    args = parser.parse_args()
    args.logdir.mkdir(parents=True, exist_ok=True)
    missing = [f for f in args.files if not (args.repo / f).is_file()]
    if missing:
        print(f"FAIL: listed files do not exist: {missing}")
        return 1
    results = [run(c, args.repo, args.logdir) for c in build_checks(args.files, args.packages)]
    print(f"gate={'PASS' if all(results) else 'FAIL'} ({sum(results)}/{len(results)} checks)")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
```

## Appendix B — `mutate_and_test.py`

Write to `$SCRATCH/mutate_and_test.py`. Spec files are JSON lists of
`{"name","file","old","new","pkg","run","expect"}`. Proven on 2026-09-25 against
today's tree:

* a planted Kilo-policy bypass
  (`if m.MayTrainOnYourPrompts { // POLICY …` → `if false { …`) was **caught**:
  exit 1, `--- FAIL: TestListKiloModels_MetadataCuration`;
* a comment-only control **passed** with exit 0;
* the working tree was unchanged afterwards (`git status --porcelain` showed
  only the MADR).
* 2026-09-26, after adding the optional `tags` field:
  * the two cases above behaved identically (2/2);
  * a planted `t.Fatal` in `live_gateways_test.go` was **caught** with
    `"tags": "live_gateways"` (`--- FAIL: TestLive_ListingsNeedNoCredential`);
  * the same plant **passed** without tags, so untagged runs exclude that
    file.

```python
#!/usr/bin/env python3
"""Prove a test can fail: plant one defect in a scratch copy and require the test to fail.

Usage:
    mutate_and_test.py --repo PATH --spec mutations.json --workdir DIR

mutations.json is a list of objects:
    {"name": str, "file": str, "old": str, "new": str, "pkg": str, "run": str,
     "expect": "fail" | "pass", "tags": str (optional build tags, e.g. "live_gateways")}

For each entry the repository working tree (uncommitted changes included, .git
excluded) is copied to DIR/<name>, `old` is replaced by `new` after asserting it
occurs exactly once, and `go test -count=1 -run RUN PKG` is executed there. An
entry with expect=fail is CAUGHT only if the run exits non-zero and prints a
"--- FAIL" line. expect=pass entries are controls: they must exit zero. The
repository itself is never written.
"""
from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

TIMEOUT_SECONDS = 900


@dataclass(frozen=True)
class Mutation:
    name: str
    file: str
    old: str
    new: str
    pkg: str
    run: str
    expect: str = "fail"
    tags: str = ""


def copy_tree(repo: Path, dst: Path) -> None:
    if dst.exists():
        shutil.rmtree(dst)
    shutil.copytree(repo, dst, ignore=shutil.ignore_patterns(".git"))


def plant(dst: Path, m: Mutation) -> None:
    target = dst / m.file
    text = target.read_text()
    count = text.count(m.old)
    if count != 1:
        raise SystemExit(f"{m.name}: anchor occurs {count} times in {m.file}; expected exactly 1")
    target.write_text(text.replace(m.old, m.new))
    assert m.new in target.read_text(), f"{m.name}: replacement did not land"


def execute(m: Mutation, repo: Path, workdir: Path) -> bool:
    dst = workdir / m.name.replace(" ", "_")
    copy_tree(repo, dst)
    plant(dst, m)
    argv = ["go", "test", "-count=1"] + (["-tags", m.tags] if m.tags else []) + ["-run", m.run, m.pkg]
    proc = subprocess.run(argv, cwd=dst,
                          capture_output=True, text=True, timeout=TIMEOUT_SECONDS, check=False)
    output = proc.stdout + proc.stderr
    (workdir / f"{dst.name}.log").write_text(output + f"\nexit={proc.returncode}\n")
    failed = proc.returncode != 0 and "--- FAIL" in output
    ok = failed if m.expect == "fail" else proc.returncode == 0
    fail_lines = [line.strip() for line in output.splitlines() if line.startswith("--- FAIL")]
    print(f"{m.name}: expect={m.expect} exit={proc.returncode} {'OK' if ok else 'NOT OK'} {fail_lines}")
    return ok


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, required=True)
    parser.add_argument("--spec", type=Path, required=True)
    parser.add_argument("--workdir", type=Path, required=True)
    args = parser.parse_args()
    args.workdir.mkdir(parents=True, exist_ok=True)
    mutations = [Mutation(**entry) for entry in json.loads(args.spec.read_text())]
    results = [execute(m, args.repo.resolve(), args.workdir) for m in mutations]
    print(f"{sum(results)}/{len(results)} behaved as expected")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
```

## Appendix C — Evidence for this plan's assertions

All gathered on 2026-09-25, read-only with respect to the repository. Scratch
artefacts lived in the session scratchpad and are not committed.

* **Baseline (§0.1).** `go test -count=1 ./...`: exit 0, 8 packages `ok`.
  `make lint`: `0 issues.`, exit 0. `gofmt -l llmprovider wizard`: no output.
  `go vet ./llmprovider ./wizard`: exit 0. `golint -set_exit_status` over the
  seven files named in §0.1: each exit 0.
* **golint exit status (§0.2 item 1).** Scratch file `bad.go` with
  `func Exported() {}`: `golint` printed
  `exported function Exported should have comment or be unexported` and exited
  0; with `-set_exit_status` it also printed `Found 1 lint suggestions;
  failing.` and exited 1.
* **Lint path exclusion (§0.2 item 2).** Scratch module with identical
  `os.Remove` calls in `model_search.go` and `model_matcher.go`, linted with this
  repository's `.golangci.yml`: exactly one issue, `model_matcher.go:7:11: Error
  return value of os.Remove is not checked (errcheck)`. A regex check of every
  file in §9 against the exclusion pattern: all linted, and only the rejected
  name `model_search.go` excluded.
* **Matcher expectations (§1.4, Phase 3 table).** A Python reference
  implementation of §1.4 (labels copied verbatim from `modelLabels`) produced
  every expected value in the Phase 3 table, 18/18 checks. Five planted
  defects in the reference were each caught by at least one of those checks:
  unanchored glob, subsequence over labels, glob routed to fuzzy, no
  length/lexicographic tie-break, and no case-insensitive dedupe. Two of the
  five were initially not caught because the *mutants* were wrong: Python's
  `re.match` anchors at the start, and the first tie-break mutant re-sorted
  output that was already sorted. The mutants were corrected to model Go's
  unanchored `MatchString` and an unsorted fuzzy result, and were then caught.
* **Wizard expectations (Phase 4 table).** The same reference, run on the
  Phase 4 fixtures, gave:
  * `sonnet` over `zenSearchFixture` = `[claude-sonnet-5]`, and `[]` over the
    recommended six only (the `p4-search-recommended` outcome);
  * `haiku` = `[claude-haiku-4-5]`;
  * `claude` minus the primary = `[claude-opus-5 claude-sonnet-5]`, and with
    the primary not excluded, `claude-haiku-4-5` is added;
  * `haiku` over `StaticClaude` = `[claude-haiku-4-5 claude-3-5-haiku-latest]`;
  * `zzzz` = `[]`;
  * `m-` over 25 ids = 25 matches, first 20 `m-01`…`m-20`.

  9/9 checks.
* **Stale-script effects (Phase 4 step 2, §6).** The reference gives no matches
  for `http://localhost:11434` over `StaticGemini`, and `[gemini-3.7-flash]` for
  `7`.
* **Existing test facts.** `fakePrompter` errors on unscripted
  `Select`/`MultiSelect`/`Confirm` and returns the default for `Input`
  (`fake_prompter_test.go:47-49`, `:64-66`, `:76-78`, `:84-95`). It records no
  `MultiSelect` choices and no `Select` default today (`:14-34`), hence the
  Phase 4 step 1 fields. Descriptor order puts Gemini at index 0
  (`descriptor.go:72`). Only OpenCode Zen, OpenCode Go, Hugging Face, Kilo and
  Ollama set `supportsBaseURL` (`descriptor.go:147-167`). No existing test
  asserts on a listing request's query string except
  `TestGemini_KeyInHeaderNotURL` (Generate only; it rejects `key=`) and an
  OpenCode test that requires an empty query (OpenCode is not paginated).
  `TestClaudeProvider_DiscoverModels` and `TestGeminiProvider_DiscoverModels`
  route on `r.URL.Path`, which a query string does not change
  (`probe_test.go:37-103`).
* **Callers (§1.2).** Repository-wide grep: the unexported listers are called
  from `discovery.go:45-59` and from `gemini.go:298`, `claude.go:298`,
  `opencode.go:310`, `kilo.go:200`, `huggingface.go:174` and `ollama.go:195`.
  `ListAvailableModelsWithSource` is also called from `grok.go:239`,
  `openai.go:204` and `wizard/configure.go:283`.
* **Consumer return-shape sensitivity (§1.5).** mcp-server-magictools persists
  fallbacks via `config.Set(res.Fallbacks)` into a field tagged `omitempty` and
  guarded by `len(...) > 0`. prepare-commit-msg checks `len(...) == 0`. Neither
  distinguishes nil from empty, and the plan preserves today's shape anyway.
* **Live scratch program (Phase 1b step 5).** A module outside the repository
  in `$SCRATCH/livelist`. Its `go.mod` is `module livelist`, `go 1.26.6`,
  `require github.com/maccavelli/mcplib v0.0.0` and
  `replace github.com/maccavelli/mcplib => <absolute path of this checkout>`.
  Its `main.go` calls the API anonymously and prints the result per provider.
  Before Phase 1, `ListAvailableModels(ctx, p, "")` was proven the same way on
  2026-09-25: it printed today's curated sixes and left
  `git status --porcelain` unchanged. After Phase 1 it calls
  `ListModelCatalog(ctx, p, "")`. Build it with `GOFLAGS=-mod=mod go mod tidy`,
  then `go run .`.
* **Phase 1b expectations.** A scratch Python replica of the Kilo and Hugging
  Face filters and orderings (`discovery.go:443-485`, `:583-611`), with the
  input rule switchable, first reproduced today's expected results from the
  existing tests: `[org/fast org/mid org/slow]` and
  `[org/cheap org/dear kilo-auto/variable]`. With the relaxed rule plus the
  planned fixture entries it gives `[org/vlm org/fast org/mid org/slow]` and
  `[org/cheap org/vlm org/dear kilo-auto/variable]`. Evaluating the new fixtures
  under the old rule gives different results, so the rewritten tests
  discriminate between the two rules.
* **CI.** `.github/workflows/ci.yml` runs `go test ./...` on the `go.mod`
  toolchain, which Phase 5 step 2 reproduces locally.
