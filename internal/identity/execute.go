package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Structural (BadRequest-class) errors surfaced by the Executor. These must be
// mapped to RFC 7807 400 responses by the HTTP/CLI layer (Task 5); no decision
// is written for any of them.
var (
	// ErrEmptyActor is returned when the actor (JWT subject) is empty.
	ErrEmptyActor = errors.New("actor must not be empty")
	// ErrInvalidURI is returned when a URI does not match its authority's pattern.
	ErrInvalidURI = errors.New("identifier URI does not match the authority pattern")
	// ErrUnknownAuthority is returned when the authority code is not registered.
	ErrUnknownAuthority = errors.New("unknown authority")
	// ErrMismatchedEntityTypes is returned when a pair spans two entity types.
	ErrMismatchedEntityTypes = errors.New("pair must reference a single entity type")
	// ErrSameEntityPair is returned when both sides of a pair are the same entity.
	ErrSameEntityPair = errors.New("pair must reference two distinct entities")
)

// Executor performs identity link/reject actions. LinkIdentifier records an
// external identifier observation and elects the primary atomically with a
// decision append; Reject records a not-duplicate suppression with an evidence
// fingerprint atomically with a decision append.
type Executor struct {
	tx        TxManager
	ids       IdentifierStore
	decisions DecisionStore
}

// NewExecutor assembles an Executor from its collaborators.
func NewExecutor(tx TxManager, ids IdentifierStore, decisions DecisionStore) *Executor {
	return &Executor{tx: tx, ids: ids, decisions: decisions}
}

// LinkIdentifier records/confirms an external identifier observation for one
// SEL entity and elects the primary, appending a decision record in the same
// transaction. It validates the actor, entity type, and URI against the
// authority's base_uri_pattern before writing anything; a structural failure
// writes no decision and no observation.
func (e *Executor) LinkIdentifier(ctx context.Context, ref IdentityRef, obs IdentifierObservation, actor string) (DecisionRecord, error) {
	if strings.TrimSpace(actor) == "" {
		return DecisionRecord{}, ErrEmptyActor
	}
	if ref.Type != EntityTypePlace && ref.Type != EntityTypeOrganization {
		return DecisionRecord{}, fmt.Errorf("%w: %q", ErrInvalidEntityType, ref.Type)
	}
	if strings.TrimSpace(obs.Authority) == "" || strings.TrimSpace(obs.URI) == "" {
		return DecisionRecord{}, ErrInvalidURI
	}

	var rec DecisionRecord
	err := e.tx.WithTx(ctx, func(q *postgres.Queries) error {
		authority, err := q.GetAuthorityByCode(ctx, obs.Authority)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("identity: authority %q: %w", obs.Authority, ErrUnknownAuthority)
			}
			return fmt.Errorf("identity: lookup authority %q: %w", obs.Authority, err)
		}
		if err := validateAuthorityURI(authority.BaseUriPattern, obs.URI); err != nil {
			return err
		}

		if _, err := e.ids.RecordObservationTx(ctx, q, ref, obs); err != nil {
			return err
		}

		rec = DecisionRecord{
			EntityType: ref.Type,
			EntityID:   ref.ULID,
			Action:     ActionLink,
			Citations:  []string{},
			Confidence: obs.Confidence,
			Actor:      actor,
			Reversible: true,
			Metadata: map[string]any{
				"authority": obs.Authority,
				"uri":       obs.URI,
				"method":    obs.Method,
				"source":    obs.Source,
			},
		}
		rec, err = e.decisions.AppendTx(ctx, q, rec)
		return err
	})
	if err != nil {
		return DecisionRecord{}, err
	}
	return rec, nil
}

