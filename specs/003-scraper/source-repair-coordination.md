# Design: source repair and delayed attribution writers

**Spec**: 003-scraper | **Date**: 2026-11-13 | **Status**: Draft
**Parent**: [Scraper plan](plan.md)
**Tracking**: `togather` ticket `t_10d9a4b0`
**Goal**: A supported writer cannot commit a noncanonical attribution after a
successful reconciliation by reusing a source UUID resolved before that repair.

This document is a proposal, not an implemented guarantee. This wave delivers the
design only; the P1 ticket's concurrency tests and implementation remain outstanding.
Until implementation and verification, retain the pause, drain, stop-old-slots,
repair, and re-audit procedure in [the scraper guide](../../docs/integration/scraper.md#reconciling-historical-source-rows).

## Current state

References describe the implementation at `11f3281f` (line numbers are orientation
aids, not a new API contract).

| Capability | Existing behavior and location |
|---|---|
| Registry serialization | `internal/storage/postgres/queries/sources.sql:6` takes a shared transaction advisory lock; `LockSourceFeed` follows it. |
| Repair serialization | `internal/storage/postgres/source_repair.go:191` takes the exclusive registry lock, then `LockSourceRepairTables`; preview uses the same locks and rolls back. |
| Early resolution | `internal/domain/events/create_event_core.go:145` resolves through `s.repo`, before the new-event transaction begins at line 764. |
| Attribution insertion | `internal/storage/postgres/events_repository.go:898` inserts the supplied UUID without revalidation. A transaction-scoped repository uses its caller's transaction; a standalone call executes directly on the pool without opening an owned transaction. |
| Existing-event merge | `internal/domain/events/create_event_core.go:222` updates the event, then separately records attribution; attribution failures are logged after the merge. The dedup-hash merge has the same shape at lines 281–290. |
| Caller-owned creation | `internal/domain/events/admin_service.go:1623` starts consolidation's transaction and locks retired events before invoking `createEventCore` at line 1730. |
| Potential external I/O | `create_event_core.go:113` can invoke geographic-boundary geocoding. Acquiring a registry lock around the entire pipeline would include this call. |
| Operational coordination | `docs/integration/scraper.md:582` requires operator pause/drain and stopping old binaries. The database does not certify those conditions. |

The repair locks protect its transaction. They do not invalidate an application
variable containing an earlier UUID. For example, resolution returns legacy source
L; repair registers canonical C and moves existing attributions; a delayed writer
then inserts another attribution using L.

## Decision

Reuse the existing registry advisory lock and source resolver at the **write
transaction boundary**. Resolve the feed again on that transaction before any
repair-touched write, and retain the shared lock through commit. Do not introduce
a pause service, writer leases, generation table, heartbeat, or global status API.

This is a bounded database critical section, not a lock around fetching, validation,
geocoding, or the entire scraper job. A UUID returned before this boundary is a hint,
not authority to write provenance after repair.

```text
prepare input / external I/O / early dedup lookup
  -> begin write transaction
  -> shared registry lock, then feed lock (existing GetOrCreateSource)
  -> resolve canonical source afresh; re-read merge inputs when applicable
  -> event/occurrence/review/attribution writes on this transaction
  -> commit or rollback (releases locks)

repair -> exclusive registry lock -> existing repair table locks -> repair -> commit
```

### Why this size of change

| Alternative | Assessment |
|---|---|
| Resolve once more before an unguarded insert | Leaves another repair/insert race; insufficient. |
| Add a shared lock only inside `CreateSource` | Too late for a transaction that already wrote a repair-locked table such as `event_changes`; introduces a lock-order inversion. |
| Lock every `EventRepository.BeginTx` | Broadens contention to unrelated admin work and can cover existing network calls; reject. |
| Database trigger or new canonical foreign-key model | Could enforce against old writers, but needs migrations and a compatibility/payload-preservation design beyond this fix; defer unless old-writer rejection becomes a requirement. |
| Early guard on the existing write transaction | Uses existing lock identity and resolver; recommended, with caller coverage below. |

## Proposed contracts

No new public HTTP endpoint, environment variable, or domain repository method is
needed. Existing `Repository.GetOrCreateSource(ctx, SourceLookupParams)` already
resolves on `r.tx` when supplied a transaction-scoped repository. Its successful
feed resolution becomes the guard for the remainder of that transaction.

1. **New events:** immediately after opening the write transaction, call
   `dbRepo.GetOrCreateSource` using the normalized feed metadata. Use its returned
   UUID for attribution and the review row. Do this before creating the event,
   occurrences, series, or history. Earlier source resolution can remain for
   read-side deduplication but cannot supply the final write UUID.
2. **Existing-event merges:** use one short transaction for fresh source resolution,
   current event/trust reads, merge calculation, update, and attribution insertion.
   Cover both feed-ID and dedup-hash merge branches. On an attribution failure,
   roll back the update and return an error; do not turn a failed guarded write
   into a successful ingest with a provenance warning. Keep existing payload,
   confidence, rename, event identity, and date behavior.
3. **Standalone `CreateSource`:** open a short owned transaction, acquire the shared
   registry lock, and read canonical ownership by normalized `SourceURL` before
   inserting. Rebind a stale supplied UUID to the existing canonical row. Do not
   invent policy, relabel a row, or create a canonical source from the attribution
   payload. A missing canonical row fails explicitly without writing; normal ingest
   registers it through `GetOrCreateSource` first.
4. **Transaction-scoped `CreateSource`:** require the early registry guard. Record
   successful acquisition in private transaction-scoped repository state, rather
   than reacquire the lock after arbitrary writes. An unguarded call fails before
   insertion; the caller must roll back and retry from a guarded boundary. Within
   the guard, verify/rebind the UUID using the canonical feed lookup as above.
5. **URL-less input:** preserve existing semantics. `recordSourceWithRepo` does not
   create an attribution without `Source.URL`; this change must not fabricate a feed
   from a pre-resolved `WithSourceID` value or alter URL-less registry identities.

The private state needs only a transaction-lifetime boolean on `EventRepository`,
set when `GetOrCreateSource` successfully takes the registry lock on `r.tx`:

```go
// sourceRegistryGuarded is meaningful only when tx != nil. It is never copied
// from a pool repository or reused after commit/rollback.
sourceRegistryGuarded bool
```

Do not add a per-feed cache: canonical ownership is looked up at each attachment,
and the registry lock protects it until commit. Keep new fixed-shape storage
queries in SQLc and regenerate output. Do not duplicate feed normalization or
trust/license policy normalization in domain code.

### Caller-owned transactions: required scope adjustment

Consolidation must prepare any network-dependent input **before** starting its
transaction, take the source guard before locking retired events, and pass the
transaction's fresh UUID to the shared creation pipeline. All resolution under
that guard must use `TxRepo`, not `s.repo` on another pooled connection. The latter
can wait behind repair while its own caller holds the lock repair needs.

This requires reviewing `internal/domain/events/admin_service.go` and its
consolidation tests, plus the affected repository/domain mocks. They are outside
the original ticket's exact path list and must be added to its scope before
implementation. Do not ship a guard that silently excludes this existing writer.
The extraction of preparation from persistence should be the smallest internal
helper needed to preserve current validation/warning behavior, not a general
pipeline rewrite. Revalidate database-dependent state inside the transaction.

## Lock order, failure, and cancellation

- Writers: registry shared -> feed lock -> existing event/table/row locks. A caller
  holding an event or repair-touched table lock must not start source resolution
  through a new connection or acquire a previously missing registry guard.
- Repair: registry exclusive -> existing fixed table-lock order. Reconciliation
  may use its own resolver while holding exclusive ownership; it is not a writer
  waiting on another connection. Preserve preview/apply equivalence.
- Each network-capable preparation step completes before these locks. No external
  requests, job waits, user prompts, or HTTP response writes occur inside them.
- Use request/command contexts for every wait. The CLI already bounds reconciliation
  to five minutes (`cmd/server/cmd/source_repair.go:53`); do not add an unrelated
  timeout framework. Roll back with a bounded cleanup context if the work context
  has expired, so a canceled waiter cannot retain an owned connection/lock.
- A lock timeout or cancellation returns an error and rolls back owned writes.
  Preserve existing contention error classification; verify whether the existing
  `ErrRetryable` mapping covers the new boundary before selecting a new error type.
  No unbounded retry loop and no retry of external side effects.
- A writer that starts first commits before repair proceeds; repair sees its
  attribution. A writer that starts after repair gets the repaired canonical UUID.
  A writer carrying an older UUID is rebound or fails before any attribution write.

## Old binaries and operational status

Advisory locks coordinate only cooperating code. They cannot prevent an old slot,
an old CLI, or direct SQL from writing legacy rows after commit. A quiet database
or `pg_stat_activity` snapshot is not proof that another process cannot resume.

For this implementation, **stopping/draining unsupported binaries remains an
operational prerequisite**, not an automatically enforced or attested condition.
Do not add a flag that merely restates an operator promise. Apply output must state
that the registry transaction barrier covers supported writers and that old writers
must be stopped; it must not claim `safe-to-apply`, `paused`, or `drained` based on
lock acquisition alone. Waiting for the registry lock means cooperating guarded
transactions are still active; acquiring it excludes those transactions for the
repair's duration, not all historical or external writers.

Rollout: install the compatible writer code, stop the old blue/green slot and any
old CLI writers, drain external submission/scrape jobs using existing maintenance
controls, preview, apply, then preview/audit after supported writers resume. The
second preview must show no newly introduced noncanonical moves. A requirement
to **automatically refuse mutation until old slots are drained** needs an enforceable
deployment/DB boundary and is not satisfied by this design. Keep this limitation
visible when assessing the full P1 acceptance; do not close it as fully enforced
old-binary protection.

## Security and data lifecycle

The database is the authority for canonical ownership; a supplied source UUID or
payload is untrusted for that decision. Resolve from the validated feed URL using
parameterized SQL, preserve CC0 defaults and raw payload/history, and retain existing
policy conflict behavior. No data is discarded to make a retry succeed. Never log
payload secrets or credentials. The proposal adds no persistent table/file and
therefore no new backup, retention, or recovery lifecycle. The private guard flag
lives only as long as its transaction-scoped repository.

## Acceptance scenarios and implementation gate

These tests are a plan, not tests run in this design-only wave. Use channels and
observed PostgreSQL blocking/lock state to order connections; never rely on sleeps
to cause the race.

| Given / when | Then |
|---|---|
| Writer resolves legacy L; repair commits C; release delayed standalone attribution write | It attaches to C, retaining event ID, source URL/event ID, and payload; no new L attribution. |
| New-event ingest resolves L before its write transaction; repair commits; ingest continues | Transaction-bound refresh uses C for attribution and review metadata; the event persists once. |
| Guarded writer has not committed; repair requests exclusive registry ownership | Repair waits, then includes the writer's committed attribution; a following preview has no further move. |
| Repair owns exclusive registry lock; new writer begins guarded transaction | Writer waits without holding event/history locks, then resolves C after repair. |
| Either existing-event merge branch encounters an attribution failure | Its field update rolls back; ingest returns a failure suitable for explicit retry; original event remains. |
| Caller-owned consolidation creates an event with a feed | Preparation makes no network call after the guard; all source operations use its transaction; retirement and creation remain atomic. |
| A transaction calls `CreateSource` without early guarding | It fails before insert without acquiring a late shared lock; caller rolls back. |
| Waiting writer/repair context expires or is canceled | No partial data, no owned lock/connection leak; a subsequent independent operation succeeds. |
| Old code bypasses the advisory protocol | Test demonstrates lack of protection; docs/output require it stopped and never infer old-slot safety. |

Implementation remains in `t_10d9a4b0`; do not create a second implementation ticket
or mark it complete because this document passes review. Before coding, resolve the
scope adjustment and the old-writer acceptance decision below. Deliver the guard,
transactional caller coverage, tests, and documentation as one verifiable fix.

Verification when implemented:

```bash
scripts/agent-run.sh go test -race ./internal/storage/postgres ./internal/domain/events -run 'TestSource.*Concurrent|TestSource.*Writer|TestSource.*Repair'
scripts/agent-run.sh make ci-fast
```

Expected: all targeted cases and CI pass, no dropped events/provenance, and zero new
noncanonical moves on the final preview after delayed supported writers finish.

## Open decisions for implementation review

1. Accept operational old-slot drain as the compatibility contract, or require
   automatic old-writer refusal? The latter needs a separately reviewed enforceable
   boundary; a new boolean CLI flag is insufficient. The current ticket's old-slot
   refusal wording is stronger than the minimal cooperating-writer design.
2. Approve the necessary consolidation/mocks/test path expansion after reviewing
   the small preparation extraction. If it grows into a pipeline rewrite, revisit
   the boundary before coding rather than expanding this fix silently.
3. Confirm existing idempotency reservation behavior on a newly returned merge
   transaction error. Preserve retryability without leaving a reservation that
   permanently converts a safe retry into `ErrConflict`; add a regression if needed.
