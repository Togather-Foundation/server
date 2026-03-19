package events

import (
	"context"
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
