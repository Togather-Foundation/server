# Event listing provenance and synthetic fixtures

**Spec 008** · **2026-11-20** · **Status: In Progress** · **Tasks t_8115fa06 and t_15acf905**

Independent review approved the contract with identity isolation, explicit URL
semantics and backward-compatible signed-run verification incorporated below.

## Context

An event has a SEL resource identifier, an optional organizer listing URL stored in
`events.public_url`, and zero or more contributing source attributions. Reviewers
and clients need to tell these apart. Development generators currently create
invented listings on real organizers’ and platforms’ domains. Generated data can
reach the same ingestion, review and public read paths as ordinary events.

## User scenarios and acceptance

1. Given an event with a listing URL, when a client reads the public event list
   or detail or an administrator lists events, the event exposes the stored
   listing as `url`. An absent listing is omitted. Its SEL identifier stays
   separate from both the listing and source attribution.
2. Given a review queue item, when a reviewer reads its list response or CLI
   output, `eventUrl` gives the stored public listing and `sourceUrl` gives the
   source registry’s feed URL (falling back to its base URL). Omit empty fields.
3. Given input containing `"sel:synthetic": true`, when it is ingested,
   reread from storage and returned in the public list or detail, the marker
   is true. Missing or false input stores false and the response omits the
   marker. Clients can exclude records with the marker set to true.
   Canonical `/events/{id}` representations and MCP event reads preserve the
   same marker and listing URL. Federation change feed snapshots expose the
   marker only when true, including on delete tombstones. Federation imports
   accept an optional Boolean and store false when omitted. A non-Boolean
   value, including `null`, returns the existing HTTP 400 JSON-LD validation
   problem. Syncing an existing `@id` with a different marker returns that
   same problem and rolls back all event and nested entity writes. Imports
   with the same classification can update the event.
4. Given generated and ordinary inputs sharing identity fields, when either
   is ingested or replayed, each retains its own event identity, marker and
   review state. Exercise both ingest orders and replay, pending and rejected
   source/external-ID matches, and recurrence external-key collisions. Existing
   ordinary event hashes continue to match.
5. Given any ordinary, recurring or review scenario from the fixture generator,
   when serialized, each event carries the marker. Invented listings, virtual
   venues and attribution links use hosts under the reserved `.invalid` suffix.
   New signed load-test runs use a run-specific path at
   `loadtest.togather.invalid`. Their version-2 signature binds the run to its
   target. Existing version-1 manifests can still verify
   and clean up their original unmarked payloads.

## Technical design and security

The ingest contract adds `Synthetic bool` with JSON tag
`json:"sel:synthetic,omitempty"` to event input and public JSON-LD models.
Migration 000064 adds a non-null Boolean with default false to `events`.
Legacy records retain that default. Normal event backups include the flag.
Synthetic provenance is declared by the submitter. Existing authentication,
validation, permissions and publication rules apply. Malformed marker types use
the existing JSON decoding error path. Automatic feed, hash, review, near-duplicate
and series matching respect the marker before applying side effects.
The pending-review source/external-ID unique index includes a stored Boolean
from the referenced event. Recurrence upserts store the supplied key in the
unique `event_series.external_key` column for ordinary events and in the new
nullable, unique `event_series.synthetic_external_key` for synthetic events.
The previous server can still upsert ordinary series during a blue/green
deployment. Generated links are identified without an outbound network request.
Historical classification and a server-side filter are outside this change.

## Implementation and verification

| Task | Test | Acceptance |
| --- | --- | --- |
| t_8115fa06 · Expose event and attribution URLs in listing and review projections | Exercise list representations with present, missing and distinct URLs | Public, administrative and review clients receive the documented values |
| t_15acf905 · Persist, serialize and generate marked events | Round-trip JSON, storage and generator cases, including identity collisions and repeat ingest | A marked fixture remains distinguishable after reading it through the API and ordinary events keep their identity |

No runtime configuration is required.
