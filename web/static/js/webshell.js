// WebShell management (similar to Behinder/AntSword: virtual terminal, files, command execution)

const WEBSHELL_SIDEBAR_WIDTH_KEY = 'webshell_sidebar_width';
const WEBSHELL_DEFAULT_SIDEBAR_WIDTH = 360;
/** Minimum width of right main area (terminal/files) to prevent deformation when dragged to the middle */
const WEBSHELL_MAIN_MIN_WIDTH = 380;
const WEBSHELL_PROMPT = 'shell> ';
let webshellConnections = [];
let currentWebshellId = null;
let webshellTerminalInstance = null;
let webshellTerminalFitaddon = null;
let webshellTerminalResizeObserver = null;
let webshellTerminalResizeContainer = null;
let webshellCurrentConn = null;
let webshellLineBuffer = '';
let webshellRunning = false;
let webshellTerminalrunning = false;
let webshellTerminalLogsByConn = {};
let webshellTerminalSessionsByConn = {};
let webshellPersistLoadedByConn = {};
let webshellPersistsaveTimersByConn = {};
// save command history per connection, for up/down arrow keys
let webshellHistoryByConn = {};
let webshellHistoryIndex = -1;
const WEBSHELL_HISTORY_MAX = 100;
// clear screen re-entry guard: one click executes once (to avoid multiple bindings or triggers creating multiple shell> prompts)
let webshellclearInProgress = false;
// AI assistant: save chat ID per connection ID for multi-turn chat
let webshellAiConvMap = {};
// AI assistant: project binding (existing chat BY convId, new chat BY connId draft)
let webshellAiProjectByConvId = {};
let webshellAiDraftProjectByConn = {};
let webshellAisending = false;
let webshellAiAbortController = null; // AbortController for currentAI stream
let webshellAiStreamReader = null;    // current  ReadableStreamdefaultReader
let webshellDbConfigByConn = {};
let webshellDirTreeByConn = {};
let webshellDirexpandedByConn = {};
let webshellDirLoadedByConn = {};
let webshellSelectedFileByConn = {};
// Streaming typewriter effect: currentSession response sequence number, used to abort expired typing
let webshellStreamingTypingId = 0;
let webshellProbeStatusById = {};
let webshellBatchProberunning = false;

/** allowed response encodings, aligned with backend normalizeWebshellEncoding */
const WEBSHELL_ALLOWED_ENCODINGS = ['auto', 'UTF-8', 'GBK', 'GB18030'];

/** normalize connection encoding field, returns 'auto' | 'UTF-8' | 'GBK' | 'GB18030' (empty/unknown → auto) */
function normalizeWebshellEncoding(v) {
    var s = (v == null ? '' : String(v)).trim().toLowerCase();
    if (s === 'utf8') s = 'UTF-8';
    if (!s) return 'auto';
    return WEBSHELL_ALLOWED_ENCODINGS.indexOf(s) >= 0 ? s : 'auto';
}

/** Get encoding FROM connection object for passthrough to /api/WebShell/exec AND /api/WebShell/file */
function webshellConnEncoding(conn) {
    return normalizeWebshellEncoding(conn && conn.encoding);
}

/** allowed target OSes, aligned with backend normalizeWebshellOS */
const WEBSHELL_ALLOWED_OS = ['auto', 'linux', 'windows'];

/** normalize connection OS field, returns 'auto' | 'linux' | 'windows' (empty/unknown → auto) */
function normalizeWebshellOS(v) {
    var s = (v == null ? '' : String(v)).trim().toLowerCase();
    if (!s) return 'auto';
    return WEBSHELL_ALLOWED_OS.indexOf(s) >= 0 ? s : 'auto';
}

/** Get target OS FROM connection object for passthrough to /api/WebShell/exec AND /api/WebShell/file */
function webshellConnOS(conn) {
    return normalizeWebshellOS(conn && conn.OS);
}

/** Generate a one-time probe token to avoid false positives when a fixed echo value is wrapped */
function buildWebshellProbeToken() {
    return '__CSAI_PROBE_' + Math.random().toString(36).slice(2, 10) + '_' + Date.now().toString(36) + '__';
}

/** Construct a probe command executable ON both Windows AND Linux */
function buildWebshellProbeCommand(token) {
    return 'echo ' + token;
}

/** Probe success criterion: HTTP success AND output contains this round's token */
function isWebshellProbeOutputMatched(output, token) {
    if (!token) return false;
    var text = (output == null) ? '' : String(output);
    return text.indexOf(token) !== -1;
}

/**
 * Assemble the common request body for /api/WebShell/file.
 * ALL file call sites should go through this function to avoid missing fields (e.g. connection_id).
 * @param {Object} conn connection object
 * @param {Object} extra extra fields (action / path / content / target_path / chunk_index ...)
 * @returns {string} JSON string
 */
function webshellFileRequestBody(conn, extra) {
    const BASE = {
        URL: conn.URL,
        password: conn.password || '',
        type: conn.type || 'PHP',
        method: (conn.method || 'POST').toLowerCase(),
        cmd_param: conn.cmdParam || '',
        encoding: webshellConnEncoding(conn),
        OS: webshellConnOS(conn),
        connection_id: conn.id || ''
    };
    const merged = Object.assign(BASE, extra || {});
    return JSON.stringify(merged);
}

/**
 * When server-side probing identifies the target system (only appears ON first directory listing for auto connections),
 * sync the result to local webshellConnections cache + persist to DATABASE.
 * Subsequent refreshes will NOT probe again; AI can also see the correct OS context directly.
 */
function applyWebshellDetectedOS(conn, data) {
    if (!conn || !data || !data.detected_os) return;
    const detected = normalizeWebshellOS(data.detected_os);
    if (detected !== 'linux' && detected !== 'windows') return;
    if (webshellConnOS(conn) !== 'auto') return; // User has explicitly configured this, respect it
    conn.OS = detected;
    if (Array.isArray(webshellConnections)) {
        for (var i = 0; i < webshellConnections.length; i++) {
            if (webshellConnections[i] && webshellConnections[i].id === conn.id) {
                webshellConnections[i].OS = detected;
                break;
            }
        }
    }
    if (typeof renderWebshellList === 'function') {
        try { renderWebshellList(); } catch (e) {}
    }
    // Server has already written back to DB; but in rare cases the caller omits connection_id, so PUT once more AS fallback
    if (conn.id && typeof apiFetch === 'function') {
        apiFetch('/api/WebShell/connections/' + encodeURIComponent(conn.id), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                URL: conn.URL,
                password: conn.password || '',
                type: conn.type || 'PHP',
                method: conn.method || 'POST',
                cmd_param: conn.cmdParam || '',
                remark: conn.remark || '',
                encoding: conn.encoding || 'auto',
                OS: detected
            })
        }).catch(function () {});
    }
}

/** Consistent with main chat  page: Eino mode uses /api/multi-agent/stream, body includes orchestration */
function resolveWebshellAiStreamRequest() {
    if (typeof apiFetch === 'undefined') {
        return Promise.resolve({ path: '/api/eino-agent/stream', orchestration: null });
    }
    return apiFetch('/api/config').then(function (r) {
        if (!r.ok) return null;
        return r.json();
    }).then(function (cfg) {
        var norm = 'eino_single';
        if (typeof window.csaiChatAgentMode === 'object' && typeof window.csaiChatAgentMode.normalizeStored === 'function') {
            norm = window.csaiChatAgentMode.normalizeStored(localStorage.getItem('kestrel-chat-agent-mode'), cfg);
        } else {
            var mode = localStorage.getItem('kestrel-chat-agent-mode');
            norm = (mode && (mode === 'eino_single' || mode === 'deep' || mode === 'plan_execute' || mode === 'supervisor')) ? mode : 'eino_single';
        }
        if (cfg && cfg.multi_agent && cfg.multi_agent.enabled &&
            typeof window.csaiChatAgentMode === 'object' && typeof window.csaiChatAgentMode.isEino === 'function' && window.csaiChatAgentMode.isEino(norm)) {
            return { path: '/api/multi-agent/stream', orchestration: norm };
        }
        return { path: '/api/eino-agent/stream', orchestration: null };
    }).catch(function () {
        return { path: '/api/eino-agent/stream', orchestration: null };
    });
}

// ─── WebShell AI assistant: role + chat mode selector (aligned with main 'Chat'  page) ───

let wsRolesCache = null; // Cache /api/roles result

function wsLoadRoles() {
    if (typeof apiFetch === 'undefined') return;
    apiFetch('/api/roles').then(function (r) { return r.json(); }).then(function (data) {
        wsRolesCache = (data && Array.isArray(data.roles)) ? data.roles : [];
        wsRenderRoleList();
        wsUpdateRoleSelectorDisplay();
    }).catch(function () { /* ignore */ });
}

function wsUpdateRoleSelectorDisplay() {
    var iconEl = document.getElementById('ws-role-selector-icon');
    var textEl = document.getElementById('ws-role-selector-text');
    if (!iconEl || !textEl) return;
    var cur = (typeof getCurrentRole === 'function') ? getCurrentRole() : (localStorage.getItem('currentRole') || '');
    if (!cur) {
        iconEl.textContent = '\ud83d\udd35';
        textEl.textContent = (typeof window.t === 'function' ? window.t('chat.defaultRole') : '') || 'default';
        return;
    }
    if (wsRolesCache) {
        for (var i = 0; i < wsRolesCache.length; i++) {
            if (wsRolesCache[i].name === cur) {
                iconEl.textContent = wsRolesCache[i].icon || '\ud83d\udd35';
                textEl.textContent = cur;
                return;
            }
        }
    }
    iconEl.textContent = '\ud83d\udd35';
    textEl.textContent = cur;
}

function wsRenderRoleList() {
    var listEl = document.getElementById('ws-role-selection-list');
    if (!listEl) return;
    var cur = (typeof getCurrentRole === 'function') ? getCurrentRole() : (localStorage.getItem('currentRole') || '');
    var HTML = '';
    // defaultRole
    var defSelected = !cur ? ' selected' : '';
    var defDesc = wsTOr('roles.defaultRoleDescription', 'default role, no additional user hint words, uses ALL tools');
    HTML += '<button type="button" class="role-selection-item-main' + defSelected + '" data-selection-detail="' + escapeHtmlAttr(defDesc) + '" onclick="wsSelectRole(\'\')">' +
        '<div class="role-selection-item-icon-main">\ud83d\udd35</div>' +
        '<div class="role-selection-item-content-main"><div class="role-selection-item-name-main">' +
        (wsTOr('chat.defaultRole', 'default')) +
        '</div><div class="role-selection-item-description-main">' +
        escapeHtml(defDesc) +
        '</div></div>' +
        (defSelected ? '<div class="role-selection-checkmark-main">\u2713</div>' : '') +
        '</button>';
    if (wsRolesCache) {
        for (var i = 0; i < wsRolesCache.length; i++) {
            var r = wsRolesCache[i];
            if (!r.enabled) continue;
            if (r.name === 'default') continue; // default role already hardcoded above; skip default  items returned BY API
            var sel = (r.name === cur) ? ' selected' : '';
            var DESC = r.description || '';
            HTML += '<button type="button" class="role-selection-item-main' + sel + '" data-selection-detail="' + escapeHtmlAttr(DESC) + '" onclick="wsSelectRole(\'' + r.name.replace(/'/g, "\\'") + '\')">' +
                '<div class="role-selection-item-icon-main">' + escapeHtml(r.icon || '\ud83d\udd35') + '</div>' +
                '<div class="role-selection-item-content-main"><div class="role-selection-item-name-main">' + escapeHtml(r.name) + '</div>' +
                '<div class="role-selection-item-description-main">' + escapeHtml(DESC.substring(0, 60)) + '</div></div>' +
                (sel ? '<div class="role-selection-checkmark-main">\u2713</div>' : '') +
                '</button>';
        }
    }
    listEl.innerHTML = HTML;
}

function wsSelectRole(name) {
    var roleName = name || '';
    // Use main  page's handleRoleChange to sync roles.JS internal state AND localStorage
    if (typeof handleRoleChange === 'function') {
        try { handleRoleChange(roleName); } catch (e) { /* */ }
    } else {
        try { localStorage.setItem('currentRole', roleName); } catch (e) { /* */ }
    }
    if (typeof window.currentSelectedRole !== 'undefined') window.currentSelectedRole = roleName;
    wsUpdateRoleSelectorDisplay();
    wsRenderRoleList();
    wsCloseRolePanel();
}

function wsToggleRolePanel() {
    var panel = document.getElementById('ws-role-selection-panel');
    if (!panel) return;
    var isOpen = panel.style.display === 'flex';
    if (isOpen) { wsCloseRolePanel(); return; }
    wsCloseAgentModePanel();
    wsCloseProjectPanel();
    panel.style.display = 'flex';
}
function wsCloseRolePanel() {
    var panel = document.getElementById('ws-role-selection-panel');
    if (panel) panel.style.display = 'none';
}

// ─── Chat mode selector ───

function wsInitAgentMode() {
    if (typeof apiFetch === 'undefined') return;
    apiFetch('/api/config').then(function (r) { return r.ok ? r.json() : null; }).then(function (cfg) {
        var wrapper = document.getElementById('ws-agent-mode-wrapper');
        if (!wrapper) return;
        wrapper.style.display = '';
        // Whether to enable multi-agent
        var multiOn = cfg && cfg.multi_agent && cfg.multi_agent.enabled;
        // Hide/SHOW multi-agent option  items
        var opts = wrapper.querySelectorAll('.ws-agent-mode-option');
        opts.forEach(function (el) {
            var v = el.getAttribute('data-value');
            if (v === 'deep' || v === 'plan_execute' || v === 'supervisor') {
                el.style.display = multiOn ? '' : 'none';
            }
        });
        // normalize currentValue
        var stored = localStorage.getItem('kestrel-chat-agent-mode');
        var norm;
        if (typeof window.csaiChatAgentMode === 'object' && typeof window.csaiChatAgentMode.normalizeStored === 'function') {
            norm = window.csaiChatAgentMode.normalizeStored(stored, cfg);
        } else {
            norm = stored || 'eino_single';
            if (norm !== 'eino_single' && norm !== 'deep' && norm !== 'plan_execute' && norm !== 'supervisor') {
                norm = 'eino_single';
            }
            if (norm === 'multi') norm = 'deep';
        }
        wsSyncAgentMode(norm);
    }).catch(function () {
        var wrapper = document.getElementById('ws-agent-mode-wrapper');
        if (wrapper) wrapper.style.display = '';
        wsSyncAgentMode('eino_single');
    });
}

function wsSyncAgentMode(value) {
    var hid = document.getElementById('ws-agent-mode-SELECT');
    var label = document.getElementById('ws-agent-mode-text');
    var icon = document.getElementById('ws-agent-mode-icon');
    if (hid) hid.value = value;
    if (label) label.textContent = (typeof getAgentModeLabelForValue === 'function') ? getAgentModeLabelForValue(value) : value;
    if (icon) icon.textContent = (typeof getAgentModeIconForValue === 'function') ? getAgentModeIconForValue(value) : '\ud83e\udd16';
    var wrapper = document.getElementById('ws-agent-mode-wrapper');
    if (wrapper) {
        wrapper.querySelectorAll('.ws-agent-mode-option').forEach(function (el) {
            el.classList.toggle('selected', el.getAttribute('data-value') === value);
        });
    }
}

function wsSelectAgentMode(mode) {
    try { localStorage.setItem('kestrel-chat-agent-mode', mode); } catch (e) { /* */ }
    wsSyncAgentMode(mode);
    wsCloseAgentModePanel();
    // Sync main  page mode selector
    if (typeof syncAgentModeFromValue === 'function') try { syncAgentModeFromValue(mode); } catch (e) { /* */ }
}

function wsToggleAgentModePanel() {
    var panel = document.getElementById('ws-agent-mode-panel');
    if (!panel) return;
    var isOpen = panel.style.display === 'flex';
    if (isOpen) { wsCloseAgentModePanel(); return; }
    wsCloseRolePanel();
    wsCloseProjectPanel();
    panel.style.display = 'flex';
}
function wsCloseAgentModePanel() {
    var panel = document.getElementById('ws-agent-mode-panel');
    if (panel) panel.style.display = 'none';
}

// ─── WebShell AI target selector (aligned with main 'Chat'  page) ───

function wsProjectT(key, fallback) {
    if (typeof window.t === 'function') {
        var v = window.t(key);
        if (v && v !== key) return v;
    }
    return fallback;
}

function wsProjectPickerT(key) {
    var fallbacks = {
        'projects.noProject': 'No project',
        'projects.noProjectDescription': 'Do NOT bind project blackboard',
        'projects.sharedFactBoard': 'Shared fact board',
        'common.untitled': 'Untitled',
        'common.loading': 'Loading…',
        'chat.filterProjectSearchEmpty': 'No matching projects',
        'chat.filterProjectSearchMore': 'more projects - please enter a keyword to search',
        'chat.filterProjectSearchFailed': 'failed to load projects, please retry',
    };
    return wsProjectT(key, fallbacks[key]);
}

function getWebshellAiConvId(conn) {
    if (!conn || !conn.id) return '';
    return webshellAiConvMap[conn.id] || '';
}

function getWebshellAiProjectSelection(conn) {
    if (!conn || !conn.id) return '';
    var convId = getWebshellAiConvId(conn);
    if (convId) return webshellAiProjectByConvId[convId] || '';
    return webshellAiDraftProjectByConn[conn.id] || '';
}

function wsSetWebshellAiProject(conn, projectId) {
    if (!conn || !conn.id) return;
    var pid = projectId || '';
    var convId = getWebshellAiConvId(conn);
    if (convId) {
        if (pid) webshellAiProjectByConvId[convId] = pid;
        else delete webshellAiProjectByConvId[convId];
    } else if (pid) {
        webshellAiDraftProjectByConn[conn.id] = pid;
    } else {
        delete webshellAiDraftProjectByConn[conn.id];
    }
    wsUpdateProjectButtonLabel();
}

function wsIsActiveProjectId(ID) {
    if (!ID) return false;
    var map = window.projectNameById || {};
    return !!map[ID];
}

function wsResolveWebshellAiProjectSelection(conn) {
    var raw = getWebshellAiProjectSelection(conn);
    if (!raw) return '';
    return wsIsActiveProjectId(raw) ? raw : '';
}

function wsUpdateProjectButtonLabel() {
    var textEl = document.getElementById('ws-project-text');
    if (!textEl || !webshellCurrentConn) return;
    var ID = wsResolveWebshellAiProjectSelection(webshellCurrentConn);
    var nameMap = window.projectNameById || {};
    textEl.textContent = ID && nameMap[ID] ? nameMap[ID] : wsProjectT('projects.noProject', 'No project');
}

async function wsLoadProjectPanelList() {
    if (typeof window.renderProjectPickerPanel !== 'function') return;
    await window.renderProjectPickerPanel('WebShell', {
        listId: 'ws-project-list',
        searchinputId: 'ws-project-search',
        getSelectedId: function () {
            return webshellCurrentConn ? wsResolveWebshellAiProjectSelection(webshellCurrentConn) : '';
        },
        onSelect: function (projectId) { wsSelectProject(projectId); },
        t: wsProjectPickerT,
    });
}

async function wsRenderProjectPanel() {
    if (typeof window.initProjectPickerPanelSearch === 'function') {
        window.initProjectPickerPanelSearch('WebShell', 'ws-project-search', function () {
            if (typeof window.scheduleProjectPickerPanelSearch === 'function') {
                window.scheduleProjectPickerPanelSearch('WebShell', function () { wsLoadProjectPanelList(); });
            }
        });
    }
    if (typeof window.clearProjectPickerPanelSearch === 'function') {
        window.clearProjectPickerPanelSearch('WebShell', 'ws-project-search');
    }
    await wsLoadProjectPanelList();
    requestAnimationFrame(function () { document.getElementById('ws-project-search')?.focus(); });
}

function wsCloseProjectPanel() {
    var panel = document.getElementById('ws-project-panel');
    var btn = document.getElementById('ws-project-btn');
    if (panel) panel.style.display = 'none';
    if (btn) {
        btn.classList.remove('active');
        btn.setAttribute('aria-expanded', 'false');
    }
    if (typeof window.clearProjectPickerPanelSearch === 'function') {
        window.clearProjectPickerPanelSearch('WebShell', 'ws-project-search');
    }
}

async function wsToggleProjectPanel() {
    var panel = document.getElementById('ws-project-panel');
    var btn = document.getElementById('ws-project-btn');
    if (!panel) return;
    var isHidden = panel.style.display === 'none' || !panel.style.display;
    if (!isHidden) {
        wsCloseProjectPanel();
        return;
    }
    wsCloseRolePanel();
    wsCloseAgentModePanel();
    panel.style.display = 'flex';
    if (btn) {
        btn.classList.add('active');
        btn.setAttribute('aria-expanded', 'true');
    }
    await wsRenderProjectPanel();
}

async function wsSelectProject(projectId) {
    wsCloseProjectPanel();
    await applyWebshellAiProjectSelection(projectId || '');
}

async function applyWebshellAiProjectSelection(projectId) {
    var conn = webshellCurrentConn;
    if (!conn || !conn.id) return;
    var prev = getWebshellAiProjectSelection(conn);
    if (projectId === prev) {
        wsUpdateProjectButtonLabel();
        return;
    }
    var convId = getWebshellAiConvId(conn);
    if (convId) {
        try {
            var res = await apiFetch('/api/conversations/' + encodeURIComponent(convId) + '/project', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ projectId: projectId }),
            });
            if (!res.ok) {
                var err = await res.json().catch(function () { return {}; });
                throw new Error(err.error || res.statusText);
            }
            wsSetWebshellAiProject(conn, projectId);
            if (typeof showNotification === 'function') {
                showNotification(
                    projectId ? wsProjectT('projects.projectBound', 'Project bound') : wsProjectT('projects.projectUnbound', 'Project unbound'),
                    'success'
                );
            }
        } catch (e) {
            console.error(e);
            alert(wsProjectT('projects.updateProjectBindingFailed', 'updateProjectbinding failed') + ': ' + (e.message || e));
            wsUpdateProjectButtonLabel();
            return;
        }
    } else {
        wsSetWebshellAiProject(conn, projectId);
    }
    wsUpdateProjectButtonLabel();
}

function showNewProjectModalFromWebshellAi() {
    wsCloseProjectPanel();
    if (webshellCurrentConn && webshellCurrentConn.id) {
        window._projectModalFromWebshellConnId = webshellCurrentConn.id;
    }
    window._projectModalFromChat = false;
    if (typeof showNewProjectModal === 'function') showNewProjectModal();
}

window.applyWebshellAiProjectSelection = applyWebshellAiProjectSelection;
window.showNewProjectModalFromWebshellAi = showNewProjectModalFromWebshellAi;
window.wsToggleProjectPanel = wsToggleProjectPanel;
window.wsCloseProjectPanel = wsCloseProjectPanel;

// ─── end WebShell AI target selector ───

/** refresh selector display when WebShell AI tab is visible (sync possible changes FROM main  page) */
function wsRefreshSelectors() {
    wsUpdateRoleSelectorDisplay();
    wsRenderRoleList();
    wsUpdateProjectButtonLabel();
    var stored = localStorage.getItem('kestrel-chat-agent-mode') || 'eino_single';
    if (stored !== 'eino_single' && stored !== 'deep' && stored !== 'plan_execute' && stored !== 'supervisor') {
        stored = 'eino_single';
    }
    wsSyncAgentMode(stored);
}

// Click outside panel to close
document.addEventListener('click', function (e) {
    var rolePanel = document.getElementById('ws-role-selection-panel');
    var roleBtn = document.getElementById('ws-role-selector-btn');
    if (rolePanel && rolePanel.style.display !== 'none' && roleBtn && !rolePanel.contains(e.target) && !roleBtn.contains(e.target)) {
        wsCloseRolePanel();
    }
    var modePanel = document.getElementById('ws-agent-mode-panel');
    var modeBtn = document.getElementById('ws-agent-mode-btn');
    if (modePanel && modePanel.style.display !== 'none' && modeBtn && !modePanel.contains(e.target) && !modeBtn.contains(e.target)) {
        wsCloseAgentModePanel();
    }
    var projectPanel = document.getElementById('ws-project-panel');
    var projectBtn = document.getElementById('ws-project-btn');
    if (projectPanel && projectPanel.style.display !== 'none' && projectBtn && !projectPanel.contains(e.target) && !projectBtn.contains(e.target)) {
        wsCloseProjectPanel();
    }
});

// ─── end WebShell AI selector ───

/** stop currentWebShell AI streaming request */
function wsStopAiStream(conn) {
    // 1. Abort the fetch
    if (webshellAiAbortController) {
        try { webshellAiAbortController.abort(); } catch (e) { /* */ }
        webshellAiAbortController = null;
    }
    // 2. Cancel the reader
    if (webshellAiStreamReader) {
        try { webshellAiStreamReader.cancel(); } catch (e) { /* */ }
        webshellAiStreamReader = null;
    }
    // 3. Call backend cancel API if we have a conversation
    var convId = conn && conn.id ? (webshellAiConvMap[conn.id] || '') : '';
    if (convId && typeof apiFetch === 'function') {
        apiFetch('/api/agent-loop/cancel', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ conversationId: convId })
        }).catch(function () { /* ignore */ });
    }
    // 4. Reset UI state
    wsSetAiSendingState(false);
}

/** Toggle send/stop button state */
function wsSetAiSendingState(sending) {
    webshellAisending = sending;
    var sendBtn = document.getElementById('webshell-AI-send');
    var stopBtn = document.getElementById('webshell-AI-stop');
    if (sendBtn) {
        sendBtn.disabled = sending;
        sendBtn.style.display = sending ? 'none' : '';
    }
    if (stopBtn) {
        stopBtn.style.display = sending ? '' : 'none';
    }
}

// Fetch connection list FROM server (SQLite)
function getWebshellConnections() {
    if (typeof apiFetch === 'undefined') {
        return Promise.resolve([]);
    }
    var URL = '/api/WebShell/connections';
    return apiFetch(URL, { method: 'GET' })
        .then(function (r) { return r.json(); })
        .then(function (list) { return Array.isArray(list) ? list : []; })
        .catch(function (e) {
            console.warn('failed to read WebShell connection list', e);
            return [];
        });
}

function webshellConnectionProjectId(conn) {
    return (conn && (conn.project_id || conn.projectId) || '').trim();
}

function webshellProjectOptionsHtml(selectedId) {
    var selected = String(selectedId || '').trim();
    var HTML = '<option value="">' + escapeHtml(wsT('assets.unboundProject') || 'Do NOT bind for now') + '</option>';
    var entries = [];
    try {
        if (typeof projectNameById !== 'undefined') entries = Object.entries(projectNameById);
    } catch (e) {}
    entries.sort(function (a, b) {
        return String(a[1] || '').localeCompare(String(b[1] || ''), undefined, { sensitivity: 'BASE' });
    });
    entries.forEach(function (entry) {
        var ID = entry[0];
        var name = entry[1] || ID;
        if (!ID) return;
        HTML += '<option value="' + escapeHtml(ID) + '"' + (ID === selected ? ' selected' : '') + '>' + escapeHtml(name) + '</option>';
    });
    if (selected && !entries.some(function (entry) { return entry[0] === selected; })) {
        HTML += '<option value="' + escapeHtml(selected) + '" selected>' + escapeHtml(selected) + '</option>';
    }
    return HTML;
}

var webshellFormSelectMap = {};
var webshellFormSelectDocBound = false;
var WEBSHELL_FORM_SELECT_CARET = '<span class="webshell-form-SELECT-caret" aria-hidden="true"></span>';

function closeAllWebshellFormSelects() {
    Object.keys(webshellFormSelectMap).forEach(function (ID) {
        var reg = webshellFormSelectMap[ID];
        if (!reg || !reg.wrapper) return;
        reg.wrapper.classList.remove('open');
        if (reg.trigger) reg.trigger.setAttribute('aria-expanded', 'false');
    });
}

function syncWebshellFormSelect(SELECT) {
    if (!SELECT || !SELECT.id) return;
    var reg = webshellFormSelectMap[SELECT.id];
    if (!reg) return;
    var dropdown = reg.dropdown;
    var trigger = reg.trigger;
    var valueSpan = trigger.querySelector('.webshell-form-SELECT-value');
    dropdown.innerHTML = '';
    Array.prototype.forEach.call(SELECT.options, function (opt) {
        var item = document.createElement('button');
        item.type = 'button';
        item.className = 'webshell-form-SELECT-option';
        item.setAttribute('role', 'option');
        item.setAttribute('data-value', opt.value);
        item.setAttribute('aria-selected', opt.value === SELECT.value ? 'true' : 'false');
        if (opt.value === SELECT.value) item.classList.add('is-selected');

        var check = document.createElement('span');
        check.className = 'webshell-form-SELECT-check';
        check.textContent = '✓';
        check.setAttribute('aria-hidden', 'true');

        var label = document.createElement('span');
        label.className = 'webshell-form-SELECT-label';
        label.textContent = opt.textContent;

        item.appendChild(check);
        item.appendChild(label);
        dropdown.appendChild(item);
    });
    var selectedOpt = SELECT.options[SELECT.selectedIndex];
    if (valueSpan) valueSpan.textContent = selectedOpt ? selectedOpt.textContent : '';
    trigger.disabled = !!SELECT.disabled;
    reg.wrapper.classList.toggle('is-disabled', !!SELECT.disabled);
}

function enhanceWebshellFormSelect(SELECT) {
    if (!SELECT || !SELECT.id) return;
    var existing = webshellFormSelectMap[SELECT.id];
    if (existing && existing.SELECT !== SELECT) delete webshellFormSelectMap[SELECT.id];
    if (SELECT.dataset.webshellFormCustom === '1') {
        syncWebshellFormSelect(SELECT);
        return;
    }

    SELECT.dataset.webshellFormCustom = '1';
    SELECT.classList.add('webshell-form-native-SELECT');
    SELECT.tabIndex = -1;
    SELECT.setAttribute('aria-hidden', 'true');

    var wrapper = document.createElement('div');
    wrapper.className = 'webshell-form-SELECT-UI';

    var trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'webshell-form-SELECT-trigger';
    trigger.setAttribute('aria-haspopup', 'listbox');
    trigger.setAttribute('aria-expanded', 'false');

    var valueSpan = document.createElement('span');
    valueSpan.className = 'webshell-form-SELECT-value';
    trigger.appendChild(valueSpan);
    trigger.insertAdjacentHTML('beforeend', WEBSHELL_FORM_SELECT_CARET);

    var dropdown = document.createElement('div');
    dropdown.className = 'webshell-form-SELECT-dropdown';
    dropdown.setAttribute('role', 'listbox');

    var parent = SELECT.parentNode;
    parent.insertBefore(wrapper, SELECT);
    wrapper.appendChild(trigger);
    wrapper.appendChild(dropdown);
    wrapper.appendChild(SELECT);

    webshellFormSelectMap[SELECT.id] = { wrapper: wrapper, trigger: trigger, dropdown: dropdown, SELECT: SELECT };

    trigger.addEventListener('click', function (e) {
        e.stopPropagation();
        if (SELECT.disabled) return;
        var open = wrapper.classList.contains('open');
        closeAllWebshellFormSelects();
        if (!open) {
            wrapper.classList.add('open');
            trigger.setAttribute('aria-expanded', 'true');
        }
    });

    dropdown.addEventListener('click', function (e) {
        var item = e.target.closest('.webshell-form-SELECT-option');
        if (!item) return;
        e.stopPropagation();
        var value = item.getAttribute('data-value');
        if (value === null) return;
        if (SELECT.value !== value) {
            SELECT.value = value;
            SELECT.dispatchEvent(new Event('change', { bubbles: true }));
        }
        wrapper.classList.remove('open');
        trigger.setAttribute('aria-expanded', 'false');
        syncWebshellFormSelect(SELECT);
    });

    SELECT.addEventListener('change', function () {
        syncWebshellFormSelect(SELECT);
    });

    syncWebshellFormSelect(SELECT);
}

function refreshWebshellFormSelects(root) {
    var container = root || document.getElementById('webshell-modal');
    if (!container) return;
    Object.keys(webshellFormSelectMap).forEach(function (ID) {
        if (!document.getElementById(ID)) delete webshellFormSelectMap[ID];
    });
    container.querySelectorAll('SELECT').forEach(enhanceWebshellFormSelect);
    if (!webshellFormSelectDocBound) {
        webshellFormSelectDocBound = true;
        document.addEventListener('click', closeAllWebshellFormSelects);
        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') closeAllWebshellFormSelects();
        });
    }
}

function populateWebshellProjectSelect(selectedId) {
    var sel = document.getElementById('webshell-project-ID');
    if (!sel) return Promise.resolve();
    var selected = String(selectedId || '').trim();
    sel.innerHTML = webshellProjectOptionsHtml(selected);
    sel.value = selected;
    syncWebshellFormSelect(sel);
    var loadPromise = Promise.resolve([]);
    if (typeof ensureProjectsLoaded === 'function') {
        loadPromise = ensureProjectsLoaded();
    } else if (typeof fetchAllProjects === 'function') {
        loadPromise = fetchAllProjects(false).then(function (list) {
            if (typeof rebuildProjectNameMap === 'function') rebuildProjectNameMap(list || []);
            return list || [];
        });
    }
    return loadPromise.then(function () {
        sel.innerHTML = webshellProjectOptionsHtml(selected);
        sel.value = selected;
        syncWebshellFormSelect(sel);
    }).catch(function (e) {
        console.warn('failed to load WebShell target option  items', e);
    });
}

// refresh connection list FROM server AND redraw sidebar
function refreshWebshellConnectionsFromServer() {
    return getWebshellConnections().then(function (list) {
        webshellConnections = list;
        renderWebshellList();
        if (typeof ensureProjectsLoaded === 'function') {
            ensureProjectsLoaded().then(function () {
                renderWebshellList();
            }).catch(function () {});
        }
        return list;
    });
}

