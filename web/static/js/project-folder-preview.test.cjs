const fs = require('node:fs');
const test = require('node:test');
const assert = require('node:assert/strict');

const projects = fs.readFileSync('web/static/js/projects.js', 'utf8');
const styles = fs.readFileSync('web/static/css/style.css', 'utf8');
const chat = fs.readFileSync('web/static/js/chat.js', 'utf8');
const html = fs.readFileSync('web/templates/index.html', 'utf8');
const rbac = fs.readFileSync('web/static/js/rbac-guards.js', 'utf8');
const zh = fs.readFileSync('web/static/i18n/zh-CN.json', 'utf8');
const en = fs.readFileSync('web/static/i18n/en-US.json', 'utf8');

function functionSource(source, name, nextName) {
    const start = source.indexOf(`function ${name}(`);
    const end = source.indexOf(`function ${nextName}(`, start);
    assert.notEqual(start, -1, `${name} should exist`);
    assert.notEqual(end, -1, `${nextName} should follow ${name}`);
    return source.slice(start, end);
}

function cssBlock(source, selector) {
    const match = source.match(new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*\\{[^}]*\\}`));
    assert.ok(match, `${selector} style block should exist`);
    return match[0];
}

test('No-project folder shares hover and keyboard-focus preview with regular projects', () => {
    const source = functionSource(projects, 'appendChatProjectFolderItem', 'appendChatProjectConversationItem');

    assert.match(source, /row\.addEventListener\('mouseenter', \(\) => scheduleShowProjectFolderPreview/);
    assert.match(source, /button\.addEventListener\('focus', \(\) => scheduleShowProjectFolderPreview/);
    assert.doesNotMatch(
        source,
        /if \(!isUnassigned\) \{\s*row\.addEventListener\('mouseenter', \(\) => scheduleShowProjectFolderPreview/
    );
});

test('No-project preview hides test scope and edit entry', () => {
    const source = functionSource(projects, 'showProjectFolderPreview', 'scheduleShowProjectFolderPreview');

    assert.match(source, /preview\.classList\.toggle\('is-unassigned', isUnassigned\)/);
    assert.match(source, /scopeRow\.hidden = isUnassigned \|\| !scope/);
    assert.match(source, /editButton\.hidden = isUnassigned/);
    assert.match(styles, /\.project-folder-preview\.is-unassigned \.project-folder-preview-edit\s*\{\s*display: none !important;/);
    assert.match(styles, /\.project-folder-preview\.is-unassigned \.project-folder-preview-details\s*\{\s*border-bottom: 0;/);
});

test('Project title provides a permission-protected new project entry', () => {
    const source = functionSource(projects, 'showNewProjectModalFromChatSidebar', 'saveProjectModal');

    assert.match(html, /class="add-group-btn project-folders-add-btn"[\s\S]*?onclick="showNewProjectModalFromChatSidebar\(\)"/);
    assert.match(chat, /projectHeader\.querySelector\('\.project-folders-add-btn'\)/);
    assert.match(source, /window\._projectModalFromChat = false/);
    assert.match(source, /window\._projectModalFromChatSidebar = true/);
    assert.match(rbac, /showNewProjectModalFromChatSidebar: 'project:write'/);
});

test('No erroneous expansion of no-project while chat project affiliation is still loading', () => {
    const resolver = functionSource(projects, 'resolveChatProjectFolderSelection', 'renderChatProjectFolders');
    const render = functionSource(projects, 'renderChatProjectFolders', 'refreshChatProjectFolders');

    assert.match(resolver, /if \(!chatProjectFolderContext\.ready\) return null/);
    assert.match(resolver, /if \(!conversation\) return null/);
    assert.match(resolver, /conversation\.projectId \|\| conversation\.project_id \|\| ''/);
    assert.match(render, /const selectedId = resolveChatProjectFolderSelection\(\)/);
    assert.match(render, /selectedId !== null && chatProjectFolderLastSelectionId !== selectedId/);
});

test('Projects toggle open/close folder in Codex style based on expand state', () => {
    const icon = functionSource(projects, 'projectFolderIconMarkup', 'clampProjectPreviewText');
    const folder = functionSource(projects, 'appendChatProjectFolderItem', 'appendChatProjectConversationItem');

    assert.match(icon, /const path = isExpanded/);
    assert.match(icon, /M3\.5 18V6\.5/);
    assert.match(icon, /M3\.5 7a2 2 0 0 1 2-2h4l2 2/);
    assert.match(folder, /icon\.className = 'project-folder-icon';/);
    assert.match(folder, /icon\.innerHTML = projectFolderIconMarkup\(isExpanded\);/);
});

test('Project name is truncated to 12 Unicode characters in the UI and full title is preserved for tooltip', () => {
    const formatterSource = functionSource(chat, 'formatProjectNameForDisplay', 'applyProjectNameDisplay');
    const formatter = new Function(
        'PROJECT_NAME_DISPLAY_MAX_CHARACTERS',
        `${formatterSource}; return formatProjectNameForDisplay;`
    )(12);
    const folder = functionSource(projects, 'appendChatProjectFolderItem', 'appendChatProjectConversationItem');
    const picker = functionSource(projects, 'appendChatProjectPanelItem', 'appendChatProjectPanelMessage');
    const button = functionSource(projects, 'updateChatProjectButtonLabel', 'renderChatProjectPanel');

    assert.equal(formatter('十二字符以内'), '十二字符以内');
    assert.equal(formatter('这是一个非常非常长的项目名称'), '这是一个非常非常长的项目…');
    assert.equal(formatter('😀😀😀😀😀😀😀😀😀😀😀😀😀'), '😀😀😀😀😀😀😀😀😀😀😀😀…');
    assert.match(chat, /const PROJECT_NAME_DISPLAY_MAX_CHARACTERS = 12/);
    assert.match(folder, /applyProjectNameDisplay\(title, project\.name/);
    assert.match(projects, /applyProjectNameDisplay\(titleEl, text\)/);
    assert.match(picker, /title="\$\{escapeAttr\(fullName\)\}"/);
    assert.match(picker, /setAttribute\('aria-label', fullName\)/);
    assert.match(button, /applyProjectNameDisplay/);
    assert.match(styles, /\.project-selector-wrapper \.role-selector-text\s*\{[\s\S]*?max-width: 13em/);
});

test('Project folder shows first 6 and allows loading more in batches', () => {
    const loadMore = functionSource(projects, 'loadMoreChatProjectFolders', 'renderChatProjectFolders');
    const render = functionSource(projects, 'renderChatProjectFolders', 'refreshChatProjectFolders');
    const search = functionSource(projects, 'handleProjectFolderSearch', 'clearProjectFolderSearch');

    assert.match(projects, /const CHAT_PROJECT_FOLDER_PAGE_SIZE = 6/);
    assert.match(loadMore, /chatProjectFolderVisibleCount \+= CHAT_PROJECT_FOLDER_PAGE_SIZE/);
    assert.match(render, /const visibleFolders = folders\.slice\(0, chatProjectFolderVisibleCount\)/);
    assert.match(render, /appendChatProjectFoldersLoadMore\(list, folders\.length - visibleFolders\.length\)/);
    assert.match(render, /chatProjectFolderVisibleCount = selectedIndex \+ 1/);
    assert.match(search, /renderChatProjectFolders\(projectsCacheAll\)/);
    assert.match(styles, /\.project-folders-load-more\s*\{/);
    assert.match(zh, /"projectFoldersLoadMoreRemaining": "(?:加载更多，剩余 \{\{count\}\} 个项目|Load more，剩余 \{\{count\}\} 项目|Load more, \{\{count\}\} projects remaining)"/);
    assert.match(en, /"projectFoldersLoadMoreRemaining": "Load more, \{\{count\}\} projects remaining"/);
});

test('Chat hover preview displays localised year-month-day hour:minute', () => {
    const age = functionSource(projects, 'formatProjectConversationPreviewAge', 'getProjectConversationModeLabel');

    assert.match(age, /date\.getFullYear\(\)/);
    assert.match(age, /date\.getMonth\(\) \+ 1/);
    assert.match(age, /date\.getDate\(\)/);
    assert.match(age, /date\.getHours\(\)/);
    assert.match(age, /date\.getMinutes\(\)/);
    assert.match(age, /chat\.conversationPreviewDateTime/);
    assert.doesNotMatch(age, /elapsedMs|conversationPreviewDays|conversationPreviewHours/);
    assert.match(zh, /"conversationPreviewDateTime": "(?:\{\{year\}\}年\{\{month\}\}月\{\{day\}\}日 \{\{hour\}\}:\{\{minute\}\}|\{\{year\}\}-\{\{month\}\}-\{\{day\}\} \{\{hour\}\}:\{\{minute\}\})"/);
    assert.match(en, /"conversationPreviewDateTime": "\{\{year\}\}-\{\{month\}\}-\{\{day\}\} \{\{hour\}\}:\{\{minute\}\}"/);
});

test('Chat hover preview shows title and time on separate lines and preserves more-title content', () => {
    const titleStyles = cssBlock(styles, '.project-conversation-preview-title');

    assert.match(styles, /\.conversation-sidebar\s*\{[\s\S]*?width: 320px;/);
    assert.match(styles, /\.project-conversation-preview\s*\{[\s\S]*?width: min\(340px, calc\(100vw - 32px\)\);/);
    assert.match(styles, /\.project-conversation-preview-header\s*\{[\s\S]*?flex-direction: column;[\s\S]*?align-items: flex-start;/);
    assert.match(titleStyles, /-webkit-line-clamp: 2;/);
    assert.doesNotMatch(titleStyles, /white-space: nowrap;/);
});

test('Project preview task stats use closed rings to avoid refresh arrows becoming jagged at small sizes', () => {
    const source = functionSource(projects, 'ensureProjectFolderPreview', 'positionProjectFolderPreview');

    assert.match(source, /class="project-folder-preview-stats"/);
    assert.match(source, /<circle cx="12" cy="12" r="8" stroke="currentColor" stroke-width="1\.5"\/>/);
    assert.doesNotMatch(source, /H21v5l-2-2/);
    assert.match(styles, /\.project-folder-preview-stats svg \{\s*width: 16px;\s*height: 16px;\s*overflow: visible;/);
});

test('Chat hover preview uses styled agent mode badge', () => {
    const projects = fs.readFileSync('web/static/js/projects.js', 'utf8');

    assert.match(projects, /function getProjectConversationModeIconClass\(conversation\)/);
    assert.match(projects, /project-conversation-preview-mode-icon agent-mode-logo agent-mode-logo--default/);
    assert.match(projects, /agent-mode-logo__svg/);
    assert.match(projects, /<rect x="3" y="11" width="18" height="10" rx="2"/);
    assert.match(projects, /agent-mode-logo--' \+ getProjectConversationModeIconClass\(conversation\)/);
    assert.match(styles, /\.agent-mode-logo\s*\{[\s\S]*?background: transparent;/);
    assert.match(styles, /\.agent-mode-logo__svg\s*\{[\s\S]*?stroke: currentColor;[\s\S]*?stroke-width: 1\.9;/);
    assert.match(styles, /\.project-conversation-preview-mode-icon\.agent-mode-logo\s*\{[\s\S]*?width: 16px;/);
    assert.doesNotMatch(cssBlock(styles, '.agent-mode-logo'), /linear-gradient|box-shadow: 0 5px/);
});
