package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Metadata contains information about a database snapshot
type Metadata struct {
	SnapshotName  string    `json:"snapshot_name"`
	Database      string    `json:"database"`
	Host          string    `json:"host"`
	Port          string    `json:"port"`
	Timestamp     time.Time `json:"timestamp"`
	Reason        string    `json:"reason"`
	GitCommit     string    `json:"git_commit,omitempty"`
	DeploymentID  string    `json:"deployment_id,omitempty"`
	RetentionDays int       `json:"retention_days"`
	ExpiresAt     time.Time `json:"expires_at"`
	SizeMB        int       `json:"size_mb,omitempty"`
	DurationSecs  int       `json:"duration_seconds,omitempty"`
}

// Snapshot represents a database snapshot file with its metadata
type Snapshot struct {
	Path     string
	Metadata Metadata
	SizeMB   int
	Age      time.Duration
}

// CreateOptions contains options for creating a snapshot
type CreateOptions struct {
	DatabaseURL   string
	Database      string
	Host          string
	Port          string
	User          string
	Password      string
	Reason        string
	RetentionDays int
	SnapshotDir   string
	GitCommit     string
	DeploymentID  string
	Validate      bool
}

// Config contains snapshot configuration
type Config struct {
	SnapshotDir   string
	RetentionDays int
}

// DefaultConfig returns the default snapshot configuration
func DefaultConfig() Config {
	return Config{
		SnapshotDir:   getDefaultSnapshotDir(),
		RetentionDays: 7,
	}
}

func getDefaultSnapshotDir() string {
	// Try standard locations in order of preference
	candidates := []string{
		"./snapshots",
		"/var/lib/togather/db-snapshots",
		"/var/backups/togather",
	}

	for _, dir := range candidates {
		if absDir, err := filepath.Abs(dir); err == nil {
			// Return first path that either exists or can be created
			if info, err := os.Stat(absDir); err == nil && info.IsDir() {
				return absDir
			}
			// If it's ./snapshots (relative), prefer it as default even if doesn't exist yet
			if strings.HasPrefix(dir, "./") {
				return absDir
			}
		}
	}

	// Fallback to ./snapshots
	absDir, _ := filepath.Abs("./snapshots")
	return absDir
}

// Create creates a new database snapshot
func Create(ctx context.Context, opts CreateOptions) (*Snapshot, error) {
	// Validate required options
	if opts.Database == "" {
		return nil, fmt.Errorf("database name is required")
	}
	if opts.Host == "" {
		return nil, fmt.Errorf("database host is required")
	}
	if opts.Port == "" {
		return nil, fmt.Errorf("database port is required")
	}
	if opts.User == "" {
		return nil, fmt.Errorf("database user is required")
	}
	if opts.Password == "" {
		return nil, fmt.Errorf("database password is required")
	}
	if opts.SnapshotDir == "" {
		return nil, fmt.Errorf("snapshot directory is required")
	}
	if opts.RetentionDays <= 0 {
		opts.RetentionDays = 7
	}
	if opts.Reason == "" {
		opts.Reason = "manual"
	}

	// Ensure snapshot directory exists
	if err := os.MkdirAll(opts.SnapshotDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Check if pg_dump is available
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return nil, fmt.Errorf("pg_dump not found in PATH (install postgresql-client): %w", err)
	}

	// Generate snapshot filename
	timestamp := time.Now().UTC().Format("20060102_150405")
	snapshotName := fmt.Sprintf("togather_%s_%s_%s.sql.gz", opts.Database, timestamp, opts.Reason)
	snapshotPath := filepath.Join(opts.SnapshotDir, snapshotName)
	metadataPath := strings.TrimSuffix(snapshotPath, ".sql.gz") + ".meta.json"

	// Create metadata
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(opts.RetentionDays) * 24 * time.Hour)
	metadata := Metadata{
		SnapshotName:  snapshotName,
		Database:      opts.Database,
		Host:          opts.Host,
		Port:          opts.Port,
		Timestamp:     now,
		Reason:        opts.Reason,
		GitCommit:     opts.GitCommit,
		DeploymentID:  opts.DeploymentID,
		RetentionDays: opts.RetentionDays,
		ExpiresAt:     expiresAt,
	}

	// Write metadata file
	if err := writeMetadata(metadataPath, metadata); err != nil {
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}

	// Create temporary file for pgpass
	pgpassFile, err := createPgpassFile(opts.Host, opts.Port, opts.Database, opts.User, opts.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgpass file: %w", err)
	}
	defer func() { _ = os.Remove(pgpassFile) }()

	// Run pg_dump with gzip compression
	startTime := time.Now()
	if err := runPgDump(ctx, pgpassFile, opts, snapshotPath); err != nil {
		// Clean up on failure
		_ = os.Remove(snapshotPath)
		return nil, fmt.Errorf("pg_dump failed: %w", err)
	}
	duration := time.Since(startTime)

	// Validate snapshot if requested
	if opts.Validate {
		if err := validateSnapshot(snapshotPath); err != nil {
			_ = os.Remove(snapshotPath)
			return nil, fmt.Errorf("snapshot validation failed: %w", err)
		}
	}

	// Get snapshot size
	fileInfo, err := os.Stat(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat snapshot file: %w", err)
	}
	sizeMB := int(fileInfo.Size() / (1024 * 1024))

	// Update metadata with size and duration
	metadata.SizeMB = sizeMB
	metadata.DurationSecs = int(duration.Seconds())
	if err := writeMetadata(metadataPath, metadata); err != nil {
		return nil, fmt.Errorf("failed to update metadata: %w", err)
	}

	return &Snapshot{
		Path:     snapshotPath,
		Metadata: metadata,
		SizeMB:   sizeMB,
		Age:      0,
	}, nil
}

