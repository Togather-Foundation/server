package places

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("place not found")

type Place struct {
	ID                      string
	ULID                    string
	Name                    string
	Description             string
	StreetAddress           string
	City                    string
	Region                  string
	PostalCode              string
	Country                 string
	Latitude                *float64
	Longitude               *float64
	Telephone               string
	Email                   string
	URL                     string
	MaximumAttendeeCapacity *int
	VenueType               string
	FederationURI           string
	Lifecycle               string
	DistanceKm              *float64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type Filters struct {
	City      string
	Query     string
	Sort      string  // "name" or "created_at" (default: "created_at")
	Order     string  // "asc" or "desc" (default: "asc")
	NearPlace *string // Place name for geocoding-based proximity search (mutually exclusive with Latitude/Longitude)
	Latitude  *float64
	Longitude *float64
	RadiusKm  *float64
}

type Pagination struct {
	Limit int
	After string
}

type ListResult struct {
	Places     []Place
	NextCursor string
}

// UpdatePlaceParams contains fields that can be updated by admins.
// Nil pointer fields are not changed (COALESCE pattern).
type UpdatePlaceParams struct {
	Name          *string
	Description   *string
	StreetAddress *string
	City          *string
	Region        *string
	PostalCode    *string
	Country       *string
	Telephone     *string
	Email         *string
	URL           *string
}

type Repository interface {
	List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error)
	GetByULID(ctx context.Context, ulid string) (*Place, error)
	Update(ctx context.Context, ulid string, params UpdatePlaceParams) (*Place, error)
	SoftDelete(ctx context.Context, ulid string, reason string) error
	CreateTombstone(ctx context.Context, params TombstoneCreateParams) error
	GetTombstoneByULID(ctx context.Context, ulid string) (*Tombstone, error)
}

// Tombstone represents a deleted place record
type Tombstone struct {
	ID           string
	PlaceID      string
	PlaceURI     string
	DeletedAt    time.Time
	Reason       string
	SupersededBy *string
	Payload      []byte
}

// TombstoneCreateParams contains data for creating a tombstone
type TombstoneCreateParams struct {
	PlaceID      string
	PlaceURI     string
	DeletedAt    time.Time
	Reason       string
	SupersededBy *string
	Payload      []byte
}
