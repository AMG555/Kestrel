const fs = require('node:fs');
const test = require('node:test');
const assert = require('node:assert/strict');

const chat = fs.readFileSync('web/static/js/chat.js', 'utf8');
const hitl = fs.readFileSync('web/static/js/hitl.js', 'utf8');

function functionSource(source, name, nextName) {
    const start = source.indexOf(`function ${name}(`);
    const end = source.indexOf(`function ${nextName}(`, start);
    assert.notEqual(start, -1, `${name} should exist`);
    assert.notEqual(end, -1, `${nextName} should follow ${name}`);
    return source.slice(start, end);
}

test('Existing session with missing local config does not inherit recent approval settings from other sessions', () => {
    const source = functionSource(chat, 'getHitlConfigForConversation', 'setHitlReviewerUI');
    const existingConversationBranch = source.slice(source.indexOf('const key = getHitlStorageKeyByConversation(cid)'));

    assert.doesNotMatch(existingConversationBranch, /getHitlLastGlobalConfig/);
    assert.match(existingConversationBranch, /if \(!raw\) \{\s*return fallback;/);
    assert.match(existingConversationBranch, /catch \(e\) \{\s*return fallback;/);
});

test('Server default approver only updates the default value, does not overwrite recent session selection', () => {
    const source = functionSource(hitl, 'applyHitlDefaultReviewerFromServer', 'fetchHitlDefaultReviewer');

    assert.match(source, /window\.csaiHitlDefaultReviewer = (?:reviewer|v)/);
    assert.doesNotMatch(source, /saveHitlLastGlobalConfig/);
});

test('Resuming session approval config preserves that session own approver', () => {
    const source = functionSource(hitl, 'syncHitlConfigFromServer', 'syncHitlConfigToServerByCurrentConversation');

    assert.match(source, /const localReviewer = hitlReviewerNormalize\(local && local\.reviewer\)/);
    assert.match(source, /merged = \{[\s\S]*?reviewer: localReviewer/);
    assert.match(source, /saveHitlConversationConfig\(conversationId, \{[\s\S]*?reviewer: localReviewer/);
    assert.doesNotMatch(source, /getHitlLastGlobalConfig/);
});

test('Async sync can only refresh the approval UI for sessions still in the current session', () => {
    const source = functionSource(hitl, 'syncHitlConfigFromServer', 'syncHitlConfigToServerByCurrentConversation');

    assert.match(source, /getCurrentConversationIdForHitl\(\) === conversationId[\s\S]*?window\.applyHitlConfigToUI\(normalizedCfg\)/);
});
