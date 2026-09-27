package llmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Model metadata for the open catalogs (MADR 0010 §2): a models.dev-format
// document, by default OpenCode's, fetched concurrently with a listing and
// cached in-process.
const (
	defaultModelMetadataURL = "https://models.opencode.ai/api.json"
	envModelMetadataURL     = "MCPLIB_MODELS_METADATA_URL"
	envDisableModelMetadata = "MCPLIB_DISABLE_MODELS_METADATA"
	modelMetadataTTL        = 10 * time.Minute
	// modelMetadataRetryAfter is how long a failed fetch is remembered before
	// the next load tries again (MADR 0013 A6).
	modelMetadataRetryAfter = time.Minute
	// metadataLookupTimeout bounds the lookup a generation request makes
	// (OpenCode's chat route) whatever the caller's context (MADR 0013 A6).
	metadataLookupTimeout = 5 * time.Second

	metadataKeyZen = "opencode"
	metadataKeyGo  = "opencode-go"
	metadataKeyHF  = "huggingface"
)

// errModelMetadataDisabled reports that the environment turned the fetch off.
var errModelMetadataDisabled = errors.New("model metadata: disabled by " + envDisableModelMetadata)

// modelCost is a models.dev price, USD per million tokens.
type modelCost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// modelMetadata is the subset of one models.dev model entry the ranker reads.
type modelMetadata struct {
	Name      string     `json:"name"`
	Family    string     `json:"family"`
	Reasoning *bool      `json:"reasoning"`
	Cost      *modelCost `json:"cost"`
	Limit     struct {
		Context int `json:"context"`
	} `json:"limit"`
	ReleaseDate      string                 `json:"release_date"`
	Status           string                 `json:"status"`
	ReasoningOptions []modelReasoningOption `json:"reasoning_options"`
	// Interleaved is {"field": name} when the provider expects prior reasoning
	// replayed on assistant messages under that field, or true/absent.
	Interleaved json.RawMessage `json:"interleaved"`
}

// modelReasoningOption is one models.dev reasoning_options entry.
type modelReasoningOption struct {
	Type   string   `json:"type"`
	Values []string `json:"values"`
}

// modelMetadataSection is one provider's entry in the document.
type modelMetadataSection struct {
	Models map[string]modelMetadata `json:"models"`
}

// modelMetadataDoc maps a document key to its models by id.
type modelMetadataDoc map[string]map[string]modelMetadata

// reasoningEfforts returns the effort values the document lists for one
// model's reasoning_options, or nil.
func (d modelMetadataDoc) reasoningEfforts(provider, model string) []string {
	for _, o := range d[modelMetadataKey(provider)][model].ReasoningOptions {
		if o.Type == jsonKeyEffort {
			return o.Values
		}
	}
	return nil
}

// interleavedField returns the message field a model expects its prior
// reasoning replayed under ("reasoning_content"), or "" when it declares none
// (MADR 0012 §2, O5).
func (d modelMetadataDoc) interleavedField(provider, model string) string {
	var declared struct {
		Field string `json:"field"`
	}
	if json.Unmarshal(d[modelMetadataKey(provider)][model].Interleaved, &declared) != nil {
		return ""
	}
	return declared.Field
}

// modelMetadataKey returns the document key for a provider, or "".
func modelMetadataKey(provider string) string {
	switch provider {
	case ProviderOpencodeZen:
		return metadataKeyZen
	case ProviderOpencodeGo:
		return metadataKeyGo
	case ProviderHuggingFace:
		return metadataKeyHF
	}
	return ""
}

// modelMetadataCacheEntry is one URL's last good document and last failure.
type modelMetadataCacheEntry struct {
	doc     modelMetadataDoc // nil until a fetch succeeds; kept when a refresh fails
	fetched time.Time        // when doc was fetched
	failed  time.Time        // when the last fetch failed; zero after a success
	err     error            // that failure
}

var (
	modelMetadataMu    sync.Mutex
	modelMetadataCache = map[string]modelMetadataCacheEntry{}
)

// modelMetadataURL resolves the document URL: the option, then the
// environment, then OpenCode's.
func modelMetadataURL(cfg ProviderConfig) string {
	if cfg.ModelMetadataURL != "" {
		return cfg.ModelMetadataURL
	}
	if v := os.Getenv(envModelMetadataURL); v != "" {
		return v
	}
	return defaultModelMetadataURL
}

// modelMetadataDisabled reports whether MCPLIB_DISABLE_MODELS_METADATA holds
// a true boolean ("1", "true", …).
func modelMetadataDisabled() bool {
	v, err := strconv.ParseBool(os.Getenv(envDisableModelMetadata))
	return err == nil && v
}

