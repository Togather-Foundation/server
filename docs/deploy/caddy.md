# Caddy Proxy for Togather Server

This guide covers deploying Togather SEL server behind Caddy reverse proxy with automatic HTTPS and blue-green deployments.

## Overview

The Togather server uses **different Caddy setups for different environments**:

- **Local Development**: Docker Caddy container (defined in docker-compose.blue-green.yml)
- **Staging/Production**: System Caddy installed via apt (managed by systemd)

## Why Caddy?

- **Automatic HTTPS**: Obtains and renews SSL certificates automatically via Let's Encrypt
- **Simple Configuration**: Human-readable Caddyfile format
- **Modern Defaults**: HTTP/2, HTTPS redirects, and security headers out of the box
- **Zero Downtime**: Graceful config reloads without dropping connections
- **Service Resilience**: System Caddy survives Docker container restarts and Docker daemon issues

## Architecture

```
┌─────────────────────────────────────────┐
│     Caddy Reverse Proxy :80/:443        │
│     Automatic SSL + Blue-Green Switch   │
└─────────────┬───────────────────────────┘
              │
        ┌─────┴─────┐
        │           │
    ┌───▼───┐   ┌───▼───┐
    │ Blue  │   │ Green │
    │ Slot  │   │ Slot  │
    │ :8081 │   │ :8082 │
    └───┬───┘   └───┬───┘
        │           │
        └─────┬─────┘
              │
    ┌─────────▼─────────┐
    │   PostgreSQL      │
    └───────────────────┘
```

## Deployment Paths

### Local Development (Docker Caddy)

**Files:**
- `deploy/docker/docker-compose.blue-green.yml` - Defines Caddy container
- `deploy/docker/Caddyfile.example` - Docker Caddy configuration

**Setup:**
```bash
# 1. Copy example Caddyfile
cp deploy/docker/Caddyfile.example deploy/docker/Caddyfile

# 2. Start containers with Caddy
docker compose -f docker-compose.yml -f docker-compose.blue-green.yml up -d

# 3. Access via proxy
curl http://localhost/health

# 4. Access slots directly
curl http://localhost:8081/health  # Blue
curl http://localhost:8082/health  # Green
```

**Traffic Switching:**
```bash
# Edit deploy/docker/Caddyfile to change reverse_proxy target
# Then reload:
docker compose -f docker-compose.blue-green.yml exec caddy caddy reload --config /etc/caddy/Caddyfile
```

### Staging/Production (System Caddy)

**Files:**
- `deploy/config/environments/Caddyfile.staging` - Staging configuration
- `deploy/config/environments/Caddyfile.production` - Production configuration
- `deploy/scripts/provision-server.sh` - Installs system Caddy
- `deploy/scripts/install.sh.template` - Deploys Caddyfile
- `deploy/scripts/deploy.sh` - Switches traffic between slots

**Configuration Path:**
- Source: `deploy/config/environments/Caddyfile.{environment}`
- Deployed to: `/etc/caddy/Caddyfile`
- Logs: `/var/log/caddy/{environment}.log`

**Traffic Switching:**
- Automatic during deployment via `deploy.sh` (syncs the environment Caddyfile)
- Manual: Edit `/etc/caddy/Caddyfile` and run `sudo caddy reload --config /etc/caddy/Caddyfile --force`

---

## Installation (Staging/Production)

### Prerequisites

- Ubuntu/Debian server (tested on Ubuntu 22.04+)
- Root or sudo access
- Ports 80 and 443 open to the internet
- DNS A records configured (see DNS Setup section)
- Togather server built and ready to deploy

### DNS Setup

Before installing Caddy, configure DNS:

#### Staging Environment

Add **A record**:
- **Name**: `staging.<your city>`
- **Type**: A
- **Value**: `<your-staging-server-ip>`
- **TTL**: 300 (5 minutes for testing)

#### Production Environment

Add **A record**:
- **Name**: `<city>`
- **Type**: A  
- **Value**: `<your-prod-server-ip>`
- **TTL**: 300 (5 minutes for testing)

