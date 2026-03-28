// Event Edit Page JavaScript
(function() {
    'use strict';
    
    let eventId = null;
    let eventData = null;
    let occurrences = [];

    /** Extract the venue URI from eventData.location — handles plain string or JSON-LD Place object. */
    function eventVenueUri() {
        const loc = eventData && eventData.location;
        if (!loc) return null;
        if (typeof loc === 'string') return loc;
        if (typeof loc === 'object' && loc['@id']) return loc['@id'];
        return null;
    }

    /** Extract a ULID from a URI like https://.../things/01KKY... or return the value if already a bare ULID. */
    function extractUlid(uri) {
        if (!uri) return null;
        const m = String(uri).match(/\/([A-Z0-9]{26})(?:\/|$)/i);
        return m ? m[1] : (/^[A-Z0-9]{26}$/i.test(uri) ? uri : null);
    }

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
                case 'show-add-form': {
                    const addEntryId = target.dataset.entryId || 'event-edit';
                    OccurrenceRendering.showAddForm(addEntryId);
                    OccurrenceRendering.fillEventVenuePlaceholder(addEntryId, eventVenueUri());
                    break;
                }
                case 'cancel-add-occurrence':
                    OccurrenceRendering.hideAddForm(target.dataset.entryId || 'event-edit');
                    break;
                case 'add-occurrence':
                    handleAddOccurrence(target);
                    break;
                case 'edit-occurrence':
                    handleEditOccurrence(target);
                    break;
                case 'save-occurrence':
                    handleSaveOccurrence(target);
                    break;
                case 'cancel-edit-occurrence':
                    if (window._occBlurDestroy) { window._occBlurDestroy(); window._occBlurDestroy = null; }
                    handleCancelEditOccurrence(target);
                    break;
                case 'remove-occurrence':
                    handleRemoveOccurrence(target);
                    break;
                case 'clear-occurrence-venue':
                    handleClearOccurrenceVenue(target);
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
            container.innerHTML = '';
            if (noOccurrences) {
                container.appendChild(noOccurrences);
                noOccurrences.style.display = 'block';
            }
            return;
        }

        if (noOccurrences) noOccurrences.style.display = 'none';

        const defaultTz = document.getElementById('occurrence-default-timezone')?.value || 'America/Toronto';
        const entryId = 'event-edit';
        
        container.innerHTML = OccurrenceRendering.renderList(occurrences, eventId, entryId, true, defaultTz);

        if (window._occBlurDestroy) window._occBlurDestroy();
        const lastOcc = occurrences.length > 0 ? occurrences[occurrences.length - 1] : null;
        window._occBlurDestroy = OccurrenceLogic.wireStartBlur('event-edit', function() {
            if (lastOcc && lastOcc.start_time && lastOcc.end_time) {
                return { copyDuration: { prevStart: lastOcc.start_time, prevEnd: lastOcc.end_time } };
            }
            return { durationHours: 2 };
        });
        OccurrenceRendering.resolveVenueNames(container);
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

            // Send PUT request using centralized API client (metadata only — occurrences use their own endpoints)
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

    async function handleAddOccurrence(target) {
        const entryId = target.dataset.entryId || 'event-edit';

        const startTimeRaw = document.getElementById('occ-start-' + entryId)?.value;
        if (!startTimeRaw) {
            showToast('Start time is required', 'error');
            return;
        }

        const endTimeRaw = document.getElementById('occ-end-' + entryId)?.value;
        const timezone = document.getElementById('occ-tz-' + entryId)?.value || 'America/Toronto';
        const doorTimeRaw = document.getElementById('occ-door-' + entryId)?.value;
        const virtualUrl = document.getElementById('occ-virtual-url-' + entryId)?.value || null;
        const venueIdRaw = document.getElementById('occ-venue-id-' + entryId)?.value || null;
        const venueId = venueIdRaw || eventVenueUri();
        const venueUlid = extractUlid(venueId);

        const body = {
            start_time: OccurrenceLogic.convertToRFC3339(startTimeRaw, timezone),
            timezone: timezone,
        };
        if (endTimeRaw) body.end_time = OccurrenceLogic.convertToRFC3339(endTimeRaw, timezone);
        if (doorTimeRaw) body.door_time = OccurrenceLogic.convertToRFC3339(doorTimeRaw, timezone);
        if (venueUlid) body.venue_ulid = venueUlid;
        else if (virtualUrl) body.virtual_url = virtualUrl;

        try {
            const created = await API.events.occurrences.create(eventId, body);
            // Normalise response to local shape
            occurrences.push({
                id: created.id || null,
                start_time: created.start_time || body.start_time,
                end_time: created.end_time || null,
                timezone: created.timezone || timezone,
                door_time: created.door_time || null,
                virtual_url: created.virtual_url || null,
                venue_id: venueId || null,
            });
            OccurrenceRendering.hideAddForm(entryId);
            renderOccurrences();
            showToast('Occurrence added', 'success');
        } catch (err) {
            showToast(err.message || 'Failed to add occurrence', 'error');
        }
    }

    function handleEditOccurrence(target) {
        const entryId = target.dataset.entryId || 'event-edit';
        const index = parseInt(target.dataset.occurrenceIndex, 10);
        
        if (isNaN(index) || index < 0 || index >= occurrences.length) {
            showToast('Invalid occurrence', 'error');
            return;
        }

        const occ = occurrences[index];
        const editHtml = OccurrenceRendering.renderEditRow(occ, eventId, entryId, index);
        
        // Row ID uses occurrence id when available, falls back to index for pending occurrences.
        const rowSuffix = (occ.id || occ['@id']) ? (occ.id || occ['@id']) : 'idx-' + index;
        const row = document.getElementById('occ-row-' + entryId + '-' + rowSuffix);
        if (row) {
            row.outerHTML = editHtml;
        }

        OccurrenceRendering.hideAddForm(entryId);
        OccurrenceRendering.resolveVenueDisplayValue(entryId);   // replace raw ULID with venue name
        OccurrenceRendering.fillEventVenuePlaceholder(entryId, eventVenueUri());
        if (window._occBlurDestroy) window._occBlurDestroy();
        window._occBlurDestroy = OccurrenceLogic.wireStartBlur(entryId, function() {
            return { durationHours: 2 };
        });
    }

    function handleCancelEditOccurrence(target) {
        const entryId = target.dataset.entryId || 'event-edit';
        const occId = target.dataset.occurrenceId || '';

        // Find the occurrence by id or by the edit container's data-occurrence-index attribute
        const saveBtn = document.querySelector('[data-action="save-occurrence"][data-entry-id="' + entryId + '"]');
        const index = saveBtn ? parseInt(saveBtn.dataset.occurrenceIndex, 10) : NaN;

        // Swap the edit container back to a read row without a full re-render (avoids venue re-fetch)
        const editContainer = document.getElementById('occ-edit-' + entryId + '-' + occId);
        if (editContainer && !isNaN(index) && index >= 0 && index < occurrences.length) {
            const occ = occurrences[index];
            const rowSuffix = (occ.id || occ['@id']) ? (occ.id || occ['@id']) : 'idx-' + index;
            const start = occ.start_time || occ.startTime;
            const end = occ.end_time || occ.endTime;
            const timezone = occ.timezone;
            const doorTime = occ.door_time || occ.doorTime;
            const venueId = occ.venue_id || occ.venueId;
            const virtualUrl = occ.virtual_url || occ.virtualUrl;
            const safeEntryId = escapeHtml(String(entryId));

            let detailsHtml = '';
            if (timezone) {
                detailsHtml += '<span class="badge bg-secondary-lt me-1">' + escapeHtml(timezone) + '</span>';
            }
            if (doorTime) {
                detailsHtml += '<span class="text-muted small me-1">Doors: ' + formatDate(doorTime, { hour: 'numeric', minute: '2-digit' }) + '</span>';
            }
            if (virtualUrl) {
                detailsHtml += '<span class="text-muted small d-block">' + escapeHtml(virtualUrl) + '</span>';
            }
            if (venueId) {
                var m = venueId.match(/\/([A-Z0-9]{26})$/i);
                var venueUlid = m ? m[1] : venueId;
                detailsHtml += '<span class="badge bg-blue-lt me-1" data-venue-label="' + escapeHtml(venueUlid) + '">Venue: <span class="venue-name-' + escapeHtml(venueUlid) + '">(loading\u2026)</span></span>';
            }

            const timeStr = OccurrenceLogic.formatTimeRange(start, end);
            const rowHtml = '<div class="d-flex align-items-start py-2 border-bottom" id="occ-row-' + safeEntryId + '-' + escapeHtml(rowSuffix) + '">' +
                '<div class="flex-grow-1">' +
                '<div class="text-body-secondary">' + escapeHtml(timeStr) + '</div>' +
                (detailsHtml ? '<div class="mt-1">' + detailsHtml + '</div>' : '') +
                '</div>' +
               '<button type="button" class="btn btn-sm btn-outline-secondary ms-2" data-action="edit-occurrence" data-entry-id="' + safeEntryId + '" data-event-ulid="' + escapeHtml(eventId) + '" data-occurrence-id="' + escapeHtml(occ.id || '') + '" data-occurrence-index="' + index + '">Edit</button>' +
               '<button type="button" class="btn btn-sm btn-ghost-danger ms-1" data-action="remove-occurrence" data-entry-id="' + safeEntryId + '" data-event-ulid="' + escapeHtml(eventId) + '" data-occurrence-id="' + escapeHtml(occ.id || '') + '" data-occurrence-index="' + index + '" title="Remove occurrence">&#10005;</button>' +
                '</div>';
            editContainer.outerHTML = rowHtml;
            // Re-resolve venue names for this row only (only fires if venueId present)
            if (venueId) {
                OccurrenceRendering.resolveVenueNames(document.getElementById('occurrences-list'));
            }
        } else {
            // Fallback: full re-render
            renderOccurrences();
            return;
        }

        OccurrenceRendering.hideAddForm(entryId);
    }

    async function handleSaveOccurrence(target) {
        const entryId = target.dataset.entryId || 'event-edit';
        const index = parseInt(target.dataset.occurrenceIndex, 10);
        if (isNaN(index) || index < 0 || index >= occurrences.length) {
            showToast('Invalid occurrence', 'error');
            return;
        }

        if (window._occBlurDestroy) { window._occBlurDestroy(); window._occBlurDestroy = null; }

        const startTimeRaw = document.getElementById('occ-start-' + entryId)?.value;
        if (!startTimeRaw) {
            showToast('Start time is required', 'error');
            return;
        }

        const endTimeRaw = document.getElementById('occ-end-' + entryId)?.value;
        const timezone = document.getElementById('occ-tz-' + entryId)?.value || 'America/Toronto';
        const doorTimeRaw = document.getElementById('occ-door-' + entryId)?.value;
        const virtualUrl = document.getElementById('occ-virtual-url-' + entryId)?.value || null;
        const venueIdRaw = document.getElementById('occ-venue-id-' + entryId)?.value || null;
        const venueId = venueIdRaw || eventVenueUri();
        const venueUlid = extractUlid(venueId);

        const body = {
            start_time: OccurrenceLogic.convertToRFC3339(startTimeRaw, timezone),
            timezone: timezone,
        };
        if (endTimeRaw) body.end_time = OccurrenceLogic.convertToRFC3339(endTimeRaw, timezone);
        if (doorTimeRaw) body.door_time = OccurrenceLogic.convertToRFC3339(doorTimeRaw, timezone);
        if (venueUlid) body.venue_ulid = venueUlid;
        else if (virtualUrl) body.virtual_url = virtualUrl;

        const occUlid = extractUlid(occurrences[index].id);
        try {
            await API.events.occurrences.update(eventId, occUlid, body);
            occurrences[index] = {
                id: occurrences[index].id,
                start_time: body.start_time,
                end_time: body.end_time || null,
                timezone: timezone,
                door_time: body.door_time || null,
                virtual_url: body.virtual_url || null,
                venue_id: venueId || null,
            };
            renderOccurrences();
            showToast('Occurrence updated', 'success');
        } catch (err) {
            showToast(err.message || 'Failed to update occurrence', 'error');
        }
    }

    async function handleRemoveOccurrence(target) {
        const index = parseInt(target.dataset.occurrenceIndex, 10);
        if (isNaN(index) || index < 0 || index >= occurrences.length) {
            showToast('Invalid occurrence', 'error');
            return;
        }

        if (!confirm('Are you sure you want to remove this occurrence?')) return;

        const occUlid = extractUlid(occurrences[index].id);
        try {
            await API.events.occurrences.delete(eventId, occUlid);
            occurrences.splice(index, 1);
            renderOccurrences();
            showToast('Occurrence removed', 'success');
        } catch (err) {
            showToast(err.message || 'Failed to remove occurrence', 'error');
        }
    }

    function handleClearOccurrenceVenue(target) {
        const entryId = target.dataset.entryId || 'event-edit';
        const venueInput = document.getElementById('occ-venue-id-' + entryId);
        const displayInput = document.getElementById('occ-venue-display-' + entryId);
        const clearBtn = target;
        
        if (venueInput) venueInput.value = '';
        if (displayInput) displayInput.value = '';
        if (clearBtn) clearBtn.style.display = 'none';
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
