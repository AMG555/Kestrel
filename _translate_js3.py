#!/usr/bin/env python3
"""
Final JS translation pass: translate remaining Chinese comment lines and strings
by doing line-by-line replacements of complete comment lines.
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

# Strategy: for each file, read lines, find Chinese lines, translate them.
# For comments: translate the whole comment text.
# For strings: find the quoted Chinese and translate.

# Master phrase dictionary (Chinese -> English)  
# Covering all remaining patterns seen in JS files
PHRASES = {
    # dashboard.js
    "Dashboard页面：拉取运行中的对话、漏洞统计、批量任务、工具与 Skills 统计并渲染。": "Dashboard page: fetch running conversations, vulnerability stats, batch tasks, tool & skills stats and render.",
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
    "Current正在进行的 fetch 的 AbortController": "AbortController for the current in-flight fetch",
    "自动轮询的 setInterval id": "setInterval id for auto-polling",
    "过期检查的 setInterval id": "setInterval id for stale checks",
    "上次成功Refresh的Time戳（ms）": "Timestamp (ms) of the last successful refresh",
    # chat.js comments
    "离开聊天页时立即让尚在初始化的发送请求失去页面所有权。": "When leaving the chat page, immediately release ownership of any in-progress send request.",
    "后端任务仍会继续执行；这里只中止浏览器前台流，避免首个 conversation": "The backend task continues; this only cancels the browser foreground stream to prevent the first conversation",
    "事件在用户已经切到其他页面后再次抢占当前会话。": "event from re-claiming the current session after the user has already navigated away.",
    "轻量会话 LRU 缓存。": "Lightweight conversation LRU cache.",
    "缓存只用作请求失败时的降级数据，不能先于服务端响应直接渲染：": "Cache is only used as fallback data on request failure — must not render before the server response:",
    "运行中会话的 process details 会持续写入，直接渲染旧快照会让": "A running conversation's process details keep writing; rendering the old snapshot would cause",
    "UI 暂时回退到旧轮次，等 task-events 接管后又突然跳到最新轮次。": "the UI to briefly revert to an old turn, then jump to the latest turn when task-events take over.",
    "@ 提及相关状态": "@ mention related state",
    "IME输入法状态跟踪": "IME input method state tracking",
    "输入框草稿保存相关": "Input box draft save related",
    "500ms防抖延迟": "500ms debounce delay",
    "对话文件上传相关（后端会拼接路径与内容发给大模型，前端不再重复发文件列表）": "Conversation file upload (backend concatenates path+content to send to LLM; frontend no longer resends file list)",
    "对话模式：eino_single = Eino ADK 单代理（/api/eino-agent/stream）；deep / plan_execute / supervisor = Eino 多代理（/api/multi-agent/stream，请求体": "Conversation mode: eino_single = Eino ADK single-agent (/api/eino-agent/stream); deep / plan_execute / supervisor = Eino multi-agent (/api/multi-agent/stream, request body",
    "人机协同（HITL）会话级配置": "Human-AI collaboration (HITL) session-level configuration",
    "非阻塞提示（与 chat-files-toast 样式共用） */": "Non-blocking toast (shared style with chat-files-toast) */",
    "字符串拆成数组（逗号或换行分隔，与 textarea 一致） */": "Split string into array (comma or newline separated, consistent with textarea) */",
    "与 config.yaml hitl.tool_whitelist 相同格式；合并为输入框展示（全局项在前，去重不区分大小写） */": "Same format as config.yaml hitl.tool_whitelist; merged for input display (global items first, case-insensitive dedup) */",
    "保存/发请求前去掉全局白名单工具，避免会话里重复存 config 已有项 */": "Remove global whitelist tools before saving/sending to avoid duplicating items already in config */",
    "侧栏已改为自动保存；同步更新输入框快捷摘要。 */": "Sidebar now auto-saves; sync-update the input box shortcut summary. */",
    "侧栏人机协同配置写入本地、合并展示并尽量同步服务端 */": "Sidebar HITL config written locally, merged for display, and synced to server when possible */",
    "将 localStorage 规范为 eino_single | deep": "Normalise localStorage value to eino_single | deep",
    "External MCP名称列表": "External MCP name list",
    "与 handler.formatInterruptContinueUserMessage 首段一致；主对话不展示，仅迭代详情（user_interrupt_continue）": "Matches first paragraph of handler.formatInterruptContinueUserMessage; not shown in main chat, only in iteration details (user_interrupt_continue)",
    "对话附件：选文件后异步 POST /api/chat-uploads，发送时只传 serverPath（绝对路径），请求体不再内联大文件内容。": "Conversation attachments: async POST /api/chat-uploads after selecting files; only serverPath (absolute) is sent; request body no longer inlines large file content.",
    "保留最后一个不完整的行": "Keep the last incomplete line",
    "addFilesToChat 已提示": "addFilesToChat already notified",
    "工具本身的启用状态": "Tool's own enabled status",
    "在当前角色中的启用状态": "Enabled status in current role",
    "保存唯一标识符": "Save unique identifier",
    "启用的工具排在前面": "Enabled tools sorted first",
    # monitor.js comments
    "主对话 POST 流仍在读取时，禁止再挂 task-events 补流，否则同一事件会画两遍（与 HITL 是否开启无关）。": "When the main chat POST stream is still reading, do not attach a task-events补流, otherwise the same event renders twice (unrelated to whether HITL is enabled).",
    "window.__csAgentLiveStream 由 chat.js sendMessage 在读到 body 后设置，在 finally 中清除。": "window.__csAgentLiveStream is set by chat.js sendMessage after reading the body, cleared in finally.",
    "新会话：conversation 事件尚未到达前 conversationId 可能仍为 null，一律不补挂": "New conversation: conversationId may still be null before the first conversation event arrives — never attach補流",
    "监控页展示：内部 mcp::tool → 模型侧 mcp__tool": "Monitor page display: internal mcp::tool → model-side mcp__tool",
    "筛选/API：mcp__tool → 内部 mcp::tool（与库存一致）": "Filter/API: mcp__tool → internal mcp::tool (consistent with inventory)",
    "当前界面语言对应的 BCP 47 标签（与时间格式化一致）": "BCP 47 tag for the current UI language (consistent with time formatting)",
    "toLocaleTimeString 选项：中文用 24 小时制，避免仍显示 AM/PM": "toLocaleTimeString options: Chinese uses 24-hour format to avoid showing AM/PM",
    "将后端下发的进度文案转为当前语言的翻译（中英双向映射，切换语言后能跟上）": "Translate progress messages from backend to current language (bidirectional zh/en mapping, follows language switches)",
    "Plan-Execute：将 Eino 内部 agent 名本地化为进度条标题用语": "Plan-Execute: localise Eino internal agent names to progress bar title wording",
    "从 Plan-Execute 模型返回的单层 JSON 中取面向用户的字符串（replanner 常用 response）。": "Extract the user-facing string from the flat JSON returned by the Plan-Execute model (replanner commonly uses 'response').",
    "少数模型在 JSON 字符串里仍留下字面量 \"\\n\"；在已解出正文后再转成换行（不误伤 Windows 盘符等极少命中）。": 'A few models leave literal "\\n" in JSON strings; convert to newlines after extracting body text (rarely matches Windows drive letters etc.).',
    "在线正文：planner/replanner 的 {\"steps\":[...]} 转为列表；{\"response\":\"...\"} 解包为纯文本；": 'Streaming body: planner/replanner {"steps":[...]} converted to list; {"response":"..."} unpacked to plain text;',
    "同样解包。流式片段非法 JSON 时保持原文。": "Same unpack. Keep original text when stream fragment is invalid JSON.",
    "在线条目：Plan-Execute 主通道流式阶段标题（替代一律「规划中」）": "Streaming entry: Plan-Execute main channel streaming phase title (replaces the generic 'Planning')",
    "主通道有模型流式输出，不显「规划中」；模型偶发复述工具 stdout 时，旧文案易被误认为工具结果标题。": "Main channel has model streaming output, don't show 'Planning'; when model occasionally echoes tool stdout, the old label was mistaken for a tool result title.",
    "未捕获助手正文占位文案；终态 response 不应覆盖已有流式 buffer。": "Uncaptured assistant body placeholder; final response should not override existing streaming buffer.",
    "Run态与审批态需要及时自刷新": "Running and approval states need timely auto-refresh",
    "Current界面语言对应的 BCP 47 Tags（与Time格式化一致）": "BCP 47 tag for current UI language (consistent with time formatting)",
    "toLocaleTimeString 选项：Chinese用 24 小时制，避免仍显示 AM/PM": "toLocaleTimeString options: Chinese uses 24-hour format to avoid showing AM/PM",
    "将后端下发的进度文案转为Current语言的翻译（中英双向映射，切换语言后能跟上）": "Translate backend progress messages to current language (bidirectional zh/en mapping, follows language switches)",
    "新Conversation：conversation 事件尚未到达前 conversationId 可能仍为 null，一律不补挂": "New conversation: conversationId may still be null before first conversation event — never attach补挂",
    "监控页展示：内部 mcp::tool → Model侧 mcp__tool": "Monitor display: internal mcp::tool → model-side mcp__tool",
    "Filter/API：mcp__tool → 内部 mcp::tool（与库存一致）": "Filter/API: mcp__tool → internal mcp::tool (consistent with inventory)",
    "将后端下发的进度文案转为Current语言的翻译（中英双向映射，切换语言后能跟上）": "Translate backend progress messages to current language (bidirectional mapping, follows language switches)",
    "从 Plan-Execute ModelBack的单层 JSON 中取面向用户的字符串（replanner 常用 response）。": "Extract user-facing string from Plan-Execute model flat JSON (replanner commonly uses 'response').",
    "少数模型在 JSON 字符串里仍留下字面量 \"\\n\"；在已解出正文后再转成换行（不误伤 Windows 盘符等极少命中）。": "A few models leave literal \\n in JSON strings; convert after extracting body (rarely matches Windows paths etc.).",
    "在线正文：planner/replanner 的 {\"steps\":[...]} 转为列表；{\"response\":\"...\"} 解包为纯文本；": 'Streaming body: planner/replanner {"steps":[...]} → list; {"response":"..."} → plain text;',
    "同样解包。流式片段非法 JSON 时保持原文。": "Same unpack. Keep original when stream fragment is invalid JSON.",
    "在线条目：Plan-Execute 主通道流式阶段标题（替代一律「规划中」）": "Streaming entry: Plan-Execute main channel streaming phase title (replaces generic 'Planning')",
    "Plan-Execute：将 Eino 内部 agent 名本地化为进度条标题用语": "Plan-Execute: localise Eino internal agent names to progress bar title wording",
    # Common comment patterns across many files
    "初始化": "Initialisation",
    "渲染": "Render",
    "更新": "Update",
    "清理": "Cleanup",
    "清空": "Clear",
    "重置": "Reset",
    "刷新": "Refresh",
    "加载": "Load",
    "保存": "Save",
    "删除": "Delete",
    "添加": "Add",
    "创建": "Create",
    "编辑": "Edit",
    "提交": "Submit",
    "取消": "Cancel",
    "关闭": "Close",
    "打开": "Open",
    "显示": "Show",
    "隐藏": "Hide",
    "切换": "Toggle",
    "展开": "Expand",
    "折叠": "Collapse",
    "选择": "Select",
    "筛选": "Filter",
    "搜索": "Search",
    "排序": "Sort",
    "翻页": "Paginate",
    "分页": "Pagination",
    "复制": "Copy",
    "粘贴": "Paste",
    "移动": "Move",
    "拖拽": "Drag",
    "缩放": "Zoom",
    "滚动": "Scroll",
    "跳转": "Navigate",
    "导航": "Navigation",
    "路由": "Router",
    "状态": "State",
    "配置": "Configuration",
    "设置": "Settings",
    "参数": "Parameters",
    "变量": "Variable",
    "常量": "Constant",
    "函数": "Function",
    "方法": "Method",
    "类": "Class",
    "模块": "Module",
    "组件": "Component",
    "页面": "Page",
    "视图": "View",
    "模板": "Template",
    "布局": "Layout",
    "样式": "Style",
    "主题": "Theme",
    "图标": "Icon",
    "图表": "Chart",
    "表格": "Table",
    "列表": "List",
    "树形": "Tree",
    "菜单": "Menu",
    "按钮": "Button",
    "输入框": "Input box",
    "文本框": "Text box",
    "下拉框": "Dropdown",
    "复选框": "Checkbox",
    "单选框": "Radio button",
    "开关": "Toggle switch",
    "滑块": "Slider",
    "进度条": "Progress bar",
    "弹窗": "Modal",
    "对话框": "Dialog",
    "提示框": "Tooltip",
    "通知": "Notification",
    "提示": "Hint",
    "标签": "Label",
    "徽章": "Badge",
    "标题": "Title",
    "描述": "Description",
    "内容": "Content",
    "正文": "Body",
    "摘要": "Summary",
    "详情": "Details",
    "概览": "Overview",
    "统计": "Statistics",
    "图形": "Graph",
    "图表": "Chart",
    "仪表板": "Dashboard",
    "工具栏": "Toolbar",
    "侧边栏": "Sidebar",
    "导航栏": "Navigation bar",
    "头部": "Header",
    "底部": "Footer",
    "分隔符": "Separator",
    "容器": "Container",
    "包装器": "Wrapper",
    "插槽": "Slot",
    "占位符": "Placeholder",
    "空状态": "Empty state",
    "错误状态": "Error state",
    "加载状态": "Loading state",
    "成功状态": "Success state",
    "警告状态": "Warning state",
    "信息状态": "Info state",
    "默认状态": "Default state",
    "禁用状态": "Disabled state",
    "激活状态": "Active state",
    "悬停状态": "Hover state",
    "焦点状态": "Focus state",
    "选中状态": "Selected state",
    "展开状态": "Expanded state",
    "折叠状态": "Collapsed state",
    "全屏状态": "Fullscreen state",
    "离线状态": "Offline state",
    "在线状态": "Online state",
    "运行状态": "Running state",
    "停止状态": "Stopped state",
    "完成状态": "Completed state",
    "失败状态": "Failed state",
    "取消状态": "Cancelled state",
    "等待状态": "Waiting state",
    "处理状态": "Processing state",
    "连接状态": "Connection state",
    "认证状态": "Authentication state",
    "权限状态": "Permission state",
}

def translate_line(line):
    """Translate all Chinese in a single line using phrase lookup + char-by-char fallback."""
    if not re.search(r"[\u4e00-\u9fff]", line):
        return line
    
    result = line
    # Apply phrase replacements (longest first)
    for zh, en in sorted(PHRASES.items(), key=lambda x: -len(x[0])):
        result = result.replace(zh, en)
    
    # Apply i18n reverse lookup for remaining quoted strings
    def repl_i18n_fallback(m):
        key = m.group(1)
        zh_text = m.group(2)
        en = get_en(key)
        if en and not re.search(r"[\u4e00-\u9fff]", en):
            return f"'{key}', '{en}'"
        return m.group(0)
    
    result = re.sub(r"'([a-zA-Z][a-zA-Z0-9._]+)',\s*'([\u4e00-\u9fff][^']*)'", repl_i18n_fallback, result)
    result = re.sub(r'"([a-zA-Z][a-zA-Z0-9._]+)",\s*"([\u4e00-\u9fff][^"]*)"', repl_i18n_fallback, result)
    
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
        
        lines = content.split("\n")
        new_lines = [translate_line(line) for line in lines]
        new_content = "\n".join(new_lines)
        
        after = len(re.findall(r"[\u4e00-\u9fff]", new_content))
        total_before += before
        total_after += after
        
        with open(path, "w", encoding="utf-8") as f:
            f.write(new_content)
        
        if after > 0:
            print(f"  PARTIAL {path}: {before} -> {after}")

print(f"\nTotal: {total_before} -> {total_after} remaining")
