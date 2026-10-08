const fs = require('fs');
const files = fs.readdirSync('web/static/js').filter(f => f.endsWith('.js'));
for (const f of files) {
    let code = fs.readFileSync('web/static/js/' + f, 'utf8');
    const matches = code.match(/['"]HITL\.[a-zA-Z0-9_\-]+['"]/g);
    if (matches) {
        console.log(f, 'has', matches.length, 'occurrences of HITL.');
        code = code.replace(/['"]HITL\.([a-zA-Z0-9_\-]+)['"]/g, "'hitl.$1'");
        fs.writeFileSync('web/static/js/' + f, code, 'utf8');
    }
}