#### Verify DNS Resolution

Wait 5-15 minutes for DNS propagation, then test:

```bash
# Test staging
dig staging.<city>.togather.foundation

# Test production
dig <city>.togather.foundation

# Should return your server IP in the ANSWER section
```

### Install Caddy

```bash
# Add Caddy repository
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | \
  sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | \
  sudo tee /etc/apt/sources.list.d/caddy-stable.list

# Install Caddy
sudo apt update
sudo apt install caddy

# Verify installation
caddy version
```

### Deploy Caddyfile

Copy the included Caddyfile to the server:

```bash
# On your development machine
scp deploy/config/environments/Caddyfile.staging user@staging.toronto.togather.foundation:/tmp/

# On the server
sudo mv /tmp/Caddyfile.staging /etc/caddy/Caddyfile
sudo chown root:root /etc/caddy/Caddyfile
sudo chmod 644 /etc/caddy/Caddyfile
```

Or edit directly on the server:

```bash
sudo nano /etc/caddy/Caddyfile
# Paste contents of deploy/config/environments/Caddyfile.{environment}
```

### Create Log Directory

```bash
sudo mkdir -p /var/log/caddy
sudo chown caddy:caddy /var/log/caddy
sudo chmod 755 /var/log/caddy
```

### Configure Firewall

```bash
# Open HTTP and HTTPS ports
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# If the application was directly exposed, remove it
sudo ufw delete allow 8081/tcp
sudo ufw delete allow 8082/tcp

# Verify
sudo ufw status
```

### Validate Configuration

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
```

Should output: `Valid configuration`

### Start Caddy

```bash
# Enable Caddy to start on boot
sudo systemctl enable caddy

# Start Caddy
sudo systemctl start caddy

# Check status
sudo systemctl status caddy
```

### Monitor Certificate Acquisition

Caddy will automatically obtain SSL certificates from Let's Encrypt:

```bash
# Watch Caddy logs for certificate acquisition
sudo journalctl -u caddy -f

# Look for lines like:
# "successfully downloaded available certificate chains"
# "certificate obtained successfully"
```

This typically takes 10-30 seconds if DNS is configured correctly.

### Verify HTTPS

```bash
# Test HTTPS endpoint
curl -I https://staging.toronto.togather.foundation/health

# Should return:
# HTTP/2 200
# (certificate is valid and working)
```

---

## Blue-Green Traffic Switching

### Switching from Blue to Green

1. **Update Caddyfile** for the environment you're deploying:

```bash
sudo nano /etc/caddy/Caddyfile
```

Change:
```caddyfile
# FROM:
reverse_proxy localhost:8081 {
    header_down X-Togather-Slot "blue"
}

# TO:
reverse_proxy localhost:8082 {
    header_down X-Togather-Slot "green"
}
```

2. **Validate configuration**:

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
```

3. **Reload Caddy** (zero downtime):

```bash
sudo caddy reload --config /etc/caddy/Caddyfile --force
```

4. **Verify traffic switch**:

```bash
curl -I https://staging.toronto.togather.foundation/health | grep X-Togather-Slot
# Should show: X-Togather-Slot: green
```

### Switching from Green to Blue

Follow the same process, but reverse the configuration changes.

### Check Which Slot Is Active

```bash
# Via proxy (what users see)
curl -I http://localhost/health | grep X-Togather-Slot

# Or check Caddyfile
grep "reverse_proxy localhost:" /etc/caddy/Caddyfile
```

## Deployment Workflow Integration

The deployment workflow should:

1. Deploy new version to inactive slot (e.g., green on port 8082)
2. Run health checks against the inactive slot directly:
   ```bash
   curl http://localhost:8082/health
   ```
3. If healthy, update Caddyfile to switch traffic
4. Reload Caddy: `sudo caddy reload --config /etc/caddy/Caddyfile --force`
5. Verify traffic is flowing to new slot
6. Keep old slot running for quick rollback if needed

---

## Caddy Reload Methods

### Preferred: Admin API (Synchronous, Reliable)

