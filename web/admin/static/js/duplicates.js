/**
 * Duplicates Review Page JavaScript
 * 
 * Duplicate detection is now integrated into the Review Queue.
 * Events flagged as potential duplicates appear in the review queue with
 * 'potential_duplicate', 'place_possible_duplicate', or 'org_possible_duplicate' warnings.
 * 
 * This page redirects to the review queue for a unified workflow.
 */
(function() {
    'use strict';
    
    // State management
    let duplicatePairs = [];
    let currentPairIndex = 0;
    let mergeSelection = null;
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        // Redirect to review queue — duplicates are now handled there
        window.location.href = '/admin/review-queue';
        return;
    }
    
    function setupEventListeners() {
        // Event delegation for data-action buttons
        document.addEventListener('click', (e) => {
            const target = e.target.closest('[data-action]');
            if (!target) return;
            
            const action = target.dataset.action;
            
            switch(action) {
                case 'navigate-prev':
                    navigatePrevious();
                    break;
                case 'navigate-next':
                    navigateNext();
                    break;
                case 'skip-pair':
                    skipPair();
                    break;
                case 'merge-keep-a':
                    showMergeModal('a');
                    break;
                case 'merge-keep-b':
                    showMergeModal('b');
                    break;
                case 'confirm-merge':
                    confirmMerge();
                    break;
            }
        });
    }

    /**
     * Load duplicate candidates from API
     */
    async function loadDuplicates() {
        showLoading();
        
        try {
            const data = await API.duplicates.list();
            
            // Handle different response formats
            if (data.items && Array.isArray(data.items)) {
                duplicatePairs = data.items;
            } else if (Array.isArray(data)) {
                duplicatePairs = data;
            } else {
                duplicatePairs = [];
            }
            
            if (duplicatePairs.length === 0) {
                showEmptyState();
            } else {
                showComparison();
                renderCurrentPair();
            }
        } catch (err) {
            console.error('Failed to load duplicates:', err);
            showToast('Failed to load duplicates', 'error');
            showEmptyState();
        }
    }

    /**
     * Display current duplicate pair
     */
    function renderCurrentPair() {
        if (duplicatePairs.length === 0) {
            showEmptyState();
            return;
        }
        
        const pair = duplicatePairs[currentPairIndex];
        
        // Update navigation
        updateNavigation();
        
        // Render both events
        renderEvent('event-a-content', pair.eventA || pair[0]);
        renderEvent('event-b-content', pair.eventB || pair[1]);
    }
    
    /**
     * Render a single event in its card
     */
    function renderEvent(containerId, event) {
        const container = document.getElementById(containerId);
        if (!container || !event) return;
        
        const fields = [
            { label: 'Name', key: 'name' },
            { label: 'Description', key: 'description' },
            { label: 'Start Date', key: 'start_date', transform: formatDate },
            { label: 'End Date', key: 'end_date', transform: formatDate },
            { label: 'Location', key: 'location', transform: formatLocation },
            { label: 'Lifecycle State', key: 'lifecycle_state', transform: formatBadge },
            { label: 'Source', key: 'source' },
            { label: 'Confidence', key: 'confidence', transform: formatConfidence },
            { label: 'Public URL', key: 'public_url', transform: formatURL },
            { label: 'Event ID', key: '@id', transform: formatURL }
        ];
        
        let html = '';
        
        for (const field of fields) {
            let value = getNestedValue(event, field.key);
            
            if (value === null || value === undefined || value === '') {
                continue; // Skip empty fields
            }
            
            if (field.transform) {
                value = field.transform(value);
            } else {
                value = escapeHtml(String(value));
            }
            
            html += `
                <div class="event-field">
                    <label>${escapeHtml(field.label)}:</label>
                    <div class="value">${value}</div>
                </div>
            `;
        }
        
        container.innerHTML = html || '<p class="text-muted">No data available</p>';
    }
    
    /**
     * Get nested object value by key path (e.g., "location.name")
     */
    function getNestedValue(obj, path) {
        if (!obj) return null;
        const keys = path.split('.');
        let value = obj;
        for (const key of keys) {
            value = value?.[key];
            if (value === undefined) return null;
        }
        return value;
    }
    
    /**
     * Update navigation controls
     */
    function updateNavigation() {
        const counter = document.getElementById('pair-counter');
        const prevBtn = document.getElementById('prev-btn');
        const nextBtn = document.getElementById('next-btn');
        
        if (counter) {
            counter.textContent = `Pair ${currentPairIndex + 1} of ${duplicatePairs.length}`;
        }
        
        if (prevBtn) {
            prevBtn.disabled = currentPairIndex === 0;
        }
        
        if (nextBtn) {
            nextBtn.disabled = currentPairIndex === duplicatePairs.length - 1;
        }
    }
    
    /**
     * Navigate to previous duplicate pair
     */
    function navigatePrevious() {
        if (currentPairIndex > 0) {
            currentPairIndex--;
            renderCurrentPair();
        }
    }
    
    /**
     * Navigate to next duplicate pair
     */
    function navigateNext() {
        if (currentPairIndex < duplicatePairs.length - 1) {
            currentPairIndex++;
            renderCurrentPair();
        }
    }
    
    /**
     * Skip current pair without merging
     */
    function skipPair() {
        if (currentPairIndex < duplicatePairs.length - 1) {
            navigateNext();
        } else {
            // Last pair - reload to check for more
            loadDuplicates();
        }
    }
    
    /**
     * Show merge confirmation modal
     */
    function showMergeModal(keep) {
        const pair = duplicatePairs[currentPairIndex];
        const eventA = pair.eventA || pair[0];
        const eventB = pair.eventB || pair[1];
        
        mergeSelection = {
            keep: keep,
            primary: keep === 'a' ? eventA : eventB,
            duplicate: keep === 'a' ? eventB : eventA
        };
        
        const message = document.getElementById('merge-message');
        const keepName = escapeHtml(mergeSelection.primary.name || 'Selected Event');
        const mergeName = escapeHtml(mergeSelection.duplicate.name || 'Other Event');
        
        message.innerHTML = `
            You are about to merge these events:<br><br>
            <strong>Keep:</strong> ${keepName}<br>
            <strong>Merge:</strong> ${mergeName}<br><br>
            The merged event will be marked as a duplicate and will redirect to the kept event.
        `;
        
        const modal = document.getElementById('merge-modal');
        modal.classList.add('show');
    }
    
    /**
     * Close merge confirmation modal
     */
    function closeMergeModal() {
        const modal = document.getElementById('merge-modal');
        modal.classList.remove('show');
        mergeSelection = null;
    }
    
    /**
     * Confirm and execute merge
     */
    async function confirmMerge() {
        if (!mergeSelection) return;
        
        const confirmBtn = document.getElementById('confirm-merge-btn');
        setLoading(confirmBtn, true);
        
        try {
            // Extract IDs from events
            const primaryId = extractEventId(mergeSelection.primary);
            const duplicateId = extractEventId(mergeSelection.duplicate);
            
            if (!primaryId || !duplicateId) {
                throw new Error('Could not extract event IDs');
            }
            
            await API.events.merge(primaryId, duplicateId);
            
            // Success - close modal and move to next pair
            closeMergeModal();
            
            // Remove merged pair from list
            duplicatePairs.splice(currentPairIndex, 1);
            
            // Adjust index if we're at the end
            if (currentPairIndex >= duplicatePairs.length) {
                currentPairIndex = Math.max(0, duplicatePairs.length - 1);
            }
            
            // Show next pair or empty state
            if (duplicatePairs.length === 0) {
                showEmptyState();
            } else {
                renderCurrentPair();
            }
            
            showToast('Events merged successfully', 'success');
        } catch (err) {
            console.error('Failed to merge events:', err);
            showToast(err.message || 'Failed to merge events', 'error');
        } finally {
            setLoading(confirmBtn, false);
        }
    }
    
    /**
     * Extract event ID from event object
     */
    function extractEventId(event) {
        if (!event) return null;
        
        // Try @id first (full URI)
        if (event['@id']) {
            const match = event['@id'].match(/events\/([A-Z0-9]{26})/i);
            if (match) return match[1];
        }
        
        // Try id field
        if (event.id) return event.id;
        
        // Try ulid field
        if (event.ulid) return event.ulid;
        
        return null;
    }
    
    /**
     * UI State Management
     */
    function showLoading() {
        document.getElementById('loading-state').style.display = 'block';
        document.getElementById('empty-state').style.display = 'none';
        document.getElementById('comparison-view').style.display = 'none';
        document.getElementById('navigation').style.display = 'none';
    }
    
    function showEmptyState() {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('empty-state').style.display = 'block';
        document.getElementById('comparison-view').style.display = 'none';
        document.getElementById('navigation').style.display = 'none';
    }
    
    function showComparison() {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('empty-state').style.display = 'none';
        document.getElementById('comparison-view').style.display = 'block';
        document.getElementById('navigation').style.display = 'flex';
    }
    
    /**
     * Formatting helpers
     */
    function formatLocation(location) {
        if (!location) return '';
        if (typeof location === 'string') {
            return escapeHtml(location);
        }
        if (location.name) {
            return escapeHtml(location.name);
        }
        if (location.address) {
            return escapeHtml(typeof location.address === 'string' ? location.address : JSON.stringify(location.address));
        }
        return escapeHtml(JSON.stringify(location));
    }
    
    function formatBadge(value) {
        if (!value) return '';
        const colors = {
            published: 'info',
            draft: 'warning',
            cancelled: 'warning'
        };
        const color = colors[value.toLowerCase()] || 'info';
        return `<span class="badge badge-${color}">${escapeHtml(value)}</span>`;
    }
    
    function formatConfidence(value) {
        if (value === null || value === undefined) return '';
        const percent = Math.round(value * 100);
        const color = percent >= 80 ? 'info' : 'warning';
        return `<span class="badge badge-${color}">${percent}%</span>`;
    }
    
    function formatURL(url) {
        if (!url) return '';
        const escaped = escapeHtml(url);
        return `<a href="${escaped}" target="_blank" rel="noopener noreferrer">${escaped}</a>`;
    }
    
    // No longer need to export to window - event delegation handles clicks
    
})();
