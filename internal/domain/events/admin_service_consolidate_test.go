package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
)

// Valid 26-char ULIDs for consolidate tests (generated, not sequential).
const (
	consolidateCanonULID  = "01KM1B4HXHZ7G8RZW4WDYCXRW9"
	consolidateRetireULID = "01KM1B4HXHKDJMETBS0D5C9HK2"
	consolidateRetire2    = "01KM1B4HXHMYFBVJ2X94RSA8RV"
	consolidateThirdULID  = "01KM1B4HXHPYAFEGKW7K3BJNTQ"
)

const consolidateBaseURL = "https://toronto.togather.foundation"

// newConsolidateSvc returns an AdminService with a NearDuplicateThreshold set
// via DedupConfig on the embedded IngestService — the single source of truth
// for the post-consolidation near-dup check.
func newConsolidateSvc(repo Repository, nearDupThreshold float64) *AdminService {
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{AllowTestDomains: true}, consolidateBaseURL)
	ingest := NewIngestService(repo, "", "America/Toronto", config.ValidationConfig{AllowTestDomains: true}).
		WithDedupConfig(config.DedupConfig{NearDuplicateThreshold: nearDupThreshold})
	svc.WithIngestService(ingest)
	return svc
}

// makeConsolidateRepo builds a minimal mockTransactionalRepo for Consolidate tests.
func makeConsolidateRepo(knownEvents map[string]*Event) *mockTransactionalRepo {
	return &mockTransactionalRepo{
		getByULIDFunc: func(_ context.Context, ulid string) (*Event, error) {
			if ev, ok := knownEvents[ulid]; ok {
				return ev, nil
			}
			return nil, ErrNotFound
		},
		lockEventForUpdateFunc: func(_ context.Context, _ string) error {
			return nil
		},
		softDeleteEventFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		createTombstoneFunc: func(_ context.Context, _ TombstoneCreateParams) error {
			return nil
		},
		dismissPendingReviewsByEventULIDsFunc: func(_ context.Context, _ []string, _ string) ([]int, error) {
			return nil, nil
		},
	}
}

// makePublishedEvent returns a minimal Event in "published" state.
func makePublishedEvent(id, ulid, name string) *Event {
	venueID := "venue-uuid-1"
	venueName := "The Tranzac"
	return &Event{
		ID:               id,
		ULID:             ulid,
		Name:             name,
		LifecycleState:   "published",
		PrimaryVenueID:   &venueID,
		PrimaryVenueName: &venueName,
		Occurrences:      []Occurrence{{StartTime: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)}},
	}
}

// ── Validation error tests ────────────────────────────────────────────────────

// TestConsolidate_NeitherEventField_Error verifies that omitting both event and
// event_ulid returns ErrConsolidateNoEventField.
func TestConsolidate_NeitherEventField_Error(t *testing.T) {
	ctx := context.Background()
	repo := makeConsolidateRepo(nil)
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		Retire: []string{consolidateRetireULID},
	})
	if !errors.Is(err, ErrConsolidateNoEventField) {
		t.Errorf("expected ErrConsolidateNoEventField, got: %v", err)
	}
}

func TestConsolidate_EmptyRetire_Error(t *testing.T) {
	ctx := context.Background()
	repo := makeConsolidateRepo(nil)
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{},
	})
	if !errors.Is(err, ErrConsolidateNoRetire) {
		t.Errorf("expected ErrConsolidateNoRetire, got: %v", err)
	}
}

func TestConsolidate_CanonicalInRetire_Error(t *testing.T) {
	ctx := context.Background()
	repo := makeConsolidateRepo(nil)
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateCanonULID},
	})
	if !errors.Is(err, ErrConsolidateCanonicalInRetire) {
		t.Errorf("expected ErrConsolidateCanonicalInRetire, got: %v", err)
	}
}

// ── Promote path (existing event as canonical) ────────────────────────────────

func TestConsolidate_PromotePath_HappyPath(t *testing.T) {
	ctx := context.Background()

	known := map[string]*Event{
		consolidateCanonULID:  makePublishedEvent("uuid-canon", consolidateCanonULID, "Canon Event"),
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Event == nil || result.Event.ULID != consolidateCanonULID {
		t.Errorf("expected canonical event ULID %s, got: %+v", consolidateCanonULID, result.Event)
	}
	if len(result.Retired) != 1 || result.Retired[0] != consolidateRetireULID {
		t.Errorf("expected retired=[%s], got: %v", consolidateRetireULID, result.Retired)
	}
	if !repo.commitCalled {
		t.Error("Commit must be called on success")
	}
}

// ── Create path (new canonical event) ────────────────────────────────────────

// consolidateCreateRepo wraps mockTransactionalRepo and overrides Create so
// the create path in Consolidate can succeed.
type consolidateCreateRepo struct {
	*mockTransactionalRepo
	createFunc func(ctx context.Context, params EventCreateParams) (*Event, error)
}

func (r *consolidateCreateRepo) Create(ctx context.Context, params EventCreateParams) (*Event, error) {
	if r.createFunc != nil {
		return r.createFunc(ctx, params)
	}
	return nil, errors.New("not implemented")
}

func (r *consolidateCreateRepo) BeginTx(ctx context.Context) (Repository, TxCommitter, error) {
	return r, &consolidateTxCommitter{repo: r.mockTransactionalRepo}, nil
}

type consolidateTxCommitter struct {
	repo *mockTransactionalRepo
}

func (c *consolidateTxCommitter) Commit(ctx context.Context) error {
	c.repo.commitCalled = true
	return nil
}

func (c *consolidateTxCommitter) Rollback(ctx context.Context) error {
	c.repo.rollbackCalled = true
	return nil
}

func TestConsolidate_CreatePath_HappyPath(t *testing.T) {
	ctx := context.Background()

	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	knownByULID := map[string]*Event{
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}

	base := makeConsolidateRepo(knownByULID)
	// After Create, GetByULID for any unknown ULID returns a fresh event (the newly created one).
	base.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ev, ok := knownByULID[ulid]; ok {
			return ev, nil
		}
		return makePublishedEvent("uuid-new", ulid, "New Canon Event"), nil
	}
	base.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		return nil
	}

	custom := &consolidateCreateRepo{
		mockTransactionalRepo: base,
		createFunc: func(_ context.Context, params EventCreateParams) (*Event, error) {
			return &Event{
				ID:             "uuid-new",
				ULID:           params.ULID,
				Name:           params.Name,
				LifecycleState: params.LifecycleState,
			}, nil
		},
	}

	svc := NewAdminService(custom, false, "America/Toronto", config.ValidationConfig{
		AllowTestDomains: true,
	}, consolidateBaseURL)

	input := EventInput{
		Name:      "New Canon Event",
		StartDate: now.Format(time.RFC3339),
		EndDate:   now.Add(2 * time.Hour).Format(time.RFC3339),
		VirtualLocation: &VirtualLocationInput{
			URL:  "https://example-stream.togather.foundation/new-event",
			Name: "Online Stream",
		},
	}
	result, err := svc.Consolidate(ctx, ConsolidateParams{
		Event:  &input,
		Retire: []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Retired) != 1 || result.Retired[0] != consolidateRetireULID {
		t.Errorf("expected retired=[%s], got: %v", consolidateRetireULID, result.Retired)
	}
	if !custom.commitCalled {
		t.Error("Commit must be called on success")
	}
}

// ── Error cases ───────────────────────────────────────────────────────────────

func TestConsolidate_RetiredNotFound_Error(t *testing.T) {
	ctx := context.Background()

	repo := makeConsolidateRepo(map[string]*Event{
		consolidateCanonULID: makePublishedEvent("uuid-canon", consolidateCanonULID, "Canon Event"),
		// retire ULID intentionally absent → ErrNotFound
	})
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if err == nil {
		t.Fatal("expected error for missing retire ULID, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound wrapped, got: %v", err)
	}
}

func TestConsolidate_RetiredAlreadyDeleted_Error(t *testing.T) {
	ctx := context.Background()

	deleted := &Event{
		ID:             "uuid-deleted",
		ULID:           consolidateRetireULID,
		Name:           "Deleted Event",
		LifecycleState: "deleted",
	}
	repo := makeConsolidateRepo(map[string]*Event{
		consolidateCanonULID:  makePublishedEvent("uuid-canon", consolidateCanonULID, "Canon Event"),
		consolidateRetireULID: deleted,
	})
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if !errors.Is(err, ErrConsolidateRetiredAlreadyDeleted) {
		t.Errorf("expected ErrConsolidateRetiredAlreadyDeleted, got: %v", err)
	}
}

