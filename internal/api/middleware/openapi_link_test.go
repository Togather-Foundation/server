package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAPILinkSetsHeaderOnAPIResponses(t *testing.T) {
	h := OpenAPILink("http://sel.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://sel.example.com/api/v1/events", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	link := res.Header().Get("Link")
	require.Contains(t, link, "<http://sel.example.com/api/v1/openapi.json>")
	require.Contains(t, link, `rel="service-desc"`)
	require.Contains(t, link, `type="application/json"`)
}

// TestOpenAPILinkUsesConfiguredBaseURL verifies the header uses the configured
// SERVER_BASE_URL (which is https behind the TLS-terminating proxy), not the
// request scheme.
func TestOpenAPILinkUsesConfiguredBaseURL(t *testing.T) {
	h := OpenAPILink("https://staging.toronto.togather.foundation")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request arrives over plain HTTP internally (proxy terminates TLS).
	req := httptest.NewRequest(http.MethodGet, "http://staging.internal/api/v1/events", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	link := res.Header().Get("Link")
	require.Contains(t, link, "<https://staging.toronto.togather.foundation/api/v1/openapi.json>")
	require.NotContains(t, link, "staging.internal")
}

func TestOpenAPILinkSkipsNonAPI(t *testing.T) {
	h := OpenAPILink("http://sel.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://sel.example.com/", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Empty(t, res.Header().Get("Link"))
}

// TestOpenAPILinkCoexistsWithHandlerLink verifies the service-desc Link header
// survives handlers that set their own Link header (e.g. the ICS alternate on
// /api/v1/events), which previously clobbered it via Header.Set.
func TestOpenAPILinkCoexistsWithHandlerLink(t *testing.T) {
	h := OpenAPILink("http://sel.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", "<http://sel.example.com/api/v1/events.ics>; rel=\"alternate\"; type=\"text/calendar\"")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://sel.example.com/api/v1/events", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	joined := ""
	for _, l := range res.Header().Values("Link") {
		joined += l + "\n"
	}
	require.Contains(t, joined, "service-desc")
	require.Contains(t, joined, "rel=\"alternate\"")
}

// TestOpenAPILinkWritePathInjectsHeader covers the implicit-WriteHeader path
// (handlers that call Write without an explicit WriteHeader).
func TestOpenAPILinkWritePathInjectsHeader(t *testing.T) {
	h := OpenAPILink("http://sel.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "http://sel.example.com/api/v1/events", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Header().Get("Link"), "service-desc")
}
