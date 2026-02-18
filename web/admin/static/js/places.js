/**
 * Places Management Page
 * 
 * Handles place listing, search, similarity detection, merging, and editing.
 */
(function() {
    'use strict';

    // State management
    let state = {
        currentSearch: '',
        nextCursor: null,
        expandedUlid: null,
        isLoading: false,
        editUlid: null,
        mergeContext: {
            primaryId: null,
            duplicateId: null,
            primaryName: '',
            duplicateName: ''
        }
    };

    // DOM elements (cached after DOMContentLoaded)
    let els = {};

    /**
     * Initialize the page
     */
    function init() {
        cacheElements();
        attachEventListeners();
        loadPlaces();
    }

    /**
     * Cache DOM elements
     */
    function cacheElements() {
        els = {
            searchInput: document.getElementById('search-input'),
            loadingState: document.getElementById('loading-state'),
            emptyState: document.getElementById('empty-state'),
            tableContainer: document.getElementById('places-table-container'),
            tbody: document.getElementById('places-tbody'),
            loadMoreContainer: document.getElementById('load-more-container'),
            mergeModal: document.getElementById('merge-modal'),
            mergePrimaryName: document.getElementById('merge-primary-name'),
            mergeDuplicateName: document.getElementById('merge-duplicate-name'),
            confirmMergeBtn: document.getElementById('confirm-merge-btn'),
            editModal: document.getElementById('edit-modal'),
            editUlid: document.getElementById('edit-ulid'),
            editName: document.getElementById('edit-name'),
            editDescription: document.getElementById('edit-description'),
            editStreetAddress: document.getElementById('edit-street-address'),
            editCity: document.getElementById('edit-city'),
            editRegion: document.getElementById('edit-region'),
            editPostalCode: document.getElementById('edit-postal-code'),
            editCountry: document.getElementById('edit-country'),
            editTelephone: document.getElementById('edit-telephone'),
            editEmail: document.getElementById('edit-email'),
            editUrl: document.getElementById('edit-url'),
            saveEditBtn: document.getElementById('save-edit-btn')
        };
    }

    /**
     * Attach event listeners using delegation
     */
    function attachEventListeners() {
        // Search input with debounce
        if (els.searchInput) {
            els.searchInput.addEventListener('input', debounce(handleSearch, 300));
        }

        // Delegated click handlers for dynamic content
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
            case 'edit':
                handleEdit(ulid);
                break;
            case 'save-edit':
                handleSaveEdit();
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
        loadPlaces();
    }

    /**
     * Load places from API
     */
    async function loadPlaces(append) {
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

            var response = await API.places.list(params);

            if (!response || !response.items) {
                throw new Error('Invalid response from server');
            }

            state.nextCursor = response.next_cursor || null;

            if (append) {
                appendPlaces(response.items);
            } else {
                renderPlaces(response.items);
            }

            updateLoadMoreButton();

        } catch (error) {
            console.error('Error loading places:', error);
            showToast(error.message || 'Failed to load places', 'error');

            if (!append) {
                renderPlaces([]);
            }
        } finally {
            state.isLoading = false;
            hideLoading();
        }
    }

    /**
     * Render places table
     */
    function renderPlaces(places) {
        if (!els.tbody) return;

        els.tbody.innerHTML = '';

        if (places.length === 0) {
            showEmptyState();
            hideTable();
            return;
        }

        hideEmptyState();
        showTable();

        places.forEach(function(place) {
            var row = createPlaceRow(place);
            els.tbody.appendChild(row);
        });
    }

    /**
     * Append places to existing table
     */
    function appendPlaces(places) {
        if (!els.tbody || places.length === 0) return;

        places.forEach(function(place) {
            var row = createPlaceRow(place);
            els.tbody.appendChild(row);
        });
    }

    /**
     * Extract place ULID from place object.
     * The API returns @id as a full URI (e.g., "https://host/places/01ABC..."),
     * so we need to extract the 26-character ULID from it.
     * @param {Object} place - Place object from API
     * @returns {string|null} Place ULID or null
     */
    function extractPlaceId(place) {
        if (!place) return null;

        // Try @id first (full URI)
        if (place['@id']) {
            var match = place['@id'].match(/places\/([A-Z0-9]{26})/i);
            if (match) return match[1];
        }

        // Fallback to id or ulid field
        if (place.id) return place.id;
        if (place.ulid) return place.ulid;

        return null;
    }

    /**
     * Create a table row for a place
     */
    function createPlaceRow(place) {
        var tr = document.createElement('tr');
        var ulid = extractPlaceId(place);
        tr.dataset.ulid = ulid || '';

        var name = escapeHtml(place.name || 'Unnamed Place');
        var city = escapeHtml((place.address && place.address.addressLocality) || '');
        var region = escapeHtml((place.address && place.address.addressRegion) || '');

        tr.innerHTML =
            '<td><strong>' + name + '</strong></td>' +
            '<td>' + (city || '\u2014') + '</td>' +
            '<td>' + (region || '\u2014') + '</td>' +
            '<td class="text-nowrap">' +
                '<button type="button" class="btn btn-sm btn-outline-secondary me-1"' +
                    ' data-action="edit"' +
                    ' data-ulid="' + escapeHtml(ulid || '') + '"' +
                    ' title="Edit place">' +
                    'Edit' +
                '</button>' +
                '<button type="button" class="btn btn-sm btn-outline-primary"' +
                    ' data-action="find-similar"' +
                    ' data-ulid="' + escapeHtml(ulid || '') + '">' +
                    'Find Similar' +
                '</button>' +
            '</td>';

        return tr;
    }

    /**
     * Handle "Edit" button click — load place data into edit modal
     */
    async function handleEdit(ulid) {
        if (!ulid) return;

        try {
            var place = await API.places.adminGet(ulid);
            if (!place) {
                showToast('Place not found', 'error');
                return;
            }

            state.editUlid = ulid;

            // Populate form fields from flat admin response
            if (els.editUlid) els.editUlid.value = ulid;
            if (els.editName) els.editName.value = place.name || '';
            if (els.editDescription) els.editDescription.value = place.description || '';
            if (els.editUrl) els.editUrl.value = place.url || '';
            if (els.editTelephone) els.editTelephone.value = place.telephone || '';
            if (els.editEmail) els.editEmail.value = place.email || '';
            if (els.editStreetAddress) els.editStreetAddress.value = place.street_address || '';
            if (els.editCity) els.editCity.value = place.city || '';
            if (els.editRegion) els.editRegion.value = place.region || '';
            if (els.editPostalCode) els.editPostalCode.value = place.postal_code || '';
            if (els.editCountry) els.editCountry.value = place.country || '';

            // Show modal
            if (els.editModal) {
                var modal = new bootstrap.Modal(els.editModal);
                modal.show();
            }

        } catch (error) {
            if (error.status === 410) {
                showToast('This place has been deleted or merged and cannot be edited', 'error');
            } else {
                console.error('Error loading place for edit:', error);
                showToast(error.message || 'Failed to load place details', 'error');
            }
        }
    }

    /**
     * Handle save edit — send update to API
     */
    async function handleSaveEdit() {
        var ulid = state.editUlid;
        if (!ulid) return;

        var payload = {};

        // Only include fields that have values (send all fields to allow clearing)
        if (els.editName) payload.name = els.editName.value.trim();
        if (els.editDescription) payload.description = els.editDescription.value.trim();
        if (els.editStreetAddress) payload.street_address = els.editStreetAddress.value.trim();
        if (els.editCity) payload.city = els.editCity.value.trim();
        if (els.editRegion) payload.region = els.editRegion.value.trim();
        if (els.editPostalCode) payload.postal_code = els.editPostalCode.value.trim();
        if (els.editCountry) payload.country = els.editCountry.value.trim();
        if (els.editTelephone) payload.telephone = els.editTelephone.value.trim();
        if (els.editEmail) payload.email = els.editEmail.value.trim();
        if (els.editUrl) payload.url = els.editUrl.value.trim();

        // Validate name is not empty
        if (!payload.name) {
            showToast('Name is required', 'error');
            return;
        }

        try {
            setLoading(els.saveEditBtn, true);

            await API.places.update(ulid, payload);

            showToast('Place updated successfully', 'success');

            // Close modal
            var modal = bootstrap.Modal.getInstance(els.editModal);
            if (modal) modal.hide();

            // Reload the list
            state.nextCursor = null;
            state.expandedUlid = null;
            loadPlaces();

        } catch (error) {
            console.error('Error updating place:', error);
            showToast(error.message || 'Failed to update place', 'error');
        } finally {
            setLoading(els.saveEditBtn, false);
        }
    }

    /**
     * Handle "Find Similar" button click
     */
    async function handleFindSimilar(ulid, button) {
        if (!ulid) return;

        // If this place is already expanded, collapse it
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

            var matches = await API.places.similar(ulid);

            if (!Array.isArray(matches)) {
                throw new Error('Invalid response from server');
            }

            if (matches.length === 0) {
                showToast('No similar places found', 'info');
                return;
            }

            // Insert similar results row
            var currentRow = button.closest('tr');
            var originalName = currentRow.cells[0].textContent.trim();
            var similarRow = createSimilarRow(ulid, matches, originalName);
            currentRow.after(similarRow);

            state.expandedUlid = ulid;

        } catch (error) {
            console.error('Error finding similar places:', error);
            showToast(error.message || 'Failed to find similar places', 'error');
        } finally {
            setLoading(button, false);
        }
    }

    /**
     * Create a row showing similar places
     */
    function createSimilarRow(ulid, matches, originalName) {
        var tr = document.createElement('tr');
        tr.className = 'similar-results-row';
        tr.dataset.parentUlid = ulid;

        var matchesHtml = matches.map(function(match) {
            var similarity = match.similarity ? Math.round(match.similarity * 100) : 0;
            var name = escapeHtml(match.name || 'Unnamed Place');

            return '<div class="d-flex align-items-center justify-content-between mb-2 p-2 border rounded">' +
                '<div>' +
                    '<strong>' + name + '</strong>' +
                    '<span class="badge bg-info ms-2">' + similarity + '% similar</span>' +
                '</div>' +
                '<button type="button" class="btn btn-sm btn-warning"' +
                    ' data-action="merge"' +
                    ' data-primary-id="' + escapeHtml(match.ulid) + '"' +
                    ' data-primary-name="' + escapeHtml(match.name || 'Unnamed Place') + '"' +
                    ' data-duplicate-id="' + escapeHtml(ulid) + '"' +
                    ' data-duplicate-name="' + escapeHtml(originalName) + '">' +
                    'Merge into this' +
                '</button>' +
            '</div>';
        }).join('');

        tr.innerHTML = '<td colspan="4" class="bg-light">' +
            '<div class="p-3">' +
                '<h6 class="mb-3">Similar Places Found:</h6>' +
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

            var result = await API.places.merge(ctx.primaryId, ctx.duplicateId);

            if (result && result.status === 'merged') {
                var message = result.already_merged
                    ? 'Places were already merged'
                    : 'Successfully merged "' + ctx.duplicateName + '" into "' + ctx.primaryName + '"';

                showToast(message, 'success');

                // Close modal
                var modal = bootstrap.Modal.getInstance(els.mergeModal);
                if (modal) modal.hide();

                // Reload the list
                state.nextCursor = null;
                state.expandedUlid = null;
                loadPlaces();
            } else {
                throw new Error('Merge failed');
            }

        } catch (error) {
            console.error('Error merging places:', error);
            showToast(error.message || 'Failed to merge places', 'error');
        } finally {
            setLoading(els.confirmMergeBtn, false);
        }
    }

    /**
     * Handle load more button
     */
    function handleLoadMore() {
        if (state.nextCursor && !state.isLoading) {
            loadPlaces(true);
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
