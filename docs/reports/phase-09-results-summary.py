"""离线复算阶段 9 正式结果，并与阶段 8 已公开的导出结果并列。

本文件只读同目录中的 JSON / Markdown 原件，不连接数据库、容器或模型。
它先逐槽位、逐 attempt 核对，再独占创建 summary.json / summary.md；已经存在
的结果不会覆盖。阶段 8 的覆盖率只能从三位小数的导出列复算，不能恢复原始精度。
在仓库根目录执行：python docs/reports/phase-09-results-summary.py
"""

from collections import Counter
from datetime import datetime, timezone
from decimal import Decimal
from hashlib import sha256
import json
from pathlib import Path
import re


HERE = Path(__file__).resolve().parent
RUN = "phase09-formal-20261103T181204Z-terra"
MODEL = "gpt-6.1-terra"
GROUPS = ("E1", "E2")
MILLION = Decimal(1000000)


def require(condition, message):
    """资料不一致就停止，不能静默丢弃不便归入分母的记录。"""
    if not condition:
        raise ValueError(message)


def load(name):
    """同时保留原文件摘要与换行统一后的摘要，便于核对阶段 10 的来源。"""
    raw = (HERE / name).read_bytes()
    metadata = {
        "path": "docs/reports/" + name,
        "bytes": len(raw),
        "sha256": sha256(raw).hexdigest(),
        "sha256_lf": sha256(raw.replace(b"\r\n", b"\n")).hexdigest(),
    }
    return raw.decode("utf-8-sig"), metadata


def table(text, heading):
    """读取指定表头的第一张表；理由中经过转义的竖线不是列分隔符。"""
    lines = text.splitlines()
    start = next(i for i, line in enumerate(lines) if line.startswith(heading))
    rows = []
    for line in lines[start + 2:]:
        if not line.startswith("|"):
            break
        rows.append([cell.strip() for cell in re.split(r"(?<!\\)\|", line)[1:-1]])
    return rows


def number(value):
    """用十进制累加导出秒数和覆盖率，避免浮点尾数改变可核验的结果。"""
    return Decimal(str(value))


def stats(values):
    values = [number(value) for value in values]
    if not values:
        return {"n": 0, "sum": None, "mean": None, "min": None, "max": None}
    total = sum(values)
    return {
        "n": len(values), "sum": float(total), "mean": float(total / len(values)),
        "min": float(min(values)), "max": float(max(values)),
    }


def amount(micros):
    return format(Decimal(micros) / MILLION, ".6f")


def phase9_group(slots):
    """组件分母是正常报告，评分和覆盖率分母分别由有效原评分决定。"""
    judged = [slot for slot in slots if slot["judge_status"] == "ok"]
    mechanisms = Counter(slot["mechanism"] for slot in judged)
    return {
        "normal_reports": len(slots),
        "component_matches": sum(slot["component_match"] for slot in slots),
        "component_denominator": len(slots),
        "valid_judgements": len(judged),
        "judge_failed": len(slots) - len(judged),
        "mechanism": {key: mechanisms[key] for key in ("correct", "partial", "incorrect")},
        "mechanism_denominator": len(judged),
        "coverage": stats(slot["coverage"] for slot in judged),
        "rounds": stats(slot["rounds"] for slot in slots),
        "tools": stats(slot["tools"] for slot in slots),
        "diagnosis_seconds": stats(slot["seconds"] for slot in slots if slot["duration_known"]),
        "duration_unknown": sum(not slot["duration_known"] for slot in slots),
        "doctor_input_tokens": sum(slot["doctor_input_tokens"] for slot in slots),
        "doctor_output_tokens": sum(slot["doctor_output_tokens"] for slot in slots),
        "doctor_cost_micros": sum(slot["doctor_cost_micros"] for slot in slots),
        "judge_input_tokens": sum(slot["judge_input_tokens"] for slot in slots),
        "judge_output_tokens": sum(slot["judge_output_tokens"] for slot in slots),
        "judge_cost_micros": sum(slot["judge_cost_micros"] for slot in slots),
    }


