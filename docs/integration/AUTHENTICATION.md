# Authentication Guide

**Version:** 0.1.0  
**Date:** 2026-01-27  
**Status:** Living Document

This document provides comprehensive authentication and authorization guidance for integrating with the Togather SEL API. It covers API key management, role-based access control, rate limiting, and security best practices.

For API endpoint details, see [API_GUIDE.md](API_GUIDE.md). For security architecture, see [../contributors/SECURITY.md](../contributors/SECURITY.md).

---

## Table of Contents

1. [Authentication Overview](#authentication-overview)
2. [Roles and Permissions](#roles-and-permissions)
3. [API Keys](#api-keys)
4. [JWT Authentication](#jwt-authentication)
5. [Rate Limiting](#rate-limiting)
6. [Making Authenticated Requests](#making-authenticated-requests)
7. [Error Handling](#error-handling)
8. [Security Best Practices](#security-best-practices)
9. [Troubleshooting](#troubleshooting)

---

## Authentication Overview

The SEL API uses **two authentication methods** depending on your use case:

| Method | Use Case | Lifetime | Example Users |
|--------|----------|----------|---------------|
| **API Keys** | Long-lived integrations (scrapers, batch jobs) | No expiration (rotatable) | Event scrapers, data feeds |
| **JWT Tokens** | Interactive sessions (admin dashboard, admin API) | 24 hours (configurable) | Admin users |

### Public Access

**No authentication required** for:
- Reading published events (`GET /api/v1/events`)
- Viewing event details (`GET /api/v1/events/{id}`)
- Reading places and organizations

Public endpoints are **rate-limited** to 60 requests/minute per IP address.

### Authenticated Access

**Authentication required** for:
- Creating events (`POST /api/v1/events`)
- Updating events (`PUT /api/v1/events/{id}`)
- Deleting events (`DELETE /api/v1/admin/events/{id}`)
- Admin operations (`/api/v1/admin/*`)

---

## Roles and Permissions

SEL uses **Role-Based Access Control (RBAC)** with three roles:

### Public (Unauthenticated)

**Permissions:**
- ✅ Read published events, places, organizations
- ✅ Search events (semantic and full-text)
- ✅ Export public data (JSON-LD, N-Triples)
- ❌ Create, update, or delete any resources

**Rate Limit**: 60 requests/minute

**Use Case**: Public web apps, mobile apps, general consumers

### Agent (Authenticated)

**Permissions:**
- ✅ All public permissions
- ✅ Create events (`POST /api/v1/events`)
- ✅ Create places and organizations (via event creation)
- ✅ Submit events (`POST /api/v1/events`)
- ❌ Update or delete events created by others
- ❌ Access admin endpoints

**Rate Limit**: 300 requests/minute

**Use Case**: Event scrapers, partner integrations, data feeds

### Admin (Authenticated)

**Permissions:**
- ✅ All agent permissions
- ✅ Delete events (`DELETE /api/v1/admin/events/{id}`)
- ✅ Create API keys (`POST /api/v1/admin/api-keys`)
- ✅ Configure federation nodes
- ✅ View system metrics and logs

**Rate Limit**: Unlimited (0 = no limit)

**Use Case**: System administrators, DevOps

---

## API Keys

API keys are the recommended authentication method for **long-lived integrations** like scrapers and batch jobs.

### Obtaining an API Key

**Contact the SEL administrator** to request an API key. Provide:
- **Organization name**: Your organization or project name
- **Use case**: Description of what you'll be doing (e.g., "Scraping events from City Arts Calendar")
- **Expected volume**: Approximate requests per hour/day
- **Contact email**: For notifications about API changes

**Response**: You'll receive:
- **API Key**: `sel_live_abcdef1234567890...` (64-character string)
- **Role**: `agent` (or `admin` if appropriate)
- **Rate Limit**: Assigned tier (usually 300 req/min for agents)

**⚠️ IMPORTANT**: API keys are shown **only once**. Store them securely (environment variables, secret manager).

### Using API Keys

Include your API key in the `Authorization` header with `Bearer` scheme:

```bash
curl -H "Authorization: Bearer sel_live_abcdef1234567890..." \
     -H "Content-Type: application/ld+json" \
     -X POST \
     https://toronto.togather.foundation/api/v1/events \
     -d @event.json
```

**Header Format**:
```
Authorization: Bearer <api_key>
```

### API Key Security

**Best Practices:**

1. **Never commit keys to version control**
   - Use `.env` files (added to `.gitignore`)
   - Use environment variables or secret managers

2. **Store keys securely**
   ```bash
   # Good: Environment variable
   export SEL_API_KEY="sel_live_abcdef1234567890..."
   
   # Good: .env file (not committed)
   SEL_API_KEY=sel_live_abcdef1234567890...
   
   # Bad: Hardcoded in source
   api_key = "sel_live_abcdef1234567890..."  # DON'T DO THIS
   ```

3. **Rotate keys periodically**
   - Request new key from admin
   - Update all services to use new key
   - Notify admin to revoke old key

4. **Use separate keys per service**
   - Scraper A: `sel_live_abc...`
   - Scraper B: `sel_live_xyz...`
   - Easier to track usage and revoke if compromised

5. **Monitor for leaked keys**
   - Use GitHub secret scanning
   - Use tools like `truffleHog` or `gitleaks`

### Key Rotation

**When to rotate:**
- Every 6-12 months (best practice)
- If key is compromised or exposed
- When employee with access leaves
- Before public repository publication

**How to rotate:**

1. Request new key from admin
2. Deploy new key to production (dual-key period)
3. Monitor for errors (old key still works)
4. After 24-48 hours, notify admin to revoke old key
5. Remove old key from all systems

### Revoking Keys

**If your key is compromised:**

1. **Immediately contact admin** to revoke key
2. Stop all services using the key
3. Request new key
4. Review logs for unauthorized usage

---

## JWT Authentication

JWT (JSON Web Tokens) are used for **interactive sessions** like admin dashboards and web applications.

### Obtaining a JWT

**Login Endpoint**: `POST /api/v1/admin/login`

**Request**:
```bash
curl -X POST https://toronto.togather.foundation/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{
       "username": "admin",
       "password": "secure_password"
     }'
```

**Response**:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2025-08-15T20:00:00Z",
  "user": {
    "id": "2e9c2fd1-6b1b-4a5e-9d54-5a9f6e6c1f1a",
    "username": "admin",
    "email": "info@togather.foundation",
    "role": "admin"
  }
}
```

### JWT Claims

The JWT contains these claims:

```json
{
  "sub": "admin",                 // Username
  "email": "info@togather.foundation",   // User email
  "role": "admin",                // Role (admin)
  "iat": 1640000000,              // Issued at (Unix timestamp)
  "exp": 1640003600,              // Expires at (Unix timestamp)
  "iss": "https://toronto.togather.foundation"  // Issuer
}
```

### Using JWT Tokens

Include JWT in the `Authorization` header:

```bash
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
     -X GET \
     https://toronto.togather.foundation/api/v1/admin/events/pending
```

### Token Expiration

**Token Lifetime**: 24 hours (configurable via `JWT_EXPIRY_HOURS`)

When a token expires, re-authenticate with `POST /api/v1/admin/login`.

---

## Rate Limiting

All API endpoints are rate-limited to prevent abuse and ensure fair usage.

### Rate Limit Tiers

| Role | Requests/Minute | Burst Allowance | Use Case |
|------|----------------|----------------|-----------|
| **Public** | 60 | 10 | Anonymous browsing |
| **Agent** | 300 | 50 | Community scrapers |
| **Admin** | Unlimited | N/A | System administration |

**Burst Allowance**: Allows short bursts above the per-minute rate (e.g., 70 requests in a single second is OK if average is <60/min).

### Rate Limit Headers

Every API response includes rate limit information:

```
X-RateLimit-Limit: 300          # Requests allowed per minute
X-RateLimit-Remaining: 247      # Requests remaining in current window
X-RateLimit-Reset: 1640003600   # Unix timestamp when limit resets
```

### Rate Limit Exceeded

**Response** (429 Too Many Requests):

```json
{
  "type": "https://sel.events/problems/rate-limit-exceeded",
  "title": "Rate limit exceeded",
  "status": 429,
  "detail": "Rate limit of 300 requests per minute exceeded. Please wait 45 seconds before retrying.",
  "instance": "/api/v1/events"
}
```

**Headers**:
```
Retry-After: 45  # Seconds until rate limit resets
```

### Handling Rate Limits

**Best Practices:**

1. **Monitor headers** and slow down before hitting limit
   ```python
   import time
   import requests
   
   def api_call(url, headers):
       response = requests.get(url, headers=headers)
       
       remaining = int(response.headers.get('X-RateLimit-Remaining', 0))
       if remaining < 10:
           # Approaching limit, wait before next call
           time.sleep(2)
       
       return response
   ```

2. **Implement exponential backoff** on 429 errors
   ```python
   import time
   
   def api_call_with_retry(url, headers, max_retries=3):
       for attempt in range(max_retries):
           response = requests.get(url, headers=headers)
           
           if response.status_code == 429:
               retry_after = int(response.headers.get('Retry-After', 60))
               wait_time = retry_after * (2 ** attempt)  # Exponential backoff
               print(f"Rate limited. Waiting {wait_time}s...")
               time.sleep(wait_time)
               continue
           
           return response
       
       raise Exception("Max retries exceeded")
   ```

3. **Batch operations** to reduce request count (not available)
   ```bash
   # Instead of 100 individual POSTs
   curl -X POST /api/v1/events -d @event1.json
   curl -X POST /api/v1/events -d @event2.json
   # ... (98 more)
   
    # Use throttling and idempotency keys for safe retries
   ```

4. **Cache responses** when appropriate
   - Cache event lists for 5-10 minutes
   - Cache event details for 1 hour
   - Use ETags for conditional requests

### Requesting Higher Limits

If your use case requires higher limits:

1. **Contact admin** with justification
2. Provide:
   - **Current usage**: Requests per day/hour
   - **Projected growth**: Expected increase
   - **Use case**: Why higher limit is needed
   - **Optimization efforts**: What you've done to minimize requests
3. Admin may upgrade your tier or provide custom limit

---

## Making Authenticated Requests

### cURL Examples

**Create Event (API Key)**:

```bash
curl -X POST https://toronto.togather.foundation/api/v1/events \
     -H "Authorization: Bearer $SEL_API_KEY" \
     -H "Content-Type: application/ld+json" \
     -d '{
       "name": "Jazz in the Park",
       "startDate": "2025-08-15T19:00:00-04:00",
       "location": {
         "name": "Centennial Park",
         "addressLocality": "Toronto",
         "addressRegion": "ON"
       }
     }'
```

**Get Pending Events (JWT)**:

```bash
curl -X GET https://toronto.togather.foundation/api/v1/admin/events/pending \
     -H "Authorization: Bearer $JWT_TOKEN"
```

### Python Examples

**Using requests library**:

```python
import requests
import os

API_KEY = os.environ['SEL_API_KEY']
BASE_URL = 'https://toronto.togather.foundation/api/v1'

headers = {
    'Authorization': f'Bearer {API_KEY}',
    'Content-Type': 'application/ld+json'
}

# Create event
event = {
    'name': 'Jazz in the Park',
    'startDate': '2025-08-15T19:00:00-04:00',
    'location': {
        'name': 'Centennial Park',
        'addressLocality': 'Toronto',
        'addressRegion': 'ON'
    }
}

response = requests.post(f'{BASE_URL}/events', json=event, headers=headers)

if response.status_code == 201:
    event_data = response.json()
    print(f"Created event: {event_data['@id']}")
else:
    print(f"Error: {response.status_code}")
    print(response.json())
```

### JavaScript Examples

**Using fetch**:

```javascript
const API_KEY = process.env.SEL_API_KEY;
const BASE_URL = 'https://toronto.togather.foundation/api/v1';

async function createEvent(event) {
  const response = await fetch(`${BASE_URL}/events`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${API_KEY}`,
      'Content-Type': 'application/ld+json'
    },
    body: JSON.stringify(event)
  });
  
  if (!response.ok) {
    const error = await response.json();
    throw new Error(`API error: ${error.detail}`);
  }
  
  return response.json();
}

// Usage
const event = {
  name: 'Jazz in the Park',
  startDate: '2025-08-15T19:00:00-04:00',
  location: {
    name: 'Centennial Park',
    addressLocality: 'Toronto',
    addressRegion: 'ON'
  }
};

createEvent(event)
  .then(data => console.log('Created:', data['@id']))
  .catch(err => console.error('Error:', err.message));
```

### Go Examples

**Using net/http**:

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
)

func main() {
    apiKey := os.Getenv("SEL_API_KEY")
    baseURL := "https://toronto.togather.foundation/api/v1"
    
    event := map[string]interface{}{
        "name":      "Jazz in the Park",
        "startDate": "2025-08-15T19:00:00-04:00",
        "location": map[string]string{
            "name":            "Centennial Park",
            "addressLocality": "Toronto",
            "addressRegion":   "ON",
        },
    }
    
    body, _ := json.Marshal(event)
    
    req, _ := http.NewRequest("POST", baseURL+"/events", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+apiKey)
    req.Header.Set("Content-Type", "application/ld+json")
    
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == 201 {
        var result map[string]interface{}
        json.NewDecoder(resp.Body).Decode(&result)
        fmt.Println("Created event:", result["@id"])
    } else {
        fmt.Println("Error:", resp.Status)
    }
}
```

---

## Error Handling

### Authentication Errors

**401 Unauthorized** - Missing or invalid credentials:

```json
{
  "type": "https://sel.events/problems/unauthorized",
  "title": "Unauthorized",
  "status": 401,
  "detail": "Missing or invalid authentication credentials",
  "instance": "/api/v1/events"
}
```

**Causes:**
- Missing `Authorization` header
- Invalid API key format
- Expired JWT token
- Revoked API key

**403 Forbidden** - Insufficient permissions:

```json
{
  "type": "https://sel.events/problems/forbidden",
  "title": "Forbidden",
  "status": 403,
  "detail": "Insufficient permissions to access this resource. Required role: admin",
  "instance": "/api/v1/admin/events/pending"
}
```

**Causes:**
- Role doesn't have permission for endpoint
- API key associated with wrong role
- Token role claim doesn't match requirement

### Handling 401 Errors

**API Key**:
```python
def api_call(url):
    response = requests.get(url, headers={'Authorization': f'Bearer {API_KEY}'})
    
    if response.status_code == 401:
        # API key invalid or revoked
        print("ERROR: API key is invalid. Please contact admin for new key.")
        sys.exit(1)
    
    return response
```

**JWT**:
```javascript
async function apiCall(url) {
  let response = await fetch(url, {
    headers: { 'Authorization': `Bearer ${accessToken}` }
  });
  
  if (response.status === 401) {
    // Try refresh
    const refreshed = await refreshToken();
    if (refreshed) {
      // Retry with new token
      response = await fetch(url, {
        headers: { 'Authorization': `Bearer ${accessToken}` }
      });
    } else {
      // Redirect to login
      window.location.href = '/login';
    }
  }
  
  return response;
}
```

---

## Security Best Practices

### API Key Security

1. **Never expose keys in client-side code**
   ```javascript
   // BAD: Key visible in browser
   const API_KEY = 'sel_live_abcdef1234...';
   fetch('/api/events', { headers: { 'Authorization': `Bearer ${API_KEY}` }});
   
   // GOOD: Proxy through your backend
   fetch('/api/my-backend/events');  // Backend adds API key
   ```

2. **Use environment variables**
   ```bash
   # .env (not committed)
   SEL_API_KEY=sel_live_abcdef1234...
   
   # Load in app
   python:
     api_key = os.environ['SEL_API_KEY']
   
   node.js:
     const apiKey = process.env.SEL_API_KEY;
   
   go:
     apiKey := os.Getenv("SEL_API_KEY")
   ```

3. **Use secret managers in production**
   - AWS Secrets Manager
   - HashiCorp Vault
   - Google Cloud Secret Manager
   - Azure Key Vault

### Network Security

1. **Always use HTTPS**
   - SEL API enforces HTTPS in production
   - Never send credentials over HTTP

2. **Validate TLS certificates**
   ```python
   # Good: Verify certificates (default)
   response = requests.get(url, verify=True)
   
   # Bad: Disable verification (NEVER do this)
   response = requests.get(url, verify=False)
   ```

3. **Use firewall rules**
   - Whitelist SEL API domains
   - Block unexpected outbound connections

### Token Storage

**Web Applications**:
- ✅ Use HttpOnly cookies for tokens (prevents XSS)
- ✅ Use secure session storage
- ❌ Don't store tokens in localStorage (XSS vulnerable)
- ❌ Don't store tokens in sessionStorage (XSS vulnerable)

**Mobile Applications**:
- ✅ Use platform keychain (iOS Keychain, Android Keystore)
- ❌ Don't store tokens in app preferences (easily accessible)

**Server Applications**:
- ✅ Use environment variables or secret managers
- ✅ Use encrypted configuration files
- ❌ Don't hardcode in source code

---

## Troubleshooting

### "Invalid API Key" Error

**Symptoms**: 401 error with "Invalid authentication credentials"

**Checks**:
1. Verify key format: `sel_live_...` (64 chars)
2. Check for extra whitespace (trim key)
3. Ensure header format: `Authorization: Bearer <key>`
4. Verify key hasn't been revoked (contact admin)

**Test**:
```bash
# Test key validity
curl -H "Authorization: Bearer $SEL_API_KEY" \
     https://toronto.togather.foundation/api/v1/events?limit=1
```

### "Rate Limit Exceeded" Error

**Symptoms**: 429 error, `Retry-After` header present

**Solutions**:
1. Implement exponential backoff
2. Reduce request frequency
3. Use batch endpoints where available
4. Request higher rate limit tier

**Example Fix**:
```python
# Before: Rapid requests
for event in events:
    create_event(event)  # Hits rate limit

# After: Batched with delay
batch_size = 50
for i in range(0, len(events), batch_size):
    batch = events[i:i+batch_size]
    create_events_batch(batch)
    time.sleep(5)  # 5-second delay between batches
```

### "Forbidden" Error on Admin Endpoint

**Symptoms**: 403 error when accessing `/api/v1/admin/*`

**Cause**: API key associated with `agent` role, but endpoint requires `admin` role

**Solution**: Contact admin to upgrade role or use separate admin account

### JWT Token Expired

**Symptoms**: 401 error with "Token expired" detail

**Solution**: Re-authenticate via `POST /api/v1/admin/login` and replace the token

---

## Quick Reference

### Authentication Header Formats

```bash
# API Key
Authorization: Bearer sel_live_abcdef1234567890...

# JWT
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### Common Response Codes

| Code | Meaning | Action |
|------|---------|--------|
| 200 | OK | Success |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Fix request payload |
| 401 | Unauthorized | Check authentication credentials |
| 403 | Forbidden | Insufficient permissions |
| 429 | Rate Limited | Wait and retry (see `Retry-After` header) |
| 500 | Server Error | Retry with exponential backoff |

### Rate Limit Tiers

| Role | Limit | Use Case |
|------|-------|----------|
| Public | 60/min | Anonymous |
| Agent | 300/min | Integrations |
| Admin | Unlimited | Administration |

---

**Next Steps:**
- [API_GUIDE.md](API_GUIDE.md) - Complete API endpoint reference
- [SCRAPERS.md](SCRAPERS.md) - Event scraper best practices
- [../contributors/SECURITY.md](../contributors/SECURITY.md) - Security architecture

**Document Version**: 0.1.0  
**Last Updated**: 2026-01-27  
**Maintenance**: Update when authentication mechanisms change or new roles are added
