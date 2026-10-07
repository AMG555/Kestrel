#!/usr/bin/env python3
"""Third pass: replace inner text of data-i18n elements and all remaining Chinese."""
import re, json

with open("web/static/i18n/en-US.json", "r", encoding="utf-8-sig") as f:
    en = json.load(f)

def get_en(key):
    parts = key.split(".")
    d = en
    for p in parts:
        if isinstance(d, dict) and p in d:
            d = d[p]
        else:
            return None
    return d if isinstance(d, str) else None

with open("web/templates/index.html", "r", encoding="utf-8") as f:
    content = f.read()

# Replace inner text of elements that have data-i18n="key" and Chinese inner text
# Handles: <tag ... data-i18n="key" ...>Chinese text</tag>
# and:     <tag ... data-i18n="key" ...>Chinese text<child/></tag>  (skip if has children)
# We only replace when the content between tags contains Chinese but no child tags (simple text nodes)
def replace_i18n_inner(m):
    full_tag = m.group(1)   # opening tag content (inside <...>)
    inner    = m.group(2)   # inner text
    close    = m.group(3)   # closing tag
    
    # Only replace plain text (no child elements)
    if re.search(r'<[a-zA-Z]', inner):
        return m.group(0)
    
    key_m = re.search(r'data-i18n="([^"]+)"', full_tag)
    if not key_m:
        return m.group(0)
    key = key_m.group(1)
    en_text = get_en(key)
    if en_text and re.search(r'[\u4e00-\u9fff]', inner):
        return f'<{full_tag}>{en_text}{close}'
    return m.group(0)

# Match opening tag + inner text + closing tag for common inline elements
content = re.sub(
    r'<((?:[a-zA-Z][a-zA-Z0-9]*)[^>]*data-i18n="[^"]*"[^>]*)>((?:(?!<[a-zA-Z]).)*?[\u4e00-\u9fff](?:(?!<[a-zA-Z]).)*?)(</[a-zA-Z][a-zA-Z0-9]*>)',
    replace_i18n_inner,
    content,
    flags=re.DOTALL
)

# Also handle option elements with data-i18n
# <option value="x" data-i18n="key">Chinese</option>
content = re.sub(
    r'(<option[^>]*data-i18n="([^"]+)"[^>]*>)([^<]*[\u4e00-\u9fff][^<]*)(</option>)',
    lambda m: (
        m.group(1) + (get_en(m.group(2)) or m.group(3)) + m.group(4)
        if get_en(m.group(2)) else m.group(0)
    ),
    content
)

# Handle div/p/h2/h3/h4/li with data-i18n and plain Chinese text
content = re.sub(
    r'(<(?:div|p|h2|h3|h4|h5|li|dt|dd|summary|label|strong|small|em|th|td|button)[^>]*data-i18n="([^"]+)"[^>]*>)([^<]*[\u4e00-\u9fff][^<]*)(</(?:div|p|h2|h3|h4|h5|li|dt|dd|summary|label|strong|small|em|th|td|button)>)',
    lambda m: (
        m.group(1) + (get_en(m.group(2)) or m.group(3)) + m.group(4)
        if get_en(m.group(2)) else m.group(0)
    ),
    content
)

