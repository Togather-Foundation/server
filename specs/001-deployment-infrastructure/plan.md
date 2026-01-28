# Implementation Plan: Deployment Infrastructure

**Branch**: `001-deployment-infrastructure` | **Date**: 2026-01-28 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-deployment-infrastructure/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Create a DevOps infrastructure and workflow that enables easy deployment of the Togather server (Go backend) to any provider with minimal complexity. The solution prioritizes KISS principles, ease of use, and low maintenance for an open-source community-run project. MVP uses Docker Compose for universal baseline, with blue-green deployment, automatic migrations, and environment-based configuration. Future phases add multi-provider support (AWS/GCP), monitoring (Prometheus/Grafana), and CI/CD integration.

## Technical Context

**Language/Version**: Go 1.25+ (existing codebase)  
**Primary Dependencies**: Docker Compose v2 (orchestration), golang-migrate (migrations), PostgreSQL 16+ with PostGIS/pgvector/pg_trgm  
**Storage**: PostgreSQL database with volume persistence, pg_dump snapshots to filesystem or S3-compatible storage  
**Testing**: Go testing framework (existing), testcontainers for integration tests  
**Target Platform**: Linux servers (cloud VMs, bare metal, developer laptops) - Docker-based universal deployment
**Project Type**: Single backend service (Go server)  
**Performance Goals**: Deployment completes in <5 minutes, migration handling for databases up to 1M events without timeout  
**Constraints**: Zero downtime deployments (99.9% availability during updates), automatic rollback within 2 minutes on failure, 95% deployment success rate  
**Scale/Scope**: Single-node-per-maintainer architecture (operators can manage multiple independent nodes), community-run project with zero budget for commercial services

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Specification Driven Development Principles:**

| Principle | Status | Notes |
|-----------|--------|-------|
| **Observability Over Opacity** | ✅ PASS | CLI deployment commands, structured logging, visible deployment status, all operations inspectable |
| **Simplicity Over Cleverness** | ✅ PASS | Docker Compose baseline (universal, proven), no Kubernetes complexity, straightforward blue-green strategy |
| **Integration Over Isolation** | ✅ PASS | Integration tests with testcontainers (real PostgreSQL), health checks against real services, no mocks for infrastructure |
| **Modularity Over Monoliths** | ✅ PASS | Provider abstraction interface enables pluggable backends (Docker, AWS, GCP), separate configuration schema per provider |

**SEL Backend Requirements:**

| Requirement | Status | Notes |
|-------------|--------|-------|
| **Database Migrations** | ✅ PASS | Uses existing golang-migrate tool, automatic snapshots before migrations, 7-day retention |
| **Zero Downtime** | ✅ PASS | Blue-green deployment strategy ensures 99.9% availability during updates |
| **Build Pipeline Integration** | ✅ PASS | Integrates with existing Makefile targets (`make ci`, `make build`), validates Git commit matches CI tests |
| **Configuration Management** | ✅ PASS | Environment files (.env) for secrets, YAML for deployment config, version-controlled structure |

**Open Source / Community Constraints:**

| Constraint | Status | Notes |
|------------|--------|-------|
| **Zero Budget** | ✅ PASS | No commercial services (Prometheus/Grafana for monitoring, self-hosted only) |
| **Ease of Use** | ✅ PASS | Single-command deployment, automatic infrastructure provisioning, clear error messages |
| **Low Maintenance** | ✅ PASS | Docker Compose requires minimal ongoing management, automatic cleanup of old snapshots, no complex orchestration |

**Overall Status**: ✅ **ALL GATES PASSED** - Proceed to Phase 0

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Deployment Infrastructure (new)
deploy/
├── docker/
│   ├── docker-compose.yml           # Base compose file (app + db)
│   ├── docker-compose.blue-green.yml # Blue-green orchestration
│   ├── docker-compose.monitoring.yml # Prometheus + Grafana (Phase 2)
│   └── Dockerfile                   # Multi-stage build for server
├── config/
│   ├── deployment.yml               # Provider-agnostic settings
│   ├── environments/
│   │   ├── .env.development.example # Template for dev environment
│   │   ├── .env.staging.example     # Template for staging environment
│   │   └── .env.production.example  # Template for production environment
│   └── providers/
│       ├── docker.yml               # Docker-specific overrides
│       ├── aws.yml                  # AWS ECS/Fargate config (Phase 2)
│       └── gcp.yml                  # GCP Cloud Run config (Phase 2)
├── monitoring/                      # Phase 2: Observability stack
│   ├── prometheus/
│   │   ├── prometheus.yml           # Scrape config
│   │   └── alerts.yml               # Alert rules
│   ├── grafana/
│   │   ├── dashboards/              # Pre-built JSON dashboards
│   │   └── datasources.yml          # Prometheus datasource config
│   └── alertmanager/
│       └── alertmanager.yml         # ntfy.sh + webhook routing
└── scripts/
    ├── deploy.sh                    # Main deployment orchestrator
    ├── rollback.sh                  # Rollback to previous version
    ├── health-check.sh              # Post-deployment validation
    ├── snapshot-db.sh               # pg_dump with retention logic
    └── cleanup.sh                   # Remove old deployments/snapshots

