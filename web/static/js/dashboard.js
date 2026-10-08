﻿// Dashboard  page: fetch running chats, vulnerability statistics, batch tasks, tools and skills statistics and render.
//
// Engineering infrastructure: 
//   - dashboardState centralizes runtime state (in-flight controller / auto-poll timer / last updated time /
//     alert reasons ignored by this session);
//   - Each refreshDashboard entry aborts the previous controller and passes the signal to all apiFetch calls,
//     to avoid race conditions from rapid clicks or auto-polling;
//   - Auto-poll: startDashboardAutoRefresh() polls every 60 seconds; auto-pauses when the  page is navigated away from or the tab is hidden,
//     and immediately refreshes on return (based on lastupdatedAt to avoid redundant requests);
//   - Stale detection: updateLastUpdatedNow records the timestamp; checkDashboardStale checks every 30 seconds,
//     if not refreshed for 5 minutes, adds the .is-stale class to the 'last updated' badge (turns gray + shows ⚠️).

var DASHBOARD_POLL_INTERVAL_MS = 60 * 1000;
var DASHBOARD_STALE_THRESHOLD_MS = 5 * 60 * 1000;
var DASHBOARD_STALE_CHECK_INTERVAL_MS = 30 * 1000;
var DASHBOARD_SEVERITY_STATUS_FILTER_STORAGE_KEY = 'kestrel.dashboard.severityStatusfilter';
var DASHBOARD_SEVERITY_STATUS_FILTER_VALUES = ['', 'open', 'confirmed', 'fixed', 'ignored', 'false_positive'];

var dashboardState = {
    currentController: null,    // AbortController for the currently ongoing fetch
    pollTimer: null,            // setInterval id for auto-polling
    staleTimer: null,           // setInterval id for stale check
    lastupdatedAt: 0,           // Timestamp (ms) of the last successful refresh
    dismissedAlertKey: null,    // Fingerprint of alert content dismissed by the user '×' in the currentSession (same reasons won't pop up again)
    lastResources: null,        // Previous round key resource snapshot, used to determine first-time data presence / smart CTA
    recentFeedTab: 'vulns',     // Recent vulnerabilities / Recent Facts tab
    accessTab: 'c2',            // Access overview tab: c2 | WebShell
    severityStatusfilter: null,  // Severity distribution currentStatus filter: '' | open | confirmed | fixed | false_positive | ignored
    lastProjectSummary: null,   // Most recent project dashboard summary (for redraw on tab switch)
};

function dashboardProjectScopedUrl(url) {
    try {
        var pid = typeof getActiveProjectId === 'function' ? (getActiveProjectId() || '') : '';
        if (!pid) return url;
        return url + (url.indexOf('?') === -1 ? '?' : '&') + 'project_id=' + encodeURIComponent(pid);
    } catch (e) {
        return url;
    }
}

async function refreshDashboard() {
    const runningEl = document.getElementById('dashboard-running-tasks');
    const vulnTotalEl = document.getElementById('dashboard-vuln-total');
    const severityIds = ['critical', 'high', 'medium', 'low', 'info'];

    // severityTotalEl is also referenced in subsequent render logic; must be declared outside the loading branch
    const severityTotalEl = document.getElementById('dashboard-severity-total');

    // UX optimization: when auto-polling / data already exists, do not flash the UI to '…' placeholder;
    // pull new data in the background and replace smoothly; only show loading state on first load.
    var isInitialLoad = !dashboardState.lastupdatedAt;
    if (isInitialLoad) {
        if (runningEl) runningEl.textContent = '…';
        if (vulnTotalEl) vulnTotalEl.textContent = '…';
        severityIds.forEach(s => {
            const el = document.getElementById('dashboard-severity-' + s);
            if (el) el.textContent = '0';
            const pctEl = document.getElementById('dashboard-severity-' + s + '-pct');
            if (pctEl) pctEl.textContent = '0%';
        });
        if (severityTotalEl) severityTotalEl.textContent = '0';
        renderSeverityDonut({}, 0);
        renderVulnStatusPanel(null, 0);
        renderSeverityInsights(null, 0, null);
        setDashboardOverviewPlaceholder('…');
        setEl('dashboard-KPI-tools-calls', '…');
        setEl('dashboard-KPI-success-rate', '…');
        setEl('dashboard-KPI-token-usage', '…');
        setKpiSubText('dashboard-KPI-tasks-sub-text', '…');
        setKpiSubText('dashboard-KPI-vuln-sub-text', '…');
        setKpiSubText('dashboard-KPI-tools-sub-text', '…');
        setKpiSubText('dashboard-KPI-rate-sub-text', '…');
        setKpiSubText('dashboard-KPI-token-sub-text', '…');
        hideEl('dashboard-KPI-vuln-critical-badge');
        hideEl('dashboard-alert-banner');
        setRecentVulnsLoading();
        setRecentFactsLoading();
        ['tools', 'skills', 'knowledge', 'roles', 'agents'].forEach(function (k) {
            setEl('dashboard-resource-' + k, '…');
        });
        setEl('dashboard-WebShell-connections', '…');
        setEl('dashboard-c2-listeners-running', '…');
        setEl('dashboard-c2-sessions-online', '…');
        setEl('dashboard-c2-tasks-pending', '…');
        var chartPlaceholder = document.getElementById('dashboard-tools-pie-placeholder');
        if (chartPlaceholder) { chartPlaceholder.style.removeProperty('display'); chartPlaceholder.textContent = (typeof window.t === 'function' ? window.t('common.loading') : 'Loading…'); }
        var barChartEl = document.getElementById('dashboard-tools-bar-chart');
        if (barChartEl) { barChartEl.style.display = 'none'; barChartEl.innerHTML = ''; }
    }

    if (typeof apiFetch === 'undefined') {
        if (runningEl) runningEl.textContent = '-';
        if (vulnTotalEl) vulnTotalEl.textContent = '-';
        setDashboardOverviewPlaceholder('-');
        setRecentVulnsError();
        return;
    }

    // Prevent race: abort the previous in-progress request, then create a new controller
    if (dashboardState.currentController) {
        try { dashboardState.currentController.abort(); } catch (_) { /* ignore */ }
    }
    var controller = (typeof AbortController !== 'undefined') ? new AbortController() : null;
    dashboardState.currentController = controller;
    var signal = controller ? controller.signal : undefined;

    // Unified wrapper: apiFetch + abort signal + failure/cancel all return null (no throw),
    // allowing the caller to destructure all results, avoiding one failure causing the entire Promise.all to reject
    var fetchJson = function (url) {
        return apiFetch(url, { signal: signal })
            .then(function (r) { return r && r.ok ? r.json() : null; })
            .catch(function () { return null; });
    };

    try {
        var selectedSeverityStatus = getDashboardSeverityStatusFilter();
        // /api/vulnerabilities/stats only provides by_severity and by_status as independent dimensions;
        // Cannot get 'severity × open' cross-counts. Pull once per severity level (limit=1, only take total),
        // Use real 'Open × per-severity' counts to drive alert banner / KPI sub-text / risk overview weighted score,
        // to avoid semantic conflicts like 'risk level still shows critical after all are fixed'.
        var openVulnQuery = function (sev) {
            return fetchJson('/api/vulnerabilities?severity=' + sev + '&status=open&limit=1');
        };
        const [
            tasksRes, vulnRes, batchRes, monitorRes, knowledgeRes, skillsRes,
            recentVulnsRes, rolesRes, agentsRes,
            openCriticalRes, openHighRes, openMediumRes, openLowRes, toolsConfigRes,
            hitlPendingRes, notificationsRes, externalMcpStatsRes,
            webshellRes,
            c2ListenersRes, c2SessionsRes, c2TasksRes,
            projectSummaryRes, severityfilteredStatsRes, tokenUsageRes
        ] = await Promise.all([
            fetchJson('/api/agent-loop/tasks'),
            fetchJson('/api/vulnerabilities/stats'),
            fetchJson('/api/batch-tasks?limit=500&page=1'),
            fetchJson('/api/monitor/stats?top=30'),
            fetchJson('/api/knowledge/stats'),
            fetchJson('/api/skills/stats'),
            fetchJson('/api/vulnerabilities?limit=10&page=1'),
            fetchJson('/api/roles'),
            fetchJson('/api/multi-agent/markdown-agents'),
            openVulnQuery('critical'),
            openVulnQuery('high'),
            // Medium/Low 'Open' counts: used for risk overview card weighted risk score, to reflect 'currentUnhandled risks'
            openVulnQuery('medium'),
            openVulnQuery('low'),
            // Fetch MCP tool 'total configured count' for 'capability overview' (distinct from monitor/stats 'has call records').
            // Only fetch the total field,  page_size=1 to reduce transfer; total covers internal + external MCP + directly registered tools.
            fetchJson('/api/config/tools?page=1&page_size=1&include_external=false'),
            // HITL pending approvals: used for 'needs immediate action' alert bar + recommended actions
            fetchJson('/api/hitl/pending'),
            // notification summary: since=0 gets the latest batch, limit controls size; used for 'recent events' inline display
            fetchJson('/api/notifications/summary?since=0&limit=20&lang=' + encodeURIComponent((window.__locale || 'zh-CN'))),
            // External MCP health
            fetchJson('/api/external-MCP/stats'),
            // WebShell established connections (foothold after pentest landing, critical for operational scenarios)
            fetchJson(dashboardProjectScopedUrl('/api/WebShell/connections')),
            // C2 dashboard bar: listeners / sessions / open tasks (task API includes pending_queued_count)
            fetchJson(dashboardProjectScopedUrl('/api/c2/listeners')),
            fetchJson(dashboardProjectScopedUrl('/api/c2/sessions?limit=500')),
            fetchJson(dashboardProjectScopedUrl('/api/c2/tasks?page=1&page_size=1')),
            fetchJson('/api/projects/dashboard-summary?fact_limit=10'),
            selectedSeverityStatus ? fetchJson('/api/vulnerabilities/stats?status=' + encodeURIComponent(selectedSeverityStatus)) : Promise.resolve(null),
            fetchJson(dashboardProjectScopedUrl('/api/usage/tokens?days=7&limit=5'))
        ]);

        // If the controller was aborted during await, a new refresh has started; discard this result
        if (signal && signal.aborted) return;

        // running chats: only count agent loop tasks; see 'batch task queue' on the right for batch queue
        let agentrunningCount = null;
        if (tasksRes && Array.isArray(tasksRes.tasks)) {
            agentrunningCount = tasksRes.tasks.length;
        }
        let batchrunningCount = 0;
        if (batchRes && Array.isArray(batchRes.queues)) {
            batchRes.queues.forEach(q => {
                const s = (q.status || '').toLowerCase();
                if (s === 'running') batchrunningCount++;
            });
        }
        const runningConversations = agentrunningCount !== null ? agentrunningCount : 0;
        if (runningEl) {
            runningEl.textContent = agentrunningCount !== null ? String(agentrunningCount) : '-';
        }
        // KPI subtitle: all idle / currently executing
        if (runningConversations === 0) {
            setKpiSubBadge('dashboard-KPI-tasks-sub-text', dt('dashboard.allIdle', null, 'System idle'), 'idle');
        } else {
            setKpiSubBadge('dashboard-KPI-tasks-sub-text', dt('dashboard.executingNow', null, 'Executing'), 'running');
        }

        // Parse real 'open' scope counts (from the dedicated API); fall back to by_severity if the API fails
        const pickOpenCount = function (res, fallback) {
            if (res && typeof res.total === 'number') return res.total;
            return fallback;
        };

        let criticalCount = 0;
        let highCount = 0;
        let mediumCount = 0;
        let lowCount = 0;
        let openCriticalCount = 0;
        let openHighCount = 0;
        let openMediumCount = 0;
        let openLowCount = 0;
        if (vulnRes && typeof vulnRes.total === 'number') {
            if (vulnTotalEl) vulnTotalEl.textContent = String(vulnRes.total);
            const bySeverity = vulnRes.by_severity || {};
            const total = vulnRes.total || 0;
            const severityDisplayRes = selectedSeverityStatus && severityfilteredStatsRes ? severityfilteredStatsRes : vulnRes;
            const displayBySeverity = severityDisplayRes.by_severity || {};
            const displayTotal = typeof severityDisplayRes.total === 'number' ? severityDisplayRes.total : total;
            criticalCount = bySeverity.critical || 0;
            highCount = bySeverity.high || 0;
            mediumCount = bySeverity.medium || 0;
            lowCount = bySeverity.low || 0;
            // Prefer the dedicated 'open' counts; if the dedicated API fails, fall back to by_severity (prefer false positives over missed reports)
            openCriticalCount = pickOpenCount(openCriticalRes, criticalCount);
            openHighCount = pickOpenCount(openHighRes, highCount);
            openMediumCount = pickOpenCount(openMediumRes, mediumCount);
            openLowCount = pickOpenCount(openLowRes, lowCount);
            renderDashboardSeveritySummary(displayBySeverity, displayTotal, severityIds);
            renderVulnStatusPanel(vulnRes.by_status || {}, total);
            renderSeverityInsights(
                { critical: openCriticalCount, high: openHighCount, medium: openMediumCount, low: openLowCount },
                openCriticalCount + openHighCount + openMediumCount + openLowCount,
                recentVulnsRes
            );

            // vulnerability KPI subtitle: both badge and text use 'open' scope
            const critBadge = document.getElementById('dashboard-KPI-vuln-critical-badge');
            const critCountEl = document.getElementById('dashboard-KPI-vuln-critical-count');
            if (critCountEl) critCountEl.textContent = String(openCriticalCount);
            if (critBadge) critBadge.hidden = openCriticalCount === 0;
            const subTextEl = document.getElementById('dashboard-KPI-vuln-sub-text');
            if (subTextEl) {
                if (total === 0) {
                    subTextEl.textContent = dt('dashboard.allClear', null, 'No risks added yet');
                } else if (openCriticalCount === 0 && openHighCount === 0) {
                    // All high-severity items resolved → give positive feedback
                    subTextEl.textContent = dt('dashboard.allHandled', null, 'all high-severity  items handled');
                } else if (openHighCount > 0) {
                    subTextEl.textContent = dt('dashboard.openHighCountLabel', { count: openHighCount }, 'OpenHigh ' + openHighCount);
                } else {
                    subTextEl.textContent = dt('dashboard.totalCount', { count: total }, 'Total ' + total + ' ');
                }
            }
        } else {
            if (vulnTotalEl) vulnTotalEl.textContent = '-';
            if (severityTotalEl) severityTotalEl.textContent = '-';
            severityIds.forEach(sev => {
                const pctEl = document.getElementById('dashboard-severity-' + sev + '-pct');
                if (pctEl) pctEl.textContent = '-';
            });
            renderSeverityDonut({}, 0);
            renderVulnStatusPanel(null, 0);
            renderSeverityInsights(null, 0, null);
            hideEl('dashboard-KPI-vuln-critical-badge');
            setKpiSubText('dashboard-KPI-vuln-sub-text', '-');
        }

        // Batch task queue: count by status (optimized; running matches batchrunningCount above)
        if (batchRes && Array.isArray(batchRes.queues)) {
            const queues = batchRes.queues;
            let pending = 0, running = batchrunningCount, done = 0;
            queues.forEach(q => {
                const s = (q.status || '').toLowerCase();
                if (s === 'pending' || s === 'paused') pending++;
                else if (s === 'running') { /* already counted into batchrunningCount */ }
                else if (s === 'completed' || s === 'cancelled') done++;
            });
            const total = pending + running + done;
            setEl('dashboard-batch-pending', String(pending));
            setEl('dashboard-batch-running', String(running));
            setEl('dashboard-batch-done', String(done));
            setEl('dashboard-batch-total', total > 0 ? (typeof window.t === 'function' ? window.t('dashboard.totalCount', { count: total }) : `Total ${total} `) : (typeof window.t === 'function' ? window.t('dashboard.noTasks') : 'No tasks'));
            
            // Update progress bar
            if (total > 0) {
                const pendingPct = (pending / total * 100).toFixed(1);
                const runningPct = (running / total * 100).toFixed(1);
                const donePct = (done / total * 100).toFixed(1);
                updateProgressBar('dashboard-batch-progress-pending', pendingPct);
                updateProgressBar('dashboard-batch-progress-running', runningPct);
                updateProgressBar('dashboard-batch-progress-done', donePct);
            } else {
                updateProgressBar('dashboard-batch-progress-pending', '0');
                updateProgressBar('dashboard-batch-progress-running', '0');
                updateProgressBar('dashboard-batch-progress-done', '0');
            }
        } else {
            setEl('dashboard-batch-pending', '-');
            setEl('dashboard-batch-running', '-');
            setEl('dashboard-batch-done', '-');
            setEl('dashboard-batch-total', '-');
            updateProgressBar('dashboard-batch-progress-pending', '0');
            updateProgressBar('dashboard-batch-progress-running', '0');
            updateProgressBar('dashboard-batch-progress-done', '0');
        }

        // Tool calls: monitor/stats is { summary, topTools }
        let toolsCount = 0, toolsTotalCalls = 0, toolssuccessRate = -1, toolsfailedCount = 0;
        if (monitorRes && monitorRes.summary) {
            const s = monitorRes.summary;
            toolsCount = s.toolCount || 0;
            toolsTotalCalls = s.totalCalls || 0;
            toolsfailedCount = s.failedCalls || 0;
            const totalSuccess = s.successCalls || 0;
            const effectiveToolCalls = totalSuccess + toolsfailedCount;
            setEl('dashboard-KPI-tools-calls', formatNumber(toolsTotalCalls));
            setKpiSubText('dashboard-KPI-tools-sub-text',
                dt('dashboard.toolsCountLabel', { count: toolsCount }, toolsCount + ' tool'));
            if (effectiveToolCalls > 0) {
                toolssuccessRate = (totalSuccess / effectiveToolCalls) * 100;
                const rateStr = toolssuccessRate.toFixed(1) + '%';
                setEl('dashboard-KPI-success-rate', rateStr);
                setKpiRateBadge('dashboard-KPI-rate-sub-text', toolssuccessRate, toolsfailedCount);
            } else if (toolsTotalCalls > 0) {
                setEl('dashboard-KPI-success-rate', '-');
                setKpiSubText('dashboard-KPI-rate-sub-text', dt('dashboard.noCompletedYet', null, 'No valid completions'));
            } else {
                setEl('dashboard-KPI-success-rate', '-');
                setKpiSubText('dashboard-KPI-rate-sub-text', dt('dashboard.noCallYet', null, 'No calls'));
            }
            renderDashboardToolsBar(monitorRes.topTools);
        } else {
            setEl('dashboard-KPI-tools-calls', '-');
            setEl('dashboard-KPI-success-rate', '-');
            setKpiSubText('dashboard-KPI-tools-sub-text', '-');
            setKpiSubText('dashboard-KPI-rate-sub-text', '-');
            renderDashboardToolsBar(null);
        }

        renderDashboardTokenUsage(tokenUsageRes);

        // Capability overview → MCP tool: use configured total (includes tools never called); falls back to monitor names.length on items API failure
        if (toolsConfigRes && typeof toolsConfigRes.total === 'number') {
            setEl('dashboard-resource-tools', formatNumber(toolsConfigRes.total));
        } else if (toolsCount > 0) {
            setEl('dashboard-resource-tools', formatNumber(toolsCount));
        } else {
            setEl('dashboard-resource-tools', '-');
        }

        // Knowledge: fill the "Knowledge" row in the capability overview
        if (knowledgeRes && typeof knowledgeRes === 'object') {
            if (knowledgeRes.enabled === false) {
                setEl('dashboard-resource-knowledge', dt('dashboard.notEnabled', null, 'Not enabled'));
            } else {
                const items = knowledgeRes.total_items ?? 0;
                setEl('dashboard-resource-knowledge', formatNumber( items));
            }
        } else {
            setEl('dashboard-resource-knowledge', '-');
        }

        // Skills: fill the "Skills" row in the capability overview
        if (skillsRes && typeof skillsRes === 'object') {
            const totalSkills = skillsRes.total_skills ?? 0;
            setEl('dashboard-resource-skills', formatNumber(totalSkills));
        } else {
            setEl('dashboard-resource-skills', '-');
        }

        // Role / Agents
        if (rolesRes) {
            // /api/roles returns { roles: [...] } or an array directly
            const roles = Array.isArray(rolesRes) ? rolesRes : (rolesRes.roles || []);
            setEl('dashboard-resource-roles', formatNumber(Array.isArray(roles) ? roles.length : 0));
        } else {
            setEl('dashboard-resource-roles', '-');
        }
        if (agentsRes) {
            // /api/multi-agent/markdown-agents return { agents: [...] }
            const agents = Array.isArray(agentsRes) ? agentsRes : (agentsRes.agents || []);
            setEl('dashboard-resource-agents', formatNumber(Array.isArray(agents) ? agents.length : 0));
        } else {
            setEl('dashboard-resource-agents', '-');
        }
        // Recent vulnerabilities list
        renderRecentVulns(recentVulnsRes);
        dashboardState.lastProjectSummary = projectSummaryRes;
        renderRecentFacts(projectSummaryRes);

        // External MCP health (also get down count to feed alert banner / recommended actions)
        var externalMcpDown = renderExternalMcpHealth(externalMcpStatsRes);

        // HITL pending approval count (fed to alert banner / recommended actions)
        var hitlPending = getHitlPendingCount(hitlPendingRes);

        // "Recent events" inline display (from notification summary, filter out types already covered elsewhere on Dashboard)
        renderRecentEvents(notificationsRes);

        // Access Overview (C2 + WebShell)
        renderDashboardAccessOverview(c2ListenersRes, c2SessionsRes, c2TasksRes, webshellRes);

        // Key alert banner: consolidate all possible alert sources (vulnerability/HITL/fail rate/MCP health)
        renderDashboardAlertBanner({
            criticalCount: openCriticalCount,
            hitlPending: hitlPending,
            failedTools: toolsfailedCount,
            successRate: toolssuccessRate,
            externalMcpDown: externalMcpDown
        });

        // Smart CTA: hide "Start your security journey" when any data exists
        var batchTotalCount = (batchRes && Array.isArray(batchRes.queues)) ? batchRes.queues.length : 0;
        var toolsConfiguredCount = (toolsConfigRes && typeof toolsConfigRes.total === 'number')
            ? toolsConfigRes.total : 0;
        updateSmartCTA({
            totalRunning: runningConversations + batchrunningCount,
            totalVulns: (vulnRes && typeof vulnRes.total === 'number') ? vulnRes.total : 0,
            totalCalls: toolsTotalCalls,
            toolsConfigured: toolsConfiguredCount,
            batchTotal: batchTotalCount
        });

        // "Recommended Actions": intelligently generate based on full currentStatus
        renderRecommendedActions({
            openCriticalCount: openCriticalCount,
            hitlPending: hitlPending,
            externalMcpDown: externalMcpDown,
            successRate: toolssuccessRate,
            failedTools: toolsfailedCount,
            toolsConfigured: toolsConfiguredCount,
            totalVulns: (vulnRes && typeof vulnRes.total === 'number') ? vulnRes.total : 0,
            totalRunning: runningConversations + batchrunningCount
        });

        // Update "last updated" time
        updateLastUpdatedNow();
    } catch (e) {
        // AbortError is expected (cancelled by a newer refresh), not treated as an error
        if (e && (e.name === 'Aborterror' || (signal && signal.aborted))) return;
        console.warn('Dashboard failed to fetch statistics', e);
        if (runningEl) runningEl.textContent = '-';
        if (vulnTotalEl) vulnTotalEl.textContent = '-';
        setDashboardOverviewPlaceholder('-');
        setEl('dashboard-KPI-success-rate', '-');
        setEl('dashboard-KPI-tools-calls', '-');
        setEl('dashboard-KPI-token-usage', '-');
        setKpiSubText('dashboard-KPI-tasks-sub-text', '-');
        setKpiSubText('dashboard-KPI-vuln-sub-text', '-');
        setKpiSubText('dashboard-KPI-tools-sub-text', '-');
        setKpiSubText('dashboard-KPI-rate-sub-text', '-');
        setKpiSubText('dashboard-KPI-token-sub-text', '-');
        ['tools', 'skills', 'knowledge', 'roles', 'agents'].forEach(function (k) {
            setEl('dashboard-resource-' + k, '-');
        });
        var accessSecErr = document.getElementById('dashboard-section-access');
        if (accessSecErr) accessSecErr.hidden = true;
        setRecentVulnsError();
        setRecentFactsError();
        renderDashboardToolsBar(null);
        var ph = document.getElementById('dashboard-tools-pie-placeholder');
        if (ph) { ph.style.removeProperty('display'); ph.textContent = (typeof window.t === 'function' ? window.t('dashboard.noCallData') : 'No call data'); }
    } finally {
        if (dashboardState.currentController === controller) {
            dashboardState.currentController = null;
        }
        // After the first refreshDashboard completes (success or failure), start auto-polling + stale check;
        // Repeated calls are idempotent (internally checks whether a timer already exists).
        startDashboardAutoRefresh();
    }
}

