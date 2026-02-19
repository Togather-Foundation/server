// Package users provides user account management functionality including user creation,
// invitation flows, account activation, and user lifecycle operations. All operations
// are audit logged and support email notifications for important events like invitations.
//
// The package follows an invitation-based signup flow where users are created in an
// inactive state and must accept an email invitation to set their password and activate
// their account. This ensures email ownership verification before account activation.
//
// Core operations include:
//   - CreateUserAndInvite: Creates inactive user and sends email invitation
//   - AcceptInvitation: Validates token, sets password, and activates account
//   - UpdateUser: Updates user profile information
//   - DeactivateUser/ActivateUser: Manages user account status
//   - DeleteUser: Soft deletes user accounts
//   - ListUsers: Retrieves paginated user lists with filtering
//   - ResendInvitation: Regenerates and resends invitation emails
//
// All operations that modify user state emit audit log events for security tracking.
package users

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/email"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

// Domain-specific errors for user operations. These errors are returned
// by Service methods to indicate specific failure conditions that callers
// can check and handle appropriately.
var (
	// ErrUserNotFound is returned when a user lookup by ID fails to find a matching user.
	ErrUserNotFound = errors.New("user not found")

	// ErrInvalidToken is returned when an invitation token is invalid, expired, or already used.
	ErrInvalidToken = errors.New("invalid or expired invitation token")

	// ErrUserAlreadyActive is returned when attempting to send an invitation to an already active user.
	ErrUserAlreadyActive = errors.New("user is already active")

	// ErrUserAlreadyInactive is returned when attempting to deactivate a user who is already inactive.
	ErrUserAlreadyInactive = errors.New("user is already inactive")

	// ErrEmailTaken is returned when attempting to create or update a user with an email that already exists.
	ErrEmailTaken = errors.New("email is already taken")

	// ErrUsernameTaken is returned when attempting to create or update a user with a username that already exists.
	ErrUsernameTaken = errors.New("username is already taken")

	// ErrPasswordTooShort is returned when a password is less than 12 characters.
	ErrPasswordTooShort = errors.New("password must be at least 12 characters")

	// ErrPasswordTooLong is returned when a password exceeds 128 characters.
	ErrPasswordTooLong = errors.New("password must not exceed 128 characters")

	// ErrPasswordTooWeak is returned when a password doesn't meet complexity requirements.
	ErrPasswordTooWeak = errors.New("password must contain uppercase, lowercase, number, and special character")
)

const (
	// DefaultInvitationExpiry is the default time until an invitation expires
	DefaultInvitationExpiry = 168 * time.Hour // 7 days

	// DefaultRole is the default role assigned to new users
	DefaultRole = "viewer"

	// BcryptCost is the cost factor for bcrypt password hashing
	BcryptCost = 12
)

// Service handles user account management operations including creation, invitation flows,
// activation, updates, and lifecycle management. All operations are audit logged and
// support email notifications for invitation events.
//
// The service enforces an invitation-based signup flow where users are created in an
// inactive state and must accept an email invitation to set their password and activate.
// This ensures email ownership verification before granting access.
//
// All password operations use bcrypt hashing with a cost factor of 12. Passwords must
// meet NIST SP 800-63B requirements: minimum 12 characters with uppercase, lowercase,
// number, and special character.
type Service struct {
	db          *pgxpool.Pool
	queries     *postgres.Queries
	emailSvc    *email.Service
	auditLogger *audit.Logger
	baseURL     string
	logger      zerolog.Logger
	validator   *validator.Validate
}

// NewService creates and initializes a new user service instance with all required dependencies.
//
// Parameters:
//   - db: PostgreSQL connection pool for database operations
//   - emailSvc: Service for sending invitation and notification emails
//   - auditLogger: Logger for recording all user management operations
//   - baseURL: Base URL for the application, used to construct invitation links
//   - logger: Structured logger for service-level logging
//
// Returns a fully initialized Service ready to handle user management operations.
func NewService(
	db *pgxpool.Pool,
	emailSvc *email.Service,
	auditLogger *audit.Logger,
	baseURL string,
	logger zerolog.Logger,
) *Service {
	return &Service{
		db:          db,
		queries:     postgres.New(db),
		emailSvc:    emailSvc,
		auditLogger: auditLogger,
		baseURL:     baseURL,
		logger:      logger.With().Str("component", "users").Logger(),
		validator:   validator.New(),
	}
}

