package mcplib

// StringValue returns the dereferenced string value of p, or an empty string if p is nil.
func StringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// PtrString returns a pointer to the provided string.
//
//go:fix inline
func PtrString(s string) *string {
	return new(s)
}
