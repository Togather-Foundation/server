package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// Provenance source-row audit and cleanup.
//
// `sources` rows are created per (base_url, name) and, before feed-URL
// resolution existed, one feed ingested under several names produced several
// rows. Ingest no longer mints a twin for a renamed config, but rows that
// already fragmented stay behind. These commands report the leftovers and
// remove the ones that hold nothing.

type sourceAuditRow struct {
	Name     string `json:"name"`
	BaseURL  string `json:"baseUrl"`
	IsActive bool   `json:"isActive"`
}

type fragmentedFeedRow struct {
	Feed   string `json:"feed"`
	Rows   int    `json:"sourceRows"`
	Events int    `json:"events"`
	// ConfigOwned is true when every source row attached to this feed is a
	// configured scraper source (its name exists in scraper_sources). Those are
	// not defects: the rows are either several configs filtering one upstream
	// feed (the five TPL BiblioCommons views, the two Lu.ma views) or a config
	// that was retargeted and left old attributions behind. A feed with a row no
	// config owns is the historical fragmentation this audit exists to find.
	ConfigOwned bool `json:"configOwned"`
}

type sourceAuditReport struct {
	ZeroEventRows   int `json:"zeroEventSourceRows"`
	FragmentedFeeds int `json:"fragmentedFeeds"`
	FragmentedRows  int `json:"sourceRowsInFragmentedFeeds"`
	// FragmentedConfigOwned counts feeds whose every row is a configured source
	// (nothing to repair — see fragmentedFeedRow.ConfigOwned).
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

A source row is created per (base_url, name). Ingest resolves an existing row
before creating one, but rows that fragmented under older behaviour remain.
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
		Short: "Delete source rows referenced by no events, provenance, changes or API keys",
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

// emptySourcePredicate matches a source row that no event, provenance entry,
// change record or API key refers to. Deleting such a row cannot orphan data.
const emptySourcePredicate = `
       NOT EXISTS (SELECT 1 FROM event_sources   es WHERE es.source_id = s.id)
   AND NOT EXISTS (SELECT 1 FROM field_provenance fp WHERE fp.source_id = s.id)
   AND NOT EXISTS (SELECT 1 FROM event_changes    ec WHERE ec.source_id = s.id)
   AND NOT EXISTS (SELECT 1 FROM api_keys         ak WHERE ak.source_id = s.id)`

func collectSourceAudit(ctx context.Context, pool *pgxpool.Pool, limit int) (*sourceAuditReport, error) {
	report := &sourceAuditReport{}

	if err := pool.QueryRow(ctx, `
SELECT count(*) FROM sources s WHERE `+emptySourcePredicate).Scan(&report.ZeroEventRows); err != nil {
		return nil, fmt.Errorf("count zero-event source rows: %w", err)
	}

	zeroQuery := `
SELECT s.name, coalesce(s.base_url, ''), s.is_active
  FROM sources s
 WHERE ` + emptySourcePredicate + `
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

	// One row per fragmented feed, with a flag saying whether every source row
	// attached to it is a configured scraper source. Two very different things
	// produce a multi-row feed:
	//   - several configs filtering one upstream feed (expected: TPL, Lu.ma);
	//   - a row that no config owns, left behind by an older ingest path
	//     (the defect: "Meetup", "Google Calendar", "Music TORONTO Events").
	// Reporting them as one number trains people to ignore it, so split them.
	fragRows, err := pool.Query(ctx, `
WITH frag AS (
  SELECT rtrim(es.source_url, '/') AS feed,
         count(DISTINCT es.source_id) AS source_rows,
         count(DISTINCT es.event_id) AS events
    FROM event_sources es
   GROUP BY 1
  HAVING count(DISTINCT es.source_id) > 1
)
SELECT left(f.feed, 90), f.source_rows, f.events,
       bool_and(EXISTS (SELECT 1 FROM scraper_sources ss WHERE ss.name = s.name)) AS all_configured
  FROM frag f
  JOIN event_sources es ON rtrim(es.source_url, '/') = f.feed
  JOIN sources s ON s.id = es.source_id
 GROUP BY f.feed, f.source_rows, f.events
 ORDER BY f.events DESC, f.feed`)
	if err != nil {
		return nil, fmt.Errorf("list fragmented feeds: %w", err)
	}
	for fragRows.Next() {
		var row fragmentedFeedRow
		if err := fragRows.Scan(&row.Feed, &row.Rows, &row.Events, &row.ConfigOwned); err != nil {
			fragRows.Close()
			return nil, fmt.Errorf("scan fragmented feed: %w", err)
		}
		report.FragmentedFeeds++
		report.FragmentedRows += row.Rows
		if row.ConfigOwned {
			report.FragmentedConfigOwned++
		} else {
			report.FragmentedUnowned++
		}
		report.Fragments = append(report.Fragments, row)
	}
	if err := fragRows.Err(); err != nil {
		fragRows.Close()
		return nil, fmt.Errorf("iterate fragmented feeds: %w", err)
	}
	fragRows.Close()

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

	report, err := collectSourceAudit(ctx, pool, sourceAuditLimit)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if sourceAuditJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	_, _ = fmt.Fprintf(out, "Zero-event source rows: %d\n", report.ZeroEventRows)
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
	_, _ = fmt.Fprintf(out, "  %d where every row belongs to a configured source (not a defect)\n", report.FragmentedConfigOwned)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := sourceAuditPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	out := cmd.OutOrStdout()

	var candidates int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sources s WHERE `+emptySourcePredicate).Scan(&candidates); err != nil {
		return fmt.Errorf("count prune candidates: %w", err)
	}

	if !sourcePruneDo {
		_, _ = fmt.Fprintf(out, "%d source row(s) reference nothing and would be deleted.\nRun again with --confirm to delete them.\n", candidates)
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	deleted := 0
	rows, err := tx.Query(ctx, `
DELETE FROM sources s
 WHERE `+emptySourcePredicate+`
RETURNING s.name`)
	if err != nil {
		return fmt.Errorf("delete empty source rows: %w", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("scan deleted source row: %w", err)
		}
		deleted++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate deleted source rows: %w", err)
	}
	rows.Close()

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	_, _ = fmt.Fprintf(out, "Deleted %d empty source row(s).\n", deleted)
	return nil
}
