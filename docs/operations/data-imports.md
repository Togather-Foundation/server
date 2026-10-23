# Bulk Imports and Restores

How to load events directly into the database without breaking public visibility,
tombstones or the change feed.

## The invariant

`events.first_published_at` answers one question: **was this event ever publicly
visible?** `NULL` means never.

- The `trg_events_first_published_at` trigger maintains it on every insert and
  lifecycle update. An insert in a public state stamps `created_at`; entering a
  public state stamps `now()`; unpublishing and deleting preserve the value.
- Public detail, HTML, ICS and MCP reads return `410` (tombstone) for a deleted
  event only when `first_published_at IS NOT NULL`. Never-public deleted events
  return `404`.
- The public change feed exposes a delete row only when
  `first_published_at IS NOT NULL AND first_published_at <= changed_at`.
- Migration `000055` backfilled the column from `event_changes` snapshots whose
  `lifecycle_state` was public (`MIN(changed_at)`).

If an import bypasses this signal, previously-public tombstones disappear (fail
closed). Nothing leaks, but federation consumers lose the deletion signal.

## Preferred path

Use the API or the scraper ingest path. The trigger runs for normal inserts and
lifecycle updates, so imports through the application need no special handling.

## Direct SQL inserts

The trigger treats an explicit non-NULL `first_published_at` as authoritative and
does not overwrite it. Use that for backfills and restores:

| Imported row | What to set |
|---|---|
| Event that was public upstream | Set `first_published_at` to the upstream first-publish time (or the best available proxy, e.g. the event's original creation time). |
| Draft / pending_review / never published | Leave `NULL`. Do not invent a value. |
| Deleted event that was public upstream | Set `first_published_at` so the tombstone and delete feed row remain visible. |
| Deleted event that was never public | Leave `NULL`; anonymous reads correctly return `404`. |

Do **not** derive this from `events.published_at`: federation sync writes that
column from the upstream occurrence `startDate`, which is not publish time.

## Restoring event history

If the restore includes `event_changes` rows, load them **before** re-running the
backfill, or run the backfill after the restore:

```sql
UPDATE events e
   SET first_published_at = fp.first_public_at
  FROM (
    SELECT ec.event_id, MIN(ec.changed_at) AS first_public_at
      FROM event_changes ec
     WHERE ec.snapshot->>'lifecycle_state' IN (
             'published','postponed','rescheduled','sold_out','cancelled','completed')
     GROUP BY ec.event_id
  ) fp
 WHERE e.id = fp.event_id
   AND e.first_published_at IS NULL;
```

The query is idempotent. Run it with the change-log update trigger disabled so it
does not inject a synthetic `update` row per event into the public feed:

```sql
ALTER TABLE events DISABLE TRIGGER trg_event_changes_update;
-- run the UPDATE above
ALTER TABLE events ENABLE TRIGGER trg_event_changes_update;
```

`ALTER TABLE ... DISABLE TRIGGER` takes an `ACCESS EXCLUSIVE` lock on `events`.
Run it in a maintenance window (the same trade-off as migration `000055`).

If the restore has no history and no explicit `first_published_at`, leave the
column `NULL` and record that those tombstones are intentionally hidden.

## Verification

```sql
-- Public events with no signal: expected for pre-change-log legacy rows until
-- they next leave a public state.
SELECT count(*) FROM events
 WHERE lifecycle_state IN ('published','postponed','rescheduled','sold_out','cancelled','completed')
   AND first_published_at IS NULL;

-- Deleted events with a tombstone but no public signal: these will return 404.
SELECT count(*)
  FROM event_tombstones t
  JOIN events e ON e.id = t.event_id
 WHERE e.lifecycle_state = 'deleted'
   AND e.first_published_at IS NULL;

-- Backfill sample: compare with the earliest public snapshot.
SELECT e.ulid, e.first_published_at, MIN(ec.changed_at) AS first_public_snapshot
  FROM events e
  JOIN event_changes ec ON ec.event_id = e.id
 WHERE ec.snapshot->>'lifecycle_state' IN (
         'published','postponed','rescheduled','sold_out','cancelled','completed')
 GROUP BY e.ulid, e.first_published_at
 LIMIT 20;
```

Then spot-check the API: a previously-public deleted ULID must return `410`; a
never-public deleted ULID must return `404` with no event name or tombstone body.

## Related

- [API guide: deleted events](../integration/api-guide.md#deleted-events)
- [Federation change feed](../interop/federation-v1.md)
- Migration [`000055_events_first_published_at.up.sql`](../../internal/storage/postgres/migrations/000055_events_first_published_at.up.sql)
