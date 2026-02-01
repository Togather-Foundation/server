package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDatabaseURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantHost     string
		wantPort     string
		wantDatabase string
		wantUser     string
		wantPassword string
		wantErr      bool
	}{
		{
			name:         "Full URL with port",
			url:          "postgresql://user:pass@localhost:5432/testdb",
			wantHost:     "localhost",
			wantPort:     "5432",
			wantDatabase: "testdb",
			wantUser:     "user",
			wantPassword: "pass",
			wantErr:      false,
		},
		{
			name:         "URL without port (default 5432)",
			url:          "postgresql://user:pass@localhost/testdb",
			wantHost:     "localhost",
			wantPort:     "5432",
			wantDatabase: "testdb",
			wantUser:     "user",
			wantPassword: "pass",
			wantErr:      false,
		},
		{
			name:         "URL with query params",
			url:          "postgresql://user:pass@localhost:5432/testdb?sslmode=disable",
			wantHost:     "localhost",
			wantPort:     "5432",
			wantDatabase: "testdb",
			wantUser:     "user",
			wantPassword: "pass",
			wantErr:      false,
		},
		{
			name:         "postgres:// protocol",
			url:          "postgres://user:pass@localhost:5432/testdb",
			wantHost:     "localhost",
			wantPort:     "5432",
			wantDatabase: "testdb",
			wantUser:     "user",
			wantPassword: "pass",
			wantErr:      false,
		},
		{
			name:    "Invalid protocol",
			url:     "mysql://user:pass@localhost/testdb",
			wantErr: true,
		},
		{
			name:    "Missing @ separator",
			url:     "postgresql://user:pass-localhost:5432/testdb",
			wantErr: true,
		},
		{
			name:    "Missing database",
			url:     "postgresql://user:pass@localhost:5432",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, database, user, password, err := ParseDatabaseURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDatabaseURL() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseDatabaseURL() unexpected error: %v", err)
				return
			}

			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("port = %q, want %q", port, tt.wantPort)
			}
			if database != tt.wantDatabase {
				t.Errorf("database = %q, want %q", database, tt.wantDatabase)
			}
			if user != tt.wantUser {
				t.Errorf("user = %q, want %q", user, tt.wantUser)
			}
			if password != tt.wantPassword {
				t.Errorf("password = %q, want %q", password, tt.wantPassword)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.SnapshotDir == "" {
		t.Error("SnapshotDir should not be empty")
	}

	if cfg.RetentionDays != 7 {
		t.Errorf("RetentionDays = %d, want 7", cfg.RetentionDays)
	}
}

func TestList_EmptyDirectory(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	snapshots, err := List(tmpDir)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(snapshots) != 0 {
		t.Errorf("List() returned %d snapshots, want 0", len(snapshots))
	}
}

func TestList_NonExistentDirectory(t *testing.T) {
	snapshots, err := List("/nonexistent/directory")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(snapshots) != 0 {
		t.Errorf("List() returned %d snapshots, want 0", len(snapshots))
	}
}

