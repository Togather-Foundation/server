package events

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("event not found")

var ErrConflict = errors.New("event conflict")

// ErrOccurrenceOverlap is returned when a new occurrence would overlap an existing one.
var ErrOccurrenceOverlap = errors.New("occurrence overlaps an existing occurrence")

// Consolidation error sentinels
var ErrConsolidateNoRetire = errors.New("consolidate: retire list is required and must contain at least one event ULID")
var ErrConsolidateBothEventFields = errors.New("consolidate: event and event_ulid are mutually exclusive — supply one or the other")
var ErrConsolidateNoEventField = errors.New("consolidate: one of event or event_ulid is required")
var ErrConsolidateCanonicalInRetire = errors.New("consolidate: canonical event ULID cannot be in the retire list")
var ErrConsolidateRetiredAlreadyDeleted = errors.New("consolidate: one or more retire targets are already deleted")

// ErrLastOccurrence is returned when an admin attempts to delete the last remaining
// occurrence on an event; doing so would leave the event in an invalid state.
var ErrLastOccurrence = errors.New("cannot delete the last occurrence of an event")

// ErrOccurrenceLocationRequired is returned when an occurrence has neither a venue_id
// nor a virtual_url, violating the occurrence_location_required DB constraint.
var ErrOccurrenceLocationRequired = errors.New("occurrence must have a venue or virtual URL")

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
	PrimaryVenueID      *string // UUID from events.primary_venue_id (for DB operations)
	PrimaryVenueULID    *string // ULID from places.ulid (for URI building)
	PrimaryVenueName    *string // Name from places.name (for display/payload reconstruction)
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
	VenueID       *string // UUID from event_occurrences.venue_id (for DB operations)
	VenueULID     *string // ULID from places.ulid (for URI building)
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
	Availability  string
}

