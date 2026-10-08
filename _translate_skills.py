import re

with open('web/static/js/skills.js', 'r', encoding='utf-8') as fp:
    content = fp.read()

phrases = [
    ("// Skills管理相关功能", "// Skills management functionality"),
    ("// 防止重复提交", "// Prevent duplicate submissions"),
    ("// 搜索防抖定时器", "// Search debounce timer"),
    ("// 每页20条（默认值，实际从localStorage读取）", "// 20 items per page (default, read from localStorage in practice)"),
    ("// 获取保存的每页显示数量", "// Get saved items per page"),
    ("console.warn('无法从localStorage读取分页设置:', e);", "console.warn('Failed to read pagination settings from localStorage:', e);"),
    ("// 默认20", "// Default 20"),
    ("// 初始化分页设置", "// Initialize pagination settings"),
    ("// 加载skills列表（支持分页）", "// Load skills list (supports pagination)"),
    ("// 如果没有指定pageSize，使用保存的值或默认值", "// If pageSize not specified, use saved value or default"),
    ("// 更新分页状态（确保使用正确的pageSize）", "// Update pagination state (ensure correct pageSize)"),
    ("// 清空搜索关键词（正常分页加载时）", "// Clear search keyword (during normal paginated loading)"),
    ("// 构建URL（支持分页）", "// Build URL (supports pagination)"),
    ("console.error('加载skills列表失败:', error);", "console.error('Failed to load skills list:', error);"),
    ("// 渲染skills列表", "// Render skills list"),
    ("// 后端已经完成搜索过滤，直接使用skillsList", "// Backend has performed search filtering, use skillsList directly"),
    ("// 搜索时隐藏分页", "// Hide pagination during search"),
    ("// 确保列表容器可以滚动，分页栏可见", "// Ensure list container can scroll and pagination bar is visible"),
    ("// 使用 setTimeout 确保 DOM 更新完成后再检查", "// Use setTimeout to check after DOM update completes"),
    ("// 确保分页栏可见", "// Ensure pagination bar is visible"),
    ("// 渲染分页组件（参考MCP管理页面样式）", "// Render pagination component (matching MCP management style)"),
    ("// 即使只有一页也显示分页信息（参考MCP样式）", "// Show pagination info even if only one page"),
    ("// 计算显示范围", "// Calculate display range"),
    ("// 左侧：显示范围信息和每页数量选择器（参考MCP样式）", "// Left: display range info and page size selector"),
    ("// 右侧：分页按钮（参考MCP样式：首页、上一页、第X/Y页、下一页、末页）", "// Right: pagination buttons (first, prev, page X/Y, next, last)"),
    ("// 确保分页组件与列表内容区域对齐（不包括滚动条）", "// Ensure pagination component aligns with list content area (excluding scrollbar)"),
    ("// 确保分页容器始终可见", "// Ensure pagination container is always visible"),
    ("// 获取列表的实际内容宽度（不包括滚动条）", "// Get actual content width of list (excluding scrollbar)"),
    ("// 可视区域宽度（不包括滚动条）", "// Client width (excluding scrollbar)"),
    ("// 内容总高度", "// Total scroll height"),
    ("// 可视区域高度", "// Client height"),
    ("// 如果列表有垂直滚动条，分页组件应该与列表内容区域对齐（clientWidth）", "// If list has vertical scrollbar, pagination aligns with clientWidth"),
    ("// 如果没有滚动条，使用100%宽度", "// If no scrollbar, use 100% width"),
    ("// 分页组件应该与列表内容区域对齐，不包括滚动条", "// Pagination aligns with content area excluding scrollbar"),
    ("// 立即执行一次", "// Execute immediately once"),
    ("// 监听窗口大小变化和列表内容变化", "// Listen for window resize and list content changes"),
    ("// 确保分页容器始终可见（防止被隐藏）", "// Ensure pagination container remains visible"),
    ("// 改变每页显示数量", "// Change items per page"),
    ("// 保存到localStorage", "// Save to localStorage"),
    ("console.warn('无法保存分页设置到localStorage:', e);", "console.warn('Failed to save pagination settings to localStorage:', e);"),
    ("// 更新分页状态", "// Update pagination state"),
    ("// 重新计算当前页（确保不超出范围）", "// Recalculate current page (ensure within bounds)"),
    ("// 重新加载数据", "// Reload data"),
    ("// 更新skills管理统计信息", "// Update skills management statistics"),
    ("// 搜索skills", "// Search skills"),
    ("// 有搜索关键词时，使用后端搜索API（加载所有匹配结果，不分页）", "// When search keyword present, use backend search API (unpaginated)"),
    ("// 更新统计信息（显示搜索结果数量）", "// Update statistics (display search result count)"),
    ("console.error('搜索skills失败:', error);", "console.error('Failed to search skills:', error);"),
    ("// 没有搜索关键词时，恢复分页加载", "// When no search keyword, restore paginated loading"),
    ("// 清除skills搜索", "// Clear skills search"),
    ("// 恢复分页加载", "// Restore paginated loading"),
    ("// 刷新skills", "// Refresh skills"),
    ("// 显示添加skill模态框", "// Show add skill modal"),
    ("// 编辑skill", "// Edit skill"),
    ("console.error('加载skill详情失败:', error);", "console.error('Failed to load skill details:', error);"),
    ("// 查看 skill：先摘要再按需拉全文（与多代理 Eino skill 渐进披露思路一致）", "// View skill: summary first, fetch full text on demand"),
    ("console.error('查看skill失败:', error);", "console.error('Failed to view skill:', error);"),
    ("// 关闭查看模态框", "// Close view modal"),
    ("// 关闭skill模态框", "// Close skill modal"),
    ("// 保存skill", "// Save skill"),
    ("console.error('保存skill失败:', error);", "console.error('Failed to save skill:', error);"),
    ("// 删除skill", "// Delete skill"),
    ("// 先检查是否有角色绑定了该skill", "// First check if any roles bind to this skill"),
    ("console.warn('检查skill绑定失败:', error);", "console.warn('Failed to check skill bindings:', error);"),
    ("// 如果检查失败，继续执行删除流程", "// If check fails, continue with deletion"),
    ("// 构建确认消息", "// Build confirmation message"),
    ("// 如果当前页没有数据了，回到上一页", "// If current page has no data, return to previous page"),
    ("console.error('删除skill失败:', error);", "console.error('Failed to delete skill:', error);"),
    ("// ==================== Skills状态监控相关函数 ====================", "// ==================== Skills Status Monitoring Functions ===================="),
    ("// 加载skills监控数据", "// Load skills monitoring data"),
    ("console.error('加载skills监控数据失败:', error);", "console.error('Failed to load skills monitoring data:', error);"),
    ("// 渲染skills监控页面", "// Render skills monitoring page"),
    ("// 渲染总体统计", "// Render overall statistics"),
    ("// 渲染调用统计表格", "// Render call statistics table"),
    ("// 如果没有统计数据，显示空状态", "// If no statistics data, show empty state"),
    ("// 按调用次数排序（降序），如果调用次数相同，按名称排序", "// Sort by call count (descending), if equal sort by name"),
    ("// 刷新skills监控", "// Refresh skills monitoring"),
    ("// 清空skills统计数据", "// Clear skills statistics"),
    ("// 重新加载统计数据", "// Reload statistics data"),
    ("console.error('清空统计数据失败:', error);", "console.error('Failed to clear statistics data:', error);"),
    ("// HTML转义函数", "// HTML escape function"),
    ("// 语言切换时重新渲染当前页（技能列表与分页使用 _t，需随语言更新）", "// Re-render current page on language change")
]

count = 0
for orig, repl in phrases:
    if orig in content:
        content = content.replace(orig, repl)
        count += 1
    else:
        print(f"Missed: {orig}")

with open('web/static/js/skills.js', 'w', encoding='utf-8') as fp:
    fp.write(content)

print(f"skills.js: replaced {count} phrases successfully!")
