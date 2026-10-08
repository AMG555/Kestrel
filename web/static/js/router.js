// Page router management
let currentPage = null;

/** chat and vulnerabilities  pages retain the currentHash query string (e.g. ?conversation= / ?conversation_id=) on switch */
function buildHashForPage( pageId) {
    if ( pageId !== 'chat' &&  pageId !== 'vulnerabilities') {
        return  pageId;
    }
    const full = window.location.hash.slice(1);
    const parts = full.split('?');
    const curPage = parts[0];
    const q = parts.length > 1 ? parts.slice(1).join('?') : '';
    if (curPage ===  pageId && q) {
        return  pageId + '?' + q;
    }
    return  pageId;
}

let chatConversationFromHashSeq = 0;

function cancelScheduledChatConversationFromHash() {
    chatConversationFromHashSeq++;
    setChatConversationRestorePending('', false);
}
window.cancelScheduledChatConversationFromHash = cancelScheduledChatConversationFromHash;

function setChatConversationRestorePending(conversationId, pending) {
    const container = document.querySelector('.chat-container');
    if (!container) return;
    const id = String(conversationId || '').trim();
    if (pending && id) {
        container.classList.add('is-conversation-restoring');
        container.dataset.restoringConversationId = id;
        container.setAttribute('aria-busy', 'true');
        return;
    }
    container.classList.remove('is-conversation-restoring');
    delete container.dataset.restoringConversationId;
    container.removeAttribute('aria-busy');
}

function finishChatConversationRestore(conversationId) {
    setChatConversationRestorePending(conversationId, false);
}
window.finishChatConversationRestore = finishChatConversationRestore;

function scheduleChatConversationFromHash(delayMs) {
    const hash = window.location.hash.slice(1);
    const hashParts = hash.split('?');
    if (hashParts[0] !== 'chat' || hashParts.length < 2) {
        return;
    }
    const params = new URLSearchParams(hashParts.slice(1).join('?'));
    const conversationId = params.get('conversation');
    const projectId = params.get('project');
    if (projectId && typeof setActiveProjectId === 'function') {
        setActiveProjectId(projectId);
        if (typeof refreshChatProjectSelector === 'function') {
            refreshChatProjectSelector();
        }
    }
    if (!conversationId) {
        return;
    }
    // Within the same event loop, mask the default new-chat status to avoid flashing "No project" before the network request returns.
    setChatConversationRestorePending(conversationId, true);
    const token = ++chatConversationFromHashSeq;
    setTimeout(() => {
        if (token !== chatConversationFromHashSeq) {
            return;
        }
        if (typeof loadConversation === 'function') {
            loadConversation(conversationId);
        } else if (typeof window.loadConversation === 'function') {
            window.loadConversation(conversationId);
        } else {
            console.warn('loadConversation function not found');
        }
    }, delayMs);
}

/** Go to specified chat: single  page switch + single load, to avoid hashchange and manual load triggering twice and causing flicker */
function navigateToConversation(conversationId) {
    const cid = String(conversationId || '').trim();
    if (!cid) return;
    const targetHash = 'chat?conversation=' + encodeURIComponent(cid);
    const alreadyOnChat = currentPage === 'chat';

    if (window.location.hash.slice(1) !== targetHash) {
        history.replaceState(null, '', '#' + targetHash);
    }

    if (!alreadyOnChat) {
        switchPage('chat');
    }

    if (typeof loadConversation === 'function') {
        void loadConversation(cid);
    } else if (typeof window.loadConversation === 'function') {
        void window.loadConversation(cid);
    }
}
window.navigateToConversation = navigateToConversation;

