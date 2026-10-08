// Roles-related functionality
function _t(key, opts) {
    if (typeof window.t === 'function') {
        try {
            var translated = window.t(key, opts);
            if (typeof translated === 'string' && translated && translated !== key) {
                return translated;
            }
        } catch (e) { /* ignore */ }
    }
    // Avoid exposing keys to users when i18n is not ready or key is missing (consistent with zh-CN default)
    if (key === 'roles.noDescription') return 'No description';
    if (key === 'roles.noDescriptionShort') return 'No description';
    if (key === 'roles.defaultRoleDescription') {
        return 'default role, no additional user hint words, uses default MCP';
    }
    return key;
}

const ROLE_MODAL_SELECT_IDS = ['role-workflow-id', 'role-workflow-policy'];
const roleModalSelectMap = {};
let roleModalSelectDocBound = false;
const ROLE_FORM_SELECT_CARET = '<svg class="role-form-select-caret" width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M6 9l6 6 6-6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>';

function closeAllRoleModalSelects() {
    Object.keys(roleModalSelectMap).forEach(function (id) {
        const reg = roleModalSelectMap[id];
        if (!reg || !reg.wrapper) return;
        reg.wrapper.classList.remove('open');
        if (reg.trigger) reg.trigger.setAttribute('aria-expanded', 'false');
    });
}

function syncRoleModalSelect(selectId) {
    const reg = roleModalSelectMap[selectId];
    if (!reg) return;
    const select = reg.select;
    const dropdown = reg.dropdown;
    const trigger = reg.trigger;
    const valueSpan = trigger.querySelector('.role-form-select-value');

    dropdown.innerHTML = '';
    Array.prototype.forEach.call(select.options, function (opt) {
        const item = document.createElement('button');
        item.type = 'button';
        item.className = 'role-form-select-option';
        item.setAttribute('role', 'option');
        item.setAttribute('data-value', opt.value);
        if (opt.value === select.value) {
            item.classList.add('is-selected');
            item.setAttribute('aria-selected', 'true');
        } else {
            item.setAttribute('aria-selected', 'false');
        }
        const check = document.createElement('span');
        check.className = 'role-form-select-check';
        check.setAttribute('aria-hidden', 'true');
        check.textContent = '✓';
        const label = document.createElement('span');
        label.className = 'role-form-select-label';
        label.textContent = opt.textContent;
        item.appendChild(check);
        item.appendChild(label);
        dropdown.appendChild(item);
    });

    const selectedOpt = select.options[select.selectedIndex];
    if (valueSpan) {
        valueSpan.textContent = selectedOpt ? selectedOpt.textContent : '';
    }
    trigger.disabled = !!select.disabled;
    reg.wrapper.classList.toggle('is-disabled', !!select.disabled);
}

function syncAllRoleModalSelects() {
    ROLE_MODAL_SELECT_IDS.forEach(syncRoleModalSelect);
}

function enhanceRoleModalSelect(selectId) {
    const select = document.getElementById(selectId);
    if (!select) return;
    const existing = roleModalSelectMap[selectId];
    if (existing && existing.select !== select) {
        delete roleModalSelectMap[selectId];
    }
    if (select.dataset.roleFormCustom === '1') {
        syncRoleModalSelect(selectId);
        return;
    }
    select.dataset.roleFormCustom = '1';
    select.classList.add('role-form-native-select');
    select.tabIndex = -1;
    select.setAttribute('aria-hidden', 'true');

    const wrapper = document.createElement('div');
    wrapper.className = 'role-form-select-ui';

    const trigger = document.createElement('button');
    trigger.type = 'button';
    trigger.className = 'role-form-select-trigger';
    trigger.setAttribute('aria-haspopup', 'listbox');
    trigger.setAttribute('aria-expanded', 'false');
    const valueSpan = document.createElement('span');
    valueSpan.className = 'role-form-select-value';
    trigger.appendChild(valueSpan);
    trigger.insertAdjacentHTML('beforeend', ROLE_FORM_SELECT_CARET);

    const dropdown = document.createElement('div');
    dropdown.className = 'role-form-select-dropdown';
    dropdown.setAttribute('role', 'listbox');

    const parent = select.parentNode;
    parent.insertBefore(wrapper, select);
    wrapper.appendChild(trigger);
    wrapper.appendChild(dropdown);
    wrapper.appendChild(select);

    roleModalSelectMap[selectId] = { wrapper: wrapper, trigger: trigger, dropdown: dropdown, select: select };

    trigger.addEventListener('click', function (e) {
        e.stopPropagation();
        if (select.disabled) return;
        const open = wrapper.classList.contains('open');
        closeAllRoleModalSelects();
        if (!open) {
            wrapper.classList.add('open');
            trigger.setAttribute('aria-expanded', 'true');
        }
    });

    dropdown.addEventListener('click', function (e) {
        const opt = e.target.closest('.role-form-select-option');
        if (!opt) return;
        e.stopPropagation();
        const val = opt.getAttribute('data-value');
        if (val === null) return;
        if (select.value !== val) {
            select.value = val;
            select.dispatchEvent(new Event('change', { bubbles: true }));
        }
        wrapper.classList.remove('open');
        trigger.setAttribute('aria-expanded', 'false');
        syncRoleModalSelect(selectId);
    });

    select.addEventListener('change', function () {
        syncRoleModalSelect(selectId);
    });

    syncRoleModalSelect(selectId);
}

function refreshRoleModalSelects() {
    const modal = document.getElementById('role-modal');
    if (!modal) return;
    Object.keys(roleModalSelectMap).forEach(function (id) {
        if (!document.getElementById(id)) delete roleModalSelectMap[id];
    });
    ROLE_MODAL_SELECT_IDS.forEach(enhanceRoleModalSelect);
    if (!roleModalSelectDocBound) {
        roleModalSelectDocBound = true;
        document.addEventListener('click', closeAllRoleModalSelects);
        document.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') closeAllRoleModalSelects();
        });
    }
}

/** Role configuration description: trim and treat literals accidentally stored as i18n keys as empty */
function rolePlainDescription(role) {
    const raw = typeof role.description === 'string' ? role.description.trim() : '';
    if (!raw) return '';
    if (raw === 'roles.noDescription' || raw === 'roles.noDescriptionShort') return '';
    return raw;
}

function initSelectionDetailTooltip() {
    if (window.__selectionDetailTooltipReady) return;
    window.__selectionDetailTooltipReady = true;

    const tooltip = document.createElement('div');
    tooltip.className = 'selection-detail-tooltip';
    tooltip.setAttribute('role', 'tooltip');
    document.body.appendChild(tooltip);

    let activeEl = null;

    function hideTooltip() {
        activeEl = null;
        tooltip.classList.remove('visible', 'placement-left');
    }

    function positionTooltip(el) {
        const text = el && el.getAttribute('data-selection-detail');
        if (!text) {
            hideTooltip();
            return;
        }
        activeEl = el;
        tooltip.textContent = text;
        tooltip.classList.add('visible');

        const rect = el.getBoundingClientRect();
        const tipRect = tooltip.getBoundingClientRect();
        const gap = 12;
        const margin = 12;
        const rightSpace = window.innerWidth - rect.right - gap - margin;
        const placeLeft = rightSpace < tipRect.width && rect.left > tipRect.width + gap + margin;
        const left = placeLeft ? rect.left - tipRect.width - gap : rect.right + gap;
        const top = Math.max(margin, Math.min(rect.top + rect.height / 2 - tipRect.height / 2, window.innerHeight - tipRect.height - margin));

        tooltip.classList.toggle('placement-left', placeLeft);
        tooltip.style.left = left + 'px';
        tooltip.style.top = top + 'px';
    }

    document.addEventListener('mouseover', function (event) {
        const el = event.target && event.target.closest && event.target.closest('[data-selection-detail]');
        if (!el || (event.relatedTarget && el.contains(event.relatedTarget))) return;
        positionTooltip(el);
    });
    document.addEventListener('mouseout', function (event) {
        const el = event.target && event.target.closest && event.target.closest('[data-selection-detail]');
        if (!el || (event.relatedTarget && el.contains(event.relatedTarget))) return;
        hideTooltip();
    });
    document.addEventListener('focusin', function (event) {
        const el = event.target && event.target.closest && event.target.closest('[data-selection-detail]');
        if (el) positionTooltip(el);
    });
    document.addEventListener('focusout', function (event) {
        const el = event.target && event.target.closest && event.target.closest('[data-selection-detail]');
        if (el) hideTooltip();
    });
    window.addEventListener('scroll', hideTooltip, true);
    window.addEventListener('resize', function () {
        if (activeEl) positionTooltip(activeEl);
    });
}

if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initSelectionDetailTooltip);
    } else {
        initSelectionDetailTooltip();
    }
}

