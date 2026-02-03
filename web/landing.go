package web

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var indexHTML []byte

// IndexHandler serves the landing page at the web root (/).
// It returns a static HTML page with information for coding agents, humans, and bots.
// The page includes live stats that fetch from /health and /version APIs.
//
// Cache headers: 1 hour with revalidation to allow updates while maintaining performance.
// Only GET and HEAD methods are allowed; other methods return 405 Method Not Allowed.
func IndexHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET and HEAD requests
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Set cache headers: cache for 1 hour, but revalidate
		w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// Write the HTML content
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexHTML) // Error is ignored as WriteHeader already sent status
	})
}
