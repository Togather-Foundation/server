#!/usr/bin/env bash
# Performance/Load Testing Script for Togather Server
# Generates realistic traffic patterns using test fixtures

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Default values
TARGET_URL="${TARGET_URL:-http://localhost:8080}"
PROFILE="${PROFILE:-light}"
DURATION="1m"
RPS=5
READ_RATIO=0.8
NO_RAMP=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Performance/load testing script for Togather server.

OPTIONS:
    -u, --url URL          Target server URL (default: http://localhost:8080)
    -p, --profile PROFILE  Load profile: light, medium, heavy, stress, burst, peak (default: light)
    -r, --rps RPS          Custom requests per second
    -d, --duration TIME    Custom test duration (e.g., 30s, 2m, 1h)
    -R, --read-ratio NUM   Read/write ratio 0.0-1.0 (default: 0.8)
    -s, --slot SLOT        Target specific slot: blue, green, or lb (load balanced)
    --no-ramp              Disable ramp-up/ramp-down (instant start/stop)
    -h, --help             Show this help message

LOAD PROFILES:
    light   - 5 req/s for 1 minute (good for smoke testing)
    medium  - 20 req/s for 2 minutes (typical load)
    heavy   - 50 req/s for 5 minutes (peak hours)
    stress  - 100 req/s for 10 minutes (stress testing)
    burst   - Burst pattern: 10 → 100 → 10 req/s
    peak    - Gradual ramp-up to 40 req/s, then ramp-down

EXAMPLES:
    # Run light load test against default URL
    $0

    # Run heavy load test against blue slot
    $0 --profile heavy --slot blue

    # Custom test: 30 req/s for 5 minutes
    $0 --rps 30 --duration 5m

    # Test load-balanced endpoint with stress profile
    $0 --profile stress --url http://localhost

    # Custom read-heavy test (95% reads)
    $0 --rps 50 --duration 2m --read-ratio 0.95

SLOT TARGETING:
    By default, tests target port 8080. You can target specific deployment slots:
    - blue: Port 8081
    - green: Port 8082
    - lb: Port 80 (load-balanced through Caddy)

EOF
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            ;;
        -u|--url)
            TARGET_URL="$2"
            shift 2
            ;;
        -p|--profile)
            PROFILE="$2"
            shift 2
            ;;
        -r|--rps)
            RPS="$2"
            shift 2
            ;;
        -d|--duration)
            DURATION="$2"
            shift 2
            ;;
        -R|--read-ratio)
            READ_RATIO="$2"
            shift 2
            ;;
        -s|--slot)
            SLOT="$2"
            case $SLOT in
                blue)
                    TARGET_URL="http://localhost:8081"
                    ;;
                green)
                    TARGET_URL="http://localhost:8082"
                    ;;
                lb)
                    TARGET_URL="http://localhost"
                    ;;
                *)
                    echo -e "${RED}Error: Invalid slot '$SLOT'. Must be: blue, green, or lb${NC}"
                    exit 1
                    ;;
            esac
            shift 2
            ;;
        --no-ramp)
            NO_RAMP=true
            shift
            ;;
        *)
            echo -e "${RED}Error: Unknown option $1${NC}"
            usage
            ;;
    esac
done

echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║        Togather Server - Performance Load Test                 ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check if server is reachable
echo -e "${YELLOW}Checking server connectivity...${NC}"
if ! curl -s -f -m 5 "$TARGET_URL/health" > /dev/null 2>&1; then
    echo -e "${RED}✗ Error: Cannot reach server at $TARGET_URL${NC}"
    echo -e "${YELLOW}  Make sure the server is running:${NC}"
    echo -e "${YELLOW}    cd deploy/docker && docker compose up -d${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Server is reachable at $TARGET_URL${NC}"
echo ""

# Build the Go load test binary if needed
LOADTEST_BIN="$PROJECT_ROOT/bin/loadtest"
if [[ ! -f "$LOADTEST_BIN" ]] || [[ "$PROJECT_ROOT/tests/performance/load_test.go" -nt "$LOADTEST_BIN" ]]; then
    echo -e "${YELLOW}Building load test binary...${NC}"
    mkdir -p "$PROJECT_ROOT/bin"
    
    cd "$PROJECT_ROOT"
    if ! go build -o "$LOADTEST_BIN" ./cmd/loadtest; then
        echo -e "${RED}✗ Failed to build load test binary${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Load test binary built${NC}"
    echo ""
fi

# Run the load test
echo -e "${GREEN}Starting load test...${NC}"
echo -e "${BLUE}─────────────────────────────────────────────────────────────────${NC}"
echo ""

cd "$PROJECT_ROOT"

# Build command arguments
ARGS=(--url "$TARGET_URL")

if [[ -n "${PROFILE:-}" ]]; then
    ARGS+=(--profile "$PROFILE")
fi

if [[ -n "${RPS:-}" ]] && [[ "$RPS" != "5" ]]; then
    ARGS+=(--rps "$RPS")
fi

if [[ -n "${DURATION:-}" ]] && [[ "$DURATION" != "1m" ]]; then
    ARGS+=(--duration "$DURATION")
fi

if [[ -n "${READ_RATIO:-}" ]] && [[ "$READ_RATIO" != "0.8" ]]; then
    ARGS+=(--read-ratio "$READ_RATIO")
fi

if [[ "$NO_RAMP" == "true" ]]; then
    ARGS+=(--no-ramp)
fi

# Execute load test
"$LOADTEST_BIN" "${ARGS[@]}"

echo ""
echo -e "${GREEN}✓ Load test completed${NC}"
echo ""
echo -e "${BLUE}TIP: View metrics in Grafana:${NC}"
echo -e "${BLUE}     http://localhost:3000/d/togather-overview/togather-server-overview${NC}"
