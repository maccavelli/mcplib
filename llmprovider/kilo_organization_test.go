package llmprovider

import (
	"net/http"
	"testing"
)

// TestKilo_OrganizationOption: WithKiloOrganization scopes generation with
// X-KILOCODE-ORGANIZATIONID and lists /api/organizations/{id}/models.
func TestKilo_OrganizationOption(t *testing.T) {
	wantKiloCalls(t, kiloCalls(t, "sk-plain", WithKiloOrganization("org-1")),
		kiloCall{http.MethodPost, kiloBaseURL + "/chat/completions", "Bearer sk-plain", "org-1"},
		kiloCall{http.MethodGet, "https://api.kilo.ai/api/organizations/org-1/models", "Bearer sk-plain", "org-1"})
}
