package fileutil_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Togather-Foundation/server/internal/fileutil"
)

func TestAtomicWrite_NormalWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.json")
	data := []byte(`{"hello":"world"}`)

	if err := fileutil.AtomicWrite(path, data, 0644); err != nil {
		t.Fatalf("AtomicWrite returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch: got %q, want %q", got, data)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// Mask to lower 9 bits (rwxrwxrwx) to ignore sticky/setuid bits.
	if got := info.Mode().Perm(); got != 0644 {
		t.Errorf("permissions: got %04o, want %04o", got, 0644)
	}
}

func TestAtomicWrite_DifferentPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.json")
	data := []byte(`{"secret":"value"}`)

	if err := fileutil.AtomicWrite(path, data, 0600); err != nil {
		t.Fatalf("AtomicWrite returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("permissions: got %04o, want %04o", got, 0600)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch: got %q, want %q", got, data)
	}
}

func TestAtomicWrite_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	old := []byte(`{"version":"1"}`)
	if err := os.WriteFile(path, old, 0644); err != nil {
		t.Fatalf("setup WriteFile: %v", err)
	}

	newData := []byte(`{"version":"2"}`)
	if err := fileutil.AtomicWrite(path, newData, 0644); err != nil {
		t.Fatalf("AtomicWrite returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(newData) {
		t.Errorf("content mismatch: got %q, want %q", got, newData)
	}
}

func TestAtomicWrite_UnwritableDirectory(t *testing.T) {
	dir := t.TempDir()

	// Make dir unwritable so CreateTemp will fail.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("Chmod dir: %v", err)
	}
	t.Cleanup(func() {
		// Restore permissions so TempDir cleanup can remove the directory.
		_ = os.Chmod(dir, 0755)
	})

	// Skip if running as root (root ignores permission bits).
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root")
	}

	path := filepath.Join(dir, "file.json")
	err := fileutil.AtomicWrite(path, []byte(`{}`), 0644)
	if err == nil {
		t.Fatal("expected error for unwritable directory, got nil")
	}

	// Ensure no temp file was left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Errorf("unexpected file left in dir: %s", e.Name())
	}
}

func TestAtomicWrite_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.json")

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			data := []byte(fmt.Sprintf(`{"writer":%d}`, i))
			// Ignore errors — rename races are acceptable.
			_ = fileutil.AtomicWrite(path, data, 0644)
		}()
	}
	wg.Wait()

	// The file must exist and contain valid JSON written by one of the goroutines.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after concurrent writes: %v", err)
	}
	var result struct {
		Writer int `json:"writer"`
	}
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatalf("file not valid JSON after concurrent writes: %v — content: %q", err, got)
	}
	if result.Writer < 0 || result.Writer >= goroutines {
		t.Errorf("writer index out of range: got %d, want [0,%d)", result.Writer, goroutines)
	}
}
