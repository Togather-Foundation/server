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
	return &Event{
		ID:             id,
		ULID:           ulid,
		Name:           name,
		LifecycleState: "published",
	}
}

// ── Validation error tests ────────────────────────────────────────────────────

func TestConsolidate_BothEventFields_Error(t *testing.T) {
	ctx := context.Background()
	repo := makeConsolidateRepo(nil)
	svc := NewAdminService(repo, false, "America/Toronto", config.ValidationConfig{}, consolidateBaseURL)

	_, err := svc.Consolidate(ctx, ConsolidateParams{
		Event:     &EventInput{Name: "some event"},
		EventULID: consolidateCanonULID,
		Retire:    []string{consolidateRetireULID},
	})
	if !errors.Is(err, ErrConsolidateBothEventFields) {
		t.Errorf("expected ErrConsolidateBothEventFields, got: %v", err)
	}
}

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
		consolidateCanonULID:  canon,
		consolidateRetireULID: makePublishedEvent("uuid-retire", consolidateRetireULID, "Old Event"),
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
	dupWarnings := 0
	for _, w := range result.Warnings {
		if w.Code == "potential_duplicate" {
			dupWarnings++
			if w.Message != fmt.Sprintf("near-duplicate of existing event %s (similarity %.2f)", consolidateNearDupULID, 0.9) {
				t.Errorf("unexpected warning message: %s", w.Message)
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
// warning pointing to the retired event, that warning is stripped. If a non-dup warning
// (multi_session_likely) remains, the entry stays pending with updated warnings.
func TestConsolidate_StripRetiredDupWarnings_ClearsNearDupWarning(t *testing.T) {
	ctx := context.Background()

	retiredULID := consolidateRetireULID
	canonULID := consolidateCanonULID

	// Canonical pending review entry: near_duplicate_of_new_event (stale) +
	// multi_session_likely (unrelated, must survive).
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
		ID:        42,
		Status:    "pending",
		EventULID: canonULID,
		// DuplicateOfEventULID points to the retired event (stale).
		DuplicateOfEventULID: &retiredULID,
		Warnings:             initialWarnings,
	}

	known := map[string]*Event{
		canonULID:   makePublishedEvent("uuid-canon", canonULID, "Canon Event"),
		retiredULID: makePublishedEvent("uuid-retire", retiredULID, "Old Event"),
	}
	repo := makeConsolidateRepo(known)

	// Return the pending review for the canonical, nil for retired (already dismissed).
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, ulid string) (*ReviewQueueEntry, error) {
		if ulid == canonULID {
			return canonReview, nil
		}
		return nil, nil
	}

	// Capture UpdateReviewQueueEntry call.
	var updatedWarningsJSON []byte
	var capturedClearDuplicateOf bool
	repo.updateReviewQueueEntryFunc = func(_ context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
		if id != 42 {
			t.Errorf("UpdateReviewQueueEntry called with unexpected id=%d, want 42", id)
		}
		if params.Warnings != nil {
			updatedWarningsJSON = *params.Warnings
		}
		capturedClearDuplicateOf = params.ClearDuplicateOf
		return &ReviewQueueEntry{ID: id}, nil
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

	// UpdateReviewQueueEntry must have been called.
	if updatedWarningsJSON == nil {
		t.Fatal("expected UpdateReviewQueueEntry to be called with updated warnings, but it was not")
	}

	// ClearDuplicateOf must be true — DuplicateOfEventULID pointed to the retired event.
	if !capturedClearDuplicateOf {
		t.Error("expected ClearDuplicateOf=true when DuplicateOfEventULID points to a retired event")
	}

	// Remaining warnings must not include near_duplicate_of_new_event.
	var remaining []ValidationWarning
	if err := json.Unmarshal(updatedWarningsJSON, &remaining); err != nil {
		t.Fatalf("updated warnings JSON is invalid: %v", err)
	}
	for _, w := range remaining {
		if w.Code == "near_duplicate_of_new_event" {
			t.Errorf("near_duplicate_of_new_event warning must be stripped but is still present: %+v", w)
		}
	}
	// multi_session_likely must still be present.
	found := false
	for _, w := range remaining {
		if w.Code == "multi_session_likely" {
			found = true
		}
	}
	if !found {
		t.Errorf("multi_session_likely warning must survive stripping; remaining: %+v", remaining)
	}
	// The canonical's lifecycle must NOT have been changed — the entry stays pending
	// because of the surviving multi_session_likely warning, but we do not alter the
	// canonical's lifecycle state through the strip helper (that is the review queue
	// entry's job, not ours).
	// result.Event comes from the promote path — lifecycle starts as "published".
	// It must remain "published" since the strip helper only touched the warnings.
	if result.Event != nil && result.Event.LifecycleState == "pending_review" {
		t.Error("canonical lifecycle must not be set to pending_review by the strip helper when non-dup warnings remain")
	}
}

// TestConsolidate_StripRetiredDupWarnings_DismissesIfNoWarningsRemain verifies that
// when the canonical has a pending review with ONLY a near_duplicate_of_new_event
// warning, stripping leaves zero warnings → the entry should be dismissed and the
// canonical restored to "published".
func TestConsolidate_StripRetiredDupWarnings_DismissesIfNoWarningsRemain(t *testing.T) {
	ctx := context.Background()

	retiredULID := consolidateRetireULID
	canonULID := consolidateCanonULID

	// Only a single near_duplicate_of_new_event warning — no match details (bare warning).
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

	// Track MergeReview calls (used to dismiss the entry).
	var mergeCallIDs []int
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		mergeCallIDs = append(mergeCallIDs, id)
		return &ReviewQueueEntry{ID: id, Status: "merged"}, nil
	}

	// Track UpdateEvent calls.
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

	// MergeReview must have been called with the canonical review entry ID.
	dismissed := false
	for _, id := range mergeCallIDs {
		if id == 99 {
			dismissed = true
		}
	}
	if !dismissed {
		t.Errorf("canonical review entry (id=99) must be dismissed via MergeReview; calls: %v", mergeCallIDs)
	}

	// Canonical must be restored to published.
	if updatedLifecycleState != "published" {
		t.Errorf("canonical lifecycle must be set to 'published' after full strip; got %q", updatedLifecycleState)
	}

	// The dismissed ID must appear in ReviewEntriesDismissed.
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

	// GetPendingReviewByEventUlid returns nil — canonical has no pending review.
	repo.getPendingReviewByEventUlidFunc = func(_ context.Context, _ string) (*ReviewQueueEntry, error) {
		return nil, nil
	}

	// Neither UpdateReviewQueueEntry nor MergeReview should be called.
	repo.updateReviewQueueEntryFunc = func(_ context.Context, id int, _ ReviewQueueUpdateParams) (*ReviewQueueEntry, error) {
		t.Errorf("UpdateReviewQueueEntry must not be called when canonical has no pending review; called with id=%d", id)
		return nil, nil
	}
	repo.mergeReviewFunc = func(_ context.Context, id int, _ string, _ string) (*ReviewQueueEntry, error) {
		t.Errorf("MergeReview must not be called when canonical has no pending review; called with id=%d", id)
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
