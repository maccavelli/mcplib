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
	opencodeGoRegionGated = []string{
		opencodeDeepSeekV41Flash, opencodeDeepSeekFlash, opencodeDeepSeekV4Flash, opencodeDeepSeekV4Pro,
	}
)

// The OpenCode Go region-gated ids are named because they recur across the
// gate, the static catalogs, the route table and the tests (goconst).
const (
	opencodeDeepSeekV41Flash = "deepseek-v4.1-flash"
	opencodeDeepSeekFlash    = "deepseek-flash"
	opencodeDeepSeekV4Flash  = "deepseek-v4-flash"
	opencodeDeepSeekV4Pro    = "deepseek-v4-pro"
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

// rankRecommended returns at most MaxListedModels distinct ids: the eligible
// candidates in profile order, at most maxPerRankGroup per group, then fill in
// order, skipping ids already chosen or excluded (MADR 0010 §4, MADR 0013 A1).
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
		if perGroup[c.group] >= maxPerRankGroup || slices.Contains(out, c.id) {
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

// kiloPrice parses one Kilo per-token price. A blank, negative ("-1", the
// variable-priced kilo-auto tiers), non-finite or unparseable price is unknown
// (MADR 0010 §3; MADR 0013 A2–A3).
func kiloPrice(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v, err == nil && v >= 0 && !math.IsInf(v, 1)
}
