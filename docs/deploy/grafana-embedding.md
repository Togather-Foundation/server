# Grafana Dashboard Embedding in Admin UI

This document explains how Grafana dashboards are integrated into the SEL Admin UI, including configuration options and troubleshooting.

## Overview

The admin dashboard (`/admin/dashboard`) includes:
1. **Links to monitoring services** - Grafana and Prometheus with availability status
2. **Embedded Grafana dashboard** - Real-time metrics displayed directly in the admin UI (when available)

## Architecture

### Docker Compose Configuration

Grafana and Prometheus are optional services controlled by Docker Compose profiles:

```bash
# Start with monitoring (includes Grafana + Prometheus)
docker compose -f deploy/docker/docker-compose.yml \
  -f deploy/docker/docker-compose.blue-green.yml \
  --profile monitoring up -d

# Start without monitoring (default)
docker compose -f deploy/docker/docker-compose.yml \
  -f deploy/docker/docker-compose.blue-green.yml up -d
```

### Service Ports

- **Grafana**: `localhost:3000` (default, configurable via `GRAFANA_PORT`)
- **Prometheus**: `localhost:9090` (default, configurable via `PROMETHEUS_PORT`)

### Dashboard Embedding

The admin UI uses an iframe to embed the Grafana dashboard:

```html
<iframe 
  src="http://localhost:3000/d/togather-overview?orgId=1&refresh=30s&kiosk=tv"
  frameborder="0">
</iframe>
```

**URL Parameters:**
- `orgId=1` - Organization ID (default org)
- `refresh=30s` - Auto-refresh every 30 seconds
- `kiosk=tv` - TV mode (hides Grafana UI chrome, shows only dashboard content)

## Configuration Options

### Environment Variables

Set these in your `.env` file:

```bash
# Grafana Configuration
GRAFANA_PORT=3000                           # Port for Grafana UI
GRAFANA_ADMIN_USER=admin                    # Admin username
GRAFANA_ADMIN_PASSWORD=<strong-password>    # Admin password (change default!)
GRAFANA_ROOT_URL=http://localhost:3000      # Base URL for Grafana

# Anonymous Access (required for embedding without login)
GRAFANA_ANONYMOUS_ENABLED=true              # Enable anonymous access
GRAFANA_ANONYMOUS_ORG_NAME=Main Org.        # Default organization for anonymous users
GRAFANA_ANONYMOUS_ORG_ROLE=Viewer           # Role for anonymous users (Viewer, Editor, Admin)

# Iframe Embedding (required for embedding in admin UI)
GRAFANA_ALLOW_EMBEDDING=true                # Allow iframe embedding

# Prometheus Configuration
PROMETHEUS_PORT=9090                        # Port for Prometheus UI
```

### Anonymous Access

**Why enable anonymous access?**

When you embed a Grafana dashboard in an iframe, users need to authenticate to Grafana to view it. There are two options:

1. **Require separate Grafana login** (default) - Users click through to Grafana and log in separately
2. **Enable anonymous access** (recommended for embedding) - Users see dashboards without Grafana login

**Enabling anonymous access:**

```bash
# In .env file
GRAFANA_ANONYMOUS_ENABLED=true
GRAFANA_ANONYMOUS_ORG_ROLE=Viewer

# Restart Grafana
docker compose --profile monitoring restart grafana
```

**Security considerations:**
- Anonymous users only get `Viewer` role (read-only access)
- Only dashboards in the anonymous user's organization are visible
- You can disable anonymous access after initial setup if needed

### Iframe Embedding

Grafana must explicitly allow iframe embedding for security reasons:

```bash
# In .env file (already enabled by default)
GRAFANA_ALLOW_EMBEDDING=true
```

This sets `GF_SECURITY_ALLOW_EMBEDDING=true` in Grafana's configuration.

## Dashboard Availability Detection

The admin UI automatically detects if Grafana is available:

1. **JavaScript check** - Tries to fetch Grafana URL with 3-second timeout
2. **Status badge** - Shows "Available", "Unavailable", or "Timeout"
3. **Conditional display** - Embedded iframe only shows if Grafana is available

**Status indicators:**
- 🟢 **Available** - Grafana is reachable, iframe will be displayed
- 🟡 **Timeout** - Grafana took >3 seconds to respond
- 🔴 **Unavailable** - Grafana is not running or not reachable

## Production/Staging Setup

### Option 1: Internal Access (Docker Network)

For development/staging where admin UI and Grafana are on the same server:

```bash
# .env file
GRAFANA_ROOT_URL=http://localhost:3000
GRAFANA_ANONYMOUS_ENABLED=true
```

### Option 2: Public URL with Reverse Proxy

For production where Grafana is behind Caddy/Nginx:

```bash
# .env file
GRAFANA_ROOT_URL=https://grafana.staging.toronto.togather.foundation
GRAFANA_ANONYMOUS_ENABLED=true
```

