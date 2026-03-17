# Plan: Agentic SEL Node Maintainer

**Spec**: 004-agentic-maintainer | **Date**: 2026-03-16 | **Status**: Planning
**Goal**: A single operator and their agent fleet manage a SEL node part-time, with human intervention only for genuinely novel situations.

## Vision

Running a SEL node should be a background activity for one person. Agents handle the
routine operational loop — monitoring metrics, reviewing ingested events, diagnosing
scraper failures, managing data quality — and only escalate when they encounter a
situation with no documented precedent. Every human decision gets recorded so the
agents can handle that class of problem autonomously next time.

The key insight: **the agent fleet gets smarter over time**. Each escalation produces a
decision record that becomes a future autonomous resolution path.

---

## Current State

### What Exists Today

| Capability | Status | Notes |
|---|---|---|
| MCP server (10 tools) | Production | events, places, orgs, search, geocoding, API keys |
| Prometheus + Grafana | Production | 10+ metric categories, 2 dashboards |
| Event review queue | Production | approve/reject/fix/merge/add-occurrence |
| Audit logging | Production | Structured JSON, admin attribution |
| Scraper pipeline | Production | 4 tiers, YAML+DB config, River scheduling |
| River job queue | Production | 10 job kinds, retry policies, metrics |
| Health checks | Production | DB, migrations, job queue, JSON-LD contexts |
| OpenCode commands | Working | `/orchestrate`, `/configure-source`, `/release` |
| Beads task tracking | Working | Local Dolt DB, persistent across sessions |

### What's Missing for Autonomous Operation

1. **No decision memory** — agents can't learn from past review decisions
2. **No anomaly detection** — agents don't proactively watch metrics
3. **No autonomous review** — review queue requires manual admin action
4. **No scraper health triage** — failures require manual investigation
5. **No data quality patrol** — stale/broken events accumulate silently
6. **No operator notification** — agents can't escalate to the human
7. **No runbook library** — common fixes aren't codified for agents

---

## Architecture

### Agent Roles

```
                    ┌─────────────────────────────┐
                    │       Human Operator         │
                    │   (escalation + oversight)   │
                    └──────────┬──────────────────┘
                               │ notifications
                               │ (only novel situations)
                    ┌──────────▼──────────────────┐
                    │     Maintainer Orchestrator  │
                    │   (OpenCode skill/command)   │
                    │   Runs on schedule or        │
                    │   triggered by alert         │
                    └──┬─────┬─────┬─────┬───────┘
                       │     │     │     │
            ┌──────────▼┐ ┌─▼────┐│  ┌──▼──────────┐
            │  Metrics   │ │Review││  │  Scraper     │
            │  Watcher   │ │Agent ││  │  Health      │
            └────────────┘ └──────┘│  └─────────────┘
                                   │
                            ┌──────▼────────┐
                            │  Data Quality  │
                            │  Patrol        │
                            └───────────────┘
```

Each role is a **subagent type or skill** invoked by the orchestrator. They share:
- The **MCP server** for data access (events, places, orgs, search)
- The **decision journal** for institutional memory
- The **Prometheus metrics** (via `/metrics` endpoint or Grafana API)
- **Beads** for tracking work items that span sessions

### Core Loop

```
1. CHECK  — Gather health signals (metrics, queue depth, scraper status)
2. TRIAGE — Classify issues by severity and novelty
3. ACT    — Handle routine issues autonomously (using runbooks + decision memory)
4. LEARN  — Record decisions and reasoning for novel resolutions
5. REPORT — Notify operator of actions taken and any escalations
```

---

## Component Design

### 1. Decision Journal (Institutional Memory)

The most critical new component. Every review decision, scraper fix, and data quality
correction gets recorded with enough context that an agent can pattern-match future
similar situations and act without human input.

**Design principles**:

- **Framework-agnostic storage**: The journal is JSONL files on disk as the canonical
  format. One file per entry, human-readable, trivially portable. Database indexes
  are a read optimization layer, not the source of truth. If/when better agentic
  memory systems emerge, JSONL converts to anything.
- **Preserve expensive reasoning**: Agent decisions that required multi-step analysis,
  tool calls, or external inspection are expensive to reproduce. The full reasoning
  chain gets stored, not just the conclusion. This is especially important for scraper
  fixes where the agent inspected a DOM, tried multiple selectors, etc.
