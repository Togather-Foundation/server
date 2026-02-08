package events

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestErrPreviouslyRejected(t *testing.T) {
	reviewedAt := time.Date(2026, 2, 7, 14, 30, 0, 0, time.UTC)

	err := ErrPreviouslyRejected{
		Reason:     "Cannot determine correct time. Please fix the data before resubmitting.",
		ReviewedAt: reviewedAt,
		ReviewedBy: "admin@togather.ca",
	}

	// Test Error() message
	expectedMsg := "event previously reviewed and rejected: Cannot determine correct time. Please fix the data before resubmitting."
	require.Equal(t, expectedMsg, err.Error())

	// Test that it can be detected with errors.As
	var target ErrPreviouslyRejected
	require.True(t, errors.As(err, &target))
	require.Equal(t, "Cannot determine correct time. Please fix the data before resubmitting.", target.Reason)
	require.Equal(t, reviewedAt, target.ReviewedAt)
	require.Equal(t, "admin@togather.ca", target.ReviewedBy)
}

func TestErrPreviouslyRejectedAsInterface(t *testing.T) {
	// Test that ErrPreviouslyRejected implements error interface
	var err error = ErrPreviouslyRejected{
		Reason:     "Test rejection",
		ReviewedAt: time.Now(),
		ReviewedBy: "test@example.com",
	}

	require.NotNil(t, err)
	require.Contains(t, err.Error(), "Test rejection")
}
