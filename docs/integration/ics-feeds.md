# ICS Feed Operations Runbook

**Version:** 1.0
**Date:** 2026-04-14
**Status:** Implemented

This runbook covers onboarding, configuring, and troubleshooting ICS (iCalendar) feed
sources in the Togather SEL scraper. It is intended for operators adding new ICS sources
and debugging existing ones. For general scraper operations, see
[scraper.md](scraper.md). For ICS platform detection heuristics, see the
[ICS Feed Discovery](event-platforms.md#ics-feed-discovery) section of the platform
knowledge base.

## Table of Contents

1. [Source Config Setup](#1-source-config-setup)
2. [Syncing to the Database](#2-syncing-to-the-database)
3. [Triggering a Scrape Run](#3-triggering-a-scrape-run)
4. [Diagnostics](#4-diagnostics)
5. [Common Warning Types and Remediation](#5-common-warning-types-and-remediation)
6. [Recurrence Sanity Checks](#6-recurrence-sanity-checks)
7. [Troubleshooting Checklist](#7-troubleshooting-checklist)

## 1. Source Config Setup

Create a YAML file in `configs/sources/` with `extraction_method: ics` to activate ICS
mode. When ICS mode is active, the scraper fetches the URL as an ICS feed and uses the
`ical` package for parsing and mapping. The T0–T3 tier dispatch is bypassed entirely —
`tier`, `selectors`, `headless`, `graphql`, and `rest` fields are all ignored.

### Example YAML Config

```yaml
name: "my-venue-ics"
url: "https://example.com/events/?ical=1"
extraction_method: ics
schedule: "daily"
trust_level: 5
license: "CC0-1.0"
timezone: "America/Toronto"
enabled: true
```

### Config Fields for ICS Sources

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | **required** | Unique identifier (used in `server scrape source <name>`) |
| `url` | string | **required** | ICS feed URL |
| `extraction_method` | string | `""` | Must be `"ics"` to activate ICS mode |
| `schedule` | string | `"manual"` | `"daily"`, `"weekly"`, or `"manual"` |
| `trust_level` | int | `5` | SEL source trust score (1–10) |
| `license` | string | `""` | License applied to ingested events (default `"CC0-1.0"` when empty) |
| `timezone` | string | `""` | IANA timezone for date fallback (overrides `DEFAULT_TIMEZONE` env var) |
| `enabled` | bool | `true` | Set `false` to disable without deleting |

ICS sources also support `default_location` for venues where every event occurs at the
same location and the ICS feed does not include structured location data:

```yaml
name: "my-venue-ics"
url: "https://example.com/events/?ical=1"
extraction_method: ics
schedule: "daily"
trust_level: 5
license: "CC0-1.0"
timezone: "America/Toronto"
default_location:
  name: "My Venue"
  street_address: "123 Main St"
  address_locality: "Toronto"
  address_region: "ON"
  postal_code: "M5V 3A9"
  address_country: "CA"
  latitude: 43.6532
  longitude: -79.3832
enabled: true
```

YAML configs live in `configs/sources/`. See [scraper.md](scraper.md) for the full
source configuration reference.

## 2. Syncing to the Database

After creating or editing a YAML config, you **must** sync it to the database. The
scraper reads from the `scraper_sources` table at runtime, not from YAML files.

```bash
server scrape sync
```

This command reads all `configs/sources/*.yaml` files and upserts them into the
`scraper_sources` table. It reports created and updated counts.

**Important:** The `enabled` field is only applied on initial insert. Existing DB rows
retain their current `enabled` value across syncs. To re-enable a disabled source,
update the DB row directly (admin UI or SQL), not the YAML file.

For full sync behavior details, see [scraper.md — Config Storage](scraper.md#config-storage-yaml--database).

## 3. Triggering a Scrape Run

### Test before committing (dry-run)

```bash
server scrape source <name> --dry-run --verbose
```

`--dry-run` displays extracted events without submitting them. `--verbose` shows
individual event details and quality warnings.

To test with a YAML file before syncing to the database:

```bash
server scrape source --source-file configs/sources/my-venue-ics.yaml --dry-run --verbose
```

### Live run

```bash
server scrape source <name>
```

Scrapes the named source and submits ingested events via the batch API.

### Scrape all enabled sources

```bash
server scrape all
```

Scrapes all enabled sources sequentially. ICS sources are included automatically
based on their `extraction_method` setting.

### Periodic scheduling

Sources with `schedule: "daily"` or `schedule: "weekly"` are automatically scraped by
the River background worker (`ScrapeSourceWorker`) registered at server startup. No
manual trigger is needed for scheduled sources — they run automatically when the
`auto_scrape` global config flag is `true` (the default).

## 4. Diagnostics

Two admin API endpoints provide scraper diagnostics:

### All-source diagnostics

```bash
curl -H "Authorization: Bearer $TOGATHER_ADMIN_TOKEN" \
  "$TOGATHER_BASE_URL/api/v1/admin/scraper/diagnostics"
```

Returns recent scraper runs across all sources. Supports a `?limit=N` query parameter
(default: 20, max: 100).

### Per-source diagnostics

```bash
curl -H "Authorization: Bearer $TOGATHER_ADMIN_TOKEN" \
  "$TOGATHER_BASE_URL/api/v1/admin/scraper/sources/<name>/diagnostics"
```

Returns the latest run, last successful run (if the latest failed), and a list of
recent runs for a specific source. Supports `?limit=N` (default: 10, max: 100).

### Key response fields

| Field | Description |
|-------|-------------|
| `latest_run.status` | `running`, `completed`, or `failed` |
| `latest_run.events_found` | Total events found in the latest run |
| `latest_run.events_new` | New events ingested |
| `latest_run.events_dup` | Duplicate events skipped |
| `latest_run.events_failed` | Events that failed to ingest |
| `latest_run.error_message` | Error detail if the run failed |
| `latest_run.metadata` | JSONB with run-specific metadata |
| `last_successful_run` | Most recent completed run (for comparison when latest failed) |
| `recent_runs` | Array of recent runs for trend analysis |

For staging environments, source credentials from `.agent-keys/staging`:
```bash
source .agent-keys/staging
# Provides TOGATHER_BASE_URL and TOGATHER_ADMIN_TOKEN
```

See [env-management.md](../deploy/env-management.md) for token refresh procedures.

## 5. Common Warning Types and Remediation

The ICS parser emits warnings during both dry-run and live runs. Warnings are logged to
stderr and attached to the `ScrapeResult` as `QualityWarnings`. They do **not** prevent
ingestion — they are advisory signals for debugging.

| Warning | Cause | Remediation |
|---------|-------|-------------|
| `expected text/calendar, got <type>` | Feed URL returns HTML error page instead of ICS | Verify the URL returns a valid ICS feed with `curl -H "Accept: text/calendar" <URL>`. Check if auth is required. |
| `VTIMEZONE missing for TZID: <tz>` | Feed uses `TZID` references without defining a `VTIMEZONE` component | The parser maps common Windows timezone names via `WindowsTZIDAliases` (e.g. `"Eastern Standard Time"` → `"America/New_York"`). If the TZID is not in the alias map, file a parser follow-up to add it. |
| `missing SUMMARY (UID: <uid>)` | VEVENT has no `SUMMARY` property (event title) | Normal — valid feeds sometimes contain empty or cancelled events that are skipped. Verify that valid events still ingest. |
| `missing DTSTART (UID: <uid>)` | VEVENT missing required `DTSTART` property | Expected for malformed entries. The event is skipped. Verify that valid events still ingest. |
| `body exceeds N bytes limit` | Feed body exceeds the `ICS_MAX_BODY_BYTES` limit | Increase `ICS_MAX_BODY_BYTES` env var (default: 10 MB / 10485760) or split the source into smaller feeds. |
| `duplicate UID (UID: <uid>)` | Two VEVENTs share the same UID | Check the upstream feed for duplicates. Provenance dedup handles this — the second occurrence is skipped. |
| `RRULE capped at N occurrences (UID: <uid>)` | RRULE expansion exceeded `ICS_MAX_OCCURRENCES` limit | Increase `ICS_MAX_OCCURRENCES` env var (default: 100) if more occurrences are needed, or widen `ICS_HORIZON_DAYS` (default: 90). |
| `unparseable EXDATE (UID: <uid>)` | EXDATE value has an invalid date format | The feed has malformed date exclusions. The EXDATE is skipped; the event is still ingested without the exclusion. |

## 6. Recurrence Sanity Checks

After ingesting a feed containing RRULE events, verify that an `event_series` row was
created for each recurring event.

### Check event_series rows

```sql
SELECT id, rrule, series_start_date, series_end_date, schedule_timezone
FROM event_series
WHERE name ILIKE '%<source-name>%'
ORDER BY series_start_date;
```

### JSON-LD eventSchedule requirements

The `eventSchedule` JSON-LD field in API responses requires both `series_start_date` and
`series_end_date` to be non-nil in the `event_series` row. If `eventSchedule` is missing
from the API response for a recurring event:

1. Check that `series_start_date` is populated — it must be set on the `event_series`
   row directly (ICS ingest does not auto-populate this field).
2. Check that `series_end_date` is populated — it must also be set on the `event_series`
   row directly (ICS ingest does not auto-populate this field).
3. If either is `NULL`, the `RecurrenceRule` struct will have nil `SeriesStart` or
   `SeriesEnd` fields, and the API omits `eventSchedule` from the response.

**Why are these NULL after ICS ingest?** The ICS extraction pipeline stores the raw
`RRULE` string and `EXDATE` list on the `event_series` row, but it does not parse the
`DTSTART`/`UNTIL`/`COUNT` combination to compute series bounds. This is a known gap —
the ingest pipeline intentionally avoids pre-computing derived dates to keep the mapper
stateless.

**Backfill when needed:** once you know the intended series bounds, set them directly:

```sql
UPDATE event_series SET
  series_start_date = '2026-07-06',
  series_end_date   = '2026-08-31'
WHERE id = '<series-uuid>';
```

Find the `event_series` UUID for a given event:

```sql
SELECT e.ulid, es.id AS series_id, es.rrule,
       es.series_start_date, es.series_end_date
FROM events e
JOIN event_series es ON es.id = e.series_id
WHERE e.ulid = '<event-ulid>';
```

The relevant Go struct is `RecurrenceRule` (`internal/domain/events/repository.go`):
```go
type RecurrenceRule struct {
    RRule       string
    ExDates     []time.Time
    RDates      []time.Time
    TZID        string
    SeriesStart *time.Time  // series_start_date from event_series (nil if absent)
    SeriesEnd   *time.Time  // series_end_date from event_series (nil if absent)
}
```

## 7. Troubleshooting Checklist

1. **Source is enabled in the database** — not just in YAML. Check with:
   ```bash
   server scrape list
   ```
   The `ENABLED` column should show `true`. Remember: `server scrape sync` does not
   overwrite the `enabled` field for existing rows.

2. **`server scrape sync` was run after editing YAML** — the scraper reads from the
   `scraper_sources` DB table at runtime, not from YAML files. After any YAML change:
   ```bash
   server scrape sync
   ```

3. **ICS URL returns valid `text/calendar` content** — test the feed directly:
   ```bash
   curl -H "Accept: text/calendar" "<ICS-URL>"
   ```
   The response should start with `BEGIN:VCALENDAR` and have a `Content-Type` header
   containing `text/calendar` or `application/ics`.

4. **Feed URL is not auth-protected** — ICS feeds must be publicly accessible. If the
   URL returns an HTML login page, the scraper will emit an
   `expected text/calendar, got text/html` warning. Use T1/T2 or T3 scraping instead.

5. **Check scraper diagnostics** — look at the latest run status and warnings:
   ```bash
   curl -H "Authorization: Bearer $TOGATHER_ADMIN_TOKEN" \
     "$TOGATHER_BASE_URL/api/v1/admin/scraper/diagnostics"
   ```

6. **Events ingest but don't appear in API** — check `lifecycle_state`. Newly ingested
   events may have `lifecycle_state: pending_review` and require admin approval before
   appearing in public API responses.

7. **Recurrence is wrong** — check `event_series.rrule` and `exdates` columns:
   ```sql
   SELECT id, rrule, exdates, series_start_date, series_end_date
   FROM event_series
   WHERE name ILIKE '%<source-name>%';
   ```
   Ensure `series_start_date` and `series_end_date` are both non-NULL (see
   [Recurrence Sanity Checks](#6-recurrence-sanity-checks)).

## 8. community-calendar: Reference & Troubleshooting Aid

[community-calendar](https://github.com/judell/community-calendar) is an open-source
project by Jon Udell that aggregates ICS feeds from community organisations in multiple
cities, including Toronto. It is the primary reference for the Toronto ICS source
inventory in this project.

### What it provides

| Resource | URL | Notes |
|----------|-----|-------|
| Toronto feed list | `cities/toronto/feeds.txt` | One feed URL per line with display name comments. Authoritative slug source for all 51 Meetup groups. |
| Feed health report | `report.json` (root) | Generated daily via GitHub Actions. Shows event count and error per feed. |
| GitHub Actions log | `.github/workflows/` | Shows fetch cadence and last-run status for every feed. |

### When to use it

**Finding the correct Meetup group slug.** All 51 Toronto Meetup slugs used in
`toronto-ics-manifest.json` were sourced from `feeds.txt`. If a Meetup feed returns
404 and you need to verify the slug:

```bash
# Check the canonical slug in feeds.txt
curl -s https://raw.githubusercontent.com/judell/community-calendar/main/cities/toronto/feeds.txt \
  | grep -i "<group-name>"
```

**Checking whether a feed is live before debugging locally.** community-calendar
fetches all Toronto feeds daily via GitHub Actions. If a feed shows `count > 0` in
`report.json`, the URL is working at least from GitHub's infrastructure:

```bash
# Check event count and error status for a specific source
curl -s https://raw.githubusercontent.com/judell/community-calendar/main/report.json \
  | python3 -c "import json,sys; r=json.load(sys.stdin); [print(k,v) for k,v in r.items() if 'meetup' in k.lower() and 'toronto' in k.lower()]"
```

A `count > 0` with no `error` field means the feed is live. A local 404 is then
almost certainly an IP rate-limit or User-Agent issue — **not** a broken URL.

**Discovering new Toronto sources.** If you suspect a Toronto organisation has a
public ICS feed, check `feeds.txt` first before attempting manual discovery. The file
also includes non-Meetup sources (Tockify, Google Calendar, WordPress, Eventbrite
bridge, static `.ics` files) that may not be obvious from a site visit.

### Known discrepancies vs. our manifest

| Source key | community-calendar entry | Difference |
|------------|--------------------------|------------|
| `meetup-dance-go-latin` | `golatindance.com/events/?ical=1` | Not a Meetup group — WordPress ICS. `feed_type` corrected to `wordpress_tribe` in manifest (2026-04-15). |

### Cautionary notes

- community-calendar fetches from GitHub Actions runners (AWS us-east-1 IPs). Meetup
  may respond differently to other IP ranges — a feed appearing healthy in `report.json`
  does not guarantee it will work from every IP. Test from staging before concluding a
  feed is broken.
- `report.json` is regenerated on each run and not versioned. Check the
  [GitHub Actions run history](https://github.com/judell/community-calendar/actions)
  for historical data.
- `feeds.txt` is maintained manually — it may be incomplete or have stale slugs for
  groups that have renamed. Always verify a URL returns `BEGIN:VCALENDAR` before
  creating a source config.

## See Also

- [scraper.md](scraper.md) — general scraper operations, CLI reference, config format
- [event-platforms.md](event-platforms.md) — ICS discovery heuristics (§ ICS Feed Discovery)
- [openapi.yaml](../api/openapi.yaml) — API reference