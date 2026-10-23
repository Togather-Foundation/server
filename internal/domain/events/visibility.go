package events

// PubliclyVisible reports whether an event in the given lifecycle state may be
// served on anonymous/public surfaces (list, detail, ICS, HTML pages, MCP
// tools, change feed).
//
// The allowlist is every lifecycle state EXCEPT the system-only states:
// draft, pending_review, and deleted. Cancelled, completed, postponed,
// rescheduled, sold_out and published events stay visible — they are public
// states per docs/glossary.md. Deleted events are intentionally not public;
// they are surfaced only as 410 tombstones by the caller.
//
// This helper is the Go-side single source of truth for visibility. SQL
// predicates in the storage layer mirror it as the primary filter; callers use
// this for defence in depth.
func PubliclyVisible(state string) bool {
	switch state {
	case "draft", "pending_review", "deleted":
		return false
	case "published", "postponed", "rescheduled", "sold_out", "cancelled", "completed":
		return true
	default:
		// Unknown lifecycle states are not public: fail closed.
		return false
	}
}
