---
status: in-progress
date: 2026-09-26
associated-madr: "0010-MADR-use-case-aware-default-model-ranking.md"
decision-makers: mcplib maintainers
---

# Implement Rank Recommended Models by Use Case from Live Catalog Metadata

Associated MADR: [0010-MADR-use-case-aware-default-model-ranking.md](0010-MADR-use-case-aware-default-model-ranking.md)
(proposed, revision 3, 2026-09-26).

This plan executes that MADR and nothing else. If execution finds a fact that
contradicts the MADR or this plan, **stop and prompt**: record a dated entry in
§10, amend the MADR when a decision or an asserted fact changes, and only then
continue. Do not absorb a deviation silently, and do not widen scope.

## Goal

The six recommended models of the four open catalogs (Kilo, OpenCode Zen,
OpenCode Go, Hugging Face) are ranked for a use case, from the metadata those
catalogs publish. The reasoning path is one option plus one retry helper away.
Specifically:

* **Profiles.** `llmprovider.ModelProfile` has two values, `ProfileUtility`
  (the zero value) and `ProfileCapable`. It is selected with `WithModelProfile`
  or `wizard.Options.Profile`, and carries a recommended `ReasoningEffort()`
  (MADR §1).
* **Kilo** ranks from its own listing.
* **Zen, Go and Hugging Face** rank from `https://models.opencode.ai/api.json`:
  * fetched concurrently with the listing and cached for 10 minutes;
  * overridable with `WithModelMetadataURL` or `MCPLIB_MODELS_METADATA_URL`,
    and disabled with `MCPLIB_DISABLE_MODELS_METADATA`;
  * degrading to today's curation on any failure (MADR §2).
* **Eligibility, ordering, diversity and fill** follow MADR §3–§4 exactly. The
  implementation reproduces MADR §7's eight sixes from the 2026-09-26 snapshot.
* **Static catalogs.** `StaticKilo`, `StaticOpencodeZen`, `StaticOpencodeGo`
  and `StaticHuggingFace` become MADR §7's utility sixes (MADR §5).
* **`GenerateThinkingWithRetry`** shares `GenerateWithRetry`'s loop.
* **Kilo's thinking path** sends the `reasoning` object (`{effort}` or
  `{enabled: true}`).
* **OpenCode's chat route** sends `reasoning_effort` when the model's published
  `reasoning_options` list the configured effort. Both reasoning changes pass a
  live gate before they are committed (MADR §6).
* **Unchanged:**
  * `Prompter`, `Result`, the `ListAvailableModels*` and `ListModelCatalog*`
    signatures, and `MaxListedModels`;
  * search and every usability filter;
  * the first-party providers' curation.

## Scope

**In scope.** The `llmprovider` and `wizard` files listed in §9; five trimmed
snapshot files under `llmprovider/testdata/ranking-2026-09-26/`; `README.md`;
a forward pointer in MADR 0003; this plan and its MADR.

**Out of scope.** Everything in the MADR's *Out of scope*, and §8 below.

## 0. Baseline, conventions and proven tool behaviour

### 0.1 Baseline (verified 2026-09-26, `mcplib` at `5a1fc70`, only the MADR modified)

* **Phase gate:** 17 of 17 checks passed (`$SCRATCH/baseline-0010/`) on
  `llmprovider/{discovery,options,provider,kilo,opencode,chatcompletions,models_catalog,discovery_test,kilo_test,opencode_test,live_gateways_test}.go`,
  `wizard/configure.go` and `wizard/model_select_test.go`.
* **`go test -count=1 ./...`:** exits 0, with 8 packages `ok`.
* **`go vet -tags live_gateways ./llmprovider`:** exits 0.
* **Git hooks** resolve to the global hooks directory, so
  `git commit --no-edit` takes its message from the `prepare-commit-msg` hook.
* **Branch state:** `main` is level with `origin/main`. The last tag is
  `v1.5.0`; 0009 is not yet tagged.

### 0.2 Tool behaviour this plan relies on

All four facts are proven in
[0009-PLAN-live-catalog-model-search.md](0009-PLAN-live-catalog-model-search.md)
§0.2 and still hold:

1. `golint` needs `-set_exit_status`, one file per call.
2. `.golangci.yml` exempts file names matching
   `(ui\.go|analyzer\.go|client\.go|tui\.go|login\.go|main\.go|search\.go|git\.go|config\.go|index\.go|internal\.go|logs\.go|kibana\.go)`.
   **Every file this plan creates was checked against the regex, and none
   matches.** `main_test.go` does not contain `main.go`, and no new file name
   contains `client`.
3. `gofmt -l` exits 0 even when it lists a file.
4. mcplib has no `pre-add-check` target.

Two more facts matter here:

5. **`unused` and struct fields.** The `unused` linter reports a struct field
   that nothing reads, even when JSON decoding fills it. Each field in §1 is
   therefore introduced in the phase whose code first reads it.
6. **`unparam` on constant arguments.** In Phase 2, `rankGroup`'s `family` and
   `rankRecommended`'s `provider` each get a single value from production
   code; other values come only from tests. A probe in a scratch copy ran
   `make lint` with a helper that production called with a constant argument.
   With one test caller passing another value, and with none, both runs
   reported `0 issues.`, even with two production call sites. The probe
   **could not be made to fail**, so it shows only that this lint config does
   not flag the pattern. It does not show whether test call sites count. If
   the Phase 2 gate flags either parameter, that is a deviation (§10).

### 0.3 Conventions

* **`$SCRATCH`** is the executing session's scratchpad directory. No Python
  artefact is ever created inside the repository.
* **Phase gate** is Appendix A of
  [0009-PLAN-live-catalog-model-search.md](0009-PLAN-live-catalog-model-search.md)
  (`phase_gate.py`), copied to `$SCRATCH` unchanged. Run it with the phase's
  exact Go file list and `--packages ./llmprovider ./wizard`. Never pipe a gate
  into `tail` or `head`.
* **Mutation runner** is Appendix B of the same plan (`mutate_and_test.py`,
  with the optional `tags` field), copied to `$SCRATCH` unchanged. Every
  planted defect runs in a scratch copy, never in the working tree.
* **Seen to fail.** Each phase's new tests are run before the implementation.
  A compile failure (`undefined: …`) is an acceptable red, copied to §11.
  Every behavioural assertion is also proven by a mutation (tables below).
  Each phase has a control mutation that must pass.
* **Mutation tables.** A `\|` in a table cell is a literal `|`, escaped for
  Markdown. `\n` and `\t` are a newline and a tab in the JSON spec. "The same
  + ` (control)`" means the old text with ` (control)` appended.
* **Clock.** Ranking tests pin `rankingNow` to `2026-09-26T00:00:00Z`, the
  reference simulation's `NOW`.
* **Literals.** Tests assert literal thresholds and lists, never the
  production constant they guard. This is the lesson of 0009's Phase 2
  deviation.
* **Commits.** One `git commit --no-edit` per phase after the gate passes,
  staging only that phase's files. No `-m`, no `--amend`, no `git push`, no
  tag.
* **§1 is the implementation.** The names, signatures, constants, strings and
  code in §1 are the implementation, not sketches. Changing one is a deviation
  (§10).

## 1. Locked design

### 1.1 Exported API

| Symbol | Kind | Phase |
|---|---|---|
| `llmprovider.ModelProfile`, `ProfileUtility`, `ProfileCapable` | type, consts | 1 |
| `(ModelProfile).ReasoningEffort() string` | method | 1 |
| `llmprovider.WithModelProfile(ModelProfile) ProviderOption` | func | 1 |
| `ProviderConfig.ModelProfile ModelProfile` | field | 1 |
| `llmprovider.GenerateThinkingWithRetry(ctx, ThinkingProvider, prompt string, retries int, delay time.Duration) (string, error)` | func | 1 |
| `llmprovider.WithModelMetadataURL(string) ProviderOption` | func | 3 |
| `ProviderConfig.ModelMetadataURL string` | field | 3 |
| `wizard.Options.Profile llmprovider.ModelProfile` | field | 4 |

Nothing else is exported. Every addition is additive; no signature changes.

### 1.2 Profiles and the retry helper (Phase 1)

**New file `llmprovider/model_profile.go`:**

```go
package llmprovider

// ModelProfile selects how the open catalogs rank their recommended models
// (MADR 0010 §1).
type ModelProfile int

const (
	// ProfileUtility (the zero value) ranks for short, frequent tasks such as
	// commit messages: reasoning-capable, recent, paid, cheap.
	ProfileUtility ModelProfile = iota
	// ProfileCapable ranks for reasoning-heavy tasks: strongest first.
	ProfileCapable
)

// ReasoningEffort is the recommended request effort for the profile: "low"
// for ProfileUtility, and "" for ProfileCapable, meaning the model's own
// default. A value outside the two profiles is treated as ProfileUtility.
func (p ModelProfile) ReasoningEffort() string {
	if p == ProfileCapable {
		return ""
	}
	return effortLow
}
```

**`llmprovider/options.go`** makes two additions:

* The field `ModelProfile ModelProfile` goes at the end of `ProviderConfig`
  (`options.go:24-45`), with this comment:
  ```go
  	// ModelProfile selects how the recommended models of the open catalogs
  	// (Kilo, OpenCode Zen and Go, Hugging Face) are ranked. The zero value is
  	// ProfileUtility. Ignored by provider constructors.
  ```
* This function goes after `WithKiloCapabilities`:
  ```go
  // WithModelProfile selects how ListAvailableModels and ListModelCatalog rank
  // the recommended models of the open catalogs (MADR 0010 §1). Ignored by
  // provider constructors.
  func WithModelProfile(p ModelProfile) ProviderOption {
  	return func(cfg *ProviderConfig) {
  		cfg.ModelProfile = p
  	}
  }
  ```

**`llmprovider/provider.go`** replaces the body of `GenerateWithRetry`
(`provider.go:126-173`) with a generic loop. The loop keeps its statements,
constants, jitter, Retry-After handling and terminal-error rule exactly.
`GenerateItemsWithRetry` (`:175-222`) is not touched (§8).

```go
// GenerateWithRetry executes a Generate call with the specified number of retries
// and jittered delay. It will stop retrying if the context is cancelled.
func GenerateWithRetry(ctx context.Context, p Provider, prompt string, retries int, delay time.Duration) (string, error) {
	return retryWithBackoff(ctx, retries, delay, "llm: retrying after failure", func() (string, error) {
		return p.Generate(ctx, prompt)
	})
}

// GenerateThinkingWithRetry is GenerateWithRetry for the extended-thinking
// path: the same backoff, jitter and error classification around
// GenerateThinking (MADR 0010 §6).
func GenerateThinkingWithRetry(ctx context.Context, p ThinkingProvider, prompt string, retries int, delay time.Duration) (string, error) {
	return retryWithBackoff(ctx, retries, delay, "llm: retrying thinking after failure", func() (string, error) {
		return p.GenerateThinking(ctx, prompt)
	})
}

// retryWithBackoff runs call up to retries+1 times with exponential, jittered
// backoff capped at 30s, honouring a server-directed Retry-After. It stops at
// once on ErrAuthFailure or ErrInvalidRequest, and on context cancellation.
func retryWithBackoff[T any](ctx context.Context, retries int, delay time.Duration, logMsg string, call func() (T, error)) (T, error) {
	const maxBackoff = 30 * time.Second
	var zero T
	var lastErr error
	for i := 0; i <= retries; i++ {
		if i > 0 {
			// … the backoff block of today's GenerateWithRetry, verbatim, with
			// slog.Warn(logMsg, …) and `return zero, ctx.Err()` on cancellation.
		}
		res, err := call()
		if err == nil {
			return res, nil
		}
		lastErr = err
		// Non-retryable errors will never succeed — stop immediately.
		if errors.Is(err, ErrAuthFailure) || errors.Is(err, ErrInvalidRequest) {
			return zero, err
		}
	}
	return zero, fmt.Errorf("failed after %d attempts: %w", retries+1, lastErr)
}
```

The elided block is `provider.go:132-160` with two edits:
* `"llm: retrying after failure"` becomes `logMsg`;
* `return "", ctx.Err()` becomes `return zero, ctx.Err()`.

### 1.3 The ranker (Phase 2, new file `llmprovider/model_ranking.go`)

