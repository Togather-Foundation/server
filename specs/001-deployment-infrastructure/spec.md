# Feature Specification: Deployment Infrastructure

**Feature Branch**: `001-deployment-infrastructure`  
**Created**: 2026-01-28  
**Status**: Draft  
**Input**: User description: "Create a the devops infrastructure and workflow that can easily deploy this project to a server. It should be easy to deploy to different providers."

## Clarifications

### Session 2026-01-28

- Q: Zero-downtime deployment strategy given single-node-per-maintainer architecture? → A: Full blue-green deployment for all deployments (provides zero-downtime, works for operators managing multiple nodes)
- Q: Secrets management implementation priority (environment files vs cloud secret managers)? → A: Start with environment files for all environments; cloud secret manager integration as future enhancement
- Q: Alert channel implementation priority for deployment notifications? → A: Simple logging output with optional generic webhook support plus ntfy.sh integration
- Q: Database backup strategy and retention for migration safety? → A: Automatic snapshot before migrations with 7-day retention and automatic cleanup

## MVP vs Future Enhancements

This section defines what must be built for MVP (Minimum Viable Product) versus what can be added later. All components are designed to be future-proof, with clear extension points for enhancements.

### MVP Scope (Must Have)

**Core Deployment Workflow:**
- ✅ Single-command deployment (`deploy` command)
- ✅ Docker Compose-based deployment (works on any host with Docker)
- ✅ One provider support initially (Docker-based hosts as universal baseline)
- ✅ Blue-green deployment strategy (build into architecture from day 1)
- ✅ Automatic database migrations before deployment
- ✅ Database snapshot before migrations (7-day retention)
- ✅ Health checks with automatic rollback on failure
- ✅ Environment configuration via .env files
- ✅ Basic rollback command (restore previous version)
- ✅ Deployment logging (stdout/stderr)
- ✅ Version tagging (Git commit SHA + timestamp)
- ✅ Concurrent deployment prevention (lock file mechanism)

**Architecture Requirements:**
- ✅ Pluggable provider system (abstraction layer for future providers)
- ✅ Configuration schema with provider-specific overrides (supports adding AWS/GCP later)
- ✅ Webhook interface for notifications (generic, not service-specific)
- ✅ Secrets management via environment files (with interface for future secret backends)

**Future-Proofing Design:**
- Deployment commands use provider-agnostic CLI interface
- Configuration files separate provider-agnostic settings from provider-specific
- Health check system supports custom check plugins
- Notification system uses event-driven architecture (easy to add channels)

### Phase 2 (Post-MVP)

**Multi-Provider Support:**
- ⏭️ AWS deployment (Terraform module: ECS/Fargate + RDS)
- ⏭️ GCP deployment (Terraform module: Cloud Run + Cloud SQL)
- ⏭️ DigitalOcean App Platform integration
- ⏭️ Bare metal / VPS with pre-installed Docker

**Enhanced Notifications:**
- ⏭️ ntfy.sh integration (simple push notifications)
- ⏭️ Generic webhook with retry logic and payload templates
- ⏭️ Email SMTP support
- ⏭️ Slack/Discord webhooks

**CI/CD Integration:**
- ⏭️ GitHub Actions workflow templates
- ⏭️ Automated deployment on merge to main
- ⏭️ Branch deployments for testing (ephemeral environments)
- ⏭️ Deployment status badges

### Phase 3 (Future Enhancements)

