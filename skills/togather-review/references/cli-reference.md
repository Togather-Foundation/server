# CLI Reference

Complete command table for the `server review` subcommand.

| Task | Command |
|------|---------|
| Overview | `./server review stats` |
| List pending | `./server review queue` |
| Paginate | `./server review queue --limit 200 --offset 200` |
| Output to file | `./server review queue --json --output /tmp/queue.json` |
| Group by name | `./server review queue --group-by name` |
| Group by source | `./server review queue --group-by source` |
| Group by warning | `./server review queue --group-by warning` |
| Inspect one | `./server review check <id>` |
| Approve one | `./server review approve <id> --notes "..."` |
| Reject one | `./server review reject <id> --reason "..."` |
| Batch by source | `./server review batch --source <uuid> --action approve` |
| Batch by name | `./server review batch --name "substring" --action approve` |
| Batch by warning | `./server review batch --warning missing_description --action approve` |
| Merge into primary | `./server review merge <primary-ulid> <new-ulid> --transfer-occurrences` |
| Find suppressed weekly series | `./server review series --days 90` |
| Scope series to one source | `./server review series --source tpl-events --limit 20` |
| Page series candidates | `./server review series --offset 20 --limit 20` |
| Include recent past | `./server review series --days 90 --lookback-days 30` |
| Series JSON output | `./server review series --days 90 --json` |
| Consolidate a series | `./server review consolidate <canonical-ulid> <member-ulids...> --transfer-occurrences` |
| Consolidate (3+ events) | `./server review consolidate <canonical-ulid> <dup1> <dup2>` |
| Fix dates | `./server review fix <id> --start-date 2026-06-15T19:00:00-04:00` |
| Dry-run any batch | append `--dry-run` to batch commands |
