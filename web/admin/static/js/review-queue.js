/**
 * Review Queue Page JavaScript
 * Handles review queue listing, filtering, expand/collapse detail view, and approve/reject/fix actions
 */
(function() {
    'use strict';
    
    // State management
    let entries = [];
    let currentFilter = 'pending';
    let expandedId = null;
    let cursor = null;
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        setupEventListeners();
        loadEntries();
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
                case 'expand-detail':
                    e.preventDefault();
                    expandDetail(id);
                    break;
                case 'collapse-detail':
                    e.preventDefault();
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
                case 'confirm-reject':
                    confirmReject();
                    break;
                case 'next-page':
                    e.preventDefault();
                    goToNextPage(target.dataset.cursor);
                    break;
            }
        });
    }
    
    /**
     * Filter by status tab
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
        
        loadEntries();
    }
    
    /**
     * Load review queue entries from API
     */
    async function loadEntries() {
        showLoading();
        
        try {
            const params = {
                status: currentFilter,
                limit: 50
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
            
            // Update pending count badge
            if (currentFilter === 'pending' && data.items) {
                updatePendingCount(data.items.length);
            }
            
            if (entries.length === 0) {
                showEmptyState();
            } else {
                showTable();
                renderTable();
                updatePagination(data.next_cursor);
            }
        } catch (err) {
            console.error('Failed to load review queue:', err);
            showToast(err.message || 'Failed to load review queue', 'error');
            showEmptyState();
        }
    }
    
    /**
     * Render entries into table
     */
    function renderTable() {
        const tbody = document.getElementById('review-queue-table');
        if (!tbody) return;
        
        tbody.innerHTML = entries.map(entry => {
            const eventName = entry.eventName || 'Untitled Event';
            const startTime = entry.eventStartTime ? formatDate(entry.eventStartTime, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : 'No date';
            const warningBadge = getWarningBadge(entry.warnings);
            const createdAgo = getRelativeTime(entry.createdAt);
            
            return `
                <tr data-entry-id="${entry.id}">
                    <td>
                        <a href="/admin/events/${entry.eventId}" class="text-reset">
                            ${escapeHtml(eventName)}
                        </a>
                    </td>
                    <td class="text-muted">${startTime}</td>
                    <td>${warningBadge}</td>
                    <td class="text-muted">${createdAgo}</td>
                    <td>
                        <button class="btn btn-sm btn-ghost-primary" data-action="expand-detail" data-id="${entry.id}">
                            <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                <polyline points="6 9 12 15 18 9"/>
                            </svg>
                        </button>
                    </td>
                </tr>
            `;
        }).join('');
        
        updateShowingText(entries.length);
    }
    
    /**
     * Expand detail view for an entry
     */
    async function expandDetail(id) {
        // Collapse any currently expanded detail
        if (expandedId) {
            collapseDetail();
        }
        
        const entryRow = document.querySelector(`tr[data-entry-id="${id}"]`);
        if (!entryRow) return;
        
        expandedId = id;
        
        // Show loading state in detail row
        const detailRow = document.createElement('tr');
        detailRow.id = `detail-${id}`;
        detailRow.innerHTML = `
            <td colspan="5" class="p-0">
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
     */
    function renderDetailCard(id, detail) {
        const detailRow = document.getElementById(`detail-${id}`);
        if (!detailRow) return;
        
        const warnings = detail.warnings || [];
        const changes = detail.changes || [];
        const original = detail.original || {};
        const normalized = detail.normalized || {};
        
        // Build warnings HTML
        const warningsHtml = warnings.map(w => {
            const badge = getWarningCodeBadge(w.code);
            return `<div class="mb-2">${badge} ${escapeHtml(w.message)}</div>`;
        }).join('');
        
        // Build changes HTML
        const changesHtml = changes.length > 0 ? `
            <div class="mb-3">
                <strong>Changes Applied:</strong>
                ${changes.map(c => `
                    <div class="mt-2">
                        <strong>${escapeHtml(c.field)}:</strong><br>
                        <span class="text-muted">From:</span> ${escapeHtml(formatDateValue(c.original))}<br>
                        <span class="text-success">To:</span> ${escapeHtml(formatDateValue(c.corrected))}<br>
                        <span class="text-muted small">${escapeHtml(c.reason)}</span>
                    </div>
                `).join('')}
            </div>
        ` : '';
        
        // Build comparison HTML
        const comparisonHtml = `
            <div class="row">
                <div class="col-md-6">
                    <h4>Original Data</h4>
                    ${renderEventData(original, changes, 'original')}
                </div>
                <div class="col-md-6">
                    <h4>Normalized Data</h4>
                    ${renderEventData(normalized, changes, 'normalized')}
                </div>
            </div>
        `;
        
        // Build action buttons (only for pending status)
        const actionButtons = detail.status === 'pending' ? `
            <div class="btn-list" id="action-buttons-${id}">
                <button class="btn btn-success" data-action="approve" data-id="${id}">
                    Approve
                </button>
                <button class="btn btn-primary" data-action="show-fix-form" data-id="${id}">
                    Fix Dates
                </button>
                <button class="btn btn-outline-danger" data-action="reject" data-id="${id}">
                    Reject
                </button>
            </div>
            <div id="fix-form-${id}" style="display: none;">
                <!-- Fix form will be inserted here -->
            </div>
        ` : `
            <div class="text-muted">
                ${detail.status === 'approved' ? 'Approved' : 'Rejected'} by ${escapeHtml(detail.reviewedBy || 'system')} on ${formatDate(detail.reviewedAt)}
                ${detail.reviewNotes ? `<br>Notes: ${escapeHtml(detail.reviewNotes)}` : ''}
                ${detail.rejectionReason ? `<br>Reason: ${escapeHtml(detail.rejectionReason)}` : ''}
            </div>
        `;
        
        detailRow.innerHTML = `
            <td colspan="5" class="p-0">
                <div class="card mb-0">
                    <div class="card-body">
                        <div class="d-flex justify-content-between mb-3">
                            <h3>Review Details</h3>
                            <button class="btn btn-ghost-secondary" data-action="collapse-detail">
                                <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                    <polyline points="6 15 12 9 18 15"/>
                                </svg>
                            </button>
                        </div>
                        
                        ${warningsHtml ? `<div class="mb-3"><strong>Warnings:</strong><br>${warningsHtml}</div>` : ''}
                        ${changesHtml}
                        ${comparisonHtml}
                        
                        <div class="mt-3">
                            ${actionButtons}
                        </div>
                    </div>
                </div>
            </td>
        `;
    }
    
    /**
     * Render event data fields
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
            
            if (typeof value === 'object') {
                value = JSON.stringify(value, null, 2);
            } else if (field.key.includes('Date')) {
                value = formatDateValue(value);
            }
            
            // Check if this field changed
            const changed = changes.find(c => c.field === field.key);
            const highlight = changed ? (type === 'original' ? 'bg-danger-lt' : 'bg-success-lt') : '';
            
            return `
                <div class="mb-2">
                    <strong>${escapeHtml(field.label)}:</strong><br>
                    <span class="${highlight}">${escapeHtml(value)}</span>
                </div>
            `;
        }).join('');
    }
    
    /**
     * Collapse detail view
     */
    function collapseDetail() {
        if (!expandedId) return;
        
        const detailRow = document.getElementById(`detail-${expandedId}`);
        if (detailRow) {
            detailRow.remove();
        }
        
        expandedId = null;
    }
    
    /**
     * Approve entry
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
     * Show reject modal
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
            
            showToast('Entry rejected', 'success');
            
            // Remove from list if filtering by pending
            if (currentFilter === 'pending') {
                removeEntryFromList(id);
            } else {
                // Reload to show updated status
                loadEntries();
            }
        } catch (err) {
            console.error('Failed to reject entry:', err);
            showToast(err.message || 'Failed to reject entry', 'error');
        } finally {
            setLoading(confirmBtn, false);
        }
    }
    
    /**
     * Show fix dates form
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
     * Apply fix dates
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
            // Convert datetime-local to ISO format
            const corrections = {
                startDate: new Date(startValue).toISOString(),
                endDate: new Date(endValue).toISOString()
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
        } else {
            updateShowingText(entries.length);
        }
        
        // Update pending count
        if (currentFilter === 'pending') {
            updatePendingCount(entries.length);
        }
    }
    
    /**
     * Update pending count badge
     */
    function updatePendingCount(count) {
        const badge = document.getElementById('pending-count');
        if (badge) {
            badge.textContent = count;
        }
    }
    
    /**
     * Update pagination controls
     */
    function updatePagination(nextCursor) {
        const pagination = document.getElementById('pagination');
        if (!pagination) return;
        
        if (nextCursor) {
            pagination.innerHTML = `
                <li class="page-item">
                    <a class="page-link" href="#" data-action="next-page" data-cursor="${nextCursor}">
                        Next
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <polyline points="9 6 15 12 9 18"/>
                        </svg>
                    </a>
                </li>
            `;
        } else {
            pagination.innerHTML = '';
        }
    }
    
    /**
     * Navigate to next page
     */
    function goToNextPage(nextCursor) {
        cursor = nextCursor;
        loadEntries();
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    /**
     * Update showing text
     */
    function updateShowingText(count) {
        const showingText = document.getElementById('showing-text');
        if (!showingText) return;
        
        showingText.textContent = count === 0 ? 'No items' : `Showing ${count} items`;
    }
    
    /**
     * UI State Management
     */
    function showLoading() {
        document.getElementById('loading-state').style.display = 'block';
        document.getElementById('empty-state').style.display = 'none';
        document.getElementById('review-queue-container').style.display = 'none';
    }
    
    function showEmptyState() {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('empty-state').style.display = 'block';
        document.getElementById('review-queue-container').style.display = 'none';
    }
    
    function showTable() {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('empty-state').style.display = 'none';
        document.getElementById('review-queue-container').style.display = 'block';
    }
    
    /**
     * Helper: Get warning badge HTML
     */
    function getWarningBadge(warnings) {
        if (!warnings || warnings.length === 0) {
            return '<span class="badge bg-secondary">Unknown</span>';
        }
        
        const firstWarning = warnings[0];
        return getWarningCodeBadge(firstWarning.code);
    }
    
    /**
     * Helper: Get warning code badge
     */
    function getWarningCodeBadge(code) {
        if (code === 'reversed_dates_timezone_likely') {
            return '<span class="badge bg-success">High Confidence</span>';
        } else if (code === 'reversed_dates_corrected_needs_review') {
            return '<span class="badge bg-warning">Low Confidence</span>';
        }
        return '<span class="badge bg-secondary">Unknown</span>';
    }
    
    /**
     * Helper: Get relative time (e.g., "2h ago")
     */
    function getRelativeTime(dateString) {
        if (!dateString) return '-';
        
        const date = new Date(dateString);
        const now = new Date();
        const diffMs = now - date;
        const diffMins = Math.floor(diffMs / 60000);
        const diffHours = Math.floor(diffMins / 60);
        const diffDays = Math.floor(diffHours / 24);
        
        if (diffMins < 60) {
            return `${diffMins}m ago`;
        } else if (diffHours < 24) {
            return `${diffHours}h ago`;
        } else {
            return `${diffDays}d ago`;
        }
    }
    
    /**
     * Helper: Format date value for display
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
     * Helper: Convert ISO date to datetime-local format
     */
    function formatDateTimeLocal(isoString) {
        if (!isoString) return '';
        try {
            const date = new Date(isoString);
            // Format as YYYY-MM-DDTHH:MM (required by datetime-local input)
            const year = date.getFullYear();
            const month = String(date.getMonth() + 1).padStart(2, '0');
            const day = String(date.getDate()).padStart(2, '0');
            const hours = String(date.getHours()).padStart(2, '0');
            const minutes = String(date.getMinutes()).padStart(2, '0');
            return `${year}-${month}-${day}T${hours}:${minutes}`;
        } catch {
            return '';
        }
    }
    
})();
