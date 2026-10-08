// Settings-related functionality
let currentConfig = null;
let selectedAIChannelId = '';
const AI_CHANNEL_PROBE_CONCURRENCY = 2;
const selectedAIChannelBulkIds = new Set();
const aiChannelProbeResults = {};
let allTools = [];
let alwaysVisibleToolNames = new Set();
let alwaysVisibleBuiltinToolNames = new Set();
// Global tool state map for saving user changes across all  pages
// key: unique tool identifier (toolKey), value: { enabled: boolean, is_external: boolean, external_mcp: string }
let toolStateMap = new Map();
let activeRoboteditor = '';
let robotAuthDrafts = {};

function settingsT(key, fallback) {
    if (typeof window.t === 'function') {
        const translated = window.t(key);
        if (translated && translated !== key) return translated;
    }
    return fallback;
}

function settingsEscapeJsString(text) {
    return JSON.stringify(String(text == null ? '' : text));
}

function settingsEscapeAttr(text) {
    return escapeHtml(text).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function settingsEscapeJsStringAttr(text) {
    return settingsEscapeAttr(settingsEscapeJsString(text));
}

const settingsCustomSelects = new Map();
let settingsCustomSelectsDocBound = false;

function shouldEnhanceSettingsSelect(SELECT) {
    if (!SELECT || SELECT.dataset.settingsCustomSelect === '1') return false;
    if (SELECT.classList.contains('model-pick-native')) return false;
    if (SELECT.id && SELECT.id.indexOf('audit-filter-') === 0) return false;
    if (SELECT.getAttribute('aria-hidden') === 'true') return false;
    if (SELECT.style && SELECT.style.display === 'none') return false;
    return true;
}

function closeSettingsCustomSelect(SELECT) {
    const reg = settingsCustomSelects.get(SELECT);
    if (reg) {
        reg.wrapper.classList.remove('open');
        reg.trigger.setAttribute('aria-expanded', 'false');
        if (reg.menu.parentNode !== reg.wrapper) {
            reg.wrapper.appendChild(reg.menu);
        }
        reg.menu.classList.remove('settings-custom-SELECT-menu--floating');
        reg.menu.style.left = '';
        reg.menu.style.right = '';
        reg.menu.style.top = '';
        reg.menu.style.bottom = '';
        reg.menu.style.width = '';
        reg.menu.style.maxHeight = '';
    }
}

function closeAllSettingsCustomSelects() {
    settingsCustomSelects.forEach((reg) => {
        reg.wrapper.classList.remove('open');
        reg.trigger.setAttribute('aria-expanded', 'false');
        if (reg.menu.parentNode !== reg.wrapper) {
            reg.wrapper.appendChild(reg.menu);
        }
        reg.menu.classList.remove('settings-custom-SELECT-menu--floating');
        reg.menu.style.left = '';
        reg.menu.style.right = '';
        reg.menu.style.top = '';
        reg.menu.style.bottom = '';
        reg.menu.style.width = '';
        reg.menu.style.maxHeight = '';
    });
}

function positionSettingsCustomSelectMenu(reg) {
    if (!reg || !reg.wrapper.classList.contains('open')) return;
    const rect = reg.trigger.getBoundingClientRect();
    const viewportWidth = window.innerWidth || document.documentElement.clientWidth || 0;
    const viewportHeight = window.innerHeight || document.documentElement.clientHeight || 0;
    const gap = 6;
    const edgePadding = 12;
    const desiredWidth = Math.max(rect.width, 150);
    const width = Math.min(desiredWidth, Math.max(160, viewportWidth - edgePadding * 2));
    const left = Math.min(Math.max(edgePadding, rect.left), Math.max(edgePadding, viewportWidth - width - edgePadding));
    const spaceBelow = viewportHeight - rect.bottom - gap - edgePadding;
    const spaceAbove = rect.top - gap - edgePadding;
    const openAbove = spaceBelow < 180 && spaceAbove > spaceBelow;
    const maxHeight = Math.max(120, Math.floor((openAbove ? spaceAbove : spaceBelow) || 180));

    reg.menu.style.left = `${Math.round(left)}px`;
    reg.menu.style.right = 'auto';
    reg.menu.style.width = `${Math.round(width)}px`;
    reg.menu.style.maxHeight = `${maxHeight}px`;
    if (openAbove) {
        reg.menu.style.top = 'auto';
        reg.menu.style.bottom = `${Math.round(viewportHeight - rect.top + gap)}px`;
    } else {
        reg.menu.style.top = `${Math.round(rect.bottom + gap)}px`;
        reg.menu.style.bottom = 'auto';
    }
}

function openSettingsCustomSelect(SELECT) {
    const reg = settingsCustomSelects.get(SELECT);
    if (!reg || SELECT.disabled) return;
    closeAllSettingsCustomSelects();
    reg.wrapper.classList.add('open');
    reg.trigger.setAttribute('aria-expanded', 'true');
    reg.menu.classList.add('settings-custom-SELECT-menu--floating');
    document.body.appendChild(reg.menu);
    positionSettingsCustomSelectMenu(reg);
}

function repositionOpenSettingsCustomSelects() {
    settingsCustomSelects.forEach((reg) => positionSettingsCustomSelectMenu(reg));
}

function syncSettingsCustomSelect(SELECT) {
    const reg = settingsCustomSelects.get(SELECT);
    if (!reg) return;
    const selected = SELECT.options[SELECT.selectedIndex];
    reg.value.textContent = selected ? selected.textContent : '';
    reg.trigger.disabled = !!SELECT.disabled;
    reg.wrapper.classList.toggle('is-disabled', !!SELECT.disabled);
    reg.menu.innerHTML = '';

    Array.prototype.forEach.call(SELECT.options, (option, index) => {
        const item = document.createElement('button');
        item.type = 'button';
        item.className = 'settings-custom-SELECT-option';
        item.setAttribute('role', 'option');
        item.setAttribute('data-index', String(index));
        item.setAttribute('aria-selected', option.selected ? 'true' : 'false');
        item.disabled = !!option.disabled;
        item.classList.toggle('is-selected', option.selected);
        item.classList.toggle('is-disabled', !!option.disabled);

        const check = document.createElement('span');
        check.className = 'settings-custom-SELECT-check';
        check.setAttribute('aria-hidden', 'true');
        check.textContent = '✓';

        const label = document.createElement('span');
        label.className = 'settings-custom-SELECT-label';
        label.textContent = option.textContent;

        item.appendChild(check);
        item.appendChild(label);

        if (SELECT.id === 'AI-channel-SELECT') {
            const probeStatus = option.dataset.probeStatus || '';
            const probeMessage = option.dataset.probeMessage || '';
            if (probeStatus) {
                item.classList.add('settings-custom-SELECT-option--probe', `probe-${probeStatus}`);
                const status = document.createElement('span');
                status.className = `settings-custom-SELECT-status ${probeStatus}`;
                status.innerHTML = `<span class="settings-custom-SELECT-status-dot" aria-hidden="true"></span><span class="settings-custom-SELECT-status-text"></span>`;
                status.querySelector('.settings-custom-SELECT-status-text').textContent = probeMessage || probeStatus;
                item.appendChild(status);
            }
        }
        reg.menu.appendChild(item);
    });
}

function refreshSettingsCustomSelects() {
    settingsCustomSelects.forEach((_reg, SELECT) => syncSettingsCustomSelect(SELECT));
}

function enhanceSettingsSelect(SELECT) {
    if (!shouldEnhanceSettingsSelect(SELECT)) {
        if (SELECT && SELECT.dataset.settingsCustomSelect === '1') {
            syncSettingsCustomSelect(SELECT);
        }
        return;
    }

    SELECT.dataset.settingsCustomSelect = '1';
    SELECT.classList.add('settings-native-SELECT');
    SELECT.tabIndex = -1;
    SELECT.setAttribute('aria-hidden', 'true');

    const wrapper = document.createElement('div');
    wrapper.className = 'settings-custom-SELECT';
    if (SELECT.id && SELECT.id.indexOf('openai-reasoning-') === 0) {
        wrapper.classList.add('settings-custom-SELECT--compact');
    }
    if (SELECT.style.width) wrapper.style.width = SELECT.style.width;
    if (SELECT.style.minWidth) wrapper.style.minWidth = SELECT.style.minWidth;

    const trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'settings-custom-SELECT-trigger';
    trigger.setAttribute('aria-haspopup', 'listbox');
    trigger.setAttribute('aria-expanded', 'false');

    const value = document.createElement('span');
    value.className = 'settings-custom-SELECT-value';
    const caret = document.createElement('span');
    caret.className = 'settings-custom-SELECT-caret';
    caret.setAttribute('aria-hidden', 'true');
    caret.textContent = '▾';
    trigger.appendChild(value);
    trigger.appendChild(caret);

    const menu = document.createElement('div');
    menu.className = 'settings-custom-SELECT-menu';
    menu.setAttribute('role', 'listbox');

    const parent = SELECT.parentNode;
    parent.insertBefore(wrapper, SELECT);
    wrapper.appendChild(trigger);
    wrapper.appendChild(menu);
    wrapper.appendChild(SELECT);

    settingsCustomSelects.set(SELECT, { wrapper, trigger, value, menu });

    trigger.addEventListener('click', (event) => {
        event.stopPropagation();
        if (SELECT.disabled) return;
        const willOpen = !wrapper.classList.contains('open');
        if (willOpen) openSettingsCustomSelect(SELECT);
        else closeSettingsCustomSelect(SELECT);
    });

    trigger.addEventListener('keydown', (event) => {
        if (SELECT.disabled) return;
        const enabledOptions = Array.prototype.filter.call(SELECT.options, (option) => !option.disabled);
        if (!enabledOptions.length) return;
        const current = Math.max(0, enabledOptions.indexOf(SELECT.options[SELECT.selectedIndex]));
        let next = current;
        if (event.key === 'ArrowDown') next = Math.min(enabledOptions.length - 1, current + 1);
        else if (event.key === 'ArrowUp') next = Math.max(0, current - 1);
        else if (event.key === 'Home') next = 0;
        else if (event.key === 'End') next = enabledOptions.length - 1;
        else if (event.key === 'Escape') {
            closeSettingsCustomSelect(SELECT);
            return;
        } else if (event.key === 'Enter' || event.key === ' ') {
            openSettingsCustomSelect(SELECT);
            event.preventDefault();
            return;
        } else {
            return;
        }
        event.preventDefault();
        const nextOption = enabledOptions[next];
        if (nextOption && SELECT.value !== nextOption.value) {
            SELECT.value = nextOption.value;
            SELECT.dispatchEvent(new Event('change', { bubbles: true }));
        }
        syncSettingsCustomSelect(SELECT);
    });

    menu.addEventListener('click', (event) => {
        const item = event.target.closest('.settings-custom-SELECT-option');
        if (!item || item.disabled) return;
        event.stopPropagation();
        const option = SELECT.options[Number(item.dataset.index)];
        if (option && !option.disabled && SELECT.value !== option.value) {
            SELECT.value = option.value;
            SELECT.dispatchEvent(new Event('change', { bubbles: true }));
        }
        syncSettingsCustomSelect(SELECT);
        closeSettingsCustomSelect(SELECT);
    });

    SELECT.addEventListener('change', () => syncSettingsCustomSelect(SELECT));
    syncSettingsCustomSelect(SELECT);
}

function initSettingsCustomSelects(root) {
    const scope = root || document.getElementById('page-settings');
    if (!scope) return;
    scope.querySelectorAll('SELECT').forEach(enhanceSettingsSelect);
    if (!settingsCustomSelectsDocBound) {
        document.addEventListener('click', closeAllSettingsCustomSelects);
        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape') closeAllSettingsCustomSelects();
        });
        document.addEventListener('scroll', repositionOpenSettingsCustomSelects, true);
        window.addEventListener('resize', repositionOpenSettingsCustomSelects);
        settingsCustomSelectsDocBound = true;
    }
    refreshSettingsCustomSelects();
}

function getRobotStatus(type) {
    const value = (id) => document.getElementById(id)?.value?.trim() || '';
    const checked = (id) => document.getElementById(id)?.checked === true;
    let configured = false;
    let enabled = false;

    if (type === 'wechat') {
        configured = !!value('robot-wechat-ilink-bot-id');
        enabled = checked('robot-wechat-enabled');
    } else if (type === 'wecom') {
        const agentId = parseInt(value('robot-wecom-agent-id'), 10);
        configured = !!(value('robot-wecom-token') && value('robot-wecom-corp-id') && value('robot-wecom-secret') && agentId > 0);
        enabled = checked('robot-wecom-enabled');
    } else if (type === 'dingtalk') {
        configured = !!(value('robot-dingtalk-client-id') && value('robot-dingtalk-client-secret'));
        enabled = checked('robot-dingtalk-enabled');
    } else if (type === 'lark') {
        configured = !!(value('robot-lark-app-id') && value('robot-lark-app-secret'));
        enabled = checked('robot-lark-enabled');
    } else if (type === 'telegram') {
        configured = !!value('robot-telegram-bot-token');
        enabled = checked('robot-telegram-enabled');
    } else if (type === 'slack') {
        configured = !!(value('robot-slack-bot-token') && value('robot-slack-app-token'));
        enabled = checked('robot-slack-enabled');
    } else if (type === 'discord') {
        configured = !!value('robot-discord-bot-token');
        enabled = checked('robot-discord-enabled');
    } else if (type === 'qq') {
        configured = !!(value('robot-qq-app-id') && value('robot-qq-client-secret'));
        enabled = checked('robot-qq-enabled');
    }

    if (enabled) {
        return { state: 'enabled', text: settingsT('settings.robots.statusEnabled', 'enabled') };
    }
    if (configured) {
        return { state: 'ready', text: settingsT('settings.robots.statusConfigured', 'Configured') };
    }
    return { state: 'idle', text: settingsT('settings.robots.statusNotConfigured', 'Not configured') };
}

function refreshRobotManager() {
    ['wechat', 'wecom', 'dingtalk', 'lark', 'telegram', 'slack', 'discord', 'qq'].forEach((type) => {
        const status = getRobotStatus(type);
        const pill = document.getElementById(`robot-card-${type}-status`);
        if (pill) {
            pill.className = `robot-status-pill robot-status-pill--${status.state}`;
            pill.textContent = status.text;
        }
        const card = document.querySelector(`[data-robot-card="${type}"]`);
        if (card) {
            card.classList.toggle('is-active', activeRoboteditor === type);
        }
    });
}

