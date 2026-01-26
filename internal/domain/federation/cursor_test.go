package federation

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/stretchr/testify/require"
)

// TestChangeCursorEncoding verifies encoding and decoding of change feed cursors
func TestChangeCursorEncoding(t *testing.T) {
	tests := []struct {
		name     string
		sequence int64
		wantErr  bool
	}{
		{
			name:     "zero sequence",
			sequence: 0,
			wantErr:  false,
		},
		{
			name:     "small sequence",
			sequence: 42,
			wantErr:  false,
		},
		{
			name:     "large sequence",
			sequence: 9223372036854775807, // max int64
			wantErr:  false,
		},
		{
			name:     "typical sequence",
			sequence: 123456789,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded := pagination.EncodeChangeCursor(tt.sequence)
			require.NotEmpty(t, encoded, "encoded cursor should not be empty")

			// Verify it's valid base64
			_, err := base64.RawURLEncoding.DecodeString(encoded)
			require.NoError(t, err, "encoded cursor should be valid base64")

			// Decode
			decoded, err := pagination.DecodeChangeCursor(encoded)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.sequence, decoded, "decoded sequence should match original")
			}
		})
	}
}

// TestChangeCursorDecoding verifies error handling for invalid cursors
func TestChangeCursorDecoding(t *testing.T) {
	tests := []struct {
		name    string
		cursor  string
		wantErr bool
	}{
		{
			name:    "empty cursor",
			cursor:  "",
			wantErr: true,
		},
		{
			name:    "whitespace cursor",
			cursor:  "   ",
			wantErr: true,
		},
		{
			name:    "invalid base64",
			cursor:  "not-base64!@#$",
			wantErr: true,
		},
		{
			name:    "missing seq prefix",
			cursor:  base64.RawURLEncoding.EncodeToString([]byte("12345")),
			wantErr: true,
		},
		{
			name:    "negative sequence",
			cursor:  base64.RawURLEncoding.EncodeToString([]byte("seq_-1")),
			wantErr: true,
		},
		{
			name:    "non-numeric sequence",
			cursor:  base64.RawURLEncoding.EncodeToString([]byte("seq_abc")),
			wantErr: true,
		},
		{
			name:    "seq prefix only",
			cursor:  base64.RawURLEncoding.EncodeToString([]byte("seq_")),
			wantErr: true,
		},
		{
			name:    "valid cursor",
			cursor:  pagination.EncodeChangeCursor(42),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pagination.DecodeChangeCursor(tt.cursor)
			if tt.wantErr {
				require.Error(t, err, "should return error for invalid cursor")
				require.ErrorIs(t, err, pagination.ErrInvalidCursor, "should return ErrInvalidCursor")
			} else {
				require.NoError(t, err, "should not return error for valid cursor")
			}
		})
	}
}

// TestChangeCursorRoundTrip verifies encode-decode round-trip preserves data
func TestChangeCursorRoundTrip(t *testing.T) {
	sequences := []int64{0, 1, 100, 999, 1234567890, 9223372036854775807}

	for _, seq := range sequences {
		t.Run("sequence_"+string(rune(seq)), func(t *testing.T) {
			encoded := pagination.EncodeChangeCursor(seq)
			decoded, err := pagination.DecodeChangeCursor(encoded)

			require.NoError(t, err, "round-trip should succeed")
			require.Equal(t, seq, decoded, "round-trip should preserve value")
		})
	}
}

// TestChangeCursorFormat verifies cursor format consistency
func TestChangeCursorFormat(t *testing.T) {
	// Encode a known sequence
	encoded := pagination.EncodeChangeCursor(42)

	// Decode and verify internal format
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	require.NoError(t, err)

	// Should follow seq_<number> format
	require.Equal(t, "seq_42", string(decoded), "cursor should follow seq_<number> format")
}

// TestChangeCursorStability verifies cursors are stable across invocations
func TestChangeCursorStability(t *testing.T) {
	seq := int64(12345)

	// Encode multiple times
	encoded1 := pagination.EncodeChangeCursor(seq)
	encoded2 := pagination.EncodeChangeCursor(seq)
	encoded3 := pagination.EncodeChangeCursor(seq)

	// All should be identical
	require.Equal(t, encoded1, encoded2, "encoding should be deterministic")
	require.Equal(t, encoded2, encoded3, "encoding should be deterministic")
}

// TestEventCursorEncoding verifies encoding and decoding of event list cursors
func TestEventCursorEncoding(t *testing.T) {
	tests := []struct {
		name      string
		timestamp time.Time
		ulid      string
		wantErr   bool
	}{
		{
			name:      "typical event cursor",
			timestamp: time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC),
			ulid:      "01HYX3KQW7ERTV9XNBM2P8QJZF",
			wantErr:   false,
		},
		{
			name:      "lowercase ulid",
			timestamp: time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC),
			ulid:      "01hyx3kqw7ertv9xnbm2p8qjzf",
			wantErr:   false,
		},
		{
			name:      "ulid with spaces",
			timestamp: time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC),
			ulid:      "  01HYX3KQW7ERTV9XNBM2P8QJZF  ",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded := pagination.EncodeEventCursor(tt.timestamp, tt.ulid)
			require.NotEmpty(t, encoded, "encoded cursor should not be empty")

			// Verify it's valid base64
			_, err := base64.RawURLEncoding.DecodeString(encoded)
			require.NoError(t, err, "encoded cursor should be valid base64")

			// Decode
			decoded, err := pagination.DecodeEventCursor(encoded)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.timestamp.UTC(), decoded.Timestamp, "decoded timestamp should match original")

				// ULIDs should be normalized to uppercase and trimmed
				expectedULID := strings.ToUpper(strings.TrimSpace(tt.ulid))
				require.Equal(t, expectedULID, decoded.ULID, "decoded ULID should be normalized (uppercase, trimmed)")
			}
		})
	}
}