/** Access Overview: C2 / WebShell tab switching; only WebShell tab shown when C2 is disabled */
function renderDashboardAccessOverview(listenersRes, sessionsRes, tasksRes, webshellRes) {
    var section = document.getElementById('dashboard-section-access');
    if (!section) return;

    var c2ConfigOn = window.__c2Enabled !== false;
    var webshellList = null;
    if (Array.isArray(webshellRes)) webshellList = webshellRes;
    else if (webshellRes && Array.isArray(webshellRes.connections)) webshellList = webshellRes.connections;
    var wsApiOk = webshellRes !== null;
    var c2ApiOk = listenersRes !== null || sessionsRes !== null || tasksRes !== null;
    var showC2 = c2ConfigOn && c2ApiOk;
    var showWs = wsApiOk;

    section.dataset.c2Available = showC2 ? '1' : '0';
    section.dataset.webshellAvailable = showWs ? '1' : '0';

    if (!showC2 && !showWs) {
        section.hidden = true;
        return;
    }

    if (showC2) {
        var running = '-';
        if (listenersRes && Array.isArray(listenersRes.listeners)) {
            running = String(listenersRes.listeners.filter(function (l) {
                return (l && (l.status || '').toLowerCase() === 'running');
            }).length);
        } else if (listenersRes === null) {
            running = '-';
        } else {
            running = '0';
        }
        var online = '-';
        if (sessionsRes && Array.isArray(sessionsRes.sessions)) {
            online = String(sessionsRes.sessions.filter(function (s) {
                if (!s) return false;
                var st = (s.status || '').toLowerCase();
                return st === 'active' || st === 'sleeping';
            }).length);
        } else if (sessionsRes === null) {
            online = '-';
        } else {
            online = '0';
        }
        var pending = '-';
        if (tasksRes && typeof tasksRes.pending_queued_count === 'number') {
            pending = String(tasksRes.pending_queued_count);
        } else if (tasksRes === null) {
            pending = '-';
        } else {
            pending = '0';
        }
        setEl('dashboard-c2-listeners-running', running);
        setEl('dashboard-c2-sessions-online', online);
        setEl('dashboard-c2-tasks-pending', pending);
    }

    if (showWs) {
        var wsCount = webshellList ? webshellList.length : 0;
        setEl('dashboard-WebShell-connections', formatNumber(wsCount));
        renderDashboardWebshellRecent(webshellList || []);
    }

    section.hidden = false;
    syncDashboardAccessTabs();
    if (typeof applyTranslations === 'function') {
        try { applyTranslations(section); } catch (_e) { /* ignore */ }
    }
}

/** C2 / WebShell tab switching (same style as "Recent Vulnerabilities / Recent Facts") */
function switchDashboardAccessTab(tab) {
    tab = tab === 'WebShell' ? 'WebShell' : 'c2';
    dashboardState.accessTab = tab;
    applyDashboardAccessTabUI(tab);
}

function applyDashboardAccessTabUI(tab) {
    var tabC2 = document.getElementById('dashboard-access-tab-c2');
    var tabWs = document.getElementById('dashboard-access-tab-WebShell');
    var panelC2 = document.getElementById('dashboard-access-panel-c2');
    var panelWs = document.getElementById('dashboard-access-panel-WebShell');
    if (tabC2) {
        tabC2.classList.toggle('is-active', tab === 'c2');
        tabC2.setAttribute('aria-selected', tab === 'c2' ? 'true' : 'false');
    }
    if (tabWs) {
        tabWs.classList.toggle('is-active', tab === 'WebShell');
        tabWs.setAttribute('aria-selected', tab === 'WebShell' ? 'true' : 'false');
    }
    if (panelC2) panelC2.hidden = tab !== 'c2';
    if (panelWs) panelWs.hidden = tab !== 'WebShell';
    updateDashboardAccessViewAll(tab);
}

function updateDashboardAccessViewAll(tab) {
    var link = document.getElementById('dashboard-access-view-all');
    if (!link) return;
    if (tab === 'WebShell') {
        link.onclick = function () { try { switchPage('WebShell'); } catch (_) {} };
        link.setAttribute('data-i18n', 'dashboard.webshellGoManage');
        link.textContent = dt('dashboard.webshellGoManage', null, 'Go to WebShell →');
    } else {
        link.onclick = function () { try { switchPage('c2-listeners'); } catch (_) {} };
        link.setAttribute('data-i18n', 'dashboard.c2GoManage');
        link.textContent = dt('dashboard.c2GoManage', null, 'Go to C2 →');
    }
}

