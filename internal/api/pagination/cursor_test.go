package pagination

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeEventCursor(t *testing.T) {
	timestamp := time.Date(2024, 1, 2, 3, 4, 5, 6, time.UTC)

	cursor := EncodeEventCursor(timestamp, "  01hyx3kqw7ertv9xnbm2p8qjzf ")

	decoded, err := DecodeEventCursor(cursor)

	require.NoError(t, err)
	require.Equal(t, timestamp, decoded.Timestamp)
	require.Equal(t, "01HYX3KQW7ERTV9XNBM2P8QJZF", decoded.ULID)
}

func TestDecodeEventCursorErrors(t *testing.T) {
	_, err := DecodeEventCursor("")

	require.ErrorIs(t, err, ErrInvalidCursor)

	_, err = DecodeEventCursor("not-base64")

	require.ErrorIs(t, err, ErrInvalidCursor)

	_, err = DecodeEventCursor("bm90LWFfdmFsaWRfZm9ybWF0")

	require.ErrorIs(t, err, ErrInvalidCursor)
}

func TestEncodeDecodeChangeCursor(t *testing.T) {
	cursor := EncodeChangeCursor(42)

	decoded, err := DecodeChangeCursor(cursor)

	require.NoError(t, err)
	require.Equal(t, int64(42), decoded)
}

func TestDecodeChangeCursorErrors(t *testing.T) {
	_, err := DecodeChangeCursor("")

	require.ErrorIs(t, err, ErrInvalidCursor)

	_, err = DecodeChangeCursor("not-base64")

	require.ErrorIs(t, err, ErrInvalidCursor)

	_, err = DecodeChangeCursor("bm90LXNlcV82")

	require.ErrorIs(t, err, ErrInvalidCursor)

	cursor := EncodeChangeCursor(-1)

	_, err = DecodeChangeCursor(cursor)

	require.ErrorIs(t, err, ErrInvalidCursor)
}
