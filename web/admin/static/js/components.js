// SEL Admin Reusable Components
// Common UI components and utilities

// Theme Management
function initTheme() {
    // Load theme from localStorage or default to light
    const savedTheme = localStorage.getItem('admin_theme') || 'light';
    applyTheme(savedTheme);
}

function applyTheme(theme) {
    // Apply theme to document
    document.documentElement.setAttribute('data-bs-theme', theme);
    
    // Save to localStorage
    localStorage.setItem('admin_theme', theme);
}

function toggleTheme() {
    const currentTheme = document.documentElement.getAttribute('data-bs-theme') || 'light';
    const newTheme = currentTheme === 'light' ? 'dark' : 'light';
    applyTheme(newTheme);
}

function setupThemeToggle() {
    // Initialize theme on page load
    initTheme();
    
    // Setup toggle buttons
    const darkToggle = document.getElementById('theme-toggle');
    const lightToggle = document.getElementById('theme-toggle-light');
    
    if (darkToggle) {
        darkToggle.addEventListener('click', (e) => {
            e.preventDefault();
            toggleTheme();
        });
    }
    
    if (lightToggle) {
        lightToggle.addEventListener('click', (e) => {
            e.preventDefault();
            toggleTheme();
        });
    }
}

// Toast notifications
function showToast(message, type = 'success', toastId = null) {
    const container = document.getElementById('toast-container');
    if (!container) {
        console.error('Toast container not found');
        return;
    }
    
    const colors = {
        success: 'bg-success',
        error: 'bg-danger',
        warning: 'bg-warning',
        info: 'bg-info'
    };
    
    // If toastId provided, try to update existing toast
    if (toastId) {
        const existingToast = container.querySelector(`[data-toast-id="${toastId}"]`);
        if (existingToast) {
            const bodyElement = existingToast.querySelector('.toast-body');
            if (bodyElement) {
                bodyElement.textContent = message;
            }
            // Update type/color
            const headerElement = existingToast.querySelector('.toast-header .badge');
            if (headerElement) {
                headerElement.className = `badge ${colors[type]} me-2`;
            }
            const titleElement = existingToast.querySelector('.toast-header strong');
            if (titleElement) {
                titleElement.textContent = type.charAt(0).toUpperCase() + type.slice(1);
            }
            return toastId;
        }
    }
    
    const toast = document.createElement('div');
    toast.className = 'toast show';
    toast.setAttribute('role', 'alert');
    if (toastId) {
        toast.setAttribute('data-toast-id', toastId);
    }
    toast.innerHTML = `
        <div class="toast-header">
            <span class="badge ${colors[type]} me-2"></span>
            <strong class="me-auto">${type.charAt(0).toUpperCase() + type.slice(1)}</strong>
            <button type="button" class="btn-close" data-bs-dismiss="toast"></button>
        </div>
        <div class="toast-body">${escapeHtml(message)}</div>
    `;
    
    container.appendChild(toast);
    
    // Auto-remove after 5 seconds (unless it's a retry toast)
    if (!toastId) {
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 300);
        }, 5000);
    }
    
    return toastId;
}

// Confirmation modal
function confirmAction(title, message, onConfirm) {
    const modal = document.getElementById('confirm-modal');
    if (!modal) {
        console.error('Confirm modal not found');
        return;
    }
    
    modal.querySelector('.modal-title').textContent = title;
    modal.querySelector('.modal-body').textContent = message;
    
    const confirmBtn = modal.querySelector('#confirm-action');
    confirmBtn.onclick = () => {
        onConfirm();
        bootstrap.Modal.getInstance(modal).hide();
    };
    
    new bootstrap.Modal(modal).show();
}

// HTML escaping for XSS protection
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

/**
 * Convert URLs in text to clickable links
 * SECURITY: Must call escapeHtml() BEFORE this function to prevent XSS
 * @param {string} escapedHtml - Already HTML-escaped text containing URLs
 * @returns {string} HTML with linkified URLs (links open in new tab with security attributes)
 */
