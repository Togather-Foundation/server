# Testing Guide

**Version:** 0.1.0  
**Date:** 2026-01-27  
**Status:** Living Document

This document provides testing guidelines and workflows for the Togather SEL backend. It covers test-driven development (TDD), test types, coverage requirements, and best practices for contributors.

For architecture context, see [architecture.md](architecture.md). For database testing, see [database.md](database.md).

---

## Table of Contents

1. [Testing Philosophy](#testing-philosophy)
2. [Test Types](#test-types)
3. [TDD Workflow](#tdd-workflow)
4. [Writing Tests](#writing-tests)
5. [Integration Testing](#integration-testing)
6. [Test Organization](#test-organization)
7. [Coverage Requirements](#coverage-requirements)
8. [Running Tests](#running-tests)
9. [Debugging Tests](#debugging-tests)
10. [CI/CD Integration](#cicd-integration)

---

## Testing Philosophy

SEL follows a **test-driven development (TDD)** approach with these principles:

### Integration Over Isolation

Test with real dependencies in real environments. Use **testcontainers** with actual PostgreSQL instances, real job queues, and real HTTP clients. Avoid mocking except when necessary (external APIs, slow operations).

**Why?** Integration tests catch problems that unit tests miss:
- Database constraint violations
- Transaction isolation issues
- JSON-LD serialization errors
- Timezone handling bugs

### Coverage as Quality Metric

Target **80%+ code coverage** with meaningful tests that:
- Test business logic, not trivial getters/setters
- Cover error paths, not just happy paths
- Validate edge cases and boundary conditions
- Ensure API contracts are respected

**Not just a number**: Coverage reports guide where tests are missing, not an end goal.

### Test First, Code Second

When adding features or fixing bugs:

1. **Write failing test** that describes expected behavior
2. **Implement feature** to make test pass
3. **Refactor** with confidence (tests prevent regressions)

This workflow ensures:
- Clear requirements (tests document expected behavior)
- No untested code paths
- Regression protection

### Observability in Tests

Tests should be **debuggable**:
- Clear test names that describe what's being tested
- Helpful error messages when assertions fail
- Structured logging enabled in test runs
- Transaction logs preserved on failure

---

## Test Types

### Unit Tests

**Purpose**: Test individual functions or methods in isolation

**Characteristics**:
- Fast (< 10ms per test)
- No external dependencies (database, network, filesystem)
- Focused on business logic

**Example: Validator Test**

```go
func TestValidateEventInput(t *testing.T) {
    tests := []struct {
        name    string
        input   CreateEventInput
        wantErr bool
    }{
        {
            name: "valid input",
            input: CreateEventInput{
                Name:      "Jazz Night",
                StartDate: "2025-08-15T19:00:00-04:00",
                Location:  Location{Name: "Centennial Park"},
            },
            wantErr: false,
        },
        {
            name: "missing name",
            input: CreateEventInput{
                StartDate: "2025-08-15T19:00:00-04:00",
                Location:  Location{Name: "Centennial Park"},
            },
            wantErr: true,
        },
        {
            name: "invalid date format",
            input: CreateEventInput{
                Name:      "Jazz Night",
                StartDate: "not-a-date",
                Location:  Location{Name: "Centennial Park"},
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateEventInput(tt.input)
            if tt.wantErr {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Integration Tests

**Purpose**: Test components working together with real dependencies

**Characteristics**:
- Use testcontainers for Postgres (real database, real extensions)
- Test full HTTP request/response cycles
- Validate database state after operations
- Test transactions and concurrency

**Example: API Integration Test**

```go
func TestCreateEvent_Integration(t *testing.T) {
    env := setupTestEnv(t)
    
    // Create API key
    key := insertAPIKey(t, env, "agent-test")
    
    // Prepare request
    payload := map[string]any{
        "name":      "Jazz Night",
        "startDate": "2025-08-15T19:00:00-04:00",
        "location": map[string]any{
            "name":            "Centennial Park",
            "addressLocality": "Toronto",
        },
    }
    
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequest(http.MethodPost, env.Server.URL+"/api/v1/events", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+key)
    req.Header.Set("Content-Type", "application/ld+json")
    
    // Send request
    resp, _ := env.Server.Client().Do(req)
    defer resp.Body.Close()
    
    // Assert response
    require.Equal(t, http.StatusCreated, resp.StatusCode)
    
    // Parse response
    var created map[string]any
    json.NewDecoder(resp.Body).Decode(&created)
    require.Equal(t, "Jazz Night", created["name"])
    
    // Verify database state
    var dbName string
    err := env.Pool.QueryRow(context.Background(), 
        "SELECT name FROM events WHERE ulid = $1", 
        extractULID(created["@id"])).Scan(&dbName)
    require.NoError(t, err)
    require.Equal(t, "Jazz Night", dbName)
}
```

### End-to-End Tests

**Purpose**: Test complete workflows from API to database to federation

**Characteristics**:
- Multi-step operations (create → update → delete)
- Cross-component interactions (events → places → organizations)
- Federation scenarios (sync between nodes)
- Background job execution (reconciliation, vector indexing)

**Example: E2E Federation Test**

```go
func TestFederationSync_E2E(t *testing.T) {
    // Setup two SEL nodes
    node1 := setupTestEnv(t)
    node2 := setupTestEnv(t)
    
    // Node 1: Create event
    event := createTestEvent(t, node1, "Jazz Night")
    
    // Node 1: Add to federation nodes registry
    registerPeerNode(t, node1, node2.Config.NodeDomain)
    
    // Node 2: Sync from Node 1
    syncFromPeer(t, node2, node1.Config.NodeDomain)
    
    // Node 2: Verify event exists with preserved origin URI
    fetchedEvent := getEventByURI(t, node2, event["@id"])
    require.Equal(t, event["@id"], fetchedEvent["@id"])
    require.Equal(t, node1.Config.NodeDomain, extractNodeDomain(fetchedEvent["@id"]))
}
```

### Contract Tests

**Purpose**: Validate API responses conform to schema.org and SEL Interoperability Profile

**Characteristics**:
- JSON-LD validation
- SHACL shape validation (against Artsdata shapes)
- OpenAPI schema validation
- External identifier format validation

**Example: Schema.org Conformance Test**

```go
func TestEventOutput_SchemaOrgConformance(t *testing.T) {
    env := setupTestEnv(t)
    event := createTestEvent(t, env, "Jazz Night")
    
    // Fetch as JSON-LD
    resp := getEventJSONLD(t, env, event["@id"])
    
    // Validate required schema.org properties
    require.Equal(t, "Event", resp["@type"])
    require.NotEmpty(t, resp["@id"], "@id is required")
    require.NotEmpty(t, resp["name"], "name is required")
    require.NotEmpty(t, resp["startDate"], "startDate is required")
    require.NotEmpty(t, resp["location"], "location is required")
    
    // Validate location is a Place
    location := resp["location"].(map[string]any)
    require.Equal(t, "Place", location["@type"])
    require.NotEmpty(t, location["name"])
}
```

---

## TDD Workflow

### Step 1: Write Failing Test

Before implementing a feature, write a test that describes the expected behavior.

**Example: Adding Event Series Support**

```go
func TestCreateEventSeries(t *testing.T) {
    env := setupTestEnv(t)
    
    payload := map[string]any{
        "@type":           "EventSeries",
        "name":            "Weekly Jazz Jam",
        "startDate":       "2025-08-01",
        "endDate":         "2025-12-31",
        "eventSchedule": map[string]any{
            "@type":      "Schedule",
            "byDay":      "Friday",
            "startTime":  "19:00:00",
            "endTime":    "22:00:00",
        },
    }
    
    // This will fail because feature not implemented yet
    resp := createEventSeries(t, env, payload)
    require.Equal(t, http.StatusCreated, resp.StatusCode)
}
```

**Run Test**: `make test` → Test fails (as expected)

### Step 2: Implement Feature

Write minimal code to make the test pass.

**Implementation: Add Event Series Handler**

```go
func (h *EventHandler) CreateEventSeries(w http.ResponseWriter, r *http.Request) {
    var input CreateEventSeriesInput
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        writeError(w, http.StatusBadRequest, "invalid JSON")
        return
    }
    
    // Validate input
    if err := ValidateEventSeriesInput(input); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    
    // Create series
    series, err := h.repo.CreateEventSeries(r.Context(), input)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "failed to create series")
        return
    }
    
    // Generate occurrences
    if err := h.generateOccurrences(r.Context(), series); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to generate occurrences")
        return
    }
    
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(series)
}
```

**Run Test**: `make test` → Test passes

### Step 3: Add Edge Cases

Add tests for error conditions and edge cases.

```go
func TestCreateEventSeries_InvalidSchedule(t *testing.T) {
    env := setupTestEnv(t)
    
    payload := map[string]any{
        "@type":      "EventSeries",
        "name":       "Weekly Jazz Jam",
        "eventSchedule": map[string]any{
            "byDay": "InvalidDay", // Invalid day
        },
    }
    
    resp := createEventSeries(t, env, payload)
    require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateEventSeries_EndBeforeStart(t *testing.T) {
    env := setupTestEnv(t)
    
    payload := map[string]any{
        "@type":     "EventSeries",
        "name":      "Weekly Jazz Jam",
        "startDate": "2025-12-31",
        "endDate":   "2025-08-01", // End before start
    }
    
    resp := createEventSeries(t, env, payload)
    require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
```

### Step 4: Refactor

Refactor implementation with confidence (tests prevent regressions).

```go
// Extract validation into separate function
func ValidateEventSeriesInput(input CreateEventSeriesInput) error {
    if input.Name == "" {
        return errors.New("name is required")
    }
    
    if input.EventSchedule.ByDay != "" {
        validDays := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
        if !contains(validDays, input.EventSchedule.ByDay) {
            return errors.New("invalid byDay value")
        }
    }
    
    if input.StartDate.After(input.EndDate) {
        return errors.New("endDate must be after startDate")
    }
    
    return nil
}
```

**Run Tests**: `make test` → All tests pass

---

## Writing Tests

### Test Naming Conventions

Use descriptive names that explain what's being tested:

**Pattern**: `Test<FunctionName>_<Scenario>`

```go
// Good
func TestCreateEvent_HappyPath(t *testing.T)
func TestCreateEvent_MissingRequiredFields(t *testing.T)
func TestCreateEvent_InvalidDateFormat(t *testing.T)
func TestCreateEvent_DuplicateDetection(t *testing.T)

// Bad
func TestCreateEvent1(t *testing.T)
func TestCreateEvent2(t *testing.T)
func TestEventCreation(t *testing.T)
```

### Table-Driven Tests

Use table-driven tests for multiple similar scenarios:

```go
func TestNormalizeName(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "lowercase",
            input:    "jazz night",
            expected: "Jazz Night",
        },
        {
            name:     "uppercase",
            input:    "JAZZ NIGHT",
            expected: "Jazz Night",
        },
        {
            name:     "mixed case",
            input:    "JaZz NiGhT",
            expected: "Jazz Night",
        },
        {
            name:     "extra whitespace",
            input:    "  Jazz   Night  ",
            expected: "Jazz Night",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := NormalizeName(tt.input)
            require.Equal(t, tt.expected, result)
        })
    }
}
```

### Assertion Best Practices

**Use testify/require for clear errors**:

```go
// Good: Clear error message
require.Equal(t, "Jazz Night", event.Name, "event name should match input")

// Better: testify provides helpful diff
require.Equal(t, expected, actual)

// Bad: Generic error
if event.Name != "Jazz Night" {
    t.Fatal("name mismatch")
}
```

**Test error conditions explicitly**:

```go
// Good
event, err := repo.CreateEvent(ctx, input)
if err != nil {
    require.ErrorContains(t, err, "duplicate event")
} else {
    require.NotEmpty(t, event.ID)
}

// Bad: Ignoring errors
event, _ := repo.CreateEvent(ctx, input)
```

### Test Helpers

Extract common setup into helper functions:

```go
// helpers_test.go
func createTestVenue(t *testing.T, env *testEnv, name string) string {
    t.Helper()
    
    payload := map[string]any{
        "@type":           "Place",
        "name":            name,
        "addressLocality": "Toronto",
        "addressRegion":   "ON",
    }
    
    resp := postJSON(t, env, "/api/v1/places", payload)
    require.Equal(t, http.StatusCreated, resp.StatusCode)
    
    var place map[string]any
    json.NewDecoder(resp.Body).Decode(&place)
    return place["@id"].(string)
}

func createTestEvent(t *testing.T, env *testEnv, name string) map[string]any {
    t.Helper()
    
    venueURI := createTestVenue(t, env, "Test Venue")
    
    payload := map[string]any{
        "name":      name,
        "startDate": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
        "location":  map[string]any{"@id": venueURI},
    }
    
    resp := postJSON(t, env, "/api/v1/events", payload)
    require.Equal(t, http.StatusCreated, resp.StatusCode)
    
    var event map[string]any
    json.NewDecoder(resp.Body).Decode(&event)
    return event
}
```

**Usage**:

```go
func TestEventWithVenue(t *testing.T) {
    env := setupTestEnv(t)
    event := createTestEvent(t, env, "Jazz Night")
    require.NotEmpty(t, event["@id"])
}
```

---

## Integration Testing

### Test Environment Setup

**Shared Container Pattern**: Reuse Postgres container across tests for speed.

```go
// helpers_test.go
var (
    sharedOnce      sync.Once
    sharedContainer *tcpostgres.PostgresContainer
    sharedPool      *pgxpool.Pool
)

func setupTestEnv(t *testing.T) *testEnv {
    t.Helper()
    
    // Initialize shared container once
    initShared(t)
    
    // Reset database for test isolation
    resetDatabase(t, sharedPool)
    
    // Create httptest server
    server := httptest.NewServer(api.NewRouter(testConfig(), testLogger()))
    t.Cleanup(server.Close)
    
    return &testEnv{
        Pool:   sharedPool,
        Server: server,
    }
}

func initShared(t *testing.T) {
    t.Helper()
    sharedOnce.Do(func() {
        ctx := context.Background()
        
        // Start PostgreSQL with PostGIS
        container, err := tcpostgres.Run(
            ctx,
            "postgis/postgis:16-3.4",
            tcpostgres.WithDatabase("sel_test"),
            tcpostgres.WithUsername("sel"),
            tcpostgres.WithPassword("sel_test"),
            testcontainers.WithReuseByName("togather-integration-db"),
        )
        require.NoError(t, err)
        sharedContainer = container
        
        // Run migrations
        dbURL, _ := container.ConnectionString(ctx, "sslmode=disable")
        runMigrations(t, dbURL)
        
        // Create connection pool
        pool, _ := pgxpool.New(ctx, dbURL)
        sharedPool = pool
    })
}

func resetDatabase(t *testing.T, pool *pgxpool.Pool) {
    t.Helper()
    
    ctx := context.Background()
    
    // Truncate all tables except migrations
    _, err := pool.Exec(ctx, `
        DO $$ 
        DECLARE r RECORD;
        BEGIN
            FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations')
            LOOP
                EXECUTE 'TRUNCATE TABLE ' || quote_ident(r.tablename) || ' CASCADE';
            END LOOP;
        END $$;
    `)
    require.NoError(t, err)
}
```

### Testing Database Operations

**Pattern: Verify State After Operation**

```go
func TestUpdateEvent_DatabaseState(t *testing.T) {
    env := setupTestEnv(t)
    
    // Create event
    event := createTestEvent(t, env, "Original Name")
    eventID := extractULID(event["@id"])
    
    // Update event
    updatePayload := map[string]any{
        "name": "Updated Name",
    }
    resp := putJSON(t, env, "/api/v1/events/"+eventID, updatePayload)
    require.Equal(t, http.StatusOK, resp.StatusCode)
    
    // Verify database state
    var dbName string
    err := env.Pool.QueryRow(context.Background(),
        "SELECT name FROM events WHERE ulid = $1", eventID).Scan(&dbName)
    require.NoError(t, err)
    require.Equal(t, "Updated Name", dbName)
    
    // Verify history was recorded
    var historyCount int
    err = env.Pool.QueryRow(context.Background(),
        "SELECT COUNT(*) FROM event_history WHERE event_id = (SELECT id FROM events WHERE ulid = $1)", 
        eventID).Scan(&historyCount)
    require.NoError(t, err)
    require.Equal(t, 1, historyCount, "should record one history entry")
}
```

### Testing Transactions

**Pattern: Verify Rollback on Error**

```go
func TestCreateEventWithOccurrence_TransactionRollback(t *testing.T) {
    env := setupTestEnv(t)
    
    // Create event with invalid occurrence (should rollback entire transaction)
    payload := map[string]any{
        "name":      "Jazz Night",
        "startDate": "2025-08-15T19:00:00-04:00",
        "endDate":   "2025-08-15T18:00:00-04:00", // End before start (invalid)
        "location":  map[string]any{"name": "Centennial Park"},
    }
    
    resp := postJSON(t, env, "/api/v1/events", payload)
    require.Equal(t, http.StatusBadRequest, resp.StatusCode)
    
    // Verify no event was created (transaction rolled back)
    var count int
    err := env.Pool.QueryRow(context.Background(),
        "SELECT COUNT(*) FROM events WHERE name = $1", "Jazz Night").Scan(&count)
    require.NoError(t, err)
    require.Equal(t, 0, count, "event should not exist after failed transaction")
}
```

### Testing Concurrency

**Pattern: Parallel Operations with Race Detector**

```go
func TestConcurrentEventCreation(t *testing.T) {
    env := setupTestEnv(t)
    
    const numGoroutines = 10
    var wg sync.WaitGroup
    wg.Add(numGoroutines)
    
    for i := 0; i < numGoroutines; i++ {
        go func(n int) {
            defer wg.Done()
            
            event := createTestEvent(t, env, fmt.Sprintf("Event %d", n))
            require.NotEmpty(t, event["@id"])
        }(i)
    }
    
    wg.Wait()
    
    // Verify all events created
    var count int
    err := env.Pool.QueryRow(context.Background(),
        "SELECT COUNT(*) FROM events").Scan(&count)
    require.NoError(t, err)
    require.Equal(t, numGoroutines, count)
}
```

**Run with Race Detector**: `make test-race`

---

## Test Organization

### Directory Structure

```
tests/
├── integration/        # Integration tests (HTTP + DB)
│   ├── events_test.go
│   ├── places_test.go
│   ├── organizations_test.go
│   ├── federation_test.go
│   ├── provenance_test.go
│   └── helpers_test.go
├── e2e/               # End-to-end tests (future)
│   └── federation_sync_test.go
└── fixtures/          # Test data (JSON-LD, SHACL shapes)
    ├── events.jsonld
    └── places.jsonld

internal/
└── domain/
    └── events/
        ├── service.go
        └── service_test.go  # Unit tests next to code
```

### File Naming

- Unit tests: `<filename>_test.go` (next to code)
- Integration tests: `<feature>_test.go` in `tests/integration/`
- Helpers: `helpers_test.go` (shared across test files in package)

---

## Coverage Requirements

### Target Coverage

**Overall Target**: 80%+ code coverage

**Current CI Threshold**: 35% minimum (see `.github/workflows/ci.yml`)

**Component Targets**:
- Core domain logic: 90%+
- API handlers: 85%+
- Repository layer: 80%+
- Utilities: 70%+

**Excluded from Coverage**:
- Generated code (SQLc, Huma)
- Main function and config loading
- Logging statements

### Measuring Coverage

**Generate Coverage Report**:

```bash
make coverage
```

This runs:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

`make coverage` also runs `make coverage-check`, which enforces the current CI threshold.

**View in Browser**: Open `coverage.html`

**Check Coverage Threshold** (CI):

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//' | awk '{if ($1 < 35) exit 1}'
```

Fails if coverage < 35%.

### Coverage Best Practices

1. **Focus on Business Logic**: Don't chase 100% coverage on trivial code
2. **Test Error Paths**: Error handling is critical, ensure it's covered
3. **Cover Edge Cases**: Boundary conditions, empty inputs, nil values
4. **Exclude Generated Code**: Use `//go:generate` comments to exclude

---

## Running Tests

### Command Reference

```bash
# Run all tests
make test

# Run tests with verbose output
make test-v

# Run tests with race detector
make test-race

# Run specific test
go test -v ./tests/integration -run TestCreateEvent

# Run tests in specific package
go test -v ./internal/domain/events

# Run tests with coverage
make coverage

# Run tests with short flag (skip slow tests)
go test -short ./...
```

### Test Flags

- `-race`: Enable race detector (detects data races)
- `-count=1`: Disable test caching (use when you need a clean rerun)
- `-v`: Verbose output (show test names)
- `-short`: Skip slow tests (annotated with `if testing.Short() { t.Skip() }`)
- `-timeout=5m`: Set global timeout for all tests

---

## Debugging Tests

### Print Debug Info

```go
func TestDebugExample(t *testing.T) {
    env := setupTestEnv(t)
    
    event := createTestEvent(t, env, "Jazz Night")
    
    // Print JSON for inspection
    eventJSON, _ := json.MarshalIndent(event, "", "  ")
    t.Logf("Created event: %s", eventJSON)
    
    // Query database state
    var dbEvent struct {
        ID          string
        Name        string
        CreatedAt   time.Time
    }
    err := env.Pool.QueryRow(context.Background(),
        "SELECT ulid, name, created_at FROM events WHERE ulid = $1",
        extractULID(event["@id"])).Scan(&dbEvent.ID, &dbEvent.Name, &dbEvent.CreatedAt)
    require.NoError(t, err)
    
    t.Logf("DB state: %+v", dbEvent)
}
```

**Run with Verbose**: `go test -v -run TestDebugExample`

### Enable Structured Logging in Tests

```go
func testLogger() zerolog.Logger {
    // Pretty console output for debugging
    return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
        With().Timestamp().Logger()
}
```

### Run Single Test with Debug

```bash
# Run specific test with verbose output and race detector
go test -v -race -run TestCreateEvent_HappyPath ./tests/integration

# With debug logging
LOG_LEVEL=debug go test -v -run TestCreateEvent_HappyPath ./tests/integration
```

### Inspect Database State

```go
func dumpDatabase(t *testing.T, pool *pgxpool.Pool) {
    t.Helper()
    
    rows, _ := pool.Query(context.Background(), "SELECT ulid, name, lifecycle_state FROM events ORDER BY created_at")
    defer rows.Close()
    
    t.Log("Current database state:")
    for rows.Next() {
        var ulid, name, state string
        rows.Scan(&ulid, &name, &state)
        t.Logf("  %s: %s (%s)", ulid, name, state)
    }
}
```

---

## CI/CD Integration

### GitHub Actions Workflow

See [GitHub workflows](../../.github/workflows/) for details. *Always* keep the external CI consistent with local Makefil based `make ci`.

### CI Checklist

SEL CI pipeline validates:

1. **Unit Tests**: All unit tests pass
2. **Integration Tests**: All integration tests pass with race detector
3. **Coverage**: ≥35% code coverage (current CI threshold)
4. **Linting**: Code passes golangci-lint
5. **SHACL Validation**: JSON-LD exports conform to Artsdata shapes
6. **Build**: `make build` succeeds
7. **Migrations**: Migrations apply and rollback cleanly

**Run Full CI Locally**:

```bash
make ci
```

This runs all CI checks locally before pushing.

---

## Best Practices Summary

### Do's

- ✅ Write tests first (TDD workflow)
- ✅ Use testcontainers for real database tests
- ✅ Use table-driven tests for multiple scenarios
- ✅ Test error paths explicitly
- ✅ Use meaningful test names
- ✅ Create reusable test helpers
- ✅ Run tests with race detector (`-race` flag)
- ✅ Aim for 80%+ coverage
- ✅ Test transactions and rollbacks
- ✅ Verify database state after operations

### Don'ts

- ❌ Don't mock the database (use testcontainers)
- ❌ Don't ignore test failures in CI
- ❌ Don't write tests without assertions
- ❌ Don't skip cleanup (`t.Cleanup()`)
- ❌ Don't hardcode test data (use helpers)
- ❌ Don't test implementation details (test behavior)
- ❌ Don't commit debug print statements
- ❌ Don't disable race detector in CI

---

## Next Steps

For more details:
- [architecture.md](architecture.md) - System architecture and design patterns
- [database.md](database.md) - Database schema and query patterns
- [development.md](development.md) - Development workflow and standards

**Document Version**: 0.1.0  
**Last Updated**: 2026-01-27  
**Maintenance**: Update when testing practices evolve or new patterns emerge
