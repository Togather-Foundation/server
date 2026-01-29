package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
)

var (
	apiKeyRole string
)

// apiKeyCmd represents the api-key command group
var apiKeyCmd = &cobra.Command{
	Use:   "api-key",
	Short: "Manage API keys",
	Long: `Manage API keys for accessing the SEL API.

API keys are used to authenticate with the SEL API endpoints.
Keys can have different roles (agent, admin) with different permissions.

Examples:
  # Create a new API key
  server api-key create my-agent

  # List all API keys
  server api-key list

  # Revoke an API key
  server api-key revoke <id>`,
}

// apiKeyCreateCmd creates a new API key
var apiKeyCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new API key",
	Long: `Create a new API key for API authentication.

The key will be displayed once and cannot be retrieved later.
Save the key in a secure location.

Roles:
  agent - Standard API access for event ingestion
  admin - Full administrative access

Examples:
  # Create agent key
  server api-key create my-agent

  # Create admin key
  server api-key create my-admin --role admin`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		_, err := createAPIKey(name, apiKeyRole)
		return err
	},
}

// apiKeyListCmd lists all API keys
var apiKeyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all API keys",
	Long: `List all API keys with their details.

Shows key ID, name, role, status, and creation date.
The actual key values are not shown (they cannot be retrieved).

Examples:
  # List all keys
  server api-key list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listAPIKeys()
	},
}

// apiKeyRevokeCmd revokes an API key
var apiKeyRevokeCmd = &cobra.Command{
	Use:   "revoke <id>",
	Short: "Revoke an API key",
	Long: `Revoke an API key by its ID.

The key will be marked as inactive and can no longer be used.
This action cannot be undone.

Examples:
  # Revoke a key
  server api-key revoke 01234567-89ab-cdef-0123-456789abcdef`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		keyID := args[0]
		return revokeAPIKey(keyID)
	},
}

func init() {
	// Add api-key command group
	rootCmd.AddCommand(apiKeyCmd)

	// Add subcommands
	apiKeyCmd.AddCommand(apiKeyCreateCmd)
	apiKeyCmd.AddCommand(apiKeyListCmd)
	apiKeyCmd.AddCommand(apiKeyRevokeCmd)

	// Flags for create
	apiKeyCreateCmd.Flags().StringVar(&apiKeyRole, "role", "agent", "role for the API key (agent or admin)")
}

func createAPIKey(name, role string) (string, error) {
	// Load DATABASE_URL from environment or .env files
	dbURL := getDatabaseURL()
	if dbURL == "" {
		return "", fmt.Errorf("DATABASE_URL not set\n\nTried loading from:\n  - Environment variable DATABASE_URL\n  - .env file in project root\n  - deploy/docker/.env\n\nPlease set DATABASE_URL or create a .env file")
	}

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return "", fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Generate API key
	key := ulid.Make().String() + "secret"
	prefix := key[:8]

	// Hash the key
	hash, err := auth.HashAPIKey(key)
	if err != nil {
		return "", fmt.Errorf("hash API key: %w", err)
	}

	// Insert into database
	_, err = pool.Exec(ctx,
		`INSERT INTO api_keys (prefix, key_hash, hash_version, name, role, is_active)
		 VALUES ($1, $2, $3, $4, $5, true)`,
		prefix, hash, auth.HashVersionBcrypt, name, role,
	)
	if err != nil {
		return "", fmt.Errorf("insert API key: %w", err)
	}

	// Success output
	fmt.Printf("✓ API key created successfully!\n\n")
	fmt.Printf("Name:   %s\n", name)
	fmt.Printf("Role:   %s\n", role)
	fmt.Printf("Key:    %s\n\n", key)
	fmt.Printf("⚠️  Save this key - it cannot be retrieved later!\n\n")
	fmt.Printf("Usage:\n")
	fmt.Printf("  export API_KEY=%s\n", key)
	fmt.Printf("  server ingest events.json\n")
	fmt.Printf("  curl -H \"Authorization: Bearer $API_KEY\" http://localhost:8080/api/v1/events\n")

	return key, nil
}

func listAPIKeys() error {
	dbURL := getDatabaseURL()
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL not set (required for database operations)")
	}

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Query API keys
	rows, err := pool.Query(ctx, `
		SELECT id, name, role, is_active, created_at, last_used_at
		FROM api_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return fmt.Errorf("query API keys: %w", err)
	}
	defer rows.Close()

	// Print results in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tROLE\tSTATUS\tCREATED\tLAST USED")
	fmt.Fprintln(w, "--\t----\t----\t------\t-------\t---------")

	count := 0
	for rows.Next() {
		var id, name, role string
		var isActive bool
		var createdAt time.Time
		var lastUsedAt *time.Time

		if err := rows.Scan(&id, &name, &role, &isActive, &createdAt, &lastUsedAt); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}

		status := "active"
		if !isActive {
			status = "revoked"
		}

		lastUsed := "never"
		if lastUsedAt != nil {
			lastUsed = lastUsedAt.Format("2006-01-02")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			id[:8]+"...", // Truncate UUID for display
			name,
			role,
			status,
			createdAt.Format("2006-01-02"),
			lastUsed,
		)
		count++
	}

	w.Flush()

	if count == 0 {
		fmt.Println("\nNo API keys found. Create one with: server api-key create <name>")
	} else {
		fmt.Printf("\nTotal: %d API key(s)\n", count)
	}

	return nil
}

func revokeAPIKey(keyID string) error {
	dbURL := getDatabaseURL()
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL not set (required for database operations)")
	}

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Update key to inactive
	result, err := pool.Exec(ctx,
		`UPDATE api_keys SET is_active = false WHERE id = $1`,
		keyID,
	)
	if err != nil {
		return fmt.Errorf("revoke API key: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("API key not found: %s", keyID)
	}

	fmt.Printf("✓ API key revoked successfully: %s\n", keyID)
	return nil
}

// getDatabaseURL gets DATABASE_URL from environment or .env files
func getDatabaseURL() string {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		return dbURL
	}

	// Try loading from .env files
	loadEnvFileSimple(".env")
	dbURL = os.Getenv("DATABASE_URL")
	if dbURL != "" {
		return dbURL
	}

	loadEnvFileSimple("deploy/docker/.env")
	return os.Getenv("DATABASE_URL")
}

// loadEnvFileSimple loads environment variables from a .env file
// Silently ignores if file doesn't exist
func loadEnvFileSimple(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Only set if not already in environment
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}