- **Permanent learning from confirmed outcomes**: Once a decision has a confirmed
  positive outcome (event was correct, scraper fix held, metric normalized), that
  decision graduates from "precedent" to "rule". Rules are never aged out — they're
  the institutional knowledge of the node.
- **KISS**: Start with flat files and simple grep-based lookup. Add database indexing
  only when file-based search becomes a bottleneck (likely hundreds of decisions
  before this matters).

#### Storage Format

**Canonical**: JSONL files in `data/decisions/`, one file per decision, named by
timestamp + short ID: `2026-03-16T14-30-00Z_rev_abc123.jsonl`

```jsonc
{
  "id": "dec-abc123",
  "created_at": "2026-03-16T14:30:00Z",
  "category": "review",
  "subcategory": "reversed_dates",
  "source_name": "venue-x",

  // What triggered this decision
  "trigger": {
    "warning_codes": ["reversed_dates_timezone_likely"],
    "review_id": 42,
    "event_ulid": "01HXYZ..."
  },

  // The decision itself
  "decision": "approve_with_fix",
  "reasoning": "Source venue-x consistently submits events with UTC times that should be EST. The auto-corrected times (20:00→20:00 EST, original was 01:00 UTC next day) match the venue's published schedule. 3 prior events from this source had the same pattern and all were correct after fix.",

  // For agent decisions involving significant work — preserve the chain
  "reasoning_chain": [
    "Checked decision journal: 3 prior reversed_dates from venue-x, all approved, all confirmed correct",
    "Compared corrected times against venue-x website: match",
    "Duration after fix: 2.5h — consistent with venue-x typical event length (2-3h)"
  ],

  // What was actually done
  "resolution": {
    "action": "approve",
    "corrections": {"startDate": "2026-03-20T20:00:00-05:00", "endDate": "2026-03-20T22:30:00-05:00"},
    "review_notes": "Auto-fixed timezone offset, consistent with source pattern"
  },

  "decided_by": "agent:review-agent",  // or "human:ryankelln"
  "confidence": 0.9,                    // null for human decisions
  "decision_source": "precedent",       // "precedent", "analysis", "escalation_response", "inferred"

  // Filled in retrospectively
  "outcome": null,                       // "confirmed", "reverted", "superseded"
  "outcome_at": null,
  "outcome_notes": null,

  // Cross-references
  "related": {
    "event_ulid": "01HXYZ...",
    "review_id": 42,
    "prior_decisions": ["dec-xyz789", "dec-def456"]
  },

  // Graduated to permanent rule?
  "is_rule": false,
  "rule_summary": null  // Populated when graduated: "Source venue-x: always approve reversed_dates_timezone_likely"
}
```

**Why JSONL and not a database table?**
- Trivially version-controllable (can be committed to git or synced separately)
- Readable by any tool, any language, any future agent framework
- No schema migration burden — add fields freely
- Grep/jq for lookup is fast enough for hundreds/low-thousands of entries
- Database index can be rebuilt from files at any time

**Optional database index** (add when file-based search is too slow):

```sql
-- Read-optimization only. Rebuilt from JSONL files via `server decisions reindex`.
CREATE TABLE decision_index (
    id              TEXT PRIMARY KEY,     -- "dec-abc123"
    created_at      TIMESTAMPTZ NOT NULL,
    category        TEXT NOT NULL,
    subcategory     TEXT,
    source_name     TEXT,
    decision        TEXT NOT NULL,
    decided_by      TEXT NOT NULL,
    outcome         TEXT,
    is_rule         BOOLEAN DEFAULT FALSE,
    trigger_codes   TEXT[],               -- extracted from trigger.warning_codes for array overlap queries
    file_path       TEXT NOT NULL          -- pointer back to canonical JSONL file
);
```

#### Decision Categories

| Category | Subcategories | Example |
|---|---|---|
| `review` | `reversed_dates`, `near_duplicate`, `missing_venue`, `suspicious_content`, `low_confidence` | "Approved: source X consistently has timezone-offset dates, auto-fix is correct" |
| `scraper` | `selector_drift`, `site_blocked`, `rate_limited`, `empty_results`, `new_page_structure` | "Fixed: venue Y redesigned, updated `.event-card` to `.event-item`" |
| `data_quality` | `stale_event`, `orphaned_place`, `missing_geocode`, `broken_url` | "Deleted: recurring event series ended, source confirmed" |
| `metrics` | `error_spike`, `latency_anomaly`, `queue_backup`, `disk_usage` | "Investigated: spike was deploy-related, resolved by next scrape cycle" |

