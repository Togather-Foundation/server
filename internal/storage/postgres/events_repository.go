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
	ID                  string
	ULID                string
	Name                string
	Description         *string
	LicenseURL          *string
	LicenseStatus       *string
	DedupHash           *string
	LifecycleState      string
	EventStatus         *string
	AttendanceMode      *string
	EventDomain         string
	OrganizerID         *string
	PrimaryVenueID      *string
	VirtualURL          *string
	ImageURL            *string
	PublicURL           *string
	Confidence          *float64
	QualityScore        *int32
	Keywords            []string
	InLanguage          []string
	IsAccessibleForFree *bool
	CreatedAt           pgtype.Timestamptz
	UpdatedAt           pgtype.Timestamptz
	PublishedAt         pgtype.Timestamptz
	StartTime           pgtype.Timestamptz
	EndTime             pgtype.Timestamptz
	Timezone            *string
	OccVenueID          *string
	OccVirtualURL       *string
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
		limit = events.DefaultListLimit
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
SELECT id, ulid, name, description, license_url, license_status, dedup_hash,
	   lifecycle_state, event_status, attendance_mode, event_domain,
	   organizer_id, primary_venue_id,
	   virtual_url, image_url, public_url, confidence, quality_score,
	   keywords, in_language, is_accessible_for_free,
	   created_at, updated_at, published_at,
	   start_time, end_time, timezone, occ_venue_id, occ_virtual_url
  FROM (
    SELECT e.id, e.ulid, e.name, e.description, e.license_url, e.license_status, e.dedup_hash,
	       e.lifecycle_state, e.event_status, e.attendance_mode, e.event_domain,
	       e.organizer_id, e.primary_venue_id,
	       e.virtual_url, e.image_url, e.public_url, e.confidence, e.quality_score,
	       e.keywords, e.in_language, e.is_accessible_for_free,
	       e.created_at, e.updated_at, e.published_at,
	       o.start_time, o.end_time, o.timezone,
	       o.venue_id AS occ_venue_id, o.virtual_url AS occ_virtual_url,
	       row_number() OVER (PARTITION BY e.id ORDER BY o.start_time ASC, e.ulid ASC) AS row_num
	  FROM events e
  JOIN event_occurrences o ON o.event_id = e.id
  LEFT JOIN places p ON p.id = COALESCE(o.venue_id, e.primary_venue_id)
  LEFT JOIN organizations org ON org.id = e.organizer_id
  WHERE e.deleted_at IS NULL
    AND ($1::timestamptz IS NULL OR o.start_time >= $1::timestamptz)
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
  ) filtered
 WHERE row_num = 1
 ORDER BY start_time ASC, ulid ASC
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
			&row.EventStatus,
			&row.AttendanceMode,
			&row.EventDomain,
			&row.OrganizerID,
			&row.PrimaryVenueID,
			&row.VirtualURL,
			&row.ImageURL,
			&row.PublicURL,
			&row.Confidence,
			&row.QualityScore,
			&row.Keywords,
			&row.InLanguage,
			&row.IsAccessibleForFree,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.PublishedAt,
			&row.StartTime,
			&row.EndTime,
			&row.Timezone,
			&row.OccVenueID,
			&row.OccVirtualURL,
		); err != nil {
			return events.ListResult{}, fmt.Errorf("scan events: %w", err)
		}
		event := events.Event{
			ID:                  row.ID,
			ULID:                row.ULID,
			Name:                row.Name,
			Description:         derefString(row.Description),
			LicenseURL:          derefString(row.LicenseURL),
			LicenseStatus:       derefString(row.LicenseStatus),
			DedupHash:           derefString(row.DedupHash),
			LifecycleState:      row.LifecycleState,
			EventStatus:         derefString(row.EventStatus),
			AttendanceMode:      derefString(row.AttendanceMode),
			EventDomain:         row.EventDomain,
			OrganizerID:         row.OrganizerID,
			PrimaryVenueID:      row.PrimaryVenueID,
			VirtualURL:          derefString(row.VirtualURL),
			ImageURL:            derefString(row.ImageURL),
			PublicURL:           derefString(row.PublicURL),
			Confidence:          row.Confidence,
			QualityScore:        intPtr(row.QualityScore),
			Keywords:            row.Keywords,
			InLanguage:          row.InLanguage,
			IsAccessibleForFree: row.IsAccessibleForFree,
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
		}
		if row.CreatedAt.Valid {
			event.CreatedAt = row.CreatedAt.Time
		}
		if row.UpdatedAt.Valid {
			event.UpdatedAt = row.UpdatedAt.Time
		}
		if row.PublishedAt.Valid {
			value := row.PublishedAt.Time
			event.PublishedAt = &value
		}
		if row.StartTime.Valid {
			occ := events.Occurrence{
				StartTime:  row.StartTime.Time,
				Timezone:   derefString(row.Timezone),
				VenueID:    row.OccVenueID,
				VirtualURL: row.OccVirtualURL,
			}
			if row.EndTime.Valid {
				value := row.EndTime.Time
				occ.EndTime = &value
			}
			event.Occurrences = []events.Occurrence{occ}
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
	   e.lifecycle_state, e.event_status, e.attendance_mode, e.event_domain,
	   e.organizer_id, e.primary_venue_id,
	   e.virtual_url, e.image_url, e.public_url, e.confidence, e.quality_score,
	   e.keywords, e.in_language, e.is_accessible_for_free,
	   e.federation_uri, e.created_at, e.updated_at, e.published_at,
	   o.id, o.start_time, o.end_time, o.timezone, o.door_time, o.venue_id, o.virtual_url,
	   o.ticket_url, o.price_min, o.price_max, o.price_currency, o.availability
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
			eventID             string
			eventULID           string
			name                string
			description         *string
			licenseURL          *string
			licenseStatus       *string
			dedupHash           *string
			lifecycleState      string
			eventStatus         *string
			attendanceMode      *string
			eventDomain         string
			organizerID         *string
			primaryVenueID      *string
			virtualURL          *string
			imageURL            *string
			publicURL           *string
			confidence          *float64
			qualityScore        *int32
			keywords            []string
			inLanguage          []string
			isAccessibleForFree *bool
			federationURI       *string
			createdAt           pgtype.Timestamptz
			updatedAt           pgtype.Timestamptz
			publishedAt         pgtype.Timestamptz
			occurrenceID        *string
			startTime           pgtype.Timestamptz
			endTime             pgtype.Timestamptz
			timezone            *string
			doorTime            pgtype.Timestamptz
			venueID             *string
			occurrenceURL       *string
			ticketURL           *string
			priceMin            *float64
			priceMax            *float64
			priceCurrency       *string
			availability        *string
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
			&eventStatus,
			&attendanceMode,
			&eventDomain,
			&organizerID,
			&primaryVenueID,
			&virtualURL,
			&imageURL,
			&publicURL,
			&confidence,
			&qualityScore,
			&keywords,
			&inLanguage,
			&isAccessibleForFree,
			&federationURI,
			&createdAt,
			&updatedAt,
			&publishedAt,
			&occurrenceID,
			&startTime,
			&endTime,
			&timezone,
			&doorTime,
			&venueID,
			&occurrenceURL,
			&ticketURL,
			&priceMin,
			&priceMax,
			&priceCurrency,
			&availability,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		if event == nil {
			event = &events.Event{
				ID:                  eventID,
				ULID:                eventULID,
				Name:                name,
				Description:         derefString(description),
				LicenseURL:          derefString(licenseURL),
				LicenseStatus:       derefString(licenseStatus),
				DedupHash:           derefString(dedupHash),
				LifecycleState:      lifecycleState,
				EventStatus:         derefString(eventStatus),
				AttendanceMode:      derefString(attendanceMode),
				EventDomain:         eventDomain,
				OrganizerID:         organizerID,
				PrimaryVenueID:      primaryVenueID,
				VirtualURL:          derefString(virtualURL),
				ImageURL:            derefString(imageURL),
				PublicURL:           derefString(publicURL),
				Confidence:          confidence,
				QualityScore:        intPtr(qualityScore),
				Keywords:            keywords,
				InLanguage:          inLanguage,
				IsAccessibleForFree: isAccessibleForFree,
				FederationURI:       federationURI,
				CreatedAt:           time.Time{},
				UpdatedAt:           time.Time{},
			}
			if createdAt.Valid {
				event.CreatedAt = createdAt.Time
			}
			if updatedAt.Valid {
				event.UpdatedAt = updatedAt.Time
			}
			if publishedAt.Valid {
				value := publishedAt.Time
				event.PublishedAt = &value
			}
		}

		if occurrenceID != nil {
			occ := events.Occurrence{
				ID:            *occurrenceID,
				Timezone:      derefString(timezone),
				VenueID:       venueID,
				VirtualURL:    occurrenceURL,
				TicketURL:     derefString(ticketURL),
				PriceMin:      priceMin,
				PriceMax:      priceMax,
				PriceCurrency: derefString(priceCurrency),
				Availability:  derefString(availability),
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
	in_language,
	is_accessible_for_free,
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
	$15,
	$16,
	$17
)
RETURNING id, ulid, name, description, license_url, license_status, dedup_hash,
	  lifecycle_state, event_status, attendance_mode, event_domain,
	  organizer_id, primary_venue_id,
	  virtual_url, image_url, public_url, confidence, quality_score,
	  keywords, in_language, is_accessible_for_free,
	  created_at, updated_at
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
		params.InLanguage,
		params.IsAccessibleForFree,
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
		&data.EventStatus,
		&data.AttendanceMode,
		&data.EventDomain,
		&data.OrganizerID,
		&data.PrimaryVenueID,
		&data.VirtualURL,
		&data.ImageURL,
		&data.PublicURL,
		&data.Confidence,
		&data.QualityScore,
		&data.Keywords,
		&data.InLanguage,
		&data.IsAccessibleForFree,
		&data.CreatedAt,
		&data.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("create event: %w", err)
	}

	event := &events.Event{
		ID:                  data.ID,
		ULID:                data.ULID,
		Name:                data.Name,
		Description:         derefString(data.Description),
		LicenseURL:          derefString(data.LicenseURL),
		LicenseStatus:       derefString(data.LicenseStatus),
		DedupHash:           derefString(data.DedupHash),
		LifecycleState:      data.LifecycleState,
		EventStatus:         derefString(data.EventStatus),
		AttendanceMode:      derefString(data.AttendanceMode),
		EventDomain:         data.EventDomain,
		OrganizerID:         data.OrganizerID,
		PrimaryVenueID:      data.PrimaryVenueID,
		VirtualURL:          derefString(data.VirtualURL),
		ImageURL:            derefString(data.ImageURL),
		PublicURL:           derefString(data.PublicURL),
		Confidence:          data.Confidence,
		QualityScore:        intPtr(data.QualityScore),
		Keywords:            data.Keywords,
		InLanguage:          data.InLanguage,
		IsAccessibleForFree: data.IsAccessibleForFree,
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
	virtual_url,
	ticket_url,
	price_min,
	price_max,
	price_currency
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''))
`,
		params.EventID,
		params.StartTime,
		params.EndTime,
		params.Timezone,
		params.DoorTime,
		params.VenueID,
		params.VirtualURL,
		params.TicketURL,
		params.PriceMin,
		params.PriceMax,
		params.PriceCurrency,
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
 WHERE base_url IS NOT DISTINCT FROM NULLIF($1, '')
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

	// Step 1: Check if an entity already exists by normalized name + location
	// This enables reconciliation across different name variants (e.g., "DROM Taberna" vs "Drom Taberna", "Studio & Gallery" vs "Studio and Gallery")
	// Uses normalize_name() function which handles: & <-> and, punctuation, whitespace, case
	lookupRow := queryer.QueryRow(ctx, `