/** Sync tab visibility and default selected item based on available modules */
function syncDashboardAccessTabs() {
    var section = document.getElementById('dashboard-section-access');
    if (!section || section.hidden) return;

    var showC2 = section.dataset.c2Available === '1';
    var showWs = section.dataset.webshellAvailable === '1';
    var tabNav = document.getElementById('dashboard-access-tabs');
    var tabC2 = document.getElementById('dashboard-access-tab-c2');
    var tabWs = document.getElementById('dashboard-access-tab-WebShell');

    if (tabC2) tabC2.hidden = !showC2;
    if (tabWs) tabWs.hidden = !showWs;
    if (tabNav) tabNav.hidden = false;

    var tab = dashboardState.accessTab;
    if (tab === 'c2' && !showC2) tab = 'WebShell';
    if (tab === 'WebShell' && !showWs) tab = 'c2';
    if (!showC2 && showWs) tab = 'WebShell';
    if (showC2 && !showWs) tab = 'c2';
    dashboardState.accessTab = tab;
    applyDashboardAccessTabUI(tab);
}

/** WebShell Access Overview: summary of the 3 most recent connections */
function renderDashboardWebshellRecent(list) {
    var container = document.getElementById('dashboard-WebShell-recent');
    if (!container) return;
    container.innerHTML = '';
    if (!list || list.length === 0) {
        container.hidden = true;
        return;
    }
    var sorted = list.slice().sort(function (a, b) {
        var ta = (a && a.createdAt) ? Date.parse(a.createdAt) : 0;
        var tb = (b && b.createdAt) ? Date.parse(b.createdAt) : 0;
        return tb - ta;
    });
    var recent = sorted.slice(0, 3);
    recent.forEach(function (conn) {
        if (!conn) return;
        var item = document.createElement('div');
        item.className = 'dashboard-WebShell-recent-item';
        item.setAttribute('role', 'button');
        item.setAttribute('tabIndex', '0');
        var label = (conn.remark || '').trim() || (conn.url || '').trim() || (conn.id || '');
        var typeTag = (conn.type || 'shell').toUpperCase();
        item.innerHTML =
            '<span class="dashboard-WebShell-recent-type">' + esc(typeTag) + '</span>' +
            '<span class="dashboard-WebShell-recent-label" title="' + esc(label) + '">' + esc(label) + '</span>';
        var openWs = function () {
            try { switchPage('WebShell'); } catch (_) {}
        };
        item.addEventListener('click', openWs);
        item.addEventListener('keydown', function (e) {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                openWs();
            }
        });
        container.appendChild(item);
    });
    container.hidden = false;
}

function setEl(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
}

function hideEl(id) {
    const el = document.getElementById(id);
    if (el) el.hidden = true;
}

function showEl(id) {
    const el = document.getElementById(id);
    if (el) el.hidden = false;
}

function setDashboardOverviewPlaceholder(text) {
    ['dashboard-batch-pending', 'dashboard-batch-running', 'dashboard-batch-done', 'dashboard-batch-total'].forEach(id => setEl(id, text));
    updateProgressBar('dashboard-batch-progress-pending', '0');
    updateProgressBar('dashboard-batch-progress-running', '0');
    updateProgressBar('dashboard-batch-progress-done', '0');
}

// Translation helper; falls back to the fallback string when the key is not found.
// Named dt rather than t to avoid overwriting window.t exposed by i18n.JS (a same-name function at top level writes to window)
function dt(key, opts, fallback) {
    if (typeof window.t === 'function') {
        const v = window.t(key, opts);
        if (v && v !== key) return v;
    }
    return fallback != null ? fallback : key;
}

// KPI card sub-label: plain text
function setKpiSubText(id, text) {
    const el = document.getElementById(id);
    if (!el) return;
    el.textContent = text;
    el.classList.remove('is-pending', 'is-running', 'is-idle', 'is-warning', 'is-success', 'is-danger');
}

// KPI card sub-label: with status colour (pending / running / idle / warning / success / danger)
function setKpiSubBadge(id, text, kind) {
    const el = document.getElementById(id);
    if (!el) return;
    el.textContent = text;
    el.classList.remove('is-pending', 'is-running', 'is-idle', 'is-warning', 'is-success', 'is-danger');
    if (kind) el.classList.add('is-' + kind);
}

// Tool success-rate badge colouring
function setKpiRateBadge(id, rate, failedCount) {
    const el = document.getElementById(id);
    if (!el) return;
    el.classList.remove('is-pending', 'is-running', 'is-idle', 'is-warning', 'is-success', 'is-danger');
    if (rate >= 95) {
        el.textContent = dt('dashboard.healthyStatus', null, 'Running smoothly');
        el.classList.add('is-success');
    } else if (rate >= 80) {
        el.textContent = dt('dashboard.normalStatus', null, 'Mostly normal') + (failedCount > 0 ? ' · ' + dt('dashboard.failedNCalls', { count: failedCount }, failedCount + ' failed') : '');
        el.classList.add('is-warning');
    } else {
        el.textContent = dt('dashboard.degradedStatus', null, 'Needs attention') + (failedCount > 0 ? ' · ' + dt('dashboard.failedNCalls', { count: failedCount }, failedCount + ' failed') : '');
        el.classList.add('is-danger');
    }
}

function renderDashboardTokenUsage(res) {
    const summary = res && res.summary ? res.summary : null;
    if (!summary) {
        setEl('dashboard-KPI-token-usage', '-');
        setKpiSubText('dashboard-KPI-token-sub-text', '-');
        return;
    }
    const total = Number(summary.totalTokens || 0);
    const calls = Number(summary.modelCalls || 0);
    const today = res && res.today ? Number(res.today.totalTokens || 0) : 0;
    if (!Number.isFinite(total) || total <= 0) {
        setEl('dashboard-KPI-token-usage', '0');
        setKpiSubText('dashboard-KPI-token-sub-text', dt('dashboard.noTokenUsageYet', null, 'No usage yet'));
        return;
    }
    setEl('dashboard-KPI-token-usage', formatTokenUsageCompact(total));
    setKpiSubText('dashboard-KPI-token-sub-text',
        dt('dashboard.tokenUsageSub', {
            today: formatTokenUsageCompact(today),
            calls: Number.isFinite(calls) ? calls : 0
        }, 'Last 7 days ' + (Number.isFinite(calls) ? calls : 0) + ' calls · today ' + formatTokenUsageCompact(today)));
}

function formatTokenUsageCompact(num) {
    const n = Number(num || 0);
    if (!Number.isFinite(n) || n <= 0) return '0';
    if (n >= 1000000) {
        return (n / 1000000).toFixed(n >= 10000000 ? 0 : 1).replace(/\.0$/, '') + 'M';
    }
    if (n >= 1000) {
        return (n / 1000).toFixed(n >= 10000 ? 0 : 1).replace(/\.0$/, '') + 'K';
    }
    return String(Math.trunc(n));
}

// sessionStorage: alert banner "×" dismiss record + the reason fragment from the last time it was **actually shown** (without level),
// used to avoid mistakenly re-applying an earlier dismiss for a subset when problems go from many to few (e.g. after reviewing HITL, only critical vulns remain).
var DASH_SESSION_ALERT_DISMISSED = 'dashboard.dismissedAlert';
var DASH_SESSION_ALERT_LAST_REASONS = 'dashboard.alertLastReasons';

function dashboardAlertReasonKeySetFromJoined(s) {
    if (!s || typeof s !== 'string') return new Set();
    return new Set(s.split(',').map(function (x) { return x.trim(); }).filter(Boolean));
}

/** Whether the currentReason fragment is a strict subset of the last-shown fragment (used to clear a stale dismiss) */
function dashboardAlertCurrentIsStrictSubsetOfLastShown(currentReasonJoined, lastReasonJoined) {
    var cur = dashboardAlertReasonKeySetFromJoined(currentReasonJoined);
    var last = dashboardAlertReasonKeySetFromJoined(lastReasonJoined);
    if (cur.size === 0 || last.size === 0) return false;
    if (cur.size >= last.size) return false;
    var ok = true;
    cur.forEach(function (k) {
        if (!last.has(k)) ok = false;
    });
    return ok;
}

// Key alert banner: render or hide based on severity status.
//   - level: danger (red) > warning (orange) > info (blue), auto-picks the highest from reasons
//   - After user clicks ×, stores currentReasons fingerprint in sessionStorage; same content in this session is auto-skipped
//   - When the reasons set changes (e.g. a new problem type is added), the fingerprint is invalidated and the banner re-appears
//   - If a larger set of problems was shown before and some subsequently resolved, the dismiss is cleared and the remaining ones are re-shown (see dashboard.alertLastReasons)
function renderDashboardAlertBanner(stats) {
    const banner = document.getElementById('dashboard-alert-banner');
    const titleEl = document.getElementById('dashboard-alert-title');
    const descEl = document.getElementById('dashboard-alert-desc');
    const actsEl = document.getElementById('dashboard-alert-actions');
    if (!banner || !titleEl || !descEl || !actsEl) return;

    const reasons = [];
    // Compute fingerprint from reasonKeys (without localised strings, so switching language won't re-show the banner)
    const reasonKeys = [];
    let level = 'info'; // info | warning | danger

    if (stats.criticalCount > 0) {
        reasons.push(dt('dashboard.alertCriticalReason', { count: stats.criticalCount },
            stats.criticalCount + ' open critical vulnerabilities found — immediate action recommended'));
        reasonKeys.push('crit:' + stats.criticalCount);
        level = 'danger';
    }
    if (stats.hitlPending > 0) {
        // HITL pending approvals block the Agent workflow; shown as a separate entry; does not raise level unless already at info→warning
        reasons.push(dt('dashboard.alertHitlReason', { count: stats.hitlPending },
            stats.hitlPending + ' Human-in-the-loop request(s) pending approval — Agent is waiting for your decision'));
        reasonKeys.push('HITL:' + stats.hitlPending);
        if (level === 'info') level = 'warning';
    }
    if (stats.successRate >= 0 && stats.successRate < 80 && stats.failedTools > 0) {
        reasons.push(dt('dashboard.alertfailedReason', { count: stats.failedTools },
            'Tool call success rate is low (' + stats.failedTools + ' failed) — check MCP monitor'));
        reasonKeys.push('rate:' + Math.round(stats.successRate) + ':' + stats.failedTools);
        if (level === 'info') level = 'warning';
    }
    if (stats.externalMcpDown > 0) {
        // External MCP server count > 0: affects tool availability
        reasons.push(dt('dashboard.alertMcpDownReason', { count: stats.externalMcpDown },
            stats.externalMcpDown + ' External MCP server(s) not running — related tools unavailable'));
        reasonKeys.push('MCP:' + stats.externalMcpDown);
        if (level === 'info') level = 'warning';
    }

    if (reasons.length === 0) {
        banner.hidden = true;
        banner.classList.remove('is-warning', 'is-danger', 'is-info');
        dashboardState.dismissedAlertKey = null;
        try { sessionStorage.removeItem(DASH_SESSION_ALERT_LAST_REASONS); } catch (_) {}
        return;
    }

    var fingerprint = level + '|' + reasonKeys.join(',');
    var reasonPartJoined = reasonKeys.join(',');

    // Check whether this session has already dismissed the same content; if currentIs a strict subset of the last-shown combination, clear the dismiss (best practice: continue alerting remaining items after partial resolution)
    var dismissed = null;
    try { dismissed = sessionStorage.getItem(DASH_SESSION_ALERT_DISMISSED); } catch (_) {}
    var lastShownReasons = '';
    try { lastShownReasons = sessionStorage.getItem(DASH_SESSION_ALERT_LAST_REASONS) || ''; } catch (_) {}

    if (dismissed === fingerprint && dashboardAlertCurrentIsStrictSubsetOfLastShown(reasonPartJoined, lastShownReasons)) {
        try {
            sessionStorage.removeItem(DASH_SESSION_ALERT_DISMISSED);
            dismissed = null;
        } catch (_) { /* ignore */ }
    }

    dashboardState.dismissedAlertKey = fingerprint;

    if (dismissed === fingerprint) {
        banner.hidden = true;
        return;
    }

    banner.hidden = false;
    banner.classList.remove('is-warning', 'is-danger', 'is-info');
    banner.classList.add('is-' + level);

    if (level === 'danger') {
        titleEl.textContent = dt('dashboard.alertDangerTitle', null, 'Immediate action required');
    } else if (level === 'warning') {
        titleEl.textContent = dt('dashboard.alertWarningTitle', null, 'Needs attention');
    } else {
        titleEl.textContent = dt('dashboard.alertTitle', null, 'Notice');
    }

    descEl.textContent = reasons.join('; ');

    actsEl.innerHTML = '';
    if (stats.criticalCount > 0) {
        const btn = document.createElement('button');
        btn.className = 'dashboard-alert-btn';
        btn.textContent = dt('dashboard.viewVulns', null, 'viewvulnerability');
        btn.onclick = function () { try { switchPage('vulnerabilities'); } catch (e) {} };
        actsEl.appendChild(btn);
    }
    if (stats.hitlPending > 0) {
        const btn = document.createElement('button');
        btn.className = 'dashboard-alert-btn dashboard-alert-btn-secondary';
        btn.textContent = dt('dashboard.viewHitl', null, 'Go to approvals');
        btn.onclick = function () { try { switchPage('HITL'); } catch (e) {} };
        actsEl.appendChild(btn);
    }
    if (stats.successRate >= 0 && stats.successRate < 80) {
        const btn = document.createElement('button');
        btn.className = 'dashboard-alert-btn dashboard-alert-btn-secondary';
        btn.textContent = dt('dashboard.viewMonitor', null, 'View monitor');
        btn.onclick = function () { try { switchPage('mcp-monitor'); } catch (e) {} };
        actsEl.appendChild(btn);
    }
    if (stats.externalMcpDown > 0) {
        const btn = document.createElement('button');
        btn.className = 'dashboard-alert-btn dashboard-alert-btn-secondary';
        btn.textContent = dt('dashboard.viewMcpManagement', null, 'Manage MCP');
        btn.onclick = function () { try { switchPage('mcp-management'); } catch (e) {} };
        actsEl.appendChild(btn);
    }

    try { sessionStorage.setItem(DASH_SESSION_ALERT_LAST_REASONS, reasonPartJoined); } catch (_) {}
}

// External MCP health: parsed from /api/external-MCP/stats (backend fields: total/enabled/disabled/connected),
// decides whether to show in the "Capability Overview" row, and returns the count of "enabled but disconnected" to the alert banner.
function renderExternalMcpHealth(stats) {
    var row = document.getElementById('dashboard-resource-external-mcp-row');
    var textEl = document.getElementById('dashboard-resource-external-mcp-text');
    var healthEl = document.getElementById('dashboard-resource-external-mcp-health');
    if (!row || !textEl) return 0;

    if (!stats || typeof stats !== 'object') {
        row.hidden = true;
        return 0;
    }
    var total = Number(stats.total ?? stats.Total ?? 0) || 0;
    var enabled = Number(stats.enabled ?? stats.enabled ?? 0) || 0;
    // Backend uses "connected" for connected count; compatible with legacy field "running"
    var connected = Number(stats.connected ?? stats.Connected ??
        stats.running ?? stats.running ?? 0) || 0;
    if (total === 0) {
        row.hidden = true;
        return 0;
    }
    // When no "enabled" External MCP is configured, do not show the health row or alert (consistent with MCP management page)
    if (enabled === 0) {
        row.hidden = true;
        return 0;
    }
    var down = Math.max(0, enabled - connected);
    row.hidden = false;
    textEl.textContent = formatNumber(connected) + ' / ' + formatNumber(enabled);
    if (healthEl) {
        healthEl.classList.remove('is-ok', 'is-warning', 'is-danger');
        if (down === 0) {
            healthEl.classList.add('is-ok');
            healthEl.textContent = dt('dashboard.mcpAllRunning', null, 'allrun');
        } else if (down < enabled) {
            healthEl.classList.add('is-warning');
            healthEl.textContent = dt('dashboard.mcpPartialDown', { count: down },
                down + ' not running');
        } else {
            healthEl.classList.add('is-danger');
            healthEl.textContent = dt('dashboard.mcpAllDown', null, 'All not running');
        }
        healthEl.hidden = false;
    }
    return down;
}

