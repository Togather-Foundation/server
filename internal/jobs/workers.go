package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/Togather-Foundation/server/internal/geocoding"
	"github.com/Togather-Foundation/server/internal/kg"
	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/Togather-Foundation/server/internal/metrics"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type DeduplicationArgs struct {
	EventID string `json:"event_id"`
}

func (DeduplicationArgs) Kind() string { return JobKindDeduplication }

type ReconciliationArgs struct {
	EntityType string `json:"entity_type"` // "place" or "organization"
	EntityID   string `json:"entity_id"`   // ULID
}

func (ReconciliationArgs) Kind() string { return JobKindReconciliation }

type EnrichmentArgs struct {
	EntityType    string `json:"entity_type"`    // "place" or "organization"
	EntityID      string `json:"entity_id"`      // ULID
	IdentifierURI string `json:"identifier_uri"` // Artsdata URI to dereference
}

func (EnrichmentArgs) Kind() string { return JobKindEnrichment }

type DeduplicationWorker struct {
	river.WorkerDefaults[DeduplicationArgs]
}

func (DeduplicationWorker) Kind() string { return JobKindDeduplication }

func (DeduplicationWorker) Work(ctx context.Context, job *river.Job[DeduplicationArgs]) error {
	if job == nil {
		return fmt.Errorf("deduplication job missing")
	}
	return nil
}

type ReconciliationWorker struct {
	river.WorkerDefaults[ReconciliationArgs]
	Pool                  *pgxpool.Pool
	ReconciliationService EntityReconciler
	Logger                *slog.Logger
}

func (ReconciliationWorker) Kind() string { return JobKindReconciliation }