#### Precedent Lookup

Agents use a simple search strategy:

1. **Exact match**: Same source + same warning codes → highest precedent value
2. **Pattern match**: Different source + same warning codes → transferable precedent
3. **Rules**: Graduated decisions with `is_rule: true` → apply without further analysis

For the JSONL-on-disk approach, this is a `grep` + `jq` pipeline. For the optional
database index, it's a SQL query with array overlap (`trigger_codes && $1`).

When an agent finds matching precedent with `outcome = 'confirmed'`, it can apply the
same resolution autonomously. When precedent shows `outcome = 'reverted'` or was
escalated, it knows to escalate. Decisions with `is_rule: true` are applied without
even checking for matching context — they represent permanent institutional knowledge.

#### The Graduation Mechanism

Decisions start as precedent. They graduate to rules when:
- The decision has a confirmed positive outcome, AND
- At least N similar decisions exist with the same outcome (default: 3), OR
- A human explicitly marks the decision as a rule

Rules get a `rule_summary` — a one-line natural language statement like:
"Source venue-x: always approve reversed_dates_timezone_likely — timezone offset is
consistent and auto-fix is reliable."

Rules are never aged out. They're the permanent institutional knowledge of the node.
The rest of the decision journal can be pruned over time.

### Human Decision Capture (The Hard Problem)

Recording agent decisions is straightforward — the agent is already in a structured
tool-calling flow and can write the journal entry as part of its action. Human
decisions are harder because they happen through different channels:

**Channel 1: Admin UI actions (structured, inferrable)**

The admin review UI already records `reviewed_by`, `reviewed_at`, `rejection_reason`,
and `review_notes`. A background process can watch for new review completions and
generate decision journal entries by inference:

```
Admin approved event X from source Y which had warnings [reversed_dates_timezone_likely]
→ Infer: "Human decided reversed_dates auto-fix was correct for this source"
→ Record with decided_by: "human:admin_username", decision_source: "inferred"
```

The inference is imperfect but captures the core signal. The `review_notes` field
(if the admin bothered to fill it in) provides reasoning. If not, the reasoning is
"inferred from admin UI action" — still useful for precedent matching even without
detailed rationale.

**Channel 2: Chat interactions during escalation (rich, unstructured)**

When an agent escalates to the human via `/maintain inbox` or chat, the human's
response contains the real reasoning. This is the richest source of institutional
knowledge but the hardest to capture.

Approaches (from simplest to most sophisticated):

1. **Post-resolution prompt**: After the human resolves an escalation, the maintainer
   agent asks: "What was your reasoning? I'll record it for future reference." The
   human types a sentence or two. This is the lowest-friction approach that actually
   captures reasoning.

2. **Conversation summary**: The maintainer agent summarizes the escalation
   conversation into a decision entry. Human confirms or edits before it's recorded.
   More structured but adds friction.

3. **Implicit capture**: Record the full escalation conversation as context in the
   decision entry. Future agents can read the conversation to understand the
   reasoning. No human effort required, but the "reasoning" is buried in chat
   rather than distilled.

**Recommended**: Start with (1) + (3). Always save the conversation context. Prompt
for a summary when possible, but don't block on it — an imperfect record is better
than none. The agent can always re-read the full conversation later if the summary
is insufficient.

**Channel 3: Out-of-band actions (invisible)**

The operator SSHes in and fixes something directly, or edits a scraper config YAML
by hand, or restarts a service. These actions are invisible to the agent system.

Mitigation:
- Git diff detection: the maintainer can notice config changes it didn't make and
  ask the human about them on next run
- `server decisions record` CLI: the human can manually record a decision after
  taking an out-of-band action (low friction, optional)
- Accept that some knowledge will be lost. The system should be robust to gaps.

### 2. MCP Server Extensions

New tools the agents need beyond the current 10:

