package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNegotiatedContentType_Defaults(t *testing.T) {
	if got := NegotiatedContentType(nil); got != contentJSON {
		t.Fatalf("expected %s, got %s", contentJSON, got)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/resource", nil)
	if got := NegotiatedContentType(req); got != contentJSON {
		t.Fatalf("expected %s, got %s", contentJSON, got)
	}
}

func TestContentNegotiation_FormatOverride(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/resource?format=html", nil)
	if got := negotiateContentType(req); got != contentHTML {
		t.Fatalf("expected %s, got %s", contentHTML, got)
	}
}

func TestContentNegotiation_AcceptHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/resource", nil)
	req.Header.Set("Accept", "text/html;q=0.5, application/ld+json;q=0.9")
	if got := negotiateContentType(req); got != contentJSONLD {
		t.Fatalf("expected %s, got %s", contentJSONLD, got)
	}
}

func TestContentNegotiation_Wildcard(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/resource", nil)
	req.Header.Set("Accept", "*/*")
	if got := negotiateContentType(req); got != contentJSON {
		t.Fatalf("expected %s, got %s", contentJSON, got)
	}
}

func TestContentNegotiation_MiddlewareSetsContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/resource", nil)
	req.Header.Set("Accept", "text/turtle")
	res := httptest.NewRecorder()

	handler := ContentNegotiation(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := NegotiatedContentType(r); got != contentTurtle {
			t.Fatalf("expected %s, got %s", contentTurtle, got)
		}
		if vary := w.Header().Get("Vary"); vary != "Accept" {
			t.Fatalf("expected Vary header Accept, got %s", vary)
		}
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(res, req)
}
