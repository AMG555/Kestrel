#!/usr/bin/env python3
"""Second pass: fix remaining Chinese attribute values and misc inline text."""
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

# Replace data-i18n-attr="title" ... title="Chinese" with English from i18n key
# Pattern: data-i18n="key" ... title="Chinese"
def replace_title_attr(m):
    full = m.group(0)
    key_m = re.search(r'data-i18n="([^"]+)"', full)
    if not key_m:
        return full
    key = key_m.group(1)
    en_text = get_en(key)
    if en_text:
        # Replace the title="Chinese" part
        full = re.sub(r'title="[^"]*[\u4e00-\u9fff][^"]*"', f'title="{en_text}"', full)
    return full

# Apply to tags that have both data-i18n-attr="title" and a Chinese title attribute
# We process tag by tag
def process_tag(m):
    tag = m.group(0)
    if 'data-i18n-attr="title"' not in tag:
        return tag
    if not re.search(r'[\u4e00-\u9fff]', tag):
        return tag
    return replace_title_attr(re.match(r'(.*)', tag, re.DOTALL))

# Replace title attributes on tags with data-i18n
content = re.sub(
    r'<[^>]+data-i18n="[^"]*"[^>]*data-i18n-attr="title"[^>]*>',
    lambda m: replace_title_attr(m),
    content, flags=re.DOTALL
)
content = re.sub(
    r'<[^>]+data-i18n-attr="title"[^>]*data-i18n="[^"]*"[^>]*>',
    lambda m: replace_title_attr(m),
    content, flags=re.DOTALL
)

# Replace data-i18n-attr="placeholder" tags
def replace_placeholder_attr(m):
    full = m.group(0)
    key_m = re.search(r'data-i18n="([^"]+)"', full)
    if not key_m:
        return full
    key = key_m.group(1)
    en_text = get_en(key)
    if en_text:
        full = re.sub(r'placeholder="[^"]*[\u4e00-\u9fff][^"]*"', f'placeholder="{en_text}"', full)
    return full

content = re.sub(
    r'<[^>]+data-i18n="[^"]*"[^>]*data-i18n-attr="placeholder"[^>]*>',
    lambda m: replace_placeholder_attr(m),
    content, flags=re.DOTALL
)
content = re.sub(
    r'<[^>]+data-i18n-attr="placeholder"[^>]*data-i18n="[^"]*"[^>]*>',
    lambda m: replace_placeholder_attr(m),
    content, flags=re.DOTALL
)

# Replace data-i18n-attr="aria-label" tags
def replace_aria_attr(m):
    full = m.group(0)
    key_m = re.search(r'data-i18n="([^"]+)"', full)
    if not key_m:
        return full
    key = key_m.group(1)
    en_text = get_en(key)
    if en_text:
        full = re.sub(r'aria-label="[^"]*[\u4e00-\u9fff][^"]*"', f'aria-label="{en_text}"', full)
    return full

content = re.sub(
    r'<[^>]+data-i18n-attr="aria-label"[^>]*>',
    lambda m: replace_aria_attr(m),
    content, flags=re.DOTALL
)

# Now handle specific hardcoded Chinese that aren't covered by i18n keys
MANUAL = [
    # title attributes
    ('title="刷新数据"', 'title="Refresh data"'),
    ('title="忽略"', 'title="Dismiss"'),
    ('title="回到最新消息"', 'title="Back to latest message"'),
    ('title="关闭"', 'title="Close"'),
    ('title="攻击链可视化"', 'title="Attack chain visualisation"'),
    ('title="搜索"', 'title="Search"'),
    ('title="刷新"', 'title="Refresh"'),
    # aria-label attributes (not from data-i18n)
    ('aria-label="回到最新消息"', 'aria-label="Back to latest message"'),
    ('aria-label="选择项目"', 'aria-label="Select project"'),
    ('aria-label="关闭"', 'aria-label="Close"'),
    ('aria-label="待审批操作"', 'aria-label="Pending approval actions"'),
    ('aria-label="最近漏洞与近期事实"', 'aria-label="Recent vulnerabilities and recent facts"'),
    ('aria-label="C2 与 WebShell"', 'aria-label="C2 and WebShell"'),
    ('aria-label="刷新"', 'aria-label="Refresh"'),
    ('aria-label="返回"', 'aria-label="Back"'),
    # placeholder attributes (not from data-i18n)
    ('placeholder="输入测试目标或命令… (@ 可提及工具)"', 'placeholder="Enter target or command... (@ to mention a tool)"'),
    ('placeholder="搜索主机、IP、域名、标题、服务、责任人或标签..."', 'placeholder="Search by host, IP, domain, title, service, owner, or tag..."'),
    ('placeholder="搜索知识..."', 'placeholder="Search knowledge..."'),
    ('placeholder="输入工具名称..."', 'placeholder="Enter tool name..."'),
    ('placeholder="搜索工具..."', 'placeholder="Search tools..."'),
    ('placeholder="搜索项目名称或描述..."', 'placeholder="Search project name or description..."'),
    ('placeholder="搜索 key、摘要、body…"', 'placeholder="Search key, summary, body..."'),
    ('placeholder="搜索节点…"', 'placeholder="Search nodes..."'),
    ('placeholder="公网"', 'placeholder="public"'),
    # span inner texts not from data-i18n
    ('>Agent 审查：关闭</span>', '>Agent review: Off</span>'),
    ('>已选择 0 项</span>', '>0 selected</span>'),
    # HTML comments
    ('<!-- MCP状态监控页面 -->', '<!-- MCP status monitor page -->'),
    ('<!-- MCP管理页面 -->', '<!-- MCP management page -->'),
    ('<!-- 知识管理页面 -->', '<!-- Knowledge management page -->'),
    ('<!-- 知识检索历史页面 -->', '<!-- Knowledge retrieval history page -->'),
    ('<!-- 角色选择下拉面板 -->', '<!-- Role selection dropdown panel -->'),
    # misc
    ('>模型</strong>', '>Model</strong>'),
    ('>单代理</span>', '>Single agent</span>'),
    ('>默认通道</span>', '>Default channel</span>'),
    ('>不指定</span>', '>Unspecified</span>'),
    ('>端口</span>', '>Port</span>'),
    ('data-title="C2 管理"', 'data-title="C2 Management"'),
]

for zh, en_text in MANUAL:
    content = content.replace(zh, en_text)

# Replace data-i18n-attr="data-title" title attributes
def replace_data_title_attr(m):
    full = m.group(0)
    key_m = re.search(r'data-i18n="([^"]+)"', full)
    if not key_m:
        return full
    key = key_m.group(1)
    en_text = get_en(key)
    if en_text:
        full = re.sub(r'data-title="[^"]*[\u4e00-\u9fff][^"]*"', f'data-title="{en_text}"', full)
    return full

content = re.sub(
    r'<[^>]+data-i18n-attr="data-title"[^>]*>',
    lambda m: replace_data_title_attr(m),
    content, flags=re.DOTALL
)

remaining = re.findall(r'[\u4e00-\u9fff]', content)
print(f"Remaining Chinese chars: {len(remaining)}")
if remaining:
    for i, line in enumerate(content.splitlines(), 1):
        if re.search(r'[\u4e00-\u9fff]', line):
            print(f"  L{i}: {line.strip()[:140]}")

with open("web/templates/index.html", "w", encoding="utf-8") as f:
    f.write(content)
print("Saved index.html")