func (w ReconciliationWorker) Work(ctx context.Context, job *river.Job[ReconciliationArgs]) error {
	// Validate dependencies
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}
	if w.ReconciliationService == nil {
		return fmt.Errorf("reconciliation service not configured")
	}
	if job == nil {
		return fmt.Errorf("reconciliation job missing")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	args := job.Args
	if args.EntityType == "" || args.EntityID == "" {
		return fmt.Errorf("entity_type and entity_id are required")
	}

	logger.Info("starting reconciliation job",
		"entity_type", args.EntityType,
		"entity_id", args.EntityID,
		"attempt", job.Attempt,
	)

	// Query entity from database based on type
	var name string
	var properties map[string]string
	var url string

	switch args.EntityType {
	case "place":
		const getPlaceQuery = `
			SELECT ulid, name, street_address, address_locality, address_region, postal_code, address_country
			FROM places
			WHERE ulid = $1 AND deleted_at IS NULL
		`

		var (
			ulid          string
			streetAddress *string
			city          *string
			region        *string
			postalCode    *string
			country       *string
		)

		err := w.Pool.QueryRow(ctx, getPlaceQuery, args.EntityID).Scan(
			&ulid, &name, &streetAddress, &city, &region, &postalCode, &country,
		)
		if err != nil {
			logger.Warn("failed to query place",
				"entity_id", args.EntityID,
				"error", err,
			)
			return fmt.Errorf("query place %s: %w", args.EntityID, err)
		}

		// Build properties map for reconciliation
		properties = make(map[string]string)
		if city != nil {
			properties["addressLocality"] = *city
		}
		if postalCode != nil {
			properties["postalCode"] = *postalCode
		}

	case "organization":
		const getOrgQuery = `
			SELECT ulid, name, url
			FROM organizations
			WHERE ulid = $1 AND deleted_at IS NULL
		`

		var ulid string
		var urlPtr *string

		err := w.Pool.QueryRow(ctx, getOrgQuery, args.EntityID).Scan(
			&ulid, &name, &urlPtr,
		)
		if err != nil {
			logger.Warn("failed to query organization",
				"entity_id", args.EntityID,
				"error", err,
			)
			return fmt.Errorf("query organization %s: %w", args.EntityID, err)
		}

		if urlPtr != nil {
			url = *urlPtr
		}
		properties = make(map[string]string)

	default:
		return fmt.Errorf("unsupported entity type: %s", args.EntityType)
	}

	// Build reconciliation request
	req := kg.ReconcileRequest{
		EntityType: args.EntityType,
		EntityID:   args.EntityID,
		Name:       name,
		Properties: properties,
		URL:        url,
	}

	// Call reconciliation service
	matches, err := w.ReconciliationService.ReconcileEntity(ctx, req)
	if err != nil {
		logger.Error("reconciliation failed",
			"entity_type", args.EntityType,
			"entity_id", args.EntityID,
			"error", err,
		)
		return fmt.Errorf("reconcile entity: %w", err)
	}

	logger.Info("reconciliation completed",
		"entity_type", args.EntityType,
		"entity_id", args.EntityID,
		"match_count", len(matches),
	)

	// If high-confidence matches found, enqueue enrichment jobs
	for _, match := range matches {
		if match.Method == "auto_high" {
			// Get River client from context
			riverClient, err := river.ClientFromContextSafely[pgx.Tx](ctx)
			if err != nil {
				logger.Warn("river client not available for enrichment enqueue",
					"entity_id", args.EntityID,
					"identifier_uri", match.IdentifierURI,
					"error", err,
				)
				continue
			}

			// Enqueue enrichment job on reconciliation queue
			_, err = riverClient.Insert(ctx, EnrichmentArgs{
				EntityType:    args.EntityType,
				EntityID:      args.EntityID,
				IdentifierURI: match.IdentifierURI,
			}, &river.InsertOpts{
				Queue:       "reconciliation",
				MaxAttempts: EnrichmentMaxAttempts,
			})

			if err != nil {
				logger.Warn("failed to enqueue enrichment job",
					"entity_id", args.EntityID,
					"identifier_uri", match.IdentifierURI,
					"error", err,
				)
			} else {
				logger.Info("enrichment job enqueued",
					"entity_id", args.EntityID,
					"identifier_uri", match.IdentifierURI,
				)
			}
		}
	}

	// Log top match details if available
	if len(matches) > 0 {
		topMatch := matches[0]
		logger.Info("top reconciliation match",
			"entity_type", args.EntityType,
			"entity_id", args.EntityID,
			"identifier_uri", topMatch.IdentifierURI,
			"confidence", topMatch.Confidence,
			"method", topMatch.Method,
		)
	}

	return nil
}

// EntityReconciler is the subset of kg.ReconciliationService used by ReconciliationWorker.
// Defined here by the consumer to allow mock injection in tests.
type EntityReconciler interface {
	ReconcileEntity(ctx context.Context, req kg.ReconcileRequest) ([]kg.MatchResult, error)
}

// compile-time assertion: *kg.ReconciliationService must satisfy EntityReconciler.
var _ EntityReconciler = (*kg.ReconciliationService)(nil)

// EntityDereferencer is the subset of kg.ReconciliationService used by EnrichmentWorker.
// Defined here by the consumer to allow mock injection in tests.
type EntityDereferencer interface {
	DereferenceEntity(ctx context.Context, uri string) (*artsdata.EntityData, error)
}

// compile-time assertion: *kg.ReconciliationService must satisfy EntityDereferencer.
var _ EntityDereferencer = (*kg.ReconciliationService)(nil)

// KGService combines EntityReconciler and EntityDereferencer for use in NewWorkersWithPool.
// *kg.ReconciliationService satisfies this interface.
type KGService interface {
	EntityReconciler
	EntityDereferencer
}

// compile-time assertion: *kg.ReconciliationService must satisfy KGService.
var _ KGService = (*kg.ReconciliationService)(nil)

// IdentifierUpserter handles upsert of entity identifiers for EnrichmentWorker.
// Defined here by the consumer to allow mock injection in tests.
type IdentifierUpserter interface {
	UpsertEntityIdentifier(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error)
}

