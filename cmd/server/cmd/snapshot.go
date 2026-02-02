package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/snapshot"
	"github.com/spf13/cobra"
)

var (
	snapshotReason        string
	snapshotRetentionDays int
	snapshotDir           string
	snapshotDryRun        bool
	snapshotForce         bool
	snapshotFormat        string
	snapshotValidate      bool
)

// snapshotCmd represents the snapshot command group
var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage database snapshots",
	Long: `Manage database snapshots for backup and rollback.

Snapshots are compressed pg_dump files with retention management.
Use snapshots before migrations or deployments for safe rollback.

Examples:
  # Create a snapshot before deployment
  server snapshot create --reason "pre-deploy-v1.2.0"

  # List all snapshots
  server snapshot list

  # Clean up old snapshots
  server snapshot cleanup --retention-days 7`,
}

// snapshotCreateCmd creates a new database snapshot
var snapshotCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new database snapshot",
	Long: `Create a compressed database snapshot using pg_dump.

The snapshot will be stored in the configured snapshot directory with
automatic retention management. Metadata is saved alongside the snapshot
for tracking and cleanup.

Requirements:
  - pg_dump must be installed (postgresql-client package)
  - DATABASE_URL environment variable must be set
  - Sufficient disk space for the compressed dump

Examples:
  # Create snapshot with reason
  server snapshot create --reason "pre-migration"

  # Create snapshot with custom retention
  server snapshot create --reason "manual-backup" --retention-days 30

  # Create and validate snapshot integrity
  server snapshot create --reason "important" --validate`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return createSnapshot()
	},
}

// snapshotListCmd lists all snapshots
var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all database snapshots",
	Long: `List all database snapshots with their metadata.

Shows snapshot name, size, age, expiration, and reason.
Snapshots are sorted by creation time (newest first).

Examples:
  # List in table format
  server snapshot list

  # List in JSON format
  server snapshot list --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listSnapshots()
	},
}

// snapshotCleanupCmd cleans up expired snapshots
var snapshotCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up expired snapshots",
	Long: `Delete snapshots older than the retention period.

Snapshots are deleted based on their individual retention periods
(stored in metadata) or the global retention period if not specified.

Safety features:
  - Shows what will be deleted before confirmation
  - Requires confirmation unless --force is used
  - Use --dry-run to preview without deleting

