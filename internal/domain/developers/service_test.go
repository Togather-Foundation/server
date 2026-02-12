package developers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

// mockRepository implements the Repository interface for testing
type mockRepository struct {
	// Function fields that tests can set
	createDeveloperFn           func(ctx context.Context, params CreateDeveloperDBParams) (*Developer, error)
	getDeveloperByIDFn          func(ctx context.Context, id uuid.UUID) (*Developer, error)
	getDeveloperByEmailFn       func(ctx context.Context, email string) (*Developer, error)
	getDeveloperByGitHubIDFn    func(ctx context.Context, githubID int64) (*Developer, error)
	listDevelopersFn            func(ctx context.Context, limit, offset int) ([]*Developer, error)
	updateDeveloperFn           func(ctx context.Context, id uuid.UUID, params UpdateDeveloperParams) (*Developer, error)
	updateDeveloperLastLoginFn  func(ctx context.Context, id uuid.UUID) error
	deactivateDeveloperFn       func(ctx context.Context, id uuid.UUID) error
	countDevelopersFn           func(ctx context.Context) (int64, error)
	createInvitationFn          func(ctx context.Context, params CreateInvitationDBParams) (*DeveloperInvitation, error)
	getInvitationByTokenHashFn  func(ctx context.Context, tokenHash string) (*DeveloperInvitation, error)
	acceptInvitationFn          func(ctx context.Context, id uuid.UUID) error
	listActiveInvitationsFn     func(ctx context.Context) ([]*DeveloperInvitation, error)
	listDeveloperAPIKeysFn      func(ctx context.Context, developerID uuid.UUID) ([]APIKey, error)
	countDeveloperAPIKeysFn     func(ctx context.Context, developerID uuid.UUID) (int64, error)
	createAPIKeyFn              func(ctx context.Context, params CreateAPIKeyDBParams) (*CreateAPIKeyResult, error)
	deactivateAPIKeyFn          func(ctx context.Context, id uuid.UUID) error
	getAPIKeyByIDFn             func(ctx context.Context, id uuid.UUID) (*APIKey, error)
	getAPIKeyUsageTotalFn       func(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error)
	getDeveloperUsageTotalFn    func(ctx context.Context, developerID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error)
	validateDeveloperPasswordFn func(ctx context.Context, id uuid.UUID, password string) (bool, error)
}

