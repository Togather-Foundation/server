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

	GeographicScope *string

	// TrustLevel indicates the trust relationship with this federation node (1-10).
	// This affects how data from this node is handled and displayed.
	//
	// Trust Level Guidelines:
	//   1-3:  Low trust - Untrusted/new nodes. Data should be heavily scrutinized.
	//         Use for: newly discovered nodes, nodes with history of data quality issues.
	//   4-6:  Medium trust - Established nodes with good track record.
	//         Use for: known community nodes, regional aggregators with verified data.
	//   7-9:  High trust - Highly trusted institutional nodes.
	//         Use for: official government sources, major cultural institutions, verified partners.
	//   10:   Maximum trust - Reserved for canonical authoritative sources only.
	//         Use for: primary source-of-truth nodes, official national registries.
	//
	// Trust levels influence:
	// - Automatic acceptance vs manual review of synced events
	// - Display prominence in federated results
	// - Conflict resolution when multiple nodes provide same event
	// - Rate limiting and sync priority
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
	NodeDomain      string
	NodeName        string
	BaseURL         string
	APIVersion      string
	GeographicScope *string

	// TrustLevel indicates the trust relationship with this federation node (1-10).
	// See Node.TrustLevel for detailed trust level guidelines.
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
	NodeName        *string
	BaseURL         *string
	APIVersion      *string
	GeographicScope *string

	// TrustLevel indicates the trust relationship with this federation node (1-10).
	// See Node.TrustLevel for detailed trust level guidelines.
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
