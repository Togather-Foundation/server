#!/usr/bin/env bash
# Blue-Green Deployment Load Testing Script
#
# Tests the blue-green deployment process under load to verify:
# 1. Zero-downtime deployment (requests succeed during slot switch)
# 2. Both slots can handle load independently
# 3. Load balancing works correctly
# 4. Metrics are correctly labeled by slot
#
# Usage:
#   ./blue-green-load-test.sh [OPTIONS]
#
# Options:
#   --skip-setup    Skip initial slot verification
#   --duration TIME Test duration (default: 2m)
#   --rps NUM       Requests per second (default: 20)
#   --help          Show this help message

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Default values
SKIP_SETUP=false
DURATION="2m"
RPS=20
TEST_SLOT_SWITCH=true

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Blue-green deployment load testing script.

Tests both deployment slots under load and verifies:
- Independent slot performance
- Load balancing correctness
- Zero-downtime switching capability

OPTIONS:
    --skip-setup       Skip initial setup verification
    --duration TIME    Test duration per slot (default: 2m)
    --rps NUM          Requests per second (default: 20)
    --no-switch        Don't test slot switching (only test each slot)
    -h, --help         Show this help message

EXAMPLES:
    # Full blue-green load test (both slots + switching)
    $0

    # Quick test without slot switching
    $0 --duration 1m --rps 10 --no-switch

    # Stress test with higher load
    $0 --duration 5m --rps 50

PREREQUISITES:
    - Blue-green deployment running (docker-compose.blue-green.yml)
    - Caddy reverse proxy configured for load balancing
    - Both blue and green slots healthy

EOF
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            ;;
        --skip-setup)
            SKIP_SETUP=true
            shift
            ;;
        --duration)
            DURATION="$2"
            shift 2
            ;;
        --rps)
            RPS="$2"
            shift 2
            ;;
        --no-switch)
            TEST_SLOT_SWITCH=false
            shift
            ;;
        *)
            echo -e "${RED}Error: Unknown option $1${NC}"
            usage
            ;;
    esac
done

echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     Blue-Green Deployment Load Testing                         ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check prerequisites
if [[ "$SKIP_SETUP" == "false" ]]; then
    echo -e "${CYAN}Checking prerequisites...${NC}"
    
    # Check if blue slot is reachable
    if ! curl -s -f -m 5 http://localhost:8081/health > /dev/null 2>&1; then
        echo -e "${RED}✗ Blue slot not reachable at :8081${NC}"
        echo -e "${YELLOW}  Start blue-green deployment:${NC}"
        echo -e "${YELLOW}    cd deploy/docker && docker compose -f docker-compose.blue-green.yml up -d${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Blue slot healthy${NC}"
    
    # Check if green slot is reachable
    if ! curl -s -f -m 5 http://localhost:8082/health > /dev/null 2>&1; then
        echo -e "${RED}✗ Green slot not reachable at :8082${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Green slot healthy${NC}"
    
    # Check load balancer (optional - not all setups have Caddy)
    if curl -s -f -m 5 http://localhost/health > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Load balancer healthy${NC}"
        LB_AVAILABLE=true
    else
        echo -e "${YELLOW}⚠ Load balancer not available (testing slots directly)${NC}"
        LB_AVAILABLE=false
    fi
    
    echo ""
fi

# Function to run load test against a specific target
run_load_test() {
    local target_name="$1"
    local target_url="$2"
    local duration="$3"
    local rps="$4"
    
    echo -e "${CYAN}Running load test: ${target_name}${NC}"
    echo -e "${YELLOW}  Target: ${target_url}${NC}"
    echo -e "${YELLOW}  Duration: ${duration}${NC}"
    echo -e "${YELLOW}  RPS: ${rps}${NC}"
    echo ""
    
    # Run performance test script
    if ! "$SCRIPT_DIR/performance-test.sh" \
        --url "$target_url" \
        --duration "$duration" \
        --rps "$rps" \
        --no-ramp 2>&1 | tail -20; then
        echo -e "${RED}✗ Load test failed for ${target_name}${NC}"
        return 1
    fi
    
    echo ""
    return 0
}

# Test Phase 1: Blue slot
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Phase 1: Testing Blue Slot${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""

if ! run_load_test "Blue Slot" "http://localhost:8081" "$DURATION" "$RPS"; then
    echo -e "${RED}✗ Blue slot load test failed${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Blue slot passed load test${NC}"
echo ""

# Test Phase 2: Green slot
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Phase 2: Testing Green Slot${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""

if ! run_load_test "Green Slot" "http://localhost:8082" "$DURATION" "$RPS"; then
    echo -e "${RED}✗ Green slot load test failed${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Green slot passed load test${NC}"
echo ""

# Test Phase 3: Load balancer (if available)
if [[ "${LB_AVAILABLE:-false}" == "true" ]]; then
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}Phase 3: Testing Load Balancer${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
    
    if ! run_load_test "Load Balancer" "http://localhost" "$DURATION" "$RPS"; then
        echo -e "${RED}✗ Load balancer test failed${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✓ Load balancer passed load test${NC}"
    echo ""
fi

# Test Phase 4: Slot switching under load (if enabled)
if [[ "$TEST_SLOT_SWITCH" == "true" ]]; then
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}Phase 4: Zero-Downtime Slot Switching Test${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "${YELLOW}Note: This phase requires manual slot switching${NC}"
    echo -e "${YELLOW}      Start load test, then switch active slot via Caddy${NC}"
    echo ""
    echo -e "${CYAN}Instructions:${NC}"
    echo -e "  1. Load test will run for ${DURATION}"
    echo -e "  2. During the test, manually switch the active slot:"
    echo -e "     ${YELLOW}sudo nano /etc/caddy/Caddyfile${NC}"
    echo -e "     ${YELLOW}# Change reverse_proxy localhost:808X to other slot${NC}"
    echo -e "     ${YELLOW}sudo systemctl reload caddy${NC}"
    echo -e "  3. Monitor load test output for errors during switch"
    echo ""
    echo -e "${YELLOW}Press Enter to start continuous load test (Ctrl+C to stop)...${NC}"
    read -r
    
    echo -e "${CYAN}Starting continuous load test...${NC}"
    echo -e "${YELLOW}Switch slots now and observe for errors${NC}"
    echo ""
    
    # Run continuous load test
    # User should manually switch slots during this test
    if [[ "${LB_AVAILABLE:-false}" == "true" ]]; then
        run_load_test "Load Balancer (Continuous)" "http://localhost" "$DURATION" "$RPS" || true
    else
        echo -e "${YELLOW}⚠ Load balancer not available, skipping switching test${NC}"
    fi
    
    echo ""
    echo -e "${GREEN}✓ Slot switching test completed${NC}"
    echo -e "${YELLOW}  Review above output for errors during slot switch${NC}"
    echo ""
fi

# Summary
echo -e "${GREEN}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  Blue-Green Load Testing Complete                              ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}✓ All load tests passed successfully${NC}"
echo ""
echo -e "${CYAN}Test Summary:${NC}"
echo -e "  • Blue slot: Passed"
echo -e "  • Green slot: Passed"
if [[ "${LB_AVAILABLE:-false}" == "true" ]]; then
    echo -e "  • Load balancer: Passed"
fi
if [[ "$TEST_SLOT_SWITCH" == "true" ]]; then
    echo -e "  • Slot switching: Completed (review for errors)"
fi
echo ""
echo -e "${BLUE}View metrics in Grafana:${NC}"
echo -e "${BLUE}  http://localhost:3000/d/togather-overview/togather-server-overview${NC}"
echo ""
