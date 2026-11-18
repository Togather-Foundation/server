#!/usr/bin/env bash
# Exercise the rollback entry point with isolated state, Caddy routing and HTTP.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
project="$fixture/project"
scripts="$project/deploy/scripts"
state="$fixture/state.json"
history="$fixture/history"
mkdir -p "$scripts" "$project/deploy/config" "$fixture/bin" "$fixture/home" "$fixture/tmp"

# Redirect only the live Caddyfile and shared deployment lock in the fixture copy.
# Use real slot selection, locking, health polling, traffic switch and state writes.
# shellcheck disable=SC2016 # Expand FIXTURE in the deploy subprocess.
sed -e 's|/etc/caddy/Caddyfile|${FIXTURE}/Caddyfile|g' \
    -e 's|/tmp/togather-deploy-|${FIXTURE}/togather-deploy-|g' \
    "${DEPLOY_SCRIPT_UNDER_TEST:-$root/deploy/scripts/deploy.sh}" > "$scripts/deploy.sh"
cp "$root/deploy/scripts/wait-for-health.sh" "$scripts"
cat > "$fixture/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -eu
case "${!#}" in
    *blue|*:bbbbbbb) tag=bbbbbbb ;;
    *green|*:aaaaaaa) tag=aaaaaaa ;;
    *) exit 99 ;;
esac
# A failed deploy can replace the inactive container without publishing new
# deployment state. Expose its new image only to the inspection under lock.
if [[ "$1" == inspect && "$CASE" == replaced-container ]] &&
    jq -e '.lock.locked' "$TOGATHER_STATE_FILE" >/dev/null; then
    tag=bad
fi
case "$1 $2" in
    'image inspect')
        if [[ "$CASE" == missing-image ]]; then echo 'No such image' >&2; exit 1; fi
        # Complete a competing rollback after target resolution, before this
        # process takes the lock. Hold its resulting state for the assertion.
        if [[ "$CASE" == competing && ! -f "$FIXTURE/competing-state" ]]; then
            jq '.current_deployment as $old | .current_deployment = .previous_deployment |
                .previous_deployment = $old' "$TOGATHER_STATE_FILE" > "$FIXTURE/competing-state"
            cp "$FIXTURE/competing-state" "$TOGATHER_STATE_FILE"
        fi
        echo "id-$tag"
        ;;
    'inspect --format={{.State.Status}}')
        [[ "$CASE" != no-compose ]] || exit 1
        echo exited ;;
    'inspect --format={{.Config.Image}}')
        [[ "$CASE" != wrong-image ]] || tag=bad
        echo "togather-server:$tag"
        ;;
    'inspect --format={{.Image}}')
        [[ "$CASE" != wrong-image ]] || tag=bad
        echo "id-$tag"
        ;;
    'start togather-server-'*|'stop togather-server-'*) echo "$*" >> "$FIXTURE/events" ;;
    *) exit 99 ;;
