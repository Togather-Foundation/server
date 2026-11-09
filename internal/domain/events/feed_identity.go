package events

// Feed-scoped event identity.
//
// A feed event's identity is the pair (feed URL, sourceEventID) — the source's
// own id for the event, scoped to the feed that published it. It is
// deliberately NOT scoped to a `sources` row: one feed can acquire several
// source rows (the `sources` upsert key is `(host-only base_url, name)`, so
// ingesting one feed under two config names creates two rows), and the same
// feed event then looks like a brand-new event under the second row. The feed
// URL is the thing that actually identifies the feed, and it is stored verbatim
// per attribution in `event_sources.source_url`.
//
// URL comparison rule: feed URLs are compared verbatim, except that trailing
// slashes are ignored on both sides. That is the whole rule — there is no
// canonicalisation of scheme or host case, no fragment stripping and no query
// rewriting, and the application never rewrites the stored URL. The comparison
// lives in the SQL (`rtrim(es.source_url, '/') = rtrim($1, '/')`,
// Repository.FindByFeedEventID) so that rows written before this rule — many of
// which carry a trailing slash — still match without a backfill. In particular:
//
//   - `https://example.com/eventslist/` (stored) matches
//     `https://example.com/eventslist` (incoming) and vice versa;
//   - an uppercase host or a fragment only matches another URL with the same
//     spelling, because both sides are compared as stored;
//   - the path and query are significant: two different feed URLs on one host
//     are two feeds.
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
// different event. Two feeds may legitimately reuse the same id namespace
// (e.g. every Google Calendar event id embeds its calendar's domain).
