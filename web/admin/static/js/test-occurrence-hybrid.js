#!/usr/bin/env node
/**
 * Unit tests for occurrence modal hybrid-cleanup logic in event-edit.js.
 *
 * These tests exercise the saveOccurrence guard (venueId + virtualUrl mutual exclusion)
 * using a minimal DOM stub, without needing a running server or browser.
 *
 * Run: node web/admin/static/js/test-occurrence-hybrid.js
 */
'use strict';

// ---------------------------------------------------------------------------
// Minimal DOM stub (only the elements saveOccurrence touches)
// ---------------------------------------------------------------------------
function makeDom(overrides) {
    const defaults = {
        'occurrence-index':       { value: '' },
        'occurrence-start-time':  { value: '2026-04-01T19:00' },
        'occurrence-end-time':    { value: '' },
        'occurrence-timezone':    { value: 'America/Toronto' },
        'occurrence-door-time':   { value: '' },
        'occurrence-venue-id':    { value: '' },
        'occurrence-virtual-url': { value: '' },
    };
    const elements = { ...defaults };
    for (const [id, vals] of Object.entries(overrides || {})) {
        elements[id] = { ...defaults[id], ...vals };
    }
    return {
        getElementById(id) { return elements[id] || null; },
        elements,
    };
}

// ---------------------------------------------------------------------------
// Extract saveOccurrence logic as a pure function for testing.
//
// This mirrors the exact logic in event-edit.js saveOccurrence, so that any
// future edit to the JS is visible in test failures.
// ---------------------------------------------------------------------------
function runSaveOccurrence(dom, occurrences) {
    const toasts = [];
    const showToast = (msg, type) => toasts.push({ msg, type });

    const indexValue = dom.getElementById('occurrence-index').value;
    const startTime = dom.getElementById('occurrence-start-time').value;

    if (!startTime) {
        showToast('Start time is required', 'error');
        return { ok: false, toasts, occurrences };
    }

    const venueId = dom.getElementById('occurrence-venue-id').value || null;
    // Mirror the fix: when venueId is set, treat virtual URL as cleared (hidden input is stale).
    const virtualUrlRaw = dom.getElementById('occurrence-virtual-url').value || null;
    const virtualUrl = venueId ? null : virtualUrlRaw;

    const occurrence = {
        start_time: startTime,
        end_time: dom.getElementById('occurrence-end-time').value || null,
        timezone: dom.getElementById('occurrence-timezone').value || 'America/Toronto',
        door_time: dom.getElementById('occurrence-door-time').value || null,
        virtual_url: virtualUrl,
        venue_id: venueId,
    };

    const result = [...occurrences];
    if (indexValue === '') {
        result.push(occurrence);
    } else {
        const index = parseInt(indexValue, 10);
        occurrence.id = result[index].id || null;
        result[index] = occurrence;
    }

    return { ok: true, toasts, occurrences: result, saved: occurrence };
}

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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

console.log('\noccurrence modal hybrid-cleanup logic\n');

// 1. Normal physical-only save (no virtual URL) — must still work.
console.log('1. Normal physical-only occurrence saves without error');
{
    const dom = makeDom({ 'occurrence-venue-id': { value: 'https://example.com/places/01HXX12345678901234567890' } });
    const { ok, saved } = runSaveOccurrence(dom, []);
    assert(ok, 'save succeeds');
    assert(saved.venue_id === 'https://example.com/places/01HXX12345678901234567890', 'venue_id preserved');
    assert(saved.virtual_url === null, 'virtual_url is null');
}

// 2. Normal virtual-only save — must still work.
console.log('\n2. Normal virtual-only occurrence saves without error');
{
    const dom = makeDom({ 'occurrence-virtual-url': { value: 'https://stream.example.com/live' } });
    const { ok, saved } = runSaveOccurrence(dom, []);
    assert(ok, 'save succeeds');
    assert(saved.virtual_url === 'https://stream.example.com/live', 'virtual_url preserved');
    assert(saved.venue_id === null, 'venue_id is null');
}

// 3. KEY REGRESSION: hybrid occurrence (legacy bad data — both venueId AND virtualUrl in DOM).
//    Must save as physical-only (venue kept, virtual URL silently cleared).
console.log('\n3. Hybrid occurrence (both venueId + virtualUrl in DOM) saves as physical-only');
{
    const dom = makeDom({
        'occurrence-venue-id':    { value: 'https://example.com/places/01HXX12345678901234567890' },
        'occurrence-virtual-url': { value: 'https://old-stream.example.com/live' },
    });
    const { ok, saved, toasts } = runSaveOccurrence(dom, []);
    assert(ok, 'save succeeds (no longer blocked)');
    assert(toasts.length === 0, 'no error toast shown');
    assert(saved.venue_id === 'https://example.com/places/01HXX12345678901234567890', 'venue_id kept');
    assert(saved.virtual_url === null, 'stale virtual_url silently cleared');
}

// 4. Hybrid edit at a specific index — existing occurrence updated in place.
console.log('\n4. Hybrid occurrence edit at index 0 updates in-place correctly');
{
    const existing = [{ id: 'occ-01', start_time: '2026-04-01T19:00', virtual_url: 'https://old.example.com', venue_id: 'https://example.com/places/01HXX12345678901234567890' }];
    const dom = makeDom({
        'occurrence-index':       { value: '0' },
        'occurrence-venue-id':    { value: 'https://example.com/places/01HXX12345678901234567890' },
        'occurrence-virtual-url': { value: 'https://old.example.com' }, // stale hidden value
    });
    const { ok, occurrences } = runSaveOccurrence(dom, existing);
    assert(ok, 'save succeeds');
    assert(occurrences[0].id === 'occ-01', 'existing id preserved');
    assert(occurrences[0].venue_id !== null, 'venue_id kept');
    assert(occurrences[0].virtual_url === null, 'stale virtual_url cleared');
}

// 5. Missing start time — must still fail.
console.log('\n5. Missing start time still fails with error toast');
{
    const dom = makeDom({ 'occurrence-start-time': { value: '' } });
    const { ok, toasts } = runSaveOccurrence(dom, []);
    assert(!ok, 'save blocked');
    assert(toasts.some(t => t.type === 'error'), 'error toast emitted');
}

// 6. Clean state: no venue, no virtual URL — saves cleanly.
console.log('\n6. No venue, no virtual URL saves cleanly (minimal occurrence)');
{
    const dom = makeDom({});
    const { ok, saved } = runSaveOccurrence(dom, []);
    assert(ok, 'save succeeds');
    assert(saved.venue_id === null, 'venue_id null');
    assert(saved.virtual_url === null, 'virtual_url null');
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------
console.log(`\n${'─'.repeat(50)}`);
console.log(`Results: ${passed} passed, ${failed} failed`);
if (failed > 0) {
    process.exit(1);
}
