package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestCleanupLoadtestCommand_Help(t *testing.T) {
	cmd := newRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"cleanup", "loadtest", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "loadtest") {
		t.Errorf("expected help output to mention loadtest, got: %s", out)
	}
	if !strings.Contains(out, "--env") {
		t.Errorf("expected help output to mention --env flag, got: %s", out)
	}
	if !strings.Contains(out, "--source-id") {
		t.Errorf("expected help output to mention --source-id flag, got: %s", out)
	}
	if !strings.Contains(out, "--legacy") {
		t.Errorf("expected help output to mention --legacy flag, got: %s", out)
	}
	if !strings.Contains(out, "--dry-run") {
		t.Errorf("expected help output to mention --dry-run flag, got: %s", out)
	}
	if !strings.Contains(out, "--confirm") {
		t.Errorf("expected help output to mention --confirm flag, got: %s", out)
	}
}

// resetLoadtestFlags resets the package-level loadtest flag vars to their
// default values between test runs. Required because cobra only sets flags that
// are explicitly present on the command line — previous test runs can pollute
// the shared vars. Also resets cobra's internal help flag so a prior --help
// invocation does not suppress RunE in the next test.
func resetLoadtestFlags(t *testing.T) {
	t.Helper()
	loadtestEnv = ""
	loadtestSourceID = ""
	loadtestLegacy = false
	loadtestDryRun = false
	loadtestConfirm = false
	// Reset cobra's internal "help" flag so a prior --help invocation does not
	// prevent RunE from being called in subsequent tests.
	if hf := cleanupLoadtestCmd.Flags().Lookup("help"); hf != nil {
		_ = hf.Value.Set("false")
	}
}

func TestCleanupLoadtestCommand_GuardEnv(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectError   bool
		errorContains string
	}{
		{
			name:          "missing --env flag",
			args:          []string{"cleanup", "loadtest", "--source-id=abc", "--dry-run"},
			expectError:   true,
			errorContains: "env",
		},
		{
			name:          "env=production rejected",
			args:          []string{"cleanup", "loadtest", "--env=production", "--source-id=abc", "--dry-run"},
			expectError:   true,
			errorContains: "staging",
		},
		{
			name:          "env=development rejected",
			args:          []string{"cleanup", "loadtest", "--env=development", "--source-id=abc", "--dry-run"},
			expectError:   true,
			errorContains: "staging",
		},
		{
			name:          "env=prod rejected",
			args:          []string{"cleanup", "loadtest", "--env=prod", "--source-id=abc", "--dry-run"},
			expectError:   true,
			errorContains: "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLoadtestFlags(t)
			cmd := newRootCommand()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil (output: %s)", buf.String())
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestCleanupLoadtestCommand_GuardMode(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectError   bool
		errorContains string
	}{
		{
			name:          "no mode specified",
			args:          []string{"cleanup", "loadtest", "--env=staging", "--dry-run"},
			expectError:   true,
			errorContains: "--source-id",
		},
		{
			name:          "no action specified (no --dry-run or --confirm)",
			args:          []string{"cleanup", "loadtest", "--env=staging", "--legacy"},
			expectError:   true,
			errorContains: "--dry-run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetLoadtestFlags(t)
			cmd := newRootCommand()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil (output: %s)", buf.String())
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestDBURLHost covers URL and DSN parsing.
func TestDBURLHost(t *testing.T) {
	tests := []struct {
		name      string
		dbURL     string
		wantHost  string
		wantError bool
	}{
		{
			name:     "postgres:// scheme",
			dbURL:    "postgres://user:pass@db.staging.example.com:5432/mydb",
			wantHost: "db.staging.example.com",
		},
		{
			name:     "postgresql:// scheme",
			dbURL:    "postgresql://user@localhost/mydb",
			wantHost: "localhost",
		},
		{
			name:     "DSN key=value format",
			dbURL:    "host=db.staging.example.com port=5432 dbname=mydb user=postgres",
			wantHost: "db.staging.example.com",
		},
		{
			name:      "unrecognised format",
			dbURL:     "not-a-url",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := dbURLHost(tt.dbURL)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil (host=%q)", host)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
		})
	}
}

// TestCleanupLoadtestCommand_InvalidSourceIDUUID verifies that an invalid UUID
// passed via --source-id is rejected before any database connection is attempted.
func TestCleanupLoadtestCommand_InvalidSourceIDUUID(t *testing.T) {
	resetLoadtestFlags(t)
	cmd := newRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"cleanup", "loadtest", "--env=staging", "--source-id=not-a-uuid", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid UUID, got nil (output: %s)", buf.String())
	}
	if !strings.Contains(err.Error(), "not a valid UUID") {
		t.Errorf("expected error to contain %q, got: %v", "not a valid UUID", err)
	}
}

// TestGuardNotProductionDB verifies production-host detection logic.
func TestGuardNotProductionDB(t *testing.T) {
	tests := []struct {
		name           string
		dbURL          string
		productionHost string // value to set in PRODUCTION_DB_HOST env var
		expectError    bool
		errorContains  string
	}{
		{
			name:        "staging host passes without env var",
			dbURL:       "postgres://user@db.staging.example.com:5432/mydb",
			expectError: false,
		},
		{
			name:          "hostname contains prod — blocked by heuristic",
			dbURL:         "postgres://user@db.production.example.com/mydb",
			expectError:   true,
			errorContains: "prod",
		},
		{
			name:           "exact match via PRODUCTION_DB_HOST env var",
			dbURL:          "postgres://user@db.myserver.com/mydb",
			productionHost: "db.myserver.com",
			expectError:    true,
			errorContains:  "production",
		},
		{
			name:           "different host passes when PRODUCTION_DB_HOST is set",
			dbURL:          "postgres://user@db.staging.myserver.com/mydb",
			productionHost: "db.production.myserver.com",
			expectError:    false,
		},
		{
			name:           "case-insensitive match via PRODUCTION_DB_HOST",
			dbURL:          "postgres://user@DB.PROD.MYSERVER.COM/mydb",
			productionHost: "db.prod.myserver.com",
			expectError:    true,
			errorContains:  "production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.productionHost != "" {
				t.Setenv(productionDBHostEnvVar, tt.productionHost)
			} else {
				t.Setenv(productionDBHostEnvVar, "")
			}

			err := guardNotProductionDB(tt.dbURL)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
