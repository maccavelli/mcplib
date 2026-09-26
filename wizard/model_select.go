package wizard

import (
	"fmt"
	"strings"

	"github.com/maccavelli/mcplib/llmprovider"
)

// Search-then-select model flow (MADR 0009 §4–§5). Every interaction uses the
// existing Prompter methods; Prompter itself does not change.
//
// otherModelLabel is the trailing escape hatch on the model menu. A live
// listing can lag a newly released model, and the catalog is curated rather
// than exhaustive, so the user must always be able to name a model directly.
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

// selectModel asks for a search query. A blank query shows the recommended
// menu; any other query searches every usable model and offers the numbered
// matches, Search again, and Other.
func selectModel(p Prompter, d llmprovider.ProviderDescriptor, cat llmprovider.ModelCatalog, o Options) (string, error) {
	title := fmt.Sprintf(chooseModelTitle, d.Label)
	for {
		q, err := p.Input(searchModelsPrompt, "")
		if err != nil {
			return "", fmt.Errorf("search models: %w", err)
		}
		q = strings.TrimSpace(q)
		if q == "" {
			return selectRecommended(p, d, cat.Recommended, o)
		}
		matches := llmprovider.SearchModels(d.ID, cat.Usable, q)
		if len(matches) == 0 {
			p.Notify(LevelWarn, "no %s models match %q", d.Label, q)
			continue
		}
		shown := capMatches(p, matches)
		choices := append(matchChoices(shown), Choice{Label: searchAgainLabel}, Choice{Label: otherModelLabel})
		idx, err := p.Select(title, choices, 0)
		if err != nil {
			return "", fmt.Errorf("select model: %w", err)
		}
		switch {
		case idx < len(shown):
			return shown[idx].ID, nil
		case idx == len(shown):
			continue
		default:
			return enterModelID(p, o)
		}
	}
}

// selectRecommended shows the curated menu. A previously configured model for
// the same provider that is not recommended is listed after the recommended
// rows, marked current, and is the default, so pressing Enter keeps it.
func selectRecommended(p Prompter, d llmprovider.ProviderDescriptor, models []string, o Options) (string, error) {
	choices := modelChoices(d.ID, models)
	defaultIdx, listed := 0, false
	for i, m := range models {
		if m == o.Existing.Model {
			defaultIdx, listed = i, true
		}
	}
	current := ""
	if !listed && o.Existing.Provider == d.ID && o.Existing.Model != "" {
		current = o.Existing.Model
		choices = append(choices, Choice{Label: llmprovider.ModelLabel(d.ID, current), Detail: currentModelDetail})
		defaultIdx = len(models)
	}
	choices = append(choices, Choice{Label: otherModelLabel})
	idx, err := p.Select(fmt.Sprintf(chooseModelTitle, d.Label), choices, defaultIdx)
	if err != nil {
		return "", fmt.Errorf("select model: %w", err)
	}
	switch {
	case idx < len(models):
		return models[idx], nil
	case current != "" && idx == len(models):
		return current, nil
	default:
		return enterModelID(p, o)
	}
}

// enterModelID is the Other escape hatch: the user types a model id.
func enterModelID(p Prompter, o Options) (string, error) {
	manual, err := p.Input("Model id", o.Existing.Model)
	if err != nil {
		return "", fmt.Errorf("enter model: %w", err)
	}
	if manual == "" {
		return "", fmt.Errorf("wizard: no model entered")
	}
	return manual, nil
}

// selectFallbacks offers fallback models, never the primary or one already
// chosen. A blank query offers the remaining recommended models and ends the
// loop; a search round offers the matches and asks whether to search again.
// The result is nil when no MultiSelect was shown, and non-nil (possibly empty)
// once one was, which is the shape this function has always returned.
func selectFallbacks(p Prompter, d llmprovider.ProviderDescriptor, cat llmprovider.ModelCatalog, primary string) ([]string, error) {
	var chosen []string
	for {
		exclude := excludedIDs(primary, chosen)
		recs, usable := without(cat.Recommended, exclude), without(cat.Usable, exclude)
		if len(recs) == 0 && len(usable) == 0 {
			return chosen, nil
		}
		q, err := p.Input(searchFallbackPrompt, "")
		if err != nil {
			return nil, fmt.Errorf("search fallback models: %w", err)
		}
		q = strings.TrimSpace(q)
		if q == "" {
			if len(recs) == 0 {
				return chosen, nil
			}
			idxs, err := p.MultiSelect(chooseFallbacksTitle, modelChoices(d.ID, recs), nil)
			if err != nil {
				return nil, fmt.Errorf("select fallbacks: %w", err)
			}
			chosen = appendPicks(chosen, recs, idxs)
			return chosen, nil // a blank round ends the loop
		}
		matches := llmprovider.SearchModels(d.ID, usable, q)
		if len(matches) == 0 {
			p.Notify(LevelWarn, "no %s models match %q", d.Label, q)
			continue
		}
		shown := capMatches(p, matches)
		idxs, err := p.MultiSelect(searchFallbacksTitle, matchChoices(shown), nil)
		if err != nil {
			return nil, fmt.Errorf("select fallbacks: %w", err)
		}
		chosen = appendPicks(chosen, matchIDs(shown), idxs)
		more, err := p.Confirm(moreFallbacksPrompt, false)
		if err != nil {
			return nil, fmt.Errorf("confirm more fallbacks: %w", err)
		}
		if !more {
			return chosen, nil
		}
	}
}

// excludedIDs is the set a fallback round must not offer.
func excludedIDs(primary string, chosen []string) map[string]struct{} {
	exclude := make(map[string]struct{}, len(chosen)+1)
	exclude[primary] = struct{}{}
	for _, c := range chosen {
		exclude[c] = struct{}{}
	}
	return exclude
}

// without returns a new slice of the models not in exclude, in order.
func without(models []string, exclude map[string]struct{}) []string {
	out := make([]string, 0, len(models))
	for _, m := range models {
		if _, skip := exclude[m]; !skip {
			out = append(out, m)
		}
	}
	return out
}

// appendPicks appends ids[i] for each valid index. chosen becomes non-nil
// even when nothing was picked, because a MultiSelect was shown.
func appendPicks(chosen, ids []string, idxs []int) []string {
	if chosen == nil {
		chosen = []string{}
	}
	for _, i := range idxs {
		if i >= 0 && i < len(ids) {
			chosen = append(chosen, ids[i])
		}
	}
	return chosen
}

// capMatches keeps the first maxSearchResults matches and says so when it
// drops any.
func capMatches(p Prompter, matches []llmprovider.ModelMatch) []llmprovider.ModelMatch {
	if len(matches) <= maxSearchResults {
		return matches
	}
	p.Notify(LevelInfo, "showing %d of %d matches; refine the search to narrow them", maxSearchResults, len(matches))
	return matches[:maxSearchResults]
}

// matchChoices renders matches as menu rows.
func matchChoices(matches []llmprovider.ModelMatch) []Choice {
	out := make([]Choice, 0, len(matches))
	for _, m := range matches {
		out = append(out, Choice{Label: m.Label})
	}
	return out
}

// matchIDs returns the ids of matches, in order.
func matchIDs(matches []llmprovider.ModelMatch) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.ID)
	}
	return out
}
