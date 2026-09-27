package llmprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// kiloRankEntry builds one text-only, tool-capable Kilo listing entry created
// ageDays before refNow (created 0 when ageDays < 0).
func kiloRankEntry(id, name, prompt, completion string, ageDays int, reasoning bool, extra string) string {
	created := int64(0)
	if ageDays >= 0 {
		created = refNow.AddDate(0, 0, -ageDays).Unix()
	}
	params := `["tools"]`
	if reasoning {
		params = `["tools","reasoning"]`
	}
	if extra != "" {
		extra = "," + extra
	}
	return fmt.Sprintf(`{"id":%q,"name":%q,"created":%d,"context_length":262144,`+
		`"architecture":{"input_modalities":["text"],"output_modalities":["text"]},`+
		`"pricing":{"prompt":%q,"completion":%q},"supported_parameters":%s,`+
		`"mayTrainOnYourPrompts":false%s}`, id, name, created, prompt, completion, params, extra)
}

// TestListModelCatalog_KiloRanksByProfile pins MADR 0010 §2 for Kilo: the
// listing's own metadata ranks the recommended six per profile, the 8B
// non-reasoning model that today's cheapest-first order puts first leaves the
// six, and kilo-auto/* is excluded under the utility profile only (§3 item 9).
func TestListModelCatalog_KiloRanksByProfile(t *testing.T) {
	pinRankingNow(t, refNow)
	entries := []string{
		kiloRankEntry("a/flash-lite", "A Flash Lite", "0.0000001", "0.0000004", 20, true, ""),
		kiloRankEntry("b/flash", "B Flash", "0.0000003", "0.0000012", 10, true, `"terminalBench":{"overallScore":0.75}`),
		kiloRankEntry("c/pro", "C Pro", "0.000005", "0.000025", 40, true, `"terminalBench":{"overallScore":0.80}`),
		kiloRankEntry("d/mid", "D Mid", "0.000001", "0.000004", 60, true, `"preferredIndex":2`),
		kiloRankEntry("e/mini", "E Mini", "0.0000002", "0.0000008", 90, true, ""),
		kiloRankEntry("f/large", "F Large", "0.000002", "0.000008", 30, true, ""),
		kiloRankEntry("kilo-auto/efficient", "Auto Efficient", "-1", "-1", -1, true, `"preferredIndex":0`),
		kiloRankEntry("o/llama-8b-instruct", "O Llama 8B", "0.00000002", "0.00000004", 100, false, ""),
		kiloRankEntry("g/flash-old", "G Flash Old", "0.0000001", "0.0000001", 600, true, ""),
	}
	srv := serveBody(t, `{"data":[`+strings.Join(entries, ",")+`]}`)

	utility := []string{"b/flash", "d/mid", "c/pro", "a/flash-lite", "e/mini", "f/large"}
	capable := []string{"c/pro", "b/flash", "kilo-auto/efficient", "d/mid", "f/large", "a/flash-lite"}
	for _, tc := range []struct {
		name string
		opts []ProviderOption
		want []string
	}{
		{"default", nil, utility},
		{"utility", []ProviderOption{WithModelProfile(ProfileUtility)}, utility},
		{"capable", []ProviderOption{WithModelProfile(ProfileCapable)}, capable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]ProviderOption{WithBaseURL(srv.URL)}, tc.opts...)
			cat, err := ListModelCatalog(context.Background(), ProviderKilo, "", opts...)
			if err != nil {
				t.Fatalf("ListModelCatalog: %v", err)
			}
			if !slices.Equal(cat.Recommended, tc.want) {
				t.Errorf("Recommended = %v\n            want %v", cat.Recommended, tc.want)
			}
			if len(cat.Usable) != 9 || cat.Usable[0] != "o/llama-8b-instruct" || !cat.Live {
				t.Errorf("Usable = %v (live %v), want 9 ids led by o/llama-8b-instruct", cat.Usable, cat.Live)
			}
		})
	}
}

// hfRankEntry is one live, text-only, tool-capable router model.
func hfRankEntry(id string, throughput int) string {
	return fmt.Sprintf(`{"id":%q,"architecture":{"input_modalities":["text"],"output_modalities":["text"]},`+
		`"providers":[{"provider":"a","status":"live","supports_tools":true,"throughput":%d,`+
		`"first_token_latency_ms":100}]}`, id, throughput)
}