// Reject records that two entities are judged distinct. It computes the
// signal-scoped evidence fingerprint from the pair's current shared identifiers
// and upserts an identity_not_duplicates row (canonical id_a < id_b ordering)
// together with a decision record, all in one transaction. Re-rejecting a pair
// after its evidence changes refreshes the stored fingerprint, re-opening the
// pair for future conflict detection.
func (e *Executor) Reject(ctx context.Context, a, b IdentityRef, actor, reason string) (DecisionRecord, error) {
	if strings.TrimSpace(actor) == "" {
		return DecisionRecord{}, ErrEmptyActor
	}
	if a.Type != EntityTypePlace && a.Type != EntityTypeOrganization {
		return DecisionRecord{}, fmt.Errorf("%w: %q", ErrInvalidEntityType, a.Type)
	}
	if b.Type != EntityTypePlace && b.Type != EntityTypeOrganization {
		return DecisionRecord{}, fmt.Errorf("%w: %q", ErrInvalidEntityType, b.Type)
	}
	if a.Type != b.Type {
		return DecisionRecord{}, ErrMismatchedEntityTypes
	}
	if a.ULID == b.ULID {
		return DecisionRecord{}, ErrSameEntityPair
	}

	idA, idB := canonicalPair(a.ULID, b.ULID)

	var rec DecisionRecord
	err := e.tx.WithTx(ctx, func(q *postgres.Queries) error {
		// Read identifiers in canonical order so concurrent Rejects of the same
		// pair with swapped arguments lock rows in the same sequence.
		first, second := a, b
		if a.ULID > b.ULID {
			first, second = b, a
		}
		obsFirst, err := e.loadIdentifiersTx(ctx, q, first)
		if err != nil {
			return err
		}
		obsSecond, err := e.loadIdentifiersTx(ctx, q, second)
		if err != nil {
			return err
		}

		fp := Fingerprint(obsFirst, obsSecond)

		rec = DecisionRecord{
			EntityType:      a.Type,
			EntityID:        idA,
			Action:          ActionReject,
			CounterpartType: &a.Type,
			CounterpartID:   &idB,
			Rationale:       reason,
			Citations:       []string{},
			Confidence:      0,
			Actor:           actor,
			Reversible:      true,
			Metadata:        map[string]any{"evidence_fingerprint": fp},
		}

		rec, err = e.decisions.AppendTx(ctx, q, rec)
		if err != nil {
			return err
		}

		if err := q.InsertIdentityNotDuplicate(ctx, postgres.InsertIdentityNotDuplicateParams{
			EntityType:          string(a.Type),
			IDA:                 idA,
			IDB:                 idB,
			EvidenceFingerprint: fp,
			DecisionID:          pgtype.Text{String: rec.ID, Valid: true},
			CreatedBy:           actor,
		}); err != nil {
			return fmt.Errorf("identity: record not-duplicate: %w", err)
		}

		return nil
	})
	if err != nil {
		return DecisionRecord{}, err
	}
	return rec, nil
}

// loadIdentifiersTx loads an entity's identifier observations inside the
// caller's transaction. Only the authority and URI are materialized — the two
// fields the evidence fingerprint consumes.
func (e *Executor) loadIdentifiersTx(ctx context.Context, q *postgres.Queries, ref IdentityRef) ([]IdentifierObservation, error) {
	rows, err := q.ListEntityIdentifiersForUpdate(ctx, postgres.ListEntityIdentifiersForUpdateParams{
		EntityType: string(ref.Type),
		EntityID:   ref.ULID,
	})
	if err != nil {
		return nil, fmt.Errorf("identity: load identifiers for %s %s: %w", ref.Type, ref.ULID, err)
	}

	obs := make([]IdentifierObservation, 0, len(rows))
	for _, row := range rows {
		obs = append(obs, IdentifierObservation{
			Authority: row.AuthorityCode,
			URI:       row.IdentifierUri,
		})
	}
	return obs, nil
}

// validateAuthorityURI reports whether uri matches the authority's compiled
// base_uri_pattern. A pattern that fails to compile is an internal error; a
// non-matching URI is a structural ErrInvalidURI.
func validateAuthorityURI(pattern, uri string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("identity: invalid authority pattern %q: %w", pattern, err)
	}
	if !re.MatchString(uri) {
		return fmt.Errorf("%w: %q does not match %q", ErrInvalidURI, uri, pattern)
	}
	return nil
}

// canonicalPair orders two ULIDs so that idA < idB, matching the
// identity_not_duplicates CHECK (id_a < id_b).
func canonicalPair(x, y string) (string, string) {
	if x < y {
		return x, y
	}
	return y, x
}
