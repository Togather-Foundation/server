-- Create event_review_queue table for admin review workflow
-- See docs/architecture/event-review-workflow.md for complete design

CREATE TABLE event_review_queue (
  id SERIAL PRIMARY KEY,
  event_id UUID UNIQUE NOT NULL,
  
  -- Original submission for admin comparison
  original_payload JSONB NOT NULL,
  normalized_payload JSONB NOT NULL,
  warnings JSONB NOT NULL,
  
  -- Deduplication keys (match events table)
  source_id TEXT,
  source_external_id TEXT,
  dedup_hash TEXT,
  
  -- Event timing (for expiry logic)
  event_start_time TIMESTAMPTZ NOT NULL,
  event_end_time TIMESTAMPTZ,
  
  -- Review workflow
  status TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, rejected, superseded
  reviewed_by TEXT,
  reviewed_at TIMESTAMPTZ,
  review_notes TEXT,
  rejection_reason TEXT,
  
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  
  -- Foreign key to events table
  CONSTRAINT fk_event FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
);

-- Partial unique indexes: only one pending review per unique event
CREATE UNIQUE INDEX idx_review_queue_unique_pending_source 
  ON event_review_queue(source_id, source_external_id) 
  WHERE status = 'pending';

CREATE UNIQUE INDEX idx_review_queue_unique_pending_dedup 
  ON event_review_queue(dedup_hash) 
  WHERE status = 'pending' AND dedup_hash IS NOT NULL;

-- Other indexes
CREATE INDEX idx_review_queue_status ON event_review_queue(status);
CREATE INDEX idx_review_queue_expired_rejections ON event_review_queue(status, event_end_time) 
  WHERE status = 'rejected';
CREATE INDEX idx_review_queue_event_id ON event_review_queue(event_id);
