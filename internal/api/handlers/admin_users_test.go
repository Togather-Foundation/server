package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/middleware"
	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/Togather-Foundation/server/internal/domain/users"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockUserService is a mock implementation of the users.Service
type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) CreateUserAndInvite(ctx context.Context, params users.CreateUserParams) (postgres.User, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(postgres.User), args.Error(1)
}

func (m *MockUserService) ListUsers(ctx context.Context, filters users.ListUsersFilters) ([]postgres.ListUsersWithFiltersRow, int64, error) {
	args := m.Called(ctx, filters)
	return args.Get(0).([]postgres.ListUsersWithFiltersRow), args.Get(1).(int64), args.Error(2)
}

func (m *MockUserService) GetUser(ctx context.Context, id pgtype.UUID) (postgres.GetUserByIDRow, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(postgres.GetUserByIDRow), args.Error(1)
}

func (m *MockUserService) UpdateUser(ctx context.Context, id pgtype.UUID, params users.UpdateUserParams, updatedBy string) error {
	args := m.Called(ctx, id, params, updatedBy)
	return args.Error(0)
}

func (m *MockUserService) DeleteUser(ctx context.Context, id pgtype.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockUserService) DeactivateUser(ctx context.Context, id pgtype.UUID, deactivatedBy string) error {
	args := m.Called(ctx, id, deactivatedBy)
	return args.Error(0)
}

func (m *MockUserService) ActivateUser(ctx context.Context, id pgtype.UUID, activatedBy string) error {
	args := m.Called(ctx, id, activatedBy)
	return args.Error(0)
}

func (m *MockUserService) ResendInvitation(ctx context.Context, userID pgtype.UUID, resentBy string) error {
	args := m.Called(ctx, userID, resentBy)
	return args.Error(0)
}

// Helper to create a test UUID
func testUUID(s string) pgtype.UUID {
	var uuid pgtype.UUID
	_ = uuid.Scan(s)
	return uuid
}

// Helper to create test timestamps
func testTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// Helper to add admin claims to request context
func withAdminClaims(r *http.Request, userID, role string) *http.Request {
	claims := &auth.Claims{
		Role: role,
	}
	claims.Subject = userID
	ctx := middleware.ContextWithAdminClaims(r.Context(), claims)
	return r.WithContext(ctx)
}

func TestCreateUser_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock successful user creation
	adminID := testUUID("00000000-0000-0000-0000-000000000001")
	userID := testUUID("00000000-0000-0000-0000-000000000002")
	now := time.Now()

	mockService.On("CreateUserAndInvite", mock.Anything, mock.MatchedBy(func(params users.CreateUserParams) bool {
		return params.Username == "testuser" && params.Email == "test@example.com" && params.Role == "editor"
	})).Return(postgres.User{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Role:      "editor",
		IsActive:  false,
		CreatedAt: testTime(now),
	}, nil)

	// Create request
	reqBody := CreateUserRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "editor",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(body))
	req = withAdminClaims(req, formatUUID(adminID), "admin")

	// Record response
	rec := httptest.NewRecorder()

	// Call handler
	handler.CreateUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp AdminUserResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", resp.Username)
	assert.Equal(t, "test@example.com", resp.Email)
	assert.Equal(t, "editor", resp.Role)
	assert.False(t, resp.IsActive)

	mockService.AssertExpectations(t)
}

func TestCreateUser_EmailTaken(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock email conflict
	mockService.On("CreateUserAndInvite", mock.Anything, mock.Anything).Return(postgres.User{}, users.ErrEmailTaken)

	// Create request
	reqBody := CreateUserRequest{
		Username: "testuser",
		Email:    "existing@example.com",
		Role:     "viewer",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(body))

	// Record response
	rec := httptest.NewRecorder()

	// Call handler
	handler.CreateUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "Email already taken")

	mockService.AssertExpectations(t)
}

func TestCreateUser_ValidationError(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	tests := []struct {
		name        string
		reqBody     CreateUserRequest
		expectedMsg string
	}{
		{
			name:        "missing username",
			reqBody:     CreateUserRequest{Email: "test@example.com", Role: "viewer"},
			expectedMsg: "Username is required",
		},
		{
			name:        "missing email",
			reqBody:     CreateUserRequest{Username: "testuser", Role: "viewer"},
			expectedMsg: "Email is required",
		},
		{
			name:        "invalid role",
			reqBody:     CreateUserRequest{Username: "testuser", Email: "test@example.com", Role: "invalid"},
			expectedMsg: "Role must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			handler.CreateUser(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.expectedMsg)
		})
	}
}