func TestConsolidate_TransactionRollbackOnRetireFailure(t *testing.T) {
	ctx := context.Background()

	known := map[string]*Event{
		consolidateCanonULID:  makePublishedEvent("uuid-canon", consolidateCanonULID, "Canon Event"),
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)
	repo.softDeleteEventFunc = func(_ context.Context, _, _ string) error {
		return fmt.Errorf("soft-delete DB error")
	}
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if err == nil {
		t.Fatal("expected error from soft-delete failure, got nil")
	}
	if repo.commitCalled {
		t.Error("Commit must not be called after soft-delete error")
	}
}

// ── Multi-retire ordering ─────────────────────────────────────────────────────

// TestConsolidate_MultiRetire verifies that all events in the retire list are
// soft-deleted and their ULIDs appear in the result, regardless of input order.
func TestConsolidate_MultiRetire(t *testing.T) {
	ctx := context.Background()

	known := map[string]*Event{
		consolidateCanonULID:  makePublishedEvent("uuid-canon", consolidateCanonULID, "Canon Event"),
		consolidateRetireULID: makePublishedEvent("uuid-r1", consolidateRetireULID, "Old Event 1"),
		consolidateRetire2:    makePublishedEvent("uuid-r2", consolidateRetire2, "Old Event 2"),
	}
	repo := makeConsolidateRepo(known)

	var deletedULIDs []string
	repo.softDeleteEventFunc = func(_ context.Context, ulid, _ string) error {
		deletedULIDs = append(deletedULIDs, ulid)
		return nil
	}

	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID, consolidateRetire2}, // intentionally unsorted
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(result.Retired) != 2 {
		t.Errorf("expected 2 retired ULIDs, got %d: %v", len(result.Retired), result.Retired)
	}
	if len(deletedULIDs) != 2 {
		t.Errorf("expected 2 soft-delete calls, got %d", len(deletedULIDs))
	}
}

// ── Near-dup / dedup checks ───────────────────────────────────────────────────

// consolidateNearDupULID is a ULID for a third-party event used in near-dup tests.
const consolidateNearDupULID = "01KM1B4HXHP3WQVX2AZNS7YKG5"

// makeVenueCanonEvent returns a published Event with a venue and a single occurrence,
// required for Step 7 (near-dup check) to trigger.
func makeVenueCanonEvent(id, ulid, name string, startTime time.Time) *Event {
	venueID := "venue-uuid-1"
	return &Event{
		ID:             id,
		ULID:           ulid,
		Name:           name,
		LifecycleState: "published",
		PrimaryVenueID: &venueID,
		Occurrences:    []Occurrence{{StartTime: startTime}},
	}
}

// TestConsolidate_PromotePath_SelfExcludedFromNearDup verifies that the canonical
// event itself is never returned as a near-duplicate (self-match filter, Issue 1).
func TestConsolidate_PromotePath_SelfExcludedFromNearDup(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	canon := makeVenueCanonEvent("uuid-canon", consolidateCanonULID, "Canon Event", now)
	known := map[string]*Event{
		consolidateCanonULID:     canon,
		consolidateRetireULID:    makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
		consolidateCompanionULID: makePublishedEvent("uuid-companion", consolidateCompanionULID, "Weekly Yoga"),
	}
	repo := makeConsolidateRepo(known)

	// FindNearDuplicates returns the canonical itself (self-match, similarity 1.0)
	// plus one genuine near-dup. After filtering, only the genuine near-dup should
	// trigger IsDuplicate / NeedsReview.
	repo.findNearDuplicatesFunc = func(_ context.Context, _ string, _ time.Time, _ string, _ float64) ([]NearDuplicateCandidate, error) {
		return []NearDuplicateCandidate{
			{ULID: consolidateCanonULID, Name: "Canon Event", Similarity: 1.0},        // self — must be filtered
			{ULID: consolidateNearDupULID, Name: "Canon Event Copy", Similarity: 0.9}, // genuine dup
		}, nil
	}

	svc := newConsolidateSvc(repo, 0.8)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !result.IsDuplicate {
		t.Error("expected IsDuplicate=true for genuine near-dup")
	}
	if !result.NeedsReview {
		t.Error("expected NeedsReview=true for genuine near-dup")
	}
	// Exactly one warning: the genuine near-dup. Self-match must not appear.
	// Warning format matches createEventCore (shared via runDedupWarningChecks).
	dupWarnings := 0
	for _, w := range result.Warnings {
		if w.Code == "potential_duplicate" {
			dupWarnings++
			if w.Message != "Potential duplicate: found 1 similar event(s) at the same venue on the same date" {
				t.Errorf("unexpected warning message: %s", w.Message)
			}
			matches, ok := w.Details["matches"].([]map[string]any)
			if !ok {
				t.Error("potential_duplicate warning missing matches array in Details")
				continue
			}
			if len(matches) != 1 {
				t.Errorf("expected 1 match, got %d: %+v", len(matches), matches)
			} else if matches[0]["ulid"] != consolidateNearDupULID {
				t.Errorf("match[0].ulid = %v, want %s", matches[0]["ulid"], consolidateNearDupULID)
			}
		}
	}
	if dupWarnings != 1 {
		t.Errorf("expected exactly 1 potential_duplicate warning (not %d); warnings: %+v", dupWarnings, result.Warnings)
	}
}

// TestConsolidate_PromotePath_SelfOnlyNearDup verifies that when FindNearDuplicates
// returns only the canonical event (pure self-match), no duplicate flag is set.
func TestConsolidate_PromotePath_SelfOnlyNearDup(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	canon := makeVenueCanonEvent("uuid-canon", consolidateCanonULID, "Canon Event", now)
	known := map[string]*Event{
		consolidateCanonULID:  canon,
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)

	// Only the canonical itself is returned — after filtering, zero candidates remain.
	repo.findNearDuplicatesFunc = func(_ context.Context, _ string, _ time.Time, _ string, _ float64) ([]NearDuplicateCandidate, error) {
		return []NearDuplicateCandidate{
			{ULID: consolidateCanonULID, Name: "Canon Event", Similarity: 1.0},
		}, nil
	}

	svc := newConsolidateSvc(repo, 0.8)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result.IsDuplicate {
		t.Error("expected IsDuplicate=false when only self returned by FindNearDuplicates")
	}
	if result.NeedsReview {
		t.Error("expected NeedsReview=false when only self returned by FindNearDuplicates")
	}
}

