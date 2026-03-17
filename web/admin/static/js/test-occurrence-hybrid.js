#!/usr/bin/env node
/**
 * Unit tests for occurrence modal hybrid-cleanup logic.
 *
 * These tests require() occurrence-logic.js directly — the module that
 * event-edit.js also loads via OccurrenceLogic.buildOccurrenceFromForm.
 * Any change to that production module will be reflected here immediately.
 *
 * Run: node web/admin/static/js/test-occurrence-hybrid.js
 */
'use strict';

// ---------------------------------------------------------------------------
// Load the real production module
// ---------------------------------------------------------------------------
const path = require('path');
const { buildOccurrenceFromForm, buildOccurrenceFields } =
    require(path.join(__dirname, 'occurrence-logic.js'));

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------
let passed = 0;
let failed = 0;

function assert(cond, desc) {
    if (cond) {
        console.log(`  ✓ ${desc}`);
        passed++;
    } else {
        console.error(`  ✗ FAIL: ${desc}`);
        failed++;
    }
}

/** Build a minimal form object with sensible defaults, spread overrides. */
function makeForm(overrides) {
    return {
        indexValue:    '',
        startTime:     '2026-04-01T19:00',
        endTime:       '',
        timezone:      'America/Toronto',
        doorTime:      '',
        venueId:       '',
        virtualUrlRaw: '',
        ...overrides,
    };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

console.log('\noccurrence-logic.js — hybrid-cleanup and form build\n');

// 1. Normal physical-only save (no virtual URL) — must still work.
console.log('1. Normal physical-only occurrence saves without error');
{
    const res = buildOccurrenceFromForm(makeForm({
        venueId: 'https://example.com/places/01HXX12345678901234567890',
    }), []);
    assert(res.ok, 'save succeeds');
    assert(res.occurrence.venue_id === 'https://example.com/places/01HXX12345678901234567890', 'venue_id preserved');
    assert(res.occurrence.virtual_url === null, 'virtual_url is null');
}

// 2. Normal virtual-only save — must still work.
console.log('\n2. Normal virtual-only occurrence saves without error');
{
    const res = buildOccurrenceFromForm(makeForm({
        virtualUrlRaw: 'https://stream.example.com/live',
    }), []);
    assert(res.ok, 'save succeeds');
    assert(res.occurrence.virtual_url === 'https://stream.example.com/live', 'virtual_url preserved');
    assert(res.occurrence.venue_id === null, 'venue_id is null');
}

// 3. KEY REGRESSION: hybrid occurrence (legacy bad data — both venueId AND virtualUrl set).
//    Must save as physical-only (venue kept, virtual URL silently cleared).
console.log('\n3. Hybrid occurrence (both venueId + virtualUrlRaw set) saves as physical-only');
{
    const res = buildOccurrenceFromForm(makeForm({
        venueId:       'https://example.com/places/01HXX12345678901234567890',
        virtualUrlRaw: 'https://old-stream.example.com/live',
    }), []);
    assert(res.ok, 'save succeeds (no longer blocked)');
    assert(res.occurrence.venue_id === 'https://example.com/places/01HXX12345678901234567890', 'venue_id kept');
    assert(res.occurrence.virtual_url === null, 'stale virtual_url silently cleared');
}

// 4. Hybrid edit at a specific index — existing occurrence updated in place, id preserved.
console.log('\n4. Hybrid occurrence edit at index 0 updates in-place and preserves id');
{
    const existing = [{ id: 'occ-01', start_time: '2026-04-01T19:00', virtual_url: 'https://old.example.com', venue_id: 'https://example.com/places/01HXX12345678901234567890' }];
    const res = buildOccurrenceFromForm(makeForm({
        indexValue:    '0',
        venueId:       'https://example.com/places/01HXX12345678901234567890',
        virtualUrlRaw: 'https://old.example.com', // stale hidden value
    }), existing);
    assert(res.ok, 'save succeeds');
    assert(res.occurrence.id === 'occ-01', 'existing id preserved');
    assert(res.occurrence.venue_id !== null, 'venue_id kept');
    assert(res.occurrence.virtual_url === null, 'stale virtual_url cleared');
}

// 5. Missing start time — must still fail.
console.log('\n5. Missing start time still fails with descriptive reason');
{
    const res = buildOccurrenceFromForm(makeForm({ startTime: '' }), []);
    assert(!res.ok, 'save blocked');
    assert(typeof res.reason === 'string' && res.reason.length > 0, 'reason string provided');
    assert(res.reason.toLowerCase().includes('start time'), 'reason mentions start time');
}

// 6. Clean state: no venue, no virtual URL — saves cleanly.
console.log('\n6. No venue, no virtual URL saves cleanly (minimal occurrence)');
{
    const res = buildOccurrenceFromForm(makeForm({}), []);
    assert(res.ok, 'save succeeds');
    assert(res.occurrence.venue_id === null, 'venue_id null');
    assert(res.occurrence.virtual_url === null, 'virtual_url null');
}

// 7. REVERSE PATH: start from hybrid legacy data, clear venue override, save as virtual-only.
//    The admin clicks "clear venue override" then saves — virtual URL must be preserved.
console.log('\n7. Reverse path: clear venue override → save preserves virtual URL');
{
    // Simulates: hybrid occurrence loaded into modal (venueId + virtualUrlRaw set by editOccurrence),
    // then admin clicks clearOccurrenceVenue() which blanks occurrence-venue-id but leaves
    // occurrence-virtual-url intact, then saves.
    const existing = [{ id: 'occ-02', start_time: '2026-04-01T19:00', virtual_url: 'https://stream.example.com/live', venue_id: 'https://example.com/places/01HXX12345678901234567890' }];
    const res = buildOccurrenceFromForm(makeForm({
        indexValue:    '0',
        venueId:       '',                                    // cleared by clearOccurrenceVenue()
        virtualUrlRaw: 'https://stream.example.com/live',    // retained from original data
    }), existing);
    assert(res.ok, 'save succeeds after venue clear');
    assert(res.occurrence.id === 'occ-02', 'id preserved');
    assert(res.occurrence.venue_id === null, 'venue_id cleared');
    assert(res.occurrence.virtual_url === 'https://stream.example.com/live', 'virtual_url preserved when no venue');
}

// 8. Reverse path: clear venue override, then also clear virtual URL — saves as bare occurrence.
console.log('\n8. Reverse path: clear venue and virtual URL → saves as minimal bare occurrence');
{
    const existing = [{ id: 'occ-03', start_time: '2026-04-01T19:00', virtual_url: 'https://stream.example.com/live', venue_id: 'https://example.com/places/01HXX12345678901234567890' }];
    const res = buildOccurrenceFromForm(makeForm({
        indexValue:    '0',
        venueId:       '',   // cleared
        virtualUrlRaw: '',   // admin also cleared the URL
    }), existing);
    assert(res.ok, 'save succeeds');
    assert(res.occurrence.venue_id === null, 'venue_id null');
    assert(res.occurrence.virtual_url === null, 'virtual_url null (admin explicitly cleared it)');
}

// 9. buildOccurrenceFields unit — low-level guard function directly.
console.log('\n9. buildOccurrenceFields pure guard function');
{
    // Both set → virtual_url dropped
    const hybrid = buildOccurrenceFields({ venueId: 'v1', virtualUrlRaw: 'u1' });
    assert(hybrid.venue_id === 'v1', 'venue_id kept');
    assert(hybrid.virtual_url === null, 'virtual_url dropped when venueId present');

    // Only virtual → both kept correctly
    const virtual = buildOccurrenceFields({ venueId: '', virtualUrlRaw: 'u1' });
    assert(virtual.venue_id === null, 'venueId normalized to null');
    assert(virtual.virtual_url === 'u1', 'virtual_url kept when no venue');

    // Neither → both null
    const bare = buildOccurrenceFields({ venueId: '', virtualUrlRaw: '' });
    assert(bare.venue_id === null, 'null venue_id');
    assert(bare.virtual_url === null, 'null virtual_url');
}

// 10. Whitespace-only venue and virtual values are treated as absent.
console.log('\n10. Whitespace-only venue/virtual values normalised to null');
{
    const wsVenue = buildOccurrenceFields({ venueId: '   ', virtualUrlRaw: 'u1' });
    assert(wsVenue.venue_id === null, 'whitespace-only venueId → null');
    assert(wsVenue.virtual_url === 'u1', 'virtual_url kept when venueId is blank');

    const wsVirtual = buildOccurrenceFields({ venueId: '', virtualUrlRaw: '   ' });
    assert(wsVirtual.venue_id === null, 'venueId null');
    assert(wsVirtual.virtual_url === null, 'whitespace-only virtualUrlRaw → null');

    const wsBoth = buildOccurrenceFields({ venueId: '  ', virtualUrlRaw: '\t' });
    assert(wsBoth.venue_id === null, 'both whitespace-only → venue_id null');
    assert(wsBoth.virtual_url === null, 'both whitespace-only → virtual_url null');

    // The form-level function also trims correctly.
    const formRes = buildOccurrenceFromForm(makeForm({
        venueId: '   ',
        virtualUrlRaw: '  ',
    }), []);
    assert(formRes.ok, 'save succeeds with whitespace-only inputs');
    assert(formRes.occurrence.venue_id === null, 'form: venue_id null after trim');
    assert(formRes.occurrence.virtual_url === null, 'form: virtual_url null after trim');
}

// 11. Browser / global-export smoke test.
//     Simulates the browser loading path: window.OccurrenceLogic is set by the UMD
//     shim, not by require().  We re-load the module with a fake globalThis/root that
//     has no module system so the else-branch (browser export) is exercised.
console.log('\n11. Browser global-export (window.OccurrenceLogic) smoke test');
{
    // Isolate: load the file source and evaluate it with a fresh fakeRoot (= window).
    const fs   = require('fs');
    const vm   = require('vm');
    const src  = fs.readFileSync(path.join(__dirname, 'occurrence-logic.js'), 'utf8');

    const fakeRoot = {};   // stand-in for window / globalThis
    const sandbox  = { globalThis: fakeRoot };   // module.exports is absent → browser branch

    vm.runInNewContext(src, sandbox);

    assert(typeof fakeRoot.OccurrenceLogic === 'object' && fakeRoot.OccurrenceLogic !== null,
        'window.OccurrenceLogic is set by UMD browser branch');
    assert(typeof fakeRoot.OccurrenceLogic.buildOccurrenceFromForm === 'function',
        'window.OccurrenceLogic.buildOccurrenceFromForm is a function');
    assert(typeof fakeRoot.OccurrenceLogic.buildOccurrenceFields === 'function',
        'window.OccurrenceLogic.buildOccurrenceFields is a function');

    // Quick sanity check that the browser-exported functions actually work.
    const res = fakeRoot.OccurrenceLogic.buildOccurrenceFromForm({
        indexValue: '', startTime: '2026-06-01T10:00', endTime: '', timezone: 'UTC',
        doorTime: '', venueId: '', virtualUrlRaw: '',
    }, []);
    assert(res.ok, 'browser-exported buildOccurrenceFromForm returns ok result');
    assert(res.occurrence.venue_id === null,  'browser path: venue_id null');
    assert(res.occurrence.virtual_url === null, 'browser path: virtual_url null');
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------
console.log(`\n${'─'.repeat(50)}`);
console.log(`Results: ${passed} passed, ${failed} failed`);
if (failed > 0) {
    process.exit(1);
}
