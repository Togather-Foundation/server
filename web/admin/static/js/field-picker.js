// Field Picker — shared chip-based field comparison table
// Used by consolidate.js and review-queue fold-down (srv-7roii).
//
// Constraints (web/AGENTS.md):
//   - No inline scripts or onclick handlers — all interactivity via data-action + event delegation
//   - Always escapeHtml() before any innerHTML interpolation
//   - Exposes window.FieldPicker for callers
//
// Design (docs/design/review-queue-ui.md):
//   - 3-state chip behavior: outline (unselected) | blue/primary (canonical default) | green/success (user-selected)
//   - Pencil icon inside blue and green chips; clicking pencil on blue atomically selects + opens editor
//   - Inline editor replaces chip td content: <input> or <textarea> + Save/Cancel
//   - Read-only fields (location.name, organizer.name) shown as plain text
//   - onPick callback: (fieldKey, subfieldKey, value, source) when chip is picked or edit saved

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

    // Fields that get a <textarea> instead of <input> in the inline editor
    const TEXTAREA_FIELDS = new Set(['description']);

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
     * Build the pencil icon span for inline editing.
     * Uses visibility:hidden on outline chips (not display:none) so layout is stable.
     * @param {string} chipClass - The CSS class of the chip ('btn-primary', 'btn-success', or 'btn-outline-secondary')
     * @returns {string} HTML string
     */
    function buildPencilIcon(chipClass) {
        const isVisible = chipClass === 'btn-primary' || chipClass === 'btn-success';
        const visibility = isVisible ? 'visible' : 'hidden';
        return (
            '<span class="field-picker-edit-icon ms-1" data-action="edit-field"' +
            ' style="visibility:' + visibility + '; font-size:1.25em; opacity:0.9; cursor:pointer; flex-shrink:0;"' +
            ' title="Edit this value"' +
            ' aria-label="Edit">✎</span>'
        );
    }

    /**
     * Build the chip + pencil wrapper for a <td>.
     * Wraps both in a flex row so the pencil is contained within the cell boundary.
     * @param {string} chipHtml - The <button> HTML string
     * @param {string} chipClass - CSS class of the chip (for pencil visibility)
     * @returns {string} Full <td> HTML
     */
    function buildChipCell(chipHtml, chipClass) {
        return (
            '<td class="pe-4">' +
            '<div class="d-flex align-items-center gap-2">' +
            chipHtml +
            buildPencilIcon(chipClass) +
            '</div>' +
            '</td>'
        );
    }

    /**
     * Build a single <td> chip cell for a top-level field.
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
            const displayVal = escapeHtml(truncateDisplay(raw));
            return '<td><span class="text-body">' + displayVal + '</span></td>';
        }

        let chipClass;
        if (selectedOverrides && selectedOverrides[fieldKey] !== undefined) {
            chipClass = selectedOverrides[fieldKey] === eventIndex ? 'btn-success' : 'btn-outline-secondary';
        } else if (eventIndex === canonicalIndex) {
            chipClass = 'btn-primary';
        } else {
            chipClass = 'btn-outline-secondary';
        }
        const displayVal = escapeHtml(truncateDisplay(raw));
        const dataVal = escapeHtml(String(raw));

        const chipHtml = (
            '<button class="btn btn-sm ' + chipClass + ' text-start field-picker-chip flex-grow-1"' +
            ' data-action="pick-field"' +
            ' data-field="' + escapeHtml(fieldKey) + '"' +
            ' data-value="' + dataVal + '"' +
            ' data-event-index="' + eventIndex + '"' +
            ' title="' + dataVal + '">' +
            displayVal +
            '</button>'
        );
        return buildChipCell(chipHtml, chipClass);
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
        const lookupObj = (parent === 'location' && subfield !== 'name' && parentObj && parentObj.address)
            ? parentObj.address
            : parentObj;
        const raw = lookupObj && lookupObj[subfield] != null ? lookupObj[subfield] : null;
        if (raw == null || raw === '') {
            return '<td><span class="text-muted small">—</span></td>';
        }

        if (isReadOnly) {
            const displayVal = escapeHtml(truncateDisplay(raw));
            return '<td><span class="text-body">' + displayVal + '</span></td>';
        }

        const compositeKey = parent + '.' + subfield;

        let chipClass;
        if (selectedOverrides && selectedOverrides[compositeKey] !== undefined) {
            chipClass = selectedOverrides[compositeKey] === eventIndex ? 'btn-success' : 'btn-outline-secondary';
        } else if (eventIndex === canonicalIndex) {
            chipClass = 'btn-primary';
        } else {
            chipClass = 'btn-outline-secondary';
        }
        const displayVal = escapeHtml(truncateDisplay(raw));
        const dataVal = escapeHtml(String(raw));

        const chipHtml = (
            '<button class="btn btn-sm ' + chipClass + ' text-start field-picker-chip flex-grow-1"' +
            ' data-action="pick-field"' +
            ' data-field="' + escapeHtml(parent) + '"' +
            ' data-subfield="' + escapeHtml(subfield) + '"' +
            ' data-value="' + dataVal + '"' +
            ' data-event-index="' + eventIndex + '"' +
            ' title="' + dataVal + '">' +
            displayVal +
            '</button>'
        );
        return buildChipCell(chipHtml, chipClass);
    }

    /**
     * Render a chip-based comparison table into containerEl.
     * @param {HTMLElement} containerEl - DOM element to render into
     * @param {Array<Object>} events - Array of event objects to compare
     * @param {Object} options - Optional configuration object
     *   - canonicalIndex: which event (0 or 1) is canonical; defaults to 0
     *   - readOnlyFields: set of field keys to show as read-only
     *   - onPick: callback fn(fieldKey, subfieldKey, value, source, edited)
     *   - selectedOverrides: map of fieldKey -> eventIndex to show as selected
     */
    function renderFieldPickerTable(containerEl, events, options) {
        if (!containerEl) return;

        const opts = options || {};
        const canonicalIndex = opts.canonicalIndex !== undefined ? opts.canonicalIndex : 0;
        const readOnlyFields = opts.readOnlyFields || new Set();
        const hasOnPickCallback = opts.onPick && typeof opts.onPick === 'function';
        const selectedOverrides = opts.selectedOverrides || null;

        let headerCells = '<th style="width:140px">Field</th>';
        events.forEach((ev, i) => {
            const name = escapeHtml(ev.name || ('Event ' + (i + 1)));
            const label = i === canonicalIndex ? '(canonical) Event ' + (i + 1) : 'Event ' + (i + 1);
            headerCells += '<th>' + escapeHtml(label) + ' — <small class="text-muted">' + name + '</small></th>';
        });

        let bodyRows = '';
        TOP_LEVEL_FIELDS.forEach(f => {
            const isReadOnly = readOnlyFields.has(f.key);
            let tds = '<td><small class="text-muted">' + escapeHtml(f.label) + '</small></td>';
            events.forEach((ev, eventIdx) => {
                tds += buildTopLevelCell(ev, f.key, eventIdx, canonicalIndex, isReadOnly, selectedOverrides);
            });
            bodyRows += '<tr>' + tds + '</tr>';
        });

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

        if (hasOnPickCallback) {
            wireUpChipPickers(containerEl, events, opts);
        }
    }

    // -------------------------------------------------------------------------
    // Inline editor
    // -------------------------------------------------------------------------

    /**
     * Open an inline editor inside a chip's <td>, replacing the chip + pencil
     * with an <input> or <textarea> and Save/Cancel buttons.
     *
     * @param {HTMLElement} td              - The <td> containing the chip
     * @param {HTMLElement} chip            - The chip button being edited
     * @param {string}      fieldKey        - Field key (used to pick input vs textarea)
     * @param {string}      restoreChipClass - Chip class to restore on cancel ('btn-primary' or 'btn-success')
     * @param {Function}    onSave          - Called with (newValue) when admin confirms
     * @param {Function}    onCancel        - Called with no args when admin cancels
     */
    function openInlineEditor(td, chip, fieldKey, restoreChipClass, onSave, onCancel) {
        const currentValue = chip.dataset.value || '';
        const useTextarea = TEXTAREA_FIELDS.has(fieldKey);

        // Snapshot what we need to rebuild the chip on cancel.
        // We do NOT use td.innerHTML because the caller may have already mutated chip state
        // (blue → green) before calling us; we want to restore to the pre-click chip class.
        const savedChipClass  = restoreChipClass;
        const savedValue      = currentValue;
        const savedFieldKey   = chip.dataset.field;
        const savedSubfield   = chip.dataset.subfield || null;
        const savedEventIndex = chip.dataset.eventIndex;
        const savedTitle      = chip.title;
        const savedDisplay    = chip.textContent.trim();

        let inputEl;
        if (useTextarea) {
            inputEl = document.createElement('textarea');
            inputEl.className = 'form-control form-control-sm mb-1';
            inputEl.rows = 4;
        } else {
            inputEl = document.createElement('input');
            inputEl.type = 'text';
            inputEl.className = 'form-control form-control-sm mb-1';
        }
        inputEl.value = currentValue;

        const saveBtn = document.createElement('button');
        saveBtn.type = 'button';
        saveBtn.className = 'btn btn-sm btn-success me-1';
        saveBtn.textContent = 'Save';
        saveBtn.dataset.action = 'edit-field-save';

        const cancelBtn = document.createElement('button');
        cancelBtn.type = 'button';
        cancelBtn.className = 'btn btn-sm btn-outline-secondary';
        cancelBtn.textContent = 'Cancel';
        cancelBtn.dataset.action = 'edit-field-cancel';

        const btnGroup = document.createElement('div');
        btnGroup.appendChild(saveBtn);
        btnGroup.appendChild(cancelBtn);

        // Replace td content with editor
        td.innerHTML = '';
        td.appendChild(inputEl);
        td.appendChild(btnGroup);
        inputEl.focus();
        inputEl.select();

        function handleSave() {
            const newValue = inputEl.value.trim();
            cleanup();
            onSave(newValue);
        }

        function handleCancel() {
            // Rebuild the chip at its pre-click class (blue if it was blue, green if it was green)
            const restoredChipHtml = (
                '<button class="btn btn-sm ' + savedChipClass + ' text-start field-picker-chip flex-grow-1"' +
                ' data-action="pick-field"' +
                ' data-field="' + escapeHtml(savedFieldKey) + '"' +
                (savedSubfield ? ' data-subfield="' + escapeHtml(savedSubfield) + '"' : '') +
                ' data-value="' + escapeHtml(savedValue) + '"' +
                ' data-event-index="' + escapeHtml(savedEventIndex) + '"' +
                ' title="' + escapeHtml(savedTitle) + '">' +
                escapeHtml(savedDisplay) +
                '</button>'
            );
            td.innerHTML = (
                '<div class="d-flex align-items-center gap-2">' +
                restoredChipHtml +
                buildPencilIcon(savedChipClass) +
                '</div>'
            );
            cleanup();
            onCancel();
        }

        function handleKeydown(e) {
            if (e.key === 'Enter' && !useTextarea) {
                e.preventDefault();
                handleSave();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                handleCancel();
            }
        }

        function cleanup() {
            saveBtn.removeEventListener('click', handleSave);
            cancelBtn.removeEventListener('click', handleCancel);
            inputEl.removeEventListener('keydown', handleKeydown);
        }

        saveBtn.addEventListener('click', handleSave);
        cancelBtn.addEventListener('click', handleCancel);
        inputEl.addEventListener('keydown', handleKeydown);
    }

    // -------------------------------------------------------------------------
    // Event wiring
    // -------------------------------------------------------------------------

    /**
     * Wire up chip picking and inline editing when onPick callback is provided.
     * Handles two data-action values:
     *   - 'pick-field'  — chip body click: select value
     *   - 'edit-field'  — pencil icon click: open inline editor
     *
     * @param {HTMLElement} containerEl
     * @param {Array<Object>} events
     * @param {Object} opts
     */
    function wireUpChipPickers(containerEl, events, opts) {
        const canonicalIndex = opts.canonicalIndex !== undefined ? opts.canonicalIndex : 0;
        const onPick = opts.onPick;

        containerEl.addEventListener('click', function(e) {
            // ---- Pencil icon click → open inline editor -------------------------
            const pencil = e.target.closest('[data-action="edit-field"]');
            if (pencil) {
                e.preventDefault();
                e.stopPropagation();

                const td = pencil.closest('td');
                const chip = td && td.querySelector('.field-picker-chip');
                if (!chip) return;

                const fieldKey    = chip.dataset.field;
                const subfieldKey = chip.dataset.subfield || null;
                const eventIndex  = parseInt(chip.dataset.eventIndex, 10);
                const source      = eventIndex === canonicalIndex ? 'this' : 'related';

                // Capture original chip class before any mutation — cancel will restore to this.
                const originalChipClass = chip.classList.contains('btn-primary') ? 'btn-primary' : 'btn-success';

                // If chip is blue (auto-canonical, not yet user-selected),
                // atomically select it (turn green) before opening the editor.
                if (originalChipClass === 'btn-primary') {
                    const row = chip.closest('tr');
                    if (row) {
                        row.querySelectorAll('.field-picker-chip').forEach(sib => {
                            if (sib !== chip) {
                                sib.classList.remove('btn-primary', 'btn-success');
                                sib.classList.add('btn-outline-secondary');
                                const sibPencil = sib.closest('td') && sib.closest('td').querySelector('[data-action="edit-field"]');
                                if (sibPencil) sibPencil.style.visibility = 'hidden';
                            }
                        });
                    }
                    chip.classList.remove('btn-outline-secondary', 'btn-primary');
                    chip.classList.add('btn-success');
                    // Fire onPick for the selection (not-edited, just selecting)
                    if (onPick) {
                        onPick(fieldKey, subfieldKey, chip.dataset.value || '', source, false);
                    }
                }

                openInlineEditor(td, chip, fieldKey, originalChipClass,
                    function onSave(newValue) {
                        // Rebuild the td with a green chip showing the new value
                        const displayVal = escapeHtml(truncateDisplay(newValue));
                        const dataVal    = escapeHtml(newValue);

                        td.innerHTML =
                            '<div class="d-flex align-items-center gap-2">' +
                            '<button class="btn btn-sm btn-success text-start field-picker-chip flex-grow-1"' +
                            ' data-action="pick-field"' +
                            ' data-field="' + escapeHtml(fieldKey) + '"' +
                            (subfieldKey ? ' data-subfield="' + escapeHtml(subfieldKey) + '"' : '') +
                            ' data-value="' + dataVal + '"' +
                            ' data-event-index="' + eventIndex + '"' +
                            ' title="' + dataVal + '">' +
                            displayVal +
                            '</button>' +
                            buildPencilIcon('btn-success') +
                            '</div>';

                        // Fire onPick with edited=true so consolidate knows to patch this field
                        if (onPick) {
                            onPick(fieldKey, subfieldKey, newValue, source, true);
                        }
                    },
                    function onCancel() {
                        // openInlineEditor's cancel handler restores the chip to originalChipClass.
                        // If the chip was blue, it returns to blue; fieldOverrides retains the
                        // canonical-value selection from the onPick call above, which is harmless
                        // (same value as the canonical default — no patch will be sent).
                    }
                );
                return;
            }

            // ---- Chip body click → select value ---------------------------------
            const chip = e.target.closest('.field-picker-chip[data-action="pick-field"]');
            if (!chip) return;

            e.preventDefault();
            e.stopPropagation();

            const fieldKey    = chip.dataset.field;
            const subfieldKey = chip.dataset.subfield || null;
            const value       = chip.dataset.value || '';
            const eventIndex  = parseInt(chip.dataset.eventIndex, 10);
            const source      = eventIndex === canonicalIndex ? 'this' : 'related';

            if (!fieldKey) return;

            if (onPick) {
                onPick(fieldKey, subfieldKey, value, source, false);
            }

            // Revert siblings in the same row to outline, hide their pencils
            const row = chip.closest('tr');
            if (row) {
                row.querySelectorAll('.field-picker-chip').forEach(sibling => {
                    if (sibling !== chip) {
                        sibling.classList.remove('btn-primary', 'btn-success');
                        sibling.classList.add('btn-outline-secondary');
                        const sibPencil = sibling.closest('td') && sibling.closest('td').querySelector('[data-action="edit-field"]');
                        if (sibPencil) sibPencil.style.visibility = 'hidden';
                    }
                });
            }

            // Highlight this chip green and show its pencil
            chip.classList.remove('btn-outline-secondary', 'btn-primary');
            chip.classList.add('btn-success');
            const ownPencil = chip.closest('td') && chip.closest('td').querySelector('[data-action="edit-field"]');
            if (ownPencil) ownPencil.style.visibility = 'visible';
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