// HITL pending approval count: returns the number of pending items; can also be used in capability overview or KPI sub-text
function getHitlPendingCount(res) {
    if (!res) return 0;
    if (Array.isArray(res. items)) return res. items.length;
    if (typeof res.total === 'number') return res.total;
    if (Array.isArray(res)) return res.length;
    return 0;
}

// "Recent Events" inline display: takes the N most important entries from the notification summary
// Design principles:
//   - Do not duplicate "new vulnerability" notifications already expressed by alert banner / KPI (vulnerability_created is still filtered)
//   - HITL pending approvals are hinted at in recommended actions etc., but are still shown here in the timeline alongside task completions etc.
//   - The entire section is hidden when there is nothing to show, to avoid empty modules taking up space
function renderRecentEvents(notifRes) {
    var section = document.getElementById('dashboard-section-events');
    var listEl = document.getElementById('dashboard-events-list');
    if (!section || !listEl) return;

    var  items = (notifRes && Array.isArray(notifRes. items)) ? notifRes. items : [];
    // Filter: remove new-vulnerability type (to avoid duplication with "recent vulnerabilities" panel); HITL is no longer filtered
    var coveredTypes = { 'vulnerability_created': true };
    var filtered =  items.filter(function (it) {
        if (!it || !it.type) return false;
        if (coveredTypes[it.type]) return false;
        return true;
    });

    // Sort by level: p0 > p1 > p2, then by time descending
    var levelOrder = { p0: 0, p1: 1, p2: 2 };
    filtered.sort(function (a, b) {
        var la = levelOrder[a.level] != null ? levelOrder[a.level] : 9;
        var lb = levelOrder[b.level] != null ? levelOrder[b.level] : 9;
        if (la !== lb) return la - lb;
        var ta = a.ts || a.createdAt || a.created_at || 0;
        var tb = b.ts || b.createdAt || b.created_at || 0;
        return new Date(tb).getTime() - new Date(ta).getTime();
    });

    var top = filtered.slice(0, 3);
    if (top.length === 0) {
        section.hidden = true;
        listEl.innerHTML = '';
        return;
    }
    section.hidden = false;

    listEl.innerHTML = top.map(function (it) {
        var level = it.level || 'p2';
        var title = esc(it.title || it.message || dt('dashboard.eventUntitled', null, 'Event'));
        var msg = esc(it.message || it.summary || it.desc || '');
        var whenRaw = timeAgoStr(it.ts || it.createdAt || it.created_at);
        var when = esc(whenRaw || '—');
        return (
            '<div class="dashboard-event-item lvl-' + esc(level) + '">' +
            '<span class="dashboard-event-dot" aria-hidden="true"></span>' +
            '<div class="dashboard-event-body">' +
            '<div class="dashboard-event-title">' + title + '</div>' +
            (msg && msg !== title ? '<div class="dashboard-event-msg">' + msg + '</div>' : '') +
            '</div>' +
            '<span class="dashboard-event-time">' + when + '</span>' +
            '</div>'
        );
    }).join('');
}

// Recommended Actions: intelligently generate "What to do next" based on currentData status.
// Design principles: every item must be clickable to reach the relevant page; sorted by priority (urgent > maintenance > setup);
// show at most 3-5 items at a time; hide the entire section when there is nothing to recommend.
function renderRecommendedActions(state) {
    var section = document.getElementById('dashboard-section-recommend');
    var listEl = document.getElementById('dashboard-recommend-list');
    if (!section || !listEl) return;

    var actions = [];

    // Urgent: unresolved critical vulnerabilities
    if (state.openCriticalCount > 0) {
        actions.push({
            level: 'urgent',
            icon: '<SVG width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><circle cx="12" cy="17" r="1" fill="currentColor" stroke="none"/></SVG>',
            title: dt('dashboard.recoFixCritical', { count: state.openCriticalCount },
                'Fix ' + state.openCriticalCount + ' open critical vulnerability(s)'),
            desc: dt('dashboard.recoFixCriticalDesc', null, 'Critical-severity vulnerabilities should be remediated first'),
             page: 'vulnerabilities'
        });
    }
    // Urgent: HITL pending approvals
    if (state.hitlPending > 0) {
        actions.push({
            level: 'urgent',
            icon: '<SVG width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></SVG>',
            title: dt('dashboard.recoapproveHitl', { count: state.hitlPending },
                'Approve ' + state.hitlPending + ' HITL request(s)'),
            desc: dt('dashboard.recoApproveHitlDesc', null, 'Agent is waiting for your decision to continue'),
             page: 'HITL'
        });
    }
    // Maintenance: External MCP abnormalities
    if (state.externalMcpDown > 0) {
        actions.push({
            level: 'warning',
            icon: '<SVG width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></SVG>',
            title: dt('dashboard.recoRestartMcp', { count: state.externalMcpDown },
                'Check ' + state.externalMcpDown + ' External MCP server(s) not running'),
            desc: dt('dashboard.recoRestartMcpDesc', null, 'Related tools will be unavailable until MCP service is restored'),
             page: 'mcp-management'
        });
    }
    // Maintenance: high failure rate
    if (state.successRate >= 0 && state.successRate < 80 && state.failedTools > 0) {
        actions.push({
            level: 'warning',
            icon: '<SVG width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></SVG>',
            title: dt('dashboard.recoCheckMonitor', { count: state.failedTools },
                'Investigate ' + state.failedTools + ' failed tool call(s)'),
            desc: dt('dashboard.recoCheckMonitorDesc', null, 'View failed request details in MCP monitor'),
             page: 'mcp-monitor'
        });
    }
    // Setup: first-run scenario
    if (state.toolsConfigured === 0) {
        actions.push({
            level: 'setup',
            icon: '<SVG width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></SVG>',
            title: dt('dashboard.recoSetupMcp', null, 'Configure your first MCP tool'),
            desc: dt('dashboard.recoSetupMcpDesc', null, 'Agent can only invoke specific capabilities after an MCP service is installed'),
             page: 'mcp-management'
        });
    }
    if (state.totalVulns === 0 && state.totalRunning === 0 && state.toolsConfigured > 0) {
        actions.push({
            level: 'setup',
            icon: '<SVG width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></SVG>',
            title: dt('dashboard.recoStartScan', null, 'Start a scan in Chat'),
            desc: dt('dashboard.recoStartScanDesc', null, 'Describe the target in Chat and let AI assist with execution'),
             page: 'chat'
        });
    }

    if (actions.length === 0) {
        section.hidden = true;
        listEl.innerHTML = '';
        return;
    }
    section.hidden = false;
    listEl.innerHTML = actions.slice(0, 5).map(function (a) {
        return (
            '<a class="dashboard-recommend-item lvl-' + a.level + '" data-page="' + esc(a. page) + '" role="button" tabIndex="0">' +
            '<span class="dashboard-recommend-icon" aria-hidden="true">' + a.icon + '</span>' +
            '<div class="dashboard-recommend-body">' +
            '<div class="dashboard-recommend-title">' + esc(a.title) + '</div>' +
            '<div class="dashboard-recommend-desc">' + esc(a.desc) + '</div>' +
            '</div>' +
            '<span class="dashboard-recommend-arrow" aria-hidden="true">→</span>' +
            '</a>'
        );
    }).join('');

    // Delegate click/keyboard on recommended items → switchPage
    Array.from(listEl.querySelectorAll('.dashboard-recommend-item')).forEach(function (el) {
        var  page = el.getAttribute('data-page');
        el.onclick = function () { try { switchPage( page); } catch (_) {} };
        el.onkeydown = function (e) {
            if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); el.click(); }
        };
    });
}

// Smart CTA: hide "Start your security journey" when any data exists (tasks running / vulnerabilities / tool calls / MCP configured);
// only keep it as a guide in a truly empty, brand-new environment
function updateSmartCTA(state) {
    var CTA = document.getElementById('dashboard-CTA-block');
    if (!CTA) return;
    var hasData = (
        (state.totalRunning || 0) > 0 ||
        (state.totalVulns || 0) > 0 ||
        (state.totalCalls || 0) > 0 ||
        (state.toolsConfigured || 0) > 0 ||
        (state.batchTotal || 0) > 0
    );
    CTA.hidden = hasData;
}

// "Last updated" time display; also records lastupdatedAt for stale checks and clears stale status
function updateLastUpdatedNow() {
    dashboardState.lastupdatedAt = Date.now();
    const el = document.getElementById('dashboard-last-updated-time');
    if (!el) return;
    const d = new Date();
    const pad = function (n) { return n < 10 ? '0' + n : String(n); };
    el.textContent = pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
    const wrap = document.getElementById('dashboard-last-updated');
    if (wrap) {
        wrap.classList.remove('is-stale');
        wrap.classList.remove('is-flash');
        // trigger reflow then add class for the flash animation
        void wrap.offsetWidth;
        wrap.classList.add('is-flash');
    }
    const stale = document.getElementById('dashboard-last-updated-stale');
    if (stale) stale.hidden = true;
}

// Stale data check: if not refreshed within DASHBOARD_STALE_THRESHOLD_MS, add .is-stale class to badge,
// showing ⚠️ icon to hint "this data may be stale — please refresh manually or check network"
function checkDashboardStale() {
    if (!dashboardState.lastupdatedAt) return;
    var ageMs = Date.now() - dashboardState.lastupdatedAt;
    var wrap = document.getElementById('dashboard-last-updated');
    var stale = document.getElementById('dashboard-last-updated-stale');
    if (!wrap) return;
    if (ageMs > DASHBOARD_STALE_THRESHOLD_MS) {
        wrap.classList.add('is-stale');
        if (stale) stale.hidden = false;
    } else {
        wrap.classList.remove('is-stale');
        if (stale) stale.hidden = true;
    }
}

// Auto-polling: silently refresh every 60 seconds when Dashboard is active and tab is visible.
// The setInterval keeps running when the tab is hidden, but each tick checks and skips the actual refresh;
// on becoming visible again, checks lastupdatedAt to decide whether an immediate catch-up refresh is needed (>= half the interval triggers one).
function startDashboardAutoRefresh() {
    if (dashboardState.pollTimer) return;
    dashboardState.pollTimer = setInterval(function () {
        try {
            var  page = document.getElementById('page-dashboard');
            if (! page || ! page.classList.contains('active')) return;
            if (typeof document !== 'undefined' && document.hidden) return;
            refreshDashboard();
        } catch (e) {
            console.warn('auto refresh tick failed', e);
        }
    }, DASHBOARD_POLL_INTERVAL_MS);

    if (!dashboardState.staleTimer) {
        dashboardState.staleTimer = setInterval(checkDashboardStale, DASHBOARD_STALE_CHECK_INTERVAL_MS);
    }
}

function stopDashboardAutoRefresh() {
    if (dashboardState.pollTimer) {
        clearInterval(dashboardState.pollTimer);
        dashboardState.pollTimer = null;
    }
    if (dashboardState.staleTimer) {
        clearInterval(dashboardState.staleTimer);
        dashboardState.staleTimer = null;
    }
}

// Severity colours and labels
var SEVERITY_LABELS_FALLBACK = {
    critical: 'Critical', high: 'High', medium: 'Medium', low: 'Low', info: ' info'
};

function severityShortLabel(id) {
    const key = 'dashboard.severity' + id.charAt(0).toUpperCase() + id.slice(1);
    return t(key, null, SEVERITY_LABELS_FALLBACK[id] || id);
}

// Human-friendly relative time: "5 minutes ago" / "2 hours ago" / "Yesterday" / "3 days ago"
function timeAgoStr(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    const diffSec = Math.max(0, Math.floor((Date.now() - d.getTime()) / 1000));
    if (diffSec < 60) return dt('common.justNow', null, 'Just now');
    const min = Math.floor(diffSec / 60);
    if (min < 60) return dt('common.minutesAgo', { n: min }, min + ' minutes ago');
    const hr = Math.floor(min / 60);
    if (hr < 24) return dt('common.hoursAgo', { n: hr }, hr + ' hours ago');
    const day = Math.floor(hr / 24);
    if (day < 7) return dt('common.daysAgo', { n: day }, day + ' days ago');
    // Show date for anything older than a week
    return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
}

// Recent vulnerabilities list
function setRecentVulnsLoading() {
    const wrap = document.getElementById('dashboard-recent-vulns');
    const empty = document.getElementById('dashboard-recent-vulns-empty');
    if (!wrap) return;
    Array.from(wrap.querySelectorAll('.dashboard-recent-vuln-item')).forEach(function (n) { n.remove(); });
    if (empty) {
        empty.hidden = false;
        empty.classList.remove('is-rich');
        empty.textContent = dt('common.loading', null, 'Loading…');
    }
}

function setRecentVulnsError() {
    const wrap = document.getElementById('dashboard-recent-vulns');
    const empty = document.getElementById('dashboard-recent-vulns-empty');
    if (!wrap) return;
    Array.from(wrap.querySelectorAll('.dashboard-recent-vuln-item')).forEach(function (n) { n.remove(); });
    if (empty) {
        empty.hidden = false;
        empty.classList.remove('is-rich');
        empty.textContent = dt('common.loadFailed', null, 'Load failed');
    }
}

