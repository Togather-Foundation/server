/**
 * Events List Page JavaScript
 * Handles event listing, filtering, pagination, and delete operations
 */
(function() {
    'use strict';
    
    // State
    let currentCursor = null;
    let filters = {
        search: '',
        status: '',
        dateFrom: '',
        dateTo: ''
    };
    let currentEvents = [];
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        setupEventListeners();
        loadEvents();
    }
    
    /**
     * Setup event listeners for filters and actions
     */
    function setupEventListeners() {
        // Create event button
        const createEventBtn = document.getElementById('create-event-btn');
        if (createEventBtn) {
            createEventBtn.addEventListener('click', createEvent);
        }
        
        // Search input with debounce
        const searchInput = document.getElementById('search-input');
        if (searchInput) {
            searchInput.addEventListener('input', debounce((e) => {
                filters.search = e.target.value.trim();
                currentCursor = null;
                loadEvents();
            }, 300));
        }
        
        // Status filter
        const statusFilter = document.getElementById('status-filter');
        if (statusFilter) {
            statusFilter.addEventListener('change', (e) => {
                filters.status = e.target.value;
                currentCursor = null;
                loadEvents();
            });
        }
        
        // Date filters
        const dateFromFilter = document.getElementById('date-from-filter');
        if (dateFromFilter) {
            dateFromFilter.addEventListener('change', (e) => {
                filters.dateFrom = e.target.value;
                currentCursor = null;
                loadEvents();
            });
        }
        
        const dateToFilter = document.getElementById('date-to-filter');
        if (dateToFilter) {
            dateToFilter.addEventListener('change', (e) => {
                filters.dateTo = e.target.value;
                currentCursor = null;
                loadEvents();
            });
        }
        
        // Clear filters button
        const clearFiltersBtn = document.getElementById('clear-filters');
        if (clearFiltersBtn) {
            clearFiltersBtn.addEventListener('click', () => {
                clearFilters();
            });
        }
    }
    
    /**
     * Clear all filters and reload
     */
    function clearFilters() {
        filters = {
            search: '',
            status: '',
            dateFrom: '',
            dateTo: ''
        };
        currentCursor = null;
        
        // Reset UI
        document.getElementById('search-input').value = '';
        document.getElementById('status-filter').value = '';
        document.getElementById('date-from-filter').value = '';
        document.getElementById('date-to-filter').value = '';
        
        loadEvents();
    }
    
    /**
     * Load events from API with current filters
     */
    async function loadEvents() {
        const tbody = document.getElementById('events-table');
        renderLoadingState(tbody, 4);
        
        try {
            const params = {
                limit: 50
            };
            
            // Add filters if set
            if (filters.search) params.q = filters.search;
            if (filters.status) params.state = filters.status;
            if (filters.dateFrom) params.startDate = filters.dateFrom;
            if (filters.dateTo) params.endDate = filters.dateTo;
            if (currentCursor) params.after = currentCursor;
            
            const data = await API.events.list(params);
            
            if (data.items && data.items.length > 0) {
                currentEvents = data.items;
                renderEvents(data.items);
                updatePagination(data.next_cursor);
                updateShowingText(data.items.length);
            } else {
                renderEmptyState(tbody, 'No events found. Try adjusting your filters.', 4);
                updateShowingText(0);
                updatePagination(null);
            }
        } catch (error) {
            console.error('Failed to load events:', error);
            showToast(error.message || 'Failed to load events', 'error');
            tbody.innerHTML = `
                <tr>
                    <td colspan="4" class="text-center text-danger py-4">
                        Failed to load events: ${escapeHtml(error.message)}
                    </td>
                </tr>
            `;
        }
    }
    
    /**
     * Extract event ULID from event object
     * The API returns @id as a full URI (e.g., "/api/v1/events/01ABC..."),
     * so we need to extract the 26-character ULID from it.
     * @param {Object} event - Event object from API
     * @returns {string|null} Event ULID or null
     */
    function extractEventId(event) {
        if (!event) return null;
        
        // Try @id first (full URI)
        if (event['@id']) {
            const match = event['@id'].match(/events\/([A-Z0-9]{26})/i);
            if (match) return match[1];
        }
        
        // Fallback to id field
        if (event.id) return event.id;
        
        // Fallback to ulid field
        if (event.ulid) return event.ulid;
        
        return null;
    }
    
    /**
     * Render events into table
     * @param {Array} events - Array of event objects
     */
    function renderEvents(events) {
        const tbody = document.getElementById('events-table');
        
        tbody.innerHTML = events.map(event => {
            const eventId = extractEventId(event);
            const eventName = event.name || 'Untitled Event';
            const startDate = event.start_date ? formatDate(event.start_date, { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : 'No date';
            const lifecycleState = event.lifecycle_state || 'draft';
            const statusColor = getStatusColor(lifecycleState);
            
            if (!eventId) {
                console.warn('Could not extract event ID from event:', event);
            }
            
            return `
                <tr data-event-id="${eventId || ''}">
                    <td>
                        ${eventId ? `<a href="/admin/events/${eventId}" class="text-reset">${escapeHtml(eventName)}</a>` : `<span class="text-muted">${escapeHtml(eventName)} <small>(missing ID)</small></span>`}
                    </td>
                    <td class="text-muted">${startDate}</td>
                    <td>
                        <span class="badge bg-${statusColor}">${escapeHtml(lifecycleState || 'unknown')}</span>
                    </td>
                    <td>
                        <div class="btn-list flex-nowrap">
                            ${eventId ? `<a href="/admin/events/${eventId}" class="btn btn-sm">Edit</a>` : ''}
                            ${eventId ? `<button class="btn btn-sm btn-ghost-danger delete-event-btn" data-event-id="${eventId}" data-event-name="${escapeHtml(eventName)}">Delete</button>` : ''}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
        
        // Wire up delete buttons with event delegation
        const deleteButtons = tbody.querySelectorAll('.delete-event-btn');
        deleteButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const eventId = btn.dataset.eventId;
                const eventName = btn.dataset.eventName;
                deleteEvent(eventId, eventName);
            });
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
            showingText.textContent = 'No events found';
        } else {
            showingText.textContent = `Showing ${count} events`;
        }
    }
    
    /**
     * Navigate to next page
     * @param {string} cursor - Next page cursor
     */
    function goToNextPage(cursor) {
        currentCursor = cursor;
        loadEvents();
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    /**
     * Navigate to previous page (reset cursor)
     */
    function goToPreviousPage() {
        currentCursor = null;
        loadEvents();
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
    
    /**
     * Delete event with confirmation
     * @param {string} eventId - Event ULID
     * @param {string} eventName - Event name for confirmation message
     */
    function deleteEvent(eventId, eventName) {
        // Show confirmation modal
        const modal = document.getElementById('delete-modal');
        const eventNameSpan = document.getElementById('delete-event-name');
        const confirmBtn = document.getElementById('confirm-delete');
        
        if (!modal || !eventNameSpan || !confirmBtn) {
            console.error('Delete modal elements not found');
            return;
        }
        
        // Set event name in modal
        eventNameSpan.textContent = eventName;
        
        // Remove old event listeners by cloning
        const newConfirmBtn = confirmBtn.cloneNode(true);
        confirmBtn.parentNode.replaceChild(newConfirmBtn, confirmBtn);
        
        // Add new event listener
        newConfirmBtn.addEventListener('click', async () => {
            // Close modal
            const bsModal = bootstrap.Modal.getInstance(modal);
            if (bsModal) {
                bsModal.hide();
            }
            
            // Show loading state on button
            setLoading(newConfirmBtn, true);
            
            try {
                await API.events.delete(eventId);
                showToast('Event deleted successfully', 'success');
                
                // Remove row from table with animation
                const row = document.querySelector(`tr[data-event-id="${eventId}"]`);
                if (row) {
                    row.style.opacity = '0';
                    row.style.transition = 'opacity 0.3s';
                    setTimeout(() => {
                        row.remove();
                        
                        // If no more rows, reload to show empty state
                        const tbody = document.getElementById('events-table');
                        if (tbody && tbody.children.length === 0) {
                            loadEvents();
                        }
                    }, 300);
                }
            } catch (error) {
                console.error('Failed to delete event:', error);
                showToast(error.message || 'Failed to delete event', 'error');
            } finally {
                setLoading(newConfirmBtn, false);
            }
        });
        
        // Show modal
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
    }
    
    /**
     * Navigate to create event page
     */
    function createEvent() {
        // For now, redirect to edit page with 'new' as ID
        // Later, this can be implemented as a proper create endpoint
        window.location.href = '/admin/events/new';
    }
    
    /**
     * Get badge color for lifecycle state
     * @param {string} state - Lifecycle state
     * @returns {string} Bootstrap color class
     */
    function getStatusColor(state) {
        const normalized = state?.toLowerCase() || 'unknown';
        const colors = {
            'published': 'success',
            'draft': 'secondary',
            'pending': 'warning',
            'cancelled': 'danger',
            'unknown': 'secondary'
        };
        return colors[normalized] || 'secondary';
    }
    
})();
