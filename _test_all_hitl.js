const fs = require('fs');
const vm = require('vm');
const assert = require('assert/strict');

const monitor = fs.readFileSync('web/static/js/monitor.js', 'utf8');
const chatScroll = fs.readFileSync('web/static/js/chat-scroll.js', 'utf8');
const projects = fs.readFileSync('web/static/js/projects.js', 'utf8');
const chat = fs.readFileSync('web/static/js/chat.js', 'utf8');
const styles = fs.readFileSync('web/static/css/style.css', 'utf8');
const template = fs.readFileSync('web/templates/index.html', 'utf8');
const handler = fs.readFileSync('internal/handler/hitl.go', 'utf8');
const zh = JSON.parse(fs.readFileSync('web/static/i18n/zh-CN.json', 'utf8'));
const en = JSON.parse(fs.readFileSync('web/static/i18n/en-US.json', 'utf8'));

const testFile = fs.readFileSync('web/static/js/hitl-approval-ui.test.cjs', 'utf8');

// Parse test blocks
const tests = [];
const regex = /test\('([^']+)',\s*(?:async\s*)?\(\)\s*=>\s*\{([\s\S]*?)\n\}\);/g;
let match;
while ((match = regex.exec(testFile)) !== null) {
    tests.push({ name: match[1], body: match[2] });
}

console.log('Total tests found:', tests.length);

for (const t of tests) {
    try {
        const fn = new Function('assert', 'fs', 'vm', 'monitor', 'chatScroll', 'projects', 'chat', 'styles', 'template', 'handler', 'zh', 'en', t.body);
        fn(assert, fs, vm, monitor, chatScroll, projects, chat, styles, template, handler, zh, en);
        console.log('PASS:', t.name);
    } catch (e) {
        const firstLine = (e.message || '').split('\n')[0];
        console.log('FAIL:', t.name, '->', firstLine);
    }
}
