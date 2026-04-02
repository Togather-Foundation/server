package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/spf13/cobra"
)

var adminTokenDuration time.Duration

// adminTokenCmd generates a short-lived admin JWT Bearer token from JWT_SECRET.
// No database connection required — reads JWT_SECRET from env/.env and derives
// the admin signing key via HKDF, then prints a Bearer token to stdout.
//
// Usage:
//
//	TOKEN=$(server admin-token)
//	curl -H "Authorization: Bearer $TOKEN" https://host/api/v1/admin/...
var adminTokenCmd = &cobra.Command{
	Use:   "admin-token",
	Short: "Generate a short-lived admin JWT token",
	Long: `Generate a short-lived admin JWT Bearer token for API access.

Reads JWT_SECRET from the environment or a local .env file, derives the admin
signing key (same HKDF derivation the server uses), and prints a signed Bearer
token to stdout.  No database connection is required.

The token is valid for the specified duration (default 1h).

Examples:
  # One-liner for curl usage
  TOKEN=$(server admin-token)
  curl -H "Authorization: Bearer $TOKEN" https://staging.toronto.togather.foundation/api/v1/admin/scraper/diagnostics

  # Write token to a file for reuse
  server admin-token > .agent-keys/staging

  # Longer-lived token (e.g. 8 hours for a work session)
  server admin-token --duration 8h`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return generateAdminToken(adminTokenDuration)
	},
}

func init() {
	rootCmd.AddCommand(adminTokenCmd)
	adminTokenCmd.Flags().DurationVar(&adminTokenDuration, "duration", time.Hour, "token validity duration (e.g. 1h, 30m, 8h)")
}

func generateAdminToken(duration time.Duration) error {
	// Load JWT_SECRET from environment or .env files
	secret := getJWTSecret()
	if secret == "" {
		return fmt.Errorf("JWT_SECRET not set\n\nTried loading from:\n  - Environment variable JWT_SECRET\n  - .env file in project root\n  - deploy/docker/.env\n\nPlease set JWT_SECRET or create a .env file")
	}

	// Derive admin JWT key (same HKDF derivation the server uses)
	derivedKey, err := auth.DeriveAdminJWTKey([]byte(secret))
	if err != nil {
		return fmt.Errorf("derive admin JWT key: %w", err)
	}

	// Generate signed token
	mgr := auth.NewJWTManagerFromKey(derivedKey, duration, "togather-server")
	token, err := mgr.Generate("agent-cli", "admin")
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	fmt.Print(token)
	return nil
}

// getJWTSecret reads JWT_SECRET from the environment or .env files.
func getJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret != "" {
		return secret
	}

	loadEnvFileSimple(".env")
	secret = os.Getenv("JWT_SECRET")
	if secret != "" {
		return secret
	}

	loadEnvFileSimple("deploy/docker/.env")
	return os.Getenv("JWT_SECRET")
}
