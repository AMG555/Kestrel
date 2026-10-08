const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');

test('tool_result merges back to tool call card and back-fills empty params', () => {
    const source = fs.readFileSync('web/static/js/monitor.js', 'utf8');
    assert.match(source, /function toolCallArgsEmpty\(args\)/);
    assert.match(source, /const resultArgs = parseToolCallArgsFromData\(data\);/);
    assert.match(source, /toolCallArgsEmpty\(state\.args\) && !toolCallArgsEmpty\(resultArgs\)/);
    assert.match(source, /state\.args = resultArgs;/);
});

test('When merging historical process_details, tool_call params are also filled from tool_result', () => {
    const source = fs.readFileSync('web/static/js/monitor.js', 'utf8');
    assert.match(source, /function absorbResult\(targetDetail, resultDetail\)/);
    assert.match(source, /targetDetail\.data\.argumentsObj = resultArgs;/);
    assert.match(source, /targetDetail\.data\.arguments = JSON\.stringify\(resultArgs\);/);
});

test('Multiple calls with the same toolCallId merge results in FIFO order to avoid result record loss from overwriting', () => {
    const source = fs.readFileSync('web/static/js/monitor.js', 'utf8');
    assert.match(source, /list\.push\(copy\)/);
    assert.match(source, /const candidate = list\.shift\(\)/);
    assert.match(source, /callName === resultName/);
});
