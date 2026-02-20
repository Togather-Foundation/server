# Grafana Localhost-Only Binding - Security Analysis

## Question: Is binding Grafana to localhost sufficient for security?

**Short answer: YES, for server-side iframe embedding. NO, for client-side JavaScript checks.**

## How Localhost Binding Works

```yaml
# docker-compose.blue-green.yml
grafana:
  ports:
    - "127.0.0.1:3000:3000"  # Only localhost can connect
```

This means:
- ✅ External network requests to `server-ip:3000` → **REJECTED**
- ✅ Requests from other containers → **REJECTED** (unless on same network)
- ✅ Requests from `localhost:3000` on the server → **ALLOWED**
- ✅ SSH tunneling required for remote access

## Security Analysis

### ✅ What This DOES Protect Against:

1. **Direct external access**
   ```bash
   # From internet - FAILS
   curl http://staging.toronto.togather.foundation:3000
   # Connection refused
   ```

2. **Port scanning**
   ```bash
   # Nmap scan - port appears closed
   nmap -p 3000 staging.toronto.togather.foundation
   # 3000/tcp closed
   ```

3. **Unauthorized network access**
   - Even if firewall is misconfigured, port is not listening on external interface
   - Defense in depth: firewall + bind address

4. **Accidental exposure**
   - Can't accidentally expose via reverse proxy without SSH tunnel
   - Must explicitly forward traffic from proxy to localhost

### ❌ What This DOESN'T Protect Against:

1. **Server-side compromise**
   - If attacker gains shell access on server, can access localhost
   - But they already have access to everything at that point

2. **SSH tunneling** (but requires server access)
   ```bash
   ssh -L 3000:localhost:3000 user@server
   # Now can access via http://localhost:3000
   ```

3. **Malicious admin user**
   - Admin with SSH access can tunnel to Grafana
   - But admin already has access to everything

## Iframe Embedding Architecture

### Server-Side Rendering (SSR) - Works Perfectly ✅

```
[Browser] → [Admin UI Server] → [Grafana on localhost:3000]
            (on same host)
```

1. Browser requests: `https://staging.toronto.togather.foundation/admin/dashboard`
2. Admin UI server (Go backend) renders HTML with iframe:
   ```html
   <iframe src="http://localhost:3000/d/dashboard"></iframe>
   ```
3. Browser loads iframe, makes request to `http://localhost:3000`
4. **This request comes from browser, NOT from server**

**Problem**: Browser can't reach `localhost:3000` because that's the browser's localhost, not the server's localhost!

### Client-Side Iframe (Current Implementation) - Doesn't Work ❌

```
[Browser] → tries to load → http://localhost:3000 (browser's localhost)
                             ↓
                          [FAILS - nothing running on browser's localhost]
```

The iframe src is interpreted by the **client browser**, which tries to connect to its own `localhost:3000`, not the server's.

## Solutions

### Solution 1: Reverse Proxy with Localhost Backend (RECOMMENDED) ✅

**Architecture:**
```
[Browser] → https://staging.toronto.togather.foundation/grafana/...
            ↓ (Caddy reverse proxy)
            → http://localhost:3000 (server-side connection)
```

**Caddy configuration:**
```caddy
staging.toronto.togather.foundation {
    # Admin UI
    reverse_proxy localhost:8081
    
    # Grafana (proxied to localhost:3000)
    handle_path /grafana/* {
        # Only allow if admin session cookie present
        @authenticated {
            cookie admin_session *
        }
        
        handle @authenticated {
            reverse_proxy localhost:3000
        }
        
        respond 401
    }
}
```

**Iframe in admin UI:**
```html
<iframe src="/grafana/d/togather-overview?kiosk=tv"></iframe>
```

**Security:**
- ✅ Grafana bound to localhost (not externally accessible)
- ✅ Only accessible via authenticated reverse proxy
- ✅ Browser connects to proxy (same domain), proxy connects to localhost
- ✅ No anonymous access needed
- ✅ Defense in depth (network + authentication)

**Docker port binding:**
```yaml
grafana:
  ports:
    - "127.0.0.1:3000:3000"  # localhost only
```

