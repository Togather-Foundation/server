# Tasks: Deployment Infrastructure

**Input**: Design documents from `/specs/001-deployment-infrastructure/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are NOT included as they were not explicitly requested in the specification.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

This is a deployment infrastructure feature for the existing Go project at repository root:
- Deployment scripts: `deploy/scripts/`
- Docker configuration: `deploy/docker/`
- Configuration files: `deploy/config/`
- Monitoring: `deploy/monitoring/` (Phase 2)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create deployment infrastructure directory structure and baseline configuration

- [x] T001 Create deployment directory structure (deploy/docker/, deploy/config/, deploy/scripts/, deploy/monitoring/)
- [x] T002 [P] Create .gitignore entries for .env files and deployment state files
- [x] T003 [P] Create deploy/config/deployment.yml base configuration file
- [x] T004 [P] Create deploy/config/environments/.env.development.example template
- [x] T005 [P] Create deploy/config/environments/.env.staging.example template
- [x] T006 [P] Create deploy/config/environments/.env.production.example template

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core deployment infrastructure that MUST be complete before ANY user story can be deployed

**⚠️ CRITICAL**: No deployment operations can begin until this phase is complete

- [x] T007 Create multi-stage Dockerfile (build stage: Go 1.25+ builder, runtime stage: Alpine) with version metadata in deploy/docker/Dockerfile (per research.md:L735)
- [x] T008 Create base docker-compose.yml for app + database in deploy/docker/docker-compose.yml
- [x] T009 Create blue-green docker-compose.blue-green.yml orchestration in deploy/docker/docker-compose.blue-green.yml
- [x] T010 [P] Create Caddy reverse proxy configuration for blue-green traffic switching in deploy/docker/Caddyfile.example
- [x] T011 [P] Implement health check endpoint handler with database, migrations, http_endpoint, and job_queue checks in internal/api/health.go (per data-model.md:L275-L323)
- [x] T012 [P] Implement version endpoint handler in internal/api/version.go
- [x] T013 Create deployment state JSON schema and initialization in deploy/config/deployment-state.schema.json

**Checkpoint**: Foundation ready - deployment scripts can now be implemented ✅

---

## Phase 3: User Story 1 - One-Command Production Deploy (Priority: P1) 🎯 MVP

**Goal**: Deploy the Togather server to production with a single command, regardless of cloud provider

**Independent Test**: Run `./deploy/scripts/deploy.sh production` on a fresh environment and verify server responds to health checks

### Implementation for User Story 1

- [x] T014 [US1] Implement config validation function in deploy/scripts/deploy.sh
- [x] T014a [US1] Validate deployment tool versions (docker >=20.10, docker-compose >=2.0, golang-migrate, jq, psql) in deploy/scripts/deploy.sh
- [x] T014b [US1] Validate deployed Git commit matches CI test results in deploy/scripts/deploy.sh
- [x] T015 [US1] Implement deployment lock acquisition/release with 30-minute timeout and stale lock detection in deploy/scripts/deploy.sh
- [x] T016 [US1] Implement Docker image build with version metadata in deploy/scripts/deploy.sh
- [x] T017 [US1] Integrate database snapshot creation (calls snapshot-db.sh from T027) into deploy/scripts/deploy.sh
- [x] T018 [US1] Implement database migration execution with golang-migrate in deploy/scripts/deploy.sh
- [x] T019 [US1] Implement blue-green deployment orchestration in deploy/scripts/deploy.sh
- [x] T020 [US1] Implement health check validation script in deploy/scripts/health-check.sh
- [x] T021 [US1] Implement traffic switching logic (Caddy config reload) in deploy/scripts/deploy.sh
- [x] T022 [US1] Implement deployment state tracking (JSON updates) in deploy/scripts/deploy.sh
- [x] T023 [US1] Implement structured deployment logging to /var/log/togather/deployments/ in deploy/scripts/deploy.sh
- [x] T023a [US1] Implement secret sanitization in deployment logs (redact DATABASE_URL passwords, JWT_SECRET, API keys) in deploy/scripts/deploy.sh
- [ ] T024 [US1] Add --dry-run validation mode to deploy/scripts/deploy.sh
- [ ] T025 [US1] Add --version, --skip-migrations, --force flags to deploy/scripts/deploy.sh
- [x] T026 [US1] Create deploy/README.md with quick start deployment instructions

**Checkpoint**: At this point, single-command deployment should work end-to-end for production ✅

---

## Phase 4: User Story 3 - Database Migration Management (Priority: P1) 🎯 MVP

**Goal**: Database migrations run automatically during deployment with safety snapshots

**Independent Test**: Deploy a version with new migrations and verify schema changes applied correctly and snapshot was created

**Note**: This story is P1 but depends on User Story 1 deployment infrastructure. Tasks integrated into US1 deployment flow.

### Implementation for User Story 3

- [x] T027 [P] [US3] Implement automatic snapshot before migrations in deploy/scripts/snapshot-db.sh (606 lines with comprehensive features)
- [x] T028 [P] [US3] Implement 7-day snapshot retention policy in deploy/scripts/snapshot-db.sh (integrated into snapshot-db.sh)
- [ ] T029 [P] [US3] Implement automatic snapshot cleanup (delete expired) in deploy/scripts/cleanup.sh (cleanup integrated into snapshot-db.sh, standalone script optional)
- [x] T030 [US3] Add migration version validation in health check in internal/api/health.go
- [ ] T031 [US3] Implement migration failure detection and automatic rollback trigger in deploy/scripts/deploy.sh
- [x] T032 [US3] Add migration execution locking to prevent concurrent migrations in deploy/scripts/deploy.sh
- [x] T033 [US3] Create migration troubleshooting documentation in deploy/docs/migrations.md

**Checkpoint**: Database migrations should run safely with automatic snapshots and failure handling ✅

---

## Phase 5: User Story 4 - Environment Configuration (Priority: P2) 🎯 MVP

**Goal**: Operators can manage dev/staging/production configurations through standardized files

**Independent Test**: Deploy to three environments with different .env files and verify each uses its specific settings

### Implementation for User Story 4

- [x] T034 [P] [US4] Implement environment-specific configuration loading in deploy/scripts/deploy.sh
- [x] T035 [P] [US4] Implement secrets validation (ensure no CHANGE_ME placeholders) in deploy/scripts/deploy.sh
- [x] T036 [P] [US4] Implement configuration file validation (YAML schema check) in deploy/scripts/deploy.sh
- [x] T037 [US4] Add environment variable override precedence (CLI > env > .env > deployment.yml) in deploy/scripts/deploy.sh
- [x] T038 [US4] Implement secure file permissions check for .env files (chmod 600) in deploy/scripts/deploy.sh
- [x] T039 [US4] Add configuration error reporting with clear remediation steps in deploy/scripts/deploy.sh
- [ ] T040 [US4] Create environment configuration guide in deploy/docs/configuration.md

**Checkpoint**: Multiple environments should be independently configurable without code changes ✅

---

## Phase 6: User Story 5 - Deployment Rollback (Priority: P2) 🎯 MVP

**Goal**: Quickly roll back to previous working version with single command

**Independent Test**: Deploy a version, deploy a "broken" version, then rollback and verify original version is restored

### Implementation for User Story 5

- [x] T041 [US5] Implement deployment history tracking in /var/lib/togather/deployments/ in deploy/scripts/deploy.sh ✅
- [x] T042 [US5] Implement previous version detection logic in deploy/scripts/rollback.sh ✅
- [x] T043 [US5] Implement Docker image tag switching for rollback in deploy/scripts/rollback.sh ✅
- [x] T044 [US5] Implement traffic switching to previous version in deploy/scripts/rollback.sh ✅
- [x] T045 [US5] Implement health check validation after rollback in deploy/scripts/rollback.sh ✅
- [x] T046 [US5] Add interactive confirmation prompt for rollback operations in deploy/scripts/rollback.sh ✅
- [x] T047 [US5] Add --force flag for non-interactive rollback in deploy/scripts/rollback.sh ✅
- [x] T048 [US5] Add --version flag to rollback to specific version in deploy/scripts/rollback.sh ✅
- [x] T049 [US5] Implement database snapshot restore instructions (manual confirmation required) in deploy/scripts/rollback.sh ✅
- [x] T050 [US5] Create rollback troubleshooting guide in deploy/docs/rollback.md ✅

**Checkpoint**: Rollback should restore previous version within 2 minutes and pass health checks ✅

**User Story 5 Status**: ✅ COMPLETE


---

## Phase 7: User Story 2 - Multi-Provider Support (Priority: P2) **[Phase 2 - Future]**

**Goal**: Switch between different hosting providers using same deployment workflow

**Independent Test**: Deploy to two different providers using identical configuration (except credentials)

**Note**: This is Phase 2 post-MVP work. Provider abstraction is designed into MVP but implementations deferred.

### Implementation for User Story 2

- [ ] T051 [P] [US2] Create provider abstraction interface documentation in deploy/docs/providers.md
- [ ] T052 [P] [US2] Create deploy/config/providers/docker.yml configuration template
- [ ] T053 [P] [US2] Create deploy/config/providers/aws.yml configuration template (Phase 2)
- [ ] T054 [P] [US2] Create deploy/config/providers/gcp.yml configuration template (Phase 2)
- [ ] T055 [US2] Implement provider selection logic in deploy/scripts/deploy.sh
- [ ] T056 [US2] Document provider-specific setup instructions in deploy/docs/providers.md
- [ ] T057 [US2] Create AWS ECS/Fargate deployment module (Phase 2 - deferred)
- [ ] T058 [US2] Create GCP Cloud Run deployment module (Phase 2 - deferred)

**Checkpoint**: Provider abstraction ready for future AWS/GCP implementations

---

## Phase 8: User Story 6 - Health Monitoring and Alerting (Priority: P3) **[Phase 2 - Future]**

**Goal**: Automatic health monitoring and alerting after deployment completes

**Independent Test**: Deploy a version, simulate health check failures, verify alerts are sent

**Note**: This is Phase 2 post-MVP work. Monitoring stack deployment deferred.

### Implementation for User Story 6

- [ ] T059 [P] [US6] Create deploy/monitoring/prometheus/prometheus.yml scrape configuration
- [ ] T060 [P] [US6] Create deploy/monitoring/prometheus/alerts.yml alert rules
- [ ] T061 [P] [US6] Create deploy/monitoring/grafana/datasources.yml Prometheus datasource config
- [ ] T062 [P] [US6] Create pre-built Grafana dashboard JSON for deployment metrics in deploy/monitoring/grafana/dashboards/deployment.json
- [ ] T063 [P] [US6] Create pre-built Grafana dashboard JSON for application health in deploy/monitoring/grafana/dashboards/application.json
- [ ] T064 [P] [US6] Create pre-built Grafana dashboard JSON for infrastructure metrics in deploy/monitoring/grafana/dashboards/infrastructure.json
- [ ] T065 [P] [US6] Create deploy/monitoring/alertmanager/alertmanager.yml with ntfy.sh webhook config
- [ ] T066 [US6] Create docker-compose.monitoring.yml for Prometheus/Grafana/Alertmanager stack in deploy/docker/docker-compose.monitoring.yml
- [ ] T067 [US6] Implement ENABLE_MONITORING flag to conditionally deploy monitoring stack in deploy/scripts/deploy.sh
- [ ] T068 [US6] Implement application /metrics endpoint with Prometheus client in internal/api/metrics.go
- [ ] T069 [US6] Add HTTP metrics middleware (request count, duration, status codes) in internal/api/middleware/metrics.go
- [ ] T070 [US6] Add database connection pool metrics in internal/storage/postgres/metrics.go
- [ ] T071 [US6] Create monitoring setup guide in deploy/docs/monitoring.md

**Checkpoint**: Monitoring stack should deploy alongside application and expose metrics/dashboards

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories and production readiness

- [ ] T072 [P] Implement deployment artifact cleanup script in deploy/scripts/cleanup.sh
- [ ] T073 [P] Add deployment log rotation configuration in deploy/config/logrotate.conf
- [ ] T074 [P] Create comprehensive troubleshooting guide in deploy/docs/troubleshooting.md
- [ ] T075 [P] Create operator quickstart guide (copy from specs/001-deployment-infrastructure/quickstart.md to deploy/docs/quickstart.md)
- [ ] T076 [P] Add CI/CD integration examples in deploy/docs/ci-cd.md
- [ ] T077 [P] Document deployment best practices in deploy/docs/best-practices.md
- [ ] T078 [P] Add deployment security checklist in deploy/docs/security.md
- [ ] T079 Implement automated deployment validation tests (full flow: Docker build, migration execution, health checks) using testcontainers in tests/deployment/
- [ ] T080 Add deployment smoke tests (health checks, version verification) in tests/deployment/
- [ ] T081 Update main project README.md with deployment quick start section
- [ ] T082 Validate quickstart.md instructions by following them on clean VM
- [ ] T083 Code review and refactoring of deployment scripts for clarity
- [ ] T084 Performance testing: Measure deployment duration and optimize to meet <5min target
- [ ] T085 Add exit code documentation and error handling improvements across all scripts

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational - Core deployment capability
- **User Story 3 (Phase 4)**: Depends on US1 - Migration safety integrated into deployment
- **User Story 4 (Phase 5)**: Depends on US1 - Environment config extends deployment
- **User Story 5 (Phase 6)**: Depends on US1 - Rollback uses deployment infrastructure
- **User Story 2 (Phase 7)**: Depends on US1 - Provider abstraction extends core deployment (Phase 2 - deferred)
- **User Story 6 (Phase 8)**: Independent of other stories but enhances all (Phase 2 - deferred)
- **Polish (Phase 9)**: Depends on US1, US3, US4, US5 completion

### User Story Dependencies

- **User Story 1 (P1) - Core Deployment**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 3 (P1) - Migrations**: Depends on US1 deployment flow - Integrates migration safety into core deployment
- **User Story 4 (P2) - Environment Config**: Depends on US1 - Extends deployment with multi-environment support
- **User Story 5 (P2) - Rollback**: Depends on US1 - Uses deployment history and infrastructure
- **User Story 2 (P2) - Multi-Provider**: Depends on US1 - Provider abstraction designed but implementations deferred to Phase 2
- **User Story 6 (P3) - Monitoring**: Independent implementation, enhances all stories - Phase 2

### Within Each User Story

- Foundational infrastructure before deployment scripts
- Core deployment logic before extensions (config, rollback)
- Health checks before traffic switching
- Logging and state tracking integrated throughout
- Documentation after implementation

### Parallel Opportunities

**Phase 1 (Setup)**: All tasks T002-T006 can run in parallel (different files)

**Phase 2 (Foundational)**: 
- T010 (nginx config), T011 (health endpoint), T012 (version endpoint), T013 (JSON schema) can run in parallel after T007-T009

**Phase 3 (User Story 1)**:
- T014-T016 (validation, lock, build) can be prototyped in parallel
- T024-T025 (flags) can run in parallel after core deployment works

**Phase 4 (User Story 3)**:
- T027-T029 (snapshot/cleanup) can run in parallel
- T030 (health check validation) can run in parallel

**Phase 5 (User Story 4)**:
- T034-T036, T040 can run in parallel

**Phase 7 (User Story 2)**:
- T051-T054, T056 (provider docs and configs) can all run in parallel

**Phase 8 (User Story 6)**:
- T059-T065 (monitoring configs) can all run in parallel
- T062-T064 (Grafana dashboards) can all run in parallel
- T069-T070 (metrics collection) can run in parallel

**Phase 9 (Polish)**:
- T072-T078, T081 (docs and cleanup) can all run in parallel

---

## Parallel Example: User Story 1

```bash
# Launch foundational tasks together:
Task: "Create nginx reverse proxy configuration in deploy/docker/nginx.conf"
Task: "Implement health check endpoint in internal/api/health.go"
Task: "Implement version endpoint in internal/api/version.go"

