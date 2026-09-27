package llmprovider

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestListModelCatalog_ErrExplainsDegrade pins MADR 0013 C2: a listing that
// degrades to the static catalog because the fetch failed carries the cause.
func TestListModelCatalog_ErrExplainsDegrade(t *testing.T) {
	failing, _ := metadataServer(t, http.StatusInternalServerError, "")
	cat := listCatalog(context.Background(), t, ProviderOpencodeZen, WithBaseURL(failing.URL))
	if cat.Live || cat.Err == nil || !strings.Contains(cat.Err.Error(), "HTTP 500") {
		t.Errorf("failed listing: Live=%v Err=%v, want Live false and an HTTP 500 cause", cat.Live, cat.Err)
	}
	ok := serveBody(t, opencodeListingFixture)
	if cat := listCatalog(context.Background(), t, ProviderOpencodeZen, WithBaseURL(ok.URL)); !cat.Live || cat.Err != nil {
		t.Errorf("good listing: Live=%v Err=%v, want Live and no error", cat.Live, cat.Err)
	}
	empty := serveBody(t, `{"object":"list","data":[]}`)
	if cat := listCatalog(context.Background(), t, ProviderOpencodeZen, WithBaseURL(empty.URL)); cat.Live || cat.Err != nil {
		t.Errorf("empty listing: Live=%v Err=%v, want the static catalog with no error", cat.Live, cat.Err)
	}
}
