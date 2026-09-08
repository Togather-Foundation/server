# Artsdata Reconciliation Fixtures

Golden fixtures for the Artsdata W3C reconciliation parity harness. These files are
recorded (not hand-authored) from the live `https://api.artsdata.ca/recon` endpoint
and `https://kg.artsdata.ca/resource/...` dereference endpoint.

## Why these exist

The hand-authored httptest mock in `tests/integration/reconciliation_test.go` asserted
shapes that do not match the real API (e.g. `"Art Gallery of Ontario"` → `score: 98.5,
match: true`). Nothing guarded mock↔real divergence. These fixtures, plus the replay
and live-compare tests, pin the real wire bytes and score semantics so mocks are updated
deliberately, never silently.

## Fixture format

One JSON file per HTTP exchange. Each fixture stores:

- `request`: method, full URL, path, `Accept`/`Content-Type` headers, the exact
  form-encoded `queries=...` body bytes (recon) or empty body (GET), and the structured
  queries that produced the body.
- `response`: status, content type, and the raw response body (re-indented, token-exact)
  so score/match semantics stay visible.

`provenance` is `live-recorded` (captured from the real API) or
`derived-from-docs-unverified` (authored from `docs/interop/artsdata.md` without a live
capture, pending refresh).

## Corpus

| Fixture | Query | Observed semantics |
|---|---|---|
| `recon-place-exact` | "Massey Hall" (schema:Place) | exact: `match:true`, score ~880, short id `K11-24` |
| `recon-place-partial` | "Art Gallery of Ontario" (schema:Place) | partial only: `match:false`, score ~3-5 — the old mock wrongly assumed an exact match here |
| `recon-place-nomatch` | "Definitely No Such Venue Xyzzy" (schema:Place) | the API returns low-score partials (`match:false`) rather than an empty result |
| `recon-org-exact` | "Canadian Opera Company" (schema:Organization) | exact: `match:true`, score ~1020, short id `K2-5143` |
| `dereference` | GET `https://kg.artsdata.ca/resource/K11-24` | 303 → `/entity.jsonld?uri=...` → 200 `application/ld+json` |

Key real-API facts encoded by these fixtures (see `docs/interop/artsdata.md` §3.1, §10):

- Scores are **unbounded**: exact matches ~1000+, partial matches ~3-12. Normalized via
  `normalizeArtsdataScore` (`match:true` → 0.99, else `score/15` capped at 0.95).
- Results return **short IDs** (`K\d+-\d+`), expanded to full URIs by `expandArtsdataID`.
- The client must **never send a `properties` array** — Artsdata returns HTTP 500.
  This is asserted on the wire in both record and replay.

## Known client drift (not yet fixed)

The real dereference body uses `id`/`type` (not `@id`/`@type`) and `{"@none": ...}`
objects for `streetAddress`/`addressLocality`/`addressRegion`/`addressCountry`, which the
client's `artsdata.EntityData`/`Address` structs do not decode. `Client.Dereference`
therefore returns a parse error against the real shape. The replay test asserts the
dereference wire request (method/path/`Accept: application/ld+json`) and records the body
so the client can be fixed to parse it; it does not assert dereference parse success.

## Refreshing fixtures

```bash
# Re-record from the live API (<=1 rps, ~5 calls). Review the diff before committing.
ARTSDATA_RECORD=1 scripts/agent-run.sh go test ./internal/kg/artsdata/ -run TestArtsdataRecord -count=1

# Re-fetch and diff against the committed fixtures (fails loudly on drift).
ARTSDATA_LIVE_COMPARE=1 scripts/agent-run.sh go test ./internal/kg/artsdata/ -run TestArtsdataLiveCompare -count=1

# Replay (default CI, no network).
scripts/agent-run.sh go test ./internal/kg/... -run 'TestArtsdataReplayParity|TestReconciliationServiceParity' -count=1
```

The recording and live-compare modes are opt-in (env-gated) and never run in CI.
