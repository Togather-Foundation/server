# Load Testing Against Staging

The load tester defaults to public read-only traffic. Explicit write runs require
credentials and a new private run manifest so stored fixtures can be verified and
cleaned up by their owning run.

## Routine read-only probes

```bash
./deploy/scripts/performance-test.sh --profile light --slot blue --read-ratio 1
```

All profiles and routine `test-remote.sh` checks default to read-only. The workload
uses GET `/health`, `/api/v1/events`, `/api/v1/places` and `/api/v1/organizations`;
protected `/metrics` is excluded. Authentication errors, 5xx, transport failures,
redirects and cancellation fail the run. Ordinary non-authentication 4xx failures
exceed the default zero error budget; `--max-error-rate` explicitly adjusts that
budget. Use the Go CLI's `--json` for endpoint/status distributions.

Smoke checks also default to no event writes. Their separate opt-in is
`SMOKE_WRITE_TESTS=true`, requiring `ALLOW_DESTRUCTIVE`, an API key and a
non-production environment; smoke writes do not use the load tester's run manifest.

## Starting an owned write run

Choose a fresh manifest filename for every run and use a key permitted to create
events on the target. A source-linked key is optional: the fixture's explicit
`source.url` determines provenance, and the runner verifies stored attribution.

```bash
mkdir -p tmp/loadtest-runs
chmod 700 tmp/loadtest-runs

# Set LOADTEST_API_KEY through your normal private credential workflow first.
./deploy/scripts/performance-test.sh --profile light --slot blue \
  --read-ratio 0.8 --api-key "$LOADTEST_API_KEY" \
  --run-manifest tmp/loadtest-runs/run-a.json
```

The Go CLI accepts the same `--run-manifest` option. Read-only runs need no manifest.
Write mode refuses a missing manifest option, an existing file, or a file that
cannot be created. There is no silent reuse or resume. The file is created with
mode `0600` before any event POST and contains a random run UUID, the actual target
and source URLs, a time anchor and a private ownership key. **Keep the manifest
private and retain it until cleanup is verified.** Its key is a cleanup capability;
do not put the file in git, logs or tickets.

The fixture builder creates deterministic virtual events with unique run/sequence
names, public and virtual URLs, a run-local image URL, signed source event IDs and
CC0 licensing. The image URL is a fixture marker, not a hosted image asset. Inputs
contain no physical places or organizations and therefore do not create or mutate
shared venue/organizer fixtures. Pure virtual events enqueue no geocoding job.
The run UUID is separate from the database source UUID; source registry identity
remains the normalized feed URL.

## Preflight and failure recovery

Before sustained traffic, every rotating credential must pass both checks:

1. A malformed-JSON authenticated POST must return HTTP 400 before ingest. This
   checks authentication without creating an event.
2. A unique owned fixture POST must return HTTP 201. The runner then reads that
   event through the existing public detail API with `include_provenance=true` and
   requires its exact fixture identity and one matching stored source URL/event ID.

HTTP 202 pending review fails closed: public detail hides those events even when
an API key is supplied. The runner never auto-approves them. Missing, mismatched or
extra attribution, HTTP 409, unreadable detail, redirects and other unexpected
responses also stop the run before sustained traffic. A failed preflight can leave
one or more accepted probe fixtures; use the retained manifest to preview and clean
them. Sustained writes also require newly created HTTP 201 responses, so a nonzero
ordinary error budget does not waive write ownership failures.

The manifest supports cleanup after cancellation, process failure or a lost HTTP
response: ownership can be reconstructed from the stored signed payload, without
assuming that every accepted event's response reached the client. Stop the runner
and wait for in-flight requests before cleanup. Manifest loss/corruption causes
automated cleanup to refuse; it never guesses ownership from timestamps or names.

Fixture inputs still obey normal server validation. If appropriate staging policy
keeps virtual probes pending review, use a disposable database or adjust that
policy explicitly before a new run. A valid key does not guarantee publishability.

## Previewing and cleaning one run

Run cleanup where `DATABASE_URL` points at the intended staging database:

```bash
# Preview exact event/source/attribution IDs and related row counts.
server cleanup loadtest --env=staging \
  --run-manifest tmp/loadtest-runs/run-a.json --dry-run --json

# After reviewing the preview and stopping the run:
server cleanup loadtest --env=staging \
  --run-manifest tmp/loadtest-runs/run-a.json --confirm --json
```

For DB-side execution through `scripts/remote.sh`, securely copy the private
manifest into the execution container first and use that container path.
`remote.sh` does **not** copy local files or propagate local environment variables.
Discover connection details from `.deploy.conf.staging`; do not guess hosts.

Cleanup resolves the actual source UUID independently and verifies canonical feed
identity, signed deterministic payloads and current fixture state. Preview rolls
back. Apply validates and deletes in one bounded transaction with ownership locks;
contention or timeout returns an error. The transaction removes owned change-feed
create records and fixture events; FK cascades remove their occurrences,
attributions, field provenance and owned review entries. Unexpected row counts or
SQL failures roll the whole operation back. A second successful cleanup is a no-op.
Writing the final report happens after commit. If that output fails, cleanup may
already have succeeded; rerun preview with the same manifest to reconcile the result.

The entire operation refuses ambiguous ownership, additional/foreign attributions,
operator changes, any field provenance, later change history, external review/merge references,
idempotency/curator decisions, tombstones or immutable repair-archive references.
It preserves the other run and real events. **All source rows and their metadata
remain intact**, including configured sources and API-key references. No places or
organizations are deleted. Empty run sources may later use the existing
preview-first `source prune-empty` workflow.

Run mode is exclusive with `--source-id` and `--legacy`. Never switch to those
broader modes to work around a run-ownership refusal.

## Older source-ID and legacy cleanup

These modes remain for independently investigated older contamination:

```bash
server cleanup loadtest --env=staging --source-id=<verified-source-uuid> --dry-run
server cleanup loadtest --env=staging --legacy --dry-run
```

After verifying the exact targets, replacing `--dry-run` with `--confirm` executes
deletion. Source-ID mode deletes whole events attributed to that source, including
events with additional attributions. Legacy mode matches broad historical
`example.com`/`images.example.com` URL patterns. These modes provide neither the new
run-ownership proof nor its atomic lifecycle guarantee; their previews show counts
rather than the run mode's exact ownership inventory.

An API-key source ID is only a fallback: an explicit event `source.url` overrides
it. The older random generator supplied a different source URL for each event.
The [cleanup retrospective](source-feed-reconciliation-retrospective.md) records
26 fixture writes that created 26 sources despite a dedicated key. Clearing the
source URL is also insufficient because attribution requires a nonempty URL.

## Environment guards

Cleanup requires `--env=staging`, a deletion mode and explicit `--dry-run` or
`--confirm`. It refuses a database host matching `PRODUCTION_DB_HOST`; without that
setting it conservatively rejects hosts containing `prod`. Set the variable to
the known production hostname through your private environment configuration.
These environment guards supplement the run proof; they do not establish ownership.

Reserved test domains such as `example.com` remain blocked by normal staging ingest.
The run builder derives its URLs from the actual target instead of those domains.
Test harnesses that intentionally need reserved domains must explicitly configure
`ValidationConfig{AllowTestDomains: true}`; there is no environment-variable bypass.

## See Also

- [Performance Testing](../deploy/performance-testing.md) — profiles, RPS and result metrics
- [API Keys](../deploy/api-keys.md) — credential creation and management
- [Run ownership specification](../../specs/001-deployment-infrastructure/loadtest-run-isolation-spec.md)