// Initialize router
function initRouter() {
    // Read  page from URL hash (if present)
    const hash = window.location.hash.slice(1);
    if (hash) {
        const hashParts = hash.split('?');
        let  pageId = hashParts[0];
        if ( pageId === 'c2')  pageId = 'c2-listeners';
        if ( pageId && ['dashboard', 'chat', 'hitl', 'tool-guard', 'asset-overview', 'asset-library', 'info-collect', 'projects', 'vulnerabilities', 'WebShell', 'chat-files', 'mcp-monitor', 'mcp-management', 'knowledge-management', 'knowledge-retrieval-logs', 'roles-management', 'platform-rbac', 'workflows', 'skills-monitor', 'skills-management', 'agents-management', 'settings', 'tasks', 'c2-listeners', 'c2-sessions', 'c2-tasks', 'c2-payloads', 'c2-events', 'c2-profiles'].includes( pageId)) {
            switchPage( pageId);
            if ( pageId === 'chat') {
                scheduleChatConversationFromHash(0);
            }
            return;
        }
    }
    
    // Show dashboard by default
    switchPage('dashboard');
}

// Switch  page
function switchPage( pageId) {
    const targetPage = document.getElementById(` page-${ pageId}`);
    if (!targetPage) return;
    if ( pageId !== 'chat') {
        setChatConversationRestorePending('', false);
        if (currentPage === 'chat' && typeof window.abandonChatConversationForPageNavigation === 'function') {
            window.abandonChatConversationForPageNavigation();
        }
    }

    // Navigation click modifies the hash; the browser will still fire hashchange afterward.
    // When the same  page is already active, do not re-initialize to avoid duplicate API requests and re-rendering.
    if (currentPage ===  pageId && targetPage.classList.contains('active')) {
        const currentHash = buildHashForPage( pageId);
        if (window.location.hash.slice(1) !== currentHash) {
            window.location.hash = currentHash;
        }
        return;
    }

    if (typeof window.syncC2NavOnceFromServer === 'function') {
        void window.syncC2NavOnceFromServer();
    }
    // Hide all  pages
    document.querySelectorAll('. page').forEach( page => {
         page.classList.remove('active');
    });
    
    // Show target  page
    targetPage.classList.add('active');
    currentPage =  pageId;
        
    const newHash = buildHashForPage( pageId);
    if (window.location.hash.slice(1) !== newHash) {
        window.location.hash = newHash;
    }
        
    // update navigation state
    updateNavState( pageId);
        
    // Page-specific initialization
    initPage( pageId);

    if (typeof applyRBACToUI === 'function') {
        applyRBACToUI(targetPage);
    }
}
window.switchPage = switchPage;

