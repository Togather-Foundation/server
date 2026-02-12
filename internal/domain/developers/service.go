// Package developers provides developer account management functionality including
// developer creation, authentication, invitation flows, and API key management.
// All developer API keys are automatically assigned the "agent" role.
//
// The package follows an invitation-based signup flow where developers receive an
// email invitation with a secure token to set their password and activate their account.
//
// Core operations include:
//   - CreateDeveloper: Creates developer account with password hashing
//   - AuthenticateDeveloper: Email/password authentication with bcrypt verification
//   - CreateAPIKey: Creates API keys with max_keys limit enforcement (always role=agent)
//   - ListOwnKeys: Lists a developer's API keys with usage statistics
//   - RevokeOwnKey: Revokes an API key after verifying ownership
//   - GetUsageStats: Retrieves aggregated usage statistics for a developer
//   - InviteDeveloper: Generates and sends developer invitation
//   - AcceptInvitation: Validates token, sets password, activates account
//
// All operations that modify developer state are designed for audit logging integration.
package developers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

// Domain-specific errors for developer operations.
var (
	// ErrDeveloperNotFound is returned when a developer lookup fails.
	ErrDeveloperNotFound = errors.New("developer not found")

	// ErrInvalidCredentials is returned when email/password authentication fails.
	ErrInvalidCredentials = errors.New("invalid email or password")

	// ErrDeveloperInactive is returned when trying to authenticate an inactive developer.
	ErrDeveloperInactive = errors.New("developer account is inactive")

	// ErrMaxKeysReached is returned when a developer has reached their max_keys limit.
	ErrMaxKeysReached = errors.New("maximum number of API keys reached")

	// ErrAPIKeyNotFound is returned when an API key lookup fails.
	ErrAPIKeyNotFound = errors.New("api key not found")

	// ErrUnauthorized is returned when a developer tries to access a key they don't own.
	ErrUnauthorized = errors.New("unauthorized to access this API key")

	// ErrInvalidToken is returned when an invitation token is invalid, expired, or already used.
	ErrInvalidToken = errors.New("invalid or expired invitation token")

	// ErrEmailTaken is returned when trying to create a developer with an existing email.
	ErrEmailTaken = errors.New("email is already taken")

	// ErrPasswordTooShort is returned when a password is less than 8 characters.
	// Also returned when trying to create a non-OAuth developer without a password.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")

	// ErrPasswordTooLong is returned when a password exceeds 128 characters.
	ErrPasswordTooLong = errors.New("password must not exceed 128 characters")
)

const (
	// DefaultInvitationExpiry is the default time until an invitation expires
	DefaultInvitationExpiry = 168 * time.Hour // 7 days

	// DefaultMaxKeys is the default maximum number of API keys per developer
	DefaultMaxKeys = 5

	// BcryptCost is the cost factor for bcrypt password hashing
	BcryptCost = 12

	// DeveloperAPIKeyRole is the role assigned to all developer API keys
	DeveloperAPIKeyRole = "agent"

	// DeveloperRateLimitTier is the default rate limit tier for developer keys
	DeveloperRateLimitTier = "agent"
)

// Service handles developer account management operations including creation,
// authentication, API key management, and invitation flows.
//
// Developer API keys are always created with role="agent" and are subject to
// a per-developer limit (max_keys, default 5). Developers can only manage their
// own API keys.
//
// Passwords use bcrypt hashing with cost factor 12. Minimum password length is
// 8 characters (less strict than user accounts, which require 12 characters).
type Service struct {
	repo      Repository
	logger    zerolog.Logger
	validator *validator.Validate
}

// NewService creates and initializes a new developer service instance.
//
// Parameters:
//   - repo: Repository implementation for data access
//   - logger: Structured logger for service-level logging
//
// Returns a fully initialized Service ready to handle developer operations.
func NewService(repo Repository, logger zerolog.Logger) *Service {
	return &Service{
		repo:      repo,
		logger:    logger.With().Str("component", "developers").Logger(),
		validator: validator.New(),
	}
}

