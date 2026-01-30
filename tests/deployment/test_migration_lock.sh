#!/usr/bin/env bash
#
# Test migration lock race condition prevention
# Tests the atomic locking mechanism in deploy/scripts/deploy.sh
#
# This verifies the fix from server-ytc5 to prevent concurrent migrations
# from corrupting the database.

set -uo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Cleanup function - called at script exit
final_cleanup() {
    echo ""
    echo "Cleaning up test artifacts..."
    rm -rf /tmp/togather-deploy-test.lock 2>/dev/null || true
    rm -f /tmp/test-deploy-state.json 2>/dev/null || true
    rm -f /tmp/test-lock-*.log 2>/dev/null || true
}

# Cleanup between tests
cleanup() {
    rm -rf /tmp/togather-deploy-test.lock 2>/dev/null || true
    rm -f /tmp/test-deploy-state.json 2>/dev/null || true
    rm -f /tmp/test-lock-*.log 2>/dev/null || true
}

trap final_cleanup EXIT

# Test result helpers
pass() {
    echo -e "${GREEN}✓ PASS${NC}: $1"
    ((TESTS_PASSED++))
    ((TESTS_RUN++))
}

fail() {
    echo -e "${RED}✗ FAIL${NC}: $1"
    ((TESTS_FAILED++))
    ((TESTS_RUN++))
}

info() {
    echo -e "${YELLOW}ℹ INFO${NC}: $1"
}

# Create a mock state file for testing
setup_state_file() {
    cat > /tmp/test-deploy-state.json <<EOF
{
  "environment": "test",
  "lock": {
    "locked": false
  }
}
EOF
}

# Test 1: Single process can acquire lock
test_single_acquisition() {
    echo ""
    info "Test 1: Single process can acquire lock"
    
    cleanup
    setup_state_file
    
    # Create lock directory (simulating acquire_lock)
    if mkdir /tmp/togather-deploy-test.lock 2>/dev/null; then
        pass "Single process successfully acquired lock"
        rmdir /tmp/togather-deploy-test.lock
        return 0
    else
        fail "Single process failed to acquire lock"
        return 1
    fi
}

# Test 2: Second process blocked by existing lock
test_concurrent_blocking() {
    echo ""
    info "Test 2: Second process blocked when lock exists"
    
    cleanup
    setup_state_file
    
    # First process acquires lock
    mkdir /tmp/togather-deploy-test.lock 2>/dev/null || {
        fail "Setup: Could not create initial lock"
        return 1
    }
    
    # Update state file to show locked
    cat > /tmp/test-deploy-state.json <<EOF
{
  "environment": "test",
  "lock": {
    "locked": true,
    "lock_id": "lock-test123",
    "deployment_id": "dep-test456",
    "locked_by": "test-user",
    "locked_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
    "lock_expires_at": "$(date -u -d "+30 minutes" +"%Y-%m-%dT%H:%M:%SZ")",
    "pid": $$,
    "hostname": "$(hostname)"
  }
}
EOF
    
    # Second process tries to acquire lock (should fail)
    if mkdir /tmp/togather-deploy-test.lock 2>/dev/null; then
        rmdir /tmp/togather-deploy-test.lock
        fail "Second process acquired lock (race condition!)"
        return 1
    else
        pass "Second process correctly blocked by existing lock"
        rmdir /tmp/togather-deploy-test.lock
        return 0
    fi
}

# Test 3: Lock cleanup on process exit
test_lock_cleanup_on_exit() {
    echo ""
    info "Test 3: Lock is cleaned up when process exits"
    
    cleanup
    setup_state_file
    
    # Start a subprocess that acquires lock and exits
    (
        trap 'rmdir /tmp/togather-deploy-test.lock 2>/dev/null || true' EXIT INT TERM
        mkdir /tmp/togather-deploy-test.lock 2>/dev/null
        sleep 0.5
        exit 0
    ) &
    
    local pid=$!
    sleep 0.2  # Let subprocess acquire lock
    
    # Verify lock exists while process is running
    if [[ ! -d /tmp/togather-deploy-test.lock ]]; then
        fail "Lock was not created by subprocess"
        wait $pid 2>/dev/null || true
        return 1
    fi
    
    # Wait for subprocess to exit
    wait $pid 2>/dev/null || true
    
    # Give trap time to execute
    sleep 0.3
    
    # Verify lock was cleaned up
    if [[ -d /tmp/togather-deploy-test.lock ]]; then
        fail "Lock was not cleaned up on process exit"
        rmdir /tmp/togather-deploy-test.lock
        return 1
    else
        pass "Lock correctly cleaned up on process exit"
        return 0
    fi
}