// update navigation state
function updateNavState( pageId) {
    // remove all active states
    document.querySelectorAll('.nav-item').forEach(item => {
        item.classList.remove('active');
        item.classList.remove('expanded');
    });
    
    document.querySelectorAll('.nav-submenu-item').forEach(item => {
        item.classList.remove('active');
    });
    
    // Set active state
    if ( pageId === 'hitl' ||  pageId === 'tool-guard') {
        const securityItem = document.querySelector('.nav-item[data-page="security"]');
        if (securityItem) {
            securityItem.classList.add('active');
            securityItem.classList.add('expanded');
        }
        const submenuItem = document.querySelector(`.nav-submenu-item[data-page="${ pageId}"]`);
        if (submenuItem) submenuItem.classList.add('active');
    } else if ( pageId === 'asset-overview' ||  pageId === 'asset-library' ||  pageId === 'info-collect') {
        const assetItem = document.querySelector('.nav-item[data-page="assets"]');
        if (assetItem) {
            assetItem.classList.add('active');
            assetItem.classList.add('expanded');
        }
        const submenuItem = document.querySelector(`.nav-submenu-item[data-page="${ pageId}"]`);
        if (submenuItem) submenuItem.classList.add('active');
    } else if ( pageId === 'mcp-monitor' ||  pageId === 'mcp-management') {
        // MCP submenu  items
        const mcpItem = document.querySelector('.nav-item[data-page="MCP"]');
        if (mcpItem) {
            mcpItem.classList.add('active');
            // expand MCP submenu
            mcpItem.classList.add('expanded');
        }
        
        const submenuItem = document.querySelector(`.nav-submenu-item[data-page="${ pageId}"]`);
        if (submenuItem) {
            submenuItem.classList.add('active');
        }
    } else if ( pageId === 'knowledge-management' ||  pageId === 'knowledge-retrieval-logs') {
        // Knowledge submenu  items
        const knowledgeItem = document.querySelector('.nav-item[data-page="knowledge"]');
        if (knowledgeItem) {
            knowledgeItem.classList.add('active');
            // Expand knowledge submenu
            knowledgeItem.classList.add('expanded');
        }
        
        const submenuItem = document.querySelector(`.nav-submenu-item[data-page="${ pageId}"]`);
        if (submenuItem) {
            submenuItem.classList.add('active');
        }
    } else if ( pageId === 'skills-monitor' ||  pageId === 'skills-management') {
        // Skills submenu items
        const skillsItem = document.querySelector('.nav-item[data-page="skills"]');
        if (skillsItem) {
            skillsItem.classList.add('active');
            // Expand Skills submenu
            skillsItem.classList.add('expanded');
        }
        
        const submenuItem = document.querySelector(`.nav-submenu-item[data-page="${ pageId}"]`);
        if (submenuItem) {
            submenuItem.classList.add('active');
        }
    } else if ( pageId === 'agents-management') {
        const agentsItem = document.querySelector('.nav-item[data-page="agents"]');
        if (agentsItem) {
            agentsItem.classList.add('active');
            agentsItem.classList.add('expanded');
        }
        const submenuItem = document.querySelector(`.nav-submenu-item[data-page="${ pageId}"]`);
        if (submenuItem) {
            submenuItem.classList.add('active');
        }
    } else if ( pageId.startsWith('c2') ||  pageId === 'c2-listeners' ||  pageId === 'c2-sessions' ||  pageId === 'c2-tasks' ||  pageId === 'c2-payloads' ||  pageId === 'c2-events' ||  pageId === 'c2-profiles') {
        // C2 submenu items
        const c2Item = document.querySelector('.nav-item[data-page="c2"]');
        if (c2Item) {
            c2Item.classList.add('active');
            c2Item.classList.add('expanded');
        }
        const submenuItem = document.querySelector(`.nav-submenu-item[data-page="${ pageId}"]`);
        if (submenuItem) {
            submenuItem.classList.add('active');
        }
    } else if ( pageId === 'roles-management') {
        // Role submenu items
        const rolesItem = document.querySelector('.nav-item[data-page="roles"]');
        if (rolesItem) {
            rolesItem.classList.add('active');
            // Expand role submenu
            rolesItem.classList.add('expanded');
        }
        
        const submenuItem = document.querySelector(`.nav-submenu-item[data-page="${ pageId}"]`);
        if (submenuItem) {
            submenuItem.classList.add('active');
        }
    } else {
        // Main menu items
        const navItem = document.querySelector(`.nav-item[data-page="${ pageId}"]`);
        if (navItem) {
            navItem.classList.add('active');
        }
    }
}

/** Read sidebar submenu items (only within .nav-submenu to avoid false matches) */
function getNavSubmenuItems(navItem) {
    if (!navItem) return [];
    const submenu = navItem.querySelector('.nav-submenu');
    if (!submenu) return [];
    return Array.from(submenu.querySelectorAll('.nav-submenu-item'));
}

// Toggle submenu
function toggleSubmenu(menuId) {
    const sidebar = document.getElementById('main-sidebar');
    const navItem = document.querySelector(`.nav-item[data-page="${menuId}"]`);
    
    if (!navItem) return;
    
    const collapsed = sidebar && sidebar.classList.contains('collapsed');

    // Check if sidebar is collapsed
    if (collapsed) {
        // Show popup menu when sidebar is collapsed
        showSubmenuPopup(navItem, menuId);
        return;
    }

    // Toggle submenu when expanded, and scroll into view so sub-items are visible
    const willExpand = !navItem.classList.contains('expanded');
    navItem.classList.toggle('expanded');
    if (willExpand) {
        requestAnimationFrame(() => {
            navItem.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
            const items = getNavSubmenuItems(navItem);
            const last =  items[ items.length - 1];
            if (last) {
                last.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
            }
        });
    }
}
window.toggleSubmenu = toggleSubmenu;

