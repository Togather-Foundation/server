#!/usr/bin/env bash
#
# wait-for-health.sh - Polls health endpoint until ready or timeout
#
# Usage:
#   ./wait-for-health.sh --url https://example.com/health
#   ./wait-for-health.sh --base-url https://example.com
#   ./wait-for-health.sh --slot blue
#
# Options:
#   --url <url>             Full health endpoint URL
#   --base-url <url>        Base URL (health endpoint is /health)
#   --slot <blue|green>     Slot to check (localhost 8081/8082)
#   --timeout <seconds>     Max wait time (default: 60)
#   --min-consecutive <n>   Require N consecutive healthy checks (default: 1)
#   --allow-degraded        Treat degraded as success
#   --verbose               Verbose output
#   --help                  Show usage

set -euo pipefail

timeout_seconds=60
min_consecutive=1
allow_degraded=false
verbose=false
health_url=""
base_url=""
slot=""

request_timeout=5
backoff_start=0.5
backoff_max=2
backoff_multiplier=1.5

usage() {
    cat <<EOF
Usage: $0 [options]

Options:
  --url <url>             Full health endpoint URL
  --base-url <url>        Base URL (health endpoint is /health)
  --slot <blue|green>     Slot to check (localhost 8081/8082)
  --timeout <seconds>     Max wait time (default: 60)
  --min-consecutive <n>   Require N consecutive healthy checks (default: 1)
  --allow-degraded        Treat degraded as success
  --verbose               Verbose output
  --help                  Show usage
EOF
}

log_info() {
    echo "[INFO] $*"
}

log_warn() {
    echo "[WARN] $*"
}

log_error() {
    echo "[ERROR] $*" >&2
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --url)
            health_url="$2"
            shift 2
            ;;
        --base-url)
            base_url="$2"
            shift 2
            ;;
        --slot)
            slot="$2"
            shift 2
            ;;
        --timeout)
            timeout_seconds="$2"
            shift 2
            ;;
        --min-consecutive)
            min_consecutive="$2"
            shift 2
            ;;
        --allow-degraded)
            allow_degraded=true
            shift
            ;;
        --verbose)
            verbose=true
            shift
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 2
            ;;
    esac
done

if ! command -v curl >/dev/null 2>&1; then
    log_error "curl is required but not installed"
    exit 2
fi

if [[ -z "$health_url" ]]; then
    if [[ -n "$base_url" ]]; then
        base_url="${base_url%/}"
        health_url="${base_url}/health"
    elif [[ -n "$slot" ]]; then
        case "$slot" in
            blue)
                health_url="http://localhost:8081/health"
                ;;
            green)
                health_url="http://localhost:8082/health"
                ;;
            *)
                log_error "Invalid slot: $slot (expected blue or green)"
                exit 2
                ;;
        esac
    else
        log_error "Must provide --url, --base-url, or --slot"
        usage
        exit 2
    fi
fi

if ! [[ "$timeout_seconds" =~ ^[0-9]+$ ]]; then
    log_error "Invalid --timeout value: $timeout_seconds"
    exit 2
fi

if ! [[ "$min_consecutive" =~ ^[0-9]+$ ]] || [[ "$min_consecutive" -lt 1 ]]; then
    log_error "Invalid --min-consecutive value: $min_consecutive"
    exit 2
fi

start_time=$(date +%s)
deadline=$((start_time + timeout_seconds))
sleep_interval="$backoff_start"
attempt=0
consecutive=0
last_reason=""

log_info "Waiting for health: ${health_url} (timeout: ${timeout_seconds}s)"

while true; do
    now=$(date +%s)
    if [[ $now -ge $deadline ]]; then
        break
    fi

    ((attempt++)) || true
    curl_err=$(mktemp)
    curl_status=0
    response=$(curl -sS -m "$request_timeout" -w "\n%{http_code}" "$health_url" 2>"$curl_err") || curl_status=$?
    curl_error_msg=$(cat "$curl_err")
    rm -f "$curl_err"

    body=$(echo "$response" | head -n -1)
    http_code=$(echo "$response" | tail -n 1)

    status=""
    if [[ $curl_status -eq 0 && "$http_code" == "200" ]]; then
        if command -v jq >/dev/null 2>&1; then
            status=$(echo "$body" | jq -r '.status // empty' 2>/dev/null || true)
        else
            status=$(echo "$body" | sed -n 's/.*"status"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
        fi
    fi

    if [[ "$status" == "healthy" ]] || { [[ "$allow_degraded" == "true" ]] && [[ "$status" == "degraded" ]]; }; then
        ((consecutive++)) || true
        last_reason="status=${status}"
        if [[ $consecutive -ge $min_consecutive ]]; then
            elapsed=$((now - start_time))
            log_info "Health check passed after ${elapsed}s (${last_reason})"
            exit 0
        fi
    else
        consecutive=0
        if [[ $curl_status -ne 0 ]]; then
            last_reason="curl_error"
            if [[ -n "$curl_error_msg" ]]; then
                last_reason="curl_error: ${curl_error_msg}"
            fi
        elif [[ -z "$http_code" || "$http_code" == "000" ]]; then
            last_reason="no_response"
        elif [[ "$http_code" != "200" ]]; then
            last_reason="http_${http_code}"
        elif [[ -z "$status" ]]; then
            last_reason="missing_status"
        else
            last_reason="status=${status}"
        fi
    fi

    if [[ "$verbose" == "true" ]]; then
        elapsed=$((now - start_time))
        log_info "Attempt ${attempt}: ${last_reason} (elapsed ${elapsed}s, consecutive ${consecutive}/${min_consecutive})"
    fi

    sleep "$sleep_interval"
    sleep_interval=$(awk -v s="$sleep_interval" -v m="$backoff_multiplier" -v max="$backoff_max" 'BEGIN{v=s*m; if (v>max) v=max; printf "%.2f", v}')
done

elapsed=$((deadline - start_time))
log_error "Timed out after ${elapsed}s waiting for healthy status (${last_reason})"
exit 1
