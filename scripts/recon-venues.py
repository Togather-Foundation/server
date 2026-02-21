#!/usr/bin/env python3
"""
recon-venues.py — Find Toronto arts/culture venue websites that are good
candidates for scraping events.

Step 1: Discover org URLs from t0ronto.ca (via /all.json)
Step 2: Probe each URL for tech stack, JSON-LD events, subpages
Step 3: Output TSV + human-readable summary
Step 4: Skip domains already in configs/sources/*.yaml
"""

import argparse
import concurrent.futures
import json
import os
import re
import sys
import warnings
from pathlib import Path
from urllib.parse import urljoin, urlparse

warnings.filterwarnings("ignore")

try:
    import requests
    from bs4 import BeautifulSoup
except ImportError:
    print("Installing required packages...")
    import subprocess

    subprocess.check_call(
        [sys.executable, "-m", "pip", "install", "requests", "beautifulsoup4", "-q"]
    )
    import requests
    from bs4 import BeautifulSoup

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

ARTS_TAGS = {
    "topic/art-form",
    "instance-of/event/festival",
    "instance-of/place/event-venue",
    "instance-of/place/gallery",
    "instance-of/place/theatre",
    "instance-of/organization",
    "topic/art-form/music",
    "topic/art-form/theatre",
    "topic/art-form/dance",
    "topic/art-form/art",
    "topic/art-form/film",
    "topic/art-form/performance",
    "topic/art-form/comedy",
    "topic/art-form/media-arts",
}

EVENT_LINK_KEYWORDS = re.compile(
    r"event|calendar|concert|performance|season|program|show|whats-on|what-s-on",
    re.IGNORECASE,
)

TECH_PATTERNS = {
    "wordpress": re.compile(r"wp-content|wp-includes|wordpress", re.IGNORECASE),
    "tribe-events": re.compile(
        r"tribe[_-]events|tribe/events|the-events-calendar", re.IGNORECASE
    ),
    "squarespace": re.compile(r"squarespace", re.IGNORECASE),
    "wix": re.compile(r"wix\.com|wixsite|wixstatic", re.IGNORECASE),
    "webflow": re.compile(r"webflow\.io|webflow\.com", re.IGNORECASE),
    "nextjs": re.compile(r"__NEXT_DATA__|next/static|_next/static", re.IGNORECASE),
    "drupal": re.compile(r"drupal|sites/default/files", re.IGNORECASE),
    "elementor": re.compile(r"elementor", re.IGNORECASE),
}

SKIP_DOMAINS = re.compile(
    r"instagram\.com|facebook\.com|twitter\.com|x\.com|tiktok\.com|"
    r"youtube\.com|linkedin\.com|eventbrite\.com|ticketmaster\.com|"
    r"meetup\.com|bsky\.app|luma\.com|discord\.gg|substack\.com|"
    r"wikipedia\.org|toronto\.ca|canada\.ca",
    re.IGNORECASE,
)

TIMEOUT = 10
HEADERS = {
    "User-Agent": (
        "Mozilla/5.0 (compatible; togather-recon/1.0; +https://togather.foundation)"
    ),
    "Accept": "text/html,application/xhtml+xml,*/*",
}

# Resolve script location for relative paths
SCRIPT_DIR = Path(__file__).parent.resolve()
REPO_ROOT = SCRIPT_DIR.parent
CONFIGS_DIR = REPO_ROOT / "configs" / "sources"
OUTPUT_FILE = SCRIPT_DIR / "recon-output.tsv"
T0RONTO_JSON = "https://t0ronto.ca/all.json"


# ---------------------------------------------------------------------------
# Step 4: Load already-configured domains
# ---------------------------------------------------------------------------


def load_configured_domains() -> set:
    """Read all configs/sources/*.yaml, extract url: fields, return base domains."""
    domains = set()
    if not CONFIGS_DIR.exists():
        return domains
    for yaml_file in CONFIGS_DIR.glob("*.yaml"):
        if yaml_file.name.startswith("_"):
            continue
        text = yaml_file.read_text(errors="replace")
        for line in text.splitlines():
            m = re.match(r'^\s*url:\s*["\']?(https?://[^\s"\']+)', line)
            if m:
                parsed = urlparse(m.group(1))
                domains.add(parsed.netloc.lower().lstrip("www."))
    return domains