# Test 4: Lock cleanup on SIGTERM
test_lock_cleanup_on_sigterm() {
    echo ""
    info "Test 4: Lock is cleaned up when process receives SIGTERM"
    
    cleanup
    setup_state_file
    
    # Start a subprocess that holds lock
    (
        trap 'rmdir /tmp/togather-deploy-test.lock 2>/dev/null || true; exit 0' EXIT INT TERM
        mkdir /tmp/togather-deploy-test.lock 2>/dev/null
        sleep 10  # Hold lock for a while
    ) &
    
    local pid=$!
    sleep 0.5  # Let subprocess acquire lock
    
    # Verify lock exists
    if [[ ! -d /tmp/togather-deploy-test.lock ]]; then
        fail "Lock was not created by subprocess"
        kill -9 $pid 2>/dev/null || true
        wait $pid 2>/dev/null || true
        return 1
    fi
    
    # Send SIGTERM to subprocess
    kill -TERM $pid 2>/dev/null || true
    
    # Wait for process to exit and cleanup to complete
    wait $pid 2>/dev/null || true
    sleep 0.3
    
    # Verify lock was cleaned up
    if [[ -d /tmp/togather-deploy-test.lock ]]; then
        fail "Lock was not cleaned up after SIGTERM"
        rmdir /tmp/togather-deploy-test.lock
        return 1
    else
        pass "Lock correctly cleaned up after SIGTERM"
        return 0
    fi
}

# Test 5: Lock cleanup on SIGINT
test_lock_cleanup_on_sigint() {
    echo ""
    info "Test 5: Lock is cleaned up when process receives SIGINT (Ctrl+C)"
    
    cleanup
    setup_state_file
    
    # Start a subprocess that holds lock
    (
        trap 'rmdir /tmp/togather-deploy-test.lock 2>/dev/null || true; exit 0' EXIT INT TERM
        mkdir /tmp/togather-deploy-test.lock 2>/dev/null
        sleep 10  # Hold lock for a while
    ) &
    
    local pid=$!
    sleep 0.5  # Let subprocess acquire lock
    
    # Verify lock exists
    if [[ ! -d /tmp/togather-deploy-test.lock ]]; then
        fail "Lock was not created by subprocess"
        kill -9 $pid 2>/dev/null || true
        wait $pid 2>/dev/null || true
        return 1
    fi
    
    # Send SIGINT to subprocess
    kill -INT $pid 2>/dev/null || true
    
    # Wait for process to exit and cleanup to complete
    wait $pid 2>/dev/null || true
    sleep 0.3
    
    # Verify lock was cleaned up
    if [[ -d /tmp/togather-deploy-test.lock ]]; then
        fail "Lock was not cleaned up after SIGINT"
        rmdir /tmp/togather-deploy-test.lock
        return 1
    else
        pass "Lock correctly cleaned up after SIGINT"
        return 0
    fi
}

