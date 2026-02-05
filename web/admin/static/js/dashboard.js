// SEL Admin Dashboard JavaScript
(function() {
    'use strict';
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        loadDashboardStats();
    }
    
    async function loadDashboardStats() {
        try {
            // Fetch pending events count
            await loadPendingCount();
            
            // Fetch total events count
            await loadTotalCount();
        } catch (err) {
            console.error('Failed to load dashboard stats:', err);
            showToast('Failed to load dashboard statistics', 'error');
        }
    }
    
    async function loadPendingCount() {
        const pendingElement = document.getElementById('pending-count');
        try {
            const data = await API.events.pending();
            const count = data.items?.length || 0;
            pendingElement.textContent = count;
        } catch (err) {
            console.error('Failed to load pending events:', err);
            pendingElement.innerHTML = '<span class="text-danger" title="' + err.message + '">Error</span>';
            // If unauthorized, might need to re-login
            if (err.message && err.message.includes('authorization')) {
                showToast('Session expired. Please log in again.', 'warning');
                setTimeout(() => window.location.href = '/admin/login', 2000);
            }
            throw err;
        }
    }
    
    async function loadTotalCount() {
        const totalElement = document.getElementById('total-events');
        try {
            // Fetch all events (with a reasonable limit)
            console.log('Loading total events...');
            const data = await API.events.list({ limit: 1000 });
            console.log('Total events response:', data);
            const count = data.items?.length || 0;
            totalElement.textContent = count;
        } catch (err) {
            console.error('Failed to load total events:', err);
            totalElement.innerHTML = '<span class="text-danger" title="' + err.message + '">Error</span>';
            // If unauthorized, might need to re-login
            if (err.message && err.message.includes('authorization')) {
                showToast('Session expired. Please log in again.', 'warning');
                setTimeout(() => window.location.href = '/admin/login', 2000);
            }
            throw err;
        }
    }
})();