// compile-time assertion: *postgres.Queries must satisfy IdentifierUpserter.
var _ IdentifierUpserter = (*postgres.Queries)(nil)

// PlaceUpdater is the subset of places.Service used by EnrichmentWorker.
// Defined here by the consumer to allow mock injection in tests.
type PlaceUpdater interface {
	GetByULID(ctx context.Context, ulid string) (*places.Place, error)
	Update(ctx context.Context, ulid string, params places.UpdatePlaceParams) (*places.Place, error)
}

// OrgUpdater is the subset of organizations.Service used by EnrichmentWorker.
// Defined here by the consumer to allow mock injection in tests.
type OrgUpdater interface {
	GetByULID(ctx context.Context, ulid string) (*organizations.Organization, error)
	Update(ctx context.Context, ulid string, params organizations.UpdateOrganizationParams) (*organizations.Organization, error)
}

// compile-time assertions: the concrete services must satisfy the interfaces.
var _ PlaceUpdater = (*places.Service)(nil)
var _ OrgUpdater = (*organizations.Service)(nil)

type EnrichmentWorker struct {
	river.WorkerDefaults[EnrichmentArgs]
	Pool                  *pgxpool.Pool
	ReconciliationService EntityDereferencer
	IdentifierStore       IdentifierUpserter // optional: defaults to postgres.New(Pool) if nil
	PlaceService          PlaceUpdater
	OrgService            OrgUpdater
	Logger                *slog.Logger
}

func (EnrichmentWorker) Kind() string { return JobKindEnrichment }

