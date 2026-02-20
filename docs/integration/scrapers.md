# Event Scraper Best Practices

**Version:** 0.1.0  
**Date:** 2026-01-27  
**Status:** Living Document

This document provides best practices for building event scrapers that integrate with the Togather SEL API. It covers idempotency patterns, duplicate handling, error handling, and operational considerations.

For authentication details, see [authentication.md](authentication.md). For API reference, see [api-guide.md](api-guide.md).

---

## Table of Contents

1. [Scraper Architecture](#scraper-architecture)
2. [Idempotency and Deduplication](#idempotency-and-deduplication)
3. [Batch Submission](#batch-submission)
4. [Error Handling and Retries](#error-handling-and-retries)
5. [Rate Limiting Strategies](#rate-limiting-strategies)
6. [Data Quality](#data-quality)
7. [Monitoring and Logging](#monitoring-and-logging)
8. [Example Implementation](#example-implementation)

---

## Scraper Architecture

### Design Principles

A well-designed event scraper should be:

1. **Idempotent**: Running multiple times produces the same result (no duplicates)
2. **Resilient**: Handles failures gracefully with retries and backoff
3. **Efficient**: Minimizes API calls via caching and smart retry logic
4. **Observable**: Logs progress, errors, and metrics for monitoring
5. **Respectful**: Respects rate limits and source website robots.txt

### Recommended Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Scraper Pipeline                      │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
         ┌────────────────────────────────┐
         │  1. Fetch Source Data          │
         │  (HTML, JSON, RSS, iCal)       │
         └────────────┬───────────────────┘
                      │
                      ▼
         ┌────────────────────────────────┐
         │  2. Parse and Normalize        │
         │  (Extract fields, dates, etc.) │
         └────────────┬───────────────────┘
                      │
                      ▼
         ┌────────────────────────────────┐
         │  3. Enrich and Validate        │
         │  (Geocode, validate dates)     │
         └────────────┬───────────────────┘
                      │
                      ▼
         ┌────────────────────────────────┐
         │  4. Check for Duplicates       │
         │  (Local cache + source_id)     │
         └────────────┬───────────────────┘
                      │
                      ▼
         ┌────────────────────────────────┐
         │  5. Submit to SEL API          │
         │  (With retries)                │
         └────────────┬───────────────────┘
                      │
                      ▼
         ┌────────────────────────────────┐
         │  6. Log Results                │
         │  (Success, skipped, errors)    │
         └────────────────────────────────┘
```

### Scraper Types

**Pull-Based Scrapers**:
- Periodically fetch from source (cron job, scheduled task)
- Common for static websites, RSS feeds
- Typical frequency: Every 6-24 hours

**Push-Based Scrapers**:
- Receive webhooks or event notifications from source
- Common for API integrations
- Real-time or near real-time updates

**Hybrid Scrapers**:
- Combination of pull and push
- Example: Webhook for new events, daily full sync for updates

---

## Idempotency and Deduplication

### Problem

Running a scraper multiple times can create duplicate events if not handled properly:

```
Run 1: Creates event "Jazz Night" (ID: evt-001)
Run 2: Creates duplicate "Jazz Night" (ID: evt-002)  ← Problem!
```

### Solution 1: Idempotency Keys

Use the `Idempotency-Key` header to ensure duplicate requests return the same event:

```python
import hashlib
import requests

def submit_event(event, source_url):
    # Generate stable idempotency key from source URL + source ID
    key_source = f"{source_url}#{event['source_event_id']}"
    idempotency_key = hashlib.sha256(key_source.encode()).hexdigest()
    
    response = requests.post(
        f'{SEL_API_URL}/events',
        json=event,
        headers={
            'Authorization': f'Bearer {API_KEY}',
            'Content-Type': 'application/ld+json',
            'Idempotency-Key': idempotency_key
        }
    )
    
    if response.status_code in [201, 200]:
        # 201: Created, 200: Already exists (idempotency)
        return response.json()
    else:
        raise Exception(f"Failed to submit event: {response.status_code}")
```

**How it works**:
- First request with key `abc123`: Creates event, returns 201 Created
- Second request with same key: Returns existing event, returns 200 OK (cached)
- **Idempotency window**: 24 hours (after that, treated as new request)

### Solution 2: Source Event IDs

Always include `source.eventId` and `source.url` to enable server-side deduplication:

```json
{
  "name": "Jazz in the Park",
  "startDate": "2025-08-15T19:00:00-04:00",
  "location": {
    "name": "Centennial Park"
  },
  "source": {
    "url": "https://example.com/events/jazz-in-the-park",
    "eventId": "evt-12345"  ← Unique ID from source system
  }
}
```

**Server-side deduplication**:
- SEL checks if event with same `source.url` OR `source.eventId` already exists
- If found, merges new data with existing event (updates fields)
- If not found, creates new event

### Solution 3: Local Cache

Maintain a local cache of submitted events to avoid unnecessary API calls:

```python
import json
import os

CACHE_FILE = '.scraper_cache.json'

def load_cache():
    if os.path.exists(CACHE_FILE):
        with open(CACHE_FILE, 'r') as f:
            return json.load(f)
    return {}

def save_cache(cache):
    with open(CACHE_FILE, 'w') as f:
        json.dump(cache, f)

def scrape_and_submit(source_url):
    cache = load_cache()
    
    events = fetch_events_from_source(source_url)
    
    for event in events:
        source_id = event['source']['eventId']
        
        # Check if already submitted
        if source_id in cache:
            print(f"Skipping {event['name']} (already submitted)")
            continue
        
        # Submit to API
        result = submit_event(event)
        
        # Cache the submitted event
        cache[source_id] = {
            'submitted_at': datetime.now().isoformat(),
            'sel_id': result['@id']
        }
    
    save_cache(cache)
```

**Cache invalidation**: Clear cache if:
- Source event is updated (check `last_modified` timestamp)
- Cache is older than 30 days
- Full resync is needed

---

## Batch Submission

Batch submission is not available. Submit events individually and rely on idempotency keys plus duplicate detection.

### Batch Size Recommendations

Batch sizing guidance is omitted until batch submission is available.

---

## Error Handling and Retries

### Error Types

**Transient Errors** (Should retry):
- `429 Too Many Requests` - Rate limit exceeded
- `500 Internal Server Error` - Temporary server issue
- `503 Service Unavailable` - Server maintenance
- Network timeouts

**Permanent Errors** (Don't retry):
- `400 Bad Request` - Invalid data (fix scraper)
- `401 Unauthorized` - Invalid API key (check credentials)
- `403 Forbidden` - Insufficient permissions (wrong role)
- `422 Unprocessable Entity` - Validation failed (fix data)

### Retry Strategy

**Exponential Backoff with Jitter**:

```python
import time
import random

def api_call_with_retry(func, max_retries=3):
    for attempt in range(max_retries):
        try:
            return func()
        except requests.exceptions.RequestException as e:
            if attempt == max_retries - 1:
                raise  # Last attempt, give up
            
            # Check if retryable error
            if hasattr(e, 'response') and e.response:
                status = e.response.status_code
                if status in [400, 401, 403, 422]:
                    raise  # Permanent error, don't retry
            
            # Exponential backoff: 1s, 2s, 4s, 8s...
            base_wait = 2 ** attempt
            # Add jitter: randomize wait time ±25%
            jitter = random.uniform(0.75, 1.25)
            wait_time = base_wait * jitter
            
            print(f"Retry {attempt + 1}/{max_retries} after {wait_time:.1f}s: {e}")
            time.sleep(wait_time)

# Usage
def submit_event(event):
    return api_call_with_retry(
        lambda: requests.post(f'{SEL_API_URL}/events', json=event, headers=headers)
    )
```

### Handling Rate Limits

**Respect `Retry-After` header**:

```python
def handle_rate_limit(response):
    if response.status_code == 429:
        retry_after = int(response.headers.get('Retry-After', 60))
        print(f"Rate limited. Waiting {retry_after}s...")
        time.sleep(retry_after)
        return True
    return False

def submit_with_rate_limit_handling(event):
    while True:
        response = requests.post(
            f'{SEL_API_URL}/events',
            json=event,
            headers={'Authorization': f'Bearer {API_KEY}'}
        )
        
        if handle_rate_limit(response):
            continue  # Retry after waiting
        
        return response
```

### Partial Batch Failures

When batch submission is available, handle partial failures gracefully. Treat each event as an independent submission with idempotency keys.

---

## Rate Limiting Strategies

### Monitoring Rate Limits

**Track headers in every response**:

```python
class RateLimitTracker:
    def __init__(self):
        self.limit = None
        self.remaining = None
        self.reset_time = None
    
    def update(self, response):
        self.limit = int(response.headers.get('X-RateLimit-Limit', 0))
        self.remaining = int(response.headers.get('X-RateLimit-Remaining', 0))
        self.reset_time = int(response.headers.get('X-RateLimit-Reset', 0))
    
    def should_wait(self, threshold=10):
        """Return True if remaining requests below threshold"""
        return self.remaining and self.remaining < threshold
    
    def wait_until_reset(self):
        if self.reset_time:
            wait_seconds = max(0, self.reset_time - time.time())
            print(f"Approaching rate limit. Waiting {wait_seconds:.0f}s...")
            time.sleep(wait_seconds + 1)  # +1 for safety

# Usage
tracker = RateLimitTracker()

for event in events:
    if tracker.should_wait(threshold=10):
        tracker.wait_until_reset()
    
    response = submit_event(event)
    tracker.update(response)
```

### Throttling Strategies

**1. Fixed Delay Between Requests**:

```python
import time

def scrape_with_throttle(events, delay=0.5):
    for event in events:
        submit_event(event)
        time.sleep(delay)  # 0.5s = max 120 req/min
```

**2. Token Bucket Algorithm**:

```python
import time
from collections import deque

class TokenBucket:
    def __init__(self, rate_per_minute=300):
        self.rate = rate_per_minute
        self.bucket = deque(maxlen=rate_per_minute)
    
    def acquire(self):
        now = time.time()
        
        # Remove tokens older than 1 minute
        while self.bucket and now - self.bucket[0] > 60:
            self.bucket.popleft()
        
        # Check if bucket is full
        if len(self.bucket) >= self.rate:
            # Calculate wait time
            wait_time = 60 - (now - self.bucket[0])
            time.sleep(wait_time)
            self.bucket.popleft()
        
        # Add new token
        self.bucket.append(time.time())

# Usage
bucket = TokenBucket(rate_per_minute=300)

for event in events:
    bucket.acquire()  # Blocks if rate limit would be exceeded
    submit_event(event)
```

**3. Adaptive Rate Limiting**:

```python
class AdaptiveRateLimiter:
    def __init__(self, initial_delay=0.2, max_delay=2.0):
        self.delay = initial_delay
        self.max_delay = max_delay
        self.min_delay = initial_delay
    
    def on_success(self):
        # Speed up on success
        self.delay = max(self.min_delay, self.delay * 0.9)
    
    def on_rate_limit(self):
        # Slow down on rate limit
        self.delay = min(self.max_delay, self.delay * 2)
    
    def wait(self):
        time.sleep(self.delay)

# Usage
limiter = AdaptiveRateLimiter()

for event in events:
    limiter.wait()
    
    response = submit_event(event)
    
    if response.status_code == 429:
        limiter.on_rate_limit()
    else:
        limiter.on_success()
```

---

## Data Quality

### Validation Before Submission

**Required Fields**:
- `name` (string, 1-500 chars)
- `startDate` (ISO8601 with timezone)
- `location` (Place or VirtualLocation)

**Validation Function**:

```python
from datetime import datetime
import re

def validate_event(event):
    errors = []
    
    # Required fields
    if not event.get('name'):
        errors.append("Missing required field: name")
    elif len(event['name']) > 500:
        errors.append("name too long (max 500 chars)")
    
    # Date validation
    if not event.get('startDate'):
        errors.append("Missing required field: startDate")
    else:
        try:
            datetime.fromisoformat(event['startDate'].replace('Z', '+00:00'))
        except ValueError:
            errors.append("Invalid startDate format (must be ISO8601)")
    
    # Location validation
    if not event.get('location'):
        errors.append("Missing required field: location")
    elif not event['location'].get('name'):
        errors.append("location.name is required")
    
    # End date logic
    if event.get('endDate'):
        try:
            start = datetime.fromisoformat(event['startDate'].replace('Z', '+00:00'))
            end = datetime.fromisoformat(event['endDate'].replace('Z', '+00:00'))
            if end < start:
                errors.append("endDate must be after startDate")
        except ValueError:
            errors.append("Invalid endDate format")
    
    return errors

# Usage
def submit_with_validation(event):
    errors = validate_event(event)
    if errors:
        print(f"Validation failed for {event.get('name', 'Unknown')}: {errors}")
        return None
    
    return submit_event(event)
```

### Data Enrichment

**Geocoding**:

```python
from geopy.geocoders import Nominatim

def enrich_with_geocoding(event):
    location = event.get('location', {})
    
    # Skip if already has coordinates
    if location.get('latitude') and location.get('longitude'):
        return event
    
    # Build address string
    address_parts = [
        location.get('streetAddress'),
        location.get('addressLocality'),
        location.get('addressRegion'),
        location.get('postalCode'),
        location.get('addressCountry', 'CA')
    ]
    address = ', '.join(filter(None, address_parts))
    
    if not address:
        return event
    
    # Geocode
    geolocator = Nominatim(user_agent="sel_scraper")
    try:
        location_result = geolocator.geocode(address)
        if location_result:
            event['location']['latitude'] = location_result.latitude
            event['location']['longitude'] = location_result.longitude
            print(f"Geocoded: {address} → ({location_result.latitude}, {location_result.longitude})")
    except Exception as e:
        print(f"Geocoding failed for {address}: {e}")
    
    return event
```

### Normalizing Data

**Text Normalization**:

```python
import re
from html import unescape

def normalize_text(text):
    if not text:
        return text
    
    # Decode HTML entities
    text = unescape(text)
    
    # Strip HTML tags
    text = re.sub(r'<[^>]+>', '', text)
    
    # Normalize whitespace
    text = ' '.join(text.split())
    
    # Trim to max length
    if len(text) > 10000:
        text = text[:9997] + '...'
    
    return text

# Usage
event['name'] = normalize_text(scraped_name)
event['description'] = normalize_text(scraped_description)
```

---

## Monitoring and Logging

### Structured Logging

```python
import logging
import json
from datetime import datetime

# Configure structured logging
logging.basicConfig(
    level=logging.INFO,
    format='%(message)s',
    handlers=[
        logging.FileHandler('scraper.log'),
        logging.StreamHandler()
    ]
)

def log_structured(level, message, **kwargs):
    log_entry = {
        'timestamp': datetime.now().isoformat(),
        'level': level,
        'message': message,
        **kwargs
    }
    logging.log(getattr(logging, level.upper()), json.dumps(log_entry))

# Usage
log_structured('info', 'Scraper started', source='example.com')
log_structured('info', 'Event submitted', event_name='Jazz Night', sel_id='01HYX...')
log_structured('error', 'Submission failed', event_name='Bad Event', error='Invalid date')
```

### Metrics Collection

```python
from collections import defaultdict

class ScraperMetrics:
    def __init__(self):
        self.metrics = defaultdict(int)
        self.start_time = time.time()
    
    def increment(self, metric):
        self.metrics[metric] += 1
    
    def summary(self):
        duration = time.time() - self.start_time
        return {
            'duration_seconds': duration,
            'events_processed': self.metrics['processed'],
            'events_created': self.metrics['created'],
            'events_updated': self.metrics['updated'],
            'events_skipped': self.metrics['skipped'],
            'events_failed': self.metrics['failed'],
            'api_calls': self.metrics['api_calls'],
            'rate_limit_hits': self.metrics['rate_limited']
        }

# Usage
metrics = ScraperMetrics()

for event in events:
    metrics.increment('processed')
    
    try:
        result = submit_event(event)
        metrics.increment('api_calls')
        
        if result['created']:
            metrics.increment('created')
        else:
            metrics.increment('updated')
    except RateLimitError:
        metrics.increment('rate_limited')
    except Exception:
        metrics.increment('failed')

# Log summary
log_structured('info', 'Scraper completed', **metrics.summary())
```

---

## Example Implementation

Example scrapers live in `docs/integration/examples/`:

- `docs/integration/examples/minimal_scraper.js`
- `docs/integration/examples/idempotency_example.py`
- `docs/integration/examples/batch_scraper.py`

---

## Best Practices Summary

### Do's

- ✅ Use idempotency keys to prevent duplicates
- ✅ Include `source.eventId` and `source.url` in all events
- ✅ Use idempotency keys for safe retries
- ✅ Implement exponential backoff with jitter for retries
- ✅ Validate data before submission
- ✅ Monitor rate limit headers and throttle accordingly
- ✅ Log structured data (JSON) for observability
- ✅ Collect metrics (created, updated, skipped, failed)
- ✅ Respect source website robots.txt and rate limits

### Don'ts

- ❌ Don't ignore rate limit headers
- ❌ Don't retry on 4xx errors (400, 401, 403, 422)
- ❌ Don't submit unvalidated data
- ❌ Don't hardcode API keys in source code
- ❌ Don't log sensitive data (API keys, user info)
- ❌ Don't scrape source website too frequently (respect bandwidth)

---

## Next Steps

- [authentication.md](authentication.md) - API key management and authentication
- [api-guide.md](api-guide.md) - Complete API endpoint reference
- [examples/](examples/) - Scraper examples in multiple languages

**Document Version**: 0.1.0
**Last Updated**: 2026-01-27
**Maintenance**: Update when API changes affect scraper patterns
