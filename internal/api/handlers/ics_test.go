package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/require"
)

func TestICSFeedHandler(t *testing.T) {
	tz, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)

	tests := []struct {
		name         string
		listFn       func(filters events.Filters, pagination events.Pagination) (events.ListResult, error)
		checkContent func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			name: "200 with events",
			listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
				return events.ListResult{
					Events: []events.Event{
						{ULID: "01J0KXMQZ8RPXJPN8J9Q6TK0WP", Name: "Jazz Fest"},
						{ULID: "01J0KXMQZ8RPXJPN8J9Q6TK0WQ", Name: "Art Walk"},
					},
					NextCursor: "",
				}, nil
			},
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, res.Code)
				require.Equal(t, "text/calendar; charset=utf-8", res.Header().Get("Content-Type"))
				require.True(t, strings.Contains(res.Body.String(), "BEGIN:VCALENDAR"), "body should contain BEGIN:VCALENDAR")
			},
		},
		{
			name: "200 empty list",
			listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
				return events.ListResult{
					Events:     []events.Event{},
					NextCursor: "",
				}, nil
			},
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, res.Code)
				require.Equal(t, "text/calendar; charset=utf-8", res.Header().Get("Content-Type"))
				require.True(t, strings.Contains(res.Body.String(), "BEGIN:VCALENDAR"), "empty list should still return valid VCALENDAR")
			},
		},
		{
			name: "Content-Disposition header",
			listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
				return events.ListResult{Events: []events.Event{}}, nil
			},
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Equal(t, `attachment; filename="events.ics"`, res.Header().Get("Content-Disposition"))
			},
		},
		{
			name: "pagination Link header",
			listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
				return events.ListResult{
					Events:     []events.Event{},
					NextCursor: "next-cursor-value",
				}, nil
			},
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				link := res.Header().Get("Link")
				require.Contains(t, link, "?after=next-cursor-value")
				require.Contains(t, link, `rel="next"`)
			},
		},
		{
			name: "no Link header when no next page",
			listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
				return events.ListResult{
					Events:     []events.Event{},
					NextCursor: "",
				}, nil
			},
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Empty(t, res.Header().Get("Link"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := stubEventsRepo{
				listFn: tc.listFn,
				getFn:  func(_ string) (*events.Event, error) { return nil, nil },
			}

			h := NewICSHandler(events.NewService(repo), "test", "https://example.org")
			h.Loc = tz

			req := httptest.NewRequest(http.MethodGet, "/api/v1/events.ics", nil)
			res := httptest.NewRecorder()

			h.FeedHandler(res, req)

			tc.checkContent(t, res)
		})
	}
}

func TestICSSingleEventHandler(t *testing.T) {
	tz, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)

	tests := []struct {
		name           string
		getFn          func(ulid string) (*events.Event, error)
		tombstoneFn    func(ulid string) (*events.Tombstone, error)
		pathValue      string
		expectedStatus int
		checkContent   func(t *testing.T, res *httptest.ResponseRecorder)
	}{
		{
			name:      "200 found",
			pathValue: "01J0KXMQZ8RPXJPN8J9Q6TK0WP",
			getFn: func(_ string) (*events.Event, error) {
				return &events.Event{
					ULID:           "01J0KXMQZ8RPXJPN8J9Q6TK0WP",
					Name:           "Jazz Fest",
					LifecycleState: "active",
					Occurrences: []events.Occurrence{
						{StartTime: time.Date(2026, 7, 10, 19, 0, 0, 0, tz), Timezone: "America/Toronto"},
					},
				}, nil
			},
			tombstoneFn:    func(_ string) (*events.Tombstone, error) { return nil, events.ErrNotFound },
			expectedStatus: http.StatusOK,
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Equal(t, "text/calendar; charset=utf-8", res.Header().Get("Content-Type"))
				require.True(t, strings.Contains(res.Body.String(), "BEGIN:VCALENDAR"), "body should contain BEGIN:VCALENDAR")
				require.Contains(t, res.Header().Get("Content-Disposition"), "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
			},
		},
		{
			name:      "404 not found",
			pathValue: "01J0KXMQZ8RPXJPN8J9Q6TK0WP",
			getFn: func(_ string) (*events.Event, error) {
				return nil, events.ErrNotFound
			},
			tombstoneFn:    func(_ string) (*events.Tombstone, error) { return nil, events.ErrNotFound },
			expectedStatus: http.StatusNotFound,
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Equal(t, "application/problem+json", res.Header().Get("Content-Type"))
			},
		},
		{
			name:      "410 deleted lifecycle",
			pathValue: "01J0KXMQZ8RPXJPN8J9Q6TK0WP",
			getFn: func(_ string) (*events.Event, error) {
				return &events.Event{
					ULID:           "01J0KXMQZ8RPXJPN8J9Q6TK0WP",
					Name:           "Deleted Event",
					LifecycleState: "deleted",
				}, nil
			},
			tombstoneFn:    func(_ string) (*events.Tombstone, error) { return nil, events.ErrNotFound },
			expectedStatus: http.StatusGone,
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusGone, res.Code)
				require.Contains(t, res.Header().Get("Content-Type"), "json")
			},
		},
		{
			name:           "invalid ULID",
			pathValue:      "not-a-ulid",
			getFn:          func(_ string) (*events.Event, error) { return nil, nil },
			tombstoneFn:    func(_ string) (*events.Tombstone, error) { return nil, nil },
			expectedStatus: http.StatusBadRequest,
			checkContent: func(t *testing.T, res *httptest.ResponseRecorder) {
				require.Equal(t, "application/problem+json", res.Header().Get("Content-Type"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := stubEventsRepo{
				listFn: func(filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
					return events.ListResult{}, nil
				},
				getFn:       tc.getFn,
				tombstoneFn: tc.tombstoneFn,
			}

			h := NewICSHandler(events.NewService(repo), "test", "https://example.org")
			h.Loc = tz

			req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+tc.pathValue+".ics", nil)
			req.SetPathValue("id", tc.pathValue)
			res := httptest.NewRecorder()

			h.SingleEventHandler(res, req)

			require.Equal(t, tc.expectedStatus, res.Code)
			tc.checkContent(t, res)
		})
	}
}