func (w EnrichmentWorker) Work(ctx context.Context, job *river.Job[EnrichmentArgs]) error {
	// Validate dependencies
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}
	if w.ReconciliationService == nil {
		return fmt.Errorf("reconciliation service not configured")
	}
	if job == nil {
		return fmt.Errorf("enrichment job missing")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	args := job.Args
	if args.EntityType == "" || args.EntityID == "" || args.IdentifierURI == "" {
		return fmt.Errorf("entity_type, entity_id, and identifier_uri are required")
	}

	logger.Info("enrichment started",
		"entity_type", args.EntityType,
		"entity_id", args.EntityID,
		"identifier_uri", args.IdentifierURI,
		"attempt", job.Attempt,
	)

	// 1. Dereference the identifier URI via the Artsdata client.
	entity, err := w.ReconciliationService.DereferenceEntity(ctx, args.IdentifierURI)
	if err != nil {
		// Non-retryable: 404 / entity not found at URI.
		var se *artsdata.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			logger.Warn("entity not found at URI, skipping enrichment",
				"entity_type", args.EntityType,
				"entity_id", args.EntityID,
				"identifier_uri", args.IdentifierURI,
				"error", err,
			)
			return nil // Non-retryable – drop the job.
		}
		return fmt.Errorf("dereference %s: %w", args.IdentifierURI, err)
	}

	// 2. Extract and store additional sameAs identifiers.
	sameAsURIs := artsdata.ExtractSameAsURIs(entity)

	// Resolve identifier store: prefer injected stub (for tests), fall back to pool.
	var identifierStore IdentifierUpserter
	if w.IdentifierStore != nil {
		identifierStore = w.IdentifierStore
	} else {
		identifierStore = postgres.New(w.Pool)
	}

	var confidence pgtype.Numeric
	if err := confidence.Scan("1.000000"); err != nil {
		return fmt.Errorf("build confidence value: %w", err)
	}

	metadataJSON, err := json.Marshal(map[string]interface{}{"source": "enrichment_sameas"})
	if err != nil {
		return fmt.Errorf("marshal sameAs metadata: %w", err)
	}

	storedSameAs := 0
	for _, uri := range sameAsURIs {
		authCode := kg.InferAuthorityCode(uri)
		if authCode == "" {
			continue // Unknown authority – skip.
		}
		_, err := identifierStore.UpsertEntityIdentifier(ctx, postgres.UpsertEntityIdentifierParams{
			EntityType:           args.EntityType,
			EntityID:             args.EntityID,
			AuthorityCode:        authCode,
			IdentifierUri:        uri,
			Confidence:           confidence,
			ReconciliationMethod: "enrichment_sameas",
			IsCanonical:          false,
			Metadata:             metadataJSON,
		})
		if err != nil {
			logger.Warn("failed to store sameAs identifier",
				"entity_id", args.EntityID,
				"uri", uri,
				"error", err,
			)
			continue
		}
		storedSameAs++
	}

	// 3. Build metadata update from Artsdata fields (conservative: only fill empty fields).
	description := kg.ExtractStringValue(entity.Description)
	urlValue := kg.ExtractStringValue(entity.URL)

	// Validate URL format before storing (reject relative, mailto:, or malformed values).
	if urlValue != "" {
		if _, err := url.ParseRequestURI(urlValue); err != nil {
			logger.Warn("ignoring malformed URL from Artsdata",
				"entity_id", args.EntityID,
				"url", urlValue,
				"error", err,
			)
			urlValue = ""
		}
	}

	var streetAddress, city, region, postalCode, country string
	if entity.Address != nil {
		streetAddress = entity.Address.StreetAddress
		city = entity.Address.AddressLocality
		region = entity.Address.AddressRegion
		postalCode = entity.Address.PostalCode
		country = entity.Address.AddressCountry
	}

	// Check whether there is anything to update at all before querying the entity.
	hasArtsdataFields := description != "" || urlValue != "" ||
		streetAddress != "" || city != "" || region != "" || postalCode != "" || country != ""

	if !hasArtsdataFields {
		logger.Info("enrichment completed – no metadata fields to update",
			"entity_type", args.EntityType,
			"entity_id", args.EntityID,
			"identifier_uri", args.IdentifierURI,
			"same_as_stored", storedSameAs,
		)
		return nil
	}

	// setIfEmpty returns a pointer to candidate only when current is empty and candidate is not.
	// Extracted once to avoid duplication across place/org cases.
	setIfEmpty := func(current, candidate string) *string {
		if current == "" && candidate != "" {
			return &candidate
		}
		return nil
	}

	// 4. Apply metadata update conservatively (only set pointer where Artsdata has a
	// value AND the current entity field is empty).
	switch args.EntityType {
	case "place":
		if w.PlaceService == nil {
			logger.Warn("place service not configured, skipping metadata update",
				"entity_id", args.EntityID,
			)
			break
		}
		current, err := w.PlaceService.GetByULID(ctx, args.EntityID)
		if err != nil {
			logger.Warn("failed to fetch current place for enrichment",
				"entity_id", args.EntityID,
				"error", err,
			)
			break
		}

		params := places.UpdatePlaceParams{}

		params.Description = setIfEmpty(current.Description, description)
		params.URL = setIfEmpty(current.URL, urlValue)
		params.StreetAddress = setIfEmpty(current.StreetAddress, streetAddress)
		params.City = setIfEmpty(current.City, city)
		params.Region = setIfEmpty(current.Region, region)
		params.PostalCode = setIfEmpty(current.PostalCode, postalCode)
		params.Country = setIfEmpty(current.Country, country)

		if params.Description == nil && params.URL == nil && params.StreetAddress == nil &&
			params.City == nil && params.Region == nil && params.PostalCode == nil && params.Country == nil {
			logger.Info("no new place fields to update (all already populated)",
				"entity_id", args.EntityID,
				"identifier_uri", args.IdentifierURI,
			)
			break
		}

		if _, err := w.PlaceService.Update(ctx, args.EntityID, params); err != nil {
			logger.Warn("failed to update place from enrichment",
				"entity_id", args.EntityID,
				"error", err,
			)
		} else {
			logger.Info("place updated from enrichment",
				"entity_id", args.EntityID,
			)
		}

	case "organization":
		if w.OrgService == nil {
			logger.Warn("organization service not configured, skipping metadata update",
				"entity_id", args.EntityID,
			)
			break
		}
		current, err := w.OrgService.GetByULID(ctx, args.EntityID)
		if err != nil {
			logger.Warn("failed to fetch current organization for enrichment",
				"entity_id", args.EntityID,
				"error", err,
			)
			break
		}

		params := organizations.UpdateOrganizationParams{}

		params.Description = setIfEmpty(current.Description, description)
		params.URL = setIfEmpty(current.URL, urlValue)
		params.StreetAddress = setIfEmpty(current.StreetAddress, streetAddress)
		params.AddressLocality = setIfEmpty(current.AddressLocality, city)
		params.AddressRegion = setIfEmpty(current.AddressRegion, region)
		params.PostalCode = setIfEmpty(current.PostalCode, postalCode)
		params.AddressCountry = setIfEmpty(current.AddressCountry, country)

		if params.Description == nil && params.URL == nil && params.StreetAddress == nil &&
			params.AddressLocality == nil && params.AddressRegion == nil &&
			params.PostalCode == nil && params.AddressCountry == nil {
			logger.Info("no new organization fields to update (all already populated)",
				"entity_id", args.EntityID,
				"identifier_uri", args.IdentifierURI,
			)
			break
		}

		if _, err := w.OrgService.Update(ctx, args.EntityID, params); err != nil {
			logger.Warn("failed to update organization from enrichment",
				"entity_id", args.EntityID,
				"error", err,
			)
		} else {
			logger.Info("organization updated from enrichment",
				"entity_id", args.EntityID,
			)
		}

	default:
		logger.Warn("unsupported entity type for enrichment metadata update",
			"entity_type", args.EntityType,
			"entity_id", args.EntityID,
		)
	}

	logger.Info("enrichment completed",
		"entity_type", args.EntityType,
		"entity_id", args.EntityID,
		"identifier_uri", args.IdentifierURI,
		"same_as_stored", storedSameAs,
	)

	return nil
}