// List returns all snapshots in the snapshot directory
func List(snapshotDir string) ([]Snapshot, error) {
	if snapshotDir == "" {
		return nil, fmt.Errorf("snapshot directory is required")
	}

	// Check if directory exists
	if _, err := os.Stat(snapshotDir); os.IsNotExist(err) {
		return []Snapshot{}, nil
	}

	// Find all .sql.gz files
	pattern := filepath.Join(snapshotDir, "*.sql.gz")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	snapshots := make([]Snapshot, 0, len(matches))

	for _, path := range matches {
		now := time.Now()
		// Get file info
		fileInfo, err := os.Stat(path)
		if err != nil {
			continue // Skip files we can't stat
		}

		sizeMB := int(fileInfo.Size() / (1024 * 1024))
		age := now.Sub(fileInfo.ModTime())

		// Try to load metadata
		metadataPath := strings.TrimSuffix(path, ".sql.gz") + ".meta.json"
		metadata, err := loadMetadata(metadataPath)
		if err != nil {
			// Create basic metadata from filename if metadata file doesn't exist
			metadata = Metadata{
				SnapshotName:  filepath.Base(path),
				Timestamp:     fileInfo.ModTime(),
				Reason:        "unknown",
				RetentionDays: 7,
			}
		}

		snapshots = append(snapshots, Snapshot{
			Path:     path,
			Metadata: metadata,
			SizeMB:   sizeMB,
			Age:      age,
		})
	}

	// Sort by timestamp (newest first)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Metadata.Timestamp.After(snapshots[j].Metadata.Timestamp)
	})

	return snapshots, nil
}

// Cleanup removes snapshots older than retention period
func Cleanup(snapshotDir string, retentionDays int, dryRun bool) ([]Snapshot, error) {
	if snapshotDir == "" {
		return nil, fmt.Errorf("snapshot directory is required")
	}
	if retentionDays <= 0 {
		return nil, fmt.Errorf("retention days must be positive")
	}

	snapshots, err := List(snapshotDir)
	if err != nil {
		return nil, err
	}

	deleted := []Snapshot{}

	for _, snapshot := range snapshots {
		retentionPeriod := time.Duration(snapshot.Metadata.RetentionDays) * 24 * time.Hour
		if snapshot.Metadata.RetentionDays == 0 {
			retentionPeriod = time.Duration(retentionDays) * 24 * time.Hour
		}

		if snapshot.Age > retentionPeriod {
			if !dryRun {
				// Delete snapshot file
				if err := os.Remove(snapshot.Path); err != nil {
					return deleted, fmt.Errorf("failed to delete %s: %w", snapshot.Path, err)
				}

				// Delete metadata file
				metadataPath := strings.TrimSuffix(snapshot.Path, ".sql.gz") + ".meta.json"
				os.Remove(metadataPath) // Ignore error if metadata doesn't exist
			}

			deleted = append(deleted, snapshot)
		}
	}

	return deleted, nil
}

