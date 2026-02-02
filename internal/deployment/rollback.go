package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RollbackOptions contains options for performing a rollback
type RollbackOptions struct {
	Environment     string
	StateFilePath   string
	Force           bool
	SkipHealthCheck bool
	DryRun          bool
	HealthCheckURL  string // URL to check health (e.g., http://localhost:8080/health)
	HealthTimeout   time.Duration
}

// RollbackResult contains the result of a rollback operation
type RollbackResult struct {
	Success            bool
	PreviousDeployment DeploymentInfo
	NewActiveSlot      Slot
	RollbackID         string
	Message            string
	Warnings           []string
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Healthy bool
	Status  string
	Message string
	Checks  map[string]interface{}
}

// CheckHealth performs a health check on the specified URL
func CheckHealth(ctx context.Context, url string, timeout time.Duration) (*HealthCheckResult, error) {
	client := &http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return &HealthCheckResult{
			Healthy: false,
			Message: fmt.Sprintf("health check failed: %v", err),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &HealthCheckResult{
			Healthy: false,
			Status:  resp.Status,
			Message: fmt.Sprintf("unhealthy status code: %d", resp.StatusCode),
		}, nil
	}

	// Try to parse response body
	var healthResp struct {
		Status string                 `json:"status"`
		Checks map[string]interface{} `json:"checks,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		// If we can't parse, but got 200, consider it healthy
		return &HealthCheckResult{
			Healthy: true,
			Status:  "ok",
			Message: "health check returned 200",
		}, nil
	}

	healthy := healthResp.Status == "healthy" || healthResp.Status == "ok"

	return &HealthCheckResult{
		Healthy: healthy,
		Status:  healthResp.Status,
		Message: fmt.Sprintf("health check returned status: %s", healthResp.Status),
		Checks:  healthResp.Checks,
	}, nil
}

// ValidateRollback validates that a rollback can be performed
func ValidateRollback(state *State) error {
	// Check if deployment is locked
	if state.IsLocked() {
		return fmt.Errorf("deployment is locked: %s (by %s)", state.Lock.Reason, state.Lock.LockedBy)
	}

	// Check if there's a previous deployment to rollback to
	if !state.CanRollback() {
		return fmt.Errorf("no previous deployment available for rollback")
	}

	// Check if current deployment exists
	if state.CurrentDeployment == nil {
		return fmt.Errorf("no current deployment found")
	}

	return nil
}

// PerformRollback performs a deployment rollback
func PerformRollback(ctx context.Context, opts RollbackOptions) (*RollbackResult, error) {
	result := &RollbackResult{
		Success:  false,
		Warnings: []string{},
	}

	// Load current state
	state, err := LoadState(opts.StateFilePath)
	if err != nil {
		return result, fmt.Errorf("failed to load deployment state: %w", err)
	}

	// Validate rollback is possible
	if err := ValidateRollback(state); err != nil {
		return result, err
	}

	// Get rollback target
	rollbackTarget, err := state.GetRollbackTarget()
	if err != nil {
		return result, err
	}

	result.PreviousDeployment = *rollbackTarget

	// Determine target slot (switch to opposite of current)
	targetSlot := state.GetInactiveSlot()
	result.NewActiveSlot = targetSlot

	// Generate rollback ID
	rollbackID := fmt.Sprintf("rollback_%s", time.Now().Format("20060102_150405"))
	result.RollbackID = rollbackID

	// Check health of target slot (unless --force or --skip-health-check)
	if !opts.Force && !opts.SkipHealthCheck && opts.HealthCheckURL != "" {
		timeout := opts.HealthTimeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}

		healthResult, err := CheckHealth(ctx, opts.HealthCheckURL, timeout)
		if err != nil {
			if opts.Force {
				result.Warnings = append(result.Warnings, fmt.Sprintf("health check error (ignored due to --force): %v", err))
			} else {
				return result, fmt.Errorf("health check failed: %w", err)
			}
		} else if !healthResult.Healthy {
			if opts.Force {
				result.Warnings = append(result.Warnings, fmt.Sprintf("target slot unhealthy (ignored due to --force): %s", healthResult.Message))
			} else {
				return result, fmt.Errorf("target slot is unhealthy: %s", healthResult.Message)
			}
		}
	}

	// If dry-run, stop here
	if opts.DryRun {
		result.Success = true
		result.Message = fmt.Sprintf("Dry-run: Would rollback to version %s on slot %s", rollbackTarget.Version, targetSlot)
		return result, nil
	}

	// Perform the rollback (switch slots)
	if err := state.SwitchToSlot(targetSlot, rollbackID); err != nil {
		return result, fmt.Errorf("failed to switch slots: %w", err)
	}

	// Save updated state
	if err := SaveState(opts.StateFilePath, state); err != nil {
		return result, fmt.Errorf("failed to save deployment state: %w", err)
	}

	result.Success = true
	result.Message = fmt.Sprintf("Successfully rolled back to version %s on slot %s", rollbackTarget.Version, targetSlot)

	// Add warnings if health check was skipped
	if opts.SkipHealthCheck {
		result.Warnings = append(result.Warnings, "Health check was skipped - manual verification recommended")
	}

	// Add warning about database snapshot if available
	if rollbackTarget.SnapshotPath != "" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Database snapshot available: %s", rollbackTarget.SnapshotPath))
		result.Warnings = append(result.Warnings, "If database migrations were applied after this deployment, you may need to restore the database")
	}

	return result, nil
}

// GetDeploymentStatus returns a formatted status of the current deployment
func GetDeploymentStatus(stateFilePath string) (*State, error) {
	return LoadState(stateFilePath)
}
