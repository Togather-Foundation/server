# Caddy Proxy Architecture

> **Note**: This document has been consolidated into the comprehensive Caddy guide.
>
> **See**: [`docs/deploy/caddy.md`](../docs/deploy/caddy.md) for complete documentation on:
> - Caddy architecture (Docker vs System)
> - Installation and configuration
> - Blue-green traffic switching
> - Monitoring and troubleshooting
> - Deployment workflow integration

## Quick Reference

**Local Development** (Docker Caddy):
```bash
# See: docs/deploy/caddy.md#local-development-docker-caddy
docker compose -f docker-compose.yml -f docker-compose.blue-green.yml up -d
```

**Staging/Production** (System Caddy):
```bash
# See: docs/deploy/caddy.md#staging-production-system-caddy
sudo systemctl status caddy
sudo caddy reload --config /etc/caddy/Caddyfile --force
```

**Troubleshooting**:
- See: [`docs/deploy/caddy.md#troubleshooting`](../docs/deploy/caddy.md#troubleshooting)

**File Locations**:
- See: [`docs/deploy/caddy.md#file-reference`](../docs/deploy/caddy.md#file-reference)
