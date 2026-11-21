package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestEventListsPublicURL(t *testing.T) {
	const (
		baseURL   = "https://example.org"
		publicURL = "https://organizer.example/events/jazz-fest"
		ulid      = "01HYX3KQW7ERTV9XNBM2P8QJZF"
	)
	repo := stubEventsRepo{
		listFn: func(_ events.Filters, _ events.Pagination) (events.ListResult, error) {
			return events.ListResult{Events: []events.Event{
				{ULID: ulid, Name: "Jazz Fest", PublicURL: publicURL},
				{Name: "Art Show"},
			}}, nil
		},
	}
	service := events.NewService(repo)
	public := NewEventsHandler(service, nil, nil, nil, nil, "test", baseURL, zerolog.Nop())
	admin := NewAdminHandler(service, nil, nil, nil, nil, nil, "test", baseURL)

	for _, tc := range []struct {
		path   string
		handle http.HandlerFunc
	}{
		{path: "/api/v1/events", handle: public.List},
		{path: "/api/v1/admin/events", handle: admin.ListEvents},
		{path: "/api/v1/admin/events/pending", handle: admin.ListPendingEvents},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Accept", "application/ld+json")
			res := httptest.NewRecorder()
			tc.handle(res, req)

			require.Equal(t, http.StatusOK, res.Code)
			require.Equal(t, "application/ld+json", res.Header().Get("Content-Type"))
			var payload struct {
				Items []map[string]any `json:"items"`
			}
			require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
			require.Len(t, payload.Items, 2)
			require.Equal(t, baseURL+"/events/"+ulid, payload.Items[0]["@id"])
			require.Equal(t, publicURL, payload.Items[0]["url"])
			require.NotContains(t, payload.Items[1], "url")
		})
	}
}
