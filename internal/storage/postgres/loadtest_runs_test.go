package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeLoadtestInputSyntheticContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		want    bool
		invalid bool
	}{
		{"legacy omission", `{}`, false, false},
		{"false", `{"sel:synthetic":false}`, false, false},
		{"true", `{"sel:synthetic":true}`, true, false},
		{"null", `{"sel:synthetic":null}`, false, true},
		{"string", `{"sel:synthetic":"true"}`, false, true},
		{"number", `{"sel:synthetic":1}`, false, true},
		{"unknown field", `{"extra":true}`, false, true},
		{"trailing value", `{} {}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, err := decodeLoadtestInput([]byte(tc.payload))
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, input.Synthetic)
		})
	}
}
