package identity

import (
	"context"
	"fmt"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

// TidyStats reports the outcome of an identity tidy pass. EntitiesScanned counts
// distinct (entity_type, entity_id) entities with at least one identifier group;
// PrimariesFilled counts groups that had no primary and received one; RowsDemoted
// counts identifier rows whose is_primary flag was cleared (never deleted).
type TidyStats struct {
	EntitiesScanned int64 `json:"entities_scanned"`
	PrimariesFilled int64 `json:"primaries_filled"`
	RowsDemoted     int64 `json:"rows_demoted"`
}

// Tidy scans every (entity_type, entity_id, authority_code) identifier group for
// places and organizations and repairs primary-slot drift:
//
//  1. A group with observations but no primary gets one elected (canonical rank).
//  2. A group with more than one primary (legacy/pre-index state) gets all but
//     the canonical winner demoted.
//
// A group with exactly one primary — even if that primary is not the canonical
// winner — is intentionally left untouched: single-primary-but-non-canonical
// rank drift is out of scope for tidy (only fill-missing and demote-extras are
// repaired). Events/persons are never scanned (Phase 4).
//
// Rows are never deleted — only demoted (is_primary=false, superseded_by_id set).
//
// typ scopes the scan to a single entity type; the zero value ("") scans both
// place and organization. When apply is false, nothing is written and the
// returned stats report what a subsequent --apply would change (dry-run).
// The operation is idempotent: a second --apply reports zero changes.
func (s *Store) Tidy(ctx context.Context, typ EntityType, apply bool) (TidyStats, error) {
	if typ != "" && typ != EntityTypePlace && typ != EntityTypeOrganization {
		return TidyStats{}, fmt.Errorf("%w: %q", ErrInvalidEntityType, typ)
	}

	q := postgres.New(s.pool)
	groups, err := q.ListIdentityGroups(ctx, pgtype.Text{String: string(typ), Valid: typ != ""})
	if err != nil {
		return TidyStats{}, fmt.Errorf("identity: list groups: %w", err)
	}

	var stats TidyStats
	entities := make(map[[2]string]struct{}, len(groups))
	for _, g := range groups {
		entities[[2]string{g.EntityType, g.EntityID}] = struct{}{}
		if g.PrimaryCount == 0 || g.PrimaryCount > 1 {
			filled, demoted, err := s.repairGroup(ctx, g.EntityType, g.EntityID, g.AuthorityCode, apply)
			if err != nil {
				return TidyStats{}, err
			}
			stats.PrimariesFilled += filled
			stats.RowsDemoted += demoted
		}
	}
	stats.EntitiesScanned = int64(len(entities))
	return stats, nil
}

// repairGroup elects the canonical primary for one (entity, authority) group and
// — when apply is true and the group is actually drifted — demotes extras and
// sets the winner. It returns the number of primary slots filled and rows
// demoted. Dry-run (apply=false) performs the same locking read and election but
// writes nothing.
func (s *Store) repairGroup(ctx context.Context, entityType, entityID, authority string, apply bool) (filled, demoted int64, err error) {
	ref := IdentityRef{Type: EntityType(entityType), ULID: entityID}
	err = s.WithTx(ctx, func(q *postgres.Queries) error {
		if err := q.LockIdentityGroup(ctx, identityLockKey(ref, authority)); err != nil {
			return fmt.Errorf("identity: acquire group lock: %w", err)
		}

		rows, err := q.ListGroupIdentifiersForUpdate(ctx, postgres.ListGroupIdentifiersForUpdateParams{
			EntityType:    entityType,
			EntityID:      entityID,
			AuthorityCode: authority,
		})
		if err != nil {
			return fmt.Errorf("identity: load group: %w", err)
		}

		observations := make([]IdentifierObservation, 0, len(rows))
		for _, row := range rows {
			observations = append(observations, IdentifierObservation{
				ID:         row.ID,
				Authority:  row.AuthorityCode,
				URI:        row.IdentifierUri,
				Method:     row.ReconciliationMethod,
				Confidence: numericToFloat64(row.Confidence),
				ObservedAt: row.ObservedAt.Time,
				TrustLevel: row.TrustLevel,
				Priority:   row.PriorityOrder,
				IsPrimary:  row.IsPrimary,
				Source:     row.Source.String,
			})
		}

		winner, ok := ElectPrimary(observations)
		if !ok {
			return fmt.Errorf("identity: empty group during tidy")
		}

		// Count repairs from the fresh, locked rows (not the pre-lock listing),
		// so a group already repaired by a concurrent writer is neither counted
		// nor rewritten.
		var primaryCount int64
		for _, o := range observations {
			if o.IsPrimary {
				primaryCount++
			}
		}

		// Scope: only fill-missing (0 primaries) and demote-extras (>1 primary)
		// are repaired. A single-primary group is left as-is even when that
		// primary is not the canonical winner (out of scope), and a group that
		// drifted to healthy between the scan and this lock is a no-op.
		needsRepair := primaryCount != 1
		switch {
		case primaryCount == 0:
			filled = 1
		case primaryCount > 1:
			if winner.IsPrimary {
				demoted = primaryCount - 1
			} else {
				demoted = primaryCount
			}
		}

		if !apply || !needsRepair {
			return nil
		}

		if err := q.DemotePrimaryAndSupersede(ctx, postgres.DemotePrimaryAndSupersedeParams{
			WinnerID:      pgtype.Int4{Int32: winner.ID, Valid: true},
			EntityType:    entityType,
			EntityID:      entityID,
			AuthorityCode: authority,
		}); err != nil {
			return fmt.Errorf("identity: demote primary: %w", err)
		}

		if err := q.SetPrimary(ctx, winner.ID); err != nil {
			return fmt.Errorf("identity: set primary: %w", err)
		}
		return nil
	})
	return filled, demoted, err
}