SELECT id, ulid FROM places 
WHERE normalized_name = normalize_name($1)
  AND COALESCE(address_locality, '') = COALESCE($2, '')
  AND COALESCE(address_region, '') = COALESCE($3, '')
LIMIT 1
`,
		params.Name,
		params.AddressLocality,
		params.AddressRegion,
	)

	var existingRecord events.PlaceRecord
	err := lookupRow.Scan(&existingRecord.ID, &existingRecord.ULID)
	if err == nil {
		// Found existing entity with same normalized name in same location - reuse it
		return &existingRecord, nil
	}
	// If error is not "no rows", something went wrong
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("lookup place by normalized name: %w", err)
	}

	// Step 2: No existing entity found - proceed with upsert by federation_uri or ulid
	var row pgx.Row
	if params.FederationURI != nil && *params.FederationURI != "" {
		row = queryer.QueryRow(ctx, `
INSERT INTO places (ulid, name, street_address, postal_code, address_locality, address_region, address_country, latitude, longitude, federation_uri)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (federation_uri) WHERE federation_uri IS NOT NULL
  DO UPDATE SET 
    name = EXCLUDED.name,
    street_address = EXCLUDED.street_address,
    postal_code = EXCLUDED.postal_code,
    address_locality = EXCLUDED.address_locality,
    address_region = EXCLUDED.address_region,
    address_country = EXCLUDED.address_country,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude
RETURNING id, ulid
`,
			params.ULID,
			params.Name,
			nilIfEmpty(params.StreetAddress),
			nilIfEmpty(params.PostalCode),
			params.AddressLocality,
			params.AddressRegion,
			params.AddressCountry,
			params.Latitude,
			params.Longitude,
			params.FederationURI,
		)
	} else {
		row = queryer.QueryRow(ctx, `
