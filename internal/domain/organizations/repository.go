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
	Lifecycle   string
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
	SoftDelete(ctx context.Context, ulid string, reason string) error
	CreateTombstone(ctx context.Context, params TombstoneCreateParams) error
	GetTombstoneByULID(ctx context.Context, ulid string) (*Tombstone, error)
}

// Tombstone represents a deleted organization record
type Tombstone struct {
	ID           string
	OrgID        string
	OrgURI       string
	DeletedAt    time.Time
	Reason       string
	SupersededBy *string
	Payload      []byte
}

// TombstoneCreateParams contains data for creating a tombstone
type TombstoneCreateParams struct {
	OrgID        string
	OrgURI       string
	DeletedAt    time.Time
	Reason       string
	SupersededBy *string
	Payload      []byte
}
