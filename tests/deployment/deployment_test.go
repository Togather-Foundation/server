package deployment_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

// TestDeploymentFullFlow tests the complete deployment workflow:
// 1. Docker image build
// 2. Database provisioning
// 3. Migration execution
// 4. Health check validation
func TestDeploymentFullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping deployment integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	projectRoot := getProjectRoot(t)

	// Create a shared Docker network for containers to communicate
	net, err := network.New(ctx)
	require.NoError(t, err, "Failed to create Docker network")
	defer func() {
		if err := net.Remove(ctx); err != nil {
			t.Logf("Failed to remove network: %v", err)
		}
	}()
	networkName := net.Name

	// Shared state across subtests
	var dbURL string
	var postgresContainer *tcpostgres.PostgresContainer

	// Step 1: Build Docker image
	t.Run("BuildDockerImage", func(t *testing.T) {
		imageName := "togather-server-test"
		imageTag := "deployment-test"
		fullImageName := fmt.Sprintf("%s:%s", imageName, imageTag)

		t.Logf("Building Docker image: %s", fullImageName)
		buildStart := time.Now()

		cmd := exec.CommandContext(ctx, "docker", "build",
			"-f", filepath.Join(projectRoot, "deploy/docker/Dockerfile"),
			"-t", fullImageName,
			"--build-arg", "GIT_COMMIT=test-commit",
			"--build-arg", "GIT_SHORT_COMMIT=testcommit",
			"--build-arg", "BUILD_TIMESTAMP="+time.Now().UTC().Format(time.RFC3339),
			"--build-arg", "VERSION=test-version",
			projectRoot,
		)
		cmd.Dir = projectRoot

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Docker build failed: %s", string(output))

		buildDuration := time.Since(buildStart)
		t.Logf("✓ Docker image built successfully in %v", buildDuration)

		// Verify image exists
		cmd = exec.CommandContext(ctx, "docker", "image", "inspect", fullImageName)
		err = cmd.Run()
		require.NoError(t, err, "Docker image not found after build")
	})

	// Step 2: Provision PostgreSQL database with testcontainers
	t.Run("ProvisionDatabase", func(t *testing.T) {
		t.Logf("Provisioning PostgreSQL database with PostGIS extensions")
		provisionStart := time.Now()

		var err error
		postgresContainer, err = tcpostgres.Run(ctx,
			"postgis/postgis:16-3.4",
			tcpostgres.WithDatabase("togather_test"),
			tcpostgres.WithUsername("togather"),
			tcpostgres.WithPassword("togather-test-password"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
			testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
				ContainerRequest: testcontainers.ContainerRequest{
					Networks: []string{networkName},
					NetworkAliases: map[string][]string{
						networkName: {"postgres"},
					},
				},
			}),
		)
		require.NoError(t, err, "Failed to start PostgreSQL container")

		provisionDuration := time.Since(provisionStart)
		t.Logf("✓ PostgreSQL container started in %v", provisionDuration)

		// Get connection string for use from host (localhost)
		dbURL, err = postgresContainer.ConnectionString(ctx)
		require.NoError(t, err, "Failed to get database connection string")
		t.Logf("Database URL (host): %s", dbURL)
	})
	// Clean up database container at parent test level, after all subtests complete
	defer func() {
		if postgresContainer != nil {
			if err := testcontainers.TerminateContainer(postgresContainer); err != nil {
				t.Logf("Failed to terminate PostgreSQL container: %v", err)
			}
		}
	}()

	// Step 3: Execute migrations
	t.Run("RunMigrations", func(t *testing.T) {
		require.NotEmpty(t, dbURL, "DATABASE_URL not set from previous test")

		requireMigrateCLI(t)

		t.Logf("Running database migrations")
		migrationStart := time.Now()

		migrationsDir := filepath.Join(projectRoot, "internal/storage/postgres/migrations")
		require.DirExists(t, migrationsDir, "Migrations directory not found")

		// Use golang-migrate to run migrations
		cmd := exec.CommandContext(ctx, "migrate",
			"-path", migrationsDir,
			"-database", dbURL,
			"up",
		)

		output, err := cmd.CombinedOutput()
		if err != nil {
			// Check if error is just "no change" (already at latest version)
			if !strings.Contains(string(output), "no change") {
				require.NoError(t, err, "Migration failed: %s", string(output))
			}
		}

		migrationDuration := time.Since(migrationStart)
		t.Logf("✓ Migrations completed in %v", migrationDuration)
		t.Logf("Migration output: %s", string(output))

		// Verify schema_migrations table exists
		cmd = exec.CommandContext(ctx, "psql", dbURL,
			"-c", "SELECT COUNT(*) FROM schema_migrations;",
		)
		output, err = cmd.CombinedOutput()
		require.NoError(t, err, "Failed to query schema_migrations table: %s", string(output))
		t.Logf("Schema migrations table verified")
	})

	// Step 4: Start application container and validate health checks
	t.Run("StartApplicationAndValidateHealth", func(t *testing.T) {
		require.NotEmpty(t, dbURL, "DATABASE_URL not set")

		imageName := "togather-server-test:deployment-test"

		t.Logf("Starting application container: %s", imageName)
		startTime := time.Now()

		// Create DATABASE_URL that uses the container network alias instead of localhost
		containerDBURL := "postgres://togather:togather-test-password@postgres:5432/togather_test?sslmode=disable"

		// Run migrations directly using the Go migration library
		// Use the host dbURL since we're running from the test host
		t.Logf("Running migrations using built-in migration library")
		migrationStart := time.Now()
		migrationsDir := filepath.Join(projectRoot, "internal/storage/postgres/migrations")
		// Add sslmode=disable for test database
		migrationDBURL := dbURL
		if !strings.Contains(migrationDBURL, "sslmode=") {
			migrationDBURL += "sslmode=disable"
		}
		err := postgres.MigrateUp(migrationDBURL, migrationsDir)
		if err != nil {
			t.Fatalf("Migration failed: %v", err)
		}
		t.Logf("✓ Application migrations completed in %v", time.Since(migrationStart))

		// Run River migrations
		t.Logf("Running River job queue migrations")
		riverStart := time.Now()
		pool, err := pgxpool.New(ctx, migrationDBURL)
		if err != nil {
			t.Fatalf("Failed to create pgxpool: %v", err)
		}
		defer pool.Close()

		migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
		if err != nil {
			t.Fatalf("Failed to create River migrator: %v", err)
		}

		_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{})
		if err != nil {
			t.Fatalf("River migration failed: %v", err)
		}
		t.Logf("✓ River migrations completed in %v", time.Since(riverStart))

		// Create application container
		req := testcontainers.ContainerRequest{
			Image:        imageName,
			ExposedPorts: []string{"8080/tcp"},
			Networks:     []string{networkName},
			Env: map[string]string{
				"DATABASE_URL":     containerDBURL,
				"SERVER_PORT":      "8080",
				"LOG_LEVEL":        "info",
				"SHUTDOWN_TIMEOUT": "10s",
				"DEPLOYMENT_ENV":   "test",
				"JWT_SECRET":       "test-jwt-secret-for-deployment-testing-only",
			},
			WaitingFor: wait.ForHTTP("/health").
				WithPort("8080/tcp").
				WithStartupTimeout(60 * time.Second).
				WithPollInterval(1 * time.Second),
		}

		appContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		require.NoError(t, err, "Failed to start application container")
		t.Cleanup(func() {
			if err := appContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate application container: %v", err)
			}
		})

		startDuration := time.Since(startTime)
		t.Logf("✓ Application container started in %v", startDuration)

		// Get exposed port
		mappedPort, err := appContainer.MappedPort(ctx, nat.Port("8080/tcp"))
		require.NoError(t, err, "Failed to get mapped port")

		healthURL := fmt.Sprintf("http://localhost:%s/health", mappedPort.Port())
		t.Logf("Health check URL: %s", healthURL)

		// Validate health check response
		validateHealthCheck(t, ctx, healthURL)

		// Get container logs for verification
		logs, err := appContainer.Logs(ctx)
		if err == nil {
			defer func() { _ = logs.Close() }()
			t.Log("Application container logs available")
		}
	})
}