// TestConsolidate_NearDupCheckFailure_NonFatal verifies that a FindNearDuplicates
// DB error does not abort the consolidation (Issue 2 — mirrors ingest.go behaviour).
func TestConsolidate_NearDupCheckFailure_NonFatal(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	canon := makeVenueCanonEvent("uuid-canon", consolidateCanonULID, "Canon Event", now)
	known := map[string]*Event{
		consolidateCanonULID:  canon,
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)

	repo.findNearDuplicatesFunc = func(_ context.Context, _ string, _ time.Time, _ string, _ float64) ([]NearDuplicateCandidate, error) {
		return nil, errors.New("simulated DB failure")
	}

	svc := newConsolidateSvc(repo, 0.8)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	// Must succeed despite FindNearDuplicates error.
	if err != nil {
		t.Fatalf("expected non-fatal near-dup failure, got error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// No duplicate flag set because the check failed — benefit of the doubt.
	if result.IsDuplicate {
		t.Error("expected IsDuplicate=false when near-dup check errors")
	}
	if !repo.commitCalled {
		t.Error("Commit must be called on success")
	}
}

// consolidateCompanionULID is a ULID for an existing companion event used in
// cross-week series tests.
const consolidateCompanionULID = "01KM1B4HXHW4ZQVP3RT9DMSX82"

// TestConsolidate_PromotePath_CrossWeekSeriesCompanion verifies that when a
// consolidated event (promote path) has a cross-week series companion, the
// result is flagged for review with a cross_week_series_companion warning.
// This mirrors Step 8b in createEventCore.
func TestConsolidate_PromotePath_CrossWeekSeriesCompanion(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	canon := makeVenueCanonEvent("uuid-canon", consolidateCanonULID, "Weekly Yoga", now)
	known := map[string]*Event{
		consolidateCanonULID:     canon,
		consolidateRetireULID:    makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
		consolidateCompanionULID: makePublishedEvent("uuid-companion", consolidateCompanionULID, "Weekly Yoga"),
	}
	repo := makeConsolidateRepo(known)

	repo.findSeriesCompanionFunc = func(_ context.Context, params SeriesCompanionQuery) (*CrossWeekCompanion, error) {
		return &CrossWeekCompanion{
			ULID:      consolidateCompanionULID,
			Name:      "Weekly Yoga",
			StartDate: now.Add(14 * 24 * time.Hour).Format(time.RFC3339),
			StartTime: "10:00:00",
			VenueName: "Community Centre",
		}, nil
	}

	svc := newConsolidateSvc(repo, 0.8)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsDuplicate {
		t.Error("expected IsDuplicate=true when cross-week companion found")
	}
	if !result.NeedsReview {
		t.Error("expected NeedsReview=true when cross-week companion found")
	}

	found := false
	for _, w := range result.Warnings {
		if w.Code == "cross_week_series_companion" {
			found = true
			if w.Message == "" {
				t.Error("cross_week_series_companion warning must have a non-empty message")
			}
			if w.Details == nil {
				t.Error("cross_week_series_companion warning must have details")
			}
			if w.Details["companion_ulid"] != consolidateCompanionULID {
				t.Errorf("companion_ulid detail = %v, want %s", w.Details["companion_ulid"], consolidateCompanionULID)
			}
		}
	}
	if !found {
		t.Error("expected cross_week_series_companion warning in result.Warnings")
	}
}

// TestConsolidate_PromotePath_CrossWeekCompanionInRetireList_NotFlagged verifies
// that a companion event that is also being retired in this consolidation does
// NOT trigger a cross_week_series_companion warning.
func TestConsolidate_PromotePath_CrossWeekCompanionInRetireList_NotFlagged(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	canon := makeVenueCanonEvent("uuid-canon", consolidateCanonULID, "Weekly Yoga", now)
	known := map[string]*Event{
		consolidateCanonULID:  canon,
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)

	repo.findSeriesCompanionFunc = func(_ context.Context, _ SeriesCompanionQuery) (*CrossWeekCompanion, error) {
		return &CrossWeekCompanion{
			ULID: consolidateRetireULID, // companion is in the retire list
		}, nil
	}

	svc := newConsolidateSvc(repo, 0.8)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsDuplicate {
		t.Error("expected IsDuplicate=false when companion is in retire list")
	}
	if result.NeedsReview {
		t.Error("expected NeedsReview=false when companion is in retire list")
	}

	for _, w := range result.Warnings {
		if w.Code == "cross_week_series_companion" {
			t.Error("cross_week_series_companion warning must not appear for companion in retire list")
		}
	}
}

// TestConsolidate_CreatePath_DedupHashMatch_FlagsAsReview verifies that when a
// newly created canonical event has the same dedup hash as an existing non-retired
// event, the result is flagged for review with an "exact_duplicate" warning (Issue 3).
func TestConsolidate_CreatePath_DedupHashMatch_FlagsAsReview(t *testing.T) {
	ctx := context.Background()

	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	existingMatchULID := "01KM1B4HXHQ9VBRZ0CDMPWX8T3"

	knownByULID := map[string]*Event{
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}

	base := makeConsolidateRepo(knownByULID)
	base.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ev, ok := knownByULID[ulid]; ok {
			return ev, nil
		}
		return makePublishedEvent("uuid-new", ulid, "New Canon Event"), nil
	}
	base.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		return nil
	}
	// FindByDedupHash returns an existing event with a different ULID (not the new one,
	// not in the retire list) — should trigger the exact_duplicate warning.
	base.findByDedupHashFunc = func(_ context.Context, _ string) (*Event, error) {
		return makePublishedEvent("uuid-existing", existingMatchULID, "New Canon Event"), nil
	}

	custom := &consolidateCreateRepo{
		mockTransactionalRepo: base,
		createFunc: func(_ context.Context, params EventCreateParams) (*Event, error) {
			return &Event{
				ID:             "uuid-new",
				ULID:           params.ULID,
				Name:           params.Name,
				LifecycleState: params.LifecycleState,
			}, nil
		},
	}

	svc := NewAdminService(custom, false, "America/Toronto", config.ValidationConfig{
		AllowTestDomains: true,
	}, consolidateBaseURL)

	input := EventInput{
		Name:        "New Canon Event",
		Description: "A clear description to avoid quality warnings.",
		StartDate:   now.Format(time.RFC3339),
		EndDate:     now.Add(2 * time.Hour).Format(time.RFC3339),
		VirtualLocation: &VirtualLocationInput{
			URL:  "https://example-stream.togather.foundation/new-event",
			Name: "Online Stream",
		},
	}
	result, err := svc.Consolidate(ctx, ConsolidateParams{
		Event:  &input,
		Retire: []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !result.IsDuplicate {
		t.Error("expected IsDuplicate=true for exact dedup hash match")
	}
	if !result.NeedsReview {
		t.Error("expected NeedsReview=true for exact dedup hash match")
	}
	exactDupWarnings := 0
	for _, w := range result.Warnings {
		if w.Code == "exact_duplicate" {
			exactDupWarnings++
		}
	}
	if exactDupWarnings != 1 {
		t.Errorf("expected exactly 1 exact_duplicate warning, got %d; warnings: %+v", exactDupWarnings, result.Warnings)
	}
}

// TestConsolidate_CreatePath_DedupHashMatch_RetiredExcluded verifies that when
// the dedup hash matches an event that is in the retire list, no duplicate warning
// is emitted (the admin is explicitly retiring the old event).
func TestConsolidate_CreatePath_DedupHashMatch_RetiredExcluded(t *testing.T) {
	ctx := context.Background()

	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	knownByULID := map[string]*Event{
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}

	base := makeConsolidateRepo(knownByULID)
	base.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ev, ok := knownByULID[ulid]; ok {
			return ev, nil
		}
		return makePublishedEvent("uuid-new", ulid, "New Canon Event"), nil
	}
	base.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		return nil
	}
	// FindByDedupHash returns the event being retired — should NOT trigger a warning.
	base.findByDedupHashFunc = func(_ context.Context, _ string) (*Event, error) {
		return makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"), nil
	}

	custom := &consolidateCreateRepo{
		mockTransactionalRepo: base,
		createFunc: func(_ context.Context, params EventCreateParams) (*Event, error) {
			return &Event{
				ID:             "uuid-new",
				ULID:           params.ULID,
				Name:           params.Name,
				LifecycleState: params.LifecycleState,
			}, nil
		},
	}

	svc := NewAdminService(custom, false, "America/Toronto", config.ValidationConfig{
		AllowTestDomains: true,
	}, consolidateBaseURL)

	input := EventInput{
		Name:        "New Canon Event",
		Description: "A clear description to avoid quality warnings.",
		StartDate:   now.Format(time.RFC3339),
		EndDate:     now.Add(2 * time.Hour).Format(time.RFC3339),
		VirtualLocation: &VirtualLocationInput{
			URL:  "https://example-stream.togather.foundation/new-event",
			Name: "Online Stream",
		},
	}
	result, err := svc.Consolidate(ctx, ConsolidateParams{
		Event:  &input,
		Retire: []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	// No duplicate warning — the matching event is being retired.
	if result.IsDuplicate {
		t.Error("expected IsDuplicate=false when dedup match is in retire list")
	}
	for _, w := range result.Warnings {
		if w.Code == "exact_duplicate" {
			t.Errorf("unexpected exact_duplicate warning when match is being retired: %+v", w)
		}
	}
}

// TestConsolidate_CreatePath_QualityWarnings verifies that quality warnings from
// appendQualityWarnings are applied to the create path (Issue 4). An event without
// a description should always produce a missing_description warning.
func TestConsolidate_CreatePath_QualityWarnings(t *testing.T) {
	ctx := context.Background()

	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	knownByULID := map[string]*Event{
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}

	base := makeConsolidateRepo(knownByULID)
	base.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ev, ok := knownByULID[ulid]; ok {
			return ev, nil
		}
		return makePublishedEvent("uuid-new", ulid, "New Canon Event"), nil
	}
	base.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		return nil
	}

	custom := &consolidateCreateRepo{
		mockTransactionalRepo: base,
		createFunc: func(_ context.Context, params EventCreateParams) (*Event, error) {
			return &Event{
				ID:             "uuid-new",
				ULID:           params.ULID,
				Name:           params.Name,
				LifecycleState: params.LifecycleState,
			}, nil
		},
	}

	svc := NewAdminService(custom, false, "America/Toronto", config.ValidationConfig{
		AllowTestDomains: true,
	}, consolidateBaseURL)

	// Event with no description — should trigger missing_description quality warning.
	input := EventInput{
		Name:      "New Canon Event",
		StartDate: now.Format(time.RFC3339),
		EndDate:   now.Add(2 * time.Hour).Format(time.RFC3339),
		VirtualLocation: &VirtualLocationInput{
			URL:  "https://example-stream.togather.foundation/new-event",
			Name: "Online Stream",
		},
	}
	result, err := svc.Consolidate(ctx, ConsolidateParams{
		Event:  &input,
		Retire: []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !result.NeedsReview {
		t.Error("expected NeedsReview=true for event missing description")
	}
	found := false
	for _, w := range result.Warnings {
		if w.Code == "missing_description" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing_description warning; got: %+v", result.Warnings)
	}
}

// ── Bug srv-4hmsu: OriginalPayload / NormalizedPayload must not be nil ────────

// TestConsolidate_PromotePath_NearDup_ReviewEntryPayloadsNotNil verifies that
// when a consolidation triggers a near-duplicate re-check and the canonical
// event is sent to pending_review, CreateReviewQueueEntry is called with
// non-nil OriginalPayload and NormalizedPayload (NOT NULL columns — null values
// cause a 23502 DB error → 500).
func TestConsolidate_PromotePath_NearDup_ReviewEntryPayloadsNotNil(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	venueName := "Test Venue"
	canon := &Event{
		ID:               "uuid-canon",
		ULID:             consolidateCanonULID,
		Name:             "Canon Event Near Dup",
		Description:      "A canonical event description.",
		PublicURL:        "https://toronto.togather.foundation/events/" + consolidateCanonULID,
		LifecycleState:   "published",
		PrimaryVenueID:   func() *string { s := "venue-uuid-1"; return &s }(),
		PrimaryVenueName: &venueName,
		Occurrences:      []Occurrence{{StartTime: now}},
	}

	known := map[string]*Event{
		consolidateCanonULID:  canon,
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)

	// FindNearDuplicates returns a genuine third-party near-dup (not the canon
	// itself, not any event in the retire list) above the threshold.
	repo.findNearDuplicatesFunc = func(_ context.Context, _ string, _ time.Time, _ string, _ float64) ([]NearDuplicateCandidate, error) {
		return []NearDuplicateCandidate{
			{ULID: consolidateNearDupULID, Name: "Canon Event Near Dup Copy", Similarity: 0.95},
		}, nil
	}

	// Capture the params passed to CreateReviewQueueEntry.
	var capturedParams ReviewQueueCreateParams
	repo.createReviewQueueEntryFunc = func(_ context.Context, params ReviewQueueCreateParams) (*ReviewQueueEntry, error) {
		capturedParams = params
		return &ReviewQueueEntry{ID: 1}, nil
	}

	svc := newConsolidateSvc(repo, 0.8)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !result.NeedsReview {
		t.Error("expected NeedsReview=true for genuine near-dup")
	}

	// The core assertion: payloads must be non-nil (NOT NULL columns).
	if len(capturedParams.OriginalPayload) == 0 {
		t.Error("CreateReviewQueueEntry called with nil/empty OriginalPayload — will hit DB NOT NULL constraint")
	}
	if len(capturedParams.NormalizedPayload) == 0 {
		t.Error("CreateReviewQueueEntry called with nil/empty NormalizedPayload — will hit DB NOT NULL constraint")
	}

	// Payload must be valid JSON containing at least the event name.
	var payload map[string]any
	if err := json.Unmarshal(capturedParams.OriginalPayload, &payload); err != nil {
		t.Errorf("OriginalPayload is not valid JSON: %v", err)
	}
	if payload["name"] != canon.Name {
		t.Errorf("OriginalPayload.name = %q, want %q", payload["name"], canon.Name)
	}

	// EventStartTime must reflect the canonical event's first occurrence.
	if !capturedParams.EventStartTime.Equal(now) {
		t.Errorf("EventStartTime = %v, want %v", capturedParams.EventStartTime, now)
	}
}

// ── Step 6b: consolidateStripRetiredDupWarnings ───────────────────────────────

// TestConsolidate_StripRetiredDupWarnings_ClearsNearDupWarning verifies that when
// the canonical event has a pending review entry containing a near_duplicate_of_new_event
// warning pointing to the retired event, the SQL atomically strips the warning but a
// non-dup warning (multi_session_likely) survives. The entry stays pending — no merge occurs.
func TestConsolidate_StripRetiredDupWarnings_ClearsNearDupWarning(t *testing.T) {
	ctx := context.Background()

	retiredULID := consolidateRetireULID
	canonULID := consolidateCanonULID

	initialWarnings, _ := json.Marshal([]ValidationWarning{
		{
			Field:   "near_duplicate",
			Code:    "near_duplicate_of_new_event",
			Message: "stale dup warning pointing to retired event",
		},
		{
			Field:   "multi_session",
			Code:    "multi_session_likely",
			Message: "this looks like a multi-session event",
		},
	})

	canonReview := &ReviewQueueEntry{
		ID:                   42,
		Status:               "pending",
		EventULID:            canonULID,
		DuplicateOfEventULID: &retiredULID,
		Warnings:             initialWarnings,
	}

	known := map[string]*Event{
		canonULID:   makePublishedEvent("uuid-canon", canonULID, "Canon Event"),
		retiredULID: makePublishedEvent("uuid-retire", retiredULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)

	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == canonULID {
			return canonReview, nil
		}
		return nil, nil
	}

	var stripCalled bool
	repo.stripRetiredDupWarningsFunc = func(_ context.Context, reviewID int, retireULIDs []string) (bool, error) {
		stripCalled = true
		if reviewID != 42 {
			t.Errorf("StripRetiredDupWarnings called with reviewID=%d, want 42", reviewID)
		}
		if len(retireULIDs) != 1 || retireULIDs[0] != retiredULID {
			t.Errorf("StripRetiredDupWarnings called with retireULIDs=%v, want [%s]", retireULIDs, retiredULID)
		}
		return false, nil
	}

	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		t.Errorf("MergeReview must NOT be called when non-dup warnings survive; called with id=%d", id)
		return nil, nil
	}

	repo.updateReviewQueueEntryFunc = func(_ context.Context, id int, _ ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
		t.Errorf("UpdateReviewQueueEntry must NOT be called — SQL handles stripping; called with id=%d", id)
		return nil, nil
	}

	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: canonULID,
		Retire:    []string{retiredULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if !stripCalled {
		t.Error("StripRetiredDupWarnings must have been called")
	}

	if result.Event != nil && result.Event.LifecycleState == "pending_review" {
		t.Error("canonical lifecycle must not be set to pending_review by the strip helper when non-dup warnings remain")
	}
}

// TestConsolidate_StripRetiredDupWarnings_DismissesIfNoWarningsRemain verifies that
// when the canonical has a pending review with ONLY a near_duplicate_of_new_event
// warning, the SQL strips it and returns warningsEmpty=true → the entry is dismissed
// and the canonical restored to "published".
func TestConsolidate_StripRetiredDupWarnings_DismissesIfNoWarningsRemain(t *testing.T) {
	ctx := context.Background()

	retiredULID := consolidateRetireULID
	canonULID := consolidateCanonULID

	onlyDupWarning, _ := json.Marshal([]ValidationWarning{
		{
			Field:   "near_duplicate",
			Code:    "near_duplicate_of_new_event",
			Message: "stale bare dup warning",
		},
	})

	canonReview := &ReviewQueueEntry{
		ID:                   99,
		Status:               "pending",
		EventULID:            canonULID,
		DuplicateOfEventULID: &retiredULID,
		Warnings:             onlyDupWarning,
	}

	known := map[string]*Event{
		canonULID:   makePublishedEvent("uuid-canon", canonULID, "Canon Event"),
		retiredULID: makePublishedEvent("uuid-retire", retiredULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)

	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == canonULID {
			return canonReview, nil
		}
		return nil, nil
	}

	repo.stripRetiredDupWarningsFunc = func(_ context.Context, reviewID int, retireULIDs []string) (bool, error) {
		return true, nil
	}

	// Track ApproveReview calls (used to approve the canonical entry).
	var approveCallIDs []int
	repo.approveReviewFunc = func(_ context.Context, id int, _ string, _ *string) (*ReviewQueueEntry, error) {
		approveCallIDs = append(approveCallIDs, id)
		return &ReviewQueueEntry{ID: id, Status: "approved"}, nil
	}

	var updatedLifecycleState string
	repo.updateEventFunc = func(_ context.Context, _ string, params UpdateEventParams) (*Event, error) {
		if params.LifecycleState != nil {
			updatedLifecycleState = *params.LifecycleState
		}
		return &Event{}, nil
	}

	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: canonULID,
		Retire:    []string{retiredULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// ApproveReview must have been called with the canonical review entry ID.
	dismissed := false
	for _, id := range approveCallIDs {
		if id == 99 {
			dismissed = true
		}
	}
	if !dismissed {
		t.Errorf("canonical review entry (id=99) must be approved via ApproveReview; calls: %v", approveCallIDs)
	}

	if updatedLifecycleState != "published" {
		t.Errorf("canonical lifecycle must be set to 'published' after full strip; got %q", updatedLifecycleState)
	}

	found := false
	for _, id := range result.ReviewEntriesDismissed {
		if id == 99 {
			found = true
		}
	}
	if !found {
		t.Errorf("review entry id=99 must appear in ReviewEntriesDismissed; got: %v", result.ReviewEntriesDismissed)
	}
}

// TestConsolidate_StripRetiredDupWarnings_NoEntryNoop verifies that when the canonical
// event has no pending review entry, consolidateStripRetiredDupWarnings is a no-op
// and returns no error.
func TestConsolidate_StripRetiredDupWarnings_NoEntryNoop(t *testing.T) {
	ctx := context.Background()

	canonULID := consolidateCanonULID
	retiredULID := consolidateRetireULID

	known := map[string]*Event{
		canonULID:   makePublishedEvent("uuid-canon", canonULID, "Canon Event"),
		retiredULID: makePublishedEvent("uuid-retire", retiredULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)

	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, _ string) (*ReviewQueueEntry, error) {
		return nil, nil
	}

	repo.stripRetiredDupWarningsFunc = func(_ context.Context, _ int, _ []string) (bool, error) {
		t.Errorf("StripRetiredDupWarnings must NOT be called when canonical has no pending review")
		return false, nil
	}

	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		t.Errorf("MergeReview must NOT be called when canonical has no pending review; called with id=%d", id)
		return nil, nil
	}

	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: canonULID,
		Retire:    []string{retiredULID},
	})
	if err != nil {
		t.Fatalf("expected success (no-op when no pending review), got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Canonical lifecycle must be unchanged (published).
	if result.Event != nil && result.Event.LifecycleState == "pending_review" {
		t.Error("canonical lifecycle must not change to pending_review when it had no prior pending review entry")
	}
}

// TestConsolidate_StripRetiredDupWarnings_CrossWeekCompanionSurvivesWhenDupOfRetired
// verifies that cross_week_series_companion survives the SQL strip when its companion_ulid
// is NOT retired, while near_duplicate_of_new_event IS stripped. The entry stays pending.
func TestConsolidate_StripRetiredDupWarnings_CrossWeekCompanionSurvivesWhenDupOfRetired(t *testing.T) {
	ctx := context.Background()

	retiredULID := consolidateRetireULID
	canonULID := consolidateCanonULID
	week1ULID := "01TEST00000WEEK1COMPANION0"

	initialWarnings, _ := json.Marshal([]ValidationWarning{
		{
			Field:   "name",
			Code:    "cross_week_series_companion",
			Message: "part of a recurring series",
			Details: map[string]any{
				"companion_ulid": week1ULID,
				"companion_name": "Week 1 Morning Session",
				"companion_date": "2026-03-31",
				"venue_name":     "The Tranzac",
			},
		},
		{
			Field:   "near_duplicate",
			Code:    "near_duplicate_of_new_event",
			Message: "may be near-duplicate of retired event",
		},
	})

	dupULID := retiredULID
	canonReview := &ReviewQueueEntry{
		ID:                   99,
		Status:               "pending",
		EventULID:            canonULID,
		DuplicateOfEventULID: &dupULID,
		Warnings:             initialWarnings,
	}

	known := map[string]*Event{
		canonULID:   makePublishedEvent("uuid-canon", canonULID, "Week 2 Morning"),
		retiredULID: makePublishedEvent("uuid-retire", retiredULID, "Week 2 Afternoon"),
	}
	repo := makeConsolidateRepo(known)

	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == canonULID {
			return canonReview, nil
		}
		return nil, nil
	}

	var stripCalled bool
	repo.stripRetiredDupWarningsFunc = func(_ context.Context, reviewID int, retireULIDs []string) (bool, error) {
		stripCalled = true
		if reviewID != 99 {
			t.Errorf("StripRetiredDupWarnings called with reviewID=%d, want 99", reviewID)
		}
		if len(retireULIDs) != 1 || retireULIDs[0] != retiredULID {
			t.Errorf("StripRetiredDupWarnings called with retireULIDs=%v, want [%s]", retireULIDs, retiredULID)
		}
		return false, nil
	}

	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		t.Errorf("MergeReview must NOT be called: cross_week_series_companion warning should survive; id=%d", id)
		return nil, nil
	}

	repo.updateReviewQueueEntryFunc = func(_ context.Context, id int, _ ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
		t.Errorf("UpdateReviewQueueEntry must NOT be called — SQL handles stripping; called with id=%d", id)
		return nil, nil
	}

	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: canonULID,
		Retire:    []string{retiredULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if !stripCalled {
		t.Error("StripRetiredDupWarnings must have been called")
	}
}

// TestConsolidate_PostRetirementSeriesCheck_ReplacesStaleCanonicalCompanion verifies
// that after the retire list is applied, the canonical's cross_week_series_companion
// warning is recomputed against the surviving events instead of continuing to point at
// the just-retired same-day duplicate.
func TestConsolidate_PostRetirementSeriesCheck_ReplacesStaleCanonicalCompanion(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	week2ULID := "01KM1B4HXH4WEEK2SURVIVOR001"
	staleRetiredULID := consolidateRetireULID

	venueID := "venue-uuid-1"
	venueName := "Pottery Studio"
	canon := &Event{
		ID:               "uuid-canon",
		ULID:             consolidateCanonULID,
		Name:             "RS-11 Pottery Studio",
		Description:      "Morning event",
		PublicURL:        "https://example.com/week1",
		LifecycleState:   "pending_review",
		PrimaryVenueID:   &venueID,
		PrimaryVenueName: &venueName,
		Occurrences:      []Occurrence{{StartTime: now}},
	}
	retired := &Event{
		ID:             "uuid-retire",
		ULID:           staleRetiredULID,
		Name:           "RS-11 Pottery Studio - Afternoon Session",
		LifecycleState: "pending_review",
	}

	staleWarnings, _ := json.Marshal([]ValidationWarning{{
		Field:   "name",
		Code:    "cross_week_series_companion",
		Message: "stale warning",
		Details: map[string]any{
			"companion_ulid": staleRetiredULID,
			"companion_name": retired.Name,
			"companion_date": "2026-04-07",
			"companion_time": "10:00:00",
			"venue_name":     venueName,
		},
	}})
	canonReview := &ReviewQueueEntry{
		ID:        41,
		Status:    "pending",
		EventULID: consolidateCanonULID,
		Warnings:  staleWarnings,
	}

	repo := makeConsolidateRepo(map[string]*Event{
		consolidateCanonULID: canon,
		staleRetiredULID:     retired,
		week2ULID:            makePublishedEvent("uuid-week2", week2ULID, "RS-11 Pottery Studio"),
	})
	repo.findSeriesCompanionFunc = func(_ context.Context, params SeriesCompanionQuery) (*CrossWeekCompanion, error) {
		if params.ExcludeULID != consolidateCanonULID {
			t.Fatalf("ExcludeULID = %q, want %q", params.ExcludeULID, consolidateCanonULID)
		}
		return &CrossWeekCompanion{
			ULID:      week2ULID,
			Name:      "RS-11 Pottery Studio",
			StartDate: "2026-04-07",
			StartTime: "06:00:00",
			VenueName: venueName,
		}, nil
	}
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == consolidateCanonULID {
			return canonReview, nil
		}
		return nil, nil
	}
	var stripReviewID int
	var stripRetireULIDs []string
	repo.stripRetiredDupWarningsFunc = func(_ context.Context, reviewID int, retireULIDs []string) (bool, error) {
		stripReviewID = reviewID
		stripRetireULIDs = retireULIDs
		return false, nil
	}

	var updatedWarningsJSON []byte
	repo.updateReviewQueueEntryFunc = func(_ context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
		if id == canonReview.ID && params.Warnings != nil {
			updatedWarningsJSON = *params.Warnings
		}
		return &ReviewQueueEntry{ID: id}, nil
	}

	svc := newConsolidateSvc(repo, 0.4)
	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{staleRetiredULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !result.NeedsReview {
		t.Fatal("expected NeedsReview=true after surviving week companion found")
	}
	if stripReviewID != canonReview.ID {
		t.Errorf("StripRetiredDupWarnings called with reviewID=%d, want %d", stripReviewID, canonReview.ID)
	}
	if len(stripRetireULIDs) != 1 || stripRetireULIDs[0] != staleRetiredULID {
		t.Errorf("StripRetiredDupWarnings called with retireULIDs=%v, want [%s]", stripRetireULIDs, staleRetiredULID)
	}
	var found bool
	for _, w := range result.Warnings {
		if w.Code == "cross_week_series_companion" {
			found = true
			if got := w.Details["companion_ulid"]; got != week2ULID {
				t.Fatalf("companion_ulid = %v, want %s", got, week2ULID)
			}
		}
	}
	if !found {
		t.Fatal("expected recomputed cross_week_series_companion warning")
	}
	if updatedWarningsJSON == nil {
		t.Fatal("expected canonical review warnings to be updated")
	}
	var updated []ValidationWarning
	if err := json.Unmarshal(updatedWarningsJSON, &updated); err != nil {
		t.Fatalf("unmarshal updated warnings: %v", err)
	}
	if len(updated) != 1 || updated[0].Code != "cross_week_series_companion" {
		t.Fatalf("updated warnings = %+v, want one cross_week_series_companion", updated)
	}
	if got := updated[0].Details["companion_ulid"]; got != week2ULID {
		t.Fatalf("updated companion_ulid = %v, want %s", got, week2ULID)
	}
}

// TestConsolidate_PostRetirementSeriesCheck_UpsertsCompanionReview verifies that
// a second same-day consolidation backfills or refreshes the surviving companion's
// pending review entry so both week-level canonicals point at each other.
func TestConsolidate_PostRetirementSeriesCheck_UpsertsCompanionReview(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 7, 6, 0, 0, 0, time.UTC)
	week1ULID := "01KM1B4HXH4WEEK1SURVIVOR001"
	week2RetiredULID := consolidateRetireULID
	venueID := "venue-uuid-1"
	venueName := "Pottery Studio"

	canon := &Event{
		ID:               "uuid-week2",
		ULID:             consolidateCanonULID,
		Name:             "RS-11 Pottery Studio",
		Description:      "Week 2 morning",
		PublicURL:        "https://example.com/week2",
		LifecycleState:   "published",
		PrimaryVenueID:   &venueID,
		PrimaryVenueName: &venueName,
		Occurrences:      []Occurrence{{StartTime: now}},
	}
	retired := &Event{ID: "uuid-retire", ULID: week2RetiredULID, Name: "Week 2 afternoon", LifecycleState: "pending_review"}
	week1 := &Event{
		ID:               "uuid-week1",
		ULID:             week1ULID,
		Name:             "RS-11 Pottery Studio",
		Description:      "Week 1 morning",
		PublicURL:        "https://example.com/week1",
		LifecycleState:   "pending_review",
		PrimaryVenueID:   &venueID,
		PrimaryVenueName: &venueName,
		Occurrences:      []Occurrence{{StartTime: now.Add(-7 * 24 * time.Hour)}},
	}

	week1ReviewWarnings, _ := json.Marshal([]ValidationWarning{{
		Field:   "name",
		Code:    "cross_week_series_companion",
		Message: "old weekly pairing",
		Details: map[string]any{
			"companion_ulid": "01KM1B4HXHOLDRETIRE000001",
			"companion_name": "Week 1 old same-day pair",
			"companion_date": "2026-03-31",
			"companion_time": "10:00:00",
			"venue_name":     venueName,
		},
	}})
	week1Review := &ReviewQueueEntry{ID: 52, Status: "pending", EventULID: week1ULID, Warnings: week1ReviewWarnings}

	repo := makeConsolidateRepo(map[string]*Event{
		consolidateCanonULID: canon,
		week2RetiredULID:     retired,
		week1ULID:            week1,
	})
	repo.findSeriesCompanionFunc = func(_ context.Context, params SeriesCompanionQuery) (*CrossWeekCompanion, error) {
		if params.ExcludeULID != consolidateCanonULID {
			return nil, nil
		}
		return &CrossWeekCompanion{
			ULID:      week1ULID,
			Name:      week1.Name,
			StartDate: "2026-03-31",
			StartTime: "06:00:00",
			VenueName: venueName,
		}, nil
	}
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		switch ulid {
		case consolidateCanonULID:
			return nil, nil
		case week1ULID:
			return week1Review, nil
		default:
			return nil, nil
		}
	}
	var stripReviewID int
	var stripRetireULIDs []string
	repo.stripRetiredDupWarningsFunc = func(_ context.Context, reviewID int, retireULIDs []string) (bool, error) {
		stripReviewID = reviewID
		stripRetireULIDs = retireULIDs
		return false, nil
	}

	var companionUpdatedWarnings []byte
	repo.updateReviewQueueEntryFunc = func(_ context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
		if id == week1Review.ID && params.Warnings != nil {
			companionUpdatedWarnings = *params.Warnings
		}
		return &ReviewQueueEntry{ID: id}, nil
	}

	svc := newConsolidateSvc(repo, 0.4)
	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{week2RetiredULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !result.NeedsReview {
		t.Fatal("expected week2 canonical to need review after finding week1 companion")
	}
	_ = stripReviewID
	_ = stripRetireULIDs
	if companionUpdatedWarnings == nil {
		t.Fatal("expected week1 companion review entry to be refreshed")
	}
	var updated []ValidationWarning
	if err := json.Unmarshal(companionUpdatedWarnings, &updated); err != nil {
		t.Fatalf("unmarshal week1 updated warnings: %v", err)
	}
	var found bool
	for _, w := range updated {
		if w.Code == "cross_week_series_companion" {
			found = true
			if got := w.Details["companion_ulid"]; got != consolidateCanonULID {
				t.Fatalf("week1 companion_ulid = %v, want %s", got, consolidateCanonULID)
			}
		}
	}
	if !found {
		t.Fatal("expected week1 review entry to contain cross_week_series_companion")
	}
}