// CreateUserParams contains all required and optional parameters for creating a new user account.
// All fields are validated before user creation using struct tags.
type CreateUserParams struct {
	// Username is the unique username for the user (3-50 alphanumeric characters, required).
	Username string `validate:"required,alphanum,min=3,max=50"`

	// Email is the unique email address for the user (valid email format, max 255 chars, required).
	Email string `validate:"required,email,max=255"`

	// Role is the user's role (admin, editor, or viewer). Defaults to "viewer" if not specified.
	Role string `validate:"omitempty,oneof=admin editor viewer"`

	// CreatedBy is the UUID of the admin creating this user (optional, for audit logging).
	CreatedBy pgtype.UUID // Admin who is creating the user
}

// UpdateUserParams contains parameters for updating an existing user's information.
// All fields are required and validated before the update operation.
type UpdateUserParams struct {
	// Username is the new username (3-50 alphanumeric characters, required).
	Username string `validate:"required,alphanum,min=3,max=50"`

	// Email is the new email address (valid email format, max 255 chars, required).
	Email string `validate:"required,email,max=255"`

	// Role is the new role (admin, editor, or viewer, required).
	Role string `validate:"required,oneof=admin editor viewer"`
}

// ListUsersFilters contains optional filters for querying the user list.
// All filter fields are optional - nil values mean no filtering on that field.
type ListUsersFilters struct {
	// IsActive filters by account active status. Nil means no filtering.
	IsActive *bool

	// Role filters by user role (admin, editor, viewer). Nil means no filtering.
	Role *string

	// Limit is the maximum number of users to return (defaults to 50 if <= 0).
	Limit int32

	// Offset is the number of users to skip for pagination (defaults to 0).
	Offset int32
}

// uuidEquals compares two pgtype.UUID values for equality.
// Returns true if both are valid and have identical bytes.
func uuidEquals(a, b pgtype.UUID) bool {
	if !a.Valid || !b.Valid {
		return false
	}
	return bytes.Equal(a.Bytes[:], b.Bytes[:])
}

