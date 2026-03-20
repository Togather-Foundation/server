/**
 * Shared occurrence rendering utilities.
 * Exposes window.OccurrenceRendering = { renderList, refreshList }.
 *
 * Dependencies (must load before this file):
 *   - components.js  (escapeHtml, formatDate)
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

    window.OccurrenceRendering = { renderList, refreshList };
})();
