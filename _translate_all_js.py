#!/usr/bin/env python3
"""
Comprehensive JS translation script.
Translates all Chinese text in web/static/js/ files to English.
Strategy:
  1. i18n translate-call fallbacks -> looked up from en-US.json
  2. Comment lines -> translated directly
  3. String literals -> translated directly
  4. Mixed lines -> replace only Chinese substrings
"""

import os
import re
import json

# ── helpers ──────────────────────────────────────────────────────────────────
CHINESE_RE = re.compile(r'[\u4e00-\u9fff\u3400-\u4dbf\uff00-\uffef\u2e80-\u2eff\u3000-\u303f]')

def has_chinese(s):
    return bool(CHINESE_RE.search(s))

def count_chinese(s):
    return len(CHINESE_RE.findall(s))

# ── i18n lookup ───────────────────────────────────────────────────────────────
i18n = json.loads(open('web/static/i18n/en-US.json', encoding='utf-8-sig').read())

def i18n_lookup(key):
    """Resolve a dotted key like 'chat.send' from en-US.json."""
    parts = key.split('.')
    node = i18n
    for p in parts:
        if isinstance(node, dict) and p in node:
            node = node[p]
        else:
            return None
    return node if isinstance(node, str) else None

# ── master phrase table ───────────────────────────────────────────────────────
# Format: (chinese_text, english_replacement)
# Ordered longest-first to avoid partial matches.
PHRASES = [
    # ── very long multi-word phrases first ───────────────────────────────────
    ("离开聊天页时立即让尚在初始化的发送请求失去页面所有权。", "Immediately relinquish page ownership for send requests still initializing when leaving the chat page."),
    ("后端任务仍会继续Execute；这里只中止浏览器前台流，避免首个 conversation", "The backend task will continue to run; this only aborts the browser foreground stream, preventing the first conversation"),
    ("事件在用户已经切到其他页面后再次抢占Current会话。", "event from re-hijacking the current session after the user has switched to another page."),
    ("缓存只用作Request failed时的降级数据，不能先于服务端响应直接渲染：", "Cache is only used as fallback data when a request fails; it must not be rendered directly before the server response:"),
    ("Running会话的 process details 会持续写入，直接渲染旧快照会让", "A running session's process details are continuously written; rendering an old snapshot directly would cause"),
    ("UI 暂时回退到旧轮次，等 task-events 接管后又突然跳到最新轮次。", "the UI to temporarily revert to an old turn, then suddenly jump to the latest turn after task-events takes over."),
    ("与 handler.formatInterruptContinueUserMessage 首段一致；主Chat不展示，仅迭代Details（user_interrupt_continue）", "Consistent with the first paragraph of handler.formatInterruptContinueUserMessage; not shown in main chat, only in iteration details (user_interrupt_continue)"),
    ("Chat附件：选文件后异步 POST /api/chat-uploads，发送时只传 serverPath（绝对路径），请求体不再内联大文件内容。", "Chat attachments: after selecting a file, async POST /api/chat-uploads; only serverPath (absolute path) is sent, request body no longer inlines large file content."),
    ("离开正在读取主 POST 流的Chat时，只断开浏览器侧响应流，不Stop后端任务。", "When leaving a chat that is reading the main POST stream, only disconnect the browser-side response stream, do not stop the backend task."),
    ("后端任务使用 detachedAgentContext，仍会继续Run；重新进入该Chat时由", "The backend task uses detachedAgentContext and will continue to run; re-entering the chat will have"),
    ("task-events 镜像流接管。这样同时Run多个Chat也只占用一个前台长连接，", "task-events mirror stream take over. This way, running multiple chats simultaneously only occupies one foreground long connection,"),
    ("不会耗尽浏览器对同一主机的连接槽位而卡住普通 GET/POST 请求。", "and will not exhaust the browser's connection slots for the same host, blocking normal GET/POST requests."),
    ("主Chat POST 流仍在读取时，禁止再挂 task-events 补流，否则同一事件会画两遍（与 HITL 是否开启无关）。", "While the main Chat POST stream is still being read, prohibit attaching task-events supplemental stream, otherwise the same event would be rendered twice (regardless of whether HITL is enabled)."),
    ("window.__csAgentLiveStream 由 chat.js sendMessage 在读到 body 后设置，在 finally 中清除。", "window.__csAgentLiveStream is set by chat.js sendMessage after reading the body, and cleared in finally."),
    ("新会话：conversation 事件尚未到达前 conversationId 可能仍为 null，一律不补挂", "New session: conversationId may still be null before the conversation event arrives; never attach supplemental stream"),
    ("主通道 response 结束时：将流式占位条目固化为 planning（与后端 flushResponsePlan 落库类型一致），", "When the main channel response ends: solidify the streaming placeholder entry as planning (consistent with backend flushResponsePlan DB type),"),
    ("避免 integrateProgressToMCPSection 快照前Delete占位导致「Assistant output」仅Refresh后才出现。", "to avoid the placeholder being deleted before integrateProgressToMCPSection snapshot, causing 'Assistant output' to only appear after refresh."),
    ("少数Model在 JSON 字符串里仍留下字面量 \"\\n\"；在已解出正文后再转成换行（不误伤 Windows 盘符时极少命中）。", "A few models still leave literal \"\\n\" in JSON strings; convert to newlines after extracting the body text (rarely triggered for Windows drive letters)."),
    ("Plan-Execute 时间线正文：planner/replanner 的 {\"steps\":[...]} 转为列表；{\"response\":\"...\"} 解包为纯文本；", "Plan-Execute timeline body: planner/replanner {\"steps\":[...]} converted to list; {\"response\":\"...\"} unpacked to plain text;"),
    ("executor 同样解包。流式片段非法 JSON 时保持原文。", "executor similarly unpacked. Keep original text when streaming fragment is invalid JSON."),
    ("时间线条目：Plan-Execute 主通道流式阶段标题（替代一律「Planning」）", "Timeline entry: Plan-Execute main channel streaming phase title (replaces generic 'Planning')"),
    ("eino_single / deep / supervisor：主通道是Model流式Output，不是「规划」；Model偶发复述工具 stdout 时，旧文案易被误认为工具结果标题。", "eino_single / deep / supervisor: main channel is model streaming output, not 'planning'; when models occasionally paraphrase tool stdout, the old label is easily mistaken for a tool result title."),
    ("Eino 未捕获助手正文占位文案；终态 response 不应覆盖已有流式 buffer。", "Eino uncaptured assistant body placeholder; terminal response should not overwrite existing streaming buffer."),
    ("将后端下发的进度文案转为Current语言的翻译（中英双向映射，切换语言后能跟上）", "Convert backend-supplied progress text to the current language translation (bidirectional CN/EN mapping, tracks language switches)"),
    ("Plan-Execute：将 Eino 内部 agent 名本地化为进度条标题用语", "Plan-Execute: localize Eino internal agent names for progress bar title display"),
    ("从 Plan-Execute ModelBack的单层 JSON 中取面向用户的字符串（replanner 常用 response）。", "Extract user-facing string from single-layer JSON returned by Plan-Execute model (replanner commonly uses response)."),
    ("兼容历史遗留：若 map 中still有旧格式 key（仅 toolCallId），兜底读取。", "Legacy compatibility: if the map still has old-format keys (toolCallId only), fall back to reading them."),
    ("已收到终态结果的调用仅清理索引，不能在任务收尾时被改写成失败。", "Calls that have received a terminal result only clean up the index; they must not be rewritten as failed at task end."),
    ("Plan-Execute 多轮 executor/planner 同名代理：仅在同轮次内复用流式条目", "Plan-Execute multi-round executor/planner same-name agents: only reuse streaming entries within the same round"),
    ("仅合并 Eino 对同一段 MessageStream 重复发出的 response_start", "Only merge duplicate response_start events Eino emits for the same MessageStream"),
    ("Refresh后从数据库Resume的 planning 行，继续复用为Current response_stream 容器。", "Planning rows resumed from the database after refresh continue to be reused as the current response_stream container."),
    ("Eino 多代理：时间线标题前加 [agentId]，标明哪一代理产生该工具调用/结果/回复", "Eino multi-agent: prefix timeline title with [agentId] to indicate which agent produced the tool call/result/reply"),
    ("主/子代理视觉区分：左边框与浅底色（与工具黄/绿状态并存时由具体项类型覆盖次要边）", "Primary/sub-agent visual distinction: left border and light background (overridden by item type when coexisting with tool yellow/green status)"),
    ("Model/网关偶发把「思考」混进正文，用伪 XML 包裹（如 &lt;redacted_thinking&gt;…&lt;/redacted_thinking&gt;）。", "Models/gateways occasionally mix 'thinking' into the body text, wrapped in pseudo XML (e.g. &lt;redacted_thinking&gt;…&lt;/redacted_thinking&gt;)."),
    ("与 Markdown 列表混排时，结束标签常被吞进 &lt;li&gt;，其后 **、` 等行内语法All无法解析；成对块整段移除。", "When mixed with Markdown lists, closing tags are often swallowed into &lt;li&gt;, causing subsequent ** and ` inline syntax to fail parsing; matched block pairs are removed entirely."),
    ("供 WebShell 等复用：在 Eino 路径下Back reasoning 请求片段或 undefined", "For reuse by WebShell etc.: returns the reasoning request fragment under the Eino path, or undefined"),
    ("先展示基础模式，避免首次Sign in时配置接口短暂失败导致入口被隐藏。", "Show basic mode first, to prevent the entry point from being hidden due to a brief config API failure on first login."),
    ("非阻塞提示（与 chat-files-toast 样式共用）", "Non-blocking notification (shared with chat-files-toast styles)"),
    ("白名单字符串拆成数组（逗号或换行分隔，与 textarea 一致）", "Split whitelist string into array (comma or newline separated, consistent with textarea)"),
    ("与 config.yaml hitl.tool_whitelist 合并为输入框展示（全局项在前，去重不区分大小写）", "Merged with config.yaml hitl.tool_whitelist for input display (global items first, case-insensitive dedup)"),
    ("Save/发请求前去掉全局白名单工具，避免会话里重复存 config 已有项", "Remove global whitelist tools before saving/sending request, to avoid re-storing items already in config within the session"),
    ("侧栏已改为自动Save；同步更新输入框快捷摘要。", "Sidebar has been changed to auto-save; synchronously update the input shortcut summary."),
    ("侧栏人机协同：自动写入本地、合并展示并尽量同步服务端", "Sidebar HITL: auto-write to local, merge display and try to sync with server"),
    ("将 localStorage 规范为 eino_single | deep | plan_execute | supervisor", "Normalize localStorage to eino_single | deep | plan_execute | supervisor"),
    ("输入框右侧展示Current会话通道的Model；审批Model只出现在 HITL 入口。", "Show current session channel model on the right side of the input; approval model only appears at the HITL entry."),
    ("fenced 块占位（BMP 私用区，正文几乎不会出现）", "Fenced block placeholder (BMP private use area, rarely appears in body text)"),
    ("存储工具调用ID到DOM元素的映射，用于更新Execute状态。", "Store mapping of tool call IDs to DOM elements for updating execution status."),
    ("键必须带 progressId 作用域，避免不同任务复用相同 toolCallId 时串线。", "Keys must be scoped with progressId to avoid cross-talk when different tasks reuse the same toolCallId."),
    ("Model流式Output缓存：progressId -> { assistantId, buffer }", "Model streaming output cache: progressId -> { assistantId, buffer }"),
    ("主通道Current迭代轮次缓存：progressId -> { iteration, orchestration }", "Main channel current iteration round cache: progressId -> { iteration, orchestration }"),
    ("Workflows多 Agent 节点切换时Clear流式聚合，避免推理/Output条目覆盖上一节点内容", "Clear streaming aggregation when Workflows multi-agent node switches, to prevent reasoning/output entries from overwriting previous node content"),
    ("同一段主通道流式Output（Eino 可能重复 response_start）", "Same segment of main channel streaming output (Eino may repeat response_start)"),
    ("AI 思考流式Output：progressId -> Map(streamId -> { itemId, buffer })", "AI thinking streaming output: progressId -> Map(streamId -> { itemId, buffer })"),
    ("Eino 子代理回复流式：progressId -> Map(streamId -> { itemId, buffer })", "Eino sub-agent reply streaming: progressId -> Map(streamId -> { itemId, buffer })"),
    ("工具Output流式增量：progressId::toolCallId -> { itemId, buffer }", "Tool output streaming increments: progressId::toolCallId -> { itemId, buffer }"),
    # ── medium phrases ────────────────────────────────────────────────────────
    ("轻量会话 LRU 缓存。", "Lightweight session LRU cache."),
    ("@ 提及相关状态", "@ mention related state"),
    ("External MCP名称列表", "External MCP name list"),
    ("IME输入法状态跟踪", "IME input method state tracking"),
    ("输入框草稿Save相关", "Input draft save related"),
    ("500ms防抖延迟", "500ms debounce delay"),
    ("Chat文件Upload相关（后端会拼接路径与内容发给大Model，前端不再重复发文件列表）", "Chat file upload related (backend concatenates path and content to send to model, frontend no longer resends file list)"),
    ("Chat模式：eino_single = Eino ADK single-agent（/api/eino-agent/stream）；deep / plan_execute / supervisor = Eino 多代理", "Chat mode: eino_single = Eino ADK single-agent (/api/eino-agent/stream); deep / plan_execute / supervisor = Eino multi-agent"),
    ("人机协同（HITL）会话级配置", "Human-in-the-loop (HITL) session-level configuration"),
    ("Save输入框草稿到localStorage（防抖版本）", "Save input draft to localStorage (debounced version)"),
    ("清除之前的定时器", "Clear the previous timer"),
    ("设置新的定时器", "Set a new timer"),
    ("Save输入框草稿到localStorage", "Save input draft to localStorage"),
    ("不要把占位提示本身当作草稿Save", "Do not save the placeholder prompt itself as a draft"),
    ("如果内容为空或等于占位提示，清除Save的草稿", "If content is empty or equals the placeholder, clear the saved draft"),
    ("localStorage可能已满或不可用，静默失败", "localStorage may be full or unavailable, fail silently"),
    ("Save草稿失败:", "Failed to save draft:"),
    ("从localStorageResume输入框草稿", "Restore input draft from localStorage"),
    ("Run态与审批态需要及时自Refresh", "Running and approval states need timely auto-refresh"),
    ("监控页展示：内部 mcp::tool → Model侧 mcp__tool", "Monitor page display: internal mcp::tool → model-side mcp__tool"),
    ("Filter/API：mcp__tool → 内部 mcp::tool（与库存一致）", "Filter/API: mcp__tool → internal mcp::tool (consistent with inventory)"),
    ("Current界面语言对应的 BCP 47 标签（与时间格式化一致）", "BCP 47 tag corresponding to current UI language (consistent with time formatting)"),
    ("toLocaleTimeString 选项：中文用 24 小时制，避免仍显示 AM/PM", "toLocaleTimeString options: use 24-hour format for Chinese, to avoid still showing AM/PM"),
    ("中英双向映射，切换语言后能跟上", "Bidirectional CN/EN mapping, tracks language switches"),
    ("将 Eino 内部 agent 名本地化为进度条标题用语", "Localize Eino internal agent names for progress bar title display"),
    ("Filter/API", "Filter/API"),
    ("本轮要求Execute证据，但没有完成工具执行记录", "This round requires execution evidence, but no completed tool execution was recorded"),
    ("Workflows正在Waiting for manual confirmation", "Workflows is awaiting manual confirmation"),
    ("任务状态仍为 ", "Task status still: "),
    ("另 ", "and "),
    (" 项", " more items"),
    ("待Complete工具：`", "Pending completion tools: `"),
    ("待Complete检查：", "Pending completion checks: "),
    ("中文", "Chinese"),
    ("英文（与 en-US.json 一致，避免后端/缓存已是英文时无法随语言切换）", "English (consistent with en-US.json, to allow language switching when backend/cache is already in English)"),
    ("正在调用工具: ", "Calling tool: "),

    # ── task status strings ───────────────────────────────────────────────────
    ("任务失败", "Task failed"),
    ("任务Cancelled", "Task cancelled"),
    ("渗透TestComplete", "Penetration test complete"),
    ("任务状态仍为", "Task status still"),

    # ── settings.js ──────────────────────────────────────────────────────────
    ("请输入", "Please enter"),
    ("密码格式错误，请重新输入", "Invalid password format, please re-enter"),
    ("两次密码不一致", "Passwords do not match"),
    ("旧密码不正确", "Old password is incorrect"),
    ("密码修改成功", "Password changed successfully"),
    ("密码修改失败", "Failed to change password"),
    ("请输入旧密码", "Please enter old password"),
    ("请输入新密码", "Please enter new password"),
    ("请再次输入新密码", "Please confirm new password"),
    ("Settings已Save", "Settings saved"),
    ("SettingsSave失败", "Failed to save settings"),
    ("请配置 API Key", "Please configure API Key"),
    ("加载Settings失败", "Failed to load settings"),
    ("Settings加载完成", "Settings loaded"),
    ("正在加载Settings...", "Loading settings..."),
    ("正在Save Settings...", "Saving settings..."),

    # ── knowledge.js ──────────────────────────────────────────────────────────
    ("知识库", "Knowledge base"),
    ("知识库名称", "Knowledge base name"),
    ("知识库描述", "Knowledge base description"),
    ("已选择", "Selected"),
    ("个文件", "files"),
    ("Upload成功", "Upload successful"),
    ("Upload失败", "Upload failed"),
    ("正在Upload...", "Uploading..."),
    ("确认Delete该知识库吗？", "Confirm delete this knowledge base?"),
    ("Delete成功", "Deleted successfully"),
    ("Delete失败", "Failed to delete"),
    ("Create成功", "Created successfully"),
    ("Create失败", "Failed to create"),
    ("加载失败", "Failed to load"),
    ("操作成功", "Operation successful"),
    ("操作失败", "Operation failed"),
    ("请输入知识库名称", "Please enter knowledge base name"),
    ("文件大小不能超过", "File size cannot exceed"),
    ("不支持的文件类型", "Unsupported file type"),
    ("知识库已存在", "Knowledge base already exists"),
    ("加载知识库列表失败", "Failed to load knowledge base list"),
    ("知识库加载完成", "Knowledge base loaded"),
    ("嵌入Model：", "Embedding model:"),
    ("嵌入维度：", "Embedding dimensions:"),
    ("文档数量：", "Document count:"),
    ("最大查询结果：", "Max query results:"),
    ("相似度阈值：", "Similarity threshold:"),
    ("正在建立嵌入索引，请稍候...", "Building embedding index, please wait..."),
    ("索引构建成功", "Index built successfully"),
    ("索引构建失败", "Failed to build index"),
    ("索引构建中...", "Building index..."),
    ("索引状态：", "Index status:"),
    ("文档已完成嵌入", "Document embedding complete"),
    ("文档嵌入中", "Document embedding in progress"),
    ("文档嵌入失败", "Document embedding failed"),
    ("文档嵌入等待中", "Document embedding queued"),
    ("暂无文档", "No documents"),
    ("暂无知识库", "No knowledge bases"),
    ("请先选择知识库", "Please select a knowledge base first"),

    # ── roles.js ──────────────────────────────────────────────────────────────
    ("角色", "Role"),
    ("角色名称", "Role name"),
    ("角色描述", "Role description"),
    ("角色列表", "Role list"),
    ("Create角色", "Create role"),
    ("Edit角色", "Edit role"),
    ("Delete角色", "Delete role"),
    ("确认Delete该角色吗？", "Confirm delete this role?"),
    ("Save角色", "Save role"),
    ("请输入角色名称", "Please enter role name"),
    ("角色名称不能为空", "Role name cannot be empty"),
    ("角色Create成功", "Role created successfully"),
    ("角色Create失败", "Failed to create role"),
    ("角色Update成功", "Role updated successfully"),
    ("角色Update失败", "Failed to update role"),
    ("角色Delete成功", "Role deleted successfully"),
    ("角色Delete失败", "Failed to delete role"),
    ("加载角色列表失败", "Failed to load role list"),
    ("角色加载完成", "Roles loaded"),
    ("暂无角色", "No roles"),
    ("工具选择", "Tool selection"),
    ("选择工具", "Select tools"),
    ("已选工具", "Selected tools"),
    ("可用工具", "Available tools"),
    ("工具搜索", "Tool search"),
    ("请输入工具名称", "Please enter tool name"),
    ("确认选择", "Confirm selection"),
    ("取消", "Cancel"),
    ("全选", "Select all"),
    ("清空", "Clear"),
    ("工具描述", "Tool description"),
    ("工具类型", "Tool type"),

    # ── dashboard.js ──────────────────────────────────────────────────────────
    ("仪表板", "Dashboard"),
    ("概览", "Overview"),
    ("任务统计", "Task statistics"),
    ("最近任务", "Recent tasks"),
    ("系统状态", "System status"),
    ("连接状态", "Connection status"),
    ("已连接", "Connected"),
    ("未连接", "Disconnected"),
    ("正在连接...", "Connecting..."),
    ("连接失败", "Connection failed"),
    ("正在加载...", "Loading..."),
    ("加载更多", "Load more"),
    ("没有更多数据", "No more data"),
    ("暂无数据", "No data"),
    ("暂无任务", "No tasks"),
    ("刷新", "Refresh"),
    ("全部", "All"),
    ("进行中", "In progress"),
    ("已完成", "Completed"),
    ("已失败", "Failed"),
    ("已取消", "Cancelled"),
    ("等待中", "Waiting"),
    ("已暂停", "Paused"),
    ("统计数据加载失败", "Failed to load statistics"),
    ("最近任务加载失败", "Failed to load recent tasks"),
    ("任务详情加载失败", "Failed to load task details"),
    ("项目", "Project"),
    ("创建时间", "Created time"),
    ("更新时间", "Updated time"),
    ("任务ID", "Task ID"),
    ("任务名称", "Task name"),
    ("任务状态", "Task status"),
    ("任务类型", "Task type"),
    ("执行时间", "Execution time"),
    ("开始时间", "Start time"),
    ("结束时间", "End time"),
    ("任务输出", "Task output"),
    ("任务错误", "Task error"),
    ("分钟前", "minutes ago"),
    ("小时前", "hours ago"),
    ("天前", "days ago"),
    ("刚刚", "Just now"),
    ("今天", "Today"),
    ("昨天", "Yesterday"),

    # ── tasks.js ──────────────────────────────────────────────────────────────
    ("创建任务", "Create task"),
    ("任务列表", "Task list"),
    ("任务详情", "Task details"),
    ("Stop任务", "Stop task"),
    ("Delete任务", "Delete task"),
    ("确认Stop该任务吗？", "Confirm stop this task?"),
    ("确认Delete该任务吗？", "Confirm delete this task?"),
    ("任务Stop成功", "Task stopped successfully"),
    ("任务Stop失败", "Failed to stop task"),
    ("任务Delete成功", "Task deleted successfully"),
    ("任务Delete失败", "Failed to delete task"),
    ("加载任务列表失败", "Failed to load task list"),
    ("任务加载完成", "Tasks loaded"),
    ("搜索任务", "Search tasks"),
    ("请输入任务名称或ID", "Please enter task name or ID"),
    ("按状态Filter", "Filter by status"),
    ("按类型Filter", "Filter by type"),
    ("按时间Filter", "Filter by time"),

    # ── webshell.js ───────────────────────────────────────────────────────────
    ("WebShell", "WebShell"),
    ("终端", "Terminal"),
    ("命令", "Command"),
    ("执行命令", "Execute command"),
    ("命令执行失败", "Command execution failed"),
    ("连接已断开", "Connection disconnected"),
    ("正在重新连接...", "Reconnecting..."),
    ("重新连接失败", "Reconnection failed"),
    ("已连接到", "Connected to"),
    ("正在连接到", "Connecting to"),
    ("认证失败", "Authentication failed"),
    ("权限不足", "Insufficient permissions"),
    ("会话超时", "Session timeout"),
    ("会话已过期", "Session expired"),
    ("请重新Sign in", "Please sign in again"),
    ("上传文件", "Upload file"),
    ("下载文件", "Download file"),
    ("文件上传成功", "File uploaded successfully"),
    ("文件上传失败", "Failed to upload file"),
    ("文件下载成功", "File downloaded successfully"),
    ("文件下载失败", "Failed to download file"),
    ("选择文件", "Select file"),
    ("拖拽文件到此处或点击Upload", "Drag file here or click to upload"),
    ("当前路径", "Current path"),
    ("切换路径", "Switch path"),
    ("路径不存在", "Path does not exist"),

    # ── assets.js / info-collect.js ───────────────────────────────────────────
    ("资产", "Asset"),
    ("资产列表", "Asset list"),
    ("资产详情", "Asset details"),
    ("资产名称", "Asset name"),
    ("资产类型", "Asset type"),
    ("资产地址", "Asset address"),
    ("资产端口", "Asset port"),
    ("资产状态", "Asset status"),
    ("添加资产", "Add asset"),
    ("Edit资产", "Edit asset"),
    ("Delete资产", "Delete asset"),
    ("确认Delete该资产吗？", "Confirm delete this asset?"),
    ("资产添加成功", "Asset added successfully"),
    ("资产添加失败", "Failed to add asset"),
    ("资产Update成功", "Asset updated successfully"),
    ("资产Update失败", "Failed to update asset"),
    ("资产Delete成功", "Asset deleted successfully"),
    ("资产Delete失败", "Failed to delete asset"),
    ("加载资产列表失败", "Failed to load asset list"),
    ("资产加载完成", "Assets loaded"),
    ("暂无资产", "No assets"),
    ("信息收集", "Information collection"),
    ("收集目标", "Collection target"),
    ("收集类型", "Collection type"),
    ("收集结果", "Collection result"),
    ("开始收集", "Start collection"),
    ("停止收集", "Stop collection"),
    ("收集中...", "Collecting..."),
    ("收集完成", "Collection complete"),
    ("收集失败", "Collection failed"),

    # ── projects.js ───────────────────────────────────────────────────────────
    ("项目列表", "Project list"),
    ("项目详情", "Project details"),
    ("项目名称", "Project name"),
    ("项目描述", "Project description"),
    ("Create项目", "Create project"),
    ("Edit项目", "Edit project"),
    ("Delete项目", "Delete project"),
    ("确认Delete该项目吗？", "Confirm delete this project?"),
    ("Save项目", "Save project"),
    ("请输入项目名称", "Please enter project name"),
    ("项目名称不能为空", "Project name cannot be empty"),
    ("项目Create成功", "Project created successfully"),
    ("项目Create失败", "Failed to create project"),
    ("项目Update成功", "Project updated successfully"),
    ("项目Update失败", "Failed to update project"),
    ("项目Delete成功", "Project deleted successfully"),
    ("项目Delete失败", "Failed to delete project"),
    ("加载项目列表失败", "Failed to load project list"),
    ("项目加载完成", "Projects loaded"),
    ("暂无项目", "No projects"),

    # ── vulnerability.js ──────────────────────────────────────────────────────
    ("漏洞", "Vulnerability"),
    ("漏洞列表", "Vulnerability list"),
    ("漏洞详情", "Vulnerability details"),
    ("漏洞名称", "Vulnerability name"),
    ("漏洞类型", "Vulnerability type"),
    ("漏洞等级", "Vulnerability severity"),
    ("漏洞描述", "Vulnerability description"),
    ("漏洞状态", "Vulnerability status"),
    ("漏洞编号", "Vulnerability ID"),
    ("发现时间", "Discovery time"),
    ("修复时间", "Fix time"),
    ("未修复", "Unpatched"),
    ("已修复", "Patched"),
    ("修复中", "Patching"),
    ("忽略", "Ignored"),
    ("确认Delete该漏洞吗？", "Confirm delete this vulnerability?"),
    ("漏洞Delete成功", "Vulnerability deleted successfully"),
    ("漏洞Delete失败", "Failed to delete vulnerability"),
    ("加载漏洞列表失败", "Failed to load vulnerability list"),
    ("漏洞加载完成", "Vulnerabilities loaded"),
    ("暂无漏洞", "No vulnerabilities"),
    ("严重", "Critical"),
    ("高危", "High"),
    ("中危", "Medium"),
    ("低危", "Low"),
    ("信息", "Info"),

    # ── router.js ─────────────────────────────────────────────────────────────
    ("路由", "Router"),
    ("页面未找到", "Page not found"),
    ("无权限访问", "Access denied"),
    ("正在加载页面...", "Loading page..."),
    ("路由加载失败", "Failed to load route"),
    ("导航失败", "Navigation failed"),
    ("请先Sign in", "Please sign in first"),
    ("登录状态已过期，请重新Sign in", "Login session expired, please sign in again"),

    # ── auth.js ───────────────────────────────────────────────────────────────
    ("用户名", "Username"),
    ("密码", "Password"),
    ("Sign in", "Sign in"),
    ("退出Sign in", "Sign out"),
    ("Sign in成功", "Sign in successful"),
    ("Sign in失败", "Sign in failed"),
    ("用户名或密码错误", "Invalid username or password"),
    ("Sign in中...", "Signing in..."),
    ("请输入用户名", "Please enter username"),
    ("请输入密码", "Please enter password"),
    ("用户名不能为空", "Username cannot be empty"),
    ("密码不能为空", "Password cannot be empty"),
    ("网络错误，请稍后重试", "Network error, please try again later"),
    ("服务器错误", "Server error"),
    ("Token已过期", "Token expired"),
    ("需要重新认证", "Re-authentication required"),

    # ── rbac.js ───────────────────────────────────────────────────────────────
    ("用户管理", "User management"),
    ("用户列表", "User list"),
    ("Create用户", "Create user"),
    ("Edit用户", "Edit user"),
    ("Delete用户", "Delete user"),
    ("确认Delete该用户吗？", "Confirm delete this user?"),
    ("用户Create成功", "User created successfully"),
    ("用户Create失败", "Failed to create user"),
    ("用户Update成功", "User updated successfully"),
    ("用户Update失败", "Failed to update user"),
    ("用户Delete成功", "User deleted successfully"),
    ("用户Delete失败", "Failed to delete user"),
    ("加载用户列表失败", "Failed to load user list"),
    ("角色管理", "Role management"),
    ("分配角色", "Assign role"),
    ("角色分配成功", "Role assigned successfully"),
    ("角色分配失败", "Failed to assign role"),
    ("权限管理", "Permission management"),
    ("权限列表", "Permission list"),

    # ── api-docs.js ───────────────────────────────────────────────────────────
    ("接口文档", "API documentation"),
    ("接口列表", "API list"),
    ("接口详情", "API details"),
    ("请求参数", "Request parameters"),
    ("响应参数", "Response parameters"),
    ("请求示例", "Request example"),
    ("响应示例", "Response example"),
    ("接口地址", "API endpoint"),
    ("请求方式", "Request method"),
    ("参数名称", "Parameter name"),
    ("参数类型", "Parameter type"),
    ("参数描述", "Parameter description"),
    ("是否必填", "Required"),
    ("默认值", "Default value"),
    ("示例值", "Example value"),
    ("接口状态", "API status"),
    ("已废弃", "Deprecated"),
    ("稳定", "Stable"),
    ("测试中", "Testing"),

    # ── workflows.js ──────────────────────────────────────────────────────────
    ("工作流", "Workflow"),
    ("工作流列表", "Workflow list"),
    ("工作流详情", "Workflow details"),
    ("工作流名称", "Workflow name"),
    ("工作流描述", "Workflow description"),
    ("Create工作流", "Create workflow"),
    ("Edit工作流", "Edit workflow"),
    ("Delete工作流", "Delete workflow"),
    ("确认Delete该工作流吗？", "Confirm delete this workflow?"),
    ("工作流Create成功", "Workflow created successfully"),
    ("工作流Create失败", "Failed to create workflow"),
    ("工作流Update成功", "Workflow updated successfully"),
    ("工作流Update失败", "Failed to update workflow"),
    ("工作流Delete成功", "Workflow deleted successfully"),
    ("工作流Delete失败", "Failed to delete workflow"),
    ("加载工作流列表失败", "Failed to load workflow list"),
    ("工作流加载完成", "Workflows loaded"),
    ("暂无工作流", "No workflows"),
    ("运行工作流", "Run workflow"),
    ("Stop工作流", "Stop workflow"),
    ("工作流运行成功", "Workflow started successfully"),
    ("工作流运行失败", "Failed to start workflow"),
    ("工作流已Stop", "Workflow stopped"),
    ("工作流StopFailed", "Failed to stop workflow"),

    # ── skills.js ─────────────────────────────────────────────────────────────
    ("技能", "Skill"),
    ("技能列表", "Skill list"),
    ("技能详情", "Skill details"),
    ("技能名称", "Skill name"),
    ("技能描述", "Skill description"),
    ("Create技能", "Create skill"),
    ("Edit技能", "Edit skill"),
    ("Delete技能", "Delete skill"),
    ("确认Delete该技能吗？", "Confirm delete this skill?"),
    ("技能Create成功", "Skill created successfully"),
    ("技能Create失败", "Failed to create skill"),
    ("技能Update成功", "Skill updated successfully"),
    ("技能Update失败", "Failed to update skill"),
    ("技能Delete成功", "Skill deleted successfully"),
    ("技能Delete失败", "Failed to delete skill"),
    ("加载技能列表失败", "Failed to load skill list"),
    ("技能加载完成", "Skills loaded"),
    ("暂无技能", "No skills"),

    # ── c2.js ─────────────────────────────────────────────────────────────────
    ("命令与控制", "Command and control"),
    ("C2服务器", "C2 server"),
    ("C2客户端", "C2 client"),
    ("C2状态", "C2 status"),
    ("C2连接", "C2 connection"),
    ("生成载荷", "Generate payload"),
    ("载荷类型", "Payload type"),
    ("目标平台", "Target platform"),
    ("混淆选项", "Obfuscation options"),
    ("加密选项", "Encryption options"),
    ("监听器", "Listener"),
    ("添加监听器", "Add listener"),
    ("监听地址", "Listen address"),
    ("监听端口", "Listen port"),
    ("监听协议", "Listen protocol"),
    ("监听器状态", "Listener status"),
    ("监听器Start成功", "Listener started successfully"),
    ("监听器Start失败", "Failed to start listener"),
    ("监听器Stop成功", "Listener stopped successfully"),
    ("监听器Stop失败", "Failed to stop listener"),
    ("会话列表", "Session list"),
    ("会话详情", "Session details"),
    ("主机名", "Hostname"),
    ("操作系统", "Operating system"),
    ("进程名", "Process name"),
    ("进程ID", "Process ID"),
    ("用户权限", "User privilege"),
    ("心跳时间", "Heartbeat time"),
    ("上线时间", "Online time"),
    ("上线", "Online"),
    ("下线", "Offline"),
    ("交互", "Interact"),
    ("发送命令", "Send command"),
    ("命令历史", "Command history"),
    ("命令Output", "Command output"),
    ("执行成功", "Execution successful"),
    ("执行失败", "Execution failed"),

    # ── storage.js ────────────────────────────────────────────────────────────
    ("存储", "Storage"),
    ("文件管理", "File management"),
    ("文件列表", "File list"),
    ("文件名称", "File name"),
    ("文件大小", "File size"),
    ("文件类型", "File type"),
    ("修改时间", "Modified time"),
    ("Upload文件", "Upload file"),
    ("Download文件", "Download file"),
    ("Delete文件", "Delete file"),
    ("Create文件夹", "Create folder"),
    ("文件夹名称", "Folder name"),
    ("确认Delete该文件吗？", "Confirm delete this file?"),
    ("文件Delete成功", "File deleted successfully"),
    ("文件Delete失败", "Failed to delete file"),
    ("文件Upload成功", "File uploaded successfully"),
    ("文件Upload失败", "Failed to upload file"),
    ("文件Download成功", "File downloaded successfully"),
    ("文件Download失败", "Failed to download file"),
    ("加载文件列表失败", "Failed to load file list"),
    ("暂无文件", "No files"),
    ("正在Upload文件...", "Uploading file..."),
    ("正在Download文件...", "Downloading file..."),

    # ── terminal.js ───────────────────────────────────────────────────────────
    ("终端模拟器", "Terminal emulator"),
    ("连接终端", "Connect terminal"),
    ("断开终端", "Disconnect terminal"),
    ("终端连接成功", "Terminal connected successfully"),
    ("终端连接失败", "Failed to connect terminal"),
    ("终端已断开", "Terminal disconnected"),
    ("清空终端", "Clear terminal"),
    ("全屏", "Fullscreen"),
    ("退出全屏", "Exit fullscreen"),
    ("字体大小", "Font size"),
    ("增大字体", "Increase font size"),
    ("减小字体", "Decrease font size"),
    ("终端设置", "Terminal settings"),

    # ── audit.js ──────────────────────────────────────────────────────────────
    ("审计日志", "Audit logs"),
    ("审计记录", "Audit record"),
    ("操作用户", "Operator"),
    ("操作类型", "Operation type"),
    ("操作时间", "Operation time"),
    ("操作结果", "Operation result"),
    ("操作详情", "Operation details"),
    ("操作对象", "Operation target"),
    ("操作成功", "Operation successful"),
    ("审计日志加载失败", "Failed to load audit logs"),
    ("审计日志加载完成", "Audit logs loaded"),
    ("暂无审计记录", "No audit records"),
    ("导出审计日志", "Export audit logs"),
    ("导出成功", "Export successful"),
    ("导出失败", "Export failed"),

    # ── wechat-robot.js ───────────────────────────────────────────────────────
    ("微信机器人", "WeChat bot"),
    ("机器人状态", "Bot status"),
    ("机器人已启动", "Bot started"),
    ("机器人已Stop", "Bot stopped"),
    ("启动机器人", "Start bot"),
    ("Stop机器人", "Stop bot"),
    ("机器人Start成功", "Bot started successfully"),
    ("机器人Start失败", "Failed to start bot"),
    ("机器人Stop成功", "Bot stopped successfully"),
    ("机器人Stop失败", "Failed to stop bot"),
    ("扫码Sign in", "Scan QR code to sign in"),
    ("等待扫码", "Waiting for QR code scan"),
    ("扫码成功", "QR code scanned"),
    ("Sign in成功", "Signed in successfully"),
    ("二维码已过期", "QR code expired"),
    ("重新获取二维码", "Re-fetch QR code"),
    ("消息列表", "Message list"),
    ("发送消息", "Send message"),
    ("消息发送成功", "Message sent successfully"),
    ("消息发送失败", "Failed to send message"),
    ("群组列表", "Group list"),
    ("群组名称", "Group name"),
    ("群成员", "Group members"),

    # ── hitl.js ───────────────────────────────────────────────────────────────
    ("人机协同", "Human-in-the-loop"),
    ("审批任务", "Approval task"),
    ("审批列表", "Approval list"),
    ("审批详情", "Approval details"),
    ("待审批", "Pending approval"),
    ("已审批", "Approved"),
    ("已拒绝", "Rejected"),
    ("审批通过", "Approve"),
    ("拒绝审批", "Reject"),
    ("审批意见", "Approval comment"),
    ("请输入审批意见", "Please enter approval comment"),
    ("审批成功", "Approval successful"),
    ("审批失败", "Approval failed"),
    ("拒绝成功", "Rejection successful"),
    ("拒绝失败", "Rejection failed"),
    ("加载审批列表失败", "Failed to load approval list"),
    ("审批列表加载完成", "Approval list loaded"),
    ("暂无审批任务", "No approval tasks"),

    # ── fact-graph.js ─────────────────────────────────────────────────────────
    ("知识图谱", "Knowledge graph"),
    ("图谱节点", "Graph node"),
    ("图谱边", "Graph edge"),
    ("节点类型", "Node type"),
    ("边类型", "Edge type"),
    ("节点标签", "Node label"),
    ("边标签", "Edge label"),
    ("图谱加载失败", "Failed to load graph"),
    ("图谱加载完成", "Graph loaded"),
    ("暂无数据", "No data"),
    ("放大", "Zoom in"),
    ("缩小", "Zoom out"),
    ("重置视图", "Reset view"),
    ("Export图谱", "Export graph"),
    ("筛选节点", "Filter nodes"),
    ("筛选边", "Filter edges"),

    # ── notifications.js ──────────────────────────────────────────────────────
    ("通知", "Notification"),
    ("通知列表", "Notification list"),
    ("未读通知", "Unread notifications"),
    ("已读通知", "Read notifications"),
    ("标为已读", "Mark as read"),
    ("全部标为已读", "Mark all as read"),
    ("Delete通知", "Delete notification"),
    ("清空通知", "Clear notifications"),
    ("确认清空所有通知吗？", "Confirm clear all notifications?"),
    ("通知加载失败", "Failed to load notifications"),
    ("暂无通知", "No notifications"),
    ("系统通知", "System notification"),
    ("任务通知", "Task notification"),
    ("警告通知", "Warning notification"),
    ("错误通知", "Error notification"),

    # ── sanitize-markdown.js ──────────────────────────────────────────────────
    ("不支持的语言", "Unsupported language"),
    ("代码块", "Code block"),
    ("内联代码", "Inline code"),
    ("代码高亮失败", "Failed to highlight code"),
    ("Markdown渲染失败", "Failed to render Markdown"),
    ("内容过长，已截断显示", "Content too long, truncated for display"),

    # ── builtin-tools.js ──────────────────────────────────────────────────────
    ("内置工具", "Built-in tools"),
    ("工具名称", "Tool name"),
    ("工具描述", "Tool description"),
    ("工具参数", "Tool parameters"),
    ("工具输出", "Tool output"),
    ("工具Execute", "Tool execution"),
    ("工具ExecuteSuccess", "Tool execution successful"),
    ("工具ExecuteFailed", "Tool execution failed"),
    ("工具ExecuteCancel", "Tool execution cancelled"),
    ("工具ExecuteTimeout", "Tool execution timeout"),
    ("工具ExecuteRunning", "Tool execution running"),
    ("工具执行中...", "Tool executing..."),
    ("工具列表", "Tool list"),
    ("工具加载失败", "Failed to load tools"),
    ("工具加载完成", "Tools loaded"),
    ("暂无工具", "No tools"),

    # ── rbac-guards.js ────────────────────────────────────────────────────────
    ("权限验证失败", "Permission verification failed"),
    ("无访问权限", "No access permission"),
    ("角色验证", "Role verification"),
    ("权限检查", "Permission check"),
    ("需要认证", "Authentication required"),
    ("Token无效", "Invalid token"),
    ("Token已过期", "Token expired"),
    ("请重新登录", "Please log in again"),

    # ── workflow-package-client.js ────────────────────────────────────────────
    ("工作流包", "Workflow package"),
    ("包列表", "Package list"),
    ("安装包", "Install package"),
    ("卸载包", "Uninstall package"),
    ("包详情", "Package details"),
    ("包名称", "Package name"),
    ("包版本", "Package version"),
    ("包描述", "Package description"),
    ("包作者", "Package author"),
    ("安装成功", "Installation successful"),
    ("安装失败", "Installation failed"),
    ("卸载成功", "Uninstallation successful"),
    ("卸载失败", "Uninstallation failed"),
    ("加载包列表失败", "Failed to load package list"),
    ("暂无包", "No packages"),

    # ── modal.js ──────────────────────────────────────────────────────────────
    ("确认", "Confirm"),
    ("确定", "OK"),
    ("关闭", "Close"),
    ("保存", "Save"),
    ("提交", "Submit"),
    ("重置", "Reset"),
    ("返回", "Back"),
    ("下一步", "Next"),
    ("上一步", "Previous"),
    ("完成", "Done"),
    ("警告", "Warning"),
    ("错误", "Error"),
    ("成功", "Success"),
    ("提示", "Hint"),

    # ── theme.js ──────────────────────────────────────────────────────────────
    ("主题", "Theme"),
    ("浅色主题", "Light theme"),
    ("深色主题", "Dark theme"),
    ("跟随系统", "Follow system"),
    ("主题切换成功", "Theme switched successfully"),

    # ── chat-plan-progress.js ─────────────────────────────────────────────────
    ("规划中...", "Planning..."),
    ("执行中...", "Executing..."),
    ("完成", "Done"),
    ("等待确认", "Awaiting confirmation"),
    ("步骤", "Step"),
    ("共", "Total"),
    ("个步骤", "steps"),

    # ── agents.js ─────────────────────────────────────────────────────────────
    ("代理", "Agent"),
    ("代理列表", "Agent list"),
    ("代理名称", "Agent name"),
    ("代理描述", "Agent description"),
    ("代理状态", "Agent status"),

    # ── chat-scroll.js ────────────────────────────────────────────────────────
    ("滚动到底部", "Scroll to bottom"),
    ("新消息", "New message"),
    ("条新消息", "new messages"),
    ("向下滚动", "Scroll down"),
    ("返回最新", "Back to latest"),
    ("自动滚动", "Auto scroll"),
    ("手动滚动", "Manual scroll"),
    ("滚动位置", "Scroll position"),

    # ── chat-files.js ─────────────────────────────────────────────────────────
    ("选择文件", "Select file"),
    ("文件已选择", "File selected"),
    ("清除文件", "Clear file"),
    ("文件类型不支持", "File type not supported"),
    ("文件过大", "File too large"),
    ("文件数量超限", "Too many files"),
    ("文件Upload中...", "Uploading file..."),
    ("文件已Upload", "File uploaded"),
    ("文件Upload失败", "Failed to upload file"),
    ("点击或拖拽上传", "Click or drag to upload"),

    # ── i18n.js ───────────────────────────────────────────────────────────────
    ("语言", "Language"),
    ("语言切换", "Language switch"),
    ("语言加载失败", "Failed to load language"),
    ("语言已切换", "Language switched"),
    ("当前语言", "Current language"),
    ("简体中文", "Simplified Chinese"),
    ("英文", "English"),
    ("俄文", "Russian"),

    # ── generic / common patterns ─────────────────────────────────────────────
    ("加载中...", "Loading..."),
    ("请稍候...", "Please wait..."),
    ("请稍后重试", "Please try again later"),
    ("网络错误", "Network error"),
    ("请求失败", "Request failed"),
    ("服务器内部错误", "Internal server error"),
    ("参数错误", "Parameter error"),
    ("未知错误", "Unknown error"),
    ("操作超时", "Operation timeout"),
    ("数据加载失败", "Failed to load data"),
    ("数据Save失败", "Failed to save data"),
    ("数据Delete失败", "Failed to delete data"),
    ("数据Update失败", "Failed to update data"),
    ("数据Create失败", "Failed to create data"),
    ("暂无更多数据", "No more data"),
    ("共", "Total"),
    ("条记录", "records"),
    ("搜索...", "Search..."),
    ("请输入搜索内容", "Please enter search content"),
    ("搜索结果", "Search results"),
    ("搜索失败", "Search failed"),
    ("暂无搜索结果", "No search results"),
    ("更多", "More"),
    ("展开", "Expand"),
    ("收起", "Collapse"),
    ("Edit", "Edit"),
    ("删除", "Delete"),
    ("查看", "View"),
    ("详情", "Details"),
    ("导出", "Export"),
    ("导入", "Import"),
    ("下载", "Download"),
    ("上传", "Upload"),
    ("Copy", "Copy"),
    ("粘贴", "Paste"),
    ("剪切", "Cut"),
    ("全选", "Select all"),
    ("反选", "Invert selection"),
    ("是", "Yes"),
    ("否", "No"),
    ("确认", "Confirm"),
    ("取消", "Cancel"),
    ("关闭", "Close"),
    ("保存", "Save"),
    ("提交", "Submit"),
    ("重置", "Reset"),
    ("刷新", "Refresh"),
    ("返回", "Back"),
    ("上一页", "Previous page"),
    ("下一页", "Next page"),
    ("首页", "First page"),
    ("末页", "Last page"),
    ("共", "Total"),
    ("页", "page"),
    ("跳转到", "Go to"),
    ("条/页", "per page"),
    ("已选择", "Selected"),
    ("项", "items"),
    ("启用", "Enable"),
    ("禁用", "Disable"),
    ("正常", "Normal"),
    ("异常", "Abnormal"),
    ("在线", "Online"),
    ("离线", "Offline"),
    ("活跃", "Active"),
    ("非活跃", "Inactive"),
    ("最新", "Latest"),
    ("最早", "Earliest"),
    ("升序", "Ascending"),
    ("降序", "Descending"),
    ("筛选", "Filter"),
    ("重置筛选", "Reset filter"),
    ("应用筛选", "Apply filter"),
    ("高级筛选", "Advanced filter"),
    ("时间范围", "Time range"),
    ("开始日期", "Start date"),
    ("结束日期", "End date"),
    ("今天", "Today"),
    ("昨天", "Yesterday"),
    ("最近7天", "Last 7 days"),
    ("最近30天", "Last 30 days"),
    ("自定义", "Custom"),
    ("请选择", "Please select"),
    ("全部", "All"),
    ("未知", "Unknown"),
    ("无", "None"),
    ("空", "Empty"),
    ("其他", "Other"),
    ("帮助", "Help"),
    ("关于", "About"),
    ("版本", "Version"),
    ("更新", "Update"),
    ("检查更新", "Check for updates"),
    ("已是最新版本", "Already latest version"),
    ("发现新版本", "New version found"),
    ("立即更新", "Update now"),
    ("稍后更新", "Update later"),
    ("Settings", "Settings"),
    ("个人Settings", "Personal settings"),
    ("系统Settings", "System settings"),
    ("安全Settings", "Security settings"),
    ("通知Settings", "Notification settings"),
    ("主题Settings", "Theme settings"),
    ("语言Settings", "Language settings"),
    ("时区Settings", "Timezone settings"),

    # progress messages
    ("最后一次迭代：正在Generate总结和下一步计划...", "Last iteration: generating summary and next steps..."),
    ("总结GenerateComplete", "Summary generation complete"),
    ("正在Generate最终回复...", "Generating final reply..."),
    ("达到最大迭代次数，正在Generate总结...", "Reached max iterations, generating summary..."),
    ("正在分析您的请求...", "Analyzing your request..."),
    ("开始分析请求并制定Test策略", "Starting request analysis and test strategy planning"),
    ("正在启动 Eino DeepAgent...", "Starting Eino DeepAgent..."),
    ("正在启动 Eino 多代理...", "Starting Eino multi-agent..."),

    # extra mixed strings from monitor.js / chat.js
    ("Current会话已有任务正在Execute，请先等待Complete或Stop任务。", "Current session already has a running task. Please wait for completion or stop the task first."),
    ("：", ": "),
    ("正在Save…", "Saving…"),
    ("已自动Save", "Auto-saved"),
    ("已获取 {count} Model", "Retrieved {count} models"),
    ("请先在Settings中配置 API Key", "Please configure API Key in Settings first"),
    ("获取Model失败", "Failed to fetch models"),
    ("应用Model失败", "Failed to apply model"),
    ("开启", "On"),
    ("自动", "Auto"),
    ("系统", "System"),
    ("选择 AI channel、Model与推理设置", "Select AI channel, model and reasoning settings"),
    ("Agent 审查", "Agent review"),
    ("Stop任务", "Stop task"),
    ("发送", "Send"),
    ("正在Save…", "Saving…"),
]

