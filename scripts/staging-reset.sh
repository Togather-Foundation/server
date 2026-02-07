#!/bin/bash
# staging-reset.sh - Reset staging database event data (preserves users)
#
# This script wipes all event-related data from staging database but preserves
# users, invitations, and authentication data. Useful for clean testing.
#
# Usage:
#   ./scripts/staging-reset.sh
#
# CAUTION: This will delete ALL events, places, organizations, sources, and related data!

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

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
echo

# Confirm with user
read -p "This will DELETE all event data from staging. Continue? (yes/NO): " confirm
if [ "$confirm" != "yes" ]; then
    echo "Aborted."
    exit 0
fi

echo -e "${YELLOW}Connecting to staging server...${NC}"

# Run reset SQL on staging
ssh "$SSH_HOST" << 'ENDSSH'
set -e

# Source env for DATABASE_URL
cd /opt/togather
source .env

echo "Checking current data counts..."
psql "$DATABASE_URL" -c "
  SELECT 
    (SELECT COUNT(*) FROM events) as events,
    (SELECT COUNT(*) FROM places) as places,
    (SELECT COUNT(*) FROM organizations) as organizations,
    (SELECT COUNT(*) FROM sources) as sources,
    (SELECT COUNT(*) FROM users) as users;
"

echo
echo "Truncating event-related tables..."

psql "$DATABASE_URL" << 'EOSQL'
BEGIN;

-- Truncate all event-related tables
-- CASCADE will handle foreign key dependencies
TRUNCATE TABLE 
  events,
  event_sources,
  places,
  organizations,
  sources
RESTART IDENTITY CASCADE;

-- Note: This cascades to:
-- - event_series
-- - event_occurrences  
-- - field_provenance
-- - event_changes
-- - api_keys (event-specific)
-- - idempotency_keys

COMMIT;

SELECT 'Event data wiped successfully!' as status;
EOSQL

echo
echo "Data counts after reset:"
psql "$DATABASE_URL" -c "
  SELECT 
    (SELECT COUNT(*) FROM events) as events,
    (SELECT COUNT(*) FROM places) as places,
    (SELECT COUNT(*) FROM organizations) as organizations,
    (SELECT COUNT(*) FROM sources) as sources,
    (SELECT COUNT(*) FROM users) as users;
"

ENDSSH

echo
echo -e "${GREEN}✓ Staging database reset complete${NC}"
echo
echo "Users and invitations preserved."
echo "Events, places, organizations, and sources deleted."
echo
echo "Next steps:"
echo "  1. Deploy your branch: ./deploy/scripts/deploy.sh staging --version HEAD"
echo "  2. Run ingestion: ./scripts/ingest-toronto-events.sh staging 50 300"
