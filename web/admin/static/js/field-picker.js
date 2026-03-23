// Field Picker — shared chip-based field comparison table
// Used by consolidate.js and review-queue fold-down (srv-7roii).
//
// Constraints (web/AGENTS.md):
//   - No inline scripts or onclick handlers — all interactivity via data-action + event delegation
//   - Always escapeHtml() before any innerHTML interpolation
//   - Exposes window.FieldPicker for callers
//
// Design (docs/design/review-queue-ui.md):
//   - 3-state chip behavior: outline → clicked → inline input/textarea + sibling reverts to outline
//   - Canonical chip pre-selected (btn-primary); other event's chip outlined
//   - Read-only fields (location.name, organizer.name) shown as plain text
//   - onPick callback: (fieldKey, subfieldKey, value, source) when chip is picked

(function () {
    'use strict';

    // -------------------------------------------------------------------------
    // Field definitions
    // -------------------------------------------------------------------------

    /**
     * Top-level fields rendered in the picker table.
     * Callers are responsible for wiring picked values to form inputs or override maps.
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
     * Supports 3-state behavior: outline (default), primary (canonical), or inline input (editable).
     * @param {Object} event - Event object
     * @param {string} fieldKey - Field key
     * @param {number} eventIndex - Index of this event in the events array (0 or 1)
     * @param {number} canonicalIndex - Which event index is canonical
     * @param {boolean} isReadOnly - Whether this field is read-only
     * @param {Object|null} selectedOverrides - Map of fieldKey -> eventIndex for user-selected overrides
     * @returns {string} HTML string
     */
    function buildTopLevelCell(event, fieldKey, eventIndex, canonicalIndex, isReadOnly, selectedOverrides) {
        const raw = event[fieldKey];
        if (raw == null || raw === '') {
            return '<td><span class="text-muted small">—</span></td>';
        }

        if (isReadOnly) {
            // Read-only: show as plain text
            const displayVal = escapeHtml(truncateDisplay(raw));
            return '<td><span class="text-body">' + displayVal + '</span></td>';
        }

        // Override selection takes precedence over canonical; otherwise use canonical
        let isSelected = eventIndex === canonicalIndex;
        if (selectedOverrides && selectedOverrides[fieldKey] !== undefined) {
            isSelected = selectedOverrides[fieldKey] === eventIndex;
        }

        const chipClass = isSelected ? 'btn-primary' : 'btn-outline-secondary';
        const displayVal = escapeHtml(truncateDisplay(raw));
        const dataVal = escapeHtml(String(raw));

        return (
            '<td>' +
            '<button class="btn btn-sm ' + chipClass + ' text-start w-100 field-picker-chip"' +
            ' data-action="pick-field"' +
            ' data-field="' + escapeHtml(fieldKey) + '"' +
            ' data-value="' + dataVal + '"' +
            ' data-event-index="' + eventIndex + '"' +
            ' title="' + dataVal + '">' +
            displayVal +
            '</button>' +
            '</td>'
        );
    }

    /**
     * Build a single <td> chip cell for a nested field.
     * @param {Object} event - Event object
     * @param {string} parent - Parent key (e.g. 'location')
     * @param {string} subfield - Subfield key (e.g. 'name')
     * @param {number} eventIndex - Index of this event in the events array
     * @param {number} canonicalIndex - Which event index is canonical
     * @param {boolean} isReadOnly - Whether this field is read-only
     * @param {Object|null} selectedOverrides - Map of compositeKey -> eventIndex for user-selected overrides
     * @returns {string} HTML string
     */
    function buildNestedCell(event, parent, subfield, eventIndex, canonicalIndex, isReadOnly, selectedOverrides) {
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

        if (isReadOnly) {
            // Read-only: show as plain text
            const displayVal = escapeHtml(truncateDisplay(raw));
            return '<td><span class="text-body">' + displayVal + '</span></td>';
        }

        // Override selection takes precedence over canonical; otherwise use canonical
        const compositeKey = parent + '.' + subfield;
        let isSelected = eventIndex === canonicalIndex;
        if (selectedOverrides && selectedOverrides[compositeKey] !== undefined) {
            isSelected = selectedOverrides[compositeKey] === eventIndex;
        }

        const chipClass = isSelected ? 'btn-primary' : 'btn-outline-secondary';
        const displayVal = escapeHtml(truncateDisplay(raw));
        const dataVal = escapeHtml(String(raw));

        return (
            '<td>' +
            '<button class="btn btn-sm ' + chipClass + ' text-start w-100 field-picker-chip"' +
            ' data-action="pick-field"' +
            ' data-field="' + escapeHtml(parent) + '"' +
            ' data-subfield="' + escapeHtml(subfield) + '"' +
            ' data-value="' + dataVal + '"' +
            ' data-event-index="' + eventIndex + '"' +
            ' title="' + dataVal + '">' +
            displayVal +
            '</button>' +
            '</td>'
        );
    }

    /**
     * Render a chip-based comparison table into containerEl.
     * Supports backward compatibility: when called without options, behaves like the original.
     * New mode (with options): supports canonical highlighting, read-only fields, and onPick callback.
     * @param {HTMLElement} containerEl - DOM element to render into
     * @param {Array<Object>} events - Array of event objects to compare
     * @param {Object} options - Optional configuration object
     *   - canonicalIndex: which event (0 or 1) is canonical; defaults to 0
     *   - readOnlyFields: set of field keys to show as read-only (e.g. { 'location.name', 'organizer.name' })
     *   - onPick: callback fn(fieldKey, subfieldKey, value, source); if not provided, uses data-action delegation
     *   - selectedOverrides: map of fieldKey -> eventIndex to show as selected (overrides canonicalIndex)
     */
    function renderFieldPickerTable(containerEl, events, options) {
        if (!containerEl) return;

        // Backward compatibility: if no options, use defaults
        const opts = options || {};
        const canonicalIndex = opts.canonicalIndex !== undefined ? opts.canonicalIndex : 0;
        const readOnlyFields = opts.readOnlyFields || new Set();
        const hasOnPickCallback = opts.onPick && typeof opts.onPick === 'function';
        const selectedOverrides = opts.selectedOverrides || null;

        // Header row — one column per event
        let headerCells = '<th style="width:140px">Field</th>';
        events.forEach((ev, i) => {
            const name = escapeHtml(ev.name || ('Event ' + (i + 1)));
            const label = i === canonicalIndex ? '(canonical) Event ' + (i + 1) : 'Event ' + (i + 1);
            headerCells += '<th>' + escapeHtml(label) + ' — <small class="text-muted">' + name + '</small></th>';
        });

        // Body rows — top-level fields
        let bodyRows = '';
        TOP_LEVEL_FIELDS.forEach(f => {
            const isReadOnly = readOnlyFields.has(f.key);
            let tds = '<td><small class="text-muted">' + escapeHtml(f.label) + '</small></td>';
            events.forEach((ev, eventIdx) => {
                tds += buildTopLevelCell(ev, f.key, eventIdx, canonicalIndex, isReadOnly, selectedOverrides);
            });
            bodyRows += '<tr>' + tds + '</tr>';
        });

        // Body rows — nested fields
        NESTED_FIELDS.forEach(f => {
            const compositeKey = f.parent + '.' + f.subfield;
            const isReadOnly = readOnlyFields.has(compositeKey);
            let tds = '<td><small class="text-muted">' + escapeHtml(f.label) + '</small></td>';
            events.forEach((ev, eventIdx) => {
                tds += buildNestedCell(ev, f.parent, f.subfield, eventIdx, canonicalIndex, isReadOnly, selectedOverrides);
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

        // If onPick callback is provided, wire up event listeners within this container
        // (instead of relying on page-level event delegation for review-queue.js)
        if (hasOnPickCallback) {
            wireUpChipPickers(containerEl, events, opts);
        }
    }

    /**
     * Wire up the 3-state chip picker behavior when onPick callback is provided.
     * @param {HTMLElement} containerEl - Container holding the picker table
     * @param {Array<Object>} events - Event objects for reference
     * @param {Object} opts - Options including onPick callback
     */
    function wireUpChipPickers(containerEl, events, opts) {
        const canonicalIndex = opts.canonicalIndex !== undefined ? opts.canonicalIndex : 0;
        const onPick = opts.onPick;

        containerEl.addEventListener('click', function(e) {
            const chip = e.target.closest('.field-picker-chip');
            if (!chip) return;

            e.preventDefault();
            e.stopPropagation();

            const fieldKey = chip.dataset.field;
            const subfieldKey = chip.dataset.subfield || null;
            const value = chip.dataset.value || '';
            const eventIndex = parseInt(chip.dataset.eventIndex, 10);
            const source = eventIndex === canonicalIndex ? 'this' : 'related';

            if (!fieldKey) return;

            // Call the onPick callback
            if (onPick) {
                onPick(fieldKey, subfieldKey, value, source);
            }

            // Revert sibling chips in this column to outline
            const table = chip.closest('table');
            if (table) {
                const rows = table.querySelectorAll('tbody tr');
                rows.forEach(row => {
                    const cells = row.querySelectorAll('td');
                    if (cells.length > eventIndex + 1) { // +1 for label cell
                        const siblingChip = cells[eventIndex + 1].querySelector('.field-picker-chip');
                        if (siblingChip && siblingChip !== chip) {
                            siblingChip.classList.remove('btn-primary');
                            siblingChip.classList.add('btn-outline-secondary');
                        }
                    }
                });
            }

            // Highlight this chip
            chip.classList.remove('btn-outline-secondary');
            chip.classList.add('btn-primary');
        });
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
