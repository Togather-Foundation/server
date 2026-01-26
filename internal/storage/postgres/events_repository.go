package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// escapeILIKEPattern escapes special characters in ILIKE patterns to prevent SQL injection.
// PostgreSQL ILIKE uses % and _ as wildcards, and \ as escape character.
func escapeILIKEPattern(pattern string) string {
	if pattern == "" {
		return ""
	}
	// Escape backslashes first, then % and _
	escaped := strings.ReplaceAll(pattern, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	return escaped
}

var _ events.Repository = (*EventRepository)(nil)

type EventRepository struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type eventRow struct {
	ID             string
	ULID           string
	Name           string
	Description    *string
	LicenseURL     *string
	LicenseStatus  *string
	DedupHash      *string
	LifecycleState string
	EventDomain    string
	OrganizerID    *string
	PrimaryVenueID *string
	VirtualURL     *string
	ImageURL       *string
	PublicURL      *string
	Confidence     *float64
	QualityScore   *int32
	Keywords       []string
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
	StartTime      pgtype.Timestamptz
}

func (r *EventRepository) List(ctx context.Context, filters events.Filters, paginationArgs events.Pagination) (events.ListResult, error) {
	queryer := r.queryer()

	var cursorTimestamp *time.Time
	var cursorULID *string
	if strings.TrimSpace(paginationArgs.After) != "" {
		cursor, err := pagination.DecodeEventCursor(paginationArgs.After)
		if err != nil {
			return events.ListResult{}, err
		}
		value := cursor.Timestamp.UTC()
		cursorTimestamp = &value
		ulid := strings.ToUpper(cursor.ULID)
		cursorULID = &ulid
	}

	limit := paginationArgs.Limit
	if limit <= 0 {
		limit = 50
	}
	limitPlusOne := limit + 1

	var keywordArray any
	if len(filters.Keywords) > 0 {
		keywordArray = filters.Keywords
	}

	// Escape ILIKE patterns to prevent SQL injection
	escapedCity := escapeILIKEPattern(filters.City)
	escapedRegion := escapeILIKEPattern(filters.Region)
	escapedQuery := escapeILIKEPattern(filters.Query)

	rows, err := queryer.Query(ctx, `
SELECT e.id, e.ulid, e.name, e.description, e.license_url, e.license_status, e.dedup_hash,
	   e.lifecycle_state, e.event_domain, e.organizer_id, e.primary_venue_id,
	   e.virtual_url, e.image_url, e.public_url, e.confidence, e.quality_score,
	   e.keywords, e.created_at, e.updated_at, o.start_time
	  FROM events e
  JOIN event_occurrences o ON o.event_id = e.id
  LEFT JOIN places p ON p.id = COALESCE(o.venue_id, e.primary_venue_id)
  LEFT JOIN organizations org ON org.id = e.organizer_id
  WHERE ($1::timestamptz IS NULL OR o.start_time >= $1::timestamptz)
    AND ($2::timestamptz IS NULL OR o.start_time <= $2::timestamptz)
    AND ($3 = '' OR p.address_locality ILIKE '%' || $3 || '%' ESCAPE '\')
    AND ($4 = '' OR p.address_region ILIKE '%' || $4 || '%' ESCAPE '\')
    AND ($5 = '' OR p.ulid = $5)
    AND ($6 = '' OR org.ulid = $6)
    AND ($7 = '' OR e.lifecycle_state = $7)
    AND ($8 = '' OR e.event_domain = $8)
    AND ($9 = '' OR (e.name ILIKE '%' || $9 || '%' ESCAPE '\' OR e.description ILIKE '%' || $9 || '%' ESCAPE '\'))
    AND (coalesce(cardinality($10::text[]), 0) = 0 OR e.keywords && $10::text[])
    AND (
      $11::timestamptz IS NULL OR
      o.start_time > $11::timestamptz OR
      (o.start_time = $11::timestamptz AND e.ulid > $12)
    )
 ORDER BY o.start_time ASC, e.ulid ASC
 LIMIT $13
`,
		filters.StartDate,
		filters.EndDate,
		escapedCity,
		escapedRegion,
		filters.VenueULID,
		filters.OrganizerULID,
		filters.LifecycleState,
		filters.Domain,
		escapedQuery,
		keywordArray,
		cursorTimestamp,
		cursorULID,
		limitPlusOne,
	)
	if err != nil {
		return events.ListResult{}, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	items := make([]events.Event, 0, limitPlusOne)
	for rows.Next() {
		var row eventRow
		if err := rows.Scan(
			&row.ID,
			&row.ULID,
			&row.Name,
			&row.Description,
			&row.LicenseURL,
			&row.LicenseStatus,
			&row.DedupHash,
			&row.LifecycleState,
			&row.EventDomain,
			&row.OrganizerID,
			&row.PrimaryVenueID,
			&row.VirtualURL,
			&row.ImageURL,
			&row.PublicURL,
			&row.Confidence,
			&row.QualityScore,
			&row.Keywords,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.StartTime,
		); err != nil {
			return events.ListResult{}, fmt.Errorf("scan events: %w", err)
		}
		event := events.Event{
			ID:             row.ID,
			ULID:           row.ULID,
			Name:           row.Name,
			Description:    derefString(row.Description),
			LicenseURL:     derefString(row.LicenseURL),
			LicenseStatus:  derefString(row.LicenseStatus),
			DedupHash:      derefString(row.DedupHash),
			LifecycleState: row.LifecycleState,
			EventDomain:    row.EventDomain,
			OrganizerID:    row.OrganizerID,
			PrimaryVenueID: row.PrimaryVenueID,
			VirtualURL:     derefString(row.VirtualURL),
			ImageURL:       derefString(row.ImageURL),
			PublicURL:      derefString(row.PublicURL),
			Confidence:     row.Confidence,
			QualityScore:   intPtr(row.QualityScore),
			Keywords:       row.Keywords,
			CreatedAt:      time.Time{},
			UpdatedAt:      time.Time{},
		}
		if row.CreatedAt.Valid {
			event.CreatedAt = row.CreatedAt.Time
		}
		if row.UpdatedAt.Valid {
			event.UpdatedAt = row.UpdatedAt.Time
		}
		if row.StartTime.Valid {
			event.Occurrences = []events.Occurrence{{StartTime: row.StartTime.Time}}
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return events.ListResult{}, fmt.Errorf("iterate events: %w", err)
	}

	result := events.ListResult{}
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		if len(last.Occurrences) > 0 {
			result.NextCursor = pagination.EncodeEventCursor(last.Occurrences[0].StartTime, last.ULID)
		}
	}
	result.Events = items
	return result, nil
}

func (r *EventRepository) GetByULID(ctx context.Context, ulid string) (*events.Event, error) {
	queryer := r.queryer()

	rows, err := queryer.Query(ctx, `
SELECT e.id, e.ulid, e.name, e.description, e.license_url, e.license_status, e.dedup_hash,
	   e.lifecycle_state, e.event_domain, e.organizer_id, e.primary_venue_id,
	   e.virtual_url, e.image_url, e.public_url, e.confidence, e.quality_score,
	   e.keywords, e.federation_uri, e.created_at, e.updated_at, o.id, o.start_time, o.end_time, o.timezone, o.door_time, o.venue_id, o.virtual_url
	  FROM events e
	  LEFT JOIN event_occurrences o ON o.event_id = e.id
	 WHERE e.ulid = $1
 ORDER BY o.start_time ASC
`, ulid)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	defer rows.Close()

	var event *events.Event
	for rows.Next() {
		var (
			eventID        string
			eventULID      string
			name           string
			description    *string
			licenseURL     *string
			licenseStatus  *string
			dedupHash      *string
			lifecycleState string
			eventDomain    string
			organizerID    *string
			primaryVenueID *string
			virtualURL     *string
			imageURL       *string
			publicURL      *string
			confidence     *float64
			qualityScore   *int32
			keywords       []string
			federationURI  *string
			createdAt      pgtype.Timestamptz
			updatedAt      pgtype.Timestamptz
			occurrenceID   *string
			startTime      pgtype.Timestamptz
			endTime        pgtype.Timestamptz
			timezone       *string
			doorTime       pgtype.Timestamptz
			venueID        *string
			occurrenceURL  *string
		)
		if err := rows.Scan(
			&eventID,
			&eventULID,
			&name,
			&description,
			&licenseURL,
			&licenseStatus,
			&dedupHash,
			&lifecycleState,
			&eventDomain,
			&organizerID,
			&primaryVenueID,
			&virtualURL,
			&imageURL,
			&publicURL,
			&confidence,
			&qualityScore,
			&keywords,
			&federationURI,
			&createdAt,
			&updatedAt,
			&occurrenceID,
			&startTime,
			&endTime,
			&timezone,
			&doorTime,
			&venueID,
			&occurrenceURL,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		if event == nil {
			event = &events.Event{
				ID:             eventID,
				ULID:           eventULID,
				Name:           name,
				Description:    derefString(description),
				LicenseURL:     derefString(licenseURL),
				LicenseStatus:  derefString(licenseStatus),
				DedupHash:      derefString(dedupHash),
				LifecycleState: lifecycleState,
				EventDomain:    eventDomain,
				OrganizerID:    organizerID,
				PrimaryVenueID: primaryVenueID,
				VirtualURL:     derefString(virtualURL),
				ImageURL:       derefString(imageURL),
				PublicURL:      derefString(publicURL),
				Confidence:     confidence,
				QualityScore:   intPtr(qualityScore),
				Keywords:       keywords,
				FederationURI:  federationURI,
				CreatedAt:      time.Time{},
				UpdatedAt:      time.Time{},
			}
			if createdAt.Valid {
				event.CreatedAt = createdAt.Time
			}
			if updatedAt.Valid {
				event.UpdatedAt = updatedAt.Time
			}
		}

		if occurrenceID != nil {
			occ := events.Occurrence{
				ID:         *occurrenceID,
				Timezone:   derefString(timezone),
				VenueID:    venueID,
				VirtualURL: occurrenceURL,
			}
			if startTime.Valid {
				occ.StartTime = startTime.Time
			}
			if endTime.Valid {
				value := endTime.Time
				occ.EndTime = &value
			}
			if doorTime.Valid {
				value := doorTime.Time
				occ.DoorTime = &value
			}
			event.Occurrences = append(event.Occurrences, occ)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event: %w", err)
	}
	if event == nil {
		return nil, events.ErrNotFound
	}
	return event, nil
}

func (r *EventRepository) Create(ctx context.Context, params events.EventCreateParams) (*events.Event, error) {
	queryer := r.queryer()

	row := queryer.QueryRow(ctx, `
INSERT INTO events (
	ulid,
	name,
	description,
	lifecycle_state,
	event_domain,
	organizer_id,
	primary_venue_id,
	virtual_url,
	image_url,
	public_url,
	keywords,
	license_url,
	license_status,
	confidence,
	quality_score
) VALUES (
	$1,
	$2,
	NULLIF($3, ''),
	$4,
	$5,
	$6,
	$7,
	NULLIF($8, ''),
	NULLIF($9, ''),
	NULLIF($10, ''),
	$11,
	$12,
	$13,
	$14,
	$15
)
RETURNING id, ulid, name, description, license_url, license_status, dedup_hash,
	  lifecycle_state, event_domain, organizer_id, primary_venue_id,
	  virtual_url, image_url, public_url, confidence, quality_score,
	  keywords, created_at, updated_at
`,
		params.ULID,
		params.Name,
		params.Description,
		params.LifecycleState,
		params.EventDomain,
		params.OrganizerID,
		params.PrimaryVenueID,
		params.VirtualURL,
		params.ImageURL,
		params.PublicURL,
		params.Keywords,
		params.LicenseURL,
		params.LicenseStatus,
		params.Confidence,
		params.QualityScore,
	)

	var data eventRow
	if err := row.Scan(
		&data.ID,
		&data.ULID,
		&data.Name,
		&data.Description,
		&data.LicenseURL,
		&data.LicenseStatus,
		&data.DedupHash,
		&data.LifecycleState,
		&data.EventDomain,
		&data.OrganizerID,
		&data.PrimaryVenueID,
		&data.VirtualURL,
		&data.ImageURL,
		&data.PublicURL,
		&data.Confidence,
		&data.QualityScore,
		&data.Keywords,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("create event: %w", err)
	}

	event := &events.Event{
		ID:             data.ID,
		ULID:           data.ULID,
		Name:           data.Name,
		Description:    derefString(data.Description),
		LicenseURL:     derefString(data.LicenseURL),
		LicenseStatus:  derefString(data.LicenseStatus),
		DedupHash:      derefString(data.DedupHash),
		LifecycleState: data.LifecycleState,
		EventDomain:    data.EventDomain,
		OrganizerID:    data.OrganizerID,
		PrimaryVenueID: data.PrimaryVenueID,
		VirtualURL:     derefString(data.VirtualURL),
		ImageURL:       derefString(data.ImageURL),
		PublicURL:      derefString(data.PublicURL),
		Confidence:     data.Confidence,
		QualityScore:   intPtr(data.QualityScore),
		Keywords:       data.Keywords,
	}
	if data.CreatedAt.Valid {
		event.CreatedAt = data.CreatedAt.Time
	}
	if data.UpdatedAt.Valid {
		event.UpdatedAt = data.UpdatedAt.Time
	}
	return event, nil
}

func (r *EventRepository) CreateOccurrence(ctx context.Context, params events.OccurrenceCreateParams) error {
	queryer := r.queryer()

	_, err := queryer.Exec(ctx, `
INSERT INTO event_occurrences (
	event_id,
	start_time,
	end_time,
	timezone,
	door_time,
	venue_id,
	virtual_url
) VALUES ($1, $2, $3, $4, $5, $6, $7)
`,
		params.EventID,
		params.StartTime,
		params.EndTime,
		params.Timezone,
		params.DoorTime,
		params.VenueID,
		params.VirtualURL,
	)
	if err != nil {
		return fmt.Errorf("create occurrence: %w", err)
	}
	return nil
}

func (r *EventRepository) CreateSource(ctx context.Context, params events.EventSourceCreateParams) error {
	queryer := r.queryer()

	_, err := queryer.Exec(ctx, `
INSERT INTO event_sources (
	event_id,
	source_id,
	source_url,
	source_event_id,
	payload,
	payload_hash,
	confidence
) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7)
`,
		params.EventID,
		params.SourceID,
		params.SourceURL,
		params.SourceEventID,
		params.Payload,
		params.PayloadHash,
		params.Confidence,
	)
	if err != nil {
		return fmt.Errorf("create event source: %w", err)
	}
	return nil
}

func (r *EventRepository) FindBySourceExternalID(ctx context.Context, sourceID string, sourceEventID string) (*events.Event, error) {
	if strings.TrimSpace(sourceID) == "" || strings.TrimSpace(sourceEventID) == "" {
		return nil, events.ErrNotFound
	}
	queryer := r.queryer()
	row := queryer.QueryRow(ctx, `
SELECT e.id, e.ulid, e.name, e.description, e.license_url, e.license_status, e.dedup_hash,
	   e.lifecycle_state, e.event_domain, e.organizer_id, e.primary_venue_id,
	   e.virtual_url, e.image_url, e.public_url, e.keywords, e.created_at, e.updated_at
  FROM events e
  JOIN event_sources es ON es.event_id = e.id
 WHERE es.source_id = $1 AND es.source_event_id = $2
 LIMIT 1
`, sourceID, sourceEventID)

	var data eventRow
	if err := row.Scan(
		&data.ID,
		&data.ULID,
		&data.Name,
		&data.Description,
		&data.LicenseURL,
		&data.LicenseStatus,
		&data.DedupHash,
		&data.LifecycleState,
		&data.EventDomain,
		&data.OrganizerID,
		&data.PrimaryVenueID,
		&data.VirtualURL,
		&data.ImageURL,
		&data.PublicURL,
		&data.Keywords,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("find event by source: %w", err)
	}

	event := &events.Event{
		ID:             data.ID,
		ULID:           data.ULID,
		Name:           data.Name,
		Description:    derefString(data.Description),
		LicenseURL:     derefString(data.LicenseURL),
		LicenseStatus:  derefString(data.LicenseStatus),
		DedupHash:      derefString(data.DedupHash),
		LifecycleState: data.LifecycleState,
		EventDomain:    data.EventDomain,
		OrganizerID:    data.OrganizerID,
		PrimaryVenueID: data.PrimaryVenueID,
		VirtualURL:     derefString(data.VirtualURL),
		ImageURL:       derefString(data.ImageURL),
		PublicURL:      derefString(data.PublicURL),
		Keywords:       data.Keywords,
	}
	if data.CreatedAt.Valid {
		event.CreatedAt = data.CreatedAt.Time
	}
	if data.UpdatedAt.Valid {
		event.UpdatedAt = data.UpdatedAt.Time
	}
	return event, nil
}

func (r *EventRepository) FindByDedupHash(ctx context.Context, dedupHash string) (*events.Event, error) {
	if strings.TrimSpace(dedupHash) == "" {
		return nil, events.ErrNotFound
	}
	queryer := r.queryer()
	row := queryer.QueryRow(ctx, `
SELECT e.id, e.ulid, e.name, e.description, e.license_url, e.license_status, e.dedup_hash,
	   e.lifecycle_state, e.event_domain, e.organizer_id, e.primary_venue_id,
	   e.virtual_url, e.image_url, e.public_url, e.keywords, e.created_at, e.updated_at
  FROM events e
 WHERE e.dedup_hash = $1
 LIMIT 1
`, dedupHash)

	var data eventRow
	if err := row.Scan(
		&data.ID,
		&data.ULID,
		&data.Name,
		&data.Description,
		&data.LicenseURL,
		&data.LicenseStatus,
		&data.DedupHash,
		&data.LifecycleState,
		&data.EventDomain,
		&data.OrganizerID,
		&data.PrimaryVenueID,
		&data.VirtualURL,
		&data.ImageURL,
		&data.PublicURL,
		&data.Keywords,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("find event by dedup hash: %w", err)
	}

	event := &events.Event{
		ID:             data.ID,
		ULID:           data.ULID,
		Name:           data.Name,
		Description:    derefString(data.Description),
		LicenseURL:     derefString(data.LicenseURL),
		LicenseStatus:  derefString(data.LicenseStatus),
		DedupHash:      derefString(data.DedupHash),
		LifecycleState: data.LifecycleState,
		EventDomain:    data.EventDomain,
		OrganizerID:    data.OrganizerID,
		PrimaryVenueID: data.PrimaryVenueID,
		VirtualURL:     derefString(data.VirtualURL),
		ImageURL:       derefString(data.ImageURL),
		PublicURL:      derefString(data.PublicURL),
		Keywords:       data.Keywords,
	}
	if data.CreatedAt.Valid {
		event.CreatedAt = data.CreatedAt.Time
	}
	if data.UpdatedAt.Valid {
		event.UpdatedAt = data.UpdatedAt.Time
	}
	return event, nil
}

func (r *EventRepository) GetOrCreateSource(ctx context.Context, params events.SourceLookupParams) (string, error) {
	queryer := r.queryer()
	row := queryer.QueryRow(ctx, `
SELECT id
  FROM sources
 WHERE base_url = $1
 LIMIT 1
`, strings.TrimSpace(params.BaseURL))

	var id string
	if err := row.Scan(&id); err == nil {
		return id, nil
	} else if err != pgx.ErrNoRows {
		return "", fmt.Errorf("get source: %w", err)
	}

	row = queryer.QueryRow(ctx, `
INSERT INTO sources (
	name,
	source_type,
	base_url,
	license_url,
	license_type,
	trust_level
) VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6)
RETURNING id
`,
		params.Name,
		params.SourceType,
		params.BaseURL,
		params.LicenseURL,
		params.LicenseType,
		params.TrustLevel,
	)
	if err := row.Scan(&id); err != nil {
		return "", fmt.Errorf("create source: %w", err)
	}
	return id, nil
}

func (r *EventRepository) GetIdempotencyKey(ctx context.Context, key string) (*events.IdempotencyKey, error) {
	queryer := r.queryer()
	row := queryer.QueryRow(ctx, `
SELECT key, request_hash, event_id, event_ulid
  FROM idempotency_keys
 WHERE key = $1
`, strings.TrimSpace(key))

	var (
		id     *string
		ulid   *string
		result events.IdempotencyKey
	)
	if err := row.Scan(&result.Key, &result.RequestHash, &id, &ulid); err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("get idempotency key: %w", err)
	}
	result.EventID = id
	result.EventULID = ulid
	return &result, nil
}

func (r *EventRepository) InsertIdempotencyKey(ctx context.Context, params events.IdempotencyKeyCreateParams) (*events.IdempotencyKey, error) {
	queryer := r.queryer()
	row := queryer.QueryRow(ctx, `
INSERT INTO idempotency_keys (key, request_hash, event_id, event_ulid)
VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, ''))
RETURNING key, request_hash, event_id, event_ulid
`,
		params.Key,
		params.RequestHash,
		params.EventID,
		params.EventULID,
	)
	var (
		id     *string
		ulid   *string
		result events.IdempotencyKey
	)
	if err := row.Scan(&result.Key, &result.RequestHash, &id, &ulid); err != nil {
		return nil, fmt.Errorf("insert idempotency key: %w", err)
	}
	result.EventID = id
	result.EventULID = ulid
	return &result, nil
}

func (r *EventRepository) UpdateIdempotencyKeyEvent(ctx context.Context, key string, eventID string, eventULID string) error {
	queryer := r.queryer()
	command, err := queryer.Exec(ctx, `
UPDATE idempotency_keys
   SET event_id = $2, event_ulid = $3
 WHERE key = $1
`, strings.TrimSpace(key), eventID, eventULID)
	if err != nil {
		return fmt.Errorf("update idempotency key: %w", err)
	}
	if command.RowsAffected() == 0 {
		return events.ErrNotFound
	}
	return nil
}

func (r *EventRepository) UpsertPlace(ctx context.Context, params events.PlaceCreateParams) (*events.PlaceRecord, error) {
	queryer := r.queryer()

	// If federation_uri is present, upsert by federation_uri; otherwise by ulid
	var row pgx.Row
	if params.FederationURI != nil && *params.FederationURI != "" {
		row = queryer.QueryRow(ctx, `
INSERT INTO places (ulid, name, address_locality, address_region, address_country, federation_uri)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (federation_uri) WHERE federation_uri IS NOT NULL
  DO UPDATE SET 
    name = EXCLUDED.name,
    address_locality = EXCLUDED.address_locality,
    address_region = EXCLUDED.address_region,
    address_country = EXCLUDED.address_country
RETURNING id, ulid
`,
			params.ULID,
			params.Name,
			params.AddressLocality,
			params.AddressRegion,
			params.AddressCountry,
			params.FederationURI,
		)
	} else {
		row = queryer.QueryRow(ctx, `
INSERT INTO places (ulid, name, address_locality, address_region, address_country, federation_uri)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (ulid)
  DO UPDATE SET 
    name = EXCLUDED.name,
    address_locality = EXCLUDED.address_locality,
    address_region = EXCLUDED.address_region,
    address_country = EXCLUDED.address_country
RETURNING id, ulid
`,
			params.ULID,
			params.Name,
			params.AddressLocality,
			params.AddressRegion,
			params.AddressCountry,
			params.FederationURI,
		)
	}

	var record events.PlaceRecord
	if err := row.Scan(&record.ID, &record.ULID); err != nil {
		return nil, fmt.Errorf("upsert place: %w", err)
	}
	return &record, nil
}

func (r *EventRepository) UpsertOrganization(ctx context.Context, params events.OrganizationCreateParams) (*events.OrganizationRecord, error) {
	queryer := r.queryer()

	// If federation_uri is present, upsert by federation_uri; otherwise by ulid
	var row pgx.Row
	if params.FederationURI != nil && *params.FederationURI != "" {
		row = queryer.QueryRow(ctx, `
INSERT INTO organizations (ulid, name, address_locality, address_region, address_country, federation_uri)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (federation_uri) WHERE federation_uri IS NOT NULL
  DO UPDATE SET 
    name = EXCLUDED.name,
    address_locality = EXCLUDED.address_locality,
    address_region = EXCLUDED.address_region,
    address_country = EXCLUDED.address_country
RETURNING id, ulid
`,
			params.ULID,
			params.Name,
			params.AddressLocality,
			params.AddressRegion,
			params.AddressCountry,
			params.FederationURI,
		)
	} else {
		row = queryer.QueryRow(ctx, `
INSERT INTO organizations (ulid, name, address_locality, address_region, address_country, federation_uri)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (ulid)
  DO UPDATE SET 
    name = EXCLUDED.name,
    address_locality = EXCLUDED.address_locality,
    address_region = EXCLUDED.address_region,
    address_country = EXCLUDED.address_country
RETURNING id, ulid
`,
			params.ULID,
			params.Name,
			params.AddressLocality,
			params.AddressRegion,
			params.AddressCountry,
			params.FederationURI,
		)
	}

	var record events.OrganizationRecord
	if err := row.Scan(&record.ID, &record.ULID); err != nil {
		return nil, fmt.Errorf("upsert organization: %w", err)
	}
	return &record, nil
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (r *EventRepository) queryer() queryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}

// BeginTx starts a new transaction and returns a transaction-scoped repository
func (r *EventRepository) BeginTx(ctx context.Context) (events.Repository, events.TxCommitter, error) {
	if r.tx != nil {
		return nil, nil, fmt.Errorf("repository already in transaction")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}

	txRepo := &EventRepository{
		pool: r.pool,
		tx:   tx,
	}

	return txRepo, &txCommitter{tx: tx}, nil
}

// txCommitter implements events.TxCommitter
type txCommitter struct {
	tx pgx.Tx
}

func (tc *txCommitter) Commit(ctx context.Context) error {
	return tc.tx.Commit(ctx)
}

func (tc *txCommitter) Rollback(ctx context.Context) error {
	return tc.tx.Rollback(ctx)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intPtr(value *int32) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}

// UpdateEvent updates an event by ULID with the provided parameters
func (r *EventRepository) UpdateEvent(ctx context.Context, ulid string, params events.UpdateEventParams) (*events.Event, error) {
	queries := Queries{db: r.queryer()}

	// Build update parameters for SQLc
	updateParams := UpdateEventParams{
		Ulid: ulid,
	}

	if params.Name != nil {
		updateParams.Name = pgtype.Text{String: *params.Name, Valid: true}
	}
	if params.Description != nil {
		updateParams.Description = pgtype.Text{String: *params.Description, Valid: true}
	}
	if params.LifecycleState != nil {
		updateParams.LifecycleState = pgtype.Text{String: *params.LifecycleState, Valid: true}
	}
	if params.ImageURL != nil {
		updateParams.ImageUrl = pgtype.Text{String: *params.ImageURL, Valid: true}
	}
	if params.PublicURL != nil {
		updateParams.PublicUrl = pgtype.Text{String: *params.PublicURL, Valid: true}
	}
	if params.EventDomain != nil {
		updateParams.EventDomain = pgtype.Text{String: *params.EventDomain, Valid: true}
	}
	if len(params.Keywords) > 0 {
		updateParams.Keywords = params.Keywords
	}

	row, err := queries.UpdateEvent(ctx, updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("update event: %w", err)
	}

	// Map back to domain Event (convert pgtype values to native Go types)
	var eventID string
	_ = row.ID.Scan(&eventID)

	event := &events.Event{
		ID:             eventID,
		ULID:           row.Ulid,
		Name:           row.Name,
		Description:    row.Description.String,
		LifecycleState: row.LifecycleState,
		EventDomain:    row.EventDomain.String,
		ImageURL:       row.ImageUrl.String,
		PublicURL:      row.PublicUrl.String,
		Keywords:       row.Keywords,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}

	return event, nil
}

// SoftDeleteEvent marks an event as deleted
func (r *EventRepository) SoftDeleteEvent(ctx context.Context, ulid string, reason string) error {
	queries := Queries{db: r.queryer()}

	err := queries.SoftDeleteEvent(ctx, SoftDeleteEventParams{
		Ulid:           ulid,
		DeletionReason: pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return events.ErrNotFound
		}
		return fmt.Errorf("soft delete event: %w", err)
	}

	return nil
}

// MergeEvents merges a duplicate event into a primary event
func (r *EventRepository) MergeEvents(ctx context.Context, duplicateULID string, primaryULID string) error {
	queries := Queries{db: r.queryer()}

	err := queries.MergeEventIntoDuplicate(ctx, MergeEventIntoDuplicateParams{
		Ulid:   duplicateULID,
		Ulid_2: primaryULID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return events.ErrNotFound
		}
		return fmt.Errorf("merge events: %w", err)
	}

	return nil
}

// CreateTombstone creates a tombstone record for a deleted event
func (r *EventRepository) CreateTombstone(ctx context.Context, params events.TombstoneCreateParams) error {
	queries := Queries{db: r.queryer()}

	var eventIDUUID pgtype.UUID
	if err := eventIDUUID.Scan(params.EventID); err != nil {
		return fmt.Errorf("invalid event ID: %w", err)
	}

	var supersededBy pgtype.Text
	if params.SupersededBy != nil {
		supersededBy = pgtype.Text{String: *params.SupersededBy, Valid: true}
	}

	err := queries.CreateEventTombstone(ctx, CreateEventTombstoneParams{
		EventID:         eventIDUUID,
		EventUri:        params.EventURI,
		DeletedAt:       pgtype.Timestamptz{Time: params.DeletedAt, Valid: true},
		DeletionReason:  pgtype.Text{String: params.Reason, Valid: params.Reason != ""},
		SupersededByUri: supersededBy,
		Payload:         params.Payload,
	})
	if err != nil {
		return fmt.Errorf("create tombstone: %w", err)
	}

	return nil
}

// GetTombstoneByEventID retrieves the tombstone for a deleted event by UUID
func (r *EventRepository) GetTombstoneByEventID(ctx context.Context, eventID string) (*events.Tombstone, error) {
	queries := Queries{db: r.queryer()}

	var eventIDUUID pgtype.UUID
	if err := eventIDUUID.Scan(eventID); err != nil {
		return nil, fmt.Errorf("invalid event ID: %w", err)
	}

	row, err := queries.GetEventTombstoneByEventID(ctx, eventIDUUID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("get tombstone: %w", err)
	}

	var supersededBy *string
	if row.SupersededByUri.Valid {
		supersededBy = &row.SupersededByUri.String
	}

	tombstone := &events.Tombstone{
		ID:           row.ID.String(),
		EventID:      eventID,
		EventURI:     row.EventUri,
		DeletedAt:    row.DeletedAt.Time,
		Reason:       row.DeletionReason.String,
		SupersededBy: supersededBy,
		Payload:      row.Payload,
	}

	return tombstone, nil
}

// GetTombstoneByEventULID retrieves the tombstone for a deleted event by ULID
func (r *EventRepository) GetTombstoneByEventULID(ctx context.Context, eventULID string) (*events.Tombstone, error) {
	queries := Queries{db: r.queryer()}

	row, err := queries.GetEventTombstoneByEventULID(ctx, eventULID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("get tombstone by ulid: %w", err)
	}

	var supersededBy *string
	if row.SupersededByUri.Valid {
		supersededBy = &row.SupersededByUri.String
	}

	tombstone := &events.Tombstone{
		ID:           row.ID.String(),
		EventID:      row.EventID.String(),
		EventURI:     row.EventUri,
		DeletedAt:    row.DeletedAt.Time,
		Reason:       row.DeletionReason.String,
		SupersededBy: supersededBy,
		Payload:      row.Payload,
	}

	return tombstone, nil
}
