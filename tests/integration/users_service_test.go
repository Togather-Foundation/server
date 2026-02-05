package integration

import (
	"errors"
	"testing"

	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/users"
	"github.com/Togather-Foundation/server/internal/email"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// setupUserServiceDeps creates email service and audit logger for user service tests
func setupUserServiceDeps(t *testing.T) (*email.Service, *audit.Logger) {
	t.Helper()

	emailConfig := config.EmailConfig{
		Enabled: false, // Disable actual email sending in tests
	}
	emailSvc, err := email.NewService(emailConfig, projectRoot(t)+"/web/email/templates", zerolog.Nop())
	require.NoError(t, err)

	auditLogger := audit.NewLoggerWithZerolog(zerolog.Nop())
	return emailSvc, auditLogger
}

// TestUsersService_CreateUserAndInvite_Success tests successful user creation
func TestUsersService_CreateUserAndInvite_Success(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	params := users.CreateUserParams{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "viewer",
	}

	user, err := svc.CreateUserAndInvite(env.Context, params)
	require.NoError(t, err)
	require.True(t, user.ID.Valid)
	require.Equal(t, "testuser", user.Username)
	require.Equal(t, "test@example.com", user.Email)
	require.Equal(t, "viewer", user.Role)
	require.False(t, user.IsActive, "user should start inactive")
}

// TestUsersService_CreateUserAndInvite_EmailTaken tests duplicate email rejection
func TestUsersService_CreateUserAndInvite_EmailTaken(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	// Create first user
	_, err := svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "user1",
		Email:    "duplicate@example.com",
		Role:     "viewer",
	})
	require.NoError(t, err)

	// Try to create second user with same email
	_, err = svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "user2",
		Email:    "duplicate@example.com",
		Role:     "editor",
	})
	require.Error(t, err)
	if !errors.Is(err, users.ErrEmailTaken) {
		t.Logf("Expected ErrEmailTaken, got: %v (type: %T)", err, err)
	}
	require.True(t, errors.Is(err, users.ErrEmailTaken))
}

// TestUsersService_CreateUserAndInvite_UsernameTaken tests duplicate username rejection
func TestUsersService_CreateUserAndInvite_UsernameTaken(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	// Create first user
	_, err := svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "duplicateuser",
		Email:    "user1@example.com",
		Role:     "viewer",
	})
	require.NoError(t, err)

	// Try to create second user with same username
	_, err = svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "duplicateuser",
		Email:    "user2@example.com",
		Role:     "editor",
	})
	require.Error(t, err)
	if !errors.Is(err, users.ErrUsernameTaken) {
		t.Logf("Expected ErrUsernameTaken, got: %v (type: %T)", err, err)
	}
	require.True(t, errors.Is(err, users.ErrUsernameTaken))
}

// TestUsersService_AcceptInvitation_InvalidToken tests invalid token rejection
func TestUsersService_AcceptInvitation_InvalidToken(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	err := svc.AcceptInvitation(env.Context, "invalid-token", "SecurePassword123!")
	require.Error(t, err)
	require.True(t, errors.Is(err, users.ErrInvalidToken))
}

// TestUsersService_UpdateUser_Success tests successful user update
func TestUsersService_UpdateUser_Success(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	// Create user
	user, err := svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "viewer",
	})
	require.NoError(t, err)

	// Update user
	err = svc.UpdateUser(env.Context, user.ID, users.UpdateUserParams{
		Username: "updateduser",
		Email:    "updated@example.com",
		Role:     "editor",
	}, "admin")
	require.NoError(t, err)

	// Verify changes
	queries := postgres.New(env.Pool)
	dbUser, err := queries.GetUserByID(env.Context, user.ID)
	require.NoError(t, err)
	require.Equal(t, "updateduser", dbUser.Username)
	require.Equal(t, "updated@example.com", dbUser.Email)
	require.Equal(t, "editor", dbUser.Role)
}

