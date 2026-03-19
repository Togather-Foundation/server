package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ─── constants shared across occurrence tests ────────────────────────────────

const (
	occTestEventULID  = "01HTEST0000000000000000001"
	occTestEventID    = "uuid-event-001"
	occTestOccID      = "a1b2c3d4-e5f6-7890-abcd-ef1234567890" // valid UUID
	occTestVirtualURL = "https://example.org/stream"
)

// ─── helper: build a published (non-deleted) test event ─────────────────────

func occTestEvent(state string) *events.Event {
	return &events.Event{
		ID:             occTestEventID,
		ULID:           occTestEventULID,
		Name:           "Test Event",
		LifecycleState: state,
	}
}

// ─── helper: build a minimal Occurrence for mock returns ────────────────────

func occTestOccurrence() *events.Occurrence {
	start := time.Date(2026, 6, 1, 19, 0, 0, 0, time.UTC)
	url := occTestVirtualURL
	return &events.Occurrence{
		ID:         occTestOccID,
		StartTime:  start,
		Timezone:   "America/Toronto",
		VirtualURL: &url,
	}
}

// ─── helper: JSON body builder ───────────────────────────────────────────────

func occBody(t *testing.T, payload any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("occBody: marshal: %v", err)
	}
	return bytes.NewReader(b)
}

// ─── helper: wire up a mock repo that returns a published event on GetByULID ─
// Also sets up BeginTx, LockEventForUpdate for the happy paths that reach
// the transaction layer.

func setupHappyTx(repo *MockRepository, eventState string) {
	setupTxMock(repo) // BeginTx → returns repo + committed TxCommitter
	ev := occTestEvent(eventState)
	repo.On("GetByULID", mock.Anything, occTestEventULID).Return(ev, nil)
	repo.On("LockEventForUpdate", mock.Anything, occTestEventID).Return(nil)
}

// ═══════════════════════════════════════════════════════════════════════════════
// TestCreateOccurrence
// ═══════════════════════════════════════════════════════════════════════════════

