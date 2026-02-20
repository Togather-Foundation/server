package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

func TestSubmitBatch(t *testing.T) {
	sampleEvents := []events.EventInput{
		{Name: "Test Event", StartDate: "2026-03-01T10:00:00Z"},
		{Name: "Another Event", StartDate: "2026-03-02T10:00:00Z"},
	}

	tests := []struct {
		name           string
		serverStatus   int
		serverBody     any
		events         []events.EventInput
		wantErr        bool
		wantErrContain string
		wantResult     IngestResult
	}{
		{
			name:         "successful batch returns correct IngestResult",
			serverStatus: http.StatusAccepted,
			serverBody: IngestResult{
				BatchID:         "01JKTEST00001",
				EventsCreated:   2,
				EventsDuplicate: 0,
				EventsFailed:    0,
			},
			events: sampleEvents,
			wantResult: IngestResult{
				BatchID:       "01JKTEST00001",
				EventsCreated: 2,
			},
		},
		{
			name:           "non-200 response returns error with status code",
			serverStatus:   http.StatusBadRequest,
			serverBody:     map[string]string{"detail": "invalid events"},
			events:         sampleEvents,
			wantErr:        true,
			wantErrContain: "400",
		},
		{
			name:           "429 response returns error mentioning rate limiting",
			serverStatus:   http.StatusTooManyRequests,
			serverBody:     map[string]string{"detail": "slow down"},
			events:         sampleEvents,
			wantErr:        true,
			wantErrContain: "rate limited",
		},
		{
			name:         "empty events slice sends request and handles empty result",
			serverStatus: http.StatusAccepted,
			serverBody:   IngestResult{BatchID: "01JKEMPTY0001"},
			events:       []events.EventInput{},
			wantResult:   IngestResult{BatchID: "01JKEMPTY0001"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify path and method
				if r.URL.Path != "/api/v1/events:batch" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}
				// Verify headers
				if r.Header.Get("Authorization") == "" {
					t.Errorf("missing Authorization header")
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
				}
				if r.Header.Get("User-Agent") == "" {
					t.Errorf("missing User-Agent header")
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.serverStatus)
				_ = json.NewEncoder(w).Encode(tc.serverBody)
			}))
			defer srv.Close()

			client := NewIngestClient(srv.URL, "test-api-key")
			result, err := client.SubmitBatch(context.Background(), tc.events)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.wantErrContain != "" && !containsString(err.Error(), tc.wantErrContain) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErrContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.BatchID != tc.wantResult.BatchID {
				t.Errorf("BatchID = %q, want %q", result.BatchID, tc.wantResult.BatchID)
			}
			if result.EventsCreated != tc.wantResult.EventsCreated {
				t.Errorf("EventsCreated = %d, want %d", result.EventsCreated, tc.wantResult.EventsCreated)
			}
		})
	}
}

func TestSubmitBatchDryRun(t *testing.T) {
	// Verify no HTTP calls are made by using a server that would fail if contacted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("DryRun should not make any HTTP calls")
		http.Error(w, "unexpected call", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewIngestClient(srv.URL, "test-api-key")

	evts := []events.EventInput{
		{Name: "Event A"},
		{Name: "Event B"},
		{Name: "Event C"},
	}

	result, err := client.SubmitBatchDryRun(context.Background(), evts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BatchID != "dry-run" {
		t.Errorf("BatchID = %q, want %q", result.BatchID, "dry-run")
	}
	if result.EventsCreated != len(evts) {
		t.Errorf("EventsCreated = %d, want %d", result.EventsCreated, len(evts))
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
