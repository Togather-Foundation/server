#!/usr/bin/env bash
# Export detailed failure information from batch ingestion
# Usage: ./scripts/export-failures.sh [staging|local] [batch_id1] [batch_id2] ...

set -euo pipefail

ENVIRONMENT="${1:-staging}"
shift || true

BATCH_IDS=("$@")

if [ ${#BATCH_IDS[@]} -eq 0 ]; then
    echo "Usage: $0 [staging|local] <batch_id1> [batch_id2] ..."
    echo ""
    echo "Example:"
    echo "  $0 staging 01KGWE1R0WHTG3DN2G3FJEQT1X 01KGWE1SDVFHN1KFQCKA74889Q"
    exit 1
fi

# Load environment configuration
if [ "$ENVIRONMENT" = "staging" ]; then
    if [ -f .deploy.conf.staging ]; then
        source .deploy.conf.staging
        SSH_CMD="ssh ${SSH_HOST:-togather}"
    else
        echo "Error: .deploy.conf.staging not found"
        exit 1
    fi
    DB_ACCESS="$SSH_CMD 'cd /opt/togather && source .env.staging && psql \"\$DATABASE_URL\"'"
elif [ "$ENVIRONMENT" = "local" ]; then
    if [ -f .env ]; then
        source .env
    fi
    DB_ACCESS="psql \"\$DATABASE_URL\""
    DATABASE_URL="${DATABASE_URL:-}"
    if [ -z "$DATABASE_URL" ]; then
        echo "Error: DATABASE_URL not set in .env"
        exit 1
    fi
else
    echo "Error: Unknown environment '$ENVIRONMENT'. Use 'staging' or 'local'"
    exit 1
fi

OUTPUT_DIR="failure-reports"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="$OUTPUT_DIR/failures_${ENVIRONMENT}_${TIMESTAMP}.json"

mkdir -p "$OUTPUT_DIR"

echo "========================================="
echo "Failure Export Tool"
echo "========================================="
echo "Environment: $ENVIRONMENT"
echo "Batch IDs: ${BATCH_IDS[*]}"
echo "Output: $REPORT_FILE"
echo "========================================="
echo ""

# Build SQL query to get all failures
BATCH_ID_LIST=$(printf "'%s'," "${BATCH_IDS[@]}" | sed 's/,$//')

SQL_QUERY="
WITH failures AS (
    SELECT 
        bir.batch_id,
        bir.results,
        bir.completed_at,
        rj.args AS job_args
    FROM batch_ingestion_results bir
    JOIN river_job rj ON rj.args->>'batch_id' = bir.batch_id
    WHERE bir.batch_id IN ($BATCH_ID_LIST)
)
SELECT json_agg(
    json_build_object(
        'batch_id', f.batch_id,
        'completed_at', f.completed_at,
        'failures', (
            SELECT json_agg(
                json_build_object(
                    'index', (result->>'index')::int,
                    'error', result->>'error',
                    'event', f.job_args->'events'->(result->>'index')::int
                )
            )
            FROM jsonb_array_elements(f.results) AS result
            WHERE result->>'status' = 'failed'
        )
    )
) FROM failures f;
"

echo "Querying database for failure details..."

if [ "$ENVIRONMENT" = "staging" ]; then
    RESULT=$(ssh "$SSH_HOST" "cd /opt/togather && source .env.staging && psql \"\$DATABASE_URL\" -t -A -c \"$SQL_QUERY\"")
else
    RESULT=$(psql "$DATABASE_URL" -t -A -c "$SQL_QUERY")
fi

if [ "$RESULT" = "null" ] || [ -z "$RESULT" ]; then
    echo "No failures found in specified batches."
    exit 0
fi

echo "$RESULT" | jq '.' > "$REPORT_FILE"

FAILURE_COUNT=$(echo "$RESULT" | jq '[.[] | .failures | length] | add')

echo ""
echo "✓ Export complete!"
echo ""
echo "Found $FAILURE_COUNT failed event(s) across ${#BATCH_IDS[@]} batch(es)"
echo ""
echo "Report saved to: $REPORT_FILE"
echo ""

# Generate human-readable summary
echo "========================================="
echo "Failure Summary"
echo "========================================="
echo ""

BATCH_NUM=0
for batch_id in "${BATCH_IDS[@]}"; do
    BATCH_NUM=$((BATCH_NUM + 1))
    
    BATCH_FAILURES=$(echo "$RESULT" | jq -r --arg batch_id "$batch_id" '.[] | select(.batch_id == $batch_id) | .failures | length')
    
    if [ "$BATCH_FAILURES" = "null" ] || [ "$BATCH_FAILURES" = "0" ]; then
        continue
    fi
    
    echo "Batch #$BATCH_NUM: $batch_id"
    echo "  Failures: $BATCH_FAILURES"
    echo ""
    
    # Show each failure
    echo "$RESULT" | jq -r --arg batch_id "$batch_id" '
        .[] | select(.batch_id == $batch_id) | .failures[] | 
        "  Failure at index \(.index):\n" +
        "    Error: \(.error)\n" +
        "    Event: \(.event.name // "N/A")\n" +
        "    URL: \(.event.url // "N/A")\n" +
        "    Start: \(.event.startDate // "N/A")\n" +
        "    End: \(.event.endDate // "N/A")\n" +
        "    Location: \(.event.location.name // .event.virtualLocation.name // "N/A")\n" +
        "    Organizer: \(.event.organizer.name // "N/A")\n"
    '
done

echo "========================================="
echo ""
echo "For detailed analysis, open the JSON file:"
echo "  cat $REPORT_FILE | jq '.'"
echo ""
echo "To view a specific failure:"
echo "  cat $REPORT_FILE | jq '.[0].failures[0]'"
echo ""

# Generate markdown report
MARKDOWN_FILE="${REPORT_FILE%.json}.md"
{
    echo "# Failure Analysis Report"
    echo ""
    echo "**Environment**: $ENVIRONMENT"
    echo "**Generated**: $(date -u +"%Y-%m-%d %H:%M:%S UTC")"
    echo "**Total Batches**: ${#BATCH_IDS[@]}"
    echo "**Total Failures**: $FAILURE_COUNT"
    echo ""
    echo "---"
    echo ""
    
    BATCH_NUM=0
    for batch_id in "${BATCH_IDS[@]}"; do
        BATCH_NUM=$((BATCH_NUM + 1))
        
        BATCH_FAILURES=$(echo "$RESULT" | jq -r --arg batch_id "$batch_id" '.[] | select(.batch_id == $batch_id) | .failures | length')
        
        if [ "$BATCH_FAILURES" = "null" ] || [ "$BATCH_FAILURES" = "0" ]; then
            continue
        fi
        
        echo "## Batch #$BATCH_NUM: \`$batch_id\`"
        echo ""
        echo "**Failures**: $BATCH_FAILURES"
        echo ""
        
        # Use jq to iterate over failures and generate markdown
        echo "$RESULT" | jq -r --arg batch_id "$batch_id" '
            .[] | select(.batch_id == $batch_id) | .failures[] |
            "### Failure (Index \(.index))\n\n" +
            "**Error**: `\(.error)`\n\n" +
            "| Field | Value |\n" +
            "|-------|-------|\n" +
            "| Name | \(.event.name // "N/A") |\n" +
            "| URL | \(.event.url // "N/A") |\n" +
            "| Start Date | `\(.event.startDate // "N/A")` |\n" +
            "| End Date | `\(.event.endDate // "N/A")` |\n" +
            "| Location | \(.event.location.name // .event.virtualLocation.name // "N/A") |\n" +
            "| Organizer | \(.event.organizer.name // "N/A") |\n\n" +
            "<details>\n<summary>Full Event JSON</summary>\n\n```json\n" +
            (.event | tojson) +
            "\n```\n\n</details>\n\n"
        '
        
        echo "---"
        echo ""
    done
} > "$MARKDOWN_FILE"

echo "Markdown report saved to: $MARKDOWN_FILE"
echo ""
echo "Done!"