// IdempotencyCleanupArgs defines the job for cleaning expired idempotency keys.
type IdempotencyCleanupArgs struct{}

func (IdempotencyCleanupArgs) Kind() string { return JobKindIdempotencyCleanup }

// IdempotencyCleanupWorker removes expired idempotency keys (>24h old).
type IdempotencyCleanupWorker struct {
	river.WorkerDefaults[IdempotencyCleanupArgs]
	Pool   *pgxpool.Pool
	Logger *slog.Logger
	Slot   string // Deployment slot (blue/green) for metrics labeling
}

func (IdempotencyCleanupWorker) Kind() string { return JobKindIdempotencyCleanup }

func (w IdempotencyCleanupWorker) Work(ctx context.Context, job *river.Job[IdempotencyCleanupArgs]) error {
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Track cleanup duration for metrics
	start := time.Now()

	logger.Info("starting idempotency cleanup job",
		"slot", w.Slot,
		"attempt", job.Attempt,
	)

	// Delete expired idempotency keys
	const deleteQuery = `DELETE FROM idempotency_keys WHERE expires_at <= now()`
	result, err := w.Pool.Exec(ctx, deleteQuery)
	duration := time.Since(start).Seconds()

	// Record duration metric (even on error, to track failed attempts)
	if w.Slot != "" {
		metrics.IdempotencyCleanupDuration.WithLabelValues(w.Slot).Observe(duration)
	}

	if err != nil {
		logger.Error("idempotency cleanup job failed",
			"slot", w.Slot,
			"error", err,
			"duration_seconds", duration,
		)
		if w.Slot != "" {
			metrics.IdempotencyCleanupErrors.WithLabelValues(w.Slot, "database_error").Inc()
		}
		return fmt.Errorf("delete expired idempotency keys: %w", err)
	}

	rows := result.RowsAffected()

	// Record successful deletion count
	if w.Slot != "" && rows > 0 {
		metrics.IdempotencyKeysDeleted.WithLabelValues(w.Slot).Add(float64(rows))
	}

	logger.Info("idempotency cleanup job completed",
		"slot", w.Slot,
		"deleted_count", rows,
		"duration_seconds", duration,
	)

	// Optionally record table size (requires additional query)
	// This is useful for tracking table growth over time
	if w.Slot != "" {
		var tableSize int64
		const countQuery = `SELECT COUNT(*) FROM idempotency_keys`
		err := w.Pool.QueryRow(ctx, countQuery).Scan(&tableSize)
		if err != nil {
			logger.Warn("failed to get idempotency_keys table size",
				"slot", w.Slot,
				"error", err,
			)
			// Don't fail the job just because we couldn't get table size
		} else {
			metrics.IdempotencyKeysTableSize.WithLabelValues(w.Slot).Set(float64(tableSize))
			logger.Info("idempotency_keys table size",
				"slot", w.Slot,
				"current_size", tableSize,
			)
		}
	}

	return nil
}