// Use wsT to avoid infinite recursion conflicting with global window.t
function wsT(key) {
    var globalT = typeof window !== 'undefined' ? window.t : null;
    if (typeof globalT === 'function' && globalT !== wsT) return globalT(key);
    var fallback = {
        'WebShell.title': 'WebShell Management',
        'WebShell.addConnection': 'Add Connection',
        'WebShell.cmdParam': 'Command Parameter',
        'WebShell.cmdParamPlaceholder': 'Defaults to cmd; if set to xxx, requests send xxx=command',
        'WebShell.encoding': 'Encoding',
        'WebShell.encodingAuto': 'Auto Detect',
        'WebShell.encodingUtf8': 'UTF-8',
        'WebShell.encodingGbk': 'GBK (Chinese Windows)',
        'WebShell.encodingGb18030': 'GB18030',
        'WebShell.encodingHint': 'If Chinese Windows target displays garbled text, switch to GBK or GB18030',
        'WebShell.OS': 'Target OS',
        'WebShell.osAuto': 'Auto (Inferred from Shell Type)',
        'WebShell.osLinux': 'Linux / Unix',
        'WebShell.osWindows': 'Windows',
        'WebShell.osHint': 'Determines file manager/upload commands for Linux vs Windows; choose Windows for PHP/JSP on Windows',
        'WebShell.connections': 'Connections',
        'WebShell.noConnections': 'No connections; click "Add Connection"',
        'WebShell.selectOrAdd': 'Select a connection from the left or add a new WebShell connection',
        'WebShell.deleteConfirm': 'Are you sure you want to delete this connection?',
        'WebShell.editConnection': 'Edit',
        'WebShell.editConnectionTitle': 'Edit Connection',
        'WebShell.tabTerminal': 'Terminal',
        'WebShell.tabFileManager': 'File Manager',
        'WebShell.tabAiAssistant': 'AI Assistant',
        'WebShell.tabDbManager': 'Database Manager',
        'WebShell.tabMemo': 'Scratchpad',
        'WebShell.dbType': 'Database Type',
        'WebShell.dbHost': 'Host',
        'WebShell.dbPort': 'Port',
        'WebShell.dbUsername': 'Username',
        'WebShell.dbPassword': 'Password',
        'WebShell.dbName': 'Database Name',
        'WebShell.dbSqlitePath': 'SQLite File Path',
        'WebShell.dbSqlPlaceholder': 'Enter SQL, e.g.: SELECT version();',
        'WebShell.dbRunSql': 'Run SQL',
        'WebShell.dbTest': 'Test Connection',
        'WebShell.dbOutput': 'Execution Output',
        'WebShell.dbNoConn': 'Please select a WebShell connection first',
        'WebShell.dbSqlRequired': 'Please enter SQL',
        'WebShell.dbRunning': 'Executing database command, please wait...',
        'WebShell.dbCliHint': 'If client command is missing, install the corresponding client (MySQL/psql/sqlite3/sqlcmd) on the target host',
        'WebShell.dbExecFailed': 'Database execution failed',
        'WebShell.dbSchema': 'Database Schema',
        'WebShell.dbLoadSchema': 'Load Schema',
        'WebShell.dbNoSchema': 'No database schema loaded, please load schema first',
        'WebShell.dbSelectTableHint': 'Click a table name to view columns and generate SELECT query',
        'WebShell.dbNoColumns': 'No column information',
        'WebShell.dbResultTable': 'Result Table',
        'WebShell.dbClearSql': 'Clear SQL',
        'WebShell.dbTemplateSql': 'Sample SQL',
        'WebShell.dbRows': 'Rows',
        'WebShell.dbColumns': 'Columns',
        'WebShell.dbSchemaFailed': 'Failed to load database schema',
        'WebShell.dbSchemaLoaded': 'Schema loaded successfully',
        'WebShell.dbAddProfile': 'Add Profile',
        'WebShell.dbExecSuccess': 'SQL executed successfully',
        'WebShell.dbNoOutput': 'Execution completed (no output)',
        'WebShell.dbRenameProfile': 'Rename',
        'WebShell.dbDeleteProfile': 'Delete Profile',
        'WebShell.dbDeleteProfileConfirm': 'Are you sure you want to delete this database profile?',
        'WebShell.dbProfileNamePrompt': 'Please enter connection name',
        'WebShell.dbProfileName': 'Connection Name',
        'WebShell.dbProfiles': 'Database Connections',
        'WebShell.aiSystemReadyMessage': 'System ready. Please enter your test requirements and the system will automatically run the appropriate security test.',
        'WebShell.aiPlaceholder': 'e.g.: List files in current directory',
        'WebShell.aiSend': 'Send',
        'WebShell.aiMemo': 'Scratchpad',
        'WebShell.aiMemoPlaceholder': 'Record key commands, test notes, reproduction steps...',
        'WebShell.aiMemoClear': 'Clear',
        'WebShell.aiMemoSaving': 'Saving...',
        'WebShell.aiMemoSaved': 'Saved locally',
        'WebShell.terminalWelcome': 'WebShell Terminal — Press Enter to execute command (Ctrl+L to clear screen)',
        'WebShell.quickCommands': 'Quick Commands',
        'WebShell.downloadFile': 'Download',
        'WebShell.filePath': 'Current Path',
        'WebShell.listDir': 'List Directory',
        'WebShell.readFile': 'Read',
        'WebShell.editFile': 'Edit',
        'WebShell.deleteFile': 'Delete',
        'WebShell.saveFile': 'Save',
        'WebShell.cancelEdit': 'Cancel',
        'WebShell.parentDir': 'Parent Directory',
        'WebShell.execError': 'Execution Failed',
        'WebShell.testConnectivity': 'Test Connectivity',
        'WebShell.testSuccess': 'Connectivity normal, Shell is accessible',
        'WebShell.testFailed': 'Connectivity test failed',
        'WebShell.testNoExpectedOutput': 'Shell returned a response without expected output; verify password and command parameter name',
        'WebShell.clearScreen': 'Clear Screen',
        'WebShell.copyTerminalLog': 'Copy Terminal Log',
        'WebShell.terminalIdle': 'Idle',
        'WebShell.terminalRunning': 'Running',
        'WebShell.terminalCopyOk': 'Log copied',
        'WebShell.terminalCopyFail': 'Copy failed',
        'WebShell.terminalNewWindow': 'New Terminal',
        'WebShell.terminalWindowPrefix': 'Terminal',
        'WebShell.running': 'Running...',
        'WebShell.waitFinish': 'Please wait for current command to complete',
        'WebShell.newDir': 'New Directory',
        'WebShell.rename': 'Rename',
        'WebShell.upload': 'Upload',
        'WebShell.newFile': 'New File',
        'WebShell.filterPlaceholder': 'Filter filenames',
        'WebShell.batchDelete': 'Batch Delete',
        'WebShell.batchDownload': 'Batch Download',
        'WebShell.moreActions': 'More Actions',
        'WebShell.refresh': 'Refresh',
        'WebShell.selectAll': 'Select All',
        'WebShell.breadcrumbHome': 'Root',
        'WebShell.dirTree': 'Directory Tree',
        'WebShell.searchPlaceholder': 'Search connections...',
        'WebShell.noMatchConnections': 'No matching connections',
        'WebShell.batchProbe': 'Batch Probe',
        'WebShell.probeRunning': 'Probing...',
        'WebShell.probeOnline': 'Online',
        'WebShell.probeOffline': 'Offline',
        'WebShell.probeNoConnections': 'No connections to probe',
        'WebShell.back': 'Back',
        'WebShell.colModifiedAt': 'Modified Time',
        'WebShell.colPerms': 'Permissions',
        'WebShell.colOwner': 'Owner',
        'WebShell.colGroup': 'Group',
        'WebShell.colType': 'Type',
        'common.delete': 'Delete',
        'common.refresh': 'Refresh',
        'common.actions': 'Actions'
    };
    return fallback[key] || key;
}

function wsTOr(key, fallbackText) {
    var text = wsT(key);
    if (!text || text === key) return fallbackText;
    return text;
}

// Clear screen bound once: recreate terminal to ensure clean prompt
function bindWebshellClearOnce() {
    if (window._webshellClearBound) return;
    window._webshellClearBound = true;
    document.body.addEventListener('click', function (e) {
        var btn = e.target && (e.target.id === 'webshell-terminal-clear' ? e.target : e.target.closest ? e.target.closest('#webshell-terminal-clear') : null);
        if (!btn || !webshellCurrentConn) return;
        e.preventDefault();
        e.stopPropagation();
        if (webshellclearInProgress) return;
        webshellclearInProgress = true;
        try {
            destroyWebshellTerminal();
            webshellLineBuffer = '';
            webshellHistoryIndex = -1;
            if (webshellCurrentConn && webshellCurrentConn.id) {
                var sid = getActiveWebshellTerminalSessionId(webshellCurrentConn.id);
                clearWebshellTerminalLog(getWebshellTerminalSessionKey(webshellCurrentConn.id, sid));
            }
            initWebshellTerminal(webshellCurrentConn);
        } finally {
            setTimeout(function () { webshellclearInProgress = false; }, 100);
        }
    }, true);
}

// WebShell row/toolbar actions dropdown: close on click outside
function bindWebshellActionMenusAutoCloseOnce() {
    if (window._webshellActionMenusAutoCloseBound) return;
    window._webshellActionMenusAutoCloseBound = true;
    document.addEventListener('click', function (e) {
        // Inside details: allow default browser toggle
        var clickedInMenu = e.target && e.target.closest && (
            e.target.closest('details.webshell-conn-actions') ||
            e.target.closest('details.webshell-row-actions') ||
            e.target.closest('details.webshell-toolbar-actions')
        );
        if (clickedInMenu) return;

        var openDetails = document.querySelectorAll(
            'details.webshell-conn-actions[open],details.webshell-row-actions[open],details.webshell-toolbar-actions[open]'
        );
        openDetails.forEach(function (d) { d.open = false; });
    }, true);
}

// Initialize WebShell page (fetch connection list from SQLite)
function initWebshellPage() {
    bindWebshellClearOnce();
    bindWebshellActionMenusAutoCloseOnce();
    destroyWebshellTerminal();
    webshellCurrentConn = null;
    currentWebshellId = null;
    webshellConnections = [];
    renderWebshellList();
    applyWebshellSidebarWidth();
    initWebshellSidebarResize();

    // Connection search: filter list in real time
    var searchEl = document.getElementById('webshell-conn-search');
    if (searchEl && searchEl.dataset.bound !== '1') {
        searchEl.dataset.bound = '1';
        searchEl.addEventListener('input', function () {
            renderWebshellList();
        });
    }

    const workspace = document.getElementById('webshell-workspace');
    if (workspace) {
        workspace.innerHTML = '<div class="webshell-workspace-placeholder" data-id18n="WebShell.selectOrAdd">' + (wsT('WebShell.selectOrAdd')) + '</div>';
    }
    getWebshellConnections().then(function (list) {
        webshellConnections = list;
        renderWebshellList();
    });

    var batchProbeBtn = document.getElementById('webshell-batch-probe-btn');
    if (batchProbeBtn && batchProbeBtn.dataset.bound !== '1') {
        batchProbeBtn.dataset.bound = '1';
        batchProbeBtn.addEventListener('click', function () {
            runBatchProbeWebshellConnections();
        });
    }
    updateWebshellBatchProbeButton();
}

function getWebshellSidebarWidth() {
    try {
        const w = parseInt(localStorage.getItem(WEBSHELL_SIDEBAR_WIDTH_KEY), 10);
        if (!isNaN(w) && w >= 260 && w <= 800) return w;
    } catch (e) {}
    return WEBSHELL_DEFAULT_SIDEBAR_WIDTH;
}

function setWebshellSidebarWidth(px) {
    localStorage.setItem(WEBSHELL_SIDEBAR_WIDTH_KEY, String(px));
}

function applyWebshellSidebarWidth() {
    const sidebar = document.getElementById('webshell-sidebar');
    if (!sidebar) return;
    const parentW = sidebar.parentElement ? sidebar.parentElement.offsetWidth : 0;
    let w = getWebshellSidebarWidth();
    if (parentW > 0) w = Math.min(w, Math.max(260, parentW - WEBSHELL_MAIN_MIN_WIDTH));
    sidebar.style.width = w + 'px';
}

function initWebshellSidebarResize() {
    const handle = document.getElementById('webshell-resize-handle');
    const sidebar = document.getElementById('webshell-sidebar');
    if (!handle || !sidebar || handle.dataset.resizeBound === '1') return;
    handle.dataset.resizeBound = '1';
    let startX = 0, startW = 0;
    function onMove(e) {
        const dx = e.clientX - startX;
        let w = Math.round(startW + dx);
        const parentW = sidebar.parentElement ? sidebar.parentElement.offsetWidth : 800;
        const min = 260;
        const max = Math.min(800, parentW - WEBSHELL_MAIN_MIN_WIDTH);
        w = Math.max(min, Math.min(max, w));
        sidebar.style.width = w + 'px';
    }
    function onUp() {
        handle.classList.remove('active');
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        document.removeEventListener('mousemove', onMove);
        document.removeEventListener('mouseup', onUp);
        setWebshellSidebarWidth(parseInt(sidebar.style.width, 10) || WEBSHELL_DEFAULT_SIDEBAR_WIDTH);
    }
    handle.addEventListener('mousedown', function (e) {
        if (e.button !== 0) return;
        e.preventDefault();
        startX = e.clientX;
        startW = sidebar.offsetWidth;
        handle.classList.add('active');
        document.body.style.cursor = 'col-resize';
        document.body.style.userSelect = 'none';
        document.addEventListener('mousemove', onMove);
        document.addEventListener('mouseup', onUp);
    });
}

// Destroy current terminal instance when switching or navigating away
function destroyWebshellTerminal() {
    if (webshellTerminalResizeObserver && webshellTerminalResizeContainer) {
        try { webshellTerminalResizeObserver.unobserve(webshellTerminalResizeContainer); } catch (e) {}
        webshellTerminalResizeObserver = null;
        webshellTerminalResizeContainer = null;
    }
    if (webshellTerminalInstance) {
        try {
            webshellTerminalInstance.dispose();
        } catch (e) {}
        webshellTerminalInstance = null;
    }
    webshellTerminalFitaddon = null;
    webshellLineBuffer = '';
    webshellRunning = false;
    webshellTerminalrunning = false;
    setWebshellTerminalStatus(false);
}

// Render connection list
function renderWebshellList() {
    const listEl = document.getElementById('webshell-list');
    if (!listEl) return;

    const searchEl = document.getElementById('webshell-conn-search');
    const searchTerm = (searchEl && typeof searchEl.value === 'string' ? searchEl.value : '').trim().toLowerCase();

    if (!webshellConnections.length) {
        listEl.innerHTML = '<div class="webshell-empty" data-id18n="WebShell.noConnections">' + (wsT('WebShell.noConnections')) + '</div>';
        return;
    }

    const filtered = searchTerm
        ? webshellConnections.filter(conn => {
            const ID = String(conn.id || '').toLowerCase();
            const URL = String(conn.URL || '').toLowerCase();
            const remark = String(conn.remark || '').toLowerCase();
            return ID.includes(searchTerm) || URL.includes(searchTerm) || remark.includes(searchTerm);
        })
        : webshellConnections;

    if (filtered.length === 0) {
        listEl.innerHTML = '<div class="webshell-empty">' + (wsT('WebShell.noMatchConnections') || 'No matching connections') + '</div>';
        return;
    }

    listEl.innerHTML = filtered.map(conn => {
        const remark = (conn.remark || conn.URL || '').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        const URL = (conn.URL || '').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        const urlTitle = (conn.URL || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
        const active = currentWebshellId === conn.id ? ' active' : '';
        const safeId = escapeHtml(conn.id);
        const actionsLabel = wsT('common.actions') || 'Actions';
        const probe = webshellProbeStatusById[conn.id] || null;
        var probeHtml = '';
        if (probe && probe.state === 'probing') {
            probeHtml = '<span class="webshell-probe-badge probing">' + (wsT('WebShell.probeRunning') || 'Probing...') + '</span>';
        } else if (probe && probe.state === 'ok') {
            probeHtml = '<span class="webshell-probe-badge ok">' + (wsT('WebShell.probeOnline') || 'Online') + '</span>';
        } else if (probe && probe.state === 'fail') {
            probeHtml = '<span class="webshell-probe-badge fail" title="' + escapeHtml(probe.message || '') + '">' + (wsT('WebShell.probeOffline') || 'Offline') + '</span>';
        }
        var encNorm = normalizeWebshellEncoding(conn.encoding);
        var encHtml = '';
        if (encNorm && encNorm !== 'auto') {
            encHtml = '<span class="webshell-probe-badge" title="' + escapeHtml(wsT('WebShell.encoding') || 'Encoding') + '">' + escapeHtml(encNorm.toUpperCase()) + '</span>';
        }
        var osNorm = normalizeWebshellOS(conn.OS);
        var osHtml = '';
        if (osNorm && osNorm !== 'auto') {
            var osLabel = osNorm === 'windows' ? 'WIN' : 'LINUX';
            osHtml = '<span class="webshell-probe-badge" title="' + escapeHtml(wsT('WebShell.OS') || 'targetSystem') + '">' + osLabel + '</span>';
        }
        return (
            '<div class="webshell-item' + active + '" data-idD="' + safeId + '">' +
            '<div class="webshell-item-remark-row"><div class="webshell-item-remark" title="' + urlTitle + '">' + remark + '</div>' + probeHtml + osHtml + encHtml + '</div>' +
            '<div class="webshell-item-URL" title="' + urlTitle + '">' + URL + '</div>' +
            '<div class="webshell-item-actions">' +
            '<details class="webshell-conn-actions"><summary class="btn-ghost btn-sm webshell-conn-actions-btn" title="' + actionsLabel + '">' + actionsLabel + '</summary>' +
            '<div class="webshell-row-actions-menu">' +
            '<button type="button" class="btn-ghost btn-sm webshell-edit-conn-btn" data-idD="' + safeId + '" title="' + wsT('WebShell.editConnection') + '">' + wsT('WebShell.editConnection') + '</button>' +
            '<button type="button" class="btn-ghost btn-sm webshell-delete-btn" data-idD="' + safeId + '" title="' + wsT('common.delete') + '">' + wsT('common.delete') + '</button>' +
            '</div></details>' +
            '</div>' +
            '</div>'
        );
    }).JOIN('');

    listEl.querySelectorAll('.webshell-item').forEach(el => {
        el.addEventListener('click', function (e) {
            if (e.target.closest('.webshell-delete-btn') || e.target.closest('.webshell-edit-conn-btn') || e.target.closest('.webshell-conn-actions-btn')) return;
            selectWebshell(el.getAttribute('data-idD'));
        });
    });
    listEl.querySelectorAll('.webshell-edit-conn-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            showEditWebshellModal(btn.getAttribute('data-idD'));
        });
    });
    listEl.querySelectorAll('.webshell-delete-btn').forEach(btn => {
        btn.addEventListener('click', function (e) {
            e.stopPropagation();
            deleteWebshell(btn.getAttribute('data-idD'));
        });
    });
}

function probeWebshellConnection(conn) {
    if (!conn || typeof apiFetch === 'undefined') {
        return Promise.resolve({ ok: false, message: wsT('WebShell.testFailed') || 'Connectivity test failed' });
    }
    var probeToken = buildWebshellProbeToken();
    return apiFetch('/api/WebShell/exec', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            URL: conn.URL,
            password: conn.password || '',
            type: conn.type || 'PHP',
            method: ((conn.method || 'POST').toLowerCase() === 'GET') ? 'GET' : 'POST',
            cmd_param: conn.cmdParam || '',
            encoding: webshellConnEncoding(conn),
            OS: webshellConnOS(conn),
            connection_id: conn.id || '',
            command: buildWebshellProbeCommand(probeToken),
            connection_id: password === '********' ? (document.getElementById('webshell-edit-ID')?.value || '') : ''
        })
    })
        .then(function (r) { return r.json(); })
        .then(function (data) {
            var output = (data && data.output != null) ? String(data.output) : '';
            var ok = !!(data && data.ok && isWebshellProbeOutputMatched(output, probeToken));
            if (ok) return { ok: true, message: wsT('WebShell.testSuccess') || 'Connectivity normal, Shell is accessible' };
            var msg = (data && data.error) ? data.error : (wsT('WebShell.testFailed') || 'Connectivity test failed');
            return { ok: false, message: msg };
        })
        .catch(function (e) {
            return { ok: false, message: (e && e.message) ? e.message : String(e) };
        });
}

function updateWebshellBatchProbeButton(done, total, okCount) {
    var btn = document.getElementById('webshell-batch-probe-btn');
    if (!btn) return;
    if (webshellBatchProberunning) {
        var d = typeof done === 'number' ? done : 0;
        var t = typeof total === 'number' ? total : webshellConnections.length;
        btn.disabled = true;
        btn.textContent = (wsT('WebShell.probeRunning') || 'Probing...') + ' ' + d + '/' + t;
        return;
    }
    btn.disabled = false;
    if (typeof done === 'number' && typeof total === 'number' && total > 0 && typeof okCount === 'number') {
        btn.textContent = (wsT('WebShell.batchProbe') || 'Batch Probe') + ' (' + okCount + '/' + total + ')';
    } else {
        btn.textContent = wsT('WebShell.batchProbe') || 'Batch Probe';
    }
}

function runBatchProbeWebshellConnections() {
    if (webshellBatchProberunning) return;
    if (!Array.isArray(webshellConnections) || webshellConnections.length === 0) {
        alert(wsT('WebShell.probeNoConnections') || 'No connections to probe');
        return;
    }
    webshellBatchProberunning = true;
    var total = webshellConnections.length;
    var done = 0;
    var okCount = 0;

    webshellConnections.forEach(function (conn) {
        if (!conn || !conn.id) return;
        webshellProbeStatusById[conn.id] = { state: 'probing', message: '' };
    });
    renderWebshellList();
    updateWebshellBatchProbeButton(done, total, okCount);

    var idx = 0;
    var concurrency = Math.min(4, total);

    function runOne() {
        if (idx >= total) return Promise.resolve();
        var conn = webshellConnections[idx++];
        if (!conn || !conn.id) {
            done++;
            updateWebshellBatchProbeButton(done, total, okCount);
            return runOne();
        }
        return probeWebshellConnection(conn).then(function (res) {
            if (res.ok) okCount++;
            webshellProbeStatusById[conn.id] = {
                state: res.ok ? 'ok' : 'fail',
                message: res.message || ''
            };
            done++;
            renderWebshellList();
            updateWebshellBatchProbeButton(done, total, okCount);
        }).then(runOne);
    }

    var workers = [];
    for (var i = 0; i < concurrency; i++) workers.push(runOne());
    Promise.ALL(workers).finally(function () {
        webshellBatchProberunning = false;
        updateWebshellBatchProbeButton(done, total, okCount);
    });
}

function escapeHtml(s) {
    if (!s) return '';
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
}

function escapeHtmlAttr(s) {
    return escapeHtml(s).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function webshellFinalizationReasonLabel(reason, status) {
    var key = String(reason || status || '').trim();
    var labels = {
        pending_tool_executions: 'waiting for tool execution to complete',
        missing_execution_evidence: 'Missing completion evidence',
        awaiting_hitl: 'waiting for manual confirmation',
        empty_response: 'No valid reply captured',
        missing_finalization_contract: 'Missing finalisation evidence',
        in_progress: 'still verifying',
        blocked: 'Check failed',
        failed: 'Task failed',
        cancelled: 'Task cancelled',
        verified: 'Verified'
    };
    return labels[key] || key || 'Check failed';
}

function webshellFinalizationMissingCheckLabel(check) {
    var s = String(check || '').trim();
    if (!s) return '';
    if (s.indexOf('tool execution still queued or running') !== -1) return 'Tool execution still in progress';
    if (s.indexOf('execution evidence is required but no completed tool execution was recorded') !== -1) return 'Execution evidence required, but no completed tool execution was recorded';
    if (s.indexOf('workflow is awaiting HITL approval') !== -1) return 'Workflows is awaiting manual confirmation';
    if (s.indexOf('assistant final text is empty') !== -1) return 'No valid final text captured';
    if (s.indexOf('agent run status is ') === 0) return 'Task status still: ' + s.replace('agent run status is ', '');
    return s;
}

function webshellFinalizationNotice(data, eventMessage, hasContract) {
    var reason = hasContract
        ? webshellFinalizationReasonLabel(data && data.completionReason, data && data.status)
        : webshellFinalizationReasonLabel('missing_finalization_contract');
    var lines = ['Still verifying, final conclusion pending', 'Status: ' + reason];
    var pending = Array.isArray(data && data.pendingExecutionIds) ? data.pendingExecutionIds.filter(Boolean).map(String) : [];
    if (pending.length) {
        lines.push('Pending tools: ' + pending.slice(0, 3).join(', ') + (pending.length > 3 ? ' etc.' : ''));
    }
    var missing = Array.isArray(data && data.missingChecks)
        ? data.missingChecks.map(webshellFinalizationMissingCheckLabel).filter(Boolean)
        : [];
    if (missing.length) {
        lines.push('Pending completion checks: ' + missing.slice(0, 2).join('; ') + (missing.length > 2 ? ' etc.' : ''));
    }
    if (!hasContract && eventMessage) {
        lines.push('Candidate output moved to process details.');
    }
    return lines.JOIN('\n');
}

function escapeSingleQuotedShellArg(value) {
    var s = value == null ? '' : String(value);
    return "'" + s.replace(/'/g, "'\\''") + "'";
}

function safeConnIdForStorage(conn) {
    if (!conn || !conn.id) return '';
    return String(conn.id).replace(/[^\w.-]/g, '_');
}

function normalizeWebshellPath(path) {
    var p = path == null ? '.' : String(path).trim();
    if (!p) return '.';
    p = p.replace(/\\/g, '/').replace(/\/+/g, '/');
    if (p === '/') return '/';
    // Windows drive root kept as "C:/" to prevent parent resolution bugs
    if (/^[A-Za-z]:\/?$/.test(p)) {
        return p.slice(0, 2) + '/';
    }
    if (!p || p === '.') return '.';
    if (p.endsWith('/')) p = p.slice(0, -1);
    return p || '.';
}

function getWebshellSelectedFile(conn) {
    if (!conn || !conn.id) return '';
    var p = webshellSelectedFileByConn[conn.id];
    if (!p) return '';
    return normalizeWebshellPath(p);
}

function setWebshellSelectedFile(conn, path) {
    if (!conn || !conn.id) return;
    if (!path) {
        delete webshellSelectedFileByConn[conn.id];
        return;
    }
    webshellSelectedFileByConn[conn.id] = normalizeWebshellPath(path);
}

function getWebshellParentPath(path) {
    var p = normalizeWebshellPath(path);
    if (p === '/') return '/';
    // Cannot navigate above Windows drive root
    if (/^[A-Za-z]:\/$/.test(p)) return p;
    // Allow upward navigation: . -> .. -> ../..
    if (p === '.') return '..';
    if (/^(?:\.\.\/)*\.\.$/.test(p)) return p + '/..';
    // Preserve relative navigation path until remote path is returned
    var idx = p.lastIndexOf('/');
    if (idx < 0) return '.';
    var parent = p.slice(0, idx) || (p.startsWith('/') ? '/' : '.');
    if (/^[A-Za-z]:$/.test(parent)) return parent + '/';
    return parent;
}

function inferPathFromWindowsDirOutput(rawOutput) {
    var text = String(rawOutput || '').replace(/\r/g, '');
    var lines = text.split('\n');
    for (var i = 0; i < lines.length; i++) {
        var line = String(lines[i] || '').trim();
        // Chinese Windows locale: "C:\xxx 的directory"
        var zh = line.match(/^([A-Za-z]:\\.*)\s+的directory$/);
        if (zh && zh[1]) return normalizeWebshellPath(zh[1]);
        // English: Directory of C:\xxx
        var en = line.match(/^Directory of\s+([A-Za-z]:\\.*)$/i);
        if (en && en[1]) return normalizeWebshellPath(en[1]);
    }
    return '';
}

function getWebshellTerminalSessionKey(connId, sessionId) {
    if (!connId || !sessionId) return '';
    return String(connId) + '::' + String(sessionId);
}

function normalizeWebshellTerminalSessions(raw) {
    var state = raw && typeof raw === 'object' ? raw : {};
    var list = Array.isArray(state.sessions) ? state.sessions.slice() : [];
    if (!list.length) {
        list = [{ ID: 't1', name: (wsT('WebShell.terminalWindowPrefix') || 'Terminal') + '1' }];
    }
    list = list.map(function (s, i) {
        var ID = (s && s.id ? String(s.id) : ('t' + (i + 1)));
        var name = (s && s.name ? String(s.name) : ((wsT('WebShell.terminalWindowPrefix') || 'Terminal') + (i + 1)));
        return { ID: ID, name: name };
    });
    var activeId = state.activeId;
    if (!activeId || !list.some(function (s) { return s.id === activeId; })) activeId = list[0].id;
    return { sessions: list, activeId: activeId };
}

function getWebshellTerminalSessions(connId) {
    if (!connId) return normalizeWebshellTerminalSessions(null);
    if (webshellTerminalSessionsByConn[connId]) return webshellTerminalSessionsByConn[connId];
    var state = normalizeWebshellTerminalSessions(null);
    webshellTerminalSessionsByConn[connId] = state;
    return state;
}

function saveWebshellTerminalSessions(connId, state) {
    if (!connId || !state) return;
    var normalized = normalizeWebshellTerminalSessions(state);
    webshellTerminalSessionsByConn[connId] = normalized;
    queueWebshellPersistStateSave(connId);
}

function getActiveWebshellTerminalSessionId(connId) {
    return getWebshellTerminalSessions(connId).activeId;
}

function getWebshellTerminalLog(connId) {
    if (!connId) return '';
    if (typeof webshellTerminalLogsByConn[connId] === 'string') return webshellTerminalLogsByConn[connId];
    webshellTerminalLogsByConn[connId] = '';
    return '';
}

function saveWebshellTerminalLog(connId, content) {
    if (!connId) return;
    var text = String(content || '');
    var maxLen = 50000; // keep recent terminal output only
    if (text.length > maxLen) text = text.slice(text.length - maxLen);
    webshellTerminalLogsByConn[connId] = text;
}

function appendWebshellTerminalLog(connId, CHUNK) {
    if (!connId || !CHUNK) return;
    var current = getWebshellTerminalLog(connId);
    saveWebshellTerminalLog(connId, current + String(CHUNK));
}

function clearWebshellTerminalLog(connId) {
    if (!connId) return;
    webshellTerminalLogsByConn[connId] = '';
}

function buildWebshellPersistState(connId) {
    var dbState = getWebshellDbState({ ID: connId });
    var terminalSessions = getWebshellTerminalSessions(connId);
    return {
        dbState: dbState || null,
        terminalSessions: terminalSessions || null
    };
}

function applyWebshellPersistState(connId, state) {
    if (!connId || !state || typeof state !== 'object') return;
    if (state.dbState && typeof state.dbState === 'object') {
        var key = getWebshellDbStateStorageKey({ ID: connId });
        webshellDbConfigByConn[key] = normalizeWebshellDbState(state.dbState);
    }
    if (state.terminalSessions && typeof state.terminalSessions === 'object') {
        webshellTerminalSessionsByConn[connId] = normalizeWebshellTerminalSessions(state.terminalSessions);
    }
}

function queueWebshellPersistStateSave(connId) {
    if (!connId || typeof apiFetch !== 'function') return;
    if (webshellPersistsaveTimersByConn[connId]) clearTimeout(webshellPersistsaveTimersByConn[connId]);
    webshellPersistsaveTimersByConn[connId] = setTimeout(function () {
        delete webshellPersistsaveTimersByConn[connId];
        var payload = buildWebshellPersistState(connId);
        apiFetch('/api/WebShell/connections/' + encodeURIComponent(connId) + '/state', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ state: payload })
        }).catch(function () {});
    }, 500);
}

function ensureWebshellPersistStateLoaded(conn) {
    if (!conn || !conn.id || typeof apiFetch !== 'function') return Promise.resolve();
    if (webshellPersistLoadedByConn[conn.id]) return Promise.resolve();
    return apiFetch('/api/WebShell/connections/' + encodeURIComponent(conn.id) + '/state', { method: 'GET' })
        .then(function (r) { return r.ok ? r.json() : Promise.reject(new Error('load state failed')); })
        .then(function (data) {
            applyWebshellPersistState(conn.id, data && data.state ? data.state : {});
            webshellPersistLoadedByConn[conn.id] = true;
        })
        .catch(function () {
            webshellPersistLoadedByConn[conn.id] = true;
        });
}

function setWebshellTerminalStatus(running) {
    webshellTerminalrunning = !!running;
    var el = document.getElementById('webshell-terminal-status');
    if (!el) return;
    el.classList.toggle('running', !!running);
    el.classList.toggle('idle', !running);
    el.textContent = running ? (wsT('WebShell.terminalRunning') || 'Running') : (wsT('WebShell.terminalIdle') || 'Idle');
}

function renderWebshellTerminalSessions(conn) {
    if (!conn || !conn.id) return;
    var tabsEl = document.getElementById('webshell-terminal-sessions');
    if (!tabsEl) return;
    var connId = conn.id;
    var state = getWebshellTerminalSessions(connId);
    var HTML = '';
    state.sessions.forEach(function (s) {
        var active = s.id === state.activeId;
        HTML += '<div class="webshell-terminal-session' + (active ? ' active' : '') + '">' +
            '<button type="button" class="webshell-terminal-session-main" data-action="switch" data-terminal-idD="' + escapeHtml(s.id) + '">' + escapeHtml(s.name) + '</button>' +
            '<button type="button" class="webshell-terminal-session-close" data-action="close" data-terminal-idD="' + escapeHtml(s.id) + '" title="' + escapeHtml(wsT('common.close') || 'Close') + '">×</button>' +
            '</div>';
    });
    HTML += '<button type="button" class="webshell-terminal-session-add" data-action="add" title="' + escapeHtml(wsT('WebShell.terminalNewWindow') || 'New Terminal') + '">+</button>';
    tabsEl.innerHTML = HTML;
    tabsEl.querySelectorAll('[data-action]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var action = btn.getAttribute('data-action') || '';
            var targetId = btn.getAttribute('data-terminal-idD') || '';
            if (webshellRunning || webshellTerminalrunning) return;
            if (action === 'add') {
                var nextState = getWebshellTerminalSessions(connId);
                var seq = nextState.sessions.length + 1;
                var nextId = 't' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
                var prefix = wsT('WebShell.terminalWindowPrefix') || 'Terminal';
                nextState.sessions.push({ ID: nextId, name: prefix + seq });
                nextState.activeId = nextId;
                saveWebshellTerminalSessions(connId, nextState);
                destroyWebshellTerminal();
                initWebshellTerminal(conn);
                renderWebshellTerminalSessions(conn);
                return;
            }
            if (!targetId) return;
            if (action === 'close') {
                var curr2 = getWebshellTerminalSessions(connId);
                if (curr2.sessions.length <= 1) return;
                var idx2 = curr2.sessions.findIndex(function (s) { return s.id === targetId; });
                if (idx2 < 0) return;
                curr2.sessions.splice(idx2, 1);
                if (curr2.activeId === targetId) {
                    var fallback = curr2.sessions[Math.max(0, idx2 - 1)] || curr2.sessions[0];
                    curr2.activeId = fallback.id;
                }
                saveWebshellTerminalSessions(connId, curr2);
                // Clean up terminal log and history
                var terminalKey = getWebshellTerminalSessionKey(connId, targetId);
                clearWebshellTerminalLog(terminalKey);
                delete webshellHistoryByConn[terminalKey];
                destroyWebshellTerminal();
                initWebshellTerminal(conn);
                renderWebshellTerminalSessions(conn);
                return;
            }
            if (targetId === getActiveWebshellTerminalSessionId(connId)) return;
            var curr = getWebshellTerminalSessions(connId);
            curr.activeId = targetId;
            saveWebshellTerminalSessions(connId, curr);
            destroyWebshellTerminal();
            initWebshellTerminal(conn);
            renderWebshellTerminalSessions(conn);
        });
    });
}

