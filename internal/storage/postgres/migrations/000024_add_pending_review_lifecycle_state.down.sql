-- Remove 'pending_review' lifecycle state

-- Drop the existing constraint
ALTER TABLE events DROP CONSTRAINT events_lifecycle_state_check;

-- Add the constraint without pending_review
ALTER TABLE events ADD CONSTRAINT events_lifecycle_state_check CHECK (
  lifecycle_state IN (
    'draft',
    'published',
    'postponed',
    'rescheduled',
    'sold_out',
    'cancelled',
    'completed',
    'deleted'
  )
);