| Tool | Purpose | Priority |
|---|---|---|
| `review_queue` | List/get pending reviews with warnings and payloads | P1 |
| `review_decide` | Approve/reject/fix/merge a review entry (with reasoning) | P1 |
| `decision_log` | Query past decisions by category/source/trigger | P1 |
| `record_decision` | Write a new decision entry | P1 |
| `scraper_status` | Get recent scrape runs, failures, quality warnings | P1 |
| `scraper_config` | Read/update source configs (enable/disable, fix selectors) | P2 |
| `metrics_snapshot` | Get current Prometheus metric values (key health indicators) | P2 |
| `metrics_anomalies` | Compare current metrics to baseline, flag outliers | P2 |
| `data_quality_report` | Stale events, missing geocodes, broken URLs, orphaned entities | P2 |
| `notify_operator` | Send escalation to human (email, webhook, or queued message) | P1 |

**Why MCP and not just HTTP API?** The MCP tools provide structured schemas and
descriptions that help agents understand what's available and how to call it. The
existing review queue HTTP endpoints require JWT auth and aren't designed for agent
ergonomics. MCP tools can compose multiple HTTP calls into a single semantic action
(e.g., `review_decide` = approve + record decision + dismiss companion warnings).

### 3. Metrics Watcher Agent

**Trigger**: Scheduled (hourly) or alert-triggered.

**Capabilities**:
- Fetch key metrics from Prometheus via `metrics_snapshot` MCP tool
- Compare against rolling baselines (7-day averages stored in decision journal)
- Detect: error rate spikes, latency anomalies, queue depth growth, scraper failure streaks
- Classify by severity: `info` (log), `warning` (investigate), `critical` (escalate)

**Baseline metrics to watch**:

```
togather_health_status                     -- < 2 = degraded/unhealthy
togather_scraper_runs_total{result="error"} -- failure rate > 30% over 24h
togather_scraper_events_total{outcome="failed"} -- event ingest failures
river_job_count{state="discarded"}          -- jobs that exhausted retries
river_job_count{state="retryable"}          -- growing retry backlog
http_request_duration_seconds{quantile="0.95"} -- p95 latency > 2s
togather_geocoding_failures_total           -- geocoding error rate
```

**Implementation path**: This could be a dedicated OpenCode subagent type
(`metrics-watcher`) or an OpenCode skill that's invoked by the maintainer orchestrator.
Given that it needs to parse Prometheus exposition format or query Grafana's API,
a skill with bundled scripts for metric fetching would be cleanest.

### 4. Review Agent

**Trigger**: Scheduled (every few hours) or when review queue depth > threshold.

**Capabilities**:
- Fetch pending reviews via `review_queue` MCP tool
- For each review entry:
  1. Check decision journal for precedent (same source + same warning codes)
  2. If high-confidence precedent exists: apply same decision autonomously
  3. If no precedent: analyze the event data, check for obvious issues
  4. If confident: decide and record reasoning in decision journal
  5. If uncertain: queue for human review with analysis notes

**Autonomous decision rules** (conservative starting point):

| Scenario | Auto-Decision | Confidence Threshold |
|---|---|---|
| `reversed_dates_timezone_likely` from known source with 3+ prior approvals | Approve | 0.9 |
| Near-duplicate of already-published event (>0.95 similarity) | Merge | 0.85 |
| Event from high-trust source (trust_level >= 8) with no warnings | Approve | 0.95 |
| Missing venue but source consistently uses same venue | Fix (add venue) | 0.8 |
| Event >1 year in future from source that never posts that far out | Reject | 0.8 |
| Anything else | Escalate | — |

These thresholds are starting points. As the decision journal accumulates data, the
agent can calibrate — if its decisions consistently get `outcome = 'success'`, thresholds
can be relaxed. If decisions get reverted, they tighten.

**Critical safety rule**: The review agent should NEVER auto-approve events with
`suspicious_content` warnings. Content moderation always escalates to human.

### 5. Scraper Health Agent

**Trigger**: After each scrape cycle completes, or on scraper failure alert.

**Capabilities**:
- Check `scraper_runs` for recent failures via `scraper_status` MCP tool
- Classify failures:
  - **Transient** (network timeout, rate limit): no action, retry will handle it
  - **Selector drift** (0 events found, quality warnings): investigate and fix
  - **Site blocked** (403/429): check robots.txt, adjust headers/rate
  - **Site down** (5xx): transient, but disable source after 7 consecutive days
