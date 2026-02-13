-- Developer Self-Service API Keys feature
-- Phase 1.1: Foundation tables

-- Developers table
CREATE TABLE developers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL DEFAULT '',
    github_id       BIGINT UNIQUE,
    github_username TEXT,
    password_hash   TEXT,
    max_keys        INTEGER NOT NULL DEFAULT 5,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at   TIMESTAMPTZ
);

CREATE INDEX idx_developers_email ON developers (email) WHERE is_active = true;
CREATE INDEX idx_developers_github ON developers (github_id) WHERE github_id IS NOT NULL;

-- Developer invitations table
CREATE TABLE developer_invitations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT NOT NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    invited_by  UUID REFERENCES users(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dev_invitations_token ON developer_invitations (token_hash)
    WHERE accepted_at IS NULL;
CREATE UNIQUE INDEX idx_dev_invitations_active_email ON developer_invitations (email)
    WHERE accepted_at IS NULL;

-- API key usage tracking table
CREATE TABLE api_key_usage (
    api_key_id  UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    date        DATE NOT NULL,
    request_count BIGINT NOT NULL DEFAULT 0,
    error_count   BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (api_key_id, date)
);

CREATE INDEX idx_api_key_usage_date ON api_key_usage (date);

-- Add developer_id column to api_keys
ALTER TABLE api_keys ADD COLUMN developer_id UUID REFERENCES developers(id);
CREATE INDEX idx_api_keys_developer ON api_keys (developer_id) WHERE developer_id IS NOT NULL;
