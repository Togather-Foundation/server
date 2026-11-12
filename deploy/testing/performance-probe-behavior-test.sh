#!/usr/bin/env bash
# Isolated wrapper contract: no live server or real load-test binary.
set -euo pipefail
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
probe_tmp=$(mktemp -d)
trap 'rm -rf "$probe_tmp"' EXIT
mkdir -p "$probe_tmp/deploy/scripts" "$probe_tmp/bin" "$probe_tmp/fakebin"
cp "$TEST_DIR/../scripts/performance-test.sh" "$probe_tmp/deploy/scripts/"
cat > "$probe_tmp/fakebin/curl" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
cat > "$probe_tmp/bin/loadtest" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$@" > "$PROBE_ARGS"
exit "${PROBE_EXIT:-0}"
STUB
chmod +x "$probe_tmp/fakebin/curl" "$probe_tmp/bin/loadtest"
export PATH="$probe_tmp/fakebin:$PATH" PROBE_ARGS="$probe_tmp/args" PERF_AGENT_API_KEY="private-probe-key"
if ! bash "$probe_tmp/deploy/scripts/performance-test.sh" > "$probe_tmp/output" 2>&1; then
    echo 'FAIL: success not propagated';exit 1
fi
python3 - "$PROBE_ARGS" <<'PY'
import sys
args=open(sys.argv[1]).read().splitlines()
assert args[args.index('--read-ratio')+1]=='1', args
assert args[args.index('--max-error-rate')+1]=='0', args
PY
if rg -q "$PERF_AGENT_API_KEY" "$probe_tmp/output"; then echo 'FAIL: leaked key';exit 1;fi
bash "$probe_tmp/deploy/scripts/performance-test.sh" --profile medium --rps 5 --duration 1m > "$probe_tmp/output" 2>&1
python3 - "$PROBE_ARGS" <<'PY'
import sys
args=open(sys.argv[1]).read().splitlines()
assert args[args.index('--profile')+1]=='medium', args
assert '--rps' in args and args[args.index('--rps')+1]=='5', args
assert '--duration' in args and args[args.index('--duration')+1]=='1m', args
PY
export PROBE_EXIT=7
if bash "$probe_tmp/deploy/scripts/performance-test.sh" > "$probe_tmp/output" 2>&1; then
    echo 'FAIL: failure hidden';exit 1
else
    status=$?
    [[ "$status" == 7 ]] || { echo "FAIL: exit changed: $status";exit 1; }
fi
if rg -q 'Load test completed|private-probe-key' "$probe_tmp/output"; then echo 'FAIL: success claim or credential leak';exit 1;fi
echo 'PASS: read-only defaults, strict budget, failure status, credential redaction'
