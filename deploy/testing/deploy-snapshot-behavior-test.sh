#!/usr/bin/env bash
# Behavior tests use no live Docker daemon, database, migrations or traffic switch.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT
mkdir -p "$TEST_DIR/bin" "$TEST_DIR/work/internal/storage/postgres/migrations"
export TEST_DIR ROOT
REAL_PG_RESTORE=$(command -v pg_restore || true)
export REAL_PG_RESTORE
cat > "$TEST_DIR/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == compose ]]; then
    [[ "${CASE}" != no-container ]] || exit 0
    [[ "${CASE}" != compose-failure ]] || exit 1
    [[ "${CASE}" != multiple-containers ]] || { printf 'db-one\ndb-two\n'; exit 0; }
    echo db-container-id
elif [[ "$1" == exec ]]; then
    shift
    [[ "${1:-}" != -i ]] || shift
    [[ "$1" == db-container-id ]] || exit 91
    shift
    case "$1" in
        sh)
            if [[ "$*" == *'command -v'* ]]; then
                [[ "$CASE" != missing-tools ]]
            else
                IFS= read -r target
                [[ "$target" == 'postgresql://fixture:secret@external-db:5432/target' ]] || exit 94
                [[ "$CASE" != empty-dump ]] || exit 0
                [[ "$CASE" != truncated-dump ]] || { printf 'custom archive'; exit 0; }
                printf 'custom archive payload'
                [[ "$CASE" != dump-failure ]]
            fi
            ;;
        pg_restore)
            [[ "$*" == 'pg_restore --exit-on-error --file=/dev/null' ]] || exit 98
            payload=$(cat)
            if [[ "$CASE" == real-corrupt-archive ]]; then
                shift
                exec "$REAL_PG_RESTORE" "$@" <<< "$payload"
            fi
            [[ "$payload" == 'custom archive payload' ]] || exit 2
            [[ "$CASE" != corrupt-archive ]] || exit 3
            printf 'validated\n' >> "$TEST_DIR/events"
            ;;
        *) exit 92 ;;
    esac
else
    exit 93
fi
MOCK
chmod +x "$TEST_DIR/bin/docker"
export PATH="$TEST_DIR/bin:$PATH"
# Extract real orchestration functions without executing deploy.sh's main.
sed -n '/^create_db_snapshot()/,/^}/p; /^deploy()/,/^}/p' "$ROOT/deploy/scripts/deploy.sh" > "$TEST_DIR/functions.sh"
cat > "$TEST_DIR/run.sh" <<'RUN'
#!/usr/bin/env bash
set -euo pipefail
source "$TEST_DIR/functions.sh"
PROJECT_ROOT="$ROOT"
DOCKER_DIR="$ROOT/deploy/docker"
COMPOSE_CMD='docker compose'
DEPLOYED_BY=test
GIT_COMMIT=abc123
DEPLOYMENT_ID=test
SNAPSHOT_PATH=''
SNAPSHOT_DIR="$TEST_DIR/work/snapshots"
log() { printf '%s %s\n' "$1" "$2" >&2; }
discover_env_file() {
    case "$CASE" in
        missing-env) : ;;
        source-failure) echo "$TEST_DIR/bad.env" ;;
        missing-target) echo "$TEST_DIR/empty.env" ;;
        *) echo "$TEST_DIR/db.env" ;;
    esac
}
resolve_database_url() { :; }
validate_config() { :; }
validate_tool_versions() { :; }
validate_git_commit() { :; }
acquire_lock() { :; }
check_disk_space() { :; }
prune_docker_cache() { :; }
build_docker_image() { :; }
run_migrations() { echo migrated >> "$TEST_DIR/events"; return 78; }
bash() {
    case "$CASE" in
        malformed-evidence) printf '{"verified":true}\n' ;;
        checksum-mismatch) printf '{"verified":true,"verification":"pg_restore_full_read","format":"postgres_custom","snapshot_path":"%s/unverified.dump","size_bytes":22,"sha256":"%064d"}\n' "$TEST_DIR" 0 ;;
        missing-artifact) printf '{"verified":true,"verification":"pg_restore_full_read","format":"postgres_custom","snapshot_path":"/nonexistent/backup.dump","size_bytes":22,"sha256":"%064d"}\n' 0 ;;
        *) command bash "$@" ;;
    esac
}
deploy staging "${SKIP:-false}" false
RUN
printf '%s\n' 'DATABASE_URL=postgresql://fixture:secret@external-db:5432/target' > "$TEST_DIR/db.env"
printf 'return 1\n' > "$TEST_DIR/bad.env"
printf 'DATABASE_URL=\n' > "$TEST_DIR/empty.env"
printf 'custom archive payload' > "$TEST_DIR/unverified.dump"
# Failure cases must stop before any migration or traffic change.
failure_cases=(missing-env source-failure missing-target compose-failure multiple-containers checksum-mismatch no-container missing-tools dump-failure empty-dump truncated-dump corrupt-archive malformed-evidence missing-artifact)
if [[ -n "$REAL_PG_RESTORE" ]]; then
    failure_cases+=(real-corrupt-archive)