function getWebshellTreeState(conn) {
    var key = safeConnIdForStorage(conn);
    if (!key) return null;
    if (!webshellDirTreeByConn[key]) webshellDirTreeByConn[key] = { '.': [] };
    if (!webshellDirexpandedByConn[key]) webshellDirexpandedByConn[key] = { '.': true };
    if (!webshellDirLoadedByConn[key]) webshellDirLoadedByConn[key] = { '.': false };
    return {
        key: key,
        tree: webshellDirTreeByConn[key],
        expanded: webshellDirexpandedByConn[key],
        loaded: webshellDirLoadedByConn[key]
    };
}

function newWebshellDbProfile(name) {
    var now = Date.now().toString(36);
    var rand = Math.random().toString(36).slice(2, 8);
    return {
        ID: 'dbp_' + now + rand,
        name: name || 'DB-1',
        type: 'MySQL',
        host: '127.0.0.1',
        port: '3306',
        username: 'root',
        password: '',
        DATABASE: '',
        selectedDatabase: '',
        sqlitePath: '/tmp/test.DB',
        SQL: 'SELECT 1;',
        output: '',
        outputIserror: false,
        schema: {}
    };
}

function getWebshellDbStateStorageKey(conn) {
    return 'webshell_db_state_' + safeConnIdForStorage(conn);
}

function normalizeWebshellDbState(rawState) {
    var state = rawState && typeof rawState === 'object' ? rawState : {};
    var profiles = Array.isArray(state.profiles) ? state.profiles.slice() : [];
    if (!profiles.length) profiles = [newWebshellDbProfile('DB-1')];
    profiles = profiles.map(function (p, idx) {
        var BASE = newWebshellDbProfile('DB-' + (idx + 1));
        return Object.assign(BASE, p || {});
    });
    var activeProfileId = state.activeProfileId || '';
    if (!profiles.some(function (p) { return p.id === activeProfileId; })) {
        activeProfileId = profiles[0].id;
    }
    var aiMemo = typeof state.aiMemo === 'string' ? state.aiMemo : '';
    if (aiMemo.length > 100000) aiMemo = aiMemo.slice(0, 100000);
    return { profiles: profiles, activeProfileId: activeProfileId, aiMemo: aiMemo };
}

function getWebshellDbState(conn) {
    var key = getWebshellDbStateStorageKey(conn);
    if (!key) return normalizeWebshellDbState(null);
    if (webshellDbConfigByConn[key]) return webshellDbConfigByConn[key];
    var state = normalizeWebshellDbState(null);
    webshellDbConfigByConn[key] = state;
    return state;
}

function saveWebshellDbState(conn, state) {
    var key = getWebshellDbStateStorageKey(conn);
    if (!key || !state) return;
    var normalized = normalizeWebshellDbState(state);
    webshellDbConfigByConn[key] = normalized;
    if (conn && conn.id) queueWebshellPersistStateSave(conn.id);
}

function getWebshellDbConfig(conn) {
    var state = getWebshellDbState(conn);
    var active = state.profiles.find(function (p) { return p.id === state.activeProfileId; });
    return active || state.profiles[0];
}

function saveWebshellDbConfig(conn, cfg) {
    if (!cfg) return;
    var state = getWebshellDbState(conn);
    var idx = state.profiles.findIndex(function (p) { return p.id === state.activeProfileId; });
    if (idx < 0) idx = 0;
    state.profiles[idx] = Object.assign({}, state.profiles[idx], cfg);
    state.activeProfileId = state.profiles[idx].id;
    saveWebshellDbState(conn, state);
}

function getWebshellAiMemo(conn) {
    var state = getWebshellDbState(conn);
    return typeof state.aiMemo === 'string' ? state.aiMemo : '';
}

function saveWebshellAiMemo(conn, text) {
    var state = getWebshellDbState(conn);
    state.aiMemo = String(text || '');
    if (state.aiMemo.length > 100000) state.aiMemo = state.aiMemo.slice(0, 100000);
    saveWebshellDbState(conn, state);
}

function webshellDbGetFieldValue(ID) {
    var el = document.getElementById(ID);
    return el && typeof el.value === 'string' ? el.value.trim() : '';
}

function webshellDbCollectConfig(conn) {
    var curr = getWebshellDbConfig(conn) || {};
    var nameVal = webshellDbGetFieldValue('webshell-DB-profile-name');
    var cfg = {
        name: nameVal || curr.name || 'DB-1',
        type: webshellDbGetFieldValue('webshell-DB-type') || 'MySQL',
        host: webshellDbGetFieldValue('webshell-DB-host') || '127.0.0.1',
        port: webshellDbGetFieldValue('webshell-DB-port') || '',
        username: webshellDbGetFieldValue('webshell-DB-user') || '',
        password: (document.getElementById('webshell-DB-pass') || {}).value || '',
        DATABASE: webshellDbGetFieldValue('webshell-DB-name') || '',
        selectedDatabase: curr.selectedDatabase || '',
        sqlitePath: webshellDbGetFieldValue('webshell-DB-SQLite-path') || '/tmp/test.DB',
        SQL: (document.getElementById('webshell-DB-SQL') || {}).value || ''
    };
    saveWebshellDbConfig(conn, cfg);
    return cfg;
}

function webshellDbUpdateFieldVisibility() {
    var type = webshellDbGetFieldValue('webshell-DB-type') || 'MySQL';
    var isSqlite = type === 'SQLite';
    var blocks = document.querySelectorAll('.webshell-DB-common-field');
    blocks.forEach(function (el) { el.style.display = isSqlite ? 'none' : ''; });
    var sqliteBlock = document.getElementById('webshell-DB-SQLite-row');
    if (sqliteBlock) sqliteBlock.style.display = isSqlite ? '' : 'none';
    var portEl = document.getElementById('webshell-DB-port');
    if (portEl && !String(portEl.value || '').trim()) {
        if (type === 'MySQL') portEl.value = '3306';
        else if (type === 'pgsql') portEl.value = '5432';
        else if (type === 'mssql') portEl.value = '1433';
    }
}

function webshellDbSetOutput(text, isError) {
    var outputEl = document.getElementById('webshell-DB-output');
    if (!outputEl) return;
    outputEl.textContent = text || '';
    outputEl.classList.toggle('error', !!isError);
}

function webshellDbRenderTable(rawOutput) {
    var wrap = document.getElementById('webshell-DB-result-TABLE');
    if (!wrap) return false;
    var raw = String(rawOutput || '').trim();
    if (!raw) {
        wrap.innerHTML = '';
        return false;
    }
    var lines = raw.split(/\r?\n/).filter(function (line) {
        var t = String(line || '').trim();
        if (!t) return false;
        if (/^\(\d+\s+rows?\)$/i.test(t)) return false;
        if (/^-{3,}$/.test(t)) return false;
        return true;
    });
    if (lines.length < 2) {
        wrap.innerHTML = '';
        return false;
    }
    var delimiter = lines[0].indexOf('\t') >= 0 ? '\t' : (lines[0].indexOf('|') >= 0 ? '|' : '');
    if (!delimiter) {
        wrap.innerHTML = '';
        return false;
    }
    var header = lines[0].split(delimiter).map(function (s) { return String(s || '').trim(); });
    if (!header.length || (header.length === 1 && !header[0])) {
        wrap.innerHTML = '';
        return false;
    }
    var rows = [];
    for (var i = 1; i < lines.length; i++) {
        var line = lines[i];
        if (/^[-+\s|]+$/.test(line)) continue;
        var cols = line.split(delimiter).map(function (s) { return String(s || '').trim(); });
        if (cols.length !== header.length) continue;
        rows.push(cols);
    }
    if (!rows.length) {
        wrap.innerHTML = '';
        return false;
    }
    var maxRows = Math.min(rows.length, 200);
    var HTML = '<TABLE class="webshell-DB-TABLE"><thead><tr>';
    header.forEach(function (h) { HTML += '<th>' + escapeHtml(h || '-') + '</th>'; });
    HTML += '</tr></thead><tbody>';
    for (var r = 0; r < maxRows; r++) {
        HTML += '<tr>';
        rows[r].forEach(function (c) { HTML += '<td>' + escapeHtml(c || '') + '</td>'; });
        HTML += '</tr>';
    }
    HTML += '</tbody></TABLE>';
    if (rows.length > maxRows) {
        HTML += '<div class="webshell-db-table-meta">Showing first ' + maxRows + ' rows, total ' + rows.length + ' rows</div>';
    } else {
        HTML += '<div class="webshell-db-table-meta">Total ' + rows.length + ' rows, ' + header.length + ' columns</div>';
    }
    wrap.innerHTML = HTML;
    return true;
}

