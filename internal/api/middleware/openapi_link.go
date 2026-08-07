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
// Link: <https://<host>/api/v1/openapi.json>; rel="service-desc"; type="application/json"
func OpenAPILink(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			scheme := "https"
			if r.TLS == nil {
				scheme = "http"
			}
			specURL := scheme + "://" + r.Host + "/api/v1/openapi.json"
			w.Header().Set("Link", "<"+specURL+">; rel=\"service-desc\"; type=\"application/json\"")
		}
		next.ServeHTTP(w, r)
	})
}
