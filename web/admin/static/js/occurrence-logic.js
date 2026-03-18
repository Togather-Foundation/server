/**
 * occurrence-logic.js — pure, DOM-free occurrence helper functions.
 *
 * These functions contain the testable business logic extracted from event-edit.js.
 * They accept plain values rather than reading from the DOM, making them importable
 * by Node.js test scripts without a browser environment.
 *
 * event-edit.js calls these helpers; tests require() this module directly.
 */
'use strict';

(function (root, factory) {
    if (typeof module !== 'undefined' && module.exports) {
        // Node.js / CommonJS — used by test scripts
        module.exports = factory();
    } else {
        // Browser — attach to window so event-edit.js can call it
        root.OccurrenceLogic = factory();
    }
}(typeof globalThis !== 'undefined' ? globalThis : this, function () {

    /**
     * buildOccurrenceFields — pure hybrid-cleanup computation.
     *
     * Given the raw form values from the occurrence modal, returns the
     * canonical {virtual_url, venue_id} pair with the hybrid guard applied:
     * when a venue override is present the virtual URL is silently discarded
     * (it was hidden from the admin and is stale legacy data).
     *
     * @param {object} inputs
     * @param {string|null} inputs.venueId      — raw venue-id input value ('' → null)
     * @param {string|null} inputs.virtualUrlRaw — raw virtual-url input value ('' → null)
     * @returns {{ venue_id: string|null, virtual_url: string|null }}
     */
    function buildOccurrenceFields(inputs) {
        // Trim and normalise — whitespace-only strings are treated as absent.
        const venueId = (inputs.venueId || '').trim() || null;
        const virtualUrlRaw = (inputs.virtualUrlRaw || '').trim() || null;
        // KEY GUARD: when a venue override is active the virtual-URL section is hidden;
        // any value remaining in that input is stale legacy data — drop it silently so
        // admins can save hybrid occurrences into a valid physical-only state.
        const virtualUrl = venueId ? null : virtualUrlRaw;
        return { venue_id: venueId, virtual_url: virtualUrl };
    }

    /**
     * buildOccurrenceFromForm — builds a complete occurrence object from raw form values.
     *
     * Pure function; no DOM access.  Returns null and a reason string if validation fails.
     *
     * @param {object} form
     * @param {string}      form.indexValue    — '' for new, numeric string for edit
     * @param {string}      form.startTime
     * @param {string}      form.endTime
     * @param {string}      form.timezone
     * @param {string}      form.doorTime
     * @param {string}      form.venueId
     * @param {string}      form.virtualUrlRaw
     * @param {object[]}    existingOccurrences — current occurrences array (for id preservation)
     * @returns {{ ok: true, occurrence: object } | { ok: false, reason: string }}
     */
    function buildOccurrenceFromForm(form, existingOccurrences) {
        if (!form.startTime) {
            return { ok: false, reason: 'Start time is required' };
        }

        const { venue_id, virtual_url } = buildOccurrenceFields({
            venueId: form.venueId,
            virtualUrlRaw: form.virtualUrlRaw,
        });

        const occurrence = {
            start_time: form.startTime,
            end_time: form.endTime || null,
            timezone: form.timezone || 'America/Toronto',
            door_time: form.doorTime || null,
            virtual_url,
            venue_id,
        };

        if (form.indexValue !== '') {
            const index = parseInt(form.indexValue, 10);
            occurrence.id = (existingOccurrences[index] && existingOccurrences[index].id) || null;
        }

        return { ok: true, occurrence };
    }

    return { buildOccurrenceFields, buildOccurrenceFromForm };
}));