// CreateDeveloper creates a new developer account with email/password authentication
// or OAuth-only authentication (GitHub).
//
// Password handling:
//   - If params.Password is empty AND params.GitHubID is set: OAuth-only account (no password)
//   - If params.Password is empty AND params.GitHubID is nil: Error (invitation-based devs need password)
//   - If params.Password is not empty: Validate and hash the password
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - params: Developer creation parameters (email, name, password, optional GitHub info)
//
// Returns the created developer or an error. Possible errors:
//   - ErrEmailTaken: Email already exists in the system
//   - ErrPasswordTooShort/TooLong: Password doesn't meet length requirements
//   - Validation errors: Invalid email format or missing required fields
//
// Side effects:
//   - Creates developer record in database with hashed password (or NULL for OAuth-only)
//   - Sets is_active to true (account is immediately active)
func (s *Service) CreateDeveloper(ctx context.Context, params CreateDeveloperParams) (*Developer, error) {
	// Set default max_keys if not provided
	if params.MaxKeys <= 0 {
		params.MaxKeys = DefaultMaxKeys
	}

	// Check if email is already taken
	existingDev, err := s.repo.GetDeveloperByEmail(ctx, params.Email)
	if err == nil && existingDev != nil {
		return nil, ErrEmailTaken
	}

	// Handle password validation and hashing
	var passwordHashPtr *string
	if params.Password == "" {
		// Empty password: only allowed for OAuth-only accounts
		if params.GitHubID == nil {
			// No password and no GitHub ID = error (invitation-based devs need a password)
			return nil, ErrPasswordTooShort
		}
		// OAuth-only account: store NULL password hash
		passwordHashPtr = nil
	} else {
		// Non-empty password: validate and hash
		if err := validatePassword(params.Password); err != nil {
			return nil, err
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(params.Password), BcryptCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		passwordHashStr := string(hashedPassword)
		passwordHashPtr = &passwordHashStr
	}

	// Create developer
	dbParams := CreateDeveloperDBParams{
		Email:          params.Email,
		Name:           params.Name,
		PasswordHash:   passwordHashPtr,
		GitHubID:       params.GitHubID,
		GitHubUsername: params.GitHubUsername,
		MaxKeys:        params.MaxKeys,
	}

	developer, err := s.repo.CreateDeveloper(ctx, dbParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create developer: %w", err)
	}

	s.logger.Info().
		Str("developer_id", developer.ID.String()).
		Str("email", developer.Email).
		Bool("oauth_only", passwordHashPtr == nil).
		Msg("developer account created")

	return developer, nil
}

// AuthenticateDeveloper validates email and password credentials and returns the
// authenticated developer. The password is verified using bcrypt comparison.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - email: Developer's email address
//   - password: Plaintext password to verify
//
// Returns the authenticated developer or an error. Possible errors:
//   - ErrInvalidCredentials: Email not found or password doesn't match
//   - ErrDeveloperInactive: Developer account is deactivated
//
// Side effects:
//   - Updates last_login_at timestamp in database
func (s *Service) AuthenticateDeveloper(ctx context.Context, email, password string) (*Developer, error) {
	// Look up developer by email
	developer, err := s.repo.GetDeveloperByEmail(ctx, email)
	if err != nil {
		// Don't reveal whether the email exists
		return nil, ErrInvalidCredentials
	}

	// Check if developer is active
	if !developer.IsActive {
		return nil, ErrDeveloperInactive
	}

	// Validate password using repository layer
	valid, err := s.repo.ValidateDeveloperPassword(ctx, developer.ID, password)
	if err != nil {
		s.logger.Warn().Err(err).Str("developer_id", developer.ID.String()).Msg("password validation error")
		return nil, ErrInvalidCredentials
	}
	if !valid {
		return nil, ErrInvalidCredentials
	}

	// Update last login
	if err := s.repo.UpdateDeveloperLastLogin(ctx, developer.ID); err != nil {
		s.logger.Warn().Err(err).Str("developer_id", developer.ID.String()).Msg("failed to update last login")
		// Don't fail authentication if we can't update last_login
	}

	s.logger.Info().
		Str("developer_id", developer.ID.String()).
		Str("email", email).
		Msg("developer authenticated")

	return developer, nil
}

// CreateAPIKey creates a new API key for a developer. The key is generated
// with cryptographically secure randomness and hashed with bcrypt before storage.
//
// All developer API keys are assigned role="agent" regardless of input.
// Enforces the max_keys limit per developer (default 5).
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - params: API key creation parameters (developer_id, name, optional expiration)
//
// Returns the plaintext API key (shown only once) and key metadata, or an error.
// Possible errors:
//   - ErrMaxKeysReached: Developer has reached their max_keys limit
//   - ErrDeveloperNotFound: Developer ID is invalid
//
// Side effects:
//   - Creates API key record in database with bcrypt-hashed key
//   - Sets role="agent" and rate_limit_tier="agent"
//   - Sets developer_id for ownership tracking
func (s *Service) CreateAPIKey(ctx context.Context, params CreateAPIKeyParams) (plainKey string, keyInfo *APIKeyWithUsage, err error) {
	// Check if developer exists and get their max_keys limit
	developer, err := s.repo.GetDeveloperByID(ctx, params.DeveloperID)
	if err != nil {
		return "", nil, ErrDeveloperNotFound
	}

	// Check current key count
	count, err := s.repo.CountDeveloperAPIKeys(ctx, params.DeveloperID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to count developer keys: %w", err)
	}

	if count >= int64(developer.MaxKeys) {
		return "", nil, ErrMaxKeysReached
	}

	// Generate API key (32 bytes = 256 bits)
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	plainKey = base64.URLEncoding.EncodeToString(keyBytes)
	prefix := plainKey[:8]

	// Hash the key using bcrypt
	keyHash, err := auth.HashAPIKey(plainKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to hash key: %w", err)
	}

	// Calculate expiration
	var expiresAt *time.Time
	if params.ExpiresInDays != nil && *params.ExpiresInDays > 0 {
		exp := time.Now().AddDate(0, 0, *params.ExpiresInDays)
		expiresAt = &exp
	}

	// Create API key in database
	dbParams := CreateAPIKeyDBParams{
		Prefix:        prefix,
		KeyHash:       keyHash,
		HashVersion:   auth.HashVersionBcrypt,
		Name:          params.Name,
		DeveloperID:   params.DeveloperID,
		Role:          DeveloperAPIKeyRole, // Always "agent" for developer keys
		RateLimitTier: DeveloperRateLimitTier,
		IsActive:      true,
		ExpiresAt:     expiresAt,
	}

	createdKey, err := s.repo.CreateAPIKey(ctx, dbParams)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create api key: %w", err)
	}

	// Convert to APIKeyWithUsage (no usage yet for a new key)
	keyInfo = &APIKeyWithUsage{
		ID:            createdKey.ID,
		Prefix:        prefix,
		Name:          createdKey.Name,
		Role:          createdKey.Role,
		RateLimitTier: createdKey.RateLimitTier,
		IsActive:      true,
		CreatedAt:     createdKey.CreatedAt,
		LastUsedAt:    nil,
		ExpiresAt:     expiresAt,
		UsageToday:    0,
		Usage7d:       0,
		Usage30d:      0,
	}

	s.logger.Info().
		Str("developer_id", params.DeveloperID.String()).
		Str("key_id", keyInfo.ID.String()).
		Str("key_name", params.Name).
		Msg("api key created")

	return plainKey, keyInfo, nil
}

