# Caddy Deployment Guide

This guide covers deploying Togather SEL server behind Caddy reverse proxy with automatic HTTPS and blue-green deployments.

## Why Caddy?

- **Automatic HTTPS**: Obtains and renews SSL certificates automatically via Let's Encrypt
- **Simple Configuration**: Human-readable Caddyfile format
- **Modern Defaults**: HTTP/2, HTTPS redirects, and security headers out of the box
- **Zero Downtime**: Graceful config reloads without dropping connections

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

## Prerequisites

- Ubuntu/Debian server (tested on Ubuntu 22.04+)
- Root or sudo access
- Ports 80 and 443 open to the internet
- DNS A records configured (see DNS Setup section)
- Togather server built and ready to deploy

## DNS Setup

Before installing Caddy, configure DNS:

### Staging Environment

Add **A record**:
- **Name**: `staging.<your city>`
- **Type**: A
- **Value**: `<your-staging-server-ip>`
- **TTL**: 300 (5 minutes for testing)

### Production Environment

Add **A record**:
- **Name**: `toronto`
- **Type**: A  
- **Value**: `<your-prod-server-ip>`
- **TTL**: 300 (5 minutes for testing)

### Verify DNS Resolution

Wait 5-15 minutes for DNS propagation, then test:

```bash
# Test staging
dig staging.<city>.togather.foundation

# Test production
dig <city>.togather.foundation

# Should return your server IP in the ANSWER section
```

## Installation

### 1. Install Caddy

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

### 2. Deploy Caddyfile

Copy the included Caddyfile to the server:

```bash
# On your development machine
scp deploy/Caddyfile user@staging.toronto.togather.foundation:/tmp/

# On the server
sudo mv /tmp/Caddyfile /etc/caddy/Caddyfile
sudo chown root:root /etc/caddy/Caddyfile
sudo chmod 644 /etc/caddy/Caddyfile
```

Or edit directly on the server:

```bash
sudo nano /etc/caddy/Caddyfile
# Paste contents of deploy/Caddyfile
```

### 3. Create Log Directory

```bash
sudo mkdir -p /var/log/caddy
sudo chown caddy:caddy /var/log/caddy
sudo chmod 755 /var/log/caddy
```

### 4. Configure Firewall

```bash
# Open HTTP and HTTPS ports
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# If the application was directly exposed, remove it
sudo ufw delete allow 8080/tcp
sudo ufw delete allow 8081/tcp
sudo ufw delete allow 8082/tcp

# Verify
sudo ufw status
```

### 5. Validate Configuration

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
```

Should output: `Valid configuration`

### 6. Start Caddy

```bash
# Enable Caddy to start on boot
sudo systemctl enable caddy

# Start Caddy
sudo systemctl start caddy

# Check status
sudo systemctl status caddy
```

### 7. Monitor Certificate Acquisition

Caddy will automatically obtain SSL certificates from Let's Encrypt:

```bash
# Watch Caddy logs for certificate acquisition
sudo journalctl -u caddy -f

# Look for lines like:
# "successfully downloaded available certificate chains"
# "certificate obtained successfully"
```

This typically takes 10-30 seconds if DNS is configured correctly.

### 8. Verify HTTPS

```bash
# Test HTTPS endpoint
curl -I https://staging.toronto.togather.foundation/health

# Should return:
# HTTP/2 200
# (certificate is valid and working)
```

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
sudo systemctl reload caddy
```

4. **Verify traffic switch**:

```bash
curl -I https://staging.toronto.togather.foundation/health | grep X-Togather-Slot
# Should show: X-Togather-Slot: green
```

### Switching from Green to Blue

Follow the same process, but reverse the configuration changes.

## Deployment Workflow Integration

The deployment workflow should:

1. Deploy new version to inactive slot (e.g., green on port 8082)
2. Run health checks against the inactive slot directly:
   ```bash
   curl http://localhost:8082/health
   ```
3. If healthy, update Caddyfile to switch traffic
4. Reload Caddy: `sudo systemctl reload caddy`
5. Verify traffic is flowing to new slot
6. Keep old slot running for quick rollback if needed

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
```

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
# Validate and show detailed errors
sudo caddy validate --config /etc/caddy/Caddyfile

# Common issues:
# - Syntax errors in Caddyfile
# - Invalid directives
# - Missing closing braces
```

### Reload Not Working

```bash
# Force reload
sudo systemctl reload caddy

# If that fails, restart (brief downtime)
sudo systemctl restart caddy

# Check for errors
sudo systemctl status caddy
sudo journalctl -u caddy -n 50
```

## Security Considerations

### Automatic Updates

Caddy does not auto-update. Set up periodic updates:

```bash
# Add to crontab (weekly updates)
sudo crontab -e

# Add line:
0 2 * * 0 apt update && apt upgrade -y caddy && systemctl reload caddy
```

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
cp /etc/caddy/Caddyfile ~/togather-server/deploy/Caddyfile.production
```

## Migration from Other Proxies

If migrating from another reverse proxy, update your firewall rules and ensure Caddy is the only process listening on ports 80/443.

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

**Last Updated**: 2026-02-02  
**Maintained By**: Togather Infrastructure Team
