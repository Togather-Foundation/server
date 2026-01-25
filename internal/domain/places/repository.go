package places

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("place not found")

type Place struct {
	ID          string
	ULID        string
	Name        string
	Description string
	City        string
	Region      string
	Country     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Filters struct {
	City  string
	Query string
}

type Pagination struct {
	Limit int
	After string
}

type ListResult struct {
	Places     []Place
	NextCursor string
}

type Repository interface {
	List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error)
	GetByULID(ctx context.Context, ulid string) (*Place, error)
}
