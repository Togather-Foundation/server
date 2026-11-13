package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/loadtest/fixture"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// LoadtestRunCleanupReport identifies the complete validated deletion set. Source
// rows are listed for inventory only and are always retained.
type LoadtestRunCleanupReport struct {
	RunID          string   `json:"run_id"`
	SourceIDs      []string `json:"source_ids"`
	EventIDs       []string `json:"event_ids"`
	EventULIDs     []string `json:"event_ulids"`
	AttributionIDs []string `json:"attribution_ids"`
	EventCount     int      `json:"event_count"`
	ChangeCount    int64    `json:"change_count"`
	ReviewCount    int64    `json:"review_count"`
	Applied        bool     `json:"applied"`
}

// CleanupLoadtestRun verifies every candidate while holding registry and mutation
// table locks. Preview and every failure roll back; apply deletes only the exact
// proven event set and its owned change records in the same transaction.
func (r *EventRepository) CleanupLoadtestRun(ctx context.Context, manifest fixture.Manifest, apply bool) (LoadtestRunCleanupReport, error) {
	report := LoadtestRunCleanupReport{RunID: manifest.RunID, SourceIDs: []string{}, EventIDs: []string{}, EventULIDs: []string{}, AttributionIDs: []string{}}
	if _, err := manifest.Event(1); err != nil {
		return report, fmt.Errorf("validate load-test manifest: %w", err)
	}
	if r.tx != nil {
		return report, fmt.Errorf("load-test cleanup requires its own transaction")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return report, fmt.Errorf("begin load-test cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := New(tx)
	if err := q.LockSourceRegistryExclusive(ctx); err != nil {
		return report, fmt.Errorf("lock source registry for run cleanup: %w", err)
	}
	if err := q.LockLoadtestRunTables(ctx); err != nil {
		return report, fmt.Errorf("lock run cleanup tables: %w", err)
	}
	canonical, err := q.GetCanonicalSourcePolicy(ctx, manifest.SourceURL)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return report, fmt.Errorf("resolve run source: %w", err)
	}
	var sourceID pgtype.UUID
	if err == nil {
		if err := sourceID.Scan(canonical.ID); err != nil {
			return report, fmt.Errorf("parse run source UUID: %w", err)
		}
		report.SourceIDs = append(report.SourceIDs, canonical.ID)
	}
	candidates, err := q.ListLoadtestRunCandidates(ctx, manifest.SourceURL)
	if err != nil {
		return report, fmt.Errorf("list run candidates: %w", err)
	}
	ids := make([]pgtype.UUID, 0, len(candidates))
	seenEvents, seenExternalIDs := map[string]bool{}, map[string]bool{}
	for _, candidate := range candidates {
		event, attribution := candidate.Event, candidate.EventSource
		id := uuid.UUID(event.ID.Bytes).String()
		if seenEvents[id] || seenExternalIDs[attribution.SourceEventID.String] {
			return report, fmt.Errorf("refuse ambiguous run attribution for event %s", id)
		}
		seenEvents[id], seenExternalIDs[attribution.SourceEventID.String] = true, true
		if !sourceID.Valid || attribution.SourceID != sourceID || attribution.SourceUrl != manifest.SourceURL || !attribution.SourceEventID.Valid {
			return report, fmt.Errorf("refuse unowned source attribution for event %s", id)
		}
		input, err := decodeLoadtestInput(attribution.Payload)
		if err != nil || input.Source == nil || input.Source.EventID != attribution.SourceEventID.String {
			return report, fmt.Errorf("refuse invalid run payload for event %s", id)
		}
		if err := manifest.Verify(input); err != nil {
			return report, fmt.Errorf("refuse unowned run payload for event %s: %w", id, err)
		}
		input = events.NormalizeEventInput(input)
		if err := verifyLoadtestCanonical(event, input); err != nil {
			return report, fmt.Errorf("refuse modified run event %s: %w", id, err)
		}
		allSources, err := q.ListLoadtestRunEventSources(ctx, event.ID)
		if err != nil {
			return report, fmt.Errorf("list event attributions: %w", err)
		}
		if len(allSources) != 1 || allSources[0].ID != attribution.ID {
			return report, fmt.Errorf("refuse shared run event %s", id)
		}
		occurrences, err := q.ListLoadtestRunOccurrences(ctx, event.ID)
		if err != nil {
			return report, fmt.Errorf("list run occurrences: %w", err)
		}
		if len(occurrences) != 1 || !loadtestOccurrenceMatches(occurrences[0], input) {
			return report, fmt.Errorf("refuse modified run occurrences for event %s", id)
		}
		lifecycle, err := q.GetLoadtestRunLifecycle(ctx, event.ID)
		if err != nil {
			return report, fmt.Errorf("inspect run lifecycle: %w", err)
		}
		if lifecycle.ChangeCount != 1 || lifecycle.OwnedCreateCount != 1 || lifecycle.FieldCount != 0 || lifecycle.ProtectedReference {
			return report, fmt.Errorf("refuse protected or modified run lifecycle for event %s", id)
		}
		reviews, err := q.ListLoadtestRunReviews(ctx, event.ID)
		if err != nil {
			return report, fmt.Errorf("list run reviews: %w", err)
		}
		for _, review := range reviews {
			if review.Status != "pending" || review.ReviewedBy.Valid || review.ReviewedAt.Valid || review.ReviewNotes.Valid || review.RejectionReason.Valid || review.DuplicateOfEventID.Valid {
				return report, fmt.Errorf("refuse operator-reviewed run event %s", id)
			}
			for _, payload := range [][]byte{review.OriginalPayload, review.NormalizedPayload} {
				reviewInput, err := decodeLoadtestInput(payload)
				if err != nil || manifest.Verify(reviewInput) != nil || reviewInput.Source == nil || reviewInput.Source.EventID != input.Source.EventID {
					return report, fmt.Errorf("refuse unowned review payload for event %s", id)
				}
			}
		}
		ids = append(ids, event.ID)
		report.EventIDs = append(report.EventIDs, id)
		report.EventULIDs = append(report.EventULIDs, event.Ulid)
		report.AttributionIDs = append(report.AttributionIDs, uuid.UUID(attribution.ID.Bytes).String())
		report.ChangeCount += lifecycle.ChangeCount
		report.ReviewCount += int64(len(reviews))
	}
	report.EventCount = len(ids)
	if !apply {
		return report, nil
	}
	changes, err := q.DeleteLoadtestRunChanges(ctx, ids)
	if err != nil {
		return report, fmt.Errorf("delete owned run changes: %w", err)
	}
	if changes != report.ChangeCount {
		return report, fmt.Errorf("run change deletion count changed; refusing cleanup")
	}
	deleted, err := q.DeleteLoadtestRunEvents(ctx, ids)
	if err != nil {
		return report, fmt.Errorf("delete owned run events: %w", err)
	}
	if deleted != int64(report.EventCount) {
		return report, fmt.Errorf("run event deletion count changed; refusing cleanup")
	}
	if err := tx.Commit(ctx); err != nil {
		return report, fmt.Errorf("commit run cleanup: %w", err)
	}
	report.Applied = true
	return report, nil
}

func decodeLoadtestInput(payload []byte) (events.EventInput, error) {
	// EventInput has a permissive custom UnmarshalJSON for interop. The plain
	// alias preserves strict top-level unknown-field rejection for owned fixtures.
	type strictInput events.EventInput
	var input strictInput
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return events.EventInput{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return events.EventInput{}, fmt.Errorf("unexpected trailing payload")
	}
	return events.EventInput(input), nil
}

func verifyLoadtestCanonical(event Event, input events.EventInput) error {
	if input.VirtualLocation == nil || event.Name != input.Name || event.Description.String != input.Description || event.PublicUrl.String != input.URL || event.VirtualUrl.String != input.VirtualLocation.URL || event.ImageUrl.String != input.Image || event.LicenseUrl != input.License || event.LicenseStatus != "cc0" {
		return fmt.Errorf("fixture identity differs")
	}
	if event.LifecycleState != "published" && event.LifecycleState != "pending_review" {
		return fmt.Errorf("fixture lifecycle differs")
	}
	if event.OrganizerID.Valid || event.PrimaryVenueID.Valid || event.SeriesID.Valid || event.OriginNodeID.Valid || event.FederationUri.Valid || event.MergedIntoID.Valid || event.DeletedAt.Valid || event.DeletionReason.Valid || event.TakedownRequested || event.TakedownRequestedAt.Valid || event.TakedownRequestNotes.Valid || event.Version != 1 {
		return fmt.Errorf("fixture has protected metadata")
	}
	if !reflect.DeepEqual(event.Keywords, input.Keywords) && len(event.Keywords)+len(input.Keywords) != 0 {
		return fmt.Errorf("fixture keywords differ")
	}
	if !reflect.DeepEqual(event.InLanguage, input.InLanguage) && len(event.InLanguage)+len(input.InLanguage) != 0 {
		return fmt.Errorf("fixture languages differ")
	}
	return nil
}

func loadtestOccurrenceMatches(occurrence EventOccurrence, input events.EventInput) bool {
	start, startErr := time.Parse(time.RFC3339, input.StartDate)
	end, endErr := time.Parse(time.RFC3339, input.EndDate)
	return startErr == nil && endErr == nil && occurrence.StartTime.Valid && occurrence.StartTime.Time.Equal(start) && occurrence.EndTime.Valid && occurrence.EndTime.Time.Equal(end) && occurrence.VirtualUrl.String == input.VirtualLocation.URL && !occurrence.VenueID.Valid && !occurrence.IsAllDay && !occurrence.DoorTime.Valid && !occurrence.StatusOverride.Valid && !occurrence.CancellationReason.Valid && !occurrence.TicketUrl.Valid && !occurrence.PriceMin.Valid && !occurrence.PriceMax.Valid && !occurrence.PriceCurrency.Valid && !occurrence.Availability.Valid
}