func (m *mockRepository) CreateDeveloper(ctx context.Context, params CreateDeveloperDBParams) (*Developer, error) {
	if m.createDeveloperFn != nil {
		return m.createDeveloperFn(ctx, params)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) GetDeveloperByID(ctx context.Context, id uuid.UUID) (*Developer, error) {
	if m.getDeveloperByIDFn != nil {
		return m.getDeveloperByIDFn(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) GetDeveloperByEmail(ctx context.Context, email string) (*Developer, error) {
	if m.getDeveloperByEmailFn != nil {
		return m.getDeveloperByEmailFn(ctx, email)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) GetDeveloperByGitHubID(ctx context.Context, githubID int64) (*Developer, error) {
	if m.getDeveloperByGitHubIDFn != nil {
		return m.getDeveloperByGitHubIDFn(ctx, githubID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) ListDevelopers(ctx context.Context, limit, offset int) ([]*Developer, error) {
	if m.listDevelopersFn != nil {
		return m.listDevelopersFn(ctx, limit, offset)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) UpdateDeveloper(ctx context.Context, id uuid.UUID, params UpdateDeveloperParams) (*Developer, error) {
	if m.updateDeveloperFn != nil {
		return m.updateDeveloperFn(ctx, id, params)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) UpdateDeveloperLastLogin(ctx context.Context, id uuid.UUID) error {
	if m.updateDeveloperLastLoginFn != nil {
		return m.updateDeveloperLastLoginFn(ctx, id)
	}
	return nil
}

func (m *mockRepository) DeactivateDeveloper(ctx context.Context, id uuid.UUID) error {
	if m.deactivateDeveloperFn != nil {
		return m.deactivateDeveloperFn(ctx, id)
	}
	return errors.New("not implemented")
}

func (m *mockRepository) CountDevelopers(ctx context.Context) (int64, error) {
	if m.countDevelopersFn != nil {
		return m.countDevelopersFn(ctx)
	}
	return 0, errors.New("not implemented")
}

func (m *mockRepository) CreateInvitation(ctx context.Context, params CreateInvitationDBParams) (*DeveloperInvitation, error) {
	if m.createInvitationFn != nil {
		return m.createInvitationFn(ctx, params)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*DeveloperInvitation, error) {
	if m.getInvitationByTokenHashFn != nil {
		return m.getInvitationByTokenHashFn(ctx, tokenHash)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) AcceptInvitation(ctx context.Context, id uuid.UUID) error {
	if m.acceptInvitationFn != nil {
		return m.acceptInvitationFn(ctx, id)
	}
	return nil
}

func (m *mockRepository) ListActiveInvitations(ctx context.Context) ([]*DeveloperInvitation, error) {
	if m.listActiveInvitationsFn != nil {
		return m.listActiveInvitationsFn(ctx)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) ListDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) ([]APIKey, error) {
	if m.listDeveloperAPIKeysFn != nil {
		return m.listDeveloperAPIKeysFn(ctx, developerID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) CountDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) (int64, error) {
	if m.countDeveloperAPIKeysFn != nil {
		return m.countDeveloperAPIKeysFn(ctx, developerID)
	}
	return 0, errors.New("not implemented")
}

func (m *mockRepository) CreateAPIKey(ctx context.Context, params CreateAPIKeyDBParams) (*CreateAPIKeyResult, error) {
	if m.createAPIKeyFn != nil {
		return m.createAPIKeyFn(ctx, params)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) DeactivateAPIKey(ctx context.Context, id uuid.UUID) error {
	if m.deactivateAPIKeyFn != nil {
		return m.deactivateAPIKeyFn(ctx, id)
	}
	return errors.New("not implemented")
}

func (m *mockRepository) GetAPIKeyByID(ctx context.Context, id uuid.UUID) (*APIKey, error) {
	if m.getAPIKeyByIDFn != nil {
		return m.getAPIKeyByIDFn(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) GetAPIKeyUsageTotal(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error) {
	if m.getAPIKeyUsageTotalFn != nil {
		return m.getAPIKeyUsageTotalFn(ctx, apiKeyID, startDate, endDate)
	}
	return 0, 0, errors.New("not implemented")
}

func (m *mockRepository) GetDeveloperUsageTotal(ctx context.Context, developerID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error) {
	if m.getDeveloperUsageTotalFn != nil {
		return m.getDeveloperUsageTotalFn(ctx, developerID, startDate, endDate)
	}
	return 0, 0, errors.New("not implemented")
}

func (m *mockRepository) ValidateDeveloperPassword(ctx context.Context, id uuid.UUID, password string) (bool, error) {
	if m.validateDeveloperPasswordFn != nil {
		return m.validateDeveloperPasswordFn(ctx, id, password)
	}
	return false, errors.New("not implemented")
}

// TestCreateDeveloper tests the CreateDeveloper service method
func TestCreateDeveloper(t *testing.T) {
	tests := []struct {
		name        string
		params      CreateDeveloperParams
		setupMock   func(*mockRepository)
		wantErr     bool
		expectedErr error
	}{
		{
			name: "success",
			params: CreateDeveloperParams{
				Email:    "test@example.com",
				Name:     "Test Developer",
				Password: "password123",
				MaxKeys:  5,
			},
			setupMock: func(m *mockRepository) {
				// No existing developer
				m.getDeveloperByEmailFn = func(ctx context.Context, email string) (*Developer, error) {
					return nil, errors.New("not found")
				}
				// Create succeeds
				m.createDeveloperFn = func(ctx context.Context, params CreateDeveloperDBParams) (*Developer, error) {
					return &Developer{
						ID:        uuid.New(),
						Email:     params.Email,
						Name:      params.Name,
						MaxKeys:   params.MaxKeys,
						IsActive:  true,
						CreatedAt: time.Now(),
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name: "duplicate email",
			params: CreateDeveloperParams{
				Email:    "existing@example.com",
				Name:     "Test Developer",
				Password: "password123",
				MaxKeys:  5,
			},
			setupMock: func(m *mockRepository) {
				// Existing developer found
				m.getDeveloperByEmailFn = func(ctx context.Context, email string) (*Developer, error) {
					return &Developer{
						ID:    uuid.New(),
						Email: email,
					}, nil
				}
			},
			wantErr:     true,
			expectedErr: ErrEmailTaken,
		},
		{
			name: "password too short",
			params: CreateDeveloperParams{
				Email:    "test@example.com",
				Name:     "Test Developer",
				Password: "short",
				MaxKeys:  5,
			},
			setupMock:   func(m *mockRepository) {},
			wantErr:     true,
			expectedErr: ErrPasswordTooShort,
		},
		{
			name: "password too long",
			params: CreateDeveloperParams{
				Email:    "test@example.com",
				Name:     "Test Developer",
				Password: strings.Repeat("a", 129),
				MaxKeys:  5,
			},
			setupMock:   func(m *mockRepository) {},
			wantErr:     true,
			expectedErr: ErrPasswordTooLong,
		},
		{
			name: "default max_keys",
			params: CreateDeveloperParams{
				Email:    "test@example.com",
				Name:     "Test Developer",
				Password: "password123",
				MaxKeys:  0, // Should default to 5
			},
			setupMock: func(m *mockRepository) {
				m.getDeveloperByEmailFn = func(ctx context.Context, email string) (*Developer, error) {
					return nil, errors.New("not found")
				}
				m.createDeveloperFn = func(ctx context.Context, params CreateDeveloperDBParams) (*Developer, error) {
					// Verify max_keys was set to default
					if params.MaxKeys != DefaultMaxKeys {
						t.Errorf("expected max_keys=%d, got %d", DefaultMaxKeys, params.MaxKeys)
					}
					return &Developer{
						ID:        uuid.New(),
						Email:     params.Email,
						Name:      params.Name,
						MaxKeys:   params.MaxKeys,
						IsActive:  true,
						CreatedAt: time.Now(),
					}, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepository{}
			tt.setupMock(mock)
			svc := NewService(mock, zerolog.Nop())

			dev, err := svc.CreateDeveloper(context.Background(), tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if dev == nil {
					t.Error("expected developer, got nil")
				}
			}
		})
	}
}

// TestAuthenticateDeveloper tests the AuthenticateDeveloper service method
func TestAuthenticateDeveloper(t *testing.T) {
	// Note: The current implementation doesn't actually validate the password
	// This is documented in service.go:227-228 as a TODO
	// These tests verify the current behavior
	tests := []struct {
		name        string
		email       string
		password    string
		setupMock   func(*mockRepository)
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "success - active developer",
			email:    "test@example.com",
			password: "password123",
			setupMock: func(m *mockRepository) {
				devID := uuid.New()
				m.getDeveloperByEmailFn = func(ctx context.Context, email string) (*Developer, error) {
					return &Developer{
						ID:        devID,
						Email:     email,
						IsActive:  true,
						CreatedAt: time.Now(),
					}, nil
				}
				m.validateDeveloperPasswordFn = func(ctx context.Context, id uuid.UUID, password string) (bool, error) {
					return true, nil
				}
				m.updateDeveloperLastLoginFn = func(ctx context.Context, id uuid.UUID) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "inactive developer",
			email:    "inactive@example.com",
			password: "password123",
			setupMock: func(m *mockRepository) {
				m.getDeveloperByEmailFn = func(ctx context.Context, email string) (*Developer, error) {
					return &Developer{
						ID:        uuid.New(),
						Email:     email,
						IsActive:  false,
						CreatedAt: time.Now(),
					}, nil
				}
			},
			wantErr:     true,
			expectedErr: ErrDeveloperInactive,
		},
		{
			name:     "non-existent email",
			email:    "notfound@example.com",
			password: "password123",
			setupMock: func(m *mockRepository) {
				m.getDeveloperByEmailFn = func(ctx context.Context, email string) (*Developer, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr:     true,
			expectedErr: ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepository{}
			tt.setupMock(mock)
			svc := NewService(mock, zerolog.Nop())

			dev, err := svc.AuthenticateDeveloper(context.Background(), tt.email, tt.password)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if dev == nil {
					t.Error("expected developer, got nil")
				}
			}
		})
	}
}

// TestCreateAPIKey tests the CreateAPIKey service method
func TestCreateAPIKey(t *testing.T) {
	devID := uuid.New()
	tests := []struct {
		name        string
		params      CreateAPIKeyParams
		setupMock   func(*mockRepository)
		wantErr     bool
		expectedErr error
		checkResult func(*testing.T, string, *APIKeyWithUsage)
	}{
		{
			name: "success",
			params: CreateAPIKeyParams{
				DeveloperID: devID,
				Name:        "Test Key",
			},
			setupMock: func(m *mockRepository) {
				m.getDeveloperByIDFn = func(ctx context.Context, id uuid.UUID) (*Developer, error) {
					return &Developer{
						ID:       id,
						MaxKeys:  5,
						IsActive: true,
					}, nil
				}
				m.countDeveloperAPIKeysFn = func(ctx context.Context, developerID uuid.UUID) (int64, error) {
					return 2, nil // Has 2 keys, under the limit
				}
				m.createAPIKeyFn = func(ctx context.Context, params CreateAPIKeyDBParams) (*CreateAPIKeyResult, error) {
					keyID := uuid.New()
					return &CreateAPIKeyResult{
						ID:            keyID,
						Prefix:        params.Prefix,
						Name:          params.Name,
						Role:          params.Role,
						RateLimitTier: params.RateLimitTier,
						CreatedAt:     time.Now(),
					}, nil
				}
			},
			wantErr: false,
			checkResult: func(t *testing.T, plainKey string, keyInfo *APIKeyWithUsage) {
				if plainKey == "" {
					t.Error("expected non-empty plain key")
				}
				if keyInfo == nil {
					t.Fatal("expected key info, got nil")
				}
				if keyInfo.Role != DeveloperAPIKeyRole {
					t.Errorf("expected role=%s, got %s", DeveloperAPIKeyRole, keyInfo.Role)
				}
				if keyInfo.RateLimitTier != DeveloperRateLimitTier {
					t.Errorf("expected rate_limit_tier=%s, got %s", DeveloperRateLimitTier, keyInfo.RateLimitTier)
				}
			},
		},
		{
			name: "max keys reached",
			params: CreateAPIKeyParams{
				DeveloperID: devID,
				Name:        "Test Key",
			},
			setupMock: func(m *mockRepository) {
				m.getDeveloperByIDFn = func(ctx context.Context, id uuid.UUID) (*Developer, error) {
					return &Developer{
						ID:       id,
						MaxKeys:  5,
						IsActive: true,
					}, nil
				}
				m.countDeveloperAPIKeysFn = func(ctx context.Context, developerID uuid.UUID) (int64, error) {
					return 5, nil // Already at limit
				}
			},
			wantErr:     true,
			expectedErr: ErrMaxKeysReached,
		},
		{
			name: "developer not found",
			params: CreateAPIKeyParams{
				DeveloperID: devID,
				Name:        "Test Key",
			},
			setupMock: func(m *mockRepository) {
				m.getDeveloperByIDFn = func(ctx context.Context, id uuid.UUID) (*Developer, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr:     true,
			expectedErr: ErrDeveloperNotFound,
		},
		{
			name: "always sets role to agent",
			params: CreateAPIKeyParams{
				DeveloperID: devID,
				Name:        "Test Key",
			},
			setupMock: func(m *mockRepository) {
				m.getDeveloperByIDFn = func(ctx context.Context, id uuid.UUID) (*Developer, error) {
					return &Developer{
						ID:       id,
						MaxKeys:  5,
						IsActive: true,
					}, nil
				}
				m.countDeveloperAPIKeysFn = func(ctx context.Context, developerID uuid.UUID) (int64, error) {
					return 0, nil
				}
				m.createAPIKeyFn = func(ctx context.Context, params CreateAPIKeyDBParams) (*CreateAPIKeyResult, error) {
					// Verify role is always "agent"
					if params.Role != DeveloperAPIKeyRole {
						t.Errorf("expected role=%s, got %s", DeveloperAPIKeyRole, params.Role)
					}
					keyID := uuid.New()
					return &CreateAPIKeyResult{
						ID:            keyID,
						Prefix:        params.Prefix,
						Name:          params.Name,
						Role:          params.Role,
						RateLimitTier: params.RateLimitTier,
						CreatedAt:     time.Now(),
					}, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepository{}
			tt.setupMock(mock)
			svc := NewService(mock, zerolog.Nop())

			plainKey, keyInfo, err := svc.CreateAPIKey(context.Background(), tt.params)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.checkResult != nil {
					tt.checkResult(t, plainKey, keyInfo)
				}
			}
		})
	}
}

// TestRevokeOwnKey tests the RevokeOwnKey service method
func TestRevokeOwnKey(t *testing.T) {
	devID := uuid.New()
	otherDevID := uuid.New()
	keyID := uuid.New()

	tests := []struct {
		name        string
		developerID uuid.UUID
		keyID       uuid.UUID
		setupMock   func(*mockRepository)
		wantErr     bool
		expectedErr error
	}{
		{
			name:        "success",
			developerID: devID,
			keyID:       keyID,
			setupMock: func(m *mockRepository) {
				m.getAPIKeyByIDFn = func(ctx context.Context, id uuid.UUID) (*APIKey, error) {
					return &APIKey{
						ID:          keyID,
						DeveloperID: devID,
						IsActive:    true,
					}, nil
				}
				m.deactivateAPIKeyFn = func(ctx context.Context, id uuid.UUID) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:        "wrong developer ID - unauthorized",
			developerID: otherDevID,
			keyID:       keyID,
			setupMock: func(m *mockRepository) {
				m.getAPIKeyByIDFn = func(ctx context.Context, id uuid.UUID) (*APIKey, error) {
					// Key belongs to devID, not otherDevID
					return &APIKey{
						ID:          keyID,
						DeveloperID: devID,
						IsActive:    true,
					}, nil
				}
			},
			wantErr:     true,
			expectedErr: ErrUnauthorized,
		},
		{
			name:        "key not found",
			developerID: devID,
			keyID:       keyID,
			setupMock: func(m *mockRepository) {
				m.getAPIKeyByIDFn = func(ctx context.Context, id uuid.UUID) (*APIKey, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr:     true,
			expectedErr: ErrAPIKeyNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepository{}
			tt.setupMock(mock)
			svc := NewService(mock, zerolog.Nop())

			err := svc.RevokeOwnKey(context.Background(), tt.developerID, tt.keyID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestInviteDeveloper tests the InviteDeveloper service method
func TestInviteDeveloper(t *testing.T) {
	inviterID := uuid.New()

	tests := []struct {
		name      string
		email     string
		invitedBy *uuid.UUID
		setupMock func(*mockRepository)
		wantErr   bool
	}{
		{
			name:      "success",
			email:     "newdev@example.com",
			invitedBy: &inviterID,
			setupMock: func(m *mockRepository) {
				m.createInvitationFn = func(ctx context.Context, params CreateInvitationDBParams) (*DeveloperInvitation, error) {
					return &DeveloperInvitation{
						ID:        uuid.New(),
						Email:     params.Email,
						TokenHash: params.TokenHash,
						InvitedBy: params.InvitedBy,
						ExpiresAt: params.ExpiresAt,
						CreatedAt: time.Now(),
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name:      "success without inviter",
			email:     "newdev@example.com",
			invitedBy: nil,
			setupMock: func(m *mockRepository) {
				m.createInvitationFn = func(ctx context.Context, params CreateInvitationDBParams) (*DeveloperInvitation, error) {
					if params.InvitedBy != nil {
						t.Error("expected InvitedBy to be nil")
					}
					return &DeveloperInvitation{
						ID:        uuid.New(),
						Email:     params.Email,
						TokenHash: params.TokenHash,
						ExpiresAt: params.ExpiresAt,
						CreatedAt: time.Now(),
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name:      "repository error",
			email:     "error@example.com",
			invitedBy: nil,
			setupMock: func(m *mockRepository) {
				m.createInvitationFn = func(ctx context.Context, params CreateInvitationDBParams) (*DeveloperInvitation, error) {
					return nil, errors.New("database error")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepository{}
			tt.setupMock(mock)
			svc := NewService(mock, zerolog.Nop())

			token, err := svc.InviteDeveloper(context.Background(), tt.email, tt.invitedBy)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if token == "" {
					t.Error("expected non-empty token")
				}
				// Token should be base64-encoded
				if len(token) < 43 {
					t.Errorf("token too short: %d chars", len(token))
				}
			}
		})
	}
}

// TestAcceptInvitation tests the AcceptInvitation service method
func TestAcceptInvitation(t *testing.T) {
	invitationID := uuid.New()
	validToken := "valid-token-abc123def456"
	tokenHash := hashToken(validToken)

	tests := []struct {
		name        string
		token       string
		devName     string
		password    string
		setupMock   func(*mockRepository)
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "success",
			token:    validToken,
			devName:  "New Developer",
			password: "password123",
			setupMock: func(m *mockRepository) {
				m.getInvitationByTokenHashFn = func(ctx context.Context, hash string) (*DeveloperInvitation, error) {
					return &DeveloperInvitation{
						ID:         invitationID,
						Email:      "newdev@example.com",
						TokenHash:  tokenHash,
						ExpiresAt:  time.Now().Add(24 * time.Hour),
						AcceptedAt: nil,
						CreatedAt:  time.Now(),
					}, nil
				}
				m.createDeveloperFn = func(ctx context.Context, params CreateDeveloperDBParams) (*Developer, error) {
					// Verify password was hashed
					if params.PasswordHash == nil {
						t.Error("expected password hash, got nil")
					} else {
						// Verify it's a bcrypt hash
						err := bcrypt.CompareHashAndPassword([]byte(*params.PasswordHash), []byte("password123"))
						if err != nil {
							t.Errorf("password hash verification failed: %v", err)
						}
					}
					return &Developer{
						ID:        uuid.New(),
						Email:     params.Email,
						Name:      params.Name,
						MaxKeys:   params.MaxKeys,
						IsActive:  true,
						CreatedAt: time.Now(),
					}, nil
				}
				m.acceptInvitationFn = func(ctx context.Context, id uuid.UUID) error {
					if id != invitationID {
						t.Errorf("wrong invitation ID: expected %s, got %s", invitationID, id)
					}
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "expired token",
			token:    validToken,
			devName:  "New Developer",
			password: "password123",
			setupMock: func(m *mockRepository) {
				m.getInvitationByTokenHashFn = func(ctx context.Context, hash string) (*DeveloperInvitation, error) {
					return &DeveloperInvitation{
						ID:         invitationID,
						Email:      "newdev@example.com",
						TokenHash:  tokenHash,
						ExpiresAt:  time.Now().Add(-24 * time.Hour), // Expired
						AcceptedAt: nil,
						CreatedAt:  time.Now().Add(-48 * time.Hour),
					}, nil
				}
			},
			wantErr:     true,
			expectedErr: ErrInvalidToken,
		},
		{
			name:     "already accepted invitation",
			token:    validToken,
			devName:  "New Developer",
			password: "password123",
			setupMock: func(m *mockRepository) {
				acceptedTime := time.Now().Add(-1 * time.Hour)
				m.getInvitationByTokenHashFn = func(ctx context.Context, hash string) (*DeveloperInvitation, error) {
					return &DeveloperInvitation{
						ID:         invitationID,
						Email:      "newdev@example.com",
						TokenHash:  tokenHash,
						ExpiresAt:  time.Now().Add(24 * time.Hour),
						AcceptedAt: &acceptedTime,
						CreatedAt:  time.Now().Add(-48 * time.Hour),
					}, nil
				}
			},
			wantErr:     true,
			expectedErr: ErrInvalidToken,
		},
		{
			name:     "weak password - too short",
			token:    validToken,
			devName:  "New Developer",
			password: "short",
			setupMock: func(m *mockRepository) {
				// Should fail before reaching the repo
			},
			wantErr:     true,
			expectedErr: ErrPasswordTooShort,
		},
		{
			name:     "weak password - too long",
			token:    validToken,
			devName:  "New Developer",
			password: strings.Repeat("a", 129),
			setupMock: func(m *mockRepository) {
				// Should fail before reaching the repo
			},
			wantErr:     true,
			expectedErr: ErrPasswordTooLong,
		},
		{
			name:     "invalid token",
			token:    "wrong-token",
			devName:  "New Developer",
			password: "password123",
			setupMock: func(m *mockRepository) {
				m.getInvitationByTokenHashFn = func(ctx context.Context, hash string) (*DeveloperInvitation, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr:     true,
			expectedErr: ErrInvalidToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepository{}
			tt.setupMock(mock)
			svc := NewService(mock, zerolog.Nop())

			dev, err := svc.AcceptInvitation(context.Background(), tt.token, tt.devName, tt.password)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if dev == nil {
					t.Error("expected developer, got nil")
				}
			}
		})
	}
}

// TestValidatePassword tests the validatePassword helper function
func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		wantErr     bool
		expectedErr error
	}{
		{
			name:     "valid - 8 chars",
			password: "password",
			wantErr:  false,
		},
		{
			name:     "valid - 128 chars",
			password: strings.Repeat("a", 128),
			wantErr:  false,
		},
		{
			name:     "valid - typical password",
			password: "MySecurePassword123!",
			wantErr:  false,
		},
		{
			name:        "too short - 7 chars",
			password:    "passwor",
			wantErr:     true,
			expectedErr: ErrPasswordTooShort,
		},
		{
			name:        "too short - empty",
			password:    "",
			wantErr:     true,
			expectedErr: ErrPasswordTooShort,
		},
		{
			name:        "too long - 129 chars",
			password:    strings.Repeat("a", 129),
			wantErr:     true,
			expectedErr: ErrPasswordTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected error %v, got %v", tt.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestGenerateSecureToken tests the generateSecureToken helper function
func TestGenerateSecureToken(t *testing.T) {
	// Generate multiple tokens and verify properties
	tokens := make(map[string]bool)
	iterations := 100

	for i := 0; i < iterations; i++ {
		token, err := generateSecureToken()
		if err != nil {
			t.Fatalf("generateSecureToken() failed: %v", err)
		}

		// Token should not be empty
		if token == "" {
			t.Error("generateSecureToken() returned empty token")
		}

		// Token should be unique
		if tokens[token] {
			t.Errorf("generateSecureToken() generated duplicate token after %d iterations", i)
		}
		tokens[token] = true

		// Token should be 43 or 44 characters (32 bytes base64-encoded)
		if len(token) < 43 || len(token) > 44 {
			t.Errorf("generateSecureToken() token length = %d, want 43-44", len(token))
		}
	}

	if len(tokens) != iterations {
		t.Errorf("generateSecureToken() uniqueness test failed: got %d unique tokens, want %d", len(tokens), iterations)
	}
}

// TestHashToken tests the hashToken helper function
func TestHashToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "simple token",
			token: "abc123",
		},
		{
			name:  "base64 token",
			token: "dGVzdC10b2tlbi0xMjM0NTY3ODkw",
		},
		{
			name:  "empty token",
			token: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := hashToken(tt.token)
			hash2 := hashToken(tt.token)

			// Same token should produce same hash (deterministic)
			if hash1 != hash2 {
				t.Errorf("hashToken() not deterministic: %s != %s", hash1, hash2)
			}

			// Hash should not be empty
			if hash1 == "" {
				t.Error("hashToken() returned empty hash")
			}

			// Hash should be different from input (unless input is empty)
			if tt.token != "" && hash1 == tt.token {
				t.Error("hashToken() returned input token (not hashed)")
			}
		})
	}

	// Different tokens should produce different hashes
	t.Run("different tokens produce different hashes", func(t *testing.T) {
		token1 := "token1"
		token2 := "token2"
		hash1 := hashToken(token1)
		hash2 := hashToken(token2)

		if hash1 == hash2 {
			t.Error("hashToken() produced same hash for different tokens")
		}
	})
}
