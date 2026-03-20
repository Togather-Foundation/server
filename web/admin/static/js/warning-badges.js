(function() {
    'use strict';

    /**
     * Get warning badge HTML for table display
     * Shows actual warning messages inline so users know WHY events need review
     * @param {Array} warnings - Array of warning objects with code, message properties
     * @param {string} status - Entry status (pending, approved, rejected, merged)
     * @returns {string} HTML string for warning display with badges and messages
     */
    function getBadge(warnings, status) {
        if (!warnings || warnings.length === 0) {
            return '<span class="badge bg-success">No Issues</span>';
        }
        
        // For resolved entries (approved/rejected/merged), use muted styling
        const isResolved = status && status !== 'pending';
        
        // Show first warning with descriptive message
        const firstWarning = warnings[0];
        
        // Get badge based on warning type
        let badge = '';
        let message = '';
        
        if (firstWarning.code === 'missing_image') {
            badge = isResolved 
                ? '<span class="badge bg-secondary">Missing Image</span>'
                : '<span class="badge bg-warning">Missing Image</span>';
            message = 'No image provided';
        } else if (firstWarning.code === 'missing_description') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Missing Description</span>'
                : '<span class="badge bg-warning">Missing Description</span>';
            message = 'No description provided';
        } else if (firstWarning.code === 'low_confidence') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Low Quality</span>'
                : '<span class="badge bg-warning">Low Quality</span>';
            // Extract percentage from message if present
            const match = firstWarning.message && firstWarning.message.match(/(\d+)%/);
            message = match ? `Data quality: ${match[1]}%` : 'Low data quality score';
        } else if (firstWarning.code === 'too_far_future') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Too Far Future</span>'
                : '<span class="badge bg-warning">Too Far Future</span>';
            message = 'Event >2 years away';
        } else if (firstWarning.code === 'link_check_failed') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Bad Link</span>'
                : '<span class="badge bg-warning">Bad Link</span>';
            message = 'Link check failed';
        } else if (firstWarning.code === 'reversed_dates_timezone_likely') {
            badge = '<span class="badge bg-info">Date Fixed</span>';
            message = isResolved ? 'Timezone corrected' : 'Timezone issue auto-corrected';
        } else if (firstWarning.code === 'reversed_dates_corrected_needs_review') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Date Fixed</span>'
                : '<span class="badge bg-warning">Date Issue</span>';
            message = isResolved ? 'Date corrected' : 'Dates corrected, review needed';
        } else if (firstWarning.code && firstWarning.code.includes('reversed_dates')) {
            badge = isResolved
                ? '<span class="badge bg-secondary">Date Issue</span>'
                : '<span class="badge bg-warning">Date Issue</span>';
            message = 'Date ordering problem';
        } else if (firstWarning.code === 'near_duplicate_of_new_event') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Near Duplicate</span>'
                : '<span class="badge bg-purple">Near Duplicate</span>';
            message = firstWarning.message || 'This existing event may be a near-duplicate of a newly ingested event';
        } else if (firstWarning.code === 'potential_duplicate') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Possible Duplicate</span>'
                : '<span class="badge bg-purple">Possible Duplicate</span>';
            message = firstWarning.message || 'May be a duplicate event';
        } else if (firstWarning.code === 'place_possible_duplicate') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Place Duplicate</span>'
                : '<span class="badge bg-purple">Place Duplicate</span>';
            message = firstWarning.message || 'Similar place already exists';
        } else if (firstWarning.code === 'org_possible_duplicate') {
            badge = isResolved
                ? '<span class="badge bg-secondary">Org Duplicate</span>'
                : '<span class="badge bg-purple">Org Duplicate</span>';
            message = firstWarning.message || 'Similar organization already exists';
        } else {
            // Fallback: use field name or generic message
            const label = firstWarning.field || 'issue';
            badge = isResolved
                ? `<span class="badge bg-secondary">${escapeHtml(label)}</span>`
                : `<span class="badge bg-warning">${escapeHtml(label)}</span>`;
            message = isResolved ? 'Resolved' : (firstWarning.message || 'Needs review');
        }
        
        // If multiple warnings, add count badge
        const additionalCount = warnings.length > 1 ? ` <span class="badge bg-secondary">+${warnings.length - 1} more</span>` : '';
        
        return `
            <div class="d-flex flex-column gap-1">
                <div>${badge}</div>
                <small class="text-muted">${escapeHtml(message)}</small>
                ${additionalCount}
            </div>
        `;
    }

    /**
     * Get warning badge for detail view
     * Returns color-coded badge based on warning code with human-readable labels
     * @param {string} code - Warning code identifier
     * @returns {string} HTML string for badge element
     */
    function getDetailBadge(code) {
        // Map warning codes to user-friendly badge labels and colors
        const badgeMap = {
            // Date/Time issues
            'reversed_dates_timezone_likely': { label: 'Date Fixed', color: 'info' },
            'reversed_dates_corrected_needs_review': { label: 'Date Issue', color: 'warning' },
            'too_far_future': { label: 'Too Far Future', color: 'warning' },
            
            // Missing data
            'missing_image': { label: 'Missing Image', color: 'warning' },
            'missing_description': { label: 'Missing Description', color: 'warning' },
            
            // Quality issues
            'low_confidence': { label: 'Low Quality', color: 'warning' },
            'link_check_failed': { label: 'Bad Link', color: 'warning' },
            
            // Duplicate detection
            'near_duplicate_of_new_event': { label: 'Near Duplicate', color: 'purple' },
            'potential_duplicate': { label: 'Possible Duplicate', color: 'purple' },
            'place_possible_duplicate': { label: 'Place Duplicate', color: 'purple' },
            'org_possible_duplicate': { label: 'Org Duplicate', color: 'purple' },
        };
        
        const badge = badgeMap[code];
        if (badge) {
            return `<span class="badge bg-${badge.color}">${badge.label}</span>`;
        }
        
        // Fallback: use code as label with generic color
        const label = code ? code.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase()) : 'Issue';
        return `<span class="badge bg-secondary">${escapeHtml(label)}</span>`;
    }

    window.WarningBadges = { getBadge, getDetailBadge };
})();
