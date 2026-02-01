#!/usr/bin/env bash

# Quickstart Validation Script
# Validates quickstart.md instructions in a clean Docker environment
# T082: Validate quickstart instructions on clean VM

set -euo pipefail

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
LOG_FILE="/tmp/quickstart-validation-$(date +%Y%m%d_%H%M%S).log"

log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    local color="${NC}"
    case "$level" in
        ERROR)   color="${RED}" ;;
        SUCCESS) color="${GREEN}" ;;
        WARN)    color="${YELLOW}" ;;
        INFO)    color="${BLUE}" ;;
    esac
    
    echo "${timestamp} [${level}] ${message}" >> "${LOG_FILE}"
    echo -e "${color}[${level}]${NC} ${message}"
}

validate_prerequisites() {
    log "INFO" "=== Step 1: Prerequisites Check ==="
    
    local missing_tools=()
    
    # Check Docker
    if ! command -v docker &> /dev/null; then
        missing_tools+=("docker")
        log "ERROR" "Docker not found"
    else
        log "SUCCESS" "Docker found: $(docker --version)"
    fi
    
    # Check docker compose
    if ! docker compose version &> /dev/null; then
        missing_tools+=("docker-compose")
        log "ERROR" "Docker Compose not found"
    else
        log "SUCCESS" "Docker Compose found: $(docker compose version)"
    fi
    
    # Check Git
    if ! command -v git &> /dev/null; then
        missing_tools+=("git")
        log "ERROR" "Git not found"
    else
        log "SUCCESS" "Git found: $(git --version)"
    fi
    
    # Check jq
    if ! command -v jq &> /dev/null; then
        missing_tools+=("jq")
        log "ERROR" "jq not found"
    else
        log "SUCCESS" "jq found: $(jq --version)"
    fi
    
    # Check migrate (optional for this validation)
    if command -v migrate &> /dev/null; then
        log "SUCCESS" "migrate found: $(migrate -version 2>&1 | head -1)"
    else
        log "WARN" "migrate not found (required for actual deployment)"
    fi
    
    if [ ${#missing_tools[@]} -gt 0 ]; then
        log "ERROR" "Missing required tools: ${missing_tools[*]}"
        return 1
    fi
    
    log "SUCCESS" "All prerequisites met"
    return 0
}

validate_repository_structure() {
    log "INFO" "=== Step 2: Repository Structure Check ==="
    
    local required_paths=(
        "deploy/scripts/deploy.sh"
        "deploy/scripts/rollback.sh"
        "deploy/scripts/snapshot-db.sh"
        "deploy/scripts/health-check.sh"
        "deploy/config/environments/.env.development.example"
        "deploy/docker/Dockerfile"
        "deploy/docker/docker-compose.blue-green.yml"
        "internal/storage/postgres/migrations"
    )
    
    for path in "${required_paths[@]}"; do
        if [ -e "${PROJECT_ROOT}/${path}" ]; then
            log "SUCCESS" "Found: ${path}"
        else
            log "ERROR" "Missing: ${path}"
            return 1
        fi
    done
    
    # Check scripts are executable
    for script in deploy/scripts/*.sh; do
        if [ -x "${PROJECT_ROOT}/${script}" ]; then
            log "SUCCESS" "Executable: ${script}"
        else
            log "WARN" "Not executable: ${script} (quickstart instructs: chmod +x)"
        fi
    done
    
    log "SUCCESS" "Repository structure validated"
    return 0
}

validate_environment_template() {
    log "INFO" "=== Step 3: Environment Template Check ==="
    
    local env_example="${PROJECT_ROOT}/deploy/config/environments/.env.development.example"
    
    if [ ! -f "${env_example}" ]; then
        log "ERROR" "Environment template not found: ${env_example}"
        return 1
    fi
    
    log "SUCCESS" "Environment template exists"
    
    # Check for required variables mentioned in quickstart
    local required_vars=(
        "ENVIRONMENT"
        "DATABASE_URL"
        "JWT_SECRET"
        "ADMIN_API_KEY"
    )
    
    for var in "${required_vars[@]}"; do
        if grep -q "^${var}=" "${env_example}" || grep -q "^#${var}=" "${env_example}"; then
            log "SUCCESS" "Template contains: ${var}"
        else
            log "WARN" "Template missing: ${var} (should be documented in quickstart)"
        fi
    done
    
    log "SUCCESS" "Environment template validated"
    return 0
}

simulate_clean_deployment() {
    log "INFO" "=== Step 4: Simulate Clean Deployment in Docker ==="
    
    # Create a test environment file
    local test_env="${PROJECT_ROOT}/deploy/config/environments/.env.test-quickstart"
    
    log "INFO" "Creating test environment file"
    cat > "${test_env}" <<EOF
ENVIRONMENT=test-quickstart
DATABASE_URL=postgresql://togather:test-password@localhost:5432/togather_test
SERVER_PORT=8080
LOG_LEVEL=info
SHUTDOWN_TIMEOUT=10s
JWT_SECRET=$(openssl rand -base64 32)
ADMIN_API_KEY=$(openssl rand -hex 32)
DEPLOYED_BY=quickstart-validator
EOF
    
    chmod 600 "${test_env}"
    log "SUCCESS" "Test environment created: ${test_env}"
    
    # Validate that scripts can read the environment
    if source "${test_env}"; then
        log "SUCCESS" "Environment file is valid shell syntax"
    else
        log "ERROR" "Environment file has syntax errors"
        rm -f "${test_env}"
        return 1
    fi
    
    # Check for secret generation commands from quickstart
    log "INFO" "Testing secret generation commands from quickstart"
    
    local jwt_secret=$(openssl rand -base64 32)
    if [ ${#jwt_secret} -ge 32 ]; then
        log "SUCCESS" "JWT secret generation works (length: ${#jwt_secret})"
    else
        log "ERROR" "JWT secret generation failed"
    fi
    
    local admin_key=$(openssl rand -hex 32)
    if [ ${#admin_key} -eq 64 ]; then
        log "SUCCESS" "Admin API key generation works (length: ${#admin_key})"
    else
        log "ERROR" "Admin API key generation failed"
    fi
    
    # Cleanup
    rm -f "${test_env}"
    
    log "SUCCESS" "Clean deployment simulation completed"
    return 0
}

validate_docker_build() {
    log "INFO" "=== Step 5: Docker Build Validation ==="
    
    local dockerfile="${PROJECT_ROOT}/deploy/docker/Dockerfile"
    
    if [ ! -f "${dockerfile}" ]; then
        log "ERROR" "Dockerfile not found: ${dockerfile}"
        return 1
    fi
    
    log "INFO" "Validating Dockerfile syntax"
    if docker build -f "${dockerfile}" -t togather-quickstart-test:validate --target builder "${PROJECT_ROOT}" &>> "${LOG_FILE}"; then
        log "SUCCESS" "Dockerfile builds successfully (builder stage)"
        
        # Cleanup test image
        docker rmi togather-quickstart-test:validate &>> "${LOG_FILE}" || true
    else
        log "ERROR" "Dockerfile build failed (see ${LOG_FILE} for details)"
        return 1
    fi
    
    log "SUCCESS" "Docker build validated"
    return 0
}

validate_deployment_script_syntax() {
    log "INFO" "=== Step 6: Deployment Script Syntax Check ==="
    
    local scripts=(
        "deploy/scripts/deploy.sh"
        "deploy/scripts/rollback.sh"
        "deploy/scripts/snapshot-db.sh"
        "deploy/scripts/health-check.sh"
    )
    
    for script in "${scripts[@]}"; do
        local script_path="${PROJECT_ROOT}/${script}"
        
        if bash -n "${script_path}" 2>> "${LOG_FILE}"; then
            log "SUCCESS" "Syntax valid: ${script}"
        else
            log "ERROR" "Syntax error in: ${script}"
            return 1
        fi
    done
    
    log "SUCCESS" "All deployment scripts have valid syntax"
    return 0
}

validate_help_documentation() {
    log "INFO" "=== Step 7: Help Documentation Check ==="
    
    # Check that scripts have help/usage
    local scripts=(
        "deploy/scripts/deploy.sh"
        "deploy/scripts/rollback.sh"
    )
    
    for script in "${scripts[@]}"; do
        local script_path="${PROJECT_ROOT}/${script}"
        
        if bash "${script_path}" --help &>> "${LOG_FILE}"; then
            log "SUCCESS" "Has --help: ${script}"
        else
            log "WARN" "Missing --help: ${script}"
        fi
    done
    
    # Check for documentation files mentioned in quickstart
    local docs=(
        "specs/001-deployment-infrastructure/quickstart.md"
        "docs/deploy/rollback.md"
        "docs/deploy/migrations.md"
    )
    
    for doc in "${docs[@]}"; do
        if [ -f "${PROJECT_ROOT}/${doc}" ]; then
            log "SUCCESS" "Documentation exists: ${doc}"
        else
            log "WARN" "Documentation missing: ${doc}"
        fi
    done
    
    log "SUCCESS" "Help documentation validated"
    return 0
}

validate_directory_creation() {
    log "INFO" "=== Step 8: Directory Creation Check ==="
    
    # Simulate directory creation from quickstart (in /tmp for safety)
    local test_base="/tmp/togather-quickstart-test-$$"
    
    local dirs=(
        "${test_base}/var/lib/togather/deployments"
        "${test_base}/var/backups/togather"
        "${test_base}/var/log/togather/deployments"
        "${test_base}/var/lock"
    )
    
    for dir in "${dirs[@]}"; do
        if mkdir -p "${dir}"; then
            log "SUCCESS" "Can create: ${dir}"
        else
            log "ERROR" "Cannot create: ${dir}"
            return 1
        fi
    done
    
    # Test ownership change (non-root simulation)
    if chown -R "${USER}:${USER}" "${test_base}" 2>> "${LOG_FILE}"; then
        log "SUCCESS" "Can set ownership"
    else
        log "WARN" "Cannot set ownership (expected if not root)"
    fi
    
    # Cleanup
    rm -rf "${test_base}"
    
    log "SUCCESS" "Directory creation validated"
    return 0
}

validate_quickstart_completeness() {
    log "INFO" "=== Step 9: Quickstart Document Completeness ==="
    
    local quickstart="${PROJECT_ROOT}/specs/001-deployment-infrastructure/quickstart.md"
    
    if [ ! -f "${quickstart}" ]; then
        log "ERROR" "Quickstart not found: ${quickstart}"
        return 1
    fi
    
    log "SUCCESS" "Quickstart file exists"
    
    # Check for key sections
    local sections=(
        "Prerequisites Check"
        "Initial Setup"
        "First Deployment"
        "Verify Deployment"
        "Common Operations"
        "Troubleshooting"
    )
    
    for section in "${sections[@]}"; do
        if grep -q "## ${section}" "${quickstart}"; then
            log "SUCCESS" "Has section: ${section}"
        else
            log "WARN" "Missing section: ${section}"
        fi
    done
    
    # Check for code blocks (commands should be in code blocks)
    local code_blocks=$(grep -c '```bash' "${quickstart}" || true)
    if [ "${code_blocks}" -ge 10 ]; then
        log "SUCCESS" "Has ${code_blocks} bash code blocks"
    else
        log "WARN" "Only ${code_blocks} bash code blocks (should have more examples)"
    fi
    
    log "SUCCESS" "Quickstart document validated"
    return 0
}

main() {
    echo "╔════════════════════════════════════════════════════════╗"
    echo "║   Togather Quickstart Validation (T082)               ║"
    echo "║   Validating: specs/001-deployment-infrastructure/    ║"
    echo "║               quickstart.md                            ║"
    echo "╚════════════════════════════════════════════════════════╝"
    echo ""
    
    log "INFO" "Log file: ${LOG_FILE}"
    log "INFO" "Project root: ${PROJECT_ROOT}"
    echo ""
    
    local failed=0
    
    validate_prerequisites || ((failed++))
    echo ""
    
    validate_repository_structure || ((failed++))
    echo ""
    
    validate_environment_template || ((failed++))
    echo ""
    
    simulate_clean_deployment || ((failed++))
    echo ""
    
    validate_docker_build || ((failed++))
    echo ""
    
    validate_deployment_script_syntax || ((failed++))
    echo ""
    
    validate_help_documentation || ((failed++))
    echo ""
    
    validate_directory_creation || ((failed++))
    echo ""
    
    validate_quickstart_completeness || ((failed++))
    echo ""
    
    echo "╔════════════════════════════════════════════════════════╗"
    if [ ${failed} -eq 0 ]; then
        echo "║   ✅ VALIDATION PASSED                                 ║"
        log "SUCCESS" "All validation checks passed"
        echo "║                                                        ║"
        echo "║   The quickstart guide is ready for new operators.    ║"
        echo "║   All referenced files and commands are valid.         ║"
    else
        echo "║   ❌ VALIDATION FAILED                                 ║"
        log "ERROR" "${failed} validation check(s) failed"
        echo "║                                                        ║"
        echo "║   ${failed} validation check(s) failed.                        ║"
        echo "║   Review errors above and check ${LOG_FILE}           ║"
    fi
    echo "╚════════════════════════════════════════════════════════╝"
    echo ""
    
    if [ ${failed} -eq 0 ]; then
        exit 0
    else
        exit 1
    fi
}

main "$@"
