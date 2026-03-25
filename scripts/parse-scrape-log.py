#!/usr/bin/env python3
"""
parse-scrape-log.py — Summarize a `server scrape all` log (dry-run or live).

Usage:
  # Capture both stdout (table) and stderr (JSON logs), then parse:
  go run ./cmd/server scrape all --dry-run --log-format console > /tmp/out.txt 2> /tmp/err.txt
  cat /tmp/out.txt /tmp/err.txt | python3 scripts/parse-scrape-log.py

  # Or pipe directly (mix of table + JSON on stderr):
  python3 scripts/parse-scrape-log.py /tmp/out.txt /tmp/err.txt

  # Single merged file:
  python3 scripts/parse-scrape-log.py scrape.log

Output sections:
  BROKEN        — sources with no_startDate, HTTP errors, all URLs failed, or
                  tier 2/3 skipped (no headless)
  ALL-MIDNIGHT  — sources with dates but T00:00:00 times (config gap, lower priority)
  OK            — sources that produced submittable events
"""

import json
import sys
from collections import defaultdict


def parse(lines):
    sources_found = {}
    sources_submit = {}
    issues = defaultdict(set)  # source -> set of issue tags
    skipped_counts = defaultdict(int)
    table_header_seen = False
    col_found = None

    for line in lines:
        line = line.strip()

        # [dry-run] summary line: [dry-run] source=foo found=5 would-submit=3
        if line.startswith("[dry-run]"):
            src = found = submit = None
            for part in line.split():
                if part.startswith("source="):
                    src = part.split("=", 1)[1]
                elif part.startswith("found="):
                    found = int(part.split("=", 1)[1])
                elif part.startswith("would-submit="):
                    submit = int(part.split("=", 1)[1])
            if src is not None:
                sources_found[src] = found or 0
                sources_submit[src] = submit or 0
            continue

        # JSON log lines — try first so they're never misidentified as table rows
        if line.startswith("{"):
            try:
                d = json.loads(line)
                src = d.get("source", "")
                msg = d.get("message", "")
                err = str(d.get("error", ""))
                qw = d.get("quality_warning", "")
                lvl = d.get("level", "")
                url = d.get("url", "")
                status = d.get("status", 0)

                if src:
                    if "no startDate" in err or "no valid dates" in err:
                        issues[src].add("no_startDate")
                        skipped_counts[src] += 1
                    if "all_midnight" in qw:
                        issues[src].add("all_midnight")
                    if "all URLs failed" in msg:
                        issues[src].add("all_urls_failed")
                    if lvl == "error":
                        issues[src].add("error: " + msg[:70])
                    if "request error" in msg and status in (
                        403,
                        404,
                        410,
                        429,
                        500,
                        503,
                    ):
                        issues[src].add("http_%d: %s" % (status, url[:70]))
            except (json.JSONDecodeError, ValueError):
                pass
            continue

        # Table header emitted by `scrape all`:
        # SOURCE                         FOUND  NEW  DUP  FAILED  STATUS
        if line.startswith("SOURCE") and "FOUND" in line and "STATUS" in line:
            table_header_seen = True
            col_found = line.index("FOUND")
            continue

        # Table data rows (fixed-width columns, after header)
        if table_header_seen and col_found is not None:
            if line.startswith("---") or line.startswith("TOTAL"):
                continue
            if len(line) > col_found:
                src = line[:col_found].strip()
                rest = line[col_found:]
                parts = rest.split()
                if src and parts:
                    try:
                        found = int(parts[0])
                        new = int(parts[1]) if len(parts) > 1 else 0
                        dup = int(parts[2]) if len(parts) > 2 else 0
                        status = " ".join(parts[4:]) if len(parts) > 4 else "ok"
                        sources_found[src] = found
                        sources_submit[src] = new + dup
                        if status.startswith("error:"):
                            err_msg = status[6:].strip()
                            if "headless" in err_msg or "RodExtractor" in err_msg:
                                issues[src].add("headless_required")
                            else:
                                issues[src].add("error: " + err_msg[:70])
                    except (ValueError, IndexError):
                        pass

    return sources_found, sources_submit, issues, skipped_counts


def classify(sources_found, sources_submit, issues):
    broken, warning_only, ok = [], [], []

    all_sources = sorted(set(list(sources_found) + list(issues)))
    for src in all_sources:
        found = sources_found.get(src, "?")
        submit = sources_submit.get(src, "?")
        tags = issues.get(src, set())

        is_broken = (
            "no_startDate" in tags
            or "all_urls_failed" in tags
            or any(t.startswith("error:") or t.startswith("http_") for t in tags)
        )
        # headless_required alone is not broken — tier 2/3 just can't run locally
        is_headless_only = (
            "headless_required" in tags
            and not is_broken
            and "all_midnight" not in tags
            and "no_startDate" not in tags
        )
        if is_broken:
            broken.append((src, found, submit, tags))
        elif "all_midnight" in tags or is_headless_only:
            warning_only.append((src, found, submit, tags))
        else:
            ok.append((src, found, submit, tags))

    return broken, warning_only, ok


def print_report(broken, warning_only, ok, skipped_counts):
    W = 36

    if broken:
        print("\n=== BROKEN — need config fixes (%d sources) ===" % len(broken))
        print("  %-*s %6s %7s  %s" % (W, "SOURCE", "FOUND", "SUBMIT", "ISSUES"))
        print("  " + "-" * 90)
        for src, found, submit, tags in broken:
            skipped = skipped_counts.get(src, "")
            tag_str = ", ".join(sorted(tags))
            if skipped:
                tag_str += "  (skipped=%d)" % skipped
            print("  %-*s %6s %7s  %s" % (W, src, str(found), str(submit), tag_str))

    if warning_only:
        print(
            "\n=== WARNINGS — all-midnight or headless-only (%d sources) ==="
            % len(warning_only)
        )
        print("  %-*s %6s %7s  %s" % (W, "SOURCE", "FOUND", "SUBMIT", "ISSUES"))
        print("  " + "-" * 70)
        for src, found, submit, tags in warning_only:
            tag_str = ", ".join(sorted(tags))
            print("  %-*s %6s %7s  %s" % (W, src, str(found), str(submit), tag_str))

    if ok:
        print("\n=== OK — %d sources ===" % len(ok))
        print("  %-*s %6s %7s" % (W, "SOURCE", "FOUND", "SUBMIT"))
        print("  " + "-" * 55)
        for src, found, submit, _ in ok:
            print("  %-*s %6s %7s" % (W, src, str(found), str(submit)))

    total = len(broken) + len(warning_only) + len(ok)
    print(
        "\nSummary: %d total | %d broken | %d warnings | %d ok"
        % (total, len(broken), len(warning_only), len(ok))
    )


def main():
    if len(sys.argv) > 1:
        lines = []
        for path in sys.argv[1:]:
            with open(path) as f:
                lines.extend(f.readlines())
    else:
        lines = sys.stdin.readlines()

    sources_found, sources_submit, issues, skipped_counts = parse(lines)
    broken, warning_only, ok = classify(sources_found, sources_submit, issues)
    print_report(broken, warning_only, ok, skipped_counts)


if __name__ == "__main__":
    main()
