package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Togather-Foundation/server/internal/deployment"
	"github.com/spf13/cobra"
)

var (
	deployForce           bool
	deployDryRun          bool
	deploySkipHealthCheck bool
	deployFormat          string
	deployHealthCheckURL  string
	deployHealthTimeout   int
	deployStateFile       string
)

// deployCmd represents the deploy command group
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Manage deployments and rollbacks",
	Long: `Manage deployment operations including rollback and status queries.

Integrates with blue-green deployment infrastructure to safely
rollback deployments and query deployment status.

Examples:
  # Show current deployment status
  server deploy status

  # Rollback to previous deployment
  server deploy rollback production

  # Dry-run rollback to see what would happen
  server deploy rollback production --dry-run`,
}

// deployStatusCmd shows deployment status
var deployStatusCmd = &cobra.Command{
	Use:   "status [environment]",
	Short: "Show deployment status",
	Long: `Show current deployment status including active slot, version, and history.

If no environment is specified, shows status for the default environment
(determined from deployment-state.json).

Examples:
  # Show status for default environment
  server deploy status

  # Show status in JSON format
  server deploy status --format json

  # Show status in YAML format
  server deploy status --format yaml`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		env := ""
		if len(args) > 0 {
			env = args[0]
		}
		return showDeploymentStatus(env)
	},
}

// deployRollbackCmd rolls back a deployment
var deployRollbackCmd = &cobra.Command{
	Use:   "rollback ENVIRONMENT",
	Short: "Rollback to previous deployment",
	Long: `Rollback to the previous deployment by switching to the inactive slot.

This command:
  1. Validates that a previous deployment exists
  2. Checks health of the target slot (unless --force or --skip-health-check)
  3. Switches the active deployment slot
  4. Updates deployment state

Safety features:
  - Requires explicit confirmation unless --force
  - Shows what will happen before executing
  - Validates target slot health
  - Prevents rollback if target unhealthy (unless --force)

Examples:
  # Rollback production deployment
  server deploy rollback production

  # Dry-run to see what would happen
  server deploy rollback production --dry-run

  # Force rollback without health checks or confirmation
  server deploy rollback production --force

  # Rollback with specific health check URL
  server deploy rollback production --health-url http://localhost:8081/health`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		env := args[0]
		return rollbackDeployment(env)
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)

	// Add subcommands
	deployCmd.AddCommand(deployStatusCmd)
	deployCmd.AddCommand(deployRollbackCmd)

	// Global deploy flags
	deployCmd.PersistentFlags().StringVar(&deployStateFile, "state-file", "", "deployment state file path (default: deploy/config/deployment-state.json)")

	// Status flags
	deployStatusCmd.Flags().StringVar(&deployFormat, "format", "table", "output format (table, json, yaml)")

	// Rollback flags
	deployRollbackCmd.Flags().BoolVar(&deployForce, "force", false, "skip confirmations and health checks")
	deployRollbackCmd.Flags().BoolVar(&deployDryRun, "dry-run", false, "show what would happen without executing")
	deployRollbackCmd.Flags().BoolVar(&deploySkipHealthCheck, "skip-health-check", false, "skip health check validation")
	deployRollbackCmd.Flags().StringVar(&deployHealthCheckURL, "health-url", "", "health check URL (auto-detected if not specified)")
	deployRollbackCmd.Flags().IntVar(&deployHealthTimeout, "health-timeout", 10, "health check timeout in seconds")
}

func getStateFilePath() string {
	if deployStateFile != "" {
		return deployStateFile
	}
	return deployment.DefaultConfig().StateFilePath
}

func showDeploymentStatus(env string) error {
	stateFilePath := getStateFilePath()

	state, err := deployment.GetDeploymentStatus(stateFilePath)
	if err != nil {
		return fmt.Errorf("failed to load deployment status: %w", err)
	}

	// Filter by environment if specified
	if env != "" && state.Environment != env {
		return fmt.Errorf("no deployment found for environment: %s (found: %s)", env, state.Environment)
	}

	// Output based on format
	switch deployFormat {
	case "json":
		return printStatusJSON(state)
	case "yaml":
		return printStatusYAML(state)
	default:
		return printStatusTable(state)
	}
}