func TestList_WithSnapshots(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock snapshot files
	snapshots := []struct {
		name     string
		metadata Metadata
	}{
		{
			name: "togather_testdb_20240101_120000_pre-deploy.sql.gz",
			metadata: Metadata{
				SnapshotName:  "togather_testdb_20240101_120000_pre-deploy.sql.gz",
				Database:      "testdb",
				Timestamp:     time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Reason:        "pre-deploy",
				RetentionDays: 7,
				ExpiresAt:     time.Date(2024, 1, 8, 12, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "togather_testdb_20240102_120000_manual.sql.gz",
			metadata: Metadata{
				SnapshotName:  "togather_testdb_20240102_120000_manual.sql.gz",
				Database:      "testdb",
				Timestamp:     time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
				Reason:        "manual",
				RetentionDays: 7,
				ExpiresAt:     time.Date(2024, 1, 9, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, snap := range snapshots {
		// Create snapshot file
		path := filepath.Join(tmpDir, snap.name)
		if err := os.WriteFile(path, []byte("mock snapshot data"), 0644); err != nil {
			t.Fatalf("Failed to create mock snapshot: %v", err)
		}

		// Create metadata file
		metadataPath := filepath.Join(tmpDir, snap.name[:len(snap.name)-7]+".meta.json")
		if err := writeMetadata(metadataPath, snap.metadata); err != nil {
			t.Fatalf("Failed to create metadata: %v", err)
		}
	}

	// List snapshots
	listed, err := List(tmpDir)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(listed) != 2 {
		t.Errorf("List() returned %d snapshots, want 2", len(listed))
	}

	// Verify snapshots are sorted by timestamp (newest first)
	if len(listed) == 2 {
		if listed[0].Metadata.SnapshotName != "togather_testdb_20240102_120000_manual.sql.gz" {
			t.Errorf("First snapshot = %s, want togather_testdb_20240102_120000_manual.sql.gz",
				listed[0].Metadata.SnapshotName)
		}
	}
}

func TestCleanup_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create old snapshot (should be deleted)
	oldPath := filepath.Join(tmpDir, "old_snapshot.sql.gz")
	if err := os.WriteFile(oldPath, []byte("old data"), 0644); err != nil {
		t.Fatalf("Failed to create old snapshot: %v", err)
	}

	// Set modification time to 10 days ago
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set mtime: %v", err)
	}

	// Create metadata for old snapshot
	oldMetadata := Metadata{
		SnapshotName:  "old_snapshot.sql.gz",
		Timestamp:     oldTime,
		RetentionDays: 7,
		ExpiresAt:     oldTime.Add(7 * 24 * time.Hour),
	}
	metadataPath := filepath.Join(tmpDir, "old_snapshot.meta.json")
	if err := writeMetadata(metadataPath, oldMetadata); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Run cleanup in dry-run mode
	deleted, err := Cleanup(tmpDir, 7, true)
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	if len(deleted) != 1 {
		t.Errorf("Cleanup() deleted %d snapshots, want 1", len(deleted))
	}

	// Verify file still exists (dry-run)
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		t.Error("Snapshot file was deleted in dry-run mode")
	}
}

func TestCleanup_ActualDelete(t *testing.T) {
	tmpDir := t.TempDir()

	// Create old snapshot (should be deleted)
	oldPath := filepath.Join(tmpDir, "old_snapshot.sql.gz")
	if err := os.WriteFile(oldPath, []byte("old data"), 0644); err != nil {
		t.Fatalf("Failed to create old snapshot: %v", err)
	}

	// Set modification time to 10 days ago
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set mtime: %v", err)
	}

	// Create metadata for old snapshot
	oldMetadata := Metadata{
		SnapshotName:  "old_snapshot.sql.gz",
		Timestamp:     oldTime,
		RetentionDays: 7,
		ExpiresAt:     oldTime.Add(7 * 24 * time.Hour),
	}
	metadataPath := filepath.Join(tmpDir, "old_snapshot.meta.json")
	if err := writeMetadata(metadataPath, oldMetadata); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Create recent snapshot (should NOT be deleted)
	recentPath := filepath.Join(tmpDir, "recent_snapshot.sql.gz")
	if err := os.WriteFile(recentPath, []byte("recent data"), 0644); err != nil {
		t.Fatalf("Failed to create recent snapshot: %v", err)
	}

	recentMetadata := Metadata{
		SnapshotName:  "recent_snapshot.sql.gz",
		Timestamp:     time.Now(),
		RetentionDays: 7,
		ExpiresAt:     time.Now().Add(7 * 24 * time.Hour),
	}
	recentMetadataPath := filepath.Join(tmpDir, "recent_snapshot.meta.json")
	if err := writeMetadata(recentMetadataPath, recentMetadata); err != nil {
		t.Fatalf("Failed to create recent metadata: %v", err)
	}

	// Run actual cleanup
	deleted, err := Cleanup(tmpDir, 7, false)
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	if len(deleted) != 1 {
		t.Errorf("Cleanup() deleted %d snapshots, want 1", len(deleted))
	}

	// Verify old file was deleted
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("Old snapshot file was not deleted")
	}

	// Verify recent file still exists
	if _, err := os.Stat(recentPath); err != nil {
		t.Error("Recent snapshot file was incorrectly deleted")
	}
}

func TestCreatePgpassFile(t *testing.T) {
	path, err := createPgpassFile("localhost", "5432", "testdb", "user", "password")
	if err != nil {
		t.Fatalf("createPgpassFile() error: %v", err)
	}
	defer os.Remove(path)

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("pgpass file does not exist: %v", err)
	}

	// Verify file permissions (should be 0600)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat pgpass file: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("pgpass file permissions = %o, want 0600", mode)
	}

	// Verify file content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read pgpass file: %v", err)
	}

	expected := "localhost:5432:testdb:user:password\n"
	if string(content) != expected {
		t.Errorf("pgpass content = %q, want %q", string(content), expected)
	}
}

func TestCreate_Validation(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		opts    CreateOptions
		wantErr string
	}{
		{
			name: "Missing database",
			opts: CreateOptions{
				Host:        "localhost",
				Port:        "5432",
				User:        "user",
				Password:    "pass",
				SnapshotDir: tmpDir,
			},
			wantErr: "database name is required",
		},
		{
			name: "Missing host",
			opts: CreateOptions{
				Database:    "testdb",
				Port:        "5432",
				User:        "user",
				Password:    "pass",
				SnapshotDir: tmpDir,
			},
			wantErr: "database host is required",
		},
		{
			name: "Missing port",
			opts: CreateOptions{
				Database:    "testdb",
				Host:        "localhost",
				User:        "user",
				Password:    "pass",
				SnapshotDir: tmpDir,
			},
			wantErr: "database port is required",
		},
		{
			name: "Missing user",
			opts: CreateOptions{
				Database:    "testdb",
				Host:        "localhost",
				Port:        "5432",
				Password:    "pass",
				SnapshotDir: tmpDir,
			},
			wantErr: "database user is required",
		},
		{
			name: "Missing password",
			opts: CreateOptions{
				Database:    "testdb",
				Host:        "localhost",
				Port:        "5432",
				User:        "user",
				SnapshotDir: tmpDir,
			},
			wantErr: "database password is required",
		},
		{
			name: "Missing snapshot dir",
			opts: CreateOptions{
				Database: "testdb",
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "pass",
			},
			wantErr: "snapshot directory is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Create(ctx, tt.opts)
			if err == nil {
				t.Error("Create() expected error, got nil")
				return
			}

			if err.Error() != tt.wantErr {
				t.Errorf("Create() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
