const fs = require('node:fs');
const vm = require('node:vm');
const test = require('node:test');
const assert = require('node:assert/strict');

const monitor = fs.readFileSync('web/static/js/monitor.js', 'utf8');
const chatScroll = fs.readFileSync('web/static/js/chat-scroll.js', 'utf8');
const projects = fs.readFileSync('web/static/js/projects.js', 'utf8');
const chat = fs.readFileSync('web/static/js/chat.js', 'utf8');
const styles = fs.readFileSync('web/static/css/style.css', 'utf8');
const template = fs.readFileSync('web/templates/index.html', 'utf8');
const handler = fs.readFileSync('internal/handler/hitl.go', 'utf8');
const zh = JSON.parse(fs.readFileSync('web/static/i18n/zh-CN.json', 'utf8'));
const en = JSON.parse(fs.readFileSync('web/static/i18n/en-US.json', 'utf8'));

test('Input area provides standalone approval entry and exposes configurable wait timeout', () => {
    assert.match(template, /id="chat-hitl-approval-dock"/);
    assert.match(template, /id="hitl-timeout-select"/);
    assert.match(template, /option value="300" selected/);
    assert.match(chat, /DEFAULT_HITL_TIMEOUT_SECONDS = 300/);
    assert.match(chat, /timeoutSeconds: normalizeHitlTimeoutForChat/);
    assert.match(chat, /body\.hitl = \{[\s\S]*?timeoutSeconds: normalizeHitlTimeoutForChat\(hitlCfg\.timeoutSeconds/);
});

test('Overlong manual approval content scrolls in height-limited area and action buttons remain visible', () => {
    assert.match(styles, /\.chat-hitl-approval-dock \{[\s\S]*?max-height: min\(62dvh, 560px\);[\s\S]*?padding: 18px 20px 74px;[\s\S]*?overflow: hidden;/);
    assert.match(styles, /\.chat-hitl-approval-scroll-region \{[\s\S]*?max-height: max\(76px, calc\(min\(62dvh, 560px\) - 94px\)\);[\s\S]*?overflow-y: auto;[\s\S]*?overscroll-behavior: contain;/);
    assert.match(styles, /\.chat-hitl-approval-dock \.hitl-edit-args \{[\s\S]*?max-height: min\(28dvh, 220px\);[\s\S]*?overflow: auto;/);
    assert.match(styles, /\.chat-hitl-approval-dock \.hitl-inline-actions \{[\s\S]*?position: absolute;[\s\S]*?bottom: 16px;[\s\S]*?box-shadow: none;/);
    assert.match(styles, /\.chat-hitl-approval-dock \.hitl-approval-heading h3 \{[\s\S]*?-webkit-line-clamp: 3;/);
    assert.match(monitor, /function wrapChatHitlApprovalScrollRegion\(dock\)/);
    assert.match(monitor, /while \(dock\.firstChild && dock\.firstChild !== actions\)/);
    assert.match(monitor, /wrapChatHitlApprovalScrollRegion\(dock\);/);
    assert.match(monitor, /url\.length > 160[\s\S]*?requestVisitLongUrl/);
    assert.ok(zh.hitl.requestVisitLongUrl === '允许 Kestrel 访问此地址？' || zh.hitl.requestVisitLongUrl === 'Allow Kestrel to visit this address?');
    assert.equal(en.hitl.requestVisitLongUrl, 'Allow Kestrel to visit this address?');
});

test('RefreshResume session: complete authoritative approval config sync before allowing send', () => {
    assert.match(chat, /function waitForHitlConfigReady\(conversationId\)/);
    assert.match(chat, /await waitForHitlConfigReady\(hitlConversationAtSendStart\)/);
    assert.match(chat, /hitlConfigSyncConversationId = conversationId;[\s\S]{0,240}await hitlConfigSyncPromise;/);
    assert.match(chat, /await hitlConfigSyncPromise;[\s\S]{0,220}seq !== loadConversationRequestSeq/);
    assert.match(fs.readFileSync('web/static/js/hitl.js', 'utf8'), /window\.csaiHitlDefaultReviewerReady = (?:initHitlDefaultReviewerFromServer\(\)|window\.csaiHitlDefaultConfigReady;)/);
});

test('Approval config writes for the same session are serialised to prevent stale requests overwriting newer choices', () => {
    const hitlPage = fs.readFileSync('web/static/js/hitl.js', 'utf8');
    assert.match(hitlPage, /const hitlConversationConfigSaveQueues = new Map\(\)/);
    assert.match(hitlPage, /const previous = hitlConversationConfigSaveQueues\.get\(normalizedConversationId\) \|\| Promise\.resolve\(\)/);
    assert.match(hitlPage, /const queued = previous\.catch\(function \(\) \{\}\)\.then\(async function \(\)/);
});

test('Input can get model per session channel and bidirectionally sync session reasoning; approval model only appears at Audit Agent entry', () => {
    assert.match(chat, /function currentSystemModelLabel\(\)/);
    assert.match(chat, /chatDefaultAIChannel \? chatAIChannels\[chatDefaultAIChannel\]/);
    assert.match(chat, /function currentHitlAuditModelLabel\(\)/);
    assert.match(chat, /const label = currentChatModelLabel\(\)/);
    assert.doesNotMatch(chat, /const label = data\.model \|\| currentChatModelLabel\(\)/);
    assert.match(chat, /const approvalModel = auditAgent \? currentHitlAuditEngineLabel\(\) : ''/);
    assert.match(chat, /hitlAuditModel\.model\.trim\(\)/);
    assert.match(template, /id="chat-model-shortcut"[^>]+onclick="openChatSystemModelPicker\(event\)"/);
    assert.match(template, /id="chat-system-model-menu"[^>]+hidden/);
    assert.doesNotMatch(template, /id="chat-reasoning-shortcut"/);
    assert.doesNotMatch(template, /session-settings-group-ai/);
    assert.match(template, /class="chat-ai-session-state" hidden[\s\S]{0,500}id="chat-ai-channel-select"/);
    assert.match(template, /openChatSystemModelView\('channel', event\)[\s\S]{0,1200}openChatSystemModelView\('model', event\)[\s\S]{0,1200}openChatSystemModelView\('mode', event\)[\s\S]{0,1200}openChatSystemModelView\('effort', event\)/);
    assert.match(chat, /function renderChatReasoningEffortOptions\(\)/);
    assert.match(chat, /function renderChatReasoningModeOptions\(\)/);
    assert.match(chat, /case 'low': return 'low'[\s\S]{0,300}case 'max': return 'max'/);
    assert.match(chat, /chatTranslate\('chat\.reasoningEffortUnset', 'Not specified'\)/);
    assert.match(chat, /\['default', 'off', 'on', 'auto'\]/);
    assert.match(chat, /\['', 'low', 'medium', 'high', 'xhigh', 'max'\]/);
    assert.match(chat, /function selectChatReasoningMode\(mode\)[\s\S]{0,700}modeControl\.value = chosen[\s\S]{0,200}finishChatReasoningPickerUpdate\(\)/);
    assert.match(chat, /function selectChatReasoningEffort\(effort\)[\s\S]{0,700}effortControl\.value = chosen[\s\S]{0,200}finishChatReasoningPickerUpdate\(\)/);
    assert.match(chat, /function fetchChatSystemModelsForChannel\(channelId, options\)[\s\S]{0,4200}apiFetch\('\/api\/config\/list-models'/);
    assert.match(chat, /function selectChatAIChannel\(channelId\)[\s\S]{0,900}fetchChatSystemModelsForChannel\(resolveChatPickerChannelId\(\), \{ force: true \}\)/);
    assert.match(chat, /const chatSystemModelCache = new Map\(\)/);
    assert.match(chat, /Date\.now\(\) - cached\.fetchedAt < CHAT_SYSTEM_MODEL_CACHE_TTL_MS/);
    assert.match(chat, /function selectChatSystemModel\(model\)[\s\S]{0,2600}method: 'PUT'[\s\S]{0,900}apiFetch\('\/api\/config\/apply'/);
    assert.match(chat, /body: JSON\.stringify\(\{ ai: state\.ai \}\)/);
    assert.ok(zh.chat.modelSettingsAria === '选择 AI channel、Model与推理设置' || zh.chat.modelSettingsAria === 'Choose AI channel, model, and reasoning settings');
    assert.equal(en.chat.modelSettingsAria, 'Choose AI channel, model, and reasoning settings');
    assert.ok(zh.chat.reasoningSessionUpdated === 'Session reasoning settings updated' || zh.chat.reasoningSessionUpdated === 'Session reasoning updated' || zh.chat.reasoningSessionUpdated === '会话推理设置已更新');
    assert.equal(en.chat.reasoningSessionUpdated, 'Session reasoning updated');
});

test('Approval request dynamically describes by browser, command, file, and generic tool', () => {
    assert.match(monitor, /function hitlApprovalTemplate/);
    assert.match(monitor, /hitlApprovalTranslate\(key, fallback\)/);
    assert.match(monitor, /replaceAll\('\{\{' \+ name \+ '\}\}'/);
    assert.match(monitor, /function describeHitlApprovalRequest/);
    assert.match(monitor, /requestVisitUrl/);
    assert.match(monitor, /requestCommand/);
    assert.match(monitor, /requestFile/);
    assert.match(monitor, /requestGeneric/);
    assert.match(monitor, /let displayTool = rawToolName/);
    assert.doesNotMatch(monitor, /displayTool = 'Browser'/);
    assert.doesNotMatch(monitor, /displayTool = hitlApprovalTranslate\('hitl\.toolTerminal'/);
    assert.doesNotMatch(monitor, /displayTool = hitlApprovalTranslate\('hitl\.toolFiles'/);
});

test('Agent review does not enter manual approval dialog, countdown, or project count', () => {
    const logsHandler = fs.readFileSync('internal/handler/hitl_logs.go', 'utf8');
    const hitlPage = fs.readFileSync('web/static/js/hitl.js', 'utf8');
    assert.match(handler, /CreatePendingInterrupt\([\s\S]{0,260}reviewer string/);
    assert.match(handler, /reviewer != "audit_agent"[\s\S]{0,120}m\.pending\[id\] = p/);
    assert.match(logsHandler, /ADD COLUMN reviewer TEXT NOT NULL DEFAULT 'human'/);
    assert.match(logsHandler, /status = 'pending' AND COALESCE\(reviewer,'human'\) = 'human'/);
    assert.match(monitor, /function isAgentReviewedHitl\(data\)/);
    assert.match(monitor, /if \(!data\.resolved && !isAgentReviewedHitl\(data\)\)/);
    assert.match(monitor, /if \(!isAgentReviewedHitl\(data\)\) \{[\s\S]{0,240}bindHitlApprovalCountdown/);
    assert.match(monitor, /if \(isAgentReviewedHitl\(data\)\) return false/);
    assert.match(projects, /filter\(isHumanProjectPendingApproval\)/);
    assert.match(projects, /if \(!isHumanProjectPendingApproval\(details\)\) return/);
    assert.match(hitlPage, /const items = rawItems\.filter/);
});

test('Manual approval does not require a note; Review & edit only sends truly modified params', () => {
    assert.match(monitor, /if \(!approveBtn \|\| !rejectBtn \|\| !statusEl\) return/);
    assert.doesNotMatch(monitor, /!commentInput \|\| !statusEl/);
    assert.match(monitor, /JSON\.stringify\(editedArgs\) === JSON\.stringify\(originalArgs\)/);
    assert.match(monitor, /editedArgs = null/);
});

test('Back-to-latest button in long history chat does not let scroll clicks pass through to approval actions', () => {
    assert.match(chatScroll, /function isolateReturnLatestPointerEvent\(event\)/);
    assert.match(chatScroll, /returnLatestButton\.addEventListener\('pointerdown', isolateReturnLatestPointerEvent\)/);
    assert.match(chatScroll, /function onReturnLatestClick\(event\)[\s\S]{0,260}event\.preventDefault\(\)[\s\S]{0,180}event\.stopPropagation\(\)/);
    assert.match(monitor, /const bindExplicitHitlAction = function \(button, decision\)/);
    assert.match(monitor, /button\.addEventListener\('pointerdown'[\s\S]{0,900}pointerClick && !explicitlyPressed/);
    assert.match(monitor, /bindExplicitHitlAction\(approveBtn, 'approve'\)/);
    assert.match(monitor, /bindExplicitHitlAction\(rejectBtn, 'reject'\)/);
});

test('Turn navigation uses continuous large hit area and allows smooth mouse entry into Codex-style preview card', () => {
    const styles = fs.readFileSync('web/static/css/style.css', 'utf8');
    assert.match(styles, /\.chat-turn-rail-markers \{[\s\S]{0,260}gap: 0;/);
    assert.match(styles, /\.chat-turn-rail-markers \{[\s\S]{0,420}overflow-x: hidden;/);
    assert.match(styles, /\.chat-turn-rail-markers \{[\s\S]{0,520}touch-action: pan-y;/);
    assert.match(styles, /\.chat-turn-rail-marker \{[\s\S]{0,260}width: 36px;[\s\S]{0,160}height: 11px;/);
    assert.match(styles, /\.chat-turn-rail-marker::before \{[\s\S]{0,420}width: 12px;[\s\S]{0,120}height: 3px;/);
    assert.match(styles, /\.chat-turn-rail-marker:hover::before \{[\s\S]{0,100}width: 22px;/);
    assert.match(styles, /\.chat-turn-rail-preview \{[\s\S]{0,520}pointer-events: auto;/);
    assert.match(chatScroll, /function scheduleHideTurnPreview\(\)/);
    assert.match(chatScroll, /window\.setTimeout\(hideTurnPreview, 160\)/);
    assert.match(chatScroll, /turnPreview\.addEventListener\('mouseenter'/);
    assert.match(chatScroll, /marker\.addEventListener\('mouseleave', scheduleHideTurnPreview\)/);
});

test('Countdown is server-time driven; on expiry only locks the UI and waits for server reject', () => {
    assert.match(handler, /payload\["hitlApproval"\]/);
    assert.match(handler, /"expiresAt":\s+approvalExpiresAt/);
    assert.match(handler, /status = "timeout"/);
    assert.match(handler, /decidedBy = "system"/);
    assert.match(monitor, /function bindHitlApprovalCountdown/);
    assert.match(monitor, /setInterval\(update, 250\)/);
    assert.match(monitor, /expiredAutoRejected/);
    assert.doesNotMatch(monitor, /remaining <= 0[\s\S]{0,240}submitHitlDecisionWithPayload/);
});

test('Project chat list can simultaneously display waiting-for-approval and running states', () => {
    assert.match(projects, /pendingApprovalByConversation: new Map/);
    assert.match(projects, /statusKinds\.push\('approval'\)/);
    assert.match(projects, /statusKinds\.push\('running'\)/);
    assert.match(projects, /window\.setProjectConversationApprovalStatus/);
    assert.match(projects, /api\/hitl\/pending\?page=1&pageSize=200/);
    assert.match(projects, /function bindProjectApprovalProgress/);
    assert.match(projects, /project-approval-progress-value/);
    assert.match(projects, /PROJECT_APPROVAL_TICK_INTERVAL_MS = 1000/);
    assert.match(projects, /function registerProjectApprovalTicker/);
    assert.match(monitor, /function renderDirectHitlSidebarApproval/);
    assert.match(monitor, /hitlSidebarApprovalSyncTimer = window\.setInterval/);
});

test('Project folder summary is always green; only individual chats change colour by remaining time', () => {
    assert.match(projects, /waitingApprovalCount/);
    assert.match(projects, /aggregate: true, count: folderApprovals\.length/);
    assert.match(projects, /project-task-status--approval-summary', 'is-urgency-normal'/);
    assert.match(projects, /status\.dataset\.approvalUrgency = 'normal'/);
    assert.match(projects, /if \(isApprovalSummary\)[\s\S]{0,520}else \{[\s\S]{0,160}bindProjectApprovalUrgency\(status, details, label\)/);
    assert.doesNotMatch(projects, /currentExpiry < earliestExpiry/);
    assert.match(projects, /PROJECT_APPROVAL_URGENCY_CLASSES/);
    assert.match(projects, /remaining <= 60 \* 1000/);
    assert.match(projects, /remaining <= 3 \* 60 \* 1000/);
    assert.doesNotMatch(projects, /remaining <= 5 \* 60 \* 1000/);
    assert.match(projects, /project-task-status--approval-summary/);
    assert.ok(zh.hitl.waitingApprovalCount === '等待批准 {{count}}' || zh.hitl.waitingApprovalCount === 'Waiting approval {{count}}');
    assert.ok(zh.hitl.approvalUrgencyMoreThanThree === '最早审批将在 3 分钟后到期' || zh.hitl.approvalUrgencyMoreThanThree === 'Earliest approval expires in more than 3 minutes');
    assert.equal(typeof en.hitl.waitingApprovalCount, 'string');
    const urgencyFunctionSource = projects.match(
        /function projectApprovalUrgencyLevel\(remainingMilliseconds, hasDeadline\) \{[\s\S]*?\n\}/
    );
    assert.ok(urgencyFunctionSource, 'Should provide a testable approval urgency function');
    const urgencyLevel = vm.runInNewContext(`(${urgencyFunctionSource[0]})`);
    assert.equal(urgencyLevel(6 * 60 * 1000, true), 'normal');
    assert.equal(urgencyLevel(4 * 60 * 1000, true), 'normal');
    assert.equal(urgencyLevel(3 * 60 * 1000 + 1, true), 'normal');
    assert.equal(urgencyLevel(3 * 60 * 1000, true), 'warning');
    assert.equal(urgencyLevel(2 * 60 * 1000, true), 'warning');
    assert.equal(urgencyLevel(30 * 1000, true), 'critical');
    assert.equal(urgencyLevel(0, false), 'normal');
});

test('Tool details delayed payload uses processDetailId from real-time event to back-fill params', () => {
    assert.match(chat, /const processDetailId = detail\.id \|\| data\.processDetailId \|\| ''/);
    assert.match(chat, /processDetailId: processDetailId/);
    assert.match(monitor, /resultDetailId: data\._mergedResultDetailId \|\| \(merged && merged\.processDetailId\) \|\| ''/);
    assert.match(monitor, /if \(state\.payloadDeferred && !state\.payloadLoaded && \(state\.processDetailId \|\| state\.resultDetailId\)\)/);
    assert.match(monitor, /const fullCall = await fetchFullProcessDetailData\(state\.processDetailId\)/);
    assert.match(monitor, /state\.args = parseToolCallArgsFromData\(fullCall\)/);
});

test('After switching chat, main button only reads running state of currently visible chat', () => {
    assert.match(chat, /function getVisibleChatConversationId\(\)/);
    assert.match(chat, /function shouldTreatLiveChatTaskAsCurrent\(/);
    assert.match(chat, /function isLiveChatTaskVisible\(/);
    assert.match(chat, /if \(visibleConversationId\) return visibleConversationId/);
    assert.match(chat, /isConversationTaskRunning\(visibleConversationId\)/);
    assert.doesNotMatch(
        chat,
        /function getCurrentChatTaskConversationId\(\) \{[\s\S]{0,220}if \(live && live\.active && live\.conversationId\) \{[\s\S]{0,100}return String\(live\.conversationId\)/
    );
    const visibilityFunctionSource = chat.match(
        /function shouldTreatLiveChatTaskAsCurrent\(liveConversationId, visibleConversationId, hasVisibleProgress\) \{[\s\S]*?\n\}/
    );
    assert.ok(visibilityFunctionSource, 'Should provide a testable current task isolation function');
    const isCurrent = vm.runInNewContext(`(${visibilityFunctionSource[0]})`);
    assert.equal(isCurrent('running-conversation', '', true), false);
    assert.equal(isCurrent('running-conversation', 'new-conversation', true), false);
    assert.equal(isCurrent('running-conversation', 'running-conversation', false), true);
    assert.equal(isCurrent('', '', true), true);
    assert.equal(isCurrent('', '', false), false);
});

test('No-project uses a standalone virtual folder and top new-task inherits current project', () => {
    assert.match(projects, /CHAT_UNASSIGNED_PROJECT_FOLDER_ID/);
    assert.match(projects, /_isUnassigned: true/);
    assert.match(projects, /\[\.\.\.pinnedProjects, unassignedProject, \.\.\.regularProjects\]/);
    assert.match(projects, /window\.startNewConversation\(\{ projectId: isUnassigned \? '' : project\.id \}\)/);
    assert.match(chat, /Object\.prototype\.hasOwnProperty\.call\(options, 'projectId'\)/);
    assert.match(chat, /typeof resolveChatProjectSelection === 'function'/);
    assert.match(chat, /String\(inheritedProjectId \|\| ''\)\.trim\(\)/);
    assert.match(chat, /typeof setActiveProjectId === 'function'\) setActiveProjectId\(requestedProjectId\)/);
    assert.ok(zh.chat.newUnassignedConversation === '新建No projectChat' || zh.chat.newUnassignedConversation === '新建无项目对话' || zh.chat.newUnassignedConversation === 'Start a new conversation without a project');
    assert.equal(typeof en.chat.newUnassignedConversation, 'string');
});

test('Single chat approval badge switches urgency colour in sync with the countdown', () => {
    assert.match(projects, /bindProjectApprovalProgress\(status, details\);\s*bindProjectApprovalUrgency\(status, details, label\);/);
    assert.match(fs.readFileSync('web/static/css/style.css', 'utf8'), /\.project-task-status--approval\.is-urgency-critical/);
});

test('Project status refresh reuses a single timer; switching chats does not repeat full project context requests', () => {
    assert.match(projects, /const projectApprovalTickerEntries = new Set\(\)/);
    assert.match(projects, /if \(!changed && !approvalChanged\) return/);
    assert.match(projects, /options\.reloadFolders !== false/);
    assert.match(chat, /refreshChatProjectSelector\(\{ reloadFolders: false, renderFolders: false \}\)/);
    assert.match(projects, /function selectChatProjectConversationItem/);
    assert.match(projects, /options\.renderFolders !== false/);
    assert.match(projects, /projectConversationPreviewSuppressedUntil = Date\.now\(\) \+ 700/);
    assert.match(projects, /project-task-status-group--folder/);
    assert.doesNotMatch(fs.readFileSync('web/static/css/style.css', 'utf8'), /project-task-status-group--folder \.project-task-status--running/);
    assert.match(fs.readFileSync('web/static/css/style.css', 'utf8'), /\.active-tasks-bar \{[\s\S]*?padding: 13px 24px 14px;/);
});

test('Running chat switch cancels the old event stream and only resumes the latest page of process details', () => {
    assert.match(chat, /window\.cancelRunningTaskEventStream\(conversationId\)/);
    assert.match(monitor, /function cancelRunningTaskEventStream/);
    assert.match(monitor, /abortController\.abort\(\)/);
    assert.match(monitor, /signal: abortController\.signal/);
    assert.match(monitor, /initialLatest: true/);
    assert.match(monitor, /autoLoadAll: false/);
});

test('With multiple concurrent chats, release hidden main stream and stale requests cannot overwrite new chat state', () => {
    assert.match(chat, /function ownsLiveChatStream\(liveStream\)/);
    assert.match(chat, /function clearLiveChatStreamIfOwned\(liveStream\)/);
    assert.match(chat, /function detachLiveChatStreamForNavigation\(nextConversationId, force = false\)/);
    assert.match(chat, /liveStream\.detached = true;[\s\S]{0,240}controller\.abort\(\)/);
    assert.match(chat, /const requestAbortController = new AbortController\(\)/);
    assert.match(chat, /signal: requestAbortController\.signal/);
    assert.match(chat, /shouldIgnoreLiveChatStreamEvent\(liveStreamState\)/);
    assert.match(chat, /const clearedOwnedStream = clearLiveChatStreamIfOwned\(liveStreamState\)/);
    assert.match(chat, /detachLiveChatStreamForNavigation\(conversationId\)/);
    assert.match(chat, /detachLiveChatStreamForNavigation\('', true\)/);
    assert.match(chat, /window\.clearChatHitlApprovalDock\(\)/);
    assert.match(monitor, /if \(conversationId && conversationId !== currentId\) return false/);
    assert.match(monitor, /function scrollProcessDetailsToLatest\(assistantMessageId, smooth = true\)/);
    assert.match(monitor, /timeline\.scrollTop = targetTop/);
    assert.match(chat, /let loadConversationAbortController = null/);
    assert.match(chat, /cancelPendingConversationLoad\(\);[\s\S]{0,900}const conversationLoadController = new AbortController\(\)/);
    assert.match(chat, /signal: conversationLoadController\.signal/);
    assert.match(template, /monitor\.js\?v=20260907-blocked-1/);
    assert.match(template, /chat-scroll\.js\?v=20260815-1/);
    assert.match(template, /chat\.js\?v=20260907-blocked-1/);
    assert.match(template, /style\.css\?v=20260907-blocked-1/);
});

test('Complete stop always uses the dialog-locked session and still cancels after status refresh', () => {
    const start = monitor.indexOf("async function performHardCancelProgressTask(progressId, conversationId = '')");
    const end = monitor.indexOf('function progressElapsedText(', start);
    assert.notEqual(start, -1);
    assert.notEqual(end, -1);
    const hardCancelSource = monitor.slice(start, end);
    assert.match(monitor, /performHardCancelProgressTask\(progressId, conversationId\)/);
    assert.match(hardCancelSource, /const targetConversationId = String\(conversationId \|\| \(state && state\.conversationId\) \|\| ''\)\.trim\(\)/);
    assert.match(hardCancelSource, /await requestCancel\(targetConversationId\)/);
    assert.doesNotMatch(hardCancelSource, /if \(!state \|\| !state\.conversationId\)/);
});

test('Input area Agent review text preserves sufficient line height and does not clip glyphs', () => {
    assert.match(styles, /\.chat-hitl-shortcut > span\s*\{[\s\S]*?display: block/);
    assert.match(styles, /\.chat-hitl-shortcut > span\s*\{[\s\S]*?padding-block: 1px/);
    assert.match(styles, /\.chat-hitl-shortcut > span\s*\{[\s\S]*?line-height: 1\.4/);
});

test('After task ends, approval buttons inside the chat are greyed out and disabled', () => {
    assert.match(monitor, /ready: false/);
    assert.match(monitor, /function setHitlApprovalTaskAvailability/);
    assert.match(monitor, /conversationExecutionTracker\.ready && !conversationExecutionTracker\.isRunning\(id\)/);
    assert.match(monitor, /hitlPendingInterruptTracker\.ready/);
    assert.match(monitor, /!hitlPendingInterruptTracker\.has\(interruptId\)/);
    assert.match(monitor, /button\.disabled = true/);
    assert.match(monitor, /function setHitlApprovalInterruptedVisualState/);
    assert.match(monitor, /stopHitlApprovalCountdown\(panel\)/);
    assert.match(monitor, /removeAttribute\('data-hitl-expires-at'\)/);
    assert.match(monitor, /hitl\.interruptedApprovalCancelled/);
    assert.match(monitor, /reconcileHitlApprovalStateWithActiveTasks\(normalizedTasks\)/);
    assert.match(monitor, /syncHitlApprovalTaskAvailability\(\)/);
    assert.match(fs.readFileSync('web/static/css/style.css', 'utf8'), /hitl-approval-task-closed/);
    assert.ok(zh.hitl.taskClosedApprovalUnavailable === '任务已结束，审批不可用' || zh.hitl.taskClosedApprovalUnavailable === 'Task ended; approval is unavailable');
    assert.ok(zh.hitl.interruptedApprovalCancelled === '任务已中断，审批Cancelled' || zh.hitl.interruptedApprovalCancelled === '任务已中断，审批已取消' || zh.hitl.interruptedApprovalCancelled === 'Task interrupted; approval cancelled');
    assert.equal(typeof en.hitl.taskClosedApprovalUnavailable, 'string');
    assert.equal(typeof en.hitl.interruptedApprovalCancelled, 'string');
});

test('Project tree only retains approval state for tasks still running in the current process', () => {
    assert.match(projects, /chatProjectFolderContext\.runningIds\.has\(conversationId\)/);
    assert.match(projects, /pendingApprovalByConversation\.delete\(conversationId\)/);
    assert.match(monitor, /conversationExecutionTracker\.ready && !conversationExecutionTracker\.isRunning\(conversationId\)/);
});

test('Approval status actively polls and immediately closes stale approvals when service is unavailable', () => {
    assert.match(monitor, /ACTIVE_TASK_REFRESH_INTERVAL = 2000/);
    assert.match(monitor, /apiFetch\('\/api\/hitl\/pending\?page=1&pageSize=200'\)/);
    assert.match(monitor, /function reconcilePendingHitlState\(rawItems\)/);
    assert.match(monitor, /renderChatHitlApprovalDock\(currentPending\)/);
    assert.match(monitor, /restoreHitlInlineForConversation\(currentId\)/);
    assert.match(monitor, /case 'conversation':[\s\S]{0,2400}window\.refreshChatProjectFolders\(\)/);
    assert.match(monitor, /renderActiveTasks\(\[\]\);[\s\S]{0,260}hitlPendingInterruptTracker\.update\(\[\]\)/);
    assert.match(projects, /function syncProjectConversationApprovalStatuses\(items\)/);
    assert.match(projects, /window\.syncProjectConversationApprovalStatuses/);
    assert.match(template, /projects\.js\?v=20260819-1/);
});

test('Old sessions first upgraded to 5-minute default approval timeout; user can still actively choose unlimited afterwards', () => {
    assert.match(fs.readFileSync('web/static/js/hitl.js', 'utf8'), /HITL_TIMEOUT_DEFAULT_MIGRATION_PREFIX/);
    assert.match(fs.readFileSync('web/static/js/hitl.js', 'utf8'), /shouldMigrateLegacyHitlTimeout/);
    assert.match(fs.readFileSync('web/static/js/hitl.js', 'utf8'), /timeoutSeconds: 300/);
    assert.match(fs.readFileSync('web/static/js/hitl.js', 'utf8'), /markLegacyHitlTimeoutMigrated/);
});

test('Human-machine collaboration page and logs display Jev / OpenAI approval engine', () => {
    const hitlPage = fs.readFileSync('web/static/js/hitl.js', 'utf8');
    assert.match(template, /id="hitl-page-audit-engine"/);
    assert.match(template, /id="hitl-log-detail-engine"/);
    assert.match(hitlPage, /function hitlAuditEngineFromItem/);
    assert.match(hitlPage, /function renderHitlPageAuditEngine/);
    assert.match(hitlPage, /function renderHitlStrategyJevHint/);
    assert.match(template, /id="hitl-strategy-hint-jev"/);
    assert.equal(zh.hitl.strategyHintJev.includes('Jev'), true);
    assert.equal(en.hitl.strategyHintJev.includes('Jev'), true);
    assert.match(handler, /auditBackend/);
    assert.match(chat, /function currentHitlAuditEngineLabel\(\)/);
    assert.equal(zh.hitl.auditEngineJev, 'TypeSafe Jev');
    assert.equal(en.hitl.auditEngineJev, 'TypeSafe Jev');
    assert.ok(zh.hitl.auditEngineOpenAI === 'OpenAI 协议' || zh.hitl.auditEngineOpenAI === 'OpenAI protocol');
    assert.equal(en.hitl.auditEngineOpenAI, 'OpenAI protocol');
});

test('Approval UX copy has complete Chinese and English resources', () => {
    const hitlKeys = [
        'waitingApprovalShort',
        'requestVisitUrl',
        'requestCommand',
        'viewRequestDetails',
        'timeoutAutoReject',
        'expiredRejected',
    ];
    const chatKeys = [
        'hitlTimeoutLabel',
        'hitlTimeoutFiveMinutes',
        'hitlTimeoutUnlimited',
        'hitlTimeoutHint',
    ];
    hitlKeys.forEach((key) => {
        assert.equal(typeof zh.hitl[key], 'string');
        assert.equal(typeof en.hitl[key], 'string');
    });
    chatKeys.forEach((key) => {
        assert.equal(typeof zh.chat[key], 'string');
        assert.equal(typeof en.chat[key], 'string');
    });
});
