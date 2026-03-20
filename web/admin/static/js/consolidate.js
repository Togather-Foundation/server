// Consolidate Events — admin page JS
// Wires up /admin/events/consolidate (consolidate.html)
//
// Constraints (web/AGENTS.md):
//   - No inline scripts or onclick handlers — all interactivity via data-action + event delegation
//   - Always use API.* (from api.js) — never raw fetch()
//   - Always escapeHtml() before any innerHTML interpolation
//   - Use showToast, confirmAction, setLoading from components.js
//   - Tabler/Bootstrap CSS classes

(function () {
    'use strict';

    // -------------------------------------------------------------------------
    // State
    // -------------------------------------------------------------------------

    /** @type {Array<Object>} Array of admin API event response objects, in load order */
    let loadedEvents = [];

    /** @type {Set<string>} ULIDs currently checked for retirement */
    let retireSet = new Set();

    // -------------------------------------------------------------------------
    // Crockford Base32 ULID validation
    // -------------------------------------------------------------------------

    /** @param {string} s */
    function isValidUlid(s) {
        return /^[0123456789ABCDEFGHJKMNPQRSTVWXYZ]{26}$/i.test(s);
    }

    // -------------------------------------------------------------------------
    // ULID input rows
    // -------------------------------------------------------------------------

    function createUlidRow(prefillValue) {
        const div = document.createElement('div');
        div.className = 'input-group mb-2 ulid-input-row';
        div.innerHTML =
            '<input type="text" class="form-control font-monospace"' +
            ' placeholder="01XXXXXXXXXXXXXXXXXXXXXXXXX" maxlength="26">' +
            '<button class="btn btn-outline-secondary" data-action="remove-ulid-input">Remove</button>';
        if (prefillValue) {
            div.querySelector('input').value = prefillValue;
        }
        return div;
    }

    function renderInitialUlidRows() {
        const container = document.getElementById('ulid-inputs');
        if (!container) return;
        container.innerHTML = '';
        container.appendChild(createUlidRow());
        container.appendChild(createUlidRow());
    }

    function addUlidRow(prefillValue) {
        const container = document.getElementById('ulid-inputs');
        if (!container) return;
        container.appendChild(createUlidRow(prefillValue));
    }

    function removeUlidRow(target) {
        const container = document.getElementById('ulid-inputs');
        if (!container) return;
        const rows = container.querySelectorAll('.ulid-input-row');
        if (rows.length <= 2) {
            showToast('At least two ULID inputs are required.', 'warning');
            return;
        }
        const row = target.closest('.ulid-input-row');
        if (row) row.remove();
    }

    function getUlidInputValues() {
        const container = document.getElementById('ulid-inputs');
        if (!container) return [];
        return Array.from(container.querySelectorAll('.ulid-input-row input'))
            .map(el => el.value.trim().toUpperCase())
            .filter(v => v.length > 0);
    }

    // -------------------------------------------------------------------------
    // URL param pre-fill
    // -------------------------------------------------------------------------

    function applyUrlParams() {
        const params = new URLSearchParams(window.location.search);
        const raw = params.get('ulids');
        if (!raw) return;
        const ulids = raw.split(',').map(s => s.trim().toUpperCase()).filter(s => s.length > 0);
        if (ulids.length < 1) return;

        const container = document.getElementById('ulid-inputs');
        if (!container) return;
        container.innerHTML = '';
        ulids.forEach(u => container.appendChild(createUlidRow(u)));
        // Ensure minimum 2 rows
        while (container.querySelectorAll('.ulid-input-row').length < 2) {
            container.appendChild(createUlidRow());
        }

        // Auto-load if at least 2 valid ULIDs
        if (ulids.length >= 2 && ulids.every(isValidUlid)) {
            loadEvents();
        }
    }

    // -------------------------------------------------------------------------
    // Load events
    // -------------------------------------------------------------------------

    async function loadEvents() {
        const errorDiv = document.getElementById('load-error');
        if (errorDiv) errorDiv.style.display = 'none';

        const ulids = getUlidInputValues();

        // Validate
        if (ulids.length < 2) {
            showError(errorDiv, 'Please enter at least two ULIDs.');
            return;
        }
        for (const u of ulids) {
            if (!isValidUlid(u)) {
                showError(errorDiv, `Invalid ULID: "${escapeHtml(u)}". ULIDs are 26 characters, Crockford Base32.`);
                return;
            }
        }

        const btn = document.querySelector('[data-action="load-events"]');
        if (btn) setLoading(btn, true);

        try {
            const results = await Promise.all(ulids.map(u => API.events.get(u)));
            loadedEvents = results;
            retireSet = new Set(ulids);
            renderWorkspace();
            const workspace = document.getElementById('consolidate-workspace');
            if (workspace) {
                workspace.style.display = '';
                workspace.scrollIntoView({ behavior: 'smooth', block: 'start' });
            }
        } catch (err) {
            const msg = err.message || 'Failed to load events.';
            showError(errorDiv, msg);
            showToast(msg, 'danger');
        } finally {
            if (btn) setLoading(btn, false);
        }
    }

    /** Show an error message in the load-error div */
    function showError(div, msg) {
        if (!div) return;
        div.textContent = msg;
        div.style.display = '';
    }

    // -------------------------------------------------------------------------
    // Workspace rendering
    // -------------------------------------------------------------------------

    function renderWorkspace() {
        renderFieldPickerTable();
        renderRetireList();
        // Reset result panel
        const result = document.getElementById('consolidate-result');
        if (result) result.style.display = 'none';
    }

    // -------------------------------------------------------------------------
    // Field picker table — delegates to shared FieldPicker module (field-picker.js)
    // -------------------------------------------------------------------------

    function renderFieldPickerTable() {
        if (typeof FieldPicker === 'undefined') return;
        FieldPicker.renderFieldPickerTable(
            document.getElementById('field-picker-table'),
            loadedEvents
        );
    }

    // -------------------------------------------------------------------------
    // Retire checklist
    // -------------------------------------------------------------------------

    function renderRetireList() {
        const container = document.getElementById('retire-list');
        if (!container) return;

        if (loadedEvents.length === 0) {
            container.innerHTML = '<p class="text-muted">No events loaded.</p>';
            return;
        }

        let html = '';
        loadedEvents.forEach(ev => {
            const ulid = extractUlid(ev);
            if (!ulid) return;
            const name = escapeHtml(ev.name || 'Untitled Event');
            const dateStr = escapeHtml(ev.startDate || '');
            const escapedUlid = escapeHtml(ulid);
            html +=
                '<div class="form-check mb-2">' +
                '<input class="form-check-input" type="checkbox" checked' +
                ' data-action="retire-toggle"' +
                ' data-ulid="' + escapedUlid + '"' +
                ' id="retire-' + escapedUlid + '">' +
                '<label class="form-check-label" for="retire-' + escapedUlid + '">' +
                '<strong>' + name + '</strong>' +
                '<small class="text-muted ms-2">' + escapedUlid +
                (dateStr ? ' — ' + dateStr : '') +
                '</small>' +
                '</label>' +
                '</div>';
        });

        container.innerHTML = html || '<p class="text-muted">No events to retire.</p>';
    }

    // -------------------------------------------------------------------------
    // ULID extraction from admin API event object
    // -------------------------------------------------------------------------

    /**
     * Extract ULID from an admin API event response.
     * The admin API returns flat JSON; ULID may be in `ulid`, `id`, or `@id`.
     * @param {Object} ev
     * @returns {string|null}
     */
    function extractUlid(ev) {
        if (!ev) return null;
        if (ev.ulid) return ev.ulid;
        if (ev.id && /^[0123456789ABCDEFGHJKMNPQRSTVWXYZ]{26}$/i.test(ev.id)) return ev.id;
        if (ev['@id']) {
            const m = ev['@id'].match(/\/([A-Z0-9]{26})(?:\/|$)/i);
            if (m) return m[1];
        }
        return null;
    }

    // -------------------------------------------------------------------------
    // Field picker handler
    // -------------------------------------------------------------------------

    /**
     * Map a field key (+ optional subfield) to the corresponding form input element.
     * @param {string} field
     * @param {string|undefined} subfield
     * @returns {HTMLElement|null}
     */
    function fieldInputElement(field, subfield) {
        if (!subfield) {
            return document.getElementById('field-' + field);
        }
        return document.getElementById('field-' + field + '-' + subfield);
    }

    function handlePickField(target) {
        const field = target.dataset.field;
        const subfield = target.dataset.subfield || null;
        const value = target.dataset.value || '';

        if (!field) return;

        const input = fieldInputElement(field, subfield);
        if (!input) {
            showToast('Could not find input for field: ' + field + (subfield ? '.' + subfield : ''), 'warning');
            return;
        }
        input.value = value;
        // Brief visual feedback
        input.classList.add('border-primary');
        setTimeout(() => input.classList.remove('border-primary'), 800);
    }

    // -------------------------------------------------------------------------
    // Populate editor from a full event object
    // -------------------------------------------------------------------------

    /**
     * Populate all editable form inputs from a loaded event data object.
     * @param {Object} ev - Admin API event response (flat camelCase JSON)
     */
    function populateEditorFromEvent(ev) {
        if (!ev) return;

        const set = (id, val) => {
            const el = document.getElementById(id);
            if (el) el.value = val != null ? String(val) : '';
        };

        set('field-name', ev.name);
        set('field-startDate', ev.startDate);
        set('field-endDate', ev.endDate);
        set('field-description', ev.description);
        set('field-url', ev.url);
        set('field-image', ev.image);

        // Public API returns location as a schema.org Place with address nested under
        // location.address.*; fall back to location.* for any flat admin response shape.
        const loc = ev.location || {};
        const locAddr = loc.address || loc;
        set('field-location-name', loc.name);
        set('field-location-streetAddress', locAddr.streetAddress);
        set('field-location-addressLocality', locAddr.addressLocality);
        set('field-location-addressRegion', locAddr.addressRegion);
        set('field-location-postalCode', locAddr.postalCode);

        const org = ev.organizer || {};
        set('field-organizer-name', org.name);
        set('field-organizer-url', org.url);
    }

    // -------------------------------------------------------------------------
    // Read-only event info panel (promote mode)
    // -------------------------------------------------------------------------

    /**
     * Render a read-only summary of an event into the promote panel.
     * @param {Object} ev
     */
    function renderReadonlyEventInfo(ev) {
        const container = document.getElementById('event-info-readonly-content');
        if (!container) return;

        const row = (label, val) => {
            if (!val) return '';
            return (
                '<div class="mb-2">' +
                '<small class="text-muted">' + escapeHtml(label) + ':</small>' +
                '<div>' + escapeHtml(String(val)) + '</div>' +
                '</div>'
            );
        };

        const loc = ev.location || {};
        const org = ev.organizer || {};
        // Public API nests address fields under location.address.*
        const locAddr = loc.address || loc;
        const locParts = [loc.name, locAddr.streetAddress, locAddr.addressLocality, locAddr.addressRegion, locAddr.postalCode]
            .filter(Boolean).join(', ');

        let html =
            row('Event Name', ev.name) +
            row('Start Date', ev.startDate) +
            row('End Date', ev.endDate) +
            row('URL', ev.url) +
            row('Image URL', ev.image) +
            row('Location', locParts) +
            row('Organizer', [org.name, org.url].filter(Boolean).join(' — ')) +
            (ev.description
                ? '<div class="mb-2"><small class="text-muted">Description:</small>' +
                  '<div class="small">' + escapeHtml(
                      ev.description.length > 300
                          ? ev.description.substring(0, 297) + '…'
                          : ev.description
                  ) + '</div></div>'
                : '');

        container.innerHTML = html || '<p class="text-muted">No data.</p>';
    }

    // -------------------------------------------------------------------------
    // canonical-mode-change handler
    // -------------------------------------------------------------------------

    function handleCanonicalModeChange(target) {
        const mode = target.value;
        const promoteRow = document.getElementById('promote-ulid-row');
        const readonlyPanel = document.getElementById('event-info-readonly');
        const editablePanel = document.getElementById('event-info-editable');

        if (!promoteRow || !editablePanel) return;

        if (mode === 'promote') {
            promoteRow.style.display = '';
            if (editablePanel) editablePanel.style.display = '';
        } else {
            // create mode
            promoteRow.style.display = 'none';
            if (readonlyPanel) readonlyPanel.style.display = 'none';
            if (editablePanel) editablePanel.style.display = '';
        }
    }

    // -------------------------------------------------------------------------
    // load-promote-event handler
    // -------------------------------------------------------------------------

    async function handleLoadPromoteEvent(target) {
        const ulidInput = document.getElementById('promote-ulid-input');
        if (!ulidInput) return;
        const ulid = ulidInput.value.trim().toUpperCase();

        if (!ulid) {
            showToast('Please enter a ULID for the canonical event.', 'warning');
            return;
        }
        if (!isValidUlid(ulid)) {
            showToast('Invalid ULID format. ULIDs are 26 Crockford Base32 characters.', 'danger');
            return;
        }

        setLoading(target, true);
        try {
            const ev = await API.events.get(ulid);
            populateEditorFromEvent(ev);
            renderReadonlyEventInfo(ev);

            const readonlyPanel = document.getElementById('event-info-readonly');
            const editablePanel = document.getElementById('event-info-editable');
            if (readonlyPanel) readonlyPanel.style.display = '';
            if (editablePanel) editablePanel.style.display = 'none';
        } catch (err) {
            showToast(err.message || 'Failed to load event.', 'danger');
        } finally {
            setLoading(target, false);
        }
    }

    // -------------------------------------------------------------------------
    // edit-canonical handler
    // -------------------------------------------------------------------------

    function handleEditCanonical() {
        const readonlyPanel = document.getElementById('event-info-readonly');
        const editablePanel = document.getElementById('event-info-editable');
        if (readonlyPanel) readonlyPanel.style.display = 'none';
        if (editablePanel) editablePanel.style.display = '';
    }

    // -------------------------------------------------------------------------
    // retire-toggle handler
    // -------------------------------------------------------------------------

    function handleRetireToggle(target) {
        const ulid = target.dataset.ulid;
        if (!ulid) return;
        if (target.checked) {
            retireSet.add(ulid);
        } else {
            retireSet.delete(ulid);
        }
    }

    // -------------------------------------------------------------------------
    // Build request body
    // -------------------------------------------------------------------------

    function buildRequestBody() {
        const modeEl = document.querySelector('[name="canonical-mode"]:checked');
        const mode = modeEl ? modeEl.value : 'create';
        const retireUlids = Array.from(retireSet);

        if (mode === 'promote') {
            const ulid = (document.getElementById('promote-ulid-input') || {}).value || '';
            return { event_ulid: ulid.trim(), retire: retireUlids };
        }

        // create mode — build EventInput
        const event = {
            name: (document.getElementById('field-name') || {}).value || '',
            startDate: (document.getElementById('field-startDate') || {}).value || '',
            endDate: (document.getElementById('field-endDate') || {}).value || '',
            description: (document.getElementById('field-description') || {}).value || '',
            url: (document.getElementById('field-url') || {}).value || '',
            image: (document.getElementById('field-image') || {}).value || '',
        };

        // Trim all strings
        Object.keys(event).forEach(k => { event[k] = event[k].trim(); });

        const locName = ((document.getElementById('field-location-name') || {}).value || '').trim();
        const locStreet = ((document.getElementById('field-location-streetAddress') || {}).value || '').trim();
        const locCity = ((document.getElementById('field-location-addressLocality') || {}).value || '').trim();
        const locRegion = ((document.getElementById('field-location-addressRegion') || {}).value || '').trim();
        const locPostal = ((document.getElementById('field-location-postalCode') || {}).value || '').trim();
        if (locName || locStreet || locCity) {
            event.location = {
                name: locName,
                streetAddress: locStreet,
                addressLocality: locCity,
                addressRegion: locRegion,
                postalCode: locPostal,
            };
        }

        const orgName = ((document.getElementById('field-organizer-name') || {}).value || '').trim();
        const orgUrl = ((document.getElementById('field-organizer-url') || {}).value || '').trim();
        if (orgName || orgUrl) {
            event.organizer = { name: orgName, url: orgUrl };
        }

        // Remove empty strings from top-level EventInput
        Object.keys(event).forEach(k => {
            if (typeof event[k] === 'string' && event[k] === '') delete event[k];
        });

        return { event, retire: retireUlids };
    }

    // -------------------------------------------------------------------------
    // Validation
    // -------------------------------------------------------------------------

    /**
     * Validate inputs before submission.
     * @returns {boolean} true if valid
     */
    function validateBeforeSubmit() {
        const modeEl = document.querySelector('[name="canonical-mode"]:checked');
        const mode = modeEl ? modeEl.value : 'create';

        if (mode === 'promote') {
            const ulidInput = document.getElementById('promote-ulid-input');
            const ulid = ulidInput ? ulidInput.value.trim() : '';
            if (!ulid) {
                showToast('Please enter the ULID of the canonical event to promote.', 'danger');
                return false;
            }
            if (!isValidUlid(ulid)) {
                showToast('Canonical event ULID is invalid. ULIDs are 26 Crockford Base32 characters.', 'danger');
                return false;
            }
        } else {
            // create mode — name is required
            const nameInput = document.getElementById('field-name');
            const name = nameInput ? nameInput.value.trim() : '';
            if (!name) {
                showToast('Event Name is required.', 'danger');
                if (nameInput) nameInput.focus();
                return false;
            }
        }

        if (retireSet.size === 0) {
            showToast('At least one event must be selected for retirement.', 'danger');
            return false;
        }

        return true;
    }

    // -------------------------------------------------------------------------
    // Lifecycle state badge
    // -------------------------------------------------------------------------

    function lifecycleBadge(state) {
        if (!state) return '<span class="badge bg-secondary">unknown</span>';
        const colours = {
            'active':         'success',
            'pending_review': 'warning',
            'retired':        'secondary',
            'draft':          'info',
        };
        const colour = colours[state] || 'secondary';
        return '<span class="badge bg-' + colour + '">' + escapeHtml(state) + '</span>';
    }

    // -------------------------------------------------------------------------
    // Submit consolidate
    // -------------------------------------------------------------------------

    async function handleSubmitConsolidate(target) {
        if (!validateBeforeSubmit()) return;

        const body = buildRequestBody();

        setLoading(target, true);
        try {
            const result = await API.events.consolidate(body);
            renderConsolidateResult(result);

            // Scroll to result
            const resultDiv = document.getElementById('consolidate-result');
            if (resultDiv) {
                resultDiv.style.display = '';
                resultDiv.scrollIntoView({ behavior: 'smooth', block: 'start' });
            }
        } catch (err) {
            showToast(err.message || 'Consolidation failed.', 'danger');
        } finally {
            setLoading(target, false);
        }
    }

    // -------------------------------------------------------------------------
    // Result rendering
    // -------------------------------------------------------------------------

    function renderConsolidateResult(result) {
        const container = document.getElementById('consolidate-result');
        if (!container) return;

        // Extract canonical event
        const ev = result.event || result.canonical_event || result || {};

        // Resolve ULID
        let ulid = ev.ulid || ev.id || null;
        if (!ulid && ev['@id']) {
            const m = ev['@id'].match(/\/([A-Z0-9]{26})(?:\/|$)/i);
            if (m) ulid = m[1];
        }

        const name = ev.name || '(no name)';
        const state = result.lifecycle_state || ev.lifecycle_state || ev.lifecycleState || ev.status || '';
        const retiredCount = Array.isArray(result.retired) ? result.retired.length : retireSet.size;

        const warnings = Array.isArray(result.warnings) ? result.warnings : [];
        const isDuplicate = result.is_duplicate || false;
        const needsReview = result.needs_review || state === 'pending_review';

        let warningsHtml = '';
        if (warnings.length > 0) {
            warningsHtml =
                '<div class="alert alert-warning mt-2">' +
                '<strong>Warnings:</strong>' +
                '<ul class="mb-0 mt-1">' +
                warnings.map(w => {
                    const msg = w.message || w.code || JSON.stringify(w);
                    const label = w.field ? escapeHtml(w.field) + ': ' : '';
                    return '<li>' + label + escapeHtml(msg) + '</li>';
                }).join('') +
                '</ul></div>';
        }

        let flagHtml = '';
        if (isDuplicate || needsReview) {
            flagHtml =
                '<div class="alert alert-warning mt-2">This event was flagged. Review warnings above.</div>';
        }

        const eventLink = ulid
            ? '<a href="/admin/events/' + escapeHtml(ulid) + '">' + escapeHtml(name) + '</a>'
            : escapeHtml(name);

        const reviewQueueBtn = (state === 'pending_review')
            ? '<button class="btn btn-outline-secondary" data-action="goto-review-queue">Go to Review Queue</button>'
            : '';

        container.innerHTML =
            '<div class="card border-success mb-4">' +
            '<div class="card-header bg-success-lt">' +
            '<h3 class="card-title text-success">Consolidation complete</h3>' +
            '</div>' +
            '<div class="card-body">' +
            '<p><strong>Canonical event:</strong> ' + eventLink + '</p>' +
            '<p><strong>Lifecycle state:</strong> ' + lifecycleBadge(state) + '</p>' +
            '<p><strong>Retired:</strong> ' + escapeHtml(String(retiredCount)) + ' event(s)</p>' +
            warningsHtml +
            flagHtml +
            '<div class="btn-list mt-3">' +
            '<button class="btn btn-primary" data-action="done-consolidate">Done</button>' +
            reviewQueueBtn +
            '</div>' +
            '</div>' +
            '</div>';
    }

    // -------------------------------------------------------------------------
    // Event delegation
    // -------------------------------------------------------------------------

    function handleAction(e) {
        const target = e.target.closest('[data-action]');
        if (!target) return;

        const action = target.dataset.action;

        switch (action) {
            case 'add-ulid-input':
                e.preventDefault();
                addUlidRow();
                break;

            case 'remove-ulid-input':
                e.preventDefault();
                removeUlidRow(target);
                break;

            case 'load-events':
                e.preventDefault();
                loadEvents();
                break;

            case 'canonical-mode-change':
                handleCanonicalModeChange(target);
                break;

            case 'load-promote-event':
                e.preventDefault();
                handleLoadPromoteEvent(target);
                break;

            case 'edit-canonical':
                e.preventDefault();
                handleEditCanonical();
                break;

            case 'pick-field':
                e.preventDefault();
                handlePickField(target);
                break;

            case 'retire-toggle':
                handleRetireToggle(target);
                break;

            case 'submit-consolidate':
                e.preventDefault();
                handleSubmitConsolidate(target);
                break;

            case 'done-consolidate':
                e.preventDefault();
                window.location.href = '/admin/events';
                break;

            case 'goto-review-queue':
                e.preventDefault();
                window.location.href = '/admin/review-queue';
                break;

            default:
                break;
        }
    }

    // -------------------------------------------------------------------------
    // Init
    // -------------------------------------------------------------------------

    function init() {
        renderInitialUlidRows();
        applyUrlParams();
        document.addEventListener('click', handleAction);
        document.addEventListener('change', handleAction);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