func TestCreateOccurrence(t *testing.T) {
	validBody := map[string]any{
		"start_time":  "2026-06-01T19:00:00Z",
		"timezone":    "America/Toronto",
		"virtual_url": occTestVirtualURL,
	}

	tests := []struct {
		name           string
		eventULID      string
		body           any
		setupMock      func(*MockRepository)
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:      "201 success",
			eventULID: occTestEventULID,
			body:      validBody,
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
				// CheckOccurrenceOverlap → no overlap
				repo.On("CheckOccurrenceOverlap", mock.Anything, occTestEventID, mock.AnythingOfType("time.Time"), mock.Anything).Return(false, nil)
				// InsertOccurrence → success
				repo.InsertOccurrenceFn = func(_ context.Context, _ events.OccurrenceCreateParams) (*events.Occurrence, error) {
					return occTestOccurrence(), nil
				}
			},
			expectedStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				assert.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, occTestOccID, resp["id"])
				assert.Equal(t, "2026-06-01T19:00:00Z", resp["start_time"])
			},
		},
		{
			name:           "400 bad JSON",
			eventULID:      occTestEventULID,
			body:           nil, // will be written as raw invalid bytes
			setupMock:      func(_ *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "400 missing start_time",
			eventULID: occTestEventULID,
			body: map[string]any{
				"timezone":    "America/Toronto",
				"virtual_url": occTestVirtualURL,
			},
			setupMock:      func(_ *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "400 bad RFC3339 start_time",
			eventULID: occTestEventULID,
			body: map[string]any{
				"start_time":  "not-a-timestamp",
				"timezone":    "America/Toronto",
				"virtual_url": occTestVirtualURL,
			},
			setupMock:      func(_ *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "404 event not found",
			eventULID: occTestEventULID,
			body:      validBody,
			setupMock: func(repo *MockRepository) {
				setupTxMock(repo)
				repo.On("GetByULID", mock.Anything, occTestEventULID).Return((*events.Event)(nil), events.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:      "400 missing timezone",
			eventULID: occTestEventULID,
			body: map[string]any{
				"start_time":  "2026-06-01T19:00:00Z",
				"virtual_url": occTestVirtualURL,
				// timezone absent
			},
			setupMock:      func(_ *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "409 occurrence overlap",
			eventULID: occTestEventULID,
			body:      validBody,
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
				repo.On("CheckOccurrenceOverlap", mock.Anything, occTestEventID, mock.AnythingOfType("time.Time"), mock.Anything).Return(true, nil)
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name:      "422 location required",
			eventULID: occTestEventULID,
			body: map[string]any{
				"start_time": "2026-06-01T19:00:00Z",
				"timezone":   "America/Toronto",
				// venue_ulid and virtual_url both absent → service returns ErrOccurrenceLocationRequired
				// before reaching the overlap check or InsertOccurrence.
			},
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:      "410 event deleted",
			eventULID: occTestEventULID,
			body:      validBody,
			setupMock: func(repo *MockRepository) {
				setupTxMock(repo)
				repo.On("GetByULID", mock.Anything, occTestEventULID).Return(occTestEvent("deleted"), nil)
			},
			expectedStatus: http.StatusGone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockRepository)
			tc.setupMock(repo)
			h := newTestAdminHandler(repo)

			var bodyReader *bytes.Reader
			if tc.body == nil {
				bodyReader = bytes.NewReader([]byte("{bad json"))
			} else {
				bodyReader = occBody(t, tc.body)
			}

			req := httptest.NewRequest(http.MethodPost, "/admin/events/"+tc.eventULID+"/occurrences", bodyReader)
			req.SetPathValue("id", tc.eventULID)
			req = withAdminUser(req, "admin@example.com")
			rec := httptest.NewRecorder()

			h.CreateOccurrence(rec, req)

			assert.Equal(t, tc.expectedStatus, rec.Code, "status mismatch")
			if tc.checkBody != nil {
				tc.checkBody(t, rec.Body.Bytes())
			}
			repo.AssertExpectations(t)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TestUpdateOccurrence
// ═══════════════════════════════════════════════════════════════════════════════

func TestUpdateOccurrence(t *testing.T) {
	validBody := map[string]any{
		"start_time": "2026-07-01T20:00:00Z",
	}

	tests := []struct {
		name           string
		eventULID      string
		occurrenceID   string
		body           any
		rawBody        []byte // set to send arbitrary bytes
		setupMock      func(*MockRepository)
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:         "200 success",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			body:         validBody,
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
				// GetOccurrenceByID → returns existing occurrence with a VirtualURL set
				// (so location constraint passes without changes)
				repo.GetOccurrenceByIDFn = func(_ context.Context, _, _ string) (*events.Occurrence, error) {
					occ := occTestOccurrence()
					return occ, nil
				}
				// start_time is updated → need overlap check excluding self
				repo.CheckOccurrenceOverlapExclFn = func(_ context.Context, _ string, _ time.Time, _ *time.Time, _ string) (bool, error) {
					return false, nil
				}
				// UpdateOccurrence → returns updated occurrence
				repo.UpdateOccurrenceFn = func(_ context.Context, _, _ string, _ events.OccurrenceUpdateParams) (*events.Occurrence, error) {
					occ := occTestOccurrence()
					t2 := time.Date(2026, 7, 1, 20, 0, 0, 0, time.UTC)
					occ.StartTime = t2
					return occ, nil
				}
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				assert.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, occTestOccID, resp["id"])
			},
		},
		{
			name:           "400 bad JSON",
			eventULID:      occTestEventULID,
			occurrenceID:   occTestOccID,
			rawBody:        []byte("{bad json"),
			setupMock:      func(_ *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:         "400 bad RFC3339 start_time",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			body: map[string]any{
				"start_time": "not-a-timestamp",
			},
			setupMock:      func(_ *MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:         "404 event not found",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			body:         validBody,
			setupMock: func(repo *MockRepository) {
				setupTxMock(repo)
				repo.On("GetByULID", mock.Anything, occTestEventULID).Return((*events.Event)(nil), events.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:         "404 occurrence not found",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			body:         validBody,
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
				// GetOccurrenceByID → not found (default stub returns ErrNotFound)
				// Fn is nil → default stub fires
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:         "409 occurrence overlap",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			body:         validBody,
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
				repo.GetOccurrenceByIDFn = func(_ context.Context, _, _ string) (*events.Occurrence, error) {
					return occTestOccurrence(), nil
				}
				repo.CheckOccurrenceOverlapExclFn = func(_ context.Context, _ string, _ time.Time, _ *time.Time, _ string) (bool, error) {
					return true, nil
				}
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name:         "422 location required",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			body: map[string]any{
				"virtual_url": nil, // explicitly clearing virtual_url; current occ has no venue → constraint fires
			},
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
				// GetOccurrenceByID returns an occurrence with VirtualURL set and no VenueID.
				// Clearing virtual_url via the patch results in both venue and URL being nil,
				// so the service returns ErrOccurrenceLocationRequired before calling UpdateOccurrence.
				repo.GetOccurrenceByIDFn = func(_ context.Context, _, _ string) (*events.Occurrence, error) {
					return occTestOccurrence(), nil
				}
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:         "410 event deleted",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			body:         validBody,
			setupMock: func(repo *MockRepository) {
				setupTxMock(repo)
				repo.On("GetByULID", mock.Anything, occTestEventULID).Return(occTestEvent("deleted"), nil)
			},
			expectedStatus: http.StatusGone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockRepository)
			tc.setupMock(repo)
			h := newTestAdminHandler(repo)

			var bodyReader *bytes.Reader
			switch {
			case tc.rawBody != nil:
				bodyReader = bytes.NewReader(tc.rawBody)
			case tc.body != nil:
				bodyReader = occBody(t, tc.body)
			default:
				bodyReader = bytes.NewReader([]byte("{}"))
			}

			url := "/admin/events/" + tc.eventULID + "/occurrences/" + tc.occurrenceID
			req := httptest.NewRequest(http.MethodPut, url, bodyReader)
			req.SetPathValue("id", tc.eventULID)
			req.SetPathValue("occurrenceId", tc.occurrenceID)
			req = withAdminUser(req, "admin@example.com")
			rec := httptest.NewRecorder()

			h.UpdateOccurrence(rec, req)

			assert.Equal(t, tc.expectedStatus, rec.Code, "status mismatch")
			if tc.checkBody != nil {
				tc.checkBody(t, rec.Body.Bytes())
			}
			repo.AssertExpectations(t)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TestDeleteOccurrence
// ═══════════════════════════════════════════════════════════════════════════════

func TestDeleteOccurrence(t *testing.T) {
	tests := []struct {
		name           string
		eventULID      string
		occurrenceID   string
		setupMock      func(*MockRepository)
		expectedStatus int
	}{
		{
			name:         "204 success",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
				// CountOccurrences → 2 (not the last one)
				repo.CountOccurrencesFn = func(_ context.Context, _ string) (int64, error) {
					return 2, nil
				}
				// DeleteOccurrenceByID → success
				repo.DeleteOccurrenceByIDFn = func(_ context.Context, _, _ string) error {
					return nil
				}
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:         "404 event not found",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			setupMock: func(repo *MockRepository) {
				setupTxMock(repo)
				repo.On("GetByULID", mock.Anything, occTestEventULID).Return((*events.Event)(nil), events.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:         "410 event deleted",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			setupMock: func(repo *MockRepository) {
				setupTxMock(repo)
				repo.On("GetByULID", mock.Anything, occTestEventULID).Return(occTestEvent("deleted"), nil)
			},
			expectedStatus: http.StatusGone,
		},
		{
			name:         "422 last occurrence",
			eventULID:    occTestEventULID,
			occurrenceID: occTestOccID,
			setupMock: func(repo *MockRepository) {
				setupHappyTx(repo, "published")
				// CountOccurrences → 1 (last one)
				repo.CountOccurrencesFn = func(_ context.Context, _ string) (int64, error) {
					return 1, nil
				}
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockRepository)
			tc.setupMock(repo)
			h := newTestAdminHandler(repo)

			url := "/admin/events/" + tc.eventULID + "/occurrences/" + tc.occurrenceID
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req.SetPathValue("id", tc.eventULID)
			req.SetPathValue("occurrenceId", tc.occurrenceID)
			req = withAdminUser(req, "admin@example.com")
			rec := httptest.NewRecorder()

			h.DeleteOccurrence(rec, req)

			assert.Equal(t, tc.expectedStatus, rec.Code, "status mismatch")
			repo.AssertExpectations(t)
		})
	}
}
