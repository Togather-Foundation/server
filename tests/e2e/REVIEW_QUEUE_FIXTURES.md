# Review Queue Test Fixtures - Implementation Summary

## Overview

Extended the Go fixtures and generate command to support creating review queue test data, replacing the Python-based database fixtures with a consistent Go-based approach.

## Changes Made

### 1. Extended `tests/testdata/fixtures.go`

Added five new fixture generator functions:

- **`EventInputReversedDates()`** - Generates late-night events (11pm-2am) where the end date appears before the start date due to timezone confusion
- **`EventInputMissingVenue()`** - Creates virtual/online events with no physical location
- **`EventInputLikelyDuplicate()`** - Produces events with recognizable patterns suggesting duplication (e.g., "Weekly Community Meetup")
- **`EventInputMultipleWarnings()`** - Generates events with several data quality issues (missing description, date-only format, missing image, partial location)
- **`BatchReviewQueueInputs(count int)`** - Batch generator that rotates through all warning scenarios

### 2. Updated `cmd/server/cmd/generate.go`

Added `--review-queue` flag to the generate command:

```bash
# Generate 5 review queue test events
server generate review-queue-fixtures.json --count 5 --review-queue

# With specific seed for reproducibility
server generate fixtures.json --count 10 --review-queue --seed 42
```

When `--review-queue` is set, the command uses `BatchReviewQueueInputs()` instead of `RandomEventInput()` to create events with data quality issues.

### 3. Created `tests/e2e/setup_fixtures.sh`

Bash script that orchestrates fixture generation and ingestion:

```bash
# Setup 5 review queue fixtures
./tests/e2e/setup_fixtures.sh 5

# With specific seed
./tests/e2e/setup_fixtures.sh 10 42
```

Features:
- Checks for prerequisites (DATABASE_URL, server binary)
- Generates fixtures using `server generate --review-queue`
- Ingests fixtures using `server ingest`
- Provides helpful next-steps output
- Cleans up temporary files automatically

### 4. Created `tests/e2e/cleanup_fixtures.sh`

Bash script for removing test fixtures:

```bash
./tests/e2e/cleanup_fixtures.sh
```

Features:
- Removes review queue entries, events, and occurrences
- Uses SQL patterns to identify test data (ULID patterns, event name patterns)
- Safe: only deletes data matching test patterns

### 5. Updated `tests/e2e/test_review_queue.py`

Replaced Python database fixture code with calls to bash scripts:

**Before:**
- Imported `psycopg2` and directly manipulated database
- Created places, events, occurrences, and review queue entries via SQL
- Required Python dependencies for database access

**After:**
- Calls `setup_fixtures.sh` via `subprocess`
- Calls `cleanup_fixtures.sh` for teardown
- No direct database access from Python
- Reuses Go ingestion pipeline

## Usage

### Running E2E Tests

```bash
# Ensure server is running
make dev

# In another terminal
source .env  # Load DATABASE_URL
uvx --from playwright --with playwright python tests/e2e/test_review_queue.py
```

### Manual Testing

```bash
# Generate fixtures
./bin/togather-server generate fixtures.json --count 5 --review-queue

# Ingest them
./bin/togather-server ingest fixtures.json

# View in UI
open http://localhost:8080/admin/review-queue

# Cleanup
./tests/e2e/cleanup_fixtures.sh
```

### Using Fixtures in Other Tests

```go
import "github.com/Togather-Foundation/server/tests/testdata"

gen := testdata.NewDeterministicGenerator()

// Single fixtures
reversedDates := gen.EventInputReversedDates()
missingVenue := gen.EventInputMissingVenue()
likelyDupe := gen.EventInputLikelyDuplicate()
multipleWarnings := gen.EventInputMultipleWarnings()

// Batch of varied scenarios
reviewEvents := gen.BatchReviewQueueInputs(10)
```

## Benefits

1. **Consistency** - Uses same Go code paths as production ingestion
2. **Maintainability** - Single source of truth for event structure
3. **Reusability** - Go fixtures can be used in unit tests, integration tests, and E2E tests
4. **No Duplication** - Eliminates Python database logic that mirrored Go behavior
5. **Better Errors** - Real ingestion pipeline provides realistic error scenarios
6. **Type Safety** - Go compiler catches schema mismatches

## Test Data Patterns

Generated events include these review-triggering scenarios:

1. **Reversed Dates** - Late-night events where `endDate < startDate` due to timezone confusion
2. **Missing Venue** - Online events without physical location
3. **Likely Duplicates** - Events with "Weekly" prefix and recognizable patterns
4. **Multiple Issues** - Events missing description, image, with date-only timestamps, partial location data

## Architecture

```
┌─────────────────────────────────────┐
│  tests/testdata/fixtures.go         │
│  (Go event generators)               │
└────────────────┬────────────────────┘
                 │
                 ↓
┌─────────────────────────────────────┐
│  cmd/server/cmd/generate.go         │
│  (CLI: server generate)              │
└────────────────┬────────────────────┘
                 │
                 ↓
┌─────────────────────────────────────┐
│  tests/e2e/setup_fixtures.sh        │
│  (Bash orchestration)                │
└────────────────┬────────────────────┘
                 │
                 ↓
┌─────────────────────────────────────┐
│  cmd/server/cmd/ingest.go           │
│  (CLI: server ingest)                │
└────────────────┬────────────────────┘
                 │
                 ↓
┌─────────────────────────────────────┐
│  internal/jobs/event_ingestion.go   │
│  (Real ingestion pipeline)           │
└────────────────┬────────────────────┘
                 │
                 ↓
┌─────────────────────────────────────┐
│  PostgreSQL Database                 │
│  (events, review_queue, etc.)        │
└─────────────────────────────────────┘
```

## Future Improvements

- Add more warning scenarios (far future dates, suspicious URLs, etc.)
- Support filtering by warning type in `generate --review-queue`
- Add `--pattern` flag to generate specific scenarios
- Create Go-based cleanup command instead of bash script with SQL
- Add fixture presets (e.g., `--preset minimal`, `--preset comprehensive`)
