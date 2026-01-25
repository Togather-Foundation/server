package jsonld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextLoaderLoadSuccessAndCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v1.jsonld")
	require.NoError(t, os.WriteFile(path, []byte(`{"@context":{"name":"schema:name"}}`), 0o600))

	loader := NewContextLoader(dir)

	first, err := loader.Load("v1")

	require.NoError(t, err)
	require.Equal(t, map[string]any{"@context": map[string]any{"name": "schema:name"}}, first)

	require.NoError(t, os.WriteFile(path, []byte(`{"@context":{"name":"schema:other"}}`), 0o600))

	second, err := loader.Load("v1")

	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestContextLoaderLoadInvalidVersion(t *testing.T) {
	loader := NewContextLoader(t.TempDir())

	_, err := loader.Load("")

	require.ErrorIs(t, err, ErrInvalidVersion)
}

func TestContextLoaderLoadNotFound(t *testing.T) {
	loader := NewContextLoader(t.TempDir())

	_, err := loader.Load("missing")

	require.ErrorIs(t, err, ErrContextNotFound)
}

func TestResolveDefaultContextDir(t *testing.T) {
	dir := t.TempDir()
	contextDir := filepath.Join(dir, DefaultContextDir)
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, DefaultContextVersion+".jsonld"), []byte(`{"@context":{}}`), 0o600))

	cwd, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	resolved := resolveDefaultContextDir()

	require.Equal(t, DefaultContextDir, resolved)
}
