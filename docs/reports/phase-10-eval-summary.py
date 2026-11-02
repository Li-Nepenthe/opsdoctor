"""只读汇总阶段 8 存档；从仓库根目录运行，不联网也不重新评分。"""

from collections import Counter
import hashlib
import json
from pathlib import Path


# 导出里既有逐报告表，也有按场景的平均值；只从逐报告表计数，避免把均值再当样本。
source_path = Path('docs/reports/phase-08-formal-results.md')
source_text = source_path.read_text(encoding='utf-8')
reports = {}
for line in source_text.split('## 分层结果')[0].splitlines():
    fields = [field.strip() for field in line.split('|')[1:-1]]
    if len(fields) != 13 or not fields[0].startswith('S'):
        continue
    if fields[4].startswith('cancelled'):
        continue
    assert fields[4].startswith('succeeded') and fields[12] == 'false', fields
    # 该冻结批次中美元医生是 Terra，人民币医生是 DeepSeek；评分费用在后面的独立表。
    comparison = fields[9].startswith('CNY')
    model = 'deepseek-flash' if comparison else 'gpt-6.1-terra'
    profile = 'comparison' if comparison else 'primary'
    key = f'phase08-formal-20261029/{fields[0]}/{fields[1]}/{profile}-{fields[2]}'
    if '一次补跑' in fields[4]:
        key += '/makeup'
    assert key not in reports
    reports[key] = {
        'group': fields[2], 'model': model, 'component_match': fields[5] == 'true'
    }

# 用原槽位编号关联原判分；S02 的原取消和获准补跑有不同编号，不能混在一起。
for line in source_text.split('## 评分与失败明细')[1].splitlines():
    fields = [field.strip() for field in line.split('|')[1:-1]]
    if len(fields) == 8 and fields[0] in reports:
        assert 'grade' not in reports[fields[0]]
        reports[fields[0]]['grade'] = fields[2]
assert len(reports) == 195
assert all(report.get('grade') in ('correct', 'partial', 'incorrect', '评分失败')
           for report in reports.values())
assert sum(report['grade'] == '评分失败' for report in reports.values()) == 14

rows = []
configurations = [('gpt-6.1-terra', group) for group in ('E0', 'E1', 'E2', 'E3')]
configurations.append(('deepseek-flash', 'E1'))
for model, group in configurations:
    selected = [report for report in reports.values()
                if report['model'] == model and report['group'] == group]
    counts = Counter(report['grade'] for report in selected)
    assert sum(counts.values()) == 39
    rows.append({
        'model': model, 'group': group, 'reports': len(selected),
        'component_matches': sum(report['component_match'] for report in selected),
        'mechanism_correct': counts['correct'],
        'mechanism_partial': counts['partial'],
        'mechanism_incorrect': counts['incorrect'],
        'judge_failed': counts['评分失败'],
    })

result = {
    'source': source_path.name,
    'source_sha256_lf': hashlib.sha256(source_text.replace('\r\n', '\n').encode()).hexdigest(),
    'method': '只读逐报告表，按原槽位关联原评分，获准补跑保留 /makeup；'
              '原可靠性取消不计报告分母，评分失败单列缺失，不填零、不重评。',
    'rows': rows,
}
output_path = Path('docs/reports/phase-10-eval-summary.json')
output_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps(rows, ensure_ascii=False, indent=2))
