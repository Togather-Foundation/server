# Audit pending review dedup hashes on staging

Use this audit to count pending review rows whose dedup hash differs from the
current event hash. Check application logs as well, because a rename that rolls
back after a hash conflict leaves no new mismatch in the database. The audit
reports aggregate counts and leaves staging data untouched.

## Connect to staging

Read `SSH_HOST` and `SSH_USER` from `.deploy.conf.staging` and connect to that host.
Confirm the database container and available application slots.

```sh
docker ps -a --format '{{.Names}} {{.Status}}'
```

The standard staging database container is `togather-db`. Run the following
query on the staging host. It uses the database role and name from the container,
sets short lock and statement timeouts, and takes a consistent read-only snapshot.

```sh
docker exec -i \
  -e PGOPTIONS='-c default_transaction_read_only=on -c statement_timeout=5000 -c lock_timeout=1000' \
  togather-db sh -c 'exec psql -X -v ON_ERROR_STOP=1 -P pager=off -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL TIME ZONE 'UTC';
SHOW transaction_read_only;

SELECT transaction_timestamp() AS snapshot_time;

SELECT indexdef
FROM pg_indexes
WHERE schemaname = 'public'
  AND tablename = 'event_review_queue'
  AND indexname = 'idx_review_queue_unique_pending_dedup';

SELECT count(*) AS pending_rows,
       count(*) FILTER (WHERE r.dedup_hash IS NULL) AS null_review_hash,
       count(*) FILTER (
           WHERE r.dedup_hash IS NOT NULL AND NULLIF(e.dedup_hash, '') IS NULL
       ) AS missing_event_hash
FROM public.event_review_queue AS r
JOIN public.events AS e ON e.id = r.event_id
WHERE r.status = 'pending';

WITH mismatches AS (
    SELECT r.id, e.dedup_hash AS target_hash
    FROM public.event_review_queue AS r
    JOIN public.events AS e ON e.id = r.event_id
    WHERE r.status = 'pending'
      AND r.dedup_hash IS NOT NULL
      AND NULLIF(e.dedup_hash, '') IS NOT NULL
      AND r.dedup_hash <> e.dedup_hash
),
classified AS (
    SELECT m.target_hash,
           count(*) OVER (PARTITION BY m.target_hash) AS target_count,
           EXISTS (
               SELECT 1
               FROM public.event_review_queue AS other
               WHERE other.status = 'pending'
                 AND other.dedup_hash = m.target_hash
                 AND other.id <> m.id
           ) AS occupied
    FROM mismatches AS m
)
SELECT count(*) AS mismatched_rows,
       count(*) FILTER (WHERE NOT occupied AND target_count = 1) AS free_unshared_targets,
       count(*) FILTER (WHERE occupied) AS occupied_target_rows,
       count(*) FILTER (WHERE target_count > 1) AS shared_target_rows,
       count(DISTINCT target_hash) FILTER (WHERE target_count > 1) AS shared_target_groups
FROM classified;

ROLLBACK;
SQL
```

Verify that `transaction_read_only` is `on` and the pending index is unique on
`dedup_hash` for pending rows with non-NULL hashes. The index currently applies
across both synthetic and ordinary rows, so the occupancy check uses both.

An occupied target is already reserved by another pending row. A shared target
is sought by multiple mismatched rows. These counts can overlap. A free,
unshared target means the audit found neither conflict at snapshot time.
Hash mismatches are candidates for investigation. Review workflows can
intentionally omit the hash, so the audit counts NULL review hashes separately.

## Check rename errors in logs

Inspect both retained staging application slots, including the stopped slot if
available, and the database container. Run this Bash snippet on the staging
host. Update the UTC window and retain only the containers found above.

```bash
set -euo pipefail
since_utc=$(date -u -d '7 days ago' +%FT%TZ)
until_utc=$(date -u +%FT%TZ)
printf 'requested_since=%s requested_until=%s\n' "$since_utc" "$until_utc"

for container in togather-server-blue togather-server-green togather-db; do
    docker inspect --format '{{.Name}} {{.Config.Image}} {{.Created}} {{.HostConfig.LogConfig.Type}}' "$container"
    timeout 45s docker logs --timestamps --since "$since_utc" --until "$until_utc" "$container" 2>&1 |
        awk '
            /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T/ {
                if (++lines == 1) first = $1
                last = $1
                refresh = /refresh pending review dedup hash/
                rename = /feed event-ID auto-merge update/
                collision = /idx_review_queue_unique_pending_dedup/ && /duplicate key value|23505/
                contention = /40P01|55P03|40001|deadlock detected|canceling statement due to lock timeout|could not serialize access|retryable database contention/
                refresh_lines += refresh
                index_conflicts += collision
                refresh_collisions += refresh && collision
                rename_contention += (refresh || rename) && contention
                broad_contention += contention
            }
            END {
                printf "lines=%d first=%s last=%s\n", lines, first, last
                printf "refresh_lines=%d index_conflicts=%d refresh_collisions=%d rename_contention=%d broad_contention=%d\n",
                    refresh_lines, index_conflicts, refresh_collisions, rename_contention, broad_contention
            }'
done
```

Use counts from successful scans. A timeout or Docker error can leave partial
output. Counts are matching log lines. Admin-side deadlock errors can lack
rename context, so broad contention helps explain why a rename-specific count
can be lower. Zero matches means the retained logs contained no matching errors
in the requested window. Container replacement and rotation can shorten
coverage, so report how long the audited revision was visible.
