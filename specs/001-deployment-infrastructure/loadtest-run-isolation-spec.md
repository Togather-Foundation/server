# Specification: Load-test Run Ownership and Cleanup

**Spec**: 001-deployment-infrastructure / operational safeguards

**Date**: 2026-11-13 | **Status**: In Review

**Parent**: [Deployment plan](plan.md) | **Ticket**: `t_5f7ba8da`
**Goal**: Verify each write run's stored attribution before sustained traffic and
delete only that run's proven, unshared fixture events in one transaction.

Independent specification review approved the ownership, cleanup and job lifecycle
on 2026-11-13 after clarifying canonical source UUID proof, change-history refusal,
lock ordering, sequence bounds and the virtual-event geocoding guard.

## Context and delivery boundary

| Existing behavior | Code reference | Consequence |
|---|---|---|
| Public reads are the default; explicit writes require credentials and malformed-JSON auth preflight. | `internal/loadtest/loadtest.go:218` | Authentication is checked, ownership is not. |
| Writes call `RandomEventInput`, including random explicit source URLs and shared venue/organizer examples. | `internal/loadtest/loadtest.go:385`; `tests/fixtures/fixtures.go:263` | Key-source fallback cannot isolate a run; auxiliary entities can be affected. |
| Attribution requires a nonempty input source URL. | `internal/domain/events/ingest.go:367` | Removing the explicit URL does not establish cleanup ownership. |
| Event detail exposes stored source URL and `sel:sourceEventId`, but only for public events. | `internal/api/handlers/events.go:359,561,824` | A published probe can prove attribution without adding an endpoint; pending-review probes cannot. |
| Source-ID cleanup selects whole events and separately clears review references. | `cmd/server/cmd/cleanup_loadtest.go:199` | A source match alone does not prove exclusive ownership or atomic cleanup. |
| Registry identity is normalized feed URL. | `internal/storage/postgres/queries/sources.sql` | A run ID must remain distinct from the source UUID and cannot change identity semantics. |

Deliver one vertical slice: a private run manifest, deterministic isolated fixtures,
published-event ownership preflight, and manifest-scoped transactional cleanup.
This does not change normal ingest, API-key attribution, endpoint schemas,
source policy, fixture generators used by other tests, or the legacy cleanup modes.
One handler guard skips geocoding jobs for events without a physical venue, matching
the geocode worker's existing no-venue no-op; physical-event enrichment is preserved.
Legacy/source-ID cleanup remains an explicitly documented operator workflow; the
new run mode cannot be combined with either mode.

Entry: read-only safeguards are available; schema migrations through source feed
identity are installed. Exit: the acceptance tests below pass, docs describe failed
preflight recovery, and code receives independent review. No deployment is needed
to validate the local implementation; shared staging writes remain opt-in.

## User scenarios and acceptance

### Independent concurrent runs

**Independent test**: Run two load testers against one local HTTP/DB test server.

1. Given two fresh manifests, when both generate fixtures, then their run IDs,
   explicit source URLs, names, virtual URLs and source event IDs differ.
2. Given an API key linked to an unrelated source, when a fixture is ingested,
   then stored attribution follows the explicit run source URL; the ownership
   preflight proves this rather than trusting the key's source ID.
3. Given write mode without a writable new manifest path, when execution starts,
   then it fails before creating an event. Read-only mode needs no manifest.

### Ownership before sustained traffic

**Independent test**: Capture every request and stored attribution of a short run.

1. Given a write run, when all credentials pass the existing auth check, then a
   unique fixture is submitted for each rotating credential before workers start.
2. Given a probe returning HTTP 201, when its event detail is fetched with
   `include_provenance=true`, then exactly one source must match the manifest's
   normalized URL and the probe's complete source event ID. The returned event
   name and public URL must match the expected fixture as well.
3. Given HTTP 202, 409, missing/mismatched/extra provenance, wrong identity,
   non-200 detail, invalid JSON, redirect, transport failure or cancellation,
   then the run exits nonzero before sustained traffic. The private manifest is
   retained so any accepted probe can be previewed and cleaned up.
4. A 202 is never auto-approved. Operators can change appropriate validation
   policy or use a disposable database; an API key cannot bypass public visibility.

### Bounded atomic cleanup

