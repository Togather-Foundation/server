package fileutil

import (
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failFile is a tempFile implementation that can inject failures on Write,
// Sync, or Close.
type failFile struct {
	name       string
	writeErr   error
	syncErr    error
	closeErr   error
	closeCount int
}

func (f *failFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}

func (f *failFile) Sync() error { return f.syncErr }

func (f *failFile) Close() error {
	f.closeCount++
	if f.closeErr != nil && f.closeCount == 1 {
		return f.closeErr
	}
	return nil
}

func (f *failFile) Name() string { return f.name }

// failFS embeds defaultFS and lets individual operations fail.
type failFS struct {
	defaultFS
	mkdirErr   error
	createErr  error
	createFile tempFile // if set, returned instead of a real file
	chmodErr   error
	renameErr  error
}

func (f failFS) MkdirAll(path string, perm os.FileMode) error {
	if f.mkdirErr != nil {
		return f.mkdirErr
	}
	return f.defaultFS.MkdirAll(path, perm)
}

func (f failFS) CreateTemp(dir, pattern string) (tempFile, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createFile != nil {
		return f.createFile, nil
	}
	return f.defaultFS.CreateTemp(dir, pattern)
}

func (f failFS) Chmod(name string, mode os.FileMode) error {
	if f.chmodErr != nil {
		return f.chmodErr
	}
	return f.defaultFS.Chmod(name, mode)
}

func (f failFS) Rename(old, new string) error {
	if f.renameErr != nil {
		return f.renameErr
	}
	return f.defaultFS.Rename(old, new)
}

// tempDirHasNoTmpFiles asserts that tmpDir contains no leftover .tmp-* files.
func tempDirHasNoTmpFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("ReadDir %s: %v", dir, err)
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after error: %s", e.Name())
		}
	}
}

func TestAtomicWrite_ErrorPaths(t *testing.T) {
	tests := []struct {
		name       string
		buildFS    func(dir string) atomicFS
		wantErrMsg string
	}{
		{
			name: "MkdirAll failure",
			buildFS: func(dir string) atomicFS {
				return failFS{mkdirErr: errors.New("mkdir: permission denied")}
			},
			wantErrMsg: "mkdir",
		},
		{
			name: "CreateTemp failure",
			buildFS: func(dir string) atomicFS {
				return failFS{createErr: errors.New("createtemp: no space")}
			},
			wantErrMsg: "create temp",
		},
		{
			name: "Write failure",
			buildFS: func(dir string) atomicFS {
				ff := &failFile{
					name:     filepath.Join(dir, ".tmp-failwrite"),
					writeErr: errors.New("write: disk full"),
				}
				return failFS{createFile: ff}
			},
			wantErrMsg: "write",
		},
		{
			name: "Sync failure",
			buildFS: func(dir string) atomicFS {
				ff := &failFile{
					name:    filepath.Join(dir, ".tmp-failsync"),
					syncErr: errors.New("sync: io error"),
				}
				return failFS{createFile: ff}
			},
			wantErrMsg: "sync",
		},
		{
			name: "Close failure",
			buildFS: func(dir string) atomicFS {
				ff := &failFile{
					name:     filepath.Join(dir, ".tmp-failclose"),
					closeErr: errors.New("close: io error"),
				}
				return failFS{createFile: ff}
			},
			wantErrMsg: "close",
		},
		{
			name: "Chmod failure",
			buildFS: func(dir string) atomicFS {
				return failFS{chmodErr: errors.New("chmod: denied")}
			},
			wantErrMsg: "chmod",
		},
		{
			name: "Rename failure",
			buildFS: func(dir string) atomicFS {
				return failFS{renameErr: errors.New("rename: cross-device")}
			},
			wantErrMsg: "rename",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "output.json")
			fs := tc.buildFS(dir)

			err := atomicWriteWithFS(fs, path, []byte(`{"test":true}`), 0644)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErrMsg)
			}
			if !strings.Contains(err.Error(), tc.wantErrMsg) {
				t.Errorf("error %q does not contain expected substring %q", err.Error(), tc.wantErrMsg)
			}

			// Destination file must not be created on error.
			if _, statErr := os.Stat(path); !errors.Is(statErr, iofs.ErrNotExist) {
				t.Errorf("destination file unexpectedly exists after error")
			}

			// No temp files should be left in the temp dir.
			tempDirHasNoTmpFiles(t, dir)
		})
	}
}
