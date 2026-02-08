package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "help flag",
			args:           []string{"--help"},
			expectedOutput: "Togather SEL server",
			expectError:    false,
		},
		{
			name:           "short help flag",
			args:           []string{"-h"},
			expectedOutput: "Togather SEL server",
			expectError:    false,
		},
		{
			name:           "invalid flag",
			args:           []string{"--invalid-flag"},
			expectedOutput: "unknown flag: --invalid-flag",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test to avoid state pollution
			cmd := newRootCommand()

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			output := buf.String()

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("expected output to contain %q, got:\n%s", tt.expectedOutput, output)
			}
		})
	}
}

func TestRootCommandPersistentFlags(t *testing.T) {
	cmd := newRootCommand()

	// Test that persistent flags are available
	flags := []string{"config", "log-level", "log-format"}
	for _, flag := range flags {
		if f := cmd.PersistentFlags().Lookup(flag); f == nil {
			t.Errorf("expected persistent flag %q to be defined", flag)
		}
	}
}

func TestRootCommandSubcommands(t *testing.T) {
	cmd := newRootCommand()

	// Test that expected subcommands are available
	expectedCommands := []string{"serve", "version"}
	for _, cmdName := range expectedCommands {
		found := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q to be registered", cmdName)
		}
	}
}

// newRootCommand creates a fresh root command for testing
func newRootCommand() *cobra.Command {
	testRootCmd := &cobra.Command{
		Use:   "server",
		Short: "Togather SEL server - Shared Events Library backend",
		Long: `Togather SEL server implements the Shared Events Library (SEL) specification,
providing a federated backend for event data aggregation, enrichment, and distribution.

The server supports:
- Event ingestion from multiple sources (HTML, JSON-LD, ICS, RSS, APIs)
- Duplicate detection and deduplication
- JSON-LD export with Schema.org vocabulary
- Federation with other SEL nodes
- Knowledge graph reconciliation (Artsdata, Wikidata)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// For tests, don't actually run the server
			return nil
		},
	}

	// Add persistent flags
	var configPath, logLevel, logFormat string
	testRootCmd.PersistentFlags().StringVar(&configPath, "config", "", "config file path (optional, uses env vars by default)")
	testRootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error) (default: info)")
	testRootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "", "log format (json, console) (default: json)")

	// Remove commands from any previous parent to avoid state pollution
	// This is necessary because commands are package-level variables
	if deployCmd.HasParent() {
		deployCmd.Parent().RemoveCommand(deployCmd)
	}
	if ingestCmd.HasParent() {
		ingestCmd.Parent().RemoveCommand(ingestCmd)
	}
	if snapshotCmd.HasParent() {
		snapshotCmd.Parent().RemoveCommand(snapshotCmd)
	}
	if healthcheckCmd.HasParent() {
		healthcheckCmd.Parent().RemoveCommand(healthcheckCmd)
	}
	if cleanupCmd.HasParent() {
		cleanupCmd.Parent().RemoveCommand(cleanupCmd)
	}
	if setupCmd.HasParent() {
		setupCmd.Parent().RemoveCommand(setupCmd)
	}
	if versionCmd.HasParent() {
		versionCmd.Parent().RemoveCommand(versionCmd)
	}

	// Add all subcommands
	testRootCmd.AddCommand(versionCmd)
	testRootCmd.AddCommand(newServeCommand())
	testRootCmd.AddCommand(setupCmd)
	testRootCmd.AddCommand(snapshotCmd)
	testRootCmd.AddCommand(deployCmd)
	testRootCmd.AddCommand(ingestCmd)
	testRootCmd.AddCommand(healthcheckCmd)
	testRootCmd.AddCommand(cleanupCmd)

	return testRootCmd
}

// newServeCommand creates a serve command for testing (doesn't start server)
func newServeCommand() *cobra.Command {
	var serverHost string
	var serverPort int

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the SEL HTTP server",
		Long:  `Start the SEL HTTP server and begin accepting API requests.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// For tests, don't actually start the server
			return nil
		},
	}

	serveCmd.Flags().StringVar(&serverHost, "host", "", "server host address (default: 0.0.0.0)")
	serveCmd.Flags().IntVar(&serverPort, "port", 0, "server port (default: 8080)")

	return serveCmd
}