// ListOwnKeys retrieves all API keys owned by a developer along with their
// usage statistics for the past 1, 7, and 30 days.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - developerID: UUID of the developer whose keys to list
//
// Returns a list of API keys with usage statistics or an error.
func (s *Service) ListOwnKeys(ctx context.Context, developerID uuid.UUID) ([]*APIKeyWithUsage, error) {
	// Get all keys for this developer
	keys, err := s.repo.ListDeveloperAPIKeys(ctx, developerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list developer keys: %w", err)
	}

	result := make([]*APIKeyWithUsage, 0, len(keys))
	now := time.Now()

	for _, key := range keys {
		// Get usage statistics
		usage1d, _, err := s.repo.GetAPIKeyUsageTotal(ctx, key.ID, now.AddDate(0, 0, -1), now)
		if err != nil {
			s.logger.Warn().Err(err).Str("key_id", key.ID.String()).Msg("failed to get 1d usage")
			usage1d = 0
		}

		usage7d, _, err := s.repo.GetAPIKeyUsageTotal(ctx, key.ID, now.AddDate(0, 0, -7), now)
		if err != nil {
			s.logger.Warn().Err(err).Str("key_id", key.ID.String()).Msg("failed to get 7d usage")
			usage7d = 0
		}

		usage30d, _, err := s.repo.GetAPIKeyUsageTotal(ctx, key.ID, now.AddDate(0, 0, -30), now)
		if err != nil {
			s.logger.Warn().Err(err).Str("key_id", key.ID.String()).Msg("failed to get 30d usage")
			usage30d = 0
		}

		result = append(result, &APIKeyWithUsage{
			ID:            key.ID,
			Prefix:        key.Prefix,
			Name:          key.Name,
			Role:          key.Role,
			RateLimitTier: key.RateLimitTier,
			IsActive:      key.IsActive,
			CreatedAt:     key.CreatedAt,
			LastUsedAt:    key.LastUsedAt,
			ExpiresAt:     key.ExpiresAt,
			UsageToday:    usage1d,
			Usage7d:       usage7d,
			Usage30d:      usage30d,
		})
	}

	return result, nil
}

