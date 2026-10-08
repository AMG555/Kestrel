const fs = require('node:fs');
const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');

const scroll = fs.readFileSync('web/static/js/chat-scroll.js', 'utf8');
const monitor = fs.readFileSync('web/static/js/monitor.js', 'utf8');
const chat = fs.readFileSync('web/static/js/chat.js', 'utf8');
const projects = fs.readFileSync('web/static/js/projects.js', 'utf8');
const router = fs.readFileSync('web/static/js/router.js', 'utf8');
const auth = fs.readFileSync('web/static/js/auth.js', 'utf8');
const webshell = fs.readFileSync('web/static/js/webshell.js', 'utf8');
const html = fs.readFileSync('web/templates/index.html', 'utf8');

function functionSource(source, name, nextName) {
    const start = source.indexOf(`function ${name}(`);
    const end = source.indexOf(`function ${nextName}(`, start);
    assert.notEqual(start, -1, `${name} should exist`);
    assert.notEqual(end, -1, `${nextName} should follow ${name}`);
    return source.slice(start, end);
}

function createScrollRuntime() {
    const listeners = new Map();
    const buttonListeners = new Map();
    const classList = { add() {}, remove() {}, toggle() {}, contains() { return false; } };
    const chatEl = {
        scrollTop: 500,
        scrollHeight: 1000,
        clientHeight: 500,
        children: [],
        classList,
        addEventListener(type, handler) { listeners.set(type, handler); },
        scrollTo(options) { this.scrollTop = Number(options && options.top) || 0; },
        getBoundingClientRect() { return { right: 1000 }; },
    };
    const returnLatest = {
        hidden: true,
        classList,
        addEventListener(type, handler) { buttonListeners.set(type, handler); },
        blur() {},
    };
    const rafQueue = new Map();
    let rafId = 0;
    const requestAnimationFrame = (handler) => {
        const id = ++rafId;
        rafQueue.set(id, handler);
        return id;
    };
    const cancelAnimationFrame = (id) => rafQueue.delete(id);
    const document = {
        readyState: 'complete',
        getElementById(id) {
            if (id === 'chat-messages') return chatEl;
            if (id === 'chat-return-latest') return returnLatest;
            return null;
        },
        querySelectorAll() { return []; },
        addEventListener() {},
    };
    const window = {
        document,
        addEventListener() {},
        setTimeout,
        clearTimeout,
        requestAnimationFrame,
        cancelAnimationFrame,
        innerWidth: 1440,
        innerHeight: 900,
    };
    const context = {
        window,
        document,
        requestAnimationFrame,
        cancelAnimationFrame,
        setTimeout,
        clearTimeout,
        console,
    };
    vm.runInNewContext(scroll, context);
    return {
        api: window.KestrelChatScroll,
        chatEl,
        returnLatest,
        listeners,
        flushAnimationFrames() {
            while (rafQueue.size) {
                const pending = Array.from(rafQueue.values());
                rafQueue.clear();
                pending.forEach((handler) => handler(Date.now()));
            }
        },
    };
}

test('Scroll up immediately unpins from bottom; only resume following when scrolled to true bottom', () => {
    const runtime = createScrollRuntime();
    runtime.flushAnimationFrames();

    runtime.listeners.get('wheel')({ deltaY: -20 });
    runtime.chatEl.scrollTop = 480;
    runtime.listeners.get('scroll')();
    assert.equal(runtime.api.captureScrollPinState(), false);

    runtime.chatEl.scrollHeight = 1100;
    runtime.api.scrollIfPinned(true);
    runtime.flushAnimationFrames();
    assert.equal(runtime.chatEl.scrollTop, 480, 'New output must not steal back the user reading position');

    runtime.chatEl.scrollTop = 597;
    runtime.listeners.get('scroll')();
    assert.equal(runtime.api.captureScrollPinState(), false, 'More than 2px from bottom: still unpinned');

    runtime.chatEl.scrollTop = 600;
    runtime.listeners.get('scroll')();
    assert.equal(runtime.api.captureScrollPinState(), true, 'User scrolled to true bottom: resume following immediately');

    runtime.chatEl.scrollHeight = 1200;
    runtime.api.scrollIfPinned(true);
    runtime.flushAnimationFrames();
    assert.equal(runtime.chatEl.scrollTop, 1200, 'After resume, addOutput continues requesting scroll to bottom');
});

