package wizard

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/mcplib/llmprovider"
)

// zenSearchIDs is an OpenCode Zen listing: the six StaticOpencodeZen ids (so
// Recommended is exactly that catalog) plus two ids that only search reaches.
var zenSearchIDs = []string{
	"deepseek-v4.1-flash", "qwen3.8-flash", "glm-5.3-flash", "deepseek-v4-flash",
	"gemini-3.5-flash-lite", "gemini-3.8-flash", "claude-sonnet-5", "claude-opus-5",
}

func zenListing(ids []string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf(`{"id":%q,"object":"model","owned_by":"opencode"}`, id))
	}
	return `{"object":"list","data":[` + strings.Join(parts, ",") + `]}`
}

func zenManyIDs() []string {
	ids := make([]string, 0, 25)
	for i := 1; i <= 25; i++ {
		ids = append(ids, fmt.Sprintf("m-%02d", i))
	}
	return ids
}

// zenServer answers every request with body at the given status.
func zenServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func zenOptions() Options {
	return Options{Discover: true, DiscoverLimit: 5 * time.Second}
}

func labels(choices []Choice) []string {
	out := make([]string, 0, len(choices))
	for _, c := range choices {
		out = append(out, c.Label)
	}
	return out
}

func countContaining(msgs []string, sub string) int {
	n := 0
	for _, m := range msgs {
		if strings.Contains(m, sub) {
			n++
		}
	}
	return n
}

func TestConfigureLLM_BlankSearchShowsRecommended(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 1}, secrets: []string{testKey}}
	res, err := ConfigureLLM(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.Model != llmprovider.StaticClaude[1] {
		t.Errorf("Model = %q, want %q", res.Model, llmprovider.StaticClaude[1])
	}
	if !slices.Contains(f.seenInput, searchModelsPrompt) {
		t.Errorf("inputs = %v, want the search prompt", f.seenInput)
	}
	menu := f.seenSelectItems[1]
	if len(menu) != 7 || menu[6].Label != otherModelLabel {
		t.Errorf("model menu = %v, want 6 recommended rows then Other", labels(menu))
	}
}

func TestConfigureLLM_SearchUsesLiveCorpus(t *testing.T) {
	withEnv(t, nil)
	srv := zenServer(t, http.StatusOK, zenListing(zenSearchIDs))
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOpencodeZen), 0},
		inputs: []string{srv.URL, "sonnet"}, secrets: []string{testKey},
	}
	res, err := ConfigureLLM(context.Background(), f, zenOptions())
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want claude-sonnet-5 (only in the live listing)", res.Model)
	}
	want := []string{llmprovider.ModelLabel(llmprovider.ProviderOpencodeZen, "claude-sonnet-5"), searchAgainLabel, otherModelLabel}
	if got := labels(f.seenSelectItems[1]); !slices.Equal(got, want) {
		t.Errorf("menu = %v, want %v", got, want)
	}
}

func TestConfigureLLM_SearchNoMatchReturnsToSearch(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0},
		inputs: []string{"zzzz", ""}, secrets: []string{testKey},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.Model != llmprovider.StaticClaude[0] {
		t.Errorf("Model = %q, want %q", res.Model, llmprovider.StaticClaude[0])
	}
	if !slices.Contains(f.seenNotify, `no Claude (Anthropic) models match "zzzz"`) {
		t.Errorf("notices = %v, want the no-match notice", f.seenNotify)
	}
}

func TestConfigureLLM_SearchAgain(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 2, 1},
		inputs: []string{"haiku", ""}, secrets: []string{testKey},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	want := []string{
		llmprovider.ModelLabel(llmprovider.ProviderClaude, "claude-haiku-4-5"),
		llmprovider.ModelLabel(llmprovider.ProviderClaude, "claude-3-5-haiku-latest"),
		searchAgainLabel, otherModelLabel,
	}
	if got := labels(f.seenSelectItems[1]); !slices.Equal(got, want) {
		t.Errorf("search menu = %v, want %v", got, want)
	}
	if res.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want claude-sonnet-5 (recommended index 1 after Search again)", res.Model)
	}
}