```go
package llmprovider

import (
	"cmp"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Use-case ranking of the recommended models (MADR 0010 §3–§4). Ranking is a
// pure function of the profile, the candidates and the clock; each lister
// builds candidates from the metadata its catalog publishes.

// rankingNow is the ranking clock. Tests pin it.
var rankingNow = time.Now

// Eligibility thresholds (MADR 0010 §3) and ordering weights (§4).
const (
	maxRankAgeDays    = 540   // 18 months of 30 days
	minRankContext    = 32768 // tokens
	minRankExpiryDays = 30
	maxPerRankGroup   = 2
	rankCostWeight    = 0.8
	rankAgeWeight     = 0.2
	rankDaysPerMonth  = 30.0
	rankNormFloor     = 0.01 // smallest normaliser, as in the reference simulation
	perMillionTokens  = 1e6
)

var (
	// smallModelRE is OpenCode's SMALL_MODEL_RE (packages/core/src/catalog.ts).
	smallModelRE = regexp.MustCompile(`\b(nano|flash|lite|mini|haiku|small|fast)\b`)
	// unstableModelRE marks preview and experimental ids (MADR 0010 §3 item 6).
	unstableModelRE = regexp.MustCompile(`(?i)preview|-exp\b|experimental|alpha`)
	// opencodeGoRegionGated lists the OpenCode Go models served only when the
	// workspace allows region cn (MADR 0010 §3 item 8). They stay searchable.
	opencodeGoRegionGated = []string{"deepseek-v4.1-flash", "deepseek-flash", "deepseek-v4-flash", "deepseek-v4-pro"}
)

// rankCandidate is one usable model with the metadata its catalog publishes.
// Each *Known flag separates "absent" from a zero value; unknown values never
// exclude (MADR 0010 §3).
type rankCandidate struct {
	id    string
	group string
	small bool

	reasoning, reasoningKnown bool
	cost                      float64 // input + output, USD per million tokens
	costKnown                 bool
	ageDays                   int
	ageKnown                  bool
	context                   int // tokens; 0 is unknown
	status                    string
	expiryDays                int
	expiryKnown               bool
	bench                     float64 // Kilo terminalBench.overallScore
	benchKnown                bool
	preferred                 int // Kilo preferredIndex
	preferredKnown            bool
}

// signal reports a provider quality signal (MADR 0010 §4).
func (c rankCandidate) signal() bool { return c.benchKnown || c.preferredKnown }

func (c rankCandidate) ageMonths() float64 { return float64(c.ageDays) / rankDaysPerMonth }

func (c rankCandidate) benchOrZero() float64 {
	if c.benchKnown {
		return c.bench
	}
	return 0
}

func (c rankCandidate) preferredOrInf() float64 {
	if c.preferredKnown {
		return float64(c.preferred)
	}
	return math.Inf(1)
}

func (c rankCandidate) costOrZero() float64 {
	if c.costKnown {
		return c.cost
	}
	return 0
}

// eligible applies MADR 0010 §3 for one provider.
func (c rankCandidate) eligible(provider string) bool {
	switch {
	case c.reasoningKnown && !c.reasoning,
		c.costKnown && c.cost == 0,
		c.ageKnown && c.ageDays > maxRankAgeDays,
		c.context > 0 && c.context < minRankContext,
		c.status == "deprecated", c.status == "alpha",
		c.expiryKnown && c.expiryDays <= minRankExpiryDays,
		unstableModelRE.MatchString(c.id),
		strings.Contains(c.id, "-contributor"):
		return false
	}
	return provider != ProviderOpencodeGo || !slices.Contains(opencodeGoRegionGated, c.id)
}

// rankNorm holds the blend normalisers over the eligible set: the largest
// known cost and age, never below rankNormFloor. An unknown cost or age counts
// as the maximum (MADR 0010 §4).
type rankNorm struct{ maxCost, maxAge float64 }

func newRankNorm(cands []rankCandidate) rankNorm {
	n := rankNorm{maxCost: rankNormFloor, maxAge: rankNormFloor}
	for _, c := range cands {
		if c.costKnown {
			n.maxCost = max(n.maxCost, c.cost)
		}
		if c.ageKnown {
			n.maxAge = max(n.maxAge, c.ageMonths())
		}
	}
	return n
}

func (n rankNorm) cost(c rankCandidate) float64 {
	if c.costKnown {
		return c.cost
	}
	return n.maxCost
}

func (n rankNorm) age(c rankCandidate) float64 {
	if c.ageKnown {
		return c.ageMonths()
	}
	return n.maxAge
}

// blend is OpenCode's 0.8 × cost + 0.2 × age, each normalised by the set's
// maximum.
func (n rankNorm) blend(c rankCandidate) float64 {
	return rankCostWeight*n.cost(c)/n.maxCost + rankAgeWeight*n.age(c)/n.maxAge
}

// utilityOrder: a quality signal first, then small, then lower blend, then id.
func (n rankNorm) utilityOrder(a, b rankCandidate) int {
	return cmp.Or(
		boolFirst(a.signal(), b.signal()),
		boolFirst(a.small, b.small),
		cmp.Compare(n.blend(a), n.blend(b)),
		strings.Compare(a.id, b.id),
	)
}

// capableOrder: a quality signal first, then higher terminalBench, lower
// preferredIndex, not-small, newer, costlier, then id. Unknown bench and cost
// count as 0, unknown preferredIndex as +Inf, unknown age as the maximum.
func (n rankNorm) capableOrder(a, b rankCandidate) int {
	return cmp.Or(
		boolFirst(a.signal(), b.signal()),
		cmp.Compare(b.benchOrZero(), a.benchOrZero()),
		cmp.Compare(a.preferredOrInf(), b.preferredOrInf()),
		boolFirst(!a.small, !b.small),
		cmp.Compare(n.age(a), n.age(b)),
		cmp.Compare(b.costOrZero(), a.costOrZero()),
		strings.Compare(a.id, b.id),
	)
}

// boolFirst orders true before false.
func boolFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// utilityExcluded reports ids the utility profile never recommends, ranked or
// filled: Kilo's kilo-auto/* managed tiers (MADR 0010 §3 item 9, a maintainer
// decision). They stay searchable and eligible under ProfileCapable.
func utilityExcluded(profile ModelProfile, provider, id string) bool {
	return profile != ProfileCapable && provider == ProviderKilo && strings.HasPrefix(id, "kilo-auto/")
}

// rankRecommended returns at most MaxListedModels ids: the eligible
// candidates in profile order, at most maxPerRankGroup per group, then fill in
// order, skipping ids already chosen or excluded (MADR 0010 §4).
func rankRecommended(profile ModelProfile, provider string, cands []rankCandidate, fill []string) []string {
	var eligible []rankCandidate
	for _, c := range cands {
		if c.eligible(provider) && !utilityExcluded(profile, provider, c.id) {
			eligible = append(eligible, c)
		}
	}
	norm := newRankNorm(eligible)
	if profile == ProfileCapable {
		slices.SortFunc(eligible, norm.capableOrder)
	} else {
		slices.SortFunc(eligible, norm.utilityOrder)
	}
	out := make([]string, 0, MaxListedModels)
	perGroup := map[string]int{}
	for _, c := range eligible {
		if len(out) == MaxListedModels {
			break
		}
		if perGroup[c.group] >= maxPerRankGroup {
			continue
		}
		perGroup[c.group]++
		out = append(out, c.id)
	}
	for _, id := range fill {
		if len(out) == MaxListedModels {
			break
		}
		if !slices.Contains(out, id) && !utilityExcluded(profile, provider, id) {
			out = append(out, id)
		}
	}
	return out
}

// rankGroup is the diversity group: the vendor prefix before "/" with any
// leading "~" removed, else the family, else the id itself.
func rankGroup(id, family string) string {
	if strings.Contains(id, "/") {
		vendor, _, _ := strings.Cut(strings.TrimLeft(id, "~"), "/")
		return vendor
	}
	if family != "" {
		return family
	}
	return id
}

// isSmallModel applies smallModelRE to the lower-cased, space-joined parts.
func isSmallModel(parts ...string) bool {
	return smallModelRE.MatchString(strings.ToLower(strings.Join(parts, " ")))
}

// floorDays is the whole days from a to b, rounded down, as Python's
// timedelta.days (the reference simulation).
func floorDays(a, b time.Time) int { return int(math.Floor(b.Sub(a).Hours() / 24)) }

// parseRankDate reads YYYY-MM-DD, YYYY-MM or YYYY at UTC midnight; a missing
// month or day is 1 (models.dev writes "2026-07" for some Hugging Face models).
func parseRankDate(s string) (time.Time, bool) {
	for _, layout := range []string{time.DateOnly, "2006-01", "2006"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// kiloCandidate reads one Kilo listing entry (MADR 0010 §2). A created of 0
// or less is unknown: Kilo reports 0 for every kilo-auto tier.
func kiloCandidate(e kiloCatalogEntry, now time.Time) rankCandidate {
	c := rankCandidate{
		id:             e.ID,
		group:          rankGroup(e.ID, ""),
		small:          isSmallModel(e.ID, e.Name),
		reasoning:      slices.Contains(e.SupportedParameters, jsonKeyReasoning),
		reasoningKnown: true,
		context:        e.ContextLength,
	}
	prompt, promptOK := kiloPrice(e.Pricing.Prompt)
	completion, completionOK := kiloPrice(e.Pricing.Completion)
	if promptOK && completionOK {
		c.cost, c.costKnown = (prompt+completion)*perMillionTokens, true
	}
	if e.Created > 0 {
		c.ageDays, c.ageKnown = floorDays(time.Unix(e.Created, 0), now), true
	}
	if t, ok := parseRankDate(e.ExpirationDate); ok {
		c.expiryDays, c.expiryKnown = floorDays(now, t), true
	}
	if e.TerminalBench != nil && e.TerminalBench.OverallScore != nil {
		c.bench, c.benchKnown = *e.TerminalBench.OverallScore, true
	}
	if e.PreferredIndex != nil {
		c.preferred, c.preferredKnown = *e.PreferredIndex, true
	}
	return c
}

// kiloPrice parses one Kilo per-token price. An empty price is 0; a negative
// ("-1", the variable-priced kilo-auto tiers) or unparseable one is unknown.
func kiloPrice(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, true
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil && v >= 0
}
```

**Amended by the 2026-09-26 goconst deviation (§10).** `opencodeGoRegionGated`
lists `opencodeDeepSeekV4Flash`, declared as:

```go
// opencodeDeepSeekV4Flash is named because the id recurs across the Go gate,
// the static catalogs and the tests (goconst).
const opencodeDeepSeekV4Flash = "deepseek-v4-flash"
```

It replaces the literal `"deepseek-v4-flash"` in the list above. Tests use the
constant wherever they need that id as a Go string. ~~One constant~~
**Extended by the second goconst deviation (§10):** all four gated ids are
named constants (`opencodeDeepSeekV41Flash`, `opencodeDeepSeekFlash`,
`opencodeDeepSeekV4Flash`, `opencodeDeepSeekV4Pro`), and the list is built only
from them.

**Fidelity to the reference simulation.** The simulation that produced MADR
§7 applies the same units and floors, in the same order of operations:
* cost in USD per million tokens, with a floor of 0.01;
* age as floor-days ÷ 30, with a floor of 0.01 months;
* `0.8*cost/maxCost + 0.2*age/maxAge`;
* tuple keys that end in the id.

`floorDays` rounds down, matching Python's `timedelta.days`. Kilo `created`
values fall mid-day, so truncation would disagree for a model created on the
snapshot day.

### 1.4 Kilo listing wiring (Phase 2, `llmprovider/discovery.go`)

`kiloCatalogEntry` (`discovery.go:628-641`) becomes the following. Its comment
now reads "Shared by listKiloModels, KiloModelCapabilities and the ranker
(MADR 0010 §2)."

```go
type kiloCatalogEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Created      int64  `json:"created"`
	Architecture struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	ContextLength         int                `json:"context_length"`
	ExpirationDate        string             `json:"expiration_date"`
	PreferredIndex        *int               `json:"preferredIndex"`
	TerminalBench         *kiloTerminalBench `json:"terminalBench"`
	SupportedParameters   []string           `json:"supported_parameters"`
	MayTrainOnYourPrompts bool               `json:"mayTrainOnYourPrompts"`
}

// kiloTerminalBench is the benchmark block Kilo publishes for some models.
type kiloTerminalBench struct {
	OverallScore *float64 `json:"overallScore"`
}
```

After `curateKilo` (`discovery.go:744-747`), add:

```go
// kiloCurate ranks Kilo's usable models from the listing's own metadata
// (MADR 0010 §2). When fewer than MaxListedModels are eligible, the rest come
// from curateKilo's order, then the usable list.
func kiloCurate(entries []kiloCatalogEntry, profile ModelProfile) func([]string) []string {
	return func(usable []string) []string {
		byID := make(map[string]kiloCatalogEntry, len(entries))
		for _, e := range entries {
			byID[e.ID] = e
		}
		now := rankingNow()
		cands := make([]rankCandidate, 0, len(usable))
		for _, id := range usable {
			e, ok := byID[id]
			if !ok {
				e.ID = id
			}
			cands = append(cands, kiloCandidate(e, now))
		}
		return rankRecommended(profile, ProviderKilo, cands, append(curateKilo(usable), usable...))
	}
}
```

In `modelCatalogFor`'s Kilo case (`discovery.go:130-134`), replace the
`curateKilo` argument with `kiloCurate(entries, cfg.ModelProfile)`. Change
nothing else there. `kiloUsable`, `curateKilo` and `kiloPriceRank` stay: they
are the usable order and the fill order.

### 1.5 The metadata client (Phase 3, new file `llmprovider/model_metadata.go`)

```go
package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Model metadata for the open catalogs (MADR 0010 §2): a models.dev-format
// document, by default OpenCode's, fetched concurrently with a listing and
// cached in-process.
const (
	defaultModelMetadataURL = "https://models.opencode.ai/api.json"
	envModelMetadataURL     = "MCPLIB_MODELS_METADATA_URL"
	envDisableModelMetadata = "MCPLIB_DISABLE_MODELS_METADATA"
	modelMetadataTTL        = 10 * time.Minute

	metadataKeyZen = "opencode"
	metadataKeyGo  = "opencode-go"
	metadataKeyHF  = "huggingface"
)

// errModelMetadataDisabled reports that the environment turned the fetch off.
var errModelMetadataDisabled = errors.New("model metadata: disabled by " + envDisableModelMetadata)

// modelCost is a models.dev price, USD per million tokens.
type modelCost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// modelMetadata is the subset of one models.dev model entry the ranker reads.
type modelMetadata struct {
	Name        string     `json:"name"`
	Family      string     `json:"family"`
	Reasoning   *bool      `json:"reasoning"`
	Cost        *modelCost `json:"cost"`
	Limit       struct {
		Context int `json:"context"`
	} `json:"limit"`
	ReleaseDate string `json:"release_date"`
	Status      string `json:"status"`
}

// modelMetadataSection is one provider's entry in the document.
type modelMetadataSection struct {
	Models map[string]modelMetadata `json:"models"`
}

// modelMetadataDoc maps a document key to its models by id.
type modelMetadataDoc map[string]map[string]modelMetadata

// modelMetadataKey returns the document key for a provider, or "".
func modelMetadataKey(provider string) string {
	switch provider {
	case ProviderOpencodeZen:
		return metadataKeyZen
	case ProviderOpencodeGo:
		return metadataKeyGo
	case ProviderHuggingFace:
		return metadataKeyHF
	}
	return ""
}

type modelMetadataCacheEntry struct {
	doc     modelMetadataDoc
	fetched time.Time
}

var (
	modelMetadataMu    sync.Mutex
	modelMetadataCache = map[string]modelMetadataCacheEntry{}
)

// modelMetadataURL resolves the document URL: the option, then the
// environment, then OpenCode's.
func modelMetadataURL(cfg ProviderConfig) string {
	if cfg.ModelMetadataURL != "" {
		return cfg.ModelMetadataURL
	}
	if v := os.Getenv(envModelMetadataURL); v != "" {
		return v
	}
	return defaultModelMetadataURL
}

// modelMetadataDisabled reports whether MCPLIB_DISABLE_MODELS_METADATA holds
// a true boolean ("1", "true", …).
func modelMetadataDisabled() bool {
	v, err := strconv.ParseBool(os.Getenv(envDisableModelMetadata))
	return err == nil && v
}

// loadModelMetadata returns the document, from the in-process cache while it
// is younger than modelMetadataTTL. A failure is returned, never cached.
func loadModelMetadata(ctx context.Context, cfg ProviderConfig) (modelMetadataDoc, error) {
	if modelMetadataDisabled() {
		return nil, errModelMetadataDisabled
	}
	url := modelMetadataURL(cfg)
	modelMetadataMu.Lock()
	e, ok := modelMetadataCache[url]
	modelMetadataMu.Unlock()
	if ok && time.Since(e.fetched) < modelMetadataTTL {
		return e.doc, nil
	}
	doc, err := fetchModelMetadata(ctx, url, cfg.HTTPClient)
	if err != nil {
		return nil, err
	}
	modelMetadataMu.Lock()
	modelMetadataCache[url] = modelMetadataCacheEntry{doc: doc, fetched: time.Now()}
	modelMetadataMu.Unlock()
	return doc, nil
}

// fetchModelMetadata performs one GET. Go's transport requests gzip itself.
func fetchModelMetadata(ctx context.Context, url string, client *http.Client) (modelMetadataDoc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("model metadata: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("model metadata: %w", err)
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model metadata: %s returned HTTP %d", url, resp.StatusCode)
	}
	return decodeModelMetadata(resp.Body)
}

// decodeModelMetadata keeps the three sections MADR 0010 reads. A section
// without models is treated as absent.
func decodeModelMetadata(r io.Reader) (modelMetadataDoc, error) {
	var raw struct {
		Zen *modelMetadataSection `json:"opencode"`
		Go  *modelMetadataSection `json:"opencode-go"`
		HF  *modelMetadataSection `json:"huggingface"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("model metadata: decode: %w", err)
	}
	doc := modelMetadataDoc{}
	for key, s := range map[string]*modelMetadataSection{
		metadataKeyZen: raw.Zen, metadataKeyGo: raw.Go, metadataKeyHF: raw.HF,
	} {
		if s != nil && s.Models != nil {
			doc[key] = s.Models
		}
	}
	return doc, nil
}

// modelMetadataResult carries one fetch's outcome across a goroutine.
type modelMetadataResult struct {
	doc modelMetadataDoc
	err error
}

// startModelMetadata fetches the document concurrently with a listing, under
// the listing's context (MADR 0010 §2). The channel receives exactly one value.
func startModelMetadata(ctx context.Context, cfg ProviderConfig) <-chan modelMetadataResult {
	ch := make(chan modelMetadataResult, 1)
	go func() {
		doc, err := loadModelMetadata(ctx, cfg)
		ch <- modelMetadataResult{doc: doc, err: err}
	}()
	return ch
}

