---
name: togather-events-install
description: Set up automated daily event curation from your city's Togather SEL instance — poll change feed, curate against your preferences, deliver to your messaging platform. Use when your city has a Togather server and you want daily event picks delivered to Telegram, Matrix, Discord, or any Hermes-supported platform.
license: MIT
compatibility: Requires Hermes with cron, terminal, and file toolsets. Requires curl and Python 3.9+ on the host. Requires a running Togather SEL instance with API access.
metadata:
  author: togather-foundation
  version: "1.0"
  hermes:
    tags: [togather, events, curation, cron, discovery]
    category: productivity
    related_skills: [togather-events-search]
---

# Togather Events — Install

Sets up a three-layer automated event curation pipeline from a Togather
Shared Events Library (SEL) instance.

**Layer 1 — Data collection** (`togather-poll.py`): A zero-dependency Python
script that polls the Togather change feed, filters by city, deduplicates,
and writes new events to JSONL files. Runs as a Hermes `no_agent` cron script.

**Layer 2 — Daily curation**: An LLM cron job that reads the new events,
scores them against your event profile, and delivers a curated shortlist to
your messaging platform (Matrix, Telegram, Discord, etc.).

**Layer 3 — Weekly digest**: An LLM cron job that reads the week's accumulated
curated events and produces a longer digest with "tell a friend" picks.

After running this skill you will receive daily event recommendations
tailored to your tastes, plus a weekly roundup.

## When to Use

- Your city has a Togather SEL server (check with your local volunteer operator)
- You want daily event picks delivered to your chat app
- You want to automate event discovery without checking websites manually
- You have Hermes running with cron scheduling enabled

## Prerequisites

- **Hermes** with gateway configured (Telegram, Matrix, Discord, etc.)
- **Terminal tools** enabled (`hermes tools enable terminal`)
- **File tools** enabled (`hermes tools enable file`)
- **A Togather API key** — get one from your city's volunteer operator, or
  create one via `server api-key create <name>` if you run the instance
- **Your city's Togather server URL** — ask your operator for this
- **Python 3.9+** and **curl** on the host machine

## Procedure

`SKILL_DIR` refers to the directory containing this SKILL.md file.

### Phase 1: Install the Poll Script

Copy the poll script to Hermes' scripts directory so cron can find it:

```bash
mkdir -p ~/.hermes/scripts
cp SKILL_DIR/scripts/togather-poll.py ~/.hermes/scripts/
chmod +x ~/.hermes/scripts/togather-poll.py
```

Test it with a dry run (replace the values with your actual server and key):

```bash
python3 ~/.hermes/scripts/togather-poll.py \
  --server https://your-instance.example.com \
  --api-key YOUR_API_KEY \
  --city "Your City"
```

Expected output: a JSON summary like `{"status":"ok","new_events":...}`.
If you get an error, check your server URL and API key.

### Phase 2: Create Your Event Profile

Copy the template and edit it:

```bash
mkdir -p ~/.hermes/togather
cp SKILL_DIR/assets/event-profile-template.md ~/.hermes/togather/event-profile.md
```

Open `~/.hermes/togather/event-profile.md` and fill in:
- Venues you care about
- Interest areas (ranked HIGH / MEDIUM / LOW)
- Collaborators whose events you always want to see
- Skips (categories to never recommend)
- Practical constraints (neighborhood, transit, timing)

Delete any sections you don't need. The curation agent handles missing
sections gracefully. The more specific you are, the better the picks.

### Phase 3: Create the Daily Curation Cron Job

Use the `cronjob` tool to create a daily job that collects and curates
events. Replace `YOUR_SERVER`, `YOUR_API_KEY`, and `YOUR_CITY` with your
actual values. For `deliver`, use your platform of choice (e.g., `matrix:!room:server.org`
for Matrix, `telegram` for Telegram, `discord:#channel` for Discord).

```
cronjob(
  action="create",
  name="Togather Daily Curation",
  schedule="0 8 * * *",
  deliver="YOUR_DELIVERY_TARGET",
  skills=["togather-events-search"],
  enabled_toolsets=["terminal", "file", "web"],
  prompt="""You are the Togather daily event curator.

STEP 1 — Collect new events
Run the poll script:
  python3 ~/.hermes/scripts/togather-poll.py \
    --server YOUR_SERVER \
    --api-key YOUR_API_KEY \
    --city "YOUR_CITY"

If the output says new_events: 0, reply with [SILENT] and stop.

STEP 2 — Read your curation profile
Read ~/.hermes/togather/event-profile.md for taste, venues, and skips.

STEP 3 — Curate
Read ~/.hermes/togather/daily_new.jsonl using read_file() in chunks
(30 lines at a time to stay under the 100K char limit). For each event:

1. Check if the venue matches a HIGH priority venue → auto-flag.
2. Check if the name or description matches HIGH interest areas.
3. Check against skips — discard immediately.
4. Check dedup: load ~/.hermes/togather/daily_mentioned.json (if it exists)
   and skip any ULID already mentioned this week.

Pick 3-7 events that best match the profile. For each, extract:
- name, startDate, venue name, street address, URL

STEP 4 — Deliver
Format as a conversational message (not a listing). For each pick:
- Bold the event name, followed by day-of-week and time if relevant
- One sentence on why it matches the profile
- Street address only (no city/province/postal code)
- Event URL for more details

Use **bold** for section labels. Bullet lists with -. Blank lines
between sections. No tables, no decorative ASCII lines.

If nothing is worth flagging, reply with [SILENT].

STEP 5 — Save state
Write the mentioned ULIDs to ~/.hermes/togather/daily_mentioned.json
so they won't be repeated in subsequent daily runs this week."""
)
```

