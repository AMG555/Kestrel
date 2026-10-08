import glob, subprocess, re

rep_dict = {
    'display infoValue': 'displayInfoValue',
    'chatFilescurrent searchQuery': 'chatFilesCurrentSearchQuery',
    'dashboardAlertcurrent IsStrictSubsetOfLastShown': 'dashboardAlertCurrentIsStrictSubsetOfLastShown',
    'is infoCollectFullenabled': 'isInfoCollectFullEnabled',
    'get infoCollectFullOption': 'getInfoCollectFullOption',
    'tool info': 'toolInfo',
    'markcurrent ProjectConversationviewed': 'markCurrentProjectConversationViewed',
    'model info': 'modelInfo',
    'itemsOncurrent Page': 'itemsOnCurrentPage',
    ' itemsOncurrent Page': ' itemsOnCurrentPage',
    "auditT(\\'pageInfo\\',": "auditT('pageInfo',",
    "hitlPaginationT(\\'pageInfo\\',": "hitlPaginationT('pageInfo',",
    'exportcurrent WorkflowPackage': 'exportCurrentWorkflowPackage',
    'current searchQuery': 'currentSearchQuery',
    'esc( page infoText)': 'esc(pageInfoText)',
    'session infoRow': 'sessionInfoRow',
    'chatFilesBrowseCanMutatecurrent Path': 'chatFilesBrowseCanMutateCurrentPath',
    'init infoCollectProviderSelect': 'initInfoCollectProviderSelect',
    'result info': 'resultInfo',
    'createvulnerabilityFromcurrent Fact': 'createVulnerabilityFromCurrentFact',
    'renderSession infoPanel': 'renderSessionInfoPanel',
    'canMutatecurrent Path': 'canMutateCurrentPath',
    'syncHitlConfigToServerBycurrent Conversation': 'syncHitlConfigToServerByCurrentConversation',
    'bind infoCollectPresetEvents': 'bindInfoCollectPresetEvents',
    'ownscurrent Attach': 'ownsCurrentAttach',
    'submitcurrent Line': 'submitCurrentLine',
    'hitlcurrent AuditEngine': 'hitlCurrentAuditEngine',
    'refresh infoCollectProviderUI': 'refreshInfoCollectProviderUI',
    "mcpMonitorT(' page info', {  page:  page, total: totalPages || 1 })": "mcpMonitorT('pageInfo', { page: page, total: totalPages || 1 })",
    'const  page infoText =': 'const pageInfoText =',
    'escapeHtml( page infoText)': 'escapeHtml(pageInfoText)',
    'pagination- page': 'pagination-page',
    'set infoCollectQueryMode': 'setInfoCollectQueryMode',
    'schedule infoCollect': 'scheduleInfoCollect',
    'bind infoCollect': 'bindInfoCollect',
    'sync infoCollect': 'syncInfoCollect',
    'render infoCollect': 'renderInfoCollect',
    'update infoCollect': 'updateInfoCollect',
    'handle infoCollect': 'handleInfoCollect',
    "' page'": "'page'",
    "' page_size'": "'page_size'",
    '" page"': '"page"',
    '" page_size"': '"page_size"',
    "' page info'": "'pageInfo'",
    "' page infoText'": "'pageInfoText'",
    'forexport': 'forExport',
}

for f in sorted(glob.glob('web/static/js/*.js')):
    with open(f, 'r', encoding='utf-8') as fp:
        content = fp.read()
    orig = content
    content = re.sub(r'\b([a-zA-Z0-9_$]+) infoCollect([A-Za-z0-9_$]*)', lambda m: m.group(1) + 'InfoCollect' + m.group(2), content)
    for pat, rep in rep_dict.items():
        content = content.replace(pat, rep)
    if content != orig:
        with open(f, 'w', encoding='utf-8') as fp:
            fp.write(content)
        print(f'Repaired {f}')

errors = []
for f in sorted(glob.glob('web/static/js/*.js')):
    chk = subprocess.run(['node', '-c', f], capture_output=True, text=True)
    if chk.returncode != 0:
        errors.append((f, chk.stderr.splitlines()[:2]))

print(f'\nTotal syntax errors remaining: {len(errors)}')
for f, e in errors:
    print(f'{f}: {e}')
