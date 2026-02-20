#!/usr/bin/env bash
#
# env-audit.sh - Environment Variable Audit Script
#
# Audits environment variable files by comparing a template (.env.example)
# against an actual .env file. Detects missing required vars, missing optional
# vars, and extra vars that may be stale.
#
# Usage:
#   ./deploy/scripts/env-audit.sh <environment> [options]
#   ./deploy/scripts/env-audit.sh --self-test
#
# Arguments:
#   environment         Target environment: development, staging, production, docker
#
# Options:
#   --template <path>   Override template file path
#   --env-file <path>   Override env file path
#   --strict            Treat warnings as errors (exit 1 for any missing var)
#   --quiet             Only output errors and warnings, no info
#   --json              Output as JSON (for programmatic use)
#   --self-test         Run built-in tests
#   --help              Show usage information
#
# Exit Codes:
#   0   No errors or warnings (clean)
#   1   Missing required vars (errors), or warnings in --strict mode
#   2   Missing optional vars only (warnings, no errors)

set -euo pipefail

# Script version
SCRIPT_VERSION="1.0.0"

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PROJECT_ROOT="$(cd "${DEPLOY_DIR}/.." && pwd)"

# Required vars that have no safe default
REQUIRED_VARS=(
    "DATABASE_URL"
    "JWT_SECRET"
    "ADMIN_PASSWORD"
)

# ============================================================================
# HELPERS
# ============================================================================

# Escape a string for safe embedding in JSON
json_escape() {
    local s="$1"
    s="${s//\\/\\\\}"  # backslash
    s="${s//\"/\\\"}"  # double quote
    s="${s//$'\t'/\\t}" # tab
    s="${s//$'\n'/\\n}" # newline
    s="${s//$'\r'/\\r}" # carriage return
    printf '%s' "$s"
}

# ============================================================================
# COLOR / LOGGING
# ============================================================================

# Color codes (disabled if NO_COLOR is set or not a terminal)
if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    CYAN=''
    BOLD=''
    NC=''
fi

log() {
    local level="$1"
    shift
    local message="$*"

    local color="${NC}"
    case "$level" in
        ERROR)   color="${RED}" ;;
        WARN)    color="${YELLOW}" ;;
        SUCCESS) color="${GREEN}" ;;
        INFO)    color="${BLUE}" ;;
    esac

    echo -e "${color}[${level}]${NC} ${message}" >&2
}

# ============================================================================
# USAGE
# ============================================================================

usage() {
    cat <<EOF
Usage: $(basename "$0") <environment> [options]
       $(basename "$0") --self-test

Arguments:
  environment         Target: development, staging, production, docker

Options:
  --template <path>   Override template file path
  --env-file <path>   Override env file path
  --strict            Treat warnings as errors (exit 1 for any missing var)
  --quiet             Only output errors and warnings, no info
  --json              Output as JSON for programmatic use
  --self-test         Run built-in self-tests
  --help              Show this help

Exit Codes:
  0   Clean (no errors or warnings)
  1   Missing required vars (or warnings in --strict mode)
  2   Missing optional vars only

Default file resolution:
  development:  template=.env.example, env-file=.env
  docker:       template=deploy/docker/.env.example, env-file=deploy/docker/.env
  staging:      template=deploy/config/environments/.env.staging.example
                env-file=deploy/config/environments/.env.staging
                        (or /opt/togather/.env.staging if on server)
  production:   template=deploy/config/environments/.env.production.example
                env-file=deploy/config/environments/.env.production
                        (or /opt/togather/.env.production if on server)
EOF
}

# ============================================================================
# FILE RESOLUTION
# ============================================================================

resolve_defaults() {
    local env="$1"
    local -n _template_ref=$2
    local -n _envfile_ref=$3

    case "$env" in
        development)
            _template_ref="${PROJECT_ROOT}/.env.example"
            _envfile_ref="${PROJECT_ROOT}/.env"
            ;;
        docker)
            _template_ref="${PROJECT_ROOT}/deploy/docker/.env.example"
            _envfile_ref="${PROJECT_ROOT}/deploy/docker/.env"
            ;;
        staging)
            _template_ref="${PROJECT_ROOT}/deploy/config/environments/.env.staging.example"
            # Prefer server path if on a server, else local
            if [[ -f "/opt/togather/.env.staging" ]]; then
                _envfile_ref="/opt/togather/.env.staging"
            else
                _envfile_ref="${PROJECT_ROOT}/deploy/config/environments/.env.staging"
            fi
            ;;
        production)
            _template_ref="${PROJECT_ROOT}/deploy/config/environments/.env.production.example"
            if [[ -f "/opt/togather/.env.production" ]]; then
                _envfile_ref="/opt/togather/.env.production"
            else
                _envfile_ref="${PROJECT_ROOT}/deploy/config/environments/.env.production"
            fi
            ;;
        *)
            log "ERROR" "Unknown environment: ${env}. Valid: development, staging, production, docker"
            exit 1
            ;;
    esac
}

