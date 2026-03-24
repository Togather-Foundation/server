package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/stretchr/testify/require"
)

type stubEventsRepo struct {
	listFn      func(filters events.Filters, pagination events.Pagination) (events.ListResult, error)
	getFn       func(ulid string) (*events.Event, error)
	tombstoneFn func(ulid string) (*events.Tombstone, error)
	idemKeyFn   func(key string) (*events.IdempotencyKey, error)
	idemInsert  func(params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error)
	idemUpdate  func(key string, eventID string, eventULID string) error
}

func (s stubEventsRepo) List(_ context.Context, filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
	return s.listFn(filters, pagination)
}

func (s stubEventsRepo) GetByULID(_ context.Context, ulid string) (*events.Event, error) {
	return s.getFn(ulid)
}

func (s stubEventsRepo) GetTombstoneByEventID(_ context.Context, _ string) (*events.Tombstone, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) GetTombstoneByEventULID(_ context.Context, ulid string) (*events.Tombstone, error) {
	if s.tombstoneFn == nil {
		return nil, events.ErrNotFound
	}
	return s.tombstoneFn(ulid)
}

func (s stubEventsRepo) Create(_ context.Context, _ events.EventCreateParams) (*events.Event, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) CreateOccurrence(_ context.Context, _ events.OccurrenceCreateParams) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) CreateSource(_ context.Context, _ events.EventSourceCreateParams) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) FindBySourceExternalID(_ context.Context, _ string, _ string) (*events.Event, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) FindByDedupHash(_ context.Context, _ string) (*events.Event, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) GetOrCreateSource(_ context.Context, _ events.SourceLookupParams) (string, error) {
	return "", errors.New("not implemented")
}

func (s stubEventsRepo) GetIdempotencyKey(_ context.Context, key string) (*events.IdempotencyKey, error) {
	if s.idemKeyFn == nil {
		return nil, events.ErrNotFound
	}
	return s.idemKeyFn(key)
}

func (s stubEventsRepo) InsertIdempotencyKey(_ context.Context, params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error) {
	if s.idemInsert == nil {
		return nil, errors.New("not implemented")
	}
	return s.idemInsert(params)
}

func (s stubEventsRepo) UpdateIdempotencyKeyEvent(_ context.Context, key string, eventID string, eventULID string) error {
	if s.idemUpdate == nil {
		return nil
	}
	return s.idemUpdate(key, eventID, eventULID)
}

func (s stubEventsRepo) UpsertPlace(_ context.Context, _ events.PlaceCreateParams) (*events.PlaceRecord, error) {
	return &events.PlaceRecord{ID: "place-id", ULID: "place-ulid"}, nil
}

func (s stubEventsRepo) GetPlaceByULID(_ context.Context, _ string) (*events.PlaceRecord, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) UpsertOrganization(_ context.Context, _ events.OrganizationCreateParams) (*events.OrganizationRecord, error) {
	return &events.OrganizationRecord{ID: "org-id", ULID: "org-ulid"}, nil
}

func (s stubEventsRepo) UpdateEvent(_ context.Context, _ string, _ events.UpdateEventParams) (*events.Event, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) DeleteOccurrencesByEventULID(_ context.Context, _ string) error {
	return nil
}
func (s stubEventsRepo) UpdateOccurrenceDates(_ context.Context, _ string, _ time.Time, _ *time.Time) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) SoftDeleteEvent(_ context.Context, _ string, _ string) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) MergeEvents(_ context.Context, _ string, _ string) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) CreateTombstone(_ context.Context, _ events.TombstoneCreateParams) error {
	return errors.New("not implemented")
}

func (s stubEventsRepo) BeginTx(_ context.Context) (events.Repository, events.TxCommitter, error) {
	return nil, nil, errors.New("not implemented")
}

// Review Queue methods
func (s stubEventsRepo) FindReviewByDedup(_ context.Context, _ *string, _ *string, _ *string) (*events.ReviewQueueEntry, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) CreateReviewQueueEntry(_ context.Context, _ events.ReviewQueueCreateParams) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) UpdateReviewQueueEntry(_ context.Context, _ int, _ events.ReviewQueueUpdateParams) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) GetReviewQueueEntry(_ context.Context, _ int) (*events.ReviewQueueEntry, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) LockReviewQueueEntryForUpdate(_ context.Context, _ int) (*events.ReviewQueueEntry, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) ListReviewQueue(_ context.Context, _ events.ReviewQueueFilters) (*events.ReviewQueueListResult, error) {
	return &events.ReviewQueueListResult{Entries: []events.ReviewQueueEntry{}, NextCursor: nil}, nil
}