func printStatusTable(state *deployment.State) error {
	fmt.Printf("Deployment Status\n")
	fmt.Printf("=================\n\n")

	fmt.Printf("Environment: %s\n", state.Environment)
	fmt.Printf("Locked: %v\n", state.IsLocked())
	if state.IsLocked() {
		fmt.Printf("  Locked by: %s\n", state.Lock.LockedBy)
		fmt.Printf("  Reason: %s\n", state.Lock.Reason)
		fmt.Printf("  Locked at: %s\n", state.Lock.LockedAt.Format(time.RFC3339))
		if !state.Lock.ExpiresAt.IsZero() {
			fmt.Printf("  Expires at: %s\n", state.Lock.ExpiresAt.Format(time.RFC3339))
		}
	}
	fmt.Println()

	// Current deployment
	if state.CurrentDeployment != nil {
		fmt.Printf("Current Deployment:\n")
		printDeploymentInfo(state.CurrentDeployment, "  ")
		fmt.Println()
	} else {
		fmt.Printf("Current Deployment: None\n\n")
	}

	// Previous deployment
	if state.PreviousDeployment != nil {
		fmt.Printf("Previous Deployment:\n")
		printDeploymentInfo(state.PreviousDeployment, "  ")
		fmt.Println()
	} else {
		fmt.Printf("Previous Deployment: None\n\n")
	}

	// Deployment history
	if len(state.DeploymentHistory) > 0 {
		fmt.Printf("Deployment History (%d):\n", len(state.DeploymentHistory))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(w, "  VERSION\tSLOT\tDEPLOYED AT\tGIT COMMIT\n")
		_, _ = fmt.Fprintf(w, "  -------\t----\t-----------\t----------\n")
		for _, dep := range state.DeploymentHistory {
			gitCommit := dep.GitCommit
			if len(gitCommit) > 8 {
				gitCommit = gitCommit[:8]
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
				dep.Version,
				dep.Slot,
				dep.DeployedAt.Format("2006-01-02 15:04"),
				gitCommit,
			)
		}
		_ = w.Flush()
	}

	return nil
}

func printDeploymentInfo(dep *deployment.DeploymentInfo, indent string) {
	fmt.Printf("%sVersion: %s\n", indent, dep.Version)
	fmt.Printf("%sSlot: %s\n", indent, dep.Slot)
	fmt.Printf("%sDeployed at: %s\n", indent, dep.DeployedAt.Format(time.RFC3339))
	if dep.GitCommit != "" {
		fmt.Printf("%sGit commit: %s\n", indent, dep.GitCommit)
	}
	if dep.DockerImage != "" {
		fmt.Printf("%sDocker image: %s\n", indent, dep.DockerImage)
	}
	if dep.DeployedBy != "" {
		fmt.Printf("%sDeployed by: %s\n", indent, dep.DeployedBy)
	}
	if dep.SnapshotPath != "" {
		fmt.Printf("%sSnapshot: %s\n", indent, dep.SnapshotPath)
	}
	if dep.Rollback {
		fmt.Printf("%sRollback: yes (ID: %s)\n", indent, dep.RollbackID)
	}
}

func printStatusJSON(state *deployment.State) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(state)
}

func printStatusYAML(state *deployment.State) error {
	// Simple YAML output (not using external library for simplicity)
	fmt.Printf("environment: %s\n", state.Environment)
	fmt.Printf("locked: %v\n", state.IsLocked())
	if state.IsLocked() {
		fmt.Printf("lock:\n")
		fmt.Printf("  locked_by: %s\n", state.Lock.LockedBy)
		fmt.Printf("  reason: %s\n", state.Lock.Reason)
		fmt.Printf("  locked_at: %s\n", state.Lock.LockedAt.Format(time.RFC3339))
	}

	if state.CurrentDeployment != nil {
		fmt.Printf("current_deployment:\n")
		printDeploymentYAML(state.CurrentDeployment, "  ")
	}

	if state.PreviousDeployment != nil {
		fmt.Printf("previous_deployment:\n")
		printDeploymentYAML(state.PreviousDeployment, "  ")
	}

	return nil
}

func printDeploymentYAML(dep *deployment.DeploymentInfo, indent string) {
	fmt.Printf("%sversion: %s\n", indent, dep.Version)
	fmt.Printf("%sslot: %s\n", indent, dep.Slot)
	fmt.Printf("%sdeployed_at: %s\n", indent, dep.DeployedAt.Format(time.RFC3339))
	if dep.GitCommit != "" {
		fmt.Printf("%sgit_commit: %s\n", indent, dep.GitCommit)
	}
	if dep.DockerImage != "" {
		fmt.Printf("%sdocker_image: %s\n", indent, dep.DockerImage)
	}
}