// CreateUserAndInvite creates a new user account in an inactive state and sends an email
// invitation with a secure token. The user must accept the invitation and set a password
// to activate their account.
//
// The operation is atomic: user creation and invitation record are created in a single
// transaction. Email sending happens after the transaction commits, so email failures
// do not rollback the database changes. Admins can resend invitations using ResendInvitation.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - params: User creation parameters (username, email, role, createdBy)
//
// Returns the created user record or an error. Possible errors:
//   - ErrEmailTaken: Email already exists in the system
//   - ErrUsernameTaken: Username already exists in the system
//   - Validation errors: Invalid username, email format, or role
//
// Side effects:
//   - Creates user record in database (inactive, no password)
//   - Creates invitation record with hashed token (expires in 7 days)
//   - Sends invitation email with acceptance link
//   - Emits "user.created" audit log event
func (s *Service) CreateUserAndInvite(ctx context.Context, params CreateUserParams) (postgres.User, error) {
	// Set default role if not provided
	if params.Role == "" {
		params.Role = DefaultRole
	}

	// Validate inputs
	if err := s.validator.Struct(params); err != nil {
		return postgres.User{}, fmt.Errorf("invalid parameters: %w", err)
	}

	// Check if email is already taken
	existingUserByEmail, err := s.queries.GetUserByEmail(ctx, params.Email)
	if err == nil && existingUserByEmail.ID.Valid {
		return postgres.User{}, ErrEmailTaken
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return postgres.User{}, fmt.Errorf("failed to check email: %w", err)
	}

	// Check if username is already taken
	existingUserByUsername, err := s.queries.GetUserByUsername(ctx, params.Username)
	if err == nil && existingUserByUsername.ID.Valid {
		return postgres.User{}, ErrUsernameTaken
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return postgres.User{}, fmt.Errorf("failed to check username: %w", err)
	}

	// Begin transaction for atomic user + invitation creation
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return postgres.User{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx) // Auto-rollback on error (ignore error - commit may have succeeded)
	}()

	// Create queries with transaction
	qtx := s.queries.WithTx(tx)

	// Create user in inactive state with empty password hash
	// User will set password when accepting invitation
	userRow, err := qtx.CreateUser(ctx, postgres.CreateUserParams{
		Username:     params.Username,
		Email:        params.Email,
		PasswordHash: "", // Empty until invitation is accepted
		Role:         params.Role,
		IsActive:     false, // Inactive until invitation is accepted
	})
	if err != nil {
		// Convert database constraint errors to domain errors
		if domainErr := convertDBError(err); domainErr != err {
			return postgres.User{}, domainErr
		}
		return postgres.User{}, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate secure invitation token
	token, err := generateSecureToken()
	if err != nil {
		return postgres.User{}, fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash the token before storing (plaintext token sent in email, hash stored in DB)
	tokenHash := hashToken(token)

	// Create invitation record
	expiresAt := pgtype.Timestamptz{
		Time:  time.Now().Add(DefaultInvitationExpiry),
		Valid: true,
	}

	_, err = qtx.CreateUserInvitation(ctx, postgres.CreateUserInvitationParams{
		UserID:    userRow.ID,
		TokenHash: tokenHash,
		Email:     params.Email,
		ExpiresAt: expiresAt,
		CreatedBy: params.CreatedBy,
	})
	if err != nil {
		return postgres.User{}, fmt.Errorf("failed to create invitation: %w", err)
	}

	// Commit transaction before sending email
	// Email failure should not rollback the database changes
	if err := tx.Commit(ctx); err != nil {
		return postgres.User{}, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Generate invitation link
	inviteLink := fmt.Sprintf("%s/accept-invitation?token=%s", s.baseURL, token)

	// Get the admin who created the user for the email
	var invitedBy string
	if params.CreatedBy.Valid {
		admin, err := s.queries.GetUserByID(ctx, params.CreatedBy)
		if err == nil {
			invitedBy = admin.Username
		} else {
			invitedBy = "Administrator"
		}
	} else {
		invitedBy = "Administrator"
	}

	// Send invitation email after commit (idempotent, can retry via ResendInvitation)
	if s.emailSvc == nil {
		s.logger.Warn().Str("email", params.Email).Msg("email service not available, skipping invitation email (admin can resend later)")
	} else if err := s.emailSvc.SendInvitation(ctx, params.Email, inviteLink, invitedBy); err != nil {
		s.logger.Error().Err(err).Str("email", params.Email).Msg("failed to send invitation email")
		// User + invitation exist, admin can resend via ResendInvitation
	}

	// Audit log
	s.auditLogger.LogSuccess(
		"user.created",
		invitedBy,
		"user",
		uuidToString(userRow.ID),
		"",
		map[string]string{
			"username": params.Username,
			"email":    params.Email,
			"role":     params.Role,
		},
	)

	// Return user object
	return postgres.User{
		ID:        userRow.ID,
		Username:  userRow.Username,
		Email:     userRow.Email,
		Role:      userRow.Role,
		IsActive:  userRow.IsActive,
		CreatedAt: userRow.CreatedAt,
	}, nil
}

// validatePassword enforces password strength requirements:
// - Minimum 12 characters
// - Maximum 128 characters
// - Must contain at least one uppercase letter
// - Must contain at least one lowercase letter
// - Must contain at least one number
// - Must contain at least one special character (punctuation or symbol)
//
// These requirements follow NIST SP 800-63B guidelines for user-chosen secrets.
func validatePassword(password string) error {
	// Check length
	if len(password) < 12 {
		return ErrPasswordTooShort
	}
	if len(password) > 128 {
		return ErrPasswordTooLong
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber || !hasSpecial {
		return ErrPasswordTooWeak
	}

	return nil
}

// AcceptInvitation validates an invitation token, sets the user's password using bcrypt,
// and activates the user account. The operation is atomic: password update, activation,
// and invitation acceptance marking all occur in a single transaction.
//
// The token is validated against its SHA-256 hash stored in the database. Tokens are
// single-use and expire after 7 days. The password must meet NIST SP 800-63B requirements:
//   - Minimum 12 characters
//   - Maximum 128 characters
//   - Contains uppercase, lowercase, number, and special character
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - token: The plaintext invitation token from the email link
//   - password: The user's chosen password (validated for strength)
//
// Returns the activated user record or an error. Possible errors:
//   - ErrInvalidToken: Token doesn't exist, expired, or already used
//   - ErrPasswordTooShort/TooLong/TooWeak: Password doesn't meet requirements
//
// Side effects:
//   - Updates user password (bcrypt hashed with cost 12)
//   - Sets user.is_active to true
//   - Marks invitation as accepted with timestamp
//   - Emits "user.invitation_accepted" audit log event
func (s *Service) AcceptInvitation(ctx context.Context, token, password string) (postgres.GetUserByIDRow, error) {
	// Validate password strength
	if err := validatePassword(password); err != nil {
		return postgres.GetUserByIDRow{}, fmt.Errorf("invalid password: %w", err)
	}

	// Hash the token to lookup the invitation
	tokenHash := hashToken(token)

	// Validate token and get invitation
	invitation, err := s.queries.GetUserInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return postgres.GetUserByIDRow{}, ErrInvalidToken
		}
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to get invitation: %w", err)
	}

	// Check if invitation has already been accepted
	if invitation.AcceptedAt.Valid {
		return postgres.GetUserByIDRow{}, ErrInvalidToken
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to hash password: %w", err)
	}

	// Begin transaction for atomic password update + activation + invitation marking
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx) // Auto-rollback on error (ignore error - commit may have succeeded)
	}()

	// Create queries with transaction
	qtx := s.queries.WithTx(tx)

	// Update user with password and activate
	if err := qtx.UpdateUserPassword(ctx, postgres.UpdateUserPasswordParams{
		ID:           invitation.UserID,
		PasswordHash: string(hashedPassword),
	}); err != nil {
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to update password: %w", err)
	}

	if err := qtx.ActivateUser(ctx, invitation.UserID); err != nil {
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to activate user: %w", err)
	}

	// Mark invitation as accepted
	if err := qtx.MarkInvitationAccepted(ctx, invitation.ID); err != nil {
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to mark invitation as accepted: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Get user for audit log and return (after successful commit)
	user, err := s.queries.GetUserByID(ctx, invitation.UserID)
	if err != nil {
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to get user: %w", err)
	}

	// Audit log
	s.auditLogger.LogSuccess(
		"user.invitation_accepted",
		user.Username,
		"user",
		uuidToString(invitation.UserID),
		"",
		map[string]string{
			"email": invitation.Email,
		},
	)

	return user, nil
}

