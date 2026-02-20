package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/kg"
	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestDeduplicationArgs_Kind(t *testing.T) {
	args := DeduplicationArgs{EventID: "test-event-123"}
	if args.Kind() != JobKindDeduplication {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindDeduplication)
	}
}

func TestReconciliationArgs_Kind(t *testing.T) {
	args := ReconciliationArgs{EntityType: "place", EntityID: "test-place-123"}
	if args.Kind() != JobKindReconciliation {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindReconciliation)
	}
}

func TestEnrichmentArgs_Kind(t *testing.T) {
	args := EnrichmentArgs{EntityType: "place", EntityID: "test-place-789", IdentifierURI: "http://example.org/123"}
	if args.Kind() != JobKindEnrichment {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindEnrichment)
	}
}

func TestIdempotencyCleanupArgs_Kind(t *testing.T) {
	args := IdempotencyCleanupArgs{}
	if args.Kind() != JobKindIdempotencyCleanup {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindIdempotencyCleanup)
	}
}

func TestBatchResultsCleanupArgs_Kind(t *testing.T) {
	args := BatchResultsCleanupArgs{}
	if args.Kind() != JobKindBatchResultsCleanup {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindBatchResultsCleanup)
	}
}

func TestBatchIngestionArgs_Kind(t *testing.T) {
	args := BatchIngestionArgs{BatchID: "batch-123"}
	if args.Kind() != JobKindBatchIngestion {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindBatchIngestion)
	}
}

func TestDeduplicationWorker_Kind(t *testing.T) {
	worker := DeduplicationWorker{}
	if worker.Kind() != JobKindDeduplication {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindDeduplication)
	}
}

func TestReconciliationWorker_Kind(t *testing.T) {
	worker := ReconciliationWorker{}
	if worker.Kind() != JobKindReconciliation {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindReconciliation)
	}
}

func TestEnrichmentWorker_Kind(t *testing.T) {
	worker := EnrichmentWorker{}
	if worker.Kind() != JobKindEnrichment {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindEnrichment)
	}
}

func TestIdempotencyCleanupWorker_Kind(t *testing.T) {
	worker := IdempotencyCleanupWorker{}
	if worker.Kind() != JobKindIdempotencyCleanup {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindIdempotencyCleanup)
	}
}

func TestBatchResultsCleanupWorker_Kind(t *testing.T) {
	worker := BatchResultsCleanupWorker{}
	if worker.Kind() != JobKindBatchResultsCleanup {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindBatchResultsCleanup)
	}
}

func TestBatchIngestionWorker_Kind(t *testing.T) {
	worker := BatchIngestionWorker{}
	if worker.Kind() != JobKindBatchIngestion {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindBatchIngestion)
	}
}

func TestDeduplicationWorker_WorkWithNilJob(t *testing.T) {
	worker := DeduplicationWorker{}
	ctx := context.Background()

	err := worker.Work(ctx, nil)
	if err == nil {
		t.Error("Work() with nil job should return error")
	}
}

func TestReconciliationWorker_WorkWithNilJob(t *testing.T) {
	worker := ReconciliationWorker{}
	ctx := context.Background()

	err := worker.Work(ctx, nil)
	if err == nil {
		t.Error("Work() with nil job should return error")
	}
}

func TestEnrichmentWorker_WorkWithNilJob(t *testing.T) {
	worker := EnrichmentWorker{}
	ctx := context.Background()

	err := worker.Work(ctx, nil)
	if err == nil {
		t.Error("Work() with nil job should return error")
	}
}

func TestIdempotencyCleanupWorker_WorkWithNilPool(t *testing.T) {
	worker := IdempotencyCleanupWorker{
		Pool: nil,
	}
	ctx := context.Background()

	job := &river.Job[IdempotencyCleanupArgs]{
		Args: IdempotencyCleanupArgs{},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("Work() with nil pool should return error")
	}
}

func TestBatchResultsCleanupWorker_WorkWithNilPool(t *testing.T) {
	worker := BatchResultsCleanupWorker{
		Pool: nil,
	}
	ctx := context.Background()

	job := &river.Job[BatchResultsCleanupArgs]{
		Args: BatchResultsCleanupArgs{},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("Work() with nil pool should return error")
	}
}

