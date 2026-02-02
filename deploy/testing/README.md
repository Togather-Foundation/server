# Remote Testing Infrastructure

Unified testing infrastructure for running smoke tests and performance tests against local, staging, and production servers.

## Quick Start

```bash
# Test local server
make test-local

# Test staging (smoke tests only)
make test-staging-smoke

# Test staging (all tests)
make test-staging

# Test production (read-only smoke tests)
make test-production-smoke
```

## Directory Structure

```
deploy/testing/
├── README.md                    # This file
├── config.sh                    # Environment config loader
├── smoke-tests.sh              # Smoke test suite
└── environments/
    ├── local.test.env          # Local server config
    ├── staging.test.env        # Staging server config
    └── production.test.env     # Production server config
```

## Configuration Files

### Environment Configs (`environments/*.test.env`)

Each environment has its own configuration file that defines:

- **BASE_URL**: Server URL to test
- **TIMEOUT**: Request timeout in seconds
- **RETRY_COUNT**: Number of retries for flaky operations
- **MAX_RESPONSE_TIME_MS**: Maximum acceptable response time
- **ALLOW_DESTRUCTIVE**: Whether tests can create/delete data
- **ALLOW_LOAD_TESTING**: Whether performance tests are allowed
- **API_KEY**: (Optional) API key for authenticated endpoints
- **JWT_TOKEN**: (Optional) JWT token for authentication

**⚠️ IMPORTANT**: Never commit real API keys or secrets to these files. Use environment variables or a separate `.env.local` file for secrets.

### Local Environment

```bash
# environments/local.test.env
BASE_URL=http://localhost:8080
ALLOW_DESTRUCTIVE=true
ALLOW_LOAD_TESTING=true
```

### Staging Environment

```bash
# environments/staging.test.env
BASE_URL=https://staging.toronto.togather.foundation
ALLOW_DESTRUCTIVE=true
ALLOW_LOAD_TESTING=true
```

### Production Environment

```bash
# environments/production.test.env
BASE_URL=https://toronto.togather.foundation
ALLOW_DESTRUCTIVE=false  # ⚠️ NEVER true in production
ALLOW_LOAD_TESTING=false # ⚠️ NEVER true in production
```

## Usage

### Using Makefile (Recommended)

```bash
# Test local server (all tests)
make test-local

# Test staging server (all tests)
make test-staging

# Test staging server (smoke tests only)
make test-staging-smoke

# Test production server (smoke tests only)
make test-production-smoke

# Custom test (specify environment and type)
make test-remote ENV=staging TYPE=smoke
```

### Using Scripts Directly

```bash
# Via wrapper script
./deploy/scripts/test-remote.sh local smoke
./deploy/scripts/test-remote.sh staging all
./deploy/scripts/test-remote.sh production smoke

# Via smoke test script directly
./deploy/testing/smoke-tests.sh local
./deploy/testing/smoke-tests.sh staging
./deploy/testing/smoke-tests.sh production

# With custom BASE_URL
BASE_URL=http://192.46.222.199:8080 ./deploy/testing/smoke-tests.sh
```

## Test Types

### Smoke Tests

Quick validation tests that verify basic functionality:

- ✅ Health endpoint responds
- ✅ Version endpoint responds
- ✅ Database connectivity
- ✅ Migration status
- ✅ HTTP endpoint health
- ✅ CORS headers (if applicable)
- ✅ Security headers
- ✅ Response time within threshold

**Duration**: ~10-30 seconds

**Safe for production**: Yes (read-only)

### Performance Tests

Light load testing to verify performance under load:

- Tests request throughput
- Measures response times under load
- Verifies server stability

**Duration**: ~1-5 minutes

**Safe for production**: ⚠️ **NO** - Only run on staging/local

### All Tests

Runs both smoke tests and performance tests sequentially.

## Production Safety

The testing infrastructure includes multiple safety mechanisms for production:

1. **Config-level protection**: Production config has `ALLOW_DESTRUCTIVE=false` and `ALLOW_LOAD_TESTING=false`