INSERT INTO places (ulid, name, street_address, postal_code, address_locality, address_region, address_country, latitude, longitude, federation_uri)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (ulid)
  DO UPDATE SET 
    name = EXCLUDED.name,
    street_address = EXCLUDED.street_address,
    postal_code = EXCLUDED.postal_code,
    address_locality = EXCLUDED.address_locality,
    address_region = EXCLUDED.address_region,
    address_country = EXCLUDED.address_country,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude
RETURNING id, ulid
`,
			params.ULID,
			params.Name,
			nilIfEmpty(params.StreetAddress),
			nilIfEmpty(params.PostalCode),
			params.AddressLocality,
			params.AddressRegion,
			params.AddressCountry,
			params.Latitude,
			params.Longitude,
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

	// Step 1: Check if an entity already exists by normalized name + location
	// This enables reconciliation across different name variants (e.g., "City of Toronto" variants, "Studio & Co" vs "Studio and Co")
	// Uses normalize_name() function which handles: & <-> and, punctuation, whitespace, case
	lookupRow := queryer.QueryRow(ctx, `
SELECT id, ulid FROM organizations 
WHERE normalized_name = normalize_name($1)
  AND COALESCE(address_locality, '') = COALESCE($2, '')
  AND COALESCE(address_region, '') = COALESCE($3, '')
LIMIT 1
`,
		params.Name,
		params.AddressLocality,
		params.AddressRegion,
	)

	var existingRecord events.OrganizationRecord
	err := lookupRow.Scan(&existingRecord.ID, &existingRecord.ULID)
	if err == nil {
		// Found existing entity with same normalized name in same location - reuse it
		return &existingRecord, nil
	}
	// If error is not "no rows", something went wrong
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("lookup organization by normalized name: %w", err)
	}

	// Step 2: No existing entity found - proceed with upsert by federation_uri or ulid
	var row pgx.Row
	if params.FederationURI != nil && *params.FederationURI != "" {
		row = queryer.QueryRow(ctx, `
INSERT INTO organizations (ulid, name, address_locality, address_region, address_country, federation_uri, email, telephone, url)
VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''))
ON CONFLICT (federation_uri) WHERE federation_uri IS NOT NULL
  DO UPDATE SET 
    name = EXCLUDED.name,
    address_locality = EXCLUDED.address_locality,
    address_region = EXCLUDED.address_region,
    address_country = EXCLUDED.address_country,
    email = COALESCE(NULLIF(EXCLUDED.email, ''), organizations.email),
    telephone = COALESCE(NULLIF(EXCLUDED.telephone, ''), organizations.telephone),
    url = COALESCE(NULLIF(EXCLUDED.url, ''), organizations.url)
RETURNING id, ulid
`,
			params.ULID,
			params.Name,
			params.AddressLocality,
			params.AddressRegion,
			params.AddressCountry,
			params.FederationURI,
			params.Email,
			params.Telephone,
			params.URL,
		)
	} else {
		row = queryer.QueryRow(ctx, `
INSERT INTO organizations (ulid, name, address_locality, address_region, address_country, federation_uri, email, telephone, url)
VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''))
ON CONFLICT (ulid)
  DO UPDATE SET 
    name = EXCLUDED.name,
    address_locality = EXCLUDED.address_locality,
    address_region = EXCLUDED.address_region,
    address_country = EXCLUDED.address_country,
    email = COALESCE(NULLIF(EXCLUDED.email, ''), organizations.email),
    telephone = COALESCE(NULLIF(EXCLUDED.telephone, ''), organizations.telephone),
    url = COALESCE(NULLIF(EXCLUDED.url, ''), organizations.url)
RETURNING id, ulid
`,
			params.ULID,
			params.Name,
			params.AddressLocality,
			params.AddressRegion,
			params.AddressCountry,
			params.FederationURI,
			params.Email,
			params.Telephone,
			params.URL,
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

// nilIfEmpty returns nil for empty strings, or a pointer to the string otherwise.
// Used to map empty Go strings to SQL NULL for optional text columns.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetSourceTrustLevel returns the highest trust level among sources linked to an event.
// Trust levels are 1-10 where higher values mean more trusted (10 = most trusted).
// Returns the default trust level of 5 if no sources are linked.
func (r *EventRepository) GetSourceTrustLevel(ctx context.Context, eventID string) (int, error) {
	var eventUUID pgtype.UUID
	if err := eventUUID.Scan(eventID); err != nil {
		return 0, fmt.Errorf("invalid event ID: %w", err)
	}

	queryer := r.queryer()
	var trustLevel int
	err := queryer.QueryRow(ctx, `
SELECT COALESCE(MAX(s.trust_level), 5)
  FROM sources s
  JOIN event_sources es ON es.source_id = s.id
 WHERE es.event_id = $1
`, eventUUID).Scan(&trustLevel)
	if err != nil {
		return 0, fmt.Errorf("get source trust level: %w", err)
	}
	return trustLevel, nil
}

// GetSourceTrustLevelBySourceID returns the trust level for a specific source.
func (r *EventRepository) GetSourceTrustLevelBySourceID(ctx context.Context, sourceID string) (int, error) {
	queryer := r.queryer()
	var trustLevel int
	err := queryer.QueryRow(ctx, `
SELECT trust_level FROM sources WHERE id = $1
`, sourceID).Scan(&trustLevel)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 5, nil // default trust level if source not found
		}
		return 0, fmt.Errorf("get source trust level by ID: %w", err)
	}
	return trustLevel, nil
}

// FindNearDuplicates finds events at the same venue on the same date with similar names.
// Uses pg_trgm similarity() for fuzzy name matching. Returns candidates above the threshold.
func (r *EventRepository) FindNearDuplicates(ctx context.Context, venueID string, startTime time.Time, eventName string, threshold float64) ([]events.NearDuplicateCandidate, error) {
	queryer := r.queryer()

	// Match events at the same venue on the same calendar date with similar names.
	// Uses pg_trgm similarity() which returns a float between 0 and 1.
	rows, err := queryer.Query(ctx, `
SELECT e.ulid, e.name, similarity(e.name, $3) AS sim
  FROM events e
  JOIN event_occurrences o ON o.event_id = e.id
 WHERE e.deleted_at IS NULL
   AND e.primary_venue_id = $1
   AND DATE(o.start_time) = DATE($2::timestamptz)
   AND similarity(e.name, $3) >= $4
 ORDER BY sim DESC
 LIMIT 5