test('After user leaves latest position, back-to-bottom button stays visible until true bottom', () => {
    const runtime = createScrollRuntime();
    runtime.flushAnimationFrames();

    runtime.listeners.get('wheel')({ deltaY: -20 });
    runtime.chatEl.scrollTop = 455;
    runtime.listeners.get('scroll')();
    assert.equal(runtime.returnLatest.hidden, false, 'Within 120px threshold: back-to-latest entry should still be shown');

    runtime.listeners.get('wheel')({ deltaY: 20 });
    runtime.chatEl.scrollTop = 499;
    runtime.listeners.get('scroll')();
    assert.equal(runtime.returnLatest.hidden, true, 'Only hidden after scrolling to the true bottom');
});

test('Refresh rebuilding details causing layout shift upward is not misidentified as user scroll-up', () => {
    const runtime = createScrollRuntime();
    runtime.flushAnimationFrames();

    runtime.chatEl.scrollTop = 460;
    runtime.listeners.get('scroll')();
    assert.equal(runtime.api.captureScrollPinState(), true, 'Layout scroll without user input should still maintain following');

    runtime.chatEl.scrollHeight = 1100;
    runtime.api.scrollIfPinned(true);
    runtime.flushAnimationFrames();
    assert.equal(runtime.chatEl.scrollTop, 1100, 'Subsequent increments after RefreshResume should continue pinning to bottom');
});

test('Reload project sidebar that previously failed due to unauthorised access after sign-in', () => {
    const refreshSource = functionSource(auth, 'refreshAppData', 'bootstrapApp');
    const conversationsIndex = refreshSource.indexOf('loadConversations()');
    const projectRetryIndex = refreshSource.indexOf('window.refreshChatProjectSelector({ reloadFolders: true })');

    assert.notEqual(conversationsIndex, -1);
    assert.ok(projectRetryIndex > conversationsIndex);
    assert.match(refreshSource, /typeof window\.refreshChatProjectSelector === 'function'/);
    assert.match(html, /\/static\/js\/auth\.js\?v=20260907-blocked-1/);
});

