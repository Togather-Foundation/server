/**
 * User Activity Page JavaScript
 * Handles displaying user information and activity timeline
 */
(function() {
    'use strict';
    
    // State
    let userId = null;
    let currentCursor = null;
    let filters = {
        eventType: '',
        dateFrom: '',
        dateTo: ''
    };
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        // Extract user ID from URL path: /admin/users/{id}/activity
        const pathParts = window.location.pathname.split('/');
        const userIdIndex = pathParts.indexOf('users') + 1;
        userId = pathParts[userIdIndex];
        
        if (!userId) {
            showToast('User ID not found in URL', 'error');
            return;
        }
        
        setupEventListeners();
        loadUserInfo();
        loadActivityStats();
        loadActivity();
    }
    
    /**
     * Setup event listeners
     */
    function setupEventListeners() {
        // Event type filter
        const eventTypeFilter = document.getElementById('event-type-filter');
        if (eventTypeFilter) {
            eventTypeFilter.addEventListener('change', (e) => {
                filters.eventType = e.target.value;
                currentCursor = null;
                loadActivity();
            });
        }
        
        // Date filters
        const dateFromFilter = document.getElementById('date-from-filter');
        if (dateFromFilter) {
            dateFromFilter.addEventListener('change', (e) => {
                filters.dateFrom = e.target.value;
                currentCursor = null;
                loadActivity();
            });
        }
        
        const dateToFilter = document.getElementById('date-to-filter');
        if (dateToFilter) {
            dateToFilter.addEventListener('change', (e) => {
                filters.dateTo = e.target.value;
                currentCursor = null;
                loadActivity();
            });
        }
        
        // Clear filters button
        const clearFiltersBtn = document.getElementById('clear-filters');
        if (clearFiltersBtn) {
            clearFiltersBtn.addEventListener('click', clearFilters);
        }
    }
    
    /**
     * Clear all filters and reload
     */
    function clearFilters() {
        filters = {
            eventType: '',
            dateFrom: '',
            dateTo: ''
        };
        currentCursor = null;
        
        // Reset UI
        document.getElementById('event-type-filter').value = '';
        document.getElementById('date-from-filter').value = '';
        document.getElementById('date-to-filter').value = '';
        
        loadActivity();
    }
    
    /**
     * Load user information
     */
    async function loadUserInfo() {
        try {
            const user = await API.users.get(userId);
            
            // Update user info card
            const avatar = document.getElementById('user-avatar');
            const name = document.getElementById('user-name');
            const email = document.getElementById('user-email');
            const roleBadge = document.getElementById('user-role-badge');
            const statusBadge = document.getElementById('user-status-badge');
            const lastLogin = document.getElementById('user-last-login');
            
            if (avatar) {
                avatar.textContent = (user.username || '?').substring(0, 2).toUpperCase();
            }
            
            if (name) {
                name.textContent = user.username || 'N/A';
            }
            
            if (email) {
                email.textContent = user.email || 'N/A';
            }
            
            if (roleBadge) {
                const roleColor = getRoleColor(user.role);
                roleBadge.className = `badge bg-${roleColor} me-1`;
                roleBadge.textContent = (user.role || 'viewer').toUpperCase();
            }
            
            if (statusBadge) {
                const statusColor = getStatusColor(user.status);
                statusBadge.className = `badge bg-${statusColor}`;
                statusBadge.textContent = (user.status || 'pending').toUpperCase();
            }
            
            if (lastLogin) {
                if (user.last_login_at) {
                    lastLogin.textContent = formatDate(user.last_login_at, { 
                        month: 'short', 
                        day: 'numeric', 
                        hour: '2-digit', 
                        minute: '2-digit' 
                    });
                } else {
                    lastLogin.textContent = 'Never';
                }
            }
        } catch (error) {
            console.error('Failed to load user info:', error);
            showToast(error.message || 'Failed to load user information', 'error');
        }
    }
    
    /**
     * Load activity statistics
     */
    async function loadActivityStats() {
        // Note: This is a placeholder since the backend doesn't yet provide aggregated stats
        // For now, we'll show zeros or "-" until the backend implements this
        
        const totalLogins = document.getElementById('stat-total-logins');
        const eventsCreated = document.getElementById('stat-events-created');
        const eventsEdited = document.getElementById('stat-events-edited');
        const recentActivity = document.getElementById('stat-recent-activity');
        
        // Set placeholder values
        if (totalLogins) totalLogins.textContent = '-';
        if (eventsCreated) eventsCreated.textContent = '-';
        if (eventsEdited) eventsEdited.textContent = '-';
        if (recentActivity) recentActivity.textContent = '-';
        
        // TODO: Once backend provides stats endpoint, uncomment:
        // try {
        //     const stats = await API.users.getActivityStats(userId);
        //     if (totalLogins) totalLogins.textContent = stats.total_logins || 0;
        //     if (eventsCreated) eventsCreated.textContent = stats.events_created || 0;
        //     if (eventsEdited) eventsEdited.textContent = stats.events_edited || 0;
        //     if (recentActivity) recentActivity.textContent = stats.recent_activity_count || 0;
        // } catch (error) {
        //     console.error('Failed to load activity stats:', error);
        // }
    }
    
    /**
     * Load activity timeline
     */
    async function loadActivity() {
        const activityList = document.getElementById('activity-list');
        
        // Show loading state
        activityList.innerHTML = `
            <div class="list-group-item text-center py-5">
                <div class="spinner-border text-primary" role="status">
                    <span class="visually-hidden">Loading...</span>
                </div>
                <div class="text-muted mt-2">Loading activity...</div>
            </div>
        `;
        
        try {
            const params = {
                limit: 50
            };
            
            // Add filters if set
            if (filters.eventType) params.event_type = filters.eventType;
            if (filters.dateFrom) params.date_from = filters.dateFrom;
            if (filters.dateTo) params.date_to = filters.dateTo;
            if (currentCursor) params.cursor = currentCursor;
            
            const data = await API.users.getActivity(userId, params);
            
            if (data.items && data.items.length > 0) {
                renderActivity(data.items);
                updatePagination(data.next_cursor);
                updateShowingText(data.items.length);
            } else {
                renderEmptyActivity();
                updateShowingText(0);
                updatePagination(null);
            }
        } catch (error) {
            console.error('Failed to load activity:', error);
            
            // Show error or empty state (activity endpoint might return 404 if not yet implemented)
            if (error.message && error.message.includes('404')) {
                renderEmptyActivity('Activity tracking is not yet available.');
            } else {
                activityList.innerHTML = `
                    <div class="list-group-item text-center text-danger py-4">
                        Failed to load activity: ${escapeHtml(error.message)}
                    </div>
                `;
            }
            updateShowingText(0);
        }
    }
    
    /**
     * Render activity items
     * @param {Array} activities - Array of activity objects
     */
    function renderActivity(activities) {
        const activityList = document.getElementById('activity-list');
        
        activityList.innerHTML = activities.map(activity => {
            const timestamp = formatDate(activity.created_at, { 
                month: 'short', 
                day: 'numeric', 
                hour: '2-digit', 
                minute: '2-digit' 
            });
            const icon = getActivityIcon(activity.event_type);
            const color = getActivityColor(activity.event_type);
            const description = activity.description || activity.event_type || 'Activity';
            
            return `
                <div class="list-group-item">
                    <div class="row align-items-center">
                        <div class="col-auto">
                            <span class="avatar avatar-sm bg-${color}-lt">
                                ${icon}
                            </span>
                        </div>
                        <div class="col">
                            <div class="text-truncate">
                                <strong>${escapeHtml(description)}</strong>
                            </div>
                            <div class="text-muted small">${timestamp}</div>
                        </div>
                    </div>
                </div>
            `;
        }).join('');
    }
    
    /**
     * Render empty activity state
     * @param {string} message - Optional custom message
     */
    function renderEmptyActivity(message = 'No activity found for this user.') {
        const activityList = document.getElementById('activity-list');
        
        activityList.innerHTML = `
            <div class="list-group-item text-center text-muted py-5">
                <svg xmlns="http://www.w3.org/2000/svg" class="icon icon-lg mb-2" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                    <path d="M3 12a9 9 0 1 0 18 0a9 9 0 0 0 -18 0"/>
                    <path d="M12 7v5l3 3"/>
                </svg>
                <div>${escapeHtml(message)}</div>
            </div>
        `;
    }
    
    /**
     * Update pagination controls
     * @param {string|null} nextCursor - Next page cursor
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
        
        const paginationLinks = pagination.querySelectorAll('[data-action]');
        paginationLinks.forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const action = link.dataset.action;
                if (action === 'next') {
                    currentCursor = link.dataset.cursor;
                    loadActivity();
                } else if (action === 'prev') {
                    currentCursor = null;
                    loadActivity();
                }
                window.scrollTo({ top: 0, behavior: 'smooth' });
            });
        });
    }
    
    /**
     * Update showing text
     * @param {number} count - Number of items shown
     */
    function updateShowingText(count) {
        const showingText = document.getElementById('showing-text');
        if (!showingText) return;
        
        if (count === 0) {
            showingText.textContent = 'No activity';
        } else {
            showingText.textContent = `Showing ${count} events`;
        }
    }
    
    /**
     * Get icon SVG for activity type
     * @param {string} eventType - Activity event type
     * @returns {string} SVG icon HTML
     */
    function getActivityIcon(eventType) {
        const type = (eventType || '').toLowerCase();
        
        if (type.includes('login')) {
            return '<svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none"><path stroke="none" d="M0 0h24v24H0z" fill="none"/><path d="M14 8v-2a2 2 0 0 0 -2 -2h-7a2 2 0 0 0 -2 2v12a2 2 0 0 0 2 2h7a2 2 0 0 0 2 -2v-2" /><path d="M20 12h-13l3 -3m0 6l-3 -3" /></svg>';
        } else if (type.includes('logout')) {
            return '<svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none"><path stroke="none" d="M0 0h24v24H0z" fill="none"/><path d="M14 8v-2a2 2 0 0 0 -2 -2h-7a2 2 0 0 0 -2 2v12a2 2 0 0 0 2 2h7a2 2 0 0 0 2 -2v-2" /><path d="M7 12h14l-3 -3m0 6l3 -3" /></svg>';
        } else if (type.includes('create')) {
            return '<svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none"><path stroke="none" d="M0 0h24v24H0z" fill="none"/><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>';
        } else if (type.includes('update') || type.includes('edit')) {
            return '<svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none"><path stroke="none" d="M0 0h24v24H0z" fill="none"/><path d="M7 7h-1a2 2 0 0 0 -2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2 -2v-1" /><path d="M20.385 6.585a2.1 2.1 0 0 0 -2.97 -2.97l-8.415 8.385v3h3l8.385 -8.415z" /><path d="M16 5l3 3" /></svg>';
        } else if (type.includes('delete')) {
            return '<svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none"><path stroke="none" d="M0 0h24v24H0z" fill="none"/><line x1="4" y1="7" x2="20" y2="7"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/><path d="M5 7l1 12a2 2 0 0 0 2 2h8a2 2 0 0 0 2 -2l1 -12"/><path d="M9 7v-3a1 1 0 0 1 1 -1h4a1 1 0 0 1 1 1v3"/></svg>';
        }
        
        // Default icon
        return '<svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none"><path stroke="none" d="M0 0h24v24H0z" fill="none"/><circle cx="12" cy="12" r="9"/></svg>';
    }
    
    /**
     * Get color for activity type
     * @param {string} eventType - Activity event type
     * @returns {string} Bootstrap color name
     */
    function getActivityColor(eventType) {
        const type = (eventType || '').toLowerCase();
        
        if (type.includes('login')) return 'success';
        if (type.includes('logout')) return 'secondary';
        if (type.includes('create')) return 'primary';
        if (type.includes('update') || type.includes('edit')) return 'info';
        if (type.includes('delete')) return 'danger';
        
        return 'secondary';
    }
    
    /**
     * Get badge color for status
     * @param {string} status - User status
     * @returns {string} Bootstrap color class
     */
    function getStatusColor(status) {
        const normalized = status?.toLowerCase() || 'unknown';
        const colors = {
            'active': 'success',
            'inactive': 'secondary',
            'pending': 'warning',
            'unknown': 'secondary'
        };
        return colors[normalized] || 'secondary';
    }
    
    /**
     * Get badge color for role
     * @param {string} role - User role
     * @returns {string} Bootstrap color class
     */
    function getRoleColor(role) {
        const normalized = role?.toLowerCase() || 'viewer';
        const colors = {
            'admin': 'danger',
            'editor': 'info',
            'viewer': 'secondary'
        };
        return colors[normalized] || 'secondary';
    }
    
})();