def build():
    text9, meta9 = load("phase-09-formal-results.md")
    snapshot_text, snapshot_meta = load("phase-09-formal-snapshot.json")
    text8, meta8 = load("phase-08-formal-results.md")
    previous_text, previous_meta = load("phase-10-eval-summary.json")
    captured, previous = json.loads(snapshot_text), json.loads(previous_text)
    snapshot = captured["snapshot"]
    require(captured["run_id"] == RUN, "不是固定正式 run")
    require(captured["capture_succeeded"] and captured["snapshot_integrity_verified"], "快照未验证")
    require(not captured["analysis"]["integrity_issues"], "快照有未解决的一致性问题")
    batch = snapshot["batch_after"][0]
    require(snapshot["batch_before"] == snapshot["batch_after"], "快照期间批次发生变化")
    require(batch["status"] == "completed" and batch["target"] == "otel-demo", "正式批次未完成")
    require(batch["config"]["kind"] == "generalization", "批次类型不匹配")
    require(previous["source_sha256_lf"] == meta8["sha256_lf"], "阶段 10 来源摘要不匹配")

    reports9 = table(text9, "| 场景 | 轮次 |")
    grades9 = table(text9, "| 槽位 | 医生错误")
    require(len(reports9) == 10 and len(grades9) == 10, "正式导出不是十个正常报告及十份原评分")
    reports9 = {row[3]: row for row in reports9}
    grades9 = {row[0]: row for row in grades9}
    slots = []
    for source in snapshot["slots"]:
        result, judge = source["result"], source["judgement"]
        audit = result["collector_audit"]
        key, diagnosis = source["result_key"], source["diagnosis_id"]
        require(source["run_id"] == RUN and source["profile"] == "primary", "槽位来源不匹配")
        require(source["group"] in GROUPS and source["iteration"] == 1 and source["attempt"] == 0,
                "正式槽位不是首轮 primary E1/E2")
        require(source["retry_of"] is None, "出现了未声明的补跑")
        require(result["model"] == MODEL and result["actual_models"] == [MODEL], "医生模型不唯一")
        require(result["status"] == "succeeded" and not result["mixed_model"]
                and not result["reliability_failure"], "混合模型或失败结果不能进入正常报告")
        require(result["report_mode"] == "voluntary" and result["data_quality"] == "normal", "报告质量状态不匹配")
        require(audit["status"] == "ok" and audit["raw_bytes"] > 0
                and len(audit["sha256"]) == 64 and audit["window_start"] and audit["window_end"],
                "缺少完整采集审计元数据")
        for field in ("dropping_data_events", "log_dropped_items", "mapper_error_lines", "mapper_error_occurrences"):
            require(audit[field] == 0, "正常报告出现丢弃或 mapper 错误")
        for field in ("signal_dropped_items", "signal_dropping_data_events"):
            require(all(value == 0 for value in audit[field].values()), "正常报告出现信号丢弃")
        require(judge["status"] == "ok" and judge["model"] == "deepseek-flash"
                and judge["version"] == "rubric-v1.1" and judge["currency"] == "CNY", "原评分状态不匹配")
        row, grade = reports9[diagnosis], grades9[key]
        require(row[:3] == [source["scenario"], "1", source["group"]], "导出槽位不匹配")
        require(row[4] == result["status"] and row[5] == str(result["component_match"]).lower(), "导出组件结果不匹配")
        require(int(row[6]) == result["rounds"] and int(row[7]) == result["tools"], "导出调用次数不匹配")
        require([int(token.strip()) for token in row[8].split("/")]
                == [result["input_tokens"], result["output_tokens"]], "导出 token 不匹配")
        require(row[9] == "USD " + amount(result["cost_micros"]), "导出医生费用不匹配")
        require(number(row[10]) == number(result["seconds"]) and result["duration_known"], "导出耗时不匹配")
        require(grade[2] == judge["grade"]["mechanism"] and number(grade[4]) == number(judge["coverage"]), "导出评分不匹配")
        require(grade[5] == "CNY " + amount(judge["cost_micros"]), "导出评分费用不匹配")
        slots.append({
            "key": key, "scenario": source["scenario"], "iteration": source["iteration"],
            "group": source["group"], "diagnosis_id": diagnosis, "judge_id": source["judge_id"],
            "model": result["model"], "actual_models": result["actual_models"],
            "report_mode": result["report_mode"], "data_quality": result["data_quality"],
            "component_match": result["component_match"], "mechanism": judge["grade"]["mechanism"],
            "judge_status": judge["status"], "coverage": judge["coverage"],
            "rounds": result["rounds"], "tools": result["tools"],
            "duration_known": result["duration_known"], "seconds": result["seconds"],
            "doctor_input_tokens": result["input_tokens"], "doctor_output_tokens": result["output_tokens"],
            "doctor_cost_micros": result["cost_micros"],
            "judge_input_tokens": judge["usage"]["prompt_tokens"],
            "judge_output_tokens": judge["usage"]["completion_tokens"],
            "judge_cost_micros": judge["cost_micros"], "collector_audit": audit,
        })
    slots.sort(key=lambda slot: (slot["scenario"], slot["group"]))
    require({(slot["scenario"], slot["group"]) for slot in slots}
            == {(f"OT{index:02}", group) for index in range(1, 6) for group in GROUPS}, "正式网格不完整")
    require(len(slots) == 10 and len(reports9) == 10 and len(grades9) == 10, "正式重复槽位")
    groups9 = {group: phase9_group([slot for slot in slots if slot["group"] == group]) for group in GROUPS}
    total9 = phase9_group(slots)

    attempts, costs = [], {}
    ids = set()
    for attempt in snapshot["attempts"]:
        require(attempt["id"] not in ids, "attempt 重复")
        ids.add(attempt["id"])
        require(attempt["linked_run_id"] == RUN and attempt["diagnosis_run_id"] == RUN, "费用桥接不一致")
        require(attempt["currency"] == attempt["diagnosis_currency"], "费用币种不一致")
        currency = attempt["currency"]
        account, trigger, model = ("phase-09-openai", "eval", MODEL) if currency == "USD" else (
            "phase-09-deepseek", "judge", "deepseek-flash")
        require(currency in ("USD", "CNY") and attempt["account"] == account
                and attempt["trigger"] == trigger and attempt["requested_model"] == model, "费用角色不匹配")
        require(attempt["status"] == "ok" and attempt["settled_at"], "出现失败或未结算 attempt，须单独处理")
        costs.setdefault(currency, {
            "account": account, "role": "doctor" if currency == "USD" else "judge",
            "attempts": 0, "statuses": {}, "input_tokens": 0, "output_tokens": 0,
            "charged_tokens": 0, "charged_micros": 0, "cache_hit_tokens": 0,
            "cache_write_tokens": 0, "reasoning_tokens": 0, "unsettled_attempts": 0,
        })
        cost = costs[currency]
        cost["attempts"] += 1
        cost["statuses"][attempt["status"]] = cost["statuses"].get(attempt["status"], 0) + 1
        for source, dest in (("tokens_in", "input_tokens"), ("tokens_out", "output_tokens"),
                             ("charged_tokens", "charged_tokens"), ("charged_micros", "charged_micros"),
                             ("cache_hit_tokens", "cache_hit_tokens"), ("cache_write_tokens", "cache_write_tokens"),
                             ("reasoning_tokens", "reasoning_tokens")):
            cost[dest] += attempt[source]
        attempts.append({key: attempt[key] for key in (
            "id", "diagnosis_id", "account", "trigger", "currency", "requested_model", "status",
            "tokens_in", "tokens_out", "charged_tokens", "charged_micros", "settled_at")})
    # 总金额相同还不够；逐诊断绑定，防止把别的槽位费用错配到当前槽位。
    by_diagnosis = {}
    for attempt in attempts:
        by_diagnosis.setdefault(attempt["diagnosis_id"], []).append(attempt)
    expected_diagnoses = {slot[key] for slot in slots for key in ("diagnosis_id", "judge_id")}
    require(set(by_diagnosis) == expected_diagnoses, "attempt 出现网格之外的诊断或缺少槽位费用")
    for slot in slots:
        for role, id_key in (("doctor", "diagnosis_id"), ("judge", "judge_id")):
            bound = by_diagnosis[slot[id_key]]
            require(sum(attempt["charged_micros"] for attempt in bound) == slot[role + "_cost_micros"], "逐诊断费用不守恒")
            require(sum(attempt["tokens_in"] for attempt in bound) == slot[role + "_input_tokens"]
                    and sum(attempt["tokens_out"] for attempt in bound) == slot[role + "_output_tokens"], "逐诊断 token 不守恒")
            require(all(attempt["currency"] == ("USD" if role == "doctor" else "CNY") for attempt in bound), "逐诊断币种不一致")
            if role == "judge":
                require(len(bound) == 1, "原评分不应隐藏自动重评")
    require(costs["USD"]["charged_micros"] == total9["doctor_cost_micros"] == 325033, "医生费用不守恒")
    require(costs["CNY"]["charged_micros"] == total9["judge_cost_micros"] == 196047, "评分费用不守恒")
    require(costs["USD"]["attempts"] == 54 and costs["CNY"]["attempts"] == 10, "attempt 数不一致")
    require(costs["USD"]["input_tokens"] == total9["doctor_input_tokens"]
            and costs["USD"]["output_tokens"] == total9["doctor_output_tokens"], "医生 token 不守恒")
    require(costs["CNY"]["input_tokens"] == total9["judge_input_tokens"]
            and costs["CNY"]["output_tokens"] == total9["judge_output_tokens"], "评委 token 不守恒")
    require(sum(cost["charged_tokens"] for cost in costs.values()) == batch["tokens_used"] == 822972, "批次 token 不守恒")
    for currency, cost in costs.items():
        cost["amount"] = amount(cost["charged_micros"])
        require(cost["charged_micros"] == captured["analysis"]["costs"]["run_all_attempts_by_currency"][currency]["charged_micros"],
                "独立费用计算与已保存分析不一致")
    accounts = []
    for account in snapshot["accounts"]:
        currency = account["currency"]
        expected = 352212 if currency == "USD" else 210967
        require(account["spent_micros"] == expected, "累计账户金额不匹配")
        accounts.append({
            **account, "amount": amount(account["spent_micros"]),
            "formal_micros": costs[currency]["charged_micros"],
            "outside_formal_micros": account["spent_micros"] - costs[currency]["charged_micros"],
            "outside_formal_note": "旧 pilot 费用保留在项目累计中，不计本次正式成本或评分分母。",
        })

    rows8 = table(text8, "| 场景 | 轮次 |")
    grades8 = {row[0]: row for row in table(text8, "| 槽位 | 医生错误")}
    groups8, slots8 = {}, []
    for group in GROUPS:
        accepted = [row for row in rows8 if row[2] == group and row[4].startswith("succeeded")
                    and row[9].startswith("USD ") and row[12] == "false"]
        require(len(accepted) == 39, "阶段 8 Terra 报告分母不匹配")
        mechanisms, coverages, failed = Counter(), [], 0
        for row in accepted:
            key = f"phase08-formal-20261029/{row[0]}/{row[1]}/primary-{group}"
            if "补跑" in row[4]:
                key += "/makeup"
            grade = grades8[key]
            valid = grade[2] in ("correct", "partial", "incorrect")
            if valid:
                require(re.fullmatch(r"\d+\.\d{3}", grade[4]) is not None, "阶段 8 覆盖率导出精度改变")
                mechanisms[grade[2]] += 1
                coverages.append(number(grade[4]))
            else:
                failed += 1
                require(grade[4] == "—", "评分失败却填入覆盖率")
            slots8.append({"key": key, "diagnosis_id": row[3], "group": group,
                           "component_match": row[5] == "true", "grading_status_or_mechanism": grade[2],
                           "coverage_display": grade[4], "accepted_makeup": "补跑" in row[4]})
        source_summary = next(row for row in previous["rows"] if row["model"] == MODEL and row["group"] == group)
        group8 = {
            "reports": len(accepted), "component_matches": sum(row[5] == "true" for row in accepted),
            "component_denominator": len(accepted), "valid_judgements": len(coverages), "judge_failed": failed,
            "mechanism": {key: mechanisms[key] for key in ("correct", "partial", "incorrect")},
            "mechanism_denominator": len(coverages), "coverage_from_rounded_export": stats(coverages),
            "coverage_source_decimal_places": 3,
        }
        require(group8["reports"] == source_summary["reports"]
                and group8["component_matches"] == source_summary["component_matches"]
                and failed == source_summary["judge_failed"], "阶段 8 与阶段 10 摘要不一致")
        for mechanism in ("correct", "partial", "incorrect"):
            require(mechanisms[mechanism] == source_summary["mechanism_" + mechanism], "阶段 8 机制计数不一致")
        groups8[group] = group8

    comparisons = []
    for group in GROUPS:
        a, b = groups8[group], groups9[group]
        comparisons.append({
            "group": group, "phase8_component": f'{a["component_matches"]}/{a["component_denominator"]}',
            "phase9_component": f'{b["component_matches"]}/{b["component_denominator"]}',
            "component_percentage_point_difference_phase9_minus_phase8": float(
                (Decimal(b["component_matches"]) / b["component_denominator"]
                 - Decimal(a["component_matches"]) / a["component_denominator"]) * 100),
            "interpretation": "仅描述两批已观测结果；系统、场景与重复次数不同，不能解释为泛化造成的因果下降或生产准确率。",
        })
    return {
        "kind": "phase09-offline-results-summary-v1", "generated_at_utc": datetime.now(timezone.utc).isoformat(),
        "run_id": RUN, "tested_commit": batch["config"]["versions"]["commit"],
        "sources": [snapshot_meta, meta9, meta8, previous_meta],
        "method": {
            "actions": "仅离线读取公开快照和导出；逐槽位与逐 attempt 复算，没有重新评分、服务请求或数据库读取。",
            "phase9_scope": "仅固定正式 run 的 primary Terra E1/E2，5 场景 × 1 次 × 2 组；Mock 和 pilot 不入正式分母。",
            "phase8_scope": "仅 phase08 正式 primary Terra E1/E2，13 场景 × 3 次 × 2 组；保留已获准 makeup，原可靠性取消不算报告。",
            "grade_missing": "评分失败不补分、不填零；组件仍用有报告的原始布尔结果。",
            "mechanism": "只统计原评委的 correct / partial / incorrect 分类，不另造部分正确的数值权重。",
            "duration": "只对 duration_known=true 的阶段 9 diagnosis seconds 求均值；不等于注入、恢复、冷却、评分在内的整轮墙钟耗时。",
            "coverage_precision": "阶段 9 用快照原 coverage；阶段 8 用 Markdown 三位小数显示值，均值不能恢复原始未舍入精度。",
            "collector_limit": "normal 仅表示各诊断窗的已保存 Collector 审计未发现最终丢弃或 mapper 错误，不证明端到端绝对无丢失；阶段 8 未追认同样审计。",
            "currency": "USD 与 CNY 分列，绝不相加。美元按项目用量账本计；Key 与其他用途共用，平台账单无法单独核对。",
            "source_integrity": "本脚本核对源文件 SHA 和快照中一致性标记，未重新读取所有 Temp 原始日志；原件链由 formal snapshot 的 evidence 保留。",
        },
        "phase9": {
            "batch_status": batch["status"], "target": batch["target"],
            "versions_as_recorded": batch["config"]["versions"],
            "rubric_version": batch["config"]["rubric_version"], "rubric_hash": batch["config"]["rubric_hash"],
            "data_quality": {"normal": len(slots), "affected": 0, "unknown": 0},
            "slots": slots, "groups": groups9, "all_formal_slots": total9,
            "attempts": attempts, "formal_costs_by_currency": costs,
            "charged_tokens_all_roles": batch["tokens_used"], "project_accounts_cumulative": accounts,
        },
        "phase8": {"groups": groups8, "accepted_slot_manifest": slots8},
        "comparison": comparisons,
        "validation": {"slot_export_consistency": True, "attempt_cost_and_token_conservation": True,
                       "per_diagnosis_attempt_binding": True,
                       "phase8_phase10_crosscheck": True, "source_snapshot_integrity_verified": True,
                       "no_regrading": True, "no_service_access": True},
    }


