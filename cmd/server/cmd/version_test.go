package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	// Save original version variables
	origVersion := Version
	origGitCommit := GitCommit
	origBuildDate := BuildDate
	defer func() {
		Version = origVersion
		GitCommit = origGitCommit
		BuildDate = origBuildDate
	}()

	// Set test values
	Version = "1.0.0"
	GitCommit = "abc123"
	BuildDate = "2026-01-27T12:00:00Z"

	// Create root command with version subcommand
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"version"})

	// Execute version command
	if err := root.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	output := buf.String()

	// Verify output contains expected information
	expectedStrings := []string{
		"Togather SEL Server",
		"Version:    1.0.0",
		"Git commit: abc123",
		"Build date: 2026-01-27T12:00:00Z",
		"Go version:",
		"Platform:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestVersionCommandDefaultValues(t *testing.T) {
	// Save original version variables
	origVersion := Version
	origGitCommit := GitCommit
	origBuildDate := BuildDate
	defer func() {
		Version = origVersion
		GitCommit = origGitCommit
		BuildDate = origBuildDate
	}()

	// Set to default values (as they would be without ldflags)
	Version = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"

	// Create root command with version subcommand
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"version"})

	// Execute version command
	if err := root.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	output := buf.String()

	// Verify default values are shown
	expectedStrings := []string{
		"Version:    dev",
		"Git commit: unknown",
		"Build date: unknown",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestVersionCommandHelp(t *testing.T) {
	// Create root command with version subcommand
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"version", "--help"})

	// Execute version command with --help
	if err := root.Execute(); err != nil {
		t.Fatalf("version command --help failed: %v", err)
	}

	output := buf.String()

	// Verify help text is present
	if !strings.Contains(output, "Print the version number") {
		t.Errorf("expected help text to contain version description, got:\n%s", output)
	}
}

func TestVersionCommandNoServerStart(t *testing.T) {
	// This test verifies that the version command runs without requiring
	// any server dependencies (config, database, etc.)

	// Save original version variables
	origVersion := Version
	origGitCommit := GitCommit
	origBuildDate := BuildDate
	defer func() {
		Version = origVersion
		GitCommit = origGitCommit
		BuildDate = origBuildDate
	}()

	Version = "test"
	GitCommit = "test"
	BuildDate = "test"

	// Execute without any environment variables set
	// If this succeeds, it proves the command doesn't try to start the server
	root := newRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Errorf("version command should not require server dependencies, got error: %v", err)
	}

	// Verify output was produced (proving the command ran)
	if buf.Len() == 0 {
		t.Error("version command produced no output")
	}
}