// metadataCandidate reads one models.dev entry (MADR 0010 §2). An id the
// document does not cover is a candidate with every field unknown.
func metadataCandidate(id string, m modelMetadata, covered bool, now time.Time) rankCandidate {
	if !covered {
		return rankCandidate{id: id, group: rankGroup(id, ""), small: isSmallModel(id)}
	}
	c := rankCandidate{
		id:      id,
		group:   rankGroup(id, m.Family),
		small:   isSmallModel(id, m.Family, m.Name),
		context: m.Limit.Context,
		status:  m.Status,
	}
	if m.Reasoning != nil {
		c.reasoning, c.reasoningKnown = *m.Reasoning, true
	}
	if m.Cost != nil {
		c.cost, c.costKnown = m.Cost.Input+m.Cost.Output, true
	}
	if t, ok := parseRankDate(m.ReleaseDate); ok {
		c.ageDays, c.ageKnown = floorDays(t, now), true
	}
	return c
}

// metadataCurate ranks a provider's usable models with the metadata document
// (MADR 0010 §2). A failed or disabled fetch, or a document without the
// provider's key, returns fallback's curation unchanged.
func metadataCurate(provider string, profile ModelProfile, meta <-chan modelMetadataResult, fallback func([]string) []string) func([]string) []string {
	return func(usable []string) []string {
		res := <-meta
		models, ok := res.doc[modelMetadataKey(provider)]
		if res.err != nil || !ok {
			return fallback(usable)
		}
		now := rankingNow()
		cands := make([]rankCandidate, 0, len(usable))
		for _, id := range usable {
			m, covered := models[id]
			cands = append(cands, metadataCandidate(id, m, covered, now))
		}
		return rankRecommended(profile, provider, cands, append(fallback(usable), usable...))
	}
}
```

**`llmprovider/options.go`** gets the Phase 3 additions:

* The field `ModelMetadataURL string` goes after `ModelProfile`, with this
  comment:
  ```go
  	// ModelMetadataURL overrides the models.dev-format document the open
  	// catalogs are ranked with, and OpenCode's chat route reads
  	// reasoning_options from. Empty uses MCPLIB_MODELS_METADATA_URL, then
  	// https://models.opencode.ai/api.json.
  ```
* This function goes after `WithModelProfile`:
  ```go
  // WithModelMetadataURL overrides the model metadata document (MADR 0010 §2).
  // MCPLIB_DISABLE_MODELS_METADATA=1 turns the fetch off whatever the URL.
  func WithModelMetadataURL(url string) ProviderOption {
  	return func(cfg *ProviderConfig) {
  		cfg.ModelMetadataURL = url
  	}
  }
  ```

The "reasoning_options" wording anticipates Phase 6. The field is read in
Phase 3 by `modelMetadataURL`.

### 1.6 Zen, Go and Hugging Face wiring (Phase 3, `llmprovider/discovery.go`)

* **Hugging Face.** `modelCatalogFor`'s Hugging Face case
  (`discovery.go:127-129`) becomes:
  ```go
  	case ProviderHuggingFace:
  		meta := startModelMetadata(ctx, cfg)
  		usable, err := fetchHuggingFaceUsable(ctx, apiKey, cfg)
  		return catalogFrom(usable, err, StaticModels(ProviderHuggingFace),
  			metadataCurate(ProviderHuggingFace, cfg.ModelProfile, meta, curateHuggingFace)), nil
  ```
* **Zen and Go.** In `opencodeCatalog` (`discovery.go:475-484`):
  * start `meta := startModelMetadata(ctx, cfg)` after the `opencodeBaseURL`
    check;
  * keep the existing `curate` closure unchanged;
  * return
    `catalogFrom(usable, fetchErr, StaticModels(gateway), metadataCurate(gateway, cfg.ModelProfile, meta, curate)), nil`.
* **No work leaks.** The channel is buffered, so a listing that fails before
  curation leaves no goroutine blocked. The fetch ends with the listing's
  context.

### 1.7 Static catalogs (Phase 4, `llmprovider/models_catalog.go:64-113`)

Replace the four lists and their comments. Leave the other lists untouched.

```go
	// StaticOpencodeZen: MADR 0010 §7's utility six, ranked from the
	// 2026-09-26 Zen listing and models.opencode.ai metadata. Routes verified
	// against api.json's npm packages on 2026-09-26.
	StaticOpencodeZen = []string{
		"deepseek-v4.1-flash",   // chat_completions
		"qwen3.8-flash",         // messages
		"glm-5.3-flash",         // chat_completions
		"deepseek-v4-flash",     // chat_completions
		"gemini-3.5-flash-lite", // google
		"gemini-3.8-flash",      // google
	}

	// StaticOpencodeGo: MADR 0010 §7's utility six (2026-09-26). It excludes
	// the region-gated DeepSeek models and the -contributor models.
	StaticOpencodeGo = []string{
		"mimo-v2.6-flash", // chat_completions
		"qwen3.8-flash",   // messages
		"glm-5.3-flash",   // chat_completions
		"gpt-6-luna",      // responses
		"mimo-v2.6-pro",   // chat_completions
		"hy3",             // chat_completions
	}

	// StaticHuggingFace: fallback only — discovery is metadata-driven. MADR
	// 0010 §7's utility six, from the 2026-09-26 router listing and
	// models.opencode.ai metadata: reasoning-capable, paid, recent.
	StaticHuggingFace = []string{
		"deepseek-ai/DeepSeek-V4-Flash-0731",
		"zai-org/GLM-5.3-Flash",
		"deepseek-ai/DeepSeek-V4.1-Flash",
		"thinkingmachines/Inkling-Small",
		"stepfun-ai/Step-3.7-Flash",
		"stepfun-ai/Step-3.5-Flash",
	}

	// StaticKilo: fallback only — discovery is metadata-driven. MADR 0010 §7's
	// utility six, from the 2026-09-26 listing: reasoning-capable, paid,
	// recent, at most two per vendor, none training on prompts.
	StaticKilo = []string{
		"deepseek/deepseek-v4.1-flash",
		"z-ai/glm-5.3-flash",
		"google/gemini-3.8-flash",
		"google/gemini-3.6-flash",
		"meta/muse-spark-1.2",
		"thinkingmachines/inkling",
	}
```

All twelve Zen and Go ids resolve to the route their `api.json` npm package
names (Appendix 1, E7), so `opencode_route.go` needs no edit. `modelLabels` is
unchanged; the new ids display as bare ids (`models_catalog.go:638-640`).

### 1.8 Wizard (Phase 4, `wizard/configure.go`)

* **New field.** `Options` (`configure.go:38-65`) gains a field after
  `NeedFallbacks`:
  ```go
  	// Profile selects how the open catalogs (Kilo, OpenCode Zen and Go,
  	// Hugging Face) rank the recommended models. The zero value,
  	// llmprovider.ProfileUtility, suits short frequent tasks such as commit
  	// messages; llmprovider.ProfileCapable suits reasoning-heavy tiers.
  	Profile llmprovider.ModelProfile
  ```
* **Forwarding.** In `discoverModels`, replace `var opts []llmprovider.ProviderOption`
  (`configure.go:283`) with
  `opts := []llmprovider.ProviderOption{llmprovider.WithModelProfile(o.Profile)}`.

### 1.9 Kilo reasoning shapes (Phase 5)

**`llmprovider/chatcompletions.go`:**
* **New field.** `chatCompletionsOpts` gains, after `ReasoningEffort`:
  ```go
  	// Reasoning, when non-nil, is sent as the OpenRouter-style reasoning
  	// object Kilo reads: {"effort": …} or {"enabled": true}.
  	Reasoning map[string]any
  ```
* **Body.** `chatCompletionsBody`, after its `reasoning_effort` block
  (`:75-77`), adds:
  ```go
  	if o.Reasoning != nil {
  		body[jsonKeyReasoning] = o.Reasoning
  	}
  ```

**`llmprovider/kilo.go`:**
* **Method.** Add:
  ```go
  // thinkingFields returns the reasoning fields for one call (MADR 0010 §6).
  // When the model accepts "reasoning" (or its capabilities are unknown), Kilo's
  // reasoning object is sent: {effort} when an effort is configured, else
  // {enabled: true} for the model's default effort. reasoning_effort is sent
  // only when the model lists it and not "reasoning".
  func (p *KiloProvider) thinkingFields(thinking bool) (effort string, reasoning map[string]any) {
  	switch {
  	case !thinking:
  		return "", nil
  	case p.supports(jsonKeyReasoning):
  		if p.reasoningEffort != "" {
  			return "", map[string]any{jsonKeyEffort: p.reasoningEffort}
  		}
  		return "", map[string]any{jsonKeyEnabled: true}
  	case p.supports(jsonKeyReasoningEffort):
  		if p.reasoningEffort != "" {
  			return p.reasoningEffort, nil
  		}
  		return effortMedium, nil
  	}
  	return "", nil
  }
  ```
* **Call site.** `doGenerateItems` (`:151-165`) replaces its `effort` block with
  `effort, reasoning := p.thinkingFields(thinking)`, and passes
  `ReasoningEffort: effort, Reasoning: reasoning`.
* **Doc comments.** At `:104`, `:122` and `:141`, "with reasoning_effort"
  becomes "with reasoning enabled".

**Two comment updates:**
* `options.go`'s `WithKiloCapabilities` comment: `"reasoning_effort" for the
  thinking path` becomes `"reasoning" and "reasoning_effort" for the thinking
  path`.
* `constants.go:50`: `// jsonKeyReasoning is the Responses API reasoning block
  (OpenCode responses route) and Kilo's reasoning object.`

### 1.10 OpenCode chat-route reasoning (Phase 6)

**`llmprovider/model_metadata.go`** adds the fields and method that only
Phase 6 reads:

```go
// modelReasoningOption is one models.dev reasoning_options entry.
type modelReasoningOption struct {
	Type   string   `json:"type"`
	Values []string `json:"values"`
}
```

* `modelMetadata` gains
  `ReasoningOptions []modelReasoningOption \`json:"reasoning_options"\``.
* The method:
  ```go
  // reasoningEfforts returns the effort values the document lists for one
  // model's reasoning_options, or nil.
  func (d modelMetadataDoc) reasoningEfforts(provider, model string) []string {
  	for _, o := range d[modelMetadataKey(provider)][model].ReasoningOptions {
  		if o.Type == jsonKeyEffort {
  			return o.Values
  		}
  	}
  	return nil
  }
  ```

**`llmprovider/opencode.go`:**
* **Metadata URL.** `OpencodeProvider` gains `metadataURL string`, set from
  `cfg.ModelMetadataURL` in `NewOpencode`.
* **Method.** Add, and add `"slices"` to the imports:
  ```go
  // chatReasoningEffort returns the reasoning_effort for the chat route: the
  // configured effort, when this is a thinking call and the model's published
  // reasoning_options list it (MADR 0010 §6); otherwise "". Metadata that is
  // unavailable, disabled or silent on the model sends nothing.
  func (p *OpencodeProvider) chatReasoningEffort(ctx context.Context, thinking bool) string {
  	if !thinking || p.reasoningEffort == "" {
  		return ""
  	}
  	doc, err := loadModelMetadata(ctx, ProviderConfig{HTTPClient: p.client, ModelMetadataURL: p.metadataURL})
  	if err != nil || !slices.Contains(doc.reasoningEfforts(p.gateway, p.model), p.reasoningEffort) {
  		return ""
  	}
  	return p.reasoningEffort
  }
  ```
* **`chatBody`** (`:222-231`) becomes:
  ```go
  // chatBody delegates to the shared primitive. The chat route carries
  // reasoning_effort only when chatReasoningEffort resolves one (MADR 0010 §6);
  // the DeepSeek/GLM/Kimi/MiniMax families routed there share no other
  // portable reasoning parameter. Asserted by TestOpencode_Thinking_PerRoute
  // and TestOpencode_ChatReasoningEffort.
  func (p *OpencodeProvider) chatBody(input []Item, tool *Tool, effort string) map[string]any {
  	return chatCompletionsBody(p.model, p.maxTokens, input, chatCompletionsOpts{
  		Tool:            tool,
  		ForceTool:       tool != nil,
  		ReasoningEffort: effort,
  	})
  }
  ```
* **Call site.** `doGenerateItems` (`:243`) becomes
  `body = p.chatBody(input, tool, p.chatReasoningEffort(ctx, thinking))`.

**`chatcompletions.go:18-19`.** The `ReasoningEffort` comment's second sentence
becomes: "OpenCode's chat route sets it only when the model's published
reasoning_options list the configured effort (MADR 0010 §6)."

### 1.11 Choices this plan makes where the MADR is silent

Each was flagged for the approver. Changing one after approval is a
deviation.

> **Maintainer decisions (2026-09-26).**
>
> * **Item 1 (lazy lookup in the provider):** approved as written.
> * **Item 6 (MADR §7 as a committed golden test):** approved as written.
> * **`kilo-auto/*` tiers:** never in the utility six. A user who wants one
>   searches for it and adds it.
>   * The ranking rules alone did not guarantee this. The plan's own Kilo
>     fixture put `kilo-auto/efficient` fourth in the utility six, and a
>     sparse catalog could also add one through the fill.
>   * The decision is therefore an explicit rule: `utilityExcluded` (§1.3),
>     MADR §3 item 9. It applies to Kilo's utility profile only, both ranked
>     and filled.
>   * Re-running the simulation with the rule leaves MADR §7 unchanged. The
>     Kilo fixture's utility expectation changes (Phase 2), and
>     `TestRankRecommended_KiloAutoUtility` pins the rule.
> * **Capable profile's effort:** no change was requested. It stays as the MADR
>   states: the model's own default (`ReasoningEffort()` returns `""`).

1. **Where OpenCode's chat route gets `reasoning_options`: a lazy lookup inside
   the provider** (§1.10). It uses the shared cached client, and only on a
   thinking call on the chat route with an effort configured.
   * *Why:* MADR §6's recipe (effort plus `GenerateThinkingWithRetry`) then
     works with no consumer change.
   * *Cost:* the first such call in a process makes one metadata GET (about
     0.2 s and 489 KB gzip on 2026-09-25) inside the request's context.
   * *Rejected:* a caller-supplied option like `WithKiloCapabilities`. It adds
     API surface, and the recipe would silently send nothing unless every
     consumer also looked the values up.