function linkifyUrls(escapedHtml) {
    if (!escapedHtml) return escapedHtml;
    
    // Regex for URLs (http://, https://, www.)
    // Matches URLs until whitespace or HTML entity
    const urlRegex = /(https?:\/\/[^\s<&]+)|(www\.[^\s<&]+)/gi;
    
    return escapedHtml.replace(urlRegex, (match) => {
        let url = match;
        // Add https:// prefix for www. URLs
        if (match.startsWith('www.')) {
            url = 'https://' + match;
        }
        // Both url and match are safe because they come from already-escaped text
        return `<a href="${url}" target="_blank" rel="noopener noreferrer">${match}</a>`;
    });
}

// Date formatting
function formatDate(dateString, options = null) {
    if (!dateString) return '-';
    const date = new Date(dateString);
    const defaultOptions = {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit'
    };
    return date.toLocaleDateString('en-US', options || defaultOptions);
}

// Copy to clipboard
async function copyToClipboard(text) {
    try {
        await navigator.clipboard.writeText(text);
        showToast('Copied to clipboard', 'success');
    } catch (err) {
        showToast('Failed to copy', 'error');
    }
}

// Loading indicator for buttons
function setLoading(element, loading) {
    if (loading) {
        element.disabled = true;
        element.dataset.originalText = element.innerHTML;
        element.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>Loading...';
    } else {
        element.disabled = false;
        element.innerHTML = element.dataset.originalText;
    }
}

// Debounce utility for search inputs
function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

// Logout handler - call this on page load to setup logout button
function setupLogout() {
    const logoutBtn = document.getElementById('logout-btn');
    if (logoutBtn) {
        logoutBtn.addEventListener('click', async (e) => {
            e.preventDefault();
            try {
                // Call logout endpoint
                await fetch('/api/v1/admin/logout', {
                    method: 'POST',
                    credentials: 'include'
                });
            } catch (err) {
                console.error('Logout error:', err);
            }
            // Clear localStorage regardless of API success
            localStorage.removeItem('admin_token');
            // Redirect to login
            window.location.href = '/admin/login';
        });
    }
}

// Render loading state for table
function renderLoadingState(tbody, colSpan) {
    tbody.innerHTML = `
        <tr>
            <td colspan="${colSpan}" class="text-center py-5">
                <div class="spinner-border text-primary" role="status">
                    <span class="visually-hidden">Loading...</span>
                </div>
                <div class="text-muted mt-2">Loading...</div>
            </td>
        </tr>
    `;
}

// Render empty state for table
function renderEmptyState(tbody, message, colSpan) {
    tbody.innerHTML = `
        <tr>
            <td colspan="${colSpan}" class="text-center text-muted py-5">
                <svg xmlns="http://www.w3.org/2000/svg" class="icon icon-lg mb-2" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                    <circle cx="12" cy="12" r="9"/>
                    <line x1="9" y1="10" x2="9.01" y2="10"/>
                    <line x1="15" y1="10" x2="15.01" y2="10"/>
                    <path d="M9.5 15.25a3.5 3.5 0 0 1 5 0"/>
                </svg>
                <div>${escapeHtml(message)}</div>
            </td>
        </tr>
    `;
}

/**
 * Format number with thousands separators
 * @param {number} num - Number to format
 * @returns {string} Formatted number (e.g., 1,500, 2,300,000)
 */
function formatNumber(num) {
    return new Intl.NumberFormat().format(num);
}

/**
 * Get badge color for user status
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
 * Get badge color for user role
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

// Auto-setup on page load
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        setupThemeToggle();
        setupLogout();
        setupCommitHashCopy();
    });
} else {
    setupThemeToggle();
    setupLogout();
    setupCommitHashCopy();
}

/**
 * Setup commit hash copy to clipboard
 */
function setupCommitHashCopy() {
    document.addEventListener('click', (e) => {
        const target = e.target.closest('[data-action="copy-commit"]');
        if (!target) return;
        
        e.preventDefault();
        e.stopPropagation();
        
        const commit = target.dataset.commit;
        if (!commit) return;
        
        copyToClipboard(commit).then(() => {
            showToast(`Copied commit hash: ${commit}`, 'success');
        }).catch((err) => {
            console.error('Failed to copy commit hash:', err);
            showToast('Failed to copy commit hash', 'error');
        });
    });
}

