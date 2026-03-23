/**
 * Shared occurrence rendering utilities.
 * Exposes window.OccurrenceRendering = { renderList, refreshList, renderMergedPickerList }.
 *
 * Dependencies (must load before this file):
 *   - components.js  (escapeHtml, formatDate)
 *
 * Design (docs/design/review-queue-ui.md):
 *   - renderList: single-event occurrence list with optional edit controls
 *   - renderMergedPickerList: dual-column occurrence picker for Case 2/3 (two-event duplicates)
 *     showing canonical occurrences (locked, primary) and related occurrences (toggleable or greyed on overlap)
 */
(function() {
    'use strict';

    /**
     * Render a compact list of occurrences.
     * When editable=true, each row includes a Remove button and an Add Occurrence form follows.
     * @param {Array} occurrences - Array of occurrenceDetail objects from the API
     * @param {string} eventUlid - ULID of the event that owns these occurrences
     * @param {string|number} entryId - Review queue entry ID (used for input IDs and data attributes)
     * @param {boolean} editable - Whether to show Remove and Add controls
     * @returns {string} HTML string
     */
    function renderList(occurrences, eventUlid, entryId, editable) {
        const count = occurrences ? occurrences.length : 0;
        if (count === 0 && !editable) {
            return '<p class="text-muted small mb-2">No occurrences</p>';
        }

        // Escape entryId once — it appears in id= and data-* attributes throughout this function.
        // entryId is sourced from the DOM (dataset) or API (integer), but escape defensively
        // since it flows into innerHTML attribute positions.
        const safeEntryId = escapeHtml(String(entryId));

        const rowsHtml = (occurrences || []).map(occ => {
            const start = occ.startTime ? formatDate(occ.startTime, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }) : '(no date)';
            const end = occ.endTime && occ.endTime.trim() ? formatDate(occ.endTime, { hour: 'numeric', minute: '2-digit' }) : '';
            const timeStr = end ? `${start} \u2013 ${end}` : start;

            const removeBtn = editable && !String(occ.id).startsWith('_pending_')
                ? `<button class="btn btn-sm btn-ghost-danger ms-auto" data-action="remove-occurrence" data-entry-id="${safeEntryId}" data-event-ulid="${escapeHtml(eventUlid)}" data-occurrence-id="${escapeHtml(occ.id)}" title="Remove occurrence">&#10005; Remove</button>`
                : '';

            return `<div class="d-flex align-items-center py-1 border-bottom">
                <span class="text-body-secondary small">${escapeHtml(timeStr)}</span>
                ${removeBtn}
            </div>`;
        }).join('');

        const addFormHtml = editable ? (() => {
            const defaultTz = (occurrences && occurrences.length > 0 && occurrences[0].timezone)
                ? occurrences[0].timezone
                : 'America/Toronto';
            return `
                <div class="d-flex gap-2 align-items-center flex-wrap mt-2" id="add-occ-form-${safeEntryId}">
                    <input type="datetime-local" class="form-control form-control-sm" id="occ-start-${safeEntryId}" style="max-width: 200px;" placeholder="Start (event local time)">
                    <input type="datetime-local" class="form-control form-control-sm" id="occ-end-${safeEntryId}" style="max-width: 200px;" placeholder="End (optional)">
                    <input type="text" class="form-control form-control-sm" id="occ-tz-${safeEntryId}" value="${escapeHtml(defaultTz)}" style="max-width: 160px;" placeholder="Timezone">
                    <button class="btn btn-sm btn-primary" data-action="add-occurrence" data-entry-id="${safeEntryId}" data-event-ulid="${escapeHtml(eventUlid)}">+ Add</button>
                </div>
                <div id="occ-error-${safeEntryId}" class="text-danger small mt-1" style="display:none;"></div>
            `;
        })() : '';

        return `
            <div class="mb-2">
                <small class="fw-semibold text-muted">OCCURRENCES (${count})</small>
                <div class="border rounded p-2 mt-1">
                    ${rowsHtml || '<span class="text-muted small">No occurrences yet</span>'}
                </div>
                ${addFormHtml}
            </div>
        `;
    }

    /**
     * Re-render the occurrence list in-place after an add or remove operation.
     * @param {string|number} entryId - Review queue entry ID
     * @param {string} eventUlid - ULID of the event
     * @param {Array} occurrences - Updated array of occurrenceDetail objects
     * @param {boolean} editable - Whether to show Remove and Add controls
     */
    function refreshList(entryId, eventUlid, occurrences, editable) {
        const container = document.getElementById(`occurrence-list-${entryId}`);
        if (container) {
            container.innerHTML = renderList(occurrences, eventUlid, entryId, editable);
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

        // Intervals [start1, end1] and [start2, end2] overlap if:
        // start1 < end2 AND start2 < end1
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
            return `${start} – ${end}`;
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
            return `
                <div class="mb-2">
                    <small class="fw-semibold text-muted">OCCURRENCES</small>
                    <div class="border rounded p-2 mt-1">
                        <span class="text-muted small">No occurrences to display</span>
                    </div>
                </div>
            `;
        }

        const safeEntryId = escapeHtml(String(entryId));

        // Build a quick lookup so we can check peer states at render time
        const entryByKey = {};
        pickerEntries.forEach(e => { entryByKey[e.key] = e; });

        const rowsHtml = pickerEntries.map(entry => {
            const occ = entry.occurrence;
            const timeStr = formatOccurrenceTime(occ);
            const occKey = escapeHtml(String(entry.key));

            // Determine chip state
            let chipClass;
            if (entry.included && !entry.userToggled) {
                chipClass = 'btn-primary';          // Auto-included (blue)
            } else if (entry.included && entry.userToggled) {
                chipClass = 'btn-success';          // User-locked-in (green)
            } else {
                chipClass = 'btn-outline-secondary'; // Excluded (outline)
            }

            // Check whether any overlapping peer is currently included (conflict warning)
            const hasActiveConflict = entry.overlapsWith.some(peerKey => {
                const peer = entryByKey[peerKey];
                return peer && peer.included;
            });

            // Warning icon: sits in a fixed-width slot on the inner edge of each column so the
            // chip text stays visually centred regardless of whether a warning is shown.
            // Active conflict = yellow; potential-only = muted. Slot is always reserved (visibility:hidden
            // when no overlap) so the chip width is stable.
            const hasAnyOverlap = entry.overlapsWith.length > 0;
            const warnVisible = hasAnyOverlap ? '' : 'visibility:hidden;';
            const warnColor = hasActiveConflict ? 'text-warning' : 'text-muted';
            const warnTitle = hasActiveConflict
                ? 'Conflict: overlapping occurrence is included'
                : 'This occurrence overlaps with another';
            const warnSlot = `<span class="${warnColor}" style="font-size:1.25rem;line-height:1;flex:0 0 1.5rem;text-align:center;${warnVisible}" title="${escapeHtml(warnTitle)}">&#9888;&#xFE0F;</span>`;

            const chipTitle = entry.included
                ? 'Included — click to exclude'
                : (hasActiveConflict ? 'Cannot include — overlapping occurrence is already included' : 'Excluded — click to include');

            const chip = `<button class="btn btn-sm ${chipClass} w-100 text-center"
                data-action="toggle-occurrence"
                data-entry-id="${safeEntryId}"
                data-occ-key="${occKey}"
                title="${escapeHtml(chipTitle)}">${escapeHtml(timeStr)}</button>`;

            // Left column:  [chip — fills remaining width] [warn slot on inner/right edge]
            // Right column: [warn slot on inner/left edge] [chip — fills remaining width]
            const isLeft = entry.eventIndex === 0;
            const leftCell = isLeft
                ? `<div class="d-flex align-items-center gap-1">${chip}${warnSlot}</div>`
                : '<span class="text-muted small">—</span>';
            const rightCell = !isLeft
                ? `<div class="d-flex align-items-center gap-1">${warnSlot}${chip}</div>`
                : '<span class="text-muted small">—</span>';

            return `
                <div class="row g-2 align-items-center py-2 border-bottom">
                    <div class="col-6">${leftCell}</div>
                    <div class="col-6">${rightCell}</div>
                </div>
            `;
        }).join('');

        return `
            <div class="mb-2">
                <div class="border rounded p-2 mt-1">
                    <div class="row g-2 fw-semibold text-muted small mb-2 border-bottom pb-1">
                        <div class="col-6">THIS EVENT</div>
                        <div class="col-6">RELATED EVENT</div>
                    </div>
                    ${rowsHtml}
                </div>
                <small class="text-muted mt-1 d-block">Blue = auto-included &nbsp; Green = selected &nbsp; Outline = excluded &nbsp; <span style="font-size:1rem;">&#9888;&#xFE0F;</span> = overlap conflict</small>
            </div>
        `;
    }

    window.OccurrenceRendering = { renderList, refreshList, renderMergedPickerList, occurrencesOverlap };
})();