2. **Units and floors mirror the reference simulation** (§1.3). The Go code
   then reproduces MADR §7 exactly (Phase 3's snapshot test), rather than
   approximately.
3. **Metadata failures are not cached.** A later listing in the same process
   retries. A success is cached for 10 minutes per URL.
4. **`MCPLIB_DISABLE_MODELS_METADATA` accepts any `strconv.ParseBool` true
   value.** MADR §2 names `=1`, and `1` is one of them.
5. **`GenerateItemsWithRetry` keeps its own loop.** MADR §6 names two helpers
   sharing one loop. 0012 §1.2 extends the loop to the third.
6. **MADR §7's tables become a committed golden test.** The five 2026-09-26
   snapshot files (319,322 bytes in total, trimmed to the fields mcplib
   decodes) are committed as test data.
   * *Why:* this makes the §7 tables executable, and a future rule change must
     change them visibly.
   * *Source:* `$SCRATCH/snapshot-trim/`, produced by Appendix 2 from the
     saved raw responses. Trimming changes nothing: both views of all four
     providers, and every simulated six, are identical (Appendix 1, E4).
   * *Digests:* checked before copying (Phase 3 step 1).

## 2. Phase sequencing

| Phase | Deliverable | New files | Modified files |
|---|---|---|---|
| 0 | MADR accepted, plan in progress, MADR 0003 annotated | — | `docs/0010-MADR-…`, `docs/0010-PLAN-…`, `docs/0003-MADR-add-gateway-llm-providers.md` |
| 1 | Profiles and `GenerateThinkingWithRetry` | `llmprovider/model_profile.go`, `llmprovider/model_profile_test.go`, `llmprovider/retry_thinking_test.go` | `llmprovider/options.go`, `llmprovider/provider.go` |
| 2 | Ranker and Kilo ranking | `llmprovider/model_ranking.go`, `llmprovider/model_ranking_test.go`, `llmprovider/discovery_ranking_test.go` | `llmprovider/discovery.go` |
| 3 | Metadata client; Zen, Go and HF ranking; isolation; golden snapshot | `llmprovider/model_metadata.go`, `llmprovider/model_metadata_test.go`, `llmprovider/main_test.go`, `wizard/main_test.go`, `llmprovider/testdata/ranking-2026-09-26/{kilo,zen,go,hf,metadata}.json` | `llmprovider/discovery.go`, `llmprovider/options.go`, `llmprovider/discovery_ranking_test.go`, `llmprovider/live_gateways_test.go` |
| 4 | Static catalogs and `Options.Profile` | — | `llmprovider/models_catalog.go`, `llmprovider/model_ranking_test.go`, `wizard/configure.go`, `wizard/model_select_test.go` |
| 5 | Kilo reasoning shapes, behind a live gate | — | `llmprovider/kilo.go`, `llmprovider/chatcompletions.go`, `llmprovider/constants.go`, `llmprovider/options.go`, `llmprovider/kilo_test.go`, `llmprovider/live_gateways_test.go` |
| 6 | OpenCode chat `reasoning_effort`, behind a live gate | — | `llmprovider/opencode.go`, `llmprovider/model_metadata.go`, `llmprovider/chatcompletions.go`, `llmprovider/opencode_test.go`, `llmprovider/live_gateways_test.go` |
| 7 | README and close-out | — | `README.md`, `docs/0010-PLAN-…` |

**Dependencies:**
* 1 → 2 → 3 → 4, and 5 needs 1;
* 6 needs 3;
* 7 needs all.

**Order of execution:** 0, 1, 2, 3, 4, 5, 6, 7.

**Why these boundaries:**
* The ranker lands with its first production caller, Kilo, so `unused` never
  sees it caller-less.
* The metadata fetch lands with the `TestMain` files that keep both test
  packages off the network.
* The static lists land with the wizard fixtures that depend on them.

## Phase 0 — Record acceptance

1. Set the MADR frontmatter to `status: accepted` and `date:` the execution
   date. Set this plan to `status: in-progress` with the same date.
2. In `docs/0003-MADR-add-gateway-llm-providers.md`, insert this blockquote
   after the 0009 pointer (line 11), separated by a blank line:
   `> **Partially superseded (<execution date>) by [0010-MADR-use-case-aware-default-model-ranking.md](0010-MADR-use-case-aware-default-model-ranking.md) §4–§5:** Kilo's price-ascending and Hugging Face's throughput-descending orders no longer choose the recommended six; they remain the fallback order when ranking cannot run and the fill order when fewer than six models are eligible. The static catalogs chosen here are replaced. The filters, training policy and tools rule below stand.`
   Change nothing else in 0003.
3. Stage the three files, then `git commit --no-edit`. The files are Markdown,
   so the phase gate does not apply.

## Phase 1 — Profiles and `GenerateThinkingWithRetry`

### Steps

1. **Red tests first.** Create the two test files below, then run
   `go test -count=1 -run 'TestModelProfile|TestGenerateThinkingWithRetry|TestWithModelProfile' ./llmprovider > "$SCRATCH/p1-red.log" 2>&1`.
   It must fail to compile (`undefined: ProfileCapable`). Copy the line to
   §11.
2. Implement §1.2.
3. Re-run step 1's command, which must pass. Then run
   `go test -count=1 -run 'Retry' ./llmprovider > "$SCRATCH/p1-retry.log" 2>&1`.
   The pre-existing `TestGenerateWithRetry_*`,
   `TestGenerateItemsWithRetry_*` and `TestOpencode_RetryOnRateLimit` must
   pass unmodified.
4. Mutation proofs, then the phase gate on
   `llmprovider/model_profile.go llmprovider/model_profile_test.go llmprovider/retry_thinking_test.go llmprovider/options.go llmprovider/provider.go`.
5. Stage those five files, then `git commit --no-edit`.

### Tests

**`llmprovider/model_profile_test.go`:**
* `TestModelProfile_ReasoningEffort`:
  * `ProfileUtility.ReasoningEffort() == "low"`;
  * `ProfileCapable.ReasoningEffort() == ""`;
  * `ModelProfile(7).ReasoningEffort() == "low"`;
  * `ProfileUtility == 0`, compared as the literal `0` against `int(ProfileUtility)`.
* `TestWithModelProfile`: `ApplyOptions(nil).ModelProfile == ProfileUtility`;
  `ApplyOptions([]ProviderOption{WithModelProfile(ProfileCapable)}).ModelProfile == ProfileCapable`.

**`llmprovider/retry_thinking_test.go`:**

The file declares `fakeThinker`:
* `plain` and `thinking int` call counters;
* `errs []error`, one result per thinking call;
* `Generate` increments `plain` and returns
  `errors.New("plain path called")`;
* `GenerateThinking` returns `errs[i]` in turn, then `"thought", nil`.

The tests:
* `TestGenerateThinkingWithRetry_RetriesThenSucceeds`:
  * errs `{ErrRateLimited, ErrProviderUnavailable}` with retries 5 and delay
    `time.Nanosecond`;
  * the result must be `"thought", nil`, with `thinking == 3` and
    `plain == 0`.
* `TestGenerateThinkingWithRetry_NonRetryable`: for `ErrAuthFailure` and
  `ErrInvalidRequest`, each wrapped with `%w`, `errors.Is` holds and
  `thinking == 1`.
* `TestGenerateThinkingWithRetry_ContextCancelled`:
  * an already-cancelled context, errs `{ErrRateLimited}`, retries 3, delay
    50ms;
  * the error `errors.Is(err, context.Canceled)`, with `thinking == 1`.
* `TestGenerateThinkingWithRetry_Exhausted`: errs of four `ErrRateLimited`
  with retries 3. The error message starts with `failed after 4 attempts:`,
  and `errors.Is(err, ErrRateLimited)` holds.

### Mutation proofs (`$SCRATCH/p1-mutations.json`, `pkg` `./llmprovider`)

| name | file | old → new | run | expect |
|---|---|---|---|---|
| `p1-thinking-calls-generate` | `llmprovider/provider.go` | `return p.GenerateThinking(ctx, prompt)` → `return p.Generate(ctx, prompt)` | `^TestGenerateThinkingWithRetry_RetriesThenSucceeds$` | fail |
| `p1-all-terminal` | same | `if errors.Is(err, ErrAuthFailure) \|\| errors.Is(err, ErrInvalidRequest) {\n\t\t\treturn zero, err` → `if true {\n\t\t\treturn zero, err` | `^TestGenerateThinkingWithRetry_RetriesThenSucceeds$` | fail |
| `p1-none-terminal` | same | `return zero, err` → `lastErr = err` | `^TestGenerateThinkingWithRetry_NonRetryable$` | fail |
| `p1-capable-medium` | `llmprovider/model_profile.go` | `if p == ProfileCapable {\n\t\treturn ""` → `if p == ProfileCapable {\n\t\treturn effortMedium` | `^TestModelProfile_ReasoningEffort$` | fail |
| `p1-retry-regression` | `llmprovider/provider.go` | `return p.Generate(ctx, prompt)` → `return "", ErrAuthFailure` | `^TestGenerateWithRetry_RetriesThenSucceeds$` | fail |
| `p1-control` | `llmprovider/model_profile.go` | `// default. A value outside the two profiles is treated as ProfileUtility.` → the same + ` (control)` | `^TestModelProfile_ReasoningEffort$` | pass |

## Phase 2 — Ranker and Kilo ranking

### Steps

1. **Red tests first.** Create `llmprovider/model_ranking_test.go` and
   `llmprovider/discovery_ranking_test.go` with the tests below, then run
   `go test -count=1 -run 'TestRankRecommended|TestKiloCandidate|TestListModelCatalog_KiloRanksByProfile' ./llmprovider > "$SCRATCH/p2-red.log" 2>&1`.
   It must fail to compile (`undefined: rankCandidate`). Copy the line to
   §11.
2. Implement §1.3 and §1.4.
3. Re-run step 1's command, which must pass. Then
   `go test -count=1 ./llmprovider > "$SCRATCH/p2-pkg.log" 2>&1` must pass
   ~~**with every pre-existing test unmodified**. The existing Kilo fixture has
   no `reasoning` parameter, so nothing in it is eligible, and the fill
   reproduces today's order (Appendix 1, E9).~~ **Amended by the 2026-09-26
   deviation (§10):** with every pre-existing test unmodified except
   `TestListKiloModels_MetadataCuration`. It now asserts `Usable`
   (`org/cheap, org/vlm, org/dear, kilo-auto/variable`, cheapest first, `"-1"`
   last) and `Recommended` (`org/cheap, org/vlm, org/dear`: the fill skips
   `kilo-auto/*` under the utility profile, per MADR §3 item 9).
4. Mutation proofs, then the phase gate on
   `llmprovider/model_ranking.go llmprovider/model_ranking_test.go llmprovider/discovery_ranking_test.go llmprovider/discovery.go`
   **plus `llmprovider/discovery_test.go`** (2026-09-26 deviation, §10).
5. Stage ~~those four files~~ those five files, then `git commit --no-edit`.

### Tests (`llmprovider/model_ranking_test.go`)

**Helpers:**
* `pinRankingNow(t, now)` sets `rankingNow`, and restores it in `t.Cleanup`.
* `good(id, group string, cost float64, ageDays int)` returns an eligible,
  not-small candidate:
  * `reasoning`, `reasoningKnown`, `costKnown` and `ageKnown` all true;
  * `context` 128000.
* `small(c)` returns a copy with `small = true`.

Every expected list below was computed by the reference simulation that
produced MADR §7 (Appendix 1, E8).

| Test | Candidates (id: group, cost $/M, age days; extras) | Profile | Expected |
|---|---|---|---|
| `TestRankRecommended_UtilityOrder` | `s/bench`: s, 9, 300, bench 0.5; `f/flash-a`: f, 1, 30, small; `g/flash-b`: g, 2, 1, small; `h/flash-c`: h, 2, 1, small; `p/pro`: p, 0.5, 30; `q/max`: q, 4, 60 | utility | `s/bench, f/flash-a, g/flash-b, h/flash-c, p/pro, q/max` |
| `TestRankRecommended_BlendWeighsAge` | `a/flash-old`: a, 2, 540; `b/flash-new`: b, 2.2, 3; `c/flash-max`: c, 4, 30; `d/flash-unk`: d, cost unknown, 3; `e/flash-noage`: e, 1, age unknown; all small | utility | `e/flash-noage, b/flash-new, a/flash-old, d/flash-unk, c/flash-max` |
| `TestRankRecommended_CapableOrder` | `x/top`: x, 5, 60, bench 0.9; `y/second`: y, 50, 30, bench 0.8; `z/pref0`: z, 2, 30, preferred 0; `w/pref3`: w, 1, 30, preferred 3; `v/big`: v, 3, 10; `t/old`: t, 10, 400; `u/flash`: u, 1, 5, small | capable | `x/top, y/second, z/pref0, w/pref3, v/big, t/old` |
| (same test, second assertion) | same candidates | utility | `w/pref3, z/pref0, x/top, y/second, u/flash, v/big` |
| `TestRankRecommended_DiversityCap` | `~a/flash-0`: group from `rankGroup`, 0.9, 10; `a/flash-1`: 1; `a/flash-2`: 1.1; `a/flash-3`: 1.2; `b/flash`: 3; `c/flash`: 4 (all small, 10 days); `d/pro`: 1, 10; `e/pro`: 2, 10 | utility | `~a/flash-0, a/flash-1, b/flash, c/flash, d/pro, e/pro` |
| `TestRankRecommended_Fill` | `m/flash`: m, 1, 10, small; `n/pro`: n, 2, 10; `o/old`: o, 1, 600 (ineligible); fill `o/old, m/flash, p/x, q/y, r/z, s/w` | utility | `m/flash, n/pro, o/old, p/x, q/y, r/z` |

In `TestRankRecommended_DiversityCap`, every candidate's group comes from
`rankGroup(id, "")`, so the tilde case exercises the real function.

**`TestRankRecommended_Eligibility`** is table-driven. There are six base
candidates, `g1/pro` … `g6/pro` (each its own group, cost 5, age 100 days). The
tested candidate `t/flash` defaults to group `t`, cost 1, age 10 days, small,
eligible. It would rank first when eligible. Each row changes one field and
asserts `slices.Contains(result, id) == want` under the named provider (Kilo
unless stated):

| Row | Change | want |
|---|---|---|
| baseline | — | true |
| non-reasoning | `reasoning=false, reasoningKnown=true` | false |
| reasoning unknown | `reasoningKnown=false` | true |
| free | `cost=0` (known) | false |
| cost unknown | `costKnown=false` | true |
| age 541 | `ageDays=541` | false |
| age 540 | `ageDays=540` | true |
| age unknown | `ageKnown=false` | true |
| context 32767 | `context=32767` | false |
| context 32768 | `context=32768` | true |
| context unknown | `context=0` | true |
| deprecated | `status="deprecated"` | false |
| alpha status | `status="alpha"` | false |
| beta | `status="beta"` | true |
| expiring 30 | `expiryDays=30, expiryKnown=true` | false |
| expiring 31 | `expiryDays=31, expiryKnown=true` | true |
| preview id | id `t/flash-preview` | false |
| Preview id | id `t/flash-Preview` | false |
| exp id | id `t/flash-exp` | false |
| expert id | id `t/flash-expert` | true |
| experimental id | id `t/flash-experimental` | false |
| alpha id | id `t/flash-alpha` | false |
| contributor | id `t/flash-contributor` | false |
| Go gate on Go | id `deepseek-v4-flash`, provider `opencode-go` | false |
| Go gate on Zen | id `deepseek-v4-flash`, provider `opencode-zen` | true |

**`TestKiloCandidate_Fields`** decodes JSON into `kiloCatalogEntry`, with now
pinned to `2026-09-26T00:00:00Z`:

* **`kilo-auto/efficient`** (created 0, prices "-1"/"-1", preferredIndex 0,
  `supported_parameters` `["tools","reasoning"]`, context 1000000):
  * `ageKnown`, `costKnown`, `benchKnown` and `small` are false;
  * `preferred` is 0 and known, and `reasoning` is true and known;
  * `group` is `"kilo-auto"`, and `context` is 1000000.
* **`org/flash`** (name `Org: Flash`, created ten days before now, context
  4095, expiration `2026-10-20`, prices `"0.0000003"`/`"0.0000012"`,
  terminalBench 0.75, parameters `["tools"]`):
  * `ageDays` 10, `cost` within 1e-9 of 1.5, `expiryDays` 24;
  * `bench` 0.75, `reasoning` false and known;
  * `small` true, `group` `"org"`.
* **`org/free`** (prices `"0"`/`"0"`): cost 0, known.
* **`org/blank`** (prices `""`/`""`): cost 0, known, as in the reference
  simulation.
* **`org/bad`** (prompt `"abc"`): cost unknown.
* **`~org/tilde`**: group `"org"`.

**`TestRankGroup`** is table-driven over `rankGroup(id, family)`:

| id | family | group |
|---|---|---|
| `~a/x` | `""` | `a` |
| `v/x` | `fam` | `v` |
| `x` | `fam` | `fam` |
| `x` | `""` | `x` |

**`TestRankRecommended_KiloAutoUtility`** (MADR §3 item 9) has these
candidates:
* `kilo-auto/small`: group `kilo-auto`, small, cost 0.45, age unknown,
  eligible;
* five goods, `g1/pro` … `g5/pro`: each its own group, cost 5, age 100 days.

The fill is `kilo-auto/free, x/y`. Expected, from the reference simulation
with the rule:

| profile | provider | expected |
|---|---|---|
| utility | kilo | `g1/pro, g2/pro, g3/pro, g4/pro, g5/pro, x/y` |
| capable | kilo | `g1/pro, g2/pro, g3/pro, g4/pro, g5/pro, kilo-auto/small` |
| utility | opencode-zen | `kilo-auto/small, g1/pro, g2/pro, g3/pro, g4/pro, g5/pro` |

The first row excludes the ranked tier and the filled `kilo-auto/free`. The
second and third show that the rule touches only Kilo's utility profile.

### Tests (`llmprovider/discovery_ranking_test.go`)

**`TestListModelCatalog_KiloRanksByProfile`** pins now to
`2026-09-26T00:00:00Z`. It serves a Kilo listing from `httptest`, built by a
helper `kiloRankEntry(id, name, prompt, completion string, ageDays int, reasoning bool, extra string) string`.
Every entry has:
* text in and text out;
* `mayTrainOnYourPrompts` false and context 262144;
* `supported_parameters` `["tools","reasoning"]`, or `["tools"]` when not
  reasoning;
* `created` = `now.AddDate(0, 0, -ageDays).Unix()`, or 0 when `ageDays < 0`.

| id | name | prompt | completion | age | extra |
|---|---|---|---|---|---|
| `a/flash-lite` | A Flash Lite | 0.0000001 | 0.0000004 | 20 | — |
| `b/flash` | B Flash | 0.0000003 | 0.0000012 | 10 | `"terminalBench":{"overallScore":0.75}` |
| `c/pro` | C Pro | 0.000005 | 0.000025 | 40 | `"terminalBench":{"overallScore":0.80}` |
| `d/mid` | D Mid | 0.000001 | 0.000004 | 60 | `"preferredIndex":2` |
| `e/mini` | E Mini | 0.0000002 | 0.0000008 | 90 | — |
| `f/large` | F Large | 0.000002 | 0.000008 | 30 | — |
| `kilo-auto/efficient` | Auto Efficient | -1 | -1 | −1 (created 0) | `"preferredIndex":0` |
| `o/llama-8b-instruct` | O Llama 8B | 0.00000002 | 0.00000004 | 100 | not reasoning |
| `g/flash-old` | G Flash Old | 0.0000001 | 0.0000001 | 600 | — |

**Assertions:**
* **Utility** (no option, then `WithModelProfile(ProfileUtility)`) must equal
  `b/flash, d/mid, c/pro, a/flash-lite, e/mini, f/large`. `kilo-auto/efficient`
  is excluded by MADR §3 item 9.
* **Capable** must equal
  `c/pro, b/flash, kilo-auto/efficient, d/mid, f/large, a/flash-lite`. This
  also proves the epoch rule end to end: the tier's `created` is 0.
* **Usable** has length 9, `cat.Usable[0] == "o/llama-8b-instruct"` (today's
  cheapest-first order), and `Live` is true.

### Mutation proofs (`$SCRATCH/p2-mutations.json`, `pkg` `./llmprovider`)

| name | file | old → new | run | expect |
|---|---|---|---|---|
| `p2-cost-only` | `llmprovider/model_ranking.go` | `return rankCostWeight*n.cost(c)/n.maxCost + rankAgeWeight*n.age(c)/n.maxAge` → `return rankCostWeight * n.cost(c) / n.maxCost` | `^TestRankRecommended_BlendWeighsAge$` | fail |
| `p2-unknown-cost-zero` | same | `return n.maxCost` → `return 0` | `^TestRankRecommended_BlendWeighsAge$` | fail |
| `p2-drop-reasoning` | same | `case c.reasoningKnown && !c.reasoning,` → `case false,` | `^TestRankRecommended_Eligibility$` | fail |
| `p2-no-cap` | same | `if perGroup[c.group] >= maxPerRankGroup {` → `if false {` | `^TestRankRecommended_DiversityCap$` | fail |
| `p2-tilde` | same | `strings.TrimLeft(id, "~")` → `id` | `^(TestRankRecommended_DiversityCap\|TestRankGroup)$` | fail |
| `p2-family-ignored` | same | `if family != "" {\n\t\treturn family` → `if false {\n\t\treturn family` | `^TestRankGroup$` | fail |
| `p2-epoch-as-date` | same | `if e.Created > 0 {` → `if e.Created >= 0 {` | `^(TestKiloCandidate_Fields\|TestListModelCatalog_KiloRanksByProfile)$` | fail |
| `p2-capable-as-utility` | same | `if profile == ProfileCapable {` → `if false {` | `^TestRankRecommended_CapableOrder$` | fail |
| `p2-small-ignored` | same | `boolFirst(a.small, b.small),` → `0,` | `^TestRankRecommended_UtilityOrder$` | fail |
| `p2-go-gate-zen` | same | `return provider != ProviderOpencodeGo \|\| !slices.Contains(opencodeGoRegionGated, c.id)` → `return !slices.Contains(opencodeGoRegionGated, c.id)` | `^TestRankRecommended_Eligibility$` | fail |
| `p2-kilo-today-order` | `llmprovider/discovery.go` | `kiloCurate(entries, cfg.ModelProfile)` → `curateKilo` | `^TestListModelCatalog_KiloRanksByProfile$` | fail |
| `p2-kiloauto-allowed` | `llmprovider/model_ranking.go` | `return profile != ProfileCapable && provider == ProviderKilo && strings.HasPrefix(id, "kilo-auto/")` → `return false` | `^(TestRankRecommended_KiloAutoUtility\|TestListModelCatalog_KiloRanksByProfile)$` | fail |
| `p2-kiloauto-fill` | same | `if !slices.Contains(out, id) && !utilityExcluded(profile, provider, id) {` → `if !slices.Contains(out, id) {` | `^TestRankRecommended_KiloAutoUtility$` | fail |
| `p2-kiloauto-all-profiles` | same | `return profile != ProfileCapable && provider == ProviderKilo && strings.HasPrefix(id, "kilo-auto/")` → `return provider == ProviderKilo && strings.HasPrefix(id, "kilo-auto/")` | `^TestRankRecommended_KiloAutoUtility$` | fail |
| `p2-control` | `llmprovider/model_ranking.go` | `// rankingNow is the ranking clock. Tests pin it.` → the same + ` (control)` | `^TestRankRecommended_UtilityOrder$` | pass |

`p2-cost-only`'s anchor is the gofmt form of the `blend` line. If the runner
reports 0 occurrences, take the formatted text, re-run, and record it.

## Phase 3 — Metadata client, Zen/Go/HF ranking, isolation, golden snapshot

### Steps

1. **Verify the snapshot source.** `shasum -a 256` of
   `$SCRATCH/snapshot-trim/{go,hf,kilo,metadata,zen}.json` must equal:

   | file | sha256 |
   |---|---|
   | `go.json` | `2fa9cdc1a10c5a2b04568b6bff7d586e57250279c5637cefaf436f7a1dd4a8f1` |
   | `hf.json` | `b9e2c3cc3c516c8cdae9da2fbbca007e92554b0cc5ba33487ba3c4f19bc33056` |
   | `kilo.json` | `d618185555e212bcae355415a8f0b89973e968315fa9d10ac7246e6e616ff828` |
   | `metadata.json` | `a8eb5c3dfa53ffb9c0fb8f4b265f763107ad2bf7b8d7ab2f358d6cc4b860d03d` |
   | `zen.json` | `6ba02fd5e733e766a11c589a976e24895651f7af4298d807bc5015e3d0e2f821` |

   If a file is missing or differs, **stop and prompt**. The 2026-09-26
   catalogs cannot be re-fetched, and a fresh snapshot would not match MADR §7.
   Then copy the five files to `llmprovider/testdata/ranking-2026-09-26/`.
2. **Isolation first.** Create `llmprovider/main_test.go` and
   `wizard/main_test.go`. Each is a `TestMain` that sets
   `MCPLIB_DISABLE_MODELS_METADATA=1` with `os.Setenv`, panics on error, and
   exits with `m.Run()`. The llmprovider file uses `envDisableModelMetadata`;
   the wizard file uses the literal. Doc comment: "TestMain keeps unit tests
   off the network: the models metadata fetch (MADR 0010 §2) is off unless a
   test re-enables it with t.Setenv and a fixture URL."
3. **Red tests.** Create `llmprovider/model_metadata_test.go` and append to
   `llmprovider/discovery_ranking_test.go` the tests below. Run
   `go test -count=1 -run 'TestLoadModelMetadata|TestModelMetadata|TestMetadataCandidate|TestListModelCatalog_(Metadata|OpencodeGoGates|HuggingFaceRanks|Snapshot)' ./llmprovider > "$SCRATCH/p3-red.log" 2>&1`.
   It must fail to compile. Copy the line to §11.
4. Implement §1.5 and §1.6.
5. Re-run step 3's command, which must pass. Then
   `go test -count=1 ./llmprovider ./wizard > "$SCRATCH/p3-pkg.log" 2>&1` must
   pass with every pre-existing test unmodified. Then
   `go vet -tags live_gateways ./llmprovider` must exit 0.
6. **Live metadata run (recorded).**
   `go test -count=1 -tags live_gateways -run '^TestLive_ModelMetadataDocument$' -v ./llmprovider > "$SCRATCH/p3-live.log" 2>&1`.
   * **PASS:** proceed.
   * **SKIP:** unreachable; record why, then proceed.
   * **FAIL:** a deviation (§10); stop.
7. Mutation proofs, then the phase gate on
   `llmprovider/model_metadata.go llmprovider/model_metadata_test.go llmprovider/main_test.go wizard/main_test.go llmprovider/discovery.go llmprovider/options.go llmprovider/discovery_ranking_test.go llmprovider/live_gateways_test.go`.
8. Stage those eight files and the five test-data files, then
   `git commit --no-edit`.

### Tests (`llmprovider/model_metadata_test.go`)

**Helpers:**
* `resetModelMetadataCache()` clears `modelMetadataCache` under
  `modelMetadataMu`.
* `enableModelMetadata(t)` calls
  `t.Setenv(envDisableModelMetadata, "0")`, then `resetModelMetadataCache()`,
  and `t.Cleanup(resetModelMetadataCache)`.
* `metadataServer(t, status int, body string) (*httptest.Server, *atomic.Int32)`
  counts requests.

**Tests:**
* `TestModelMetadataURL_Precedence`:
  * the option wins over `t.Setenv(envModelMetadataURL, "http://env")`;
  * the environment wins over the default;
  * with neither, the URL is the literal `"https://models.opencode.ai/api.json"`.
* `TestLoadModelMetadata_Disabled`:
  * `t.Setenv(envDisableModelMetadata, "1")` gives `errModelMetadataDisabled`
    and 0 requests;
  * `"true"` behaves the same.
* `TestLoadModelMetadata_CachesSuccess`: two loads of one URL give 1 request
  and equal documents.
* `TestLoadModelMetadata_FailureNotCached`:
  * the server answers 500 first, then 200;
  * the first load errors, and the second succeeds;
  * 2 requests.
* `TestLoadModelMetadata_Decode`:
  * a document with keys `opencode`, `opencode-go`, `huggingface` and `other`
    yields exactly the three MADR keys;
  * a document with `huggingface` but no `models` yields no `huggingface` key;
  * invalid JSON returns an error containing `model metadata: decode`.
* `TestMetadataCandidate_Fields`, with now pinned:
  * a covered entry with release `2026-07` has `ageDays` 87 and `ageKnown`;
  * `reasoning` absent means `reasoningKnown` false;
  * `cost` absent means `costKnown` false;
  * `status` `deprecated` is copied;
  * family `glm-flash` makes group `glm-flash` and small true;
  * an uncovered `org/flash` has only `id`, group `org` and small true.

### Tests (appended to `llmprovider/discovery_ranking_test.go`)

**`TestListModelCatalog_HuggingFaceRanksWithMetadata`** pins now and enables
metadata. The listing `hfRankListing` has five live, text-only, tool-capable
models with throughputs:

| id | throughput |
|---|---|
| `v/no-reason` | 400 |
| `a/large` | 300 |
| `b/mid` | 200 |
| `c/flash` | 100 |
| `d/uncovered` | 50 |

The metadata `huggingface` section, each with `limit.context` 131072 and
`name` equal to its family:

| id | reasoning | cost in/out | release | family |
|---|---|---|---|---|
| `v/no-reason` | false | 1/1 | 2026-09-01 | noreason |
| `a/large` | true | 1/4 | 2026-09-01 | large |
| `b/mid` | true | 0.5/1.5 | 2026-08-01 | mid |
| `c/flash` | true | 0.1/0.4 | 2026-09-10 | flash |

`d/uncovered` is absent from the metadata.

* **Utility** is `c/flash, b/mid, a/large, d/uncovered, v/no-reason`. The
  uncovered id is ranked, ahead of the fill.
* **Capable** is `a/large, b/mid, d/uncovered, c/flash, v/no-reason`.

**`TestListModelCatalog_MetadataFallback`** uses the same listing. For each
row, `cat.Recommended` must equal `curateHuggingFace(cat.Usable)`. That is the
literal `v/no-reason, a/large, b/mid, c/flash, d/uncovered`, also asserted.
Rows:

| Row | Setup |
|---|---|
| 500 | the metadata server answers 500 |
| bad JSON | it answers `{` |
| missing key | it answers `{"opencode":{"models":{}}}` |
| disabled | TestMain's default (not re-enabled) |
| timeout | the handler blocks on `<-r.Context().Done()`; the call uses a 300ms parent context |

**`TestListModelCatalog_MetadataFallbackZen`:**
* It uses the existing `opencodeListingFixture` and the 500 row.
* `cat.Recommended` must equal
  `curateFromCatalog(staticOpencodeCatalog(ProviderOpencodeZen), cat.Usable, isUsableOpencodeModel, RankOpencodeModel)`.

**`TestListModelCatalog_MetadataIsolation`** uses a counting metadata server
and a Zen listing:
* not re-enabled: 0 requests;
* after `enableModelMetadata(t)`: 1 request.

**`TestListModelCatalog_OpencodeGoGates`** pins now and enables metadata. The
listing ids are `deepseek-v4-flash, muse-spark-1.3-contributor, glm-5.3-flash,
qwen3.8-flash, kimi-k2.6, mimo-v2.6-flash, gpt-6-luna, hy3`. The same entries
go under both `opencode-go` and `opencode`, each with reasoning true and
`limit.context` 131072:

| id | cost in/out | release | family |
|---|---|---|---|
| `deepseek-v4-flash` | 0.05/0.1 | 2026-09-01 | deepseek-flash |
| `muse-spark-1.3-contributor` | 0.06/0.1 | 2026-09-01 | muse |
| `glm-5.3-flash` | 0.15/0.5 | 2026-08-26 | glm-flash |
| `qwen3.8-flash` | 0.2/0.8 | 2026-08-20 | qwen-flash |
| `kimi-k2.6` | 0.6/2.5 | 2026-07-01 | kimi |
| `mimo-v2.6-flash` | 0.14/0.28 | 2026-09-22 | mimo |
| `gpt-6-luna` | 0.1/0.5 | 2026-09-22 | gpt-luna |
| `hy3` | 0.3/1.2 | 2026-08-01 | hy |

* **Go utility** is `mimo-v2.6-flash, glm-5.3-flash, qwen3.8-flash,
  gpt-6-luna, hy3, kimi-k2.6`. `cat.Usable` contains both
  `deepseek-v4-flash` and `muse-spark-1.3-contributor`.
* **Zen utility** (same listing) is `deepseek-v4-flash, mimo-v2.6-flash,
  glm-5.3-flash, qwen3.8-flash, gpt-6-luna, hy3`.

**`TestListModelCatalog_Snapshot20260926`** pins now to
`2026-09-26T00:00:00Z` and enables metadata. It serves
`testdata/ranking-2026-09-26/metadata.json` as the metadata URL, and each
listing file as that provider's base URL. For both profiles,
`cat.Recommended` must equal MADR §7 literally, and `len(cat.Usable)` must be:

| provider | listing | usable |
|---|---|---|
| kilo | `kilo.json` | 284 |
| opencode-zen | `zen.json` | 80 |
| opencode-go | `go.json` | 38 |
| huggingface | `hf.json` | 131 |

Its doc comment names MADR 0010 §7 and states that a rule change must update
§7 and this test together.

**`TestLive_ModelMetadataDocument`** (in `live_gateways_test.go`) enables
metadata against the default URL, and skips on a transport error. Then:
* all three keys must be present;
* `opencode` must contain `glm-5.3-flash` with reasoning true;
* on failure it reports `DRIFT:`.

### Mutation proofs (`$SCRATCH/p3-mutations.json`, `pkg` `./llmprovider`)

| name | file | old → new | run | tags | expect |
|---|---|---|---|---|---|
| `p3-ignore-disable` | `llmprovider/model_metadata.go` | `return err == nil && v` → ~~`return false`~~ `return err == nil && v && false` **(2026-09-26 deviation, §10)** | `^TestListModelCatalog_MetadataIsolation$` | — | fail |
| `p3-no-cache` | same | `if ok && time.Since(e.fetched) < modelMetadataTTL {` → ~~`if false {`~~ `if ok && false {` **(2026-09-26 deviation, §10)** | `^TestLoadModelMetadata_CachesSuccess$` | — | fail |
| `p3-cache-failure` | same | `doc, err := fetchModelMetadata(ctx, url, cfg.HTTPClient)` → the same line + `\n\tmodelMetadataMu.Lock()\n\tmodelMetadataCache[url] = modelMetadataCacheEntry{doc: doc, fetched: time.Now()}\n\tmodelMetadataMu.Unlock()` | `^TestLoadModelMetadata_FailureNotCached$` | — | fail |
| `p3-uncovered-dropped` | same | `cands = append(cands, metadataCandidate(id, m, covered, now))` → `if covered {\n\t\t\t\tcands = append(cands, metadataCandidate(id, m, covered, now))\n\t\t\t}` | `^TestListModelCatalog_HuggingFaceRanksWithMetadata$` | — | fail |
| `p3-missing-key-ranks` | same | `if res.err != nil \|\| !ok {` → ~~`if res.err != nil {`~~ `if res.err != nil \|\| (!ok && false) {` **(2026-09-26 deviation, §10)** | `^TestListModelCatalog_MetadataFallback$` | — | fail |
| `p3-hf-today-order` | `llmprovider/discovery.go` | `metadataCurate(ProviderHuggingFace, cfg.ModelProfile, meta, curateHuggingFace)` → ~~`curateHuggingFace`~~ `func(u []string) []string { _ = meta; return curateHuggingFace(u) }` **(2026-09-26 deviation, §10)** | `^TestListModelCatalog_HuggingFaceRanksWithMetadata$` | — | fail |
| `p3-go-gate-off` | `llmprovider/model_ranking.go` | `return provider != ProviderOpencodeGo \|\| !slices.Contains(opencodeGoRegionGated, c.id)` → `return true` | `^TestListModelCatalog_OpencodeGoGates$` | — | fail |
| `p3-snapshot-cap` | same | `if perGroup[c.group] >= maxPerRankGroup {` → `if perGroup[c.group] >= 1 {` | `^TestListModelCatalog_Snapshot20260926$` | — | fail |
| `p3-live-key` | `llmprovider/model_metadata.go` | `` Go  *modelMetadataSection `json:"opencode-go"` `` → `` Go  *modelMetadataSection `json:"opencode-goo"` `` | `^TestLive_ModelMetadataDocument$` | `live_gateways` | fail |
| `p3-control` | same | `// cached in-process.` → `// cached in-process (control).` | `^TestListModelCatalog_Snapshot20260926$` | — | pass |

**Notes:**
* `p3-live-key`'s anchor depends on gofmt's alignment of the anonymous struct
  in `decodeModelMetadata`. Take it from the formatted file. If the runner
  reports `anchor occurs 0 times`, correct the anchor to the formatted text
  and record that. It is not a retry of a failing proof.
* `p3-live-key` needs network access. If it reports SKIP, record that it
  could not be proven, and why.

## Phase 4 — Static catalogs and `Options.Profile`

### Steps

1. **Red tests first.**
   * Add `TestStaticOpenCatalogs_UtilityCriteria` to
     `llmprovider/model_ranking_test.go`, and
     `TestConfigureLLM_ProfileReachesListing` to `wizard/model_select_test.go`.
   * Make the fixture edits below.
   * Run
     `go test -count=1 -run 'TestStaticOpenCatalogs_UtilityCriteria' ./llmprovider > "$SCRATCH/p4-red-llm.log" 2>&1`.
     It must fail on the old lists: the 8B entries, `kilo-auto/free`, `:free`,
     and Go's `deepseek-v4-flash`.
   * Run
     `go test -count=1 ./wizard > "$SCRATCH/p4-red-wiz.log" 2>&1`, which must
     fail to compile (`unknown field Profile`).
   * Copy both to §11.
2. Implement §1.7 and §1.8.
3. `go test -count=1 ./llmprovider ./wizard > "$SCRATCH/p4-pkg.log" 2>&1`
   must pass.
4. Mutation proofs, then the phase gate on
   `llmprovider/models_catalog.go llmprovider/model_ranking_test.go wizard/configure.go wizard/model_select_test.go`.
5. Stage those four files, then `git commit --no-edit`.

### Tests and fixture edits

**`TestStaticOpenCatalogs_UtilityCriteria`.** For each of the four lists:
* **Size:** it has exactly 6 entries.
* **No free models:** no entry contains `:free` or `-free`, or equals
  `kilo-auto/free`.
* **No small dense models:** no entry matches
  `(?i)(^|[^a-z0-9])(\d+(?:\.\d+)?)b([^a-z0-9]|$)` with a size ≤ 14, unless it
  also matches the MoE form `(?i)-a\d+(?:\.\d+)?b\b`.
* **Gates:** no entry contains `-contributor`, and `StaticOpencodeGo` has none
  of the four region-gated ids, written as literals in the test. **Amended by
  the 2026-09-26 goconst deviation (§10):** `deepseek-v4-flash` is written as
  `opencodeDeepSeekV4Flash`; the other three stay literals.

The dense-size rule is the reference simulation's `dense_small`.

**`TestConfigureLLM_ProfileReachesListing`** uses a Kilo `httptest` listing
with two models, using the real clock:
* `a/flash`: terminalBench 0.5, prices `0.0000001`/`0.0000004`, created ten
  days ago;
* `b/pro`: terminalBench 0.9, prices `0.000005`/`0.000025`, created twenty days
  ago.

Both are reasoning, tools, and text-only. The fake prompter's script is:
* `selects`: `{providerIdx(Kilo), 0}`;
* `inputs`: `{srv.URL, ""}` (a blank search shows the recommended menu);
* `secrets`: `{testKey}`.

With the zero `Options.Profile`, `res.Model == "a/flash"`. With
`Profile: llmprovider.ProfileCapable`, `res.Model == "b/pro"`.

**Edits in `wizard/model_select_test.go`.** Each keeps the test's intent now
that the Zen static six has changed:
* **`zenSearchIDs` (`:16-21`)** becomes `"deepseek-v4.1-flash",
  "qwen3.8-flash", "glm-5.3-flash", "deepseek-v4-flash",
  "gemini-3.5-flash-lite", "gemini-3.8-flash", "claude-sonnet-5",
  "claude-opus-5"`. Its comment still says "the six StaticOpencodeZen ids".
* **`TestConfigureLLM_FallbackSearch`:**
  * the primary query `"haiku"` becomes `"qwen"`;
  * the wanted `res.Model` becomes `"qwen3.8-flash"`.
* **`TestConfigureLLM_FallbackSearchLoops`:**
  * `"haiku"` becomes `"qwen"`;
  * the wanted `Fallbacks` become `{"claude-opus-5", "deepseek-v4.1-flash"}`;
  * the excluded labels become `ModelLabel(Zen, "qwen3.8-flash")` and
    `ModelLabel(Zen, "claude-opus-5")`;
  * the second menu stays at 5 rows.
* **Why the matcher results hold:** measured on this fixture,
  `SearchModels` returns `"qwen"` → `[qwen3.8-flash]`,
  `"claude"` → `[claude-opus-5, claude-sonnet-5]` and
  `"sonnet"` → `[claude-sonnet-5]` (Appendix 1, E10).
* **Unchanged:** `TestConfigureLLM_SearchUsesLiveCorpus` and every other
  wizard test.

### Mutation proofs (`$SCRATCH/p4-mutations.json`)

| name | file | old → new | pkg | run | expect |
|---|---|---|---|---|---|
| `p4-profile-dropped` | `wizard/configure.go` | `llmprovider.WithModelProfile(o.Profile)` → `llmprovider.WithModelProfile(llmprovider.ProfileUtility)` | `./wizard` | `^TestConfigureLLM_ProfileReachesListing$` | fail |
| `p4-static-8b` | `llmprovider/models_catalog.go` | `"deepseek/deepseek-v4.1-flash",` → `"meta-llama/llama-3.1-8b-instruct",` | `./llmprovider` | `^TestStaticOpenCatalogs_UtilityCriteria$` | fail |
| `p4-static-gated-go` | same | `"hy3",` → `"deepseek-v4-flash",` | `./llmprovider` | `^TestStaticOpenCatalogs_UtilityCriteria$` | fail |
| `p4-control` | `wizard/configure.go` | `// messages; llmprovider.ProfileCapable suits reasoning-heavy tiers.` → the same + ` (control)` | `./wizard` | `^TestConfigureLLM_ProfileReachesListing$` | pass |

If `"hy3",` is ambiguous in `models_catalog.go`, the runner stops. Use the
formatted line with its route comment, and record that.

## Phase 5 — Kilo reasoning shapes (live gate)

### Steps

1. **Red tests first.** Rewrite `TestKilo_SupportedParameterGating`
   (`kilo_test.go:50-93`), add `TestKilo_ReasoningEffortConfigured`, and add
   `TestLive_KiloReasoningShapes`. Run
   `go test -count=1 -run 'TestKilo_' ./llmprovider > "$SCRATCH/p5-red.log" 2>&1`.
   It must fail: the nil-caps row still gets `reasoning_effort`, and there is
   no `reasoning` object. Copy the `--- FAIL` lines to §11.
2. Implement §1.9.
3. Re-run step 1's command, which must pass. Then
   `go test -count=1 ./llmprovider > "$SCRATCH/p5-pkg.log" 2>&1` and
   `go vet -tags live_gateways ./llmprovider`.
4. **Live gate (MADR §6).**
   `go test -count=1 -tags live_gateways -run '^TestLive_KiloReasoningShapes$' -v ./llmprovider > "$SCRATCH/p5-live.log" 2>&1`.
   ~~It uses `kilo-auto/free` and needs no key.~~ **Amended by the 2026-09-26
   deviation (§10):** it uses `deepseek/deepseek-v4.1-flash` with
   `KILO_API_KEY`, making two short paid calls, and skips only when the key is
   unset.
   * **PASS** on both subtests: proceed.
   * **SKIP** (rate-limited or unavailable): retry once after 60 s. A second
     SKIP is a stop, because the gate has not been met.
   * **FAIL:** stop. MADR §6 requires the change to stop and the MADR to be
     amended.
5. Mutation proofs, then the phase gate on
   `llmprovider/kilo.go llmprovider/chatcompletions.go llmprovider/constants.go llmprovider/options.go llmprovider/kilo_test.go llmprovider/live_gateways_test.go`.
6. Stage those six files, then `git commit --no-edit`.

### Tests

**`TestKilo_SupportedParameterGating`** replaces its `wantEffort bool` column
with `wantEffort string` (`""` means absent) and
`wantReasoning map[string]any` (nil means absent). It keeps the call
`GenerateItemsWithToolThinking` on `kilo-auto/free`, with no effort
configured.

| row | caps | tools | tool_choice | reasoning_effort | reasoning |
|---|---|---|---|---|---|
| nil caps sends the reasoning object | nil | yes | yes | — | `{"enabled":true}` |
| tools only | `tools` | yes | no | — | — |
| tools and tool_choice | `tools, tool_choice` | yes | yes | — | — |
| reasoning_effort without reasoning | `tools, tool_choice, reasoning_effort` | yes | yes | `medium` | — |
| reasoning | `tools, reasoning` | yes | no | — | `{"enabled":true}` |
| reasoning and reasoning_effort | `tools, reasoning, reasoning_effort` | yes | no | — | `{"enabled":true}` |

**`TestKilo_ReasoningEffortConfigured`** calls
`GenerateItemsThinking` with `WithReasoningEffort("low")`:

| caps | expected |
|---|---|
| nil | `reasoning` `{"effort":"low"}`, no `reasoning_effort` |
| `tools, reasoning` | the same |
| `tools, reasoning_effort` | `reasoning_effort` `"low"`, no `reasoning` |
| `tools` | neither |

A final subtest calls plain `GenerateItems` with nil caps and the effort set,
and gets neither.

**`TestLive_KiloReasoningShapes`** has two subtests:
* `enabled`: no effort;
* `effort low`: `WithReasoningEffort("low")`.

Each builds `NewKilo("unused-free-model", "kilo-auto/free",
WithMaxTokens(400), …)` and calls `GenerateItemsThinking` with "Reply with
only the word ALPHA". Then:
* **Skip** only on `ErrRateLimited` or `ErrProviderUnavailable`.
* **Any other error is a failure:** `DRIFT (probed <wireShapesProbedOnKilo>):
  gateway rejected reasoning shape …`. The existing `skipIfTransient` is
  deliberately not used, because it skips `ErrInvalidRequest`, and a 400 is
  exactly what this gate must catch.
* **Output:** it must contain a `ReasoningItem`.

**Limit, recorded honestly.** `kilo-auto/free` reasons by default. This gate
proves the gateway **accepts** both shapes and still returns reasoning. It
cannot prove that `{effort:"low"}` changes the effort.

**Amended by the 2026-09-26 deviations (§10):**
* The gate builds its provider with `NewKilo(kiloKey(t),
  "deepseek/deepseek-v4.1-flash", WithMaxTokens(400), …)`.
* `kiloKey(t)` reads `KILO_API_KEY` and skips when it is unset.
* `TestLive_KiloChatCompletions` and `TestLive_KiloToolCall` use `kiloKey(t)`
  in place of `"unused-free-model"`, and the file header drops the claim that
  Kilo ignores a bogus bearer.
* The gateway accepts even an invalid effort with HTTP 200, so the
  `p5-live-shape` mutation is expected to be unprovable. Its result is
  recorded either way.

### Mutation proofs (`$SCRATCH/p5-mutations.json`, `pkg` `./llmprovider`)

| name | file | old → new | run | tags | expect |
|---|---|---|---|---|---|
| `p5-effort-as-param` | `llmprovider/kilo.go` | `return "", map[string]any{jsonKeyEffort: p.reasoningEffort}` → `return p.reasoningEffort, nil` | `^TestKilo_ReasoningEffortConfigured$` | — | fail |
| `p5-enabled-as-param` | same | `return "", map[string]any{jsonKeyEnabled: true}` → `return effortMedium, nil` | `^TestKilo_SupportedParameterGating$` | — | fail |
| `p5-prefer-effort-param` | same | `case p.supports(jsonKeyReasoning):` → `case p.supports(jsonKeyReasoning) && !p.supports(jsonKeyReasoningEffort):` | `^TestKilo_SupportedParameterGating$` | — | fail |
| `p5-non-thinking` | same | `case !thinking:` → `case false:` | `^TestKilo_ReasoningEffortConfigured$` | — | fail |
| `p5-body-dropped` | `llmprovider/chatcompletions.go` | `body[jsonKeyReasoning] = o.Reasoning` → `body["x-reasoning"] = o.Reasoning` | `^TestKilo_SupportedParameterGating$` | — | fail |
| `p5-live-shape` | `llmprovider/kilo.go` | `return "", map[string]any{jsonKeyEnabled: true}` → `return "", map[string]any{jsonKeyEnabled: "yes"}` | `^TestLive_KiloReasoningShapes$` | `live_gateways` | fail |
| `p5-control` | same | `// thinkingFields returns the reasoning fields for one call (MADR 0010 §6).` → the same + ` (control)` | `^TestKilo_SupportedParameterGating$` | — | pass |

`p5-live-shape` depends on the gateway validating the field. If it reports
PASS or SKIP, record that the live gate could not be shown to fail. The
unit-level mutations still prove the request body.

## Phase 6 — OpenCode chat `reasoning_effort` (live gate)

### Steps

1. **Red tests first.** Add `TestOpencode_ChatReasoningEffort` and
   `TestLive_OpencodeChatReasoningEffort`. Run
   `go test -count=1 -run 'TestOpencode_ChatReasoningEffort|TestOpencode_Thinking_PerRoute' ./llmprovider > "$SCRATCH/p6-red.log" 2>&1`.
   The "listed" row must fail, because no `reasoning_effort` is sent yet.
   Copy the `--- FAIL` line to §11.
2. Implement §1.10.
3. Re-run step 1's command, which must pass. Then
   `go test -count=1 ./llmprovider > "$SCRATCH/p6-pkg.log" 2>&1` and
   `go vet -tags live_gateways ./llmprovider`.
4. **Live gate (MADR §6).**
   `go test -count=1 -tags live_gateways -run '^TestLive_OpencodeChatReasoningEffort$' -v ./llmprovider > "$SCRATCH/p6-live.log" 2>&1`.
   * It needs `OPENCODE_API_KEY`, which was set in the planning session, and
     makes two short paid calls.
   * **PASS** on both models: proceed.
   * **SKIP:** stop and prompt. The gate is unmet, and this change must not
     ship unproven.
   * **FAIL:** stop, then amend the MADR.
5. Mutation proofs, then the phase gate on
   `llmprovider/opencode.go llmprovider/model_metadata.go llmprovider/chatcompletions.go llmprovider/opencode_test.go llmprovider/live_gateways_test.go`.
6. Stage those five files, then `git commit --no-edit`.

### Tests

**`TestOpencode_ChatReasoningEffort`** enables metadata, and serves an
`opencode` section containing only `deepseek-v4-pro` with
`reasoning_options: [{"type":"effort","values":["low","high","max"]}]`. Each
row calls `GenerateThinking`, except where stated, on the chat route with
`WithModelMetadataURL`:

| row | model | effort | expected body |
|---|---|---|---|
| listed | `deepseek-v4-pro` | `low` | `reasoning_effort: "low"` |
| unlisted value | `deepseek-v4-pro` | `medium` | no `reasoning_effort` |
| uncovered model | `kimi-k2.6` | `low` | no `reasoning_effort` |
| no effort | `deepseek-v4-pro` | — | no `reasoning_effort` |
| plain call | `deepseek-v4-pro` | `low`, `Generate` | no `reasoning_effort` |
| disabled | `deepseek-v4-pro` | `low`, `t.Setenv(envDisableModelMetadata, "1")` | no `reasoning_effort` |

In every row the body carries no `reasoning` and no `thinking` key.
`TestOpencode_Thinking_PerRoute`'s chat subtest stays unmodified: it
configures no effort.

**`TestLive_OpencodeChatReasoningEffort`** calls `opencodeKey(t)` and enables
metadata against the default URL. For `deepseek-v4.1-flash` and
`glm-5.3-flash` on Zen:
* **Metadata:** skip if the metadata is unreachable. It must list `low` for
  the model, or the test reports `DRIFT: … reasoning_options no longer list
  "low"`.
* **Route:** `NewOpencode(Zen, key, model, WithReasoningEffort("low"),
  WithMaxTokens(400)).Route()` must be `OpencodeRouteChatCompletions`.
* **Call:** `GenerateThinking("Reply with only the word ALPHA")`:
  * skip only on `ErrRateLimited`;
  * any other error is a failure (`DRIFT … gateway rejected reasoning_effort`);
  * the output must not be empty.

### Mutation proofs (`$SCRATCH/p6-mutations.json`, `pkg` `./llmprovider`)

| name | file | old → new | run | tags | expect |
|---|---|---|---|---|---|
| `p6-unlisted-sent` | `llmprovider/opencode.go` | `if err != nil \|\| !slices.Contains(doc.reasoningEfforts(p.gateway, p.model), p.reasoningEffort) {` → ~~`if err != nil {`~~ `if err != nil \|\| (!slices.Contains(doc.reasoningEfforts(p.gateway, p.model), p.reasoningEffort) && false) {` **(2026-09-26 deviation, §10)** | `^TestOpencode_ChatReasoningEffort$` | — | fail |
| `p6-non-thinking` | same | `if !thinking \|\| p.reasoningEffort == "" {` → `if p.reasoningEffort == "" {` | `^TestOpencode_ChatReasoningEffort$` | — | fail |
| `p6-chat-ignores` | same | `body = p.chatBody(input, tool, p.chatReasoningEffort(ctx, thinking))` → `body = p.chatBody(input, tool, "")` | `^TestOpencode_ChatReasoningEffort$` | — | fail |
| `p6-wrong-option-type` | `llmprovider/model_metadata.go` | `if o.Type == jsonKeyEffort {` → `if o.Type == "budget" {` | `^TestOpencode_ChatReasoningEffort$` | — | fail |
| `p6-live-value` | `llmprovider/opencode.go` | `return p.reasoningEffort` → `return "not-an-effort"` | `^TestLive_OpencodeChatReasoningEffort$` | `live_gateways` | fail |
| `p6-control` | same | `// unavailable, disabled or silent on the model sends nothing.` → the same + ` (control)` | `^TestOpencode_ChatReasoningEffort$` | — | pass |

`p6-live-value` spends two more paid calls. If the gateway accepts an invalid
effort, the result is PASS; record that the live gate could not be shown to
fail.

## Phase 7 — README and close-out

1. In `README.md`, after the paragraph ending "…before each model and fallback
   selection." (`README.md:128-136`), insert:

   > Recommended models for Kilo, OpenCode Zen and Go, and Hugging Face are
   > ranked by use case. The default profile, `llmprovider.ProfileUtility`,
   > suits short, frequent tasks such as commit messages. It recommends
   > recent, paid, reasoning-capable models, and never one its catalog marks
   > as non-reasoning, free, expiring or preview.
   > `WithModelProfile(llmprovider.ProfileCapable)` (or
   > `wizard.Options.Profile`) ranks the strongest first instead. Kilo's
   > ranking reads its own listing. The others read
   > `https://models.opencode.ai/api.json`, cached for ten minutes.
   > `WithModelMetadataURL` or `MCPLIB_MODELS_METADATA_URL` points elsewhere,
   > and `MCPLIB_DISABLE_MODELS_METADATA=1` turns the fetch off, which
   > restores the previous ordering for those three. Search still covers every
   > usable model. Kilo's `kilo-auto/*` managed tiers are never recommended
   > under the default profile: search for them (`kilo-auto/*`) to use one.

2. Replace "Retries are opt-in (`GenerateWithRetry`)." (`README.md:138`) with:
   "Retries are opt-in (`GenerateWithRetry`, and `GenerateThinkingWithRetry`
   for the reasoning path). For commit-message-sized work, construct the
   provider with `WithReasoningEffort(llmprovider.ProfileUtility.ReasoningEffort())`
   and call `GenerateThinkingWithRetry`."