let currentRole = localStorage.getItem('currentRole') || '';
let roles = [];
let rolessearchKeyword = ''; // Role search keyword
let rolessearchTimeout = null; // search debounce timer
let allRoleTools = []; // Store all tool list (used for role tool selection)
// Shared localStorage with MCP tool configuration for unified operational habits
function getRoleToolsPageSize() {
    const saved = localStorage.getItem('toolsPageSize');
    const n = saved ? parseInt(saved, 10) : 20;
    return isNaN(n) || n < 1 ? 20 : n;
}
// This role association filter: '' = all, 'role_on' = this role is checked, 'role_off' = this role is not associated
let roleToolsStatusfilter = '';
/** Cache the full list when filtering by role association (matching currentSearch), to avoid losing state on  page turn */
let roleToolsListCacheFull = [];
let roleToolsListCachesearch = '';
/** Whether to use client-side pagination (true in role association filter mode) */
let roleToolsClientMode = false;
let roleToolsPagination = {
     page: 1,
     pageSize: getRoleToolsPageSize(),
    total: 0,
    totalPages: 1
};
let roleToolssearchKeyword = ''; // Tool search keyword
let roleToolStateMap = new Map(); // Tool state map: toolKey -> { enabled: boolean, ... }
let roleUsesallTools = false; // Flag indicating if the role uses all tools (when no tools are configured)
let totalenabledToolsInMCP = 0; // Total count of enabled tools (obtained from MCP, from API response)
// Only update on results with 'no status filter, no search', used as denominator for statistics bar (avoid filter causing total to shrink, leading to errors like 25/9)
let roleToolsStatsGrandTotal = 0; // Total tool count (consistent with MCP list 'all')
let roleToolsStatsMcpenabledTotal = 0; // MCP globally enabled tool count
let roleConfiguredTools = new Set(); // Role-configured tool list (used to determine which tools should be selected)

// Sort role list: default role first, others sorted by name
function sortRoles(rolesArray) {
    const sortedRoles = [...rolesArray];
    // Separate the 'default' role
    const defaultRole = sortedRoles.find(r => r.name === 'default');
    const otherRoles = sortedRoles.filter(r => r.name !== 'default');
    
    // Sort other roles by name, maintain fixed order
    otherRoles.sort((a, b) => {
        const nameA = a.name || '';
        const nameB = b.name || '';
        return nameA.localeCompare(nameB, 'zh-CN');
    });
    
    // Put 'default' role first, followed by other roles in sorted order
    const result = defaultRole ? [defaultRole, ...otherRoles] : otherRoles;
    return result;
}

// Load all roles
async function loadRoles() {
    if (window.i18nReady && typeof window.i18nReady.then === 'function') {
        try {
            await window.i18nReady;
        } catch (e) { /* ignore */ }
    }
    try {
        const response = await apiFetch('/api/roles');
        if (!response.ok) {
            throw new Error('failed to load roles');
        }
        const data = await response.json();
        roles = data.roles || [];
        updateRoleSelectorDisplay();
        renderRoleSelectionSidebar(); // Render sidebar role list
        return roles;
    } catch (error) {
        console.error('failed to load roles:', error);
        // hint text uses i18n; if i18n is not yet initialized, fall back to readable Chinese rather than exposing the key (roles.loadFailed)
        var loadfailedLabel = (typeof window !== 'undefined' && typeof window.t === 'function')
            ? window.t('roles.loadFailed')
            : 'failed to load roles';
        showNotification(loadfailedLabel + ': ' + error.message, 'error');
        return [];
    }
}

// Handle role change
function handleRoleChange(roleName) {
    const oldRole = currentRole;
    currentRole = roleName || '';
    localStorage.setItem('currentRole', currentRole);
    updateRoleSelectorDisplay();
    renderRoleSelectionSidebar(); // update sidebar selected state
    
    // When the role switches, if the tool list is loaded, mark it for reload
    // This way, the next @ tool suggestion will reload the tool list with the new role
    if (oldRole !== currentRole && typeof window !== 'undefined') {
        // Set a flag to notify chat.js that the tool list needs to be reloaded
        window._mentionToolsRoleChanged = true;
    }
}

// update role selector display
function updateRoleSelectorDisplay() {
    const roleSelectorBtn = document.getElementById('role-selector-btn');
    const roleSelectorIcon = document.getElementById('role-selector-icon');
    const roleSelectorText = document.getElementById('role-selector-text');
    
    if (!roleSelectorBtn || !roleSelectorIcon || !roleSelectorText) return;

    let selectedRole;
    if (currentRole && currentRole !== 'default') {
        selectedRole = roles.find(r => r.name === currentRole);
    } else {
        selectedRole = roles.find(r => r.name === 'default');
    }

    if (selectedRole) {
        // Use the icon from configuration; use default icon if none
        let icon = selectedRole.icon || '🔵';
        // If icon is in Unicode escape format (\U0001F3C6), convert to emoji
        if (icon && typeof icon === 'string') {
            const unicodeMatch = icon.match(/^"?\\U([0-9A-F]{8})"?$/i);
            if (unicodeMatch) {
                try {
                    const codePoint = parseInt(unicodeMatch[1], 16);
                    icon = String.fromCodePoint(codePoint);
                } catch (e) {
                    // If conversion fails, use default icon
                    console.warn('Failed to convert icon Unicode escape:', icon, e);
                    icon = '🔵';
                }
            }
        }
        roleSelectorIcon.textContent = icon;
        const isdefaultRole = selectedRole.name === 'default' || !selectedRole.name;
        const displayName = isdefaultRole && typeof window.t === 'function'
            ? window.t('chat.defaultRole') : (selectedRole.name || (typeof window.t === 'function' ? window.t('chat.defaultRole') : 'default'));
        // For non-default roles, prevent the i18n data-i18n attribute from overwriting the text with "default"
        roleSelectorText.setAttribute('data-i18n-skip-text', isdefaultRole ? 'false' : 'true');
        roleSelectorText.textContent = displayName;
    } else {
        // defaultRole
        roleSelectorText.setAttribute('data-i18n-skip-text', 'false');
        roleSelectorIcon.textContent = '🔵';
        roleSelectorText.textContent = typeof window.t === 'function' ? window.t('chat.defaultRole') : 'default';
    }
}

// Render main content area role selection list
function renderRoleSelectionSidebar() {
    const roleList = document.getElementById('role-selection-list');
    if (!roleList) return;

    // clear list
    roleList.innerHTML = '';

    // Get icon from role configuration; use default if not configured
    function getRoleIcon(role) {
        if (role.icon) {
            // If icon is in Unicode escape format (\U0001F3C6), convert to emoji
            let icon = role.icon;
            // Check if it is in Unicode escape format (may contain quotes)
            const unicodeMatch = icon.match(/^"?\\U([0-9A-F]{8})"?$/i);
            if (unicodeMatch) {
                try {
                    const codePoint = parseInt(unicodeMatch[1], 16);
                    icon = String.fromCodePoint(codePoint);
                } catch (e) {
                    // If conversion fails, use original value
                    console.warn('Failed to convert icon Unicode escape:', icon, e);
                }
            }
            return icon;
        }
        // If no icon is configured, generate default icon from first character of role name
        // Use a generic default icon
        return '👤';
    }
    
    // Sort roles: default role first, others by name
    const sortedRoles = sortRoles(roles);
    
    // Only show enabled roles
    const enabledSortedRoles = sortedRoles.filter(r => r.enabled !== false);
    
    enabledSortedRoles.forEach(role => {
        const isdefaultRole = role.name === 'default';
        const isSelected = isdefaultRole ? (currentRole === '' || currentRole === 'default') : (currentRole === role.name);
        const roleItem = document.createElement('div');
        roleItem.className = 'role-selection-item-main' + (isSelected ? ' selected' : '');
        roleItem.setAttribute('role', 'option');
        roleItem.tabIndex = 0;
        roleItem.onclick = () => {
            selectRole(role.name);
            closeRoleSelectionPanel(); // Auto-close panel after selection
        };
        roleItem.onkeydown = (event) => {
            if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                roleItem.click();
            }
        };
        const icon = getRoleIcon(role);
        
        // Handle description for default role
        const plainDesc = rolePlainDescription(role);
        let description = plainDesc || _t('roles.noDescription');
        if (isdefaultRole && !plainDesc) {
            description = _t('roles.defaultRoleDescription');
        }
        roleItem.setAttribute('data-selection-detail', description);
        
        roleItem.innerHTML = `
            <div class="role-selection-item-icon-main">${icon}</div>
            <div class="role-selection-item-content-main">
                <div class="role-selection-item-name-main">${escapeHtml(role.name)}</div>
                <div class="role-selection-item-description-main">${escapeHtml(description)}</div>
            </div>
            ${isSelected ? '<div class="role-selection-checkmark-main">✓</div>' : ''}
        `;
        roleList.appendChild(roleItem);
    });
}

// Select role
function selectRole(roleName) {
    // Map "default" to empty string (represents the default role)
    if (roleName === 'default') {
        roleName = '';
    }
    handleRoleChange(roleName);
    renderRoleSelectionSidebar(); // Re-render to update selection state
}

