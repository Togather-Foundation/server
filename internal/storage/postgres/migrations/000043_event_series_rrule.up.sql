-- Add canonical RFC 5545 recurrence columns to event_series.
-- rrule: raw RRULE string (stored WITHOUT the "RRULE:" prefix, per iCalendar convention)
-- exdates: exclusion datetimes (RFC 5545 EXDATE)
-- rdates: additional occurrence datetimes (RFC 5545 RDATE)
-- These replace the legacy repeat_frequency/repeat_on_days/repeat_on_dates columns
-- which will be dropped in migration 000044 once all code is wired.
ALTER TABLE event_series
  ADD COLUMN rrule TEXT,
  ADD COLUMN exdates TIMESTAMPTZ[] NOT NULL DEFAULT '{}',
  ADD COLUMN rdates  TIMESTAMPTZ[] NOT NULL DEFAULT '{}';
