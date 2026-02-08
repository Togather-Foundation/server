-- Add 'pending_review' lifecycle state for events that need admin review
-- This state is used when events have data quality warnings (e.g., auto-corrected
-- reversed dates) that should be reviewed by an admin before publishing.

-- Drop the existing constraint
ALTER TABLE events DROP CONSTRAINT events_lifecycle_state_check;

-- Add the constraint with pending_review included
ALTER TABLE events ADD CONSTRAINT events_lifecycle_state_check CHECK (
  lifecycle_state IN (
    'draft',
    'published',
    'pending_review',
    'postponed',
    'rescheduled',
    'sold_out',
    'cancelled',
    'completed',
    'deleted'
  )
);