```bash
sudo caddy reload --config /etc/caddy/Caddyfile --force
```

The `--force` flag tells Caddy to apply the config even if it hasn't changed. This method uses the Caddy admin API at `localhost:2019` and **returns only after the new config is fully applied**.

### Fallback: systemctl reload (Asynchronous, Less Reliable)

```bash
sudo systemctl reload caddy
```

This sends SIGUSR1 to the Caddy process, which triggers a graceful reload. However, the command **returns before Caddy finishes applying the new config**, creating a window where the old config is still active. This was the root cause of intermittent traffic switch failures in blue-green deployments.

**Recommendation**: Always use `caddy reload` with the admin API for deployment automation.

---

## Monitoring

### Check Caddy Status

```bash
# Service status
sudo systemctl status caddy

# Live logs
sudo journalctl -u caddy -f

# Recent logs
sudo journalctl -u caddy -n 100
```

### Check Access Logs

```bash
# Staging logs
sudo tail -f /var/log/caddy/staging.toronto.log

# Production logs
sudo tail -f /var/log/caddy/toronto.log
```

### Check Configuration

```bash
# View running config via admin API
curl http://localhost:2019/config/ | jq
```

### Check Certificate Status

```bash
# Certificate details
echo | openssl s_client -connect staging.toronto.togather.foundation:443 -servername staging.toronto.togather.foundation 2>/dev/null | openssl x509 -noout -dates

# Should show valid dates and issuer (Let's Encrypt)

# List all certificates managed by Caddy
sudo caddy list-certificates
```

---

## Troubleshooting

### DNS Not Resolving

```bash
# Test DNS
dig staging.toronto.togather.foundation

# If no response, check:
# 1. DNS records in Gandi are correct
# 2. Wait 5-15 minutes for propagation
# 3. Try different DNS server: dig @8.8.8.8 staging.toronto.togather.foundation
```

### Certificate Acquisition Fails

```bash
# Check Caddy logs for details
sudo journalctl -u caddy -n 100

# Common issues:
# - "acme: error: 400" - DNS not propagated yet
# - "dial tcp: connection refused" - Caddy can't reach Let's Encrypt
# - "timeout" - Firewall blocking port 80 or 443

# Verify ports are open
sudo ufw status | grep -E '80|443'

# Test from external network
curl -I http://staging.toronto.togather.foundation
```

### SSL Certificate Issues (Production)

```bash
# Check certificate status
sudo caddy list-certificates

# Force certificate renewal
sudo systemctl stop caddy
sudo caddy renew --config /etc/caddy/Caddyfile
sudo systemctl start caddy
```

### Application Connection Refused

```bash
# Check if application is running
curl http://localhost:8081/health
curl http://localhost:8082/health

# Check application logs
# (depends on your systemd service or docker setup)
```

### Config Validation Errors

```bash
# Validate syntax
sudo caddy validate --config /etc/caddy/Caddyfile

# View detailed errors
sudo caddy validate --config /etc/caddy/Caddyfile 2>&1

# Common issues:
# - Syntax errors in Caddyfile
# - Invalid directives
# - Missing closing braces
```

### Caddy Not Running

```bash
# Check status
systemctl status caddy

# View logs
sudo journalctl -u caddy -f

# Restart
sudo systemctl restart caddy
```

### Caddyfile Validation Failed

```bash
# Validate syntax
sudo caddy validate --config /etc/caddy/Caddyfile

# View errors
sudo caddy validate --config /etc/caddy/Caddyfile 2>&1
```

### Health Check Fails

```bash
# Check if slots are running
curl http://localhost:8081/health  # Blue
curl http://localhost:8082/health  # Green

# Check if Caddy can reach them
sudo journalctl -u caddy | tail -50
```

### Reload Not Working

```bash
# Force reload with admin API (preferred)
sudo caddy reload --config /etc/caddy/Caddyfile --force

# If that fails, restart (brief downtime)
sudo systemctl restart caddy

# Check for errors
sudo systemctl status caddy
sudo journalctl -u caddy -n 50
```

---

