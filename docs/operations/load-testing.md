# Load Testing Against Staging

This guide describes how to run load tests against the staging environment without
leaving fixture data behind.

## Overview

The load tester (`cmd/loadtest`) defaults to public read-only traffic. Deliberately
enabled writes create persistent fixture events. Before write testing on shared
staging, prepare and verify cleanup ownership:

1. Create a dedicated load-test source in the staging database.
2. Create an API key linked to that source.
3. Verify actual event attribution; the key alone does not establish isolation.
4. Run write traffic only with proven ownership or a disposable database.
5. Clean up verified fixtures with `server cleanup loadtest`.

## Current isolation limitation

An API-key source id is a fallback: an explicit event `source.url` takes precedence.
The current `RandomEventInput` generator supplies a random source URL for every event,
so a dedicated key alone does **not** isolate the stock load tester's writes under one
source. Verify the actual `event_sources.source_id` with a one-event probe before
relying on source-scoped cleanup. The [source cleanup retrospective](source-feed-reconciliation-retrospective.md)
records a run where 26 fixture writes created 26 independent sources and required
individually verified cleanup.

For a routine post-deployment load probe, use the supported read-only option:

```bash
./deploy/scripts/performance-test.sh --profile light --slot blue --read-ratio 1
```

Read-only is now the default for every profile and routine `test-remote.sh` run.
The public workload excludes protected `/metrics`. Unexpected authentication errors,
5xx and transport failures always return nonzero; other failures exceed the default
zero error budget. Use the Go CLI's `--json` to inspect endpoint/status distributions.
The wrapper forwards `--max-error-rate` for deliberate non-authentication 4xx budgets.
Write testing needs verified run isolation and cleanup ownership, or a disposable
database, before sustained traffic.

An explicit `--read-ratio` below 1 requires credentials. Before sustained traffic,
each key must pass a harmless authenticated malformed-JSON request (expected HTTP400
from the create handler before ingest). This preflight creates no event; it does not
establish fixture ownership or guarantee subsequent event validation will succeed.
Smoke checks also default to no event writes: `SMOKE_WRITE_TESTS=true` is an explicit
opt-in requiring `ALLOW_DESTRUCTIVE`, an API key, and a non-production environment.

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
reads this field and calls `WithSourceID(...)` as a fallback. An explicit input
`source.url` overrides it; verify attribution as described above before write testing.

### 3. Configure the load tester to use the key

Pass the key to the Go load tester:

```go
tester = tester.WithAPIKeys([]string{key})
```

Or set the key in your environment and reference it in the shell wrapper:

```bash
export LOADTEST_API_KEY=<key>
./deploy/scripts/performance-test.sh --profile medium --slot blue --read-ratio 0.8 --api-key "$LOADTEST_API_KEY"
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