`, venueID, startTime, eventName, threshold)
	if err != nil {
		return nil, fmt.Errorf("find near duplicates: %w", err)
	}
	defer rows.Close()

	var candidates []events.NearDuplicateCandidate
	for rows.Next() {
		var c events.NearDuplicateCandidate
		if err := rows.Scan(&c.ULID, &c.Name, &c.Similarity); err != nil {
			return nil, fmt.Errorf("scan near duplicate: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate near duplicates: %w", err)
	}

	return candidates, nil
}

// FindSimilarPlaces returns places with similar normalized names in the same locality/region.
// Uses pg_trgm similarity() against the normalized_name column, which has a GIN trgm index.
// Excludes places that have already been merged into another place.
func (r *EventRepository) FindSimilarPlaces(ctx context.Context, name string, locality string, region string, threshold float64) ([]events.SimilarPlaceCandidate, error) {
	queryer := r.queryer()

	rows, err := queryer.Query(ctx, `
SELECT id, ulid, name, similarity(normalized_name, normalize_name($1)) AS sim
  FROM places
 WHERE deleted_at IS NULL
   AND merged_into_id IS NULL
   AND COALESCE(address_locality, '') = COALESCE($2, '')
   AND COALESCE(address_region, '') = COALESCE($3, '')
   AND similarity(normalized_name, normalize_name($1)) >= $4
 ORDER BY sim DESC
 LIMIT 5
`, name, locality, region, threshold)
	if err != nil {
		return nil, fmt.Errorf("find similar places: %w", err)
	}
	defer rows.Close()

	var candidates []events.SimilarPlaceCandidate
	for rows.Next() {
		var c events.SimilarPlaceCandidate
		if err := rows.Scan(&c.ID, &c.ULID, &c.Name, &c.Similarity); err != nil {
			return nil, fmt.Errorf("scan similar place: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate similar places: %w", err)
	}

	return candidates, nil
}

// FindSimilarOrganizations returns organizations with similar normalized names in the same locality/region.
// Uses pg_trgm similarity() against the normalized_name column, which has a GIN trgm index.
// Excludes organizations that have already been merged into another organization.
func (r *EventRepository) FindSimilarOrganizations(ctx context.Context, name string, locality string, region string, threshold float64) ([]events.SimilarOrgCandidate, error) {
	queryer := r.queryer()

	rows, err := queryer.Query(ctx, `
SELECT id, ulid, name, similarity(normalized_name, normalize_name($1)) AS sim
  FROM organizations
 WHERE deleted_at IS NULL
   AND merged_into_id IS NULL
   AND COALESCE(address_locality, '') = COALESCE($2, '')
   AND COALESCE(address_region, '') = COALESCE($3, '')
   AND similarity(normalized_name, normalize_name($1)) >= $4
 ORDER BY sim DESC
 LIMIT 5
`, name, locality, region, threshold)
	if err != nil {
		return nil, fmt.Errorf("find similar organizations: %w", err)
	}
	defer rows.Close()

	var candidates []events.SimilarOrgCandidate
	for rows.Next() {
		var c events.SimilarOrgCandidate
		if err := rows.Scan(&c.ID, &c.ULID, &c.Name, &c.Similarity); err != nil {
			return nil, fmt.Errorf("scan similar organization: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate similar organizations: %w", err)
	}

	return candidates, nil
}

// MergePlaces merges a duplicate place into a primary place.
// Sets merged_into_id on the duplicate, reassigns all events pointing to the duplicate,
// fills empty fields on the primary from the duplicate, and soft-deletes the duplicate.
func (r *EventRepository) MergePlaces(ctx context.Context, duplicateID string, primaryID string) error {
	queryer := r.queryer()

	// Fill gaps on the primary from the duplicate: only overwrite NULL/empty fields.
	// full_address and geo_point are generated columns and must not be set directly.
	_, err := queryer.Exec(ctx, `
UPDATE places p
   SET description              = COALESCE(NULLIF(p.description, ''),              d.description),
       street_address           = COALESCE(NULLIF(p.street_address, ''),           d.street_address),
       address_locality         = COALESCE(NULLIF(p.address_locality, ''),         d.address_locality),
       address_region           = COALESCE(NULLIF(p.address_region, ''),           d.address_region),
       postal_code              = COALESCE(NULLIF(p.postal_code, ''),              d.postal_code),
       address_country          = COALESCE(NULLIF(p.address_country, ''),          d.address_country),
       latitude                 = COALESCE(p.latitude,                             d.latitude),
       longitude                = COALESCE(p.longitude,                            d.longitude),
       telephone                = COALESCE(NULLIF(p.telephone, ''),                d.telephone),
       email                    = COALESCE(NULLIF(p.email, ''),                    d.email),
       url                      = COALESCE(NULLIF(p.url, ''),                      d.url),
       maximum_attendee_capacity = COALESCE(p.maximum_attendee_capacity,           d.maximum_attendee_capacity),
       venue_type               = COALESCE(NULLIF(p.venue_type, ''),               d.venue_type),
       accessibility_features   = CASE WHEN p.accessibility_features IS NULL OR array_length(p.accessibility_features, 1) IS NULL
                                       THEN d.accessibility_features
                                       ELSE p.accessibility_features END,
       updated_at               = NOW()
  FROM places d
 WHERE p.id = $2 AND d.id = $1
`, duplicateID, primaryID)
	if err != nil {
		return fmt.Errorf("fill place gaps from duplicate: %w", err)
	}

	// Reassign all events from duplicate place to primary place
	_, err = queryer.Exec(ctx, `
UPDATE events SET primary_venue_id = $2
 WHERE primary_venue_id = $1
`, duplicateID, primaryID)
	if err != nil {
		return fmt.Errorf("reassign events from duplicate place: %w", err)
	}

	// Reassign all occurrences from duplicate place to primary place
	_, err = queryer.Exec(ctx, `
UPDATE event_occurrences SET venue_id = $2
 WHERE venue_id = $1
`, duplicateID, primaryID)
	if err != nil {
		return fmt.Errorf("reassign occurrences from duplicate place: %w", err)
	}

	// Mark the duplicate as merged and soft-delete it
	cmd, err := queryer.Exec(ctx, `
UPDATE places SET merged_into_id = $2, deleted_at = NOW(), deletion_reason = 'merged'
 WHERE id = $1
`, duplicateID, primaryID)
	if err != nil {
		return fmt.Errorf("merge place: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return events.ErrNotFound
	}

	return nil
}

// MergeOrganizations merges a duplicate organization into a primary organization.
// Sets merged_into_id on the duplicate, reassigns all events pointing to the duplicate,
// fills empty fields on the primary from the duplicate, and soft-deletes the duplicate.
func (r *EventRepository) MergeOrganizations(ctx context.Context, duplicateID string, primaryID string) error {
	queryer := r.queryer()

	// Fill gaps on the primary from the duplicate: only overwrite NULL/empty fields.
	_, err := queryer.Exec(ctx, `
