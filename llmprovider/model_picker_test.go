package llmprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

// pickerSections maps models.opencode.ai's sections to the providers the live
// suite picks models for (MADR 0012 §3.4). mcplib's own decoder keeps only
// the sections it ranks with, so the picker reads the document itself.
var pickerSections = map[string]string{
	metadataKeyZen: ProviderOpencodeZen,
	metadataKeyGo:  ProviderOpencodeGo,
	"kilo":         ProviderKilo,
}

// pickerDoc is provider -> model id -> status, from the document.
type pickerDoc map[string]map[string]string

// decodePickerDoc reads the picker's sections of a models.dev-format document.
func decodePickerDoc(r io.Reader) (pickerDoc, error) {
	var raw map[string]struct {
		Models map[string]struct {
			Status string `json:"status"`
		} `json:"models"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("model picker: decode: %w", err)
	}
	doc := pickerDoc{}
	for key, provider := range pickerSections {
		section, ok := raw[key]
		if !ok {
			continue
		}
		doc[provider] = make(map[string]string, len(section.Models))
		for id, m := range section.Models {
			doc[provider][id] = m.Status
		}
	}
	return doc, nil
}

// firstActiveModel returns the first candidate the document lists for
// provider with a status other than "deprecated", and false when none is.
func firstActiveModel(doc pickerDoc, provider string, candidates []string) (string, bool) {
	models := doc[provider]
	for _, id := range candidates {
		if status, ok := models[id]; ok && status != "deprecated" {
			return id, true
		}
	}
	return "", false
}

// TestFirstActiveModel pins the picker the live suite uses.
func TestFirstActiveModel(t *testing.T) {
	doc := pickerDoc{
		ProviderOpencodeGo: {"old": "deprecated", "live1": "", "live2": "beta"},
		ProviderKilo:       {"vendor/model": ""},
	}
	for _, tc := range []struct {
		name, provider string
		candidates     []string
		want           string
		ok             bool
	}{
		{"skips deprecated", ProviderOpencodeGo, []string{"old", "live1"}, "live1", true},
		{"skips absent", ProviderOpencodeGo, []string{"gone", "live2", "live1"}, "live2", true},
		{"keeps order", ProviderOpencodeGo, []string{"live1", "live2"}, "live1", true},
		{"kilo section", ProviderKilo, []string{"gone/model", "vendor/model"}, "vendor/model", true},
		{"other provider only", ProviderOpencodeGo, []string{"vendor/model"}, "", false},
		{"none active", ProviderOpencodeGo, []string{"old", "gone"}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := firstActiveModel(doc, tc.provider, tc.candidates)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("firstActiveModel(%s, %v) = %q, %t; want %q, %t", tc.provider, tc.candidates, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestDecodePickerDoc: the Zen, Go and Kilo sections are kept with each
// model's status; other sections are not.
func TestDecodePickerDoc(t *testing.T) {
	doc, err := decodePickerDoc(strings.NewReader(`{
  "opencode": {"models": {"zen-model": {"status": "beta"}}},
  "opencode-go": {"models": {"go-model": {"status": "deprecated"}}},
  "kilo": {"models": {"vendor/kilo-model": {}}},
  "huggingface": {"models": {"hf/model": {}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	want := pickerDoc{
		ProviderOpencodeZen: {"zen-model": "beta"},
		ProviderOpencodeGo:  {"go-model": "deprecated"},
		ProviderKilo:        {"vendor/kilo-model": ""},
	}
	if fmt.Sprint(doc) != fmt.Sprint(want) {
		t.Fatalf("decodePickerDoc = %v, want %v", doc, want)
	}
}
