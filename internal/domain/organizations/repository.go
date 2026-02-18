package organizations

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("organization not found")

type Organization struct {
	ID               string
	ULID             string
	Name             string
	LegalName        string
	Description      string
	URL              string
	Email            string
	Telephone        string
	AddressLocality  string
	AddressRegion    string
	AddressCountry   string
	StreetAddress    string
	PostalCode       string
	OrganizationType string
	FederationURI    string
	AlternateName    string
	Lifecycle        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CreateParams struct {
	ULID            string
	Name            string
	LegalName       string
	Description     string
	URL             string
	StreetAddress   string
	AddressLocality string
	AddressRegion   string
	PostalCode      string
	AddressCountry  string
	FederationURI   *string
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

// UpdateOrganizationParams contains fields that can be updated by admins.
// Nil pointer fields are not changed (COALESCE pattern).
type UpdateOrganizationParams struct {
	Name            *string
	Description     *string
	StreetAddress   *string
	AddressLocality *string
	AddressRegion   *string
	PostalCode      *string
	AddressCountry  *string
	Telephone       *string
	Email           *string
	URL             *string
}

type Repository interface {
	List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error)
	GetByULID(ctx context.Context, ulid string) (*Organization, error)
	// TODO(srv-d7cnu): Create removed during rebase - check if needed
	// Create(ctx context.Context, params CreateParams) (*Organization, error)
	Update(ctx context.Context, ulid string, params UpdateOrganizationParams) (*Organization, error)
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
