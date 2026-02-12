/**
 * Developer Login Form Handler
 * Handles developer portal authentication
 */
(function() {
    'use strict';
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', init);
    
    function init() {
        setupEventListeners();
        checkOAuthErrors();
    }
    
    /**
     * Setup event listeners
     */
    function setupEventListeners() {
        const form = document.getElementById('login-form');
        if (form) {
            form.addEventListener('submit', handleSubmit);
        }
    }
    
    /**
     * Handle form submission
     * @param {Event} e - Submit event
     */
    async function handleSubmit(e) {
        e.preventDefault();
        
        const form = e.target;
        const email = document.getElementById('email').value;
        const password = document.getElementById('password').value;
        const submitBtn = document.getElementById('submit-btn');
        const errorDiv = document.getElementById('error-message');
        
        // Clear previous errors
        if (errorDiv) {
            errorDiv.textContent = '';
            errorDiv.style.display = 'none';
        }
        
        // Show loading state
        setLoading(submitBtn, true);
        
        try {
            const response = await fetch('/api/v1/dev/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    email: email,
                    password: password
                })
            });
            
            const data = await response.json();
            
            if (response.ok) {
                // Store JWT token in localStorage for API calls
                if (data.token) {
                    localStorage.setItem('dev_token', data.token);
                }
                
                // Login successful - redirect to dashboard
                window.location.href = '/dev/dashboard';
            } else {
                // Show error message
                const message = data.detail || 'Login failed. Please check your credentials.';
                if (errorDiv) {
                    errorDiv.textContent = message;
                    errorDiv.style.display = 'block';
                } else {
                    alert(message);
                }
                setLoading(submitBtn, false);
            }
        } catch (error) {
            console.error('Login error:', error);
            const message = 'Network error. Please try again.';
            if (errorDiv) {
                errorDiv.textContent = message;
                errorDiv.style.display = 'block';
            } else {
                alert(message);
            }
            setLoading(submitBtn, false);
        }
    }
    
    /**
     * Check for OAuth error parameters in URL
     */
    function checkOAuthErrors() {
        const urlParams = new URLSearchParams(window.location.search);
        const error = urlParams.get('error');
        
        if (error) {
            const errorDiv = document.getElementById('error-message');
            let message = '';
            
            switch (error) {
                case 'oauth_failed':
                    message = 'GitHub authentication failed. Please try again.';
                    break;
                case 'no_email':
                    message = 'Your GitHub account does not have a public email. Please make your email public in GitHub settings or use email/password login.';
                    break;
                case 'account_inactive':
                    message = 'Your account is inactive. Please contact your administrator.';
                    break;
                default:
                    message = 'Authentication error. Please try again.';
            }
            
            if (errorDiv) {
                errorDiv.textContent = message;
                errorDiv.style.display = 'block';
            }
            
            // Clean up URL without reloading
            const url = new URL(window.location);
            url.searchParams.delete('error');
            window.history.replaceState({}, '', url);
        }
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
            element.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>Signing in...';
        } else {
            element.disabled = false;
            element.innerHTML = element.dataset.originalText;
        }
    }
    
})();
