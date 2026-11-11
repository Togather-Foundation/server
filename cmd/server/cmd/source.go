package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// Provenance source-row audit and cleanup.
//
// A canonical row identifies one normalized feed URL. Reconciliation repairs
// old host/name attribution, and pruning protects configuration and metadata.

type sourceAuditRow struct {
	Name     string `json:"name"`
	BaseURL  string `json:"baseUrl"`
	IsActive bool   `json:"isActive"`
}

type fragmentedFeedRow struct {
	Feed   string `json:"feed"`
	Rows   int    `json:"sourceRows"`
	Events int    `json:"events"`
	// ConfigOwned means every row's name exists in scraper_sources. A canonical
	// feed still needs one row, so configured fragments also need reconciliation.
	ConfigOwned bool `json:"configOwned"`
}

type sourceAuditReport struct {
	ZeroEventRows   int                         `json:"zeroEventSourceRows"`
	Counts          postgres.SourceRepairCounts `json:"counts"`
	FragmentedFeeds int                         `json:"fragmentedFeeds"`
	FragmentedRows  int                         `json:"sourceRowsInFragmentedFeeds"`
	// FragmentedConfigOwned counts feeds whose every row is a configured source
	// (these can still require canonical reconciliation).
	FragmentedConfigOwned int `json:"fragmentedConfigOwned"`
	// FragmentedUnowned counts feeds with at least one row no config owns.
	FragmentedUnowned int                 `json:"fragmentedWithUnownedRows"`
	ZeroEvents        []sourceAuditRow    `json:"zeroEventSources"`
	Fragments         []fragmentedFeedRow `json:"fragments"`
}

var (
	sourceCmd = &cobra.Command{
		Use:   "source",
		Short: "Audit and clean up provenance source rows",
		Long: `Audit and clean up provenance source rows.

A source row represents one normalized feed URL. Historical host-level and
name-based rows can be reconciled with the canonical feed registry.
` + "`audit`" + ` reports them; ` + "`prune-empty`" + ` deletes the ones that reference
nothing at all.

Examples:
  server source audit
  server source audit --json
  server source prune-empty                 # dry run
  server source prune-empty --confirm       # actually delete`,
	}

	sourceAuditCmd = &cobra.Command{
		Use:   "audit",
		Short: "Report zero-event and fragmented provenance rows",
		RunE:  runSourceAudit,
	}

	sourcePruneEmptyCmd = &cobra.Command{
		Use:   "prune-empty",
		Short: "Delete unreferenced orphan source rows while protecting config and metadata",
		RunE:  runSourcePruneEmpty,
	}

	sourceAuditJSON  bool
	sourceAuditLimit int
	sourcePruneDo    bool
)

func init() {
	rootCmd.AddCommand(sourceCmd)
	sourceCmd.AddCommand(sourceAuditCmd)
	sourceCmd.AddCommand(sourcePruneEmptyCmd)

	sourceAuditCmd.Flags().BoolVar(&sourceAuditJSON, "json", false, "Output as JSON")
	sourceAuditCmd.Flags().IntVar(&sourceAuditLimit, "limit", 20, "Sample rows to print (0 = all)")

	sourcePruneEmptyCmd.Flags().BoolVar(&sourcePruneDo, "confirm", false, "Delete the rows (without this flag the command only reports)")
}

