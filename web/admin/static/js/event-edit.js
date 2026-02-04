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
        eventId = pathParts[pathParts.length - 1];
        
        if (!eventId || eventId === 'new') {
            showError('Invalid event ID');
            return;
        }

        // Load event data
        loadEvent();

        // Setup form submission
        document.getElementById('event-form').addEventListener('submit', handleSubmit);
    }

    async function loadEvent() {
        try {
            showLoading(true);
            
            // Fetch event data from API
            const response = await fetch(`/api/v1/events/${eventId}`, {
                credentials: 'include'
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.detail || `Failed to load event: ${response.status}`);
            }

            eventData = await response.json();
            populateForm(eventData);
            showLoading(false);
        } catch (error) {
            console.error('Failed to load event:', error);
            showError(error.message);
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
            occurrences = event.subEvent.map(occ => ({
                id: occ['@id'] || null,
                start_time: occ.startDate || '',
                end_time: occ.endDate || null,
                timezone: occ.timezone || 'America/Toronto',
                door_time: occ.doorTime || null,
                virtual_url: occ.location?.url || null
            }));
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

        // Show the form
        document.getElementById('event-form').style.display = 'block';
    }

    function setFieldValue(fieldId, value) {
        const field = document.getElementById(fieldId);
        if (field) {
            field.value = value || '';
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
                        </div>
                        <div class="col-auto">
                            <div class="btn-list">
                                <button type="button" class="btn btn-sm btn-icon" onclick="editOccurrence(${index})" title="Edit">
                                    <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                        <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                        <path d="M7 7h-1a2 2 0 0 0 -2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2 -2v-1"/>
                                        <path d="M20.385 6.585a2.1 2.1 0 0 0 -2.97 -2.97l-8.415 8.385v3h3l8.385 -8.415z"/>
                                        <path d="M16 5l3 3"/>
                                    </svg>
                                </button>
                                <button type="button" class="btn btn-sm btn-icon btn-ghost-danger" onclick="removeOccurrence(${index})" title="Remove">
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

            // Send PUT request
            const response = await fetch(`/api/v1/admin/events/${eventId}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                },
                credentials: 'include',
                body: JSON.stringify(payload)
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.detail || error.title || `Update failed: ${response.status}`);
            }

            const result = await response.json();
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
        // Clear modal fields
        document.getElementById('occurrence-index').value = '';
        document.getElementById('occurrence-start-time').value = '';
        document.getElementById('occurrence-end-time').value = '';
        document.getElementById('occurrence-timezone').value = 'America/Toronto';
        document.getElementById('occurrence-door-time').value = '';
        document.getElementById('occurrence-virtual-url').value = '';
        document.getElementById('occurrence-modal-title').textContent = 'Add Occurrence';

        // Show modal
        const modal = new bootstrap.Modal(document.getElementById('occurrence-modal'));
        modal.show();
    };

    window.editOccurrence = function(index) {
        const occ = occurrences[index];
        if (!occ) return;

        // Populate modal with occurrence data
        document.getElementById('occurrence-index').value = index;
        document.getElementById('occurrence-start-time').value = formatForDatetimeLocal(occ.start_time);
        document.getElementById('occurrence-end-time').value = occ.end_time ? formatForDatetimeLocal(occ.end_time) : '';
        document.getElementById('occurrence-timezone').value = occ.timezone || 'America/Toronto';
        document.getElementById('occurrence-door-time').value = occ.door_time ? formatForDatetimeLocal(occ.door_time) : '';
        document.getElementById('occurrence-virtual-url').value = occ.virtual_url || '';
        document.getElementById('occurrence-modal-title').textContent = 'Edit Occurrence';

        // Show modal
        const modal = new bootstrap.Modal(document.getElementById('occurrence-modal'));
        modal.show();
    };

    window.saveOccurrence = function() {
        const indexValue = document.getElementById('occurrence-index').value;
        const startTime = document.getElementById('occurrence-start-time').value;
        
        if (!startTime) {
            showToast('Start time is required', 'error');
            return;
        }

        const occurrence = {
            start_time: startTime,
            end_time: document.getElementById('occurrence-end-time').value || null,
            timezone: document.getElementById('occurrence-timezone').value || 'America/Toronto',
            door_time: document.getElementById('occurrence-door-time').value || null,
            virtual_url: document.getElementById('occurrence-virtual-url').value || null
        };

        if (indexValue === '') {
            // Add new occurrence
            occurrences.push(occurrence);
        } else {
            // Update existing occurrence
            const index = parseInt(indexValue, 10);
            occurrences[index] = occurrence;
        }

        renderOccurrences();

        // Close modal
        const modal = bootstrap.Modal.getInstance(document.getElementById('occurrence-modal'));
        modal.hide();

        showToast(indexValue === '' ? 'Occurrence added' : 'Occurrence updated', 'success');
    };

    window.removeOccurrence = function(index) {
        if (confirm('Are you sure you want to remove this occurrence?')) {
            occurrences.splice(index, 1);
            renderOccurrences();
            showToast('Occurrence removed', 'success');
        }
    };

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

    function showToast(message, type = 'success') {
        const container = document.getElementById('toast-container');
        const colors = {
            success: 'bg-success',
            error: 'bg-danger',
            warning: 'bg-warning',
            info: 'bg-info'
        };

        const toast = document.createElement('div');
        toast.className = 'toast show';
        toast.setAttribute('role', 'alert');
        toast.innerHTML = `
            <div class="toast-header">
                <span class="badge ${colors[type]} me-2"></span>
                <strong class="me-auto">${type.charAt(0).toUpperCase() + type.slice(1)}</strong>
                <button type="button" class="btn-close" data-bs-dismiss="toast"></button>
            </div>
            <div class="toast-body">${escapeHtml(message)}</div>
        `;

        container.appendChild(toast);
        setTimeout(() => toast.remove(), 5000);
    }

    function setLoading(element, loading) {
        if (loading) {
            element.disabled = true;
            element.dataset.originalText = element.innerHTML;
            element.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>Saving...';
        } else {
            element.disabled = false;
            element.innerHTML = element.dataset.originalText;
        }
    }

    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
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
