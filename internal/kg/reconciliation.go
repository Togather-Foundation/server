package kg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ArtsdataClient defines the subset of the Artsdata client API used by ReconciliationService.
// This interface is defined by the consumer (idiomatic Go), allowing the concrete
// *artsdata.Client to be swapped with a mock in tests.
type ArtsdataClient interface {
	Reconcile(ctx context.Context, queries map[string]artsdata.ReconciliationQuery) (map[string][]artsdata.ReconciliationResult, error)
	Dereference(ctx context.Context, uri string) (*artsdata.EntityData, error)
}

// compile-time assertion: *artsdata.Client must satisfy ArtsdataClient.
var _ ArtsdataClient = (*artsdata.Client)(nil)

// ReconciliationCacheStore defines the cache persistence methods used by ReconciliationService.
// Defined by the consumer (idiomatic Go); *postgres.Queries satisfies it.
type ReconciliationCacheStore interface {
	GetReconciliationCache(ctx context.Context, arg postgres.GetReconciliationCacheParams) (postgres.ReconciliationCache, error)
	UpsertReconciliationCache(ctx context.Context, arg postgres.UpsertReconciliationCacheParams) (postgres.ReconciliationCache, error)
	UpsertEntityIdentifier(ctx context.Context, arg postgres.UpsertEntityIdentifierParams) (postgres.EntityIdentifier, error)
}

// compile-time assertion: *postgres.Queries must satisfy ReconciliationCacheStore.
var _ ReconciliationCacheStore = (*postgres.Queries)(nil)

// ReconciliationService orchestrates entity reconciliation against knowledge graphs.
type ReconciliationService struct {
	artsdataClient ArtsdataClient
	cache          ReconciliationCacheStore
	logger         *slog.Logger
	cacheTTL       time.Duration // positive cache TTL (default 30 days)
	failureTTL     time.Duration // negative cache TTL (default 7 days)
}

// NewReconciliationService creates a new reconciliation service.
func NewReconciliationService(
	artsdataClient ArtsdataClient,
	cache ReconciliationCacheStore,
	logger *slog.Logger,
	cacheTTL time.Duration,
	failureTTL time.Duration,
) *ReconciliationService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReconciliationService{
		artsdataClient: artsdataClient,
		cache:          cache,
		logger:         logger,
		cacheTTL:       cacheTTL,
		failureTTL:     failureTTL,
	}
}

// ReconcileRequest represents a request to reconcile an entity.
type ReconcileRequest struct {
	EntityType string            // "place" or "organization"
	EntityID   string            // ULID
	Name       string            // Entity name
	Properties map[string]string // e.g., {"addressLocality": "Toronto", "postalCode": "M6J"}
	URL        string            // Entity URL (optional)
}

// MatchResult represents a matched identifier from reconciliation.
type MatchResult struct {
	AuthorityCode string   `json:"authority_code"` // e.g., "artsdata"
	IdentifierURI string   `json:"identifier_uri"` // e.g., "http://kg.artsdata.ca/resource/K11-211"
	Confidence    float64  `json:"confidence"`     // 0.0-1.0
	Method        string   `json:"method"`         // "auto_high", "auto_low"
	SameAsURIs    []string `json:"same_as_uris"`   // transitive sameAs from the matched entity
}

