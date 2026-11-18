#!/usr/bin/env bash
# Exercise the verifier's CLI, SSH transport and streamed remote collector.
# No fixture command can contact a real host, start a container or restore data.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
project="$tmp/checkout"
home_dir="$tmp/home"
history="$tmp/history/staging"
scripts="$project/deploy/scripts"
state="$project/deploy/config/deployment-state.json"
archive="$home_dir/.togather/snapshots/pre-deploy-fixture.dump"
sync_log="$home_dir/.togather/logs/deployments/staging_fixture.log"
keys="$project/.agent-keys/staging"
sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
prev=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
current_id=dep_0000000000000001_aaaaaaaaaaaaaaaaaaaaaaaa
previous_id=dep_0000000000000000_bbbbbbbbbbbbbbbbbbbbbbbb
secret=UNSAFE_PRIVATE_FIXTURE_CONTENT
mkdir -p "$tmp/bin" "$scripts" "$project/deploy/config" "$project/deploy/testing" \
    "$history" "$(dirname "$archive")" "$(dirname "$sync_log")" "$(dirname "$keys")"
cp "$root/deploy/scripts/verify-deployment.sh" "$scripts/"
# Map the host-wide deployment lock into the simulated remote checkout. Both
# SSH calls still execute the verifier source and examine a real directory.
sed 's|/tmp/togather-deploy-|deploy/config/togather-deploy-|g' \
    "$root/deploy/scripts/verify_deployment.py" > "$scripts/verify_deployment.py"
cp "$root/deploy/testing/fixtures/verification-probes.py" "$tmp/probe"
chmod +x "$tmp/probe"
for stub in curl docker ssh pg_restore pg_dump psql migrate sudo docker-compose; do ln -s "$tmp/probe" "$tmp/bin/$stub"; done
ln -s "$tmp/probe" "$scripts/deploy.sh"
ln -s "$tmp/probe" "$scripts/performance-test.sh"
ln -s "$tmp/probe" "$project/deploy/testing/smoke-tests.sh"
cat > "$project/.deploy.conf.staging" <<'CONF'
NODE_DOMAIN=staging.fixture.invalid
SSH_HOST=host.fixture.invalid
SSH_USER=fixture
CONF
printf 'TOGATHER_ADMIN_API_KEY=%s\n' "$secret" > "$tmp/good-key"
chmod 600 "$tmp/good-key"

record() {
    local id=$1 commit=$2 slot=$3
    jq -n --arg id "$id" --arg sha "$commit" --arg version "${commit:0:7}" --arg slot "$slot" \
        --arg archive "$archive" --arg log "$sync_log" \
        '{deployment_id:$id,git_commit:$sha,version:$version,active_slot:$slot,status:"active",
          environment:"staging",docker_image:("togather-server:"+$version),snapshot_path:$archive,
          log_file:$log,compose_project:"togather-staging",deployed_at:"2026-01-01T00:00:00Z",deployed_by:"fixture"}'
}
record "$current_id" "$sha" blue > "$history/$current_id.json"
record "$previous_id" "$prev" green > "$history/$previous_id.json"
ln -s "$current_id.json" "$history/current.json"
ln -s "$previous_id.json" "$history/previous.json"
cp "$history/$current_id.json" "$tmp/good-history"
jq -n --slurpfile current "$history/current.json" --slurpfile previous "$history/previous.json" '
    def state: {id:.deployment_id, git_commit,version,active_slot,status,environment,deployed_at,deployed_by,health_status:"healthy"};
    {environment:"staging", lock:{locked:false}, current_deployment:($current[0]|state),
     previous_deployment:($previous[0]|state)}' > "$tmp/good-state"
printf 'PGDMP%s' "$secret" > "$tmp/good-archive"
sum=$(sha256sum "$tmp/good-archive")
jq -n --arg sha "$sha" --arg id "$current_id" --arg path "$archive" \
    --argjson size "$(wc -c < "$tmp/good-archive")" --arg checksum "${sum%% *}" \
    '{snapshot_path:$path,size_bytes:$size,sha256:$checksum,verified:true,verification:"pg_restore_full_read",
      format:"postgres_custom",client_mode:"container",reason:"pre-deploy-staging",git_commit:$sha,deployment_id:$id}' > "$tmp/good-manifest"
