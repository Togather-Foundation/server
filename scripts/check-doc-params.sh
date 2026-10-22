#!/usr/bin/env bash
# check-doc-params.sh — Verify query parameter names in documented public REST
# API URLs are canonical and supported.
#
# Canonical parameter sets mirror the parsers in:
#   internal/domain/events/service.go          (events, events.ics)
#   internal/domain/places/service.go          (places)
#   internal/domain/organizations/service.go   (organizations)
#   internal/api/handlers/feeds.go             (feeds/changes)
#
# Deprecated aliases (still accepted by the API, but they add a `warnings`
# entry to the response) are reported as errors so documentation always shows
# the canonical spelling. Removed/never-supported names (e.g. venueULID,
# start_after) are reported too.
#
# Scanned surfaces: README.md, docs/**/*.md, web/**/*.{md,html,txt},
# skills/**/*.md. Specs, changelogs and the OpenAPI file are intentionally
# excluded — they describe intent/history rather than usage.
#
# Covered endpoints: /api/v1/{events,events.ics,places,organizations} and
# /api/v1/feeds/changes. Other paths (geocode, events:batch, admin/dev) have
# different parameter conventions and are not linted.
#
# Suppress deliberate examples by putting `check-doc-params:ignore` on the URL
# line, or on an opening code-fence line to skip every URL inside that block
# (useful for future/proposed API shapes).
#
# Run from the repo root:
#   scripts/check-doc-params.sh
#
# Exit codes: 0 = clean, 1 = deprecated or unknown parameter found

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

python3 - "$ROOT" <<'PYEOF'
import html
import os
import re
import sys
from urllib.parse import unquote

root = sys.argv[1]

EVENT_PARAMS = {
    "after",
    "startDate",
    "endDate",
    "city",
    "region",
    "venueId",
    "organizerId",
    "state",
    "domain",
    "q",
    "keywords",
    "limit",
    "context",
}

CANONICAL = {
    "events": EVENT_PARAMS,
    "events.ics": EVENT_PARAMS,
    "places": {
        "after",
        "q",
        "city",
        "sort",
        "order",
        "near_place",
        "near_lat",
        "near_lon",
        "radius",
        "countrycodes",
        "limit",
    },
    "organizations": {
        "after",
        "q",
        "city",
        "sort",
        "order",
        "limit",
    },
    "feeds/changes": {
        "since",
        "after",
        "action",
        "include_snapshot",
        "limit",
    },
}

# Deprecated names still accepted by the API -> canonical replacement.
ALIASES = {
    "events": {
        "search": "q",
        "start_date": "startDate",
        "end_date": "endDate",
        "venue_id": "venueId",
        "organizer_id": "organizerId",
        "lifecycle_state": "state",
        "event_domain": "domain",
        "cursor": "after",
    },
    "events.ics": {
        "search": "q",
        "start_date": "startDate",
        "end_date": "endDate",
        "venue_id": "venueId",
        "organizer_id": "organizerId",
        "lifecycle_state": "state",
        "event_domain": "domain",
        "cursor": "after",
    },
    "places": {"search": "q", "cursor": "after"},
    "organizations": {"search": "q", "cursor": "after"},
    "feeds/changes": {},
}

URL_RE = re.compile(
    r"/api/v1/(events\.ics|events|places|organizations|feeds/changes)"
    r"(\?[^\s'\"<>`\)\]]*)?"
)
KEY_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_.-]*$")
FENCE_RE = re.compile(r"^(`{3,}|~{3,})")


def ignored_ranges(content):
    """Return (start, end) offsets of fenced blocks marked with
    check-doc-params:ignore on their opening fence line."""
    ranges = []
    in_block = False
    marked = False
    start = 0
    fence_char = ""
    fence_len = 0
    offset = 0

    for line in content.splitlines(keepends=True):
        fence = FENCE_RE.match(line.lstrip(" \t"))
        if not in_block:
            if fence:
                in_block = True
                marked = "check-doc-params:ignore" in line
                start = offset
                fence_char = fence.group(1)[0]
                fence_len = len(fence.group(1))
        elif fence and fence.group(1)[0] == fence_char and len(fence.group(1)) >= fence_len:
            if marked:
                ranges.append((start, offset + len(line)))
            in_block = False
        offset += len(line)

    if in_block and marked:
        ranges.append((start, offset))
    return ranges


def iter_files():
    readme = os.path.join(root, "README.md")
    if os.path.isfile(readme):
        yield readme

    surfaces = [
        (os.path.join(root, "docs"), (".md",)),
        (os.path.join(root, "web"), (".md", ".html", ".txt")),
        (os.path.join(root, "skills"), (".md",)),
    ]
    for base, extensions in surfaces:
        if not os.path.isdir(base):
            continue
        for dirpath, dirnames, filenames in os.walk(base):
            rel = os.path.relpath(dirpath, root)
            # Generated Scalar assets are vendored, not our documentation.
            if rel == os.path.join("web", "api-docs") or rel.startswith(
                os.path.join("web", "api-docs") + os.sep
            ):
                dirnames[:] = []
                continue
            dirnames[:] = [
                d
                for d in dirnames
                if d not in {"node_modules", ".git", "dist", "vendor"}
            ]
            for name in sorted(filenames):
                if name.endswith(extensions):
                    yield os.path.join(dirpath, name)


def check_file(path):
    try:
        with open(path, encoding="utf-8") as handle:
            content = handle.read()
    except UnicodeDecodeError:
        print(f"  UNREADABLE  {os.path.relpath(path, root)} (not valid UTF-8)", file=sys.stderr)
        return 1, 0

    errors = 0
    seen_params = 0
    ignored_blocks = ignored_ranges(content)

    for match in URL_RE.finditer(content):
        if any(start <= match.start() < end for start, end in ignored_blocks):
            continue
        endpoint = match.group(1)
        query = match.group(2)
        if not query:
            continue

        line_number = content.count("\n", 0, match.start()) + 1
        line_start = content.rfind("\n", 0, match.start()) + 1
        line_end = content.find("\n", match.start())
        if line_end == -1:
            line_end = len(content)
        line = content[line_start:line_end]
        if "check-doc-params:ignore" in line:
            continue

        # HTML escapes ampersands in attributes (&amp;) and may encode entities.
        raw_query = html.unescape(query[1:]).rstrip(".,;:")
        for part in raw_query.split("&"):
            if not part:
                continue
            key = unquote(part.split("=", 1)[0]).strip()
            if not key or not KEY_RE.match(key):
                continue
            seen_params += 1

            canonical = CANONICAL.get(endpoint, set())
            aliases = ALIASES.get(endpoint, {})
            if key in canonical:
                continue

            rel_path = os.path.relpath(path, root)
            if key in aliases:
                print(f"  DEPRECATED  {rel_path}:{line_number}  ?{key}=... -> use `{aliases[key]}`")
            else:
                print(f"  UNKNOWN     {rel_path}:{line_number}  ?{key}=...  (not a supported `{endpoint}` parameter)")
            errors += 1

    return errors, seen_params


def main():
    errors = 0
    files = 0
    params = 0
    for path in iter_files():
        files += 1
        file_errors, file_params = check_file(path)
        errors += file_errors
        params += file_params

    print()
    print(f"Checked {params} query parameters across {files} documentation files.")
    if errors:
        print(f"✗ {errors} deprecated/unknown query parameter(s) found.")
        print("  Use the canonical names listed in docs/api/openapi.yaml.")
        return 1
    print("✓ All documented query parameters are canonical.")
    return 0


sys.exit(main())
PYEOF
