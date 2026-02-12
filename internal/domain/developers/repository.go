package developers

import (
	"context"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/google/uuid"
)

// Repository defines the interface for developer data access.
// This abstracts the postgres repository and allows the domain service
// to remain independent of storage implementation details.
type Repository interface {
	// Developer CRUD operations
	CreateDeveloper(ctx context.Context, params CreateDeveloperDBParams) (*Developer, error)
	GetDeveloperByID(ctx context.Context, id uuid.UUID) (*Developer, error)
	GetDeveloperByEmail(ctx context.Context, email string) (*Developer, error)
	GetDeveloperByGitHubID(ctx context.Context, githubID int64) (*Developer, error)
	ListDevelopers(ctx context.Context, limit, offset int) ([]*Developer, error)
	UpdateDeveloper(ctx context.Context, id uuid.UUID, params UpdateDeveloperParams) (*Developer, error)
	UpdateDeveloperLastLogin(ctx context.Context, id uuid.UUID) error
	DeactivateDeveloper(ctx context.Context, id uuid.UUID) error
	CountDevelopers(ctx context.Context) (int64, error)

	// Invitation operations
	CreateInvitation(ctx context.Context, params CreateInvitationDBParams) (*DeveloperInvitation, error)
	GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*DeveloperInvitation, error)
	AcceptInvitation(ctx context.Context, id uuid.UUID) error
	ListActiveInvitations(ctx context.Context) ([]*DeveloperInvitation, error)

	// API key operations
	ListDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) ([]postgres.ApiKey, error)
	CountDeveloperAPIKeys(ctx context.Context, developerID uuid.UUID) (int64, error)
	CreateAPIKey(ctx context.Context, params CreateAPIKeyDBParams) (*postgres.CreateAPIKeyRow, error)
	DeactivateAPIKey(ctx context.Context, id uuid.UUID) error
	GetAPIKeyByID(ctx context.Context, id uuid.UUID) (*postgres.ApiKey, error)

	// Usage operations
	GetAPIKeyUsageTotal(ctx context.Context, apiKeyID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error)
	GetDeveloperUsageTotal(ctx context.Context, developerID uuid.UUID, startDate, endDate time.Time) (totalRequests, totalErrors int64, err error)
}

// CreateDeveloperDBParams contains database-level parameters for creating a developer.
// This separates domain parameters from database implementation.
type CreateDeveloperDBParams struct {
	Email          string
	Name           string
	PasswordHash   *string // nil if using OAuth only
	GitHubID       *int64
	GitHubUsername *string
	MaxKeys        int
}

// UpdateDeveloperParams contains optional fields for updating a developer.
// Only non-nil fields will be updated.
type UpdateDeveloperParams struct {
	Name           *string
	GitHubID       *int64
	GitHubUsername *string
	MaxKeys        *int
	IsActive       *bool
}

// CreateInvitationDBParams contains database-level parameters for creating an invitation.
type CreateInvitationDBParams struct {
	Email     string
	TokenHash string
	InvitedBy *uuid.UUID
	ExpiresAt time.Time
}

// CreateAPIKeyDBParams contains database-level parameters for creating an API key.
type CreateAPIKeyDBParams struct {
	Prefix        string
	KeyHash       string
	HashVersion   int
	Name          string
	DeveloperID   uuid.UUID
	Role          string
	RateLimitTier string
	IsActive      bool
	ExpiresAt     *time.Time
}
