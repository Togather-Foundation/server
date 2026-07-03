#!/usr/bin/env python3
"""togather-poll — poll a Togather change feed and accumulate new events.

Usage:
    togather-poll --server https://your-instance.example.com \\
                  --api-key YOUR_KEY \\
                  [--city "Toronto"] \\
                  [--output-dir ~/.hermes/togather]

Consumes the Togather /api/v1/feeds/changes endpoint with cursor-based
pagination. Filters events by city (case-insensitive blacklist of known
non-local cities). Deduplicates via a seen-ULIDs file. Writes new events
to daily_new.jsonl and appends everything to weekly_draft.jsonl.

Outputs a machine-readable JSON summary to stdout. Exit code 0 on success
(even if zero new events), non-zero on failure.

Designed for use as a Hermes cron script (no_agent: true). Zero external
dependencies — Python stdlib only.
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

# ── City filter: known non-local cities (case-insensitive substrings) ──
# Extend this list as you discover new false-positive cities in your feed.
DEFAULT_NON_LOCAL_CITIES = [
    "san francisco", "new york", "philadelphia", "los angeles",
    "washington", "miami", "houston", "seattle", "baltimore",
    "oakland", "cambridge", "australia", "chicago", "denver",
    "boston", "portland", "london", "brooklyn", "san diego",
    "tampa", "orlando", "vancouver",
]


def parse_args():
    p = argparse.ArgumentParser(
        description="Poll Togather change feed for new events"
    )
    p.add_argument("--server", required=True,
                   help="Base URL of the Togather instance")
    p.add_argument("--api-key", required=True,
                   help="Togather API key (agent/public scope)")
    p.add_argument("--city", default="Toronto",
                   help="City name for venue filtering (default: Toronto)")
    p.add_argument("--output-dir", default=None,
                   help="State directory (default: ~/.hermes/togather)")
    p.add_argument("--non-local-cities", default=None,
                   help="Comma-separated list of non-local city substrings")
    return p.parse_args()


def load_seen_ulids(seen_file: Path) -> set:
    """Load previously-seen ULIDs from a flat text file."""
    if not seen_file.exists():
        return set()
    seen = set()
    with open(seen_file) as f:
        for line in f:
            ulid = line.strip()
            if ulid:
                seen.add(ulid)
    return seen


def is_local(venue_name: str, non_local: list[str]) -> bool:
    """Check if a venue name suggests a local event.

    Uses a blacklist approach: if the venue contains any known non-local
    city name, it's filtered out. Everything else passes through.
    """
    venue_lower = venue_name.lower()
    for city in non_local:
        if city in venue_lower:
            return False
    return True


def extract_ulid(snapshot: dict) -> str:
    """Extract the ULID from a JSON-LD @id field.

    Handles both URL forms:
      https://instance.example.com/events/01ABCDEFGH
      /events/01ABCDEFGH
    """
    raw = snapshot.get("@id", "")
    return raw.rstrip("/").rsplit("/", 1)[-1]


def fetch_page(server: str, api_key: str, cursor: str,
               limit: int = 200) -> dict:
    """Fetch one page of the change feed. Returns parsed JSON."""
    url = (
        f"{server}/api/v1/feeds/changes"
        f"?since={cursor}&action=create&include_snapshot=true&limit={limit}"
    )
    req = urllib.request.Request(url)
    req.add_header("Authorization", f"Bearer {api_key}")
    req.add_header("Accept", "application/json")

    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        print(json.dumps({
            "status": "error",
            "message": f"HTTP {e.code} from change feed",
            "detail": body[:500],
        }), file=sys.stderr)
        sys.exit(1)
    except urllib.error.URLError as e:
        print(json.dumps({
            "status": "error",
            "message": f"Could not reach {server}: {e.reason}",
        }), file=sys.stderr)
        sys.exit(1)


def main():
    args = parse_args()

    # Resolve output directory
    if args.output_dir:
        state_dir = Path(args.output_dir).expanduser()
    else:
        hermes_home = os.environ.get("HERMES_HOME",
                                     os.path.expanduser("~/.hermes"))
        state_dir = Path(hermes_home) / "togather"
    state_dir.mkdir(parents=True, exist_ok=True)

    # Resolve paths
    cursor_file = state_dir / "cursor.txt"
    draft_file = state_dir / "weekly_draft.jsonl"
    new_file = state_dir / "daily_new.jsonl"
    seen_file = state_dir / "seen_ulids.txt"

    # Load seen ULIDs
    seen = load_seen_ulids(seen_file)

    # Resolve cursor
    if cursor_file.exists() and cursor_file.stat().st_size > 0:
        cursor = cursor_file.read_text().strip()
    else:
        # Default: start from 30 days ago
        cursor = (datetime.now(timezone.utc)
                  .replace(day=datetime.now(timezone.utc).day - 30)
                  .strftime("%Y-%m-%dT%H:%M:%SZ"))

    # Resolve non-local city list
    non_local = DEFAULT_NON_LOCAL_CITIES.copy()
    if args.non_local_cities:
        non_local.extend(
            c.strip().lower() for c in args.non_local_cities.split(",") if c.strip()
        )

    # ── Main loop: paginate through the change feed ──
    new_total = 0
    domain_counts: dict[str, int] = {}

    while True:
        data = fetch_page(args.server, args.api_key, cursor)

        changes = data.get("changes", [])
        if not changes:
            break

        next_cursor = data.get("next_cursor", "")
        if next_cursor:
            cursor_file.write_text(next_cursor)

        for change in changes:
            snapshot = change.get("snapshot")
            if not snapshot:
                continue

            ulid = extract_ulid(snapshot)
            if not ulid:
                continue
            if ulid in seen:
                continue

            # City filter
            venue_name = (
                snapshot.get("location", {}).get("name", "") or ""
            )
            if venue_name and not is_local(venue_name, non_local):
                continue

            # Mark as seen
            seen.add(ulid)
            with open(seen_file, "a") as sf:
                sf.write(f"{ulid}\n")

            # Serialize snapshot
            line = json.dumps(snapshot, ensure_ascii=False)

            # Write to daily_new (this run only)
            with open(new_file, "a") as nf:
                nf.write(line + "\n")

            # Append to weekly_draft (accumulation)
            with open(draft_file, "a") as df:
                df.write(line + "\n")

            new_total += 1
            domain = snapshot.get("eventDomain", "unknown")
            domain_counts[domain] = domain_counts.get(domain, 0) + 1

        if not next_cursor:
            break
        cursor = next_cursor

    # ── Output summary ──
    if new_total == 0:
        # Clean up empty daily file
        if new_file.exists():
            new_file.unlink()
        print(json.dumps({
            "status": "ok",
            "new_events": 0,
            "events": [],
        }))
    else:
        print(json.dumps({
            "status": "ok",
            "new_events": new_total,
            "domains": domain_counts,
            "daily_new_file": str(new_file),
        }))


if __name__ == "__main__":
    main()
