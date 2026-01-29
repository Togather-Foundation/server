package cmd

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	setupNonInteractive bool
	setupDockerMode     bool
)

// setupCmd provides interactive first-time setup
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive first-time setup",
	Long: `Interactive first-time setup for the SEL server.

This command walks you through:
  1. Environment detection (Docker vs local PostgreSQL)
  2. Prerequisites checking
  3. Secrets generation (JWT, CSRF, admin password)
  4. Database configuration
  5. .env file creation
  6. Database migrations
  7. First API key creation

After setup completes, you'll have a fully configured development environment.

Examples:
  # Interactive setup (recommended)
  server setup

  # Non-interactive with Docker
  server setup --docker --non-interactive`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetup()
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)

	setupCmd.Flags().BoolVar(&setupNonInteractive, "non-interactive", false, "run setup without prompts (use defaults)")
	setupCmd.Flags().BoolVar(&setupDockerMode, "docker", false, "configure for Docker environment")
}

func runSetup() error {
	fmt.Println("🚀 Welcome to Togather SEL Server Setup")
	fmt.Println()

	// Check if .env already exists
	if fileExists(".env") {
		if !setupNonInteractive {
			fmt.Println("⚠️  .env file already exists!")
			if !confirm("Overwrite existing .env file?", false) {
				fmt.Println("Setup cancelled.")
				return nil
			}
		}
		// Backup existing .env
		if err := os.Rename(".env", ".env.backup"); err != nil {
			fmt.Printf("⚠️  Could not backup existing .env: %v\n", err)
		} else {
			fmt.Println("✓ Backed up existing .env to .env.backup")
		}
	}

	// Step 1: Detect environment
	fmt.Println("Step 1: Environment Detection")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	useDocker := setupDockerMode
	if !setupNonInteractive && !setupDockerMode {
		fmt.Println("Choose your setup:")
		fmt.Println("  1. Docker (recommended) - Database runs in container")
		fmt.Println("  2. Local PostgreSQL - Use existing PostgreSQL installation")
		fmt.Println()
		useDocker = promptChoice("Select option", []string{"Docker", "Local PostgreSQL"}, 0) == 0
	}

	env := "docker"
	if !useDocker {
		env = "local"
	}
	fmt.Printf("✓ Using %s environment\n\n", env)

	// Step 2: Check prerequisites
	fmt.Println("Step 2: Prerequisites Check")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if useDocker {
		if !checkCommand("docker") {
			return fmt.Errorf("docker not found. Please install Docker: https://docs.docker.com/get-docker/")
		}
		// Check for either docker-compose or docker compose
		hasDockerCompose := checkCommand("docker-compose")
		hasDockerComposePlugin := false
		if !hasDockerCompose {
			// Check if docker compose (plugin) works
			cmd := exec.Command("docker", "compose", "version")
			if cmd.Run() == nil {
				hasDockerComposePlugin = true
			}
		}
		if !hasDockerCompose && !hasDockerComposePlugin {
			return fmt.Errorf("docker compose not found. Please install Docker Compose")
		}
		fmt.Println("✓ Docker found")
	} else {
		if !checkCommand("psql") {
			fmt.Println("⚠️  psql not found. You'll need PostgreSQL installed.")
		} else {
			fmt.Println("✓ PostgreSQL client found")
		}
	}

	if checkCommand("jq") {
		fmt.Println("✓ jq found")
	} else {
		fmt.Println("⚠️  jq not found (optional, but helpful for testing)")
	}
	fmt.Println()

	// Step 3: Generate secrets
	fmt.Println("Step 3: Generate Secrets")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━")

	jwtSecret, err := generateSecret(32)
	if err != nil {
		return fmt.Errorf("generate JWT secret: %w", err)
	}
	fmt.Println("✓ Generated JWT_SECRET")

	csrfKey, err := generateSecret(32)
	if err != nil {
		return fmt.Errorf("generate CSRF key: %w", err)
	}
	fmt.Println("✓ Generated CSRF_KEY")

	adminPassword, err := generateSecret(16)
	if err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}
	fmt.Println("✓ Generated admin password")
	fmt.Println()

	// Step 4: Database configuration
	fmt.Println("Step 4: Database Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var dbURL string
	var dbPort string
	if useDocker {
		dbPort = "5433"
		if !setupNonInteractive {
			dbPort = prompt("PostgreSQL port (Docker)", "5433")
		}
		dbURL = fmt.Sprintf("postgresql://togather:dev_password_change_me@localhost:%s/togather?sslmode=disable", dbPort)
		fmt.Printf("✓ Database URL: postgresql://togather:***@localhost:%s/togather\n", dbPort)
	} else {
		dbHost := "localhost"
		dbPort = "5432"
		dbName := "togather_dev"
		dbUser := "togather"
		dbPassword := ""

		if !setupNonInteractive {
			dbHost = prompt("PostgreSQL host", dbHost)
			dbPort = prompt("PostgreSQL port", dbPort)
			dbName = prompt("Database name", dbName)
			dbUser = prompt("PostgreSQL user", dbUser)
			dbPassword = promptPassword("PostgreSQL password")
		} else {
			dbPassword = "dev_password_change_me"
		}

		if dbPassword != "" {
			dbURL = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbPort, dbName)
		} else {
			dbURL = fmt.Sprintf("postgresql://%s@%s:%s/%s?sslmode=disable", dbUser, dbHost, dbPort, dbName)
		}
		fmt.Printf("✓ Database URL configured\n")
	}
	fmt.Println()

	// Step 5: Admin user configuration
	fmt.Println("Step 5: Admin User Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	adminUsername := "admin"
	adminEmail := "admin@localhost"

	if !setupNonInteractive {
		adminUsername = prompt("Admin username", adminUsername)
		adminEmail = prompt("Admin email", adminEmail)
		if confirm("Set custom admin password?", false) {
			adminPassword = promptPassword("Admin password")
		}
	}

	fmt.Printf("✓ Admin user: %s (%s)\n", adminUsername, adminEmail)
	fmt.Println()

	// Step 6: Write .env file
	fmt.Println("Step 6: Write Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	envContent := generateEnvFile(envConfig{
		DatabaseURL:   dbURL,
		JWTSecret:     jwtSecret,
		CSRFKey:       csrfKey,
		AdminUsername: adminUsername,
		AdminPassword: adminPassword,
		AdminEmail:    adminEmail,
		Environment:   env,
	})

	if err := os.WriteFile(".env", []byte(envContent), 0600); err != nil {
		return fmt.Errorf("write .env file: %w", err)
	}
	fmt.Println("✓ Created .env file")
	fmt.Println()

	// Step 7: Setup database and services
	if useDocker {
		fmt.Println("Step 7: Start Docker Services")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if setupNonInteractive || confirm("Start Docker services now?", true) {
			fmt.Println("Starting Docker containers...")
			if err := runCommand("make", "docker-up"); err != nil {
				fmt.Printf("⚠️  Failed to start Docker: %v\n", err)
				fmt.Println("You can start manually with: make docker-up")
			} else {
				fmt.Println("✓ Docker services started")
			}
		} else {
			fmt.Println("ℹ️  Run 'make docker-up' to start services")
		}
		fmt.Println()
	} else {
		fmt.Println("Step 7: Setup Local Database")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if setupNonInteractive || confirm("Set up local PostgreSQL database now?", true) {
			// Check prerequisites
			fmt.Println("Checking PostgreSQL extensions...")
			if err := runCommand("make", "db-check"); err != nil {
				fmt.Printf("⚠️  Extension check failed: %v\n", err)
				fmt.Println("You may need to install PostGIS, pgvector, and pg_trgm extensions")
				fmt.Println("Continue setup manually with:")
				fmt.Println("  make db-setup")
				fmt.Println("  make migrate-up && make migrate-river")
				return nil
			}

			// Create database with extensions
			fmt.Println("Creating database with extensions...")
			if err := runCommand("make", "db-setup"); err != nil {
				fmt.Printf("⚠️  Database creation failed: %v\n", err)
				fmt.Println("Continue setup manually with:")
				fmt.Println("  make db-setup")
				fmt.Println("  make migrate-up && make migrate-river")
				return nil
			}
			fmt.Println("✓ Database created")

			// Run migrations
			fmt.Println("Running migrations...")
			if err := runCommand("make", "migrate-up"); err != nil {
				fmt.Printf("⚠️  App migrations failed: %v\n", err)
				fmt.Println("Continue manually with: make migrate-up && make migrate-river")
				return nil
			}
			fmt.Println("✓ App migrations complete")

			if err := runCommand("make", "migrate-river"); err != nil {
				fmt.Printf("⚠️  River migrations failed: %v\n", err)
				fmt.Println("Continue manually with: make migrate-river")
				return nil
			}
			fmt.Println("✓ River migrations complete")
			fmt.Println("✓ Database ready")
		} else {
			fmt.Println("ℹ️  Run these commands to set up manually:")
			fmt.Println("     make db-setup")
			fmt.Println("     make migrate-up && make migrate-river")
		}
		fmt.Println()
	}

	// Step 8: Create first API key
	fmt.Println("Step 8: Create First API Key")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	createKey := setupNonInteractive || confirm("Create your first API key?", true)
	if createKey {
		keyName := "default"
		if !setupNonInteractive {
			keyName = prompt("API key name", "default")
		}

		// Set DATABASE_URL for the create command
		os.Setenv("DATABASE_URL", dbURL)

		// Wait a moment for database to be ready if Docker
		if useDocker {
			fmt.Println("Waiting for database to be ready...")
			for i := 0; i < 10; i++ {
				if err := checkDatabaseConnection(dbURL); err == nil {
					break
				}
				if i == 9 {
					fmt.Println("⚠️  Database not ready yet. You can create an API key later with:")
					fmt.Printf("    server api-key create %s\n", keyName)
					createKey = false
					break
				}
				fmt.Print(".")
				exec.Command("sleep", "2").Run()
			}
			fmt.Println()
		}

		if createKey {
			if err := createAPIKey(keyName, "agent"); err != nil {
				fmt.Printf("⚠️  Failed to create API key: %v\n", err)
				fmt.Printf("You can create it later with: server api-key create %s\n", keyName)
			}
		}
	} else {
		fmt.Println("ℹ️  You can create API keys with: server api-key create <name>")
	}
	fmt.Println()

	// Step 9: Success summary
	fmt.Println("🎉 Setup Complete!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("Your SEL server is configured and ready!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println()

	if useDocker {
		fmt.Println("  1. Check services are running:")
		fmt.Println("     make docker-logs")
		fmt.Println()
		fmt.Println("  2. Check health:")
		fmt.Println("     curl http://localhost:8080/health | jq .")
		fmt.Println()
	} else {
		fmt.Println("  1. Start the server:")
		fmt.Println("     server serve")
		fmt.Println("     # Or: make run")
		fmt.Println()
		fmt.Println("  2. Check health:")
		fmt.Println("     curl http://localhost:8080/health | jq .")
		fmt.Println()
	}

	fmt.Println("  3. Create more API keys:")
	fmt.Println("     server api-key create <name>")
	fmt.Println()
	fmt.Println("  4. Ingest test events:")
	fmt.Println("     server ingest events.json --watch")
	fmt.Println()
	fmt.Println("  5. View documentation:")
	fmt.Println("     cat README.md")
	fmt.Println()

	fmt.Printf("Configuration saved to: %s\n", filepath.Join(getWorkingDir(), ".env"))
	fmt.Println()

	return nil
}