// ── TransferOccurrences ───────────────────────────────────────────────────────

// TestConsolidate_TransferOccurrences_CopiesOccurrenceToCanonical verifies that
// when TransferOccurrences=true, the retired event's occurrence is copied to the
// canonical and the retired event's occurrences are deleted.
func TestConsolidate_TransferOccurrences_CopiesOccurrenceToCanonical(t *testing.T) {
	ctx := context.Background()

	occStart := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	retiredVenueID := "venue-uuid-retired"

	retiredEvent := &Event{
		ID:             "uuid-retire",
		ULID:           consolidateRetireULID,
		Name:           "Old Event",
		LifecycleState: "published",
		Occurrences: []Occurrence{
			{
				ID:        "occ-uuid-1",
				StartTime: occStart,
				Timezone:  "America/Vancouver",
				VenueID:   &retiredVenueID,
			},
		},
	}
	canonEvent := makePublishedEvent("uuid-canon", consolidateCanonULID, "Canon Event")

	known := map[string]*Event{
		consolidateCanonULID:  canonEvent,
		consolidateRetireULID: retiredEvent,
	}
	repo := makeConsolidateRepo(known)

	// No overlap.
	repo.checkOccurrenceOverlapFunc = func(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
		return false, nil
	}

	var capturedCreateParams OccurrenceCreateParams
	repo.createOccurrenceFunc = func(_ context.Context, params OccurrenceCreateParams) error {
		capturedCreateParams = params
		return nil
	}

	var deletedEventULID string
	repo.deleteOccurrencesByEventULIDFunc = func(_ context.Context, ulid string) error {
		deletedEventULID = ulid
		return nil
	}

	svc := newConsolidateSvc(repo, 0.4)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID:           consolidateCanonULID,
		Retire:              []string{consolidateRetireULID},
		TransferOccurrences: true,
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Occurrence must have been created on the canonical.
	if capturedCreateParams.EventID != canonEvent.ID {
		t.Errorf("CreateOccurrence must target canonical event ID %q, got %q", canonEvent.ID, capturedCreateParams.EventID)
	}
	if !capturedCreateParams.StartTime.Equal(occStart) {
		t.Errorf("CreateOccurrence start time mismatch: got %v, want %v", capturedCreateParams.StartTime, occStart)
	}
	if capturedCreateParams.Timezone != "America/Vancouver" {
		t.Errorf("CreateOccurrence timezone mismatch: got %q, want %q", capturedCreateParams.Timezone, "America/Vancouver")
	}
	if capturedCreateParams.VenueID == nil || *capturedCreateParams.VenueID != retiredVenueID {
		t.Errorf("CreateOccurrence venue ID mismatch: got %v, want %q", capturedCreateParams.VenueID, retiredVenueID)
	}

	// DeleteOccurrencesByEventULID must be called for the retired event.
	if deletedEventULID != consolidateRetireULID {
		t.Errorf("DeleteOccurrencesByEventULID must be called with retired ULID %q, got %q", consolidateRetireULID, deletedEventULID)
	}

	// Committed.
	if !repo.commitCalled {
		t.Error("Commit must be called on success")
	}
}

