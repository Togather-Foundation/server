/**
 * Developers List Page JavaScript
 * Handles developer listing, filtering, pagination, invitation, and management
 */
(function() {
    'use strict';
    
    // State
    let currentCursor = null;
    let filters = {
        search: '',
        status: ''
    };
    let currentDevelopers = [];
    let abortController = null;
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        setupEventListeners();
        loadDevelopers();
    }
    
    /**
     * Setup event listeners for filters and actions
     */
    function setupEventListeners() {
        // Invite developer button
        const inviteBtn = document.getElementById('invite-developer-btn');
        if (inviteBtn) {
            inviteBtn.addEventListener('click', () => openInviteModal());
        }
        
        // Search input with debounce
        const searchInput = document.getElementById('search-input');
        if (searchInput) {
            searchInput.addEventListener('input', debounce((e) => {
                const query = e.target.value.trim();
                const sanitized = query.replace(/\0/g, '').substring(0, 100);
                filters.search = sanitized;
                currentCursor = null;
                loadDevelopers();
            }, 300));
        }
        
        // Status filter
        const statusFilter = document.getElementById('status-filter');
        if (statusFilter) {
            statusFilter.addEventListener('change', (e) => {
                filters.status = e.target.value;
                currentCursor = null;
                loadDevelopers();
            });
        }
        
        // Clear filters button
        const clearFiltersBtn = document.getElementById('clear-filters');
        if (clearFiltersBtn) {
            clearFiltersBtn.addEventListener('click', clearFilters);
        }
        
        // Invite form submit
        const inviteForm = document.getElementById('invite-developer-form');
        if (inviteForm) {
            inviteForm.addEventListener('submit', handleInviteSubmit);
        }
        
        // Edit form submit
        const editForm = document.getElementById('edit-developer-form');
        if (editForm) {
            editForm.addEventListener('submit', handleEditSubmit);
        }
    }
    
    /**
     * Clear all filters and reload
     */
    function clearFilters() {
        filters = {
            search: '',
            status: ''
        };
        currentCursor = null;
        
        // Reset UI
        document.getElementById('search-input').value = '';
        document.getElementById('status-filter').value = '';
        
        loadDevelopers();
    }
    
    /**
     * Load developers from API with current filters
     */
    async function loadDevelopers() {
        const tbody = document.getElementById('developers-table');
        const showingText = document.getElementById('showing-text');
        
        if (showingText) {
            showingText.textContent = 'Loading developers...';
        }
        
        renderLoadingState(tbody, 7);
        
        // Cancel any in-flight request
        if (abortController) {
            abortController.abort();
        }
        abortController = new AbortController();
        
        try {
            const params = {
                limit: 50
            };
            
            // Add filters if set
            if (filters.search) params.search = filters.search;
            if (filters.status) params.status = filters.status;
            if (currentCursor) params.offset = currentCursor;
            
            const data = await API.developers.list(params, abortController.signal);
            
            if (data.items && data.items.length > 0) {
                currentDevelopers = data.items;
                renderDevelopers(data.items);
                updatePagination(data.next_cursor);
                updateShowingText(data.items.length, data.total);
            } else {
                renderEmptyState(tbody, 'No developers found. Try adjusting your filters.', 7);
                updateShowingText(0, 0);
                updatePagination(null);
            }
        } catch (error) {
            if (error.name === 'AbortError') {
                return;
            }
            
            console.error('Failed to load developers:', error);
            showToast(error.message || 'Failed to load developers', 'error');
            tbody.innerHTML = `
                <tr>
                    <td colspan="7" class="text-center text-danger py-4">
                        Failed to load developers: ${escapeHtml(error.message)}
                    </td>
                </tr>
            `;
        }
    }
    
    /**
     * Render developers into table
     */
    function renderDevelopers(developers) {
        const tbody = document.getElementById('developers-table');
        
        tbody.innerHTML = developers.map(dev => {
            const name = dev.name || 'N/A';
            const email = dev.email || 'N/A';
            const github = dev.github_username ? `@${escapeHtml(dev.github_username)}` : '—';
            const keyCount = dev.key_count || 0;
            const maxKeys = dev.max_keys || 5;
            const requests = dev.requests_last_30d || 0;
            const status = dev.status || 'invited';
            const statusColor = getStatusColor(status);
            const createdAt = dev.created_at ? formatDate(dev.created_at, { month: 'short', day: 'numeric', year: 'numeric' }) : 'N/A';
            
            // Build action buttons based on status
            let actionButtons = '';
            
            actionButtons += `
                <button class="btn btn-sm view-developer-btn" data-developer-id="${dev.id}">
                    View
                </button>
                <button class="btn btn-sm edit-developer-btn" data-developer-id="${dev.id}">
                    Edit
                </button>
            `;
            
            if (status !== 'deactivated') {
                actionButtons += `
                    <button class="btn btn-sm btn-ghost-danger deactivate-developer-btn" data-developer-id="${dev.id}" data-developer-email="${escapeHtml(email)}">
                        Deactivate
                    </button>
                `;
            }
            
            return `
                <tr data-developer-id="${dev.id}">
                    <td>
                        <div class="d-flex flex-column">
                            <div class="fw-bold">${escapeHtml(name)}</div>
                            <div class="text-muted small">${escapeHtml(email)}</div>
                        </div>
                    </td>
                    <td class="text-muted">${github}</td>
                    <td>
                        <span class="badge ${keyCount >= maxKeys ? 'bg-warning' : 'bg-secondary-lt'}">${keyCount}/${maxKeys}</span>
                    </td>
                    <td class="text-muted">${formatNumber(requests)}</td>
                    <td>
                        <span class="badge bg-${statusColor}">${escapeHtml(status)}</span>
                    </td>
                    <td class="text-muted">${createdAt}</td>
                    <td>
                        <div class="btn-list flex-nowrap">
                            ${actionButtons}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
        
        // Wire up action buttons
        tbody.querySelectorAll('.view-developer-btn').forEach(btn => {
            btn.addEventListener('click', () => viewDeveloper(btn.dataset.developerId));
        });
        
        tbody.querySelectorAll('.edit-developer-btn').forEach(btn => {
            btn.addEventListener('click', () => editDeveloper(btn.dataset.developerId));
        });
        
        tbody.querySelectorAll('.deactivate-developer-btn').forEach(btn => {
            btn.addEventListener('click', () => deactivateDeveloper(btn.dataset.developerId, btn.dataset.developerEmail));
        });
    }
    
    /**
     * Get status badge color
     */
    function getStatusColor(status) {
        switch (status) {
            case 'active': return 'success';
            case 'invited': return 'info';
            case 'deactivated': return 'secondary';
            default: return 'secondary';
        }
    }
    
    /**
     * Update pagination controls
     */
    function updatePagination(nextCursor) {
        const pagination = document.getElementById('pagination');
        if (!pagination) return;
        
        const hasNext = !!nextCursor;
        const hasPrev = !!currentCursor;
        
        let html = '';
        
        if (hasPrev) {
            html += `
                <li class="page-item">
                    <a class="page-link" href="#" data-action="prev">
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <polyline points="15 6 9 12 15 18"/>
                        </svg>
                        Prev
                    </a>
                </li>
            `;
        }
        
        if (hasNext) {
            html += `
                <li class="page-item">
                    <a class="page-link" href="#" data-action="next" data-cursor="${nextCursor}">
                        Next
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <polyline points="9 6 15 12 9 18"/>
                        </svg>
                    </a>
                </li>
            `;
        }
        
        pagination.innerHTML = html;
        
        pagination.querySelectorAll('[data-action]').forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const action = link.dataset.action;
                if (action === 'next') {
                    goToNextPage(link.dataset.cursor);
                } else if (action === 'prev') {
                    goToPreviousPage();
                }
            });
        });
    }
    
    /**
     * Update showing text
     */
    function updateShowingText(count, total) {
        const showingText = document.getElementById('showing-text');
        if (!showingText) return;
        
        if (count === 0) {
            showingText.textContent = 'No developers found';
        } else {
            showingText.textContent = `Showing ${count} of ${total} ${total === 1 ? 'developer' : 'developers'}`;
        }
    }
    
    /**
     * Navigate to next page
     */
    function goToNextPage(cursor) {
        currentCursor = cursor;
        loadDevelopers();
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    /**
     * Navigate to previous page
     */
    function goToPreviousPage() {
        currentCursor = null;
        loadDevelopers();
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    /**
     * Open invite developer modal
     */
    function openInviteModal() {
        const modal = document.getElementById('invite-developer-modal');
        const form = document.getElementById('invite-developer-form');
        
        if (!modal || !form) return;
        
        // Reset form
        form.reset();
        form.classList.remove('was-validated');
        
        // Show modal
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
    }
    
    /**
     * Handle invite form submission
     */
    async function handleInviteSubmit(e) {
        e.preventDefault();
        
        const form = document.getElementById('invite-developer-form');
        const submitBtn = document.getElementById('invite-submit-btn');
        const modal = document.getElementById('invite-developer-modal');
        
        const email = document.getElementById('developer-email').value.trim();
        const name = document.getElementById('developer-name').value.trim();
        const maxKeys = parseInt(document.getElementById('developer-max-keys').value) || 5;
        
        // Validate email
        if (!email || !email.includes('@')) {
            showToast('Please enter a valid email address', 'error');
            return;
        }
        
        setLoading(submitBtn, true);
        
        try {
            await API.developers.invite({ email, name, max_keys: maxKeys });
            
            showToast('Developer invitation sent successfully', 'success');
            
            // Close modal
            const bsModal = bootstrap.Modal.getInstance(modal);
            if (bsModal) {
                bsModal.hide();
            }
            
            // Reload developers list
            await loadDevelopers();
        } catch (error) {
            console.error('Failed to invite developer:', error);
            showToast(error.message || 'Failed to invite developer', 'error');
        } finally {
            setLoading(submitBtn, false);
        }
    }
    
    /**
     * View developer details
     */
    async function viewDeveloper(developerId) {
        const modal = document.getElementById('developer-detail-modal');
        const content = document.getElementById('developer-detail-content');
        
        if (!modal || !content) return;
        
        content.innerHTML = '<div class="text-center py-4">Loading...</div>';
        
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
        
        try {
            const dev = await API.developers.get(developerId);
            
            // Render developer details
            const keys = dev.keys || [];
            const keysHtml = keys.length > 0 ? keys.map(key => `
                <tr>
                    <td>${escapeHtml(key.name)}</td>
                    <td><code>${escapeHtml(key.prefix)}...</code></td>
                    <td><span class="badge ${key.is_active ? 'bg-success' : 'bg-secondary'}">${key.is_active ? 'Active' : 'Inactive'}</span></td>
                    <td>${formatNumber(key.usage_30d)}</td>
                    <td>${key.last_used_at ? formatDate(key.last_used_at, { month: 'short', day: 'numeric' }) : 'Never'}</td>
                </tr>
            `).join('') : '<tr><td colspan="5" class="text-center text-muted">No API keys</td></tr>';
            
            content.innerHTML = `
                <div class="row mb-3">
                    <div class="col-md-6">
                        <label class="form-label">Email</label>
                        <div class="fw-bold">${escapeHtml(dev.email)}</div>
                    </div>
                    <div class="col-md-6">
                        <label class="form-label">Name</label>
                        <div class="fw-bold">${escapeHtml(dev.name || 'N/A')}</div>
                    </div>
                </div>
                <div class="row mb-3">
                    <div class="col-md-6">
                        <label class="form-label">Status</label>
                        <div><span class="badge bg-${getStatusColor(dev.status)}">${escapeHtml(dev.status)}</span></div>
                    </div>
                    <div class="col-md-6">
                        <label class="form-label">Max Keys</label>
                        <div class="fw-bold">${dev.max_keys} (using ${dev.key_count})</div>
                    </div>
                </div>
                <div class="row mb-3">
                    <div class="col-md-6">
                        <label class="form-label">Created</label>
                        <div>${formatDate(dev.created_at, { month: 'short', day: 'numeric', year: 'numeric' })}</div>
                    </div>
                    <div class="col-md-6">
                        <label class="form-label">Last Login</label>
                        <div>${dev.last_login_at ? formatDate(dev.last_login_at, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : 'Never'}</div>
                    </div>
                </div>
                <div class="mb-3">
                    <label class="form-label">Requests (Last 30 Days)</label>
                    <div class="fw-bold">${formatNumber(dev.requests_last_30d || 0)}</div>
                </div>
                <hr>
                <h4>API Keys</h4>
                <div class="table-responsive">
                    <table class="table table-sm">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Prefix</th>
                                <th>Status</th>
                                <th>Usage (30d)</th>
                                <th>Last Used</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${keysHtml}
                        </tbody>
                    </table>
                </div>
            `;
        } catch (error) {
            console.error('Failed to load developer:', error);
            content.innerHTML = `<div class="alert alert-danger">Failed to load developer details: ${escapeHtml(error.message)}</div>`;
        }
    }
    
    /**
     * Edit developer
     */
    async function editDeveloper(developerId) {
        const modal = document.getElementById('edit-developer-modal');
        const form = document.getElementById('edit-developer-form');
        
        if (!modal || !form) return;
        
        try {
            const dev = await API.developers.get(developerId);
            
            // Populate form
            document.getElementById('edit-developer-id').value = dev.id;
            document.getElementById('edit-developer-email').value = dev.email;
            document.getElementById('edit-developer-max-keys').value = dev.max_keys;
            document.getElementById('edit-developer-active').value = dev.is_active ? 'true' : 'false';
            
            // Show modal
            const bsModal = new bootstrap.Modal(modal);
            bsModal.show();
        } catch (error) {
            console.error('Failed to load developer:', error);
            showToast(error.message || 'Failed to load developer', 'error');
        }
    }
    
    /**
     * Handle edit form submission
     */
    async function handleEditSubmit(e) {
        e.preventDefault();
        
        const submitBtn = document.getElementById('edit-submit-btn');
        const modal = document.getElementById('edit-developer-modal');
        
        const developerId = document.getElementById('edit-developer-id').value;
        const maxKeys = parseInt(document.getElementById('edit-developer-max-keys').value);
        const isActive = document.getElementById('edit-developer-active').value === 'true';
        
        setLoading(submitBtn, true);
        
        try {
            await API.developers.update(developerId, { max_keys: maxKeys, is_active: isActive });
            
            showToast('Developer updated successfully', 'success');
            
            // Close modal
            const bsModal = bootstrap.Modal.getInstance(modal);
            if (bsModal) {
                bsModal.hide();
            }
            
            // Reload developers list
            await loadDevelopers();
        } catch (error) {
            console.error('Failed to update developer:', error);
            showToast(error.message || 'Failed to update developer', 'error');
        } finally {
            setLoading(submitBtn, false);
        }
    }
    
    /**
     * Deactivate developer
     */
    function deactivateDeveloper(developerId, developerEmail) {
        confirmAction(
            'Deactivate Developer',
            `Are you sure you want to deactivate "${developerEmail}"? This will revoke all their API keys.`,
            async () => {
                try {
                    await API.developers.delete(developerId);
                    
                    showToast('Developer deactivated successfully', 'success');
                    
                    // Remove row from table
                    const row = document.querySelector(`tr[data-developer-id="${developerId}"]`);
                    if (row) {
                        row.style.opacity = '0';
                        row.style.transition = 'opacity 0.3s';
                        setTimeout(() => {
                            row.remove();
                            
                            // Reload if empty
                            const tbody = document.getElementById('developers-table');
                            if (tbody && tbody.children.length === 0) {
                                loadDevelopers();
                            }
                        }, 300);
                    }
                } catch (error) {
                    console.error('Failed to deactivate developer:', error);
                    showToast(error.message || 'Failed to deactivate developer', 'error');
                }
            }
        );
    }
    
})();