# Existing project structure (unchanged)
cmd/server/                          # Server binary entrypoint
internal/                            # Application code
├── api/                             # HTTP handlers
├── domain/                          # Business logic
├── storage/postgres/migrations/     # Database migrations (used by deploy)
└── ...
tests/integration/                   # Integration tests (pre-deployment validation)
Makefile                             # Build targets (used by deploy)
```

**Structure Decision**: All deployment infrastructure lives under `deploy/` directory to isolate infrastructure-as-code from application code. Docker Compose provides the MVP baseline with clear upgrade paths to cloud-native providers (AWS/GCP configs prepared but not implemented until Phase 2). Monitoring stack is conditionally deployed based on environment configuration.

**Key Design Choices:**
- **`deploy/docker/`**: Contains all Docker Compose orchestration files. Separate files for base, blue-green, and monitoring enable composing features as needed.
- **`deploy/config/`**: Provider-agnostic configuration (`deployment.yml`) plus provider-specific overrides. Environment templates (`.env.*.example`) provide clear guidance for operators.
- **`deploy/scripts/`**: Bash scripts handle deployment orchestration. Kept simple and readable (KISS principle). Each script has a single responsibility.
- **`deploy/monitoring/`**: Self-contained observability stack (Phase 2). Pre-built Grafana dashboards and Prometheus alert rules enable zero-configuration monitoring.

## Complexity Tracking

> **No constitutional violations requiring justification.**

This feature adheres to all constitutional principles:
- **Simplicity**: Docker Compose instead of Kubernetes, bash scripts instead of complex orchestration
- **Observability**: All operations logged, CLI-driven, visible deployment status
- **Modularity**: Provider abstraction enables future expansion without breaking existing deployments
- **Integration Testing**: Validates against real infrastructure (testcontainers, actual Docker daemon)

---

## Post-Design Constitution Check

*Re-evaluation after Phase 1 design completion*

### Design Artifacts Review

**Artifacts Generated:**
- `research.md`: Comprehensive technical research resolving all unknowns (11 key decisions documented)
- `data-model.md`: Entity definitions with schemas, relationships, validation rules
- `contracts/`: API schemas (health check, deployment scripts CLI interface)
- `quickstart.md`: Operator guide for first deployment with troubleshooting

### Constitutional Compliance: CONFIRMED ✅

**Post-Design Validation:**

1. **Observability Over Opacity**: ✅ PASS
   - Deployment scripts output human-readable progress logs
   - Structured JSON logs for automation/audit trails
   - Health check endpoint exposes granular component status
   - Deployment history tracked in JSON state files

2. **Simplicity Over Cleverness**: ✅ PASS
   - Docker Compose chosen over Kubernetes (simpler, works everywhere)
   - Bash scripts for deployment orchestration (no complex frameworks)
   - Filesystem locks instead of distributed coordination (Redis/etcd)
   - Environment files instead of cloud secret managers (MVP)

3. **Integration Over Isolation**: ✅ PASS
   - Health checks validate real database connectivity
   - Migration testing against actual PostgreSQL (not mocks)
   - Deployment scripts tested on real Docker daemon
   - Testcontainers for integration tests

4. **Modularity Over Monoliths**: ✅ PASS
   - Provider abstraction interface designed for pluggability
   - Separate configuration files (deployment.yml + .env per environment)
   - Monitoring stack optional and independently deployable
   - Scripts follow single-responsibility principle

### Design Quality Gates: PASSED ✅

| Gate | Status | Evidence |
|------|--------|----------|
| **KISS Principle** | ✅ PASS | No unnecessary complexity; Docker Compose universally understood |
| **Future-Proof** | ✅ PASS | Provider abstraction enables AWS/GCP expansion without breaking changes |
| **Zero Budget** | ✅ PASS | All tools open-source (Docker, Prometheus, Grafana, golang-migrate) |
| **Ease of Use** | ✅ PASS | Single-command deployment, interactive error messages, quickstart guide |
| **Low Maintenance** | ✅ PASS | Automatic cleanup of old artifacts, self-documenting scripts, minimal operational overhead |

### Risk Assessment

**Low Risk:**
- Docker Compose is mature, stable technology
- golang-migrate already used in project (no new dependencies)
- Bash scripts are simple, auditable, and portable

**Mitigated Risks:**
- **Concurrent deployment**: Filesystem locks prevent conflicts
- **Migration failures**: Automatic snapshots enable rollback
- **Health check timeouts**: Configurable retry limits + rollback on failure

### Complexity Audit: NO VIOLATIONS

**Complexity Score: LOW** (5/10)

| Component | Complexity | Justification |
|-----------|-----------|---------------|
| Bash scripts | Low | Standard Unix tools, no magic |
| Docker Compose | Low | Industry standard, extensive documentation |
| Migration strategy | Low | Using existing golang-migrate tool |
| Configuration management | Low | YAML + .env files (human-readable) |
| Monitoring setup | Medium | Prometheus/Grafana require configuration but well-documented |

**No unjustified complexity introduced.**

### Recommendation: APPROVED FOR IMPLEMENTATION ✅

All constitutional gates passed. Design is simple, maintainable, and meets all MVP requirements. Ready to proceed to Phase 2 (Task Generation).

**Next Command**: `/speckit.tasks` to generate implementation task breakdown.

---

**Plan Status**: Phase 1 Complete  
**Design Review Date**: 2026-01-28  
**Artifacts Location**: `specs/001-deployment-infrastructure/`