// mdModel is one models.dev entry; name equals family, context 131072.
func mdModel(id, family string, reasoning bool, in, out float64, release string) string {
	return fmt.Sprintf(`%q:{"id":%q,"name":%q,"family":%q,"reasoning":%t,"cost":{"input":%g,"output":%g},`+
		`"release_date":%q,"limit":{"context":131072}}`, id, id, family, family, reasoning, in, out, release)
}

// mdSection wraps entries as one document section.
func mdSection(key string, entries ...string) string {
	return fmt.Sprintf(`%q:{"models":{%s}}`, key, strings.Join(entries, ","))
}

var hfRankListing = `{"object":"list","data":[` + strings.Join([]string{
	hfRankEntry("v/no-reason", 400), hfRankEntry("a/large", 300), hfRankEntry("b/mid", 200),
	hfRankEntry("c/flash", 100), hfRankEntry("d/uncovered", 50),
}, ",") + `]}`

var hfRankMetadata = `{` + mdSection("huggingface",
	mdModel("v/no-reason", "noreason", false, 1, 1, "2026-09-01"),
	mdModel("a/large", "large", true, 1, 4, "2026-09-01"),
	mdModel("b/mid", "mid", true, 0.5, 1.5, "2026-08-01"),
	mdModel("c/flash", "flash", true, 0.1, 0.4, "2026-09-10"),
) + `}`

// hfFallbackOrder is curateHuggingFace's order for hfRankListing: fastest first.
var hfFallbackOrder = []string{"v/no-reason", "a/large", "b/mid", "c/flash", "d/uncovered"}

func listCatalog(ctx context.Context, t *testing.T, provider string, opts ...ProviderOption) ModelCatalog {
	t.Helper()
	cat, err := ListModelCatalog(ctx, provider, "", opts...)
	if err != nil {
		t.Fatalf("ListModelCatalog(%s): %v", provider, err)
	}
	return cat
}

// TestListModelCatalog_HuggingFaceRanksWithMetadata pins MADR 0010 §2: the
// router listing is ranked with models.dev metadata, and an id the metadata
// does not cover is ranked with every field unknown, ahead of the fill.
func TestListModelCatalog_HuggingFaceRanksWithMetadata(t *testing.T) {
	pinRankingNow(t, refNow)
	enableModelMetadata(t)
	listing := serveBody(t, hfRankListing)
	meta, _ := metadataServer(t, http.StatusOK, hfRankMetadata)
	for _, tc := range []struct {
		profile ModelProfile
		want    []string
	}{
		{ProfileUtility, []string{"c/flash", "b/mid", "a/large", "d/uncovered", "v/no-reason"}},
		{ProfileCapable, []string{"a/large", "b/mid", "d/uncovered", "c/flash", "v/no-reason"}},
	} {
		cat := listCatalog(context.Background(), t, ProviderHuggingFace,
			WithBaseURL(listing.URL), WithModelMetadataURL(meta.URL), WithModelProfile(tc.profile))
		assertRanked(t, cat.Recommended, tc.want)
	}
}

// TestListModelCatalog_MetadataFallback pins MADR 0010 §2's degradation: any
// failure of the metadata source leaves today's curation in place.
func TestListModelCatalog_MetadataFallback(t *testing.T) {
	blocking := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blocking.Close)
	for _, tc := range []struct {
		name    string
		enable  bool
		status  int
		body    string
		timeout time.Duration
	}{
		{"500", true, http.StatusInternalServerError, "", 0},
		{"bad JSON", true, http.StatusOK, "{", 0},
		{"missing key", true, http.StatusOK, `{"opencode":{"models":{}}}`, 0},
		{"disabled", false, http.StatusOK, hfRankMetadata, 0},
		{"timeout", true, 0, "", 300 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pinRankingNow(t, refNow)
			if tc.enable {
				enableModelMetadata(t)
			}
			metaURL := blocking.URL
			if tc.timeout == 0 {
				meta, _ := metadataServer(t, tc.status, tc.body)
				metaURL = meta.URL
			}
			ctx := context.Background()
			if tc.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.timeout)
				defer cancel()
			}
			listing := serveBody(t, hfRankListing)
			cat := listCatalog(ctx, t, ProviderHuggingFace, WithBaseURL(listing.URL), WithModelMetadataURL(metaURL))
			assertRanked(t, cat.Recommended, hfFallbackOrder)
			assertRanked(t, cat.Recommended, curateHuggingFace(cat.Usable))
		})
	}
}

