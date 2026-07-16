# Example Source Profiles

Patterns observed on a real Togather instance. Your sources will differ, but
these are instructive for recognizing common patterns.

## Academic talks (university source)

Massive recurring series by design. Weekly seminars scraped as separate events.
~165 items in a single pass, all with `cross_week_series_companion` warning.
Action: merge-into-primary or batch-approve as individuals by name.

## Comedy clubs

Recurring weekly shows. ~52 items, same weekly-series pattern as academic talks.
Action: batch-approve by source or by name substring.

## Performing arts venues

Cleanest sources. Orchestras, ballet companies, theatres produce structured data.
Only `missing_description` warnings. Action: batch-approve by source.

## Global ICS feeds (DISABLED)

Some scrapers ingest events from worldwide feeds with no city filter. Items from
New York, London, San Francisco end up in the queue. If a source produces only
wrong-city events, disable it in config and batch-reject its orphans.