function renderRecentVulns(res) {
    const wrap = document.getElementById('dashboard-recent-vulns');
    const empty = document.getElementById('dashboard-recent-vulns-empty');
    if (!wrap) return;

    Array.from(wrap.querySelectorAll('.dashboard-recent-vuln-item')).forEach(function (n) { n.remove(); });

    const list = res && Array.isArray(res.vulnerabilities) ? res.vulnerabilities : [];
    if (list.length === 0) {
        if (empty) {
            empty.hidden = false;
            // Rich empty state: title + description + action button, easier to guide users than plain text
            empty.classList.add('is-rich');
            empty.innerHTML = (
                '<div class="dashboard-empty-title">' + esc(dt('dashboard.noVulnYet', null, 'No recent vulnerabilities')) + '</div>' +
                '<div class="dashboard-empty-desc">' + esc(dt('dashboard.noVulnDesc', null, 'Recent vulnerability records appear here; new results will show up after completing a scan in Chat')) + '</div>' +
                '<button type="button" class="dashboard-empty-action" data-action="scan">' +
                esc(dt('dashboard.startScanBtn', null, 'Go to Chat to start a scan')) + ' →</button>'
            );
            var btn = empty.querySelector('[data-action="scan"]');
            if (btn) btn.onclick = function () { try { switchPage('chat'); } catch (_) {} };
        }
        return;
    }
    if (empty) {
        empty.hidden = true;
        empty.classList.remove('is-rich');
    }

    list.slice(0, 10).forEach(function (v) {
        const sev = (v.severity || 'info').toLowerCase();
        const status = (v.status || 'open').toLowerCase();
        const item = document.createElement('a');
        item.className = 'dashboard-recent-vuln-item';
        item.setAttribute('role', 'button');
        item.tabIndex = 0;
        item.onclick = function () { try { switchPage('vulnerabilities'); } catch (e) {} };
        item.onkeydown = function (e) { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); item.click(); } };

        const severityBadge = '<span class="dashboard-recent-vuln-sev sev-' + sev + '">' + esc(severityShortLabel(sev)) + '</span>';
        const title = '<span class="dashboard-recent-vuln-title" title="' + esc(v.title || '') + '">' + esc(v.title || dt('common.untitled', null, 'no title')) + '</span>';
        const target = v.target ? ('<span class="dashboard-recent-vuln-target" title="' + esc(v.target) + '">' + esc(v.target) + '</span>') : '<span class="dashboard-recent-vuln-target"></span>';
        const statusPill = '<span class="dashboard-recent-vuln-status st-' + esc(statusKey(status)) + '"><span class="dashboard-recent-vuln-status-dot"></span>' + esc(statusShortLabel(status)) + '</span>';
        const time = '<span class="dashboard-recent-vuln-time">' + esc(timeAgoStr(v.created_at)) + '</span>';

        item.innerHTML = severityBadge + title + target + statusPill + time;
        wrap.appendChild(item);
    });
}

// Recent Vulnerabilities / Recent Facts tab switching (shared list area; "view all" link changes with tab)
function switchDashboardFeedTab(tab) {
    tab = tab === 'facts' ? 'facts' : 'vulns';
    dashboardState.recentFeedTab = tab;

    var tabVulns = document.getElementById('dashboard-feed-tab-vulns');
    var tabFacts = document.getElementById('dashboard-feed-tab-facts');
    var panelVulns = document.getElementById('dashboard-feed-panel-vulns');
    var panelFacts = document.getElementById('dashboard-feed-panel-facts');
    if (tabVulns) {
        tabVulns.classList.toggle('is-active', tab === 'vulns');
        tabVulns.setAttribute('aria-selected', tab === 'vulns' ? 'true' : 'false');
    }
    if (tabFacts) {
        tabFacts.classList.toggle('is-active', tab === 'facts');
        tabFacts.setAttribute('aria-selected', tab === 'facts' ? 'true' : 'false');
    }
    if (panelVulns) panelVulns.hidden = tab !== 'vulns';
    if (panelFacts) panelFacts.hidden = tab !== 'facts';
    updateDashboardFeedViewAll(tab);
}

function updateDashboardFeedViewAll(tab) {
    var link = document.getElementById('dashboard-feed-view-all');
    if (!link) return;
    if (tab === 'facts') {
        link.onclick = function () { try { switchPage('projects'); } catch (_) {} };
    } else {
        link.onclick = function () { try { switchPage('vulnerabilities'); } catch (_) {} };
    }
}

function setRecentFactsLoading() {
    var wrap = document.getElementById('dashboard-recent-facts');
    var empty = document.getElementById('dashboard-recent-facts-empty');
    if (!wrap) return;
    clearRecentFactsList(wrap);
    if (empty) {
        empty.hidden = false;
        empty.classList.remove('is-rich');
        empty.textContent = dt('common.loading', null, 'Loading…');
    }
}

function clearRecentFactsList(wrap) {
    if (!wrap) return;
    Array.from(wrap.querySelectorAll('.dashboard-recent-fact-item, .dashboard-recent-facts-meta')).forEach(function (n) { n.remove(); });
}

function setRecentFactsError() {
    var wrap = document.getElementById('dashboard-recent-facts');
    var empty = document.getElementById('dashboard-recent-facts-empty');
    if (!wrap) return;
    clearRecentFactsList(wrap);
    if (empty) {
        empty.hidden = false;
        empty.classList.remove('is-rich');
        empty.textContent = dt('common.loadFailed', null, 'Load failed');
    }
}

function factConfidenceShortLabel(confidence) {
    var c = String(confidence || '').toLowerCase();
    if (c === 'confirmed') return dt('projects.confidenceConfirmed', null, 'confirmed');
    if (c === 'tentative') return dt('projects.confidenceTentative', null, 'Tentative');
    return c || '—';
}

function factCategoryShortLabel(category) {
    var raw = String(category || '').trim();
    return raw || 'note';
}

// Stably map 8 colour tones by project_id (fallback to project_name) so the same project keeps the same colour across refreshes
function projectFactProjectTone(projectId, projectName) {
    var key = String(projectId || projectName || '').trim();
    if (!key) return 0;
    var hash = 0;
    for (var i = 0; i < key.length; i++) {
        hash = ((hash << 5) - hash) + key.charCodeAt(i);
        hash |= 0;
    }
    return Math.abs(hash) % 8;
}

function openProjectFactFromDashboard(projectId, factKey) {
    if (!projectId) return;
    if (typeof switchPage === 'function') {
        switchPage('projects');
    }
    setTimeout(async function () {
        if (typeof window.initProjectsPage === 'function') {
            await window.initProjectsPage();
        }
        if (typeof window.selectProject === 'function') {
            await window.selectProject(projectId);
        }
        if (typeof window.switchProjectTab === 'function') {
            window.switchProjectTab('facts');
        }
        if (factKey && typeof window.viewProjectFactBody === 'function') {
            window.viewProjectFactBody(factKey);
        }
    }, 350);
}

function renderRecentFacts(res) {
    var wrap = document.getElementById('dashboard-recent-facts');
    var empty = document.getElementById('dashboard-recent-facts-empty');
    if (!wrap) return;

    clearRecentFactsList(wrap);

    var list = (res && Array.isArray(res.recent_facts)) ? res.recent_facts : [];
    var totals = (res && res.totals) ? res.totals : {};
    var activeProjects = totals.active_projects || 0;
    var totalFacts = totals.total_facts || 0;

    if (list.length === 0) {
        if (empty) {
            empty.hidden = false;
            empty.classList.add('is-rich');
            var desc = activeProjects > 0
                ? dt('dashboard.noFactsDesc', null, 'In a Chat linked to a project, the Agent will auto-record facts such as targets, vulnerabilities, and attack chains')
                : dt('projects.selectOrCreateHint', null, 'Projects enable a shared Fact board across Chats: targets, environment, auth info, etc. are auto-injected in Chats linked to the project.');
            var ctaLabel = activeProjects > 0
                ? dt('dashboard.goToChat', null, 'Go to Chat')
                : dt('dashboard.createFirstProjectBtn', null, 'Create your first project');
            var ctaAction = activeProjects > 0 ? 'chat' : 'project';
            empty.innerHTML = (
                '<div class="dashboard-empty-title">' + esc(dt('dashboard.noFactsYet', null, 'No recent facts')) + '</div>' +
                '<div class="dashboard-empty-desc">' + esc(desc) + '</div>' +
                '<button type="button" class="dashboard-empty-action" data-action="' + esc(ctaAction) + '">' +
                esc(ctaLabel) + ' →</button>'
            );
            var btn = empty.querySelector('[data-action]');
            if (btn) {
                btn.onclick = function () {
                    var action = btn.getAttribute('data-action');
                    if (action === 'project') {
                        try { switchPage('projects'); } catch (_) {}
                        setTimeout(function () {
                            if (typeof window.showNewProjectModal === 'function') {
                                window.showNewProjectModal();
                            }
                        }, 350);
                    } else {
                        try { switchPage('chat'); } catch (_) {}
                    }
                };
            }
        }
        return;
    }

    if (empty) {
        empty.hidden = true;
        empty.classList.remove('is-rich');
    }

    list.slice(0, 10).forEach(function (f) {
        if (!f) return;
        var category = factCategoryShortLabel(f.category);
        var confidence = String(f.confidence || 'tentative').toLowerCase();
        var item = document.createElement('a');
        item.className = 'dashboard-recent-fact-item';
        item.setAttribute('role', 'button');
        item.tabIndex = 0;
        var pid = f.project_id || '';
        var fkey = f.fact_key || '';
        item.onclick = function () { openProjectFactFromDashboard(pid, fkey); };
        item.onkeydown = function (e) {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                item.click();
            }
        };

        // Pin column always reserves space so subsequent columns don't shift when pin is present/absent
        var pinMark = '<span class="dashboard-recent-fact-pin' + (f.pinned ? ' is-pinned' : '') + '"' +
            (f.pinned ? (' title="' + esc(dt('projects.pinned', null, 'Pinned')) + '"') : '') +
            ' aria-hidden="true">' + (f.pinned ? '📌' : '') + '</span>';
        var projectLabel = (f.project_name || '').trim() || dt('projects.defaultProjectName', null, 'Project');
        var factKeyLabel = (f.fact_key || '').trim() || '—';
        var projectTone = projectFactProjectTone(pid, projectLabel);
        var projectCol = '<span class="dashboard-recent-fact-project proj-tone-' + projectTone + '" title="' + esc(projectLabel) + '">' + esc(projectLabel) + '</span>';
        var categoryBadge = '<span class="dashboard-recent-fact-cat cat-' + esc(category.toLowerCase().replace(/[^a-z0-9_-]/g, '')) + '">' + esc(category) + '</span>';
        var confBadge = '<span class="dashboard-recent-fact-conf conf-' + esc(confidence) + '">' + esc(factConfidenceShortLabel(confidence)) + '</span>';
        var summary = '<span class="dashboard-recent-fact-summary" title="' + esc(f.summary || '') + '">' + esc(f.summary || dt('common.untitled', null, 'no title')) + '</span>';
        var factKeyCol = '<span class="dashboard-recent-fact-key" title="' + esc(factKeyLabel) + '">' + esc(factKeyLabel) + '</span>';
        var time = '<span class="dashboard-recent-fact-time">' + esc(timeAgoStr(f.updated_at)) + '</span>';

        item.innerHTML = pinMark + categoryBadge + confBadge + summary + factKeyCol + projectCol + time;
        wrap.appendChild(item);
    });
}

// Vulnerability status mapping: normalises status string to 4 categories (guards against dirty data)
function statusKey(s) {
    s = String(s || '').toLowerCase();
    if (s === 'fixed' || s === 'closed' || s === 'resolved') return 'fixed';
    if (s === 'confirmed') return 'confirmed';
    if (s === 'false_positive' || s === 'false-positive' || s === 'fp') return 'fp';
    if (s === 'ignored') return 'ignored';
    return 'open';
}

function statusShortLabel(s) {
    const k = statusKey(s);
    if (k === 'fixed') return dt('dashboard.statusFixed', null, 'Fixed');
    if (k === 'confirmed') return dt('dashboard.statusConfirmed', null, 'confirmed');
    if (k === 'fp') return dt('dashboard.statusFalsePositive', null, 'False positive');
    if (k === 'ignored') return dt('dashboard.statusIgnored', null, 'ignored');
    return dt('dashboard.statusOpen', null, 'Open');
}

// Format number with thousands separator
function formatNumber(num) {
    if (typeof num !== 'number' || isNaN(num)) return '-';
    if (num === 0) return '0';
    return num.toLocaleString('zh-CN');
}

// Update progress bar width
function updateProgressBar(id, percentage) {
    const el = document.getElementById(id);
    if (el) {
        const pct = parseFloat(percentage) || 0;
        el.style.width = Math.max(0, Math.min(100, pct)) + '%';
    }
}

// Top 30 tool execution-count bar chart colours (30 non-repeating, soft, easy to distinguish)
var DASHBOARD_BAR_COLORS = [
    '#93c5fd', '#a78bfa', '#6ee7b7', '#fde047', '#fda4af',
    '#7dd3fc', '#a5b4fc', '#5eead4', '#fdba74', '#e9d5ff',
    '#67e8f9', '#c4b5fd', '#86efac', '#fcd34d', '#f9a8d4',
    '#bae6fd', '#c7d2fe', '#99f6e4', '#fed7aa', '#ddd6fe',
    '#22d3ee', '#8b5cf6', '#4ade80', '#fbbf24', '#fb7185',
    '#38bdf8', '#818cf8', '#2dd4bf', '#fb923c', '#e0e7ff'
];

function esc(s) {
    if (typeof s !== 'string') return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
}

// Vulnerability remediation status + fix progress panel
// byStatus: { open, confirmed, fixed, false_positive, ignored } (any missing field treated as 0)
// total: total vulnerability count (from stats.total)
function renderVulnStatusPanel(byStatus, total) {
    var get = function (k) {
        if (!byStatus || typeof byStatus !== 'object') return 0;
        return Number(byStatus[k] || 0) || 0;
    };
    var open = get('open');
    var confirmed = get('confirmed');
    var fixed = get('fixed');
    var fp = get('false_positive');
    var ignored = get('ignored');

    setEl('dashboard-status-open', formatNumber(open));
    setEl('dashboard-status-confirmed', formatNumber(confirmed));
    setEl('dashboard-status-fixed', formatNumber(fixed));
    setEl('dashboard-status-fp', formatNumber(fp));
    setEl('dashboard-status-ignored', formatNumber(ignored));

    // Fix rate is calculated only for actionable vulnerabilities; false positives/ignored are neutral closures and do not lower the rate.
    var actionableTotal = open + confirmed + fixed;
    var rate = actionableTotal > 0 ? (fixed / actionableTotal) * 100 : 0;
    var rateStr = actionableTotal > 0 ? rate.toFixed(rate >= 100 ? 0 : 1) + '%' : '-';
    setEl('dashboard-fix-rate', rateStr);

    var detailEl = document.getElementById('dashboard-fix-detail');
    if (detailEl) {
        detailEl.textContent = '(' + formatNumber(fixed) + ' / ' + formatNumber(actionableTotal) + ')';
    }

    var fixedPct = actionableTotal > 0 ? (fixed / actionableTotal) * 100 : 0;
    var confirmedPct = actionableTotal > 0 ? (confirmed / actionableTotal) * 100 : 0;
    var fixedBar = document.getElementById('dashboard-fix-progress-fixed');
    var confirmedBar = document.getElementById('dashboard-fix-progress-confirmed');
    if (fixedBar) fixedBar.style.width = fixedPct.toFixed(2) + '%';
    if (confirmedBar) confirmedBar.style.width = confirmedPct.toFixed(2) + '%';
}