else
    echo 'SKIP: real pg_restore corruption check (client not installed); stubbed full-read checks still required'
fi
for case_name in "${failure_cases[@]}"; do
    : > "$TEST_DIR/events"
    export CASE="$case_name"
    if bash "$TEST_DIR/run.sh" > "$TEST_DIR/output" 2>&1; then
        echo "FAIL: $case_name accepted an unverifiable backup"; exit 1
    fi
    if rg -q secret "$TEST_DIR/output"; then
        echo "FAIL: $case_name leaked database credential"; exit 1
    fi
    if rg -q migrated "$TEST_DIR/events"; then
        echo "FAIL: $case_name reached migrations"; exit 1
    fi
    if [[ -d "$TEST_DIR/work/snapshots" ]] && find "$TEST_DIR/work/snapshots" -type f | rg -q .; then
        echo "FAIL: $case_name left partial backup artifacts"; exit 1
    fi
    echo "PASS: $case_name stops before migrations"
done
: > "$TEST_DIR/events"
export CASE=success
status=0
bash "$TEST_DIR/run.sh" > "$TEST_DIR/output" 2>&1 || status=$?
[[ "$status" == 1 ]] # deploy converts the deliberately failing migration into exit 1
[[ "$(cat "$TEST_DIR/events")" == $'validated\nmigrated' ]]
evidence=$(find "$TEST_DIR/work/snapshots" -name '*.json' -print -quit)
jq -e '.verified == true and .verification == "pg_restore_full_read" and .size_bytes > 0 and (.sha256 | length == 64)' "$evidence" > /dev/null
archive=$(jq -r .snapshot_path "$evidence")
[[ "$(stat -c %a "$archive")" == 600 && "$(stat -c %a "$evidence")" == 600 ]]
echo 'PASS: private fully read backup precedes migration and records evidence'
# Host-loopback targets use host clients without translating the address.
cat > "$TEST_DIR/bin/pg_dump" <<'HOSTDUMP'
#!/usr/bin/env bash
[[ "$1" == --dbname && "$2" == 'postgresql://fixture:secret@localhost:5432/target' ]] || exit 95
printf 'host-dump\n' >> "$TEST_DIR/events"
printf 'custom archive payload'
HOSTDUMP
cat > "$TEST_DIR/bin/pg_restore" <<'HOSTRESTORE'
#!/usr/bin/env bash
[[ "$*" == '--exit-on-error --file=/dev/null' ]] || exit 96
[[ "$(cat)" == 'custom archive payload' ]] || exit 97
printf 'host-validated\n' >> "$TEST_DIR/events"
HOSTRESTORE
chmod +x "$TEST_DIR/bin/pg_dump" "$TEST_DIR/bin/pg_restore"
printf '%s\n' 'DATABASE_URL=postgresql://fixture:secret@localhost:5432/target' > "$TEST_DIR/db.env"
: > "$TEST_DIR/events"
bash "$TEST_DIR/run.sh" > "$TEST_DIR/output" 2>&1 || true
[[ "$(cat "$TEST_DIR/events")" == $'host-dump\nhost-validated\nmigrated' ]]
if rg -q secret "$TEST_DIR/output"; then echo 'FAIL: leaked connection secret'; exit 1; fi
echo 'PASS: loopback target uses exact host connection before migration'
# --skip-migrations intentionally bypasses this gate; stop at get_inactive_slot.
printf '\nget_inactive_slot() { exit 79; }\n' >> "$TEST_DIR/functions.sh"
: > "$TEST_DIR/events"
SKIP=true bash "$TEST_DIR/run.sh" > "$TEST_DIR/output" 2>&1 || true
[[ ! -s "$TEST_DIR/events" ]]
rg -q 'Skipping database snapshot' "$TEST_DIR/output"
echo 'PASS: explicit no-migration path does not create backup or migrate'
