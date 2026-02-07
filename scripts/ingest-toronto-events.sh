#!/usr/bin/env bash
# Ingest Toronto events from civictechto JSON-LD proxy
# Usage: ./scripts/ingest-toronto-events.sh [staging|local] [batch_size] [max_events]

set -euo pipefail

ENVIRONMENT="${1:-staging}"
BATCH_SIZE="${2:-50}"
MAX_EVENTS="${3:-0}"  # 0 = all events

# Determine API endpoint and authentication
if [ "$ENVIRONMENT" = "staging" ]; then
    if [ -f .deploy.conf.staging ]; then
        source .deploy.conf.staging
        API_URL="https://${NODE_DOMAIN}/api/v1"
        API_KEY="${PERF_AGENT_API_KEY}"
    else
        echo "Error: .deploy.conf.staging not found"
        exit 1
    fi
elif [ "$ENVIRONMENT" = "local" ]; then
    API_URL="http://localhost:8080/api/v1"
    # For local, read from .env if available
    if [ -f .env ]; then
        API_KEY=$(grep "^PERF_AGENT_API_KEY=" .env | cut -d= -f2-)
    fi
    # If not set, user can provide via environment variable
    API_KEY="${API_KEY:-}"
else
    echo "Error: Unknown environment '$ENVIRONMENT'. Use 'staging' or 'local'"
    exit 1
fi

# Verify we have an API key
if [ -z "$API_KEY" ]; then
    echo "Error: API key not found. Set PERF_AGENT_API_KEY in .deploy.conf.$ENVIRONMENT or .env"
    exit 1
fi

SOURCE_URL="https://civictechto.github.io/toronto-opendata-festivalsandevents-jsonld-proxy/all.jsonld"

echo "========================================="
echo "Toronto Events Ingestion"
echo "========================================="
echo "Environment: $ENVIRONMENT"
echo "API URL: $API_URL"
echo "Source: $SOURCE_URL"
echo "Batch size: $BATCH_SIZE"
echo "Max events: $MAX_EVENTS (0 = all)"
echo "========================================="
echo ""

# Fetch events
echo "Fetching events from Toronto Open Data proxy..."
EVENTS_JSON=$(curl -s "$SOURCE_URL")

TOTAL_EVENTS=$(echo "$EVENTS_JSON" | jq 'length')
echo "Found $TOTAL_EVENTS events"

if [ "$MAX_EVENTS" -gt 0 ] && [ "$MAX_EVENTS" -lt "$TOTAL_EVENTS" ]; then
    echo "Limiting to first $MAX_EVENTS events"
    TOTAL_EVENTS=$MAX_EVENTS
fi

# Transform and batch events
echo ""
echo "Transforming events to SEL format..."

