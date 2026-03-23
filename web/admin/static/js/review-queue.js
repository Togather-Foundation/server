/**
 * Review Queue Page JavaScript
 * Handles review queue listing, filtering, expand/collapse detail view, and approve/reject/fix actions
 */
(function() {
    'use strict';
    
    // Constants
    /** Default number of entries to fetch per page */
    const DEFAULT_PAGE_SIZE = 50;
    
    /** Number of columns in the review queue table */
    const TABLE_COLUMN_COUNT = 5;
    
    /** Time conversion constants */
    const MILLISECONDS_PER_MINUTE = 60000;
    const MINUTES_PER_HOUR = 60;
    const HOURS_PER_DAY = 24;
    
    /** Thresholds for relative time display (when to switch from minutes to hours to days) */
    const MINUTES_DISPLAY_THRESHOLD = 60;
    const HOURS_DISPLAY_THRESHOLD = 24;
    
    /** Date formatting constants */
    const DATE_PADDING_LENGTH = 2;
    const DATE_PADDING_CHAR = '0';
    const MONTH_INDEX_OFFSET = 1; // JavaScript months are 0-indexed, add 1 for human-readable
    
    // State management
    let entries = [];
    let currentFilter = 'pending';
    let expandedId = null;
    let pagination = null;
    let cursor = null;
    let currentEntryDetail = null; // Cached detail for the currently expanded entry

    /**
     * Per-entry field override map for the field picker.
     * Key: entryId (string), Value: { fieldKey: { value, source, edited } }
     * source: 'this' = canonical, 'related' = other event's value
     * edited: true = user modified inline after picking
     * Cleared when the fold-down is collapsed.
     */
    const fieldOverrides = {};

    /**
     * Per-entry occurrence picker state for multi-event consolidation.
     * Key: entryId (string), Value: array of pickerEntry objects
     * pickerEntry shape: { key, source, occurrence, occurrenceId, included, overlaps }
     * Cleared when the fold-down is collapsed.
     */
    const occurrencePicker = {};
    
    /** Debounce delay for primary event ULID lookup */
    const MERGE_LOOKUP_DEBOUNCE_MS = 400;
    /** ULID character length */
    const ULID_LENGTH = 26;
    /** Regex for valid ULID format */
    const ULID_PATTERN = /^[0-9A-Z]{26}$/i;
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    /**
     * Initialize the review queue page
     * Sets up event listeners, pagination component, and loads initial entries
     */
    function init() {
        setupEventListeners();
        setupPagination();
        loadEntries();
    }
    
    /**
     * Setup pagination component
     * Creates reusable Pagination instance for the review queue
     */
    function setupPagination() {
        pagination = new Pagination({
            container: document.getElementById('pagination'),
            limit: DEFAULT_PAGE_SIZE,
            mode: 'cursor',
            showingTextElement: document.getElementById('showing-text'),
            onPageChange: async (cursor, direction) => {
                await loadEntries(cursor);
            }
        });
    }
    
    /**
     * Setup event listeners using event delegation
     */
    function setupEventListeners() {
        // Event delegation for data-action buttons
        document.addEventListener('click', (e) => {
            const target = e.target.closest('[data-action]');
            if (!target) return;
            
            const action = target.dataset.action;
            const id = target.dataset.id;
            
            switch(action) {
                case 'filter-status':
                    e.preventDefault();
                    filterByStatus(target.dataset.status);
                    break;
                case 'navigate-to-event':
                    // Store review queue context for event detail page
                    sessionStorage.setItem('from_review_queue', target.dataset.reviewId);
                    // Prevent row expansion when clicking event link
                    e.stopPropagation();
                    // Allow default navigation to proceed
                    break;
                case 'expand-detail':
                    e.preventDefault();
                    e.stopPropagation();
                    // Toggle: if this entry is already expanded, collapse it
                    if (expandedId === id) {
                        collapseDetail();
                    } else {
                        expandDetail(id);
                    }
                    break;
                case 'collapse-detail':
                    e.preventDefault();
                    e.stopPropagation();
                    collapseDetail();
                    break;
                case 'approve':
                    e.preventDefault();
                    approve(id);
                    break;
                case 'reject':
                    e.preventDefault();
                    showRejectModal(id);
                    break;
                case 'show-fix-form':
                    e.preventDefault();
                    showFixForm(id);
                    break;
                case 'cancel-fix':
                    e.preventDefault();
                    hideFixForm();
                    break;
                case 'apply-fix':
                    e.preventDefault();
                    applyFix(id);
                    break;
                case 'not-a-duplicate':
                    e.preventDefault();
                    notADuplicate(id);
                    break;
                case 'show-more':
                    e.preventDefault();
                    showMoreText(target);
                    break;
                case 'confirm-reject':
                    confirmReject();
                    break;
                case 'confirm-merge':
                    confirmMerge();
                    break;
                case 'remove-occurrence':
                    e.preventDefault();
                    removeOccurrence(
                        target.dataset.entryId,
                        target.dataset.eventUlid,
                        target.dataset.occurrenceId
                    );
                    break;
                case 'add-occurrence':
                    e.preventDefault();
                    addOccurrence(
                        target.dataset.entryId,
                        target.dataset.eventUlid
                    );
                    break;
                case 'canonical-select':
                    // Rebuild both pickers when canonical selection changes
                    rebuildPickers(id);
                    break;
                case 'pick-field': {
                    e.preventDefault();
                    // Resolve entryId by walking up to the detail row (id="detail-{entryId}")
                    const detailTr = target.closest('tr[id]');
                    if (!detailTr) break;
                    const trId = detailTr.id; // "detail-{entryId}"
                    const pickedEntryId = trId.startsWith('detail-') ? trId.slice('detail-'.length) : null;
                    if (!pickedEntryId) break;

                    const pickedField = target.dataset.field;
                    const pickedSubfield = target.dataset.subfield || null;
                    const pickedValue = target.dataset.value !== undefined ? target.dataset.value : '';
                    const pickedSource = target.dataset.source || null;

                    if (!pickedField) break;

                    // Call handleFieldPick which handles the new onPick callback logic
                    handleFieldPick(pickedEntryId, pickedField, pickedSubfield, pickedValue, pickedSource);
                    break;
                }
                case 'consolidate':
                    e.preventDefault();
                    consolidate(id);
                    break;
                case 'toggle-occurrence':
                    e.preventDefault();
                    toggleOccurrence(target.dataset.entryId, target.dataset.occKey);
                    break;
            }
        });
        
        // Make table rows clickable to expand/collapse
        document.addEventListener('click', (e) => {
            // Check if we clicked inside a table row (but not on a link or button)
            const row = e.target.closest('tr[data-entry-id]');
            if (!row) return;
            
            // Don't trigger if clicking on a link, button, or element with data-action
            if (e.target.closest('a, button, [data-action]')) return;
            
            const entryId = row.dataset.entryId;
            if (!entryId) return;
            
            // Toggle expand/collapse
            if (expandedId === entryId) {
                collapseDetail();
            } else {
                expandDetail(entryId);
            }
        });

        // Handle change events for radio buttons (canonical selector)
        document.addEventListener('change', (e) => {
            const target = e.target;
            if (target.dataset.action === 'canonical-select' && target.dataset.entryId) {
                rebuildPickers(target.dataset.entryId);
            }
        });
    }
    
    /**
     * Filter entries by review status
     * @param {string} status - Status to filter by ('pending', 'approved', 'rejected', 'all')
     */
    function filterByStatus(status) {
        currentFilter = status;
        cursor = null;
        
        // Update active tab
        document.querySelectorAll('[data-action="filter-status"]').forEach(link => {
            if (link.dataset.status === status) {
                link.classList.add('active');
            } else {
                link.classList.remove('active');
            }
        });
        
        // Show/hide rejection reason column header
        const reasonHeader = document.getElementById('rejection-reason-header');
        if (reasonHeader) {
            reasonHeader.style.display = status === 'rejected' ? '' : 'none';
        }
        
        loadEntries();
    }
    
    /**
     * Load review queue entries from API
     * Fetches entries based on current filter and cursor, handles pagination
     * @async
     * @param {string|null} cursor - Optional cursor for pagination
     * @throws {Error} If API request fails
     */
    async function loadEntries(cursor = null) {
        showLoading();
        
        try {
            const params = {
                status: currentFilter,
                limit: DEFAULT_PAGE_SIZE
            };
            
            if (cursor) {
                params.cursor = cursor;
            }
            
            const data = await API.reviewQueue.list(params);
            
            // Handle different response formats
            if (data.items && Array.isArray(data.items)) {
                entries = data.items;
            } else if (Array.isArray(data)) {
                entries = data;
            } else {
                entries = [];
            }
            
            // Update badge counts with total from API
            if (data.total !== undefined) {
                updateBadgeCount(currentFilter, data.total);
            }
            
            if (entries.length === 0) {
                showEmptyState();
            } else {
                showTable();
                renderTable();
                // Update pagination component with response data
                if (pagination) {
                    pagination.update(data);
                }
            }
        } catch (err) {
            console.error('Failed to load review queue:', err);
            showToast(err.message || 'Failed to load review queue', 'error');
            showEmptyState();
        }
    }
    
    /**
     * Render entries into table rows
     * Creates HTML table rows for each entry with event name, start time, warnings, and actions
     */
    function renderTable() {
        const tbody = document.getElementById('review-queue-table');
        if (!tbody) return;
        
        tbody.innerHTML = entries.map(entry => {
            const eventName = entry.eventName || 'Untitled Event';
            const startTime = entry.eventStartTime ? formatDate(entry.eventStartTime, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }) : 'No date';
            const warningBadge = WarningBadges.getBadge(entry.warnings, entry.status);
            const createdAgo = getRelativeTime(entry.createdAt);
            
            // Build rejection reason cell (only for rejected events)
            let rejectionReasonCell = '';
            if (currentFilter === 'rejected' && entry.rejectionReason) {
                const reason = entry.rejectionReason;
                // Truncate long reasons with Show more button
                if (reason.length > 100) {
                    const truncated = reason.substring(0, 100) + '...';
                    const escapedFull = escapeHtml(reason).replace(/'/g, '&apos;');
                    rejectionReasonCell = `
                        <td>
                            <span class="rejection-reason-text">${escapeHtml(truncated)}</span>
                            <button class="btn btn-sm btn-link p-0" data-action="show-more" data-full-text="${escapedFull}">Show more</button>
                        </td>
                    `;
                } else {
                    rejectionReasonCell = `<td>${escapeHtml(reason)}</td>`;
                }
            } else if (currentFilter === 'rejected') {
                rejectionReasonCell = '<td class="text-muted">(no reason provided)</td>';
            }
            
            return `
                <tr data-entry-id="${escapeHtml(String(entry.id))}">
                    <td>
                        <a href="/admin/events/${entry.eventId}" class="text-reset" data-action="navigate-to-event" data-review-id="${entry.id}">
                            ${escapeHtml(eventName)}
                        </a>
                        ${entry.duplicateOfEventUlid ? '<span class="badge bg-purple-lt ms-1" title="Near-duplicate">dup</span>' : ''}
                    </td>
                    <td class="text-muted">${startTime}</td>
                    <td>${warningBadge}</td>
                    ${rejectionReasonCell}
                    <td class="text-muted">${createdAgo}</td>
                    <td>
                        <button class="btn btn-sm btn-ghost-primary expand-arrow" data-action="expand-detail" data-id="${entry.id}">
                            <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                <polyline points="6 9 12 15 18 9"/>
                            </svg>
                        </button>
                    </td>
                </tr>
            `;
        }).join('');
    }
    
    /**
     * Expand detail view for an entry
     * Fetches full entry details from API and displays in expandable row below entry
     * @async
     * @param {string} id - Review queue entry ID
     * @throws {Error} If API request fails
     */
    async function expandDetail(id) {
        // Collapse any currently expanded detail
        if (expandedId) {
            collapseDetail();
        }
        
        const entryRow = document.querySelector(`tr[data-entry-id="${id}"]`);
        if (!entryRow) return;
        
        expandedId = id;
        
        // Change arrow direction to up (collapsed → expanded)
        const arrowButton = entryRow.querySelector('.expand-arrow');
        if (arrowButton) {
            const arrowIcon = arrowButton.querySelector('polyline');
            if (arrowIcon) {
                arrowIcon.setAttribute('points', '6 15 12 9 18 15'); // Up arrow
            }
        }
        
        // Calculate colspan based on current filter (rejected tab has extra column)
        const colspan = currentFilter === 'rejected' ? TABLE_COLUMN_COUNT + 1 : TABLE_COLUMN_COUNT;
        
        // Show loading state in detail row
        const detailRow = document.createElement('tr');
        detailRow.id = `detail-${id}`;
        detailRow.innerHTML = `
            <td colspan="${colspan}" class="p-0">
                <div class="card mb-0">
                    <div class="card-body text-center py-5">
                        <div class="spinner-border text-primary" role="status">
                            <span class="visually-hidden">Loading...</span>
                        </div>
                        <p class="text-muted mt-3">Loading details...</p>
                    </div>
                </div>
            </td>
        `;
        entryRow.after(detailRow);
        
        // Fetch detail from API
        try {
            const detail = await API.reviewQueue.get(id);
            currentEntryDetail = detail;
            renderDetailCard(id, detail);
        } catch (err) {
            console.error('Failed to load detail:', err);
            showToast(err.message || 'Failed to load entry detail', 'error');
            detailRow.remove();
            expandedId = null;
        }
    }
    
    /**
     * Render detail card content
     * Displays warnings, changes, original vs normalized data comparison, and action buttons
     * @param {string} id - Review queue entry ID
     * @param {Object} detail - Entry detail object from API
     * @param {Array} detail.warnings - Array of warning objects
     * @param {Array} detail.changes - Array of change objects with field, original, corrected, reason
     * @param {Object} detail.original - Original event data
     * @param {Object} detail.normalized - Normalized event data
     * @param {string} detail.status - Review status ('pending', 'approved', 'rejected')
     */
    function renderDetailCard(id, detail) {
        const detailRow = document.getElementById(`detail-${id}`);
        if (!detailRow) return;
        
        const warnings = detail.warnings || [];
        const changes = detail.changes || [];
        const original = detail.original || {};
        const normalized = detail.normalized || {};

        const isCase1 = (!detail.relatedEvents || detail.relatedEvents.length === 0) &&
                        !detail.duplicateOfEventUlid;
        const isCase2or3 = !isCase1;
        
        // Check if there are any date-related warnings
        const hasDateWarnings = warnings.some(w => 
            w.code && (w.code.includes('date') || w.code.includes('time') || w.code.includes('reversed'))
        );
        
        // Build warnings HTML - simple list without redundant heading
        const warningsHtml = warnings.length > 0 ? `
            <div class="mb-3">
                ${warnings.map(w => {
                    const badge = WarningBadges.getDetailBadge(w.code);
                    const message = w.message || '(no message)';
                    let warningHtml = `<div class="mb-2">${badge} ${escapeHtml(message)}`;
                    
                    // Show match details for duplicate warnings
                    if (w.code === 'potential_duplicate' && w.details && w.details.matches && Array.isArray(w.details.matches) && w.details.matches.length > 0) {
                        warningHtml += `<div class="ms-4 mt-2">`;
                        w.details.matches.forEach(match => {
                            const similarity = match.similarity ? Math.round(match.similarity * 100) : 0;
                            const matchName = escapeHtml(match.name || 'Unknown');
                            const matchUlid = escapeHtml(match.ulid || '');
                            const matchHref = encodeURIComponent(match.ulid || '');
                            if (matchUlid) {
                                warningHtml += `
                                    <div class="mb-1">
                                        <a href="/admin/events/${matchHref}" class="text-reset">${matchName}</a>
                                        <span class="badge bg-purple-lt ms-1">${similarity}%</span>
                                    </div>
                                    <div id="dup-detail-${matchUlid}" data-dup-ulid="${matchUlid}" class="mt-2 mb-2">
                                        <div class="text-center text-muted small py-2">
                                            <div class="spinner-border spinner-border-sm" role="status">
                                                <span class="visually-hidden">Loading...</span>
                                            </div>
                                            Loading duplicate event details...
                                        </div>
                                    </div>
                                `;
                            } else {
                                warningHtml += `
                                    <div class="mb-1">
                                        <span>${matchName}</span>
                                        <span class="badge bg-purple-lt ms-1">${similarity}%</span>
                                    </div>
                                `;
                            }
                        });
                        warningHtml += `</div>`;
                    } else if (w.code === 'place_possible_duplicate' && w.details && w.details.matches && Array.isArray(w.details.matches) && w.details.matches.length > 0) {
                        // PlaceInput does not carry url/telephone/email — set to null explicitly
                        // so renderPlaceSummary doesn't show amber "missing" highlights on the left card.
                        const newPlaceData = {
                            name: w.details.new_place_name || '',
                            address_street: w.details.new_place_street || null,
                            address_locality: w.details.new_place_locality || null,
                            address_region: w.details.new_place_region || null,
                            postal_code: w.details.new_place_postal_code || null,
                            url: null,
                            telephone: null,
                            email: null,
                        };
                        warningHtml += `<div class="ms-4 mt-2">`;
                        w.details.matches.forEach(match => {
                            const similarity = match.similarity ? Math.round(match.similarity * 100) : 0;
                            const matchUlid = escapeHtml(match.ulid || '');
                            const matchName = escapeHtml(match.name || 'Unknown');
                            warningHtml += `<div class="mb-1"><a href="/admin/places/${encodeURIComponent(match.ulid || '')}" class="text-reset">${matchName}</a><span class="badge bg-purple-lt ms-1">${similarity}%</span></div>`;
                            warningHtml += `<div class="row g-2 mt-1 mb-2">
                                <div class="col-md-6"><div class="card bg-light"><div class="card-header py-1"><small class="text-muted fw-semibold">New place</small></div><div class="card-body py-2">${renderPlaceSummary(newPlaceData, match)}</div></div></div>
                                <div class="col-md-6"><div class="card bg-light"><div class="card-header py-1"><small class="text-muted fw-semibold">Existing place</small></div><div class="card-body py-2">${renderPlaceSummary(match, newPlaceData)}</div></div></div>
                            </div>`;
                        });
                        warningHtml += `</div>`;
                    } else if (w.code === 'org_possible_duplicate' && w.details && w.details.matches && Array.isArray(w.details.matches) && w.details.matches.length > 0) {
                        const newOrgData = {
                            name: w.details.new_org_name || '',
                            address_locality: w.details.new_org_locality || null,
                            address_region: w.details.new_org_region || null,
                            url: w.details.new_org_url || null,
                            email: w.details.new_org_email || null,
                            telephone: w.details.new_org_telephone || null,
                        };
                        warningHtml += `<div class="ms-4 mt-2">`;
                        w.details.matches.forEach(match => {
                            const similarity = match.similarity ? Math.round(match.similarity * 100) : 0;
                            const matchUlid = escapeHtml(match.ulid || '');
                            const matchName = escapeHtml(match.name || 'Unknown');
                            warningHtml += `<div class="mb-1"><a href="/admin/organizations/${encodeURIComponent(match.ulid || '')}" class="text-reset">${matchName}</a><span class="badge bg-purple-lt ms-1">${similarity}%</span></div>`;
                            warningHtml += `<div class="row g-2 mt-1 mb-2">
                                <div class="col-md-6"><div class="card bg-light"><div class="card-header py-1"><small class="text-muted fw-semibold">New org</small></div><div class="card-body py-2">${renderOrgSummary(newOrgData, match)}</div></div></div>
                                <div class="col-md-6"><div class="card bg-light"><div class="card-header py-1"><small class="text-muted fw-semibold">Existing org</small></div><div class="card-body py-2">${renderOrgSummary(match, newOrgData)}</div></div></div>
                            </div>`;
                        });
                        warningHtml += `</div>`;
                    } else if (w.code === 'near_duplicate_of_new_event') {
                        // The full side-by-side panel rendered below (Case 2/3) handles the
                        // diff using the enriched relatedEvents data. No inline card needed here.
                    }
                    
                    warningHtml += `</div>`;
                    return warningHtml;
                }).join('')}
            </div>
        ` : '';
        
        // Build changes HTML with visual emphasis
        const changesHtml = changes.length > 0 ? `
            <div class="card bg-light mb-3">
                <div class="card-header">
                    <h4 class="card-title mb-0">Automatic Corrections Applied</h4>
                </div>
                <div class="card-body">
                    ${changes.map(c => `
                        <div class="mb-3">
                            <div class="row align-items-center">
                                <div class="col">
                                    <strong class="text-muted">${escapeHtml(c.field)}</strong>
                                </div>
                            </div>
                            <div class="row mt-1">
                                <div class="col-md-6">
                                    <small class="text-muted d-block">Original:</small>
                                    <span class="badge bg-danger-lt">${escapeHtml(formatDateValue(c.original))}</span>
                                </div>
                                <div class="col-md-6">
                                    <small class="text-muted d-block">Corrected:</small>
                                    <span class="badge bg-success-lt">${escapeHtml(formatDateValue(c.corrected))}</span>
                                </div>
                            </div>
                            <small class="text-muted d-block mt-1">
                                <svg xmlns="http://www.w3.org/2000/svg" class="icon icon-sm" width="16" height="16" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                    <circle cx="12" cy="12" r="9"/>
                                    <path d="M12 8h.01"/>
                                    <path d="M11 12h1v4h1"/>
                                </svg>
                                ${escapeHtml(c.reason)}
                            </small>
                        </div>
                    `).join('')}
                </div>
            </div>
        ` : '';
        
        // Build event data section (always show normalized data)
        const eventDataHtml = `
            <div class="card mb-3">
                <div class="card-header">
                    <h4 class="card-title mb-0">Event Information</h4>
                    <small class="text-secondary">This is the data that will be published</small>
                </div>
                <div class="card-body">
                    ${renderFullEventData(normalized)}
                </div>
            </div>
        `;
        
        // Build comparison HTML with diff highlighting (only if there are changes)
        const comparisonHtml = changes.length > 0 ? `
            <div class="card mb-3">
                <div class="card-header">
                    <h4 class="card-title mb-0">Changes Made</h4>
                    <small class="text-muted">Comparison of original vs corrected data</small>
                </div>
                <div class="card-body">
                    <div class="row">
                        <div class="col-md-6">
                            <h5>Original Data</h5>
                            <small class="text-muted d-block mb-2">Data as received from source</small>
                            ${renderEventData(original, changes, 'original')}
                        </div>
                        <div class="col-md-6">
                            <h5>Normalized Data</h5>
                            <small class="text-muted d-block mb-2">Data after automatic corrections</small>
                            ${renderEventData(normalized, changes, 'normalized')}
                        </div>
                    </div>
                </div>
            </div>
        ` : '';
        
        // Build cross-link banner for pending items with a known duplicate event.
        // Only show the raw cross-link alert for Case 2/3 when there are NO embedded relatedEvents
        // (legacy rows). When relatedEvents is populated, the side-by-side panel replaces the alert.
        let crossLinkHtml = '';
        const relatedEvents = detail.relatedEvents || [];
        if (detail.status === 'pending' && detail.duplicateOfEventUlid && relatedEvents.length === 0) {
            const linkedUlid = detail.duplicateOfEventUlid;
            const isNearDupEntry = warnings.some(w => w.code === 'near_duplicate_of_new_event');
            if (isNearDupEntry) {
                crossLinkHtml = `
                    <div class="alert alert-warning mb-3">
                        <strong>Near-duplicate detected:</strong>
                        A newly-ingested event
                        <a href="/admin/events/${encodeURIComponent(linkedUlid)}" class="alert-link">${escapeHtml(linkedUlid)}</a>
                        may be a near-duplicate of this existing event.
                    </div>
                `;
            } else {
                crossLinkHtml = `
                    <div class="alert alert-warning mb-3">
                        <strong>Near-duplicate detected:</strong>
                        This event may be a duplicate of
                        <a href="/admin/events/${encodeURIComponent(linkedUlid)}" class="alert-link">${escapeHtml(linkedUlid)}</a>.
                    </div>
                `;
            }
        }

        // Build the occurrences section.
        // Case 1: editable (add/remove occurrences inline).
        // Case 2/3: occurrences are shown inside the side-by-side panel; this wrapper
        // is not mounted for Case 2/3 (the panel renders its own occurrence lists).
        const thisOccurrences = detail.occurrences || [];
        const occurrencesHtml = isCase1 ? `
            <div id="occurrence-list-${id}">
                ${OccurrenceRendering.renderList(thisOccurrences, detail.eventId, id, true)}
            </div>
        ` : '';

        // Build the Case 2/3 side-by-side related event panel.
        let relatedEventPanelHtml = '';
        // Build field picker section (Case 2/3 only, when relatedEvents present)
        let fieldPickerSectionHtml = '';
        // safeId is used in element IDs — always escape before use in HTML attributes
        const safeId = escapeHtml(String(id));
        if (isCase2or3 && relatedEvents.length > 0) {
            const relatedEvent = relatedEvents[0];
            const similarity = relatedEvent.similarity != null ? Math.round(relatedEvent.similarity * 100) : null;
            const similarityBadge = similarity != null
                ? `<span class="badge bg-purple-lt ms-2">${similarity}%</span>`
                : '';

            // Map relatedEventDetail to the shape extractMergeFields expects.
            // NOTE: relatedEvent.name and relatedEvent.description come from the API (unescaped).
            // They flow through renderMergeEventSummary → extractMergeFields → renderMergeFieldRows
            // → renderMergeField where escapeHtml() is applied at render time. The truncation in
            // renderMergeFieldRows happens BEFORE escapeHtml, which is safe — do not re-escape
            // here or you will double-encode. Maintainers: if you add new display paths for these
            // fields, ensure escapeHtml() is called before innerHTML assignment.
            const relatedAsNormalized = {
                name: relatedEvent.name,
                description: relatedEvent.description,
                startDate: relatedEvent.occurrences && relatedEvent.occurrences.length > 0
                    ? relatedEvent.occurrences[0].startTime
                    : null,
                location: relatedEvent.venueName ? { name: relatedEvent.venueName } : null,
            };

            relatedEventPanelHtml = `
                <div class="row g-3 mb-3">
                    <div class="col-md-6">
                        <div class="card">
                            <div class="card-header">
                                <strong>This event</strong>
                            </div>
                            <div class="card-body">
                                ${renderMergeEventSummary(normalized, relatedAsNormalized)}
                                ${OccurrenceRendering.renderList(thisOccurrences, detail.eventId, id, false)}
                            </div>
                        </div>
                    </div>
                    <div class="col-md-6">
                        <div class="card">
                            <div class="card-header">
                                <strong>${escapeHtml(relatedEvent.name || 'Related event')}</strong>
                                ${similarityBadge}
                            </div>
                            <div class="card-body">
                                ${renderMergeEventSummary(relatedAsNormalized, normalized)}
                                ${OccurrenceRendering.renderList(relatedEvent.occurrences || [], relatedEvent.ulid, id, false)}
                            </div>
                        </div>
                    </div>
                </div>
            `;

            // Field picker section — shown between side-by-side cards and action buttons
            fieldPickerSectionHtml = `
                <div class="card mt-3">
                    <div class="card-header">
                        <h4 class="card-title mb-0">Choose canonical field values</h4>
                        <small class="text-muted">Click a chip to select that value for the merged event</small>
                    </div>
                    <div class="card-body p-0">
                        <div id="field-picker-table-${safeId}"></div>
                    </div>
                </div>
                <div class="card mt-3">
                    <div class="card-header">
                        <h4 class="card-title mb-0">Which occurrences to include?</h4>
                        <small class="text-muted">Select related occurrences to add to the canonical event</small>
                    </div>
                    <div class="card-body p-0">
                        <div id="occurrence-picker-${safeId}"></div>
                    </div>
                </div>
            `;
        }

        // Build action buttons (only for pending status)
        const actionButtons = detail.status === 'pending' ? (() => {
            if (isCase2or3) {
                // Case 2/3: show canonical selector + consolidate action buttons
                return `
                    <div class="mb-3" id="canonical-selector-${id}">
                        <label class="form-label fw-semibold">Which event is canonical?</label>
                        <div class="form-check">
                            <input class="form-check-input" type="radio" name="canonical-${id}"
                                   id="canonical-this-${escapeHtml(String(id))}" value="this" checked
                                   data-action="canonical-select" data-entry-id="${escapeHtml(String(id))}">
                            <label class="form-check-label" for="canonical-this-${escapeHtml(String(id))}">This event</label>
                        </div>
                        <div class="form-check">
                            <input class="form-check-input" type="radio" name="canonical-${id}"
                                   id="canonical-related-${escapeHtml(String(id))}" value="related"
                                   data-action="canonical-select" data-entry-id="${escapeHtml(String(id))}">
                            <label class="form-check-label" for="canonical-related-${escapeHtml(String(id))}">Related event</label>
                        </div>
                    </div>
                    <div class="btn-list" id="action-buttons-${id}">
                        <button class="btn btn-success" data-action="consolidate" data-id="${id}">
                            Consolidate
                        </button>
                        <button class="btn btn-outline-secondary" data-action="not-a-duplicate" data-id="${id}">
                            &#8800; Not a Duplicate
                        </button>
                        <button class="btn btn-outline-danger" data-action="reject" data-id="${id}">
                            &#10005; Reject
                        </button>
                    </div>
                    <div id="consolidate-error-${id}" class="alert alert-danger mt-2" style="display:none;"></div>
                    <div id="fix-form-${id}" style="display: none;">
                        <!-- Fix form will be inserted here -->
                    </div>
                `;
            } else {
                // Case 1: standalone — approve, fix dates, reject only
                return `
                    <div class="btn-list" id="action-buttons-${id}">
                        <button class="btn btn-success" data-action="approve" data-id="${id}">
                            <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                <path d="M5 12l5 5l10 -10"/>
                            </svg>
                            Approve
                        </button>
                        ${hasDateWarnings ? `
                            <button class="btn btn-primary" data-action="show-fix-form" data-id="${id}">
                                <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                    <rect x="4" y="5" width="16" height="16" rx="2"/>
                                    <line x1="16" y1="3" x2="16" y2="7"/>
                                    <line x1="8" y1="3" x2="8" y2="7"/>
                                    <line x1="4" y1="11" x2="20" y2="11"/>
                                </svg>
                                Fix Dates
                            </button>
                        ` : ''}
                        <button class="btn btn-outline-danger" data-action="reject" data-id="${id}">
                            <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                <line x1="18" y1="6" x2="6" y2="18"/>
                                <line x1="6" y1="6" x2="18" y2="18"/>
                            </svg>
                            Delete Event
                        </button>
                    </div>
                    <div id="fix-form-${id}" style="display: none;">
                        <!-- Fix form will be inserted here -->
                    </div>
                `;
            }
        })() : `
            <div class="text-muted">
                ${detail.status === 'merged' ? 'Merged' : detail.status === 'approved' ? 'Approved' : detail.status === 'dismissed' ? 'Dismissed' : 'Rejected'} by ${escapeHtml(detail.reviewedBy || 'system')} on ${formatDate(detail.reviewedAt)}
                ${detail.duplicateOfEventUlid ? `<br>Merged into: <a href="/admin/events/${encodeURIComponent(detail.duplicateOfEventUlid)}" class="text-reset">${escapeHtml(detail.duplicateOfEventUlid)}</a>` : ''}
                ${detail.reviewNotes ? `<br>Notes: ${escapeHtml(detail.reviewNotes)}` : ''}
                ${detail.rejectionReason ? `<br>Reason: ${escapeHtml(detail.rejectionReason)}` : ''}
            </div>
        `;
        
        // Calculate colspan based on current filter (rejected tab has extra column)
        const colspan = currentFilter === 'rejected' ? TABLE_COLUMN_COUNT + 1 : TABLE_COLUMN_COUNT;
        
        detailRow.innerHTML = `
            <td colspan="${colspan}" class="p-0">
                <div class="card mb-0">
                    <div class="card-body">
                        ${crossLinkHtml}
                        ${warningsHtml}
                        ${changesHtml}
                        ${isCase2or3 ? relatedEventPanelHtml + fieldPickerSectionHtml : eventDataHtml + occurrencesHtml}
                        ${comparisonHtml}
                        
                        <div class="mt-3">
                            ${actionButtons}
                        </div>
                    </div>
                </div>
            </td>
        `;

        // If Case 2/3 with related events, initialise the field picker table now that the DOM exists.
        if (isCase2or3 && relatedEvents.length > 0 && typeof window.FieldPicker !== 'undefined') {
            const pickerContainer = document.getElementById('field-picker-table-' + safeId);
            if (pickerContainer) {
                const relatedEvent = relatedEvents[0];
                // Build the relatedEvent in the same shape as normalized for the picker.
                // This mirrors the relatedAsNormalized shape built above. See the NOTE on
                // relatedAsNormalized above regarding escaping of name/description fields.
                const relatedForPicker = {
                    name: relatedEvent.name,
                    description: relatedEvent.description,
                    startDate: relatedEvent.occurrences && relatedEvent.occurrences.length > 0
                        ? relatedEvent.occurrences[0].startTime
                        : null,
                    location: relatedEvent.venueName
                        ? { name: relatedEvent.venueName }
                        : (relatedEvent.location || null),
                    organizer: relatedEvent.organizer || null,
                    url: relatedEvent.url || null,
                    image: relatedEvent.image || null,
                    endDate: relatedEvent.occurrences && relatedEvent.occurrences.length > 0
                        ? relatedEvent.occurrences[0].endTime
                        : null,
                };

                // Determine canonical based on radio selection (default to 'this' which is index 0)
                const canonicalInput = document.querySelector(`input[name="canonical-${id}"]:checked`);
                const canonicalValue = canonicalInput ? canonicalInput.value : 'this';
                const canonicalIndex = canonicalValue === 'this' ? 0 : 1;

                // Build occurrence picker entries
                // Columns are fixed: left = this event, right = related event
                // canonicalIndex just determines which side is locked/auto-selected
                occurrencePicker[id] = buildOccurrencePicker(thisOccurrences, relatedEvent.occurrences || [], canonicalIndex);

                window.FieldPicker.renderFieldPickerTable(pickerContainer, [normalized, relatedForPicker], {
                    canonicalIndex: canonicalIndex,
                    readOnlyFields: new Set(['location.name', 'organizer.name']),
                    onPick: (fieldKey, subfieldKey, value, source) => handleFieldPick(id, fieldKey, subfieldKey, value, source)
                });

                // Render occurrence picker
                renderOccurrencePicker(id);
            }
        }

        // Fetch and render inline duplicate diffs for each potential_duplicate match
        // that is NOT already covered by detail.relatedEvents (legacy fast path).
        const relatedEventUlids = new Set(relatedEvents.map(r => r.ulid));
        const dupContainers = detailRow.querySelectorAll('[data-dup-ulid]');
        if (dupContainers.length > 0) {
            const thisEventData = detail.normalized || null;

            // Build a lookup map from ulid → match object for embedded fast-path rendering
            const matchDataByUlid = {};
            const dupWarnings = (detail.warnings || []).filter(w => w.code === 'potential_duplicate');
            dupWarnings.forEach(w => {
                if (w.details && Array.isArray(w.details.matches)) {
                    w.details.matches.forEach(m => {
                        if (m.ulid) matchDataByUlid[m.ulid] = m;
                    });
                }
            });

            dupContainers.forEach(container => {
                const dupUlid = container.dataset.dupUlid;
                if (!dupUlid) return;
                // Skip ULIDs already shown in the side-by-side relatedEvents panel
                if (relatedEventUlids.has(dupUlid)) {
                    container.style.display = 'none';
                    return;
                }
                fetchAndRenderInlineDuplicate(container, dupUlid, thisEventData, matchDataByUlid[dupUlid] || null);
            });
        }
    }

    /**
     * Remove an occurrence from an event.
     * Shows a spinner on the button, calls the delete API, then re-renders the occurrence list.
     * @async
     * @param {string|number} entryId - Review queue entry ID
     * @param {string} eventUlid - ULID of the event
     * @param {string} occurrenceId - UUID of the occurrence to delete
     */
    async function removeOccurrence(entryId, eventUlid, occurrenceId) {
        if (String(occurrenceId).startsWith('_pending_')) {
            showToast('Cannot remove occurrence that hasn\'t been saved yet', 'error');
            return;
        }

        const btn = document.querySelector(
            `[data-action="remove-occurrence"][data-entry-id="${escapeHtml(String(entryId))}"][data-occurrence-id="${escapeHtml(String(occurrenceId))}"]`
        );
        if (btn) setLoading(btn, true);

        try {
            await API.events.occurrences.delete(eventUlid, occurrenceId);
            if (currentEntryDetail) {
                currentEntryDetail.occurrences = (currentEntryDetail.occurrences || []).filter(o => o.id !== occurrenceId);
                OccurrenceRendering.refreshList(entryId, eventUlid, currentEntryDetail.occurrences, true);
            } else {
                const detail = await API.reviewQueue.get(entryId);
                currentEntryDetail = detail;
                renderDetailCard(entryId, detail);
            }
            if (btn) setLoading(btn, false);
        } catch (err) {
            console.error('Failed to remove occurrence:', err);
            const msg = (err && err.detail) || (err && err.message) || 'Failed to remove occurrence';
            showToast(msg, 'error');
            if (btn) setLoading(btn, false);
        }
    }

    /**
     * Add a new occurrence to an event.
     * Reads the occ-start-{entryId}, occ-end-{entryId}, occ-tz-{entryId} inputs,
     * validates, calls the API, and refreshes the occurrence list on success.
     * @async
     * @param {string|number} entryId - Review queue entry ID
     * @param {string} eventUlid - ULID of the event
     */
    async function addOccurrence(entryId, eventUlid) {
        const startInput = document.getElementById(`occ-start-${entryId}`);
        const endInput = document.getElementById(`occ-end-${entryId}`);
        const tzInput = document.getElementById(`occ-tz-${entryId}`);
        const errorDiv = document.getElementById(`occ-error-${entryId}`);
        const addBtn = document.querySelector(`[data-action="add-occurrence"][data-entry-id="${entryId}"]`);

        if (!startInput) return;

        const hideError = () => { if (errorDiv) errorDiv.style.display = 'none'; };
        const showError = (msg) => {
            if (errorDiv) {
                errorDiv.textContent = msg;
                errorDiv.style.display = 'block';
            }
        };

        hideError();

        const startVal = startInput.value;
        if (!startVal) {
            showError('Start date/time is required');
            return;
        }

        const timezone = tzInput ? tzInput.value.trim() : 'America/Toronto';
        if (!timezone) {
            showError('Timezone is required');
            return;
        }

        if (addBtn) setLoading(addBtn, true);

        try {
            // datetime-local inputs yield a timezone-free string like "2026-03-20T19:30".
            // Treat the string as UTC by appending 'Z', avoiding browser local-TZ conversion.
            // The timezone field carries the event's original timezone as metadata.
            const toUTC = (localStr) => localStr ? localStr + 'Z' : '';
            const body = {
                start_time: toUTC(startVal),
                timezone: timezone,
            };
            const endVal = endInput ? endInput.value : '';
            if (endVal) {
                body.end_time = toUTC(endVal);
            }

            const created = await API.events.occurrences.create(eventUlid, body);

            // Update cached detail — append the returned occurrence (or refetch)
            if (currentEntryDetail) {
                if (created && created.id) {
                    currentEntryDetail.occurrences = [...(currentEntryDetail.occurrences || []), created];
                } else {
                    // API may return 201 with no body; add a minimal placeholder so list updates
                    currentEntryDetail.occurrences = [...(currentEntryDetail.occurrences || []), {
                        id: '_pending_' + Date.now(),
                        startTime: body.start_time,
                        endTime: body.end_time || null,
                        timezone: body.timezone,
                    }];
                }
                OccurrenceRendering.refreshList(entryId, eventUlid, currentEntryDetail.occurrences, true);
            }

            // Clear form inputs
            if (startInput) startInput.value = '';
            if (endInput) endInput.value = '';

        } catch (err) {
            console.error('Failed to add occurrence:', err);
            const status = err && err.status;
            if (status === 409) {
                showError('Overlap: an occurrence already exists at this time');
            } else {
                const msg = (err && err.detail) || (err && err.message) || 'Failed to add occurrence';
                showError(msg);
            }
        } finally {
            if (addBtn) setLoading(addBtn, false);
        }
    }

    /**
     * Render full event data for display
     * Shows all event fields with proper formatting, including pretty-printed JSON for location
     * @param {Object} data - Event data object
     * @returns {string} HTML string of formatted event fields
     */
    function renderFullEventData(data) {
        if (!data) return '<p class="text-muted">No data available</p>';
        
        const fields = [
            { label: 'Event Name', key: 'name' },
            { label: 'Start Date', key: 'startDate', isDate: true },
            { label: 'End Date', key: 'endDate', isDate: true },
            { label: 'Description', key: 'description' },
            { label: 'Location', key: 'location', isJSON: true },
            { label: 'Organizer', key: 'organizer', isJSON: true },
            { label: 'Image URL', key: 'image' },
            { label: 'URL', key: 'url' },
            { label: 'Offers', key: 'offers', isJSON: true },
            { label: 'Event Status', key: 'eventStatus' },
            { label: 'Event Attendance Mode', key: 'eventAttendanceMode' }
        ];
        
        return fields.map(field => {
            let value = data[field.key];
            if (!value) return '';
            
            // Format based on field type
            if (field.isJSON && typeof value === 'object') {
                // Render JSON as formatted HTML for better readability
                return `
                    <div class="mb-3">
                        <strong>${escapeHtml(field.label)}:</strong>
                        ${renderJSONAsHTML(value)}
                    </div>
                `;
            } else if (field.isDate) {
                value = formatDateValue(value);
            } else if (typeof value === 'string' && value.length > 200) {
                // Truncate long text with expand option
                const truncated = value.substring(0, 200) + '...';
                const escapedTruncated = escapeHtml(truncated);
                const linkedTruncated = linkifyUrls(escapedTruncated);
                const escapedFull = escapeHtml(value).replace(/'/g, '&apos;');
                const linkedFull = linkifyUrls(escapedFull).replace(/"/g, '&quot;');
                return `
                    <div class="mb-2">
                        <strong>${escapeHtml(field.label)}:</strong><br>
                        <span class="description-text">${linkedTruncated}</span>
                        <button class="btn btn-sm btn-link p-0" data-action="show-more" data-full-text="${linkedFull}">Show more</button>
                    </div>
                `;
            }
            
            // Apply linkification for text fields (escape first, then linkify)
            const escapedValue = escapeHtml(String(value));
            const displayValue = linkifyUrls(escapedValue);
            
            return `
                <div class="mb-2">
                    <strong>${escapeHtml(field.label)}:</strong><br>
                    <span>${displayValue}</span>
                </div>
            `;
        }).filter(html => html).join('');
    }
    
    /**
     * Render JSON object as formatted HTML
     * Converts JSON objects into readable HTML with definition lists for nested objects
     * @param {Object|Array} data - JSON data to render
     * @param {number} depth - Current nesting depth (for limiting recursion)
     * @returns {string} HTML string representation of JSON
     */
    function renderJSONAsHTML(data, depth = 0) {
        if (depth > 3) {
            // Too deep, fall back to JSON string
            return `<pre class="border rounded p-2 mt-1 text-body" style="background-color: var(--tblr-bg-surface);"><code>${escapeHtml(JSON.stringify(data, null, 2))}</code></pre>`;
        }
        
        if (Array.isArray(data)) {
            if (data.length === 0) return '<span class="text-muted">[]</span>';
            return `
                <ul class="list-unstyled ms-3 mt-1">
                    ${data.map(item => {
                        if (typeof item === 'object') {
                            return `<li>${renderJSONAsHTML(item, depth + 1)}</li>`;
                        } else {
                            const escapedItem = escapeHtml(String(item));
                            const linkedItem = linkifyUrls(escapedItem);
                            return `<li>${linkedItem}</li>`;
                        }
                    }).join('')}
                </ul>
            `;
        }
        
        if (typeof data === 'object' && data !== null) {
            const entries = Object.entries(data);
            if (entries.length === 0) return '<span class="text-muted">{}</span>';
            
            return `
                <dl style="display: grid; grid-template-columns: 150px 1fr; gap: 0.5rem 1rem; margin-left: 0.5rem; margin-top: 0.25rem; margin-bottom: 0; font-size: 0.95em;">
                    ${entries.map(([key, value]) => {
                        let renderedValue;
                        if (typeof value === 'object' && value !== null) {
                            renderedValue = renderJSONAsHTML(value, depth + 1);
                        } else if (value === null) {
                            renderedValue = '<span class="text-muted">null</span>';
                        } else if (typeof value === 'boolean') {
                            renderedValue = `<span class="badge bg-${value ? 'success' : 'secondary'}-lt">${value}</span>`;
                        } else {
                            const escapedValue = escapeHtml(String(value));
                            renderedValue = linkifyUrls(escapedValue);
                        }
                        
                        return `
                            <dt class="text-muted text-truncate" style="max-width: 150px;" title="${escapeHtml(key)}">${escapeHtml(key)}</dt>
                            <dd style="margin: 0;">${renderedValue}</dd>
                        `;
                    }).join('')}
                </dl>
            `;
        }
        
        // Linkify any remaining string values (at max depth)
        const escapedData = escapeHtml(String(data));
        return linkifyUrls(escapedData);
    }
    
    /**
     * Render event data fields for comparison view
     * @param {Object} data - Event data object containing name, startDate, endDate, location
     * @param {Array} changes - Array of change objects to highlight differences
     * @param {string} type - Type of data ('original' or 'normalized') for highlighting
     * @returns {string} HTML string of formatted event fields
     */
    function renderEventData(data, changes, type) {
        const fields = [
            { label: 'Name', key: 'name' },
            { label: 'Start Date', key: 'startDate' },
            { label: 'End Date', key: 'endDate' },
            { label: 'Location', key: 'location' }
        ];
        
        return fields.map(field => {
            let value = data[field.key];
            if (!value) return '';
            
            let isJSON = false;
            if (typeof value === 'object') {
                isJSON = true;
                value = JSON.stringify(value, null, 2);
            } else if (field.key.includes('Date')) {
                value = formatDateValue(value);
            }
            
            // Check if this field changed
            const changed = changes.find(c => c.field === field.key);
            let highlightClass = '';
            let changeIndicator = '';
            
            if (changed) {
                if (type === 'original') {
                    highlightClass = 'bg-danger-lt text-danger';
                    changeIndicator = `
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon icon-sm ms-1" width="16" height="16" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <circle cx="12" cy="12" r="9"/>
                            <line x1="9" y1="12" x2="15" y2="12"/>
                        </svg>
                    `;
                } else {
                    highlightClass = 'bg-success-lt text-success';
                    changeIndicator = `
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon icon-sm ms-1" width="16" height="16" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <circle cx="12" cy="12" r="9"/>
                            <path d="M12 7v6l3 3"/>
                        </svg>
                    `;
                }
            }
            
            // Use <pre> for JSON to preserve formatting with good contrast
            if (isJSON) {
                return `
                    <div class="mb-2 ${changed ? 'p-2 rounded' : ''}">
                        <strong class="${changed ? highlightClass : ''}">${escapeHtml(field.label)}:${changeIndicator}</strong>
                        <pre class="border rounded p-2 mt-1 text-body ${highlightClass}" style="max-height: 200px; overflow-y: auto; background-color: var(--tblr-bg-surface);"><code>${escapeHtml(value)}</code></pre>
                    </div>
                `;
            }
            
            return `
                <div class="mb-2 ${changed ? 'p-2 rounded' : ''}">
                    <strong class="${changed ? highlightClass : ''}">${escapeHtml(field.label)}:${changeIndicator}</strong><br>
                    <span class="${highlightClass}">${escapeHtml(value)}</span>
                </div>
            `;
        }).join('');
    }
    
    /**
     * Update the field-overrides display panel for a given entryId.
     * Shows a summary of selected field values the admin has picked.
     * @param {string} entryId - Review queue entry ID
     */
    function updateOverridesDisplay(entryId) {
        const overridesCard = document.getElementById('field-overrides-' + entryId);
        const overridesList = document.getElementById('field-overrides-list-' + entryId);
        if (!overridesList) return;

        const overrides = fieldOverrides[entryId] || {};
        const keys = Object.keys(overrides);

        if (keys.length === 0) {
            if (overridesCard) overridesCard.style.display = 'none';
            overridesList.innerHTML = '';
            return;
        }

        if (overridesCard) overridesCard.style.display = '';

        const rows = keys.map(k => {
            const val = String(overrides[k]);
            const display = val.length > 80 ? val.substring(0, 77) + '…' : val;
            return '<div class="mb-1">' +
                '<small class="text-muted fw-semibold">' + escapeHtml(k) + ':</small> ' +
                '<span>' + escapeHtml(display) + '</span>' +
                '</div>';
        });
        overridesList.innerHTML = rows.join('');
    }

    /**
     * Build occurrence picker entries by merging this event and related event occurrences.
     * @param {Array} thisEventOccs - Array of occurrences for "this event" (left column, never changes)
     * @param {Array} relatedEventOccs - Array of occurrences for "related event" (right column, never changes)
     * @param {number} canonicalIndex - Which event is canonical: 0 = this event, 1 = related event
     * @returns {Array} Sorted array of pickerEntry objects
     */
    function buildOccurrencePicker(thisEventOccs, relatedEventOccs, canonicalIndex) {
        const entries = [];
        const canonicalIsThis = canonicalIndex === 0;

        // Add "this event" occurrences (left column, never swaps)
        (thisEventOccs || []).forEach((occ, idx) => {
            entries.push({
                key: 'this-' + idx,
                source: 'this',
                eventIndex: 0,  // Absolute index - left column always = 0
                occurrence: {
                    startTime: occ.startTime,
                    endTime: occ.endTime,
                    timezone: occ.timezone,
                    venueName: occ.venueName || null,
                    venueUlid: occ.venueUlid || null,
                },
                occurrenceId: occ.id || 'this-' + idx,
                isCanonical: canonicalIsThis,  // This side is canonical if canonicalIndex === 0
                included: true,  // Canonical side always included (locked)
                overlaps: false,
            });
        });

        // Add "related event" occurrences (right column, never swaps)
        (relatedEventOccs || []).forEach((occ, idx) => {
            const occData = {
                startTime: occ.startTime,
                endTime: occ.endTime,
                timezone: occ.timezone,
                venueName: occ.venueName || null,
                venueUlid: occ.venueUlid || null,
            };

            // Check if this related occurrence overlaps any "this event" occurrence
            let overlaps = false;
            (thisEventOccs || []).forEach(thisOcc => {
                if (OccurrenceRendering.occurrencesOverlap(occData, {
                    startTime: thisOcc.startTime,
                    endTime: thisOcc.endTime,
                })) {
                    overlaps = true;
                }
            });

            entries.push({
                key: 'related-' + idx,
                source: 'related',
                eventIndex: 1,  // Absolute index - right column always = 1
                occurrence: occData,
                occurrenceId: occ.id || 'related-' + idx,
                isCanonical: !canonicalIsThis,  // Related side is canonical if canonicalIndex === 1
                included: false,  // Default: not included unless user toggles
                overlaps: overlaps,
            });
        });

        // Sort by start time
        entries.sort((a, b) => {
            const aTime = a.occurrence.startTime ? new Date(a.occurrence.startTime).getTime() : 0;
            const bTime = b.occurrence.startTime ? new Date(b.occurrence.startTime).getTime() : 0;
            return aTime - bTime;
        });

        return entries;
    }

    /**
     * Render the occurrence picker for a given entry ID.
     * @param {string|number} entryId - Review queue entry ID
     */
    function renderOccurrencePicker(entryId) {
        const container = document.getElementById('occurrence-picker-' + entryId);
        if (!container) return;

        const entries = occurrencePicker[entryId] || [];
        if (typeof window.OccurrenceRendering !== 'undefined') {
            container.innerHTML = window.OccurrenceRendering.renderMergedPickerList(entries, entryId);
        } else {
            container.innerHTML = '<p class="text-muted">OccurrenceRendering not loaded</p>';
        }
    }

    /**
     * Handle field pick from the field picker onPick callback.
     * Updates the fieldOverrides store with the picked value, source, and marks as user-edited.
     * @param {string|number} entryId - Review queue entry ID
     * @param {string} fieldKey - Field key (e.g., 'name', 'description')
     * @param {string|null} subfieldKey - Subfield key for nested fields (e.g., 'name' for location.name)
     * @param {string} value - Picked value
     * @param {string} source - Source of value ('this' = canonical, 'related' = other event)
     */
    function handleFieldPick(entryId, fieldKey, subfieldKey, value, source) {
        const overrideKey = subfieldKey ? fieldKey + '.' + subfieldKey : fieldKey;

        if (!fieldOverrides[entryId]) {
            fieldOverrides[entryId] = {};
        }

        // Store the absolute event index (0 or 1) regardless of which is canonical.
        // This way the selection persists when canonical switches.
        const eventIndex = source === 'this' ? 0 : 1;
        fieldOverrides[entryId][overrideKey] = {
            value: value,
            eventIndex: eventIndex,  // Absolute index - doesn't change when canonical switches
            edited: true,
        };
    }

    /**
     * Toggle an occurrence's included status in the occurrence picker.
     * @param {string|number} entryId - Review queue entry ID
     * @param {string} occKey - The key of the occurrence to toggle
     */
    function toggleOccurrence(entryId, occKey) {
        const entries = occurrencePicker[entryId];
        if (!entries) return;

        const entry = entries.find(e => e.key === occKey);
        if (entry && entry.source === 'related' && !entry.overlaps) {
            entry.included = !entry.included;
            renderOccurrencePicker(entryId);
        }
    }

    /**
     * Rebuild both field picker and occurrence picker when canonical selection changes.
     * Preserves user-edited field overrides and user-toggled occurrence selections.
     * @param {string|number} entryId - Review queue entry ID
     */
    function rebuildPickers(entryId) {
        const detail = currentEntryDetail;
        if (!detail) return;

        const relatedEvents = detail.relatedEvents || [];
        if (relatedEvents.length === 0) return;

        const safeId = escapeHtml(String(entryId));
        const canonicalInput = document.querySelector(`input[name="canonical-${entryId}"]:checked`);
        const canonicalValue = canonicalInput ? canonicalInput.value : 'this';
        const canonicalIndex = canonicalValue === 'this' ? 0 : 1;

        const relatedEvent = relatedEvents[0];
        const normalized = detail.normalized || {};
        const thisOccurrences = detail.occurrences || [];

        // Build the related event object for field picker
        const relatedForPicker = {
            name: relatedEvent.name,
            description: relatedEvent.description,
            startDate: relatedEvent.occurrences && relatedEvent.occurrences.length > 0
                ? relatedEvent.occurrences[0].startTime
                : null,
            location: relatedEvent.venueName
                ? { name: relatedEvent.venueName }
                : (relatedEvent.location || null),
            organizer: relatedEvent.organizer || null,
            url: relatedEvent.url || null,
            image: relatedEvent.image || null,
            endDate: relatedEvent.occurrences && relatedEvent.occurrences.length > 0
                ? relatedEvent.occurrences[0].endTime
                : null,
        };

        // Preserve user-edited field overrides (edited === true)
        // Use absolute eventIndex - doesn't change when canonical switches
        const oldOverrides = fieldOverrides[entryId] || {};
        const preservedOverrides = {};
        const selectedOverrides = {};
        Object.entries(oldOverrides).forEach(([key, override]) => {
            if (override.edited === true && override.eventIndex !== undefined) {
                preservedOverrides[key] = override;
                // Use the absolute eventIndex - this is what the user selected regardless of canonical
                selectedOverrides[key] = override.eventIndex;
            }
        });

        // Rebuild field picker (field-picker.js handles canonical highlighting automatically)
        const fieldPickerContainer = document.getElementById('field-picker-table-' + safeId);
        if (fieldPickerContainer && typeof window.FieldPicker !== 'undefined') {
            window.FieldPicker.renderFieldPickerTable(fieldPickerContainer, [normalized, relatedForPicker], {
                canonicalIndex: canonicalIndex,
                readOnlyFields: new Set(['location.name', 'organizer.name']),
                selectedOverrides: selectedOverrides,
                onPick: (fieldKey, subfieldKey, value, source) => handleFieldPick(entryId, fieldKey, subfieldKey, value, source)
            });
        }

        // Rebuild occurrence picker
        // IMPORTANT: Don't swap columns! Left column = entry's own event, right column = related event.
        // Only the "canonical" designation changes - which one is locked/auto-selected.
        const thisEventOccs = thisOccurrences;      // "This event" - always stays left column
        const relatedEventOccs = relatedEvent.occurrences || [];  // "Related event" - always stays right column
        occurrencePicker[entryId] = buildOccurrencePicker(thisEventOccs, relatedEventOccs, canonicalIndex);

        // Preserve user-toggled related occurrence selections (matched by eventIndex + startTime)
        const oldPicker = occurrencePicker[entryId] || [];
        oldPicker.forEach(oldEntry => {
            if (oldEntry.included === true && oldEntry.occurrence.startTime) {
                // Find matching occurrence in new picker by absolute eventIndex + startTime
                const newEntry = (occurrencePicker[entryId] || []).find(e =>
                    e.eventIndex === oldEntry.eventIndex && e.occurrence.startTime === oldEntry.occurrence.startTime
                );
                if (newEntry) {
                    newEntry.included = true;
                }
            }
        });

        // Clear fieldOverrides and restore preserved user edits
        fieldOverrides[entryId] = preservedOverrides;

        renderOccurrencePicker(entryId);
    }

    /**
     * Consolidate the review entry's event with its related event.
     * Executes the three-step sequence: patch fields, add occurrences, consolidate.
     * @async
     * @param {string|number} entryId - Review queue entry ID
     */
    async function consolidate(entryId) {
        const detail = currentEntryDetail;
        if (!detail) return;

        const relatedEvents = detail.relatedEvents || [];
        if (relatedEvents.length === 0) {
            showToast('No related event found to consolidate with', 'error');
            return;
        }

        // Determine canonical and retire ULIDs
        const canonicalInput = document.querySelector(`input[name="canonical-${entryId}"]:checked`);
        const canonicalValue = canonicalInput ? canonicalInput.value : 'this';

        let canonicalUlid, retireUlid;
        if (canonicalValue === 'this') {
            canonicalUlid = detail.eventId;
            retireUlid = relatedEvents[0].ulid;
        } else {
            canonicalUlid = relatedEvents[0].ulid;
            retireUlid = detail.eventId;
        }

        const btn = document.querySelector(`[data-action="consolidate"][data-id="${entryId}"]`);
        if (btn) setLoading(btn, true);

        // Hide any previous consolidate error
        const consolidateError = document.getElementById(`consolidate-error-${entryId}`);
        if (consolidateError) consolidateError.style.display = 'none';

        // Step 1: Build field patch from fieldOverrides where user edited (edited === true)
        // If user selected from related event (eventIndex !== canonicalIndex), patch needed
        const overrides = fieldOverrides[entryId] || {};
        const patch = {};

        Object.entries(overrides).forEach(([key, override]) => {
            if (override.edited === true && override.eventIndex !== undefined) {
                // Patch if user selected from non-canonical event
                if (override.eventIndex !== canonicalIndex) {
                    // Map field keys to API fields
                    // name→name, description→description, url→public_url, image→image_url
                    // skip location.name, organizer.name — not patchable via PUT
                    const fieldMap = {
                        'name': 'name',
                        'description': 'description',
                        'url': 'public_url',
                        'image': 'image_url',
                    };
                    const apiKey = fieldMap[key];
                    if (apiKey) {
                        patch[apiKey] = override.value;
                    }
                }
            }
        });

        // Step 2: Update canonical event with field overrides
        if (Object.keys(patch).length > 0) {
            try {
                await API.events.update(canonicalUlid, patch);
            } catch (err) {
                console.error('Failed to update event fields:', err);
                const msg = (err && err.detail) || (err && err.message) || 'Failed to update event fields';
                if (consolidateError) {
                    consolidateError.textContent = msg;
                    consolidateError.style.display = 'block';
                } else {
                    showToast(msg, 'error');
                }
                if (btn) setLoading(btn, false);
                return;
            }
        }

        // Step 3: Add selected related occurrences to canonical event
        const pickerEntries = occurrencePicker[entryId] || [];
        const relatedIncludedEntries = pickerEntries.filter(e => e.source === 'related' && e.included && !e.overlaps);

        for (const entry of relatedIncludedEntries) {
            try {
                const occData = entry.occurrence;
                const body = {
                    start_time: occData.startTime,
                    timezone: occData.timezone || 'America/Toronto',
                };
                if (occData.endTime) {
                    body.end_time = occData.endTime;
                }
                await API.events.occurrences.create(canonicalUlid, body);
            } catch (err) {
                // On 409 overlap, show inline error but continue with other occurrences
                if (err && err.status === 409) {
                    const msg = 'Conflict: overlaps with existing occurrence';
                    showToast(msg, 'error');
                    console.warn('Occurrence conflict:', err);
                } else {
                    console.error('Failed to add occurrence:', err);
                    const msg = (err && err.detail) || (err && err.message) || 'Failed to add occurrence';
                    showToast(msg, 'error');
                }
            }
        }

        // Step 4: Consolidate (retire the duplicate)
        try {
            await API.events.consolidate({
                event_ulid: canonicalUlid,
                retire: [retireUlid],
                transfer_occurrences: false,
            });
            showToast('Consolidated successfully', 'success');

            // Remove this entry and any companion entries dismissed by the backend.
            const dismissed = new Set([String(entryId)]);
            dismissed.forEach(id => removeEntryFromList(id));

            if (btn) setLoading(btn, false);

            // Always reload from the server — consolidation can affect multiple entries
            loadEntries();
        } catch (err) {
            console.error('Failed to consolidate events:', err);
            const msg = (err && err.detail) || (err && err.message) || 'Failed to consolidate events';
            if (consolidateError) {
                consolidateError.textContent = msg;
                consolidateError.style.display = 'block';
            } else {
                showToast(msg, 'error');
            }
            if (btn) setLoading(btn, false);
        }
    }

    /**
     * Collapse detail view
     * Removes the expanded detail row and resets expandedId state
     */
    function collapseDetail() {
        if (!expandedId) return;
        
        const detailRow = document.getElementById(`detail-${expandedId}`);
        if (detailRow) {
            detailRow.remove();
        }
        
        // Change arrow direction back to down (expanded -> collapsed)
        const entryRow = document.querySelector(`tr[data-entry-id="${expandedId}"]`);
        if (entryRow) {
            const arrowButton = entryRow.querySelector('.expand-arrow');
            if (arrowButton) {
                const arrowIcon = arrowButton.querySelector('polyline');
                if (arrowIcon) {
                    arrowIcon.setAttribute('points', '6 9 12 15 18 9'); // Down arrow
                }
            }
        }
        
        // Clean up field overrides for collapsed entry
        if (fieldOverrides[expandedId]) {
            delete fieldOverrides[expandedId];
        }

        // Clean up occurrence picker for collapsed entry
        if (occurrencePicker[expandedId]) {
            delete occurrencePicker[expandedId];
        }

        expandedId = null;
        currentEntryDetail = null;
    }
    
    /**
     * Show more text for truncated descriptions
     * Expands a truncated text field and hides the "Show more" button
     * @param {HTMLElement} button - The "Show more" button element
     */
    function showMoreText(button) {
        const fullText = button.dataset.fullText;
        if (!fullText) return;
        
        // Find the text span (previous sibling)
        const textSpan = button.previousElementSibling;
        if (textSpan && textSpan.classList.contains('description-text')) {
            // Use innerHTML to preserve linkified URLs (already escaped + linkified)
            textSpan.innerHTML = fullText;
            button.style.display = 'none';
        }
    }
    
    /**
     * Approve a review queue entry
     * Sends approval to API and removes entry from list if filtering by pending
     * @async
     * @param {string} id - Review queue entry ID
     * @throws {Error} If API request fails
     */
    async function approve(id) {
        const button = document.querySelector(`[data-action="approve"][data-id="${id}"]`);
        if (!button) return;
        
        setLoading(button, true);
        
        try {
            await API.reviewQueue.approve(id, {});
            showToast('Entry approved successfully', 'success');
            
            // Remove from list if filtering by pending
            if (currentFilter === 'pending') {
                removeEntryFromList(id);
                
                // Increment approved count badge
                const approvedBadge = document.querySelector(`[data-action="filter-status"][data-status="approved"] .badge`);
                if (approvedBadge) {
                    const currentCount = parseInt(approvedBadge.textContent) || 0;
                    approvedBadge.textContent = currentCount + 1;
                }
            } else {
                // Reload to show updated status
                loadEntries();
            }
        } catch (err) {
            console.error('Failed to approve entry:', err);
            showToast(err.message || 'Failed to approve entry', 'error');
            setLoading(button, false);
        }
    }
    
    /**
     * Mark a review entry as "Not a Duplicate" — approves/publishes the event
     * and records the duplicate warning pairs as not-duplicates so future
     * ingestion won't re-flag them.
     * @async
     * @param {string} id - Review queue entry ID
     * @throws {Error} If API request fails
     */
    async function notADuplicate(id) {
        const button = document.querySelector(`[data-action="not-a-duplicate"][data-id="${id}"]`);
        if (!button) return;
        
        setLoading(button, true);
        
        try {
            await API.reviewQueue.approve(id, { record_not_duplicates: true });
            showToast('Approved as not a duplicate', 'success');
            
            // Remove from list if filtering by pending
            if (currentFilter === 'pending') {
                removeEntryFromList(id);
                
                // Increment approved count badge
                const approvedBadge = document.querySelector(`[data-action="filter-status"][data-status="approved"] .badge`);
                if (approvedBadge) {
                    const currentCount = parseInt(approvedBadge.textContent) || 0;
                    approvedBadge.textContent = currentCount + 1;
                }
            } else {
                // Reload to show updated status
                loadEntries();
            }
        } catch (err) {
            console.error('Failed to mark as not a duplicate:', err);
            showToast(err.message || 'Failed to mark as not a duplicate', 'error');
            setLoading(button, false);
        }
    }
    
    /**
     * Show reject modal dialog
     * Opens Bootstrap modal for entering rejection reason
     * @param {string} id - Review queue entry ID to reject
     */
    function showRejectModal(id) {
        const modal = document.getElementById('reject-modal');
        const textarea = document.getElementById('reject-reason');
        const confirmBtn = document.getElementById('confirm-reject-btn');
        const errorDiv = document.getElementById('reject-reason-error');
        
        if (!modal || !textarea || !confirmBtn) return;
        
        // Clear previous input
        textarea.value = '';
        textarea.classList.remove('is-invalid');
        errorDiv.textContent = '';
        
        // Store entry ID on confirm button
        confirmBtn.dataset.id = id;
        
        // Show modal
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
    }
    
    /**
     * Confirm reject action
     * Validates rejection reason, sends rejection to API, and removes entry from list
     * @async
     * @throws {Error} If API request fails
     */
    async function confirmReject() {
        const modal = document.getElementById('reject-modal');
        const textarea = document.getElementById('reject-reason');
        const confirmBtn = document.getElementById('confirm-reject-btn');
        const errorDiv = document.getElementById('reject-reason-error');
        const id = confirmBtn.dataset.id;
        
        if (!id) return;
        
        const reason = textarea.value.trim();
        
        // Validate reason
        if (!reason) {
            textarea.classList.add('is-invalid');
            errorDiv.textContent = 'Reason is required';
            return;
        }
        
        textarea.classList.remove('is-invalid');
        setLoading(confirmBtn, true);
        
        try {
            await API.reviewQueue.reject(id, { reason });
            
            // Close modal
            const bsModal = bootstrap.Modal.getInstance(modal);
            if (bsModal) {
                bsModal.hide();
            }
            
            showToast('Event deleted', 'success');
            
            // Remove from list if filtering by pending
            if (currentFilter === 'pending') {
                removeEntryFromList(id);
                
                // Increment rejected count badge
                const rejectedBadge = document.querySelector(`[data-action="filter-status"][data-status="rejected"] .badge`);
                if (rejectedBadge) {
                    const currentCount = parseInt(rejectedBadge.textContent) || 0;
                    rejectedBadge.textContent = currentCount + 1;
                }
            } else {
                // Reload to show updated status
                loadEntries();
            }
        } catch (err) {
            console.error('Failed to delete event:', err);
            showToast(err.message || 'Failed to delete event', 'error');
        } finally {
            setLoading(confirmBtn, false);
        }
    }
    
    /**
     * Fetch a candidate event and render its summary inline in the fold-down.
     * Shows a side-by-side diff against the current review entry's normalized data.
     * @param {HTMLElement} container - The placeholder div to populate
     * @param {string} dupUlid - ULID of the candidate event to fetch
     * @param {Object|null} thisEventData - Normalized data of the current review entry for diff highlighting
     * @param {Object|null} matchData - Embedded match data from the warning (fast path, avoids API fetch)
     */
    async function fetchAndRenderInlineDuplicate(container, dupUlid, thisEventData, matchData) {
        try {
            let event;
            // Fast path: use embedded data from the warning — no API fetch needed.
            // NOTE: Only name/startDate/endDate/location are embedded; description and
            // organizer fields will be absent from the side-by-side diff card. This is
            // intentional — the fast path prioritises avoiding 404s for pending-review
            // candidates over showing a complete diff.
            if (matchData && matchData.name) {
                event = {
                    name: matchData.name,
                    startDate: matchData.startDate || null,
                    endDate: matchData.endDate || null,
                    location: matchData.location || null,
                };
            } else {
                event = await API.request('/api/v1/events/' + encodeURIComponent(dupUlid));
            }
            const html = `
                <div class="row g-2 mt-1">
                    <div class="col-md-6">
                        <div class="card bg-light">
                            <div class="card-header py-1"><small class="text-muted fw-semibold">This event</small></div>
                            <div class="card-body py-2">${renderMergeEventSummary(thisEventData, event)}</div>
                        </div>
                    </div>
                    <div class="col-md-6">
                        <div class="card bg-light">
                            <div class="card-header py-1"><small class="text-muted fw-semibold">Candidate duplicate</small></div>
                            <div class="card-body py-2">${renderMergeEventSummary(event, thisEventData)}</div>
                        </div>
                    </div>
                </div>
            `;
            container.innerHTML = html;
        } catch (err) {
            // Note: a 404 here is expected when the candidate event is still in
            // pending_review state (not yet published) and not accessible via the
            // public events API.
            console.error('Failed to fetch duplicate event:', err);
            container.innerHTML = `<div class="text-muted small">Could not load duplicate details: ${escapeHtml(err.message || 'unknown error')}</div>`;
        }
    }

    /**
     * Directly merge a duplicate without showing a modal.
     * Uses the warning code and IDs already stored on the Merge Duplicate button.
     * Shows a spinner on the button while the request is in flight.
     * @param {string} id - Review queue entry ID
     * @param {string} duplicateEventId - ULID of the primary event (potential_duplicate only)
     * @param {string} warningCode - The duplicate warning type
     * @param {string} primaryId - Primary place/org ULID (place/org duplicate types)
     * @param {string} duplicateId - Duplicate place/org ULID (place/org duplicate types)
     */
    async function mergeDirect(id, duplicateEventId, warningCode, primaryId, duplicateId) {
        const btn = document.querySelector(`[data-action="merge"][data-id="${id}"]`);
        if (btn) setLoading(btn, true);
        try {
            if (warningCode === 'place_possible_duplicate') {
                if (!primaryId || !duplicateId) {
                    showToast('No place IDs available to merge', 'error');
                    if (btn) setLoading(btn, false);
                    return;
                }
                await API.places.merge(primaryId, duplicateId);
                await API.reviewQueue.approve(id);
                showToast('Places merged successfully', 'success');
            } else if (warningCode === 'org_possible_duplicate') {
                if (!primaryId || !duplicateId) {
                    showToast('No organization IDs available to merge', 'error');
                    if (btn) setLoading(btn, false);
                    return;
                }
                await API.organizations.merge(primaryId, duplicateId);
                await API.reviewQueue.approve(id);
                showToast('Organizations merged successfully', 'success');
            } else {
                if (!duplicateEventId) {
                    showToast('No duplicate event ID available to merge', 'error');
                    if (btn) setLoading(btn, false);
                    return;
                }
                showToast('Events merged successfully', 'success');
            }
            if (currentFilter === 'pending') {
                removeEntryFromList(id);
                if (warningCode !== 'place_possible_duplicate' && warningCode !== 'org_possible_duplicate') {
                    const mergedBadge = document.querySelector(`[data-action="filter-status"][data-status="merged"] .badge`);
                    if (mergedBadge) {
                        const currentCount = parseInt(mergedBadge.textContent) || 0;
                        mergedBadge.textContent = currentCount + 1;
                    }
                }
            } else {
                loadEntries();
            }
        } catch (err) {
            console.error('Failed to merge:', err);
            showToast(err.message || 'Failed to merge', 'error');
            if (btn) setLoading(btn, false);
        }
    }

    /**
     * Render a compact place summary for side-by-side duplicate diff cards.
     * Shows name, address, URL, phone, and email with diff highlighting.
     * @param {Object} data - Place data with name, address_street, address_locality, address_region, postal_code, url, telephone, email
     * @param {Object|null} compareData - Optional data to compare against for diff highlighting
     * @returns {string} HTML string
     */
    function renderPlaceSummary(data, compareData) {
        if (!data) return '<p class="text-muted">No data available</p>';
        var rows = [];
        rows.push(renderMergeField('', data.name || '', compareData ? compareData.name || '' : null, true));
        var addr = [data.address_street, data.address_locality, data.address_region, data.postal_code].filter(Boolean).join(', ');
        var cmpAddr = compareData ? [compareData.address_street, compareData.address_locality, compareData.address_region, compareData.postal_code].filter(Boolean).join(', ') : null;
        if (addr || cmpAddr) rows.push(renderMergeField('Address', addr, cmpAddr, false));
        if (data.url || (compareData && compareData.url)) rows.push(renderMergeField('URL', data.url || '', compareData ? compareData.url || '' : null, false));
        if (data.telephone || (compareData && compareData.telephone)) rows.push(renderMergeField('Phone', data.telephone || '', compareData ? compareData.telephone || '' : null, false));
        if (data.email || (compareData && compareData.email)) rows.push(renderMergeField('Email', data.email || '', compareData ? compareData.email || '' : null, false));
        return rows.join('');
    }

    /**
     * Render a compact organization summary for side-by-side duplicate diff cards.
     * Shows name, location, URL, phone, and email with diff highlighting.
     * @param {Object} data - Org data with name, address_locality, address_region, url, telephone, email
     * @param {Object|null} compareData - Optional data to compare against for diff highlighting
     * @returns {string} HTML string
     */
    function renderOrgSummary(data, compareData) {
        if (!data) return '<p class="text-muted">No data available</p>';
        var rows = [];
        rows.push(renderMergeField('', data.name || '', compareData ? compareData.name || '' : null, true));
        var addr = [data.address_locality, data.address_region].filter(Boolean).join(', ');
        var cmpAddr = compareData ? [compareData.address_locality, compareData.address_region].filter(Boolean).join(', ') : null;
        if (addr || cmpAddr) rows.push(renderMergeField('Location', addr, cmpAddr, false));
        if (data.url || (compareData && compareData.url)) rows.push(renderMergeField('URL', data.url || '', compareData ? compareData.url || '' : null, false));
        if (data.telephone || (compareData && compareData.telephone)) rows.push(renderMergeField('Phone', data.telephone || '', compareData ? compareData.telephone || '' : null, false));
        if (data.email || (compareData && compareData.email)) rows.push(renderMergeField('Email', data.email || '', compareData ? compareData.email || '' : null, false));
        return rows.join('');
    }

    /**
     * Render a compact event summary for the merge comparison panels.
     * Shows name, date, venue, organizer, and description excerpt.
     * When compareData is provided, highlights fields that differ.
     * @param {Object} data - Event data object with name, startDate, location, description, organizer
     * @param {Object|null} compareData - Optional data to compare against for diff highlighting
     * @returns {string} HTML string
     */
    function renderMergeEventSummary(data, compareData) {
        if (!data) return '<p class="text-muted">No data available</p>';
        
        var fields = extractMergeFields(data);
        var compareFields = compareData ? extractMergeFields(compareData) : null;
        
        return renderMergeFieldRows(fields, compareFields);
    }
    
    /**
     * Extract display fields from an event data object for merge comparison.
     * Normalizes both normalized-payload and JSON-LD response shapes.
     * @param {Object} data - Event data
     * @returns {Object} Extracted fields: name, startDate, endDate, venue, organizer, description
     */
    function extractMergeFields(data) {
        if (!data) return {};
        
        // Prefer top-level camelCase startDate/endDate (original submission or enriched
        // reconstructed payload). Fall back to occurrences[0].start_date (snake_case) for
        // older reconstructed payloads that predate the camelCase enrichment.
        let startDate = data.startDate || null;
        let endDate = data.endDate || null;
        if (!startDate && Array.isArray(data.occurrences) && data.occurrences.length > 0) {
            startDate = data.occurrences[0].start_date || null;
            endDate = data.occurrences[0].end_date || null;
        }

        return {
            name: data.name || 'Untitled Event',
            startDate: startDate ? formatDateValue(startDate) : 'No date',
            endDate: endDate ? formatDateValue(endDate) : '',
            venue: extractVenueName(data.location),
            organizer: extractOrganizerName(data.organizer),
            description: data.description || ''
        };
    }
    
    /**
     * Render merge field rows with optional diff highlighting.
     * When compareFields is provided, matching values get green, different values get amber.
     * @param {Object} fields - Primary fields to render
     * @param {Object|null} compareFields - Fields to compare against (null = no highlighting)
     * @returns {string} HTML string
     */
    function renderMergeFieldRows(fields, compareFields) {
        var rows = [];
        
        // Name
        rows.push(renderMergeField('', fields.name, compareFields ? compareFields.name : null, true));
        
        // Date
        var dateStr = fields.startDate + (fields.endDate ? ' \u2013 ' + fields.endDate : '');
        var compareDateStr = compareFields
            ? compareFields.startDate + (compareFields.endDate ? ' \u2013 ' + compareFields.endDate : '')
            : null;
        rows.push(renderMergeField('Date', dateStr, compareDateStr, false));
        
        // Venue
        if (fields.venue || (compareFields && compareFields.venue)) {
            rows.push(renderMergeField('Venue', fields.venue, compareFields ? compareFields.venue : null, false));
        }
        
        // Organizer
        if (fields.organizer || (compareFields && compareFields.organizer)) {
            rows.push(renderMergeField('Organizer', fields.organizer, compareFields ? compareFields.organizer : null, false));
        }
        
        // Description excerpt
        var desc = fields.description;
        var descExcerpt = desc.length > 120 ? desc.substring(0, 120) + '...' : desc;
        var compareDescExcerpt = null;
        if (compareFields && compareFields.description) {
            var cd = compareFields.description;
            compareDescExcerpt = cd.length > 120 ? cd.substring(0, 120) + '...' : cd;
        }
        if (descExcerpt || compareDescExcerpt) {
            rows.push(renderMergeField('Description', descExcerpt, compareDescExcerpt, false, true));
        }
        
        return rows.join('');
    }
    
    /**
     * Render a single merge comparison field with optional diff highlighting.
     * @param {string} label - Field label (empty for title row)
     * @param {string} value - Field value to display
     * @param {string|null} compareValue - Value to compare against (null = no highlighting)
     * @param {boolean} isTitle - Whether this is the title/name row (rendered as bold)
     * @param {boolean} isSmall - Whether to render value in small text
     * @returns {string} HTML string
     */
    function renderMergeField(label, value, compareValue, isTitle, isSmall) {
        if (!value && !compareValue) return '';
        
        var displayValue = value || '';
        
        // Determine diff class
        var diffClass = '';
        if (compareValue !== null && compareValue !== undefined) {
            if (displayValue === compareValue) {
                diffClass = 'bg-success-lt';
            } else {
                diffClass = 'bg-warning-lt';
            }
        }
        
        var classStr = 'mb-2' + (diffClass ? ' p-1 rounded ' + diffClass : '');
        var valueClass = isSmall ? 'small' : '';
        
        if (isTitle) {
            return '<div class="' + classStr + '">' +
                '<strong class="d-block">' + escapeHtml(displayValue) + '</strong>' +
                '</div>';
        }
        
        return '<div class="' + classStr + '">' +
            (label ? '<small class="text-muted">' + escapeHtml(label) + ':</small>' : '') +
            '<span class="d-block ' + valueClass + '">' + escapeHtml(displayValue) + '</span>' +
            '</div>';
    }
    
    /**
     * Extract venue name from a location object or string.
     * Handles normalized payload format, JSON-LD embedded Place, and URI strings.
     * @param {Object|string} location - Location data
     * @returns {string} Venue name or empty string
     */
    function extractVenueName(location) {
        if (!location) return '';
        if (typeof location === 'string') {
            // URI string fallback — shouldn't happen with embedded objects, but handle gracefully
            return '';
        }
        if (typeof location === 'object') {
            return location.name || '';
        }
        return '';
    }
    
    /**
     * Extract organizer name from an organizer object or string.
     * Handles both embedded Organization objects and URI strings.
     * @param {Object|string} organizer - Organizer data
     * @returns {string} Organizer name or empty string
     */
    function extractOrganizerName(organizer) {
        if (!organizer) return '';
        if (typeof organizer === 'string') return '';
        if (typeof organizer === 'object') {
            return organizer.name || '';
        }
        return '';
    }
    
    /**
     * Setup debounced input handler for the ULID field in merge modal
     * Fetches and renders the primary event when a valid ULID is entered
     * @param {HTMLInputElement} input - The ULID input element
     */
    function setupMergeUlidInput(input) {
        // Remove any existing handler by cloning the element
        const newInput = input.cloneNode(true);
        input.parentNode.replaceChild(newInput, input);
        
        let debounceTimer = null;
        
        newInput.addEventListener('input', function() {
            const value = this.value.trim();
            
            // Clear any pending lookup
            if (debounceTimer) {
                clearTimeout(debounceTimer);
                debounceTimer = null;
            }
            
            // Reset validation state
            this.classList.remove('is-invalid');
            const errorDiv = document.getElementById('merge-event-error');
            if (errorDiv) errorDiv.textContent = '';
            
            // Check if it's a valid ULID
            if (value.length === ULID_LENGTH && ULID_PATTERN.test(value)) {
                debounceTimer = setTimeout(function() {
                    fetchAndRenderPrimaryEvent(value);
                }, MERGE_LOOKUP_DEBOUNCE_MS);
            } else if (value.length === 0) {
                // Clear the primary panel
                const primaryInfo = document.getElementById('merge-primary-info');
                if (primaryInfo) {
                    primaryInfo.innerHTML = '<p class="text-muted">Enter a ULID below to load the primary event</p>';
                }
            } else if (value.length >= ULID_LENGTH) {
                // Invalid format
                const primaryInfo = document.getElementById('merge-primary-info');
                if (primaryInfo) {
                    primaryInfo.innerHTML = '<p class="text-warning">Invalid ULID format</p>';
                }
            }
        });
    }
    
    /**
     * Fetch a primary event by ULID and render it in the merge modal
     * Uses the public events API endpoint
     * @async
     * @param {string} ulid - Event ULID to fetch
     */
    async function fetchAndRenderPrimaryEvent(ulid) {
        const primaryInfo = document.getElementById('merge-primary-info');
        if (!primaryInfo) return;
        
        // Show loading
        primaryInfo.innerHTML = `
            <div class="text-center py-3">
                <div class="spinner-border spinner-border-sm text-primary" role="status">
                    <span class="visually-hidden">Loading...</span>
                </div>
                <small class="text-muted d-block mt-1">Loading event...</small>
            </div>
        `;
        
        try {
            // Use public events endpoint (admin GET doesn't exist)
            const event = await API.request('/api/v1/events/' + encodeURIComponent(ulid));
            
            // Get duplicate data for comparison highlighting
            var duplicateData = (currentEntryDetail && currentEntryDetail.normalized) ? currentEntryDetail.normalized : null;
            
            // Render primary event with diff highlighting against duplicate
            primaryInfo.innerHTML = renderMergeEventSummary(event, duplicateData);
            
            // Also re-render duplicate panel with diff highlighting against primary
            var duplicateInfo = document.getElementById('merge-duplicate-info');
            if (duplicateInfo && duplicateData) {
                duplicateInfo.innerHTML = renderMergeEventSummary(duplicateData, event);
            }
        } catch (err) {
            console.error('Failed to fetch primary event:', err);
            primaryInfo.innerHTML = `
                <div class="text-danger">
                    <svg xmlns="http://www.w3.org/2000/svg" class="icon icon-sm" width="16" height="16" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                        <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                        <circle cx="12" cy="12" r="9"/>
                        <line x1="12" y1="8" x2="12" y2="12"/>
                        <line x1="12" y1="16" x2="12.01" y2="16"/>
                    </svg>
                    <span class="ms-1">${escapeHtml(err.message || 'Event not found')}</span>
                </div>
            `;
        }
    }
    
    /**
     * Confirm merge action
     * Validates primary event ID, sends merge request to API, and removes entry from list
     * @async
     * @throws {Error} If validation fails or API request fails
     */
    async function confirmMerge() {
        const modal = document.getElementById('merge-modal');
        const input = document.getElementById('merge-primary-event-id');
        const confirmBtn = document.getElementById('confirm-merge-btn');
        const errorDiv = document.getElementById('merge-event-error');
        const id = confirmBtn.dataset.id;
        
        if (!id) return;
        
        const primaryEventId = input.value.trim();
        
        // Validate
        if (!primaryEventId) {
            input.classList.add('is-invalid');
            errorDiv.textContent = 'Primary event ID is required';
            return;
        }
        
        input.classList.remove('is-invalid');
        setLoading(confirmBtn, true);
        
        try {
            // Close modal
            const bsModal = bootstrap.Modal.getInstance(modal);
            if (bsModal) {
                bsModal.hide();
            }
            
            showToast('Events merged successfully', 'success');
            
            // Remove from list if filtering by pending
            if (currentFilter === 'pending') {
                removeEntryFromList(id);
                
                // Increment merged count badge
                const mergedBadge = document.querySelector(`[data-action="filter-status"][data-status="merged"] .badge`);
                if (mergedBadge) {
                    const currentCount = parseInt(mergedBadge.textContent) || 0;
                    mergedBadge.textContent = currentCount + 1;
                }
            } else {
                // Reload to show updated status
                loadEntries();
            }
        } catch (err) {
            console.error('Failed to merge events:', err);
            showToast(err.message || 'Failed to merge events', 'error');
        } finally {
            setLoading(confirmBtn, false);
        }
    }
    
    /**
     * Show fix dates form
     * Displays inline form for correcting event start/end dates with current values pre-filled
     * @param {string} id - Review queue entry ID
     */
    function showFixForm(id) {
        const entry = entries.find(e => e.id === parseInt(id));
        if (!entry) return;
        
        const actionButtons = document.getElementById(`action-buttons-${id}`);
        const fixFormContainer = document.getElementById(`fix-form-${id}`);
        
        if (!actionButtons || !fixFormContainer) return;
        
        // Hide action buttons
        actionButtons.style.display = 'none';
        
        // Convert ISO dates to datetime-local format (YYYY-MM-DDTHH:MM)
        const startValue = entry.eventStartTime ? formatDateTimeLocal(entry.eventStartTime) : '';
        const endValue = entry.eventEndTime ? formatDateTimeLocal(entry.eventEndTime) : '';
        
        // Show fix form
        fixFormContainer.style.display = 'block';
        fixFormContainer.innerHTML = `
            <div class="card bg-light">
                <div class="card-body">
                    <h4>Correct Dates</h4>
                    <div class="mb-3">
                        <label class="form-label">Start Date & Time</label>
                        <input type="datetime-local" class="form-control" id="fix-start-${id}" value="${startValue}">
                    </div>
                    <div class="mb-3">
                        <label class="form-label">End Date & Time</label>
                        <input type="datetime-local" class="form-control" id="fix-end-${id}" value="${endValue}">
                    </div>
                    <div class="mb-3">
                        <label class="form-label">Notes (optional)</label>
                        <textarea class="form-control" id="fix-notes-${id}" rows="2"></textarea>
                    </div>
                    <div class="btn-list">
                        <button class="btn" data-action="cancel-fix" data-id="${id}">Cancel</button>
                        <button class="btn btn-primary" data-action="apply-fix" data-id="${id}">Apply Fix</button>
                    </div>
                </div>
            </div>
        `;
    }
    
    /**
     * Hide fix dates form
     * Removes inline fix form and restores action buttons
     */
    function hideFixForm() {
        if (!expandedId) return;
        
        const actionButtons = document.getElementById(`action-buttons-${expandedId}`);
        const fixFormContainer = document.getElementById(`fix-form-${expandedId}`);
        
        if (actionButtons) {
            actionButtons.style.display = 'block';
        }
        
        if (fixFormContainer) {
            fixFormContainer.style.display = 'none';
            fixFormContainer.innerHTML = '';
        }
    }
    
    /**
     * Apply date corrections
     * Validates and submits corrected start/end dates to API
     * @async
     * @param {string} id - Review queue entry ID
     * @throws {Error} If validation fails or API request fails
     */
    async function applyFix(id) {
        const startInput = document.getElementById(`fix-start-${id}`);
        const endInput = document.getElementById(`fix-end-${id}`);
        const notesInput = document.getElementById(`fix-notes-${id}`);
        const applyBtn = document.querySelector(`[data-action="apply-fix"][data-id="${id}"]`);
        
        if (!startInput || !endInput || !applyBtn) return;
        
        const startValue = startInput.value;
        const endValue = endInput.value;
        const notes = notesInput ? notesInput.value.trim() : '';
        
        // Validate
        if (!startValue || !endValue) {
            showToast('Both start and end dates are required', 'error');
            return;
        }
        
        setLoading(applyBtn, true);
        
        try {
            // datetime-local inputs are treated as UTC by appending 'Z'.
            const toUTC = (localStr) => localStr ? localStr + 'Z' : '';
            const corrections = {
                startDate: toUTC(startValue),
                endDate: toUTC(endValue)
            };
            
            const payload = { corrections };
            if (notes) {
                payload.notes = notes;
            }
            
            await API.reviewQueue.fix(id, payload);
            showToast('Dates corrected successfully', 'success');
            
            // Remove from list if filtering by pending
            if (currentFilter === 'pending') {
                removeEntryFromList(id);
                
                // Increment approved count badge (fix action approves the entry)
                const approvedBadge = document.querySelector(`[data-action="filter-status"][data-status="approved"] .badge`);
                if (approvedBadge) {
                    const currentCount = parseInt(approvedBadge.textContent) || 0;
                    approvedBadge.textContent = currentCount + 1;
                }
            } else {
                // Reload to show updated status
                loadEntries();
            }
        } catch (err) {
            console.error('Failed to apply fix:', err);
            showToast(err.message || 'Failed to apply fix', 'error');
            setLoading(applyBtn, false);
        }
    }
    
    /**
     * Remove entry from list after action
     * Removes entry from state array and DOM, updates UI accordingly
     * @param {string|number} id - Review queue entry ID
     */
    function removeEntryFromList(id) {
        const entryId = parseInt(id);
        entries = entries.filter(e => e.id !== entryId);
        
        // Remove rows from DOM
        const entryRow = document.querySelector(`tr[data-entry-id="${entryId}"]`);
        const detailRow = document.getElementById(`detail-${entryId}`);
        
        if (entryRow) entryRow.remove();
        if (detailRow) detailRow.remove();
        
        expandedId = null;
        
        // Update UI
        if (entries.length === 0) {
            showEmptyState();
        }
        
        // Decrement badge count for current filter
        const badge = document.querySelector(`[data-action="filter-status"][data-status="${currentFilter}"] .badge`);
        if (badge) {
            const currentCount = parseInt(badge.textContent) || 0;
            if (currentCount > 0) {
                badge.textContent = currentCount - 1;
            }
        }
    }
    
    /**
     * Update badge count for a specific status tab
     * Updates the visual badge showing number of entries for the given status
     * @param {string} status - Status to update ('pending', 'approved', 'rejected')
     * @param {number} count - Total number of entries for this status
     */
    function updateBadgeCount(status, count) {
        const badge = document.querySelector(`[data-action="filter-status"][data-status="${status}"] .badge`);
        if (badge) {
            badge.textContent = count;
        }
    }
    

    
    /**
     * Show loading state
     * Displays loading spinner and hides empty state and table
     */
    function showLoading() {
        document.getElementById('loading-state').style.display = 'block';
        document.getElementById('empty-state').style.display = 'none';
        document.getElementById('review-queue-container').style.display = 'none';
    }
    
    /**
     * Show empty state
     * Displays empty state message and hides loading and table
     */
    function showEmptyState() {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('empty-state').style.display = 'block';
        document.getElementById('review-queue-container').style.display = 'none';
    }
    
    /**
     * Show table with entries
     * Displays review queue table and hides loading and empty states
     */
    function showTable() {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('empty-state').style.display = 'none';
        document.getElementById('review-queue-container').style.display = 'block';
    }
    
    /**
     * Get relative time string
     * Converts date to human-readable relative format (e.g., "2h ago", "5d ago")
     * @param {string} dateString - ISO 8601 date string
     * @returns {string} Relative time string or '-' if invalid
     */
    function getRelativeTime(dateString) {
        if (!dateString) return '-';
        
        const date = new Date(dateString);
        const now = new Date();
        const diffMs = now - date;
        const diffMins = Math.floor(diffMs / MILLISECONDS_PER_MINUTE);
        const diffHours = Math.floor(diffMins / MINUTES_PER_HOUR);
        const diffDays = Math.floor(diffHours / HOURS_PER_DAY);
        
        if (diffMins < MINUTES_DISPLAY_THRESHOLD) {
            return `${diffMins}m ago`;
        } else if (diffHours < HOURS_DISPLAY_THRESHOLD) {
            return `${diffHours}h ago`;
        } else {
            return `${diffDays}d ago`;
        }
    }
    
    /**
     * Format date value for display
     * Safely formats date strings, returning original value if formatting fails
     * @param {string} value - Date string to format
     * @returns {string} Formatted date or original value
     */
    function formatDateValue(value) {
        if (!value) return '';
        try {
            return formatDate(value);
        } catch {
            return value;
        }
    }
    
    /**
     * Convert ISO date to datetime-local format
     * Formats ISO 8601 date string for use in datetime-local input field
     * @param {string} isoString - ISO 8601 date string
     * @returns {string} Date in YYYY-MM-DDTHH:MM format, or empty string if invalid
     */
    function formatDateTimeLocal(isoString) {
        if (!isoString) return '';
        try {
            const date = new Date(isoString);
            // Format as YYYY-MM-DDTHH:MM (required by datetime-local input)
            const year = date.getFullYear();
            const month = String(date.getMonth() + MONTH_INDEX_OFFSET).padStart(DATE_PADDING_LENGTH, DATE_PADDING_CHAR);
            const day = String(date.getDate()).padStart(DATE_PADDING_LENGTH, DATE_PADDING_CHAR);
            const hours = String(date.getHours()).padStart(DATE_PADDING_LENGTH, DATE_PADDING_CHAR);
            const minutes = String(date.getMinutes()).padStart(DATE_PADDING_LENGTH, DATE_PADDING_CHAR);
            return `${year}-${month}-${day}T${hours}:${minutes}`;
        } catch {
            return '';
        }
    }
    
})();
