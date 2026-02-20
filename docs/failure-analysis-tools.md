# Failure Analysis Tools

Tools for analyzing and reporting on failed events during batch ingestion.

## Quick Start

### 1. Run Ingestion with Automatic Failure Detection

```bash
# Ingest events and automatically see failure summaries
./scripts/ingest-toronto-events.sh staging 50 300
```

The script will automatically:
- Submit events in batches
- Wait for processing to complete
- Show summary of created/failed/duplicate events
- Display failure details inline
- Provide commands for deeper analysis

### 2. Export Detailed Failure Reports

```bash
# Export failures from specific batch(es)
./scripts/export-failures.sh staging <batch_id1> [batch_id2] ...
```

**Example**:
```bash
./scripts/export-failures.sh staging 01KGWE1R0WHTG3DN2G3FJEQT1X
```

**Output**:
- `failure-reports/failures_staging_YYYYMMDD_HHMMSS.json` - Machine-readable JSON
- `failure-reports/failures_staging_YYYYMMDD_HHMMSS.md` - Human-readable markdown report

## Tools Overview

### `ingest-toronto-events.sh`

**Purpose**: Ingest events from Toronto Open Data and track failures

**Features**:
- Batched ingestion with configurable batch size
- Real-time progress tracking
- Automatic failure collection and reporting
- Shows event names and error messages for each failure
- Provides commands for detailed analysis

**Usage**:
```bash
./scripts/ingest-toronto-events.sh [staging|local] [batch_size] [max_events]
```

**Examples**:
```bash
# Ingest 300 events in batches of 50
./scripts/ingest-toronto-events.sh staging 50 300

# Ingest all events locally
./scripts/ingest-toronto-events.sh local 50 0

# Test with just 10 events
./scripts/ingest-toronto-events.sh staging 5 10
```

**Output**:
```
========================================
Batch Processing Results
========================================

Checking batch: 01KGWE1R0WHTG3DN2G3FJEQT1X
  ✓ Complete: 41 created, 1 failed, 8 duplicates (total: 50)
  Failed events:
    - Monday Latin Nights: invalid endDate: must be on or after startDate

========================================
Final Summary
========================================
Total events created: 189
Total events failed: 1
Total duplicates: 28
```

### `export-failures.sh`

**Purpose**: Export detailed failure information with full event data

**Features**:
- Queries database for complete failure details
- Generates both JSON and Markdown reports
- Includes full event JSON for each failure
- Shows error message, event name, dates, location, organizer
- Human-readable summary in terminal

**Usage**:
```bash
./scripts/export-failures.sh [staging|local] <batch_id1> [batch_id2] ...
```

**Examples**:
```bash
# Export failures from one batch
./scripts/export-failures.sh staging 01KGWE1R0WHTG3DN2G3FJEQT1X

# Export failures from multiple batches
./scripts/export-failures.sh staging 01KGWE1R0... 01KGWE1SD... 01KGWE1TW...

# Export from local environment
./scripts/export-failures.sh local 01KGWE1R0...
```

**Output Directory**: `failure-reports/`

## Report Formats

### JSON Report

Machine-readable format for programmatic analysis:

```json
[
  {
    "batch_id": "01KGWE1R0WHTG3DN2G3FJEQT1X",
    "completed_at": "2026-02-07T16:13:07.761909+00:00",
    "failures": [
      {
        "index": 28,
        "error": "invalid endDate: must be on or after startDate",
        "event": {
          "name": "Monday Latin Nights with Latin Grooves and Dancing",
          "url": "https://...",
          "startDate": "2025-03-31T23:00:00.000Z",
          "endDate": "2025-03-31T06:00:00.000Z",
          "location": { ... },
          "organizer": { ... }
        }
      }
    ]
  }
]
```

**Use cases**:
- Automated analysis
- Pattern detection
- Integration with other tools
- Statistical analysis

### Markdown Report

Human-readable format for documentation:

```markdown
# Failure Analysis Report

**Environment**: staging
**Generated**: 2026-02-07 16:21:59 UTC
**Total Batches**: 1
**Total Failures**: 1

## Batch #1: `01KGWE1R0...`

### Failure (Index 28)

**Error**: `invalid endDate: must be on or after startDate`

| Field | Value |
|-------|-------|
| Name | Monday Latin Nights... |
| Start Date | `2025-03-31T23:00:00.000Z` |
| End Date | `2025-03-31T06:00:00.000Z` |
```

**Use cases**:
- Bug reports
- Documentation
- Code review
- Issue tracking

## Common Failure Types

### 1. Timezone Errors

**Error**: `invalid endDate: must be on or after startDate`

**Cause**: Midnight-spanning events with incorrect timezone conversion

