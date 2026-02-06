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

// Date formatting
function formatDate(dateString, options = null) {
    if (!dateString) return '-';
    const date = new Date(dateString);
    const defaultOptions = {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
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
    });
} else {
    setupThemeToggle();
    setupLogout();
}
