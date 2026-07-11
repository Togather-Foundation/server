# Server Maintenance

Automated disk space management for Togather SEL nodes. All maintenance runs via
systemd timers and cron — no manual intervention required after provisioning.

## Automated Maintenance

Three systemd timers and one cron-based job are installed during `provision-server.sh`:

| Timer | Schedule | What it does |
|---|---|---|
| `togather-docker-prune.timer` | Weekly | Removes Docker images older than 48h and build cache |
| `containerd-pid-cleanup.timer` | Hourly | Deletes stale containerd exec PID files from `/run` tmpfs |
| `togather-prometheus-cleanup.timer` | Weekly (Sat 03:00) | Stops Prometheus, clears WAL + chunks_head, restarts |
| `logrotate` (system) | Daily | Rotates Togather logs per `/etc/logrotate.d/togather` |

Journald is capped at 500 MB via `/etc/systemd/journald.conf.d/togather.conf`.

### Check status

```bash
systemctl list-timers | grep -E 'togather|containerd'
systemctl status togather-docker-prune.timer
systemctl status containerd-pid-cleanup.timer
```

### Trigger a run immediately

```bash
sudo systemctl start togather-docker-prune.service
sudo systemctl start containerd-pid-cleanup.service
```

### View logs

```bash
# Docker prune output
journalctl -u togather-docker-prune.service -n 20

# Containerd PID cleanup errors (usually silent)
journalctl -u containerd-pid-cleanup.service -n 10
```

## Log Rotation

Config: `/etc/logrotate.d/togather` (installed from `deploy/config/logrotate.conf`).

| Log category | Rotation | Retention |
|---|---|---|
| Deployment logs | daily | 90 days |
| DB snapshot logs | weekly | 12 weeks |
| Application logs | daily | 30 days |
| Docker container logs | daily | 7 days |
| Health check logs | daily | 7 days |
| Migration logs | weekly | 12 weeks |

Files rotate on size trigger (100 MB for app and Docker logs) or time, whichever
comes first.

```bash
sudo logrotate -d /etc/logrotate.d/togather   # dry-run
sudo logrotate -f /etc/logrotate.d/togather   # force rotation
```

## Docker Prune

The weekly timer runs:
```
docker system prune -a -f --filter "until=48h"
docker builder prune -f
```

This keeps images from the last 48 hours (safe for rollback) and removes all
dangling build cache. The 48h window means a deploy creates a new image and
the old one survives long enough for a rollback.

`deploy.sh` also runs Docker pruning before each deploy (standard: 24h filter;
aggressive if <10 GB free). This is a safety net, not a replacement for the timer.

## Journald Limits

```bash
cat /etc/systemd/journald.conf.d/togather.conf
# [Journal]
# SystemMaxUse=500M
# MaxFileSec=14day
```

500 MB is generous for debugging while preventing the journal from consuming the
entire disk. Individual journal files rotate after 14 days.

## Containerd PID Cleanup

Containerd (Docker's runtime) leaks `.pid` files in `/run/containerd/...` from
health check exec processes. The hourly timer deletes files older than 5 minutes,
excluding `init.pid` (the container's main process).

Without this timer, a `/run` tmpfs (197 MB) fills in ~7-8 weeks from health checks
alone. See [troubleshooting.md](./troubleshooting.md#issue-run-tmpfs-full--docker-cannot-start-containers)
for details.

## Prometheus WAL Cleanup

Prometheus `--storage.tsdb.retention.size` only caps compacted TSDB blocks — the
write-ahead log (WAL) can grow unbounded if the disk fills before compaction runs.
When the disk is full, Prometheus can't compact, and the WAL enters a
self-reinforcing growth loop (cascade outage — see
[troubleshooting.md](./troubleshooting.md#issue-prometheus-wal-fills-disk-cascade-outage)).

The weekly timer stops Prometheus, clears the WAL and in-memory chunks, then
restarts it. This loses up to 2 hours of metrics (the compaction window) but is a
safety valve against disk exhaustion. Prometheus retention is configured at
7 days / 5 GB in `docker-compose.blue-green.yml`.

```bash
systemctl status togather-prometheus-cleanup.timer
systemctl start togather-prometheus-cleanup.service   # trigger immediately
```

## Manual Cleanup

The `server cleanup` CLI wraps `deploy/scripts/cleanup.sh` for targeted cleanup:

```bash
server cleanup --dry-run                    # preview
server cleanup --force                      # interactive skip
server cleanup --keep-images 2              # keep only 2 most recent images
server cleanup --snapshots-only --keep-snapshots 14
server cleanup --logs-only --keep-logs-days 60
```

## Database-Level Cleanup (River Jobs)

Daily River jobs handle DB-level cleanup — these run inside the application,
not as systemd timers:

| Job | Retention |
|---|---|
| Idempotency key cleanup | 24 hours |
| Batch results cleanup | 7 days |
| Review queue cleanup | 7 days (rejected), event start (unreviewed), 90 days (archived) |
| Submissions cleanup | 90 days |
| Geocoding cache cleanup | 30 days |
| Usage rollup | Daily |

## Disk Space Monitoring

```bash
df -h /
docker system df
sudo journalctl --disk-usage
```

If disk usage exceeds 90%, check for:
- **Docker build cache bloat:** `docker system prune -a -f`
- **Prometheus WAL:** see [troubleshooting.md](./troubleshooting.md#issue-prometheus-wal-fills-disk-cascade-outage)
- **Journal bloat:** `sudo journalctl --vacuum-size=500M`
- **Large log files:** `find /var/log -type f -size +100M`

## Adding Maintenance to an Existing Server

If the server was provisioned before the `configure_maintenance` step was added
to `provision-server.sh`, run these steps manually:

```bash
# Logrotate
sudo cp deploy/config/logrotate.conf /etc/logrotate.d/togather
sudo chmod 644 /etc/logrotate.d/togather
sudo mkdir -p /var/log/togather/{deployments,db-snapshots,health,migrations}

# Containerd PID cleanup
sudo cp deploy/config/containerd-pid-cleanup.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now containerd-pid-cleanup.timer

# Docker prune
sudo cp deploy/config/togather-docker-prune.{service,timer} /etc/systemd/system/

# Prometheus WAL cleanup
sudo cp deploy/config/togather-prometheus-cleanup.{service,timer} /etc/systemd/system/

sudo systemctl daemon-reload
sudo systemctl enable --now togather-docker-prune.timer
sudo systemctl enable --now togather-prometheus-cleanup.timer

# Journald
sudo ./deploy/config/journald-configure.sh
```
