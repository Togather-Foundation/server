package identity

import "time"

// IdentifierObservation is a single external-identifier observation for a SEL
// entity. Multiple observations for the same (entity, authority) form a group;
// ElectPrimary elects the single row that holds the group's primary slot.
type IdentifierObservation struct {
	ID         int32     // entity_identifiers.id (SERIAL)
	Authority  string    // authority code, e.g. "artsdata"
	URI        string    // external identifier URI
	Method     string    // manual|imported|auto_high|auto_low|enrichment_sameas
	Confidence float64   // 0.0–1.0
	ObservedAt time.Time // entity_identifiers.observed_at
	TrustLevel int32     // knowledge_graph_authorities.trust_level
	Priority   int32     // knowledge_graph_authorities.priority_order (lower wins)
	IsPrimary  bool      // current primary-slot state (read path)
	Source     string    // provenance: reconciliation|enrichment_sameas|manual|agent
}

// methodRank maps a reconciliation method to its canonical rank (higher wins).
// This is the single source of truth for the method tier of the election order;
// it mirrors the CASE in migration 000051's backfill.
func methodRank(method string) int {
	switch method {
	case "manual":
		return 5
	case "imported":
		return 4
	case "auto_high":
		return 3
	case "auto_low":
		return 2
	case "enrichment_sameas":
		return 1
	default:
		return 0
	}
}

// less reports whether a is strictly lower-ranked than b under the canonical
// election order (plan.md § Component Design):
//
//  1. method rank (manual > imported > auto_high > auto_low > enrichment_sameas)
//  2. authority trust_level DESC
//  3. authority priority_order ASC (lower wins)
//  4. confidence DESC
//  5. observed_at DESC (newest)
//  6. id DESC (terminal deterministic tie-break)
func less(a, b IdentifierObservation) bool {
	if ra, rb := methodRank(a.Method), methodRank(b.Method); ra != rb {
		return ra < rb
	}
	if a.TrustLevel != b.TrustLevel {
		return a.TrustLevel < b.TrustLevel
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if a.Confidence != b.Confidence {
		return a.Confidence < b.Confidence
	}
	if !a.ObservedAt.Equal(b.ObservedAt) {
		return a.ObservedAt.Before(b.ObservedAt)
	}
	return a.ID < b.ID
}

// ElectPrimary returns the observation that should hold the primary slot for a
// group, per the canonical order. It returns (zero, false) when obs is empty.
func ElectPrimary(obs []IdentifierObservation) (IdentifierObservation, bool) {
	if len(obs) == 0 {
		return IdentifierObservation{}, false
	}

	winner := obs[0]
	for i := 1; i < len(obs); i++ {
		if less(winner, obs[i]) {
			winner = obs[i]
		}
	}
	return winner, true
}
