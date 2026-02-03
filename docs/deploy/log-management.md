# Log Management and Rotation

Comprehensive guide for managing and rotating Togather server logs to prevent disk exhaustion while maintaining adequate history for debugging and compliance.

## Table of Contents

1. [Overview](#overview)
2. [Log Locations](#log-locations)
3. [Logrotate Setup](#logrotate-setup)
4. [Rotation Policies](#rotation-policies)
5. [Manual Rotation](#manual-rotation)
6. [Monitoring Disk Usage](#monitoring-disk-usage)
7. [Log Retention Best Practices](#log-retention-best-practices)
8. [Troubleshooting](#troubleshooting)

---

## Overview

Togather deployment system generates several types of logs:
- **Deployment logs**: Records of deployment executions
- **Rollback logs**: Records of rollback operations
- **Snapshot logs**: Database backup operations
- **Migration logs**: Database schema changes
- **Application logs**: Runtime application output
- **Health check logs**: Health endpoint monitoring

Without rotation, these logs can fill disk space and cause deployment failures.

**Recommendation**: Configure logrotate to automatically manage log files.

---

## Log Locations

### System Logs (Root Owned)

```bash
/var/log/togather/
├── deployments/        # Deployment and rollback logs
│   ├── deploy_*.log
│   └── rollback_*.log
├── db-snapshots/       # Database snapshot logs
│   ├── snapshot_*.log
│   └── restore_*.log
├── migrations/         # Migration execution logs
│   └── migration_*.log
└── health/            # Health check logs (optional)
    └── health_*.log
```

### User Logs (User Owned)

```bash
~/.togather/logs/
├── deployments/        # User-initiated deployment logs
│   └── deploy_*.log
└── app.log            # Application runtime logs (if file-based)
```

### Docker Logs

```bash
/var/lib/docker/containers/
└── <container-id>/
    └── <container-id>-json.log  # Container stdout/stderr
```

---

## Logrotate Setup

### Installation

1. **Copy logrotate configuration:**
   ```bash
   sudo cp deploy/config/logrotate.conf /etc/logrotate.d/togather
   sudo chmod 644 /etc/logrotate.d/togather
   ```

2. **Create log directories:**
   ```bash
   sudo mkdir -p /var/log/togather/{deployments,db-snapshots,migrations,health}
   sudo chown -R root:togather /var/log/togather
   sudo chmod 750 /var/log/togather
   sudo chmod 770 /var/log/togather/*
   ```

3. **Create togather group (if it doesn't exist):**
   ```bash
   sudo groupadd togather
   sudo usermod -aG togather $USER
   ```

4. **Verify configuration:**
   ```bash
   sudo logrotate -d /etc/logrotate.d/togather
   ```
   
   Output should show:
   ```
   reading config file /etc/logrotate.d/togather
   Handling 6 logs
   ...
   ```

5. **Test rotation (dry run):**
   ```bash
   sudo logrotate -v -f /etc/logrotate.d/togather
   ```

### Automatic Rotation

Logrotate runs automatically via cron:
- **Ubuntu/Debian**: `/etc/cron.daily/logrotate`
- **Runs**: Daily at ~6:25 AM (system dependent)
- **Logs**: `/var/log/logrotate.log`

Check if logrotate is scheduled:
```bash
ls -l /etc/cron.daily/logrotate
cat /etc/cron.daily/logrotate
```

---

## Rotation Policies

### Deployment Logs

**Location**: `~/.togather/logs/deployments/*.log`

**Policy**:
- **Frequency**: Daily
- **Retention**: 90 days
- **Compression**: Yes (delayed)
- **Max age**: 90 days

**Rationale**: Deployment logs are critical for audit trail and troubleshooting. 90 days provides adequate history for compliance and incident investigation.

**Example rotation**:
```bash
deploy_20260130.log         # Today (uncompressed)
deploy_20260129.log         # Yesterday (uncompressed)
deploy_20260128.log.gz      # 2 days ago (compressed)
...
deploy_20251031.log.gz      # 90 days ago (will be deleted next rotation)
```

---

### Database Snapshot Logs

**Location**: `/var/log/togather/db-snapshots/*.log`

**Policy**:
- **Frequency**: Weekly
- **Retention**: 12 weeks (3 months)
- **Compression**: Yes (delayed)
- **Max age**: 90 days

**Rationale**: Snapshot logs are less frequently accessed. Weekly rotation reduces overhead while maintaining adequate history.

---

### Application Logs

**Location**: `~/.togather/logs/*.log`

**Policy**:
- **Frequency**: Daily
- **Retention**: 30 days
- **Compression**: Yes (delayed)
- **Size threshold**: 100MB (rotate early if exceeded)
- **Max age**: 30 days

**Rationale**: Application logs can be large. 30 days balances disk usage with debugging needs. Size-based rotation prevents individual files from growing too large.

---

### Docker Container Logs

**Location**: `/var/lib/docker/containers/*/*.log`

**Policy**:
- **Frequency**: Daily
- **Retention**: 7 days
- **Compression**: Yes (delayed)
- **Size threshold**: 100MB
- **Max age**: 7 days

**Rationale**: Docker logs can grow very quickly. Short retention period conserves disk space. Prefer structured logging to stdout with Docker's built-in log drivers.

**Alternative**: Configure Docker log driver for automatic rotation:

```bash
# Edit /etc/docker/daemon.json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m",
    "max-file": "3",
    "compress": "true"
  }
}

# Restart Docker
sudo systemctl restart docker
```

---

### Migration Logs

**Location**: `/var/log/togather/migrations/*.log`

**Policy**:
- **Frequency**: Weekly
- **Retention**: 12 weeks
- **Compression**: Yes (delayed)
- **Max age**: 90 days

**Rationale**: Migration logs are important for database change history but generated infrequently.

---

### Health Check Logs

**Location**: `/var/log/togather/health/*.log`

**Policy**:
- **Frequency**: Daily
- **Retention**: 7 days
- **Compression**: Yes (delayed)
- **Max age**: 7 days

**Rationale**: Health checks generate high-volume logs. Short retention is adequate since issues are usually detected quickly.

---

## Manual Rotation

### Force Immediate Rotation

```bash
# Rotate all Togather logs now
sudo logrotate -f /etc/logrotate.d/togather

# Verbose output (useful for debugging)
sudo logrotate -v -f /etc/logrotate.d/togather

# Debug mode (show what would happen, don't actually rotate)
sudo logrotate -d /etc/logrotate.d/togather
```

### Rotate Specific Log Type

```bash
# Create temporary logrotate config for specific logs
cat > /tmp/rotate-deployments.conf <<EOF
~/.togather/logs/deployments/*.log {
    daily
    rotate 90
    compress
    delaycompress
    missingok
    notifempty
}
EOF

# Rotate
sudo logrotate -f /tmp/rotate-deployments.conf
```

### Emergency: Delete Old Logs Manually

```bash
# Delete logs older than 90 days
find /var/log/togather/ -name "*.log*" -mtime +90 -delete

# Delete compressed logs older than 30 days
find /var/log/togather/ -name "*.gz" -mtime +30 -delete

# Delete logs larger than 1GB
find /var/log/togather/ -name "*.log" -size +1G -delete
```

**Warning**: Manual deletion bypasses logrotate. Use only in emergencies.

---

## Monitoring Disk Usage

### Check Log Directory Sizes

```bash
# Total size of all Togather logs
du -sh /var/log/togather/

# Size by log type
du -sh /var/log/togather/*/

# Largest log files
du -h /var/log/togather/*/*.log* | sort -rh | head -10

# Number of log files by type
find /var/log/togather/ -name "*.log*" | wc -l
```

### Set Up Disk Usage Alerts

**Option 1: Cron job with email alert**

```bash
# Add to crontab
crontab -e

# Check disk usage daily at 8 AM
0 8 * * * /usr/bin/du -sh /var/log/togather/ | awk '$1 ~ /G$/ {if ($1+0 > 10) print "WARNING: Togather logs using " $1}' | mail -s "Togather Log Disk Usage Alert" ops@example.com
```

**Option 2: systemd timer + script**

Create `/usr/local/bin/check-togather-logs.sh`:

```bash
#!/bin/bash
THRESHOLD_GB=10
CURRENT_GB=$(du -s /var/log/togather/ | awk '{print int($1/1024/1024)}')

if [ "$CURRENT_GB" -gt "$THRESHOLD_GB" ]; then
    echo "WARNING: Togather logs using ${CURRENT_GB}GB (threshold: ${THRESHOLD_GB}GB)"
    exit 1
fi
```

**Option 3: Monitoring tool integration**

```bash
# Prometheus node_exporter (if using Prometheus)
# Add to monitoring queries:
# node_filesystem_avail_bytes{mountpoint="/var/log"}

# Telegraf (if using InfluxDB)
[[inputs.disk]]
  mount_points = ["/var/log"]

# Datadog (if using Datadog)
# Add to datadog.yaml:
# logs:
#   - type: file
#     path: /var/log/togather/**/*.log
#     service: togather
```

---

## Log Retention Best Practices

### Development Environment

**Recommendation**: Short retention, aggressive rotation

```
Deployment logs: 7 days
Snapshot logs: 7 days
Application logs: 7 days
Docker logs: 3 days
```

**Rationale**: Development generates many deployments. Conserve disk space.

---

### Staging Environment

**Recommendation**: Medium retention

```
Deployment logs: 30 days
Snapshot logs: 30 days
Application logs: 30 days
Docker logs: 7 days
```

**Rationale**: Staging mimics production. Adequate retention for testing and validation.

---

### Production Environment

**Recommendation**: Long retention for compliance

```
Deployment logs: 90 days (or longer for regulatory compliance)
Snapshot logs: 90 days (aligned with snapshot retention)
Application logs: 30 days (or ship to log aggregation service)
Docker logs: 7 days (or use log driver with external storage)
Migration logs: 90 days (important for audit trail)
Health logs: 7 days (high volume, short-lived usefulness)
```

**Rationale**: Production requires longer retention for:
- Incident investigation
- Compliance/audit requirements
- Trend analysis
- Post-mortem debugging

**For regulated industries**: Consider shipping logs to WORM (Write Once Read Many) storage or log aggregation service (Splunk, ELK, Datadog, etc.) for indefinite retention.

---

### Archiving Old Logs

For compliance requiring >90 days retention:

```bash
#!/bin/bash
# Archive logs to S3 before deletion
# Run before logrotate deletes old logs

ARCHIVE_DATE=$(date -d '85 days ago' +%Y%m%d)
BUCKET="s3://your-bucket/togather-logs/"

# Find logs about to be deleted (85-90 days old)
find /var/log/togather/ -name "*.log.gz" -mtime +85 -mtime -90 | while read -r logfile; do
    aws s3 cp "$logfile" "${BUCKET}$(basename "$logfile")"
done
```

Add to cron:
```bash
# Archive logs weekly at 2 AM Sunday
0 2 * * 0 /usr/local/bin/archive-togather-logs.sh
```

---

## Troubleshooting

### Issue: Logs Not Rotating

**Symptom**: Old log files still present after expected rotation time

**Diagnosis**:
```bash
# Check if logrotate is running
sudo journalctl -u cron | grep logrotate

# Check logrotate status
sudo cat /var/lib/logrotate/status

# Check for errors
sudo cat /var/log/logrotate.log | grep -i error
```

**Common Causes**:

1. **Logrotate config syntax error**
   ```bash
   sudo logrotate -d /etc/logrotate.d/togather
   # Look for errors in output
   ```

2. **File permissions issue**
   ```bash
   # Check permissions
   ls -la /var/log/togather/
   
   # Fix if needed
   sudo chown -R root:togather /var/log/togather/
   sudo chmod 750 /var/log/togather/
   sudo chmod 770 /var/log/togather/*
   ```

3. **Cron not running**
   ```bash
   # Check cron status
   sudo systemctl status cron
   
   # Restart if needed
   sudo systemctl restart cron
   ```

4. **Log files locked by process**
   ```bash
   # Check if processes have files open
   lsof ~/.togather/logs/deployments/*.log
   
   # Use copytruncate option in logrotate config (already enabled)
   ```

---

### Issue: Disk Space Still Running Out

**Symptom**: `/var/log` partition filling up despite logrotate configured

**Diagnosis**:
```bash
# Find largest files
sudo du -h /var/log/ | sort -rh | head -20

# Find unrotated logs
find /var/log/togather/ -name "*.log" -size +100M
```

**Solutions**:

1. **Compress uncompressed logs immediately**
   ```bash
   find /var/log/togather/ -name "*.log" -mtime +1 -exec gzip {} \;
   ```

2. **Delete very old logs manually**
   ```bash
   # Delete logs older than 180 days
   find /var/log/togather/ -name "*.log.gz" -mtime +180 -delete
   ```

3. **Ship logs to external storage**
   ```bash
   # Example: rsync to remote server
   rsync -avz --remove-source-files /var/log/togather/ backup-server:/mnt/logs/togather/
   ```

4. **Increase log partition size**
   ```bash
   # Resize /var/log partition (requires downtime)
   # Or mount separate disk for /var/log/togather
   ```

---

### Issue: Missing Log Files After Rotation

**Symptom**: Cannot find expected log files after rotation

**Diagnosis**:
```bash
# Check rotated logs with date suffix
ls -lh ~/.togather/logs/deployments/*.log*

# Check compressed logs
ls -lh ~/.togather/logs/deployments/*.gz

# Check logrotate history
sudo cat /var/lib/logrotate/status | grep togather
```

**Solution**:
Logs are renamed with date suffix and compressed:

```bash
# Original: deploy_abc123.log
# After rotation: deploy_abc123.log-20260130.gz

# View rotated/compressed logs
zcat ~/.togather/logs/deployments/deploy_abc123.log-20260130.gz | less
zgrep "ERROR" ~/.togather/logs/deployments/deploy_*.log*.gz
```

---

### Issue: Logrotate Permission Denied

**Symptom**:
```
error: error creating output file ~/.togather/logs/deployments/deploy.log.1: Permission denied
```

**Solution**:
```bash
# Ensure togather group exists
sudo groupadd togather

# Fix ownership
sudo chown -R root:togather /var/log/togather/
chmod 770 ~/.togather/logs/deployments/

# Add current user to togather group
sudo usermod -aG togather $USER

# Apply new group (logout/login or use newgrp)
newgrp togather
```

---

## Related Documentation

- **Deployment Guide**: [quickstart.md](./quickstart.md)
- **Troubleshooting**: [troubleshooting.md](./troubleshooting.md)
- **Best Practices**: [best-practices.md](./best-practices.md)
- **Logrotate Man Page**: `man logrotate`
- **Logrotate Config**: `deploy/config/logrotate.conf`

---

**Last Updated**: 2026-01-30  
**Applies To**: Togather Server Deployment System Phase 1
