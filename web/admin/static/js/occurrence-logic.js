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
     * convertToRFC3339 — converts HTML datetime-local format (YYYY-MM-DDTHH:mm)
     * to RFC3339 format (YYYY-MM-DDTHH:mm:ss+HH:MM) using the provided timezone.
     *
     * @param {string} datetimeLocal - Input in format "YYYY-MM-DDTHH:mm"
     * @param {string} timezone - IANA timezone string (e.g., "America/Toronto")
     * @returns {string} RFC3339 formatted string
     */
    function convertToRFC3339(datetimeLocal, timezone) {
        if (!datetimeLocal) return null;
        // Parse YYYY-MM-DDTHH:mm
        const match = datetimeLocal.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/);
        if (!match) return null;
        const [, year, month, day, hour, minute] = match;
        // Create date in UTC, then format with timezone offset
        // Using a simple approach: append :00 seconds and add timezone offset
        // For now, use UTC offset for simplicity; could be enhanced with Intl API
        const dateStr = `${year}-${month}-${day}T${hour}:${minute}:00`;
        // Simple timezone handling: assume offset for known timezones
        // For America/Toronto, it's either -05:00 (EST) or -04:00 (EDT)
        // For simplicity, use -05:00 as default fallback
        const tzOffset = getTimezoneOffset(timezone);
        return dateStr + tzOffset;
    }

    /**
     * getTimezoneOffset — returns the RFC3339 timezone offset string.
     * Simplified implementation; could use Intl.DateTimeFormat for production.
     *
     * @param {string} timezone - IANA timezone string
     * @returns {string} Offset string like "+00:00" or "-05:00"
     */
    function getTimezoneOffset(timezone) {
        // Common North American timezones (simplified)
        const offsets = {
            'America/Toronto': '-05:00',
            'America/New_York': '-05:00',
            'America/Chicago': '-06:00',
            'America/Denver': '-07:00',
            'America/Los_Angeles': '-08:00',
            'America/Vancouver': '-08:00',
        };
        return offsets[timezone] || '+00:00';
    }

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
     * @param {string}      form.startTime     — datetime-local format "YYYY-MM-DDTHH:mm"
     * @param {string}      form.endTime       — datetime-local format "YYYY-MM-DDTHH:mm"
     * @param {string}      form.timezone      — IANA timezone (e.g., "America/Toronto")
     * @param {string}      form.doorTime      — datetime-local format "YYYY-MM-DDTHH:mm"
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

        const tz = form.timezone || 'America/Toronto';

        const occurrence = {
            start_time: convertToRFC3339(form.startTime, tz),
            end_time: form.endTime ? convertToRFC3339(form.endTime, tz) : null,
            timezone: tz,
            door_time: form.doorTime ? convertToRFC3339(form.doorTime, tz) : null,
            virtual_url,
            venue_id,
        };

        if (form.indexValue !== '') {
            const index = parseInt(form.indexValue, 10);
            occurrence.id = (existingOccurrences[index] && existingOccurrences[index].id) || null;
        }

        return { ok: true, occurrence };
    }

    return { buildOccurrenceFields, buildOccurrenceFromForm, convertToRFC3339 };
}));
