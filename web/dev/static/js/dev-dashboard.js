// SEL Developer Portal - Dashboard Page
// Displays key count, usage stats, and quick actions

(function() {
    'use strict';
    
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        loadDashboardData();
        setupLogout(); // From components.js
    }
    
    async function loadDashboardData() {
        try {
            // Load API keys to get count
            const response = await DevAPI.apiKeys.list();
            updateKeyCount(response);
            
            // Calculate total usage stats (use items array from response)
            const keys = response.items || response;
            const stats = calculateUsageStats(keys);
            updateUsageStats(stats);
            
        } catch (err) {
            console.error('Failed to load dashboard data:', err);
            showToast('Failed to load dashboard data', 'error');
            
            // Show error state
            document.getElementById('key-count').textContent = '—';
            document.getElementById('key-limit').textContent = 'Error loading data';
            document.getElementById('requests-today').textContent = '—';
            document.getElementById('requests-week').textContent = '—';
            document.getElementById('requests-month').textContent = '—';
        }
    }
    
    function updateKeyCount(response) {
        // Extract items and metadata from response
        const keys = response.items || response;
        const activeKeys = keys.filter(k => k.is_active);
        const keyCount = activeKeys.length;
        
        // Read max_keys from API response, fall back to 5 for backward compatibility
        const maxKeys = response.max_keys || 5;
        
        document.getElementById('key-count').textContent = keyCount;
        
        const limitText = `You have ${keyCount} of ${maxKeys} API key${keyCount !== 1 ? 's' : ''}`;
        document.getElementById('key-limit').textContent = limitText;
    }
    
    function calculateUsageStats(keys) {
        let requestsToday = 0;
        let requestsWeek = 0;
        let requestsMonth = 0;
        
        // Sum up usage across all keys
        keys.forEach(key => {
            requestsToday += key.usage_today || 0;
            requestsWeek += key.usage_7d || 0;
            requestsMonth += key.usage_30d || 0;
        });
        
        return {
            today: requestsToday,
            week: requestsWeek,
            month: requestsMonth
        };
    }
    
    function updateUsageStats(stats) {
        document.getElementById('requests-today').textContent = formatNumber(stats.today);
        document.getElementById('requests-week').textContent = formatNumber(stats.week);
        document.getElementById('requests-month').textContent = formatNumber(stats.month);
    }
})();
