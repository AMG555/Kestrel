const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');

const { deriveProgress, applyTaskUpdate } = require('./chat-plan-progress.js');

test('Task progress: prioritise in-progress step and retain completed items', () => {
    const progress = deriveProgress([
        { id: '1', subject: 'Gather requirements', status: 'completed' },
        { id: '2', subject: 'Implement component', activeForm: 'Implementing component', status: 'in_progress' },
        { id: '3', subject: 'Browser verification', status: 'pending' }
    ]);
    assert.equal(progress.activeStep, 2);
    assert.equal(progress.completed, 1);
    assert.equal(progress.total, 3);
    assert.equal(progress.allCompleted, false);
});

test('TaskUpdate: tick immediately on success, final step shows AllComplete', () => {
    const initial = [
        { id: '1', subject: 'API', status: 'completed' },
        { id: '2', subject: 'UI', status: 'in_progress' }
    ];
    const updated = applyTaskUpdate(initial, { taskId: '2', status: 'completed' });
    const progress = deriveProgress(updated);
    assert.equal(progress.activeStep, 2);
    assert.equal(progress.completed, 2);
    assert.equal(progress.allCompleted, true);
});

test('Deleted tasks do not appear in the floating task list', () => {
    const tasks = applyTaskUpdate([
        { id: '1', subject: 'Keep', status: 'pending' },
        { id: '2', subject: 'Delete', status: 'pending' }
    ], { taskId: '2', status: 'deleted' });
    assert.deepEqual(tasks.map((task) => task.id), ['1']);
});

test('Task progress style follows system theme variables instead of hardcoded dark', () => {
    const css = fs.readFileSync('web/static/css/chat-plan-progress.css', 'utf8');
    assert.match(css, /--agent-plan-surface:\s*var\(--card-bg\)/);
    assert.match(css, /background:\s*var\(--agent-plan-surface\)/);
    assert.match(css, /color:\s*var\(--agent-plan-text\)/);
    assert.doesNotMatch(css, /background:\s*#(?:292929|2b2b2b|303030)/i);
});

test('Back-to-latest button and task progress use stacked avoidance layout when both visible', () => {
    const css = fs.readFileSync('web/static/css/chat-plan-progress.css', 'utf8');
    const template = fs.readFileSync('web/templates/index.html', 'utf8');
    assert.match(css, /\.chat-return-latest:not\(\[hidden\]\)\s*\+\s*\.agent-plan-progress:not\(\[hidden\]\)/);
    assert.match(css, /--agent-plan-trigger-bottom:\s*64px/);
    assert.match(css, /--agent-plan-panel-bottom:\s*120px/);
    assert.match(css, /bottom:\s*var\(--agent-plan-trigger-bottom\)/);
    assert.match(css, /bottom:\s*var\(--agent-plan-panel-bottom\)/);
    assert.match(template, /<button[^>]+id="chat-return-latest"[\s\S]*?<\/button>\s*<div id="agent-plan-progress"/);
});

test('Plan details only expand after a real mouse move or explicit user action', () => {
    const css = fs.readFileSync('web/static/css/chat-plan-progress.css', 'utf8');
    const source = fs.readFileSync('web/static/js/chat-plan-progress.js', 'utf8');
    assert.doesNotMatch(css, /\.agent-plan-progress:hover\s+\.agent-plan-progress-panel/);
    assert.match(css, /\.agent-plan-progress\.is-hover-active\s+\.agent-plan-progress-panel/);
    assert.match(source, /host\.addEventListener\('pointermove',\s*armHoverAfterPointerMove\)/);
    assert.match(source, /returnLatestButton\.addEventListener\('pointerdown',\s*disarmPassiveHover\)/);
    assert.match(source, /passiveHoverAnchor = \{ x: event\.clientX, y: event\.clientY \}/);
    assert.match(source, /event\.clientX === passiveHoverAnchor\.x && event\.clientY === passiveHoverAnchor\.y/);
    assert.match(source, /host\.classList\.remove\('is-hover-active'\)/);
});

test('Server-determined task stop immediately clears stale task cards', () => {
    const source = fs.readFileSync('web/static/js/chat-plan-progress.js', 'utf8');
    assert.match(source, /payload && payload\.running === false/);
    assert.match(source, /state\.tasks = \[\][\s\S]{0,160}state\.expanded = false/);
});