def render(result):
    p9, p8 = result["phase9"], result["phase8"]
    lines = [
        "# 阶段 9 正式结果与阶段 8 并列比较", "",
        f"固定批次 `{RUN}` 已完成。E1、E2 各有 5 份 normal 的 Terra 原报告，组件均为 3/5；原评委机制分类均为 2 正确、2 部分正确、1 错误，证据覆盖均值均为 0.75。以下只做离线复算，没有重新评分。", "",
        "来源：[正式 SQL 快照](phase-09-formal-snapshot.json)、[正式导出](phase-09-formal-results.md)、[阶段 8 正式导出](phase-08-formal-results.md)、[阶段 10 已核对摘要](phase-10-eval-summary.json)。逐槽位、逐 attempt 与源文件 SHA 见[统计 JSON](phase-09-results-summary.json)，可复算程序为[离线脚本](phase-09-results-summary.py)。", "",
        "## 本次正式逐槽位", "",
        "全部医生均为 `gpt-6.1-terra`，实际模型唯一，报告均为 voluntary；评委均为 `deepseek-flash`、`rubric-v1.1`。采集 affected / unknown 均为 0，10 份评分均有效，失败为 0。组件列保留原程序的布尔结果，机制列保留原评委分类，不用其中一列改写另一列。", "",
        "| 场景 | 组 | 组件一致 | 原机制评分 | 证据覆盖 | 轮数 | 工具调用 | 诊断秒 | 医生输入 / 输出 token | 医生 USD | 评委 CNY |",
        "|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|",
    ]
    for slot in p9["slots"]:
        lines.append(f'| {slot["scenario"]} | {slot["group"]} | {str(slot["component_match"]).lower()} | {slot["mechanism"]} | {slot["coverage"]:.2f} | {slot["rounds"]} | {slot["tools"]} | {slot["seconds"]:.3f} | {slot["doctor_input_tokens"]} / {slot["doctor_output_tokens"]} | {amount(slot["doctor_cost_micros"])} | {amount(slot["judge_cost_micros"])} |')
    lines += ["", "## 轮数、工具与诊断耗时", "",
              "每格为平均值 [最小, 最大]，并列有效样本数 n。耗时只统计 `duration_known=true`；本次 10 份均已知。它是诊断本身的耗时，不含完整注入、恢复、冷却与评分流程。总体行是 10 个诊断槽位，来自 5 次成对场景，不能解释成 10 次独立故障注入。", "",
              "| 组 | 轮数 | 工具调用 | 诊断秒 |", "|---|---|---|---|"]
    for group, values in list(p9["groups"].items()) + [("总体", p9["all_formal_slots"])]:
        cells = []
        for key in ("rounds", "tools", "diagnosis_seconds"):
            stat = values[key]
            cells.append(f'{stat["mean"]:g} [{stat["min"]:g}, {stat["max"]:g}] n={stat["n"]}')
        lines.append(f'| {group} | ' + " | ".join(cells) + " |")
    lines += ["", "## 正式请求与账本费用", "",
              "逐 attempt 汇总包含本正式批次的全部状态；本次所有请求均为 ok 且已结算，没有遗漏失败费用或未结算预留。输入 / 输出 token 不把缓存字段再次相加；批次 charged token 包含医生和评委，共 822972。美元按项目用量账本计；Key 与其他用途共用，平台账单无法单独核对。", "",
              "| 范围 | 币种 | attempt | 输入 token | 输出 token | charged token | 账本金额 |", "|---|---|---:|---:|---:|---:|---:|"]
    for currency in ("USD", "CNY"):
        cost = p9["formal_costs_by_currency"][currency]
        role = "正式医生" if currency == "USD" else "正式评委"
        lines.append(f'| {role} | {currency} | {cost["attempts"]} | {cost["input_tokens"]} | {cost["output_tokens"]} | {cost["charged_tokens"]} | {cost["amount"]} |')
    lines += ["", "| 组 | 医生输入 / 输出 token | 医生 USD | 评委输入 / 输出 token | 评委 CNY |", "|---|---:|---:|---:|---:|"]
    for group, values in p9["groups"].items():
        lines.append(f'| {group} | {values["doctor_input_tokens"]} / {values["doctor_output_tokens"]} | {amount(values["doctor_cost_micros"])} | {values["judge_input_tokens"]} / {values["judge_output_tokens"]} | {amount(values["judge_cost_micros"])} |')
    lines += ["", "项目账户累计另列如下。它保留旧 pilot 的真实费用，不能把累计金额当成本次正式成本；旧 pilot 的报告、失败评分和 Mock 均不进入上面的正式分母。USD 与 CNY 不相加。", "",
              "| 账户 | 币种 | 项目累计 | 本次正式 | 正式之外保留 |", "|---|---|---:|---:|---:|"]
    for account in p9["project_accounts_cumulative"]:
        lines.append(f'| {account["account"]} | {account["currency"]} | {account["amount"]} | {amount(account["formal_micros"])} | {amount(account["outside_formal_micros"])} |')
    lines += ["", "## 与阶段 8 的对应组并列", "",
              "仅比较 primary Terra 的 E1 / E2。阶段 8 是 shop 的 13 场景 × 3 次，本次是 OpenTelemetry Demo 的 5 场景 × 1 次。阶段 8 的获准补跑沿用原规则保留，原可靠性取消不计报告；评分失败单列缺失，不补零。", "",
              "| 系统 / 批次 | 组 | 正常或原合格报告数 | 组件一致 | 机制正确 / 部分 / 错误 | 有效评分 / 报告 | 评分失败 | 证据覆盖均值（n） |",
              "|---|---|---:|---:|---|---:|---:|---:|"]
    for group in GROUPS:
        a, b = p8["groups"][group], p9["groups"][group]
        for label, values, coverage in (("阶段 8 shop", a, a["coverage_from_rounded_export"]),
                                        ("阶段 9 Demo", b, b["coverage"])):
            mechanism = values["mechanism"]
            count = values.get("normal_reports", values.get("reports"))
            lines.append(f'| {label} | {group} | {count} | {values["component_matches"]}/{values["component_denominator"]} | {mechanism["correct"]} / {mechanism["partial"]} / {mechanism["incorrect"]} | {values["valid_judgements"]}/{count} | {values["judge_failed"]} | {coverage["mean"]:.4f}（{coverage["n"]}） |')
    lines += ["", "阶段 8 覆盖率来自原 Markdown 的三位小数显示值：E1 为 23.667 / 35 = 0.6762，E2 为 23.184 / 35 = 0.6624。这里保留复算结果，不能把四位均值误认为原始未舍入精度。阶段 9 来自快照原 coverage，各组为 3.75 / 5 = 0.75。机制分类只报告原分类计数，不另给 partial 指定权重。", "",
              "两组的组件比例均由阶段 8 的 36/39（约 92.3%）到本次 3/5（60.0%），描述性差值约为 −32.3 个百分点。系统、故障组成、工具可用性和重复次数同时不同，样本也很小；该差值不能证明下降由某一因素造成，更不能外推生产准确率。各阶段的报告、机制和覆盖率分母保持分开。", "",
              "本次 normal 只说明各诊断窗的已保存 Collector 审计未发现最终丢弃或 mapper 错误，不证明端到端绝对无丢失；阶段 8 没有据此追认同样的审计。原始报告、原评委评分、旧 pilot 的失败及费用均保留，没有为了比较而改分或重评。", ""]
    return "\n".join(lines)


if __name__ == "__main__":
    result = build()
    outputs = {
        HERE / "phase-09-results-summary.json": json.dumps(result, ensure_ascii=False, indent=2) + "\n",
        HERE / "phase-09-results-summary.md": render(result),
    }
    require(not any(path.exists() for path in outputs), "结果已经存在，不覆盖原件")
    for path, content in outputs.items():
        with path.open("x", encoding="utf-8", newline="\n") as target:
            target.write(content)
    print("offline summary created; no service access")
