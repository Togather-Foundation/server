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

`server deploy rollback <environment>` is **not** the same thing: it only
re-points the state file and never starts a container or touches Caddy. Use it
only when you want the state file moved and nothing else.

## Rollback Workflow

1. Preview the plan (read-only, changes nothing):
   ```bash
   deploy/scripts/deploy.sh rollback <environment> --dry-run
   ```
2. Roll back:
   ```bash
   deploy/scripts/deploy.sh rollback <environment>
   ```
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

## Database Considerations

Restore a snapshot if migrations ran after the target deployment.

```bash
cd /opt/togather/src/deploy/docker
docker compose -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml down

gunzip -c /var/lib/togather/db-snapshots/<snapshot-file>.sql.gz | psql "$DATABASE_URL"

deploy/scripts/deploy.sh rollback <environment>
```

## Troubleshooting

- **No previous deployment found**: first deployment or cleared state; redeploy target version.
- **Health checks fail**: code/schema mismatch or infra issue; check logs and DB connectivity.
- **Traffic does not switch**: Caddy not running or Caddyfile targets wrong port.
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