func TestListUsers_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock user list
	now := time.Now()
	users := []postgres.ListUsersWithFiltersRow{
		{
			ID:        testUUID("00000000-0000-0000-0000-000000000001"),
			Username:  "user1",
			Email:     "user1@example.com",
			Role:      "admin",
			IsActive:  true,
			CreatedAt: testTime(now),
		},
		{
			ID:        testUUID("00000000-0000-0000-0000-000000000002"),
			Username:  "user2",
			Email:     "user2@example.com",
			Role:      "viewer",
			IsActive:  true,
			CreatedAt: testTime(now),
		},
	}

	mockService.On("ListUsers", mock.Anything, mock.Anything).Return(users, int64(2), nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	rec := httptest.NewRecorder()

	// Call handler
	handler.ListUsers(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ListUsersResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	assert.Equal(t, int64(2), resp.Total)
	assert.Nil(t, resp.NextCursor)

	mockService.AssertExpectations(t)
}

func TestListUsers_WithFilters(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock filtered results
	mockService.On("ListUsers", mock.Anything, mock.Anything).Return([]postgres.ListUsersWithFiltersRow{}, int64(0), nil)

	// Create request with filters
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?status=active&role=admin", nil)
	rec := httptest.NewRecorder()

	// Call handler
	handler.ListUsers(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)

	mockService.AssertExpectations(t)
}

func TestListUsers_Pagination(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock with pagination
	mockService.On("ListUsers", mock.Anything, mock.Anything).Return([]postgres.ListUsersWithFiltersRow{}, int64(100), nil)

	// Create request with limit
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?limit=10", nil)
	rec := httptest.NewRecorder()

	// Call handler
	handler.ListUsers(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ListUsersResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.NotNil(t, resp.NextCursor) // Should have next cursor since total > limit
	assert.Equal(t, "10", *resp.NextCursor)

	mockService.AssertExpectations(t)
}

func TestGetUser_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock user retrieval
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	now := time.Now()

	mockService.On("GetUser", mock.Anything, userID).Return(postgres.GetUserByIDRow{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Role:      "viewer",
		IsActive:  true,
		CreatedAt: testTime(now),
	}, nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+formatUUID(userID), nil)
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.GetUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AdminUserResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", resp.Username)
	assert.Equal(t, "test@example.com", resp.Email)

	mockService.AssertExpectations(t)
}

func TestGetUser_NotFound(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock user not found
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	mockService.On("GetUser", mock.Anything, userID).Return(postgres.GetUserByIDRow{}, users.ErrUserNotFound)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+formatUUID(userID), nil)
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.GetUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "User not found")

	mockService.AssertExpectations(t)
}

func TestUpdateUser_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock existing user and update
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	now := time.Now()

	existingUser := postgres.GetUserByIDRow{
		ID:        userID,
		Username:  "oldusername",
		Email:     "old@example.com",
		Role:      "viewer",
		IsActive:  true,
		CreatedAt: testTime(now),
	}

	updatedUser := postgres.GetUserByIDRow{
		ID:        userID,
		Username:  "newusername",
		Email:     "new@example.com",
		Role:      "editor",
		IsActive:  true,
		CreatedAt: testTime(now),
	}

	mockService.On("GetUser", mock.Anything, userID).Return(existingUser, nil).Once()
	mockService.On("UpdateUser", mock.Anything, userID, mock.Anything, mock.Anything).Return(nil)
	mockService.On("GetUser", mock.Anything, userID).Return(updatedUser, nil).Once()

	// Create request
	newUsername := "newusername"
	newEmail := "new@example.com"
	newRole := "editor"
	reqBody := UpdateUserRequest{
		Username: &newUsername,
		Email:    &newEmail,
		Role:     &newRole,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+formatUUID(userID), bytes.NewReader(body))
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.UpdateUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AdminUserResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, "newusername", resp.Username)
	assert.Equal(t, "new@example.com", resp.Email)
	assert.Equal(t, "editor", resp.Role)

	mockService.AssertExpectations(t)
}

func TestUpdateUser_EmailConflict(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock existing user and email conflict
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	now := time.Now()

	existingUser := postgres.GetUserByIDRow{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Role:      "viewer",
		IsActive:  true,
		CreatedAt: testTime(now),
	}

	mockService.On("GetUser", mock.Anything, userID).Return(existingUser, nil)
	mockService.On("UpdateUser", mock.Anything, userID, mock.Anything, mock.Anything).Return(users.ErrEmailTaken)

	// Create request
	newEmail := "taken@example.com"
	reqBody := UpdateUserRequest{
		Email: &newEmail,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+formatUUID(userID), bytes.NewReader(body))
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.UpdateUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "Email already taken")

	mockService.AssertExpectations(t)
}

func TestDeleteUser_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock user deletion
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	mockService.On("DeleteUser", mock.Anything, userID, mock.Anything).Return(nil)

	// Create request
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+formatUUID(userID), nil)
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.DeleteUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusNoContent, rec.Code)

	mockService.AssertExpectations(t)
}

