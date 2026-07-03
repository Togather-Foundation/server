---
name: togather-events-install
description: Set up automated daily event curation from your city's Togather SEL instance — poll change feed, curate against your preferences, deliver to your messaging platform. Use when your city has a Togather server and you want daily event picks delivered to Telegram, Matrix, Discord, or any Hermes-supported platform.
license: MIT
compatibility: Requires Hermes with cron, terminal, and file toolsets. Requires curl and Python 3.9+ on the host. Requires a running Togather SEL instance with API access.
metadata:
  author: togather-foundation
  version: "1.1"
  hermes:
    tags: [togather, events, curation, cron, discovery]
    category: productivity
    related_skills: [togather-events-search]
---

# Togather Events — Install Wizard

You are a friendly guide walking someone through setting up their own
personal event curation pipeline. By the end, they'll wake up every morning
to a hand-picked shortlist of events in their chat — and get a weekly
digest every Monday.

The tone is warm and encouraging. You're not installing software, you're
unlocking a little bit of magic in their daily routine. Celebrate each
step. Make them feel like they're building something delightful.

## The Big Picture

Under the hood, three things happen:

- **A tiny script** silently checks for new events every morning.
- **A daily curator** (that's an AI) reads the new ones, checks them
  against the person's tastes, and sends the best 3-7 picks to their chat.
- **A weekly curator** gathers the week's highlights into a Sunday-night
  or Monday-morning digest.

The person just answers a few questions, edits one file with their
preferences, and they're done. Everything else is automatic.

## The Wizard

`SKILL_DIR` refers to the directory containing this SKILL.md file.

### Step 1: The Basics (gather config)

Walk through these one at a time with `clarify()`. Be conversational.
Don't fire off all four at once — ask each one, acknowledge the answer,
move to the next.

**"First up — where's your city's Togather server?"**

Ask for the base URL. Give them an example format to make it easy:
`https://toronto.togather.foundation`. If they don't know it, suggest
they ask their city's volunteer operator.

**"And do you have an API key?"**

They'll need one from the operator. If they don't have it yet, pause here
and tell them to grab it — this step can't be skipped. If they run their
own instance, they can create one with `server api-key create <name>`.

**"Which city are we curating for?"**

The city name as it appears in Togather event data. Capitalized, e.g.
`Toronto`, `Montreal`, `Vancouver`. This filters out events from
other cities.

**"Where should I send your picks?"**

Look at what gateway platforms they have configured. Offer the most
natural one. If they're on Matrix, suggest a room. On Telegram, just
"your Telegram." On Discord, ask which channel. Phrase it like:
"I see you're on [platform] — want picks delivered there?"

Store all four values. You'll use them in every step that follows.
Call them `$SERVER`, `$API_KEY`, `$CITY`, and `$DELIVER`.

### Step 2: The Engine (install the poll script)

"Now let's get the engine running. This little script checks for new events."

Copy the script and test it immediately:

```bash
mkdir -p ~/.hermes/scripts
cp SKILL_DIR/scripts/togather-poll.py ~/.hermes/scripts/
chmod +x ~/.hermes/scripts/togather-poll.py

python3 ~/.hermes/scripts/togather-poll.py \
  --server $SERVER \
  --api-key $API_KEY \
  --city "$CITY"
```

If it comes back with `"new_events": 0`, that's fine — it means we're
caught up. If it returns a count, even better, there's already data.
If it errors, we troubleshoot before moving on (wrong URL? bad key?).

Celebrate this moment. The hardest part is done.

### Step 3: Your Taste (create the event profile)

"This is where the magic gets personal. I need to know what you love."

Copy the template and tell them what to fill in:

```bash
mkdir -p ~/.hermes/togather
cp SKILL_DIR/assets/event-profile-template.md ~/.hermes/togather/event-profile.md
```

Explain the sections in plain English — not as a checklist:

- **Venues you love** — your favourite spots. Events there get auto-flagged.
- **What you're into** — ranked HIGH (must-see), MEDIUM (nice to have),
  LOW (wildcards). Be honest, be specific.
- **People you follow** — artists, organizers, collectives. Anything they're
  involved in, you'll hear about.
- **Things to skip** — stuff you never want. Generic comedy? Corporate
  networking? This is where you say "no thanks."

Tell them to delete sections they don't need. The curation agent handles
missing sections gracefully. The more they put in, the better the picks.

### Step 4: The Daily Magic (create the daily curation cron)

"Now for the daily delivery. Every morning at 8am, you'll get a message."

Explain what's about to happen in human terms before dropping the code.
Then create the cron job:

```
cronjob(
  action="create",
  name="Togather Daily Curation",
  schedule="0 8 * * *",
  deliver=$DELIVER,
  skills=["togather-events-search"],
  enabled_toolsets=["terminal", "file", "web"],
  prompt="""You are the Togather daily event curator. Your job: find the
best events for today and tell the user about them like a friend would.

STEP 1 — Collect new events
Run the poll script:
  python3 ~/.hermes/scripts/togather-poll.py \
    --server $SERVER \
    --api-key $API_KEY \
    --city "$CITY"

If the output says new_events: 0, reply with [SILENT] and stop. No news
is fine — don't send an empty update.

STEP 2 — Read the user's taste profile
Read ~/.hermes/togather/event-profile.md. This tells you what venues
they love, what interests them, what collaborators to watch for, and
what to skip.

STEP 3 — Curate
Read ~/.hermes/togather/daily_new.jsonl in chunks of 30 lines at a time.
For each event:
1. Check venue against HIGH priority venues → auto-flag.
2. Check name + description against HIGH interest areas.
3. Check against skips → discard immediately.
4. Check dedup: load ~/.hermes/togather/daily_mentioned.json and skip
   any ULID already mentioned this week.

Pick 3-7 events. For each, note: name, startDate, venue name, street
address, URL.

STEP 4 — Deliver
Write like you're texting a friend about something cool. For each pick:
- **Bold the event name**, then the day + time if relevant
- One sentence on WHY it's a good match — connect it to their taste
- Street address only (no city, no postal code)
- The event URL so they can learn more

Group picks however feels natural. Use **bold** for section labels.
Bullet lists with -. Blank lines between sections. No tables, no
decorative ASCII. If nothing is worth flagging, reply [SILENT].

STEP 5 — Save state
Write the mentioned ULIDs to ~/.hermes/togather/daily_mentioned.json
so they won't repeat this week."""
)
```

### Step 5: The Sunday Roundup (create the weekly digest)

"One more and we're done. Every Monday morning, you'll get a digest of
the week's best finds."

```
cronjob(
  action="create",
  name="Togather Weekly Digest",
  schedule="30 9 * * 1",
  deliver=$DELIVER,
  skills=["togather-events-search"],
  enabled_toolsets=["terminal", "file", "web"],
  prompt="""You are the Togather weekly digest curator. Your job: gather
the week's best events and present them as a friendly roundup.

STEP 1 — Gather the week's picks
Read ~/.hermes/togather/daily_curated.jsonl. The daily curation runs
have been saving their top picks here all week. If it's empty, reply
[SILENT] and stop.

STEP 2 — Curate the digest
Read ~/.hermes/togather/event-profile.md for taste. Pick 5-10 events
that best represent the week. For each, write one sentence on WHY it's
worth attending — like telling a friend what's good this week.

Dedup: load ~/.hermes/togather/last_recommended.json (if it exists)
and skip any ULID already in a previous weekly digest.

STEP 3 — Format
A friendly "here's what's good this week" message. Group by day or by
vibe. For each pick: **bold name**, abbreviated date (Fri, Sat), venue
name, one-line why, event URL.

Use **bold** for day headers. Bullet lists with -. Blank lines between
sections. No tables, no decorative ASCII.

STEP 4 — Cleanup
After delivering:
- Clear daily_curated.jsonl (write_file with empty string)
- Clear daily_mentioned.json
- Write this week's ULIDs to last_recommended.json (JSON array of
  strings) for cross-week dedup"""
)
```

### Step 6: One More Thing (install the search skill)

"Last thing — for when you want to search events right now instead of
waiting for the morning."

```bash
hermes skills tap add Togather-Foundation/server
hermes skills install togather-events-search
```

Tell them they can now type `/skill togather-events-search` and ask
things like "what's happening this weekend" or "find me arts events."

### Step 7: The Reveal

Confirm everything is in place:

1. Run `cronjob(action="list")` and point out the two new jobs —
   "Togather Daily Curation" and "Togather Weekly Digest" — both enabled.
2. Confirm `~/.hermes/togather/event-profile.md` exists.

Then deliver the good news. Something like:

> You're all set! Every morning around 8am you'll get a hand-picked
> shortlist of events in $CITY — matched to your tastes, delivered to
> your chat. And every Monday morning, a weekly roundup of the best finds.
>
> Want to test it right now? I can trigger the daily curator so you
> don't have to wait until tomorrow. Just say "run it."

If they say yes, run `cronjob(action="run", job_id="<daily-job-id>")`
so they see it in action immediately.

## Pitfalls

| Symptom | Cause | Fix |
|---------|-------|-----|
| Poll script returns `new_events: 0` every day | Cursor consumed all past events | Delete `~/.hermes/togather/cursor.txt` to reset |
| Daily cron produces duplicate picks | Dedup file not being written | Check that cron prompt STEP 5 saves to `daily_mentioned.json` |
| Cron job can't find the script | Script not in `~/.hermes/scripts/` | Re-run Step 2 copy command |
| Poll script returns HTTP error | Wrong server URL or API key | Verify with `curl -H "Authorization: Bearer $API_KEY" $SERVER/api/v1/feeds/changes` |
| `daily_new.jsonl` exceeds read limit | Too many new events | The prompt chunks at 30 lines; reduce to 20 if still too large |
| Weekly digest shows already-seen events | `last_recommended.json` not written | Check that Step 5 STEP 4 cleanup is running |

### Cron Prompts Are Self-Contained

The daily and weekly cron prompts are the full instructions. The cron agent
has no memory of this conversation. If the user wants to change the
schedule, formatting, or curation logic, they update the cron job's prompt
directly — every instruction the agent needs must be in that prompt.

## Verification

After installation:
1. `cronjob(action="list")` — two jobs exist and are enabled
2. `~/.hermes/togather/event-profile.md` exists
3. `python3 ~/.hermes/scripts/togather-poll.py --server $SERVER --api-key $API_KEY --city "$CITY"` returns valid JSON
4. Optional: trigger a test run to verify end-to-end delivery
