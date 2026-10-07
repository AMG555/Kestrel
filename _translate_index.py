#!/usr/bin/env python3
"""
Replace Chinese inner text in data-i18n elements with English.
Strategy: use the i18n key to look up the English translation from en-US.json,
then replace the inner text of each element with that translation.
"""
import re, json

# Load en-US translations
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

# Pattern: any tag with data-i18n="key" that contains Chinese text as inner content
# This handles: <tag data-i18n="key">Chinese text</tag>
# and: <tag ... data-i18n="key" ...>Chinese text</tag>
def replace_i18n_inner(m):
    tag_start = m.group(1)   # everything up to >
    inner = m.group(2)       # inner text
    tag_end = m.group(3)     # closing tag
    # extract the key from tag_start
    key_match = re.search(r'data-i18n="([^"]+)"', tag_start)
    if not key_match:
        return m.group(0)
    key = key_match.group(1)
    en_text = get_en(key)
    if en_text and re.search(r'[\u4e00-\u9fff]', inner):
        return tag_start + ">" + en_text + tag_end
    return m.group(0)

# Match opening tag (containing data-i18n) followed by Chinese inner text, then closing tag
# We limit inner text to not contain < to avoid matching nested elements
pattern = re.compile(
    r'(<[^>]+data-i18n="[^"]*"[^>]*>)'  # opening tag with data-i18n
    r'([^<]*[\u4e00-\u9fff][^<]*)'       # inner text containing Chinese
    r'(</[^>]+>)',                        # closing tag
    re.DOTALL
)
content = pattern.sub(replace_i18n_inner, content)

# Handle data-i18n on attributes (data-title="...Chinese...", title="...Chinese...", placeholder="...Chinese...")
# Also handle aria-label, placeholder, title with Chinese
# These are fallback attributes — replace common ones