3. Run `go test -count=1 ./... > "$SCRATCH/p7-full.log" 2>&1` and branch on
   its own exit status.
4. `git diff --stat 5a1fc70 -- go.mod go.sum llmprovider/probe.go wizard/prompter.go wizard/text_prompter.go llmprovider/opencode_route.go`
   must be empty (A14).
5. Fill in §11: each phase's red lines, mutation summaries, gate summaries and
   live results, verbatim. Set this plan to `status: complete` only if every
   §5 criterion holds.
6. Stage `README.md` and this plan, then `git commit --no-edit`. Do not push
   or tag. Report the number of commits ahead of `origin/main`.

## 4. Verification commands

After each phase's implementation, from the repository root:

```bash
python3 -B "$SCRATCH/mutate_and_test.py" --repo . --spec "$SCRATCH/pN-mutations.json" --workdir "$SCRATCH/pN-mut" > "$SCRATCH/pN-mut.log" 2>&1; MUT=$?
python3 -B "$SCRATCH/phase_gate.py" --repo . --logdir "$SCRATCH/pN-gate" --packages ./llmprovider ./wizard --files <phase Go files> > "$SCRATCH/pN-gate.log" 2>&1; GATE=$?
echo "mut=$MUT gate=$GATE"
```

* **Read both logs in full.** Commit only when `MUT` and `GATE` are both 0,
  except where a phase records a live mutation as unprovable (Phases 3, 5 and
  6).
