package llmprovider

import "testing"

// TestModelProfile_ReasoningEffort pins MADR 0010 §1: utility recommends
// "low", capable defers to the model's default, and any other value is
// treated as utility.
func TestModelProfile_ReasoningEffort(t *testing.T) {
	if int(ProfileUtility) != 0 {
		t.Errorf("ProfileUtility = %d, want the zero value", int(ProfileUtility))
	}
	tests := []struct {
		profile ModelProfile
		want    string
	}{
		{ProfileUtility, "low"},
		{ProfileCapable, ""},
		{ModelProfile(7), "low"},
	}
	for _, tc := range tests {
		if got := tc.profile.ReasoningEffort(); got != tc.want {
			t.Errorf("ModelProfile(%d).ReasoningEffort() = %q, want %q", int(tc.profile), got, tc.want)
		}
	}
}

// TestWithModelProfile proves the option, not a default, populates the field.
func TestWithModelProfile(t *testing.T) {
	if got := ApplyOptions(nil).ModelProfile; got != ProfileUtility {
		t.Errorf("default ModelProfile = %d, want ProfileUtility", int(got))
	}
	if got := ApplyOptions([]ProviderOption{WithModelProfile(ProfileCapable)}).ModelProfile; got != ProfileCapable {
		t.Errorf("WithModelProfile(ProfileCapable) = %d, want ProfileCapable", int(got))
	}
}
