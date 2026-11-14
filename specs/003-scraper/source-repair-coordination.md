# Decision: offline source reconciliation

**Spec**: 003-scraper | **Date**: 2026-11-13 | **Status**: In Review
**Parent**: [Scraper plan](plan.md)
**Tracking**: `togather` ticket `t_10d9a4b0`
**Goal**: Prevent a delayed writer from undoing a rare source repair by keeping all
application writers stopped until repair and verification finish.

## Decision and scope

On 2026-11-13 the user clarified: “yeah if we ahve to stop everything for the rare
case thats totally fine”. Use an offline maintenance window. Update the binaries,
stop and drain **all writers**, keep PostgreSQL running, perform the repair with a
standalone CLI, verify the result, then restart the updated application.

This supersedes the proposed online transaction guard. No new writer coordination
code, private repository flag, transaction/consolidation refactor, database trigger,
maintenance controller, or compatibility machinery is required for this decision.
The safety boundary is the operational absence of concurrent writers, not a new
automatic CLI guarantee. This document does not authorize a deployment or repair.

## Existing capabilities and limitation

| Capability | Verified implementation |
|---|---|
| Source repair | `cmd/server/cmd/source_repair.go` provides `source reconcile`, preview by default, `--confirm` to apply, `--json-report`, and `--sources` for the complete YAML inventory. It connects directly to PostgreSQL with a five-minute context. |
| Repair transaction | `internal/storage/postgres/source_repair.go` takes the exclusive registry advisory lock and fixed repair table locks. Preview rolls back the same transaction. |
| Audit | `cmd/server/cmd/source.go` provides `source audit --json` using direct database access. |
| Verified backup | `deploy/scripts/snapshot-db.sh` takes an explicit DB container and trusted `DATABASE_URL`, creates a private custom archive, and records checksum/size/full-read evidence. |
| Application slots | `deploy/docker/docker-compose.blue-green.yml` defines `togather-blue` and `togather-green`; both can access the shared database regardless of traffic routing. |
| Remote wrapper | `scripts/remote.sh` discovers a **running** server container and uses `docker exec`; it cannot run repair after all server containers stop. |

The existing locks do not invalidate a UUID resolved by an earlier request.
`internal/domain/events/create_event_core.go` resolves a source before its new-event
transaction; `internal/storage/postgres/events_repository.go:CreateSource` accepts
that UUID. A delayed writer can therefore recreate a noncanonical attribution
after repair. Stopping and draining every writer removes that interleaving.

## Maintenance prerequisites

- Read the environment's `.deploy.conf.{environment}` and deployed configuration;
  never guess hosts, database addresses, container names, or deployment paths.
- Prepare the updated application binary/image and a **standalone repair CLI** with
  the complete intended `configs/sources` inventory before the outage. The CLI must
  reach the same database using trusted `DATABASE_URL`; protect credentials and
  retain command reports privately. Do not start `server serve` to obtain a CLI.
- A host-compatible standalone binary is sufficient. A separately prepared one-shot
  CLI container is also possible, but its image, network, credentials and config
  mounts must be verified for that environment before stopping the application.
  There is no new one-shot wrapper in this change.
- Confirm the backup/rollback tools work while app containers are stopped. The
  database and its backup client remain available; do not use a whole-stack teardown
  that also stops PostgreSQL. See the [backup runbook](../../docs/deploy/remote-deployment.md#snapshot-tooling-and-database-container-fallback).

## Procedure

1. Begin the maintenance window and block new submissions/scrapes using the actual
   deployment controls. Stop active **and inactive** server slots, embedded workers,
   standalone schedulers/job workers, external scrape schedules and CLI writers.
   Drain in-flight work; verify the processes have exited and no application writer
   remains connected or can automatically restart. A traffic switch alone is not
   sufficient. Keep writers stopped throughout backup, preview, apply and audit.
2. Keep PostgreSQL up. Create a fresh verified backup against the exact repair
   database using the existing snapshot helper. Retain the archive and adjacent
   `.dump.json` evidence. Full `pg_restore` reading verifies archive readability;
   it is not a restore rehearsal. A failed dump/read/checksum gate means no repair.
3. From the prepared standalone CLI, with trusted `DATABASE_URL` and the complete
   YAML inventory, run the existing commands below. Here `server` means that
   standalone executable, **not** `scripts/remote.sh` or exec into a stopped slot.
   Run from the directory containing `configs/sources`, or supply the verified
   inventory path with `--sources` on each command.

   ```bash
   server source audit --json
   server source reconcile --json-report
   # Inspect the preview and policy/collision issues before applying.
   server source reconcile --json-report --confirm
   server source reconcile --json-report
   server source audit --json
   ```

4. Before restarting anything, require the second preview to propose no further
   moves, archives or trust changes and check audit/report counts against the
   expected result. Preserve live-plus-archived provenance counts and investigate
   retained policy/history collisions; a zero exit code alone is not acceptance.
   Trust changes require an explicit operator decision and the existing
   `--use-config-trust` preview; do not implicitly overwrite trust or licenses.
5. Restart only updated application binaries and resume jobs/submissions after
   verification passes. Confirm service health and repeat the read-only audit after
   resume. Retain the backup and reports under existing operational retention rules.

If draining cannot be established, do not apply. If preview, apply or verification
fails, keep writers stopped, retain the evidence, and investigate or use the
[rollback runbook](../../docs/deploy/rollback.md). Do not blindly retry an apply whose
commit outcome is uncertain: inspect preview/audit first. Never discard provenance
to make the result appear clean.

## Acceptance and deferred alternative

The operational acceptance is a verified maintenance window: all application
writers stopped and drained, a usable backup, standalone CLI access with the app
down, inspected preview, successful apply, no-op preview and audited conservation
**before** restart, followed by healthy updated writers. No maintenance operation
was run and no runtime behavior changed by this documentation work; document review
must not be reported as a completed deployment rehearsal or live repair.

Online repair remains deferred unless outages become unacceptable or repairs become
frequent. At that point, revisit fresh canonical resolution under a transaction-wide
registry guard, including existing-event merges and caller-owned consolidation.
The current CLI does not automatically prove pause/drain or reject an unsupported
writer; those facts remain explicit operational prerequisites.
