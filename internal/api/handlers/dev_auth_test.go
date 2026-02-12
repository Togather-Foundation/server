package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/domain/developers"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDeveloperRepository is a mock implementation of developers.Repository
type MockDeveloperRepository struct {
	mock.Mock
}

func (m *MockDeveloperRepository) CreateDeveloper(ctx context.Context, params developers.CreateDeveloperDBParams) (*developers.Developer, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.Developer), args.Error(1)
}

func (m *MockDeveloperRepository) GetDeveloperByID(ctx context.Context, id uuid.UUID) (*developers.Developer, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.Developer), args.Error(1)
}

func (m *MockDeveloperRepository) GetDeveloperByEmail(ctx context.Context, email string) (*developers.Developer, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.Developer), args.Error(1)
}

func (m *MockDeveloperRepository) GetDeveloperByGitHubID(ctx context.Context, githubID int64) (*developers.Developer, error) {
	args := m.Called(ctx, githubID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.Developer), args.Error(1)
}

func (m *MockDeveloperRepository) ListDevelopers(ctx context.Context, limit, offset int) ([]*developers.Developer, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*developers.Developer), args.Error(1)
}

func (m *MockDeveloperRepository) UpdateDeveloper(ctx context.Context, id uuid.UUID, params developers.UpdateDeveloperParams) (*developers.Developer, error) {
	args := m.Called(ctx, id, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.Developer), args.Error(1)
}

func (m *MockDeveloperRepository) UpdateDeveloperLastLogin(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDeveloperRepository) DeactivateDeveloper(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDeveloperRepository) CountDevelopers(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockDeveloperRepository) ValidateDeveloperPassword(ctx context.Context, id uuid.UUID, password string) (bool, error) {
	args := m.Called(ctx, id, password)
	return args.Bool(0), args.Error(1)
}

func (m *MockDeveloperRepository) CreateInvitation(ctx context.Context, params developers.CreateInvitationDBParams) (*developers.DeveloperInvitation, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.DeveloperInvitation), args.Error(1)
}

func (m *MockDeveloperRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*developers.DeveloperInvitation, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.DeveloperInvitation), args.Error(1)
}

func (m *MockDeveloperRepository) AcceptInvitation(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDeveloperRepository) ListActiveInvitations(ctx context.Context) ([]*developers.DeveloperInvitation, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*developers.DeveloperInvitation), args.Error(1)
}

func (m *MockDeveloperRepository) ListDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) ([]developers.APIKey, error) {
	args := m.Called(ctx, developerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]developers.APIKey), args.Error(1)
}

func (m *MockDeveloperRepository) CountDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) (int64, error) {
	args := m.Called(ctx, developerID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockDeveloperRepository) CreateAPIKey(ctx context.Context, params developers.CreateAPIKeyDBParams) (*developers.CreateAPIKeyResult, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.CreateAPIKeyResult), args.Error(1)
}

func (m *MockDeveloperRepository) DeactivateAPIKey(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDeveloperRepository) GetAPIKeyByID(ctx context.Context, id uuid.UUID) (*developers.APIKey, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*developers.APIKey), args.Error(1)
}

func (m *MockDeveloperRepository) GetAPIKeyUsageTotal(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error) {
	args := m.Called(ctx, apiKeyID, startDate, endDate)
	return args.Get(0).(int64), args.Get(1).(int64), args.Error(2)
}

func (m *MockDeveloperRepository) GetDeveloperUsageTotal(ctx context.Context, developerID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error) {
	args := m.Called(ctx, developerID, startDate, endDate)
	return args.Get(0).(int64), args.Get(1).(int64), args.Error(2)
}

func (m *MockDeveloperRepository) GetAPIKeyUsage(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate time.Time) ([]developers.DailyUsage, error) {
	args := m.Called(ctx, apiKeyID, startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]developers.DailyUsage), args.Error(1)
}

func (m *MockDeveloperRepository) BeginTx(ctx context.Context) (developers.Repository, developers.TxCommitter, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(developers.Repository), args.Get(1).(developers.TxCommitter), args.Error(2)
}

