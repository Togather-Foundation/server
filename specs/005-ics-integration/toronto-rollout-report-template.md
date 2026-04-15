# Toronto ICS Rollout Report — Cohort N

**Date**: YYYY-MM-DD
**Cohort**: [1 = Net-New High-Value | 2 = Overlap Validation]
**Branch/Commit**: 

## Summary

| Metric | Value |
|--------|-------|
| Sources attempted | |
| Onboarded | |
| Deferred | |
| Blocked | |
| Non-starters | |
| Median setup time (minutes) | |

## Recurring Source Verification

- [ ] `event_series` rows populated with `series_start_date`, `schedule_timezone`
- [ ] `eventSchedule` present in `/api/v1/events/{id}` JSON-LD for recurring sources
- [ ] Re-scraping same source upserts via `external_key` (no duplicates)

### Recurring Source Detail

| source_key | RRULE present? | `event_series` row? | `eventSchedule` in API? | Notes |
|------------|---------------|---------------------|------------------------|-------|
| | | | | |

## Per-Source Outcomes

| source_key | display_name | outcome | setup_time_min | notes | next_action |
|------------|-------------|---------|----------------|-------|-------------|
| | | | | | |

## Blocked Sources — Root Causes

| source_key | feed_url | error_observed | date_attempted | next_action | follow_up_bead |
|------------|----------|----------------|---------------|-------------|---------------|
| | | | | | |

## Overlap Comparison (Cohort 2 Only)

| source_key | ICS event count | HTML event count | coverage % | dedup working? | decision | rationale |
|------------|----------------|----------------|-----------|---------------|----------|-----------|
| | | | | | | |

## Observations & Lessons

### What Worked

(Platform-specific notes: which feed types onboarded smoothly, which needed
no special handling, which produced recurring events correctly.)

### What Surprised

(Unexpected feed shapes, timezone issues, encoding problems, platform quirks.)

### Platform-Specific Notes

(Per feed type: WordPress Tribe, Meetup, Tockify, Google Calendar, etc.
Include specific guidance for future onboarding of same platform type.)

## Follow-up Beads Created

| Bead ID | Title | Priority | Component |
|---------|-------|----------|-----------|
| | | | |