# Sort by length descending so longer phrases match first
PHRASES.sort(key=lambda x: len(x[0]), reverse=True)

def replace_chinese_in_string_literal(content):
    """Replace Chinese phrases inside quoted string literals."""
    for cn, en in PHRASES:
        if cn in content:
            content = content.replace(cn, en)
    return content

def translate_i18n_fallbacks(line, i18n):
    """Replace Chinese fallback strings in i18nTranslate / chatTranslate calls."""
    # Pattern: translate('some.key', 'Chinese text')  or  translate("key", "Chinese")
    pattern = re.compile(
        r"""((?:chatTranslate|i18nTranslate|translate)\s*\(\s*['"][^'"]+['"]\s*,\s*)(['"])(.*?)(\2)""",
        re.DOTALL
    )
    def replacer(m):
        prefix = m.group(1)
        quote = m.group(2)
        fallback = m.group(3)
        if not has_chinese(fallback):
            return m.group(0)
        # Try i18n lookup
        key_match = re.search(r"""['"]([^'"]+)['"]\s*,\s*['"]""", prefix)
        if key_match:
            key = key_match.group(1)
            en = i18n_lookup(key)
            if en and not has_chinese(en):
                return prefix + quote + en + quote
        # Fall back to phrase table
        translated = replace_chinese_in_string_literal(fallback)
        return prefix + quote + translated + quote
    return pattern.sub(replacer, line)