// Helper functions

// ParseDatabaseURL parses a PostgreSQL connection string
// Format: postgresql://user:password@host:port/database?params
func ParseDatabaseURL(dbURL string) (host, port, database, user, password string, err error) {
	if !strings.HasPrefix(dbURL, "postgresql://") && !strings.HasPrefix(dbURL, "postgres://") {
		return "", "", "", "", "", fmt.Errorf("invalid DATABASE_URL format (must start with postgresql:// or postgres://)")
	}

	// Remove protocol
	url := strings.TrimPrefix(dbURL, "postgresql://")
	url = strings.TrimPrefix(url, "postgres://")

	// Split user:pass@rest
	parts := strings.SplitN(url, "@", 2)
	if len(parts) != 2 {
		return "", "", "", "", "", fmt.Errorf("invalid DATABASE_URL format (missing @ separator)")
	}

	// Parse user:password
	userPass := parts[0]
	if strings.Contains(userPass, ":") {
		userParts := strings.SplitN(userPass, ":", 2)
		user = userParts[0]
		password = userParts[1]
	} else {
		user = userPass
	}

	// Parse host:port/database?params
	rest := parts[1]

	// Remove query params if present
	if idx := strings.Index(rest, "?"); idx >= 0 {
		rest = rest[:idx]
	}

	// Split host:port and database
	parts = strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", "", "", "", "", fmt.Errorf("invalid DATABASE_URL format (missing database name)")
	}

	hostPort := parts[0]
	database = parts[1]

	// Parse host and port
	if strings.Contains(hostPort, ":") {
		hostParts := strings.SplitN(hostPort, ":", 2)
		host = hostParts[0]
		port = hostParts[1]
	} else {
		host = hostPort
		port = "5432" // Default PostgreSQL port
	}

	return host, port, database, user, password, nil
}

func createPgpassFile(host, port, database, user, password string) (string, error) {
	tmpFile, err := os.CreateTemp("", "pgpass-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = tmpFile.Close() }()

	// Set restrictive permissions
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}

	// Write pgpass entry: hostname:port:database:username:password
	line := fmt.Sprintf("%s:%s:%s:%s:%s\n", host, port, database, user, password)
	if _, err := tmpFile.WriteString(line); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func runPgDump(ctx context.Context, pgpassFile string, opts CreateOptions, outputPath string) error {
	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	// Run pg_dump
	pgDumpCmd := exec.CommandContext(ctx, "pg_dump",
		"-h", opts.Host,
		"-p", opts.Port,
		"-U", opts.User,
		"-d", opts.Database,
		"--format=plain",
		"--no-owner",
		"--no-acl",
		"--clean",
		"--if-exists",
	)
	pgDumpCmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSFILE=%s", pgpassFile))

	// Pipe pg_dump output to gzip
	pgDumpOut, err := pgDumpCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	gzipCmd := exec.CommandContext(ctx, "gzip")
	gzipCmd.Stdin = pgDumpOut
	gzipCmd.Stdout = outFile

	// Capture stderr for both commands
	var pgDumpErr, gzipErr strings.Builder
	pgDumpCmd.Stderr = &pgDumpErr
	gzipCmd.Stderr = &gzipErr

	// Start both commands
	if err := gzipCmd.Start(); err != nil {
		return fmt.Errorf("failed to start gzip: %w", err)
	}
	if err := pgDumpCmd.Start(); err != nil {
		return fmt.Errorf("failed to start pg_dump: %w", err)
	}

	// Wait for pg_dump to complete
	if err := pgDumpCmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump failed: %w\nStderr: %s", err, pgDumpErr.String())
	}

	// Wait for gzip to complete
	if err := gzipCmd.Wait(); err != nil {
		return fmt.Errorf("gzip failed: %w\nStderr: %s", err, gzipErr.String())
	}

	return nil
}

func validateSnapshot(path string) error {
	// Validate gzip integrity
	cmd := exec.Command("gzip", "-t", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gzip integrity check failed: %w", err)
	}
	return nil
}

func writeMetadata(path string, metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func loadMetadata(path string) (Metadata, error) {
	var metadata Metadata
	data, err := os.ReadFile(path)
	if err != nil {
		return metadata, err
	}
	err = json.Unmarshal(data, &metadata)
	return metadata, err
}
