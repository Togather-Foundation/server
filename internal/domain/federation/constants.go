package federation

// Default values for federation and event processing
const (
	// DefaultEventVersion is the initial version number for new events
	DefaultEventVersion = 1

	// DefaultLifecycleState is the default lifecycle state for newly created events
	DefaultLifecycleState = "published"

	// DefaultChangeFeedLimit is the default number of items returned in change feed queries
	DefaultChangeFeedLimit = 50

	// MaxChangeFeedLimit is the maximum number of items allowed in change feed queries
	MaxChangeFeedLimit = 200

	// MinChangeFeedLimit is the minimum number of items allowed in change feed queries
	MinChangeFeedLimit = 1
)

// Federation node statuses. These mirror the CHECK constraint on
// federation_nodes.federation_status.
const (
	FederationStatusPending = "pending"
	FederationStatusActive  = "active"
	FederationStatusPaused  = "paused"
	FederationStatusBlocked = "blocked"
)

// Federation sync directions. These mirror the CHECK constraint on
// federation_nodes.sync_direction.
const (
	SyncDirectionBidirectional = "bidirectional"
	SyncDirectionPullOnly      = "pull_only"
	SyncDirectionPushOnly      = "push_only"
	SyncDirectionDisabled      = "disabled"
)
