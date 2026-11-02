"""复算这二十份固定评分；只读本地材料，不连接数据库或模型。

数据库摘录来自只读事务，保留原来已经合并好的 evidence。这里不重新执行
证据规则，也不把 Claude 的意见写回数据库。--check 用来检查归档能否复现。
"""

import argparse
import hashlib
import json
from fractions import Fraction
from pathlib import Path


LABELS = ["incorrect", "partial", "correct"]
HERE = Path(__file__).resolve().parent


def agreement(pairs):
    """相邻两档的分歧算半份，跨两档算一份，再扣除边际分布带来的偶合。"""
    matrix = [[0] * 3 for _ in range(3)]
    for left, right in pairs:
        matrix[LABELS.index(left)][LABELS.index(right)] += 1
    count = len(pairs)
    observed = sum(
        Fraction(abs(i - j), 2) * matrix[i][j] / count
        for i in range(3) for j in range(3)
    )
    expected = sum(
        Fraction(abs(i - j), 2) * sum(matrix[i]) * sum(row[j] for row in matrix)
        / (count * count)
        for i in range(3) for j in range(3)
    )
    # 全部落在同一档时分母为零，不能硬填成“一致性完美”。
    kappa = 1 - observed / expected if expected else None
    matches = sum(matrix[i][i] for i in range(3))
    return {
        "n": count, "matches": matches, "agreement": matches / count,
        "labels": LABELS, "rows": "DeepSeek", "columns": "Claude",
        "matrix": matrix, "weighted_observed_disagreement": str(observed),
        "weighted_expected_disagreement": str(expected),
        "linear_weighted_kappa_fraction": str(kappa) if kappa is not None else None,
        "linear_weighted_kappa": float(kappa) if kappa is not None else None,
    }


def claude_scores(path):
    """只解析评分表的二十行；原理由原样留在输出，不能靠程序补分。"""
    scores = {}
    for line in path.read_text(encoding="utf-8-sig").splitlines():
        if not line.startswith("| [C"):
            continue
        cells = [value.strip() for value in line.strip("|").split("|")]
        assert len(cells) == 5, "评分表列数改变，请先核对原材料"
        number = cells[0][1:4]
        assert number not in scores and cells[1] in LABELS
        assert cells[3] in ("是", "否") and cells[4]
        evidence = [] if cells[2] == "无" else [int(x) for x in cells[2].split(",")]
        assert len(evidence) == len(set(evidence))
        scores[number] = {
            "mechanism": cells[1], "evidence": evidence,
            "unrelated_action": cells[3] == "是", "reason": cells[4],
        }
    assert set(scores) == {f"C{i:02}" for i in range(1, 21)}
    return scores


def calculate(raw, scores):
    """按数据库保存的抽样顺序配对，检查每项证据都恰好比较一次。"""
    keys = raw["keys"]
    by_key = {row["key"]: row for row in raw["rows"]}
    assert len(keys) == len(set(keys)) == len(by_key) == len(raw["rows"]) == 20
    assert set(keys) == set(by_key)
    rows = []
    for number, key in enumerate(keys, 1):
        original = by_key[key]
        identifier = f"C{number:02}"
        grade = original["judgement"]
        assert grade["status"] == "ok" and grade["version"] == "rubric-v1.1"
        assert grade["grade"]["mechanism"] in LABELS
        evidence = sorted(grade["evidence"], key=lambda x: x["milestone"])
        assert [item["milestone"] for item in evidence] == list(range(1, len(evidence) + 1))
        assert set(scores[identifier]["evidence"]) <= {item["milestone"] for item in evidence}
        comparisons = []
        for item in evidence:
            assert item["method"] in ("rule", "judge") and type(item["achieved"]) is bool
            comparisons.append({
                **item, "claude_achieved": item["milestone"] in scores[identifier]["evidence"],
            })
        rows.append({
            "id": identifier, "key": key, "scenario": original["scenario"],
            "iteration": original["iteration"], "group": original["group"],
            "model": original["model"], "deepseek": grade["grade"],
            "claude": scores[identifier], "evidence": comparisons,
        })
    result = {"run_id": raw["run_id"], "all": agreement([
        (r["deepseek"]["mechanism"], r["claude"]["mechanism"]) for r in rows
    ]), "without_e0": agreement([
        (r["deepseek"]["mechanism"], r["claude"]["mechanism"])
        for r in rows if r["group"] != "E0"
    ]), "evidence": {}, "rows": rows}
    for method in ("rule", "judge", "all"):
        items = [item for row in rows for item in row["evidence"]
                 if method == "all" or item["method"] == method]
        more = sum(item["achieved"] and not item["claude_achieved"] for item in items)
        less = sum(not item["achieved"] and item["claude_achieved"] for item in items)
        result["evidence"][method] = {
            "total": len(items), "matches": len(items) - more - less,
            "pipeline_more_than_claude": more, "pipeline_less_than_claude": less,
        }
    result["unrelated_actions"] = {
        "total": len(rows), "matches": sum(
            r["deepseek"]["unrelated_action"] == r["claude"]["unrelated_action"] for r in rows
        ), "deepseek_yes": sum(r["deepseek"]["unrelated_action"] for r in rows),
        "claude_yes": sum(r["claude"]["unrelated_action"] for r in rows),
    }
    return result


