#!/usr/bin/env python3
"""
Safe JS translation: only translate complete quoted strings and comment content.
Never does partial word replacements that could break syntax.
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

# Flat reverse lookup: Chinese i18n value -> English value
ZH_TO_EN = {}
def flatten(d, prefix=""):
    for k, v in d.items():
        key = f"{prefix}.{k}" if prefix else k
        if isinstance(v, dict):
            flatten(v, key)
        elif isinstance(v, str) and re.search(r"[\u4e00-\u9fff]", v):
            en = get_en(key)
            if en and not re.search(r"[\u4e00-\u9fff]", en):
                if v not in ZH_TO_EN:
                    ZH_TO_EN[v] = en
flatten(en_i18n)

# Comprehensive translation dictionary for JS strings and comments
# Entries are EXACT strings that appear in JS
TRANSLATIONS = {
    # ── Exact quoted strings ──────────────────────────────────────────────
    "请根据上传的文件内容进行分析。": "Please analyze the uploaded file content.",
    "【用户补充 / 中断后继续】": "[User note / continue after interrupt]",
    "审批方已保存。": "Reviewer saved.",
    "同步到服务器失败": "Failed to sync to server",
    "人机协同配置已保存并同步到服务器。": "HITL configuration saved and synced to server.",
    "人机协同默认配置已写入 config.yaml 并生效。": "HITL default configuration written to config.yaml and active.",
    "免审批工具已合并进 config.yaml 并生效。会话配置会自动保存。": "Approval-exempt tools merged into config.yaml and active. Session config will be saved automatically.",
    "已保存到本浏览器。": "Saved to local browser.",
    "Eino 单代理": "Eino single-agent",
    "跟随默认通道": "Follow default channel",
    "默认通道": "Default channel",
    "OpenAI 协议模型": "OpenAI protocol model",
    "不指定": "Not specified",
    "当前": "Current",
    "会话推理设置已更新": "Session reasoning settings updated",
    "重新获取": "Retry",
    "正在获取模型列表…": "Fetching model list…",
    "AI 通道": "AI channel",
    "推理模式": "Reasoning mode",
    "推理强度": "Reasoning effort",
    "模型": "Model",
    "默认": "Default",
    "单代理": "Single agent",
    "Eino ADK 单代理": "Eino ADK single-agent",
    "Select AI channel、Model与推理Settings": "Select AI channel, model and reasoning settings",
    "深度代理": "Deep agent",
    "规划执行": "Plan-execute",
    "监督者": "Supervisor",
    "另 {n} 项": "and {n} more",
    "已验证": "Verified",
    "规划器": "Planner",
    "执行器": "Executor",
    "重规划": "Replan",
    "助手输出": "Assistant output",
    "规划中": "Planning",
    "最终回复检查通过": "Final reply check passed",
    "最终回复检查未通过": "Final reply check failed",
    "等待工具执行完成": "Waiting for tool execution to complete",
    "缺少完成态证据": "Missing completion evidence",
    "等待人工确认": "Waiting for manual confirmation",
    "未捕获到有效回复": "No valid reply captured",
    "缺少最终化证明": "Missing finalisation evidence",
    "仍在验证": "Still verifying",
    "检查未通过": "Check failed",
    "仍有工具执行未结束": "Tool execution still in progress",
    "未捕获到助手文本输出": "No assistant text output captured",
    "未捕获到有效最终文本": "No valid final text captured",
    "正在调用AI模型...": "Calling AI model...",
    "**仍在验证，暂不生成最终结论**": "**Still verifying, not generating final conclusion yet**",
    "状态：": "Status: ",
    "候选输出已移入过程详情，避免误判为最终结论。": "Candidate output moved to process details to avoid being mistaken for a final conclusion.",
    "输出": "Output",
    "**Still verifying，暂不Generate最终结论**": "**Still verifying, not generating final conclusion yet**",
    "还": "still",
    # Dashboard
    "System空闲": "System idle",
    "正在Execute": "Executing",
    "高Critical度已All处置": "All high-criticality items handled",
    "None有效Complete": "No valid completions",
    "None调用": "No calls",
    "未Enable": "Not enabled",
    " 个": " ",
    # Common toast/status messages
    "暂无数据": "No data",
    "暂无记录": "No records",
    "暂无结果": "No results",
    "正在加载...": "Loading...",
    "正在加载…": "Loading…",
    "加载中...": "Loading...",
    "加载中…": "Loading…",
    "加载失败": "Load failed",
    "加载中": "Loading",
    "加载更多": "Load more",
    "保存成功": "Saved successfully",
    "保存失败": "Save failed",
    "删除成功": "Deleted successfully",
    "删除失败": "Delete failed",
    "操作成功": "Operation successful",
    "操作失败": "Operation failed",
    "请求失败": "Request failed",
    "请求超时": "Request timed out",
    "网络错误": "Network error",
    "服务器错误": "Server error",
    "未知错误": "Unknown error",
    "复制成功": "Copied",
    "复制失败": "Copy failed",
    "已复制!": "Copied!",
    "点击复制": "Click to copy",
    "导入成功": "Import successful",
    "导入失败": "Import failed",
    "导出成功": "Export successful",
    "导出失败": "Export failed",
    "创建成功": "Created successfully",
    "创建失败": "Creation failed",
    "更新成功": "Updated successfully",
    "更新失败": "Update failed",
    "添加成功": "Added successfully",
    "添加失败": "Add failed",
    "连接成功": "Connected successfully",
    "连接失败": "Connection failed",
    "获取数据失败": "Failed to fetch data",
    "获取列表失败": "Failed to fetch list",
    "获取失败": "Fetch failed",
    "索引构建成功": "Index built successfully",
    "索引构建失败": "Index build failed",
    "索引重建中": "Rebuilding index",
    "构建成功": "Build successful",
    "构建失败": "Build failed",
    "构建中": "Building",
    "同步成功": "Sync successful",
    "同步失败": "Sync failed",
    "重置成功": "Reset successful",
    "重置失败": "Reset failed",
    "清空成功": "Cleared successfully",
    "清空失败": "Clear failed",
    "上传成功": "Upload successful",
    "上传失败": "Upload failed",
    "下载失败": "Download failed",
    "绑定成功": "Bound successfully",
    "绑定失败": "Binding failed",
    "解绑成功": "Unbound successfully",
    "解绑失败": "Unbinding failed",
    "生成成功": "Generated successfully",
    "生成失败": "Generation failed",
    "连接测试成功": "Connection test successful",
    "连接测试失败": "Connection test failed",
    "配置已保存": "Configuration saved",
    "配置保存失败": "Configuration save failed",
    "配置已应用": "Configuration applied",
    "确认删除": "Confirm deletion",
    "确定": "OK",
    "取消": "Cancel",
    "确认": "Confirm",
    "关闭": "Close",
    "保存": "Save",
    "编辑": "Edit",
    "删除": "Delete",
    "新增": "Add",
    "搜索": "Search",
    "刷新": "Refresh",
    "复制": "Copy",
    "重置": "Reset",
    "清空": "Clear",
    "测试": "Test",
    "预览": "Preview",
    "详情": "Details",
    "查看": "View",
    "返回": "Back",
    "提交": "Submit",
    "下载": "Download",
    "上传": "Upload",
    "导入": "Import",
    "导出": "Export",
    "生成": "Generate",
    "运行": "Run",
    "执行": "Execute",
    "停止": "Stop",
    "暂停": "Pause",
    "恢复": "Resume",
    "完成": "Complete",
    "跳过": "Skip",
    "忽略": "Ignore",
    "通过": "Approve",
    "拒绝": "Reject",
    "拦截": "Block",
    "启用": "Enable",
    "禁用": "Disable",
    "激活": "Activate",
    "全选": "Select all",
    "取消全选": "Deselect all",
    "已选中": "Selected",
    "未选中": "Not selected",
    "全部": "All",
    "更多": "More",
    "展开": "Expand",
    "折叠": "Collapse",
    "筛选": "Filter",
    "排序": "Sort",
    # Status values
    "运行中": "Running",
    "已停止": "Stopped",
    "已暂停": "Paused",
    "已完成": "Completed",
    "已取消": "Cancelled",
    "已失败": "Failed",
    "已超时": "Timed out",
    "等待中": "Waiting",
    "排队中": "Queued",
    "执行中": "Executing",
    "已拦截": "Blocked",
    "已终止": "Terminated",
    "已拒绝": "Rejected",
    "已通过": "Approved",
    "已忽略": "Ignored",
    "待处理": "Open",
    "待审批": "Pending approval",
    "待审计": "Pending review",
    "审批中": "Under review",
    "已审批": "Reviewed",
    "已修复": "Fixed",
    "已确认": "Confirmed",
    "误报": "False positive",
    "待确认": "Tentative",
    "在线": "Online",
    "离线": "Offline",
    "活跃": "Active",
    "停用": "Inactive",
    "严重": "Critical",
    "高危": "High",
    "中危": "Medium",
    "低危": "Low",
    "未知": "Unknown",
    "未设置": "Not set",
    "未绑定": "Unbound",
    "已绑定": "Bound",
    "安全": "Safe",
    # form
    "不能为空": "Cannot be empty",
    "格式错误": "Invalid format",
    "参数错误": "Parameter error",
    "请选择": "Please select",
    "请输入": "Please enter",
    "请输入名称": "Please enter a name",
    "请输入工具名称": "Please enter a tool name",
    "请输入参数": "Please enter parameters",
    "请输入正则表达式": "Please enter a regular expression",
    "正则表达式无效": "Invalid regular expression",
    "字段不能为空": "Field cannot be empty",
    "名称不能为空": "Name cannot be empty",
    "密码不能为空": "Password cannot be empty",
    "用户名不能为空": "Username cannot be empty",
    "已经存在": "Already exists",
    "密码不匹配": "Passwords do not match",
    # pagination/nav
    "上一页": "Previous",
    "下一页": "Next",
    "第一页": "First page",
    "最后一页": "Last page",
    "没有更多数据": "No more data",
    "到底了": "End of list",
    "正在加载更多": "Loading more",
    # Auth
    "登录": "Sign in",
    "退出登录": "Sign out",
    "用户名": "Username",
    "密码": "Password",
    "登录成功": "Signed in",
    "登录失败": "Sign in failed",
    "注销成功": "Signed out",
    "修改密码": "Change password",
    "旧密码": "Current password",
    "新密码": "New password",
    "确认密码": "Confirm password",
    "密码修改成功": "Password changed",
    "密码修改失败": "Password change failed",
    "用户名或密码错误": "Invalid username or password",
    "密码强度": "Password strength",
    "密码不匹配": "Passwords do not match",
    "密码太短": "Password too short",
    "密码太简单": "Password too weak",
    "密码已修改成功": "Password changed successfully",
    "密码修改失败，请检查旧密码": "Password change failed, please check your current password",
    "使用平台账号登录，默认管理员为 admin。": "Sign in with your platform account. Default admin is admin.",
    # Knowledge
    "构建索引": "Build index",
    "重建索引": "Rebuild index",
    "全量重建": "Full rebuild",
    "增量构建": "Incremental build",
    "知识库": "Knowledge base",
    "知识项": "Knowledge item",
    "知识检索": "Knowledge retrieval",
    "检索历史": "Retrieval history",
    "知识管理": "Knowledge management",
    "知识详情": "Knowledge details",
    # Projects / facts
    "新建项目": "New project",
    "创建项目": "Create project",
    "选择项目": "Select project",
    "无项目": "No project",
    "绑定项目": "Bind project",
    "解除项目绑定": "Unbind project",
    "事实黑板": "Fact board",
    "攻击链": "Attack chain",
    "攻击链可视化": "Attack chain visualisation",
    "置信度": "Confidence",
    "已废弃": "Deprecated",
    "攻击链模板": "Attack chain template",
    "环境模板": "Environment template",
    # MCP / tools
    "全部规则验证": "Validate all rules",
    "规则验证通过": "Rule validation passed",
    "规则验证失败": "Rule validation failed",
    "没有匹配的规则": "No matching rules",
    "规则名称": "Rule name",
    "添加规则": "Add rule",
    "删除规则": "Delete rule",
    "拦截规则": "Block rules",
    "调用拦截": "Call interception",
    "试运行": "Dry run",
    "已启用": "Enabled",
    "已停用": "Disabled",
    "外部 MCP": "External MCP",
    "外部MCP": "External MCP",
    "添加外部MCP": "Add external MCP",
    "MCP状态监控": "MCP status monitor",
    "MCP管理": "MCP management",
    "最新执行记录": "Latest execution records",
    "内置工具": "Built-in tools",
    "孤儿任务": "Orphaned task",
    # HITL
    "待审计": "Pending",
    "审计策略": "Audit strategy",
    "工具白名单": "Tool whitelist",
    "审计日志": "Audit log",
    "审批方": "Reviewer",
    "人工审批": "Manual approval",
    "审计 Agent": "Audit Agent",
    "审批模式": "Approval mode",
    "审查编辑": "Review & edit",
    "拒绝原因": "Rejection reason",
    "等待审批": "Awaiting approval",
    "审批超时": "Approval timeout",
    "免审批": "Approval-exempt",
    "白名单工具（免审批，逗号分隔）": "Whitelist tools (approval-exempt, comma-separated)",
    # Settings
    "系统设置": "System Settings",
    "通道配置": "Channel configuration",
    "AI通道配置": "AI channel configuration",
    "视觉分析": "Vision analysis",
    "应用配置": "Apply configuration",
    "保存配置": "Save configuration",
    "恢复默认": "Restore defaults",
    "测试连接": "Test connection",
    "机器人设置": "Bot settings",
    "存储清理": "Storage cleanup",
    "绑定码": "Binding code",
    "生成绑定码": "Generate binding code",
    "一次性绑定码": "One-time binding code",
    "已绑定平台账号": "Bound platform accounts",
    "绑定机器人账号": "Bind bot account",
    "生成中": "Generating",
    "等待生成": "Waiting for generation",
    "一次性安全码": "One-time security code",
    "复制命令": "Copy command",
    "请在机器人中发送绑定命令": "Please send the binding command in the bot",
    "绑定码仅保存哈希、只能使用一次，新码会使旧码立即失效": "Binding code is stored as a hash, can only be used once; a new code immediately invalidates the old one",
    "正在加载绑定信息…": "Loading binding information…",
    # Theme
    "跟随系统": "Follow system",
    "暗色": "Dark",
    "亮色": "Light",
    "切换主题": "Switch theme",
    # Sanitize markdown
    "这段内容包含不安全的 HTML，已被过滤": "This content contains unsafe HTML and has been filtered",
    "包含不安全内容": "Contains unsafe content",
    "已过滤不安全内容": "Unsafe content filtered",
    # Router
    "仪表盘": "Dashboard",
    "对话": "Chat",
    "安全防护": "Security",
    "人机协同审批": "HITL Approval",
    "安全作业": "Operations",
    "项目管理": "Projects",
    "资产管理": "Assets",
    "资产概览": "Asset overview",
    "资产库": "Asset library",
    "信息收集": "Info collection",
    "漏洞管理": "Vulnerabilities",
    "任务管理": "Tasks",
    "工作流": "Workflows",
    "WebShell管理": "WebShell",
    "C2管理": "C2",
    "文件管理": "Files",
    "能力中心": "Capabilities",
    "MCP状态监控": "MCP Monitor",
    "MCP管理": "MCP",
    "知识检索历史": "Retrieval History",
    "技能状态": "Skills Monitor",
    "技能管理": "Skills",
    "智能体管理": "Agents",
    "角色管理": "Roles",
    "平台管理": "Administration",
    "平台权限": "RBAC",
    "系统设置": "Settings",
    "工作台": "Workbench",
    # Fact graph
    "缩小": "Zoom out",
    "放大": "Zoom in",
    "适应屏幕": "Fit to screen",
    "重置视图": "Reset view",
    "事实节点": "Fact node",
    "关联关系": "Relationship",
    "无事实节点": "No fact nodes",
    "正在构建图谱": "Building graph",
    # Notifications
    "暂无新事件": "No new events",
    "事件通知": "Event notifications",
    "查看全部": "View all",
    "标记已读": "Mark as read",
    "标记全部已读": "Mark all as read",
    "全部已读": "All read",
    # specific test/code patterns
    "测试消息": "Test message",
    "示例消息": "Example message",
    "用户消息": "User message",
    "助手消息": "Assistant message",
    "系统消息": "System message",
    "工具消息": "Tool message",
    "回到最新消息": "Back to latest message",
    "新消息": "New messages",
    "滚动到底部": "Scroll to bottom",
    "当前Conversation已有Task正在Execute，请先等待Complete或Stop task。": "The current conversation already has a running task. Please wait for it to complete or stop the task first.",
    "正在等待附件UploadComplete…": "Waiting for attachments to finish uploading…",
    "部分附件未Uploaded successfully，请Remove failed项或重新Select file后再Send。": "Some attachments failed to upload. Please remove the failed items or re-select the files before sending.",
    # workflow specific
    "移除文件": "Remove file",
    "工作流审批": "Workflow approval",
    "Workflows审批": "Workflow approval",
    # various remaining
    "未捕获到助手文本输出' : '未捕获到助手文本输出": "No assistant text output captured' : 'No assistant text output captured",
    "已获取 {count} 个Model": "Fetched {count} models",
    "请先在Settings中配置 API Key": "Please configure an API Key in Settings first",
    "获取ModelFailed": "Failed to fetch models",
    "Connection提前结束，未收到TaskComplete信号。Task可能仍在后端Execute，请View顶部RunningTask或RefreshCurrent conversation。": "Connection ended early without a task completion signal. The task may still be running in the backend. Please check the running tasks at the top or refresh the current conversation.",
    "Connection已中断（": "Connection interrupted (",
    "）。长TimeTask可能仍在后端Execute，请View顶部RunningTask或稍后刷New conversation。": "). Long-running tasks may still be running in the backend. Please check the running tasks at the top or refresh the conversation later.",
    "错误: ": "Error: ",
    "Parse事件数据Failed:": "Failed to parse event data:",
    "Save草稿Failed:": "Save draft failed:",
    "Resume草稿Failed:": "Restore draft failed:",
    "清除草稿Failed:": "Clear draft failed:",
}

def replace_in_quoted(content, translations):
    """Replace Chinese inside quoted strings only."""
    result = content
    for zh, en in sorted(translations.items(), key=lambda x: -len(x[0])):
        if re.search(r"[\u4e00-\u9fff]", zh):
            result = result.replace(zh, en)
    return result

def translate_comment_text(text, translations):
    """Translate Chinese text within a comment."""
    result = text
    for zh, en in sorted(translations.items(), key=lambda x: -len(x[0])):
        if re.search(r"[\u4e00-\u9fff]", zh):
            result = result.replace(zh, en)
    return result

def translate_file(path, translations):
    with open(path, "r", encoding="utf-8", errors="replace") as f:
        content = f.read()
    
    before = len(re.findall(r"[\u4e00-\u9fff]", content))
    if before == 0:
        return 0, 0
    
    # Apply all replacements (they target complete strings so should be safe)
    new_content = replace_in_quoted(content, translations)
    
    after = len(re.findall(r"[\u4e00-\u9fff]", new_content))
    
    with open(path, "w", encoding="utf-8") as f:
        f.write(new_content)
    
    return before, after

# Build combined translation dict (i18n reverse + TRANSLATIONS)
ALL_TRANSLATIONS = {}
ALL_TRANSLATIONS.update(ZH_TO_EN)
ALL_TRANSLATIONS.update(TRANSLATIONS)

total_before = 0
total_after = 0

for root, dirs, files in os.walk("web/static/js"):
    dirs[:] = [d for d in dirs if d != "vendor"]
    for fname in sorted(files):
        if not fname.endswith((".js", ".cjs")):
            continue
        path = os.path.join(root, fname)
        before, after = translate_file(path, ALL_TRANSLATIONS)
        total_before += before
        total_after += after
        if after > 0:
            print(f"  {after:5d} remaining: {path}")

print(f"\nTotal: {total_before} -> {total_after} ({total_before - total_after} replaced)")
