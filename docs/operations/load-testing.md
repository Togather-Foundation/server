# Load Testing Against Staging

This guide describes how to run load tests against the staging environment without
leaving fixture data behind.

## Overview

The load tester (`cmd/loadtest`) generates write traffic using fixture events. Without
source tagging these events accumulate in staging and pollute queries. The recommended
workflow is:

1. Create a dedicated load-test source in the staging database.
2. Create an API key linked to that source.
3. Run the load test with that key.
4. Clean up after the run with `server cleanup loadtest`.

## Setup (one-time per environment)

### 1. Create a load-test source record

```bash
# Run on staging (or via psql tunnel)
psql "$DATABASE_URL" -c "
  INSERT INTO sources (name, source_type, trust_level, base_url)
  VALUES ('Load Tester', 'api', 1, 'https://loadtest.internal')
  RETURNING id;
"
# Note the returned UUID — this is your <source-uuid>.
```

### 2. Create a load-test API key linked to that source

```bash
server api-key create loadtest --source-id <source-uuid>
# Save the printed key — it is shown only once.
```

The `--source-id` flag stores the UUID on the `api_keys` row. The ingest handler
reads this field and calls `WithSourceID(...)` automatically, so every event ingested
with this key is linked to the load-test source in `event_sources`.

### 3. Configure the load tester to use the key

Pass the key to the Go load tester:

```go
tester = tester.WithAPIKeys([]string{key})
```

Or set the key in your environment and reference it in the shell wrapper:

```bash
export LOADTEST_API_KEY=<key>
./deploy/scripts/performance-test.sh --profile medium --slot blue
```

## Running a load test

```bash
./deploy/scripts/performance-test.sh --profile light --slot blue
# See docs/deploy/performance-testing.md for full profile reference
```

## Cleaning up after a run

### Preferred: source-id mode (post-tagging)

Deletes all events linked to the load-test source UUID:

```bash
# Preview (no changes)
server cleanup loadtest --env=staging --source-id=<source-uuid> --dry-run

# Execute
server cleanup loadtest --env=staging --source-id=<source-uuid> --confirm
```

### Legacy mode (pre-tagging contamination)

For events ingested before the source-tagging workflow was in place. Matches events
whose `image_url` or `public_url` contains `example.com` / `images.example.com`
placeholder patterns injected by the fixture generator:

```bash
# Preview
server cleanup loadtest --env=staging --legacy --dry-run

# Execute
server cleanup loadtest --env=staging --legacy --confirm
```

Both modes can be combined in a single command:

```bash
server cleanup loadtest --env=staging --source-id=<source-uuid> --legacy --confirm
```

## Safety guards

The cleanup command refuses to run unless all of the following are true:

| Check | Requirement |
|-------|-------------|
| `--env` flag | Must be `staging` |
| Deletion mode | At least one of `--source-id` or `--legacy` |
| Execution mode | At least one of `--dry-run` or `--confirm` |
| `DATABASE_URL` host | Must not match `PRODUCTION_DB_HOST` env var (or contain `"prod"`) |

Set `PRODUCTION_DB_HOST` to the production DB hostname in your shell profile to make the
heuristic exact:

```bash
export PRODUCTION_DB_HOST=db.prod.togather.foundation
```

## Notes

- `example.com` and `images.example.com` URLs are **blocked at ingest** (HTTP 422) in
  production and staging. Tests that need to submit fixture events with those URLs must
  use `ValidationConfig{AllowTestDomains: true}` — this field is never set via env var
  and must be set explicitly in test harnesses.
- The load-test source record and API key are **infrastructure** — create them once and
  reuse across runs. Do not delete the source record between runs.

## See Also

- [Performance Testing](../deploy/performance-testing.md) — load profiles, RPS config, metrics
- [API Keys](../deploy/api-keys.md) — key creation and management
