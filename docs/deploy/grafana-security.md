# Secure Grafana Embedding - Security Analysis & Options

## Problem with Anonymous Access

The anonymous access approach has critical security flaws:

### Security Issues:
1. **Public metrics exposure**: Anyone who can reach Grafana URL can view dashboards
2. **No authentication boundary**: Grafana becomes publicly accessible if port is exposed
3. **Network-only security**: Relies entirely on firewall rules (not defense-in-depth)
4. **Staging/production risk**: If Grafana is accidentally exposed (misconfigured proxy, port forward), metrics are public

### Attack Scenarios:
- Port scanning discovers Grafana on 3000
- Misconfigured reverse proxy exposes Grafana subdomain
- Internal network access (compromised device, malicious insider)
- Cloud environment with permissive security groups

**Verdict: Anonymous access is NOT recommended for production use.**

---

## Secure Alternatives

### Option 1: JWT Authentication (RECOMMENDED)

**How it works:**
1. Admin UI backend generates a JWT token for authenticated admin user
2. Token is passed to Grafana via URL parameter: `?auth_token=<jwt>`
3. Grafana validates JWT and authenticates as that user
4. User sees only dashboards they have permission to access

**Security benefits:**
- ✅ No anonymous access required
- ✅ Uses existing admin authentication
- ✅ Grafana validates token signature
- ✅ Tokens can expire (short-lived)
- ✅ Defense-in-depth (multiple auth layers)

**Implementation:**

```ini
# Grafana configuration (custom.ini or env vars)
[auth.jwt]
enabled = true
header_name = X-JWT-Assertion
url_login = true                    # Allow JWT in URL ?auth_token= param
username_claim = sub
email_claim = email
jwk_set_file = /etc/grafana/jwks.json
cache_ttl = 60m
expect_claims = {"iss": "togather-sel"}
auto_sign_up = true
role_attribute_path = contains(role, 'admin') && 'Admin' || 'Viewer'
```

**Backend changes:**
```go
// Generate JWT for Grafana access
func (h *AdminHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
    // Get authenticated admin user from session
    user := getAuthenticatedUser(r)
    
    // Generate short-lived JWT for Grafana (5 min expiry)
    grafanaToken, err := generateGrafanaJWT(user, 5*time.Minute)
    if err != nil {
        // Handle error
    }
    
    data := map[string]interface{}{
        "Title":         "Dashboard - SEL Admin",
        "ActivePage":    "dashboard",
        "GrafanaURL":    os.Getenv("GRAFANA_ROOT_URL"),
        "GrafanaToken":  grafanaToken,  // Pass to template
    }
    
    h.renderTemplate(w, "dashboard.html", data)
}
```

**Frontend iframe:**
```html
<iframe 
    src="{{ .GrafanaURL }}/d/togather-overview?auth_token={{ .GrafanaToken }}&refresh=30s&kiosk=tv"
    frameborder="0">
</iframe>
```

**Pros:**
- Secure authentication flow
- No anonymous access needed
- Token expiry limits exposure window
- Works with existing admin auth

**Cons:**
- Requires JWT key management
- Additional backend code
- Token refresh logic needed for long sessions

---

### Option 2: Reverse Proxy Authentication

**How it works:**
1. Grafana sits behind reverse proxy (Caddy/Nginx)
2. Proxy validates admin session cookie
3. Proxy adds auth headers (X-WEBAUTH-USER) to requests
4. Grafana trusts proxy headers (auth proxy mode)

**Security benefits:**
- ✅ No anonymous access
- ✅ Single authentication point
- ✅ No token management
- ✅ Leverages existing session auth

**Implementation:**

```caddy
# Caddyfile - Grafana behind authenticated proxy
grafana.staging.toronto.togather.foundation {
    # Require admin session cookie
    @authenticated {
        cookie admin_session *
    }
    
    # Reject unauthenticated requests
    handle @authenticated {
        reverse_proxy localhost:3000 {
            # Pass authenticated user to Grafana
            header_up X-WEBAUTH-USER {http.request.cookie.admin_username}
            header_up X-WEBAUTH-EMAIL {http.request.cookie.admin_email}
        }
    }
    
    respond 401
}
```

```ini
# Grafana auth proxy config
[auth.proxy]
enabled = true
header_name = X-WEBAUTH-USER
header_property = username
auto_sign_up = true
whitelist = 127.0.0.1
headers = Email:X-WEBAUTH-EMAIL
```

