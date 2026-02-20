# Developer Quick Start Guide

**Version:** 0.1.0  
**Last Updated:** 2026-02-12  
**For:** Application developers integrating with the SEL API

---

## Getting Started in 4 Steps

### 1. Get an Invitation

Contact an admin to request developer access:
- Email: [info@togather.foundation](mailto:info@togather.foundation)
- Provide: Your name, email, and intended use case

The admin will send you an invitation via the CLI:
```bash
server developer invite your-email@example.com --name "Your Name"
```

### 2. Accept Your Invitation

1. Check your email for the invitation (valid for 7 days)
2. Click the invitation link
3. Set your password and display name
4. You'll be automatically logged in to the developer dashboard

### 3. Create Your First API Key

**Via Web Interface:**
1. Navigate to `/dev/api-keys`
2. Click "Create New Key"
3. Enter a descriptive name (e.g., "My Event Scraper")
4. Copy the key immediately (it's shown only once)
5. Store it securely (environment variable or secret manager)

**Via API:**
```bash
# First, get your JWT token
curl -X POST https://toronto.togather.foundation/api/v1/dev/login \
     -H "Content-Type: application/json" \
     -d '{"email": "your-email@example.com", "password": "your-password"}'

# Then create a key
curl -X POST https://toronto.togather.foundation/api/v1/dev/api-keys \
     -H "Authorization: Bearer $YOUR_JWT_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"name": "My Event Scraper"}'
```

### 4. Start Using the API

Use your API key to submit events:

```bash
export SEL_API_KEY="your-api-key-here"

curl -X POST https://toronto.togather.foundation/api/v1/events \
     -H "Authorization: Bearer $SEL_API_KEY" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "My First Event",
       "startDate": "2026-03-15T19:00:00-04:00",
       "location": {
         "name": "Test Venue",
         "addressLocality": "Toronto",
         "addressRegion": "ON"
       },
       "source": {
         "url": "https://example.com/events/my-first-event"
       }
     }'
```

---

## Key Management Best Practices

### Storing Your API Key

✅ **Good Practices:**
```bash
# Environment variable
export SEL_API_KEY="your-key-here"

# .env file (add to .gitignore)
SEL_API_KEY=your-key-here

# Secret manager (production)
aws secretsmanager get-secret-value --secret-id sel-api-key
```

❌ **Bad Practices:**
```javascript
// DON'T hardcode in source
const API_KEY = "sel_live_abc123...";  // NEVER DO THIS

// DON'T commit to version control
git add .env  // NEVER DO THIS
```

### Managing Multiple Keys

You can create up to 5 API keys (default limit). Use separate keys for:
- Different environments (development, staging, production)
- Different applications or scrapers
- Different team members or services

This makes it easier to:
- Track usage per application
- Revoke specific keys if compromised
- Identify which service is causing issues

### Revoking Keys

Revoke keys immediately if:
- They're compromised or exposed
- You no longer need them
- You're rotating keys for security

**Via Web Interface:**
1. Go to `/dev/api-keys`
2. Click "Revoke" next to the key
3. Confirm the action

**Via API:**
```bash
curl -X DELETE https://toronto.togather.foundation/api/v1/dev/api-keys/$KEY_ID \
     -H "Authorization: Bearer $YOUR_JWT_TOKEN"
```

---

## Common Operations

### List Your API Keys

```bash
curl https://toronto.togather.foundation/api/v1/dev/api-keys \
     -H "Authorization: Bearer $YOUR_JWT_TOKEN"
```

Response shows key prefixes (not full keys), usage stats, and last used time.

### Submit an Event

See [api-guide.md](api-guide.md#event-submission) for complete event submission documentation.

**Minimal event:**
```json
{
  "name": "Event Name",
  "startDate": "2026-03-15T19:00:00-04:00",
  "location": {"name": "Venue Name"},
  "source": {"url": "https://source.com/events/123"}
}
```

**Recommended event (better data quality):**
```json
{
  "name": "Event Name",
  "description": "Full event description",
  "startDate": "2026-03-15T19:00:00-04:00",
  "endDate": "2026-03-15T22:00:00-04:00",
  "location": {
    "name": "Venue Name",
    "streetAddress": "123 Main St",
    "addressLocality": "Toronto",
    "addressRegion": "ON",
    "postalCode": "M5A 1A1"
  },
  "organizer": {"name": "Organizer Name"},
  "image": "https://example.com/poster.jpg",
  "url": "https://example.com/events/123",
  "source": {"url": "https://source.com/events/123"}
}
```

### Handle Duplicates

The SEL automatically detects duplicate events. If you submit the same event twice (same `source.url`), you'll get a `409 Conflict` response with the existing event:

```bash
HTTP/1.1 409 Conflict
{
  "@id": "https://toronto.togather.foundation/events/01KG...",
  "name": "Event Name"
}
```

This is **not an error** — it means the event already exists. Extract the `@id` to track which events you've submitted.

---

## Rate Limits

Your API keys have the following limits:

| Tier | Limit | Use Case |
|------|-------|----------|
| **Agent** (your keys) | 300 requests/minute | Event submission, scraping |
| **Public** (no key) | 60 requests/minute | Read-only access |

**Rate limit headers** are included in every response:
```
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 247
X-RateLimit-Reset: 1643673600
```

**If you exceed the limit**, you'll get a `429 Too Many Requests` response. Implement exponential backoff:

```python
import time
import requests

def api_call_with_retry(url, headers):
    for attempt in range(3):
        response = requests.get(url, headers=headers)
        
        if response.status_code == 429:
            retry_after = int(response.headers.get('Retry-After', 60))
            wait_time = retry_after * (2 ** attempt)
            print(f"Rate limited. Waiting {wait_time}s...")
            time.sleep(wait_time)
            continue
        
        return response
    
    raise Exception("Max retries exceeded")
```

---

## Troubleshooting

### "Invalid API Key" Error

**Symptoms:** 401 Unauthorized

**Solutions:**
1. Check header format: `Authorization: Bearer <key>`
2. Verify key hasn't been revoked (check `/dev/api-keys`)
3. Check for whitespace or typos in the key
4. Ensure key is from the correct environment (staging vs production)

### "Maximum API Keys Exceeded"

**Symptoms:** 409 Conflict when creating a key

**Solutions:**
1. Revoke unused keys in `/dev/api-keys`
2. Contact an admin to request a higher limit
3. Use the same key for multiple environments (if appropriate)

### "Forbidden" Error

**Symptoms:** 403 Forbidden

**Cause:** Developer keys have `agent` role and cannot access admin endpoints (`/api/v1/admin/*`)

**Solution:** Use the developer portal (`/dev/*`) for key management, not admin endpoints.

### Password Reset

Currently, password reset is handled by admins. Contact [info@togather.foundation](mailto:info@togather.foundation) if you've forgotten your password.

---

## Next Steps

- **Complete API Reference:** [api-guide.md](api-guide.md)
- **Authentication Details:** [authentication.md](authentication.md#developer-self-service)
- **Event Scraper Best Practices:** [scrapers.md](scrapers.md)
- **Security Guidelines:** [../contributors/security.md](../contributors/security.md)

---

**Questions or Issues?**

- GitHub Issues: https://github.com/Togather-Foundation/server/issues
- Email: [info@togather.foundation](mailto:info@togather.foundation)

---

**Developer Quick Start Guide** — Part of the [Togather Foundation](https://togather.foundation)  
*Building shared infrastructure for event discovery as a public good.*
