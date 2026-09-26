package llmprovider

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Model search (MADR 0009 §3). A query containing * or ? takes the glob path;
// any other query is scored by a tiered fuzzy match. SearchModels ranks; it
// does not filter for usability, which is the listing's job.

// Fuzzy score tiers: a stronger kind of match always outranks a weaker one.
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

// ModelMatch is one ranked SearchModels result.
type ModelMatch struct {
	ID    string
	Label string // ModelLabel(provider, ID)
	Score int    // higher is better; comparable only within one call
}

// SearchModels ranks models against query. Glob queries (* matches any run,
// including "/", and ? one character) keep input order; other queries are
// sorted by score, then shorter id, then id. An empty query returns nil.
// Duplicate ids (compared case-insensitively) are searched once.
func SearchModels(provider string, models []string, query string) []ModelMatch {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	candidates := uniqueModelMatches(provider, models)
	if strings.ContainsAny(q, "*?") {
		return globMatches(candidates, q)
	}
	var out []ModelMatch
	for _, c := range candidates {
		if score, ok := fuzzyModelScore(c.ID, c.Label, q); ok {
			c.Score = score
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b ModelMatch) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		if c := cmp.Compare(len(a.ID), len(b.ID)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// uniqueModelMatches labels each id once, in input order.
func uniqueModelMatches(provider string, models []string) []ModelMatch {
	seen := make(map[string]struct{}, len(models))
	out := make([]ModelMatch, 0, len(models))
	for _, id := range models {
		key := strings.ToLower(id)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ModelMatch{ID: id, Label: ModelLabel(provider, id)})
	}
	return out
}

// globMatches keeps, in input order, each candidate whose whole id or whole
// label matches the glob q. Unlike path.Match, * crosses "/".
func globMatches(candidates []ModelMatch, q string) []ModelMatch {
	var b strings.Builder
	for _, r := range q {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	pattern := "(?i)^" + b.String() + "$"
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil // unreachable: every literal rune is quoted
	}
	var out []ModelMatch
	for _, c := range candidates {
		if re.MatchString(c.ID) || re.MatchString(c.Label) {
			c.Score = scoreGlob
			out = append(out, c)
		}
	}
	return out
}

// fuzzyModelScore returns the tier of the strongest predicate q satisfies, or
// false when it satisfies none.
func fuzzyModelScore(id, label, q string) (int, bool) {
	lowerID := strings.ToLower(id)
	lowerLabel := strings.ToLower(label)
	switch {
	case lowerID == q:
		return scoreExact, true
	case strings.HasPrefix(lowerID, q):
		return scoreIDPrefix, true
	case strings.Contains(lowerID, q):
		return scoreIDSubstring, true
	case strings.Contains(lowerLabel, q):
		return scoreLabelSubstring, true
	case tokenPrefixMatch(q, lowerID, lowerLabel):
		return scoreTokenPrefix, true
	}
	// Subsequence runs on the id only: curated labels carry annotation text
	// that a short query would match almost always.
	if bonus, ok := subsequenceBonus(lowerID, q); ok {
		return scoreSubsequence + bonus, true
	}
	return 0, false
}

// isModelSeparator reports whether r splits model-id tokens.
func isModelSeparator(r rune) bool {
	return strings.ContainsRune("-_/.:~", r) || unicode.IsSpace(r)
}

// modelTokens splits s into lowercase tokens on isModelSeparator.
func modelTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), isModelSeparator)
}

// tokenPrefixMatch reports whether every token of q prefixes some id or label
// token. A query with no tokens never matches.
func tokenPrefixMatch(q, lowerID, lowerLabel string) bool {
	queryTokens := modelTokens(q)
	if len(queryTokens) == 0 {
		return false
	}
	surface := slices.Concat(modelTokens(lowerID), modelTokens(lowerLabel))
	for _, qt := range queryTokens {
		if !slices.ContainsFunc(surface, func(t string) bool { return strings.HasPrefix(t, qt) }) {
			return false
		}
	}
	return true
}

// subsequenceBonus matches the non-separator runes of q, in order, against
// lowerID (greedy leftmost). Each matched rune earns 10 when it directly
// follows the previous match and 5 at the start of a token.
func subsequenceBonus(lowerID, q string) (int, bool) {
	var needle []rune
	for _, r := range q {
		if !isModelSeparator(r) {
			needle = append(needle, r)
		}
	}
	if len(needle) == 0 {
		return 0, false
	}
	hay := []rune(lowerID)
	j, prev, bonus := 0, -2, 0
	for i, r := range hay {
		if j == len(needle) {
			break
		}
		if r != needle[j] {
			continue
		}
		if i == prev+1 {
			bonus += 10
		}
		if i == 0 || isModelSeparator(hay[i-1]) {
			bonus += 5
		}
		prev = i
		j++
	}
	if j < len(needle) {
		return 0, false
	}
	return min(bonus, maxSubsequenceBonus), true
}
