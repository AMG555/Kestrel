#!/usr/bin/env python3
"""
Targeted translation for chat.js and other JS files.
Handles: 1) i18n fallback patterns, 2) comment line translation.
"""
import re, os, json

with open("web/static/i18n/en-US.json", "r", encoding="utf-8-sig") as f:
    en_i18n = json.load(f)

def get_en(key):
    parts = key.split(".")
    d = en_i18n
    for p in parts:
        if isinstance(d, dict) and p in d:
            d = d[p]
        else:
            return None
    return d if isinstance(d, str) else None

# === Step 1: Replace i18n fallback patterns ===
# Pattern: chatTranslate('key', 'Chinese') or t('key', 'Chinese') or window.t('key')
def replace_i18n_fallbacks(content):
    """Replace Chinese fallback strings in i18n call patterns."""
    
    def repl_single(m):
        key = m.group(1)
        zh = m.group(2)
        en = get_en(key)
        if en and not re.search(r"[\u4e00-\u9fff]", en):
            return f"'{key}', '{en}'"
        return m.group(0)
    
    def repl_double(m):
        key = m.group(1)
        zh = m.group(2)
        en = get_en(key)
        if en and not re.search(r"[\u4e00-\u9fff]", en):
            return f"'{key}', \"{en}\""
        return m.group(0)
    
    content = re.sub(r"'([a-zA-Z][a-zA-Z0-9._]+)',\s*'([^']*[\u4e00-\u9fff][^']*)'", repl_single, content)
    content = re.sub(r"'([a-zA-Z][a-zA-Z0-9._]+)',\s*\"([^\"]*[\u4e00-\u9fff][^\"]*)\"", repl_double, content)
    return content

