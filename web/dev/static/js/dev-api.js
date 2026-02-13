// SEL Developer Portal API Client
// Centralized API wrapper for developer portal backend calls
//
// ERROR HANDLING CONTRACT:
// Backend returns RFC 7807 Problem Details for HTTP APIs (https://datatracker.ietf.org/doc/html/rfc7807)
// Error response format:
// {
//   "type": "https://sel.events/problems/validation-error",     // URI identifying the problem type
//   "title": "Validation Error",                                // Human-readable summary
//   "status": 400,                                               // HTTP status code
//   "detail": "Email is required",                               // Human-readable explanation specific to this occurrence
//   "instance": "/api/v1/dev/login"                             // URI reference identifying this specific occurrence (optional)
// }
//
// The `detail` field should be used for displaying error messages to users.
// The `title` field provides a generic category of error (e.g., "Validation Error", "Not Found").

const DevAPI = {
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
        const token = localStorage.getItem('dev_token');
        
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
        
        // Handle empty responses (e.g., 204 No Content for DELETE operations)
        if (response.status === 204 || response.headers.get('content-length') === '0') {
            return null;
        }
        
        // Parse JSON response
        const contentType = response.headers.get('content-type');
        if (contentType && contentType.includes('application/json')) {
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
    
    // Developer API Keys
    apiKeys: {
        list: () => DevAPI.request('/api/v1/dev/api-keys'),
        create: (data) => DevAPI.request('/api/v1/dev/api-keys', {
            method: 'POST',
            body: JSON.stringify(data)
        }),
        revoke: (id) => DevAPI.request(`/api/v1/dev/api-keys/${id}`, {
            method: 'DELETE'
        }),
        getUsage: (id) => DevAPI.request(`/api/v1/dev/api-keys/${id}/usage`)
    }
};