## Security Considerations

### Automatic Updates

Caddy does not auto-update. Set up periodic updates:

```bash
# Add to crontab (weekly updates)
sudo crontab -e

# Add line:
0 2 * * 0 apt update && apt upgrade -y caddy && systemctl reload caddy
```

### Restricting Metrics Access

Prometheus runs locally in our deployment, so `/metrics` should only be reachable from `127.0.0.1` (or `::1`).
The staging and production Caddyfiles include a localhost-only block for `/metrics`.

If Prometheus moves off-host, replace the localhost allowlist with your private subnet IPs instead.

### Rate Limiting

The included Caddyfile does not include rate limiting (Caddy Enterprise feature). 

Rate limiting is implemented in the Togather application layer. To add additional reverse proxy rate limiting, consider using a Caddy rate limit plugin.

### Monitoring

Set up monitoring for:
- Caddy service status
- Certificate expiry (though Caddy auto-renews)
- Access logs for anomalies
- Application health checks

### Backups

```bash
# Backup Caddyfile
sudo cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.backup

# Store in version control
cp /etc/caddy/Caddyfile ~/togather-server/deploy/config/environments/Caddyfile.production
```

---

## Common Mistakes

### ❌ Trying to use Docker Caddy in production
**Don't do this.** Use system Caddy for staging/production.

### ❌ Editing docker-compose Caddy config for staging
**Wrong location.** Staging uses `/etc/caddy/Caddyfile`, not Docker config.

### ❌ Assuming Caddy is running after provision
**Not guaranteed.** Run `install.sh` to deploy Caddyfile and start Caddy.

### ❌ Forgetting to reload after Caddyfile changes
**Changes don't apply automatically.** Run `sudo caddy reload --config /etc/caddy/Caddyfile --force` after editing.

### ❌ Syncing Caddyfile from repo without preserving active slot
**The repo Caddyfile has a hardcoded default port (blue/8081).** If the live Caddyfile points to green/8082 and you blindly copy from the repo, the port resets. The deploy script handles this by preserving the live port/slot during sync.

### ❌ Using systemctl reload instead of caddy reload
**systemctl reload is asynchronous.** Use `caddy reload --config /etc/caddy/Caddyfile --force` for reliable, synchronous reloads.

---

## Migration from Other Proxies

If migrating from nginx or another reverse proxy:

1. Stop the old proxy service
2. Update firewall rules to ensure Caddy is the only process listening on ports 80/443
3. Deploy Caddyfile as described in Installation section
4. Start Caddy and verify certificate acquisition
5. Test traffic routing to application slots

---

## File Reference

### Docker Caddy (Local Dev)
| File | Purpose |
|------|---------|
| `deploy/docker/docker-compose.blue-green.yml` | Defines Caddy container service |
| `deploy/docker/Caddyfile.example` | Docker Caddy config template |
| `deploy/docker/Caddyfile` | Active Docker Caddy config (git-ignored) |

### System Caddy (Staging/Prod)
| File | Purpose |
|------|---------|
| `deploy/config/environments/Caddyfile.staging` | Staging config source |
| `deploy/config/environments/Caddyfile.production` | Production config source |
| `/etc/caddy/Caddyfile` | Active system Caddy config (on server) |
| `deploy/scripts/provision-server.sh` | Installs system Caddy |
| `deploy/scripts/install.sh.template` | Deploys Caddyfile |
| `deploy/scripts/deploy.sh` | Switches traffic |

---

## Additional Resources

- [Caddy Documentation](https://caddyserver.com/docs/)
- [Caddyfile Syntax](https://caddyserver.com/docs/caddyfile)
- [Automatic HTTPS](https://caddyserver.com/docs/automatic-https)
- [Reverse Proxy Guide](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)

## Related Documentation

- [Deployment Overview](README.md)
- [Quickstart Guide](quickstart.md)
- [Rollback Guide](rollback.md)
- [Linode Deployment](linode-deployment.md)

---

**Last Updated**: 2026-02-12  
**Maintained By**: Togather Infrastructure Team
