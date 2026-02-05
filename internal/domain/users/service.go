package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/email"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"
)

// Error types for user domain operations
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidToken      = errors.New("invalid or expired invitation token")
	ErrUserAlreadyActive = errors.New("user is already active")
	ErrEmailTaken        = errors.New("email is already taken")
	ErrUsernameTaken     = errors.New("username is already taken")
)

const (
	// DefaultInvitationExpiry is the default time until an invitation expires
	DefaultInvitationExpiry = 168 * time.Hour // 7 days

	// DefaultRole is the default role assigned to new users
	DefaultRole = "viewer"

	// BcryptCost is the cost factor for bcrypt password hashing
	BcryptCost = 12
)

// Service handles user management operations
type Service struct {
	queries     *postgres.Queries
	emailSvc    *email.Service
	auditLogger *audit.Logger
	baseURL     string
	logger      zerolog.Logger
}

// NewService creates a new user service instance
func NewService(
	queries *postgres.Queries,
	emailSvc *email.Service,
	auditLogger *audit.Logger,
	baseURL string,
	logger zerolog.Logger,
) *Service {
	return &Service{
		queries:     queries,
		emailSvc:    emailSvc,
		auditLogger: auditLogger,
		baseURL:     baseURL,
		logger:      logger.With().Str("component", "users").Logger(),
	}
}

// CreateUserParams contains parameters for creating a new user
type CreateUserParams struct {
	Username  string
	Email     string
	Role      string
	CreatedBy pgtype.UUID // Admin who is creating the user
}

// UpdateUserParams contains parameters for updating a user
type UpdateUserParams struct {
	Username string
	Email    string
	Role     string
}

// ListUsersFilters contains filters for listing users
type ListUsersFilters struct {
	IsActive *bool
	Role     *string
	Limit    int32
	Offset   int32
}

// CreateUserAndInvite creates a new inactive user and sends an invitation email
func (s *Service) CreateUserAndInvite(ctx context.Context, params CreateUserParams) (postgres.User, error) {
	// Set default role if not provided
	if params.Role == "" {
		params.Role = DefaultRole
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

	// Create user in inactive state with empty password hash
	// User will set password when accepting invitation
	userRow, err := s.queries.CreateUser(ctx, postgres.CreateUserParams{
		Username:     params.Username,
		Email:        params.Email,
		PasswordHash: "", // Empty until invitation is accepted
		Role:         params.Role,
		IsActive:     false, // Inactive until invitation is accepted
	})
	if err != nil {
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

	_, err = s.queries.CreateUserInvitation(ctx, postgres.CreateUserInvitationParams{
		UserID:    userRow.ID,
		TokenHash: tokenHash,
		Email:     params.Email,
		ExpiresAt: expiresAt,
		CreatedBy: params.CreatedBy,
	})
	if err != nil {
		return postgres.User{}, fmt.Errorf("failed to create invitation: %w", err)
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

	// Send invitation email
	if err := s.emailSvc.SendInvitation(params.Email, inviteLink, invitedBy); err != nil {
		s.logger.Error().Err(err).Str("email", params.Email).Msg("failed to send invitation email")
		// Don't fail the operation if email sending fails
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

// AcceptInvitation validates an invitation token, sets the user's password, and activates the account
func (s *Service) AcceptInvitation(ctx context.Context, token, password string) error {
	// Hash the token to lookup the invitation
	tokenHash := hashToken(token)

	// Validate token and get invitation
	invitation, err := s.queries.GetUserInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		return fmt.Errorf("failed to get invitation: %w", err)
	}

	// Check if invitation has already been accepted
	if invitation.AcceptedAt.Valid {
		return ErrInvalidToken
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update user with password and activate
	if err := s.queries.UpdateUserPassword(ctx, postgres.UpdateUserPasswordParams{
		ID:           invitation.UserID,
		PasswordHash: string(hashedPassword),
	}); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	if err := s.queries.ActivateUser(ctx, invitation.UserID); err != nil {
		return fmt.Errorf("failed to activate user: %w", err)
	}

	// Mark invitation as accepted
	if err := s.queries.MarkInvitationAccepted(ctx, invitation.ID); err != nil {
		return fmt.Errorf("failed to mark invitation as accepted: %w", err)
	}

	// Get user for audit log
	user, err := s.queries.GetUserByID(ctx, invitation.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
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

	return nil
}

// UpdateUser updates user details
func (s *Service) UpdateUser(ctx context.Context, id pgtype.UUID, params UpdateUserParams, updatedBy string) error {
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
		if err == nil && userWithEmail.ID.Valid && userWithEmail.ID.Bytes != id.Bytes {
			return ErrEmailTaken
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to check email: %w", err)
		}
	}

	// Check if username is taken by another user
	if params.Username != existingUser.Username {
		userWithUsername, err := s.queries.GetUserByUsername(ctx, params.Username)
		if err == nil && userWithUsername.ID.Valid && userWithUsername.ID.Bytes != id.Bytes {
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

// DeactivateUser deactivates a user account
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
		return ErrUserAlreadyActive // Reusing this error, could create ErrUserAlreadyInactive
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

// ActivateUser reactivates a user account
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

// DeleteUser soft deletes a user account
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

// ListUsers returns a list of users with optional filtering
func (s *Service) ListUsers(ctx context.Context, filters ListUsersFilters) ([]postgres.ListUsersWithFiltersRow, int64, error) {
	// Set default pagination if not provided
	if filters.Limit <= 0 {
		filters.Limit = 50
	}

	// Get total count
	countParams := postgres.CountUsersParams{}
	if filters.IsActive != nil {
		countParams.Column1 = *filters.IsActive
	}
	if filters.Role != nil {
		countParams.Column2 = *filters.Role
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
		listParams.Column1 = *filters.IsActive
	}
	if filters.Role != nil {
		listParams.Column2 = *filters.Role
	}

	users, err := s.queries.ListUsersWithFilters(ctx, listParams)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	return users, totalCount, nil
}

// GetUser retrieves a single user by ID
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

// ResendInvitation resends an invitation email to a user
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

	// Get pending invitations
	invitations, err := s.queries.ListPendingInvitationsForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get pending invitations: %w", err)
	}

	// If there are existing valid invitations, use the first one
	var token string
	if len(invitations) > 0 {
		// Cannot reuse existing token - it was sent once and we don't store plaintext
		// Generate a new token instead
		token, err = generateSecureToken()
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
			// Try to get the admin user ID
			admin, err := s.queries.GetUserByUsername(ctx, resentBy)
			if err == nil {
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
	} else {
		// Generate new invitation
		token, err = generateSecureToken()
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
			// Try to get the admin user ID
			admin, err := s.queries.GetUserByUsername(ctx, resentBy)
			if err == nil {
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
	}

	// Generate invitation link
	inviteLink := fmt.Sprintf("%s/accept-invitation?token=%s", s.baseURL, token)

	// Send invitation email
	if err := s.emailSvc.SendInvitation(user.Email, inviteLink, resentBy); err != nil {
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

// uuidToString converts a pgtype.UUID to a string, returning empty string if invalid
func uuidToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
