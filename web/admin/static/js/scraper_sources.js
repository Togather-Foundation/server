// Scraper Sources Admin Page (srv-5127b, srv-pfeud)
// Manages scraper source listing, enable/disable, trigger, run history, and auto-scrape toggle.

(function () {
    'use strict';

    document.addEventListener('DOMContentLoaded', init);

    var _sseConn = null;

    function init() {
        setupEventHandlers();
        loadSources();
        loadConfig();
        connectSSE();
    }

    function connectSSE() {
        if (!window.EventSource) {
            // Graceful degradation: browser doesn't support SSE
            return;
        }
        _sseConn = new EventSource('/api/v1/admin/scraper/events');
        var reloadTimer = null;
        _sseConn.onmessage = function (e) {
            try {
                var ev = JSON.parse(e.data);
                if (ev.kind === 'job_completed' || ev.kind === 'job_failed' || ev.kind === 'job_cancelled') {
                    // Debounce: if multiple jobs complete rapidly, reload once
                    if (reloadTimer) { clearTimeout(reloadTimer); }
                    reloadTimer = setTimeout(loadSources, 500);
                }
            } catch (_) {}
        };
        _sseConn.onerror = function () {
            // Browser handles auto-reconnect; log for debugging only
            console.debug('scraper SSE: connection error, browser will retry');
        };
    }

    // -------------------------------------------------------------------------
    // Event delegation
    // -------------------------------------------------------------------------

    function setupEventHandlers() {
        document.addEventListener('click', function (e) {
            var target = e.target.closest('[data-action]');
            if (!target) return;

            var action = target.dataset.action;
            var name = target.dataset.name;

            if (action === 'view-runs') {
                openRunsModal(name);
            } else if (action === 'trigger-scrape') {
                triggerScrape(target, name);
            } else if (action === 'toggle-enabled') {
                toggleEnabled(target, name);
            } else if (action === 'toggle-run-detail') {
                var detailRow = document.getElementById(target.dataset.target);
                if (detailRow) {
                    var isHidden = detailRow.style.display === 'none';
                    detailRow.style.display = isHidden ? '' : 'none';
                    // Flip the arrow indicator
                    var arrow = target.querySelector('[title="Click row for details"]');
                    if (arrow) arrow.textContent = isHidden ? '▾' : '▸';
                }
            }
        });

        document.addEventListener('change', function (e) {
            var target = e.target.closest('[data-action="toggle-auto-scrape"]');
            if (!target) return;
            toggleAutoScrape(target.checked);
        });
    }

    // -------------------------------------------------------------------------
    // Load sources
    // -------------------------------------------------------------------------

    async function loadSources() {
        showState('loading');
        try {
            var data = await API.scraper.listSources();
            var items = data.items || [];
            if (items.length === 0) {
                showState('empty');
                return;
            }
            renderSources(items);
            showState('table');
            document.getElementById('showing-text').textContent =
                'Showing ' + items.length + ' source' + (items.length === 1 ? '' : 's');
        } catch (err) {
            showState('empty');
            showToast('Failed to load scraper sources: ' + err.message, 'error');
        }
    }

    function renderSources(items) {
        var tbody = document.getElementById('sources-table');
        tbody.innerHTML = items.map(renderSourceRow).join('');
    }

    function renderSourceRow(src) {
        var statusBadge = '';
        if (src.last_run_status) {
            var cls = src.last_run_status === 'completed' ? 'bg-success-lt'
                : src.last_run_status === 'failed' ? 'bg-danger-lt'
                : src.last_run_status === 'running' ? 'bg-warning-lt'
                : 'bg-secondary-lt';
            statusBadge = '<span class="badge ' + cls + '">' + escapeHtml(src.last_run_status) + '</span>';
        } else {
            statusBadge = '<span class="badge bg-secondary-lt">never</span>';
        }

        var lastRun = src.last_run_started_at
            ? formatDate(src.last_run_started_at)
            : '—';

        var eventCounts = '—';
        if (src.last_run_status) {
            var newCount = src.last_run_events_new != null ? src.last_run_events_new : 0;
            var dupCount = src.last_run_events_dup != null ? src.last_run_events_dup : 0;
            var failCount = src.last_run_events_failed != null ? src.last_run_events_failed : 0;
            eventCounts = escapeHtml(String(newCount)) + ' / ' +
                escapeHtml(String(dupCount)) + ' / ' +
                (failCount > 0
                    ? '<span class="badge bg-danger-lt">' + escapeHtml(String(failCount)) + '</span>'
                    : escapeHtml(String(failCount)));
        }

        var enabledToggleLabel = src.enabled ? 'Disable' : 'Enable';
        var enabledBtnClass = src.enabled ? 'btn-success' : 'btn-outline-secondary';

        return '<tr>' +
            '<td>' +
                '<div class="font-weight-medium">' + escapeHtml(src.name) + '</div>' +
                '<div class="text-muted small">' + escapeHtml(src.url) + '</div>' +
            '</td>' +
            '<td>' + escapeHtml(String(src.tier)) + '</td>' +
            '<td>' + escapeHtml(src.schedule || '—') + '</td>' +
            '<td class="text-muted small">' + escapeHtml(lastRun) + '</td>' +
            '<td>' + eventCounts + '</td>' +
            '<td>' + statusBadge + '</td>' +
            '<td>' +
                '<button class="btn btn-sm ' + enabledBtnClass + '" ' +
                    'data-action="toggle-enabled" data-name="' + escapeHtml(src.name) + '" ' +
                    'data-enabled="' + String(!src.enabled) + '">' +
                    enabledToggleLabel +
                '</button>' +
            '</td>' +
            '<td>' +
                '<div class="btn-group">' +
                    '<button class="btn btn-sm btn-outline-primary" data-action="trigger-scrape" data-name="' + escapeHtml(src.name) + '"' +
                        (!src.enabled ? ' disabled title="Enable this source before running"' : '') + '>Run</button>' +
                    '<button class="btn btn-sm btn-outline-secondary" data-action="view-runs" data-name="' + escapeHtml(src.name) + '">History</button>' +
                '</div>' +
            '</td>' +
            '</tr>';
    }

    // -------------------------------------------------------------------------
    // Toggle enabled
    // -------------------------------------------------------------------------

    async function toggleEnabled(btn, name) {
        var enabled = btn.dataset.enabled === 'true';
        setLoading(btn, true);
        try {
            await API.scraper.setEnabled(name, enabled);
            showToast('Source ' + (enabled ? 'enabled' : 'disabled') + ': ' + name, 'success');
            loadSources();
        } catch (err) {
            showToast('Failed to update source: ' + err.message, 'error');
            setLoading(btn, false);
        }
    }

    // -------------------------------------------------------------------------
    // Trigger scrape
    // -------------------------------------------------------------------------

    async function triggerScrape(btn, name) {
        setLoading(btn, true);
        try {
            await API.scraper.triggerScrape(name);
            showToast('Scrape triggered for: ' + name, 'success');
            // SSE will update the table when the job completes.
            // Reload once immediately so "running" status appears quickly.
            setTimeout(function () { loadSources(); }, 1000);
        } catch (err) {
            showToast('Failed to trigger scrape: ' + err.message, 'error');
        } finally {
            setLoading(btn, false);
        }
    }

    // -------------------------------------------------------------------------
    // Auto-scrape config toggle (srv-pfeud)
    // -------------------------------------------------------------------------

    async function loadConfig() {
        try {
            var cfg = await API.scraper.getConfig();
            var toggle = document.getElementById('auto-scrape-toggle');
            var wrap = document.getElementById('auto-scrape-toggle-wrap');
            if (toggle && wrap) {
                toggle.checked = cfg.auto_scrape === true;
                wrap.classList.remove('d-none');
            }
        } catch (err) {
            // Config endpoint may not exist in older deployments — fail silently.
            console.warn('scraper config unavailable:', err.message);
        }
    }

    async function toggleAutoScrape(enabled) {
        try {
            await API.scraper.patchConfig({ auto_scrape: enabled });
            showToast('Auto-scrape ' + (enabled ? 'enabled' : 'disabled'), 'success');
        } catch (err) {
            showToast('Failed to update auto-scrape setting: ' + err.message, 'error');
            // Revert checkbox on failure.
            var toggle = document.getElementById('auto-scrape-toggle');
            if (toggle) toggle.checked = !enabled;
        }
    }

    // -------------------------------------------------------------------------
    // Run history modal
    // -------------------------------------------------------------------------

    async function openRunsModal(name) {
        document.getElementById('runs-modal-source-name').textContent = name;
        showRunsState('loading');

        var modalEl = document.getElementById('runs-modal');
        var modal = bootstrap.Modal.getOrCreateInstance(modalEl);
        modal.show();

        try {
            var data = await API.scraper.listRuns(name);
            var runs = data.items || [];
            if (runs.length === 0) {
                showRunsState('empty');
                return;
            }
            renderRuns(runs);
            showRunsState('table');
        } catch (err) {
            showRunsState('empty');
            showToast('Failed to load run history: ' + err.message, 'error');
        }
    }

    function renderRuns(runs) {
        var tbody = document.getElementById('runs-table');
        var rows = [];
        runs.forEach(function (run, i) {
            var cls = run.status === 'completed' ? 'bg-success-lt'
                : run.status === 'failed' ? 'bg-danger-lt'
                : run.status === 'running' ? 'bg-warning-lt'
                : 'bg-secondary-lt';
            var hasError = run.error_message && run.error_message.length > 0;
            var detailId = 'run-detail-' + i;

            // Main row — if there's an error, clicking the row expands it
            var rowAttrs = hasError
                ? ' class="cursor-pointer" data-action="toggle-run-detail" data-target="' + detailId + '"'
                : '';
            var statusCell = hasError
                ? '<span class="badge ' + cls + '">' + escapeHtml(run.status) + '</span>' +
                  ' <span class="text-muted small" title="Click row for details">▸</span>'
                : '<span class="badge ' + cls + '">' + escapeHtml(run.status) + '</span>';

            rows.push(
                '<tr' + rowAttrs + '>' +
                '<td class="text-muted small">' + escapeHtml(run.started_at ? formatDate(run.started_at) : '—') + '</td>' +
                '<td class="text-muted small">' + escapeHtml(run.completed_at ? formatDate(run.completed_at) : '—') + '</td>' +
                '<td>' + statusCell + '</td>' +
                '<td>' + escapeHtml(String(run.events_found)) + '</td>' +
                '<td>' + escapeHtml(String(run.events_new)) + '</td>' +
                '<td>' + escapeHtml(String(run.events_dup)) + '</td>' +
                '<td>' +
                    (run.events_failed > 0
                        ? '<span class="badge bg-danger-lt">' + escapeHtml(String(run.events_failed)) + '</span>'
                        : escapeHtml(String(run.events_failed))) +
                '</td>' +
                '</tr>'
            );

            // Collapsible detail row for error message
            if (hasError) {
                rows.push(
                    '<tr id="' + detailId + '" style="display:none;">' +
                    '<td colspan="7" class="bg-danger-lt">' +
                    '<div class="small text-danger"><strong>Error:</strong> <span class="font-monospace">' +
                    escapeHtml(run.error_message) +
                    '</span></div>' +
                    '</td>' +
                    '</tr>'
                );
            }
        });
        tbody.innerHTML = rows.join('');
    }

    // -------------------------------------------------------------------------
    // State helpers
    // -------------------------------------------------------------------------

    function showState(state) {
        document.getElementById('loading-state').style.display = state === 'loading' ? '' : 'none';
        document.getElementById('empty-state').style.display = state === 'empty' ? '' : 'none';
        document.getElementById('sources-container').style.display = state === 'table' ? '' : 'none';
    }

    function showRunsState(state) {
        document.getElementById('runs-loading').style.display = state === 'loading' ? '' : 'none';
        document.getElementById('runs-empty').style.display = state === 'empty' ? '' : 'none';
        document.getElementById('runs-table-container').style.display = state === 'table' ? '' : 'none';
    }

})();