# deploy.sh log() interpolates the CLI JSON into the outer message without escaping it.
sync_fixture() {
    local warnings=$1
    cat > "$sync_log" <<LOG
{"timestamp":"2026-01-01T00:00:00Z","level":"INFO","deployment_id":"$current_id","message":"  {"sources_found":5,"total":5,"created":1,"updated":4,"warnings":$warnings,"errors":0}"}
{"timestamp":"2026-01-01T00:00:00Z","level":"SUCCESS","deployment_id":"$current_id","message":"Scraper source configs synced successfully (5 sources: 1 created, 4 updated)"}
LOG
}

check_names='deployment_state deployment_history containers direct_identity public_identity migrations backup source_sync source_audit rollback_preview smoke admin_auth load_test restore_rehearsal real_rollback final_identity'
expected_keys=$(jq -cn --arg keys "$check_names" '$keys | split(" ") | sort')
fail() {
    echo "FAIL: $case_name: $*" >&2
    # Avoid printing untrusted fixture data; show only failing check names for diagnostics.
    jq -rc '[.checks | to_entries[] | select(.value.status=="fail") | .key]' "$tmp/result" >&2 2>/dev/null || true
    exit 1
}
verify() {
    local case_name=$1 failed_check=${2:-} status=0
    local args=(staging --expected-commit "$sha")
    cp "$tmp/good-state" "$state"
    cp "$tmp/good-history" "$history/$current_id.json"
    cp "$tmp/good-archive" "$archive"
    cp "$tmp/good-manifest" "$archive.json"
    cp "$tmp/good-key" "$keys"
    if [[ -d "$project/deploy/config/togather-deploy-staging.lock" ]]; then
        rmdir "$project/deploy/config/togather-deploy-staging.lock"
    fi
    chmod 600 "$archive" "$archive.json" "$keys"
    sync_fixture 0
    case "$case_name" in
        wrong-state) jq --arg sha "$prev" '.current_deployment.git_commit=$sha' "$tmp/good-state" > "$state" ;;
        locked) jq '.lock.locked=true' "$tmp/good-state" > "$state" ;;
        directory-lock) mkdir "$project/deploy/config/togather-deploy-staging.lock" ;;
        history-sha) jq --arg sha "$prev" '.git_commit=$sha' "$tmp/good-history" > "$history/$current_id.json" ;;
        archive-missing) rm "$archive" ;;
        archive-hash) printf '%s' "PGDMP$(printf "%0${#secret}d" 0)" > "$archive" ;;
        archive-ref) jq --arg sha "$prev" '.git_commit=$sha' "$tmp/good-manifest" > "$archive.json" ;;
        archive-id) jq --arg id "$previous_id" '.deployment_id=$id' "$tmp/good-manifest" > "$archive.json" ;;
        archive-verification) jq '.verified=false' "$tmp/good-manifest" > "$archive.json" ;;
        sync-warnings) sync_fixture 1 ;;
        sync-missing) : > "$sync_log" ;;
        no-key) rm "$keys" ;;
        load|load-failure) args+=(--load-test) ;;
    esac

    : > "$tmp/events"
    env -i PATH="$tmp/bin:/usr/bin:/bin" HOME="$home_dir" VERIFY_CASE="$case_name" \
        VERIFY_REMOTE_ROOT="$project" TOGATHER_DEPLOYMENT_HISTORY_DIR="$tmp/history" \
        PROBE_EVENTS="$tmp/events" SMOKE_WRITE_TESTS=true ALLOW_DESTRUCTIVE=true \
        bash "$scripts/verify-deployment.sh" "${args[@]}" > "$tmp/result" 2> "$tmp/summary" || status=$?
    grep -Fq "$secret" "$tmp/result" "$tmp/summary" && fail 'leaked private command output, token or archive contents'
    grep -q '^DENIED ' "$tmp/events" && fail 'unexpected host command, network request or write operation'
    [[ -s "$tmp/summary" ]] || fail 'missing human summary'
    jq -es --arg sha "$sha" --argjson keys "$expected_keys" '
      length == 1 and (.[0] | .schema_version==1 and (.observed_at|type)=="string" and
        .environment=="staging" and .expected_commit==$sha and (.checks|keys)==$keys
        and all(.checks[]; .status=="pass" or .status=="fail" or .status=="skip")
        and .checks.restore_rehearsal.status=="skip" and .checks.real_rollback.status=="skip")' \
        "$tmp/result" >/dev/null || fail 'expected one complete JSON report'

    if [[ -n "$failed_check" ]]; then
        [[ $status -ne 0 ]] || fail 'verification falsely succeeded'
        if [[ "$failed_check" == transport ]]; then
            jq -e '.status=="fail" and any(.checks[]; .status=="fail" and (.reason|strings|length>0))' \
                "$tmp/result" >/dev/null || fail 'missing structured transport failure'
        else
            jq -e --arg check "$failed_check" '.status=="fail" and .checks[$check].status=="fail" and
                (.checks[$check].reason|strings|length>0)' "$tmp/result" >/dev/null || fail "missing failure reason for $failed_check"
        fi
    else
        local admin=pass load=skip
        [[ "$case_name" != no-key ]] || admin=skip
        [[ "$case_name" != load ]] || load=pass
        [[ $status -eq 0 ]] || fail 'valid deployment rejected'
        jq -e --arg admin "$admin" --arg load "$load" '.status=="pass" and .checks.admin_auth.status==$admin and
          .checks.load_test.status==$load and all(.checks | del(.real_rollback,.restore_rehearsal,.admin_auth,.load_test)[]; .status=="pass")' \
          "$tmp/result" >/dev/null || fail 'wrong required or optional check results'
        for step in collector audit preview smoke; do grep -qx "$step" "$tmp/events" || fail "omitted $step"; done
        cmp -s "$state" "$tmp/good-state" || fail 'verifier changed deployment state'
    fi
    case "$case_name" in
        smoke-failure)
            jq -e '.checks.admin_auth.status=="skip" and (.checks.admin_auth.reason|strings|length>0)' \
                "$tmp/result" >/dev/null || fail 'auth was never reached but reported an attempted result' ;;
        auth-failure)
            jq -e '.checks.smoke.status=="fail"' "$tmp/result" >/dev/null || fail 'auth rejection passed smoke' ;;
    esac
    [[ "$(grep -cx smoke "$tmp/events" || true)" -le 1 ]] || fail 'ran the smoke credential probe more than once'
    if [[ "$case_name" == load || "$case_name" == load-failure ]]; then
        grep -qx load "$tmp/events" || fail 'requested load probe did not run'
    else
        grep -qx load "$tmp/events" && fail 'ran load without opt-in'
    fi
    echo "PASS: $case_name"
}

verify healthy
verify no-key
verify load
verify wrong-state deployment_state
verify locked deployment_state
verify directory-lock deployment_state
verify history-sha deployment_history
verify wrong-sha public_identity
verify wrong-direct direct_identity
verify wrong-header public_identity
verify old-writer containers
verify unhealthy containers
verify missing-image containers
verify missing-container containers
verify dirty migrations
verify direct-health-sha migrations
verify direct-health-slot migrations
verify public-health-sha public_identity
verify public-health-slot public_identity
verify archive-missing backup
verify archive-hash backup
verify archive-ref backup
verify archive-id backup
verify archive-verification backup
verify sync-missing source_sync
verify sync-warnings source_sync
verify source-audit source_audit
verify audit-counts source_audit
verify preview-write rollback_preview
verify wrong-preview rollback_preview
verify late-change final_identity
verify late-lock final_identity
verify http-failure public_identity
verify ssh-failure transport
verify smoke-failure smoke
verify auth-failure admin_auth
verify auth-unverified admin_auth
verify load-failure load_test
