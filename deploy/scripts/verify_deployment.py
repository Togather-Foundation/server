"""Deployment verification entry point and read-only probes streamed over SSH."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys


SHA = r"[0-9a-f]{40}"
DIGEST = r"[0-9a-f]{64}"
SLOTS = {"blue": 8081, "green": 8082}
REMOTE_CHECKS = ("deployment_state", "deployment_history", "containers", "direct_identity",
                 "migrations", "backup", "source_sync", "source_audit", "rollback_preview")
STATE_FILE = Path("deploy/config/deployment-state.json")
PROBE_ERRORS = (OSError, ValueError, KeyError, TypeError, AttributeError, subprocess.SubprocessError)


class EvidenceError(ValueError):
    """Only verifier-authored codes can appear in failure reports."""


def require(condition, code="invalid_evidence"):
    if not condition:
        raise EvidenceError(code)


def matches(pattern, value):
    return isinstance(value, str) and re.fullmatch(pattern, value) is not None


def natural(value):
    return type(value) is int and value >= 0


def run(args, *, timeout=30, **kwargs):
    return subprocess.run(args, check=True, timeout=timeout, stderr=subprocess.DEVNULL,
                          stdout=subprocess.PIPE, **kwargs).stdout


def check(checks, name, probe):
    try:
        evidence = probe()
        checks[name] = {"status": "pass", "evidence": evidence}
        return evidence
    except EvidenceError as exc:
        checks[name] = {"status": "fail", "reason": str(exc)}
        return {}
    except PROBE_ERRORS:
        checks[name] = {"status": "fail", "reason": name + "_evidence_unavailable"}
        return {}


def skip(reason):
    return {"status": "skip", "reason": reason}


def read_json(path):
    with path.open() as stream:
        return json.load(stream)


def digest(path):
    hasher = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            hasher.update(block)
    return hasher.hexdigest()


def response(url):
    raw = run(["curl", "-q", "--silent", "--show-error", "--fail", "--max-time", "15",
               "--include", "--suppress-connect-headers", url])
    head, separator, body = raw.replace(b"\r\n", b"\n").partition(b"\n\n")
    require(separator and re.match(rb"HTTP/[0-9.]+ 200(?: |\n|$)", head))
    return head, json.loads(body)


def version(url, sha, slot=None):
    head, body = response(url + "/version")
    require(body["git_commit"] == sha, "served_commit_mismatch")
    evidence = {"git_commit": sha}
    if slot is not None:
        require(slot in SLOTS)
        headers = re.findall(rb"(?im)^x-togather-slot:[ \t]*([^\n]*?)\s*$", head)
        require(headers == [slot.encode()], "served_slot_mismatch")
        evidence["slot"] = slot
    return evidence


def health(url, sha, slot):
    _, body = response(url + "/health")
    require(body["slot"] == slot and body["git_commit"] == sha, "health_identity_mismatch")
    require(body["status"] in ("healthy", "degraded"), "service_unhealthy")
    migration = body["checks"]["migrations"]
    details = migration["details"]
    require(details["dirty"] is False, "migration_dirty")
    require(migration["status"] == "pass" and natural(details["version"]) and
            details["version"] > 0, "migration_version_unverified")
    return {"health": body["status"], "version": details["version"], "dirty": False}


def require_unlocked(env):
    # deploy.sh acquires this directory before it records the JSON lock.
    try:
        Path(f"/tmp/togather-deploy-{env}.lock").lstat()
    except FileNotFoundError:
        return
    raise EvidenceError("deployment_directory_locked")


def final_state(env):
    state_hash = digest(STATE_FILE)
    require_unlocked(env)
    return {"state_sha256": state_hash}


def deployment_state(env, sha):
    require_unlocked(env)
    raw = STATE_FILE.read_bytes()
    state = json.loads(raw)
    current, previous = state["current_deployment"], state["previous_deployment"]
    require(state["environment"] == env, "state_environment_mismatch")
    require(state["lock"]["locked"] is False, "deployment_locked")
    require(current["git_commit"] == sha, "recorded_commit_mismatch")
    for deployment in (current, previous):
        require(deployment["environment"] == env and deployment["active_slot"] in SLOTS and
                matches(r"[0-9a-f]{7,40}", deployment["version"]) and
                matches(SHA, deployment["git_commit"]) and
                deployment["git_commit"].startswith(deployment["version"]), "invalid_deployment_identity")
    require(current["active_slot"] != previous["active_slot"], "previous_slot_mismatch")
    require(current["status"] == "active" and matches(r"[A-Za-z0-9_-]{1,100}", current["id"]),
            "invalid_current_deployment")
    return {"id": current["id"], "git_commit": sha, "version": current["version"],
            "active_slot": current["active_slot"], "previous_slot": previous["active_slot"],
            "previous_commit": previous["git_commit"], "previous_version": previous["version"],
            "state_sha256": hashlib.sha256(raw).hexdigest()}


def history(env, state):
    base = Path(os.getenv("TOGATHER_DEPLOYMENT_HISTORY_DIR", "/var/lib/togather/deployments")) / env
    record = read_json(base / (state["id"] + ".json"))
    require(record["deployment_id"] == state["id"] and record["environment"] == env and
            record["status"] == "active" and record["git_commit"] == state["git_commit"] and
            record["active_slot"] == state["active_slot"] and record["version"] == state["version"] and
            record["docker_image"] == "togather-server:" + state["version"], "history_identity_mismatch")
    return record


def containers(state, record):
    evidence = {}
    template = ('{"image":{{json .Config.Image}},"image_id":{{json .Image}},'
                '"status":{{json .State.Status}},"health":{{if .State.Health}}'
                '{{json .State.Health.Status}}{{else}}null{{end}}}')
    for role, slot, image in (("active", state["active_slot"], record["docker_image"]),
                              ("previous", state["previous_slot"], "togather-server:" + state["previous_version"])):
        info = json.loads(run(["docker", "inspect", "--format", template, "togather-server-" + slot]))
        image_id = run(["docker", "image", "inspect", "--format", "{{.Id}}", image]).decode().strip()
        require(matches("sha256:" + DIGEST, image_id) and info["image_id"] == image_id and info["image"] == image,
                role + "_image_mismatch")
        if role == "active":
            require(info["status"] == "running" and info["health"] == "healthy", "active_container_unhealthy")
        else:
            require(info["status"] == "exited", "retired_container_not_stopped")
        evidence[role] = {"slot": slot, "image": image, "image_id": image_id, "status": info["status"]}
    return evidence


def private_artifact(value, subdir, pattern):
    root = Path.home() / ".togather" / subdir
    path = Path(value)
    require(path.is_absolute() and path.parent == root and matches(pattern, path.name)
            and path.resolve().parent == root.resolve() and not path.is_symlink(), "invalid_artifact_path")
    return path


def backup(env, state, record):
    path = private_artifact(record["snapshot_path"], "snapshots", r"[\w.-]+\.dump")
    sidecar = Path(str(path) + ".json")
    require(not sidecar.is_symlink())
    meta = read_json(sidecar)
    size = path.stat().st_size
    require(meta["snapshot_path"] == str(path) and meta["format"] == "postgres_custom" and
            meta["verified"] is True and meta["verification"] == "pg_restore_full_read" and
            meta["reason"] == "pre-deploy-" + env and meta["deployment_id"] == state["id"] and
            meta["git_commit"] == state["git_commit"], "backup_evidence_mismatch")
    require(natural(meta["size_bytes"]) and meta["size_bytes"] == size > 0, "backup_size_mismatch")
    require(matches(DIGEST, meta["sha256"]) and digest(path) == meta["sha256"], "backup_checksum_mismatch")
    return {"snapshot_path": str(path), "sha256": meta["sha256"], "size_bytes": size,
            "verification": "pg_restore_full_read", "verified": True}


def source_sync(env, state, record):
    path = private_artifact(record["log_file"], "logs/deployments", env + r"_[\w.-]+\.log")
    fields = ("sources_found", "created", "updated", "total", "warnings", "errors")
    # deploy.sh logs CLI output inside its message without escaping nested quotes.
    candidates = []
    with path.open(errors="replace") as stream:
        for line in stream:
            prefix, separator, message = line.partition(',"message":"')
            if not separator or not message.lstrip().startswith("{"):
                continue
            try:
                # Parse the outer metadata independently, so quoted text inside
                # a message cannot impersonate another deployment's log entry.
                metadata = json.loads(prefix + "}")
                if metadata.get("deployment_id") != state["id"] or metadata.get("level") != "INFO":
                    continue
                payload, end = json.JSONDecoder().raw_decode(message.lstrip())
                if message.lstrip()[end:].strip() != '"}':
                    continue
            except ValueError:
                continue
            if isinstance(payload, dict) and "sources_found" in payload:
                candidates.append({key: payload[key] for key in fields})
    require(len(candidates) == 1, "sync_result_missing_or_ambiguous")
    counts = candidates[0]
    require(all(natural(count) for count in counts.values()) and
            counts["sources_found"] == counts["total"] == counts["created"] + counts["updated"] > 0 and
            counts["warnings"] == counts["errors"] == 0, "incomplete_or_failed_sync")
    return counts


def source_audit(slot):
    data = json.loads(run(["docker", "exec", "togather-server-" + slot, "/app/server",
                           "source", "audit", "--json"], timeout=120))
    fields = ("noncanonicalAttributions", "unregisteredAttributions", "configuredIdle",
              "metadataProtected", "sourceRows", "attributionRows", "zeroEventRows", "orphanedIdle")
    counts = {key: data["counts"][key] for key in fields if key in data["counts"]}
    counts["fragmentedFeeds"] = data["fragmentedFeeds"]
    counts["registryFragmentedFeeds"] = data["counts"]["fragmentedFeeds"]
    require(all(natural(count) for count in counts.values()) and counts["noncanonicalAttributions"] ==
            counts["unregisteredAttributions"] == counts["fragmentedFeeds"] == counts["registryFragmentedFeeds"] == 0, "source_registry_invariant_failed")
    return counts


def rollback(env, domain, state, images):
    before = digest(STATE_FILE)
    plan = run(["deploy/scripts/deploy.sh", "rollback", env, "--dry-run"], timeout=90).decode()
    after = digest(STATE_FILE)
    require(before == after == state["state_sha256"], "state_changed_during_preview")
    target = state["previous_slot"]
    image = images["previous"]["image"]
    expected = {"target slot": target, "version": state["previous_version"], "git commit": state["previous_commit"],
                "image": image + " [present]", "verify": "https://" + domain + "/version"}
    for label, value in expected.items():
        require(re.search(r"(?m)\b" + re.escape(label) + r":[ \t]+" + re.escape(value) + r"[ \t]*$", plan),
                "rollback_plan_target_mismatch")
    require(re.search(r"\bactive slot:\s+" + state["active_slot"] + r"\b", plan) and
            re.search(r"\bcontainer:\s+togather-server-" + target + r" \([^\n]*, matches\)", plan) and
            "would REFUSE" not in plan and "Preflight: image present, fallback container usable; a real run would proceed." in plan
            and "Dry run: nothing was changed." in plan, "rollback_preflight_refused_or_incomplete")
    return {"slot": target, "image": image, "state_sha256_before": before, "state_sha256_after": after}


def remote_checks(env, sha, domain):
    checks = {}
    state = check(checks, "deployment_state", lambda: deployment_state(env, sha))
    record = {}
    def read_history():
        record.update(history(env, state))
        return {"deployment_id": state["id"], "docker_image": record["docker_image"]}
    check(checks, "deployment_history", read_history)
    images = check(checks, "containers", lambda: containers(state, record))
    def slot_url():
        return "http://127.0.0.1:" + str(SLOTS[state["active_slot"]])
    check(checks, "direct_identity", lambda: version(slot_url(), sha))
    check(checks, "migrations", lambda: health(slot_url(), sha, state["active_slot"]))
    check(checks, "backup", lambda: backup(env, state, record))
    check(checks, "source_sync", lambda: source_sync(env, state, record))
    check(checks, "source_audit", lambda: source_audit(state["active_slot"]))
    check(checks, "rollback_preview", lambda: rollback(env, domain, state, images))
    return checks


def admin_outcome(output, exit_code):
    # Smoke's credential-bearing transport is private. Only its fixed result
    # messages are interpreted; no subprocess text is copied into the report.
    lines = re.sub(rb"\x1b\[[0-9;]*m", b"", output).splitlines()
    failures = (rb"\[FAIL\] (?:Token exchange request failed|Token exchange response missing a usable token|"
                rb"Admin stats request failed|(?:Token exchange failed|Admin stats endpoint): HTTP (?:[0-9]{3}|unknown))")
    if any(re.fullmatch(failures, line) for line in lines):
        return {"status": "fail", "reason": "admin_auth_failed"}
    if b"[PASS] Admin stats endpoint (200 OK with JWT from STS exchange)" in lines:
        return {"status": "pass", "evidence": {"stats_http_status": 200}}
    if exit_code == 0:
        return {"status": "fail", "reason": "admin_result_unconfirmed"}
    return skip("not_reached_due_smoke_failure")


def local_checks(env, sha, load):
    root = Path(__file__).resolve().parents[2]
    domain, user, host = (os.getenv(name, "") for name in ("NODE_DOMAIN", "SSH_USER", "SSH_HOST"))
    checks = {}
    configured = (matches(r"[A-Za-z0-9][A-Za-z0-9.-]*", domain) and
                  matches(r"[A-Za-z0-9_][A-Za-z0-9_-]*", user) and
                  matches(r"[A-Za-z0-9][A-Za-z0-9.:-]*", host))
    url = "https://" + domain
    target = user + "@" + host
    remote_root = os.getenv("VERIFY_REMOTE_ROOT", "/opt/togather/src")

    def ssh(command, **kwargs):
        require(configured and Path(remote_root).is_absolute(), "invalid_deployment_configuration")
        remote_command = "cd " + shlex.quote(remote_root) + " && " + shlex.join(command)
        return run(["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", target,
                    remote_command], **kwargs)

    try:
        wire = ssh(["python3", "-", "remote", env, sha, domain], timeout=300,
                   input=Path(__file__).read_bytes())
        remote = json.loads(wire)
        require(set(remote) == set(REMOTE_CHECKS) and
                all(entry["status"] in ("pass", "fail") for entry in remote.values()))
        checks.update(remote)
    except PROBE_ERRORS:
        for name in REMOTE_CHECKS:
            checks[name] = {"status": "fail", "reason": "remote_evidence_unavailable" if configured else "invalid_deployment_configuration"}

    state = checks["deployment_state"].get("evidence", {})

    def public():
        require(configured, "invalid_deployment_configuration")
        identity = version(url, sha, state["active_slot"])
        migrations = health(url, sha, state["active_slot"])
        require(migrations["version"] == checks["migrations"].get("evidence", {}).get("version"), "public_migration_version_mismatch")
        return {**identity, "health": migrations["health"], "migration_version": migrations["version"]}

    check(checks, "public_identity", public)

    admin_key_present = bool(os.getenv("ADMIN_API_KEY"))
    checks["admin_auth"] = skip("not_reached_due_smoke_failure" if admin_key_present else "admin API key unavailable")

    def smoke():
        require(configured, "invalid_deployment_configuration")
        smoke_env = {**os.environ, "BASE_URL": url, "SSH_SERVER": target, "ENVIRONMENT": env,
                     "SMOKE_WRITE_TESTS": "false", "ALLOW_DESTRUCTIVE": "false", "TIMEOUT": "10"}
        result = subprocess.run([root / "deploy/testing/smoke-tests.sh"], timeout=300, env=smoke_env,
                                stderr=subprocess.DEVNULL, stdout=subprocess.PIPE, check=False)
        if admin_key_present:
            checks["admin_auth"] = admin_outcome(result.stdout, result.returncode)
        require(result.returncode == 0, "smoke_suite_failed")
        if admin_key_present:
            require(checks["admin_auth"]["status"] == "pass", "smoke_admin_result_unconfirmed")
        return {"exit_code": 0}

    check(checks, "smoke", smoke)

    def load_test():
        require(configured and env == "staging")
        run([root / "deploy/scripts/performance-test.sh", "--profile", "light", "--url", url,
             "--read-ratio", "1", "--max-error-rate", "0"], timeout=300)
        return {"profile": "light", "read_ratio": 1, "exit_code": 0}

    if load:
        check(checks, "load_test", load_test)
    else:
        checks["load_test"] = skip("load test not requested")
    checks["restore_rehearsal"] = skip("requires a separate restore rehearsal")
    checks["real_rollback"] = skip("only rollback preview requested")

    def final_identity():
        identity = public()
        output = json.loads(ssh(["python3", "-", "final-state", env], timeout=30,
                                input=Path(__file__).read_bytes()))
        require(output.get("reason") != "deployment_directory_locked", "deployment_directory_locked")
        require(output["status"] == "pass" and output["evidence"]["state_sha256"] == state["state_sha256"],
                "deployment_state_changed")
        return {**identity, "state_sha256": state["state_sha256"]}

    check(checks, "final_identity", final_identity)
    return checks


def main():
    parser = argparse.ArgumentParser(prog="verify-deployment.sh", description="Verify an existing deployment and emit JSON evidence.")
    parser.add_argument("environment", choices=("staging", "production"))
    parser.add_argument("--expected-commit", required=True, metavar="FULL_SHA")
    parser.add_argument("--load-test", action="store_true", help="include read-only staging load")
    args = parser.parse_args()
    if not matches(SHA, args.expected_commit):
        parser.error("--expected-commit requires a full 40-character lowercase Git SHA")
    if args.environment == "production" and args.load_test:
        parser.error("--load-test is available only on staging")
    checks = local_checks(args.environment, args.expected_commit, args.load_test)
    failed = [name for name, entry in checks.items() if entry["status"] == "fail"]
    status = "fail" if failed else "pass"
    print(json.dumps({"schema_version": 1, "observed_at": datetime.now(timezone.utc).isoformat(),
                      "environment": args.environment, "expected_commit": args.expected_commit,
                      "status": status, "checks": checks}, sort_keys=True))
    summary = "failed checks: " + ", ".join(failed) if failed else "all required checks passed"
    print(f"{args.environment} verification {status}: {summary}", file=sys.stderr)
    return int(bool(failed))


if __name__ == "__main__":
    if sys.argv[0] == "-" and sys.argv[1:2] == ["remote"]:
        print(json.dumps(remote_checks(*sys.argv[2:])))
    elif sys.argv[0] == "-" and sys.argv[1:2] == ["final-state"]:
        results = {}
        check(results, "final_state", lambda: final_state(sys.argv[2]))
        print(json.dumps(results["final_state"]))
    else:
        sys.exit(main())
