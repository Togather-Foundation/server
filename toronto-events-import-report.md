# Toronto Open Data Events Import Report

**Date:** February 7, 2026  
**Environment:** Staging (staging.toronto.togather.foundation)  
**Source:** https://civictechto.github.io/toronto-opendata-festivalsandevents-jsonld-proxy/all.jsonld

---

## Executive Summary

Successfully imported **619 of 1,671 Toronto Open Data events** (37%) to staging.

- ✅ **619 events created** (75.6% success rate of submitted events)
- 🔄 **102 duplicates detected** (12.5%)
- ❌ **21 events failed** (2.6%)
- ⚠️ **7 batches rejected** (350 events not submitted due to validation errors)
- 📊 **15 batches successful**, 7 batches failed at submission

---

## Import Statistics

### Submission Phase

| Metric | Count | Percentage |
|--------|-------|-----------|
| **Total events in source** | 1,671 | 100% |
| **Batches submitted** | 15 | 68% |
| **Batches rejected (HTTP 400)** | 7 | 32% |
| **Events submitted** | 742 | 44% |
| **Events not submitted** | 929 | 56% |

### Processing Phase (of 742 submitted)

| Metric | Count | Percentage |
|--------|-------|-----------|
| **Created successfully** | 619 | 83.4% |
| **Duplicates detected** | 102 | 13.7% |
| **Failed validation** | 21 | 2.8% |

---

## Failure Analysis

### Failure Breakdown by Type

| Error Type | Count | % of Failures |
|------------|-------|---------------|
| **Invalid URL** | 19 | 90.5% |
| **Reversed dates** | 1 | 4.8% |
| **Missing source.eventId** | 1 | 4.8% |
| **Total** | 21 | 100% |

### 1. Invalid URL Errors (19 failures)

**Error:** `invalid url: invalid URI`