// RevokeOwnKey deactivates an API key after verifying that it belongs to the
// specified developer. This prevents developers from revoking keys they don't own.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - developerID: UUID of the developer requesting revocation
//   - keyID: UUID of the API key to revoke
//
// Returns nil on success or an error. Possible errors:
//   - ErrAPIKeyNotFound: Key ID doesn't exist
//   - ErrUnauthorized: Key doesn't belong to this developer
//
// Side effects:
//   - Sets is_active=false for the API key
func (s *Service) RevokeOwnKey(ctx context.Context, developerID uuid.UUID, keyID uuid.UUID) error {
	// Get the API key to verify ownership
	key, err := s.repo.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		return ErrAPIKeyNotFound
	}

	// Verify the key belongs to this developer
	if key.DeveloperID != developerID {
		s.logger.Warn().
			Str("developer_id", developerID.String()).
			Str("key_id", keyID.String()).
			Str("key_owner_id", key.DeveloperID.String()).
			Msg("unauthorized key revocation attempt")
		return ErrUnauthorized
	}

	// Deactivate the key
	if err := s.repo.DeactivateAPIKey(ctx, keyID); err != nil {
		return fmt.Errorf("failed to deactivate key: %w", err)
	}

	s.logger.Info().
		Str("developer_id", developerID.String()).
		Str("key_id", keyID.String()).
		Msg("api key revoked")

	return nil
}

// GetUsageStats retrieves aggregated usage statistics across all of a developer's
// API keys for a specified date range.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - developerID: UUID of the developer
//   - startDate: Start of the date range (inclusive)
//   - endDate: End of the date range (inclusive)
//
// Returns usage statistics or an error.
func (s *Service) GetUsageStats(ctx context.Context, developerID uuid.UUID, startDate, endDate time.Time) (*UsageStats, error) {
	totalRequests, totalErrors, err := s.repo.GetDeveloperUsageTotal(ctx, developerID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get developer usage: %w", err)
	}

	return &UsageStats{
		DeveloperID:   developerID,
		TotalRequests: totalRequests,
		TotalErrors:   totalErrors,
		StartDate:     startDate,
		EndDate:       endDate,
	}, nil
}