// Risk Overview card: compute weighted risk score + urgent badge based on "open" vulnerability severity distribution
//
// Why use the "open" scope instead of all:
//   With all vulnerabilities, by_severity doesn't change after all are fixed, so the risk score stays high,
//   but the urgent badge (pending Critical/High) drops to zero — creating a visual contradiction of "extreme risk + 0 open".
//   Switching to the "open" scope means fixing a vulnerability immediately removes its risk contribution,
//   keeping the risk level and urgent counts fully in sync.
//
// bySeverityOpen: { critical, high, medium, low } (only counts status=open vulnerabilities; info excluded)
// totalOpen:      total open vulnerability count (= critical + high + medium + low), used only for "no open → safe" check
// recentVulnsRes: /api/vulnerabilities?limit=10 response (used for "most recently found" time; scope is all vulnerabilities, unrelated to remediation status)
function renderSeverityInsights(bySeverityOpen, totalOpen, recentVulnsRes) {
    var riskBox = document.querySelector('.dashboard-severity-insight-risk');
    var levelEl = document.getElementById('dashboard-severity-risk-level');
    var fillEl = document.getElementById('dashboard-severity-risk-fill');
    var scoreEl = document.getElementById('dashboard-severity-risk-score');
    var urgentCriticalEl = document.getElementById('dashboard-severity-urgent-critical');
    var urgentHighEl = document.getElementById('dashboard-severity-urgent-high');
    var urgentCriticalCell = urgentCriticalEl ? urgentCriticalEl.closest('.dashboard-severity-insight-urgent-item') : null;
    var urgentHighCell = urgentHighEl ? urgentHighEl.closest('.dashboard-severity-insight-urgent-item') : null;
    var latestEl = document.getElementById('dashboard-severity-latest-time');

    var sev = bySeverityOpen && typeof bySeverityOpen === 'object' ? bySeverityOpen : {};
    var c = Number(sev.critical || 0) || 0;
    var h = Number(sev.high || 0) || 0;
    var m = Number(sev.medium || 0) || 0;
    var l = Number(sev.low || 0) || 0;

    // Weighted score: Critical ×10, High ×5, Medium ×2, Low ×0.5; info excluded
    // Thresholds are deliberately conservative: 1 open Critical → "medium", 2 → "high", ≥4 → "severe"
    var score = c * 10 + h * 5 + m * 2 + l * 0.5;
    var level, levelKey, levelFallback;
    var t = Number(totalOpen || 0) || 0;
    if (t === 0 || score === 0) {
        level = 'safe'; levelKey = 'dashboard.riskSafe'; levelFallback = 'Safe';
    } else if (score <= 3) {
        level = 'low'; levelKey = 'dashboard.riskLow'; levelFallback = 'Low';
    } else if (score <= 10) {
        level = 'medium'; levelKey = 'dashboard.riskMedium'; levelFallback = 'Medium';
    } else if (score <= 30) {
        level = 'high'; levelKey = 'dashboard.riskHigh'; levelFallback = 'High';
    } else {
        level = 'severe'; levelKey = 'dashboard.riskSevere'; levelFallback = 'Severe';
    }

    if (riskBox) riskBox.setAttribute('data-level', level);
    if (levelEl) levelEl.textContent = dt(levelKey, null, levelFallback);
    // Progress bar uses 0-100 linear mapping: >=100 fills completely
    var pct = Math.max(0, Math.min(100, score));
    if (fillEl) fillEl.style.width = pct.toFixed(1) + '%';
    if (scoreEl) {
        // Score to 1 decimal place (Low 0.5 weight can produce non-integers); show integer directly if whole
        var displayScore = Math.round(score) === score ? String(score) : score.toFixed(1);
        scoreEl.textContent = score >= 100 ? displayScore + '+' : displayScore;
    }

    // Urgent badge directly uses the open-scope critical / high counts (same source as the weighted score, so "severe risk + 0 open" contradiction cannot occur)
    if (urgentCriticalEl) urgentCriticalEl.textContent = formatNumber(c);
    if (urgentHighEl) urgentHighEl.textContent = formatNumber(h);
    if (urgentCriticalCell) urgentCriticalCell.classList.toggle('is-zero', c === 0);
    if (urgentHighCell) urgentHighCell.classList.toggle('is-zero', h === 0);

    if (latestEl) {
        var list = recentVulnsRes && Array.isArray(recentVulnsRes.vulnerabilities) ? recentVulnsRes.vulnerabilities : [];
        var latestIso = list.length > 0 ? list[0].created_at : null;
        var timeStr = latestIso ? timeAgoStr(latestIso) : '';
        if (timeStr) {
            latestEl.textContent = timeStr;
            latestEl.classList.remove('is-empty');
        } else {
            latestEl.textContent = dt('dashboard.noneYet', null, 'No ');
            latestEl.classList.add('is-empty');
        }
    }
}

function renderDashboardToolsBar(topTools) {
    const placeholder = document.getElementById('dashboard-tools-pie-placeholder');
    const barChartEl = document.getElementById('dashboard-tools-bar-chart');
    if (!placeholder || !barChartEl) return;

    if (!Array.isArray(topTools) || topTools.length === 0) {
        placeholder.style.removeProperty('display');
        placeholder.textContent = (typeof window.t === 'function' ? window.t('dashboard.noCallData') : 'No call data');
        barChartEl.style.display = 'none';
        barChartEl.innerHTML = '';
        return;
    }

    const entries = topTools.map(function (t) {
        return {
            name: t.toolName || '',
            totalCalls: typeof t.totalCalls === 'number' ? t.totalCalls : 0,
        };
    }).filter(function (e) { return e.name && e.totalCalls > 0; })
        .sort(function (a, b) { return b.totalCalls - a.totalCalls; })
        .slice(0, 30);

    if (entries.length === 0) {
        placeholder.style.removeProperty('display');
        placeholder.textContent = (typeof window.t === 'function' ? window.t('dashboard.noCallData') : 'No call data');
        barChartEl.style.display = 'none';
        barChartEl.innerHTML = '';
        return;
    }

    placeholder.style.display = 'none';
    barChartEl.style.display = 'block';

    const maxCalls = Math.max.apply(null, entries.map(function (e) { return e.totalCalls; }));
    var html = '';
    entries.forEach(function (e, i) {
        var pct = maxCalls > 0 ? (e.totalCalls / maxCalls) * 100 : 0;
        var label = e.name.length > 12 ? e.name.slice(0, 10) + '…' : e.name;
        var color = DASHBOARD_BAR_COLORS[i % DASHBOARD_BAR_COLORS.length];
        var fullName = esc(e.name);
        html += '<div class="dashboard-tools-bar-item" data-tooltip="' + fullName + '">';
        html += '<span class="dashboard-tools-bar-label">' + esc(label) + '</span>';
        html += '<div class="dashboard-tools-bar-track"><div class="dashboard-tools-bar-fill" style="width:' + pct + '%;background:' + color + '"></div></div>';
        html += '<span class="dashboard-tools-bar-value">' + e.totalCalls + '</span>';
        html += '</div>';
    });
    barChartEl.innerHTML = html;
    attachDashboardBarTooltips(barChartEl);
}

var dashboardBarTooltipEl = null;
var dashboardBarTooltipTimer = null;

function attachDashboardBarTooltips(barChartEl) {
    if (!barChartEl) return;
    if (!dashboardBarTooltipEl) {
        dashboardBarTooltipEl = document.createElement('div');
        dashboardBarTooltipEl.className = 'dashboard-tools-bar-tooltip';
        dashboardBarTooltipEl.setAttribute('role', 'tooltip');
        document.body.appendChild(dashboardBarTooltipEl);
    }
    barChartEl.removeEventListener('mouseover', dashboardBarTooltipOnOver);
    barChartEl.removeEventListener('mouseout', dashboardBarTooltipOnOut);
    barChartEl.addEventListener('mouseover', dashboardBarTooltipOnOver);
    barChartEl.addEventListener('mouseout', dashboardBarTooltipOnOut);
}

function dashboardBarTooltipOnOver(ev) {
    var item = ev.target && ev.target.closest && ev.target.closest('.dashboard-tools-bar-item');
    if (!item || !dashboardBarTooltipEl) return;
    var text = item.getAttribute('data-tooltip');
    if (!text) return;
    clearTimeout(dashboardBarTooltipTimer);
    dashboardBarTooltipTimer = setTimeout(function () {
        dashboardBarTooltipEl.textContent = text;
        dashboardBarTooltipEl.style.display = 'block';
        requestAnimationFrame(function () {
            var rect = item.getBoundingClientRect();
            var ttRect = dashboardBarTooltipEl.getBoundingClientRect();
            var x = rect.left + (rect.width / 2) - (ttRect.width / 2);
            var y = rect.top - ttRect.height - 6;
            if (y < 8) y = rect.bottom + 6;
            var pad = 8;
            if (x < pad) x = pad;
            if (x + ttRect.width > window.innerWidth - pad) x = window.innerWidth - ttRect.width - pad;
            dashboardBarTooltipEl.style.left = x + 'px';
            dashboardBarTooltipEl.style.top = y + 'px';
        });
    }, 180);
}

function dashboardBarTooltipOnOut(ev) {
    var item = ev.target && ev.target.closest && ev.target.closest('.dashboard-tools-bar-item');
    var related = ev.relatedTarget && ev.relatedTarget.closest && ev.relatedTarget.closest('.dashboard-tools-bar-item');
    if (item && item === related) return;
    clearTimeout(dashboardBarTooltipTimer);
    dashboardBarTooltipTimer = null;
    if (dashboardBarTooltipEl) dashboardBarTooltipEl.style.display = 'none';
}

// Dashboard → vulnerabilities: navigate with severity/status filter
function navigateToVulnerabilitiesWithFilter(opts) {
    opts = opts || {};
    var params = new URLSearchParams();
    if (opts.severity) params.set('severity', opts.severity);
    if (opts.status) params.set('status', opts.status);
    var qs = params.toString();
    window.location.hash = qs ? 'vulnerabilities?' + qs : 'vulnerabilities';
}
window.navigateToVulnerabilitiesWithFilter = navigateToVulnerabilitiesWithFilter;

function normalizeDashboardSeverityStatusFilter(status) {
    status = String(status || '');
    return DASHBOARD_SEVERITY_STATUS_FILTER_VALUES.indexOf(status) >= 0 ? status : '';
}

function readDashboardSeverityStatusFilterFromStorage() {
    try {
        return normalizeDashboardSeverityStatusFilter(window.localStorage.getItem(DASHBOARD_SEVERITY_STATUS_FILTER_STORAGE_KEY));
    } catch (_) {
        return '';
    }
}

function writeDashboardSeverityStatusFilterToStorage(status) {
    try {
        if (status) {
            window.localStorage.setItem(DASHBOARD_SEVERITY_STATUS_FILTER_STORAGE_KEY, status);
        } else {
            window.localStorage.removeItem(DASHBOARD_SEVERITY_STATUS_FILTER_STORAGE_KEY);
        }
    } catch (_) {
        // localStorage may be disabled by browser privacy settings; the filter itself still works on the currentPage.
    }
}

function dashboardSeverityStatusFilterLabel(status) {
    status = normalizeDashboardSeverityStatusFilter(status);
    if (!status) return dt('dashboard.allStatuses', null, 'allstatus');
    return statusShortLabel(status);
}

function getDashboardSeverityStatusFilter() {
    if (dashboardState.severityStatusfilter === null) {
        dashboardState.severityStatusfilter = readDashboardSeverityStatusFilterFromStorage();
    }
    syncDashboardSeverityStatusFilterUI();
    return dashboardState.severityStatusfilter || '';
}

function updateDashboardSeverityStatusFilter(status) {
    dashboardState.severityStatusfilter = normalizeDashboardSeverityStatusFilter(status);
    writeDashboardSeverityStatusFilterToStorage(dashboardState.severityStatusfilter);
    syncDashboardSeverityStatusFilterUI();
    refreshDashboard();
}
window.updateDashboardSeverityStatusFilter = updateDashboardSeverityStatusFilter;

function selectDashboardSeverityStatusFilter(status, ev) {
    if (ev) {
        ev.preventDefault();
        ev.stopPropagation();
    }
    closeDashboardSeverityStatusFilterMenu();
    updateDashboardSeverityStatusFilter(status);
}
window.selectDashboardSeverityStatusFilter = selectDashboardSeverityStatusFilter;

function syncDashboardSeverityStatusFilterUI() {
    var status = dashboardState.severityStatusfilter;
    if (status === null) status = readDashboardSeverityStatusFilterFromStorage();
    status = normalizeDashboardSeverityStatusFilter(status);

    var root = document.getElementById('dashboard-severity-status-filter');
    var textEl = document.getElementById('dashboard-severity-status-filter-text');
    if (root) root.setAttribute('data-value', status);
    if (textEl) textEl.textContent = dashboardSeverityStatusFilterLabel(status);

    document.querySelectorAll('.dashboard-severity-status-filter-option[data-status]').forEach(function (item) {
        var active = item.getAttribute('data-status') === status;
        item.classList.toggle('is-active', active);
        item.setAttribute('aria-selected', active ? 'true' : 'false');
    });
}

function toggleDashboardSeverityStatusFilterMenu(ev) {
    if (ev) {
        ev.preventDefault();
        ev.stopPropagation();
    }
    syncDashboardSeverityStatusFilterUI();
    var root = document.getElementById('dashboard-severity-status-filter');
    var btn = document.getElementById('dashboard-severity-status-filter-btn');
    var menu = document.getElementById('dashboard-severity-status-filter-menu');
    if (!root || !btn || !menu) return;
    var willOpen = menu.hasAttribute('hidden');
    menu.toggleAttribute('hidden', !willOpen);
    root.classList.toggle('is-open', willOpen);
    btn.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
    ensureDashboardSeverityStatusFilterOutsideListener();
}
window.toggleDashboardSeverityStatusFilterMenu = toggleDashboardSeverityStatusFilterMenu;

function closeDashboardSeverityStatusFilterMenu() {
    var root = document.getElementById('dashboard-severity-status-filter');
    var btn = document.getElementById('dashboard-severity-status-filter-btn');
    var menu = document.getElementById('dashboard-severity-status-filter-menu');
    if (menu) menu.setAttribute('hidden', '');
    if (root) root.classList.remove('is-open');
    if (btn) btn.setAttribute('aria-expanded', 'false');
}

function ensureDashboardSeverityStatusFilterOutsideListener() {
    if (dashboardState.severityStatusfilterOutsideBound) return;
    dashboardState.severityStatusfilterOutsideBound = true;
    document.addEventListener('click', function (ev) {
        var root = document.getElementById('dashboard-severity-status-filter');
        if (root && root.contains(ev.target)) return;
        closeDashboardSeverityStatusFilterMenu();
    });
    document.addEventListener('keydown', function (ev) {
        if (ev.key === 'Escape') closeDashboardSeverityStatusFilterMenu();
    });
}

function openDashboardSeverityVulnerabilities() {
    navigateToVulnerabilitiesWithFilter({ status: getDashboardSeverityStatusFilter() });
}
window.openDashboardSeverityVulnerabilities = openDashboardSeverityVulnerabilities;

function navigateToSeverityWithDashboardStatus(severity) {
    navigateToVulnerabilitiesWithFilter({
        severity: severity,
        status: getDashboardSeverityStatusFilter()
    });
}