**Caddy configuration** (`Caddyfile.staging`):

```caddy
# Grafana subdomain
grafana.staging.toronto.togather.foundation {
    reverse_proxy localhost:3000
    
    # Security headers
    header {
        X-Frame-Options "SAMEORIGIN"
        Content-Security-Policy "frame-ancestors 'self' staging.toronto.togather.foundation"
    }
}
```

This allows:
- Direct access: `https://grafana.staging.toronto.togather.foundation`
- Iframe embedding: from `staging.toronto.togather.foundation` domain

## Troubleshooting

### Dashboard shows "Unavailable"

**Possible causes:**
1. Monitoring services not started with `--profile monitoring`
2. Grafana container not running
3. Network connectivity issues

**Fix:**
```bash
# Check if Grafana is running
docker ps | grep grafana

# Start monitoring services
docker compose --profile monitoring up -d

# Check Grafana logs
docker compose logs grafana
```

### Iframe shows Grafana login screen

**Cause:** Anonymous access is not enabled

**Fix:**
```bash
# Enable anonymous access in .env
echo "GRAFANA_ANONYMOUS_ENABLED=true" >> .env

# Restart Grafana
docker compose --profile monitoring restart grafana
```

### Iframe shows blank/error

**Possible causes:**
1. `allow_embedding` is disabled
2. Dashboard UID is incorrect
3. CORS or CSP blocking iframe

**Fix:**
```bash
# Check Grafana configuration
docker compose exec grafana cat /etc/grafana/grafana.ini | grep allow_embedding

# Should show: allow_embedding = true
# If not, add to .env:
echo "GRAFANA_ALLOW_EMBEDDING=true" >> .env
docker compose --profile monitoring restart grafana
```

### Dashboard UID not found

**Cause:** The dashboard may not be provisioned yet

**Fix:**
```bash
# Check if dashboards are provisioned
docker compose exec grafana ls -la /etc/grafana/provisioning/dashboards/json/

# Should show: togather-overview.json

# Check dashboard in Grafana UI
open http://localhost:3000/dashboards
# Look for "Togather Overview" dashboard
```

To get the correct dashboard UID:
1. Open Grafana: `http://localhost:3000`
2. Navigate to dashboard
3. Check URL: `/d/<UID>/dashboard-name`
4. Update iframe src in `dashboard.html` template

## Backend Integration

To pass custom Grafana URLs from the Go backend:

```go
// In internal/api/handlers/admin_html.go
func (h *AdminHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
    data := map[string]interface{}{
        "Title":         "Dashboard - SEL Admin",
        "ActivePage":    "dashboard",
        "GrafanaURL":    os.Getenv("GRAFANA_ROOT_URL"),      // e.g., "http://localhost:3000"
        "PrometheusURL": os.Getenv("PROMETHEUS_ROOT_URL"),   // e.g., "http://localhost:9090"
    }
    
    h.renderTemplate(w, "dashboard.html", data)
}
```

## Dashboard Customization

### Change Refresh Rate

Edit the iframe src in `web/admin/templates/dashboard.html`:

```html
<!-- Refresh every 30 seconds (default) -->
src="http://localhost:3000/d/togather-overview?refresh=30s&kiosk=tv"

<!-- Refresh every 10 seconds -->
src="http://localhost:3000/d/togather-overview?refresh=10s&kiosk=tv"

<!-- No auto-refresh -->
src="http://localhost:3000/d/togather-overview?kiosk=tv"
```

### Change Dashboard

To embed a different dashboard:

1. Find dashboard UID in Grafana
2. Update iframe src: `/d/<new-uid>/<dashboard-name>`

### Add Multiple Dashboards

Create tabs or accordion to switch between dashboards:

```html
<ul class="nav nav-tabs">
  <li class="nav-item">
    <a class="nav-link active" data-dashboard="togather-overview">Overview</a>
  </li>
  <li class="nav-item">
    <a class="nav-link" data-dashboard="togather-events">Events</a>
  </li>
</ul>

<iframe id="dashboard-iframe" src="..."></iframe>

<script>
  document.querySelectorAll('[data-dashboard]').forEach(tab => {
    tab.addEventListener('click', (e) => {
      const dashboard = e.target.dataset.dashboard;
      document.getElementById('dashboard-iframe').src = 
        `http://localhost:3000/d/${dashboard}?refresh=30s&kiosk=tv`;
    });
  });
</script>
```

## References

- **Grafana Embedding Docs**: https://grafana.com/docs/grafana/latest/dashboards/share-dashboards-panels/
- **Anonymous Access**: https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-authentication/grafana/#anonymous-authentication
- **Docker Compose**: `deploy/docker/docker-compose.blue-green.yml`
- **Dashboard Provisioning**: `deploy/config/grafana/dashboards/`