// BatchResultsCleanupArgs defines the job for cleaning expired batch ingestion results.
type BatchResultsCleanupArgs struct{}

func (BatchResultsCleanupArgs) Kind() string { return JobKindBatchResultsCleanup }

// BatchResultsCleanupWorker removes old batch ingestion results (>7 days old).
// This prevents the batch_ingestion_results table from growing indefinitely.
// Clients should poll for results within 7 days of submission.
type BatchResultsCleanupWorker struct {
	river.WorkerDefaults[BatchResultsCleanupArgs]
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

func (BatchResultsCleanupWorker) Kind() string { return JobKindBatchResultsCleanup }

func (w BatchResultsCleanupWorker) Work(ctx context.Context, job *river.Job[BatchResultsCleanupArgs]) error {
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Delete batch results older than 7 days
	const deleteQuery = `DELETE FROM batch_ingestion_results WHERE completed_at < now() - INTERVAL '7 days'`
	result, err := w.Pool.Exec(ctx, deleteQuery)
	if err != nil {
		logger.Error("failed to delete expired batch results", "error", err)
		return fmt.Errorf("delete expired batch results: %w", err)
	}

	rows := result.RowsAffected()
	if rows > 0 {
		logger.Info("cleaned up expired batch results",
			"deleted_count", rows,
		)
	}

	return nil
}

// BatchIngestionArgs defines the job arguments for processing batch event submissions.
// Each batch job processes multiple events asynchronously and stores results
// in the batch_ingestion_results table for status queries.
type BatchIngestionArgs struct {
	// BatchID is the unique identifier for this batch job (ULID format)
	BatchID string `json:"batch_id"`
	// Events is the list of event inputs to process (max 100 per batch)
	Events []events.EventInput `json:"events"`
}

func (BatchIngestionArgs) Kind() string { return JobKindBatchIngestion }

// BatchIngestionWorker processes batch event ingestion requests asynchronously.
// It ingests each event individually, tracks success/failure/duplicate status,
// and stores aggregate results in the database for client polling via GET /batch-status/{id}.
//
// The worker logs structured information about batch progress including counts of
// created, duplicate, and failed events. Individual event failures do not fail the
// entire batch - partial success is supported.
type BatchIngestionWorker struct {
	river.WorkerDefaults[BatchIngestionArgs]
	// IngestService handles individual event ingestion logic
	IngestService *events.IngestService
	// Pool provides database access for storing batch results
	Pool *pgxpool.Pool
	// Logger provides structured logging (defaults to slog.Default() if nil)
	Logger *slog.Logger
	// ReconciliationEnabled controls whether reconciliation jobs are enqueued
	// after successful event ingestion. Set to true only when a ReconciliationService
	// is configured and the ReconciliationWorker is registered.
	ReconciliationEnabled bool
}

func (BatchIngestionWorker) Kind() string { return JobKindBatchIngestion }

func (w BatchIngestionWorker) Work(ctx context.Context, job *river.Job[BatchIngestionArgs]) error {
	if w.IngestService == nil {
		return fmt.Errorf("ingest service not configured")
	}
	if w.Pool == nil {
		return fmt.Errorf("database pool not configured")
	}
	if job == nil {
		return fmt.Errorf("batch ingestion job missing")
	}

	batchID := job.Args.BatchID
	if batchID == "" {
		return fmt.Errorf("batch ID is required")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("starting batch ingestion",
		"batch_id", batchID,
		"event_count", len(job.Args.Events),
		"attempt", job.Attempt,
	)

	// Process each event in the batch
	var successCount, duplicateCount, failureCount int
	results := make([]map[string]any, 0, len(job.Args.Events))
	for i, eventInput := range job.Args.Events {
		result, err := w.IngestService.Ingest(ctx, eventInput)
		itemResult := map[string]any{
			"index": i,
		}

		if err != nil {
			itemResult["status"] = "failed"
			itemResult["error"] = err.Error()
			failureCount++
			logger.Warn("batch event ingestion failed",
				"batch_id", batchID,
				"index", i,
				"error", err,
			)
		} else if result.IsDuplicate {
			itemResult["status"] = "duplicate"
			if result.Event != nil {
				itemResult["event_id"] = result.Event.ULID
			}
			duplicateCount++
		} else {
			itemResult["status"] = "created"
			if result.Event != nil {
				itemResult["event_id"] = result.Event.ULID

				// Log reconciliation-relevant result fields for debugging (srv-titkr)
				logger.Debug("batch event created with reconciliation fields",
					"batch_id", batchID,
					"event_ulid", result.Event.ULID,
					"place_ulid", result.PlaceULID,
					"org_ulid", result.OrganizerULID,
				)

				// Enqueue geocoding job for newly created events (srv-dlrfw)
				// Uses ClientFromContext to get the River client within the worker.
				// Geocoding is not critical — log and continue on failure.
				if riverClient, err := river.ClientFromContextSafely[pgx.Tx](ctx); err != nil {
					logger.Warn("river client not available for geocoding enqueue",
						"batch_id", batchID,
						"event_ulid", result.Event.ULID,
						"error", err,
					)
				} else {
					_, err := riverClient.Insert(ctx, GeocodeEventArgs{EventID: result.Event.ULID}, &river.InsertOpts{
						Queue:       "geocoding",
						MaxAttempts: GeocodingMaxAttempts,
					})
					if err != nil {
						logger.Warn("failed to enqueue geocoding job for batch event",
							"batch_id", batchID,
							"event_ulid", result.Event.ULID,
							"error", err,
						)
					}

					// Enqueue reconciliation jobs for place and org (srv-titkr)
					// Only enqueue when reconciliation is enabled and the worker is registered.
					if w.ReconciliationEnabled {
						if result.PlaceULID != "" {
							_, err := riverClient.Insert(ctx, ReconciliationArgs{
								EntityType: "place",
								EntityID:   result.PlaceULID,
							}, &river.InsertOpts{
								Queue:       "reconciliation",
								MaxAttempts: ReconciliationMaxAttempts,
							})
							if err != nil {
								logger.Warn("failed to enqueue place reconciliation for batch event",
									"batch_id", batchID,
									"place_ulid", result.PlaceULID,
									"error", err,
								)
							}
						}
						if result.OrganizerULID != "" {
							_, err := riverClient.Insert(ctx, ReconciliationArgs{
								EntityType: "organization",
								EntityID:   result.OrganizerULID,
							}, &river.InsertOpts{
								Queue:       "reconciliation",
								MaxAttempts: ReconciliationMaxAttempts,
							})
							if err != nil {
								logger.Warn("failed to enqueue org reconciliation for batch event",
									"batch_id", batchID,
									"org_ulid", result.OrganizerULID,
									"error", err,
								)
							}
						}
					}
				}
			}
			successCount++
		}

		results = append(results, itemResult)
	}

	logger.Info("batch ingestion processing complete",
		"batch_id", batchID,
		"total", len(job.Args.Events),
		"created", successCount,
		"duplicates", duplicateCount,
		"failures", failureCount,
	)

	// Store batch results in a table for status queries using SQLc
	resultsJSON, err := json.Marshal(results)
	if err != nil {
		logger.Error("failed to marshal batch results",
			"batch_id", batchID,
			"error", err,
		)
		return fmt.Errorf("marshal batch results: %w", err)
	}

	// Use SQLc to store batch results
	queries := postgres.New(w.Pool)
	err = queries.CreateBatchIngestionResult(ctx, postgres.CreateBatchIngestionResultParams{
		BatchID:     batchID,
		Results:     resultsJSON,
		CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		logger.Error("failed to store batch results",
			"batch_id", batchID,
			"error", err,
		)
		return fmt.Errorf("store batch results: %w", err)
	}

	logger.Info("batch ingestion completed successfully",
		"batch_id", batchID,
	)

	return nil
}

func NewWorkers() *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker[DeduplicationArgs](workers, DeduplicationWorker{})
	return workers
}

// NewWorkersWithPool creates workers including cleanup jobs that need DB access.
func NewWorkersWithPool(pool *pgxpool.Pool, ingestService *events.IngestService, eventsRepo events.Repository, geocodingService *geocoding.GeocodingService, reconciliationService KGService, placeService PlaceUpdater, orgService OrgUpdater, logger *slog.Logger, slot string) *river.Workers {
	workers := NewWorkers()
	river.AddWorker[IdempotencyCleanupArgs](workers, IdempotencyCleanupWorker{
		Pool:   pool,
		Logger: logger,
		Slot:   slot,
	})
	river.AddWorker[BatchResultsCleanupArgs](workers, BatchResultsCleanupWorker{
		Pool:   pool,
		Logger: logger,
	})
	river.AddWorker[BatchIngestionArgs](workers, BatchIngestionWorker{
		IngestService:         ingestService,
		Pool:                  pool,
		Logger:                logger,
		ReconciliationEnabled: reconciliationService != nil,
	})
	river.AddWorker[ReviewQueueCleanupArgs](workers, ReviewQueueCleanupWorker{
		Repo:   eventsRepo,
		Pool:   pool,
		Logger: logger,
		Slot:   slot,
	})
	river.AddWorker[UsageRollupArgs](workers, UsageRollupWorker{
		Pool:   pool,
		Logger: logger,
		Slot:   slot,
	})
	river.AddWorker[CleanupGeocodingCacheArgs](workers, CleanupGeocodingCacheWorker{
		Pool:             pool,
		Logger:           logger,
		Slot:             slot,
		PreserveTopCount: 10000, // TODO: Read from config
	})

	// Geocoding enrichment workers (srv-qq7o1)
	if geocodingService != nil {
		river.AddWorker[GeocodePlaceArgs](workers, GeocodePlaceWorker{
			Pool:             pool,
			GeocodingService: geocodingService,
			Logger:           logger,
		})
		river.AddWorker[GeocodeEventArgs](workers, GeocodeEventWorker{
			Pool:             pool,
			GeocodingService: geocodingService,
			Logger:           logger,
		})
	}

	// Knowledge graph reconciliation workers (srv-titkr)
	if reconciliationService != nil {
		river.AddWorker[ReconciliationArgs](workers, ReconciliationWorker{
			Pool:                  pool,
			ReconciliationService: reconciliationService,
			Logger:                logger,
		})
		river.AddWorker[EnrichmentArgs](workers, EnrichmentWorker{
			Pool:                  pool,
			ReconciliationService: reconciliationService,
			PlaceService:          placeService,
			OrgService:            orgService,
			Logger:                logger,
		})
	}

	return workers
}
