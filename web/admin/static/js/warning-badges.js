(function() {
    'use strict';

    /**
     * Centralized badge map — shared by getBadge() and getDetailBadge().
     * Each entry: { label: string, color: Bootstrap bg color }
     *
     * Colors in use:
     *   warning  — yellow  — data quality / missing data issues
     *   purple   — purple  — duplicate detection
     *   info     — teal    — auto-corrected / series relationship
     *   secondary — grey   — resolved / fallback
     */
    const BADGE_MAP = {
        // Date/Time issues
        'reversed_dates_timezone_likely':          { label: 'Date Fixed',           color: 'info'    },
        'reversed_dates_corrected_needs_review':   { label: 'Date Issue',           color: 'warning' },
        'too_far_future':                          { label: 'Too Far Future',       color: 'warning' },

        // Missing data
        'missing_image':                           { label: 'Missing Image',        color: 'warning' },
        'missing_description':                     { label: 'Missing Description',  color: 'warning' },

        // Quality issues
        'low_confidence':                          { label: 'Low Quality',          color: 'warning' },
        'link_check_failed':                       { label: 'Bad Link',             color: 'warning' },

        // Duplicate detection
        'near_duplicate_of_new_event':             { label: 'Near Duplicate',       color: 'purple'  },
        'potential_duplicate':                     { label: 'Possible Duplicate',   color: 'purple'  },
        'place_possible_duplicate':                { label: 'Place Duplicate',      color: 'purple'  },
        'org_possible_duplicate':                  { label: 'Org Duplicate',        color: 'purple'  },

        // Series / recurring event relationships
        'cross_week_series_companion':             { label: 'Weekly Series',        color: 'info'    },
        'multi_session_likely':                    { label: 'Multi-Session',        color: 'info'    },
    };

    /**
     * Return a single badge element HTML for a warning code.
     * Falls back to a prettified code label with bg-secondary.
     * @param {string} code
     * @param {boolean} isResolved - if true, force bg-secondary
     * @returns {string} HTML <span> badge
     */
    function _badgeHtml(code, isResolved) {
        // Handle reversed_dates wildcard (catches any variant not in the map)
        const entry = BADGE_MAP[code] || (code && code.includes('reversed_dates')
            ? { label: 'Date Issue', color: 'warning' }
            : null);

        if (entry) {
            const color = isResolved ? 'secondary' : entry.color;
            return `<span class="badge bg-${color}">${escapeHtml(entry.label)}</span>`;
        }

        // Generic fallback: prettify the code itself
        const label = code ? code.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase()) : 'Issue';
        const color = isResolved ? 'secondary' : 'warning';
        return `<span class="badge bg-${color}">${escapeHtml(label)}</span>`;
    }

    /**
     * Get warning badge HTML for table display.
     * Renders ALL warnings as an inline horizontal row of badges.
     * @param {Array} warnings - Array of warning objects with {code, message, field} properties
     * @param {string} status - Entry status (pending, approved, rejected, merged)
     * @returns {string} HTML string
     */
    function getBadge(warnings, status) {
        if (!warnings || warnings.length === 0) {
            return '<span class="badge bg-success">No Issues</span>';
        }

        const isResolved = status && status !== 'pending';

        // Deduplicate by code so the same code doesn't show twice
        const seen = new Set();
        const badges = [];
        for (const w of warnings) {
            const key = w.code || w.field || 'unknown';
            if (!seen.has(key)) {
                seen.add(key);
                badges.push(_badgeHtml(w.code, isResolved));
            }
        }

        return `<div class="d-flex flex-wrap gap-1">${badges.join('')}</div>`;
    }

    /**
     * Get warning badge for detail view.
     * Returns a single color-coded badge based on warning code.
     * @param {string} code - Warning code identifier
     * @returns {string} HTML string for badge element
     */
    function getDetailBadge(code) {
        return _badgeHtml(code, false);
    }

    window.WarningBadges = { getBadge, getDetailBadge };
})();