* **A mutation reported `NOT OK`** means either the test does not catch its
  defect, or the anchor is wrong.
* **An anchor that is missing or ambiguous** makes the runner exit early with
  `anchor occurs N times`.
* **Either case is a stop**, not a retry.

## 5. Acceptance criteria

| # | Criterion | Evidence |
|---|---|---|
| A1 | Behaviour the MADR keeps is unchanged | every pre-existing test passes, except the edits named in Phase 2 (`TestListKiloModels_MetadataCuration`, 2026-09-26 deviation), Phase 4 (wizard Zen fixture) and Phase 5 (Kilo gating table); `TestListModelCatalog_RecommendedMatchesListAvailable` |
| A2 | §3 eligibility, including unknowns, boundaries and the epoch rule | `TestRankRecommended_Eligibility`, `TestKiloCandidate_Fields`; `p2-drop-reasoning`, `p2-epoch-as-date` |
| A3 | §4 ordering: signal, small, blend weights, capable order, diversity with `~`, fill | Phase 2 order tests; `p2-cost-only`, `p2-unknown-cost-zero`, `p2-small-ignored`, `p2-capable-as-utility`, `p2-no-cap`, `p2-tilde` |
| A4 | Kilo ranks from its own listing; the 8B and non-reasoning models leave the six but stay in `Usable` | `TestListModelCatalog_KiloRanksByProfile`; `p2-kilo-today-order` |
| A5 | Zen, Go and HF rank with metadata; uncovered ids ranked as unknown | `TestListModelCatalog_HuggingFaceRanksWithMetadata`; `p3-uncovered-dropped`, `p3-hf-today-order` |
| A6 | Every metadata failure degrades to today's curation | `TestListModelCatalog_MetadataFallback`, `_MetadataFallbackZen`; `p3-missing-key-ranks` |
| A7 | No unit test reaches the metadata URL; cache and disable behave | `TestMain` ×2, `TestListModelCatalog_MetadataIsolation`, the `TestLoadModelMetadata_*` tests; `p3-ignore-disable`, `p3-no-cache`, `p3-cache-failure` |
| A8 | Go gates: region and `-contributor` excluded from the six, searchable, Go-only | `TestListModelCatalog_OpencodeGoGates`; `p3-go-gate-off`, `p2-go-gate-zen` |
| A9 | The implementation reproduces MADR §7 on the 2026-09-26 snapshot | `TestListModelCatalog_Snapshot20260926`; `p3-snapshot-cap` |
| A10 | Static catalogs are §7's utility sixes and meet the criteria | `TestStaticOpenCatalogs_UtilityCriteria`; `p4-static-*`; existing count tests |
| A11 | `wizard.Options.Profile` reaches the listing | `TestConfigureLLM_ProfileReachesListing`; `p4-profile-dropped` |
| A12 | `GenerateThinkingWithRetry` has `GenerateWithRetry`'s behaviour; `ReasoningEffort()` values | Phase 1 tests; `p1-*` |
| A13 | Kilo's reasoning shapes are correct, and the live gate passes | Phase 5 tests and `p5-live.log` PASS; `p5-*` |
| A14 | OpenCode chat sends only listed efforts, and the live gate passes; `go.mod`, `go.sum`, probe, `Prompter`, `TextPrompter` and the route table untouched | Phase 6 tests and `p6-live.log` PASS; Phase 7 step 4 diff empty |
| A15 | Every gate green; full module green; README documents the feature | §11; `p7-full.log` exit 0 |
| A16 | Kilo's utility six never contains a `kilo-auto/*` id, ranked or filled; capable and other providers are unaffected (MADR §3 item 9) | `TestRankRecommended_KiloAutoUtility`, `TestListModelCatalog_KiloRanksByProfile`, the golden snapshot; `p2-kiloauto-*` |