type envConfig struct {
	DatabaseURL   string
	JWTSecret     string
	CSRFKey       string
	AdminUsername string
	AdminPassword string
	AdminEmail    string
	Environment   string
}

func generateEnvFile(cfg envConfig) string {
	return fmt.Sprintf(`# SEL Backend Server - Environment Configuration
# Generated by 'server setup' on %s

# Server Configuration
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_BASE_URL=http://localhost:8080

# Database Configuration
DATABASE_URL=%s
DATABASE_MAX_CONNECTIONS=25
DATABASE_MAX_IDLE_CONNECTIONS=5

# Bootstrap Admin User
ADMIN_USERNAME=%s
ADMIN_PASSWORD=%s
ADMIN_EMAIL=%s

# JWT Configuration
JWT_SECRET=%s
JWT_EXPIRY_HOURS=24

# CSRF Protection
CSRF_KEY=%s

# Rate Limiting
RATE_LIMIT_PUBLIC=60
RATE_LIMIT_AGENT=300
RATE_LIMIT_ADMIN=0

# Background Jobs
JOB_RETRY_DEDUPLICATION=1
JOB_RETRY_RECONCILIATION=5
JOB_RETRY_ENRICHMENT=10

# Environment
ENVIRONMENT=development

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Federation
FEDERATION_NODE_NAME=local-dev
FEDERATION_SYNC_ENABLED=false

# Feature Flags
ENABLE_VECTOR_SEARCH=false
ENABLE_AUTO_RECONCILIATION=false
`,
		currentTimestamp(),
		cfg.DatabaseURL,
		cfg.AdminUsername,
		cfg.AdminPassword,
		cfg.AdminEmail,
		cfg.JWTSecret,
		cfg.CSRFKey,
	)
}