esac
DOCKER
cat > "$fixture/bin/systemctl" <<'SYSTEMCTL'
#!/usr/bin/env bash
[[ "$*" == 'is-active --quiet caddy' ]]
SYSTEMCTL
cat > "$fixture/bin/sudo" <<'SUDO'
#!/usr/bin/env bash
set -eu
case "$1" in
    caddy)
        [[ "$3" == --config && "$4" == "$FIXTURE/Caddyfile" ]] || exit 99
        case "$2" in
            validate) exit 0 ;;
            reload) cp "$4" "$FIXTURE/loaded"; echo reload >> "$FIXTURE/events" ;;
            *) exit 99 ;;
        esac ;;
    cp|sed|mkdir)
        [[ "${!#}" == "$FIXTURE"/* ]] || exit 99
        exec "$@" ;;
    chown) exit 0 ;;
    *) exit 99 ;;
esac
SUDO
cat > "$fixture/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -eu
url=${!#}
echo "$url" >> "$FIXTURE/events"
case "$url" in
    http://localhost:808[12]/health) printf '{"status":"healthy"}\n200\n' ;;
    "https://$EXPECTED_DOMAIN/version")
        if grep -q 'reverse_proxy localhost:8082' "$FIXTURE/loaded"; then
            echo '{"git_commit":"aaaaaaa"}'
        else
            echo '{"git_commit":"bbbbbbb"}'
        fi ;;
    *) exit 22 ;;
esac
CURL
# Keep Compose detection and retry delays independent of the runner's tools.
printf '#!/bin/sh\nexit 1\n' > "$fixture/bin/docker-compose"
printf '#!/bin/sh\nexit 0\n' > "$fixture/bin/sleep"
chmod +x "$fixture/bin/"*

fail() {
    echo "FAIL: $*" >&2
    cat "$fixture/out" "$fixture/events" >&2
    exit 1
}
setup() {
    local env=$1
    mkdir -p "$history/$env"
    jq -n --arg env "$env" '
        def deployment($slot; $version):
          {id:$version, version:$version, git_commit:$version, deployed_at:"fixture",
           deployed_by:"fixture", active_slot:$slot, health_status:"healthy"};
        {environment:$env, lock:{locked:false}, current_deployment:deployment("blue"; "bbbbbbb"),
         previous_deployment:deployment("green"; "aaaaaaa")}' > "$state"
    for tag in aaaaaaa bbbbbbb; do
        jq -n --arg tag "$tag" '{version:$tag, docker_image:("togather-server:"+$tag)}' > "$history/$env/$tag.json"
    done
    ln -s bbbbbbb.json "$history/$env/current.json"
    ln -s aaaaaaa.json "$history/$env/previous.json"
    cat > "$fixture/Caddyfile" <<'CADDY'
# BLUE_GREEN_SLOT_START
reverse_proxy localhost:8081 {
    header_down X-Togather-Slot "blue"
}
# BLUE_GREEN_SLOT_END
CADDY
    cp "$fixture/Caddyfile" "$fixture/loaded"
}
run() {
    : > "$fixture/events"
    env -i PATH="$fixture/bin:/usr/bin:/bin" HOME="$fixture/home" TMPDIR="$fixture/tmp" USER=test \
        FIXTURE="$fixture" CASE="${test_case:-normal}" EXPECTED_DOMAIN="$expected_domain" \
        HEALTH_WAIT_TIMEOUT="${health_timeout:-60}" \
        TOGATHER_STATE_FILE="$state" TOGATHER_DEPLOYMENT_HISTORY_DIR="$history" \
        bash "$scripts/deploy.sh" rollback "$@" > "$fixture/out" 2>&1
}
plan() {
    local env=$1
    cp "$state" "$fixture/before"
    run "$env" --dry-run || fail "$env dry run"
    grep -Fq "https://$expected_domain/version" "$fixture/out" || fail "$env plan omitted its verification URL"
    [[ ! -s "$fixture/events" ]] || fail "$env dry run touched services"
    cmp -s "$fixture/before" "$state" || fail "$env dry run changed state"
}
flip() {
    local env=$1 slot=$2 port=$3 tag=$4 old=$5
    local old_record
    old_record=$(readlink "$history/$env/current.json")
    run "$env" || fail "$env rollback to $slot"
    local events
    events=$(printf '%s\n' "start togather-server-$slot" "http://localhost:$port/health" reload \
        "https://$expected_domain/version" "stop togather-server-$old")
    [[ "$(cat "$fixture/events")" == "$events" ]] || fail "unexpected $env operation order or endpoint"
    grep -Fq "header_down X-Togather-Slot \"$slot\"" "$fixture/loaded" || fail "wrong loaded slot header"
    grep -Fq "reverse_proxy localhost:$port" "$fixture/loaded" || fail "wrong loaded port"
    jq -e --arg tag "$tag" --arg slot "$slot" \
        '.current_deployment.version == $tag and .current_deployment.active_slot == $slot and (.lock.locked == false)' \
        "$state" >/dev/null || fail "incorrect $env deployment state"
    jq -e --arg tag "$tag" '.version == $tag' "$history/$env/current.json" >/dev/null || fail "incorrect history record"
    [[ "$(readlink "$history/$env/previous.json")" == "$old_record" ]] || fail 'previous history link failed to advance'
}

setup production
expected_domain=prod.fixture.invalid
printf 'NODE_DOMAIN=%s\n' "$expected_domain" > "$project/.deploy.conf.production"
printf 'NODE_DOMAIN=stage.fixture.invalid\n' > "$project/.deploy.conf.staging"
cp "$state" "$fixture/before"
for test_case in missing-image wrong-image; do
    if run production; then fail "$test_case passed preflight"; fi
    [[ ! -s "$fixture/events" ]] || fail "$test_case touched services"
    cmp -s "$fixture/before" "$state" || fail "$test_case changed state"
done
for test_case in no-compose unhealthy; do
    health_timeout=0
    if run production; then fail "$test_case rollback succeeded"; fi
    cmp -s "$fixture/before" "$state" || fail "$test_case changed deployment state"
    if grep -Eq 'reload|https://|stop togather-server' "$fixture/events"; then fail "$test_case switched traffic"; fi
    case "$test_case" in
        no-compose) grep -Fq 'No docker compose' "$fixture/out" || fail 'missing Compose diagnosis' ;;
        unhealthy) grep -Fq 'Health check failed for the green slot' "$fixture/out" || fail 'missing health diagnosis' ;;
    esac
done
unset health_timeout
unset test_case
flip production green 8082 aaaaaaa blue
plan production
flip production blue 8081 bbbbbbb green
echo 'PASS: production plan and both Caddy cutovers use its configured domain; image, Compose and health failures preserve the active deployment'

rm "$project/.deploy.conf.production"
expected_domain=production.toronto.togather.foundation
plan production
grep -E 'WARN.*\.deploy\.conf\.production' "$fixture/out" >/dev/null || fail 'missing production configuration warning'
echo 'PASS: absent production configuration warns and shows the fallback URL'

setup staging
expected_domain=stage.fixture.invalid
plan staging
flip staging green 8082 aaaaaaa blue
rm "$project/.deploy.conf.staging"
expected_domain=staging.toronto.togather.foundation
plan staging
echo 'PASS: staging selects its own configuration and retains its fallback'

test_case=competing
cp -a "$history/staging" "$fixture/before-history"
if run staging; then fail 'concurrent rollback reused a stale target'; fi
cmp -s "$state" "$fixture/competing-state" || fail 'concurrent rollback overwrote committed state'
diff -r --no-dereference "$fixture/before-history" "$history/staging" || fail 'concurrent rollback overwrote history'
[[ ! -s "$fixture/events" ]] || fail 'concurrent rollback touched containers or Caddy'
echo 'PASS: a concurrent rollback invalidates the target before any slot or traffic change'

test_case=replaced-container
if run staging; then fail 'rollback accepted a container replaced during preflight'; fi
grep -Fq 'different build' "$fixture/out" || fail 'missing replaced container diagnosis'
cmp -s "$state" "$fixture/competing-state" || fail 'replaced container check changed state'
diff -r --no-dereference "$fixture/before-history" "$history/staging" || fail 'replaced container check changed history'
[[ ! -s "$fixture/events" ]] || fail 'rollback started the replaced container or changed traffic'
echo 'PASS: rollback rechecks the inactive container image under lock'

# Run the public Make recipes with their default shell and capture both SSH
# arguments. Config files and the Makefile all live in the same fixture checkout.
cp "${MAKEFILE_UNDER_TEST:-$root/Makefile}" "$project/Makefile"
cat > "$fixture/bin/ssh" <<'SSH'
#!/usr/bin/env bash
set -eu
printf '%s\n' "$#" "$@" >> "$FIXTURE/events"
exit "$SSH_STATUS"
SSH
chmod +x "$fixture/bin/ssh"
run_make() {
    : > "$fixture/events"
    env -i PATH="$fixture/bin:/usr/bin:/bin" HOME="$fixture/home" FIXTURE="$fixture" \
        SSH_STATUS="${ssh_status:-0}" make --no-print-directory -C "$project" "$@" \
        > "$fixture/out" 2>&1
}
for env in staging production; do
    target="rollback-$env"
    conf="$project/.deploy.conf.$env"
    remote_command="cd /opt/togather/src && ./deploy/scripts/deploy.sh rollback $env"
    if run_make "$target" </dev/null; then fail "$target accepted missing config"; fi
    grep -Fq ".deploy.conf.$env not found" "$fixture/out" || fail "$target omitted missing config"
    [[ ! -s "$fixture/events" ]] || fail "$target used SSH without config"

    printf 'SSH_USER=%s-user\nSSH_HOST=%s.fixture.invalid\nNODE_DOMAIN=node-%s.fixture.invalid\n' \
        "$env" "$env" "$env" > "$conf"
    run_make -n "$target" EXTRA_ARGS=--dry-run || fail "$target Make dry run failed"
    grep -Fq "$remote_command --dry-run" "$fixture/out" || fail "$target preview has wrong remote command"
    [[ ! -s "$fixture/events" ]] || fail "$target preview ran SSH"

    for extra in '' --dry-run; do
        args=("$target")
        [[ -z "$extra" ]] || args+=("EXTRA_ARGS=$extra")
        run_make "${args[@]}" <<< y || fail "$target failed with $extra"
        expected=$(printf '%s\n' 2 "$env-user@$env.fixture.invalid" "$remote_command${extra:+ $extra}")
        [[ "$(cat "$fixture/events")" == "$expected" ]] || fail "$target dispatched incorrectly with $extra"
        if [[ "$env" == production ]]; then
            grep -Fq 'node-production.fixture.invalid' "$fixture/out" || fail 'production confirmation omitted configured domain'
        fi
    done

    ssh_status=42
    if run_make "$target" <<< y; then fail "$target suppressed SSH failure"; fi
    [[ -s "$fixture/events" ]] || fail "$target failed before SSH"
    unset ssh_status
    echo "PASS: $target selects its configured SSH host, runs the checkout script, forwards --dry-run and propagates SSH failure"
done
run_make rollback-production <<< n || fail 'production decline failed'
[[ ! -s "$fixture/events" ]] || fail 'declining production dispatched SSH'
run_make rollback-production </dev/null || fail 'noninteractive production decline failed'
[[ ! -s "$fixture/events" ]] || fail 'production dispatched SSH without confirmation'
echo 'PASS: declining or omitting production confirmation leaves SSH idle'