function webshellDbQuoteIdentifier(type, name) {
    var v = String(name || '');
    if (!v) return '';
    if (type === 'MySQL') return '`' + v.replace(/`/g, '``') + '`';
    if (type === 'mssql') return '[' + v.replace(/]/g, ']]') + ']';
    return '"' + v.replace(/"/g, '""') + '"';
}

function webshellDbQuoteLiteral(value) {
    return "'" + String(value == null ? '' : value).replace(/'/g, "''") + "'";
}

function buildWebshellDbCommand(cfg, isTestOnly, options) {
    options = options || {};
    var type = cfg.type || 'MySQL';
    var SQL = String(isTestOnly ? 'SELECT 1;' : (options.SQL || cfg.SQL || '')).trim();
    if (!SQL) return { error: wsT('WebShell.dbSqlRequired') || 'Please enter SQL' };

    var sqlB64 = btoa(unescape(encodeURIComponent(SQL)));
    var sqlB64Arg = escapeSingleQuotedShellArg(sqlB64);
    var tmpFile = '/tmp/.csai_sql_$$.SQL';
    var decodeToFile = 'printf %s ' + sqlB64Arg + " | base64 -d > " + tmpFile;
    var cleanup = '; rc=$?; rm -f ' + tmpFile + '; echo "__CSAI_DB_RC__:$rc"; exit $rc';
    var command = '';

    if (type === 'MySQL') {
        var host = escapeSingleQuotedShellArg(cfg.host || '127.0.0.1');
        var port = escapeSingleQuotedShellArg(cfg.port || '3306');
        var user = escapeSingleQuotedShellArg(cfg.username || 'root');
        var pass = escapeSingleQuotedShellArg(cfg.password || '');
        var dbName = cfg.selectedDatabase || cfg.DATABASE || '';
        var DB = dbName ? (' -D ' + escapeSingleQuotedShellArg(dbName)) : '';
        command = decodeToFile + '; MYSQL_PWD=' + pass + ' MySQL -h ' + host + ' -P ' + port + ' -u ' + user + DB + ' --batch --raw < ' + tmpFile + cleanup;
    } else if (type === 'pgsql') {
        var pHost = escapeSingleQuotedShellArg(cfg.host || '127.0.0.1');
        var pPort = escapeSingleQuotedShellArg(cfg.port || '5432');
        var pUser = escapeSingleQuotedShellArg(cfg.username || 'postgres');
        var pPass = escapeSingleQuotedShellArg(cfg.password || '');
        var pDb = escapeSingleQuotedShellArg(cfg.selectedDatabase || cfg.DATABASE || 'postgres');
        command = decodeToFile + '; PGPASSWORD=' + pPass + ' psql -h ' + pHost + ' -p ' + pPort + ' -U ' + pUser + ' -d ' + pDb + ' -v ON_ERROR_STOP=1 -A -F "|" -P footer=off -f ' + tmpFile + cleanup;
    } else if (type === 'SQLite') {
        var sqlitePath = escapeSingleQuotedShellArg(cfg.sqlitePath || '/tmp/test.DB');
        command = decodeToFile + '; sqlite3 -header -separator "|" ' + sqlitePath + ' < ' + tmpFile + cleanup;
    } else if (type === 'mssql') {
        var sHost = cfg.host || '127.0.0.1';
        var sPort = cfg.port || '1433';
        var sUser = escapeSingleQuotedShellArg(cfg.username || 'sa');
        var sPass = escapeSingleQuotedShellArg(cfg.password || '');
        var sDb = escapeSingleQuotedShellArg(cfg.selectedDatabase || cfg.DATABASE || 'master');
        var server = escapeSingleQuotedShellArg(sHost + ',' + sPort);
        command = decodeToFile + '; sqlcmd -S ' + server + ' -U ' + sUser + ' -P ' + sPass + ' -W -s "|" -d ' + sDb + ' -i ' + tmpFile + cleanup;
    } else {
        return { error: (wsT('WebShell.dbExecFailed') || 'Database execution failed') + ': unsupported type ' + type };
    }

    return { command: command };
}

function buildWebshellDbSchemaCommand(cfg) {
    var type = cfg.type || 'MySQL';
    var schemaSQL = '';
    if (type === 'MySQL') {
        schemaSQL = "SELECT SCHEMA_NAME AS DB_NAME, '' AS TABLE_NAME, '' AS COLUMN_NAME FROM INFORMATION_SCHEMA.SCHEMATA UNION ALL SELECT TABLE_SCHEMA AS DB_NAME, TABLE_NAME AS TABLE_NAME, '' AS COLUMN_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE='BASE TABLE' UNION ALL SELECT TABLE_SCHEMA AS DB_NAME, TABLE_NAME AS TABLE_NAME, COLUMN_NAME AS COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS ORDER BY DB_NAME, TABLE_NAME, COLUMN_NAME;";
    } else if (type === 'pgsql') {
        schemaSQL = "SELECT TABLE_SCHEMA AS DB_NAME, TABLE_NAME, '' AS COLUMN_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE='BASE TABLE' AND TABLE_SCHEMA NOT IN ('pg_catalog','INFORMATION_SCHEMA') UNION ALL SELECT TABLE_SCHEMA AS DB_NAME, TABLE_NAME, COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA NOT IN ('pg_catalog','INFORMATION_SCHEMA') ORDER BY DB_NAME, TABLE_NAME, COLUMN_NAME;";
    } else if (type === 'SQLite') {
        schemaSQL = "SELECT 'main' AS DB_NAME, name AS TABLE_NAME, '' AS COLUMN_NAME FROM sqlite_master WHERE type='TABLE' AND name NOT LIKE 'sqlite_%' UNION ALL SELECT 'main' AS DB_NAME, m.name AS TABLE_NAME, p.name AS COLUMN_NAME FROM sqlite_master m JOIN pragma_table_info(m.name) p ON 1=1 WHERE m.type='TABLE' AND m.name NOT LIKE 'sqlite_%' ORDER BY DB_NAME, TABLE_NAME, COLUMN_NAME;";
    } else if (type === 'mssql') {
        schemaSQL = "SELECT TABLE_SCHEMA AS DB_NAME, TABLE_NAME AS TABLE_NAME, '' AS COLUMN_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE='BASE TABLE' UNION ALL SELECT TABLE_SCHEMA AS DB_NAME, TABLE_NAME AS TABLE_NAME, COLUMN_NAME AS COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS ORDER BY DB_NAME, TABLE_NAME, COLUMN_NAME;";
    } else {
        return { error: (wsT('WebShell.dbExecFailed') || 'Database execution failed') + ': unsupported type ' + type };
    }
    return buildWebshellDbCommand(cfg, false, { SQL: schemaSQL });
}

function buildWebshellDbColumnsCommand(cfg, dbName, tableName) {
    var type = cfg.type || 'MySQL';
    var SQL = '';
    if (type === 'MySQL') {
        SQL = "SELECT COLUMN_NAME AS COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=" + webshellDbQuoteLiteral(dbName) + " AND TABLE_NAME=" + webshellDbQuoteLiteral(tableName) + " ORDER BY ORDINAL_POSITION;";
    } else if (type === 'pgsql') {
        SQL = "SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=" + webshellDbQuoteLiteral(dbName) + " AND TABLE_NAME=" + webshellDbQuoteLiteral(tableName) + " ORDER BY ORDINAL_POSITION;";
    } else if (type === 'SQLite') {
        SQL = "SELECT name AS COLUMN_NAME FROM pragma_table_info(" + webshellDbQuoteLiteral(tableName) + ") ORDER BY cid;";
    } else if (type === 'mssql') {
        SQL = "SELECT COLUMN_NAME AS COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=" + webshellDbQuoteLiteral(dbName) + " AND TABLE_NAME=" + webshellDbQuoteLiteral(tableName) + " ORDER BY ORDINAL_POSITION;";
    } else {
        return { error: (wsT('WebShell.dbExecFailed') || 'Database execution failed') + ': unsupported type ' + type };
    }
    return buildWebshellDbCommand(cfg, false, { SQL: SQL });
}

function buildWebshellDbColumnsByDatabaseCommand(cfg, dbName) {
    var type = cfg.type || 'MySQL';
    var SQL = '';
    if (type === 'MySQL') {
        SQL = "SELECT TABLE_NAME AS TABLE_NAME, COLUMN_NAME AS COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=" + webshellDbQuoteLiteral(dbName) + " ORDER BY TABLE_NAME, ORDINAL_POSITION;";
    } else if (type === 'pgsql') {
        SQL = "SELECT TABLE_NAME, COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=" + webshellDbQuoteLiteral(dbName) + " ORDER BY TABLE_NAME, ORDINAL_POSITION;";
    } else if (type === 'SQLite') {
        SQL = "SELECT m.name AS TABLE_NAME, p.name AS COLUMN_NAME FROM sqlite_master m JOIN pragma_table_info(m.name) p ON 1=1 WHERE m.type='TABLE' AND m.name NOT LIKE 'sqlite_%' ORDER BY m.name, p.cid;";
    } else if (type === 'mssql') {
        SQL = "SELECT TABLE_NAME AS TABLE_NAME, COLUMN_NAME AS COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=" + webshellDbQuoteLiteral(dbName) + " ORDER BY TABLE_NAME, ORDINAL_POSITION;";
    } else {
        return { error: (wsT('WebShell.dbExecFailed') || 'Database execution failed') + ': unsupported type ' + type };
    }
    return buildWebshellDbCommand(cfg, false, { SQL: SQL });
}

function parseWebshellDbExecOutput(rawOutput) {
    var raw = String(rawOutput || '');
    var rc = null;
    var cleaned = raw.replace(/__CSAI_DB_RC__:(\d+)\s*$/m, function (_, code) {
        rc = parseInt(code, 10);
        return '';
    }).trim();
    return { rc: rc, output: cleaned };
}

function parseWebshellDbSchema(rawOutput) {
    var text = String(rawOutput || '').trim();
    if (!text) return {};
    var lines = text.split(/\r?\n/).filter(function (line) {
        return line && line.trim() && !/^\(\d+\s+rows?\)$/i.test(line.trim()) && !/^[-+\s|]+$/.test(line.trim());
    });
    if (lines.length < 2) return {};
    var delimiter = lines[0].indexOf('\t') >= 0 ? '\t' : (lines[0].indexOf('|') >= 0 ? '|' : '');
    if (!delimiter) return {};
    var headers = lines[0].split(delimiter).map(function (s) { return String(s || '').trim().toLowerCase(); });
    var dbIdx = headers.indexOf('DB_NAME');
    var tableIdx = headers.indexOf('TABLE_NAME');
    var columnIdx = headers.indexOf('COLUMN_NAME');
    if (dbIdx < 0 || tableIdx < 0) return {};
    var schema = {};
    for (var i = 1; i < lines.length; i++) {
        var cols = lines[i].split(delimiter).map(function (s) { return String(s || '').trim(); });
        if (cols.length !== headers.length) continue;
        var DB = cols[dbIdx] || 'default';
        var TABLE = cols[tableIdx] || '';
        var column = columnIdx >= 0 ? (cols[columnIdx] || '') : '';
        if (!schema[DB]) schema[DB] = { TABLES: {} };
        if (!TABLE) continue;
        if (!schema[DB].TABLES[TABLE]) schema[DB].TABLES[TABLE] = [];
        if (column && schema[DB].TABLES[TABLE].indexOf(column) < 0) {
            schema[DB].TABLES[TABLE].push(column);
        }
    }
    return normalizeWebshellDbSchema(schema);
}

function parseWebshellDbColumns(rawOutput) {
    var text = String(rawOutput || '').trim();
    if (!text) return [];
    var lines = text.split(/\r?\n/).filter(function (line) {
        return line && line.trim() && !/^\(\d+\s+rows?\)$/i.test(line.trim()) && !/^[-+\s|]+$/.test(line.trim());
    });
    if (lines.length < 2) return [];
    var delimiter = lines[0].indexOf('\t') >= 0 ? '\t' : (lines[0].indexOf('|') >= 0 ? '|' : '');
    if (!delimiter) {
        if (String(lines[0] || '').trim().toLowerCase() !== 'COLUMN_NAME') return [];
        var plainColumns = [];
        for (var p = 1; p < lines.length; p++) {
            var plainName = String(lines[p] || '').trim();
            if (!plainName || plainColumns.indexOf(plainName) >= 0) continue;
            plainColumns.push(plainName);
        }
        return plainColumns;
    }
    var headers = lines[0].split(delimiter).map(function (s) { return String(s || '').trim().toLowerCase(); });
    var colIdx = headers.indexOf('COLUMN_NAME');
    if (colIdx < 0) return [];
    var COLUMNS = [];
    for (var i = 1; i < lines.length; i++) {
        var cols = lines[i].split(delimiter).map(function (s) { return String(s || '').trim(); });
        if (cols.length !== headers.length) continue;
        var name = cols[colIdx] || '';
        if (!name || COLUMNS.indexOf(name) >= 0) continue;
        COLUMNS.push(name);
    }
    return COLUMNS;
}

function parseWebshellDbTableColumns(rawOutput) {
    var text = String(rawOutput || '').trim();
    if (!text) return {};
    var lines = text.split(/\r?\n/).filter(function (line) {
        return line && line.trim() && !/^\(\d+\s+rows?\)$/i.test(line.trim()) && !/^[-+\s|]+$/.test(line.trim());
    });
    if (lines.length < 2) return {};
    var delimiter = lines[0].indexOf('\t') >= 0 ? '\t' : (lines[0].indexOf('|') >= 0 ? '|' : '');
    if (!delimiter) return {};
    var headers = lines[0].split(delimiter).map(function (s) { return String(s || '').trim().toLowerCase(); });
    var tableIdx = headers.indexOf('TABLE_NAME');
    var colIdx = headers.indexOf('COLUMN_NAME');
    if (tableIdx < 0 || colIdx < 0) return {};
    var tableColumns = {};
    for (var i = 1; i < lines.length; i++) {
        var cols = lines[i].split(delimiter).map(function (s) { return String(s || '').trim(); });
        if (cols.length !== headers.length) continue;
        var tableName = cols[tableIdx] || '';
        var colName = cols[colIdx] || '';
        if (!tableName || !colName) continue;
        if (!tableColumns[tableName]) tableColumns[tableName] = [];
        if (tableColumns[tableName].indexOf(colName) >= 0) continue;
        tableColumns[tableName].push(colName);
    }
    return tableColumns;
}

function normalizeWebshellDbSchema(rawSchema) {
    if (!rawSchema || typeof rawSchema !== 'object') return {};
    var normalized = {};
    Object.keys(rawSchema).forEach(function (dbName) {
        var dbEntry = rawSchema[dbName];
        var tableMap = {};

        if (Array.isArray(dbEntry)) {
            dbEntry.forEach(function (tableName) {
                var t = String(tableName || '').trim();
                if (!t) return;
                if (!tableMap[t]) tableMap[t] = [];
            });
        } else if (dbEntry && typeof dbEntry === 'object') {
            var tablesSource = (dbEntry.TABLES && typeof dbEntry.TABLES === 'object') ? dbEntry.TABLES : dbEntry;
            Object.keys(tablesSource).forEach(function (tableName) {
                if (tableName === 'TABLES') return;
                var t = String(tableName || '').trim();
                if (!t) return;
                var rawColumns = tablesSource[tableName];
                var COLUMNS = Array.isArray(rawColumns) ? rawColumns : [];
                var uniqColumns = [];
                COLUMNS.forEach(function (colName) {
                    var c = String(colName || '').trim();
                    if (!c || uniqColumns.indexOf(c) >= 0) return;
                    uniqColumns.push(c);
                });
                uniqColumns.sort(function (a, b) { return a.localeCompare(b); });
                tableMap[t] = uniqColumns;
            });
        }

        var sortedTables = {};
        Object.keys(tableMap).sort(function (a, b) { return a.localeCompare(b); }).forEach(function (tableName) {
            sortedTables[tableName] = tableMap[tableName];
        });
        normalized[dbName] = { TABLES: sortedTables };
    });
    return normalized;
}

function simplifyWebshellAiError(rawMessage) {
    var msg = String(rawMessage || '').trim();
    var lower = msg.toLowerCase();
    if ((lower.indexOf('401') !== -1 || lower.indexOf('unauthorized') !== -1) &&
        (lower.indexOf('API key') !== -1 || lower.indexOf('apikey') !== -1)) {
        return 'Authentication failed: API key not configured or invalid (401)';
    }
    if (lower.indexOf('timeout') !== -1 || lower.indexOf('timed out') !== -1) {
        return 'Request timed out, please try again later';
    }
    if (lower.indexOf('network') !== -1 || lower.indexOf('failed to fetch') !== -1) {
        return 'Network error, please verify service connectivity';
    }
    return msg || 'Request failed';
}

function renderWebshellAiErrorMessage(targetEl, rawMessage) {
    if (!targetEl) return;
    var full = String(rawMessage || '').trim();
    var shortMsg = simplifyWebshellAiError(full);
    targetEl.classList.add('webshell-AI-msg-error');
    targetEl.innerHTML = '';
    var head = document.createElement('div');
    head.className = 'webshell-AI-error-head';
    head.textContent = shortMsg;
    targetEl.appendChild(head);
    if (full && full !== shortMsg) {
        var detail = document.createElement('details');
        detail.className = 'webshell-AI-error-detail';
        var summary = document.createElement('summary');
        summary.textContent = 'View error details';
        var pre = document.createElement('pre');
        pre.textContent = full;
        detail.appendChild(summary);
        detail.appendChild(pre);
        targetEl.appendChild(detail);
    }
}

function isLikelyWebshellAiErrorMessage(content, msg) {
    var text = String(content || '').trim();
    if (!text) return false;
    var lower = text.toLowerCase();
    if (/^(executefailed|Request failed|request error|error)\s*[:: ]/i.test(text)) return true;
    if (/(status code\s*:\s*4\d{2}|unauthorized|forbidden|apikey|API key|invalid API key)/i.test(lower)) return true;
    if (/(noderunerror|tool[-_ ]?error|agent[-_ ]?error|executefailed)/i.test(lower)) return true;
    var details = msg && Array.isArray(msg.processDetails) ? msg.processDetails : [];
    return details.some(function (d) { return String((d && d.eventType) || '').toLowerCase() === 'error'; });
}

function formatWebshellAiConvDate(updatedAt) {
    if (!updatedAt) return '';
    var d = typeof updatedAt === 'string' ? new Date(updatedAt) : updatedAt;
    if (isNaN(d.getTime())) return '';
    var now = new Date();
    var sameDay = d.getDate() === now.getDate() && d.getMonth() === now.getMonth() && d.getFullYear() === now.getFullYear();
    if (sameDay) return d.getHours() + ':' + String(d.getMinutes()).padStart(2, '0');
    return (d.getMonth() + 1) + '/' + d.getDate();
}

function webshellAgentPx(data) {
    if (!data || data.einoAgent == null) return '';
    var s = String(data.einoAgent).trim();
    return s ? ('[' + s + '] ') : '';
}

function formatWebshellCompactInteger(value) {
    var n = Number(value || 0);
    if (!Number.isFinite(n)) return '0';
    try {
        var locale = (typeof getCurrentTimeLocale === 'function') ? getCurrentTimeLocale() : undefined;
        return Math.max(0, Math.trunc(n)).toLocaleString(locale);
    } catch (e) {
        return String(Math.max(0, Math.trunc(n)));
    }
}

function formatWebshellEinoUsageSummaryTitle(data) {
    var d = data && typeof data === 'object' ? data : {};
    var BASE = wsTOr('chat.einoUsageSummaryTitle', 'token usage summary');
    var total = Number(d.totalTokens || 0);
    if (Number.isFinite(total) && total > 0) {
        return BASE + ' · ' + formatWebshellCompactInteger(total);
    }
    return BASE;
}

function formatWebshellEinoUsageSummaryMessage(data) {
    var d = data && typeof data === 'object' ? data : {};
    var rows = [
        [wsTOr('chat.einoUsageModelCalls', 'model calls'), d.modelCalls],
        [wsTOr('chat.einoUsagePromptTokens', 'input tokens'), d.promptTokens],
        [wsTOr('chat.einoUsageCompletionTokens', 'output tokens'), d.completionTokens],
        [wsTOr('chat.einoUsageTotalTokens', 'total tokens'), d.totalTokens]
    ];
    if (Number(d.cachedTokens || 0) > 0) {
        rows.push([wsTOr('chat.einoUsageCachedTokens', 'cached tokens'), d.cachedTokens]);
    }
    if (Number(d.reasoningTokens || 0) > 0) {
        rows.push([wsTOr('chat.einoUsageReasoningTokens', 'reasoning tokens'), d.reasoningTokens]);
    }
    return rows.map(function (row) {
        return row[0] + ': ' + formatWebshellCompactInteger(row[1]);
    }).JOIN('\n');
}

// Build timeline items HTML from processDetails
function buildWebshellTimelineItemFromDetail(detail) {
    var eventType = detail.eventType || '';
    var title = detail.message || '';
    var data = detail.data || {};
    var ap = webshellAgentPx(data);
    if (eventType === 'iteration') {
        title = ap + ((typeof window.t === 'function') ? window.t('chat.iterationRound', { n: data.iteration || 1 }) : ('Round ' + (data.iteration || 1) + ' iteration'));
    } else if (eventType === 'thinking') {
        title = ap + '🤔 ' + ((typeof window.t === 'function') ? window.t('chat.aiThinking') : 'AI Thinking');
    } else if (eventType === 'reasoning_chain') {
        title = ap + '🔗 ' + ((typeof window.t === 'function') ? window.t('chat.reasoningChain') : 'reasoning process');
    } else if (eventType === 'tool_calls_detected') {
        title = ap + '🔧 ' + ((typeof window.t === 'function') ? window.t('chat.toolCallsDetected', { count: data.count || 0 }) : ('Detected ' + (data.count || 0) + ' tool calls'));
    } else if (eventType === 'tool_call') {
        var tn = data.toolName || ((typeof window.t === 'function') ? window.t('chat.unknownTool') : 'unknown tool');
        var idx = data.index || 0;
        var total = data.total || 0;
        var wsCallTitle = typeof window.formatToolCallTimelineTitle === 'function'
            ? window.formatToolCallTimelineTitle(tn, idx, total)
            : ((typeof window.t === 'function') ? window.t('chat.callTool', { name: tn, index: idx, total: total }) : ('Call: ' + tn + (total ? ' (' + idx + '/' + total + ')' : '')));
        title = ap + '🔧 ' + wsCallTitle;
    } else if (eventType === 'tool_result') {
        var tname = data.toolName || 'tool';
        var wsNoResultText = (typeof window.t === 'function') ? window.t('timeline.noResult') : 'No result';
        var wsResult = data.result != null ? data.result : (data.error != null ? data.error : (data.resultPreview != null ? data.resultPreview : wsNoResultText));
        var wsResultStr = (typeof wsResult === 'string') ? wsResult : JSON.stringify(wsResult);
        var wsDisplayState = (typeof window.getToolResultDisplayState === 'function')
            ? window.getToolResultDisplayState(data, { rawText: wsResultStr })
            : { kind: ((data.isError || data.success === false) ? 'error' : 'success'), isError: (data.isError || data.success === false) };
        var wsreturngroundrunning = wsDisplayState.kind === 'background_running';
        var success = !wsDisplayState.isError && !wsreturngroundrunning;
        var wsIcon = wsDisplayState.kind === 'blocked' ? '🛡 ' : (wsreturngroundrunning ? '⏳ ' : (success ? '✅ ' : '❌ '));
        var wsLabel = wsDisplayState.kind === 'blocked'
            ? ((typeof window.t === 'function') ? window.t('chat.toolExecBlocked', { name: tname }) : tname + ' Blocked')
            : wsreturngroundrunning
            ? (((typeof window.getBackgroundRunningToolLabel === 'function') ? window.getBackgroundRunningToolLabel() : 'Running in background') + ': ' + tname)
            : ((typeof window.t === 'function') ? (success ? window.t('chat.toolExecComplete', { name: tname }) : window.t('chat.toolExecFailed', { name: tname })) : (tname + (success ? ' executecomplete' : ' executefailed')));
        title = ap + wsIcon + wsLabel;
    } else if (eventType === 'eino_agent_reply') {
        title = ap + '💬 ' + ((typeof window.t === 'function') ? window.t('chat.einoAgentReplyTitle') : 'sub-agent reply');
    } else if (eventType === 'eino_usage_summary') {
        title = ap + '📊 ' + formatWebshellEinoUsageSummaryTitle(data);
    } else if (eventType === 'progress') {
        title = (typeof window.translateProgressMessage === 'function') ? window.translateProgressMessage(detail.message || '') : (detail.message || '');
    }
    var HTML = '<span class="webshell-AI-timeline-title">' + escapeHtml(title || '') + '</span>';
    if (eventType === 'eino_agent_reply' && detail.message) {
        HTML += '<div class="webshell-AI-timeline-msg"><pre style="white-space:pre-wrap;">' + escapeHtml(detail.message) + '</pre></div>';
    }
    if (eventType === 'eino_usage_summary') {
        HTML += '<div class="webshell-AI-timeline-msg"><pre style="white-space:pre-wrap;">' + escapeHtml(formatWebshellEinoUsageSummaryMessage(data)) + '</pre></div>';
    }
    if (eventType === 'tool_call' && data && (data.argumentsObj || data.arguments)) {
        try {
            var args = data.argumentsObj;
            if (args == null && data.arguments != null && String(data.arguments).trim() !== '') {
                try {
                    args = JSON.parse(String(data.arguments));
                } catch (e2) {
                    args = { _raw: String(data.arguments) };
                }
            }
            if (args && typeof args === 'object') {
                var paramsLabel = (typeof window.t === 'function') ? window.t('timeline.params') : 'Parameters:';
                HTML += '<div class="webshell-AI-timeline-msg"><div class="tool-arg-section"><strong>' + escapeHtml(paramsLabel) + '</strong><pre class="tool-args">' + escapeHtml(JSON.stringify(args, null, 2)) + '</pre></div></div>';
            }
        } catch (e) {}
    }
    if (eventType === 'tool_call' && data && data._mergedResult && typeof window.buildToolResultSectionHtml === 'function') {
        HTML += '<div class="webshell-AI-timeline-msg tool-result-slot">' + window.buildToolResultSectionHtml(data._mergedResult) + '</div>';
    } else if (eventType === 'tool_result' && data) {
        var noResultText = (typeof window.t === 'function') ? window.t('timeline.noResult') : 'No result';
        var result = data.result != null ? data.result : (data.error != null ? data.error : (data.resultPreview != null ? data.resultPreview : noResultText));
        var resultStr = (typeof result === 'string') ? result : JSON.stringify(result);
        var displayState = (typeof window.getToolResultDisplayState === 'function')
            ? window.getToolResultDisplayState(data, { rawText: resultStr })
            : { kind: ((data.isError || data.success === false) ? 'error' : 'success'), isError: (data.isError || data.success === false) };
        var execResultLabel = (typeof window.t === 'function') ? window.t('timeline.executionResult') : 'Result:';
        var execIdLabel = (typeof window.t === 'function') ? window.t('timeline.executionId') : 'executeID:';
        var sectionClass = displayState.kind === 'blocked' ? 'blocked' : (displayState.kind === 'background_running' ? 'pending' : (displayState.isError ? 'error' : 'success'));
        HTML += '<div class="webshell-AI-timeline-msg"><div class="tool-result-section ' + sectionClass + '"><strong>' + escapeHtml(execResultLabel) + '</strong><pre class="tool-result">' + escapeHtml(resultStr) + '</pre>' + (data.executionId ? '<div class="tool-execution-ID"><span>' + escapeHtml(execIdLabel) + '</span> <code>' + escapeHtml(String(data.executionId)) + '</code></div>' : '') + '</div></div>';
    } else if (eventType !== 'eino_usage_summary' && detail.message && detail.message !== title) {
        HTML += '<div class="webshell-AI-timeline-msg">' + escapeHtml(detail.message) + '</div>';
    }
    return HTML;
}

// Render collapsible execution process and tool calls
function renderWebshellProcessDetailsBlock(processDetails, defaultCollapsed) {
    if (!processDetails || processDetails.length === 0) return null;
    if (typeof window.filterNoiseProcessDetails === 'function') {
        processDetails = window.filterNoiseProcessDetails(processDetails);
    }
    if (!processDetails.length) return null;
    if (typeof window.coalesceProcessDetailsToolPairs === 'function') {
        processDetails = window.coalesceProcessDetailsToolPairs(processDetails);
    }
    var expandLabel = (typeof window.t === 'function') ? window.t('chat.expandDetail') : 'expandDetails';
    var collapseLabel = (typeof window.t === 'function') ? window.t('tasks.collapseDetail') : 'collapseDetails';
    var headerLabel = (typeof window.t === 'function') ? (window.t('chat.penetrationTestDetail') || 'taskexecuteDetails') : 'taskexecuteDetails';
    var wrapper = document.createElement('div');
    wrapper.className = 'process-details-container webshell-AI-process-block';
    var collapsed = defaultCollapsed !== false;
    wrapper.innerHTML = '<button type="button" class="webshell-AI-process-toggle" aria-expanded="' + (!collapsed) + '">' + escapeHtml(headerLabel) + ' <span class="ws-toggle-icon">' + (collapsed ? '▶' : '▼') + '</span></button><div class="process-details-content"><div class="progress-timeline webshell-AI-timeline has- items' + (collapsed ? '' : ' expanded') + '"></div></div>';
    var timeline = wrapper.querySelector('.progress-timeline');
    processDetails.forEach(function (d) {
        var item = document.createElement('div');
        item.className = 'webshell-AI-timeline-item webshell-AI-timeline-' + (d.eventType || '');
        item.innerHTML = buildWebshellTimelineItemFromDetail(d);
        timeline.appendChild(item);
    });
    var toggleBtn = wrapper.querySelector('.webshell-AI-process-toggle');
    var toggleIcon = wrapper.querySelector('.ws-toggle-icon');
    toggleBtn.addEventListener('click', function () {
        var isExpanded = timeline.classList.contains('expanded');
        timeline.classList.toggle('expanded');
        toggleBtn.setAttribute('aria-expanded', !isExpanded);
        if (toggleIcon) toggleIcon.textContent = isExpanded ? '▶' : '▼';
    });
    return wrapper;
}

function fetchAndRenderWebshellAiConvList(conn, listEl) {
    if (!conn || !conn.id || !listEl || typeof apiFetch !== 'function') return Promise.resolve();
    return apiFetch('/api/WebShell/connections/' + encodeURIComponent(conn.id) + '/AI-conversations', { method: 'GET' })
        .then(function (r) { return r.json(); })
        .then(function (list) {
            if (!Array.isArray(list)) list = [];
            listEl.innerHTML = '';
            list.forEach(function (item) {
                var row = document.createElement('div');
                row.className = 'webshell-AI-conv-item';
                row.dataset.convId = item.id;
                var title = (item.title || '').trim() || item.id.slice(0, 8);
                var dateStr = item.updatedAt ? formatWebshellAiConvDate(item.updatedAt) : '';
                row.innerHTML = '<span class="webshell-AI-conv-item-title">' + escapeHtml(title) + '</span><span class="webshell-AI-conv-item-date">' + escapeHtml(dateStr) + '</span>';
                if (webshellAiConvMap[conn.id] === item.id) row.classList.add('active');
                row.addEventListener('click', function () {
                    webshellAiConvListSelect(conn, item.id, document.getElementById('webshell-AI-messages'), listEl);
                });
                var delBtn = document.createElement('button');
                delBtn.type = 'button';
                delBtn.className = 'btn-ghost btn-sm webshell-AI-conv-del';
                delBtn.textContent = '×';
                delBtn.title = wsT('WebShell.aiDeleteConversation') || 'deleteChat';
                delBtn.addEventListener('click', function (e) {
                    e.stopPropagation();
                    if (!confirm(wsT('WebShell.aiDeleteConversationConfirm') || 'Delete this chat?')) return;
                    var deletedId = item.id;
                    apiFetch('/api/conversations/' + encodeURIComponent(deletedId), { method: 'DELETE' })
                        .then(function (r) {
                            if (r.ok) {
                                if (webshellAiConvMap[conn.id] === deletedId) {
                                    delete webshellAiConvMap[conn.id];
                                    var msgs = document.getElementById('webshell-AI-messages');
                                    if (msgs) msgs.innerHTML = '';
                                }
                                fetchAndRenderWebshellAiConvList(conn, listEl);
                                try {
                                    document.dispatchEvent(new CustomEvent('conversation-deleted', { detail: { conversationId: deletedId } }));
                                } catch (err) { /* ignore */ }
                            }
                        })
                        .catch(function (e) { console.warn('deleteChatfailed', e); });
                });
                row.appendChild(delBtn);
                listEl.appendChild(row);
            });
        })
        .catch(function (e) { console.warn('Failed to load chat list', e); });
}

function webshellAiConvListSelect(conn, convId, messagesContainer, listEl) {
    if (!conn || !convId || !messagesContainer) return;
    webshellAiConvMap[conn.id] = convId;
    if (listEl) listEl.querySelectorAll('.webshell-AI-conv-item').forEach(function (el) {
        el.classList.toggle('active', el.dataset.convId === convId);
    });
    if (typeof apiFetch !== 'function') return;
    apiFetch('/api/conversations/' + encodeURIComponent(convId) + '?include_process_details=1', { method: 'GET' })
        .then(function (r) { return r.json(); })
        .then(function (data) {
            wsSetWebshellAiProject(conn, data.projectId || data.project_id || '');
            messagesContainer.innerHTML = '';
            var list = data.messages || [];
            list.forEach(function (msg) {
                var role = (msg.role || '').toLowerCase();
                var content = (msg.content || '').trim();
                if (!content && role !== 'assistant') return;
                var div = document.createElement('div');
                div.className = 'webshell-AI-msg ' + (role === 'user' ? 'user' : 'assistant');
                if (role === 'user') {
                    div.textContent = content;
                } else {
                    if (isLikelyWebshellAiErrorMessage(content, msg)) {
                        renderWebshellAiErrorMessage(div, content);
                    } else if (typeof formatMarkdown === 'function') {
                        div.innerHTML = formatMarkdown(content);
                    } else {
                        div.textContent = content;
                    }
                }
                messagesContainer.appendChild(div);
                if (role === 'assistant') {
                    var wsMergedDetails = (typeof window.mergeMessageReasoningContentIntoProcessDetails === 'function')
                        ? window.mergeMessageReasoningContentIntoProcessDetails(msg.processDetails || [], msg.reasoningContent)
                        : (msg.processDetails || []);
                    if (wsMergedDetails.length > 0) {
                        var block = renderWebshellProcessDetailsBlock(wsMergedDetails, true);
                        if (block) messagesContainer.appendChild(block);
                    }
                }
            });
            if (list.length === 0) {
                var readyMsg = wsT('WebShell.aiSystemReadyMessage') || 'System ready. Please enter your test requirements and the system will automatically run the appropriate security test.';
                var readyDiv = document.createElement('div');
                readyDiv.className = 'webshell-AI-msg assistant';
                readyDiv.textContent = readyMsg;
                messagesContainer.appendChild(readyDiv);
            }
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
        })
        .catch(function (e) { console.warn('Failed to load chat', e); });
}

// Select connection: render terminal and file manager tabs
function selectWebshell(ID, stateReady) {
    currentWebshellId = ID;
    renderWebshellList();
    const conn = webshellConnections.find(c => c.id === ID);
    const workspace = document.getElementById('webshell-workspace');
    if (!workspace) return;
    if (!conn) {
        workspace.innerHTML = '<div class="webshell-workspace-placeholder">' + wsT('WebShell.selectOrAdd') + '</div>';
        return;
    }
    if (!stateReady) {
        ensureWebshellPersistStateLoaded(conn).then(function () {
            if (currentWebshellId === ID) selectWebshell(ID, true);
        });
        return;
    }

    destroyWebshellTerminal();
    webshellCurrentConn = conn;

    workspace.innerHTML =
        '<div class="webshell-tabs">' +
        '<button type="button" class="webshell-tab active" data-tab="terminal">' + wsT('WebShell.tabTerminal') + '</button>' +
        '<button type="button" class="webshell-tab" data-tab="file">' + wsT('WebShell.tabFileManager') + '</button>' +
        '<button type="button" class="webshell-tab" data-tab="DB">' + (wsT('WebShell.tabDbManager') || 'Database Manager') + '</button>' +
        '<button type="button" class="webshell-tab" data-tab="AI">' + (wsT('WebShell.tabAiAssistant') || 'AI Assistant') + '</button>' +
        '<button type="button" class="webshell-tab" data-tab="memo">' + (wsT('WebShell.tabMemo') || 'Scratchpad') + '</button>' +
        '</div>' +
        '<div ID="webshell-pane-terminal" class="webshell-pane active">' +
        '<div class="webshell-terminal-toolbar">' +
        '<button type="button" class="btn-ghost btn-sm" id="webshell-terminal-clear" title="' + (wsT('WebShell.clearScreen') || 'Clear Screen') + '">' + (wsT('WebShell.clearScreen') || 'Clear Screen') + '</button> ' +
        '<button type="button" class="btn-ghost btn-sm" id="webshell-terminal-copy-log" title="' + (wsT('WebShell.copyTerminalLog') || 'Copy Terminal Log') + '">' + (wsT('WebShell.copyTerminalLog') || 'Copy Terminal Log') + '</button> ' +
        '<span id="webshell-terminal-status" class="webshell-terminal-status idle">' + (wsT('WebShell.terminalIdle') || 'Idle') + '</span> ' +
        '<span class="webshell-quick-label">' + (wsT('WebShell.quickCommands') || 'Quick Commands') + ':</span> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="whoami">whoami</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="ID">ID</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="pwd">pwd</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="ls -la">ls -la</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="uname -a">uname -a</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="ifconfig">ifconfig</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="ip a">ip a</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="env">env</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="hostname">hostname</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="ps aux">ps aux</button> ' +
        '<button type="button" class="btn-ghost btn-sm webshell-quick-cmd" data-cmd="netstat -tulnp">netstat</button>' +
        '</div>' +
        '<div class="webshell-terminal-shell">' +
        '<div ID="webshell-terminal-sessions" class="webshell-terminal-sessions"></div>' +
        '<div ID="webshell-terminal-container" class="webshell-terminal-container"></div>' +
        '</div>' +
        '</div>' +
        '<div ID="webshell-pane-file" class="webshell-pane">' +
        '<div class="webshell-file-layout">' +
        '<aside class="webshell-file-sidebar">' +
        '<div class="webshell-file-sidebar-title">' + wsTOr('WebShell.dirTree', 'Directory Tree') + '</div>' +
        '<div ID="webshell-dir-tree" class="webshell-dir-tree"></div>' +
        '</aside>' +
        '<section class="webshell-file-main">' +
        '<div class="webshell-file-toolbar">' +
        '<div class="webshell-file-breadcrumb" ID="webshell-file-breadcrumb"></div>' +
        '<div class="webshell-file-toolbar-main">' +
        '<label class="webshell-file-path-field"><span>' + wsT('WebShell.filePath') + '</span> <input type="text" ID="webshell-file-path" class="form-control" value="." /></label>' +
        '<input type="text" ID="webshell-file-filter" class="form-control webshell-file-filter" placeholder="' + (wsT('WebShell.filterPlaceholder') || 'Filter filenames') + '" />' +
        '<button type="button" class="btn-secondary" ID="webshell-list-dir">' + wsT('WebShell.listDir') + '</button>' +
        '<button type="button" class="btn-ghost" ID="webshell-parent-dir">' + wsT('WebShell.parentDir') + '</button>' +
        '</div>' +
        '<div class="webshell-file-toolbar-actions">' +
        '<button type="button" class="btn-ghost" ID="webshell-file-refresh" title="' + (wsT('WebShell.refresh') || 'refresh') + '">' + (wsT('WebShell.refresh') || 'refresh') + '</button>' +
        '<details class="webshell-toolbar-actions">' +
        '<summary class="btn-ghost webshell-toolbar-actions-btn">' + (wsT('WebShell.moreActions') || 'More Actions') + '</summary>' +
        '<div class="webshell-row-actions-menu">' +
        '<button type="button" class="btn-ghost" ID="webshell-mkdir-btn">' + (wsT('WebShell.newDir') || 'New Directory') + '</button>' +
        '<button type="button" class="btn-ghost" ID="webshell-newFile-btn">' + (wsT('WebShell.newFile') || 'New File') + '</button>' +
        '<button type="button" class="btn-ghost" ID="webshell-upload-btn">' + (wsT('WebShell.upload') || 'Upload') + '</button>' +
        '<button type="button" class="btn-ghost" ID="webshell-batch-delete-btn">' + (wsT('WebShell.batchDelete') || 'Batch Delete') + '</button>' +
        '<button type="button" class="btn-ghost" ID="webshell-batch-download-btn">' + (wsT('WebShell.batchDownload') || 'Batch Download') + '</button>' +
        '</div></details>' +
        '</div>' +
        '</div>' +
        '<div ID="webshell-file-list" class="webshell-file-list"></div>' +
        '</section>' +
        '</div>' +
        '</div>' +
        '<div ID="webshell-pane-AI" class="webshell-pane webshell-pane-AI-with-sidebar">' +
        '<div class="webshell-AI-sidebar">' +
        '<button type="button" class="btn-primary btn-sm webshell-AI-new-btn" ID="webshell-AI-new-conv">' + (wsT('WebShell.aiNewConversation') || 'New Chat') + '</button>' +
        '<div class="webshell-AI-conv-list" ID="webshell-AI-conv-list"></div>' +
        '</div>' +
        '<div class="webshell-AI-main">' +
        '<div ID="webshell-AI-messages" class="webshell-AI-messages"></div>' +
        '<div class="webshell-AI-input-area">' +
        '<div class="webshell-AI-selectors-row">' +
        '<div class="ws-project-selector-wrapper project-selector-wrapper">' +
        '<button type="button" ID="ws-project-btn" class="role-selector-btn" onclick="wsToggleProjectPanel()" aria-label="' + escapeHtml(wsProjectT('projects.chatSelectorButton', 'Select project')) + '" aria-haspopup="listbox" aria-expanded="false" title="' + escapeHtml(wsProjectT('projects.chatSelectorButton', 'Shared fact board across chats after project binding')) + '">' +
        '<span class="role-selector-icon" aria-hidden="true">📁</span>' +
        '<span ID="ws-project-text" class="role-selector-text">' + escapeHtml(wsProjectT('projects.noProject', 'No project')) + '</span>' +
        '<svg class="role-selector-arrow" width="10" height="10" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg"><path d="M6 9l6 6 6-6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>' +
        '</button>' +
        '<div ID="ws-project-panel" class="role-selection-panel chat-project-panel" style="display:none;" role="listbox">' +
        '<div class="role-selection-panel-header">' +
        '<h3 class="role-selection-panel-title">' + escapeHtml(wsProjectT('projects.selectProject', 'Select project')) + '</h3>' +
        '<button type="button" class="role-selection-panel-close" onclick="wsCloseProjectPanel()" title="' + escapeHtml(wsProjectT('common.close', 'Close')) + '" aria-label="' + escapeHtml(wsProjectT('common.close', 'Close')) + '">' +
        '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg"><path d="M18 6L6 18M6 6l12 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg></button>' +
        '</div>' +
        '<div class="chat-project-panel-body">' +
        '<div class="chat-project-panel-search">' +
        '<input type="search" ID="ws-project-search" class="chat-project-panel-search-input" autocomplete="off" placeholder="' + escapeHtml(wsProjectT('projects.searchProjectsPlaceholder', 'Search projects...')) + '">' +
        '</div>' +
        '<div ID="ws-project-list" class="role-selection-list-main"></div>' +
        '<div class="chat-project-panel-footer">' +
        ((typeof hasPermission === 'function' && hasPermission('project:write'))
            ? ('<button type="button" class="role-selection-item-main chat-project-panel-create-btn" onclick="showNewProjectModalFromWebshellAi()">' +
                '<span class="chat-project-panel-create-icon" aria-hidden="true">+</span>' +
                '<span class="chat-project-panel-create-label">' + escapeHtml(wsProjectT('projects.newProject', 'New project')) + '</span>' +
                '</button>')
            : '') +
        '</div></div></div></div>' +
        '<div class="ws-role-selector-wrapper">' +
        '<button type="button" class="role-selector-btn ws-role-selector-btn" ID="ws-role-selector-btn" onclick="wsToggleRolePanel()">' +
        '<span ID="ws-role-selector-icon" class="role-selector-icon">\ud83d\udd35</span>' +
        '<span ID="ws-role-selector-text" class="role-selector-text">' + (wsT('chat.defaultRole') || 'default') + '</span>' +
        '<svg class="role-selector-arrow" width="10" height="10" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg"><path d="M6 9l6 6 6-6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>' +
        '</button>' +
        '<div ID="ws-role-selection-panel" class="role-selection-panel" style="display:none;">' +
        '<div class="role-selection-panel-header"><h3 class="role-selection-panel-title">' + (wsT('chat.rolePanelTitle') || 'Select Role') + '</h3>' +
        '<button type="button" class="role-selection-panel-close" onclick="wsCloseRolePanel()"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg"><path d="M18 6L6 18M6 6l12 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg></button>' +
        '</div><div ID="ws-role-selection-list" class="role-selection-list-main"></div></div>' +
        '</div>' +
        '<div class="ws-agent-mode-wrapper" ID="ws-agent-mode-wrapper" style="display:none;">' +
        '<div class="agent-mode-inner">' +
        '<button type="button" class="role-selector-btn agent-mode-btn" ID="ws-agent-mode-btn" onclick="wsToggleAgentModePanel()">' +
        '<span ID="ws-agent-mode-icon" class="role-selector-icon">\ud83e\udd16</span>' +
        '<span ID="ws-agent-mode-text" class="role-selector-text">' + (wsT('chat.agentModeEinoSingle') || 'Eino single-agent') + '</span>' +
        '<svg class="role-selector-arrow" width="10" height="10" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg"><path d="M6 9l6 6 6-6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>' +
        '</button>' +
        '<div ID="ws-agent-mode-panel" class="agent-mode-panel" style="display:none;" role="listbox">' +
        '<div class="role-selection-panel-header agent-mode-panel-header"><h3 class="role-selection-panel-title">' + (wsT('chat.agentModePanelTitle') || 'Chat Mode') + '</h3>' +
        '<button type="button" class="role-selection-panel-close" onclick="wsCloseAgentModePanel()"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="HTTP://www.w3.org/2000/svg"><path d="M18 6L6 18M6 6l12 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg></button>' +
        '</div>' +
        '<div class="agent-mode-options">' +
        '<button type="button" class="role-selection-item-main agent-mode-option ws-agent-mode-option" data-value="eino_single" role="option" onclick="wsSelectAgentMode(\'eino_single\')" data-agent-mode-detail="' + escapeHtmlAttr(wsT('chat.agentModeEinoSingleHint') || 'Eino ChatModelAgent + runner') + '"><div class="role-selection-item-icon-main">\u26a1</div><div class="role-selection-item-content-main"><div class="role-selection-item-name-main">' + (wsT('chat.agentModeEinoSingle') || 'Eino single-agent (ADK)') + '</div><div class="role-selection-item-description-main">' + (wsT('chat.agentModeEinoSingleHint') || 'Eino ChatModelAgent + runner') + '</div></div><div class="role-selection-checkmark-main agent-mode-check" data-agent-mode-check="eino_single">\u2713</div></button>' +
        '<button type="button" class="role-selection-item-main agent-mode-option ws-agent-mode-option" data-value="deep" role="option" onclick="wsSelectAgentMode(\'deep\')" data-agent-mode-detail="' + escapeHtmlAttr(wsT('chat.agentModeDeepHint') || 'Eino DeepAgent, suitable for multi-stage security testing and delegation') + '"><div class="role-selection-item-icon-main">\ud83e\udde9</div><div class="role-selection-item-content-main"><div class="role-selection-item-name-main">' + (wsT('chat.agentModeDeep') || 'Deep (DeepAgent)') + '</div><div class="role-selection-item-description-main">' + (wsT('chat.agentModeDeepHint') || 'Eino DeepAgent, suitable for multi-stage security testing and delegation') + '</div></div><div class="role-selection-checkmark-main agent-mode-check" data-agent-mode-check="deep">\u2713</div></button>' +
        '<button type="button" class="role-selection-item-main agent-mode-option ws-agent-mode-option" data-value="plan_execute" role="option" onclick="wsSelectAgentMode(\'plan_execute\')" data-agent-mode-detail="' + escapeHtmlAttr(wsT('chat.agentModePlanExecuteHint') || 'Plan -> Execute -> Replan') + '"><div class="role-selection-item-icon-main">\ud83d\udccb</div><div class="role-selection-item-content-main"><div class="role-selection-item-name-main">' + (wsT('chat.agentModePlanExecuteLabel') || 'Plan-Execute') + '</div><div class="role-selection-item-description-main">' + (wsT('chat.agentModePlanExecuteHint') || 'Plan -> Execute -> Replan') + '</div></div><div class="role-selection-checkmark-main agent-mode-check" data-agent-mode-check="plan_execute">\u2713</div></button>' +
        '<button type="button" class="role-selection-item-main agent-mode-option ws-agent-mode-option" data-value="supervisor" role="option" onclick="wsSelectAgentMode(\'supervisor\')" data-agent-mode-detail="' + escapeHtmlAttr(wsT('chat.agentModeSupervisorHint') || 'Expert Router scenario: Supervisor dynamically delegates to specialized agents') + '"><div class="role-selection-item-icon-main">\ud83c\udfaf</div><div class="role-selection-item-content-main"><div class="role-selection-item-name-main">' + (wsT('chat.agentModeSupervisorLabel') || 'Supervisor (Expert Router)') + '</div><div class="role-selection-item-description-main">' + (wsT('chat.agentModeSupervisorHint') || 'Expert Router scenario: Supervisor dynamically delegates to specialized agents') + '</div></div><div class="role-selection-checkmark-main agent-mode-check" data-agent-mode-check="supervisor">\u2713</div></button>' +
        '</div></div></div>' +
        '<input type="hidden" ID="ws-agent-mode-SELECT" value="eino_single" autocomplete="off" />' +
        '</div>' +
        '</div>' +
        '<div class="webshell-AI-input-row">' +
        '<textarea ID="webshell-AI-input" class="webshell-AI-input form-control" rows="2" placeholder="' + (wsT('WebShell.aiPlaceholder') || 'e.g.: List files in current directory') + '"></textarea>' +
        '<button type="button" class="btn-primary" ID="webshell-AI-send">' + (wsT('WebShell.aiSend') || 'send') + '</button>' +
        '<button type="button" class="btn-danger webshell-AI-stop-btn" ID="webshell-AI-stop" style="display:none;">' + wsTOr('WebShell.aiStop', 'stop') + '</button>' +
        '</div>' +
        '</div>' +
        '</div>' +
        '</div>' +
        '<div ID="webshell-pane-memo" class="webshell-pane webshell-pane-memo">' +
        '<div class="webshell-memo-layout">' +
        '<div class="webshell-memo-head"><span>' + (wsT('WebShell.aiMemo') || 'Scratchpad') + '</span><button type="button" class="btn-ghost btn-sm" ID="webshell-AI-memo-clear">' + (wsT('WebShell.aiMemoClear') || 'Clear') + '</button></div>' +
        '<textarea ID="webshell-AI-memo-input" class="webshell-memo-input form-control" rows="18" placeholder="' + (wsT('WebShell.aiMemoPlaceholder') || 'Record key commands, test notes, reproduction steps...') + '"></textarea>' +
        '<div ID="webshell-AI-memo-status" class="webshell-memo-status">' + (wsT('WebShell.aiMemoSaved') || 'Saved locally') + '</div>' +
        '</div>' +
        '</div>' +
        '<div ID="webshell-pane-DB" class="webshell-pane webshell-pane-DB">' +
        '<div class="webshell-DB-profiles-bar"><div ID="webshell-DB-profiles" class="webshell-DB-profiles"></div><div class="webshell-DB-profile-actions"><button type="button" class="btn-ghost btn-sm" ID="webshell-DB-add-profile-btn">+ ' + (wsT('WebShell.dbAddProfile') || 'Add Profile') + '</button></div></div>' +
        '<div class="webshell-DB-layout">' +
        '<aside class="webshell-DB-sidebar">' +
        '<div class="webshell-DB-sidebar-head"><span>' + (wsT('WebShell.dbSchema') || 'Database Schema') + '</span><button type="button" class="btn-ghost btn-sm" ID="webshell-DB-load-schema-btn">' + (wsT('WebShell.dbLoadSchema') || 'Load Schema') + '</button></div>' +
        '<div ID="webshell-DB-schema-tree" class="webshell-DB-schema-tree"><div class="webshell-empty">' + (wsT('WebShell.dbNoSchema') || 'No database schema loaded, please load schema first') + '</div></div>' +
        '<div class="webshell-DB-sidebar-hint">' + (wsT('WebShell.dbSelectTableHint') || 'Click table name to generate query SQL') + '</div>' +
        '</aside>' +
        '<section class="webshell-DB-main">' +
        '<div class="webshell-DB-SQL-tools"><button type="button" class="btn-ghost btn-sm" ID="webshell-DB-template-btn">' + (wsT('WebShell.dbTemplateSql') || 'Sample SQL') + '</button><button type="button" class="btn-ghost btn-sm" ID="webshell-DB-clear-btn">' + (wsT('WebShell.dbClearSql') || 'Clear SQL') + '</button></div>' +
        '<textarea ID="webshell-DB-SQL" class="webshell-DB-SQL form-control" rows="8" placeholder="' + (wsT('WebShell.dbSqlPlaceholder') || 'Enter SQL, e.g.: SELECT version();') + '"></textarea>' +
        '<div class="webshell-DB-actions">' +
        '<button type="button" class="btn-ghost" ID="webshell-DB-test-btn">' + (wsT('WebShell.dbTest') || 'Test connection') + '</button>' +
        '<button type="button" class="btn-primary" ID="webshell-DB-run-btn">' + (wsT('WebShell.dbRunSql') || 'execute SQL') + '</button>' +
        '</div>' +
        '<div class="webshell-DB-output-wrap"><div class="webshell-DB-output-title">' + (wsT('WebShell.dbOutput') || 'Execution Output') + '</div><div ID="webshell-DB-result-TABLE" class="webshell-DB-result-TABLE"></div><pre ID="webshell-DB-output" class="webshell-DB-output"></pre><div class="webshell-DB-hint">' + (wsT('WebShell.dbCliHint') || 'If client command is missing, install the corresponding client (MySQL/psql/sqlite3/sqlcmd) on the target host') + '</div></div>' +
        '<div ID="webshell-DB-profile-modal" class="modal">' +
        '<div class="modal-content webshell-DB-profile-modal-content">' +
        '<div class="modal-header"><h2 ID="webshell-DB-profile-modal-title">' + (wsT('WebShell.editConnectionTitle') || 'Edit Connection') + '</h2><span class="modal-close" ID="webshell-DB-profile-modal-close">&times;</span></div>' +
        '<div class="modal-body">' +
        '<div class="webshell-DB-toolbar">' +
        '<label><span>' + (wsT('WebShell.dbProfileName') || 'Connection Name') + '</span><input ID="webshell-DB-profile-name" class="form-control" type="text" maxlength="30" /></label>' +
        '<label><span>' + (wsT('WebShell.dbType') || 'Database Type') + '</span><SELECT ID="webshell-DB-type" class="form-control"><option value="MySQL">MySQL</option><option value="pgsql">PostgreSQL</option><option value="SQLite">SQLite</option><option value="mssql">SQL Server</option></SELECT></label>' +
        '<label class="webshell-DB-common-field"><span>' + (wsT('WebShell.dbHost') || 'host') + '</span><input ID="webshell-DB-host" class="form-control" type="text" value="127.0.0.1" /></label>' +
        '<label class="webshell-DB-common-field"><span>' + (wsT('WebShell.dbPort') || 'port') + '</span><input ID="webshell-DB-port" class="form-control" type="text" /></label>' +
        '<label class="webshell-DB-common-field"><span>' + (wsT('WebShell.dbUsername') || 'Username') + '</span><input ID="webshell-DB-user" class="form-control" type="text" /></label>' +
        '<label class="webshell-DB-common-field"><span>' + (wsT('WebShell.dbPassword') || 'Password') + '</span><input ID="webshell-DB-pass" class="form-control" type="password" /></label>' +
        '<label class="webshell-DB-common-field"><span>' + (wsT('WebShell.dbName') || 'Database Name') + '</span><input ID="webshell-DB-name" class="form-control" type="text" /></label>' +
        '<label ID="webshell-DB-SQLite-row"><span>' + (wsT('WebShell.dbSqlitePath') || 'SQLite File Path') + '</span><input ID="webshell-DB-SQLite-path" class="form-control" type="text" value="/tmp/test.DB" /></label>' +
        '</div>' +
        '</div>' +
        '<div class="modal-footer"><button type="button" class="btn-secondary" ID="webshell-DB-profile-cancel-btn">Cancel</button><button type="button" class="btn-primary" ID="webshell-DB-profile-save-btn">save</button></div>' +
        '</div>' +
        '</div>' +
        '</section>' +
        '</div>' +
        '</div>';

    // Tab switching
    workspace.querySelectorAll('.webshell-tab').forEach(btn => {
        btn.addEventListener('click', function () {
            const tab = btn.getAttribute('data-tab');
            workspace.querySelectorAll('.webshell-tab').forEach(b => b.classList.remove('active'));
            workspace.querySelectorAll('.webshell-pane').forEach(p => p.classList.remove('active'));
            btn.classList.add('active');
            const pane = document.getElementById('webshell-pane-' + tab);
            if (pane) pane.classList.add('active');
            if (tab === 'terminal' && webshellTerminalInstance && webshellTerminalFitaddon) {
                try { webshellTerminalFitaddon.fit(); } catch (e) {}
            }
            if (tab === 'AI') {
                try { wsRefreshSelectors(); } catch (e) {}
            }
        });
    });

    // File Manager: list directory, parent directory
    const pathInput = document.getElementById('webshell-file-path');
    document.getElementById('webshell-list-dir').addEventListener('click', function () {
        // Apply changes immediately on save
        webshellFileListDir(webshellCurrentConn, pathInput ? pathInput.value.trim() || '.' : '.');
    });
    document.getElementById('webshell-parent-dir').addEventListener('click', function () {
        const p = (pathInput && pathInput.value.trim()) || '.';
        pathInput.value = getWebshellParentPath(p);
        webshellFileListDir(webshellCurrentConn, pathInput.value || '.');
    });

    // Clear screen is handled once via event delegation
    var terminalcopyLogBtn = document.getElementById('webshell-terminal-copy-log');
    if (terminalcopyLogBtn) {
        terminalcopyLogBtn.addEventListener('click', function () {
            if (!webshellCurrentConn || !webshellCurrentConn.id) return;
            var activeId = getActiveWebshellTerminalSessionId(webshellCurrentConn.id);
            var log = getWebshellTerminalLog(getWebshellTerminalSessionKey(webshellCurrentConn.id, activeId)) || '';
            if (navigator && navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(log).then(function () {
                    terminalcopyLogBtn.title = wsT('WebShell.terminalCopyOk') || 'Log copied';
                    setTimeout(function () {
                        terminalcopyLogBtn.title = wsT('WebShell.copyTerminalLog') || 'Copy Terminal Log';
                    }, 1200);
                }).catch(function () {
                    terminalcopyLogBtn.title = wsT('WebShell.terminalCopyFail') || 'copy failed';
                });
                return;
            }
            try {
                var ta = document.createElement('textarea');
                ta.value = log;
                document.body.appendChild(ta);
                ta.SELECT();
                document.execCommand('copy');
                document.body.removeChild(ta);
            } catch (e) {}
        });
    }
    renderWebshellTerminalSessions(conn);
    // Quick commands: execute and output to terminal
    workspace.querySelectorAll('.webshell-quick-cmd').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var cmd = btn.getAttribute('data-cmd');
            if (cmd) runQuickCommand(cmd);
        });
    });
    // File manager: refresh, create dir/file, upload, batch actions
    var filterInput = document.getElementById('webshell-file-filter');
    document.getElementById('webshell-file-refresh').addEventListener('click', function () {
        webshellFileListDir(webshellCurrentConn, pathInput ? pathInput.value.trim() || '.' : '.');
    });
    if (filterInput) filterInput.addEventListener('input', function () {
        webshellFileListApplyFilter();
    });
    document.getElementById('webshell-mkdir-btn').addEventListener('click', function () { webshellFileMkdir(webshellCurrentConn, pathInput); });
    document.getElementById('webshell-newFile-btn').addEventListener('click', function () { webshellFileNewFile(webshellCurrentConn, pathInput); });
    document.getElementById('webshell-upload-btn').addEventListener('click', function () { webshellFileUpload(webshellCurrentConn, pathInput); });
    document.getElementById('webshell-batch-delete-btn').addEventListener('click', function () { webshellBatchDelete(webshellCurrentConn, pathInput); });
    document.getElementById('webshell-batch-download-btn').addEventListener('click', function () { webshellBatchDownload(webshellCurrentConn, pathInput); });

    // AI Assistant: sidebar chat list and main messages
    var aiInput = document.getElementById('webshell-AI-input');
    var aisendBtn = document.getElementById('webshell-AI-send');
    var aiMessages = document.getElementById('webshell-AI-messages');
    var aiNewConvBtn = document.getElementById('webshell-AI-new-conv');
    var aiConvListEl = document.getElementById('webshell-AI-conv-list');

    // Initialize role, mode, and target selectors
    wsLoadRoles();
    wsInitAgentMode();
    if (typeof prefetchProjectsForChat === 'function') prefetchProjectsForChat();
    wsUpdateProjectButtonLabel();
    var aiMemoinput = document.getElementById('webshell-AI-memo-input');
    var aiMemoStatus = document.getElementById('webshell-AI-memo-status');
    var aiMemoclearBtn = document.getElementById('webshell-AI-memo-clear');
    var aiMemosaveTimer = null;

    function setWebshellAiMemoStatus(text, isError) {
        if (!aiMemoStatus) return;
        aiMemoStatus.textContent = text || '';
        aiMemoStatus.classList.toggle('error', !!isError);
    }

    function flushWebshellAiMemo() {
        if (!aiMemoinput) return;
        saveWebshellAiMemo(conn, aiMemoinput.value || '');
        setWebshellAiMemoStatus(wsT('WebShell.aiMemoSaved') || 'Saved locally', false);
    }

    if (aiMemoinput) {
        aiMemoinput.value = getWebshellAiMemo(conn);
        setWebshellAiMemoStatus(wsT('WebShell.aiMemoSaved') || 'Saved locally', false);
        aiMemoinput.addEventListener('input', function () {
            setWebshellAiMemoStatus(wsT('WebShell.aiMemoSaving') || 'Saving...', false);
            if (aiMemosaveTimer) clearTimeout(aiMemosaveTimer);
            aiMemosaveTimer = setTimeout(function () {
                aiMemosaveTimer = null;
                flushWebshellAiMemo();
            }, 500);
        });
        aiMemoinput.addEventListener('blur', function () {
            if (aiMemosaveTimer) {
                clearTimeout(aiMemosaveTimer);
                aiMemosaveTimer = null;
            }
            flushWebshellAiMemo();
        });
    }

    if (aiMemoclearBtn && aiMemoinput) {
        aiMemoclearBtn.addEventListener('click', function () {
            aiMemoinput.value = '';
            flushWebshellAiMemo();
            aiMemoinput.focus();
        });
    }

    if (aiNewConvBtn) {
        aiNewConvBtn.addEventListener('click', function () {
            delete webshellAiConvMap[conn.id];
            delete webshellAiDraftProjectByConn[conn.id];
            wsUpdateProjectButtonLabel();
            if (aiMessages) {
                aiMessages.innerHTML = '';
                var readyMsg = wsT('WebShell.aiSystemReadyMessage') || 'System ready. Please enter your test requirements and the system will automatically run the appropriate security test.';
                var div = document.createElement('div');
                div.className = 'webshell-AI-msg assistant';
                div.textContent = readyMsg;
                aiMessages.appendChild(div);
            }
            if (aiConvListEl) aiConvListEl.querySelectorAll('.webshell-AI-conv-item').forEach(function (el) { el.classList.remove('active'); });
        });
    }
    if (aisendBtn && aiInput && aiMessages) {
        var aistopBtn = document.getElementById('webshell-AI-stop');
        aisendBtn.addEventListener('click', function () { runWebshellAiSend(conn, aiInput, aisendBtn, aiMessages); });
        if (aistopBtn) {
            aistopBtn.addEventListener('click', function () { wsStopAiStream(conn); });
        }
        aiInput.addEventListener('keydown', function (e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                runWebshellAiSend(conn, aiInput, aisendBtn, aiMessages);
            }
        });
        fetchAndRenderWebshellAiConvList(conn, aiConvListEl).then(function () {
            loadWebshellAiHistory(conn, aiMessages).then(function () {
                if (webshellAiConvMap[conn.id] && aiConvListEl) {
                    aiConvListEl.querySelectorAll('.webshell-AI-conv-item').forEach(function (el) {
                        el.classList.toggle('active', el.dataset.convId === webshellAiConvMap[conn.id]);
                    });
                }
            });
        });
    }

    // Database manager: support multi-connection profiles
    var dbTypeEl = document.getElementById('webshell-DB-type');
    var dbrunBtn = document.getElementById('webshell-DB-run-btn');
    var dbTestBtn = document.getElementById('webshell-DB-test-btn');
    var dbSqlEl = document.getElementById('webshell-DB-SQL');
    var dbLoadSchemaBtn = document.getElementById('webshell-DB-load-schema-btn');
    var dbTemplateBtn = document.getElementById('webshell-DB-template-btn');
    var dbclearBtn = document.getElementById('webshell-DB-clear-btn');
    var dbSchemaTreeEl = document.getElementById('webshell-DB-schema-tree');
    var dbProfilesEl = document.getElementById('webshell-DB-profiles');
    var dbaddProfileBtn = document.getElementById('webshell-DB-add-profile-btn');
    var dbProfileModalEl = document.getElementById('webshell-DB-profile-modal');
    var dbProfileModalTitleEl = document.getElementById('webshell-DB-profile-modal-title');
    var dbProfileModalCloseBtn = document.getElementById('webshell-DB-profile-modal-close');
    var dbProfileModalCancelBtn = document.getElementById('webshell-DB-profile-cancel-btn');
    var dbProfileModalsaveBtn = document.getElementById('webshell-DB-profile-save-btn');
    var dbProfileNameEl = document.getElementById('webshell-DB-profile-name');
    var dbHostEl = document.getElementById('webshell-DB-host');
    var dbPortEl = document.getElementById('webshell-DB-port');
    var dbUserEl = document.getElementById('webshell-DB-user');
    var dbPassEl = document.getElementById('webshell-DB-pass');
    var dbNameEl = document.getElementById('webshell-DB-name');
    var dbSqliteEl = document.getElementById('webshell-DB-SQLite-path');
    var dbColumnsLoading = {};
    var dbColumnsBatchLoading = {};
    var dbColumnsBatchLoaded = {};

    function resetDbColumnLoadCache() {
        dbColumnsLoading = {};
        dbColumnsBatchLoading = {};
        dbColumnsBatchLoaded = {};
    }

    function setDbActionButtonsDisabled(disabled) {
        if (dbrunBtn) dbrunBtn.disabled = disabled;
        if (dbTestBtn) dbTestBtn.disabled = disabled;
        if (dbLoadSchemaBtn) dbLoadSchemaBtn.disabled = disabled;
        if (dbaddProfileBtn) dbaddProfileBtn.disabled = disabled;
    }

    function setDbProfileModalVisible(visible, mode) {
        if (!dbProfileModalEl) return;
        if (visible) {
            if (dbProfileModalTitleEl) {
                if (mode === 'add') dbProfileModalTitleEl.textContent = wsT('WebShell.dbAddProfile') || 'Add Profile';
                else dbProfileModalTitleEl.textContent = wsT('WebShell.editConnectionTitle') || 'Edit Connection';
            }
            openAppModal(dbProfileModalEl);
        } else {
            closeAppModal(dbProfileModalEl);
        }
    }

    function applyActiveDbProfileToForm() {
        var dbCfg = getWebshellDbConfig(conn);
        if (!dbCfg) return;
        if (dbProfileNameEl) dbProfileNameEl.value = dbCfg.name || 'DB-1';
        if (dbTypeEl) dbTypeEl.value = dbCfg.type || 'MySQL';
        if (dbHostEl) dbHostEl.value = dbCfg.host || '127.0.0.1';
        if (dbPortEl) dbPortEl.value = dbCfg.port || '';
        if (dbUserEl) dbUserEl.value = dbCfg.username || '';
        if (dbPassEl) dbPassEl.value = dbCfg.password || '';
        if (dbNameEl) dbNameEl.value = dbCfg.DATABASE || dbCfg.selectedDatabase || '';
        if (dbSqliteEl) dbSqliteEl.value = dbCfg.sqlitePath || '/tmp/test.DB';
        if (dbSqlEl) dbSqlEl.value = dbCfg.SQL || 'SELECT 1;';
        webshellDbUpdateFieldVisibility();
        webshellDbSetOutput(dbCfg.output || '', !!dbCfg.outputIserror);
        webshellDbRenderTable(dbCfg.output || '');
    }

    function renderDbProfileTabs() {
        if (!dbProfilesEl) return;
        var state = getWebshellDbState(conn);
        var HTML = '';
        state.profiles.forEach(function (p) {
            var active = p.id === state.activeProfileId;
            HTML += '<div class="webshell-DB-profile-tab' + (active ? ' active' : '') + '" data-idD="' + escapeHtml(p.id) + '">' +
                '<button type="button" class="webshell-DB-profile-main" data-action="switch" data-idD="' + escapeHtml(p.id) + '">' + escapeHtml(p.name || 'DB') + '</button>' +
                '<button type="button" class="webshell-DB-profile-menu" data-action="edit" data-idD="' + escapeHtml(p.id) + '" title="' + escapeHtml(wsT('WebShell.editConnection') || 'edit') + '">⚙</button>' +
                '<button type="button" class="webshell-DB-profile-menu" data-action="delete" data-idD="' + escapeHtml(p.id) + '" title="' + escapeHtml(wsT('WebShell.dbDeleteProfile') || 'Delete Profile') + '">×</button>' +
                '</div>';
        });
        dbProfilesEl.innerHTML = HTML;
        dbProfilesEl.querySelectorAll('[data-action]').forEach(function (btn) {
            btn.addEventListener('click', function () {
                var action = btn.getAttribute('data-action');
                var ID = btn.getAttribute('data-idD') || '';
                if (!ID) return;
                var state = getWebshellDbState(conn);
                var idx = state.profiles.findIndex(function (p) { return p.id === ID; });
                if (idx < 0) return;
                if (action === 'switch') {
                    state.activeProfileId = ID;
                    saveWebshellDbState(conn, state);
                    applyActiveDbProfileToForm();
                    renderDbProfileTabs();
                    resetDbColumnLoadCache();
                    renderDbSchemaTree();
                    return;
                }
                if (action === 'edit') {
                    state.activeProfileId = ID;
                    saveWebshellDbState(conn, state);
                    applyActiveDbProfileToForm();
                    renderDbProfileTabs();
                    setDbProfileModalVisible(true, 'edit');
                    return;
                }
                if (action === 'delete') {
                    if (state.profiles.length <= 1) return;
                    if (!confirm(wsT('WebShell.dbDeleteProfileConfirm') || 'Are you sure you want to delete this database profile?')) return;
                    state.profiles.splice(idx, 1);
                    if (!state.profiles.some(function (p) { return p.id === state.activeProfileId; })) {
                        state.activeProfileId = state.profiles[0].id;
                    }
                    saveWebshellDbState(conn, state);
                    applyActiveDbProfileToForm();
                    renderDbProfileTabs();
                    resetDbColumnLoadCache();
                    renderDbSchemaTree();
                }
            });
        });
    }

    function renderDbSchemaTree() {
        if (!dbSchemaTreeEl) return;
        var cfg = getWebshellDbConfig(conn);
        var schema = normalizeWebshellDbSchema((cfg && cfg.schema && typeof cfg.schema === 'object') ? cfg.schema : {});
        var dbs = Object.keys(schema).sort(function (a, b) { return a.localeCompare(b); });
        var openTableKeys = {};
        dbSchemaTreeEl.querySelectorAll('.webshell-DB-TABLE-node[open]').forEach(function (node) {
            var openDb = node.getAttribute('data-db') || '';
            var openTable = node.getAttribute('data-table') || '';
            if (!openDb || !openTable) return;
            openTableKeys[openDb + '::' + openTable] = true;
        });
        if (!dbs.length) {
            dbSchemaTreeEl.innerHTML = '<div class="webshell-empty">' + escapeHtml(wsT('WebShell.dbNoSchema') || 'No database schema loaded, please load schema first') + '</div>';
            return;
        }
        var selectedDb = (cfg.selectedDatabase || '').trim();
        var HTML = '';
        dbs.forEach(function (dbName) {
            var TABLES = (schema[dbName] && schema[dbName].TABLES) ? schema[dbName].TABLES : {};
            var tableNames = Object.keys(TABLES).sort(function (a, b) { return a.localeCompare(b); });
            var isActive = selectedDb && selectedDb === dbName;
            HTML += '<details class="webshell-DB-group"' + (isActive ? ' open' : '') + '>';
            HTML += '<summary class="webshell-DB-group-title" data-db="' + escapeHtml(dbName) + '" title="' + escapeHtml(dbName) + '"><span class="webshell-DB-icon">🗄</span><span class="webshell-DB-label">' + escapeHtml(dbName) + '</span><span class="webshell-DB-count">' + tableNames.length + '</span></summary>';
            HTML += '<div class="webshell-DB-group- items">';
            tableNames.forEach(function (tableName) {
                var COLUMNS = Array.isArray(TABLES[tableName]) ? TABLES[tableName] : [];
                var columnCountText = COLUMNS.length > 0 ? String(COLUMNS.length) : '-';
                var tableKey = dbName + '::' + tableName;
                var tableOpen = !!openTableKeys[tableKey];
                HTML += '<details class="webshell-DB-TABLE-node" data-db="' + escapeHtml(dbName) + '" data-table="' + escapeHtml(tableName) + '" data-columns-loaded="' + (COLUMNS.length ? '1' : '0') + '"' + (tableOpen ? ' open' : '') + '>';
                HTML += '<summary class="webshell-DB-TABLE-item" data-db="' + escapeHtml(dbName) + '" data-table="' + escapeHtml(tableName) + '" title="' + escapeHtml(tableName) + '"><span class="webshell-DB-icon">📄</span><span class="webshell-DB-label">' + escapeHtml(tableName) + '</span><span class="webshell-DB-count">' + escapeHtml(columnCountText) + '</span></summary>';
                if (COLUMNS.length) {
                    HTML += '<div class="webshell-DB-column-list">';
                    COLUMNS.forEach(function (columnName) {
                        HTML += '<button type="button" class="webshell-DB-column-item" data-db="' + escapeHtml(dbName) + '" data-table="' + escapeHtml(tableName) + '" data-column="' + escapeHtml(columnName) + '" title="' + escapeHtml(columnName) + '"><span class="webshell-DB-icon">🧱</span><span class="webshell-DB-label">' + escapeHtml(columnName) + '</span></button>';
                    });
                    HTML += '</div>';
                } else {
                    HTML += '<div class="webshell-DB-column-empty">' + escapeHtml(wsT('WebShell.dbNoColumns') || 'No column information') + '</div>';
                }
                HTML += '</details>';
            });
            HTML += '</div></details>';
        });
        dbSchemaTreeEl.innerHTML = HTML;

        dbSchemaTreeEl.querySelectorAll('.webshell-DB-group-title').forEach(function (el) {
            el.addEventListener('click', function () {
                var cfg = webshellDbCollectConfig(conn);
                cfg.selectedDatabase = el.getAttribute('data-db') || '';
                saveWebshellDbConfig(conn, cfg);
                if (dbNameEl && cfg.type !== 'SQLite') dbNameEl.value = cfg.selectedDatabase;
                ensureDbDatabaseColumns(cfg.selectedDatabase);
            });
        });
        dbSchemaTreeEl.querySelectorAll('.webshell-DB-TABLE-item').forEach(function (el) {
            el.addEventListener('click', function () {
                var TABLE = el.getAttribute('data-table') || '';
                var dbName = el.getAttribute('data-db') || '';
                if (!TABLE) return;
                var cfg = webshellDbCollectConfig(conn);
                cfg.selectedDatabase = dbName;
                if (cfg.type !== 'SQLite') cfg.DATABASE = dbName;
                saveWebshellDbConfig(conn, cfg);
                if (dbNameEl && cfg.type !== 'SQLite') dbNameEl.value = dbName;
                var tableRef = cfg.type === 'SQLite'
                    ? webshellDbQuoteIdentifier(cfg.type, TABLE)
                    : webshellDbQuoteIdentifier(cfg.type, dbName) + '.' + webshellDbQuoteIdentifier(cfg.type, TABLE);
                if (dbSqlEl) {
                    dbSqlEl.value = 'SELECT * FROM ' + tableRef + ' ORDER BY 1 DESC LIMIT 20;';
                    webshellDbCollectConfig(conn);
                }
                ensureDbTableColumns(dbName, TABLE);
            });
        });
        dbSchemaTreeEl.querySelectorAll('.webshell-DB-TABLE-node').forEach(function (node) {
            node.addEventListener('toggle', function () {
                if (!node.open) return;
                var dbName = node.getAttribute('data-db') || '';
                var TABLE = node.getAttribute('data-table') || '';
                if (!dbName || !TABLE) return;
                ensureDbTableColumns(dbName, TABLE);
            });
        });
        dbSchemaTreeEl.querySelectorAll('.webshell-DB-column-item').forEach(function (el) {
            el.addEventListener('click', function (evt) {
                evt.preventDefault();
                evt.stopPropagation();
                var TABLE = el.getAttribute('data-table') || '';
                var column = el.getAttribute('data-column') || '';
                var dbName = el.getAttribute('data-db') || '';
                if (!TABLE || !column) return;
                var cfg = webshellDbCollectConfig(conn);
                cfg.selectedDatabase = dbName;
                if (cfg.type !== 'SQLite') cfg.DATABASE = dbName;
                saveWebshellDbConfig(conn, cfg);
                if (dbNameEl && cfg.type !== 'SQLite') dbNameEl.value = dbName;
                var tableRef = cfg.type === 'SQLite'
                    ? webshellDbQuoteIdentifier(cfg.type, TABLE)
                    : webshellDbQuoteIdentifier(cfg.type, dbName) + '.' + webshellDbQuoteIdentifier(cfg.type, TABLE);
                if (dbSqlEl) {
                    dbSqlEl.value = 'SELECT ' + webshellDbQuoteIdentifier(cfg.type, column) + ' FROM ' + tableRef + ' LIMIT 20;';
                    webshellDbCollectConfig(conn);
                }
            });
        });
        var autoDb = selectedDb || dbs[0] || '';
        if (autoDb) ensureDbDatabaseColumns(autoDb);
    }

    function ensureDbTableColumns(dbName, tableName) {
        if (!dbName || !tableName || webshellRunning) return;
        var loadKey = (conn && conn.id ? conn.id : 'local') + '::' + dbName + '::' + tableName;
        if (dbColumnsLoading[loadKey]) return;
        webshellDbCollectConfig(conn);
        var cfg = getWebshellDbConfig(conn);
        var schema = normalizeWebshellDbSchema((cfg && cfg.schema && typeof cfg.schema === 'object') ? cfg.schema : {});
        if (!schema[dbName]) return;
        if (!schema[dbName].TABLES[tableName]) return;
        if (Array.isArray(schema[dbName].TABLES[tableName]) && schema[dbName].TABLES[tableName].length > 0) return;

        var built = buildWebshellDbColumnsCommand(cfg, dbName, tableName);
        if (!built.command) return;
        dbColumnsLoading[loadKey] = true;
        webshellRunning = true;
        setDbActionButtonsDisabled(true);
        execWebshellCommand(conn, built.command).then(function (out) {
            var parsed = parseWebshellDbExecOutput(out);
            var success = parsed.rc === 0 || (parsed.rc == null && parsed.output && !/error|failed|denied|unknown|NOT found|access/i.test(parsed.output));
            if (!success) return;
            var COLUMNS = parseWebshellDbColumns(parsed.output);
            if (!COLUMNS.length) return;
            var nextCfg = getWebshellDbConfig(conn);
            var nextSchema = normalizeWebshellDbSchema((nextCfg && nextCfg.schema && typeof nextCfg.schema === 'object') ? nextCfg.schema : {});
            if (!nextSchema[dbName]) nextSchema[dbName] = { TABLES: {} };
            if (!nextSchema[dbName].TABLES[tableName]) nextSchema[dbName].TABLES[tableName] = [];
            nextSchema[dbName].TABLES[tableName] = COLUMNS;
            nextCfg.schema = nextSchema;
            saveWebshellDbConfig(conn, nextCfg);
            renderDbSchemaTree();
        }).catch(function () {
            // ignore single-TABLE column load errors to avoid interrupting main flow
        }).finally(function () {
            delete dbColumnsLoading[loadKey];
            webshellRunning = false;
            setDbActionButtonsDisabled(false);
        });
    }

    function ensureDbDatabaseColumns(dbName) {
        if (!dbName || webshellRunning) return;
        webshellDbCollectConfig(conn);
        var cfg = getWebshellDbConfig(conn);
        var schema = normalizeWebshellDbSchema((cfg && cfg.schema && typeof cfg.schema === 'object') ? cfg.schema : {});
        if (!schema[dbName] || !schema[dbName].TABLES) return;
        var hasUnknown = Object.keys(schema[dbName].TABLES).some(function (tableName) {
            var cols = schema[dbName].TABLES[tableName];
            return !Array.isArray(cols) || cols.length === 0;
        });
        if (!hasUnknown) return;

        var batchKey = (conn && conn.id ? conn.id : 'local') + '::' + (cfg.type || 'MySQL') + '::' + dbName;
        if (dbColumnsBatchLoading[batchKey] || dbColumnsBatchLoaded[batchKey]) return;
        var built = buildWebshellDbColumnsByDatabaseCommand(cfg, dbName);
        if (!built.command) return;

        dbColumnsBatchLoading[batchKey] = true;
        webshellRunning = true;
        setDbActionButtonsDisabled(true);
        execWebshellCommand(conn, built.command).then(function (out) {
            var parsed = parseWebshellDbExecOutput(out);
            var success = parsed.rc === 0 || (parsed.rc == null && parsed.output && !/error|failed|denied|unknown|NOT found|access/i.test(parsed.output));
            if (!success) return;
            var tableColumns = parseWebshellDbTableColumns(parsed.output);
            if (!Object.keys(tableColumns).length) return;

            var nextCfg = getWebshellDbConfig(conn);
            var nextSchema = normalizeWebshellDbSchema((nextCfg && nextCfg.schema && typeof nextCfg.schema === 'object') ? nextCfg.schema : {});
            if (!nextSchema[dbName]) nextSchema[dbName] = { TABLES: {} };
            Object.keys(tableColumns).forEach(function (tableName) {
                nextSchema[dbName].TABLES[tableName] = tableColumns[tableName];
            });
            nextCfg.schema = nextSchema;
            saveWebshellDbConfig(conn, nextCfg);
            renderDbSchemaTree();
        }).catch(function () {
            // ignore batch column load errors to avoid interrupting main flow
        }).finally(function () {
            delete dbColumnsBatchLoading[batchKey];
            dbColumnsBatchLoaded[batchKey] = true;
            webshellRunning = false;
            setDbActionButtonsDisabled(false);
        });
    }

    function loadDbSchema() {
        if (!conn || !conn.id) {
            webshellDbSetOutput(wsT('WebShell.dbNoConn') || 'Please select a WebShell connection first', true);
            return;
        }
        if (webshellRunning) {
            webshellDbSetOutput(wsT('WebShell.dbRunning') || 'Executing database command, please wait...', true);
            return;
        }
        var cfg = webshellDbCollectConfig(conn);
        resetDbColumnLoadCache();
        var built = buildWebshellDbSchemaCommand(cfg);
        if (!built.command) {
            webshellDbSetOutput(built.error || (wsT('WebShell.dbSchemaFailed') || 'Failed to load database schema'), true);
            return;
        }
        webshellDbSetOutput(wsT('WebShell.running') || 'Executing…', false);
        webshellRunning = true;
        setDbActionButtonsDisabled(true);
        execWebshellCommand(conn, built.command).then(function (out) {
            var parsed = parseWebshellDbExecOutput(out);
            var success = parsed.rc === 0 || (parsed.rc == null && parsed.output && !/error|failed|denied|unknown|NOT found|access/i.test(parsed.output));
            if (!success) {
                webshellDbSetOutput((wsT('WebShell.dbSchemaFailed') || 'Failed to load database schema') + ':\n' + (parsed.output || ''), true);
                return;
            }
            cfg.schema = parseWebshellDbSchema(parsed.output);
            cfg.output = wsT('WebShell.dbSchemaLoaded') || 'Schema loaded successfully';
            cfg.outputIserror = false;
            saveWebshellDbConfig(conn, cfg);
            renderDbSchemaTree();
            webshellDbSetOutput(wsT('WebShell.dbSchemaLoaded') || 'Schema loaded successfully', false);
        }).catch(function (err) {
            webshellDbSetOutput((wsT('WebShell.dbSchemaFailed') || 'Failed to load database schema') + ': ' + (err && err.message ? err.message : String(err)), true);
        }).finally(function () {
            webshellRunning = false;
            setDbActionButtonsDisabled(false);
        });
    }

    function runDbQuery(isTestOnly) {
        if (!conn || !conn.id) {
            webshellDbSetOutput(wsT('WebShell.dbNoConn') || 'Please select a WebShell connection first', true);
            return;
        }
        if (webshellRunning) {
            webshellDbSetOutput(wsT('WebShell.dbRunning') || 'Executing database command, please wait...', true);
            return;
        }
        var cfg = webshellDbCollectConfig(conn);
        var built = buildWebshellDbCommand(cfg, !!isTestOnly);
        if (!built.command) {
            webshellDbSetOutput(built.error || (wsT('WebShell.dbExecFailed') || 'Database execution failed'), true);
            return;
        }
        webshellDbSetOutput(wsT('WebShell.running') || 'Executing…', false);
        webshellRunning = true;
        setDbActionButtonsDisabled(true);
        execWebshellCommand(conn, built.command).then(function (out) {
            var parsed = parseWebshellDbExecOutput(out);
            var code = parsed.rc;
            var content = parsed.output || '';
            var success = (code === 0) || (code == null && content && !/error|failed|denied|unknown|NOT found|access/i.test(content));
            if (isTestOnly) {
                if (success) {
                    cfg.output = 'Connection test passed';
                    cfg.outputIserror = false;
                    saveWebshellDbConfig(conn, cfg);
                    webshellDbSetOutput(cfg.output, false);
                } else {
                    cfg.output = 'Connection test failed' + (content ? (':\n' + content) : '');
                    cfg.outputIserror = true;
                    saveWebshellDbConfig(conn, cfg);
                    webshellDbSetOutput(cfg.output, true);
                }
                return;
            }
            if (!success) {
                cfg.output = (wsT('WebShell.dbExecFailed') || 'Database execution failed') + (content ? (':\n' + content) : '');
                cfg.outputIserror = true;
                saveWebshellDbConfig(conn, cfg);
                webshellDbSetOutput(cfg.output, true);
                return;
            }
            var hasTable = webshellDbRenderTable(content);
            if (hasTable) {
                cfg.output = wsT('WebShell.dbExecSuccess') || 'SQL executesuccess';
                cfg.outputIserror = false;
                saveWebshellDbConfig(conn, cfg);
                webshellDbSetOutput(cfg.output, false);
            } else {
                cfg.output = content || (wsT('WebShell.dbNoOutput') || 'Execute complete (no output)');
                cfg.outputIserror = false;
                saveWebshellDbConfig(conn, cfg);
                webshellDbSetOutput(cfg.output, false);
            }
        }).catch(function (err) {
            cfg.output = (wsT('WebShell.dbExecFailed') || 'Database execution failed') + ': ' + (err && err.message ? err.message : String(err));
            cfg.outputIserror = true;
            saveWebshellDbConfig(conn, cfg);
            webshellDbSetOutput(cfg.output, true);
        }).finally(function () {
            webshellRunning = false;
            setDbActionButtonsDisabled(false);
        });
    }

    if (dbTypeEl) dbTypeEl.addEventListener('change', function () {
        webshellDbUpdateFieldVisibility();
        var cfg = webshellDbCollectConfig(conn);
        cfg.selectedDatabase = '';
        cfg.schema = {};
        saveWebshellDbConfig(conn, cfg);
        resetDbColumnLoadCache();
        renderDbSchemaTree();
    });
    ['webshell-DB-profile-name', 'webshell-DB-host', 'webshell-DB-port', 'webshell-DB-user', 'webshell-DB-pass', 'webshell-DB-name', 'webshell-DB-SQLite-path'].forEach(function (ID) {
        var el = document.getElementById(ID);
        if (el) el.addEventListener('change', function () {
            webshellDbCollectConfig(conn);
            resetDbColumnLoadCache();
            renderDbProfileTabs();
        });
    });
    if (dbSqlEl) dbSqlEl.addEventListener('change', function () { webshellDbCollectConfig(conn); });
    if (dbrunBtn) dbrunBtn.addEventListener('click', function () { runDbQuery(false); });
    if (dbTestBtn) dbTestBtn.addEventListener('click', function () { runDbQuery(true); });
    if (dbLoadSchemaBtn) dbLoadSchemaBtn.addEventListener('click', function () { loadDbSchema(); });
    if (dbTemplateBtn) dbTemplateBtn.addEventListener('click', function () {
        if (!dbSqlEl) return;
        var cfg = webshellDbCollectConfig(conn);
        if (cfg.type === 'MySQL') dbSqlEl.value = 'SHOW DATABASES;\nSELECT DATABASE() AS current_db;';
        else if (cfg.type === 'pgsql') dbSqlEl.value = 'SELECT current_database();\nSELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA ORDER BY SCHEMA_NAME;';
        else if (cfg.type === 'SQLite') dbSqlEl.value = "SELECT name FROM sqlite_master WHERE type='TABLE' ORDER BY name;";
        else dbSqlEl.value = "SELECT name FROM sys.DATABASES ORDER BY name;\nSELECT DB_NAME() AS current_db;";
        webshellDbCollectConfig(conn);
    });
    if (dbclearBtn) dbclearBtn.addEventListener('click', function () {
        if (dbSqlEl) dbSqlEl.value = '';
        webshellDbCollectConfig(conn);
    });
    if (dbProfileModalCloseBtn) dbProfileModalCloseBtn.addEventListener('click', function () {
        setDbProfileModalVisible(false);
    });
    if (dbProfileModalCancelBtn) dbProfileModalCancelBtn.addEventListener('click', function () {
        applyActiveDbProfileToForm();
        setDbProfileModalVisible(false);
    });
    if (dbProfileModalsaveBtn) dbProfileModalsaveBtn.addEventListener('click', function () {
        webshellDbCollectConfig(conn);
        renderDbProfileTabs();
        resetDbColumnLoadCache();
        setDbProfileModalVisible(false);
    });
    if (dbProfileModalEl) dbProfileModalEl.addEventListener('click', function (evt) {
        if (evt.target === dbProfileModalEl) {
            applyActiveDbProfileToForm();
            setDbProfileModalVisible(false);
        }
    });
    if (dbaddProfileBtn) dbaddProfileBtn.addEventListener('click', function () {
        var state = getWebshellDbState(conn);
        var name = 'DB-' + (state.profiles.length + 1);
        var p = newWebshellDbProfile(name);
        state.profiles.push(p);
        state.activeProfileId = p.id;
        saveWebshellDbState(conn, state);
        applyActiveDbProfileToForm();
        renderDbProfileTabs();
        renderDbSchemaTree();
        setDbProfileModalVisible(true, 'add');
    });
    renderDbProfileTabs();
    applyActiveDbProfileToForm();
    renderDbSchemaTree();
    setDbProfileModalVisible(false);

    initWebshellTerminal(conn);
}

