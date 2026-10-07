#!/usr/bin/env python3
"""Fifth pass: exact string replacements for remaining Chinese in index.html."""
import re

with open("web/templates/index.html", "r", encoding="utf-8") as f:
    content = f.read()

replacements = [
    ('<option value="other">其他</option>', '<option value="other">Other</option>'),
    ('<!-- Agent Markdown 编辑弹窗 -->', '<!-- Agent Markdown edit dialog -->'),
    ('<!-- AI 通道配置 -->', '<!-- AI channel configuration -->'),
    ('<!-- Vision 视觉分析 -->', '<!-- Vision analysis -->'),
    ('<!-- Agent配置 -->', '<!-- Agent configuration -->'),
    ('<!-- FOFA配置 -->', '<!-- FOFA configuration -->'),
    ('<!-- C2 总开关 -->', '<!-- C2 master switch -->'),
    ('<!-- MCP调用详情模态框 -->', '<!-- MCP execution detail modal -->'),
    ('<!-- MCP 工具终止：可填写给模型的说明 -->', '<!-- MCP tool abort: optional note for the model -->'),
    ('aria-label="类别"', 'aria-label="Category"'),
    ('aria-label="操作"', 'aria-label="Action"'),
    ('data-rbac-page-info>第 1 页</span>', 'data-rbac-page-info>Page 1</span>'),
    ('data-rbac-page-info>第 1 / 1 页</span>', 'data-rbac-page-info>Page 1 / 1</span>'),
    ('data-rbac-page-range>显示 0-0 / 共 0 条记录</span>', 'data-rbac-page-range>Showing 0–0 of 0 records</span>'),
    ('data-i18n="rbac.resourceTypes.asset">资产</option>', 'data-i18n="rbac.resourceTypes.asset">Asset</option>'),
    ('<span class="terminal-tab-label" onclick="switchTerminalTab(1)">终端 1</span>', '<span class="terminal-tab-label" onclick="switchTerminalTab(1)">Terminal 1</span>'),
    ('>终止工具</button>', '>Terminate tool</button>'),
    ('aria-label="响应结果视图"', 'aria-label="Response result view"'),
    ('<!-- Marked.js + DOMPurify + 其他前端依赖：本地 vendor，内网/离线部署不依赖 CDN -->', '<!-- Marked.js + DOMPurify + other frontend deps: local vendor, works offline/intranet without CDN -->'),
    ('        // 确保ELK对象全局可用', '        // Ensure the ELK object is globally available'),
    ('<p class="form-hint">用于信息收集页调用 ZoomEye API；也可设置环境变量 ZOOMEYE_API_KEY。</p>', '<p class="form-hint">Used for ZoomEye API calls on the info collection page; or set environment variable ZOOMEYE_API_KEY.</p>'),
    ('placeholder="https://api.zoomeye.ai/v2/search（可选）"', 'placeholder="https://api.zoomeye.ai/v2/search (optional)"'),
    ('<small class="form-hint">留空则使用默认地址。</small>', '<small class="form-hint">Leave blank to use the default address.</small>'),
    ('placeholder="输入 ZoomEye API Key"', 'placeholder="Enter ZoomEye API Key"'),
    ('<small class="form-hint">仅保存在服务器配置中（config.yaml）。</small>', '<small class="form-hint">Stored only in server configuration (config.yaml).</small>'),
    ('<p class="form-hint">用于信息收集页调用 360 Quake API；也可设置环境变量 QUAKE_API_KEY。</p>', '<p class="form-hint">Used for 360 Quake API calls on the info collection page; or set environment variable QUAKE_API_KEY.</p>'),
    ('placeholder="https://quake.360.net/api/v3/search/quake_service（可选）"', 'placeholder="https://quake.360.net/api/v3/search/quake_service (optional)"'),
    ('placeholder="输入 Quake API Token"', 'placeholder="Enter Quake API Token"'),
    ('<p class="form-hint">用于信息收集页调用 Shodan API；也可设置环境变量 SHODAN_API_KEY。</p>', '<p class="form-hint">Used for Shodan API calls on the info collection page; or set environment variable SHODAN_API_KEY.</p>'),
    ('placeholder="https://api.shodan.io（可选）"', 'placeholder="https://api.shodan.io (optional)"'),
    ('placeholder="输入 Shodan API Key"', 'placeholder="Enter Shodan API Key"'),
    ('<label for="robot-auth-mode">鉴权模式</label>', '<label for="robot-auth-mode">Auth mode</label>'),
    ('<option value="user_binding">逐用户绑定（多人机器人推荐）</option>', '<option value="user_binding">Per-user binding (recommended for multi-user bots)</option>'),
    ('<option value="service_account">专用服务账号（仅白名单发送者）</option>', '<option value="service_account">Dedicated service account (whitelist senders only)</option>'),
    ('<small class="form-hint">平台验签负责确认消息来源；这里决定消息最终使用哪个 RBAC 身份。</small>', '<small class="form-hint">Platform signature verification confirms message source; this determines which RBAC identity handles the message.</small>'),
    ('<label for="robot-service-user-id">服务账号 RBAC User ID</label>', '<label for="robot-service-user-id">Service Account RBAC User ID</label>'),
    ('placeholder="例如 admin 或专用服务账号 User ID"', 'placeholder="e.g. admin or dedicated service account User ID"'),
    ('<small id="robot-service-admin-warning" class="form-hint robot-service-admin-warning" hidden>当前使用 admin：白名单发送者将拥有完整平台权限，请仅添加你完全信任的账号。</small>', '<small id="robot-service-admin-warning" class="form-hint robot-service-admin-warning" hidden>Currently using admin: whitelisted senders will have full platform permissions — only add accounts you fully trust.</small>'),
    ('<label for="robot-allowed-external-users">允许的平台发送者 ID</label>', '<label for="robot-allowed-external-users">Allowed platform sender IDs</label>'),
    ('\u201c\u8eab\u4efd\u201d', '"identity"'),
    ('<small class="form-hint">\u4e0d\u652f\u6301 * \u901a\u914d\u7b26\u3002\u670d\u52a1\u8d26\u53f7\u6a21\u5f0f\u53ea\u6709\u8fd9\u91cc\u5217\u51fa\u7684\u771f\u5b9e\u53d1\u9001\u8005\u53ef\u4ee5\u6267\u884c\u3002</small>', '<small class="form-hint">Wildcard * is not supported. In service account mode, only senders listed here can execute commands.</small>'),
    ('placeholder="\u4ece @BotFather \u83b7\u53d6"', 'placeholder="Get from @BotFather"'),
    ('placeholder="\u4e0d\u542b @\uff0c\u7559\u7a7a\u5219\u81ea\u52a8 getMe"', 'placeholder="Without @, leave blank for auto getMe"'),
]

count_before = len(re.findall(r'[\u4e00-\u9fff]', content))
for zh, en in replacements:
    content = content.replace(zh, en)
count_after = len(re.findall(r'[\u4e00-\u9fff]', content))
print(f'Replaced {count_before - count_after} Chinese chars ({count_before} -> {count_after})')

# Print remaining
remaining = [(i+1, line.rstrip()) for i, line in enumerate(content.splitlines()) 
             if re.search(r'[\u4e00-\u9fff]', line)]
print(f'Remaining lines: {len(remaining)}')
for lineno, line in remaining:
    parts = re.findall(r'[\u4e00-\u9fff]+[^\u4e00-\u9fff]{0,10}', line)
    print(f'  L{lineno}: {str(parts)[:140]}')

with open("web/templates/index.html", "w", encoding="utf-8") as f:
    f.write(content)
print("Saved")