func TestConfigureLLM_OtherFromSearchResults(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 3},
		inputs: []string{"haiku", "my-id"}, secrets: []string{testKey},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.Model != "my-id" {
		t.Errorf("Model = %q, want my-id", res.Model)
	}
}

func TestConfigureLLM_SearchResultsCapped(t *testing.T) {
	withEnv(t, nil)
	many := zenManyIDs()
	srv := zenServer(t, http.StatusOK, zenListing(many))
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOpencodeZen), 0},
		inputs: []string{srv.URL, "m-"}, secrets: []string{testKey},
	}
	if _, err := ConfigureLLM(context.Background(), f, zenOptions()); err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	menu := labels(f.seenSelectItems[1])
	want := append(slices.Clone(many[:20]), searchAgainLabel, otherModelLabel)
	if !slices.Equal(menu, want) {
		t.Errorf("menu = %v, want %v", menu, want)
	}
	notice := "showing 20 of 25 matches; refine the search to narrow them"
	if n := countContaining(f.seenNotify, notice); n != 1 {
		t.Errorf("cap notice seen %d times, want 1: %v", n, f.seenNotify)
	}
}

func TestConfigureLLM_CurrentModelListed(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 6}, secrets: []string{testKey}}
	res, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{Provider: llmprovider.ProviderClaude, Model: "claude-opus-5"},
	})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	menu := f.seenSelectItems[1]
	if len(menu) != 8 || menu[6] != (Choice{Label: "claude-opus-5", Detail: currentModelDetail}) || menu[7].Label != otherModelLabel {
		t.Errorf("menu = %+v, want 6 recommended rows, the current row, then Other", menu)
	}
	if f.seenSelectDefault[1] != 6 {
		t.Errorf("default = %d, want 6 (the current row)", f.seenSelectDefault[1])
	}
	if res.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want claude-opus-5", res.Model)
	}
}

func TestConfigureLLM_CurrentModelOnlyForSameProvider(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0}, secrets: []string{testKey}}
	if _, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{Provider: llmprovider.ProviderGemini, Model: "claude-opus-5"},
	}); err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	menu := f.seenSelectItems[1]
	if len(menu) != 7 {
		t.Errorf("menu = %+v, want 7 rows (no current row for another provider)", menu)
	}
	for _, c := range menu {
		if c.Detail == currentModelDetail {
			t.Errorf("unexpected current row %+v", c)
		}
	}
	if f.seenSelectDefault[1] != 0 {
		t.Errorf("default = %d, want 0", f.seenSelectDefault[1])
	}
}

func TestConfigureLLM_StaticCatalogNotice(t *testing.T) {
	withEnv(t, nil)
	srv := zenServer(t, http.StatusInternalServerError, "")
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOpencodeZen), 0},
		inputs: []string{srv.URL}, secrets: []string{testKey},
	}
	res, err := ConfigureLLM(context.Background(), f, zenOptions())
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	notice := "live model listing for OpenCode Zen is unavailable; search covers the built-in catalog only"
	if n := countContaining(f.seenNotify, notice); n != 1 {
		t.Errorf("static notice seen %d times, want 1: %v", n, f.seenNotify)
	}
	if res.Model != llmprovider.StaticOpencodeZen[0] {
		t.Errorf("Model = %q, want %q", res.Model, llmprovider.StaticOpencodeZen[0])
	}
}

func TestConfigureLLM_NoStaticNoticeWithoutDiscover(t *testing.T) {
	withEnv(t, nil)
	f := &fakePrompter{t: t, selects: []int{providerIdx(t, llmprovider.ProviderClaude), 0}, secrets: []string{testKey}}
	if _, err := ConfigureLLM(context.Background(), f, Options{}); err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if n := countContaining(f.seenNotify, "live model listing"); n != 0 {
		t.Errorf("notices = %v, want no static notice without Discover", f.seenNotify)
	}
}

