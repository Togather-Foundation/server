package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAPILinkSetsHeaderOnAPIResponses(t *testing.T) {
	h := OpenAPILink(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestOpenAPILinkHTTPS(t *testing.T) {
	h := OpenAPILink(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "https://sel.example.com/api/v1/events", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	link := res.Header().Get("Link")
	require.Contains(t, link, "<https://sel.example.com/api/v1/openapi.json>")
}

func TestOpenAPILinkSkipsNonAPI(t *testing.T) {
	h := OpenAPILink(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://sel.example.com/", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Empty(t, res.Header().Get("Link"))
}
