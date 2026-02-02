// Package integration_batch contains integration tests that require River job queue workers.
//
// This package is separated from tests/integration to optimize test execution time.
// Only batch ingestion tests require River workers to be running, which adds significant
// overhead (~6+ minutes). By isolating these tests, the main integration test suite can
// run faster without River worker startup.
//
// Tests in this package:
// - Batch event ingestion (events:batch endpoint)
// - Job queue processing and status checks
// - Deduplication and idempotency for batch operations
//
// The test helpers in this package START River workers during setupTestEnv().
// All other integration tests use helpers that do NOT start River workers.
package integration_batch
