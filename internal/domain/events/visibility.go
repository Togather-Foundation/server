package events

// PublicStates is the single source of truth for lifecycle states that may be
// served on anonymous/public surfaces (list, detail, ICS, HTML pages, MCP
// tools, change feed).
//
// It is a fixed-size array so it cannot be appended to accidentally. The set is
// every lifecycle state EXCEPT the system-only states draft, pending_review and
// deleted. Cancelled, completed, postponed, rescheduled, sold_out and published
// events stay visible — they are public states per docs/glossary.md.
//
// SQL call sites pass this slice as a `text[]` parameter instead of repeating
// the allowlist literal; query-parameter validation (isAllowedLifecycleState)
// is a separate concern and intentionally differs from this visibility list.
var PublicStates = [...]string{
	"published",
	"postponed",
	"rescheduled",
	"sold_out",
	"cancelled",
	"completed",
}

// PubliclyVisible reports whether an event in the given lifecycle state may be
// served publicly. Deleted events are intentionally not public; they are
// surfaced only as 410 tombstones by the caller.
func PubliclyVisible(state string) bool {
	for _, public := range PublicStates {
		if state == public {
			return true
		}
	}
	// Draft, pending_review, deleted and unknown states are not public: fail closed.
	return false
}