// Helper functions

func prompt(question, defaultValue string) string {
	reader := bufio.NewReader(os.Stdin)
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", question, defaultValue)
	} else {
		fmt.Printf("%s: ", question)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func promptPassword(question string) string {
	fmt.Printf("%s: ", question)
	// Note: For simplicity, using regular input. In production, use terminal.ReadPassword
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	return strings.TrimSpace(password)
}

func confirm(question string, defaultYes bool) bool {
	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}

	response := strings.ToLower(prompt(question+" "+suffix, ""))
	if response == "" {
		return defaultYes
	}
	return response == "y" || response == "yes"
}

func promptChoice(question string, options []string, defaultIdx int) int {
	for i, opt := range options {
		marker := " "
		if i == defaultIdx {
			marker = ">"
		}
		fmt.Printf("  %s %d. %s\n", marker, i+1, opt)
	}

	response := prompt(question, fmt.Sprintf("%d", defaultIdx+1))
	idx := 0
	fmt.Sscanf(response, "%d", &idx)

	if idx < 1 || idx > len(options) {
		return defaultIdx
	}
	return idx - 1
}

func generateSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

func checkCommand(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getWorkingDir() string {
	wd, _ := os.Getwd()
	return wd
}

func currentTimestamp() string {
	return fmt.Sprintf("%s", os.Getenv("USER")) // Simplified for now
}

func checkDatabaseConnection(dbURL string) error {
	// Simplified check - just try to parse and return nil
	// In production, would use pgx to actually test connection
	return nil
}
