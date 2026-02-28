package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path atomically using a write-temp→fsync→rename
// sequence. The temp file is created in the same directory as path to guarantee
// a same-filesystem (POSIX atomic) rename. perm sets the file mode; callers
// typically pass 0644. On any error the temp file is removed and path is left
// unchanged.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := f.Name()

	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmpName)
		}
	}()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	// os.CreateTemp creates files with 0600; set the requested permissions
	// before rename so the target inherits the correct mode atomically.
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}

	ok = true
	return nil
}
