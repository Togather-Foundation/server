package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

// mockStatsQuerier implements postgres.Querier for testing stats
type mockStatsQuerier struct {
	postgres.Querier
	totalEvents     int64
	publishedEvents int64
	pendingEvents   int64
	countAllErr     error
	countStateErr   error
}

func (m *mockStatsQuerier) CountAllEvents(ctx context.Context) (int64, error) {
	if m.countAllErr != nil {
		return 0, m.countAllErr
	}
	return m.totalEvents, nil
}

func (m *mockStatsQuerier) CountEventsByLifecycleState(ctx context.Context, state string) (int64, error) {
	if m.countStateErr != nil {
		return 0, m.countStateErr
	}
	switch state {
	case "published":
		return m.publishedEvents, nil
	case "draft":
		return m.pendingEvents, nil
	default:
		return 0, nil
	}
}

// New methods for enhanced stats
func (m *mockStatsQuerier) CountAllPlaces(ctx context.Context) (int64, error) {
	return 10, nil
}

func (m *mockStatsQuerier) CountAllOrganizations(ctx context.Context) (int64, error) {
	return 5, nil
}

func (m *mockStatsQuerier) CountAllSources(ctx context.Context) (int64, error) {
	return 3, nil
}

func (m *mockStatsQuerier) CountAllUsers(ctx context.Context) (int64, error) {
	return 2, nil
}

func (m *mockStatsQuerier) CountEventsCreatedSince(ctx context.Context, since pgtype.Timestamptz) (int64, error) {
	return 50, nil
}

func (m *mockStatsQuerier) CountUpcomingEvents(ctx context.Context) (int64, error) {
	return 100, nil
}

func (m *mockStatsQuerier) CountPastEvents(ctx context.Context) (int64, error) {
	return 900, nil
}

func (m *mockStatsQuerier) GetEventDateRange(ctx context.Context) (postgres.GetEventDateRangeRow, error) {
	return postgres.GetEventDateRangeRow{
		OldestEventDate: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -365), Valid: true},
		NewestEventDate: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, 30), Valid: true},
	}, nil
}

func TestStatsHandler_GetStats(t *testing.T) {
	startTime := time.Now().Add(-24 * time.Hour) // Server started 24 hours ago

	tests := []struct {
		name            string
		querier         *mockStatsQuerier
		wantStatus      int
		wantTotalEvents int64
		wantPublished   int64
		wantPending     int64
		checkUptime     bool
		expectError     bool
	}{
		{
			name: "successful stats retrieval",
			querier: &mockStatsQuerier{
				totalEvents:     1000,
				publishedEvents: 850,
				pendingEvents:   150,
			},
			wantStatus:      http.StatusOK,
			wantTotalEvents: 1000,
			wantPublished:   850,
			wantPending:     150,
			checkUptime:     true,
		},
		{
			name: "zero events",
			querier: &mockStatsQuerier{
				totalEvents:     0,
				publishedEvents: 0,
				pendingEvents:   0,
			},
			wantStatus:      http.StatusOK,
			wantTotalEvents: 0,
			wantPublished:   0,
			wantPending:     0,
		},
		{
			name: "only published events",
			querier: &mockStatsQuerier{
				totalEvents:     500,
				publishedEvents: 500,
				pendingEvents:   0,
			},
			wantStatus:      http.StatusOK,
			wantTotalEvents: 500,
			wantPublished:   500,
			wantPending:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewStatsHandler(tt.querier, "1.0.0", "abc123", startTime, "test")

			req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
			w := httptest.NewRecorder()

			handler.GetStats(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetStats() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.expectError {
				return
			}

			// Parse response
			var stats StatsResponse
			if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Verify response fields
			if stats.TotalEvents != tt.wantTotalEvents {
				t.Errorf("TotalEvents = %v, want %v", stats.TotalEvents, tt.wantTotalEvents)
			}
			if stats.PublishedEvents != tt.wantPublished {
				t.Errorf("PublishedEvents = %v, want %v", stats.PublishedEvents, tt.wantPublished)
			}
			if stats.PendingEvents != tt.wantPending {
				t.Errorf("PendingEvents = %v, want %v", stats.PendingEvents, tt.wantPending)
			}

			// Verify metadata fields
			if stats.Status != "healthy" {
				t.Errorf("Status = %v, want healthy", stats.Status)
			}
			if stats.Version != "1.0.0" {
				t.Errorf("Version = %v, want 1.0.0", stats.Version)
			}
			if stats.GitCommit != "abc123" {
				t.Errorf("GitCommit = %v, want abc123", stats.GitCommit)
			}

			// Check uptime is reasonable (should be around 24 hours = 86400 seconds)
			if tt.checkUptime {
				if stats.Uptime < 86000 || stats.Uptime > 87000 {
					t.Errorf("Uptime = %v, expected around 86400 seconds", stats.Uptime)
				}
			}

			// Verify timestamp format
			if _, err := time.Parse(time.RFC3339, stats.Timestamp); err != nil {
				t.Errorf("Invalid timestamp format: %v", stats.Timestamp)
			}

			// Verify cache headers
			cacheControl := w.Header().Get("Cache-Control")
			if cacheControl != "public, max-age=300" {
				t.Errorf("Cache-Control = %v, want 'public, max-age=300'", cacheControl)
			}
		})
	}
}

func TestStatsHandler_GetStats_NilQuerier(t *testing.T) {
	handler := &StatsHandler{
		queries: nil,
		env:     "test",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()

	handler.GetStats(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("GetStats() with nil querier status = %v, want %v", w.Code, http.StatusInternalServerError)
	}
}