// Load WebShell AI chat history (persisted), returns Promise
function loadWebshellAiHistory(conn, messagesContainer) {
    if (!conn || !conn.id || !messagesContainer) return Promise.resolve();
    if (typeof apiFetch !== 'function') return Promise.resolve();
    return apiFetch('/api/WebShell/connections/' + encodeURIComponent(conn.id) + '/AI-history', { method: 'GET' })
        .then(function (r) { return r.json(); })
        .then(function (data) {
            if (data.conversationId) {
                webshellAiConvMap[conn.id] = data.conversationId;
                apiFetch('/api/conversations/' + encodeURIComponent(data.conversationId), { method: 'GET' })
                    .then(function (r) { return r.ok ? r.json() : null; })
                    .then(function (conv) {
                        if (conv) wsSetWebshellAiProject(conn, conv.projectId || conv.project_id || '');
                    })
                    .catch(function () { /* ignore */ });
            }
            var list = Array.isArray(data.messages) ? data.messages : [];
            list.forEach(function (msg) {
                var role = (msg.role || '').toLowerCase();
                var content = (msg.content || '').trim();
                if (!content && role !== 'assistant') return;
                var div = document.createElement('div');
                div.className = 'webshell-AI-msg ' + (role === 'user' ? 'user' : 'assistant');
                if (role === 'user') {
                    div.textContent = content;
                } else {
                    if (isLikelyWebshellAiErrorMessage(content, msg)) {
                        renderWebshellAiErrorMessage(div, content);
                    } else if (typeof formatMarkdown === 'function') {
                        div.innerHTML = formatMarkdown(content);
                    } else {
                        div.textContent = content;
                    }
                }
                messagesContainer.appendChild(div);
                if (role === 'assistant') {
                    var wsHistMerged = (typeof window.mergeMessageReasoningContentIntoProcessDetails === 'function')
                        ? window.mergeMessageReasoningContentIntoProcessDetails(msg.processDetails || [], msg.reasoningContent)
                        : (msg.processDetails || []);
                    if (wsHistMerged.length > 0) {
                        var block = renderWebshellProcessDetailsBlock(wsHistMerged, true);
                        if (block) messagesContainer.appendChild(block);
                    }
                }
            });
            if (list.length === 0) {
                var readyMsg = wsT('WebShell.aiSystemReadyMessage') || 'System ready. Please enter your test requirements and the system will automatically run the appropriate security test.';
                var readyDiv = document.createElement('div');
                readyDiv.className = 'webshell-AI-msg assistant';
                readyDiv.textContent = readyMsg;
                messagesContainer.appendChild(readyDiv);
            }
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
        })
        .catch(function (e) {
            console.warn('Failed to load WebShell AI history', conn.id, e);
        });
}