func rollbackDeployment(env string) error {
	stateFilePath := getStateFilePath()

	// Load current state to show preview
	state, err := deployment.LoadState(stateFilePath)
	if err != nil {
		return fmt.Errorf("failed to load deployment state: %w", err)
	}

	// Validate environment matches
	if env != "" && state.Environment != env {
		return fmt.Errorf("environment mismatch: requested %s, state file has %s", env, state.Environment)
	}

	// Check if rollback is possible
	if err := deployment.ValidateRollback(state); err != nil {
		return err
	}

	// Get rollback target
	rollbackTarget, err := state.GetRollbackTarget()
	if err != nil {
		return err
	}

	// Show what will happen
	fmt.Printf("Rollback Preview\n")
	fmt.Printf("================\n\n")

	fmt.Printf("Current Deployment:\n")
	if state.CurrentDeployment != nil {
		printDeploymentInfo(state.CurrentDeployment, "  ")
	}
	fmt.Println()

	fmt.Printf("Will rollback to:\n")
	printDeploymentInfo(rollbackTarget, "  ")
	fmt.Println()

	targetSlot := state.GetInactiveSlot()
	fmt.Printf("Target slot: %s\n", targetSlot)
	fmt.Println()

	// Determine health check URL if not specified
	healthCheckURL := deployHealthCheckURL
	if healthCheckURL == "" && !deploySkipHealthCheck && !deployForce {
		// Try to auto-detect based on slot
		// This assumes standard Docker Compose setup
		port := 8080
		if targetSlot == deployment.SlotGreen {
			port = 8081
		}
		healthCheckURL = fmt.Sprintf("http://localhost:%d/health", port)
		fmt.Printf("Health check URL: %s (auto-detected)\n", healthCheckURL)
		fmt.Println()
	}

	// Show warnings
	if rollbackTarget.SnapshotPath != "" {
		fmt.Printf("⚠️  Database snapshot available: %s\n", rollbackTarget.SnapshotPath)
		fmt.Printf("⚠️  If database migrations were applied, you may need to restore the database\n")
		fmt.Println()
	}

	// If dry-run, perform validation and exit
	if deployDryRun {
		fmt.Println("Dry-run mode: No changes will be made")
		fmt.Println()

		// Perform dry-run
		opts := deployment.RollbackOptions{
			Environment:     env,
			StateFilePath:   stateFilePath,
			Force:           deployForce,
			SkipHealthCheck: deploySkipHealthCheck,
			DryRun:          true,
			HealthCheckURL:  healthCheckURL,
			HealthTimeout:   time.Duration(deployHealthTimeout) * time.Second,
		}

		result, err := deployment.PerformRollback(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("dry-run validation failed: %w", err)
		}

		fmt.Printf("✓ Dry-run successful\n")
		fmt.Printf("  %s\n", result.Message)
		return nil
	}

	// Ask for confirmation unless --force
	if !deployForce {
		if !confirm("Do you want to continue with the rollback?", false) {
			fmt.Println("Rollback cancelled")
			return nil
		}
		fmt.Println()
	}

	// Perform rollback
	opts := deployment.RollbackOptions{
		Environment:     env,
		StateFilePath:   stateFilePath,
		Force:           deployForce,
		SkipHealthCheck: deploySkipHealthCheck,
		DryRun:          false,
		HealthCheckURL:  healthCheckURL,
		HealthTimeout:   time.Duration(deployHealthTimeout) * time.Second,
	}

	result, err := deployment.PerformRollback(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	// Show result
	if result.Success {
		fmt.Printf("✓ %s\n", result.Message)
		fmt.Printf("  Rollback ID: %s\n", result.RollbackID)
		fmt.Printf("  New active slot: %s\n", result.NewActiveSlot)

		if len(result.Warnings) > 0 {
			fmt.Println()
			fmt.Println("Warnings:")
			for _, warning := range result.Warnings {
				fmt.Printf("  ⚠️  %s\n", warning)
			}
		}

		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Verify the application is working correctly")
		fmt.Println("  2. Monitor logs for errors")
		if rollbackTarget.SnapshotPath != "" {
			fmt.Println("  3. Restore database snapshot if needed:")
			fmt.Printf("     server snapshot restore %s\n", rollbackTarget.SnapshotPath)
		}
	} else {
		return fmt.Errorf("rollback failed: %s", result.Message)
	}

	return nil
}
