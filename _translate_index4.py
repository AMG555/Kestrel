#!/usr/bin/env python3
"""Fourth pass: handle all remaining Chinese in index.html."""
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

# ─── 1. Replace title/aria-label/placeholder Chinese attributes on long tags ─────────────────

def translate_attrs_in_tag(tag_str):
    """Given a full tag string, replace Chinese in title=, aria-label=, placeholder=, data-title= attrs."""
    # Try to get i18n key from the tag
    key_m = re.search(r'data-i18n="([^"]+)"', tag_str)
    i18n_key = key_m.group(1) if key_m else None
    en_val = get_en(i18n_key) if i18n_key else None

    def repl_attr(m, attr_name, value):
        if not re.search(r'[\u4e00-\u9fff]', value):
            return m.group(0)
        if en_val:
            return f'{attr_name}="{en_val}"'
        return m.group(0)

    # For multi-value data-i18n-attr tags, use i18n key per attribute
    # Check if data-i18n-attr specifies multiple attributes
    multi_m = re.search(r'data-i18n-attr="([^"]+)"', tag_str)
    attrs_specified = multi_m.group(1).split(',') if multi_m else []

    result = tag_str
    for attr in ['title', 'aria-label', 'placeholder', 'data-title']:
        result = re.sub(
            rf'{attr}="([^"]*[\u4e00-\u9fff][^"]*)"',
            lambda m, a=attr: (
                f'{a}="{en_val}"' if en_val else m.group(0)
            ),
            result
        )
    return result

# Apply to all tags on long lines that contain Chinese in attributes
def process_tag_attrs(m):
    return translate_attrs_in_tag(m.group(0))

content = re.sub(r'<[^>]+[\u4e00-\u9fff][^>]*>', process_tag_attrs, content)

# ─── 2. Hard-coded manual replacements ───────────────────────────────────────────────────────

