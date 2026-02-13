-- SQLc queries for API key usage tracking.

-- name: UpsertAPIKeyUsage :exec
INSERT INTO api_key_usage (api_key_id, date, request_count, error_count)
VALUES ($1, $2, $3, $4)
ON CONFLICT (api_key_id, date) DO UPDATE SET
    request_count = api_key_usage.request_count + EXCLUDED.request_count,
    error_count = api_key_usage.error_count + EXCLUDED.error_count;

-- name: GetAPIKeyUsage :many
SELECT * FROM api_key_usage 
WHERE api_key_id = $1 AND date >= $2 AND date <= $3
ORDER BY date DESC;

-- name: GetAPIKeyUsageTotal :one
SELECT COALESCE(SUM(request_count), 0)::bigint AS total_requests,
       COALESCE(SUM(error_count), 0)::bigint AS total_errors
FROM api_key_usage 
WHERE api_key_id = $1 AND date >= $2 AND date <= $3;

-- name: GetDeveloperUsageTotal :one
SELECT COALESCE(SUM(u.request_count), 0)::bigint AS total_requests,
       COALESCE(SUM(u.error_count), 0)::bigint AS total_errors
FROM api_key_usage u
JOIN api_keys k ON u.api_key_id = k.id
WHERE k.developer_id = $1 AND u.date >= $2 AND u.date <= $3;
