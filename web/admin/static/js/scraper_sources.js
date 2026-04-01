// Scraper Sources Admin Page (srv-5127b, srv-pfeud, srv-opp9d)
// Manages scraper source listing, enable/disable, trigger, diagnostics fold-down, and auto-scrape toggle.

(function () {
    'use strict';

    document.addEventListener('DOMContentLoaded', init);

    var _sseConn = null;
    var _runAllActiveCount = 0;
    var _expandedSource = null; // track which source's diagnostics panel is open

    function init() {
        setupEventHandlers();
        loadSources();
        loadConfig();
        connectSSE();
        setRunAllButtonState(0, false);
        window.addEventListener('pagehide', handlePageHide);
    }

    function handlePageHide() {
        if (_sseConn) { _sseConn.close(); }
    }

    function connectSSE() {
        if (!window.EventSource) {
            return;
        }
        _sseConn = new EventSource('/api/v1/admin/scraper/events');
        var fallbackTimer = null;
        _sseConn.onmessage = function (e) {
            try {
                var ev = JSON.parse(e.data);
                if (ev.kind === 'job_completed' || ev.kind === 'job_failed' || ev.kind === 'job_cancelled') {
                    if (ev.source_name) {
                        updateSourceRow(ev.source_name);
                        // Close diagnostics panel for this source so it can be reopened with fresh data
                        if (_expandedSource === ev.source_name) {
                            closeDiagnostics(ev.source_name);
                        }
                    } else {
                        if (fallbackTimer) { clearTimeout(fallbackTimer); }
                        fallbackTimer = setTimeout(loadSources, 500);
                    }
                }
            } catch (_) {}
        };
        _sseConn.onerror = function () {
            console.debug('scraper SSE: connection error, browser will retry');
        };
    }

    async function updateSourceRow(name) {
        try {
            var data = await API.scraper.listSources();
            var items = data.items || [];
            refreshRunAllStateFromSources(items);
            var src = null;
            for (var i = 0; i < items.length; i++) {
                if (items[i].name === name) { src = items[i]; break; }
            }
            if (!src) return;

            var tbody = document.getElementById('sources-table');
            var existing = tbody ? tbody.querySelector('tr[data-source-name="' + CSS.escape(name) + '"]') : null;
            if (!existing) {
                loadSources();
                return;
            }
            var newHtml = renderSourceRow(src);
            var tmp = document.createElement('tbody');
            tmp.innerHTML = newHtml;
            var newRow = tmp.firstElementChild;
            if (!newRow) return;

            var UPDATE_CELLS = [3, 4, 5, 6, 7];
            UPDATE_CELLS.forEach(function (idx) {
                if (existing.cells[idx] && newRow.cells[idx]) {
                    existing.cells[idx].innerHTML = newRow.cells[idx].innerHTML;
                }
            });
        } catch (err) {
            console.debug('scraper SSE row update failed, falling back:', err.message);
            loadSources();
        }
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

            if (action === 'toggle-diagnostics') {
                toggleDiagnostics(name);
            } else if (action === 'trigger-scrape') {
                triggerScrape(target, name);
            } else if (action === 'trigger-all-scrape') {
                triggerAllScrape(target);
            } else if (action === 'toggle-enabled') {
                toggleEnabled(target, name);
            } else if (action === 'toggle-run-detail') {
                var detailRow = document.getElementById(target.dataset.target);
                if (detailRow) {
                    var isHidden = detailRow.style.display === 'none';
                    detailRow.style.display = isHidden ? '' : 'none';
                    var arrow = target.querySelector('[data-arrow]');
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
        var alreadyShowing = document.getElementById('sources-container').style.display !== 'none';
        if (!alreadyShowing) { showState('loading'); }
        try {
            var data = await API.scraper.listSources();
            var items = data.items || [];
            if (items.length === 0) {
                showState('empty');
                return;
            }
            renderSources(items);
            refreshRunAllStateFromSources(items);
            if (!alreadyShowing) { showState('table'); }
            document.getElementById('showing-text').textContent =
                'Showing ' + items.length + ' source' + (items.length === 1 ? '' : 's');
        } catch (err) {
            if (!alreadyShowing) { showState('empty'); }
            showToast('Failed to load scraper sources: ' + err.message, 'error');
        }
    }

    function renderSources(items) {
        var tbody = document.getElementById('sources-table');
        var existing = tbody.querySelectorAll('tr[data-source-name]');
        if (existing.length > 0) {
            items.forEach(function (src) {
                var row = tbody.querySelector('tr[data-source-name="' + CSS.escape(src.name) + '"]');
                if (row) {
                    var tmp = document.createElement('tbody');
                    tmp.innerHTML = renderSourceRow(src);
                    var newRow = tmp.firstElementChild;
                    if (!newRow) return;
                    [3, 4, 5, 6, 7].forEach(function (idx) {
                        if (row.cells[idx] && newRow.cells[idx]) {
                            row.cells[idx].innerHTML = newRow.cells[idx].innerHTML;
                        }
                    });
                } else {
                    var tmp2 = document.createElement('tbody');
                    tmp2.innerHTML = renderSourceRow(src);
                    if (tmp2.firstElementChild) { tbody.appendChild(tmp2.firstElementChild); }
                }
            });
            existing.forEach(function (row) {
                var stillExists = items.some(function (s) { return s.name === row.dataset.sourceName; });
                if (!stillExists) { row.parentNode.removeChild(row); }
            });
        } else {
            tbody.innerHTML = items.map(renderSourceRow).join('');
        }
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

        return '<tr data-source-name="' + escapeHtml(src.name) + '">' +
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
                    '<button class="btn btn-sm btn-outline-secondary" data-action="toggle-diagnostics" data-name="' + escapeHtml(src.name) + '">Diagnostics</button>' +
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
            await updateSourceRow(name);
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
            setTimeout(function () { updateSourceRow(name); }, 1000);
        } catch (err) {
            showToast('Failed to trigger scrape: ' + err.message, 'error');
        } finally {
            setLoading(btn, false);
        }
    }

    // -------------------------------------------------------------------------
    // Trigger all scrape (srv-x7oba)
    // -------------------------------------------------------------------------

    async function triggerAllScrape(btn) {
        setRunAllButtonState(_runAllActiveCount, true);
        try {
            var respectAutoScrape = document.getElementById('respect-auto-scrape')?.checked ?? true;
            var skipUpToDate = document.getElementById('skip-up-to-date')?.checked ?? true;
            var result = await API.scraper.triggerAll({
                respect_auto_scrape: respectAutoScrape,
                skip_up_to_date: skipUpToDate
            });
            if (result.status === 'skipped') {
                showToast('Auto-scrape is disabled — serial run skipped', 'info');
                setRunAllButtonState(0, false);
            } else {
                showToast('Serial scrape triggered: ' + result.status, 'success');
                var active = typeof result.running_sources === 'number' ? result.running_sources : 1;
                if (active < 1) active = 1;
                setRunAllButtonState(active, false);
                setTimeout(loadSources, 1200);
            }
        } catch (err) {
            if (err && err.status === 409) {
                var running = parseRunningSourcesFromError(err);
                if (running > 0) {
                    showToast('Run already in progress (' + running + ' source' + (running === 1 ? '' : 's') + ' running)', 'warning');
                    setRunAllButtonState(running, false);
                } else {
                    showToast('Run already in progress — wait for current run to finish', 'warning');
                    setRunAllButtonState(1, false);
                }
            } else {
                showToast('Failed to trigger serial scrape: ' + err.message, 'error');
                setRunAllButtonState(_runAllActiveCount, false);
            }
        }
    }

    function parseRunningSourcesFromError(err) {
        if (!err) return 0;
        if (err.body && typeof err.body.running_sources === 'number') {
            return err.body.running_sources;
        }
        var m = String(err.message || '').match(/\((\d+) source/);
        if (!m) return 0;
        var n = parseInt(m[1], 10);
        return Number.isFinite(n) ? n : 0;
    }

    function refreshRunAllStateFromSources(items) {
        var running = 0;
        for (var i = 0; i < items.length; i++) {
            if (items[i].last_run_status === 'running') {
                running++;
            }
        }
        setRunAllButtonState(running, false);
    }

    function setRunAllButtonState(runningCount, loading) {
        var btn = document.getElementById('trigger-all-btn');
        if (!btn) return;
        if (!btn.dataset.baseText) {
            btn.dataset.baseText = btn.textContent.trim() || 'Run All';
        }
        _runAllActiveCount = runningCount > 0 ? runningCount : 0;

        if (loading) {
            btn.disabled = true;
            btn.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>Starting...';
            return;
        }

        if (_runAllActiveCount > 0) {
            btn.disabled = true;
            btn.textContent = btn.dataset.baseText + ' (' + _runAllActiveCount + ' running)';
        } else {
            btn.disabled = false;
            btn.textContent = btn.dataset.baseText;
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
            var triggerAllBtn = document.getElementById('trigger-all-btn');
            var orchestratorOptions = document.getElementById('orchestrator-options');
            if (toggle && wrap) {
                toggle.checked = cfg.auto_scrape === true;
                wrap.classList.remove('d-none');
            }
            if (triggerAllBtn) {
                triggerAllBtn.classList.remove('d-none');
            }
            if (orchestratorOptions) {
                orchestratorOptions.classList.remove('d-none');
            }
        } catch (err) {
            console.warn('scraper config unavailable:', err.message);
        }
    }

    async function toggleAutoScrape(enabled) {
        try {
            await API.scraper.patchConfig({ auto_scrape: enabled });
            showToast('Auto-scrape ' + (enabled ? 'enabled' : 'disabled'), 'success');
        } catch (err) {
            showToast('Failed to update auto-scrape setting: ' + err.message, 'error');
            var toggle = document.getElementById('auto-scrape-toggle');
            if (toggle) toggle.checked = !enabled;
        }
    }

    // -------------------------------------------------------------------------
    // Diagnostics fold-down panel (srv-opp9d)
    // -------------------------------------------------------------------------

    async function toggleDiagnostics(name) {
        if (_expandedSource === name) {
            closeDiagnostics(name);
            return;
        }
        // Close any currently open panel first
        if (_expandedSource) {
            closeDiagnostics(_expandedSource);
        }

        var tbody = document.getElementById('sources-table');
        var sourceRow = tbody ? tbody.querySelector('tr[data-source-name="' + CSS.escape(name) + '"]') : null;
        if (!sourceRow) return;

        var colspan = sourceRow.cells.length;

        // Insert loading row
        var detailId = 'diagnostics-' + name;
        var loadingRow = document.createElement('tr');
        loadingRow.id = detailId;
        loadingRow.setAttribute('data-diagnostics-for', name);
        loadingRow.innerHTML = '<td colspan="' + colspan + '" class="p-0">' +
            '<div class="p-3 text-center text-muted">' +
            '<span class="spinner-border spinner-border-sm me-2"></span>Loading diagnostics...' +
            '</div></td>';
        sourceRow.parentNode.insertBefore(loadingRow, sourceRow.nextSibling);
        _expandedSource = name;

        // Update button state
        var diagBtn = sourceRow.querySelector('[data-action="toggle-diagnostics"]');
        if (diagBtn) diagBtn.textContent = 'Close';

        try {
            var data = await API.scraper.getDiagnostics(name);
            renderDiagnostics(detailId, data);
        } catch (err) {
            var errorRow = document.getElementById(detailId);
            if (errorRow) {
                var tbody = document.getElementById('sources-table');
                var sourceRow = tbody ? tbody.querySelector('tr[data-source-name="' + CSS.escape(name) + '"]') : null;
                var colspan = sourceRow ? sourceRow.cells.length : 8;
                errorRow.innerHTML = '<td colspan="' + colspan + '" class="p-0">' +
                    '<div class="p-3 text-center text-danger">Failed to load diagnostics: ' + escapeHtml(err.message) + '</div></td>';
            }
        }
    }

    function closeDiagnostics(name) {
        var detailId = 'diagnostics-' + name;
        var detailRow = document.getElementById(detailId);
        if (detailRow) detailRow.remove();

        var tbody = document.getElementById('sources-table');
        var sourceRow = tbody ? tbody.querySelector('tr[data-source-name="' + CSS.escape(name) + '"]') : null;
        if (sourceRow) {
            var diagBtn = sourceRow.querySelector('[data-action="toggle-diagnostics"]');
            if (diagBtn) diagBtn.textContent = 'Diagnostics';
        }
        _expandedSource = null;
    }

    function renderDiagnostics(detailId, data) {
        var container = document.getElementById(detailId);
        if (!container) return;

        var sections = [];

        // Section 1: Last Run
        if (data.latest_run) {
            sections.push(renderDiagnosticsSection('Last Run', renderRunDetails(data.latest_run), true));
        }

        // Section 2: Last Successful Run
        if (data.last_successful_run) {
            sections.push(renderDiagnosticsSection('Last Successful Run', renderRunDetails(data.last_successful_run), false));
        }

        // Section 3: Run History
        if (data.recent_runs && data.recent_runs.length > 0) {
            sections.push(renderDiagnosticsSection('Run History (' + data.recent_runs.length + ')', renderRunHistoryTable(data.recent_runs), false));
        }

        if (sections.length === 0) {
            var tbody = document.getElementById('sources-table');
            var sourceRow = tbody ? tbody.querySelector('tr[data-source-name="' + CSS.escape(data.source_name) + '"]') : null;
            var colspan = sourceRow ? sourceRow.cells.length : 8;
            container.innerHTML = '<td colspan="' + colspan + '" class="p-0">' +
                '<div class="p-3 text-center text-muted">No run history yet.</div></td>';
            return;
        }

        var tbody2 = document.getElementById('sources-table');
        var sourceRow2 = tbody2 ? tbody2.querySelector('tr[data-source-name="' + CSS.escape(data.source_name) + '"]') : null;
        var colspan2 = sourceRow2 ? sourceRow2.cells.length : 8;
        container.innerHTML = '<td colspan="' + colspan2 + '" class="p-0">' + sections.join('') + '</td>';
    }

    function renderDiagnosticsSection(title, content, defaultOpen) {
        var sectionId = 'diag-section-' + Math.random().toString(36).substr(2, 9);
        return '<div class="border-bottom">' +
            '<div class="d-flex align-items-center px-3 py-2 bg-muted-lt cursor-pointer" ' +
                'data-action="toggle-run-detail" data-target="' + sectionId + '">' +
            '<span class="me-2" data-arrow>' + (defaultOpen ? '▾' : '▸') + '</span>' +
            '<strong class="small">' + escapeHtml(title) + '</strong>' +
            '</div>' +
            '<div id="' + sectionId + '" style="display:' + (defaultOpen ? '' : 'none') + ';">' +
            '<div class="px-3 py-2">' + content + '</div>' +
            '</div></div>';
    }

    function renderRunDetails(run) {
        var statusCls = run.status === 'completed' ? 'text-success'
            : run.status === 'failed' ? 'text-danger'
            : run.status === 'running' ? 'text-warning'
            : 'text-muted';

        var html = '<div class="row small">' +
            '<div class="col-auto"><strong>Status:</strong></div>' +
            '<div class="col ' + statusCls + '">' + escapeHtml(run.status) + '</div>' +
            '<div class="col-auto"><strong>Started:</strong></div>' +
            '<div class="col-auto">' + (run.started_at ? formatDate(run.started_at) : '—') + '</div>' +
            '<div class="col-auto"><strong>Completed:</strong></div>' +
            '<div class="col-auto">' + (run.completed_at ? formatDate(run.completed_at) : '—') + '</div>' +
            '</div>' +
            '<div class="row small mt-1">' +
            '<div class="col-auto"><strong>Events:</strong></div>' +
            '<div class="col">' +
                escapeHtml(String(run.events_found)) + ' found / ' +
                escapeHtml(String(run.events_new)) + ' new / ' +
                escapeHtml(String(run.events_dup)) + ' dup / ' +
                (run.events_failed > 0
                    ? '<span class="badge bg-danger-lt">' + escapeHtml(String(run.events_failed)) + ' failed</span>'
                    : escapeHtml(String(run.events_failed)) + ' failed') +
            '</div></div>';

        if (run.error_message) {
            html += '<div class="row small mt-1">' +
                '<div class="col-auto"><strong>Error:</strong></div>' +
                '<div class="col"><span class="text-danger font-monospace">' + escapeHtml(run.error_message) + '</span></div></div>';
        }

        return html;
    }

    function renderRunHistoryTable(runs) {
        var rows = [];
        runs.forEach(function (run) {
            var cls = run.status === 'completed' ? 'bg-success-lt'
                : run.status === 'failed' ? 'bg-danger-lt'
                : run.status === 'running' ? 'bg-warning-lt'
                : 'bg-secondary-lt';
            var hasError = run.error_message && run.error_message.length > 0;
            var detailId = 'run-detail-' + Math.random().toString(36).substr(2, 9);

            rows.push('<tr' + (hasError ? ' class="cursor-pointer" data-action="toggle-run-detail" data-target="' + detailId + '"' : '') + '>' +
                '<td class="text-muted small">' + escapeHtml(run.started_at ? formatDate(run.started_at) : '—') + '</td>' +
                '<td class="text-muted small">' + escapeHtml(run.completed_at ? formatDate(run.completed_at) : '—') + '</td>' +
                '<td><span class="badge ' + cls + '">' + escapeHtml(run.status) + '</span>' +
                    (hasError ? ' <span class="text-muted small" data-arrow>▸</span>' : '') + '</td>' +
                '<td>' + escapeHtml(String(run.events_found)) + '</td>' +
                '<td>' + escapeHtml(String(run.events_new)) + '</td>' +
                '<td>' + escapeHtml(String(run.events_dup)) + '</td>' +
                '<td>' + (run.events_failed > 0
                    ? '<span class="badge bg-danger-lt">' + escapeHtml(String(run.events_failed)) + '</span>'
                    : escapeHtml(String(run.events_failed))) + '</td>' +
                '</tr>');

            if (hasError) {
                rows.push('<tr id="' + detailId + '" style="display:none;">' +
                    '<td colspan="7" class="bg-danger-lt">' +
                    '<div class="small text-danger"><strong>Error:</strong> <span class="font-monospace">' +
                    escapeHtml(run.error_message) + '</span></div></td></tr>');
            }
        });

        return '<table class="table table-sm table-vcenter mb-0">' +
            '<thead><tr>' +
            '<th class="small">Started</th><th class="small">Completed</th><th class="small">Status</th>' +
            '<th class="small">Found</th><th class="small">New</th><th class="small">Dup</th><th class="small">Failed</th>' +
            '</tr></thead><tbody>' + rows.join('') + '</tbody></table>';
    }

    // -------------------------------------------------------------------------
    // State helpers
    // -------------------------------------------------------------------------

    function showState(state) {
        document.getElementById('loading-state').style.display = state === 'loading' ? '' : 'none';
        document.getElementById('empty-state').style.display = state === 'empty' ? '' : 'none';
        document.getElementById('sources-container').style.display = state === 'table' ? '' : 'none';
    }

})();
