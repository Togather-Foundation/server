# Example Source Profiles

Patterns observed on a real Togather instance. Your sources will differ, but
these are instructive for recognizing common patterns.

## Academic talks (university source)

Massive recurring series by design. Weekly seminars scraped as separate events.
~165 items in a single pass, all with `cross_week_series_companion` warning.
Action: merge-into-primary or batch-approve as individuals by name. If the source
is confirmed legitimate and this pattern is permanent, the durable fix is
per-source `suppress_review_warnings: [cross_week_series_companion]` in the YAML
plus `server scrape sync`, then clear the items already queued — suppression only
affects future ingest. See
[Per-source review-warning suppression](https://github.com/Togather-Foundation/server/blob/main/docs/integration/scraper.md#per-source-review-warning-suppression).

## Comedy clubs

Recurring weekly shows. ~52 items, same weekly-series pattern as academic talks.
Action: batch-approve by source or by name substring; for a confirmed-legitimate
source, prefer per-source `suppress_review_warnings: [cross_week_series_companion]`
over approving the same flood every week (clear the existing queue once; it is not
retroactive).

## Performing arts venues

Cleanest sources. Orchestras, ballet companies, theatres produce structured data.
Only `missing_description` warnings. Action: batch-approve by source.

## Global ICS feeds (DISABLED)

Some scrapers ingest events from worldwide feeds with no city filter. Items from
New York, London, San Francisco end up in the queue. If a source produces only
wrong-city events, disable it in config and batch-reject its orphans.