// TestConsolidate_TransferOccurrences_409OnOverlap verifies that when
// TransferOccurrences=true and a retired event's occurrence overlaps an
// existing canonical occurrence, ErrOccurrenceOverlap is returned and the
// transaction is rolled back.
func TestConsolidate_TransferOccurrences_409OnOverlap(t *testing.T) {
	ctx := context.Background()

	occStart := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)

	retiredEvent := &Event{
		ID:             "uuid-retire",
		ULID:           consolidateRetireULID,
		Name:           "Old Event",
		LifecycleState: "published",
		Occurrences: []Occurrence{
			{
				ID:        "occ-uuid-1",
				StartTime: occStart,
				Timezone:  "America/Toronto",
			},
		},
	}
	canonEvent := makePublishedEvent("uuid-canon", consolidateCanonULID, "Canon Event")

	known := map[string]*Event{
		consolidateCanonULID:  canonEvent,
		consolidateRetireULID: retiredEvent,
	}
	repo := makeConsolidateRepo(known)

	// Overlap detected.
	repo.checkOccurrenceOverlapFunc = func(_ context.Context, _ string, _ time.Time, _ *time.Time) (bool, error) {
		return true, nil
	}

	svc := newConsolidateSvc(repo, 0.4)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID:           consolidateCanonULID,
		Retire:              []string{consolidateRetireULID},
		TransferOccurrences: true,
	})
	if err == nil {
		t.Fatal("expected ErrOccurrenceOverlap, got nil")
	}
	if !errors.Is(err, ErrOccurrenceOverlap) {
		t.Errorf("expected ErrOccurrenceOverlap, got: %v", err)
	}
	// Transaction must not be committed.
	if repo.commitCalled {
		t.Error("Commit must not be called when occurrence overlap is detected")
	}
}

