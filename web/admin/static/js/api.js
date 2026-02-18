// SEL Admin API Client
// Centralized API wrapper for all backend calls
//
// ERROR HANDLING CONTRACT:
// Backend returns RFC 7807 Problem Details for HTTP APIs (https://datatracker.ietf.org/doc/html/rfc7807)
// Error response format:
// {
//   "type": "https://sel.events/problems/validation-error",     // URI identifying the problem type
//   "title": "Validation Error",                                // Human-readable summary
//   "status": 400,                                               // HTTP status code
//   "detail": "Email is required",                               // Human-readable explanation specific to this occurrence
//   "instance": "/api/v1/admin/users"                           // URI reference identifying this specific occurrence (optional)
// }
//
// The `detail` field should be used for displaying error messages to users.
// The `title` field provides a generic category of error (e.g., "Validation Error", "Not Found").

const API = {
    // Retry configuration
    retryConfig: {
        maxAttempts: 3,      // Total attempts (initial + 2 retries)
        delays: [1000, 2000] // Exponential backoff delays in ms
    },
    
    /**
     * Retry wrapper with exponential backoff
     * @param {Function} fn - Async function to retry
     * @param {Object} options - Retry options
     * @param {Function} onRetry - Callback called before each retry (attempt, maxAttempts, delay)
     * @returns {Promise} - Result of fn
     */
    async retryWithBackoff(fn, options = {}, onRetry = null) {
        const maxAttempts = options.maxAttempts || this.retryConfig.maxAttempts;
        const delays = options.delays || this.retryConfig.delays;
        
        let lastError;
        for (let attempt = 1; attempt <= maxAttempts; attempt++) {
            try {
                return await fn();
            } catch (error) {
                lastError = error;
                
                // Don't retry on client errors (4xx except 408 Request Timeout and 429 Rate Limited)
                if (error.status >= 400 && error.status < 500 && error.status !== 408 && error.status !== 429) {
                    throw error;
                }
                
                // Don't retry if this was the last attempt
                if (attempt >= maxAttempts) {
                    throw error;
                }
                
                // Calculate delay for this retry (use last delay if we exceed array)
                const delayIndex = attempt - 1;
                const delay = delays[delayIndex] || delays[delays.length - 1];
                
                // Notify caller about retry
                if (onRetry) {
                    onRetry(attempt, maxAttempts, delay);
                }
                
                // Wait before retrying
                await new Promise(resolve => setTimeout(resolve, delay));
            }
        }
        
        // Should never reach here, but throw last error just in case
        throw lastError;
    },
    
    // Base request method
    async request(url, options = {}) {
        // Get JWT token from localStorage
        const token = localStorage.getItem('admin_token');
        
        // Build headers: only set Content-Type when there's a request body
        const headers = {
            'Accept': 'application/json',
            ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
            ...options.headers
        };
        if (options.body) {
            headers['Content-Type'] = 'application/json';
        }
        
        const response = await fetch(url, {
            ...options,
            headers,
            credentials: 'include', // Include cookies for HTML page auth
            signal: options.signal // Pass AbortSignal for cancellation
        });
        
        if (!response.ok) {
            // Handle rate limiting (429 Too Many Requests)
            if (response.status === 429) {
                const retryAfter = response.headers.get('Retry-After');
                const message = retryAfter 
                    ? `Too many requests. Please wait ${retryAfter} seconds and try again.`
                    : 'Too many requests. Please wait a moment and try again.';
                const error = new Error(message);
                error.status = 429;
                error.retryAfter = retryAfter;
                throw error;
            }
            
            let error;
            try {
                // Try to parse body as text first, then as JSON
                // This handles non-JSON error pages from reverse proxies
                const text = await response.text();
                try {
                    error = JSON.parse(text);
                } catch {
                    error = { detail: text.substring(0, 200) || ('Request failed with status ' + response.status) };
                }
            } catch {
                error = { detail: 'Request failed with status ' + response.status };
            }
            // Throw an Error with the detail field (specific error message)
            // Fall back to title or generic message if detail is missing
            const err = new Error(error.detail || error.title || error.message || 'Request failed');
            err.status = response.status; // Preserve status code for error handling
            throw err;
        }
        
        // Handle empty responses (e.g., 204 No Content for DELETE operations)
        if (response.status === 204 || response.headers.get('content-length') === '0') {
            return null;
        }
        
        // Parse JSON response (covers application/json, application/ld+json, application/problem+json)
        const contentType = response.headers.get('content-type');
        if (contentType && contentType.includes('json')) {
            return response.json();
        }
        
        // If not JSON, return text
        return response.text();
    },
    
    /**
     * Request with automatic retry logic
     * @param {string} url - Request URL
     * @param {Object} options - Fetch options
     * @param {Object} retryOptions - Retry configuration (maxAttempts, delays)
     * @param {Function} onRetry - Callback for retry notifications
     * @returns {Promise} - Response data
     */
    async requestWithRetry(url, options = {}, retryOptions = {}, onRetry = null) {
        return this.retryWithBackoff(
            () => this.request(url, options),
            retryOptions,
            onRetry
        );
    },
    
    // Events API
    events: {
        list: (params = {}) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/events?${query}`);
        },
        
        get: (id) => API.request(`/api/v1/admin/events/${id}`),
        
        update: (id, data) => API.request(`/api/v1/admin/events/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        }),
        
        delete: (id) => API.request(`/api/v1/admin/events/${id}`, {
            method: 'DELETE'
        }),
        
        merge: (sourceId, targetId) => API.request('/api/v1/admin/events/merge', {
            method: 'POST',
            body: JSON.stringify({ source_id: sourceId, target_id: targetId })
        }),
        
        pending: () => API.request('/api/v1/admin/events/pending')
    },
    
    // Admin Stats API
    stats: {
        get: () => API.request('/api/v1/admin/stats')
    },
    
    // API Keys
    apiKeys: {
        list: () => API.request('/api/v1/admin/api-keys'),
        create: (data) => API.request('/api/v1/admin/api-keys', {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        revoke: (id) => API.request(`/api/v1/admin/api-keys/${id}`, {
            method: 'DELETE'
        })
    },
    
    // Duplicates
    duplicates: {
        list: () => API.request('/api/v1/admin/duplicates')
    },
    
    // Places
    places: {
        list: (params = {}) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/places?${query}`);
        },
        
        get: (id) => API.request(`/api/v1/places/${id}`),
        
        adminGet: (id) => API.request(`/api/v1/admin/places/${id}`),
        
        similar: (id) => API.request(`/api/v1/admin/places/${id}/similar`),
        
        merge: (primaryId, duplicateId) => API.request('/api/v1/admin/places/merge', {
            method: 'POST',
            body: JSON.stringify({ primary_id: primaryId, duplicate_id: duplicateId })
        }),
        
        delete: (id, reason) => API.request(`/api/v1/admin/places/${id}`, {
            method: 'DELETE',
            body: JSON.stringify({ reason: reason })
        }),
        
        update: (id, data) => API.request(`/api/v1/admin/places/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        })
    },
    
    // Organizations
    organizations: {
        list: (params = {}) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/organizations?${query}`);
        },
        
        get: (id) => API.request(`/api/v1/organizations/${id}`),
        
        adminGet: (id) => API.request(`/api/v1/admin/organizations/${id}`),
        
        similar: (id) => API.request(`/api/v1/admin/organizations/${id}/similar`),
        
        merge: (primaryId, duplicateId) => API.request('/api/v1/admin/organizations/merge', {
            method: 'POST',
            body: JSON.stringify({ primary_id: primaryId, duplicate_id: duplicateId })
        }),
        
        delete: (id, reason) => API.request(`/api/v1/admin/organizations/${id}`, {
            method: 'DELETE',
            body: JSON.stringify({ reason: reason })
        }),
        
        update: (id, data) => API.request(`/api/v1/admin/organizations/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        })
    },
    
    // Federation Nodes
    federation: {
        list: () => API.request('/api/v1/admin/federation/nodes'),
        get: (id) => API.request(`/api/v1/admin/federation/nodes/${id}`),
        create: (data) => API.request('/api/v1/admin/federation/nodes', {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        update: (id, data) => API.request(`/api/v1/admin/federation/nodes/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        }),
        delete: (id) => API.request(`/api/v1/admin/federation/nodes/${id}`, {
            method: 'DELETE'
        })
    },
    
    // Users API
    users: {
        list: (params = {}, signal = null) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/users?${query}`, { signal });
        },
        
        get: (id) => API.request(`/api/v1/admin/users/${id}`),
        
        create: (data) => API.request('/api/v1/admin/users', {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        
        update: (id, data) => API.request(`/api/v1/admin/users/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        }),
        
        delete: (id) => API.request(`/api/v1/admin/users/${id}`, {
            method: 'DELETE'
        }),
        
        activate: (id, onRetry) => API.requestWithRetry(
            `/api/v1/admin/users/${id}/activate`, 
            { method: 'POST' },
            {},
            onRetry
        ),
        
        deactivate: (id, onRetry) => API.requestWithRetry(
            `/api/v1/admin/users/${id}/deactivate`, 
            { method: 'POST' },
            {},
            onRetry
        ),
        
        resendInvitation: (id, onRetry) => API.requestWithRetry(
            `/api/v1/admin/users/${id}/resend-invitation`, 
            { method: 'POST' },
            {},
            onRetry
        ),
        
        getActivity: (id, params = {}, signal = null) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/users/${id}/activity?${query}`, { signal });
        }
    },
    
    // Review Queue API
    reviewQueue: {
        list: (params = {}) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/review-queue?${query}`);
        },
        
        get: (id) => API.request(`/api/v1/admin/review-queue/${id}`),
        
        approve: (id, data = {}) => API.request(`/api/v1/admin/review-queue/${id}/approve`, {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        
        reject: (id, data) => API.request(`/api/v1/admin/review-queue/${id}/reject`, {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        
        fix: (id, data) => API.request(`/api/v1/admin/review-queue/${id}/fix`, {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        
        merge: (id, primaryEventId) => API.request(`/api/v1/admin/review-queue/${id}/merge`, {
            method: 'POST',
            body: JSON.stringify({ primary_event_id: primaryEventId })
        })
    },
    
    // Developers API
    developers: {
        list: (params = {}, signal = null) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/developers?${query}`, { signal });
        },
        
        get: (id) => API.request(`/api/v1/admin/developers/${id}`),
        
        invite: (data) => API.request('/api/v1/admin/developers/invite', {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        
        update: (id, data) => API.request(`/api/v1/admin/developers/${id}`, {
            method: 'PUT',
            body: JSON.stringify(data)
        }),
        
        delete: (id) => API.request(`/api/v1/admin/developers/${id}`, {
            method: 'DELETE'
        })
    }
};
