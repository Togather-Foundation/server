package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var (
	loadtestEnv      string
	loadtestSourceID string
	loadtestLegacy   bool
	loadtestDryRun   bool
	loadtestConfirm  bool
)

// productionDBHostEnvVar is the env var operators set to the production DB
// hostname. The cleanup loadtest command refuses to run if the DATABASE_URL
// host matches this value.
const productionDBHostEnvVar = "PRODUCTION_DB_HOST"

// cleanupLoadtestCmd deletes load-test fixture events from the database.
// It is a subcommand of cleanupCmd (server cleanup loadtest ...).
var cleanupLoadtestCmd = &cobra.Command{
	Use:   "loadtest",
	Short: "Delete load-test fixture events from the database",
	Long: `Delete events that were created by the load tester.

Two deletion modes are supported:

  --source-id  Delete events linked to a specific source UUID (preferred;
               requires that the load-test API key was created with --source-id).

  --legacy     Delete events whose image_url or url contain example.com /
               images.example.com placeholder patterns injected by the fixture
               generator before source tagging was implemented.

Both modes require --env=staging to prevent accidental production data loss.
The command also verifies that the DATABASE_URL hostname does not match the
value of the PRODUCTION_DB_HOST environment variable.

Add --dry-run to preview what would be deleted without making any changes.
Add --confirm to actually execute the deletion (required unless --dry-run).

Examples:
  # Preview events that would be deleted (source-id mode)
  server cleanup loadtest --env=staging --source-id=<uuid> --dry-run

  # Delete events linked to a source
  server cleanup loadtest --env=staging --source-id=<uuid> --confirm

  # Preview legacy example.com events
  server cleanup loadtest --env=staging --legacy --dry-run

  # Delete legacy example.com events
  server cleanup loadtest --env=staging --legacy --confirm`,
	RunE: runCleanupLoadtest,
}

func init() {
	cleanupCmd.AddCommand(cleanupLoadtestCmd)

	cleanupLoadtestCmd.Flags().StringVar(&loadtestEnv, "env", "", `target environment; must be "staging" (required)`)
	cleanupLoadtestCmd.Flags().StringVar(&loadtestSourceID, "source-id", "", "UUID of the load-test source whose events should be deleted")
	cleanupLoadtestCmd.Flags().BoolVar(&loadtestLegacy, "legacy", false, "delete events with example.com / images.example.com placeholder URLs")
	cleanupLoadtestCmd.Flags().BoolVar(&loadtestDryRun, "dry-run", false, "show what would be deleted without deleting")
	cleanupLoadtestCmd.Flags().BoolVar(&loadtestConfirm, "confirm", false, "actually execute the deletion (required unless --dry-run)")

}

func runCleanupLoadtest(cmd *cobra.Command, args []string) error {
	// --- Guard 1: --env flag must be provided and equal "staging" ---
	if loadtestEnv == "" {
		return fmt.Errorf("--env is required; use --env=staging")
	}
	if loadtestEnv != "staging" {
		return fmt.Errorf("--env must be \"staging\" (got %q); this command refuses to run against other environments", loadtestEnv)
	}

	// --- Guard 2: at least one deletion mode must be specified ---
	if loadtestSourceID == "" && !loadtestLegacy {
		return fmt.Errorf("specify --source-id=<uuid> or --legacy (or both)")
	}

	// --- Guard 3: either --dry-run or --confirm required ---
	if !loadtestDryRun && !loadtestConfirm {
		return fmt.Errorf("specify --dry-run to preview, or --confirm to execute deletion")
	}

	// --- Connect to database ---
	dbURL := getDatabaseURL()
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL not set")
	}

	// --- Guard 4: DB host must not match production hostname ---
	if err := guardNotProductionDB(dbURL); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// --- Count + delete ---
	if loadtestSourceID != "" {
		if err := handleSourceIDCleanup(ctx, pool, cmd, loadtestSourceID, loadtestDryRun, loadtestConfirm); err != nil {
			return err
		}
	}
	if loadtestLegacy {
		if err := handleLegacyCleanup(ctx, pool, cmd, loadtestDryRun, loadtestConfirm); err != nil {
			return err
		}
	}

	return nil
}

