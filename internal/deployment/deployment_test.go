package deployment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSlot_Opposite(t *testing.T) {
	tests := []struct {
		slot     Slot
		opposite Slot
	}{
		{SlotBlue, SlotGreen},
		{SlotGreen, SlotBlue},
	}

	for _, tt := range tests {
		t.Run(string(tt.slot), func(t *testing.T) {
			if got := tt.slot.Opposite(); got != tt.opposite {
				t.Errorf("Slot.Opposite() = %v, want %v", got, tt.opposite)
			}
		})
	}
}

func TestLoadState_NewFile(t *testing.T) {
	// Test loading non-existent file returns empty state
	tmpFile := filepath.Join(t.TempDir(), "nonexistent.json")

	state, err := LoadState(tmpFile)
	if err != nil {
		t.Fatalf("LoadState() error = %v, want nil", err)
	}

	if state == nil {
		t.Fatal("LoadState() returned nil state")
	}

	if state.Environment != "" {
		t.Errorf("new state Environment = %q, want empty", state.Environment)
	}

	if state.Lock.Locked {
		t.Error("new state should not be locked")
	}
}

func TestLoadState_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "state.json")

	// Create test state
	testState := &State{
		Environment: "test",
		CurrentDeployment: &DeploymentInfo{
			Version:    "v1.0.0",
			DeployedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			GitCommit:  "abc123",
			Slot:       SlotBlue,
		},
		DeploymentHistory: []DeploymentInfo{},
		Lock: Lock{
			Locked: false,
		},
	}

	// Save state
	if err := SaveState(tmpFile, testState); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Load state
	loaded, err := LoadState(tmpFile)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if loaded.Environment != "test" {
		t.Errorf("Environment = %q, want %q", loaded.Environment, "test")
	}

	if loaded.CurrentDeployment == nil {
		t.Fatal("CurrentDeployment is nil")
	}

	if loaded.CurrentDeployment.Version != "v1.0.0" {
		t.Errorf("Version = %q, want %q", loaded.CurrentDeployment.Version, "v1.0.0")
	}
}

func TestSaveState(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "subdir", "state.json")

	state := &State{
		Environment:       "production",
		DeploymentHistory: []DeploymentInfo{},
		Lock: Lock{
			Locked: false,
		},
	}

	// Save should create directory if needed
	if err := SaveState(tmpFile, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("SaveState() did not create file")
	}

	// Verify content
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	var loaded State
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal saved state: %v", err)
	}

	if loaded.Environment != "production" {
		t.Errorf("saved Environment = %q, want %q", loaded.Environment, "production")
	}
}

