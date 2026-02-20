# CI/CD Integration Guide

This guide shows how to integrate Togather deployment scripts into CI/CD pipelines.

## Table of Contents

- [GitHub Actions](#github-actions)
- [GitLab CI](#gitlab-ci)
- [Jenkins](#jenkins)
- [Environment Variables](#environment-variables)
- [Security Best Practices](#security-best-practices)
- [Deployment Strategies](#deployment-strategies)

---

## GitHub Actions

### Basic Deployment Workflow

Create `.github/workflows/deploy.yml`:

```yaml
name: Deploy to Production

on:
  push:
    branches:
      - main
  workflow_dispatch:  # Allow manual trigger

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: production  # Use GitHub Environments for protection rules
    
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for git commit info
      
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
      
      - name: Install dependencies
        run: |
          # Install golang-migrate
          curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
          sudo mv migrate /usr/local/bin/
          
          # Install PostgreSQL client
          sudo apt-get update
          sudo apt-get install -y postgresql-client jq
      
      - name: Configure environment
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
          JWT_SECRET: ${{ secrets.JWT_SECRET }}
        run: |
          # Create production environment file
          cat > /opt/togather/.env.production <<EOF
          ENVIRONMENT=production
          DATABASE_URL=$DATABASE_URL
          JWT_SECRET=$JWT_SECRET
          SERVER_PORT=8080
          LOG_LEVEL=info
          SHUTDOWN_TIMEOUT=30s
          DEPLOYED_BY=${{ github.actor }}@github-actions
          EOF
          
          chmod 600 /opt/togather/.env.production
      
      - name: Deploy to production
        run: |
          cd deploy/scripts
          ./deploy.sh production
      
      - name: Verify deployment
        run: |
          # Wait for health checks
          sleep 10
          
          # Check health endpoint
          curl -f http://localhost:8080/health || exit 1
      
      - name: Notify on failure
        if: failure()
        run: |
          echo "Deployment failed! Consider rolling back."
          # Add notification logic (Slack, email, etc.)
```

### Deployment with Rollback on Failure

```yaml
name: Deploy with Auto-Rollback

on:
  push:
    branches:
      - main

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: production
    
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Setup
        run: |
          # Install dependencies (as above)
          curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
          sudo mv migrate /usr/local/bin/
          sudo apt-get update && sudo apt-get install -y postgresql-client jq
      
      - name: Configure environment
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
          JWT_SECRET: ${{ secrets.JWT_SECRET }}
        run: |
          cat > /opt/togather/.env.production <<EOF
          ENVIRONMENT=production
          DATABASE_URL=$DATABASE_URL
          JWT_SECRET=$JWT_SECRET
          DEPLOYED_BY=${{ github.actor }}@github-actions
          EOF
          chmod 600 /opt/togather/.env.production
      
      - name: Deploy
        id: deploy
        run: |
          cd deploy/scripts
          ./deploy.sh production
        continue-on-error: true
      
      - name: Run smoke tests
        id: smoke_tests
        if: steps.deploy.outcome == 'success'
        run: |
          # Wait for application to stabilize
          sleep 15
          
          # Run health checks
          ./deploy/scripts/health-check.sh production || exit 1
          
          # Run smoke tests
          curl -f http://localhost:8080/health || exit 1
          curl -f http://localhost:8080/api/v1/events || exit 1
        continue-on-error: true
      
      - name: Rollback on failure
        if: steps.deploy.outcome == 'failure' || steps.smoke_tests.outcome == 'failure'
        run: |
          echo "Deployment or smoke tests failed - rolling back"
          server deploy rollback production --force
      
      - name: Verify rollback
        if: steps.deploy.outcome == 'failure' || steps.smoke_tests.outcome == 'failure'
        run: |
          sleep 10
          ./deploy/scripts/health-check.sh production || exit 1
      
      - name: Fail workflow if rolled back
        if: steps.deploy.outcome == 'failure' || steps.smoke_tests.outcome == 'failure'
        run: exit 1
```

### Staging → Production Pipeline

```yaml
name: Multi-Stage Deployment

on:
  push:
    branches:
      - main

jobs:
  deploy-staging:
    runs-on: ubuntu-latest
    environment: staging
    
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Setup
        run: |
          curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
          sudo mv migrate /usr/local/bin/
          sudo apt-get update && sudo apt-get install -y postgresql-client jq
      
      - name: Deploy to staging
        env:
          DATABASE_URL: ${{ secrets.STAGING_DATABASE_URL }}
          JWT_SECRET: ${{ secrets.STAGING_JWT_SECRET }}
        run: |
          # Note: deploy.sh expects .env files on the SERVER, not locally
          # The server should have /opt/togather/.env.staging pre-configured
          # This script connects remotely and deploys
          cd deploy/scripts
          ./deploy.sh staging --remote deploy@staging.server.com
      
      - name: Test staging
        run: |
          sleep 10
          ./deploy/scripts/health-check.sh staging
  
  deploy-production:
    needs: deploy-staging
    runs-on: ubuntu-latest
    environment: production
    
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Setup
        run: |
          curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
          sudo mv migrate /usr/local/bin/
          sudo apt-get update && sudo apt-get install -y postgresql-client jq
      
      - name: Configure production
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
          JWT_SECRET: ${{ secrets.JWT_SECRET }}
        run: |
          cat > /opt/togather/.env.production <<EOF
          ENVIRONMENT=production
          DATABASE_URL=$DATABASE_URL
          JWT_SECRET=$JWT_SECRET
          EOF
          chmod 600 /opt/togather/.env.production
      
      - name: Deploy to production
        run: |
          cd deploy/scripts
          ./deploy.sh production
      
      - name: Verify production
        run: |
          sleep 10
          ./deploy/scripts/health-check.sh production
```

---

## GitLab CI

### Basic Deployment Pipeline

Create `.gitlab-ci.yml`:

```yaml
stages:
  - build
  - deploy
  - verify

variables:
  DOCKER_DRIVER: overlay2

deploy-production:
  stage: deploy
  image: ubuntu:22.04
  environment:
    name: production
  only:
    - main
  before_script:
    # Install dependencies
    - apt-get update
    - apt-get install -y curl docker.io postgresql-client jq
    - curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
    - mv migrate /usr/local/bin/
  script:
    # Configure environment
    - |
      cat > /opt/togather/.env.production <<EOF
      ENVIRONMENT=production
      DATABASE_URL=$DATABASE_URL
      JWT_SECRET=$JWT_SECRET
      DEPLOYED_BY=$GITLAB_USER_LOGIN@gitlab-ci
      EOF
    - chmod 600 /opt/togather/.env.production
    
    # Deploy
    - cd deploy/scripts
    - ./deploy.sh production
  after_script:
    # Verify deployment
    - sleep 10
    - ./deploy/scripts/health-check.sh production || exit 1

verify-production:
  stage: verify
  image: curlimages/curl:latest
  environment:
    name: production
  only:
    - main
  script:
    - sleep 15
    - curl -f http://localhost:8080/health
    - curl -f http://localhost:8080/api/v1/events
```

---

## Jenkins

### Declarative Pipeline

Create `Jenkinsfile`:

```groovy
pipeline {
    agent any
    
    environment {
        DATABASE_URL = credentials('production-database-url')
        JWT_SECRET = credentials('production-jwt-secret')
    }
    
    stages {
        stage('Setup') {
            steps {
                sh '''
                    # Install dependencies
                    curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
                    sudo mv migrate /usr/local/bin/
                    
                    # Verify tools
                    docker --version
                    migrate -version
                    jq --version
                '''
            }
        }
        
        stage('Configure') {
            steps {
                sh '''
                    cat > /opt/togather/.env.production <<EOF
                    ENVIRONMENT=production
                    DATABASE_URL=$DATABASE_URL
                    JWT_SECRET=$JWT_SECRET
                    DEPLOYED_BY=${BUILD_USER}@jenkins
                    EOF
                    
                    chmod 600 /opt/togather/.env.production
                '''
            }
        }
        
        stage('Deploy') {
            steps {
                sh '''
                    cd deploy/scripts
                    ./deploy.sh production
                '''
            }
        }
        
        stage('Verify') {
            steps {
                sh '''
                    sleep 10
                    ./deploy/scripts/health-check.sh production
                '''
            }
        }
    }
    
    post {
        failure {
            sh '''
                echo "Deployment failed - consider rollback"
                cd deploy/scripts
                server deploy rollback production --force
            '''
        }
        success {
            echo "Deployment successful!"
        }
    }
}
```

---

## Environment Variables

### Required Secrets

Store these as secrets in your CI/CD platform:

**Production:**
- `DATABASE_URL` - PostgreSQL connection string
- `JWT_SECRET` - JWT signing key (generate: `openssl rand -base64 32`)

**Staging:**
- `STAGING_DATABASE_URL`
- `STAGING_JWT_SECRET`

**Development:**
- `DEV_DATABASE_URL`
- `DEV_JWT_SECRET`

### GitHub Actions Setup

1. Go to repository Settings → Secrets and variables → Actions
2. Add repository secrets or use Environments for environment-specific secrets
3. Use GitHub Environments for deployment protection rules

### GitLab CI Setup

1. Go to Settings → CI/CD → Variables
2. Add protected and masked variables
3. Scope variables to specific environments

### Jenkins Setup

1. Manage Jenkins → Credentials
2. Add credentials (secret text or secret file)
3. Reference with `credentials()` function

---

## Security Best Practices

### 1. Never Commit Secrets

```bash
# .gitignore should include:
.env
.env.*
!.env.*.example
/opt/togather/.env.*
!.env.*.example
```

### 2. Use Deployment Keys

Generate SSH keys for deployment:

```bash
ssh-keygen -t ed25519 -C "deploy@togather" -f deploy_key
```

Add public key to server, private key to CI/CD secrets.

### 3. Restrict Environment Access

- Use branch protection rules
- Require approvals for production deployments
- Use environment-specific secrets
- Enable deployment protection rules

### 4. Audit Logging

All deployments log:
- Who deployed (`DEPLOYED_BY`)
- When deployed (`DEPLOYED_AT`)
- What version (`GIT_COMMIT`)
- Where deployed (`ENVIRONMENT`)

Logs stored in:
- `~/.togather/logs/deployments/`
- `~/.togather/logs/deployments/`

### 5. Rollback Plan

Always have rollback capability:

```yaml
- name: Rollback on failure
  if: failure()
  run: |
    cd deploy/scripts
    server deploy rollback production --force
```

---

## Deployment Strategies

### Strategy 1: Direct Deployment

**Use when**: Single production environment, low traffic

```yaml
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - # ... setup
      - run: ./deploy/scripts/deploy.sh production
```

**Pros**: Simple, fast  
**Cons**: Immediate production impact

---

### Strategy 2: Staging → Production

**Use when**: Need validation before production

```yaml
jobs:
  deploy-staging:
    # ... deploy to staging
  
  deploy-production:
    needs: deploy-staging
    # ... deploy to production
```

**Pros**: Safer, catch issues in staging  
**Cons**: Slower, requires staging environment

---

### Strategy 3: Manual Approval

**Use when**: Critical production changes need human approval

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: production  # Requires manual approval
    steps:
      - run: ./deploy/scripts/deploy.sh production
```

Configure in GitHub: Settings → Environments → Required reviewers

**Pros**: Human oversight, control  
**Cons**: Slower, requires human availability

---

### Strategy 4: Canary Deployment

**Use when**: High-traffic production, gradual rollout desired

```yaml
jobs:
  deploy-canary:
    # Deploy to 10% of servers
    - run: ./deploy/scripts/deploy.sh production-canary
  
  verify-canary:
    needs: deploy-canary
    # Monitor metrics for 10 minutes
  
  deploy-full:
    needs: verify-canary
    # Deploy to remaining 90%
    - run: ./deploy/scripts/deploy.sh production
```

**Pros**: Minimize blast radius, gradual rollout  
**Cons**: Complex, requires multiple server sets

---

## Monitoring Deployment Success

### Health Check Validation

```bash
# Built into deployment script
./deploy/scripts/deploy.sh production

# Manual verification
./deploy/scripts/health-check.sh production

# Check specific endpoint
curl -f http://localhost:8080/health
```

### Smoke Tests

```yaml
- name: Run smoke tests
  run: |
    # Critical endpoints
    curl -f http://localhost:8080/health
    curl -f http://localhost:8080/api/v1/events
    curl -f http://localhost:8080/api/v1/places
    
    # Check version endpoint
    VERSION=$(curl -s http://localhost:8080/version | jq -r '.commit')
    echo "Deployed version: $VERSION"
    
    # Verify it matches expected commit
    test "$VERSION" = "$GITHUB_SHA"
```

### Integration Tests

```yaml
- name: Run integration tests
  run: |
    cd tests/integration
    go test -v -tags=integration ./...
```

---

## Complete Production Examples

This section provides complete, copy-paste ready CI/CD configurations that include all best practices.

### GitHub Actions: Production-Ready Pipeline

Complete `.github/workflows/production-deploy.yml`:

```yaml
name: Production Deployment

on:
  push:
    branches:
      - main
  workflow_dispatch:

env:
  DOCKER_BUILDKIT: 1

jobs:
  test:
    name: Run Tests
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      
      - name: Run unit tests
        run: make test
      
      - name: Run linter
        run: make lint-ci
  
  deploy:
    name: Deploy to Production
    runs-on: ubuntu-latest
    needs: test
    environment:
      name: production
      url: https://api.togather.example.com
    
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for rollback
      
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
      
      - name: Install deployment dependencies
        run: |
          # Install golang-migrate
          curl -sL https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz
          sudo mv migrate /usr/local/bin/
          
          # Install PostgreSQL client
          sudo apt-get update
          sudo apt-get install -y postgresql-client jq
          
          # Verify installations
          migrate -version
          psql --version
          docker --version
      
      - name: Configure production environment
        env:
          DATABASE_URL: ${{ secrets.PROD_DATABASE_URL }}
          JWT_SECRET: ${{ secrets.PROD_JWT_SECRET }}
          POSTGRES_HOST: ${{ secrets.PROD_POSTGRES_HOST }}
          POSTGRES_PORT: ${{ secrets.PROD_POSTGRES_PORT }}
          POSTGRES_DB: ${{ secrets.PROD_POSTGRES_DB }}
          POSTGRES_USER: ${{ secrets.PROD_POSTGRES_USER }}
          POSTGRES_PASSWORD: ${{ secrets.PROD_POSTGRES_PASSWORD }}
        run: |
          # .env files live on the server under /opt/togather
          
          cat > /opt/togather/.env.production <<EOF
          # Environment
          ENVIRONMENT=production
          DEPLOYED_BY=${{ github.actor }}@github-actions
          DEPLOYED_AT=$(date -Iseconds)
          GIT_COMMIT=${{ github.sha }}
          
          # Database
          POSTGRES_HOST=${POSTGRES_HOST}
          POSTGRES_PORT=${POSTGRES_PORT}
          POSTGRES_DB=${POSTGRES_DB}
          POSTGRES_USER=${POSTGRES_USER}
          POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
          POSTGRES_SSLMODE=verify-full
          DB_MAX_CONNECTIONS=50
          
          # Application
          SERVER_HOST=0.0.0.0
          SERVER_PORT=8080
          LOG_LEVEL=info
          SHUTDOWN_TIMEOUT=30s
          
          # Security
          JWT_SECRET=${JWT_SECRET}
          TLS_ENABLED=true
          
          # Snapshots
          SNAPSHOT_ENABLED=true
          RETENTION_DAYS=30
          EOF
          
          chmod 600 /opt/togather/.env.production
      
      - name: Pre-deployment checks
        run: |
          # Verify no CHANGE_ME placeholders
          if grep -r "CHANGE_ME" /opt/togather/.env.production; then
            echo "ERROR: CHANGE_ME placeholders found in configuration"
            exit 1
          fi
          
          # Test database connectivity
          psql "${{ secrets.PROD_DATABASE_URL }}" -c "SELECT 1" || {
            echo "ERROR: Cannot connect to database"
            exit 1
          }
          
          # Check disk space
          df -h | grep -v "tmpfs" | awk '{if (NR>1 && int($5) > 80) exit 1}'
      
      - name: Create pre-deployment snapshot
        env:
          DATABASE_URL: ${{ secrets.PROD_DATABASE_URL }}
        run: |
          server snapshot create --reason "pre-deploy-${{ github.sha }}"
      
      - name: Deploy to production
        id: deploy
        run: |
          cd deploy/scripts
          ./deploy.sh production 2>&1 | tee /tmp/deploy.log
        timeout-minutes: 30
      
      - name: Post-deployment verification
        if: steps.deploy.outcome == 'success'
        run: |
          echo "Waiting for application to stabilize..."
          sleep 15
          
          # Health check with retry
          for i in {1..10}; do
            if curl -f -s http://localhost:8080/health | jq -e '.status == "healthy"'; then
              echo "✓ Health check passed"
              break
            fi
            if [ $i -eq 10 ]; then
              echo "✗ Health check failed after 10 attempts"
              exit 1
            fi
            echo "Retry $i/10..."
            sleep 10
          done
          
          # Smoke tests
          curl -f http://localhost:8080/api/v1/events || exit 1
          echo "✓ Events API responding"
          
          curl -f http://localhost:8080/api/v1/places || exit 1
          echo "✓ Places API responding"
          
          curl -f http://localhost:8080/api/v1/organizations || exit 1
          echo "✓ Organizations API responding"
      
      - name: Rollback on failure
        if: failure() && steps.deploy.outcome == 'failure'
        run: |
          echo "Deployment failed, initiating automatic rollback..."
          server deploy rollback production --force
          
          # Wait for rollback to complete
          sleep 15
          
          # Verify rollback health
          curl -f http://localhost:8080/health || {
            echo "ERROR: Rollback failed to restore service"
            exit 1
          }
          
          echo "✓ Rolled back successfully"
      
      - name: Upload deployment logs
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: deployment-logs-${{ github.sha }}
          path: |
            /tmp/deploy.log
            ~/.togather/logs/deployments/
          retention-days: 30
      
      - name: Notify deployment status
        if: always()
        run: |
          if [ "${{ job.status }}" == "success" ]; then
            echo "✓ Deployment successful"
            # Add Slack/email notification here
          else
            echo "✗ Deployment failed"
            # Add Slack/email notification here
          fi
```

---

### GitLab CI: Complete Production Pipeline

Complete `.gitlab-ci.yml`:

```yaml
stages:
  - test
  - deploy
  - verify
  - notify

variables:
  DOCKER_DRIVER: overlay2
  DOCKER_BUILDKIT: "1"
  FF_USE_FASTZIP: "true"
  CACHE_COMPRESSION_LEVEL: "fastest"

.install_deps: &install_deps
  - apt-get update -qq
  - apt-get install -y -qq curl docker.io postgresql-client jq
  - curl -sL https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz
  - mv migrate /usr/local/bin/
  - migrate -version

test:
  stage: test
  image: golang:1.25
  script:
    - go version
    - make test
    - make lint-ci
  only:
    - main
    - merge_requests

deploy-production:
  stage: deploy
  image: ubuntu:22.04
  environment:
    name: production
    url: https://api.togather.example.com
    on_stop: rollback-production
  only:
    - main
  before_script:
    - *install_deps
  script:
    # Configure environment
    - |
      cat > /opt/togather/.env.production <<EOF
      ENVIRONMENT=production
      DEPLOYED_BY=${GITLAB_USER_LOGIN}@gitlab-ci
      DEPLOYED_AT=$(date -Iseconds)
      GIT_COMMIT=${CI_COMMIT_SHA}
      
      # Database
      POSTGRES_HOST=${PROD_POSTGRES_HOST}
      POSTGRES_PORT=${PROD_POSTGRES_PORT}
      POSTGRES_DB=${PROD_POSTGRES_DB}
      POSTGRES_USER=${PROD_POSTGRES_USER}
      POSTGRES_PASSWORD=${PROD_POSTGRES_PASSWORD}
      POSTGRES_SSLMODE=verify-full
      DB_MAX_CONNECTIONS=50
      
      # Application
      SERVER_HOST=0.0.0.0
      SERVER_PORT=8080
      LOG_LEVEL=info
      SHUTDOWN_TIMEOUT=30s
      
      # Security
      JWT_SECRET=${PROD_JWT_SECRET}
      TLS_ENABLED=true
      
      # Snapshots
      SNAPSHOT_ENABLED=true
      RETENTION_DAYS=30
      EOF
    - chmod 600 /opt/togather/.env.production
    
    # Pre-deployment checks
    - |
      if grep -r "CHANGE_ME" /opt/togather/.env.production; then
        echo "ERROR: CHANGE_ME placeholders found"
        exit 1
      fi
    - psql "${PROD_DATABASE_URL}" -c "SELECT 1"
    
    # Create snapshot
    - server snapshot create --reason "pre-deploy-${CI_COMMIT_SHA}"
    
    # Deploy
    - cd deploy/scripts
    - ./deploy.sh production
  after_script:
    # Always upload logs
    - mkdir -p deploy-logs
    - cp -r ~/.togather/logs/deployments/* deploy-logs/ || true
  artifacts:
    paths:
      - deploy-logs/
    expire_in: 30 days
    when: always
  retry:
    max: 1
    when:
      - runner_system_failure
      - stuck_or_timeout_failure

verify-production:
  stage: verify
  image: curlimages/curl:latest
  environment:
    name: production
  dependencies: []
  only:
    - main
  script:
    - echo "Waiting for application to stabilize..."
    - sleep 15
    
    # Health check with retries
    - |
      for i in $(seq 1 10); do
        if curl -f -s http://localhost:8080/health | grep -q "healthy"; then
          echo "✓ Health check passed"
          break
        fi
        if [ $i -eq 10 ]; then
          echo "✗ Health check failed"
          exit 1
        fi
        echo "Retry $i/10..."
        sleep 10
      done
    
    # Smoke tests
    - curl -f http://localhost:8080/api/v1/events
    - curl -f http://localhost:8080/api/v1/places
    - curl -f http://localhost:8080/api/v1/organizations

rollback-production:
  stage: deploy
  image: ubuntu:22.04
  environment:
    name: production
    action: stop
  when: manual
  before_script:
    - *install_deps
  script:
    - cd deploy/scripts
    - server deploy rollback production --force
    - sleep 15
    - curl -f http://localhost:8080/health

notify-success:
  stage: notify
  image: alpine:latest
  only:
    - main
  when: on_success
  script:
    - echo "✓ Deployment to production succeeded"
    # Add Slack/email notification

notify-failure:
  stage: notify
  image: ubuntu:22.04
  only:
    - main
  when: on_failure
  before_script:
    - *install_deps
  script:
    - echo "✗ Deployment to production failed, rolling back..."
    - cd deploy/scripts
    - server deploy rollback production --force
    # Add Slack/email notification
```

---

### Jenkins: Production Pipeline with Manual Approval

Complete `Jenkinsfile`:

```groovy
pipeline {
    agent any
    
    options {
        buildDiscarder(logRotator(numToKeepStr: '30'))
        disableConcurrentBuilds()
        timeout(time: 1, unit: 'HOURS')
        timestamps()
    }
    
    environment {
        DOCKER_BUILDKIT = '1'
        PATH = "$PATH:/usr/local/bin"
        DEPLOYED_BY = "${env.BUILD_USER ?: 'jenkins'}@jenkins"
        DEPLOYED_AT = sh(returnStdout: true, script: 'date -Iseconds').trim()
        
        // Credentials (configured in Jenkins)
        PROD_DATABASE_URL = credentials('prod-database-url')
        PROD_JWT_SECRET = credentials('prod-jwt-secret')
        PROD_POSTGRES_HOST = credentials('prod-postgres-host')
        PROD_POSTGRES_PORT = credentials('prod-postgres-port')
        PROD_POSTGRES_DB = credentials('prod-postgres-db')
        PROD_POSTGRES_USER = credentials('prod-postgres-user')
        PROD_POSTGRES_PASSWORD = credentials('prod-postgres-password')
    }
    
    stages {
        stage('Checkout') {
            steps {
                checkout scm
                sh 'git fetch --tags'
            }
        }
        
        stage('Install Dependencies') {
            steps {
                sh '''
                    # Install golang-migrate
                    curl -sL https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz
                    sudo mv migrate /usr/local/bin/
                    
                    # Verify installations
                    migrate -version
                    docker --version
                    psql --version
                    jq --version
                '''
            }
        }
        
        stage('Test') {
            parallel {
                stage('Unit Tests') {
                    steps {
                        sh 'make test'
                    }
                }
                stage('Lint') {
                    steps {
                        sh 'make lint-ci'
                    }
                }
            }
        }
        
        stage('Approval') {
            when {
                branch 'main'
            }
            steps {
                script {
                    def deploymentInfo = """
                    Commit: ${env.GIT_COMMIT}
                    Author: ${env.GIT_AUTHOR_NAME}
                    Message: ${env.GIT_COMMIT_MSG}
                    """
                    
                    timeout(time: 1, unit: 'HOURS') {
                        input message: 'Deploy to Production?', 
                              ok: 'Deploy',
                              parameters: [
                                  text(name: 'DEPLOYMENT_NOTES', 
                                       description: 'Deployment notes (optional)',
                                       defaultValue: '')
                              ],
                              submitterParameter: 'APPROVER'
                    }
                }
            }
        }
        
        stage('Configure Production') {
            when {
                branch 'main'
            }
            steps {
                sh '''
                    # .env files live on the server under /opt/togather
                    
                    cat > /opt/togather/.env.production <<EOF
ENVIRONMENT=production
DEPLOYED_BY=${DEPLOYED_BY}
DEPLOYED_AT=${DEPLOYED_AT}
GIT_COMMIT=${GIT_COMMIT}

# Database
POSTGRES_HOST=${PROD_POSTGRES_HOST}
POSTGRES_PORT=${PROD_POSTGRES_PORT}
POSTGRES_DB=${PROD_POSTGRES_DB}
POSTGRES_USER=${PROD_POSTGRES_USER}
POSTGRES_PASSWORD=${PROD_POSTGRES_PASSWORD}
POSTGRES_SSLMODE=verify-full
DB_MAX_CONNECTIONS=50

# Application
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
LOG_LEVEL=info
SHUTDOWN_TIMEOUT=30s

# Security
JWT_SECRET=${PROD_JWT_SECRET}
TLS_ENABLED=true

# Snapshots
SNAPSHOT_ENABLED=true
RETENTION_DAYS=30
EOF
                    
                    chmod 600 /opt/togather/.env.production
                '''
            }
        }
        
        stage('Pre-Deployment Checks') {
            when {
                branch 'main'
            }
            steps {
                sh '''
                    # Verify no placeholders
                    if grep -r "CHANGE_ME" /opt/togather/.env.production; then
                        echo "ERROR: CHANGE_ME placeholders found"
                        exit 1
                    fi
                    
                    # Test database connectivity
                    psql "${PROD_DATABASE_URL}" -c "SELECT 1"
                    
                    # Check disk space
                    df -h | grep -v "tmpfs" | awk '{if (NR>1 && int($5) > 80) exit 1}'
                '''
            }
        }
        
        stage('Create Snapshot') {
            when {
                branch 'main'
            }
            steps {
                sh '''
                    server snapshot create --reason "pre-deploy-${GIT_COMMIT}"
                '''
            }
        }
        
        stage('Deploy') {
            when {
                branch 'main'
            }
            steps {
                sh '''
                    cd deploy/scripts
                    ./deploy.sh production 2>&1 | tee /tmp/deploy.log
                '''
            }
        }
        
        stage('Verify Deployment') {
            when {
                branch 'main'
            }
            steps {
                sh '''
                    echo "Waiting for application to stabilize..."
                    sleep 15
                    
                    # Health check with retry
                    for i in {1..10}; do
                        if curl -f -s http://localhost:8080/health | jq -e '.status == "healthy"'; then
                            echo "✓ Health check passed"
                            break
                        fi
                        if [ $i -eq 10 ]; then
                            echo "✗ Health check failed"
                            exit 1
                        fi
                        echo "Retry $i/10..."
                        sleep 10
                    done
                    
                    # Smoke tests
                    curl -f http://localhost:8080/api/v1/events
                    curl -f http://localhost:8080/api/v1/places
                    curl -f http://localhost:8080/api/v1/organizations
                '''
            }
        }
    }
    
    post {
        failure {
            script {
                if (env.BRANCH_NAME == 'main') {
                    sh '''
                        echo "Deployment failed, initiating rollback..."
                        cd deploy/scripts
                        server deploy rollback production --force
                        
                        # Verify rollback
                        sleep 15
                        curl -f http://localhost:8080/health
                    '''
                }
            }
            // Add email/Slack notification
            emailext(
                subject: "Deployment FAILED: ${env.JOB_NAME} #${env.BUILD_NUMBER}",
                body: """
                Deployment to production failed and was rolled back.
                
                Job: ${env.JOB_NAME}
                Build: ${env.BUILD_NUMBER}
                Commit: ${env.GIT_COMMIT}
                
                Check console output: ${env.BUILD_URL}
                """,
                to: 'ops-team@example.com'
            )
        }
        success {
            // Add email/Slack notification
            emailext(
                subject: "Deployment SUCCESS: ${env.JOB_NAME} #${env.BUILD_NUMBER}",
                body: """
                Deployment to production completed successfully.
                
                Job: ${env.JOB_NAME}
                Build: ${env.BUILD_NUMBER}
                Commit: ${env.GIT_COMMIT}
                Deployed by: ${env.APPROVER}
                """,
                to: 'ops-team@example.com'
            )
        }
        always {
            // Archive deployment logs
            archiveArtifacts(
                artifacts: '/tmp/deploy.log,~/.togather/logs/deployments/**/*',
                allowEmptyArchive: true,
                fingerprint: true
            )
        }
    }
}
```

---

## Troubleshooting

### Deployment Fails

1. Check deployment logs:
   ```bash
   tail -f ~/.togather/logs/deployments/deploy_*.log
   ```

2. Check deployment state:
   ```bash
   cat deploy/config/deployment-state.json | jq .
   ```

3. Check container status:
   ```bash
   docker ps -a | grep togather
   docker logs togather-production-blue
   ```

### Health Checks Fail

1. Check application logs:
   ```bash
   docker logs togather-production-blue
   ```

2. Test database connectivity:
   ```bash
   psql $DATABASE_URL -c "SELECT 1"
   ```

3. Manual health check:
   ```bash
   curl -v http://localhost:8080/health
   ```

### Rollback Needed

```bash
# Automatic (with prompt)
server deploy rollback production

# Force (no prompt)
server deploy rollback production --force

# Specific version
server deploy rollback production --version abc123
```

---

## Related Documentation

- **Deployment Guide**: [quickstart.md](./quickstart.md)
- **Rollback Guide**: [rollback.md](./rollback.md)
- **Migration Guide**: [migrations.md](./migrations.md)
- **Testing**: [../../tests/deployment/README.md](../../tests/deployment/README.md)

---

**Last Updated**: 2026-01-28  
**Related Task**: T076 - CI/CD Integration Examples
