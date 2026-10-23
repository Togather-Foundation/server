package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCreateAPIKeyRejectsInvalidRole guards the pre-DB validation added so that
// a role outside the api_keys CHECK constraint (agent/admin) returns 400 rather
// than a 500 from the database. The handler returns before touching Queries,
// so a nil query set is safe here.
func TestCreateAPIKeyRejectsInvalidRole(t *testing.T) {
	h := NewAPIKeyHandler(nil, "test")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys",
		strings.NewReader(`{"name":"bad-role-key","role":"superuser"}`))
	rec := httptest.NewRecorder()

	h.CreateAPIKey(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), "https://sel.events/problems/validation-error")
}
