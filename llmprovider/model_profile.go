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