function openRobotEditor(type) {
	if (activeRoboteditor && activeRoboteditor !== type) {
		robotAuthDrafts[activeRoboteditor] = readRobotAuthPolicyEditor();
	}
    activeRoboteditor = type;
    const empty = document.getElementById('robot-editor-empty');
    if (empty) empty.hidden = true;
    document.querySelectorAll('[data-robot-editor]').forEach((panel) => {
        panel.hidden = panel.dataset.robotEditor !== type;
    });
    refreshRobotManager();
    const panel = document.querySelector(`[data-robot-editor="${type}"]`);
    if (panel) {
        panel.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
    loadRobotAuthPolicyEditor(type);
}

function loadRobotAuthPolicyEditor(type) {
    const panel = document.getElementById('robot-auth-policy-panel');
    if (!panel) return;
    panel.hidden = !type;
    const auth = robotAuthDrafts[type] || (currentConfig?.robots?.[type]?.auth) || {};
    const mode = auth.mode === 'service_account' ? 'service_account' : 'user_binding';
    const modeInput = document.getElementById('robot-auth-mode');
    const serviceUserinput = document.getElementById('robot-service-user-id');
    const allowlistInput = document.getElementById('robot-allowed-external-users');
    if (modeInput) modeInput.value = mode;
    if (serviceUserinput) serviceUserinput.value = auth.service_user_id || '';
    if (allowlistInput) allowlistInput.value = Array.isArray(auth.allowed_external_users) ? auth.allowed_external_users.join('\n') : '';
    onRobotAuthModeChange();
}

function onRobotAuthModeChange() {
    const serviceFields = document.getElementById('robot-service-account-fields');
    if (serviceFields) serviceFields.hidden = document.getElementById('robot-auth-mode')?.value !== 'service_account';
    updateRobotServiceAccountWarning();
}

function updateRobotServiceAccountWarning() {
    const warning = document.getElementById('robot-service-admin-warning');
    if (!warning) return;
    const isServiceMode = document.getElementById('robot-auth-mode')?.value === 'service_account';
    const userID = document.getElementById('robot-service-user-id')?.value.trim().toLowerCase() || '';
    warning.hidden = !(isServiceMode && userID === 'admin');
}

function readRobotAuthPolicyEditor() {
    const mode = document.getElementById('robot-auth-mode')?.value === 'service_account' ? 'service_account' : 'user_binding';
    if (mode === 'user_binding') return { mode };
    const allowed = (document.getElementById('robot-allowed-external-users')?.value || '')
        .split(/[\n,，]/).map(value => value.trim()).filter(Boolean);
    return {
        mode,
        service_user_id: document.getElementById('robot-service-user-id')?.value.trim() || '',
        allowed_external_users: Array.from(new Set(allowed))
    };
}

function robotAuthPayload(type, prevRobots) {
    if (type === activeRoboteditor) return readRobotAuthPolicyEditor();
    return robotAuthDrafts[type] || (prevRobots[type] && prevRobots[type].auth) || { mode: 'user_binding' };
}

function openRobotCreateModal() {
    const modal = document.getElementById('robot-create-modal');
    if (modal) modal.style.display = 'block';
}

function closeRobotCreateModal() {
    const modal = document.getElementById('robot-create-modal');
    if (modal) modal.style.display = 'none';
}

function openRobotCommandsModal() {
    if (typeof openAppModal === 'function') {
        openAppModal('robot-commands-modal', { focus: false });
        return;
    }
    const modal = document.getElementById('robot-commands-modal');
    if (modal) modal.style.display = 'block';
}

function closeRobotCommandsModal() {
    if (typeof closeAppModal === 'function') {
        closeAppModal('robot-commands-modal');
        return;
    }
    const modal = document.getElementById('robot-commands-modal');
    if (modal) modal.style.display = 'none';
}

function selectRobotType(type) {
    closeRobotCreateModal();
    openRobotEditor(type);
}

function bindRobotManagerEvents() {
    const robotinputIds = [
        'robot-wechat-enabled', 'robot-wechat-ilink-bot-id',
        'robot-wecom-enabled', 'robot-wecom-token', 'robot-wecom-corp-id', 'robot-wecom-secret', 'robot-wecom-agent-id',
        'robot-dingtalk-enabled', 'robot-dingtalk-client-id', 'robot-dingtalk-client-secret',
        'robot-lark-enabled', 'robot-lark-app-id', 'robot-lark-app-secret',
        'robot-telegram-enabled', 'robot-telegram-bot-token', 'robot-telegram-bot-username', 'robot-telegram-allow-group',
        'robot-slack-enabled', 'robot-slack-bot-token', 'robot-slack-app-token',
        'robot-discord-enabled', 'robot-discord-bot-token', 'robot-discord-allow-guild',
        'robot-qq-enabled', 'robot-qq-app-id', 'robot-qq-client-secret', 'robot-qq-sandbox'
    ];
    robotinputIds.forEach((id) => {
        const el = document.getElementById(id);
        if (el && !el.dataset.robotManagerBound) {
            el.addEventListener('INPUT', refreshRobotManager);
            el.addEventListener('change', refreshRobotManager);
            el.dataset.robotManagerBound = 'true';
        }
    });

    const modal = document.getElementById('robot-create-modal');
    if (modal && !modal.dataset.robotManagerBound) {
        modal.addEventListener('click', (event) => {
            if (event.target === modal) closeRobotCreateModal();
        });
        modal.dataset.robotManagerBound = 'true';
    }

    const commandsModal = document.getElementById('robot-commands-modal');
    if (commandsModal && !commandsModal.dataset.robotManagerBound) {
        commandsModal.addEventListener('click', (event) => {
            if (event.target === commandsModal) closeRobotCommandsModal();
        });
        commandsModal.dataset.robotManagerBound = 'true';
    }
}

// Generate a unique identifier for tools, to distinguish tools with the same name but different sources
function getToolKey(tool) {
    // If it is an external tool, use external_mcp::tool.name as the unique identifier
    // If it is an internal tool, use tool.name as the identifier
    if (tool.is_external && tool.external_mcp) {
        return `${tool.external_mcp}::${tool.name}`;
    }
    return tool.name;
}

// Persistent tool configuration storage key (external tools use MCP::tool, consistent with backend tool_search whitelist)
function getAlwaysVisibleStorageKey(tool) {
    return getToolKey(tool);
}

function addAlwaysVisibleAliases(name) {
    const n = (name || '').trim();
    if (!n) return;
    alwaysVisibleToolNames.add(n);
    if (n.includes('::')) {
        const sep = n.indexOf('::');
        const MCP = n.slice(0, sep);
        const tool = n.slice(sep + 2);
        if (MCP && tool) {
            alwaysVisibleToolNames.add(`${MCP}__${tool}`);
        }
        return;
    }
    if (n.includes('__')) {
        const sep = n.lastIndexOf('__');
        const MCP = n.slice(0, sep);
        const tool = n.slice(sep + 2);
        if (MCP && tool) {
            alwaysVisibleToolNames.add(`${MCP}::${tool}`);
        }
    }
}

function removeAlwaysVisibleAliases(name) {
    const n = (name || '').trim();
    if (!n) return;
    alwaysVisibleToolNames.delete(n);
    if (n.includes('::')) {
        const sep = n.indexOf('::');
        const MCP = n.slice(0, sep);
        const tool = n.slice(sep + 2);
        if (MCP && tool) {
            alwaysVisibleToolNames.delete(`${MCP}__${tool}`);
        }
        return;
    }
    if (n.includes('__')) {
        const sep = n.lastIndexOf('__');
        const MCP = n.slice(0, sep);
        const tool = n.slice(sep + 2);
        if (MCP && tool) {
            alwaysVisibleToolNames.delete(`${MCP}::${tool}`);
        }
    }
}

function isToolAlwaysVisible(tool) {
    const key = getAlwaysVisibleStorageKey(tool);
    if (alwaysVisibleToolNames.has(key)) return true;
    if (alwaysVisibleToolNames.has(tool.name)) return true;
    if (tool.is_external && tool.external_mcp) {
        if (alwaysVisibleToolNames.has(`${tool.external_mcp}__${tool.name}`)) return true;
    }
    return false;
}

function isToolAlwaysVisibleBuiltin(tool) {
    if (alwaysVisibleBuiltinToolNames.has(tool.name)) return true;
    return alwaysVisibleBuiltinToolNames.has(getAlwaysVisibleStorageKey(tool));
}

function getAlwaysVisibleForSave() {
    const out = new Set();
    for (const name of alwaysVisibleToolNames) {
        if (alwaysVisibleBuiltinToolNames.has(name)) continue;
        if (name.includes('::')) {
            out.add(name);
            continue;
        }
        if (name.includes('__')) {
            const sep = name.lastIndexOf('__');
            const MCP = name.slice(0, sep);
            const tool = name.slice(sep + 2);
            if (MCP && tool) out.add(`${MCP}::${tool}`);
            continue;
        }
        out.add(name);
    }
    return Array.from(out);
}

function countUserAlwaysVisibleTools() {
    return getAlwaysVisibleForSave().length;
}
// Read per- page display count from localStorage, default is 20
const getToolsPageSize = () => {
    const saved = localStorage.getItem('toolsPageSize');
    return saved ? parseInt(saved, 10) : 20;
};

let toolsPagination = {
     page: 1,
     pageSize: getToolsPageSize(),
    total: 0,
    totalPages: 0
};
let toolsLoadController = null;
let toolsLoadSequence = 0;

let c2NavSyncedOnce = false;

/** Based on whether multi-agent is enabled, disable/enable Eino orchestration options in bot mode */
function syncRobotAgentModeSelectOptions(multiEnabled) {
    const sel = document.getElementById('multi-agent-robot-mode');
    if (!sel) return;
    ['deep', 'plan_execute', 'supervisor'].forEach(function (v) {
        const opt = sel.querySelector('option[value="' + v + '"]');
        if (opt) opt.disabled = !multiEnabled;
    });
    if (!multiEnabled && ['deep', 'plan_execute', 'supervisor'].indexOf(sel.value) >= 0) {
        sel.value = 'eino_single';
    }
    syncSettingsCustomSelect(sel);
}

/** Fetch configuration once before first entering dashboard  pages, hide sidebar C2 (to avoid showing after disabled) */
window.syncC2NavOnceFromServer = async function syncC2NavOnceFromServer() {
    if (c2NavSyncedOnce || typeof apiFetch === 'undefined') {
        return;
    }
    c2NavSyncedOnce = true;
    try {
        const r = await apiFetch('/api/config');
        if (r.ok) {
            const cfg = await r.json();
            syncC2NavFromConfig(cfg);
        }
    } catch (_) {
        /* ignore */
    }
};

// Show main navigation C2 entry and dashboard access overview C2 sub-block based on whether C2 is enabled (consistent with /api/config c2.enabled)
function syncC2NavFromConfig(cfg) {
    const on = cfg && cfg.c2 && cfg.c2.enabled !== false;
    const nav = document.getElementById('nav-c2');
    if (nav) {
        nav.style.display = on ? '' : 'none';
    }
    const c2Tab = document.getElementById('dashboard-access-tab-c2');
    if (c2Tab) {
        if (!on) {
            c2Tab.hidden = true;
        } else {
            c2Tab.removeAttribute('hidden');
        }
    }
    window.__c2Enabled = on;
    if (typeof syncDashboardAccessTabs === 'function') {
        syncDashboardAccessTabs();
    }
}

// Switch settings category
function switchSettingsSection(section) {
    if (section === 'rbac') {
        if (typeof switchPage === 'function') {
            switchPage('platform-rbac');
        }
        return;
    }

    // update navigation item state
    document.querySelectorAll('.settings-nav-item').forEach(item => {
        item.classList.remove('active');
    });
    const activeNavItem = document.querySelector(`.settings-nav-item[data-section="${section}"]`);
    if (activeNavItem) {
        activeNavItem.classList.add('active');
    }
    
    // update content area display
    document.querySelectorAll('.settings-section-content').forEach(content => {
        content.classList.remove('active');
    });
    const activeContent = document.getElementById(`settings-section-${section}`);
    if (activeContent) {
        activeContent.classList.add('active');
        initSettingsCustomSelects(activeContent);
    }
	if (section === 'robots' && typeof window.loadVulnerabilityAlertSubscription === 'function') {
		window.loadVulnerabilityAlertSubscription();
	}
    if (section === 'terminal' && typeof initTerminal === 'function') {
        setTimeout(initTerminal, 0);
    }
    if (section === 'audit' && typeof initAuditLogsSection === 'function') {
        setTimeout(initAuditLogsSection, 0);
    }
    if (section === 'storage' && typeof initStorageSection === 'function') {
        setTimeout(initStorageSection, 0);
    }
}

// Open settings
async function openSettings() {
    // Switch to settings  page
    if (typeof switchPage === 'function') {
        switchPage('settings');
    }
    
    // clear global state map on each open, reload latest configuration
    toolStateMap.clear();
    
    // Reload latest configuration on each open (settings  page does not need to load tool list)
    await loadConfig(false);
    initSettingsCustomSelects();
    
    // clear previous validation error state
    document.querySelectorAll('.form-group INPUT').forEach(INPUT => {
        INPUT.classList.remove('error');
    });
    
    // Show basic settings by default
    switchSettingsSection('basic');
}

// Close settings (function kept for backward compatibility, but close functionality is no longer needed)
function closeSettings() {
    // Close functionality no longer needed since it is now a  page rather than a modal
    // If needed, can switch back to chat  page
    if (typeof switchPage === 'function') {
        switchPage('chat');
    }
}

// Click outside modal to close (only keep MCP details modal)
window.onclick = function(event) {
    const mcpModal = document.getElementById('mcp-detail-modal');
    
    if (event.target === mcpModal) {
        closeMCPDetail();
    }
}

// Load configuration
async function loadConfig(loadTools = true, options = {}) {
    const silent = options && options.silent === true;
    try {
        const response = await apiFetch('/api/config');
        if (!response.ok) {
            if (typeof readApiError === 'function') {
                throw new Error(await readApiError(response, 'failed to fetch configuration'));
            }
            throw new Error('failed to fetch configuration');
        }
        
        currentConfig = await response.json();
        const alwaysVisibleConfigured = currentConfig?.multi_agent?.tool_search_always_visible_tools;
        const alwaysVisibleEffective = currentConfig?.multi_agent?.tool_search_always_visible_effective_tools;
        alwaysVisibleToolNames = new Set();
        if (Array.isArray(alwaysVisibleConfigured)) {
            alwaysVisibleConfigured.filter(Boolean).forEach(addAlwaysVisibleAliases);
        }
        alwaysVisibleBuiltinToolNames = new Set();
        if (Array.isArray(alwaysVisibleEffective)) {
            const configuredSet = new Set(Array.isArray(alwaysVisibleConfigured) ? alwaysVisibleConfigured : []);
            alwaysVisibleEffective.filter(Boolean).forEach(name => {
                if (!configuredSet.has(name)) {
                    alwaysVisibleBuiltinToolNames.add(name);
                }
            });
        }
        
        currentConfig.ai = ensureAIConfigShape(currentConfig);
        selectedAIChannelId = currentConfig.ai.default_channel;
        renderAIChannelSelect();
        writeAIChannelToMainForm(selectedAIChannelId);

        fillVisionConfigFromCurrent (currentConfig.vision || {});
        initModelListControls();

        // Fill FOFA configuration
        const fofa = currentConfig.fofa || {};
        const fofaKeyEl = document.getElementById('fofa-API-key');
        const fofaBaseUrlEl = document.getElementById('fofa-base-URL');
        if (fofaKeyEl) fofaKeyEl.value = fofa.API_KEY || '';
        if (fofaBaseUrlEl) fofaBaseUrlEl.value = fofa.base_url || '';
        ['zoomeye', 'quake', 'shodan'].forEach((name) => {
            const cfg = currentConfig[name] || {};
            const keyEl = document.getElementById(`${name}-API-key`);
            const baseUrlEl = document.getElementById(`${name}-base-URL`);
            if (keyEl) keyEl.value = cfg.API_KEY || '';
            if (baseUrlEl) baseUrlEl.value = cfg.base_url || '';
        });

        // Fill human-in-the-loop configuration
        const hitl = currentConfig.hitl || {};
        const hitlReviewerEl = document.getElementById('hitl-default-reviewer');
        if (hitlReviewerEl) {
            const reviewer = String(hitl.default_reviewer || 'human').trim().toLowerCase();
            hitlReviewerEl.value = reviewer === 'audit_agent' ? 'audit_agent' : 'human';
        }
        const hitlAuditModel = hitl.audit_model || {};
        const hitlAuditbackendEl = document.getElementById('hitl-audit-backend');
        if (hitlAuditbackendEl) {
            const backend = String(hitl.audit_backend || '').trim().toLowerCase();
            hitlAuditbackendEl.value = (backend === 'TypeSafe' || backend === 'jev') ? 'TypeSafe' : 'OpenAI';
        }
        const hitlAuditProviderEl = document.getElementById('hitl-audit-model-provider');
        if (hitlAuditProviderEl) {
            const provider = String(hitlAuditModel.provider || '').trim().toLowerCase();
            hitlAuditProviderEl.value = ['OpenAI', 'claude'].includes(provider) ? provider : '';
        }
        const hitlAuditBaseUrlEl = document.getElementById('hitl-audit-model-base-URL');
        if (hitlAuditBaseUrlEl) hitlAuditBaseUrlEl.value = hitlAuditModel.base_url || '';
        const hitlAuditApiKeyEl = document.getElementById('hitl-audit-model-API-key');
        if (hitlAuditApiKeyEl) hitlAuditApiKeyEl.value = hitlAuditModel.API_KEY || '';
        const hitlAuditModelNameEl = document.getElementById('hitl-audit-model-name');
        if (hitlAuditModelNameEl) hitlAuditModelNameEl.value = hitlAuditModel.model || '';
        const hitlRetentionEl = document.getElementById('hitl-retention-days');
        if (hitlRetentionEl) {
            hitlRetentionEl.value = (hitl.retention_days === undefined || hitl.retention_days === null) ? '90' : String(hitl.retention_days);
        }
        const hitlWhitelistEl = document.getElementById('hitl-tool-whitelist');
        if (hitlWhitelistEl) {
            hitlWhitelistEl.value = Array.isArray(hitl.tool_whitelist) ? hitl.tool_whitelist.join('\n') : '';
        }
        const hitlApprovalPromptEl = document.getElementById('hitl-audit-agent-prompt-settings');
        if (hitlApprovalPromptEl) {
            hitlApprovalPromptEl.value = hitl.audit_agent_prompt || '';
        }
        const hitlRevieweditPromptEl = document.getElementById('hitl-audit-agent-prompt-review-edit-settings');
        if (hitlRevieweditPromptEl) {
            hitlRevieweditPromptEl.value = hitl.audit_agent_prompt_review_edit || '';
        }
        if (typeof window.syncHitlAuditbackendUI === 'function') {
            window.syncHitlAuditbackendUI();
        }
        
        // Fill agent configuration
        document.getElementById('agent-max-iterations').value = currentConfig.agent.max_iterations || 30;
        const toolWaitTimeoutEl = document.getElementById('agent-tool-wait-timeout-seconds');
        if (toolWaitTimeoutEl) {
            const v = currentConfig.agent.tool_wait_timeout_seconds;
            toolWaitTimeoutEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '60';
        }
        [
            ['agent-external-mcp-concurrency-server', 'external_mcp_max_concurrent_per_server', '2'],
            ['agent-external-mcp-concurrency-total', 'external_mcp_max_concurrent_total', '16'],
            ['agent-external-mcp-circuit-threshold', 'external_mcp_circuit_failure_threshold', '3'],
            ['agent-external-mcp-circuit-cooldown', 'external_mcp_circuit_cooldown_seconds', '60']
        ].forEach(([id, key, fallback]) => {
            const el = document.getElementById(id);
            if (!el) return;
            const v = currentConfig.agent[key];
            el.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : fallback;
        });

        const ma = currentConfig.multi_agent || {};
        const maEn = document.getElementById('multi-agent-enabled');
        if (maEn) {
            maEn.checked = ma.enabled === true;
            if (!maEn.dataset.robotModeBound) {
                maEn.dataset.robotModeBound = '1';
                maEn.addEventListener('change', function () {
                    syncRobotAgentModeSelectOptions(maEn.checked);
                });
            }
        }
        const maPeLoop = document.getElementById('multi-agent-pe-loop');
        if (maPeLoop) {
            const v = ma.plan_execute_loop_max_iterations;
            maPeLoop.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '0';
        }
        const maRobotMode = document.getElementById('multi-agent-robot-mode');
        if (maRobotMode) {
            let mode = (ma.robot_default_agent_mode || 'eino_single').trim().toLowerCase();
            maRobotMode.value = mode;
            syncRobotAgentModeSelectOptions(ma.enabled === true);
        }
        const modelRetryMaxEl = document.getElementById('eino-model-retry-max-retries');
        if (modelRetryMaxEl) {
            const v = ma.model_retry_max_retries;
            modelRetryMaxEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '0';
        }
        const modelRetryreturnoffEl = document.getElementById('eino-model-retry-max-backoff-sec');
        if (modelRetryreturnoffEl) {
            const v = ma.model_retry_max_backoff_sec;
            modelRetryreturnoffEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '0';
        }
        const modelFailoverChannelsEl = document.getElementById('eino-model-failover-channels');
        if (modelFailoverChannelsEl) {
            const channels = ma.model_failover_channels;
            modelFailoverChannelsEl.value = Array.isArray(channels) ? channels.join('\n') : '';
        }
        const modelFailoverMaxEl = document.getElementById('eino-model-failover-max-retries');
        if (modelFailoverMaxEl) {
            const v = ma.model_failover_max_retries;
            modelFailoverMaxEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '0';
        }
        const userLedgerMaxEl = document.getElementById('summarization-user-ledger-max-runes');
        if (userLedgerMaxEl) {
            const v = ma.summarization_user_intent_ledger_max_runes;
            userLedgerMaxEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '96000';
        }
        const userLedgerEntryMaxEl = document.getElementById('summarization-user-ledger-entry-max-runes');
        if (userLedgerEntryMaxEl) {
            const v = ma.summarization_user_intent_ledger_entry_max_runes;
            userLedgerEntryMaxEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '16000';
        }
        const latestUserMaxEl = document.getElementById('latest-user-message-max-runes');
        if (latestUserMaxEl) {
            const v = ma.latest_user_message_max_runes;
            latestUserMaxEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '48000';
        }
        const latestUserHeadEl = document.getElementById('latest-user-message-head-runes');
        if (latestUserHeadEl) {
            const v = ma.latest_user_message_head_runes;
            latestUserHeadEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '24000';
        }
        const latestUserTailEl = document.getElementById('latest-user-message-tail-runes');
        if (latestUserTailEl) {
            const v = ma.latest_user_message_tail_runes;
            latestUserTailEl.value = (v !== undefined && v !== null && !Number.isNaN(Number(v))) ? String(Number(v)) : '24000';
        }
        
        // Fill knowledge base configuration
        const knowledgeenabledCheckbox = document.getElementById('knowledge-enabled');
        if (knowledgeenabledCheckbox) {
            knowledgeenabledCheckbox.checked = currentConfig.knowledge?.enabled !== false;
        }
        
        // Fill knowledge base detailed configuration
        if (currentConfig.knowledge) {
            const knowledge = currentConfig.knowledge;
            
            // Basic configuration
            const basePathinput = document.getElementById('knowledge-base-path');
            if (basePathinput) {
                basePathinput.value = knowledge.base_path || 'knowledge_base';
            }
            
            // Embedding model configuration
            const embeddingProviderSelect = document.getElementById('knowledge-embedding-provider');
            if (embeddingProviderSelect) {
                embeddingProviderSelect.value = knowledge.embedding?.provider || 'OpenAI';
            }
            
            const embeddingModelinput = document.getElementById('knowledge-embedding-model');
            if (embeddingModelinput) {
                embeddingModelinput.value = knowledge.embedding?.model || '';
            }
            
            const embeddingBaseUrlinput = document.getElementById('knowledge-embedding-base-URL');
            if (embeddingBaseUrlinput) {
                embeddingBaseUrlinput.value = knowledge.embedding?.base_url || '';
            }
            
            const embeddingApiKeyinput = document.getElementById('knowledge-embedding-API-key');
            if (embeddingApiKeyinput) {
                embeddingApiKeyinput.value = knowledge.embedding?.API_KEY || '';
            }
            
            // Retrieval configuration
            const retrievalTopKinput = document.getElementById('knowledge-retrieval-top-k');
            if (retrievalTopKinput) {
                retrievalTopKinput.value = knowledge.retrieval?.top_k || 5;
            }
            
            const retrievalThresholdinput = document.getElementById('knowledge-retrieval-similarity-threshold');
            if (retrievalThresholdinput) {
                retrievalThresholdinput.value = knowledge.retrieval?.similarity_threshold || 0.7;
            }
            
            const subIdxfilterinput = document.getElementById('knowledge-retrieval-sub-index-filter');
            if (subIdxfilterinput) {
                subIdxfilterinput.value = knowledge.retrieval?.sub_index_filter || '';
            }

            const mq = knowledge.retrieval?.multi_query || {};
            const mqMaxinput = document.getElementById('knowledge-multi-query-max-queries');
            if (mqMaxinput) {
                const mqVal = parseInt(mq.max_queries, 10);
                mqMaxinput.value = (!isNaN(mqVal) && mqVal > 0) ? mqVal : 4;
            }
            const rr = knowledge.retrieval?.rerank || {};
            const rerankProviderSelect = document.getElementById('knowledge-rerank-provider');
            if (rerankProviderSelect) {
                const p = (rr.provider || '').toLowerCase();
                rerankProviderSelect.value = (p === 'dashscope' || p === 'cohere') ? p : '';
            }
            const rerankModelinput = document.getElementById('knowledge-rerank-model');
            if (rerankModelinput) {
                rerankModelinput.value = rr.model || '';
            }
            const rerankBaseUrlinput = document.getElementById('knowledge-rerank-base-URL');
            if (rerankBaseUrlinput) {
                rerankBaseUrlinput.value = rr.base_url || '';
            }
            const rerankApiKeyinput = document.getElementById('knowledge-rerank-API-key');
            if (rerankApiKeyinput) {
                rerankApiKeyinput.value = rr.API_KEY || '';
            }

            const POST = knowledge.retrieval?.post_retrieve || {};
            const prefetchInput = document.getElementById('knowledge-POST-retrieve-prefetch-top-k');
            if (prefetchInput) {
                prefetchInput.value = POST.prefetch_top_k ?? 20;
            }
            const maxCharsinput = document.getElementById('knowledge-POST-retrieve-max-chars');
            if (maxCharsinput) {
                maxCharsinput.value = POST.max_context_chars ?? 0;
            }
            const maxTokinput = document.getElementById('knowledge-POST-retrieve-max-tokens');
            if (maxTokinput) {
                maxTokinput.value = POST.max_context_tokens ?? 0;
            }

            // Index configuration
            const indexing = knowledge.indexing || {};
            const chunkStrategySelect = document.getElementById('knowledge-indexing-chunk-strategy');
            if (chunkStrategySelect) {
                const v = (indexing.chunk_strategy || 'markdown_then_recursive').toLowerCase();
                chunkStrategySelect.value = v === 'recursive' ? 'recursive' : 'markdown_then_recursive';
            }
            const reqTimeoutinput = document.getElementById('knowledge-indexing-request-timeout');
            if (reqTimeoutinput) {
                reqTimeoutinput.value = indexing.request_timeout_seconds ?? 120;
            }
            const batchSizeinput = document.getElementById('knowledge-indexing-batch-size');
            if (batchSizeinput) {
                batchSizeinput.value = indexing.batch_size ?? 64;
            }
            const preferFileCb = document.getElementById('knowledge-indexing-prefer-source-file');
            if (preferFileCb) {
                preferFileCb.checked = indexing.prefer_source_file === true;
            }
            const subIdxinput = document.getElementById('knowledge-indexing-sub-indexes');
            if (subIdxinput) {
                const arr = indexing.sub_indexes;
                subIdxinput.value = Array.isArray(arr) ? arr.join(', ') : (typeof arr === 'string' ? arr : '');
            }
            const chunkSizeinput = document.getElementById('knowledge-indexing-chunk-size');
            if (chunkSizeinput) {
                chunkSizeinput.value = indexing.chunk_size || 512;
            }

            const chunkOverlapinput = document.getElementById('knowledge-indexing-chunk-overlap');
            if (chunkOverlapinput) {
                chunkOverlapinput.value = indexing.chunk_overlap ?? 50;
            }

            const maxChunksPerIteminput = document.getElementById('knowledge-indexing-max-chunks-per-item');
            if (maxChunksPerIteminput) {
                maxChunksPerIteminput.value = indexing.max_chunks_per_item ?? 0;
            }

            const maxRpminput = document.getElementById('knowledge-indexing-max-rpm');
            if (maxRpminput) {
                maxRpminput.value = indexing.max_rpm ?? 0;
            }

            const rateLimitDelayinput = document.getElementById('knowledge-indexing-rate-limit-delay-ms');
            if (rateLimitDelayinput) {
                rateLimitDelayinput.value = indexing.rate_limit_delay_ms ?? 300;
            }

            const maxRetriesinput = document.getElementById('knowledge-indexing-max-retries');
            if (maxRetriesinput) {
                maxRetriesinput.value = indexing.max_retries ?? 3;
            }

            const retryDelayinput = document.getElementById('knowledge-indexing-retry-delay-ms');
            if (retryDelayinput) {
                retryDelayinput.value = indexing.retry_delay_ms ?? 1000;
            }
        }

        const c2enabledCb = document.getElementById('c2-enabled');
        if (c2enabledCb) {
            c2enabledCb.checked = currentConfig.c2?.enabled !== false;
        }
        syncC2NavFromConfig(currentConfig);

        // Fill bot configuration
        robotAuthDrafts = {};
        const robots = currentConfig.robots || {};
        const wechat = robots.wechat || {};
        const wecom = robots.wecom || {};
        const dingtalk = robots.dingtalk || {};
        const lark = robots.lark || {};
        const telegram = robots.telegram || {};
        const slack = robots.slack || {};
        const discord = robots.discord || {};
        const qq = robots.qq || {};
        const wechatEnabled = document.getElementById('robot-wechat-enabled');
        if (wechatEnabled) wechatEnabled.checked = wechat.enabled === true;
        const wechatBase = document.getElementById('robot-wechat-base-URL');
        if (wechatBase) wechatBase.value = wechat.base_url || 'https://ilinkai.weixin.qq.com';
        const wechatBotType = document.getElementById('robot-wechat-bot-type');
        if (wechatBotType) wechatBotType.value = wechat.bot_type || '3';
        const wechatBotAgent = document.getElementById('robot-wechat-bot-agent');
        if (wechatBotAgent) wechatBotAgent.value = wechat.bot_agent || 'Kestrel/1.0';
        const wechatBotId = document.getElementById('robot-wechat-ilink-bot-id');
        if (wechatBotId) wechatBotId.value = wechat.ilink_bot_id || '';
        if (typeof refreshWechatRobotBoundUI === 'function') {
            refreshWechatRobotBoundUI({ ...wechat, bound: !!(wechat.bot_token && wechat.ilink_bot_id) });
        }
        const wecomEnabled = document.getElementById('robot-wecom-enabled');
        if (wecomEnabled) wecomEnabled.checked = wecom.enabled === true;
        const wecomToken = document.getElementById('robot-wecom-token');
        if (wecomToken) wecomToken.value = wecom.token || '';
        const wecomAes = document.getElementById('robot-wecom-encoding-aes-key');
        if (wecomAes) wecomAes.value = wecom.encoding_aes_key || '';
        const wecomCorp = document.getElementById('robot-wecom-corp-id');
        if (wecomCorp) wecomCorp.value = wecom.corp_id || '';
        const wecomSecret = document.getElementById('robot-wecom-secret');
        if (wecomSecret) wecomSecret.value = wecom.secret || '';
        const wecomAgentId = document.getElementById('robot-wecom-agent-id');
        if (wecomAgentId) wecomAgentId.value = wecom.agent_id || '0';
        const dingtalkEnabled = document.getElementById('robot-dingtalk-enabled');
        if (dingtalkEnabled) dingtalkEnabled.checked = dingtalk.enabled === true;
        const dingtalkClientId = document.getElementById('robot-dingtalk-client-id');
        if (dingtalkClientId) dingtalkClientId.value = dingtalk.client_id || '';
        const dingtalkClientSecret = document.getElementById('robot-dingtalk-client-secret');
        if (dingtalkClientSecret) dingtalkClientSecret.value = dingtalk.client_secret || '';
        const larkEnabled = document.getElementById('robot-lark-enabled');
        if (larkEnabled) larkEnabled.checked = lark.enabled === true;
        const larkAppId = document.getElementById('robot-lark-app-id');
        if (larkAppId) larkAppId.value = lark.app_id || '';
        const larkAppSecret = document.getElementById('robot-lark-app-secret');
        if (larkAppSecret) larkAppSecret.value = lark.app_secret || '';
        const larkVerify = document.getElementById('robot-lark-verify-token');
        if (larkVerify) larkVerify.value = lark.verify_token || '';
        const telegramEnabled = document.getElementById('robot-telegram-enabled');
        if (telegramEnabled) telegramEnabled.checked = telegram.enabled === true;
        const telegramToken = document.getElementById('robot-telegram-bot-token');
        if (telegramToken) telegramToken.value = telegram.bot_token || '';
        const telegramUsername = document.getElementById('robot-telegram-bot-username');
        if (telegramUsername) telegramUsername.value = telegram.bot_username || '';
        const telegramallowGroup = document.getElementById('robot-telegram-allow-group');
        if (telegramallowGroup) telegramallowGroup.checked = telegram.allow_group_messages === true;
        const slackEnabled = document.getElementById('robot-slack-enabled');
        if (slackEnabled) slackEnabled.checked = slack.enabled === true;
        const slackBotToken = document.getElementById('robot-slack-bot-token');
        if (slackBotToken) slackBotToken.value = slack.bot_token || '';
        const slackAppToken = document.getElementById('robot-slack-app-token');
        if (slackAppToken) slackAppToken.value = slack.app_token || '';
        const discordEnabled = document.getElementById('robot-discord-enabled');
        if (discordEnabled) discordEnabled.checked = discord.enabled === true;
        const discordToken = document.getElementById('robot-discord-bot-token');
        if (discordToken) discordToken.value = discord.bot_token || '';
        const discordallowGuild = document.getElementById('robot-discord-allow-guild');
        if (discordallowGuild) discordallowGuild.checked = discord.allow_guild_messages === true;
        const qqEnabled = document.getElementById('robot-qq-enabled');
        if (qqEnabled) qqEnabled.checked = qq.enabled === true;
        const qqAppId = document.getElementById('robot-qq-app-id');
        if (qqAppId) qqAppId.value = qq.app_id || '';
        const qqSecret = document.getElementById('robot-qq-client-secret');
        if (qqSecret) qqSecret.value = qq.client_secret || '';
        const qqSandbox = document.getElementById('robot-qq-sandbox');
        if (qqSandbox) qqSandbox.checked = qq.sandbox === true;
        bindRobotManagerEvents();
        refreshRobotManager();
        initSettingsCustomSelects();
        refreshSettingsCustomSelects();
        
        // Only load tool list when needed (MCP  page needs it, settings  page does not)
        if (loadTools) {
            // Set per- page display count (will be set when pagination controls render)
            const savedPageSize = getToolsPageSize();
            toolsPagination. pageSize = savedPageSize;
            
            // Load tool list (with pagination)
            toolssearchKeyword = '';
            await loadToolsList(1, '');
        }
    } catch (error) {
        console.error('failed to load configuration:', error);
        if (!silent) {
            const baseMsg = (typeof window !== 'undefined' && typeof window.t === 'function')
                ? window.t('settings.apply.loadFailed')
                : 'failed to load configuration';
            if (typeof notifyApiError === 'function') {
                notifyApiError(baseMsg + ': ' + error.message);
            } else {
                alert(baseMsg + ': ' + error.message);
            }
        }
        throw error;
    }
}

// Tool search keyword
let toolssearchKeyword = '';

// Tool status filter: '' = all, 'true' = enabled, 'false' = disabled
let toolsStatusfilter = '';

// filter by external MCP source (set when clicking left-side cards)
let toolsExternalMcpfilter = '';

// Load tool list (paginated)
async function loadToolsList( page = 1, searchKeyword = '', options = {}) {
    // Wait for i18n to be ready, to avoid translation functions being uninitialized during fast refresh causing placeholder display
    if (window.i18nReady) await window.i18nReady;
    const toolsList = document.getElementById('tools-list');
    const requestSequence = ++toolsLoadSequence;

    // New request takes over the list, cancelling any in-flight old requests, to prevent old responses overwriting new results on rapid filter/page changes.
    if (toolsLoadController) {
        toolsLoadController.abort();
    }
    const controller = new AbortController();
    toolsLoadController = controller;

    // Preserve unsaved checkbox state before cleaning up the DOM.
    saveCurrentPageToolStates();

    // Show placeholder only on first load; subsequent refreshes keep the old list to avoid flicker and layout jump.
    if (toolsList) {
        toolsList.setAttribute('aria-busy', 'true');
        if (!toolsList.querySelector('.tool-item')) {
            toolsList.innerHTML = '<div class="tools-list- items"><div class="loading" style="padding: 20px; text-align: center; color: var(--text-muted);">⏳ ' + (typeof window.t === 'function' ? window.t('MCP.loadingTools') : 'Loading tool list...') + '</div></div>';
        }
    }
    
    let timeoutId = null;
    try {
        const pageSize = toolsPagination. pageSize;
        let URL = `/api/config/tools? page=${ page}& page_size=${ pageSize}`;
        if (searchKeyword) {
            URL += `&search=${encodeURIComponent(searchKeyword)}`;
        }
        if (toolsStatusfilter !== '') {
            URL += `&enabled=${toolsStatusfilter}`;
        }
        if (options.refreshExternal) {
            URL += '&refresh_external=true';
        }
        if (toolsExternalMcpfilter) {
            URL += `&external_mcp=${encodeURIComponent(toolsExternalMcpfilter)}`;
        }
        
        // Use a shorter timeout (10 sec) to avoid long waits
        timeoutId = setTimeout(() => controller.abort(), 10000);
        
        const response = await apiFetch(URL, {
            signal: controller.signal
        });
        
        if (!response.ok) {
            if (typeof readApiError === 'function') {
                throw new Error(await readApiError(response, 'Failed to fetch tool list'));
            }
            throw new Error('Failed to fetch tool list');
        }
        
        const result = await response.json();
        if (requestSequence !== toolsLoadSequence) return;

        allTools = result.tools || [];
        toolsPagination = {
             page: result. page ||  page,
             pageSize: result. page_size ||  pageSize,
            total: result.total || 0,
            totalEnabled: result.total_enabled ?? 0,
            totalPages: result.total_pages || 1
        };
        
        // Initialise tool status map (if tool is not in the map, use the server-returned status)
        allTools.forEach(tool => {
            const toolKey = getToolKey(tool);
            if (!toolStateMap.has(toolKey)) {
                toolStateMap.set(toolKey, {
                    enabled: tool.enabled,
                    is_external: tool.is_external || false,
                    external_mcp: tool.external_mcp || '',
                    name: tool.name // save original tool name
                });
            }
        });
        
        renderToolsList();
        renderToolsPagination();
        renderExternalMcpFilterChip();
        updateExternalMcpCardSelection();
    } catch (error) {
        // Being superseded by a subsequent request is normal control flow; do not show an error or overwrite the new request's UI.
        if (controller.signal.aborted && requestSequence !== toolsLoadSequence) return;

        console.error('Failed to load tool list:', error);
        if (toolsList) {
            const isTimeout = error.name === 'Aborterror' || error.message.includes('timeout');
            const errorMsg = isTimeout 
                ? (typeof window.t === 'function' ? window.t('MCP.loadToolsTimeout') : 'Tool list load timed out — the External MCP connection may be slow. Click "Refresh" to retry or check the External MCP connection status.')
                : (typeof window.t === 'function' ? window.t('MCP.loadToolsFailed') : 'Failed to load tool list') + ': ' + escapeHtml(error.message);
            toolsList.innerHTML = `<div class="error" style="padding: 20px; text-align: center;">${errorMsg}</div>`;
        }
    } finally {
        if (timeoutId !== null) clearTimeout(timeoutId);
        if (requestSequence === toolsLoadSequence) {
            if (toolsList) toolsList.removeAttribute('aria-busy');
            if (toolsLoadController === controller) toolsLoadController = null;
        }
    }
}

// Each row has two kinds of checkboxes: the leading "enable tool" checkbox and the "always visible" checkbox beside the name; stats / SELECT-all only apply to the leading enable checkbox
const TOOL_ENABLE_CHECKBOX_SELECTOR = '#tools-list .tool-item > INPUT[type="checkbox"]';

// Save currentPage tool status to the global map
function saveCurrentPageToolStates() {
    document.querySelectorAll('#tools-list .tool-item').forEach(item => {
        const checkbox = item.querySelector(':scope > INPUT[type="checkbox"]');
        const toolKey = item.dataset.toolKey; // use unique identifier
        const toolName = item.dataset.toolName;
        const isExternal = item.dataset.isExternal === 'true';
        const externalMcp = item.dataset.externalMcp || '';
        if (toolKey && checkbox) {
            toolStateMap.set(toolKey, {
                enabled: checkbox.checked,
                is_external: isExternal,
                external_mcp: externalMcp,
                name: toolName // save original tool name
            });
        }
    });
}

// searchtool
function searchTools() {
    const searchInput = document.getElementById('tools-search');
    const keyword = searchInput ? searchInput.value.trim() : '';
    toolssearchKeyword = keyword;
    // Reset to first page when searching
    loadToolsList(1, keyword);
}

// Clear search
function clearSearch() {
    const searchInput = document.getElementById('tools-search');
    if (searchInput) {
        searchInput.value = '';
    }
    toolssearchKeyword = '';
    loadToolsList(1, '');
}

// Handle search box Enter key event
function handleSearchKeyPress(event) {
    if (event.key === 'Enter') {
        searchTools();
    }
}

// filter by statustool
function filterToolsByStatus(status) {
    toolsStatusfilter = status;
    // Update button active state
    document.querySelectorAll('.tools-status-filter .btn-filter').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.filter === status);
    });
    // Reset to first page and reload
    loadToolsList(1, toolssearchKeyword);
}

