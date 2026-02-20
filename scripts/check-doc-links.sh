#!/usr/bin/env bash
# check-doc-links.sh — Verify all relative Markdown links in docs/ resolve to real files.
#
# Checks only local relative links (not http/https/mailto/#anchors).
# Skips content inside fenced code blocks (``` or ~~~).
# Run from the repo root:
#   scripts/check-doc-links.sh
#   scripts/check-doc-links.sh docs/deploy/  # check a subdirectory only
#
# Exit codes: 0 = all links OK, 1 = broken links found

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SEARCH_DIR="${1:-$ROOT/docs}"
ERRORS=0
FILES_CHECKED=0
LINKS_CHECKED=0

check_link() {
    local src_file="$1"
    local link="$2"

    # Strip anchor fragment
    local path="${link%%#*}"

    # Skip empty (anchor-only links like #section) and null placeholders
    [[ -z "$path" ]] && return 0
    [[ "$path" == "null" ]] && return 0

    local src_dir
    src_dir="$(dirname "$src_file")"
    local target
    target="$(cd "$src_dir" && realpath -m "$path" 2>/dev/null || echo "")"

    if [[ -z "$target" ]] || [[ ! -e "$target" ]]; then
        echo "  BROKEN  $link"
        echo "          in: ${src_file#$ROOT/}"
        ERRORS=$((ERRORS + 1))
    fi
}

# Extract local relative links from a markdown file, skipping fenced code blocks.
extract_links() {
    local file="$1"
    python3 - "$file" <<'PYEOF'
import re, sys

with open(sys.argv[1], encoding="utf-8") as f:
    content = f.read()

# Remove fenced code blocks (``` or ~~~), including indented ones
content = re.sub(r'(?m)^[ \t]*(`{3,}|~{3,}).*?^\1[ \t]*$', '', content, flags=re.DOTALL | re.MULTILINE)

# Extract link targets from [text](target) syntax
for m in re.finditer(r'\[(?:[^\]]*)\]\(([^)]+)\)', content):
    target = m.group(1).strip()
    # Skip http/https/ftp/mailto and pure anchors
    if re.match(r'^(https?|ftp)://', target):
        continue
    if target.startswith('mailto:'):
        continue
    print(target)
PYEOF
}

while IFS= read -r -d '' md_file; do
    FILES_CHECKED=$((FILES_CHECKED + 1))

    while IFS= read -r link; do
        [[ -z "$link" ]] && continue
        LINKS_CHECKED=$((LINKS_CHECKED + 1))
        check_link "$md_file" "$link"
    done < <(extract_links "$md_file")
done < <(find "$SEARCH_DIR" -name "*.md" -print0 | sort -z)

echo ""
echo "Checked $LINKS_CHECKED local links across $FILES_CHECKED Markdown files."

if [[ $ERRORS -gt 0 ]]; then
    echo "✗ $ERRORS broken link(s) found."
    exit 1
else
    echo "✓ All local links OK."
fi
