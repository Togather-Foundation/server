#!/usr/bin/env bash
# No application services or database are started by this suite.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT
REAL_DOCKER=$(command -v docker || true)
export ROOT TEST_DIR REAL_DOCKER
mkdir -p "$TEST_DIR/work/deploy/docker" "$TEST_DIR/work/internal/storage/postgres/migrations" "$TEST_DIR/bin"
printf '%s\n' 'DATABASE_URL=postgresql://fixture:secret@db:5432/events' > "$TEST_DIR/db.env"
printf 'DATABASE_URL=\n' > "$TEST_DIR/empty.env"
printf 'echo secret >&2; return 1\n' > "$TEST_DIR/bad.env"
touch "$TEST_DIR/work/deploy/docker/docker-compose.yml"
sed -n '/^preflight_config()/,/^}/p; /^snapshot_database_url()/,/^}/p; /^snapshot_container()/,/^}/p; /^create_db_snapshot()/,/^}/p; /^deploy()/,/^}/p' "${DEPLOY_SCRIPT_UNDER_TEST:-$ROOT/deploy/scripts/deploy.sh}" > "$TEST_DIR/functions.sh"
cat > "$TEST_DIR/run.sh" <<'RUN'
#!/usr/bin/env bash
set -euo pipefail
source "$TEST_DIR/functions.sh"
PROJECT_ROOT="$TEST_DIR/work"
DOCKER_DIR="$PROJECT_ROOT/deploy/docker"
COMPOSE_CMD='docker compose'
DEPLOYED_BY=test
GIT_SHORT_COMMIT=test
SNAPSHOT_DIR="$TEST_DIR/snapshots"
PREFLIGHT_DATABASE_URL=stale
PREFLIGHT_DB_CONTAINER=stale
command() {
    if [[ "$CASE" == host-tools && "$*" == '-v pg_dump' ]]; then return 1; fi
    builtin command "$@"
}
log() { printf '%s %s\n' "$1" "$2"; }
discover_env_file() {
    case "$CASE" in
        missing-env) : ;;
        source-failure) echo "$TEST_DIR/bad.env" ;;
        missing-target|malformed-target|multiline-target) echo "$TEST_DIR/empty.env" ;;
        *) echo "$TEST_DIR/db.env" ;;
    esac
}
resolve_database_url() { :; }
validate_config() { :; }
validate_tool_versions() { :; }
validate_git_commit() { :; }
acquire_lock() { :; }
preflight_sources() { echo source >> "$TEST_DIR/events"; }
check_disk_space() { :; }
prune_docker_cache() { echo prune >> "$TEST_DIR/events"; }
build_docker_image() {
    echo build >> "$TEST_DIR/events"
    [[ "$CASE" != changed-target ]] || printf 'DATABASE_URL=postgresql://fixture:secret@other:5432/events\n' > "$TEST_DIR/db.env"
    touch "$TEST_DIR/built"
}
run_migrations() { echo migrated >> "$TEST_DIR/events"; return 78; }
get_inactive_slot() { echo green; }
get_active_slot() { echo blue; }
deploy_to_slot() { echo slot >> "$TEST_DIR/events"; return 79; }
switch_traffic() { echo traffic >> "$TEST_DIR/events"; }
docker() {
    if [[ "$1" == compose ]]; then
        [[ "$2" == --env-file && ( "$3" == "$TEST_DIR/db.env" || "$3" == "$TEST_DIR/empty.env" || "$3" == "$TEST_DIR/bad.env" ) && "$4" == -f && "$5" == "$DOCKER_DIR/docker-compose.yml" ]] || return 90
        local selected_env="$3"
        shift 5
        case "$*" in
            'config --quiet')
                echo config >> "$TEST_DIR/events"
                if [[ "$CASE" == real-invalid-compose ]]; then
                    env -u PREFLIGHT_FIXTURE_PASSWORD "$REAL_DOCKER" compose --env-file "$selected_env" -f "$DOCKER_DIR/docker-compose.yml" config --quiet
                    return $?
                fi
                [[ "$CASE" != invalid-compose ]] || { echo secret >&2; return 1; }
                ;;
            'ps --status running -q togather-db')
                echo discovery >> "$TEST_DIR/events"
                case "$CASE" in
                    no-container) return 0 ;;
                    multiple-containers) printf 'db-one\ndb-two\n'; return 0 ;;
                    compose-failure) echo secret >&2; return 1 ;;
                    changed-service) [[ ! -e "$TEST_DIR/built" ]] || return 0 ;;
                    changed-identity) [[ ! -e "$TEST_DIR/built" ]] || { echo db-other; return 0; } ;;
                esac
                echo db-id
                ;;
            *) return 91 ;;
        esac
    elif [[ "$1" == exec ]]; then
        [[ "$CASE" != missing-tools ]] || return 1
    else
        return 92
    fi
}
bash() { echo backup >> "$TEST_DIR/events"; return 1; }
deploy staging "${SKIP:-false}" false
RUN
run_case() {
    export CASE="$1"
    : > "$TEST_DIR/events"
    rm -f "$TEST_DIR/built"
    printf '%s\n' 'DATABASE_URL=postgresql://fixture:secret@db:5432/events' > "$TEST_DIR/db.env"
    case "$CASE" in
        host-tools) printf 'DATABASE_URL=%s\n' "$HOST_TARGET" > "$TEST_DIR/db.env" ;;
        malformed-target) printf 'DATABASE_URL=https://secret.invalid\n' > "$TEST_DIR/empty.env" ;;
        multiline-target) printf "DATABASE_URL=\$'postgresql://fixture:secret@db/events\\nother'\n" > "$TEST_DIR/empty.env" ;;
    esac
    if bash "$TEST_DIR/run.sh" > "$TEST_DIR/output" 2>&1; then
        echo "FAIL: $CASE unexpectedly succeeded"; exit 1
    fi
    if rg -q secret "$TEST_DIR/output"; then echo "FAIL: $CASE leaked secret"; exit 1; fi
}
start=$SECONDS
for case_name in missing-env source-failure missing-target malformed-target multiline-target invalid-compose no-container multiple-containers compose-failure missing-tools; do
    run_case "$case_name"
    if rg -q 'source|prune|build|backup|migrated|slot|traffic' "$TEST_DIR/events"; then
        echo "FAIL: $CASE reached expensive/mutating phase"; exit 1
    fi
    case "$CASE" in
        source-failure|missing-target|malformed-target|multiline-target) rg -q 'Invalid migration database target' "$TEST_DIR/output" ;;
    esac
    echo "PASS: $CASE stops before build"
