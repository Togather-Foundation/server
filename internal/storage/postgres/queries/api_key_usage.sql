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

-- name: UpsertAPIKeyUsageIP :exec
INSERT INTO api_key_usage_ips (api_key_id, date, ip, request_count, error_count)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (api_key_id, date, ip) DO UPDATE SET
    request_count = api_key_usage_ips.request_count + EXCLUDED.request_count,
    error_count = api_key_usage_ips.error_count + EXCLUDED.error_count;

-- name: GetDailyUsageReportData :many
SELECT k.id AS api_key_id, k.name AS key_name, k.prefix AS key_prefix,
       d.id AS developer_id, d.name AS developer_name, d.email AS developer_email,
       u.date, u.ip, u.request_count, u.error_count
FROM api_key_usage_ips u
JOIN api_keys k ON k.id = u.api_key_id
JOIN developers d ON d.id = k.developer_id
WHERE u.date >= $1 AND u.date <= $2
  AND k.developer_id IS NOT NULL
ORDER BY u.date, u.request_count DESC;
