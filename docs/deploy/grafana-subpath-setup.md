# Grafana Subpath Configuration Guide

This guide explains how to configure Grafana to work at `/grafana` subpath on staging/production servers.

## Problem

When Grafana is accessed via a reverse proxy at a subpath (e.g., `/grafana`), it needs to be configured to:
1. Know its public URL includes the subpath
2. Serve content from that subpath (URL rewriting)

Without this configuration:
- Visiting `https://domain.com/grafana/` shows the root page instead of Grafana
- Embedded iframes load the wrong content
- Grafana assets (CSS, JS) fail to load because paths are incorrect

## Solution

Configure both Caddy and Grafana to handle the `/grafana` subpath correctly.

## Configuration Steps

### 1. Update Docker Compose Environment Variables

In your server's `.env` file (usually `/home/deploy/togather/server/.env`), add:

```bash
# Grafana Subpath Configuration
GRAFANA_ROOT_URL=https://staging.toronto.togather.foundation/grafana
GRAFANA_SERVE_FROM_SUB_PATH=true
GRAFANA_PORT=127.0.0.1:3000

# Optional: Enable anonymous access for embedding
GRAFANA_ANONYMOUS_ENABLED=true
GRAFANA_ALLOW_EMBEDDING=true
```

**Environment Variables Explained:**

- `GRAFANA_ROOT_URL` - The full public URL including the subpath
- `GRAFANA_SERVE_FROM_SUB_PATH` - Tells Grafana to serve from `/grafana` instead of `/`
- `GRAFANA_PORT` - Bind to localhost only for security
- `GRAFANA_ANONYMOUS_ENABLED` - Allow viewing dashboards without login (optional)
- `GRAFANA_ALLOW_EMBEDDING` - Allow embedding in iframes (required for admin UI)

### 2. Deploy Updated Caddyfile

The updated `Caddyfile.staging` includes:

```caddy
# Grafana monitoring proxy at /grafana subpath
handle /grafana* {
    reverse_proxy localhost:3000 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}
```

**Key changes:**
- Uses `handle /grafana*` instead of `handle_path /grafana/*`
- `handle` preserves the full path when forwarding to Grafana
- `handle_path` strips the prefix, which breaks Grafana's subpath serving

Deploy the updated Caddyfile:

```bash
# From your local machine
cd /path/to/togather/server
./deploy/scripts/deploy.sh staging --version HEAD
```

This will automatically deploy the updated Caddyfile and reload Caddy.

### 3. Restart Grafana Container

After updating the `.env` file on the server, restart Grafana to pick up the new configuration:

```bash
# SSH to the server
ssh deploy@togather

# Navigate to deployment directory
cd togather/server

# Restart Grafana
docker compose --profile monitoring restart grafana

# Verify Grafana is healthy
docker compose ps grafana
docker compose logs grafana | tail -20
```

### 4. Verify Configuration

Test the Grafana integration:

```bash
# Check Grafana health endpoint
curl -i https://staging.toronto.togather.foundation/grafana/api/health

# Should return:
# HTTP/2 200
# {"commit":"...","database":"ok","version":"..."}

# Check Grafana login page
curl -s https://staging.toronto.togather.foundation/grafana/ | grep -i grafana

# Should contain Grafana UI HTML, not SEL root page
```

### 5. Update Admin UI (if needed)

If the admin dashboard embeds Grafana, ensure the iframe uses the correct path:

```html
<!-- Use relative path -->
<iframe src="/grafana/d/togather-overview?orgId=1&refresh=30s&kiosk=tv"></iframe>

<!-- NOT absolute localhost URL -->
<!-- <iframe src="http://localhost:3000/..."></iframe> -->
```

## Troubleshooting

### Issue: Still seeing root page at /grafana

**Cause:** Grafana hasn't restarted with new environment variables

**Fix:**
```bash
# Check current Grafana config
docker compose exec grafana env | grep GF_SERVER

# Should show:
# GF_SERVER_ROOT_URL=https://staging.toronto.togather.foundation/grafana
# GF_SERVER_SERVE_FROM_SUB_PATH=true

# If not, restart Grafana
docker compose --profile monitoring restart grafana
```

### Issue: 502 Bad Gateway

**Cause:** Grafana container not running or health check failing

**Fix:**
```bash
# Check Grafana status
docker compose ps grafana

# Check Grafana logs
docker compose logs grafana --tail=50

# Restart if needed
docker compose --profile monitoring up -d grafana
```

### Issue: Blank iframe in admin UI

**Possible causes:**
1. `GRAFANA_ALLOW_EMBEDDING` not enabled
2. Anonymous access not enabled (requires login)
3. Dashboard UID incorrect
4. Browser CSP/CORS blocking

**Fix:**
```bash
# Check embedding settings
docker compose exec grafana env | grep GF_SECURITY_ALLOW_EMBEDDING

# Should show: GF_SECURITY_ALLOW_EMBEDDING=true

# Check anonymous access
docker compose exec grafana env | grep GF_AUTH_ANONYMOUS

# Enable if needed
echo "GRAFANA_ANONYMOUS_ENABLED=true" >> .env
docker compose --profile monitoring restart grafana
```

### Issue: Assets not loading (CSS/JS broken)

**Cause:** `SERVE_FROM_SUB_PATH` not enabled or Caddy using wrong directive

**Fix:**
```bash
# Verify Grafana subpath config
docker compose exec grafana env | grep SERVE_FROM_SUB_PATH

# Should show: GF_SERVER_SERVE_FROM_SUB_PATH=true

# Verify Caddy config uses 'handle' not 'handle_path'
ssh deploy@togather
sudo cat /etc/caddy/Caddyfile | grep -A 10 grafana
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Browser                                                     │
│   ↓                                                        │
│ https://staging.toronto.togather.foundation/grafana/      │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ Caddy Reverse Proxy (port 443)                             │
│   - Receives: /grafana/*                                   │
│   - Forwards: /grafana/* (path preserved with 'handle')   │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ Grafana (localhost:3000)                                    │
│   - Root URL: https://staging.../grafana                   │
│   - Serve from subpath: true                               │
│   - Receives: /grafana/*                                   │
│   - Serves: Dashboard at /grafana with correct asset paths │
└─────────────────────────────────────────────────────────────┘
```

## Key Points

1. **Use `handle` not `handle_path` in Caddy** - `handle_path` strips the prefix which breaks subpath serving
2. **Set both environment variables** - Both `ROOT_URL` and `SERVE_FROM_SUB_PATH` are required
3. **Restart Grafana after env changes** - Environment variables only apply on container start
4. **Use relative paths in iframes** - `/grafana/...` not `http://localhost:3000/...`
5. **Localhost-only binding is secure** - Grafana is only accessible via the reverse proxy

## References

- [Grafana Documentation: Run behind a reverse proxy](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/#root_url)
- [Caddy Documentation: handle vs handle_path](https://caddyserver.com/docs/caddyfile/directives/handle)
- [SEL Grafana Embedding Guide](./grafana-embedding.md)
- [SEL Grafana Security Guide](./grafana-security.md)

---

**Last Updated:** 2026-02-20
