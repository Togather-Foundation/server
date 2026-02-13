# Batch Integration Tests

This package contains integration tests that require River job queue workers to be running.

## Why Separate?

The batch ingestion tests are the **only** integration tests that require River workers. Starting and stopping River workers adds ~6+ minutes of overhead to test execution. By separating these tests:

- **Regular integration tests** (`tests/integration/`) run faster without River worker startup (26 test files)
- **Batch integration tests** (`tests/integration_batch/`) include River worker overhead but only when needed (7 tests)

## Running Tests

```bash
# Run all integration tests (both regular and batch, no race detector)
make test-ci

# Run all integration tests with race detector (~10min)
make test-ci-race

# Run only regular integration tests (faster, no River workers)
go test ./tests/integration/...

# Run only batch integration tests (with River workers)
go test ./tests/integration_batch/...
```

## What's Tested Here

- Batch event ingestion (`/api/v1/events:batch`)
- Job queue processing and status checks
- Batch validation (empty arrays, size limits)
- Partial failures in batch submissions
- Idempotency and deduplication
- Authentication for batch endpoints

## Implementation Notes

- The `helpers_test.go` in this package **starts River workers** during `setupTestEnv()`
- The `helpers_test.go` in `tests/integration/` does **NOT** start River workers
- Both packages share the same testcontainers PostgreSQL instance
- Tests use separate container names to avoid conflicts
