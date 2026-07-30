package llmprovider

import (
	"log/slog"
	"net/http"
)

// closeResponseBody closes an HTTP response body, logging close failures at debug level.
func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		slog.Debug("llmprovider: close response body", "error", err)
	}
}