**Advanced Features:**
- ⏭️ Cloud secret manager integration (AWS Secrets Manager, GCP Secret Manager)
- ⏭️ SSL/TLS certificate automation (Let's Encrypt via ACME protocol)
- ⏭️ Centralized logging aggregation (ship logs to external service)
- ⏭️ Performance monitoring integration (response time tracking, alerting)
- ⏭️ Multi-version rollback (specify exact version to restore)
- ⏭️ Deployment approval workflows (require manual confirmation for production)
- ⏭️ Infrastructure drift detection (compare deployed vs configured state)

**Future-Proof Extension Points Built into MVP:**
1. **Provider Abstraction**: Interface defines `provision()`, `deploy()`, `healthCheck()`, `rollback()` - new providers implement this interface
2. **Notification Pipeline**: Event bus for deployment lifecycle events (started, completed, failed) - subscribers can process events independently
3. **Secrets Backend**: Interface for `getSecret(key)` - swap implementation without changing deployment code
4. **Health Check Plugins**: Registry-based system for custom health checks (database, API endpoints, external dependencies)
5. **Configuration Schema**: YAML/TOML with `provider` section for provider-specific overrides - adding providers doesn't break existing configs

### MVP Success Metrics

These must work before moving to Phase 2:
- Deploy to Docker host with single command ✓
- Zero-downtime updates via blue-green ✓
- Automatic migration + snapshot ✓
- Rollback on health check failure ✓
- Deploy to dev/staging/prod with different configs ✓
- Complete deployment in <5 minutes ✓

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One-Command Production Deploy (Priority: P1) **[MVP]**

DevOps engineers and developers can deploy the Togather server to a production environment with a single command, regardless of the cloud provider or hosting platform being used.

**Why this priority**: This is the core value proposition - enabling rapid, reliable deployments without manual configuration or provider-specific knowledge. Every deployment after this must work or the system is broken.

**Independent Test**: Can be fully tested by running the deployment command against a fresh cloud account and verifying the server responds to health check requests, delivers value by allowing immediate production deployment.

**Acceptance Scenarios**:

1. **Given** a configured deployment environment, **When** operator runs the deploy command, **Then** the server binary is built, deployed to the target environment, and passes health checks within 5 minutes
2. **Given** no prior deployment exists, **When** operator runs the deploy command for the first time, **Then** all required infrastructure (database, networking, storage) is automatically provisioned and configured
3. **Given** an existing deployment, **When** operator runs the deploy command with a new version, **Then** the new version is deployed with zero downtime using blue-green deployment strategy

---

### User Story 2 - Multi-Provider Support (Priority: P2) **[Phase 2]**

Operations teams can switch between different hosting providers (AWS, GCP, Azure, DigitalOcean, bare metal servers) using the same deployment workflow and configuration format.

**Why this priority**: Provider flexibility prevents vendor lock-in and enables cost optimization. However, the deployment must work somewhere (P1) before it needs to work everywhere.

**Independent Test**: Can be tested independently by deploying to two different providers using identical configuration files (except provider-specific credentials), delivers value by proving portability.

**Acceptance Scenarios**:

1. **Given** deployment configuration for AWS, **When** operator changes provider to GCP and re-runs deploy, **Then** application deploys successfully to GCP with equivalent functionality
2. **Given** provider-agnostic configuration file, **When** operator specifies different target providers, **Then** deployment adapts infrastructure choices appropriately (managed database vs. self-hosted, load balancer vs. reverse proxy)
3. **Given** a bare metal server with Docker support, **When** operator deploys using container-based workflow, **Then** application runs identically to cloud deployments

---

### User Story 3 - Database Migration Management (Priority: P1) **[MVP]**

Database schema migrations run automatically during deployment, ensuring the database schema matches the application code version without manual intervention.

**Why this priority**: Data persistence is critical for the Shared Events Library. Incorrect schema deployment causes data loss or service outage. Must be part of MVP deployment.

**Independent Test**: Can be tested independently by deploying a version with new migrations and verifying schema changes applied correctly and application starts successfully, delivers value by preventing schema drift.

**Acceptance Scenarios**:

1. **Given** pending database migrations, **When** deployment starts, **Then** migrations run to completion before application servers start handling traffic
2. **Given** a migration failure, **When** migration cannot complete, **Then** deployment rolls back automatically and alerts operators with detailed error information
3. **Given** multiple application instances, **When** deployment runs migrations, **Then** only one instance executes migrations while others wait, preventing race conditions

---

### User Story 4 - Environment Configuration (Priority: P2) **[MVP]**

Operators can manage environment-specific configuration (development, staging, production) through standardized configuration files without modifying code or deployment scripts.

**Why this priority**: Configuration management enables safe testing and gradual rollout. Essential for production readiness but can be handled manually initially.

**Independent Test**: Can be tested independently by deploying to three environments with different configurations and verifying each environment uses its specific settings, delivers value by enabling environment isolation.

**Acceptance Scenarios**:

1. **Given** separate configuration files for dev, staging, and production, **When** operator deploys to staging, **Then** staging-specific values (database URL, API keys, feature flags) are applied
2. **Given** sensitive credentials in configuration, **When** deployment runs, **Then** secrets are never logged or exposed in deployment outputs
3. **Given** a configuration error, **When** deployment validates config, **Then** deployment fails fast with clear error message before any infrastructure changes

---

### User Story 5 - Deployment Rollback (Priority: P2) **[MVP]**

When a deployment causes issues in production, operators can quickly roll back to the previous working version with a single command, restoring service without data loss.

**Why this priority**: Rollback capability is essential for production confidence but assumes at least one successful deployment exists (depends on P1).

**Independent Test**: Can be tested independently by deploying a version, deploying a "broken" version, then rolling back and verifying the original version is restored and functional.

**Acceptance Scenarios**:

1. **Given** a failed deployment in production, **When** operator runs rollback command, **Then** previous application version is restored within 2 minutes
2. **Given** a rollback operation, **When** database migrations were run in the failed deployment, **Then** deployment system warns operator about manual migration rollback requirements
3. **Given** multiple recent deployments, **When** operator specifies a version to rollback to, **Then** system restores that specific version and its associated infrastructure state

---

### User Story 6 - Health Monitoring and Alerting (Priority: P3) **[Phase 2]**

After deployment completes, the system automatically monitors application health and notifies operators if the deployment degraded performance or introduced errors.

**Why this priority**: Post-deployment validation prevents silent failures but is less critical than core deployment functionality. Can be added after basic deployment works.

**Independent Test**: Can be tested independently by deploying a version, simulating health check failures, and verifying alerts are sent to configured channels.

**Acceptance Scenarios**:

1. **Given** a newly deployed version, **When** deployment completes, **Then** system monitors health checks for 10 minutes and reports success/failure statistics
2. **Given** health checks failing after deployment, **When** failure threshold is exceeded, **Then** system sends alerts via configured channels (stdout/stderr logs, generic webhook, ntfy.sh) with deployment details
3. **Given** performance degradation detected, **When** response times increase by more than 50%, **Then** system alerts operators with performance comparison metrics

---

### Edge Cases

- What happens when deployment command runs while another deployment is in progress? **System detects concurrent deployment attempt and exits with error indicating deployment in progress.**
- How does system handle partial infrastructure failures (database provisioned but network configuration fails)? **Deployment fails fast, logs all completed steps for manual cleanup or retry, optionally attempts automatic resource cleanup.**
- What happens when operator's local environment has outdated deployment tooling? **Deployment script checks tool versions on startup and prompts operator to upgrade if incompatible.**
- How does system handle database migration that exceeds timeout during high-load periods? **Migration runs with extended timeout in maintenance mode; deployment optionally schedules migrations during low-traffic windows.**
- What happens when rolling back to a version with incompatible database schema? **System detects schema compatibility issues and requires operator confirmation or manual migration steps.**
- How does system handle provider-specific quota limits or rate limiting during deployment? **Deployment retries with exponential backoff; provides clear error messages if quotas are exhausted.**

## Requirements *(mandatory)*

### Functional Requirements

**MVP Requirements (Phase 1):**
- **FR-001** [MVP]: System MUST support deploying the Go server application with a single command from developer workstation or CI/CD pipeline
- **FR-002** [MVP]: System MUST build the server binary with correct version metadata (Git commit, build date, semantic version) embedded in the binary
- **FR-003** [MVP]: System MUST provision and configure PostgreSQL database with required extensions (PostGIS, pgvector, pg_trgm) on target infrastructure
- **FR-004** [MVP]: System MUST run database migrations automatically before starting application servers during deployment
- **FR-005** [MVP]: System MUST support Docker-based deployment as universal baseline (works on any host with Docker installed)
- **FR-006** [MVP]: System MUST manage environment-specific configuration through environment files (.env format) without requiring code changes
- **FR-007** [MVP]: System MUST support deploying to development, staging, and production environments with environment-specific settings
- **FR-008** [MVP]: System MUST validate configuration files before beginning deployment to catch errors early
- **FR-009** [MVP]: System MUST implement zero-downtime deployments using blue-green deployment strategy (new version deployed alongside old, traffic switches after health checks pass)
- **FR-010** [MVP]: System MUST tag deployed versions with Git commit SHA and deployment timestamp for traceability
- **FR-011** [MVP]: System MUST provide rollback capability to restore previous application version
- **FR-012** [MVP]: System MUST perform health checks after deployment completes to verify application is responding correctly
- **FR-013** [MVP]: System MUST collect and display deployment logs for troubleshooting failures
- **FR-014** [MVP]: System MUST prevent concurrent deployments to the same environment by detecting and blocking simultaneous deploy operations
- **FR-015** [MVP]: System MUST support deploying specific Git branches or tags for testing feature branches or hotfixes
- **FR-016** [MVP]: System MUST expose deployment configuration through version-controlled files in the repository
- **FR-017** [MVP]: System MUST handle secrets securely through environment files without storing them in version control or deployment logs (cloud secret manager integration deferred to Phase 3)
- **FR-023** [MVP]: System MUST support single-node-per-maintainer architecture while allowing operators to manage multiple independent nodes (each node is a separate deployment target)
- **FR-024** [MVP]: System MUST create automatic database snapshot before running migrations with 7-day retention and automatic cleanup to enable recovery from migration failures
- **FR-025** [MVP]: System MUST validate that deployed version matches CI test results (same Git commit that passed tests)

**Post-MVP Requirements (Phase 2):**
- **FR-018** [Phase 2]: System MUST integrate with CI/CD pipelines (GitHub Actions) for automated deployment on merge to main branch
- **FR-019** [Phase 2]: System MUST provide deployment status visibility showing current version, deployment time, and deployer identity
- **FR-022** [Phase 2]: System MUST configure monitoring and alerting for critical application metrics (uptime, response time, error rate) with support for stdout/stderr logging, generic webhooks, and ntfy.sh notifications

**Future Enhancements (Phase 3):**
- **FR-020** [Phase 3]: System MUST configure SSL/TLS certificates automatically for HTTPS endpoints
- **FR-021** [Phase 3]: System MUST set up logging infrastructure to collect application logs centrally

**Provider Expansion (Phase 2+):**
- **FR-005a** [Phase 2]: System MUST support AWS deployment (ECS/Fargate with RDS)
- **FR-005b** [Phase 2]: System MUST support GCP deployment (Cloud Run with Cloud SQL)
- **FR-005c** [Phase 2]: System MUST support DigitalOcean and bare metal servers with Docker

### Key Entities

- **Deployment Configuration**: Represents target environment settings including provider type (AWS/GCP/Docker), region, instance sizing, database configuration, domain names, and environment variables. Stored as version-controlled files in repository.

- **Deployment Target**: Represents a specific environment (development, staging, production) or independent node with its unique infrastructure resources (server, database), current deployed version, and deployment history. Architecture supports one node per maintainer, with operators potentially managing multiple independent nodes.

- **Migration**: Represents database schema change with version number, up/down SQL scripts, and execution status. Tracked in database migrations table and filesystem.

- **Deployment Artifact**: Represents built server binary with embedded metadata (version, commit SHA, build timestamp), deployment scripts, and configuration files packaged for deployment.

- **Health Check**: Represents application health status including HTTP endpoint responses, database connectivity, and critical service availability. Used to validate deployment success.

## Success Criteria *(mandatory)*

### Measurable Outcomes

**MVP Success Criteria:**
- **SC-001** [MVP]: Developers can deploy the server to a Docker host in under 10 minutes with zero manual infrastructure configuration
- **SC-002** [MVP]: Deployments complete successfully 95% of the time without manual intervention
- **SC-003** [MVP]: Failed deployments roll back automatically within 2 minutes without operator intervention
- **SC-004** [MVP]: Zero downtime deployments achieve 99.9% availability during update operations
- **SC-006** [MVP]: Database migrations complete successfully on databases with up to 1 million events without timeout failures
- **SC-007** [MVP]: Deployment configuration changes are applied successfully without modifying deployment scripts or application code
- **SC-009** [MVP]: 90% of production deployments complete within 5 minutes from command execution to health check pass

**Post-MVP Success Criteria:**
- **SC-005** [Phase 2]: Operators can switch between three different cloud providers using the same deployment command with only credential changes
- **SC-008** [Phase 2]: Health checks detect deployment issues within 30 seconds of completion, triggering automatic rollback
- **SC-010** [Phase 2]: Deployment system handles provider rate limiting and quota errors gracefully with clear error messages

## Assumptions *(include when relevant)*

- Operators have administrative access to target cloud providers (IAM credentials, API keys)
- Target environments have internet connectivity for downloading dependencies and container images
- PostgreSQL 16+ is available either as managed service (AWS RDS, GCP Cloud SQL) or can be deployed in containers
- Operators have Docker installed locally for container-based workflows
- Git repository contains valid database migrations in expected directory structure
- Environment configuration files (.env) are created manually or through CI/CD before first deployment
- SSL/TLS certificates can be obtained automatically via Let's Encrypt or provided by operator
- DNS records for production domains are configured to point to deployed infrastructure
- CI/CD pipeline has necessary permissions to trigger deployments (GitHub Actions with cloud provider credentials)
- Deployment targets have sufficient resources (CPU, memory, disk) to run the server application with expected load
- Operators can specify deployment targets through command-line arguments or environment variables
- Database backup storage is available either through provider-managed backups or object storage (S3, GCS)
- Database snapshots before migrations are retained for 7 days with automatic cleanup

## Dependencies *(include when relevant)*

- **External Dependencies**:
  - Docker and Docker Compose for containerized deployment workflow
  - Cloud provider CLIs (AWS CLI, gcloud, Azure CLI) for infrastructure provisioning
  - Terraform or similar infrastructure-as-code tool for provider-agnostic resource management
  - GitHub Actions or equivalent CI/CD platform for automated deployments
  - SSL certificate provisioning service (Let's Encrypt, cloud provider certificate manager)

- **Internal Dependencies**:
  - Existing Makefile build targets (`make build`, `make test-ci`) must pass before deployment
  - Database migration files in `internal/storage/postgres/migrations/` must be valid and tested
  - Application health check endpoint must be implemented and return meaningful status
  - Environment variable validation in application startup must catch configuration errors early

- **Technical Constraints**:
  - Deployment must work from both developer workstations (manual deployment) and CI/CD pipelines (automated deployment)
  - Configuration format must be human-readable and version-controllable (YAML, TOML, or environment files)
  - Deployment tooling must be cross-platform (Linux, macOS, Windows via WSL)
  - Provider-specific configuration should be isolated to enable adding new providers without changing core deployment logic

## Out of Scope *(include when relevant)*

- **Explicitly Excluded**:
  - Kubernetes orchestration (initial version focuses on simpler deployment models; K8s can be added later)
  - Multi-region active-active deployment with geographic failover
  - Automated performance testing or load testing during deployment
  - Canary deployments or A/B testing infrastructure
  - Custom monitoring dashboards or observability platform integration beyond basic metrics
  - Disaster recovery automation or multi-region backup replication
  - Cost optimization recommendations or infrastructure rightsizing automation
  - Deployment scheduling or deployment windows (deployments can run anytime)

- **Future Considerations**:
  - Additional alert channel integrations (email SMTP, Slack, PagerDuty, custom webhooks)
  - Integration with cloud provider secret managers (AWS Secrets Manager, GCP Secret Manager) for production environments
  - Integration with infrastructure cost tracking and budget alerts
  - Automated security scanning of deployment artifacts and infrastructure
  - Deployment approval workflows for production environments
  - Advanced deployment strategies (shadow deployments, traffic mirroring)
  - Infrastructure drift detection between deployed and configured state

## Technical Considerations *(optional)*

### MVP Architecture (Phase 1)

The deployment infrastructure prioritizes simplicity and maintainability over feature completeness. Docker Compose provides the universal baseline that works everywhere (cloud VMs, bare metal, developer laptops) with a clear upgrade path to cloud-native services.

**MVP Technical Stack:**
- **Deployment Engine**: Docker Compose v2 (standardized, works everywhere)
- **Configuration**: YAML-based with .env file support
- **Blue-Green Orchestration**: Docker networks + health checks + traffic switching via nginx/traefik
- **Database**: PostgreSQL 16+ in Docker container with volume persistence
- **Migrations**: golang-migrate CLI (already in project dependencies)
- **Snapshots**: pg_dump to local filesystem or S3-compatible storage
- **Secrets**: Environment files (.env) excluded from git
- **Logging**: Docker logs (stdout/stderr) with structured JSON format
- **Deployment Lock**: Filesystem lock or Redis key with TTL

**Future-Proof Design Patterns:**
1. **Provider Abstraction Interface**:
   ```
   type DeploymentProvider interface {
       Provision(config Config) error
       Deploy(artifact Artifact) error
       HealthCheck() (bool, error)
       Rollback(version string) error
       Cleanup() error
   }
   ```
   MVP implements `DockerComposeProvider`, Phase 2 adds `AWSProvider`, `GCPProvider`

2. **Configuration Schema** (provider-agnostic + provider-specific):
   ```yaml
   # Shared across all providers
   app:
     name: togather-server
     version: ${GIT_COMMIT}
   database:
     extensions: [postgis, pgvector, pg_trgm]
   
   # Provider-specific overrides
   providers:
     docker:
       compose_file: docker-compose.yml
     aws:
       region: us-east-1
       service: ecs
     gcp:
       region: us-central1
       service: cloud-run
   ```

3. **Event-Driven Notifications** (webhook interface):
   ```
   POST /webhook
   {
     "event": "deployment.started|completed|failed",
     "deployment_id": "abc123",
     "version": "v1.2.3",
     "timestamp": "2026-01-28T10:00:00Z",
     "metadata": {...}
   }
   ```
   MVP: Generic webhook endpoint, Phase 2: ntfy.sh, Phase 3: Email/Slack adapters

4. **Pluggable Health Checks**:
   ```
   type HealthCheck interface {
       Check(ctx context.Context) error
   }
   ```
   MVP: HTTP endpoint + database connectivity, Phase 2+: Custom checks per deployment

### Phase 2: Cloud-Native Optimization

When operators need cloud-specific features (managed databases, auto-scaling, geographic distribution):

**AWS Stack:**
- ECS/Fargate for container orchestration
- RDS PostgreSQL with automated backups
- Application Load Balancer for blue-green traffic switching
- Secrets Manager for credentials
- CloudWatch for logging/metrics

**GCP Stack:**
- Cloud Run for serverless containers
- Cloud SQL with automatic backups
- Cloud Load Balancing for traffic management
- Secret Manager for credentials
- Cloud Logging for centralized logs

**Terraform Modules** (one per provider, sharing same interface):
- Variables match configuration schema (provider-agnostic where possible)
- Outputs expose connection strings, URLs, status endpoints
- State management via Terraform Cloud or S3 backend

### Database Migration Strategy

**MVP Implementation:**
- Use existing `golang-migrate` tool from project dependencies
- Run migrations in init container before app starts (blue-green ensures zero downtime)
- Snapshot via `pg_dump` before migration runs
- Store snapshots with naming: `{database}_{timestamp}_{git_commit}.sql.gz`
- Retention: Keep 7 days (cron job or lifecycle policy deletes old snapshots)
- Rollback: Manual restore from snapshot if migration fails (provide clear instructions)

**Forward Compatibility:**
- Migration scripts must be backward-compatible during blue-green window
- Old code runs against new schema until traffic switches
- Use additive migrations (add columns as nullable, add indexes concurrently)
- Breaking changes require two-phase deployments

**Future Enhancement (Phase 3):**
- Automated schema compatibility validation before migration
- Transaction-wrapped migrations with automatic rollback
- Migration dry-run mode for testing

### Secrets Management

**MVP (Environment Files):**
```bash
# .env.production (NOT in git, managed by operator)
DATABASE_URL=postgresql://user:pass@host:5432/db
JWT_SECRET=xxx
ADMIN_API_KEY=yyy
```

**Future-Proof Interface:**
```go
type SecretsBackend interface {
    GetSecret(key string) (string, error)
    ListSecrets(prefix string) (map[string]string, error)
}
```

MVP: `EnvFileBackend`, Phase 3: `AWSSecretsManagerBackend`, `GCPSecretManagerBackend`

### Alerting Architecture

**MVP (Structured Logging):**
```json
{
  "level": "error",
  "event": "deployment_failed",
  "deployment_id": "abc123",
  "version": "v1.2.3",
  "error": "health check failed after 5 attempts",
  "timestamp": "2026-01-28T10:00:00Z"
}
```

**Phase 2 (ntfy.sh Integration):**
```bash
curl -d "Deployment v1.2.3 failed: health check timeout" \
  https://ntfy.sh/togather-deployments
```

**Phase 3 (Service-Specific Adapters):**
- Email: SMTP with templated messages
- Slack: Webhook with rich formatting (buttons, status colors)
- PagerDuty: Incident creation with severity levels

All alerting goes through event bus (future-proof for adding channels without changing deployment code).
