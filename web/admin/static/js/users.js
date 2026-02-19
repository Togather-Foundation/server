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
    let emailEnabled = true; // Whether server has email configured
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        // Read email enabled status from page data attribute
        var pageEl = document.querySelector('.page[data-email-enabled]');
        if (pageEl) {
            emailEnabled = pageEl.dataset.emailEnabled === 'true';
        }

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
        const showingText = document.getElementById('showing-text');
        
        // Announce loading for screen readers
        if (showingText) {
            showingText.textContent = 'Loading users...';
        }
        
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
     * 
     * SECURITY: All user-controlled data (username, email) is escaped with escapeHtml()
     * before being inserted into HTML attributes and content to prevent XSS attacks.
     * User IDs (UUIDs from backend) are trusted as they're server-generated.
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
                var resendLabel = emailEnabled ? 'Resend Invitation' : 'Resend Invitation (no email)';
                actionButtons += `
                    <button class="btn btn-sm btn-info resend-invitation-btn" data-user-id="${user.id}" data-username="${escapeHtml(username)}"${emailEnabled ? '' : ' title="Email is disabled — invitation will be regenerated but no email sent"'}>
                        ${resendLabel}
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
     * 
     * CURSOR-BASED PAGINATION PATTERN:
     * This implementation uses cursor-based pagination (next_cursor from API) which provides
     * efficient pagination for large datasets but has specific behavior constraints:
     * 
     * - FORWARD navigation: Uses cursor tokens from API (goToNextPage with cursor)
     * - BACKWARD navigation: Limited to "return to first page" (goToPreviousPage resets cursor to null)
     * - No page numbers: Cannot jump to arbitrary pages (e.g., "go to page 5")
     * - No true "Previous": Clicking Prev always goes to page 1, not the actual previous page
     * 
     * Example flow:
     *   Page 1 (cursor=null) → Next → Page 2 (cursor=abc123) → Next → Page 3 (cursor=def456)
     *   If user clicks "Prev" on Page 3, they go to Page 1, NOT Page 2
     * 
     * WHY THIS DESIGN:
     * - Cursor pagination is stateless and doesn't require server to track page history
     * - Works reliably with dynamic data (insertions/deletions don't shift page boundaries)
     * - Scales efficiently to millions of records without offset/limit performance issues
     * 
     * ALTERNATIVE (if true bidirectional pagination needed):
     * - Implement client-side page history stack to track visited cursors
     * - Store: [{cursor: null, page: 1}, {cursor: 'abc123', page: 2}, ...]
     * - goToPreviousPage() would pop from stack instead of resetting to null
     * - Trade-off: More complex state management, memory usage for long navigation sessions
     * 
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
                    goToNextPage(cursor, link);
                } else if (action === 'prev') {
                    goToPreviousPage(link);
                }
            });
        });
    }
    
    /**
     * Update showing text (e.g., "Showing 1-20 of 50")
     * Also announces to screen readers via aria-live region
     * @param {number} count - Number of items shown
     */
    function updateShowingText(count) {
        const showingText = document.getElementById('showing-text');
        if (!showingText) return;
        
        if (count === 0) {
            showingText.textContent = 'No users found';
        } else {
            // Announce loaded count for screen readers
            showingText.textContent = `Loaded ${count} ${count === 1 ? 'user' : 'users'}`;
        }
    }
    
    /**
     * Navigate to next page
     * @param {string} cursor - Next page cursor
     * @param {HTMLElement} button - Clicked pagination button
     */
    function goToNextPage(cursor, button) {
        currentCursor = cursor;
        
        // Disable all pagination buttons during load
        const paginationLinks = document.querySelectorAll('#pagination .page-link');
        paginationLinks.forEach(link => link.classList.add('disabled'));
        
        // Show loading spinner in clicked button
        if (button) {
            setLoading(button, true);
        }
        
        loadUsers().finally(() => {
            // Re-enable pagination buttons after load
            paginationLinks.forEach(link => link.classList.remove('disabled'));
            if (button) {
                setLoading(button, false);
            }
        });
        
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    /**
     * Navigate to previous page (reset cursor to null = first page)
     * 
     * IMPORTANT: This does NOT go to the actual previous page in the sequence!
     * Due to cursor-based pagination, we can only navigate forward with cursors.
     * This function resets to the first page (cursor=null), regardless of which
     * page the user is currently on.
     * 
     * Example: User on page 5 clicks "Prev" → goes to page 1 (not page 4)
     * 
     * See updatePagination() docs for full explanation and alternatives.
     * 
     * @param {HTMLElement} button - Clicked pagination button
     */
    function goToPreviousPage(button) {
        currentCursor = null;
        
        // Disable all pagination buttons during load
        const paginationLinks = document.querySelectorAll('#pagination .page-link');
        paginationLinks.forEach(link => link.classList.add('disabled'));
        
        // Show loading spinner in clicked button
        if (button) {
            setLoading(button, true);
        }
        
        loadUsers().finally(() => {
            // Re-enable pagination buttons after load
            paginationLinks.forEach(link => link.classList.remove('disabled'));
            if (button) {
                setLoading(button, false);
            }
        });
        
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
        
        // Clear validation classes and data attributes
        const usernameInput = document.getElementById('user-username');
        const emailInput = document.getElementById('modal-user-email');
        const modalError = document.getElementById('user-modal-error');
        if (usernameInput) {
            usernameInput.classList.remove('is-invalid', 'is-valid');
            delete usernameInput.dataset.validationSetup;
        }
        if (emailInput) {
            emailInput.classList.remove('is-invalid', 'is-valid');
            delete emailInput.dataset.validationSetup;
        }
        document.getElementById('username-error').textContent = '';
        document.getElementById('modal-email-error').textContent = '';
        if (modalError) {
            modalError.textContent = '';
            modalError.classList.add('d-none');
        }
        
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
            document.getElementById('modal-user-email').value = user.email;
            document.getElementById('user-role').value = user.role;
        } else {
            // Create mode
            title.textContent = 'Invite User';
            var sendLabel = emailEnabled ? 'Send Invitation' : 'Create User (no email)';
            submitBtn.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" class="icon me-1" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                    <line x1="12" y1="5" x2="12" y2="19"/>
                    <line x1="5" y1="12" x2="19" y2="12"/>
                </svg>
                ${sendLabel}
            `;
        }
        
        // Show modal
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
        
        // Setup real-time validation for username input (if not already set up)
        if (usernameInput && !usernameInput.dataset.validationSetup) {
            usernameInput.dataset.validationSetup = 'true';
            usernameInput.addEventListener('input', function() {
                const username = this.value.trim();
                const error = validateUsername(username);
                const errorDiv = document.getElementById('username-error');
                
                if (error && username.length > 0) {
                    this.classList.add('is-invalid');
                    this.classList.remove('is-valid');
                    errorDiv.textContent = error;
                } else if (username.length > 0) {
                    this.classList.remove('is-invalid');
                    this.classList.add('is-valid');
                    errorDiv.textContent = '';
                } else {
                    this.classList.remove('is-invalid', 'is-valid');
                    errorDiv.textContent = '';
                }
            });
        }
        
        // Setup real-time validation for email input (if not already set up)
        if (emailInput && !emailInput.dataset.validationSetup) {
            emailInput.dataset.validationSetup = 'true';
            emailInput.addEventListener('input', function() {
                const email = this.value.trim();
                const error = validateEmail(email);
                const errorDiv = document.getElementById('modal-email-error');
                
                if (error && email.length > 0) {
                    this.classList.add('is-invalid');
                    this.classList.remove('is-valid');
                    errorDiv.textContent = error;
                } else if (email.length > 0) {
                    this.classList.remove('is-invalid');
                    this.classList.add('is-valid');
                    errorDiv.textContent = '';
                } else {
                    this.classList.remove('is-invalid', 'is-valid');
                    errorDiv.textContent = '';
                }
            });
        }
    }
    
    /**
     * Validate username format
     * @param {string} username - Username to validate
     * @returns {string|null} - Error message if invalid, null if valid
     */
    function validateUsername(username) {
        // Match backend validation: alphanum (letters and numbers only), 3-50 chars
        if (!username || username.length < 3) {
            return 'Username must be at least 3 characters';
        }
        if (username.length > 50) {
            return 'Username must not exceed 50 characters';
        }
        const pattern = /^[a-zA-Z0-9]+$/;
        if (!pattern.test(username)) {
            return 'Username must contain only letters and numbers';
        }
        return null;
    }
    
    /**
     * Validate email format
     * @param {string} email - Email to validate
     * @returns {string|null} - Error message if invalid, null if valid
     */
    function validateEmail(email) {
        if (!email || email.trim() === '') {
            return 'Email is required';
        }
        
        // Basic email format validation (more strict than HTML5's type="email")
        // Matches: user@domain.tld (allows subdomains, hyphens, underscores)
        const pattern = /^[a-zA-Z0-9._+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
        if (!pattern.test(email)) {
            return 'Please enter a valid email address (e.g., user@example.com)';
        }
        
        // Check for common mistakes
        if (email.includes('..') || email.startsWith('.') || email.endsWith('.')) {
            return 'Email cannot have consecutive dots or start/end with a dot';
        }
        
        if (email.length > 254) {
            return 'Email address is too long (max 254 characters)';
        }
        
        return null;
    }
    
    /**
     * Handle user form submission
     */
    async function handleUserSubmit() {
        const form = document.getElementById('user-form');
        const submitBtn = document.getElementById('user-submit-btn');
        const modal = document.getElementById('user-modal');
        
        const userId = document.getElementById('user-id').value;
        const username = document.getElementById('user-username').value.trim();
        const email = document.getElementById('modal-user-email').value.trim();
        const role = document.getElementById('user-role').value;
        
        // Client-side username validation
        const usernameError = validateUsername(username);
        if (usernameError) {
            const usernameInput = document.getElementById('user-username');
            const usernameErrorDiv = document.getElementById('username-error');
            
            usernameInput.classList.add('is-invalid');
            usernameErrorDiv.textContent = usernameError;
            form.classList.add('was-validated');
            
            showToast(usernameError, 'error');
            return;
        }
        
        // Client-side email validation
        const emailError = validateEmail(email);
        if (emailError) {
            const emailInput = document.getElementById('modal-user-email');
            const emailErrorDiv = document.getElementById('modal-email-error');
            
            emailInput.classList.add('is-invalid');
            emailErrorDiv.textContent = emailError;
            form.classList.add('was-validated');
            
            showToast(emailError, 'error');
            return;
        }
        
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
                if (emailEnabled) {
                    showToast('User invited successfully. Invitation email sent.', 'success');
                } else {
                    showToast('User created successfully. Email is disabled — no invitation was sent.', 'warning');
                }
            }
            
            // Close modal
            const bsModal = bootstrap.Modal.getInstance(modal);
            if (bsModal) {
                bsModal.hide();
            }
            
            // Remove any lingering modal backdrops
            setTimeout(() => {
                const backdrop = document.querySelector('.modal-backdrop');
                if (backdrop) {
                    backdrop.remove();
                }
            }, 300);
            
            // Reload users list
            await loadUsers();
        } catch (error) {
            console.error('Failed to save user:', error);
            var errorMsg = error.message || 'Failed to save user';
            
            // Show error inline in the modal so it's visible above the backdrop
            var modalError = document.getElementById('user-modal-error');
            if (modalError) {
                modalError.textContent = errorMsg;
                modalError.classList.remove('d-none');
            } else {
                // Fallback to toast if modal error element not found
                showToast(errorMsg, 'error');
            }
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
        let retryToastId = null;
        
        try {
            await API.users.activate(userId, (attempt, maxAttempts, delay) => {
                // Show "Retrying..." toast on subsequent attempts
                const message = `Network error. Retrying... (${attempt}/${maxAttempts - 1})`;
                if (!retryToastId) {
                    retryToastId = `retry-${Date.now()}`;
                }
                showToast(message, 'warning', retryToastId);
            });
            
            // Clear retry toast on success
            if (retryToastId) {
                const toast = document.querySelector(`[data-toast-id="${retryToastId}"]`);
                if (toast) toast.remove();
            }
            
            showToast(`User "${username}" activated successfully`, 'success');
            loadUsers();
        } catch (error) {
            console.error('Failed to activate user:', error);
            
            // Clear retry toast on final failure
            if (retryToastId) {
                const toast = document.querySelector(`[data-toast-id="${retryToastId}"]`);
                if (toast) toast.remove();
            }
            
            // Show final error with connection hint
            const message = error.message || 'Failed to activate user';
            const hint = error.status >= 500 || !error.status 
                ? ' Check your connection and try again.' 
                : '';
            showToast(message + hint, 'error');
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
                let retryToastId = null;
                
                try {
                    await API.users.deactivate(userId, (attempt, maxAttempts, delay) => {
                        // Show "Retrying..." toast on subsequent attempts
                        const message = `Network error. Retrying... (${attempt}/${maxAttempts - 1})`;
                        if (!retryToastId) {
                            retryToastId = `retry-${Date.now()}`;
                        }
                        showToast(message, 'warning', retryToastId);
                    });
                    
                    // Clear retry toast on success
                    if (retryToastId) {
                        const toast = document.querySelector(`[data-toast-id="${retryToastId}"]`);
                        if (toast) toast.remove();
                    }
                    
                    showToast(`User "${username}" deactivated successfully`, 'success');
                    loadUsers();
                } catch (error) {
                    console.error('Failed to deactivate user:', error);
                    
                    // Clear retry toast on final failure
                    if (retryToastId) {
                        const toast = document.querySelector(`[data-toast-id="${retryToastId}"]`);
                        if (toast) toast.remove();
                    }
                    
                    // Show final error with connection hint
                    const message = error.message || 'Failed to deactivate user';
                    const hint = error.status >= 500 || !error.status 
                        ? ' Check your connection and try again.' 
                        : '';
                    showToast(message + hint, 'error');
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
        let retryToastId = null;
        
        try {
            await API.users.resendInvitation(userId, (attempt, maxAttempts, delay) => {
                // Show "Retrying..." toast on subsequent attempts
                const message = `Network error. Retrying... (${attempt}/${maxAttempts - 1})`;
                if (!retryToastId) {
                    retryToastId = `retry-${Date.now()}`;
                }
                showToast(message, 'warning', retryToastId);
            });
            
            // Clear retry toast on success
            if (retryToastId) {
                const toast = document.querySelector(`[data-toast-id="${retryToastId}"]`);
                if (toast) toast.remove();
            }
            
            showToast(`Invitation resent to "${username}"${emailEnabled ? '' : ' (email disabled — no email sent)'}`, emailEnabled ? 'success' : 'warning');
        } catch (error) {
            console.error('Failed to resend invitation:', error);
            
            // Clear retry toast on final failure
            if (retryToastId) {
                const toast = document.querySelector(`[data-toast-id="${retryToastId}"]`);
                if (toast) toast.remove();
            }
            
            // Show final error with connection hint
            const message = error.message || 'Failed to resend invitation';
            const hint = error.status >= 500 || !error.status 
                ? ' Check your connection and try again.' 
                : '';
            showToast(message + hint, 'error');
        }
    }
    
})();