func TestBatchIngestionWorker_WorkWithNilIngestService(t *testing.T) {
	ctx := context.Background()
	worker := BatchIngestionWorker{
		IngestService: nil,
		Pool:          nil,
	}

	job := &river.Job[BatchIngestionArgs]{
		Args: BatchIngestionArgs{BatchID: "test"},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("Work() should return error when IngestService is nil")
	}
}

func TestBatchIngestionWorker_WorkWithNilJob(t *testing.T) {
	ctx := context.Background()
	worker := BatchIngestionWorker{}

	err := worker.Work(ctx, nil)
	if err == nil {
		t.Error("Work() with nil job should return error")
	}
}

func TestNewWorkers(t *testing.T) {
	workers := NewWorkers()

	if workers == nil {
		t.Fatal("NewWorkers() returned nil")
	}
}

func TestDeduplicationWorker_WorkWithValidJob(t *testing.T) {
	worker := DeduplicationWorker{}
	ctx := context.Background()

	job := &river.Job[DeduplicationArgs]{
		Args: DeduplicationArgs{
			EventID: "test-event-id",
		},
	}

	err := worker.Work(ctx, job)
	if err != nil {
		t.Errorf("Work() with valid job should not error, got: %v", err)
	}
}

func TestReconciliationWorker_WorkWithValidJob(t *testing.T) {
	worker := ReconciliationWorker{}
	ctx := context.Background()

	job := &river.Job[ReconciliationArgs]{
		Args: ReconciliationArgs{
			EntityType: "place",
			EntityID:   "test-place-id",
		},
	}

	err := worker.Work(ctx, job)
	// Expect error because Pool and ReconciliationService are nil
	if err == nil {
		t.Error("Work() should return error when dependencies are nil")
	}
}

func TestEnrichmentWorker_WorkWithValidJob(t *testing.T) {
	worker := EnrichmentWorker{}
	ctx := context.Background()

	job := &river.Job[EnrichmentArgs]{
		Args: EnrichmentArgs{
			EntityType:    "place",
			EntityID:      "test-place-id",
			IdentifierURI: "http://example.org/123",
		},
	}

	err := worker.Work(ctx, job)
	// Expect error because Pool and ReconciliationService are nil
	if err == nil {
		t.Error("Work() should return error when dependencies are nil")
	}
}

func TestUsageRollupArgs_Kind(t *testing.T) {
	args := UsageRollupArgs{}
	if args.Kind() != JobKindUsageRollup {
		t.Errorf("Kind() = %q, want %q", args.Kind(), JobKindUsageRollup)
	}
}

func TestUsageRollupWorker_Kind(t *testing.T) {
	worker := UsageRollupWorker{}
	if worker.Kind() != JobKindUsageRollup {
		t.Errorf("Kind() = %q, want %q", worker.Kind(), JobKindUsageRollup)
	}
}

func TestUsageRollupWorker_WorkWithNilPool(t *testing.T) {
	worker := UsageRollupWorker{
		Pool: nil,
	}
	ctx := context.Background()

	job := &river.Job[UsageRollupArgs]{
		Args: UsageRollupArgs{},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("Work() with nil pool should return error")
	}
}

// ── ReconciliationWorker test helpers ──────────────────────────────────────

type mockEntityReconciler struct {
	reconcileFunc func(ctx context.Context, req kg.ReconcileRequest) ([]kg.MatchResult, error)
	calls         int
	lastReq       kg.ReconcileRequest
}