// UpdateUser updates a user's profile information (username, email, role).
// The user's active status is preserved and cannot be changed through this method.
// Use ActivateUser or DeactivateUser to change account status.
//
// Email and username uniqueness is enforced - returns an error if the new values
// conflict with another user's data (excluding the user being updated).
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - id: UUID of the user to update
//   - params: New values for username, email, and role
//   - updatedBy: Username of the admin performing the update (for audit logging)
//
// Returns nil on success or an error. Possible errors:
//   - ErrUserNotFound: User with the given ID doesn't exist
//   - ErrEmailTaken: New email is already used by another user
//   - ErrUsernameTaken: New username is already used by another user
//   - Validation errors: Invalid username, email format, or role
//
// Side effects:
//   - Updates user record in database
//   - Emits "user.updated" audit log event
func (s *Service) UpdateUser(ctx context.Context, id pgtype.UUID, params UpdateUserParams, updatedBy string) error {
	// Validate inputs
	if err := s.validator.Struct(params); err != nil {
		return fmt.Errorf("invalid parameters: %w", err)
	}

	// Get existing user
	existingUser, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if email is taken by another user
	if params.Email != existingUser.Email {
		userWithEmail, err := s.queries.GetUserByEmail(ctx, params.Email)
		if err == nil && userWithEmail.ID.Valid && !uuidEquals(userWithEmail.ID, id) {
			return ErrEmailTaken
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to check email: %w", err)
		}
	}

	// Check if username is taken by another user
	if params.Username != existingUser.Username {
		userWithUsername, err := s.queries.GetUserByUsername(ctx, params.Username)
		if err == nil && userWithUsername.ID.Valid && !uuidEquals(userWithUsername.ID, id) {
			return ErrUsernameTaken
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to check username: %w", err)
		}
	}

	// Update user
	if err := s.queries.UpdateUser(ctx, postgres.UpdateUserParams{
		ID:       id,
		Username: params.Username,
		Email:    params.Email,
		Role:     params.Role,
		IsActive: existingUser.IsActive, // Keep existing active state
	}); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	// Audit log
	s.auditLogger.LogSuccess(
		"user.updated",
		updatedBy,
		"user",
		uuidToString(id),
		"",
		map[string]string{
			"username": params.Username,
			"email":    params.Email,
			"role":     params.Role,
		},
	)

	return nil
}

