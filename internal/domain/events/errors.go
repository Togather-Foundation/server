package events

import (
	"fmt"
	"time"
)

// ErrPreviouslyRejected indicates that an event was previously reviewed and rejected.
// This error is returned when a source resubmits the same event with the same issues
// that were previously rejected by an admin reviewer.
type ErrPreviouslyRejected struct {
	Reason     string
	ReviewedAt time.Time
	ReviewedBy string
}

func (e ErrPreviouslyRejected) Error() string {
	return fmt.Sprintf("event previously reviewed and rejected: %s", e.Reason)
}
