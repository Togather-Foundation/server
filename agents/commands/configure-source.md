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

### Step 3 — Dispatch subagents in parallel batches

Launch subagents in parallel batches of **up to 5** at a time.
Wait for all agents in a batch to complete before starting the next batch.

For each URL, launch one Task subagent with `subagent_type: "scraper-worker"`.

The worker agent already has full instructions for inspecting pages, identifying tiers,
validating selectors, and writing YAML configs. You only need to pass it a prompt
containing:

- **URL** — the target URL to process
- **Conflict policy** — `skip`, `overwrite`, or `ask` (resolved in Step 2)
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
RESULT | <url> | <name> | <event_count> | <status> | <notes>
```

Where:
- `status` is one of: `written`, `failed`, `skipped`, `js-rendered`, `blocked`, `downgraded`
- `event_count` is a number, or `-` if not applicable

The worker also returns an **Issues** section reporting any scraper bugs, UX problems,
or documentation gaps encountered. Collect these for the summary.

### Step 5 — Print summary

After all batches complete, print:

```
## Selector Generation Results

| URL | Name | Events | Status | Notes |
|-----|------|--------|--------|-------|
| ... | ...  | ...    | ✓ written / ✗ failed / — skipped / ⚠ js-rendered | ... |
```

If any workers reported issues, print them in a consolidated section:

```
## Issues Reported

- [severity] category: description (from <source-name>)
```

Then, if any configs were written:
> Review the generated files in `configs/sources/`, then:
> ```
> make ci
> git add configs/sources/ && git commit -m "feat(scraper): add selectors for <names>"
> ```
