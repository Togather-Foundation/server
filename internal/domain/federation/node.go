package federation

import (
	"time"

	"github.com/google/uuid"
)

// Node represents a federated SEL node
type Node struct {
	ID         uuid.UUID
	NodeDomain string
	NodeName   string
	BaseURL    string
	APIVersion string

	GeographicScope  *string
	TrustLevel       int
	FederationStatus string

	SyncEnabled          bool
	SyncDirection        string
	LastSyncAt           *time.Time
	LastSuccessfulSyncAt *time.Time
	SyncCursor           *string

	RequiresAuth bool
	ContactEmail *string
	ContactName  *string

	IsOnline          bool
	LastHealthCheckAt *time.Time
	LastErrorAt       *time.Time
	LastErrorMessage  *string

	Notes *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateNodeParams contains fields for creating a federation node
type CreateNodeParams struct {
	NodeDomain       string
	NodeName         string
	BaseURL          string
	APIVersion       string
	GeographicScope  *string
	TrustLevel       int
	FederationStatus string
	SyncEnabled      bool
	SyncDirection    string
	ContactEmail     *string
	ContactName      *string
	Notes            *string
}

// UpdateNodeParams contains fields that can be updated
type UpdateNodeParams struct {
	NodeName         *string
	BaseURL          *string
	APIVersion       *string
	GeographicScope  *string
	TrustLevel       *int
	FederationStatus *string
	SyncEnabled      *bool
	SyncDirection    *string
	ContactEmail     *string
	ContactName      *string
	Notes            *string
}

// ListNodesFilters contains filters for listing federation nodes
type ListNodesFilters struct {
	FederationStatus string
	SyncEnabled      *bool
	IsOnline         *bool
	Limit            int
}
