# Canonical source repair — t_eafc6fae

Staging repair applied on 2026-11-12 UTC using commit `f815a6bddfda26adbaa6407a565215563db33262` on `fix/t_eafc6fae-source-feed-identity`. The rollback-only preview was independently reviewed before application. Preview and apply counts and all seven trust changes matched exactly.

## Result

| Measure | Before repair | After reconciliation | After orphan cleanup |
|---|---:|---:|---:|
| Source rows | 441 | 616 | 597 |
| Live attributions | 32,183 | 23,902 | 23,902 |
| Archived attributions | 0 | 8,281 | 8,281 |
| Noncanonical attributions | 29,735 | 0 | 0 |
| Unregistered attributions | 500 | 0 | 0 |
| Fragmented feeds | 83 | 0 | 0 |
| Zero-event source rows | 289 | 325 | 306 |
| Configured idle rows | 289 | 305 | 305 |
| Orphaned idle rows | 0 | 20 | 1 |
| Protected orphan rows | 0 | 1 | 1 |

Reconciliation moved **21,652** attributions and archived **8,281** colliding live rows. It registered/adopted feed identities, including sitemap detail URLs, without changing event identity or source URLs. All **32,183 observations** survive: **23,902 live + 8,281 archived**. Each archive contains the complete original attribution, original source metadata and canonical attribution id; its immutability is enforced by a trigger.

No sources were deleted by reconciliation. Separate orphan cleanup deleted only **19 unreferenced rows with default metadata**, retaining all **305 configured idle rows** and the **one metadata-protected orphan**. Config presence is checked against both YAML and database inventories; event attributions, field provenance, change records and API keys also protect rows.

## Policy preservation and explicit trust repair

Licenses and scraper warning suppressions were preserved. Suppression belongs to `scraper_sources` and remains independent for query-filtered configs sharing one canonical feed. Normal ingest/sync does not silently overwrite stored source policy. For this repair, `--use-config-trust` explicitly corrected seven legacy default-trust values to their unambiguous configured values, with matching licenses; every other source metadata field was retained. Shared catch-all rows were not assigned conflicting per-feed policies.

The source links below identify the exact normalized feeds.

