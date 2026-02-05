// SEL Admin API Client
// Centralized API wrapper for all backend calls

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
            credentials: 'include' // Include cookies for HTML page auth
        });
        
        if (!response.ok) {
            let error;
            try {
                error = await response.json();
            } catch {
                error = { detail: 'Request failed' };
            }
            throw new Error(error.detail || error.message || 'Request failed');
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
    }
};
