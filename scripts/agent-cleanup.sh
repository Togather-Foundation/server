#!/usr/bin/env bash
# agent-cleanup.sh — Clean up agent output files
#
# Usage:
#   scripts/agent-cleanup.sh                  — remove all sessions
#   scripts/agent-cleanup.sh <session-id>     — remove one session
#   scripts/agent-cleanup.sh --list           — list sessions and file counts
#   scripts/agent-cleanup.sh --older-than 1h  — remove sessions older than duration
#
# This should be called at the end of an agent session or manually when
# disk space needs reclaiming.

set -uo pipefail

OUTPUT_BASE="${AGENT_OUTPUT:-.agent-output}"

# ---------------------------------------------------------------------------
# Functions
# ---------------------------------------------------------------------------

list_sessions() {
    if [ ! -d "$OUTPUT_BASE" ]; then
        echo "No agent output directory found."
        return
    fi

    echo "Agent output sessions in $OUTPUT_BASE/:"
    echo ""

    local total_files=0
    local total_size=0

    for session_dir in "$OUTPUT_BASE"/*/; do
        [ -d "$session_dir" ] || continue
        local session=$(basename "$session_dir")
        local count=$(find "$session_dir" -name '*.log' -type f 2>/dev/null | wc -l | tr -d ' ')
        local size=$(du -sh "$session_dir" 2>/dev/null | cut -f1)
        local newest=$(find "$session_dir" -name '*.log' -type f -printf '%T@ %f\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)

        echo "  $session: $count files, $size, latest: ${newest:-none}"
        total_files=$((total_files + count))
    done

    if [ "$total_files" -eq 0 ]; then
        echo "  (no sessions found)"
    else
        echo ""
        local total_size=$(du -sh "$OUTPUT_BASE" 2>/dev/null | cut -f1)
        echo "Total: $total_files files, $total_size"
    fi
}

clean_session() {
    local session="$1"
    local session_dir="$OUTPUT_BASE/$session"

    if [ ! -d "$session_dir" ]; then
        echo "Session '$session' not found in $OUTPUT_BASE/"
        return 1
    fi

    local count=$(find "$session_dir" -name '*.log' -type f 2>/dev/null | wc -l | tr -d ' ')
    rm -rf "$session_dir"
    echo "Cleaned session '$session' ($count files)"
}

clean_all() {
    if [ ! -d "$OUTPUT_BASE" ]; then
        echo "No agent output directory found."
        return
    fi

    local count=$(find "$OUTPUT_BASE" -name '*.log' -type f 2>/dev/null | wc -l | tr -d ' ')
    rm -rf "$OUTPUT_BASE"
    echo "Cleaned all agent output ($count files)"
}

clean_older_than() {
    local duration="$1"

    if [ ! -d "$OUTPUT_BASE" ]; then
        echo "No agent output directory found."
        return
    fi

    # Parse duration (e.g., "1h", "30m", "2d")
    local minutes
    case "$duration" in
        *h) minutes=$(( ${duration%h} * 60 )) ;;
        *m) minutes=${duration%m} ;;
        *d) minutes=$(( ${duration%d} * 1440 )) ;;
        *)  echo "Invalid duration: $duration (use e.g., 1h, 30m, 2d)"; exit 1 ;;
    esac

    local cleaned=0
    for session_dir in "$OUTPUT_BASE"/*/; do
        [ -d "$session_dir" ] || continue

        # Check if all files in this session are older than the threshold
        local recent=$(find "$session_dir" -name '*.log' -type f -mmin -"$minutes" 2>/dev/null | wc -l | tr -d ' ')
        if [ "$recent" -eq 0 ]; then
            local session=$(basename "$session_dir")
            local count=$(find "$session_dir" -name '*.log' -type f 2>/dev/null | wc -l | tr -d ' ')
            rm -rf "$session_dir"
            echo "Cleaned session '$session' ($count files, older than $duration)"
            cleaned=$((cleaned + 1))
        fi
    done

    if [ "$cleaned" -eq 0 ]; then
        echo "No sessions older than $duration found."
    fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

case "${1:-}" in
    --list|-l)
        list_sessions
        ;;
    --older-than)
        if [ -z "${2:-}" ]; then
            echo "Usage: scripts/agent-cleanup.sh --older-than <duration>"
            echo "  duration: 1h, 30m, 2d, etc."
            exit 1
        fi
        clean_older_than "$2"
        ;;
    --help|-h)
        echo "Usage:"
        echo "  scripts/agent-cleanup.sh                  — remove all sessions"
        echo "  scripts/agent-cleanup.sh <session-id>     — remove one session"
        echo "  scripts/agent-cleanup.sh --list           — list sessions with stats"
        echo "  scripts/agent-cleanup.sh --older-than 1h  — remove stale sessions"
        ;;
    "")
        clean_all
        ;;
    *)
        clean_session "$1"
        ;;
esac
