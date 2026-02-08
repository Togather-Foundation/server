# Review Queue Test Fixtures

Test fixtures for populating the database with review queue test data for E2E testing.

## Overview

The `review_queue_fixture.py` module provides functions to populate the database with realistic test data for testing the review queue UI and workflow.

## Features

- **Direct database access** using psycopg2 for fast fixture setup
- **Realistic test scenarios** covering common warning types
- **Transactional cleanup** with rollback on error
- **Reusable helper functions** for creating events, venues, and review entries
- **Configurable via environment** using `DATABASE_URL`

## Test Scenarios Created

The fixtures create 5 test review queue entries with different warning scenarios:

1. **Reversed date warning** - Event with end time before start time due to timezone confusion (11pm-2am becomes same day)
2. **Missing venue warning** - Online event with no physical location specified
3. **Duplicate detection warning** - Event flagged as potential duplicate
4. **Multiple warnings** - Event with several issues (time inferred, duration assumed, location fuzzy matched)
5. **Timezone assumed warning** - Event with no timezone, assumed from venue location

## Usage

### In E2E Tests

```python
from fixtures.review_queue_fixture import (
    setup_review_queue_fixtures,
    cleanup_review_queue_fixtures
)

# In your test setup
fixture_data = setup_review_queue_fixtures()

# Run your tests...
test_review_queue_ui(page)

# In your test teardown
cleanup_review_queue_fixtures(fixture_data)
```

### Standalone

```bash
# Setup fixtures
source .env
python tests/e2e/fixtures/review_queue_fixture.py setup

# Cleanup fixtures
python tests/e2e/fixtures/review_queue_fixture.py cleanup
```

## Return Value

`setup_review_queue_fixtures()` returns a dictionary with:

```python
{
    'entry_ids': [1, 2, 3, 4, 5],        # Review queue entry IDs
    'event_ids': ['uuid1', 'uuid2', ...], # Event UUIDs
    'place_ids': ['uuid1', ...]           # Place UUIDs
}
```

Pass this to `cleanup_review_queue_fixtures()` to clean up specific fixtures, or call with no arguments to clean up all test fixtures (by ULID pattern `TESTRQ%`).

## Configuration

Set the `DATABASE_URL` environment variable:

```bash
export DATABASE_URL="postgresql://user:pass@localhost:5433/togather?sslmode=disable"
```

Or source your `.env` file:

```bash
source .env
```

## Helper Functions

### Database Connection

- `get_db_connection()` - Create database connection from `DATABASE_URL`

### Data Creation

- `create_test_place(conn, name, locality, region)` - Create a venue
- `create_test_event(conn, ulid, name, desc, venue_id, lifecycle_state)` - Create an event
- `create_test_occurrence(conn, event_id, start_time, venue_id, end_time, tz)` - Create occurrence
- `create_review_queue_entry(conn, event_id, payloads, warnings, ...)` - Create review entry

### Setup & Cleanup

- `setup_review_queue_fixtures()` - Create all test fixtures
- `cleanup_review_queue_fixtures(created_data)` - Remove test fixtures

## Error Handling

All database operations use transactions and rollback on error. Cleanup is safe to call multiple times.

```python
try:
    fixture_data = setup_review_queue_fixtures()
    run_tests()
except Exception as e:
    print(f"Failed: {e}")
finally:
    cleanup_review_queue_fixtures(fixture_data)
```

## Database Schema

Fixtures work with these tables:

- `places` - Venues
- `events` - Events with `pending_review` lifecycle state
- `event_occurrences` - Event start/end times
- `event_review_queue` - Review queue entries with warnings

## Notes

- Fixtures use predictable ULIDs starting with `TESTRQ` for easy identification
- All events are created with `pending_review` lifecycle state
- Occurrences are required by the `event_location_required` constraint
- Review entries default to `pending` status
- Cleanup by pattern (`TESTRQ%`) allows cleaning up orphaned fixtures

## Dependencies

### Python Packages

Install `psycopg2-binary`:

```bash
pip install psycopg2-binary
```

Or use `uvx` to run without installing:

```bash
uvx --with psycopg2-binary python tests/e2e/fixtures/review_queue_fixture.py setup
```

### Environment

- `DATABASE_URL` environment variable must be set

## Example: Manual Testing

```bash
# 1. Setup fixtures
source .env
python tests/e2e/fixtures/review_queue_fixture.py setup > /tmp/fixtures.json

# 2. View in browser
open http://localhost:8080/admin/review-queue

# 3. Cleanup when done
python tests/e2e/fixtures/review_queue_fixture.py cleanup
```

## Integration with test_review_queue.py

The `test_review_queue.py` E2E test automatically:

1. Sets up fixtures before tests
2. Passes fixture data to tests that need it
3. Cleans up fixtures in finally block
4. Gracefully handles fixture failures (tests continue, some may skip)

Tests that require fixtures:
- `test_expand_collapse_detail_view`
- `test_action_buttons_in_detail_view`
- `test_reject_modal_requires_reason`
- `test_fix_dates_form_functionality`