func TestDeactivateUser_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock user deactivation
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	now := time.Now()

	mockService.On("DeactivateUser", mock.Anything, userID, mock.Anything).Return(nil)
	mockService.On("GetUser", mock.Anything, userID).Return(postgres.GetUserByIDRow{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Role:      "viewer",
		IsActive:  false, // Now inactive
		CreatedAt: testTime(now),
	}, nil)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+formatUUID(userID)+"/deactivate", nil)
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.DeactivateUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AdminUserResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.False(t, resp.IsActive)

	mockService.AssertExpectations(t)
}

func TestActivateUser_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock user activation
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	now := time.Now()

	mockService.On("ActivateUser", mock.Anything, userID, mock.Anything).Return(nil)
	mockService.On("GetUser", mock.Anything, userID).Return(postgres.GetUserByIDRow{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Role:      "viewer",
		IsActive:  true, // Now active
		CreatedAt: testTime(now),
	}, nil)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+formatUUID(userID)+"/activate", nil)
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.ActivateUser(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AdminUserResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.True(t, resp.IsActive)

	mockService.AssertExpectations(t)
}

func TestResendInvitation_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock invitation resend
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	mockService.On("ResendInvitation", mock.Anything, userID, mock.Anything).Return(nil)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+formatUUID(userID)+"/resend-invitation", nil)
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.ResendInvitation(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp MessageResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Contains(t, resp.Message, "Invitation email has been resent")

	mockService.AssertExpectations(t)
}

func TestGetUserActivity_Success(t *testing.T) {
	mockService := new(MockUserService)
	auditLogger := audit.NewLogger()
	handler := NewAdminUsersHandler(mockService, auditLogger, "test")

	// Mock user exists
	userID := testUUID("00000000-0000-0000-0000-000000000001")
	now := time.Now()
	mockService.On("GetUser", mock.Anything, userID).Return(postgres.GetUserByIDRow{
		ID:        userID,
		Username:  "testuser",
		Email:     "test@example.com",
		Role:      "viewer",
		IsActive:  true,
		CreatedAt: testTime(now),
	}, nil)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+formatUUID(userID)+"/activity", nil)
	req.SetPathValue("id", formatUUID(userID))
	rec := httptest.NewRecorder()

	// Call handler
	handler.GetUserActivity(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "items")

	mockService.AssertExpectations(t)
}

func TestMapUserError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedType   string
		expectedTitle  string
	}{
		{
			name:           "email taken",
			err:            users.ErrEmailTaken,
			expectedStatus: http.StatusConflict,
			expectedType:   "https://sel.events/problems/conflict",
			expectedTitle:  "Email already taken",
		},
		{
			name:           "username taken",
			err:            users.ErrUsernameTaken,
			expectedStatus: http.StatusConflict,
			expectedType:   "https://sel.events/problems/conflict",
			expectedTitle:  "Username already taken",
		},
		{
			name:           "user not found",
			err:            users.ErrUserNotFound,
			expectedStatus: http.StatusNotFound,
			expectedType:   "https://sel.events/problems/not-found",
			expectedTitle:  "User not found",
		},
		{
			name:           "invalid token",
			err:            users.ErrInvalidToken,
			expectedStatus: http.StatusBadRequest,
			expectedType:   "https://sel.events/problems/validation-error",
			expectedTitle:  "Invalid or expired invitation token",
		},
		{
			name:           "generic error",
			err:            errors.New("something went wrong"),
			expectedStatus: http.StatusInternalServerError,
			expectedType:   "https://sel.events/problems/server-error",
			expectedTitle:  "Server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, problemType, title := mapUserError(tt.err)
			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectedType, problemType)
			assert.Equal(t, tt.expectedTitle, title)
		})
	}
}

func TestFormatUUID(t *testing.T) {
	tests := []struct {
		name     string
		uuid     pgtype.UUID
		expected string
	}{
		{
			name:     "valid UUID",
			uuid:     testUUID("123e4567-e89b-12d3-a456-426614174000"),
			expected: "123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:     "invalid UUID",
			uuid:     pgtype.UUID{Valid: false},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatUUID(tt.uuid)
			if tt.expected != "" {
				// For valid UUIDs, check format is correct (8-4-4-4-12 hex characters)
				assert.Len(t, result, 36)
				assert.Contains(t, result, "-")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestTimePtr(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    pgtype.Timestamptz
		expected *time.Time
	}{
		{
			name:     "valid timestamp",
			input:    testTime(now),
			expected: &now,
		},
		{
			name:     "invalid timestamp",
			input:    pgtype.Timestamptz{Valid: false},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := timePtr(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.Unix(), result.Unix())
			}
		})
	}
}
