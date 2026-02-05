/**
 * Users List Page JavaScript
 * Handles user listing, filtering, pagination, and CRUD operations
 */
(function() {
    'use strict';
    
    // State
    let currentCursor = null;
    let filters = {
        search: '',
        status: '',
        role: ''
    };
    let currentUsers = [];
    let abortController = null; // For cancelling in-flight requests
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        setupEventListeners();
        loadUsers();
    }
    
    /**
     * Setup event listeners for filters and actions
     */
    function setupEventListeners() {
        // Create user button
        const createUserBtn = document.getElementById('create-user-btn');
        if (createUserBtn) {
            createUserBtn.addEventListener('click', () => openUserModal());
        }
        
        // Search input with debounce
        const searchInput = document.getElementById('search-input');
        if (searchInput) {
            searchInput.addEventListener('input', debounce((e) => {
                const query = e.target.value.trim();
                
                // Sanitize input: remove null bytes, limit length
                const sanitized = query.replace(/\0/g, '').substring(0, 100);
                
                // Warn for very short queries
                if (sanitized.length > 0 && sanitized.length < 2) {
                    // Don't search yet, but don't show error either (just wait)
                    return;
                }
                
                filters.search = sanitized;
                currentCursor = null;
                loadUsers();
            }, 300));
        }
        
        // Status filter
        const statusFilter = document.getElementById('status-filter');
        if (statusFilter) {
            statusFilter.addEventListener('change', (e) => {
                filters.status = e.target.value;
                currentCursor = null;
                loadUsers();
            });
        }
        
        // Role filter
        const roleFilter = document.getElementById('role-filter');
        if (roleFilter) {
            roleFilter.addEventListener('change', (e) => {
                filters.role = e.target.value;
                currentCursor = null;
                loadUsers();
            });
        }
        
        // Clear filters button
        const clearFiltersBtn = document.getElementById('clear-filters');
        if (clearFiltersBtn) {
            clearFiltersBtn.addEventListener('click', () => {
                clearFilters();
            });
        }
        
        // User form submit
        const submitBtn = document.getElementById('user-submit-btn');
        if (submitBtn) {
            submitBtn.addEventListener('click', handleUserSubmit);
        }
    }
    
    /**
     * Clear all filters and reload
     */
    function clearFilters() {
        filters = {
            search: '',
            status: '',
            role: ''
        };
        currentCursor = null;
        
        // Reset UI
        document.getElementById('search-input').value = '';
        document.getElementById('status-filter').value = '';
        document.getElementById('role-filter').value = '';
        
        loadUsers();
    }
    
    /**
     * Load users from API with current filters
     */
    async function loadUsers() {
        const tbody = document.getElementById('users-table');
        renderLoadingState(tbody, 6);
        
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
            if (filters.role) params.role = filters.role;
            if (currentCursor) params.cursor = currentCursor;
            
            const data = await API.users.list(params, abortController.signal);
            
            if (data.items && data.items.length > 0) {
                currentUsers = data.items;
                renderUsers(data.items);
                updatePagination(data.next_cursor);
                updateShowingText(data.items.length);
            } else {
                renderEmptyState(tbody, 'No users found. Try adjusting your filters.', 6);
                updateShowingText(0);
                updatePagination(null);
            }
        } catch (error) {
            // Ignore abort errors (expected when cancelling requests)
            if (error.name === 'AbortError') {
                return;
            }
            
            console.error('Failed to load users:', error);
            showToast(error.message || 'Failed to load users', 'error');
            tbody.innerHTML = `
                <tr>
                    <td colspan="6" class="text-center text-danger py-4">
                        Failed to load users: ${escapeHtml(error.message)}
                    </td>
                </tr>
            `;
        }
    }
    
    /**
     * Render users into table
     * @param {Array} users - Array of user objects
     */
    function renderUsers(users) {
        const tbody = document.getElementById('users-table');
        
        tbody.innerHTML = users.map(user => {
            const username = user.username || 'N/A';
            const email = user.email || 'N/A';
            const role = user.role || 'viewer';
            const status = user.status || 'pending';
            const statusColor = getStatusColor(status);
            const roleColor = getRoleColor(role);
            const lastLogin = user.last_login_at ? formatDate(user.last_login_at, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : 'Never';
            const createdAt = user.created_at ? formatDate(user.created_at, { month: 'short', day: 'numeric', year: 'numeric' }) : 'N/A';
            
            // Build action buttons based on status
            let actionButtons = '';
            
            if (status === 'active') {
                actionButtons += `
                    <button class="btn btn-sm btn-warning deactivate-user-btn" data-user-id="${user.id}" data-username="${escapeHtml(username)}">
                        Deactivate
                    </button>
                `;
            } else if (status === 'inactive') {
                actionButtons += `
                    <button class="btn btn-sm btn-success activate-user-btn" data-user-id="${user.id}" data-username="${escapeHtml(username)}">
                        Activate
                    </button>
                `;
            } else if (status === 'pending') {
                actionButtons += `
                    <button class="btn btn-sm btn-info resend-invitation-btn" data-user-id="${user.id}" data-username="${escapeHtml(username)}">
                        Resend Invitation
                    </button>
                `;
            }
            
            actionButtons += `
                <button class="btn btn-sm edit-user-btn" data-user-id="${user.id}">
                    Edit
                </button>
                <a href="/admin/users/${user.id}/activity" class="btn btn-sm">
                    Activity
                </a>
                <button class="btn btn-sm btn-ghost-danger delete-user-btn" data-user-id="${user.id}" data-username="${escapeHtml(username)}">
                    Delete
                </button>
            `;
            
            return `
                <tr data-user-id="${user.id}">
                    <td>
                        <div class="d-flex flex-column">
                            <div class="fw-bold">${escapeHtml(username)}</div>
                            <div class="text-muted small">${escapeHtml(email)}</div>
                        </div>
                    </td>
                    <td>
                        <span class="badge bg-${roleColor}">${escapeHtml(role)}</span>
                    </td>
                    <td>
                        <span class="badge bg-${statusColor}">${escapeHtml(status)}</span>
                    </td>
                    <td class="text-muted">${lastLogin}</td>
                    <td class="text-muted">${createdAt}</td>
                    <td>
                        <div class="btn-list flex-nowrap">
                            ${actionButtons}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
        
        // Wire up action buttons with event delegation
        tbody.querySelectorAll('.edit-user-btn').forEach(btn => {
            btn.addEventListener('click', () => editUser(btn.dataset.userId));
        });
        
        tbody.querySelectorAll('.delete-user-btn').forEach(btn => {
            btn.addEventListener('click', () => deleteUser(btn.dataset.userId, btn.dataset.username));
        });
        
        tbody.querySelectorAll('.activate-user-btn').forEach(btn => {
            btn.addEventListener('click', () => activateUser(btn.dataset.userId, btn.dataset.username));
        });
        
        tbody.querySelectorAll('.deactivate-user-btn').forEach(btn => {
            btn.addEventListener('click', () => deactivateUser(btn.dataset.userId, btn.dataset.username));
        });
        
        tbody.querySelectorAll('.resend-invitation-btn').forEach(btn => {
            btn.addEventListener('click', () => resendInvitation(btn.dataset.userId, btn.dataset.username));
        });
    }
    
    /**
     * Update pagination controls
     * @param {string|null} nextCursor - Next page cursor or null if no more pages
     */
    function updatePagination(nextCursor) {
        const pagination = document.getElementById('pagination');
        
        if (!pagination) return;
        
        const hasNext = !!nextCursor;
        const hasPrev = !!currentCursor;
        
        let html = '';
        
        // Previous page button
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
        
        // Next page button
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
        
        // Wire up pagination event listeners
        const paginationLinks = pagination.querySelectorAll('[data-action]');
        paginationLinks.forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const action = link.dataset.action;
                if (action === 'next') {
                    const cursor = link.dataset.cursor;
                    goToNextPage(cursor);
                } else if (action === 'prev') {
                    goToPreviousPage();
                }
            });
        });
    }
    
    /**
     * Update showing text (e.g., "Showing 1-20 of 50")
     * @param {number} count - Number of items shown
     */
    function updateShowingText(count) {
        const showingText = document.getElementById('showing-text');
        if (!showingText) return;
        
        if (count === 0) {
            showingText.textContent = 'No users found';
        } else {
            showingText.textContent = `Showing ${count} users`;
        }
    }
    
    /**
     * Navigate to next page
     * @param {string} cursor - Next page cursor
     */
    function goToNextPage(cursor) {
        currentCursor = cursor;
        loadUsers();
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    /**
     * Navigate to previous page (reset cursor)
     */
    function goToPreviousPage() {
        currentCursor = null;
        loadUsers();
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    /**
     * Open user modal for create or edit
     * @param {Object|null} user - User object for edit, null for create
     */
    function openUserModal(user = null) {
        const modal = document.getElementById('user-modal');
        const form = document.getElementById('user-form');
        const title = document.getElementById('user-modal-title');
        const submitBtn = document.getElementById('user-submit-btn');
        
        if (!modal || !form) {
            console.error('User modal elements not found');
            return;
        }
        
        // Reset form
        form.reset();
        form.classList.remove('was-validated');
        
        if (user) {
            // Edit mode
            title.textContent = 'Edit User';
            submitBtn.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" class="icon me-1" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                    <path d="M7 7h-1a2 2 0 0 0 -2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2 -2v-1" />
                    <path d="M20.385 6.585a2.1 2.1 0 0 0 -2.97 -2.97l-8.415 8.385v3h3l8.385 -8.415z" />
                    <path d="M16 5l3 3" />
                </svg>
                Update User
            `;
            
            document.getElementById('user-id').value = user.id;
            document.getElementById('user-username').value = user.username;
            document.getElementById('user-email').value = user.email;
            document.getElementById('user-role').value = user.role;
        } else {
            // Create mode
            title.textContent = 'Invite User';
            submitBtn.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" class="icon me-1" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                    <line x1="12" y1="5" x2="12" y2="19"/>
                    <line x1="5" y1="12" x2="19" y2="12"/>
                </svg>
                Send Invitation
            `;
        }
        
        // Show modal
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
    }
    
    /**
     * Handle user form submission
     */
    async function handleUserSubmit() {
        const form = document.getElementById('user-form');
        const submitBtn = document.getElementById('user-submit-btn');
        const modal = document.getElementById('user-modal');
        
        if (!form.checkValidity()) {
            form.classList.add('was-validated');
            return;
        }
        
        const userId = document.getElementById('user-id').value;
        const username = document.getElementById('user-username').value.trim();
        const email = document.getElementById('user-email').value.trim();
        const role = document.getElementById('user-role').value;
        
        const data = { username, email, role };
        
        setLoading(submitBtn, true);
        
        try {
            if (userId) {
                // Update existing user
                await API.users.update(userId, data);
                showToast('User updated successfully', 'success');
            } else {
                // Create new user
                await API.users.create(data);
                showToast('User invited successfully. Invitation email sent.', 'success');
            }
            
            // Close modal
            const bsModal = bootstrap.Modal.getInstance(modal);
            if (bsModal) {
                bsModal.hide();
            }
            
            // Reload users
            loadUsers();
        } catch (error) {
            console.error('Failed to save user:', error);
            showToast(error.message || 'Failed to save user', 'error');
        } finally {
            setLoading(submitBtn, false);
        }
    }
    
    /**
     * Edit user
     * @param {string} userId - User ID
     */
    async function editUser(userId) {
        try {
            const user = await API.users.get(userId);
            openUserModal(user);
        } catch (error) {
            console.error('Failed to load user:', error);
            showToast(error.message || 'Failed to load user', 'error');
        }
    }
    
    /**
     * Delete user with confirmation
     * @param {string} userId - User ID
     * @param {string} username - Username for confirmation message
     */
    function deleteUser(userId, username) {
        confirmAction(
            'Delete User',
            `Are you sure you want to delete user "${username}"? This action cannot be undone.`,
            async () => {
                const row = document.querySelector(`tr[data-user-id="${userId}"]`);
                
                try {
                    await API.users.delete(userId);
                    showToast('User deleted successfully', 'success');
                    
                    // Only animate removal AFTER successful API response
                    if (row) {
                        row.style.opacity = '0';
                        row.style.transition = 'opacity 0.3s';
                        setTimeout(() => {
                            row.remove();
                            
                            // If no more rows, reload to show empty state
                            const tbody = document.getElementById('users-table');
                            if (tbody && tbody.children.length === 0) {
                                loadUsers();
                            }
                        }, 300);
                    }
                } catch (error) {
                    console.error('Failed to delete user:', error);
                    showToast(error.message || 'Failed to delete user', 'error');
                    
                    // If API failed, ensure row is still visible
                    if (row) {
                        row.style.opacity = '1';
                    }
                }
            }
        );
    }
    
    /**
     * Activate user
     * @param {string} userId - User ID
     * @param {string} username - Username for confirmation
     */
    async function activateUser(userId, username) {
        try {
            await API.users.activate(userId);
            showToast(`User "${username}" activated successfully`, 'success');
            loadUsers();
        } catch (error) {
            console.error('Failed to activate user:', error);
            showToast(error.message || 'Failed to activate user', 'error');
        }
    }
    
    /**
     * Deactivate user
     * @param {string} userId - User ID
     * @param {string} username - Username for confirmation
     */
    function deactivateUser(userId, username) {
        confirmAction(
            'Deactivate User',
            `Are you sure you want to deactivate user "${username}"? They will not be able to log in until reactivated.`,
            async () => {
                try {
                    await API.users.deactivate(userId);
                    showToast(`User "${username}" deactivated successfully`, 'success');
                    loadUsers();
                } catch (error) {
                    console.error('Failed to deactivate user:', error);
                    showToast(error.message || 'Failed to deactivate user', 'error');
                }
            }
        );
    }
    
    /**
     * Resend invitation to pending user
     * @param {string} userId - User ID
     * @param {string} username - Username
     */
    async function resendInvitation(userId, username) {
        try {
            await API.users.resendInvitation(userId);
            showToast(`Invitation resent to "${username}"`, 'success');
        } catch (error) {
            console.error('Failed to resend invitation:', error);
            showToast(error.message || 'Failed to resend invitation', 'error');
        }
    }
    
})();
