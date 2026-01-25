package events

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("event not found")

type Event struct {
	ID             string
	ULID           string
	Name           string
	Description    string
	LifecycleState string
	EventDomain    string
	OrganizerID    *string
	PrimaryVenueID *string
	Keywords       []string
	Occurrences    []Occurrence
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Occurrence struct {
	ID         string
	StartTime  time.Time
	EndTime    *time.Time
	Timezone   string
	VenueID    *string
	VirtualURL *string
}

type Filters struct {
	StartDate      *time.Time
	EndDate        *time.Time
	City           string
	Region         string
	VenueULID      string
	OrganizerULID  string
	LifecycleState string
	Query          string
	Keywords       []string
	Domain         string
}

type Pagination struct {
	Limit int
	After string
}

type ListResult struct {
	Events     []Event
	NextCursor string
}

type Repository interface {
	List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error)
	GetByULID(ctx context.Context, ulid string) (*Event, error)
}