// TestConsolidate_NoTransfer_DoesNotCopyOccurrences verifies that when
// TransferOccurrences=false (default), no CreateOccurrence call is made,
// even if the retired event has occurrences.
func TestConsolidate_NoTransfer_DoesNotCopyOccurrences(t *testing.T) {
	ctx := context.Background()

	occStart := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)

	retiredEvent := &Event{
		ID:             "uuid-retire",
		ULID:           consolidateRetireULID,
		Name:           "Old Event",
		LifecycleState: "published",
		Occurrences: []Occurrence{
			{
				ID:        "occ-uuid-1",
				StartTime: occStart,
				Timezone:  "America/Toronto",
			},
		},
	}
	canonEvent := makePublishedEvent("uuid-canon", consolidateCanonULID, "Canon Event")

	known := map[string]*Event{
		consolidateCanonULID:  canonEvent,
		consolidateRetireULID: retiredEvent,
	}
	repo := makeConsolidateRepo(known)

	createCalled := false
	repo.createOccurrenceFunc = func(_ context.Context, _ OccurrenceCreateParams) error {
		createCalled = true
		return nil
	}

	svc := newConsolidateSvc(repo, 0.4)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID:           consolidateCanonULID,
		Retire:              []string{consolidateRetireULID},
		TransferOccurrences: false, // default — no occurrence transfer
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if createCalled {
		t.Error("CreateOccurrence must not be called when TransferOccurrences=false")
	}
	if !repo.commitCalled {
		t.Error("Commit must be called on success")
	}
}

