const fs = require('fs');
const monitor = fs.readFileSync('web/static/js/monitor.js', 'utf8');
const caseIdx = monitor.indexOf("case 'conversation':");
const targetIdx = monitor.indexOf("window.refreshChatProjectFolders()");
console.log('caseIdx:', caseIdx, 'targetIdx:', targetIdx, 'distance:', targetIdx - caseIdx);
