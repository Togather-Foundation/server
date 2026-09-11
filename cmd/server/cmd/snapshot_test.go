package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/snapshot"
)

func TestSnapshotCommandHelp(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"snapshot", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("snapshot command --help failed: %v", err)
	}

	output := buf.String()

	// Verify help text contains expected content
	expectedStrings := []string{
		"Manage database snapshots",
		"create",
		"list",
		"cleanup",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestSnapshotCreateCommandHelp(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"snapshot", "create", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("snapshot create --help failed: %v", err)
	}

	output := buf.String()

	expectedStrings := []string{
		"Create a new database snapshot",
		"--reason",
		"--retention-days",
		"--validate",
		"--format",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestSnapshotListCommandHelp(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"snapshot", "list", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("snapshot list --help failed: %v", err)
	}

	output := buf.String()

	expectedStrings := []string{
		"List all database snapshots",
		"--format",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestSnapshotCleanupCommandHelp(t *testing.T) {
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"snapshot", "cleanup", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("snapshot cleanup --help failed: %v", err)
	}

	output := buf.String()

	expectedStrings := []string{
		"Clean up expired snapshots",
		"--retention-days",
		"--dry-run",
		"--force",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestSnapshotCommandFlags(t *testing.T) {
	// Test create flags
	createFlags := []string{"reason", "retention-days", "validate", "format"}
	for _, flag := range createFlags {
		if f := snapshotCreateCmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q to be defined on snapshot create command", flag)
		}
	}

	// Test list flags
	listFlags := []string{"format"}
	for _, flag := range listFlags {
		if f := snapshotListCmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q to be defined on snapshot list command", flag)
		}
	}

	// Test cleanup flags
	cleanupFlags := []string{"retention-days", "dry-run", "force"}
	for _, flag := range cleanupFlags {
		if f := snapshotCleanupCmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q to be defined on snapshot cleanup command", flag)
		}
	}

	// Test persistent flags
	persistentFlags := []string{"snapshot-dir"}
	for _, flag := range persistentFlags {
		if f := snapshotCmd.PersistentFlags().Lookup(flag); f == nil {
			t.Errorf("expected persistent flag %q to be defined on snapshot command", flag)
		}
	}
}

func TestWriteSnapshotCreatedJSON(t *testing.T) {
	now := time.Now().UTC()
	snap := &snapshot.Snapshot{
		Path:      "/var/lib/togather/db-snapshots/togather_testdb_20240101_120000_pre-deploy.sql.gz",
		SizeBytes: 42 * 1024 * 1024,
		SizeMB:    42,
		Metadata: snapshot.Metadata{
			SnapshotName:  "togather_testdb_20240101_120000_pre-deploy.sql.gz",
			Database:      "testdb",
			Timestamp:     now,
			Reason:        "pre-deploy",
			GitCommit:     "abc123def456",
			DeploymentID:  "deploy-001",
			RetentionDays: 7,
			ExpiresAt:     now.Add(7 * 24 * time.Hour),
		},
	}

	var buf bytes.Buffer
	if err := writeSnapshotCreatedJSON(&buf, snap); err != nil {
		t.Fatalf("writeSnapshotCreatedJSON() error: %v", err)
	}

	// The only output must be a single JSON object (no trailing progress text).
	dec := json.NewDecoder(&buf)
	var result map[string]any
	if err := dec.Decode(&result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	// snapshot_path is the field deploy.sh reads; it must be non-empty.
	path, ok := result["snapshot_path"].(string)
	if !ok || path == "" {
		t.Fatalf("snapshot_path missing or empty in JSON: %v", result)
	}
	if path != snap.Path {
		t.Errorf("snapshot_path = %q, want %q", path, snap.Path)
	}

	if _, ok := result["name"].(string); !ok {
		t.Errorf("name missing from JSON: %v", result)
	}
	if _, ok := result["size_bytes"].(float64); !ok {
		t.Errorf("size_bytes missing from JSON: %v", result)
	}
	if _, ok := result["created_at"].(string); !ok {
		t.Errorf("created_at missing from JSON: %v", result)
	}
	if _, ok := result["expires_at"].(string); !ok {
		t.Errorf("expires_at missing from JSON: %v", result)
	}
	if _, ok := result["reason"].(string); !ok {
		t.Errorf("reason missing from JSON: %v", result)
	}

	// Ensure a second decode yields EOF (single JSON object, nothing trailing).
	var extra any
	if err := dec.Decode(&extra); err == nil {
		t.Errorf("expected a single JSON object, but found trailing content: %v", extra)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"minutes", 45 * time.Minute, "45m"},
		{"hours", 2 * time.Hour, "2h"},
		{"hours and minutes", 2*time.Hour + 30*time.Minute, "2h"},
		{"days", 3 * 24 * time.Hour, "3d"},
		{"days and hours", 3*24*time.Hour + 12*time.Hour, "3d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, expected %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestFormatExpiration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"expired", -1 * time.Hour, "EXPIRED"},
		{"minutes", 45 * time.Minute, "soon"},
		{"hours", 2 * time.Hour, "2h"},
		{"days", 3 * 24 * time.Hour, "3d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatExpiration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatExpiration(%v) = %q, expected %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestSnapshotCommandIntegration(t *testing.T) {
	// This test verifies that snapshot commands are registered correctly
	root := newRootCommand()

	// Find snapshot command
	var snapshotFound bool
	for _, cmd := range root.Commands() {
		if cmd.Name() == "snapshot" {
			snapshotFound = true

			// Verify subcommands
			subcommands := map[string]bool{
				"create":  false,
				"list":    false,
				"cleanup": false,
			}

			for _, subCmd := range cmd.Commands() {
				if _, ok := subcommands[subCmd.Name()]; ok {
					subcommands[subCmd.Name()] = true
				}
			}

			for name, found := range subcommands {
				if !found {
					t.Errorf("expected snapshot subcommand %q to be registered", name)
				}
			}

			break
		}
	}

	if !snapshotFound {
		t.Error("expected snapshot command to be registered")
	}
}

func TestSnapshotCreateRequiresDatabase(t *testing.T) {
	// Verify that create command fails gracefully without DATABASE_URL
	// This is tested by attempting to create without config

	// Note: Full integration test would require actual database
	// This test documents the requirement
	t.Skip("Requires DATABASE_URL environment variable, tested in integration tests")
}

func TestSnapshotListNoSnapshots(t *testing.T) {
	// Test that list command handles empty snapshot directory gracefully
	// This would require setting up a temp directory

	// Note: Full integration test would set up temp directory
	t.Skip("Requires snapshot directory setup, tested in integration tests")
}

func TestSnapshotCleanupDryRun(t *testing.T) {
	// Test that dry-run doesn't delete files
	// This would require setting up test snapshots

	// Note: Full integration test would create test snapshots
	t.Skip("Requires snapshot setup, tested in integration tests")
}