function renderDashboardSeveritySummary(bySeverity, total, severityIds) {
    severityIds = Array.isArray(severityIds) && severityIds.length ? severityIds : ['critical', 'high', 'medium', 'low', 'info'];
    total = Number(total || 0);
    bySeverity = bySeverity && typeof bySeverity === 'object' ? bySeverity : {};
    severityIds.forEach(function (sev) {
        var count = Number(bySeverity[sev] || 0) || 0;
        var el = document.getElementById('dashboard-severity-' + sev);
        if (el) el.textContent = String(count);
        var pctEl = document.getElementById('dashboard-severity-' + sev + '-pct');
        if (pctEl) {
            var pct = total > 0 ? Math.round((count / total) * 100) : 0;
            pctEl.textContent = pct + '%';
        }
    });
    renderSeverityDonut(bySeverity, total);
}

// Vulnerability severity distribution: half-ring (donut) rendering
// Geometry parameters are fixed to pair with the SVG viewBox 0 0 560 320
// Segment gaps are achieved via gapRad geometric spacing, not strokes, to avoid heavy white/black borders in light/dark themes
var SEVERITY_DONUT_CFG = {
    // viewBox 0 0 480 260: overall compact, but ring thickness brought back near the "golden ratio",
    // giving the arc band visual weight without consuming as much space as the earliest version did.
    // Principle: rInner / rOuter ≈ 0.70, ring thickness ≈ rOuter * 0.30.
    cx: 240,
    cy: 215,
    rOuter: 165,
    rInner: 115,    // ring thickness = 50 (between the original 90 and the previous 35, natural and substantial)
    labelOffset: 14,
    gapRad: 0.022
};

// Three-stop gradient: [highlight light tone, mid saturated colour, dark edge] — creates a 3D glazed-surface layered look
var SEVERITY_DONUT_GRADIENTS = {
    critical: ['#fecaca', '#f87171', '#dc2626'],
    high: ['#fed7aa', '#fb923c', '#ea580c'],
    medium: ['#fef08a', '#facc15', '#ca8a04'],
    low: ['#99f6e4', '#2dd4bf', '#0f766e'],
    info: ['#bfdbfe', '#60a5fa', '#2563eb']
};

var severityDonutCenterDisplayed = { total: null, hoverCount: null };

var severityDonutState = {
    bySeverity: {},
    total: 0,
    hoverId: null,
    bound: false
};

var severityDonutTooltipEl = null;
var severityDonutTooltipTimer = null;
var severityDonutHoverclearTimer = null;

var SEVERITY_DEFAULT_LABELS = {
    critical: 'Critical',
    high: 'High',
    medium: 'Medium',
    low: 'Low',
    info: ' info'
};

function severityLabel(id) {
    var key = 'dashboard.severity' + id.charAt(0).toUpperCase() + id.slice(1);
    if (typeof window.t === 'function') {
        var v = window.t(key);
        if (v && v !== key) return v;
    }
    return SEVERITY_DEFAULT_LABELS[id] || id;
}

function isDashboardDarkTheme() {
    return document.documentElement.getAttribute('data-theme') === 'dark';
}

function ensureSeverityDonutThemeObserver() {
    if (severityDonutState.themeObserver) return;
    severityDonutState.themeObserver = new MutationObserver(function (mutations) {
        for (var i = 0; i < mutations.length; i++) {
            if (mutations[i].attributeName === 'data-theme') {
                renderSeverityDonut(severityDonutState.bySeverity, severityDonutState.total);
                break;
            }
        }
    });
    severityDonutState.themeObserver.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ['data-theme']
    });
}

function ensureSeverityDonutDefs() {
    var defsEl = document.getElementById('dashboard-severity-donut-defs');
    if (!defsEl) return;
    var dark = isDashboardDarkTheme();
    var html = '';
    html += '<linearGradient id="donut-track-face" x1="0%" y1="0%" x2="0%" y2="100%">';
    if (dark) {
        html += '<stop offset="0%" stop-color="#334155"/>';
        html += '<stop offset="55%" stop-color="#1e293b"/>';
        html += '<stop offset="100%" stop-color="#172033"/>';
    } else {
        html += '<stop offset="0%" stop-color="#f8fafc"/>';
        html += '<stop offset="55%" stop-color="#e8eef5"/>';
        html += '<stop offset="100%" stop-color="#dce5ef"/>';
    }
    html += '</linearGradient>';
    html += '<radialGradient id="donut-track-vignette" cx="50%" cy="85%" r="75%" fx="50%" fy="85%">';
    if (dark) {
        html += '<stop offset="0%" stop-color="#0f172a" stop-opacity="0.55"/>';
        html += '<stop offset="70%" stop-color="#0f172a" stop-opacity="0"/>';
    } else {
        html += '<stop offset="0%" stop-color="#ffffff" stop-opacity="0.35"/>';
        html += '<stop offset="70%" stop-color="#ffffff" stop-opacity="0"/>';
    }
    html += '</radialGradient>';
    html += '<radialGradient id="donut-inner-gloss" cx="35%" cy="75%" r="55%">';
    if (dark) {
        html += '<stop offset="0%" stop-color="#94a3b8" stop-opacity="0.10"/>';
        html += '<stop offset="55%" stop-color="#94a3b8" stop-opacity="0.03"/>';
        html += '<stop offset="100%" stop-color="#94a3b8" stop-opacity="0"/>';
    } else {
        html += '<stop offset="0%" stop-color="#ffffff" stop-opacity="0.45"/>';
        html += '<stop offset="55%" stop-color="#ffffff" stop-opacity="0.08"/>';
        html += '<stop offset="100%" stop-color="#ffffff" stop-opacity="0"/>';
    }
    html += '</radialGradient>';
    html += '<filter id="donut-segment-soften" x="-18%" y="-18%" width="136%" height="136%" color-interpolation-filters="sRGB">';
    html += '<feGaussianBlur in="SourceAlpha" stdDeviation="0.8" result="blur"/>';
    html += '<feOffset dx="0" dy="1.5" in="blur" result="off"/>';
    html += '<feFlood flood-color="' + (dark ? '#000000' : '#0f172a') + '" flood-opacity="' + (dark ? '0.28' : '0.13') + '" result="flood"/>';
    html += '<feComposite in="flood" in2="off" operator="in" result="shadow"/>';
    html += '<feMerge><feMergeNode in="shadow"/><feMergeNode in="SourceGraphic"/></feMerge>';
    html += '</filter>';
    Object.keys(SEVERITY_DONUT_GRADIENTS).forEach(function (id) {
        var stops = SEVERITY_DONUT_GRADIENTS[id];
        html += '<linearGradient id="donut-grad-' + id + '" x1="18%" y1="12%" x2="88%" y2="94%">';
        html += '<stop offset="0%" stop-color="' + stops[0] + '"/>';
        html += '<stop offset="52%" stop-color="' + stops[1] + '"/>';
        html += '<stop offset="100%" stop-color="' + stops[2] + '"/>';
        html += '</linearGradient>';
    });
    defsEl.innerHTML = html;
}

function renderSeverityDonut(bySeverity, total) {
    var svgEl = document.getElementById('dashboard-severity-donut');
    var trackEl = document.getElementById('dashboard-severity-donut-track');
    var leadersEl = document.getElementById('dashboard-severity-donut-leaders');
    var segmentsEl = document.getElementById('dashboard-severity-donut-segments');
    var hitsEl = document.getElementById('dashboard-severity-donut-hits');
    var labelsEl = document.getElementById('dashboard-severity-donut-labels');
    if (!trackEl || !segmentsEl || !labelsEl) return;

    severityDonutState.bySeverity = bySeverity && typeof bySeverity === 'object' ? bySeverity : {};
    severityDonutState.total = total || 0;
    severityDonutState.hoverId = null;

    ensureSeverityDonutThemeObserver();

    var cfg = SEVERITY_DONUT_CFG;
    ensureSeverityDonutDefs();

    // Background track (full half-ring): two-layer fill creates groove + highlight
    var trackPath = halfRingPath(cfg.cx, cfg.cy, cfg.rOuter, cfg.rInner);
    trackEl.innerHTML =
        '<path class="donut-track-shadow" d="' + trackPath + '"/>' +
        '<path class="donut-track" fill="url(#donut-track-face)" d="' + trackPath + '"/>' +
        '<path class="donut-track-vignette" fill="url(#donut-track-vignette)" d="' + trackPath + '"/>';

    var ids = ['critical', 'high', 'medium', 'low', 'info'];
    var severities = ids.map(function (id) {
        return { id: id, value: (bySeverity && typeof bySeverity[id] === 'number') ? bySeverity[id] : 0 };
    });
    var visible = severities.filter(function (s) { return s.value > 0; });

    if (svgEl) {
        svgEl.classList.remove('is-highlighting');
        svgEl.removeAttribute('data-hover-severity');
    }
    if (!total || total <= 0 || visible.length === 0) {
        segmentsEl.innerHTML = '';
        if (hitsEl) hitsEl.innerHTML = '';
        labelsEl.innerHTML = '';
        if (leadersEl) leadersEl.innerHTML = '';
        clearSeverityDonutLegendHighlight();
        resetSeverityDonutCenter(false);
        _clearSeverityDonutChartWrapHover();
        if (svgEl) svgEl.classList.remove('donut-ready');
        return;
    }

    resetSeverityDonutCenter(true);

    // Arc length is calculated by value/total; if the severity sum < total (some unclassified), the background track gap remains on the right
    var sumVisible = visible.reduce(function (s, seg) { return s + seg.value; }, 0);
    var coverage = sumVisible / total; // proportion of the half-ring covered by actual segments
    var visibleCount = visible.length;
    var totalGapRad = cfg.gapRad * Math.max(0, visibleCount - 1);
    // Total arc available in the half-ring = π * coverage (proportionally filled), minus segment gaps
    var arcsTotalRad = Math.max(0, Math.PI * coverage - totalGapRad);

    var segmentsHtml = '';
    var hitsHtml = '';
    var glossHtml = '';
    var labelsHtml = '';
    var leadersHtml = '';
    var cumRad = 0;

    visible.forEach(function (seg, i) {
        var arcFraction = seg.value / sumVisible;
        var segRad = arcsTotalRad * arcFraction;
        var angleStart = Math.PI - cumRad;
        var angleEnd = angleStart - segRad;

        var path = arcSegmentPath(cfg.cx, cfg.cy, cfg.rOuter, cfg.rInner, angleStart, angleEnd);
        var pctOfTotal = (seg.value / total) * 100;
        var pctRounded = Math.round(pctOfTotal);
        var name = esc(severityLabel(seg.id));
        var ariaLabel = name + ' ' + seg.value + ' (' + pctRounded + '%)';
        segmentsHtml += '<path class="donut-segment seg-' + seg.id + '" data-severity="' + seg.id + '" data-count="' + seg.value + '" data-pct="' + pctRounded + '" fill="url(#donut-grad-' + seg.id + ')" d="' + path + '"/>';
        hitsHtml += '<path class="donut-segment-hit seg-' + seg.id + '" data-severity="' + seg.id + '" fill="transparent" d="' + path + '" tabIndex="0" role="button" aria-label="' + ariaLabel + '"/>';
        glossHtml += '<path class="donut-segment-gloss seg-' + seg.id + '" data-severity="' + seg.id + '" fill="url(#donut-inner-gloss)" d="' + arcSegmentPath(cfg.cx, cfg.cy, cfg.rOuter - 2, cfg.rInner + 6, angleStart, angleEnd) + '" pointer-events="none"/>';

        // Only show external labels when the segment is >= 5% to avoid small-segment label overlap
        if (pctOfTotal >= 5) {
            var midAngle = (angleStart + angleEnd) / 2;
            var labelR = cfg.rOuter + cfg.labelOffset + 6;
            var sinMid = Math.sin(midAngle);
            var cosMid = Math.cos(midAngle);
            var lx = cfg.cx + labelR * cosMid;
            var topLift = sinMid > 0.4 ? Math.round((sinMid - 0.3) * 10) : 0;
            var ly = cfg.cy - labelR * sinMid - topLift;

            var anchor = 'middle';
            if (cosMid < -0.15) anchor = 'end';
            else if (cosMid > 0.15) anchor = 'start';

            var pctText = pctRounded + '%';
            var arcR = cfg.rOuter + 4;
            var lineX1 = cfg.cx + arcR * cosMid;
            var lineY1 = cfg.cy - arcR * sinMid;
            var lineX2 = cfg.cx + (cfg.rOuter + cfg.labelOffset - 2) * cosMid;
            var lineY2 = cfg.cy - (cfg.rOuter + cfg.labelOffset - 2) * sinMid;
            leadersHtml += '<line class="donut-leader label-' + seg.id + '" data-severity="' + seg.id + '" pathLength="100" x1="' + lineX1.toFixed(1) + '" y1="' + lineY1.toFixed(1) + '" x2="' + lineX2.toFixed(1) + '" y2="' + lineY2.toFixed(1) + '"/>';

            labelsHtml += '<text class="donut-label-text label-' + seg.id + '" data-severity="' + seg.id + '" text-anchor="' + anchor + '" x="' + lx.toFixed(1) + '" y="' + ly.toFixed(1) + '">';
            labelsHtml += '<tspan x="' + lx.toFixed(1) + '" dy="0">' + seg.value + ' <tspan class="donut-label-pct">(' + pctText + ')</tspan></tspan>';
            labelsHtml += '<tspan class="donut-label-name" x="' + lx.toFixed(1) + '" dy="14">' + name + '</tspan>';
            labelsHtml += '</text>';
        }

        cumRad += segRad;
        if (i < visibleCount - 1) cumRad += cfg.gapRad;
    });

    if (leadersEl) leadersEl.innerHTML = leadersHtml;
    segmentsEl.innerHTML = segmentsHtml + glossHtml;
    if (hitsEl) hitsEl.innerHTML = hitsHtml;
    labelsEl.innerHTML = labelsHtml;
    if (svgEl) {
        svgEl.classList.remove('donut-ready');
        void svgEl.offsetWidth;
        requestAnimationFrame(function () {
            svgEl.classList.add('donut-ready');
        });
    }
    scheduleSeverityCenterCountUp(total);
    attachSeverityDonutInteractivity();
}

function scheduleSeverityCenterCountUp(targetTotal) {
    if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        var totalEl = document.getElementById('dashboard-severity-total');
        if (totalEl) totalEl.textContent = String(targetTotal);
        severityDonutCenterDisplayed.total = targetTotal;
        return;
    }
    var totalEl = document.getElementById('dashboard-severity-total');
    if (!totalEl || severityDonutState.hoverId) return;
    var from = typeof severityDonutCenterDisplayed.total === 'number' ? severityDonutCenterDisplayed.total : 0;
    var to = targetTotal;
    if (from === to) {
        totalEl.textContent = String(to);
        severityDonutCenterDisplayed.total = to;
        return;
    }
    var start = null;
    var dur = Math.min(520, 180 + Math.abs(to - from) * 28);
    function tick(now) {
        if (!start) start = now;
        var t = Math.min(1, (now - start) / dur);
        var eased = 1 - Math.pow(1 - t, 3);
        var val = Math.round(from + (to - from) * eased);
        totalEl.textContent = String(val);
        if (t < 1) {
            requestAnimationFrame(tick);
        } else {
            totalEl.textContent = String(to);
            severityDonutCenterDisplayed.total = to;
        }
    }
    requestAnimationFrame(tick);
}

