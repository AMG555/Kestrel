$content = Get-Content -Path "web\static\css\style.css" -Raw -Encoding UTF8

# Use regex to replace all remaining Chinese comment lines
$patterns = @(
  @{ Find = '/* 内容区域 */'; Replace = '/* Content area */' }
  @{ Find = '/* 确保底部左右角都是圆角 */'; Replace = '/* Ensure both bottom corners are rounded */' }
  @{ Find = "覆盖 .new-chat-btn 的 width:100%，让 flex 分配剩余空间"; Replace = "Override .new-chat-btn width:100% to let flex distribute the remaining space" }
  @{ Find = "窄条：先「+」再折叠，纵向排列"; Replace = "Narrow strip: '+' first then collapse, laid out vertically" }
  @{ Find = '/* 设置页面样式 */'; Replace = '/* Settings page styles */' }
  @{ Find = '/* 对话界面样式 */'; Replace = '/* Chat interface styles */' }
  @{ Find = '   该状态在同一事件循环内设置，避免"无项目"与欢迎页短暂闪现。 */'; Replace = '   Set in the same event loop to avoid a brief flash of no-project or the welcome page. */' }
  @{ Find = '/* 自定义滚动条样式 */'; Replace = '/* Custom scrollbar styles */' }
  @{ Find = '       继续把滚轮自然传给外层 #chat-messages，避免鼠标悬停卡片后"滚不动"。 */'; Replace = '       naturally pass the wheel to the outer #chat-messages, avoiding the cannot-scroll trap when hovering over a card. */' }
  @{ Find = '/* 模态框样式 */'; Replace = '/* Modal styles */' }
  @{ Find = '/* 响应式设计 */'; Replace = '/* Responsive design */' }
  @{ Find = '/* 进度展示样式 */'; Replace = '/* Progress display styles */' }
  @{ Find = '/* 活跃任务栏 */'; Replace = '/* Active task bar */' }
  @{ Find = '/* 设置模态框样式 */'; Replace = '/* Settings modal styles */' }
  @{ Find = '/* 设置页面布局 */'; Replace = '/* Settings page layout */' }
  @{ Find = '/* 设置侧边栏 */'; Replace = '/* Settings sidebar */' }
  @{ Find = '/* 设置内容区域 */'; Replace = '/* Settings content area */' }
  @{ Find = '/* 现代化复选框样式 */'; Replace = '/* Modern checkbox styles */' }
  @{ Find = '/* 展开图标 */'; Replace = '/* Expand icon */' }
  @{ Find = '/* 展开后的详情面板 */'; Replace = '/* Expanded details panel */' }
  @{ Find = '/* 参数表格 */'; Replace = '/* Parameter table */' }
  @{ Find = '/* 响应式优化 */'; Replace = '/* Responsive optimisation */' }
  @{ Find = '/* 饼图侧栏 */'; Replace = '/* Pie chart sidebar */' }
  @{ Find = '/* 任务管理页面样式 */'; Replace = '/* Task management page styles */' }
  @{ Find = '/* 漏洞管理页面样式 */'; Replace = '/* Vulnerability management page styles */' }
  @{ Find = '/* WebShell 行内"操作"下拉菜单（替代一堆按钮） */'; Replace = '/* WebShell inline Actions dropdown menu (replaces a row of buttons) */' }
  @{ Find = '/* 不设 display，避免覆盖 .webshell-pane 的 display:none（否则终端/文件管理页会露出 AI 面板）。左右布局由 #webshell-pane-ai 的 flex-direction:row 提供 */'; Replace = '/* Do not set display — would override .webshell-pane display:none (causing the AI panel to show through the terminal/file-manager tabs). Left-right layout is provided by flex-direction:row on #webshell-pane-ai */' }
  @{ Find = '    /* 让"任务执行详情"视觉上跟随助手气泡宽度，而不是强行 100% 宽 */'; Replace = '    /* Let task execution details visually follow the assistant bubble width instead of forcing 100% */' }
  @{ Find = '    /* 覆盖通用 .process-details-container 的边框/内边距，避免重复一层"边框卡片" */'; Replace = '    /* Override the border/padding of the generic .process-details-container to avoid double-nesting a border card */' }
  @{ Find = '    /* 避免与外层卡片重复背景/边框 */'; Replace = '    /* Avoid duplicating the background/border of the outer card */' }
  @{ Find = '/* 展开后才把宽度撑满；未展开时保持折叠按钮"缩回去"的视觉 */'; Replace = '/* Only stretch to full width when expanded; keep the collapsed visual for the toggle button when not expanded */' }
  @{ Find = '/* 让 timeline item 更"像条目"而不是松散的分隔块 */'; Replace = '/* Make timeline items feel more like list entries than loose dividers */' }
  @{ Find = '    /* 避免每条详情都出现内层滚动条（体验会显得很"碎"） */'; Replace = '    /* Avoid an inner scrollbar on every detail entry (it makes the UX feel fragmented) */' }
  @{ Find = '    /* markdown 里已经有块级元素，不需要再整体 pre-wrap，否则容易在块之间产生"空行"感 */'; Replace = '    /* Markdown already has block elements; no need for global pre-wrap, which tends to create an empty-line feel between blocks */' }
  @{ Find = '/* 关键提醒条 */'; Replace = '/* Critical alert bar */' }
  @{ Find = '/* 两列主内容网格 */'; Replace = '/* Two-column main content grid */' }
  @{ Find = '/* 侧栏：批量任务队列（窄栏专用布局） */'; Replace = '/* Sidebar: batch task queue (narrow-column dedicated layout) */' }
  @{ Find = '    /* 时间列固定宽度：每行独立 grid 时若用 auto，「刚刚」与「N 分钟前」列宽不同 + 右对齐会看起来歪 */'; Replace = '    /* Fixed-width time column: with independent row grids, using auto would make "just now" and "N minutes ago" differ in width + right-align would look skewed */' }
  @{ Find = '/* External MCP 行可能在没配置时隐藏，display:grid 会覆盖 [hidden] 默认行为，需补一条 */'; Replace = '/* External MCP row may be hidden when unconfigured; display:grid overrides the [hidden] default, so an extra rule is needed */' }
  @{ Find = '/* 运行概览优化样式 */'; Replace = '/* Run overview optimised styles */' }
  @{ Find = '/* 不同模块的图标颜色 */'; Replace = '/* Icon colors for different modules */' }
  @{ Find = '/* 悬停效果增强 */'; Replace = '/* Enhanced hover effect */' }
  @{ Find = "   —— 左列的「风险概览」填补原来 donut 左侧的留白，把`"多危险 / 还有几个紧急项 / 多久前更新`"这类"; Replace = "   —— The left risk overview column fills the original white space left of the donut, providing conclusion-type info" }
  @{ Find = '/* 风险概览卡：竖向堆叠三块小模块（风险等级/待处理/最新时间） */'; Replace = '/* Risk overview card: three small modules stacked vertically (risk level / pending / latest time) */' }
  @{ Find = '/* 风险分进度条 */'; Replace = '/* Risk score progress bar */' }
  @{ Find = '/* 底部氛围光：轻微呼吸 + 悬停扇区时整体染上该等级色调 */'; Replace = '/* Bottom ambient glow: subtle pulse + tints the whole chart with the hovered segment color */' }
  @{ Find = '/* 透明命中层：几何固定，悬停时只改视觉层，避免 scale/描边导致边缘频闪 */'; Replace = '/* Transparent hit layer: geometrically fixed; only the visual layer changes on hover to avoid edge flickering from scale/stroke */' }
  @{ Find = '    /* 不用 scale / stroke-width，防止命中区抖动 */'; Replace = '    /* Do not use scale / stroke-width to prevent the hit area from jittering */' }
  @{ Find = '/* 主内容区快捷入口：去掉图标外层的"涂层"，降低按钮高度 */'; Replace = '/* Main content area quick-access entries: remove the icon wrapper overlay to reduce button height */' }
  @{ Find = '/* HTML5 hidden 属性默认 display:none 会被 .dashboard-cta-block 的 display:flex 覆盖，'; Replace = '/* The HTML5 hidden attribute default display:none is overridden by .dashboard-cta-block display:flex;' }
  @{ Find = '   补一条同特异性规则确保智能 CTA 隐藏逻辑生效 */'; Replace = '   add a same-specificity rule to ensure the smart CTA hide logic works */' }
  @{ Find = '/* 尊重用户"减少动效"偏好（无障碍最佳实践） */'; Replace = '/* Respect the user reduce-motion preference (accessibility best practice) */' }
  @{ Find = '    /* 批量队列详情弹窗：即使在窄屏也保持按钮不"变形" */'; Replace = '    /* Batch queue detail modal: prevent buttons from distorting even on narrow screens */' }
  @{ Find = '/* 角色选择器样式 */'; Replace = '/* Role selector styles */' }
  @{ Find = '/* 角色选择器包装器 */'; Replace = '/* Role selector wrapper */' }
  @{ Find = '    /* 限制显示8个角色：每个角色约70px高度 + gap，8个角色约580px */'; Replace = '    /* Limit to 8 visible roles: each ~70px tall + gap, 8 roles ≈ 580px */' }
  @{ Find = '/* 角色管理页面样式 */'; Replace = '/* Role management page styles */' }
  @{ Find = '/* 角色搜索框样式 */'; Replace = '/* Role search box styles */' }
  @{ Find = '/* 角色卡片网格布局 */'; Replace = '/* Role card grid layout */' }
  @{ Find = '/* 角色卡片样式 */'; Replace = '/* Role card styles */' }
  @{ Find = '/* 角色MCP选择列表样式 */'; Replace = '/* Role MCP selection list styles */' }
  @{ Find = '/* 角色工具选择列表样式 */'; Replace = '/* Role tool selection list styles */' }
  @{ Find = '/* 默认角色提示信息样式 */'; Replace = '/* Default role hint styles */' }
  @{ Find = '/* 角色选择面板响应式样式 */'; Replace = '/* Role selection panel responsive styles */' }
  @{ Find = '/* Skills管理页面样式 */'; Replace = '/* Skills management page styles */' }
  @{ Find = '/* 技能搜索框样式 */'; Replace = '/* Skill search box styles */' }
  @{ Find = '/* 技能列表布局 */'; Replace = '/* Skill list layout */' }
  @{ Find = '/* 技能列表项样式 */'; Replace = '/* Skill list item styles */' }
  @{ Find = '/* 技能列表响应式布局优化 */'; Replace = '/* Skill list responsive layout optimisation */' }
  @{ Find = '/* Skills监控页面样式 */'; Replace = '/* Skills monitor page styles */' }
  @{ Find = '    overflow: hidden; /* 配合 JS 自动增高 */'; Replace = '    overflow: hidden; /* Used with JS auto-height */' }
  @{ Find = '/* 操作列：放到最右侧并固定，避免横向滚动找不到 */'; Replace = '/* Actions column: placed at the far right and pinned so it is always visible during horizontal scroll */' }
  @{ Find = '    text-align: center; /* 表头"操作"居中 */'; Replace = '    text-align: center; /* Centre the "Actions" header */' }
  @{ Find = '    justify-content: flex-start; /* 按钮向左 */'; Replace = '    justify-content: flex-start; /* Buttons aligned left */' }
  @{ Find = '/* 单元格详情弹窗 */'; Replace = '/* Cell detail popup */' }
  @{ Find = '/* 对话附件文件管理 */'; Replace = '/* Conversation attachment file management */' }
  @{ Find = '/* GitHub 式：单表 + 首列缩进，无嵌套子表、无重复表头 */'; Replace = '/* GitHub-style: single table + first-column indent, no nested sub-tables, no repeated headers */' }
  @{ Find = '/* display 由 .modal（默认 none）与 openAppModal 内联 flex 控制，勿在此写 display:flex */'; Replace = '/* display is controlled by .modal (default none) and openAppModal inline flex; do not set display:flex here */' }
  @{ Find = '/* 新建文件夹弹窗：层次清晰、留白舒适，无强装饰 */'; Replace = '/* New folder modal: clear hierarchy, comfortable whitespace, no heavy decoration */' }
  @{ Find = '/* 全局 Toast 须高于模态遮罩 (10050) */'; Replace = '/* Global toast must appear above the modal overlay (10050) */' }
  @{ Find = '/* 对话附件读取 / 文件管理上传 进度条 */'; Replace = '/* Conversation attachment read / file management upload progress bar */' }
  @{ Find = '/* [hidden] 默认会被本类的 display:flex 覆盖，须显式隐藏否则空闲时仍露出灰条 */'; Replace = '/* [hidden] default would be overridden by this class display:flex; must be explicitly hidden or the gray bar shows when idle */' }
  @{ Find = '/* 微信 iLink 机器人 */'; Replace = '/* WeChat iLink robot */' }
  @{ Find = '/* display:flex 会覆盖 [hidden] 默认 display:none，须显式隐藏 */'; Replace = '/* display:flex overrides the [hidden] default display:none; must be explicitly hidden */' }
  @{ Find = '/* 通用数据表格（项目管理等） */'; Replace = '/* Generic data table (project management etc.) */' }
  @{ Find = '/* 项目管理 */'; Replace = '/* Project management */' }
  @{ Find = '/* display:flex 会覆盖 [hidden]，须显式隐藏 */'; Replace = '/* display:flex overrides [hidden]; must be explicitly hidden */' }
  @{ Find = '/* display:flex 会覆盖 [hidden] 默认 display:none，非激活 Tab 会叠在事实黑板下方 */'; Replace = '/* display:flex overrides [hidden] default display:none; inactive tabs would stack under the fact blackboard */' }
  @{ Find = '/* —— 事实黑板：说明 + 筛选工具栏 —— */'; Replace = '/* —— Fact blackboard: description + filter toolbar —— */' }
  @{ Find = '/* 项目事实攻击路径图 */'; Replace = '/* Project facts attack path graph */' }
  @{ Find = '/* —— 项目设置：左右分栏 + 底部危险区，无内层滚动 —— */'; Replace = '/* —— Project settings: left-right split + bottom danger zone, no inner scroll —— */' }
  @{ Find = '/* 项目管理弹窗遮罩 */'; Replace = '/* Project management modal overlay */' }
  @{ Find = '/* 对话区项目选择器（与角色/代理模式共用 role-selector-*） */'; Replace = '/* Conversation area project selector (shares role-selector-* with role/agent-mode) */' }
  @{ Find = '/* 列表 + 底部按钮共用同一内容宽度，避免滚动条缩进导致左右不齐 */'; Replace = '/* List + bottom buttons share the same content width to avoid misalignment caused by scrollbar indent */' }
  @{ Find = '    /* 工具参数与结果沿用外层卡片底色，不再额外铺一层色块。 */'; Replace = '    /* Tool parameters and results reuse the outer card background; no extra color block is applied. */' }
  @{ Find = '/* 批量任务：新建队列 / 详情内联编辑 — 自定义下拉 */'; Replace = '/* Batch tasks: new queue / inline detail editing — custom dropdown */' }
  @{ Find = '/* 概览区保持透明，让 KPI 与工具统计之间的缝隙透出页面底色 */'; Replace = '/* Keep the overview area transparent so the gap between KPIs and tool statistics shows the page background */' }
  @{ Find = '/* 主代理轮次是一次新的"模型决策 → 工具结果 → 继续决策"边界。'; Replace = '/* The primary agent turn is a new "model decision → tool result → continue decision" boundary.' }
  @{ Find = ' * 用轻量横线把长执行过程分组；子代理步骤仍沿用普通时间线节点。 */'; Replace = ' * Use a lightweight divider to group long execution runs; sub-agent steps still use regular timeline nodes. */' }
  @{ Find = '   系统设置 -> 存储清理（运行空间垃圾回收）'; Replace = '   System settings -> Storage cleanup (workspace garbage collection)' }
  @{ Find = '/* display:grid/flex 会盖过 UA 的 [hidden]，需显式提权 */'; Replace = '/* display:grid/flex overrides the UA [hidden]; must explicitly force-hide */' }
  @{ Find = '   —— 左列的「风险概览」填补原来 donut 左侧的留白，把"多危险 / 还有几个紧急项 / 多久前更新"这类'; Replace = '   —— The left risk overview column fills the original white space left of the donut, providing conclusion-type info' }
}

foreach ($p in $patterns) {
  $content = $content.Replace($p.Find, $p.Replace)
}

$remaining = [regex]::Matches($content, '[\u4e00-\u9fff]')
Write-Host "Remaining Chinese chars: $($remaining.Count)"
if ($remaining.Count -gt 0) {
  # Show lines
  $lines = $content -split "`n"
  $lineIdx = 0
  foreach ($line in $lines) {
    $lineIdx++
    if ($line -match '[\u4e00-\u9fff]') {
      Write-Host "  L$lineIdx : $($line.Trim())"
    }
  }
}

Set-Content -Path "web\static\css\style.css" -Value $content -Encoding UTF8 -NoNewline
Write-Host "Saved style.css"
