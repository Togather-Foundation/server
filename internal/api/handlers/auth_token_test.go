package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/stretchr/testify/assert"
)

func TestTokenExchange(t *testing.T) {
	jwtSecret := "test-token-exchange-secret"
	jwtExpiry := 5 * time.Minute
	env := "test"

	jwtMgr := auth.NewJWTManager(jwtSecret, jwtExpiry, "sel.events")
	handler := NewTokenHandler(jwtMgr, jwtExpiry, env)

	adminKey := &auth.APIKey{
		ID:   "key-admin-1",
		Name: "admin-key",
		Role: string(auth.RoleAdmin),
	}
	agentKey := &auth.APIKey{
		ID:   "key-agent-1",
		Name: "agent-key",
		Role: string(auth.RoleAgent),
	}

	tests := []struct {
		name           string
		setupContext   func(*http.Request) *http.Request
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "admin key returns token",
			setupContext: func(r *http.Request) *http.Request {
				return r.WithContext(middleware.ContextWithAgentKey(r.Context(), adminKey))
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp struct {
					Token     string `json:"token"`
					ExpiresAt string `json:"expires_at"`
				}
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				assert.NotEmpty(t, resp.Token)
				assert.NotEmpty(t, resp.ExpiresAt)

				expiresAt, err := time.Parse(time.RFC3339, resp.ExpiresAt)
				assert.NoError(t, err)
				assert.WithinDuration(t, time.Now().Add(jwtExpiry), expiresAt, 2*time.Second)

				claims, err := jwtMgr.Validate(resp.Token)
				assert.NoError(t, err)
				assert.Equal(t, "admin", claims.Role)
				assert.Equal(t, "admin-key", claims.Subject)
			},
		},
		{
			name: "agent key returns forbidden",
			setupContext: func(r *http.Request) *http.Request {
				return r.WithContext(middleware.ContextWithAgentKey(r.Context(), agentKey))
			},
			expectedStatus: http.StatusForbidden,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Admin API key required")
			},
		},
		{
			name: "nil key in context returns unauthorized",
			setupContext: func(r *http.Request) *http.Request {
				return r
			},
			expectedStatus: http.StatusUnauthorized,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Unauthorized")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", nil)
			req = tt.setupContext(req)
			rec := httptest.NewRecorder()

			handler.Exchange(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code, "Response body: %s", rec.Body.String())
			if tt.checkResponse != nil {
				tt.checkResponse(t, rec)
			}
		})
	}
}

func TestTokenExchangeNilDependencies(t *testing.T) {
	env := "test"
	handler := &TokenHandler{
		JWTManager: nil,
		Env:        env,
	}

	adminKey := &auth.APIKey{
		ID:   "key-admin-1",
		Name: "admin-key",
		Role: string(auth.RoleAdmin),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", nil)
	req = req.WithContext(middleware.ContextWithAgentKey(req.Context(), adminKey))
	rec := httptest.NewRecorder()

	handler.Exchange(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