**Important:** The cron agent has no persistent memory. Every instruction the
agent needs MUST be in the prompt above. If you want to change the curation
behavior (different formatting, different schedule), edit the prompt when
you update the cron job.

### Phase 4: Create the Weekly Digest Cron Job

Create a second cron job for the Monday morning digest:

```
cronjob(
  action="create",
  name="Togather Weekly Digest",
  schedule="30 9 * * 1",
  deliver="YOUR_DELIVERY_TARGET",
  skills=["togather-events-search"],
  enabled_toolsets=["terminal", "file", "web"],
  prompt="""You are the Togather weekly digest curator.

STEP 1 — Gather the week's curated events
Read ~/.hermes/togather/daily_curated.jsonl. This was populated by
the daily curation runs all week. Each line is a full event JSON-LD.

If the file is empty or doesn't exist, reply with [SILENT] and stop.

STEP 2 — Curate the digest
Read ~/.hermes/togather/event-profile.md for taste.
Pick 5-10 events that best represent the week. For each, write one
sentence on WHY it's worth attending — like telling a friend.

Dedup: load ~/.hermes/togather/last_recommended.json (if it exists)
and skip any ULID already in a previous weekly digest.

STEP 3 — Format
Deliver as a friendly weekly roundup. Group by day or mood.
For each pick: bold name, abbreviated date (Fri, Sat), venue name,
one-line why, event URL.

Use **bold** for day headers. Bullet lists with -. Blank lines
between sections. No tables, no decorative ASCII.

STEP 4 — Cleanup
After delivering:
- Clear ~/.hermes/togather/daily_curated.jsonl (use write_file to
  write an empty string)
- Clear ~/.hermes/togather/daily_mentioned.json
- Write this week's picks to ~/.hermes/togather/last_recommended.json
  (as a JSON array of ULID strings) for cross-week dedup"""
)
```

### Phase 5: Install the Search Skill (Optional)

For ad-hoc event queries, install the companion search skill:

```bash
hermes skills install togather-events-search
```

If the skill isn't in the public registry yet, install it from the server repo:

```bash
hermes skills tap add Togather-Foundation/server
hermes skills install togather-events-search
```

### Phase 6: Verify

1. **Check the poll script works**:
   ```bash
   python3 ~/.hermes/scripts/togather-poll.py \
     --server YOUR_SERVER --api-key YOUR_API_KEY --city "YOUR CITY"
   ```
   Should output a JSON summary with `new_events` count.

2. **Check cron jobs are scheduled**:
   ```
   cronjob(action="list")
   ```
   Look for "Togather Daily Curation" and "Togather Weekly Digest" with
   `enabled: true`.

3. **Trigger a test run** (optional):
   ```
   cronjob(action="run", job_id="<daily-job-id>")
   ```
   This forces the daily cron to fire immediately so you can verify the
   full pipeline without waiting for 8am.

4. **Check the search skill**: Load it with `/skill togather-events-search`
   and query for today's events.

## Pitfalls

| Symptom | Cause | Fix |
|---------|-------|-----|
| Poll script returns `new_events: 0` every day | Cursor already consumed all past events | Delete `~/.hermes/togather/cursor.txt` to reset the cursor |
| Daily cron produces duplicate picks | Dedup file not being written | Check that the cron prompt's STEP 5 is saving to `daily_mentioned.json` |
| Cron job can't find the script | Script not in `~/.hermes/scripts/` | Re-run Phase 1 copy command |
| Poll script returns HTTP error | Wrong server URL or API key | Verify with `curl -H "Authorization: Bearer KEY" YOUR_SERVER/api/v1/feeds/changes` |
| `daily_new.jsonl` exceeds read limit | Too many new events for one `read_file()` call | The prompt uses chunked reads (30 lines); reduce to 20 if still too large |
| Weekly digest shows already-seen events | `last_recommended.json` not written | Check that STEP 4 cleanup is running; manually create the file if missing |
| City filter lets through wrong-city events | Non-local city list doesn't cover a city in your feed | Edit `togather-poll.py` and add the city to `DEFAULT_NON_LOCAL_CITIES` |

### Cron Prompt Specifics

The daily and weekly cron prompts above are starting points. You will
likely want to customize them after a few runs:

- **Formatting**: Adjust the delivery format for your platform (Matrix
  has different quirks than Telegram or Discord).
- **Curation logic**: Add more specific scoring rules, venue priorities,
  or skip patterns in the prompt if the event profile alone isn't enough.
- **Schedule**: Change the cron schedule from `0 8 * * *` to whatever
  time works for your timezone.

The prompts are self-contained — every time you edit one, make sure all
instructions the agent needs are still in the prompt. The cron agent has
no memory of previous runs.

## Verification

After installation, confirm each layer works:

1. **Layer 1**: Run `python3 ~/.hermes/scripts/togather-poll.py --server ...` —
   should produce valid JSON with `new_events` count.
2. **Layer 2**: `cronjob(action="list")` — daily job exists and is enabled.
3. **Layer 3**: `cronjob(action="list")` — weekly job exists and is enabled.
4. Verify `~/.hermes/togather/event-profile.md` exists and has your preferences.
5. (Optional) Trigger a test run to verify end-to-end delivery.
