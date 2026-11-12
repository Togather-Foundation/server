package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

var sourceRepairDirectory string
var sourceRepairApply bool
var sourceRepairJSON bool
var sourceRepairUseConfigTrust bool

func init() {
	sourceCmd.PersistentFlags().StringVar(&sourceRepairDirectory, "sources", "configs/sources", "YAML source directory (required for safe config protection)")
	sourceCmd.PersistentFlags().BoolVar(&sourceRepairJSON, "json-report", false, "Output reconciliation or prune report as JSON, including SQL")
	cmd := &cobra.Command{Use: "reconcile", Short: "Repoint historical source provenance to canonical feeds (dry run by default)", RunE: runSourceReconcile}
	cmd.Flags().BoolVar(&sourceRepairApply, "confirm", false, "Commit reconciliation (without this flag all changes are rolled back)")
	cmd.Flags().BoolVar(&sourceRepairUseConfigTrust, "use-config-trust", false, "Explicitly reconcile canonical trust to agreed config trust when licenses match; report each old/new value")
	sourceCmd.AddCommand(cmd)
}

func loadSourceRepairConfigs() ([]postgres.SourceRepairConfig, error) {
	// A missing/incomplete config inventory must never make configured rows
	// look orphaned. Operators can supply the deployed config directory.
	if _, err := os.Stat(sourceRepairDirectory); err != nil {
		return nil, fmt.Errorf("read source config inventory: %w", err)
	}
	configs, err := scraper.LoadSourceConfigs(sourceRepairDirectory, zerolog.Nop())
	if err != nil {
		return nil, fmt.Errorf("load source config inventory: %w", err)
	}
	result := make([]postgres.SourceRepairConfig, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, postgres.SourceRepairConfig{Name: cfg.Name, URLs: cfg.GetURLs(), TrustLevel: cfg.TrustLevel, License: cfg.License, SuppressReviewWarnings: cfg.SuppressReviewWarnings})
	}
	return result, nil
}

func runSourceReconcile(cmd *cobra.Command, args []string) error {
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
	report, err := postgres.ReconcileSources(ctx, pool, postgres.SourceRepairOptions{Apply: sourceRepairApply, Configs: configs, UseConfigTrust: sourceRepairUseConfigTrust})
	if err != nil {
		return err
	}
	return printSourceRepairReport(cmd, report)
}

func printSourceRepairReport(cmd *cobra.Command, report *postgres.SourceRepairReport) error {
	out := cmd.OutOrStdout()
	if sourceRepairJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	mode := "DRY RUN (rolled back)"
	if report.Applied {
		mode = "APPLIED"
	}
	_, err := fmt.Fprintf(out, "%s\nBefore: %d source rows, %d attributions, %d zero-event rows (%d configured, %d orphaned, %d protected)\nAfter:  %d source rows, %d attributions, %d zero-event rows (%d configured, %d orphaned, %d protected)\nMoved: %d attributions, %d field provenance, %d change records; deleted %d orphan rows\n", mode, report.Before.SourceRows, report.Before.AttributionRows, report.Before.ZeroEventRows, report.Before.ConfiguredIdle, report.Before.OrphanedIdle, report.Before.MetadataProtected, report.After.SourceRows, report.After.AttributionRows, report.After.ZeroEventRows, report.After.ConfiguredIdle, report.After.OrphanedIdle, report.After.MetadataProtected, report.AttributionsMoved, report.FieldsMoved, report.ChangesMoved, report.Deleted)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Feed attribution counts before -> after: %d -> %d noncanonical, %d -> %d unregistered, %d -> %d fragmented feeds\n", report.Before.NoncanonicalAttributions, report.After.NoncanonicalAttributions, report.Before.UnregisteredAttributions, report.After.UnregisteredAttributions, report.Before.FragmentedFeeds, report.After.FragmentedFeeds)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Archived %d colliding attributions; archive count %d -> %d (live + archived: %d -> %d)\n", report.AttributionsArchived, report.Before.ArchivedAttributions, report.After.ArchivedAttributions, int64(report.Before.AttributionRows)+report.Before.ArchivedAttributions, int64(report.After.AttributionRows)+report.After.ArchivedAttributions)
	if err != nil {
		return err
	}
	for _, issue := range report.Issues {
		if _, err = fmt.Fprintf(out, "Retained: %s\n", issue); err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintf(out, "Configured trust changes: %d\n", report.TrustChangesCount); err != nil {
		return err
	}
	for _, change := range report.TrustChanges {
		if _, err = fmt.Fprintf(out, "Configured trust: %s (%s), feed %s: %d -> %d\n", change.Name, change.SourceID, change.Feed, change.OldTrust, change.NewTrust); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "SQL used:\n%s\n", report.Query)
	return err
}