// Show submenu popup
function showSubmenuPopup(navItem, menuId) {
    const existingPopup = document.querySelector('.submenu-popup');
    if (existingPopup) {
        const sameMenu = existingPopup.dataset.menuId === menuId;
        existingPopup.remove();
        // Clicking the same item again: just close; clicking another item: open the new menu
        if (sameMenu) {
            return;
        }
    }

    const navItemContent = navItem.querySelector('.nav-item-content');
    const submenu = navItem.querySelector('.nav-submenu');
    
    if (!submenu) return;
    
    // Get menu position
    const rect = navItemContent.getBoundingClientRect();
    
    // Create popup menu
    const popup = document.createElement('div');
    popup.className = 'submenu-popup';
    popup.dataset.menuId = menuId;
    popup.style.position = 'fixed';
    popup.style.left = (rect.right + 8) + 'px';
    popup.style.top = rect.top + 'px';
    popup.style.zIndex = '1000';
    
    // Copy submenu items into popup
    const submenuItems = submenu.querySelectorAll('.nav-submenu-item');
    submenuItems.forEach(item => {
        if (item.hidden || (typeof permissionAllowedForElement === 'function' && !permissionAllowedForElement(item))) return;
        const popupItem = document.createElement('div');
        popupItem.className = 'submenu-popup-item';
        popupItem.textContent = item.textContent.trim();
        
        // Check if this is the currently active page
        const  pageId = item.getAttribute('data-page');
        if ( pageId && document.querySelector(`.nav-submenu-item[data-page="${ pageId}"].active`)) {
            popupItem.classList.add('active');
        }
        
        popupItem.onclick = function(e) {
            e.stopPropagation();
            e.preventDefault();
            
            // Get page ID and switch
            const  pageId = item.getAttribute('data-page');
            if ( pageId) {
                switchPage( pageId);
            }
            
            // Close popup menu
            popup.remove();
            document.removeEventListener('click', closePopup);
        };
        popup.appendChild(popupItem);
    });
    
    document.body.appendChild(popup);
    
    // Close popup on outside click
    const closePopup = function(e) {
        if (!popup.contains(e.target) && !navItem.contains(e.target)) {
            popup.remove();
            document.removeEventListener('click', closePopup);
        }
    };
    
    // Delay adding listener to avoid immediate trigger
    setTimeout(() => {
        document.addEventListener('click', closePopup);
    }, 0);
}

