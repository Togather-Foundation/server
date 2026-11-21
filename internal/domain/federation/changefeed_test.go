package federation

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/ids"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformSnapshotSyntheticMarker(t *testing.T) {
	service := &ChangeFeedService{}
	for _, tc := range []struct {
		name     string
		snapshot string
		marked   bool
	}{
		{name: "marked", snapshot: `{"name":"Fixture","synthetic":true}`, marked: true},
		{name: "ordinary", snapshot: `{"name":"Event","synthetic":false}`},
		{name: "legacy", snapshot: `{"name":"Event"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := service.transformSnapshotToJSONLD([]byte(tc.snapshot), "", snapshotExtras{})
			require.NoError(t, err)
			if tc.marked {
				require.Contains(t, string(snapshot), `"sel:synthetic":true`)
			} else {
				require.NotContains(t, string(snapshot), `"sel:synthetic"`)
			}
			require.NotContains(t, string(snapshot), `"synthetic"`)
		})
	}
}

func TestUuidToString(t *testing.T) {
	tests := []struct {
		name     string
		input    pgtype.UUID
		expected string
	}{
		{
			name: "valid UUID",
			input: pgtype.UUID{
				Bytes: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
				Valid: true,
			},
			expected: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name: "another valid UUID",
			input: pgtype.UUID{
				Bytes: uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
				Valid: true,
			},
			expected: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		},
		{
			name: "invalid UUID",
			input: pgtype.UUID{
				Valid: false,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ids.UUIDToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUuidToString_NotGarbled(t *testing.T) {
	// Regression test: ensure we don't return raw bytes as string
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	input := pgtype.UUID{
		Bytes: testUUID,
		Valid: true,
	}

	result := ids.UUIDToString(input)

	// Should be properly formatted, not raw bytes
	assert.NotContains(t, result, "\\x", "UUID should not contain hex escape sequences")
	assert.Len(t, result, 36, "UUID should be 36 characters (including dashes)")
	assert.Contains(t, result, "-", "UUID should contain dashes")
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", result)

	// Should NOT be the wrong format
	wrongFormat := string(testUUID[:])
	assert.NotEqual(t, wrongFormat, result, "Should not return raw byte string")
}