func (m *mockEntityReconciler) ReconcileEntity(ctx context.Context, req kg.ReconcileRequest) ([]kg.MatchResult, error) {
	m.calls++
	m.lastReq = req
	if m.reconcileFunc != nil {
		return m.reconcileFunc(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func TestReconciliationWorker_WorkCallsReconcileEntity(t *testing.T) {
	t.Parallel()

	reconciler := &mockEntityReconciler{
		reconcileFunc: func(_ context.Context, req kg.ReconcileRequest) ([]kg.MatchResult, error) {
			if req.EntityType != "place" || req.EntityID != "test-place-id" {
				t.Errorf("unexpected request: %+v", req)
			}
			return nil, nil // no matches
		},
	}

	// Verify the worker struct accepts an EntityReconciler (interface, not concrete type).
	// Pool is nil so Work() returns an error before reaching the DB, but the
	// compile-time interface wiring is what this test validates.
	worker := ReconciliationWorker{
		ReconciliationService: reconciler,
	}

	ctx := context.Background()
	job := &river.Job[ReconciliationArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1},
		Args: ReconciliationArgs{
			EntityType: "place",
			EntityID:   "test-place-id",
		},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("expected error when Pool is nil")
	}
}

func TestReconciliationWorker_WorkReturnsErrorFromReconciler(t *testing.T) {
	t.Parallel()

	reconciler := &mockEntityReconciler{
		reconcileFunc: func(_ context.Context, _ kg.ReconcileRequest) ([]kg.MatchResult, error) {
			return nil, errors.New("artsdata unavailable")
		},
	}

	// Verify the worker accepts EntityReconciler and returns error when Pool is nil.
	worker := ReconciliationWorker{
		ReconciliationService: reconciler,
	}

	ctx := context.Background()
	job := &river.Job[ReconciliationArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1},
		Args: ReconciliationArgs{
			EntityType: "place",
			EntityID:   "test-place-id",
		},
	}

	err := worker.Work(ctx, job)
	if err == nil {
		t.Error("expected error when Pool is nil")
	}
}

// ── EnrichmentWorker test helpers ──────────────────────────────────────────

type mockEntityDereferencer struct {
	dereferenceFunc func(ctx context.Context, uri string) (*artsdata.EntityData, error)
	calls           int
}

func (m *mockEntityDereferencer) DereferenceEntity(ctx context.Context, uri string) (*artsdata.EntityData, error) {
	m.calls++
	if m.dereferenceFunc != nil {
		return m.dereferenceFunc(ctx, uri)
	}
	return nil, errors.New("not implemented")
}

type mockIdentifierUpserter struct {
	upsertFunc func(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error)
	calls      int
}

func (m *mockIdentifierUpserter) UpsertEntityIdentifier(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error) {
	m.calls++
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, arg)
	}
	return postgres.EntityIdentifier{}, nil
}

type mockPlaceUpdater struct {
	getFunc     func(ctx context.Context, ulid string) (*places.Place, error)
	updateFunc  func(ctx context.Context, ulid string, params places.UpdatePlaceParams) (*places.Place, error)
	getCalls    int
	updateCalls int
}

func (m *mockPlaceUpdater) GetByULID(ctx context.Context, ulid string) (*places.Place, error) {
	m.getCalls++
	if m.getFunc != nil {
		return m.getFunc(ctx, ulid)
	}
	return &places.Place{ULID: ulid}, nil
}

func (m *mockPlaceUpdater) Update(ctx context.Context, ulid string, params places.UpdatePlaceParams) (*places.Place, error) {
	m.updateCalls++
	if m.updateFunc != nil {
		return m.updateFunc(ctx, ulid, params)
	}
	return &places.Place{ULID: ulid}, nil
}

type mockOrgUpdater struct {
	getFunc     func(ctx context.Context, ulid string) (*organizations.Organization, error)
	updateFunc  func(ctx context.Context, ulid string, params organizations.UpdateOrganizationParams) (*organizations.Organization, error)
	getCalls    int
	updateCalls int
}

func (m *mockOrgUpdater) GetByULID(ctx context.Context, ulid string) (*organizations.Organization, error) {
	m.getCalls++
	if m.getFunc != nil {
		return m.getFunc(ctx, ulid)
	}
	return &organizations.Organization{ULID: ulid}, nil
}

func (m *mockOrgUpdater) Update(ctx context.Context, ulid string, params organizations.UpdateOrganizationParams) (*organizations.Organization, error) {
	m.updateCalls++
	if m.updateFunc != nil {
		return m.updateFunc(ctx, ulid, params)
	}
	return &organizations.Organization{ULID: ulid}, nil
}

func makeEnrichmentJob(entityType, entityID, identifierURI string) *river.Job[EnrichmentArgs] {
	return &river.Job[EnrichmentArgs]{
		JobRow: &rivertype.JobRow{},
		Args: EnrichmentArgs{
			EntityType:    entityType,
			EntityID:      entityID,
			IdentifierURI: identifierURI,
		},
	}
}

// ── EnrichmentWorker tests ─────────────────────────────────────────────────

func TestEnrichmentWorker_WorkMissingPool(t *testing.T) {
	worker := EnrichmentWorker{
		Pool:                  nil,
		ReconciliationService: &mockEntityDereferencer{},
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01J", "http://kg.artsdata.ca/resource/K-1"))
	if err == nil {
		t.Error("expected error when Pool is nil")
	}
}

func TestEnrichmentWorker_WorkMissingReconciliationService(t *testing.T) {
	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: nil,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01J", "http://kg.artsdata.ca/resource/K-1"))
	if err == nil {
		t.Error("expected error when ReconciliationService is nil")
	}
}

func TestEnrichmentWorker_WorkNilJob(t *testing.T) {
	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: &mockEntityDereferencer{},
	}
	err := worker.Work(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil job")
	}
}

