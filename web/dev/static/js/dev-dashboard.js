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
            const keys = await DevAPI.apiKeys.list();
            updateKeyCount(keys);
            
            // Calculate total usage stats
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
    
    function updateKeyCount(keys) {
        const activeKeys = keys.filter(k => k.status === 'active');
        const keyCount = activeKeys.length;
        const maxKeys = 5; // Default limit
        
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
    
    function formatNumber(num) {
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        } else if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
        return num.toString();
    }
})();