## 6. Rollout and rollback

**Rollout.** The maintainer tags the next minor release after the one that
carries 0009. This plan does not tag or push. The release note must state:

1. **The recommended six change** for Kilo, OpenCode Zen and Go, and Hugging
   Face. prepare-commit-msg's `--yes` path and magicdev's listing install
   different defaults without any code change. No consumer test references
   the replaced static ids (Appendix 1, E11).
2. **Listing Zen, Go or Hugging Face now also GETs
   `https://models.opencode.ai/api.json`**, unless
   `MCPLIB_DISABLE_MODELS_METADATA=1` is set. Networks that block the host
   degrade to the previous ordering.
3. **Kilo's thinking path now sends `reasoning: {…}`** instead of
   `reasoning_effort`, except for models that list only `reasoning_effort`.
4. **OpenCode's chat route sends `reasoning_effort`** on a thinking call with
   an effort configured, when `api.json` lists that effort.
5. **The four static catalogs changed.**

Consumers adopt through their own bump. prepare-commit-msg's switch to
`GenerateThinkingWithRetry` and magictools' `ProfileCapable` are their own
changes.

**Rollback:**
* **Before any consumer adopts the new API,** revert the phase commits, newest
  first.
* **After a consumer adopts it,** reverting removes exported symbols they
  call. Instead:
  * set `MCPLIB_DISABLE_MODELS_METADATA=1` to return Zen, Go and HF to the
    previous ordering;
  * revert Phases 5–6 on their own, which touch no exported symbol, if a
    reasoning wire change misbehaves.