### Solution 2: Separate Grafana Domain with Auth Proxy ✅

**Architecture:**
```
[Browser] → https://grafana.staging.toronto.togather.foundation
            ↓ (Caddy validates admin session)
            → http://localhost:3000
```

**Caddy configuration:**
```caddy
# Grafana subdomain (requires admin auth)
grafana.staging.toronto.togather.foundation {
    # Check for admin session cookie
    @authenticated {
        cookie admin_session *
    }
    
    handle @authenticated {
        reverse_proxy localhost:3000 {
            # Pass user identity to Grafana
            header_up X-WEBAUTH-USER {http.request.cookie.admin_user}
        }
    }
    
    respond "Unauthorized" 401
}
```

**Grafana auth proxy config:**
```bash
GF_AUTH_PROXY_ENABLED=true
GF_AUTH_PROXY_HEADER_NAME=X-WEBAUTH-USER
GF_AUTH_PROXY_AUTO_SIGN_UP=true
```

**Security:**
- ✅ Grafana bound to localhost
- ✅ Proxy validates admin session before forwarding
- ✅ Grafana automatically logs in user via auth proxy
- ✅ No anonymous access

### Solution 3: Link-Only (No Embedding) ✅

**Implementation:**
```html
<!-- Just a link to Grafana via reverse proxy -->
<a href="/grafana/d/togather-overview" target="_blank" class="btn btn-primary">
    Open Grafana Dashboard
</a>
```

**Security:**
- ✅ Grafana bound to localhost
- ✅ Accessed via authenticated reverse proxy
- ✅ Separate login (or SSO via auth proxy)
- ✅ Simplest implementation

### Solution 4: Development Only - Localhost with Anonymous Access

**When this works:**
- Running everything on same machine (dev laptop)
- Browser, admin UI, Grafana all on localhost

**Configuration:**
```bash
# .env for development
GRAFANA_PORT=127.0.0.1:3000
GRAFANA_ANONYMOUS_ENABLED=true
GRAFANA_ROOT_URL=http://localhost:3000
```

**Security:**
- ✅ Safe for development (all localhost)
- ❌ DON'T use in staging/production

## Recommendation

**For Staging/Production:**

Use **Solution 1** (Reverse Proxy with Authentication):

1. Bind Grafana to `127.0.0.1:3000` (localhost only)
2. Configure Caddy to proxy `/grafana/*` → `localhost:3000`
3. Add authentication check in Caddy (verify admin session)
4. Update iframe src to `/grafana/d/dashboard`

This gives:
- ✅ Grafana never directly exposed to network
- ✅ Only accessible via authenticated admin UI proxy
- ✅ Embedded iframe works (browser connects to proxy, proxy connects to localhost)
- ✅ No anonymous access needed
- ✅ Multiple layers of security

**Configuration:**
```yaml
# docker-compose
grafana:
  ports:
    - "127.0.0.1:3000:3000"  # localhost only
  environment:
    GF_AUTH_ANONYMOUS_ENABLED: "false"  # no anonymous access
    GF_SERVER_ROOT_URL: "https://staging.toronto.togather.foundation/grafana"
```

```caddy
# Caddyfile.staging
staging.toronto.togather.foundation {
    # Main app
    reverse_proxy localhost:8081
    
    # Grafana proxy (requires admin auth)
    handle_path /grafana/* {
        @authenticated cookie admin_session *
        handle @authenticated {
            reverse_proxy localhost:3000
        }
        respond 401
    }
}
```

```html
<!-- Admin dashboard template -->
<iframe src="/grafana/d/togather-overview?kiosk=tv"></iframe>
```

## Answer to Original Question

> "If Grafana only accepts connections from localhost, will that be enough?"

**Yes, IF you use a reverse proxy** to bridge between browser and localhost.

**No, if you try to embed `http://localhost:3000` directly** - browser's localhost ≠ server's localhost.

The localhost binding IS sufficient security-wise, but requires proper proxy architecture for embedding to work.

---

**Last Updated:** 2026-02-20
