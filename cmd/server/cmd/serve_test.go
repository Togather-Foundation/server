package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestServeCommandHelp(t *testing.T) {
	cmd := newServeCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("serve command --help failed: %v", err)
	}

	output := buf.String()

	// Verify help text contains expected content
	expectedStrings := []string{
		"Start the SEL HTTP server",
		"--host",
		"--port",
		"server host address",
		"server port",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected help text to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestServeCommandFlags(t *testing.T) {
	cmd := newServeCommand()

	// Verify that serve-specific flags are registered
	flags := []string{"host", "port"}
	for _, flag := range flags {
		if f := cmd.Flags().Lookup(flag); f == nil {
			t.Errorf("expected flag %q to be defined on serve command", flag)
		}
	}
}

func TestServeCommandFlagParsing(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "valid host flag",
			args:        []string{"--host", "127.0.0.1"},
			expectError: false,
		},
		{
			name:        "valid port flag",
			args:        []string{"--port", "9090"},
			expectError: false,
		},
		{
			name:        "valid host and port",
			args:        []string{"--host", "0.0.0.0", "--port", "8080"},
			expectError: false,
		},
		{
			name:        "invalid port value",
			args:        []string{"--port", "invalid"},
			expectError: true,
		},
		{
			name:        "unknown flag",
			args:        []string{"--unknown"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newServeCommand()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestServeCommandGlobalFlags(t *testing.T) {
	// Create root command with serve as subcommand to test global flag inheritance
	root := newRootCommand()

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"serve", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("serve command with global flags failed: %v", err)
	}

	output := buf.String()

	// Verify global flags are available in serve command
	globalFlags := []string{"--config", "--log-level", "--log-format"}
	for _, flag := range globalFlags {
		if !strings.Contains(output, flag) {
			t.Errorf("expected help text to contain global flag %q, got:\n%s", flag, output)
		}
	}
}

func TestLoadConfigFallback(t *testing.T) {
	// Test that loadConfig falls back gracefully when env vars are missing
	// This is a basic smoke test - actual config loading would require database setup

	// Set minimal required env vars
	os.Setenv("DATABASE_URL", "postgres://test")
	os.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("JWT_SECRET")
	}()

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig should succeed with minimal env vars: %v", err)
	}

	// Verify defaults are applied
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoadConfigFlagOverrides(t *testing.T) {
	// Set minimal required env vars
	os.Setenv("DATABASE_URL", "postgres://test")
	os.Setenv("JWT_SECRET", "test-secret-at-least-32-characters-long")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("JWT_SECRET")
	}()

	// Set global flag variables (simulating flags being set)
	logLevel = "debug"
	logFormat = "console"
	defer func() {
		logLevel = ""
		logFormat = ""
	}()

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Verify flag overrides are applied
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level 'debug', got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "console" {
		t.Errorf("expected log format 'console', got %s", cfg.Logging.Format)
	}
}

func TestLoadConfigMissingRequiredVars(t *testing.T) {
	// Clear required env vars to test error handling
	origDatabaseURL := os.Getenv("DATABASE_URL")
	origJWTSecret := os.Getenv("JWT_SECRET")
	defer func() {
		if origDatabaseURL != "" {
			os.Setenv("DATABASE_URL", origDatabaseURL)
		}
		if origJWTSecret != "" {
			os.Setenv("JWT_SECRET", origJWTSecret)
		}
	}()

	tests := []struct {
		name        string
		databaseURL string
		jwtSecret   string
		expectError bool
	}{
		{
			name:        "missing DATABASE_URL",
			databaseURL: "",
			jwtSecret:   "test-secret-at-least-32-characters-long",
			expectError: true,
		},
		{
			name:        "missing JWT_SECRET",
			databaseURL: "postgres://test",
			jwtSecret:   "",
			expectError: true,
		},
		{
			name:        "JWT_SECRET too short",
			databaseURL: "postgres://test",
			jwtSecret:   "short",
			expectError: true,
		},
		{
			name:        "valid config",
			databaseURL: "postgres://test",
			jwtSecret:   "test-secret-at-least-32-characters-long",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("DATABASE_URL")
			os.Unsetenv("JWT_SECRET")
			if tt.databaseURL != "" {
				os.Setenv("DATABASE_URL", tt.databaseURL)
			}
			if tt.jwtSecret != "" {
				os.Setenv("JWT_SECRET", tt.jwtSecret)
			}

			_, err := loadConfig()

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
