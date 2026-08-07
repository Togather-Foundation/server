package middleware

import (
	"net/http"
	"strings"
)

// OpenAPILink adds an RFC 8631 service-description Link header to every API
// response, advertising the machine-readable OpenAPI spec location. Clients
// (curl, agents, tooling) can discover the spec without executing JavaScript —
// /api/docs is a Scalar JS shell and would otherwise be a dead end.
//
// Link: <https://<baseURL>/api/v1/openapi.json>; rel="service-desc"; type="application/json"
//
// baseURL is the server's configured external URL (SERVER_BASE_URL); the
// backend sits behind a TLS-terminating proxy, so request-derived schemes would
// yield http:// links. The header is injected at WriteHeader time via a
// ResponseWriter wrapper so handlers that set their own Link header (e.g. the
// ICS alternate on /api/v1/events) cannot clobber it — both coexist.
func OpenAPILink(baseURL string) func(http.Handler) http.Handler {
	specURL := strings.TrimRight(baseURL, "/") + "/api/v1/openapi.json"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(&linkHeaderWriter{ResponseWriter: w, specURL: specURL}, r)
		})
	}
}

// linkHeaderWriter adds the service-desc Link header just before the response
// is flushed, after the handler has run, so it survives handler-written Link
// headers.
type linkHeaderWriter struct {
	http.ResponseWriter
	specURL string
	wrote   bool
}

func (w *linkHeaderWriter) WriteHeader(status int) {
	if !w.wrote {
		w.Header().Add("Link", "<"+w.specURL+">; rel=\"service-desc\"; type=\"application/json\"")
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *linkHeaderWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