function getChatRoleSelectorWrapper() {
    return document.getElementById('role-selector-wrapper')
        || document.getElementById('role-selector-btn')?.closest('.role-selector-wrapper:not(.project-selector-wrapper)');
}

function isRoleSelectionPanelOpen() {
    const panel = document.getElementById('role-selection-panel');
    if (!panel) return false;
    return panel.style.display !== 'none' && panel.style.display !== '';
}

// Toggle role selection panel visibility
function toggleRoleSelectionPanel() {
    const panel = document.getElementById('role-selection-panel');
    const roleSelectorBtn = document.getElementById('role-selector-btn');
    if (!panel) return;
    
    const isHidden = !isRoleSelectionPanelOpen();
    
    if (isHidden) {
        if (typeof closeAgentModePanel === 'function') {
            closeAgentModePanel();
        }
        if (typeof closeChatProjectPanel === 'function') {
            closeChatProjectPanel();
        }
        if (typeof closeChatReasoningPanel === 'function') {
            closeChatReasoningPanel();
        }
        renderRoleSelectionSidebar();
        panel.style.display = 'flex'; // Use flex layout
        // Add visual feedback for open state
        if (roleSelectorBtn) {
            roleSelectorBtn.classList.add('active');
            roleSelectorBtn.setAttribute('aria-expanded', 'true');
        }
        
        // Ensure position is checked after panel has rendered
        setTimeout(() => {
            const wrapper = getChatRoleSelectorWrapper();
            if (wrapper) {
                const rect = wrapper.getBoundingClientRect();
                const panelHeight = panel.offsetHeight || 400;
                const viewportHeight = window.innerHeight;
                
                // If panel top exceeds viewport, scroll to a suitable position
                if (rect.top - panelHeight < 0) {
                    const scrollY = window.scrollY + rect.top - panelHeight - 20;
                    window.scrollTo({ top: Math.max(0, scrollY), behavior: 'smooth' });
                }
            }
        }, 10);
    } else {
        closeRoleSelectionPanel();
    }
}

// Close role selection panel (called automatically after role selection)
function closeRoleSelectionPanel() {
    const panel = document.getElementById('role-selection-panel');
    const roleSelectorBtn = document.getElementById('role-selector-btn');
    if (panel) {
        panel.style.display = 'none';
    }
    if (roleSelectorBtn) {
        roleSelectorBtn.classList.remove('active');
        roleSelectorBtn.setAttribute('aria-expanded', 'false');
    }
}

// Escape HTML
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function escapeJsString(text) {
    return JSON.stringify(String(text == null ? '' : text));
}