- For selector drift:
  1. Check decision journal for prior fixes to this source
  2. Dispatch `scraper-worker` subagent to inspect current DOM and propose new selectors
  3. Test proposed selectors with `server scrape test`
  4. If successful: update config, record decision
  5. If failed: escalate with DOM analysis attached

**Proactive monitoring**:
- Track events-found-per-run trend per source
- Alert on sudden drops (source that usually finds 20 events now finds 2)
- Alert on sources that haven't been scraped in > 2x their schedule interval

### 6. Data Quality Patrol

**Trigger**: Weekly scheduled run.

**Capabilities**:
- Scan for stale events (past `endDate`, still `lifecycle_state = 'published'`)
- Scan for places with missing/failed geocoding
- Scan for events with broken source URLs (HTTP HEAD check)
- Scan for orphaned entities (places/orgs not linked to any events)
- Scan for events with missing required fields that passed validation
- Check for duplicate place/org entries (similar names, same address)

**Actions**:
- Stale events: update lifecycle_state (routine, no escalation needed)
- Missing geocodes: re-queue geocoding job
- Broken URLs: flag for review, disable source if pattern
- Orphans: report to operator (may be legitimate, may indicate data loss)
- Duplicates: propose merges, queue for human confirmation

### 7. Operator Notification System

The human needs a single, low-noise channel for escalations. Options to evaluate:

| Channel | Pros | Cons |
|---|---|---|
| Email digest | Async, searchable, no new tooling | Easy to miss, no threading |
| GitHub Issues | Integrated with codebase, threaded | Noisy if overused |
| Dedicated queue table + CLI | Fully integrated, `server inbox` command | Requires building |
| Webhook (Slack/Discord/Matrix) | Real-time, conversational | Requires external service |

**Recommended**: Hybrid approach:
- **Queue table** (`maintainer_notifications`) for all notifications — agents write, human reads via `server inbox` CLI
- **Webhook** for critical escalations only (optional, configurable)
- **Beads** for work items that need tracking across sessions

Notification structure:
```
- severity: info | warning | critical
- category: review | scraper | data_quality | metrics | system
- title: One-line summary
- detail: Full context, agent analysis, proposed actions
- requires_action: bool (true = needs human decision)
- related_decisions: []int (links to decision journal entries)
```

---

## OpenCode Integration

### New Subagent Types

| Agent | Model | Purpose |
|---|---|---|
| `maintainer` | claude-sonnet-4.6 | Orchestrator — runs the core loop, dispatches specialists |
| `review-agent` | claude-sonnet-4.6 | Event review specialist — decides on review queue entries |
| `metrics-watcher` | claude-haiku-4.5 | Lightweight metrics checker — runs frequently, escalates anomalies |

The existing `scraper-worker` and `diagnose` agents are already suitable for their
roles in this system. No new agent types needed for scraper health or diagnosis.

### New OpenCode Command: `/maintain`

Top-level command that runs the maintenance loop:

```
/maintain              — Full maintenance pass (all checks)
/maintain review       — Process review queue only
/maintain metrics      — Check metrics only
/maintain scraper      — Check scraper health only
/maintain quality      — Run data quality patrol only
/maintain inbox        — Show pending operator notifications
/maintain decisions    — Browse/search decision journal
```

Implementation: `agents/commands/maintain.md` — dispatches to specialist subagents
in parallel where possible (metrics + scraper health can run concurrently; review
depends on metrics context).

### New OpenCode Skill: `sel-maintainer`

A skill (`.opencode/skill/sel-maintainer/SKILL.md`) that provides:
- The maintenance loop workflow
- Decision journal query patterns
- Escalation decision tree
- Runbook library for common issues
- Confidence calibration guidelines

This skill gets loaded by the `maintainer` orchestrator agent at the start of each
maintenance pass, giving it the full context of how to operate.

### Scripts

| Script | Purpose |
|---|---|
| `scripts/maintenance-check.sh` | Quick health summary (for cron or manual check) |
| `scripts/metrics-snapshot.sh` | Fetch key Prometheus metrics, output as JSON |
| `scripts/review-queue-summary.sh` | Count pending reviews by category/source |
| `scripts/scraper-health.sh` | Summarize recent scrape runs and failure rates |
| `scripts/decision-report.sh` | Generate decision journal summary for time period |

