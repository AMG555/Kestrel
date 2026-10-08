#!/usr/bin/env python3
"""
Aggressive line-by-line translation for remaining Chinese in JS comment lines.
Extract full comment lines and replace Chinese segments comprehensively.
"""
import re, os

# All known Chinese -> English word/phrase mappings
# This is a CHARACTER/WORD level mapping for all remaining patterns
WORD_MAP = {
    # Characters that appear as standalone Chinese in mixed lines
    "聊天页": "chat page",
    "聊天": "chat",
    "尚在": "still in",
    "初始化": "initialisation",
    "的": "",  # possessive particle - often just noise
    "请求": "request",
    "失去": "lose",
    "页面所有权": "page ownership",
    "后端": "backend",
    "任务": "task",
    "仍会继续执行": "will continue to execute",
    "仍会": "still will",
    "继续": "continue",
    "执行": "execute",
    "这里只": "this only",
    "只": "only",
    "中止浏览器前台流": "cancels the browser foreground stream",
    "避免": "to avoid",
    "首个": "first",
    "事件": "event",
    "在用户": "after the user",
    "用户": "user",
    "已经": "has already",
    "切到": "switched to",
    "其他": "another",
    "后再次": "then re-",
    "抢占": "claim",
    "当前": "current",
    "会话": "session",
    "轻量": "lightweight",
    "缓存": "cache",
    "只用作": "is only used as",
    "失败时": "on failure",
    "降级数据": "fallback data",
    "不能先于": "must not come before",
    "服务端响应": "server response",
    "直接渲染": "directly render",
    "运行中": "running",
    "进程详情": "process details",
    "会持续写入": "keeps writing",
    "直接": "directly",
    "旧快照": "old snapshot",
    "暂时": "temporarily",
    "回退到": "revert to",
    "旧轮次": "old turn",
    "等": "wait for",
    "接管后": "takes over",
    "又突然": "then suddenly",
    "跳到": "jump to",
    "最新": "latest",
    "输入框草稿保存": "input box draft save",
    "对话文件上传": "conversation file upload",
    "相关": "related",
    "后端会拼接路径与内容发给大模型": "backend concatenates path+content to send to LLM",
    "前端不再重复发文件列表": "frontend no longer resends file list",
    "对话模式": "conversation mode",
    "单代理": "single-agent",
    "多代理": "multi-agent",
    "人机协同": "HITL",
    "会话级": "session-level",
    "配置": "configuration",
    "字符串拆成数组": "split string into array",
    "逗号或换行分隔": "comma or newline separated",
    "保存/发请求前": "before saving/sending",
    "去掉全局白名单工具": "remove global whitelist tools",
    "避免会话里重复存": "to avoid duplicating",
    "已有项": "items already present",
    "侧栏已改为自动保存": "sidebar now auto-saves",
    "同步更新输入框快捷摘要": "sync-update the input box shortcut summary",
    "自动写入本地": "auto-written locally",
    "合并展示": "merged for display",
    "尽量同步服务端": "synced to server when possible",
    # monitor.js
    "主对话 POST 流": "main chat POST stream",
    "主聊天": "main chat",
    "仍在读取时": "still reading",
    "禁止再挂": "do not attach",
    "补流": "supplementary stream",
    "否则同一事件": "otherwise the same event",
    "会画两遍": "will be rendered twice",
    "与 HITL 是否开启无关": "unrelated to whether HITL is enabled",
    "由 chat.js": "set by chat.js",
    "在读到 body 后": "after reading the body",
    "设置": "sets",
    "在 finally 中清除": "cleared in finally",
    "新会话": "new conversation",
    "尚未到达前": "before arriving",
    "可能仍为 null": "may still be null",
    "一律不补挂": "never attach",
    "监控页展示": "monitor page display",
    "内部": "internal",
    "模型侧": "model-side",
    "筛选/API": "filter/API",
    "与库存一致": "consistent with inventory",
    "当前界面语言": "current UI language",
    "对应的 BCP 47 标签": "corresponding BCP 47 tag",
    "与时间格式化一致": "consistent with time formatting",
    "选项": "options",
    "中文用 24 小时制": "Chinese uses 24-hour format",
    "避免仍显示 AM/PM": "to avoid showing AM/PM",
    "将后端下发的进度文案": "translate progress messages from backend",
    "转为当前语言的翻译": "to current language",
    "中英双向映射": "bidirectional zh/en mapping",
    "切换语言后能跟上": "follows language switches",
    "将 Eino 内部 agent 名": "localise Eino internal agent names",
    "本地化为进度条标题用语": "to progress bar title wording",
    "从 Plan-Execute 模型返回的单层 JSON 中": "from the flat JSON returned by the Plan-Execute model",
    "取面向用户的字符串": "extract the user-facing string",
    "replanner 常用 response": "replanner commonly uses 'response'",
    "少数模型在 JSON 字符串里仍留下字面量": "a few models leave literal",
    "在已解出正文后再转成换行": "convert to newlines after extracting body text",
    "不误伤 Windows 盘符等极少命中": "rarely matches Windows drive letters etc.",
    "在线正文": "streaming body",
    "转为列表": "converted to list",
    "解包为纯文本": "unpacked to plain text",
    "同样解包": "same unpack",
    "流式片段非法 JSON 时保持原文": "keep original text when stream fragment is invalid JSON",
    "在线条目": "streaming entry",
    "主通道流式阶段标题": "main channel streaming phase title",
    "替代一律": "replaces the generic",
    "规划中": "planning",
    "主通道有模型流式输出": "main channel has model streaming output",
    "不显": "don't show",
    "模型偶发复述工具 stdout 时": "when model occasionally echoes tool stdout",
    "旧文案易被误认为工具结果标题": "the old label was mistaken for a tool result title",
    "未捕获助手正文占位文案": "uncaptured assistant body placeholder",
    "终态 response 不应覆盖已有流式 buffer": "final response should not override existing streaming buffer",
    "Run态与审批态需要及时自刷新": "running and approval states need timely auto-refresh",
    "将后端下发的进度文案转为": "translate backend progress messages to",
    "当前语言的翻译": "current language",
    "切换语言后能跟上": "follows language switches",
    # common HTML span embedded
    "工作流审批": "Workflow approval",
    # dashboard
    "工程基础设施": "engineering infrastructure",
    "集中保存运行时状态": "centralises runtime state",
    "每次 refreshDashboard 入口": "each refreshDashboard entry",
    "abort 上一个 controller": "aborts the previous controller",
    "把 signal 传给所有 apiFetch": "passes signal to all apiFetch calls",
    "避免快速连点": "to avoid rapid clicks",
    "自动轮询触发 race condition": "auto-polling triggering race conditions",
    "自动轮询": "auto-polling",
    "每 60 秒拉一次": "fetches every 60 s",
    "页面切走": "when page is navigated away",
    "tab 隐藏时自动暂停": "auto-pauses when tab is hidden",
    "再切回时立即补一次刷新": "immediately re-fetches on return",
    "基于 lastUpdatedAt 避免无效请求": "based on lastUpdatedAt to avoid unnecessary requests",
    "过期检测": "stale detection",
    "记录时间戳": "records timestamp",
    "每 30 秒检查": "checks every 30 s",
    "超过 5 分钟未刷新": "not refreshed in 5 min",
    "在「上次更新」徽章上加": "adds the",
    ".is-stale 类": ".is-stale class to the 'last updated' badge",
    "变灰 + 显示 ⚠️": "greys out + shows ⚠️",
    "当前正在进行的 fetch 的 AbortController": "AbortController for the current in-flight fetch",
    "自动轮询的 setInterval id": "setInterval id for auto-polling",
    "过期检查的 setInterval id": "setInterval id for stale checks",
    "上次成功刷新的时间戳（ms）": "timestamp (ms) of the last successful refresh",
    "接入概览 Tab：c2 | webshell": "access overview tab: c2 | webshell",
    "在后续渲染逻辑中也被引用": "is also referenced in later render logic",
    "必须在 loading 分支外声明": "must be declared outside the loading branch",
    # more common
    "保留最后一个不完整的行": "keep the last incomplete line",
    "addFilesToChat 已提示": "addFilesToChat already notified",
    "工具本身的启用状态": "tool's own enabled status",
    "在当前角色中的启用状态": "enabled status in current role",
    "保存唯一标识符": "save unique identifier",
    "启用的工具排在前面": "enabled tools sorted first",
    # Additional remaining mixed patterns
    "打开会话设置": "open session settings",
    "折叠/展开对话列表": "collapse/expand conversation list",
    "展开/折叠最近对话": "expand/collapse recent conversations",
    "新建项目": "new project",
    "会话设置": "session settings",
    "人机协同设置只影响后续消息": "HITL settings only affect subsequent messages",
    "选择对话执行模式": "select conversation execution mode",
    "Agent 审查": "Agent review",
    "已选择 0 项": "0 selected",
    "默认通道": "default channel",
    "当前": "current",
    "正在获取模型列表": "fetching model list",
    "已获取 {count} 个": "fetched {count}",
    "请先在": "please configure",
    "中配置 API Key": "first",
    "获取": "fetch",
    "失败": "failed",
    "审批方已保存": "reviewer saved",
    "同步到服务器失败": "failed to sync to server",
    "人机协同配置已保存并同步到服务器": "HITL configuration saved and synced to server",
    "人机协同默认配置已写入 config.yaml 并生效": "HITL default configuration written to config.yaml and active",
    "免审批工具已合并进 config.yaml 并生效": "approval-exempt tools merged into config.yaml and active",
    "会话配置会自动保存": "session config will be saved automatically",
    "已保存到本浏览器": "saved to local browser",
    "Select AI channel": "Select AI channel",
    "Model与推理Settings": "model and reasoning settings",
    # span inner text
    "工作流审批": "Workflow approval",
}

def translate_line_words(line):
    """Replace Chinese word/phrase sequences in a line."""
    if not re.search(r"[\u4e00-\u9fff]", line):
        return line
    
    result = line
    # Sort by length descending to avoid partial replacements
    for zh, en in sorted(WORD_MAP.items(), key=lambda x: -len(x[0])):
        if zh and en is not None:
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
        
        lines = content.split("\n")
        new_lines = [translate_line_words(line) for line in lines]
        new_content = "\n".join(new_lines)
        
        after = len(re.findall(r"[\u4e00-\u9fff]", new_content))
        total_before += before
        total_after += after
        
        with open(path, "w", encoding="utf-8") as f:
            f.write(new_content)
        
        if after > 0:
            print(f"  {after:5d} remaining: {path}")

print(f"\nTotal: {total_before} -> {total_after} ({total_before - total_after} replaced)")