// ---------------------------------------------------------------------------
// Regression tests for srv-2snn7: FindSeriesCompanion must not poison tx
// ---------------------------------------------------------------------------

// TestConsolidate_FindSeriesCompanionErrorDoesNotPoisonTx is the direct
// regression test for the "tx is closed" bug (srv-2snn7).
//
// Before the fix, FindSeriesCompanion called r.Rollback on any scan error,
// poisoning the caller's transaction. Subsequent operations (e.g.
// SoftDeleteEvent) then returned "tx is closed".
//
// This test simulates the scan error and verifies:
// 1. Consolidate still succeeds (FindSeriesCompanion errors are non-fatal).
// 2. SoftDeleteEvent is reached — the transaction is NOT poisoned.
// 3. Commit is called.
func TestConsolidate_FindSeriesCompanionErrorDoesNotPoisonTx(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	canon := makeVenueCanonEvent("uuid-canon", consolidateCanonULID, "Jazz Jam Session", now)
	known := map[string]*Event{
		consolidateCanonULID:  canon,
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Jazz Jam Session"),
	}
	repo := makeConsolidateRepo(known)

	// Simulate a pgx scan error (e.g. timestamptz scanned into string).
	repo.findSeriesCompanionFunc = func(_ context.Context, _ SeriesCompanionQuery) (*CrossWeekCompanion, error) {
		return nil, errors.New("simulated pgx scan error")
	}

	softDeleteCalled := false
	repo.softDeleteEventFunc = func(_ context.Context, _, _ string) error {
		softDeleteCalled = true
		return nil
	}

	svc := newConsolidateSvc(repo, 0.4)
	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})

	if err != nil {
		t.Fatalf("expected success (FindSeriesCompanion error is non-fatal), got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !softDeleteCalled {
		t.Error("SoftDeleteEvent must be called — tx must not be poisoned by FindSeriesCompanion error")
	}
	if !repo.commitCalled {
		t.Error("Commit must be called on overall success")
	}
}

// ── EventPatch atomicity tests ────────────────────────────────────────────────

// TestConsolidate_PromotePath_WithEventPatch_AppliedInTransaction verifies that
// when event is supplied alongside event_ulid, UpdateEvent is called inside the
// transaction and the result reflects the patched name.
func TestConsolidate_PromotePath_WithEventPatch_AppliedInTransaction(t *testing.T) {
	ctx := context.Background()

	patchedName := "RS-11 Pottery Studio"
	known := map[string]*Event{
		consolidateCanonULID:  makePublishedEvent("uuid-canon", consolidateCanonULID, "RS-11 Pottery Studio — Morning Session"),
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "RS-11 Pottery Studio — Afternoon Session"),
	}
	repo := makeConsolidateRepo(known)

	updateEventCalled := false
	repo.updateEventFunc = func(_ context.Context, ulid string, params UpdateEventParams) (*Event, error) {
		if ulid != consolidateCanonULID {
			t.Errorf("UpdateEvent called with wrong ULID: got %s, want %s", ulid, consolidateCanonULID)
		}
		if params.Name == nil || *params.Name != patchedName {
			t.Errorf("UpdateEvent called with wrong Name: got %v, want %q", params.Name, patchedName)
		}
		updateEventCalled = true
		// Update the stored event so GetByULID returns the patched version.
		known[ulid].Name = patchedName
		return known[ulid], nil
	}
	// Also need to override GetByULID to return from known map.
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ev, ok := known[ulid]; ok {
			return ev, nil
		}
		return nil, ErrNotFound
	}

	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Event:     &EventInput{Name: patchedName},
		Retire:    []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !updateEventCalled {
		t.Error("expected UpdateEvent to be called for the event patch, but it was not")
	}
	if result.Event == nil || result.Event.Name != patchedName {
		t.Errorf("expected result.Event.Name=%q, got: %+v", patchedName, result.Event)
	}
	if !repo.commitCalled {
		t.Error("Commit must be called on success")
	}
}

