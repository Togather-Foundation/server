# Deployment & Operations Documentation

This directory contains guides for deploying, operating, and maintaining Togather SEL server instances in production.

## Audience

These docs are for:
- **Operators** deploying SEL nodes to production
- **DevOps/SRE** managing infrastructure
- **System Administrators** maintaining running instances

For other audiences, see:
- **Developers**: [`docs/contributors/`](../contributors/)
- **API Users**: [`docs/integration/`](../integration/)
- **Federation Partners**: [`docs/interop/`](../interop/)

## Quick Start

New to deployment? Start here:

1. **[Quickstart Guide](quickstart.md)** - End-to-end deployment walkthrough
2. **[CI/CD Integration](ci-cd.md)** - Automate deployments with GitHub Actions, GitLab CI, Jenkins
3. **[Rollback Guide](rollback.md)** - Recover from failed deployments

## Documentation Index

### Essential Guides

- **[Quickstart Guide](quickstart.md)** - Complete deployment setup from scratch
- **[Caddy Deployment](CADDY-DEPLOYMENT.md)** - Production deployment with Caddy reverse proxy and automatic HTTPS
- **[Linode Deployment](LINODE-DEPLOYMENT.md)** - Deploy to Linode cloud platform
- **[Rollback Guide](rollback.md)** - Troubleshoot and recover from failed deployments
- **[Migrations Guide](migrations.md)** - Manage database schema changes safely
- **[Troubleshooting](troubleshooting.md)** - Common issues and solutions

### Operations & Maintenance

- **[Monitoring](monitoring.md)** - Observability setup (Prometheus, Grafana)
- **[Log Management](log-management.md)** - Structured logging and analysis
- **[Performance Testing](performance-testing.md)** - Load testing and benchmarking
- **[Best Practices](best-practices.md)** - Production deployment recommendations

### CI/CD & Automation

- **[CI/CD Integration](ci-cd.md)** - GitHub Actions, GitLab CI, Jenkins examples
- **[Grafana Dashboard Guidelines](grafana-dashboard-guidelines.md)** - Dashboard creation standards

## Deployment Architecture

Togather SEL uses **blue-green zero-downtime deployments**:

```
┌─────────────────────────────────────────┐
│  Reverse Proxy (Caddy/Nginx) :80/:443  │
│    Routes traffic to active slot        │
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
    │   (Shared)        │
    └───────────────────┘
```

**Key Features:**
- ✅ Zero-downtime deployments
- ✅ Automatic database snapshots before migrations
- ✅ Health check validation before traffic switch
- ✅ One-command rollback
- ✅ Multi-environment support (dev/staging/prod)

## CLI Commands

All deployment operations use the `server` CLI:

```bash
# Database Snapshots
server snapshot create --reason "pre-deploy"
server snapshot list
server snapshot cleanup --retention-days 7

# Deployment Operations
server deploy status
server deploy rollback <environment>

# Health Checks
server healthcheck --slot blue
server healthcheck --watch
```

See individual guides for detailed usage.

## Getting Help

1. Check **[Troubleshooting Guide](troubleshooting.md)** for common issues
2. Review deployment logs: `~/.togather/logs/deployments/`
3. Check rollback logs: `~/.togather/logs/rollbacks/`
4. Contact Togather infrastructure team

## Related Documentation

- **Development Setup**: [`docs/contributors/DEVELOPMENT.md`](../contributors/DEVELOPMENT.md)
- **Architecture Overview**: [`docs/contributors/ARCHITECTURE.md`](../contributors/ARCHITECTURE.md)
- **Security Practices**: [`docs/contributors/SECURITY.md`](../contributors/SECURITY.md)
- **API Documentation**: [`docs/integration/API_GUIDE.md`](../integration/API_GUIDE.md)

---

**Last Updated**: 2026-02-01  
**Maintained By**: Togather Infrastructure Team