// ReconcileEntity reconciles an entity against Artsdata.
// Returns matched identifiers (may be empty if no match found).
func (s *ReconciliationService) ReconcileEntity(ctx context.Context, req ReconcileRequest) ([]MatchResult, error) {
	// 1. Build lookup key from request
	lookupKey := NormalizeLookupKey(req.EntityType, req.Name, req.Properties)

	// 2. Check reconciliation_cache
	cached, err := s.cache.GetReconciliationCache(ctx, postgres.GetReconciliationCacheParams{
		EntityType:    req.EntityType,
		AuthorityCode: "artsdata",
		LookupKey:     lookupKey,
	})

	if err == nil {
		// Cache hit
		s.logger.DebugContext(ctx, "reconciliation cache hit",
			"entity_type", req.EntityType,
			"entity_id", req.EntityID,
			"lookup_key", lookupKey,
		)

		// Parse cached result
		var cachedResults []MatchResult
		if err := json.Unmarshal(cached.ResultJson, &cachedResults); err != nil {
			return nil, fmt.Errorf("parse cached result: %w", err)
		}

		// If negative cache, return empty
		if cached.IsNegative {
			return []MatchResult{}, nil
		}

		return cachedResults, nil
	} else if err != pgx.ErrNoRows {
		// Actual DB error (not just cache miss) - return it
		return nil, fmt.Errorf("check reconciliation cache: %w", err)
	}

	// 3. Build W3C query
	var query artsdata.ReconciliationQuery
	switch req.EntityType {
	case "place":
		query = BuildPlaceQuery(req.Name, req.Properties["addressLocality"], req.Properties["postalCode"])
	case "organization":
		query = BuildOrgQuery(req.Name, req.URL)
	default:
		return nil, fmt.Errorf("unsupported entity type: %s", req.EntityType)
	}

	// 4. Call Artsdata reconciliation API
	queries := map[string]artsdata.ReconciliationQuery{
		"q0": query,
	}

	results, err := s.artsdataClient.Reconcile(ctx, queries)
	if err != nil {
		return nil, fmt.Errorf("artsdata reconciliation: %w", err)
	}

	apiResults := results["q0"]

	// 5. Apply confidence thresholds and classify
	var matches []MatchResult
	var topResult *artsdata.ReconciliationResult

	for i := range apiResults {
		result := &apiResults[i]
		// Artsdata scores are unbounded (exact matches ~1000+, partial ~3-6).
		// Normalize to 0.0-1.0: exact matches (match=true) get 0.99,
		// others are capped based on score relative to a reference threshold.
		confidence := normalizeArtsdataScore(result.Score, result.Match)
		method := ClassifyConfidence(confidence, result.Match)

		if method == "reject" {
			continue // Skip low confidence results
		}

		// Track top result for dereferencing
		if topResult == nil || result.Score > topResult.Score {
			topResult = result
		}

		matches = append(matches, MatchResult{
			AuthorityCode: "artsdata",
			IdentifierURI: result.ID,
			Confidence:    confidence,
			Method:        method,
			SameAsURIs:    []string{}, // Will be populated from dereference
		})
	}

	// 6. For high confidence matches, dereference to get sameAs links
	if topResult != nil && len(matches) > 0 {
		entity, err := s.artsdataClient.Dereference(ctx, topResult.ID)
		if err != nil {
			s.logger.WarnContext(ctx, "failed to dereference entity",
				"uri", topResult.ID,
				"error", err,
			)
		} else {
			sameAsURIs := artsdata.ExtractSameAsURIs(entity)
			// Find the match corresponding to topResult and assign sameAs to it
			for i := range matches {
				if matches[i].IdentifierURI == topResult.ID {
					matches[i].SameAsURIs = sameAsURIs
					break
				}
			}

			s.logger.DebugContext(ctx, "extracted sameAs URIs",
				"uri", topResult.ID,
				"same_as", sameAsURIs,
			)
		}
	}

	// 7. Store results in entity_identifiers via SQLc
	for i := range matches {
		match := &matches[i]
		if err := s.storeIdentifier(ctx, req.EntityID, req.EntityType, match); err != nil {
			s.logger.ErrorContext(ctx, "failed to store identifier",
				"entity_id", req.EntityID,
				"identifier_uri", match.IdentifierURI,
				"error", err,
			)
			continue
		}

		// Store transitive sameAs URIs as additional identifiers
		for _, sameAsURI := range match.SameAsURIs {
			// Determine authority code from URI
			authCode := inferAuthorityCode(sameAsURI)
			if authCode == "" {
				continue // Skip unknown authorities
			}

			transMatch := &MatchResult{
				AuthorityCode: authCode,
				IdentifierURI: sameAsURI,
				Confidence:    match.Confidence, // Inherit confidence
				Method:        match.Method,
				SameAsURIs:    []string{}, // Don't recurse
			}

			if err := s.storeIdentifier(ctx, req.EntityID, req.EntityType, transMatch); err != nil {
				s.logger.WarnContext(ctx, "failed to store transitive identifier",
					"entity_id", req.EntityID,
					"identifier_uri", sameAsURI,
					"error", err,
				)
			}
		}
	}

	// 8. Cache the result
	isNegative := len(matches) == 0
	cacheTTL := s.cacheTTL
	if isNegative {
		cacheTTL = s.failureTTL
	}

	resultJSON, err := json.Marshal(matches)
	if err != nil {
		return nil, fmt.Errorf("marshal results for cache: %w", err)
	}

	expiresAt := pgtype.Timestamptz{Time: time.Now().Add(cacheTTL), Valid: true}
	_, err = s.cache.UpsertReconciliationCache(ctx, postgres.UpsertReconciliationCacheParams{
		EntityType:    req.EntityType,
		AuthorityCode: "artsdata",
		LookupKey:     lookupKey,
		ResultJson:    resultJSON,
		IsNegative:    isNegative,
		ExpiresAt:     expiresAt,
	})

	if err != nil {
		s.logger.WarnContext(ctx, "failed to cache reconciliation result",
			"entity_type", req.EntityType,
			"lookup_key", lookupKey,
			"error", err,
		)
	}

	return matches, nil
}