**Independent test**: Seed two runs plus real and shared events and compare all IDs.

1. Given a stopped run and its valid manifest, when preview runs, then JSON output
   lists the exact source UUIDs, event UUIDs/ULIDs, attribution IDs and related row
   counts, with no writes.
2. Given exclusively owned fixtures, when confirmed cleanup succeeds, then their
   events, cascading occurrences/attributions/field provenance/reviews and owned
   change records disappear atomically; the other run and real events remain.
3. Given any ownership mismatch, foreign attribution/provenance/change history,
   external review/merge reference, operator decision, archived attribution or
   idempotency reference, then the whole operation refuses before deleting any row.
4. Given a fault after related-record deletion but before event deletion, then
   rollback preserves all records. A successful second cleanup is a no-op.
5. Source rows are retained by this slice, including their metadata and references.
   Empty run sources can subsequently use the existing preview-first prune workflow.
   Cleanup never deletes places, organizations, configured sources or API keys.

## Technical design

### Shared manifest and deterministic fixtures

Add `internal/loadtest/fixture` as a small shared package importing domain event
types only. It must not import storage, the load runner, or `tests/fixtures`.

```go
type Manifest struct {
    Version      int       `json:"version"`         // exactly 1
    RunID        string    `json:"run_id"`          // cryptographically random UUID
    TargetURL    string    `json:"target_url"`      // canonical operator-supplied base URL
    SourceURL    string    `json:"source_url"`      // explicit normalized feed URL
    AnchorTime   time.Time `json:"anchor_time"`     // UTC, second precision
    OwnershipKey string    `json:"ownership_key"`   // 32 random bytes, base64
}

func New(targetURL string, now time.Time) (Manifest, error)
func Create(path string, manifest Manifest) error // exclusive create, mode 0600
func Read(path string) (Manifest, error)          // strict schema and validation
func (m Manifest) Event(sequence uint64) (events.EventInput, error)
func (m Manifest) Verify(input events.EventInput) error
```

The canonical source URL is derived from the actual supplied target base path plus
`/_loadtest/runs/<run UUID>`; do not guess a deployment domain. This is a normal feed
URL, not a special registry key. Its source UUID is resolved independently in the DB.
No query/fragment or credentials are permitted in the target URL. Existing base paths
are preserved. Local HTTP targets remain usable by local tests.

Each deterministic input uses a positive sequence number and contains:

- A unique name including the complete run UUID and sequence, and a useful fixed
  description. No physical location, organizer, offers, series or external links.
- A virtual location and public URL beneath the run URL with the sequence in their
  path. This prevents ordinary dedup identity from colliding with real events.
- A run-local image URL beneath the same source URL; this is a fixture identity
  marker, not a promise to host an image asset. No shared image source is needed.
- A fixed future start relative to `AnchorTime` (seven days) and a one-hour end;
  event uniqueness does not depend on advancing the date by the sequence.
- Explicit CC0 license and the manifest's exact `SourceURL`.
- A source event ID `loadtest:<run UUID>:<sequence>:<MAC>`. `MAC` is the lowercase
  hex HMAC-SHA256 of the ASCII prefix through the sequence using `OwnershipKey`.
  The secret key never appears in any HTTP request or report.

`Verify` parses the complete event ID, verifies the MAC with constant-time equality,
reconstructs the expected input, and compares the complete normalized input. Reject
unknown JSON fields when decoding stored payloads. The payload stored by ingest is
validated/normalized (`create_event_core.go:88,813`), so compare through the same
`events.NormalizeEventInput` contract, not JSONB byte ordering or an original wire hash.
Include every fixture field when comparing, not merely its name or source URL.

`New` uses `crypto/rand`; no timestamp-only run identity. The manifest is immutable
and created before any HTTP mutation. An existing path is refused: no silent reuse
or resume. Sequence numbers are unique within one runner, including preflight.
No per-event append log or database migration is required. A lost HTTP response can
still be reconciled from the signed payload and exact source URL in the database.
Sequence is canonical unsigned decimal in `1..math.MaxUint64`; reject signs, zero,
overflow and leading zeros. A runner aborts before incrementing past the maximum.

### Runner and CLI contract