// TestEventCursorDecoding verifies error handling for invalid event cursors
func TestEventCursorDecoding(t *testing.T) {
	tests := []struct {
		name    string
		cursor  string
		wantErr bool
	}{
		{
			name:    "empty cursor",
			cursor:  "",
			wantErr: true,
		},
		{
			name:    "invalid base64",
			cursor:  "not-base64!@#$",
			wantErr: true,
		},
		{
			name:    "missing colon separator",
			cursor:  base64.RawURLEncoding.EncodeToString([]byte("123456789")),
			wantErr: true,
		},
		{
			name:    "invalid timestamp",
			cursor:  base64.RawURLEncoding.EncodeToString([]byte("abc:01HYX3KQW7ERTV9XNBM2P8QJZF")),
			wantErr: true,
		},
		{
			name:    "empty ulid",
			cursor:  base64.RawURLEncoding.EncodeToString([]byte("1720636800000000000:")),
			wantErr: true,
		},
		{
			name:    "valid cursor",
			cursor:  pagination.EncodeEventCursor(time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC), "01HYX3KQW7ERTV9XNBM2P8QJZF"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pagination.DecodeEventCursor(tt.cursor)
			if tt.wantErr {
				require.Error(t, err, "should return error for invalid cursor")
				require.ErrorIs(t, err, pagination.ErrInvalidCursor, "should return ErrInvalidCursor")
			} else {
				require.NoError(t, err, "should not return error for valid cursor")
			}
		})
	}
}

// TestEventCursorRoundTrip verifies encode-decode round-trip preserves data
func TestEventCursorRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		timestamp time.Time
		ulid      string
	}{
		{
			name:      "typical event",
			timestamp: time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC),
			ulid:      "01HYX3KQW7ERTV9XNBM2P8QJZF",
		},
		{
			name:      "recent event",
			timestamp: time.Now().UTC(),
			ulid:      "01JBN5W3M2KSQR8ZVTPX4YF7HA",
		},
		{
			name:      "old event",
			timestamp: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			ulid:      "01E0000000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := pagination.EncodeEventCursor(tt.timestamp, tt.ulid)
			decoded, err := pagination.DecodeEventCursor(encoded)

			require.NoError(t, err, "round-trip should succeed")
			require.Equal(t, tt.timestamp.UTC(), decoded.Timestamp, "round-trip should preserve timestamp")
			require.Contains(t, decoded.ULID, tt.ulid, "round-trip should preserve ULID")
		})
	}
}

// TestEventCursorFormat verifies cursor format consistency
func TestEventCursorFormat(t *testing.T) {
	ts := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	ulid := "01HYX3KQW7ERTV9XNBM2P8QJZF"

	// Encode
	encoded := pagination.EncodeEventCursor(ts, ulid)

	// Decode and verify internal format
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	require.NoError(t, err)

	// Should follow <timestamp_unix_nano>:<ULID> format
	decodedStr := string(decoded)
	require.Contains(t, decodedStr, ":", "cursor should contain colon separator")
	require.Contains(t, decodedStr, ulid, "cursor should contain ULID")
}

// TestCursorComparison verifies cursors can be used for ordering
func TestCursorComparison(t *testing.T) {
	// Create cursors with increasing timestamps
	ts1 := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 7, 10, 20, 0, 0, 0, time.UTC)
	ts3 := time.Date(2026, 7, 10, 21, 0, 0, 0, time.UTC)

	cursor1 := pagination.EncodeEventCursor(ts1, "01HYX3KQW7ERTV9XNBM2P8QJZF")
	cursor2 := pagination.EncodeEventCursor(ts2, "01HYX3KQW8ERTV9XNBM2P8QJZG")
	cursor3 := pagination.EncodeEventCursor(ts3, "01HYX3KQW9ERTV9XNBM2P8QJZH")

	// Decode all
	decoded1, err := pagination.DecodeEventCursor(cursor1)
	require.NoError(t, err)
	decoded2, err := pagination.DecodeEventCursor(cursor2)
	require.NoError(t, err)
	decoded3, err := pagination.DecodeEventCursor(cursor3)
	require.NoError(t, err)

	// Verify ordering
	require.True(t, decoded1.Timestamp.Before(decoded2.Timestamp), "cursor1 should be before cursor2")
	require.True(t, decoded2.Timestamp.Before(decoded3.Timestamp), "cursor2 should be before cursor3")
}

// BenchmarkChangeCursorEncode benchmarks change cursor encoding
func BenchmarkChangeCursorEncode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = pagination.EncodeChangeCursor(int64(i))
	}
}

// BenchmarkChangeCursorDecode benchmarks change cursor decoding
func BenchmarkChangeCursorDecode(b *testing.B) {
	cursor := pagination.EncodeChangeCursor(123456789)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pagination.DecodeChangeCursor(cursor)
	}
}

// BenchmarkEventCursorEncode benchmarks event cursor encoding
func BenchmarkEventCursorEncode(b *testing.B) {
	ts := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	ulid := "01HYX3KQW7ERTV9XNBM2P8QJZF"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pagination.EncodeEventCursor(ts, ulid)
	}
}

// BenchmarkEventCursorDecode benchmarks event cursor decoding
func BenchmarkEventCursorDecode(b *testing.B) {
	cursor := pagination.EncodeEventCursor(time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC), "01HYX3KQW7ERTV9XNBM2P8QJZF")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pagination.DecodeEventCursor(cursor)
	}
}