# ============================================================================
# PARSING
# ============================================================================

# Extract uncommented KEY=value lines from a file.
# Output: one KEY per line.
parse_keys() {
    local file="$1"
    # Match lines like KEY=value or KEY= (not starting with #, not blank)
    grep -E '^[A-Za-z_][A-Za-z0-9_]*=' "$file" | sed 's/=.*//' || true
}

# Extract commented-out KEY=value lines (optional vars in template).
# Matches lines like "# KEY=value" or "#KEY=value"
parse_optional_keys() {
    local file="$1"
    grep -E '^#[[:space:]]*[A-Za-z_][A-Za-z0-9_]*=' "$file" \
        | sed 's/^#[[:space:]]*//' \
        | sed 's/=.*//' || true
}

# Get the default value for a key from the template file.
get_template_default() {
    local file="$1"
    local key="$2"
    # Look for KEY=value in uncommented lines
    local line
    line=$(grep -E "^${key}=" "$file" | head -1 || true)
    if [[ -n "$line" ]]; then
        echo "${line#*=}"
    fi
}

# ============================================================================
# AUDIT LOGIC
# ============================================================================

run_audit() {
    local environment="$1"
    local template_file="$2"
    local env_file="$3"
    local strict="$4"
    local quiet="$5"
    local json_mode="$6"

    # Validate files exist
    if [[ ! -f "$template_file" ]]; then
        log "ERROR" "Template file not found: ${template_file}"
        exit 1
    fi
    if [[ ! -f "$env_file" ]]; then
        log "ERROR" "Env file not found: ${env_file}"
        exit 1
    fi

    # Parse keys
    local template_keys env_keys optional_keys
    mapfile -t template_keys < <(parse_keys "$template_file")
    mapfile -t env_keys      < <(parse_keys "$env_file")
    mapfile -t optional_keys < <(parse_optional_keys "$template_file")

    # Build lookup sets
    declare -A template_set optional_set env_set
    for k in "${template_keys[@]+"${template_keys[@]}"}"; do
        template_set["$k"]=1
    done
    for k in "${optional_keys[@]+"${optional_keys[@]}"}"; do
        optional_set["$k"]=1
    done
    for k in "${env_keys[@]+"${env_keys[@]}"}"; do
        env_set["$k"]=1
    done

    # Classify
    local errors=() warnings=() infos=()

    # Check template keys (required = uncommented and not in optional_set)
    for key in "${template_keys[@]+"${template_keys[@]}"}"; do
        if [[ -z "${env_set[$key]+x}" ]]; then
            # Key is in template but missing from env file
            local is_required=false
            for rk in "${REQUIRED_VARS[@]}"; do
                if [[ "$rk" == "$key" ]]; then
                    is_required=true
                    break
                fi
            done

            if [[ "$is_required" == "true" ]]; then
                errors+=("${key}")
            else
                local default_val
                default_val=$(get_template_default "$template_file" "$key")
                warnings+=("${key}|${default_val}")
            fi
        fi
    done

    # Check env keys not in template at all (not even commented out)
    for key in "${env_keys[@]+"${env_keys[@]}"}"; do
        if [[ -z "${template_set[$key]+x}" ]] && [[ -z "${optional_set[$key]+x}" ]]; then
            infos+=("${key}")
        fi
    done

    # ---- Output ----

    if [[ "$json_mode" == "true" ]]; then
        # Serialize arrays to newline-delimited strings to avoid nameref+set -u issues
        local errors_str warnings_str infos_str
        errors_str=$(printf '%s\n' "${errors[@]+"${errors[@]}"}")
        warnings_str=$(printf '%s\n' "${warnings[@]+"${warnings[@]}"}")
        infos_str=$(printf '%s\n' "${infos[@]+"${infos[@]}"}")
        output_json "$environment" "$template_file" "$env_file" \
            "$errors_str" "$warnings_str" "$infos_str" "$strict"
        return $?
    fi

    # Human-readable output
    echo ""
    echo -e "${BOLD}Environment Audit: ${environment}${NC}"
    echo -e "Template:  ${template_file}"
    echo -e "Env file:  ${env_file}"
    echo ""

    local exit_code=0

    if [[ ${#errors[@]} -gt 0 ]]; then
        echo -e "${RED}${BOLD}ERRORS (deploy will fail):${NC}"
        for key in "${errors[@]}"; do
            echo -e "  ${RED}${key}${NC} - required but missing"
        done
        echo ""
        exit_code=1
    fi

    if [[ ${#warnings[@]} -gt 0 ]]; then
        echo -e "${YELLOW}${BOLD}WARNINGS (may cause issues):${NC}"
        for entry in "${warnings[@]}"; do
            local key="${entry%%|*}"
            local default="${entry##*|}"
            if [[ -n "$default" ]]; then
                echo -e "  ${YELLOW}${key}${NC} - missing (template default: ${default})"
            else
                echo -e "  ${YELLOW}${key}${NC} - missing (no default in template)"
            fi
        done
        echo ""
        if [[ "$strict" == "true" ]]; then
            exit_code=1
        elif [[ $exit_code -eq 0 ]]; then
            exit_code=2
        fi
    fi

    if [[ ${#infos[@]} -gt 0 ]] && [[ "$quiet" != "true" ]]; then
        echo -e "${CYAN}${BOLD}INFO:${NC}"
        for key in "${infos[@]}"; do
            echo -e "  ${CYAN}${key}${NC} - in env file but not in template (may be stale)"
        done
        echo ""
    fi

    local error_count=${#errors[@]}
    local warning_count=${#warnings[@]}
    local info_count=${#infos[@]}

    local summary_color="${GREEN}"
    [[ $exit_code -eq 2 ]] && summary_color="${YELLOW}"
    [[ $exit_code -eq 1 ]] && summary_color="${RED}"

    echo -e "${summary_color}Summary: ${error_count} error(s), ${warning_count} warning(s), ${info_count} info${NC}"
    echo ""

    if [[ $exit_code -eq 0 ]]; then
        echo -e "${GREEN}[SUCCESS]${NC} Audit passed — no issues found."
        echo ""
    fi

    return $exit_code
}

# ============================================================================
# JSON OUTPUT
# ============================================================================

# Args: environment template_file env_file errors_str warnings_str infos_str strict
# *_str are newline-delimited lists (empty string = no items)
output_json() {
    local environment="$1"
    local template_file="$2"
    local env_file="$3"
    local errors_str="$4"
    local warnings_str="$5"
    local infos_str="$6"
    local strict="$7"

    # Convert newline strings back to arrays
    local errors=() warnings=() infos=()
    if [[ -n "$errors_str" ]]; then
        mapfile -t errors <<<"$errors_str"
    fi
    if [[ -n "$warnings_str" ]]; then
        mapfile -t warnings <<<"$warnings_str"
    fi
    if [[ -n "$infos_str" ]]; then
        mapfile -t infos <<<"$infos_str"
    fi

    local exit_code=0
    [[ ${#errors[@]} -gt 0 ]] && exit_code=1
    if [[ ${#warnings[@]} -gt 0 ]] && [[ $exit_code -eq 0 ]]; then
        if [[ "$strict" == "true" ]]; then
            exit_code=1
        else
            exit_code=2
        fi
    fi

    # Build JSON arrays
    local errors_json="["
    local first=true
    for key in "${errors[@]+"${errors[@]}"}"; do
        [[ "$first" != "true" ]] && errors_json+=","
        errors_json+="{\"key\":\"${key}\",\"severity\":\"error\",\"reason\":\"required but missing\"}"
        first=false
    done
    errors_json+="]"

    local warnings_json="["
    first=true
    for entry in "${warnings[@]+"${warnings[@]}"}"; do
        [[ "$first" != "true" ]] && warnings_json+=","
        local key="${entry%%|*}"
        local default="${entry##*|}"
        warnings_json+="{\"key\":\"$(json_escape "${key}")\",\"severity\":\"warning\",\"template_default\":\"$(json_escape "${default}")\"}"
        first=false
    done
    warnings_json+="]"

    local infos_json="["
    first=true
    for key in "${infos[@]+"${infos[@]}"}"; do
        [[ "$first" != "true" ]] && infos_json+=","
        infos_json+="{\"key\":\"${key}\",\"severity\":\"info\",\"reason\":\"in env file but not in template\"}"
        first=false
    done
    infos_json+="]"

    cat <<EOF
{
  "environment": "${environment}",
  "template": "${template_file}",
  "env_file": "${env_file}",
  "exit_code": ${exit_code},
  "summary": {
    "errors": ${#errors[@]},
    "warnings": ${#warnings[@]},
    "info": ${#infos[@]}
  },
  "issues": {
    "errors": ${errors_json},
    "warnings": ${warnings_json},
    "info": ${infos_json}
  }
}
EOF

    return $exit_code
}

# ============================================================================
# SELF-TEST
# ============================================================================

self_test() {
    local pass=0
    local fail=0
    local tmpdir=""
    tmpdir=$(mktemp -d)
    trap '[[ -n "${tmpdir:-}" ]] && rm -rf "$tmpdir"' EXIT

    run_test() {
        local name="$1"
        local expected_exit="$2"
        shift 2
        local actual_exit=0
        "$@" >/dev/null 2>&1 || actual_exit=$?
        if [[ $actual_exit -eq $expected_exit ]]; then
            echo -e "${GREEN}[PASS]${NC} ${name}"
            ((pass++)) || true
        else
            echo -e "${RED}[FAIL]${NC} ${name} (expected exit ${expected_exit}, got ${actual_exit})"
            ((fail++)) || true
        fi
    }

    # ---- Test fixtures ----

    # Template: required + optional + commented-out
    cat >"${tmpdir}/template.env" <<'TMPL'
DATABASE_URL=postgresql://localhost/db
JWT_SECRET=change_me
ADMIN_PASSWORD=change_me
LOG_LEVEL=info
OPTIONAL_VAR=some_default
# GITHUB_CLIENT_ID=optional_commented
# GITHUB_CLIENT_SECRET=optional_commented
TMPL

    # Clean env (all required + optional present)
    cat >"${tmpdir}/clean.env" <<'ENV'
DATABASE_URL=postgresql://real/db
JWT_SECRET=supersecret
ADMIN_PASSWORD=realpassword
LOG_LEVEL=info
OPTIONAL_VAR=myvalue
ENV

    # Missing required var (DATABASE_URL)
    cat >"${tmpdir}/missing_required.env" <<'ENV'
JWT_SECRET=supersecret
ADMIN_PASSWORD=realpassword
LOG_LEVEL=info
OPTIONAL_VAR=myvalue
ENV

    # Missing optional var (OPTIONAL_VAR present in template but not in env)
    cat >"${tmpdir}/missing_optional.env" <<'ENV'
DATABASE_URL=postgresql://real/db
JWT_SECRET=supersecret
ADMIN_PASSWORD=realpassword
LOG_LEVEL=info
ENV

    # Extra var in env file not in template
    cat >"${tmpdir}/extra_var.env" <<'ENV'
DATABASE_URL=postgresql://real/db
JWT_SECRET=supersecret
ADMIN_PASSWORD=realpassword
LOG_LEVEL=info
OPTIONAL_VAR=myvalue
LEGACY_VAR=old_value
ENV

    # Env with only commented-out template vars missing (should be clean)
    cat >"${tmpdir}/no_commented.env" <<'ENV'
DATABASE_URL=postgresql://real/db
JWT_SECRET=supersecret
ADMIN_PASSWORD=realpassword
LOG_LEVEL=info
OPTIONAL_VAR=myvalue
ENV

    echo ""
    echo -e "${BOLD}Running self-tests...${NC}"
    echo ""

    # Test 1: Clean audit -> exit 0
    run_test "Clean audit (all vars present)" 0 \
        "${BASH_SOURCE[0]}" development \
        --template "${tmpdir}/template.env" \
        --env-file "${tmpdir}/clean.env"

    # Test 2: Missing required var -> exit 1
    run_test "Missing required var (DATABASE_URL)" 1 \
        "${BASH_SOURCE[0]}" development \
        --template "${tmpdir}/template.env" \
        --env-file "${tmpdir}/missing_required.env"

    # Test 3: Missing optional var -> exit 2
    run_test "Missing optional var (OPTIONAL_VAR)" 2 \
        "${BASH_SOURCE[0]}" development \
        --template "${tmpdir}/template.env" \
        --env-file "${tmpdir}/missing_optional.env"

    # Test 4: Extra var -> exit 0 (info only)
    run_test "Extra var in env file -> exit 0 (info only)" 0 \
        "${BASH_SOURCE[0]}" development \
        --template "${tmpdir}/template.env" \
        --env-file "${tmpdir}/extra_var.env"

    # Test 5: Commented-out template var not in env -> exit 0
    run_test "Commented-out template var missing -> exit 0" 0 \
        "${BASH_SOURCE[0]}" development \
        --template "${tmpdir}/template.env" \
        --env-file "${tmpdir}/no_commented.env"

    # Test 6: --strict upgrades warnings to errors -> exit 1
    run_test "--strict: missing optional var -> exit 1" 1 \
        "${BASH_SOURCE[0]}" development \
        --template "${tmpdir}/template.env" \
        --env-file "${tmpdir}/missing_optional.env" \
        --strict

    # Test 7: --json output (missing required) -> exit 1 with JSON
    local json_output
    json_output=$("${BASH_SOURCE[0]}" development \
        --template "${tmpdir}/template.env" \
        --env-file "${tmpdir}/missing_required.env" \
        --json 2>/dev/null) || true

    if echo "$json_output" | grep -q '"exit_code": 1' && \
       echo "$json_output" | grep -q '"environment": "development"' && \
       echo "$json_output" | grep -q '"key":"DATABASE_URL"'; then
        echo -e "${GREEN}[PASS]${NC} --json output format (missing required var)"
        ((pass++)) || true
    else
        echo -e "${RED}[FAIL]${NC} --json output format (missing required var)"
        echo "  Output was: ${json_output}"
        ((fail++)) || true
    fi

    # Test 8: --json output (clean) -> exit 0 with JSON
    local json_clean
    json_clean=$("${BASH_SOURCE[0]}" development \
        --template "${tmpdir}/template.env" \
        --env-file "${tmpdir}/clean.env" \
        --json 2>/dev/null) || true

    if echo "$json_clean" | grep -q '"exit_code": 0' && \
       echo "$json_clean" | grep -q '"errors": 0'; then
        echo -e "${GREEN}[PASS]${NC} --json output format (clean audit)"
        ((pass++)) || true
    else
        echo -e "${RED}[FAIL]${NC} --json output format (clean audit)"
        echo "  Output was: ${json_clean}"
        ((fail++)) || true
    fi

    echo ""
    if [[ $fail -eq 0 ]]; then
        echo -e "${GREEN}${BOLD}All ${pass} tests passed.${NC}"
        return 0
    else
        echo -e "${RED}${BOLD}${fail} test(s) failed, ${pass} passed.${NC}"
        return 1
    fi
}

# ============================================================================
# MAIN
# ============================================================================

main() {
    local environment=""
    local template_file=""
    local env_file=""
    local strict=false
    local quiet=false
    local json_mode=false

    if [[ $# -eq 0 ]]; then
        usage
        exit 1
    fi

    # Handle --self-test and --help before positional arg parsing
    case "${1:-}" in
        --self-test)
            self_test
            exit $?
            ;;
        --help|-h)
            usage
            exit 0
            ;;
    esac

    # First positional arg is environment
    environment="$1"
    shift

    # Parse remaining options
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --template)
                if [[ -z "${2:-}" || "${2:-}" == --* ]]; then
                    log "ERROR" "--template requires a file path argument"
                    exit 1
                fi
                template_file="$2"
                shift 2
                ;;
            --env-file)
                if [[ -z "${2:-}" || "${2:-}" == --* ]]; then
                    log "ERROR" "--env-file requires a file path argument"
                    exit 1
                fi
                env_file="$2"
                shift 2
                ;;
            --strict)
                strict=true
                shift
                ;;
            --quiet)
                quiet=true
                shift
                ;;
            --json)
                json_mode=true
                shift
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                log "ERROR" "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done

    # Resolve defaults if not provided
    if [[ -z "$template_file" ]] || [[ -z "$env_file" ]]; then
        local resolved_template=""
        local resolved_envfile=""
        resolve_defaults "$environment" resolved_template resolved_envfile
        [[ -z "$template_file" ]] && template_file="$resolved_template"
        [[ -z "$env_file" ]]      && env_file="$resolved_envfile"
    fi

    run_audit "$environment" "$template_file" "$env_file" \
        "$strict" "$quiet" "$json_mode"
}

main "$@"