// TestConsolidate_PromotePath_WithEventPatch_RefreshesReviewPayload verifies that
// when event is supplied alongside event_ulid and the canonical already has a pending
// review queue entry, UpdateReviewQueueEntry is called with a payload that contains
// the post-patch name (not the original stale name).
func TestConsolidate_PromotePath_WithEventPatch_RefreshesReviewPayload(t *testing.T) {
	ctx := context.Background()

	patchedName := "RS-11 Pottery Studio"
	originalName := "RS-11 Pottery Studio — Morning Session"

	known := map[string]*Event{
		consolidateCanonULID:  makePublishedEvent("uuid-canon", consolidateCanonULID, originalName),
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "RS-11 Pottery Studio — Afternoon Session"),
	}
	repo := makeConsolidateRepo(known)

	// UpdateEvent applies the patch and updates the stored event.
	repo.updateEventFunc = func(_ context.Context, ulid string, params UpdateEventParams) (*Event, error) {
		if params.Name != nil {
			known[ulid].Name = *params.Name
		}
		return known[ulid], nil
	}
	// Also need to override GetByULID to return from known map.
	repo.getByULIDFunc = func(_ context.Context, ulid string) (*Event, error) {
		if ev, ok := known[ulid]; ok {
			return ev, nil
		}
		return nil, ErrNotFound
	}

	// Canonical already has a pending review entry (the RS-11 morning session scenario).
	existingReviewID := 77
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == consolidateCanonULID {
			return &ReviewQueueEntry{ID: existingReviewID, EventULID: consolidateCanonULID}, nil
		}
		return nil, ErrNotFound
	}

	updateReviewCalled := false
	repo.updateReviewQueueEntryFunc = func(_ context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
		if id != existingReviewID {
			t.Errorf("UpdateReviewQueueEntry called with wrong id: got %d, want %d", id, existingReviewID)
		}
		// The payload must contain the post-patch name, not the original.
		if params.NormalizedPayload != nil {
			payload := string(*params.NormalizedPayload)
			if !containsString(payload, patchedName) {
				t.Errorf("NormalizedPayload does not contain patched name %q; got: %s", patchedName, payload)
			}
			if containsString(payload, originalName) {
				t.Errorf("NormalizedPayload still contains stale original name %q; got: %s", originalName, payload)
			}
		}
		updateReviewCalled = true
		return &ReviewQueueEntry{ID: id}, nil
	}

	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: consolidateCanonULID,
		Event:     &EventInput{Name: patchedName},
		Retire:    []string{consolidateRetireULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !updateReviewCalled {
		t.Error("expected UpdateReviewQueueEntry to be called to refresh stale payload, but it was not")
	}
	if !repo.commitCalled {
		t.Error("Commit must be called on success")
	}
}

// ── Step 6c.5: consolidateUpdateThirdPartyCompanionWarnings ────────────────────

// TestConsolidate_UpdateThirdPartyCompanionWarnings verifies that when a
// third-party review entry has a cross_week_series_companion warning whose
// companion_ulid references a now-retired event, the warning is updated to
// point to the surviving canonical instead.
func TestConsolidate_UpdateThirdPartyCompanionWarnings(t *testing.T) {
	ctx := context.Background()

	canonULID := consolidateCanonULID
	retiredULID := consolidateRetireULID
	thirdULID := consolidateThirdULID

	startTime := time.Date(2026, 3, 31, 10, 30, 0, 0, time.UTC)

	// Week3 (third-party) has a cross_week_series_companion warning pointing to week2 (retired).
	thirdWarnings := []ValidationWarning{
		{
			Field:   "name",
			Code:    "cross_week_series_companion",
			Message: "third-party companion warning",
			Details: map[string]any{
				"companion_ulid": retiredULID,
				"companion_name": "Old Retired Event",
				"companion_date": "2026-03-24",
				"companion_time": "10:30:00",
				"venue_name":     "Test Venue",
			},
		},
		{
			Field:   "other",
			Code:    "suspicious_duration",
			Message: "keeps this warning",
		},
	}
	thirdWarningsJSON, _ := json.Marshal(thirdWarnings)

	// Week2 (retired) has a cross_week_series_companion warning pointing to week3.
	retiredWarnings := []ValidationWarning{
		{
			Field:   "name",
			Code:    "cross_week_series_companion",
			Message: "retired companion warning",
			Details: map[string]any{
				"companion_ulid": thirdULID,
				"companion_name": "Third Party Event",
				"companion_date": "2026-03-31",
				"companion_time": "10:30:00",
				"venue_name":     "Test Venue",
			},
		},
	}
	retiredWarningsJSON, _ := json.Marshal(retiredWarnings)

	// In-memory review storage for test assertions.
	reviewEntries := map[int]*ReviewQueueEntry{
		1: {
			ID:        1,
			EventID:   "uuid-retired",
			EventULID: retiredULID,
			Status:    "pending",
			Warnings:  retiredWarningsJSON,
		},
		2: {
			ID:        2,
			EventID:   "uuid-third",
			EventULID: thirdULID,
			Status:    "pending",
			Warnings:  thirdWarningsJSON,
		},
	}

	known := map[string]*Event{
		canonULID:   makePublishedEvent("uuid-canon", canonULID, "Canonical Event"),
		retiredULID: makePublishedEvent("uuid-retire", retiredULID, "Retired Event"),
		thirdULID:   makePublishedEvent("uuid-third", thirdULID, "Third Party Event"),
	}
	repo := makeConsolidateRepo(known)

	// Canonical has no pending review (it's published).
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		return nil, nil
	}
	repo.getReviewQueueEntryFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
		entry, ok := reviewEntries[id]
		if !ok {
			return nil, ErrNotFound
		}
		return entry, nil
	}

	var updatedReviewID int
	var updatedWarnings []byte
	repo.updateReviewQueueEntryFunc = func(_ context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
		if params.Warnings == nil {
			return nil, nil
		}
		updatedReviewID = id
		updatedWarnings = *params.Warnings
		return &ReviewQueueEntry{ID: id}, nil
	}

	repo.findCrossWeekCompanionTargetsFunc = func(_ context.Context, retireULIDs []string) ([]CrossWeekCompanionTarget, error) {
		if len(retireULIDs) != 1 || retireULIDs[0] != retiredULID {
			t.Errorf("FindCrossWeekCompanionTargets called with retireULIDs=%v, want [%s]", retireULIDs, retiredULID)
		}
		return []CrossWeekCompanionTarget{
			{ReviewID: 2, EventULID: thirdULID},
		}, nil
	}

	// The canonical has occurrences so the third-party update can compute dates.
	canonEvent := known[canonULID]
	canonEvent.Occurrences = []Occurrence{{StartTime: startTime}}

	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	result, err := svc.Consolidate(ctx, ConsolidateParams{
		EventULID: canonULID,
		Retire:    []string{retiredULID},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if updatedReviewID != 2 {
		t.Errorf("expected UpdateReviewQueueEntry to be called with reviewID=2 (third-party), got %d", updatedReviewID)
	}

	if len(updatedWarnings) == 0 {
		t.Fatal("expected updated warnings to be non-empty")
	}

	var finalWarnings []ValidationWarning
	if err := json.Unmarshal(updatedWarnings, &finalWarnings); err != nil {
		t.Fatalf("failed to unmarshal updated warnings: %v", err)
	}

	foundCompanion := false
	foundSuspicious := false
	for _, w := range finalWarnings {
		switch w.Code {
		case "cross_week_series_companion":
			foundCompanion = true
			companionULID, _ := w.Details["companion_ulid"].(string)
			if companionULID != canonULID {
				t.Errorf("cross_week_series_companion companion_ulid = %q, want %q (canonical)", companionULID, canonULID)
			}
			companionName, _ := w.Details["companion_name"].(string)
			if companionName != "Canonical Event" {
				t.Errorf("cross_week_series_companion companion_name = %q, want %q", companionName, "Canonical Event")
			}
			companionDate, _ := w.Details["companion_date"].(string)
			if companionDate != "2026-03-31" {
				t.Errorf("cross_week_series_companion companion_date = %q, want %q", companionDate, "2026-03-31")
			}
			companionTime, _ := w.Details["companion_time"].(string)
			if companionTime != "10:30:00" {
				t.Errorf("cross_week_series_companion companion_time = %q, want %q", companionTime, "10:30:00")
			}
			venueName, _ := w.Details["venue_name"].(string)
			if venueName != "The Tranzac" {
				t.Errorf("cross_week_series_companion venue_name = %q, want %q", venueName, "The Tranzac")
			}
		case "suspicious_duration":
			foundSuspicious = true
		}
	}
	if !foundCompanion {
		t.Error("updated warnings must contain cross_week_series_companion")
	}
	if !foundSuspicious {
		t.Error("non-companion warning (suspicious_duration) must survive the update")
	}
}

// containsString is a trivial substring helper used in test assertions.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