// DeactivateUser sets a user's is_active flag to false, preventing login.
// The user's data remains in the database and can be reactivated using ActivateUser.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - id: UUID of the user to deactivate
//   - deactivatedBy: Username of the admin performing the deactivation (for audit logging)
//
// Returns nil on success or an error. Possible errors:
//   - ErrUserNotFound: User with the given ID doesn't exist
//
// Side effects:
//   - Sets user.is_active to false in database
//   - Emits "user.deactivated" audit log event
func (s *Service) DeactivateUser(ctx context.Context, id pgtype.UUID, deactivatedBy string) error {
	// Check if user exists
	user, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if already inactive
	if !user.IsActive {
		return ErrUserAlreadyInactive
	}

	// Deactivate user
	if err := s.queries.DeactivateUser(ctx, id); err != nil {
		return fmt.Errorf("failed to deactivate user: %w", err)
	}

	// Audit log
	s.auditLogger.LogSuccess(
		"user.deactivated",
		deactivatedBy,
		"user",
		uuidToString(id),
		"",
		map[string]string{
			"username": user.Username,
			"email":    user.Email,
		},
	)

	return nil
}

// ActivateUser sets a user's is_active flag to true, allowing login.
// This is typically used to reactivate a previously deactivated account.
// For new users accepting invitations, use AcceptInvitation instead.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - id: UUID of the user to activate
//   - activatedBy: Username of the admin performing the activation (for audit logging)
//
// Returns nil on success or an error. Possible errors:
//   - ErrUserNotFound: User with the given ID doesn't exist
//
// Side effects:
//   - Sets user.is_active to true in database
//   - Emits "user.activated" audit log event
func (s *Service) ActivateUser(ctx context.Context, id pgtype.UUID, activatedBy string) error {
	// Check if user exists
	user, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Activate user
	if err := s.queries.ActivateUser(ctx, id); err != nil {
		return fmt.Errorf("failed to activate user: %w", err)
	}

	// Audit log
	s.auditLogger.LogSuccess(
		"user.activated",
		activatedBy,
		"user",
		uuidToString(id),
		"",
		map[string]string{
			"username": user.Username,
			"email":    user.Email,
		},
	)

	return nil
}

// DeleteUser performs a soft delete of a user account by setting the deleted_at timestamp.
// The user's data remains in the database for audit and referential integrity purposes.
// Soft-deleted users are excluded from normal queries but remain accessible for historical records.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - id: UUID of the user to delete
//   - deletedBy: Username of the admin performing the deletion (for audit logging)
//
// Returns nil on success or an error. Possible errors:
//   - ErrUserNotFound: User with the given ID doesn't exist
//
// Side effects:
//   - Sets user.deleted_at timestamp in database
//   - User is excluded from standard queries
//   - Emits "user.deleted" audit log event
func (s *Service) DeleteUser(ctx context.Context, id pgtype.UUID, deletedBy string) error {
	// Check if user exists
	user, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Delete user (soft delete)
	if err := s.queries.DeleteUser(ctx, id); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Audit log
	s.auditLogger.LogSuccess(
		"user.deleted",
		deletedBy,
		"user",
		uuidToString(id),
		"",
		map[string]string{
			"username": user.Username,
			"email":    user.Email,
		},
	)

	return nil
}

// ListUsers retrieves a paginated list of users with optional filtering by active status
// and role. Returns both the user list and the total count matching the filters.
//
// Default pagination limit is 50 users if not specified. All filter fields are optional.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - filters: Optional filters for is_active, role, limit, and offset
//
// Returns:
//   - User list matching the filters
//   - Total count of users matching the filters (for pagination)
//   - Error if the query fails
//
// Example:
//
//	filters := ListUsersFilters{
//	    IsActive: boolPtr(true),
//	    Role: stringPtr("admin"),
//	    Limit: 20,
//	    Offset: 40,
//	}
//	users, total, err := svc.ListUsers(ctx, filters)
func (s *Service) ListUsers(ctx context.Context, filters ListUsersFilters) ([]postgres.ListUsersWithFiltersRow, int64, error) {
	// Set default pagination if not provided
	if filters.Limit <= 0 {
		filters.Limit = 50
	}

	// Get total count
	countParams := postgres.CountUsersParams{}
	if filters.IsActive != nil {
		countParams.IsActive = pgtype.Bool{Bool: *filters.IsActive, Valid: true}
	}
	if filters.Role != nil {
		countParams.Role = pgtype.Text{String: *filters.Role, Valid: true}
	}

	totalCount, err := s.queries.CountUsers(ctx, countParams)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// Get users list
	listParams := postgres.ListUsersWithFiltersParams{
		Limit:  filters.Limit,
		Offset: filters.Offset,
	}
	if filters.IsActive != nil {
		listParams.IsActive = pgtype.Bool{Bool: *filters.IsActive, Valid: true}
	}
	if filters.Role != nil {
		listParams.Role = pgtype.Text{String: *filters.Role, Valid: true}
	}

	users, err := s.queries.ListUsersWithFilters(ctx, listParams)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	return users, totalCount, nil
}

