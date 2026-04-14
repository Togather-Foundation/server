# Testing

## Commands

```bash
go test ./internal/storage/postgres   # needs live DB (see .env)
make e2e                               # browser E2E tests; requires running server + uvx
```

Run targeted package tests first; expand to `make ci` only when needed.

For E2E details see `tests/e2e/AGENTS.md`.

## Patterns

**Fault injection:** for packages that call `os.*` directly (like `internal/fileutil`), introduce an unexported interface (e.g. `atomicFS`) with a `defaultFS` production impl and a `failFS` test impl. Use same-package tests (`package foo`, not `package foo_test`) to access the seam. See `internal/fileutil/atomicwrite.go` + `atomicwrite_fault_test.go` as a reference.

**Wire-bytes assertions for serializers:** when testing any package that serializes to a wire format (ICS, JSON-LD, Turtle, etc.), assert the raw output bytes directly *before* feeding them back through a parser. A roundtrip test (`serialize → parse → compare`) passes even when the wire output is malformed, because the parser undoes the corruption symmetrically. Use `bytes.Contains` / `bytes.Index` on `result.Data` to confirm the exact on-wire encoding. Example: `internal/ical/serialize_compat_test.go` (`TestCompatMatrix_ICSEscaping`).

**Library escaping — read before wrapping:** when calling a library method that accepts a "text" value (e.g. `SetSummary`, `SetDescription`), check whether the library already applies wire-format escaping internally before adding your own. Double-escaping is invisible to roundtrip tests and produces garbage on the wire. The pattern: pass raw strings, let the library escape; only add custom escaping when the library explicitly documents that it accepts pre-escaped input.

**Golden fixtures must be generated, not hand-authored:** fixtures in `tests/testdata/` should be produced by running the serializer under test, not written by hand. Hand-authored fixtures silently diverge from actual output and give false confidence. If a `-update` flag pattern isn't in place yet, generate the fixture once and commit it — then review the diff carefully. See `tests/testdata/ics/README.md` for the naming convention.

## Staging Constraints

**`example.com` and any `*.example.com` subdomain URLs are a hard ingest error (HTTP 422)** in staging and production (all RFC 2606 reserved test domains are blocked). Tests or test harnesses that submit fixture events using those domains **must** set `ValidationConfig{AllowTestDomains: true}`. This field is never set via an env var — it is test-only and must be set explicitly. `server generate` and `cmd/loadtest` inject these placeholder URLs; never ingest their output against staging without source-tagging.

**Load-test cleanup on staging:** use `server cleanup loadtest --env=staging --source-id=<uuid> --confirm` (preferred) or `--legacy` for pre-tagging contamination. See `docs/operations/load-testing.md` for full workflow.