function runWebshellAiSend(conn, inputEl, sendBtn, messagesContainer) {
    if (!conn || !conn.id) return;
    var message = (inputEl && inputEl.value || '').trim();
    if (!message) return;
    if (webshellAisending) return;
    if (typeof apiFetch !== 'function') {
        if (messagesContainer) {
            var errDiv = document.createElement('div');
            errDiv.className = 'webshell-AI-msg assistant';
            errDiv.textContent = 'Cannot send: not logged in or apiFetch is unavailable';
            messagesContainer.appendChild(errDiv);
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
        }
        return;
    }

    webshellAiAbortController = new AbortController();
    wsSetAiSendingState(true);

    var userDiv = document.createElement('div');
    userDiv.className = 'webshell-AI-msg user';
    userDiv.textContent = message;
    messagesContainer.appendChild(userDiv);

    var timelineContainer = document.createElement('div');
    timelineContainer.className = 'webshell-AI-timeline';
    timelineContainer.setAttribute('aria-live', 'polite');

    var assistantDiv = document.createElement('div');
    assistantDiv.className = 'webshell-AI-msg assistant';
    assistantDiv.textContent = '…';
    messagesContainer.appendChild(timelineContainer);
    messagesContainer.appendChild(assistantDiv);
    messagesContainer.scrollTop = messagesContainer.scrollHeight;

    function appendTimelineItem(type, title, message, data) {
        var item = document.createElement('div');
        item.className = 'webshell-AI-timeline-item webshell-AI-timeline-' + type;

        var HTML = '<span class="webshell-AI-timeline-title">' + escapeHtml(title || message || '') + '</span>';

        // Tool call inputs and outputs in unified card
        if (type === 'tool_call' && data) {
            try {
                var args = data.argumentsObj;
                if (args == null && data.arguments != null && String(data.arguments).trim() !== '') {
                    try {
                        args = JSON.parse(String(data.arguments));
                    } catch (e1) {
                        args = { _raw: String(data.arguments) };
                    }
                }
                if (args == null || typeof args !== 'object') {
                    args = {};
                }
                var paramsLabel = (typeof window.t === 'function') ? window.t('timeline.params') : 'Parameters:';
                var pendingResult = (typeof window.buildToolResultSectionHtml === 'function')
                    ? window.buildToolResultSectionHtml({}, { pending: true })
                    : '';
                HTML += '<div class="webshell-AI-timeline-msg"><div class="tool-arg-section"><strong>' +
                    escapeHtml(paramsLabel) +
                    '</strong><pre class="tool-args">' +
                    escapeHtml(JSON.stringify(args, null, 2)) +
                    '</pre></div>' +
                    (pendingResult ? '<div class="tool-result-slot">' + pendingResult + '</div>' : '') +
                    '</div>';
            } catch (e) {
                // Ignore parameter details on JSON parse error to avoid interrupting flow
            }
        } else if (type === 'eino_agent_reply' && message) {
            HTML += '<div class="webshell-AI-timeline-msg"><pre style="white-space:pre-wrap;">' + escapeHtml(message) + '</pre></div>';
        } else if (type === 'eino_usage_summary') {
            var usageText = message || formatWebshellEinoUsageSummaryMessage(data);
            HTML += '<div class="webshell-AI-timeline-msg"><pre style="white-space:pre-wrap;">' + escapeHtml(usageText) + '</pre></div>';
        } else if (type === 'tool_result' && data) {
            // Tool call outputs
            var noResultText = (typeof window.t === 'function') ? window.t('timeline.noResult') : 'No result';
            var result = data.result != null ? data.result : (data.error != null ? data.error : (data.resultPreview != null ? data.resultPreview : noResultText));
            var resultStr = (typeof result === 'string') ? result : JSON.stringify(result);
            var displayState = (typeof window.getToolResultDisplayState === 'function')
                ? window.getToolResultDisplayState(data, { rawText: resultStr })
                : { kind: ((data.isError || data.success === false) ? 'error' : 'success'), isError: (data.isError || data.success === false) };
            var execResultLabel = (typeof window.t === 'function') ? window.t('timeline.executionResult') : 'Result:';
            var execIdLabel = (typeof window.t === 'function') ? window.t('timeline.executionId') : 'executeID:';
            var sectionClass = displayState.kind === 'blocked' ? 'blocked' : (displayState.kind === 'background_running' ? 'pending' : (displayState.isError ? 'error' : 'success'));
            HTML += '<div class="webshell-AI-timeline-msg"><div class="tool-result-section ' +
                sectionClass +
                '"><strong>' + escapeHtml(execResultLabel) + '</strong><pre class="tool-result">' +
                escapeHtml(resultStr) +
                '</pre>' +
                (data.executionId ? '<div class="tool-execution-ID"><span>' +
                    escapeHtml(execIdLabel) +
                    '</span> <code>' +
                    escapeHtml(String(data.executionId)) +
                    '</code></div>' : '') +
                '</div></div>';
        } else if (message && message !== title) {
            HTML += '<div class="webshell-AI-timeline-msg">' + escapeHtml(message) + '</div>';
        }

        item.innerHTML = HTML;
        timelineContainer.appendChild(item);
        timelineContainer.classList.add('has- items');
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
        return item;
    }

    var einoSubReplyStreams = new Map();
    var wsThinkingStreams = new Map();        // streamId → { el, buf }
    var wsToolResultStreams = new Map();      // toolCallId → { el, buf }
    var wsToolCallItems = new Map();          // toolCallId -> DOM item

    if (inputEl) inputEl.value = '';

    var convId = webshellAiConvMap[conn.id] || '';
    var wsRole = (typeof getCurrentRole === 'function') ? getCurrentRole() : (localStorage.getItem('currentRole') || '');
    var body = {
        message: message,
        webshellConnectionId: conn.id,
        conversationId: convId,
        role: wsRole,
        finalization: {
            requireExecutionEvidence: true
        }
    };
    if (!convId) {
        var wsPid = getWebshellAiProjectSelection(conn);
        if (wsPid) body.projectId = wsPid;
    }

    // Streaming output: supports progress updates and typing effect
    var streamingTarget = '';  // Current target text for typing effect
    var streamingTypingId = 0;  // Re-entrancy guard, increments per response

    resolveWebshellAiStreamRequest().then(function (info) {
        if (info && info.orchestration) {
            body.orchestration = info.orchestration;
        }
        if (typeof window.buildReasoningRequestPayload === 'function') {
            var rp = window.buildReasoningRequestPayload();
            if (rp) {
                body.reasoning = rp;
            }
        }
        return apiFetch(info.path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
            signal: webshellAiAbortController ? webshellAiAbortController.signal : undefined
        });
    }).then(function (response) {
        if (!response.ok) {
            renderWebshellAiErrorMessage(assistantDiv, 'Request failed: HTTP ' + response.status);
            return;
        }
        return response.body.getReader();
    }).then(function (reader) {
        if (!reader) return;
        webshellAiStreamReader = reader;
        var decoder = new TextDecoder();
        var buffer = '';
        return reader.read().then(function processChunk(result) {
            if (result.done) return;
            buffer += decoder.decode(result.value, { stream: true });
            var lines = buffer.split('\n');
            buffer = lines.pop() || '';
            for (var i = 0; i < lines.length; i++) {
                var line = lines[i];
                if (line.indexOf('data: ') !== 0) continue;
                try {
                    var eventData = JSON.parse(line.slice(6));
                    var _et = eventData.type;
                    var _ed = eventData.data || {};
                    var _em = eventData.message || '';

                    if (_et === 'conversation' && _ed.conversationId) {
                        var convId = _ed.conversationId;
                        var prevDraft = webshellAiDraftProjectByConn[conn.id];
                        if (prevDraft) {
                            webshellAiProjectByConvId[convId] = prevDraft;
                            delete webshellAiDraftProjectByConn[conn.id];
                        }
                        webshellAiConvMap[conn.id] = convId;
                        var listEl = document.getElementById('webshell-AI-conv-list');
                        if (listEl) fetchAndRenderWebshellAiConvList(conn, listEl).then(function () {
                            listEl.querySelectorAll('.webshell-AI-conv-item').forEach(function (el) {
                                el.classList.toggle('active', el.dataset.convId === convId);
                            });
                        });

                    // ─── Response streaming ───
                    } else if (_et === 'response_start') {
                        streamingTarget = '';
                        webshellStreamingTypingId += 1;
                        streamingTypingId = webshellStreamingTypingId;
                        assistantDiv.dataset.finalized = 'false';
                        assistantDiv.classList.add('webshell-AI-candidate-output');
                        assistantDiv.textContent = '…';
                        messagesContainer.scrollTop = messagesContainer.scrollHeight;
                    } else if (_et === 'response_delta') {
                        var deltaText = (_em != null && _em !== '') ? String(_em) : '';
                        var mergeBuf = (typeof window.mergeStreamBuffer === 'function')
                            ? window.mergeStreamBuffer
                            : function (cur, dlt) {
                                var normR = (typeof window.normalizeStreamingDeltaJs === 'function')
                                    ? window.normalizeStreamingDeltaJs(cur, dlt)
                                    : [cur + dlt, dlt];
                                return normR[0];
                            };
                        if (deltaText || (_ed && _ed.accumulated != null)) {
                            streamingTarget = mergeBuf(streamingTarget, deltaText, _ed);
                            webshellStreamingTypingId += 1;
                            streamingTypingId = webshellStreamingTypingId;
                            runWebshellAiStreamingTyping(assistantDiv, streamingTarget, streamingTypingId, messagesContainer);
                        }
                    } else if (_et === 'response') {
                        var text = (_em != null && _em !== '') ? _em : (typeof _ed === 'string' ? _ed : '');
                        var finalized = !!(_ed && _ed.finalized === true);
                        var hasFinalizationContract = !!(_ed && (
                            Object.prototype.hasOwnProperty.call(_ed, 'finalized') ||
                            Object.prototype.hasOwnProperty.call(_ed, 'finalizable') ||
                            Object.prototype.hasOwnProperty.call(_ed, 'completionReason') ||
                            Object.prototype.hasOwnProperty.call(_ed, 'evidenceVerified') ||
                            Object.prototype.hasOwnProperty.call(_ed, 'missingChecks')
                        ));
                        assistantDiv.dataset.finalized = finalized ? 'true' : 'false';
                        assistantDiv.classList.toggle('webshell-AI-candidate-output', !finalized);
                        assistantDiv.classList.toggle('webshell-AI-finalized-output', finalized);
                        if (!finalized && !hasFinalizationContract && text) {
                            appendTimelineItem('finalization_check', 'Candidate output missing finalisation evidence', text, Object.assign({}, _ed || {}, { missingFinalizationContract: true }));
                            text = webshellFinalizationNotice(_ed || {}, text, false);
                        } else if (!finalized && hasFinalizationContract) {
                            text = webshellFinalizationNotice(_ed || {}, text, true);
                        }
                        if (text) {
                            streamingTarget = String(text);
                            webshellStreamingTypingId += 1;
                            streamingTypingId = webshellStreamingTypingId;
                            runWebshellAiStreamingTyping(assistantDiv, streamingTarget, streamingTypingId, messagesContainer);
                        }

                    // ─── Terminal events ───
                    } else if (_et === 'finalization_check') {
                        var finalizationOk = !!(_ed && _ed.finalized === true);
                        appendTimelineItem('finalization_check', finalizationOk ? 'Final reply check passed' : 'Final reply check failed', finalizationOk ? (_em || 'Final reply check passed.') : webshellFinalizationNotice(_ed || {}, _em, true), _ed);
                    } else if (_et === 'finalization_auto_continue') {
                        appendTimelineItem('progress', 'Continue verification', _em, _ed);
                    } else if (_et === 'eino_model_retry') {
                        var retryAttempt = _ed && _ed.attempt ? (' (' + _ed.attempt + ')') : '';
                        var retryMsg = _em || 'Model call encountered a transient error; Eino retrying...';
                        if (_ed && _ed.reason) retryMsg += '\nReason: ' + _ed.reason;
                        if (_ed && _ed.error) retryMsg += '\nerrorDetails: ' + _ed.error;
                        appendTimelineItem('warning', '🔁 model call retry' + retryAttempt, retryMsg, _ed);
                    } else if (_et === 'eino_model_failover') {
                        var failoverAttempt = _ed && _ed.attempt ? (' (' + _ed.attempt + ')') : '';
                        var failoverMsg = _em || 'Primary model retries exhausted; switching to fallback model.';
                        if (_ed && _ed.channel) failoverMsg += '\nChannel: ' + _ed.channel;
                        if (_ed && _ed.model) failoverMsg += '\nModel: ' + _ed.model;
                        appendTimelineItem('warning', '🔀 switch to backup model' + failoverAttempt, failoverMsg, _ed);
                    } else if (_et === 'eino_usage_summary') {
                        appendTimelineItem('eino_usage_summary', '📊 ' + formatWebshellEinoUsageSummaryTitle(_ed), formatWebshellEinoUsageSummaryMessage(_ed), _ed);
                    } else if (_et === 'error' && _em) {
                        streamingTypingId += 1;
                        var errLabel = wsTOr('chat.error', 'error');
                        appendTimelineItem('error', '❌ ' + errLabel, _em, _ed);
                        renderWebshellAiErrorMessage(assistantDiv, errLabel + ': ' + _em);
                    } else if (_et === 'cancelled') {
                        streamingTypingId += 1;
                        var cancelLabel = wsTOr('chat.taskCancelled', 'Task cancelled');
                        appendTimelineItem('cancelled', '⛔ ' + cancelLabel, _em, _ed);
                        if (!streamingTarget && !assistantDiv.dataset.hasContent) {
                            assistantDiv.textContent = cancelLabel;
                        }
                    } else if (_et === 'done') {
                        // Clear streaming state
                        wsThinkingStreams.clear();
                        wsToolResultStreams.clear();
                        einoSubReplyStreams.clear();

                    // ─── Iteration / Progress ───
                    } else if (_et === 'progress' && _em) {
                        var progressMsg = (typeof window.translateProgressMessage === 'function')
                            ? window.translateProgressMessage(_em) : _em;
                        appendTimelineItem('progress', '🔍 ' + progressMsg, '', _ed);
                        if (!streamingTarget) assistantDiv.textContent = '…';
                    } else if (_et === 'iteration') {
                        var iterN = _ed.iteration || 0;
                        var iterTitle = wsTOr('chat.iterationRound', '') || (iterN ? ('Round ' + iterN + ' iteration') : (_em || 'Iteration'));
                        if (typeof window.t === 'function' && iterN) {
                            iterTitle = window.t('chat.iterationRound', { n: iterN });
                        }
                        var iterMessage = _em || '';
                        if (iterMessage && typeof window.translateProgressMessage === 'function') {
                            iterMessage = window.translateProgressMessage(iterMessage);
                        }
                        appendTimelineItem('iteration', '🔍 ' + iterTitle, iterMessage, _ed);
                        if (!streamingTarget) assistantDiv.textContent = '…';

                    // ─── Thinking / reasoning_chain (reasoning process, reasoning_content) ───
                    } else if ((_et === 'thinking_stream_start' || _et === 'reasoning_chain_stream_start') && _ed.streamId) {
                        var isRcstart = _et === 'reasoning_chain_stream_start';
                        if (wsThinkingStreams.has(_ed.streamId)) {
                            var tsExist = wsThinkingStreams.get(_ed.streamId);
                            tsExist.buf = '';
                            if (tsExist.body) tsExist.body.textContent = '';
                        } else {
                        var thinkSLabel = wsTOr(isRcstart ? 'chat.reasoningChain' : 'chat.aiThinking', isRcstart ? 'Reasoning Process' : 'AI Thinking');
                        var thinkEmoji = isRcstart ? '🔗' : '🤔';
                        var thinkSItem = document.createElement('div');
                        thinkSItem.className = 'webshell-AI-timeline-item webshell-AI-timeline-' + (isRcstart ? 'reasoning_chain' : 'thinking');
                        thinkSItem.innerHTML = '<span class="webshell-AI-timeline-title">' + escapeHtml(webshellAgentPx(_ed) + thinkEmoji + ' ' + thinkSLabel) + '</span>';
                        var thinkSPre = document.createElement('div');
                        thinkSPre.className = 'webshell-AI-timeline-msg webshell-thinking-stream-body';
                        thinkSItem.appendChild(thinkSPre);
                        timelineContainer.appendChild(thinkSItem);
                        timelineContainer.classList.add('has- items');
                        wsThinkingStreams.set(_ed.streamId, { el: thinkSItem, body: thinkSPre, buf: '' });
                        }
                        if (!streamingTarget) assistantDiv.textContent = '…';
                    } else if ((_et === 'thinking_stream_delta' || _et === 'reasoning_chain_stream_delta') && _ed && _ed.streamId) {
                        var tsD = wsThinkingStreams.get(_ed.streamId);
                        if (tsD) {
                            var mergeThink = (typeof window.mergeStreamBuffer === 'function')
                                ? window.mergeStreamBuffer
                                : function (cur, dlt) {
                                    var normT = (typeof window.normalizeStreamingDeltaJs === 'function')
                                        ? window.normalizeStreamingDeltaJs(cur, dlt) : [cur + dlt, dlt];
                                    return normT[0];
                                };
                            tsD.buf = mergeThink(tsD.buf, _em || '', _ed);
                            if (typeof formatMarkdown === 'function') {
                                tsD.body.innerHTML = formatMarkdown(tsD.buf);
                            } else {
                                tsD.body.textContent = tsD.buf;
                            }
                        }
                        if (!streamingTarget) assistantDiv.textContent = '…';
                    } else if ((_et === 'thinking_stream_end' || _et === 'reasoning_chain_stream_end') && _ed.streamId) {
                        var tsE = wsThinkingStreams.get(_ed.streamId);
                        if (tsE) {
                            var fullThink = (_em != null && _em !== '') ? String(_em) : tsE.buf;
                            if (typeof formatMarkdown === 'function') {
                                tsE.body.innerHTML = formatMarkdown(fullThink);
                            } else {
                                tsE.body.textContent = fullThink;
                            }
                            wsThinkingStreams.delete(_ed.streamId);
                        }
                    } else if ((_et === 'thinking' || _et === 'reasoning_chain') && _em) {
                        // Skip if streamId already exists to avoid duplicates
                        if (_ed.streamId && wsThinkingStreams.has(_ed.streamId)) {
                            // Handled by *_stream_*
                        } else {
                            var isRc = _et === 'reasoning_chain';
                            var thinkLabel = wsTOr(isRc ? 'chat.reasoningChain' : 'chat.aiThinking', isRc ? 'Reasoning Process' : 'AI Thinking');
                            var thinkEm = isRc ? '🔗' : '🤔';
                            appendTimelineItem(isRc ? 'reasoning_chain' : 'thinking', webshellAgentPx(_ed) + thinkEm + ' ' + thinkLabel, _em, _ed);
                        }
                        if (!streamingTarget) assistantDiv.textContent = '…';

                    // ─── Warning ───
                    } else if (_et === 'warning') {
                        appendTimelineItem('warning', '⚠️ ' + (_em || ''), '', _ed);

                    // ─── Tool calls ───
                    } else if (_et === 'tool_calls_detected' && _ed) {
                        var count = _ed.count || 0;
                        var detectedLabel = wsTOr('chat.toolCallsDetected', '') || ('Detected ' + count + ' tool calls');
                        if (typeof window.t === 'function') {
                            try { detectedLabel = window.t('chat.toolCallsDetected', { count: count }); } catch (e) { /* */ }
                        }
                        appendTimelineItem('tool_calls_detected', webshellAgentPx(_ed) + '🔧 ' + detectedLabel, _em || '', _ed);
                        if (!streamingTarget) assistantDiv.textContent = '…';
                    } else if (_et === 'tool_call' && _ed) {
                        var tn = _ed.toolName || 'unknown tool';
                        var idx = _ed.index || 0;
                        var total = _ed.total || 0;
                        var callTitle = typeof window.formatToolCallTimelineTitle === 'function'
                            ? window.formatToolCallTimelineTitle(tn, idx, total)
                            : (wsTOr('chat.callTool', '') || ('Calling tool: ' + tn + (total ? ' (' + idx + '/' + total + ')' : '')));
                        var callItem = appendTimelineItem('tool_call', webshellAgentPx(_ed) + '🔧 ' + callTitle, _em || '', _ed);
                        if (_ed.toolCallId && callItem) {
                            wsToolCallItems.set(_ed.toolCallId, callItem);
                        }
                        if (!streamingTarget) assistantDiv.textContent = '…';

                    // ─── Tool result (final) ───
                    } else if (_et === 'tool_result' && _ed) {
                        var wsLiveState = typeof window.getToolResultDisplayState === 'function' ? window.getToolResultDisplayState(_ed) : { success: _ed.success !== false };
                        var blocked = wsLiveState.kind === 'blocked';
                        var success = wsLiveState.success;
                        var tname = _ed.toolName || 'tool';
                        var merged = false;
                        if (_ed.toolCallId) {
                            var streamSt = wsToolResultStreams.get(_ed.toolCallId);
                            var callElRes = wsToolCallItems.get(_ed.toolCallId) || (streamSt && streamSt.el);
                            if (callElRes && typeof window.mergeToolResultIntoCallItem === 'function') {
                                window.mergeToolResultIntoCallItem(callElRes, _ed);
                                merged = true;
                                wsToolResultStreams.delete(_ed.toolCallId);
                                wsToolCallItems.delete(_ed.toolCallId);
                            }
                        }
                        if (!merged) {
                            var titleText = wsTOr(blocked ? 'chat.toolExecBlocked' : (success ? 'chat.toolExecComplete' : 'chat.toolExecFailed'), '') ||
                                (tname + (blocked ? ' Blocked' : (success ? ' executecomplete' : ' executefailed')));
                            if (typeof window.t === 'function') {
                                try { titleText = window.t(blocked ? 'chat.toolExecBlocked' : (success ? 'chat.toolExecComplete' : 'chat.toolExecFailed'), { name: tname }); } catch (e) { /* */ }
                            }
                            var title = webshellAgentPx(_ed) + (blocked ? '🛡 ' : (success ? '✅ ' : '❌ ')) + titleText;
                            var sub = _em || (_ed.result ? String(_ed.result).slice(0, 300) : '');
                            appendTimelineItem('tool_result', title, sub, _ed);
                        }
                        if (!streamingTarget) assistantDiv.textContent = '…';

                    // ─── Eino sub-agent reply streaming ───
                    } else if (_et === 'eino_agent_reply_stream_start' && _ed.streamId) {
                        if (einoSubReplyStreams.has(_ed.streamId)) {
                            var stExist = einoSubReplyStreams.get(_ed.streamId);
                            stExist.buf = '';
                            var preExist = stExist.el && stExist.el.querySelector('.webshell-eino-reply-stream-body');
                            if (preExist) preExist.textContent = '';
                        } else {
                        var repTS = wsTOr('chat.einoAgentReplyTitle', 'sub-agent reply');
                        var runTS = wsTOr('timeline.running', 'Executing...');
                        var items = document.createElement('div');
                        items.className = 'webshell-AI-timeline-item webshell-AI-timeline-eino_agent_reply';
                        items.innerHTML = '<span class="webshell-AI-timeline-title">' + escapeHtml(webshellAgentPx(_ed) + '💬 ' + repTS + ' · ' + runTS) + '</span>';
                        timelineContainer.appendChild(items);
                        timelineContainer.classList.add('has- items');
                        einoSubReplyStreams.set(_ed.streamId, { el: items, buf: '' });
                        }
                        if (!streamingTarget) assistantDiv.textContent = '…';
                    } else if (_et === 'eino_agent_reply_stream_delta' && _ed.streamId) {
                        var stD = einoSubReplyStreams.get(_ed.streamId);
                        if (stD) {
                            var mergeSub = (typeof window.mergeStreamBuffer === 'function')
                                ? window.mergeStreamBuffer
                                : function (cur, dlt) {
                                    var normS = (typeof window.normalizeStreamingDeltaJs === 'function')
                                        ? window.normalizeStreamingDeltaJs(cur, dlt) : [cur + dlt, dlt];
                                    return normS[0];
                                };
                            stD.buf = mergeSub(stD.buf, _em || '', _ed);
                            var preD = stD.el.querySelector('.webshell-eino-reply-stream-body');
                            if (!preD) {
                                preD = document.createElement('pre');
                                preD.className = 'webshell-AI-timeline-msg webshell-eino-reply-stream-body';
                                preD.style.whiteSpace = 'pre-wrap';
                                stD.el.appendChild(preD);
                            }
                            if (typeof formatMarkdown === 'function') {
                                preD.innerHTML = formatMarkdown(stD.buf);
                            } else {
                                preD.textContent = stD.buf;
                            }
                        }
                        if (!streamingTarget) assistantDiv.textContent = '…';
                    } else if (_et === 'eino_agent_reply_stream_end' && _ed.streamId) {
                        var stE = einoSubReplyStreams.get(_ed.streamId);
                        if (stE) {
                            var fullE = (_em != null && _em !== '') ? String(_em) : stE.buf;
                            var repTE = wsTOr('chat.einoAgentReplyTitle', 'sub-agent reply');
                            var titE = stE.el.querySelector('.webshell-AI-timeline-title');
                            if (titE) titE.textContent = webshellAgentPx(_ed) + '💬 ' + repTE;
                            var preE = stE.el.querySelector('.webshell-eino-reply-stream-body');
                            if (!preE) {
                                preE = document.createElement('pre');
                                preE.className = 'webshell-AI-timeline-msg webshell-eino-reply-stream-body';
                                preE.style.whiteSpace = 'pre-wrap';
                                stE.el.appendChild(preE);
                            }
                            if (typeof formatMarkdown === 'function') {
                                preE.innerHTML = formatMarkdown(fullE);
                            } else {
                                preE.textContent = fullE;
                            }
                            einoSubReplyStreams.delete(_ed.streamId);
                        }
                        if (!streamingTarget) assistantDiv.textContent = '…';
                    } else if (_et === 'eino_agent_reply' && _em) {
                        var replyT = wsTOr('chat.einoAgentReplyTitle', 'sub-agent reply');
                        appendTimelineItem('eino_agent_reply', webshellAgentPx(_ed) + '💬 ' + replyT, _em, _ed);
                        if (!streamingTarget) assistantDiv.textContent = '…';
                    }
                } catch (e) { /* ignore parse error */ }
            }
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
            return reader.read().then(processChunk);
        });
    }).catch(function (err) {
        var msg = err && err.message ? err.message : String(err);
        var isAbort = /abort/i.test(msg);
        if (!isAbort) {
            renderWebshellAiErrorMessage(assistantDiv, 'Request error: ' + msg);
        }
    }).then(function () {
        webshellAiAbortController = null;
        webshellAiStreamReader = null;
        wsSetAiSendingState(false);
        if (assistantDiv.textContent === '…' && !streamingTarget) {
            // No response content; keep plaintext hint
            assistantDiv.textContent = 'No reply content';
        } else if (streamingTarget) {
            // Stream end: stop typing animation before Markdown rendering
            webshellStreamingTypingId += 1;
            // Render full Markdown content
            if (typeof formatMarkdown === 'function') {
                assistantDiv.innerHTML = formatMarkdown(streamingTarget);
            } else {
                assistantDiv.textContent = streamingTarget;
            }
        }
        // Collapse execution details beneath assistant response
        if (timelineContainer && timelineContainer.classList.contains('has- items') && !timelineContainer.closest('.webshell-AI-process-block')) {
            var headerLabel = (typeof window.t === 'function') ? (window.t('chat.penetrationTestDetail') || 'taskexecuteDetails') : 'taskexecuteDetails';
            var wrap = document.createElement('div');
            wrap.className = 'process-details-container webshell-AI-process-block';
            wrap.innerHTML = '<button type="button" class="webshell-AI-process-toggle" aria-expanded="false">' + escapeHtml(headerLabel) + ' <span class="ws-toggle-icon">▶</span></button><div class="process-details-content"></div>';
            var contentDiv = wrap.querySelector('.process-details-content');
            contentDiv.appendChild(timelineContainer);
            timelineContainer.classList.add('progress-timeline');
            messagesContainer.insertBefore(wrap, assistantDiv.nextSibling);
            var toggleBtn = wrap.querySelector('.webshell-AI-process-toggle');
            var toggleIcon = wrap.querySelector('.ws-toggle-icon');
            toggleBtn.addEventListener('click', function () {
                var isExpanded = timelineContainer.classList.contains('expanded');
                timelineContainer.classList.toggle('expanded');
                toggleBtn.setAttribute('aria-expanded', !isExpanded);
                if (toggleIcon) toggleIcon.textContent = isExpanded ? '▶' : '▼';
            });
        }
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
    });
}

// Typewriter effect: write target text incrementally
function runWebshellAiStreamingTyping(el, target, ID, scrollContainer) {
    if (!el || ID === undefined) return;
    var chunkSize = 3;
    var delayMs = 24;
    function tick() {
        if (ID !== webshellStreamingTypingId) return;
        var cur = el.textContent || '';
        if (cur.length >= target.length) {
            el.textContent = target;
            if (scrollContainer) scrollContainer.scrollTop = scrollContainer.scrollHeight;
            return;
        }
        var next = target.slice(0, cur.length + chunkSize);
        el.textContent = next;
        if (scrollContainer) scrollContainer.scrollTop = scrollContainer.scrollHeight;
        setTimeout(tick, delayMs);
    }
    if (el.textContent.length < target.length) setTimeout(tick, delayMs);
}

function getWebshellHistory(connId) {
    if (!connId) return [];
    if (!webshellHistoryByConn[connId]) webshellHistoryByConn[connId] = [];
    return webshellHistoryByConn[connId];
}
function pushWebshellHistory(connId, cmd) {
    if (!connId || !cmd) return;
    if (!webshellHistoryByConn[connId]) webshellHistoryByConn[connId] = [];
    var h = webshellHistoryByConn[connId];
    if (h[h.length - 1] === cmd) return;
    h.push(cmd);
    if (h.length > WEBSHELL_HISTORY_MAX) h.shift();
}

// Execute quick command and write to current terminal
function runQuickCommand(cmd) {
    if (!webshellCurrentConn || !webshellTerminalInstance) return;
    if (webshellRunning || webshellTerminalrunning) return;
    var term = webshellTerminalInstance;
    var connId = webshellCurrentConn.id;
    var sessionId = getActiveWebshellTerminalSessionId(connId);
    var terminalKey = getWebshellTerminalSessionKey(connId, sessionId);
    term.writeln('');
    pushWebshellHistory(terminalKey, cmd);
    appendWebshellTerminalLog(terminalKey, '\n$ ' + cmd + '\n');
    webshellRunning = true;
    setWebshellTerminalStatus(true);
    execWebshellCommand(webshellCurrentConn, cmd).then(function (out) {
        var s = String(out || '').replace(/\r\n/g, '\n').replace(/\r/g, '\n');
        s.split('\n').forEach(function (line) { term.writeln(line.replace(/\r/g, '')); });
        appendWebshellTerminalLog(terminalKey, s + '\n');
        term.write(WEBSHELL_PROMPT);
    }).catch(function (err) {
        var em = (err && err.message ? err.message : wsT('WebShell.execError'));
        term.writeln('\x1b[31m' + em + '\x1b[0m');
        appendWebshellTerminalLog(terminalKey, em + '\n');
        term.write(WEBSHELL_PROMPT);
    }).finally(function () {
        webshellRunning = false;
        setWebshellTerminalStatus(false);
        renderWebshellTerminalSessions(webshellCurrentConn);
    });
}