func TestListModelCatalog_MetadataFallbackZen(t *testing.T) {
	enableModelMetadata(t)
	listing := serveBody(t, opencodeListingFixture)
	meta, _ := metadataServer(t, http.StatusInternalServerError, "")
	cat := listCatalog(context.Background(), t, ProviderOpencodeZen, WithBaseURL(listing.URL), WithModelMetadataURL(meta.URL))
	want := curateFromCatalog(staticOpencodeCatalog(ProviderOpencodeZen), cat.Usable, isUsableOpencodeModel, RankOpencodeModel)
	assertRanked(t, cat.Recommended, want)
}

// TestListModelCatalog_MetadataIsolation proves TestMain's switch works: with
// the fetch disabled no request reaches the metadata URL, and enabling it
// makes exactly one.
func TestListModelCatalog_MetadataIsolation(t *testing.T) {
	listing := serveBody(t, opencodeListingFixture)
	meta, hits := metadataServer(t, http.StatusOK, smallMetadataDoc)
	opts := []ProviderOption{WithBaseURL(listing.URL), WithModelMetadataURL(meta.URL)}
	listCatalog(context.Background(), t, ProviderOpencodeZen, opts...)
	if n := hits.Load(); n != 0 {
		t.Fatalf("disabled: metadata requests = %d, want 0", n)
	}
	enableModelMetadata(t)
	listCatalog(context.Background(), t, ProviderOpencodeZen, opts...)
	if n := hits.Load(); n != 1 {
		t.Errorf("enabled: metadata requests = %d, want 1", n)
	}
}

// TestListModelCatalog_OpencodeGoGates pins MADR 0010 §3 items 7–8: the
// region-gated and -contributor models leave Go's six but stay searchable,
// and the region gate is Go-only.
func TestListModelCatalog_OpencodeGoGates(t *testing.T) {
	pinRankingNow(t, refNow)
	enableModelMetadata(t)
	models := []string{
		mdModel("deepseek-v4-flash", "deepseek-flash", true, 0.05, 0.1, "2026-09-01"),
		mdModel("muse-spark-1.3-contributor", "muse", true, 0.06, 0.1, "2026-09-01"),
		mdModel("glm-5.3-flash", "glm-flash", true, 0.15, 0.5, "2026-08-26"),
		mdModel("qwen3.8-flash", "qwen-flash", true, 0.2, 0.8, "2026-08-20"),
		mdModel("kimi-k2.6", "kimi", true, 0.6, 2.5, "2026-07-01"),
		mdModel("mimo-v2.6-flash", "mimo", true, 0.14, 0.28, "2026-09-22"),
		mdModel("gpt-6-luna", "gpt-luna", true, 0.1, 0.5, "2026-09-22"),
		mdModel("hy3", "hy", true, 0.3, 1.2, "2026-08-01"),
	}
	meta, _ := metadataServer(t, http.StatusOK,
		`{`+mdSection("opencode-go", models...)+`,`+mdSection("opencode", models...)+`}`)
	listing := serveBody(t, zenStyleListing(opencodeDeepSeekV4Flash, "muse-spark-1.3-contributor", "glm-5.3-flash",
		"qwen3.8-flash", "kimi-k2.6", "mimo-v2.6-flash", "gpt-6-luna", "hy3"))
	opts := []ProviderOption{WithBaseURL(listing.URL), WithModelMetadataURL(meta.URL)}

	goCat := listCatalog(context.Background(), t, ProviderOpencodeGo, opts...)
	assertRanked(t, goCat.Recommended,
		[]string{"mimo-v2.6-flash", "glm-5.3-flash", "qwen3.8-flash", "gpt-6-luna", "hy3", "kimi-k2.6"})
	for _, id := range []string{opencodeDeepSeekV4Flash, "muse-spark-1.3-contributor"} {
		if !slices.Contains(goCat.Usable, id) {
			t.Errorf("Go Usable must keep %s searchable: %v", id, goCat.Usable)
		}
	}
	zenCat := listCatalog(context.Background(), t, ProviderOpencodeZen, opts...)
	assertRanked(t, zenCat.Recommended,
		[]string{opencodeDeepSeekV4Flash, "mimo-v2.6-flash", "glm-5.3-flash", "qwen3.8-flash", "gpt-6-luna", "hy3"})
}