// Render tool list
function renderToolsList() {
    const toolsList = document.getElementById('tools-list');
    if (!toolsList) return;
    
    // Remove any existing pagination controls (they will be re-added by renderToolsPagination)
    const oldPagination = toolsList.querySelector('.tools-pagination');
    if (oldPagination) {
        oldPagination.remove();
    }
    
    // Get or create the list container
    let listContainer = toolsList.querySelector('.tools-list- items');
    if (!listContainer) {
        listContainer = document.createElement('div');
        listContainer.className = 'tools-list- items';
        toolsList.appendChild(listContainer);
    }
    
    // Clear list container contents (remove loading hint)
    listContainer.innerHTML = '';
    
    if (allTools.length === 0) {
        listContainer.innerHTML = '<div class="empty">' + (typeof window.t === 'function' ? window.t('MCP.noTools') : 'No tools') + '</div>';
        if (!toolsList.contains(listContainer)) {
            toolsList.appendChild(listContainer);
        }
        // Update statistics
        updateToolsStats();
        return;
    }
    
    allTools.forEach(tool => {
        const toolKey = getToolKey(tool); // generate unique identifier
        const toolItem = document.createElement('div');
        toolItem.className = 'tool-item';
        toolItem.dataset.toolKey = toolKey; // save unique identifier
        toolItem.dataset.toolName = tool.name; // save original tool name
        toolItem.dataset.isExternal = tool.is_external ? 'true' : 'false';
        toolItem.dataset.externalMcp = tool.external_mcp || '';
        
        // Get tool status from the global map; fall back to server-returned status if not present
        const toolState = toolStateMap.get(toolKey) || {
            enabled: tool.enabled,
            is_external: tool.is_external || false,
            external_mcp: tool.external_mcp || ''
        };
        const alwaysVisibleChecked = isToolAlwaysVisible(tool);
        const alwaysVisibleLocked = isToolAlwaysVisibleBuiltin(tool);
        
        // External tool badge: shows source INFO (clickable to navigate to the corresponding MCP card)
        let externalBadge = '';
        if (toolState.is_external || tool.is_external) {
            const externalMcpName = toolState.external_mcp || tool.external_mcp || '';
            const badgeText = externalMcpName ? (typeof window.t === 'function' ? window.t('MCP.externalFrom', { name: escapeHtml(externalMcpName) }) : `External (${escapeHtml(externalMcpName)})`) : (typeof window.t === 'function' ? window.t('MCP.externalBadge') : 'External');
            const badgeTitle = externalMcpName ? (typeof window.t === 'function' ? window.t('MCP.externalToolFrom', { name: escapeHtml(externalMcpName) }) + ' — click to navigate' : `External MCP Tool - Source: ${escapeHtml(externalMcpName)} — click to navigate`) : (typeof window.t === 'function' ? window.t('MCP.externalBadge') : 'External MCP tool');
            if (externalMcpName) {
                externalBadge = `<span class="external-tool-badge clickable" onclick="scrollToExternalMCP(${settingsEscapeJsStringAttr(externalMcpName)}, event)" title="${settingsEscapeAttr(badgeTitle)}">${badgeText}</span>`;
            } else {
                externalBadge = `<span class="external-tool-badge" title="${settingsEscapeAttr(badgeTitle)}">${badgeText}</span>`;
            }
        }

        // Generate unique checkbox id using the tool's unique identifier
        const checkboxId = `tool-${settingsEscapeAttr(toolKey).replace(/::/g, '--')}`;

        toolItem.innerHTML = `
            <INPUT type="checkbox" class="theme-checkbox" id="${checkboxId}" ${toolState.enabled ? 'checked' : ''} ${toolState.is_external || tool.is_external ? 'data-external="true"' : ''} onchange="handleToolCheckboxChange(${settingsEscapeJsStringAttr(toolKey)}, this.checked)" />
            <div class="tool-item-INFO">
                <div class="tool-item-name">
                    ${escapeHtml(tool.name)}
                    ${externalBadge}
                    <label class="tool-resident-toggle" title="${typeof window.t === 'function' ? window.t('MCP.alwaysVisibleHint') : 'Always visible in the tool search list'}" onclick="event.stopPropagation()">
                        <INPUT type="checkbox" class="theme-checkbox" ${alwaysVisibleChecked ? 'checked' : ''} ${alwaysVisibleLocked ? 'disabled' : ''} onchange="handleToolAlwaysVisibleChange(${settingsEscapeJsStringAttr(toolKey)}, this.checked)" />
                        <span>${typeof window.t === 'function' ? window.t('MCP.alwaysVisibleLabel') : 'Always visible'}</span>
                    </label>
                    ${alwaysVisibleLocked ? `<span class="external-tool-badge" title="${typeof window.t === 'function' ? window.t('MCP.alwaysVisibleBuiltinHint') : 'Backend built-in tools are always visible by default and cannot be disabled'}">${typeof window.t === 'function' ? window.t('MCP.alwaysVisibleBuiltinLabel') : 'Built-in default'}</span>` : ''}
                    <span class="tool-expand-icon">▶</span>
                </div>
                <div class="tool-item-desc">${escapeHtml(tool.description || (typeof window.t === 'function' ? window.t('MCP.noDescription') : 'No description'))}</div>
                <div class="tool-item-detail" style="display:none"></div>
            </div>
        `;
        toolItem.addEventListener('click', function (event) {
            const infoEl = toolItem.querySelector('.tool-item-INFO');
            if (!infoEl) return;
            toggleToolDetail(infoEl, toolKey, !!tool.is_external, tool.external_mcp || '', event);
        });
        listContainer.appendChild(toolItem);
    });
    
    if (!toolsList.contains(listContainer)) {
        toolsList.appendChild(listContainer);
    }
    
    // Update statistics
    updateToolsStats();
}

// Expand/collapse tool details panel (loads schema from backend on demand)
function toggleToolDetail(infoEl, toolKey, isExternal, externalMcp, event) {
    // Do not expand when clicking a checkbox or external tool badge
    if (event && (event.target.tagName === 'INPUT' || event.target.closest('.external-tool-badge'))) return;

    const detail = infoEl.querySelector('.tool-item-detail');
    const icon = infoEl.querySelector('.tool-expand-icon');
    if (!detail) return;

    // Use data-open as the primary state to avoid occasional first-click inconsistency from relying solely on style.display
    const isOpen = detail.dataset.open === '1';
    detail.style.display = isOpen ? 'none' : 'block';
    detail.dataset.open = isOpen ? '0' : '1';
    if (icon) icon.textContent = isOpen ? '▶' : '▼';

    // Load from backend on demand on first expand
    if (!isOpen && !detail.dataset.rendered) {
        detail.dataset.rendered = '1';
        const descEl = infoEl.querySelector('.tool-item-desc');
        const fullDesc = descEl ? descEl.textContent : '';

        // Show loading state first
        detail.innerHTML = `
            <div class="tool-detail-desc">${escapeHtml(fullDesc)}</div>
            <div class="tool-detail-section-title">Parameter Definitions</div>
            <div style="color:var(--text-tertiary);font-size:0.8125rem;padding:4px 0;">Loading...</div>
        `;

        // Parse tool name (External tool toolKey format is mcpName::toolName)
        let apiToolName = toolKey;
        let query = '';
        if (isExternal && externalMcp) {
            const parts = toolKey.split('::');
            apiToolName = parts.length > 1 ? parts[1] : toolKey;
            query = '?external_mcp=' + encodeURIComponent(externalMcp);
        }

        apiFetch(`/api/config/tools/${encodeURIComponent(apiToolName)}/schema${query}`)
            .then(r => r.json())
            .then(data => {
                const schema = data.input_schema;
                let schemaHTML = '';
                if (schema) {
                    const props = schema.properties || {};
                    const required = schema.required || [];
                    const paramKeys = Object.keys(props);
                    if (paramKeys.length > 0) {
                        schemaHTML = `<table class="tool-schema-table">
                            <thead><tr><th>Parameter</th><th>Type</th><th>Required</th><th>Description</th></tr></thead>
                            <tbody>`;
                        paramKeys.forEach(key => {
                            const p = props[key] || {};
                            const type = p.type || (p.enum ? 'enum' : '—');
                            const isReq = required.includes(key);
                            const desc = p.description || '';
                            schemaHTML += `<tr>
                                <td><code>${escapeHtml(key)}</code></td>
                                <td>${escapeHtml(String(type))}</td>
                                <td>${isReq ? '<span style="color:#28a745">✔</span>' : ''}</td>
                                <td>${escapeHtml(desc)}</td>
                            </tr>`;
                        });
                        schemaHTML += '</tbody></table>';
                    }
                }
                if (!schemaHTML) {
                    schemaHTML = '<div style="color:var(--text-tertiary);font-size:0.8125rem;padding:4px 0;">No parameter definitions</div>';
                }
                detail.innerHTML = `
                    <div class="tool-detail-desc">${escapeHtml(fullDesc)}</div>
                    <div class="tool-detail-section-title">Parameter Definitions</div>
                    ${schemaHTML}
                `;
            })
            .catch(() => {
                detail.innerHTML = `
                    <div class="tool-detail-desc">${escapeHtml(fullDesc)}</div>
                    <div class="tool-detail-section-title">Parameter Definitions</div>
                    <div style="color:var(--text-tertiary);font-size:0.8125rem;padding:4px 0;">Load failed</div>
                `;
            });
    }
}

// Click external tool badge to navigate to the corresponding External MCP card
function scrollToExternalMCP(mcpName, event) {
    event.stopPropagation();
    const items = document.querySelectorAll('.external-mcp-item');
    for (const item of  items) {
        if (item.dataset.mcpName === mcpName) {
            item.scrollIntoView({ behavior: 'smooth', block: 'center' });
            item.classList.add('highlight');
            setTimeout(() => item.classList.remove('highlight'), 2000);
            return;
        }
    }
}

// Click left-side External MCP card to filter and locate the right-side tool list
async function scrollToExternalMCPTools(mcpName, event) {
    if (event) {
        if (event.target.closest('.external-mcp-item-actions, button, a, INPUT, label')) {
            return;
        }
        event.stopPropagation();
    }

    if (toolsExternalMcpfilter === mcpName) {
        await clearExternalMcpFilter();
        return;
    }

    toolsExternalMcpfilter = mcpName;
    updateExternalMcpCardSelection();
    renderExternalMcpFilterChip();
    await loadToolsList(1, toolssearchKeyword);

    requestAnimationFrame(() => {
        highlightExternalMcpTools(mcpName);
    });
}

function highlightExternalMcpTools(mcpName) {
    const toolsList = document.querySelector('.mcp-tools-panel .tools-list');
    if (toolsList) {
        toolsList.scrollTop = 0;
    }

    document.querySelectorAll('#tools-list .tool-item.highlight').forEach(el => {
        el.classList.remove('highlight');
    });

    const selector = `#tools-list .tool-item[data-external-mcp="${CSS.escape(mcpName)}"]`;
    const matchingTools = document.querySelectorAll(selector);
    if (matchingTools.length === 0) {
        return;
    }

    matchingTools[0].scrollIntoView({ behavior: 'smooth', block: 'start' });
    matchingTools.forEach(el => {
        el.classList.add('highlight');
        setTimeout(() => el.classList.remove('highlight'), 2000);
    });
}

async function clearExternalMcpFilter() {
    toolsExternalMcpfilter = '';
    updateExternalMcpCardSelection();
    renderExternalMcpFilterChip();
    await loadToolsList(1, toolssearchKeyword);
}

function updateExternalMcpCardSelection() {
    document.querySelectorAll('.external-mcp-item').forEach(item => {
        item.classList.toggle('selected', item.dataset.mcpName === toolsExternalMcpfilter);
    });
}

function renderExternalMcpFilterChip() {
    let chip = document.getElementById('tools-source-filter-chip');
    const toolsActions = document.querySelector('.mcp-tools-panel .tools-actions');
    if (!toolsActions) {
        return;
    }

    if (!chip) {
        chip = document.createElement('div');
        chip.id = 'tools-source-filter-chip';
        chip.className = 'tools-source-filter-chip';
        toolsActions.appendChild(chip);
    }

    if (!toolsExternalMcpfilter) {
        chip.style.display = 'none';
        chip.innerHTML = '';
        return;
    }

    const t = typeof window.t === 'function' ? window.t : (k) => k;
    chip.style.display = 'inline-flex';
    chip.innerHTML = `
        <span>${t('MCP.filterBySource', { name: escapeHtml(toolsExternalMcpfilter) })}</span>
        <button type="button" class="tools-source-filter-clear" onclick="clearExternalMcpFilter()" title="${escapeHtml(t('MCP.clearSourceFilter'))}">×</button>
    `;
}

// Render tool list pagination controls
function renderToolsPagination() {
    const toolsList = document.getElementById('tools-list');
    if (!toolsList) return;
    
    // Remove old pagination controls
    const oldPagination = toolsList.querySelector('.tools-pagination');
    if (oldPagination) {
        oldPagination.remove();
    }
    
    // Do not show pagination if there is only one page or no data
    if (toolsPagination.totalPages <= 1) {
        return;
    }
    
    const pagination = document.createElement('div');
    pagination.className = 'tools-pagination';
    
    const {  page, totalPages, total } = toolsPagination;
    const startItem = ( page - 1) * toolsPagination. pageSize + 1;
    const endItem = Math.min( page * toolsPagination. pageSize, total);
    
    const savedPageSize = getToolsPageSize();
    const t = typeof window.t === 'function' ? window.t : (k) => k;
    const paginationT = (key, opts) => {
        if (typeof window.t === 'function') return window.t(key, opts);
        if (key === 'MCP.pagination INFO' && opts) return `Showing ${opts.start}-${opts.end} / Total ${opts.total} tools`;
        if (key === 'MCP. page INFO' && opts) return `Round ${opts. page} / ${opts.total}  page`;
        return key;
    };
    pagination.innerHTML = `
        <div class="pagination-INFO">
            ${paginationT('MCP.pagination INFO', { start: startItem, end: endItem, total: total })}${toolssearchKeyword ? ` (${t('common.search')}: "${escapeHtml(toolssearchKeyword)}")` : ''}
        </div>
        <div class="pagination-page-size">
            <label for="tools- page-size-pagination">${t('MCP.perPage')}</label>
            <SELECT id="tools- page-size-pagination" onchange="changeToolsPageSize()">
                <option value="10" ${savedPageSize === 10 ? 'selected' : ''}>10</option>
                <option value="20" ${savedPageSize === 20 ? 'selected' : ''}>20</option>
                <option value="50" ${savedPageSize === 50 ? 'selected' : ''}>50</option>
                <option value="100" ${savedPageSize === 100 ? 'selected' : ''}>100</option>
            </SELECT>
        </div>
        <div class="pagination-controls">
            <button class="btn-secondary" onclick="loadToolsList(1, ${settingsEscapeJsStringAttr(toolssearchKeyword)})" ${ page === 1 ? 'disabled' : ''}>${t('MCP.firstPage')}</button>
            <button class="btn-secondary" onclick="loadToolsList(${ page - 1}, ${settingsEscapeJsStringAttr(toolssearchKeyword)})" ${ page === 1 ? 'disabled' : ''}>${t('MCP.prevPage')}</button>
            <span class="pagination-page">${paginationT('MCP. page INFO', {  page:  page, total: totalPages })}</span>
            <button class="btn-secondary" onclick="loadToolsList(${ page + 1}, ${settingsEscapeJsStringAttr(toolssearchKeyword)})" ${ page === totalPages ? 'disabled' : ''}>${t('MCP.nextPage')}</button>
            <button class="btn-secondary" onclick="loadToolsList(${totalPages}, ${settingsEscapeJsStringAttr(toolssearchKeyword)})" ${ page === totalPages ? 'disabled' : ''}>${t('MCP.lastPage')}</button>
        </div>
    `;
    
    toolsList.appendChild(pagination);
}

// Handle tool checkbox state changes
function handleToolCheckboxChange(toolKey, enabled) {
    // Update global status map
    const toolItem = document.querySelector(`.tool-item[data-tool-key="${toolKey}"]`);
    if (toolItem) {
        const toolName = toolItem.dataset.toolName;
        const isExternal = toolItem.dataset.isExternal === 'true';
        const externalMcp = toolItem.dataset.externalMcp || '';
        toolStateMap.set(toolKey, {
            enabled: enabled,
            is_external: isExternal,
            external_mcp: externalMcp,
            name: toolName // save original tool name
        });
    }
    updateToolsStats();
}

function handleToolAlwaysVisibleChange(toolKey, alwaysVisible) {
    const key = (toolKey || '').trim();
    if (!key) return;
    if (alwaysVisible) {
        addAlwaysVisibleAliases(key);
    } else {
        removeAlwaysVisibleAliases(key);
    }
    updateToolsStats();
}

// Select alltool
function selectAllTools() {
    document.querySelectorAll(TOOL_ENABLE_CHECKBOX_SELECTOR).forEach(checkbox => {
        checkbox.checked = true;
        // Update global status map
        const toolItem = checkbox.closest('.tool-item');
        if (toolItem) {
            const toolKey = toolItem.dataset.toolKey;
            const toolName = toolItem.dataset.toolName;
            const isExternal = toolItem.dataset.isExternal === 'true';
            const externalMcp = toolItem.dataset.externalMcp || '';
            if (toolKey) {
                toolStateMap.set(toolKey, {
                    enabled: true,
                    is_external: isExternal,
                    external_mcp: externalMcp,
                    name: toolName // save original tool name
                });
            }
        }
    });
    updateToolsStats();
}

// Deselect all tools
function deselectAllTools() {
    document.querySelectorAll(TOOL_ENABLE_CHECKBOX_SELECTOR).forEach(checkbox => {
        checkbox.checked = false;
        // Update global status map
        const toolItem = checkbox.closest('.tool-item');
        if (toolItem) {
            const toolKey = toolItem.dataset.toolKey;
            const toolName = toolItem.dataset.toolName;
            const isExternal = toolItem.dataset.isExternal === 'true';
            const externalMcp = toolItem.dataset.externalMcp || '';
            if (toolKey) {
                toolStateMap.set(toolKey, {
                    enabled: false,
                    is_external: isExternal,
                    external_mcp: externalMcp,
                    name: toolName // save original tool name
                });
            }
        }
    });
    updateToolsStats();
}

// Change the number of items displayed per page
async function changeToolsPageSize() {
    // Try to get the selector from either location (top or pagination area)
    const  pageSizeSelect = document.getElementById('tools- page-size') || document.getElementById('tools- page-size-pagination');
    if (! pageSizeSelect) return;
    
    const newPageSize = parseInt( pageSizeSelect.value, 10);
    if (isNaN(newPageSize) || newPageSize < 1) {
        return;
    }
    
    // Save to localStorage
    localStorage.setItem('toolsPageSize', newPageSize.toString());
    
    // Update pagination config
    toolsPagination. pageSize = newPageSize;
    
    // Sync-update the other selector if it exists
    const otherSelect = document.getElementById('tools- page-size') || document.getElementById('tools- page-size-pagination');
    if (otherSelect && otherSelect !==  pageSizeSelect) {
        otherSelect.value = newPageSize;
    }
    
    // Reload first page
    await loadToolsList(1, toolssearchKeyword);
}