`cmd/loadtest` adds `--run-manifest <new-private-file>`, required when read ratio is
below 1. The wrapper forwards the same option. Paths and manifest contents are not
printed as JSON secrets; reports may include run ID/source URL and manifest path.
The runner receives the manifest through `WithFixtureRun(manifest)` and uses its
builder instead of the shared random fixture generator for every write.

After the existing per-key malformed-JSON check, create one sequence-specific
fixture per credential, require 201, parse its `@id` ULID, then GET the existing detail
path with `include_provenance=true`. Construct that path from the validated ULID;
never follow an arbitrary `@id` URL or redirect with credentials. Require the
exact ownership conditions above. A public proof shows stored URL/event-ID
attribution; it does not expose a source UUID, which cleanup resolves independently.
Count ownership probes separately in structured endpoint statistics. Their failures
are unconditional regardless of the ordinary 4xx error budget.

All POST responses during sustained writes must also be newly-created 201 responses;
202/409 responses are failed write expectations, even if a nonzero ordinary error
budget is configured. Probes are themselves fixtures and remain part of the run.
Stopping a client does not automatically delete anything.

`EventsHandler.Create` enqueues geocoding only when the created result has a
nonempty `PlaceULID`. Pure virtual fixtures therefore leave no event-geocoding jobs
that would later retry after cleanup; physical events retain enrichment. Add a
real-route integration regression and reflect this narrower enrichment behavior in the existing
create endpoint's OpenAPI description. No new route or response field is needed.

### Cleanup interface and storage contract

```text
server cleanup loadtest --env=staging --run-manifest FILE --dry-run [--json]
server cleanup loadtest --env=staging --run-manifest FILE --confirm [--json]
```

Run mode is exclusive with `--source-id` and `--legacy`. Existing production-host
and environment guards apply. The operator must stop the run and wait for in-flight
requests before cleanup; this slice does not provide a distributed run cancellation
protocol. A manifest is an ownership capability, not authority to delete arbitrary
records or an API credential. Keep it private and copy it securely when using remote
DB-side cleanup. Invalid/missing manifests never fall back to legacy selection.

Add a storage method in `internal/storage/postgres/loadtest_runs.go` using SQLc
queries from `queries/loadtest_runs.sql`; keep business validation in the shared
fixture contract. Proposed boundary:

```go
type LoadtestRunCleanupReport struct {
    RunID          string   `json:"run_id"`
    SourceIDs      []string `json:"source_ids"`
    EventIDs       []string `json:"event_ids"`
    EventULIDs     []string `json:"event_ulids"`
    AttributionIDs []string `json:"attribution_ids"`
    EventCount     int      `json:"event_count"`
    ChangeCount    int64    `json:"change_count"`
    ReviewCount    int64    `json:"review_count"`
    Applied        bool     `json:"applied"`
}

func (r *EventRepository) CleanupLoadtestRun(ctx context.Context,
    manifest fixture.Manifest, apply bool) (LoadtestRunCleanupReport, error)
```

Start one transaction. Acquire the **exclusive** registry advisory lock before table locks,
following source repair's registry-first ordering. Lock mutation tables using
`SHARE ROW EXCLUSIVE` to prevent ownership changing between validation and deletion.
Lock `events` before its child tables so an in-flight create can finish its child
inserts while cleanup waits. The fixed order is events, event_occurrences,
event_sources, field_provenance, event_changes, event_review_queue,
event_not_duplicates, idempotency_keys, event_tombstones, sources, api_keys,
scraper_sources. Honor the caller's bounded context while waiting and return an
error on timeout/contention; do not retry indefinitely. Standard
queries, including locks, belong in SQLc, not ad hoc command SQL.

Select candidates only by exact normalized source URL, then prove ownership of
every attribution with the manifest MAC and deterministic payload. Reject duplicate
or conflicting sequence mappings. Each attribution's source UUID must match the
canonical registry row whose `feed_url` equals the manifest's exact `SourceURL`;
the source URL string alone is insufficient. Require a single owned attribution and
no field provenance: normal fixture creation generates none, so even same-source
field history is outside this run's proven lifecycle. Current canonical name/public URL/
virtual URL, dates, CC0 and absence of physical venue/organizer must still match.
Reject operator-modified lifecycle/takedown/merge states or auxiliary links.
Require exactly one `event_changes` create record with null source/user IDs and an
original snapshot matching the fixture identity. Any update/delete record refuses,
even with null source/user: trigger history can represent operator edits without
those columns identifying them. The owned create record is deleted by exact event
UUID, not by run name or timestamp.

