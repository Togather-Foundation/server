# Source cleanup retrospective

Recorded 2026-11-12 UTC after completion of `t_eafc6fae`. The
[repair evidence](source-feed-reconciliation-t_eafc6fae.md) is the authoritative
measurement record, including SQL, source UUIDs, backup checksum and validation.
This retrospective explains the decisions and improvements; implementation work
continues to be tracked on the shared `togather` board.

## Outcome and confidence

The cleanup fixed source identity and attribution, rather than merely reducing row
counts. Canonical sources now resolve by normalized feed URL; names are labels and
event identity remains `(feed URL, sourceEventId)`.

| Measure | Before | Final |
|---|---:|---:|
| Fragmented feeds | 83 | 0 |
| Noncanonical attributions | 29,735 | 0 |
| Unregistered attributions | 500 | 0 |
| Live attributions | 32,183 | 23,902 |
| Archived duplicate observations | 0 | 8,281 |
| Total preserved observations | 32,183 | 32,183 |
| Source rows | 441 | 597 |

Reconciliation moved 21,652 attributions, registered/adopted the missing canonical
feed identities and archived complete duplicate attribution/source snapshots before
removing redundant live rows. Seven legacy trust values were explicitly corrected
from 5 to configured values 6–8, with matching licenses and an old/new audit report.
Licenses and scraper suppression settings were preserved.

Source count increased because previously mixed feeds acquired their own rows.
Separate cleanup removed 19 unreferenced historical orphans. The remaining 306
zero-event rows are classified: 305 configured idle rows and one metadata-protected
orphan. Zero-event does not mean disposable.

Both reconciliation and pruning produced a second preview with no changes. Final
post-rollout verification, after removing load-test fixtures, restored exactly the
reported counts and zero identity defects.

Confidence comes from several different kinds of evidence:

- The implementer measured staging preview/apply equality, observation conservation,
  backup readability, idempotence, successful config sync and final audit.
- An independent reviewer reran build, CLI tests, SQLc generation and full fast CI.
  Four new identity regression tests failed on the old base, establishing that they
  discriminate the bug. A disposable-database migration rollback/re-forward probe
  also passed.
- Staging synced all 262 configurations, served the implementation from blue and
  stopped old green. All 20 smoke checks passed.

The reviewer did not have staging SSH access and did not independently verify its
runtime counts or backup checksum. SHACL subtests were skipped on the reviewer's
machine because `pyshacl` was absent; no shape/contract files changed. Reading a
complete backup archive is useful verification, but a full restore rehearsal was
not performed. Rolling migration 61 down drops the archive: a schema rollback probe
is not proof that a live data-repair rollback preserves archived observations.

## What worked and should be repeated

**Separate registry identity from event identity.** The repair addressed source-row
fragmentation without making source UUIDs event keys again. Shared-host feeds,
query-filtered views and renamed configs could therefore reconcile without creating
new events or moving event dates.

**Keep source policy explicit.** Normal sync refused seven mismatched trust policies
before traffic switched. The explicit trust-only repair resolved those known stale
defaults while refusing license conflicts. Per-config warning suppression remained
independent of the canonical registry row.

**Archive before consolidating.** Our initial collision approach would have retained
unmoved duplicate rows. Review correctly challenged its completeness. Complete,
immutable attribution and source snapshots let the final repair consolidate live
rows while conserving every observation.

**Use preview, conservation checks and a second no-op.** These gave stronger evidence
than command exit status. Source rows, live/archived observations, trust changes,
configured idle rows, orphans and protected rows were measured separately.

**Review the deployed transition, not just the final schema.** Review caught the
initial proposal to remove an index still required by the old blue/green slot. The
retained index allowed old and new binaries to coexist during cutover.

**Give agents distinct responsibilities.** Read-only exploration, separate identity
and repair ownership, dedicated validation and independent review found different
classes of defects. The final native reviewer added base-version falsification and
rollback testing instead of only repeating the implementation's assertions.

**Persist operational state across interruptions.** A concrete handoff retained the
branch/SHA, validation state, backup reference, actual staging state and next safe
operation. Reconnecting MCP did not require repeating the cleanup or guessing what
had already run.

## What I would do differently

