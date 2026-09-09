package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/kg"
	"github.com/Togather-Foundation/server/internal/kg/artsdata"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var (
	reconcileDryRun bool
	reconcileLimit  int
	reconcileForce  bool
	statsJSON       bool
)

// reconcileCmd represents the reconcile command group
var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Reconcile entities against knowledge graphs",
	Long: `Bulk reconcile places and organizations against Artsdata's knowledge graph.

Reconciliation matches local entities (places, organizations) to authoritative
identifiers from knowledge graphs like Artsdata, Wikidata, and MusicBrainz.

This command connects directly to the database and processes entities in batches.
By default, it only reconciles entities that haven't been reconciled yet.

Examples:
  # Reconcile all unreconciled places
  server reconcile places

  # Reconcile first 100 unreconciled organizations
  server reconcile organizations --limit 100

  # Re-reconcile all places (bypass cache)
  server reconcile places --force

  # Dry run - count entities without reconciling
  server reconcile all --dry-run`,
}

// reconcilePlacesCmd reconciles places
var reconcilePlacesCmd = &cobra.Command{
	Use:   "places",
	Short: "Reconcile places against Artsdata",
	Long: `Reconcile places against Artsdata's knowledge graph.

Queries places that haven't been reconciled yet and attempts to match them
to Artsdata entities based on name and address fields.

Examples:
  # Reconcile all unreconciled places
  server reconcile places

  # Reconcile first 50 places
  server reconcile places --limit 50

  # Re-reconcile all places (bypass cache)
  server reconcile places --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pool, service, err := setupReconciliation()
		if err != nil {
			return err
		}
		defer pool.Close()

		return reconcilePlaces(pool, service)
	},
}

// reconcileOrganizationsCmd reconciles organizations
var reconcileOrganizationsCmd = &cobra.Command{
	Use:   "organizations",
	Short: "Reconcile organizations against Artsdata",
	Long: `Reconcile organizations against Artsdata's knowledge graph.

Queries organizations that haven't been reconciled yet and attempts to match them
to Artsdata entities based on name and URL.

Examples:
  # Reconcile all unreconciled organizations
  server reconcile organizations

  # Reconcile first 50 organizations
  server reconcile organizations --limit 50

  # Re-reconcile all organizations (bypass cache)
  server reconcile organizations --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pool, service, err := setupReconciliation()
		if err != nil {
			return err
		}
		defer pool.Close()

		return reconcileOrganizations(pool, service)
	},
}

// reconcileAllCmd reconciles both places and organizations
var reconcileAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Reconcile all entities (places and organizations)",
	Long: `Reconcile both places and organizations against Artsdata's knowledge graph.

This command runs place reconciliation first, then organization reconciliation.

Examples:
  # Reconcile all unreconciled entities
  server reconcile all

  # Dry run - count entities without reconciling
  server reconcile all --dry-run

  # Reconcile first 100 of each type
  server reconcile all --limit 100`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pool, service, err := setupReconciliation()
		if err != nil {
			return err
		}
		defer pool.Close()

		// Reconcile places first
		if err := reconcilePlaces(pool, service); err != nil {
			return fmt.Errorf("reconcile places: %w", err)
		}

		fmt.Println() // Blank line between sections

		// Then organizations
		if err := reconcileOrganizations(pool, service); err != nil {
			return fmt.Errorf("reconcile organizations: %w", err)
		}

		return nil
	},
}