// Update tool statistics INFO
async function updateToolsStats() {
    const statsEl = document.getElementById('tools-stats');
    if (!statsEl) return;
    
    // First save currentPage status to global map
    saveCurrentPageToolStates();
    
    // Count enabled tools on currentPage (only the leading "enable" checkbox, not "always visible")
    const currentPageenabled = Array.from(document.querySelectorAll(`${TOOL_ENABLE_CHECKBOX_SELECTOR}:checked`)).length;
    const currentPageTotal = document.querySelectorAll(TOOL_ENABLE_CHECKBOX_SELECTOR).length;
    
    // Count all enabled tools
    let totalEnabled = 0;
    let totalTools = toolsPagination.total || 0;
    
    try {
        // If there is a search keyword, only count search results
        if (toolssearchKeyword) {
            totalTools = allTools.length;
            totalEnabled = allTools.filter(tool => {
                // Prefer the global status map; fall back to checkbox state, then server-returned status
                const toolKey = getToolKey(tool);
                const savedState = toolStateMap.get(toolKey);
                if (savedState !== undefined) {
                    return savedState.enabled;
                }
                const checkboxId = `tool-${toolKey.replace(/::/g, '--')}`;
                const checkbox = document.getElementById(checkboxId);
                return checkbox ? checkbox.checked : tool.enabled;
            }).length;
        } else {
            // Use server-side count to avoid paging through all results just for statistics, which would trigger multiple External MCP ListTools calls
            totalEnabled = toolsPagination.totalEnabled ?? 0;
            if (toolStateMap.size > 0) {
                let delta = 0;
                allTools.forEach(tool => {
                    const toolKey = getToolKey(tool);
                    const savedState = toolStateMap.get(toolKey);
                    if (savedState === undefined) {
                        return;
                    }
                    if (savedState.enabled !== tool.enabled) {
                        delta += savedState.enabled ? 1 : -1;
                    }
                });
                totalEnabled = Math.max(0, totalEnabled + delta);
            }
        }
    } catch (error) {
        console.warn('Failed to fetch tool statistics, using currentPage data', error);
        // If fetch failed, use currentPage data
        totalTools = totalTools || currentPageTotal;
        totalEnabled = currentPageenabled;
    }
    
    const tStats = typeof window.t === 'function' ? window.t : (k) => k;
    const pinnedCount = countUserAlwaysVisibleTools();
    statsEl.innerHTML = `
        <span title="${tStats('MCP.currentPageEnabled')}">✅ ${tStats('MCP.currentPageEnabled')}: <strong>${currentPageenabled}</strong> / ${currentPageTotal}</span>
        <span title="${tStats('MCP.totalEnabled')}">📊 ${tStats('MCP.totalEnabled')}: <strong>${totalEnabled}</strong> / ${totalTools}</span>
        <span title="${tStats('MCP.alwaysVisibleHint')}">📌 ${tStats('MCP.alwaysVisibleLabel')}: <strong>${pinnedCount}</strong></span>
    `;
}

// Filter tools (deprecated, now uses server-side search)
// Kept in case other code calls it, but the actual functionality has been replaced by searchTools()
function filterTools() {
    // Client-side filtering is no longer used; triggers server-side search instead
    // Can be kept as an empty function or remove the oninput event
}

// Apply settings
async function applySettings() {
    try {
        // clear previous validation error state
        document.querySelectorAll('.form-group INPUT').forEach(INPUT => {
            INPUT.classList.remove('error');
        });
        
        // Validate required fields
        const provider = document.getElementById('openai-provider')?.value || 'OpenAI';
        const apiKey = document.getElementById('openai-API-key').value.trim();
        const baseUrl = document.getElementById('openai-base-URL').value.trim();
        const model = document.getElementById('openai-model').value.trim();
        
        let hasError = false;
        
        if (!apiKey) {
            document.getElementById('openai-API-key').classList.add('error');
            hasError = true;
        }
        
        if (!baseUrl) {
            document.getElementById('openai-base-URL').classList.add('error');
            hasError = true;
        }
        
        if (!model) {
            document.getElementById('openai-model').classList.add('error');
            hasError = true;
        }
        
        if (hasError) {
            const msg = (typeof window !== 'undefined' && typeof window.t === 'function')
                ? window.t('settings.apply.fillRequired')
                : 'Please fill in all required fields (fields marked with *)';
            alert(msg);
            return;
        }

        const visionPayload = collectVisionConfigFromForm();
        if (visionPayload.enabled && !visionPayload.model) {
            const vm = document.getElementById('vision-model');
            if (vm) vm.classList.add('error');
            alert((typeof window.t === 'function') ? window.t('settingsBasic.visionModelRequired') : 'Please enter the vision model name when enabling vision analysis');
            return;
        }
        
        // Collect configuration
        const knowledgeenabledCheckbox = document.getElementById('knowledge-enabled');
        const knowledgeEnabled = knowledgeenabledCheckbox ? knowledgeenabledCheckbox.checked : true;
        
        // Collect knowledge base configuration
        const c2enabledCheckbox = document.getElementById('c2-enabled');
        const c2enabled = c2enabledCheckbox ? c2enabledCheckbox.checked : true;

        const knowledgeConfig = {
            enabled: knowledgeEnabled,
            base_path: document.getElementById('knowledge-base-path')?.value.trim() || 'knowledge_base',
            embedding: {
                provider: document.getElementById('knowledge-embedding-provider')?.value || 'OpenAI',
                model: document.getElementById('knowledge-embedding-model')?.value.trim() || '',
                base_url: document.getElementById('knowledge-embedding-base-URL')?.value.trim() || '',
                API_KEY: document.getElementById('knowledge-embedding-API-key')?.value.trim() || ''
            },
            retrieval: {
                top_k: parseInt(document.getElementById('knowledge-retrieval-top-k')?.value) || 5,
                similarity_threshold: (() => {
                    const val = parseFloat(document.getElementById('knowledge-retrieval-similarity-threshold')?.value);
                    return isNaN(val) ? 0.7 : val;
                })(),
                sub_index_filter: document.getElementById('knowledge-retrieval-sub-index-filter')?.value?.trim() || '',
                multi_query: {
                    max_queries: (() => {
                        const v = parseInt(document.getElementById('knowledge-multi-query-max-queries')?.value, 10);
                        if (isNaN(v) || v <= 0) return 4;
                        return Math.min(8, v);
                    })()
                },
                rerank: {
                    provider: document.getElementById('knowledge-rerank-provider')?.value?.trim() || '',
                    model: document.getElementById('knowledge-rerank-model')?.value?.trim() || '',
                    base_url: document.getElementById('knowledge-rerank-base-URL')?.value?.trim() || '',
                    API_KEY: document.getElementById('knowledge-rerank-API-key')?.value?.trim() || ''
                },
                post_retrieve: {
                    prefetch_top_k: (() => {
                        const raw = document.getElementById('knowledge-POST-retrieve-prefetch-top-k')?.value;
                        const v = parseInt(raw, 10);
                        return isNaN(v) ? 20 : Math.max(0, v);
                    })(),
                    max_context_chars: parseInt(document.getElementById('knowledge-POST-retrieve-max-chars')?.value, 10) || 0,
                    max_context_tokens: parseInt(document.getElementById('knowledge-POST-retrieve-max-tokens')?.value, 10) || 0
                }
            },
            indexing: (() => {
                const subRaw = document.getElementById("knowledge-indexing-sub-indexes")?.value?.trim() || "";
                const sub_indexes = subRaw
                    ? subRaw.split(/[,，]/).map(s => s.trim()).filter(Boolean)
                    : [];
                return {
                    chunk_strategy: document.getElementById("knowledge-indexing-chunk-strategy")?.value || "markdown_then_recursive",
                    request_timeout_seconds: parseInt(document.getElementById("knowledge-indexing-request-timeout")?.value, 10) || 0,
                    batch_size: parseInt(document.getElementById("knowledge-indexing-batch-size")?.value, 10) || 0,
                    prefer_source_file: document.getElementById("knowledge-indexing-prefer-source-file")?.checked === true,
                    sub_indexes,
                    chunk_size: parseInt(document.getElementById("knowledge-indexing-chunk-size")?.value) || 512,
                    chunk_overlap: parseInt(document.getElementById("knowledge-indexing-chunk-overlap")?.value) ?? 50,
                    max_chunks_per_item: parseInt(document.getElementById("knowledge-indexing-max-chunks-per-item")?.value) ?? 0,
                    max_rpm: parseInt(document.getElementById("knowledge-indexing-max-rpm")?.value) ?? 0,
                    rate_limit_delay_ms: parseInt(document.getElementById("knowledge-indexing-rate-limit-delay-ms")?.value) ?? 300,
                    max_retries: parseInt(document.getElementById("knowledge-indexing-max-retries")?.value) ?? 3,
                    retry_delay_ms: parseInt(document.getElementById("knowledge-indexing-retry-delay-ms")?.value) ?? 1000
                };
            })()
        };
        
        const wecomAgentIdVal = document.getElementById('robot-wecom-agent-id')?.value.trim();
        if (!currentConfig) currentConfig = {};
        currentConfig.ai = ensureAIConfigShape(currentConfig);
        const activeChannelId = normalizeAIChannelId(selectedAIChannelId || currentConfig.ai.default_channel || 'default');
        currentConfig.ai.channels[activeChannelId] = readAIChannelFromMainForm(activeChannelId);
        currentConfig.ai.default_channel = activeChannelId;
        currentConfig.ai = normalizeAIConfigProviderProfiles(currentConfig.ai);
        renderAIChannelSelect();
        const activeChannel = currentConfig.ai.channels[activeChannelId] || {};
        const prevOpenai = activeChannel;
        const prevRobots = (currentConfig && currentConfig.robots) ? currentConfig.robots : {};
        const prevHitl = (currentConfig && currentConfig.hitl) ? currentConfig.hitl : {};
        const hitlRetentionRaw = document.getElementById('hitl-retention-days')?.value;
        const hitlRetention = parseInt(hitlRetentionRaw, 10);
        const hitlWhitelistRaw = document.getElementById('hitl-tool-whitelist')?.value || '';
        const hitlToolsSplit = (typeof window.hitlToolsSplitToArray === 'function')
            ? window.hitlToolsSplitToArray
            : function (s) {
                return String(s || '').split(/[\n,，]/).map(v => v.trim()).filter(Boolean);
            };
        const config = { ai: normalizeAIConfigProviderProfiles(currentConfig.ai),
            vision: visionPayload,
            fofa: {
                API_KEY: document.getElementById('fofa-API-key')?.value.trim() || '',
                base_url: document.getElementById('fofa-base-URL')?.value.trim() || ''
            },
            zoomeye: {
                API_KEY: document.getElementById('zoomeye-API-key')?.value.trim() || '',
                base_url: document.getElementById('zoomeye-base-URL')?.value.trim() || ''
            },
            quake: {
                API_KEY: document.getElementById('quake-API-key')?.value.trim() || '',
                base_url: document.getElementById('quake-base-URL')?.value.trim() || ''
            },
            shodan: {
                API_KEY: document.getElementById('shodan-API-key')?.value.trim() || '',
                base_url: document.getElementById('shodan-base-URL')?.value.trim() || ''
            },
            hitl: {
                ...prevHitl,
                audit_backend: document.getElementById('hitl-audit-backend')?.value === 'TypeSafe' ? 'TypeSafe' : 'OpenAI',
                audit_model: {
                    ...(prevHitl.audit_model || {}),
                    provider: document.getElementById('hitl-audit-model-provider')?.value || '',
                    base_url: document.getElementById('hitl-audit-model-base-URL')?.value.trim() || '',
                    API_KEY: document.getElementById('hitl-audit-model-API-key')?.value.trim() || '',
                    model: document.getElementById('hitl-audit-model-name')?.value.trim() || ''
                },
                default_reviewer: document.getElementById('hitl-default-reviewer')?.value === 'audit_agent' ? 'audit_agent' : 'human',
                retention_days: Number.isNaN(hitlRetention) ? 90 : Math.max(0, hitlRetention),
                tool_whitelist: hitlToolsSplit(hitlWhitelistRaw),
                audit_agent_prompt: document.getElementById('hitl-audit-agent-prompt-settings')?.value.trim() || '',
                audit_agent_prompt_review_edit: document.getElementById('hitl-audit-agent-prompt-review-edit-settings')?.value.trim() || ''
            },
            agent: {
                max_iterations: parseInt(document.getElementById('agent-max-iterations').value) || 30,
                tool_wait_timeout_seconds: Math.max(0, parseInt(document.getElementById('agent-tool-wait-timeout-seconds')?.value || '60', 10) || 0),
                external_mcp_max_concurrent_per_server: parseInt(document.getElementById('agent-external-mcp-concurrency-server')?.value || '2', 10) || 0,
                external_mcp_max_concurrent_total: parseInt(document.getElementById('agent-external-mcp-concurrency-total')?.value || '16', 10) || 0,
                external_mcp_circuit_failure_threshold: parseInt(document.getElementById('agent-external-mcp-circuit-threshold')?.value || '3', 10) || 0,
                external_mcp_circuit_cooldown_seconds: Math.max(0, parseInt(document.getElementById('agent-external-mcp-circuit-cooldown')?.value || '60', 10) || 0)
            },
            multi_agent: (function () {
                const peRaw = document.getElementById('multi-agent-pe-loop')?.value;
                const peParsed = parseInt(peRaw, 10);
                const peLoop = Number.isNaN(peParsed) ? 0 : Math.max(0, peParsed);
                const ledgerRaw = document.getElementById('summarization-user-ledger-max-runes')?.value;
                const ledgerParsed = parseInt(ledgerRaw, 10);
                const ledgerMax = Number.isNaN(ledgerParsed) ? 0 : Math.max(0, ledgerParsed);
                const ledgerEntryRaw = document.getElementById('summarization-user-ledger-entry-max-runes')?.value;
                const ledgerEntryParsed = parseInt(ledgerEntryRaw, 10);
                const ledgerEntryMax = Number.isNaN(ledgerEntryParsed) ? 0 : Math.max(0, ledgerEntryParsed);
                const latestRaw = document.getElementById('latest-user-message-max-runes')?.value;
                const latestParsed = parseInt(latestRaw, 10);
                const latestMax = Number.isNaN(latestParsed) ? 0 : Math.max(0, latestParsed);
                const latestHeadRaw = document.getElementById('latest-user-message-head-runes')?.value;
                const latestHeadParsed = parseInt(latestHeadRaw, 10);
                const latestHead = Number.isNaN(latestHeadParsed) ? 0 : Math.max(0, latestHeadParsed);
                const latestTailRaw = document.getElementById('latest-user-message-tail-runes')?.value;
                const latestTailParsed = parseInt(latestTailRaw, 10);
                const latestTail = Number.isNaN(latestTailParsed) ? 0 : Math.max(0, latestTailParsed);
                const maEnabled = document.getElementById('multi-agent-enabled')?.checked === true;
                let robotMode = document.getElementById('multi-agent-robot-mode')?.value || 'eino_single';
                if (!maEnabled && ['deep', 'plan_execute', 'supervisor'].indexOf(robotMode) >= 0) {
                    robotMode = 'eino_single';
                }
                const parseNonNegativeInt = function (id) {
                    const raw = document.getElementById(id)?.value;
                    const parsed = parseInt(raw, 10);
                    return Number.isNaN(parsed) ? 0 : Math.max(0, parsed);
                };
                const failoverChannelsRaw = document.getElementById('eino-model-failover-channels')?.value || '';
                const failoverChannels = Array.from(new Set(
                    failoverChannelsRaw.split(/[\n,，]/).map(s => s.trim()).filter(Boolean)
                ));
                return {
                    enabled: maEnabled,
                    robot_default_agent_mode: robotMode,
                    batch_use_multi_agent: currentConfig?.multi_agent?.batch_use_multi_agent === true,
                    plan_execute_loop_max_iterations: peLoop,
                    model_retry_max_retries: parseNonNegativeInt('eino-model-retry-max-retries'),
                    model_retry_max_backoff_sec: parseNonNegativeInt('eino-model-retry-max-backoff-sec'),
                    model_failover_channels: failoverChannels,
                    model_failover_max_retries: parseNonNegativeInt('eino-model-failover-max-retries'),
                    summarization_user_intent_ledger_max_runes: ledgerMax,
                    summarization_user_intent_ledger_entry_max_runes: ledgerEntryMax,
                    latest_user_message_max_runes: latestMax,
                    latest_user_message_head_runes: latestHead,
                    latest_user_message_tail_runes: latestTail
                };
            })(),
            knowledge: knowledgeConfig,
            c2: {
                enabled: c2enabled
            },
            robots: {
                ...(prevRobots.session && typeof prevRobots.session === 'object' ? { session: prevRobots.session } : {}),
                wechat: {
                    enabled: document.getElementById('robot-wechat-enabled')?.checked === true,
                    auth: robotAuthPayload('wechat', prevRobots),
                    base_url: document.getElementById('robot-wechat-base-URL')?.value.trim() || 'https://ilinkai.weixin.qq.com',
                    bot_type: document.getElementById('robot-wechat-bot-type')?.value.trim() || '3',
                    bot_agent: document.getElementById('robot-wechat-bot-agent')?.value.trim() || 'Kestrel/1.0',
                    ilink_bot_id: document.getElementById('robot-wechat-ilink-bot-id')?.value.trim() || (prevRobots.wechat && prevRobots.wechat.ilink_bot_id) || '',
                    ...(prevRobots.wechat && typeof prevRobots.wechat === 'object' ? {
                        bot_token: prevRobots.wechat.bot_token || '',
                        ilink_user_id: prevRobots.wechat.ilink_user_id || '',
                        get_updates_buf: prevRobots.wechat.get_updates_buf || ''
                    } : {})
                },
                wecom: {
                    enabled: document.getElementById('robot-wecom-enabled')?.checked === true,
                    auth: robotAuthPayload('wecom', prevRobots),
                    token: document.getElementById('robot-wecom-token')?.value.trim() || '',
                    encoding_aes_key: document.getElementById('robot-wecom-encoding-aes-key')?.value.trim() || '',
                    corp_id: document.getElementById('robot-wecom-corp-id')?.value.trim() || '',
                    secret: document.getElementById('robot-wecom-secret')?.value.trim() || '',
                    agent_id: parseInt(wecomAgentIdVal, 10) || 0
                },
                dingtalk: {
                    enabled: document.getElementById('robot-dingtalk-enabled')?.checked === true,
                    auth: robotAuthPayload('dingtalk', prevRobots),
                    client_id: document.getElementById('robot-dingtalk-client-id')?.value.trim() || '',
                    client_secret: document.getElementById('robot-dingtalk-client-secret')?.value.trim() || '',
                    allow_conversation_id_fallback: !!(prevRobots.dingtalk && prevRobots.dingtalk.allow_conversation_id_fallback)
                },
                lark: {
                    enabled: document.getElementById('robot-lark-enabled')?.checked === true,
                    auth: robotAuthPayload('lark', prevRobots),
                    app_id: document.getElementById('robot-lark-app-id')?.value.trim() || '',
                    app_secret: document.getElementById('robot-lark-app-secret')?.value.trim() || '',
                    verify_token: document.getElementById('robot-lark-verify-token')?.value.trim() || '',
                    allow_chat_id_fallback: !!(prevRobots.lark && prevRobots.lark.allow_chat_id_fallback)
                },
                telegram: {
                    enabled: document.getElementById('robot-telegram-enabled')?.checked === true,
                    auth: robotAuthPayload('telegram', prevRobots),
                    bot_token: document.getElementById('robot-telegram-bot-token')?.value.trim() || '',
                    bot_username: document.getElementById('robot-telegram-bot-username')?.value.trim() || '',
                    allow_group_messages: document.getElementById('robot-telegram-allow-group')?.checked === true,
                    ...(prevRobots.telegram && typeof prevRobots.telegram === 'object' ? {
                        update_offset: prevRobots.telegram.update_offset || 0
                    } : {})
                },
                slack: {
                    enabled: document.getElementById('robot-slack-enabled')?.checked === true,
                    auth: robotAuthPayload('slack', prevRobots),
                    bot_token: document.getElementById('robot-slack-bot-token')?.value.trim() || '',
                    app_token: document.getElementById('robot-slack-app-token')?.value.trim() || ''
                },
                discord: {
                    enabled: document.getElementById('robot-discord-enabled')?.checked === true,
                    auth: robotAuthPayload('discord', prevRobots),
                    bot_token: document.getElementById('robot-discord-bot-token')?.value.trim() || '',
                    allow_guild_messages: document.getElementById('robot-discord-allow-guild')?.checked === true
                },
                qq: {
                    enabled: document.getElementById('robot-qq-enabled')?.checked === true,
                    auth: robotAuthPayload('qq', prevRobots),
                    app_id: document.getElementById('robot-qq-app-id')?.value.trim() || '',
                    client_secret: document.getElementById('robot-qq-client-secret')?.value.trim() || '',
                    sandbox: document.getElementById('robot-qq-sandbox')?.checked === true
                }
            },
            tools: []
        };
        
        // Collect tool enable status
        // First save currentPage status to global map
        saveCurrentPageToolStates();
        
        // Fetch all tool list pages to get complete status
        // Note: regardless of whether in search state, always fetch all tool statuses to ensure a complete save
        try {
            const allToolsMap = new Map();
            let  page = 1;
            let hasMore = true;
            const pageSize = 100; // use a reasonable page size
            
            // Iterate all pages to fetch all tools (no search keyword — fetches every tool)
            while (hasMore) {
                const URL = `/api/config/tools? page=${ page}& page_size=${ pageSize}`;
                
                const  pageResponse = await apiFetch(URL);
                if (! pageResponse.ok) {
                    throw new Error('Failed to fetch tool list');
                }
                
                const  pageResult = await  pageResponse.json();
                
                // Add tools to the map
                // Prefer status from global map (user-modified); fall back to server-returned status
                 pageResult.tools.forEach(tool => {
                    const toolKey = getToolKey(tool);
                    const savedState = toolStateMap.get(toolKey);
                    allToolsMap.set(toolKey, {
                        name: tool.name,
                        enabled: savedState ? savedState.enabled : tool.enabled,
                        is_external: savedState ? savedState.is_external : (tool.is_external || false),
                        external_mcp: savedState ? savedState.external_mcp : (tool.external_mcp || '')
                    });
                });
                
                // Check whether there are more pages
                if (page >= pageResult.total_pages) {
                    hasMore = false;
                } else {
                     page++;
                }
            }
            
            // Add all tools to the configuration
            allToolsMap.forEach((tool, toolKey) => {
                config.tools.push({
                    name: tool.name,
                    enabled: tool.enabled,
                    is_external: tool.is_external,
                    external_mcp: tool.external_mcp
                });
            });
        } catch (error) {
            console.warn('Failed to fetch all tool list pages; using global status map only', error);
            // If fetch failed, use global status map
            toolStateMap.forEach((toolData, toolKey) => {
                // toolData.name stores the original tool name
                const toolName = toolData.name || toolKey.split('::').pop();
                config.tools.push({
                    name: toolName,
                    enabled: toolData.enabled,
                    is_external: toolData.is_external,
                    external_mcp: toolData.external_mcp
                });
            });
        }
        
        // Update configuration
        const updateResponse = await apiFetch('/api/config', {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(config)
        });
        
        if (!updateResponse.ok) {
            const error = await updateResponse.json();
            const fallback = (typeof window !== 'undefined' && typeof window.t === 'function')
                ? window.t('settings.apply.applyFailed')
                : 'Apply configurationfailed';
            throw new Error(error.error || fallback);
        }
        
        // Apply configuration
        const applyResponse = await apiFetch('/api/config/apply', {
            method: 'POST'
        });
        
        if (!applyResponse.ok) {
            const error = await applyResponse.json();
            const fallback = (typeof window !== 'undefined' && typeof window.t === 'function')
                ? window.t('settings.apply.applyFailed')
                : 'Apply configurationfailed';
            throw new Error(error.error || fallback);
        }
        
        const successMsg = (typeof window !== 'undefined' && typeof window.t === 'function')
            ? window.t('settings.apply.applySuccess')
            : 'Settings applied successfully!';
        alert(successMsg);
        try {
            const cfgResp = await apiFetch('/api/config');
            if (cfgResp.ok) {
                const fresh = await cfgResp.json();
                syncC2NavFromConfig(fresh);
            }
        } catch (e) {
            console.warn('refresh C2 nav after apply', e);
        }
        try {
            if (typeof initChatAgentModeFromConfig === 'function') {
                await initChatAgentModeFromConfig();
            }
        } catch (e) {
            console.warn('initChatAgentModeFromConfig after settings', e);
        }
        closeSettings();
    } catch (error) {
        console.error('Apply configurationfailed:', error);
        const baseMsg = (typeof window !== 'undefined' && typeof window.t === 'function')
            ? window.t('settings.apply.applyFailed')
            : 'Apply configurationfailed';
        alert(baseMsg + ': ' + error.message);
    }
}

function fillVisionConfigFromCurrent (v) {
    const en = document.getElementById('vision-enabled');
    if (en) en.checked = v.enabled === true;
    const prov = document.getElementById('vision-provider');
    if (prov) prov.value = (v.provider || '').trim();
    const setVal = (id, val) => {
        const el = document.getElementById(id);
        if (el) el.value = val != null && val !== '' ? String(val) : '';
    };
    setVal('vision-API-key', v.API_KEY || '');
    setVal('vision-base-URL', v.base_url || '');
    setVal('vision-model', v.model || '');
    setVal('vision-max-image-bytes', v.max_image_bytes || 5242880);
    setVal('vision-max-dimension', v.max_dimension || 2048);
    setVal('vision-jpeg-quality', v.jpeg_quality || 82);
    setVal('vision-max-payload-bytes', v.max_payload_bytes || 524288);
    setVal('vision-skip-preprocess-bytes', v.skip_preprocess_below_bytes != null ? v.skip_preprocess_below_bytes : 2097152);
    setVal('vision-timeout-seconds', v.timeout_seconds || 60);
    const det = document.getElementById('vision-detail');
    if (det) {
        const d = (v.detail || 'low').toString().toLowerCase();
        det.value = ['low', 'auto', 'high'].includes(d) ? d : 'low';
    }
    syncVisionFormEnabled();
}

