#!/usr/bin/env bash
# agent-run.sh — Agent-aware command runner
#
# Captures full output to .agent-output/<session>/, emits a concise summary
# to stdout so coding agents preserve context window while retaining access
# to the complete output via Grep/Read.
#
# Usage:
#   scripts/agent-run.sh <command> [args...]
#   scripts/agent-run.sh make test
#   scripts/agent-run.sh go build ./...
#   AGENT_SESSION=abc123 scripts/agent-run.sh make ci
#
# Environment:
#   AGENT_SESSION  — Session ID for isolating output from parallel sessions.
#                    Auto-generated if not set. Pass this to agent-cleanup.sh
#                    to clean up only your session's files.
#   AGENT_OUTPUT   — Override output directory (default: .agent-output)
#   AGENT_SUMMARY  — Max error/warning lines to show (default: 30)
#
# The script:
#   1. Runs the command, capturing all stdout+stderr to a log file
#   2. Reports exit status and timing
#   3. On failure: extracts error lines (Go compile errors, test failures, lint issues)
#   4. On success: shows only summary lines (ok, PASS, coverage, checkmarks)
#   5. Always tells you where the full log is so you can grep/read it
#
# Cleanup:
#   scripts/agent-cleanup.sh                    — clean all sessions
#   scripts/agent-cleanup.sh <session-id>       — clean one session
#   AGENT=1 make agent-clean                    — clean all sessions via make

set -uo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

OUTPUT_BASE="${AGENT_OUTPUT:-.agent-output}"
SESSION="${AGENT_SESSION:-$(date +%s)-$$}"
OUTPUT_DIR="${OUTPUT_BASE}/${SESSION}"
MAX_SUMMARY="${AGENT_SUMMARY:-30}"

# ---------------------------------------------------------------------------
# Validate
# ---------------------------------------------------------------------------

if [ $# -eq 0 ]; then
    echo "[agent-run] Error: no command specified"
    echo "[agent-run] Usage: scripts/agent-run.sh <command> [args...]"
    exit 1
fi

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

mkdir -p "$OUTPUT_DIR"

# Generate log filename from command (sanitize for filesystem)
CMD_SLUG=$(echo "$*" | tr ' /' '-' | tr -cd 'a-zA-Z0-9._-' | head -c 80)
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
LOG_FILE="${OUTPUT_DIR}/${CMD_SLUG}-${TIMESTAMP}.log"

# ---------------------------------------------------------------------------
# Run command
# ---------------------------------------------------------------------------

echo "[agent-run] Running: $*"
echo "[agent-run] Session: $SESSION"

START_TIME=$(date +%s)

# Run the command, capture output, preserve exit code
set +e
"$@" > "$LOG_FILE" 2>&1
EXIT_CODE=$?
set -e

END_TIME=$(date +%s)
ELAPSED=$(( END_TIME - START_TIME ))
TOTAL_LINES=$(wc -l < "$LOG_FILE" | tr -d ' ')

echo "[agent-run] Full output: $LOG_FILE ($TOTAL_LINES lines, ${ELAPSED}s)"

# ---------------------------------------------------------------------------
# Extract summary
# ---------------------------------------------------------------------------

if [ "$EXIT_CODE" -eq 0 ]; then
    # --- SUCCESS ---
    echo "[agent-run] Status: PASSED"

    # Show meaningful success lines (test results, coverage, completion markers)
    SUMMARY=$(grep -E '(^ok[[:space:]]|^PASS$|^PASS |coverage:|All .* passed|successful|is up to date|is valid|properly formatted)' "$LOG_FILE" 2>/dev/null | tail -10)
    if [ -n "$SUMMARY" ]; then
        echo "$SUMMARY"
    fi
else
    # --- FAILURE ---
    echo "[agent-run] Status: FAILED (exit code $EXIT_CODE)"

    # Count different error types
    COMPILE_ERRORS=$(grep -c -E '\.go:[0-9]+:[0-9]+:' "$LOG_FILE" 2>/dev/null || echo "0")
    TEST_FAILURES=$(grep -c -E '(--- FAIL|^FAIL[[:space:]])' "$LOG_FILE" 2>/dev/null || echo "0")
    PANICS=$(grep -c -E '^panic:' "$LOG_FILE" 2>/dev/null || echo "0")
    LINT_ISSUES=$(grep -c -E '^\S+\.go:[0-9]+:[0-9]+: ' "$LOG_FILE" 2>/dev/null || echo "0")

    echo "[agent-run] Found: ${COMPILE_ERRORS} compile errors, ${TEST_FAILURES} test failures, ${PANICS} panics, ${LINT_ISSUES} lint issues"
    echo "[agent-run] ---"

    # Extract the most useful error lines
    # Priority: compile errors > test failures > panics > general errors
    ERRORS=""

    # Go compile errors (file:line:col: message)
    if [ "$COMPILE_ERRORS" -gt 0 ]; then
        ERRORS=$(grep -n -E '\.go:[0-9]+:[0-9]+:' "$LOG_FILE" | head -"$MAX_SUMMARY")
    fi

    # Test failures (--- FAIL lines with context)
    if [ "$TEST_FAILURES" -gt 0 ]; then
        TEST_ERRS=$(grep -n -E '(--- FAIL|^FAIL[[:space:]]|Error Trace:|Error:|Expected|Received|Got:|Want:)' "$LOG_FILE" | head -"$MAX_SUMMARY")
        if [ -n "$ERRORS" ]; then
            ERRORS="${ERRORS}
${TEST_ERRS}"
        else
            ERRORS="$TEST_ERRS"
        fi
    fi

    # Panics
    if [ "$PANICS" -gt 0 ]; then
        PANIC_LINES=$(grep -n -A 3 '^panic:' "$LOG_FILE" | head -"$MAX_SUMMARY")
        if [ -n "$ERRORS" ]; then
            ERRORS="${ERRORS}
${PANIC_LINES}"
        else
            ERRORS="$PANIC_LINES"
        fi
    fi

    # If we found nothing specific, fall back to general error/warning grep
    if [ -z "$ERRORS" ]; then
        ERRORS=$(grep -n -i -E '(error[: ]|ERROR[: ]|fatal|FATAL|failed|FAILED)' "$LOG_FILE" | head -"$MAX_SUMMARY")
    fi

    # Last resort: show the last 15 lines (often contains the actual error)
    if [ -z "$ERRORS" ]; then
        echo "[agent-run] No recognizable error patterns found. Last 15 lines:"
        tail -15 "$LOG_FILE"
    else
        echo "$ERRORS" | head -"$MAX_SUMMARY"

        # Note if truncated
        TOTAL_ERRORS=$(( COMPILE_ERRORS + TEST_FAILURES + PANICS ))
        if [ "$TOTAL_ERRORS" -gt "$MAX_SUMMARY" ]; then
            echo "[agent-run] ... (showing $MAX_SUMMARY of $TOTAL_ERRORS issues)"
        fi
    fi

    echo "[agent-run] ---"
    echo "[agent-run] Grep the full log for details: $LOG_FILE"
fi

exit "$EXIT_CODE"
