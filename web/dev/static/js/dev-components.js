/**
 * SEL Developer Portal - Shared Components and Utilities
 * Common UI components and utilities for developer portal pages
 */

/**
 * HTML escaping for XSS protection
 * @param {string} text - Text to escape
 * @returns {string} Escaped HTML
 */
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

/**
 * Set loading state on button
 * @param {HTMLElement} element - Button element
 * @param {boolean} loading - Loading state
 * @param {string} [loadingText='Loading...'] - Custom loading text
 */
function setLoading(element, loading, loadingText = 'Loading...') {
    if (loading) {
        element.disabled = true;
        element.dataset.originalText = element.innerHTML;
        element.innerHTML = `<span class="spinner-border spinner-border-sm me-2"></span>${loadingText}`;
    } else {
        element.disabled = false;
        element.innerHTML = element.dataset.originalText;
    }
}

/**
 * Format number with K/M suffixes (compact format)
 * @param {number} num - Number to format
 * @returns {string} Formatted number (e.g., 1.5K, 2.3M)
 */
function formatNumber(num) {
    if (num >= 1000000) {
        return (num / 1000000).toFixed(1) + 'M';
    } else if (num >= 1000) {
        return (num / 1000).toFixed(1) + 'K';
    }
    return num.toString();
}

/**
 * Show toast notification
 * @param {string} message - Toast message
 * @param {string} type - Toast type (success, error, warning, info)
 */
function showToast(message, type = 'success') {
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
    
    const toast = document.createElement('div');
    toast.className = 'toast show';
    toast.setAttribute('role', 'alert');
    toast.innerHTML = `
        <div class="toast-header">
            <span class="badge ${colors[type]} me-2"></span>
            <strong class="me-auto">${type.charAt(0).toUpperCase() + type.slice(1)}</strong>
            <button type="button" class="btn-close" data-bs-dismiss="toast"></button>
        </div>
        <div class="toast-body">${escapeHtml(message)}</div>
    `;
    
    container.appendChild(toast);
    
    // Auto-remove after 5 seconds
    setTimeout(() => {
        toast.classList.remove('show');
        setTimeout(() => toast.remove(), 300);
    }, 5000);
}

/**
 * Setup logout button handler
 * Handles developer logout by calling the logout endpoint and redirecting to login
 */
function setupLogout() {
    setupLogoutButton({
        buttonId: 'logout-btn',
        logoutEndpoint: '/api/v1/dev/logout',
        tokenKey: 'dev_token',
        redirectUrl: '/dev/login'
    });
}

/**
 * Generic logout button setup
 * @param {Object} config - Logout configuration
 * @param {string} config.buttonId - ID of logout button
 * @param {string} config.logoutEndpoint - Logout API endpoint
 * @param {string} config.tokenKey - localStorage key for token
 * @param {string} config.redirectUrl - URL to redirect after logout
 */
function setupLogoutButton(config) {
    const logoutBtn = document.getElementById(config.buttonId);
    if (logoutBtn) {
        logoutBtn.addEventListener('click', async (e) => {
            e.preventDefault();
            try {
                // Call logout endpoint
                await fetch(config.logoutEndpoint, {
                    method: 'POST',
                    credentials: 'include'
                });
            } catch (err) {
                console.error('Logout error:', err);
            }
            // Clear localStorage regardless of API success
            localStorage.removeItem(config.tokenKey);
            // Redirect to login
            window.location.href = config.redirectUrl;
        });
    }
}