func (s stubEventsRepo) ApproveReview(_ context.Context, _ int, _ string, _ *string) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) RejectReview(_ context.Context, _ int, _ string, _ string) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}
func (s stubEventsRepo) MergeReview(_ context.Context, _ int, _ string, _ string) (*events.ReviewQueueEntry, error) {
	return nil, errors.New("not implemented")
}

func (s stubEventsRepo) CleanupExpiredReviews(_ context.Context) error {
	return nil
}

func (s stubEventsRepo) GetSourceTrustLevel(_ context.Context, _ string) (int, error) {
	return 5, nil
}

func (s stubEventsRepo) GetSourceTrustLevelBySourceID(_ context.Context, _ string) (int, error) {
	return 5, nil
}

func (s stubEventsRepo) FindNearDuplicates(_ context.Context, _ string, _ time.Time, _ string, _ float64) ([]events.NearDuplicateCandidate, error) {
	return nil, nil
}
func (s stubEventsRepo) FindSimilarPlaces(_ context.Context, _ string, _ string, _ string, _ float64) ([]events.SimilarPlaceCandidate, error) {
	return nil, nil
}
func (s stubEventsRepo) FindSimilarOrganizations(_ context.Context, _ string, _ string, _ string, _ float64) ([]events.SimilarOrgCandidate, error) {
	return nil, nil
}
func (s stubEventsRepo) MergePlaces(_ context.Context, _ string, primaryID string) (*events.MergeResult, error) {
	return &events.MergeResult{CanonicalID: primaryID}, nil
}
func (s stubEventsRepo) MergeOrganizations(_ context.Context, _ string, primaryID string) (*events.MergeResult, error) {
	return &events.MergeResult{CanonicalID: primaryID}, nil
}
func (s stubEventsRepo) InsertNotDuplicate(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (s stubEventsRepo) IsNotDuplicate(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}
func (s stubEventsRepo) GetPendingReviewByEventUlid(_ context.Context, _ string) (*events.ReviewQueueEntry, error) {
	return nil, nil
}
func (s stubEventsRepo) GetPendingReviewByEventUlidAndDuplicateUlid(_ context.Context, _ string, _ string) (*events.ReviewQueueEntry, error) {
	return nil, nil
}
func (s stubEventsRepo) UpdateReviewWarnings(_ context.Context, _ int, _ []byte) error {
	return nil
}
func (s stubEventsRepo) DismissCompanionWarningMatch(_ context.Context, _ string, _ string) error {
	return nil
}

func (s stubEventsRepo) DismissWarningMatchByReviewID(_ context.Context, _ int, _ string) error {
	return nil
}

func (s stubEventsRepo) CheckOccurrenceOverlap(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
	return false, nil
}

func (s stubEventsRepo) LockEventForUpdate(_ context.Context, _ string) error {
	return nil
}

func (s stubEventsRepo) InsertOccurrence(_ context.Context, params events.OccurrenceCreateParams) (*events.Occurrence, error) {
	return &events.Occurrence{StartTime: params.StartTime}, nil
}

func (s stubEventsRepo) GetOccurrenceByID(_ context.Context, _, _ string) (*events.Occurrence, error) {
	return nil, events.ErrNotFound
}

func (s stubEventsRepo) UpdateOccurrence(_ context.Context, _, occurrenceID string, _ events.OccurrenceUpdateParams) (*events.Occurrence, error) {
	return &events.Occurrence{ID: occurrenceID}, nil
}

func (s stubEventsRepo) DeleteOccurrenceByID(_ context.Context, _, _ string) error {
	return nil
}

func (s stubEventsRepo) CountOccurrences(_ context.Context, _ string) (int64, error) {
	return 2, nil
}

func (s stubEventsRepo) CheckOccurrenceOverlapExcluding(_ context.Context, _ string, _ time.Time, _ *time.Time, _ string) (bool, error) {
	return false, nil
}

func (s stubEventsRepo) DismissPendingReviewsByEventULIDs(_ context.Context, _ []string, _ string) ([]int, error) {
	return nil, nil
}

func (s stubEventsRepo) FindSeriesCompanion(_ context.Context, _ events.SeriesCompanionQuery) (*events.CrossWeekCompanion, error) {
	return nil, nil
}

func (s stubEventsRepo) Rollback(ctx context.Context) error {
	return nil
}

func TestEventsHandlerListSuccess(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			require.Equal(t, 2, pagination.Limit)
			return events.ListResult{
				Events:     []events.Event{{Name: "Jazz Fest"}},
				NextCursor: "next",
			}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
		tombstoneFn: func(_ string) (*events.Tombstone, error) {
			return nil, events.ErrNotFound
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=2", nil)
	req.Header.Set("Accept", "application/ld+json")
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "application/ld+json", res.Header().Get("Content-Type"))

	var payload listResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	items, ok := payload.Items.([]any)
	require.True(t, ok, "Items should be a slice")
	require.Len(t, items, 1)
	item0, ok := items[0].(map[string]any)
	require.True(t, ok, "Item should be a map")
	require.Equal(t, "Jazz Fest", item0["name"])
	require.Equal(t, "Event", item0["@type"])
	require.Equal(t, "next", payload.NextCursor)
}

func TestEventsHandlerListValidationError(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=abc", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Equal(t, "application/problem+json", res.Header().Get("Content-Type"))
}

func TestEventsHandlerGetNotFound(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, events.ErrNotFound
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestEventsHandlerGetSuccess(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return &events.Event{Name: "Jazz Fest"}, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	require.Equal(t, "Jazz Fest", payload["name"])
}

func TestEventsHandlerGetInvalidID(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/bad", nil)
	req.SetPathValue("id", "bad")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestEventsHandlerListServiceError(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, errors.New("boom")
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	params := url.Values{}
	params.Set("limit", "1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?"+params.Encode(), nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
}

func TestEventsHandlerCreateUsesIdempotencyHeader(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
		idemKeyFn: func(key string) (*events.IdempotencyKey, error) {
			require.Equal(t, "abc-123", key)
			return nil, events.ErrNotFound
		},
		idemInsert: func(params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error) {
			require.Equal(t, "abc-123", params.Key)
			return &events.IdempotencyKey{Key: params.Key}, nil
		},
		idemUpdate: func(key string, eventID string, eventULID string) error {
			require.Equal(t, "abc-123", key)
			return nil
		},
	}

	service := events.NewService(repo)
	ingest := events.NewIngestService(repo, "example.org", "America/Toronto", config.ValidationConfig{RequireImage: true, AllowTestDomains: true})
	h := NewEventsHandler(service, ingest, nil, nil, nil, "test", "https://example.org")

	body := `{"name":"Jazz","startDate":"2026-07-10T19:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.IdempotencyHeader, "abc-123")
	rec := httptest.NewRecorder()

	middleware.Idempotency(http.HandlerFunc(h.Create)).ServeHTTP(rec, req)

	require.Contains(t, []int{http.StatusCreated, http.StatusConflict, http.StatusBadRequest}, rec.Code)
}

func TestEventsHandlerListDefaultStartDate(t *testing.T) {
	// When no date params are provided, startDate should default to today so past events
	// are excluded.
	var capturedFilters events.Filters
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			capturedFilters = filters
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	h.Loc = time.UTC
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	require.NotNil(t, capturedFilters.StartDate, "startDate should be defaulted to today")
	now := time.Now().In(time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	require.Equal(t, today, *capturedFilters.StartDate)
}

// TestEventsHandlerGetMultipleOccurrencesSubEvent verifies that when an event
// has multiple occurrences, the Get handler populates the "subEvent" array in
// the JSON-LD response with one entry per occurrence — fixing the admin detail
// UI bug where only a single synthetic occurrence was shown.
// It also pins the full serialization of name, endDate, doorTime, and
// virtualLocation so regressions in those fields are caught immediately.
func TestEventsHandlerGetMultipleOccurrencesSubEvent(t *testing.T) {
	t0 := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	t0end := time.Date(2026, 7, 10, 22, 0, 0, 0, time.UTC)
	t0door := time.Date(2026, 7, 10, 18, 30, 0, 0, time.UTC)
	virtualURL := "https://stream.example.org/weekly-jazz"
	t1 := time.Date(2026, 7, 17, 19, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 24, 19, 0, 0, 0, time.UTC)

	repo := stubEventsRepo{
		getFn: func(_ string) (*events.Event, error) {
			return &events.Event{
				Name: "Weekly Jazz",
				Occurrences: []events.Occurrence{
					// First occurrence: full fields — endDate, doorTime, virtualURL
					{
						StartTime:  t0,
						EndTime:    &t0end,
						DoorTime:   &t0door,
						VirtualURL: &virtualURL,
						Timezone:   "America/Toronto",
					},
					// Second and third: minimal occurrences
					{StartTime: t1, Timezone: "America/Toronto"},
					{StartTime: t2, Timezone: "America/Toronto"},
				},
			}, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))

	subEvent, ok := payload["subEvent"]
	require.True(t, ok, "response must contain 'subEvent' key")

	subEvents, ok := subEvent.([]any)
	require.True(t, ok, "subEvent must be an array")
	require.Len(t, subEvents, 3, "subEvent should contain one entry per occurrence")

	// --- occurrence 0: full fields ---
	m0, ok := subEvents[0].(map[string]any)
	require.True(t, ok, "subEvent[0] must be an object")
	require.Equal(t, "Weekly Jazz", m0["name"], "subEvent[0].name must equal event name")
	require.Equal(t, t0.Format(time.RFC3339), m0["startDate"], "subEvent[0].startDate mismatch")
	require.Equal(t, t0end.Format(time.RFC3339), m0["endDate"], "subEvent[0].endDate mismatch")
	require.Equal(t, t0door.Format(time.RFC3339), m0["doorTime"], "subEvent[0].doorTime mismatch")
	require.Equal(t, "America/Toronto", m0["timezone"], "subEvent[0].timezone mismatch")
	loc0, ok := m0["location"].(map[string]any)
	require.True(t, ok, "subEvent[0].location must be an object (VirtualLocation)")
	require.Equal(t, "VirtualLocation", loc0["@type"], "subEvent[0].location.@type mismatch")
	require.Equal(t, virtualURL, loc0["url"], "subEvent[0].location.url mismatch")

	// --- occurrences 1 & 2: name + startDate present; optional fields absent ---
	for i, entry := range subEvents[1:] {
		idx := i + 1
		m, ok := entry.(map[string]any)
		require.True(t, ok, "subEvent[%d] must be an object", idx)
		require.Equal(t, "Weekly Jazz", m["name"], "subEvent[%d].name must equal event name", idx)
		require.Equal(t, []time.Time{t1, t2}[i].Format(time.RFC3339), m["startDate"], "subEvent[%d].startDate mismatch", idx)
		require.Equal(t, "America/Toronto", m["timezone"], "subEvent[%d].timezone mismatch", idx)
		require.Empty(t, m["endDate"], "subEvent[%d].endDate should be absent for minimal occurrence", idx)
		require.Empty(t, m["doorTime"], "subEvent[%d].doorTime should be absent for minimal occurrence", idx)
		require.Nil(t, m["location"], "subEvent[%d].location should be absent for non-virtual occurrence", idx)
	}
}

// stubPlaceResolver is a minimal EventPlaceResolver for handler tests.
type stubPlaceResolver struct {
	getByULIDFn func(ctx context.Context, ulid string) (*places.Place, error)
}

func (s stubPlaceResolver) GetByULID(ctx context.Context, ulid string) (*places.Place, error) {
	if s.getByULIDFn != nil {
		return s.getByULIDFn(ctx, ulid)
	}
	return nil, places.ErrNotFound
}

// TestEventsHandlerGetSubEventPhysicalVenueOverride verifies that the Get handler
// includes per-occurrence physical venue data in the subEvent array.
//
// Previously the serialization only checked for VirtualURL and silently omitted
// occurrence-level venue overrides, so admin detail could hide which specific
// venue an occurrence was at when it differed from the event's primary venue.
//
// This test pins that a subEvent entry's location is an embedded Place object
// (or at minimum a URI string) when the occurrence carries a VenueULID, and that
// occurrences without a venue override emit no location.
func TestEventsHandlerGetSubEventPhysicalVenueOverride(t *testing.T) {
	const overrideULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	const overrideName = "The Override Venue"
	const overrideCity = "Toronto"

	t0 := time.Date(2026, 8, 1, 19, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 8, 8, 19, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 15, 19, 0, 0, 0, time.UTC)

	repo := stubEventsRepo{
		getFn: func(_ string) (*events.Event, error) {
			overrideULIDVal := overrideULID
			return &events.Event{
				Name: "Touring Series",
				Occurrences: []events.Occurrence{
					// First occurrence: has a venue override (different from primary).
					{StartTime: t0, Timezone: "America/Toronto", VenueULID: &overrideULIDVal},
					// Second: no venue override — no location in subEvent.
					{StartTime: t1, Timezone: "America/Toronto"},
					// Third: also no venue override.
					{StartTime: t2, Timezone: "America/Toronto"},
				},
			}, nil
		},
	}

	resolver := stubPlaceResolver{
		getByULIDFn: func(_ context.Context, ulid string) (*places.Place, error) {
			if ulid == overrideULID {
				return &places.Place{
					ULID: overrideULID,
					Name: overrideName,
					City: overrideCity,
				}, nil
			}
			return nil, places.ErrNotFound
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	h.WithPlaceResolver(resolver)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))

	subEvent, ok := payload["subEvent"]
	require.True(t, ok, "response must contain 'subEvent' key")
	subEvents, ok := subEvent.([]any)
	require.True(t, ok, "subEvent must be an array")
	require.Len(t, subEvents, 3, "subEvent should contain one entry per occurrence")

	// --- occurrence 0: has venue override — subEvent[0].location must be a Place ---
	m0, ok := subEvents[0].(map[string]any)
	require.True(t, ok, "subEvent[0] must be an object")
	loc0, ok := m0["location"].(map[string]any)
	require.True(t, ok, "subEvent[0].location must be an object (Place)")
	require.Equal(t, "Place", loc0["@type"], "subEvent[0].location.@type must be Place")
	require.Equal(t, overrideName, loc0["name"], "subEvent[0].location.name must match override venue")

	// --- occurrences 1 & 2: no venue override — location must be absent ---
	for i, entry := range subEvents[1:] {
		idx := i + 1
		m, ok := entry.(map[string]any)
		require.True(t, ok, "subEvent[%d] must be an object", idx)
		require.Nil(t, m["location"], "subEvent[%d].location must be absent when no venue override", idx)
	}
}

func TestEventsHandlerListSnakeCaseAliasWarning(t *testing.T) {
	repo := stubEventsRepo{
		listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
			return events.ListResult{}, nil
		},
		getFn: func(_ string) (*events.Event, error) {
			return nil, nil
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	h.Loc = time.UTC
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?start_date=2026-06-01", nil)
	res := httptest.NewRecorder()

	h.List(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload listResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
	require.NotEmpty(t, payload.Warnings, "response should include alias warning")
	require.Contains(t, payload.Warnings[0], "start_date")
}

// TestEventsHandlerGetSubEventVenueURIFallback verifies that when the place resolver
// fails for an occurrence venue, the subEvent entry's location falls back to a URI
// string (rather than being absent or panicking).
//
// This is a regression test for the resolveOccurrenceVenueLocation fallback path:
// an occurrence with a VenueULID whose resolver lookup errors must still emit a
// non-nil location using the canonical place URI as a stub.
func TestEventsHandlerGetSubEventVenueURIFallback(t *testing.T) {
	const venueULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	t0 := time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC)

	venueULIDVal := venueULID
	repo := stubEventsRepo{
		getFn: func(_ string) (*events.Event, error) {
			return &events.Event{
				Name: "Fallback URI Event",
				Occurrences: []events.Occurrence{
					{StartTime: t0, Timezone: "America/Toronto", VenueULID: &venueULIDVal},
				},
			}, nil
		},
	}

	// Resolver always fails — must trigger URI fallback.
	resolver := stubPlaceResolver{
		getByULIDFn: func(_ context.Context, _ string) (*places.Place, error) {
			return nil, places.ErrNotFound
		},
	}

	h := NewEventsHandler(events.NewService(repo), nil, nil, nil, nil, "test", "https://example.org")
	h.WithPlaceResolver(resolver)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/01J0KXMQZ8RPXJPN8J9Q6TK0WP", nil)
	req.SetPathValue("id", "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	res := httptest.NewRecorder()

	h.Get(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))

	subEvent, ok := payload["subEvent"]
	require.True(t, ok, "response must contain 'subEvent' key")
	subEvents, ok := subEvent.([]any)
	require.True(t, ok, "subEvent must be an array")
	require.Len(t, subEvents, 1)

	m0, ok := subEvents[0].(map[string]any)
	require.True(t, ok, "subEvent[0] must be an object")

	// When the resolver fails, location must fall back to the place URI string —
	// not nil/absent, not a panic.
	loc := m0["location"]
	require.NotNil(t, loc, "subEvent[0].location must be a URI string (resolver-fallback), not nil")
	locStr, ok := loc.(string)
	require.True(t, ok, "subEvent[0].location must be a string URI when resolver fails, got %T", loc)
	require.Contains(t, locStr, venueULID, "URI fallback must contain the venue ULID")
}