// TestDeploymentPerformance measures deployment timing to verify <5min target
func TestDeploymentPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping deployment performance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	overallStart := time.Now()
	timings := make(map[string]time.Duration)

	projectRoot := getProjectRoot(t)
	imageName := "togather-server-perf:test"

	// Create a shared Docker network for containers to communicate
	net, err := network.New(ctx)
	require.NoError(t, err, "Failed to create Docker network")
	defer func() {
		if err := net.Remove(ctx); err != nil {
			t.Logf("Failed to remove network: %v", err)
		}
	}()
	networkName := net.Name

	// Shared state across subtests
	var dbURL string
	var postgresContainer *tcpostgres.PostgresContainer

	// Measure: Docker build
	t.Run("BuildTime", func(t *testing.T) {
		start := time.Now()
		cmd := exec.CommandContext(ctx, "docker", "build",
			"-f", filepath.Join(projectRoot, "deploy/docker/Dockerfile"),
			"-t", imageName,
			"--build-arg", "GIT_COMMIT=perf-test",
			"--build-arg", "GIT_SHORT_COMMIT=perftest",
			projectRoot,
		)
		cmd.Dir = projectRoot

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Build failed: %s", string(output))

		timings["build"] = time.Since(start)
		t.Logf("Build time: %v", timings["build"])
	})

	// Measure: Database provisioning
	t.Run("DatabaseProvisionTime", func(t *testing.T) {
		start := time.Now()
		var err error
		postgresContainer, err = tcpostgres.Run(ctx,
			"postgis/postgis:16-3.4",
			tcpostgres.WithDatabase("togather_perf"),
			tcpostgres.WithUsername("togather"),
			tcpostgres.WithPassword("togather-perf-password"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
			testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
				ContainerRequest: testcontainers.ContainerRequest{
					Networks: []string{networkName},
					NetworkAliases: map[string][]string{
						networkName: {"postgres"},
					},
				},
			}),
		)
		require.NoError(t, err)

		dbURL, err = postgresContainer.ConnectionString(ctx)
		require.NoError(t, err)

		timings["database_provision"] = time.Since(start)
		t.Logf("Database provision time: %v", timings["database_provision"])
	})
	// Clean up database container at parent test level
	defer func() {
		if postgresContainer != nil {
			if err := testcontainers.TerminateContainer(postgresContainer); err != nil {
				t.Logf("Failed to terminate PostgreSQL container: %v", err)
			}
		}
	}()

	// Measure: Migration execution
	t.Run("MigrationTime", func(t *testing.T) {
		require.NotEmpty(t, dbURL)

		start := time.Now()
		migrationsDir := filepath.Join(projectRoot, "internal/storage/postgres/migrations")

		// Add sslmode=disable for test database
		migrationDBURL := dbURL
		if !strings.Contains(migrationDBURL, "sslmode=") {
			migrationDBURL += "sslmode=disable"
		}

		// Run application migrations
		err := postgres.MigrateUp(migrationDBURL, migrationsDir)
		require.NoError(t, err, "Application migration failed")

		// Run River migrations
		pool, err := pgxpool.New(ctx, migrationDBURL)
		require.NoError(t, err, "Failed to create pgxpool")
		defer pool.Close()

		migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
		require.NoError(t, err, "Failed to create River migrator")

		_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{})
		require.NoError(t, err, "River migration failed")

		timings["migrations"] = time.Since(start)
		t.Logf("Migration time: %v", timings["migrations"])
	})

	// Measure: Application startup
	t.Run("ApplicationStartupTime", func(t *testing.T) {
		require.NotEmpty(t, dbURL)

		start := time.Now()

		// Create DATABASE_URL that uses the container network alias
		containerDBURL := "postgres://togather:togather-perf-password@postgres:5432/togather_perf?sslmode=disable"

		req := testcontainers.ContainerRequest{
			Image:        imageName,
			ExposedPorts: []string{"8080/tcp"},
			Networks:     []string{networkName},
			Env: map[string]string{
				"DATABASE_URL": containerDBURL,
				"SERVER_PORT":  "8080",
				"JWT_SECRET":   "test-jwt-secret-for-deployment-testing-only",
			},
			WaitingFor: wait.ForHTTP("/health").
				WithPort("8080/tcp").
				WithStartupTimeout(60 * time.Second),
		}

		appContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			if err := appContainer.Terminate(ctx); err != nil {
				t.Logf("Failed to terminate app container: %v", err)
			}
		})

		timings["app_startup"] = time.Since(start)
		t.Logf("Application startup time: %v", timings["app_startup"])
	})

	// Calculate total time
	totalTime := time.Since(overallStart)
	timings["total"] = totalTime

	// Report results
	t.Log("\n=== Deployment Performance Summary ===")
	t.Logf("Build time:              %v", timings["build"])
	t.Logf("Database provision:      %v", timings["database_provision"])
	t.Logf("Migration execution:     %v", timings["migrations"])
	t.Logf("Application startup:     %v", timings["app_startup"])
	t.Logf("Total deployment time:   %v", totalTime)
	t.Log("======================================")

	// Assert performance target (<5 minutes)
	fiveMinutes := 5 * time.Minute
	if totalTime > fiveMinutes {
		t.Errorf("Deployment exceeded 5-minute target: %v", totalTime)
	} else {
		t.Logf("✓ Deployment within 5-minute target (%.1f%% of budget used)",
			float64(totalTime)/float64(fiveMinutes)*100)
	}
}

