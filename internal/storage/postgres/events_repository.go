package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Togather-Foundation/server/internal/api/pagination"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ events.Repository = (*EventRepository)(nil)

type eventRow struct {
	ID             string
	ULID           string
	Name           string
	Description    *string
	LifecycleState string
	EventDomain    string
	OrganizerID    *string
	PrimaryVenueID *string
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

	rows, err := queryer.Query(ctx, `
SELECT e.id, e.ulid, e.name, e.description, e.lifecycle_state, e.event_domain,
       e.organizer_id, e.primary_venue_id, e.keywords, e.created_at, e.updated_at,
       o.start_time
  FROM events e
  JOIN event_occurrences o ON o.event_id = e.id
  LEFT JOIN places p ON p.id = COALESCE(o.venue_id, e.primary_venue_id)
  LEFT JOIN organizations org ON org.id = e.organizer_id
  WHERE ($1::timestamptz IS NULL OR o.start_time >= $1::timestamptz)
    AND ($2::timestamptz IS NULL OR o.start_time <= $2::timestamptz)
    AND ($3 = '' OR p.address_locality ILIKE '%' || $3 || '%')
    AND ($4 = '' OR p.address_region ILIKE '%' || $4 || '%')
    AND ($5 = '' OR p.ulid = $5)
    AND ($6 = '' OR org.ulid = $6)
    AND ($7 = '' OR e.lifecycle_state = $7)
    AND ($8 = '' OR e.event_domain = $8)
    AND ($9 = '' OR (e.name ILIKE '%' || $9 || '%' OR e.description ILIKE '%' || $9 || '%'))
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
		filters.City,
		filters.Region,
		filters.VenueULID,
		filters.OrganizerULID,
		filters.LifecycleState,
		filters.Domain,
		filters.Query,
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
			&row.LifecycleState,
			&row.EventDomain,
			&row.OrganizerID,
			&row.PrimaryVenueID,
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
			LifecycleState: row.LifecycleState,
			EventDomain:    row.EventDomain,
			OrganizerID:    row.OrganizerID,
			PrimaryVenueID: row.PrimaryVenueID,
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
SELECT e.id, e.ulid, e.name, e.description, e.lifecycle_state, e.event_domain,
       e.organizer_id, e.primary_venue_id, e.keywords, e.created_at, e.updated_at,
       o.id, o.start_time, o.end_time, o.timezone, o.venue_id, o.virtual_url
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
			lifecycleState string
			eventDomain    string
			organizerID    *string
			primaryVenueID *string
			keywords       []string
			createdAt      pgtype.Timestamptz
			updatedAt      pgtype.Timestamptz
			occurrenceID   *string
			startTime      pgtype.Timestamptz
			endTime        pgtype.Timestamptz
			timezone       *string
			venueID        *string
			virtualURL     *string
		)
		if err := rows.Scan(
			&eventID,
			&eventULID,
			&name,
			&description,
			&lifecycleState,
			&eventDomain,
			&organizerID,
			&primaryVenueID,
			&keywords,
			&createdAt,
			&updatedAt,
			&occurrenceID,
			&startTime,
			&endTime,
			&timezone,
			&venueID,
			&virtualURL,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		if event == nil {
			event = &events.Event{
				ID:             eventID,
				ULID:           eventULID,
				Name:           name,
				Description:    derefString(description),
				LifecycleState: lifecycleState,
				EventDomain:    eventDomain,
				OrganizerID:    organizerID,
				PrimaryVenueID: primaryVenueID,
				Keywords:       keywords,
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
				VirtualURL: virtualURL,
			}
			if startTime.Valid {
				occ.StartTime = startTime.Time
			}
			if endTime.Valid {
				value := endTime.Time
				occ.EndTime = &value
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

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r *EventRepository) queryer() queryer {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
