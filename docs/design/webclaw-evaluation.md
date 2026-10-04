# webclaw Evaluation — Hermes tooling + Togather scraper

> **Status: evaluated, not adopted (2026-10-03).** Togather kept its existing extraction
> engines and runs no webclaw process; the recommendation below is a proposal, not a
> decision. Kept because the findings stay relevant if the parser gaps it targets ever
> outrank the cost of another moving part. Wider context lives in the research notes:
> the eight-tool scraping survey and the scrapling / fastcrw comparisons. The separate
> question of using webclaw as this workstation's local page-extraction engine is a
> Hermes-side matter, not a Togather one.


Evaluated: 2026-08-14 (repo state: v0.6.19, pushed 2026-08-10)
Repo: github.com/0xMassi/webclaw — Rust, AGPL-3.0, 2,189★ / 222 forks, 40.8k LOC across 7 crates, prebuilt Linux static binaries in releases.

## TL;DR

- **For ourselves (Hermes): YES — adopt as the local web-extraction engine.** Single static binary (~30MB, musl/gnu), local-first, MCP server included. It fills the exact gap where our `web_extract` is dead (search-only backend): a fast, safe, one-shot "URL → clean markdown/JSON-LD" tool with zero cloud dependency for normal pages.
- **For Togather: YES as a Tier 0/1 sidecar augmentation — NO as a scraper replacement.** Its JSON-LD extraction (with full Schema.org Event blocks) and `__NEXT_DATA__` SPA handling directly target Togather's two live pain points (mis-parsed address fields, React-shell sources), and the TLS fingerprint impersonation is a cheap anti-bot layer. But the real anti-bot moat (Cloudflare Turnstile / DataDome bypass, JS rendering) is **closed-source and hosted-only** — so the Tier 2 Rod path and the open StealthyFetcher option stay relevant.
- **License is fine for both** as long as it's a sidecar process (AGPL-3.0 vs Togather's GPL-3.0: HTTP interop between separate processes = no derivative work; embedding Rust into Go is impractical anyway).

## 1. What webclaw Is

"Turn websites into clean markdown, JSON, and LLM-ready context" — a local-first Firecrawl alternative. The repo is a Rust workspace:

| Crate | LOC | Role |
|---|---|---|
| webclaw-core | 14,680 | Extraction engine: main-content, markdown, JSON-LD, `__NEXT_DATA__`, SvelteKit islands |
| webclaw-fetch | 17,332 | HTTP client with TLS fingerprint impersonation, Cloudflare/DataDome detection, crawler, vertical extractors |
| webclaw-cli | 3,651 | CLI: scrape/crawl/map/search/watch/diff/batch |
| webclaw-llm | 1,983 | LLM structured extraction (JSON schema), summarize; providers: ollama, openai, anthropic, atlascloud |
| webclaw-mcp | 1,588 | MCP server (stdio, rmcp crate) |
| webclaw-server | 1,235 | Minimal self-host REST API (axum), Firecrawl-compatible shape |
| webclaw-pdf | 306 | PDF text extraction |

Own benchmark (v0.3.18, Apr 2026, 18 sites, 90 hand-curated facts): 84.4% fidelity / 92.5% token reduction vs trafilatura 50% and firecrawl 77.8% — vendor benchmark, treat as directional.

## 2. How It Works Under the Hood (code reality check)

**Anti-bot is TLS-layer impersonation, not a browser.** `browser.rs` maps BrowserProfile → client builder methods: Chrome/Firefox/Safari-iOS/Edge variants with matching HTTP/2 fingerprints, header order, UA. `tls.rs` manually strips `sec-fetch-*`/`priority` leaks. This defeats signature-based bot detection (basic Cloudflare, DataDome-class rules) with no browser overhead. The repo is honest about it: the *full* antibot/CDP pipeline and JS rendering are **closed-source**, living behind `api.webclaw.io`. The OSS `cloud.rs` detects challenge pages (public CF/DataDome signatures), and `smart_fetch` escalates to the paid hosted API when hit — no key = clear "signup required" error, nothing silent. Verified: zero chromium/playwright deps in the OSS tree; the only CDP reference is "the cloud does CDP".

**Extraction is genuinely good for events:**
- `structured_data.rs`: walks all `<script type="application/ld+json">` blocks with newline-sanitizing JSON parse (handles broken JSON-LD), plus `__NEXT_DATA__` (Next.js SSR data) and SvelteKit island data — i.e. many "React SPA" sources are covered at the static-HTML level, no browser needed.
- Local `/v1/scrape` returns `structured_data` array in JSON output (verified in `routes/scrape.rs`).
- `--format llm` = token-optimized text; `--extract '{json schema}'` = LLM structured extraction (works with local ollama, no key required).

**Self-host server** (`webclaw-server`): stateless, single binary, localhost-bound by default, optional bearer token, routes: `/v1/scrape`, `/scrape/{vertical}`, `/crawl`, `/map`, `/search`, `/batch`, `/extract`, `/extractors`, `/summarize`, `/diff`, `/brand`. Hosted-only features (anti-bot, JS render, async crawl jobs, multi-tenant, billing) are explicitly "not implemented here and never will be — closed-source."

**Vertical extractors:** shopify product/collection, ebay listing, etsy listing, woocommerce product, substack post, trustpilot reviews, github release. No Event/Eventbrite vertical, but generic JSON-LD returns Event schema blocks.

## 3. Match Assessment

### For Hermes ("ourselves")

