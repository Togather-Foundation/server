package federation

// Default values for federation and event processing
const (
	// DefaultEventVersion is the initial version number for new events
	DefaultEventVersion = 1

	// DefaultLifecycleState is the default lifecycle state for newly created events
	DefaultLifecycleState = "published"

	// DefaultChangeFeedLimit is the default number of items returned in change feed queries
	DefaultChangeFeedLimit = 50
)
