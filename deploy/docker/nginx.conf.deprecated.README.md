# nginx.conf.deprecated

This nginx configuration file has been deprecated in favor of Caddy.

**Date**: 2026-02-02
**Reason**: Migrated to Caddy for automatic HTTPS and simpler configuration

## Migration

- **New file**: `Caddyfile` - Caddy configuration for Docker blue-green deployments
- **Production**: See `deploy/Caddyfile` for production server configuration
- **Documentation**: See `docs/deploy/CADDY-DEPLOYMENT.md` for deployment guide

## Why Keep This File?

This file is kept for reference purposes:
- Historical reference for teams transitioning from nginx
- Comparison for understanding nginx→Caddy configuration mapping
- Backup in case rollback to nginx is needed

## If You Need Nginx

If you need to use nginx instead of Caddy:

1. Rename this file back to `nginx.conf`
2. Update `docker-compose.blue-green.yml` to use nginx:alpine image
3. Update deployment scripts to use `nginx -s reload` instead of `caddy reload`

See commit history for the full nginx implementation.
