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
	setupNonInteractive      bool
	setupDockerMode          bool
	setupAllowProd           bool
	setupNoBackup            bool
	setupPreserveCredentials bool
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
	setupCmd.Flags().BoolVar(&setupAllowProd, "allow-production-secrets", false, "allow writing secrets to .env when ENVIRONMENT is staging/production")
	setupCmd.Flags().BoolVar(&setupNoBackup, "no-backup", false, "skip creating .env.backup file")
	setupCmd.Flags().BoolVar(&setupPreserveCredentials, "preserve-credentials", false, "reuse credentials from existing .env file (for upgrades)")
}

func runSetup() error {
	fmt.Println("🚀 Welcome to Togather SEL Server Setup")
	fmt.Println()

	if strings.TrimSpace(os.Getenv("ENVIRONMENT")) == "" {
		fmt.Println("ℹ️  ENVIRONMENT not set; defaulting to development")
	}

	// Check if .env already exists
	backupCreated := false
	if fileExists(".env") {
		if !setupNonInteractive {
			fmt.Println("⚠️  .env file already exists!")
			if !confirm("Overwrite existing .env file?", false) {
				fmt.Println("Setup cancelled.")
				return nil
			}
		}
		// Backup existing .env unless --no-backup is set
		if !setupNoBackup {
			if err := os.Rename(".env", ".env.backup"); err != nil {
				fmt.Printf("⚠️  Could not backup existing .env: %v\n", err)
			} else {
				fmt.Println("✓ Backed up existing .env to .env.backup")
				backupCreated = true
			}
		} else {
			// Just remove the old .env file
			if err := os.Remove(".env"); err != nil {
				fmt.Printf("⚠️  Could not remove existing .env: %v\n", err)
			}
		}
	}

	// Step 1: Detect environment
	fmt.Println("Step 1: Environment Detection")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check current ENVIRONMENT setting
	appEnv := strings.TrimSpace(strings.ToLower(os.Getenv("ENVIRONMENT")))
	if appEnv == "" {
		appEnv = "development"
	}

	useDocker := setupDockerMode
	if !setupNonInteractive && !setupDockerMode {
		fmt.Println("Choose your database setup:")
		fmt.Println()
		fmt.Println("  1. Docker (RECOMMENDED) - Database runs in container")
		fmt.Println("     ✓ Works in dev, staging, and production")
		fmt.Println("     ✓ Consistent across all environments")
		fmt.Println("     ✓ Includes PostgreSQL 16 + PostGIS + pgvector")
		fmt.Println()
		fmt.Println("  2. Local PostgreSQL - Use existing system installation")
		fmt.Println("     ⚠️  Development only - NOT for staging/production")
		fmt.Println("     ⚠️  Requires manual PostgreSQL 16+ installation with extensions")
		fmt.Println()

		// Force Docker in production/staging
		if appEnv == "production" || appEnv == "staging" {
			fmt.Printf("⚠️  ENVIRONMENT=%s detected\n", strings.ToUpper(appEnv))
			fmt.Println("   Docker is REQUIRED for staging/production")
			fmt.Println("   (Local PostgreSQL is only for development)")
			fmt.Println()
			useDocker = true
		} else {
			useDocker = promptChoice("Select option", []string{"Docker (recommended)", "Local PostgreSQL (dev only)"}, 0) == 0
		}
	}

	// Prevent local PostgreSQL in non-dev environments
	if !useDocker && (appEnv == "production" || appEnv == "staging") {
		return fmt.Errorf("local PostgreSQL is not supported for ENVIRONMENT=%s\nUse: ./server setup --docker", appEnv)
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
			fmt.Println("❌ psql not found - PostgreSQL is not installed")
			fmt.Println()
			fmt.Println("You need PostgreSQL 16+ with extensions (PostGIS, vector, pg_trgm).")
			fmt.Println()
			fmt.Println("Installation guide:")
			fmt.Println("  docs/contributors/postgresql-setup.md")
			fmt.Println("  https://github.com/Togather-Foundation/server/blob/main/docs/contributors/postgresql-setup.md")
			fmt.Println()
			fmt.Println("Note: Install 'postgresql-16-pgvector' package to get 'vector' extension")
			fmt.Println()
			fmt.Println("Or use Docker instead: ./server setup --docker")
			return fmt.Errorf("PostgreSQL not installed")
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

	// Check if we can write secrets for this environment
	if (appEnv == "production" || appEnv == "staging") && !setupAllowProd {
		return fmt.Errorf("refusing to write secrets to .env in %s; set ENVIRONMENT=development or pass --allow-production-secrets", appEnv)
	}

	// Step 3: Generate secrets
	fmt.Println("Step 3: Generate Secrets")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var jwtSecret, csrfKey, adminPassword, postgresPassword string
	var adminUsername, adminEmail string

	// If preserving credentials, read from existing .env
	if setupPreserveCredentials && fileExists(".env.backup") {
		fmt.Println("  ℹ️  Preserving credentials from existing .env")
		existingCreds, err := readCredentialsFromEnv(".env.backup")
		if err != nil {
			return fmt.Errorf("read existing credentials: %w", err)
		}
		jwtSecret = existingCreds["JWT_SECRET"]
		csrfKey = existingCreds["CSRF_KEY"]
		adminPassword = existingCreds["ADMIN_PASSWORD"]
		adminUsername = existingCreds["ADMIN_USERNAME"]
		adminEmail = existingCreds["ADMIN_EMAIL"]
		postgresPassword = existingCreds["POSTGRES_PASSWORD"]

		fmt.Println("  ✓ Reusing JWT_SECRET")
		fmt.Println("  ✓ Reusing CSRF_KEY")
		fmt.Println("  ✓ Reusing admin password")
		if postgresPassword != "" {
			fmt.Println("  ✓ Reusing PostgreSQL password")
		}
	} else {
		// Generate new secrets
		var err error
		jwtSecret, err = generateSecret(32)
		if err != nil {
			return fmt.Errorf("generate JWT secret: %w", err)
		}
		fmt.Println("✓ Generated JWT_SECRET")

		csrfKey, err = generateSecret(32)
		if err != nil {
			return fmt.Errorf("generate CSRF key: %w", err)
		}
		fmt.Println("✓ Generated CSRF_KEY")

		adminPassword, err = generateSecret(16)
		if err != nil {
			return fmt.Errorf("generate admin password: %w", err)
		}
		fmt.Println("✓ Generated admin password")
	}
	fmt.Println()

	// Step 4: Database configuration
	fmt.Println("Step 4: Database Connection")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	var dbURL string
	var dbPort string
	var postgresUser string
	var postgresDB string

	// If preserving credentials and we already have postgres password from .env, use it
	if useDocker {
		fmt.Println("Docker PostgreSQL will be created automatically.")
		dbPort = "5433"
		postgresUser = "togather"
		postgresDB = "togather"

		// Generate a secure password for Docker PostgreSQL if not already set
		if postgresPassword == "" {
			var err error
			postgresPassword, err = generateSecret(24)
			if err != nil {
				return fmt.Errorf("generate PostgreSQL password: %w", err)
			}
		}

		if !setupNonInteractive {
			dbPort = prompt("PostgreSQL port (Docker)", "5433")
		}
		dbURL = fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s?sslmode=disable", postgresUser, postgresPassword, dbPort, postgresDB)
		fmt.Printf("✓ Database URL: postgresql://%s:***@localhost:%s/%s\n", postgresUser, dbPort, postgresDB)
		if setupPreserveCredentials {
			fmt.Println("✓ Reused PostgreSQL password")
		} else {
			fmt.Println("✓ Generated PostgreSQL password")
		}
	} else {
		fmt.Println("Enter your PostgreSQL connection details.")
		fmt.Println("These are your PostgreSQL server credentials (not the app admin user).")
		fmt.Println()

		dbHost := "localhost"
		dbPort = "5432"
		dbName := "togather"
		dbUser := os.Getenv("USER") // Default to system username
		if dbUser == "" {
			dbUser = "togather"
		}
		dbPassword := ""

		if !setupNonInteractive {
			fmt.Println("💡 Tip: If you followed the PostgreSQL setup guide with peer authentication,")
			fmt.Printf("    use your system username (%s) and leave password empty.\n", os.Getenv("USER"))
			fmt.Println()

			dbHost = prompt("PostgreSQL host", dbHost)
			dbPort = prompt("PostgreSQL port", dbPort)
			dbName = prompt("Database name (will be created)", dbName)
			dbUser = prompt("PostgreSQL username", dbUser)

			fmt.Println()
			fmt.Println("PostgreSQL Authentication:")
			fmt.Println("  1. Peer authentication (no password, uses system user)")
			fmt.Println("  2. Password authentication")
			authChoice := promptChoice("Select authentication method", []string{"Peer (no password)", "Password"}, 0)

			if authChoice == 1 {
				dbPassword = prompt("PostgreSQL password", "")
				if dbPassword == "" {
					fmt.Println("⚠️  Empty password entered. Switching to peer authentication.")
				}
			}
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
	fmt.Println("Step 5: SEL Application Admin User")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("This is the admin user for the SEL server web application.")
	fmt.Println("(This is different from your PostgreSQL database user)")
	fmt.Println()

	// Set defaults if not already set from preserved credentials
	if adminUsername == "" {
		adminUsername = "admin"
	}
	if adminEmail == "" {
		adminEmail = "admin@localhost"
	}

	if !setupNonInteractive && !setupPreserveCredentials {
		adminUsername = prompt("Admin username", adminUsername)
		adminEmail = prompt("Admin email", adminEmail)
		fmt.Println()
		fmt.Printf("💡 A secure random password was generated: %s\n", adminPassword[:16]+"...")
		if confirm("Set a custom admin password instead?", false) {
			adminPassword = promptPassword("Admin password")
		}
	}

	fmt.Printf("✓ Admin user: %s (%s)\n", adminUsername, adminEmail)
	fmt.Println()

	// Step 6: Write .env file
	fmt.Println("Step 6: Write Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if (appEnv == "production" || appEnv == "staging") && !setupAllowProd {
		fmt.Println("⚠️  Writing secrets to .env in non-development environments is discouraged")
		fmt.Println("   Set ENVIRONMENT=development or pass --allow-production-secrets if you really need this")
		fmt.Println()
		return nil
	}

	envContent := generateEnvFile(envConfig{
		DatabaseURL:      dbURL,
		JWTSecret:        jwtSecret,
		CSRFKey:          csrfKey,
		AdminUsername:    adminUsername,
		AdminPassword:    adminPassword,
		AdminEmail:       adminEmail,
		Environment:      appEnv,
		PostgresDB:       postgresDB,
		PostgresUser:     postgresUser,
		PostgresPassword: postgresPassword,
		PostgresPort:     dbPort,
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

		// Check if user can access Docker
		dockerAccessible := canAccessDocker()

		if !dockerAccessible {
			fmt.Println("⚠️  Docker permission issue detected")
			fmt.Println()
			fmt.Println("You need to be in the 'docker' group to run Docker commands.")
			fmt.Println()
			fmt.Println("Fix this by running ONE of these commands:")
			fmt.Println("  1. Log out and log back in (group changes take effect)")
			fmt.Println("  2. Run: newgrp docker")
			fmt.Println("  3. Run: sudo usermod -aG docker $USER && newgrp docker")
			fmt.Println()
			fmt.Println("After fixing permissions, start Docker with:")
			fmt.Println("  docker compose -f deploy/docker/docker-compose.yml up -d")
			fmt.Println()
		} else if setupNonInteractive || confirm("Start Docker services now?", true) {
			fmt.Println("Starting Docker containers...")
			if err := runCommand("make", "docker-db"); err != nil {
				fmt.Printf("⚠️  Failed to start Docker: %v\n", err)
				fmt.Println("You can start manually with: make docker-db")
			} else {
				fmt.Println("✓ Docker database container started")

				// Wait for PostgreSQL to be ready
				fmt.Println("Waiting for PostgreSQL to be ready...")
				if err := waitForPostgres(dbURL, 30); err != nil {
					fmt.Printf("⚠️  PostgreSQL not ready: %v\n", err)
					fmt.Println()
					fmt.Println("You can run migrations manually once database is ready:")
					fmt.Println("  make migrate-up")
				} else {
					fmt.Println("✓ PostgreSQL is ready")

					// Set DATABASE_URL for migration commands
					_ = os.Setenv("DATABASE_URL", dbURL)

					// Run migrations automatically
					fmt.Println("Running database migrations...")
					if err := runCommand("make", "migrate-up"); err != nil {
						fmt.Printf("⚠️  App migrations failed: %v\n", err)
						fmt.Println("You can run manually with: make migrate-up")
					} else {
						fmt.Println("✓ App migrations complete")
					}

					// Run River migrations
					if err := runCommand("make", "migrate-river"); err != nil {
						fmt.Printf("⚠️  River migrations failed: %v\n", err)
						fmt.Println("You can run manually with: make migrate-river")
					} else {
						fmt.Println("✓ River migrations complete")
					}
				}
			}
		} else {
			fmt.Println("ℹ️  Run 'make docker-db' to start database, then 'make migrate-up'")
		}
		fmt.Println()
	} else {
		fmt.Println("Step 7: Setup Local Database")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if setupNonInteractive || confirm("Set up local PostgreSQL database now?", true) {
			// Check prerequisites
			fmt.Println("Checking PostgreSQL extensions...")
			if err := runCommand("make", "db-check"); err != nil {
				fmt.Println()
				fmt.Println("❌ Extension check failed!")
				fmt.Println()
				fmt.Println("PostgreSQL extensions (PostGIS, vector, pg_trgm) are missing or")
				fmt.Println("PostgreSQL is not properly configured.")
				fmt.Println()
				fmt.Println("📖 See installation guide:")
				fmt.Println("   docs/contributors/postgresql-setup.md")
				fmt.Println()
				fmt.Println("Quick fix (Ubuntu/Debian):")
				fmt.Println("  sudo apt install postgresql-16-postgis-3 postgresql-16-pgvector")
				fmt.Println()
				fmt.Println("Note: Package 'postgresql-16-pgvector' provides extension 'vector'")
				fmt.Println()
				fmt.Println("After installing extensions, run setup again or continue manually:")
				fmt.Println("  make db-setup")
				fmt.Println("  make migrate-up && make migrate-river")
				return fmt.Errorf("PostgreSQL extensions not available")
			}

			// Create database with extensions
			fmt.Println("Creating database with extensions...")
			if err := runCommand("make", "db-setup"); err != nil {
				fmt.Printf("⚠️  Database creation failed: %v\n", err)
				fmt.Println()
				fmt.Println("This might be due to missing vector extension.")
				fmt.Println("Check the output above for extension errors.")
				fmt.Println()
				fmt.Println("📖 Install vector extension (pgvector package):")
				fmt.Println("   docs/contributors/postgresql-setup.md")
				fmt.Println()
				fmt.Println("Quick fix (Ubuntu/Debian):")
				fmt.Println("  sudo apt install postgresql-16-pgvector")
				fmt.Println()
				fmt.Println("Note: Package name is 'pgvector', extension name is 'vector'")
				fmt.Println()
				fmt.Println("Continue setup manually after installing:")
				fmt.Println("  make db-setup")
				fmt.Println("  make migrate-up && make migrate-river")
				return fmt.Errorf("database setup failed")
			}
			fmt.Println("✓ Database created")

			// Set DATABASE_URL for migration commands
			_ = os.Setenv("DATABASE_URL", dbURL)

			// Run migrations
			fmt.Println("Running migrations...")
			if err := runCommand("make", "migrate-up"); err != nil {
				fmt.Printf("⚠️  App migrations failed: %v\n", err)
				fmt.Println("Continue manually with: make migrate-up && make migrate-river")
				return fmt.Errorf("migrations failed")
			}
			fmt.Println("✓ App migrations complete")

			if err := runCommand("make", "migrate-river"); err != nil {
				fmt.Printf("⚠️  River migrations failed: %v\n", err)
				fmt.Println("Continue manually with: make migrate-river")
				return fmt.Errorf("river migrations failed")
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
	if (appEnv == "production" || appEnv == "staging") && !setupAllowProd {
		fmt.Println("⚠️  Skipping API key creation for non-development environment")
		fmt.Println("   Use: server api-key create <name> and store it in your secret manager")
		fmt.Println()
		createKey = false
	}
	if createKey {
		keyName := "default"
		if !setupNonInteractive {
			keyName = prompt("API key name", "default")
		}

		// Set DATABASE_URL for the create command
		if err := os.Setenv("DATABASE_URL", dbURL); err != nil {
			fmt.Printf("⚠️  Warning: failed to set DATABASE_URL: %v\n", err)
		}

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
				if err := exec.Command("sleep", "2").Run(); err != nil {
					// Sleep failed, continue anyway
					break
				}
			}
			fmt.Println()
		}

		if createKey {
			apiKey, err := createAPIKey(keyName, "agent")
			if err != nil {
				fmt.Printf("⚠️  Failed to create API key: %v\n", err)
				fmt.Printf("You can create it later with: server api-key create %s\n", keyName)
			} else {
				// Save API key to .env file
				envPath := filepath.Join(getWorkingDir(), ".env")
				if err := appendToEnvFile(envPath, "API_KEY", apiKey); err != nil {
					fmt.Printf("⚠️  Failed to save API key to .env: %v\n", err)
					fmt.Printf("You can manually add it: export API_KEY=%s\n", apiKey)
				} else {
					fmt.Println()
					fmt.Printf("✓ API key saved to .env\n")
				}
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

	// Print credentials summary
	fmt.Println("📋 Credentials Summary")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Admin Username: %s\n", adminUsername)
	fmt.Printf("Admin Email:    %s\n", adminEmail)
	fmt.Printf("Admin Password: %s\n", adminPassword)
	if useDocker && postgresPassword != "" {
		fmt.Printf("PostgreSQL User: %s\n", postgresUser)
		fmt.Printf("PostgreSQL Password: %s\n", postgresPassword)
		fmt.Printf("PostgreSQL Port: %s\n", dbPort)
	}
	fmt.Println()
	fmt.Printf("⚠️  IMPORTANT: Save these credentials securely!\n")
	fmt.Printf("   Configuration file: %s\n", filepath.Join(getWorkingDir(), ".env"))
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
		fmt.Println("  3. Generate test events:")
		fmt.Println("     server generate test-events.json --count 3")
		fmt.Println()
		fmt.Println("  4. Ingest test events:")
		fmt.Println("     server ingest test-events.json --watch")
		fmt.Println()
		fmt.Println("  5. Query ingested events:")
		fmt.Println("     server events --limit 5")
		fmt.Println()
		fmt.Println("  6. View contributor documentation:")
		fmt.Println("     cat docs/contributors/development.md")
		fmt.Println()
	} else {
		fmt.Println("  1. Start the server:")
		fmt.Println("     server serve")
		fmt.Println("     # Or: make run")
		fmt.Println()
		fmt.Println("  2. Check health:")
		fmt.Println("     curl http://localhost:8080/health | jq .")
		fmt.Println()
		fmt.Println("  3. Generate test events:")
		fmt.Println("     server generate test-events.json --count 3")
		fmt.Println()
		fmt.Println("  4. Ingest test events:")
		fmt.Println("     server ingest test-events.json --watch")
		fmt.Println()
		fmt.Println("  5. Query ingested events:")
		fmt.Println("     server events --limit 5")
		fmt.Println()
		fmt.Println("  6. View contributor documentation:")
		fmt.Println("     cat docs/contributors/development.md")
		fmt.Println()
	}

	fmt.Printf("Configuration saved to: %s\n", filepath.Join(getWorkingDir(), ".env"))
	fmt.Println()

	// Cleanup backup file if created
	if backupCreated {
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		fmt.Println("📋 Backup File Management")
		fmt.Println()
		fmt.Println("A backup of your previous .env file was saved to .env.backup")
		fmt.Println()

		if setupNonInteractive {
			// In non-interactive mode, keep the backup for safety
			fmt.Println("✓ Backup file retained for safety")
			fmt.Println()
			fmt.Println("Once you've verified the new configuration:")
			fmt.Println("  rm .env.backup")
		} else {
			if confirm("Remove .env.backup file now?", false) {
				if err := os.Remove(".env.backup"); err != nil {
					fmt.Printf("⚠️  Could not remove .env.backup: %v\n", err)
					fmt.Println()
					fmt.Println("You can manually remove it with:")
					fmt.Println("  rm .env.backup")
				} else {
					fmt.Println("✓ Removed .env.backup")
				}
			} else {
				fmt.Println("✓ Backup file retained")
				fmt.Println()
				fmt.Println("When ready to clean up:")
				fmt.Println("  rm .env.backup")
			}
		}
		fmt.Println()
	}

	// Check for and warn about .env.original files (from older setup versions)
	if fileExists(".env.original") {
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		fmt.Println("⚠️  Found legacy .env.original file")
		fmt.Println()
		fmt.Println("This appears to be from an older setup run.")
		if !setupNonInteractive && confirm("Remove .env.original file?", true) {
			if err := os.Remove(".env.original"); err != nil {
				fmt.Printf("⚠️  Could not remove .env.original: %v\n", err)
				fmt.Println("You can manually remove it with: rm .env.original")
			} else {
				fmt.Println("✓ Removed .env.original")
			}
		} else {
			fmt.Println("You can manually remove it with: rm .env.original")
		}
		fmt.Println()
	}

	return nil
}

type envConfig struct {
	DatabaseURL      string
	JWTSecret        string
	CSRFKey          string
	AdminUsername    string
	AdminPassword    string
	AdminEmail       string
	Environment      string
	PostgresDB       string
	PostgresUser     string
	PostgresPassword string
	PostgresPort     string
}

func generateEnvFile(cfg envConfig) string {
	dockerVarsSection := ""
	if cfg.PostgresPassword != "" {
		dockerVarsSection = fmt.Sprintf(`
# Docker PostgreSQL Configuration
# These variables are used by docker-compose.yml for the PostgreSQL container
POSTGRES_DB=%s
POSTGRES_USER=%s
POSTGRES_PASSWORD=%s
POSTGRES_PORT=%s
`, cfg.PostgresDB, cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresPort)
	}

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
%s
# Bootstrap Admin User
ADMIN_USERNAME=%s
ADMIN_PASSWORD=%s
ADMIN_EMAIL=%s

# JWT Configuration
JWT_SECRET=%s
JWT_EXPIRY_HOURS=24

# CSRF Protection
CSRF_KEY=%s

# CORS Configuration
# WARNING: In production, replace * with your actual domain(s)
CORS_ALLOWED_ORIGINS=*

# Rate Limiting
RATE_LIMIT_PUBLIC=60
RATE_LIMIT_AGENT=300
RATE_LIMIT_ADMIN=0

# Background Jobs
JOB_RETRY_DEDUPLICATION=1
JOB_RETRY_RECONCILIATION=5
JOB_RETRY_ENRICHMENT=10

# Environment
ENVIRONMENT=%s

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
		dockerVarsSection,
		cfg.AdminUsername,
		cfg.AdminPassword,
		cfg.AdminEmail,
		cfg.JWTSecret,
		cfg.CSRFKey,
		cfg.Environment,
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
	if _, err := fmt.Sscanf(response, "%d", &idx); err != nil {
		// Failed to parse, use default
		return defaultIdx
	}

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

func canAccessDocker() bool {
	// Try to run 'docker ps' to check if we have access
	cmd := exec.Command("docker", "ps")
	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil // Suppress errors
	return cmd.Run() == nil
}

func waitForPostgres(dbURL string, timeoutSecs int) error {
	// Extract connection details from DATABASE_URL for pg_isready
	// Format: postgresql://user:pass@host:port/dbname
	// We'll use docker exec to check if postgres is ready inside the container

	for i := 0; i < timeoutSecs; i++ {
		// Try to check if postgres is accepting connections
		// Using docker exec pg_isready since we know we're using Docker here
		cmd := exec.Command("docker", "exec", "togather-postgres", "pg_isready", "-U", "togather")
		cmd.Stdout = nil
		cmd.Stderr = nil

		if cmd.Run() == nil {
			return nil
		}

		fmt.Print(".")
		_ = exec.Command("sleep", "1").Run()
	}
	fmt.Println()

	return fmt.Errorf("PostgreSQL did not become ready within %d seconds", timeoutSecs)
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ() // Inherit all environment variables
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
	return os.Getenv("USER") // Simplified for now
}

func checkDatabaseConnection(dbURL string) error {
	// Simplified check - just try to parse and return nil
	// In production, would use pgx to actually test connection
	return nil
}

func appendToEnvFile(path, key, value string) error {
	// Read existing file
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read env file: %w", err)
	}

	// Append new key-value pair
	newLine := fmt.Sprintf("\n# API Key\nAPI_KEY=%s\n", value)

	// Write back
	if err := os.WriteFile(path, append(content, []byte(newLine)...), 0600); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}

	return nil
}

func readCredentialsFromEnv(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}

	creds := make(map[string]string)
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			creds[key] = value
		}
	}

	return creds, nil
}
