with open('web/static/js/c2.js', 'r', encoding='utf-8') as fp:
    content = fp.read()

phrases = [
    ("// C2 模块前端逻辑 - 完整实现", "// C2 module frontend logic - full implementation"),
    ("// 支持: xterm 终端、文件管理、监听器/会话/任务/事件/Payload/Profile 管理", "// Supports: xterm terminal, file manager, listener/session/task/event/payload/profile management"),
    ("// C2 模块命名空间", "// C2 module namespace"),
    ("// xterm 相关", "// xterm related"),
    ("// 文件管理", "// File manager"),
    ("// 任务轮询", "// Task polling"),
    ("// API 基础路径", "// API base path"),
    ("/** 下拉展示用项目名：优先可读名称，绝不回退成 UUID */", "/** Project name for dropdown display: prefers human-readable name, never falls back to UUID */"),
    ("return c2t('batchManageModal.unknownProject') || '未知项目';", "return c2t('batchManageModal.unknownProject') || 'Unknown Project';"),
    ("let html = `<option value=\"\">${escapeHtml(c2t('assets.unboundProject') || '暂不绑定')}</option>`;",
     "let html = `<option value=\"\">${escapeHtml(c2t('assets.unboundProject') || 'Do not bind')}</option>`;"),
    ("/** 确保当前选中项目有可读名称（孤儿 / 未进缓存的 ID） */", "/** Ensure currently selected project has a readable name (orphan / uncached ID) */"),
    ("title=\"${escapeAttr(c2t('assets.project') || '所属项目')}\"", "title=\"${escapeAttr(c2t('assets.project') || 'Project')}\""),
    ("// 工具函数", "// Utility functions"),
    ("/** 任务列表操作按钮（查看/取消/删除）— 事件委托 */", "/** Task list action buttons (view/cancel/delete) - event delegation */"),
    ("/** C2 动态内容操作按钮 — 避免把用户可控值拼入 inline onclick */", "/** C2 dynamic content action buttons - avoids interpolating user-controlled values into inline onclick */"),
    ("/** 监听器表单：Malleable Profile 下拉选项 HTML（value / 文本已转义） */", "/** Listener form: Malleable Profile dropdown options HTML (value / text escaped) */"),
    ("/** 监听器卡片展示用 Profile 名称（依赖 C2.profiles，由 loadListeners 一并拉取） */", "/** Listener card display Profile name (depends on C2.profiles, fetched via loadListeners) */"),
    ("/** 避免 i18n 插值把日期里的「/」转成 &#x2F;，与 formatTime 拼接后整体转义 */", "/** Avoid i18n interpolation turning '/' into &#x2F;, escaped together after concatenating with formatTime */"),
    ("// 页面初始化", "// Page initialization"),
    ("// 监听器管理", "// Listener management"),
    ("/** 拉取 Profile 列表（监听器表单用）；失败时置空列表不阻断弹窗 */", "/** Fetch Profile list (used by listener form); resets to empty list on failure without blocking modal */"),
    ("escapeHtml(c2t('assets.project') || '所属项目')", "escapeHtml(c2t('assets.project') || 'Project')"),
    ("/** 非 HTTP/HTTPS Beacon 时隐藏 Profile 行；tcp_reverse 时显示经典 shell 开关 */", "/** Hide Profile row when not HTTP/HTTPS Beacon; show classic shell toggle when tcp_reverse */"),
    ("showToast(c2t('projects.projectBound') || '已绑定项目', 'success');", "showToast(c2t('projects.projectBound') || 'Project bound', 'success');"),
    ("showToast(err && err.message ? err.message : '绑定项目失败', 'error');", "showToast(err && err.message ? err.message : 'Failed to bind project', 'error');"),
    ("// 会话管理", "// Session management"),
    ("// xterm 终端", "// xterm terminal"),
    ("/** 将相对浏览路径解析为 implant 工作目录下的绝对路径 */", "/** Resolves relative browse path to absolute path under implant working directory */"),
    ("/** 将 /d:/path/file 转为 Windows 远程路径 d:\\path\\file */", "/** Convert /d:/path/file to Windows remote path d:\\path\\file */"),
    ("// Beacon 结构化输出：type\\tmode\\tsize\\tname", "// Beacon structured output: type\\tmode\\tsize\\tname"),
    ("// 原生 ls -l 输出", "// Native ls -l output"),
    ("// 兼容误传：仅传路径时（如旧版 loadFileList('..')）自动纠正", "// Compatibility fallback: auto-correct when only path is passed (e.g. legacy loadFileList('..'))"),
    ("// 编译 Beacon：HTTP/HTTPS/TCP(CSB1) 均走二进制/结构化协议，支持 upload", "// Compile Beacon: HTTP/HTTPS/TCP(CSB1) use binary/structured protocols and support upload"),
    ("// 经典 TCP 反弹 Shell（bash/nc，metadata.transport=tcp_reverse）", "// Classic TCP reverse Shell (bash/nc, metadata.transport=tcp_reverse)"),
    ("// 任务管理", "// Task management"),
    ("// Payload 生成", "// Payload generation"),
    ("// 事件审计", "// Event audit"),
    ("// Profile 管理", "// Profile management"),
    ("// 模态框", "// Modal dialogs"),
    ("// 暴露到全局", "// Export to global scope"),
    ("// 页面切换监听", "// Page switch listener"),
    ("// DOM 加载完成后初始化", "// Initialize after DOMContentLoaded"),
]

count = 0
for orig, repl in phrases:
    if orig in content:
        content = content.replace(orig, repl)
        count += 1
    else:
        print(f"Missed in c2.js: {orig}")

with open('web/static/js/c2.js', 'w', encoding='utf-8') as fp:
    fp.write(content)

print(f"c2.js: replaced {count} phrases successfully!")
