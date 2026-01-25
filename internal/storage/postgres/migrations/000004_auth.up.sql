CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  username TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,

  role TEXT NOT NULL CHECK (role IN ('admin', 'editor', 'viewer')),
  is_active BOOLEAN NOT NULL DEFAULT true,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ
);

CREATE INDEX idx_users_role ON users (role, is_active);

CREATE TABLE api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  prefix TEXT NOT NULL UNIQUE,
  key_hash TEXT NOT NULL,
  name TEXT NOT NULL,

  source_id UUID REFERENCES sources(id),
  role TEXT NOT NULL DEFAULT 'agent' CHECK (role IN ('agent')),
  rate_limit_tier TEXT NOT NULL DEFAULT 'agent',

  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_active ON api_keys (is_active, expires_at);
CREATE INDEX idx_api_keys_source ON api_keys (source_id, is_active);