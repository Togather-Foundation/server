/**
 * Shared occurrence rendering utilities.
 * Exposes window.OccurrenceRendering = { renderList, refreshList, renderEditRow, renderMergedPickerList, occurrencesOverlap }.
 *
 * Dependencies (must load before this file):
 *   - components.js  (escapeHtml, formatDate)
 *   - occurrence-logic.js (OccurrenceLogic.formatForDatetimeLocal)
 *
 * Design (docs/design/review-queue-ui.md):
 *   - renderList: single-event occurrence list with optional inline edit controls
 *   - renderEditRow: inline edit form for a single occurrence
 *   - renderMergedPickerList: dual-column occurrence picker for Case 2/3 (two-event duplicates)
 *     showing canonical occurrences (locked, primary) and related occurrences (toggleable or greyed on overlap)
 */
(function() {
    'use strict';

    /**
     * Render a list of occurrences with optional inline editing.
     * When editable=true, each row includes an Edit button and an Add Occurrence form follows.
     * @param {Array} occurrences - Array of occurrence objects from the API
     * @param {string} eventUlid - ULID of the event that owns these occurrences
     * @param {string|number} entryId - Review queue entry ID (used for input IDs and data attributes)
     * @param {boolean} editable - Whether to show Edit and Add controls
     * @param {string} defaultTz - Default timezone string (fallback to 'America/Toronto' if falsy)
     * @returns {string} HTML string
     */
    function renderList(occurrences, eventUlid, entryId, editable, defaultTz) {
        if (!defaultTz) {
            defaultTz = 'America/Toronto';
        }

        const count = occurrences ? occurrences.length : 0;
        if (count === 0 && !editable) {
            return '<p class="text-muted small mb-2">No occurrences</p>';
        }

        const safeEntryId = escapeHtml(String(entryId));

        const rowsHtml = (occurrences || []).map(function(occ, index) {
            const start = occ.start_time || occ.startTime;
            const end = occ.end_time || occ.endTime;
            const timezone = occ.timezone;
            const doorTime = occ.door_time || occ.doorTime;
            const venueId = occ.venue_id || occ.venueId;
            const virtualUrl = occ.virtual_url || occ.virtualUrl;
            const occId = occ.id || occ['@id'] || '';

            const startDisplay = start ? formatDate(start, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }) : '(no date)';
            const endDisplay = end ? formatDate(end, { hour: 'numeric', minute: '2-digit' }) : '';
            const timeStr = endDisplay ? startDisplay + ' \u2013 ' + endDisplay : startDisplay;

            let detailsHtml = '';
            if (timezone) {
                detailsHtml += '<span class="badge bg-secondary-lt me-1">' + escapeHtml(timezone) + '</span>';
            }
            if (doorTime) {
                detailsHtml += '<span class="text-muted small me-1">Doors: ' + formatDate(doorTime, { hour: 'numeric', minute: '2-digit' }) + '</span>';
            }
            if (virtualUrl) {
                detailsHtml += '<span class="text-muted small d-block">' + escapeHtml(virtualUrl) + '</span>';
            }
            if (venueId) {
                const venueUlid = venueUlidFromId(venueId);
                detailsHtml += '<span class="badge bg-blue-lt me-1">Venue: ' + escapeHtml(venueUlid) + '</span>';
            }

            let actionBtns = '';
            if (editable) {
                actionBtns =
                    '<button class="btn btn-sm btn-outline-secondary ms-2" data-action="edit-occurrence" data-entry-id="' + safeEntryId + '" data-event-ulid="' + escapeHtml(eventUlid) + '" data-occurrence-id="' + escapeHtml(occId) + '" data-occurrence-index="' + index + '">Edit</button>' +
                    '<button class="btn btn-sm btn-ghost-danger ms-1" data-action="remove-occurrence" data-entry-id="' + safeEntryId + '" data-event-ulid="' + escapeHtml(eventUlid) + '" data-occurrence-id="' + escapeHtml(occId) + '" data-occurrence-index="' + index + '" title="Remove occurrence">&#10005;</button>';
            }

            // Row ID uses occurrence id when available, falls back to index for pending (unsaved) occurrences.
            const rowSuffix = occId ? escapeHtml(occId) : 'idx-' + index;
            return '<div class="d-flex align-items-start py-2 border-bottom" id="occ-row-' + safeEntryId + '-' + rowSuffix + '">' +
                '<div class="flex-grow-1">' +
                '<div class="text-body-secondary">' + escapeHtml(timeStr) + '</div>' +
                (detailsHtml ? '<div class="mt-1">' + detailsHtml + '</div>' : '') +
                '</div>' +
                actionBtns +
                '</div>';
        }).join('');

        const addFormHtml = editable ? renderAddForm(safeEntryId, eventUlid, defaultTz, occurrences) : '';

        return '<div class="mb-2" id="occurrence-list-' + safeEntryId + '">' +
            '<small class="fw-semibold text-muted">OCCURRENCES (' + count + ')</small>' +
            '<div class="border rounded p-2 mt-1">' +
            (rowsHtml || '<span class="text-muted small">No occurrences yet</span>') +
            '</div>' +
            addFormHtml +
            '</div>';
    }

    /**
     * Render the add occurrence form.
     * @param {string} safeEntryId - Escaped entry ID for namespacing
     * @param {string} eventUlid - Event ULID
     * @param {string} defaultTz - Default timezone
     * @param {Array} occurrences - Current occurrences (for smart defaults)
     * @returns {string} HTML string
     */
    function renderAddForm(safeEntryId, eventUlid, defaultTz, occurrences) {
        let smartStart = '';
        let smartEnd = '';
        if (occurrences && occurrences.length > 0) {
            const lastOcc = occurrences[occurrences.length - 1];
            const lastStart = lastOcc.start_time || lastOcc.startTime;
            if (lastStart) {
                smartStart = ' value="' + escapeHtml(OccurrenceLogic.formatForDatetimeLocal(lastStart)) + '"';
                const lastEnd = lastOcc.end_time || lastOcc.endTime;
                if (lastEnd) {
                    smartEnd = ' value="' + escapeHtml(OccurrenceLogic.formatForDatetimeLocal(lastEnd)) + '"';
                }
            }
        }

        return '<div class="mt-2 p-2 bg-light rounded" id="add-occ-form-' + safeEntryId + '">' +
            '<div class="row g-2">' +
            '<div class="col-md-6">' +
            '<input type="datetime-local" class="form-control form-control-sm"' + smartStart + ' id="occ-start-' + safeEntryId + '" placeholder="Start (event local time)">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<input type="datetime-local" class="form-control form-control-sm"' + smartEnd + ' id="occ-end-' + safeEntryId + '" placeholder="End (optional)">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<input type="text" class="form-control form-control-sm" id="occ-tz-' + safeEntryId + '" value="' + escapeHtml(defaultTz) + '" placeholder="Timezone">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<input type="datetime-local" class="form-control form-control-sm" id="occ-door-' + safeEntryId + '" placeholder="Door time (optional)">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<input type="url" class="form-control form-control-sm" id="occ-virtual-url-' + safeEntryId + '" placeholder="https://... (online only)">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<div class="input-group input-group-sm">' +
            '<input type="hidden" id="occ-venue-id-' + safeEntryId + '" value="">' +
            '<input type="text" class="form-control" id="occ-venue-display-' + safeEntryId + '" readonly placeholder="(none \u2014 uses event default)">' +
            '<button class="btn btn-outline-danger" type="button" data-action="clear-occurrence-venue" data-entry-id="' + safeEntryId + '" style="display:none;" title="Clear venue override">Clear</button>' +
            '</div>' +
            '</div>' +
            '<div class="col-12">' +
            '<button class="btn btn-sm btn-primary" data-action="add-occurrence" data-entry-id="' + safeEntryId + '" data-event-ulid="' + escapeHtml(eventUlid) + '">+ Add Occurrence</button>' +
            '</div>' +
            '</div>' +
            '<div id="occ-error-' + safeEntryId + '" class="text-danger small mt-1" style="display:none;"></div>' +
            '</div>';
    }

    /**
     * Render an inline edit row for a single occurrence.
     * @param {Object} occ - Occurrence object with current values
     * @param {string} eventUlid - Event ULID
     * @param {string|number} entryId - Entry ID for namespacing
     * @param {number} [occurrenceIndex] - Index of the occurrence in the list (for local-array callers)
     * @returns {string} HTML string
     */
    function renderEditRow(occ, eventUlid, entryId, occurrenceIndex) {
        const safeEntryId = escapeHtml(String(entryId));
        const occId = occ.id || occ['@id'] || '';
        const safeOccId = escapeHtml(occId);

        const startTime = occ.start_time || occ.startTime || '';
        const endTime = occ.end_time || occ.endTime || '';
        const timezone = occ.timezone || 'America/Toronto';
        const doorTime = occ.door_time || occ.doorTime || '';
        const venueId = occ.venue_id || occ.venueId || '';
        const virtualUrl = occ.virtual_url || occ.virtualUrl || '';

        const startValue = startTime ? ' value="' + escapeHtml(OccurrenceLogic.formatForDatetimeLocal(startTime)) + '"' : '';
        const endValue = endTime ? ' value="' + escapeHtml(OccurrenceLogic.formatForDatetimeLocal(endTime)) + '"' : '';
        const doorValue = doorTime ? ' value="' + escapeHtml(OccurrenceLogic.formatForDatetimeLocal(doorTime)) + '"' : '';

        const venueUlid = venueUlidFromId(venueId);
        const hasVenue = !!venueId;

        return '<div class="p-2 bg-light rounded" id="occ-edit-' + safeEntryId + '-' + safeOccId + '">' +
            '<div class="row g-2">' +
            '<div class="col-md-6">' +
            '<input type="datetime-local" class="form-control form-control-sm"' + startValue + ' id="occ-start-' + safeEntryId + '" placeholder="Start (event local time)">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<input type="datetime-local" class="form-control form-control-sm"' + endValue + ' id="occ-end-' + safeEntryId + '" placeholder="End (optional)">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<input type="text" class="form-control form-control-sm" id="occ-tz-' + safeEntryId + '" value="' + escapeHtml(timezone) + '" placeholder="Timezone">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<input type="datetime-local" class="form-control form-control-sm"' + doorValue + ' id="occ-door-' + safeEntryId + '" placeholder="Door time (optional)">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<input type="url" class="form-control form-control-sm" id="occ-virtual-url-' + safeEntryId + '" value="' + escapeHtml(virtualUrl) + '" placeholder="https://... (online only)">' +
            '</div>' +
            '<div class="col-md-6">' +
            '<div class="input-group input-group-sm">' +
            '<input type="hidden" id="occ-venue-id-' + safeEntryId + '" value="' + escapeHtml(venueId) + '">' +
            '<input type="text" class="form-control" id="occ-venue-display-' + safeEntryId + '" value="' + escapeHtml(venueUlid) + '" readonly placeholder="(none \u2014 uses event default)">' +
            '<button class="btn btn-outline-danger" type="button" data-action="clear-occurrence-venue" data-entry-id="' + safeEntryId + '"' + (hasVenue ? '' : ' style="display:none;"') + ' title="Clear venue override">Clear</button>' +
            '</div>' +
            '</div>' +
            '<div class="col-12">' +
            '<button class="btn btn-sm btn-primary" data-action="save-occurrence" data-entry-id="' + safeEntryId + '" data-event-ulid="' + escapeHtml(eventUlid) + '" data-occurrence-id="' + safeOccId + '"' + (occurrenceIndex !== undefined ? ' data-occurrence-index="' + occurrenceIndex + '"' : '') + '>Save</button>' +
            '<button class="btn btn-sm btn-secondary ms-1" data-action="cancel-edit-occurrence" data-entry-id="' + safeEntryId + '" data-occurrence-id="' + safeOccId + '">Cancel</button>' +
            '</div>' +
            '</div>' +
            '<div id="occ-error-' + safeEntryId + '" class="text-danger small mt-1" style="display:none;"></div>' +
            '</div>';
    }

    /**
     * Re-render the occurrence list in-place after an add or remove operation.
     * @param {string|number} entryId - Review queue entry ID
     * @param {string} eventUlid - ULID of the event
     * @param {Array} occurrences - Updated array of occurrence objects
     * @param {boolean} editable - Whether to show Edit and Add controls
     * @param {string} defaultTz - Default timezone string (fallback to 'America/Toronto' if falsy)
     */
    function refreshList(entryId, eventUlid, occurrences, editable, defaultTz) {
        // This function cannot be param-first because it manipulates DOM directly.
        // It wraps renderList by injecting the result into the appropriate container.
        const containerId = 'occurrence-list-' + String(entryId);
        var container = document.getElementById(containerId);
        if (container) {
            container.outerHTML = renderList(occurrences, eventUlid, entryId, editable, defaultTz);
        }
    }

    /**
     * Check if two occurrences overlap by time interval.
     * @param {Object} occ1 - First occurrence { startTime, endTime }
     * @param {Object} occ2 - Second occurrence { startTime, endTime }
     * @returns {boolean}
     */
    function occurrencesOverlap(occ1, occ2) {
        if (!occ1.startTime || !occ2.startTime) return false;

        const start1 = new Date(occ1.startTime).getTime();
        const end1 = occ1.endTime ? new Date(occ1.endTime).getTime() : start1;
        const start2 = new Date(occ2.startTime).getTime();
        const end2 = occ2.endTime ? new Date(occ2.endTime).getTime() : start2;

        return start1 < end2 && start2 < end1;
    }

    /**
     * Format a datetime for display in the occurrence picker.
     * @param {string} dateTime - ISO datetime string
     * @returns {string} Formatted date+time, e.g. "Apr 8, 7:00–9:00 PM"
     */
    function formatOccurrenceTime(occ) {
        if (!occ.startTime) return '(no date)';

        const start = formatDate(occ.startTime, {
            month: 'short',
            day: 'numeric',
            hour: 'numeric',
            minute: '2-digit'
        });

        if (occ.endTime && occ.endTime.trim()) {
            const end = formatDate(occ.endTime, {
                hour: 'numeric',
                minute: '2-digit'
            });
            return start + ' – ' + end;
        }

        return start;
    }

    /**
     * Render a two-column occurrence picker for multi-event consolidation.
     * Shows "this event" occurrences in left column and related event occurrences in right column.
     * Each occurrence is rendered as a chip that can be toggled included/excluded.
     *
     * Chip colours mirror the field picker:
     *   - Blue  (btn-primary)           — auto-included based on canonical selection, not yet user-touched
     *   - Green (btn-success)           — user explicitly locked in (included)
     *   - Outline (btn-outline-secondary) — excluded (auto or user)
     *
     * Overlap behaviour:
     *   - An occurrence with active overlapping peers shows a ⚠ warning icon on its chip.
     *   - Clicking an excluded chip to enable it is blocked if any of its `overlapsWith` peers are
     *     currently included (the warning icon signals the conflict).
     *   - Clicking an included chip always excludes it regardless of overlaps.
     *
     * @param {Array} pickerEntries - Array of picker entry objects from buildOccurrencePicker:
     *   { key, source, eventIndex, occurrence, occurrenceId, hasRealId,
     *     included, userToggled, overlapsWith: string[] }
     * @param {string|number} entryId - Review queue entry ID (for data attributes)
     * @returns {string} HTML string
     */
    function renderMergedPickerList(pickerEntries, entryId) {
        if (!pickerEntries || pickerEntries.length === 0) {
            return '<div class="mb-2">' +
                '<small class="fw-semibold text-muted">OCCURRENCES</small>' +
                '<div class="border rounded p-2 mt-1">' +
                '<span class="text-muted small">No occurrences to display</span>' +
                '</div>' +
                '</div>';
        }

        const safeEntryId = escapeHtml(String(entryId));

        // Build a quick lookup so we can check peer states at render time
        var entryByKey = {};
        pickerEntries.forEach(function(e) { entryByKey[e.key] = e; });

        var rowsHtml = pickerEntries.map(function(entry) {
            var occ = entry.occurrence;
            var timeStr = formatOccurrenceTime(occ);
            var occKey = escapeHtml(String(entry.key));

            // Determine chip state
            var chipClass;
            if (entry.included && !entry.userToggled) {
                chipClass = 'btn-primary';
            } else if (entry.included && entry.userToggled) {
                chipClass = 'btn-success';
            } else {
                chipClass = 'btn-outline-secondary';
            }

            // Check whether any overlapping peer is currently included (conflict warning)
            var hasActiveConflict = entry.overlapsWith.some(function(peerKey) {
                var peer = entryByKey[peerKey];
                return peer && peer.included;
            });

            var hasAnyOverlap = entry.overlapsWith.length > 0;
            var warnVisible = hasAnyOverlap ? '' : 'visibility:hidden;';
            var warnColor = hasActiveConflict ? 'text-warning' : 'text-muted';
            var warnTitle = hasActiveConflict
                ? 'Conflict: overlapping occurrence is included'
                : 'This occurrence overlaps with another';
            var warnSlot = '<span class="' + warnColor + '" style="font-size:1.25rem;line-height:1;flex:0 0 1.5rem;text-align:center;' + warnVisible + '" title="' + escapeHtml(warnTitle) + '">⚠️</span>';

            var chipTitle = entry.included
                ? 'Included — click to exclude'
                : (hasActiveConflict ? 'Cannot include — overlapping occurrence is already included' : 'Excluded — click to include');

            var chip = '<button class="btn btn-sm ' + chipClass + ' w-100 text-center"' +
                ' data-action="toggle-occurrence"' +
                ' data-entry-id="' + safeEntryId + '"' +
                ' data-occ-key="' + occKey + '"' +
                ' title="' + escapeHtml(chipTitle) + '">' + escapeHtml(timeStr) + '</button>';

            // Left column:  [chip — fills remaining width] [warn slot on inner/right edge]
            // Right column: [warn slot on inner/left edge] [chip — fills remaining width]
            var isLeft = entry.eventIndex === 0;
            var leftCell = isLeft
                ? '<div class="d-flex align-items-center gap-1">' + chip + warnSlot + '</div>'
                : '<span class="text-muted small">—</span>';
            var rightCell = !isLeft
                ? '<div class="d-flex align-items-center gap-1">' + warnSlot + chip + '</div>'
                : '<span class="text-muted small">—</span>';

            return '<div class="row g-2 align-items-center py-2 border-bottom">' +
                '<div class="col-6">' + leftCell + '</div>' +
                '<div class="col-6">' + rightCell + '</div>' +
                '</div>';
        }).join('');

        return '<div class="mb-2">' +
            '<div class="border rounded p-2 mt-1">' +
            '<div class="row g-2 fw-semibold text-muted small mb-2 border-bottom pb-1">' +
            '<div class="col-6">THIS EVENT</div>' +
            '<div class="col-6">RELATED EVENT</div>' +
            '</div>' +
            rowsHtml +
            '</div>' +
            '<small class="text-muted mt-1 d-block">Blue = auto-included &nbsp; Green = selected &nbsp; Outline = excluded &nbsp; ⚠️ = overlap conflict</small>' +
            '</div>';
    }

    /**
     * Extract the trailing ULID from a venue URI, or return the raw value.
     * @param {string} venueId - Venue URI string
     * @returns {string} ULID or raw value
     */
    function venueUlidFromId(venueId) {
        if (!venueId) return '';
        var m = venueId.match(/\/([A-Z0-9]{26})$/i);
        return m ? m[1] : venueId;
    }

    window.OccurrenceRendering = {
        renderList: renderList,
        refreshList: refreshList,
        renderEditRow: renderEditRow,
        renderMergedPickerList: renderMergedPickerList,
        occurrencesOverlap: occurrencesOverlap
    };
})();
