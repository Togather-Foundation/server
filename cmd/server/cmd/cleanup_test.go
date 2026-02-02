package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestCleanupCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "help flag",
			args: []string{"cleanup", "--help"},
		},
		{
			name: "dry-run flag accepted",
			args: []string{"cleanup", "--dry-run", "--help"},
		},
		{
			name: "force flag accepted",
			args: []string{"cleanup", "--force", "--help"},
		},
		{
			name: "keep-images flag accepted",
			args: []string{"cleanup", "--keep-images=5", "--help"},
		},
		{
			name: "keep-snapshots flag accepted",
			args: []string{"cleanup", "--keep-snapshots=14", "--help"},
		},
		{
			name: "keep-logs-days flag accepted",
			args: []string{"cleanup", "--keep-logs-days=60", "--help"},
		},
		{
			name: "images-only flag accepted",
			args: []string{"cleanup", "--images-only", "--help"},
		},
		{
			name: "snapshots-only flag accepted",
			args: []string{"cleanup", "--snapshots-only", "--help"},
		},
		{
			name: "logs-only flag accepted",
			args: []string{"cleanup", "--logs-only", "--help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCommand()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			// Should not error when showing help
			err := cmd.Execute()
			if err != nil {
				t.Fatalf("Expected no error for %v, got: %v", tt.args, err)
			}

			output := buf.String()
			if !strings.Contains(output, "cleanup") && !strings.Contains(output, "Clean") {
				t.Errorf("Expected help output to mention cleanup, got: %s", output)
			}
		})
	}
}

func TestCleanupCommand_FlagParsing(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid integer for keep-images",
			args:        []string{"cleanup", "--keep-images=5", "--help"},
			expectError: false,
		},
		{
			name:          "invalid integer for keep-images",
			args:          []string{"cleanup", "--keep-images=abc", "--help"},
			expectError:   true,
			errorContains: "invalid argument",
		},
		{
			name:          "negative keep-images",
			args:          []string{"cleanup", "--keep-images=-1", "--help"},
			expectError:   false, // cobra accepts it, script validates
			errorContains: "",
		},
		{
			name:        "all flags together",
			args:        []string{"cleanup", "--dry-run", "--force", "--keep-images=2", "--keep-snapshots=14", "--help"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCommand()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.expectError {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestFindCleanupScript(t *testing.T) {
	// This test verifies the function exists and can be called
	// In CI/real environment, it may or may not find the script
	_, err := findCleanupScript()
	// We don't assert on the result since it depends on the environment
	// Just verify it doesn't panic
	if err != nil {
		t.Logf("Cleanup script not found (expected in test environment): %v", err)
	} else {
		t.Log("Cleanup script found successfully")
	}
}
