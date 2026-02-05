// SEL Admin Reusable Components
// Common UI components and utilities

// Toast notifications
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
function formatDate(dateString) {
    if (!dateString) return '-';
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    });
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

// Auto-setup logout on page load
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', setupLogout);
} else {
    setupLogout();
}