// TestUsersService_UpdateUser_NotFound tests updating non-existent user
func TestUsersService_UpdateUser_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	nonExistentID := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}

	err := svc.UpdateUser(env.Context, nonExistentID, users.UpdateUserParams{
		Username: "newuser",
		Email:    "new@example.com",
		Role:     "viewer",
	}, "admin")
	require.Error(t, err)
	require.True(t, errors.Is(err, users.ErrUserNotFound))
}

// TestUsersService_DeactivateUser_Success tests user deactivation
func TestUsersService_DeactivateUser_Success(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	// Create and activate user
	user, err := svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "viewer",
	})
	require.NoError(t, err)

	queries := postgres.New(env.Pool)
	err = queries.ActivateUser(env.Context, user.ID)
	require.NoError(t, err)

	// Deactivate user
	err = svc.DeactivateUser(env.Context, user.ID, "admin")
	require.NoError(t, err)

	// Verify user is inactive
	dbUser, err := queries.GetUserByID(env.Context, user.ID)
	require.NoError(t, err)
	require.False(t, dbUser.IsActive)
}

// TestUsersService_ActivateUser_Success tests user activation
func TestUsersService_ActivateUser_Success(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	// Create user (starts inactive)
	user, err := svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "viewer",
	})
	require.NoError(t, err)

	// Activate user
	err = svc.ActivateUser(env.Context, user.ID, "admin")
	require.NoError(t, err)

	// Verify user is active
	queries := postgres.New(env.Pool)
	dbUser, err := queries.GetUserByID(env.Context, user.ID)
	require.NoError(t, err)
	require.True(t, dbUser.IsActive)
}

// TestUsersService_DeleteUser_Success tests user deletion
func TestUsersService_DeleteUser_Success(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	// Create user
	user, err := svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "viewer",
	})
	require.NoError(t, err)

	// Delete user
	err = svc.DeleteUser(env.Context, user.ID, "admin")
	require.NoError(t, err)

	// FIXME: GetUserByID doesn't filter by deleted_at, so deleted users are still returned
	// This is a known limitation - soft-deleted users can still be retrieved via GetUserByID
	// In a production system, this query should be fixed to add "AND deleted_at IS NULL"

	// For now, we verify that deleted_at is set via direct database query
	var deletedAt pgtype.Timestamptz
	err = env.Pool.QueryRow(env.Context, "SELECT deleted_at FROM users WHERE id = $1", user.ID).Scan(&deletedAt)
	require.NoError(t, err)
	require.True(t, deletedAt.Valid, "deleted_at should be set")
}

// TestUsersService_GetUser_Success tests getting a user
func TestUsersService_GetUser_Success(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	// Create user
	user, err := svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "viewer",
	})
	require.NoError(t, err)

	// Get user
	dbUser, err := svc.GetUser(env.Context, user.ID)
	require.NoError(t, err)
	require.Equal(t, "testuser", dbUser.Username)
	require.Equal(t, "test@example.com", dbUser.Email)
}

// TestUsersService_ResendInvitation_UserAlreadyActive tests that active users can't receive invitations
func TestUsersService_ResendInvitation_UserAlreadyActive(t *testing.T) {
	env := setupTestEnv(t)
	emailSvc, auditLogger := setupUserServiceDeps(t)

	svc := users.NewService(env.Pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())

	// Create and activate user
	user, err := svc.CreateUserAndInvite(env.Context, users.CreateUserParams{
		Username: "testuser",
		Email:    "test@example.com",
		Role:     "viewer",
	})
	require.NoError(t, err)

	queries := postgres.New(env.Pool)
	err = queries.ActivateUser(env.Context, user.ID)
	require.NoError(t, err)

	// Try to resend invitation
	err = svc.ResendInvitation(env.Context, user.ID, "admin")
	require.Error(t, err)
	require.True(t, errors.Is(err, users.ErrUserAlreadyActive))
}