done
echo "PASS: invalid configuration paths finished in $((SECONDS-start))s without Docker build"
for target in 'postgresql://fixture:secret@localhost:5432/events' 'postgresql://fixture:secret@127.0.0.1/events' 'postgresql:///events?host=/tmp' 'postgresql://fixture:secret@db/events?hostaddr=127.0.0.1'; do
    HOST_TARGET="$target" run_case host-tools
    rg -q 'Host-target backup requires pg_dump' "$TEST_DIR/output"
    if rg -q 'build|backup|migrated|slot|traffic' "$TEST_DIR/events"; then echo 'FAIL: host target built without backup tools'; exit 1; fi
done
echo 'PASS: loopback/socket/query targets require host backup clients before build'
run_case valid
[[ "$(cat "$TEST_DIR/events")" == $'config\ndiscovery\nsource\nprune\nbuild\ndiscovery\nbackup' ]] || { cat "$TEST_DIR/output"; cat "$TEST_DIR/events"; exit 1; }
echo 'PASS: fresh service lookup and backup follow build and precede migrations'
for case_name in changed-service changed-identity changed-target; do
    run_case "$case_name"
    rg -q build "$TEST_DIR/events"
    if rg -q 'backup|migrated|slot|traffic' "$TEST_DIR/events"; then echo "FAIL: $CASE reached backup/mutation"; exit 1; fi
    echo "PASS: $CASE fails closed after build"
done
SKIP=true run_case missing-target
[[ "$(cat "$TEST_DIR/events")" == $'config\ndiscovery\nsource\nprune\nbuild\nslot' ]]
echo 'PASS: skip-migrations keeps Compose/service checks and skips backup requirements'
# The real Compose parser validates required interpolation without a daemon.
if docker compose version >/dev/null 2>&1; then
    mkdir -p "$TEST_DIR/fixture"
    cat > "$TEST_DIR/fixture/docker-compose.yml" <<'YAML'
services:
  togather-db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: ${PREFLIGHT_FIXTURE_PASSWORD:?required fixture password}
YAML
    : > "$TEST_DIR/fixture/empty.env"
    printf 'PREFLIGHT_FIXTURE_PASSWORD=fixture-secret\n' > "$TEST_DIR/fixture/valid.env"
    if env -u PREFLIGHT_FIXTURE_PASSWORD docker compose --env-file "$TEST_DIR/fixture/empty.env" -f "$TEST_DIR/fixture/docker-compose.yml" config --quiet > "$TEST_DIR/compose-output" 2>&1; then
        echo 'FAIL: real Compose accepted missing required interpolation'; exit 1
    fi
    env -u PREFLIGHT_FIXTURE_PASSWORD docker compose --env-file "$TEST_DIR/fixture/valid.env" -f "$TEST_DIR/fixture/docker-compose.yml" config --quiet
    cp "$TEST_DIR/fixture/docker-compose.yml" "$TEST_DIR/work/deploy/docker/docker-compose.yml"
    run_case real-invalid-compose
    [[ "$(cat "$TEST_DIR/events")" == config ]]
    echo 'PASS: real isolated Compose rejects interpolation before build and accepts selected env file'
else
    echo 'SKIP: real Compose parser fixture (Compose unavailable)'
fi
