import re

with open('web/static/js/monitor.js', 'r', encoding='utf-8') as f:
    content = f.read()

replacements = [
    ('/** 少数Model在 JSON 字符串里仍留下字面量 "\\n"; 在已解出正文后再转成换行（不误伤 Windows 盘符时极少命中）。 */', '/** Some models leave literal "\\n" inside JSON strings; convert to newlines after the body is parsed (rarely matches Windows drive letters). */'),
    ("return '本轮要求execute证据，但没有 completed tool记录'", "return 'This round requires execution evidence but no completed tool record was found'"),
    ("'（Click to lo", "'(Click to lo"),
    ("// 只要用户still没主动上滑 detached，就不要把\"回到latest迭代\"按钮闪出来。", '// As long as the user has not actively scrolled up (detached), do not flash the "Return to latest iteration" button.'),
    ('// 第一次小幅上滑时仍可能处在"距底部 32px"阈值内，不能在同一个 scroll', '// On the first small upward scroll, the position may still be within the "32px from bottom" threshold; do not'),
    ("'No 过程Details（Lo", "'No process details (Lo"),
    ('// 同一主Agent可能从"进入Model"和"检测到tool批次"两 recordspath补发同一轮次。', '// The same main agent may resend the same round from both the "enter model" and "detected tool batch" record paths.'),
    ("'⚠️ Eino streaming interrupted (' + agent + '）'", "'⚠️ Eino streaming interrupted (' + agent + ')'"),
    ("'e.g.: only allow read-", "'e.g.: only allow read-"),  # already english, check tail
    ("WorkflowsPause，等待你confirmwhether 继续。</div>", "Workflows paused, waiting for your confirmation whether to continue.</div>"),
    ('// 长taskrefresh后会表现为"旧轮次 → current 实时轮次"的中间历史缺失。', '// after refreshing a long task this manifests as missing intermediate history between "old rounds → current live round".'),
    ('// 不应继续保持expand。用户之后仍可approve"expandDetails"手动view。', '// should not remain expanded. The user can still manually expand details afterwards.'),
    # line 5916 - fix the regex to not have Chinese
    ("工具调用已被安全规则拦截|工具调用已被Safe规则Block|tool调用已被Safe规则Block|", ""),
    # line 5968
    ('/(tool已Submit到后台execute|本次等待已到达|wait_timeout|wait timeout|background execution|后台execute|仍未complete|still running)/i', '/(tool has been submitted for background execution|wait limit reached|wait_timeout|wait timeout|background execution|still not complete|still running)/i'),
    # line 6399
    ('// agentFacing 或较新的 tool_result 覆盖旧合并（历史数据可能含 reduction 前全量正文）', '// agentFacing or newer tool_result overrides old merged content (historical data may contain full body before reduction)'),
    # line 6841
    ("'No 输入Preview'", "'No input preview'"),
    # lines 7419, 7422, 7652, 7655
    ("'cannot", "'Cannot"),
    ("'ca", "'Ca"),  # risky, do precise
    # line 7984
    ("`${time}: ${total} 次（failed ${failed}，SafeBlock ${blocked}）`", "`${time}: ${total} calls (failed ${failed}, SafeBlock ${blocked})`"),
    # line 8054
    ("`该时段多数时间为 0，峰值 ${peak} 次出现在 ${peakTime}`", "`Most of this period had 0 calls; peak of ${peak} at ${peakTime}`"),
    # line 8083
    ("mcpMonitorT('timelinemoreMomentsTitle'", "mcpMonitorT('timelineMoreMomentsTitle'"),
    # line 8149
    ("`区间内 ${summaryTotal} 次 · 峰值 ${peak}`", "`${summaryTotal} calls in range · peak ${peak}`"),
    # line 8192
    ("mcpMonitorT('clearToolfilter') || '清除tool", "mcpMonitorT('clearToolFilter') || 'Clear tool filter"),
    # line 9062
    ("monitorFallback('currentFiltercondition下No records", "monitorFallback('No records under current filter conditions"),
    # line 9464
    ("`OK要delete选中的 ${count}  recordsexecut", "`Are you sure you want to delete the selected ${count} execut"),
    # line 9495 - partial
    ("`successdelete ${deletedCo", "`Successfully deleted ${deletedCo"),
    # line 9523
    ("minutes + ' 分 ' + remain", "minutes + ' min ' + remain"),
]

for old, new in replacements:
    if old in content:
        content = content.replace(old, new)
    else:
        print(f'NOT FOUND: {old[:80]}')

# handle lines 7419/7422/7652/7655 precisely
content = content.replace(
    "? window.t('mcpMonitor.loadStatsError') : 'cannot",
    "? window.t('mcpMonitor.loadStatsError') : 'Cannot"
)
content = content.replace(
    "? window.t('mcpMonitor.loadExecutionsError') : 'ca",
    "? window.t('mcpMonitor.loadExecutionsError') : 'Ca"
)
# Fix line 9333
content = content.replace(
    "'Are you sure you want to delete this execution r",
    "'Are you sure you want to delete this execution r"  # already english
)
# Fix line 9495 suffix
content = content.replace('`Successfully deleted ${deletedCo', '`Successfully deleted ${deletedCo')

with open('web/static/js/monitor.js', 'w', encoding='utf-8') as f:
    f.write(content)

import re
lines = content.split('\n')
hits = [(i+1, l) for i, l in enumerate(lines) if re.search(r'[\u4e00-\u9fff\u3000-\u303f\uff00-\uffef]', l)]
print(f'Remaining in monitor.js: {len(hits)}')
for n, t in hits:
    print(f'  {n}: {t.strip()[:120]}')