function collectVisionConfigFromForm() {
    const parseIntOr = (id, fallback) => {
        const n = parseInt(document.getElementById(id)?.value, 10);
        return Number.isNaN(n) ? fallback : n;
    };
    const provider = document.getElementById('vision-provider')?.value.trim() || '';
    return {
        enabled: document.getElementById('vision-enabled')?.checked === true,
        API_KEY: document.getElementById('vision-API-key')?.value.trim() || '',
        base_url: document.getElementById('vision-base-URL')?.value.trim() || '',
        model: document.getElementById('vision-model')?.value.trim() || '',
        provider: provider,
        timeout_seconds: parseIntOr('vision-timeout-seconds', 60),
        max_image_bytes: parseIntOr('vision-max-image-bytes', 5242880),
        max_dimension: parseIntOr('vision-max-dimension', 2048),
        jpeg_quality: parseIntOr('vision-jpeg-quality', 82),
        max_payload_bytes: parseIntOr('vision-max-payload-bytes', 524288),
        skip_preprocess_below_bytes: parseIntOr('vision-skip-preprocess-bytes', 2097152),
        detail: document.getElementById('vision-detail')?.value || 'low'
    };
}

function syncVisionFormEnabled() {
    const enabled = document.getElementById('vision-enabled')?.checked === true;
    const panel = document.getElementById('vision-fields-panel');
    if (panel) {
        panel.style.opacity = enabled ? '1' : '0.55';
        panel.querySelectorAll('INPUT, SELECT, textarea, a').forEach(el => {
            if (el.id === 'test-vision-btn' || el.id === 'fetch-vision-models-btn' || el.id === 'vision-model-SELECT') return;
            el.disabled = !enabled;
        });
        syncModelListFetchButtons();
    }
}

const modelPickSelectMap = {};
let modelPickSelectDocListener = false;

function modelPickT(key) {
    return typeof window.t === 'function' ? window.t(key) : key;
}

function closeAllModelPickDropdowns() {
    Object.keys(modelPickSelectMap).forEach(function (id) {
        modelPickSelectMap[id].wrapper.classList.remove('open');
    });
}

function syncModelPickDropdown(selectId) {
    const reg = modelPickSelectMap[selectId];
    if (!reg) return;
    const { SELECT, dropdown, trigger, wrapper, menuList, countBadge } = reg;
    const placeholder = modelPickT('settingsBasic.modelsListSelectPlaceholder');

    menuList.innerHTML = '';
    let optionCount = 0;
    Array.prototype.forEach.call(SELECT.options, function (opt) {
        if (!opt.value) return;
        optionCount += 1;
        const item = document.createElement('div');
        item.className = 'model-pick-option';
        item.setAttribute('role', 'option');
        item.setAttribute('data-value', opt.value);
        if (opt.value === SELECT.value) {
            item.classList.add('is-selected');
            item.setAttribute('aria-selected', 'true');
        }
        const check = document.createElement('span');
        check.className = 'model-pick-option-check';
        check.setAttribute('aria-hidden', 'true');
        check.textContent = '✓';
        const label = document.createElement('span');
        label.className = 'model-pick-option-label';
        label.textContent = opt.textContent;
        item.appendChild(check);
        item.appendChild(label);
        menuList.appendChild(item);
    });

    const selectedOpt = SELECT.selectedIndex >= 0 ? SELECT.options[SELECT.selectedIndex] : null;
    const labelEl = trigger.querySelector('.model-pick-trigger-label');
    if (labelEl) {
        labelEl.textContent = (selectedOpt && selectedOpt.value) ? selectedOpt.textContent : placeholder;
    }
    if (countBadge) {
        countBadge.textContent = String(optionCount);
        countBadge.style.display = optionCount > 0 ? '' : 'none';
    }
    const header = wrapper.querySelector('.model-pick-menu-header');
    if (header) {
        header.textContent = optionCount > 0
            ? placeholder + ' · ' + optionCount
            : placeholder;
    }

    trigger.disabled = !!SELECT.disabled;
    wrapper.classList.toggle('is-disabled', !!SELECT.disabled);
    wrapper.style.display = optionCount > 0 ? '' : 'none';
    SELECT.style.display = 'none';
}

function enhanceModelPickSelect(selectId) {
    const SELECT = document.getElementById(selectId);
    if (!SELECT) return;
    if (SELECT.dataset.modelPickEnhanced === '1') {
        syncModelPickDropdown(selectId);
        return;
    }
    SELECT.dataset.modelPickEnhanced = '1';
    SELECT.classList.add('model-pick-native');
    SELECT.tabIndex = -1;
    SELECT.setAttribute('aria-hidden', 'true');

    const wrapper = document.createElement('div');
    wrapper.className = 'model-pick-dropdown';
    wrapper.style.display = 'none';

    const trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'model-pick-trigger';
    trigger.setAttribute('aria-haspopup', 'listbox');

    const labelSpan = document.createElement('span');
    labelSpan.className = 'model-pick-trigger-label';
    labelSpan.textContent = modelPickT('settingsBasic.modelsListSelectPlaceholder');

    const meta = document.createElement('span');
    meta.className = 'model-pick-trigger-meta';

    const countBadge = document.createElement('span');
    countBadge.className = 'model-pick-count';
    countBadge.style.display = 'none';

    const caret = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    caret.setAttribute('class', 'model-pick-caret');
    caret.setAttribute('viewBox', '0 0 16 16');
    caret.setAttribute('aria-hidden', 'true');
    caret.innerHTML = '<path fill="currentColor" d="M4.47 6.47a.75.75 0 0 1 1.06 0L8 8.94l2.47-2.47a.75.75 0 1 1 1.06 1.06l-3 3a.75.75 0 0 1-1.06 0l-3-3a.75.75 0 0 1 0-1.06z"/>';

    meta.appendChild(countBadge);
    meta.appendChild(caret);
    trigger.appendChild(labelSpan);
    trigger.appendChild(meta);

    const menu = document.createElement('div');
    menu.className = 'model-pick-menu';

    const header = document.createElement('div');
    header.className = 'model-pick-menu-header';
    menu.appendChild(header);

    const menuList = document.createElement('div');
    menuList.className = 'model-pick-menu-list';
    menuList.setAttribute('role', 'listbox');
    menu.appendChild(menuList);

    const parent = SELECT.parentNode;
    const fetchLink = parent.querySelector('.model-pick-fetch-link');
    if (fetchLink) {
        parent.insertBefore(wrapper, fetchLink);
    } else {
        parent.appendChild(wrapper);
    }
    wrapper.appendChild(trigger);
    wrapper.appendChild(menu);
    wrapper.appendChild(SELECT);

    modelPickSelectMap[selectId] = {
        wrapper,
        trigger,
        menu,
        menuList,
        countBadge,
        SELECT
    };

    if (!modelPickSelectDocListener) {
        document.addEventListener('click', closeAllModelPickDropdowns);
        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') closeAllModelPickDropdowns();
        });
        modelPickSelectDocListener = true;
    }

    trigger.addEventListener('click', function (e) {
        e.stopPropagation();
        if (SELECT.disabled) return;
        const open = wrapper.classList.contains('open');
        closeAllModelPickDropdowns();
        if (!open) wrapper.classList.add('open');
    });

    menuList.addEventListener('click', function (e) {
        const opt = e.target.closest('.model-pick-option');
        if (!opt) return;
        const val = opt.getAttribute('data-value');
        if (val === null || val === '') return;
        if (SELECT.value !== val) {
            SELECT.value = val;
            SELECT.dispatchEvent(new Event('change', { bubbles: true }));
        }
        wrapper.classList.remove('open');
        syncModelPickDropdown(selectId);
    });

    syncModelPickDropdown(selectId);
}

function normalizeAIChannelId(name) {
    const raw = String(name || '').trim().toLowerCase().replace(/_/g, '-');
    const id = raw.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
    return id || 'default';
}

function aiChannelBaseURLHost(baseUrl) {
    const raw = String(baseUrl || '').trim();
    if (!raw) return '';
    try {
        return new URL(raw).hostname.toLowerCase().replace(/^www\./, '');
    } catch (e) {
        try {
            return new URL(`https://${raw.replace(/^\/+/, '')}`).hostname.toLowerCase().replace(/^www\./, '');
        } catch (_) {
            return '';
        }
    }
}

function isOfficialDeepSeekBaseURL(baseUrl) {
    return aiChannelBaseURLHost(baseUrl) === 'api.deepseek.com';
}

function normalizeAIChannelProviderProfile(channel) {
    if (!channel || typeof channel !== 'object') return channel;
    if (isOfficialDeepSeekBaseURL(channel.base_url)) {
        channel.reasoning = {
            ...(channel.reasoning || {}),
            profile: 'deepseek'
        };
    }
    return channel;
}

function normalizeAIConfigProviderProfiles(AI) {
    if (!AI || typeof AI !== 'object' || !AI.channels || typeof AI.channels !== 'object') return AI;
    Object.keys(AI.channels).forEach((id) => {
        AI.channels[id] = normalizeAIChannelProviderProfile(AI.channels[id] || {});
    });
    return AI;
}

function escapeAIChannelHtml(value) {
    return String(value == null ? '' : value)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

function ensureAIConfigShape(cfg) {
    const AI = cfg && cfg.ai && typeof cfg.ai === 'object' ? cfg.ai : {};
    const channels = AI.channels && typeof AI.channels === 'object' ? { ...ai.channels } : {};
    let def = normalizeAIChannelId(AI.default_channel || '');
    if (!channels[def]) {
        const oa = (cfg && cfg.OpenAI) ? cfg.OpenAI : {};
        channels[def] = {
            name: def === 'default' ? 'default' : def,
            provider: oa.provider || 'OpenAI',
            API_KEY: oa.API_KEY || '',
            base_url: oa.base_url || '',
            model: oa.model || '',
            max_total_tokens: oa.max_total_tokens || 120000,
            max_completion_tokens: oa.max_completion_tokens || 0,
            reasoning: oa.reasoning || {}
        };
    }
    return normalizeAIConfigProviderProfiles({ default_channel: def, channels });
}

function readAIChannelFromMainForm(id) {
    const prev = currentConfig?.ai?.channels?.[id] || {};
    const maxCompletionTokens = parseInt(document.getElementById('openai-max-completion-tokens')?.value, 10) || 32768;
    return normalizeAIChannelProviderProfile({
        ...prev,
        name: (document.getElementById('AI-channel-name')?.value || '').trim() || prev.name || id,
        provider: document.getElementById('openai-provider')?.value || 'OpenAI',
        API_KEY: document.getElementById('openai-API-key')?.value.trim() || '',
        base_url: document.getElementById('openai-base-URL')?.value.trim() || '',
        model: document.getElementById('openai-model')?.value.trim() || '',
        max_total_tokens: parseInt(document.getElementById('openai-max-total-tokens')?.value, 10) || 120000,
        max_completion_tokens: maxCompletionTokens,
        reasoning: {
            ...(prev.reasoning || {}),
            mode: document.getElementById('openai-reasoning-mode')?.value || 'auto',
            effort: (document.getElementById('openai-reasoning-effort')?.value || '').trim(),
            profile: document.getElementById('openai-reasoning-profile')?.value || 'auto',
            allow_client_reasoning: document.getElementById('openai-reasoning-allow-client')?.checked !== false
        }
    });
}

function writeAIChannelToMainForm(id) {
    const AI = ensureAIConfigShape(currentConfig || {});
    const ch = AI.channels[id] || AI.channels[AI.default_channel] || {};
    selectedAIChannelId = id || AI.default_channel;
    const nameEl = document.getElementById('AI-channel-name');
    if (nameEl) nameEl.value = ch.name || selectedAIChannelId;
    const providerEl = document.getElementById('openai-provider');
    if (providerEl) {
        const provider = (ch.provider === 'OpenAI' || !ch.provider) ? 'openai_compatible' : ch.provider;
        providerEl.value = provider;
        syncSettingsCustomSelect(providerEl);
    }
    const keyEl = document.getElementById('openai-API-key');
    if (keyEl) keyEl.value = ch.API_KEY || '';
    const baseEl = document.getElementById('openai-base-URL');
    if (baseEl) baseEl.value = ch.base_url || '';
    const modelEl = document.getElementById('openai-model');
    if (modelEl) modelEl.value = ch.model || '';
    const maxTokensEl = document.getElementById('openai-max-total-tokens');
    if (maxTokensEl) maxTokensEl.value = ch.max_total_tokens || 120000;
    const maxCompletionTokensEl = document.getElementById('openai-max-completion-tokens');
    if (maxCompletionTokensEl) maxCompletionTokensEl.value = ch.max_completion_tokens || 32768;
    const r = ch.reasoning || {};
    const modeEl = document.getElementById('openai-reasoning-mode');
    if (modeEl) {
        modeEl.value = ['auto', 'on', 'off'].includes(String(r.mode || '').toLowerCase()) ? String(r.mode).toLowerCase() : 'auto';
        syncSettingsCustomSelect(modeEl);
    }
    const effEl = document.getElementById('openai-reasoning-effort');
    if (effEl) {
        effEl.value = ['', 'low', 'medium', 'high', 'max', 'xhigh'].includes(String(r.effort || '').toLowerCase()) ? String(r.effort || '').toLowerCase() : '';
        syncSettingsCustomSelect(effEl);
    }
    const profileEl = document.getElementById('openai-reasoning-profile');
    if (profileEl) {
        profileEl.value = ['auto', 'deepseek', 'deepseek_compat', 'openai_compat', 'output_config_effort'].includes(String(r.profile || '').toLowerCase()) ? String(r.profile || '').toLowerCase() : 'auto';
        syncSettingsCustomSelect(profileEl);
    }
    const allowEl = document.getElementById('openai-reasoning-allow-client');
    if (allowEl) allowEl.checked = r.allow_client_reasoning !== false;
    syncModelListFetchButtons();
    syncAIChannelEditorPreview();
    syncConnectionTestResultForSelectedAIChannel();
}

function displayAIChannelName(id, ch) {
    const name = String(ch?.name || '').trim();
    if ((name === 'New Channel' || name === 'New Channel') && !String(ch?.model || '').trim()) {
        return settingsT('settingsBasic.aiChannelUntitled', name || id);
    }
    return name || id;
}

function aiChannelSelectLabel(id, ch) {
    const marker = id === currentConfig?.ai?.default_channel ? ' *' : '';
    return `${displayAIChannelName(id, ch)}${marker} · ${ch?.model || '-'}`;
}

function aiChannelOptionProbeMeta(id) {
    const probe = aiChannelProbeResults[id];
    if (!probe) return null;
    const status = probe.status || '';
    if (!['testing', 'ready', 'failed'].includes(status)) return null;
    return {
        status,
        message: probe.message || (status === 'ready'
            ? settingsT('settingsBasic.aiChannelReady', 'Available')
            : status === 'testing'
                ? settingsT('settingsBasic.testing', 'Testing...')
                : settingsT('settingsBasic.testFailed', 'Connection failed'))
    };
}

function updateAIChannelSelectOption(id) {
    const SELECT = document.getElementById('AI-channel-SELECT');
    if (!SELECT || !currentConfig?.ai?.channels) return;
    const channelId = normalizeAIChannelId(id || selectedAIChannelId || currentConfig.ai.default_channel || 'default');
    const ch = currentConfig.ai.channels[channelId];
    if (!ch) return;
    const opt = Array.from(SELECT.options).find((option) => option.value === channelId);
    if (opt) {
        opt.textContent = aiChannelSelectLabel(channelId, ch);
        const probeMeta = aiChannelOptionProbeMeta(channelId);
        if (probeMeta) {
            opt.dataset.probeStatus = probeMeta.status;
            opt.dataset.probeMessage = probeMeta.message;
        } else {
            delete opt.dataset.probeStatus;
            delete opt.dataset.probeMessage;
        }
        if (channelId === selectedAIChannelId) {
            SELECT.value = channelId;
            SELECT.selectedIndex = opt.index;
        }
    }
    if (typeof syncSettingsCustomSelect === 'function') {
        syncSettingsCustomSelect(SELECT);
    }
}

function syncSelectedAIChannelUI() {
    updateAIChannelSelectOption(selectedAIChannelId);
    updateAIChannelEditorChrome(selectedAIChannelId);
    renderAIChannelList();
    syncConnectionTestResultForSelectedAIChannel();
}

function syncAIChannelEditorPreview() {
    if (!currentConfig?.ai?.channels || !selectedAIChannelId || !currentConfig.ai.channels[selectedAIChannelId]) return;
    const id = normalizeAIChannelId(selectedAIChannelId);
    const prev = currentConfig.ai.channels[id] || {};
    const next = readAIChannelFromMainForm(id);
    const connectionChanged = ['provider', 'base_url', 'API_KEY', 'model'].some((key) => String(prev[key] || '') !== String(next[key] || ''));
    if (connectionChanged) {
        delete aiChannelProbeResults[id];
    }
    currentConfig.ai.channels[id] = next;
    syncSelectedAIChannelUI();
}

function bindAIChannelEditorPreviewSync() {
    const ids = [
        'AI-channel-name',
        'openai-provider',
        'openai-API-key',
        'openai-base-URL',
        'openai-model'
    ];
    ids.forEach((fieldId) => {
        const el = document.getElementById(fieldId);
        if (!el || el.dataset.aiChannelPreviewBound === '1') return;
        el.dataset.aiChannelPreviewBound = '1';
        const eventName = el.tagName === 'SELECT' ? 'change' : 'INPUT';
        el.addEventListener(eventName, syncAIChannelEditorPreview);
    });
}

function renderAIChannelSelect() {
    if (!currentConfig) return;
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    const SELECT = document.getElementById('AI-channel-SELECT');
    if (!SELECT) return;
    SELECT.innerHTML = '';
    const ids = Object.keys(currentConfig.ai.channels || {}).sort();
    ids.forEach((id) => {
        const ch = currentConfig.ai.channels[id] || {};
        const opt = document.createElement('option');
        opt.value = id;
        opt.textContent = aiChannelSelectLabel(id, ch);
        const probeMeta = aiChannelOptionProbeMeta(id);
        if (probeMeta) {
            opt.dataset.probeStatus = probeMeta.status;
            opt.dataset.probeMessage = probeMeta.message;
        }
        SELECT.appendChild(opt);
    });
    selectedAIChannelId = selectedAIChannelId && currentConfig.ai.channels[selectedAIChannelId]
        ? selectedAIChannelId
        : currentConfig.ai.default_channel;
    SELECT.value = selectedAIChannelId;
    updateAIChannelSelectOption(selectedAIChannelId);
    if (typeof syncSettingsCustomSelect === 'function') {
        syncSettingsCustomSelect(SELECT);
    }
    updateAIChannelEditorChrome(selectedAIChannelId);
    renderAIChannelList(ids);
    const countLabel = typeof window.t === 'function'
        ? window.t('settingsBasic.aiChannelCount').replace('{count}', String(ids.length))
        : `Saved ${ids.length} channel(s)`;
    showAIChannelSaveHint(countLabel, true);
}

function channelHostLabel(baseUrl) {
    const raw = String(baseUrl || '').trim();
    if (!raw) return '-';
    try {
        return new URL(raw).host || raw;
    } catch (e) {
        return raw.replace(/^https?:\/\//, '').split('/')[0] || raw;
    }
}

function renderAIChannelList(ids) {
    const list = document.getElementById('AI-channel-list');
    if (!list || !currentConfig?.ai?.channels) return;
    list.innerHTML = '';
    (ids || Object.keys(currentConfig.ai.channels).sort()).forEach((id) => {
        const ch = currentConfig.ai.channels[id] || {};
        const isDefault = id === currentConfig.ai.default_channel;
        const isComplete = !validateSelectedAIChannelPayload(ch);
        const probe = aiChannelProbeResults[id] || null;
        const item = document.createElement('div');
        item.className = 'AI-channel-list-item' + (id === selectedAIChannelId ? ' active' : '') + (selectedAIChannelBulkIds.has(id) ? ' checked' : '');
        item.setAttribute('role', 'button');
        item.setAttribute('tabIndex', '0');
        item.setAttribute('aria-current', id === selectedAIChannelId ? 'true' : 'false');
        item.onclick = () => selectAIChannelForEditing(id);
        item.onkeydown = (event) => {
            if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                selectAIChannelForEditing(id);
            }
        };
        const checkbox = document.createElement('INPUT');
        checkbox.type = 'checkbox';
        checkbox.className = 'AI-channel-bulk-check';
        checkbox.checked = selectedAIChannelBulkIds.has(id);
        checkbox.setAttribute('aria-label', settingsT('settingsBasic.aiChannelSelectAria', 'Select {name}').replace('{name}', displayAIChannelName(id, ch)));
        checkbox.onclick = (event) => {
            event.stopPropagation();
            if (checkbox.checked) {
                selectedAIChannelBulkIds.add(id);
            } else {
                selectedAIChannelBulkIds.delete(id);
            }
            item.classList.toggle('checked', checkbox.checked);
        };
        const displayName = displayAIChannelName(id, ch);
        const defaultBadge = isDefault ? `<span class="AI-channel-badge">${escapeAIChannelHtml(settingsT('settingsBasic.aiChannelDefaultBadge', 'default'))}</span>` : '';
        let statusText = isComplete
            ? settingsT('settingsBasic.aiChannelComplete', 'Configuration complete')
            : settingsT('settingsBasic.aiChannelDraft', 'Incomplete');
        let statusClass = isComplete ? 'complete' : 'draft';
        if (probe) {
            statusText = probe.message || statusText;
            statusClass = probe.status || statusClass;
        }
        const body = document.createElement('div');
        body.className = 'AI-channel-card-body';
        body.innerHTML = `
            <div class="AI-channel-list-main">
                <span class="AI-channel-status-dot ${statusClass}" aria-hidden="true"></span>
                <strong title="${escapeAIChannelHtml(displayName)}">${escapeAIChannelHtml(displayName)}</strong>
                ${defaultBadge}
            </div>
            <div class="AI-channel-list-meta" title="${escapeAIChannelHtml(ch.model || '-')} · ${escapeAIChannelHtml(channelHostLabel(ch.base_url))}">${escapeAIChannelHtml(ch.model || '-')} · ${escapeAIChannelHtml(channelHostLabel(ch.base_url))}</div>
            <div class="AI-channel-list-foot">
                <span class="AI-channel-status-label ${statusClass}" title="${escapeAIChannelHtml(statusText)}">${escapeAIChannelHtml(statusText)}</span>
                <span title="${escapeAIChannelHtml(id)}">${escapeAIChannelHtml(id)}</span>
            </div>
        `;
        item.appendChild(checkbox);
        item.appendChild(body);
        list.appendChild(item);
    });
}

function showAIChannelSaveHint(message, ok) {
    const el = document.getElementById('AI-channel-save-hint');
    if (!el) return;
    el.textContent = message;
    el.classList.toggle('is-error', ok === false);
    el.classList.toggle('is-success', ok !== false);
}

function updateAIChannelEditorChrome(id) {
    const AI = ensureAIConfigShape(currentConfig || {});
    const channelId = normalizeAIChannelId(id || AI.default_channel || 'default');
    const ch = AI.channels[channelId] || {};
    const isDefault = channelId === AI.default_channel;
    const isComplete = !validateSelectedAIChannelPayload(ch);
    const probe = aiChannelProbeResults[channelId] || null;
    const title = document.getElementById('AI-channel-editor-title');
    const meta = document.getElementById('AI-channel-editor-meta');
    if (title) {
        title.textContent = settingsT('settingsBasic.aiChannelFormContextHint', 'The channel configuration will update after the form is saved.');
    }
    if (meta) {
        const provider = ch.provider === 'claude' ? 'Claude' : settingsT('settingsBasic.aiChannelOpenAICompat', 'OpenAI compatible');
        const statusText = probe?.message || (isComplete
            ? settingsT('settingsBasic.aiChannelComplete', 'Configuration complete')
            : settingsT('settingsBasic.aiChannelDraft', 'Incomplete'));
        const statusClass = probe?.status || (isComplete ? 'complete' : 'draft');
        const chips = [
            {
                label: isDefault
                    ? settingsT('settingsBasic.aiChannelDefaultMeta', 'default channel')
                    : settingsT('settingsBasic.aiChannelCustomMeta', 'Custom channel'),
                className: isDefault ? 'default' : ''
            },
            { label: provider },
            { label: ch.model || settingsT('settingsBasic.aiChannelModelMissing', 'Model not specified') },
            { label: channelHostLabel(ch.base_url) },
            { label: statusText, className: statusClass }
        ].filter((chip) => chip.label);
        meta.innerHTML = chips.map((chip) => {
            const className = chip.className ? ` ${escapeAIChannelHtml(chip.className)}` : '';
            return `<span class="AI-channel-editor-chip${className}" title="${escapeAIChannelHtml(chip.label)}">${escapeAIChannelHtml(chip.label)}</span>`;
        }).join('');
    }
}

function validateSelectedAIChannelPayload(ch) {
    const missing = [];
    if (!String(ch.base_url || '').trim()) missing.push('Base URL');
    if (!String(ch.API_KEY || '').trim()) missing.push('API Key');
    if (!String(ch.model || '').trim()) missing.push('Model');
    if (missing.length) {
        return missing.join(', ');
    }
    return '';
}

function resolveSavedAIChannelId(AI, preferredId, preferredPayload) {
    const channels = AI?.channels || {};
    const normalizedPreferred = normalizeAIChannelId(preferredId || '');
    if (normalizedPreferred && channels[normalizedPreferred]) return normalizedPreferred;

    const payload = preferredPayload || {};
    const targetName = String(payload.name || '').trim();
    const targetModel = String(payload.model || '').trim();
    const targetBaseUrl = String(payload.base_url || '').trim();
    const targetProvider = String(payload.provider || '').trim();
    const ids = Object.keys(channels).sort();
    const matched = ids.find((id) => {
        const ch = channels[id] || {};
        return String(ch.name || '').trim() === targetName
            && String(ch.model || '').trim() === targetModel
            && String(ch.base_url || '').trim() === targetBaseUrl
            && String(ch.provider || '').trim() === targetProvider;
    });
    return matched || AI?.default_channel || ids[0] || normalizedPreferred || 'default';
}

async function refreshAIChannelsFromServer(preferredId, preferredPayload) {
    const response = await apiFetch('/api/config');
    if (!response.ok) return false;
    currentConfig = await response.json();
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    selectedAIChannelId = resolveSavedAIChannelId(currentConfig.ai, preferredId, preferredPayload);
    renderAIChannelSelect();
    writeAIChannelToMainForm(selectedAIChannelId);
    if (typeof populateChatAIChannelSelect === 'function') {
        populateChatAIChannelSelect(currentConfig.ai);
    }
    return true;
}

async function persistAIChannelsToServer(successMessage, options = {}) {
    if (typeof requirePermission === 'function' && !requirePermission('config:write')) return false;
    if (!currentConfig) currentConfig = {};
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    const id = normalizeAIChannelId(selectedAIChannelId || currentConfig.ai.default_channel || 'default');
    const channelPayload = readAIChannelFromMainForm(id);
    currentConfig.ai.channels[id] = channelPayload;
    selectedAIChannelId = id;
    const missing = validateSelectedAIChannelPayload(channelPayload);
    if (missing) {
        showAIChannelSaveHint(`Please fill in: ${missing}`, false);
        alert(`Please fill in: ${missing}`);
        return false;
    }
    renderAIChannelSelect();
    showAIChannelSaveHint(settingsT('settingsBasic.aiChannelSaving', 'Saving channel...'), true);
    try {
        const shouldMergelatest = options.mergeLatest !== false;
        const latestResponse = shouldMergelatest ? await apiFetch('/api/config') : null;
        if (latestResponse && latestResponse.ok) {
            const latest = await latestResponse.json();
            const latestAI = ensureAIConfigShape(latest || {});
            currentConfig.ai.channels = {
                ...(latestAI.channels || {}),
                ...(currentConfig.ai.channels || {}),
                [id]: channelPayload
            };
            if (!currentConfig.ai.default_channel) {
                currentConfig.ai.default_channel = latestAI.default_channel || id;
            }
        }
        currentConfig.ai = normalizeAIConfigProviderProfiles(currentConfig.ai);
        const updateResponse = await apiFetch('/api/config', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ai: currentConfig.ai })
        });
        if (!updateResponse.ok) {
            const error = await updateResponse.json().catch(() => ({}));
            throw new Error(error.error || 'Failed to save channel');
        }
        const applyResponse = await apiFetch('/api/config/apply', { method: 'POST' });
        if (!applyResponse.ok) {
            const error = await applyResponse.json().catch(() => ({}));
            throw new Error(error.error || 'Failed to apply channel');
        }
        await refreshAIChannelsFromServer(id, channelPayload);
        showAIChannelSaveHint(successMessage || 'Channel saved', true);
        return true;
    } catch (error) {
        showAIChannelSaveHint(error.message || 'Failed to save channel', false);
        alert(error.message || 'Failed to save channel');
        return false;
    }
}