// OccurrenceUpdateParams holds PATCH-semantic updates for a single occurrence.
// Pointer fields that are nil are left unchanged. Fields that can be explicitly
// cleared (end_time, door_time, venue_id, virtual_url, price_min, price_max)
// use a Set/Value pair: Set=true, Value=nil means "set to NULL".
type OccurrenceUpdateParams struct {
	StartTime     *time.Time
	EndTime       *time.Time
	EndTimeSet    bool
	Timezone      *string
	DoorTime      *time.Time
	DoorTimeSet   bool
	VenueID       *string
	VenueIDSet    bool
	VirtualURL    *string
	VirtualURLSet bool
	TicketURL     *string
	TicketURLSet  bool
	PriceMin      *float64
	PriceMinSet   bool
	PriceMax      *float64
	PriceMaxSet   bool
	PriceCurrency *string
	Availability  *string
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
	// GetPlaceByULID looks up a place row by its ULID and returns its UUID and ULID.
	// Used to resolve occurrence venueId canonical URIs (which carry the ULID) to the
	// underlying DB UUID required by event_occurrences.venue_id.
	// Returns ErrNotFound when no matching place row exists.
	GetPlaceByULID(ctx context.Context, ulid string) (*PlaceRecord, error)
	UpsertOrganization(ctx context.Context, params OrganizationCreateParams) (*OrganizationRecord, error)

	// Trust level queries for auto-merge
	GetSourceTrustLevel(ctx context.Context, eventID string) (int, error)
	GetSourceTrustLevelBySourceID(ctx context.Context, sourceID string) (int, error)

	// Near-duplicate detection (Layer 2)
	FindNearDuplicates(ctx context.Context, venueID string, startTime time.Time, eventName string, threshold float64) ([]NearDuplicateCandidate, error)

	// Cross-week series detection: checks for an existing published/pending event
	// at the same venue with the same normalized name and a start date 7–21 days
	// away and the same time-of-day (±30 min).
	FindSeriesCompanion(ctx context.Context, params SeriesCompanionQuery) (*CrossWeekCompanion, error)

	// Rollback aborts the current transaction. Idempotent — safe to call even if
	// no transaction is active. Used by callers that run read-only queries inside a
	// transaction and need to recover from query errors without corrupting the
	// transaction state.
	Rollback(ctx context.Context) error

	// Place/Organization fuzzy dedup (Layer 3)
	FindSimilarPlaces(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarPlaceCandidate, error)
	FindSimilarOrganizations(ctx context.Context, name string, locality string, region string, threshold float64) ([]SimilarOrgCandidate, error)
	MergePlaces(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error)
	MergeOrganizations(ctx context.Context, duplicateID string, primaryID string) (*MergeResult, error)

	// Occurrence overlap check: returns true if [startTime, endTime) overlaps any
	// existing occurrence on the event identified by eventID (UUID).
	// endTime may be nil, in which case a point-in-time check is used (start < existing_end).
	CheckOccurrenceOverlap(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time) (bool, error)

	// CheckOccurrenceOverlapExcluding is like CheckOccurrenceOverlap but excludes the
	// occurrence identified by excludeOccurrenceID (UUID) from the check. Used when
	// updating an existing occurrence so it doesn't conflict with itself.
	CheckOccurrenceOverlapExcluding(ctx context.Context, eventID string, startTime time.Time, endTime *time.Time, excludeOccurrenceID string) (bool, error)

	// LockEventForUpdate acquires a row-level lock on the event row identified by
	// eventID (UUID) using SELECT ... FOR UPDATE. Must be called inside a transaction.
	// Use this to serialise concurrent operations that read-then-write the same event
	// (e.g. add-occurrence overlap check + insert).
	LockEventForUpdate(ctx context.Context, eventID string) error

	// InsertOccurrence inserts a new occurrence row and returns the created occurrence.
	// It differs from CreateOccurrence (which returns only an error) in that it returns
	// the full Occurrence domain object including the generated UUID.
	InsertOccurrence(ctx context.Context, params OccurrenceCreateParams) (*Occurrence, error)

	// GetOccurrenceByID fetches a single occurrence row by its UUID, scoped to the given event.
	// Returns ErrNotFound if no matching row exists or the occurrence belongs to a different event.
	GetOccurrenceByID(ctx context.Context, eventID string, occurrenceID string) (*Occurrence, error)

	// UpdateOccurrence applies a PATCH-semantic partial update to an occurrence, scoped to the given event.
	// Only fields with non-nil (or Set=true) values are written.
	// Returns the updated Occurrence, or ErrNotFound if the row does not exist or belongs to a different event.
	UpdateOccurrence(ctx context.Context, eventID string, occurrenceID string, params OccurrenceUpdateParams) (*Occurrence, error)

	// DeleteOccurrenceByID deletes a single occurrence row by its UUID, scoped to the given event.
	// Returns ErrNotFound if no matching row exists or the occurrence belongs to a different event.
	DeleteOccurrenceByID(ctx context.Context, eventID string, occurrenceID string) error

	// CountOccurrences returns the number of occurrences for the event identified
	// by eventID (UUID). Used to enforce the last-occurrence guard.
	CountOccurrences(ctx context.Context, eventID string) (int64, error)

	// Admin operations
	UpdateOccurrenceDates(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error
	DeleteOccurrencesByEventULID(ctx context.Context, eventULID string) error
	UpdateEvent(ctx context.Context, ulid string, params UpdateEventParams) (*Event, error)
	SoftDeleteEvent(ctx context.Context, ulid string, reason string) error
	MergeEvents(ctx context.Context, duplicateULID string, primaryULID string) error
	CreateTombstone(ctx context.Context, params TombstoneCreateParams) error
	GetTombstoneByEventID(ctx context.Context, eventID string) (*Tombstone, error)
	GetTombstoneByEventULID(ctx context.Context, eventULID string) (*Tombstone, error)

	// Not-duplicate tracking (suppresses re-flagging during near-duplicate detection)
	InsertNotDuplicate(ctx context.Context, eventIDa string, eventIDb string, createdBy string) error
	IsNotDuplicate(ctx context.Context, eventIDa string, eventIDb string) (bool, error)

	// DismissPendingReviewsByEventULIDs batch-dismisses all pending review queue entries
	// for the given event ULIDs. Returns the IDs of dismissed entries.
	// Used by consolidation to clean up review entries for retired events.
	DismissPendingReviewsByEventULIDs(ctx context.Context, eventULIDs []string, reviewedBy string) ([]int, error)

	// Review Queue operations
	FindReviewByDedup(ctx context.Context, sourceID *string, externalID *string, dedupHash *string) (*ReviewQueueEntry, error)
	CreateReviewQueueEntry(ctx context.Context, params ReviewQueueCreateParams) (*ReviewQueueEntry, error)
	UpdateReviewQueueEntry(ctx context.Context, id int, params ReviewQueueUpdateParams) (*ReviewQueueEntry, error)
	GetReviewQueueEntry(ctx context.Context, id int) (*ReviewQueueEntry, error)
	// LockReviewQueueEntryForUpdate acquires a row-level lock on the review queue
	// row identified by id using SELECT ... FOR UPDATE.  Must be called inside a
	// transaction.  Returns (nil, ErrNotFound) if the row no longer exists.
	// Use this to serialise concurrent admin actions on the same review entry so
	// that only the first request proceeds and the second sees the updated status.
	LockReviewQueueEntryForUpdate(ctx context.Context, id int) (*ReviewQueueEntry, error)
	GetPendingReviewByEventUlid(ctx context.Context, eventULID string) (*ReviewQueueEntry, error)
	// GetPendingReviewByEventUlidAndDuplicateUlid looks up the pending review for
	// eventULID that is specifically linked to duplicateULID via duplicate_of_event_id.
	// This is the precise companion-review lookup needed during add-occurrence
	// consolidation: when an event has multiple pending reviews, only the one
	// corresponding to the counterpart of the current consolidation pair is returned.
	// Returns nil (not ErrNotFound) if no matching pending review exists.
	GetPendingReviewByEventUlidAndDuplicateUlid(ctx context.Context, eventULID string, duplicateULID string) (*ReviewQueueEntry, error)
	UpdateReviewWarnings(ctx context.Context, id int, warnings []byte) error
	DismissCompanionWarningMatch(ctx context.Context, companionULID string, eventULID string) error
	// DismissWarningMatchByReviewID removes any potential_duplicate match referencing
	// eventULID from the specific review row identified by id. Strictly narrower than
	// DismissCompanionWarningMatch: targets exactly one row to avoid affecting unrelated
	// pending reviews on the same companion event.
	DismissWarningMatchByReviewID(ctx context.Context, id int, eventULID string) error
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
	Name string // canonical stored name; populated by GetPlaceByULID for mismatch detection
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
	ID                   int
	EventID              string // UUID (events.id)
	EventULID            string // ULID (events.ulid) - populated via JOIN
	OriginalPayload      []byte
	NormalizedPayload    []byte
	Warnings             []byte
	SourceID             *string
	SourceExternalID     *string
	DedupHash            *string
	DuplicateOfEventID   *string // UUID of the event this is a duplicate of (for merge workflow)
	DuplicateOfEventULID *string // ULID of the duplicate event (from JOIN)
	EventStartTime       time.Time
	EventEndTime         *time.Time
	Status               string
	ReviewedBy           *string
	ReviewedAt           *time.Time
	ReviewNotes          *string
	RejectionReason      *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ReviewQueueCreateParams contains data for creating a review queue entry
type ReviewQueueCreateParams struct {
	EventID            string
	OriginalPayload    []byte
	NormalizedPayload  []byte
	Warnings           []byte
	SourceID           *string
	SourceExternalID   *string
	DedupHash          *string
	EventStartTime     time.Time
	EventEndTime       *time.Time
	DuplicateOfEventID *string // UUID of the event this is a potential duplicate of (for merge workflow)
}

// ReviewQueueUpdateParams contains data for updating a review queue entry
type ReviewQueueUpdateParams struct {
	OriginalPayload    *[]byte
	NormalizedPayload  *[]byte
	Warnings           *[]byte
	DuplicateOfEventID *string // UUID of the event this is a duplicate of (nil = no change)
	ClearDuplicateOf   bool    // if true, set duplicate_of_event_id = NULL (takes precedence over DuplicateOfEventID)
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
	StartDate  string  // ISO-8601 from event_occurrences.start_time (may be empty)
	EndDate    string  // ISO-8601 from event_occurrences.end_time (may be empty)
	VenueName  string  // from places.name (may be empty)
}

// SimilarPlaceCandidate represents an existing place that may be a fuzzy duplicate
type SimilarPlaceCandidate struct {
	ID              string  // UUID of the candidate place
	ULID            string  // ULID of the candidate place
	Name            string  // Name of the candidate place
	Similarity      float64 // Trigram similarity score (0.0 to 1.0)
	AddressStreet   *string // street_address (may be nil)
	AddressLocality *string // address_locality (may be nil)
	AddressRegion   *string // address_region (may be nil)
	PostalCode      *string // postal_code (may be nil)
	URL             *string // url (may be nil)
	Telephone       *string // telephone (may be nil)
	Email           *string // email (may be nil)
}

// SimilarOrgCandidate represents an existing organization that may be a fuzzy duplicate
type SimilarOrgCandidate struct {
	ID              string  // UUID of the candidate organization
	ULID            string  // ULID of the candidate organization
	Name            string  // Name of the candidate organization
	Similarity      float64 // Trigram similarity score (0.0 to 1.0)
	AddressLocality *string // address_locality (may be nil)
	AddressRegion   *string // address_region (may be nil)
	URL             *string // url (may be nil)
	Telephone       *string // telephone (may be nil)
	Email           *string // email (may be nil)
}
