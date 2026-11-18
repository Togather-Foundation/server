#!/usr/bin/env python3
"""Allowlisted command doubles for the deployment verification behavior suite."""
import json
import os
import shlex
import sys
from pathlib import Path

case = os.environ["VERIFY_CASE"]
root = Path(os.environ["VERIFY_REMOTE_ROOT"])
name = Path(sys.argv[0]).name
args = sys.argv[1:]
sha = "a" * 40
previous = "b" * 40
domain = "staging.fixture.invalid"
sentinel = "UNSAFE_PRIVATE_FIXTURE_CONTENT"


def event(message):
    with Path(os.environ["PROBE_EVENTS"]).open("a") as stream:
        print(message, file=stream)


def deny():
    event("DENIED " + name)
    sys.exit(93)


def ssh():
    if args[-2] != "fixture@host.fixture.invalid":
        deny()
    command = shlex.split(args[-1])
    if command[:3] != ["cd", str(root), "&&"]:
        deny()
    command = command[3:]
    if command == ["python3", "-", "remote", "staging", sha, domain]:
        event("collector")
        if case == "ssh-failure":
            print(sentinel, file=sys.stderr)
            sys.exit(255)
    elif command == ["python3", "-", "final-state", "staging"]:
        event("final state")
        if case == "late-change":
            with (root / "deploy/config/deployment-state.json").open("a") as stream:
                stream.write("\n")
        elif case == "late-lock":
            (root / "deploy/config/togather-deploy-staging.lock").mkdir()
    else:
        deny()
    os.chdir(root)
    os.execvp(command[0], command)  # Preserve the streamed helper on stdin.


def docker():
    def check_slot(container):
        if container not in ("togather-server-blue", "togather-server-green"):
            deny()
        return container == "togather-server-blue"

    if len(args) == 4 and args[:2] == ["inspect", "--format"]:
        active = check_slot(args[-1])
        commit = sha if active else previous
        result = {
            "image": "togather-server:" + commit[:7],
            "image_id": "sha256:" + commit[0] * 64,
            "status": "running" if active else "exited",
            "health": "healthy" if active else "",
        }
        if not active and case == "old-writer":
            result["status"] = "running"
        if active and case == "unhealthy":
            result["health"] = "unhealthy"
        if not active and case == "missing-container":
            sys.exit(1)
        print(json.dumps(result))
    elif len(args) == 5 and args[:3] == ["image", "inspect", "--format"]:
        if args[-1] not in ("togather-server:aaaaaaa", "togather-server:bbbbbbb"):
            deny()
        if case == "missing-image" and args[-1].endswith("bbbbbbb"):
            sys.exit(1)
        print("sha256:" + args[-1][-1] * 64)
    elif args == ["exec", "togather-server-blue", "/app/server", "source", "audit", "--json"]:
        counts = dict(noncanonicalAttributions=0, unregisteredAttributions=0,
                      fragmentedFeeds=0, sourceRows=2, attributionRows=100,
                      zeroEventRows=0, archivedAttributions=50, configuredIdle=0,
                      orphanedIdle=0, metadataProtected=0)
        if case == "source-audit":
            counts["noncanonicalAttributions"] = 1
        if case == "audit-counts":
            counts["fragmentedFeeds"] = 1
        print(json.dumps(dict(counts=counts, fragmentedFeeds=0, fragmentedWithUnownedRows=0,
                              fragmentedConfigOwned=0, zeroEventSourceRows=0)))
        event("audit")
    else:
        deny()


def curl():
    url = args[-1]
    allowed_options = {"-q", "--silent", "--show-error", "--fail", "--max-time",
                       "15", "--include", "--suppress-connect-headers"}
    if set(args[:-1]) - allowed_options:
        deny()
    public = "https://" + domain
    direct = "http://127.0.0.1:8081"
    if url not in (public + "/version", direct + "/version", public + "/health", direct + "/health"):
        deny()
    event("GET " + url)
    if url == public + "/version" and case == "http-failure":
        print(sentinel, file=sys.stderr)
        sys.exit(22)
    header_slot = "green" if case == "wrong-header" else "blue"
    commit = sha
    if (case == "wrong-sha" and url == public + "/version" or
            case == "wrong-direct" and url == direct + "/version"):
        commit = previous
    body = dict(version=commit[:7], git_commit=commit, slot="blue", private=sentinel)
    if url.endswith("/health"):
        probe = "public" if url.startswith(public) else "direct"
        if case == probe + "-health-sha":
            body["git_commit"] = previous
        if case == probe + "-health-slot":
            body["slot"] = "green"
        migration = dict(status="pass", details=dict(version=62, dirty=case == "dirty"))
        body.update(status="healthy", checks=dict(migrations=migration))
    print(f"HTTP/1.1 200 OK\r\nX-Togather-Slot: {header_slot}\r\n\r\n{json.dumps(body)}")


def preview():
    if args != ["rollback", "staging", "--dry-run"]:
        deny()
    event("preview")
    if case == "preview-write":
        with (root / "deploy/config/deployment-state.json").open("a") as stream:
            stream.write("\n")
    actual_version = "badbeef" if case == "wrong-preview" else previous[:7]
    for line in (
        "Rollback plan for staging",
        "  active slot:   blue  (from state)", "  target slot:   green",
        "  version:       " + actual_version, "  git commit:    " + previous,
        "  image:         togather-server:bbbbbbb [present]",
        "  container:     togather-server-green (state exited, image togather-server:bbbbbbb, matches)",
        "  verify:        https://" + domain + "/version",
        "Preflight: image present, fallback container usable; a real run would proceed.",
        "Dry run: nothing was changed.",
    ):
        print("[INFO] " + line)


def smoke():
    if (args or os.environ.get("SMOKE_WRITE_TESTS") != "false" or
            os.environ.get("ALLOW_DESTRUCTIVE") != "false" or
            os.environ.get("ENVIRONMENT") != "staging" or
            os.environ.get("BASE_URL") != "https://" + domain or
            os.environ.get("SSH_SERVER") != "fixture@host.fixture.invalid" or
            os.environ.get("ADMIN_API_KEY", "") != ("" if case == "no-key" else sentinel)):
        deny()
    event("smoke")
    print(sentinel)
    if case == "smoke-failure":
        sys.exit(1)
    if os.environ.get("ADMIN_API_KEY") and case != "auth-unverified":
        print("\033[0;34m[INFO]\033[0m Test 20/20: Admin auth smoke test (staging) - POST "
              "https://" + domain + "/api/v1/auth/token")
        if case == "auth-failure":
            print("\033[0;31m[FAIL]\033[0m Token exchange failed: HTTP 401")
            sys.exit(1)
        print("\033[0;32m[PASS]\033[0m Token exchange (200 OK, token received)")
        print("\033[0;32m[PASS]\033[0m Admin stats endpoint (200 OK with JWT from STS exchange)")


def load_test():
    if args != ["--profile", "light", "--url", "https://" + domain,
                "--read-ratio", "1", "--max-error-rate", "0"]:
        deny()
    event("load")
    print(sentinel, file=sys.stderr)
    if case == "load-failure":
        sys.exit(1)


dispatch = {"ssh": ssh, "docker": docker, "curl": curl, "deploy.sh": preview,
            "smoke-tests.sh": smoke, "performance-test.sh": load_test}
dispatch.get(name, deny)()