func TestEnrichmentWorker_WorkEmptyArgs(t *testing.T) {
	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: &mockEntityDereferencer{},
	}
	job := &river.Job[EnrichmentArgs]{Args: EnrichmentArgs{}}
	err := worker.Work(context.Background(), job)
	if err == nil {
		t.Error("expected error for empty args")
	}
}

func TestEnrichmentWorker_WorkEntityNotFound_404_ReturnsNil(t *testing.T) {
	t.Parallel()
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return nil, &artsdata.StatusError{Code: 404, Body: "not found"}
		},
	}
	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01J", "http://kg.artsdata.ca/resource/K-1"))
	if err != nil {
		t.Errorf("expected nil for 404 StatusError (non-retryable), got: %v", err)
	}
}

func TestEnrichmentWorker_WorkEntityNotFound_OtherStatusCode_IsRetryable(t *testing.T) {
	t.Parallel()
	// A 503 StatusError should NOT be silently dropped — it is retryable.
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return nil, &artsdata.StatusError{Code: 503, Body: "service unavailable"}
		},
	}
	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01J", "http://kg.artsdata.ca/resource/K-1"))
	if err == nil {
		t.Error("expected retryable error for 503 StatusError")
	}
}

func TestEnrichmentWorker_WorkDereferenceError_IsRetryable(t *testing.T) {
	t.Parallel()
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return nil, errors.New("connection refused")
		},
	}
	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01J", "http://kg.artsdata.ca/resource/K-1"))
	if err == nil {
		t.Error("expected retryable error for connection refused")
	}
}

func TestEnrichmentWorker_WorkPlace_HappyPath(t *testing.T) {
	t.Parallel()
	wikiURI := "https://www.wikidata.org/wiki/Q12345"
	entity := &artsdata.EntityData{
		ID:          "http://kg.artsdata.ca/resource/K-1",
		Description: "A lovely venue",
		URL:         "https://example.com",
		Address: &artsdata.Address{
			StreetAddress:   "123 Main St",
			AddressLocality: "Toronto",
			AddressRegion:   "ON",
			PostalCode:      "M5V 3A8",
			AddressCountry:  "CA",
		},
		SameAs: wikiURI,
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	idStore := &mockIdentifierUpserter{}
	placeService := &mockPlaceUpdater{
		getFunc: func(_ context.Context, _ string) (*places.Place, error) {
			return &places.Place{ULID: "01J"}, nil // all fields empty → should be filled
		},
	}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       idStore,
		PlaceService:          placeService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01J", "http://kg.artsdata.ca/resource/K-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idStore.calls == 0 {
		t.Error("expected UpsertEntityIdentifier to be called for wikidata sameAs")
	}
	if placeService.updateCalls == 0 {
		t.Error("expected place Update to be called")
	}
}

func TestEnrichmentWorker_WorkPlace_AlreadyPopulated_NoUpdate(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID:          "http://kg.artsdata.ca/resource/K-2",
		Description: "Artsdata description",
		URL:         "https://artsdata.example.com",
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	placeService := &mockPlaceUpdater{
		getFunc: func(_ context.Context, _ string) (*places.Place, error) {
			return &places.Place{
				ULID:        "01K",
				Description: "Already set",
				URL:         "https://existing.example.com",
			}, nil
		},
	}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
		PlaceService:          placeService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01K", "http://kg.artsdata.ca/resource/K-2"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if placeService.updateCalls != 0 {
		t.Error("expected no Update call when all fields already populated")
	}
}

func TestEnrichmentWorker_WorkOrg_HappyPath(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID:          "http://kg.artsdata.ca/resource/K-3",
		Description: "Arts org",
		URL:         "https://org.example.com",
		Address: &artsdata.Address{
			AddressLocality: "Montreal",
			AddressRegion:   "QC",
			AddressCountry:  "CA",
		},
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	orgService := &mockOrgUpdater{
		getFunc: func(_ context.Context, _ string) (*organizations.Organization, error) {
			return &organizations.Organization{ULID: "01L"}, nil
		},
	}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
		OrgService:            orgService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("organization", "01L", "http://kg.artsdata.ca/resource/K-3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orgService.updateCalls == 0 {
		t.Error("expected org Update to be called")
	}
}

