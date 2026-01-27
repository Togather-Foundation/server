"""Extract user instructions from OpenCode chat history.

Usage:
  uv run extract_opencode_instructions.py --output opencode_agent_instructions.txt
  uv run extract_opencode_instructions.py --output opencode_agent_instructions_filtered.txt --from "3 days ago"
  uv run extract_opencode_instructions.py --output opencode_agent_instructions_all.txt --filter none --keep-tool-output --include-synthetic --max-lines 0 --min-length 0
"""

import argparse
import json
from datetime import datetime, timedelta, timezone
from pathlib import Path
import re


def load_json(path):
    try:
        with path.open("r", encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, json.JSONDecodeError):
        return None


def iso_timestamp(ms):
    if ms is None:
        return ""
    try:
        dt = datetime.fromtimestamp(ms / 1000, tz=timezone.utc)
    except (TypeError, ValueError, OSError):
        return ""
    return dt.isoformat()


def parse_date(value, now):
    if not value:
        return None
    raw = value.strip().lower()
    if raw in {"now", "today"}:
        return now
    if raw == "yesterday":
        return now - timedelta(days=1)

    try:
        return datetime.fromisoformat(value).replace(tzinfo=timezone.utc)
    except ValueError:
        pass

    try:
        return datetime.strptime(value, "%Y-%m-%d").replace(tzinfo=timezone.utc)
    except ValueError:
        pass

    match = re.match(r"^(\d+)\s+(day|days|hour|hours|week|weeks)\s+ago$", raw)
    if match:
        amount = int(match.group(1))
        unit = match.group(2)
        if unit.startswith("day"):
            return now - timedelta(days=amount)
        if unit.startswith("hour"):
            return now - timedelta(hours=amount)
        if unit.startswith("week"):
            return now - timedelta(weeks=amount)
    return None


def build_message_index(message_root):
    index = {}
    if not message_root.exists():
        return index
    for path in sorted(message_root.glob("**/*.json")):
        data = load_json(path)
        if not data:
            continue
        msg_id = data.get("id")
        if not msg_id:
            continue
        index[msg_id] = {
            "role": data.get("role"),
            "session_id": data.get("sessionID"),
            "created": data.get("time", {}).get("created"),
        }
    return index


def build_parts_index(part_root, exclude_synthetic):
    parts = {}
    if not part_root.exists():
        return parts
    for path in sorted(part_root.glob("**/*.json")):
        data = load_json(path)
        if not data:
            continue
        if exclude_synthetic and data.get("synthetic") is True:
            continue
        text = data.get("text")
        if not text:
            continue
        msg_id = data.get("messageID")
        if not msg_id:
            continue
        parts.setdefault(msg_id, []).append((data.get("id", ""), text))
    return parts


def strip_tool_output(text):
    lines = text.splitlines()
    cleaned = []
    skip_block = False
    skip_until_end_marker = False
    for line in lines:
        if skip_until_end_marker:
            if line.strip().startswith("(End of file"):
                skip_until_end_marker = False
            continue
        if skip_block:
            if line.strip().lower().startswith("</file>"):
                skip_block = False
            continue
        if "Called the Read tool with the following input:" in line:
            skip_block = True
            continue
        if line.strip().startswith("<file>"):
            skip_until_end_marker = True
            continue
        cleaned.append(line)
    return "\n".join(cleaned)


def format_entries(message_index, parts_index, strip_tools):
    entries = []
    for msg_id, meta in message_index.items():
        if meta.get("role") != "user":
            continue
        message_parts = parts_index.get(msg_id, [])
        if not message_parts:
            continue
        message_parts.sort(key=lambda item: item[0])
        text = "".join(part for _, part in message_parts).strip()
        if strip_tools:
            text = strip_tool_output(text).strip()
        if not text:
            continue
        entries.append(
            {
                "id": msg_id,
                "session_id": meta.get("session_id", ""),
                "created": meta.get("created"),
                "timestamp": iso_timestamp(meta.get("created")),
                "text": text,
            }
        )
    entries.sort(key=lambda item: item.get("created") or 0)
    return entries


def write_output(entries, output_path, storage_root, label, from_dt, to_dt):
    output_lines = [
        f"OpenCode user instructions extracted ({label})",
        f"Storage root: {storage_root}",
        f"Total entries: {len(entries)}",
        f"Date range: {format_date(from_dt)} to {format_date(to_dt)}",
        "",
    ]
    for idx, entry in enumerate(entries, start=1):
        header = f"[{idx}] {entry['timestamp']} session={entry['session_id']} message={entry['id']}"
        output_lines.append(header)
        output_lines.append(entry["text"])
        output_lines.append("-" * 72)
    output_path.write_text("\n".join(output_lines).rstrip() + "\n", encoding="utf-8")


