# Review Queue Fixtures - Quick Reference

## Generate Review Queue Test Data

```bash
# Generate 5 review queue events to a file
./bin/togather-server generate fixtures.json --count 5 --review-queue

# With specific seed for reproducibility
./bin/togather-server generate fixtures.json --count 10 --review-queue --seed 42

# Generate to stdout (for piping)
./bin/togather-server generate --review-queue --count 3
```

## Setup Fixtures for E2E Tests

```bash
# Setup 5 review queue fixtures (generate + ingest)
./tests/e2e/setup_fixtures.sh 5

# With specific seed
./tests/e2e/setup_fixtures.sh 10 42

# Cleanup test fixtures
./tests/e2e/cleanup_fixtures.sh
```

## Run Review Queue E2E Tests

```bash
# Start server
make dev

# In another terminal
source .env
uvx --from playwright --with playwright python tests/e2e/test_review_queue.py
```

## Fixture Types

The `--review-queue` flag generates events that rotate through these scenarios:

1. **Reversed Dates** - Late-night events where end appears before start
   - Event name includes "(Late Night)" suffix
   - `startDate`: 23:00, `endDate`: 02:00 (same date = wrong!)

2. **Missing Venue** - Online events without physical location
   - Event name includes "(Online)" suffix
   - Has `virtualLocation` but no `location` field

3. **Likely Duplicate** - Events suggesting duplication
   - Name pattern: "Weekly Community Meetup at [venue]"
   - Regular occurrence implied in description

4. **Multiple Warnings** - Events with several issues
   - Missing: description, endDate, image
   - Date-only format (no time): `2026-04-09`
   - Partial location data (name + locality only)

## Examples

### Generate and View

```bash
# Generate
./bin/togather-server generate /tmp/review.json --count 3 --review-queue

# View
cat /tmp/review.json | jq '.events[0]'

# Ingest
./bin/togather-server ingest /tmp/review.json
```

### Check Review Queue

```bash
# Via web UI
open http://localhost:8080/admin/review-queue

# Via database
source .env
psql "$DATABASE_URL" -c "SELECT id, event_start_time, warnings FROM event_review_queue WHERE status = 'pending';"
```

### Cleanup

```bash
# Using cleanup script
./tests/e2e/cleanup_fixtures.sh

# Manual SQL cleanup
psql "$DATABASE_URL" -c "
  DELETE FROM event_review_queue 
  WHERE event_id IN (
    SELECT id FROM events 
    WHERE name LIKE '%(Late Night)%' 
       OR name LIKE '%(Online)%'
       OR name LIKE 'Weekly Community Meetup%'
  );
"
```
