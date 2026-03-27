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
        // Format: preserve wall-clock time in the timezone with offset appended
        // This is the RFC3339 "with-timezone" form, not UTC
        var monthStr = String(month).padStart(2, '0');
        var dayStr = String(day).padStart(2, '0');
        var hourStr = String(hour).padStart(2, '0');
        var minuteStr = String(minute).padStart(2, '0');
        return year + '-' + monthStr + '-' + dayStr + 'T' + hourStr + ':' + minuteStr + ':00' + offset;
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

    /**
     * formatForDatetimeLocal — converts RFC3339 datetime string to HTML datetime-local format (YYYY-MM-DDTHH:mm).
     * @param {string} dateString - RFC3339 formatted datetime string
     * @returns {string} Formatted string in "YYYY-MM-DDTHH:mm" format, or empty string if invalid
     */
    function formatForDatetimeLocal(dateString) {
        if (!dateString) return '';
        var date = new Date(dateString);
        var year = date.getFullYear();
        var month = String(date.getMonth() + 1).padStart(2, '0');
        var day = String(date.getDate()).padStart(2, '0');
        var hours = String(date.getHours()).padStart(2, '0');
        var minutes = String(date.getMinutes()).padStart(2, '0');
        return year + '-' + month + '-' + day + 'T' + hours + ':' + minutes;
    }

    /**
     * formatTimeRange — formats a start+end RFC3339 pair into a compact, human-readable string.
     * @param {string|null} start - RFC3339 start datetime
     * @param {string|null} end - RFC3339 end datetime
     * @param {Object} options - Formatting options
     * @param {string} [options.locale] - BCP 47 locale; default: window.ADMIN_LOCALE ?? 'en-CA'
     * @param {boolean} [options.showYear] - Force year display; default: auto
     * @param {string} [options.timezone] - IANA timezone (reserved, unused)
     * @returns {string} Formatted time range string
     */
    function formatTimeRange(start, end, options) {
        options = options || {};
        var locale = options.locale || window.ADMIN_LOCALE || 'en-CA';
        var showYear = options.showYear;

        if (!start) return '(no date)';

        var startDate = new Date(start);
        if (isNaN(startDate.getTime())) return '(no date)';

        var currentYear = new Date().getFullYear();
        if (showYear === undefined) {
            showYear = startDate.getFullYear() !== currentYear;
        }

        var dateOpts = { month: 'short', day: 'numeric' };
        var timeOpts = { hour: 'numeric', minute: '2-digit', hour12: true };

        var startDateStr = startDate.toLocaleDateString(locale, dateOpts);
        if (showYear) {
            startDateStr = startDate.toLocaleDateString(locale, { year: 'numeric', month: 'short', day: 'numeric' });
        }

        var startTimeStr = startDate.toLocaleTimeString(locale, timeOpts);

        if (!end) {
            return startDateStr + ', ' + startTimeStr;
        }

        var endDate = new Date(end);
        if (isNaN(endDate.getTime())) {
            return startDateStr + ', ' + startTimeStr;
        }

        var sameDay = startDate.getFullYear() === endDate.getFullYear() &&
            startDate.getMonth() === endDate.getMonth() &&
            startDate.getDate() === endDate.getDate();

        var endDateStr = endDate.toLocaleDateString(locale, dateOpts);
        var endTimeStr = endDate.toLocaleTimeString(locale, timeOpts);

        if (sameDay) {
            var startPeriod = startTimeStr.slice(-2);
            var endPeriod = endTimeStr.slice(-2);
            var startTimeNum = startTimeStr.slice(0, -3);
            var endTimeNum = endTimeStr.slice(0, -3);
            if (startPeriod === endPeriod) {
                return startDateStr + ', ' + startTimeNum + ' \u2013 ' + endTimeStr;
            } else {
                return startDateStr + ', ' + startTimeStr + ' \u2013 ' + endTimeStr;
            }
        } else {
            var fullStartStr = startDate.toLocaleDateString(locale, { year: 'numeric', month: 'short', day: 'numeric' });
            var fullEndStr = endDate.toLocaleDateString(locale, { year: 'numeric', month: 'short', day: 'numeric' });
            return fullStartStr + ', ' + startTimeStr + ' \u2013 ' + fullEndStr + ', ' + endTimeStr;
        }
    }

    /**
     * defaultEndTime — returns a suggested RFC3339 end time string based on start + hints.
     * @param {string|null} startRFC3339 - RFC3339 start datetime
     * @param {Object} hints - Hints for computing duration
     * @param {number} [hints.durationMs] - Exact duration in milliseconds
     * @param {number} [hints.durationHours] - Duration in hours (shorthand)
     * @param {Object} [hints.copyDuration] - Copy duration from previous occurrence
     * @param {string} [hints.copyDuration.prevStart] - Previous start RFC3339
     * @param {string} [hints.copyDuration.prevEnd] - Previous end RFC3339
     * @returns {string|null} Suggested end time as RFC3339, or null if start is invalid
     */
    function defaultEndTime(startRFC3339, hints) {
        if (!startRFC3339) return null;
        var startDate = new Date(startRFC3339);
        if (isNaN(startDate.getTime())) return null;

        var durationMs = 2 * 3600000;
        if (hints) {
            if (hints.durationMs != null) {
                durationMs = hints.durationMs;
            } else if (hints.durationHours != null) {
                durationMs = hints.durationHours * 3600000;
            } else if (hints.copyDuration) {
                var ps = hints.copyDuration.prevStart;
                var pe = hints.copyDuration.prevEnd;
                if (ps && pe) {
                    var dur = new Date(pe).getTime() - new Date(ps).getTime();
                    if (dur > 0) durationMs = dur;
                }
            }
        }

        var endDate = new Date(startDate.getTime() + durationMs);
        var offsetMatch = startRFC3339.match(/([+-]\d{2}:\d{2})$/);
        var offset = offsetMatch ? offsetMatch[1] : null;

        if (offset) {
            var sign = offset[0] === '+' ? 1 : -1;
            var oh = parseInt(offset.slice(1, 3), 10);
            var om = parseInt(offset.slice(4, 6), 10);
            var offsetMs = sign * (oh * 60 + om) * 60000;
            var wallMs = endDate.getTime() + offsetMs;
            var w = new Date(wallMs);
            var pad = function(n) { return String(n).padStart(2, '0'); };
            return w.getUTCFullYear() + '-' + pad(w.getUTCMonth() + 1) + '-' + pad(w.getUTCDate()) +
                'T' + pad(w.getUTCHours()) + ':' + pad(w.getUTCMinutes()) + ':00' + offset;
        }
        return endDate.toISOString().slice(0, 19) + 'Z';
    }

    /**
     * wireStartBlur — attaches a change event listener to auto-fill end time when start changes.
     * @param {string} entryId - Entry ID for input element IDs
     * @param {Function} hintsProvider - Function returning hints for defaultEndTime
     * @returns {{ destroy: Function }} Object with destroy method to remove listener
     */
    function wireStartBlur(entryId, hintsProvider) {
        var startInput = document.getElementById('occ-start-' + entryId);
        if (!startInput) return { destroy: function() {} };

        function handler() {
            var endInput = document.getElementById('occ-end-' + entryId);
            if (!endInput || endInput.value) return;
            var tzInput = document.getElementById('occ-tz-' + entryId);
            var tz = (tzInput && tzInput.value) || 'America/Toronto';
            var startRFC = convertToRFC3339(startInput.value, tz);
            if (!startRFC) return;
            var hints = hintsProvider ? hintsProvider() : {};
            var endRFC = defaultEndTime(startRFC, hints);
            if (endRFC) endInput.value = formatForDatetimeLocal(endRFC);
        }

        startInput.addEventListener('change', handler);
        return {
            destroy: function() { startInput.removeEventListener('change', handler); }
        };
    }

    return { buildOccurrenceFields, buildOccurrenceFromForm, convertToRFC3339, formatForDatetimeLocal, formatTimeRange, defaultEndTime, wireStartBlur };
}));
