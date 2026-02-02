package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up deployment artifacts (Docker images, snapshots, logs)",
	Long: `Clean up old deployment artifacts to free disk space.

This command provides a wrapper around the cleanup script functionality,
removing old Docker images, database snapshots, and deployment logs.

Examples:
  # Interactive cleanup (prompts for confirmation)
  server cleanup

  # Dry run to see what would be deleted
  server cleanup --dry-run

  # Force cleanup without prompts
  server cleanup --force

  # Keep only 2 Docker images per tag pattern
  server cleanup --keep-images 2

  # Clean only snapshots (keep last 14 days)
  server cleanup --snapshots-only --keep-snapshots 14

  # Clean only logs (keep last 60 days)
  server cleanup --logs-only --keep-logs-days 60`,
	RunE: runCleanup,
}

var (
	cleanupDryRun        bool
	cleanupForce         bool
	cleanupKeepImages    int
	cleanupKeepSnapshots int
	cleanupKeepLogsDays  int
	cleanupImagesOnly    bool
	cleanupSnapshotsOnly bool
	cleanupLogsOnly      bool
)

func init() {
	rootCmd.AddCommand(cleanupCmd)

	cleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be cleaned without deleting")
	cleanupCmd.Flags().BoolVar(&cleanupForce, "force", false, "Skip confirmation prompts")
	cleanupCmd.Flags().IntVar(&cleanupKeepImages, "keep-images", 3, "Number of Docker images to keep per tag")
	cleanupCmd.Flags().IntVar(&cleanupKeepSnapshots, "keep-snapshots", 7, "Days of database snapshots to keep")
	cleanupCmd.Flags().IntVar(&cleanupKeepLogsDays, "keep-logs-days", 30, "Days of deployment logs to keep")
	cleanupCmd.Flags().BoolVar(&cleanupImagesOnly, "images-only", false, "Clean only Docker images")
	cleanupCmd.Flags().BoolVar(&cleanupSnapshotsOnly, "snapshots-only", false, "Clean only database snapshots")
	cleanupCmd.Flags().BoolVar(&cleanupLogsOnly, "logs-only", false, "Clean only deployment logs")
}

func runCleanup(cmd *cobra.Command, args []string) error {
	// Find the cleanup script
	scriptPath, err := findCleanupScript()
	if err != nil {
		return fmt.Errorf("cleanup script not found: %w\nPlease ensure deploy/scripts/cleanup.sh exists", err)
	}

	// Build cleanup script arguments
	scriptArgs := []string{}

	if cleanupDryRun {
		scriptArgs = append(scriptArgs, "--dry-run")
	}
	if cleanupForce {
		scriptArgs = append(scriptArgs, "--force")
	}
	if cleanupKeepImages != 3 {
		scriptArgs = append(scriptArgs, fmt.Sprintf("--keep-images=%d", cleanupKeepImages))
	}
	if cleanupKeepSnapshots != 7 {
		scriptArgs = append(scriptArgs, fmt.Sprintf("--keep-snapshots=%d", cleanupKeepSnapshots))
	}
	if cleanupKeepLogsDays != 30 {
		scriptArgs = append(scriptArgs, fmt.Sprintf("--keep-logs-days=%d", cleanupKeepLogsDays))
	}
	if cleanupImagesOnly {
		scriptArgs = append(scriptArgs, "--images-only")
	}
	if cleanupSnapshotsOnly {
		scriptArgs = append(scriptArgs, "--snapshots-only")
	}
	if cleanupLogsOnly {
		scriptArgs = append(scriptArgs, "--logs-only")
	}

	// Execute the cleanup script
	cleanupCmd := exec.Command(scriptPath, scriptArgs...)
	cleanupCmd.Stdout = os.Stdout
	cleanupCmd.Stderr = os.Stderr
	cleanupCmd.Stdin = os.Stdin

	if err := cleanupCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("cleanup failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to run cleanup: %w", err)
	}

	return nil
}

// findCleanupScript locates the cleanup.sh script relative to the binary or working directory
func findCleanupScript() (string, error) {
	// Try relative to working directory first
	candidates := []string{
		"deploy/scripts/cleanup.sh",
		"../deploy/scripts/cleanup.sh",
		"../../deploy/scripts/cleanup.sh",
	}

	for _, candidate := range candidates {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			return absPath, nil
		}
	}

	return "", fmt.Errorf("cleanup.sh not found in expected locations")
}
