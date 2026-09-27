package llmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestChatReasoningEffort_FailureBackoff pins MADR 0013 A6: with the metadata
// host failing, three chat-route thinking calls make one fetch, not three.
func TestChatReasoningEffort_FailureBackoff(t *testing.T) {
	enableModelMetadata(t)
	srv, hits := metadataServer(t, http.StatusServiceUnavailable, "")
	p, err := NewOpencode(ProviderOpencodeGo, "k", "glm-5.3-flash",
		WithModelMetadataURL(srv.URL), WithReasoningEffort(effortLow))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if got := p.chatReasoningEffort(context.Background(), true); got != "" {
			t.Fatalf("effort = %q with metadata down, want none", got)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("3 thinking calls made %d metadata fetches, want 1", n)
	}
}

// TestChatReasoningEffort_LookupHasDeadline pins MADR 0013 A6: the lookup
// inside a request carries its own deadline of at most 5 s, whatever the
// caller's context.
func TestChatReasoningEffort_LookupHasDeadline(t *testing.T) {
	enableModelMetadata(t)
	srv, _ := metadataServer(t, http.StatusOK, smallMetadataDoc)
	rec := &deadlineTransport{}
	p, err := NewOpencode(ProviderOpencodeGo, "k", "glm-5.3-flash", WithModelMetadataURL(srv.URL+"/api.json"),
		WithReasoningEffort(effortLow), WithHTTPClient(&http.Client{Transport: rec}))
	if err != nil {
		t.Fatal(err)
	}
	p.chatReasoningEffort(context.Background(), true)
	left, ok := rec.left("GET /api.json")
	switch {
	case !ok:
		t.Fatal("no metadata request was made")
	case left < 0:
		t.Error("metadata lookup ran without a deadline, want one within 5s")
	case left > 5*time.Second:
		t.Errorf("metadata lookup had %v left, want at most 5s", left.Round(time.Second))
	}
}

// TestLoadModelMetadata_StaleOnRefreshFailure pins MADR 0013 A6: when the
// cached document has expired and the refresh fails, the stale copy is used.
func TestLoadModelMetadata_StaleOnRefreshFailure(t *testing.T) {
	enableModelMetadata(t)
	var fail atomic.Bool
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(smallMetadataDoc))
	}))
	t.Cleanup(srv.Close)
	cfg := ApplyOptions([]ProviderOption{WithModelMetadataURL(srv.URL)})
	if _, err := loadModelMetadata(context.Background(), cfg); err != nil {
		t.Fatalf("first load: %v", err)
	}
	modelMetadataMu.Lock()
	e := modelMetadataCache[srv.URL]
	e.fetched = time.Now().Add(-modelMetadataTTL - time.Minute)
	modelMetadataCache[srv.URL] = e
	modelMetadataMu.Unlock()
	fail.Store(true)
	doc, err := loadModelMetadata(context.Background(), cfg)
	if err != nil || doc[metadataKeyZen] == nil {
		t.Errorf("refresh failed: doc=%v err=%v, want the stale document", doc, err)
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("requests = %d, want 2 (one fetch, one failed refresh)", n)
	}
}

// TestLoadModelMetadata_FailureCachedBriefly pins MADR 0013 A6: a failed
// fetch is remembered, so an immediate second load does not fetch again.
func TestLoadModelMetadata_FailureCachedBriefly(t *testing.T) {
	enableModelMetadata(t)
	srv, hits := metadataServer(t, http.StatusInternalServerError, "")
	cfg := ApplyOptions([]ProviderOption{WithModelMetadataURL(srv.URL)})
	for i := range 2 {
		if _, err := loadModelMetadata(context.Background(), cfg); err == nil {
			t.Fatalf("load %d: want the HTTP 500 error", i+1)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("requests = %d, want 1 (the failure is cached)", n)
	}
}
