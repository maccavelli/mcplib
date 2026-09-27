package llmprovider

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// countID reports how many times id appears in ids.
func countID(ids []string, id string) int {
	n := 0
	for _, v := range ids {
		if v == id {
			n++
		}
	}
	return n
}

// TestListModelCatalog_DuplicateIDs pins MADR 0013 A1: a listing that repeats
// an id yields it once in Usable and once in Recommended.
func TestListModelCatalog_DuplicateIDs(t *testing.T) {
	pinRankingNow(t, refNow)
	a := kiloRankEntry("a/pro", "A Pro", "0.000001", "0.000004", 20, true, "")
	b := kiloRankEntry("b/pro", "B Pro", "0.000002", "0.000004", 20, true, "")
	srv := serveBody(t, `{"data":[`+strings.Join([]string{a, a, b}, ",")+`]}`)
	cat := listCatalog(context.Background(), t, ProviderKilo, WithBaseURL(srv.URL))
	if n := countID(cat.Recommended, "a/pro"); n != 1 {
		t.Errorf("Recommended lists a/pro %d times: %v", n, cat.Recommended)
	}
	if n := countID(cat.Usable, "a/pro"); n != 1 {
		t.Errorf("Usable lists a/pro %d times: %v", n, cat.Usable)
	}
}

// TestRankRecommended_SkipsDuplicateCandidates pins MADR 0013 A1 at the
// ranker: two candidates with one id take one slot.
func TestRankRecommended_SkipsDuplicateCandidates(t *testing.T) {
	c := good("x/a", "x", 1, 10)
	got := rankRecommended(ProfileUtility, ProviderKilo, []rankCandidate{c, c, good("y/b", "y", 2, 10)}, nil)
	assertRanked(t, got, []string{"x/a", "y/b"})
}

// kiloPricedEntry is one reasoning Kilo entry created ten days before refNow;
// pricing is a JSON member (with its leading comma) or "".
func kiloPricedEntry(t *testing.T, id, pricing string) kiloCatalogEntry {
	t.Helper()
	return decodeKiloEntry(t, fmt.Sprintf(`{"id":%q,"name":%q,"created":%d,"context_length":200000,`+
		`"supported_parameters":["tools","reasoning"]%s}`, id, id, refNow.AddDate(0, 0, -10).Unix(), pricing))
}

// TestKiloCandidate_AbsentPriceIsUnknown pins MADR 0013 A2 (MADR 0010 §3: an
// absent field never excludes). An explicit "0" is still free and excluded.
func TestKiloCandidate_AbsentPriceIsUnknown(t *testing.T) {
	absent := kiloCandidate(kiloPricedEntry(t, "x/y", ""), refNow)
	if absent.costKnown || !absent.eligible(ProviderKilo) {
		t.Errorf("no pricing block: costKnown=%v eligible=%v, want unknown and eligible",
			absent.costKnown, absent.eligible(ProviderKilo))
	}
	free := kiloCandidate(kiloPricedEntry(t, "x/free", `,"pricing":{"prompt":"0","completion":"0"}`), refNow)
	if !free.costKnown || free.eligible(ProviderKilo) {
		t.Errorf(`pricing "0": costKnown=%v eligible=%v, want known and excluded`,
			free.costKnown, free.eligible(ProviderKilo))
	}
}

// TestKiloPrice_NonFiniteIsUnknown pins MADR 0013 A2–A3: blank and
// non-finite prices are unknown; finite non-negative prices parse.
func TestKiloPrice_NonFiniteIsUnknown(t *testing.T) {
	for _, s := range []string{"", "  ", "Infinity", "+Inf", "inf", "NaN", "-1", "abc"} {
		if v, ok := kiloPrice(s); ok {
			t.Errorf("kiloPrice(%q) = %v, known; want unknown", s, v)
		}
	}
	for s, want := range map[string]float64{"0": 0, "0.000001": 0.000001, " 2e-6 ": 2e-6} {
		if v, ok := kiloPrice(s); !ok || v != want {
			t.Errorf("kiloPrice(%q) = %v/%v, want %v/true", s, v, ok, want)
		}
	}
}

// TestRankRecommended_InfinitePriceNotCheapest pins MADR 0013 A3: before the
// fix an "Infinity" price made the blend NaN, which cmp.Compare orders first.
func TestRankRecommended_InfinitePriceNotCheapest(t *testing.T) {
	mk := func(id, price string) rankCandidate {
		return kiloCandidate(kiloPricedEntry(t, id,
			fmt.Sprintf(`,"pricing":{"prompt":%q,"completion":%q}`, price, price)), refNow)
	}
	got := rankRecommended(ProfileUtility, ProviderKilo,
		[]rankCandidate{mk("a/cheap", "0.0000001"), mk("b/mid", "0.000001"), mk("z/infinite", "Infinity")}, nil)
	assertRanked(t, got, []string{"a/cheap", "b/mid", "z/infinite"})
}