// guardNotProductionDB refuses to proceed when DATABASE_URL points at the
// production database host. The production host is determined by (in order):
//  1. The PRODUCTION_DB_HOST environment variable.
//  2. Any URL whose host contains the substring "prod" (conservative heuristic
//     that matches typical naming conventions like db.prod.example.com).
func guardNotProductionDB(dbURL string) error {
	host, err := dbURLHost(dbURL)
	if err != nil {
		// If we can't parse the URL, refuse to continue.
		return fmt.Errorf("cannot parse DATABASE_URL to verify it is not production: %w", err)
	}

	productionHost := os.Getenv(productionDBHostEnvVar)
	if productionHost != "" {
		if strings.EqualFold(host, productionHost) {
			return fmt.Errorf(
				"DATABASE_URL host %q matches PRODUCTION_DB_HOST %q; refusing to run against production",
				host, productionHost,
			)
		}
		return nil
	}

	// Fallback heuristic: refuse if hostname contains "prod".
	if strings.Contains(strings.ToLower(host), "prod") {
		return fmt.Errorf(
			"DATABASE_URL host %q appears to be a production host (contains \"prod\"); "+
				"set PRODUCTION_DB_HOST to override this heuristic, or use a different DATABASE_URL",
			host,
		)
	}

	return nil
}

// dbURLHost extracts the hostname from a postgres:// or postgresql:// URL, or
// a DSN key=value string.
func dbURLHost(dbURL string) (string, error) {
	trimmed := strings.TrimSpace(dbURL)
	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", fmt.Errorf("parse URL: %w", err)
		}
		return u.Hostname(), nil
	}

	// DSN format: key=value pairs
	for _, part := range strings.Fields(trimmed) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && strings.EqualFold(kv[0], "host") {
			return kv[1], nil
		}
	}

	return "", fmt.Errorf("could not extract host from DATABASE_URL (unsupported format)")
}

// handleSourceIDCleanup counts (and optionally deletes) events linked to the
// given source UUID via the event_sources table.
func handleSourceIDCleanup(ctx context.Context, pool *pgxpool.Pool, cmd *cobra.Command, sourceID string, dryRun, confirm bool) error {
	// Count events linked to this source.
	var count int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT e.id)
		 FROM events e
		 JOIN event_sources es ON es.event_id = e.id
		 WHERE es.source_id = $1`,
		sourceID,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("count events for source %s: %w", sourceID, err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[source-id] source=%s  events=%d\n", sourceID, count)

	if count == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[source-id] nothing to delete")
		return nil
	}

	if dryRun {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[source-id] dry-run: no changes made")
		return nil
	}

	// confirm must be true here (enforced earlier in runCleanupLoadtest).
	result, err := pool.Exec(ctx,
		`DELETE FROM events
		 WHERE id IN (
		   SELECT DISTINCT event_id FROM event_sources WHERE source_id = $1
		 )`,
		sourceID,
	)
	if err != nil {
		return fmt.Errorf("delete events for source %s: %w", sourceID, err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[source-id] deleted %d event(s)\n", result.RowsAffected())
	return nil
}

// handleLegacyCleanup counts (and optionally deletes) events whose image_url
// or public_url matches the fixture-generator placeholder patterns:
//
//	image_url LIKE 'https://images.example.com/events/%.jpg'
//	public_url LIKE 'https://example.com/events/%'
func handleLegacyCleanup(ctx context.Context, pool *pgxpool.Pool, cmd *cobra.Command, dryRun, confirm bool) error {
	var count int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM events
		 WHERE image_url   LIKE 'https://images.example.com/events/%.jpg'
		    OR public_url  LIKE 'https://example.com/events/%'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("count legacy load-test events: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[legacy] events matching example.com patterns: %d\n", count)

	if count == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[legacy] nothing to delete")
		return nil
	}

	if dryRun {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[legacy] dry-run: no changes made")
		return nil
	}

	result, err := pool.Exec(ctx,
		`DELETE FROM events
		 WHERE image_url   LIKE 'https://images.example.com/events/%.jpg'
		    OR public_url  LIKE 'https://example.com/events/%'`,
	)
	if err != nil {
		return fmt.Errorf("delete legacy load-test events: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[legacy] deleted %d event(s)\n", result.RowsAffected())
	return nil
}
