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
| Consolidate (3+ events) | `./server review consolidate <canonical-ulid> <dup1> <dup2>` |
| Fix dates | `./server review fix <id> --start-date 2026-06-15T19:00:00-04:00` |
| Dry-run any batch | append `--dry-run` to batch commands |