# ---------------------------------------------------------------------------
# Step 1: Discover org URLs from t0ronto.ca
# ---------------------------------------------------------------------------


def is_arts_community(entry: dict) -> bool:
    """Return True if the entry has arts/culture/event tags."""
    tags = entry.get("tags", []) or []
    for tag in tags:
        for arts_tag in ARTS_TAGS:
            if tag.startswith(arts_tag):
                return True
    return False


def discover_t0ronto_urls(configured_domains: set) -> list:
    """Fetch t0ronto.ca/all.json and extract arts org URLs."""
    print(f"Fetching {T0RONTO_JSON} ...", flush=True)
    try:
        resp = requests.get(T0RONTO_JSON, timeout=30, headers=HEADERS)
        resp.raise_for_status()
        data = resp.json()
    except Exception as e:
        print(f"ERROR fetching t0ronto.ca data: {e}", file=sys.stderr)
        return []

    communities = data.get("communities", [])
    total = len(communities)
    print(f"Total communities in directory: {total}", flush=True)

    urls = []
    skipped_no_arts = 0
    skipped_no_url = 0
    skipped_social_only = 0
    skipped_already_configured = 0

    for entry in communities:
        link = entry.get("link", "")
        if not link or not link.startswith("http"):
            skipped_no_url += 1
            continue

        parsed = urlparse(link)
        domain = parsed.netloc.lower().lstrip("www.")

        # Skip social media / ticketing aggregators
        if SKIP_DOMAINS.search(domain):
            skipped_social_only += 1
            continue

        # Skip already-configured
        base = domain.lstrip("www.")
        if base in configured_domains or domain in configured_domains:
            skipped_already_configured += 1
            continue

        # Filter to arts/culture orgs
        if not is_arts_community(entry):
            skipped_no_arts += 1
            continue

        urls.append(
            {
                "name": entry.get("name", ""),
                "url": link,
                "domain": domain,
                "tags": entry.get("tags", []),
            }
        )

    print(
        f"Discovered {len(urls)} arts/culture org URLs "
        f"(skipped: {skipped_no_arts} non-arts, {skipped_no_url} no-url, "
        f"{skipped_social_only} social-only, {skipped_already_configured} already-configured)",
        flush=True,
    )
    return urls


# ---------------------------------------------------------------------------
# Step 2: Probe each URL
# ---------------------------------------------------------------------------


def detect_tech(html: str) -> list:
    """Return list of detected tech identifiers."""
    found = []
    for name, pattern in TECH_PATTERNS.items():
        if pattern.search(html):
            found.append(name)
    if not found:
        found.append("static")
    return found


def count_jsonld_events(html: str) -> int:
    """Count <script type=application/ld+json> blocks containing Event @type."""
    soup = BeautifulSoup(html, "html.parser")
    count = 0
    for tag in soup.find_all("script", type="application/ld+json"):
        try:
            data = json.loads(tag.string or "")
        except Exception:
            continue
        # data can be a list or dict
        items = data if isinstance(data, list) else [data]
        for item in items:
            if isinstance(item, dict):
                types = item.get("@type", "")
                if isinstance(types, list):
                    if any("event" in t.lower() for t in types):
                        count += 1
                elif isinstance(types, str) and "event" in types.lower():
                    count += 1
    return count


def find_candidate_event_links(html: str, base_url: str) -> list:
    """Find hrefs on the page that look like event listing links."""
    soup = BeautifulSoup(html, "html.parser")
    seen = set()
    results = []
    for a in soup.find_all("a", href=True):
        href = a["href"].strip()
        if not href or href.startswith("#") or href.startswith("mailto:"):
            continue
        if EVENT_LINK_KEYWORDS.search(href) or EVENT_LINK_KEYWORDS.search(
            a.get_text(strip=True)
        ):
            full = urljoin(base_url, href)
            if full not in seen:
                seen.add(full)
                results.append(full)
    return results[:10]  # cap at 10


