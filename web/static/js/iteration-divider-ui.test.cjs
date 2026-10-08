const fs = require('node:fs');
const test = require('node:test');
const assert = require('node:assert/strict');

const monitor = fs.readFileSync('web/static/js/monitor.js', 'utf8');
const styles = fs.readFileSync('web/static/css/style.css', 'utf8');

function functionSource(source, name, nextName) {
    const start = source.indexOf(`function ${name}(`);
    const end = source.indexOf(`function ${nextName}(`, start);
    assert.notEqual(start, -1, `${name} should exist`);
    assert.notEqual(end, -1, `${nextName} should follow ${name}`);
    return source.slice(start, end);
}

test('Main agent iteration node gets accessible divider semantics', () => {
    const source = functionSource(monitor, 'addTimelineItem', 'loadActiveTasks');

    assert.match(source, /if \(type === 'iteration'\)/);
    assert.match(source, /if \(scope !== 'sub'\)/);
    assert.match(source, /classList\.add\('timeline-iteration-divider'\)/);
    assert.match(source, /setAttribute\('role', 'separator'\)/);
    assert.match(source, /setAttribute\('aria-label', String\(options\.title \|\| ''\)\)/);
});

test('Iteration divider only uses lightweight gradient horizontal line in main chat timeline', () => {
    assert.match(styles, /\.timeline-item-iteration\.timeline-iteration-divider::after/);
    assert.match(styles, /linear-gradient\(/);
    assert.match(styles, /color-mix\(in srgb, var\(--border-color\) 88%, transparent\)/);
    assert.match(styles, /@media \(max-width: 768px\)/);
});
