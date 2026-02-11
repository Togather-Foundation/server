package events

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("event not found")

var ErrConflict = errors.New("event conflict")

// ErrAlreadyMerged is returned when attempting to merge an entity that has
// already been merged into another entity by a concurrent operation.
var ErrAlreadyMerged = errors.New("entity already merged")

// MergeResult is returned by MergePlaces/MergeOrganizations to communicate
// the canonical entity ID when a concurrent merge was detected.
type MergeResult struct {
	// CanonicalID is the UUID of the entity that should be used.
	// When the merge succeeds normally, this equals the primaryID passed in.
	// When the duplicate was already merged by another goroutine, this is
	// the ID at the end of the merge chain.
	CanonicalID string
	// AlreadyMerged is true when the duplicate had already been merged
	// by a concurrent operation, making this call a no-op.
	AlreadyMerged bool
}

type Event struct {
	ID                  string
	ULID                string
	Name                string
	Description         string
	LicenseURL          string
	LicenseStatus       string
	DedupHash           string
	LifecycleState      string
	EventStatus         string
	AttendanceMode      string
	EventDomain         string
	OrganizerID         *string
	PrimaryVenueID      *string
	VirtualURL          string
	ImageURL            string
	PublicURL           string
	Confidence          *float64
	QualityScore        *int
	Keywords            []string
	InLanguage          []string
	IsAccessibleForFree *bool
	FederationURI       *string
	Occurrences         []Occurrence
	CreatedAt           time.Time
	UpdatedAt           time.Time
	PublishedAt         *time.Time
}

type Occurrence struct {
	ID            string
	StartTime     time.Time
	EndTime       *time.Time
	Timezone      string
	DoorTime      *time.Time
	VenueID       *string
	VirtualURL    *string
	TicketURL     string
	PriceMin      *float64
	PriceMax      *float64
	PriceCurrency string
	Availability  string
}

type EventCreateParams struct {
	ULID                string
	Name                string
	Description         string
	LifecycleState      string
	EventDomain         string
	OrganizerID         *string
	PrimaryVenueID      *string
	VirtualURL          string
	ImageURL            string
	PublicURL           string
	Keywords            []string
	InLanguage          []string
	IsAccessibleForFree *bool
	LicenseURL          string
	LicenseStatus       string
	DedupHash           string
	Confidence          *float64
	QualityScore        *int
	OriginNodeID        *string
}