2. **Script-level validation**: Scripts check environment and refuse destructive operations

3. **Explicit enforcement**: Test wrapper blocks performance tests on production

4. **Read-only mode**: Production tests only verify endpoints respond, no data modifications

## Authentication

If your endpoints require authentication, set credentials via:

### Option 1: Environment Variables

```bash
export API_KEY=your_staging_api_key
export JWT_TOKEN=your_staging_jwt_token
make test-staging-smoke
```

### Option 2: Local Override File

Create `environments/staging.test.env.local` (gitignored):

```bash
# environments/staging.test.env.local
API_KEY=your_staging_api_key
JWT_TOKEN=your_staging_jwt_token
```

Then source it before tests:

```bash
source deploy/testing/environments/staging.test.env.local
make test-staging-smoke
```

### Option 3: CI/CD Secrets

In GitHub Actions or other CI:

```yaml
- name: Run staging smoke tests
  env:
    API_KEY: ${{ secrets.STAGING_API_KEY }}
    JWT_TOKEN: ${{ secrets.STAGING_JWT_TOKEN }}
  run: make test-staging-smoke
```

## Integration with Deployment

### Blue-Green Deployment Workflow

The recommended deployment workflow with smoke tests:

1. **Deploy to inactive slot** (blue or green)
2. **Run smoke tests** against the new deployment
3. **If tests pass**: Switch traffic to new slot
4. **If tests fail**: Keep traffic on old slot, investigate

```bash
# Example deployment workflow
./deploy/scripts/deploy.sh production

# Deployment script should:
# 1. Deploy to inactive slot
# 2. Run: make test-production-smoke (against new slot)
# 3. If success: switch Caddy config
# 4. If failure: rollback, alert
```

### Post-Deployment Verification

After switching traffic, run smoke tests again to verify:

```bash
# After traffic switch
make test-production-smoke
```

This confirms the production slot is serving traffic correctly.

## Exit Codes

All scripts follow standard exit code conventions:

- **0**: All tests passed
- **1**: One or more tests failed
- **2**: Invalid arguments or configuration error

## Dependencies

Required tools:

- `curl` - For HTTP requests
- `jq` - For JSON parsing
- `bash` 4.0+ - For script execution

## Troubleshooting

### Tests Fail with "Connection Refused"

- Check BASE_URL is correct
- Verify server is running
- Check firewall rules

### Tests Fail with "Timeout"

- Increase TIMEOUT in environment config
- Check server performance
- Verify network connectivity

### Tests Pass Locally but Fail in CI

- Check CI environment has curl and jq
- Verify network access from CI to server
- Check authentication credentials in CI secrets

### Authentication Errors

- Verify API_KEY or JWT_TOKEN is set
- Check credentials are valid and not expired
- Ensure credentials match the environment (staging vs production)

## Extending

### Adding New Tests

To add new smoke tests:

1. Edit `deploy/testing/smoke-tests.sh`
2. Add new test function: `test_your_feature()`
3. Call it from `main()` function
4. Follow existing patterns for logging and error handling

### Adding New Environments

To add a new environment (e.g., `qa`):

1. Create `deploy/testing/environments/qa.test.env`
2. Set BASE_URL and other config
3. Add Makefile target:
   ```makefile
   test-qa-smoke:
       @./deploy/scripts/test-remote.sh qa smoke
   ```

### Custom Test Scripts

To add custom test types:

1. Create script in `deploy/testing/`
2. Source `config.sh` for environment config
3. Use same exit code conventions
4. Add to `test-remote.sh` wrapper

## Future Improvements

See epic `server-khvc` for planned comprehensive .env refactor:

- Separate runtime, deploy, and testing configs
- Create `.deploy.conf` files (no secrets)
- Consolidate all .env templates
- Migration guide for existing deployments

## Related Documentation

- [Deployment Quickstart](../../docs/deploy/quickstart.md)
- [Blue-Green Deployment](../../docs/deploy/blue-green.md)
- [CI/CD Pipeline](../../.github/workflows/)
- [Original Smoke Tests](../../tests/deployment/) (deprecated)
