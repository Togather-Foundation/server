-- Track pairs of events that an admin has confirmed are NOT duplicates.
-- When near-duplicate detection flags a pair during ingestion, this table
-- is checked first so that previously-reviewed pairs are not re-flagged.
-- The canonical ordering (event_id_a < event_id_b) prevents storing both (A,B) and (B,A).

CREATE TABLE event_not_duplicates (
    event_id_a TEXT NOT NULL,       -- ULID of first event (smaller ULID lexicographically)
    event_id_b TEXT NOT NULL,       -- ULID of second event (larger ULID lexicographically)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,                -- admin who made the decision
    PRIMARY KEY (event_id_a, event_id_b)
);

-- Index for efficient lookup when checking either event in a pair
CREATE INDEX idx_event_not_duplicates_b ON event_not_duplicates (event_id_b);
