// Package pagination provides cursor-based pagination for SEL API endpoints.
//
// # Cursor Format
//
// All cursors are opaque strings that encode internal pagination state using
// base64.RawURLEncoding (URL-safe base64 without padding).
//
// Event Cursors:
//
//	Format: base64url(timestamp_unix_nano:ULID)
//	Example raw: "1706886000000000000:01HYX3KQW7ERTV9XNBM2P8QJZF"
//	Example encoded: "MTcwNjg4NjAwMDAwMDAwMDAwMDowMUhZWDNLUVc3RVJUV jlYTkJNMlA4UUpaRg"
//
// Change Feed Cursors:
//
//	Format: base64url(seq_<sequence_number>)
//	Example raw: "seq_1000"
//	Example encoded: "c2VxXzEwMDA"
//
// # Encoding Choice
//
// We use base64.RawURLEncoding for all cursors:
//   - URL-safe: No special characters (+, /, =) that need escaping in query params
//   - No padding: Cleaner appearance without trailing "=" characters
//   - Standard: Well-established RFC 4648 encoding
//
// Clients should treat cursors as opaque strings and use the next_cursor value
// from API responses as-is. Do not attempt to parse or modify cursor values.
package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidCursor = errors.New("invalid cursor")

// EventCursor encodes a timestamp + ULID for stable event ordering.
// The cursor enables consistent pagination even when events are added/removed
// during iteration.
type EventCursor struct {
	Timestamp time.Time
	ULID      string
}

// EncodeEventCursor encodes the cursor as base64url(ts_unix_nano:ULID).
//
// The encoded cursor is URL-safe and can be used directly in query parameters.
// Example usage:
//
//	cursor := pagination.EncodeEventCursor(event.CreatedAt, event.ULID)
//	// Returns: "MTcwNjg4NjAwMDAwMDAwMDAwMDowMUhZWDNLUVc3RVJUV jlYTkJNMlA4UUpaRg"
func EncodeEventCursor(timestamp time.Time, ulid string) string {
	value := fmt.Sprintf("%d:%s", timestamp.UTC().UnixNano(), strings.ToUpper(strings.TrimSpace(ulid)))
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

// DecodeEventCursor decodes base64(ts_unix_nano:ULID) into an EventCursor.
func DecodeEventCursor(cursor string) (EventCursor, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return EventCursor{}, ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return EventCursor{}, ErrInvalidCursor
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return EventCursor{}, ErrInvalidCursor
	}
	unixNano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return EventCursor{}, ErrInvalidCursor
	}
	if strings.TrimSpace(parts[1]) == "" {
		return EventCursor{}, ErrInvalidCursor
	}
	return EventCursor{Timestamp: time.Unix(0, unixNano).UTC(), ULID: strings.ToUpper(strings.TrimSpace(parts[1]))}, nil
}

// EncodeChangeCursor encodes a BIGSERIAL sequence number for change feeds.
//
// Change feed cursors are used for streaming updates and federation sync.
// The encoded cursor is URL-safe and can be used directly in query parameters.
// Example usage:
//
//	cursor := pagination.EncodeChangeCursor(1000)
//	// Returns: "c2VxXzEwMDA"
func EncodeChangeCursor(sequence int64) string {
	value := fmt.Sprintf("seq_%d", sequence)
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

// DecodeChangeCursor decodes base64(seq_<number>) into a sequence number.
func DecodeChangeCursor(cursor string) (int64, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, ErrInvalidCursor
	}
	value := string(decoded)
	if !strings.HasPrefix(value, "seq_") {
		return 0, ErrInvalidCursor
	}
	seq, err := strconv.ParseInt(strings.TrimPrefix(value, "seq_"), 10, 64)
	if err != nil || seq < 0 {
		return 0, ErrInvalidCursor
	}
	return seq, nil
}