// Helper to add developer claims to request context
func withDeveloperClaims(r *http.Request, developerID uuid.UUID, email, name string) *http.Request {
	claims := &auth.DeveloperClaims{
		DeveloperID: developerID,
		Email:       email,
		Name:        name,
	}
	ctx := middleware.ContextWithDeveloper(r.Context(), claims)
	return r.WithContext(ctx)
}

// TestDevLogin tests successful and failed login scenarios
func TestDevLogin(t *testing.T) {
	devID := uuid.New()

	tests := []struct {
		name           string
		requestBody    devLoginRequest
		mockSetup      func(*MockDeveloperRepository)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "successful login",
			requestBody: devLoginRequest{
				Email:    "dev@example.com",
				Password: "password123",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				dev := &developers.Developer{
					ID:       devID,
					Email:    "dev@example.com",
					Name:     "Test Developer",
					IsActive: true,
				}
				m.On("GetDeveloperByEmail", mock.Anything, "dev@example.com").Return(dev, nil)
				m.On("ValidateDeveloperPassword", mock.Anything, devID, "password123").Return(true, nil)
				m.On("UpdateDeveloperLastLogin", mock.Anything, devID).Return(nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp devLoginResponse
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				assert.NotEmpty(t, resp.Token)
				assert.NotEmpty(t, resp.ExpiresAt)
				assert.Equal(t, "dev@example.com", resp.Developer.Email)
				assert.Equal(t, "Test Developer", resp.Developer.Name)

				// Check cookie was set
				cookies := rec.Result().Cookies()
				assert.NotEmpty(t, cookies)
				var foundCookie bool
				for _, cookie := range cookies {
					if cookie.Name == middleware.DevAuthCookieName {
						foundCookie = true
						assert.NotEmpty(t, cookie.Value)
						assert.True(t, cookie.HttpOnly)
						break
					}
				}
				assert.True(t, foundCookie, "dev_auth_token cookie should be set")
			},
		},
		{
			name: "wrong password",
			requestBody: devLoginRequest{
				Email:    "dev@example.com",
				Password: "wrongpassword",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				dev := &developers.Developer{
					ID:       devID,
					Email:    "dev@example.com",
					Name:     "Test Developer",
					IsActive: true,
				}
				m.On("GetDeveloperByEmail", mock.Anything, "dev@example.com").Return(dev, nil)
				m.On("ValidateDeveloperPassword", mock.Anything, devID, "wrongpassword").Return(false, nil)
			},
			expectedStatus: http.StatusUnauthorized,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Invalid credentials")
			},
		},
		{
			name: "nonexistent email",
			requestBody: devLoginRequest{
				Email:    "nonexistent@example.com",
				Password: "password123",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetDeveloperByEmail", mock.Anything, "nonexistent@example.com").
					Return(nil, developers.ErrDeveloperNotFound)
			},
			expectedStatus: http.StatusUnauthorized,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Invalid credentials")
			},
		},
		{
			name: "inactive developer",
			requestBody: devLoginRequest{
				Email:    "inactive@example.com",
				Password: "password123",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				dev := &developers.Developer{
					ID:       devID,
					Email:    "inactive@example.com",
					Name:     "Inactive Developer",
					IsActive: false,
				}
				m.On("GetDeveloperByEmail", mock.Anything, "inactive@example.com").Return(dev, nil)
				// When developer is inactive, the service checks IsActive before ValidateDeveloperPassword
			},
			expectedStatus: http.StatusForbidden,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Account is inactive")
			},
		},
		{
			name: "missing email",
			requestBody: devLoginRequest{
				Password: "password123",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				// No mock setup needed - validation happens before service call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Email and password are required")
			},
		},
		{
			name: "missing password",
			requestBody: devLoginRequest{
				Email: "dev@example.com",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				// No mock setup needed - validation happens before service call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Email and password are required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockDeveloperRepository)
			tt.mockSetup(mockRepo)

			service := developers.NewService(mockRepo, zerolog.Nop())
			auditLogger := audit.NewLogger()
			handler := NewDeveloperAuthHandler(
				service,
				zerolog.Nop(),
				"test-secret",
				24*time.Hour,
				"test-issuer",
				"test",
				auditLogger,
			)

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/dev/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.Login(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code, "Response body: %s", rec.Body.String())
			if tt.checkResponse != nil {
				tt.checkResponse(t, rec)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestDevAcceptInvitation tests invitation acceptance scenarios
func TestDevAcceptInvitation(t *testing.T) {
	invitationID := uuid.New()
	developerID := uuid.New()

	tests := []struct {
		name           string
		requestBody    acceptInvitationRequest
		mockSetup      func(*MockDeveloperRepository)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "successful invitation acceptance",
			requestBody: acceptInvitationRequest{
				Token:    "valid-token-123",
				Name:     "New Developer",
				Password: "SecurePass123!",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				invitation := &developers.DeveloperInvitation{
					ID:        invitationID,
					Email:     "newdev@example.com",
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				m.On("GetInvitationByTokenHash", mock.Anything, mock.Anything).Return(invitation, nil)
				m.On("AcceptInvitation", mock.Anything, invitationID).Return(nil)
				m.On("CreateDeveloper", mock.Anything, mock.MatchedBy(func(params developers.CreateDeveloperDBParams) bool {
					return params.Email == "newdev@example.com" && params.Name == "New Developer"
				})).Return(&developers.Developer{
					ID:    developerID,
					Email: "newdev@example.com",
					Name:  "New Developer",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp acceptInvitationResponse
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)
				assert.NotEmpty(t, resp.Token)
				assert.Equal(t, "newdev@example.com", resp.Developer.Email)
				assert.Equal(t, "New Developer", resp.Developer.Name)
			},
		},
		{
			name: "invalid token",
			requestBody: acceptInvitationRequest{
				Token:    "invalid-token",
				Name:     "New Developer",
				Password: "SecurePass123!",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				m.On("GetInvitationByTokenHash", mock.Anything, mock.Anything).
					Return(nil, developers.ErrInvalidToken)
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Invalid or expired invitation")
			},
		},
		{
			name: "weak password - too short",
			requestBody: acceptInvitationRequest{
				Token:    "valid-token",
				Name:     "New Developer",
				Password: "Short1!",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				// Password validation happens in service before DB call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Password does not meet requirements")
			},
		},
		{
			name: "missing token",
			requestBody: acceptInvitationRequest{
				Name:     "New Developer",
				Password: "SecurePass123!",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				// No mock setup - validation happens before service call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Token is required")
			},
		},
		{
			name: "missing name",
			requestBody: acceptInvitationRequest{
				Token:    "valid-token",
				Password: "SecurePass123!",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				// No mock setup - validation happens before service call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Name is required")
			},
		},
		{
			name: "missing password",
			requestBody: acceptInvitationRequest{
				Token: "valid-token",
				Name:  "New Developer",
			},
			mockSetup: func(m *MockDeveloperRepository) {
				// No mock setup - validation happens before service call
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Contains(t, rec.Body.String(), "Password is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockDeveloperRepository)
			tt.mockSetup(mockRepo)

			service := developers.NewService(mockRepo, zerolog.Nop())
			auditLogger := audit.NewLogger()
			handler := NewDeveloperAuthHandler(
				service,
				zerolog.Nop(),
				"test-secret",
				24*time.Hour,
				"test-issuer",
				"test",
				auditLogger,
			)

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/dev/accept-invitation", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.AcceptInvitation(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code, "Response body: %s", rec.Body.String())
			if tt.checkResponse != nil {
				tt.checkResponse(t, rec)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// TestDevLogout tests logout functionality
func TestDevLogout(t *testing.T) {
	mockRepo := new(MockDeveloperRepository)
	service := developers.NewService(mockRepo, zerolog.Nop())
	auditLogger := audit.NewLogger()
	handler := NewDeveloperAuthHandler(
		service,
		zerolog.Nop(),
		"test-secret",
		24*time.Hour,
		"test-issuer",
		"test",
		auditLogger,
	)

	developerID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dev/logout", nil)
	req = withDeveloperClaims(req, developerID, "dev@example.com", "Test Developer")
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Check cookie was cleared
	cookies := rec.Result().Cookies()
	assert.NotEmpty(t, cookies)
	var foundCookie bool
	for _, cookie := range cookies {
		if cookie.Name == middleware.DevAuthCookieName {
			foundCookie = true
			assert.Equal(t, "", cookie.Value)
			assert.Equal(t, -1, cookie.MaxAge)
			break
		}
	}
	assert.True(t, foundCookie, "dev_auth_token cookie should be cleared")
}
