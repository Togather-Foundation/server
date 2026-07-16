# Troubleshooting

Common issues and their workarounds. Load this file when batch commands fail
or when dealing with recurring series.

## Batch name matching failures

Name substring matching is fragile. Causes and workarounds:

- **Apostrophes** — both `'` (U+0027) and `'` (U+2019) may appear. HTML-encoded
  right single quotes (`&#8217;`) appear in event names too. Try short substrings
  without apostrophes: `--name "Symphony"` instead of `--name "Tchaikovsky's"`.
- **Ampersands** — may appear as `&` or `&amp;`. Try splitting on `&`: target
  each half separately.
- **Emoji flags** — some scrapers include flag emoji in event names. Try the text
  portion only.
- **JSON control characters** — unescaped HTML entities (`&amp;`, `&#39;`) may
  remain in event name fields. Use `json.loads(strict=False)` if parsing inline.

## 409 "already approved" errors

When batches overlap (same item matched by two different `--name` or `--source`
patterns), the second attempt gets a 409. These are harmless — the item was
already approved. The batch continues processing remaining items.

## Stale companion ULIDs (pre-fix orphaned items)

Items orphaned before commit 33ef8f55 may have companion ULIDs pointing to
retired events. The error chain:
- `merge-into-primary` sends POST to `/api/v1/admin/events/consolidate`
- Canonical event is checked → returns 422 "Event has been deleted"

**Recovery (for pre-fix orphans):** Approve as individuals via batch by name:
```bash
./server review batch --name "Series Name" --action approve \
  --notes "Pre-fix orphan, companion deleted" --server ... --key ...
```

## Disabled sources leave orphaned queue items

Setting `enabled: false` in a scraper config stops new scrapes but does NOT
clear items already in the queue. Before starting a full pass, check source
groups against the `configs/sources/*.yaml` directory. Batch-reject orphans:
```bash
./server review batch --source "<uuid>" --action reject \
  --reason "Source disabled — global feed with no city filter" --server ... --key ...
```

## Cron interruption edge case

If a gateway restart kills a cron mid-run, some events may have been
consolidated (retired) but their review items remain "pending." After the fix
(33ef8f55), consolidate refreshes stale companion references automatically.
Pre-fix items may still need individual batch-approve by name.

## Pagination limits

- `--limit` max is 200. Use `--offset N` for pagination on newer builds.
- If `--offset` isn't recognized, update the server binary.

## Recurring series are intentional

Scrapers push every occurrence as a separate event by design — dumb scrapers,
intelligent review. The review pass handles consolidation via merge or
batch-approve by name. This is normal operation, not a bug.
