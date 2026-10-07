# Read the file
$content = Get-Content -Path "web\static\css\style.css" -Raw -Encoding UTF8

# Define replacements as simple arrays of [find, replace] pairs
# Using here-strings to avoid escaping issues
$pairs = @(
  ,@('/* 内容区域 */', '/* Content area */')
  ,@('/* 确保底部左右角都是圆角 */', '/* Ensure both bottom corners are rounded */')
  ,@('/* 设置页面样式 */', '/* Settings page styles */')
  ,@('/* 对话界面样式 */', '/* Chat interface styles */')
  ,@('/* 自定义滚动条样式 */', '/* Custom scrollbar styles */')
  ,@('/* 模态框样式 */', '/* Modal styles */')
  ,@('/* 响应式设计 */', '/* Responsive design */')
  ,@('/* 进度展示样式 */', '/* Progress display styles */')
  ,@('/* 活跃任务栏 */', '/* Active task bar */')
  ,@('/* 设置模态框样式 */', '/* Settings modal styles */')
  ,@('/* 设置页面布局 */', '/* Settings page layout */')
  ,@('/* 设置侧边栏 */', '/* Settings sidebar */')
  ,@('/* 设置内容区域 */', '/* Settings content area */')
  ,@('/* 现代化复选框样式 */', '/* Modern checkbox styles */')
  ,@('/* 展开图标 */', '/* Expand icon */')
  ,@('/* 展开后的详情面板 */', '/* Expanded details panel */')
  ,@('/* 参数表格 */', '/* Parameter table */')
  ,@('/* 响应式优化 */', '/* Responsive optimisation */')
  ,@('/* 饼图侧栏 */', '/* Pie chart sidebar */')
  ,@('/* 任务管理页面样式 */', '/* Task management page styles */')
  ,@('/* 漏洞管理页面样式 */', '/* Vulnerability management page styles */')
  ,@('/* 关键提醒条 */', '/* Critical alert bar */')
  ,@('/* 两列主内容网格 */', '/* Two-column main content grid */')
  ,@('/* 运行概览优化样式 */', '/* Run overview optimised styles */')
  ,@('/* 不同模块的图标颜色 */', '/* Icon colors for different modules */')
  ,@('/* 悬停效果增强 */', '/* Enhanced hover effect */')
  ,@('/* 风险分进度条 */', '/* Risk score progress bar */')
  ,@('/* 角色选择器样式 */', '/* Role selector styles */')
  ,@('/* 角色选择器包装器 */', '/* Role selector wrapper */')
  ,@('/* 角色管理页面样式 */', '/* Role management page styles */')
  ,@('/* 角色搜索框样式 */', '/* Role search box styles */')
  ,@('/* 角色卡片网格布局 */', '/* Role card grid layout */')
  ,@('/* 角色卡片样式 */', '/* Role card styles */')
  ,@('/* 角色MCP选择列表样式 */', '/* Role MCP selection list styles */')
  ,@('/* 角色工具选择列表样式 */', '/* Role tool selection list styles */')
  ,@('/* 默认角色提示信息样式 */', '/* Default role hint styles */')
  ,@('/* 角色选择面板响应式样式 */', '/* Role selection panel responsive styles */')
  ,@('/* Skills管理页面样式 */', '/* Skills management page styles */')
  ,@('/* 技能搜索框样式 */', '/* Skill search box styles */')
  ,@('/* 技能列表布局 */', '/* Skill list layout */')
  ,@('/* 技能列表项样式 */', '/* Skill list item styles */')
  ,@('/* 技能列表响应式布局优化 */', '/* Skill list responsive layout optimisation */')
  ,@('/* Skills监控页面样式 */', '/* Skills monitor page styles */')
  ,@('/* 操作列：放到最右侧并固定，避免横向滚动找不到 */', '/* Actions column: placed at the far right and pinned so it is always visible during horizontal scroll */')
  ,@('/* 单元格详情弹窗 */', '/* Cell detail popup */')
  ,@('/* 对话附件文件管理 */', '/* Conversation attachment file management */')
  ,@('/* 微信 iLink 机器人 */', '/* WeChat iLink robot */')
  ,@('/* 通用数据表格（项目管理等） */', '/* Generic data table (project management etc.) */')
  ,@('/* 项目管理 */', '/* Project management */')
  ,@('/* 工具执行次数（仅柱状图） */', '/* Tool execution count (bar chart only) */')
  ,@('/* 更多筛选生效数量：弱提示文字 */', '/* Number of active additional filters: subtle hint text */')
  ,@('/* 风险概览卡：竖向堆叠三块小模块（风险等级/待处理/最新时间） */', '/* Risk overview card: three small modules stacked vertically (risk level / pending / latest time) */')
  ,@('/* 侧栏：批量任务队列（窄栏专用布局） */', '/* Sidebar: batch task queue (narrow-column dedicated layout) */')
  ,@('/* External MCP 行可能在没配置时隐藏，display:grid 会覆盖 [hidden] 默认行为，需补一条 */', '/* External MCP row may be hidden when unconfigured; display:grid overrides the [hidden] default, so an extra rule is needed */')
  ,@('/* 最近漏洞列表 */', '/* Recent vulnerabilities list */')
  ,@('/* 状态药丸：和处置状态卡片用同一套语义色，但采用更克制的尺寸 */', '/* Status pill: uses the same semantic colors as the disposition status card, but in a more restrained size */')
  ,@('/* 能力总览 - 侧栏列表 */', '/* Capability overview — sidebar list */')
  ,@('/* 批量任务：新建队列 / 详情内联编辑 — 自定义下拉 */', '/* Batch tasks: new queue / inline detail editing — custom dropdown */')
  ,@('/* 概览区保持透明，让 KPI 与工具统计之间的缝隙透出页面底色 */', '/* Keep the overview area transparent so the gap between KPIs and tool statistics shows the page background */')
  ,@('/* 项目事实攻击路径图 */', '/* Project facts attack path graph */')
  ,@('/* 项目管理弹窗遮罩 */', '/* Project management modal overlay */')
)

foreach ($pair in $pairs) {
  $content = $content.Replace($pair[0], $pair[1])
}

# Now use regex to catch remaining lines
$lines = $content -split "`n"
$out = @()
foreach ($line in $lines) {
  if ($line -match '[\u4e00-\u9fff]') {
    Write-Host "STILL HAS CHINESE: $($line.Trim())"
  }
  $out += $line
}

$content = $out -join "`n"
Set-Content -Path "web\static\css\style.css" -Value $content -Encoding UTF8 -NoNewline
Write-Host "Done"