function escapeAttr(text) {
    return escapeHtml(text).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function escapeJsStringAttr(text) {
    return escapeAttr(escapeJsString(text));
}

// refreshRole list
async function refreshRoles() {
    await loadRoles();
    // Check whether current page is the Roles page
    const currentPage = typeof window.currentPage === 'function' ? window.currentPage() : (window.currentPage || 'chat');
    if (currentPage === 'roles-management') {
        renderRolesList();
    }
    // Always update the sidebar role selection list
    renderRoleSelectionSidebar();
    showNotification('Refreshed', 'success');
}

// Render role list
function renderRolesList() {
    const rolesList = document.getElementById('roles-list');
    if (!rolesList) return;

    // Filter roles (by search keyword)
    let filteredRoles = roles;
    if (rolessearchKeyword) {
        const keyword = rolessearchKeyword.toLowerCase();
        filteredRoles = roles.filter(role => 
            role.name.toLowerCase().includes(keyword) ||
            (role.description && role.description.toLowerCase().includes(keyword))
        );
    }

    if (filteredRoles.length === 0) {
        rolesList.innerHTML = '<div class="empty-state">' + 
            (rolessearchKeyword ? _t('roles.noMatchingRoles') : _t('roles.noRoles')) + 
            '</div>';
        return;
    }

    // Sort roles: default role first, others by name
    const sortedRoles = sortRoles(filteredRoles);
    
    rolesList.innerHTML = sortedRoles.map(role => {
        const plainDesc = rolePlainDescription(role);
        // Get role icon; if in Unicode escape format, convert to emoji
        let roleIcon = role.icon || '👤';
        if (roleIcon && typeof roleIcon === 'string') {
            // Check if it is in Unicode escape format (may contain quotes)
            const unicodeMatch = roleIcon.match(/^"?\\U([0-9A-F]{8})"?$/i);
            if (unicodeMatch) {
                try {
                    const codePoint = parseInt(unicodeMatch[1], 16);
                    roleIcon = String.fromCodePoint(codePoint);
                } catch (e) {
                    // If conversion fails, use default icon
                    console.warn('Failed to convert icon Unicode escape:', roleIcon, e);
                    roleIcon = '👤';
                }
            }
        }

        // Get tool list display text
        let toolsDisplay = '';
        let toolsCount = 0;
        if (role.name === 'default') {
            toolsDisplay = _t('roleModal.usingAllTools');
        } else if (role.tools && role.tools.length > 0) {
            toolsCount = role.tools.length;
            // Show first 5 tool names
            const toolNames = role.tools.slice(0, 5).map(tool => {
                // If it is an external tool (format: external_mcp::tool_name), show only the tool name
                const toolName = tool.includes('::') ? tool.split('::')[1] : tool;
                return escapeHtml(toolName);
            });
            if (toolsCount <= 5) {
                toolsDisplay = toolNames.join(', ');
            } else {
                toolsDisplay = toolNames.join(', ') + _t('roleModal.andNMore', { count: toolsCount });
            }
        } else if (role.mcps && role.mcps.length > 0) {
            toolsCount = role.mcps.length;
            toolsDisplay = _t('roleModal.andNMore', { count: toolsCount });
        } else {
            toolsDisplay = _t('roleModal.usingAllTools');
        }

        return `
        <div class="role-card">
            <div class="role-card-header">
                <h3 class="role-card-title">
                    <span class="role-card-icon">${roleIcon}</span>
                    ${escapeHtml(role.name)}
                </h3>
                <span class="role-card-badge ${role.enabled !== false ? 'enabled' : 'disabled'}">
                    ${role.enabled !== false ? _t('roles.enabled') : _t('roles.disabled')}
                </span>
            </div>
            <div class="role-card-description">${escapeHtml(plainDesc || _t('roles.noDescriptionShort'))}</div>
            <div class="role-card-tools">
                <span class="role-card-tools-label">${_t('roleModal.toolsLabel')}</span>
                <span class="role-card-tools-value">${toolsDisplay}</span>
            </div>
            <div class="role-card-actions">
                <button class="btn-secondary btn-small" onclick="editRole(${escapeJsStringAttr(role.name)})">${_t('common.edit')}</button>
                ${role.name !== 'default' ? `<button class="btn-secondary btn-small btn-danger" onclick="deleteRole(${escapeJsStringAttr(role.name)})">${_t('common.delete')}</button>` : ''}
            </div>
        </div>
    `;
    }).join('');
}

// Handle role search input
function handleRolesSearchInput() {
    clearTimeout(rolessearchTimeout);
    rolessearchTimeout = setTimeout(() => {
        searchRoles();
    }, 300);
}

// searchRole
function searchRoles() {
    const searchInput = document.getElementById('roles-search');
    if (!searchInput) return;
    
    rolessearchKeyword = searchInput.value.trim();
    const clearBtn = document.getElementById('roles-search-clear');
    if (clearBtn) {
        clearBtn.style.display = rolessearchKeyword ? 'block' : 'none';
    }
    
    renderRolesList();
}

// Clear role search
function clearRolesSearch() {
    const searchInput = document.getElementById('roles-search');
    if (searchInput) {
        searchInput.value = '';
    }
    rolessearchKeyword = '';
    const clearBtn = document.getElementById('roles-search-clear');
    if (clearBtn) {
        clearBtn.style.display = 'none';
    }
    renderRolesList();
}

// Generate unique tool identifier (consistent with getToolKey in settings.js)
function getToolKey(tool) {
    // If it is an external tool, use external_mcp::tool.name as the unique identifier
    if (tool.is_external && tool.external_mcp) {
        return `${tool.external_mcp}::${tool.name}`;
    }
    // Built-in tools use the tool name directly
    return tool.name;
}

// Merge a single tool into roleToolStateMap (consistent with the single-record logic in loadRoleTools)
function mergeToolIntoRoleStateMap(tool) {
    const toolKey = getToolKey(tool);
    if (!roleToolStateMap.has(toolKey)) {
        let enabled = false;
        if (roleUsesallTools) {
            enabled = tool.enabled ? true : false;
        } else {
            enabled = roleConfiguredTools.has(toolKey);
        }
        roleToolStateMap.set(toolKey, {
            enabled: enabled,
            is_external: tool.is_external || false,
            external_mcp: tool.external_mcp || '',
            name: tool.name,
            mcpEnabled: tool.enabled
        });
    } else {
        const state = roleToolStateMap.get(toolKey);
        if (roleUsesallTools && tool.enabled) {
            state.enabled = true;
        }
        state.is_external = tool.is_external || false;
        state.external_mcp = tool.external_mcp || '';
        state.mcpEnabled = tool.enabled;
        if (!state.name || state.name === toolKey.split('::').pop()) {
            state.name = tool.name;
        }
    }
}

function getRoleLinkedForTool(toolKey, tool) {
    if (roleToolStateMap.has(toolKey)) {
        return !!roleToolStateMap.get(toolKey).enabled;
    }
    if (roleUsesallTools) {
        return tool.enabled !== false;
    }
    return roleConfiguredTools.has(toolKey);
}

function computeRoleLinkFilteredTools() {
    if (!roleToolsListCacheFull.length) {
        return [];
    }
    return roleToolsListCacheFull.filter(tool => {
        const key = getToolKey(tool);
        const linked = getRoleLinkedForTool(key, tool);
        if (roleToolsStatusfilter === 'role_on') {
            return linked;
        }
        if (roleToolsStatusfilter === 'role_off') {
            return !linked;
        }
        return true;
    });
}

async function fetchAllRoleToolsIntoCache(searchKeyword) {
    const pageSize = 100;
    let  page = 1;
    const all = [];
    let totalPages = 1;
    do {
        let url = `/api/config/tools?page=${ page}&page_size=${ pageSize}`;
        if (searchKeyword) {
            url += `&search=${encodeURIComponent(searchKeyword)}`;
        }
        const response = await apiFetch(url);
        if (!response.ok) {
            throw new Error('Failed to get tool list');
        }
        const result = await response.json();
        const tools = result.tools || [];
        tools.forEach(tool => mergeToolIntoRoleStateMap(tool));
        all.push(...tools);
        totalPages = Math.max(1, result.total_pages || 1);
         page++;
    } while ( page <= totalPages);
    roleToolsListCacheFull = all;
    roleToolsStatsGrandTotal = all.length;
    roleToolsStatsMcpenabledTotal = all.filter(t => t.enabled !== false).length;
    totalenabledToolsInMCP = roleToolsStatsMcpenabledTotal;
}

// Save current page tool state to the global map
function saveCurrentRolePageToolStates() {
    document.querySelectorAll('#role-tools-list .role-tool-item').forEach(item => {
        const toolKey = item.dataset.toolKey;
        const checkbox = item.querySelector('input[type="checkbox"]');
        if (toolKey && checkbox) {
            const toolName = item.dataset.toolName;
            const isExternal = item.dataset.isExternal === 'true';
            const externalMcp = item.dataset.externalMcp || '';
            const existingState = roleToolStateMap.get(toolKey);
            roleToolStateMap.set(toolKey, {
                enabled: checkbox.checked,
                is_external: isExternal,
                external_mcp: externalMcp,
                name: toolName,
                mcpEnabled: existingState ? existingState.mcpEnabled : true // Preserve MCP enabled state
            });
        }
    });
}

// Load all tool list (for role tool selection)
async function loadRoleTools( page = 1, searchKeyword = '') {
    try {
        // Save current page state to the global map before loading a new page
        saveCurrentRolePageToolStates();

        const pageSize = roleToolsPagination. pageSize;
        const needRoleLinkfilter =
            roleToolsStatusfilter === 'role_on' || roleToolsStatusfilter === 'role_off';

        if (needRoleLinkfilter) {
            roleToolsClientMode = true;
            const searchChanged = searchKeyword !== roleToolsListCachesearch;
            if (searchChanged || roleToolsListCacheFull.length === 0) {
                await fetchAllRoleToolsIntoCache(searchKeyword);
                roleToolsListCachesearch = searchKeyword;
            }
            const filtered = computeRoleLinkFilteredTools();
            const total = filtered.length;
            let totalPages = Math.max(1, Math.ceil(total /  pageSize) || 1);
            let p =  page;
            if (p > totalPages) {
                p = totalPages;
            }
            if (p < 1) {
                p = 1;
            }
            roleToolsPagination = {
                 page: p,
                 pageSize,
                total,
                totalPages
            };
            allRoleTools = filtered.slice((p - 1) *  pageSize, p *  pageSize);
        } else {
            roleToolsClientMode = false;
            roleToolsListCacheFull = [];
            roleToolsListCachesearch = '';

            let url = `/api/config/tools?page=${ page}&page_size=${ pageSize}`;
            if (searchKeyword) {
                url += `&search=${encodeURIComponent(searchKeyword)}`;
            }

            const response = await apiFetch(url);
            if (!response.ok) {
                throw new Error('Failed to get tool list');
            }

            const result = await response.json();
            allRoleTools = result.tools || [];
            roleToolsPagination = {
                 page: result. page ||  page,
                 pageSize: result. page_size ||  pageSize,
                total: result.total || 0,
                totalPages: result.total_pages || 1
            };

            if (roleToolsStatusfilter === '' && !searchKeyword) {
                roleToolsStatsGrandTotal = result.total || 0;
                if (result.total_enabled !== undefined) {
                    roleToolsStatsMcpenabledTotal = result.total_enabled;
                    totalenabledToolsInMCP = result.total_enabled;
                }
            }

            allRoleTools.forEach(tool => mergeToolIntoRoleStateMap(tool));
        }

        renderRoleToolsList();
        renderRoleToolsPagination();
        updateRoleToolsStats();
    } catch (error) {
        console.error('Failed to load tool list:', error);
        const toolsList = document.getElementById('role-tools-list');
        if (toolsList) {
            toolsList.innerHTML = `<div class="tools-error">${_t('roleModal.loadToolsFailed')}: ${escapeHtml(error.message)}</div>`;
        }
    }
}

// Render role tool selection list
function renderRoleToolsList() {
    const toolsList = document.getElementById('role-tools-list');
    if (!toolsList) return;
    
    // Clear loading indicator and old content
    toolsList.innerHTML = '';

    if (roleToolsStatusfilter === 'role_on') {
        const banner = document.createElement('div');
        banner.className = 'role-tools-filter-banner role-tools-filter-banner-on';
        banner.setAttribute('role', 'status');
        banner.textContent = _t('roleModal.roleFilterOnBanner');
        toolsList.appendChild(banner);
    } else if (roleToolsStatusfilter === 'role_off') {
        const banner = document.createElement('div');
        banner.className = 'role-tools-filter-banner role-tools-filter-banner-off';
        banner.setAttribute('role', 'status');
        banner.textContent = _t('roleModal.roleFilterOffBanner');
        toolsList.appendChild(banner);
    }
    
    const listContainer = document.createElement('div');
    listContainer.className = 'role-tools-list- items';
    listContainer.innerHTML = '';
    
    if (allRoleTools.length === 0) {
        listContainer.innerHTML = '<div class="tools-empty">' + _t('roleModal.noTools') + '</div>';
        toolsList.appendChild(listContainer);
        return;
    }

    const chkTitle = escapeAttr(_t('roleModal.checkboxLinkTitle'));
    
    allRoleTools.forEach(tool => {
        const toolKey = getToolKey(tool);
        const toolItem = document.createElement('div');
        toolItem.className = 'role-tool-item';
        toolItem.dataset.toolKey = toolKey;
        toolItem.dataset.toolName = tool.name;
        toolItem.dataset.isExternal = tool.is_external ? 'true' : 'false';
        toolItem.dataset.externalMcp = tool.external_mcp || '';
        
        // Get tool state from the state map
        const toolState = roleToolStateMap.get(toolKey) || {
            enabled: tool.enabled,
            is_external: tool.is_external || false,
            external_mcp: tool.external_mcp || ''
        };
        
        // Externaltooltags
        let externalBadge = '';
        if (toolState.is_external || tool.is_external) {
            const externalMcpName = toolState.external_mcp || tool.external_mcp || '';
            const badgeText = externalMcpName ? `External (${escapeHtml(externalMcpName)})` : 'External';
            const badgeTitle = externalMcpName ? `External MCPTool - Source: ${escapeHtml(externalMcpName)}` : 'External MCPtool';
            externalBadge = `<span class="external-tool-badge" title="${escapeAttr(badgeTitle)}">${badgeText}</span>`;
        }
        let mcpdisabledBadge = '';
        if (tool.enabled === false) {
            mcpdisabledBadge = `<span class="role-tool-mcp-disabled-badge" title="${escapeHtml(_t('roleModal.mcpDisabledBadgeTitle'))}">${escapeHtml(_t('roleModal.mcpDisabledBadge'))}</span>`;
        }
        // Generate unique checkbox id
        const checkboxId = `role-tool-${escapeAttr(toolKey).replace(/::/g, '--')}`;
        
        toolItem.innerHTML = `
            <input type="checkbox" id="${checkboxId}" ${toolState.enabled ? 'checked' : ''} 
                   title="${chkTitle}" aria-label="${chkTitle}"
                   onchange="handleRoleToolCheckboxChange(${escapeJsStringAttr(toolKey)}, this.checked)" />
            <div class="role-tool-item-info">
                <div class="role-tool-item-name">
                    ${escapeHtml(tool.name)}
                    ${externalBadge}
                    ${mcpdisabledBadge}
                </div>
                <div class="role-tool-item-desc">${escapeHtml(tool.description || 'No description')}</div>
            </div>
        `;
        listContainer.appendChild(toolItem);
    });
    
    toolsList.appendChild(listContainer);
}

// Render tool list pagination controls (always show range and per-page count, so page size can be adjusted even on a single page)
function renderRoleToolsPagination() {
    const toolsList = document.getElementById('role-tools-list');
    if (!toolsList) return;
    
    // Remove old pagination controls
    const oldPagination = toolsList.querySelector('.role-tools-pagination');
    if (oldPagination) {
        oldPagination.remove();
    }
    
    const pagination = document.createElement('div');
    pagination.className = 'role-tools-pagination';
    
    const {  page, totalPages, total,  pageSize } = roleToolsPagination;
    const startItem = total === 0 ? 0 : ( page - 1) *  pageSize + 1;
    const endItem = total === 0 ? 0 : Math.min( page *  pageSize, total);
    const savedPageSize = getRoleToolsPageSize();
    const perPageLabel = typeof window.t === 'function' ? window.t('MCP.perPage') : 'Per page';
    
    const paginationShowText = _t('roleModal.paginationShow', { start: startItem, end: endItem, total: total }) +
        (roleToolssearchKeyword ? _t('roleModal.paginationSearch', { keyword: roleToolssearchKeyword }) : '');
    const navDisabled = total === 0 || totalPages <= 1;
    pagination.innerHTML = `
        <div class="pagination-info">${paginationShowText}</div>
        <div class="pagination-page-size">
            <label for="role-tools- page-size-pagination">${escapeHtml(perPageLabel)}</label>
            <select id="role-tools- page-size-pagination" onchange="changeRoleToolsPageSize()">
                <option value="10" ${savedPageSize === 10 ? 'selected' : ''}>10</option>
                <option value="20" ${savedPageSize === 20 ? 'selected' : ''}>20</option>
                <option value="50" ${savedPageSize === 50 ? 'selected' : ''}>50</option>
                <option value="100" ${savedPageSize === 100 ? 'selected' : ''}>100</option>
            </select>
        </div>
        <div class="pagination-controls">
            <button class="btn-secondary" onclick="loadRoleTools(1, ${escapeJsStringAttr(roleToolssearchKeyword)})" ${ page === 1 || navDisabled ? 'disabled' : ''}>${_t('roleModal.firstPage')}</button>
            <button class="btn-secondary" onclick="loadRoleTools(${ page - 1}, ${escapeJsStringAttr(roleToolssearchKeyword)})" ${ page === 1 || navDisabled ? 'disabled' : ''}>${_t('roleModal.prevPage')}</button>
            <span class="pagination-page">${_t('roleModal.pageOf', {  page:  page, total: totalPages })}</span>
            <button class="btn-secondary" onclick="loadRoleTools(${ page + 1}, ${escapeJsStringAttr(roleToolssearchKeyword)})" ${ page === totalPages || navDisabled ? 'disabled' : ''}>${_t('roleModal.nextPage')}</button>
            <button class="btn-secondary" onclick="loadRoleTools(${totalPages}, ${escapeJsStringAttr(roleToolssearchKeyword)})" ${ page === totalPages || navDisabled ? 'disabled' : ''}>${_t('roleModal.lastPage')}</button>
        </div>
    `;
    
    toolsList.appendChild(pagination);
}

function syncRoleToolsFilterButtons() {
    const wrap = document.getElementById('role-tools-status-filter');
    if (!wrap) return;
    wrap.querySelectorAll('.btn-filter').forEach(btn => {
        const v = btn.getAttribute('data-filter');
        const filterVal = v === null || v === undefined ? '' : String(v);
        btn.classList.toggle('active', filterVal === roleToolsStatusfilter);
    });
}

function roleToolsListScopeLine() {
    const n = roleToolsPagination.total || 0;
    if (roleToolsStatusfilter === 'role_on') {
        return _t('roleModal.statsListScopeRoleOn', { n: n });
    }
    if (roleToolsStatusfilter === 'role_off') {
        return _t('roleModal.statsListScopeRoleOff', { n: n });
    }
    return _t('roleModal.statsListScopeAll', { n: n });
}

function filterRoleToolsByStatus(status) {
    roleToolsStatusfilter = status;
    syncRoleToolsFilterButtons();
    loadRoleTools(1, roleToolssearchKeyword);
}

async function changeRoleToolsPageSize() {
    const sel = document.getElementById('role-tools- page-size-pagination');
    if (!sel) return;
    const newPageSize = parseInt(sel.value, 10);
    if (isNaN(newPageSize) || newPageSize < 1) return;
    localStorage.setItem('toolsPageSize', String(newPageSize));
    roleToolsPagination. pageSize = newPageSize;
    await loadRoleTools(1, roleToolssearchKeyword);
}

// Handle tool checkbox state change
function handleRoleToolCheckboxChange(toolKey, enabled) {
    const toolItem = document.querySelector(`.role-tool-item[data-tool-key="${toolKey}"]`);
    if (toolItem) {
        const toolName = toolItem.dataset.toolName;
        const isExternal = toolItem.dataset.isExternal === 'true';
        const externalMcp = toolItem.dataset.externalMcp || '';
        const existingState = roleToolStateMap.get(toolKey);
        roleToolStateMap.set(toolKey, {
            enabled: enabled,
            is_external: isExternal,
            external_mcp: externalMcp,
            name: toolName,
            mcpEnabled: existingState ? existingState.mcpEnabled : true // Preserve MCP enabled state
        });
    }
    if (
        roleToolsClientMode &&
        (roleToolsStatusfilter === 'role_on' || roleToolsStatusfilter === 'role_off')
    ) {
        loadRoleTools(roleToolsPagination. page, roleToolssearchKeyword);
    } else {
        updateRoleToolsStats();
    }
}

// Select alltool
function selectAllRoleTools() {
    document.querySelectorAll('#role-tools-list input[type="checkbox"]').forEach(checkbox => {
        const toolItem = checkbox.closest('.role-tool-item');
        if (toolItem) {
            const toolKey = toolItem.dataset.toolKey;
            const toolName = toolItem.dataset.toolName;
            const isExternal = toolItem.dataset.isExternal === 'true';
            const externalMcp = toolItem.dataset.externalMcp || '';
            if (toolKey) {
                const existingState = roleToolStateMap.get(toolKey);
                // Only select tools enabled in MCP
                const shouldEnable = existingState && existingState.mcpEnabled !== false;
                checkbox.checked = shouldEnable;
                roleToolStateMap.set(toolKey, {
                    enabled: shouldEnable,
                    is_external: isExternal,
                    external_mcp: externalMcp,
                    name: toolName,
                    mcpEnabled: existingState ? existingState.mcpEnabled : true
                });
            }
        }
    });
    if (
        roleToolsClientMode &&
        (roleToolsStatusfilter === 'role_on' || roleToolsStatusfilter === 'role_off')
    ) {
        loadRoleTools(roleToolsPagination. page, roleToolssearchKeyword);
    } else {
        updateRoleToolsStats();
    }
}

// Deselect all tools
function deselectAllRoleTools() {
    document.querySelectorAll('#role-tools-list input[type="checkbox"]').forEach(checkbox => {
        checkbox.checked = false;
        const toolItem = checkbox.closest('.role-tool-item');
        if (toolItem) {
            const toolKey = toolItem.dataset.toolKey;
            const toolName = toolItem.dataset.toolName;
            const isExternal = toolItem.dataset.isExternal === 'true';
            const externalMcp = toolItem.dataset.externalMcp || '';
            if (toolKey) {
                const existingState = roleToolStateMap.get(toolKey);
                roleToolStateMap.set(toolKey, {
                    enabled: false,
                    is_external: isExternal,
                    external_mcp: externalMcp,
                    name: toolName,
                    mcpEnabled: existingState ? existingState.mcpEnabled : true // Preserve MCP enabled state
                });
            }
        }
    });
    if (
        roleToolsClientMode &&
        (roleToolsStatusfilter === 'role_on' || roleToolsStatusfilter === 'role_off')
    ) {
        loadRoleTools(roleToolsPagination. page, roleToolssearchKeyword);
    } else {
        updateRoleToolsStats();
    }
}

// searchtool
function searchRoleTools(keyword) {
    roleToolssearchKeyword = keyword;
    const clearBtn = document.getElementById('role-tools-search-clear');
    if (clearBtn) {
        clearBtn.style.display = keyword ? 'block' : 'none';
    }
    loadRoleTools(1, keyword);
}

// Clear search
function clearRoleToolsSearch() {
    document.getElementById('role-tools-search').value = '';
    searchRoleTools('');
}

// Update tool statistics (denominator "max linkable" = total MCP-enabled tools in the library, matching the MCP management page "MCP enabled" filter count; checked = linked to this role)
function updateRoleToolsStats() {
    const statsEl = document.getElementById('role-tools-stats');
    if (!statsEl) return;

    const  pageChecked = Array.from(document.querySelectorAll('#role-tools-list input[type="checkbox"]:checked')).length;
    const  pageTotal = document.querySelectorAll('#role-tools-list input[type="checkbox"]').length;
    const mcpOnMax =
        (roleToolsStatsMcpenabledTotal > 0 ? roleToolsStatsMcpenabledTotal : totalenabledToolsInMCP) || 0;
    const grandAll =
        (roleToolsStatsGrandTotal > 0 ? roleToolsStatsGrandTotal : roleToolsPagination.total) || 0;
    const scopeLine = roleToolsListScopeLine();

    if (roleUsesallTools) {
        statsEl.innerHTML = `
            <div class="role-tools-stats-row">
                <span title="${escapeHtml(_t('roleModal.statsPageLinkedTitle'))}">✅ ${_t('roleModal.statsPageLinked', { current:  pageChecked, total:  pageTotal })}</span>
            </div>
            <div class="role-tools-stats-row">
                <span title="${escapeHtml(_t('roleModal.statsRoleUsesAllTitle'))}">📊 ${_t('roleModal.statsRoleUsesAll', { mcpOn: mcpOnMax, all: grandAll })}</span>
            </div>
            <div class="role-tools-stats-hint">📋 ${escapeHtml(scopeLine)}</div>
        `;
        return;
    }

    let roleLinked = 0;
    roleToolStateMap.forEach(state => {
        if (state.enabled && state.mcpEnabled !== false) {
            roleLinked++;
        }
    });
    document.querySelectorAll('#role-tools-list input[type="checkbox"]').forEach(checkbox => {
        const toolItem = checkbox.closest('.role-tool-item');
        if (toolItem) {
            const toolKey = toolItem.dataset.toolKey;
            const savedState = roleToolStateMap.get(toolKey);
            if (savedState && savedState.enabled !== checkbox.checked && savedState.mcpEnabled !== false) {
                if (checkbox.checked && !savedState.enabled) {
                    roleLinked++;
                } else if (!checkbox.checked && savedState.enabled) {
                    roleLinked--;
                }
            }
        }
    });

    const roleRow =
        mcpOnMax > 0
            ? `<span title="${escapeHtml(_t('roleModal.statsRoleLinkedTitle'))}">📊 ${_t('roleModal.statsRoleLinked', { current: roleLinked, max: mcpOnMax })}</span>`
            : `<span title="${escapeHtml(_t('roleModal.statsRoleLinkedNoMaxTitle'))}">📊 ${_t('roleModal.statsRoleLinkedNoMax', { current: roleLinked })}</span>`;

    statsEl.innerHTML = `
        <div class="role-tools-stats-row">
            <span title="${escapeHtml(_t('roleModal.statsPageLinkedTitle'))}">✅ ${_t('roleModal.statsPageLinked', { current:  pageChecked, total:  pageTotal })}</span>
        </div>
        <div class="role-tools-stats-row">${roleRow}</div>
        <div class="role-tools-stats-hint">📋 ${escapeHtml(scopeLine)}</div>
    `;
}

// Get selected tool list (returns toolKey array)
async function getSelectedRoleTools() {
    // Save current page state first
    saveCurrentRolePageToolStates();
    
    // If there is no search keyword, all pages' tools need to be loaded to ensure the state map is complete.
    // However, for performance we can just get selected tools from the state map.
    // The issue is: if the user only selected tools on certain pages, other pages' tool states may not be in the map.
    
    // If the total tool count exceeds the loaded count, we need to ensure unloaded pages are also considered.
    // But for role tool selection, we only need tools the user has explicitly selected.
    // So we can directly get selected tools from the state map.
    
    // Get all selected tools from the state map (only return tools enabled in MCP)
    const selectedTools = [];
    roleToolStateMap.forEach((state, toolKey) => {
        // Only return tools that are both MCP-enabled and selected for this role
        if (state.enabled && state.mcpEnabled !== false) {
            selectedTools.push(toolKey);
        }
    });
    
    // If the user may have selected tools on other pages, the current page state should already be saved.
    // The state map should already contain state for all visited pages.
    
    return selectedTools;
}

// Set selected tools (used when editing a role)
function setSelectedRoleTools(selectedToolKeys) {
    const selectedSet = new Set(selectedToolKeys || []);
    
    // Update state map
    roleToolStateMap.forEach((state, toolKey) => {
        state.enabled = selectedSet.has(toolKey);
    });
    
    // Update current page checkbox state
    document.querySelectorAll('#role-tools-list .role-tool-item').forEach(item => {
        const toolKey = item.dataset.toolKey;
        const checkbox = item.querySelector('input[type="checkbox"]');
        if (toolKey && checkbox) {
            checkbox.checked = selectedSet.has(toolKey);
        }
    });
    
    updateRoleToolsStats();
}

// Show add role modal
async function showAddRoleModal() {
    if (typeof requirePermission === 'function' && !requirePermission('roles:write')) return;
    const modal = document.getElementById('role-modal');
    if (!modal) return;

    document.getElementById('role-modal-title').textContent = _t('roleModal.addRole');
    document.getElementById('role-name').value = '';
    document.getElementById('role-name').disabled = false;
    document.getElementById('role-description').value = '';
    document.getElementById('role-icon').value = '';
    document.getElementById('role-user-prompt').value = '';
    document.getElementById('role-enabled').checked = true;
    if (typeof loadWorkflowOptionsForRoleModal === 'function') {
        await loadWorkflowOptionsForRoleModal('');
    }
    const workflowPolicy = document.getElementById('role-workflow-policy');
    if (workflowPolicy) {
        workflowPolicy.value = 'auto';
    }

    // When adding a role: show tool selection UI, hide default role hint
    const toolsSection = document.getElementById('role-tools-section');
    const defaultHint = document.getElementById('role-tools-default-hint');
    const toolsControls = document.querySelector('.role-tools-controls');
    const toolsList = document.getElementById('role-tools-list');
    const formHint = toolsSection ? toolsSection.querySelector('.form-hint') : null;
    
    if (defaultHint) {
        defaultHint.style.display = 'none';
    }
    if (toolsControls) {
        toolsControls.style.display = 'block';
    }
    if (toolsList) {
        toolsList.style.display = 'block';
    }
    if (formHint) {
        formHint.style.display = 'block';
    }

    // Resettoolstatus
    roleToolStateMap.clear();
    roleConfiguredTools.clear(); // Clear the role's configured tool list
    roleUsesallTools = false; // When adding a role, do not use all tools by default
    roleToolssearchKeyword = '';
    const searchInput = document.getElementById('role-tools-search');
    if (searchInput) {
        searchInput.value = '';
    }
    const clearBtn = document.getElementById('role-tools-search-clear');
    if (clearBtn) {
        clearBtn.style.display = 'none';
    }
    roleToolsStatusfilter = '';
    syncRoleToolsFilterButtons();
    roleToolsPagination. pageSize = getRoleToolsPageSize();
    
    // Clear the tool list DOM to prevent saveCurrentRolePageToolStates in loadRoleTools from reading stale state
    if (toolsList) {
        toolsList.innerHTML = '';
    }

    // Load and render tool list
    await loadRoleTools(1, '');
    
    // Ensure tool list is visible
    if (toolsList) {
        toolsList.style.display = 'block';
    }
    
    // Ensure statistics info is correctly updated (shows 0/N)
    updateRoleToolsStats();

    refreshRoleModalSelects();
    openAppModal('role-modal');
}

// edit role
async function editRole(roleName) {
    const role = roles.find(r => r.name === roleName);
    if (!role) {
        showNotification(_t('roleModal.roleNotFound'), 'error');
        return;
    }

    const modal = document.getElementById('role-modal');
    if (!modal) return;

    document.getElementById('role-modal-title').textContent = _t('roleModal.editRole');
    document.getElementById('role-name').value = role.name;
    document.getElementById('role-name').disabled = true; // Name cannot be changed when editing
    document.getElementById('role-description').value = role.description || '';
    // Handle icon field: if in Unicode escape format, convert to emoji; otherwise use as-is
    let iconValue = role.icon || '';
    if (iconValue && iconValue.startsWith('\\U')) {
        // Convert Unicode escape format (e.g. \U0001F3C6) to emoji
        try {
            const codePoint = parseInt(iconValue.substring(2), 16);
            iconValue = String.fromCodePoint(codePoint);
        } catch (e) {
            // If conversion fails, use original value
        }
    }
    document.getElementById('role-icon').value = iconValue;
    document.getElementById('role-user-prompt').value = role.user_prompt || '';
    document.getElementById('role-enabled').checked = role.enabled !== false;
    if (typeof loadWorkflowOptionsForRoleModal === 'function') {
        await loadWorkflowOptionsForRoleModal(role.workflow_id || '');
    }
    const workflowPolicy = document.getElementById('role-workflow-policy');
    if (workflowPolicy) {
        workflowPolicy.value = role.workflow_policy || 'auto';
    }

    // Check whether it is the default role
    const isdefaultRole = roleName === 'default';
    const toolsSection = document.getElementById('role-tools-section');
    const defaultHint = document.getElementById('role-tools-default-hint');
    const toolsControls = document.querySelector('.role-tools-controls');
    const toolsList = document.getElementById('role-tools-list');
    const formHint = toolsSection ? toolsSection.querySelector('.form-hint') : null;
    
    if (isdefaultRole) {
        // Default role: hide tool selection UI, show hint info
        if (defaultHint) {
            defaultHint.style.display = 'block';
        }
        if (toolsControls) {
            toolsControls.style.display = 'none';
        }
        if (toolsList) {
            toolsList.style.display = 'none';
        }
        if (formHint) {
            formHint.style.display = 'none';
        }
    } else {
        // Non-default role: show tool selection UI, hide hint info
        if (defaultHint) {
            defaultHint.style.display = 'none';
        }
        if (toolsControls) {
            toolsControls.style.display = 'block';
        }
        if (toolsList) {
            toolsList.style.display = 'block';
        }
        if (formHint) {
            formHint.style.display = 'block';
        }

        // Resettoolstatus
        roleToolStateMap.clear();
        roleConfiguredTools.clear(); // Clear the role's configured tool list
        roleToolssearchKeyword = '';
        const searchInput = document.getElementById('role-tools-search');
        if (searchInput) {
            searchInput.value = '';
        }
        const clearBtn = document.getElementById('role-tools-search-clear');
        if (clearBtn) {
            clearBtn.style.display = 'none';
        }
        roleToolsStatusfilter = '';
        syncRoleToolsFilterButtons();
        roleToolsPagination. pageSize = getRoleToolsPageSize();

        // Prefer the tools field; fall back to mcps field if absent (backwards compatibility)
        const selectedTools = role.tools || (role.mcps && role.mcps.length > 0 ? role.mcps : []);
        
        // Determine whether to use all tools: if no tools are configured (or tools is an empty array), use all tools
        roleUsesallTools = !role.tools || role.tools.length === 0;
        
        // Save role's configured tool list
        if (selectedTools.length > 0) {
            selectedTools.forEach(toolKey => {
                roleConfiguredTools.add(toolKey);
            });
        }
        
        // If there are selected tools, initialize the state map first
        if (selectedTools.length > 0) {
            roleUsesallTools = false; // Tools are configured, do not use all tools
            // Add selected tools to the state map (mark as selected)
            selectedTools.forEach(toolKey => {
                // If this tool is not yet in the map, create a default state (enabled = true)
                if (!roleToolStateMap.has(toolKey)) {
                    roleToolStateMap.set(toolKey, {
                        enabled: true,
                        is_external: false,
                        external_mcp: '',
                        name: toolKey.split('::').pop() || toolKey // Extract tool name from toolKey
                    });
                } else {
                    // If already exists, update to selected state
                    const state = roleToolStateMap.get(toolKey);
                    state.enabled = true;
                }
            });
        }

        // Load tool list (first page)
        await loadRoleTools(1, '');
        
        // If using all tools, mark all enabled tools on the current page as selected
        if (roleUsesallTools) {
            // Mark all MCP-enabled tools on the current page as selected
            document.querySelectorAll('#role-tools-list input[type="checkbox"]').forEach(checkbox => {
                const toolItem = checkbox.closest('.role-tool-item');
                if (toolItem) {
                    const toolKey = toolItem.dataset.toolKey;
                    const toolName = toolItem.dataset.toolName;
                    const isExternal = toolItem.dataset.isExternal === 'true';
                    const externalMcp = toolItem.dataset.externalMcp || '';
                    if (toolKey) {
                        const state = roleToolStateMap.get(toolKey);
                        // Only select tools enabled in MCP
                        // If state exists, use mcpEnabled from state; otherwise assume enabled (loadRoleTools should have initialized all tools)
                        const shouldEnable = state ? (state.mcpEnabled !== false) : true;
                        checkbox.checked = shouldEnable;
                        if (state) {
                            state.enabled = shouldEnable;
                        } else {
                            // If state does not exist, create new state (this should not happen since loadRoleTools should have initialized it)
                            roleToolStateMap.set(toolKey, {
                                enabled: shouldEnable,
                                is_external: isExternal,
                                external_mcp: externalMcp,
                                name: toolName,
                                mcpEnabled: true // Assume enabled; actual value will be updated in loadRoleTools
                            });
                        }
                    }
                }
            });
            // Update statistics info to ensure the correct selected count is shown
            updateRoleToolsStats();
        } else if (selectedTools.length > 0) {
            // After loading is complete, set selected state again (ensure current page tools are also correctly set)
            setSelectedRoleTools(selectedTools);
        }
    }

    refreshRoleModalSelects();
    openAppModal('role-modal');
}

// Close role modal
function closeRoleModal() {
    closeAllRoleModalSelects();
    closeAppModal('role-modal');
}

function closeRoleSelectModal() {
    closeAppModal('role-select-modal');
}

// Get all selected tools (including tools not enabled in MCP)
function getAllSelectedRoleTools() {
    // Save current page state first
    saveCurrentRolePageToolStates();
    
    // Get all selected tools from the state map (regardless of whether enabled in MCP)
    const selectedTools = [];
    roleToolStateMap.forEach((state, toolKey) => {
        if (state.enabled) {
            selectedTools.push({
                key: toolKey,
                name: state.name || toolKey.split('::').pop() || toolKey,
                mcpEnabled: state.mcpEnabled !== false // When mcpEnabled is false it is not enabled; otherwise treat as enabled
            });
        }
    });
    
    return selectedTools;
}

// Check and get tools not enabled in MCP
function getDisabledTools(selectedTools) {
    return selectedTools.filter(tool => {
        const state = roleToolStateMap.get(tool.key);
        // If mcpEnabled is explicitly false, consider it not enabled
        return state && state.mcpEnabled === false;
    });
}

// Load all tools into state map (used when switching from "use all tools" to partial tool selection)
async function loadAllToolsToStateMap() {
    try {
        const pageSize = 100; // Use a larger page size to reduce the number of requests
        let  page = 1;
        let hasMore = true;
        
        // Iterate through all pages to get all tools
        while (hasMore) {
            const url = `/api/config/tools?page=${ page}&page_size=${ pageSize}`;
            const response = await apiFetch(url);
            if (!response.ok) {
                throw new Error('Failed to get tool list');
            }
            
            const result = await response.json();
            
            // Add all tools to the state map
            result.tools.forEach(tool => {
                const toolKey = getToolKey(tool);
                if (!roleToolStateMap.has(toolKey)) {
                    // Tool not in map; initialize based on current mode
                    let enabled = false;
                    if (roleUsesallTools) {
                        // If using all tools and the tool is enabled in MCP, mark as selected
                        enabled = tool.enabled ? true : false;
                    } else {
                        // If not using all tools, only mark as selected if the tool is in the role's configured tool list
                        enabled = roleConfiguredTools.has(toolKey);
                    }
                    roleToolStateMap.set(toolKey, {
                        enabled: enabled,
                        is_external: tool.is_external || false,
                        external_mcp: tool.external_mcp || '',
                        name: tool.name,
                        mcpEnabled: tool.enabled // Save original MCP enabled state
                    });
                } else {
                    // Tool already in map; update other attributes but preserve enabled state
                    const state = roleToolStateMap.get(toolKey);
                    state.is_external = tool.is_external || false;
                    state.external_mcp = tool.external_mcp || '';
                    state.mcpEnabled = tool.enabled; // Update original MCP enabled state
                    if (!state.name || state.name === toolKey.split('::').pop()) {
                        state.name = tool.name; // updateTool name
                    }
                }
            });
            
            // Check whether there are more pages
            if ( page >= result.total_pages) {
                hasMore = false;
            } else {
                 page++;
            }
        }
    } catch (error) {
        console.error('Failed to load all tools into state map:', error);
        throw error;
    }
}

// save role
async function saveRole() {
    if (typeof requirePermission === 'function' && !requirePermission('roles:write')) return;
    const name = document.getElementById('role-name').value.trim();
    if (!name) {
        showNotification(_t('roleModal.roleNameRequired'), 'error');
        return;
    }

    const description = document.getElementById('role-description').value.trim();
    let icon = document.getElementById('role-icon').value.trim();
    // Convert emoji to Unicode escape format to match YAML format (e.g. \U0001F3C6)
    if (icon) {
        // Get the Unicode code point of the first character (handles emoji that may be multiple characters)
        const codePoint = icon.codePointAt(0);
        if (codePoint && codePoint > 0x7F) {
            // Convert to 8-digit hex format (\U0001F3C6)
            icon = '\\U' + codePoint.toString(16).toUpperCase().padStart(8, '0');
        }
    }
    const userPrompt = document.getElementById('role-user-prompt').value.trim();
    const enabled = document.getElementById('role-enabled').checked;
    const workflowIdEl = document.getElementById('role-workflow-id');
    const workflowPolicyEl = document.getElementById('role-workflow-policy');
    const workflowId = workflowIdEl ? workflowIdEl.value.trim() : '';
    const workflowPolicy = workflowPolicyEl ? workflowPolicyEl.value.trim() : 'auto';

    const isEdit = document.getElementById('role-name').disabled;
    
    // Check whether it is the default role
    const isdefaultRole = name === 'default';
    
    // Check whether this is the first user-created role (no user-created roles exist after excluding the default role)
    const isFirstUserRole = !isEdit && !isdefaultRole && roles.filter(r => r.name !== 'default').length === 0;
    
    // Default role does not save the tools field (uses all tools)
    // Non-default role: if using all tools (roleUsesallTools is true), also do not save the tools field
    let tools = [];
    let disabledTools = []; // Stores tools not enabled in MCP
    
    if (!isdefaultRole) {
        // Save current page state
        saveCurrentRolePageToolStates();
        
        // Collect all selected tools (including those not enabled in MCP)
        let allSelectedTools = getAllSelectedRoleTools();
        
        // If this is the first user role and no tools are selected, use all tools by default
        if (isFirstUserRole && allSelectedTools.length === 0) {
            roleUsesallTools = true;
            showNotification(_t('roleModal.firstRoleNoToolsHint'), 'info');
        } else if (roleUsesallTools) {
            // If currently using all tools, check whether the user has deselected some
            // Check whether there are enabled but not selected tools in the state map
            let hasUnselectedTools = false;
            roleToolStateMap.forEach((state) => {
                // If a tool is MCP-enabled but not selected, the user has deselected it
                if (state.mcpEnabled !== false && !state.enabled) {
                    hasUnselectedTools = true;
                }
            });
            
            // If the user deselected some enabled tools, switch to partial tool mode
            if (hasUnselectedTools) {
                // Before switching, load all tools into the state map
                // so we can correctly save the state of all tools (except those deselected by the user)
                await loadAllToolsToStateMap();
                
                // Mark all enabled tools as selected (except those deselected by the user)
                // Tools deselected by the user have enabled = false in the state map and remain unchanged
                roleToolStateMap.forEach((state, toolKey) => {
                    // If a tool is MCP-enabled and not explicitly marked as not selected (i.e. enabled is not false),
                    // mark it as selected
                    if (state.mcpEnabled !== false && state.enabled !== false) {
                        state.enabled = true;
                    }
                });
                
                roleUsesallTools = false;
            } else {
                // Even when using all tools, load all tools into the state map to check whether any disabled tools were selected
                // This detects whether the user has manually selected some disabled tools
                await loadAllToolsToStateMap();
                
                // Check whether any disabled tool has been manually selected (enabled = true but mcpenabled = false)
                let hasdisabledToolsSelected = false;
                roleToolStateMap.forEach((state) => {
                    if (state.enabled && state.mcpEnabled === false) {
                        hasdisabledToolsSelected = true;
                    }
                });
                
                // If no disabled tools are selected, mark all enabled tools as selected (the default "use all tools" behavior)
                if (!hasdisabledToolsSelected) {
                    roleToolStateMap.forEach((state) => {
                        if (state.mcpEnabled !== false) {
                            state.enabled = true;
                        }
                    });
                }
                
                // Update allSelectedTools because the state map now contains all tools
                allSelectedTools = getAllSelectedRoleTools();
            }
        }
        
        // Check which tools are not enabled in MCP (must check regardless of whether using all tools)
        disabledTools = getDisabledTools(allSelectedTools);
        
        // If there are disabled tools, warn the user
        if (disabledTools.length > 0) {
            const toolNames = disabledTools.map(t => t.name).join(', ');
            const message = `The following ${disabledTools.length} tool(s) are not enabled in MCP and cannot be configured in this role:\n\n${toolNames}\n\nPlease enable these tools in "MCP" first before configuring them in a role.\n\nContinue saving? (Only enabled tools will be saved)`;
            
            if (!confirm(message)) {
                return; // User cancelled save
            }
        }
        
        // If using all tools, no need to get the tool list
        if (!roleUsesallTools) {
            // Get selected tool list (only includes tools enabled in MCP)
            tools = await getSelectedRoleTools();
        }
    }

    const roleData = {
        name: name,
        description: description,
        icon: icon || undefined, // If empty string, do not send this field
        user_prompt: userPrompt,
        tools: tools, // Default role uses empty array to indicate all tools
        enabled: enabled,
        workflow_id: workflowId || undefined,
        workflow_version: workflowId ? 'latest' : undefined,
        workflow_policy: workflowId ? (workflowPolicy || 'auto') : undefined
    };
    const url = isEdit ? `/api/roles/${encodeURIComponent(name)}` : '/api/roles';
    const method = isEdit ? 'PUT' : 'POST';

    try {
        const response = await apiFetch(url, {
            method: method,
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(roleData)
        });

        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'save rolefailed');
        }

        // If any disabled tools were filtered out, notify the user
        if (disabledTools.length > 0) {
            let toolNames = disabledTools.map(t => t.name).join(', ');
            // If the tool name list is too long, truncate for display
            if (toolNames.length > 100) {
                toolNames = toolNames.substring(0, 100) + '...';
            }
            showNotification(
                `${isEdit ? 'Role updated' : 'Role created'} — ${disabledTools.length} tool(s) not enabled in MCP were filtered out: ${toolNames}. Please enable these tools in "MCP" first before configuring them in a role.`,
                'warning'
            );
        } else {
            showNotification(isEdit ? 'Role updated' : 'Role created', 'success');
        }
        
        closeRoleModal();
        await refreshRoles();
    } catch (error) {
        console.error('save rolefailed:', error);
        showNotification('save rolefailed: ' + error.message, 'error');
    }
}