ATTR_REPLACEMENTS = [
    # HTML comments
    ('<!-- 主侧边栏 -->', '<!-- Main sidebar -->'),
    ('<!-- C2 侧栏入口（带子菜单） -->', '<!-- C2 sidebar entry (with sub-menu) -->'),
    ('<!-- 内容区域 -->', '<!-- Content area -->'),
    ('<!-- 仪表盘页面 -->', '<!-- Dashboard page -->'),
    ('<!-- 关键提醒条（仅当存在严重风险时渲染，默认 hidden）；右侧 × 可在 session 内忽略 -->', '<!-- Critical alert bar (only rendered when serious risk exists, hidden by default); right-side × dismissible within the session -->'),
    ('<!-- 第一行：核心 KPI（关键指标置顶 + 副标徽章承载次级信息） -->', '<!-- First row: core KPIs (key metrics at top + sub-label badges carry secondary info) -->'),
    ('<!-- 两列主内容区 -->', '<!-- Two-column main content area -->'),
    ('<!-- 风险概览卡：填充 donut 左侧留白；提供「结论性」洞察（风险等级/加权分/待处理计数/最新时间），', '<!-- Risk overview card: fills whitespace left of donut; provides conclusive insights (risk level/weighted score/pending count/latest time),'),
    ('          与右侧 legend 的「明细」形成互补，避免和下方「最近漏洞」列表重复 -->', '          complementing the right legend details, avoiding duplication with the recent-vulnerabilities list below -->'),
    ('<!-- 处置状态 + 修复进度（利用 by_status 数据，避免下半部分留白） -->', '<!-- Disposition status + fix progress (using by_status data, avoiding whitespace in the lower half) -->'),
    ('<!-- 接入概览：C2 / WebShell Tab 切换（样式同「最近漏洞 / 近期事实」） -->', '<!-- Access overview: C2 / WebShell tab switching (same style as Recent vulnerabilities / Recent facts) -->'),
    ('<!-- 推荐操作：基于当前数据状态智能生成（如「修复 4 个待处理严重漏洞」「审批 2 个 HITL」），', '<!-- Recommended actions: intelligently generated based on current data state (e.g. Fix 4 pending critical vulnerabilities, Approve 2 HITLs),'),
    ('         比纯静态导航更有意义；当没有任何推荐时整个 section 隐藏 -->', '         more meaningful than static navigation; the entire section is hidden when there are no recommendations -->'),
    ('<!-- 最近事件：拉 /api/notifications/summary 取最新 3 条；空时整个隐藏 -->', '<!-- Recent events: fetches latest 3 from /api/notifications/summary; hidden entirely when empty -->'),
    ('<!-- "开始你的安全之旅" CTA：默认显示；当用户已经有数据（任务/漏洞/调用）后，由 JS 隐藏避免冗余 -->', '<!-- "Start your security journey" CTA: shown by default; hidden by JS once the user has data (tasks/vulnerabilities/calls) to avoid redundancy -->'),
    ('<!-- 对话页面 -->', '<!-- Chat page -->'),
    ('<!-- 历史对话侧边栏（可折叠，与主导航侧边栏类似） -->', '<!-- Conversation history sidebar (collapsible, similar to the main nav sidebar) -->'),
    ('<!-- 头部一行：折叠与「新任务」并排；任务底层复用会话数据 -->', '<!-- Header row: collapse toggle and New task side by side; tasks reuse session data internally -->'),
    ('<!-- 项目搜索 -->', '<!-- Project search -->'),
    ('<!-- 项目文件夹：复用对话底部项目选择器的真实项目数据 -->', '<!-- Project folders: reuses real project data from the conversation bottom project selector -->'),
    ('<!-- 按项目筛选对话 -->', '<!-- Filter conversations by project -->'),
    ('<!-- 最近对话 -->', '<!-- Recent conversations -->'),
    ('<!-- 对话界面 -->', '<!-- Chat interface -->'),
    ('<!-- 会话顶部栏（只在有会话选中时显示） -->', '<!-- Session top bar (only shown when a session is selected) -->'),
    ('<!-- External MCP 服务器健康度：N 运行 / N 异常；只有配置过 External MCP 才显示 -->', '<!-- External MCP server health: N running / N error; only shown when External MCP has been configured -->'),
    ('<!-- MCP Monitor页面 -->', '<!-- MCP Monitor page -->'),
    ('<!-- MCP Management页面 -->', '<!-- MCP Management page -->'),
    ('<!-- MCP工具配置 -->', '<!-- MCP tool configuration -->'),
    ('<!-- 外部MCP配置 -->', '<!-- External MCP configuration -->'),
    ('<!-- Knowledge管理页面 -->', '<!-- Knowledge management page -->'),
    ('<!-- KnowledgeRetrieval History页面 -->', '<!-- Knowledge retrieval history page -->'),
    ('<!-- 资产概览页面 -->', '<!-- Asset overview page -->'),
    ('<!-- 资产库页面 -->', '<!-- Asset library page -->'),
    ('<!-- Info收集页面 -->', '<!-- Information collection page -->'),
    ('<!-- 字段显示/隐藏面板 -->', '<!-- Field show/hide panel -->'),
    ('<!-- Projects页面 -->', '<!-- Projects page -->'),
    ('<!-- Roles选择下拉面板 -->', '<!-- Roles selection dropdown panel -->'),
    # data-title attributes with Chinese
    ('data-title="仪表盘"', 'data-title="Dashboard"'),
    ('data-title="对话"', 'data-title="Chat"'),
    ('data-title="安全防护"', 'data-title="Security"'),
    ('data-title="项目管理"', 'data-title="Projects"'),
    ('data-title="资产管理"', 'data-title="Assets"'),
    ('data-title="漏洞管理"', 'data-title="Vulnerabilities"'),
    ('data-title="任务管理"', 'data-title="Tasks"'),
    ('data-title="工作流"', 'data-title="Workflows"'),
    ('data-title="WebShell管理"', 'data-title="WebShell"'),
    ('data-title="文件管理"', 'data-title="Files"'),
    ('data-title="知识"', 'data-title="Knowledge"'),
    ('data-title="技能"', 'data-title="Skills"'),
    ('data-title="智能体"', 'data-title="Agents"'),
    ('data-title="角色"', 'data-title="Roles"'),
    ('data-title="平台权限"', 'data-title="Platform RBAC"'),
    ('data-title="系统设置"', 'data-title="Settings"'),
    # title attributes
    ('title="当前版本"', 'title="Current version"'),
    # aria-label
    ('aria-label="风险概览"', 'aria-label="Risk overview"'),
    ('aria-label="对话轮次导航"', 'aria-label="Conversation turn navigation"'),
    ('aria-label="当前会话上下文"', 'aria-label="Current session context"'),
    ('aria-label="待审批Actions"', 'aria-label="Pending approval actions"'),
    ('aria-label="已选文件列表"', 'aria-label="Selected files list"'),
    ('aria-label="工具提及候选"', 'aria-label="Tool mention suggestions"'),
    ('aria-label="AI 通道、模型与推理设置"', 'aria-label="AI channel, model and reasoning settings"'),
    ('aria-label="选项列表"', 'aria-label="Options list"'),
    # placeholder
    ('placeholder="输入登录密码"', 'placeholder="Enter login password"'),
    # span id inner text (not covered by data-i18n)
    ('>跟随系统</span>', '>Follow system</span>'),
    ('>中文</span>', '>Chinese</span>'),
    ('>单代理</span>', '>Single agent</span>'),
    ('>默认通道</span>', '>Default channel</span>'),
    ('>不指定</span>', '>Unspecified</span>'),
    ('>Agent 审查：Off</span>', '>Agent review: Off</span>'),
    # lang dropdown option
    ('>中文</div>', '>Chinese</div>'),
    # Bind robot account
    ('>绑定机器人账号</span>', '>Bind robot account</span>'),
    # Session settings hint (multi-word, not caught by i18n key)
]

for zh, en_text in ATTR_REPLACEMENTS:
    content = content.replace(zh, en_text)

remaining = re.findall(r'[\u4e00-\u9fff]', content)
print(f"Remaining Chinese chars: {len(remaining)}")
if remaining:
    for i, line in enumerate(content.splitlines(), 1):
        if re.search(r'[\u4e00-\u9fff]', line):
            print(f"  L{i}: {line.strip()[:140]}")

with open("web/templates/index.html", "w", encoding="utf-8") as f:
    f.write(content)
print("Saved index.html")
