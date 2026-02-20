# Webfiles Automation Testing Guide

## Overview

The webfiles automation (`server webfiles` command) generates `robots.txt` and `sitemap.xml` with environment-specific domains during deployment. This document describes how to test and verify the automation.

## Testing Locally

### 1. Test webfiles command directly

```bash
# Test production domain
./server webfiles --domain togather.foundation
cat web/robots.txt | grep Sitemap
# Should show: Sitemap: https://togather.foundation/sitemap.xml

# Test staging domain
./server webfiles --domain staging.toronto.togather.foundation
cat web/robots.txt | grep Sitemap
# Should show: Sitemap: https://staging.toronto.togather.foundation/sitemap.xml

# Test development domain
./server webfiles --domain localhost:8080
cat web/robots.txt | grep Sitemap
# Should show: Sitemap: https://localhost:8080/sitemap.xml
```

### 2. Test deployment script integration

```bash
# Source the deploy.sh to test the function
source ./deploy/scripts/deploy.sh

# Test generate_web_files for each environment
generate_web_files "production"
generate_web_files "staging"
generate_web_files "development"
```

### 3. Verify embedded content

```bash
# Generate webfiles for target environment
./server webfiles --domain staging.toronto.togather.foundation

# Rebuild binary
make build

# Verify embedded content
strings bin/togather-server | grep "staging.toronto.togather.foundation"
```

## Testing After Deployment

### Smoke Tests

The smoke test script (`tests/deployment/smoke_test.sh`) includes automated verification of robots.txt and sitemap.xml:

```bash
# Test against staging
./tests/deployment/smoke_test.sh https://staging.toronto.togather.foundation

# Test against production
./tests/deployment/smoke_test.sh https://togather.foundation

# Test locally
./tests/deployment/smoke_test.sh http://localhost:8080
```

The smoke tests verify:
- `robots.txt` contains correct domain in Sitemap directive
- `sitemap.xml` contains valid XML with correct domain in all URLs
- Files are accessible via HTTP endpoints

### Manual Verification

After deploying to staging or production:

```bash
# Check robots.txt
curl https://staging.toronto.togather.foundation/robots.txt

# Verify sitemap URL matches environment
# Should show: Sitemap: https://staging.toronto.togather.foundation/sitemap.xml

# Check sitemap.xml  
curl https://staging.toronto.togather.foundation/sitemap.xml

# Verify all URLs use correct domain
# All <loc> tags should contain: https://staging.toronto.togather.foundation/...
```

## Deployment Flow

The webfiles are automatically generated during deployment:

1. **deploy.sh** calls `generate_web_files()` before Docker build
2. `generate_web_files()` determines domain based on environment:
   - `production` → `togather.foundation`
   - `staging` → `staging.toronto.togather.foundation`
   - `development` → `localhost:8080`
3. Runs `server webfiles --domain <domain>` to generate files
4. Files are written to `web/` directory
5. Docker build embeds files using `go:embed` directives in `web/static.go`
6. Deployed server serves embedded files via `/robots.txt` and `/sitemap.xml`

## Troubleshooting

### Files have wrong domain after deployment

**Cause**: Web files were generated for wrong environment before Docker build.

**Solution**: 
```bash
# Regenerate for correct environment
./server webfiles --domain <correct-domain>

# Rebuild and redeploy
make build
./deploy/scripts/deploy.sh <environment>
```

### Binary doesn't contain updated files

**Cause**: Go embed cache not cleared after file changes.

**Solution**:
```bash
# Clean build
rm -rf bin/
make build
```

### Smoke tests fail after deployment

**Cause**: Server may still be starting or files weren't embedded correctly.

**Solution**:
```bash
# Wait for server health
curl https://<domain>/health

# Manually verify files
curl https://<domain>/robots.txt
curl https://<domain>/sitemap.xml

# Check server logs
docker logs togather-server-<slot>
```

## CI/CD Integration

The webfiles automation is integrated into the deployment pipeline:

1. **Local deployment**: `deploy.sh` generates files automatically
2. **Remote deployment**: `deploy.sh --remote` generates files on remote server
3. **Docker build**: Files are embedded at build time
4. **Health checks**: Deployment validates server is healthy before traffic switch
5. **Smoke tests**: Can be run post-deployment to verify correctness

---

**Last Updated:** 2026-02-20
