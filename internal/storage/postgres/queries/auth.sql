-- SQLc queries for authentication.

-- name: GetUserByEmail :one
SELECT id, username, email, password_hash, role, is_active, created_at, last_login_at
FROM users
WHERE email = $1 AND is_active = true
LIMIT 1;

-- name: GetUserByUsername :one
SELECT id, username, email, password_hash, role, is_active, created_at, last_login_at
FROM users
WHERE username = $1 AND is_active = true
LIMIT 1;

-- name: GetUserByID :one
SELECT id, username, email, password_hash, role, is_active, created_at, last_login_at
FROM users
WHERE id = $1
LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, role, is_active)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, username, email, role, is_active, created_at;

-- name: UpdateLastLogin :exec
UPDATE users
SET last_login_at = now()
WHERE id = $1;

-- name: ListUsers :many
SELECT id, username, email, role, is_active, created_at, last_login_at
FROM users
ORDER BY created_at DESC;

-- name: UpdateUser :exec
UPDATE users
SET username = $2,
    email = $3,
    role = $4,
    is_active = $5
WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2
WHERE id = $1;

-- name: CreateAPIKey :one
INSERT INTO api_keys (prefix, key_hash, hash_version, name, source_id, role, rate_limit_tier, is_active, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, prefix, name, role, rate_limit_tier, is_active, created_at, expires_at;

-- name: GetAPIKeyByPrefix :one
SELECT id, prefix, key_hash, hash_version, name, source_id, role, rate_limit_tier, is_active, last_used_at, expires_at
FROM api_keys
WHERE prefix = $1 AND is_active = true
LIMIT 1;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys
SET last_used_at = now()
WHERE id = $1;

-- name: ListAPIKeys :many
SELECT id, prefix, name, source_id, role, rate_limit_tier, is_active, created_at, last_used_at, expires_at
FROM api_keys
ORDER BY created_at DESC;

-- name: DeactivateAPIKey :exec
UPDATE api_keys
SET is_active = false
WHERE id = $1;

-- User Invitation Queries

-- name: CreateUserInvitation :one
INSERT INTO user_invitations (user_id, token_hash, email, expires_at, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, token_hash, email, expires_at, created_at;

-- name: GetUserInvitationByTokenHash :one
SELECT id, user_id, token_hash, email, expires_at, accepted_at, created_by, created_at
FROM user_invitations
WHERE token_hash = $1 AND expires_at > now() AND accepted_at IS NULL
LIMIT 1;

-- name: MarkInvitationAccepted :exec
UPDATE user_invitations
SET accepted_at = now()
WHERE id = $1;

-- name: ListPendingInvitationsForUser :many
SELECT id, token_hash, email, expires_at, created_at
FROM user_invitations
WHERE user_id = $1 AND accepted_at IS NULL AND expires_at > now()
ORDER BY created_at DESC;

-- User Management Queries

-- name: DeactivateUser :exec
UPDATE users
SET is_active = false
WHERE id = $1;

-- name: ActivateUser :exec
UPDATE users
SET is_active = true
WHERE id = $1;

-- name: DeleteUser :exec
UPDATE users
SET deleted_at = now()
WHERE id = $1;

-- name: ListUsersWithFilters :many
SELECT id, username, email, role, is_active, created_at, last_login_at
FROM users
WHERE 
  ($1::boolean IS NULL OR is_active = $1) AND
  ($2::text IS NULL OR role = $2) AND
  deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountUsers :one
SELECT COUNT(*) FROM users
WHERE 
  ($1::boolean IS NULL OR is_active = $1) AND
  ($2::text IS NULL OR role = $2) AND
  deleted_at IS NULL;