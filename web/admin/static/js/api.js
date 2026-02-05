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
    // Base request method
    async request(url, options = {}) {
        // Get JWT token from localStorage
        const token = localStorage.getItem('admin_token');
        
        const response = await fetch(url, {
            ...options,
            headers: {
                'Content-Type': 'application/json',
                // Send Bearer token for API authentication
                ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
                ...options.headers
            },
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
                // Parse RFC 7807 Problem Details error response
                error = await response.json();
            } catch {
                error = { detail: 'Request failed' };
            }
            // Throw an Error with the detail field (specific error message)
            // Fall back to title or generic message if detail is missing
            const err = new Error(error.detail || error.title || error.message || 'Request failed');
            err.status = response.status; // Preserve status code for error handling
            throw err;
        }
        
        return response.json();
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
        
        activate: (id) => API.request(`/api/v1/admin/users/${id}/activate`, {
            method: 'POST'
        }),
        
        deactivate: (id) => API.request(`/api/v1/admin/users/${id}/deactivate`, {
            method: 'POST'
        }),
        
        resendInvitation: (id) => API.request(`/api/v1/admin/users/${id}/resend-invitation`, {
            method: 'POST'
        }),
        
        getActivity: (id, params = {}, signal = null) => {
            const query = new URLSearchParams(params);
            return API.request(`/api/v1/admin/users/${id}/activity?${query}`, { signal });
        }
    }
};