MANUAL = [
    # Remaining spans without data-i18n
    ('<span id="chat-system-model-mode-value">跟随系统</span>', '<span id="chat-system-model-mode-value">Follow system</span>'),
    ('>选择全部结果</button>', '>Select all results</button>'),

    # Bulk edit asset modal
    ('<h2 id="asset-bulk-edit-title">批量编辑资产</h2>', '<h2 id="asset-bulk-edit-title">Bulk Edit Assets</h2>'),
    ('<span>状态（留空不修改）</span>', '<span>Status (leave blank to keep)</span>'),
    ('<option value="">不修改</option>', '<option value="">Keep current</option>'),
    ('<option value="active">活跃</option>', '<option value="active">Active</option>'),
    ('<option value="inactive">停用</option>', '<option value="inactive">Inactive</option>'),
    ('<span>负责人</span>', '<span>Owner</span>'),
    ('placeholder="留空不修改"', 'placeholder="Leave blank to keep"'),
    ('<span>部门</span>', '<span>Department</span>'),
    ('<span>业务系统</span>', '<span>Business system</span>'),
    ('<span>环境（留空不修改）</span>', '<span>Environment (leave blank to keep)</span>'),
    ('<option value="production">生产</option>', '<option value="production">Production</option>'),
    ('<option value="staging">预发布</option>', '<option value="staging">Staging</option>'),
    ('<option value="testing">测试</option>', '<option value="testing">Testing</option>'),
    ('<option value="development">开发</option>', '<option value="development">Development</option>'),
    ('<span>重要性（留空不修改）</span>', '<span>Criticality (leave blank to keep)</span>'),
    ('<option value="critical">核心</option>', '<option value="critical">Critical</option>'),
    ('<option value="high">重要</option>', '<option value="high">Important</option>'),
    ('<option value="medium">一般</option>', '<option value="medium">Normal</option>'),
    ('<option value="low">低</option>', '<option value="low">Low</option>'),
    ('<span>增加标签</span>', '<span>Add tags</span>'),
    ('placeholder="逗号分隔"', 'placeholder="Comma-separated"'),
    ('<span>移除标签</span>', '<span>Remove tags</span>'),
    ('<button type="button" class="btn-secondary" onclick="closeAssetBulkEdit()">取消</button>', '<button type="button" class="btn-secondary" onclick="closeAssetBulkEdit()">Cancel</button>'),
    ('id="asset-bulk-edit-submit" type="submit" class="btn-primary"', 'id="asset-bulk-edit-submit" type="submit" class="btn-primary"'),
    ('>应用修改</button>', '>Apply changes</button>'),

    # Asset editor environment/criticality unset options
    ('<option value="">未设置</option>', '<option value="">Not set</option>'),

    # Info collect page (no data-i18n on these elements)
    ('<!-- 信息收集页面 -->', '<!-- Info Collection page -->'),
    ('<label for="fofa-provider">数据源</label>', '<label for="fofa-provider">Data source</label>'),
    ('<small class="form-hint">支持 FOFA、ZoomEye、Quake、Shodan；API Key 可在系统设置或环境变量中配置。</small>', '<small class="form-hint">Supports FOFA, ZoomEye, Quake, Shodan; API Key can be configured in System Settings or environment variables.</small>'),
    ('<div>查询方式</div>', '<div>Query method</div>'),
    ('<label for="fofa-nl" id="info-collect-nl-label" class="info-collect-inner-label">自然语言（可选，AI 解析为查询语法）</label>', '<label for="fofa-nl" id="info-collect-nl-label" class="info-collect-inner-label">Natural language (optional, AI converts to query syntax)</label>'),
    ('placeholder="例如：找中国的 Apache 站点"', 'placeholder="e.g. Find Apache sites in China"'),
    ('<small class="form-hint" id="info-collect-parse-hint">解析后会弹窗展示对应数据源语法（可编辑），确认无误后再填入查询框并执行查询。</small>', '<small class="form-hint" id="info-collect-parse-hint">After parsing, a popup shows the corresponding data source syntax (editable). Confirm then paste into the query box and execute.</small>'),
    ('<label for="fofa-query" id="info-collect-query-label" class="info-collect-inner-label">查询语法（可编辑，可直接查询）</label>', '<label for="fofa-query" id="info-collect-query-label" class="info-collect-inner-label">Query syntax (editable, can query directly)</label>'),
    ('<button id="fofa-query-submit-btn" class="btn-primary" data-require-permission="fofa:execute" type="button" onclick="submitFofaSearch()">查询</button>', '<button id="fofa-query-submit-btn" class="btn-primary" data-require-permission="fofa:execute" type="button" onclick="submitFofaSearch()">Search</button>'),
    ('<small class="form-hint" id="info-collect-query-hint">选择数据源后会显示对应查询语法提示。</small>', '<small class="form-hint" id="info-collect-query-hint">Query syntax hints will appear after selecting a data source.</small>'),
    ('<div>返回配置</div>', '<div>Return config</div>'),
    ('<small class="form-hint info-collect-row-hint" id="info-collect-size-hint">不同数据源的单次返回上限可能不同。</small>', '<small class="form-hint info-collect-row-hint" id="info-collect-size-hint">The per-query return limit may vary by data source.</small>'),
    ('<!-- 字段显示/隐藏面板 -->', '<!-- Field show/hide panel -->'),

    # Project page stats chips (dynamic, no data-i18n)
    ('条事实</span>', ' facts</span>'),
    ('个漏洞</span>', ' vulnerabilities</span>'),
    ('个对话</span>', ' conversations</span>'),
    ('待补全</span>', ' pending</span>'),
    ('<!-- 项目管理页面 -->', '<!-- Projects page -->'),

    # Vulnerability page comments
    ('<!-- 漏洞管理页面 -->', '<!-- Vulnerability Management page -->'),
    ('<!-- 统计看板：点击卡片筛选严重度，与下方下拉/地址栏 hash 同步 -->', '<!-- Stats panel: click card to filter by severity, synced with dropdown/hash -->'),
    ('<!-- 筛选 -->', '<!-- Filters -->'),
    ('<!-- 漏洞列表 -->', '<!-- Vulnerability list -->'),
    ('<!-- 分页控件 -->', '<!-- Pagination controls -->'),

    # WebShell
    ('<!-- WebShell 管理页面 -->', '<!-- WebShell Management page -->'),
    ('title="拖拽调整宽度"', 'title="Drag to resize"'),

    # Chat files
    ('<!-- 对话附件 / 文件管理 -->', '<!-- Chat attachments / File management -->'),

    # Tasks
    ('<!-- 任务管理页面 -->', '<!-- Task Management page -->'),
    ('<!-- 批量任务队列列表 -->', '<!-- Batch task queue list -->'),
    ('<!-- 筛选控件 -->', '<!-- Filter controls -->'),

    # C2 pages
    ('<!-- C2 监听器管理页面 -->', '<!-- C2 Listener Management page -->'),
    ('<!-- C2 会话管理页面 -->', '<!-- C2 Session Management page -->'),
    ('<!-- C2 任务管理页面 -->', '<!-- C2 Task Management page -->'),
    ('<!-- C2 Payload 生成页面 -->', '<!-- C2 Payload Generation page -->'),
    ('<!-- C2 事件审计页面 -->', '<!-- C2 Event Audit page -->'),
    ('<!-- C2 Profile 管理页面 -->', '<!-- C2 Profile Management page -->'),
    ('<!-- C2 模态框 -->', '<!-- C2 modals -->'),

    # Workflows
    ('<!-- 工作流页面 -->', '<!-- Workflows page -->'),

    # Roles
    ('<!-- 角色管理页面 -->', '<!-- Role Management page -->'),

    # Skills
    ('<!-- Skills状态监控页面 -->', '<!-- Skills Status Monitor page -->'),
    ('<!-- Skills管理页面 -->', '<!-- Skills Management page -->'),

    # Agents
    ('<!-- 多代理子 Agent（Markdown）管理 -->', '<!-- Multi-agent sub-agent (Markdown) management -->'),
    ('<!-- 角色选择弹窗 -->', '<!-- Role selection dialog -->'),
    ('<!-- 角色编辑模态框 -->', '<!-- Role edit modal -->'),

    # Settings
    ('<!-- 系统设置页面 -->', '<!-- System Settings page -->'),
    ('<!-- 左侧导航栏 -->', '<!-- Left navigation bar -->'),
    ('<!-- 右侧内容区域 -->', '<!-- Right content area -->'),
    ('<!-- 基本设置 -->', '<!-- Basic settings -->'),
    ('<!-- 通道配置 -->', '<!-- Channel configuration -->'),
    ('<!-- 视觉分析 -->', '<!-- Visual analysis -->'),
    ('<!-- 人机协同 -->', '<!-- Human-machine collaboration -->'),
    ('<!-- 资产管理设置 -->', '<!-- Asset management settings -->'),
    ('<!-- 知识库设置 -->', '<!-- Knowledge base settings -->'),
    ('<!-- 总开关 -->', '<!-- Master switch -->'),
    ('<!-- 机器人设置 -->', '<!-- Bot settings -->'),
    ('<!-- 微信 / iLink -->', '<!-- WeChat / iLink -->'),
    ('<!-- 企业微信 -->', '<!-- Enterprise WeChat (WeCom) -->'),
    ('<!-- 钉钉 -->', '<!-- DingTalk -->'),
    ('<!-- 飞书 -->', '<!-- Feishu / Lark -->'),
    ('<!-- 安全设置 -->', '<!-- Security settings -->'),
    ('<!-- 存储清理 -->', '<!-- Storage cleanup -->'),
    ('<!-- 终端 -->', '<!-- Terminal -->'),
    ('<!-- 日志审计 -->', '<!-- Audit log -->'),
    ('<!-- 编辑弹窗 -->', '<!-- Edit dialog -->'),
    ('<!-- 项目管理弹窗（挂 body 下，避免被 .page overflow 裁剪） -->', '<!-- Project management dialog (mounted under body to avoid .page overflow clipping) -->'),

    # RBAC
    ('<!-- 平台权限页面 -->', '<!-- Platform RBAC page -->'),

    # Modals
    ('<!-- 调用详情模态框 -->', '<!-- Execution detail modal -->'),
    ('<!-- 外部MCP配置模态框 -->', '<!-- External MCP config modal -->'),
    ('<!-- 攻击链可视化模态框 -->', '<!-- Attack chain visualisation modal -->'),
    ('<!-- 工作流信息 -->', '<!-- Workflow info -->'),
    ('<!-- 知识项编辑模态框 -->', '<!-- Knowledge item edit modal -->'),
    ('<!-- 角色选择弹窗 -->', '<!-- Role selection dialog -->'),
    ('<!-- 批量管理对话模态框 -->', '<!-- Bulk conversation management modal -->'),
    ('<!-- 上下文菜单 -->', '<!-- Context menu -->'),
    ('<!-- 项目列表操作菜单 -->', '<!-- Project list action menu -->'),
    ('<!-- 新建任务模态框 -->', '<!-- New task modal -->'),
    ('<!-- 批量任务队列详情模态框 -->', '<!-- Batch task queue detail modal -->'),
    ('<!-- 添加批量任务模态框 -->', '<!-- Add batch task modal -->'),
    ('<!-- 漏洞编辑模态框 -->', '<!-- Vulnerability edit modal -->'),
    ('<!-- 添加连接模态框 -->', '<!-- Add connection modal -->'),
    ('<!-- 用户中断并说明（继续迭代） -->', '<!-- User interrupt with note (continue iterating) -->'),
    ('<!-- 工具终止：可填写给模型的说明 -->', '<!-- Tool termination: optional message for the model -->'),
    ('<!-- 知识项编辑模态框 -->', '<!-- Knowledge item edit modal -->'),
    ('<!-- 模态框 -->', '<!-- Modal -->'),
    ('<!-- 其他前端依赖：本地 vendor，内网/离线部署不依赖 CDN -->', '<!-- Other frontend dependencies: local vendor, works offline/intranet without CDN -->'),
    ('<!-- 确保ELK对象全局可用', '<!-- Ensure ELK object is globally available'),

    # Settings page content (no data-i18n)
    ('<h4>ZoomEye 配置</h4>', '<h4>ZoomEye Configuration</h4>'),
    ('<p>用于信息收集页调用 ZoomEye API；也可设置环境变量 ZOOMEYE_API_KEY。</p>', '<p>Used for ZoomEye API calls on the info collection page; or set environment variable ZOOMEYE_API_KEY.</p>'),
    ('<h4>Quake 配置</h4>', '<h4>Quake Configuration</h4>'),
    ('<p>用于信息收集页调用 360 Quake API；也可设置环境变量 QUAKE_API_KEY。</p>', '<p>Used for 360 Quake API calls on the info collection page; or set environment variable QUAKE_API_KEY.</p>'),
    ('<h4>Shodan 配置</h4>', '<h4>Shodan Configuration</h4>'),
    ('<p>用于信息收集页调用 Shodan API；也可设置环境变量 SHODAN_API_KEY。</p>', '<p>Used for Shodan API calls on the info collection page; or set environment variable SHODAN_API_KEY.</p>'),
    ('<small>留空则使用默认地址。</small>', '<small>Leave blank to use the default address.</small>'),
    ('<small>仅保存在服务器配置中（config.yaml）。</small>', '<small>Stored only in server configuration (config.yaml).</small>'),

    # Bot settings - business auth section
    ('<h4>业务鉴权策略</h4>', '<h4>Business Auth Policy</h4>'),
    ('<label>鉴权模式</label>', '<label>Auth mode</label>'),
    ('<option>逐用户绑定（多人机器人推荐）</option>', '<option>Per-user binding (recommended for multi-user bots)</option>'),
    ('<option>专用服务账号（仅白名单发送者）</option>', '<option>Dedicated service account (whitelist senders only)</option>'),
    ('<small>平台验签负责确认消息来源；这里决定消息最终使用哪个 RBAC 身份。</small>', '<small>Platform signature verification confirms the message source; this decides which RBAC identity is used for the message.</small>'),
    ('<label>服务账号 RBAC User ID</label>', '<label>Service Account RBAC User ID</label>'),
    ('<label>允许的平台发送者 ID</label>', '<label>Allowed platform sender IDs</label>'),
    ('<small>不支持 * 通配符。服务账号模式只有这里列出的真实发送者可以执行。</small>', '<small>Wildcard * is not supported. In service account mode, only the senders listed here can execute commands.</small>'),
    ('<small>当前使用 admin：白名单发送者将拥有完整平台权限，请仅添加你完全信任的账号。</small>', '<small>Currently using admin: whitelisted senders will have full platform permissions — only add accounts you fully trust.</small>'),

    # Bot command reference table
    ('>帮助</code>', '>help</code>'),
    ('>版本</code>', '>version</code>'),
    ('>身份</code>', '>identity</code>'),
    ('>绑定 &lt;绑定码&gt;</code>', '>bind &lt;code&gt;</code>'),
    ('>解绑</code>', '>unbind</code>'),
    ('>列表</code>', '>list</code>'),
    ('>切换 &lt;ID&gt;</code>', '>switch &lt;ID&gt;</code>'),
    ('>新对话</code>', '>new</code>'),
    ('>清空</code>', '>clear</code>'),
    ('>状态</code>', '>status</code>'),
    ('>任务</code>', '>task</code>'),
    ('>重命名 &lt;名称&gt;</code>', '>rename &lt;name&gt;</code>'),
    ('>停止</code>', '>stop</code>'),
    ('>删除 &lt;ID&gt;</code>', '>delete &lt;ID&gt;</code>'),
    ('>角色</code>', '>roles</code>'),
    ('>角色 &lt;名&gt;</code>', '>role &lt;name&gt;</code>'),
    ('>模式</code>', '>modes</code>'),
    ('>模式 &lt;名称&gt;</code>', '>mode &lt;name&gt;</code>'),
    ('>项目</code>', '>projects</code>'),
    ('>新建项目 &lt;名称&gt;</code>', '>new-project &lt;name&gt;</code>'),
    ('>绑定项目 &lt;ID或名称&gt;</code>', '>bind-project &lt;ID or name&gt;</code>'),
    ('>解除项目</code>', '>unbind-project</code>'),
    ('>权限</code>', '>permissions</code>'),
    ('>诊断</code>', '>doctor</code>'),
    ('>确认</code>', '>confirm</code>'),
    ('>取消</code>', '>cancel</code>'),

    # Terminal tab
    ('<span>终端 1</span>', '<span>Terminal 1</span>'),

    # Audit log columns
    ('<option>类别</option>', '<option>Category</option>'),
    ('<option>操作</option>', '<option>Action</option>'),

    # RBAC page
    ('<span>权限管理视图</span>', '<span>Permission management view</span>'),
    ('<option>资产</option>', '<option>Assets</option>'),
    ('<span>第 1 页</span>', '<span>Page 1</span>'),
    ('<span>筛选资源类型</span>', '<span>Filter resource type</span>'),
    ('<span>显示 0-0 / 共 0 条记录</span>', '<span>Showing 0–0 of 0 records</span>'),
    ('<span>第 1 / 1 页</span>', '<span>Page 1 / 1</span>'),

    # Execution tool modal
    ('<button class="btn-danger" onclick="terminateTool()">终止工具</button>', '<button class="btn-danger" onclick="terminateTool()">Terminate tool</button>'),
    ('<div role="tabpanel" aria-label="响应结果视图">', '<div role="tabpanel" aria-label="Response result view">'),

    # Knowledge modal
    ('<h2>添加知识</h2>', '<h2>Add Knowledge</h2>'),
    ('<span>分类（风险类型）<span style="color:', '<span>Category (risk type)<span style="color:'),
    ('placeholder="例如：SQL注入"', 'placeholder="e.g. SQL Injection"'),
    ('<span>标题<span style="color:', '<span>Title<span style="color:'),
    ('placeholder="知识项标题"', 'placeholder="Knowledge item title"'),
    ('<span>内容（Markdown格式）<span style="color:', '<span>Content (Markdown format)<span style="color:'),
    ('placeholder="输入知识内容，支持Markdown格式..."', 'placeholder="Enter knowledge content, Markdown supported..."'),
    ('<button type="button" class="btn-secondary" onclick="closeKnowledgeItemModal()">取消</button>', '<button type="button" class="btn-secondary" onclick="closeKnowledgeItemModal()">Cancel</button>'),
    ('<button type="submit" class="btn-primary">保存</button>', '<button type="submit" class="btn-primary">Save</button>'),

    # Fact/note modal
    ('<span>摘要（索引一行）</span>', '<span>Summary (one-line index)</span>'),
    ('placeholder="什么 + 在哪 + 如何验证（勿仅写「存在 XSS」）"', 'placeholder="What + where + how to verify (do not just write \'XSS exists\')"'),
    ('<span>可复现详情）</span>', '<span>Reproducible details)</span>'),
    ('>插入攻击链模板</button>', '>Insert attack chain template</button>'),
    ('>插入环境模板</button>', '>Insert environment template</button>'),
    ('placeholder="攻击链步骤、HTTP/命令 POC、响应现象、证据…"', 'placeholder="Attack chain steps, HTTP/command PoC, response, evidence..."'),

    # Project modal
    ('<span class="required">项目名称 </span>', '<span class="required">Project name </span>'),
    ('placeholder="或 finding/sqli-login"', 'placeholder="or finding/sqli-login"'),
    ('<small>环境类：target/、auth/、infra<br>发现/利用：finding/、chain/、exp</small>', '<small>Environment: target/, auth/, infra<br>Discovery/Exploit: finding/, chain/, exp</small>'),
    ('<option value="target">目标）</option>', '<option value="target">Target)</option>'),
    ('<option value="auth">认证）</option>', '<option value="auth">Auth)</option>'),
    ('<option value="infra">基础设施）</option>', '<option value="infra">Infrastructure)</option>'),
    ('<option value="business">业务）</option>', '<option value="business">Business)</option>'),
    ('<option value="finding">发现）</option>', '<option value="finding">Finding)</option>'),
    ('<option value="chain">攻击链）</option>', '<option value="chain">Attack chain)</option>'),
    ('<option value="exp">利用）</option>', '<option value="exp">Exploit)</option>'),
    ('<option value="note">备注）</option>', '<option value="note">Note)</option>'),
    ('<option value="pending">待确认</option>', '<option value="pending">Pending</option>'),
    ('<option value="confirmed">已确认</option>', '<option value="confirmed">Confirmed</option>'),
    ('<option value="archived">已废弃</option>', '<option value="archived">Archived</option>'),

    # Robot account binding modal
    ('<h2>绑定机器人账号</h2>', '<h2>Bind Bot Account</h2>'),
    ('<p>将 IM 平台身份安全关联到当前 RBAC 用户</p>', '<p>Securely link your IM platform identity to the current RBAC user</p>'),
    ('>生成一次性绑定码</span>', '>Generate one-time binding code</span>'),
    ('>在机器人中发送绑定命令</span>', '>Send binding command in the bot</span>'),
    ('>立即继承当前用户权限</span>', '>Immediately inherit current user permissions</span>'),
    ('>一次性安全码</span>', '>One-time security code</span>'),
    ('>等待生成</span>', '>Waiting for generation</span>'),
    ('>生成绑定码</button>', '>Generate code</button>'),
    ('>复制命令</span>', '>Copy command</span>'),
    ('>请在机器人中发送绑定命令</span>', '>Please send the binding command in the bot</span>'),
    ('<small>绑定码仅保存哈希、只能使用一次，新码会使旧码立即失效</small>', '<small>The binding code is stored as a hash, can only be used once, and a new code immediately invalidates the old one</small>'),
    ('<h3>已绑定平台账号</h3>', '<h3>Bound platform accounts</h3>'),
    ('<p>这些平台身份可使用当前用户的实时 RBAC 权限</p>', '<p>These platform identities can use the current user\'s live RBAC permissions</p>'),
    ('>如需配置专用服务账号，请前往"系统设置 → 机器人设置"。</span>', '>To configure a dedicated service account, go to "System Settings → Bot Settings".</span>'),

    # asset placeholder with Chinese chars in example
    ('placeholder="https://example.com:443、example.com 或 1.1.1.1:443"', 'placeholder="https://example.com:443, example.com or 1.1.1.1:443"'),

    # Misc comments  
    ('<!-- 配置 -->', '<!-- Configuration -->'),
    (' data-i18n-attr="配置"', ''),

    # Fofa query example with Chinese
    ('app="Apache" && country="CN"', 'app="Apache" && country="CN"'),  # keep this, it's a query example

    # Misc remaining
    ('>刷新</button>', '>Refresh</button>'),
    ('>正在加载绑定信息…</div>', '>Loading binding information…</div>'),
    ('<!-- 终端 1</span>', '<!-- Terminal 1</span>'),

    # workflow file remove
    ('title="移除文件"', 'title="Remove file"'),

    # aria-label remaining
    ('aria-label="漏洞严重度统计"', 'aria-label="Vulnerability severity statistics"'),
    ('aria-label="查询示例"', 'aria-label="Query examples"'),
    ('aria-label="字段模板"', 'aria-label="Field templates"'),
    ('aria-label="权限管理视图"', 'aria-label="Permission management view"'),
    ('aria-label="筛选资源类型"', 'aria-label="Filter resource type"'),
]

for zh, en_text in MANUAL:
    content = content.replace(zh, en_text)

# Final count
remaining = [(i+1, line.rstrip()) for i, line in enumerate(content.splitlines()) 
             if re.search(r'[\u4e00-\u9fff]', line)]
total_chars = sum(len(re.findall(r'[\u4e00-\u9fff]', line)) for _, line in remaining)
print(f"Remaining Chinese chars: {total_chars}")
print(f"Remaining lines with Chinese: {len(remaining)}")
for lineno, line in remaining[:60]:
    parts = re.findall(r'[\u4e00-\u9fff]+[^\u4e00-\u9fff]{0,15}', line)
    print(f"  L{lineno}: {str(parts)[:160]}")

with open("web/templates/index.html", "w", encoding="utf-8") as f:
    f.write(content)
print("Saved index.html")