# === Step 2: Replace remaining Chinese strings (not in i18n) ===
DIRECT_STRINGS = {
    # inline comment text (appears after //)
    "External MCP名称列表": "External MCP name list",
    "500ms防抖延迟": "500ms debounce delay",
    "@ 提及相关状态": "@ mention related state",
    "IME输入法状态跟踪": "IME input method state tracking",
    "输入框草稿Save相关": "Input box draft save related",
    "Chat文件Upload相关（后端会拼接路径与内容发给大Model，前端不再重复发文件列表）": "Chat file upload (backend concatenates path+content for LLM; frontend no longer resends file list)",
    "人机协同（HITL）会话级配置": "Human-AI collaboration (HITL) session-level configuration",
    "非阻塞提示（与 chat-files-toast 样式共用）": "Non-blocking toast (shared style with chat-files-toast)",
    "字符串拆成数组（逗号或换行分隔，与 textarea 一致）": "Split string into array (comma or newline separated, consistent with textarea)",
    "与 config.yaml hitl.tool_whitelist 相同格式；合并为输入框展示（全局项在前，去重不区分大小写）": "Same format as config.yaml hitl.tool_whitelist; merged for display (global items first, case-insensitive dedup)",
    "保存/发请求前去掉全局白名单工具，避免会话里重复存 config 已有项": "Remove global whitelist tools before saving/sending to avoid duplicating items already in config",
    "侧栏已改为自动保存；同步更新输入框快捷摘要。": "Sidebar now auto-saves; sync-update the input box shortcut summary.",
    "侧栏人机协同配置：自动写入本地、合并展示并尽量同步服务端": "Sidebar HITL config: auto-written locally, merged for display, synced to server when possible",
    "输入框右侧展示当前会话通道的模型；审批模型只出现在 HITL 入口。": "Right side of input box shows current session channel's model; approval model only appears in HITL entry.",
    "选择 AI channel、Model与推理设置": "Select AI channel, model and reasoning settings",
    "离开正在读取主 POST 流的Chat时，只断开浏览器侧响应流，不Stop后端Task。": "When leaving a chat reading the main POST stream, only disconnect the browser-side response stream; do not stop the backend task.",
    "后端Task使用 detachedAgentContext，仍会ContinueRun；重新进入该Chat时由": "The backend task uses detachedAgentContext and continues running; when re-entering the chat,",
    "task-events 镜像流接管。这样同时Run多个Chat也只占用一个前台长Connection，": "the task-events mirror stream takes over. Running multiple chats only occupies one foreground long connection,",
    "不会耗尽浏览器对同一主机的Connection槽位而卡住普通 GET/POST 请求。": "preventing browser connection slots to the same host from being exhausted.",
    "供 WebShell 等复用：在 Eino Path下Back reasoning 请求片段或 undefined": "Reusable by WebShell etc.: returns reasoning request fragment or undefined for Eino path",
    "先展示基础Mode，避免首次Sign in时Configuration接口短暂Failed导致入口被Hide。": "Show basic mode first to avoid hiding entry if config API briefly fails on first login.",
    "保存输入框草稿到localStorage（防抖版本）": "Save input box draft to localStorage (debounced)",
    "清除之前的定时器": "Clear previous timer",
    "设置新的定时器": "Set new timer",
    "保存输入框草稿到localStorage": "Save input box draft to localStorage",
    "不要把占位提示本身当作草稿保存": "Do not save the placeholder hint text as a draft",
    "如果内容为空或等于占位提示，清除保存的草稿": "If content is empty or equals the placeholder hint, clear the saved draft",
    "localStorage可能已满或不可用，静默失败": "localStorage may be full or unavailable, fail silently",
    "从localStorage恢复输入框草稿": "Restore input box draft from localStorage",
    "若当前 value 与 placeholder 相同，说明提示被误当作内容，清除以便正确显示Placeholder": "If current value equals placeholder, the hint was mistakenly treated as content — clear to show placeholder correctly",
    "如果输入框已有内容，不恢复草稿（避免覆盖用户输入）": "If the input box already has content, do not restore the draft (to avoid overwriting user input)",
    "如果草稿内容和占位提示一样，则认为是无效草稿，不恢复": "If the draft content equals the placeholder hint, treat it as invalid and do not restore",
    "调整输入框高度以适应内容": "Adjust input box height to fit content",
    "清除掉无效草稿，避免之后继续干扰": "Clean up invalid draft to avoid interference later",
    "清除保存的草稿": "Clear saved draft",
    "同步清除，确保立即生效": "Synchronously clear to ensure immediate effect",
    "调整textarea高度以适应内容": "Adjust textarea height to fit content",
    "先重置高度为auto，然后立即设置为固定值，确保能准确获取scrollHeight": "First reset height to auto, then set to a fixed value to accurately get scrollHeight",
    "强制浏览器重新计算布局": "Force browser to recalculate layout",
    "计算新高度（最小40px，最大不超过300px）": "Calculate new height (min 40px, max 300px)",
    "如果内容为空或只有很少内容，立即重置到最小高度": "If content is empty or minimal, immediately reset to minimum height",
    "Enter 会直接调用 sendMessage；同一会话在其他标签页已启动任务时，": "Enter directly calls sendMessage; when the same conversation already has a running task in another tab,",
    "保留当前页面，不再把这次尚未发出的请求写入新的可见Chat。": "keep the current page and do not write this unsent request to the newly visible chat.",
    "显示用户消息（含附件名，便于用户确认）": "Show user message (including attachment names for user confirmation)",
    "清除防抖定时器，防止在清除输入框后重新保存草稿": "Clear debounce timer to prevent re-saving draft after clearing the input box",
    "立即清除草稿，防止页面刷新时恢复": "Immediately clear draft to prevent restoration on page refresh",
    "使用同步方式确保草稿被清除": "Use synchronous method to ensure draft is cleared",
    "立即清除输入框并清除草稿（在发送请求之前）": "Immediately clear input box and draft (before sending the request)",
    "强制重置输入框高度为初始高度（40px）": "Force-reset input box height to initial height (40px)",
    "构建请求体（含附件）": "Build request body (including attachments)",
    "发送后清除附件列表": "Clear attachment list after sending",
    "创建进度消息容器（使用详细的进度展示）": "Create progress message container (using detailed progress display)",
    "切换Chat后仍可能收到旧响应流中已缓冲的 conversation、response_start": "After switching chats, buffered conversation/response_start events from the old stream may still arrive",
    "或 response 事件。它们只能补齐后台Task归属，不能重新抢占当前会话。": "or response events. They can only fill in the background task attribution, not re-claim the current session.",
    "处理剩余的buffer": "Process remaining buffer",
    "消息发送成功后，再次确保草稿被清除": "After message sent successfully, ensure the draft is cleared again",
    "工具本身的启用状态": "Tool's own enabled status",
    "在当前角色中的启用状态": "Enabled status in current role",
    "保存唯一标识符": "Save unique identifier",
    "启用的工具排在前面": "Enabled tools sorted first",
    # monitor.js
    "主Chat POST 流仍在读取时，禁止再挂 task-events 补流，否则同一事件会画两遍（与 HITL 是否开启无关）。": "When the main chat POST stream is still reading, do not attach a task-events补流; otherwise the same event renders twice (unrelated to whether HITL is enabled).",
    "window.__csAgentLiveStream 由 chat.js sendMessage 在读到 body 后设置，在 finally 中清除。": "window.__csAgentLiveStream is set by chat.js sendMessage after reading the body, cleared in finally.",
    "新会话：conversation 事件尚未到达前 conversationId 可能仍为 null，一律不补挂": "New conversation: conversationId may still be null before first conversation event arrives — never attach",
    "监控页展示：内部 mcp::tool → 模型侧 mcp__tool": "Monitor page display: internal mcp::tool → model-side mcp__tool",
    "筛选/API：mcp__tool → 内部 mcp::tool（与库存一致）": "Filter/API: mcp__tool → internal mcp::tool (consistent with inventory)",
    "当前界面语言对应的 BCP 47 标签（与时间格式化一致）": "BCP 47 tag for current UI language (consistent with time formatting)",
    "toLocaleTimeString 选项：中文用 24 小时制，避免仍显示 AM/PM": "toLocaleTimeString options: Chinese uses 24-hour format to avoid showing AM/PM",
    "将后端下发的进度文案转为当前语言的翻译（中英双向映射，切换语言后能跟上）": "Translate progress messages from backend to current language (bidirectional zh/en mapping, follows language switches)",
    "Plan-Execute：将 Eino 内部 agent 名本地化为进度条标题用语": "Plan-Execute: localise Eino internal agent names to progress bar title wording",
    "从 Plan-Execute 模型返回的单层 JSON 中取面向用户的字符串（replanner 常用 response）。": "Extract the user-facing string from the flat JSON returned by the Plan-Execute model (replanner commonly uses 'response').",
    "在线正文：planner/replanner 的 {\"steps\":[...]} 转为列表；{\"response\":\"...\"} 解包为纯文本；": 'Streaming body: planner/replanner {"steps":[...]} → list; {"response":"..."} → plain text;',
    "同样解包。流式片段非法 JSON 时保持原文。": "Same unpack. Keep original text when stream fragment is invalid JSON.",
    "在线条目：Plan-Execute 主通道流式阶段标题（替代一律「规划中」）": "Streaming entry: Plan-Execute main channel streaming phase title (replaces the generic 'Planning')",
    "主通道有模型流式输出，不显「规划中」；模型偶发复述工具 stdout 时，旧文案易被误认为工具结果标题。": "Main channel has model streaming output, don't show 'Planning'; when model occasionally echoes tool stdout, the old label was mistaken for a tool result title.",
    "未捕获助手正文占位文案；终态 response 不应覆盖已有流式 buffer。": "Uncaptured assistant body placeholder; final response should not override existing streaming buffer.",
    "Run态与审批态需要及时自刷新": "Running and approval states need timely auto-refresh",
    # dashboard
    "工程基础设施：": "Engineering infrastructure:",
    "dashboardState 集中保存运行时状态（in-flight controller / 自动轮询 timer / 上次更新时间 /": "dashboardState centralises runtime state (in-flight controller / auto-poll timer / last-updated time /",
    "已被本会话忽略的告警条 reasons）；": "alert bar reasons dismissed in this session);",
    "每次 refreshDashboard 入口 abort 上一个 controller，把 signal 传给所有 apiFetch，": "Each refreshDashboard entry aborts the previous controller, passes signal to all apiFetch calls,",
    "避免快速连点 / 自动轮询触发 race condition；": "to avoid race conditions from rapid clicks / auto-polling;",
    "自动轮询：startDashboardAutoRefresh() 每 60 秒拉一次；页面切走 / tab 隐藏时自动暂停，": "Auto-polling: startDashboardAutoRefresh() fetches every 60 s; auto-pauses when page/tab hidden,",
    "再切回时立即补一次刷新（基于 lastUpdatedAt 避免无效请求）；": "immediately re-fetches on return (based on lastUpdatedAt to avoid unnecessary requests);",
    "过期检测：updateLastUpdatedNow 记录时间戳；checkDashboardStale 每 30 秒检查，": "Stale detection: updateLastUpdatedNow records timestamp; checkDashboardStale checks every 30 s,",
    "超过 5 分钟未刷新则在「上次更新」徽章上加 .is-stale 类（变灰 + 显示 ⚠️）。": "adds .is-stale class to the 'last updated' badge (greys out + shows ⚠️) if not refreshed in 5 min.",
    "当前正在进行的 fetch 的 AbortController": "AbortController for the current in-flight fetch",
    "自动轮询的 setInterval id": "setInterval id for auto-polling",
    "过期检查的 setInterval id": "setInterval id for stale checks",
    "上次成功刷新的时间戳（ms）": "Timestamp (ms) of the last successful refresh",
    "接入概览 Tab：c2 | webshell": "Access overview tab: c2 | webshell",
    "severityTotalEl 在后续渲染逻辑中也被引用，必须在 loading 分支外声明": "severityTotalEl is also referenced in later render logic, must be declared outside the loading branch",
    # common inline comment patterns
    "保留最后一个不完整的行": "keep the last incomplete line",
    "addFilesToChat 已提示": "addFilesToChat already notified",
    # other files
    "Workflows审批": "Workflow approval",
    "工作流审批": "Workflow approval",
    # i18n fallbacks that are partial
    "已获取 {count} Model": "Fetched {count} models",
    "正在Save…": "Saving…",
    "应用Model失败": "Failed to apply model",
    "已自动Save": "Auto-saved",
    "获取Model失败": "Failed to fetch models",
    "开启": "On",
    "自动": "Auto",
    "系统": "System",
    "选择 AI channel、Model与推理设置": "Select AI channel, model and reasoning settings",
    # chat-scroll.js specific
    "聊天滚动监测": "Chat scroll monitoring",
    "用户最近滚动": "User recently scrolled",
    "自动滚动": "Auto-scroll",
    "距底部": "distance from bottom",
    # various
    "少数模型在 JSON 字符串里仍留下字面量": 'A few models leave literal',
    "在已解出正文后再转成换行（不误伤 Windows 盘符等极少命中）。": "convert to newlines after extracting body text (rarely matches Windows drive letters etc.).",
    "Chat模式：eino_single = Eino ADK single-agent（/api/eino-agent/stream）；deep / plan_execute / supervisor = Eino 多代理（/api/m": "Conversation mode: eino_single = Eino ADK single-agent (/api/eino-agent/stream); deep/plan_execute/supervisor = Eino multi-agent (/api/m",
    "Chat附件：选文件后异步 POST /api/chat-uploads，发送时只传 serverPath（绝对路径），请求体不再内联大文件内容。": "Chat attachments: async POST /api/chat-uploads after selecting files; only serverPath (absolute) is sent; request body no longer inlines large file content.",
    "离开聊天页时立即让尚在初始化的发送请求失去页面所有权。": "When leaving the chat page, immediately release page ownership from any in-progress send request.",
    "后端任务仍会继续Execute；这里只中止浏览器前台流，避免首个 conversation": "The backend task continues; this only cancels the browser foreground stream, preventing the first conversation",
    "事件在用户已经切到其他页面后再次抢占Current会话。": "event from re-claiming the current session after the user has navigated away.",
    "轻量会话 LRU 缓存。": "Lightweight conversation LRU cache.",
    "缓存只用作Request failed时的降级数据，不能先于服务端响应直接渲染：": "Cache is fallback data for failed requests only — must not render before the server response:",
    "Running会话的 process details 会持续写入，直接渲染旧快照会让": "A running conversation's process details keep writing; rendering the old snapshot would cause",
    "UI 暂时回退到旧轮次，等 task-events 接管后又突然跳到最新轮次。": "the UI to briefly revert to an old turn, then jump to the latest turn when task-events take over.",
    "与 handler.formatInterruptContinueUserMessage 首段一致；主Chat不展示，仅迭代Details（user_interrupt_continue）": "Matches first paragraph of handler.formatInterruptContinueUserMessage; not shown in main chat, only in iteration details",
    # Mixed patterns that are comments
    "仍有工具执行未结束": "Tool execution still in progress",
    "本轮要求执行证据，但没有 completed Tool": "This turn requires execution evidence, but no completed tool",
    "正在等待人工确认": "Waiting for manual confirmation",
    "未捕获到有效最终文本": "No valid final text captured",
    "仍为 ' + s.replace(": "still ' + s.replace(",
}

def apply_translations(content):
    result = content
    for zh, en in sorted(DIRECT_STRINGS.items(), key=lambda x: -len(x[0])):
        result = result.replace(zh, en)
    return result

total_before = 0
total_after = 0

for root, dirs, files in os.walk("web/static/js"):
    dirs[:] = [d for d in dirs if d != "vendor"]
    for fname in sorted(files):
        if not fname.endswith((".js", ".cjs")):
            continue
        path = os.path.join(root, fname)
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            content = f.read()
        
        before = len(re.findall(r"[\u4e00-\u9fff]", content))
        if before == 0:
            continue
        
        # Step 1: Replace i18n fallbacks
        new_content = replace_i18n_fallbacks(content)
        # Step 2: Replace direct strings
        new_content = apply_translations(new_content)
        
        after = len(re.findall(r"[\u4e00-\u9fff]", new_content))
        total_before += before
        total_after += after
        
        with open(path, "w", encoding="utf-8") as f:
            f.write(new_content)
        
        if after > 0:
            print(f"  {after:5d} remaining: {path}")

print(f"\nTotal: {total_before} -> {total_after} ({total_before - total_after} replaced)")
