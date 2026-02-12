package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var (
	developerName string
)

// developerCmd represents the developer command group
var developerCmd = &cobra.Command{
	Use:   "developer",
	Short: "Manage developer accounts",
	Long: `Manage developer accounts for accessing the SEL API.

Developers can create and manage API keys for their applications.
This command provides administrative operations for developer management.

Examples:
  # Invite a new developer
  server developer invite alice@example.com --name "Alice"

  # List all developers
  server developer list

  # Deactivate a developer
  server developer deactivate <id>`,
}

// developerInviteCmd invites a new developer
var developerInviteCmd = &cobra.Command{
	Use:   "invite <email>",
	Short: "Invite a new developer",
	Long: `Invite a new developer by email.

Generates a secure invitation token that can be used to create
a developer account. The token is single-use and expires in 7 days.

Examples:
  # Invite developer with name
  server developer invite alice@example.com --name "Alice"

  # Invite developer without name
  server developer invite bob@example.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		email := args[0]
		return inviteDeveloper(email, developerName)
	},
}

// developerListCmd lists all developers
var developerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all developers",
	Long: `List all developers with their details.

Shows developer ID, email, name, GitHub username, active status,
and the number of active API keys they have created.

Examples:
  # List all developers
  server developer list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listDevelopers()
	},
}

// developerDeactivateCmd deactivates a developer
var developerDeactivateCmd = &cobra.Command{
	Use:   "deactivate <id>",
	Short: "Deactivate a developer account",
	Long: `Deactivate a developer account by its ID.

This will mark the developer as inactive and revoke all their API keys.
Deactivated developers cannot authenticate or use their API keys.
This action cannot be undone from the CLI (requires database access).

Examples:
  # Deactivate a developer
  server developer deactivate 550e8400-e29b-41d4-a716-446655440000`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		developerID := args[0]
		return deactivateDeveloper(developerID)
	},
}

func init() {
	// Add developer command group
	rootCmd.AddCommand(developerCmd)

	// Add subcommands
	developerCmd.AddCommand(developerInviteCmd)
	developerCmd.AddCommand(developerListCmd)
	developerCmd.AddCommand(developerDeactivateCmd)

	// Flags for invite
	developerInviteCmd.Flags().StringVar(&developerName, "name", "", "developer's name (optional)")
}

func inviteDeveloper(email, name string) error {
	// Load DATABASE_URL from environment or .env files
	dbURL := getDatabaseURL()
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL not set\n\nTried loading from:\n  - Environment variable DATABASE_URL\n  - .env file in project root\n  - deploy/docker/.env\n\nPlease set DATABASE_URL or create a .env file")
	}

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Generate secure token (32 bytes)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Hash the token with SHA-256
	hash := sha256.Sum256([]byte(token))
	tokenHash := base64.URLEncoding.EncodeToString(hash[:])

	// Create invitation
	expiresAt := time.Now().Add(168 * time.Hour) // 7 days
	repo := postgres.NewDeveloperRepository(pool)

	params := postgres.NewCreateDeveloperInvitationParams(
		email,
		tokenHash,
		pgtype.UUID{Valid: false}, // No inviter (admin CLI)
		expiresAt,
	)

	invitation, err := repo.CreateDeveloperInvitation(ctx, params)
	if err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}

	// Generate invitation URL
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	invitationURL := fmt.Sprintf("%s/developer/accept?token=%s", baseURL, token)

	// Success output
	fmt.Printf("✓ Developer invitation created successfully!\n\n")
	fmt.Printf("Email:        %s\n", email)
	if name != "" {
		fmt.Printf("Name:         %s\n", name)
	}
	fmt.Printf("Invitation:   %s\n", invitation.ID.Bytes)
	fmt.Printf("Token:        %s\n\n", token)
	fmt.Printf("Invitation URL:\n%s\n\n", invitationURL)
	fmt.Printf("⚠️  The invitation expires in 7 days and is single-use.\n")
	fmt.Printf("    Send this URL to the developer to complete registration.\n")

	return nil
}

func listDevelopers() error {
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

	// Create repository
	repo := postgres.NewDeveloperRepository(pool)

	// List developers (fetch up to 1000 developers)
	devs, err := repo.ListDevelopers(ctx, 1000, 0)
	if err != nil {
		return fmt.Errorf("list developers: %w", err)
	}

	// Print results in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tEMAIL\tNAME\tGITHUB\tACTIVE\tKEYS")
	_, _ = fmt.Fprintln(w, "--\t-----\t----\t------\t------\t----")

	count := 0
	for _, dev := range devs {
		// Count API keys for this developer
		keyCount, err := repo.CountDeveloperAPIKeys(ctx, dev.ID)
		if err != nil {
			keyCount = 0
		}

		activeStatus := "active"
		if !dev.IsActive {
			activeStatus = "inactive"
		}

		github := "-"
		if dev.GithubUsername.Valid {
			github = dev.GithubUsername.String
		}

		// Convert pgtype.UUID to string for display
		idStr := "unknown"
		if dev.ID.Valid {
			// Format UUID bytes as hex string
			b := dev.ID.Bytes
			idStr = fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])[:8] + "..."
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n",
			idStr,
			dev.Email,
			dev.Name,
			github,
			activeStatus,
			keyCount,
		)
		count++
	}

	_ = w.Flush()

	if count == 0 {
		fmt.Println("\nNo developers found. Invite one with: server developer invite <email>")
	} else {
		fmt.Printf("\nTotal: %d developer(s)\n", count)
	}

	return nil
}

func deactivateDeveloper(developerIDStr string) error {
	dbURL := getDatabaseURL()
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL not set (required for database operations)")
	}

	// Parse UUID using google/uuid package
	parsedUUID, err := uuid.Parse(developerIDStr)
	if err != nil {
		return fmt.Errorf("invalid developer ID format: %w", err)
	}

	// Convert google/uuid.UUID to pgtype.UUID
	var developerID pgtype.UUID
	developerID.Bytes = parsedUUID
	developerID.Valid = true

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Create repository
	repo := postgres.NewDeveloperRepository(pool)

	// Get developer to verify it exists
	dev, err := repo.GetDeveloperByID(ctx, developerID)
	if err != nil {
		return fmt.Errorf("developer not found: %s", developerIDStr)
	}

	// Deactivate developer
	if err := repo.DeactivateDeveloper(ctx, developerID); err != nil {
		return fmt.Errorf("deactivate developer: %w", err)
	}

	// Revoke all active API keys atomically using the new query
	revokedCount, err := repo.RevokeAllDeveloperAPIKeys(ctx, developerID)
	if err != nil {
		return fmt.Errorf("failed to revoke API keys: %w", err)
	}

	fmt.Printf("✓ Developer deactivated successfully!\n\n")
	fmt.Printf("Developer:    %s (%s)\n", dev.Email, dev.Name)
	fmt.Printf("API keys:     %d revoked\n\n", revokedCount)
	fmt.Printf("⚠️  This action cannot be undone from the CLI.\n")
	fmt.Printf("    To reactivate, update is_active in the database.\n")

	return nil
}
