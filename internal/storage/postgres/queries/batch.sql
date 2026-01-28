-- SQLc queries for batch ingestion operations.

-- name: GetBatchIngestionResult :one
SELECT batch_id, results, completed_at, created_at
FROM batch_ingestion_results
WHERE batch_id = $1;

-- name: CreateBatchIngestionResult :exec
INSERT INTO batch_ingestion_results (batch_id, results, completed_at)
VALUES ($1, $2, $3)
ON CONFLICT (batch_id) DO UPDATE
SET results = EXCLUDED.results, completed_at = EXCLUDED.completed_at;
