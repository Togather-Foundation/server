# Rollback Guide

Use this when a deployment fails or health checks stay unhealthy.

## Quick Reference

```bash
server deploy rollback <environment>
server deploy rollback <environment> --force
server deploy rollback <environment> --dry-run
```

## Rollback Workflow

1. Confirm deployment state:
   ```bash
   cat /opt/togather/src/deploy/config/deployment-state.json | jq .
   ```
2. Run rollback:
   ```bash
   server deploy rollback <environment>
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

## Database Considerations

Restore a snapshot if migrations ran after the target deployment.

```bash
cd /opt/togather/src/deploy/docker
docker compose -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml down

gunzip -c /var/lib/togather/db-snapshots/<snapshot-file>.sql.gz | psql "$DATABASE_URL"

server deploy rollback <environment> --force
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
