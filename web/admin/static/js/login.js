// Admin login form handler
document.addEventListener('DOMContentLoaded', function() {
    const form = document.querySelector('form');
    const errorDiv = document.getElementById('error-message');

    form.addEventListener('submit', async function(e) {
        e.preventDefault();
        
        // Clear previous errors
        if (errorDiv) {
            errorDiv.textContent = '';
            errorDiv.style.display = 'none';
        }

        const username = document.getElementById('username').value;
        const password = document.getElementById('password').value;

        try {
            const response = await fetch('/api/v1/admin/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    username: username,
                    password: password
                })
            });

            const data = await response.json();

            if (response.ok) {
                // Store JWT token in localStorage for API calls
                if (data.token) {
                    localStorage.setItem('admin_token', data.token);
                }
                
                // Login successful - redirect to dashboard
                window.location.href = '/admin/dashboard';
            } else {
                // Show error message
                if (errorDiv) {
                    errorDiv.textContent = data.detail || 'Login failed. Please check your credentials.';
                    errorDiv.style.display = 'block';
                } else {
                    alert(data.detail || 'Login failed. Please check your credentials.');
                }
            }
        } catch (error) {
            console.error('Login error:', error);
            if (errorDiv) {
                errorDiv.textContent = 'Network error. Please try again.';
                errorDiv.style.display = 'block';
            } else {
                alert('Network error. Please try again.');
            }
        }
    });
});
