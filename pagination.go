package mcplib

// Pagination provides standard bounding for standalone tool execution constraints.
// Used by servers that expose paginated tool results (brainstorm, go-refactor, socratic-thinker).
type Pagination struct {
	SessionID *string `json:"session_id,omitempty" jsonschema:"Optional cumulative session correlation ID. Pass this to accumulate structural analysis payloads securely in the background."`
	Limit     int     `json:"limit,omitempty" jsonschema:"Maximum items to return. Defaults to 500 if unassigned."`
	Offset    int     `json:"offset,omitempty" jsonschema:"Pagination offset slice start."`
}

// Apply scales the length input strictly within the bounded limits defensively.
func (p *Pagination) Apply(length int) (start, end int) {
	limit := p.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	start = min(max(p.Offset, 0), length)
	end = min(start+limit, length)
	return start, end
}