// ---------- Virtual Terminal (xterm + line execution) ----------
function initWebshellTerminal(conn) {
    const container = document.getElementById('webshell-terminal-container');
    if (!container || typeof Terminal === 'undefined') {
        if (container) {
            container.innerHTML = '<p class="terminal-error">' + escapeHtml('xterm.js not loaded, please refresh page') + '</p>';
        }
        return;
    }

    const term = new Terminal({
        cursorBlink: true,
        cursorStyle: 'underline',
        fontSize: 13,
        fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        lineHeight: 1.2,
        scrollback: 2000,
        theme: {
            background: '#0d1117',
            foreground: '#e6edf3',
            cursor: '#58a6ff',
            cursorAccent: '#0d1117',
            selection: 'rgba(88, 166, 255, 0.3)'
        }
    });

    let FitAddon = null;
    if (typeof Fitaddon !== 'undefined') {
        const FitCtor = Fitaddon.Fitaddon || Fitaddon;
        FitAddon = new FitCtor();
        term.loadAddon(FitAddon);
    }

    term.open(container);
    // Fit terminal before writing content to ensure proper dimensions
    try {
        if (FitAddon) FitAddon.fit();
    } catch (e) {}
    setWebshellTerminalStatus(false);
    var connId = conn && conn.id ? conn.id : '';
    var sessionId = getActiveWebshellTerminalSessionId(connId);
    var terminalKey = getWebshellTerminalSessionKey(connId, sessionId);
    var cachedLog = getWebshellTerminalLog(terminalKey);
    if (cachedLog) {
        // Restore content using CRLF to prevent staircasing
        term.write(String(cachedLog).replace(/\r\n/g, '\n').replace(/\r/g, '\n').replace(/\n/g, '\r\n'));
    }
    term.write(WEBSHELL_PROMPT);

    // Write output line by line
    function writeWebshellOutput(term, text, isError) {
        if (!term || !text) return;
        var s = String(text).replace(/\r\n/g, '\n').replace(/\r/g, '\n');
        var lines = s.split('\n');
        var prefix = isError ? '\x1b[31m' : '';
        var suffix = isError ? '\x1b[0m' : '';
        term.write(prefix);
        for (var i = 0; i < lines.length; i++) {
            term.writeln(lines[i].replace(/\r/g, ''));
        }
        term.write(suffix);
    }

    term.onData(function (data) {
        // Ctrl+L clear screen
        if (data === '\x0c') {
            term.clear();
            webshellLineBuffer = '';
            webshellHistoryIndex = -1;
            term.write(WEBSHELL_PROMPT);
            clearWebshellTerminalLog(terminalKey);
            return;
        }
        // Ctrl+C: remote interruption not supported; print notice and return to prompt
        if (data === '\x03') {
            if (webshellTerminalrunning) {
                writeWebshellOutput(term, '^C (interrupting remote command is currently not supported)', true);
                appendWebshellTerminalLog(terminalKey, '^C (interrupting remote command is currently not supported)\n');
            }
            webshellLineBuffer = '';
            webshellHistoryIndex = -1;
            term.write(WEBSHELL_PROMPT);
            return;
        }
        // Ctrl+U: clear current input line
        if (data === '\x15') {
            webshellLineBuffer = '';
            term.write('\x1b[2K\r' + WEBSHELL_PROMPT);
            return;
        }
        // Up/Down arrow: command history
        if (data === '\x1b[A' || data === '\x1bOA') {
            var hist = getWebshellHistory(terminalKey);
            if (hist.length === 0) return;
            webshellHistoryIndex = webshellHistoryIndex < 0 ? hist.length : Math.max(0, webshellHistoryIndex - 1);
            webshellLineBuffer = hist[webshellHistoryIndex] || '';
            term.write('\x1b[2K\r' + WEBSHELL_PROMPT + webshellLineBuffer);
            return;
        }
        if (data === '\x1b[B' || data === '\x1bOB') {
            var hist2 = getWebshellHistory(terminalKey);
            if (hist2.length === 0) return;
            webshellHistoryIndex = webshellHistoryIndex < 0 ? -1 : Math.min(hist2.length - 1, webshellHistoryIndex + 1);
            if (webshellHistoryIndex < 0) webshellLineBuffer = '';
            else webshellLineBuffer = hist2[webshellHistoryIndex] || '';
            term.write('\x1b[2K\r' + WEBSHELL_PROMPT + webshellLineBuffer);
            return;
        }
        // Enter: send line to backend for execution
        if (data === '\r' || data === '\n') {
            term.writeln('');
            var cmd = webshellLineBuffer.trim();
            webshellLineBuffer = '';
            webshellHistoryIndex = -1;
            if (cmd) {
                if (webshellRunning) {
                    writeWebshellOutput(term, wsT('WebShell.waitFinish'), true);
                    appendWebshellTerminalLog(terminalKey, (wsT('WebShell.waitFinish') || 'Please wait for current command to complete') + '\n');
                    term.write(WEBSHELL_PROMPT);
                    return;
                }
                pushWebshellHistory(terminalKey, cmd);
                appendWebshellTerminalLog(terminalKey, '$ ' + cmd + '\n');
                webshellRunning = true;
                setWebshellTerminalStatus(true);
                renderWebshellTerminalSessions(conn);
                execWebshellCommand(webshellCurrentConn, cmd).then(function (out) {
                    webshellRunning = false;
                    setWebshellTerminalStatus(false);
                    renderWebshellTerminalSessions(conn);
                    if (out && out.length) {
                        writeWebshellOutput(term, out, false);
                        appendWebshellTerminalLog(terminalKey, String(out).replace(/\r\n/g, '\n').replace(/\r/g, '\n') + '\n');
                    }
                    term.write(WEBSHELL_PROMPT);
                }).catch(function (err) {
                    webshellRunning = false;
                    setWebshellTerminalStatus(false);
                    renderWebshellTerminalSessions(conn);
                    var errMsg = err && err.message ? err.message : wsT('WebShell.execError');
                    writeWebshellOutput(term, errMsg, true);
                    appendWebshellTerminalLog(terminalKey, String(errMsg || '') + '\n');
                    term.write(WEBSHELL_PROMPT);
                });
            } else {
                term.write(WEBSHELL_PROMPT);
            }
            return;
        }
        // Multiline paste: execute lines sequentially
        if (data.indexOf('\n') !== -1 || data.indexOf('\r') !== -1) {
            var full = (webshellLineBuffer + data).replace(/\r\n/g, '\n').replace(/\r/g, '\n');
            var lines = full.split('\n');
            webshellLineBuffer = lines.pop() || '';
            if (lines.length > 0 && !webshellRunning && webshellCurrentConn) {
                var runNext = function (idx) {
                    if (idx >= lines.length) {
                        term.write(WEBSHELL_PROMPT + webshellLineBuffer);
                        return;
                    }
                    var line = lines[idx].trim();
                    if (!line) { runNext(idx + 1); return; }
                    pushWebshellHistory(terminalKey, line);
                    appendWebshellTerminalLog(terminalKey, '$ ' + line + '\n');
                    webshellRunning = true;
                    setWebshellTerminalStatus(true);
                    renderWebshellTerminalSessions(conn);
                    execWebshellCommand(webshellCurrentConn, line).then(function (out) {
                        if (out && out.length) {
                            writeWebshellOutput(term, out, false);
                            appendWebshellTerminalLog(terminalKey, String(out).replace(/\r\n/g, '\n').replace(/\r/g, '\n') + '\n');
                        }
                        webshellRunning = false;
                        setWebshellTerminalStatus(false);
                        renderWebshellTerminalSessions(conn);
                        runNext(idx + 1);
                    }).catch(function (err) {
                        var em = err && err.message ? err.message : wsT('WebShell.execError');
                        writeWebshellOutput(term, em, true);
                        appendWebshellTerminalLog(terminalKey, String(em || '') + '\n');
                        webshellRunning = false;
                        setWebshellTerminalStatus(false);
                        renderWebshellTerminalSessions(conn);
                        runNext(idx + 1);
                    });
                };
                runNext(0);
            } else {
                term.write(data);
            }
            return;
        }
        // Backspace
        if (data === '\x7f' || data === '\b') {
            if (webshellLineBuffer.length > 0) {
                webshellLineBuffer = webshellLineBuffer.slice(0, -1);
                term.write('\b \b');
            }
            return;
        }
        webshellLineBuffer += data;
        term.write(data);
    });

    webshellTerminalInstance = term;
    webshellTerminalFitaddon = FitAddon;
    // Re-fit with delay to ensure stable cursor placement
    setTimeout(function () {
        try { if (FitAddon) FitAddon.fit(); } catch (e) {}
    }, 100);
    // Re-fit on container resize
    if (FitAddon && typeof ResizeObserver !== 'undefined' && container) {
        webshellTerminalResizeContainer = container;
        webshellTerminalResizeObserver = new ResizeObserver(function () {
            try { FitAddon.fit(); } catch (e) {}
        });
        webshellTerminalResizeObserver.observe(container);
    }
    renderWebshellTerminalSessions(conn);
}

// Call backend to execute command
function execWebshellCommand(conn, command) {
    return new Promise(function (resolve, reject) {
        if (typeof apiFetch === 'undefined') {
            reject(new Error('apiFetch is undefined'));
            return;
        }
        apiFetch('/api/WebShell/exec', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                URL: conn.URL,
                password: conn.password || '',
                type: conn.type || 'PHP',
                method: (conn.method || 'POST').toLowerCase(),
                cmd_param: conn.cmdParam || '',
                encoding: webshellConnEncoding(conn),
                OS: webshellConnOS(conn),
                connection_id: conn.id || '',
                command: command
            })
        }).then(function (r) { return r.json(); })
            .then(function (data) {
                if (data && data.output !== undefined) resolve(data.output || '');
                else if (data && data.error) reject(new Error(data.error));
                else resolve('');
            })
            .catch(reject);
    });
}

// ---------- Files ----------
function webshellFileListDir(conn, path) {
    const listEl = document.getElementById('webshell-file-list');
    if (!listEl) return;
    listEl.innerHTML = '<div class="webshell-loading">' + wsT('common.refresh') + '...</div>';

    if (typeof apiFetch === 'undefined') {
        listEl.innerHTML = '<div class="webshell-file-error">apiFetch is undefined</div>';
        return;
    }

    apiFetch('/api/WebShell/file', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: webshellFileRequestBody(conn, { action: 'list', path: path })
    }).then(function (r) { return r.json(); })
        .then(function (data) {
            applyWebshellDetectedOS(conn, data);
            if (!data.ok && data.error) {
                listEl.innerHTML = '<div class="webshell-file-error">' + escapeHtml(data.error) + '</div><pre class="webshell-file-raw">' + escapeHtml(data.output || '') + '</pre>';
                return;
            }
            var normalizedPath = normalizeWebshellPath(path);
            var inferredPath = inferPathFromWindowsDirOutput(data.output || '');
            var displayPath = inferredPath || normalizedPath;
            listEl.dataset.currentPath = displayPath;
            listEl.dataset.rawOutput = data.output || '';
            var pathInput = document.getElementById('webshell-file-path');
            if (pathInput) pathInput.value = displayPath;
            renderFileList(listEl, displayPath, data.output || '', conn);
        })
        .catch(function (err) {
            listEl.innerHTML = '<div class="webshell-file-error">' + escapeHtml(err && err.message ? err.message : wsT('WebShell.execError')) + '</div>';
        });
}

function normalizeLsMtime(month, day, timeOrYear) {
    if (!month || !day || !timeOrYear) return '';
    var token = String(timeOrYear).trim();
    if (/^\d{4}$/.test(token)) return token + ' ' + month + ' ' + day;
    var now = new Date();
    var year = now.getFullYear();
    if (/^\d{1,2}:\d{2}$/.test(token)) {
        var monthMap = { Jan: 0, Feb: 1, Mar: 2, Apr: 3, May: 4, Jun: 5, Jul: 6, Aug: 7, Sep: 8, Oct: 9, Nov: 10, Dec: 11 };
        var m = monthMap[month];
        var d = parseInt(day, 10);
        if (m != null && !isNaN(d)) {
            var inferred = new Date(year, m, d);
            if (inferred.getTime() > now.getTime()) year = year - 1;
        }
        return year + ' ' + month + ' ' + day + ' ' + token;
    }
    return month + ' ' + day + ' ' + token;
}

function modeToType(mode) {
    if (!mode || !mode.length) return '';
    var c = mode.charAt(0);
    if (c === 'd') return 'dir';
    if (c === '-') return 'file';
    if (c === 'l') return 'link';
    if (c === 'c') return 'char';
    if (c === 'b') return 'block';
    if (c === 's') return 'socket';
    if (c === 'p') return 'pipe';
    return c;
}

function parseWindowsDirEntry(line) {
    var m = String(line || '').match(/^(\d{4}[\/-]\d{1,2}[\/-]\d{1,2})\s+(\d{1,2}:\d{2})(?:\s*(AM|PM))?\s+(<[^>]+>|[\d,]+)\s+(.+?)\s*$/i);
    if (!m) return null;
    var kind = (m[4] || '').trim();
    var name = (m[5] || '').trim();
    if (!name || name === '.' || name === '..') return null;
    var isDir = /^<(dir|junction|symlinkd)>$/i.test(kind);
    var size = isDir ? '' : kind.replace(/,/g, '');
    var mtime = (m[1] + ' ' + m[2] + (m[3] ? (' ' + m[3].toUpperCase()) : '')).trim();
    return {
        name: name,
        isDir: isDir,
        size: size,
        mtime: mtime,
        mode: isDir ? 'd' : '-',
        owner: '',
        group: '',
        type: isDir ? 'dir' : 'file'
    };
}

function parseWebshellListItems(rawOutput) {
    var lines = (rawOutput || '').split(/\n/).filter(function (l) { return l.trim(); });
    var  items = [];
    for (var i = 0; i < lines.length; i++) {
        var line = lines[i];
        var trimmedLine = String(line || '').trim();
        // ls -la first line often has 'total 12' (or locale equivalent); skip it.
        if (/^(total|总计)\s+\d+$/i.test(trimmedLine)) continue;
        // dir headers and footers; skip header/footer summary lines.
        if (/^(驱动器|卷的序列号|volume in drive|volume serial number is|directory of)/i.test(trimmedLine)) continue;
        if (/^[A-Za-z]:\\.*\s+(的directory|的目录)$/i.test(trimmedLine)) continue;
        if (/^\d+\s+(files|file\(s\))\s+[\d,]+\s+(字节|bytes?)$/i.test(trimmedLine)) continue;
        if (/^\d+\s+(个目录|个directory|dir\(s\))\s+[\d,]+\s+(可用字节|Available字节|bytes free)$/i.test(trimmedLine)) continue;
        if (/^[^>\n]*>\s*dir(?:\s|$)/i.test(trimmedLine)) continue;
        var name = '';
        var isDir = false;
        var size = '';
        var mode = '';
        var mtime = '';
        var owner = '';
        var group = '';
        var type = '';
        var mLs = line.match(/^(\S+)\s+(\d+)\s+(\S+)\s+(\S+)\s+(\d+)\s+([A-Za-z]{3})\s+(\d{1,2})\s+(\S+)\s+(.+)$/);
        if (mLs) {
            mode = mLs[1];
            owner = mLs[3];
            group = mLs[4];
            size = mLs[5];
            mtime = normalizeLsMtime(mLs[6], mLs[7], mLs[8]);
            name = (mLs[9] || '').trim();
            isDir = mode && mode.startsWith('d');
            type = modeToType(mode);
        } else {
            var winItem = parseWindowsDirEntry(line);
            if (winItem) {
                 items.push({
                    name: winItem.name,
                    isDir: winItem.isDir,
                    line: line,
                    size: winItem.size,
                    mode: winItem.mode,
                    mtime: winItem.mtime,
                    owner: winItem.owner,
                    group: winItem.group,
                    type: winItem.type
                });
                continue;
            }
            // Fallback parser for Unix permissions to prevent misidentifying dir summary lines.
            if (/^[bcdlps-][rwxStTs-]{9}[+.@]?\s/.test(line)) {
                var parts = line.trim().split(/\s+/);
                if (parts.length >= 9) {
                    name = parts.slice(8).JOIN(' ').trim();
                } else {
                    name = parts.length ? parts[parts.length - 1].trim() : line.trim();
                }
                if (name === '.' || name === '..') continue;
                isDir = line.startsWith('d');
                parts = line.split(/\s+/);
                if (parts.length >= 5) { mode = parts[0]; size = parts[4]; }
                if (parts.length >= 4) { owner = parts[2] || ''; group = parts[3] || ''; }
                if (parts.length >= 8 && /^[A-Za-z]{3}$/.test(parts[5])) mtime = normalizeLsMtime(parts[5], parts[6], parts[7]);
                type = modeToType(mode);
            } else {
                continue;
            }
        }
        if (name === '.' || name === '..') continue;
         items.push({ name: name, isDir: isDir, line: line, size: size, mode: mode, mtime: mtime, owner: owner, group: group, type: type });
    }
    return  items;
}

function fetchWebshellDirectoryItems(conn, path) {
    if (!conn || typeof apiFetch === 'undefined') return Promise.resolve([]);
    return apiFetch('/api/WebShell/file', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: webshellFileRequestBody(conn, { action: 'list', path: path })
    }).then(function (r) { return r.json(); }).then(function (data) {
        applyWebshellDetectedOS(conn, data);
        if (!data || data.error || !data.ok) return [];
        return parseWebshellListItems(data.output || '');
    }).catch(function () {
        return [];
    });
}

function renderFileList(listEl, currentPath, rawOutput, conn, nameFilter) {
    currentPath = normalizeWebshellPath(currentPath);
    var  items = parseWebshellListItems(rawOutput);
    var selectedPath = getWebshellSelectedFile(conn || webshellCurrentConn);
    if (nameFilter && nameFilter.trim()) {
        var f = nameFilter.trim().toLowerCase();
         items =  items.filter(function (item) { return item.name.toLowerCase().indexOf(f) !== -1; });
    }
    // Breadcrumb
    var breadcrumbEl = document.getElementById('webshell-file-breadcrumb');
    if (breadcrumbEl) {
        var absolute = currentPath.startsWith('/');
        var parts = (currentPath === '.' || currentPath === '/' || currentPath === '') ? [] : currentPath.replace(/^\//, '').split('/');
        breadcrumbEl.innerHTML = '<a href="#" class="webshell-breadcrumb-item" data-path="' + (absolute ? '/' : '.') + '">' + (wsT('WebShell.breadcrumbHome') || 'Root') + '</a>' +
            parts.map(function (p, idx) {
                var path = (absolute ? '/' : '') + parts.slice(0, idx + 1).JOIN('/');
                return ' / <a href="#" class="webshell-breadcrumb-item" data-path="' + escapeHtml(path) + '">' + escapeHtml(p) + '</a>';
            }).JOIN('');
    }
    renderDirectoryTree(currentPath,  items, conn);
    var HTML = '';
    if ( items.length === 0) {
        // Empty state when directory is empty or filtered
        if (rawOutput.trim() && !nameFilter) {
            HTML = '<pre class="webshell-file-raw">' + escapeHtml(rawOutput) + '</pre>';
        } else {
            HTML = '<table class="webshell-file-table"><thead><tr><th class="webshell-col-check"><input type="checkbox" id="webshell-file-select-all" title="' + (wsT('WebShell.selectAll') || 'Select All') + '" /></th><th>' + wsT('WebShell.filePath') + '</th><th class="webshell-col-size">' + (wsT('WebShell.colSize') || 'Size') + '</th><th class="webshell-col-mtime">' + (wsT('WebShell.colModifiedAt') || 'Modified Time') + '</th><th class="webshell-col-owner">' + (wsT('WebShell.colOwner') || 'Owner') + '</th><th class="webshell-col-perms">' + (wsT('WebShell.colPerms') || 'Permissions') + '</th><th class="webshell-col-actions"></th></tr></thead><tbody>' +
                '<tr><td colspan="7" class="webshell-file-empty-state">' + (wsT('common.noData') || 'No files') + '</td></tr>' +
                '</tbody></TABLE>';
        }
    } else {
        HTML = '<table class="webshell-file-table"><thead><tr><th class="webshell-col-check"><input type="checkbox" id="webshell-file-select-all" title="' + (wsT('WebShell.selectAll') || 'Select All') + '" /></th><th>' + wsT('WebShell.filePath') + '</th><th class="webshell-col-size">' + (wsT('WebShell.colSize') || 'Size') + '</th><th class="webshell-col-mtime">' + (wsT('WebShell.colModifiedAt') || 'Modified Time') + '</th><th class="webshell-col-owner">' + (wsT('WebShell.colOwner') || 'Owner') + '</th><th class="webshell-col-perms">' + (wsT('WebShell.colPerms') || 'Permissions') + '</th><th class="webshell-col-actions"></th></tr></thead><tbody>';
        if (currentPath !== '.' && currentPath !== '') {
            HTML += '<tr><td></td><td><a href="#" class="webshell-file-link" data-path="' + escapeHtml(getWebshellParentPath(currentPath)) + '" data-idsDir="1">..</a></td><td></td><td></td><td></td><td></td><td></td></tr>';
        }
         items.forEach(function (item) {
            var pathNext = currentPath === '.' ? item.name : currentPath + '/' + item.name;
            var pathNextNorm = normalizeWebshellPath(pathNext);
            var nameClass = item.isDir ? 'is-dir' : 'is-file';
            HTML += '<tr class="' + (!item.isDir && selectedPath === pathNextNorm ? 'webshell-file-row-selected' : '') + '"><td class="webshell-col-check">';
            if (!item.isDir) HTML += '<input type="checkbox" class="webshell-file-cb" data-path="' + escapeHtml(pathNext) + '" />';
            HTML += '</td><td class="webshell-col-name"><a href="#" class="webshell-file-link ' + nameClass + '" title="' + escapeHtml(item.name) + '" data-path="' + escapeHtml(pathNext) + '" data-idsDir="' + (item.isDir ? '1' : '0') + '">' + escapeHtml(item.name) + (item.isDir ? '/' : '') + '</a></td>';
            HTML += '<td class="webshell-col-size">' + escapeHtml(item.size) + '</td>';
            HTML += '<td class="webshell-col-mtime">' + escapeHtml(item.mtime || '') + '</td>';
            HTML += '<td class="webshell-col-owner">' + escapeHtml(item.owner || '') + '</td>';
            HTML += '<td class="webshell-col-perms">' + escapeHtml(item.mode || '') + '</td>';
            HTML += '<td class="webshell-col-actions">';
            if (item.isDir) {
                HTML += '<button type="button" class="btn-ghost btn-sm webshell-file-rename" data-path="' + escapeHtml(pathNext) + '" data-name="' + escapeHtml(item.name) + '">' + (wsT('WebShell.rename') || 'Rename') + '</button>';
            } else {
                var actionsLabel = wsT('common.actions') || 'Actions';
                HTML += '<details class="webshell-row-actions"><summary class="btn-ghost btn-sm webshell-row-actions-btn" title="' + actionsLabel + '">' + actionsLabel + '</summary>' +
                    '<div class="webshell-row-actions-menu">' +
                    '<button type="button" class="btn-ghost btn-sm webshell-file-read" data-path="' + escapeHtml(pathNext) + '">' + wsT('WebShell.readFile') + '</button>' +
                    '<button type="button" class="btn-ghost btn-sm webshell-file-download" data-path="' + escapeHtml(pathNext) + '">' + wsT('WebShell.downloadFile') + '</button>' +
                    '<button type="button" class="btn-ghost btn-sm webshell-file-edit" data-path="' + escapeHtml(pathNext) + '">' + wsT('WebShell.editFile') + '</button>' +
                    '<button type="button" class="btn-ghost btn-sm webshell-file-rename" data-path="' + escapeHtml(pathNext) + '" data-name="' + escapeHtml(item.name) + '">' + (wsT('WebShell.rename') || 'Rename') + '</button>' +
                    '<button type="button" class="btn-ghost btn-sm webshell-file-del" data-path="' + escapeHtml(pathNext) + '">' + wsT('WebShell.deleteFile') + '</button>' +
                    '</div></details>';
            }
            HTML += '</td></tr>';
        });
        HTML += '</tbody></TABLE>';
    }
    listEl.innerHTML = HTML;

    listEl.querySelectorAll('.webshell-file-link').forEach(function (a) {
        a.addEventListener('click', function (e) {
            e.preventDefault();
            const path = a.getAttribute('data-path');
            const isDir = a.getAttribute('data-idsDir') === '1';
            const pathInput = document.getElementById('webshell-file-path');
            if (isDir) {
                setWebshellSelectedFile(webshellCurrentConn, '');
                if (pathInput) pathInput.value = path;
                webshellFileListDir(webshellCurrentConn, path);
            } else {
                // Retain directory context when opening file
                setWebshellSelectedFile(webshellCurrentConn, path);
                renderDirectoryTree(currentPath,  items, conn || webshellCurrentConn);
                webshellFileRead(webshellCurrentConn, path, listEl, currentPath);
            }
        });
    });
    listEl.querySelectorAll('.webshell-file-read').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
            e.preventDefault();
            var filePath = btn.getAttribute('data-path');
            setWebshellSelectedFile(webshellCurrentConn, filePath);
            renderDirectoryTree(currentPath,  items, conn || webshellCurrentConn);
            webshellFileRead(webshellCurrentConn, filePath, listEl, currentPath);
        });
    });
    listEl.querySelectorAll('.webshell-file-download').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
            e.preventDefault();
            webshellFileDownload(webshellCurrentConn, btn.getAttribute('data-path'));
        });
    });
    listEl.querySelectorAll('.webshell-file-edit').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
            e.preventDefault();
            webshellFileEdit(webshellCurrentConn, btn.getAttribute('data-path'), listEl);
        });
    });
    listEl.querySelectorAll('.webshell-file-del').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
            e.preventDefault();
            if (!confirm(wsT('WebShell.deleteConfirm'))) return;
            webshellFileDelete(webshellCurrentConn, btn.getAttribute('data-path'), function () {
                webshellFileListDir(webshellCurrentConn, document.getElementById('webshell-file-path').value.trim() || '.');
            });
        });
    });
    listEl.querySelectorAll('.webshell-file-rename').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
            e.preventDefault();
            webshellFileRename(webshellCurrentConn, btn.getAttribute('data-path'), btn.getAttribute('data-name'), listEl);
        });
    });
    var selectAll = document.getElementById('webshell-file-SELECT-ALL');
    if (selectAll) {
        selectAll.addEventListener('change', function () {
            listEl.querySelectorAll('.webshell-file-cb').forEach(function (cb) { cb.checked = selectAll.checked; });
        });
    }
    if (breadcrumbEl) {
        breadcrumbEl.querySelectorAll('.webshell-breadcrumb-item').forEach(function (a) {
            a.addEventListener('click', function (e) {
                e.preventDefault();
                var p = a.getAttribute('data-path');
                var pathInput = document.getElementById('webshell-file-path');
                if (pathInput) pathInput.value = p;
                webshellFileListDir(webshellCurrentConn, p);
            });
        });
    }
}

function renderDirectoryTree(currentPath,  items, conn) {
    var treeEl = document.getElementById('webshell-dir-tree');
    if (!treeEl) return;
    var state = getWebshellTreeState(conn || webshellCurrentConn);
    var curr = normalizeWebshellPath(currentPath);
    var dirs = ( items || []).filter(function (item) { return item && item.isDir; });
    if (!state) {
        treeEl.innerHTML = '<div class="webshell-empty">No directory</div>';
        return;
    }
    var tree = state.tree;
    var expanded = state.expanded;
    var loaded = state.loaded;
    var selectedPath = getWebshellSelectedFile(conn || webshellCurrentConn);
    var rootPath = curr.startsWith('/') ? '/' : '.';
    if (!tree[rootPath]) tree[rootPath] = [];
    if (expanded[rootPath] !== false) expanded[rootPath] = true;

    // Sync directory children to tree cache
    var childNodes = ( items || []).map(function (item) {
        var childPath = curr === '.' ? normalizeWebshellPath(item.name) : normalizeWebshellPath(curr + '/' + item.name);
        return {
            path: childPath,
            name: item.name,
            isDir: !!item.isDir
        };
    }).filter(function (n) { return !!n.path; });
    childNodes.sort(function (a, b) {
        // Directories first, then sorted by name
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        return (a.name || '').localeCompare(b.name || '');
    });
    tree[curr] = childNodes;
    loaded[curr] = true;
    childNodes.forEach(function (node) {
        if (node.isDir && !tree[node.path]) tree[node.path] = [];
    });

    // Build ancestor chain only for canonical paths
    var isRelativeUpChain = /^(?:\.\.\/)*\.\.$/.test(curr);
    var parts = (curr === '.' || curr === '/') ? [] : curr.replace(/^\//, '').split('/');
    var parentPath = rootPath;
    if (!isRelativeUpChain) {
        for (var i = 0; i < parts.length; i++) {
            var nextPath = parentPath === '.' ? parts[i] : normalizeWebshellPath(parentPath + '/' + parts[i]);
            if (!tree[parentPath]) tree[parentPath] = [];
            var parentChildren = tree[parentPath];
            var hasAncestorNode = parentChildren.some(function (n) { return n && n.path === nextPath; });
            if (!hasAncestorNode) {
                parentChildren.push({ path: nextPath, name: parts[i], isDir: true });
                parentChildren.sort(function (a, b) {
                    if (!!a.isDir !== !!b.isDir) return a.isDir ? -1 : 1;
                    return (a.name || '').localeCompare(b.name || '');
                });
            }
            if (!tree[nextPath]) tree[nextPath] = [];
            expanded[parentPath] = true;
            parentPath = nextPath;
        }
    }
    if (expanded[curr] == null) expanded[curr] = true;

    function renderNode(node, depth) {
        var path = node.path;
        var isDir = !!node.isDir;
        var children = isDir ? (tree[path] || []).slice() : [];
        var hasLoadedChildren = isDir ? (loaded[path] === true) : true;
        var canExpand = isDir && (path === rootPath || !hasLoadedChildren || children.length > 0);
        var hasChildren = children.length > 0;
        var isExpanded = isDir ? (expanded[path] === true) : false;
        var isActive = path === curr;
        var isSelectedFile = !isDir && path === selectedPath;
        var name = node.name;
        var icon = isDir ? (path === rootPath ? '🗂' : '📁') : '📄';
        var nodeHtml =
            '<div class="webshell-tree-node" data-depth="' + depth + '">' +
            '<div class="webshell-tree-row' + (isActive ? ' active' : '') + (isSelectedFile ? ' selected-file' : '') + '">' +
            '<button type="button" class="webshell-tree-toggle' + (canExpand ? '' : ' empty') + '" data-path="' + escapeHtml(path) + '">' + (canExpand ? (isExpanded ? '▾' : '▸') : '·') + '</button>' +
            '<button type="button" class="webshell-dir-item' + (isDir ? ' is-dir' : ' is-file') + '" title="' + escapeHtml(name) + '" data-path="' + escapeHtml(path) + '" data-idsDir="' + (isDir ? '1' : '0') + '"><span class="webshell-tree-icon">' + icon + '</span><span class="webshell-tree-name">' + escapeHtml(name) + '</span></button>' +
            '</div>';
        if (isDir && hasChildren && isExpanded) {
            nodeHtml += '<div class="webshell-tree-children">';
            for (var j = 0; j < children.length; j++) {
                nodeHtml += renderNode(children[j], depth + 1);
            }
            nodeHtml += '</div>';
        }
        nodeHtml += '</div>';
        return nodeHtml;
    }

    treeEl.innerHTML = '<div class="webshell-tree-root">' + renderNode({ path: rootPath, name: '/', isDir: true }, 0) + '</div>';
    treeEl.querySelectorAll('.webshell-tree-toggle').forEach(function (btn) {
        btn.addEventListener('click', function (e) {
            e.preventDefault();
            e.stopPropagation();
            var p = normalizeWebshellPath(btn.getAttribute('data-path') || '.');
            if (expanded[p] === true) {
                expanded[p] = false;
                renderDirectoryTree(curr,  items, conn || webshellCurrentConn);
                return;
            }
            if (loaded[p] === true) {
                expanded[p] = true;
                renderDirectoryTree(curr,  items, conn || webshellCurrentConn);
                return;
            }
            fetchWebshellDirectoryItems(conn || webshellCurrentConn, p).then(function (subItems) {
                var nextChildren = (subItems || []).map(function (it) {
                    return {
                        path: p === '.' ? normalizeWebshellPath(it.name) : normalizeWebshellPath(p + '/' + it.name),
                        name: it.name,
                        isDir: !!it.isDir
                    };
                }).filter(function (n) { return !!n.path; }).sort(function (a, b) {
                    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
                    return (a.name || '').localeCompare(b.name || '');
                });
                tree[p] = nextChildren;
                nextChildren.forEach(function (childNode) {
                    if (childNode.isDir) {
                        if (!tree[childNode.path]) tree[childNode.path] = [];
                        if (loaded[childNode.path] == null) loaded[childNode.path] = false;
                    }
                });
                loaded[p] = true;
                expanded[p] = true;
                renderDirectoryTree(curr,  items, conn || webshellCurrentConn);
            });
        });
    });
    treeEl.querySelectorAll('.webshell-dir-item').forEach(function (btn) {
        btn.addEventListener('click', function () {
            var p = normalizeWebshellPath(btn.getAttribute('data-path') || '.');
            var isDir = btn.getAttribute('data-idsDir') === '1';
            var pathInput = document.getElementById('webshell-file-path');
            if (isDir) {
                setWebshellSelectedFile(webshellCurrentConn, '');
                if (pathInput) pathInput.value = p;
                webshellFileListDir(webshellCurrentConn, p);
                return;
            }
            var listEl = document.getElementById('webshell-file-list');
            var browsePath = p.replace(/\/[^/]+$/, '') || '.';
            setWebshellSelectedFile(webshellCurrentConn, p);
            renderDirectoryTree(curr,  items, conn || webshellCurrentConn);
            if (listEl) webshellFileRead(webshellCurrentConn, p, listEl, browsePath);
        });
    });
}

function webshellFileListApplyFilter() {
    var listEl = document.getElementById('webshell-file-list');
    var path = listEl && listEl.dataset.currentPath ? listEl.dataset.currentPath : (document.getElementById('webshell-file-path') && document.getElementById('webshell-file-path').value.trim()) || '.';
    var raw = listEl && listEl.dataset.rawOutput ? listEl.dataset.rawOutput : '';
    var filterInput = document.getElementById('webshell-file-filter');
    var filter = filterInput ? filterInput.value : '';
    if (!listEl || !raw) return;
    renderFileList(listEl, path, raw, webshellCurrentConn, filter);
}

function webshellFileMkdir(conn, pathInput) {
    if (!conn || typeof apiFetch === 'undefined') return;
    var BASE = (pathInput && pathInput.value.trim()) || '.';
    var name = prompt(wsT('WebShell.newDir') || 'New Directory', 'newDir');
    if (name == null || !name.trim()) return;
    var path = BASE === '.' ? name.trim() : BASE + '/' + name.trim();
    apiFetch('/api/WebShell/file', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: webshellFileRequestBody(conn, { action: 'mkdir', path: path }) })
        .then(function (r) { return r.json(); })
        .then(function () { webshellFileListDir(conn, BASE); })
        .catch(function () { webshellFileListDir(conn, BASE); });
}

function webshellFileNewFile(conn, pathInput) {
    if (!conn || typeof apiFetch === 'undefined') return;
    var BASE = (pathInput && pathInput.value.trim()) || '.';
    var name = prompt(wsT('WebShell.newFile') || 'New File', 'newFile.txt');
    if (name == null || !name.trim()) return;
    var path = BASE === '.' ? name.trim() : BASE + '/' + name.trim();
    var content = prompt('Initial content (optional)', '');
    if (content === null) return;
    var listEl = document.getElementById('webshell-file-list');
    webshellFileWrite(conn, path, content || '', function () { webshellFileListDir(conn, BASE); }, listEl);
}

