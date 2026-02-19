#!/bin/bash
# staging-reset.sh - Reset staging database event data (preserves users, API keys, sources)
#
# This script wipes all event-related data from staging database but preserves:
# - Users and invitations
# - API keys (access credentials)
# - Sources (data source configurations like API endpoints, trust levels)
#
# Usage:
#   ./scripts/staging-reset.sh              # Preserve sources (default)
#   ./scripts/staging-reset.sh --wipe-all   # Also wipe sources (fully clean)
#   ./scripts/staging-reset.sh --yes        # Skip confirmation prompt
#
# CAUTION: This will delete ALL events, places, organizations, and related data!

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Parse arguments
WIPE_SOURCES=false
AUTO_CONFIRM=false
for arg in "$@"; do
    case "$arg" in
        --wipe-all)
            WIPE_SOURCES=true
            ;;
        --yes|-y)
            AUTO_CONFIRM=true
            ;;
        *)
            echo "Unknown argument: $arg"
            echo "Usage: $0 [--wipe-all] [--yes]"
            exit 1
            ;;
    esac
done

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Load deployment config
if [ ! -f "$PROJECT_ROOT/.deploy.conf.staging" ]; then
    echo -e "${RED}Error: .deploy.conf.staging not found${NC}"
    exit 1
fi

source "$PROJECT_ROOT/.deploy.conf.staging"

echo -e "${YELLOW}=== Staging Database Reset ===${NC}"
echo "Target: $SSH_HOST ($NODE_DOMAIN)"
if [ "$WIPE_SOURCES" = true ]; then
    echo "Mode: Full wipe (including sources)"
else
    echo "Mode: Preserve sources (default)"
fi
echo

# Confirm with user (unless --yes flag provided)
if [ "$AUTO_CONFIRM" = false ]; then
    read -p "This will DELETE all event data from staging. Continue? (yes/NO): " confirm
    if [ "$confirm" != "yes" ]; then
        echo "Aborted."
        exit 0
    fi
fi

echo -e "${YELLOW}Connecting to staging server...${NC}"

# Run reset SQL on staging
ssh "$SSH_HOST" << ENDSSH
set -e

# Source env for DATABASE_URL
cd /opt/togather
source .env

# Pass the WIPE_SOURCES flag to the remote script
WIPE_SOURCES=$WIPE_SOURCES

echo "Checking current data counts..."
psql "\$DATABASE_URL" -c "
  SELECT 
    (SELECT COUNT(*) FROM events) as events,
    (SELECT COUNT(*) FROM places) as places,
    (SELECT COUNT(*) FROM organizations) as organizations,
    (SELECT COUNT(*) FROM sources) as sources,
    (SELECT COUNT(*) FROM api_keys) as api_keys,
    (SELECT COUNT(*) FROM users) as users;
"

echo
echo "Truncating event-related tables..."

if [ "\$WIPE_SOURCES" = "true" ]; then
    echo "Mode: Wiping sources and API keys"
    psql "\$DATABASE_URL" << 'EOSQL'
BEGIN;

-- Full wipe mode: Delete everything except users
TRUNCATE TABLE 
  event_occurrences,
  event_series,
  field_provenance,
  event_changes,
  event_sources,
  idempotency_keys,
  entity_identifiers,
  reconciliation_cache,
  events,
  places,
  organizations,
  sources,
  api_keys
RESTART IDENTITY CASCADE;

-- Only users and invitations are preserved

COMMIT;

SELECT 'Full wipe complete! Only users and invitations preserved.' as status;
EOSQL
else
    echo "Mode: Preserving sources and API keys"
    psql "\$DATABASE_URL" << 'EOSQL'
BEGIN;

-- Preserve sources and API keys - only wipe event data
-- Step 1: Break the link between api_keys and sources (api_keys.source_id can reference sources)
UPDATE api_keys SET source_id = NULL WHERE source_id IS NOT NULL;

-- Step 2: Truncate event data tables in dependency order
TRUNCATE TABLE 
  event_occurrences,
  event_series,
  field_provenance,
  event_changes,
  event_sources,     -- Links between events and sources (delete this)
  idempotency_keys,
  event_review_queue,  -- Review queue for events
  entity_identifiers,  -- Knowledge graph entity identifiers
  reconciliation_cache, -- Reconciliation API cache
  events,
  places,
  organizations
RESTART IDENTITY;

-- Preserved:
-- ✓ sources (data source configurations - API endpoints, trust levels, etc.)
-- ✓ api_keys (access credentials)
-- ✓ users (user accounts)
-- ✓ user_invitations (invitations)

COMMIT;

SELECT 'Event data wiped! Users, API keys, and sources preserved.' as status;
EOSQL
fi

echo
echo "Data counts after reset:"
psql "\$DATABASE_URL" -c "
  SELECT 
    (SELECT COUNT(*) FROM events) as events,
    (SELECT COUNT(*) FROM places) as places,
    (SELECT COUNT(*) FROM organizations) as organizations,
    (SELECT COUNT(*) FROM sources) as sources,
    (SELECT COUNT(*) FROM api_keys) as api_keys,
    (SELECT COUNT(*) FROM users) as users;
"

ENDSSH

echo
echo -e "${GREEN}✓ Staging database reset complete${NC}"
echo
if [ "$WIPE_SOURCES" = true ]; then
    echo "Full wipe mode:"
    echo "  ✓ Users and invitations preserved"
    echo "  ✗ API keys deleted"
    echo "  ✗ Sources deleted"
else
    echo "Partial wipe mode:"
    echo "  ✓ Users and invitations preserved"
    echo "  ✓ API keys preserved"
    echo "  ✓ Sources preserved"
fi
echo "  ✗ Events, places, organizations deleted"
echo
echo "Next steps:"
echo "  1. Deploy your branch: ./deploy/scripts/deploy.sh staging --version HEAD"
echo "  2. Run ingestion: ./scripts/ingest-toronto-events.sh staging 50 300"
