package events

// Feed-scoped event identity.
//
// A feed event's identity is the pair (feed URL, sourceEventID) — the source's
// own id for the event, scoped to the feed that published it. It is
// deliberately NOT scoped to a `sources` row: a feed used to acquire several
// source rows (the `sources` upsert key is `(host-only base_url, name)`, so
// ingesting one feed under two config names created two rows — ingest now reuses
// the row that owns the feed), and the same feed event then looked like a
// brand-new event under the second row. The feed URL is the thing that actually
// identifies the feed, and it is stored per attribution in
// `event_sources.source_url` in the normalised form described below.
//
// Feed URL form: the feed is `scheme://host/path`. `source.url` goes through the
// same validation normalisation as every other URL (`normalize.go`), which strips
// the query string and fragment before anything is stored or compared. So:
//
//   - `.../rss/events?types=A` and `.../rss/events?programs=B` are the SAME feed.
//     Filter-scoped configs over one catalogue collapse to one identity, and an
//     event returned by several of them becomes one event with several
//     attributions. That is deliberate: the five TPL BiblioCommons configs are
//     views over one library catalogue, and separating them would duplicate
//     thousands of events (5,981 of 6,188 TPL events are currently seen by more
//     than one TPL config). The risk this accepts is an upstream that reuses its
//     event ids per query-scoped feed; no current source does, and
//     TestSourceConfigs_FeedIdentityCollisions makes any collapse explicit.
//   - the path is significant: two different paths on one host are two feeds;
//   - an uppercase host only matches the same spelling (this field is not
//     lowercased by validation);
//
// Comparison rule: URLs are compared verbatim in their normalised form, except
// that trailing slashes are ignored on both sides. The comparison lives in SQL
// (`rtrim(es.source_url, '/') = rtrim($1, '/')`, Repository.FindByFeedEventID)
// so rows written before the rule — many of which carry a trailing slash — still
// match without a backfill. `https://example.com/eventslist/` therefore matches
// `https://example.com/eventslist`, in either direction.
//
// Conflict rule: the feed that owns the id is authoritative for the NAME. Every
// other field keeps the trust-based auto-merge in AutoMergeFields, and ingest
// does not move dates at all (the occurrence rows are untouched on this path).
// So a rename in the feed renames the event — including over an admin edit,
// which is a known temporary conflict tracked by
// specs/006-curator-protection/plan.md — but it does not push any other field
// past what trust allows.
//
// Tombstones: the repository lookup prefers a live twin when one exists, but it
// does not resolve events.merged_into_id. MergeEvents soft-deletes a duplicate
// without moving its event_sources rows, so after a consolidation the tombstone
// can be the only row matching a key. That is the open gap also tracked by
// specs/006-curator-protection/plan.md; filtering deleted events is not the fix,
// because it would re-create occurrences that were deliberately retired.
//
// Negative control: the same sourceEventID from a genuinely different feed is a
// different event. "Different feed" here means a different host or path — a
// different query string is NOT a different feed (see the URL form above).
// Two feeds may legitimately reuse the same id namespace
// (e.g. every Google Calendar event id embeds its calendar's domain).
