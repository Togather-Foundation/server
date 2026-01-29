package main

import (
	"context"
	"fmt"
	"os"

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

	// Get DATABASE_URL from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Println("Error: DATABASE_URL environment variable not set")
		fmt.Println("For Docker: export DATABASE_URL=postgres://togather:dev_password_change_me@localhost:5433/togather?sslmode=disable")
		fmt.Println("For local:  export DATABASE_URL=postgres://togather:dev_password_change_me@localhost:5432/togather?sslmode=disable")
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