def markdown(result):
    """同时展示好看的总体数字和去掉无工具组后的数字，保留所有分歧。"""
    lines = [
        "# 阶段 8：DeepSeek 评委与 Claude 交叉评分的一致率", "",
        "2026-11-02，批次 `phase08-formal-20261029`。二十个抽样编号沿用数据库中的首次抽样，"
        "没有重抽、重评或改分。Claude 评分由用户提供，来源和盲评局限见[原评分](phase-08-claude-scores.md)。", "",
        "**未做人工校准；两个模型可能有共同偏差，这个数不能当作评委与人的一致率。** "
        "只比较有报告且评分有效的固定样本，无法说明另外十四份无效评分的质量。", "",
        "## 机制判断", "",
        "| 样本 | 原始一致率 | 线性加权 Cohen’s kappa | 精确分数 |",
        "|---|---|---|---|",
    ]
    for title, key in (("全部二十份", "all"), ("去掉 E0", "without_e0")):
        item = result[key]
        lines.append(f"| {title} | {item['matches']}/{item['n']}（{item['agreement']:.0%}） | "
                     f"{item['linear_weighted_kappa']:.6f} | {item['linear_weighted_kappa_fraction']} |")
    lines += ["", "档位顺序为 incorrect、partial、correct。权重 w(i,j)=1−|i−j|/2；"
              "相邻档分歧扣半份、跨两档扣一份。κ=1−实际加权分歧/按两边档位比例推得的加权分歧。"
              "κ还受档位分布影响，不能把它当作正确率。五份 E0 都是双方一致的 incorrect，"
              "所以必须同时阅读去掉 E0 的结果；这里不套用固定的“好坏”等级。"]
    for title, key in (("全部样本", "all"), ("去掉 E0", "without_e0")):
        lines += ["", f"### {title}的 3×3 混淆矩阵", "",
                  "行是 DeepSeek，列是 Claude；每格表示有几份报告。", "",
                  "| DeepSeek \\ Claude | incorrect | partial | correct |", "|---|---|---|---|"]
        for label, values in zip(LABELS, result[key]["matrix"]):
            lines.append(f"| {label} | {' | '.join(map(str, values))} |")
    lines += ["", "## 二十份逐份对照", "",
              "证据列列出最终判为已达成的编号。DeepSeek一侧使用已保存的完整 evidence（规则项＋评委项），"
              "没有只拿 grade.milestones 来比较。", "",
              "| 编号 | 场景/轮次 | 组 | 医生模型 | DeepSeek机制 | Claude机制 | 最终证据 | Claude证据 | 不相关写建议（DeepSeek/Claude） |",
              "|---|---|---|---|---|---|---|---|---|"]
    def numbers(items):
        return ",".join(map(str, items)) or "无"
    for row in result["rows"]:
        ds, cs = row["deepseek"], row["claude"]
        lines.append(f"| [{row['id']}](phase-08-human-calibration.md#{row['id'].lower()}) | "
                     f"{row['scenario']}/{row['iteration']} | {row['group']} | {row['model']} | "
                     f"{ds['mechanism']} | {cs['mechanism']} | "
                     f"{numbers([e['milestone'] for e in row['evidence'] if e['achieved']])} | "
                     f"{numbers(cs['evidence'])} | "
                     f"{'是' if ds['unrelated_action'] else '否'}/{'是' if cs['unrelated_action'] else '否'} |")
    lines += ["", "## 证据逐项统计", "",
              "多给指最终判 true、Claude 判 false；少给指反过来。这里只描述分歧，不把 Claude 当绝对答案。", "",
              "| 来源 | 项目总数 | 一致数 | 最终判定比Claude多给 | 最终判定比Claude少给 |",
              "|---|---|---|---|---|"]
    for title, key in (("规则项", "rule"), ("DeepSeek评委项", "judge"), ("合计", "all")):
        value = result["evidence"][key]
        lines.append(f"| {title} | {value['total']} | {value['matches']} | "
                     f"{value['pipeline_more_than_claude']} | {value['pipeline_less_than_claude']} |")
    lines += ["", "下面列出全部证据分歧；理由完整保存在[逐项 JSON](phase-08-cross-scoring.json)。", "",
              "| 编号 | 项目 | 来源 | 最终判定 | Claude判定 | 原最终判定的理由 |",
              "|---|---|---|---|---|---|"]
    for row in result["rows"]:
        for item in row["evidence"]:
            if item["achieved"] != item["claude_achieved"]:
                reason = item["reason"].replace("|", "\\|").replace("\n", " ")
                lines.append(f"| {row['id']} | {item['milestone']} | {item['method']} | "
                             f"{item['achieved']} | {item['claude_achieved']} | {reason} |")
    actions = result["unrelated_actions"]
    lines += ["", "## 不相关写建议", "",
              f"逐份对照见上表，一致 {actions['matches']}/{actions['total']}。"
              f"DeepSeek判“是”{actions['deepseek_yes']}份，Claude判“是”{actions['claude_yes']}份。"
              "这批抽样没有正例，不能据此说明能识别不当写建议；原正式矩阵中的不当提议与审批记录保留。", "",
              "## 三份机制分歧", ""]
    for row in result["rows"]:
        ds, cs = row["deepseek"], row["claude"]
        if ds["mechanism"] != cs["mechanism"]:
            lines += [f"### {row['id']}：{row['scenario']}/{row['iteration']}，{row['group']}，{row['model']}", "",
                      f"- DeepSeek：{ds['mechanism']}。{ds['reason']}",
                      f"- Claude：{cs['mechanism']}。{cs['reason']}", ""]
    lines += ["## 怎样复算", "",
              "原数据见[数据库只读摘录](phase-08-cross-scoring-source.json)，包含抽样顺序、"
              "最终评分和原结果 SHA-256。模型、组在评分完成后才与编号关联。"
              "[脚本](phase-08-cross-scoring.py)只用 Python 标准库，按有序三档和精确分数计算。", "",
              "```powershell", "python docs/reports/phase-08-cross-scoring.py --check", "```", "",
              "--check 只读文件并比对归档字节；不传它才重新生成本页与 JSON。"
              "不连接模型或数据库，不修改项目的 USD/CNY 账户。"]
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    # 用与实际分布不同的小例子检查公式：完全一致=1、反向两端=-1。
    assert agreement([("incorrect", "incorrect"), ("correct", "correct")])["linear_weighted_kappa"] == 1
    assert agreement([("incorrect", "correct"), ("correct", "incorrect")])["linear_weighted_kappa"] == -1
    assert agreement([("correct", "correct")])["linear_weighted_kappa"] is None
    source = HERE / "phase-08-cross-scoring-source.json"
    scores = HERE / "phase-08-claude-scores.md"
    result = calculate(json.loads(source.read_text(encoding="utf-8")), claude_scores(scores))
    result["source_sha256"] = hashlib.sha256(source.read_bytes()).hexdigest()
    result["claude_scores_sha256"] = hashlib.sha256(scores.read_bytes()).hexdigest()
    outputs = {
        HERE / "phase-08-cross-scoring.json": json.dumps(result, ensure_ascii=False, indent=2) + "\n",
        HERE / "phase-08-cross-scoring.md": markdown(result),
    }
    for path, content in outputs.items():
        if args.check:
            assert path.read_bytes() == content.encode("utf-8"), f"无法复现：{path.name}"
        else:
            path.write_bytes(content.encode("utf-8"))
    print("Verified: 20 fixed scores; mechanism, evidence and actions reproduced; no model calls.")


if __name__ == "__main__":
    main()
