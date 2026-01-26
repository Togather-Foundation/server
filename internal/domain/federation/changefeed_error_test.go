package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test error scenarios for change feed

func TestChangeFeedService_ErrorPaths(t *testing.T) {
	t.Run("invalid limit", func(t *testing.T) {
		repo := &mockChangeFeedRepo{}
		service := NewChangeFeedService(repo)

		_, err := service.GetChanges(context.Background(), ChangeFeedParams{
			Limit: 1001, // Exceeds max of 1000
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidLimit)
	})

	t.Run("invalid action", func(t *testing.T) {
		repo := &mockChangeFeedRepo{}
		service := NewChangeFeedService(repo)

		_, err := service.GetChanges(context.Background(), ChangeFeedParams{
			Action: "invalid",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidAction)
	})

	t.Run("invalid cursor", func(t *testing.T) {
		repo := &mockChangeFeedRepo{}
		service := NewChangeFeedService(repo)

		_, err := service.GetChanges(context.Background(), ChangeFeedParams{
			After: "invalid-cursor",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidCursor)
	})

	t.Run("database error", func(t *testing.T) {
		repo := &mockChangeFeedRepoWithError{
			err: errors.New("database connection failed"),
		}
		service := NewChangeFeedService(repo)

		_, err := service.GetChanges(context.Background(), ChangeFeedParams{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "database connection failed")
	})

	t.Run("context cancelled before query", func(t *testing.T) {
		repo := &mockChangeFeedRepo{}
		service := NewChangeFeedService(repo)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := service.GetChanges(ctx, ChangeFeedParams{})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("context timeout during query", func(t *testing.T) {
		repo := &slowChangeFeedRepo{delay: 100 * time.Millisecond}
		service := NewChangeFeedService(repo)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		_, err := service.GetChanges(ctx, ChangeFeedParams{})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("default limit applied when zero", func(t *testing.T) {
		repo := &mockChangeFeedRepo{}
		service := NewChangeFeedService(repo)

		result, err := service.GetChanges(context.Background(), ChangeFeedParams{
			Limit: 0, // Should default to 50
		})
		require.NoError(t, err)
		assert.NotNil(t, result)
		// Verify repo received limit+1 (51) for pagination check
		assert.Equal(t, int32(51), repo.lastParams.Limit)
	})

	t.Run("valid actions accepted", func(t *testing.T) {
		repo := &mockChangeFeedRepo{}
		service := NewChangeFeedService(repo)

		validActions := []string{"create", "update", "delete", ""}
		for _, action := range validActions {
			_, err := service.GetChanges(context.Background(), ChangeFeedParams{
				Action: action,
			})
			require.NoError(t, err, "action %q should be valid", action)
		}
	})
}

// Mock repository for change feed
type mockChangeFeedRepo struct {
	lastParams ListEventChangesParams
}

func (m *mockChangeFeedRepo) ListEventChanges(ctx context.Context, arg ListEventChangesParams) ([]ListEventChangesRow, error) {
	m.lastParams = arg
	return []ListEventChangesRow{}, nil
}

// Mock repository that returns errors
type mockChangeFeedRepoWithError struct {
	err error
}

func (m *mockChangeFeedRepoWithError) ListEventChanges(ctx context.Context, arg ListEventChangesParams) ([]ListEventChangesRow, error) {
	return nil, m.err
}

// Mock repository with artificial delay
type slowChangeFeedRepo struct {
	delay time.Duration
}

func (m *slowChangeFeedRepo) ListEventChanges(ctx context.Context, arg ListEventChangesParams) ([]ListEventChangesRow, error) {
	time.Sleep(m.delay)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return []ListEventChangesRow{}, nil
}
