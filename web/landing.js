(function () {
    'use strict';

    // Resolve the <this-node> placeholder shown in the agent prompt to this
    // server's real origin so humans and agents both see a working URL.
    function resolveNodePlaceholders() {
        const origin = window.location.origin;
        const nodeUrlEl = document.getElementById('node-url');
        if (nodeUrlEl) {
            nodeUrlEl.textContent = origin;
        }
        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, {
            acceptNode(node) {
                return node.textContent.indexOf('<this-node>') !== -1
                    ? NodeFilter.FILTER_ACCEPT
                    : NodeFilter.FILTER_REJECT;
            }
        });
        while (walker.nextNode()) {
            walker.currentNode.textContent = walker.currentNode.textContent.split('<this-node>').join(origin);
        }
    }

    // Copy the agent prompt to the clipboard. Reads the code block's text
    // content at click time so the <this-node> placeholder has already been
    // resolved to the real origin by resolveNodePlaceholders().
    function setupCopyPrompt() {
        const button = document.querySelector('[data-action="copy-prompt"]');
        if (!button) return;

        const wrapper = button.closest('div');
        if (!wrapper) return;
        const codeBlock = wrapper.querySelector('pre code');
        if (!codeBlock) return;

        function flash(label) {
            const original = button.textContent;
            button.textContent = label;
            button.disabled = true;
            setTimeout(function () {
                button.textContent = original;
                button.disabled = false;
            }, 1500);
        }

        button.addEventListener('click', function () {
            const text = codeBlock.textContent;
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(text).then(
                    function () { flash('Copied!'); },
                    function () { copyFallback(text); }
                );
            } else {
                copyFallback(text);
            }
        });

        function copyFallback(text) {
            // Selection-based fallback for contexts where the Clipboard API is
            // unavailable or permission-denied.
            try {
                const range = document.createRange();
                range.selectNodeContents(codeBlock);
                const selection = window.getSelection();
                selection.removeAllRanges();
                selection.addRange(range);
                const ok = document.execCommand('copy');
                selection.removeAllRanges();
                flash(ok ? 'Copied!' : 'Copy failed');
            } catch (err) {
                flash('Copy failed');
            }
        }
    }

    // Fetch live server stats: health, version, and real entity counts so a
    // visitor immediately sees proof of life.
    async function loadStats() {
        try {
            const [healthRes, versionRes, statsRes] = await Promise.all([
                fetch('/health'),
                fetch('/version'),
                fetch('/api/v1/stats')
            ]);

            const health = await healthRes.json().catch(() => ({}));
            const version = await versionRes.json().catch(() => ({}));
            const stats = await statsRes.json().catch(() => ({}));

            const setText = (id, text, className) => {
                const el = document.getElementById(id);
                if (el) {
                    el.textContent = text;
                    if (className) el.className = 'stat-value ' + className;
                }
            };

            const statusEl = document.getElementById('status');
            if (statusEl) {
                if (health.status === 'healthy') {
                    statusEl.textContent = 'Healthy';
                    statusEl.className = 'stat-value status-healthy';
                } else if (health.status === 'degraded') {
                    statusEl.textContent = 'Degraded';
                    statusEl.className = 'stat-value status-degraded';
                } else {
                    statusEl.textContent = 'Error';
                    statusEl.className = 'stat-value status-error';
                }
            }

            setText('version', version.git_commit
                ? `${version.version} (${version.git_commit.substring(0, 7)})`
                : (version.version || 'Unavailable'));

            if (typeof stats.published_events === 'number') {
                setText('event-count', stats.published_events.toLocaleString());
            } else if (typeof stats.total_events === 'number') {
                setText('event-count', stats.total_events.toLocaleString());
            } else {
                setText('event-count', 'Unavailable', 'status-error');
            }

            if (typeof stats.upcoming_events === 'number') {
                setText('upcoming-count', stats.upcoming_events.toLocaleString());
            }
        } catch (error) {
            console.error('Failed to fetch server stats:', error);
            ['status', 'version', 'event-count', 'upcoming-count'].forEach(function (id) {
                const el = document.getElementById(id);
                if (el) {
                    el.textContent = 'Unavailable';
                    el.className = 'stat-value status-error';
                }
            });
        }
    }

    document.addEventListener('DOMContentLoaded', function () {
        resolveNodePlaceholders();
        setupCopyPrompt();
        loadStats();
    });
})();
