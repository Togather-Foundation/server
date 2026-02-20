# Developer Self-Service API Keys

Developers can create, manage, and monitor their own API keys without admin involvement. Admins gate initial access via invitations; ongoing key management is self-service.

## Architecture Decisions

### Separate `developers` table (not extending `users`)

Admin users and API developers are distinct populations with different auth flows, permissions, and lifecycle. Keeping them separate avoids privilege confusion, auth complexity, and permission scope creep. The `users` table continues to serve admin/editor/viewer roles. `developers` is a separate first-class entity.

### API keys for auth + monitoring only

The existing rate limiting system is IP-based and tier-based on route (public: 60/min, agent: 300/min, admin: unlimited). API keys serve three purposes: identity, access control, and usage monitoring. The `rate_limit_tier` column on `api_keys` is currently unused.

### Developer JWT (separate from admin JWT)

Developer sessions use JWTs with a `"type": "developer"` claim, stored in a `dev_auth_token` cookie (distinct from admin's `auth_token` cookie). This prevents accidental auth crossover between portals.

### Fixed key limit per developer

Each developer can create up to `max_keys` API keys (default: 5, admin-adjustable).

---

## Data Model

### New Tables

#### `developers`

```sql
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
```

#### `developer_invitations`

```sql
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
```

#### `api_key_usage`

```sql
CREATE TABLE api_key_usage (
    api_key_id    UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    date          DATE NOT NULL,
    request_count BIGINT NOT NULL DEFAULT 0,
    error_count   BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (api_key_id, date)
);

CREATE INDEX idx_api_key_usage_date ON api_key_usage (date);
```

### Schema Changes to Existing Tables

#### `api_keys` — `developer_id` column

```sql
ALTER TABLE api_keys ADD COLUMN developer_id UUID REFERENCES developers(id);
CREATE INDEX idx_api_keys_developer ON api_keys (developer_id) WHERE developer_id IS NOT NULL;
```

Existing keys (created via CLI/admin) have `developer_id = NULL`. Developer-created keys always have `developer_id` set and `role = 'agent'`.

---

## Authentication Flows

### Flow 1: Invitation-Based Onboarding

```
Admin                          Server                        Developer
  |                              |                              |
  |-- POST /admin/developers/invite {email} ------------------>|
  |                              |-- Send invitation email ---->|
  |                              |                              |
  |                              |<-- GET /dev/accept-invitation?token=xyz
  |                              |                              |
  |                              |<-- POST /dev/accept-invitation
  |                              |    {token, password, name}   |
  |                              |                              |
  |                              |-- Create developer record -->|
  |                              |-- Set dev_auth_token cookie  |
  |                              |-- Redirect to /dev/dashboard |
```

### Flow 2: GitHub OAuth

```
Developer                      Server                        GitHub
  |                              |                              |
  |-- GET /auth/github --------->|                              |
  |                              |-- Redirect to GitHub ------->|
  |<-- Redirect to GitHub auth --|                              |
  |                              |                              |
  |-- Authorize on GitHub ---------------------------------------->|
  |                              |                              |
  |<-- GET /auth/github/callback?code=abc&state=xyz ------------|
  |                              |                              |
  |                              |-- POST github.com/access_token
  |                              |<-- {access_token} -----------|
  |                              |-- GET api.github.com/user    |
  |                              |<-- {id, login, email} ------|
  |                              |                              |
  |                              |-- Find/create developer      |
  |                              |-- Set dev_auth_token cookie  |
  |<-- Redirect to /dev/dashboard|                              |
```

### Developer Session Management

- Developer JWTs stored in `dev_auth_token` HttpOnly cookie
- JWT lifetime: 24h (configurable via `JWT_EXPIRY_HOURS`)
- JWT claims include `"type": "developer"` to distinguish from admin tokens

---

## API Endpoints

### Developer Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/dev/login` | Rate-limited | Authenticate with email/password |
| POST | `/api/v1/dev/logout` | Dev cookie | Clear dev_auth_token cookie |
| GET | `/api/v1/dev/api-keys` | Dev cookie/JWT | List own API keys with usage summary |
| POST | `/api/v1/dev/api-keys` | Dev cookie/JWT | Create new API key (enforces max_keys) |
| DELETE | `/api/v1/dev/api-keys/{id}` | Dev cookie/JWT | Revoke own API key |
| GET | `/api/v1/dev/api-keys/{id}/usage` | Dev cookie/JWT | Usage stats for own key |
| POST | `/api/v1/dev/accept-invitation` | Rate-limited | Accept invitation + set password |

### GitHub OAuth Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/auth/github` | None | Redirect to GitHub OAuth |
| GET | `/auth/github/callback` | None | GitHub OAuth callback |

### Admin Developer Management

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/admin/developers/invite` | JWT (admin) | Invite developer by email |
| GET | `/api/v1/admin/developers` | JWT (admin) | List all developers |
| GET | `/api/v1/admin/developers/{id}` | JWT (admin) | View developer + their keys |
| PUT | `/api/v1/admin/developers/{id}` | JWT (admin) | Update developer (max_keys, is_active) |
| DELETE | `/api/v1/admin/developers/{id}` | JWT (admin) | Deactivate + revoke all keys |

### Pages

| Path | Description |
|------|-------------|
| `/dev/login` | Login (email/password + GitHub OAuth) |
| `/dev/accept-invitation` | Invitation acceptance |
| `/dev/dashboard` | Usage overview |
| `/dev/api-keys` | Key management |
| `/admin/developers` | Admin developer list and invite |

---

## Request/Response Formats

### POST /api/v1/dev/login

**Request:**
```json
{"email": "dev@example.com", "password": "secure-password-here"}
```

**Response (200):** Sets `dev_auth_token` HttpOnly cookie.
```json
{
    "token": "eyJhbGciOi...",
    "expires_at": "2026-02-13T12:00:00Z",
    "developer": {"id": "uuid", "email": "dev@example.com", "name": "Dev Name", "github_username": null}
}
```

### POST /api/v1/dev/api-keys

**Request:** `{"name": "My Scraper Key"}`

**Response (201):** The `key` field is returned only on creation.
```json
{
    "id": "uuid",
    "name": "My Scraper Key",
    "prefix": "01KGJZ40",
    "key": "01KGJZ40ZG0WQ5SQ...",
    "role": "agent",
    "created_at": "2026-02-12T12:00:00Z"
}
```

### GET /api/v1/dev/api-keys

```json
{
    "items": [{
        "id": "uuid",
        "name": "My Scraper Key",
        "prefix": "01KGJZ40",
        "role": "agent",
        "is_active": true,
        "created_at": "2026-02-12T12:00:00Z",
        "last_used_at": "2026-02-12T15:30:00Z",
        "usage_today": 1523,
        "usage_7d": 8420,
        "usage_30d": 34210
    }],
    "max_keys": 5,
    "key_count": 1
}
```

### GET /api/v1/dev/api-keys/{id}/usage

**Query params:** `?from=2026-01-01&to=2026-02-12`

```json
{
    "api_key_id": "uuid",
    "api_key_name": "My Scraper Key",
    "period": {"from": "2026-01-01", "to": "2026-02-12"},
    "total_requests": 34210,
    "total_errors": 142,
    "daily": [
        {"date": "2026-02-12", "request_count": 1523, "error_count": 3}
    ]
}
```

### Error Responses (RFC 7807)

```json
{
    "type": "https://sel.events/problems/max-keys-exceeded",
    "title": "Maximum API keys exceeded",
    "status": 409,
    "detail": "You have reached your maximum of 5 API keys.",
    "instance": "/api/v1/dev/api-keys"
}
```

Error types: `unauthorized` (401), `forbidden` (403), `developer-not-found` (404), `key-not-found` (404), `max-keys-exceeded` (409), `email-taken` (409), `invitation-invalid` (400), `password-too-weak` (400).

---

## Usage Tracking

### Architecture

Usage is recorded via an in-memory buffer flushed to `api_key_usage` every 30 seconds:

```
Request → AgentAuth middleware → UsageRecorder middleware → Handler
                                     |
                               in-memory buffer
                                     |
                    Background goroutine (every 30s): UPSERT to DB
```

### In-Memory Buffer

```go
type UsageRecorder struct {
    mu     sync.Mutex
    counts map[uuid.UUID]*usageDelta
    repo   UsageRepository
    flush  *time.Ticker
    done   chan struct{}
}
```

The buffer is flushed every 30 seconds, when it exceeds 100 entries, or on graceful shutdown.

### SQL Upsert

```sql
INSERT INTO api_key_usage (api_key_id, date, request_count, error_count)
VALUES ($1, CURRENT_DATE, $2, $3)
ON CONFLICT (api_key_id, date) DO UPDATE SET
    request_count = api_key_usage.request_count + EXCLUDED.request_count,
    error_count = api_key_usage.error_count + EXCLUDED.error_count;
```

---

## Package Structure

```
internal/
  domain/
    developers/
      service.go           -- DeveloperService
      types.go             -- Developer, DeveloperInvitation, CreateParams
      repository.go        -- Repository interface
      usage_recorder.go    -- In-memory usage buffer
  auth/
    developer_jwt.go       -- Developer JWT claims, generate, validate
    oauth/
      github.go            -- GitHub OAuth client
  api/
    handlers/
      dev_auth.go          -- Login, logout, accept invitation
      dev_apikeys.go       -- Developer key CRUD
      dev_html.go          -- Developer portal pages
      admin_developers.go  -- Admin developer management
    middleware/
      dev_auth.go          -- DevCookieAuth, DevAPIAuth
  storage/
    postgres/
      queries/
        developers.sql     -- SQLc queries
        api_key_usage.sql  -- Usage queries
      developer_repository.go
      usage_repository.go
  jobs/
    usage_rollup.go        -- River daily usage rollup job
web/
  dev/
    templates/             -- login, accept_invitation, dashboard, api_keys
    static/js/             -- dev-login, dev-apikeys, dev-dashboard, dev-api
  admin/
    templates/developers.html
    static/js/developers.js
```

---

## CLI Commands

```bash
server developer invite dev@example.com --name "Developer Name"
server developer list
server developer deactivate <developer-id>
```

---

## GitHub OAuth Configuration

```bash
GITHUB_CLIENT_ID=your-github-oauth-app-client-id
GITHUB_CLIENT_SECRET=your-github-oauth-app-client-secret
GITHUB_CALLBACK_URL=https://toronto.togather.foundation/auth/github/callback
GITHUB_ALLOWED_ORGS=togather-foundation,partner-org  # optional
```

**Required scope:** `user:email`

The OAuth state parameter is a 32-byte random value stored in a short-lived `oauth_state` cookie (5 minutes, HttpOnly, Secure, SameSite=Lax). Validated on callback to prevent CSRF.

---

## Security

### Authentication

- Passwords hashed with bcrypt (cost 12)
- Developer JWTs distinguished from admin JWTs by `"type": "developer"` claim
- `DevCookieAuth` middleware rejects `type != "developer"`
- `AdminCookieAuth` middleware rejects `type == "developer"`

### API Key Ownership

- Developers can only list/revoke keys where `developer_id` matches their own ID
- Key creation always sets `role = 'agent'` and `developer_id` to the authenticated developer's ID

### GitHub OAuth

- State parameter in short-lived cookie prevents CSRF
- Access token used only to fetch profile, then discarded (not stored)
- `GITHUB_ALLOWED_ORGS` optionally restricts which GitHub users can register

### Invitation Tokens

- 32 random bytes, SHA-256 hashed before storage
- 7-day expiry
- One active invitation per email (unique partial index)

### Backwards Compatibility

- Existing API keys continue to work (`developer_id = NULL`)
- Existing admin users are unaffected
- Existing rate limiting is unchanged
