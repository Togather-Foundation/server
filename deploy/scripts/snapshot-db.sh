#!/usr/bin/env bash
# Create a private custom-format backup using an explicitly selected DB container.
# No restore is executed: pg_restore reads every archive entry into /dev/null.
set -euo pipefail
umask 077
container=''
output_dir=''
reason='manual'
while [[ $# -gt 0 ]]; do
    case "$1" in
        --container) container="${2:?--container requires an ID}"; shift 2 ;;
        --output-dir) output_dir="${2:?--output-dir requires a directory}"; shift 2 ;;
        --reason) reason="${2:?--reason requires a label}"; shift 2 ;;
        --help)
            echo 'Usage: snapshot-db.sh --container <DB container ID> --output-dir <private directory> [--reason <label>]'
            exit 0 ;;
        *) echo 'Unsupported argument; use --help (legacy flags are no longer accepted)' >&2; exit 1 ;;
    esac
done
[[ -n "$container" && -n "$output_dir" && "$reason" =~ ^[a-zA-Z0-9_-]+$ ]] || {
    echo 'Explicit database container, output directory and safe reason label required' >&2; exit 1;
}
for tool in docker jq sha256sum mktemp wc; do
    command -v "$tool" > /dev/null || { echo "Required backup tool unavailable: $tool" >&2; exit 1; }
done
# Use host clients for loopback/socket namespaces; container clients for shared
# network URLs. Never translate localhost or guess a published host port.
[[ "${DATABASE_URL:-}" == postgres://* || "${DATABASE_URL:-}" == postgresql://* ]] || {
    echo 'A PostgreSQL DATABASE_URL matching the mutation target is required' >&2; exit 1;
}
case "$DATABASE_URL" in
    *$'\n'*) echo 'Multiline database URLs are unsupported' >&2; exit 1 ;;
esac
authority="${DATABASE_URL#*://}"
authority="${authority%%/*}"
authority="${authority##*@}"
backup_mode=container
case "${authority,,}" in
    ''|0.*|localhost|localhost:*|*.localhost|*.localhost:*|localhost.localdomain*|127.*|\[*|*%*|*,*)
        backup_mode=host ;;
esac
case "$DATABASE_URL" in
    *\?host=*|*\&host=*|*\?hostaddr=*|*\&hostaddr=*) backup_mode=host ;;
esac
if [[ "$backup_mode" == host ]]; then
    for tool in pg_dump pg_restore; do
        command -v "$tool" > /dev/null || { echo "Host-target backup requires $tool on the deployment host" >&2; exit 1; }
    done
else
    docker exec "$container" sh -c 'command -v pg_dump >/dev/null && command -v pg_restore >/dev/null' > /dev/null 2>&1 || {
        echo 'Database container lacks backup tools' >&2; exit 1;
    }
fi
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)
archive=$(mktemp "$output_dir/${reason}-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX.dump")
evidence="${archive}.json"
complete=false
cleanup() {
    if [[ "$complete" != true ]]; then
        rm -f -- "$archive" "$evidence"
    fi
}
trap cleanup EXIT
# Connection secrets are never printed in output/evidence. Container clients
# receive the exact migration URL over stdin, outside host docker arguments.
if [[ "$backup_mode" == host ]]; then
    if ! pg_dump --dbname "$DATABASE_URL" -Fc > "$archive" 2>/dev/null; then
        echo 'Host database dump failed; deployment must stop' >&2; exit 1
    fi
else
    if ! docker exec -i "$container" sh -c 'IFS= read -r connection; exec pg_dump --dbname "$connection" -Fc' \
        <<< "$DATABASE_URL" > "$archive" 2>/dev/null; then
        echo 'Database dump failed; deployment must stop' >&2; exit 1
    fi
fi
[[ -s "$archive" ]] || { echo 'Database dump is empty' >&2; exit 1; }
if [[ "$backup_mode" == host ]]; then
    if ! pg_restore --exit-on-error --file=/dev/null < "$archive" > /dev/null 2>&1; then
        echo 'Full host archive read failed; deployment must stop' >&2; exit 1
    fi
else
    if ! docker exec -i "$container" pg_restore --exit-on-error --file=/dev/null < "$archive" > /dev/null 2>&1; then
        echo 'Full archive read failed; deployment must stop' >&2; exit 1
    fi
fi
size_bytes=$(wc -c < "$archive")
checksum=$(sha256sum "$archive")
checksum="${checksum%% *}"
jq -n --arg path "$archive" --arg sha "$checksum" --arg reason "$reason" \
    --arg commit "${GIT_COMMIT:-}" --arg deployment "${DEPLOYMENT_ID:-}" \
    --arg mode "$backup_mode" --argjson size "$size_bytes" \
    '{snapshot_path:$path,size_bytes:$size,sha256:$sha,verified:true,verification:"pg_restore_full_read",format:"postgres_custom",client_mode:$mode,reason:$reason,git_commit:$commit,deployment_id:$deployment}' > "$evidence"
complete=true
cat "$evidence"
