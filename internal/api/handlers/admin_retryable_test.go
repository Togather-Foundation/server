package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/api/problem"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func assertAdminWriteProblem(t *testing.T, rec *httptest.ResponseRecorder, path string, status int) {
	t.Helper()
	assert.Equal(t, status, rec.Code)
	if status == http.StatusServiceUnavailable {
		assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	} else {
		assert.Empty(t, rec.Header().Get("Retry-After"))
	}
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var response problem.ProblemDetails
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "https://sel.events/problems/server-error", response.Type)
	assert.Equal(t, status, response.Status)
	assert.Equal(t, path, response.Instance)
	assert.NotEmpty(t, response.Title)
	assert.NotEmpty(t, response.Detail)
}

func TestAdminEventWrites_Retryable(t *testing.T) {
	tests := []struct {
		name   string
		method string
		suffix string
		state  string
		handle func(*AdminHandler, http.ResponseWriter, *http.Request)
	}{
		{"update", http.MethodPut, "", "published", (*AdminHandler).UpdateEvent},
		{"publish", http.MethodPost, "/publish", "draft", (*AdminHandler).PublishEvent},
		{"unpublish", http.MethodPost, "/unpublish", "published", (*AdminHandler).UnpublishEvent},
		{"delete", http.MethodDelete, "", "published", (*AdminHandler).DeleteEvent},
	}
	errors := []struct {
		name   string
		err    error
		status int
	}{
		{"sentinel", events.ErrRetryable, http.StatusServiceUnavailable},
		{"wrapped", fmt.Errorf("database contention: %w", events.ErrRetryable), http.StatusServiceUnavailable},
		{"non-retryable", fmt.Errorf("database unavailable"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		for _, serviceErr := range errors {
			t.Run(tc.name+"/"+serviceErr.name, func(t *testing.T) {
				repo := new(MockRepository)
				repo.On("GetByULID", mock.Anything, occTestEventULID).Return(occTestEvent(tc.state), nil)
				if tc.name == "delete" {
					repo.On("SoftDeleteEvent", mock.Anything, occTestEventULID, "").Return(serviceErr.err).Once()
				} else {
					repo.On("UpdateEvent", mock.Anything, occTestEventULID, mock.Anything).Return((*events.Event)(nil), serviceErr.err).Once()
				}
				path := "/api/v1/admin/events/" + occTestEventULID + tc.suffix
				req := httptest.NewRequest(tc.method, path, strings.NewReader(`{"name":"Updated event"}`))
				req.SetPathValue("id", occTestEventULID)
				req = withAdminUser(req, "admin")
				rec := httptest.NewRecorder()
				tc.handle(newTestAdminHandler(repo), rec, req)
				assertAdminWriteProblem(t, rec, path, serviceErr.status)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestConsolidate_Retryable(t *testing.T) {
	const retireULID = "01HTEST0000000000000000002"
	const path = "/api/v1/admin/events/consolidate"
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"sentinel", events.ErrRetryable, http.StatusServiceUnavailable},
		{"wrapped", fmt.Errorf("database contention: %w", events.ErrRetryable), http.StatusServiceUnavailable},
		{"non-retryable", fmt.Errorf("database unavailable"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(MockRepository)
			setupTxMock(repo)
			repo.On("GetByULID", mock.Anything, retireULID).Return(&events.Event{
				ID: "uuid-retire", ULID: retireULID, LifecycleState: "published",
			}, nil).Once()
			repo.On("LockEventForUpdate", mock.Anything, "uuid-retire").Return(tc.err).Once()
			body := consolidateBody(t, map[string]any{
				"event_ulid": occTestEventULID,
				"retire":     []string{retireULID},
			})
			req := withAdminUser(httptest.NewRequest(http.MethodPost, path, body), "admin")
			rec := httptest.NewRecorder()
			newTestAdminHandler(repo).ConsolidateEvents(rec, req)
			assertAdminWriteProblem(t, rec, path, tc.status)
			repo.AssertExpectations(t)
		})
	}
}
