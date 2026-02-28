package fileutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// tempFile is an interface over the temporary file used during an atomic write.
// It allows test code to inject failures at the write, sync, or close step.
type tempFile interface {
	io.WriteCloser
	Sync() error
	Name() string
}

// atomicFS is an interface over os functions used by atomicWriteWithFS.
// It exists solely to allow fault injection in tests.
type atomicFS interface {
	MkdirAll(path string, perm os.FileMode) error
	CreateTemp(dir, pattern string) (tempFile, error)
	Chmod(name string, mode os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
}

// defaultFS is the production implementation backed by the real os package.
type defaultFS struct{}

func (defaultFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}
func (defaultFS) CreateTemp(dir, pattern string) (tempFile, error) {
	return os.CreateTemp(dir, pattern)
}
func (defaultFS) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}
func (defaultFS) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
func (defaultFS) Remove(name string) error {
	return os.Remove(name)
}

// atomicWriteWithFS is the internal implementation. The exported AtomicWrite
// calls this with defaultFS{}.
func atomicWriteWithFS(fs atomicFS, path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := fs.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	f, err := fs.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := f.Name()

	ok := false
	closed := false
	defer func() {
		if !ok {
			if !closed {
				_ = f.Close()
			}
			_ = fs.Remove(tmpName)
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
	closed = true
	// os.CreateTemp creates files with 0600; set the requested permissions
	// before rename so the target inherits the correct mode atomically.
	if err := fs.Chmod(tmpName, perm); err != nil {
		_ = fs.Remove(tmpName)
		return fmt.Errorf("chmod: %w", err)
	}
	if err := fs.Rename(tmpName, path); err != nil {
		_ = fs.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}

	ok = true
	return nil
}

// AtomicWrite atomically writes data to path with the given permissions.
//
// It uses a write-temp→fsync→rename sequence. The temp file is created in the
// same directory as path to guarantee a same-filesystem (POSIX atomic) rename.
// perm sets the file mode; callers typically pass 0644. Unlike os.WriteFile,
// the process umask does not affect the final mode — perm is applied exactly
// via os.Chmod after the file is written. On any error the temp file is removed
// and path is left unchanged.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return atomicWriteWithFS(defaultFS{}, path, data, perm)
}
