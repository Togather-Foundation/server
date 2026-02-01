package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestDeployCommandHelp(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"deploy", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("deploy command --help failed: %v", err)
	}

	output := buf.String()

	// Verify help text contains expected content
	expectedStrings := []string{
		"Manage deployments and rollbacks",
		"status",
		"rollback",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestDeployStatusCommandHelp(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"deploy", "status", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("deploy status --help failed: %v", err)
	}

	output := buf.String()

	expectedStrings := []string{
		"Show deployment status",
		"--format",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestDeployRollbackCommandHelp(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"deploy", "rollback", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("deploy rollback --help failed: %v", err)
	}

	output := buf.String()

	expectedStrings := []string{
		"Rollback to previous deployment",
		"--force",
		"--dry-run",
		"--skip-health-check",
		"--health-url",
		"--health-timeout",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestDeployCommandFlags(t *testing.T) {
	// Test status flags
	statusFlags := []string{"format"}
	for _, flag := range statusFlags {
		if f := deployStatusCmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q to be defined on deploy status command", flag)
		}
	}

	// Test rollback flags
	rollbackFlags := []string{"force", "dry-run", "skip-health-check", "health-url", "health-timeout"}
	for _, flag := range rollbackFlags {
		if f := deployRollbackCmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q to be defined on deploy rollback command", flag)
		}
	}

	// Test persistent flags
	persistentFlags := []string{"state-file"}
	for _, flag := range persistentFlags {
		if f := deployCmd.PersistentFlags().Lookup(flag); f == nil {
			t.Errorf("expected persistent flag %q to be defined on deploy command", flag)
		}
	}
}

func TestDeployStatusCommandArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no args",
			args:        []string{"deploy", "status"},
			expectError: false,
		},
		{
			name:        "with environment",
			args:        []string{"deploy", "status", "production"},
			expectError: false,
		},
		{
			name:        "too many args",
			args:        []string{"deploy", "status", "production", "extra"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newRootCommand()
			buf := new(bytes.Buffer)
			root.SetOut(buf)
			root.SetErr(buf)
			root.SetArgs(tt.args)

			err := root.Execute()

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil && !strings.Contains(err.Error(), "deployment status") {
				// Ignore errors from missing state file - we're testing arg parsing
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDeployRollbackCommandArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "missing environment",
			args:        []string{"deploy", "rollback"},
			expectError: true,
		},
		{
			name:        "with environment",
			args:        []string{"deploy", "rollback", "production"},
			expectError: false,
		},
		{
			name:        "too many args",
			args:        []string{"deploy", "rollback", "production", "extra"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newRootCommand()
			buf := new(bytes.Buffer)
			root.SetOut(buf)
			root.SetErr(buf)
			root.SetArgs(tt.args)

			err := root.Execute()

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil && !strings.Contains(err.Error(), "deployment state") {
				// Ignore errors from missing state file - we're testing arg parsing
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetStateFilePath(t *testing.T) {
	// Save original value
	origDeployStateFile := deployStateFile
	defer func() {
		deployStateFile = origDeployStateFile
	}()

	tests := []struct {
		name         string
		stateFile    string
		expectCustom bool
	}{
		{
			name:         "default path",
			stateFile:    "",
			expectCustom: false,
		},
		{
			name:         "custom path",
			stateFile:    "/custom/path/state.json",
			expectCustom: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployStateFile = tt.stateFile
			result := getStateFilePath()

			if tt.expectCustom {
				if result != tt.stateFile {
					t.Errorf("expected custom path %q, got %q", tt.stateFile, result)
				}
			} else {
				if result == "" {
					t.Error("expected default path, got empty string")
				}
			}
		})
	}
}

func TestDeployCommandIntegration(t *testing.T) {
	// This test verifies that deploy commands are registered correctly
	root := newRootCommand()

	// Find deploy command
	var deployFound bool
	for _, cmd := range root.Commands() {
		if cmd.Name() == "deploy" {
			deployFound = true

			// Verify subcommands
			subcommands := map[string]bool{
				"status":   false,
				"rollback": false,
			}

			for _, subCmd := range cmd.Commands() {
				if _, ok := subcommands[subCmd.Name()]; ok {
					subcommands[subCmd.Name()] = true
				}
			}

			for name, found := range subcommands {
				if !found {
					t.Errorf("expected deploy subcommand %q to be registered", name)
				}
			}

			break
		}
	}

	if !deployFound {
		t.Error("expected deploy command to be registered")
	}
}

func TestDeployStatusMissingStateFile(t *testing.T) {
	// Test that status command handles missing state file gracefully
	t.Skip("Requires deployment state file, tested in integration tests")
}

func TestDeployRollbackDryRun(t *testing.T) {
	// Test that dry-run doesn't perform actual rollback
	t.Skip("Requires deployment state file, tested in integration tests")
}

func TestDeployRollbackValidation(t *testing.T) {
	// Test that rollback validates prerequisites
	t.Skip("Requires deployment state file, tested in integration tests")
}