// GetAPIKeyUsageStats retrieves usage statistics for a specific API key including
// daily breakdown. This is for per-key usage tracking.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - keyID: UUID of the API key
//   - startDate: Start of the date range (inclusive)
//   - endDate: End of the date range (inclusive)
//
// Returns:
//   - totalRequests: Total request count across the date range
//   - totalErrors: Total error count across the date range
//   - daily: Array of daily usage records
//   - error: Any error that occurred
func (s *Service) GetAPIKeyUsageStats(ctx context.Context, keyID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, daily []DailyUsage, err error) {
	// Get total usage for the key
	totalRequests, totalErrors, err = s.repo.GetAPIKeyUsageTotal(ctx, keyID, startDate, endDate)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to get api key usage total: %w", err)
	}

	// Get daily breakdown
	daily, err = s.repo.GetAPIKeyUsage(ctx, keyID, startDate, endDate)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to get api key usage daily: %w", err)
	}

	return totalRequests, totalErrors, daily, nil
}

// InviteDeveloper generates a secure invitation token, hashes it with SHA-256,
// and stores the invitation record. Returns the plaintext token for sending via email.
//
// The token is 32 random bytes encoded as URL-safe base64. It is single-use and
// expires after 7 days.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - email: Email address to invite
//   - invitedBy: UUID of the admin/developer who created the invitation (optional)
//
// Returns the plaintext invitation token (for email) or an error.
//
// Side effects:
//   - Creates invitation record with hashed token
//   - Token expires in 7 days
func (s *Service) InviteDeveloper(ctx context.Context, email string, invitedBy *uuid.UUID) (string, error) {
	// Generate secure token (32 bytes)
	token, err := generateSecureToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash the token with SHA-256
	tokenHash := hashToken(token)

	// Create invitation
	expiresAt := time.Now().Add(DefaultInvitationExpiry)
	dbParams := CreateInvitationDBParams{
		Email:     email,
		TokenHash: tokenHash,
		InvitedBy: invitedBy,
		ExpiresAt: expiresAt,
	}

	invitation, err := s.repo.CreateInvitation(ctx, dbParams)
	if err != nil {
		return "", fmt.Errorf("failed to create invitation: %w", err)
	}

	s.logger.Info().
		Str("invitation_id", invitation.ID.String()).
		Str("email", email).
		Msg("developer invitation created")

	return token, nil
}

