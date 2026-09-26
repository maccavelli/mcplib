package llmprovider

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"
)

// refNow is the reference simulation's clock (MADR 0010 §7).
var refNow = time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)

// pinRankingNow fixes the ranking clock for one test.
func pinRankingNow(t *testing.T, now time.Time) {
	t.Helper()
	orig := rankingNow
	rankingNow = func() time.Time { return now }
	t.Cleanup(func() { rankingNow = orig })
}

// good returns an eligible, not-small candidate with known cost and age.
func good(id, group string, cost float64, ageDays int) rankCandidate {
	return rankCandidate{
		id: id, group: group,
		reasoning: true, reasoningKnown: true,
		cost: cost, costKnown: true,
		ageDays: ageDays, ageKnown: true,
		context: 128000,
	}
}

func small(c rankCandidate) rankCandidate {
	c.small = true
	return c
}

func withBench(c rankCandidate, bench float64) rankCandidate {
	c.bench, c.benchKnown = bench, true
	return c
}

func withPreferred(c rankCandidate, idx int) rankCandidate {
	c.preferred, c.preferredKnown = idx, true
	return c
}

func assertRanked(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("ranked = %v\n            want %v", got, want)
	}
}

// Expected lists in this file were computed by the reference simulation that
// produced MADR 0010 §7.

func TestRankRecommended_UtilityOrder(t *testing.T) {
	cands := []rankCandidate{
		withBench(good("s/bench", "s", 9, 300), 0.5),
		small(good("f/flash-a", "f", 1, 30)),
		small(good("g/flash-b", "g", 2, 1)),
		small(good("h/flash-c", "h", 2, 1)),
		good("p/pro", "p", 0.5, 30),
		good("q/max", "q", 4, 60),
	}
	assertRanked(t, rankRecommended(ProfileUtility, ProviderKilo, cands, nil),
		[]string{"s/bench", "f/flash-a", "g/flash-b", "h/flash-c", "p/pro", "q/max"})
}

// TestRankRecommended_BlendWeighsAge pins the 0.8/0.2 blend and "unknown
// counts as the maximum": cost-only or age-only ordering, or unknown-as-zero,
// all produce a different list.
func TestRankRecommended_BlendWeighsAge(t *testing.T) {
	unknownCost := small(good("d/flash-unk", "d", 0, 3))
	unknownCost.costKnown = false
	unknownAge := small(good("e/flash-noage", "e", 1, 0))
	unknownAge.ageKnown = false
	cands := []rankCandidate{
		small(good("a/flash-old", "a", 2, 540)),
		small(good("b/flash-new", "b", 2.2, 3)),
		small(good("c/flash-max", "c", 4, 30)),
		unknownCost,
		unknownAge,
	}
	assertRanked(t, rankRecommended(ProfileUtility, ProviderKilo, cands, nil),
		[]string{"e/flash-noage", "b/flash-new", "a/flash-old", "d/flash-unk", "c/flash-max"})
}

func TestRankRecommended_CapableOrder(t *testing.T) {
	cands := []rankCandidate{
		withBench(good("x/top", "x", 5, 60), 0.9),
		withBench(good("y/second", "y", 50, 30), 0.8),
		withPreferred(good("z/pref0", "z", 2, 30), 0),
		withPreferred(good("w/pref3", "w", 1, 30), 3),
		good("v/big", "v", 3, 10),
		good("t/old", "t", 10, 400),
		small(good("u/flash", "u", 1, 5)),
	}
	assertRanked(t, rankRecommended(ProfileCapable, ProviderKilo, cands, nil),
		[]string{"x/top", "y/second", "z/pref0", "w/pref3", "v/big", "t/old"})
	assertRanked(t, rankRecommended(ProfileUtility, ProviderKilo, cands, nil),
		[]string{"w/pref3", "z/pref0", "x/top", "y/second", "u/flash", "v/big"})
}

func TestRankRecommended_DiversityCap(t *testing.T) {
	var cands []rankCandidate
	for _, s := range []struct {
		id    string
		cost  float64
		small bool
	}{
		{"~a/flash-0", 0.9, true}, {"a/flash-1", 1, true}, {"a/flash-2", 1.1, true},
		{"a/flash-3", 1.2, true}, {"b/flash", 3, true}, {"c/flash", 4, true},
		{"d/pro", 1, false}, {"e/pro", 2, false},
	} {
		c := good(s.id, rankGroup(s.id, ""), s.cost, 10)
		c.small = s.small
		cands = append(cands, c)
	}
	assertRanked(t, rankRecommended(ProfileUtility, ProviderKilo, cands, nil),
		[]string{"~a/flash-0", "a/flash-1", "b/flash", "c/flash", "d/pro", "e/pro"})
}

func TestRankRecommended_Fill(t *testing.T) {
	cands := []rankCandidate{
		small(good("m/flash", "m", 1, 10)),
		good("n/pro", "n", 2, 10),
		good("o/old", "o", 1, 600),
	}
	fill := []string{"o/old", "m/flash", "p/x", "q/y", "r/z", "s/w"}
	assertRanked(t, rankRecommended(ProfileUtility, ProviderKilo, cands, fill),
		[]string{"m/flash", "n/pro", "o/old", "p/x", "q/y", "r/z"})
}