| Observation from this run | Better approach next time |
|---|---|
| The ticket's mechanism was partly stale: code already had a name-based lookup and attribution fallback; sync only wrote scraper configs. | Start with a short code-and-data inventory and revise the diagnosis before designing the repair. |
| Sitemap detail-page identities were recognized after the first implementation and CI run. | Inventory distinct stored feed URLs early and include listing, sitemap/detail, query-view, unknown-source and mixed-host examples in the first fixtures. |
| Backup tooling was checked during rollout. Deploy skipped its automatic snapshot because the host binary was missing; the app container lacked `pg_dump`. Migrations ran before the later manually verified backup. | Make backup capability and a verified backup the first operational gate, before either migrations or data repair. The DB-container fallback is now documented; enforcing this in automation remains future work. |
| Seven trust conflicts surfaced at deployment sync. | Compare configured and stored policies in the initial read-only inventory. Resolve intended policy explicitly before a long image build. Keep license conflicts separate from trust corrections. |
| Some validation began while workers were still editing or regenerating SQLc output. Storage test runs also collided through a shared reusable test database. | Require a stable-files handoff before CI, one owner for SQLc regeneration, and one owner for the storage test harness. Parallelize exploration and independent edits, then serialize shared-state validation. |
| The SQLc consistency gate required committed generated output; an earlier attempt stopped on that condition. | Regenerate, inspect, commit the coherent change, then run the final gate. Report exact tested SHA and treat later docs-only changes separately. |
| Several long image builds were needed before all operational prerequisites were understood. | Separate read-only preflight, image preparation, repair and traffic switch. Reuse a verified immutable image when tooling supports it; investigate build-cache/timestamp costs before changing the deployment pipeline. |
| JSON sync initially reported only an error count. | Preserve per-source actionable errors in structured output with credential/token redaction. This improvement was delivered during this work. |
| The performance wrapper exited zero with substantial authorization failures and created 26 fixture events/sources. | Use read-only load for ordinary post-deploy smoke checks. For write tests, verify credentials, accepted status/error thresholds, cleanup ownership and fixture isolation before the run. |
| A dedicated API-key source was not sufficient to isolate the current generated write payloads. | Verify actual attribution with a one-event probe. Explicit `source.url` takes precedence over the key's fallback source; the current generator supplies it. A dedicated run identity/cleanup design remains future work. |
| Orca resolved to a screen reader; MCP disconnected; the local Hermes installation had a different board; the requested reviewer profile was not installed. | Check tool identity, shared board, reviewer roster and claim state early. Use supported fallback tools, avoid creating replacement boards, and distinguish a successfully recorded claim from a failed worktree side effect. |

The biggest execution mistake was discovering operational prerequisites through failed
rollouts. The failures stopped before traffic switch and the eventual data repair
was backed up, but earlier preflight would have made the run shorter and easier to
reason about. I also should have kept CI away from active edits and treated fixture
cleanup as part of the load-test design rather than a follow-up operation.

## Remaining review concerns

The cleanup review recorded these nonblocking limitations. Follow-up status below
distinguishes implemented safeguards from remaining operational work; feature-branch
changes do not imply deployment.

| Concern | Current behavior | Improvement to consider |
|---|---|---|
| Label ownership | Follow-up `t_cf4c6269` reserves existing-row renames for explicit config sync after policy validation. Ingest initializes new labels and preserves existing and adopted labels, including `unknown`. | Same-policy configs sharing a feed retain sync-order naming; choose the intended config label when syncing. |
| In-flight writers | Locks protect the repair transaction, but a request that resolved an old source earlier can insert a legacy attribution after repair commits. Old-slot code can also mint host rows until stopped. | Quiesce and drain writers before repair; verify after the old slot stops. A stronger maintenance/barrier mechanism would need a separate design. |
| Raw URL variants | Distinct stored trailing-slash spellings can survive as multiple observations on the same canonical source. Zero fragmentation does not establish that these variants are absent. | Measure their frequency first. If consolidating them, preserve complete originals in the archive and do not rewrite stored provenance URLs casually. |
| Report precision | Ambiguous attribution choices produce history warnings even when no field/change rows exist; 65 such notices occurred here. | Distinguish ambiguous attribution pairs from actual retained history records in the report. |
| CI coverage | Follow-up `t_d8a7acf4` includes command packages in shared local, CI and release unit/coverage gates. | Independent command factories remain `t_ee618493`; package-local sequencing currently protects shared Cobra test state. |
| Write-test ownership | Follow-up `t_5f7ba8da` adds private run manifests, stored-provenance preflight and atomic cleanup that refuses shared or modified events. | Stop and drain the run before cleanup; retain its manifest. Source rows remain for separate preview-first pruning. |

The first safeguard wave added verified migration backups, read-only probes with
enforced failure status, and automated tooling/CLI checks. The second addresses
label ownership and deliberate write-test isolation. Writer coordination, restore
rehearsal and custom-backup retention remain separate work. URL-variant consolidation
should follow measurement, not an assumption that every extra observation is wrong.

## Procedure to reuse

1. Confirm the repository/ref, shared kanban, reviewer availability, and exactly which
   binary/container will run each command. Separate known facts from assumptions.
2. Capture a read-only inventory: feed identities and variants, source/config policy
   differences, collision counts, references and protected metadata.
3. Define preservation invariants and representative fixtures before implementing.
   Include observation conservation and the second-run no-op.
4. Verify backup tooling in its actual environment, create the private backup and
   read the full archive. Use an isolated restore rehearsal when recovery assurance
   requires it. Retain the path, checksum and validation result.
5. Prepare the compatible binary. Quiesce new scrape/submission work and drain
   in-flight writers using the environment's maintenance controls; stopping future
   scheduling alone does not drain active work.
6. Review the rollback-only preview and explicit policy changes, then apply. Keep
   reconciliation separate from orphan retirement. Never treat zero-event rows as
   automatically deletable.
7. Verify conservation and a second no-op, finish cutover, stop old workers, and audit
   again. Repeat reconciliation after writers are quiescent if stale writes appeared.
8. Run smoke checks and a deliberately selected load profile. Verify write isolation
   before any write traffic; inspect status-code distributions even on exit zero.
9. Remove only positively identified test artifacts, audit again, commit the evidence,
   push and obtain independent review. State what the reviewer could not verify.

This run established a reliable data result. The next run should preserve these
checks while moving discovery and operational preparation ahead of deployment.
