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
            pendingElement.innerHTML = '<span class="text-danger">Error</span>';
            throw err;
        }
    }
    
    async function loadTotalCount() {
        const totalElement = document.getElementById('total-events');
        try {
            // Fetch all events (with a reasonable limit)
            const data = await API.events.list({ limit: 1000 });
            const count = data.items?.length || 0;
            totalElement.textContent = count;
        } catch (err) {
            totalElement.innerHTML = '<span class="text-danger">Error</span>';
            throw err;
        }
    }
})();