**What would work well:**
- Our `web_extract` is backend-crippled (search-only ddgs → silently returns nothing). webclaw gives a local, deterministic replacement: `webclaw <url> --format llm` or the MCP server. This directly answers the standing ask ("single script for smart+safe web extract, check if something already exists") — it exists.
- Local-first, zero token cost, no API key for normal pages. Static musl binary → no Rust toolchain needed.
- JSON-LD extraction means article/dataset/Event pages come back structured without an LLM pass.
- MCP server (stdio) wires straight into Hermes' native MCP client (config.yaml).

**What doesn't fit:**
- Pages behind hard anti-bot (Turnstile, DataDome) need the paid cloud — same wall as any hosted extractor. Acceptable: those are the rare case, and `smart_fetch` fails loudly.
- AGPL: irrelevant for internal tooling (no distribution).

### For Togather (comparative vs the bespoke Go scraper + prior Scrapling eval)

**What would work well:**
- **JSON-LD-first extraction maps exactly to Tier 0** (currently goquery JSON-LD). webclaw's parser is more robust (newline sanitize, all-block walk, `__NEXT_DATA__`/SvelteKit). Event schema blocks carry `startDate`, `location`, `offers` — the fields Togather's address parser keeps mis-assigning (CITY-03 pain: postal codes / org names landing in `addressLocality`). Feeding structured JSON-LD instead of raw HTML would bypass that whole defect class.
- **`__NEXT_DATA__` coverage eats into Tier 2** (Rod/headless): React SSG sites (e.g. TSO-class) yield data from static HTML, no browser, no Rod cost.
- **TLS impersonation** is a free anti-bot layer for the ~68 Tier 1 (Colly) sources and ~38 Tier 0 — better than plain Go stdlib, no proxy needed for signature-based blocks.
- Sidecar fits the architecture: single binary, REST API, optional bearer token; `scraper.go` gains one "webclaw tier" calling localhost.
- License: AGPL-3.0 sidecar vs GPL-3.0 server = separate processes, clean.

**What doesn't fit:**
- **Hard anti-bot is paid-cloud-only.** For the ~28 Tier 2 sources needing real bypass, the open answer remains Scrapling's StealthyFetcher (fully open, BSD-3) or keep Rod. webclaw does not replace either.
- **No adaptive selectors** — Scrapling's `auto_save`/`adaptive` element tracking (redesign survival) has no webclaw equivalent; Tier 1 redesign breakage remains.
- **Language mismatch** Rust vs Go → HTTP sidecar only, no library embedding (also avoids the AGPL-combined-work question entirely).
- No event-specific vertical extractor; ICS/sitemap tiers are untouched (webclaw has crawl/sitemap but Togather's ICS is its own thing).

### Scrapling vs webclaw (both evaluated as sidecars)

| Dimension | Scrapling (Jun 5 eval) | webclaw |
|---|---|---|
| License | BSD-3 (permissive) | AGPL-3.0 (copyleft, OK as sidecar) |
| Anti-bot | StealthyFetcher open, Turnstile bypass | TLS impersonation open; real bypass = paid cloud |
| Extraction | Python stack | JSON-LD/`__NEXT_DATA__` first — better for Event schema |
| Adaptive selectors | Yes | No |
| MCP | Yes | Yes |
| Ops | Python env + playwright install | Single static binary |
| Best for | CF-protected sources, redesign-prone sites | JSON-LD/Event-heavy sources, agent-driven extraction, token-efficient markdown |

They're complementary, not rivals. If anything gets adopted, it's webclaw for the JSON-LD/Tier-0-adjacent path and Scrapling for the CF-bypass path — or neither until a spike proves value.

## 4. What to Steal (regardless)

- **The thin-body escalation rule** (latency.md): HTTP 200 + <10KB HTML or <50 extracted words = "not the real page" → re-fetch through browser. A cheap, principled detector Togather's scraper could adopt to route failing sources automatically.
- **The honest self-host/cloud split**: "what self-hosting gives you vs what the cloud gives you" documented as a first-class matrix in the server README. Mirror for any future Togather hosted/OSS boundary.
- **Benchmark methodology**: hand-curated visible facts per site, committed as data (PR-able), token-efficiency always paired with fidelity, and the "fast failures win races" A/B trap — genuinely good measurement discipline for evaluating Togather's own extractor quality.
- **Proxies**: CLI accepts socks5/http proxy URLs and files with per-request rotation — a feature Togather's per-node scraper will eventually want.

## 5. Recommendation

1. **Hermes: adopt now, cheap spike.** Download the x86_64 musl binary, try `--format llm` + JSON-LD on a few sites, wire the MCP server into config.yaml. If the spike passes, it becomes our `web_extract` fallback for the search-only-backend failure mode.
2. **Togather: evaluate as sidecar, don't commit yet.** Run webclaw against 5-10 Tier 0 sources (eventbrite-class, museum, festival) and diff its `structured_data` JSON-LD vs the current goquery parse — specifically whether Event blocks carry clean `startDate`/`location` that would have dodged the CITY-03 mis-parsing. If clean, add a "webclaw sidecar" tier for JSON-LD-heavy sources and route the thin-body/escalation rule at failing sources. Keep Rod and/or Scrapling StealthyFetcher for hard-bot Tier 2. File as a kanban evaluation ticket (research-only, worker-safe).
3. **Don't** adopt it as a general crawl replacement (no adaptive selectors, crawl is depth/config based, not site-config based like Togather's YAML sources).

## 6. Sources
- Repo + code inspection: github.com/0xMassi/webclaw (cloned v0.6.19, /tmp/webclaw)
- Own benchmark: benchmarks/results/2026-04-17.json, methodology.md, latency.md
- Togather context: local worktree ~/Documents/art/togather-server (GPL-3.0), prior Scrapling comparative eval (Jun 5, 2026), CITY-03 city-filter review history
