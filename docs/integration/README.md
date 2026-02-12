# Integration Guide

Welcome to the SEL integration documentation!

## Who This Is For

You're building systems that submit events to or consume events from SEL:
- **Event scrapers** collecting data from websites
- **Data submitters** pushing events from ticketing systems
- **API consumers** retrieving event data

## Quick Start

### For Developers (API Key Self-Service)

1. **[Developer Quick Start](DEVELOPER_QUICKSTART.md)** - Get started in 4 steps
2. **[Authentication](AUTHENTICATION.md#developer-self-service)** - Invitation, login, key management
3. **[API Guide](API_GUIDE.md)** - Practical guide with code examples

### For Event Scrapers

1. **[API Guide](API_GUIDE.md)** - Practical guide with code examples
2. **[Authentication](AUTHENTICATION.md)** - Get API keys, understand rate limits
3. **[Scraper Best Practices](SCRAPERS.md)** - Idempotency, deduplication, batch patterns

### For API Consumers

1. **[API Guide](API_GUIDE.md)** - Event retrieval, filtering, pagination
2. **[Authentication](AUTHENTICATION.md)** - Rate limits for public endpoints

## Minimal Scraper Example

```javascript
const response = await fetch('https://sel.togather.events/api/v1/events', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${process.env.SEL_API_KEY}`
  },
  body: JSON.stringify({
    name: 'Event Name',
    startDate: '2026-02-15T20:00:00-05:00',
    location: { name: 'Venue Name' },
    source: { url: 'https://source-site.com/event-123' }
  })
});
```

See [examples/](examples/) for complete working code.

## Key Features

### Duplicate Detection
Submit the same event twice using `source.url` - SEL automatically detects duplicates and returns the existing event.

### Idempotency
Use `Idempotency-Key` header for safe retries:
```javascript
headers: {
  'Idempotency-Key': 'scraper-20260127-evt123'
}
```

### Rate Limiting
- **Public endpoints**: 60 requests/minute
- **Agent tier** (with API key): 300 requests/minute

## API Reference

### Submit Event
```http
POST /api/v1/events
Authorization: Bearer your-api-key
Content-Type: application/json
```

### Retrieve Events
```http
GET /api/v1/events?limit=100&city=Toronto
```

### Get Single Event
```http
GET /api/v1/events/{ulid}
Accept: application/ld+json
```

## Code Examples

Browse working examples in [examples/](examples/):
- `minimal_scraper.js` - Simplest possible event submission
- `batch_scraper.py` - Batch submission with rate limiting
- `idempotency_example.py` - Safe retry patterns

## Required Fields

Every event needs:
- `name` - Event name (max 500 chars)
- `startDate` - ISO 8601 with timezone
- `location` - Physical or virtual venue

Recommended:
- `description` - Event details
- `source.url` - Where you scraped from (enables duplicate detection)
- `organizer` - Who's running the event

## Error Handling

All errors follow RFC 7807 format:

```json
{
  "type": "https://sel.events/problems/validation-error",
  "title": "Validation error",
  "status": 400,
  "detail": "Missing required field: startDate"
}
```

Common status codes:
- `201` - Event created successfully
- `409` - Duplicate detected (returns existing event)
- `400` - Validation error
- `401` - Missing/invalid API key
- `429` - Rate limit exceeded

## Getting Help

- **API Guide**: [API_GUIDE.md](API_GUIDE.md) - comprehensive reference
- **GitHub Issues**: [togather/server/issues](https://github.com/Togather-Foundation/server/issues)
- **Email**: [info@togather.foundation](mailto:info@togather.foundation)

## Related Documentation

Need to build a SEL-compatible node? See [interop/README.md](../interop/README.md)

---

**Back to:** [Documentation Index](../README.md)
