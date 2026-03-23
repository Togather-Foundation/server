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
     * Shows canonical (this event) occurrences in one column and related event occurrences in another.
     * Canonical occurrences are locked (read-only). Related occurrences are toggleable unless they overlap.
     *
     * Design (docs/design/review-queue-ui.md):
     *   - One row per occurrence per event (no row-merging)
     *   - Sorted by start time across both events
     *   - Canonical chip: btn-primary, lock icon, no data-action
     *   - Related chip (no overlap): btn-primary if included, btn-outline-secondary if excluded; data-action="toggle-occurrence"
     *   - Related chip (overlap): greyed btn-secondary disabled, no data-action
     *
     * @param {Array} pickerEntries - Array of picker entry objects
     *   Each entry: { source: 'this'|'related', occurrenceId, occurrence: {...}, included: boolean, overlaps: boolean }
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

        // Group entries by date for row labels
        const rowsByDate = {};
        pickerEntries.forEach(entry => {
            const occ = entry.occurrence;
            const dateLabel = occ.startTime
                ? formatDate(occ.startTime, { month: 'short', day: 'numeric' })
                : '(no date)';

            if (!rowsByDate[dateLabel]) {
                rowsByDate[dateLabel] = [];
            }
            rowsByDate[dateLabel].push(entry);
        });

        // Build rows
        const rowsHtml = Object.entries(rowsByDate).map(([dateLabel, entries]) => {
            return entries.map(entry => {
                const occ = entry.occurrence;
                const timeStr = formatOccurrenceTime(occ);

                // Determine chip class and action
                let chipHtml;
                if (entry.source === 'this') {
                    // Canonical chip: locked, primary (blue), no action
                    chipHtml = `
                        <button class="btn btn-sm btn-primary" disabled style="cursor: not-allowed;">
                            <span style="margin-right: 0.25rem;">🔒</span>
                            ${escapeHtml(timeStr)}
                        </button>
                    `;
                } else if (entry.overlaps) {
                    // Related chip with overlap: greyed, disabled
                    chipHtml = `
                        <button class="btn btn-sm btn-secondary" disabled style="opacity: 0.6; cursor: not-allowed;">
                            <span style="margin-right: 0.25rem;">⚠</span>
                            ${escapeHtml(timeStr)}
                        </button>
                    `;
                } else {
                    // Related chip without overlap: toggleable
                    const chipClass = entry.included ? 'btn-primary' : 'btn-outline-secondary';
                    const occKey = escapeHtml(entry.occurrenceId);
                    chipHtml = `
                        <button class="btn btn-sm ${chipClass}" data-action="toggle-occurrence"
                            data-entry-id="${safeEntryId}" data-occ-key="${occKey}">
                            ${escapeHtml(timeStr)}
                        </button>
                    `;
                }

                // Determine column: left for 'this', right for 'related'
                const isThisEvent = entry.source === 'this';
                const leftChip = isThisEvent ? chipHtml : '<span class="text-muted">—</span>';
                const rightChip = !isThisEvent ? chipHtml : '<span class="text-muted">—</span>';

                return `
                    <div class="row g-2 align-items-center py-2 border-bottom">
                        <div class="col-md-6" style="text-align: center;">
                            ${leftChip}
                        </div>
                        <div class="col-md-6" style="text-align: center;">
                            ${rightChip}
                        </div>
                    </div>
                `;
            }).join('');
        }).join('');

        return `
            <div class="mb-2">
                <small class="fw-semibold text-muted">OCCURRENCES</small>
                <div class="border rounded p-2 mt-1">
                    <div class="row g-2 fw-semibold text-muted small mb-2">
                        <div class="col-md-6" style="text-align: center;">THIS EVENT</div>
                        <div class="col-md-6" style="text-align: center;">RELATED EVENT</div>
                    </div>
                    ${rowsHtml}
                </div>
            </div>
        `;
    }

    window.OccurrenceRendering = { renderList, refreshList, renderMergedPickerList, occurrencesOverlap };
})();