// Initialize page
async function initPage( pageId) {
    // Wait for i18n to be ready to avoid showing raw placeholder keys on fast refresh
    if (window.i18nReady) await window.i18nReady;
    if (typeof stopExternalMcpPoll === 'function') {
        stopExternalMcpPoll();
    }
    switch( pageId) {
        case 'dashboard':
            if (typeof refreshDashboard === 'function') {
                refreshDashboard();
            }
            break;
        case 'chat':
            // Restore chat list collapse state (preserve user choice when returning from another page)
            initConversationSidebarState();
            if (typeof prefetchProjectsForChat === 'function') {
                prefetchProjectsForChat();
            }
            if (typeof refreshChatProjectSelector === 'function') {
                refreshChatProjectSelector();
            }
            break;
        case 'tool-guard':
            if (typeof loadToolGuardConfig === 'function') loadToolGuardConfig();
            break;
        case 'hitl':
            if (typeof refreshHitlActivePanel === 'function') {
                refreshHitlActivePanel();
            } else if (typeof refreshHitlPending === 'function') {
                refreshHitlPending();
            }
            break;
        case 'info-collect':
            //  info collection  page
            if (typeof initInfoCollectPage === 'function') {
                initInfoCollectPage();
            }
            break;
        case 'asset-overview':
            if (typeof loadAssetOverview === 'function') loadAssetOverview();
            break;
        case 'asset-library':
            if (typeof loadAssets === 'function') loadAssets();
            break;
        case 'tasks':
            // Initialize Tasks page
            if (typeof initTasksPage === 'function') {
                initTasksPage();
            }
            break;
        case 'mcp-monitor':
            // Initialize monitor panel
            if (typeof refreshMonitorPanel === 'function') {
                refreshMonitorPanel();
            }
            if (typeof startMonitorPoll === 'function') {
                startMonitorPoll();
            }
            break;
        case 'mcp-management':
            // Initialize MCP
            const startLoadMcpTools = () => {
                // Load tool list (MCP tool config has moved to the MCP page)
                // Use async loading to avoid blocking page rendering
                if (typeof loadToolsList === 'function') {
                    // Ensure tool pagination settings are initialized
                    if (typeof getToolsPageSize === 'function' && typeof toolsPagination !== 'undefined') {
                        toolsPagination. pageSize = getToolsPageSize();
                    }
                    // Delay loading to let the page render first
                    setTimeout(() => {
                        loadToolsList(1, '').catch(err => {
                            console.error('Failed to load tool list:', err);
                        });
                    }, 100);
                }
            };
            const afterMcpConfigReady = () => {
                startLoadMcpTools();
                if (typeof loadExternalMCPs === 'function') {
                    loadExternalMCPs().catch(err => {
                        console.warn('failed to load external MCP list:', err);
                    });
                }
                if (typeof startExternalMcpPoll === 'function') {
                    startExternalMcpPoll();
                }
            };
            // Fetch config first (includes tool_search persistent list), then load tools and external MCPs.
            // Users with only MCP:read do not need to fetch the full /api/config (requires config:read).
            const canLoadFullConfig = typeof hasPermission !== 'function' || hasPermission('config:read');
            if (typeof loadConfig === 'function' && canLoadFullConfig) {
                loadConfig(false, { silent: true })
                    .catch(err => {
                        console.warn('Failed to load configuration (will continue loading MCP list):', err);
                    })
                    .finally(afterMcpConfigReady);
            } else {
                afterMcpConfigReady();
            }
            break;
        case 'projects':
            if (typeof initProjectsPage === 'function') {
                initProjectsPage();
            }
            break;
        case 'vulnerabilities':
            // Initialize vulnerabilities  page
            if (typeof initVulnerabilityPage === 'function') {
                initVulnerabilityPage();
            }
            break;
        case 'WebShell':
            // Initialize WebShell management page
            if (typeof initWebshellPage === 'function') {
                initWebshellPage();
            }
            break;
        case 'chat-files':
            if (typeof initChatFilesPage === 'function') {
                initChatFilesPage();
            }
            break;
        case 'settings':
            // Initialize settings page (no need to load tool list)
            if (typeof loadConfig === 'function') {
                loadConfig(false);
            }
            break;
        case 'roles-management':
            // Initialize Roles page
            // Reset search UI (variable auto-updates on next search)
            const rolesSearchInput = document.getElementById('roles-search');
            if (rolesSearchInput) {
                rolesSearchInput.value = '';
            }
            const rolesSearchClear = document.getElementById('roles-search-clear');
            if (rolesSearchClear) {
                rolesSearchClear.style.display = 'none';
            }
            if (typeof loadRoles === 'function') {
                loadRoles().then(() => {
                    if (typeof renderRolesList === 'function') {
                        renderRolesList();
                    }
                });
            }
            break;
        case 'platform-rbac':
            if (typeof initPlatformRbacPage === 'function') {
                initPlatformRbacPage();
            }
            break;
        case 'workflows':
            if (typeof refreshWorkflows === 'function') {
                refreshWorkflows();
            }
            break;
        case 'skills-monitor':
            // Initialize Skills status monitoring page
            if (typeof loadSkillsMonitor === 'function') {
                loadSkillsMonitor();
            }
            break;
        case 'skills-management':
            // Initialize Skills management page
            // Reset search UI (variable auto-updates on next search)
            const skillsSearchInput = document.getElementById('skills-search');
            if (skillsSearchInput) {
                skillsSearchInput.value = '';
            }
            const skillsSearchClear = document.getElementById('skills-search-clear');
            if (skillsSearchClear) {
                skillsSearchClear.style.display = 'none';
            }
            if (typeof initSkillsPagination === 'function') {
                initSkillsPagination();
            }
            if (typeof loadSkills === 'function') {
                loadSkills();
            }
            break;
        case 'agents-management':
            if (typeof loadMarkdownAgents === 'function') {
                loadMarkdownAgents();
            }
            break;
        case 'c2-listeners':
        case 'c2-sessions':
        case 'c2-tasks':
        case 'c2-payloads':
        case 'c2-events':
        case 'c2-profiles':
            window.currentPageId =  pageId;
            if (window.C2 && typeof window.C2.init === 'function') {
                window.C2.init();
            }
            break;
    }
    
    // Clean up timers from other pages
    if ( pageId !== 'tasks' && typeof cleanupTasksPage === 'function') {
        cleanupTasksPage();
    }
}

