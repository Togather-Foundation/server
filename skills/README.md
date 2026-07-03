# Agent Skills for Togather SEL

[Agent Skills](https://agentskills.io) for Togather SEL. These skills teach AI agents (Hermes, Claude Code, Codex, OpenCode, and others) how to get event picks, search for events, and review event queues.

## For City Residents

If your city has a Togather server (ask your local volunteer operator):

### [togather-events-install](togather-events-install/)
Setup wizard for automated daily event curation. Installs a polling script, creates cron jobs for daily picks and weekly digests, and walks you through configuring your event profile. The fastest path from "my city has Togather" to "I get event recommendations in my chat."

### [togather-events-search](togather-events-search/)
Ad-hoc event queries via the public API. Domain, keyword, city, and date filters. For when you want to search events without the full curation pipeline.

## For Server Operators

If you run a Togather SEL instance:

### [togather-review](togather-review/)
Guidebook for triaging the event review queue. Warning types, batch strategies, source patterns, and CLI reference. For server operators doing data janitor work.

### [togather-server-ops](togather-server-ops/)
Authentication, REST API reference, MCP connectivity, and data quality patterns. The operator's reference manual for the server's interfaces.

## Install

### Hermes

Add this repo as a skill source, then install individual skills:

```bash
hermes skills tap add Togather-Foundation/server
hermes skills install togather-events-install
hermes skills install togather-events-search
hermes skills install togather-review
hermes skills install togather-server-ops
```

Or install them all:

```bash
hermes skills tap add Togather-Foundation/server
hermes skills install togather-events-install togather-events-search togather-review togather-server-ops
```

### Other Agents

Copy the skill directory you want into your agent's skills folder. Each skill is a self-contained directory with a `SKILL.md` file following the [Agent Skills specification](https://agentskills.io/specification).

## Prerequisites

- A running Togather SEL server
- API keys: agent/public scope for queries, admin scope for review queue operations
- For the curation pipeline: Hermes with cron scheduling, Python 3.9+, curl

## Format

All skills follow the [Agent Skills v1 specification](https://agentskills.io/specification). Each is a directory containing a `SKILL.md` with YAML frontmatter (name, description, license, compatibility, metadata) and Markdown body. Optional `scripts/`, `references/`, and `assets/` directories as needed.