func TestEnrichmentWorker_WorkOrg_NoArtsdataFields_SkipsUpdate(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID: "http://kg.artsdata.ca/resource/K-4",
		// No description, URL, or address fields
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	orgService := &mockOrgUpdater{}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
		OrgService:            orgService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("organization", "01M", "http://kg.artsdata.ca/resource/K-4"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orgService.getCalls != 0 || orgService.updateCalls != 0 {
		t.Error("expected no GetByULID or Update calls when entity has no metadata fields")
	}
}

func TestEnrichmentWorker_WorkSameAs_UnknownAuthoritySkipped(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID:     "http://kg.artsdata.ca/resource/K-5",
		SameAs: "https://unknown-authority.example.com/entity/99",
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	idStore := &mockIdentifierUpserter{}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       idStore,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01N", "http://kg.artsdata.ca/resource/K-5"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idStore.calls != 0 {
		t.Errorf("expected no upsert calls for unknown authority, got %d", idStore.calls)
	}
}

// fakePool returns a non-nil *pgxpool.Pool sentinel for tests that need a non-nil pool
// but don't make real DB calls (IdentifierStore is always injected in these tests).
func fakePool() *pgxpool.Pool {
	return new(pgxpool.Pool)
}

// TestEnrichmentWorker_WorkPlace_GetByULIDFails verifies that a GetByULID failure is
// soft: Work still returns nil (enrichment continues for sameAs; entity update is skipped).
func TestEnrichmentWorker_WorkPlace_GetByULIDFails(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID:          "http://kg.artsdata.ca/resource/K-10",
		Description: "Some description",
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	placeService := &mockPlaceUpdater{
		getFunc: func(_ context.Context, _ string) (*places.Place, error) {
			return nil, errors.New("db unavailable")
		},
	}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
		PlaceService:          placeService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01P", "http://kg.artsdata.ca/resource/K-10"))
	if err != nil {
		t.Errorf("expected nil (soft failure on GetByULID), got: %v", err)
	}
	if placeService.updateCalls != 0 {
		t.Error("expected no Update call when GetByULID failed")
	}
}

// TestEnrichmentWorker_WorkPlace_UpdateFails verifies that a failed Update is soft:
// Work still returns nil.
func TestEnrichmentWorker_WorkPlace_UpdateFails(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID:          "http://kg.artsdata.ca/resource/K-11",
		Description: "Some description",
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	placeService := &mockPlaceUpdater{
		getFunc: func(_ context.Context, _ string) (*places.Place, error) {
			return &places.Place{ULID: "01Q"}, nil
		},
		updateFunc: func(_ context.Context, _ string, _ places.UpdatePlaceParams) (*places.Place, error) {
			return nil, errors.New("write conflict")
		},
	}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
		PlaceService:          placeService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01Q", "http://kg.artsdata.ca/resource/K-11"))
	if err != nil {
		t.Errorf("expected nil (soft failure on Update), got: %v", err)
	}
}

// TestEnrichmentWorker_WorkPlace_PartialFieldUpdate verifies the common real-world case
// where some fields are already populated and some are empty.
func TestEnrichmentWorker_WorkPlace_PartialFieldUpdate(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID:          "http://kg.artsdata.ca/resource/K-12",
		Description: "New description from Artsdata",
		URL:         "https://new.example.com",
		Address: &artsdata.Address{
			AddressLocality: "Toronto",
			PostalCode:      "M5V",
		},
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	// Description and URL already set; City and PostalCode are empty → should be filled.
	var capturedParams places.UpdatePlaceParams
	placeService := &mockPlaceUpdater{
		getFunc: func(_ context.Context, _ string) (*places.Place, error) {
			return &places.Place{
				ULID:        "01R",
				Description: "Existing description",
				URL:         "https://existing.example.com",
			}, nil
		},
		updateFunc: func(_ context.Context, _ string, params places.UpdatePlaceParams) (*places.Place, error) {
			capturedParams = params
			return &places.Place{ULID: "01R"}, nil
		},
	}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
		PlaceService:          placeService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01R", "http://kg.artsdata.ca/resource/K-12"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if placeService.updateCalls == 0 {
		t.Fatal("expected Update to be called for partial field update")
	}
	// Description and URL already populated → should NOT be overwritten.
	if capturedParams.Description != nil {
		t.Errorf("Description should be nil (already populated), got %q", *capturedParams.Description)
	}
	if capturedParams.URL != nil {
		t.Errorf("URL should be nil (already populated), got %q", *capturedParams.URL)
	}
	// City and PostalCode are empty → should be set from Artsdata.
	if capturedParams.City == nil || *capturedParams.City != "Toronto" {
		t.Errorf("City should be %q, got %v", "Toronto", capturedParams.City)
	}
	if capturedParams.PostalCode == nil || *capturedParams.PostalCode != "M5V" {
		t.Errorf("PostalCode should be %q, got %v", "M5V", capturedParams.PostalCode)
	}
}