// Initialize router after page load
document.addEventListener('DOMContentLoaded', function() {
    initRouter();
    document.documentElement.classList.remove('initial-route-pending');
    initSidebarState();
    
    // Listen for hash changes
    window.addEventListener('hashchange', function() {
        const hash = window.location.hash.slice(1);
        // Handle hashes with query params (e.g. chat?conversation=xxx)
        const hashParts = hash.split('?');
        let  pageId = hashParts[0];
        
        if ( pageId === 'c2')  pageId = 'c2-listeners';
        if ( pageId && ['dashboard', 'chat', 'hitl', 'tool-guard', 'asset-overview', 'asset-library', 'info-collect', 'projects', 'tasks', 'workflows', 'vulnerabilities', 'WebShell', 'chat-files', 'mcp-monitor', 'mcp-management', 'knowledge-management', 'knowledge-retrieval-logs', 'roles-management', 'platform-rbac', 'skills-monitor', 'skills-management', 'agents-management', 'settings', 'c2-listeners', 'c2-sessions', 'c2-tasks', 'c2-payloads', 'c2-events', 'c2-profiles'].includes( pageId)) {
            switchPage( pageId);
            if ( pageId === 'chat') {
                scheduleChatConversationFromHash(0);
            }
        }
    });
});

// Toggle sidebar collapse/expand
function toggleSidebar() {
    const sidebar = document.getElementById('main-sidebar');
    if (sidebar) {
        sidebar.classList.toggle('collapsed');
        // Save collapse state to localStorage
        const isCollapsed = sidebar.classList.contains('collapsed');
        localStorage.setItem('sidebarCollapsed', isCollapsed ? 'true' : 'false');
    }
}
window.toggleSidebar = toggleSidebar;

// Initialize sidebar state
function initSidebarState() {
    const sidebar = document.getElementById('main-sidebar');
    if (sidebar) {
        const savedState = localStorage.getItem('sidebarCollapsed');
        if (savedState === 'true') {
            sidebar.classList.add('collapsed');
        }
    }
    initConversationSidebarState();
}

// Toggle chat page left-panel collapse/expand
function toggleConversationSidebar() {
    const sidebar = document.getElementById('conversation-sidebar');
    if (sidebar) {
        sidebar.classList.toggle('collapsed');
        const isCollapsed = sidebar.classList.contains('collapsed');
        localStorage.setItem('conversationSidebarcollapsed', isCollapsed ? 'true' : 'false');
    }
}
window.toggleConversationSidebar = toggleConversationSidebar;

// Restore chat list collapse state (applied when entering the Chat page)
function initConversationSidebarState() {
    const sidebar = document.getElementById('conversation-sidebar');
    if (sidebar) {
        const savedState = localStorage.getItem('conversationSidebarcollapsed');
        if (savedState === 'true') {
            sidebar.classList.add('collapsed');
        } else {
            sidebar.classList.remove('collapsed');
        }
    }
}

// Export functions for use by other scripts (consistent with early binding above, for external script detection)
window.currentPage = function() { return currentPage; };