These scripts are thin wrappers that agents can call via bash, providing structured
output without needing to parse raw Prometheus exposition format or construct complex
SQL queries.

---

## CLI Extensions

New `server` subcommands for the operator:

```
server maintain run             — Trigger a full maintenance pass
server maintain status          — Show last maintenance pass results
server inbox                    — List pending operator notifications
server inbox resolve <id>       — Mark notification as handled
server decisions list           — Browse decision journal
server decisions search <query> — Search decisions by keyword
server decisions record         — Manually record a decision (for human decisions)
```

---

## Implementation Phases

### Phase 1: Decision Journal + Review Agent (Foundation)

**Why first**: The review queue is the most frequent operational task, and the decision
journal is the foundation everything else builds on.

1. Brief survey of existing agentic memory tools (Mem0, Letta, MCP memory servers) —
   adopt if one is both KISS and interoperable; default to JSONL otherwise
2. Decision journal: JSONL file format, `data/decisions/` directory, read/write CLI
3. MCP tools: `review_queue`, `review_decide`, `decision_log`, `record_decision`
4. Human decision inference from admin UI audit log entries
5. Review agent subagent type with conservative auto-decision rules
6. `/maintain review` command
7. `server decisions` CLI commands (list, search, record, reindex)
8. Seed decision journal with existing review patterns from audit logs

**Success criteria**: Agent can autonomously handle >50% of review queue entries
(reversed dates, known-source duplicates) with 0 incorrect decisions over 2 weeks.

### Phase 2: Metrics Watcher + Notifications

**Why second**: Gives the operator confidence that the system is being watched, even
before all autonomous actions are in place.

1. `metrics_snapshot` and `metrics_anomalies` MCP tools
2. `scripts/metrics-snapshot.sh` for structured metric fetching
3. Metrics watcher agent with baseline comparison
4. Notification table + `server inbox` CLI
5. `/maintain metrics` command
6. Optional webhook integration for critical alerts

**Success criteria**: Agent detects simulated anomalies (injected via test) within
one check cycle and produces actionable notification with correct severity.

### Phase 3: Scraper Health Agent

**Why third**: Builds on metrics watcher (scraper failures show up in metrics) and
decision journal (fix patterns are recorded).

1. `scraper_status` and `scraper_config` MCP tools
2. Scraper health classification logic
3. Integration with existing `scraper-worker` for DOM inspection and selector repair
4. `/maintain scraper` command
5. `scripts/scraper-health.sh` summary script

**Success criteria**: Agent autonomously fixes a selector-drift failure using the
`scraper-worker` subagent and records the fix in the decision journal.

### Phase 4: Data Quality Patrol + Full Orchestration

1. `data_quality_report` MCP tool
2. Data quality scan queries (stale events, missing geocodes, broken URLs)
3. `/maintain quality` command
4. Full `/maintain` orchestrator that runs all checks
5. `sel-maintainer` OpenCode skill with complete runbook library
6. Schedule integration (cron or River periodic job to trigger maintenance passes)

**Success criteria**: Full maintenance pass runs autonomously, processes review queue,
checks metrics, verifies scraper health, scans data quality, and produces a summary
report — with human intervention only for genuinely novel situations.

### Phase 5: Learning Loop + Confidence Calibration

1. Automatic outcome confirmation heuristics (event stayed published, scraper fix held, etc.)
2. Graduation pipeline: precedent → confirmed → rule (automated based on outcome count)
3. Decision journal analytics (most common issues, resolution time trends, cost savings from cached reasoning)
4. Source quality scoring based on historical decision patterns
5. Cost tracking: measure token savings from precedent reuse vs. fresh reasoning
6. Periodic review: surface rules that haven't been exercised recently for human validation

**Success criteria**: Agent's autonomous handling rate increases from 50% to 80%+ of
routine operations over 3 months, with maintained 0% error rate on decisions.

---

## Decision Journal: The Learning Flywheel

This deserves emphasis because it's the mechanism that makes part-time operation viable.