function webshellFileUpload(conn, pathInput) {
    if (!conn || typeof apiFetch === 'undefined') return;
    var BASE = (pathInput && pathInput.value.trim()) || '.';
    var input = document.createElement('input');
    input.type = 'file';
    input.multiple = false;
    input.onchange = function () {
        var file = input.files && input.files[0];
        if (!file) return;
        var reader = new FileReader();
        reader.onload = function () {
            var buf = reader.result;
            var bin = new Uint8Array(buf);
            var CHUNK = 32000;
            var base64Chunks = [];
            for (var i = 0; i < bin.length; i += CHUNK) {
                var slice = bin.subarray(i, Math.min(i + CHUNK, bin.length));
                var b64 = btoa(String.fromCharCode.apply(null, slice));
                base64Chunks.push(b64);
            }
            var path = BASE === '.' ? file.name : BASE + '/' + file.name;
            var listEl = document.getElementById('webshell-file-list');
            if (listEl) listEl.innerHTML = '<div class="webshell-loading">' + (wsT('WebShell.upload') || 'Upload') + '...</div>';
            var idx = 0;
            function sendNext() {
                if (idx >= base64Chunks.length) {
                    webshellFileListDir(conn, BASE);
                    return;
                }
                apiFetch('/api/WebShell/file', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: webshellFileRequestBody(conn, { action: 'upload_chunk', path: path, content: base64Chunks[idx], chunk_index: idx }) })
                    .then(function (r) { return r.json(); })
                    .then(function () { idx++; sendNext(); })
                    .catch(function () { idx++; sendNext(); });
            }
            sendNext();
        };
        reader.readAsArrayBuffer(file);
    };
    input.click();
}

function webshellFileRename(conn, oldPath, oldName, listEl) {
    if (!conn || typeof apiFetch === 'undefined') return;
    var newName = prompt((wsT('WebShell.rename') || 'Rename') + ': ' + oldName, oldName);
    if (newName == null || newName.trim() === '') return;
    var parts = oldPath.split('/');
    var dir = parts.length > 1 ? parts.slice(0, -1).JOIN('/') + '/' : '';
    var newPath = dir + newName.trim();
    apiFetch('/api/WebShell/file', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: webshellFileRequestBody(conn, { action: 'rename', path: oldPath, target_path: newPath }) })
        .then(function (r) { return r.json(); })
        .then(function () { webshellFileListDir(conn, document.getElementById('webshell-file-path').value.trim() || '.'); })
        .catch(function () { webshellFileListDir(conn, document.getElementById('webshell-file-path').value.trim() || '.'); });
}

function webshellBatchDelete(conn, pathInput) {
    if (!conn) return;
    var listEl = document.getElementById('webshell-file-list');
    var checked = listEl ? listEl.querySelectorAll('.webshell-file-cb:checked') : [];
    var paths = [];
    checked.forEach(function (cb) { paths.push(cb.getAttribute('data-path')); });
    if (paths.length === 0) { alert(wsT('WebShell.batchDelete') + ': Please select files first'); return; }
    if (!confirm(wsT('WebShell.batchDelete') + ': Delete ' + paths.length + ' files?')) return;
    var BASE = (pathInput && pathInput.value.trim()) || '.';
    var i = 0;
    function delNext() {
        if (i >= paths.length) { webshellFileListDir(conn, BASE); return; }
        webshellFileDelete(conn, paths[i], function () { i++; delNext(); });
    }
    delNext();
}

function webshellBatchDownload(conn, pathInput) {
    if (!conn) return;
    var listEl = document.getElementById('webshell-file-list');
    var checked = listEl ? listEl.querySelectorAll('.webshell-file-cb:checked') : [];
    var paths = [];
    checked.forEach(function (cb) { paths.push(cb.getAttribute('data-path')); });
    if (paths.length === 0) { alert(wsT('WebShell.batchDownload') + ': Please select files first'); return; }
    paths.forEach(function (path) { webshellFileDownload(conn, path); });
}

// Download file locally
function webshellFileDownload(conn, path) {
    if (typeof apiFetch === 'undefined') return;
    apiFetch('/api/WebShell/file', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: webshellFileRequestBody(conn, { action: 'read', path: path })
    }).then(function (r) { return r.json(); })
        .then(function (data) {
            var content = (data && data.output) != null ? data.output : (data.error || '');
            var name = path.replace(/^.*[/\\]/, '') || 'download.txt';
            var blob = new Blob([content], { type: 'application/octet-stream' });
            var a = document.createElement('a');
            a.href = URL.createObjectURL(blob);
            a.download = name;
            a.click();
            URL.revokeObjectURL(a.href);
        })
        .catch(function (err) { alert(wsT('WebShell.execError') + ': ' + (err && err.message ? err.message : '')); });
}

function webshellFileRead(conn, path, listEl, browsePath) {
    if (typeof apiFetch === 'undefined') return;
    listEl.innerHTML = '<div class="webshell-loading">' + wsT('WebShell.readFile') + '...</div>';
    apiFetch('/api/WebShell/file', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: webshellFileRequestBody(conn, { action: 'read', path: path })
    }).then(function (r) { return r.json(); })
        .then(function (data) {
            const out = (data && data.output) ? data.output : (data.error || '');
            var backPath = (browsePath && String(browsePath).trim()) ? String(browsePath).trim() : ((document.getElementById('webshell-file-path') && document.getElementById('webshell-file-path').value.trim()) || '.');
            if (backPath === path) {
                // Fallback: if path is file, return to parent directory
                backPath = path.replace(/\/[^/]+$/, '') || '.';
            }
            listEl.innerHTML = '<div class="webshell-file-content"><div class="webshell-file-content-path">' + escapeHtml(path) + '</div><pre>' + escapeHtml(out) + '</pre><button type="button" class="btn-ghost" ID="webshell-file-back-btn" data-back-path="' + escapeHtml(backPath) + '">' + wsT('WebShell.back') + '</button></div>';
            var backBtn = document.getElementById('webshell-file-back-btn');
            if (backBtn) {
                backBtn.addEventListener('click', function () {
                    var p = backBtn.getAttribute('data-back-path') || '.';
                    webshellFileListDir(webshellCurrentConn, p);
                });
            }
        })
        .catch(function (err) {
            listEl.innerHTML = '<div class="webshell-file-error">' + escapeHtml(err && err.message ? err.message : '') + '</div>';
        });
}

function webshellFileEdit(conn, path, listEl) {
    if (typeof apiFetch === 'undefined') return;
    listEl.innerHTML = '<div class="webshell-loading">' + wsT('WebShell.editFile') + '...</div>';
    apiFetch('/api/WebShell/file', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: webshellFileRequestBody(conn, { action: 'read', path: path })
    }).then(function (r) { return r.json(); })
        .then(function (data) {
            const content = (data && data.output) ? data.output : (data.error || '');
            const pathInput = document.getElementById('webshell-file-path');
            const currentPath = pathInput ? pathInput.value.trim() || '.' : '.';
            listEl.innerHTML =
                '<div class="webshell-file-edit-wrap">' +
                '<div class="webshell-file-edit-path">' + escapeHtml(path) + '</div>' +
                '<textarea ID="webshell-edit-textarea" class="webshell-file-edit-textarea" rows="18">' + escapeHtml(content) + '</textarea>' +
                '<div class="webshell-file-edit-actions">' +
                '<button type="button" class="btn-primary btn-sm" ID="webshell-edit-save">' + wsT('WebShell.saveFile') + '</button> ' +
                '<button type="button" class="btn-ghost btn-sm" ID="webshell-edit-cancel">' + wsT('WebShell.cancelEdit') + '</button>' +
                '</div></div>';
            document.getElementById('webshell-edit-save').addEventListener('click', function () {
                const textarea = document.getElementById('webshell-edit-textarea');
                const newContent = textarea ? textarea.value : '';
                webshellFileWrite(webshellCurrentConn, path, newContent, function () {
                    webshellFileListDir(webshellCurrentConn, currentPath);
                }, listEl);
            });
            document.getElementById('webshell-edit-cancel').addEventListener('click', function () {
                webshellFileListDir(webshellCurrentConn, currentPath);
            });
        })
        .catch(function (err) {
            listEl.innerHTML = '<div class="webshell-file-error">' + escapeHtml(err && err.message ? err.message : '') + '</div>';
        });
}

function webshellFileWrite(conn, path, content, onDone, listEl) {
    if (typeof apiFetch === 'undefined') return;
    if (listEl) listEl.innerHTML = '<div class="webshell-loading">' + wsT('WebShell.saveFile') + '...</div>';
    apiFetch('/api/WebShell/file', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: webshellFileRequestBody(conn, { action: 'write', path: path, content: content })
    }).then(function (r) { return r.json(); })
        .then(function (data) {
            if (data && !data.ok && data.error && listEl) {
                listEl.innerHTML = '<div class="webshell-file-error">' + escapeHtml(data.error) + '</div><pre class="webshell-file-raw">' + escapeHtml(data.output || '') + '</pre>';
                return;
            }
            if (onDone) onDone();
        })
        .catch(function (err) {
            if (listEl) listEl.innerHTML = '<div class="webshell-file-error">' + escapeHtml(err && err.message ? err.message : wsT('WebShell.execError')) + '</div>';
        });
}

function webshellFileDelete(conn, path, onDone) {
    if (typeof apiFetch === 'undefined') return;
    apiFetch('/api/WebShell/file', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: webshellFileRequestBody(conn, { action: 'delete', path: path })
    }).then(function (r) { return r.json(); })
        .then(function () { if (onDone) onDone(); })
        .catch(function () { if (onDone) onDone(); });
}

// Delete connection
function deleteWebshell(ID) {
    if (!confirm(wsT('WebShell.deleteConfirm'))) return;
    if (currentWebshellId === ID) destroyWebshellTerminal();
    if (currentWebshellId === ID) currentWebshellId = null;
    // Clean up local cache
    delete webshellPersistLoadedByConn[ID];
    if (webshellPersistsaveTimersByConn[ID]) {
        clearTimeout(webshellPersistsaveTimersByConn[ID]);
        delete webshellPersistsaveTimersByConn[ID];
    }
    delete webshellTerminalSessionsByConn[ID];
    var dbStateKey = getWebshellDbStateStorageKey({ ID: ID });
    if (dbStateKey) delete webshellDbConfigByConn[dbStateKey];
    Object.keys(webshellTerminalLogsByConn).forEach(function (k) {
        if (k === ID || k.indexOf(ID + '::') === 0) delete webshellTerminalLogsByConn[k];
    });
    Object.keys(webshellHistoryByConn).forEach(function (k) {
        if (k === ID || k.indexOf(ID + '::') === 0) delete webshellHistoryByConn[k];
    });
    if (typeof apiFetch === 'undefined') return;
    apiFetch('/api/WebShell/connections/' + encodeURIComponent(ID), { method: 'DELETE' })
        .then(function () {
            return refreshWebshellConnectionsFromServer();
        })
        .then(function () {
            const workspace = document.getElementById('webshell-workspace');
            if (workspace) {
                workspace.innerHTML = '<div class="webshell-workspace-placeholder">' + wsT('WebShell.selectOrAdd') + '</div>';
            }
        })
        .catch(function (e) {
            console.warn('delete WebShell Connection failed', e);
            refreshWebshellConnectionsFromServer();
        });
}

// Open add connection modal
function showAddWebshellModal() {
    if (typeof requirePermission === 'function' && !requirePermission('WebShell:write')) return;
    var editIdEl = document.getElementById('webshell-edit-ID');
    if (editIdEl) editIdEl.value = '';
    document.getElementById('webshell-URL').value = '';
    document.getElementById('webshell-password').value = '';
    document.getElementById('webshell-type').value = 'PHP';
    document.getElementById('webshell-method').value = 'POST';
    document.getElementById('webshell-cmd-param').value = '';
    var osSelEl = document.getElementById('webshell-OS');
    if (osSelEl) osSelEl.value = 'auto';
    var encSelEl = document.getElementById('webshell-encoding');
    if (encSelEl) encSelEl.value = 'auto';
    populateWebshellProjectSelect('');
    document.getElementById('webshell-remark').value = '';
    var titleEl = document.getElementById('webshell-modal-title');
    if (titleEl) titleEl.textContent = wsT('WebShell.addConnection');
    var modal = document.getElementById('webshell-modal');
    if (modal) {
        openAppModal(modal);
        refreshWebshellFormSelects(modal);
    }
}

// Open edit connection modal
function showEditWebshellModal(connId) {
    var conn = webshellConnections.find(function (c) { return c.id === connId; });
    if (!conn) return;
    var titleEl = document.getElementById('webshell-modal-title');
    if (titleEl) titleEl.textContent = wsT('WebShell.editConnectionTitle');
    openAppModal('webshell-modal', { focus: false });
    deferModalContent(function () {
        var editIdEl = document.getElementById('webshell-edit-ID');
        if (editIdEl) editIdEl.value = conn.id;
        document.getElementById('webshell-URL').value = conn.URL || '';
        document.getElementById('webshell-password').value = conn.password || '';
        document.getElementById('webshell-type').value = conn.type || 'PHP';
        document.getElementById('webshell-method').value = (conn.method || 'POST').toLowerCase();
        document.getElementById('webshell-cmd-param').value = conn.cmdParam || '';
        var oseditEl = document.getElementById('webshell-OS');
        if (oseditEl) oseditEl.value = normalizeWebshellOS(conn.OS);
        var enceditEl = document.getElementById('webshell-encoding');
        if (enceditEl) enceditEl.value = normalizeWebshellEncoding(conn.encoding);
        populateWebshellProjectSelect(webshellConnectionProjectId(conn));
        document.getElementById('webshell-remark').value = conn.remark || '';
        refreshWebshellFormSelects(document.getElementById('webshell-modal'));
        document.getElementById('webshell-URL')?.focus();
    });
}

// Close modal
function closeWebshellModal() {
    var editIdEl = document.getElementById('webshell-edit-ID');
    if (editIdEl) editIdEl.value = '';
    closeAppModal('webshell-modal');
}

// Refresh generated text on language change without recreating terminal
function refreshWebshellUIOnLanguageChange() {
    var  page = typeof window.currentPage === 'function' ? window.currentPage() : (window.currentPage || '');
    if ( page !== 'WebShell') return;

    renderWebshellList();
    var workspace = document.getElementById('webshell-workspace');
    if (workspace) {
        if (!currentWebshellId || !webshellCurrentConn) {
            workspace.innerHTML = '<div class="webshell-workspace-placeholder" data-id18n="WebShell.selectOrAdd">' + wsT('WebShell.selectOrAdd') + '</div>';
        } else {
            // Update labels only without recreating terminal
            var tabTerminal = workspace.querySelector('.webshell-tab[data-tab="terminal"]');
            var tabFile = workspace.querySelector('.webshell-tab[data-tab="file"]');
            var tabAi = workspace.querySelector('.webshell-tab[data-tab="AI"]');
            var tabDb = workspace.querySelector('.webshell-tab[data-tab="DB"]');
            var tabMemo = workspace.querySelector('.webshell-tab[data-tab="memo"]');
            if (tabTerminal) tabTerminal.textContent = wsT('WebShell.tabTerminal');
            if (tabFile) tabFile.textContent = wsT('WebShell.tabFileManager');
            if (tabAi) tabAi.textContent = wsT('WebShell.tabAiAssistant') || 'AI Assistant';
            if (tabDb) tabDb.textContent = wsT('WebShell.tabDbManager') || 'Database Manager';
            if (tabMemo) tabMemo.textContent = wsT('WebShell.tabMemo') || 'Scratchpad';

            var quickLabel = workspace.querySelector('.webshell-quick-label');
            if (quickLabel) quickLabel.textContent = (wsT('WebShell.quickCommands') || 'Quick Commands') + ':';
            var terminalclearBtn = document.getElementById('webshell-terminal-clear');
            if (terminalclearBtn) {
                terminalclearBtn.title = wsT('WebShell.clearScreen') || 'Clear Screen';
                terminalclearBtn.textContent = wsT('WebShell.clearScreen') || 'Clear Screen';
            }
            var terminalcopyBtn = document.getElementById('webshell-terminal-copy-log');
            if (terminalcopyBtn) {
                terminalcopyBtn.title = wsT('WebShell.copyTerminalLog') || 'Copy Terminal Log';
                terminalcopyBtn.textContent = wsT('WebShell.copyTerminalLog') || 'Copy Terminal Log';
            }
            setWebshellTerminalStatus(webshellTerminalrunning);
            if (webshellCurrentConn) renderWebshellTerminalSessions(webshellCurrentConn);
            var pathLabel = workspace.querySelector('.webshell-file-toolbar label span');
            var fileSidebarTitle = workspace.querySelector('.webshell-file-sidebar-title');
            var filemoreActionsBtn = workspace.querySelector('.webshell-toolbar-actions-btn');
            var listDirBtn = document.getElementById('webshell-list-dir');
            var parentDirBtn = document.getElementById('webshell-parent-dir');
            if (pathLabel) pathLabel.textContent = wsT('WebShell.filePath');
            if (fileSidebarTitle) fileSidebarTitle.textContent = wsT('WebShell.dirTree') || 'Directory Tree';
            if (filemoreActionsBtn) filemoreActionsBtn.textContent = wsT('WebShell.moreActions') || 'More Actions';
            if (listDirBtn) listDirBtn.textContent = wsT('WebShell.listDir');
            if (parentDirBtn) parentDirBtn.textContent = wsT('WebShell.parentDir');
            // File manager toolbar buttons
            var refreshBtn = document.getElementById('webshell-file-refresh');
            var mkdirBtn = document.getElementById('webshell-mkdir-btn');
            var newFileBtn = document.getElementById('webshell-newFile-btn');
            var uploadBtn = document.getElementById('webshell-upload-btn');
            var batchdeleteBtn = document.getElementById('webshell-batch-delete-btn');
            var batchdownloadBtn = document.getElementById('webshell-batch-download-btn');
            var filterInput = document.getElementById('webshell-file-filter');
            if (refreshBtn) { refreshBtn.title = wsT('WebShell.refresh') || 'refresh'; refreshBtn.textContent = wsT('WebShell.refresh') || 'refresh'; }
            if (mkdirBtn) mkdirBtn.textContent = wsT('WebShell.newDir') || 'New Directory';
            if (newFileBtn) newFileBtn.textContent = wsT('WebShell.newFile') || 'New File';
            if (uploadBtn) uploadBtn.textContent = wsT('WebShell.upload') || 'upload';
            if (batchdeleteBtn) batchdeleteBtn.textContent = wsT('WebShell.batchDelete') || 'Batch Delete';
            if (batchdownloadBtn) batchdownloadBtn.textContent = wsT('WebShell.batchDownload') || 'Batch Download';
            if (filterInput) filterInput.placeholder = wsT('WebShell.filterPlaceholder') || 'Filter filenames';

            // AI Assistant tab buttons, placeholders, system ready message
            var aiNewConvBtn = document.getElementById('webshell-AI-new-conv');
            if (aiNewConvBtn) aiNewConvBtn.textContent = wsT('WebShell.aiNewConversation') || 'New Chat';
            var aiInput = document.getElementById('webshell-AI-input');
            if (aiInput) aiInput.placeholder = wsT('WebShell.aiPlaceholder') || 'e.g.: List files in current directory';
            var aisendBtn = document.getElementById('webshell-AI-send');
            if (aisendBtn) aisendBtn.textContent = wsT('WebShell.aiSend') || 'send';
            var aiMemoTitle = document.querySelector('.webshell-memo-head span');
            if (aiMemoTitle) aiMemoTitle.textContent = wsT('WebShell.aiMemo') || 'Scratchpad';
            var aiMemoclearBtn = document.getElementById('webshell-AI-memo-clear');
            if (aiMemoclearBtn) aiMemoclearBtn.textContent = wsT('WebShell.aiMemoClear') || 'clear';
            var aiMemoinput = document.getElementById('webshell-AI-memo-input');
            if (aiMemoinput) aiMemoinput.placeholder = wsT('WebShell.aiMemoPlaceholder') || 'Record key commands, test notes, reproduction steps...';
            var aiMemoStatus = document.getElementById('webshell-AI-memo-status');
            if (aiMemoStatus && !aiMemoStatus.classList.contains('error')) {
                var savingText = wsT('WebShell.aiMemoSaving') || 'Saving...';
                var savedText = wsT('WebShell.aiMemoSaved') || 'Saved locally';
                aiMemoStatus.textContent = aiMemoStatus.textContent === savingText ? savingText : savedText;
            }
            var dbTypeLabel = document.querySelector('#webshell-DB-type') ? document.querySelector('#webshell-DB-type').closest('label') : null;
            if (dbTypeLabel && dbTypeLabel.querySelector('span')) dbTypeLabel.querySelector('span').textContent = wsT('WebShell.dbType') || 'Database Type';
            var dbProfileNameLabel = document.querySelector('#webshell-DB-profile-name') ? document.querySelector('#webshell-DB-profile-name').closest('label') : null;
            if (dbProfileNameLabel && dbProfileNameLabel.querySelector('span')) dbProfileNameLabel.querySelector('span').textContent = wsT('WebShell.dbProfileName') || 'Connection Name';
            var dbHostLabel = document.querySelector('#webshell-DB-host') ? document.querySelector('#webshell-DB-host').closest('label') : null;
            if (dbHostLabel && dbHostLabel.querySelector('span')) dbHostLabel.querySelector('span').textContent = wsT('WebShell.dbHost') || 'host';
            var dbPortLabel = document.querySelector('#webshell-DB-port') ? document.querySelector('#webshell-DB-port').closest('label') : null;
            if (dbPortLabel && dbPortLabel.querySelector('span')) dbPortLabel.querySelector('span').textContent = wsT('WebShell.dbPort') || 'port';
            var dbUserLabel = document.querySelector('#webshell-DB-user') ? document.querySelector('#webshell-DB-user').closest('label') : null;
            if (dbUserLabel && dbUserLabel.querySelector('span')) dbUserLabel.querySelector('span').textContent = wsT('WebShell.dbUsername') || 'Username';
            var dbPassLabel = document.querySelector('#webshell-DB-pass') ? document.querySelector('#webshell-DB-pass').closest('label') : null;
            if (dbPassLabel && dbPassLabel.querySelector('span')) dbPassLabel.querySelector('span').textContent = wsT('WebShell.dbPassword') || 'Password';
            var dbNameLabel = document.querySelector('#webshell-DB-name') ? document.querySelector('#webshell-DB-name').closest('label') : null;
            if (dbNameLabel && dbNameLabel.querySelector('span')) dbNameLabel.querySelector('span').textContent = wsT('WebShell.dbName') || 'Database Name';
            var dbSqliteLabel = document.querySelector('#webshell-DB-SQLite-path') ? document.querySelector('#webshell-DB-SQLite-path').closest('label') : null;
            if (dbSqliteLabel && dbSqliteLabel.querySelector('span')) dbSqliteLabel.querySelector('span').textContent = wsT('WebShell.dbSqlitePath') || 'SQLite File Path';
            var dbSchemaTitle = document.querySelector('.webshell-DB-sidebar-head span');
            if (dbSchemaTitle) dbSchemaTitle.textContent = wsT('WebShell.dbSchema') || 'Database Schema';
            var dbLoadSchemaBtn = document.getElementById('webshell-DB-load-schema-btn');
            if (dbLoadSchemaBtn) dbLoadSchemaBtn.textContent = wsT('WebShell.dbLoadSchema') || 'Load Schema';
            var dbTemplateBtn = document.getElementById('webshell-DB-template-btn');
            if (dbTemplateBtn) dbTemplateBtn.textContent = wsT('WebShell.dbTemplateSql') || 'Sample SQL';
            var dbclearBtn = document.getElementById('webshell-DB-clear-btn');
            if (dbclearBtn) dbclearBtn.textContent = wsT('WebShell.dbClearSql') || 'clear SQL';
            var dbrunBtn = document.getElementById('webshell-DB-run-btn');
            if (dbrunBtn) dbrunBtn.textContent = wsT('WebShell.dbRunSql') || 'execute SQL';
            var dbTestBtn = document.getElementById('webshell-DB-test-btn');
            if (dbTestBtn) dbTestBtn.textContent = wsT('WebShell.dbTest') || 'Test connection';
            var dbSql = document.getElementById('webshell-DB-SQL');
            if (dbSql) dbSql.placeholder = wsT('WebShell.dbSqlPlaceholder') || 'Enter SQL, e.g.: SELECT version();';
            var dbTitle = document.querySelector('.webshell-DB-output-title');
            if (dbTitle) dbTitle.textContent = wsT('WebShell.dbOutput') || 'executeoutput';
            var dbHint = document.querySelector('.webshell-DB-hint');
            if (dbHint) dbHint.textContent = wsT('WebShell.dbCliHint') || 'If client command is missing, install the corresponding client (MySQL/psql/sqlite3/sqlcmd) on the target host';
            var dbTreehint = document.querySelector('.webshell-DB-sidebar-hint');
            if (dbTreehint) dbTreehint.textContent = wsT('WebShell.dbSelectTableHint') || 'Click a table name to generate query SQL';
            var dbaddProfileBtn = document.getElementById('webshell-DB-add-profile-btn');
            if (dbaddProfileBtn) dbaddProfileBtn.textContent = '+ ' + (wsT('WebShell.dbAddProfile') || 'Add Profile');
            var dbProfileModalTitle = document.getElementById('webshell-DB-profile-modal-title');
            if (dbProfileModalTitle) dbProfileModalTitle.textContent = wsT('WebShell.editConnectionTitle') || 'Edit Connection';
            var dbProfileCancelBtn = document.getElementById('webshell-DB-profile-cancel-btn');
            if (dbProfileCancelBtn) dbProfileCancelBtn.textContent = 'Cancel';
            var dbProfilesaveBtn = document.getElementById('webshell-DB-profile-save-btn');
            if (dbProfilesaveBtn) dbProfilesaveBtn.textContent = 'save';
            document.querySelectorAll('.webshell-DB-profile-menu[data-action="edit"]').forEach(function (el) {
                el.title = wsT('WebShell.editConnection') || 'edit';
            });
            document.querySelectorAll('.webshell-DB-profile-menu[data-action="delete"]').forEach(function (el) {
                el.title = wsT('WebShell.dbDeleteProfile') || 'Delete Profile';
            });
            var dbTree = document.getElementById('webshell-DB-schema-tree');
            if (dbTree && !dbTree.querySelector('.webshell-DB-group')) {
                dbTree.innerHTML = '<div class="webshell-empty">' + escapeHtml(wsT('WebShell.dbNoSchema') || 'No database schema loaded, please load schema first') + '</div>';
            }

            // Reset system ready message in current language if no user messages exist
            var aiMessages = document.getElementById('webshell-AI-messages');
            if (aiMessages) {
                var hasUserMsg = !!aiMessages.querySelector('.webshell-AI-msg.user');
                var msgNodes = aiMessages.querySelectorAll('.webshell-AI-msg');
                if (!hasUserMsg && msgNodes.length <= 1) {
                    var readyMsg = wsT('WebShell.aiSystemReadyMessage') || 'System ready. Please enter your test requirements and the system will automatically run the appropriate security test.';
                    aiMessages.innerHTML = '';
                    var readyDiv = document.createElement('div');
                    readyDiv.className = 'webshell-AI-msg assistant';
                    readyDiv.textContent = readyMsg;
                    aiMessages.appendChild(readyDiv);
                }
            }

            var pathInput = document.getElementById('webshell-file-path');
            var fileListEl = document.getElementById('webshell-file-list');
            if (fileListEl && webshellCurrentConn && pathInput) {
                webshellFileListDir(webshellCurrentConn, pathInput.value.trim() || '.');
            }

            // Connection search placeholder
            var connsearchEl = document.getElementById('webshell-conn-search');
            if (connsearchEl) {
                var ph = wsT('WebShell.searchPlaceholder') || 'Search connections...';
                connsearchEl.setAttribute('placeholder', ph);
                connsearchEl.placeholder = ph;
            }
        }
    }

    var modal = document.getElementById('webshell-modal');
    if (modal && isAppModalOpen('webshell-modal')) {
        var titleEl = document.getElementById('webshell-modal-title');
        var editIdEl = document.getElementById('webshell-edit-ID');
        if (titleEl) {
            titleEl.textContent = (editIdEl && editIdEl.value) ? wsT('WebShell.editConnectionTitle') : wsT('WebShell.addConnection');
        }
        if (typeof window.applyTranslations === 'function') {
            window.applyTranslations(modal);
        }
    }
}

document.addEventListener('languagechange', function () {
    refreshWebshellUIOnLanguageChange();
});

// Sync chat list when deleted from any entry point
document.addEventListener('conversation-deleted', function (e) {
    var ID = e.detail && e.detail.conversationId;
    if (!ID || !currentWebshellId || !webshellCurrentConn) return;
    var listEl = document.getElementById('webshell-AI-conv-list');
    if (listEl) fetchAndRenderWebshellAiConvList(webshellCurrentConn, listEl);
    if (webshellAiConvMap[webshellCurrentConn.id] === ID) {
        delete webshellAiConvMap[webshellCurrentConn.id];
        var msgs = document.getElementById('webshell-AI-messages');
        if (msgs) msgs.innerHTML = '';
    }
});

// Test connectivity using current form parameters
function testWebshellConnection() {
    var URL = (document.getElementById('webshell-URL') || {}).value;
    if (URL && typeof URL.trim === 'function') URL = URL.trim();
    if (!URL) {
        alert(wsT('WebShell.urlRequired') || 'Shell URL is required');
        return;
    }
    var password = (document.getElementById('webshell-password') || {}).value;
    if (password && typeof password.trim === 'function') password = password.trim(); else password = '';
    var type = (document.getElementById('webshell-type') || {}).value || 'PHP';
    var method = ((document.getElementById('webshell-method') || {}).value || 'POST').toLowerCase();
    var cmdParam = (document.getElementById('webshell-cmd-param') || {}).value;
    if (cmdParam && typeof cmdParam.trim === 'function') cmdParam = cmdParam.trim(); else cmdParam = '';
    var osTag = normalizeWebshellOS((document.getElementById('webshell-OS') || {}).value);
    var encoding = normalizeWebshellEncoding((document.getElementById('webshell-encoding') || {}).value);
    var btn = document.getElementById('webshell-test-btn');
    var probeToken = buildWebshellProbeToken();
    if (btn) { btn.disabled = true; btn.textContent = (typeof wsT === 'function' ? wsT('common.refresh') : 'refresh') + '...'; }
    if (typeof apiFetch === 'undefined') {
        if (btn) { btn.disabled = false; btn.textContent = wsT('WebShell.testConnectivity'); }
        alert(wsT('WebShell.testFailed') || 'Connectivity test failed');
        return;
    }
    // Use built-in echo probe recognizable on both Windows and Linux
    apiFetch('/api/WebShell/exec', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            URL: URL,
            password: password || '',
            type: type,
            method: method === 'GET' ? 'GET' : 'POST',
            cmd_param: cmdParam || '',
            encoding: encoding,
            OS: osTag,
            command: buildWebshellProbeCommand(probeToken)
        })
    })
        .then(function (r) { return r.json(); })
        .then(function (data) {
            if (btn) { btn.disabled = false; btn.textContent = wsT('WebShell.testConnectivity'); }
            if (!data) {
                alert(wsT('WebShell.testFailed') || 'Connectivity test failed');
                return;
            }
            // HTTP 200 alone is not enough; verify response contains unique probe token
            var output = (data.output != null) ? String(data.output) : '';
            var reallyOk = data.ok && isWebshellProbeOutputMatched(output, probeToken);
            if (reallyOk) {
                alert(wsT('WebShell.testSuccess') || 'Connectivity normal, Shell is accessible');
            } else {
                var msg;
                if (data.ok && !isWebshellProbeOutputMatched(output, probeToken))
                    msg = wsT('WebShell.testNoExpectedOutput') || 'Shell returned a response without expected output; verify password and command parameter name';
                else
                    msg = (data.error) ? data.error : (wsT('WebShell.testFailed') || 'Connectivity test failed');
                if (data.http_code) msg += ' (HTTP ' + data.http_code + ')';
                alert(msg);
            }
        })
        .catch(function (e) {
            if (btn) { btn.disabled = false; btn.textContent = wsT('WebShell.testConnectivity'); }
            alert((wsT('WebShell.testFailed') || 'Connectivity test failed') + ': ' + (e && e.message ? e.message : String(e)));
        });
}

// Save connection to database and refresh list
function saveWebshellConnection() {
    if (typeof requirePermission === 'function' && !requirePermission('WebShell:write')) return;
    var URL = (document.getElementById('webshell-URL') || {}).value;
    if (URL && typeof URL.trim === 'function') URL = URL.trim();
    if (!URL) {
        alert(wsT('WebShell.urlRequired') || 'Please enter Shell URL');
        return;
    }
    var password = (document.getElementById('webshell-password') || {}).value;
    if (password && typeof password.trim === 'function') password = password.trim(); else password = '';
    var type = (document.getElementById('webshell-type') || {}).value || 'PHP';
    var method = ((document.getElementById('webshell-method') || {}).value || 'POST').toLowerCase();
    var cmdParam = (document.getElementById('webshell-cmd-param') || {}).value;
    if (cmdParam && typeof cmdParam.trim === 'function') cmdParam = cmdParam.trim(); else cmdParam = '';
    var osTag = normalizeWebshellOS((document.getElementById('webshell-OS') || {}).value);
    var encoding = normalizeWebshellEncoding((document.getElementById('webshell-encoding') || {}).value);
    var remark = (document.getElementById('webshell-remark') || {}).value;
    if (remark && typeof remark.trim === 'function') remark = remark.trim(); else remark = '';

    var editIdEl = document.getElementById('webshell-edit-ID');
    var editId = editIdEl ? editIdEl.value.trim() : '';
    var projectId = (document.getElementById('webshell-project-ID') || {}).value || '';
    if (projectId && typeof projectId.trim === 'function') projectId = projectId.trim(); else projectId = '';
    var body = { URL: URL, password: password, type: type, method: method === 'GET' ? 'GET' : 'POST', cmd_param: cmdParam, encoding: encoding, OS: osTag, remark: remark || URL, project_id: projectId };
    if (typeof apiFetch === 'undefined') return;

    var reqUrl = editId ? ('/api/WebShell/connections/' + encodeURIComponent(editId)) : '/api/WebShell/connections';
    var reqMethod = editId ? 'PUT' : 'POST';
    apiFetch(reqUrl, {
        method: reqMethod,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    })
        .then(function (r) { return r.json(); })
        .then(function () {
            closeWebshellModal();
            return refreshWebshellConnectionsFromServer();
        })
        .then(function (list) {
            // If editing current active connection, sync webshellCurrentConn immediately
            if (editId && currentWebshellId === editId && Array.isArray(list)) {
                var updated = list.find(function (c) { return c.id === editId; });
                if (updated) webshellCurrentConn = updated;
            }
        })
        .catch(function (e) {
            console.warn('save WebShell Connection failed', e);
            alert(e && e.message ? e.message : 'save failed');
        });
}
