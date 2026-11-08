package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCreateBatch_RejectsInvalidSuppressReviewWarnings verifies the synchronous
// 4xx guard on the batch ingest path. The handler has no River client in these
// tests, so a request that passes validation falls through to the 500
// dependency check — which proves the 400 came from validation, not wiring.
func TestCreateBatch_RejectsInvalidSuppressReviewWarnings(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{
			name: "unknown code",
			body: `{"events":[{"name":"A","startDate":"2026-12-01T19:00:00Z","suppress_review_warnings":["not_a_code"]}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "non-suppressible integrity code",
			body: `{"events":[{"name":"A","startDate":"2026-12-01T19:00:00Z","suppress_review_warnings":["potential_duplicate"]}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "empty entry",
			body: `{"events":[{"name":"A","startDate":"2026-12-01T19:00:00Z","suppress_review_warnings":[""]}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "allowlisted code passes validation",
			body: `{"events":[{"name":"A","startDate":"2026-12-01T19:00:00Z","suppress_review_warnings":["cross_week_series_companion"]}]}`,
			want: http.StatusInternalServerError, // nil RiverClient, not a validation failure
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			h := &EventsHandler{Env: "test"}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/events:batch", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.CreateBatch(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == http.StatusBadRequest {
				if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "problem+json") {
					t.Errorf("Content-Type = %q, want application/problem+json", ct)
				}
				if !strings.Contains(rec.Body.String(), "suppress_review_warnings") {
					t.Errorf("problem body does not mention the rejected field: %s", rec.Body.String())
				}
			}
		})
	}
}
