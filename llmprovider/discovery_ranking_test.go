package llmprovider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
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