// storeIdentifier stores an entity identifier in the database.
func (s *ReconciliationService) storeIdentifier(ctx context.Context, entityID, entityType string, match *MatchResult) error {
	// Convert confidence to pgtype.Numeric
	confidenceStr := fmt.Sprintf("%.6f", match.Confidence)
	var confidence pgtype.Numeric
	if err := confidence.Scan(confidenceStr); err != nil {
		return fmt.Errorf("convert confidence: %w", err)
	}

	// Store metadata as JSON
	metadata := map[string]interface{}{
		"same_as": match.SameAsURIs,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	// Determine if this is the canonical identifier
	// For now, use the highest confidence match as canonical
	isCanonical := match.Method == "auto_high"

	_, err = s.cache.UpsertEntityIdentifier(ctx, postgres.UpsertEntityIdentifierParams{
		EntityType:           entityType,
		EntityID:             entityID,
		AuthorityCode:        match.AuthorityCode,
		IdentifierUri:        match.IdentifierURI,
		Confidence:           confidence,
		ReconciliationMethod: match.Method,
		IsCanonical:          isCanonical,
		Metadata:             metadataJSON,
	})

	return err
}

// BuildPlaceQuery builds a W3C reconciliation query for a Place.
// Note: city and postalCode are accepted for cache key differentiation but NOT sent
// as W3C properties because Artsdata's reconciliation endpoint returns HTTP 500 when
// any properties are included in the query. When Artsdata adds property support, these
// can be sent as schema:address/schema:addressLocality and schema:address/schema:postalCode.
func BuildPlaceQuery(name, city, postalCode string) artsdata.ReconciliationQuery {
	return artsdata.ReconciliationQuery{
		Query: name,
		Type:  "schema:Place",
	}
}

// BuildOrgQuery builds a W3C reconciliation query for an Organization.
// Note: url is accepted for cache key differentiation but NOT sent as a W3C property
// because Artsdata's reconciliation endpoint returns HTTP 500 when any properties are
// included in the query. When Artsdata adds property support, this can be sent as schema:url.
func BuildOrgQuery(name, url string) artsdata.ReconciliationQuery {
	return artsdata.ReconciliationQuery{
		Query: name,
		Type:  "schema:Organization",
	}
}

// NormalizeLookupKey creates a normalized cache key from entity properties.
// Format: "entityType|name|prop1=val1|prop2=val2" (sorted, lowercase)
func NormalizeLookupKey(entityType, name string, props map[string]string) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(entityType)),
		strings.ToLower(strings.TrimSpace(name)),
	}

	// Sort properties for consistent cache keys
	var propParts []string
	for k, v := range props {
		if v != "" {
			propParts = append(propParts, fmt.Sprintf("%s=%s",
				strings.ToLower(strings.TrimSpace(k)),
				strings.ToLower(strings.TrimSpace(v)),
			))
		}
	}

	// Sort properties for consistent cache keys
	sort.Strings(propParts)

	parts = append(parts, propParts...)
	return strings.Join(parts, "|")
}

// normalizeArtsdataScore converts Artsdata's unbounded score to a 0.0-1.0 confidence value.
// Artsdata exact matches have match=true with scores in the ~1000+ range.
// Partial matches have match=false with scores in the ~3-12 range.
// We map: match=true → 0.99, otherwise normalize relative to observed score ranges.
func normalizeArtsdataScore(score float64, match bool) float64 {
	if match {
		return 0.99 // Exact match confirmed by Artsdata
	}
	// For non-exact matches, normalize relative to observed score ranges.
	// Typical partial match scores range from ~3 to ~12.
	normalized := score / 15.0
	if normalized > 0.95 {
		normalized = 0.95 // Cap below exact-match threshold
	}
	if normalized < 0.0 {
		normalized = 0.0
	}
	return normalized
}

// ClassifyConfidence determines the reconciliation method based on confidence score and match flag.
func ClassifyConfidence(score float64, match bool) string {
	if score >= 0.95 && match {
		return "auto_high"
	}
	if score >= 0.80 {
		return "auto_low"
	}
	return "reject"
}

// inferAuthorityCode infers the authority code from a URI.
func inferAuthorityCode(uri string) string {
	switch {
	case strings.Contains(uri, "wikidata.org"):
		return "wikidata"
	case strings.Contains(uri, "musicbrainz.org"):
		return "musicbrainz"
	case strings.Contains(uri, "isni.org"):
		return "isni"
	case strings.Contains(uri, "openstreetmap.org"):
		return "osm"
	case strings.Contains(uri, "kg.artsdata.ca"):
		return "artsdata"
	default:
		return "" // Unknown authority
	}
}
