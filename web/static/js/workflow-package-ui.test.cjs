const fs = require('node:fs');
const test = require('node:test');
const assert = require('node:assert/strict');

test('Workflows provide import, export and overwrite confirm containers', () => {
    const html = fs.readFileSync('web/templates/index.html', 'utf8');
    const zh = JSON.parse(fs.readFileSync('web/static/i18n/zh-CN.json', 'utf8'));
    assert.match(html, /onclick="openWorkflowPackageImportModal\(\)"/);
    assert.match(html, /onclick="[^"]*exportCurrentWorkflowPackage\(\)"/);
    assert.match(html, /id="workflow-package-import-modal"/);
    assert.match(html, /id="workflow-package-overwrite-modal"/);
    assert.equal(typeof zh.workflows.package.importLocal, 'string');
});

test('Local package upload supports drag and drop, keyboard selection and selected file feedback', () => {
    const html = fs.readFileSync('web/templates/index.html', 'utf8');
    const workflows = fs.readFileSync('web/static/js/workflows.js', 'utf8');
    const css = fs.readFileSync('web/static/css/style.css', 'utf8');
    assert.match(html, /id="workflow-package-dropzone"[\s\S]*?ondrop="onWorkflowPackageDrop\(event\)"/);
    assert.match(html, /onkeydown="onWorkflowPackageDropzoneKeydown\(event\)"/);
    assert.match(html, /id="workflow-package-selected-file"/);
    assert.match(workflows, /window\.onWorkflowPackageDrop = function \(event\)/);
    assert.match(workflows, /endsWith\('\.csapkg\.zip'\)/);
    assert.match(css, /\.workflow-package-dropzone\.is-dragging/);
});

test('Workflows script calls all package contract endpoints and conflict error codes', () => {
    const workflows = fs.readFileSync('web/static/js/workflows.js', 'utf8');
    const client = fs.readFileSync('web/static/js/workflow-package-client.js', 'utf8');
    assert.match(workflows, /\/api\/workflows\/\$\{encodeURIComponent\(id\)\}\/package/);
    assert.match(client, /\/api\/workflow-package-inspections/);
    assert.match(client, /\/api\/workflow-package-imports/);
    assert.match(workflows, /WFPKG_CONFLICT_CHANGED/);
    assert.match(workflows, /WFPKG_INSPECTION_EXPIRED/);
});

test('Preflight invalid package contract error codes all have status branches', () => {
    const workflows = fs.readFileSync('web/static/js/workflows.js', 'utf8');
    [
        'WFPKG_INVALID_ARCHIVE',
        'WFPKG_UNSUPPORTED_FORMAT',
        'WFPKG_INVALID_MANIFEST',
        'WFPKG_CHECKSUM_MISMATCH',
        'WFPKG_MULTIPLE_WORKFLOWS',
        'WFPKG_WORKFLOW_INVALID'
    ].forEach((code) => assert.match(workflows, new RegExp(code)));
});

test('Import request ID generation failure triggers error handling', () => {
    const workflows = fs.readFileSync('web/static/js/workflows.js', 'utf8');
    assert.match(workflows, /async function performWorkflowPackageImport\(request\)[\s\S]*?try\s*\{[\s\S]*?client\.createIdempotencyKey\(\)[\s\S]*?catch \(error\) \{\s*displayWorkflowPackageError\(error\)/);
});

test('Workflow package dynamic status provides locale entries', () => {
    const zh = JSON.parse(fs.readFileSync('web/static/i18n/zh-CN.json', 'utf8'));
    const en = JSON.parse(fs.readFileSync('web/static/i18n/en-US.json', 'utf8'));
    const keys = [
        ['errors', 'invalidArchive'],
        ['conflict', 'idConflict'],
        ['summary', 'workflowName'],
        ['resolution', 'keepExisting'],
        ['result', 'overwritten']
    ];
    keys.forEach(([section, key]) => {
        assert.equal(typeof zh.workflows.package[section][key], 'string');
        assert.equal(typeof en.workflows.package[section][key], 'string');
    });
});

test('Language switch refreshes workflow package modal and dynamic status', () => {
    const workflows = fs.readFileSync('web/static/js/workflows.js', 'utf8');
    assert.match(workflows, /function refreshWorkflowsI18n\(\)[\s\S]*?workflow-package-import-modal[\s\S]*?workflow-package-overwrite-modal[\s\S]*?renderWorkflowPackageInspection\(\)[\s\S]*?renderWorkflowPackageResolution\(\)/);
});

test('Opening import modal starts new import session each time', () => {
    const workflows = fs.readFileSync('web/static/js/workflows.js', 'utf8');
    const start = workflows.indexOf('window.openWorkflowPackageImportModal = async function ()');
    const end = workflows.indexOf('window.closeWorkflowPackageImportModal = function ()', start);
    const openHandler = workflows.slice(start, end);
    assert.match(openHandler, /resetWorkflowPackageImport\(\);/);
    assert.doesNotMatch(openHandler, /restoreWorkflowPackageState\(\)/);
});

test('Uncommitted import results display skip or keep instead of import complete', () => {
    const vm = require('node:vm');
    const source = fs.readFileSync('web/static/js/workflows.js', 'utf8');
    const start = source.indexOf('    function renderWorkflowPackageImportResult(');
    const end = source.indexOf('    async function restoreWorkflowPackageState(', start);
    for (const [result, expected] of [['skipped_identical', 'Import skipped'], ['kept_existing', 'Local version kept'], ['created', 'Import completed'], ['overwritten', 'Import completed'], ['renamed', 'Import completed']]) {
        const button = {};
        const c = vm.createContext({ workflowPackageText: (_, fallback) => fallback, renderWorkflowPackageResolution() {}, workflowPackageSubmitBtn: () => button });
        vm.runInContext(source.slice(start, end), c);
        c.renderWorkflowPackageImportResult({ result });
        assert.equal(button.textContent, expected);
        assert.equal(button.disabled, true);
    }
});

test('Duplicate package confirmation distinguishes skip and keep while preserving semantics', () => {
    const vm = require('node:vm');
    const source = fs.readFileSync('web/static/js/workflows.js', 'utf8');
    const start = source.indexOf('    function renderWorkflowPackageResolution(');
    const end = source.indexOf('    function resetWorkflowPackageImport(', start);
    for (const [state, action, expected] of [['identical', 'keep_existing', 'Confirm skip import'], ['id_conflict', 'keep_existing', 'Confirm keep local version'], ['none', 'create', 'Confirm create'], ['id_conflict', 'overwrite', 'Continue and confirm overwrite'], ['id_conflict', 'rename', 'Confirm save as new']]) {
        const button = {};
        const c = vm.createContext({ workflowPackageState: { inspection: { conflict: { state } }, resolutionAction: action, riskChoicesVisible: true, newWorkflowId: 'copy' }, workflowPackageResolutionEl: () => ({}), workflowPackageSubmitBtn: () => button, workflowPackageClient: () => ({ allowedActions: () => [action] }), workflowPackageText: (_, fallback) => fallback, esc: value => value, workflowPackageResolutionCard: () => '', workflowPackageSetStep() {} });
        vm.runInContext(source.slice(start, end), c);
        c.renderWorkflowPackageResolution();
        assert.equal(button.textContent, expected);
        assert.equal(button.disabled, false);
    }
});
