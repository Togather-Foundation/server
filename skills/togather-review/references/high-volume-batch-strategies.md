# High-Volume Batch Strategies

Patterns and strategies for clearing large review queues efficiently.

## Primary strategy: Batch by source

With the `--source` standalone batch filter (commit fca23d53), approve an entire
source in one command:

```bash
./server review batch --source "11f98d04-f875-403f-92ff-d65eccd4dd7f" \
  --action approve --notes "Real events" --server ... --key ...
```

Combine with `--warning` for narrower targeting on mixed sources:

```bash
./server review batch --source "14a37f6d-..." --warning "missing_description" \
  --action approve --server ... --key ...
```

Use `--limit` to cap a batch:

```bash
./server review batch --source "14a37f6d-..." --action approve --limit 50 \
  --server ... --key ...
```

## Name-substring profiling (narrow targeting)

When you want to approve a subset of a source, find common name substrings:

```bash
./server review queue --source <uuid> --json --output /tmp/items.json

python3 -c "
import json
items = json.load(open('/tmp/items.json'))
names = [i.get('eventName','') for i in items]
from collections import Counter
words = Counter()
for n in names:
    for w in n.split()[:2]:
        words[w.lower()] += 1
for w, c in words.most_common(20):
    if c >= 2 and len(w) > 3:
        print(f'{w}: {c}')
"
```

## Recurring series merge workflow

### Normal weekly merge (one merge per week)

When a single new weekly occurrence appears:
1. The `cross_week_series_companion` warning fires with the canonical primary ULID
2. After the stale-companion fix (33ef8f55), `companion_ulid` is the canonical primary
3. Run: `./server review merge <companion-ulid> <new-event-ulid> --transfer-occurrences`

### Backlog recovery (merge unavailable)

If `merge-into-primary` fails with 422 "Event has been deleted" (pre-fix orphan):
```bash
./server review batch --name "Series Name" --action approve \
  --notes "Pre-fix orphan" --server ... --key ...
```

## Apostrophe handling

HTML-encoded right single quotes (`&#8217;`) appear in event names. Use the
unicode character directly:
- `--name "'"` (right single quote U+2019)
- Short substrings without apostrophes: `--name "Symphony"` for "Tchaikovsky's Symphony"

## 409 "already approved" errors

When batches overlap, the second attempt gets a 409. Harmless — the item was
already approved. Continue processing.
