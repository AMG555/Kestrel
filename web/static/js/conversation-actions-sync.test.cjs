const fs = require('node:fs');
const test = require('node:test');
const assert = require('node:assert/strict');

const chat = fs.readFileSync('web/static/js/chat.js', 'utf8');
const projects = fs.readFileSync('web/static/js/projects.js', 'utf8');
const template = fs.readFileSync('web/templates/index.html', 'utf8');

function functionSource(source, name, nextName) {
    const start = source.indexOf(`function ${name}(`);
    const end = nextName ? source.indexOf(`function ${nextName}(`, start) : source.length;
    assert.notEqual(start, -1, `${name} should exist`);
    if (nextName) {
        assert.notEqual(end, -1, `${nextName} should follow ${name}`);
    }
    return source.slice(start, end);
}

test('Global pin checks API result and immediately notifies project folder', () => {
    const source = functionSource(chat, 'pinConversation');

    assert.match(source, /assertConversationActionResponse\(updateResponse, (?:'更新置顶状态失败'|'Failed to update pinned status')\)/);
    assert.match(source, /notifyConversationPinnedChanged\(convId, newPinned\)/);
    assert.match(source, /loadConversations\(\)/);
});

test('Pinned chats inside a project folder sort first and show the pin icon', () => {
    const sortSource = functionSource(projects, 'sortProjectFolderConversations', 'updateChatProjectConversationPinnedState');
    const itemSource = functionSource(projects, 'appendChatProjectConversationItem', 'selectChatProjectConversationItem');

    assert.match(sortSource, /Number\(!!b\?\.pinned\) - Number\(!!a\?\.pinned\)/);
    assert.match(itemSource, /if \(conversation\.pinned\)/);
    assert.match(itemSource, /project-conversation-pinned/);
});

test('Delete event immediately removes project cache and triggers authoritative refresh', () => {
    const removeSource = functionSource(projects, 'removeChatProjectConversation', 'refreshChatProjectFoldersAfterAction');

    assert.match(removeSource, /chatProjectFolderContext\.conversations = chatProjectFolderContext\.conversations\.filter/);
    assert.match(projects, /document\.addEventListener\('conversation-deleted',[\s\S]{0,300}removeChatProjectConversation\(conversationId\)[\s\S]{0,180}refreshChatProjectFoldersAfterAction\(\)/);
    assert.match(chat, /document\.dispatchEvent\(new CustomEvent\('conversation-deleted'/);
});

test('Older project folder requests cannot overwrite newer operation results', () => {
    const source = functionSource(projects, 'loadChatProjectFolderContext', 'getProjectConversationSortTime');

    assert.match(source, /const loadSeq = \+\+chatProjectFolderContextLoadSeq/);
    assert.match(source, /if \(loadSeq !== chatProjectFolderContextLoadSeq\) return false/);
});

test('Project folder menu can pin and immediately updates sort', () => {
    const toggleSource = functionSource(projects, 'toggleProjectPinnedFromListMenu', 'initProjectListActionMenu');
    const folderSource = functionSource(projects, 'appendChatProjectFolderItem', 'appendChatProjectConversationItem');

    assert.match(template, /onclick="toggleProjectPinnedFromListMenu\(\)"/);
    assert.match(toggleSource, /JSON\.stringify\(\{ pinned: nextPinned \}\)/);
    assert.match(toggleSource, /updateCachedProjectPinnedState\(projectId, nextPinned\)/);
    assert.match(folderSource, /if \(!isUnassigned && project\.pinned\)/);
    assert.match(folderSource, /project-folder-pinned/);
    assert.match(projects, /\[\.\.\.pinnedProjects, unassignedProject, \.\.\.regularProjects\]/);
});

test('Chat sidebar retains only the recent chats section', () => {
    assert.doesNotMatch(template, /class="conversation-groups-section"/);
    assert.doesNotMatch(template, /id="conversation-groups-list"/);
});

test('Chat three-dot menu still binds to open context menu', () => {
    const itemSource = functionSource(chat, 'createConversationListItemWithMenu', 'openConversationContextMenuForId');
    const menuSource = functionSource(chat, 'showConversationContextMenu', 'ensureConversationRenameModal');

    assert.match(itemSource, /menuBtn\.onclick = \(e\) => openConversationContextMenuForId\(e, conversation\.id, conversation\.title \|\| ''\)/);
    assert.match(menuSource, /const menu = document\.getElementById\('conversation-context-menu'\)/);
    assert.match(menuSource, /menu\.style\.display = 'block'/);
    assert.match(chat, /function clearDownloadMarkdownSubmenuHideTimeout\(/);
    assert.match(chat, /function handleDownloadMarkdownSubmenuEnter\(/);
    assert.match(chat, /function handleDownloadMarkdownSubmenuLeave\(/);
    assert.match(chat, /function hideDownloadMarkdownSubmenu\(/);
});
