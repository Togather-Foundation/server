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
          ADMIN_API_KEY: ${{ secrets.ADMIN_API_KEY }}
        run: |
          # Create production environment file
          cat > deploy/config/environments/.env.production <<EOF
          ENVIRONMENT=production
          DATABASE_URL=$DATABASE_URL
          JWT_SECRET=$JWT_SECRET
          ADMIN_API_KEY=$ADMIN_API_KEY
          SERVER_PORT=8080
          LOG_LEVEL=info
          SHUTDOWN_TIMEOUT=30s
          DEPLOYED_BY=${{ github.actor }}@github-actions
          EOF
          
          chmod 600 deploy/config/environments/.env.production
      
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
          ADMIN_API_KEY: ${{ secrets.ADMIN_API_KEY }}
        run: |
          cat > deploy/config/environments/.env.production <<EOF
          ENVIRONMENT=production
          DATABASE_URL=$DATABASE_URL
          JWT_SECRET=$JWT_SECRET
          ADMIN_API_KEY=$ADMIN_API_KEY
          DEPLOYED_BY=${{ github.actor }}@github-actions
          EOF
          chmod 600 deploy/config/environments/.env.production
      
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
          cd deploy/scripts
          ./rollback.sh production --force
      
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
      
      - name: Configure staging
        env:
          DATABASE_URL: ${{ secrets.STAGING_DATABASE_URL }}
          JWT_SECRET: ${{ secrets.STAGING_JWT_SECRET }}
          ADMIN_API_KEY: ${{ secrets.STAGING_ADMIN_API_KEY }}
        run: |
          cat > deploy/config/environments/.env.staging <<EOF
          ENVIRONMENT=staging
          DATABASE_URL=$DATABASE_URL
          JWT_SECRET=$JWT_SECRET
          ADMIN_API_KEY=$ADMIN_API_KEY
          EOF
          chmod 600 deploy/config/environments/.env.staging
      
      - name: Deploy to staging
        run: |
          cd deploy/scripts
          ./deploy.sh staging
      
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
          ADMIN_API_KEY: ${{ secrets.ADMIN_API_KEY }}
        run: |
          cat > deploy/config/environments/.env.production <<EOF
          ENVIRONMENT=production
          DATABASE_URL=$DATABASE_URL
          JWT_SECRET=$JWT_SECRET
          ADMIN_API_KEY=$ADMIN_API_KEY
          EOF
          chmod 600 deploy/config/environments/.env.production
      
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
      cat > deploy/config/environments/.env.production <<EOF
      ENVIRONMENT=production
      DATABASE_URL=$DATABASE_URL
      JWT_SECRET=$JWT_SECRET
      ADMIN_API_KEY=$ADMIN_API_KEY
      DEPLOYED_BY=$GITLAB_USER_LOGIN@gitlab-ci
      EOF
    - chmod 600 deploy/config/environments/.env.production
    
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
        ADMIN_API_KEY = credentials('production-admin-api-key')
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
                    cat > deploy/config/environments/.env.production <<EOF
                    ENVIRONMENT=production
                    DATABASE_URL=$DATABASE_URL
                    JWT_SECRET=$JWT_SECRET
                    ADMIN_API_KEY=$ADMIN_API_KEY
                    DEPLOYED_BY=${BUILD_USER}@jenkins
                    EOF
                    
                    chmod 600 deploy/config/environments/.env.production
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
                ./rollback.sh production --force
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
- `ADMIN_API_KEY` - Admin API key (generate: `openssl rand -hex 32`)

**Staging:**
- `STAGING_DATABASE_URL`
- `STAGING_JWT_SECRET`
- `STAGING_ADMIN_API_KEY`

**Development:**
- `DEV_DATABASE_URL`
- `DEV_JWT_SECRET`
- `DEV_ADMIN_API_KEY`

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
deploy/config/environments/.env.*
!deploy/config/environments/.env.*.example
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
- `/var/lib/togather/deployments/{env}/`

### 5. Rollback Plan

Always have rollback capability:

```yaml
- name: Rollback on failure
  if: failure()
  run: |
    cd deploy/scripts
    ./rollback.sh production --force
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
./deploy/scripts/rollback.sh production

# Force (no prompt)
./deploy/scripts/rollback.sh production --force

# Specific version
./deploy/scripts/rollback.sh production --version abc123
```

---

## Related Documentation

- **Deployment Guide**: [quickstart.md](./quickstart.md)
- **Rollback Guide**: [rollback.md](./rollback.md)
- **Migration Guide**: [migrations.md](./migrations.md)
- **Testing**: [../tests/deployment/README.md](../tests/deployment/README.md)

---

**Last Updated**: 2026-01-28  
**Related Task**: T076 - CI/CD Integration Examples
