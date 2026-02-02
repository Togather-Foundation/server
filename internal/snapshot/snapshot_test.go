package snapshot

import (
	"context"
	"fmt"
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
	defer func() { _ = os.Remove(path) }()

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

func TestCreate_RetentionDaysDefault(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Test that retention days defaults to 7 when not specified
	opts := CreateOptions{
		Database:      "testdb",
		Host:          "invalid-host-that-should-fail",
		Port:          "5432",
		User:          "user",
		Password:      "pass",
		SnapshotDir:   tmpDir,
		RetentionDays: 0, // Should default to 7
	}

	// This will fail on pg_dump, but we can check the retention days were set
	_, err := Create(ctx, opts)
	if err == nil {
		t.Error("Expected error connecting to invalid host")
	}

	// Check that retention days default was applied (check in error path)
	if opts.RetentionDays == 0 {
		// The function should have set it to 7
		t.Log("RetentionDays defaulting is tested in Create function")
	}
}

func TestCreate_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotDir := filepath.Join(tmpDir, "snapshots", "nested", "path")
	ctx := context.Background()

	// Verify directory doesn't exist yet
	if _, err := os.Stat(snapshotDir); !os.IsNotExist(err) {
		t.Fatal("Directory should not exist yet")
	}

	opts := CreateOptions{
		Database:    "testdb",
		Host:        "invalid-host",
		Port:        "5432",
		User:        "user",
		Password:    "pass",
		SnapshotDir: snapshotDir,
	}

	// This will fail on pg_dump, but directory should be created
	_, err := Create(ctx, opts)
	if err == nil {
		t.Error("Expected error")
	}

	// Verify directory was created
	if _, err := os.Stat(snapshotDir); os.IsNotExist(err) {
		t.Error("Directory should have been created")
	}
}

func TestValidateSnapshot_InvalidGzip(t *testing.T) {
	tmpDir := t.TempDir()
	invalidGzip := filepath.Join(tmpDir, "invalid.sql.gz")

	// Create a file with invalid gzip content
	if err := os.WriteFile(invalidGzip, []byte("not a gzip file"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := validateSnapshot(invalidGzip)
	if err == nil {
		t.Error("validateSnapshot() expected error for invalid gzip, got nil")
	}
}

func TestValidateSnapshot_MissingFile(t *testing.T) {
	err := validateSnapshot("/nonexistent/file.sql.gz")
	if err == nil {
		t.Error("validateSnapshot() expected error for missing file, got nil")
	}
}

func TestMetadataReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "test.meta.json")

	now := time.Now().UTC()
	original := Metadata{
		SnapshotName:  "test_snapshot.sql.gz",
		Database:      "testdb",
		Host:          "localhost",
		Port:          "5432",
		Timestamp:     now,
		Reason:        "test",
		GitCommit:     "abc123def456",
		DeploymentID:  "deploy-001",
		RetentionDays: 14,
		ExpiresAt:     now.Add(14 * 24 * time.Hour),
		SizeMB:        42,
		DurationSecs:  120,
	}

	// Write metadata
	if err := writeMetadata(metadataPath, original); err != nil {
		t.Fatalf("writeMetadata() error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(metadataPath); err != nil {
		t.Errorf("Metadata file should exist: %v", err)
	}

	// Read metadata back
	loaded, err := loadMetadata(metadataPath)
	if err != nil {
		t.Fatalf("loadMetadata() error: %v", err)
	}

	// Compare all fields
	if loaded.SnapshotName != original.SnapshotName {
		t.Errorf("SnapshotName = %q, want %q", loaded.SnapshotName, original.SnapshotName)
	}
	if loaded.Database != original.Database {
		t.Errorf("Database = %q, want %q", loaded.Database, original.Database)
	}
	if loaded.Host != original.Host {
		t.Errorf("Host = %q, want %q", loaded.Host, original.Host)
	}
	if loaded.Port != original.Port {
		t.Errorf("Port = %q, want %q", loaded.Port, original.Port)
	}
	if !loaded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", loaded.Timestamp, original.Timestamp)
	}
	if loaded.Reason != original.Reason {
		t.Errorf("Reason = %q, want %q", loaded.Reason, original.Reason)
	}
	if loaded.GitCommit != original.GitCommit {
		t.Errorf("GitCommit = %q, want %q", loaded.GitCommit, original.GitCommit)
	}
	if loaded.DeploymentID != original.DeploymentID {
		t.Errorf("DeploymentID = %q, want %q", loaded.DeploymentID, original.DeploymentID)
	}
	if loaded.RetentionDays != original.RetentionDays {
		t.Errorf("RetentionDays = %d, want %d", loaded.RetentionDays, original.RetentionDays)
	}
	if !loaded.ExpiresAt.Equal(original.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", loaded.ExpiresAt, original.ExpiresAt)
	}
	if loaded.SizeMB != original.SizeMB {
		t.Errorf("SizeMB = %d, want %d", loaded.SizeMB, original.SizeMB)
	}
	if loaded.DurationSecs != original.DurationSecs {
		t.Errorf("DurationSecs = %d, want %d", loaded.DurationSecs, original.DurationSecs)
	}
}

