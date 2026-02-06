package users

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/audit"
	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/email"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testEnv holds resources for integration tests
type testEnv struct {
	Container testcontainers.Container
	Pool      *pgxpool.Pool
	Context   context.Context
	Cancel    context.CancelFunc
}

// setupTestDB creates a PostgreSQL testcontainer and runs migrations
func setupTestDB(t *testing.T) *testEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Start PostgreSQL container
	container, err := testpostgres.Run(ctx,
		"docker.io/postgres:16-alpine",
		testpostgres.WithDatabase("testdb"),
		testpostgres.WithUsername("testuser"),
		testpostgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		cancel()
		t.Fatalf("failed to start postgres container: %v", err)
	}

	// Get connection string
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		cancel()
		_ = container.Terminate(ctx)
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Create connection pool
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		cancel()
		_ = container.Terminate(ctx)
		t.Fatalf("failed to create connection pool: %v", err)
	}

	// Run migrations
	if err := runMigrations(ctx, connStr); err != nil {
		pool.Close()
		cancel()
		_ = container.Terminate(ctx)
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Cleanup on test completion
	t.Cleanup(func() {
		pool.Close()
		cancel()
		_ = container.Terminate(context.Background())
	})

	return &testEnv{
		Container: container,
		Pool:      pool,
		Context:   ctx,
		Cancel:    cancel,
	}
}