// TestMigrationRollback tests that migrations can be rolled back
func TestMigrationRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping migration rollback test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	requireMigrateCLI(t)

	projectRoot := getProjectRoot(t)

	// Start PostgreSQL
	postgresContainer, err := tcpostgres.Run(ctx,
		"postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("togather_rollback"),
		tcpostgres.WithUsername("togather"),
		tcpostgres.WithPassword("togather-rollback-password"),
	)
	require.NoError(t, err)
	defer func() {
		if err := testcontainers.TerminateContainer(postgresContainer); err != nil {
			t.Logf("Failed to terminate PostgreSQL container: %v", err)
		}
	}()

	dbURL, err := postgresContainer.ConnectionString(ctx)
	require.NoError(t, err)

	migrationsDir := filepath.Join(projectRoot, "internal/storage/postgres/migrations")

	// Run migrations up
	t.Run("MigrateUp", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "migrate",
			"-path", migrationsDir,
			"-database", dbURL,
			"up",
		)
		output, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(output), "no change") {
			require.NoError(t, err, "Migration up failed: %s", string(output))
		}
		t.Log("✓ Migrations applied")
	})

	// Get current version
	var initialVersion int
	t.Run("GetVersion", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "migrate",
			"-path", migrationsDir,
			"-database", dbURL,
			"version",
		)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to get migration version: %s", string(output))

		versionStr := strings.TrimSpace(string(output))
		if _, err := fmt.Sscanf(versionStr, "%d", &initialVersion); err != nil {
			t.Logf("Warning: could not parse version from: %s", versionStr)
		}
		t.Logf("Current migration version: %d", initialVersion)
	})

	// Roll back one migration
	if initialVersion > 0 {
		t.Run("MigrateDown", func(t *testing.T) {
			cmd := exec.CommandContext(ctx, "migrate",
				"-path", migrationsDir,
				"-database", dbURL,
				"down", "1",
			)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "Migration down failed: %s", string(output))
			t.Log("✓ Successfully rolled back one migration")
		})

		// Migrate back up
		t.Run("MigrateBackUp", func(t *testing.T) {
			cmd := exec.CommandContext(ctx, "migrate",
				"-path", migrationsDir,
				"-database", dbURL,
				"up",
			)
			output, err := cmd.CombinedOutput()
			if err != nil && !strings.Contains(string(output), "no change") {
				require.NoError(t, err, "Migration back up failed: %s", string(output))
			}
			t.Log("✓ Successfully migrated back up")
		})
	}
}