test('After user truly scrolls to bottom, resume auto-follow without forcing a premature jump', () => {
    const resumeSource = functionSource(scroll, 'resumeFollowingIfAtBottom', 'captureScrollPinState');
    const captureSource = functionSource(scroll, 'captureScrollPinState', 'setScrollFollowing');
    const autoSource = functionSource(scroll, 'canAutoScrollNow', 'scheduleChatScrollToBottomIfFollowing');
    const scrollSource = functionSource(scroll, 'onChatMessagesScroll', 'bindChatScrollListeners');

    assert.match(resumeSource, /thresholdPx/);
    assert.match(scroll, /CHAT_SCROLL_FOLLOW_RESUME_THRESHOLD_PX = 2/);
    assert.doesNotMatch(captureSource, /resumeFollowingIfAtBottom/);
    assert.doesNotMatch(autoSource, /resumeFollowingIfAtBottom/);
    assert.match(resumeSource, /if \(!userInitiated\) return false/);
    assert.match(scrollSource, /scrolledDown/);
    assert.match(scrollSource, /hasUserScrollIntent/);
    assert.match(scrollSource, /resumeFollowingIfAtBottom\(CHAT_SCROLL_FOLLOW_RESUME_THRESHOLD_PX, true\)/);
    assert.doesNotMatch(scrollSource, /resumeFollowingIfAtBottom\(CHAT_SCROLL_NAV_BOTTOM_THRESHOLD_PX\)/);
    assert.doesNotMatch(scrollSource, /else if \(resumeFollowingIfAtBottom\(\)\)/);
    assert.doesNotMatch(scrollSource, /scheduleChatScrollToBottomIfFollowing\(true\)/);
    assert.doesNotMatch(scrollSource, /else if \(resumeFollowingIfAtBottom\(\)\)/);
    assert.match(scrollSource, /contentShrank/);
    assert.match(scrollSource, /sh < lastScrollHeight - 1/);
    assert.match(scrollSource, /if \(scrolledUp && \(scrollMode === 'detached' \|\| hasUserScrollIntent\)\) \{[\s\S]*?setScrollDetached\(\)/);
    assert.match(scrollSource, /if \(programmaticScroll\) \{[\s\S]*?st < lastScrollTop - 1 && \(scrollMode === 'detached' \|\| hasUserScrollIntent\)[\s\S]*?setScrollDetached\(\)/);
});

test('Layout scroll caused by switching chat mode does not re-enable pin-to-bottom', () => {
    const scrollSource = functionSource(scroll, 'onChatMessagesScroll', 'bindChatScrollListeners');
    const bindSource = functionSource(scroll, 'bindChatScrollListeners', 'initChatScroll');
    const selectModeSource = functionSource(chat, 'selectAgentMode', 'initChatAgentModeFromConfig');

    assert.match(scroll, /let userScrollIntentUntil = 0/);
    assert.match(scrollSource, /const hasUserScrollIntent = Date\.now\(\) <= userScrollIntentUntil/);
    assert.match(scrollSource, /scrolledDown &&[\s\S]*?hasUserScrollIntent &&[\s\S]*?resumeFollowingIfAtBottom/);
    assert.doesNotMatch(scrollSource, /else if \(resumeFollowingIfAtBottom\(\)\)/);
    assert.match(bindSource, /Math\.abs\(e\.deltaY\) > 1/);
    assert.match(bindSource, /userScrollIntentUntil = Date\.now\(\) \+ 1800/);
    assert.doesNotMatch(selectModeSource, /setScrollFollowing|forceScrollToBottom|scrollTop/);
});

test('RefreshRunning: after catching up latest details, keep pin but respect user scroll-up', () => {
    const attachSource = functionSource(monitor, 'attachRunningTaskEventStream', 'parseToolCallArgsFromData');
    const settleSource = functionSource(scroll, 'settleChatToBottomIfFollowing', 'scrollChatMessagesToBottomIfPinned');

    assert.match(attachSource, /window\.captureScrollPinState\(\)/);
    assert.match(attachSource, /settleToBottomIfFollowing\(12\)/);
    assert.match(attachSource, /settleToBottomIfFollowing\(18\)/);
    assert.match(attachSource, /(?:用户期间没有主动上滑|no user scroll-up during)/);
    assert.match(attachSource, /keepFollowingFinalRender/);
    assert.match(attachSource, /(?:最终消息和Details重绘都会增高 DOM|final message and details redraw increase DOM)/);
    assert.match(settleSource, /scrollMode !== 'following'/);
    assert.match(settleSource, /Date\.now\(\) < detachLockUntil/);
    assert.match(settleSource, /settleFrame\(remaining - 1\)/);
    assert.match(settleSource, /scrollChatToBottomInstant\(\)/);
    assert.match(scroll, /function settleConversationRestoreToBottom\(frameCount\)/);
    assert.match(scroll, /CONVERSATION_RESTORE_SETTLE_MIN_MS = 3000/);
    assert.match(scroll, /CONVERSATION_RESTORE_SETTLE_MAX_MS = 6000/);
    assert.match(scroll, /const generation = \+\+conversationRestoreGeneration/);
    assert.match(scroll, /scrollMode !== 'following'/);
    assert.match(scroll, /stableFrames >= CONVERSATION_RESTORE_STABLE_FRAMES/);
    assert.match(scroll, /requestAnimationFrame\(settleRestoreFrame\)/);
    assert.match(chat, /settleConversationRestoreToBottom\(30\)/);
});

test('After refresh: iteration thinking section independently follows latest content and allows user to scroll up to unpin', () => {
    const startSource = functionSource(monitor, 'startProcessDetailsLatestFollow', 'loadProcessDetailsPaginated');
    const loadSource = functionSource(monitor, 'loadProcessDetailsPaginated', 'shouldInitiallyOpenProcessDetailsAtLatest');
    const attachSource = functionSource(monitor, 'attachRunningTaskEventStream', 'parseToolCallArgsFromData');
    const returnLatestSource = functionSource(monitor, 'ensureProcessDetailsReturnLatestControl', 'scrollProcessDetailsToLatest');

    assert.match(monitor, /const processDetailsReturnLatestControls = new WeakMap\(\)/);
    assert.match(returnLatestSource, /className = 'process-details-return-latest'/);
    assert.match(monitor, /function getProcessDetailsLatestFollowStateForTimeline\(timeline\)/);
    assert.match(monitor, /const followingLatest = !!\(followState && !followState\.detached\)/);
    assert.match(monitor, /const shouldShow = !followingLatest && expanded && scrollable && awayFromLatest/);
    assert.match(returnLatestSource, /timeline\.scrollTo\(\{ top: targetTop, behavior: 'smooth' \}\)/);
    assert.match(returnLatestSource, /timeline\.addEventListener\('scroll', onScroll/);
    assert.match(returnLatestSource, /window\.ensureProcessDetailsReturnLatestControl = ensureProcessDetailsReturnLatestControl/);
    assert.match(startSource, /new MutationObserver\(scheduleFollowLatest\)/);
    assert.match(startSource, /characterData: true/);
    assert.match(startSource, /new ResizeObserver\(scheduleFollowLatest\)/);
    assert.match(startSource, /ensureProcessDetailsReturnLatestControl\(timeline\)/);
    assert.match(startSource, /markProcessDetailsReturnLatestPending\(timeline\)/);
    assert.match(startSource, /scrollProcessDetailsToLatest\(String\(assistantMessageId \|\| ''\), false\)/);
    assert.match(startSource, /event\.deltaY < -1/);
    assert.match(startSource, /state\.userScrollIntentUntil = Date\.now\(\) \+ 1200/);
    assert.match(startSource, /event\.clientX >= rect\.right - PROCESS_DETAILS_FOLLOW_SCROLLBAR_GUTTER_PX/);
    assert.match(startSource, /event\.key === 'ArrowUp'/);
    assert.match(startSource, /cancelAnimationFrame\(state\.rafId\)/);
    assert.match(startSource, /if \(scrolledUp && \(state\.detached \|\| Date\.now\(\) <= state\.userScrollIntentUntil\)\) \{[\s\S]*?detachForUserNavigation\(\)/);
    assert.match(startSource, /state\.detached &&[\s\S]*?scrolledDown &&[\s\S]*?Date\.now\(\) <= state\.userScrollIntentUntil/);
    assert.match(monitor, /PROCESS_DETAILS_FOLLOW_RESUME_THRESHOLD_PX = 2/);
    assert.match(startSource, /distance <= PROCESS_DETAILS_FOLLOW_RESUME_THRESHOLD_PX/);
    assert.match(startSource, /state\.detached = false/);
    assert.doesNotMatch(startSource, /if \(distance <= PROCESS_DETAILS_FOLLOW_RESUME_THRESHOLD_PX\) \{\s*state\.detached = false/);
    assert.match(loadSource, /startProcessDetailsLatestFollow\(assistantMessageId/);
    assert.match(attachSource, /startProcessDetailsLatestFollow\(asEl\.id, \{ persistent: true \}\)/);
    assert.match(attachSource, /stopProcessDetailsLatestFollow\(asEl\.id\)/);
    assert.match(chat, /window\.ensureProcessDetailsReturnLatestControl\(timeline\)/);
    assert.doesNotMatch(returnLatestSource, /chat-return-latest/);
});

test('After refresh: tool call resume badges match real-time success/failure badges', () => {
    const renderSource = functionSource(chat, 'renderProcessDetails', 'finishProcessDetailsRender');
    const presentationSource = functionSource(monitor, 'getToolCallStatusPresentation', 'applyToolCallStatus');
    const applySource = functionSource(monitor, 'applyToolCallStatus', 'updateToolCallStatus');
    const addSource = functionSource(monitor, 'addTimelineItem', 'loadActiveTasks');

    assert.match(renderSource, /toolStatusByProcessDetailId/);
    assert.match(renderSource, /timelineOpts\.toolStatus = toolStatusByProcessDetailId\.get/);
    assert.match(presentationSource, /normalized === 'completed'/);
    assert.match(presentationSource, /normalized === 'failed'/);
    assert.match(applySource, /tool-status-badge/);
    assert.match(applySource, /item\.dataset\.toolDisplayStatus = presentation\.status/);
    assert.match(addSource, /initialToolStatus = item\.dataset\.toolDisplayStatus/);
    assert.match(addSource, /applyToolCallStatus\(item, initialToolStatus\)/);
    assert.match(monitor, /refreshProgressAndTimelineI18n\(\)[\s\S]*?applyToolCallStatus\(item, item\.dataset\.toolDisplayStatus\)/);
});

test('First real-time output and RefreshResume both preserve independent iteration scrolling and follow latest content', () => {
    const css = fs.readFileSync('web/static/css/style.css', 'utf8');
    const addSource = functionSource(monitor, 'addProgressMessage', 'toggleProgressDetails');
    const liveSource = functionSource(monitor, 'startLiveProgressLatestFollow', 'stopLiveProgressLatestFollow');

    assert.match(css, /\.progress-container\.is-streaming \.progress-timeline\.expanded,[\s\S]{0,360}max-height: min\(64vh, 720px\);[\s\S]{0,180}overflow-y: auto;/);
    assert.match(css, /\.message\.progress-message \.progress-timeline\.expanded \{[\s\S]{0,260}max-height: min\(64vh, 720px\);[\s\S]{0,160}overflow-y: auto;/);
    assert.match(css, /\.process-details-return-latest \{[\s\S]{0,260}position: absolute;[\s\S]{0,260}border-radius: 50%;/);
    assert.match(css, /\.process-details-return-latest\.has-pending-new::after,/);
    assert.doesNotMatch(css, /(?:流式Executing|streaming-executing)[\s\S]{0,320}overflow-y: visible;/);
    assert.match(addSource, /startLiveProgressLatestFollow\(id\)/);
    assert.match(liveSource, /stateKey: liveProgressLatestFollowKey\(id\)/);
    assert.match(liveSource, /persistent: true/);
    assert.match(liveSource, /target\.scrollTop = Math\.max\(0, target\.scrollHeight - target\.clientHeight\)/);
    assert.match(monitor, /function finalizeProgressTask\(progressId, finalLabel\) \{[\s\S]{0,120}stopLiveProgressLatestFollow\(progressId\)/);
});

test('Other tabs in the same session auto-resume stream and block duplicate tasks before sending', () => {
    const syncSource = functionSource(monitor, 'syncVisibleConversationTaskReplay', 'getActiveTaskDisplayName');
    const sendSource = functionSource(chat, 'sendMessage', 'renderChatFileChips');

    assert.match(monitor, /new BroadcastChannel\(CHAT_TASK_SYNC_CHANNEL_NAME\)/);
    assert.match(monitor, /payload\.type !== 'task-started'/);
    assert.match(monitor, /conversationExecutionTracker\.markRunning\(id\)/);
    assert.match(syncSource, /await window\.loadConversation\(conversationId\)/);
    assert.match(syncSource, /return attachRunningTaskEventStream\(conversationId\)/);
    assert.match(monitor, /syncVisibleConversationTaskReplay\(normalizedTasks\)/);
    assert.match(sendSource, /await loadActiveTasks\(\)/);
    assert.match(sendSource, /if \(isCurrentChatTaskActive\(\)\)/);
    assert.ok(sendSource.indexOf('if (isCurrentChatTaskActive())') < sendSource.indexOf("addMessage('user'"));
    assert.match(sendSource, /window\.notifyConversationTaskStarted\(streamConversationId\)/);
});

test('Refresh stream catch-up reconciles final body from database on subscription race or terminal frame loss', () => {
    const attachSource = functionSource(monitor, 'attachRunningTaskEventStream', 'parseToolCallArgsFromData');
    const reconcileSource = functionSource(monitor, 'reconcileConversationAfterTaskReplay', 'cancelRunningTaskEventStream');

    assert.match(attachSource, /const eventStreamResponsePromise = apiFetch\(url/);
    assert.ok(attachSource.indexOf('const eventStreamResponsePromise') < attachSource.indexOf('loadProcessDetailsPaginated'));
    assert.match(attachSource, /if \(!active\) \{[\s\S]*?assistantMessageNeedsTaskReplayReconcile\(staleAssistant\)[\s\S]*?reconcileConversationAfterTaskReplay\(conversationId, true\)/);
    assert.match(attachSource, /if \(!response\.ok\) \{[\s\S]*?reconcileConversationAfterTaskReplay\(conversationId, true\)/);
    assert.match(attachSource, /if \(!replaySawDone\) \{[\s\S]*?reconcileConversationAfterTaskReplay/);
    assert.match(reconcileSource, /updateAssistantBubbleContent\(assistantEl\.id, finalMessage\.content \|\| '', true\)/);
    assert.match(reconcileSource, /loadProcessDetailsPaginated\(assistantEl\.id, finalMessage\.id,[\s\S]*?initialLatest: true,[\s\S]*?autoLoadAll: false/);
});

test('Message bubble internal stream height increase continues pinning only in following mode', () => {
    const bindSource = functionSource(scroll, 'bindChatScrollListeners', 'initChatScroll');

    assert.match(bindSource, /scrollMode === 'following'/);
    assert.match(bindSource, /scheduleChatScrollToBottomIfFollowing\(true\)/);
    assert.match(bindSource, /\{ childList: true, subtree: true, characterData: true \}/);
    assert.match(bindSource, /new ResizeObserver/);
    assert.match(bindSource, /chatMessagesResizeObserver\.observe\(el\)/);
    assert.match(bindSource, /(?:改变消息区 clientHeight|alter message area clientHeight)/);
    assert.match(bindSource, /Math\.abs\(e\.deltaY\) > 1/);
    assert.match(bindSource, /e\.deltaY < -1/);
    assert.match(bindSource, /e\.clientX >= rect\.right - 18/);
    assert.match(bindSource, /e\.key === 'ArrowUp'/);
});

test('Page loads smart scroll controller before task stream catch-up script', () => {
    const scrollIndex = html.indexOf('/static/js/chat-scroll.js?v=20260815-1');
    const monitorIndex = html.indexOf('/static/js/monitor.js?v=20260907-blocked-1');

    assert.notEqual(scrollIndex, -1);
    assert.notEqual(monitorIndex, -1);
    assert.ok(scrollIndex < monitorIndex);
});

test('Clicking a project chat directly also writes hash for resume and stream catch-up after refresh', () => {
    const loadSource = functionSource(chat, 'loadConversation', 'attachDeleteTurnButton');
    const syncSource = functionSource(chat, 'syncChatConversationHash', 'getConversationLiteFromCache');
    const streamSource = functionSource(monitor, 'setCurrentConversationIdFromStream', 'shouldSkipTaskEventReplayAttach');

    assert.match(syncSource, /window\.location\.hash\.split\('\?'\)\[0\] !== '#chat'/);
    assert.match(syncSource, /#chat\?conversation=/);
    assert.match(syncSource, /window\.history\.replaceState/);
    assert.match(loadSource, /syncChatConversationHash\(conversationId\)/);
    assert.match(streamSource, /window\.syncChatConversationHash\(cid\)/);
});

test('Task plan progress is event-driven; no fixed polling of plan-tasks when idle', () => {
    const planSource = fs.readFileSync('web/static/js/chat-plan-progress.js', 'utf8');
    assert.doesNotMatch(planSource, /setInterval\(fetchPlanTasks/);
    assert.match(planSource, /ACTIVE_POLL_INTERVAL_MS = 1500/);
    assert.match(planSource, /function shouldContinuePolling\(payload\)/);
    assert.match(planSource, /payload && payload\.running === false/);
    assert.match(planSource, /payload && payload\.running === true[\s\S]*?state\.tasks\.length > 0/);
    assert.match(planSource, /isCurrentConversationRunning\(\) && state\.tasks\.length > 0/);
    assert.match(planSource, /conversation-task-state-changed/);
    assert.match(monitor, /window\.dispatchEvent\(new CustomEvent\('conversation-task-state-changed'/);
    assert.match(monitor, /window\.isConversationTaskRunning = isConversationTaskRunning/);
    assert.match(html, /chat-plan-progress\.js\?v=20260815-1/);
});

test('Task plan progress events are emitted when the active task list changes or a new task starts', () => {
    const notifySource = functionSource(monitor, 'notifyConversationTaskStarted', 'initChatTaskSyncChannel');
    const renderSource = functionSource(monitor, 'renderActiveTasks', 'reconcileHitlApprovalStateWithActiveTasks');

    assert.match(notifySource, /conversation-task-state-changed/);
    assert.match(notifySource, /detail: \{ conversationId: id, running: true \}/);
    assert.match(renderSource, /conversationExecutionTracker\.update\(normalizedTasks\)/);
    assert.match(renderSource, /detail: \{ tasks: normalizedTasks \}/);
});

test('Active tasks are sorted stably by start time; refresh without changes does not rebuild stop buttons', () => {
    const sortSource = functionSource(monitor, 'stableActiveTasksForDisplay', 'activeTasksRenderSignature');
    const sortTasks = vm.runInNewContext(`(${sortSource.trim()})`);
    const tasks = [
        { conversationId: 'conversation-z', startedAt: '2026-08-19T10:00:00Z' },
        { conversationId: 'conversation-late', startedAt: '2026-08-19T10:01:00Z' },
        { conversationId: 'conversation-a', startedAt: '2026-08-19T10:00:00Z' }
    ];
    assert.deepEqual(
        Array.from(sortTasks(tasks), task => task.conversationId),
        ['conversation-a', 'conversation-z', 'conversation-late']
    );

    const renderSource = functionSource(monitor, 'renderActiveTasks', 'reconcileHitlApprovalStateWithActiveTasks');
    assert.match(renderSource, /nextVisualSignature === activeTasksVisualSignature/);
    assert.match(renderSource, /bar\.querySelectorAll\('\.active-task-item'\)\.length === normalizedTasks\.length/);
    assert.match(renderSource, /const previousScrollLeft = bar\.scrollLeft/);
    assert.match(renderSource, /bar\.scrollLeft = previousScrollLeft/);
});

test('During new chat initialisation, switching sessions prevents stale stream events from pulling the page back', () => {
    const guardSource = functionSource(chat, 'shouldIgnoreLiveChatStreamEvent', 'clearLiveChatStreamIfOwned');
    const shouldIgnore = vm.runInNewContext(`(${guardSource.trim()})`);
    const activeStream = { active: true, detached: false, navigationSeq: 7 };

    assert.equal(shouldIgnore(activeStream, activeStream, 7), false);
    activeStream.detached = true;
    assert.equal(shouldIgnore(activeStream, activeStream, 7), true);
    activeStream.detached = false;
    activeStream.active = false;
    assert.equal(shouldIgnore(activeStream, activeStream, 7), true);
    activeStream.active = true;
    assert.equal(shouldIgnore(activeStream, activeStream, 8), true);
    assert.equal(shouldIgnore({ active: true, detached: false, navigationSeq: 7 }, activeStream, 7), true);

    const sendSource = functionSource(chat, 'sendMessage', 'renderChatFileChips');
    const guardIndex = sendSource.indexOf('shouldIgnoreLiveChatStreamEvent(liveStreamState)');
    const handlerIndex = sendSource.indexOf('handleStreamEvent(eventData');
    assert.notEqual(guardIndex, -1);
    assert.notEqual(handlerIndex, -1);
    assert.ok(guardIndex < handlerIndex);
    assert.match(sendSource, /const requestNavigationSeq = chatConversationNavigationSeq;[\s\S]*?await loadActiveTasks\(\)/);
    assert.match(sendSource, /if \(requestNavigationSeq !== chatConversationNavigationSeq\) \{[\s\S]{0,80}return;/);
    assert.match(sendSource, /navigationSeq: requestNavigationSeq/);
    assert.match(sendSource, /if \(!streamConversationId\) \{[\s\S]{0,180}liveStreamState\.conversationId = eventConvId/);
    assert.match(sendSource, /if \(eventConvId\) updateProgressConversation\(progressId, eventConvId\);[\s\S]{0,80}return;/);

    const loadSource = functionSource(chat, 'loadConversation', 'attachDeleteTurnButton');
    const newConversationSource = functionSource(chat, 'startNewConversation', 'loadConversations');
    assert.match(loadSource, /markChatConversationNavigation\(conversationId\)/);
    assert.match(loadSource, /window\.cancelScheduledChatConversationFromHash\(\)/);
    assert.match(newConversationSource, /markChatConversationNavigation\('', true\)/);
    assert.match(newConversationSource, /clearChatConversationHash\(\)/);
    assert.match(router, /function cancelScheduledChatConversationFromHash\(\)[\s\S]{0,160}chatConversationFromHashSeq\+\+/);
    assert.match(chat, /function abandonChatConversationForPageNavigation\(\)[\s\S]{0,260}markChatConversationNavigation\('', true\)/);
    assert.match(chat, /abandonChatConversationForPageNavigation\(\)[\s\S]{0,420}detachLiveChatStreamForNavigation\('', true\)/);
    assert.match(router, /currentPage === 'chat'[\s\S]{0,140}window\.abandonChatConversationForPageNavigation\(\)/);
    assert.match(chat, /const targetConversationId = String\(item\.dataset\.conversationId \|\| ''\)\.trim\(\);[\s\S]{0,80}loadConversation\(targetConversationId\)/);
    assert.match(projects, /const targetConversationId = String\(event\.currentTarget && event\.currentTarget\.dataset\.conversationId \|\| ''\)\.trim\(\)/);
    assert.match(projects, /window\.loadConversation\(targetConversationId\)/);
    assert.match(chat, /let loadConversationPendingId = ''/);
    assert.match(chat, /window\.isChatConversationLoadPending = isChatConversationLoadPending/);
    const immediateSelectionIndex = loadSource.indexOf('currentConversationId = conversationId;');
    const conversationFetchIndex = loadSource.indexOf('await apiFetch(`/api/conversations/${conversationId}?include_process_details=0`');
    assert.notEqual(immediateSelectionIndex, -1);
    assert.notEqual(conversationFetchIndex, -1);
    assert.ok(immediateSelectionIndex < conversationFetchIndex);
    assert.match(monitor, /String\(window\.currentConversationId \|\| ''\) !== conversationId[\s\S]{0,300}window\.isChatConversationLoadPending\(conversationId\)/);
});

test('Refreshing a specific chat immediately resumes and does not flash no-project state before load completes', () => {
    const scheduleSource = functionSource(router, 'scheduleChatConversationFromHash', 'navigateToConversation');
    const restoreStateSource = functionSource(router, 'setChatConversationRestorePending', 'finishChatConversationRestore');
    const loadSource = functionSource(chat, 'loadConversation', 'attachDeleteTurnButton');
    const css = fs.readFileSync('web/static/css/style.css', 'utf8');

    assert.match(router, /scheduleChatConversationFromHash\(0\)/);
    assert.doesNotMatch(router, /scheduleChatConversationFromHash\((200|500)\)/);
    assert.match(scheduleSource, /setChatConversationRestorePending\(conversationId, true\)/);
    assert.match(restoreStateSource, /is-conversation-restoring/);
    assert.match(restoreStateSource, /aria-busy/);
    assert.match(loadSource, /finally \{[\s\S]*?finishChatConversationRestore\(conversationId\)/);
    assert.match(css, /\.chat-container\.is-conversation-restoring #chat-messages/);
    assert.match(css, /\.chat-container\.is-conversation-restoring #chat-input-container/);
    assert.match(html, /router\.js\?v=20260907-1/);
    assert.match(html, /chat\.js\?v=20260907-blocked-1/);
});

test('RefreshRunning reply reuses persisted planning and continues appending future increments', () => {
    const findSource = functionSource(monitor, 'findRestoredMainResponseStreamItem', 'responseStreamStateFromRestoredItem');
    const handleSource = functionSource(monitor, 'handleStreamEvent', 'hitlApprovalTranslate');

    assert.match(findSource, /timeline-item-planning/);
    assert.match(findSource, /dataset\.responseStreamId/);
    assert.match(handleSource, /case 'response_start':[\s\S]*?findRestoredMainResponseStreamItem/);
    assert.match(handleSource, /case 'response_delta':[\s\S]*?responseStreamStateFromRestoredItem/);
    assert.match(monitor, /item\.dataset\.responseStreamId = String\(options\.data\.streamId\)/);
});

test('Eino native model retry/failover events are visible in main chat and WebShell', () => {
    const handleSource = functionSource(monitor, 'handleStreamEvent', 'hitlApprovalTranslate');
    assert.match(handleSource, /case 'eino_model_retry'/);
    assert.match(handleSource, /formatEinoModelRetryTitle/);
    assert.match(handleSource, /case 'eino_model_failover'/);
    assert.match(handleSource, /formatEinoModelFailoverTitle/);
    assert.match(monitor, /function formatEinoModelRetryMessage/);
    assert.match(monitor, /function formatEinoModelFailoverMessage/);
    assert.match(webshell, /_et === 'eino_model_retry'/);
    assert.match(webshell, /_et === 'eino_model_failover'/);
});

test('Non-dashboard hash hides DefaultDashboard before routing is ready', () => {
    const css = fs.readFileSync('web/static/css/style.css', 'utf8');
    assert.match(html, /document\.documentElement\.classList\.add\('initial-route-pending'\)/);
    assert.match(router, /document\.documentElement\.classList\.remove\('initial-route-pending'\)/);
    assert.match(css, /html\.initial-route-pending \.content-area \{[\s\S]*?visibility: hidden;/);
});

test('RefreshResume running assistant message: hide processing placeholder and re-show final body', () => {
    const loadSource = functionSource(chat, 'loadConversation', 'attachDeleteTurnButton');
    const updateSource = functionSource(monitor, 'updateAssistantBubbleContent', 'isConversationTaskRunning');

    assert.match(loadSource, /hideAssistantPlaceholder: isAssistantPlaceholder/);
    assert.match(chat, /bubble\.hidden = true/);
    assert.match(updateSource, /assistant-placeholder-content/);
    assert.match(updateSource, /bubble\.hidden = false/);
});

test('After refresh stream task completes, force-collapse auto-expanded iteration details', () => {
    const collapseSource = functionSource(monitor, 'collapseAllProgressDetails', 'getAssistantId');
    const attachSource = functionSource(monitor, 'attachRunningTaskEventStream', 'parseToolCallArgsFromData');

    assert.match(collapseSource, /options/);
    assert.match(collapseSource, /forceCollapse/);
    assert.match(collapseSource, /delete detailsContainer\.dataset\.userExpanded/);
    assert.match(attachSource, /collapseAllProgressDetails\(finalAssistant\.id, progressId, \{ force: true \}\)/);
    assert.doesNotMatch(attachSource, /if \(keepExpanded\)/);
});

test('Dark mode user bubble uses coordinated deep blue-grey levels', () => {
    const css = fs.readFileSync('web/static/css/style.css', 'utf8');
    assert.match(css, /html\[data-theme="dark"\] \.message\.user \.message-bubble \{[\s\S]*?background: #1b2638;/);
    assert.match(css, /border-color: rgba\(96, 165, 250, 0\.18\)/);
});

test('Dark mode chat three-dot hover does not trigger light parent row background', () => {
    const css = fs.readFileSync('web/static/css/style.css', 'utf8');
    assert.match(css, /html\[data-theme="dark"\] \.project-conversation-row:hover \.project-conversation-item/);
    assert.match(css, /html\[data-theme="dark"\] \.project-folder-action:hover,[\s\S]*?background: rgba\(71, 85, 105, 0\.28\);[\s\S]*?box-shadow: none;/);
    assert.match(html, /style\.css\?v=20260907-blocked-1/);
});