// loadModelMetadata returns the document, from the in-process cache while it
// is younger than modelMetadataTTL. A failure is remembered for
// modelMetadataRetryAfter, and while it is, loads answer from the cache
// without fetching: the stale document when there is one, else the failure
// (MADR 0013 A6).
func loadModelMetadata(ctx context.Context, cfg ProviderConfig) (modelMetadataDoc, error) {
	if modelMetadataDisabled() {
		return nil, errModelMetadataDisabled
	}
	url := modelMetadataURL(cfg)
	modelMetadataMu.Lock()
	e := modelMetadataCache[url]
	modelMetadataMu.Unlock()
	switch {
	case e.doc != nil && time.Since(e.fetched) < modelMetadataTTL:
		return e.doc, nil
	case !e.failed.IsZero() && time.Since(e.failed) < modelMetadataRetryAfter:
		return e.cached()
	}
	doc, err := fetchModelMetadata(ctx, url, cfg.HTTPClient)
	modelMetadataMu.Lock()
	defer modelMetadataMu.Unlock()
	if err != nil {
		e.failed, e.err = time.Now(), err
		modelMetadataCache[url] = e
		return e.cached()
	}
	modelMetadataCache[url] = modelMetadataCacheEntry{doc: doc, fetched: time.Now()}
	return doc, nil
}

// cached answers from a cache entry after a failure: the stale document when
// there is one, else the failure.
func (e modelMetadataCacheEntry) cached() (modelMetadataDoc, error) {
	if e.doc != nil {
		return e.doc, nil
	}
	return nil, e.err
}

// fetchModelMetadata performs one GET. Go's transport requests gzip itself.
func fetchModelMetadata(ctx context.Context, url string, client *http.Client) (modelMetadataDoc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("model metadata: %w", err)
	}
	identityOf(ProviderConfig{}).setUserAgent(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("model metadata: %w", err)
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model metadata: %s returned HTTP %d", url, resp.StatusCode)
	}
	return decodeModelMetadata(resp.Body)
}

// decodeModelMetadata keeps the three sections MADR 0010 reads. A section
// without models is treated as absent.
func decodeModelMetadata(r io.Reader) (modelMetadataDoc, error) {
	var raw struct {
		Zen *modelMetadataSection `json:"opencode"`
		Go  *modelMetadataSection `json:"opencode-go"`
		HF  *modelMetadataSection `json:"huggingface"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("model metadata: decode: %w", err)
	}
	doc := modelMetadataDoc{}
	for key, s := range map[string]*modelMetadataSection{
		metadataKeyZen: raw.Zen, metadataKeyGo: raw.Go, metadataKeyHF: raw.HF,
	} {
		if s != nil && s.Models != nil {
			doc[key] = s.Models
		}
	}
	return doc, nil
}

// modelMetadataResult carries one fetch's outcome across a goroutine.
type modelMetadataResult struct {
	doc modelMetadataDoc
	err error
}

// startModelMetadata fetches the document concurrently with a listing, under
// the listing's context (MADR 0010 §2). The channel receives exactly one value.
func startModelMetadata(ctx context.Context, cfg ProviderConfig) <-chan modelMetadataResult {
	ch := make(chan modelMetadataResult, 1)
	go func() {
		doc, err := loadModelMetadata(ctx, cfg)
		ch <- modelMetadataResult{doc: doc, err: err}
	}()
	return ch
}

// metadataCandidate reads one models.dev entry (MADR 0010 §2). An id the
// document does not cover is a candidate with every field unknown.
func metadataCandidate(id string, m modelMetadata, covered bool, now time.Time) rankCandidate {
	if !covered {
		return rankCandidate{id: id, group: rankGroup(id, ""), small: isSmallModel(id)}
	}
	c := rankCandidate{
		id:      id,
		group:   rankGroup(id, m.Family),
		small:   isSmallModel(id, m.Family, m.Name),
		context: m.Limit.Context,
		status:  m.Status,
	}
	if m.Reasoning != nil {
		c.reasoning, c.reasoningKnown = *m.Reasoning, true
	}
	if m.Cost != nil {
		c.cost, c.costKnown = m.Cost.Input+m.Cost.Output, true
	}
	if t, ok := parseRankDate(m.ReleaseDate); ok {
		c.ageDays, c.ageKnown = floorDays(t, now), true
	}
	return c
}

// metadataCurate ranks a provider's usable models with the metadata document
// (MADR 0010 §2). A failed or disabled fetch, or a document without the
// provider's key, returns fallback's curation unchanged.
func metadataCurate(provider string, profile ModelProfile, meta <-chan modelMetadataResult, fallback func([]string) []string) func([]string) []string {
	return func(usable []string) []string {
		res := <-meta
		models, ok := res.doc[modelMetadataKey(provider)]
		if res.err != nil || !ok {
			return fallback(usable)
		}
		now := rankingNow()
		cands := make([]rankCandidate, 0, len(usable))
		for _, id := range usable {
			m, covered := models[id]
			cands = append(cands, metadataCandidate(id, m, covered, now))
		}
		return rankRecommended(profile, provider, cands, append(fallback(usable), usable...))
	}
}
