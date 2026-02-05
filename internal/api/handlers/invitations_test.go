package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/domain/users"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

// mockUserService is a mock implementation of the user service for testing
type mockUserService struct {
	acceptInvitationFunc func(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error)
}

func (m *mockUserService) AcceptInvitation(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error) {
	if m.acceptInvitationFunc != nil {
		return m.acceptInvitationFunc(ctx, token, password)
	}
	return postgres.GetUserByIDRow{}, nil
}

// mockAuditLogger is a mock audit logger for testing
type mockAuditLogger struct {
	loggedEntries []audit.Entry
}

func (m *mockAuditLogger) Log(entry audit.Entry) {
	m.loggedEntries = append(m.loggedEntries, entry)
}

func (m *mockAuditLogger) LogSuccess(action, adminUser, resourceType, resourceID, ipAddress string, details map[string]string) {
	m.Log(audit.Entry{
		Action:       action,
		AdminUser:    adminUser,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPAddress:    ipAddress,
		Status:       "success",
		Details:      details,
	})
}

func (m *mockAuditLogger) LogFailure(action, adminUser, ipAddress string, details map[string]string) {
	m.Log(audit.Entry{
		Action:    action,
		AdminUser: adminUser,
		IPAddress: ipAddress,
		Status:    "failure",
		Details:   details,
	})
}

func TestAcceptInvitation_Success(t *testing.T) {
	// Create mock user service that returns a successful user
	mockService := &mockUserService{
		acceptInvitationFunc: func(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error) {
			if token != "valid-token" {
				return postgres.GetUserByIDRow{}, users.ErrInvalidToken
			}
			// Return a mock user
			userID := pgtype.UUID{
				Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
				Valid: true,
			}
			return postgres.GetUserByIDRow{
				ID:       userID,
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "viewer",
			}, nil
		},
	}

	mockAudit := &mockAuditLogger{}
	handler := NewInvitationsHandler(mockService, mockAudit, "test")

	reqBody := `{"token": "valid-token", "password": "SecurePassword123!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.AcceptInvitation(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify response body
	var response AcceptInvitationResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Message == "" {
		t.Error("Expected non-empty message")
	}

	if response.User.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", response.User.Username)
	}

	if response.User.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", response.User.Email)
	}

	if response.User.Role != "viewer" {
		t.Errorf("Expected role 'viewer', got '%s'", response.User.Role)
	}
}

func TestAcceptInvitation_InvalidToken(t *testing.T) {
	mockService := &mockUserService{
		acceptInvitationFunc: func(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error) {
			return postgres.GetUserByIDRow{}, users.ErrInvalidToken
		},
	}

	mockAudit := &mockAuditLogger{}
	handler := NewInvitationsHandler(mockService, mockAudit, "test")

	reqBody := `{"token": "invalid-token", "password": "SecurePassword123!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.AcceptInvitation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	// Verify audit log was called
	if len(mockAudit.loggedEntries) != 1 {
		t.Errorf("Expected 1 audit log entry, got %d", len(mockAudit.loggedEntries))
	} else {
		entry := mockAudit.loggedEntries[0]
		if entry.Action != "user.invitation_accept_failed" {
			t.Errorf("Expected action 'user.invitation_accept_failed', got '%s'", entry.Action)
		}
		if entry.Status != "failure" {
			t.Errorf("Expected status 'failure', got '%s'", entry.Status)
		}
	}
}

func TestAcceptInvitation_ExpiredToken(t *testing.T) {
	mockService := &mockUserService{
		acceptInvitationFunc: func(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error) {
			return postgres.GetUserByIDRow{}, users.ErrInvalidToken
		},
	}

	mockAudit := &mockAuditLogger{}
	handler := NewInvitationsHandler(mockService, mockAudit, "test")

	reqBody := `{"token": "expired-token", "password": "SecurePassword123!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.AcceptInvitation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAcceptInvitation_WeakPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "password too short",
			password: "Short1!",
			wantErr:  users.ErrPasswordTooShort,
		},
		{
			name:     "password too weak",
			password: "NoSpecialChar123",
			wantErr:  users.ErrPasswordTooWeak,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockUserService{
				acceptInvitationFunc: func(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error) {
					return postgres.GetUserByIDRow{}, tt.wantErr
				},
			}

			mockAudit := &mockAuditLogger{}
			handler := NewInvitationsHandler(mockService, mockAudit, "test")

			reqBody := map[string]string{
				"token":    "valid-token",
				"password": tt.password,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			handler.AcceptInvitation(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}

			// Verify audit log
			if len(mockAudit.loggedEntries) != 1 {
				t.Errorf("Expected 1 audit log entry, got %d", len(mockAudit.loggedEntries))
			}
		})
	}
}

func TestAcceptInvitation_MissingToken(t *testing.T) {
	mockService := &mockUserService{}
	mockAudit := &mockAuditLogger{}
	handler := NewInvitationsHandler(mockService, mockAudit, "test")

	reqBody := `{"password": "SecurePassword123!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.AcceptInvitation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAcceptInvitation_MissingPassword(t *testing.T) {
	mockService := &mockUserService{}
	mockAudit := &mockAuditLogger{}
	handler := NewInvitationsHandler(mockService, mockAudit, "test")

	reqBody := `{"token": "valid-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.AcceptInvitation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAcceptInvitation_InvalidJSON(t *testing.T) {
	mockService := &mockUserService{}
	mockAudit := &mockAuditLogger{}
	handler := NewInvitationsHandler(mockService, mockAudit, "test")

	reqBody := `{invalid json`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.AcceptInvitation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAcceptInvitation_ServiceError(t *testing.T) {
	mockService := &mockUserService{
		acceptInvitationFunc: func(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error) {
			return postgres.GetUserByIDRow{}, errors.New("database connection error")
		},
	}

	mockAudit := &mockAuditLogger{}
	handler := NewInvitationsHandler(mockService, mockAudit, "test")

	reqBody := `{"token": "valid-token", "password": "SecurePassword123!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.AcceptInvitation(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	// Verify audit log
	if len(mockAudit.loggedEntries) != 1 {
		t.Errorf("Expected 1 audit log entry, got %d", len(mockAudit.loggedEntries))
	}
}

func TestAcceptInvitation_ClientIPExtraction(t *testing.T) {
	mockService := &mockUserService{
		acceptInvitationFunc: func(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error) {
			return postgres.GetUserByIDRow{}, users.ErrInvalidToken
		},
	}

	mockAudit := &mockAuditLogger{}
	handler := NewInvitationsHandler(mockService, mockAudit, "test")

	reqBody := `{"token": "invalid-token", "password": "SecurePassword123!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accept-invitation", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.1")

	w := httptest.NewRecorder()
	handler.AcceptInvitation(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	// Verify IP was extracted correctly
	if len(mockAudit.loggedEntries) != 1 {
		t.Errorf("Expected 1 audit log entry, got %d", len(mockAudit.loggedEntries))
	} else {
		entry := mockAudit.loggedEntries[0]
		if entry.IPAddress != "203.0.113.1" {
			t.Errorf("Expected IP '203.0.113.1', got '%s'", entry.IPAddress)
		}
	}
}
