// Event Edit Page JavaScript
(function() {
    'use strict';
    
    let eventId = null;
    let eventData = null;
    let occurrences = [];

    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);

    function init() {
        // Extract event ID from URL path
        const pathParts = window.location.pathname.split('/');
        const rawId = pathParts[pathParts.length - 1];
        
        // Validate the ID looks like a ULID (26 alphanumeric characters)
        if (!rawId || rawId === 'new' || rawId === 'undefined') {
            showError('Invalid event ID. The event link may be broken — try returning to the events list.');
            return;
        }
        
        // Extract ULID if the ID is a full URI, otherwise use as-is
        const ulidMatch = rawId.match(/^([A-Z0-9]{26})$/i);
        if (ulidMatch) {
            eventId = ulidMatch[1];
        } else {
            // Try extracting from a URI path
            const uriMatch = rawId.match(/events\/([A-Z0-9]{26})/i);
            eventId = uriMatch ? uriMatch[1] : rawId;
        }

        // Check if we came from review queue
        checkReviewContext();

        // Load event data
        loadEvent();

        // Setup form submission
        document.getElementById('event-form').addEventListener('submit', handleSubmit);
        
        // Setup event delegation for data-action buttons
        setupEventListeners();
    }
    
    function setupEventListeners() {
        // Use event delegation for dynamically created buttons
        document.addEventListener('click', (e) => {
            const target = e.target.closest('[data-action]');
            if (!target) return;
            
            const action = target.dataset.action;
            
            switch(action) {
                case 'cancel':
                    handleCancel();
                    break;
                case 'reload':
                    window.location.reload();
                    break;
                case 'add-occurrence':
                    window.addOccurrence();
                    break;
                case 'save-occurrence':
                    window.saveOccurrence();
                    break;
                case 'edit-occurrence':
                    window.editOccurrence(parseInt(target.dataset.index, 10));
                    break;
                case 'remove-occurrence':
                    window.removeOccurrence(parseInt(target.dataset.index, 10));
                    break;
                case 'clear-occurrence-venue':
                    clearOccurrenceVenue();
                    break;
            }
        });
    }
    
    function handleCancel() {
        // If we came from review queue, go back to it
        const reviewQueueId = sessionStorage.getItem('from_review_queue');
        if (reviewQueueId) {
            sessionStorage.removeItem('from_review_queue');
            window.location.href = '/admin/review-queue';
        } else {
            window.history.back();
        }
    }
    
    function checkReviewContext() {
        // Check if we came from review queue (set by review-queue.js)
        const reviewQueueId = sessionStorage.getItem('from_review_queue');
        if (reviewQueueId) {
            const banner = document.getElementById('review-context-banner');
            if (banner) {
                banner.style.display = 'block';
                banner.innerHTML = `
                    <div class="alert alert-info mt-3 mb-0" role="alert">
                        <div class="d-flex">
                            <div>
                                <svg xmlns="http://www.w3.org/2000/svg" class="icon alert-icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                    <circle cx="12" cy="12" r="9"/>
                                    <line x1="12" y1="8" x2="12" y2="12"/>
                                    <line x1="12" y1="16" x2="12.01" y2="16"/>
                                </svg>
                            </div>
                            <div>
                                <h4 class="alert-title">Viewing from Review Queue</h4>
                                <div class="text-muted">This event was flagged for review. You can edit it here or <a href="/admin/review-queue" class="alert-link">return to the review queue</a>.</div>
                            </div>
                        </div>
                    </div>
                `;
            }
        }
    }

    async function loadEvent() {
        try {
            showLoading(true);
            
            // Use the public events endpoint for reading (admin endpoint only supports PUT/DELETE)
            eventData = await API.request(`/api/v1/events/${eventId}`);
            populateForm(eventData);
            showLoading(false);
        } catch (error) {
            console.error('Failed to load event:', error);
            showError(error.message || 'Failed to load event. The event may not exist or you may not have permission to view it.');
            showLoading(false);
        }
    }

    function populateForm(event) {
        // Populate basic fields
        setFieldValue('name', event.name || '');
        setFieldValue('description', event.description || '');
        setFieldValue('lifecycle_state', event.lifecycle_state || event.sel?.lifecycleState || 'draft');
        setFieldValue('event_domain', event.eventDomain || event.sel?.eventDomain || '');
        
        // Handle keywords (can be array or comma-separated string)
        if (Array.isArray(event.keywords)) {
            setFieldValue('keywords', event.keywords.join(', '));
        } else if (event.keywords) {
            setFieldValue('keywords', event.keywords);
        }

        setFieldValue('image_url', event.image || event.image_url || '');
        setFieldValue('public_url', event.url || event.public_url || '');
        setFieldValue('virtual_url', event.virtual_url || '');
        
        // Handle optional numeric fields
        if (event.confidence !== undefined && event.confidence !== null) {
            setFieldValue('confidence', event.confidence);
        }
        if (event.quality_score !== undefined && event.quality_score !== null) {
            setFieldValue('quality_score', event.quality_score);
        }

        // Load occurrences from subEvent array or occurrences property
        occurrences = [];
        if (Array.isArray(event.subEvent)) {
            occurrences = event.subEvent.map(occ => {
                // occ.location can be:
                //   - a VirtualLocation object  → { "@type": "VirtualLocation", "url": "..." }
                //   - an embedded Place object   → { "@type": "Place", "@id": "https://.../places/<ULID>", ... }
                //   - a URI string               → "https://.../places/<ULID>"  (resolver unavailable)
                //   - absent                     → undefined / null
                let virtualUrl = null;
                let venueId = null;
                const loc = occ.location;
                if (loc) {
                    if (typeof loc === 'string') {
                        // URI string — extract ULID and treat as venue override
                        venueId = loc;
                    } else if (loc['@type'] === 'VirtualLocation') {
                        virtualUrl = loc.url || null;
                    } else if (loc['@type'] === 'Place') {
                        // Embedded Place — use the @id URI as the venue reference
                        venueId = loc['@id'] || null;
                    }
                }
                return {
                    id: occ['@id'] || null,
                    start_time: occ.startDate || '',
                    end_time: occ.endDate || null,
                    timezone: occ.timezone || 'America/Toronto',
                    door_time: occ.doorTime || null,
                    virtual_url: virtualUrl,
                    venue_id: venueId
                };
            });
        } else if (Array.isArray(event.occurrences)) {
            occurrences = event.occurrences;
        } else if (event.startDate) {
            // If there's a single startDate, create one occurrence
            occurrences = [{
                id: null,
                start_time: event.startDate,
                end_time: event.endDate || null,
                timezone: event.timezone || 'America/Toronto',
                door_time: event.doorTime || null,
                virtual_url: event.location?.url || null
            }];
        }

        renderOccurrences();

        // Load location data if available
        loadLocation(event.location);

        // Show the form
        document.getElementById('event-form').style.display = 'block';
    }

    function setFieldValue(fieldId, value) {
        const field = document.getElementById(fieldId);
        if (field) {
            field.value = value || '';
        }
    }

    async function loadLocation(location) {
        if (!location) return;

        var section = document.getElementById('location-section');
        if (!section) return;

        // If location is a VirtualLocation object, skip (already shown via virtual_url field)
        if (typeof location === 'object' && location['@type'] === 'VirtualLocation') {
            return;
        }

        // Location should be a place URI string
        if (typeof location !== 'string') return;

        // Extract the place ULID from the URI (last path segment)
        var parts = location.replace(/\/+$/, '').split('/');
        var placeULID = parts[parts.length - 1];
        if (!placeULID || !/^[A-Z0-9]{26}$/i.test(placeULID)) return;

        try {
            var place = await API.request('/api/v1/places/' + placeULID);
            displayLocation(place);
        } catch (err) {
            console.error('Failed to load place:', err);
            // Show the section with an error message
            section.style.display = 'block';
            document.getElementById('location-name').textContent = '(Failed to load venue details)';
        }
    }

    function displayLocation(place) {
        var section = document.getElementById('location-section');
        section.style.display = 'block';

        // Venue name
        document.getElementById('location-name').textContent = place.name || '-';

        // Address
        var address = place.address;
        if (address) {
            var parts = [];
            if (address.streetAddress) parts.push(address.streetAddress);
            if (address.addressLocality) parts.push(address.addressLocality);
            if (address.addressRegion) parts.push(address.addressRegion);
            if (address.addressCountry) parts.push(address.addressCountry);
            document.getElementById('location-address').textContent = parts.join(', ') || '-';
        }

        // Coordinates with OpenStreetMap link
        var coordsEl = document.getElementById('location-coords');
        var geo = place.geo;
        if (geo && geo.latitude != null && geo.longitude != null) {
            var lat = geo.latitude.toFixed(6);
            var lon = geo.longitude.toFixed(6);
            coordsEl.textContent = '';
            var coordText = document.createTextNode(lat + ', ' + lon + ' ');
            coordsEl.appendChild(coordText);
            var mapLink = document.createElement('a');
            mapLink.href = 'https://www.openstreetmap.org/?mlat=' + lat + '&mlon=' + lon + '#map=17/' + lat + '/' + lon;
            mapLink.target = '_blank';
            mapLink.rel = 'noopener';
            mapLink.textContent = '(map)';
            mapLink.className = 'text-muted';
            coordsEl.appendChild(mapLink);
        } else {
            coordsEl.textContent = 'Not geocoded';
        }
    }

    function renderOccurrences() {
        const container = document.getElementById('occurrences-list');
        const noOccurrences = document.getElementById('no-occurrences');

        if (occurrences.length === 0) {
            noOccurrences.style.display = 'block';
            return;
        }

        noOccurrences.style.display = 'none';

        const html = occurrences.map((occ, index) => `
            <div class="card mb-2">
                <div class="card-body">
                    <div class="row align-items-center">
                        <div class="col">
                            <div class="fw-bold">${formatDateTime(occ.start_time)}</div>
                            ${occ.end_time ? `<div class="text-muted small">Ends: ${formatDateTime(occ.end_time)}</div>` : ''}
                            ${occ.timezone ? `<div class="text-muted small">Timezone: ${escapeHtml(occ.timezone)}</div>` : ''}
                            ${occ.door_time ? `<div class="text-muted small">Doors: ${formatDateTime(occ.door_time)}</div>` : ''}
                            ${occ.virtual_url ? `<div class="text-muted small">Virtual: ${escapeHtml(occ.virtual_url)}</div>` : ''}
                            ${occ.venue_id ? `<div class="text-muted small"><span class="badge bg-blue-lt">Venue override</span> ${escapeHtml(venueUlidFromId(occ.venue_id))}</div>` : ''}
                        </div>
                        <div class="col-auto">
                            <div class="btn-list">
                                <button type="button" class="btn btn-sm btn-icon" data-action="edit-occurrence" data-index="${index}" title="Edit">
                                    <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                        <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                        <path d="M7 7h-1a2 2 0 0 0 -2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2 -2v-1"/>
                                        <path d="M20.385 6.585a2.1 2.1 0 0 0 -2.97 -2.97l-8.415 8.385v3h3l8.385 -8.415z"/>
                                        <path d="M16 5l3 3"/>
                                    </svg>
                                </button>
                                <button type="button" class="btn btn-sm btn-icon btn-ghost-danger" data-action="remove-occurrence" data-index="${index}" title="Remove">
                                    <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                        <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                        <line x1="4" y1="7" x2="20" y2="7"/>
                                        <line x1="10" y1="11" x2="10" y2="17"/>
                                        <line x1="14" y1="11" x2="14" y2="17"/>
                                        <path d="M5 7l1 12a2 2 0 0 0 2 2h8a2 2 0 0 0 2 -2l1 -12"/>
                                        <path d="M9 7v-3a1 1 0 0 1 1 -1h4a1 1 0 0 1 1 1v3"/>
                                    </svg>
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `).join('');

        // Replace content but keep no-occurrences div
        const cards = container.querySelectorAll('.card');
        cards.forEach(card => card.remove());
        container.insertAdjacentHTML('afterbegin', html);
    }

    async function handleSubmit(e) {
        e.preventDefault();

        try {
            const form = e.target;
            const submitBtn = form.querySelector('button[type="submit"]');
            setLoading(submitBtn, true);

            // Build update payload
            const payload = {
                name: document.getElementById('name').value,
                description: document.getElementById('description').value || null,
                lifecycle_state: document.getElementById('lifecycle_state').value,
                event_domain: document.getElementById('event_domain').value || null,
                image_url: document.getElementById('image_url').value || null,
                public_url: document.getElementById('public_url').value || null,
            };

            // Add keywords as array
            const keywordsValue = document.getElementById('keywords').value;
            if (keywordsValue) {
                payload.keywords = keywordsValue.split(',').map(k => k.trim()).filter(k => k);
            }

            // Add optional numeric fields if set
            const confidence = document.getElementById('confidence').value;
            if (confidence) {
                payload.confidence = parseFloat(confidence);
            }

            const qualityScore = document.getElementById('quality_score').value;
            if (qualityScore) {
                payload.quality_score = parseInt(qualityScore, 10);
            }

            // Send PUT request using centralized API client
            const result = await API.events.update(eventId, payload);
            showToast('Event updated successfully', 'success');
            
            // Reload the event data to reflect changes
            setTimeout(() => {
                window.location.reload();
            }, 1000);
        } catch (error) {
            console.error('Failed to update event:', error);
            showToast(error.message, 'error');
        } finally {
            const submitBtn = document.querySelector('button[type="submit"]');
            if (submitBtn) {
                setLoading(submitBtn, false);
            }
        }
    }

    // Occurrence Management Functions
    window.addOccurrence = function() {
        // Get default timezone from server (passed via hidden input)
        const defaultTz = document.getElementById('occurrence-default-timezone')?.value || 'America/Toronto';

        // Smart defaults: use last occurrence's times if available
        let defaultStartTime = '';
        let defaultEndTime = '';
        if (occurrences && occurrences.length > 0) {
            const lastOcc = occurrences[occurrences.length - 1];
            defaultStartTime = formatForDatetimeLocal(lastOcc.start_time);
            defaultEndTime = formatForDatetimeLocal(lastOcc.end_time);
            // If no end time, default to 2 hours after start
            if (!lastOcc.end_time && lastOcc.start_time) {
                const startDate = new Date(lastOcc.start_time);
                const endDate = new Date(startDate.getTime() + 2 * 60 * 60 * 1000);
                defaultEndTime = formatForDatetimeLocal(endDate.toISOString());
            }
        }

        // Clear modal fields
        document.getElementById('occurrence-index').value = '';
        document.getElementById('occurrence-venue-id').value = '';
        document.getElementById('occurrence-start-time').value = defaultStartTime;
        document.getElementById('occurrence-end-time').value = defaultEndTime;
        document.getElementById('occurrence-timezone').value = defaultTz;
        document.getElementById('occurrence-door-time').value = '';
        document.getElementById('occurrence-virtual-url').value = '';
        document.getElementById('occurrence-modal-title').textContent = 'Add Occurrence';
        setOccurrenceVenueDisplay(null);

        // Show modal
        const modal = new bootstrap.Modal(document.getElementById('occurrence-modal'));
        modal.show();
    };

    window.editOccurrence = function(index) {
        const occ = occurrences[index];
        if (!occ) return;

        // Populate modal with occurrence data
        document.getElementById('occurrence-index').value = index;
        document.getElementById('occurrence-venue-id').value = occ.venue_id || '';
        document.getElementById('occurrence-start-time').value = formatForDatetimeLocal(occ.start_time);
        document.getElementById('occurrence-end-time').value = occ.end_time ? formatForDatetimeLocal(occ.end_time) : '';
        document.getElementById('occurrence-timezone').value = occ.timezone || 'America/Toronto';
        document.getElementById('occurrence-door-time').value = occ.door_time ? formatForDatetimeLocal(occ.door_time) : '';
        document.getElementById('occurrence-virtual-url').value = occ.virtual_url || '';
        document.getElementById('occurrence-modal-title').textContent = 'Edit Occurrence';
        setOccurrenceVenueDisplay(occ.venue_id || null);

        // Show modal
        const modal = new bootstrap.Modal(document.getElementById('occurrence-modal'));
        modal.show();
    };

    // occurrenceUUIDFromId extracts the UUID from an occurrence @id URI like
    // "https://.../api/v1/admin/events/{ULID}/occurrences/{UUID}".
    // Returns null if the URI does not match or id is falsy.
    function occurrenceUUIDFromId(id) {
        if (!id) return null;
        const m = id.match(/occurrences\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i);
        return m ? m[1] : null;
    }

    // buildOccurrencePayload converts the internal occurrence object to the
    // shape expected by POST /api/v1/admin/events/{id}/occurrences.
    function buildOccurrencePayload(occ) {
        const payload = {
            start_time: occ.start_time,
            timezone: occ.timezone || 'America/Toronto',
        };
        if (occ.end_time)      payload.end_time     = occ.end_time;
        if (occ.door_time)     payload.door_time     = occ.door_time;
        if (occ.virtual_url)   payload.virtual_url   = occ.virtual_url;
        // venue_id in local state is a venue URI; extract the ULID for the API.
        if (occ.venue_id) {
            const m = occ.venue_id.match(/\/([A-Z0-9]{26})$/i);
            if (m) payload.venue_ulid = m[1];
        }
        return payload;
    }

    window.saveOccurrence = async function() {
        // Guard: occurrence-logic.js must have loaded before this is called.
        // If the script failed to load (network error, asset misconfiguration) we
        // surface a meaningful error rather than letting a ReferenceError propagate.
        if (typeof OccurrenceLogic === 'undefined' || typeof OccurrenceLogic.buildOccurrenceFromForm !== 'function') {
            showToast('Occurrence helper failed to load. Please refresh the page and try again.', 'error');
            return;
        }

        const indexValue = document.getElementById('occurrence-index').value;

        const result = OccurrenceLogic.buildOccurrenceFromForm({
            indexValue,
            startTime:      document.getElementById('occurrence-start-time').value,
            endTime:        document.getElementById('occurrence-end-time').value,
            timezone:       document.getElementById('occurrence-timezone').value,
            doorTime:       document.getElementById('occurrence-door-time').value,
            venueId:        document.getElementById('occurrence-venue-id').value,
            virtualUrlRaw:  document.getElementById('occurrence-virtual-url').value,
        }, occurrences);

        if (!result.ok) {
            showToast(result.reason, 'error');
            return;
        }

        const occurrence = result.occurrence;
        const payload = buildOccurrencePayload(occurrence);

        try {
            if (indexValue === '') {
                // POST new occurrence
                const created = await API.events.occurrences.create(eventId, payload);
                // Store the server-assigned UUID as the @id URI so future edits/deletes work.
                occurrence.id = created.id
                    ? (window.location.origin + '/api/v1/admin/events/' + eventId + '/occurrences/' + created.id)
                    : null;
                occurrences.push(occurrence);
                showToast('Occurrence added', 'success');
            } else {
                // PUT existing occurrence
                const idx = parseInt(indexValue, 10);
                const existing = occurrences[idx];
                const uuid = occurrenceUUIDFromId(existing && existing.id);
                if (!uuid) {
                    showToast('Cannot update occurrence: missing ID. Please reload and try again.', 'error');
                    return;
                }
                await API.events.occurrences.update(eventId, uuid, payload);
                // Preserve the @id on the updated record.
                occurrence.id = existing.id;
                occurrences[idx] = occurrence;
                showToast('Occurrence updated', 'success');
            }
        } catch (error) {
            console.error('Failed to save occurrence:', error);
            showToast(error.message || 'Failed to save occurrence', 'error');
            return;
        }

        renderOccurrences();

        // Close modal
        const modal = bootstrap.Modal.getInstance(document.getElementById('occurrence-modal'));
        modal.hide();
    };

    window.removeOccurrence = async function(index) {
        if (!confirm('Are you sure you want to remove this occurrence?')) return;

        const occ = occurrences[index];
        const uuid = occurrenceUUIDFromId(occ && occ.id);
        if (!uuid) {
            // Occurrence not yet persisted (added locally but save failed?) — just remove locally.
            occurrences.splice(index, 1);
            renderOccurrences();
            showToast('Occurrence removed', 'success');
            return;
        }

        try {
            await API.events.occurrences.delete(eventId, uuid);
            occurrences.splice(index, 1);
            renderOccurrences();
            showToast('Occurrence removed', 'success');
        } catch (error) {
            console.error('Failed to remove occurrence:', error);
            showToast(error.message || 'Failed to remove occurrence', 'error');
        }
    };

    // setOccurrenceVenueDisplay controls the venue-override section visibility in the modal.
    // venueId is the raw URI string (e.g. "https://.../places/01HXX...") or null.
    function setOccurrenceVenueDisplay(venueId) {
        const section = document.getElementById('occurrence-venue-section');
        const display = document.getElementById('occurrence-venue-display');
        const virtualSection = document.getElementById('occurrence-virtual-section');
        const hybridWarning = document.getElementById('occurrence-hybrid-warning');
        if (venueId) {
            display.value = venueUlidFromId(venueId);
            section.style.display = 'block';
            // Hide virtual URL when a physical venue override is active to prevent hybrids.
            virtualSection.style.display = 'none';
            // Show hybrid warning if the virtual URL input still has a stale value (legacy bad data).
            const hasStaleVirtualUrl = !!(document.getElementById('occurrence-virtual-url').value);
            if (hybridWarning) {
                hybridWarning.style.display = hasStaleVirtualUrl ? 'block' : 'none';
            }
        } else {
            display.value = '';
            section.style.display = 'none';
            if (hybridWarning) {
                hybridWarning.style.display = 'none';
            }
            virtualSection.style.display = 'block';
        }
    }

    // clearOccurrenceVenue removes the venue override from the modal (user action).
    // The virtual URL input is shown again, retaining whatever stale value it may have —
    // the admin can then clear it manually or keep it to save the occurrence as virtual-only.
    function clearOccurrenceVenue() {
        document.getElementById('occurrence-venue-id').value = '';
        setOccurrenceVenueDisplay(null);
    }

    // venueUlidFromId extracts the trailing ULID from a venue URI, or returns the raw value.
    function venueUlidFromId(venueId) {
        if (!venueId) return '';
        const m = venueId.match(/\/([A-Z0-9]{26})$/i);
        return m ? m[1] : venueId;
    }

    // Utility Functions
    function showLoading(loading) {
        document.getElementById('loading-state').style.display = loading ? 'block' : 'none';
        document.getElementById('event-form').style.display = loading ? 'none' : 'block';
        document.getElementById('error-state').style.display = 'none';
    }

    function showError(message) {
        document.getElementById('error-message').textContent = message;
        document.getElementById('error-state').style.display = 'block';
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('event-form').style.display = 'none';
    }

    function formatDateTime(dateString) {
        if (!dateString) return '';
        const date = new Date(dateString);
        return date.toLocaleDateString('en-US', {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        });
    }

    function formatForDatetimeLocal(dateString) {
        if (!dateString) return '';
        const date = new Date(dateString);
        // Format: YYYY-MM-DDTHH:mm
        const year = date.getFullYear();
        const month = String(date.getMonth() + 1).padStart(2, '0');
        const day = String(date.getDate()).padStart(2, '0');
        const hours = String(date.getHours()).padStart(2, '0');
        const minutes = String(date.getMinutes()).padStart(2, '0');
        return `${year}-${month}-${day}T${hours}:${minutes}`;
    }
})();