**Example**:
```json
{
  "startDate": "2025-03-31T23:00:00Z",  // 11 PM
  "endDate": "2025-03-31T06:00:00Z"     // 6 AM (should be next day!)
}
```

**Fix**: Normalization runs before validation (see FAILURE_ANALYSIS.md)

### 2. Source Name Collisions

**Error**: `duplicate key violation on sources_name_key`

**Cause**: Multiple events from same domain trying to create sources with identical names

**Fix**: Migration 000023 (partial unique indexes)

### 3. Validation Errors

**Errors**:
- `name: required`
- `location: location or virtualLocation required`
- `invalid URI`

**Cause**: Missing required fields or invalid formats

**Fix**: Data quality improvements at source

## Analysis Workflows

### Workflow 1: Post-Ingestion Analysis

```bash
# 1. Run ingestion
./scripts/ingest-toronto-events.sh staging 50 300

# 2. Note any batch IDs with failures

# 3. Export detailed reports
./scripts/export-failures.sh staging <batch_ids...>

# 4. Analyze patterns
cat failure-reports/failures_*.json | jq '[.[] | .failures[] | .error] | group_by(.) | map({error: .[0], count: length})'
```

### Workflow 2: Pattern Detection

```bash
# Find common error types
jq '[.[] | .failures[] | .error] | group_by(.) | map({error: .[0], count: length})' \
  failure-reports/failures_staging_*.json

# Find events from specific organizer
jq '.[] | .failures[] | select(.event.organizer.name == "DROM Taberna")' \
  failure-reports/failures_staging_*.json

# Extract all failed event URLs
jq -r '.[] | .failures[] | .event.url' \
  failure-reports/failures_staging_*.json
```

### Workflow 3: Regression Testing

```bash
# Before fix
./scripts/ingest-toronto-events.sh staging 50 300
./scripts/export-failures.sh staging <batch_ids> 
cp failure-reports/failures_staging_*.json before-fix.json

# Apply fix
git checkout feature/my-fix

# After fix
./scripts/staging-reset.sh --yes
./scripts/ingest-toronto-events.sh staging 50 300
./scripts/export-failures.sh staging <batch_ids>
cp failure-reports/failures_staging_*.json after-fix.json

# Compare
echo "Before: $(jq '[.[] | .failures | length] | add' before-fix.json) failures"
echo "After: $(jq '[.[] | .failures | length] | add' after-fix.json) failures"
```

## Database Queries

### Manual failure queries

```bash
# Get all failures from recent batches
psql "$DATABASE_URL" -c "
  SELECT 
    bir.batch_id,
    (result->>'index')::int AS index,
    result->>'error' AS error,
    result->'event'->>'name' AS event_name
  FROM batch_ingestion_results bir,
       jsonb_array_elements(bir.results) AS result
  WHERE result->>'status' = 'failed'
    AND bir.completed_at >= NOW() - INTERVAL '1 day'
  ORDER BY bir.completed_at DESC;
"

# Count failures by error type
psql "$DATABASE_URL" -c "
  SELECT 
    result->>'error' AS error,
    COUNT(*) AS count
  FROM batch_ingestion_results bir,
       jsonb_array_elements(bir.results) AS result
  WHERE result->>'status' = 'failed'
  GROUP BY result->>'error'
  ORDER BY count DESC;
"

# Find events that failed validation
psql "$DATABASE_URL" -c "
  SELECT 
    result->'event'->>'name' AS event_name,
    result->'event'->>'startDate' AS start_date,
    result->'event'->>'endDate' AS end_date,
    result->>'error' AS error
  FROM batch_ingestion_results bir,
       jsonb_array_elements(bir.results) AS result
  WHERE result->>'status' = 'failed'
    AND result->>'error' ILIKE '%endDate%';
"
```

## Troubleshooting

### "No failures found"

**Cause**: Batch IDs are incorrect or batches haven't completed processing

**Solution**:
```bash
# Check if batches exist
psql "$DATABASE_URL" -c "SELECT batch_id, completed_at FROM batch_ingestion_results ORDER BY completed_at DESC LIMIT 10;"

# Wait longer if processing
sleep 10 && ./scripts/export-failures.sh staging <batch_id>
```

### "DATABASE_URL not set"

**Cause**: Environment configuration not loaded

**Solution**:
```bash
# Staging
source .deploy.conf.staging

# Local
source .env

# Verify
echo $DATABASE_URL
```

### "Permission denied: export-failures.sh"

**Cause**: Script not executable

**Solution**:
```bash
chmod +x scripts/export-failures.sh
```

## See Also

- `FAILURE_ANALYSIS.md` - Detailed analysis of 300-event test
- `docs/deploy/deployment-testing.md` - Full deployment testing checklist
- `STAGING_TEST_RESULTS.md` - Staging test results summary

---

**Last Updated:** 2026-02-20