# Launch deployment flags together after core works:
Task: "Add --dry-run validation mode to deploy/scripts/deploy.sh"
Task: "Add --version, --skip-migrations, --force flags to deploy/scripts/deploy.sh"
```

---

## Implementation Strategy

### MVP First (User Stories 1, 3, 4, 5 Only)

1. Complete Phase 1: Setup → Directory structure ready
2. Complete Phase 2: Foundational (CRITICAL) → Core infrastructure ready
3. Complete Phase 3: User Story 1 → Single-command deployment works
4. Complete Phase 4: User Story 3 → Migration safety integrated
5. Complete Phase 5: User Story 4 → Multi-environment configuration
6. Complete Phase 6: User Story 5 → Rollback capability
7. **STOP and VALIDATE**: Test all MVP stories independently
8. Complete Phase 9: Polish → Production-ready
9. Deploy to production

### Post-MVP Enhancement (Phase 2)

After MVP deployed and validated:

1. Phase 7: User Story 2 → Multi-provider support (AWS/GCP)
2. Phase 8: User Story 6 → Monitoring and alerting stack
3. Each enhancement adds value without breaking core deployment

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together (Phases 1-2)
2. Once Foundational is done:
   - Developer A: User Story 1 (Core deployment) → MUST complete first
   - After US1 complete:
     - Developer B: User Story 3 (Migrations integration)
     - Developer C: User Story 4 (Environment config)
     - Developer D: User Story 5 (Rollback)
3. Stories integrate incrementally without breaking core deployment

---

## Success Metrics (MVP)

Track these metrics to verify MVP success criteria:

- [ ] **SC-001**: Deploy to Docker host completes in under 10 minutes (target: 5 minutes)
- [ ] **SC-002**: Deployment success rate ≥ 95% without manual intervention
- [ ] **SC-003**: Failed deployments roll back automatically within 2 minutes
- [ ] **SC-004**: Zero-downtime deployments achieve 99.9% availability during updates
- [ ] **SC-006**: Database migrations complete successfully on databases with 1M+ events
- [ ] **SC-007**: 90% of deployments complete within 5 minutes from command execution to health check pass

---

## Notes

- [P] tasks = different files, no dependencies, can run in parallel
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group of tasks
- Run `make ci` before pushing to catch CI failures locally
- Stop at any checkpoint to validate story independently
- Tests not included as not explicitly requested in specification
- Phase 2 features (US2, US6) designed but implementation deferred post-MVP
- All scripts follow deployment-scripts.md contract specifications
- Monitoring stack (US6) is optional, enabled via ENABLE_MONITORING flag

### Code Quality Improvements (2026-01-28)

**P2 Issues Resolved** (12 issues closed):
- **Documentation**: Fixed non-existent restore command refs, corrected psql restore examples (4 locations)
- **Portability**: Added portable `get_file_perms()` helper, epoch-based date calculations (Linux/macOS compatible)
- **Security**: Removed unnecessary sudo usage, uses `~/.togather/logs` instead of `/var/log`
- **Validation**: Added DATABASE_URL validation, snapshot integrity checks (opt-in), schema table existence checks
- **Code Quality**: Fixed hardcoded blue/green ports (8081, 8082), improved error context in health checks, parameterized SQL queries
- **Health Checks**: Better error reporting (Details map), SQL parameterization (ANY($1)), verified context timeout propagation

All deployment scripts now work portably on both Linux and macOS systems.

---

## Total Task Count

- **Phase 1 (Setup)**: 6 tasks
- **Phase 2 (Foundational)**: 7 tasks
- **Phase 3 (User Story 1 - P1 MVP)**: 16 tasks (T014-T026, including T014a, T014b, T023a)
- **Phase 4 (User Story 3 - P1 MVP)**: 7 tasks
- **Phase 5 (User Story 4 - P2 MVP)**: 7 tasks
- **Phase 6 (User Story 5 - P2 MVP)**: 10 tasks
- **Phase 7 (User Story 2 - P2 Phase 2)**: 8 tasks
- **Phase 8 (User Story 6 - P3 Phase 2)**: 13 tasks
- **Phase 9 (Polish)**: 14 tasks

**Total**: 88 tasks

**MVP Scope (Phases 1-6 + 9)**: 67 tasks
**Phase 2 Scope (Phases 7-8)**: 21 tasks

**Parallel Opportunities**: 42 tasks marked [P] can run in parallel within their phases