/**
 * Pagination Component
 * Reusable pagination control for admin pages supporting both cursor-based and offset-based pagination.
 * 
 * Features:
 * - Hides Next button when on last page
 * - Shows Previous button when not on first page
 * - Displays page count (e.g., 'Page 2 of 5') for offset-based pagination
 * - Hides pagination controls for single-page results
 * - Handles dynamic list changes (threshold crossing)
 * - Supports both cursor-based (API standard) and offset-based pagination
 * 
 * Usage:
 * 
 * // Cursor-based pagination (review queue, events list)
 * const pagination = new Pagination({
 *     container: document.getElementById('pagination'),
 *     limit: 50,
 *     mode: 'cursor',
 *     onPageChange: async (cursor, direction) => {
 *         const data = await API.reviewQueue.list({ cursor, limit: 50 });
 *         renderItems(data.items);
 *         pagination.update(data);
 *     }
 * });
 * 
 * // After loading data, update pagination state
 * pagination.update({
 *     items: data.items,
 *     next_cursor: data.next_cursor,
 *     prev_cursor: data.prev_cursor,
 *     total: data.total
 * });
 * 
 * // Offset-based pagination (future use)
 * const pagination = new Pagination({
 *     container: document.getElementById('pagination'),
 *     limit: 25,
 *     mode: 'offset',
 *     onPageChange: async (offset, direction) => {
 *         const data = await API.items.list({ offset, limit: 25 });
 *         renderItems(data.items);
 *         pagination.update(data);
 *     }
 * });
 */
class Pagination {
    /**
     * Create a Pagination component
     * @param {Object} options - Configuration options
     * @param {HTMLElement} options.container - Container element for pagination controls
     * @param {number} options.limit - Number of items per page
     * @param {string} options.mode - Pagination mode: 'cursor' or 'offset'
     * @param {Function} options.onPageChange - Callback when page changes: (cursorOrOffset, direction) => Promise
     * @param {HTMLElement} [options.showingTextElement] - Optional element to display "Showing X items" text
     */
    constructor(options) {
        this.container = options.container;
        this.limit = options.limit;
        this.mode = options.mode || 'cursor';
        this.onPageChange = options.onPageChange;
        this.showingTextElement = options.showingTextElement;
        
        // State
        this.currentCursor = null;
        this.nextCursor = null;
        this.prevCursor = null;
        this.currentOffset = 0;
        this.total = 0;
        this.itemCount = 0;
        
        // Bind methods
        this._handlePageChange = this._handlePageChange.bind(this);
        
        // Setup event listeners
        this._setupEventListeners();
    }
    
    /**
     * Setup event listeners for pagination controls
     * @private
     */
    _setupEventListeners() {
        document.addEventListener('click', (e) => {
            const target = e.target.closest('[data-pagination-action]');
            if (!target) return;
            if (!this.container.contains(target)) return;
            
            e.preventDefault();
            
            const action = target.dataset.paginationAction;
            const value = target.dataset.paginationValue;
            
            this._handlePageChange(action, value);
        });
    }
    
    /**
     * Handle page change events
     * @private
     * @param {string} action - Action type: 'next', 'prev', or 'goto'
     * @param {string} value - Cursor or offset value
     */
    async _handlePageChange(action, value) {
        if (this.mode === 'cursor') {
            if (action === 'next' && this.nextCursor) {
                this.currentCursor = this.nextCursor;
                await this.onPageChange(this.nextCursor, 'next');
                window.scrollTo({ top: 0, behavior: 'smooth' });
            } else if (action === 'prev' && this.prevCursor) {
                this.currentCursor = this.prevCursor;
                await this.onPageChange(this.prevCursor, 'prev');
                window.scrollTo({ top: 0, behavior: 'smooth' });
            }
        } else if (this.mode === 'offset') {
            let newOffset = this.currentOffset;
            
            if (action === 'next') {
                newOffset = this.currentOffset + this.limit;
            } else if (action === 'prev') {
                newOffset = Math.max(0, this.currentOffset - this.limit);
            } else if (action === 'goto' && value !== undefined) {
                newOffset = parseInt(value, 10);
            }
            
            if (newOffset !== this.currentOffset) {
                this.currentOffset = newOffset;
                await this.onPageChange(newOffset, action);
                window.scrollTo({ top: 0, behavior: 'smooth' });
            }
        }
    }
    