Refuse external `duplicate_of_event_id` and `merged_into_id` references rather than
editing another event/review. Refuse `event_not_duplicates` entries, idempotency rows,
tombstones and immutable repair-archive references involving candidates; these
indicate a lifecycle beyond this narrowly owned fixture. No archive is mutated.
SQL candidates include empty-registry/no-event outcomes so a second run is a no-op.

Build the exact report before mutation. Preview rolls back. Apply deletes owned
`event_changes`, then owned events by the already validated UUID list; FK cascades
remove occurrences, source attributions, field provenance and owned reviews.
Commit only after expected row counts match. Any SQL, ownership or output-report
construction failure rolls back. Source rows and metadata are never altered.

### Security, persistence and error paths

The private manifest is local runtime data, outside committed fixtures; document a
gitignored default such as `tmp/loadtest-runs/<UUID>.json`. It survives client restart
and is retained until verified cleanup; back it up securely with run evidence if
recovery matters. Loss/corruption means automated run cleanup refuses; never infer
ownership from timestamps, names, a source UUID or a URL prefix alone.

Only the capability holder can generate valid run signatures. Public provenance
reveals signatures but not the key; replayed signed fixtures retain the same feed
event identity. Altered payloads fail deterministic verification. A malicious DB
administrator remains outside the threat model. Manifest keys and API credentials
must not enter logs, ticket comments, reports or error text. Validate URL shape,
schema version, UUID, UTC anchor, exact key length and positive bounded sequence.
Use parameterized SQL and ordinary wrapped errors. No new runtime configuration or
HTTP endpoint is introduced. Update the create endpoint's OpenAPI description for
the geocoding guard; request and response schemas remain unchanged.

## Implementation task: t_5f7ba8da

**What**: Implement this complete manifest/probe/cleanup slice on the existing ticket;
do not create duplicate implementation tickets.

**Exact file scope**:

- `internal/loadtest/fixture/run.go`, `run_test.go` (new shared contract).
- `internal/loadtest/loadtest.go`, `loadtest_test.go`; `cmd/loadtest/main.go`, `main_test.go`.
- `cmd/server/cmd/cleanup_loadtest.go`, `cleanup_loadtest_test.go`.
- `internal/storage/postgres/loadtest_runs.go` (new); rollback and lifecycle
  regressions exercise the real route in `tests/integration/loadtest_run_isolation_test.go`.
- `internal/storage/postgres/queries/loadtest_runs.sql` and regenerated SQLc outputs.
- `tests/integration/loadtest_run_isolation_test.go` (new).
- `deploy/scripts/performance-test.sh`, `deploy/testing/performance-probe-behavior-test.sh`.
- `internal/api/handlers/events.go`; `docs/api/openapi.yaml` for
  the virtual-event geocoding guard and its regression/description.
- `docs/operations/load-testing.md`, `docs/deploy/performance-testing.md`.
- This spec and its deployment-plan crosslink; no migration or shared fixture rewrite.

**Test**: First prove the stock generator/key-only behavior cannot establish owned
attribution. Unit tests cover manifest validation, signature mutation, altered payloads,
sequence/run separation, fixture absence of shared entities, and exclusive private
creation. HTTP tests cover every probe refusal and every rotating key; wrapper tests
prove manifest forwarding and no write without it. Integration tests ingest two runs
and real/shared events, verify exact preview, guarded refusal, atomic rollback,
run-A-only deletion, related-row removal and second-cleanup no-op. Include an in-flight
foreign attribution writer to prove the validation/delete lock window is protected.

**Acceptance**: Wrapped targeted fixture/loadtest/CLI tests and
`go test ./tests/integration -run LoadtestRunIsolation` pass, meaningful red cases are
recorded, generated SQLc output is current, `make ci-fast` passes, docs match actual
behavior and independent spec/code review has no unresolved blocking findings.
Database verification is serialized under coordinator ownership.

## Open questions

None before review. Public-only 201 preflight and retained source rows are deliberate
limits; pending-review testing and source retention automation can be separate work.
