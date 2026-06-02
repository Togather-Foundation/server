# Testing

## Commands

```bash
go test ./internal/storage/postgres   # needs live DB (see .env)
make e2e                               # browser E2E tests; requires running server + uvx
```

Run targeted package tests first; expand to `make ci-fast` for quick CI feedback, or `make ci` for full verification.

For E2E details see `tests/e2e/AGENTS.md`.

## Shared Test Helpers (`tests/testhelpers`)

The `tests/testhelpers` package provides shared utilities for `integration`, `integration_batch`, and `contracts` test packages. **Always use these instead of duplicating them.**

| Function | Signature | Purpose |
|---|---|---|
| `TestLogger()` | `() zerolog.Logger` | Returns a no-op logger |
| `TestConfig(dbURL)` | `(string) config.Config` | Standard test config (AllowTestDomains, high rate limits, etc.) |
| `ProjectRoot(t)` | `(*testing.T) string` | Repo root via `runtime.Caller` — **must** be called from within the `tests/testhelpers` package; if you need the repo root from another package, define a local `projectRoot(t)` that wraps `runtime.Caller(0)` there |
| `ResetDatabase(t, pool)` | `(*testing.T, *pgxpool.Pool)` | Truncates all public tables + restarts sequences |
| `MigrateWithRetry(dbURL, path, timeout)` | `(string, string, time.Duration) error` | Runs migrations with retry loop |
| `InsertAPIKey(t, pool, ctx, name)` | `(*testing.T, *pgxpool.Pool, context.Context, string) string` | Inserts a random API key using SHA-256 (not bcrypt). Returns the raw key. |
| `InsertAdminUser(t, pool, ctx, ...)` | `(*testing.T, *pgxpool.Pool, context.Context, string×4)` | Inserts a user with bcrypt-hashed password (~100ms — use sparingly) |

**Key design notes:**

- `InsertAPIKey` uses `crypto/rand` (16 bytes → 32 hex chars) for fully random keys and SHA-256 hashing. Never use ULID-based key generation in tests — ULID prefixes collide within the same millisecond at test speed, causing unique constraint failures. Never use `auth.HashAPIKey` (bcrypt cost 12, ~300ms/call) in tests — it accumulated to test suite timeouts.
- `ProjectRoot` uses `runtime.Caller(1)` relative to the testhelpers file. If you call it from a different test package, define your own local `projectRoot(t)` using `runtime.Caller(0)` so the path resolves correctly from your file.
- `TestConfig` sets `AllowTestDomains: true` — fixture generators (`RandomEventInput`, etc.) no longer produce `example.com` URLs, but the flag is still required because integration tests construct `example.com` URLs inline and `server generate`/`cmd/loadtest` continue to inject placeholder domains. The `integration` package extends it locally to add `DeveloperConfig.UsageFlushTimeoutSeconds: 2`.
- All helpers take explicit `pool`/`ctx` args (not a package-specific `testEnv` struct) so they work across packages.

## Patterns

**Fault injection:** for packages that call `os.*` directly (like `internal/fileutil`), introduce an unexported interface (e.g. `atomicFS`) with a `defaultFS` production impl and a `failFS` test impl. Use same-package tests (`package foo`, not `package foo_test`) to access the seam. See `internal/fileutil/atomicwrite.go` + `atomicwrite_fault_test.go` as a reference.

**Wire-bytes assertions for serializers:** when testing any package that serializes to a wire format (ICS, JSON-LD, Turtle, etc.), assert the raw output bytes directly *before* feeding them back through a parser. A roundtrip test (`serialize → parse → compare`) passes even when the wire output is malformed, because the parser undoes the corruption symmetrically. Use `bytes.Contains` / `bytes.Index` on `result.Data` to confirm the exact on-wire encoding. Example: `internal/ical/serialize_compat_test.go` (`TestCompatMatrix_ICSEscaping`).

**Library escaping — read before wrapping:** when calling a library method that accepts a "text" value (e.g. `SetSummary`, `SetDescription`), check whether the library already applies wire-format escaping internally before adding your own. Double-escaping is invisible to roundtrip tests and produces garbage on the wire. The pattern: pass raw strings, let the library escape; only add custom escaping when the library explicitly documents that it accepts pre-escaped input.

**Golden fixtures must be generated, not hand-authored:** fixtures in `tests/testdata/` should be produced by running the serializer under test, not written by hand. Hand-authored fixtures silently diverge from actual output and give false confidence. If a `-update` flag pattern isn't in place yet, generate the fixture once and commit it — then review the diff carefully. See `tests/testdata/ics/README.md` for the naming convention.

## Scraping Local Fixtures

`server scrape test-fixture <path>` serves a local fixture file and scrapes it in one command — no manual `python3 -m http.server` or hand-written YAML needed.

```bash
# ICS fixture (auto-detected: extraction_method=ics, tier=0)
server scrape test-fixture tests/testdata/ics/interop-recurrence-exdate.ics --dry-run

# With overrides
server scrape test-fixture tests/testdata/ics/interop-recurrence-exdate.ics --dry-run --verbose --source-name my-test

# Non-ICS fixture (tier 0 JSON-LD by default)
server scrape test-fixture /tmp/events.jsonld --dry-run
```

Flags: `--extraction-method`, `--tier` (default: auto-detect from extension), `--source-name`, `--trust-level` (default 5), `--headless`, plus all global scrape flags (`--dry-run`, `--verbose`, `--limit`, `--cache`).

## Staging Constraints

**`example.com` and any `*.example.com` subdomain URLs are a hard ingest error (HTTP 422)** in staging and production (all RFC 2606 reserved test domains are blocked). Tests or test harnesses that submit fixture events using those domains **must** set `ValidationConfig{AllowTestDomains: true}`. This field is never set via an env var — it is test-only and must be set explicitly.

**Fixture generators in `tests/testdata/fixtures.go` (`RandomEventInput`, `EventInputReversedDates`, `EventInputMissingVenue`, `EventInputLikelyDuplicate`, `EventInputMultipleWarnings`, review scenarios) no longer produce `example.com` URLs** — they use `source.BaseURL` (realistic domains like eventbrite.ca, meetup.com, squarespace.com) for event URLs and `unsplashImage` for images. Tests using these generators do not require `AllowTestDomains` on that basis alone.

**`server generate` and `cmd/loadtest` still inject `example.com` placeholder URLs** — never ingest their output against staging without source-tagging.

**Load-test cleanup on staging:** use `server cleanup loadtest --env=staging --source-id=<uuid> --confirm` (preferred) or `--legacy` for pre-tagging contamination. See `docs/operations/load-testing.md` for full workflow.
