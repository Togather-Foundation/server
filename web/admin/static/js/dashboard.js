// SEL Admin Dashboard JavaScript
document.addEventListener('DOMContentLoaded', () => {
    loadDashboardStats();
});

async function loadDashboardStats() {
    try {
        const response = await fetch('/api/v1/admin/events/pending', {
            credentials: 'include'
        });
        if (response.ok) {
            const data = await response.json();
            document.getElementById('pending-count').textContent = data.items?.length || 0;
        }
    } catch (err) {
        console.error('Failed to load stats:', err);
    }
}
