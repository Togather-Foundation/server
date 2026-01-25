package events

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/stretchr/testify/require"
)

func TestValidateULID(t *testing.T) {
	valid, err := ids.NewULID()
	require.NoError(t, err)

	require.NoError(t, ids.ValidateULID(valid))
	require.ErrorIs(t, ids.ValidateULID("not-a-ulid"), ids.ErrInvalidULID)
}
