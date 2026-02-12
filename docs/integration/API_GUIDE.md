# SEL API Documentation

**Version:** 0.1.0  
**Last Updated:** 2026-01-27  
**Base URL:** `https://sel.togather.events` (production) or `http://localhost:8080` (development)

---

## Table of Contents

1. [Overview](#overview)
2. [OpenAPI Specification](#openapi-specification)
3. [Authentication](#authentication)
4. [Rate Limiting](#rate-limiting)
5. [Event Submission (POST /api/v1/events)](#event-submission)
6. [Event Retrieval (GET /api/v1/events)](#event-retrieval)
7. [Idempotency](#idempotency)
8. [Duplicate Detection](#duplicate-detection)
9. [Error Handling](#error-handling)
10. [Examples](#examples)

---

## Overview

The SEL API provides endpoints for submitting, retrieving, and managing cultural event data. The API follows RESTful principles and uses JSON-LD for all payloads, based on Schema.org vocabulary.

### Key Features

- **JSON-LD Native**: All data uses Schema.org vocabulary with linked data semantics
- **Duplicate Detection**: Automatic detection via source tracking and content hashing
- **Idempotency**: Safe retries using `Idempotency-Key` header
- **Provenance Tracking**: Full source attribution for all submitted data
- **Auto-Review**: Intelligent triage based on data completeness
- **Rate Limited**: Role-based limits protect service availability

### Content Types

The API returns JSON or JSON-LD via the `Accept` header. Dereferenceable entity pages (under `/events/{id}`, `/places/{id}`, `/organizations/{id}`) support HTML (and may support Turtle) but those are **public pages**, not `/api/v1/*` endpoints.

---

## OpenAPI Specification

The complete OpenAPI 3.1.0 specification is available at:

**Endpoint:** `GET /api/v1/openapi.json`

**Example:**
```bash
curl https://sel.togather.events/api/v1/openapi.json
```

**Response:** OpenAPI 3.1.0 specification in JSON format

The OpenAPI spec provides:
- Complete endpoint documentation with request/response schemas
- Authentication requirements for each endpoint
- Rate limiting information
- Pagination patterns
- Error response formats
- Example requests and responses

**Using the Specification:**
- Import into **Postman** or **Insomnia** for API testing
- Generate client libraries with **OpenAPI Generator**
- Validate API contracts in CI/CD pipelines
- Auto-generate documentation with **ReDoc** or **Swagger UI**

**Source:** The canonical YAML source is maintained at `docs/api/openapi.yaml` in the repository.

---

## Authentication

### API Keys

Most write operations require authentication via API key.

**Header:**
```http
Authorization: Bearer your-api-key-here
```

**Obtaining an API Key:**

API keys are created by administrators via the admin interface:

1. Login to admin dashboard: `POST /api/v1/admin/login`
2. Create API key: `POST /api/v1/admin/api-keys`
3. Keys are returned once at creation - store securely

**Security:**
- API keys are hashed with bcrypt before storage
- Keys should be treated as secrets (like passwords)
- Use environment variables, never commit to version control
- Rotate keys periodically

---

## Rate Limiting

Rate limits are applied per IP address and vary by endpoint tier:

| Tier | Endpoints | Limit | Description |
|------|-----------|-------|-------------|
| **Public** | `GET /api/v1/events`, `GET /api/v1/places`, etc. | 60 req/min | Read-only public access |
| **Agent** | `POST /api/v1/events` (with API key) | 300 req/min | Authenticated scrapers/agents |
| **Federation** | `POST /api/v1/federation/sync` | 500 req/min | Federated node sync |
| **Admin** | `/api/v1/admin/*` | Unlimited (0) | Administrative operations |
| **Login** | `POST /api/v1/admin/login` | 5 per 15 min | Login attempts (strict) |

**Rate Limit Headers:**

```http
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 299
X-RateLimit-Reset: 1643673600
```

**Rate Limit Exceeded:**

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/problem+json

{
  "type": "https://sel.events/problems/rate-limit-exceeded",
  "title": "Rate limit exceeded",
  "status": 429,
  "detail": "Rate limit exceeded. Try again in 60 seconds."
}
```

---

## Event Submission

Submit events to the SEL via `POST /api/v1/events`.

### Endpoint

```http
POST /api/v1/events
Content-Type: application/json
Authorization: Bearer your-api-key-here
```

### Request Body

The request body should be a JSON-LD object representing a Schema.org Event.

#### Minimal Event (Simplest Scraper Submission)

The absolute minimum required fields for a scraper:

```json
{
  "name": "Comedy Night at The Tranzac",
  "startDate": "2026-02-15T20:00:00-05:00",
  "location": {
    "name": "The Tranzac"
  },
  "source": {
    "url": "https://thetranzac.com/events/comedy-night"
  }
}
```

**Note:** The `source.url` is the URL you scraped from. This enables duplicate detection - if you submit the same URL twice, the system will recognize it as a duplicate (returning `409 Conflict` with the existing event).

#### Recommended Event

For better data quality, include these additional fields:

```json
{
  "name": "Comedy Night at The Tranzac",
  "description": "Join us for an evening of stand-up comedy featuring local Toronto comedians.",
  "startDate": "2026-02-15T20:00:00-05:00",
  "endDate": "2026-02-15T23:00:00-05:00",
  "doorTime": "2026-02-15T19:30:00-05:00",
  
  "location": {
    "name": "The Tranzac",
    "streetAddress": "292 Brunswick Ave",
    "addressLocality": "Toronto",
    "addressRegion": "ON",
    "postalCode": "M5S 2M7",
    "addressCountry": "CA"
  },
  
  "organizer": {
    "name": "Toronto Comedy Collective"
  },
  
  "image": "https://example.com/comedy-night-poster.jpg",
  "url": "https://thetranzac.com/events/comedy-night",
  "keywords": ["comedy", "standup", "entertainment"],
  
  "isAccessibleForFree": false,
  "offers": {
    "price": "15.00",
    "priceCurrency": "CAD",
    "url": "https://thetranzac.com/tickets/comedy-night"
  },
  
  "license": "https://creativecommons.org/publicdomain/zero/1.0/",
  
  "source": {
    "url": "https://thetranzac.com/events/comedy-night",
    "name": "Tranzac Events Scraper"
  }
}
```

#### Full Event Payload

All supported fields:

```json
{
  "name": "Event Name",
  "description": "Full event description with HTML is preserved",
  "startDate": "2026-02-15T20:00:00-05:00",
  "endDate": "2026-02-15T23:00:00-05:00",
  "doorTime": "2026-02-15T19:30:00-05:00",
  
  "location": {
    "@id": "https://toronto.togather.foundation/places/01ABC...",
    "name": "Venue Name",
    "streetAddress": "123 Main St",
    "addressLocality": "Toronto",
    "addressRegion": "ON",
    "postalCode": "M5A 1A1",
    "addressCountry": "CA",
    "latitude": 43.6532,
    "longitude": -79.3832
  },
  
  "virtualLocation": {
    "@type": "VirtualLocation",
    "url": "https://zoom.us/j/123456789",
    "name": "Zoom Meeting Room"
  },
  
  "organizer": {
    "@id": "https://toronto.togather.foundation/organizations/01XYZ...",
    "name": "Organizer Name",
    "url": "https://organizer.com"
  },
  
  "image": "https://example.com/event-poster.jpg",
  "url": "https://example.com/events/123",
  "keywords": ["arts", "music", "jazz"],
  "inLanguage": ["en", "fr"],
  
  "isAccessibleForFree": false,
  "offers": {
    "url": "https://example.com/tickets",
    "price": "25.00",
    "priceCurrency": "CAD"
  },
  
  "sameAs": [
    "http://kg.artsdata.ca/resource/K12-345",
    "https://www.wikidata.org/wiki/Q123456"
  ],
  
  "license": "https://creativecommons.org/publicdomain/zero/1.0/",
  
  "source": {
    "url": "https://source-site.com/events/123",
    "eventId": "evt-123",
    "name": "Source Site Events Scraper",
    "license": "CC0"
  },
  
  "occurrences": [
    {
      "startDate": "2026-02-15T20:00:00-05:00",
      "endDate": "2026-02-15T23:00:00-05:00",
      "timezone": "America/Toronto",
      "doorTime": "2026-02-15T19:30:00-05:00"
    },
    {
      "startDate": "2026-02-22T20:00:00-05:00",
      "endDate": "2026-02-22T23:00:00-05:00",
      "timezone": "America/Toronto"
    }
  ]
}
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Event name (max 500 chars) |
| `startDate` | ISO 8601 | **Yes** | Event start date/time with timezone |
| `location` or `virtualLocation` | object | **Yes** | Physical or virtual venue (at least one required) |
| `description` | string | No | Event description (max 10,000 chars, HTML preserved) |
| `endDate` | ISO 8601 | No | Event end date/time (must be after `startDate`) |
| `doorTime` | ISO 8601 | No | Door opening time |
| `organizer` | object | No | Organizing entity |
| `image` | URL | No | Event poster/image (HTTPS recommended) |
| `url` | URL | No | Public event webpage |
| `keywords` | array | No | Descriptive keywords for search/discovery |
| `inLanguage` | array | No | Language codes (e.g., `["en", "fr"]`) |
| `isAccessibleForFree` | boolean | No | Whether event is free |
| `offers` | object | No | Ticket pricing information |
| `sameAs` | array | No | URIs to equivalent entities (Artsdata, Wikidata, etc.) |
| `license` | string | No | License URL (defaults to CC0) |
| `source` | object | No | Provenance information (**recommended for scrapers**) |
| `source.url` | string | No | URL where event was found (used for duplicate detection) |
| `source.eventId` | string | No | External event ID from source system (optional, for advanced duplicate tracking) |
| `source.name` | string | No | Human-readable scraper/source name |
| `source.license` | string | No | License for scraped content (if different from event license) |
| `occurrences` | array | No | Multiple date/time instances for recurring events |

#### Understanding the `source` Field

The `source` object provides **provenance tracking** - it records where the event data came from:

- **`source.url`** (recommended): The URL you scraped. This is the most important field for scrapers:
  - Used for duplicate detection: submitting the same URL twice returns `409 Conflict`
  - Stored for provenance: users can see where data originated
  - Example: `"https://thetranzac.com/events/comedy-night"`

- **`source.eventId`** (optional): Use this if the source system has its own event ID:
  - Enables duplicate detection even if the URL changes
  - Example: If EventBrite's event ID is `evt-123`, you can submit it
  - Most scrapers don't need this - the URL is sufficient

- **`source.name`** (optional): Human-readable identifier for your scraper
  - Example: `"Tranzac Events Scraper"` or `"Toronto Arts Crawler"`

- **`source.license`** (optional): License of the scraped content if different from the event license

**For most scrapers:** Just provide `source.url` with the URL you scraped from. This is enough for duplicate detection and provenance tracking.

### Response

**Success (201 Created):**

```http
HTTP/1.1 201 Created
Content-Type: application/ld+json

{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://toronto.togather.foundation/events/01KG123ABC...",
  "name": "Comedy Night at The Tranzac",
  "location": {
    "@type": "Place",
    "name": "The Tranzac"
  }
}
```

**Duplicate Detected (409 Conflict):**

```http
HTTP/1.1 409 Conflict
Content-Type: application/ld+json

{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://toronto.togather.foundation/events/01KG123ABC...",
  "name": "Comedy Night at The Tranzac"
}
```

Note: A 409 response includes the existing event's details. This is NOT an error - it means the event was already in the system.

---

## Event Retrieval

### List Events

Get a paginated list of events.

```http
GET /api/v1/events
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `limit` | integer | Number of results (default: 100, max: 1000) |
| `after` | string | Cursor for pagination |
| `startDate` | ISO 8601 | Filter events starting after this date |
| `endDate` | ISO 8601 | Filter events ending before this date |
| `city` | string | Filter by city (addressLocality) |
| `region` | string | Filter by region (addressRegion) |
| `venueULID` | ULID | Filter by specific venue |
| `organizerULID` | ULID | Filter by specific organizer |
| `lifecycleState` | string | Filter by state: `draft`, `published`, `deleted` |
| `query` | string | Full-text search query |
| `keywords` | string | Comma-separated keywords |
| `domain` | string | Event domain: `arts`, `sports`, `community`, etc. |

#### Response

```json
{
  "items": [
    {
      "@context": "https://schema.org",
      "@type": "Event",
      "@id": "https://toronto.togather.foundation/events/01KG...",
      "name": "Comedy Night at The Tranzac",
      "startDate": "2026-02-15T20:00:00-05:00",
      "location": "https://toronto.togather.foundation/places/01KH..."
    }
  ],
  "next_cursor": "MTczODg5MTIwMDAwMDowMUtHMTIzQUJD"
}
```

### Get Single Event

Retrieve a specific event by ULID.

```http
GET /api/v1/events/{ulid}
Accept: application/ld+json
```

#### Response

```json
{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://toronto.togather.foundation/events/01KG...",
  "name": "Comedy Night at The Tranzac",
  "description": "Join us for an evening of stand-up comedy...",
  "startDate": "2026-02-15T20:00:00-05:00",
  "endDate": "2026-02-15T23:00:00-05:00",
  "location": {
    "@type": "Place",
    "@id": "https://toronto.togather.foundation/places/01KH...",
    "name": "The Tranzac",
    "address": {
      "@type": "PostalAddress",
      "streetAddress": "292 Brunswick Ave",
      "addressLocality": "Toronto",
      "addressRegion": "ON",
      "postalCode": "M5S 2M7",
      "addressCountry": "CA"
    }
  }
}
```

#### Deleted Events

If an event has been deleted, the endpoint returns a tombstone:

```http
HTTP/1.1 410 Gone
Content-Type: application/ld+json

{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://toronto.togather.foundation/events/01KG...",
  "sel:tombstone": true,
  "sel:deletedAt": "2026-01-15T10:30:00Z"
}
```

---

## Idempotency

To safely retry requests without creating duplicates, use the optional `Idempotency-Key` header.

---

## Duplicate Detection

SEL automatically detects duplicate events using multiple strategies:

### 1. Source URL Detection (Simplest for Scrapers)

If you provide `source.url`, duplicate submissions of the same URL are detected automatically:

```json
{
  "name": "Comedy Night",
  "startDate": "2026-02-15T20:00:00-05:00",
  "location": { "name": "The Tranzac" },
  "source": {
    "url": "https://thetranzac.com/events/comedy-night"
  }
}
```

**Behavior:**
- First submission: Creates event, returns `201 Created`
- Second submission of same URL: Returns existing event with `409 Conflict`
- **Important:** `409` is not an error - it means the event already exists

**Why this matters for scrapers:**
- Run your scraper daily without worrying about duplicates
- The system tracks which events came from which URLs
- You can safely re-submit the same URL on subsequent scrapes

### 2. Source-Based Detection with External IDs (Advanced)

If the source system has its own event IDs, you can use both `source.url` and `source.eventId`:

```json
{
  "source": {
    "url": "https://eventbrite.com/event/123",
    "eventId": "evt-123",
    "name": "EventBrite Scraper"
  }
}
```

This enables duplicate detection even if the URL structure changes (e.g., EventBrite adds tracking parameters).

### 3. Content Hash Detection (Cross-Source)

Even if sources differ, events with the same:
- Event name (normalized, case-insensitive)
- Venue name  
- Start date/time

...are detected as duplicates.

**Example:**

```json
// Scraper A submits:
{
  "name": "Jazz Night",
  "startDate": "2026-02-15T20:00:00-05:00",
  "location": { "name": "Massey Hall" },
  "source": { "url": "https://source-a.com/events/123" }
}

// Scraper B submits (different source):
{
  "name": "Jazz Night",  // Same name
  "startDate": "2026-02-15T20:00:00-05:00",  // Same time
  "location": { "name": "Massey Hall" },  // Same venue
  "source": { "url": "https://source-b.com/event/jazz-night" }  // Different URL!
}
```

**Result:** Second submission returns `409 Conflict` with existing event. Both sources are recorded as contributing to the same event.

### 4. Handling 409 Responses

When you get a `409 Conflict`, the response includes the existing event:

```http
HTTP/1.1 409 Conflict
Content-Type: application/ld+json

{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://toronto.togather.foundation/events/01KG123ABC...",
  "name": "Jazz Night",
  "startDate": "2026-02-15T20:00:00-05:00"
}
```

**For scrapers:**
- Extract the `@id` field - this is the canonical event ID
- Store this mapping: `source_url -> sel_event_id`
- Use this for tracking which events you've already submitted

### 5. Disambiguation (When You Need Separate Events)

To force separate events (e.g., two "Open Mic" shows on the same day at the same venue), make the names distinct:

```json
// First show
{ "name": "Open Mic Night - Early Show (6pm)" }

// Second show  
{ "name": "Open Mic Night - Late Show (10pm)" }
```

---

## Error Handling

All errors follow RFC 7807 (Problem Details for HTTP APIs).

### Error Response Format

```json
{
  "type": "https://sel.events/problems/validation-error",
  "title": "Validation error",
  "status": 400,
  "detail": "invalid startDate: must be valid RFC3339",
  "instance": "/api/v1/events"
}
```

### Common Errors

#### 400 Bad Request

```json
{
  "type": "https://sel.events/problems/validation-error",
  "title": "Validation error",
  "status": 400,
  "detail": "missing required field: name"
}
```

**Common causes:**
- Missing required fields (`name`, `startDate`, `location`)
- Invalid date format (must be RFC3339 with timezone)
- `endDate` before `startDate`
- Invalid URLs
- Field length exceeded (name > 500 chars, description > 10,000 chars)

#### 401 Unauthorized

```json
{
  "type": "https://sel.events/problems/unauthorized",
  "title": "Unauthorized",
  "status": 401,
  "detail": "Missing or invalid API key"
}
```

**Cause:** Missing or invalid `Authorization: Bearer <api_key>` header

#### 409 Conflict

```json
{
  "type": "https://sel.events/problems/conflict",
  "title": "Conflict",
  "status": 409,
  "detail": "Event already exists"
}
```

**Causes:**
- Duplicate event detected (by source or content hash)
- Idempotency key conflict

**Note:** For duplicate events, the response body contains the existing event details. This is often NOT an error condition for scrapers.

#### 429 Too Many Requests

```json
{
  "type": "https://sel.events/problems/rate-limit-exceeded",
  "title": "Rate limit exceeded",
  "status": 429,
  "detail": "Rate limit exceeded. Try again in 60 seconds."
}
```

**Solution:** Respect rate limits and implement exponential backoff.

---

## Examples

### Example 1: Minimal Scraper (Node.js)

```javascript
const fetch = require('node-fetch');

const API_KEY = process.env.SEL_API_KEY;
const BASE_URL = 'https://sel.togather.events';

async function submitEvent(event) {
  const response = await fetch(`${BASE_URL}/api/v1/events`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`
    },
    body: JSON.stringify(event)
  });
  
  if (response.status === 201) {
    console.log('✓ Event created');
    return await response.json();
  } else if (response.status === 409) {
    console.log('ℹ Event already exists');
    return await response.json();
  } else {
    console.error('✗ Error:', response.status);
    console.error(await response.json());
    throw new Error('Failed to submit event');
  }
}

// Example usage
submitEvent({
  name: 'Comedy Night at The Tranzac',
  startDate: '2026-02-15T20:00:00-05:00',
  location: {
    name: 'The Tranzac',
    addressLocality: 'Toronto',
    addressRegion: 'ON'
  },
  source: {
    url: 'https://thetranzac.com/events/comedy-night',
    eventId: 'evt-123',
    name: 'Tranzac Scraper'
  }
});
```

### Example 2: Scraper with Idempotency (Python)

```python
import requests
import uuid
from datetime import datetime

API_KEY = os.environ['SEL_API_KEY']
BASE_URL = 'https://sel.togather.events'

def submit_event(event_data, source_url, event_id):
    # Generate idempotency key based on source + event ID
    idempotency_key = f"scraper-{datetime.now().strftime('%Y%m%d')}-{event_id}"
    
    response = requests.post(
        f'{BASE_URL}/api/v1/events',
        headers={
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${API_KEY}`,
            'Idempotency-Key': idempotency_key
        },
        json={
            'name': event_data['title'],
            'description': event_data['description'],
            'startDate': event_data['start_date'],
            'endDate': event_data['end_date'],
            'location': {
                'name': event_data['venue_name'],
                'addressLocality': event_data['city']
            },
            'url': event_data['url'],
            'image': event_data['image_url'],
            'source': {
                'url': source_url,
                'eventId': event_id,
                'name': 'My Event Scraper'
            }
        }
    )
    
    if response.status_code in [201, 409]:
        print(f"✓ Event processed: {event_data['title']}")
        return response.json()
    else:
        print(f"✗ Error {response.status_code}: {response.json()}")
        raise Exception('Failed to submit event')

# Example usage
submit_event(
    {
        'title': 'Jazz Night',
        'description': 'Live jazz performance',
        'start_date': '2026-02-15T20:00:00-05:00',
        'end_date': '2026-02-15T23:00:00-05:00',
        'venue_name': 'The Rex Hotel',
        'city': 'Toronto',
        'url': 'https://therex.ca/events/jazz-night',
        'image_url': 'https://therex.ca/images/jazz-night.jpg'
    },
    source_url='https://therex.ca/events',
    event_id='evt-456'
)
```

### Example 3: Batch Submission with Rate Limiting

```python
import time
from ratelimit import limits, sleep_and_retry

# Agent tier: 300 requests per minute
CALLS_PER_MINUTE = 300
ONE_MINUTE = 60

@sleep_and_retry
@limits(calls=CALLS_PER_MINUTE, period=ONE_MINUTE)
def submit_event_with_rate_limit(event):
    return submit_event(event)

def scrape_and_submit(events):
    results = {
        'created': 0,
        'duplicates': 0,
        'errors': 0
    }
    
    for event in events:
        try:
            response = submit_event_with_rate_limit(event)
            if response.status_code == 201:
                results['created'] += 1
            elif response.status_code == 409:
                results['duplicates'] += 1
        except Exception as e:
            results['errors'] += 1
            print(f"Error: {e}")
    
    print(f"\nResults:")
    print(f"  Created: {results['created']}")
    print(f"  Duplicates: {results['duplicates']}")
    print(f"  Errors: {results['errors']}")
    
    return results
```

### Example 4: Recurring Event with Multiple Occurrences

```javascript
{
  "name": "Weekly Open Mic Night",
  "description": "Join us every Friday for open mic performances",
  "location": {
    "name": "The Cameron House",
    "addressLocality": "Toronto"
  },
  "startDate": "2026-02-07T20:00:00-05:00",
  "occurrences": [
    {
      "startDate": "2026-02-07T20:00:00-05:00",
      "endDate": "2026-02-07T23:00:00-05:00",
      "timezone": "America/Toronto"
    },
    {
      "startDate": "2026-02-14T20:00:00-05:00",
      "endDate": "2026-02-14T23:00:00-05:00",
      "timezone": "America/Toronto"
    },
    {
      "startDate": "2026-02-21T20:00:00-05:00",
      "endDate": "2026-02-21T23:00:00-05:00",
      "timezone": "America/Toronto"
    },
    {
      "startDate": "2026-02-28T20:00:00-05:00",
      "endDate": "2026-02-28T23:00:00-05:00",
      "timezone": "America/Toronto"
    }
  ],
  "source": {
    "url": "https://thecameron.com/events",
    "eventId": "open-mic-feb-2026"
  }
}
```

---

## See Also

- [SEL Core Profile](../interop/CORE_PROFILE_v0.1.md) - Detailed JSON-LD specifications
- [Security Model](../contributors/SECURITY.md) - Authentication and security practices
- [Development Guide](../contributors/DEVELOPMENT.md) - For contributors
- [Schema Design](../contributors/DATABASE.md) - Database schema and provenance model

---

**Questions or Issues?**

- GitHub Issues: https://github.com/Togather-Foundation/server/issues
- Email: [info@togather.foundation](mailto:info@togather.foundation)

---

**SEL API Documentation** — Part of the [Togather Foundation](https://togather.foundation)  
*Building shared infrastructure for event discovery as a public good.*