// delete role
async function deleteRole(roleName) {
    if (roleName === 'default') {
        showNotification(_t('roleModal.cannotDeleteDefaultRole'), 'error');
        return;
    }

    if (!confirm(`Delete role "${roleName}"? This action cannot be undone.`)) {
        return;
    }

    try {
        const response = await apiFetch(`/api/roles/${encodeURIComponent(roleName)}`, {
            method: 'DELETE'
        });

        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.error || 'delete rolefailed');
        }

        showNotification('Role deleted', 'success');
        
        // If the deleted role is the currently selected role, switch to the default role
        if (currentRole === roleName) {
            handleRoleChange('');
        }

        await refreshRoles();
    } catch (error) {
        console.error('delete rolefailed:', error);
        showNotification('delete rolefailed: ' + error.message, 'error');
    }
}

// Initialize role list on page switch
if (typeof window.switchPage === 'function') {
    const originalSwitchPage = window.switchPage;
    window.switchPage = function( page) {
        originalSwitchPage( page);
        if ( page === 'roles-management') {
            loadRoles().then(() => renderRolesList());
        }
    };
}

// Close modal on outside click
document.addEventListener('click', (e) => {
    const roleSelectModal = document.getElementById('role-select-modal');
    if (roleSelectModal && e.target === roleSelectModal) {
        closeRoleSelectModal();
    }

    const roleModal = document.getElementById('role-modal');
    if (roleModal && e.target === roleModal) {
        closeRoleModal();
    }

    // Close role selection panel on outside click (must use #role-selector-wrapper, not .role-selector-wrapper: the project selector also uses that class)
    if (isRoleSelectionPanelOpen()) {
        const roleSelectorWrapper = getChatRoleSelectorWrapper();
        if (!roleSelectorWrapper?.contains(e.target)) {
            closeRoleSelectionPanel();
        }
    }
});

