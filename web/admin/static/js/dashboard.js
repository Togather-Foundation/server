// SEL Admin Dashboard JavaScript
(function() {
    'use strict';
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        loadDashboardStats();
        checkMonitoringServices();
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
    
    async function checkMonitoringServices() {
        // Check if monitoring elements exist on the page
        const grafanaLink = document.getElementById('grafana-link');
        
        if (!grafanaLink) {
            return; // Monitoring section not present
        }
        
        const grafanaStatus = document.getElementById('grafana-status');
        
        // Check Grafana availability
        const grafanaAvailable = await checkGrafana(grafanaStatus);
        
        // Show embedded Grafana dashboard if available
        if (grafanaAvailable) {
            const embedContainer = document.getElementById('grafana-embed-container');
            const monitoringHelp = document.getElementById('monitoring-help');
            if (embedContainer) {
                embedContainer.style.display = 'block';
                if (monitoringHelp) {
                    monitoringHelp.style.display = 'none';
                }
            }
        }
    }
    
    async function checkGrafana(statusElement) {
        try {
            // Check Grafana API health endpoint (requires auth)
            // Since we're authenticated as admin, this should succeed
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), 3000);
            
            const response = await fetch('/grafana/api/health', {
                method: 'GET',
                credentials: 'same-origin', // Include cookies for auth
                signal: controller.signal
            });
            
            clearTimeout(timeoutId);
            
            // Check if we got a valid response
            if (response.ok) {
                // Grafana is available and we're authenticated
                statusElement.innerHTML = '<span class="badge bg-success">Available</span>';
                return true;
            } else if (response.status === 401) {
                // Not authenticated to Grafana (shouldn't happen for logged-in admin)
                statusElement.innerHTML = '<span class="badge bg-warning">Auth Required</span>';
                return false;
            } else {
                // Other error
                statusElement.innerHTML = '<span class="badge bg-danger">Error</span>';
                return false;
            }
        } catch (err) {
            // Service is not reachable
            if (err.name === 'AbortError') {
                statusElement.innerHTML = '<span class="badge bg-warning">Timeout</span>';
            } else {
                statusElement.innerHTML = '<span class="badge bg-danger">Unavailable</span>';
            }
            return false;
        }
    }
})();