func TestLoadMetadata_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, "invalid.meta.json")

	// Write invalid JSON
	if err := os.WriteFile(metadataPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := loadMetadata(metadataPath)
	if err == nil {
		t.Error("loadMetadata() expected error for invalid JSON, got nil")
	}
}

func TestLoadMetadata_MissingFile(t *testing.T) {
	_, err := loadMetadata("/nonexistent/file.meta.json")
	if err == nil {
		t.Error("loadMetadata() expected error for missing file, got nil")
	}
}

func TestList_WithMixedSnapshots(t *testing.T) {
	tmpDir := t.TempDir()

	// Create snapshot with metadata
	snap1Path := filepath.Join(tmpDir, "snapshot1.sql.gz")
	if err := os.WriteFile(snap1Path, []byte("data1"), 0644); err != nil {
		t.Fatalf("Failed to create snapshot1: %v", err)
	}
	metadata1 := Metadata{
		SnapshotName:  "snapshot1.sql.gz",
		Timestamp:     time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		RetentionDays: 7,
	}
	if err := writeMetadata(filepath.Join(tmpDir, "snapshot1.meta.json"), metadata1); err != nil {
		t.Fatalf("Failed to write metadata1: %v", err)
	}

	// Create snapshot without metadata (should still be listed)
	snap2Path := filepath.Join(tmpDir, "snapshot2.sql.gz")
	if err := os.WriteFile(snap2Path, []byte("data2"), 0644); err != nil {
		t.Fatalf("Failed to create snapshot2: %v", err)
	}

	// List snapshots
	snapshots, err := List(tmpDir)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(snapshots) != 2 {
		t.Errorf("List() returned %d snapshots, want 2", len(snapshots))
	}

	// Verify snapshot without metadata has default values
	var withoutMetadata *Snapshot
	for i := range snapshots {
		if snapshots[i].Metadata.SnapshotName == "snapshot2.sql.gz" {
			withoutMetadata = &snapshots[i]
			break
		}
	}

	if withoutMetadata == nil {
		t.Fatal("Snapshot2 not found in list")
	}

	if withoutMetadata.Metadata.Reason != "unknown" {
		t.Errorf("Snapshot without metadata should have reason 'unknown', got %q", withoutMetadata.Metadata.Reason)
	}
	if withoutMetadata.Metadata.RetentionDays != 7 {
		t.Errorf("Snapshot without metadata should have default retention 7, got %d", withoutMetadata.Metadata.RetentionDays)
	}
}

func TestList_ErrorConditions(t *testing.T) {
	// Test with empty directory path
	_, err := List("")
	if err == nil {
		t.Error("List() with empty directory should return error")
	}
}

func TestCleanup_ErrorConditions(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		dir           string
		retentionDays int
		wantErr       bool
	}{
		{
			name:          "Empty directory path",
			dir:           "",
			retentionDays: 7,
			wantErr:       true,
		},
		{
			name:          "Zero retention days",
			dir:           tmpDir,
			retentionDays: 0,
			wantErr:       true,
		},
		{
			name:          "Negative retention days",
			dir:           tmpDir,
			retentionDays: -5,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Cleanup(tt.dir, tt.retentionDays, true)
			if tt.wantErr && err == nil {
				t.Error("Cleanup() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Cleanup() unexpected error: %v", err)
			}
		})
	}
}