def check_subpage(session: requests.Session, base_url: str, path: str) -> bool:
    """Return True if base_url + path returns HTTP 200."""
    try:
        url = base_url.rstrip("/") + path
        r = session.head(url, timeout=TIMEOUT, allow_redirects=True)
        if r.status_code == 405:  # HEAD not allowed, try GET
            r = session.get(url, timeout=TIMEOUT, allow_redirects=True, stream=True)
            r.close()
        return r.status_code == 200
    except Exception:
        return False


def probe_url(entry: dict) -> dict:
    """Probe a single org URL and return a result dict."""
    url = entry["url"]
    domain = entry["domain"]
    name = entry["name"]

    result = {
        "name": name,
        "domain": domain,
        "final_url": url,
        "status": 0,
        "tech_stack": "unknown",
        "jsonld_event_count": 0,
        "events_subpage": False,
        "tribe_api": False,
        "tier_guess": "SKIP",
        "candidate_urls": [],
        "error": None,
    }

    session = requests.Session()
    session.headers.update(HEADERS)

    try:
        resp = session.get(url, timeout=TIMEOUT, allow_redirects=True)
        result["final_url"] = resp.url
        result["status"] = resp.status_code

        if resp.status_code >= 400:
            result["error"] = f"HTTP {resp.status_code}"
            return result

        html = resp.text
        tech = detect_tech(html)
        result["tech_stack"] = ",".join(tech)
        result["jsonld_event_count"] = count_jsonld_events(html)
        result["candidate_urls"] = find_candidate_event_links(html, resp.url)

        # Check subpages
        result["events_subpage"] = check_subpage(
            session, resp.url, "/events/"
        ) or check_subpage(session, resp.url, "/calendar/")

        # Tribe REST API
        if "wordpress" in tech or "tribe-events" in tech:
            result["tribe_api"] = check_subpage(
                session, resp.url, "/wp-json/tribe/events/v1/events"
            )

        # Tier classification
        if result["jsonld_event_count"] > 0:
            result["tier_guess"] = "T0"
        elif ("wordpress" in tech or "drupal" in tech) and result["events_subpage"]:
            result["tier_guess"] = "T1"
        elif any(
            t in tech for t in ("squarespace", "wix", "nextjs", "elementor", "webflow")
        ):
            result["tier_guess"] = "T2"
        elif result["events_subpage"] or result["candidate_urls"]:
            result["tier_guess"] = "T1"
        else:
            result["tier_guess"] = "SKIP"

    except requests.exceptions.Timeout:
        result["error"] = "timeout"
    except requests.exceptions.TooManyRedirects:
        result["error"] = "too_many_redirects"
    except Exception as e:
        result["error"] = str(e)[:80]

    return result


# ---------------------------------------------------------------------------
# Step 3: Output
# ---------------------------------------------------------------------------

TSV_HEADER = (
    "domain\tfinal_url\tstatus\ttech_stack\t"
    "jsonld_event_count\tevents_subpage\ttribe_api\ttier_guess\tcandidate_urls"
)


def result_to_tsv_row(r: dict) -> str:
    return "\t".join(
        [
            r["domain"],
            r["final_url"],
            str(r["status"]),
            r["tech_stack"],
            str(r["jsonld_event_count"]),
            "yes" if r["events_subpage"] else "no",
            "yes" if r["tribe_api"] else "no",
            r["tier_guess"],
            "|".join(r["candidate_urls"]),
        ]
    )


