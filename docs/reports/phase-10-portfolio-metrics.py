"""从已有评测导出复算展示指标；不调用模型，不修改原始结果。"""

import hashlib
import json
from decimal import Decimal
from pathlib import Path
from statistics import mean, median


def main():
    source = Path(__file__).with_name("phase-08-formal-results.md")
    rows = []
    for line in source.read_text(encoding="utf-8").splitlines():
        cells = [cell.strip() for cell in line.split("|")[1:-1]]
        if len(cells) != 13 or not cells[0].startswith("S"):
            continue
        # 该冻结批次中 USD E1 对应 Terra 完整工具组，CNY E1 是对比模型。
        if cells[2] == "E1" and cells[4].startswith("succeeded") and cells[9].startswith("USD "):
            assert cells[12] == "false", "不把混合模型结果并入单模型统计"
            rows.append((cells[3], Decimal(cells[10]), Decimal(cells[9].split()[1])))
    assert len(rows) == len({row[0] for row in rows}) == 39
    result = {
        "calculated_on": "2026-11-09",
        "method": "对已有导出离线复算；耗时不含注入、恢复和评分，费用为诊断模型项目账本，不含评委",
        "source": source.name,
        "source_sha256": hashlib.sha256(source.read_text(encoding="utf-8").encode("utf-8")).hexdigest(),
        "source_hash_normalization": "UTF-8 / LF，消除检出平台换行差异",
        "group": "Terra E1 / 完整工具",
        "reports": len(rows),
        "mean_seconds": str(round(mean(row[1] for row in rows), 6)),
        "median_seconds": str(median(row[1] for row in rows)),
        "mean_model_ledger_usd": str(round(mean(row[2] for row in rows), 9)),
        "median_model_ledger_usd": str(median(row[2] for row in rows)),
        "singleflight": {
            "source": "数据台账 P4-13",
            "queries_per_group": 6400,
            "loads_disabled": 6295,
            "loads_enabled": 400,
            "reduction_percent": str(round(Decimal(6295 - 400) / Decimal(6295) * 100, 6)),
        },
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
