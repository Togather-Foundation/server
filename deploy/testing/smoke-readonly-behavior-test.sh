#!/usr/bin/env bash
# Exercise the real River smoke function without network requests or fixture writes.
# Extracted production functions consume these globals and mocks indirectly.
# shellcheck disable=SC2034,SC2329
set -euo pipefail
command -v jq >/dev/null || { echo 'FAIL: required assertion tool missing: jq' >&2; exit 1; }
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT
sed -n '/^test_river_worker_health()/,/^test_container_health()/p' "$ROOT/deploy/testing/smoke-tests.sh" | sed '$d' > "$TEST_DIR/function.sh"
# shellcheck source=/dev/null
source "$TEST_DIR/function.sh"
log() { :; }
sleep() { :; }
http_get() {
    if [[ "$1" == */health ]]; then
        printf '{"checks":{"job_queue":{"status":"pass"}}}\n'
    else
        printf '{"status":"completed","total":1,"created":1,"failed":0}\n'
    fi
}
curl() {
    printf 'POST\n' >> "$TEST_DIR/requests"
    printf '{"batch_id":"fixture-batch"}\n202\n'
}
BASE_URL=http://unused.invalid
TIMEOUT=1
TESTS_TOTAL=20
run_case() {
    local label="$1" environment="$2" opt_in="$3" destructive="$4" key="$5" expected_status="$6" expected_posts="$7"
    ENVIRONMENT="$environment"
    SMOKE_WRITE_TESTS="$opt_in"
    ALLOW_DESTRUCTIVE="$destructive"
    API_KEY="$key"
    TESTS_RUN=0 TESTS_PASSED=0 TESTS_FAILED=0
    : > "$TEST_DIR/requests"
    local status=0
    test_river_worker_health || status=$?
    local posts
    posts=$(wc -l < "$TEST_DIR/requests")
    if [[ "$status" != "$expected_status" || "$posts" != "$expected_posts" ]]; then
        echo "FAIL: $label (status=$status posts=$posts)"
        exit 1
    fi
    echo "PASS: $label"
}
run_case 'configured staging credentials do not enable writes' staging '' true fixture-key 0 0
run_case 'local smoke defaults to read-only' local false true fixture-key 0 0
run_case 'explicit write mode requires destructive permission' staging true false fixture-key 1 0
run_case 'explicit write mode requires credentials' staging true true '' 1 0
run_case 'production refuses explicit write mode' production true true fixture-key 1 0
run_case 'explicit permitted local write smoke remains available' local true true fixture-key 0 1

# A valid-looking body cannot turn a failed HTTP expectation into a passed smoke.
sed -n '/^test_health_endpoint()/,/^main()/p' "$ROOT/deploy/testing/smoke-tests.sh" | sed '$d' > "$TEST_DIR/read-functions.sh"
# shellcheck source=/dev/null
source "$TEST_DIR/read-functions.sh"
http_get() { printf '{"items":[]}\n'; return 1; }
run_case 'failed health blocks an explicit write probe' local true true fixture-key 1 0
MAX_RESPONSE_TIME_MS=1000000
for probe in test_events_api test_places_api test_organizations_api test_health_endpoint test_version_endpoint test_database_connectivity test_migration_status test_http_endpoint_check test_response_time test_admin_ui test_openapi_schema test_dev_portal_login test_dev_portal_accept_invitation; do
    TESTS_RUN=0 TESTS_PASSED=0 TESTS_FAILED=0
    status=0
    "$probe" || status=$?
    if [[ "$status" != 1 || "$TESTS_FAILED" != 1 ]]; then
        echo "FAIL: $probe masked a failed HTTP expectation"
        exit 1
    fi
    echo "PASS: $probe preserves HTTP failure"
done

# Cover the real HTTP helper too, including status-bearing bodies and transport errors.
sed -n '/^http_get()/,/^test_health_endpoint()/p' "$ROOT/deploy/testing/smoke-tests.sh" | sed '$d' > "$TEST_DIR/http-function.sh"
# shellcheck source=/dev/null
source "$TEST_DIR/http-function.sh"
RETRY_COUNT=1 RETRY_DELAY=0
curl() {
    printf '{"items":[]}\n%s\n' "$HTTP_STATUS"
    [[ "$HTTP_STATUS" != 000 ]]
}
for HTTP_STATUS in 401 403 500 000 200; do
    TESTS_RUN=0 TESTS_PASSED=0 TESTS_FAILED=0
    status=0
    test_events_api || status=$?
    expected=1
    [[ "$HTTP_STATUS" != 200 ]] || expected=0
    if [[ "$status" != "$expected" ]]; then
        echo "FAIL: event smoke status $HTTP_STATUS returned $status"
        exit 1
    fi
    echo "PASS: event smoke enforces status $HTTP_STATUS"
done
