# Rollback Guide

Use this when a deployment fails or health checks stay unhealthy.

## Quick Reference

```bash
deploy/scripts/deploy.sh rollback <environment> --dry-run   # show the plan
deploy/scripts/deploy.sh rollback <environment>             # do the rollback
```

That one command resolves the retired slot from deployment state, checks the
image for its version is still on the host, starts the container, waits for
health on its direct port (8081 blue, 8082 green), switches Caddy traffic,
flips deployment state, and stops the outgoing slot.

The version to restore comes from `.previous_deployment` in the deployment state
file, the same source the slot comes from. The history directory is only used to
fill in the exact `docker_image` and the snapshot path; a `previous.json` link
that names a different version is reported and ignored.

`server deploy rollback <environment>` is **not** the same thing: it only
re-points the state file and never starts a container or touches Caddy. Use it
only when you want the state file moved and nothing else.

## Rollback Workflow

1. Preview the plan (read-only, changes nothing):
   ```bash
   deploy/scripts/deploy.sh rollback <environment> --dry-run
   ```
   The plan shows the HTTPS `/version` URL used to verify traffic after the switch.
   Rollback loads `NODE_DOMAIN` from the repository's `.deploy.conf.<environment>`.
   If the file is missing, rollback logs a warning and keeps the current `NODE_DOMAIN`.
   When `NODE_DOMAIN` is unset or empty, the URL defaults to
   `https://<environment>.toronto.togather.foundation/version`.
2. Roll back:
   ```bash
   deploy/scripts/deploy.sh rollback <environment>
   ```
   If deployment state changed during preflight, rollback stops. Preview the
   updated plan before retrying. Rollback rechecks the target image and container
   under the deployment lock because a failed deploy can change either one
   before recording deployment state.
3. Verify health:
   ```bash
   server healthcheck --slot blue --format json
   server healthcheck --slot green --format table
   curl http://localhost:8081/health
   curl http://localhost:8082/health
   ```
4. If health fails, continue with troubleshooting below.

**What rollback changes:**

- ✅ Application code/image and active slot
- ✅ Deployment state metadata
- ❌ Database schema (manual restore if needed)
- ❌ Environment variables
- ❌ External dependencies

If the image for the target version is gone, the rollback refuses and prints the
exact rebuild command instead of starting a slot on the wrong build.

If the container on the target slot was created from a different image than the
version being restored (a deploy that died after recreating the container but
before recording itself), the rollback refuses too. Remove that container and
run the rollback again, which recreates it on the recorded image:

```bash
docker rm -f togather-server-<slot>
deploy/scripts/deploy.sh rollback <environment>
```

## Database Considerations

Restore a snapshot if migrations ran after the target deployment.

Current deployments record a private custom-format `.dump` backup path and adjacent
`.dump.json` evidence in `$HOME/.togather/snapshots`. Verify its checksum and target,
quiesce writers, and plan the restore before executing it. `server snapshot list`
only lists legacy `.sql.gz` backups. To restore a current custom archive using
compatible clients in the target database's network environment:

```bash
# Destructive recovery: DATABASE_URL must identify the intended recovery database.
pg_restore --exit-on-error --clean --if-exists --dbname "$DATABASE_URL" < /path/to/verified-backup.dump
```

The following gzip/psql example applies only to legacy `.sql.gz` CLI snapshots:

```bash
cd /opt/togather/src/deploy/docker
docker compose -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml down

gunzip -c /var/lib/togather/db-snapshots/<snapshot-file>.sql.gz | psql "$DATABASE_URL"

deploy/scripts/deploy.sh rollback <environment>
```

### Per-feed source identity (migration 000056)

Migration `000056_per_feed_source_provenance` replaces `sources_base_url_unique` with
`sources_base_url_name_unique (base_url, name)`. The per-feed build **requires** that index:
`GetOrCreateSource` uses `ON CONFLICT (base_url, name)`, so rolling the migration back while
the new binary is running makes every ingest fail with

```
no unique or exclusion constraint matching the ON CONFLICT specification
```

Correct order: roll the **application** back first, then run the migration down. The down
migration also fails while multiple sources share a `base_url` (the per-feed state); merge or
delete the extra rows before reverting, and expect historical `event_sources` attribution for
the removed rows to be lost.

### Canonical feed registry (migration 000060)

Migration `000060` adds nullable `sources.feed_url` and its unique index while retaining
`sources_base_url_name_unique` for the previous binary during blue/green cutover and
application rollback. Historical rows are not automatically merged. Canonical
registration uses the new feed key; the older base/name index remains a compatibility
constraint. A conflicting legacy row must be reviewed rather than silently overwritten.

Roll the application back before removing `feed_url`: the new binary requires it.
The down migration removes only the canonical index/column; provenance rows and repaired
references remain. Migration `000061` holds complete attribution/source snapshots
for duplicate attribution consolidation; do not drop that archive after repair. Restore
the pre-deployment snapshot if the data repair itself must be undone. Never delete provenance rows merely to make a rollback index succeed.

## Troubleshooting

- **No previous deployment found**: first deployment or cleared state; redeploy target version.
- **Health checks fail**: code/schema mismatch or infra issue; check logs and DB connectivity.
- **Traffic does not switch**: Caddy not running or Caddyfile targets wrong port.
- **Container is a different build**: `docker rm -f togather-server-<slot>` then rerun the rollback.
- **`previous.json` names the wrong version**: harmless, the rollback reads the state file; the link advances on the next deployment.
- **Schema mismatch**: restore snapshot or roll back migrations.

```bash
cat /opt/togather/src/deploy/config/deployment-state.json | jq .
docker logs --tail 100 togather-server-blue
docker logs --tail 100 togather-server-green
sudo systemctl status caddy
```

## Recovery

If rollback fails and the service is down:

```bash
cd /opt/togather/src/deploy/docker
docker compose -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml down

git checkout <known-good-commit>
cd /opt/togather/src
./deploy/scripts/deploy.sh <environment> --force
```

Manual slot switch (only if you know which slot is healthy):

```bash
cat /opt/togather/src/deploy/config/deployment-state.json | jq -r '.current_deployment.active_slot'
sudo nano /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

## Additional Resources

- `server deploy --help`
- `server healthcheck --help`
- `server snapshot --help`
- `deploy/scripts/deploy.sh --help`
- `migrations.md`

## Getting Help

1. Check deployment logs: `~/.togather/logs/deployments/<env>_*.log`
2. Review deployment state: `/opt/togather/src/deploy/config/deployment-state.json`
3. Contact the Togather infrastructure team

---

**Last Updated:** 2026-02-20