// Initialize on page load
document.addEventListener('DOMContentLoaded', () => {
    loadRoles();
    updateRoleSelectorDisplay();
    refreshRoleModalSelects();
});

// Refresh role selector and role selection list text after a language switch
document.addEventListener('languagechange', () => {
    updateRoleSelectorDisplay();
    renderRoleSelectionSidebar();
    syncAllRoleModalSelects();
});

// Get currently selected role (used by chat.js)
function getCurrentRole() {
    return currentRole || '';
}

// Expose functions to global scope
if (typeof window !== 'undefined') {
    window.getCurrentRole = getCurrentRole;
    window.setCurrentRole = handleRoleChange;
    window.toggleRoleSelectionPanel = toggleRoleSelectionPanel;
    window.closeRoleSelectionPanel = closeRoleSelectionPanel;
    window.closeRoleSelectModal = closeRoleSelectModal;
    window.filterRoleToolsByStatus = filterRoleToolsByStatus;
    window.refreshRoleModalSelects = refreshRoleModalSelects;
    window.currentSelectedRole = getCurrentRole();
    
    // Listen for role changes, update global variable
    const originalHandleRoleChange = handleRoleChange;
    handleRoleChange = function(roleName) {
        originalHandleRoleChange(roleName);
        if (typeof window !== 'undefined') {
            window.currentSelectedRole = getCurrentRole();
            window.setCurrentRole = handleRoleChange;
        }
    };
}