**Pros:**
- Simple configuration
- No token management
- Leverages existing auth

**Cons:**
- Requires separate Grafana subdomain
- Proxy must validate sessions
- More complex Caddy config

---

### Option 3: Network Isolation (Current with Restrictions)

**How it works:**
1. Keep anonymous access enabled
2. Bind Grafana to localhost ONLY (not 0.0.0.0)
3. Only accessible from same host
4. Admin UI on same host can embed

**Configuration:**
```yaml
# docker-compose.blue-green.yml
grafana:
  ports:
    # Bind to localhost only - NOT accessible externally
    - "127.0.0.1:3000:3000"
  
  environment:
    GF_SERVER_ROOT_URL: http://localhost:3000
    GF_AUTH_ANONYMOUS_ENABLED: "true"
    GF_AUTH_ANONYMOUS_ORG_ROLE: "Viewer"
```

**Security benefits:**
- ✅ Not exposed to network
- ✅ Only localhost can access
- ✅ Simple configuration

**Limitations:**
- ❌ Only works if admin UI and Grafana on same host
- ❌ Can't separate services across servers
- ❌ SSH tunneling could bypass restrictions
- ❌ Doesn't work for remote browser access (need proxy)

**When this works:**
- Development: Browser, admin UI, Grafana all on localhost
- Production: Browser accesses admin UI via proxy, admin UI embeds Grafana on localhost

**When this DOESN'T work:**
- Browser directly accessing Grafana URL (shows unavailable)
- Grafana on different server than admin UI
- Client-side JavaScript trying to check availability

---

### Option 4: Link-Only (No Embedding) - SIMPLEST & SECURE

**How it works:**
1. Don't embed Grafana in iframe
2. Provide link to Grafana
3. User logs into Grafana separately
4. Grafana uses its own authentication

**Implementation:**
```html
<!-- Just a link, no iframe -->
<a href="{{ .GrafanaURL }}/d/togather-overview" target="_blank" class="btn btn-primary">
    Open Grafana Dashboard
</a>
```

```yaml
# docker-compose.blue-green.yml
grafana:
  environment:
    # No anonymous access
    GF_AUTH_ANONYMOUS_ENABLED: "false"
    
    # Separate Grafana auth
    GF_SECURITY_ADMIN_USER: ${GRAFANA_ADMIN_USER}
    GF_SECURITY_ADMIN_PASSWORD: ${GRAFANA_ADMIN_PASSWORD}
```

**Security benefits:**
- ✅ Separate authentication boundary
- ✅ Grafana manages its own users
- ✅ No embedding complexity
- ✅ No anonymous access

**Pros:**
- Simplest implementation
- Most secure (separate auth)
- No token management
- Works everywhere

**Cons:**
- User must log in twice (admin UI + Grafana)
- No embedded view (just link)
- Separate user management

---

## Recommended Approach by Environment

### Development (Localhost)
**Use:** Network Isolation (Option 3)
- Bind Grafana to `127.0.0.1:3000`
- Enable anonymous access (localhost only)
- Embed in admin UI

```bash
GRAFANA_PORT=127.0.0.1:3000
GRAFANA_ANONYMOUS_ENABLED=true
```

### Staging
**Use:** JWT Authentication (Option 1) OR Link-Only (Option 4)
- JWT if you want embedded view
- Link-only for simplicity

### Production
**Use:** Link-Only (Option 4) OR JWT Authentication (Option 1)
- Link-only is simplest and most secure
- JWT if embedded view is critical

**DO NOT USE anonymous access in production/staging unless:**
- Grafana is 100% localhost-only (not exposed via proxy)
- AND you have defense-in-depth (network segmentation, VPN, etc.)

---

## Implementation Plan

I recommend implementing **Option 4 (Link-Only) by default** with **Option 1 (JWT) as opt-in** for those who want embedding.

### Phase 1: Secure by Default (Link-Only)
1. Remove anonymous access from default config
2. Keep embedded iframe code but hide by default
3. Show "Open in Grafana" button
4. Grafana uses separate authentication

### Phase 2: JWT Embedding (Optional)
1. Add JWT token generation to admin backend
2. Document JWT setup in grafana-embedding.md
3. Enable iframe when JWT is configured

This gives security by default with opt-in complexity for embedding.

Would you like me to implement this approach?

---

**Last Updated:** 2026-02-20
