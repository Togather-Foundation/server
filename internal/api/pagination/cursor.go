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
type EventCursor struct {
	Timestamp time.Time
	ULID      string
}

// EncodeEventCursor encodes the cursor as base64(ts_unix_nano:ULID).
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
