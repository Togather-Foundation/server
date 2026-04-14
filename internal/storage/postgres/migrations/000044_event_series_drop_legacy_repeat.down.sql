-- Re-add the legacy repeat columns removed in 000044.
-- Data is not restored — the columns come back empty.
ALTER TABLE event_series
  ADD COLUMN repeat_frequency TEXT,
  ADD COLUMN repeat_on_days TEXT[],
  ADD COLUMN repeat_on_dates INTEGER[];