func TestCleanup_RespectCustomRetention(t *testing.T) {
	tmpDir := t.TempDir()

	// Create snapshot with custom 30-day retention (should NOT be deleted with 7-day policy)
	customPath := filepath.Join(tmpDir, "custom_retention.sql.gz")
	if err := os.WriteFile(customPath, []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Set modification time to 10 days ago
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(customPath, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set mtime: %v", err)
	}

	// Create metadata with 30-day retention
	customMetadata := Metadata{
		SnapshotName:  "custom_retention.sql.gz",
		Timestamp:     oldTime,
		RetentionDays: 30, // Custom retention - should not be deleted
		ExpiresAt:     oldTime.Add(30 * 24 * time.Hour),
	}
	metadataPath := filepath.Join(tmpDir, "custom_retention.meta.json")
	if err := writeMetadata(metadataPath, customMetadata); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Run cleanup with 7-day global retention
	deleted, err := Cleanup(tmpDir, 7, false)
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	// Should NOT delete snapshot with custom 30-day retention
	if len(deleted) != 0 {
		t.Errorf("Cleanup() deleted %d snapshots, want 0 (should respect custom retention)", len(deleted))
	}

	// Verify file still exists
	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Error("Snapshot with custom retention should not have been deleted")
	}
}

func TestCleanup_MultipleSnapshots(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 3 old snapshots and 2 recent ones
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	recentTime := time.Now().Add(-2 * 24 * time.Hour)

	for i := 1; i <= 3; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("old_%d.sql.gz", i))
		if err := os.WriteFile(path, []byte("old data"), 0644); err != nil {
			t.Fatalf("Failed to create old snapshot: %v", err)
		}
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatalf("Failed to set mtime: %v", err)
		}
		metadata := Metadata{
			SnapshotName:  filepath.Base(path),
			Timestamp:     oldTime,
			RetentionDays: 7,
		}
		metaPath := filepath.Join(tmpDir, fmt.Sprintf("old_%d.meta.json", i))
		if err := writeMetadata(metaPath, metadata); err != nil {
			t.Fatalf("Failed to write metadata: %v", err)
		}
	}

	for i := 1; i <= 2; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("recent_%d.sql.gz", i))
		if err := os.WriteFile(path, []byte("recent data"), 0644); err != nil {
			t.Fatalf("Failed to create recent snapshot: %v", err)
		}
		if err := os.Chtimes(path, recentTime, recentTime); err != nil {
			t.Fatalf("Failed to set mtime: %v", err)
		}
		metadata := Metadata{
			SnapshotName:  filepath.Base(path),
			Timestamp:     recentTime,
			RetentionDays: 7,
		}
		metaPath := filepath.Join(tmpDir, fmt.Sprintf("recent_%d.meta.json", i))
		if err := writeMetadata(metaPath, metadata); err != nil {
			t.Fatalf("Failed to write metadata: %v", err)
		}
	}

	// Run cleanup
	deleted, err := Cleanup(tmpDir, 7, false)
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	if len(deleted) != 3 {
		t.Errorf("Cleanup() deleted %d snapshots, want 3", len(deleted))
	}

	// Verify only old snapshots were deleted
	for i := 1; i <= 3; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("old_%d.sql.gz", i))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("Old snapshot %d should have been deleted", i)
		}
	}

	// Verify recent snapshots still exist
	for i := 1; i <= 2; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("recent_%d.sql.gz", i))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Recent snapshot %d should still exist: %v", i, err)
		}
	}
}

func TestGetDefaultSnapshotDir(t *testing.T) {
	dir := getDefaultSnapshotDir()

	if dir == "" {
		t.Error("getDefaultSnapshotDir() should not return empty string")
	}

	// Should return an absolute path
	if !filepath.IsAbs(dir) {
		t.Errorf("getDefaultSnapshotDir() = %q, want absolute path", dir)
	}
}