UPDATE organizations p
   SET legal_name         = COALESCE(NULLIF(p.legal_name, ''),         d.legal_name),
       alternate_name     = COALESCE(NULLIF(p.alternate_name, ''),     d.alternate_name),
       description        = COALESCE(NULLIF(p.description, ''),        d.description),
       email              = COALESCE(NULLIF(p.email, ''),              d.email),
       telephone          = COALESCE(NULLIF(p.telephone, ''),          d.telephone),
       url                = COALESCE(NULLIF(p.url, ''),                d.url),
       street_address     = COALESCE(NULLIF(p.street_address, ''),     d.street_address),
       address_locality   = COALESCE(NULLIF(p.address_locality, ''),   d.address_locality),
       address_region     = COALESCE(NULLIF(p.address_region, ''),     d.address_region),
       postal_code        = COALESCE(NULLIF(p.postal_code, ''),        d.postal_code),
       address_country    = COALESCE(NULLIF(p.address_country, ''),    d.address_country),
       organization_type  = COALESCE(NULLIF(p.organization_type, ''),  d.organization_type),
       founding_date      = COALESCE(p.founding_date,                  d.founding_date),
       updated_at         = NOW()
  FROM organizations d
 WHERE p.id = $2 AND d.id = $1
`, duplicateID, primaryID)
	if err != nil {
		return fmt.Errorf("fill organization gaps from duplicate: %w", err)
	}

	// Reassign all events from duplicate org to primary org
	_, err = queryer.Exec(ctx, `
UPDATE events SET organizer_id = $2
 WHERE organizer_id = $1
`, duplicateID, primaryID)
	if err != nil {
		return fmt.Errorf("reassign events from duplicate organization: %w", err)
	}

	// Mark the duplicate as merged and soft-delete it
	cmd, err := queryer.Exec(ctx, `
UPDATE organizations SET merged_into_id = $2, deleted_at = NOW(), deletion_reason = 'merged'
 WHERE id = $1