async function persistAIConfigOnlyToServer(successMessage) {
    if (typeof requirePermission === 'function' && !requirePermission('config:write')) return false;
    if (!currentConfig) return false;
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    currentConfig.ai = normalizeAIConfigProviderProfiles(currentConfig.ai);
    showAIChannelSaveHint(settingsT('settingsBasic.aiChannelSaving', 'Saving channel...'), true);
    try {
        const updateResponse = await apiFetch('/api/config', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ai: currentConfig.ai })
        });
        if (!updateResponse.ok) {
            const error = await updateResponse.json().catch(() => ({}));
            throw new Error(error.error || 'Failed to save channel');
        }
        const applyResponse = await apiFetch('/api/config/apply', { method: 'POST' });
        if (!applyResponse.ok) {
            const error = await applyResponse.json().catch(() => ({}));
            throw new Error(error.error || 'Failed to apply channel');
        }
        await refreshAIChannelsFromServer(selectedAIChannelId);
        showAIChannelSaveHint(successMessage || 'Channel saved', true);
        return true;
    } catch (error) {
        showAIChannelSaveHint(error.message || 'Failed to save channel', false);
        alert(error.message || 'Failed to save channel');
        return false;
    }
}

async function saveSelectedAIChannel() {
    await persistAIChannelsToServer(typeof window.t === 'function' ? window.t('settingsBasic.aiChannelSaved') : 'Channel saved');
}

async function setSelectedAIChannelDefault() {
    if (!currentConfig) return;
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    const id = normalizeAIChannelId(selectedAIChannelId || currentConfig.ai.default_channel || 'default');
    currentConfig.ai.channels[id] = readAIChannelFromMainForm(id);
    currentConfig.ai.default_channel = id;
    await persistAIChannelsToServer(typeof window.t === 'function' ? window.t('settingsBasic.aiChannelDefaultSaved') : 'Set as default channel');
}

function selectAIChannelForEditing(id) {
    if (!currentConfig) return;
    if (selectedAIChannelId && currentConfig.ai?.channels?.[selectedAIChannelId]) {
        currentConfig.ai.channels[selectedAIChannelId] = readAIChannelFromMainForm(selectedAIChannelId);
    }
    const next = normalizeAIChannelId(id || currentConfig.ai?.default_channel || 'default');
    selectedAIChannelId = next;
    writeAIChannelToMainForm(next);
    renderAIChannelSelect();
}

function uniqueAIChannelId(base) {
    const AI = ensureAIConfigShape(currentConfig || {});
    let id = normalizeAIChannelId(base);
    if (!AI.channels[id]) return id;
    let i = 2;
    while (AI.channels[`${id}-${i}`]) i++;
    return `${id}-${i}`;
}

function createAIChannelFromForm() {
    if (!currentConfig) currentConfig = {};
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    if (selectedAIChannelId && currentConfig.ai.channels[selectedAIChannelId]) {
        currentConfig.ai.channels[selectedAIChannelId] = readAIChannelFromMainForm(selectedAIChannelId);
    }
    const baseName = (typeof window.t === 'function' ? window.t('settingsBasic.aiChannelUntitled') : 'New Channel');
    const id = uniqueAIChannelId(baseName);
    currentConfig.ai.channels[id] = {
        name: baseName,
        provider: 'openai_compatible',
        API_KEY: '',
        base_url: '',
        model: '',
        max_total_tokens: 120000,
        max_completion_tokens: 32768,
        reasoning: { mode: 'auto', effort: '', profile: 'auto', allow_client_reasoning: true }
    };
    selectedAIChannelId = id;
    renderAIChannelSelect();
    writeAIChannelToMainForm(id);
    showAIChannelSaveHint(settingsT('settingsBasic.aiChannelNewUnsaved', 'New channel has not been saved yet. Fill in the details and click "Save changes".'), true);
}

function copyAIChannelFromForm() {
    if (!currentConfig) return;
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    const source = readAIChannelFromMainForm(selectedAIChannelId || currentConfig.ai.default_channel);
    const id = uniqueAIChannelId((source.name || selectedAIChannelId || 'channel') + '-copy');
    currentConfig.ai.channels[id] = { ...source, name: (source.name || id) + ' copy' };
    selectedAIChannelId = id;
    renderAIChannelSelect();
    writeAIChannelToMainForm(id);
    showAIChannelSaveHint(settingsT('settingsBasic.aiChannelCopyUnsaved', 'Copied channel has not been saved yet. Confirm and click "Save changes".'), true);
}

async function deleteSelectedAIChannel() {
    if (!currentConfig) return;
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    const ids = Object.keys(currentConfig.ai.channels || {});
    if (ids.length <= 1) {
        alert('At least one AI channel must be kept');
        return;
    }
    const id = selectedAIChannelId || currentConfig.ai.default_channel;
    const ch = currentConfig.ai.channels[id] || {};
    const name = ch.name || id;
    const msg = typeof window.t === 'function'
        ? window.t('settingsBasic.aiChannelDeleteConfirm').replace('{name}', name)
        : `Are you sure you want to delete AI channel "${name}"?`;
    if (!confirm(msg)) {
        return;
    }
    delete currentConfig.ai.channels[id];
    delete aiChannelProbeResults[id];
    selectedAIChannelBulkIds.delete(id);

    const remainingIds = Object.keys(currentConfig.ai.channels || {}).sort();
    if (!currentConfig.ai.channels[currentConfig.ai.default_channel]) {
        currentConfig.ai.default_channel = remainingIds[0];
    }
    selectedAIChannelId = currentConfig.ai.default_channel || remainingIds[0];
    renderAIChannelSelect();
    writeAIChannelToMainForm(selectedAIChannelId);
    const saved = await persistAIConfigOnlyToServer(settingsT('settingsBasic.aiChannelDeleted', 'Channel deleted'));
    if (saved) {
        renderAIChannelSelect();
        writeAIChannelToMainForm(selectedAIChannelId);
    }
}

function selectedOrAllAIChannelIdsForProbe() {
    if (!currentConfig) return [];
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    if (selectedAIChannelId && currentConfig.ai.channels[selectedAIChannelId]) {
        currentConfig.ai.channels[selectedAIChannelId] = readAIChannelFromMainForm(selectedAIChannelId);
    }
    const checked = Array.from(selectedAIChannelBulkIds).filter((id) => currentConfig.ai.channels[id]);
    const ids = checked.length ? checked : Object.keys(currentConfig.ai.channels || {}).sort();
    return ids.filter((id) => !validateSelectedAIChannelPayload(currentConfig.ai.channels[id] || {}));
}

async function probeSelectedAIChannels() {
    if (typeof requirePermission === 'function' && !requirePermission('config:write')) return;
    const ids = selectedOrAllAIChannelIdsForProbe();
    if (!ids.length) {
        alert(settingsT('settingsBasic.aiChannelProbeNoComplete', 'No complete channels to test. Please fill in Base URL, API Key, and Model first'));
        return;
    }
    showAIChannelSaveHint(settingsT('settingsBasic.aiChannelProbing', 'Testing {count} channel(s)...').replace('{count}', String(ids.length)), true);
    ids.forEach((id) => {
        aiChannelProbeResults[id] = { status: 'testing', message: settingsT('settingsBasic.testing', 'Testing...') };
        updateAIChannelSelectOption(id);
    });
    renderAIChannelList();
    let okCount = 0;
    let nextIndex = 0;
    async function probeNextAIChannel() {
        const id = ids[nextIndex++];
        if (!id) return;
        const ch = currentConfig.ai.channels[id] || {};
        try {
            const response = await apiFetch('/api/config/test-OpenAI', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    provider: ch.provider || 'openai_compatible',
                    channel_id: id,
                    base_url: ch.base_url || '',
                    API_KEY: ch.API_KEY || '',
                    model: ch.model || ''
                })
            });
            const result = await response.json().catch(() => ({}));
            if (response.ok && result.success) {
                okCount += 1;
                const latency = result.latency_ms ? ` ${result.latency_ms}ms` : '';
                aiChannelProbeResults[id] = { status: 'ready', message: settingsT('settingsBasic.aiChannelReadyWithLatency', 'Available{latency}').replace('{latency}', latency) };
            } else {
                aiChannelProbeResults[id] = { status: 'failed', message: formatConnectionTestError(result.error || settingsT('settingsBasic.testFailed', 'Connection failed')).message };
            }
        } catch (error) {
            aiChannelProbeResults[id] = { status: 'failed', message: formatConnectionTestError(error.message || settingsT('settingsBasic.testError', 'Test error')).message };
        }
        updateAIChannelSelectOption(id);
        renderAIChannelList();
    }
    const workers = Array.from({ length: Math.min(AI_CHANNEL_PROBE_CONCURRENCY, ids.length) }, async function () {
        while (nextIndex < ids.length) {
            await probeNextAIChannel();
        }
    });
    await Promise.all(workers);
    showAIChannelSaveHint(settingsT('settingsBasic.aiChannelProbeDone', 'Test complete: {ok}/{total} available').replace('{ok}', String(okCount)).replace('{total}', String(ids.length)), okCount === ids.length);
}

async function deleteCheckedAIChannels() {
    if (typeof requirePermission === 'function' && !requirePermission('config:write')) return;
    if (!currentConfig) return;
    currentConfig.ai = ensureAIConfigShape(currentConfig);
    const ids = Array.from(selectedAIChannelBulkIds).filter((id) => currentConfig.ai.channels[id]);
    if (!ids.length) {
        alert('Please SELECT the channels you want to delete');
        return;
    }
    const deletable = ids.filter((id) => id !== currentConfig.ai.default_channel);
    if (!deletable.length) {
        alert('The default channel cannot be bulk deleted. Please switch the default channel first');
        return;
    }
    if (Object.keys(currentConfig.ai.channels || {}).length - deletable.length < 1) {
        alert('At least one AI channel must be kept');
        return;
    }
    const names = deletable.map((id) => currentConfig.ai.channels[id]?.name || id).join(', ');
    if (!confirm(`Are you sure you want to delete ${deletable.length} AI channel(s)?\n${names}`)) {
        return;
    }
    deletable.forEach((id) => {
        delete currentConfig.ai.channels[id];
        selectedAIChannelBulkIds.delete(id);
        delete aiChannelProbeResults[id];
    });
    if (!currentConfig.ai.channels[selectedAIChannelId]) {
        selectedAIChannelId = currentConfig.ai.default_channel;
    }
    renderAIChannelSelect();
    writeAIChannelToMainForm(selectedAIChannelId);
    await persistAIConfigOnlyToServer(`Deleted ${deletable.length} channel(s)`);
}

if (typeof window !== 'undefined') {
    window.selectAIChannelForEditing = selectAIChannelForEditing;
    window.saveSelectedAIChannel = saveSelectedAIChannel;
    window.setSelectedAIChannelDefault = setSelectedAIChannelDefault;
    window.createAIChannelFromForm = createAIChannelFromForm;
    window.copyAIChannelFromForm = copyAIChannelFromForm;
    window.deleteSelectedAIChannel = deleteSelectedAIChannel;
    window.probeSelectedAIChannels = probeSelectedAIChannels;
    window.deleteCheckedAIChannels = deleteCheckedAIChannels;
}

if (typeof document !== 'undefined' && !document.__aiChannelI18nBound) {
    document.__aiChannelI18nBound = true;
    document.addEventListener('languagechange', function () {
        if (!currentConfig?.ai) return;
        renderAIChannelSelect();
        updateAIChannelEditorChrome(selectedAIChannelId || currentConfig.ai.default_channel);
    });
}

function initModelListControls() {
    bindAIChannelEditorPreviewSync();
    const providerEl = document.getElementById('openai-provider');
    if (providerEl && !providerEl.dataset.modelListBound) {
        providerEl.dataset.modelListBound = '1';
        providerEl.addEventListener('change', syncModelListFetchButtons);
    }
    const visionProv = document.getElementById('vision-provider');
    if (visionProv && !visionProv.dataset.modelListBound) {
        visionProv.dataset.modelListBound = '1';
        visionProv.addEventListener('change', syncModelListFetchButtons);
    }
    const hitlAuditProv = document.getElementById('hitl-audit-model-provider');
    if (hitlAuditProv && !hitlAuditProv.dataset.modelListBound) {
        hitlAuditProv.dataset.modelListBound = '1';
        hitlAuditProv.addEventListener('change', syncModelListFetchButtons);
    }
    const hitlAuditbackend = document.getElementById('hitl-audit-backend');
    if (hitlAuditbackend && !hitlAuditbackend.dataset.backendBound) {
        hitlAuditbackend.dataset.backendBound = '1';
        hitlAuditbackend.addEventListener('change', function () {
            syncHitlAuditbackendUI();
            syncModelListFetchButtons();
        });
        syncHitlAuditbackendUI();
    }
    const knowledgeEmbeddingProv = document.getElementById('knowledge-embedding-provider');
    if (knowledgeEmbeddingProv && !knowledgeEmbeddingProv.dataset.modelListBound) {
        knowledgeEmbeddingProv.dataset.modelListBound = '1';
        knowledgeEmbeddingProv.addEventListener('change', syncModelListFetchButtons);
    }
    bindModelSelect('OpenAI');
    bindModelSelect('vision');
    bindModelSelect('hitlAudit');
    bindModelSelect('knowledgeEmbedding');
    syncModelListFetchButtons();
}

function modelSelectIds(scope) {
    if (scope === 'vision') {
        return { selectId: 'vision-model-SELECT', inputId: 'vision-model' };
    }
    if (scope === 'hitlAudit') {
        return { selectId: 'hitl-audit-model-SELECT', inputId: 'hitl-audit-model-name' };
    }
    if (scope === 'knowledgeEmbedding') {
        return { selectId: 'knowledge-embedding-model-SELECT', inputId: 'knowledge-embedding-model' };
    }
    return { selectId: 'openai-model-SELECT', inputId: 'openai-model' };
}

function bindModelSelect(scope) {
    const { selectId, inputId } = modelSelectIds(scope);
    const SELECT = document.getElementById(selectId);
    if (!SELECT || SELECT.dataset.bound) return;
    SELECT.dataset.bound = '1';
    enhanceModelPickSelect(selectId);
    SELECT.addEventListener('change', function () {
        if (!SELECT.value) return;
        const INPUT = document.getElementById(inputId);
        if (INPUT) INPUT.value = SELECT.value;
        if (scope === 'OpenAI') {
            syncAIChannelEditorPreview();
        }
    });
}

function resolveModelListCredentials(scope) {
    if (scope === 'vision') {
        const vp = (document.getElementById('vision-provider')?.value || '').trim();
        const provider = vp || document.getElementById('openai-provider')?.value || 'OpenAI';
        const baseUrl = (document.getElementById('vision-base-URL')?.value || '').trim()
            || (document.getElementById('openai-base-URL')?.value || '').trim();
        const apiKey = (document.getElementById('vision-API-key')?.value || '').trim()
            || (document.getElementById('openai-API-key')?.value || '').trim();
        return { provider, base_url: baseUrl, API_KEY: apiKey };
    }
    if (scope === 'hitlAudit') {
        const hp = (document.getElementById('hitl-audit-model-provider')?.value || '').trim();
        const provider = hp || document.getElementById('openai-provider')?.value || 'OpenAI';
        const baseUrl = (document.getElementById('hitl-audit-model-base-URL')?.value || '').trim()
            || (document.getElementById('openai-base-URL')?.value || '').trim();
        const apiKey = (document.getElementById('hitl-audit-model-API-key')?.value || '').trim()
            || (document.getElementById('openai-API-key')?.value || '').trim();
        return { provider, base_url: baseUrl, API_KEY: apiKey };
    }
    if (scope === 'knowledgeEmbedding') {
        const kp = (document.getElementById('knowledge-embedding-provider')?.value || '').trim();
        const provider = kp || document.getElementById('openai-provider')?.value || 'OpenAI';
        const baseUrl = (document.getElementById('knowledge-embedding-base-URL')?.value || '').trim()
            || (document.getElementById('openai-base-URL')?.value || '').trim();
        const apiKey = (document.getElementById('knowledge-embedding-API-key')?.value || '').trim()
            || (document.getElementById('openai-API-key')?.value || '').trim();
        return { provider, base_url: baseUrl, API_KEY: apiKey };
    }
    return {
        provider: document.getElementById('openai-provider')?.value || 'OpenAI',
        base_url: (document.getElementById('openai-base-URL')?.value || '').trim(),
        API_KEY: (document.getElementById('openai-API-key')?.value || '').trim()
    };
}

function syncModelListFetchButtons() {
    const tFn = typeof window.t === 'function' ? window.t : (k) => k;
    const openaiProv = document.getElementById('openai-provider')?.value || 'OpenAI';
    const openaiBtn = document.getElementById('fetch-openai-models-btn');
    const openaiHint = document.getElementById('fetch-openai-models-hint');
    const openaiSelect = document.getElementById('openai-model-SELECT');
    const isClaudeOpenai = openaiProv === 'claude';
    if (openaiBtn) {
        openaiBtn.style.display = isClaudeOpenai ? 'none' : '';
    }
    if (openaiSelect && isClaudeOpenai) {
        openaiSelect.style.display = 'none';
        const openaiWrap = modelPickSelectMap['openai-model-SELECT'];
        if (openaiWrap) openaiWrap.wrapper.style.display = 'none';
    } else if (openaiSelect && !isClaudeOpenai) {
        syncModelPickDropdown('openai-model-SELECT');
    }
    if (openaiHint) {
        if (isClaudeOpenai) {
            openaiHint.textContent = tFn('settingsBasic.modelsListClaudeHint');
            openaiHint.style.display = '';
        } else {
            openaiHint.textContent = '';
            openaiHint.style.display = 'none';
        }
    }

    const vp = (document.getElementById('vision-provider')?.value || '').trim();
    const visionEffectiveProv = vp || openaiProv;
    const visionBtn = document.getElementById('fetch-vision-models-btn');
    const visionHint = document.getElementById('fetch-vision-models-hint');
    const visionSelect = document.getElementById('vision-model-SELECT');
    const isClaudeVision = visionEffectiveProv === 'claude';
    if (visionBtn) {
        visionBtn.style.display = isClaudeVision ? 'none' : '';
    }
    if (visionSelect && isClaudeVision) {
        visionSelect.style.display = 'none';
        const visionWrap = modelPickSelectMap['vision-model-SELECT'];
        if (visionWrap) visionWrap.wrapper.style.display = 'none';
    } else if (visionSelect && !isClaudeVision) {
        syncModelPickDropdown('vision-model-SELECT');
    }
    if (visionHint) {
        if (isClaudeVision) {
            visionHint.textContent = tFn('settingsBasic.modelsListClaudeHint');
            visionHint.style.display = '';
        } else {
            visionHint.textContent = '';
            visionHint.style.display = 'none';
        }
    }

    const hp = (document.getElementById('hitl-audit-model-provider')?.value || '').trim();
    const hitlAuditEffectiveProv = hp || openaiProv;
    const hitlAuditBtn = document.getElementById('fetch-hitl-audit-models-btn');
    const hitlAudithint = document.getElementById('fetch-hitl-audit-models-hint');
    const hitlAuditSelect = document.getElementById('hitl-audit-model-SELECT');
    const isClaudeHitlAudit = hitlAuditEffectiveProv === 'claude';
    if (hitlAuditBtn) {
        hitlAuditBtn.style.display = isClaudeHitlAudit ? 'none' : '';
    }
    if (hitlAuditSelect && isClaudeHitlAudit) {
        hitlAuditSelect.style.display = 'none';
        const hitlAuditWrap = modelPickSelectMap['hitl-audit-model-SELECT'];
        if (hitlAuditWrap) hitlAuditWrap.wrapper.style.display = 'none';
    } else if (hitlAuditSelect && !isClaudeHitlAudit) {
        syncModelPickDropdown('hitl-audit-model-SELECT');
    }
    if (hitlAudithint) {
        if (isClaudeHitlAudit) {
            hitlAudithint.textContent = tFn('settingsBasic.modelsListClaudeHint');
            hitlAudithint.style.display = '';
        } else {
            hitlAudithint.textContent = '';
            hitlAudithint.style.display = 'none';
        }
    }

    const kp = (document.getElementById('knowledge-embedding-provider')?.value || '').trim();
    const knowledgeEmbeddingEffectiveProv = kp || openaiProv;
    const knowledgeEmbeddingBtn = document.getElementById('fetch-knowledge-embedding-models-btn');
    const knowledgeEmbeddinghint = document.getElementById('fetch-knowledge-embedding-models-hint');
    const knowledgeEmbeddingSelect = document.getElementById('knowledge-embedding-model-SELECT');
    const isClaudeKnowledgeEmbedding = knowledgeEmbeddingEffectiveProv === 'claude';
    if (knowledgeEmbeddingBtn) {
        knowledgeEmbeddingBtn.style.display = isClaudeKnowledgeEmbedding ? 'none' : '';
    }
    if (knowledgeEmbeddingSelect && isClaudeKnowledgeEmbedding) {
        knowledgeEmbeddingSelect.style.display = 'none';
        const knowledgeEmbeddingWrap = modelPickSelectMap['knowledge-embedding-model-SELECT'];
        if (knowledgeEmbeddingWrap) knowledgeEmbeddingWrap.wrapper.style.display = 'none';
    } else if (knowledgeEmbeddingSelect && !isClaudeKnowledgeEmbedding) {
        syncModelPickDropdown('knowledge-embedding-model-SELECT');
    }
    if (knowledgeEmbeddinghint) {
        if (isClaudeKnowledgeEmbedding) {
            knowledgeEmbeddinghint.textContent = tFn('settingsBasic.modelsListClaudeHint');
            knowledgeEmbeddinghint.style.display = '';
        } else {
            knowledgeEmbeddinghint.textContent = '';
            knowledgeEmbeddinghint.style.display = 'none';
        }
    }
}

function populateModelSelect(scope, models, currentValue) {
    const { selectId, inputId } = modelSelectIds(scope);
    const SELECT = document.getElementById(selectId);
    const INPUT = document.getElementById(inputId);
    if (!SELECT) return;
    const tFn = typeof window.t === 'function' ? window.t : (k) => k;
    SELECT.innerHTML = '';
    const placeholder = document.createElement('option');
    placeholder.value = '';
    placeholder.disabled = true;
    placeholder.textContent = tFn('settingsBasic.modelsListSelectPlaceholder');
    SELECT.appendChild(placeholder);

    const seen = new Set();
    const addOption = (id) => {
        const val = (id || '').trim();
        if (!val || seen.has(val)) return;
        seen.add(val);
        const opt = document.createElement('option');
        opt.value = val;
        opt.textContent = val;
        SELECT.appendChild(opt);
    };
    (models || []).forEach(addOption);
    const cur = (currentValue || (INPUT && INPUT.value) || '').trim();
    if (cur && seen.has(cur)) {
        SELECT.value = cur;
    } else {
        SELECT.value = '';
    }
    enhanceModelPickSelect(selectId);
    syncModelPickDropdown(selectId);
}