// TestRankRecommended_Eligibility pins MADR 0010 §3 one rule at a time. The
// tested candidate ranks first whenever it is eligible.
func TestRankRecommended_Eligibility(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		edit     func(*rankCandidate)
		want     bool
	}{
		{"baseline", ProviderKilo, func(*rankCandidate) {}, true},
		{"non-reasoning", ProviderKilo, func(c *rankCandidate) { c.reasoning = false }, false},
		{"reasoning unknown", ProviderKilo, func(c *rankCandidate) { c.reasoning, c.reasoningKnown = false, false }, true},
		{"free", ProviderKilo, func(c *rankCandidate) { c.cost = 0 }, false},
		{"cost unknown", ProviderKilo, func(c *rankCandidate) { c.costKnown = false }, true},
		{"age 541", ProviderKilo, func(c *rankCandidate) { c.ageDays = 541 }, false},
		{"age 540", ProviderKilo, func(c *rankCandidate) { c.ageDays = 540 }, true},
		{"age unknown", ProviderKilo, func(c *rankCandidate) { c.ageDays, c.ageKnown = 10000, false }, true},
		{"context 32767", ProviderKilo, func(c *rankCandidate) { c.context = 32767 }, false},
		{"context 32768", ProviderKilo, func(c *rankCandidate) { c.context = 32768 }, true},
		{"context unknown", ProviderKilo, func(c *rankCandidate) { c.context = 0 }, true},
		{"deprecated", ProviderKilo, func(c *rankCandidate) { c.status = "deprecated" }, false},
		{"alpha status", ProviderKilo, func(c *rankCandidate) { c.status = "alpha" }, false},
		{"beta", ProviderKilo, func(c *rankCandidate) { c.status = "beta" }, true},
		{"expiring 30", ProviderKilo, func(c *rankCandidate) { c.expiryDays, c.expiryKnown = 30, true }, false},
		{"expiring 31", ProviderKilo, func(c *rankCandidate) { c.expiryDays, c.expiryKnown = 31, true }, true},
		{"preview id", ProviderKilo, func(c *rankCandidate) { c.id = "t/flash-preview" }, false},
		{"Preview id", ProviderKilo, func(c *rankCandidate) { c.id = "t/flash-Preview" }, false},
		{"exp id", ProviderKilo, func(c *rankCandidate) { c.id = "t/flash-exp" }, false},
		{"expert id", ProviderKilo, func(c *rankCandidate) { c.id = "t/flash-expert" }, true},
		{"experimental id", ProviderKilo, func(c *rankCandidate) { c.id = "t/flash-experimental" }, false},
		{"alpha id", ProviderKilo, func(c *rankCandidate) { c.id = "t/flash-alpha" }, false},
		{"contributor", ProviderKilo, func(c *rankCandidate) { c.id = "t/flash-contributor" }, false},
		{"Go gate on Go", ProviderOpencodeGo, func(c *rankCandidate) { c.id = opencodeDeepSeekV4Flash }, false},
		{"Go gate on Zen", ProviderOpencodeZen, func(c *rankCandidate) { c.id = opencodeDeepSeekV4Flash }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tested := small(good("t/flash", "t", 1, 10))
			tc.edit(&tested)
			cands := []rankCandidate{tested}
			for _, id := range []string{"g1/pro", "g2/pro", "g3/pro", "g4/pro", "g5/pro", "g6/pro"} {
				cands = append(cands, good(id, id[:2], 5, 100))
			}
			got := rankRecommended(ProfileUtility, tc.provider, cands, nil)
			if slices.Contains(got, tested.id) != tc.want {
				t.Errorf("%s in %v = %v, want %v", tested.id, got, !tc.want, tc.want)
			}
		})
	}
}

// TestRankRecommended_KiloAutoUtility pins MADR 0010 §3 item 9: Kilo's
// kilo-auto/* tiers are never recommended under the utility profile, ranked
// or filled, and the rule touches nothing else.
func TestRankRecommended_KiloAutoUtility(t *testing.T) {
	tier := small(good("kilo-auto/small", "kilo-auto", 0.45, 0))
	tier.ageKnown = false
	tier.context = 262144
	cands := []rankCandidate{tier}
	for _, id := range []string{"g1/pro", "g2/pro", "g3/pro", "g4/pro", "g5/pro"} {
		cands = append(cands, good(id, id[:2], 5, 100))
	}
	fill := []string{"kilo-auto/free", "x/y"}
	tests := []struct {
		profile  ModelProfile
		provider string
		want     []string
	}{
		{ProfileUtility, ProviderKilo, []string{"g1/pro", "g2/pro", "g3/pro", "g4/pro", "g5/pro", "x/y"}},
		{ProfileCapable, ProviderKilo, []string{"g1/pro", "g2/pro", "g3/pro", "g4/pro", "g5/pro", "kilo-auto/small"}},
		{ProfileUtility, ProviderOpencodeZen, []string{"kilo-auto/small", "g1/pro", "g2/pro", "g3/pro", "g4/pro", "g5/pro"}},
	}
	for _, tc := range tests {
		assertRanked(t, rankRecommended(tc.profile, tc.provider, cands, fill), tc.want)
	}
}