function resetSeverityDonutCenter(skipTotalSnapshot) {
    var totalEl = document.getElementById('dashboard-severity-total');
    var labelEl = document.getElementById('dashboard-severity-center-label');
    var centerEl = document.getElementById('dashboard-severity-center');
    var n = severityDonutState.total || 0;
    if (!skipTotalSnapshot && totalEl) totalEl.textContent = String(n);
    if (!skipTotalSnapshot) severityDonutCenterDisplayed.total = n;
    severityDonutCenterDisplayed.hoverCount = null;
    if (labelEl) {
        labelEl.textContent = (typeof window.t === 'function' ? window.t('dashboard.totalVulns') : 'Total vulnerabilities');
        labelEl.classList.remove('is-severity');
        labelEl.removeAttribute('data-severity');
    }
    if (centerEl) centerEl.classList.remove('is-hovering');
}

function setSeverityDonutHover(severityId) {
    var svgEl = document.getElementById('dashboard-severity-donut');
    var centerEl = document.getElementById('dashboard-severity-center');
    var totalEl = document.getElementById('dashboard-severity-total');
    var labelEl = document.getElementById('dashboard-severity-center-label');
    if (!severityId) {
        severityDonutState.hoverId = null;
        if (svgEl) {
            svgEl.classList.remove('is-highlighting');
            svgEl.removeAttribute('data-hover-severity');
        }
        clearSeverityDonutLegendHighlight();
        resetSeverityDonutCenter(false);
        _clearSeverityDonutChartWrapHover();
        return;
    }
    var count = (severityDonutState.bySeverity && severityDonutState.bySeverity[severityId]) || 0;
    severityDonutState.hoverId = severityId;
    if (svgEl) {
        svgEl.classList.add('is-highlighting');
        svgEl.setAttribute('data-hover-severity', severityId);
    }
    highlightSeverityDonutParts(severityId);
    highlightSeverityLegendItem(severityId);
    if (totalEl) {
        totalEl.textContent = String(count);
        severityDonutCenterDisplayed.hoverCount = count;
    }
    if (labelEl) {
        labelEl.textContent = severityLabel(severityId);
        labelEl.classList.add('is-severity');
        labelEl.setAttribute('data-severity', severityId);
    }
    if (centerEl) centerEl.classList.add('is-hovering');
    var chartWrap = document.querySelector('.dashboard-severity-chart');
    if (chartWrap) chartWrap.setAttribute('data-hover-severity', severityId);
}

function _clearSeverityDonutChartWrapHover() {
    var chartWrap = document.querySelector('.dashboard-severity-chart');
    if (chartWrap) chartWrap.removeAttribute('data-hover-severity');
}

function highlightSeverityDonutParts(severityId) {
    var svgEl = document.getElementById('dashboard-severity-donut');
    if (!svgEl) return;
    svgEl.querySelectorAll('.donut-segment[data-severity], .donut-segment-gloss[data-severity], .donut-leader[data-severity], .donut-label-text[data-severity]').forEach(function (el) {
        var match = el.getAttribute('data-severity') === severityId;
        el.classList.toggle('is-active', match);
        el.classList.toggle('is-dimmed', !match);
    });
}

function highlightSeverityLegendItem(severityId) {
    var legend = document.getElementById('dashboard-vuln-bars');
    if (!legend) return;
    legend.querySelectorAll('.dashboard-severity-legend-item').forEach(function (item) {
        var match = item.getAttribute('data-severity') === severityId;
        item.classList.toggle('is-active', match);
    });
}

function clearSeverityDonutLegendHighlight() {
    var legend = document.getElementById('dashboard-vuln-bars');
    if (legend) {
        legend.querySelectorAll('.dashboard-severity-legend-item.is-active').forEach(function (el) {
            el.classList.remove('is-active');
        });
    }
    var svgEl = document.getElementById('dashboard-severity-donut');
    if (svgEl) {
        svgEl.querySelectorAll('.is-active, .is-dimmed').forEach(function (el) {
            el.classList.remove('is-active', 'is-dimmed');
        });
    }
}

function severityDonutTooltipText(severityId) {
    var count = (severityDonutState.bySeverity && severityDonutState.bySeverity[severityId]) || 0;
    var pct = severityDonutState.total > 0 ? Math.round((count / severityDonutState.total) * 100) : 0;
    var hint = (typeof window.t === 'function' ? window.t('dashboard.severityClickHint') : 'Click to view');
    return severityLabel(severityId) + ' · ' + count + ' (' + pct + '%) — ' + hint;
}

function showSeverityDonutTooltip(ev, severityId) {
    if (!severityDonutTooltipEl) {
        severityDonutTooltipEl = document.createElement('div');
        severityDonutTooltipEl.className = 'dashboard-severity-donut-tooltip';
        severityDonutTooltipEl.setAttribute('role', 'tooltip');
        document.body.appendChild(severityDonutTooltipEl);
    }
    clearTimeout(severityDonutTooltipTimer);
    severityDonutTooltipTimer = setTimeout(function () {
        severityDonutTooltipEl.textContent = severityDonutTooltipText(severityId);
        severityDonutTooltipEl.style.display = 'block';
        requestAnimationFrame(function () {
            var x = ev.clientX;
            var y = ev.clientY;
            var ttRect = severityDonutTooltipEl.getBoundingClientRect();
            var left = x - ttRect.width / 2;
            var top = y - ttRect.height - 12;
            if (top < 8) top = y + 16;
            var pad = 8;
            if (left < pad) left = pad;
            if (left + ttRect.width > window.innerWidth - pad) left = window.innerWidth - ttRect.width - pad;
            severityDonutTooltipEl.style.left = left + 'px';
            severityDonutTooltipEl.style.top = top + 'px';
        });
    }, 120);
}

function hideSeverityDonutTooltip() {
    clearTimeout(severityDonutTooltipTimer);
    severityDonutTooltipTimer = null;
    if (severityDonutTooltipEl) severityDonutTooltipEl.style.display = 'none';
}

function attachSeverityDonutInteractivity() {
    var hitsEl = document.getElementById('dashboard-severity-donut-hits');
    var legend = document.getElementById('dashboard-vuln-bars');
    if (!hitsEl) return;

    if (!severityDonutState.bound) {
        severityDonutState.bound = true;
        hitsEl.addEventListener('mouseover', severityDonutPointerOver);
        hitsEl.addEventListener('mouseout', severityDonutPointerOut);
        hitsEl.addEventListener('click', severityDonutClick);
        hitsEl.addEventListener('keydown', severityDonutKeydown);
        if (legend) {
            legend.addEventListener('mouseover', severityLegendPointerOver);
            legend.addEventListener('mouseout', severityLegendPointerOut);
            legend.addEventListener('click', severityLegendClick);
            legend.addEventListener('keydown', severityLegendKeydown);
        }
    }

    legend && legend.querySelectorAll('.dashboard-severity-legend-item').forEach(function (item) {
        if (!item.getAttribute('data-severity')) return;
        var sev = item.getAttribute('data-severity');
        var count = (severityDonutState.bySeverity && severityDonutState.bySeverity[sev]) || 0;
        item.classList.toggle('is-zero', count === 0);
        item.setAttribute('aria-label', severityDonutTooltipText(sev));
    });
}

function severityDonutHitTarget(el) {
    return el && el.closest && el.closest('.donut-segment-hit');
}

function severityDonutCancelHoverClear() {
    clearTimeout(severityDonutHoverclearTimer);
    severityDonutHoverclearTimer = null;
}

function severityDonutScheduleHoverClear() {
    severityDonutCancelHoverClear();
    severityDonutHoverclearTimer = setTimeout(function () {
        severityDonutHoverclearTimer = null;
        setSeverityDonutHover(null);
        hideSeverityDonutTooltip();
    }, 60);
}

function severityDonutPointerOver(ev) {
    var target = severityDonutHitTarget(ev.target);
    if (!target) return;
    var id = target.getAttribute('data-severity');
    if (!id) return;
    severityDonutCancelHoverClear();
    if (severityDonutState.hoverId === id) return;
    setSeverityDonutHover(id);
    showSeverityDonutTooltip(ev, id);
}

function severityDonutPointerOut(ev) {
    var related = ev.relatedTarget;
    if (related) {
        if (severityDonutHitTarget(related)) return;
        var legendItem = related.closest && related.closest('.dashboard-severity-legend-item[data-severity]');
        if (legendItem) return;
        var hitsRoot = document.getElementById('dashboard-severity-donut-hits');
        if (hitsRoot && hitsRoot.contains(related)) return;
    }
    severityDonutScheduleHoverClear();
}

function severityDonutClick(ev) {
    var target = severityDonutHitTarget(ev.target);
    if (!target) return;
    var id = target.getAttribute('data-severity');
    if (!id) return;
    ev.preventDefault();
    navigateToSeverityWithDashboardStatus(id);
}

function severityDonutKeydown(ev) {
    if (ev.key !== 'Enter' && ev.key !== ' ') return;
    var target = severityDonutHitTarget(ev.target);
    if (!target) return;
    ev.preventDefault();
    var id = target.getAttribute('data-severity');
    if (id) navigateToSeverityWithDashboardStatus(id);
}

function severityLegendPointerOver(ev) {
    var item = ev.target && ev.target.closest && ev.target.closest('.dashboard-severity-legend-item[data-severity]');
    if (!item) return;
    var id = item.getAttribute('data-severity');
    if (!id) return;
    severityDonutCancelHoverClear();
    setSeverityDonutHover(id);
    showSeverityDonutTooltip(ev, id);
}

function severityLegendPointerOut(ev) {
    var item = ev.target && ev.target.closest && ev.target.closest('.dashboard-severity-legend-item[data-severity]');
    var related = ev.relatedTarget && ev.relatedTarget.closest && ev.relatedTarget.closest('.dashboard-severity-legend-item[data-severity]');
    if (item && item === related) return;
    severityDonutScheduleHoverClear();
}

function severityLegendClick(ev) {
    var item = ev.target && ev.target.closest && ev.target.closest('.dashboard-severity-legend-item[data-severity]');
    if (!item) return;
    var id = item.getAttribute('data-severity');
    if (!id) return;
    ev.preventDefault();
    navigateToSeverityWithDashboardStatus(id);
}

function severityLegendKeydown(ev) {
    if (ev.key !== 'Enter' && ev.key !== ' ') return;
    var item = ev.target && ev.target.closest && ev.target.closest('.dashboard-severity-legend-item[data-severity]');
    if (!item) return;
    ev.preventDefault();
    var id = item.getAttribute('data-severity');
    if (id) navigateToSeverityWithDashboardStatus(id);
}

// SVG half-ring (background track) path
function halfRingPath(cx, cy, rOuter, rInner) {
    var x1Outer = cx - rOuter;
    var y1Outer = cy;
    var x2Outer = cx + rOuter;
    var y2Outer = cy;
    var x1Inner = cx - rInner;
    var y1Inner = cy;
    var x2Inner = cx + rInner;
    var y2Inner = cy;
    return 'M ' + x1Outer + ' ' + y1Outer +
        ' A ' + rOuter + ' ' + rOuter + ' 0 0 1 ' + x2Outer + ' ' + y2Outer +
        ' L ' + x2Inner + ' ' + y2Inner +
        ' A ' + rInner + ' ' + rInner + ' 0 0 0 ' + x1Inner + ' ' + y1Inner + ' Z';
}

// Single arc segment (angleStart > angleEnd, angle decreases counter-clockwise, visually advances clockwise from the top of the half-ring)
function arcSegmentPath(cx, cy, rOuter, rInner, angleStart, angleEnd) {
    var x1Outer = cx + rOuter * Math.cos(angleStart);
    var y1Outer = cy - rOuter * Math.sin(angleStart);
    var x2Outer = cx + rOuter * Math.cos(angleEnd);
    var y2Outer = cy - rOuter * Math.sin(angleEnd);
    var x1Inner = cx + rInner * Math.cos(angleStart);
    var y1Inner = cy - rInner * Math.sin(angleStart);
    var x2Inner = cx + rInner * Math.cos(angleEnd);
    var y2Inner = cy - rInner * Math.sin(angleEnd);

    var largeArc = (angleStart - angleEnd) > Math.PI ? 1 : 0;

    return 'M ' + x1Outer.toFixed(2) + ' ' + y1Outer.toFixed(2) +
        ' A ' + rOuter + ' ' + rOuter + ' 0 ' + largeArc + ' 1 ' + x2Outer.toFixed(2) + ' ' + y2Outer.toFixed(2) +
        ' L ' + x2Inner.toFixed(2) + ' ' + y2Inner.toFixed(2) +
        ' A ' + rInner + ' ' + rInner + ' 0 ' + largeArc + ' 0 ' + x1Inner.toFixed(2) + ' ' + y1Inner.toFixed(2) + ' Z';
}

// After a language switch, JS-dynamically-rendered Dashboard parts (KPI sub-text, alert banner, donut labels,
// status cards, recent vulnerability list, capability overview badges, etc.) are not automatically redrawn by applyTranslations;
// they need an explicit data re-fetch and re-render in the new language — consistent with tasks/vulnerability and other pages.
document.addEventListener('languagechange', function () {
    try {
        var dashboardPage = document.getElementById('page-dashboard');
        if (!dashboardPage || !dashboardPage.classList.contains('active')) {
            return;
        }
        if (typeof refreshDashboard === 'function') {
            refreshDashboard();
        }
    } catch (e) {
        console.warn('languagechange dashboard refresh failed', e);
    }
});

// Page visibility: when switching back from another tab, immediately refresh if more than half a polling interval has elapsed;
// prevents stale data after long background stays without hammering the API every time the user switches back.
document.addEventListener('visibilitychange', function () {
    if (document.hidden) return;
    var  page = document.getElementById('page-dashboard');
    if (! page || ! page.classList.contains('active')) return;
    var ageMs = Date.now() - (dashboardState.lastupdatedAt || 0);
    if (ageMs >= DASHBOARD_POLL_INTERVAL_MS / 2) {
        try { refreshDashboard(); } catch (_) { /* ignore */ }
    } else {
        // No need to re-fetch, but run a stale check to update badge status
        checkDashboardStale();
    }
});

// Dismiss alert banner button: stores the currentReasons fingerprint in sessionStorage so the same content is not shown again in this session
document.addEventListener('click', function (ev) {
    var btn = ev.target && ev.target.closest && ev.target.closest('#dashboard-alert-close');
    if (!btn) return;
    ev.preventDefault();
    var key = dashboardState.dismissedAlertKey || '';
    try { sessionStorage.setItem(DASH_SESSION_ALERT_DISMISSED, key); } catch (_) {}
    var banner = document.getElementById('dashboard-alert-banner');
    if (banner) banner.hidden = true;
});
