/**
 * Federation Nodes Management Page
 * Handles loading, creating, updating, and deleting federation nodes
 */
(function() {
    'use strict';
    
    let currentNodeIdToDelete = null;
    let filters = {
        status: '',
        syncEnabled: '',
        isOnline: ''
    };
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        loadNodes();
        setupEventListeners();
    }
    
    function setupEventListeners() {
        // Create node form submission
        document.getElementById('create-node-form').addEventListener('submit', handleCreateNode);
        
        // Edit node form submission
        document.getElementById('edit-node-form').addEventListener('submit', handleEditNode);
        
        // Delete confirmation
        document.getElementById('confirm-delete').addEventListener('click', handleDeleteConfirm);
        
        // Filter changes
        document.getElementById('status-filter').addEventListener('change', (e) => {
            filters.status = e.target.value;
            loadNodes();
        });
        
        document.getElementById('sync-filter').addEventListener('change', (e) => {
            filters.syncEnabled = e.target.value;
            loadNodes();
        });
        
        document.getElementById('online-filter').addEventListener('change', (e) => {
            filters.isOnline = e.target.value;
            loadNodes();
        });
        
        // Clear filters
        document.getElementById('clear-filters').addEventListener('click', () => {
            clearFilters();
        });
    }
    
    /**
     * Clear all filters and reload
     */
    function clearFilters() {
        filters = {
            status: '',
            syncEnabled: '',
            isOnline: ''
        };
        
        document.getElementById('status-filter').value = '';
        document.getElementById('sync-filter').value = '';
        document.getElementById('online-filter').value = '';
        
        loadNodes();
    }
    
    /**
     * Load federation nodes from backend
     */
    async function loadNodes() {
        const tbody = document.getElementById('nodes-table');
        
        try {
            // Show loading state
            renderLoadingState(tbody, 7);
            
            // Build query parameters
            const params = {};
            if (filters.status) params.status = filters.status;
            if (filters.syncEnabled) params.sync_enabled = filters.syncEnabled;
            if (filters.isOnline) params.is_online = filters.isOnline;
            
            const data = await API.federation.list(params);
            
            // Backend returns {items: [...]} envelope format
            const nodes = data.items || [];
            
            if (nodes.length === 0) {
                renderEmptyState(tbody, 'No federation nodes found. Create one to get started.', 7);
                return;
            }
            
            renderNodes(nodes);
        } catch (error) {
            console.error('Failed to load federation nodes:', error);
            tbody.innerHTML = `
                <tr>
                    <td colspan="7" class="text-center text-danger py-4">
                        Failed to load federation nodes: ${escapeHtml(error.message)}
                    </td>
                </tr>
            `;
            showToast(error.message, 'error');
        }
    }
    
    /**
     * Render federation nodes table rows
     * @param {Array} nodes - Array of node objects
     */
    function renderNodes(nodes) {
        const tbody = document.getElementById('nodes-table');
        
        tbody.innerHTML = nodes.map(node => {
            const statusColor = getStatusColor(node.federation_status || 'pending');
            const statusTextColor = getStatusTextColor(node.federation_status || 'pending');
            const trustBadge = getTrustBadge(node.trust_level || 5);
            const syncStatus = node.sync_enabled ? 'Enabled' : 'Disabled';
            const syncColor = node.sync_enabled ? 'success' : 'secondary';
            const lastSync = node.last_successful_sync_at ? formatDate(node.last_successful_sync_at) : 'Never';
            const onlineIndicator = node.is_online 
                ? '<span class="badge bg-success-lt">Online</span>' 
                : '<span class="badge bg-danger-lt">Offline</span>';
            
            return `
                <tr>
                    <td>
                        <div class="d-flex align-items-center">
                            <div>
                                <div>${escapeHtml(node.node_name || 'Unnamed')}</div>
                                ${node.geographic_scope ? `<div class="text-muted small">${escapeHtml(node.geographic_scope)}</div>` : ''}
                            </div>
                        </div>
                    </td>
                    <td>
                        <span class="font-monospace small">${escapeHtml(node.node_domain || 'N/A')}</span>
                        <div class="mt-1">${onlineIndicator}</div>
                    </td>
                    <td>
                        <span class="badge bg-${statusColor} ${statusTextColor}">${escapeHtml(node.federation_status || 'pending')}</span>
                    </td>
                    <td>
                        ${trustBadge}
                    </td>
                    <td>
                        <span class="badge bg-${syncColor}">${syncStatus}</span>
                        ${node.sync_direction ? `<div class="text-muted small mt-1">${escapeHtml(node.sync_direction)}</div>` : ''}
                    </td>
                    <td class="text-muted small">${lastSync}</td>
                    <td>
                        <div class="btn-list flex-nowrap">
                            <button class="btn btn-sm" onclick="editNode('${node.id}')">
                                Edit
                            </button>
                            <button class="btn btn-sm btn-ghost-danger" onclick="deleteNode('${node.id}', '${escapeHtml(node.node_name || 'Unnamed').replace(/'/g, "&#39;")}')">
                                Delete
                            </button>
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    }
    
    /**
     * Get badge color for federation status
     * @param {string} status - Federation status
     * @returns {string} Bootstrap color class
     */
    function getStatusColor(status) {
        const colors = {
            'active': 'success',
            'pending': 'warning',
            'paused': 'secondary',
            'blocked': 'danger'
        };
        return colors[status] || 'secondary';
    }
    
    /**
     * Get text color class for status badge (ensures readability in both themes)
     * @param {string} status - Federation status
     * @returns {string} Text color class
     */
    function getStatusTextColor(status) {
        // States with light backgrounds need dark text in both themes
        const needsDarkText = ['pending']; // warning badge has yellow/light background
        return needsDarkText.includes(status) ? 'text-dark' : '';
    }
    
    /**
     * Get trust level badge
     * @param {number} trustLevel - Trust level (1-10)
     * @returns {string} HTML badge
     */
    function getTrustBadge(trustLevel) {
        let color = 'secondary';
        let label = 'Unknown';
        let textClass = ''; // For readability in both themes
        
        if (trustLevel >= 1 && trustLevel <= 3) {
            color = 'danger';
            label = `Low (${trustLevel})`;
        } else if (trustLevel >= 4 && trustLevel <= 6) {
            color = 'warning';
            label = `Medium (${trustLevel})`;
            textClass = 'text-dark'; // Yellow background needs dark text
        } else if (trustLevel >= 7 && trustLevel <= 9) {
            color = 'success';
            label = `High (${trustLevel})`;
        } else if (trustLevel === 10) {
            color = 'primary';
            label = 'Maximum (10)';
        }
        
        return `<span class="badge bg-${color} ${textClass}">${label}</span>`;
    }
    
    /**
     * Handle create node form submission
     * @param {Event} e - Form submit event
     */
    async function handleCreateNode(e) {
        e.preventDefault();
        
        const form = e.target;
        const submitBtn = form.querySelector('button[type="submit"]');
        
        try {
            // Get form data
            const formData = new FormData(form);
            const data = {
                node_name: formData.get('node_name').trim(),
                node_domain: formData.get('node_domain').trim(),
                base_url: formData.get('base_url').trim(),
                api_version: formData.get('api_version').trim() || 'v1',
                trust_level: parseInt(formData.get('trust_level'), 10),
                federation_status: formData.get('federation_status'),
                sync_enabled: formData.get('sync_enabled') === 'on',
                sync_direction: formData.get('sync_direction')
            };
            
            // Add optional fields
            const geographicScope = formData.get('geographic_scope').trim();
            if (geographicScope) data.geographic_scope = geographicScope;
            
            const contactName = formData.get('contact_name').trim();
            if (contactName) data.contact_name = contactName;
            
            const contactEmail = formData.get('contact_email').trim();
            if (contactEmail) data.contact_email = contactEmail;
            
            const notes = formData.get('notes').trim();
            if (notes) data.notes = notes;
            
            // Validate required fields
            if (!data.node_name || !data.node_domain || !data.base_url) {
                showToast('Please fill in all required fields', 'error');
                return;
            }
            
            // Show loading state
            setLoading(submitBtn, true);
            
            // Create node
            await API.federation.create(data);
            
            // Hide create modal
            const createModal = bootstrap.Modal.getInstance(document.getElementById('create-node-modal'));
            createModal.hide();
            
            // Reset form
            form.reset();
            
            // Reload nodes list
            await loadNodes();
            
            showToast('Federation node created successfully', 'success');
        } catch (error) {
            console.error('Failed to create federation node:', error);
            showToast(error.message, 'error');
        } finally {
            setLoading(submitBtn, false);
        }
    }
    
    /**
     * Show edit node modal with current values
     * @param {string} nodeId - Node UUID
     */
    window.editNode = async function(nodeId) {
        try {
            const node = await API.federation.get(nodeId);
            
            // Populate form
            document.getElementById('edit-node-id').value = node.id;
            document.getElementById('edit-node-name').value = node.node_name || '';
            document.getElementById('edit-node-domain').value = node.node_domain || '';
            document.getElementById('edit-base-url').value = node.base_url || '';
            document.getElementById('edit-api-version').value = node.api_version || '';
            document.getElementById('edit-trust-level').value = node.trust_level || 5;
            document.getElementById('edit-geographic-scope').value = node.geographic_scope || '';
            document.getElementById('edit-federation-status').value = node.federation_status || 'pending';
            document.getElementById('edit-sync-direction').value = node.sync_direction || 'bidirectional';
            document.getElementById('edit-sync-enabled').checked = node.sync_enabled || false;
            document.getElementById('edit-contact-name').value = node.contact_name || '';
            document.getElementById('edit-contact-email').value = node.contact_email || '';
            document.getElementById('edit-notes').value = node.notes || '';
            
            // Show modal
            const modal = new bootstrap.Modal(document.getElementById('edit-node-modal'));
            modal.show();
        } catch (error) {
            console.error('Failed to load node:', error);
            showToast(error.message, 'error');
        }
    };
    
    /**
     * Handle edit node form submission
     * @param {Event} e - Form submit event
     */
    async function handleEditNode(e) {
        e.preventDefault();
        
        const form = e.target;
        const submitBtn = form.querySelector('button[type="submit"]');
        const nodeId = document.getElementById('edit-node-id').value;
        
        try {
            // Get form data
            const formData = new FormData(form);
            const data = {
                node_name: formData.get('node_name').trim(),
                base_url: formData.get('base_url').trim(),
                api_version: formData.get('api_version').trim() || null,
                trust_level: parseInt(formData.get('trust_level'), 10),
                federation_status: formData.get('federation_status'),
                sync_enabled: formData.get('sync_enabled') === 'on',
                sync_direction: formData.get('sync_direction')
            };
            
            // Add optional fields
            const geographicScope = formData.get('geographic_scope').trim();
            data.geographic_scope = geographicScope || null;
            
            const contactName = formData.get('contact_name').trim();
            data.contact_name = contactName || null;
            
            const contactEmail = formData.get('contact_email').trim();
            data.contact_email = contactEmail || null;
            
            const notes = formData.get('notes').trim();
            data.notes = notes || null;
            
            // Show loading state
            setLoading(submitBtn, true);
            
            // Update node
            await API.federation.update(nodeId, data);
            
            // Hide edit modal
            const editModal = bootstrap.Modal.getInstance(document.getElementById('edit-node-modal'));
            editModal.hide();
            
            // Reload nodes list
            await loadNodes();
            
            showToast('Federation node updated successfully', 'success');
        } catch (error) {
            console.error('Failed to update federation node:', error);
            showToast(error.message, 'error');
        } finally {
            setLoading(submitBtn, false);
        }
    }
    
    /**
     * Show delete confirmation modal
     * @param {string} nodeId - Node UUID
     * @param {string} nodeName - Node name
     */
    window.deleteNode = function(nodeId, nodeName) {
        currentNodeIdToDelete = nodeId;
        
        const modal = document.getElementById('delete-modal');
        const nodeNameSpan = document.getElementById('delete-node-name');
        
        nodeNameSpan.textContent = nodeName;
        
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
    };
    
    /**
     * Handle delete confirmation
     */
    async function handleDeleteConfirm() {
        if (!currentNodeIdToDelete) return;
        
        const confirmBtn = document.getElementById('confirm-delete');
        
        try {
            setLoading(confirmBtn, true);
            
            await API.federation.delete(currentNodeIdToDelete);
            
            // Hide modal
            const modal = bootstrap.Modal.getInstance(document.getElementById('delete-modal'));
            modal.hide();
            
            // Reload nodes
            await loadNodes();
            
            showToast('Federation node deleted successfully', 'success');
        } catch (error) {
            console.error('Failed to delete federation node:', error);
            showToast(error.message, 'error');
        } finally {
            setLoading(confirmBtn, false);
            currentNodeIdToDelete = null;
        }
    }
})();