| Source / feed | Source UUID | Old trust | Configured trust |
|---|---|---:|---:|
| [absolute-comedy](https://www.absolutecomedy.ca) | `37fe7a09-c8cc-470c-ae42-4b56a4f2bf94` | 5 | 6 |
| [east-end-arts](https://eastendarts.ca) | `3c25a7ca-3b33-4e21-acf3-87d35ba9f352` | 5 | 6 |
| [hughs-room-live](https://hughsroomlive.com) | `27a1d6c7-c71f-4448-b697-9ff548727181` | 5 | 7 |
| [mhrth-tnew](https://mass-tnew-prod.tnhs.cloud) | `877468c0-355f-4e56-8add-4dd2d12ad612` | 5 | 8 |
| [opera-atelier](https://www.operaatelier.com) | `b812e169-725c-4597-a253-5ea72ef28305` | 5 | 8 |
| [toronto-waterfront-festival](https://towaterfrontfest.com) | `08b5051f-bd4c-498d-a74a-87d382e63bee` | 5 | 7 |
| [york-university](https://events.yorku.ca) | `827c8dcd-5d3a-4453-8979-322245e70fcb` | 5 | 8 |

The preview/apply reported two original-metadata retention notices and 65 ambiguous event/source attribution pairs. Those ambiguity notices are generated conservatively from attribution choices. A read-only database check found **zero actual field-provenance and zero change records** referencing the affected legacy `Meetup` source (`6b7ef2b4-0847-4df1-ada4-183474ea4ba7`). Thus these notices did not represent unresolved stored histories. The second reconciliation preview returned an empty issues list, with no outstanding feed-policy conflicts.

## Commands and idempotence

Commands ran against the latest staging repair binary; defaults use the complete deployed `configs/sources` inventory. The refreshed backup below was validated before apply.

```bash
server source reconcile --json-report --use-config-trust
server source reconcile --json-report --use-config-trust --confirm
server source reconcile --json-report --use-config-trust
server source prune-empty --json-report
server source prune-empty --json-report --confirm
server source prune-empty --json-report
```

The second reconciliation preview reported **zero attribution moves, zero archives, zero trust changes, zero field/change moves and identical before/after counts**. The second orphan-cleanup preview also proposed **zero deletions** and retained identical counts. No event or occurrence mutation is part of this repair.

Canonical registration and policy comparison use [source queries](../../internal/storage/postgres/queries/sources.sql). The exact repair DML returned in the applied report was:

```sql
-- name: UseConfiguredSourceTrust :execrows
UPDATE sources SET trust_level=$1
WHERE id=$2 AND feed_url=$3::text
  AND trust_level=$4
  AND license_url=$5 AND license_type=$6

-- name: MoveSourceRepairAttribution :execrows
UPDATE event_sources SET source_id=$1
WHERE id=$2

-- name: ArchiveSourceRepairAttribution :execrows
INSERT INTO source_repair_attribution_archive (
  original_attribution_id, canonical_attribution_id, original_attribution, original_source
)
SELECT es.id, $1::uuid, to_jsonb(es), to_jsonb(s)
FROM event_sources es JOIN sources s ON s.id=es.source_id
WHERE es.id=$2
ON CONFLICT (original_attribution_id) DO NOTHING

-- name: DeleteArchivedSourceRepairAttribution :execrows
DELETE FROM event_sources es
WHERE es.id=$1
AND EXISTS (
  SELECT 1 FROM source_repair_attribution_archive archived
  WHERE archived.original_attribution_id=es.id AND archived.original_attribution=to_jsonb(es)
)

-- name: MoveSourceRepairFields :execrows
UPDATE field_provenance fp SET source_id=$1
WHERE fp.event_id=$2 AND fp.source_id=$3
AND NOT EXISTS (
  SELECT 1 FROM field_provenance twin
  WHERE twin.event_id=fp.event_id AND twin.field_path=fp.field_path
    AND twin.source_id=$1 AND twin.observed_at=fp.observed_at
)

-- name: CountSourceRepairRetainedFields :one
SELECT count(*) FROM field_provenance fp
WHERE fp.event_id=$1 AND fp.source_id=$2
AND EXISTS (
  SELECT 1 FROM field_provenance twin
  WHERE twin.event_id=fp.event_id AND twin.field_path=fp.field_path
    AND twin.source_id=$3 AND twin.observed_at=fp.observed_at
)

-- name: MoveSourceRepairChanges :execrows
UPDATE event_changes SET source_id=$1
WHERE event_id=$2 AND source_id=$3
```

Cleanup used this additional statement after the config/reference/metadata-protected classification under the same locked transaction:

```sql
-- name: DeleteSourceRepairOrphan :execrows
DELETE FROM sources s WHERE s.id=$1
  AND NOT EXISTS (SELECT 1 FROM event_sources es WHERE es.source_id=s.id)
  AND NOT EXISTS (SELECT 1 FROM field_provenance fp WHERE fp.source_id=s.id)
  AND NOT EXISTS (SELECT 1 FROM event_changes ec WHERE ec.source_id=s.id)
  AND NOT EXISTS (SELECT 1 FROM api_keys ak WHERE ak.source_id=s.id)
```

## Backup and operational tooling

Private backup on the staging host identified by `.deploy.conf.staging`:

- Path: `/home/deploy/.togather/snapshots/source-feed-preapply-20261112T025738Z.dump`
- Size: **47,964,820 bytes**
- SHA-256: `21311de10d590b72a2d94e7522f76f371c7c05a540770eb5a35ba2a8411140ce`
- Created with the database container’s `pg_dump -Fc`, under `umask 077`.
- Validated by reading the full archive with `pg_restore --exit-on-error --file=/dev/null`; no restore executed.

The dump is outside git. Deploy’s automatic snapshot was unavailable because the host server binary was missing; the application container also lacked `pg_dump`. The database-container fallback required no software installation. [Deployment documentation](../deploy/remote-deployment.md#snapshot-tooling-and-database-container-fallback), the release skill and the server-ops skill now document prerequisites, full-read validation and durable provisioning.

## Validation

- `scripts/agent-run.sh make ci-fast` passed at `f815a6bd`, including lint, formatting, OpenAPI, docs, shell, YAML, build, SQLc consistency and all test suites.
- Focused tests cover concurrency, rename before first attribution, actual config sync, legacy policy selection/conflicts, blue/green index compatibility, archival conservation/immutability, preview rollback, detail-feed ownership, explicit trust repair, safe pruning and second-run no-op.
- Independent code re-review and live-preview review found no blocking issues before apply.

The final staging rollout completed successfully at `f815a6bd`: **262/262 configs synced**, zero warnings/errors, blue traffic verified at the full commit SHA, and old green stopped. `scripts/agent-run.sh ./deploy/scripts/test-remote.sh staging all` exited zero: **20/20 smoke checks** passed and the light performance wrapper completed.

The load probe sent **343 requests**, with **250 successful**, **28 HTTP 401** responses and **65 HTTP 403** responses (protected metrics requests). Its exit-zero result is not a claim of error-free authenticated traffic. The 26 successful fixture writes created 26 events and 26 independent sources; each event/source pair was verified to have one attribution, creation within the load-test window, and no merge, external-review or API-key references. Existing source-scoped staging cleanup removed those events, and the protected orphan cleanup separately removed their 26 unused source rows. Fixture cleanup is separate from the 19 historical orphan rows in the result table.

The post-rollout audit remained at zero fragmented, unregistered and noncanonical attributions even while the test fixtures existed. The final post-cleanup audit exactly matched the result table: **597 source rows, 23,902 live + 8,281 archived attributions, 305 configured idle rows, one protected orphan, and zero fragmented/unregistered/noncanonical attributions**.