func TestConfigureLLM_ChatGPTNoStaticNotice(t *testing.T) {
	store := newMemoryTokenStore()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"gpt-6-astra","visibility":"list"}]}`)),
			Request:    r,
		}, nil
	})}
	f := &fakePrompter{
		t:        t,
		selects:  []int{providerIdx(t, llmprovider.ProviderOpenAI), 1, 0},
		confirms: []bool{true},
	}
	res, err := ConfigureLLM(context.Background(), f, Options{
		Existing: Result{
			Provider:     llmprovider.ProviderOpenAI,
			Kind:         CredOAuth,
			AccessToken:  "existing-access-abcd",
			RefreshToken: "existing-refresh",
			TokenExpiry:  time.Now().Add(time.Hour),
			Issuer:       llmprovider.DefaultOpenAIIssuer,
			ClientID:     llmprovider.DefaultOpenAIClientID,
			AccountID:    "acct_test",
		},
		TokenStore: store,
		Discover:   true,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if n := countContaining(f.seenNotify, "live model listing"); n != 0 {
		t.Errorf("notices = %v, want no static notice for ChatGPT", f.seenNotify)
	}
	if res.Model != "gpt-6-astra" {
		t.Errorf("Model = %q, want the listed gpt-6-astra", res.Model)
	}
}

func TestConfigureLLM_FallbackSearch(t *testing.T) {
	withEnv(t, nil)
	srv := zenServer(t, http.StatusOK, zenListing(zenSearchIDs))
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOpencodeZen), 0},
		inputs: []string{srv.URL, "qwen", "claude"}, secrets: []string{testKey},
		multiSelects: [][]int{{0, 1}}, confirms: []bool{false},
	}
	opts := zenOptions()
	opts.NeedFallbacks = true
	res, err := ConfigureLLM(context.Background(), f, opts)
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if res.Model != "qwen3.8-flash" {
		t.Errorf("Model = %q, want qwen3.8-flash", res.Model)
	}
	wantMenu := []string{
		llmprovider.ModelLabel(llmprovider.ProviderOpencodeZen, "claude-opus-5"),
		llmprovider.ModelLabel(llmprovider.ProviderOpencodeZen, "claude-sonnet-5"),
	}
	if got := labels(f.seenMultiSelectItems[0]); !slices.Equal(got, wantMenu) {
		t.Errorf("fallback menu = %v, want %v (primary excluded)", got, wantMenu)
	}
	if want := []string{"claude-opus-5", "claude-sonnet-5"}; !slices.Equal(res.Fallbacks, want) {
		t.Errorf("Fallbacks = %v, want %v", res.Fallbacks, want)
	}
	if !slices.Equal(f.seenConfirm, []string{moreFallbacksPrompt}) {
		t.Errorf("confirms = %v, want exactly one %q", f.seenConfirm, moreFallbacksPrompt)
	}
}

func TestConfigureLLM_FallbackSearchLoops(t *testing.T) {
	withEnv(t, nil)
	srv := zenServer(t, http.StatusOK, zenListing(zenSearchIDs))
	f := &fakePrompter{
		t: t, selects: []int{providerIdx(t, llmprovider.ProviderOpencodeZen), 0},
		inputs: []string{srv.URL, "qwen", "claude", ""}, secrets: []string{testKey},
		multiSelects: [][]int{{0}, {0}}, confirms: []bool{true},
	}
	opts := zenOptions()
	opts.NeedFallbacks = true
	res, err := ConfigureLLM(context.Background(), f, opts)
	if err != nil {
		t.Fatalf("ConfigureLLM: %v", err)
	}
	if want := []string{"claude-opus-5", "deepseek-v4.1-flash"}; !slices.Equal(res.Fallbacks, want) {
		t.Errorf("Fallbacks = %v, want %v", res.Fallbacks, want)
	}
	second := labels(f.seenMultiSelectItems[1])
	if len(second) != 5 {
		t.Errorf("second menu = %v, want 5 rows", second)
	}
	for _, excluded := range []string{
		llmprovider.ModelLabel(llmprovider.ProviderOpencodeZen, "qwen3.8-flash"),
		llmprovider.ModelLabel(llmprovider.ProviderOpencodeZen, "claude-opus-5"),
	} {
		if slices.Contains(second, excluded) {
			t.Errorf("second menu %v must exclude %q", second, excluded)
		}
	}
	if len(f.seenConfirm) != 1 {
		t.Errorf("confirms = %v, want exactly one", f.seenConfirm)
	}
}

func TestSelectFallbacks_ReturnShape(t *testing.T) {
	d := llmprovider.Descriptors()[0]

	nothingLeft := &fakePrompter{t: t}
	got, err := selectFallbacks(nothingLeft, d, llmprovider.ModelCatalog{Recommended: []string{"a"}, Usable: []string{"a"}}, "a")
	if err != nil {
		t.Fatalf("selectFallbacks: %v", err)
	}
	if got != nil || len(nothingLeft.seenInput) != 0 {
		t.Errorf("nothing remaining: got %#v with inputs %v, want nil and no prompt", got, nothingLeft.seenInput)
	}

	emptyPick := &fakePrompter{t: t, multiSelects: [][]int{{}}}
	got, err = selectFallbacks(emptyPick, d, llmprovider.ModelCatalog{Recommended: []string{"a", "b"}, Usable: []string{"a", "b"}}, "a")
	if err != nil {
		t.Fatalf("selectFallbacks: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("empty MultiSelect: got %#v, want a non-nil empty slice", got)
	}
}

// kiloProfileEntry is one text-only, tool- and reasoning-capable Kilo listing
// entry, created ageDays ago on the real clock.
func kiloProfileEntry(id, prompt, completion string, bench float64, ageDays int) string {
	created := time.Now().AddDate(0, 0, -ageDays).Unix()
	return fmt.Sprintf(`{"id":%q,"name":%q,"created":%d,"context_length":262144,`+
		`"architecture":{"input_modalities":["text"],"output_modalities":["text"]},`+
		`"pricing":{"prompt":%q,"completion":%q},"terminalBench":{"overallScore":%g},`+
		`"supported_parameters":["tools","reasoning"],"mayTrainOnYourPrompts":false}`,
		id, id, created, prompt, completion, bench)
}

// TestConfigureLLM_ProfileReachesListing pins MADR 0010 §1: Options.Profile
// reaches the listing, so a blank search offers each profile's first choice.
func TestConfigureLLM_ProfileReachesListing(t *testing.T) {
	listing := `{"data":[` + kiloProfileEntry("a/flash", "0.0000001", "0.0000004", 0.5, 10) + "," +
		kiloProfileEntry("b/pro", "0.000005", "0.000025", 0.9, 20) + `]}`
	for _, tc := range []struct {
		name    string
		profile llmprovider.ModelProfile
		want    string
	}{
		{"utility (zero value)", llmprovider.ProfileUtility, "a/flash"},
		{"capable", llmprovider.ProfileCapable, "b/pro"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withEnv(t, nil)
			srv := zenServer(t, http.StatusOK, listing)
			f := &fakePrompter{
				t: t, selects: []int{providerIdx(t, llmprovider.ProviderKilo), 0},
				inputs: []string{srv.URL, ""}, secrets: []string{testKey},
			}
			opts := zenOptions()
			opts.Profile = tc.profile
			res, err := ConfigureLLM(context.Background(), f, opts)
			if err != nil {
				t.Fatalf("ConfigureLLM: %v", err)
			}
			if res.Model != tc.want {
				t.Errorf("Model = %q, want %q", res.Model, tc.want)
			}
		})
	}
}