def is_trivial(text, min_length, trivial_phrases, trivial_words):
    normalized = " ".join(text.split()).strip().lower()
    if not normalized:
        return True
    if len(normalized) < min_length:
        return True
    if normalized in trivial_phrases:
        return True
    words = normalized.split()
    if len(words) <= 3 and all(word in trivial_words for word in words):
        return True
    return False


def format_date(value):
    if value is None:
        return "(none)"
    return value.isoformat()


def line_count(text):
    return len([line for line in text.splitlines() if line.strip()])


def apply_date_filter(entries, from_dt, to_dt):
    if not from_dt and not to_dt:
        return entries
    filtered = []
    for entry in entries:
        created_ms = entry.get("created")
        if created_ms is None:
            continue
        created_dt = datetime.fromtimestamp(created_ms / 1000, tz=timezone.utc)
        if from_dt and created_dt < from_dt:
            continue
        if to_dt and created_dt > to_dt:
            continue
        filtered.append(entry)
    return filtered


def filter_entries(entries, mode, min_length, max_lines, from_dt, to_dt):
    if mode == "none":
        if max_lines:
            entries = [
                entry for entry in entries if line_count(entry["text"]) < max_lines
            ]
        return apply_date_filter(entries, from_dt, to_dt)

    trivial_phrases = {
        "yes",
        "no",
        "ok",
        "okay",
        "thanks",
        "thank you",
        "thx",
        "yep",
        "yup",
        "sure",
        "sounds good",
        "looks good",
        "great",
        "cool",
        "nice",
        "please",
        "proceed",
        "continue",
        "continue if you have next steps",
        "done",
    }
    trivial_words = {
        "yes",
        "no",
        "ok",
        "okay",
        "thanks",
        "thank",
        "you",
        "thx",
        "yep",
        "yup",
        "sure",
        "please",
        "proceed",
        "continue",
        "done",
        "great",
        "cool",
        "nice",
    }

    filtered = []
    for entry in entries:
        text = entry["text"]
        if max_lines and line_count(text) >= max_lines:
            continue
        if is_trivial(text, min_length, trivial_phrases, trivial_words):
            continue
        filtered.append(entry)
    return apply_date_filter(filtered, from_dt, to_dt)


def main():
    parser = argparse.ArgumentParser(
        description="Extract user instructions from OpenCode chat history."
    )
    parser.add_argument(
        "--root",
        default=str(Path.home() / ".local/share/opencode/storage"),
        help="OpenCode storage root (default: ~/.local/share/opencode/storage)",
    )
    parser.add_argument(
        "--output",
        default="opencode_agent_instructions.txt",
        help="Output file path (default: opencode_agent_instructions.txt)",
    )
    parser.add_argument(
        "--filter",
        choices=["none", "smart"],
        default="smart",
        help="Filter mode for trivial responses (default: smart)",
    )
    parser.add_argument(
        "--keep-tool-output",
        action="store_true",
        help="Keep embedded tool output blocks in user messages",
    )
    parser.add_argument(
        "--include-synthetic",
        action="store_true",
        help="Include synthetic injected content",
    )
    parser.add_argument(
        "--max-lines",
        type=int,
        default=10,
        help="Drop entries with this many non-empty lines or more (0 disables)",
    )
    parser.add_argument(
        "--from",
        dest="from_date",
        default="",
        help='Start date filter (e.g., "2026-01-01", "3 days ago", "yesterday")',
    )
    parser.add_argument(
        "--to",
        dest="to_date",
        default="",
        help='End date filter (e.g., "2026-01-26", "now")',
    )
    parser.add_argument(
        "--min-length",
        type=int,
        default=12,
        help="Minimum character length to keep in smart filter (default: 12)",
    )
    args = parser.parse_args()

    storage_root = Path(args.root).expanduser()
    message_root = storage_root / "message"
    part_root = storage_root / "part"

    message_index = build_message_index(message_root)
    exclude_synthetic = not args.include_synthetic
    strip_tools = not args.keep_tool_output
    parts_index = build_parts_index(part_root, exclude_synthetic)
    entries = format_entries(message_index, parts_index, strip_tools)

    output_path = Path(args.output).expanduser()
    now = datetime.now(timezone.utc)
    from_dt = parse_date(args.from_date, now)
    to_dt = parse_date(args.to_date, now)
    filtered_entries = filter_entries(
        entries, args.filter, args.min_length, args.max_lines, from_dt, to_dt
    )
    write_output(
        filtered_entries, output_path, storage_root, args.filter, from_dt, to_dt
    )
    print(f"Wrote {len(filtered_entries)} entries to {output_path}")


if __name__ == "__main__":
    main()
