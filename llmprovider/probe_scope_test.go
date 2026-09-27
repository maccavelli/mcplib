package llmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// generationCounter answers every listing (GET) with an empty or minimal
// catalog and counts generation requests (POST).
func generationCounter(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch {
		case r.URL.Path == "/api.json":
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Query().Get("client_version") != "":
			_, _ = w.Write([]byte(`{"models":[{"slug":"gpt-6-astra","visibility":"list"}]}`))
		default:
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1-mini"}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &posts
}

// TestDiscoverModels_MeteredServicesDoNotProbe: DiscoverModels spends no
// generation on Kilo, OpenCode, Hugging Face or a ChatGPT session (MADR 0012
// §1.6); it returns the curated listing.
func TestDiscoverModels_MeteredServicesDoNotProbe(t *testing.T) {
	for name, build := range map[string]func(url string) (discoverer, error){
		"kilo": func(url string) (discoverer, error) { return NewKilo("k", "some/model", WithBaseURL(url)) },
		"opencode": func(url string) (discoverer, error) {
			return NewOpencode(ProviderOpencodeGo, "k", "glm-5.3-flash", WithBaseURL(url), WithModelMetadataURL(url+"/api.json"))
		},
		"huggingface": func(url string) (discoverer, error) {
			return NewHuggingFace("k", "org/model", WithBaseURL(url), WithModelMetadataURL(url+"/api.json"))
		},
		"chatgpt": func(url string) (discoverer, error) {
			session := &OAuthSession{Issuer: DefaultOpenAIIssuer, Access: "a", Expiry: time.Now().Add(time.Hour)}
			return NewOpenAIWithSource(session, "gpt-6-astra", WithBaseURL(url))
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv, posts := generationCounter(t)
			p, err := build(srv.URL)
			if err != nil {
				t.Fatalf("construct: %v", err)
			}
			models, err := p.DiscoverModels(context.Background())
			if err != nil || len(models) == 0 {
				t.Fatalf("DiscoverModels = %v/%v, want the curated listing", models, err)
			}
			if n := posts.Load(); n != 0 {
				t.Fatalf("DiscoverModels made %d generation requests, want 0", n)
			}
		})
	}
}

// TestDiscoverModels_APIKeyOpenAIStillProbes keeps §1.6 scoped: the providers
// it does not name still probe.
func TestDiscoverModels_APIKeyOpenAIStillProbes(t *testing.T) {
	srv, posts := generationCounter(t)
	p, err := NewOpenAI("k", "gpt-4.1-mini", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewOpenAI: %v", err)
	}
	if _, err := p.DiscoverModels(context.Background()); err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	if posts.Load() == 0 {
		t.Fatal("API-key OpenAI DiscoverModels made no probe")
	}
}
