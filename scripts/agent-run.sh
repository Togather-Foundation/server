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
# On success: shows pass/fail status, timing, and a brief summary.
# On failure: extracts and displays errors (compile, test, lint, panic)
#             with log file line numbers so you can Read/Grep for context.
#
# Cleanup:
#   scripts/agent-cleanup.sh                    — clean all sessions
#   scripts/agent-cleanup.sh <session-id>       — clean one session
#   make agent-clean                            — clean all sessions via make

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

# Run the command, capture all output, preserve exit code
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
    # --- SUCCESS: minimal output, just confirm it passed ---
    echo "[agent-run] Status: PASSED"

    # Show only high-signal success lines (coverage %, completion markers)
    SUMMARY=$(grep -E '(coverage:|All .* passed|✓)' "$LOG_FILE" 2>/dev/null || true)
    if [ -n "$SUMMARY" ]; then
        echo "$SUMMARY" | tail -5
    fi
else
    # --- FAILURE: show errors prominently, this is what matters ---
    echo "[agent-run] Status: FAILED (exit code $EXIT_CODE)"

    # Count error types for the summary line
    COMPILE_ERRORS=$(grep -c -E '\.go:[0-9]+:[0-9]+:' "$LOG_FILE" 2>/dev/null || true)
    TEST_FAILURES=$(grep -c -E '^--- FAIL:' "$LOG_FILE" 2>/dev/null || true)
    FAIL_PACKAGES=$(grep -c -E '^FAIL\s' "$LOG_FILE" 2>/dev/null || true)
    PANICS=$(grep -c -E '^panic:' "$LOG_FILE" 2>/dev/null || true)
    : "${COMPILE_ERRORS:=0}" "${TEST_FAILURES:=0}" "${FAIL_PACKAGES:=0}" "${PANICS:=0}"

    echo "[agent-run] Found: ${COMPILE_ERRORS} compile errors, ${TEST_FAILURES} test failures (${FAIL_PACKAGES} packages), ${PANICS} panics"
    echo ""

    # --- Compile errors (file:line:col: message) ---
    if [ "$COMPILE_ERRORS" -gt 0 ]; then
        echo "Compile errors:"
        # Show source file paths with their error messages
        grep -E '\.go:[0-9]+:[0-9]+:' "$LOG_FILE" 2>/dev/null | head -"$MAX_SUMMARY"
        echo ""
    fi

    # --- Test failures ---
    # Extract structured failure info: test name + error message + location
    if [ "$TEST_FAILURES" -gt 0 ]; then
        echo "Test failures:"
        # Show --- FAIL lines (test names) and FAIL package lines
        grep -E '(^--- FAIL:|^FAIL\s)' "$LOG_FILE" 2>/dev/null | head -"$MAX_SUMMARY"
        echo ""

        # Show error details (the actual assertion messages)
        # Use log line numbers (L:NNN) so agent can Read the file at that offset
        echo "Error details (log line numbers for reference):"
        grep -n -E '(Error Trace:|Error:|Expected |Received |Got:|Want:|actual  :|expected:)' "$LOG_FILE" 2>/dev/null \
            | sed 's/^\([0-9]*\):/  L\1: /' \
            | head -"$MAX_SUMMARY"
        echo ""
    fi

    # --- Panics ---
    if [ "$PANICS" -gt 0 ]; then
        echo "Panics:"
        grep -n -A 5 '^panic:' "$LOG_FILE" 2>/dev/null \
            | sed 's/^\([0-9]*\):/  L\1: /' \
            | head -"$MAX_SUMMARY"
        echo ""
    fi

    # --- Fallback: if no specific patterns matched ---
    if [ "$COMPILE_ERRORS" -eq 0 ] && [ "$TEST_FAILURES" -eq 0 ] && [ "$PANICS" -eq 0 ]; then
        # Try general error patterns
        GENERAL_ERRORS=$(grep -n -i -E '(error[: ]|ERROR[: ]|fatal|FATAL|FAILED)' "$LOG_FILE" 2>/dev/null \
            | sed 's/^\([0-9]*\):/  L\1: /' \
            | head -"$MAX_SUMMARY" || true)

        if [ -n "$GENERAL_ERRORS" ]; then
            echo "Errors found:"
            echo "$GENERAL_ERRORS"
        else
            echo "No recognizable error patterns. Last 15 lines of output:"
            tail -15 "$LOG_FILE"
        fi
        echo ""
    fi

    echo "[agent-run] Full log: $LOG_FILE"
    echo "[agent-run] Use Read tool with offset to see context around specific line numbers."
fi

exit "$EXIT_CODE"
