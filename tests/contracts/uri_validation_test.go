package contracts_test

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/stretchr/testify/require"
)

func TestULIDValidation(t *testing.T) {
	require.NoError(t, ids.ValidateULID("01HYX3KQW7ERTV9XNBM2P8QJZF"))
	require.Error(t, ids.ValidateULID("not-a-ulid"))
}

func TestCanonicalURIValidation(t *testing.T) {
	uri, err := ids.BuildCanonicalURI("https://toronto.togather.foundation", "events", "01HYX3KQW7ERTV9XNBM2P8QJZF")
	require.NoError(t, err)
	require.Equal(t, "https://toronto.togather.foundation/events/01HYX3KQW7ERTV9XNBM2P8QJZF", uri)
}

func TestSameAsNormalization(t *testing.T) {
	uri, err := ids.NormalizeSameAs("https://toronto.togather.foundation", "events", "01HYX3KQW7ERTV9XNBM2P8QJZF")
	require.NoError(t, err)
	require.Equal(t, "https://toronto.togather.foundation/events/01HYX3KQW7ERTV9XNBM2P8QJZF", uri)
}
