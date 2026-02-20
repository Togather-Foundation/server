# Grafana

Grafana and Prometheus are optional monitoring services controlled by Docker Compose profiles.

## Docker Setup

```bash
# Start with monitoring
docker compose -f deploy/docker/docker-compose.yml \
  -f deploy/docker/docker-compose.blue-green.yml \
  --profile monitoring up -d

# Start without monitoring (default)
docker compose -f deploy/docker/docker-compose.yml \
  -f deploy/docker/docker-compose.blue-green.yml up -d
```

**Default ports:**
- Grafana: `localhost:3000` (configurable via `GRAFANA_PORT`)
- Prometheus: `localhost:9090` (configurable via `PROMETHEUS_PORT`)

## Configuration

Set in your `.env` file:

```bash
GRAFANA_PORT=3000
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=<strong-password>
GRAFANA_ROOT_URL=http://localhost:3000
GRAFANA_ANONYMOUS_ENABLED=true
GRAFANA_ANONYMOUS_ORG_NAME=Main Org.
GRAFANA_ANONYMOUS_ORG_ROLE=Viewer
GRAFANA_ALLOW_EMBEDDING=true
PROMETHEUS_PORT=9090
```

`GRAFANA_ALLOW_EMBEDDING=true` sets `GF_SECURITY_ALLOW_EMBEDDING=true` in Grafana's configuration, required for iframe embedding in the admin UI.

## Subpath Proxy Setup (Staging/Production)

For staging/production, bind Grafana to localhost and proxy it at `/grafana`:

```bash
# .env
GRAFANA_ROOT_URL=https://staging.toronto.togather.foundation/grafana
GRAFANA_SERVE_FROM_SUB_PATH=true
GRAFANA_PORT=127.0.0.1:3000
GRAFANA_ANONYMOUS_ENABLED=true
GRAFANA_ALLOW_EMBEDDING=true
```

**Caddy configuration:**

```caddy
staging.toronto.togather.foundation {
    handle /grafana* {
        reverse_proxy localhost:3000 {
            header_up Host {host}
            header_up X-Real-IP {remote_host}
            header_up X-Forwarded-For {remote_host}
            header_up X-Forwarded-Proto {scheme}
        }
    }

    reverse_proxy localhost:8081
}
```

**Critical:** Use `handle /grafana*` (not `handle_path /grafana/*`). `handle` preserves the full path when forwarding to Grafana. `handle_path` strips the prefix, which breaks Grafana's subpath asset serving.

After updating `.env` on the server, restart Grafana:

```bash
docker compose --profile monitoring restart grafana
```

Verify:

```bash
curl -i https://staging.toronto.togather.foundation/grafana/api/health
# {"commit":"...","database":"ok","version":"..."}
```

## Embedding in Admin UI

The admin dashboard (`/admin/dashboard`) embeds Grafana via iframe. The JavaScript availability check tries to fetch the Grafana URL with a 3-second timeout and shows/hides the iframe accordingly.

**Iframe URL parameters:**
- `orgId=1` — default organization
- `refresh=30s` — auto-refresh interval
- `kiosk=tv` — hides Grafana chrome, shows only dashboard content

For localhost development, the iframe uses the absolute URL:
```html
<iframe src="http://localhost:3000/d/togather-overview?orgId=1&refresh=30s&kiosk=tv"></iframe>
```

For staging/production with subpath proxy, use a relative path:
```html
<iframe src="/grafana/d/togather-overview?orgId=1&refresh=30s&kiosk=tv"></iframe>
```

**Security note:** `localhost:3000` in an iframe src is interpreted by the browser, not the server. If Grafana is bound to `127.0.0.1:3000` on the server, the browser cannot reach it directly. A reverse proxy (as above) is required for iframe embedding in staging/production.

The Go backend passes the Grafana URL to the template via `GRAFANA_ROOT_URL`:

```go
// internal/api/handlers/admin_html.go
data := map[string]interface{}{
    "GrafanaURL":    os.Getenv("GRAFANA_ROOT_URL"),
    "PrometheusURL": os.Getenv("PROMETHEUS_ROOT_URL"),
}
```

## Security

Grafana is bound to `127.0.0.1:3000` in staging/production (`GRAFANA_PORT=127.0.0.1:3000`). This prevents direct external access — port 3000 is not exposed on the external interface.

The `/grafana` proxy in Caddy is the only external access path. To add authentication in front of it:

```caddy
handle /grafana* {
    @authenticated cookie admin_session *
    handle @authenticated {
        reverse_proxy localhost:3000 { ... }
    }
    respond 401
}
```

Alternatively, Grafana supports JWT authentication (`[auth.jwt]`) or auth proxy headers (`[auth.proxy]`) if per-user Grafana authentication is needed.

For anonymous access (current default): anonymous users receive the `Viewer` role (read-only). Only dashboards in the anonymous user's organization are visible. This is acceptable when Grafana is only reachable via an authenticated admin proxy.

## Dashboard Design Guidelines

### Color Scheme

Use **both color and line style** to distinguish blue/green deployment slots — provides two visual cues, accessible for colorblind users and print.

| Priority  | Line Style          | Blue      | Green     |
|-----------|---------------------|-----------|-----------|
| Primary   | Solid               | `#1f77b4` | `#2ca02c` |
| Secondary | Dashed `[10, 5]`    | `#5da5da` | `#6ec16e` |
| Tertiary  | Dotted `[2, 4]`     | `#aec7e8` | `#b1d8b1` |

Line width: `2px`. Legend format: `{{slot}} - MetricName`.

### Active Slot Indicator

The dashboard includes an "Active Slot" panel driven by `togather_app_info{active_slot="true"}`.

- Set `BLUE_ACTIVE_SLOT=true` / `GREEN_ACTIVE_SLOT=false` in docker-compose for the active slot
- Panel query: `togather_app_info{slot=~"$slot", active_slot="true"}`

### Panel Patterns

**Single metric per slot** (Request Rate, Goroutines): primary color, solid line.

**Multiple metrics per slot:**
- First (Total, p50): dark color, solid
- Second (In Use, p95): medium color, dashed
- Third (Idle, p99): light color, dotted

**Examples:**
- Database Connections: Total/In Use/Idle per slot
- Request Latency: p50/p95/p99 per slot
- Error Rate: 5xx (solid) / 4xx (dashed) per slot

### Field Override JSON

```json
{
  "matcher": {"id": "byRegexp", "options": ".*blue.*Total"},
  "properties": [
    {"id": "color", "value": {"fixedColor": "#1f77b4", "mode": "fixed"}},
    {"id": "custom.lineStyle", "value": {"fill": "solid"}},
    {"id": "custom.lineWidth", "value": 2}
  ]
}
```

### Automation Script

Apply the color scheme to an existing dashboard JSON:

```bash
python3 deploy/scripts/update-dashboard-colors.py \
  deploy/config/grafana/dashboards/json/togather-overview.json
```

## Troubleshooting

### Dashboard shows "Unavailable"

```bash
docker ps | grep grafana
docker compose --profile monitoring up -d
docker compose logs grafana
```

### Iframe shows Grafana login screen

Anonymous access is not enabled:

```bash
echo "GRAFANA_ANONYMOUS_ENABLED=true" >> .env
docker compose --profile monitoring restart grafana
```

### Iframe shows blank or error

```bash
docker compose exec grafana env | grep GF_SECURITY_ALLOW_EMBEDDING
# Should show: GF_SECURITY_ALLOW_EMBEDDING=true
```

If not set:
```bash
echo "GRAFANA_ALLOW_EMBEDDING=true" >> .env
docker compose --profile monitoring restart grafana
```

### Still seeing root page at /grafana (subpath)

Grafana hasn't restarted with new env vars, or Caddy is using `handle_path` instead of `handle`:

```bash
docker compose exec grafana env | grep GF_SERVER
# GF_SERVER_ROOT_URL=https://.../grafana
# GF_SERVER_SERVE_FROM_SUB_PATH=true

ssh deploy@togather sudo cat /etc/caddy/Caddyfile | grep -A5 grafana
# Confirm 'handle /grafana*' not 'handle_path'
```

### Dashboard UID not found

```bash
docker compose exec grafana ls /etc/grafana/provisioning/dashboards/json/
# Should list: togather-overview.json
```

To find a dashboard UID: open Grafana, navigate to the dashboard, check the URL `/d/<UID>/dashboard-name`.

### 502 Bad Gateway

```bash
docker compose ps grafana
docker compose logs grafana --tail=50
docker compose --profile monitoring up -d grafana
```

### Assets not loading (CSS/JS broken)

Verify `SERVE_FROM_SUB_PATH` is set and Caddy uses `handle` not `handle_path` (see above).