// zenStyleListing is an OpenCode /models body for ids.
func zenStyleListing(ids ...string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf(`{"id":%q,"object":"model","owned_by":"opencode"}`, id))
	}
	return `{"object":"list","data":[` + strings.Join(parts, ",") + `]}`
}

func serveTestdata(t *testing.T, name string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "ranking-2026-09-26", name))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return serveBody(t, string(body))
}

// TestListModelCatalog_Snapshot20260926 replays the 2026-09-26 catalogs
// (testdata/ranking-2026-09-26) and requires MADR 0010 §7's sixes exactly. A
// rule change must update §7 and this test together.
func TestListModelCatalog_Snapshot20260926(t *testing.T) {
	pinRankingNow(t, refNow)
	enableModelMetadata(t)
	meta := serveTestdata(t, "metadata.json")
	for _, tc := range []struct {
		provider, listing string
		usable            int
		utility, capable  []string
	}{
		{ProviderKilo, "kilo.json", 284,
			[]string{"deepseek/deepseek-v4.1-flash", "z-ai/glm-5.3-flash", "google/gemini-3.8-flash",
				"google/gemini-3.6-flash", "meta/muse-spark-1.2", "thinkingmachines/inkling"},
			[]string{"openai/gpt-6-astra", "anthropic/claude-fable-5.1", "openai/gpt-5.6-sol",
				"deepseek/deepseek-v4.1-flash", "google/gemini-3.8-flash", "x-ai/grok-4.6"}},
		// 78: the listing's jev-1.13 and jev-1.13-free are not usable (MADR 0012 §3.1).
		{ProviderOpencodeZen, "zen.json", 78,
			[]string{opencodeDeepSeekV41Flash, "qwen3.8-flash", "glm-5.3-flash", opencodeDeepSeekV4Flash,
				"gemini-3.5-flash-lite", "gemini-3.8-flash"},
			[]string{"claude-opus-5-5", "gpt-6-sol", "gpt-6-luna", "grok-4.7", "gpt-6-astra", "muse-spark-1.3"}},
		{ProviderOpencodeGo, "go.json", 38,
			[]string{"mimo-v2.6-flash", "qwen3.8-flash", "glm-5.3-flash", "gpt-6-luna", "mimo-v2.6-pro", "hy3"},
			[]string{"mimo-v2.6-pro", "gpt-6-luna", "grok-4.7", "glm-5.3", "grok-4.6", "qwen3.8-max"}},
		{ProviderHuggingFace, "hf.json", 131,
			[]string{"deepseek-ai/DeepSeek-V4-Flash-0731", "zai-org/GLM-5.3-Flash", "deepseek-ai/DeepSeek-V4.1-Flash",
				"thinkingmachines/Inkling-Small", "stepfun-ai/Step-3.7-Flash", "stepfun-ai/Step-3.5-Flash"},
			[]string{"zai-org/GLM-5.3", "Qwen/Qwen3.8-27B", "Qwen/Qwen3.8-2.4T-A95B",
				"deepseek-ai/DeepSeek-V4-Pro-0813", "moonshotai/Kimi-K3", "thinkingmachines/Inkling"}},
	} {
		listing := serveTestdata(t, tc.listing)
		for _, p := range []struct {
			profile ModelProfile
			want    []string
		}{{ProfileUtility, tc.utility}, {ProfileCapable, tc.capable}} {
			cat := listCatalog(context.Background(), t, tc.provider,
				WithBaseURL(listing.URL), WithModelMetadataURL(meta.URL), WithModelProfile(p.profile))
			if !slices.Equal(cat.Recommended, p.want) {
				t.Errorf("%s profile %d:\n got  %v\n want %v", tc.provider, p.profile, cat.Recommended, p.want)
			}
			if len(cat.Usable) != tc.usable {
				t.Errorf("%s: %d usable, want %d", tc.provider, len(cat.Usable), tc.usable)
			}
		}
	}
}
