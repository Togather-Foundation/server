// Field Picker — shared chip-based field comparison table
// Used by consolidate.js and review-queue fold-down (srv-3p8kv).
//
// Constraints (web/AGENTS.md):
//   - No inline scripts or onclick handlers — all interactivity via data-action + event delegation
//   - Always escapeHtml() before any innerHTML interpolation
//   - Exposes window.FieldPicker for callers

(function () {
    'use strict';

    // -------------------------------------------------------------------------
    // Field definitions
    // -------------------------------------------------------------------------

    /**
     * Top-level fields rendered in the picker table.
     * Each entry maps to a form input via fieldInputId().
     */
    const TOP_LEVEL_FIELDS = [
        { label: 'Event Name',   key: 'name' },
        { label: 'Start Date',   key: 'startDate' },
        { label: 'End Date',     key: 'endDate' },
        { label: 'Description',  key: 'description' },
        { label: 'URL',          key: 'url' },
        { label: 'Image URL',    key: 'image' },
    ];

    /**
     * Nested fields for location and organizer objects.
     * parent = top-level key on the event object.
     * subfield = key within that nested object.
     */
    const NESTED_FIELDS = [
        { label: 'Venue name',      parent: 'location',  subfield: 'name' },
        { label: 'Street address',  parent: 'location',  subfield: 'streetAddress' },
        { label: 'City',            parent: 'location',  subfield: 'addressLocality' },
        { label: 'Province/State',  parent: 'location',  subfield: 'addressRegion' },
        { label: 'Postal code',     parent: 'location',  subfield: 'postalCode' },
        { label: 'Organizer name',  parent: 'organizer', subfield: 'name' },
        { label: 'Organizer URL',   parent: 'organizer', subfield: 'url' },
    ];

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    /**
     * Get a displayable (possibly truncated) value string for a field chip.
     * @param {string} raw
     * @returns {string}
     */
    function truncateDisplay(raw) {
        if (!raw) return '';
        const s = String(raw);
        return s.length > 80 ? s.substring(0, 77) + '…' : s;
    }

    /**
     * Build a single <td> chip cell for a top-level field.
     * @param {Object} event
     * @param {string} fieldKey
     * @returns {string} HTML string
     */
    function buildTopLevelCell(event, fieldKey) {
        const raw = event[fieldKey];
        if (raw == null || raw === '') {
            return '<td><span class="text-muted small">—</span></td>';
        }
        const displayVal = escapeHtml(truncateDisplay(raw));
        const dataVal = escapeHtml(String(raw));
        return (
            '<td>' +
            '<button class="btn btn-sm btn-outline-secondary text-start w-100"' +
            ' data-action="pick-field"' +
            ' data-field="' + escapeHtml(fieldKey) + '"' +
            ' data-value="' + dataVal + '"' +
            ' title="' + dataVal + '">' +
            displayVal +
            '</button>' +
            '</td>'
        );
    }

    /**
     * Build a single <td> chip cell for a nested field.
     * @param {Object} event
     * @param {string} parent  e.g. 'location'
     * @param {string} subfield e.g. 'name'
     * @returns {string} HTML string
     */
    function buildNestedCell(event, parent, subfield) {
        const parentObj = event[parent];
        // Public API (JSON-LD) nests location address fields under location.address.*;
        // fall back to parentObj[subfield] for any flat response shape.
        const lookupObj = (parent === 'location' && subfield !== 'name' && parentObj && parentObj.address)
            ? parentObj.address
            : parentObj;
        const raw = lookupObj && lookupObj[subfield] != null ? lookupObj[subfield] : null;
        if (raw == null || raw === '') {
            return '<td><span class="text-muted small">—</span></td>';
        }
        const displayVal = escapeHtml(truncateDisplay(raw));
        const dataVal = escapeHtml(String(raw));
        return (
            '<td>' +
            '<button class="btn btn-sm btn-outline-secondary text-start w-100"' +
            ' data-action="pick-field"' +
            ' data-field="' + escapeHtml(parent) + '"' +
            ' data-subfield="' + escapeHtml(subfield) + '"' +
            ' data-value="' + dataVal + '"' +
            ' title="' + dataVal + '">' +
            displayVal +
            '</button>' +
            '</td>'
        );
    }

    /**
     * Render a chip-based comparison table into containerEl.
     * @param {HTMLElement} containerEl - DOM element to render into
     * @param {Array<Object>} events - Array of event objects to compare
     */
    function renderFieldPickerTable(containerEl, events) {
        if (!containerEl) return;

        // Header row — one column per event
        let headerCells = '<th style="width:140px">Field</th>';
        events.forEach((ev, i) => {
            const name = escapeHtml(ev.name || ('Event ' + (i + 1)));
            headerCells += '<th>Event ' + (i + 1) + ' — <small class="text-muted">' + name + '</small></th>';
        });

        // Body rows — top-level fields
        let bodyRows = '';
        TOP_LEVEL_FIELDS.forEach(f => {
            let tds = '<td><small class="text-muted">' + escapeHtml(f.label) + '</small></td>';
            events.forEach(ev => {
                tds += buildTopLevelCell(ev, f.key);
            });
            bodyRows += '<tr>' + tds + '</tr>';
        });

        // Body rows — nested fields
        NESTED_FIELDS.forEach(f => {
            let tds = '<td><small class="text-muted">' + escapeHtml(f.label) + '</small></td>';
            events.forEach(ev => {
                tds += buildNestedCell(ev, f.parent, f.subfield);
            });
            bodyRows += '<tr>' + tds + '</tr>';
        });

        containerEl.innerHTML =
            '<div class="table-responsive">' +
            '<table class="table table-sm table-hover align-middle">' +
            '<thead><tr>' + headerCells + '</tr></thead>' +
            '<tbody>' + bodyRows + '</tbody>' +
            '</table>' +
            '</div>';
    }

    // -------------------------------------------------------------------------
    // Public API
    // -------------------------------------------------------------------------

    window.FieldPicker = {
        TOP_LEVEL_FIELDS,
        NESTED_FIELDS,
        truncateDisplay,
        buildTopLevelCell,
        buildNestedCell,
        renderFieldPickerTable,
    };

})();