func TestState_GetActiveSlot(t *testing.T) {
	tests := []struct {
		name  string
		state *State
		want  Slot
	}{
		{
			name: "blue slot active",
			state: &State{
				CurrentDeployment: &DeploymentInfo{
					Slot: SlotBlue,
				},
			},
			want: SlotBlue,
		},
		{
			name: "green slot active",
			state: &State{
				CurrentDeployment: &DeploymentInfo{
					Slot: SlotGreen,
				},
			},
			want: SlotGreen,
		},
		{
			name:  "no deployment defaults to blue",
			state: &State{},
			want:  SlotBlue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.GetActiveSlot(); got != tt.want {
				t.Errorf("GetActiveSlot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestState_GetInactiveSlot(t *testing.T) {
	state := &State{
		CurrentDeployment: &DeploymentInfo{
			Slot: SlotBlue,
		},
	}

	if got := state.GetInactiveSlot(); got != SlotGreen {
		t.Errorf("GetInactiveSlot() = %v, want %v", got, SlotGreen)
	}
}

func TestState_IsLocked(t *testing.T) {
	tests := []struct {
		name  string
		state *State
		want  bool
	}{
		{
			name: "not locked",
			state: &State{
				Lock: Lock{
					Locked: false,
				},
			},
			want: false,
		},
		{
			name: "locked without expiry",
			state: &State{
				Lock: Lock{
					Locked:   true,
					LockedBy: "user",
				},
			},
			want: true,
		},
		{
			name: "locked but expired",
			state: &State{
				Lock: Lock{
					Locked:    true,
					LockedBy:  "user",
					ExpiresAt: time.Now().Add(-1 * time.Hour),
				},
			},
			want: false,
		},
		{
			name: "locked and not expired",
			state: &State{
				Lock: Lock{
					Locked:    true,
					LockedBy:  "user",
					ExpiresAt: time.Now().Add(1 * time.Hour),
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsLocked(); got != tt.want {
				t.Errorf("IsLocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestState_AcquireLock(t *testing.T) {
	t.Run("acquire unlocked", func(t *testing.T) {
		state := &State{
			Lock: Lock{
				Locked: false,
			},
		}

		err := state.AcquireLock("testuser", "testing", 10*time.Minute)
		if err != nil {
			t.Fatalf("AcquireLock() error = %v, want nil", err)
		}

		if !state.Lock.Locked {
			t.Error("lock should be acquired")
		}

		if state.Lock.LockedBy != "testuser" {
			t.Errorf("LockedBy = %q, want %q", state.Lock.LockedBy, "testuser")
		}
	})

	t.Run("acquire already locked", func(t *testing.T) {
		state := &State{
			Lock: Lock{
				Locked:   true,
				LockedBy: "otheruser",
				Reason:   "other reason",
			},
		}

		err := state.AcquireLock("testuser", "testing", 10*time.Minute)
		if err == nil {
			t.Error("AcquireLock() should fail when already locked")
		}
	})
}

func TestState_ReleaseLock(t *testing.T) {
	state := &State{
		Lock: Lock{
			Locked:   true,
			LockedBy: "testuser",
		},
	}

	state.ReleaseLock()

	if state.Lock.Locked {
		t.Error("lock should be released")
	}
}

func TestState_RecordDeployment(t *testing.T) {
	state := &State{
		DeploymentHistory: []DeploymentInfo{},
	}

	// First deployment
	deployment1 := DeploymentInfo{
		Version:    "v1.0.0",
		DeployedAt: time.Now(),
		Slot:       SlotBlue,
	}

	state.RecordDeployment(deployment1)

	if state.CurrentDeployment == nil {
		t.Fatal("CurrentDeployment should be set")
	}

	if state.CurrentDeployment.Version != "v1.0.0" {
		t.Errorf("CurrentDeployment.Version = %q, want %q", state.CurrentDeployment.Version, "v1.0.0")
	}

	if state.PreviousDeployment != nil {
		t.Error("PreviousDeployment should be nil for first deployment")
	}

	// Second deployment
	deployment2 := DeploymentInfo{
		Version:    "v1.1.0",
		DeployedAt: time.Now(),
		Slot:       SlotGreen,
	}

	state.RecordDeployment(deployment2)

	if state.CurrentDeployment.Version != "v1.1.0" {
		t.Errorf("CurrentDeployment.Version = %q, want %q", state.CurrentDeployment.Version, "v1.1.0")
	}

	if state.PreviousDeployment == nil {
		t.Fatal("PreviousDeployment should be set")
	}

	if state.PreviousDeployment.Version != "v1.0.0" {
		t.Errorf("PreviousDeployment.Version = %q, want %q", state.PreviousDeployment.Version, "v1.0.0")
	}

	if len(state.DeploymentHistory) != 1 {
		t.Errorf("DeploymentHistory length = %d, want 1", len(state.DeploymentHistory))
	}
}

func TestState_CanRollback(t *testing.T) {
	tests := []struct {
		name  string
		state *State
		want  bool
	}{
		{
			name: "can rollback with previous deployment",
			state: &State{
				CurrentDeployment: &DeploymentInfo{
					Version: "v1.1.0",
				},
				PreviousDeployment: &DeploymentInfo{
					Version: "v1.0.0",
				},
			},
			want: true,
		},
		{
			name: "cannot rollback without previous deployment",
			state: &State{
				CurrentDeployment: &DeploymentInfo{
					Version: "v1.0.0",
				},
				PreviousDeployment: nil,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.CanRollback(); got != tt.want {
				t.Errorf("CanRollback() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestState_SwitchToSlot(t *testing.T) {
	state := &State{
		CurrentDeployment: &DeploymentInfo{
			Version:    "v1.1.0",
			DeployedAt: time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
			Slot:       SlotBlue,
		},
		PreviousDeployment: &DeploymentInfo{
			Version:    "v1.0.0",
			DeployedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Slot:       SlotGreen,
		},
	}

	rollbackID := "rollback_test_123"
	err := state.SwitchToSlot(SlotGreen, rollbackID)
	if err != nil {
		t.Fatalf("SwitchToSlot() error = %v", err)
	}

	// Current should be the rolled-back version
	if state.CurrentDeployment.Version != "v1.0.0" {
		t.Errorf("CurrentDeployment.Version = %q, want %q", state.CurrentDeployment.Version, "v1.0.0")
	}

	if state.CurrentDeployment.Slot != SlotGreen {
		t.Errorf("CurrentDeployment.Slot = %v, want %v", state.CurrentDeployment.Slot, SlotGreen)
	}

	if !state.CurrentDeployment.Rollback {
		t.Error("CurrentDeployment.Rollback should be true")
	}

	if state.CurrentDeployment.RollbackID != rollbackID {
		t.Errorf("CurrentDeployment.RollbackID = %q, want %q", state.CurrentDeployment.RollbackID, rollbackID)
	}

	// Previous should be the old current
	if state.PreviousDeployment.Version != "v1.1.0" {
		t.Errorf("PreviousDeployment.Version = %q, want %q", state.PreviousDeployment.Version, "v1.1.0")
	}
}

func TestCheckHealth(t *testing.T) {
	t.Run("healthy service", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "healthy",
				"checks": map[string]interface{}{
					"database": "pass",
				},
			})
		}))
		defer server.Close()

		result, err := CheckHealth(context.Background(), server.URL, 5*time.Second)
		if err != nil {
			t.Fatalf("CheckHealth() error = %v", err)
		}

		if !result.Healthy {
			t.Error("expected healthy result")
		}

		if result.Status != "healthy" {
			t.Errorf("Status = %q, want %q", result.Status, "healthy")
		}
	})

	t.Run("unhealthy service", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		result, err := CheckHealth(context.Background(), server.URL, 5*time.Second)
		if err != nil {
			t.Fatalf("CheckHealth() error = %v", err)
		}

		if result.Healthy {
			t.Error("expected unhealthy result")
		}
	})

	t.Run("unreachable service", func(t *testing.T) {
		result, err := CheckHealth(context.Background(), "http://localhost:9999/health", 100*time.Millisecond)
		if err != nil {
			t.Fatalf("CheckHealth() error = %v", err)
		}

		if result.Healthy {
			t.Error("expected unhealthy result for unreachable service")
		}
	})
}

func TestValidateRollback(t *testing.T) {
	tests := []struct {
		name    string
		state   *State
		wantErr bool
	}{
		{
			name: "valid rollback",
			state: &State{
				CurrentDeployment: &DeploymentInfo{
					Version: "v1.1.0",
				},
				PreviousDeployment: &DeploymentInfo{
					Version: "v1.0.0",
				},
				Lock: Lock{
					Locked: false,
				},
			},
			wantErr: false,
		},
		{
			name: "locked deployment",
			state: &State{
				CurrentDeployment: &DeploymentInfo{
					Version: "v1.1.0",
				},
				PreviousDeployment: &DeploymentInfo{
					Version: "v1.0.0",
				},
				Lock: Lock{
					Locked:   true,
					LockedBy: "user",
					Reason:   "maintenance",
				},
			},
			wantErr: true,
		},
		{
			name: "no previous deployment",
			state: &State{
				CurrentDeployment: &DeploymentInfo{
					Version: "v1.0.0",
				},
				PreviousDeployment: nil,
				Lock: Lock{
					Locked: false,
				},
			},
			wantErr: true,
		},
		{
			name: "no current deployment",
			state: &State{
				CurrentDeployment: nil,
				PreviousDeployment: &DeploymentInfo{
					Version: "v1.0.0",
				},
				Lock: Lock{
					Locked: false,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRollback(tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRollback() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPerformRollback_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")

	// Create test state
	state := &State{
		Environment: "test",
		CurrentDeployment: &DeploymentInfo{
			Version:    "v1.1.0",
			DeployedAt: time.Now(),
			Slot:       SlotBlue,
		},
		PreviousDeployment: &DeploymentInfo{
			Version:    "v1.0.0",
			DeployedAt: time.Now().Add(-1 * time.Hour),
			Slot:       SlotGreen,
		},
		DeploymentHistory: []DeploymentInfo{},
		Lock: Lock{
			Locked: false,
		},
	}

	if err := SaveState(stateFile, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	opts := RollbackOptions{
		Environment:     "test",
		StateFilePath:   stateFile,
		Force:           true,
		SkipHealthCheck: true,
		DryRun:          true,
	}

	result, err := PerformRollback(context.Background(), opts)
	if err != nil {
		t.Fatalf("PerformRollback() error = %v", err)
	}

	if !result.Success {
		t.Error("dry-run should succeed")
	}

	if result.PreviousDeployment.Version != "v1.0.0" {
		t.Errorf("PreviousDeployment.Version = %q, want %q", result.PreviousDeployment.Version, "v1.0.0")
	}

	// Verify state wasn't actually modified
	loadedState, err := LoadState(stateFile)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if loadedState.CurrentDeployment.Version != "v1.1.0" {
		t.Error("state should not be modified in dry-run")
	}
}
