/**
 * Organizations Management Page
 * 
 * Handles organization listing, search, similarity detection, and merging.
 */
(function() {
    'use strict';

    // State management
    var state = {
        currentSearch: '',
        nextCursor: null,
        expandedUlid: null,
        isLoading: false,
        mergeContext: {
            primaryId: null,
            duplicateId: null,
            primaryName: '',
            duplicateName: ''
        }
    };

    // DOM elements (cached after DOMContentLoaded)
    var els = {};

    /**
     * Initialize the page
     */
    function init() {
        cacheElements();
        attachEventListeners();
        loadOrganizations();
    }

    /**
     * Cache DOM elements
     */
    function cacheElements() {
        els = {
            searchInput: document.getElementById('search-input'),
            loadingState: document.getElementById('loading-state'),
            emptyState: document.getElementById('empty-state'),
            tableContainer: document.getElementById('orgs-table-container'),
            tbody: document.getElementById('orgs-tbody'),
            loadMoreContainer: document.getElementById('load-more-container'),
            mergeModal: document.getElementById('merge-modal'),
            mergePrimaryName: document.getElementById('merge-primary-name'),
            mergeDuplicateName: document.getElementById('merge-duplicate-name'),
            confirmMergeBtn: document.getElementById('confirm-merge-btn')
        };
    }

    /**
     * Attach event listeners using delegation
     */
    function attachEventListeners() {
        if (els.searchInput) {
            els.searchInput.addEventListener('input', debounce(handleSearch, 300));
        }

        document.addEventListener('click', handleClick);
    }

    /**
     * Handle all click events via delegation
     */
    function handleClick(e) {
        var target = e.target.closest('[data-action]');
        if (!target) return;

        var action = target.dataset.action;
        var ulid = target.dataset.ulid;

        switch (action) {
            case 'find-similar':
                handleFindSimilar(ulid, target);
                break;
            case 'merge':
                handleMergeClick(target);
                break;
            case 'load-more':
                handleLoadMore();
                break;
            case 'confirm-merge':
                handleConfirmMerge();
                break;
        }
    }

    /**
     * Handle search input
     */
    function handleSearch(e) {
        state.currentSearch = e.target.value.trim();
        state.nextCursor = null;
        state.expandedUlid = null;
        loadOrganizations();
    }

    /**
     * Load organizations from API
     */
    async function loadOrganizations(append) {
        if (state.isLoading) return;

        try {
            state.isLoading = true;
            if (!append) {
                showLoading();
            }

            var params = { limit: 50 };

            if (state.currentSearch) {
                params.q = state.currentSearch;
            }

            if (append && state.nextCursor) {
                params.cursor = state.nextCursor;
            }

            var response = await API.organizations.list(params);

            if (!response || !response.items) {
                throw new Error('Invalid response from server');
            }

            state.nextCursor = response.next_cursor || null;

            if (append) {
                appendOrganizations(response.items);
            } else {
                renderOrganizations(response.items);
            }

            updateLoadMoreButton();

        } catch (error) {
            console.error('Error loading organizations:', error);
            showToast(error.message || 'Failed to load organizations', 'error');

            if (!append) {
                renderOrganizations([]);
            }
        } finally {
            state.isLoading = false;
            hideLoading();
        }
    }

    /**
     * Render organizations table
     */
    function renderOrganizations(orgs) {
        if (!els.tbody) return;

        els.tbody.innerHTML = '';

        if (orgs.length === 0) {
            showEmptyState();
            hideTable();
            return;
        }

        hideEmptyState();
        showTable();

        orgs.forEach(function(org) {
            var row = createOrgRow(org);
            els.tbody.appendChild(row);
        });
    }

    /**
     * Append organizations to existing table
     */
    function appendOrganizations(orgs) {
        if (!els.tbody || orgs.length === 0) return;

        orgs.forEach(function(org) {
            var row = createOrgRow(org);
            els.tbody.appendChild(row);
        });
    }

    /**
     * Create a table row for an organization
     */
    function createOrgRow(org) {
        var tr = document.createElement('tr');
        tr.dataset.ulid = org.ulid;

        var statusBadge = getStatusBadge(org.lifecycle);
        var name = escapeHtml(org.name || 'Unnamed Organization');
        var location = buildLocationText(org);

        tr.innerHTML =
            '<td><strong>' + name + '</strong></td>' +
            '<td>' + location + '</td>' +
            '<td>' + statusBadge + '</td>' +
            '<td>' +
                '<button type="button" class="btn btn-sm btn-outline-primary"' +
                    ' data-action="find-similar"' +
                    ' data-ulid="' + escapeHtml(org.ulid) + '">' +
                    'Find Similar' +
                '</button>' +
            '</td>';

        return tr;
    }

    /**
     * Build location text from organization fields.
     * Organizations use addressLocality/addressRegion (not city/region).
     */
    function buildLocationText(org) {
        var parts = [];
        if (org.addressLocality) parts.push(escapeHtml(org.addressLocality));
        if (org.addressRegion) parts.push(escapeHtml(org.addressRegion));
        // Fallback: some API responses may use address.addressLocality
        if (parts.length === 0 && org.address) {
            if (org.address.addressLocality) parts.push(escapeHtml(org.address.addressLocality));
            if (org.address.addressRegion) parts.push(escapeHtml(org.address.addressRegion));
        }
        return parts.length > 0 ? parts.join(', ') : '—';
    }

    /**
     * Get status badge HTML
     */
    function getStatusBadge(lifecycle) {
        var statusMap = {
            'active': { cls: 'success', text: 'Active' },
            'published': { cls: 'success', text: 'Published' },
            'draft': { cls: 'warning', text: 'Draft' },
            'deleted': { cls: 'danger', text: 'Deleted' },
            'merged': { cls: 'secondary', text: 'Merged' }
        };

        var status = statusMap[lifecycle] || { cls: 'secondary', text: lifecycle || 'Unknown' };
        return '<span class="badge bg-' + status.cls + '">' + escapeHtml(status.text) + '</span>';
    }

    /**
     * Handle "Find Similar" button click
     */
    async function handleFindSimilar(ulid, button) {
        if (!ulid) return;

        // If this org is already expanded, collapse it
        if (state.expandedUlid === ulid) {
            removeSimilarRow(ulid);
            state.expandedUlid = null;
            return;
        }

        // Collapse any previously expanded row
        if (state.expandedUlid) {
            removeSimilarRow(state.expandedUlid);
        }

        try {
            setLoading(button, true);

            var matches = await API.organizations.similar(ulid);

            if (!Array.isArray(matches)) {
                throw new Error('Invalid response from server');
            }

            if (matches.length === 0) {
                showToast('No similar organizations found', 'info');
                return;
            }

            var currentRow = button.closest('tr');
            var originalName = currentRow.cells[0].textContent.trim();
            var similarRow = createSimilarRow(ulid, matches, originalName);
            currentRow.after(similarRow);

            state.expandedUlid = ulid;

        } catch (error) {
            console.error('Error finding similar organizations:', error);
            showToast(error.message || 'Failed to find similar organizations', 'error');
        } finally {
            setLoading(button, false);
        }
    }

    /**
     * Create a row showing similar organizations
     */
    function createSimilarRow(ulid, matches, originalName) {
        var tr = document.createElement('tr');
        tr.className = 'similar-results-row';
        tr.dataset.parentUlid = ulid;

        var matchesHtml = matches.map(function(match) {
            var similarity = match.similarity ? Math.round(match.similarity * 100) : 0;
            var name = escapeHtml(match.name || 'Unnamed Organization');

            return '<div class="d-flex align-items-center justify-content-between mb-2 p-2 border rounded">' +
                '<div>' +
                    '<strong>' + name + '</strong>' +
                    '<span class="badge bg-info ms-2">' + similarity + '% similar</span>' +
                '</div>' +
                '<button type="button" class="btn btn-sm btn-warning"' +
                    ' data-action="merge"' +
                    ' data-primary-id="' + escapeHtml(match.ulid) + '"' +
                    ' data-primary-name="' + escapeHtml(match.name || 'Unnamed Organization') + '"' +
                    ' data-duplicate-id="' + escapeHtml(ulid) + '"' +
                    ' data-duplicate-name="' + escapeHtml(originalName) + '">' +
                    'Merge into this' +
                '</button>' +
            '</div>';
        }).join('');

        tr.innerHTML = '<td colspan="4" class="bg-light">' +
            '<div class="p-3">' +
                '<h6 class="mb-3">Similar Organizations Found:</h6>' +
                matchesHtml +
            '</div>' +
        '</td>';

        return tr;
    }

    /**
     * Remove similar results row
     */
    function removeSimilarRow(ulid) {
        var similarRow = document.querySelector('.similar-results-row[data-parent-ulid="' + ulid + '"]');
        if (similarRow) {
            similarRow.remove();
        }
    }

    /**
     * Handle merge button click
     */
    function handleMergeClick(button) {
        var primaryId = button.dataset.primaryId;
        var primaryName = button.dataset.primaryName;
        var duplicateId = button.dataset.duplicateId;
        var duplicateName = button.dataset.duplicateName;

        if (!primaryId || !duplicateId) {
            showToast('Missing merge information', 'error');
            return;
        }

        state.mergeContext = {
            primaryId: primaryId,
            duplicateId: duplicateId,
            primaryName: primaryName,
            duplicateName: duplicateName
        };

        showMergeModal();
    }

    /**
     * Show merge confirmation modal
     */
    function showMergeModal() {
        if (!els.mergeModal) return;

        if (els.mergePrimaryName) {
            els.mergePrimaryName.textContent = state.mergeContext.primaryName;
        }
        if (els.mergeDuplicateName) {
            els.mergeDuplicateName.textContent = state.mergeContext.duplicateName;
        }

        var modal = new bootstrap.Modal(els.mergeModal);
        modal.show();
    }

    /**
     * Handle merge confirmation
     */
    async function handleConfirmMerge() {
        var ctx = state.mergeContext;

        if (!ctx.primaryId || !ctx.duplicateId) {
            showToast('Missing merge information', 'error');
            return;
        }

        try {
            setLoading(els.confirmMergeBtn, true);

            var result = await API.organizations.merge(ctx.primaryId, ctx.duplicateId);

            if (result && result.status === 'merged') {
                var message = result.already_merged
                    ? 'Organizations were already merged'
                    : 'Successfully merged "' + ctx.duplicateName + '" into "' + ctx.primaryName + '"';

                showToast(message, 'success');

                var modal = bootstrap.Modal.getInstance(els.mergeModal);
                if (modal) modal.hide();

                state.nextCursor = null;
                state.expandedUlid = null;
                loadOrganizations();
            } else {
                throw new Error('Merge failed');
            }

        } catch (error) {
            console.error('Error merging organizations:', error);
            showToast(error.message || 'Failed to merge organizations', 'error');
        } finally {
            setLoading(els.confirmMergeBtn, false);
        }
    }

    /**
     * Handle load more button
     */
    function handleLoadMore() {
        if (state.nextCursor && !state.isLoading) {
            loadOrganizations(true);
        }
    }

    /**
     * Update load more button visibility
     */
    function updateLoadMoreButton() {
        if (!els.loadMoreContainer) return;
        els.loadMoreContainer.style.display = state.nextCursor ? '' : 'none';
    }

    /**
     * Show loading state
     */
    function showLoading() {
        if (els.loadingState) els.loadingState.style.display = '';
        if (els.tableContainer) els.tableContainer.style.display = 'none';
        if (els.emptyState) els.emptyState.style.display = 'none';
    }

    /**
     * Hide loading state
     */
    function hideLoading() {
        if (els.loadingState) els.loadingState.style.display = 'none';
    }

    /**
     * Show table
     */
    function showTable() {
        if (els.tableContainer) els.tableContainer.style.display = '';
    }

    /**
     * Hide table
     */
    function hideTable() {
        if (els.tableContainer) els.tableContainer.style.display = 'none';
    }

    /**
     * Show empty state
     */
    function showEmptyState() {
        if (els.emptyState) els.emptyState.style.display = '';
    }

    /**
     * Hide empty state
     */
    function hideEmptyState() {
        if (els.emptyState) els.emptyState.style.display = 'none';
    }

    // Initialize when DOM is ready
    document.addEventListener('DOMContentLoaded', init);

})();
