-- Drop legacy repeat columns from event_series.
-- These have been superseded by the canonical RRULE columns (rrule, exdates, rdates)
-- added in migration 000043.
ALTER TABLE event_series
  DROP COLUMN IF EXISTS repeat_frequency,
  DROP COLUMN IF EXISTS repeat_on_days,
  DROP COLUMN IF EXISTS repeat_on_dates;