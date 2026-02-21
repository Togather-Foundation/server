# Generate Scraper Selectors

Given one or more URLs, generate working Tier 1 CSS selector configs for the SEL scraper,
validate them live, check the database for a matching organization, and write the result
to `configs/sources/<name>.yaml`.

You are an **orchestrator**. Parse the input, build the URL list, then delegate each URL
to a parallel subagent via the Task tool. Collect results and print a summary.

## Input

The argument(s) to this command can be:
- A single URL: `/generate-selectors https://example.com/events`
- Space-separated URLs: `/generate-selectors https://a.com/events https://b.com/events`
- A plain-text file of URLs (one per line): `/generate-selectors urls.txt`

If a file path is given, read it and extract all non-empty, non-comment lines as URLs.

## Orchestration

### Step 1 — Build the URL list

Parse the input into a flat list of URLs. Skip blank lines and lines starting with `#`.

### Step 2 — Pre-flight checks

For each URL, check whether a config already exists:

```bash
ls configs/sources/   # scan existing names
```

Derive the source name for each URL (see naming rules in the worker section below).
Flag any URLs where a config already exists. If more than one such URL exists, ask
the user once: "The following configs already exist: X, Y, Z. Overwrite all, skip all,
or decide per-URL?" Apply the answer to all subagents as their `conflict_policy`.

### Step 3 — Dispatch subagents in parallel batches

Launch subagents in parallel batches of **up to 5** at a time.
Pass each subagent exactly the information it needs (see Worker Prompt below).
Wait for all agents in a batch to complete before starting the next batch.

For each URL, launch one Task subagent (`subagent_type: general`) with the worker
prompt below, substituting `<URL>` and `<conflict_policy>`.

### Step 4 — Collect results

Each subagent returns a one-line result in this exact format:

```
RESULT | <url> | <name> | <event_count> | <status> | <notes>
```

Where:
- `status` is one of: `written`, `failed`, `skipped`, `js-rendered`, `blocked`
- `event_count` is a number, or `-` if not applicable

### Step 5 — Print summary

After all batches complete, print:

```
## Selector Generation Results

| URL | Name | Events | Status | Notes |
|-----|------|--------|--------|-------|
| ... | ...  | ...    | ✓ written / ✗ failed / — skipped / ⚠ js-rendered | ... |
```

Then, if any configs were written:
> Review the generated files in `configs/sources/`, then:
> ```
> make ci
> git add configs/sources/ && git commit -m "feat(scraper): add Tier 1 selectors for <names>"
> ```

---

## Worker Prompt

Use this prompt verbatim for each subagent, substituting `<URL>` and `<conflict_policy>`:

---

Process this URL for Tier 1 CSS selector generation:

**URL:** `<URL>`
**Conflict policy:** `<conflict_policy>`  (one of: `ask`, `overwrite`, `skip`)
**Working directory:** `/home/ryankelln/Documents/Projects/Art/togather/server`
**Server binary:** `./server`
**SEL API:** `${SEL_SERVER_URL:-http://localhost:8080}` (may not be running — if org lookup fails with connection refused, note and continue)

Follow these steps in order:

### Step 1 — Inspect the page

```bash
./server scrape inspect <URL>
```

Read the output carefully:
- **Top CSS classes** — look for classes with counts ≥ 3 that sound like event containers
  (event, card, item, listing, show, program, performance, film, concert, exhibition)
- **data-* attributes** — often signal repeating structured elements
- **Candidate event containers** — the sample HTML snippets are the most important signal;
  read them to understand actual DOM structure
- **Event hrefs** — confirm the page actually has event links

If the page returns < 5KB body or the candidate containers section is empty, the page is
likely JS-rendered. Return:
`RESULT | <URL> | - | - | js-rendered | <body_size>KB body, candidate containers empty`

### Step 2 — Derive a source name

Derive a short, hyphenated, lowercase name from the domain:
- Strip `www.`, `tpl.`, and similar common subdomains
- Strip TLD (`.com`, `.ca`, `.org`, `.net`)
- Convert to lowercase-hyphenated (e.g. `harbourfrontcentre.com` → `harbourfront-centre`)
- Keep it recognizable but concise (max ~4 words)

### Step 3 — Check for an existing config

```bash
ls configs/sources/<name>.yaml 2>/dev/null
```

If it exists:
- `conflict_policy: skip` → return `RESULT | <URL> | <name> | - | skipped | config already exists`
- `conflict_policy: overwrite` → proceed, overwrite silently
- `conflict_policy: ask` → this won't happen (orchestrator resolved it before dispatching)

### Step 4 — Check the database for a matching organization

Derive a human-readable search term from the source name (e.g. `soulpepper` → `soulpepper`,
`factory-theatre` → `factory theatre`). Search:

```bash
curl -s "${SEL_SERVER_URL:-http://localhost:8080}/api/v1/organizations?q=<search_term>&limit=5"
```

- If connection refused: note "API unavailable"
- If match found: note best match name + ULID
- If no match: note "no match found in database"

### Step 5 — Propose selectors

Based on the inspect output, reason about the DOM structure and propose values for:

| Field | Notes |
|-------|-------|
| `event_list` | **Required.** Selector for the repeating event container. Most important — get this right first. |
| `name` | Selector for the event title. Often `h2`, `h3`, or a classed `<span>`/`<div>`. |
| `start_date` | Selector for start date. Prefer `<time datetime="...">` when present. |
| `end_date` | Selector for end date if present. Leave empty if not visible. |
| `location` | Selector for venue/location name. |
| `description` | Selector for a short description blurb. Leave empty if not present. |
| `url` | Selector for the `<a>` linking to the event detail page. |
| `image` | Selector for the event thumbnail `<img>`. Leave empty if not present. |
| `pagination` | Selector for the "next page" link. Leave empty if single-page. |

### Step 6 — Validate with scrape test

```bash
./server scrape test <URL> \
  --event-list "<event_list>" \
  --name "<name_selector>" \
  --start-date "<start_date_selector>" \
  --location "<location_selector>" \
  --url "<url_selector>" \
  --image "<image_selector>"
```

Evaluate:
- **Pass**: ≥ 3 events with non-empty `name` → proceed
- **Partial**: events found but key fields empty/garbled → adjust and retry
- **Fail**: 0 events → `event_list` selector is wrong; revise and retry

Retry up to **3 rounds**. If still failing after 3 rounds, return:
`RESULT | <URL> | <name> | 0 | failed | <what you tried and why it failed>`

**Note on duplicated text**: Sites may inject hidden `<span>` elements (e.g.
`cp-screen-reader-message`) that get concatenated into field values. If you see
`"BendaleEvent location: Bendale"`-style output, note it in the YAML comment.

### Step 7 — Write the config

```yaml
name: "<derived-name>"
url: "<URL>"
tier: 1
schedule: "daily"
trust_level: 5
license: "CC0-1.0"
enabled: true
max_pages: 3
# Organization match: <org name> (<ulid>) — or "no match found in database"
selectors:
  event_list: "<selector>"
  name: "<selector>"
  start_date: "<selector>"
  url: "<selector>"
```

Omit any selector line whose value is empty — do not write `field: ""`.

`trust_level`: `8` for official government/library/museum sites, `3` for aggregators,
`5` for everything else.

### Step 8 — Final dry-run

```bash
./server scrape source <name> --dry-run
```

If the binary is missing, build first:
```bash
go build -o ./server ./cmd/server && ./server scrape source <name> --dry-run
```

### Return result

Return exactly one line in this format (no other content needed):

```
RESULT | <URL> | <name> | <event_count> | written | <any notable notes>
```

---

*End of worker prompt*