    /**
     * Update pagination state and render controls
     * @param {Object} data - Response data from API
     * @param {Array} data.items - Array of items on current page
     * @param {string} [data.next_cursor] - Next page cursor (cursor mode)
     * @param {string} [data.prev_cursor] - Previous page cursor (cursor mode)
     * @param {number} [data.total] - Total number of items (for displaying counts)
     * @param {number} [data.offset] - Current offset (offset mode)
     */
    update(data) {
        this.itemCount = data.items ? data.items.length : 0;
        this.total = data.total || 0;
        
        if (this.mode === 'cursor') {
            this.nextCursor = data.next_cursor || null;
            this.prevCursor = data.prev_cursor || null;
        } else if (this.mode === 'offset') {
            this.currentOffset = data.offset || this.currentOffset;
        }
        
        this._render();
        this._updateShowingText();
    }
    
    /**
     * Reset pagination state (e.g., when changing filters)
     */
    reset() {
        this.currentCursor = null;
        this.nextCursor = null;
        this.prevCursor = null;
        this.currentOffset = 0;
        this.total = 0;
        this.itemCount = 0;
        this._render();
        this._updateShowingText();
    }
    
    /**
     * Render pagination controls
     * @private
     */
    _render() {
        if (!this.container) return;
        
        // Hide pagination if single page or empty
        if (this.mode === 'cursor') {
            // For cursor-based: hide if no next/prev cursors
            if (!this.nextCursor && !this.prevCursor) {
                this.container.innerHTML = '';
                return;
            }
        } else if (this.mode === 'offset') {
            // For offset-based: hide if total items fit on one page
            if (this.total <= this.limit) {
                this.container.innerHTML = '';
                return;
            }
        }
        
        let html = '';
        
        if (this.mode === 'cursor') {
            html = this._renderCursorPagination();
        } else if (this.mode === 'offset') {
            html = this._renderOffsetPagination();
        }
        
        this.container.innerHTML = html;
    }
    
    /**
     * Render cursor-based pagination controls
     * @private
     * @returns {string} HTML string
     */
    _renderCursorPagination() {
        let html = '';
        
        // Previous button
        if (this.prevCursor) {
            html += `
                <li class="page-item">
                    <a class="page-link" href="#" data-pagination-action="prev" data-pagination-value="${escapeHtml(this.prevCursor)}">
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <polyline points="15 6 9 12 15 18"/>
                        </svg>
                        Previous
                    </a>
                </li>
            `;
        }
        
        // Next button
        if (this.nextCursor) {
            html += `
                <li class="page-item">
                    <a class="page-link" href="#" data-pagination-action="next" data-pagination-value="${escapeHtml(this.nextCursor)}">
                        Next
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <polyline points="9 6 15 12 9 18"/>
                        </svg>
                    </a>
                </li>
            `;
        }
        
        return html;
    }
    
    /**
     * Render offset-based pagination controls with page numbers
     * @private
     * @returns {string} HTML string
     */
    _renderOffsetPagination() {
        const totalPages = Math.ceil(this.total / this.limit);
        const currentPage = Math.floor(this.currentOffset / this.limit) + 1;
        
        let html = '';
        
        // Previous button
        if (currentPage > 1) {
            html += `
                <li class="page-item">
                    <a class="page-link" href="#" data-pagination-action="prev">
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <polyline points="15 6 9 12 15 18"/>
                        </svg>
                        Previous
                    </a>
                </li>
            `;
        }
        
        // Page count indicator
        html += `
            <li class="page-item disabled">
                <span class="page-link">Page ${currentPage} of ${totalPages}</span>
            </li>
        `;
        
        // Next button
        if (currentPage < totalPages) {
            html += `
                <li class="page-item">
                    <a class="page-link" href="#" data-pagination-action="next">
                        Next
                        <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                            <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                            <polyline points="9 6 15 12 9 18"/>
                        </svg>
                    </a>
                </li>
            `;
        }
        
        return html;
    }
    
    /**
     * Update "Showing X items" text
     * @private
     */
    _updateShowingText() {
        if (!this.showingTextElement) return;
        
        if (this.itemCount === 0) {
            this.showingTextElement.textContent = 'No items';
        } else if (this.mode === 'offset' && this.total > 0) {
            const start = this.currentOffset + 1;
            const end = Math.min(this.currentOffset + this.itemCount, this.total);
            this.showingTextElement.textContent = `Showing ${start}-${end} of ${this.total} items`;
        } else {
            this.showingTextElement.textContent = `Showing ${this.itemCount} items`;
        }
    }
}
