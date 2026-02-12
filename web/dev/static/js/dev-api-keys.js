// SEL Developer Portal - API Keys Management Page
// Handles API key creation, listing, and revocation

(function() {
    'use strict';
    
    let currentKeys = [];
    let keyToRevoke = null;
    
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        setupEventHandlers();
        loadAPIKeys();
        setupLogout(); // From components.js
    }
    
    function setupEventHandlers() {
        // Create key form submission
        const createForm = document.getElementById('create-key-form');
        if (createForm) {
            createForm.addEventListener('submit', handleCreateKey);
        }
        
        // Copy new key button
        document.addEventListener('click', (e) => {
            const target = e.target.closest('[data-action="copy-new-key"]');
            if (target) {
                handleCopyNewKey();
            }
        });
        
        // Revoke confirmation
        const confirmRevoke = document.getElementById('confirm-revoke');
        if (confirmRevoke) {
            confirmRevoke.addEventListener('click', handleConfirmRevoke);
        }
        
        // Event delegation for table actions
        document.addEventListener('click', (e) => {
            const target = e.target.closest('[data-action]');
            if (!target) return;
            
            const action = target.dataset.action;
            const keyId = target.dataset.keyId;
            
            if (action === 'copy-prefix') {
                handleCopyPrefix(keyId);
            } else if (action === 'revoke') {
                handleRevokeClick(keyId);
            }
        });
        
        // Reload keys when create modal is closed
        const createModal = document.getElementById('create-key-modal');
        if (createModal) {
            createModal.addEventListener('hidden.bs.modal', () => {
                document.getElementById('create-key-form').reset();
            });
        }
        
        // Reload keys when show key modal is closed
        const showKeyModal = document.getElementById('show-key-modal');
        if (showKeyModal) {
            showKeyModal.addEventListener('hidden.bs.modal', () => {
                loadAPIKeys(); // Refresh the list
            });
        }
    }
    
    async function loadAPIKeys() {
        try {
            const response = await DevAPI.apiKeys.list();
            const keys = response.items || response;
            currentKeys = keys;
            
            // Load usage data for each key (for sparklines)
            const keysWithUsage = await Promise.all(keys.map(async key => {
                try {
                    const usage = await DevAPI.apiKeys.getUsage(key.id);
                    return { ...key, usageData: usage };
                } catch (err) {
                    console.error(`Failed to load usage for key ${key.id}:`, err);
                    return { ...key, usageData: null };
                }
            }));
            
            renderAPIKeys(keysWithUsage);
        } catch (err) {
            console.error('Failed to load API keys:', err);
            showToast('Failed to load API keys', 'error');
            renderEmptyState('Failed to load API keys. Please try again.');
        }
    }
    
    function renderAPIKeys(keys) {
        const tbody = document.getElementById('api-keys-table');
        
        if (!keys || keys.length === 0) {
            renderEmptyState('No API keys yet. Create your first key to get started.');
            return;
        }
        
        tbody.innerHTML = keys.map(key => {
            const createdDate = formatDate(key.created_at);
            const lastUsed = key.last_used_at ? formatDate(key.last_used_at) : 'Never';
            const statusBadge = key.is_active 
                ? '<span class="badge bg-success-lt">Active</span>'
                : '<span class="badge bg-secondary-lt">Revoked</span>';
            const requestCount = key.usage_30d || 0;
            const sparkline = renderSparkline(key.usageData);
            
            return `
                <tr>
                    <td>
                        <div class="fw-bold">${escapeHtml(key.name)}</div>
                        ${key.description ? `<div class="text-muted small">${escapeHtml(key.description)}</div>` : ''}
                    </td>
                    <td>
                        <code class="font-monospace">${escapeHtml(key.prefix)}</code>
                    </td>
                    <td>${statusBadge}</td>
                    <td>${createdDate}</td>
                    <td>${lastUsed}</td>
                    <td>
                        <div class="d-flex align-items-center">
                            <div class="me-2">${formatNumber(requestCount)}</div>
                            ${sparkline}
                        </div>
                    </td>
                    <td>
                        <div class="btn-list">
                            <button class="btn btn-sm btn-icon" 
                                    data-action="copy-prefix" 
                                    data-key-id="${key.id}"
                                    title="Copy key prefix">
                                <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                    <rect x="8" y="8" width="12" height="12" rx="2"/>
                                    <path d="M16 8v-2a2 2 0 0 0 -2 -2h-8a2 2 0 0 0 -2 2v8a2 2 0 0 0 2 2h2"/>
                                </svg>
                            </button>
                            ${key.is_active ? `
                            <button class="btn btn-sm btn-icon text-danger" 
                                    data-action="revoke" 
                                    data-key-id="${key.id}"
                                    title="Revoke key">
                                <svg xmlns="http://www.w3.org/2000/svg" class="icon" width="24" height="24" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" fill="none">
                                    <path stroke="none" d="M0 0h24v24H0z" fill="none"/>
                                    <line x1="18" y1="6" x2="6" y2="18"/>
                                    <line x1="6" y1="6" x2="18" y2="18"/>
                                </svg>
                            </button>
                            ` : ''}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    }
    
    /**
     * Render a usage sparkline (last 30 days)
     * @param {Object} usageData - Usage data from API
     * @returns {string} SVG sparkline HTML
     */
    function renderSparkline(usageData) {
        if (!usageData || !usageData.daily || usageData.daily.length === 0) {
            return '<span class="text-muted small">—</span>';
        }
        
        const daily = usageData.daily;
        const values = daily.map(d => d.requests);
        const maxValue = Math.max(...values, 1); // Avoid division by zero
        
        // SVG dimensions
        const width = 100;
        const height = 24;
        const barWidth = width / values.length;
        
        // Generate bars
        const bars = values.map((value, index) => {
            const barHeight = (value / maxValue) * height;
            const x = index * barWidth;
            const y = height - barHeight;
            return `<rect x="${x}" y="${y}" width="${barWidth - 1}" height="${barHeight}" fill="currentColor" opacity="0.6"/>`;
        }).join('');
        
        return `<svg width="${width}" height="${height}" class="text-primary" style="display: inline-block; vertical-align: middle;">${bars}</svg>`;
    }
    
    function renderEmptyState(message) {
        const tbody = document.getElementById('api-keys-table');
        tbody.innerHTML = `
            <tr>
                <td colspan="7" class="text-center py-4 text-muted">
                    ${escapeHtml(message)}
                </td>
            </tr>
        `;
    }
    
    async function handleCreateKey(e) {
        e.preventDefault();
        
        const form = e.target;
        const submitBtn = document.getElementById('create-key-btn');
        const formData = new FormData(form);
        
        const data = {
            name: formData.get('name'),
            description: formData.get('description') || undefined
        };
        
        setLoading(submitBtn, true);
        
        try {
            const result = await DevAPI.apiKeys.create(data);
            
            // Hide create modal
            const createModal = bootstrap.Modal.getInstance(document.getElementById('create-key-modal'));
            createModal.hide();
            
            // Show the new key in a modal (ONLY TIME IT'S VISIBLE)
            showNewKeyModal(result.key);
            
            showToast('API key created successfully', 'success');
        } catch (err) {
            console.error('Failed to create API key:', err);
            showToast(err.message || 'Failed to create API key', 'error');
        } finally {
            setLoading(submitBtn, false);
        }
    }
    
    function showNewKeyModal(apiKey) {
        const modal = new bootstrap.Modal(document.getElementById('show-key-modal'));
        document.getElementById('new-key-value').value = apiKey;
        modal.show();
    }
    
    function handleCopyNewKey() {
        const input = document.getElementById('new-key-value');
        copyToClipboard(input.value, 'API key copied to clipboard');
    }
    
    function handleCopyPrefix(keyId) {
        const key = currentKeys.find(k => k.id === keyId);
        if (key) {
            copyToClipboard(key.prefix, 'Key prefix copied to clipboard');
        }
    }
    
    function handleRevokeClick(keyId) {
        const key = currentKeys.find(k => k.id === keyId);
        if (!key) return;
        
        keyToRevoke = keyId;
        
        const message = `Are you sure you want to revoke "${key.name}"? This action cannot be undone.`;
        document.getElementById('revoke-message').textContent = message;
        
        const modal = new bootstrap.Modal(document.getElementById('revoke-modal'));
        modal.show();
    }
    
    async function handleConfirmRevoke() {
        if (!keyToRevoke) return;
        
        const confirmBtn = document.getElementById('confirm-revoke');
        setLoading(confirmBtn, true);
        
        try {
            await DevAPI.apiKeys.revoke(keyToRevoke);
            
            showToast('API key revoked successfully', 'success');
            
            // Close modal
            const modal = bootstrap.Modal.getInstance(document.getElementById('revoke-modal'));
            modal.hide();
            
            // Reload keys
            await loadAPIKeys();
        } catch (err) {
            console.error('Failed to revoke API key:', err);
            showToast(err.message || 'Failed to revoke API key', 'error');
        } finally {
            setLoading(confirmBtn, false);
            keyToRevoke = null;
        }
    }
    
    function copyToClipboard(text, successMessage) {
        navigator.clipboard.writeText(text).then(() => {
            showToast(successMessage, 'success');
        }).catch(err => {
            console.error('Failed to copy:', err);
            showToast('Failed to copy to clipboard', 'error');
        });
    }
    
    function formatNumber(num) {
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        } else if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
        return num.toString();
    }
})();