```
  Decision made        Recorded with full       Agent encounters
  (human or agent) ──► context + reasoning ──►  similar situation
       │                                              │
       │                                              ▼
       │                                    Finds matching precedent
       │                                    (avoids repeating expensive reasoning)
       │                                              │
       │                                              ▼
       │                                    Applies same decision
       │                                    autonomously
       │                                              │
       │              Outcome confirmed     ◄─────────┘
       │              (event correct, fix held)
       │                     │
       │                     ▼
       │               Graduates to RULE
       │               (permanent learning,
       ▼                never aged out)
  Next novel
  situation ─────────► New reasoning chain
                       (expensive but recorded
                        for future reuse)
```

### Why Preserve Reasoning Chains

Agent decisions that required real work — inspecting a DOM, comparing selectors,
cross-referencing a venue's published schedule, trying multiple approaches — are
expensive to reproduce. The LLM reasoning tokens, tool calls, and external lookups
involved might cost real money and minutes of wall-clock time.

When we record the `reasoning_chain` alongside the decision, future agents can skip
all that work. They read the prior reasoning, verify the conclusion still applies
(quick check), and act. This is the difference between "agent remembers" and "agent
has to re-derive from scratch every time."

**What to preserve** (non-exhaustive):
- The tool calls made and their results (DOM snapshots, metric values, search results)
- The reasoning steps ("checked X, found Y, concluded Z")
- Failed approaches ("tried selector A, matched 0 events; tried B, matched 15")
- External context consulted (venue website, social media confirmation)
- The confidence assessment and what drove it

### From Precedent to Rule

Decisions start uncertain. They become certain through confirmation:

1. **Precedent**: "Last time we saw reversed_dates from venue-x, we approved with fix
   and it was correct." Useful but tentative — the agent still checks context.