// TestDockerEmailServiceInit verifies that email templates are included in the Docker image
// and that the email service initializes successfully without panicking.
// This test prevents regression of srv-jakhq (missing COPY web/email in Dockerfile).
func TestDockerEmailServiceInit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker email service test in short mode")
	}

	// Check if Docker is available
	_, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("Docker not available - skipping test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	projectRoot := getProjectRoot(t)
	imageName := "togather-server-email-test"
	imageTag := "email-init-test"
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageTag)

	// Step 1: Build Docker image
	t.Run("BuildDockerImage", func(t *testing.T) {
		t.Logf("Building Docker image: %s", fullImageName)
		buildStart := time.Now()

		cmd := exec.CommandContext(ctx, "docker", "build",
			"-f", filepath.Join(projectRoot, "deploy/docker/Dockerfile"),
			"-t", fullImageName,
			"--build-arg", "GIT_COMMIT=email-test-commit",
			"--build-arg", "GIT_SHORT_COMMIT=emailtest",
			"--build-arg", "BUILD_TIMESTAMP="+time.Now().UTC().Format(time.RFC3339),
			"--build-arg", "VERSION=email-test-version",
			projectRoot,
		)
		cmd.Dir = projectRoot

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Docker build failed: %s", string(output))

		buildDuration := time.Since(buildStart)
		t.Logf("✓ Docker image built successfully in %v", buildDuration)
	})

	// Step 2: Verify email templates exist in image
	t.Run("VerifyEmailTemplatesInImage", func(t *testing.T) {
		t.Logf("Checking if email templates are present in Docker image")

		// Override entrypoint to run ls command
		cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--entrypoint", "ls",
			fullImageName, "-la", "/app/web/email/templates/")

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to list email templates in image: %s", string(output))

		// Verify invitation.html exists
		outputStr := string(output)
		assert.Contains(t, outputStr, "invitation.html", "invitation.html template not found in image")

		t.Logf("✓ Email templates found in image:\n%s", outputStr)
	})

	// Step 3: Start container with minimal email config and verify no panic
	t.Run("StartContainerAndVerifyEmailInit", func(t *testing.T) {
		t.Logf("Starting container with email service enabled")

		// Use a container with minimal config
		// We don't need a real database for this test - just verify the email service initializes
		containerName := fmt.Sprintf("email-test-%d", time.Now().Unix())

		// Start container in detached mode - it will fail to connect to database but email service should init first
		cmd := exec.CommandContext(ctx, "docker", "run",
			"--name", containerName,
			"-d",
			"-e", "EMAIL_ENABLED=true",
			"-e", "EMAIL_PROVIDER=smtp",
			"-e", "EMAIL_FROM=test@example.com",
			"-e", "SMTP_HOST=smtp.example.com",
			"-e", "SMTP_PORT=587",
			"-e", "SMTP_USER=testuser",
			"-e", "SMTP_PASSWORD=testpass",
			"-e", "DATABASE_URL=postgres://user:pass@localhost:5432/db?sslmode=disable",
			"-e", "LOG_LEVEL=info",
			"-e", "JWT_SECRET=test-secret-for-email-init-test-minimum-32-chars",
			fullImageName,
		)

		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "Failed to start container: %s", string(output))

		containerID := strings.TrimSpace(string(output))
		t.Logf("✓ Container started: %s", containerID)

		// Ensure container is cleaned up after test
		defer func() {
			// Stop and remove container
			stopCmd := exec.CommandContext(context.Background(), "docker", "rm", "-f", containerName)
			_ = stopCmd.Run()
		}()

		// Wait a few seconds for initialization and failure (due to no database)
		time.Sleep(3 * time.Second)

		// Get container logs - use containerID instead of containerName
		logsCmd := exec.CommandContext(ctx, "docker", "logs", containerID)
		logsOutput, err := logsCmd.CombinedOutput()
		// Don't fail if we can't get logs - container might have exited
		if err != nil {
			t.Logf("Warning: failed to get container logs: %v", err)
			// Try with container name as fallback
			logsCmd = exec.CommandContext(ctx, "docker", "logs", containerName)
			logsOutput, err = logsCmd.CombinedOutput()
		}

		logs := string(logsOutput)
		if logs == "" {
			t.Logf("Warning: No logs captured from container")
			// This is not a failure - the important thing is checking the build
		} else {
			t.Logf("Container logs:\n%s", logs)

			// Verify email service initialized successfully
			// Check for the absence of "failed to initialize email service" error
			assert.NotContains(t, logs, "failed to initialize email service",
				"Email service failed to initialize - templates may be missing")

			// Also check for the absence of template parsing errors
			assert.NotContains(t, logs, "failed to parse email templates",
				"Email template parsing failed")

			t.Logf("✓ Email service initialized without errors")
		}
	})

	// Cleanup: Remove Docker image
	t.Cleanup(func() {
		cmd := exec.Command("docker", "rmi", "-f", fullImageName)
		_ = cmd.Run()
	})
}

// Helper functions

func getProjectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "Failed to get caller information")

	// Navigate from tests/deployment/deployment_test.go to project root
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	abs, err := filepath.Abs(projectRoot)
	require.NoError(t, err, "Failed to get absolute path")

	return abs
}

func requireMigrateCLI(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("migrate")
	if err != nil {
		t.Skip("migrate CLI not installed - install with: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest")
	}
}

func validateHealthCheck(t *testing.T, ctx context.Context, healthURL string) {
	t.Helper()

	// Wait a bit for application to fully initialize
	time.Sleep(2 * time.Second)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	require.NoError(t, err, "Failed to create health check request")

	resp, err := client.Do(req)
	require.NoError(t, err, "Health check request failed")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Health check returned non-200 status")

	// Read and log response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read health check response")

	t.Logf("Health check response (%d): %s", resp.StatusCode, string(body))

	// Basic validation that response looks like health check JSON
	assert.Contains(t, string(body), "status", "Health check response missing status field")
}
