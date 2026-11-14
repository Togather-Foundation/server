package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestSourcePreflightRejectsInvalidConfigWithoutSecrets(t *testing.T) {
	previous := sourceRepairDirectory
	t.Cleanup(func() { sourceRepairDirectory = previous })
	sourceRepairDirectory = "https://operator:supersecret@invalid.example.org/absent?token=secretquery"
	command := &cobra.Command{}
	command.SetContext(context.Background())
	var output bytes.Buffer
	command.SetOut(&output)
	err := runSourcePreflight(command, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read source config inventory")
	require.NotContains(t, err.Error(), "supersecret")
	require.NotContains(t, err.Error(), "secretquery")
	require.Empty(t, output.String())
}

func TestSourcePreflightCommandAvailable(t *testing.T) {
	command, _, err := sourceCmd.Find([]string{"preflight"})
	require.NoError(t, err)
	require.Equal(t, "preflight", command.Name())
	require.NotNil(t, command.Flags().Lookup("json"))
	require.NotNil(t, command.RunE)
}

func TestSourcePreflightEmptyInventoryFailsBeforeDatabase(t *testing.T) {
	previous := sourceRepairDirectory
	t.Cleanup(func() { sourceRepairDirectory = previous })
	sourceRepairDirectory = t.TempDir()
	command := &cobra.Command{}
	command.SetContext(context.Background())
	err := runSourcePreflight(command, nil)
	require.ErrorContains(t, err, "no source configs found")
}