def print_summary(results: list):
    tiers = {"T0": [], "T1": [], "T2": [], "SKIP": []}
    errors = []

    for r in results:
        if r.get("error") and r["status"] == 0:
            errors.append(r)
        tiers[r["tier_guess"]].append(r)

    print("\n" + "=" * 70)
    print("RECON SUMMARY")
    print("=" * 70)
    print(f"Total probed: {len(results)}")
    print(f"  T0 (JSON-LD events found):          {len(tiers['T0'])}")
    print(f"  T1 (WP/Drupal + events subpage):    {len(tiers['T1'])}")
    print(f"  T2 (JS-heavy / hard to scrape):     {len(tiers['T2'])}")
    print(f"  SKIP (no event content / errors):   {len(tiers['SKIP'])}")
    print(f"  Probe errors:                       {len(errors)}")

    for tier_name, tier_results in [
        ("T0", tiers["T0"]),
        ("T1", tiers["T1"]),
        ("T2", tiers["T2"]),
    ]:
        if not tier_results:
            continue
        print(f"\n--- {tier_name} ---")
        for r in tier_results:
            jsonld = (
                f" [{r['jsonld_event_count']} JSON-LD]"
                if r["jsonld_event_count"]
                else ""
            )
            tribe = " [tribe-api]" if r["tribe_api"] else ""
            sub = " [/events/]" if r["events_subpage"] else ""
            print(f"  {r['name']:<45}  {r['domain']}{jsonld}{tribe}{sub}")

    if errors:
        print("\n--- PROBE ERRORS ---")
        for r in errors:
            print(f"  {r['name']:<45}  {r['error']}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main():
    parser = argparse.ArgumentParser(
        description="Recon Toronto arts/culture venue websites for scraping candidates."
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=None,
        help="Cap number of orgs probed (default: no limit)",
    )
    parser.add_argument(
        "--input",
        metavar="FILE",
        default=None,
        help="Newline-delimited list of URLs to probe instead of crawling t0ronto.ca",
    )
    args = parser.parse_args()

    configured_domains = load_configured_domains()
    print(
        f"Loaded {len(configured_domains)} already-configured domains to skip.",
        flush=True,
    )

    # --- Discover or load URLs ---
    if args.input:
        input_path = Path(args.input)
        raw_urls = [u.strip() for u in input_path.read_text().splitlines() if u.strip()]
        entries = []
        for u in raw_urls:
            parsed = urlparse(u)
            domain = parsed.netloc.lower().lstrip("www.")
            entries.append({"name": domain, "url": u, "domain": domain, "tags": []})
        print(f"Loaded {len(entries)} URLs from {args.input}", flush=True)
    else:
        entries = discover_t0ronto_urls(configured_domains)

    if args.limit:
        entries = entries[: args.limit]
        print(f"Limiting to {args.limit} orgs.", flush=True)

    if not entries:
        print("No URLs to probe. Exiting.")
        return

    # --- Probe concurrently ---
    print(
        f"\nProbing {len(entries)} URLs (max 10 workers, {TIMEOUT}s timeout each)...\n",
        flush=True,
    )
    results = []
    completed = 0

    with concurrent.futures.ThreadPoolExecutor(max_workers=10) as executor:
        future_to_entry = {executor.submit(probe_url, e): e for e in entries}
        for future in concurrent.futures.as_completed(future_to_entry):
            entry = future_to_entry[future]
            completed += 1
            try:
                result = future.result()
            except Exception as exc:
                result = {
                    "name": entry["name"],
                    "domain": entry["domain"],
                    "final_url": entry["url"],
                    "status": 0,
                    "tech_stack": "unknown",
                    "jsonld_event_count": 0,
                    "events_subpage": False,
                    "tribe_api": False,
                    "tier_guess": "SKIP",
                    "candidate_urls": [],
                    "error": str(exc)[:80],
                }
            results.append(result)
            tier = result["tier_guess"]
            err = f" ERROR: {result['error']}" if result.get("error") else ""
            print(
                f"[{completed:>3}/{len(entries)}] {tier:<4}  {result['domain']}{err}",
                flush=True,
            )

    # --- Write TSV ---
    tsv_lines = [TSV_HEADER]
    for r in results:
        tsv_lines.append(result_to_tsv_row(r))

    OUTPUT_FILE.write_text("\n".join(tsv_lines) + "\n")
    print(f"\nTSV written to: {OUTPUT_FILE}", flush=True)

    # --- Print summary ---
    print_summary(results)


if __name__ == "__main__":
    main()
