#!/usr/bin/env bash
# Generate and ingest review-event regression fixtures.
#
# Usage:
#   scripts/review-regression-test.sh [local|staging]    # generate + ingest
#   scripts/review-regression-test.sh --generate-only     # generate only
#   scripts/review-regression-test.sh --clean [local|staging]  # delete RS-XX events
#
# Prerequisites:
#   - Built server binary (make build)
#   - For local: server running on localhost:8080, PERF_AGENT_API_KEY in .env
#   - For staging: .deploy.conf.staging with NODE_DOMAIN and PERF_AGENT_API_KEY

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
FIXTURE_FILE="${PROJECT_ROOT}/review-fixtures.json"

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
GENERATE_ONLY=false
CLEAN_MODE=false
ENVIRONMENT="local"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --generate-only)
            GENERATE_ONLY=true
            shift
            ;;
        --clean)
            CLEAN_MODE=true
            shift
            if [[ $# -gt 0 && "$1" != --* ]]; then
                ENVIRONMENT="$1"
                shift
            fi
            ;;
        local|staging)
            ENVIRONMENT="$1"
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [local|staging] [--generate-only] [--clean [local|staging]]"
            echo ""
            echo "Options:"
            echo "  local              Target localhost:8080 (default)"
            echo "  staging            Target staging environment"
            echo "  --generate-only    Generate fixture file but do not ingest"
            echo "  --clean            Delete all RS-XX fixture events from the database"
            echo "  -h, --help         Show this help"
            exit 0
            ;;
        *)
            echo "Error: Unknown argument '$1'"
            echo "Run '$0 --help' for usage."
            exit 1
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Resolve API URL and key
# ---------------------------------------------------------------------------
resolve_api() {
    if [ "$ENVIRONMENT" = "staging" ]; then
        if [ -f "$PROJECT_ROOT/.deploy.conf.staging" ]; then
            # shellcheck source=/dev/null
            source "$PROJECT_ROOT/.deploy.conf.staging"
            API_URL="https://${NODE_DOMAIN}/api/v1"
            API_KEY="${PERF_AGENT_API_KEY:-}"
        else
            echo "Error: .deploy.conf.staging not found"
            exit 1
        fi
    else
        API_URL="http://localhost:8080/api/v1"
        if [ -f "$PROJECT_ROOT/.env" ]; then
            API_KEY=$(grep "^PERF_AGENT_API_KEY=" "$PROJECT_ROOT/.env" | cut -d= -f2- || true)
        fi
        API_KEY="${API_KEY:-}"
    fi

    if [ -z "$API_KEY" ]; then
        echo "Error: API key not found."
        echo "Set PERF_AGENT_API_KEY in .env (local) or .deploy.conf.staging (staging)."
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Clean mode: delete RS-XX events
# ---------------------------------------------------------------------------
if [ "$CLEAN_MODE" = true ]; then
    echo "========================================="
    echo "Review Fixture Cleanup"
    echo "========================================="
    echo "Environment: $ENVIRONMENT"

    if [ "$ENVIRONMENT" = "local" ]; then
        DB_URL=$(grep "^DATABASE_URL=" "$PROJECT_ROOT/.env" 2>/dev/null | cut -d= -f2- || true)
        if [ -z "$DB_URL" ]; then
            echo "Error: DATABASE_URL not found in .env"
            exit 1
        fi
        echo "Deleting RS-XX fixture events from local database..."
        psql "$DB_URL" <<'SQL'
BEGIN;
DELETE FROM event_review_queue WHERE event_id IN (SELECT id FROM events WHERE name LIKE 'RS-%');
DELETE FROM event_tombstones WHERE event_id IN (SELECT id FROM events WHERE name LIKE 'RS-%');
DELETE FROM event_occurrences WHERE event_id IN (SELECT id FROM events WHERE name LIKE 'RS-%');
-- not_duplicates may not exist on all branches; delete only if present.
DO $$ BEGIN
  DELETE FROM not_duplicates WHERE event_id_a IN (SELECT id FROM events WHERE name LIKE 'RS-%') OR event_id_b IN (SELECT id FROM events WHERE name LIKE 'RS-%');
EXCEPTION WHEN undefined_table THEN NULL;
END $$;
DELETE FROM events WHERE name LIKE 'RS-%';
COMMIT;
SQL
        echo "Done. All RS-XX fixture events removed."
    else
        echo "Error: --clean only works with local environment (requires direct DB access)."
        echo "For staging, use the admin API or SSH tunnel."
        exit 1
    fi
    exit 0
fi

# ---------------------------------------------------------------------------
# Generate fixtures
# ---------------------------------------------------------------------------
echo "========================================="
echo "Review Regression Fixtures"
echo "========================================="
echo "Environment: $ENVIRONMENT"
echo "Fixture file: $FIXTURE_FILE"
echo ""

echo "Generating review event fixtures..."
go run "$PROJECT_ROOT/cmd/server/main.go" generate "$FIXTURE_FILE" --review-events

if [ "$GENERATE_ONLY" = true ]; then
    echo ""
    echo "Generated fixtures to $FIXTURE_FILE (--generate-only, skipping ingest)."
    echo ""
    echo "Next steps:"
    echo "  # Ingest all events:"
    echo "  go run cmd/server/main.go ingest $FIXTURE_FILE"
    exit 0
fi

# ---------------------------------------------------------------------------
# Ingest fixtures
# ---------------------------------------------------------------------------
resolve_api

echo ""
echo "Ingesting fixtures into $ENVIRONMENT ($API_URL)..."
echo ""

# Extract the flat events array and ingest each one individually.
# This ensures each event gets its own ingest request (matching real-world flow).
EVENT_COUNT=$(jq '.events | length' "$FIXTURE_FILE")
echo "Total events to ingest: $EVENT_COUNT"
echo ""

SUCCESS=0
WARNINGS=0
ERRORS=0

for i in $(seq 0 $((EVENT_COUNT - 1))); do
    EVENT_NAME=$(jq -r ".events[$i].name" "$FIXTURE_FILE")
    EVENT_JSON=$(jq -c ".events[$i]" "$FIXTURE_FILE")

    printf "[%2d/%d] %-55s " "$((i + 1))" "$EVENT_COUNT" "$EVENT_NAME"

    RESPONSE=$(curl -s -w "\n%{http_code}" \
        -X POST "${API_URL}/events" \
        -H "Content-Type: application/json" \
        -H "Accept: application/ld+json" \
        -H "X-API-Key: ${API_KEY}" \
        -d "$EVENT_JSON" 2>&1)

    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    case "$HTTP_CODE" in
        201)
            echo "201 Created"
            SUCCESS=$((SUCCESS + 1))
            ;;
        202)
            WARN_CODES=$(echo "$BODY" | jq -r '.warnings[]?.code // empty' 2>/dev/null | tr '\n' ', ' | sed 's/,$//')
            echo "202 Accepted [${WARN_CODES:-review}]"
            WARNINGS=$((WARNINGS + 1))
            ;;
        409)
            echo "409 Conflict (already exists)"
            SUCCESS=$((SUCCESS + 1))
            ;;
        *)
            DETAIL=$(echo "$BODY" | jq -r '.detail // .message // empty' 2>/dev/null)
            echo "${HTTP_CODE} ERROR: ${DETAIL:-unknown}"
            ERRORS=$((ERRORS + 1))
            ;;
    esac
done

echo ""
echo "========================================="
echo "Ingest Summary"
echo "========================================="
echo "Published:    $SUCCESS"
echo "Review Queue: $WARNINGS"
echo "Errors:       $ERRORS"
echo "Total:        $EVENT_COUNT"
echo ""

if [ "$ERRORS" -gt 0 ]; then
    echo "WARNING: Some events failed to ingest. Check the errors above."
    exit 1
fi

echo "Next steps:"
echo "  1. Open admin UI: ${API_URL%/api/v1}/admin/review-queue"
echo "  2. Follow docs/testing/review-regression-test-plan.md"
echo ""
echo "To clean up after testing:"
echo "  $0 --clean $ENVIRONMENT"