async function fetchModelList(scope) {
    const tFn = typeof window.t === 'function' ? window.t : (k) => k;
    const creds = resolveModelListCredentials(scope);
    creds.credential_scope = scope;
    if (scope === 'OpenAI') creds.channel_id = selectedAIChannelId;
    const keyinputByScope = {vision: 'vision-API-key', hitlAudit: 'hitl-audit-model-API-key', knowledgeEmbedding: 'knowledge-embedding-API-key'};
    if (keyinputByScope[scope] && !document.getElementById(keyinputByScope[scope])?.value.trim()) {
        creds.channel_id = selectedAIChannelId;
    }
    const modelListUiIds = {
        OpenAI: {
            btnId: 'fetch-openai-models-btn',
            resultId: 'fetch-openai-models-result'
        },
        vision: {
            btnId: 'fetch-vision-models-btn',
            resultId: 'fetch-vision-models-result'
        },
        hitlAudit: {
            btnId: 'fetch-hitl-audit-models-btn',
            resultId: 'fetch-hitl-audit-models-result'
        },
        knowledgeEmbedding: {
            btnId: 'fetch-knowledge-embedding-models-btn',
            resultId: 'fetch-knowledge-embedding-models-result'
        }
    };
    const uiIds = modelListUiIds[scope] || modelListUiIds.OpenAI;
    const btnId = uiIds.btnId;
    const resultId = uiIds.resultId;
    const inputId = modelSelectIds(scope).inputId;
    const btn = document.getElementById(btnId);
    const resultEl = document.getElementById(resultId);
    const inputEl = document.getElementById(inputId);

    if (creds.provider === 'claude') {
        if (resultEl) {
            resultEl.textContent = tFn('settingsBasic.modelsListClaudeHint');
            resultEl.style.color = 'var(--text-muted, #718096)';
        }
        return;
    }
    if (!creds.API_KEY) {
        if (resultEl) {
            resultEl.textContent = tFn('settingsBasic.modelsListNeedApiKey');
            resultEl.style.color = 'var(--error-color, #e53e3e)';
        }
        return;
    }

    if (btn) {
        btn.style.pointerEvents = 'none';
        btn.style.opacity = '0.5';
    }
    if (resultEl) {
        resultEl.textContent = tFn('settingsBasic.modelsListFetching');
        resultEl.style.color = 'var(--text-muted, #718096)';
    }

    try {
        const response = await apiFetch('/api/config/list-models', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(creds)
        });
        const result = await response.json();
        if (!response.ok) {
            throw new Error(result.error || 'Request failed');
        }
        if (!result.success) {
            if (resultEl) {
                resultEl.textContent = (result.supported === false
                    ? tFn('settingsBasic.modelsListClaudeHint')
                    : tFn('settingsBasic.modelsListFailed')) + ': ' + (result.error || '');
                resultEl.style.color = 'var(--error-color, #e53e3e)';
            }
            return;
        }
        const currentValue = inputEl ? inputEl.value.trim() : '';
        populateModelSelect(scope, result.models || [], currentValue);
        if (resultEl) {
            const count = result.count != null ? result.count : (result.models || []).length;
            resultEl.textContent = tFn('settingsBasic.modelsListSuccess').replace('{count}', String(count));
            resultEl.style.color = 'var(--success-color, #38a169)';
        }
    } catch (error) {
        if (resultEl) {
            resultEl.textContent = tFn('settingsBasic.modelsListFailed') + ': ' + error.message;
            resultEl.style.color = 'var(--error-color, #e53e3e)';
        }
    } finally {
        if (btn) {
            btn.style.pointerEvents = '';
            btn.style.opacity = '';
        }
    }
}

async function testVisionConnection() {
    const resultEl = document.getElementById('test-vision-result');
    const vision = collectVisionConfigFromForm();
    const OpenAI = {
        provider: document.getElementById('openai-provider')?.value || 'OpenAI',
        API_KEY: document.getElementById('openai-API-key')?.value.trim() || '',
        base_url: document.getElementById('openai-base-URL')?.value.trim() || '',
        model: document.getElementById('openai-model')?.value.trim() || ''
    };
    const apiKey = vision.API_KEY || OpenAI.API_KEY;
    const model = vision.model;
    if (!apiKey || !model) {
        if (resultEl) {
            resultEl.textContent = typeof window.t === 'function' ? window.t('settingsBasic.visionTestFillRequired') : 'Please fill in the vision model and ensure the API Key is available';
        }
        return;
    }
    if (resultEl) {
        resultEl.textContent = typeof window.t === 'function' ? window.t('settingsBasic.testing') : 'Testing...';
        resultEl.style.color = '';
    }
    try {
        const response = await apiFetch('/api/config/test-vision', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ vision: vision, OpenAI: OpenAI, channel_id: selectedAIChannelId })
        });
        const result = await response.json();
        if (result.success) {
            const latency = result.latency_ms != null ? ` (${result.latency_ms}ms)` : '';
            const modelInfo = result.model ? ` [${result.model}]` : '';
            resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testSuccess') : 'Connected successfully') + modelInfo + latency;
            resultEl.style.color = 'var(--success-color, #38a169)';
        } else {
            resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testFailed') : 'Connection failed') + ': ' + (result.error || 'Unknown error');
            resultEl.style.color = 'var(--error-color, #e53e3e)';
        }
    } catch (error) {
        resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testError') : 'Test error') + ': ' + error.message;
        resultEl.style.color = 'var(--error-color, #e53e3e)';
    }
}

function isHitlAuditTypeSafe() {
    const v = (document.getElementById('hitl-audit-backend')?.value || '').trim().toLowerCase();
    return v === 'TypeSafe' || v === 'jev';
}

function syncHitlAuditbackendUI() {
    const ts = isHitlAuditTypeSafe();
    const providerGroup = document.getElementById('hitl-audit-openai-provider-group');
    if (providerGroup) providerGroup.style.display = ts ? 'none' : '';
    const fetchBtn = document.getElementById('fetch-hitl-audit-models-btn');
    if (fetchBtn) fetchBtn.style.display = ts ? 'none' : '';
    const openaiHint = document.getElementById('hitl-audit-model-openai-hint');
    const tsHint = document.getElementById('hitl-audit-model-TypeSafe-hint');
    if (openaiHint) openaiHint.hidden = ts;
    if (tsHint) tsHint.hidden = !ts;
    const promptHint = document.getElementById('hitl-audit-prompt-TypeSafe-hint');
    if (promptHint) promptHint.hidden = !ts;

    const tFn = function (key, fallback) {
        return typeof settingsT === 'function' ? settingsT(key, fallback) : (fallback || key);
    };
    const baseUrlEl = document.getElementById('hitl-audit-model-base-URL');
    const apiKeyEl = document.getElementById('hitl-audit-model-API-key');
    const modelEl = document.getElementById('hitl-audit-model-name');
    if (baseUrlEl) {
        baseUrlEl.placeholder = ts
            ? tFn('settings.hitl.auditModelTypeSafeBaseUrlPlaceholder', 'Leave blank to use https://API.TypeSafe.ai')
            : tFn('settings.hitl.auditModelBaseUrlPlaceholder', 'Leave blank to reuse the main model Base URL');
    }
    if (apiKeyEl) {
        apiKeyEl.placeholder = ts
            ? tFn('settings.hitl.auditModelTypeSafeApiKeyPlaceholder', 'TypeSafe API Key (required, does not reuse the main model key)')
            : tFn('settings.hitl.auditModelApiKeyPlaceholder', 'Leave blank to reuse the main model API Key');
    }
    if (modelEl) {
        modelEl.placeholder = ts
            ? tFn('settings.hitl.auditModelTypeSafeNamePlaceholder', 'Leave blank to use jev-latest')
            : tFn('settings.hitl.auditModelNamePlaceholder', 'Leave blank to reuse the main model; a smaller model is recommended');
    }
}
window.syncHitlAuditbackendUI = syncHitlAuditbackendUI;

function collectHitlAuditModelEffectiveConfig() {
    const main = {
        provider: document.getElementById('openai-provider')?.value || 'OpenAI',
        API_KEY: document.getElementById('openai-API-key')?.value.trim() || '',
        base_url: document.getElementById('openai-base-URL')?.value.trim() || '',
        model: document.getElementById('openai-model')?.value.trim() || ''
    };
    return {
        provider: document.getElementById('hitl-audit-model-provider')?.value || main.provider,
        base_url: document.getElementById('hitl-audit-model-base-URL')?.value.trim() || main.base_url,
        API_KEY: document.getElementById('hitl-audit-model-API-key')?.value.trim() || main.API_KEY,
        model: document.getElementById('hitl-audit-model-name')?.value.trim() || main.model
    };
}

async function testHitlAuditModelConnection() {
    const btn = document.getElementById('test-hitl-audit-model-btn');
    const resultEl = document.getElementById('test-hitl-audit-model-result');
    const typeSafe = isHitlAuditTypeSafe();
    const cfg = collectHitlAuditModelEffectiveConfig();
    const apiKey = typeSafe
        ? (document.getElementById('hitl-audit-model-API-key')?.value.trim() || '')
        : cfg.API_KEY;
    const baseUrl = typeSafe
        ? (document.getElementById('hitl-audit-model-base-URL')?.value.trim() || '')
        : cfg.base_url;
    const model = typeSafe
        ? (document.getElementById('hitl-audit-model-name')?.value.trim() || 'jev-latest')
        : cfg.model;

    if (typeSafe) {
        if (!apiKey) {
            if (resultEl) {
                resultEl.style.color = 'var(--danger-color, #e53e3e)';
                resultEl.textContent = typeof settingsT === 'function'
                    ? settingsT('settings.hitl.testTypeSafeFillRequired', 'Please fill in the TypeSafe API Key first')
                    : 'Please fill in the TypeSafe API Key first';
            }
            return;
        }
    } else if (!cfg.base_url || !cfg.API_KEY || !cfg.model) {
        if (resultEl) {
            resultEl.style.color = 'var(--danger-color, #e53e3e)';
            resultEl.textContent = typeof window.t === 'function' ? window.t('settingsBasic.testFillRequired') : 'Please fill in Base URL, API Key, and Model first';
        }
        return;
    }

    if (btn) {
        btn.style.pointerEvents = 'none';
        btn.style.opacity = '0.5';
    }
    if (resultEl) {
        resultEl.style.color = 'var(--text-muted, #888)';
        resultEl.textContent = typeof window.t === 'function' ? window.t('settingsBasic.testing') : 'Testing...';
    }

    try {
        const endpoint = typeSafe ? '/api/config/test-TypeSafe' : '/api/config/test-OpenAI';
        const payload = typeSafe
            ? { base_url: baseUrl, API_KEY: apiKey, model: model }
            : cfg;
        payload.credential_scope = 'hitlAudit';
        if (!document.getElementById('hitl-audit-model-API-key')?.value.trim()) payload.channel_id = selectedAIChannelId;
        const response = await apiFetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
        const result = await response.json();

        if (result.success) {
            if (resultEl) {
                resultEl.style.color = 'var(--success-color, #38a169)';
                const latency = result.latency_ms ? ` (${result.latency_ms}ms)` : '';
                const modelInfo = result.model ? ` [${result.model}]` : '';
                resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testSuccess') : 'Connected successfully') + modelInfo + latency;
            }
        } else if (resultEl) {
            resultEl.style.color = 'var(--danger-color, #e53e3e)';
            resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testFailed') : 'Connection failed') + ': ' + (result.error || 'Unknown error');
        }
    } catch (error) {
        if (resultEl) {
            resultEl.style.color = 'var(--danger-color, #e53e3e)';
            resultEl.textContent = (typeof window.t === 'function' ? window.t('settingsBasic.testError') : 'Test error') + ': ' + error.message;
        }
    } finally {
        if (btn) {
            btn.style.pointerEvents = '';
            btn.style.opacity = '';
        }
    }
}

function formatConnectionTestError(errorText) {
    const raw = String(errorText || '').trim() || settingsT('settingsBasic.testError', 'Test error');
    return {
        message: raw,
        detail: raw
    };
}

function setConnectionTestResult(resultEl, state, message, title) {
    if (!resultEl) return;
    resultEl.classList.remove('is-error', 'is-success', 'is-muted', 'is-visible');
    resultEl.style.color = '';
    resultEl.textContent = message || '';
    resultEl.title = title || '';
    if (message) {
        resultEl.classList.add('is-visible', state || 'is-muted');
    }
}

function syncConnectionTestResultForSelectedAIChannel() {
    const resultEl = document.getElementById('test-openai-result');
    if (!resultEl) return;
    const channelId = normalizeAIChannelId(selectedAIChannelId || currentConfig?.ai?.default_channel || 'default');
    const probe = aiChannelProbeResults[channelId];
    if (!probe) {
        setConnectionTestResult(resultEl, '', '');
        return;
    }
    const state = probe.status === 'ready'
        ? 'is-success'
        : probe.status === 'failed'
            ? 'is-error'
            : 'is-muted';
    let message = probe.message || '';
    const fillRequiredMessage = settingsT('settingsBasic.testFillRequired', 'Please fill in Base URL, API Key, and Model first');
    const failedPrefix = (typeof window.t === 'function' ? window.t('settingsBasic.testFailed') : 'Connection failed') + ': ';
    if (probe.status === 'failed' && message && message !== fillRequiredMessage && !message.startsWith(failedPrefix)) {
        message = failedPrefix + message;
    }
    setConnectionTestResult(resultEl, state, message);
}

function showConnectionTestFailure(resultEl, errorText) {
    if (!resultEl) return;
    const formatted = formatConnectionTestError(errorText);
    setConnectionTestResult(
        resultEl,
        'is-error',
        (typeof window.t === 'function' ? window.t('settingsBasic.testFailed') : 'Connection failed') + ': ' + formatted.message,
        formatted.detail || formatted.message
    );
}

// Test OpenAI connection
async function testOpenAIConnection() {
    const btn = document.getElementById('test-openai-btn');
    const resultEl = document.getElementById('test-openai-result');

    const provider = document.getElementById('openai-provider')?.value || 'OpenAI';
    const baseUrl = document.getElementById('openai-base-URL').value.trim();
    const apiKey = document.getElementById('openai-API-key').value.trim();
    const model = document.getElementById('openai-model').value.trim();
    const channelId = normalizeAIChannelId(selectedAIChannelId || currentConfig?.ai?.default_channel || 'default');
    const isTestingSameChannel = () => normalizeAIChannelId(selectedAIChannelId || currentConfig?.ai?.default_channel || 'default') === channelId;

    if (!baseUrl || !apiKey || !model) {
        const message = typeof window.t === 'function' ? window.t('settingsBasic.testFillRequired') : 'Please fill in Base URL, API Key, and Model first';
        aiChannelProbeResults[channelId] = { status: 'failed', message };
        if (isTestingSameChannel()) {
            setConnectionTestResult(resultEl, 'is-error', message);
        }
        syncSelectedAIChannelUI();
        return;
    }

    if (btn) {
        btn.style.pointerEvents = 'none';
        btn.style.opacity = '0.5';
    }
    const testingMessage = settingsT('settingsBasic.testing', 'Testing...');
    aiChannelProbeResults[channelId] = { status: 'testing', message: testingMessage };
    if (isTestingSameChannel()) {
        setConnectionTestResult(resultEl, 'is-muted', testingMessage);
    }
    syncSelectedAIChannelUI();

    try {
        const response = await apiFetch('/api/config/test-OpenAI', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                provider: provider,
                channel_id: channelId,
                base_url: baseUrl,
                API_KEY: apiKey,
                model: model
            })
        });

        const result = await response.json();

        if (result.success) {
            const latency = result.latency_ms ? ` (${result.latency_ms}ms)` : '';
            const modelInfo = result.model ? ` [${result.model}]` : '';
            const message = (typeof window.t === 'function' ? window.t('settingsBasic.testSuccess') : 'Connected successfully') + modelInfo + latency;
            aiChannelProbeResults[channelId] = { status: 'ready', message };
            if (isTestingSameChannel()) {
                setConnectionTestResult(resultEl, 'is-success', message);
            }
        } else {
            const message = formatConnectionTestError(result.error || 'Unknown error').message;
            aiChannelProbeResults[channelId] = { status: 'failed', message };
            if (isTestingSameChannel()) {
                showConnectionTestFailure(resultEl, message);
            }
        }
    } catch (error) {
        const message = formatConnectionTestError(error.message || 'Test error').message;
        aiChannelProbeResults[channelId] = { status: 'failed', message };
        if (isTestingSameChannel()) {
            showConnectionTestFailure(resultEl, message);
        }
    } finally {
        updateAIChannelSelectOption(channelId);
        syncSelectedAIChannelUI();
        if (btn) {
            btn.style.pointerEvents = '';
            btn.style.opacity = '';
        }
    }
}

// Save tool configuration (standalone function, used by MCP page)
async function saveToolsConfig() {
    if (typeof requirePermission === 'function' && !requirePermission('config:write')) return;
    try {
        // First save currentPage status to global map
        saveCurrentPageToolStates();
        
        // Fetch currentConfiguration (tool section only)
        const response = await apiFetch('/api/config');
        if (!response.ok) {
            throw new Error('failed to fetch configuration');
        }
        
        const currentConfig = await response.json();
        
        // Build a config object containing only the tool configuration
        const config =  ensureAIConfigShape(currentConfig || {}),
            agent: currentConfig.agent || {},
            multi_agent: {
                enabled: currentConfig?.multi_agent?.enabled === true,
                robot_default_agent_mode: currentConfig?.multi_agent?.robot_default_agent_mode || 'eino_single',
                batch_use_multi_agent: currentConfig?.multi_agent?.batch_use_multi_agent === true,
                plan_execute_loop_max_iterations: Number(currentConfig?.multi_agent?.plan_execute_loop_max_iterations || 0),
                tool_search_always_visible_tools: getAlwaysVisibleForSave()
            },
            tools: []
        };
        
        // Collect tool enable status (same logic as in applySettings)
        try {
            const allToolsMap = new Map();
            let  page = 1;
            let hasMore = true;
            const pageSize = 100;
            
            // Iterate all pages to fetch all tools
            while (hasMore) {
                const URL = `/api/config/tools? page=${ page}& page_size=${ pageSize}`;
                
                const  pageResponse = await apiFetch(URL);
                if (! pageResponse.ok) {
                    throw new Error('Failed to fetch tool list');
                }
                
                const  pageResult = await  pageResponse.json();
                
                // Add tools to the map
                 pageResult.tools.forEach(tool => {
                    const toolKey = getToolKey(tool);
                    const savedState = toolStateMap.get(toolKey);
                    allToolsMap.set(toolKey, {
                        name: tool.name,
                        enabled: savedState ? savedState.enabled : tool.enabled,
                        is_external: savedState ? savedState.is_external : (tool.is_external || false),
                        external_mcp: savedState ? savedState.external_mcp : (tool.external_mcp || '')
                    });
                });
                
                // Check whether there are more pages
                if (page >= pageResult.total_pages) {
                    hasMore = false;
                } else {
                     page++;
                }
            }
            
            // Add all tools to the configuration
            allToolsMap.forEach((tool, toolKey) => {
                config.tools.push({
                    name: tool.name,
                    enabled: tool.enabled,
                    is_external: tool.is_external,
                    external_mcp: tool.external_mcp
                });
            });
        } catch (error) {
            console.warn('Failed to fetch all tool list pages; using global status map only', error);
            // If fetch failed, use global status map
            toolStateMap.forEach((toolData, toolKey) => {
                // toolData.name stores the original tool name
                const toolName = toolData.name || toolKey.split('::').pop();
                config.tools.push({
                    name: toolName,
                    enabled: toolData.enabled,
                    is_external: toolData.is_external,
                    external_mcp: toolData.external_mcp
                });
            });
        }
        
        // Update configuration
        const updateResponse = await apiFetch('/api/config', {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(config)
        });
        
        if (!updateResponse.ok) {
            const error = await updateResponse.json();
            throw new Error(error.error || 'Failed to update configuration');
        }
        
        // Apply configuration
        const applyResponse = await apiFetch('/api/config/apply', {
            method: 'POST'
        });
        
        if (!applyResponse.ok) {
            const error = await applyResponse.json();
            throw new Error(error.error || 'Apply configurationfailed');
        }
        
        alert(typeof window.t === 'function' ? window.t('MCP.toolsConfigSaved') : 'Tool configuration saved successfully!');
        
        // Reload tool list to reflect latest status
        if (typeof loadToolsList === 'function') {
            await loadToolsList(toolsPagination. page, toolssearchKeyword);
        }
    } catch (error) {
        console.error('Failed to save tool configuration:', error);
        alert((typeof window.t === 'function' ? window.t('MCP.saveToolsConfigFailed') : 'Failed to save tool configuration') + ': ' + error.message);
    }
}

function setPasswordFieldError(INPUT, message) {
    if (!INPUT) return;
    INPUT.classList.toggle('error', !!message);
    INPUT.setAttribute('aria-invalid', message ? 'true' : 'false');
    const error = document.getElementById(INPUT.id + '-error');
    if (error) {
        error.textContent = message || '';
        error.hidden = !message;
        INPUT.setAttribute('aria-describedby', error.id);
    }
}

function passwordValidationText(key, fallback) {
    return typeof window.t === 'function' ? window.t('settingsSecurity.' + key) : fallback;
}

function clearPasswordFieldError(INPUT) {
    setPasswordFieldError(INPUT, '');
    const error = document.getElementById('password-form-error');
    if (error) { error.textContent = ''; error.hidden = true; }
}

function resetPasswordForm() {
    const currentInput = document.getElementById('auth-current-password');
    const newInput = document.getElementById('auth-new-password');
    const confirmInput = document.getElementById('auth-confirm-password');

    [currentInput, newInput, confirmInput].forEach(INPUT => {
        if (INPUT) {
            INPUT.value = '';
            setPasswordFieldError(INPUT, '');
        }
    });
    const error = document.getElementById('password-form-error');
    if (error) { error.textContent = ''; error.hidden = true; }
}

async function changePassword() {
    const currentInput = document.getElementById('auth-current-password');
    const newInput = document.getElementById('auth-new-password');
    const confirmInput = document.getElementById('auth-confirm-password');
    const submitBtn = document.querySelector('.change-password-submit');

    [currentInput, newInput, confirmInput].forEach(INPUT => setPasswordFieldError(INPUT, ''));
    const formError = document.getElementById('password-form-error');
    if (formError) { formError.textContent = ''; formError.hidden = true; }

    const currentPassword = currentInput?.value.trim() || '';
    const newPassword = newInput?.value.trim() || '';
    const confirmPassword = confirmInput?.value.trim() || '';

    let hasError = false;

    if (!currentPassword) {
        setPasswordFieldError(currentInput, passwordValidationText('currentRequired', 'Please entercurrent Password'));
        hasError = true;
    }

    if (!newPassword || newPassword.length < 8) {
        setPasswordFieldError(newInput, passwordValidationText('newTooShort', 'New password must be at least 8 characters'));
        hasError = true;
    }

    if (newPassword !== confirmPassword) {
        setPasswordFieldError(confirmInput, passwordValidationText('confirmMismatch', 'New passwords do not match'));
        hasError = true;
    }

    if (hasError) {
        [currentInput, newInput, confirmInput].find(INPUT => INPUT && INPUT.getAttribute('aria-invalid') === 'true')?.focus();
        return;
    }

    if (submitBtn) {
        submitBtn.disabled = true;
    }

    try {
        const response = await apiFetch('/api/auth/change-password', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                oldPassword: currentPassword,
                newPassword: newPassword
            })
        });

        const result = await response.json().catch(() => ({}));
        if (!response.ok) {
            throw new Error(result.error || 'Change passwordfailed');
        }

        const pwdMsg = typeof window.t === 'function' ? window.t('settings.security.passwordUpdated') : 'Password updated. Please sign in again with your new password.';
        alert(pwdMsg);
        resetPasswordForm();
        handleUnauthorized({ message: pwdMsg, silent: false });
        closeSettings();
    } catch (error) {
        console.error('Change passwordfailed:', error);
        if (formError) { formError.textContent = error.message; formError.hidden = false; }
        alert((typeof window.t === 'function' ? window.t('settings.security.changePasswordFailed') : 'Change passwordfailed') + ': ' + error.message);
    } finally {
        if (submitBtn) {
            submitBtn.disabled = false;
        }
    }
}

// ==================== External MCP ====================

let currenteditingMCPName = null;

// Fetch External MCP list data (for polling; returns { servers, stats })
async function fetchExternalMCPs() {
    const response = await apiFetch('/api/external-MCP');
    if (!response.ok) {
        if (typeof readApiError === 'function') {
            throw new Error(await readApiError(response, 'Failed to fetch External MCP list'));
        }
        throw new Error('Failed to fetch External MCP list');
    }
    return response.json();
}

// MCP management page: periodically refresh External MCP status (detects background disconnects / auto-reconnects)
let externalMcpPollTimer = null;
const EXTERNAL_MCP_POLL_INTERVAL_MS = 8000;
let externalMcpRenderSignature = '';

function renderExternalMCPData(data, forceRender = false) {
    const servers = data.servers || {};
    const stats = data.stats || {};
    const signature = JSON.stringify({ servers, stats });

    if (!forceRender && signature === externalMcpRenderSignature) {
        updateExternalMcpCardSelection();
        return false;
    }

    externalMcpRenderSignature = signature;
    renderExternalMCPList(servers);
    renderExternalMCPStats(stats);
    return true;
}

