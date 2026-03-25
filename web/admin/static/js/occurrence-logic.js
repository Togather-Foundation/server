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
     * getTimezoneOffset — returns the RFC3339 timezone offset string for a specific date.
     * Uses Intl.DateTimeFormat to properly handle Daylight Saving Time.
     *
     * @param {string} timezone - IANA timezone string
     * @param {number} year - Year for which to get the offset
     * @param {number} month - Month (1-12) for which to get the offset
     * @param {number} day - Day for which to get the offset
     * @returns {string} Offset string like "+00:00" or "-04:00"
     */
    function getTimezoneOffset(timezone, year, month, day) {
        try {
            const formatter = new Intl.DateTimeFormat('en-US', {
                timeZone: timezone,
                timeZoneName: 'shortOffset',
            });
            const parts = formatter.formatToParts(new Date(year, month - 1, day, 12, 0, 0));
            const offsetPart = parts.find(function(p) { return p.type === 'timeZoneName'; });
            if (offsetPart) {
                const offsetStr = offsetPart.value;
                if (offsetStr === 'GMT' || offsetStr === 'UTC' || offsetStr === 'Z') {
                    return '+00:00';
                }
                var sign = '+';
                var offsetHM = offsetStr;
                // Handle formats like "GMT-4", "GMT+5", "EST", "PST" etc.
                if (offsetStr.indexOf('GMT') === 0 || offsetStr.indexOf('UT') === 0) {
                    sign = offsetStr.charAt(3);
                    offsetHM = offsetStr.slice(4);
                } else if (offsetStr.charAt(0) === '+' || offsetStr.charAt(0) === '-') {
                    sign = offsetStr.charAt(0);
                    offsetHM = offsetStr.slice(1);
                }
                if (offsetHM.indexOf(':') === -1) {
                    var hours = parseInt(offsetHM, 10);
                    var minutes = 0;
                    if (!isNaN(hours)) {
                        var isHalfHour = Math.abs(hours) * 10 % 10 === 5;
                        minutes = isHalfHour ? 30 : 0;
                        hours = Math.floor(hours);
                    }
                    offsetHM = String(Math.abs(hours)).padStart(2, '0') + ':' + String(minutes).padStart(2, '0');
                }
                return (sign === '-' ? '-' : '+') + offsetHM;
            }
        } catch (e) {}
        return '+00:00';
    }

    /**
     * convertToRFC3339 — converts HTML datetime-local format (YYYY-MM-DDTHH:mm)
     * to RFC3339 format (YYYY-MM-DDTHH:mm:ss+HH:MM) using the provided timezone.
     *
     * The datetime-local input is treated as local time in the specified timezone,
     * then converted to UTC for the RFC3339 output while preserving the timezone offset.
     *
     * @param {string} datetimeLocal - Input in format "YYYY-MM-DDTHH:mm"
     * @param {string} timezone - IANA timezone string (e.g., "America/Toronto")
     * @returns {string} RFC3339 formatted string
     */
    function convertToRFC3339(datetimeLocal, timezone) {
        if (!datetimeLocal) return null;
        var match = datetimeLocal.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/);
        if (!match) return null;
        var year = parseInt(match[1], 10);
        var month = parseInt(match[2], 10);
        var day = parseInt(match[3], 10);
        var hour = parseInt(match[4], 10);
        var minute = parseInt(match[5], 10);

        var offset = getTimezoneOffset(timezone, year, month, day);
        var offsetSign = offset.charAt(0);
        var offsetHM = offset.slice(1);
        var offsetParts = offsetHM.split(':');
        var offsetHours = parseInt(offsetParts[0], 10);
        var offsetMinutes = parseInt(offsetParts[1], 10);
        var totalOffsetMinutes = offsetHours * 60 + offsetMinutes;
        if (offsetSign === '-') {
            totalOffsetMinutes = -totalOffsetMinutes;
        }

        var localDate = new Date(year, month - 1, day, hour, minute);
        var utcTime = localDate.getTime() - totalOffsetMinutes * 60 * 1000;
        var utcDate = new Date(utcTime);

        var utcYear = utcDate.getUTCFullYear();
        var utcMonth = String(utcDate.getUTCMonth() + 1).padStart(2, '0');
        var utcDay = String(utcDate.getUTCDate()).padStart(2, '0');
        var utcHour = String(utcDate.getUTCHours()).padStart(2, '0');
        var utcMinute = String(utcDate.getUTCMinutes()).padStart(2, '0');
        var utcSecond = String(utcDate.getUTCSeconds()).padStart(2, '0');

        return utcYear + '-' + utcMonth + '-' + utcDay + 'T' + utcHour + ':' + utcMinute + ':' + utcSecond + offset;
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
