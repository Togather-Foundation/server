# Configure Source

Given one or more URLs, generate working scraper source configs (Tier 0–3) for the
SEL scraper by dispatching each URL to a `scraper-worker` subagent.

You are an **orchestrator**. Parse the input, build the URL list, then delegate each URL
to a parallel subagent via the Task tool. Collect results and print a summary.

## Input

The argument(s) to this command can be:
- A single URL: `/configure-source https://example.com/events`
- Space-separated URLs: `/configure-source https://a.com/events https://b.com/events`
- A plain-text file of URLs (one per line): `/configure-source urls.txt`

If a file path is given, read it and extract all non-empty, non-comment lines as URLs.

## Orchestration

### Step 1 — Build the URL list

Parse the input into a flat list of URLs. Skip blank lines and lines starting with `#`.

### Step 2 — Pre-flight checks

For each URL, check whether a config already exists:

```bash
ls configs/sources/   # scan existing names
```

Derive the source name for each URL using these rules:
- Strip `www.`, `tpl.`, and similar common subdomains
- Strip TLD (`.com`, `.ca`, `.org`, `.net`)
- Convert to lowercase-hyphenated (e.g. `harbourfrontcentre.com` → `harbourfront-centre`)
- Keep it recognizable but concise (max ~4 words)

Flag any URLs where a config already exists. If more than one such URL exists, ask
the user once: "The following configs already exist: X, Y, Z. Overwrite all, skip all,
or decide per-URL?" Apply the answer to all subagents as their `conflict_policy`.

### Step 2b — Sitemap hint (optional)

If the user mentions "sitemap" or the URL points to a sitemap XML, or you have prior
knowledge that the site's listing page is unscrapeable (Wix, Duda, etc.), include a
**sitemap hint** in the worker dispatch prompt. This tells the worker to check for a
sitemap before attempting regular listing-page scraping.

The scraper-worker already knows how to discover and configure sitemaps — you just
need to trigger the path by adding to the prompt:

```
Sitemap hint: the listing page may be unscrapeable. Check for a sitemap at
<DOMAIN>/sitemap.xml and /robots.txt before attempting listing-page scraping.
If a sitemap with event URLs exists, use sitemap-based config (see Sitemap Config
section in your instructions).
```

### Step 3 — Dispatch subagents in parallel batches

Launch subagents in parallel batches of **up to 5** at a time.
Wait for all agents in a batch to complete before starting the next batch.

For each URL, launch one Task subagent with `subagent_type: "scraper-worker"`.

The worker agent already has full instructions for inspecting pages, identifying tiers,
validating selectors, and writing YAML configs. You only need to pass it a prompt
containing:

- **URL** — the target URL to process
- **Conflict policy** — `skip`, `overwrite`, or `ask` (resolved in Step 2)
- **Domain** — infer `event_domain` from the site content (music venue → "music", art gallery → "arts", etc.) and set `domain:` in the config
- **No-git reminder** — reinforce that the worker must not run git commands

Example dispatch prompt:

```
Process this URL for CSS selector generation:

URL: https://example.com/events
Conflict policy: skip

Do NOT run git add, git commit, or git push — the orchestrator handles git.
```

Do **not** duplicate the worker's instructions in the prompt. The `scraper-worker`
agent type already contains all platform detection, selector identification,
validation, and YAML template logic.

### Step 4 — Collect results

Each subagent returns a structured report including a result line in this format:

```
RESULT | <url> | <name> | <event_count> | <domain> | <status> | <notes>
```

Where:
- `status` is one of: `written`, `failed`, `skipped`, `js-rendered`, `blocked`, `downgraded`
- `event_count` is a number, or `-` if not applicable
- `domain` is the configured `event_domain`, or `-` if not applicable

Each written source must also report:
- the chosen `suppress_review_warnings` list — `none` for a new source (leave it
  unset), or the existing source's configured list when editing one
- the chosen `max_pages`
- `cap: none | max_pages_reached | result_truncated_by_limit | manual (found N vs cap/limit M)`
  from the dry run. Only `bibliocommons_rss` emits the two warnings;
  `result_truncated_by_limit` fires whenever `Limit > 0` (including an explicit
  `--limit`) but not on a default dry run. For non-BiblioCommons paths, report the
  manual comparison; for sitemap, use the `sitemap_total` / `scraping` discovery log.

The worker also returns an **Issues** section reporting any scraper bugs, UX problems,
or documentation gaps encountered. Collect these for the summary.

### Step 5 — Print summary

After all batches complete, print:

```
## Selector Generation Results

| URL | Name | Events | Domain | Suppression | max_pages | Cap | Status | Notes |
|-----|------|--------|--------|-------------|-----------|-----|--------|-------|
| ... | ...  | ...    | ...    | ...         | ...       | ... | ✓ written / ✗ failed / — skipped / ⚠ js-rendered | ... |
```

`Suppression` is `none (new source)` or the existing source's list; `Cap` is
`none` / `max_pages_reached` / `result_truncated_by_limit` from the dry run, or
`manual (found N vs cap/limit M)` for paths that truncate silently.

If any workers reported issues, print them in a consolidated section:

```
## Issues Reported

- [severity] category: description (from <source-name>)
```

Then, if any configs were written:
> Review the generated files in `configs/sources/`, then:
> ```
> make ci-fast
> git add configs/sources/ && git commit -m "feat(scraper): add selectors for <names>"
> ```
> After the config lands, sync it to the running server:
> ```
> ./server scrape sync
> ```
> On staging/production, deploy first — the YAML is baked into the image and
> `deploy.sh` runs `scrape sync --json` automatically in the new container. Only run
> `scripts/remote.sh staging "scrape sync"` manually when you are sure the running
> image already contains the YAML you want; from an undeployed tree it would upsert the
> old image's configs. YAML changes are ignored at runtime until synced.