// GetUser retrieves a single user by their UUID.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - id: UUID of the user to retrieve
//
// Returns the user record or an error. Possible errors:
//   - ErrUserNotFound: User with the given ID doesn't exist
func (s *Service) GetUser(ctx context.Context, id pgtype.UUID) (postgres.GetUserByIDRow, error) {
	user, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return postgres.GetUserByIDRow{}, ErrUserNotFound
		}
		return postgres.GetUserByIDRow{}, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// ResendInvitation generates a new invitation token and resends the invitation email
// to an inactive user. This is used when the original invitation email was not received
// or the token has expired.
//
// A new token is always generated for security (original tokens are not stored in plaintext).
// The invitation expiration is reset to 7 days from now.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - userID: UUID of the user to resend invitation to
//   - resentBy: Username of the admin resending the invitation (for audit logging)
//
// Returns nil on success or an error. Possible errors:
//   - ErrUserNotFound: User with the given ID doesn't exist
//   - ErrUserAlreadyActive: Cannot resend invitation to an active user
//
// Side effects:
//   - Creates new invitation record with fresh token and expiration
//   - Sends invitation email with new acceptance link
//   - Emits "user.invitation_resent" audit log event
func (s *Service) ResendInvitation(ctx context.Context, userID pgtype.UUID, resentBy string) error {
	// Get user
	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user is already active
	if user.IsActive {
		return ErrUserAlreadyActive
	}

	// Expire any existing pending invitations so the unique partial index
	// (idx_user_invitations_active) allows a new row.
	if err := s.queries.ExpirePendingInvitationsForUser(ctx, userID); err != nil {
		return fmt.Errorf("failed to expire pending invitations: %w", err)
	}

	// Generate a fresh invitation token (we never store plaintext, so
	// existing tokens cannot be reused).
	token, err := generateSecureToken()
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}

	tokenHash := hashToken(token)

	expiresAt := pgtype.Timestamptz{
		Time:  time.Now().Add(DefaultInvitationExpiry),
		Valid: true,
	}

	var createdBy pgtype.UUID
	if resentBy != "" {
		admin, adminErr := s.queries.GetUserByUsername(ctx, resentBy)
		if adminErr == nil {
			createdBy = admin.ID
		}
	}

	_, err = s.queries.CreateUserInvitation(ctx, postgres.CreateUserInvitationParams{
		UserID:    userID,
		TokenHash: tokenHash,
		Email:     user.Email,
		ExpiresAt: expiresAt,
		CreatedBy: createdBy,
	})
	if err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}

	// Generate invitation link
	inviteLink := fmt.Sprintf("%s/accept-invitation?token=%s", s.baseURL, token)

	// Send invitation email
	if s.emailSvc == nil {
		return fmt.Errorf("email service not available, cannot send invitation")
	}
	if err := s.emailSvc.SendInvitation(ctx, user.Email, inviteLink, resentBy); err != nil {
		return fmt.Errorf("failed to send invitation email: %w", err)
	}

	// Audit log
	s.auditLogger.LogSuccess(
		"user.invitation_resent",
		resentBy,
		"user",
		uuidToString(userID),
		"",
		map[string]string{
			"username": user.Username,
			"email":    user.Email,
		},
	)

	return nil
}

// generateSecureToken generates a cryptographically secure random token
// Returns a 32-byte token encoded as URL-safe base64 (43 characters)
func generateSecureToken() (string, error) {
	// Generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode as URL-safe base64
	token := base64.URLEncoding.EncodeToString(b)
	return token, nil
}

// hashToken hashes an invitation token using SHA-256
// Returns the hash as a URL-safe base64 string
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.URLEncoding.EncodeToString(hash[:])
}

// convertDBError converts database constraint errors to domain-specific errors
func convertDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// PostgreSQL error code 23505 = unique_violation
		if pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "email") {
				return ErrEmailTaken
			}
			if strings.Contains(pgErr.ConstraintName, "username") {
				return ErrUsernameTaken
			}
		}
	}
	return err
}

// uuidToString converts a pgtype.UUID to a string, returning empty string if invalid
func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
