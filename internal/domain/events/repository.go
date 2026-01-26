package events

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("event not found")

var ErrConflict = errors.New("event conflict")

type Event struct {
	ID             string
	ULID           string
	Name           string
	Description    string
	LicenseURL     string
	LicenseStatus  string
	DedupHash      string
	LifecycleState string
	EventDomain    string
	OrganizerID    *string
	PrimaryVenueID *string
	VirtualURL     string
	ImageURL       string
	PublicURL      string
	Confidence     *float64
	QualityScore   *int
	Keywords       []string
	FederationURI  *string
	Occurrences    []Occurrence
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Occurrence struct {
	ID         string
	StartTime  time.Time
	EndTime    *time.Time
	Timezone   string
	DoorTime   *time.Time
	VenueID    *string
	VirtualURL *string
}

type EventCreateParams struct {
	ULID           string
	Name           string
	Description    string
	LifecycleState string
	EventDomain    string
	OrganizerID    *string
	PrimaryVenueID *string
	VirtualURL     string
	ImageURL       string
	PublicURL      string
	Keywords       []string
	LicenseURL     string
	LicenseStatus  string
	DedupHash      string
	Confidence     *float64
	QualityScore   *int
	OriginNodeID   *string
}

type OccurrenceCreateParams struct {
	EventID    string
	StartTime  time.Time
	EndTime    *time.Time
	Timezone   string
	DoorTime   *time.Time
	VenueID    *string
	VirtualURL *string
}

type EventSourceCreateParams struct {
	EventID       string
	SourceID      string
	SourceURL     string
	SourceEventID string
	Payload       []byte
	PayloadHash   string
	Confidence    *float64
}

type IdempotencyKey struct {
	Key         string
	RequestHash string
	EventID     *string
	EventULID   *string
}

type IdempotencyKeyCreateParams struct {
	Key         string
	RequestHash string
	EventID     string
	EventULID   string
}

type SourceLookupParams struct {
	Name        string
	SourceType  string
	BaseURL     string
	LicenseURL  string
	LicenseType string
	TrustLevel  int
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
	Create(ctx context.Context, params EventCreateParams) (*Event, error)
	CreateOccurrence(ctx context.Context, params OccurrenceCreateParams) error
	CreateSource(ctx context.Context, params EventSourceCreateParams) error
	FindBySourceExternalID(ctx context.Context, sourceID string, sourceEventID string) (*Event, error)
	FindByDedupHash(ctx context.Context, dedupHash string) (*Event, error)
	GetOrCreateSource(ctx context.Context, params SourceLookupParams) (string, error)
	GetIdempotencyKey(ctx context.Context, key string) (*IdempotencyKey, error)
	InsertIdempotencyKey(ctx context.Context, params IdempotencyKeyCreateParams) (*IdempotencyKey, error)
	UpdateIdempotencyKeyEvent(ctx context.Context, key string, eventID string, eventULID string) error
	UpsertPlace(ctx context.Context, params PlaceCreateParams) (*PlaceRecord, error)
	UpsertOrganization(ctx context.Context, params OrganizationCreateParams) (*OrganizationRecord, error)

	// Admin operations
	UpdateEvent(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error)
	SoftDeleteEvent(ctx context.Context, ulid string, reason string) error
	MergeEvents(ctx context.Context, duplicateULID string, primaryULID string) error
	CreateTombstone(ctx context.Context, params TombstoneCreateParams) error
	GetTombstoneByEventID(ctx context.Context, eventID string) (*Tombstone, error)
	GetTombstoneByEventULID(ctx context.Context, eventULID string) (*Tombstone, error)

	// Transaction support
	BeginTx(ctx context.Context) (Repository, TxCommitter, error)
}

// TxCommitter provides transaction commit/rollback functionality
type TxCommitter interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Tombstone represents a deleted event record
type Tombstone struct {
	ID           string
	EventID      string
	EventURI     string
	DeletedAt    time.Time
	Reason       string
	SupersededBy *string
	Payload      []byte
}

// TombstoneCreateParams contains data for creating a tombstone
type TombstoneCreateParams struct {
	EventID      string
	EventURI     string
	DeletedAt    time.Time
	Reason       string
	SupersededBy *string
	Payload      []byte
}

type PlaceCreateParams struct {
	ULID            string
	Name            string
	AddressLocality string
	AddressRegion   string
	AddressCountry  string
}

type PlaceRecord struct {
	ID   string
	ULID string
}

type OrganizationCreateParams struct {
	ULID            string
	Name            string
	AddressLocality string
	AddressRegion   string
	AddressCountry  string
}

type OrganizationRecord struct {
	ID   string
	ULID string
}
