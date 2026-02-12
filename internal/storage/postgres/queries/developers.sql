-- SQLc queries for developer management and invitations.

-- Developer CRUD operations

-- name: CreateDeveloper :one
INSERT INTO developers (email, name, github_id, github_username, password_hash, max_keys)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetDeveloperByID :one
SELECT * FROM developers WHERE id = $1;

-- name: GetDeveloperByEmail :one
SELECT * FROM developers WHERE email = $1;

-- name: GetDeveloperByGitHubID :one
SELECT * FROM developers WHERE github_id = $1;

-- name: ListDevelopers :many
SELECT * FROM developers 
ORDER BY created_at DESC 
LIMIT $1 OFFSET $2;

-- name: UpdateDeveloper :one
UPDATE developers 
SET name = COALESCE(sqlc.narg('name'), name),
    github_id = COALESCE(sqlc.narg('github_id'), github_id),
    github_username = COALESCE(sqlc.narg('github_username'), github_username),
    max_keys = COALESCE(sqlc.narg('max_keys'), max_keys),
    is_active = COALESCE(sqlc.narg('is_active'), is_active)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: UpdateDeveloperLastLogin :exec
UPDATE developers SET last_login_at = now() WHERE id = $1;

-- name: DeactivateDeveloper :exec
UPDATE developers SET is_active = false WHERE id = $1;

-- name: CountDevelopers :one
SELECT COUNT(*) FROM developers;

-- Developer invitation operations

-- name: CreateDeveloperInvitation :one
INSERT INTO developer_invitations (email, token_hash, invited_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetDeveloperInvitationByTokenHash :one
SELECT * FROM developer_invitations 
WHERE token_hash = $1 AND accepted_at IS NULL;

-- name: AcceptDeveloperInvitation :exec
UPDATE developer_invitations SET accepted_at = now() WHERE id = $1;

-- name: ListActiveDeveloperInvitations :many
SELECT * FROM developer_invitations 
WHERE accepted_at IS NULL AND expires_at > now()
ORDER BY created_at DESC;

-- Developer API key operations

-- name: ListDeveloperAPIKeys :many
SELECT * FROM api_keys 
WHERE developer_id = $1 
ORDER BY created_at DESC;

-- name: CountDeveloperAPIKeys :one
SELECT COUNT(*) FROM api_keys 
WHERE developer_id = $1 AND is_active = true;

-- name: CreateDeveloperAPIKey :one
INSERT INTO api_keys (prefix, key_hash, hash_version, name, developer_id, role, rate_limit_tier, is_active, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, prefix, name, role, rate_limit_tier, is_active, created_at, expires_at;

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys WHERE id = $1;
