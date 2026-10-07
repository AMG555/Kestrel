#!/usr/bin/env python3
"""Fix remaining Chinese in style.css after partial replacements."""
import re

FIXES = [
    ("任务管理页面Content area底部圆角", "Bottom border-radius for the task management page content area"),
    ("该状态在同一事件循环内设置，避免\u201c无项目\u201d与欢迎页短暂闪现。", "Set in the same event loop to avoid a brief flash of no-project or the welcome page."),
    ('继续把滚轮自然传给外层 #chat-messages，避免鼠标悬停卡片后\u201c滚不动\u201d。', 'the inner scroll and naturally pass the wheel to the outer #chat-messages, avoiding the cannot-scroll trap when hovering over a card.'),
    ("设置Modal styles", "Settings modal styles"),
    ("设置Content area", "Settings content area"),
    ("确保分页组件宽度与Content area一致，不包括滚动条", "Ensure the pagination component width matches the content area, excluding the scrollbar"),
    ("当列表有滚动条时，分页组件应该与Content area对齐", "When the list has a scrollbar, the pagination component should align with the content area"),
    ('WebShell 行内\u201c操作\u201d下拉菜单（替代一堆按钮）', "WebShell inline 'Actions' dropdown menu (replaces a row of buttons)"),
    ('让\u201c任务执行详情\u201d视觉上跟随助手气泡宽度，而不是强行 100% 宽', "Let task execution details visually follow the assistant bubble width instead of forcing 100%"),
    ('覆盖通用 .process-details-container 的边框/内边距，避免重复一层\u201c边框卡片\u201d', "Override the border/padding of .process-details-container to avoid double-nesting a border card"),
    ('展开后才把宽度撑满；未展开时保持折叠按钮\u201c缩回去\u201d的视觉', "Only stretch to full width when expanded; keep the collapsed visual for the toggle button when not expanded"),
    ('让 timeline item 更\u201c像条目\u201d而不是松散的分隔块', "Make timeline items feel more like list entries than loose dividers"),
    ('避免每条详情都出现内层滚动条（体验会显得很\u201c碎\u201d）', "Avoid an inner scrollbar on every detail entry (it makes the UX feel fragmented)"),
    ('markdown 里已经有块级元素，不需要再整体 pre-wrap，否则容易在块之间产生\u201c空行\u201d感', "Markdown already has block elements; no need for global pre-wrap, which tends to create an empty-line feel between blocks"),
    ("风险概览卡：竖向堆叠三块小模块（风险等级/待处理/最新时间）", "Risk overview card: three small modules stacked vertically (risk level / pending / latest time)"),
    ('主内容区快捷入口：去掉图标外层的\u201c涂层\u201d，降低按钮高度', "Main content area quick-access: remove the icon wrapper overlay to reduce button height"),
    ('尊重用户\u201c减少动效\u201d偏好（无障碍最佳实践）', "Respect the user reduce-motion preference (accessibility best practice)"),
    ('批量队列详情弹窗：即使在窄屏也保持按钮不\u201c变形\u201d', "Batch queue detail modal: prevent buttons from distorting even on narrow screens"),
    ("主Content area角色选择面板样式（下拉菜单形式）", "Role selection panel styles in the main content area (dropdown form)"),
    ('表头\u201c操作\u201d居中', "Centre the Actions header"),
    ("Project management弹窗遮罩", "Project management modal overlay"),
    ('主代理轮次是一次新的\u201c模型决策 → 工具结果 → 继续决策\u201d边界。', 'The primary agent turn is a new "model decision -> tool result -> continue decision" boundary.'),
]

with open("web/static/css/style.css", "r", encoding="utf-8") as f:
    content = f.read()

for zh, en in FIXES:
    content = content.replace(zh, en)

remaining = re.findall(r'[\u4e00-\u9fff]', content)
print(f"Remaining Chinese chars: {len(remaining)}")
if remaining:
    for i, line in enumerate(content.splitlines(), 1):
        if re.search(r'[\u4e00-\u9fff]', line):
            print(f"  L{i}: {line.strip()}")

with open("web/static/css/style.css", "w", encoding="utf-8") as f:
    f.write(content)
print("Saved.")