* **Kilo ranking has no runtime switch.** Reverting Phase 2 restores
  price-first for Kilo.

## 7. Risks

| Risk | Mitigation |
|---|---|
| A unit test silently fetches the real metadata | `TestMain` in both packages; `TestListModelCatalog_MetadataIsolation`; `p3-ignore-disable` |
| The Go ranker diverges from the MADR's evidence | the golden snapshot test pins §7; units and floors mirror the simulation (§1.11 item 2) |
| Cache keyed by URL leaks between tests (httptest ports are reused) | every metadata test uses `enableModelMetadata`, which resets the cache before and after |
| On a sparse catalog, the fill brings back ineligible ids | by design (MADR §4: never shorter than today); the fixtures with six or more eligible prove the gates themselves |
| Metadata GET adds latency to a listing | concurrent with the listing, cached for 10 minutes, bounded by the listing's 10 s context |
| Live gates cannot be shown to fail | recorded per phase; unit mutations prove the request bodies |
| Paid calls in Phase 6 | two calls plus two for the mutation, `max_tokens` 400 |
| Third-party catalog data committed as test data | trimmed to ids, prices, dates, capabilities and public benchmark scores from public, unauthenticated endpoints; models.dev is MIT |
| New files silently exempt from lint | names checked against the regex (§0.2); the gate runs `make lint` |

## 8. Out of scope (do not implement "while here")

* Everything under the MADR's *Out of scope*:
  * first-party provider ranking;
  * the evaluation harness;
  * consumer changes;
  * the id deny lists;
  * a disk cache or embedded snapshot.
* `GenerateItemsWithRetry` moving onto the shared loop (0012 §1.2).
* Typed `RegionError` and `DataPolicyError` (0012 §1.1), the 300 s timeouts
  (0012 §1.3), and Kilo `provider.data_collection` (0012 §3.3).
* The route table (`opencode_route.go`): all new static ids already route
  correctly (E7).
* `modelLabels` entries for the new static ids.
* A size cap on the metadata body. It was considered, then dropped: an
  untested guard is not added.

## 9. File summary

**New:**
* `llmprovider/model_profile.go`, `llmprovider/model_profile_test.go`,
  `llmprovider/retry_thinking_test.go`;
* `llmprovider/model_ranking.go`, `llmprovider/model_ranking_test.go`,
  `llmprovider/discovery_ranking_test.go`;
* `llmprovider/model_metadata.go`, `llmprovider/model_metadata_test.go`,
  `llmprovider/main_test.go`, `wizard/main_test.go`;
* `llmprovider/testdata/ranking-2026-09-26/{kilo,zen,go,hf,metadata}.json`.

**Modified:**
* `llmprovider/options.go`, `llmprovider/provider.go`,
  `llmprovider/discovery.go`, `llmprovider/models_catalog.go` (the four static
  lists only);
* `llmprovider/kilo.go`, `llmprovider/chatcompletions.go`,
  `llmprovider/constants.go` (one comment), `llmprovider/opencode.go`;
* `llmprovider/discovery_test.go` (`TestListKiloModels_MetadataCuration`
  only; 2026-09-26 deviation),
* `llmprovider/kilo_test.go` (the gating table and one new test),
  `llmprovider/opencode_test.go` (one new test),
  `llmprovider/live_gateways_test.go` (three new tests);
* `wizard/configure.go`, `wizard/model_select_test.go`;
* `README.md`, `docs/0003-MADR-add-gateway-llm-providers.md` (pointer only),
  `docs/0010-MADR-…` (status only), and this plan.

**Must stay untouched** (checked in Phase 7 step 4): `go.mod`, `go.sum`,
`llmprovider/probe.go`, `llmprovider/opencode_route.go`, `wizard/prompter.go`,
`wizard/text_prompter.go`.

## 10. Deviation log

| Date | Phase | Finding | Decision | Files added to phase |
|---|---|---|---|---|
| 2026-09-26 | 2 | `TestListKiloModels_MetadataCuration` (`discovery_test.go:474-509`, file untouched, passing at the baseline) failed: `got [org/cheap org/vlm org/dear], want [org/cheap org/vlm org/dear kilo-auto/variable]`. Nothing in its fixture is eligible, so the fill supplies the list, and MADR §3 item 9 (the maintainer's `kilo-auto` decision, added after E9 was verified) now drops `kilo-auto/variable` from the fill under the utility profile. Phase 2 step 3's "every pre-existing test unmodified" and E9 were not re-checked after that decision. | Maintainer chose "Assert both views". The test calls `ListModelCatalog`. `Usable` keeps the cheapest-first order with `"-1"` last: `org/cheap, org/vlm, org/dear, kilo-auto/variable`, preserving the price-trap coverage. `Recommended` is `org/cheap, org/vlm, org/dear`, with no `kilo-auto/*` id. No MADR amendment: MADR §4 already says the fill skips §3 item 9. | `llmprovider/discovery_test.go` |
| 2026-09-26 | 2 | The phase gate's `make lint` failed: `model_ranking.go:40:76: string deepseek-v4-flash has 4 occurrences, make it a constant (goconst)`. Not pre-existing: the baseline and Phase 1 gates reported `0 issues.` goconst counts test files but reports outside them. The four occurrences were §1.3's gate list, `StaticOpencodeGo` (`models_catalog.go:82`), and the two Go-gate eligibility rows. The route table's map keys are not counted. | Maintainer chose "Name the id". `model_ranking.go` declares `opencodeDeepSeekV4Flash = "deepseek-v4-flash"`. `opencodeGoRegionGated`, the eligibility rows, and later phases' tests use it wherever that id is needed as a Go string: Phase 3's Go-gate assertions and Phase 4's static-criteria gate list. No MADR amendment (implementation detail). | none (same files) |
| 2026-09-26 | 2 | Re-running the gate after that fix: `string deepseek-v4-pro has 3 occurrences, make it a constant (goconst)`, again at the gate list. The id's other occurrences are pre-existing test literals (`opencode_test.go`, `chatcompletions_test.go`, `opencode_route_test.go`) and route-table map keys. goconst counts test literals but reports only outside test files, so the gate list is the first reportable occurrence. | Maintainer chose "Name all four gated ids". `model_ranking.go` declares `opencodeDeepSeekV41Flash`, `opencodeDeepSeekFlash`, `opencodeDeepSeekV4Flash` and `opencodeDeepSeekV4Pro`, and builds `opencodeGoRegionGated` only from them. New tests that need a gated id use the constant; pre-existing tests keep their literals. No MADR amendment. | none (same files) |
| 2026-09-26 | 3 | Four Phase 3 mutations reported NOT OK because the planted copy does not compile. Each leaves a variable unused: `p3-ignore-disable` (`declared and not used: v`), `p3-no-cache` and `p3-missing-key-ranks` (`… ok`), and `p3-hf-today-order` (`… meta`). The runner correctly refuses a build failure as proof, so those behaviours were unproven. The plan's mutation rows were wrong as written. | Maintainer chose "Compiling equivalents". The four rows plant the same defect while keeping the variable used; see the annotated table. No code, test or MADR change. | none |
| 2026-09-26 | 6 (found in 3) | Checking the remaining rows found one more by inspection: `p6-unlisted-sent` (`if err != nil \|\| !slices.Contains(…) {` → `if err != nil {`) leaves `doc` and the `slices` import unused, so it cannot compile. | Maintainer chose "Compiling equivalent now". The row becomes `if err != nil \|\| (!slices.Contains(doc.reasoningEfforts(p.gateway, p.model), p.reasoningEffort) && false) {`, the same defect (an unlisted effort is sent). No code or MADR change. | none |
| 2026-09-26 | 5 | The live gate failed: `TestLive_KiloReasoningShapes` got `kilo HTTP 401` on both shapes. Probes without a credential (`kilo_shape_probe*.py`, 3 rounds) showed three things. `kilo-auto/free` now routes to `poolside/laguna-s-2.1:free`, which returns no reasoning for any request, even with no reasoning field (`reasoning_tokens=0` on every baseline round). The gateway answers both shapes, and an invalid effort, with HTTP 200 whenever the upstream is not rate-limiting (429). MADR §6's premise, "a reasoning-only model reachable without a key", no longer holds. | Maintainer chose "Real key, utility default". The gate runs against `deepseek/deepseek-v4.1-flash` with `KILO_API_KEY` and skips only when it is unset. Both shapes must return 200 and a `ReasoningItem`. **MADR §6 and *Confirmation* amended.** | none (same files) |
| 2026-09-26 | 5 | Pre-existing, reproduced on an unmodified `git archive 60dae10` export: `TestLive_KiloChatCompletions` and `TestLive_KiloToolCall` fail with `authentication failed: kilo HTTP 401`. Kilo now rejects the placeholder bearer (`"unused-free-model"`), contradicting the live file's header ("Kilo ignores a bogus bearer for free models (measured 200)"). `TestLive_KiloReasoningSpelling`, which sends no Authorization header, passes. | Maintainer chose "Fix now". A `kiloKey(t)` helper (`KILO_API_KEY`, skip if unset, like `opencodeKey`) serves those two tests and the new gate, and the file header is corrected. No MADR change beyond the §6 amendment above. | none (`live_gateways_test.go` is already in Phase 5) |
| 2026-09-26 | 6 | The live gate failed with `opencode-zen/chat_completions HTTP 402` on both models. With and without `reasoning_effort` (plain `Generate` too), Zen answers `{"message":"Upstream request failed: Insufficient account funds"}`: an account balance problem, not this change. OpenCode Go is no alternative. Every mcplib request there gets `HTTP 400 MissingSessionID: Request is missing x-opencode-session`, a pre-existing defect already recorded in `0011-REPORT-provider-source-compatibility-audit.md` and scheduled in `0012-MADR-conform-providers-to-reference-clients.md` §1. | Maintainer chose "Top up Zen, re-run". Phase 6 waits, uncommitted, until the maintainer confirms funds, then re-runs the gate on `deepseek-v4.1-flash` and `glm-5.3-flash`. The Go header stays with 0012. No MADR change. | none |
| 2026-09-26 | 6 | The maintainer directed "stage all, commit and push" while the Phase 6 live gate was still unmet (Zen funds). Offline evidence at that point: `TestOpencode_ChatReasoningEffort` green; mutations 5/5 caught (`p6-unlisted-sent`, `p6-non-thinking`, `p6-chat-ignores`, `p6-wrong-option-type`, control passes); phase gate 9/9. `p6-live-value` and the live gate have **not** run. A gocritic `typeDefFirst` finding was fixed by moving `reasoningEfforts` below `modelMetadataDoc` in the same file. | Phase 6 is committed and pushed **with its live gate outstanding**. The gate and `p6-live-value` must run once the Zen balance is restored, before Phase 7 closes the plan. A gate failure reopens Phase 6 (§10). The plan stays `in-progress`. | none |

## 11. Execution record

Not started.

## Appendix 1 — Evidence for this plan's assertions

All of this was gathered on 2026-09-26, read-only against `mcplib` at
`5a1fc70`. The scripts live in the planning session's scratchpad, never in the
repository.

* **E1 — Baseline:** the phase gate, `go test ./...`, the live-tag vet and
  the hooks path (§0.1).
* **E2 — Code facts:** each cited `file:line` was read at `5a1fc70`:
  * `discovery.go:93-102`, `:127-134`, `:475-484`, `:623-626`, `:628-641`,
    `:706-747`;
  * `options.go:24-45`;
  * `provider.go:126-222`;
  * `kilo.go:104`, `:122`, `:141`, `:151-165`;
  * `chatcompletions.go:12-21`, `:75-77`;
  * `opencode.go:222-231`, `:243`;
  * `constants.go:49-51`, `:57`, `:63-64`;
  * `models_catalog.go:64-113`;
  * `configure.go:38-65`, `:283`;
  * `model_select_test.go:16-21`, `:295-359`;
  * `kilo_test.go:50-93`;
  * `opencode_test.go:175-238`;
  * `live_gateways_test.go:39-49`, `:63`.
* **E3 — Snapshot replay:** the saved 2026-09-26 raw listings, served through
  `httptest` to the shipped `ListModelCatalog`, reproduce the simulation's
  input exactly. `Usable` and `Recommended` are equal for all four providers
  (284, 80, 38 and 131 usable).
* **E4 — Trimming is lossless:** the trimmed files give the same replay
  result, and the simulation's output over the trimmed Kilo and metadata
  files is byte-identical to its output over the raw files.
* **E5 — Snapshot file sizes and digests:** Phase 3 step 1.
* **E6 — Shapes:**
  * `api.json` `reasoning_options` is a list of `{type, values}`;
  * every section has `cost`, `limit`, `reasoning` and `release_date` on every
    model;
  * `release_date` is `YYYY-MM-DD`, or `YYYY-MM` for 3 Hugging Face models;
  * no model has `limit.context` 0, and no Kilo model has `context_length` 0
    or an empty price.
  So §1.3's "0 is unknown" and "empty price is 0" never disagree with the
  simulation on real data.
* **E7 — Routes:** for each of the twelve new Zen and Go static ids, the route
  from `NewOpencode(...).Route()` equals the route implied by its `api.json`
  npm package (0 mismatches).
* **E8 — Fixture expectations:** every expected list in Phases 2–3 was
  computed by the reference simulation, from fixtures mirroring the Go tests.
* **E9 — Existing Kilo fixture:** it has no `reasoning` parameter, so nothing
  in it is eligible, and the fill reproduces today's
  `org/cheap, org/vlm, org/dear, kilo-auto/variable`. **Superseded by the
  2026-09-26 deviation (§10):** that held before the maintainer's `kilo-auto`
  decision. Under MADR §3 item 9 the fill yields
  `org/cheap, org/vlm, org/dear`.
* **E10 — Wizard search:** the matcher results quoted in Phase 4, measured
  with `SearchModels` on the new fixture ids.
* **E11 — Consumers:** a search of prepare-commit-msg, mcp-server-magictools
  and mcp-server-magicdev finds no reference to the four static catalogs or
  their replaced ids.
* **E12 — Credentials:** `OPENCODE_API_KEY` is set in the planning
  environment. Its value was never read.

## Appendix 2 — `trim_snapshot.py` (provenance of the test data)

It reads the saved raw responses (`raw-kilo.json`, `raw-zen.json`,
`raw-go.json`, `raw-hf.json`, `raw-modelsopencode.json`, all from 2026-09-26)
and writes JSON with sorted keys and no whitespace. It keeps:

* **Kilo:** every entry's `id`, `name`, `created`, `context_length`,
  `expiration_date`, `preferredIndex`, `supported_parameters`,
  `mayTrainOnYourPrompts`, the `architecture` modalities,
  `pricing.{prompt,completion}` and `terminalBench.overallScore`.
* **Zen and Go:** `id`, `object: "model"` and `owned_by: "opencode"`.
* **Hugging Face:** `id`, `architecture`, and each provider's `status`,
  `supports_tools`, `throughput` and `first_token_latency_ms`.
* **Metadata:** only the `opencode`, `opencode-go` and `huggingface` sections.
  For each model it keeps `id`, `name`, `family`, `reasoning`,
  `reasoning_options`, `release_date`, `status`, `cost.{input,output}` and
  `limit.context`.
