package events

import (
	"context"
	"errors"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
)

// visibilityRepositoryStub embeds Repository so only the methods under test
// need real implementations.
type visibilityRepositoryStub struct {
	Repository
	listedWith Filters
	getResult  *Event
	getErr     error
}

func (s *visibilityRepositoryStub) List(_ context.Context, filters Filters, _ Pagination) (ListResult, error) {
	s.listedWith = filters
	return ListResult{}, nil
}

func (s *visibilityRepositoryStub) GetPublicByULID(_ context.Context, _ string) (*Event, error) {
	return s.getResult, s.getErr
}

func TestPubliclyVisible(t *testing.T) {
	t.Parallel()

	visible := []string{"published", "postponed", "rescheduled", "sold_out", "cancelled", "completed"}
	for _, state := range visible {
		state := state
		t.Run("visible_"+state, func(t *testing.T) {
			t.Parallel()
			if !PubliclyVisible(state) {
				t.Fatalf("PubliclyVisible(%q) = false, want true", state)
			}
		})
	}

	hidden := []string{"draft", "pending_review", "deleted", "", "unknown", "ACTIVE"}
	for _, state := range hidden {
		state := state
		t.Run("hidden_"+state, func(t *testing.T) {
			t.Parallel()
			if PubliclyVisible(state) {
				t.Fatalf("PubliclyVisible(%q) = true, want false", state)
			}
		})
	}
}

func TestListPublicForcesPublicOnlyFilter(t *testing.T) {
	t.Parallel()

	repo := &visibilityRepositoryStub{}
	service := NewService(repo)

	result, err := service.ListPublic(context.Background(), Filters{LifecycleState: "cancelled"}, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("ListPublic returned error: %v", err)
	}
	if len(result.Events) != 0 || result.NextCursor != "" {
		t.Fatalf("expected empty result, got %+v", result)
	}
	if !repo.listedWith.PublicOnly {
		t.Fatal("ListPublic must set Filters.PublicOnly = true")
	}
	if repo.listedWith.LifecycleState != "cancelled" {
		t.Fatalf("ListPublic must preserve caller filters, got %q", repo.listedWith.LifecycleState)
	}
}

func TestListPublicHonoursGeographicBoundary(t *testing.T) {
	t.Parallel()

	repo := &visibilityRepositoryStub{}
	service := NewService(repo)
	service.WithGeographicBoundaryConfig(config.GeographicBoundaryConfig{Localities: []string{"Toronto"}})

	result, err := service.ListPublic(context.Background(), Filters{City: "Ottawa"}, Pagination{})
	if err != nil {
		t.Fatalf("ListPublic returned error: %v", err)
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected empty result outside boundary, got %+v", result)
	}
}

func TestGetPublicByULIDHidesNonPublicStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state     string
		wantErr   bool
		wantState string
	}{
		{state: "published", wantState: "published"},
		{state: "cancelled", wantState: "cancelled"},
		{state: "draft", wantErr: true},
		{state: "pending_review", wantErr: true},
		{state: "deleted", wantState: "deleted"}, // 410 tombstone path needs the row
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()
			repo := &visibilityRepositoryStub{getResult: &Event{ULID: "01J0KXMQZ8RPXJPN8J9Q6TK0WP", LifecycleState: tt.state}}
			service := NewService(repo)

			event, err := service.GetPublicByULID(context.Background(), "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
			if tt.wantErr {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("GetPublicByULID(%q) error = %v, want ErrNotFound", tt.state, err)
				}
				if event != nil {
					t.Fatalf("GetPublicByULID(%q) returned event despite error", tt.state)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetPublicByULID(%q) returned error: %v", tt.state, err)
			}
			if event == nil || event.LifecycleState != tt.wantState {
				t.Fatalf("GetPublicByULID(%q) = %+v, want state %q", tt.state, event, tt.wantState)
			}
		})
	}
}

func TestGetPublicByULIDPropagatesNotFound(t *testing.T) {
	t.Parallel()

	repo := &visibilityRepositoryStub{getErr: ErrNotFound}
	service := NewService(repo)

	_, err := service.GetPublicByULID(context.Background(), "01J0KXMQZ8RPXJPN8J9Q6TK0WP")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
