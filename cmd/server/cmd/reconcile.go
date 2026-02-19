package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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

func init() {
	// Add reconcile command group to root
	rootCmd.AddCommand(reconcileCmd)

	// Add subcommands
	reconcileCmd.AddCommand(reconcilePlacesCmd)
	reconcileCmd.AddCommand(reconcileOrganizationsCmd)
	reconcileCmd.AddCommand(reconcileAllCmd)

	// Add persistent flags to parent so they're available to all subcommands
	reconcileCmd.PersistentFlags().BoolVar(&reconcileDryRun, "dry-run", false, "count entities without reconciling")
	reconcileCmd.PersistentFlags().IntVar(&reconcileLimit, "limit", 0, "max entities to process (0 = all)")
	reconcileCmd.PersistentFlags().BoolVar(&reconcileForce, "force", false, "re-reconcile even cached entities")
}

// setupReconciliation initializes database connection and reconciliation service
func setupReconciliation() (*pgxpool.Pool, *kg.ReconciliationService, error) {
	// Load .env files
	config.LoadEnvFile(".env")
	config.LoadEnvFile("deploy/docker/.env")

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