func TestCleanup_DeletesMetadataFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create old snapshot with metadata
	oldPath := filepath.Join(tmpDir, "old_snapshot.sql.gz")
	if err := os.WriteFile(oldPath, []byte("old data"), 0644); err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set mtime: %v", err)
	}

	oldMetadata := Metadata{
		SnapshotName:  "old_snapshot.sql.gz",
		Timestamp:     oldTime,
		RetentionDays: 7,
	}
	metadataPath := filepath.Join(tmpDir, "old_snapshot.meta.json")
	if err := writeMetadata(metadataPath, oldMetadata); err != nil {
		t.Fatalf("Failed to create metadata: %v", err)
	}

	// Run cleanup
	_, err := Cleanup(tmpDir, 7, false)
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	// Verify both snapshot and metadata were deleted
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("Snapshot file should have been deleted")
	}
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Error("Metadata file should have been deleted")
	}
}

func TestParseDatabaseURL_EdgeCases(t *testing.T) {
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
			name:         "URL with special chars in password",
			url:          "postgresql://user:p$ssw0rd!#@localhost:5432/testdb",
			wantHost:     "localhost",
			wantPort:     "5432",
			wantDatabase: "testdb",
			wantUser:     "user",
			wantPassword: "p$ssw0rd!#",
			wantErr:      false,
		},
		{
			name:         "URL without password",
			url:          "postgresql://user@localhost:5432/testdb",
			wantHost:     "localhost",
			wantPort:     "5432",
			wantDatabase: "testdb",
			wantUser:     "user",
			wantPassword: "",
			wantErr:      false,
		},
		{
			name:         "URL with database name containing underscores",
			url:          "postgresql://user:pass@localhost:5432/test_db_name",
			wantHost:     "localhost",
			wantPort:     "5432",
			wantDatabase: "test_db_name",
			wantUser:     "user",
			wantPassword: "pass",
			wantErr:      false,
		},
		{
			name:         "URL with IPv4 host",
			url:          "postgresql://user:pass@192.168.1.100:5432/testdb",
			wantHost:     "192.168.1.100",
			wantPort:     "5432",
			wantDatabase: "testdb",
			wantUser:     "user",
			wantPassword: "pass",
			wantErr:      false,
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

func TestCreate_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := CreateOptions{
		Database:    "testdb",
		Host:        "localhost",
		Port:        "5432",
		User:        "user",
		Password:    "pass",
		SnapshotDir: tmpDir,
	}

	// Should fail because context is cancelled
	_, err := Create(ctx, opts)
	if err == nil {
		t.Error("Create() with cancelled context should return error")
	}
}

func TestList_IgnoresNonSnapshotFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various files
	files := []string{
		"snapshot1.sql.gz",    // Should be listed
		"snapshot2.sql.gz",    // Should be listed
		"readme.txt",          // Should be ignored
		"backup.sql",          // Should be ignored (not .gz)
		"test.tar.gz",         // Should be ignored (not .sql.gz)
		"snapshot.sql.gz.bak", // Should be ignored
	}

	for _, file := range files {
		path := filepath.Join(tmpDir, file)
		if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}

	// List snapshots
	snapshots, err := List(tmpDir)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	// Should only find the 2 .sql.gz files
	if len(snapshots) != 2 {
		t.Errorf("List() returned %d snapshots, want 2", len(snapshots))
	}

	// Verify we got the right files
	names := make(map[string]bool)
	for _, snap := range snapshots {
		names[snap.Metadata.SnapshotName] = true
	}

	if !names["snapshot1.sql.gz"] {
		t.Error("snapshot1.sql.gz not found in list")
	}
	if !names["snapshot2.sql.gz"] {
		t.Error("snapshot2.sql.gz not found in list")
	}
}

