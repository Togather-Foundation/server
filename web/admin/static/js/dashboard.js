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
            // Fetch stats from efficient COUNT endpoint (server-m11c)
            const stats = await API.stats.get();
            
            // Update pending count
            const pendingElement = document.getElementById('pending-count');
            pendingElement.textContent = stats.pending_count || 0;
            
            // Update total events count
            const totalElement = document.getElementById('total-events');
            totalElement.textContent = stats.total_count || 0;
            
        } catch (err) {
            console.error('Failed to load dashboard stats:', err);
            
            // Show error state in UI
            const pendingElement = document.getElementById('pending-count');
            const totalElement = document.getElementById('total-events');
            
            pendingElement.innerHTML = '<span class="text-danger" title="' + err.message + '">Error</span>';
            totalElement.innerHTML = '<span class="text-danger" title="' + err.message + '">Error</span>';
            
            showToast('Failed to load dashboard statistics', 'error');
            
            // If unauthorized, redirect to login
            if (err.message && err.message.includes('authorization')) {
                showToast('Session expired. Please log in again.', 'warning');
                setTimeout(() => window.location.href = '/admin/login', 2000);
            }
        }
    }
})();
