const fs = require('fs');
const cp = require('child_process');

const out = cp.spawnSync('node', ['--test', 'web/static/js/hitl-approval-ui.test.cjs'], { encoding: 'utf8' });
const lines = out.stdout.split(/\r?\n/);
let currentTest = '';
for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.startsWith('✖ failing tests:')) {
        console.log(lines.slice(i, i + 80).join('\n'));
        break;
    }
}