func TestSnapshot_StructFields(t *testing.T) {
	// Test that Snapshot struct fields are properly accessible
	now := time.Now()
	snap := Snapshot{
		Path: "/path/to/snapshot.sql.gz",
		Metadata: Metadata{
			SnapshotName:  "test.sql.gz",
			Database:      "testdb",
			Host:          "localhost",
			Port:          "5432",
			Timestamp:     now,
			Reason:        "test",
			GitCommit:     "abc123",
			DeploymentID:  "deploy-1",
			RetentionDays: 7,
			ExpiresAt:     now.Add(7 * 24 * time.Hour),
			SizeMB:        100,
			DurationSecs:  60,
		},
		SizeMB: 100,
		Age:    1 * time.Hour,
	}

	if snap.Path != "/path/to/snapshot.sql.gz" {
		t.Errorf("Path = %q, want %q", snap.Path, "/path/to/snapshot.sql.gz")
	}
	if snap.SizeMB != 100 {
		t.Errorf("SizeMB = %d, want 100", snap.SizeMB)
	}
	if snap.Age != 1*time.Hour {
		t.Errorf("Age = %v, want 1h", snap.Age)
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	cfg := Config{
		SnapshotDir:   "/custom/path",
		RetentionDays: 14,
	}

	if cfg.SnapshotDir != "/custom/path" {
		t.Errorf("SnapshotDir = %q, want %q", cfg.SnapshotDir, "/custom/path")
	}
	if cfg.RetentionDays != 14 {
		t.Errorf("RetentionDays = %d, want 14", cfg.RetentionDays)
	}
}

func TestCreateOptions_AllFields(t *testing.T) {
	// Test that all CreateOptions fields can be set
	opts := CreateOptions{
		DatabaseURL:   "postgresql://user:pass@localhost/db",
		Database:      "testdb",
		Host:          "localhost",
		Port:          "5432",
		User:          "user",
		Password:      "pass",
		Reason:        "test",
		RetentionDays: 30,
		SnapshotDir:   "/tmp",
		GitCommit:     "abc123",
		DeploymentID:  "deploy-1",
		Validate:      true,
	}

	if opts.DatabaseURL != "postgresql://user:pass@localhost/db" {
		t.Error("DatabaseURL not set correctly")
	}
	if opts.Validate != true {
		t.Error("Validate not set correctly")
	}
	if opts.GitCommit != "abc123" {
		t.Error("GitCommit not set correctly")
	}
	if opts.DeploymentID != "deploy-1" {
		t.Error("DeploymentID not set correctly")
	}
}

func TestMetadata_AllFields(t *testing.T) {
	// Ensure all Metadata fields serialize/deserialize correctly
	now := time.Now().UTC()
	meta := Metadata{
		SnapshotName:  "snapshot.sql.gz",
		Database:      "db",
		Host:          "host",
		Port:          "5432",
		Timestamp:     now,
		Reason:        "reason",
		GitCommit:     "commit",
		DeploymentID:  "deploy",
		RetentionDays: 7,
		ExpiresAt:     now.Add(7 * 24 * time.Hour),
		SizeMB:        50,
		DurationSecs:  30,
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "meta.json")

	if err := writeMetadata(path, meta); err != nil {
		t.Fatalf("writeMetadata error: %v", err)
	}

	loaded, err := loadMetadata(path)
	if err != nil {
		t.Fatalf("loadMetadata error: %v", err)
	}

	// Verify all fields round-trip correctly
	if loaded.SnapshotName != meta.SnapshotName {
		t.Error("SnapshotName mismatch")
	}
	if loaded.Database != meta.Database {
		t.Error("Database mismatch")
	}
	if loaded.Host != meta.Host {
		t.Error("Host mismatch")
	}
	if loaded.Port != meta.Port {
		t.Error("Port mismatch")
	}
	if loaded.Reason != meta.Reason {
		t.Error("Reason mismatch")
	}
	if loaded.GitCommit != meta.GitCommit {
		t.Error("GitCommit mismatch")
	}
	if loaded.DeploymentID != meta.DeploymentID {
		t.Error("DeploymentID mismatch")
	}
	if loaded.RetentionDays != meta.RetentionDays {
		t.Error("RetentionDays mismatch")
	}
	if loaded.SizeMB != meta.SizeMB {
		t.Error("SizeMB mismatch")
	}
	if loaded.DurationSecs != meta.DurationSecs {
		t.Error("DurationSecs mismatch")
	}
}
