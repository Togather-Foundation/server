/**
 * Developer Accept Invitation Page JavaScript
 * Handles invitation token validation and password setup
 */
(function() {
    'use strict';
    
    let invitationToken = null;
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        // Extract token from URL query parameter
        const urlParams = new URLSearchParams(window.location.search);
        invitationToken = urlParams.get('token');
        
        if (!invitationToken) {
            showError('No invitation token provided. Please use the link from your invitation email.');
            return;
        }
        
        // Show loading state while "verifying" token
        // Note: We don't have a backend endpoint to pre-verify tokens, so we show
        // the form after a brief delay to give the user feedback that something is happening.
        // The actual token validation happens on form submission.
        setTimeout(() => {
            showForm();
            setupEventListeners();
        }, 500);
    }
    
    /**
     * Setup event listeners
     */
    function setupEventListeners() {
        const form = document.getElementById('accept-invitation-form');
        if (form) {
            form.addEventListener('submit', handleSubmit);
        }
        
        // Password strength indicator
        const passwordInput = document.getElementById('password');
        if (passwordInput) {
            passwordInput.addEventListener('input', updatePasswordStrength);
        }
        
        // Confirm password validation
        const confirmPasswordInput = document.getElementById('confirm-password');
        if (confirmPasswordInput) {
            confirmPasswordInput.addEventListener('input', validatePasswordMatch);
        }
    }
    
    /**
     * Show the form
     */
    function showForm() {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('accept-invitation-form').style.display = 'block';
    }
    
    /**
     * Show error state
     * @param {string} message - Error message
     */
    function showError(message) {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('error-state').style.display = 'block';
        document.getElementById('error-message').textContent = message;
    }
    
    /**
     * Show success state
     */
    function showSuccess() {
        document.getElementById('loading-state').style.display = 'none';
        document.getElementById('accept-invitation-form').style.display = 'none';
        document.getElementById('success-state').style.display = 'block';
    }
    
    /**
     * Update password strength indicator
     */
    function updatePasswordStrength() {
        const password = document.getElementById('password').value;
        const strengthBar = document.getElementById('password-strength');
        const strengthText = document.getElementById('password-strength-text');
        
        if (!password) {
            strengthBar.style.width = '0%';
            strengthBar.className = 'progress-bar';
            strengthText.textContent = 'Password strength: None';
            return;
        }
        
        let strength = 0;
        let feedback = [];
        
        // Length check
        if (password.length >= 12) {
            strength += 25;
        } else {
            feedback.push('at least 12 characters');
        }
        
        // Uppercase check
        if (/[A-Z]/.test(password)) {
            strength += 25;
        } else {
            feedback.push('uppercase letter');
        }
        
        // Lowercase check
        if (/[a-z]/.test(password)) {
            strength += 25;
        } else {
            feedback.push('lowercase letter');
        }
        
        // Number check
        if (/[0-9]/.test(password)) {
            strength += 12.5;
        } else {
            feedback.push('number');
        }
        
        // Special character check
        if (/[^A-Za-z0-9]/.test(password)) {
            strength += 12.5;
        } else {
            feedback.push('special character');
        }
        
        // Update UI
        strengthBar.style.width = strength + '%';
        
        // Only show "Strong" when password meets ALL requirements (100%)
        // Show missing criteria at all strength levels for clarity
        if (strength < 100) {
            // Determine color based on how many requirements are met
            if (strength < 50) {
                strengthBar.className = 'progress-bar bg-danger';
                strengthText.textContent = 'Password strength: Weak - needs: ' + feedback.join(', ');
            } else if (strength < 75) {
                strengthBar.className = 'progress-bar bg-warning';
                strengthText.textContent = 'Password strength: Fair - missing: ' + feedback.join(', ');
            } else {
                strengthBar.className = 'progress-bar bg-info';
                strengthText.textContent = 'Password strength: Almost there - missing: ' + feedback.join(', ');
            }
        } else {
            // All requirements met
            strengthBar.className = 'progress-bar bg-success';
            strengthText.textContent = 'Password strength: Strong ✓ All requirements met';
        }
    }
    
    /**
     * Validate password match
     */
    function validatePasswordMatch() {
        const password = document.getElementById('password').value;
        const confirmPassword = document.getElementById('confirm-password').value;
        const confirmPasswordInput = document.getElementById('confirm-password');
        const confirmPasswordError = document.getElementById('confirm-password-error');
        
        if (confirmPassword && password !== confirmPassword) {
            confirmPasswordInput.classList.add('is-invalid');
            confirmPasswordError.textContent = 'Passwords do not match';
            return false;
        } else {
            confirmPasswordInput.classList.remove('is-invalid');
            confirmPasswordError.textContent = '';
            return true;
        }
    }
    
    /**
     * Validate password requirements
     * @param {string} password - Password to validate
     * @returns {Object} Validation result
     * 
     * IMPORTANT: These requirements MUST match backend validation in:
     * internal/domain/developers/service.go:validatePassword()
     * 
     * Backend requirements (NIST SP 800-63B guidelines):
     * - Minimum 12 characters (ErrPasswordTooShort)
     * - Maximum 128 characters (ErrPasswordTooLong)
     * - At least one uppercase letter
     * - At least one lowercase letter
     * - At least one number
     * - At least one special character (punctuation or symbol)
     */
    function validatePassword(password) {
        const errors = [];
        
        if (password.length < 12) {
            errors.push('Password must be at least 12 characters');
        }
        
        if (!/[A-Z]/.test(password)) {
            errors.push('Password must contain at least one uppercase letter');
        }
        
        if (!/[a-z]/.test(password)) {
            errors.push('Password must contain at least one lowercase letter');
        }
        
        if (!/[0-9]/.test(password)) {
            errors.push('Password must contain at least one number');
        }
        
        if (!/[^A-Za-z0-9]/.test(password)) {
            errors.push('Password must contain at least one special character');
        }
        
        return {
            valid: errors.length === 0,
            errors: errors
        };
    }
    
    /**
     * Handle form submission
     * @param {Event} e - Submit event
     */
    async function handleSubmit(e) {
        e.preventDefault();
        
        const form = e.target;
        const name = document.getElementById('name').value;
        const password = document.getElementById('password').value;
        const confirmPassword = document.getElementById('confirm-password').value;
        const submitBtn = document.getElementById('submit-btn');
        const passwordInput = document.getElementById('password');
        const passwordError = document.getElementById('password-error');
        
        // Reset validation state
        form.classList.remove('was-validated');
        passwordInput.classList.remove('is-invalid');
        
        // Validate password requirements
        const passwordValidation = validatePassword(password);
        if (!passwordValidation.valid) {
            passwordInput.classList.add('is-invalid');
            passwordError.textContent = passwordValidation.errors.join('. ');
            showToast(passwordValidation.errors[0], 'error');
            return;
        }
        
        // Validate password match
        if (password !== confirmPassword) {
            document.getElementById('confirm-password').classList.add('is-invalid');
            document.getElementById('confirm-password-error').textContent = 'Passwords do not match';
            showToast('Passwords do not match', 'error');
            return;
        }
        
        // Show loading state
        setLoading(submitBtn, true);
        
        try {
            // SECURITY NOTE: No CSRF token required for this endpoint.
            // The invitation token itself serves as proof of authorization (single-use, time-limited).
            // This is a public endpoint that doesn't rely on session cookies for authentication.
            const response = await fetch('/api/v1/dev/accept-invitation', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    token: invitationToken,
                    name: name,
                    password: password
                })
            });
            
            if (!response.ok) {
                let error;
                try {
                    error = await response.json();
                } catch {
                    error = { detail: 'Failed to accept invitation' };
                }
                throw new Error(error.detail || error.message || 'Failed to accept invitation');
            }
            
            // Success!
            showSuccess();
            
            // Redirect to login after 3 seconds
            setTimeout(() => {
                window.location.href = '/dev/login';
            }, 3000);
            
        } catch (error) {
            console.error('Failed to accept invitation:', error);
            
            // Check for specific error messages
            if (error.message.includes('expired')) {
                showError('This invitation has expired. Please contact your administrator for a new invitation.');
            } else if (error.message.includes('already accepted') || error.message.includes('already active')) {
                showError('This invitation has already been accepted. You can log in with your credentials.');
            } else if (error.message.includes('invalid token')) {
                showError('This invitation token is invalid. Please check your invitation link or contact your administrator.');
            } else {
                showToast(error.message, 'error');
                setLoading(submitBtn, false);
            }
        }
    }
    
    /**
     * Show toast notification
     * @param {string} message - Toast message
     * @param {string} type - Toast type (success, error, warning, info)
     */
    function showToast(message, type = 'success') {
        const container = document.getElementById('toast-container');
        if (!container) return;
        
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
        
        // Auto-remove after 5 seconds
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 300);
        }, 5000);
    }
    
    /**
     * HTML escaping for XSS protection
     * @param {string} text - Text to escape
     * @returns {string} Escaped HTML
     */
    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
    
    /**
     * Set loading state on button
     * @param {HTMLElement} element - Button element
     * @param {boolean} loading - Loading state
     */
    function setLoading(element, loading) {
        if (loading) {
            element.disabled = true;
            element.dataset.originalText = element.innerHTML;
            element.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>Activating...';
        } else {
            element.disabled = false;
            element.innerHTML = element.dataset.originalText;
        }
    }
    
})();