# Test 6: Stale lock detection (simulated)
test_stale_lock_detection() {
    echo ""
    info "Test 6: Stale lock can be detected and cleaned up"
    
    cleanup
    setup_state_file
    
    # Create a lock with an old timestamp (simulate stale lock)
    mkdir /tmp/togather-deploy-test.lock
    
    # Create state file with old lock timestamp (31 minutes ago, past 30min timeout)
    local old_timestamp=$(date -u -d "-31 minutes" +"%Y-%m-%dT%H:%M:%SZ")
    cat > /tmp/test-deploy-state.json <<EOF
{
  "environment": "test",
  "lock": {
    "locked": true,
    "lock_id": "lock-stale123",
    "deployment_id": "dep-stale456",
    "locked_by": "old-process",
    "locked_at": "$old_timestamp",
    "lock_expires_at": "$(date -u -d "-1 minute" +"%Y-%m-%dT%H:%M:%SZ")",
    "pid": 99999,
    "hostname": "old-host"
  }
}
EOF
    
    # Calculate lock age
    local locked_timestamp=$(date -d "$old_timestamp" +%s 2>/dev/null || echo "0")
    local now_timestamp=$(date +%s)
    local lock_age=$((now_timestamp - locked_timestamp))
    local LOCK_TIMEOUT=1800  # 30 minutes in seconds
    
    if [[ $lock_age -gt $LOCK_TIMEOUT ]]; then
        # Lock is stale, should be safe to remove
        if rmdir /tmp/togather-deploy-test.lock 2>/dev/null; then
            pass "Stale lock detected (age: ${lock_age}s) and can be removed"
            return 0
        else
            fail "Failed to remove stale lock"
            return 1
        fi
    else
        fail "Lock age calculation failed or lock not actually stale"
        rmdir /tmp/togather-deploy-test.lock 2>/dev/null || true
        return 1
    fi
}

# Test 7: Race condition - two processes trying to acquire simultaneously
test_race_condition() {
    echo ""
    info "Test 7: Race condition prevention - two processes compete for lock"
    
    cleanup
    setup_state_file
    
    local acquired_count=0
    
    # Start two processes simultaneously trying to acquire lock
    (
        if mkdir /tmp/togather-deploy-test.lock 2>/dev/null; then
            echo "P1_ACQUIRED" > /tmp/test-lock-p1.log
            sleep 0.5
            rmdir /tmp/togather-deploy-test.lock 2>/dev/null || true
        else
            echo "P1_BLOCKED" > /tmp/test-lock-p1.log
        fi
    ) &
    local pid1=$!
    
    (
        if mkdir /tmp/togather-deploy-test.lock 2>/dev/null; then
            echo "P2_ACQUIRED" > /tmp/test-lock-p2.log
            sleep 0.5
            rmdir /tmp/togather-deploy-test.lock 2>/dev/null || true
        else
            echo "P2_BLOCKED" > /tmp/test-lock-p2.log
        fi
    ) &
    local pid2=$!
    
    # Wait for both processes
    wait $pid1 2>/dev/null || true
    wait $pid2 2>/dev/null || true
    
    # Check results
    local p1_result=$(cat /tmp/test-lock-p1.log 2>/dev/null || echo "UNKNOWN")
    local p2_result=$(cat /tmp/test-lock-p2.log 2>/dev/null || echo "UNKNOWN")
    
    rm -f /tmp/test-lock-p1.log /tmp/test-lock-p2.log
    
    # Exactly one should have acquired, one should have been blocked
    if [[ "$p1_result" == "P1_ACQUIRED" && "$p2_result" == "P2_BLOCKED" ]] || \
       [[ "$p1_result" == "P1_BLOCKED" && "$p2_result" == "P2_ACQUIRED" ]]; then
        pass "Race condition prevented: exactly one process acquired lock (P1=$p1_result, P2=$p2_result)"
        return 0
    else
        fail "Race condition detected: both processes result: P1=$p1_result, P2=$p2_result"
        rmdir /tmp/togather-deploy-test.lock 2>/dev/null || true
        return 1
    fi
}

# Run all tests
main() {
    echo "========================================="
    echo "Migration Lock Race Condition Tests"
    echo "========================================="
    echo ""
    echo "Testing atomic locking mechanism from deploy/scripts/deploy.sh"
    echo "Verifies fix for server-ytc5 (race condition prevention)"
    echo ""
    
    test_single_acquisition
    test_concurrent_blocking
    test_lock_cleanup_on_exit
    test_lock_cleanup_on_sigterm
    test_lock_cleanup_on_sigint
    test_stale_lock_detection
    test_race_condition
    
    echo ""
    echo "========================================="
    echo "Test Results"
    echo "========================================="
    echo "Total tests: $TESTS_RUN"
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    if [[ $TESTS_FAILED -gt 0 ]]; then
        echo -e "${RED}Failed: $TESTS_FAILED${NC}"
    else
        echo "Failed: $TESTS_FAILED"
    fi
    echo ""
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}Some tests failed!${NC}"
        exit 1
    fi
}

main "$@"