`, duplicateID, primaryID)
	if err != nil {
		return fmt.Errorf("merge organization: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return events.ErrNotFound
	}

	return nil
}

// UpdateOccurrenceDates updates the start_time and end_time of all occurrences for an event.
// Used by the FixReview workflow to correct occurrence dates during admin review.
func (r *EventRepository) UpdateOccurrenceDates(ctx context.Context, eventULID string, startTime time.Time, endTime *time.Time) error {
	queries := Queries{db: r.queryer()}

	params := UpdateOccurrenceDatesByEventULIDParams{
		EventUlid: eventULID,
		StartTime: pgtype.Timestamptz{Time: startTime, Valid: true},
	}
	if endTime != nil {
		params.EndTime = pgtype.Timestamptz{Time: *endTime, Valid: true}
	}

	err := queries.UpdateOccurrenceDatesByEventULID(ctx, params)
	if err != nil {
		return fmt.Errorf("update occurrence dates: %w", err)
	}
	return nil
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

// MergeEvents merges a duplicate event into a primary event.
// Verifies the primary event exists and is not deleted before merging.
func (r *EventRepository) MergeEvents(ctx context.Context, duplicateULID string, primaryULID string) error {
	queryer := r.queryer()

	// Verify primary event exists and is not deleted
	var exists bool
	err := queryer.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM events WHERE ulid = $1 AND deleted_at IS NULL)`,
		primaryULID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify primary event: %w", err)
	}
	if !exists {
		return fmt.Errorf("primary event %s not found or deleted: %w", primaryULID, events.ErrNotFound)
	}

	queries := Queries{db: queryer}

	err = queries.MergeEventIntoDuplicate(ctx, MergeEventIntoDuplicateParams{
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

// FindReviewByDedup finds an existing review by deduplication keys
func (r *EventRepository) FindReviewByDedup(ctx context.Context, sourceID *string, externalID *string, dedupHash *string) (*events.ReviewQueueEntry, error) {
	queries := Queries{db: r.queryer()}

	params := FindReviewByDedupParams{}
	if sourceID != nil {
		params.SourceID = pgtype.Text{String: *sourceID, Valid: true}
	}
	if externalID != nil {
		params.SourceExternalID = pgtype.Text{String: *externalID, Valid: true}
	}
	if dedupHash != nil {
		params.DedupHash = pgtype.Text{String: *dedupHash, Valid: true}
	}

	row, err := queries.FindReviewByDedup(ctx, params)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("find review by dedup: %w", err)
	}

	return convertFindReviewByDedupRow(row), nil
}

// reviewQueueRowFields defines the common interface for review queue row types.
// All SQLc-generated row types (FindReviewByDedupRow, ListReviewQueueRow, GetReviewQueueEntryRow)
// share this structure, allowing DRY conversion logic.
type reviewQueueRowFields interface {
	GetID() int32
	GetEventID() pgtype.UUID
	GetEventUlid() string
	GetOriginalPayload() []byte
	GetNormalizedPayload() []byte
	GetWarnings() []byte
	GetSourceID() pgtype.Text
	GetSourceExternalID() pgtype.Text
	GetDedupHash() pgtype.Text
	GetEventStartTime() pgtype.Timestamptz
	GetEventEndTime() pgtype.Timestamptz
	GetStatus() string
	GetReviewedBy() pgtype.Text
	GetReviewedAt() pgtype.Timestamptz
	GetReviewNotes() pgtype.Text
	GetRejectionReason() pgtype.Text
	GetCreatedAt() pgtype.Timestamptz
	GetUpdatedAt() pgtype.Timestamptz
}

// Implement reviewQueueRowFields for FindReviewByDedupRow
func (r FindReviewByDedupRow) GetID() int32                          { return r.ID }
func (r FindReviewByDedupRow) GetEventID() pgtype.UUID               { return r.EventID }
func (r FindReviewByDedupRow) GetEventUlid() string                  { return r.EventUlid }
func (r FindReviewByDedupRow) GetOriginalPayload() []byte            { return r.OriginalPayload }
func (r FindReviewByDedupRow) GetNormalizedPayload() []byte          { return r.NormalizedPayload }
func (r FindReviewByDedupRow) GetWarnings() []byte                   { return r.Warnings }
func (r FindReviewByDedupRow) GetSourceID() pgtype.Text              { return r.SourceID }
func (r FindReviewByDedupRow) GetSourceExternalID() pgtype.Text      { return r.SourceExternalID }
func (r FindReviewByDedupRow) GetDedupHash() pgtype.Text             { return r.DedupHash }
func (r FindReviewByDedupRow) GetEventStartTime() pgtype.Timestamptz { return r.EventStartTime }
func (r FindReviewByDedupRow) GetEventEndTime() pgtype.Timestamptz   { return r.EventEndTime }
func (r FindReviewByDedupRow) GetStatus() string                     { return r.Status }
func (r FindReviewByDedupRow) GetReviewedBy() pgtype.Text            { return r.ReviewedBy }
func (r FindReviewByDedupRow) GetReviewedAt() pgtype.Timestamptz     { return r.ReviewedAt }
func (r FindReviewByDedupRow) GetReviewNotes() pgtype.Text           { return r.ReviewNotes }
func (r FindReviewByDedupRow) GetRejectionReason() pgtype.Text       { return r.RejectionReason }
func (r FindReviewByDedupRow) GetCreatedAt() pgtype.Timestamptz      { return r.CreatedAt }
func (r FindReviewByDedupRow) GetUpdatedAt() pgtype.Timestamptz      { return r.UpdatedAt }

// Implement reviewQueueRowFields for ListReviewQueueRow
func (r ListReviewQueueRow) GetID() int32                          { return r.ID }
func (r ListReviewQueueRow) GetEventID() pgtype.UUID               { return r.EventID }
func (r ListReviewQueueRow) GetEventUlid() string                  { return r.EventUlid }
func (r ListReviewQueueRow) GetOriginalPayload() []byte            { return r.OriginalPayload }
func (r ListReviewQueueRow) GetNormalizedPayload() []byte          { return r.NormalizedPayload }
func (r ListReviewQueueRow) GetWarnings() []byte                   { return r.Warnings }
func (r ListReviewQueueRow) GetSourceID() pgtype.Text              { return r.SourceID }
func (r ListReviewQueueRow) GetSourceExternalID() pgtype.Text      { return r.SourceExternalID }
func (r ListReviewQueueRow) GetDedupHash() pgtype.Text             { return r.DedupHash }
func (r ListReviewQueueRow) GetEventStartTime() pgtype.Timestamptz { return r.EventStartTime }
func (r ListReviewQueueRow) GetEventEndTime() pgtype.Timestamptz   { return r.EventEndTime }
func (r ListReviewQueueRow) GetStatus() string                     { return r.Status }
func (r ListReviewQueueRow) GetReviewedBy() pgtype.Text            { return r.ReviewedBy }
func (r ListReviewQueueRow) GetReviewedAt() pgtype.Timestamptz     { return r.ReviewedAt }
func (r ListReviewQueueRow) GetReviewNotes() pgtype.Text           { return r.ReviewNotes }
func (r ListReviewQueueRow) GetRejectionReason() pgtype.Text       { return r.RejectionReason }
func (r ListReviewQueueRow) GetCreatedAt() pgtype.Timestamptz      { return r.CreatedAt }
func (r ListReviewQueueRow) GetUpdatedAt() pgtype.Timestamptz      { return r.UpdatedAt }

// Implement reviewQueueRowFields for GetReviewQueueEntryRow
func (r GetReviewQueueEntryRow) GetID() int32                          { return r.ID }
func (r GetReviewQueueEntryRow) GetEventID() pgtype.UUID               { return r.EventID }
func (r GetReviewQueueEntryRow) GetEventUlid() string                  { return r.EventUlid }
func (r GetReviewQueueEntryRow) GetOriginalPayload() []byte            { return r.OriginalPayload }
func (r GetReviewQueueEntryRow) GetNormalizedPayload() []byte          { return r.NormalizedPayload }
func (r GetReviewQueueEntryRow) GetWarnings() []byte                   { return r.Warnings }
func (r GetReviewQueueEntryRow) GetSourceID() pgtype.Text              { return r.SourceID }
func (r GetReviewQueueEntryRow) GetSourceExternalID() pgtype.Text      { return r.SourceExternalID }
func (r GetReviewQueueEntryRow) GetDedupHash() pgtype.Text             { return r.DedupHash }
func (r GetReviewQueueEntryRow) GetEventStartTime() pgtype.Timestamptz { return r.EventStartTime }
func (r GetReviewQueueEntryRow) GetEventEndTime() pgtype.Timestamptz   { return r.EventEndTime }
func (r GetReviewQueueEntryRow) GetStatus() string                     { return r.Status }
func (r GetReviewQueueEntryRow) GetReviewedBy() pgtype.Text            { return r.ReviewedBy }
func (r GetReviewQueueEntryRow) GetReviewedAt() pgtype.Timestamptz     { return r.ReviewedAt }
func (r GetReviewQueueEntryRow) GetReviewNotes() pgtype.Text           { return r.ReviewNotes }
func (r GetReviewQueueEntryRow) GetRejectionReason() pgtype.Text       { return r.RejectionReason }
func (r GetReviewQueueEntryRow) GetCreatedAt() pgtype.Timestamptz      { return r.CreatedAt }
func (r GetReviewQueueEntryRow) GetUpdatedAt() pgtype.Timestamptz      { return r.UpdatedAt }

// Implement reviewQueueRowFields for EventReviewQueue (standard row type without JOIN)
func (r EventReviewQueue) GetID() int32                          { return r.ID }
func (r EventReviewQueue) GetEventID() pgtype.UUID               { return r.EventID }
func (r EventReviewQueue) GetEventUlid() string                  { return "" } // Not available without JOIN
func (r EventReviewQueue) GetOriginalPayload() []byte            { return r.OriginalPayload }
func (r EventReviewQueue) GetNormalizedPayload() []byte          { return r.NormalizedPayload }
func (r EventReviewQueue) GetWarnings() []byte                   { return r.Warnings }
func (r EventReviewQueue) GetSourceID() pgtype.Text              { return r.SourceID }
func (r EventReviewQueue) GetSourceExternalID() pgtype.Text      { return r.SourceExternalID }
func (r EventReviewQueue) GetDedupHash() pgtype.Text             { return r.DedupHash }
func (r EventReviewQueue) GetEventStartTime() pgtype.Timestamptz { return r.EventStartTime }
func (r EventReviewQueue) GetEventEndTime() pgtype.Timestamptz   { return r.EventEndTime }
func (r EventReviewQueue) GetStatus() string                     { return r.Status }
func (r EventReviewQueue) GetReviewedBy() pgtype.Text            { return r.ReviewedBy }
func (r EventReviewQueue) GetReviewedAt() pgtype.Timestamptz     { return r.ReviewedAt }
func (r EventReviewQueue) GetReviewNotes() pgtype.Text           { return r.ReviewNotes }
func (r EventReviewQueue) GetRejectionReason() pgtype.Text       { return r.RejectionReason }
func (r EventReviewQueue) GetCreatedAt() pgtype.Timestamptz      { return r.CreatedAt }
func (r EventReviewQueue) GetUpdatedAt() pgtype.Timestamptz      { return r.UpdatedAt }

// Note: ApproveReviewRow, RejectReviewRow, CreateReviewQueueEntryRow, and
// UpdateReviewQueueEntryRow no longer exist as separate types — these queries
// now use RETURNING * and return EventReviewQueue directly (which already
// implements reviewQueueRowFields above).

// convertReviewQueueRowGeneric converts any SQLc review queue row type to domain ReviewQueueEntry.
// This generic converter eliminates ~120 lines of duplicated conversion logic across four
// nearly-identical converter functions.
func convertReviewQueueRowGeneric(row reviewQueueRowFields) *events.ReviewQueueEntry {
	entry := &events.ReviewQueueEntry{
		ID:                int(row.GetID()),
		EventID:           row.GetEventID().String(),
		EventULID:         row.GetEventUlid(),
		OriginalPayload:   row.GetOriginalPayload(),
		NormalizedPayload: row.GetNormalizedPayload(),
		Warnings:          row.GetWarnings(),
		EventStartTime:    row.GetEventStartTime().Time,
		Status:            row.GetStatus(),
		CreatedAt:         row.GetCreatedAt().Time,
		UpdatedAt:         row.GetUpdatedAt().Time,
	}

	if sourceID := row.GetSourceID(); sourceID.Valid {
		entry.SourceID = &sourceID.String
	}
	if sourceExternalID := row.GetSourceExternalID(); sourceExternalID.Valid {
		entry.SourceExternalID = &sourceExternalID.String
	}
	if dedupHash := row.GetDedupHash(); dedupHash.Valid {
		entry.DedupHash = &dedupHash.String
	}
	if eventEndTime := row.GetEventEndTime(); eventEndTime.Valid {
		entry.EventEndTime = &eventEndTime.Time
	}
	if reviewedBy := row.GetReviewedBy(); reviewedBy.Valid {
		entry.ReviewedBy = &reviewedBy.String
	}
	if reviewedAt := row.GetReviewedAt(); reviewedAt.Valid {
		entry.ReviewedAt = &reviewedAt.Time
	}
	if reviewNotes := row.GetReviewNotes(); reviewNotes.Valid {
		entry.ReviewNotes = &reviewNotes.String
	}
	if rejectionReason := row.GetRejectionReason(); rejectionReason.Valid {
		entry.RejectionReason = &rejectionReason.String
	}

	return entry
}

// convertFindReviewByDedupRow converts SQLc-generated FindReviewByDedupRow to domain ReviewQueueEntry
func convertFindReviewByDedupRow(row FindReviewByDedupRow) *events.ReviewQueueEntry {
	return convertReviewQueueRowGeneric(row)
}

// CreateReviewQueueEntry creates a new review queue entry
func (r *EventRepository) CreateReviewQueueEntry(ctx context.Context, params events.ReviewQueueCreateParams) (*events.ReviewQueueEntry, error) {
	queries := Queries{db: r.queryer()}

	var eventIDUUID pgtype.UUID
	if err := eventIDUUID.Scan(params.EventID); err != nil {
		return nil, fmt.Errorf("invalid event ID: %w", err)
	}

	createParams := CreateReviewQueueEntryParams{
		EventID:           eventIDUUID,
		OriginalPayload:   params.OriginalPayload,
		NormalizedPayload: params.NormalizedPayload,
		Warnings:          params.Warnings,
		EventStartTime:    pgtype.Timestamptz{Time: params.EventStartTime, Valid: true},
	}

	if params.SourceID != nil {
		createParams.SourceID = pgtype.Text{String: *params.SourceID, Valid: true}
	}
	if params.SourceExternalID != nil {
		createParams.SourceExternalID = pgtype.Text{String: *params.SourceExternalID, Valid: true}
	}
	if params.DedupHash != nil {
		createParams.DedupHash = pgtype.Text{String: *params.DedupHash, Valid: true}
	}
	if params.EventEndTime != nil {
		createParams.EventEndTime = pgtype.Timestamptz{Time: *params.EventEndTime, Valid: true}
	}

	row, err := queries.CreateReviewQueueEntry(ctx, createParams)
	if err != nil {
		return nil, fmt.Errorf("create review queue entry: %w", err)
	}

	return convertReviewQueueRowGeneric(row), nil
}

// UpdateReviewQueueEntry updates an existing review queue entry
func (r *EventRepository) UpdateReviewQueueEntry(ctx context.Context, id int, params events.ReviewQueueUpdateParams) (*events.ReviewQueueEntry, error) {
	queries := Queries{db: r.queryer()}

	updateParams := UpdateReviewQueueEntryParams{
		ID: int32(id),
	}

	if params.OriginalPayload != nil {
		updateParams.OriginalPayload = *params.OriginalPayload
	}
	if params.NormalizedPayload != nil {
		updateParams.NormalizedPayload = *params.NormalizedPayload
	}
	if params.Warnings != nil {
		updateParams.Warnings = *params.Warnings
	}

	row, err := queries.UpdateReviewQueueEntry(ctx, updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("update review queue entry: %w", err)
	}

	return convertReviewQueueRowGeneric(row), nil
}

// GetReviewQueueEntry retrieves a single review queue entry by ID
func (r *EventRepository) GetReviewQueueEntry(ctx context.Context, id int) (*events.ReviewQueueEntry, error) {
	queries := Queries{db: r.queryer()}

	row, err := queries.GetReviewQueueEntry(ctx, int32(id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("get review queue entry: %w", err)
	}

	return convertGetReviewQueueEntryRow(row), nil
}

// ListReviewQueue lists review queue entries with filters and pagination
func (r *EventRepository) ListReviewQueue(ctx context.Context, filters events.ReviewQueueFilters) (*events.ReviewQueueListResult, error) {
	queries := Queries{db: r.queryer()}

	// Request LIMIT + 1 to detect if there are more pages
	params := ListReviewQueueParams{
		Limit: int32(filters.Limit + 1),
	}

	// Build count param with same status filter
	var countStatus pgtype.Text
	if filters.Status != nil {
		params.Status = pgtype.Text{String: *filters.Status, Valid: true}
		countStatus = pgtype.Text{String: *filters.Status, Valid: true}
	}
	if filters.NextCursor != nil {
		params.AfterID = pgtype.Int4{Int32: int32(*filters.NextCursor), Valid: true}
	}

	// Get total count for this filter (for badge display)
	totalCount, err := queries.CountReviewQueueByStatus(ctx, countStatus)
	if err != nil {
		return nil, fmt.Errorf("count review queue: %w", err)
	}

	rows, err := queries.ListReviewQueue(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list review queue: %w", err)
	}

	// Check if there are more pages (we got LIMIT + 1 items)
	hasMore := len(rows) > filters.Limit

	// Trim to requested limit if we got more
	if hasMore {
		rows = rows[:filters.Limit]
	}

	entries := make([]events.ReviewQueueEntry, 0, len(rows))
	var nextCursor *int
	for i, row := range rows {
		entries = append(entries, *convertListReviewQueueRow(row))
		// Set next cursor to the ID of the last item ONLY if there are more pages
		if i == len(rows)-1 && hasMore {
			cursor := int(row.ID)
			nextCursor = &cursor
		}
	}

	return &events.ReviewQueueListResult{
		Entries:    entries,
		NextCursor: nextCursor,
		TotalCount: totalCount,
	}, nil
}

// ApproveReview marks a review as approved
func (r *EventRepository) ApproveReview(ctx context.Context, id int, reviewedBy string, notes *string) (*events.ReviewQueueEntry, error) {
	queries := Queries{db: r.queryer()}

	params := ApproveReviewParams{
		ID:         int32(id),
		ReviewedBy: pgtype.Text{String: reviewedBy, Valid: true},
	}

	if notes != nil {
		params.Notes = pgtype.Text{String: *notes, Valid: true}
	}

	row, err := queries.ApproveReview(ctx, params)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("approve review: %w", err)
	}

	return convertReviewQueueRowGeneric(row), nil
}

// RejectReview marks a review as rejected
func (r *EventRepository) RejectReview(ctx context.Context, id int, reviewedBy string, reason string) (*events.ReviewQueueEntry, error) {
	queries := Queries{db: r.queryer()}

	params := RejectReviewParams{
		ID:         int32(id),
		ReviewedBy: pgtype.Text{String: reviewedBy, Valid: true},
		Reason:     pgtype.Text{String: reason, Valid: true},
	}

	row, err := queries.RejectReview(ctx, params)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("reject review: %w", err)
	}

	return convertReviewQueueRowGeneric(row), nil
}

