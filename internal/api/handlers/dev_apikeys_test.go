package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestDevKeyCreate tests API key creation scenarios
func TestDevKeyCreate(t *testing.T) {
	developerID := uuid.New()
	keyID := uuid.New()

	tests := []struct {
		name           string
		requestBody    createDevAPIKeyRequest
		mockSetup      func(*MockDeveloperRepository)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "successful key creation",
			requestBody: createDevAPIKeyRequest{
				Name: "Test API Key",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetDeveloperByID", mock.Anything, developerID).Return(&developers.Developer{
					ID:      developerID,
					MaxKeys: 5,
				}, nil)
				m.On("CountDeveloperAPIKeys", mock.Anything, developerID).Return(int64(2), nil)
				m.On("CreateAPIKey", mock.Anything, mock.MatchedBy(func(params developers.CreateAPIKeyDBParams) bool {
					return params.Name == "Test API Key" && params.DeveloperID == developerID && params.Role == "agent"
				})).Return(&developers.CreateAPIKeyResult{
					ID:            keyID,
					Key:           "sel_test_abcdefghijklmnopqrstuvwxyz123456",
					Prefix:        "sel_test",
					Name:          "Test API Key",
					Role:          "agent",
					RateLimitTier: "agent",
					CreatedAt:     time.Now(),
				}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp createDevAPIKeyResponse
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				assert.NotEmpty(t, resp.ID)
				assert.NotEmpty(t, resp.Key)
				assert.NotEmpty(t, resp.Prefix)
				assert.Equal(t, "Test API Key", resp.Name)
				assert.Equal(t, "agent", resp.Role)
				assert.Contains(t, resp.Warning, "IMPORTANT")
			},
		},
		{
			name: "max keys limit reached",
			requestBody: createDevAPIKeyRequest{
				Name: "Another Key",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetDeveloperByID", mock.Anything, developerID).Return(&developers.Developer{
					ID:      developerID,
					MaxKeys: 5,
				}, nil)
				m.On("CountDeveloperAPIKeys", mock.Anything, developerID).Return(int64(5), nil)
			},
			expectedStatus: http.StatusForbidden,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Maximum number of API keys reached")
			},
		},
		{
			name: "key format verification - has prefix",
			requestBody: createDevAPIKeyRequest{
				Name: "Verified Key",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetDeveloperByID", mock.Anything, developerID).Return(&developers.Developer{
					ID:      developerID,
					MaxKeys: 5,
				}, nil)
				m.On("CountDeveloperAPIKeys", mock.Anything, developerID).Return(int64(0), nil)
				m.On("CreateAPIKey", mock.Anything, mock.Anything).Return(&developers.CreateAPIKeyResult{
					ID:            keyID,
					Key:           "sel_dev_1234567890abcdefghijklmnopqrst",
					Prefix:        "sel_dev",
					Name:          "Verified Key",
					Role:          "agent",
					RateLimitTier: "agent",
					CreatedAt:     time.Now(),
				}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp createDevAPIKeyResponse
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				// Just verify key and prefix are present - actual values depend on service implementation
				assert.NotEmpty(t, resp.Key, "API key should be present")
				assert.NotEmpty(t, resp.Prefix, "Prefix should be present")
			},
		},
		{
			name: "with expiry",
			requestBody: createDevAPIKeyRequest{
				Name:          "Expiring Key",
				ExpiresInDays: intPtr(30),
			},
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetDeveloperByID", mock.Anything, developerID).Return(&developers.Developer{
					ID:      developerID,
					MaxKeys: 5,
				}, nil)
				m.On("CountDeveloperAPIKeys", mock.Anything, developerID).Return(int64(0), nil)
				m.On("CreateAPIKey", mock.Anything, mock.Anything).Return(&developers.CreateAPIKeyResult{
					ID:            keyID,
					Key:           "sel_dev_abc123",
					Prefix:        "sel_dev",
					Name:          "Expiring Key",
					Role:          "agent",
					RateLimitTier: "agent",
					ExpiresAt:     ptrTime(time.Now().AddDate(0, 0, 30)),
					CreatedAt:     time.Now(),
				}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp createDevAPIKeyResponse
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				assert.NotNil(t, resp.ExpiresAt)
			},
		},
		{
			name:        "missing name",
			requestBody: createDevAPIKeyRequest{},
			mockSetup: func(m *MockDeveloperRepository) {
				// No mock setup - validation happens before service call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Name is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockDeveloperRepository)
			tt.mockSetup(mockRepo)

			service := developers.NewService(mockRepo, zerolog.Nop())
			auditLogger := audit.NewLogger()
			handler := NewDeveloperAPIKeyHandler(
				service,
				zerolog.Nop(),
				"test",
				auditLogger,
			)

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/dev/api-keys", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = withDeveloperClaims(req, developerID, "dev@example.com", "Test Developer")
			rec := httptest.NewRecorder()

			handler.CreateAPIKey(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code, "Response body: %s", rec.Body.String())
			if tt.checkResponse != nil {
				tt.checkResponse(t, rec)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestDevKeyList tests listing developer's own API keys
func TestDevKeyList(t *testing.T) {
	developer1ID := uuid.New()
	developer2ID := uuid.New()
	key1ID := uuid.New()
	key2ID := uuid.New()

	t.Run("only shows own keys", func(t *testing.T) {
		mockRepo := new(MockDeveloperRepository)

		// Developer 1 has 2 keys
		mockRepo.On("ListDeveloperAPIKeys", mock.Anything, developer1ID).Return([]developers.APIKey{
			{
				ID:            key1ID,
				Prefix:        "sel_dev1",
				Name:          "Developer 1 Key 1",
				Role:          "agent",
				RateLimitTier: "agent",
				IsActive:      true,
				DeveloperID:   developer1ID,
				CreatedAt:     time.Now(),
			},
			{
				ID:            key2ID,
				Prefix:        "sel_dev2",
				Name:          "Developer 1 Key 2",
				Role:          "agent",
				RateLimitTier: "agent",
				IsActive:      true,
				DeveloperID:   developer1ID,
				CreatedAt:     time.Now(),
			},
		}, nil)

		// Mock usage stats for each key
		mockRepo.On("GetAPIKeyUsageTotal", mock.Anything, key1ID, mock.Anything, mock.Anything).
			Return(int64(100), int64(5), nil)
		mockRepo.On("GetAPIKeyUsageTotal", mock.Anything, key2ID, mock.Anything, mock.Anything).
			Return(int64(200), int64(10), nil)

		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		handler := NewDeveloperAPIKeyHandler(
			service,
			zerolog.Nop(),
			"test",
			auditLogger,
		)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/dev/api-keys", nil)
		req = withDeveloperClaims(req, developer1ID, "dev1@example.com", "Developer 1")
		rec := httptest.NewRecorder()

		handler.ListAPIKeys(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp apiKeyListResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Len(t, resp.Items, 2)
		assert.Equal(t, 2, resp.KeyCount)
		assert.Equal(t, 5, resp.MaxKeys)

		// Verify all keys belong to developer 1
		for _, item := range resp.Items {
			assert.Contains(t, []string{key1ID.String(), key2ID.String()}, item.ID)
		}

		mockRepo.AssertExpectations(t)
	})

	t.Run("empty list for developer with no keys", func(t *testing.T) {
		mockRepo := new(MockDeveloperRepository)
		mockRepo.On("ListDeveloperAPIKeys", mock.Anything, developer2ID).Return([]developers.APIKey{}, nil)

		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		handler := NewDeveloperAPIKeyHandler(
			service,
			zerolog.Nop(),
			"test",
			auditLogger,
		)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/dev/api-keys", nil)
		req = withDeveloperClaims(req, developer2ID, "dev2@example.com", "Developer 2")
		rec := httptest.NewRecorder()

		handler.ListAPIKeys(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp apiKeyListResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Empty(t, resp.Items)
		assert.Equal(t, 0, resp.KeyCount)

		mockRepo.AssertExpectations(t)
	})
}

// TestDevKeyRevoke tests API key revocation
func TestDevKeyRevoke(t *testing.T) {
	ownerID := uuid.New()
	otherDevID := uuid.New()
	ownKeyID := uuid.New()
	otherKeyID := uuid.New()

	tests := []struct {
		name           string
		keyID          string
		developerID    uuid.UUID
		mockSetup      func(*MockDeveloperRepository)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:        "successfully revoke own key",
			keyID:       ownKeyID.String(),
			developerID: ownerID,
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetAPIKeyByID", mock.Anything, ownKeyID).Return(&developers.APIKey{
					ID:          ownKeyID,
					DeveloperID: ownerID,
					IsActive:    true,
				}, nil)
				m.On("DeactivateAPIKey", mock.Anything, ownKeyID).Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:        "cannot revoke others' keys",
			keyID:       otherKeyID.String(),
			developerID: ownerID,
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetAPIKeyByID", mock.Anything, otherKeyID).Return(&developers.APIKey{
					ID:          otherKeyID,
					DeveloperID: otherDevID,
					IsActive:    true,
				}, nil)
			},
			expectedStatus: http.StatusForbidden,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "You do not own this API key")
			},
		},
		{
			name:        "key not found",
			keyID:       uuid.New().String(),
			developerID: ownerID,
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetAPIKeyByID", mock.Anything, mock.Anything).
					Return(nil, developers.ErrAPIKeyNotFound)
			},
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "API key not found")
			},
		},
		{
			name:        "invalid key ID format",
			keyID:       "invalid-uuid",
			developerID: ownerID,
			mockSetup: func(m *MockDeveloperRepository) {
				// No mock setup - validation happens before service call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Invalid key ID")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockDeveloperRepository)
			tt.mockSetup(mockRepo)

			service := developers.NewService(mockRepo, zerolog.Nop())
			auditLogger := audit.NewLogger()
			handler := NewDeveloperAPIKeyHandler(
				service,
				zerolog.Nop(),
				"test",
				auditLogger,
			)

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/dev/api-keys/"+tt.keyID, nil)
			req.SetPathValue("id", tt.keyID)
			req = withDeveloperClaims(req, tt.developerID, "dev@example.com", "Test Developer")
			rec := httptest.NewRecorder()

			handler.RevokeAPIKey(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code, "Response body: %s", rec.Body.String())
			if tt.checkResponse != nil {
				tt.checkResponse(t, rec)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestDevUsageStats tests usage statistics retrieval
func TestDevUsageStats(t *testing.T) {
	developerID := uuid.New()
	keyID := uuid.New()

	t.Run("basic usage stats retrieval", func(t *testing.T) {
		mockRepo := new(MockDeveloperRepository)

		// Mock key ownership verification via ListOwnKeys -> ListDeveloperAPIKeys
		mockRepo.On("ListDeveloperAPIKeys", mock.Anything, developerID).Return([]developers.APIKey{
			{
				ID:          keyID,
				DeveloperID: developerID,
				Prefix:      "sel_dev",
				Name:        "Test Key",
			},
		}, nil)

		// Mock usage stats for each key (called by ListOwnKeys)
		mockRepo.On("GetAPIKeyUsageTotal", mock.Anything, keyID, mock.Anything, mock.Anything).
			Return(int64(1000), int64(50), nil)

		// Mock daily usage for the key (called by GetAPIKeyUsageStats)
		mockRepo.On("GetAPIKeyUsage", mock.Anything, keyID, mock.Anything, mock.Anything).
			Return([]developers.DailyUsage{
				{Date: time.Now().AddDate(0, 0, -1), Requests: 500, Errors: 25},
				{Date: time.Now(), Requests: 500, Errors: 25},
			}, nil)

		// Mock developer usage stats (called by GetUsageStats) - note: never actually called in this test
		// but included for completeness if the handler changes
		mockRepo.On("GetDeveloperUsageTotal", mock.Anything, developerID, mock.Anything, mock.Anything).
			Return(int64(1000), int64(50), nil).Maybe()

		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		handler := NewDeveloperAPIKeyHandler(
			service,
			zerolog.Nop(),
			"test",
			auditLogger,
		)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/dev/api-keys/"+keyID.String()+"/usage", nil)
		req.SetPathValue("id", keyID.String())
		req = withDeveloperClaims(req, developerID, "dev@example.com", "Test Developer")
		rec := httptest.NewRecorder()

		handler.GetAPIKeyUsage(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp usageStatsResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, keyID.String(), resp.APIKeyID)
		assert.Equal(t, int64(1000), resp.TotalRequests)
		assert.Equal(t, int64(50), resp.TotalErrors)
		assert.NotEmpty(t, resp.Daily)

		mockRepo.AssertExpectations(t)
	})

	t.Run("cannot view other developer's key usage", func(t *testing.T) {
		otherDevID := uuid.New()
		mockRepo := new(MockDeveloperRepository)

		// Mock key ownership - returns empty list (key doesn't belong to this developer)
		mockRepo.On("ListDeveloperAPIKeys", mock.Anything, otherDevID).Return([]developers.APIKey{}, nil)

		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		handler := NewDeveloperAPIKeyHandler(
			service,
			zerolog.Nop(),
			"test",
			auditLogger,
		)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/dev/api-keys/"+keyID.String()+"/usage", nil)
		req.SetPathValue("id", keyID.String())
		req = withDeveloperClaims(req, otherDevID, "other@example.com", "Other Developer")
		rec := httptest.NewRecorder()

		handler.GetAPIKeyUsage(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "You do not own this API key")

		mockRepo.AssertExpectations(t)
	})
}

// TestAdminDeveloperInvite tests that admins can invite developers
func TestAdminDeveloperInvite(t *testing.T) {
	adminID := uuid.New()

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		mockSetup      func(*MockDeveloperRepository)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "successful developer invitation",
			requestBody: map[string]interface{}{
				"email": "newdev@example.com",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				// Service calls CreateInvitation
				invitationID := uuid.New()
				m.On("CreateInvitation", mock.Anything, mock.MatchedBy(func(params developers.CreateInvitationDBParams) bool {
					return params.Email == "newdev@example.com"
				})).Return(&developers.DeveloperInvitation{
					ID:        invitationID,
					Email:     "newdev@example.com",
					ExpiresAt: time.Now().Add(168 * time.Hour),
				}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				assert.Equal(t, "newdev@example.com", resp["email"])
				assert.Equal(t, "invited", resp["status"])
				assert.NotEmpty(t, resp["invitation_url"])
				assert.NotEmpty(t, resp["invitation_expires_at"])
				assert.NotEmpty(t, resp["note"])
				// Verify raw token is NOT in the response (security fix)
				_, hasToken := resp["invitation_token"]
				assert.False(t, hasToken, "invitation_token should not be in response")
			},
		},
		{
			name: "email already taken",
			requestBody: map[string]interface{}{
				"email": "existing@example.com",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				// CreateInvitation fails with email already taken
				m.On("CreateInvitation", mock.Anything, mock.Anything).
					Return(nil, developers.ErrEmailTaken)
			},
			expectedStatus: http.StatusConflict,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Email already taken")
			},
		},
		{
			name:        "missing email",
			requestBody: map[string]interface{}{},
			mockSetup: func(m *MockDeveloperRepository) {
				// No mock setup - validation happens before service call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Email is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockDeveloperRepository)
			tt.mockSetup(mockRepo)

			service := developers.NewService(mockRepo, zerolog.Nop())
			auditLogger := audit.NewLogger()
			handler := NewAdminDevelopersHandler(service, mockRepo, auditLogger, "test")

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/developers/invite", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = withAdminClaims(req, adminID.String(), "admin")
			rec := httptest.NewRecorder()

			handler.InviteDeveloper(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code, "Response body: %s", rec.Body.String())
			if tt.checkResponse != nil {
				tt.checkResponse(t, rec)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestAdminDeveloperList tests that admins can list all developers
func TestAdminDeveloperList(t *testing.T) {
	adminID := uuid.New()
	dev1ID := uuid.New()
	dev2ID := uuid.New()

	t.Run("list all developers with pagination", func(t *testing.T) {
		mockRepo := new(MockDeveloperRepository)

		now := time.Now()
		lastLoginTime := now.Add(-24 * time.Hour)

		// Mock list developers - return []*developers.Developer
		mockRepo.On("ListDevelopers", mock.Anything, 50, 0).Return([]*developers.Developer{
			{
				ID:          dev1ID,
				Email:       "dev1@example.com",
				Name:        "Developer One",
				MaxKeys:     5,
				IsActive:    true,
				CreatedAt:   now.Add(-48 * time.Hour),
				LastLoginAt: &lastLoginTime,
			},
			{
				ID:          dev2ID,
				Email:       "dev2@example.com",
				Name:        "Developer Two",
				MaxKeys:     3,
				IsActive:    false,
				CreatedAt:   now.Add(-24 * time.Hour),
				LastLoginAt: nil,
			},
		}, nil)

		// Mock total count
		mockRepo.On("CountDevelopers", mock.Anything).Return(int64(2), nil)

		// Mock key counts
		mockRepo.On("CountDeveloperAPIKeys", mock.Anything, dev1ID).Return(int64(3), nil)
		mockRepo.On("CountDeveloperAPIKeys", mock.Anything, dev2ID).Return(int64(0), nil)

		// Mock usage stats
		mockRepo.On("GetDeveloperUsageTotal", mock.Anything, dev1ID, mock.Anything, mock.Anything).
			Return(int64(500), int64(5), nil)
		mockRepo.On("GetDeveloperUsageTotal", mock.Anything, dev2ID, mock.Anything, mock.Anything).
			Return(int64(0), int64(0), nil)

		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		handler := NewAdminDevelopersHandler(service, mockRepo, auditLogger, "test")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/developers", nil)
		req = withAdminClaims(req, adminID.String(), "admin")
		rec := httptest.NewRecorder()

		handler.ListDevelopers(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp ListDevelopersResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Len(t, resp.Items, 2)
		assert.Equal(t, int64(2), resp.Total)

		// Verify developer 1
		dev1 := resp.Items[0]
		assert.Equal(t, dev1ID.String(), dev1.ID)
		assert.Equal(t, "dev1@example.com", dev1.Email)
		assert.Equal(t, "active", dev1.Status)
		assert.True(t, dev1.IsActive)
		assert.Equal(t, 3, dev1.KeyCount)
		assert.Equal(t, int64(500), dev1.RequestsLast30d)

		// Verify developer 2
		dev2 := resp.Items[1]
		assert.Equal(t, dev2ID.String(), dev2.ID)
		assert.Equal(t, "dev2@example.com", dev2.Email)
		assert.Equal(t, "deactivated", dev2.Status)
		assert.False(t, dev2.IsActive)
		assert.Equal(t, 0, dev2.KeyCount)

		mockRepo.AssertExpectations(t)
	})

	t.Run("empty list when no developers", func(t *testing.T) {
		mockRepo := new(MockDeveloperRepository)

		mockRepo.On("ListDevelopers", mock.Anything, 50, 0).Return([]*developers.Developer{}, nil)
		mockRepo.On("CountDevelopers", mock.Anything).Return(int64(0), nil)

		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		handler := NewAdminDevelopersHandler(service, mockRepo, auditLogger, "test")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/developers", nil)
		req = withAdminClaims(req, adminID.String(), "admin")
		rec := httptest.NewRecorder()

		handler.ListDevelopers(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp ListDevelopersResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Empty(t, resp.Items)
		assert.Equal(t, int64(0), resp.Total)

		mockRepo.AssertExpectations(t)
	})
}

// TestAuthIsolation tests authentication isolation between developer and admin contexts
// NOTE: In production, middleware enforces these boundaries. Handlers assume middleware
// has already validated the correct auth type. These tests verify handler behavior when
// auth context is missing or incorrect (which should never happen in production).
func TestAuthIsolation(t *testing.T) {
	developerID := uuid.New()

	t.Run("developer endpoint requires developer claims in context", func(t *testing.T) {
		mockRepo := new(MockDeveloperRepository)
		// Successful case: proper developer claims
		mockRepo.On("ListDeveloperAPIKeys", mock.Anything, developerID).
			Return([]developers.APIKey{}, nil)

		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		devHandler := NewDeveloperAPIKeyHandler(service, zerolog.Nop(), "test", auditLogger)

		// Request WITH proper developer claims should succeed
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dev/api-keys", nil)
		req = withDeveloperClaims(req, developerID, "dev@example.com", "Test Developer")
		rec := httptest.NewRecorder()

		devHandler.ListAPIKeys(rec, req)

		// Should succeed with proper claims
		assert.Equal(t, http.StatusOK, rec.Code,
			"Developer endpoint should work with proper developer claims")

		mockRepo.AssertExpectations(t)
	})

	t.Run("developer endpoint fails without claims in context", func(t *testing.T) {
		mockRepo := new(MockDeveloperRepository)
		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		devHandler := NewDeveloperAPIKeyHandler(service, zerolog.Nop(), "test", auditLogger)

		// Request WITHOUT any claims (middleware would prevent this in production)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dev/api-keys", nil)
		rec := httptest.NewRecorder()

		devHandler.ListAPIKeys(rec, req)

		// Handler should fail when claims are missing
		// (In production, middleware ensures claims are present)
		assert.NotEqual(t, http.StatusOK, rec.Code,
			"Developer endpoint should fail without claims")
	})

	t.Run("admin endpoint requires repository access", func(t *testing.T) {
		adminID := uuid.New()
		mockRepo := new(MockDeveloperRepository)
		// Mock successful repository calls
		mockRepo.On("ListDevelopers", mock.Anything, 50, 0).
			Return([]*developers.Developer{}, nil)
		mockRepo.On("CountDevelopers", mock.Anything).Return(int64(0), nil)

		service := developers.NewService(mockRepo, zerolog.Nop())
		auditLogger := audit.NewLogger()
		adminHandler := NewAdminDevelopersHandler(service, mockRepo, auditLogger, "test")

		// Request WITH proper admin claims should succeed
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/developers", nil)
		req = withAdminClaims(req, adminID.String(), "admin")
		rec := httptest.NewRecorder()

		adminHandler.ListDevelopers(rec, req)

		// Should succeed with proper admin claims
		assert.Equal(t, http.StatusOK, rec.Code,
			"Admin endpoint should work with proper admin claims")

		mockRepo.AssertExpectations(t)
	})
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