func TestRankGroup(t *testing.T) {
	tests := []struct{ id, family, want string }{
		{"~a/x", "", "a"},
		{"v/x", "fam", "v"},
		{"x", "fam", "fam"},
		{"x", "", "x"},
	}
	for _, tc := range tests {
		if got := rankGroup(tc.id, tc.family); got != tc.want {
			t.Errorf("rankGroup(%q, %q) = %q, want %q", tc.id, tc.family, got, tc.want)
		}
	}
}

func decodeKiloEntry(t *testing.T, js string) kiloCatalogEntry {
	t.Helper()
	var e kiloCatalogEntry
	if err := json.Unmarshal([]byte(js), &e); err != nil {
		t.Fatalf("decode %s: %v", js, err)
	}
	return e
}

// TestKiloCandidate_Fields pins how a Kilo listing entry becomes a candidate,
// including MADR 0010 §3's epoch rule: created 0 is unknown, not 1970.
func TestKiloCandidate_Fields(t *testing.T) {
	auto := kiloCandidate(decodeKiloEntry(t, `{"id":"kilo-auto/efficient","name":"Auto Efficient","created":0,
		"context_length":1000000,"pricing":{"prompt":"-1","completion":"-1"},"preferredIndex":0,
		"supported_parameters":["tools","reasoning"]}`), refNow)
	if auto.ageKnown || auto.costKnown || auto.benchKnown || auto.small {
		t.Errorf("kilo-auto/efficient: ageKnown=%v costKnown=%v benchKnown=%v small=%v, want all false",
			auto.ageKnown, auto.costKnown, auto.benchKnown, auto.small)
	}
	if !auto.preferredKnown || auto.preferred != 0 || !auto.reasoning || !auto.reasoningKnown {
		t.Errorf("kilo-auto/efficient: preferred=%d/%v reasoning=%v/%v", auto.preferred, auto.preferredKnown,
			auto.reasoning, auto.reasoningKnown)
	}
	if auto.group != "kilo-auto" || auto.context != 1000000 {
		t.Errorf("kilo-auto/efficient: group=%q context=%d", auto.group, auto.context)
	}

	created := refNow.AddDate(0, 0, -10).Unix()
	flash := kiloCandidate(decodeKiloEntry(t, `{"id":"org/flash","name":"Org: Flash","created":`+
		jsonInt(created)+`,"context_length":4095,"expiration_date":"2026-10-20",
		"pricing":{"prompt":"0.0000003","completion":"0.0000012"},"terminalBench":{"overallScore":0.75},
		"supported_parameters":["tools"]}`), refNow)
	if !flash.ageKnown || flash.ageDays != 10 {
		t.Errorf("org/flash ageDays = %d/%v, want 10", flash.ageDays, flash.ageKnown)
	}
	if !flash.costKnown || math.Abs(flash.cost-1.5) > 1e-9 {
		t.Errorf("org/flash cost = %v/%v, want 1.5", flash.cost, flash.costKnown)
	}
	if !flash.expiryKnown || flash.expiryDays != 24 {
		t.Errorf("org/flash expiryDays = %d/%v, want 24", flash.expiryDays, flash.expiryKnown)
	}
	if !flash.benchKnown || flash.bench != 0.75 {
		t.Errorf("org/flash bench = %v/%v, want 0.75", flash.bench, flash.benchKnown)
	}
	if flash.reasoning || !flash.reasoningKnown || !flash.small || flash.group != "org" || flash.context != 4095 {
		t.Errorf("org/flash: reasoning=%v/%v small=%v group=%q context=%d", flash.reasoning,
			flash.reasoningKnown, flash.small, flash.group, flash.context)
	}

	for _, tc := range []struct {
		js        string
		cost      float64
		costKnown bool
	}{
		{`{"id":"org/free","pricing":{"prompt":"0","completion":"0"}}`, 0, true},
		{`{"id":"org/blank","pricing":{"prompt":"","completion":""}}`, 0, true},
		{`{"id":"org/bad","pricing":{"prompt":"abc","completion":"0.000001"}}`, 0, false},
	} {
		c := kiloCandidate(decodeKiloEntry(t, tc.js), refNow)
		if c.costKnown != tc.costKnown || (tc.costKnown && c.cost != tc.cost) {
			t.Errorf("%s: cost = %v/%v, want %v/%v", c.id, c.cost, c.costKnown, tc.cost, tc.costKnown)
		}
	}
	if g := kiloCandidate(decodeKiloEntry(t, `{"id":"~org/tilde"}`), refNow).group; g != "org" {
		t.Errorf("~org/tilde group = %q, want org", g)
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