// reconcileStatsCmd reports reconciliation/enrichment coverage for places and
// organizations without contacting the Artsdata API.
var reconcileStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Report Artsdata reconciliation/enrichment coverage",
	Long: `Report reconciliation and enrichment coverage for places and organizations.

For each entity type, prints:
  total    — rows where deleted_at IS NULL
  matched  — rows with an entity_identifiers row where authority_code = 'artsdata'
  enriched — rows with enriched_at NOT NULL (requires the enriched_at column,
             added by ticket t_027a1bf2; reported as n/a until merged)
  stale    — enriched rows whose enriched_at is older than now() - ARTSDATA_ENRICH_REFRESH_DAYS
             (default 30; requires the enriched_at column)

Use --json for machine-readable output.

Examples:
  server reconcile stats
  server reconcile stats --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pool, _, err := setupReconciliation()
		if err != nil {
			return err
		}
		defer pool.Close()

		return reconcileStats(pool)
	},
}

// entityStats is the coverage row for a single entity type. Enriched and Stale
// are nil when the enriched_at column is absent (capability-detected).
type entityStats struct {
	Label             string `json:"-"`
	Total             int64  `json:"total"`
	Matched           int64  `json:"matched"`
	Enriched          *int64 `json:"enriched"`
	Stale             *int64 `json:"stale"`
	EnrichedAvailable bool   `json:"enriched_available"`
}

func reconcileStats(pool *pgxpool.Pool) error {
	ctx := context.Background()

	// ARTSDATA_ENRICH_REFRESH_DAYS is introduced alongside the enriched_at column
	// (ticket t_027a1bf2); read the env var directly with a 30-day default so this
	// command compiles and runs against current main without the config field.
	refreshDays := 30
	if v := os.Getenv("ARTSDATA_ENRICH_REFRESH_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			refreshDays = n
		}
	}

	entities := []struct {
		table      string
		entityType string
	}{
		{"places", "place"},
		{"organizations", "organization"},
	}

	stats := make([]entityStats, 0, len(entities))
	for _, e := range entities {
		row := entityStats{Label: e.table, EnrichedAvailable: true}

		total, err := countActive(ctx, pool, e.table)
		if err != nil {
			return fmt.Errorf("count %s: %w", e.table, err)
		}
		row.Total = total

		matched, err := countMatched(ctx, pool, e.table, e.entityType)
		if err != nil {
			return fmt.Errorf("count matched %s: %w", e.table, err)
		}
		row.Matched = matched

		hasCol, err := columnExists(ctx, pool, e.table, "enriched_at")
		if err != nil {
			return fmt.Errorf("check enriched_at on %s: %w", e.table, err)
		}
		if hasCol {
			enriched, err := countEnriched(ctx, pool, e.table)
			if err != nil {
				return fmt.Errorf("count enriched %s: %w", e.table, err)
			}
			stale, err := countStale(ctx, pool, e.table, refreshDays)
			if err != nil {
				return fmt.Errorf("count stale %s: %w", e.table, err)
			}
			row.Enriched = &enriched
			row.Stale = &stale
		} else {
			row.EnrichedAvailable = false
		}

		stats = append(stats, row)
	}

	if statsJSON {
		return printStatsJSON(stats)
	}
	printStatsTable(stats)
	return nil
}

func printStatsTable(stats []entityStats) {
	fmt.Printf("%-14s %10s %10s %10s %10s\n", "entity", "total", "matched", "enriched", "stale")
	for _, s := range stats {
		enriched, stale := "n/a", "n/a"
		if s.EnrichedAvailable {
			enriched = strconv.FormatInt(*s.Enriched, 10)
			stale = strconv.FormatInt(*s.Stale, 10)
		}
		fmt.Printf("%-14s %10d %10d %10s %10s\n", s.Label, s.Total, s.Matched, enriched, stale)
	}
	for _, s := range stats {
		if !s.EnrichedAvailable {
			fmt.Printf("enriched/stale n/a for %s: requires enriched_at column (ticket t_027a1bf2)\n", s.Label)
		}
	}
}

func printStatsJSON(stats []entityStats) error {
	out := make(map[string]entityStats, len(stats))
	for _, s := range stats {
		out[s.Label] = s
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal stats: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

// countActive returns the number of non-deleted rows in table. The table name is
// from a hardcoded whitelist and is quoted via pgx.Identifier.
func countActive(ctx context.Context, pool *pgxpool.Pool, table string) (int64, error) {
	q := fmt.Sprintf("SELECT COUNT(*)::bigint FROM %s WHERE deleted_at IS NULL", pgx.Identifier{table}.Sanitize())
	var n int64
	if err := pool.QueryRow(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// countMatched returns the number of non-deleted rows in table that carry an
// Artsdata entity_identifier.
func countMatched(ctx context.Context, pool *pgxpool.Pool, table, entityType string) (int64, error) {
	q := fmt.Sprintf(`
		SELECT COUNT(DISTINCT e.ulid)::bigint
		FROM %s e
		JOIN entity_identifiers ei
		  ON ei.entity_type = $1 AND ei.entity_id = e.ulid AND ei.authority_code = 'artsdata'
		WHERE e.deleted_at IS NULL`, pgx.Identifier{table}.Sanitize())
	var n int64
	if err := pool.QueryRow(ctx, q, entityType).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// columnExists reports whether a public-schema column exists on table, allowing
// the enriched/stale stats to degrade gracefully until the enriched_at column
// merges (ticket t_027a1bf2).
func columnExists(ctx context.Context, pool *pgxpool.Pool, table, column string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists)
	return exists, err
}

func countEnriched(ctx context.Context, pool *pgxpool.Pool, table string) (int64, error) {
	q := fmt.Sprintf("SELECT COUNT(*)::bigint FROM %s WHERE deleted_at IS NULL AND enriched_at IS NOT NULL", pgx.Identifier{table}.Sanitize())
	var n int64
	if err := pool.QueryRow(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func countStale(ctx context.Context, pool *pgxpool.Pool, table string, refreshDays int) (int64, error) {
	q := fmt.Sprintf("SELECT COUNT(*)::bigint FROM %s WHERE deleted_at IS NULL AND enriched_at IS NOT NULL AND enriched_at < now() - make_interval(days => $1)", pgx.Identifier{table}.Sanitize())
	var n int64
	if err := pool.QueryRow(ctx, q, refreshDays).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func init() {
	// Add reconcile command group to root
	rootCmd.AddCommand(reconcileCmd)

	// Add subcommands
	reconcileCmd.AddCommand(reconcilePlacesCmd)
	reconcileCmd.AddCommand(reconcileOrganizationsCmd)
	reconcileCmd.AddCommand(reconcileAllCmd)
	reconcileCmd.AddCommand(reconcileStatsCmd)

	// Add persistent flags to parent so they're available to all subcommands
	reconcileCmd.PersistentFlags().BoolVar(&reconcileDryRun, "dry-run", false, "count entities without reconciling")
	reconcileCmd.PersistentFlags().IntVar(&reconcileLimit, "limit", 0, "max entities to process (0 = all)")
	reconcileCmd.PersistentFlags().BoolVar(&reconcileForce, "force", false, "re-reconcile even cached entities")
	reconcileStatsCmd.Flags().BoolVar(&statsJSON, "json", false, "print stats as JSON")
}

// setupReconciliation initializes database connection and reconciliation service
func setupReconciliation() (*pgxpool.Pool, *kg.ReconciliationService, error) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	// Get database URL
	dbURL := getDatabaseURL()
	if dbURL == "" {
		return nil, nil, fmt.Errorf("DATABASE_URL not set\n\nTried loading from:\n  - Environment variable DATABASE_URL\n  - .env file in project root\n  - deploy/docker/.env\n\nPlease set DATABASE_URL or create a .env file")
	}

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}

	// Create queries
	queries := postgres.New(pool)

	// Setup logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Use config values with fallback to defaults
	endpoint := cfg.Artsdata.Endpoint
	if endpoint == "" {
		endpoint = artsdata.DefaultEndpoint
	}

	rateLimit := cfg.Artsdata.RateLimitPerSec
	if rateLimit == 0 {
		rateLimit = 1.0
	}

	cacheTTL := time.Duration(cfg.Artsdata.CacheTTLDays) * 24 * time.Hour
	if cacheTTL == 0 {
		cacheTTL = 30 * 24 * time.Hour
	}

	failureTTL := time.Duration(cfg.Artsdata.FailureTTLDays) * 24 * time.Hour
	if failureTTL == 0 {
		failureTTL = 7 * 24 * time.Hour
	}

	// Create Artsdata client
	artsdataClient := artsdata.NewClient(endpoint, artsdata.WithRateLimit(rateLimit))

	// Create reconciliation service
	service := kg.NewReconciliationService(artsdataClient, queries, logger, cacheTTL, failureTTL)

	return pool, service, nil
}

// reconcilePlaces reconciles places against Artsdata
func reconcilePlaces(pool *pgxpool.Pool, service *kg.ReconciliationService) error {
	ctx := context.Background()

	// Build query based on --force flag
	var query string
	if reconcileForce {
		// Reconcile all places (force re-reconciliation)
		query = `
			SELECT p.ulid, p.name, p.street_address, p.address_locality, p.address_region, p.postal_code, p.address_country
			FROM places p
			WHERE p.deleted_at IS NULL
			ORDER BY p.created_at DESC
		`
	} else {
		// Only reconcile unreconciled places
		query = `
			SELECT p.ulid, p.name, p.street_address, p.address_locality, p.address_region, p.postal_code, p.address_country
			FROM places p
			LEFT JOIN entity_identifiers ei ON ei.entity_type = 'place' AND ei.entity_id = p.ulid AND ei.authority_code = 'artsdata'
			WHERE p.deleted_at IS NULL
			  AND ei.id IS NULL
			ORDER BY p.created_at DESC
		`
	}

	// Apply limit if specified (use parameterized query for safety)
	var rows pgx.Rows
	var err error
	if reconcileLimit > 0 {
		query = query + " LIMIT $1"
		rows, err = pool.Query(ctx, query, reconcileLimit)
	} else {
		rows, err = pool.Query(ctx, query)
	}
	if err != nil {
		return fmt.Errorf("query places: %w", err)
	}
	defer rows.Close()

	// Collect places
	type place struct {
		ulid            string
		name            string
		streetAddress   *string
		addressLocality *string
		addressRegion   *string
		postalCode      *string
		addressCountry  *string
	}

	var places []place
	for rows.Next() {
		var p place
		if err := rows.Scan(&p.ulid, &p.name, &p.streetAddress, &p.addressLocality, &p.addressRegion, &p.postalCode, &p.addressCountry); err != nil {
			return fmt.Errorf("scan place row: %w", err)
		}
		places = append(places, p)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate place rows: %w", err)
	}

	// Dry run mode - just report counts
	if reconcileDryRun {
		fmt.Printf("Dry run - would reconcile:\n")
		fmt.Printf("  Places: %d unreconciled\n", len(places))
		return nil
	}

	// Reconcile each place
	fmt.Printf("Reconciling places against Artsdata...\n")

	var matched, noMatch, errors int
	for i, p := range places {
		// Build properties map
		props := make(map[string]string)
		if p.addressLocality != nil && *p.addressLocality != "" {
			props["addressLocality"] = *p.addressLocality
		}
		if p.postalCode != nil && *p.postalCode != "" {
			props["postalCode"] = *p.postalCode
		}

		// Build reconciliation request
		req := kg.ReconcileRequest{
			EntityType: "place",
			EntityID:   p.ulid,
			Name:       p.name,
			Properties: props,
		}

		// Reconcile
		results, err := service.ReconcileEntity(ctx, req)
		if err != nil {
			fmt.Printf("  [%d/%d] %s -> error: %v\n", i+1, len(places), p.name, err)
			errors++
			continue
		}

		// Report result
		if len(results) > 0 {
			// Found a match
			topMatch := results[0]
			fmt.Printf("  [%d/%d] %s -> matched (%s, confidence: %.2f)\n",
				i+1, len(places), p.name,
				extractArtsdataID(topMatch.IdentifierURI),
				topMatch.Confidence,
			)
			matched++
		} else {
			// No match
			fmt.Printf("  [%d/%d] %s -> no match\n", i+1, len(places), p.name)
			noMatch++
		}
	}

	// Print summary
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total: %d\n", len(places))
	fmt.Printf("  Matched: %d\n", matched)
	fmt.Printf("  No match: %d\n", noMatch)
	fmt.Printf("  Errors: %d\n", errors)

	return nil
}

// reconcileOrganizations reconciles organizations against Artsdata
func reconcileOrganizations(pool *pgxpool.Pool, service *kg.ReconciliationService) error {
	ctx := context.Background()

	// Build query based on --force flag
	var query string
	if reconcileForce {
		// Reconcile all organizations (force re-reconciliation)
		query = `
			SELECT o.ulid, o.name, o.url
			FROM organizations o
			WHERE o.deleted_at IS NULL
			ORDER BY o.created_at DESC
		`
	} else {
		// Only reconcile unreconciled organizations
		query = `
			SELECT o.ulid, o.name, o.url
			FROM organizations o
			LEFT JOIN entity_identifiers ei ON ei.entity_type = 'organization' AND ei.entity_id = o.ulid AND ei.authority_code = 'artsdata'
			WHERE o.deleted_at IS NULL
			  AND ei.id IS NULL
			ORDER BY o.created_at DESC
		`
	}

	// Apply limit if specified (use parameterized query for safety)
	var rows pgx.Rows
	var err error
	if reconcileLimit > 0 {
		query = query + " LIMIT $1"
		rows, err = pool.Query(ctx, query, reconcileLimit)
	} else {
		rows, err = pool.Query(ctx, query)
	}
	if err != nil {
		return fmt.Errorf("query organizations: %w", err)
	}
	defer rows.Close()

	// Collect organizations
	type org struct {
		ulid string
		name string
		url  *string
	}

	var orgs []org
	for rows.Next() {
		var o org
		if err := rows.Scan(&o.ulid, &o.name, &o.url); err != nil {
			return fmt.Errorf("scan organization row: %w", err)
		}
		orgs = append(orgs, o)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate organization rows: %w", err)
	}

	// Dry run mode - just report counts
	if reconcileDryRun {
		fmt.Printf("Dry run - would reconcile:\n")
		fmt.Printf("  Organizations: %d unreconciled\n", len(orgs))
		return nil
	}

	// Reconcile each organization
	fmt.Printf("Reconciling organizations against Artsdata...\n")

	var matched, noMatch, errors int
	for i, o := range orgs {
		// Build properties map
		props := make(map[string]string)

		// Build URL string
		url := ""
		if o.url != nil {
			url = *o.url
		}

		// Build reconciliation request
		req := kg.ReconcileRequest{
			EntityType: "organization",
			EntityID:   o.ulid,
			Name:       o.name,
			Properties: props,
			URL:        url,
		}

		// Reconcile
		results, err := service.ReconcileEntity(ctx, req)
		if err != nil {
			fmt.Printf("  [%d/%d] %s -> error: %v\n", i+1, len(orgs), o.name, err)
			errors++
			continue
		}

		// Report result
		if len(results) > 0 {
			// Found a match
			topMatch := results[0]
			fmt.Printf("  [%d/%d] %s -> matched (%s, confidence: %.2f)\n",
				i+1, len(orgs), o.name,
				extractArtsdataID(topMatch.IdentifierURI),
				topMatch.Confidence,
			)
			matched++
		} else {
			// No match
			fmt.Printf("  [%d/%d] %s -> no match\n", i+1, len(orgs), o.name)
			noMatch++
		}
	}

	// Print summary
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total: %d\n", len(orgs))
	fmt.Printf("  Matched: %d\n", matched)
	fmt.Printf("  No match: %d\n", noMatch)
	fmt.Printf("  Errors: %d\n", errors)

	return nil
}

// extractArtsdataID extracts the ID portion from an Artsdata URI
// e.g., "http://kg.artsdata.ca/resource/K11-211" -> "K11-211"
func extractArtsdataID(uri string) string {
	// Simple extraction: take everything after the last '/'
	for i := len(uri) - 1; i >= 0; i-- {
		if uri[i] == '/' {
			return uri[i+1:]
		}
	}
	return uri
}