function startExternalMcpPoll() {
    stopExternalMcpPoll();
    externalMcpPollTimer = setInterval(function () {
        const mcpPage = document.getElementById('page-mcp-management');
        if (!mcpPage || !mcpPage.classList.contains('active')) {
            stopExternalMcpPoll();
            return;
        }
        if (document.hidden) {
            return;
        }
        loadExternalMCPs().catch(function () { /* ignore */ });
    }, EXTERNAL_MCP_POLL_INTERVAL_MS);
}

function stopExternalMcpPoll() {
    if (externalMcpPollTimer) {
        clearInterval(externalMcpPollTimer);
        externalMcpPollTimer = null;
    }
}

// Load and render the External MCP list
async function loadExternalMCPs(options = {}) {
    try {
        // Wait for i18n to be ready, to avoid translation functions being uninitialized during fast refresh causing placeholder display
        if (window.i18nReady) await window.i18nReady;
        const data = await fetchExternalMCPs();
        renderExternalMCPData(data, options.forceRender === true);
    } catch (error) {
        console.error('failed to load external MCP list:', error);
        const list = document.getElementById('external-mcp-list');
        if (list) {
            const errT = typeof window.t === 'function' ? window.t : (k) => k;
        list.innerHTML = `<div class="error">${escapeHtml(errT('MCP.loadExternalMCPFailed'))}: ${escapeHtml(error.message)}</div>`;
        }
    }
}

async function reloadMcpToolsAfterExternalChange(refreshExternal = false) {
    if (typeof loadToolsList === 'function') {
        const  page = (toolsPagination && toolsPagination. page) ? toolsPagination. page : 1;
        await loadToolsList( page, toolssearchKeyword, { refreshExternal });
    }
}

// Poll the list until the tool count for the specified MCP has updated (polls every second, stops as soon as it has, no fixed delay)
// When name is null, poll maxAttempts times only, without checking tool_count
async function pollExternalMCPToolCount(name, maxAttempts = 10) {
    const pollIntervalMs = 1000;
    for (let attempt = 0; attempt < maxAttempts; attempt++) {
        await new Promise(r => setTimeout(r, pollIntervalMs));
        try {
            const data = await fetchExternalMCPs();
            renderExternalMCPData(data);
            if (name != null) {
                const server = data.servers && data.servers[name];
                if (server && server.tool_count > 0) break;
            }
        } catch (e) {
            console.warn('Failed to poll tool count:', e);
        }
    }
    await reloadMcpToolsAfterExternalChange(true);
    if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
        window.refreshMentionTools();
    }
}

// Render the External MCP list
function renderExternalMCPList(servers) {
    const list = document.getElementById('external-mcp-list');
    if (!list) return;
    const layout = document.querySelector('.mcp-management-layout');
    
    if (Object.keys(servers).length === 0) {
        if (layout) layout.classList.add('external-empty');
        const emptyT = typeof window.t === 'function' ? window.t : (k) => k;
        list.innerHTML = '<div class="empty">📋 ' + emptyT('MCP.noExternalMCP') + '<br><span style="font-size: 0.875rem; margin-top: 8px; display: block;">' + emptyT('MCP.clickToAddExternal') + '</span></div>';
        return;
    }
    if (layout) layout.classList.remove('external-empty');
    
    let html = '<div class="external-mcp- items">';
    for (const [name, server] of Object.entries(servers)) {
        const status = server.status || 'disconnected';
        const statusClass = status === 'connected' ? 'status-connected' : 
                           status === 'connecting' ? 'status-connecting' :
                           status === 'error' ? 'status-error' :
                           status === 'disabled' ? 'status-disabled' : 'status-disconnected';
        const statusT = typeof window.t === 'function' ? window.t : (k) => k;
        const statusText = status === 'connected' ? statusT('MCP.connected') : 
                          status === 'connecting' ? statusT('MCP.connecting') :
                          status === 'error' ? statusT('MCP.connectionFailed') :
                          status === 'disabled' ? statusT('MCP.disabled') : statusT('MCP.disconnected');
        const transport = server.config.type || server.config.transport || (server.config.command ? 'stdio' : 'http');
        const transportIcon = transport === 'stdio' ? '⚙️' : '🌐';
        
        const hasTools = server.tool_count !== undefined && server.tool_count > 0;
        const cardClickTitle = hasTools
            ? escapeHtml(statusT('MCP.clickToViewTools', { name }))
            : '';
        const cardClass = hasTools ? 'external-mcp-item clickable' : 'external-mcp-item';
        const selectedClass = toolsExternalMcpfilter === name ? ' selected' : '';

        html += `
            <div class="${cardClass}${selectedClass}" data-mcp-name="${settingsEscapeAttr(name)}"${hasTools ? ` onclick="scrollToExternalMCPTools(${settingsEscapeJsStringAttr(name)}, event)" title="${settingsEscapeAttr(cardClickTitle)}"` : ''}>
                <div class="external-mcp-item-header">
                    <div class="external-mcp-item-INFO">
                        <h4>${transportIcon} ${escapeHtml(name)}${server.tool_count !== undefined && server.tool_count > 0 ? `<span class="tool-count-badge" title="${escapeHtml(statusT('MCP.toolCount'))}">🔧 ${server.tool_count}</span>` : ''}</h4>
                        <span class="external-mcp-status ${statusClass}">${statusText}</span>
                    </div>
                    <div class="external-mcp-item-actions">
                        ${status === 'connected' || status === 'disconnected' || status === 'error' || status === 'disabled' ?
                            `<button class="btn-small" id="btn-toggle-${settingsEscapeAttr(name)}" onclick="toggleExternalMCP(${settingsEscapeJsStringAttr(name)}, ${settingsEscapeJsStringAttr(status)})" title="${settingsEscapeAttr(status === 'connected' ? statusT('MCP.stopConnection') : statusT('MCP.startConnection'))}">
                                ${status === 'connected' ? '⏸ ' + statusT('MCP.stop') : '▶ ' + statusT('MCP.start')}
                            </button>` :
                            status === 'connecting' ?
                            `<button class="btn-small" id="btn-toggle-${settingsEscapeAttr(name)}" disabled style="opacity: 0.6; cursor: not-allowed;">
                                ⏳ ${statusT('MCP.connecting')}
                            </button>` : ''}
                        <button class="btn-small" onclick="editExternalMCP(${settingsEscapeJsStringAttr(name)})" title="${settingsEscapeAttr(statusT('MCP.editConfig'))}" ${status === 'connecting' ? 'disabled' : ''}>✏️ ${statusT('common.edit')}</button>
                        <button class="btn-small btn-danger" onclick="deleteExternalMCP(${settingsEscapeJsStringAttr(name)})" title="${settingsEscapeAttr(statusT('MCP.deleteConfig'))}" ${status === 'connecting' ? 'disabled' : ''}>🗑 ${statusT('common.delete')}</button>
                    </div>
                </div>
                ${(status === 'error' || status === 'disconnected') && server.error ? `
                <div class="external-mcp-error" style="margin: 12px 0; padding: 12px; background: ${status === 'error' ? '#fee' : '#fff8e6'}; border-left: 3px solid ${status === 'error' ? '#f44' : '#e6a700'}; border-radius: 4px; color: ${status === 'error' ? '#c33' : '#8a6d00'}; font-size: 0.875rem;">
                    <strong>${status === 'error' ? '❌' : '⚠️'} ${statusT('MCP.connectionErrorLabel')}</strong>${escapeHtml(server.error)}
                </div>` : ''}
                <div class="external-mcp-item-details">
                    <div>
                        <strong>${statusT('MCP.transportMode')}</strong>
                        <span>${transportIcon} ${escapeHtml(transport.toUpperCase())}</span>
                    </div>
                    ${server.tool_count !== undefined && server.tool_count > 0 ? `
                    <div>
                        <strong>${statusT('MCP.toolCount')}</strong>
                        <span style="font-weight: 600; color: var(--accent-color);">${statusT('MCP.toolsCountValue', { count: server.tool_count })}</span>
                    </div>` : server.tool_count === 0 && status === 'connected' ? `
                    <div>
                        <strong>${statusT('MCP.toolCount')}</strong>
                        <span style="color: var(--text-muted);">${statusT('MCP.noTools')}</span>
                    </div>` : ''}
                    ${server.config.description ? `
                    <div>
                        <strong>${statusT('MCP.description')}</strong>
                        <span>${escapeHtml(server.config.description)}</span>
                    </div>` : ''}
                    ${server.config.timeout ? `
                    <div>
                        <strong>${statusT('MCP.timeout')}</strong>
                        <span>${server.config.timeout} ${statusT('MCP.secondsUnit')}</span>
                    </div>` : ''}
                    ${transport === 'stdio' && server.config.command ? `
                    <div>
                        <strong>${statusT('MCP.command')}</strong>
                        <span style="font-family: monospace; font-size: 0.8125rem;">${escapeHtml(server.config.command)}</span>
                    </div>` : ''}
                    ${transport === 'http' && server.config.URL ? `
                    <div>
                        <strong>${statusT('MCP.urlLabel')}</strong>
                        <span style="font-family: monospace; font-size: 0.8125rem; word-break: break-all;">${escapeHtml(server.config.URL)}</span>
                    </div>` : ''}
                </div>
            </div>
        `;
    }
    html += '</div>';
    list.innerHTML = html;
    updateExternalMcpCardSelection();
}

// Render External MCP statistics INFO
function renderExternalMCPStats(stats) {
    const statsEl = document.getElementById('external-mcp-stats');
    if (!statsEl) return;
    
    const total = stats.total || 0;
    const enabled = stats.enabled || 0;
    const disabled = stats.disabled || 0;
    const connected = stats.connected || 0;
    
    const statsT = typeof window.t === 'function' ? window.t : (k) => k;
    statsEl.innerHTML = `
        <span title="${statsT('MCP.totalCount')}">📊 ${statsT('MCP.totalCount')}: <strong>${total}</strong></span>
        <span title="${statsT('MCP.enabledCount')}">✅ ${statsT('MCP.enabledCount')}: <strong>${enabled}</strong></span>
        <span title="${statsT('MCP.disabledCount')}">⏸ ${statsT('MCP.disabledCount')}: <strong>${disabled}</strong></span>
        <span title="${statsT('MCP.connectedCount')}">🔗 ${statsT('MCP.connectedCount')}: <strong>${connected}</strong></span>
    `;
}

// Show add external MCP modal
function showAddExternalMCPModal() {
    if (typeof requirePermission === 'function' && !requirePermission('MCP:write')) return;
    currenteditingMCPName = null;
    document.getElementById('external-mcp-modal-title').textContent = (typeof window.t === 'function' ? window.t('MCP.addExternalMCP') : 'add external MCP');
    document.getElementById('external-mcp-JSON').value = '';
    document.getElementById('external-mcp-JSON-error').style.display = 'none';
    document.getElementById('external-mcp-JSON-error').textContent = '';
    document.getElementById('external-mcp-JSON').classList.remove('error');
    openAppModal('external-mcp-modal');
}

// Close External MCP modal
function closeExternalMCPModal() {
    closeAppModal('external-mcp-modal');
    currenteditingMCPName = null;
}

// editExternal MCP
async function editExternalMCP(name) {
    try {
        currenteditingMCPName = name;
        document.getElementById('external-mcp-modal-title').textContent = (typeof window.t === 'function' ? window.t('MCP.editExternalMCP') : 'editExternal MCP');
        document.getElementById('external-mcp-JSON').value = '';
        document.getElementById('external-mcp-JSON-error').style.display = 'none';
        document.getElementById('external-mcp-JSON-error').textContent = '';
        document.getElementById('external-mcp-JSON').classList.remove('error');
        openAppModal('external-mcp-modal', { focus: false });
        const response = await apiFetch(`/api/external-MCP/${encodeURIComponent(name)}`);
        if (!response.ok) {
            throw new Error(typeof window.t === 'function' ? window.t('MCP.getConfigFailed') : 'Failed to fetch External MCP configuration');
        }
        const server = await response.json();
        const config = { ...server.config };
        delete config.tool_count;
        delete config.external_mcp_enable;
        const configObj = {};
        configObj[name] = config;
        const jsonStr = JSON.stringify(configObj, null, 2);
        deferModalContent(() => {
            document.getElementById('external-mcp-JSON').value = jsonStr;
            document.getElementById('external-mcp-JSON')?.focus();
        });
    } catch (error) {
        closeExternalMCPModal();
        console.error('editExternal MCPfailed:', error);
        alert((typeof window.t === 'function' ? window.t('MCP.operationFailed') : 'editfailed') + ': ' + error.message);
    }
}

// Format JSON
function formatExternalMCPJSON() {
    const jsonTextarea = document.getElementById('external-mcp-JSON');
    const errorDiv = document.getElementById('external-mcp-JSON-error');
    
    try {
        const jsonStr = jsonTextarea.value.trim();
        if (!jsonStr) {
            errorDiv.textContent = (typeof window.t === 'function' ? window.t('MCP.jsonEmpty') : 'JSONCannot be empty');
            errorDiv.style.display = 'block';
            jsonTextarea.classList.add('error');
            return;
        }
        
        const parsed = JSON.parse(jsonStr);
        const formatted = JSON.stringify(parsed, null, 2);
        jsonTextarea.value = formatted;
        errorDiv.style.display = 'none';
        jsonTextarea.classList.remove('error');
    } catch (error) {
        errorDiv.textContent = (typeof window.t === 'function' ? window.t('MCP.jsonError') : 'JSONInvalid format') + ': ' + error.message;
        errorDiv.style.display = 'block';
        jsonTextarea.classList.add('error');
    }
}

// Load example
function loadExternalMCPExample() {
    const example = {
        "my-stdio-server": {
            command: "python3",
            args: [
                "${HOME}/mcp-servers/main.py",
                "--port",
                "${MCP_PORT:-3000}"
            ],
            env: {
                "API_KEY": "${API_KEY}",
                "LOG_LEVEL": "${LOG_LEVEL:-INFO}"
            },
            timeout: 300
        },
        "my-http-server": {
            type: "http",
            URL: "https://MCP.example.com/MCP",
            headers: {
                "Authorization": "Bearer ${MCP_TOKEN}"
            }
        },
        "my-sse-server": {
            type: "sse",
            URL: "http://127.0.0.1:8081/MCP/sse"
        }
    };
    
    document.getElementById('external-mcp-JSON').value = JSON.stringify(example, null, 2);
    document.getElementById('external-mcp-JSON-error').style.display = 'none';
    document.getElementById('external-mcp-JSON').classList.remove('error');
}

// saveExternal MCP
async function saveExternalMCP() {
    if (typeof requirePermission === 'function' && !requirePermission('MCP:write')) return;
    const jsonTextarea = document.getElementById('external-mcp-JSON');
    const jsonStr = jsonTextarea.value.trim();
    const errorDiv = document.getElementById('external-mcp-JSON-error');
    
    if (!jsonStr) {
        errorDiv.textContent = (typeof window.t === 'function' ? window.t('MCP.jsonEmpty') : 'JSONCannot be empty');
        errorDiv.style.display = 'block';
        jsonTextarea.classList.add('error');
        jsonTextarea.focus();
        return;
    }
    
    let configObj;
    try {
        configObj = JSON.parse(jsonStr);
    } catch (error) {
        errorDiv.textContent = (typeof window.t === 'function' ? window.t('MCP.jsonError') : 'JSONInvalid format') + ': ' + error.message;
        errorDiv.style.display = 'block';
        jsonTextarea.classList.add('error');
        jsonTextarea.focus();
        return;
    }
    
    const t = (typeof window.t === 'function' ? window.t : function (k, opts) { return k; });
    // Validate that it must be object format
    if (typeof configObj !== 'object' || Array.isArray(configObj) || configObj === null) {
        errorDiv.textContent = t('MCP.configMustBeObject');
        errorDiv.style.display = 'block';
        jsonTextarea.classList.add('error');
        return;
    }
    
    // Get all configuration names
    const names = Object.keys(configObj);
    if (names.length === 0) {
        errorDiv.textContent = t('MCP.configNeedOne');
        errorDiv.style.display = 'block';
        jsonTextarea.classList.add('error');
        return;
    }
    
    // Validate each configuration
    for (const name of names) {
        if (!name || name.trim() === '') {
            errorDiv.textContent = t('MCP.configNameEmpty');
            errorDiv.style.display = 'block';
            jsonTextarea.classList.add('error');
            return;
        }
        
        const config = configObj[name];
        if (typeof config !== 'object' || Array.isArray(config) || config === null) {
            errorDiv.textContent = t('MCP.configMustBeObj', { name: name });
            errorDiv.style.display = 'block';
            jsonTextarea.classList.add('error');
            return;
        }
        
        // Remove the external_mcp_enable field (controlled by button, but keep enabled/disabled for backward compatibility)
        delete config.external_mcp_enable;
        
        // Validate configuration content (supports both the official "type" field and the legacy "transport" field)
        const transport = config.type || config.transport || (config.command ? 'stdio' : config.URL ? 'http' : '');
        if (!transport) {
            errorDiv.textContent = t('MCP.configNeedCommand', { name: name });
            errorDiv.style.display = 'block';
            jsonTextarea.classList.add('error');
            return;
        }
        
        if (transport === 'stdio' && !config.command) {
            errorDiv.textContent = t('MCP.configStdioNeedCommand', { name: name });
            errorDiv.style.display = 'block';
            jsonTextarea.classList.add('error');
            return;
        }
        
        if (transport === 'http' && !config.URL) {
            errorDiv.textContent = t('MCP.configHttpNeedUrl', { name: name });
            errorDiv.style.display = 'block';
            jsonTextarea.classList.add('error');
            return;
        }
        
        if (transport === 'sse' && !config.URL) {
            errorDiv.textContent = t('MCP.configSseNeedUrl', { name: name });
            errorDiv.style.display = 'block';
            jsonTextarea.classList.add('error');
            return;
        }
    }
    
    // Clear error hint
    errorDiv.style.display = 'none';
    jsonTextarea.classList.remove('error');
    
    try {
        // If in edit mode, only update the currently edited configuration
        if (currenteditingMCPName) {
            if (!configObj[currenteditingMCPName]) {
                errorDiv.textContent = (typeof window.t === 'function' ? window.t('MCP.configEditMustContainName', { name: currenteditingMCPName }) : 'Configuration error: in edit mode, the JSON must contain the configuration name "' + currenteditingMCPName + '"');
                errorDiv.style.display = 'block';
                jsonTextarea.classList.add('error');
                return;
            }
            
            const response = await apiFetch(`/api/external-MCP/${encodeURIComponent(currenteditingMCPName)}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ config: configObj[currenteditingMCPName] }),
            });
            
            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'save failed');
            }
        } else {
            // Add mode: save all configurations
            for (const name of names) {
                const config = configObj[name];
                const response = await apiFetch(`/api/external-MCP/${encodeURIComponent(name)}`, {
                    method: 'PUT',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ config }),
                });
                
                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(`save "${name}" failed: ${error.error || 'Unknown error'}`);
                }
            }
        }
        
        closeExternalMCPModal();
        await loadExternalMCPs();
        if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
            window.refreshMentionTools();
        }
        // Poll a few times to fetch the backend's asynchronously updated tool count (no fixed delay, stops when received)
        pollExternalMCPToolCount(null, 5);
        alert(typeof window.t === 'function' ? window.t('MCP.saveSuccess') : 'saved successfully');
    } catch (error) {
        console.error('saveExternal MCPfailed:', error);
        errorDiv.textContent = (typeof window.t === 'function' ? window.t('MCP.operationFailed') : 'save failed') + ': ' + error.message;
        errorDiv.style.display = 'block';
        jsonTextarea.classList.add('error');
    }
}

// deleteExternal MCP
async function deleteExternalMCP(name) {
    if (!confirm((typeof window.t === 'function' ? window.t('MCP.deleteExternalConfirm', { name: name }) : `Are you sure you want to delete External MCP "${name}"?`))) {
        return;
    }
    
    try {
        const response = await apiFetch(`/api/external-MCP/${encodeURIComponent(name)}`, {
            method: 'DELETE',
        });
        
        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'delete failed');
        }
        
        await loadExternalMCPs();
        // Refresh the Chat interface tool list, removing the deleted MCP tools
        if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
            window.refreshMentionTools();
        }
        alert(typeof window.t === 'function' ? window.t('MCP.deleteSuccess') : 'deleted successfully');
    } catch (error) {
        console.error('deleteExternal MCPfailed:', error);
        alert((typeof window.t === 'function' ? window.t('MCP.operationFailed') : 'delete failed') + ': ' + error.message);
    }
}

// Toggle External MCP start/stop
async function toggleExternalMCP(name, currentStatus) {
    const action = currentStatus === 'connected' ? 'stop' : 'start';
    const buttonId = `btn-toggle-${name}`;
    const button = document.getElementById(buttonId);
    
    // If starting, show loading state
    if (action === 'start' && button) {
        button.disabled = true;
        button.style.opacity = '0.6';
        button.style.cursor = 'not-allowed';
        button.innerHTML = '⏳ Connecting...';
    }
    
    try {
        const response = await apiFetch(`/api/external-MCP/${encodeURIComponent(name)}/${action}`, {
            method: 'POST',
        });
        
        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'Operation failed');
        }
        
        const result = await response.json();
        
        // If starting, do an immediate status check first
        if (action === 'start') {
            // Do an immediate status check (may already be connected)
            try {
                const statusResponse = await apiFetch(`/api/external-MCP/${encodeURIComponent(name)}`);
                if (statusResponse.ok) {
                    const statusData = await statusResponse.json();
                    const status = statusData.status || 'disconnected';
                    
                    if (status === 'connected') {
                        await loadExternalMCPs();
                        if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
                            window.refreshMentionTools();
                        }
                        // Poll until the MCP tool count has updated (every second, no fixed delay)
                        pollExternalMCPToolCount(name, 10);
                        await reloadMcpToolsAfterExternalChange(true);
                        return;
                    }
                }
            } catch (error) {
                console.error('Status check failed:', error);
            }
            
            // If still disconnected, start polling
            await pollExternalMCPStatus(name, 30); // poll at most 30 times (approx. 30 sec)
        } else {
            // Stop operation, refresh directly
            await loadExternalMCPs();
            await reloadMcpToolsAfterExternalChange(false);
            // Refresh Chat interface tool list
            if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
                window.refreshMentionTools();
            }
        }
    } catch (error) {
        console.error('Failed to toggle External MCP status:', error);
        alert((typeof window.t === 'function' ? window.t('MCP.operationFailed') : 'Operation failed') + ': ' + error.message);
        
        // Restore button state
        if (button) {
            button.disabled = false;
            button.style.opacity = '1';
            button.style.cursor = 'pointer';
            button.innerHTML = '▶ Start';
        }
        
        // refreshstatus
        await loadExternalMCPs();
        // Refresh Chat interface tool list
        if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
            window.refreshMentionTools();
        }
    }
}

// Poll External MCP status
async function pollExternalMCPStatus(name, maxAttempts = 30) {
    let attempts = 0;
    const pollInterval = 1000; // poll once per second
    
    while (attempts < maxAttempts) {
        await new Promise(resolve => setTimeout(resolve, pollInterval));
        
        try {
            const response = await apiFetch(`/api/external-MCP/${encodeURIComponent(name)}`);
            if (response.ok) {
                const data = await response.json();
                const status = data.status || 'disconnected';
                
                // Update button state
                const buttonId = `btn-toggle-${name}`;
                const button = document.getElementById(buttonId);
                
                if (status === 'connected') {
                    await loadExternalMCPs();
                    if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
                        window.refreshMentionTools();
                    }
                    // Poll until the MCP tool count has updated (every second, no fixed delay)
                    pollExternalMCPToolCount(name, 10);
                    await reloadMcpToolsAfterExternalChange(true);
                    return;
                } else if (status === 'error' || status === 'disconnected') {
                    // Connection failed; refresh list and show error
                    await loadExternalMCPs();
                    // Refresh Chat interface tool list
                    if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
                        window.refreshMentionTools();
                    }
                    if (status === 'error') {
                        alert(typeof window.t === 'function' ? window.t('MCP.connectionFailedCheck') : 'Connection failed. Please check configuration and network connection');
                    }
                    return;
                } else if (status === 'connecting') {
                    // Still connecting, continue polling
                    attempts++;
                    continue;
                }
            }
        } catch (error) {
            console.error('Status polling failed:', error);
        }
        
        attempts++;
    }
    
    // Timed out; refresh list
    await loadExternalMCPs();
    // Refresh Chat interface tool list
    if (typeof window !== 'undefined' && typeof window.refreshMentionTools === 'function') {
        window.refreshMentionTools();
    }
    alert(typeof window.t === 'function' ? window.t('MCP.connectionTimeout') : 'Connection timed out. Please check configuration and network connection');
}

// Load External MCP list when settings are opened
const originalOpenSettings = openSettings;
openSettings = async function() {
    await originalOpenSettings();
    await loadExternalMCPs();
};

// After a language switch, re-render MCP management page sections written by JS (innerHTML is not auto-updated by data-i18n)
document.addEventListener('languagechange', function () {
    try {
        const settingsPage = document.getElementById('page-settings');
        if (settingsPage) {
            initSettingsCustomSelects(settingsPage);
            refreshSettingsCustomSelects();
        }
        const mcpPage = document.getElementById('page-mcp-management');
        if (mcpPage && mcpPage.classList.contains('active')) {
            if (typeof loadExternalMCPs === 'function') {
                loadExternalMCPs({ forceRender: true }).catch(function () { /* ignore */ });
            }
            if (typeof updateToolsStats === 'function') {
                updateToolsStats().catch(function () { /* ignore */ });
            }
        }
    } catch (e) {
        console.warn('languagechange MCP refresh failed', e);
    }
});

window.initSettingsCustomSelects = initSettingsCustomSelects;
window.refreshSettingsCustomSelects = refreshSettingsCustomSelects;
window.closeAllSettingsCustomSelects = closeAllSettingsCustomSelects;