# HTML comment translations (remaining ones)
comment_map = {
    '<!-- MCP状态监控页面 -->': '<!-- MCP Status Monitor page -->',
    '<!-- MCP管理页面 -->': '<!-- MCP Management page -->',
    '<!-- 知识管理页面 -->': '<!-- Knowledge Management page -->',
    '<!-- 知识检索历史页面 -->': '<!-- Knowledge Retrieval History page -->',
    '<!-- 角色选择下拉面板 -->': '<!-- Role selection dropdown panel -->',
    '<!-- 对话页面 -->': '<!-- Chat page -->',
    '<!-- 历史对话侧边栏（可折叠，与主导航侧边栏类似） -->': '<!-- History conversation sidebar (collapsible, similar to main nav sidebar) -->',
    '<!-- 头部一行：折叠与「新任务」并排；任务底层复用会话数据 -->': '<!-- Header row: collapse and "New Task" side by side; tasks reuse session data -->',
    '<!-- 项目搜索 -->': '<!-- Project search -->',
    '<!-- 项目文件夹：复用对话底部项目选择器的真实项目数据 -->': '<!-- Project folders: reuse real project data from the chat bottom project selector -->',
    '<!-- 按项目筛选对话 -->': '<!-- Filter conversations by project -->',
    '<!-- 最近对话 -->': '<!-- Recent conversations -->',
    '<!-- 对话界面 -->': '<!-- Chat interface -->',
    '<!-- 会话顶部栏（只在有会话选中时显示） -->': '<!-- Session top bar (only shown when a session is selected) -->',
    '<!-- 主侧边栏 -->': '<!-- Main sidebar -->',
    '<!-- 内容区域 -->': '<!-- Content area -->',
    '<!-- 仪表盘页面 -->': '<!-- Dashboard page -->',
    '<!-- 对话页面 -->': '<!-- Chat page -->',
    '<!-- C2 侧栏入口（带子菜单） -->': '<!-- C2 sidebar entry (with submenu) -->',
    '<!-- 关键提醒条（仅当存在严重风险时渲染，默认 hidden）；右侧 × 可在 session 内忽略 -->': '<!-- Critical alert bar (rendered only when severe risk exists, hidden by default); × on right dismisses for session -->',
    '<!-- 第一行：核心 KPI（关键指标置顶 + 副标徽章承载次级信息） -->': '<!-- Row 1: Core KPIs (key metrics at top + badge for secondary info) -->',
    '<!-- 两列主内容区 -->': '<!-- Two-column main content area -->',
    '<!-- 风险概览卡：填充 donut 左侧留白；提供「结论性」洞察（风险等级/加权分/待处理计数/最新时间），\n        与右侧 legend 的「明细」形成互补，避免和下方「最近漏洞」列表重复 -->': '<!-- Risk overview card: fills left space of donut; provides "conclusive" insights (risk level/weighted score/pending count/latest time),\n        complementing the right legend "details", avoiding duplication with the "Recent Vulnerabilities" list below -->',
    '<!-- 处置状态 + 修复进度（利用 by_status 数据，避免下半部分留白） -->': '<!-- Disposition status + fix progress (uses by_status data to avoid empty lower half) -->',
    '<!-- 推荐操作：基于当前数据状态智能生成（如「修复 4 个待处理严重漏洞」「审批 2 个 HITL」），\n        比纯静态导航更有意义；当没有任何推荐时整个 section 隐藏 -->': '<!-- Recommended actions: auto-generated based on current state (e.g. "Fix 4 critical open vulns", "Approve 2 HITL"),\n        more meaningful than pure static navigation; the whole section hides when no recommendations -->',
    '<!-- 最近事件：拉 /api/notifications/summary 取最新 3 条；空时整个隐藏 -->': '<!-- Recent events: fetches /api/notifications/summary for latest 3; hidden when empty -->',
    '<!-- "开始你的安全之旅" CTA：默认显示；当用户已经有数据（任务/漏洞/调用）后，由 JS 隐藏避免冗余 -->': '<!-- "Start your security journey" CTA: shown by default; hidden by JS when user already has data (tasks/vulns/calls) -->',
    '<!-- 资产概览页面 -->': '<!-- Asset Overview page -->',
    '<!-- 资产库页面 -->': '<!-- Asset Library page -->',
    '<!-- External MCP 服务器健康度：N 运行 / N 异常；只有配置过 External MCP 才显示 -->': '<!-- External MCP server health: N running / N error; only shown when External MCP is configured -->',
    '<!-- MCP工具配置 -->': '<!-- MCP Tool Configuration -->',
    '<!-- 外部MCP配置 -->': '<!-- External MCP Configuration -->',
    '<!-- 接入概览：C2 / WebShell Tab 切换（样式同「最近漏洞 / 近期事实」） -->': '<!-- Access overview: C2 / WebShell tab switch (same style as "Recent Vulnerabilities / Recent Facts") -->',
    'aria-label="风险概览"': 'aria-label="Risk overview"',
    'aria-label="对话轮次导航"': 'aria-label="Conversation turn navigation"',
    'aria-label="当前会话上下文"': 'aria-label="Current session context"',
    'aria-label="AI 通道、模型与推理设置"': 'aria-label="AI channel, model and reasoning settings"',
    'aria-label="选项列表"': 'aria-label="Options list"',
    'aria-label="工具提及候选"': 'aria-label="Tool mention suggestions"',
    'aria-label="已选文件列表"': 'aria-label="Selected files list"',
}

for zh, en_text in comment_map.items():
    content = content.replace(zh, en_text)

# Remaining hardcoded Chinese not tied to data-i18n keys
MANUAL = [
    # Inline text not from data-i18n
    ('>绑定机器人账号</span>', '>Bind bot account</span>'),
    ('>刷新</button>', '>Refresh</button>'),
    ('>正在加载绑定信息…</div>', '>Loading binding information…</div>'),
    ('>如需配置专用服务账号，请前往"系统设置 → 机器人设置"。</span>', '>To configure a dedicated service account, go to "System Settings → Bot Settings".</span>'),
    ('>关闭</button>', '>Close</button>'),
    ('<span id="theme-toggle-label">跟随系统</span>', '<span id="theme-toggle-label">Follow system</span>'),
    ('<span id="current-lang-label">中文</span>', '<span id="current-lang-label">English</span>'),
    (">中文</div>", ">Chinese</div>"),
    (">English</div>", ">English</div>"),
    (">Русский</div>", ">Русский</div>"),
    # remaining Chinese spans
    ('>攻击链</span>', '>Attack chain</span>'),
    ('data-i18n-attr="aria-label" aria-label="对话轮次导航"', 'data-i18n-attr="aria-label" aria-label="Conversation turn navigation"'),
]

for zh, en_text in MANUAL:
    content = content.replace(zh, en_text)

# Final check
remaining = [(i+1, line.strip()[:140]) for i, line in enumerate(content.splitlines()) 
             if re.search(r'[\u4e00-\u9fff]', line)]
print(f"Remaining Chinese chars: {sum(len(re.findall(chr(0x4e00)+'-'+chr(0x9fff), line[1])) for line in remaining)}")
print(f"Remaining lines with Chinese: {len(remaining)}")
for lineno, line in remaining[:30]:
    print(f"  L{lineno}: {line}")

with open("web/templates/index.html", "w", encoding="utf-8") as f:
    f.write(content)
print("Saved index.html")