**Root Cause:** URLs in Toronto Open Data are missing protocol prefix (http:// or https://)

**Examples:**
- `www.gladstonehouse.ca` (should be `https://www.gladstonehouse.ca`)
- `www.thebroadviewhotel.ca` (should be `https://www.thebroadviewhotel.ca`)
- `heliconianclub.org` (should be `https://heliconianclub.org`)
- `a.tixlink.co/wizard` (should be `https://a.tixlink.co/wizard`)

**Total in source dataset:** ~29 events with malformed URLs

**Recommendation:** Update `scripts/ingest-toronto-events.sh` to prepend `https://` to URLs that:
- Start with `www.`
- Don't start with `http://` or `https://`
- Don't start with `mailto:`

### 2. Reversed Date Error (1 failure)

**Error:** `invalid endDate: must be on or after startDate`

**Event:** Weston Farmers Market
- Start: 2025-06-08T00:30:00.000Z (June 8, 12:30 AM)
- End: 2025-06-07T17:00:00.000Z (June 7, 5:00 PM)

**Root Cause:** The event legitimately has endDate before startDate in source data. This is NOT a timezone error (end is ~7.5 hours BEFORE start, not a midnight-crossing issue).

**Note:** Our normalization logic SHOULD have corrected this if it met timezone error criteria:
- Early morning end (0-4 AM) ✅ (00:30 is within range)
- But the corrected duration would be ~24.5 hours, which exceeds the 7-hour threshold ❌

**Total in source dataset:** 4 events with reversed dates

**Recommendation:** This is a data quality issue in the source. The event has bad data that doesn't match our timezone error pattern. Could be:
1. Entry error (dates swapped)
2. Multi-day event with incorrect dates

### 3. Missing source.eventId (1 failure)

**Error:** `invalid source.eventId: required`

**Event:** The East African Experience 2025
- URL: `https://www.instagram.com/theeastafricanexperience?utm_source=ig_web_button_share_sheet&igsh=ZDNlZDc0MzIxNw==`

**Root Cause:** The jq transform in `scripts/ingest-toronto-events.sh` failed to extract an eventId from the Instagram URL. The regex patterns don't match Instagram profile URLs.

**Recommendation:** Update the source.eventId extraction logic to handle:
- Instagram profile URLs (use username as eventId)
- Other social media URLs
- Or fallback to hash of the URL

---

## Batch Submission Failures (7 batches, ~350 events)

**Error:** HTTP 400 "Bad Request" with validation-error type

These batches were rejected at the API level before individual event processing. This means the entire batch JSON was malformed or contained events that failed pre-processing validation.

**Affected batches:**
- Batch 12 (events 550-599)
- Batch 14 (events 650-699)
- Batch 15 (events 700-749)
- Batch 16 (events 750-799)
- Batch 17 (events 800-849)
- Batch 18 (events 850-899)
- Batch 19 (events 900-949)

**Likely causes:**
1. Events with null/missing required fields (name, url, startDate)
2. Events with malformed JSON after jq transformation
3. Events with invalid date formats

**Recommendation:** Add debug logging to capture batch rejection details and inspect the raw events in these ranges.

---

## Data Quality Observations

### Source Data Issues (Toronto Open Data)

1. **Malformed URLs:** ~29 events (1.7%) missing protocol prefix
2. **Reversed dates:** 4 events (0.2%) with endDate < startDate
3. **Missing required fields:** Unknown count (caused 7 batch rejections)

### Duplicate Detection Working

- **102 duplicates found** across 742 submitted events (13.7%)
- Duplicates correctly identified against existing 200 test events
- Deduplication by source URL + external ID working as expected

---

## Recommendations

### Immediate (Fix Known Issues)

1. **Update URL normalization in ingest script:**
   ```bash
   # In scripts/ingest-toronto-events.sh, add URL fixing:
   url: (
     .url | 
     if type == "string" then
       if startswith("www.") then "https://" + .
       elif startswith("http") or startswith("mailto:") then .
       else "https://" + .
       end
     else null
     end
   )
   ```

2. **Improve source.eventId extraction:**
   ```bash
   # Add Instagram username extraction:
   if test("instagram.com/([^/?]+)") then
     (match("instagram.com/([^/?]+)") | .captures[0].string)
   # ... existing logic ...
   ```

3. **Add null field filtering:**
   ```bash
   # Filter out events missing critical fields earlier:
   map(select(
     .name != null and 
     .url != null and 
     .startDate != null and
     (.location != null or .virtualLocation != null)
   ))
   ```

### Medium Term (Handle Edge Cases)

1. **Expand reversed date correction criteria:**
   - Consider allowing up to 12-hour corrected duration (not just 7h)
   - Or send to review queue instead of hard rejection
   - Add warning for "unusual duration after correction"

2. **Add batch validation pre-flight:**
   - Validate each event before adding to batch
   - Log validation errors to separate file
   - Skip invalid events instead of failing entire batch

3. **Enhance error reporting:**
   - Include event name in batch status results
   - Add structured error details (field, value, constraint)
   - Generate CSV of failures for easy analysis

---

## Current Database State

**After import:**
- Total events: 819 (200 test + 619 Toronto)
- Total places: ~110 (10 test + ~100 Toronto venues)
- Total organizations: ~110 (10 test + ~100 Toronto)
- Total sources: ~58 (8 test + ~50 Toronto sources)

**Success rate:** 75.6% of submitted events imported successfully

---

## Next Steps

1. ✅ **Complete:** Import Toronto Open Data events (619/1671 = 37%)
2. ⏭️ **Next:** Fix URL and source.eventId issues in ingest script
3. ⏭️ **Next:** Re-run import to capture the ~350 events from failed batches
4. ⏭️ **Next:** Investigate the 21 individual failures in detail
5. ⏭️ **Next:** Consider sending reversed date events to review queue instead of rejecting

---

## Files Generated

- `toronto-events-import-report.md` - This report
- Batch IDs: See script output for curl commands to inspect individual batches

---

**Report generated:** February 7, 2026  
**Server version:** 88e379f (feature/complete-dedupe-fixes)