// MergeReview marks a review as merged, linking it to the primary event it was merged into.
// The duplicate event (from the review entry) is merged into primaryEventULID via AdminService.MergeEvents.
// This method only updates the review queue status — the caller is responsible for the actual event merge.
func (r *EventRepository) MergeReview(ctx context.Context, id int, reviewedBy string, primaryEventULID string) (*events.ReviewQueueEntry, error) {
	queryer := r.queryer()

	// Look up the primary event's UUID from its ULID
	var primaryEventID pgtype.UUID
	err := queryer.QueryRow(ctx, `SELECT id FROM events WHERE ulid = $1 AND deleted_at IS NULL`, primaryEventULID).Scan(&primaryEventID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("primary event not found: %s", primaryEventULID)
		}
		return nil, fmt.Errorf("lookup primary event: %w", err)
	}

	row := queryer.QueryRow(ctx, `
UPDATE event_review_queue
   SET status = 'merged',
       reviewed_by = $2,
       reviewed_at = NOW(),
       review_notes = 'Merged into event ' || $4,
       duplicate_of_event_id = $3,
       updated_at = NOW()
 WHERE id = $1
   AND status = 'pending'
RETURNING id, event_id, original_payload, normalized_payload, warnings,
          source_id, source_external_id, dedup_hash, event_start_time,
          event_end_time, status, reviewed_by, reviewed_at, review_notes,
          rejection_reason, created_at, updated_at
`, id, reviewedBy, primaryEventID, primaryEventULID)

	var entry events.ReviewQueueEntry
	var eventID pgtype.UUID
	var sourceID, sourceExternalID, dedupHash, rReviewedBy, reviewNotes, rejectionReason pgtype.Text
	var eventStartTime, eventEndTime, reviewedAt, createdAt, updatedAt pgtype.Timestamptz

	err = row.Scan(
		&entry.ID, &eventID, &entry.OriginalPayload, &entry.NormalizedPayload, &entry.Warnings,
		&sourceID, &sourceExternalID, &dedupHash, &eventStartTime,
		&eventEndTime, &entry.Status, &rReviewedBy, &reviewedAt, &reviewNotes,
		&rejectionReason, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, events.ErrNotFound
		}
		return nil, fmt.Errorf("merge review: %w", err)
	}

	entry.EventID = eventID.String()
	entry.EventStartTime = eventStartTime.Time
	entry.CreatedAt = createdAt.Time
	entry.UpdatedAt = updatedAt.Time
	entry.DuplicateOfEventID = &primaryEventULID

	if sourceID.Valid {
		entry.SourceID = &sourceID.String
	}
	if sourceExternalID.Valid {
		entry.SourceExternalID = &sourceExternalID.String
	}
	if dedupHash.Valid {
		entry.DedupHash = &dedupHash.String
	}
	if eventEndTime.Valid {
		entry.EventEndTime = &eventEndTime.Time
	}
	if rReviewedBy.Valid {
		entry.ReviewedBy = &rReviewedBy.String
	}
	if reviewedAt.Valid {
		entry.ReviewedAt = &reviewedAt.Time
	}
	if reviewNotes.Valid {
		entry.ReviewNotes = &reviewNotes.String
	}
	if rejectionReason.Valid {
		entry.RejectionReason = &rejectionReason.String
	}

	return &entry, nil
}

