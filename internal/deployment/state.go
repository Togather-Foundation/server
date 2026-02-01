package deployment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Slot represents a deployment slot (blue or green)
type Slot string

const (
	SlotBlue  Slot = "blue"
	SlotGreen Slot = "green"
)

// String returns the string representation of a Slot
func (s Slot) String() string {
	return string(s)
}

// Opposite returns the opposite slot
func (s Slot) Opposite() Slot {
	if s == SlotBlue {
		return SlotGreen
	}
	return SlotBlue
}

// DeploymentInfo represents information about a single deployment
type DeploymentInfo struct {
	Version      string    `json:"version"`
	DeployedAt   time.Time `json:"deployed_at"`
	GitCommit    string    `json:"git_commit"`
	DockerImage  string    `json:"docker_image"`
	Slot         Slot      `json:"slot"`
	DeployedBy   string    `json:"deployed_by,omitempty"`
	SnapshotPath string    `json:"snapshot_path,omitempty"`
	Rollback     bool      `json:"rollback,omitempty"`
	RollbackID   string    `json:"rollback_id,omitempty"`
}

// Lock represents a deployment lock state
type Lock struct {
	Locked    bool      `json:"locked"`
	LockedAt  time.Time `json:"locked_at,omitempty"`
	LockedBy  string    `json:"locked_by,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// State represents the full deployment state
type State struct {
	Environment        string           `json:"environment"`
	CurrentDeployment  *DeploymentInfo  `json:"current_deployment"`
	PreviousDeployment *DeploymentInfo  `json:"previous_deployment"`
	DeploymentHistory  []DeploymentInfo `json:"deployment_history"`
	Lock               Lock             `json:"lock"`
}

// Config contains paths for deployment state management
type Config struct {
	StateFilePath string
}

// DefaultConfig returns the default deployment configuration
func DefaultConfig() Config {
	// Try to find state file in standard locations
	stateFile := findStateFile()
	if stateFile == "" {
		// Default location
		stateFile = filepath.Join("deploy", "config", "deployment-state.json")
	}

	return Config{
		StateFilePath: stateFile,
	}
}

func findStateFile() string {
	// Try to locate state file relative to project root
	candidates := []string{
		"deploy/config/deployment-state.json",
		"../deploy/config/deployment-state.json",
		"../../deploy/config/deployment-state.json",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			absPath, err := filepath.Abs(candidate)
			if err == nil {
				return absPath
			}
		}
	}

	return ""
}

// LoadState reads the deployment state from the JSON file
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty state for new deployments
			return &State{
				DeploymentHistory: []DeploymentInfo{},
				Lock: Lock{
					Locked: false,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// SaveState writes the deployment state to the JSON file
func SaveState(path string, state *State) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Marshal with indentation for readability
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temporary file first, then rename (atomic operation)
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		os.Remove(tmpFile) // Clean up temp file
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// GetActiveSlot returns the currently active deployment slot
func (s *State) GetActiveSlot() Slot {
	if s.CurrentDeployment != nil {
		return s.CurrentDeployment.Slot
	}
	// Default to blue if no deployment
	return SlotBlue
}

// GetInactiveSlot returns the currently inactive deployment slot
func (s *State) GetInactiveSlot() Slot {
	return s.GetActiveSlot().Opposite()
}

// IsLocked checks if deployments are currently locked
func (s *State) IsLocked() bool {
	if !s.Lock.Locked {
		return false
	}

	// Check if lock has expired
	if !s.Lock.ExpiresAt.IsZero() && time.Now().After(s.Lock.ExpiresAt) {
		return false
	}

	return true
}

// AcquireLock attempts to acquire the deployment lock
func (s *State) AcquireLock(by string, reason string, duration time.Duration) error {
	if s.IsLocked() {
		return fmt.Errorf("deployment is already locked by %s (reason: %s)", s.Lock.LockedBy, s.Lock.Reason)
	}

	s.Lock = Lock{
		Locked:    true,
		LockedAt:  time.Now(),
		LockedBy:  by,
		Reason:    reason,
		ExpiresAt: time.Now().Add(duration),
	}

	return nil
}

// ReleaseLock releases the deployment lock
func (s *State) ReleaseLock() {
	s.Lock = Lock{
		Locked: false,
	}
}

// RecordDeployment records a new deployment and updates history
func (s *State) RecordDeployment(deployment DeploymentInfo) {
	// Move current to previous
	if s.CurrentDeployment != nil {
		s.PreviousDeployment = s.CurrentDeployment

		// Add previous to history (keep last 10)
		s.DeploymentHistory = append([]DeploymentInfo{*s.PreviousDeployment}, s.DeploymentHistory...)
		if len(s.DeploymentHistory) > 10 {
			s.DeploymentHistory = s.DeploymentHistory[:10]
		}
	}

	// Set new current
	s.CurrentDeployment = &deployment
}

// CanRollback checks if rollback is possible
func (s *State) CanRollback() bool {
	return s.PreviousDeployment != nil
}

// GetRollbackTarget returns the deployment info to rollback to
func (s *State) GetRollbackTarget() (*DeploymentInfo, error) {
	if !s.CanRollback() {
		return nil, fmt.Errorf("no previous deployment available for rollback")
	}
	return s.PreviousDeployment, nil
}

// SwitchToSlot switches the active deployment to the specified slot
// This is used during rollback to activate the previously deployed version
func (s *State) SwitchToSlot(slot Slot, rollbackID string) error {
	if s.CurrentDeployment == nil {
		return fmt.Errorf("no current deployment to switch from")
	}

	if s.PreviousDeployment == nil {
		return fmt.Errorf("no previous deployment to switch to")
	}

	// Swap current and previous
	current := s.CurrentDeployment
	previous := s.PreviousDeployment

	// Mark previous as the new current (with rollback metadata)
	rollbackDeployment := *previous
	rollbackDeployment.Slot = slot
	rollbackDeployment.DeployedAt = time.Now()
	rollbackDeployment.Rollback = true
	rollbackDeployment.RollbackID = rollbackID

	s.CurrentDeployment = &rollbackDeployment
	s.PreviousDeployment = current

	return nil
}