Examples:
  # Preview what will be deleted
  server snapshot cleanup --dry-run

  # Clean up with default retention (7 days)
  server snapshot cleanup

  # Clean up with custom retention
  server snapshot cleanup --retention-days 14

  # Clean up without confirmation
  server snapshot cleanup --force`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cleanupSnapshots()
	},
}

func init() {
	rootCmd.AddCommand(snapshotCmd)

	// Add subcommands
	snapshotCmd.AddCommand(snapshotCreateCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotCleanupCmd)

	// Global snapshot flags
	snapshotCmd.PersistentFlags().StringVar(&snapshotDir, "snapshot-dir", "", "snapshot directory (default: ./snapshots)")

	// Create flags
	snapshotCreateCmd.Flags().StringVar(&snapshotReason, "reason", "manual", "reason for snapshot (used in filename)")
	snapshotCreateCmd.Flags().IntVar(&snapshotRetentionDays, "retention-days", 7, "retention period in days")
	snapshotCreateCmd.Flags().BoolVar(&snapshotValidate, "validate", false, "validate snapshot integrity after creation")

	// List flags
	snapshotListCmd.Flags().StringVar(&snapshotFormat, "format", "table", "output format (table, json)")

	// Cleanup flags
	snapshotCleanupCmd.Flags().IntVar(&snapshotRetentionDays, "retention-days", 7, "retention period in days")
	snapshotCleanupCmd.Flags().BoolVar(&snapshotDryRun, "dry-run", false, "show what would be deleted without deleting")
	snapshotCleanupCmd.Flags().BoolVar(&snapshotForce, "force", false, "skip confirmation prompt")
}

func createSnapshot() error {
	// Load configuration to get DATABASE_URL
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	// Parse DATABASE_URL
	host, port, database, user, password, err := snapshot.ParseDatabaseURL(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("failed to parse DATABASE_URL: %w", err)
	}

	// Get snapshot directory
	dir := snapshotDir
	if dir == "" {
		dir = snapshot.DefaultConfig().SnapshotDir
	}

	// Get git commit if available
	gitCommit := os.Getenv("GIT_COMMIT")

	// Get deployment ID if available
	deploymentID := os.Getenv("DEPLOYMENT_ID")
	if deploymentID == "" {
		deploymentID = "manual"
	}

	fmt.Printf("Creating database snapshot...\n")
	fmt.Printf("  Database: %s\n", database)
	fmt.Printf("  Reason: %s\n", snapshotReason)
	fmt.Printf("  Retention: %d days\n", snapshotRetentionDays)
	fmt.Printf("  Directory: %s\n", dir)
	fmt.Println()

	// Create snapshot
	ctx := context.Background()
	snap, err := snapshot.Create(ctx, snapshot.CreateOptions{
		DatabaseURL:   cfg.Database.URL,
		Database:      database,
		Host:          host,
		Port:          port,
		User:          user,
		Password:      password,
		Reason:        snapshotReason,
		RetentionDays: snapshotRetentionDays,
		SnapshotDir:   dir,
		GitCommit:     gitCommit,
		DeploymentID:  deploymentID,
		Validate:      snapshotValidate,
	})
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	fmt.Printf("✓ Snapshot created successfully\n")
	fmt.Printf("  Path: %s\n", snap.Path)
	fmt.Printf("  Size: %dMB (compressed)\n", snap.SizeMB)
	fmt.Printf("  Duration: %ds\n", snap.Metadata.DurationSecs)
	if snapshotValidate {
		fmt.Printf("  Validation: passed\n")
	}
	fmt.Println()

	return nil
}

func listSnapshots() error {
	// Get snapshot directory
	dir := snapshotDir
	if dir == "" {
		dir = snapshot.DefaultConfig().SnapshotDir
	}

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Printf("No snapshots found (directory does not exist: %s)\n", dir)
		return nil
	}

	// List snapshots
	snapshots, err := snapshot.List(dir)
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		fmt.Printf("No snapshots found in %s\n", dir)
		return nil
	}

	// Output based on format
	if snapshotFormat == "json" {
		return printSnapshotsJSON(snapshots)
	}

	return printSnapshotsTable(snapshots)
}

func printSnapshotsTable(snapshots []snapshot.Snapshot) error {
	fmt.Printf("Snapshots in %s:\n\n", snapshotDir)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	_, _ = fmt.Fprintf(w, "SNAPSHOT\tSIZE\tAGE\tEXPIRES\tREASON\n")
	_, _ = fmt.Fprintf(w, "--------\t----\t---\t-------\t------\n")

	for _, snap := range snapshots {
		age := formatDuration(snap.Age)

		// Calculate time until expiration
		expiresIn := time.Until(snap.Metadata.ExpiresAt)
		expiresStr := formatExpiration(expiresIn)

		fmt.Fprintf(w, "%s\t%dMB\t%s\t%s\t%s\n",
			snap.Metadata.SnapshotName,
			snap.SizeMB,
			age,
			expiresStr,
			snap.Metadata.Reason,
		)
	}

	return nil
}

func printSnapshotsJSON(snapshots []snapshot.Snapshot) error {
	// Simple JSON output
	fmt.Println("[")
	for i, snap := range snapshots {
		fmt.Printf("  {\n")
		fmt.Printf("    \"name\": %q,\n", snap.Metadata.SnapshotName)
		fmt.Printf("    \"path\": %q,\n", snap.Path)
		fmt.Printf("    \"size_mb\": %d,\n", snap.SizeMB)
		fmt.Printf("    \"age_seconds\": %d,\n", int(snap.Age.Seconds()))
		fmt.Printf("    \"timestamp\": %q,\n", snap.Metadata.Timestamp.Format(time.RFC3339))
		fmt.Printf("    \"expires_at\": %q,\n", snap.Metadata.ExpiresAt.Format(time.RFC3339))
		fmt.Printf("    \"reason\": %q,\n", snap.Metadata.Reason)
		fmt.Printf("    \"retention_days\": %d\n", snap.Metadata.RetentionDays)
		if i < len(snapshots)-1 {
			fmt.Printf("  },\n")
		} else {
			fmt.Printf("  }\n")
		}
	}
	fmt.Println("]")
	return nil
}

func cleanupSnapshots() error {
	// Get snapshot directory
	dir := snapshotDir
	if dir == "" {
		dir = snapshot.DefaultConfig().SnapshotDir
	}

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Printf("No snapshots to clean up (directory does not exist: %s)\n", dir)
		return nil
	}

	// Find expired snapshots
	deleted, err := snapshot.Cleanup(dir, snapshotRetentionDays, true) // Always dry-run first
	if err != nil {
		return fmt.Errorf("failed to check for expired snapshots: %w", err)
	}

	if len(deleted) == 0 {
		fmt.Printf("No expired snapshots to clean up (retention: %d days)\n", snapshotRetentionDays)
		return nil
	}

	// Show what will be deleted
	fmt.Printf("Found %d expired snapshot(s):\n\n", len(deleted))
	for _, snap := range deleted {
		age := formatDuration(snap.Age)
		fmt.Printf("  - %s (%dMB, %s old, reason: %s)\n",
			snap.Metadata.SnapshotName,
			snap.SizeMB,
			age,
			snap.Metadata.Reason,
		)
	}
	fmt.Println()

	// Calculate total space to free
	totalMB := 0
	for _, snap := range deleted {
		totalMB += snap.SizeMB
	}
	fmt.Printf("Total space to free: %dMB\n\n", totalMB)

	// If dry-run, stop here
	if snapshotDryRun {
		fmt.Println("Dry-run mode: no files were deleted")
		return nil
	}

	// Ask for confirmation unless --force
	if !snapshotForce {
		if !confirm("Delete these snapshots?", false) {
			fmt.Println("Cleanup cancelled")
			return nil
		}
	}

	// Actually delete
	deleted, err = snapshot.Cleanup(dir, snapshotRetentionDays, false)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Printf("✓ Deleted %d snapshot(s), freed %dMB\n", len(deleted), totalMB)
	return nil
}

// Helper functions

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days == 0 {
		hours := int(d.Hours())
		if hours == 0 {
			return fmt.Sprintf("%dm", int(d.Minutes()))
		}
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dd", days)
}

func formatExpiration(d time.Duration) string {
	if d < 0 {
		return "EXPIRED"
	}
	days := int(d.Hours() / 24)
	if days == 0 {
		hours := int(d.Hours())
		if hours == 0 {
			return "soon"
		}
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dd", days)
}