// CleanupExpiredReviews runs all cleanup operations for the review queue
func (r *EventRepository) CleanupExpiredReviews(ctx context.Context) error {
	queries := Queries{db: r.queryer()}

	// First mark unreviewed events as deleted
	if err := queries.MarkUnreviewedEventsAsDeleted(ctx); err != nil {
		return fmt.Errorf("mark unreviewed events as deleted: %w", err)
	}

	if ctx.Err() != nil {
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	}

	// Delete expired rejections
	if err := queries.CleanupExpiredRejections(ctx); err != nil {
		return fmt.Errorf("cleanup expired rejections: %w", err)
	}

	if ctx.Err() != nil {
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	}

	// Delete unreviewed events
	if err := queries.CleanupUnreviewedEvents(ctx); err != nil {
		return fmt.Errorf("cleanup unreviewed events: %w", err)
	}

	if ctx.Err() != nil {
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	}

	// Archive old approved/superseded reviews
	if err := queries.CleanupArchivedReviews(ctx); err != nil {
		return fmt.Errorf("cleanup archived reviews: %w", err)
	}

	return nil
}

// convertListReviewQueueRow converts SQLc ListReviewQueueRow to events.ReviewQueueEntry
// Used when the query includes a JOIN with events table to get event_ulid
func convertListReviewQueueRow(row ListReviewQueueRow) *events.ReviewQueueEntry {
	return convertReviewQueueRowGeneric(row)
}

// convertGetReviewQueueEntryRow converts SQLc GetReviewQueueEntryRow to events.ReviewQueueEntry
// Used when the query includes a JOIN with events table to get event_ulid
func convertGetReviewQueueEntryRow(row GetReviewQueueEntryRow) *events.ReviewQueueEntry {
	return convertReviewQueueRowGeneric(row)
}

func convertReviewQueueRow(row EventReviewQueue) *events.ReviewQueueEntry {
	return convertReviewQueueRowGeneric(row)
}