2. **Confirmed precedent**: Same decision, outcome explicitly marked "confirmed" (the
   approved event had correct dates, nobody complained, venue's own website matched).
   Higher confidence on reuse.

3. **Rule**: 3+ confirmed instances of the same pattern → graduates to permanent rule.
   "Source venue-x: always approve reversed_dates_timezone_likely." Applied without
   re-analysis. Never aged out. This is permanent institutional knowledge.

Rules are the end state. They represent the accumulated wisdom of operating this
specific node with these specific sources. A node with 200 rules needs far less
agent reasoning time than a fresh node with 0.

### Examples of the Flywheel

1. **First time**: Source "venue-x" submits events with reversed dates. Agent
   escalates — no precedent. Human approves with fix, agent records decision with
   reasoning (inferred from admin UI action + optional human explanation).

2. **Second time**: Same source, same warning code. Agent finds precedent, reads
   the prior reasoning (no need to re-derive), applies same fix. Records its own
   decision with `confidence = 0.9`, `decision_source = "precedent"`.

3. **Confirmation**: The event goes live, dates are correct, no corrections needed.
   Agent marks prior decision `outcome = "confirmed"`.

4. **Third time**: Now we have 3 confirmed decisions for this pattern. Agent
   graduates the pattern to a rule: "Source venue-x: always approve
   reversed_dates_timezone_likely." Future instances skip even the precedent lookup.

5. **Variant**: Different source, same warning pattern. Agent finds the venue-x
   rule but it's source-specific. Falls back to pattern-match precedent: "reversed
   dates have been correct 3/3 times across sources." Applies fix with lower
   confidence (`0.7`). If confirmed, this could generalize into a broader rule.

6. **Novel twist**: Source with reversed dates but the auto-fix produces a 36-hour
   duration. No precedent for this variant. Agent escalates with full context
   including the prior reasoning chain: "This looks like reversed dates but the
   corrected duration is unusual. See 3 prior decisions for this source where
   durations were 2-4h. The reasoning from those decisions suggests the pattern
   is timezone-offset, but 36h doesn't fit."

### Portability

The JSONL-on-disk format is deliberately boring technology. If the agentic memory
ecosystem converges on a better standard (vector stores, knowledge graphs, structured
memory protocols), converting JSONL entries is trivial — each entry is a self-contained
document with all context inline.

What matters is that the **information** is captured, not the **storage mechanism**.
The schema can evolve (JSONL is schema-flexible). The files can be git-tracked,
backed up, synced, grepped, or piped into any future system.

We specifically avoid:
- Proprietary memory formats tied to specific agent frameworks
- Embedding-only storage (vectors without the original text)
- Database-first designs that make export/migration painful
- Complex graph schemas that add maintenance burden without proven value

---

## Risks and Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Agent makes incorrect review decision | High | Conservative initial thresholds; human spot-check first 100 decisions; outcome tracking with auto-revert |
| Decision journal grows stale (patterns change) | Medium | Outcome tracking; rules are permanent but precedent can be invalidated by new counter-evidence |
| Agent over-escalates (too noisy) | Medium | Severity calibration; batch notifications into digests; operator can mute categories |
| Agent under-escalates (misses critical issue) | High | Health check on the maintainer itself; "watchdog" metric for time-since-last-escalation |
| MCP tool permissions too broad | Medium | Separate API key with scoped permissions; audit all agent actions |
| Scraper fixes break working configs | Medium | Always test via `server scrape test` before applying; rollback on failure |
| Memory ecosystem shifts make our format obsolete | Low | JSONL is maximally portable; converting to any future format is a script, not a migration |
| Human decisions captured with low fidelity | Medium | Multiple capture channels (UI inference, chat prompts, manual CLI); accept imperfect records over none |
| Expensive reasoning chains lost on agent crash | Medium | Write decision journal entry before taking action, update with outcome after; crash = incomplete entry, not lost entry |

---

## Open Questions

1. **Scheduling mechanism**: Should maintenance passes be triggered by cron (external),
   River periodic jobs (internal), or the operator running `/maintain` manually?
   Likely: start manual, graduate to River periodic job.

2. **Decision journal retention**: Rules (graduated decisions) are permanent.
   Unconfirmed decisions: propose 6 months. Routine entries with no novel reasoning:
   90 days. But since JSONL files are small, "keep everything" may be simpler than
   building a retention policy.

3. **Multi-node considerations**: If multiple SEL nodes exist, should decision journals
   be federated? Probably out of scope for now (single-node focus), but JSONL files
   could be shared via git or rsync trivially.

4. **Agent identity**: Should each agent type have its own API key and audit trail, or
   share one? Propose: one key per agent role (review-agent, metrics-watcher, etc.)
   for auditability.

5. **Confidence threshold governance**: Who adjusts the thresholds — the agent itself
   (based on outcomes), the operator (manually), or both? Propose: agent proposes,
   operator approves (via `/maintain decisions calibrate`).

6. **Cost management**: Running agents costs money (LLM API calls). How do we ensure
   the maintenance loop doesn't run up excessive costs? Propose: haiku for frequent
   lightweight checks, sonnet for decisions, opus only for diagnosis escalation.
   The reasoning-chain preservation directly reduces costs over time — cached
   reasoning is free, re-derived reasoning costs tokens.

7. **Agentic memory ecosystem**: The decision journal is deliberately
   framework-agnostic (JSONL files), but the space is moving fast. Should we
   evaluate existing tools before building? Candidates to survey:
   - [Mem0](https://github.com/mem0ai/mem0) — agent memory with automatic extraction
   - [Letta/MemGPT](https://github.com/letta-ai/letta) — stateful agent memory
   - MCP-native memory servers (emerging)
   - Simple vector stores (pgvector, already in our stack) for semantic precedent search

   Decision: survey briefly, but default to JSONL unless something is both simple
   AND interoperable. Most memory frameworks today are tightly coupled to specific
   agent runtimes. Our JSONL approach is boring but portable.

8. **Human decision capture fidelity**: How much friction is acceptable to capture
   human reasoning? The post-resolution prompt ("what was your reasoning?") is
   lowest friction but optional. Should it be required for escalation resolution?
   Propose: strongly encouraged, not required. An inferred decision from admin UI
   actions is better than nothing.

9. **Outcome confirmation**: How do we know a decision was correct? For review
   decisions: event stays published, no corrections needed within N days. For
   scraper fixes: next scrape run succeeds. For data quality: no regressions.
   These heuristics need to be defined per category and could be automated.

---

## Relationship to Existing Specs

- **001-sel-backend**: This spec builds on the event lifecycle, review queue, and
  provenance tracking defined there.
- **002-mcp-server**: The MCP tools proposed here extend the existing MCP server.
  New tools follow the same patterns (list/get unification, JSON-LD context resources).
- **003-scraper**: The scraper health agent directly manages the scraper pipeline
  defined in this spec. The `/configure-source` workflow is reused for selector fixes.
