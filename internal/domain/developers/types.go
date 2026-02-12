package developers

import (
	"time"

	"github.com/google/uuid"
)

// Developer represents a developer account in the domain layer.
// This is the clean domain model, free from database implementation details.
type Developer struct {
	ID             uuid.UUID
	Email          string
	Name           string
	GitHubID       *int64  // nil if not using GitHub OAuth
	GitHubUsername *string // nil if not using GitHub OAuth
	MaxKeys        int     // Maximum number of API keys this developer can create
	IsActive       bool    // Whether the developer account is active
	CreatedAt      time.Time
	LastLoginAt    *time.Time // nil if never logged in
}

// DeveloperInvitation represents an invitation to join the developer program.
// Invitations are single-use and have an expiration time.
type DeveloperInvitation struct {
	ID         uuid.UUID
	Email      string
	TokenHash  string     // SHA-256 hash of the invitation token
	InvitedBy  *uuid.UUID // nil if system-generated
	ExpiresAt  time.Time
	AcceptedAt *time.Time // nil if not yet accepted
	CreatedAt  time.Time
}

// CreateDeveloperParams contains all required and optional parameters for creating
// a new developer account with email/password authentication.
type CreateDeveloperParams struct {
	Email          string
	Name           string
	Password       string  // Plaintext password (will be hashed with bcrypt)
	GitHubID       *int64  // Optional GitHub OAuth ID
	GitHubUsername *string // Optional GitHub username
	MaxKeys        int     // Maximum number of API keys (default 5)
}

// APIKeyWithUsage represents an API key along with its usage statistics.
// This is used to provide developers with visibility into their key usage.
type APIKeyWithUsage struct {
	ID            uuid.UUID
	Prefix        string
	Name          string
	Role          string
	RateLimitTier string
	IsActive      bool
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	ExpiresAt     *time.Time
	// Usage statistics
	UsageToday int64 // Request count in last 24 hours
	Usage7d    int64 // Request count in last 7 days
	Usage30d   int64 // Request count in last 30 days
}

// CreateAPIKeyParams contains parameters for creating a new developer API key.
type CreateAPIKeyParams struct {
	DeveloperID   uuid.UUID
	Name          string
	ExpiresInDays *int // nil for no expiration
}

// UsageStats contains aggregated usage statistics for a developer's keys.
type UsageStats struct {
	DeveloperID   uuid.UUID
	TotalRequests int64
	TotalErrors   int64
	StartDate     time.Time
	EndDate       time.Time
}
