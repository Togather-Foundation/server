package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Togather-Foundation/server/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run scripts/create-api-key.go <name> [role]")
		fmt.Println("  name: Name for the API key (e.g., 'test-agent', 'curl-test')")
		fmt.Println("  role: Optional role (agent or admin, defaults to agent)")
		os.Exit(1)
	}

	name := os.Args[1]
	role := "agent"
	if len(os.Args) > 2 {
		role = os.Args[2]
	}

	// Try to load .env file if DATABASE_URL not set
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		loadEnvFile()
		dbURL = os.Getenv("DATABASE_URL")
	}

	// Also try deploy/docker/.env for Docker users
	if dbURL == "" {
		loadEnvFile("deploy/docker/.env")
		dbURL = os.Getenv("DATABASE_URL")
	}

	if dbURL == "" {
		fmt.Println("Error: DATABASE_URL not found")
		fmt.Println("")
		fmt.Println("Tried loading from:")
		fmt.Println("  - Environment variable DATABASE_URL")
		fmt.Println("  - .env file in project root")
		fmt.Println("  - deploy/docker/.env")
		fmt.Println("")
		fmt.Println("Please set DATABASE_URL or create a .env file:")
		fmt.Println("  For Docker: DATABASE_URL=postgres://togather:dev_password_change_me@localhost:5433/togather?sslmode=disable")
		fmt.Println("  For local:  DATABASE_URL=postgres://togather:dev_password_change_me@localhost:5432/togather?sslmode=disable")
		os.Exit(1)
	}

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Generate API key
	key := ulid.Make().String() + "secret"
	prefix := key[:8]

	// Hash the key
	hash, err := auth.HashAPIKey(key)
	if err != nil {
		fmt.Printf("Error hashing API key: %v\n", err)
		os.Exit(1)
	}

	// Insert into database
	_, err = pool.Exec(ctx,
		`INSERT INTO api_keys (prefix, key_hash, hash_version, name, role, is_active)
		 VALUES ($1, $2, $3, $4, $5, true)`,
		prefix, hash, auth.HashVersionBcrypt, name, role,
	)
	if err != nil {
		fmt.Printf("Error inserting API key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ API key created successfully!\n\n")
	fmt.Printf("Name:   %s\n", name)
	fmt.Printf("Role:   %s\n", role)
	fmt.Printf("Key:    %s\n\n", key)
	fmt.Printf("Save this key - it cannot be retrieved later!\n\n")
	fmt.Printf("Usage:\n")
	fmt.Printf("  export API_KEY=%s\n", key)
	fmt.Printf("  curl -H \"Authorization: Bearer $API_KEY\" http://localhost:8080/api/v1/events\n")
}

// loadEnvFile loads environment variables from a .env file
// Silently ignores if file doesn't exist (not all setups use .env)
func loadEnvFile(paths ...string) {
	envPath := ".env"
	if len(paths) > 0 {
		envPath = paths[0]
	}

	file, err := os.Open(envPath)
	if err != nil {
		return // File doesn't exist, that's ok
	}
	defer func() { _ = file.Close() }()

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
			_ = os.Setenv(key, value)
		}
	}
}
