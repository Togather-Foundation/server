/**
 * API Keys Management Page
 * Handles loading, creating, and revoking API keys
 */
(function() {
    'use strict';
    
    let currentKeyIdToRevoke = null;
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        loadApiKeys();
        setupEventListeners();
    }
    
    function setupEventListeners() {
        // Create key form submission
        document.getElementById('create-key-form').addEventListener('submit', handleCreateKey);
        
        // Revoke confirmation
        document.getElementById('confirm-revoke').addEventListener('click', handleRevokeConfirm);
    }
    
    /**
     * Load API keys from backend
     */
    async function loadApiKeys() {
        const tbody = document.getElementById('api-keys-table');
        
        try {
            // Show loading state
            tbody.innerHTML = `
                <tr>
                    <td colspan="5" class="text-center py-4">
                        <div class="spinner-border spinner-border-sm" role="status">
                            <span class="visually-hidden">Loading...</span>
                        </div>
                    </td>
                </tr>
            `;
            
            const data = await API.apiKeys.list();
            
            // Backend returns {items: [...]} envelope format
            const keys = data.items || [];
            
            if (keys.length === 0) {
                tbody.innerHTML = `
                    <tr>
                        <td colspan="5" class="text-center text-muted py-4">
                            No API keys found. Create one to get started.
                        </td>
                    </tr>
                `;
                return;
            }
            
            renderApiKeys(keys);
        } catch (error) {
            console.error('Failed to load API keys:', error);
            tbody.innerHTML = `
                <tr>
                    <td colspan="5" class="text-center text-danger py-4">
                        Failed to load API keys: ${escapeHtml(error.message)}
                    </td>
                </tr>
            `;
            showToast(error.message, 'error');
        }
    }
    
    /**
     * Render API keys table rows
     * @param {Array} keys - Array of API key objects
     */
    function renderApiKeys(keys) {
        const tbody = document.getElementById('api-keys-table');
        
        tbody.innerHTML = keys.map(key => {
            const keyPrefix = key.key_prefix || (key.key ? key.key.substring(0, 8) + '...' : 'N/A');
            const lastUsed = key.last_used_at ? formatDate(key.last_used_at) : 'Never';
            
            return `
                <tr>
                    <td>
                        <span class="font-monospace">${escapeHtml(keyPrefix)}</span>
                    </td>
                    <td>
                        <div>${escapeHtml(key.name)}</div>
                        ${key.description ? `<div class="text-muted small">${escapeHtml(key.description)}</div>` : ''}
                    </td>
                    <td>${formatDate(key.created_at)}</td>
                    <td>
                        ${lastUsed === 'Never' ? '<span class="text-muted">Never</span>' : lastUsed}
                    </td>
                    <td>
                        <div class="btn-list flex-nowrap">
                            <button class="btn btn-sm btn-ghost-danger" onclick="revokeApiKey('${key.id}', '${escapeHtml(key.name)}')">
                                Revoke
                            </button>
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    }
    
    /**
     * Handle create key form submission
     * @param {Event} e - Form submit event
     */
    async function handleCreateKey(e) {
        e.preventDefault();
        
        const form = e.target;
        const submitBtn = form.querySelector('button[type="submit"]');
        
        try {
            // Get form data
            const formData = new FormData(form);
            const data = {
                name: formData.get('name').trim(),
                description: formData.get('description').trim()
            };
            
            // Validate
            if (!data.name) {
                showToast('Key name is required', 'error');
                return;
            }
            
            // Show loading state
            setLoading(submitBtn, true);
            
            // Create key
            const result = await API.apiKeys.create(data);
            
            // Hide create modal
            const createModal = bootstrap.Modal.getInstance(document.getElementById('create-key-modal'));
            createModal.hide();
            
            // Show the new key in a modal (only shown once!)
            showNewKeyModal(result.key || result.api_key);
            
            // Reset form
            form.reset();
            
            // Reload keys list
            await loadApiKeys();
            
            showToast('API key created successfully', 'success');
        } catch (error) {
            console.error('Failed to create API key:', error);
            showToast(error.message, 'error');
        } finally {
            setLoading(submitBtn, false);
        }
    }
    
    /**
     * Show new key in modal (only shown once)
     * @param {string} key - Full API key
     */
    function showNewKeyModal(key) {
        const modal = document.getElementById('show-key-modal');
        const input = document.getElementById('new-key-value');
        
        input.value = key;
        
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
    }
    
    /**
     * Copy new key to clipboard
     */
    window.copyNewKey = async function() {
        const input = document.getElementById('new-key-value');
        await copyToClipboard(input.value);
    };
    
    /**
     * Show revoke confirmation modal
     * @param {string} keyId - API key ID
     * @param {string} keyName - API key name
     */
    window.revokeApiKey = function(keyId, keyName) {
        currentKeyIdToRevoke = keyId;
        
        const modal = document.getElementById('revoke-modal');
        const message = document.getElementById('revoke-message');
        
        message.textContent = `This will immediately revoke the API key "${keyName}". Any services using this key will no longer be able to authenticate.`;
        
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
    };
    
    /**
     * Handle revoke confirmation
     */
    async function handleRevokeConfirm() {
        if (!currentKeyIdToRevoke) return;
        
        const confirmBtn = document.getElementById('confirm-revoke');
        
        try {
            setLoading(confirmBtn, true);
            
            await API.apiKeys.revoke(currentKeyIdToRevoke);
            
            // Hide modal
            const modal = bootstrap.Modal.getInstance(document.getElementById('revoke-modal'));
            modal.hide();
            
            // Reload keys
            await loadApiKeys();
            
            showToast('API key revoked successfully', 'success');
        } catch (error) {
            console.error('Failed to revoke API key:', error);
            showToast(error.message, 'error');
        } finally {
            setLoading(confirmBtn, false);
            currentKeyIdToRevoke = null;
        }
    }
})();
