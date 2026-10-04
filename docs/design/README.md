# Design Documents

Feature design docs and UI specifications for the Togather SEL server.

## Files

- **[developer-api-keys.md](developer-api-keys.md)** - Design spec for developer self-service API key management, including data model, authentication flows, and phased rollout plan (Status: Design, not yet implemented)
- **[duplicate-detection.md](duplicate-detection.md)** - Unified duplicate detection and review workflow design
- **[duplicate-review-scenarios.md](duplicate-review-scenarios.md)** - Comprehensive scenario coverage for the duplicate detection and review workflow
- **[review-queue-ui.md](review-queue-ui.md)** - UI design for the duplicate review queue interface
- **[webclaw-evaluation.md](webclaw-evaluation.md)** - Evaluation of webclaw (Rust extraction engine) as a Togather extraction sidecar (Status: **evaluated, not adopted**, 2026-10-03)

## Page-Fetching / Extraction Tool Evaluations

The scraper's fetch-and-extract tooling has been evaluated more than once, and every candidate was
**deliberately not adopted** (Ryan, 2026-10-03: no sidecar processes in Togather yet). The repo
keeps no external extraction engine wired in; the built-in Go tier system is unchanged. The
findings are kept because they name the specific defects that would justify another moving part.

- **[webclaw-evaluation.md](webclaw-evaluation.md)** — webclaw (Rust, AGPL-3.0): JSON-LD / `__NEXT_DATA__` extraction, TLS impersonation. Verdict: adopt-with-caveats as a Tier 0/1 JSON-LD sidecar (not actioned).
- **Scrapling** (Python, BSD-3) — StealthyFetcher / adaptive selectors. Verdict: sidecar for Tier 2 / Cloudflare walls; not actioned. Its static wins (uTLS impersonation, stealth browser flags) are already present natively via `refraction-networking/utls` + Rod.
- **fastCRW** (`us/crw`, Rust, AGPL-3.0) — local Lightpanda→Chrome render ladder, `map`/crawl. Verdict: second-engine candidate for Tier 2 / vendor-blocked sources; not actioned.

A single consolidated index — one entry per tool with what was tested, what was measured, and the
specific trigger that would reopen each — is maintained in the project research wiki alongside the
evaluation notes and the live validation spikes. Read it before proposing any of these tools again.

## Related Documentation

- **[Spec Kit](../../specs/)** - Feature specifications (source of intent)
- **[Admin UI Overview](../admin/admin-ui-overview.md)** - Design system and core concepts for implemented features
- **[Admin UI Components](../admin/admin-ui-components.md)** - Component library and JS patterns for implemented features