func sourceAuditPool(ctx context.Context) (*pgxpool.Pool, error) {
	dbURL := getDatabaseURL()
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set (or not found in .env)")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

func collectSourceAudit(ctx context.Context, pool *pgxpool.Pool, limit int, configs []postgres.SourceRepairConfig) (*sourceAuditReport, error) {
	report := &sourceAuditReport{}

	counts, err := postgres.InspectSourceRegistry(ctx, pool, postgres.SourceRepairOptions{Configs: configs})
	if err != nil {
		return nil, err
	}
	report.Counts = counts
	report.ZeroEventRows = counts.ZeroEventRows

	zeroQuery := `
SELECT s.name, coalesce(s.base_url, ''), s.is_active
  FROM sources s
 WHERE NOT EXISTS (SELECT 1 FROM event_sources es WHERE es.source_id=s.id)` + `
 ORDER BY s.name`
	if limit > 0 {
		zeroQuery += fmt.Sprintf(" LIMIT %d", limit)
	}
	zeroRows, err := pool.Query(ctx, zeroQuery)
	if err != nil {
		return nil, fmt.Errorf("list zero-event source rows: %w", err)
	}
	for zeroRows.Next() {
		var row sourceAuditRow
		if err := zeroRows.Scan(&row.Name, &row.BaseURL, &row.IsActive); err != nil {
			zeroRows.Close()
			return nil, fmt.Errorf("scan zero-event source row: %w", err)
		}
		report.ZeroEvents = append(report.ZeroEvents, row)
	}
	if err := zeroRows.Err(); err != nil {
		zeroRows.Close()
		return nil, fmt.Errorf("iterate zero-event source rows: %w", err)
	}
	zeroRows.Close()

	fragments, err := postgres.New(pool).ListSourceAuditFragments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list fragmented feeds: %w", err)
	}
	for _, fragment := range fragments {
		row := fragmentedFeedRow{Feed: fragment.Feed, Rows: int(fragment.SourceRows), Events: int(fragment.Events), ConfigOwned: fragment.AllConfigured}
		report.FragmentedFeeds++
		report.FragmentedRows += row.Rows
		if row.ConfigOwned {
			report.FragmentedConfigOwned++
		} else {
			report.FragmentedUnowned++
		}
		report.Fragments = append(report.Fragments, row)
	}

	return report, nil
}

func runSourceAudit(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := sourceAuditPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	configs, err := loadSourceRepairConfigs()
	if err != nil {
		return err
	}
	report, err := collectSourceAudit(ctx, pool, sourceAuditLimit, configs)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if sourceAuditJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	_, _ = fmt.Fprintf(out, "Zero-event source rows: %d (%d configured, %d orphaned, %d protected)\n", report.ZeroEventRows, report.Counts.ConfiguredIdle, report.Counts.OrphanedIdle, report.Counts.MetadataProtected)
	if len(report.ZeroEvents) > 0 {
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  NAME\tBASE URL\tACTIVE")
		for _, row := range report.ZeroEvents {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%t\n", row.Name, row.BaseURL, row.IsActive)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(out, "\nFeeds served by more than one source row: %d feeds / %d rows\n",
		report.FragmentedFeeds, report.FragmentedRows)
	_, _ = fmt.Fprintf(out, "  %d where every row belongs to a configured source (may require reconciliation)\n", report.FragmentedConfigOwned)
	_, _ = fmt.Fprintf(out, "  %d with a row no config owns (historical fragmentation — needs attention)\n", report.FragmentedUnowned)

	unowned := make([]fragmentedFeedRow, 0, len(report.Fragments))
	for _, row := range report.Fragments {
		if !row.ConfigOwned {
			unowned = append(unowned, row)
		}
	}
	if len(unowned) > 0 {
		_, _ = fmt.Fprintln(out)
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  UNOWNED FEED\tSOURCE ROWS\tEVENTS")
		printed := 0
		for _, row := range unowned {
			if sourceAuditLimit > 0 && printed >= sourceAuditLimit {
				break
			}
			_, _ = fmt.Fprintf(tw, "  %s\t%d\t%d\n", row.Feed, row.Rows, row.Events)
			printed++
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if printed < len(unowned) {
			_, _ = fmt.Fprintf(out, "  ... and %d more (use --json for the full list)\n", len(unowned)-printed)
		}
	}
	return nil
}

func runSourcePruneEmpty(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer cancel()
	configs, err := loadSourceRepairConfigs()
	if err != nil {
		return err
	}
	pool, err := sourceAuditPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	report, err := postgres.PruneEmptySources(ctx, pool, postgres.SourceRepairOptions{Apply: sourcePruneDo, Configs: configs})
	if err != nil {
		return err
	}
	return printSourceRepairReport(cmd, report)
}