// AcceptInvitation validates an invitation token, creates a developer account
// with the provided password, and marks the invitation as accepted.
//
// The token is validated against its SHA-256 hash. Tokens are single-use and
// expire after 7 days. Password must be at least 8 characters.
//
// This operation is atomic: both developer creation and invitation acceptance
// happen within a single database transaction. If either operation fails, both
// are rolled back to prevent partial state.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - token: Plaintext invitation token from the email
//   - name: Developer's name
//   - password: Developer's chosen password (plaintext, will be hashed)
//
// Returns the created developer or an error. Possible errors:
//   - ErrInvalidToken: Token doesn't exist, expired, or already used
//   - ErrPasswordTooShort/TooLong: Password doesn't meet requirements
//
// Side effects:
//   - Creates developer account with hashed password (transactional)
//   - Marks invitation as accepted with timestamp (transactional)
//   - Sets developer is_active=true
func (s *Service) AcceptInvitation(ctx context.Context, token, name, password string) (*Developer, error) {
	// Validate password
	if err := validatePassword(password); err != nil {
		return nil, err
	}

	// Hash the token to look up invitation
	tokenHash := hashToken(token)

	// Get invitation (outside transaction for validation)
	invitation, err := s.repo.GetInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Check if already accepted
	if invitation.AcceptedAt != nil {
		return nil, ErrInvalidToken
	}

	// Check if expired
	if time.Now().After(invitation.ExpiresAt) {
		return nil, ErrInvalidToken
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	passwordHashStr := string(hashedPassword)

	// Begin transaction to ensure atomicity
	txRepo, txCommitter, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure rollback on error (no-op after commit)
	defer func() {
		_ = txCommitter.Rollback(ctx)
	}()

	// Create developer within transaction
	dbParams := CreateDeveloperDBParams{
		Email:        invitation.Email,
		Name:         name,
		PasswordHash: &passwordHashStr,
		MaxKeys:      DefaultMaxKeys,
	}

	developer, err := txRepo.CreateDeveloper(ctx, dbParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create developer: %w", err)
	}

	// Mark invitation as accepted within the same transaction
	if err := txRepo.AcceptInvitation(ctx, invitation.ID); err != nil {
		return nil, fmt.Errorf("failed to mark invitation as accepted: %w", err)
	}

	// Commit transaction - all operations succeed or none do
	if err := txCommitter.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	s.logger.Info().
		Str("developer_id", developer.ID.String()).
		Str("email", developer.Email).
		Str("invitation_id", invitation.ID.String()).
		Msg("developer invitation accepted")

	return developer, nil
}

// GetDeveloperByID retrieves a developer by their ID.
// Returns ErrDeveloperNotFound if no developer exists with this ID.
func (s *Service) GetDeveloperByID(ctx context.Context, id uuid.UUID) (*Developer, error) {
	developer, err := s.repo.GetDeveloperByID(ctx, id)
	if err != nil {
		return nil, ErrDeveloperNotFound
	}
	return developer, nil
}

// GetDeveloperByGitHubID retrieves a developer by their GitHub ID.
// Returns ErrDeveloperNotFound if no developer exists with this GitHub ID.
func (s *Service) GetDeveloperByGitHubID(ctx context.Context, githubID int64) (*Developer, error) {
	developer, err := s.repo.GetDeveloperByGitHubID(ctx, githubID)
	if err != nil {
		return nil, ErrDeveloperNotFound
	}
	return developer, nil
}

// GetDeveloperByEmail retrieves a developer by their email address.
// Returns ErrDeveloperNotFound if no developer exists with this email.
func (s *Service) GetDeveloperByEmail(ctx context.Context, email string) (*Developer, error) {
	developer, err := s.repo.GetDeveloperByEmail(ctx, email)
	if err != nil {
		return nil, ErrDeveloperNotFound
	}
	return developer, nil
}

// UpdateDeveloper updates a developer's profile information.
// Only non-nil fields in params will be updated.
func (s *Service) UpdateDeveloper(ctx context.Context, id uuid.UUID, params UpdateDeveloperParams) (*Developer, error) {
	developer, err := s.repo.UpdateDeveloper(ctx, id, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update developer: %w", err)
	}
	return developer, nil
}

// UpdateDeveloperLastLogin updates the last_login_at timestamp for a developer.
// This is typically called after successful authentication.
func (s *Service) UpdateDeveloperLastLogin(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.UpdateDeveloperLastLogin(ctx, id); err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}
	return nil
}

// validatePassword enforces password length requirements:
// - Minimum 8 characters (less strict than user accounts)
// - Maximum 128 characters
//
// Developer passwords have simpler requirements than user passwords since
// developers are typically more security-aware and may use password managers.
func validatePassword(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 128 {
		return ErrPasswordTooLong
	}
	return nil
}

// generateSecureToken generates a cryptographically secure random token.
// Returns a 32-byte token encoded as URL-safe base64 (43 characters).
func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// hashToken hashes an invitation token using SHA-256.
// Returns the hash as a URL-safe base64 string.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.URLEncoding.EncodeToString(hash[:])
}
