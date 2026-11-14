package postgres

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Opt-in because this regression starts two additional PostgreSQL containers.
func TestStorageProcessIsolation(t *testing.T) {
	if os.Getenv("RUN_STORAGE_PROCESS_ISOLATION") != "1" {
		t.Skip("set RUN_STORAGE_PROCESS_ISOLATION=1 to run the two-process regression")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	dir := t.TempDir()
	executable, err := os.Executable()
	require.NoError(t, err)
	start := func(role string) (*exec.Cmd, *bytes.Buffer) {
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestStorageProcessIsolationChild$", "-test.count=1")
		cmd.Env = append(os.Environ(), "STORAGE_ISOLATION_ROLE="+role, "STORAGE_ISOLATION_DIR="+dir)
		output := new(bytes.Buffer)
		cmd.Stdout, cmd.Stderr = output, output
		require.NoError(t, cmd.Start())
		return cmd, output
	}
	// B must reset its database while A still has data. A then exits (including
	// TestMain teardown) before B checks its data and connection again.
	a, aOutput := start("a")
	b, bOutput := start("b")
	aErr := a.Wait()
	// Always reap both children, including on assertion failures.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a-exited"), nil, 0o600))
	bErr := b.Wait()
	require.NoError(t, aErr, "first process: %s", aOutput.String())
	require.NoError(t, bErr, "second process: %s", bOutput.String())
}

func TestStorageProcessIsolationChild(t *testing.T) {
	role := os.Getenv("STORAGE_ISOLATION_ROLE")
	if role == "" {
		t.Skip("subprocess helper")
	}
	require.Contains(t, []string{"a", "b"}, role)
	dir := os.Getenv("STORAGE_ISOLATION_DIR")
	require.NotEmpty(t, dir)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	wait := func(name string) {
		t.Helper()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			if name == "a-ready" {
				if _, err := os.Stat(filepath.Join(dir, "a-exited")); err == nil {
					t.Fatal("first process exited before database initialization")
				}
			}
			_, err := os.Stat(filepath.Join(dir, name))
			if err == nil {
				return
			}
			require.True(t, os.IsNotExist(err), "wait for %s: %v", name, err)
			select {
			case <-ctx.Done():
				t.Fatalf("wait for %s: %v", name, ctx.Err())
			case <-ticker.C:
			}
		}
	}
	if role == "b" {
		wait("a-ready")
	}
	pool, _ := setupPostgres(t, ctx)
	entity := insertOrganization(t, ctx, pool, "process "+role)
	require.NoError(t, os.WriteFile(filepath.Join(dir, role+"-ready"), nil, 0o600))
	if role == "a" {
		wait("b-ready")
	} else {
		wait("a-exited")
	}
	var name string
	require.NoError(t, pool.QueryRow(ctx, "SELECT name FROM organizations WHERE id = $1", entity.ID).Scan(&name), "another process reset or terminated this database")
	require.Equal(t, "process "+role, name)
}
