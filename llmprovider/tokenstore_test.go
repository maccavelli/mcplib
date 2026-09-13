package llmprovider

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFileTokenStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileTokenStore(dir)
	if err != nil {
		t.Fatalf("NewFileTokenStore: %v", err)
	}
	want := &OAuthSession{
		Provider:  "openai",
		Access:    "at-1",
		Refresh:   "rt-1",
		Expiry:    time.Date(2026, 9, 12, 18, 0, 0, 0, time.UTC),
		Issuer:    "https://auth.openai.com",
		ClientID:  "app_test",
		AccountID: "acct_x",
		TokenURL:  "https://auth.openai.com/oauth/token",
	}
	if err := fs.Save(context.Background(), "openai", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := fs.Load(context.Background(), "openai")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil")
	}
	if got.Provider != want.Provider || got.Access != want.Access || got.Refresh != want.Refresh ||
		!got.Expiry.Equal(want.Expiry) || got.Issuer != want.Issuer || got.ClientID != want.ClientID ||
		got.AccountID != want.AccountID || got.TokenURL != want.TokenURL {
		t.Errorf("roundtrip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if got.Store != fs {
		t.Errorf("Load did not set Store to fs")
	}
}

func TestFileTokenStore_LoadMissingIsNil(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileTokenStore(dir)
	if err != nil {
		t.Fatalf("NewFileTokenStore: %v", err)
	}
	got, err := fs.Load(context.Background(), "openai")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load of missing file = %+v, want nil", got)
	}
}

func TestFileTokenStore_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileTokenStore(dir)
	if err != nil {
		t.Fatalf("NewFileTokenStore: %v", err)
	}
	sess := &OAuthSession{Provider: "openai", Access: "x"}
	cases := []string{"", "open..ai", "../openai", "sub/openai", "openai/../x", `..\openai`}
	for _, p := range cases {
		err := fs.Save(context.Background(), p, sess)
		if err == nil {
			t.Errorf("Save(%q) returned nil error; want error", p)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), "provider") {
			t.Errorf("Save(%q) error %v should mention provider", p, err)
		}
	}
}