def translate_line(line):
    """Translate all Chinese in a single line."""
    if not has_chinese(line):
        return line

    stripped = line.lstrip()

    # 1. Full-line comment -> translate everything
    if stripped.startswith('//') or stripped.startswith('*') or stripped.startswith('/*'):
        result = replace_chinese_in_string_literal(line)
        return result

    # 2. i18n fallback calls
    if re.search(r'(?:chatTranslate|i18nTranslate|translate)\s*\(', line):
        line = translate_i18n_fallbacks(line, i18n)

    # 3. Any remaining Chinese phrases in string literals
    line = replace_chinese_in_string_literal(line)

    return line

def translate_file(path):
    text = open(path, encoding='utf-8').read()
    before = count_chinese(text)
    if before == 0:
        return 0, 0

    lines = text.split('\n')
    new_lines = [translate_line(l) for l in lines]
    new_text = '\n'.join(new_lines)
    after = count_chinese(new_text)
    if new_text != text:
        open(path, 'w', encoding='utf-8').write(new_text)
    return before, after

# ── run ───────────────────────────────────────────────────────────────────────
total_before = 0
total_after = 0
files_changed = 0

for root, dirs, files in os.walk('web/static/js'):
    dirs[:] = [d for d in dirs if d != 'vendor']
    for f in sorted(files):
        if not f.endswith('.js'):
            continue
        path = os.path.join(root, f)
        before, after = translate_file(path)
        if before > 0:
            total_before += before
            total_after += after
            files_changed += 1
            reduced = before - after
            print(f'  {before:5d} -> {after:4d}  (-{reduced:4d})  {path}')

print(f'\nTotal: {total_before} -> {total_after} ({total_before - total_after} translated, {total_after} remaining)')
print(f'Files processed: {files_changed}')
