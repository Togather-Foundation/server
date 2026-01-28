-- Batch ingestion results table for tracking batch job status
CREATE TABLE IF NOT EXISTS batch_ingestion_results (
    batch_id TEXT PRIMARY KEY,
    results JSONB NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

-- Index for efficient lookups by completion time
CREATE INDEX IF NOT EXISTS idx_batch_ingestion_results_completed_at 
    ON batch_ingestion_results(completed_at DESC);

-- Add comments for documentation
COMMENT ON TABLE batch_ingestion_results IS 'Stores results of batch event ingestion jobs';
COMMENT ON COLUMN batch_ingestion_results.batch_id IS 'Unique identifier for the batch job (PRIMARY KEY provides implicit index for lookups)';
COMMENT ON COLUMN batch_ingestion_results.results IS 'JSON array of ingestion results per event';
COMMENT ON COLUMN batch_ingestion_results.completed_at IS 'When the batch job completed';
