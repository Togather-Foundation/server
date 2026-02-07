# Toronto Events Ingestion Script

## Overview

Script to ingest Toronto events from the CivicTechTO JSON-LD proxy into SEL nodes (staging or local).

**Source:** https://civictechto.github.io/toronto-opendata-festivalsandevents-jsonld-proxy/all.jsonld  
**Total Events Available:** ~1,671 events (as of Feb 2026)

## Usage

```bash
# Ingest to staging with default batch size (50)
./scripts/ingest-toronto-events.sh staging

# Ingest first 100 events to staging in batches of 50
./scripts/ingest-toronto-events.sh staging 50 100

# Ingest to local development server
./scripts/ingest-toronto-events.sh local

# Test with small batch (5 events)
./scripts/ingest-toronto-events.sh staging 5 5
```

## Requirements

1. **API Key:** Script reads from `.deploy.conf.staging` or `.env` file
   - Variable: `PERF_AGENT_API_KEY`
   - Format: API key must be sent as `Authorization: Bearer <key>`

2. **Dependencies:** 
   - `curl` (HTTP requests)
   - `jq` (JSON processing)
   - `bash` (shell)

3. **River Worker:** The background job processor must be running to process batches
   - **IMPORTANT:** Currently the River worker is NOT running on staging
   - This prevents batch jobs from being processed
   - See issue below

## Authentication

The batch ingestion endpoint requires API key authentication:

```bash
Authorization: Bearer <PERF_AGENT_API_KEY>
```

The API key must have:
- Active status
- Valid expiration date (if set)
- Proper prefix (first 8 characters used for lookup)
- Valid hash (bcrypt or legacy SHA-256)

## Field Mappings

The script transforms schema.org Event format to SEL EventInput format:

| Schema.org Field | SEL EventInput Field | Notes |
|------------------|---------------------|-------|
| `name` | `name` | Direct copy |
| `description` | `description` | Direct copy |
| `startDate` | `startDate` | RFC3339 format |
| `endDate` | `endDate` | RFC3339 format |
| `url` | `url` | Public event URL |
| `image` | `image` | Image URL |
| `keywords[]` | `keywords[]` | Array of strings |
| `location.address.streetAddress` | `location.streetAddress` | Flattened structure |
| `location.address.addressLocality` | `location.addressLocality` | Flattened structure |
| `location.address.addressRegion` | `location.addressRegion` | Flattened structure |
| `location.address.postalCode` | `location.postalCode` | Flattened structure |
| `location.address.addressCountry` | `location.addressCountry` | Flattened structure |
| `location.geo.latitude` | `location.latitude` | Flattened structure |
| `location.geo.longitude` | `location.longitude` | Flattened structure |
| `organizer.name` | `organizer.name` | Basic fields only |
| `organizer.url` | `organizer.url` | Basic fields only |
| `offers.price` (number) | `offers.price` (string) | Type conversion |
| `offers.priceCurrency` | `offers.priceCurrency` | Direct copy |
| `offers.url` | `offers.url` | Direct copy |
| `isAccessibleForFree` | `isAccessibleForFree` | Direct copy |

### Additions

The script adds:

```json
{
  "license": "https://creativecommons.org/publicdomain/zero/1.0/",
  "source": {
    "url": "<event.url>",
    "eventId": "<extracted-from-url>",
    "name": "Toronto Open Data",
    "license": "https://creativecommons.org/publicdomain/zero/1.0/"
  }
}
```

## Schema.org Compatibility Issues

**Created Bead:** `srv-zqh` - "Support full schema.org Event ingestion format"

The current SEL EventInput format differs from standard schema.org in several ways:

1. **Nested Address Structure:** schema.org uses `location.address.streetAddress` but we expect flat `location.streetAddress`
2. **Nested Geo Coordinates:** schema.org uses `location.geo.latitude` but we expect flat `location.latitude`  
3. **Organization Details:** schema.org organizer has `email`/`telephone` fields we don't capture
4. **Offers Price Type:** schema.org uses number, we use string
5. **Context Handling:** We don't properly parse/normalize schema.org `@context`

This violates the SEL Core Interoperability Profile principle: **"Schema.org First"**

See:
- `docs/interop/CORE_PROFILE_v0.1.md` - Section 1 (URI Scheme)
- `docs/interop/API_CONTRACT_v1.md` - Section 1 (Public Read API)

## Batch Processing Status

**Issue Found:** River worker not running on staging

```bash
# Check if River worker is running
ssh togather "ps aux | grep river | grep -v grep"
# Output: (empty - worker not running)
```

This means:
- Batch submissions are accepted (HTTP 202)
- Jobs are queued in database
- Jobs are NOT processed
- Batch status returns 404 "still processing" indefinitely

**Resolution Required:**
1. Start River worker on staging server
2. Configure as systemd service for auto-restart
3. Monitor job processing logs

## Testing Results

### Test 1: Small Batch (5 events)

```bash
./scripts/ingest-toronto-events.sh staging 5 5
```

**Result:**
- ✓ Batch accepted (HTTP 202)
- ✓ Batch ID: `01KGW6HKSXKH11H2NCCE10AHPR`
- ✓ Job ID: `1`
- ✗ Processing incomplete (River worker not running)

### Expected Flow

1. Script fetches events from Toronto proxy
2. Transforms to SEL format
3. Submits batches via `POST /api/v1/events:batch`
4. River worker processes jobs asynchronously
5. Results stored in `batch_ingestion_results` table
6. Client polls `GET /api/v1/batch-status/{id}` for completion

## Next Steps

1. **Fix River Worker:** Start River worker on staging
   ```bash
   ssh togather "cd /opt/togather && ./river worker"
   # Or configure as systemd service
   ```

2. **Re-test Ingestion:** Once worker is running
   ```bash
   ./scripts/ingest-toronto-events.sh staging 10 10
   ```

3. **Implement Schema.org Compatibility:** Work on bead `srv-zqh`
   - Add normalization layer in ingestion
   - Support nested address/geo structures
   - Capture additional organizer fields

4. **Production Ingestion:** After testing complete
   ```bash
   ./scripts/ingest-toronto-events.sh staging 100 0  # All events
   ```

## Related Files

- `/home/ryankelln/Documents/Projects/Art/togather/server/scripts/ingest-toronto-events.sh` - Main script
- `/home/ryankelln/Documents/Projects/Art/togather/server/internal/domain/events/validation.go` - EventInput validation
- `/home/ryankelln/Documents/Projects/Art/togather/server/internal/domain/events/ingest.go` - Ingestion logic
- `/home/ryankelln/Documents/Projects/Art/togather/server/internal/api/handlers/events.go` - Batch handler
- `/home/ryankelln/Documents/Projects/Art/togather/server/internal/jobs/workers.go` - River batch worker

## References

- [SEL Core Interoperability Profile](../docs/interop/CORE_PROFILE_v0.1.md)
- [SEL API Contract](../docs/interop/API_CONTRACT_v1.md)
- [Toronto Open Data Events](https://civictechto.github.io/toronto-opendata-festivalsandevents-jsonld-proxy/)
- [Schema.org Event](https://schema.org/Event)