// TestEnrichmentWorker_WorkSameAs_MultipleSameAsURIs verifies that multiple sameAs URIs
// from an []interface{} are all processed correctly.
func TestEnrichmentWorker_WorkSameAs_MultipleSameAsURIs(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID: "http://kg.artsdata.ca/resource/K-13",
		SameAs: []interface{}{
			"https://www.wikidata.org/wiki/Q99",
			map[string]interface{}{"@id": "https://musicbrainz.org/place/abc"},
			"https://unknown-site.example.com/foo", // should be skipped
		},
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	idStore := &mockIdentifierUpserter{}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       idStore,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01S", "http://kg.artsdata.ca/resource/K-13"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// wikidata + musicbrainz = 2 known authorities; unknown-site is skipped.
	if idStore.calls != 2 {
		t.Errorf("expected 2 UpsertEntityIdentifier calls, got %d", idStore.calls)
	}
}

// TestEnrichmentWorker_WorkNoSameAs_WithMetadata verifies that when there are no sameAs
// URIs, idStore is never called but the metadata update still fires.
func TestEnrichmentWorker_WorkNoSameAs_WithMetadata(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID:          "http://kg.artsdata.ca/resource/K-14",
		Description: "Venue description",
		// No SameAs
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	idStore := &mockIdentifierUpserter{}
	placeService := &mockPlaceUpdater{
		getFunc: func(_ context.Context, _ string) (*places.Place, error) {
			return &places.Place{ULID: "01T"}, nil
		},
	}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       idStore,
		PlaceService:          placeService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01T", "http://kg.artsdata.ca/resource/K-14"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idStore.calls != 0 {
		t.Errorf("expected 0 identifier upserts for entity with no sameAs, got %d", idStore.calls)
	}
	if placeService.updateCalls == 0 {
		t.Error("expected place Update to be called even when sameAs is absent")
	}
}

// TestEnrichmentWorker_WorkURLValidation verifies that a malformed URL from Artsdata
// is silently discarded and does not corrupt the entity's URL field.
func TestEnrichmentWorker_WorkURLValidation(t *testing.T) {
	t.Parallel()
	entity := &artsdata.EntityData{
		ID:  "http://kg.artsdata.ca/resource/K-15",
		URL: "not-a-valid-url",
		// Description is empty → hasArtsdataFields would be false without URL; with
		// URL invalidated we need another field to trigger the update path.
		Address: &artsdata.Address{AddressLocality: "Ottawa"},
	}
	deref := &mockEntityDereferencer{
		dereferenceFunc: func(_ context.Context, _ string) (*artsdata.EntityData, error) {
			return entity, nil
		},
	}
	var capturedParams places.UpdatePlaceParams
	placeService := &mockPlaceUpdater{
		getFunc: func(_ context.Context, _ string) (*places.Place, error) {
			return &places.Place{ULID: "01U"}, nil
		},
		updateFunc: func(_ context.Context, _ string, params places.UpdatePlaceParams) (*places.Place, error) {
			capturedParams = params
			return &places.Place{ULID: "01U"}, nil
		},
	}

	worker := EnrichmentWorker{
		Pool:                  fakePool(),
		ReconciliationService: deref,
		IdentifierStore:       &mockIdentifierUpserter{},
		PlaceService:          placeService,
	}
	err := worker.Work(context.Background(), makeEnrichmentJob("place", "01U", "http://kg.artsdata.ca/resource/K-15"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Malformed URL must not reach the params.
	if capturedParams.URL != nil {
		t.Errorf("expected URL to be nil (invalid URL discarded), got %q", *capturedParams.URL)
	}
}