// runMigrations runs database migrations for tests
func runMigrations(ctx context.Context, connStr string) error {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	// Create users table matching production schema
	_, err = conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL CHECK (role IN ('admin', 'editor', 'viewer')),
			is_active BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			last_login_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ DEFAULT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_users_role ON users (role, is_active);
		CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

		CREATE TABLE IF NOT EXISTS user_invitations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			accepted_at TIMESTAMPTZ,
			created_by UUID REFERENCES users(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);

		CREATE INDEX IF NOT EXISTS idx_user_invitations_token_hash ON user_invitations(token_hash, expires_at, accepted_at)
		WHERE accepted_at IS NULL;
		
		CREATE INDEX IF NOT EXISTS idx_user_invitations_user ON user_invitations(user_id);
		
		CREATE UNIQUE INDEX IF NOT EXISTS idx_user_invitations_active ON user_invitations(user_id) WHERE accepted_at IS NULL;
	`)

	return err
}

// setupUserService creates a test user service with dependencies
func setupUserService(t *testing.T, pool *pgxpool.Pool) *Service {
	t.Helper()

	emailConfig := config.EmailConfig{
		Enabled: false, // Disable actual email sending in tests
	}
	emailSvc, err := email.NewService(emailConfig, "../../../web/email/templates", zerolog.Nop())
	if err != nil {
		t.Fatalf("failed to create email service: %v", err)
	}

	auditLogger := audit.NewLoggerWithZerolog(zerolog.Nop())

	return NewService(pool, emailSvc, auditLogger, "http://localhost:8080", zerolog.Nop())
}

// Integration tests for service methods

func TestService_CreateUserAndInvite_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	t.Run("successfully creates user with invitation", func(t *testing.T) {
		params := CreateUserParams{
			Username: "testuser",
			Email:    "test@example.com",
			Role:     "viewer",
		}

		user, err := svc.CreateUserAndInvite(env.Context, params)
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		if !user.ID.Valid {
			t.Error("expected valid user ID")
		}
		if user.Username != "testuser" {
			t.Errorf("expected username 'testuser', got %s", user.Username)
		}
		if user.Email != "test@example.com" {
			t.Errorf("expected email 'test@example.com', got %s", user.Email)
		}
		if user.Role != "viewer" {
			t.Errorf("expected role 'viewer', got %s", user.Role)
		}
		if user.IsActive {
			t.Error("expected user to start inactive")
		}
	})

	t.Run("default role is viewer when not specified", func(t *testing.T) {
		params := CreateUserParams{
			Username: "defaultrole",
			Email:    "defaultrole@example.com",
			Role:     "", // Empty role should default to viewer
		}

		user, err := svc.CreateUserAndInvite(env.Context, params)
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		if user.Role != "viewer" {
			t.Errorf("expected default role 'viewer', got %s", user.Role)
		}
	})

	t.Run("rejects duplicate email", func(t *testing.T) {
		params1 := CreateUserParams{
			Username: "user1",
			Email:    "duplicate@example.com",
			Role:     "viewer",
		}
		_, err := svc.CreateUserAndInvite(env.Context, params1)
		if err != nil {
			t.Fatalf("first CreateUserAndInvite() error = %v", err)
		}

		params2 := CreateUserParams{
			Username: "user2",
			Email:    "duplicate@example.com",
			Role:     "editor",
		}
		_, err = svc.CreateUserAndInvite(env.Context, params2)
		if err == nil {
			t.Error("expected error for duplicate email, got nil")
		}
		if !errors.Is(err, ErrEmailTaken) {
			t.Errorf("expected ErrEmailTaken, got %v", err)
		}
	})

	t.Run("rejects duplicate username", func(t *testing.T) {
		params1 := CreateUserParams{
			Username: "duplicateusername",
			Email:    "email1@example.com",
			Role:     "viewer",
		}
		_, err := svc.CreateUserAndInvite(env.Context, params1)
		if err != nil {
			t.Fatalf("first CreateUserAndInvite() error = %v", err)
		}

		params2 := CreateUserParams{
			Username: "duplicateusername",
			Email:    "email2@example.com",
			Role:     "editor",
		}
		_, err = svc.CreateUserAndInvite(env.Context, params2)
		if err == nil {
			t.Error("expected error for duplicate username, got nil")
		}
		if !errors.Is(err, ErrUsernameTaken) {
			t.Errorf("expected ErrUsernameTaken, got %v", err)
		}
	})
}

func TestService_AcceptInvitation_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	t.Run("rejects invalid token", func(t *testing.T) {
		_, err := svc.AcceptInvitation(env.Context, "invalid-token", "SecurePassword123!")
		if err == nil {
			t.Error("expected error for invalid token, got nil")
		}
		if !errors.Is(err, ErrInvalidToken) {
			t.Errorf("expected ErrInvalidToken, got %v", err)
		}
	})

	t.Run("rejects weak password", func(t *testing.T) {
		_, err := svc.AcceptInvitation(env.Context, "some-token", "weak")
		if err == nil {
			t.Error("expected error for weak password, got nil")
		}
		if !errors.Is(err, ErrPasswordTooShort) {
			t.Errorf("expected ErrPasswordTooShort, got %v", err)
		}
	})
}

func TestService_UpdateUser_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	t.Run("successfully updates user", func(t *testing.T) {
		// Create initial user
		user, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
			Username: "original",
			Email:    "original@example.com",
			Role:     "viewer",
		})
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		// Update user
		err = svc.UpdateUser(env.Context, user.ID, UpdateUserParams{
			Username: "updated",
			Email:    "updated@example.com",
			Role:     "editor",
		}, "admin")
		if err != nil {
			t.Fatalf("UpdateUser() error = %v", err)
		}

		// Verify changes
		queries := postgres.New(env.Pool)
		updatedUser, err := queries.GetUserByID(env.Context, user.ID)
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}

		if updatedUser.Username != "updated" {
			t.Errorf("expected username 'updated', got %s", updatedUser.Username)
		}
		if updatedUser.Email != "updated@example.com" {
			t.Errorf("expected email 'updated@example.com', got %s", updatedUser.Email)
		}
		if updatedUser.Role != "editor" {
			t.Errorf("expected role 'editor', got %s", updatedUser.Role)
		}
	})

	t.Run("rejects update to non-existent user", func(t *testing.T) {
		nonExistentID := pgtype.UUID{
			Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			Valid: true,
		}

		err := svc.UpdateUser(env.Context, nonExistentID, UpdateUserParams{
			Username: "newuser",
			Email:    "new@example.com",
			Role:     "viewer",
		}, "admin")

		if err == nil {
			t.Error("expected error for non-existent user, got nil")
		}
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("expected ErrUserNotFound, got %v", err)
		}
	})
}

func TestService_DeactivateUser_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	t.Run("successfully deactivates active user", func(t *testing.T) {
		// Create and activate user
		user, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
			Username: "activeuser",
			Email:    "active@example.com",
			Role:     "viewer",
		})
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		queries := postgres.New(env.Pool)
		err = queries.ActivateUser(env.Context, user.ID)
		if err != nil {
			t.Fatalf("ActivateUser() error = %v", err)
		}

		// Deactivate user
		err = svc.DeactivateUser(env.Context, user.ID, "admin")
		if err != nil {
			t.Fatalf("DeactivateUser() error = %v", err)
		}

		// Verify user is inactive
		dbUser, err := queries.GetUserByID(env.Context, user.ID)
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}

		if dbUser.IsActive {
			t.Error("expected user to be inactive")
		}
	})

	t.Run("rejects deactivating already inactive user", func(t *testing.T) {
		// Create user (starts inactive)
		user, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
			Username: "inactiveuser",
			Email:    "inactive@example.com",
			Role:     "viewer",
		})
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		// Try to deactivate already inactive user
		err = svc.DeactivateUser(env.Context, user.ID, "admin")
		if err == nil {
			t.Error("expected error for already inactive user, got nil")
		}
	})
}

func TestService_ActivateUser_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	t.Run("successfully activates inactive user", func(t *testing.T) {
		// Create user (starts inactive)
		user, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
			Username: "toactivate",
			Email:    "toactivate@example.com",
			Role:     "viewer",
		})
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		// Activate user
		err = svc.ActivateUser(env.Context, user.ID, "admin")
		if err != nil {
			t.Fatalf("ActivateUser() error = %v", err)
		}

		// Verify user is active
		queries := postgres.New(env.Pool)
		dbUser, err := queries.GetUserByID(env.Context, user.ID)
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}

		if !dbUser.IsActive {
			t.Error("expected user to be active")
		}
	})
}

func TestService_ListUsers_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	// Create test users
	_, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
		Username: "admin1",
		Email:    "admin1@example.com",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("CreateUserAndInvite() error = %v", err)
	}

	user2, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
		Username: "editor1",
		Email:    "editor1@example.com",
		Role:     "editor",
	})
	if err != nil {
		t.Fatalf("CreateUserAndInvite() error = %v", err)
	}

	// Activate one user
	queries := postgres.New(env.Pool)
	err = queries.ActivateUser(env.Context, user2.ID)
	if err != nil {
		t.Fatalf("ActivateUser() error = %v", err)
	}

	t.Run("list all users with default limit", func(t *testing.T) {
		users, count, err := svc.ListUsers(env.Context, ListUsersFilters{})
		if err != nil {
			t.Fatalf("ListUsers() error = %v", err)
		}

		if count < 2 {
			t.Errorf("expected at least 2 users, got %d", count)
		}
		if len(users) < 2 {
			t.Errorf("expected at least 2 users in list, got %d", len(users))
		}
	})

	t.Run("filter by active status", func(t *testing.T) {
		activeTrue := true
		users, count, err := svc.ListUsers(env.Context, ListUsersFilters{
			IsActive: &activeTrue,
		})
		if err != nil {
			t.Fatalf("ListUsers() error = %v", err)
		}

		if count < 1 {
			t.Errorf("expected at least 1 active user, got %d", count)
		}
		for _, u := range users {
			if !u.IsActive {
				t.Error("expected all users to be active")
			}
		}
	})

	t.Run("filter by role", func(t *testing.T) {
		role := "admin"
		users, count, err := svc.ListUsers(env.Context, ListUsersFilters{
			Role: &role,
		})
		if err != nil {
			t.Fatalf("ListUsers() error = %v", err)
		}

		if count < 1 {
			t.Errorf("expected at least 1 admin user, got %d", count)
		}
		for _, u := range users {
			if u.Role != "admin" {
				t.Errorf("expected all users to be admin, got %s", u.Role)
			}
		}
	})
}

func TestService_GetUser_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	t.Run("successfully gets existing user", func(t *testing.T) {
		// Create user
		createdUser, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
			Username: "getuser",
			Email:    "getuser@example.com",
			Role:     "viewer",
		})
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		// Get user
		user, err := svc.GetUser(env.Context, createdUser.ID)
		if err != nil {
			t.Fatalf("GetUser() error = %v", err)
		}

		if user.Username != "getuser" {
			t.Errorf("expected username 'getuser', got %s", user.Username)
		}
		if user.Email != "getuser@example.com" {
			t.Errorf("expected email 'getuser@example.com', got %s", user.Email)
		}
	})

	t.Run("returns error for non-existent user", func(t *testing.T) {
		nonExistentID := pgtype.UUID{
			Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			Valid: true,
		}

		_, err := svc.GetUser(env.Context, nonExistentID)
		if err == nil {
			t.Error("expected error for non-existent user, got nil")
		}
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("expected ErrUserNotFound, got %v", err)
		}
	})
}

func TestService_DeleteUser_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	t.Run("successfully soft deletes user", func(t *testing.T) {
		// Create user
		user, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
			Username: "todelete",
			Email:    "todelete@example.com",
			Role:     "viewer",
		})
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		// Delete user
		err = svc.DeleteUser(env.Context, user.ID, "admin")
		if err != nil {
			t.Fatalf("DeleteUser() error = %v", err)
		}

		// Verify deleted_at is set
		var deletedAt pgtype.Timestamptz
		err = env.Pool.QueryRow(env.Context, "SELECT deleted_at FROM users WHERE id = $1", user.ID).Scan(&deletedAt)
		if err != nil {
			t.Fatalf("failed to query deleted_at: %v", err)
		}
		if !deletedAt.Valid {
			t.Error("expected deleted_at to be set")
		}
	})
}

func TestService_ResendInvitation_Integration(t *testing.T) {
	env := setupTestDB(t)
	svc := setupUserService(t, env.Pool)

	t.Run("rejects resending to already active user", func(t *testing.T) {
		// Create and activate user
		user, err := svc.CreateUserAndInvite(env.Context, CreateUserParams{
			Username: "alreadyactive",
			Email:    "alreadyactive@example.com",
			Role:     "viewer",
		})
		if err != nil {
			t.Fatalf("CreateUserAndInvite() error = %v", err)
		}

		queries := postgres.New(env.Pool)
		err = queries.ActivateUser(env.Context, user.ID)
		if err != nil {
			t.Fatalf("ActivateUser() error = %v", err)
		}

		// Try to resend invitation
		err = svc.ResendInvitation(env.Context, user.ID, "admin")
		if err == nil {
			t.Error("expected error for already active user, got nil")
		}
		if !errors.Is(err, ErrUserAlreadyActive) {
			t.Errorf("expected ErrUserAlreadyActive, got %v", err)
		}
	})
}