# Transform schema.org -> EventInput format
TRANSFORMED=$(echo "$EVENTS_JSON" | jq -c --argjson maxEvents "$MAX_EVENTS" '
  if $maxEvents > 0 then .[0:$maxEvents] else . end |
  # Filter out events without required fields
  map(select(.name != null and .url != null and .startDate != null)) |
  map({
    name: .name,
    description: .description,
    startDate: .startDate,
    endDate: .endDate,
    image: .image,
    url: .url,
    keywords: .keywords,
    license: "https://creativecommons.org/publicdomain/zero/1.0/",
    location: (
      if .location."@type" == "Place" then
        {
          name: .location.name,
          streetAddress: .location.address.streetAddress,
          addressLocality: .location.address.addressLocality,
          addressRegion: .location.address.addressRegion,
          postalCode: .location.address.postalCode,
          addressCountry: .location.address.addressCountry,
          latitude: .location.geo.latitude,
          longitude: .location.geo.longitude
        }
      else
        null
      end
    ),
    virtualLocation: (
      if .location."@type" == "VirtualLocation" then
        {
          url: .location.url,
          name: .location.name
        }
      else
        null
      end
    ),
    organizer: (
      if .organizer and (.organizer.name // "") != "" then
        {
          name: .organizer.name,
          url: .organizer.url
        }
      else
        null
      end
    ),
    isAccessibleForFree: .isAccessibleForFree,
    offers: (
      if .offers then
        {
          price: (.offers.price | tostring),
          priceCurrency: .offers.priceCurrency,
          url: .offers.url
        }
      else
        null
      end
    ),
    source: {
      url: .url,
      eventId: (
        (.url // "") | 
        if test("\\?") then 
          # Extract from query params or path
          (match("(?:tickets-|events?[-/])([0-9a-zA-Z]+)") | .captures[0].string) // 
          (match("[?&]e=([^&]+)") | .captures[0].string) // 
          (match("[?&]id=([^&]+)") | .captures[0].string) //
          (sub(".*[/=]"; ""))
        else 
          # Use last path segment
          (split("/") | .[-1] | select(. != ""))
        end
      ),
      name: (
        # Use organizer name or venue name for unique source identification
        ((.organizer.name // .location.name // "Toronto Open Data") + " Events") | 
        # If still empty, use a fallback
        if . == " Events" then "Toronto Open Data Events" else . end
      ),
      license: "https://creativecommons.org/publicdomain/zero/1.0/"
    }
  })
')

# Count batches
BATCH_COUNT=$(echo "$TRANSFORMED" | jq --argjson batchSize "$BATCH_SIZE" '
  (length / $batchSize | ceil)
')

echo "Will submit in $BATCH_COUNT batches of up to $BATCH_SIZE events each"
echo ""

# Submit batches
SUCCESS_COUNT=0
FAIL_COUNT=0
BATCH_IDS=()

for i in $(seq 0 $((BATCH_COUNT - 1))); do
    BATCH_NUM=$((i + 1))
    START_IDX=$((i * BATCH_SIZE))
    
    echo "----------------------------------------"
    echo "Batch $BATCH_NUM of $BATCH_COUNT"
    echo "----------------------------------------"
    
    BATCH_EVENTS=$(echo "$TRANSFORMED" | jq -c --argjson start "$START_IDX" --argjson size "$BATCH_SIZE" '
      .[$start:($start + $size)]
    ')
    
    BATCH_JSON=$(jq -n --argjson events "$BATCH_EVENTS" '{events: $events}')
    
    echo "Submitting $(echo "$BATCH_EVENTS" | jq 'length') events..."
    
    RESPONSE=$(curl -s -w "\n%{http_code}" \
        -X POST \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $API_KEY" \
        -d "$BATCH_JSON" \
        "$API_URL/events:batch")
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')
    
    if [ "$HTTP_CODE" = "202" ]; then
        BATCH_ID=$(echo "$BODY" | jq -r '.batch_id')
        JOB_ID=$(echo "$BODY" | jq -r '.job_id')
        BATCH_IDS+=("$BATCH_ID")
        
        echo "✓ Batch accepted (HTTP $HTTP_CODE)"
        echo "  Batch ID: $BATCH_ID"
        echo "  Job ID: $JOB_ID"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo "✗ Batch failed (HTTP $HTTP_CODE)"
        echo "$BODY" | jq '.' || echo "$BODY"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    
    # Rate limit: wait between batches
    if [ "$BATCH_NUM" -lt "$BATCH_COUNT" ]; then
        sleep 1
    fi
done

echo ""
echo "========================================="
echo "Submission Complete"
echo "========================================="
echo "Batches submitted: $SUCCESS_COUNT"
echo "Batches failed: $FAIL_COUNT"
echo ""

if [ ${#BATCH_IDS[@]} -gt 0 ]; then
    echo "Batch IDs:"
    for batch_id in "${BATCH_IDS[@]}"; do
        echo "  - $batch_id"
    done
    echo ""
    echo "Check batch status with:"
    echo "  curl $API_URL/batch-status/{batch_id}"
    echo ""
    echo "Waiting 5 seconds before checking statuses..."
    sleep 5
    
    echo ""
    echo "========================================="
    echo "Batch Processing Results"
    echo "========================================="
    
    TOTAL_CREATED=0
    TOTAL_FAILED=0
    TOTAL_DUPLICATES=0
    
    for batch_id in "${BATCH_IDS[@]}"; do
        echo ""
        echo "Checking batch: $batch_id"
        
        STATUS_RESPONSE=$(curl -s "$API_URL/batch-status/$batch_id")
        
        if echo "$STATUS_RESPONSE" | jq -e '.status == "completed"' > /dev/null 2>&1; then
            CREATED=$(echo "$STATUS_RESPONSE" | jq -r '.created // 0')
            FAILED=$(echo "$STATUS_RESPONSE" | jq -r '.failed // 0')
            DUPLICATES=$(echo "$STATUS_RESPONSE" | jq -r '.duplicates // 0')
            TOTAL=$(echo "$STATUS_RESPONSE" | jq -r '.total // 0')
            
            echo "  ✓ Complete: $CREATED created, $FAILED failed, $DUPLICATES duplicates (total: $TOTAL)"
            
            TOTAL_CREATED=$((TOTAL_CREATED + CREATED))
            TOTAL_FAILED=$((TOTAL_FAILED + FAILED))
            TOTAL_DUPLICATES=$((TOTAL_DUPLICATES + DUPLICATES))
            
            # Show first few failures if any
            if [ "$FAILED" -gt 0 ]; then
                echo "  Failed events:"
                echo "$STATUS_RESPONSE" | jq -r '.results[] | select(.status == "failed") | "    - \(.name): \(.error)"' | head -5
            fi
        else
            echo "  ⏳ Still processing..."
        fi
    done
    
    echo ""
    echo "========================================="
    echo "Final Summary"
    echo "========================================="
    echo "Total events created: $TOTAL_CREATED"
    echo "Total events failed: $TOTAL_FAILED"
    echo "Total duplicates: $TOTAL_DUPLICATES"
    echo ""
fi

echo "Done!"
