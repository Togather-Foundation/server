package organizations

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("organization not found")

type Organization struct {
	ID          string
	ULID        string
	Name        string
	LegalName   string
	Description string
	URL         string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Filters struct {
	Query string
}

type Pagination struct {
	Limit int
	After string
}

type ListResult struct {
	Organizations []Organization
	NextCursor    string
}

type Repository interface {
	List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error)
	GetByULID(ctx context.Context, ulid string) (*Organization, error)
}