type OccurrenceCreateParams struct {
	EventID       string
	StartTime     time.Time
	EndTime       *time.Time
	Timezone      string
	DoorTime      *time.Time
	VenueID       *string
	VirtualURL    *string
	TicketURL     *string
	PriceMin      *float64
	PriceMax      *float64
	PriceCurrency string
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

	// Trust level queries for auto-merge
	GetSourceTrustLevel(ctx context.Context, eventID string) (int, error)
	GetSourceTrustLevelBySourceID(ctx context.Context, sourceID string) (int, error)

	// Near-duplicate detection (Layer 2)
	FindNearDuplicates(ctx context.Context, venueID string, startTime time.Time, eventName string, threshold float64) ([]NearDuplicateCandidate, error)

	// Place/Organization fuzzy dedup (Layer 3)
	FindSimilarPlaces(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarPlaceCandidate, error)
	FindSimilarOrganizations(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarOrgCandidate, error)
	MergePlaces(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error)
	MergeOrganizations(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error)

	// Admin operations
	UpdateOccurrenceDates(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error
	UpdateEvent(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error)
	SoftDeleteEvent(ctx context.Context, ulid string, reason string) error
	MergeEvents(ctx context.Context, duplicateULID string, primaryULID string) error
	CreateTombstone(ctx context.Context, params TombstoneCreateParams) error
	GetTombstoneByEventID(ctx context.Context, eventID string) (*Tombstone, error)
	GetTombstoneByEventULID(ctx context.Context, eventULID string) (*Tombstone, error)

	// Not-duplicate tracking (suppresses re-flagging during near-duplicate detection)
	InsertNotDuplicate(ctx context.Context, eventIDa string, eventIDb string, createdBy string) error
	IsNotDuplicate(ctx context.Context, eventIDa string, eventIDb string) (bool, error)

	// Review Queue operations
	FindReviewByDedup(ctx context.Context, sourceID *string, externalID *string, dedupHash *string) (*ReviewQueueEntry, error)
	CreateReviewQueueEntry(ctx context.Context, params ReviewQueueCreateParams) (*ReviewQueueEntry, error)
	UpdateReviewQueueEntry(ctx context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error)
	GetReviewQueueEntry(ctx context.Context, id int) (*ReviewQueueEntry, error)
	ListReviewQueue(ctx context.Context, filters ReviewQueueFilters) (*ReviewQueueListResult, error)
	ApproveReview(ctx context.Context, id int, reviewedBy string, notes *string) (*ReviewQueueEntry, error)
	RejectReview(ctx context.Context, id int, reviewedBy string, reason string) (*ReviewQueueEntry, error)
	MergeReview(ctx context.Context, id int, reviewedBy string, primaryEventULID string) (*ReviewQueueEntry, error)
	CleanupExpiredReviews(ctx context.Context) error

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

// EntityCreateFields contains common fields for creating places and organizations
type EntityCreateFields struct {
	ULID            string
	Name            string
	StreetAddress   string
	PostalCode      string
	AddressLocality string
	AddressRegion   string
	AddressCountry  string
	FederationURI   *string
}

type PlaceCreateParams struct {
	EntityCreateFields
	Latitude  *float64
	Longitude *float64
}

type PlaceRecord struct {
	ID   string
	ULID string
}

type OrganizationCreateParams struct {
	EntityCreateFields
	Email     string
	Telephone string
	URL       string
}

type OrganizationRecord struct {
	ID   string
	ULID string
}

// ReviewQueueEntry represents an event in the review queue
type ReviewQueueEntry struct {
	ID                 int
	EventID            string // UUID (events.id)
	EventULID          string // ULID (events.ulid) - populated via JOIN
	OriginalPayload    []byte
	NormalizedPayload  []byte
	Warnings           []byte
	SourceID           *string
	SourceExternalID   *string
	DedupHash          *string
	DuplicateOfEventID *string // UUID of the event this is a duplicate of (for merge workflow)
	EventStartTime     time.Time
	EventEndTime       *time.Time
	Status             string
	ReviewedBy         *string
	ReviewedAt         *time.Time
	ReviewNotes        *string
	RejectionReason    *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ReviewQueueCreateParams contains data for creating a review queue entry
type ReviewQueueCreateParams struct {
	EventID           string
	OriginalPayload   []byte
	NormalizedPayload []byte
	Warnings          []byte
	SourceID          *string
	SourceExternalID  *string
	DedupHash         *string
	EventStartTime    time.Time
	EventEndTime      *time.Time
}

// ReviewQueueUpdateParams contains data for updating a review queue entry
type ReviewQueueUpdateParams struct {
	OriginalPayload   *[]byte
	NormalizedPayload *[]byte
	Warnings          *[]byte
}

// ReviewQueueFilters contains filters for listing review queue entries
type ReviewQueueFilters struct {
	Status     *string
	Limit      int
	NextCursor *int
}

// ReviewQueueListResult contains paginated review queue results
type ReviewQueueListResult struct {
	Entries    []ReviewQueueEntry
	NextCursor *int
	TotalCount int64 // Total count for current filter (for badge display)
}

// NearDuplicateCandidate represents an existing event that may be a near-duplicate
type NearDuplicateCandidate struct {
	ULID       string  // ULID of the candidate event
	Name       string  // Name of the candidate event
	Similarity float64 // Trigram similarity score (0.0 to 1.0)
}

// SimilarPlaceCandidate represents an existing place that may be a fuzzy duplicate
type SimilarPlaceCandidate struct {
	ID         string  // UUID of the candidate place
	ULID       string  // ULID of the candidate place
	Name       string  // Name of the candidate place
	Similarity float64 // Trigram similarity score (0.0 to 1.0)
}

// SimilarOrgCandidate represents an existing organization that may be a fuzzy duplicate
type SimilarOrgCandidate struct {
	ID         string  // UUID of the candidate organization
	ULID       string  // ULID of the candidate organization
	Name       string  // Name of the candidate organization
	Similarity float64 // Trigram similarity score (0.0 to 1.0)
}
