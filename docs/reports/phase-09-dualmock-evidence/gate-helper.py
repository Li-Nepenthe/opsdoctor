"""Read-only gate for a NEW dual-Mock run; default is a plan with no service access."""
from pathlib import Path
from decimal import Decimal, InvalidOperation
import argparse
import datetime as dt
import hashlib
import json
import os
import re
import subprocess
import sys
import urllib.request

ROOT = Path.cwd()
TEMP = Path(os.environ["TEMP"]) / "opsdoctor-phase09"
CASES = ["OT01", "OT02", "OT03", "OT04", "OT05"]
PILOT = "phase09-pilot2-20261103-terra"
DOCTOR = "http://127.0.0.1:8090"
FLAGD = "http://127.0.0.1:18080/api/read"
BALANCE = "https://api.deepseek.com/user/balance"
MODEL, JUDGE = "mock-script-v1", "mock-judge-v1"
MAX_NS, CAP, MAX_RAW = 300_000_000_000, 10_000_000, 32 << 20

class GateError(Exception):
    pass

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None

def require(ok, reason):
    if not ok:
        raise GateError(reason)

def utc():
    return dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z")

def digest(raw):
    return hashlib.sha256(raw).hexdigest()

def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)

def integer(value):
    return isinstance(value, int) and not isinstance(value, bool)

def time_valid(value):
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
        return bool(parsed.tzinfo) and parsed.year >= 2026
    except (AttributeError, TypeError, ValueError):
        return False

def time_after(later, earlier):
    if not time_valid(later) or not time_valid(earlier):
        return False
    return dt.datetime.fromisoformat(later.replace("Z", "+00:00")) > dt.datetime.fromisoformat(earlier.replace("Z", "+00:00"))

def duration_valid(value, prefix="", capped=True):
    # Only original monotonic fields supply durations, never DB or UTC subtraction.
    ns, seconds = value.get(prefix + "elapsed_ns"), value.get(prefix + "elapsed_seconds")
    if value.get(prefix + "monotonic_elapsed_known") is not True or not integer(ns) or ns < 0:
        return False
    try:
        seconds = Decimal(str(seconds))
        return (seconds.is_finite() and seconds >= 0
                and abs(seconds * Decimal(1_000_000_000) - ns) <= Decimal(1)
                and (not capped or ns <= MAX_NS))
    except (InvalidOperation, TypeError, ValueError):
        return False

class Evidence:
    def __init__(self, path):
        path.mkdir(parents=True, exist_ok=False)
        self.path, self.records, self.number = path, [], 0

    def save(self, label, raw, origin):
        self.number += 1
        target = self.path / f"{self.number:03d}-{label}.log"
        with target.open("xb") as stream:
            stream.write(raw)
            stream.flush()
            os.fsync(stream.fileno())
        item = {"path": str(target), "bytes": len(raw), "sha256": digest(raw), "origin": origin}
        self.records.append(item)
        return item

    def local_json(self, path, label):
        try:
            raw = path.read_bytes()
        except OSError:
            raise GateError("local original is missing/unreadable: " + label) from None
        require(len(raw) <= MAX_RAW, "local evidence exceeds bound: " + label)
        self.save(label, raw, str(path))
        try:
            return json.loads(raw)
        except (ValueError, UnicodeError):
            raise GateError("local original is not valid JSON: " + label) from None

    def sql(self, label, statement):
        require(statement.lstrip().upper().startswith("SELECT "), "only SELECT is allowed")
        args = ["docker", "exec", "-i", "opsdoctor-phase09-doctor-mysql", "sh", "-c",
                'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --default-character-set=utf8mb4 --batch --raw --skip-column-names -uroot doctor']
        response = subprocess.run(args, cwd=ROOT, input=statement.encode("utf-8"),
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=40)
        self.save(label + "-stdout", response.stdout, "fixed new doctor DB UTF8 SELECT")
        self.save(label + "-stderr", response.stderr, "fixed new doctor DB stderr")
        require(response.returncode == 0, "read-only DB query failed: " + label)
        require(len(response.stdout) <= MAX_RAW, "DB output exceeds bound: " + label)
        return [json.loads(line) for line in response.stdout.decode("utf-8").splitlines() if line.strip()]

    def request(self, label, url, data=None, token=None):
        require(url in {BALANCE, FLAGD} or url.startswith(DOCTOR + "/api/v1/"),
                "HTTP destination is not fixed")
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        req = urllib.request.Request(url, headers=headers,
                  data=None if data is None else json.dumps(data).encode("utf-8"))
        with urllib.request.build_opener(NoRedirect).open(req, timeout=20) as response:
            require(response.status == 200, "HTTP200 required: " + label)
            raw = response.read(MAX_RAW + 1)
        require(len(raw) <= MAX_RAW, "HTTP output exceeds bound: " + label)
        # Login contains a token: retain it only in memory, never in evidence.
        if label != "doctor-login":
            self.save(label, raw, "fixed HTTP endpoint " + url)
        return json.loads(raw)

def settings(path):
    result = {}
    for line in path.read_text(encoding="utf-8-sig").splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        require("=" in line, "invalid private settings line")
        name, value = line.split("=", 1)
        name, value = name.strip(), value.strip()
        require(name not in result, "duplicate private setting")
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        result[name] = value
    return result

def query_snapshot(ev, run_id):
    # The run ID is validated before interpolation. Pilot2 is queried for costs ONLY.
    batch = ev.sql("batch", "SELECT JSON_OBJECT('id',id,'target',target,'config',config,'status',status,"
                   "'token_limit',token_limit,'tokens_used',tokens_used,'stopped_reason',stopped_reason)"
                   f" FROM eval_runs WHERE id='{run_id}';")
    trials = ev.sql("trials", "SELECT JSON_OBJECT('scenario',scenario,'iteration',iteration,'state',state,"
                   "'fault_started_at',fault_started_at,'restored_at',restored_at)"
                   f" FROM eval_trials WHERE run_id='{run_id}' ORDER BY scenario,iteration;")
    slots = ev.sql("slots", "SELECT JSON_OBJECT('result_key',r.result_key,'run_id',r.run_id,'scenario',r.scenario,"
                  "'iteration',r.iteration,'group',r.experiment_group,'profile',r.execution_profile,"
                  "'position',r.position,'attempt',r.attempt,'retry_of',r.retry_of,'input',r.input,"
                  "'diagnosis_id',r.diagnosis_id,'judge_id',r.judge_id,'result',r.result,'judgement',r.judgement,"
                  "'doctor',JSON_OBJECT('status',d.status,'model',d.model,'provider',d.provider,"
                  "'report_mode',d.report_mode,'error',d.error,'budget_reason',d.budget_reason,'report',d.report),"
                  "'judge',JSON_OBJECT('status',j.status,'model',j.model,'provider',j.provider,'error',j.error))"
                  " FROM eval_results r LEFT JOIN diagnoses d ON d.id=r.diagnosis_id"
                  f" LEFT JOIN diagnoses j ON j.id=r.judge_id WHERE r.run_id='{run_id}'"
                  " ORDER BY r.scenario,r.iteration,r.position,r.attempt;")
    attempts = ev.sql("dualmock-attempts", "SELECT JSON_OBJECT('id',a.id,'account',a.account,"
                     "'charged_micros',a.charged_micros,'status',a.status)"
                     " FROM llm_attempts a JOIN diagnoses d ON d.id=a.diagnosis_id"
                     f" WHERE d.eval_run_id='{run_id}';")
    interruptions = ev.sql("interruptions", "SELECT JSON_OBJECT('id',id,'scenario',scenario,'iteration',iteration,"
                          "'state',state,'reason',reason)"
                          f" FROM eval_interruptions WHERE run_id='{run_id}';")
    diagnoses = ev.sql("dualmock-diagnoses", "SELECT JSON_OBJECT('id',id,'trigger',"
                       + chr(96) + "trigger" + chr(96) + ",'status',status,"
                       "'provider',provider,'model',model,'error',error)"
                       f" FROM diagnoses WHERE eval_run_id='{run_id}' ORDER BY id;")
    accounts = ev.sql("accounts", "SELECT JSON_OBJECT('account',account,'currency',currency,"
                      "'spent_micros',spent_micros,'stopped_reason',stopped_reason)"
                      " FROM llm_budget WHERE account IN ('phase-09-openai','phase-09-deepseek') ORDER BY account;")
    pilot = ev.sql("pilot2-cost-attempts-only", "SELECT JSON_OBJECT('id',a.id,'account',a.account,"
                   "'currency',b.currency,'charged_micros',a.charged_micros,'status',a.status,"
                   "'settled_at',a.settled_at) FROM llm_attempts a JOIN diagnoses d ON d.id=a.diagnosis_id"
                   " JOIN llm_budget b ON b.account=a.account"
                   f" WHERE d.eval_run_id='{PILOT}' ORDER BY a.created_at,a.id;")
    return {"batch": batch, "trials": trials, "slots": slots, "dualmock_attempts": attempts,
            "interruptions": interruptions, "diagnoses": diagnoses, "accounts": accounts,
            "pilot2_cost_attempts_only": pilot}

def execute(opts, result, ev):
    checks = result["checks"]
    def check(name, condition, reason):
        checks[name] = bool(condition)
        if not condition:
            result["reasons"].append(reason)

    require((ROOT / "AGENTS.md").is_file() and (ROOT / "internal/eval/runner/generalization.go").is_file(),
            "execute from the opsdoctor repository root")
    snapshot = query_snapshot(ev, opts.run)
    result["snapshot"] = snapshot
    batches = snapshot["batch"]
    check("one_dualmock_batch", len(batches) == 1, "new Mock batch is missing or ambiguous")
    require(len(batches) == 1, "cannot validate an absent or ambiguous new Mock batch")
    batch, expected = batches[0], {(n, 1, g) for n in CASES for g in ("E1", "E2")}
    config = batch.get("config") or {}
    check("batch_completed", batch.get("id") == opts.run and batch.get("target") == "otel-demo"
          and batch.get("status") == "completed" and not batch.get("stopped_reason"), "new Mock batch is not completed")
    check("batch_no_real_tokens", batch.get("tokens_used") == 0, "Mock has real API token reservations or usage")
    orders = config.get("orders") or []
    check("frozen_matrix", config.get("kind") == "generalization" and config.get("iterations") == 1
          and len(orders) == 5 and {(o.get("scenario"), o.get("iteration")) for o in orders} == {(n, 1) for n in CASES}
          and all(len(o.get("configurations") or []) == 2
                  and {c.get("group") for c in o["configurations"]} == {"E1", "E2"}
                  and all(c.get("profile") == "primary" and c.get("provider") == "mock"
                          and c.get("model") == MODEL and c.get("account") == "phase-09-openai"
                          and c.get("currency") == "USD" for c in o["configurations"]) for o in orders),
          "frozen matrix is not five cases E1/E2 once with fixed Mock identity")
    trials, rows = snapshot["trials"], snapshot["slots"]
    check("five_done_trials", len(trials) == 5
          and {(t.get("scenario"), t.get("iteration")) for t in trials} == {(n, 1) for n in CASES}
          and all(t.get("state") == "done" and t.get("fault_started_at") and t.get("restored_at") for t in trials),
          "five trials are not normally done/restored")
    check("ten_original_slots", len(rows) == 10
          and {(r.get("scenario"), r.get("iteration"), r.get("group")) for r in rows} == expected
          and all(r.get("run_id") == opts.run and r.get("attempt") == 0 and not r.get("retry_of")
                  and r.get("profile") == "primary" for r in rows), "expected ten original E1/E2 slots are absent")
    ids = [r.get("diagnosis_id") for r in rows] + [r.get("judge_id") for r in rows]
    check("unique_saved_diagnoses", len(ids) == 20 and all(ids) and len(set(ids)) == 20, "diagnosis IDs are absent/reused")
    check("no_interruptions", not snapshot["interruptions"], "new Mock batch has interruptions")
    check("no_paid_mock_attempts", not snapshot["dualmock_attempts"], "new Mock has real HTTP model attempts")
    check("all_twenty_mock_terminal", len(snapshot["diagnoses"]) == 20
          and {d.get("id") for d in snapshot["diagnoses"]} == set(ids)
          and all(d.get("provider") == "mock" and d.get("status") == "succeeded" and not d.get("error")
                  and d.get("trigger") in {"eval", "judge"} for d in snapshot["diagnoses"]),
          "Mock diagnoses/judges include non-Mock, failed, pending or extra tasks")
    recovery_dir, collector_root = TEMP / "recovery" / opts.run, (TEMP / "collector-audit").resolve()
    result["collector_files"] = []
    for index, row in enumerate(rows):
        summary, grade = row.get("result") or {}, row.get("judgement") or {}
        doctor, judge = row.get("doctor") or {}, row.get("judge") or {}
        check(f"mock_doctor_{index}", summary.get("diagnosis_id") == row.get("diagnosis_id")
              and summary.get("status") == "succeeded" and summary.get("model") == MODEL
              and summary.get("report_mode") == "voluntary" and summary.get("mixed_model") is False
              and summary.get("reliability_failure") is False and summary.get("cost_micros") == 0
              and summary.get("write_executions") == 0 and summary.get("approval_proposals") == 0
              and doctor.get("provider") == "mock" and doctor.get("model") == MODEL
              and doctor.get("status") == "succeeded" and doctor.get("report_mode") == "voluntary"
              and bool(doctor.get("report")) and not doctor.get("error") and not doctor.get("budget_reason"),
              "Mock doctor not normally completed, writes/mixed models/cost: " + str(row.get("result_key")))
        check(f"mock_judge_{index}", grade.get("status") == "ok" and grade.get("model") == JUDGE
              and grade.get("version") == "rubric-v1.1" and grade.get("hash") == config.get("rubric_hash")
              and grade.get("response_returned") is True and grade.get("cost_micros") == 0
              and judge.get("status") == "succeeded" and judge.get("model") == JUDGE
              and judge.get("provider") == "mock" and not judge.get("error"), "new fixed Mock judge not completed")
        audit = summary.get("collector_audit") or {}
        counts = [audit.get(k) for k in ("dropping_data_events", "log_dropped_items",
                  "mapper_error_lines", "mapper_error_occurrences")]
        maps = [audit.get(k) for k in ("signal_dropped_items", "signal_dropping_data_events")]
        maps_ok = all(isinstance(m, dict) for m in maps)
        if maps_ok:
            counts += [n for m in maps for n in m.values()]
        complete = (summary.get("data_quality") == "normal" and audit.get("status") == "ok"
                    and not audit.get("error") and maps_ok and all(integer(n) and n == 0 for n in counts)
                    and re.fullmatch(r"[a-f0-9]{64}", str(audit.get("container_id", "")))
                    and time_after(audit.get("window_end"), audit.get("window_start"))
                    and integer(audit.get("raw_bytes")) and audit["raw_bytes"] > 0
                    and re.fullmatch(r"[a-f0-9]{64}", str(audit.get("sha256", ""))))
        check(f"collector_normal_{index}", complete, "Collector normal/zero-drop/completeness check failed")
        try:
            raw_path = Path(audit["raw_path"]).resolve()
            require(raw_path.is_relative_to(collector_root), "Collector path outside fixed area")
            key_hash = hashlib.sha256(row["result_key"].encode("utf-8")).hexdigest()
            require(raw_path == collector_root / (key_hash + ".log"), "Collector filename not bound to slot")
            raw = raw_path.read_bytes()
            require(0 < len(raw) <= (64 << 20), "Collector raw size invalid")
            record = ev.save(f"collector-{index}", raw, str(raw_path))
            start = ev.local_json(raw_path.with_suffix(".start.json"), f"collector-start-{index}")
            saved_audit = ev.local_json(raw_path.with_suffix(".audit.json"), f"collector-audit-{index}")
            saved_summary = ev.local_json(raw_path.with_suffix(".summary.json"), f"collector-summary-{index}")
            ok = (len(raw) == audit.get("raw_bytes") and digest(raw) == audit.get("sha256")
                  and canonical(saved_audit) == canonical(audit) and canonical(saved_summary) == canonical(summary)
                  and start.get("key") == row["result_key"] and start.get("start") == audit.get("window_start")
                  and start.get("identity", {}).get("id") == audit.get("container_id") and bool(start.get("anchor")))
            check(f"collector_originals_{index}", ok, "Collector raw/immutable proof/DB summary mismatch")
            result["collector_files"].append(record)
        except (OSError, KeyError, ValueError, GateError):
            check(f"collector_originals_{index}", False, "Collector original/proof is unverifiable")
    paths = sorted(recovery_dir.glob("baseline-*.json"))
    observations = [ev.local_json(p, f"baseline-{i}") for i, p in enumerate(paths)]
    check("eleven_baseline_files", len(observations) == 11, "baseline count is not freeze1+pre5+post5")
    phases = {"freeze": [], "pre-inject": [], "post-cool": []}
    for value in observations:
        phases.setdefault(value.get("phase"), []).append(value)
    check("baseline_phase_coverage", len(phases["freeze"]) == 1
          and len(phases["pre-inject"]) == 5 and len(phases["post-cool"]) == 5
          and {(v.get("scenario"), v.get("iteration")) for v in phases["pre-inject"]} == {(n, 1) for n in CASES}
          and {(v.get("scenario"), v.get("iteration")) for v in phases["post-cool"]} == {(n, 1) for n in CASES}
          and all(v.get("scenario") == "" and v.get("iteration") == 0 for v in phases["freeze"]),
          "baseline phases/cases/iterations do not cover the matrix")
    for index, value in enumerate(observations):
        check(f"baseline_observed_{index}", value.get("run_id") == opts.run and value.get("kind") == "baseline"
              and value.get("status") == "passed" and not value.get("error")
              and len(value.get("scenarios") or []) == 5 and set(value["scenarios"]) == set(CASES)
              and integer(value.get("polls")) and value["polls"] >= 1
              and value.get("queries") == 5 * value["polls"]
              and time_valid(value.get("start_utc")) and time_valid(value.get("finished_utc"))
              and time_valid(value.get("passed_utc")) and duration_valid(value, capped=False)
              and time_valid(value.get("health_start_utc")) and time_valid(value.get("health_finished_utc"))
              and duration_valid(value, "health_") and value["elapsed_ns"] >= value["health_elapsed_ns"],
              "baseline timing/complete rounds/300-second health window check failed")
    result["baseline_files"], result["baseline_observations"] = [str(p) for p in paths], observations
    original_flags = config.get("baseline_flags")
    check("frozen_original_flags_present", isinstance(original_flags, dict) and bool(original_flags.get("flags")),
          "frozen original flags missing")
    result["restoration_proofs"], result["timing_observations"] = [], []
    for name in CASES:
        path = recovery_dir / (name + "-1.json")
        recovery = ev.local_json(path, name + "-recovery-original")
        text = path.read_text(encoding="utf-8")
        proof = ev.local_json(Path(str(path) + ".restored"), name + "-restored-proof")
        match = re.search(r'"original"\s*:\s*', text)
        require(match is not None, "recovery original raw JSON missing")
        _, end = json.JSONDecoder().raw_decode(text, match.end())
        check(name + "_original_restore", recovery.get("run_id") == opts.run and recovery.get("scenario") == name
              and recovery.get("iteration") == 1 and canonical(recovery.get("original")) == canonical(original_flags)
              and digest(text[match.end():end].encode("utf-8")) == proof.get("original_sha256")
              and time_valid(proof.get("confirmed_at")), "original flags/restoration SHA mismatch: " + name)
        result["restoration_proofs"].append(proof)
        for kind in ("symptom", "recovery"):
            observed = ev.local_json(recovery_dir / (name + "-1." + kind + "-observed.json"), name + "-" + kind)
            check(name + "_" + kind + "_timing", observed.get("run_id") == opts.run
                  and observed.get("scenario") == name and observed.get("iteration") == 1
                  and observed.get("kind") == kind and time_valid(observed.get("start_utc"))
                  and time_valid(observed.get("passed_utc")) and integer(observed.get("polls"))
                  and observed["polls"] >= 1 and duration_valid(observed),
                  "original symptom/recovery monotonic timing unknown/missing/over300s: " + name + "/" + kind)
            result["timing_observations"].append(observed)
    check("exact_symptom_recovery_counts", len(list(recovery_dir.glob("*.symptom-observed.json"))) == 5
          and len(list(recovery_dir.glob("*.recovery-observed.json"))) == 5, "symptom/recovery file count must be five each")
    private = settings(Path(opts.settings))
    require(private.get("LLM_PROVIDER", "mock") == "mock"
            and private.get("EVAL_COMPARISON_ENABLED", "false").lower() == "false",
            "private doctor config is not Mock/comparison disabled")
    require(private.get("DOCTOR_ADMIN_EMAIL") and private.get("DOCTOR_ADMIN_PASSWORD"), "doctor credentials missing")
    login = ev.request("doctor-login", DOCTOR + "/api/v1/auth/login",
                       {"email": private["DOCTOR_ADMIN_EMAIL"], "password": private["DOCTOR_ADMIN_PASSWORD"]})
    require(login.get("code") == "OK" and login.get("data", {}).get("access_token"), "doctor login not OK")
    token = login["data"]["access_token"]
    status = ev.request("doctor-runtime", DOCTOR + "/api/v1/status", token=token)
    targets = ev.request("doctor-targets", DOCTOR + "/api/v1/targets", token=token)
    require(status.get("code") == targets.get("code") == "OK", "doctor status/targets not OK")
    runtime, primary = status["data"], status["data"].get("executions", {}).get("primary", {})
    check("current_runtime_still_mock", runtime.get("provider") == "mock" and runtime.get("model") == MODEL
          and set(runtime.get("executions", {})) == {"primary"} and primary.get("provider") == "mock"
          and primary.get("model") == MODEL and primary.get("account") == "phase-09-openai"
          and primary.get("budget", {}).get("currency") == "USD"
          and primary.get("budget", {}).get("limit_micros") == CAP
          and primary.get("budget", {}).get("blocked") is False, "running doctor not original-account single-primary Mock")
    selected = [t for t in targets["data"] if t.get("name") == "otel-demo"]
    check("mock_target_restored_and_finished", len(selected) == 1
          and selected[0].get("fault", {}).get("open") is False
          and selected[0].get("evaluation", {}).get("enabled") is False
          and selected[0].get("evaluation", {}).get("run_id") == opts.run, "Mock target fault/evaluation mode not normally closed for this run")
    check("current_flags_equal_frozen_original",
          canonical(ev.request("current-original-flags", FLAGD)) == canonical(original_flags),
          "current complete flags differ from frozen original")
    costs = {"USD": 0, "CNY": 0}
    pilot = snapshot["pilot2_cost_attempts_only"]
    accounts_by_name = {"phase-09-openai": "USD", "phase-09-deepseek": "CNY"}
    check("pilot_cost_source_six_attempts", len(pilot) == 6
          and sum(a.get("currency") == "USD" for a in pilot) == 5
          and sum(a.get("currency") == "CNY" for a in pilot) == 1
          and all(a.get("account") in accounts_by_name and accounts_by_name[a["account"]] == a.get("currency")
                  and a.get("status") == "ok" and a.get("settled_at")
                  and integer(a.get("charged_micros")) and a["charged_micros"] >= 0 for a in pilot),
          "pilot2 cost source is not the original six settled attempts")
    for attempt in pilot:
        currency, amount = attempt.get("currency"), attempt.get("charged_micros")
        require(currency in costs and integer(amount) and amount >= 0, "pilot cost source invalid")
        costs[currency] += amount
    accounts = snapshot["accounts"]
    check("original_project_accounts", len(accounts) == 2
          and {a.get("account") for a in accounts} == set(accounts_by_name)
          and all(accounts_by_name[a["account"]] == a.get("currency")
                  and integer(a.get("spent_micros")) and a["spent_micros"] >= costs[a["currency"]]
                  and not a.get("stopped_reason") for a in accounts), "original accounts missing/reset/currency changed/blocked")
    remaining = {a["currency"]: CAP - int(a["spent_micros"]) for a in accounts}
    forecast = {currency: 10 * amount for currency, amount in costs.items()}
    key = private.get("DEEPSEEK_API_KEY")  # Same private file as the paid CLI; never prefer host environment.
    require(bool(key), "DeepSeek key missing: current free balance cannot be verified")
    balance = ev.request("deepseek-free-balance", BALANCE, token=key)
    infos = [v for v in balance.get("balance_infos", []) if v.get("currency") == "CNY"]
    require(len(infos) == 1, "DeepSeek CNY balance absent or ambiguous")
    money = Decimal(infos[0]["total_balance"]) * Decimal(1_000_000)
    require(money.is_finite() and money >= 0, "DeepSeek balance invalid")
    check("formal_usd_affordable", forecast["USD"] <= remaining.get("USD", -1), "formal USD exceeds remaining project budget")
    check("formal_cny_project_affordable", forecast["CNY"] <= remaining.get("CNY", -1), "formal CNY exceeds remaining project budget")
    check("formal_cny_supplier_affordable", balance.get("is_available") is True
          and forecast["CNY"] <= int(money), "formal CNY exceeds actual available DeepSeek balance")
    result.update(pilot_cost_micros_all_attempts=costs, forecast_formal_cost_micros_x10=forecast,
                  remaining_project_micros=remaining, deepseek_balance=balance)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", required=True)
    parser.add_argument("--output")
    parser.add_argument("--settings", default=str(ROOT / ".env.phase09-doctor"))
    parser.add_argument("--execute", action="store_true")
    opts = parser.parse_args()
    require(re.fullmatch(r"phase09-dualmock-[A-Za-z0-9_-]{1,40}", opts.run) is not None,
            "only a specified new phase09-dualmock-* run is allowed")
    output = Path(opts.output) if opts.output else TEMP / "dualmock-entry-gates" / (opts.run + "-" + dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ") + ".json")
    if not opts.execute:
        print(json.dumps({"executed": False, "run_id": opts.run, "output": str(output), "model_calls": 0,
                          "read_only_scope": ["new Mock/local originals", "current Mock/flags",
                                              "pilot2 six cost attempts only", "original accounts", "free DeepSeek balance"],
                          "old_pilot_grade_or_trial_gate": False}, indent=2))
        return 0
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    result = {"checked_at_utc": utc(), "run_id": opts.run, "allow_formal": False, "checks": {}, "reasons": [],
              "helper_sha256": digest(Path(__file__).read_bytes()), "model_calls": 0,
              "currency_note": "USD\u6309\u9879\u76ee\u7528\u91cf\u8d26\u672c\u8ba1\uff1bKey\u4e0e\u5176\u4ed6\u7528\u9014\u5171\u7528\uff0c\u5e73\u53f0\u8d26\u5355\u65e0\u6cd5\u5355\u72ec\u6838\u5bf9\u3002\u4e24\u5e01\u79cd\u4e0d\u76f8\u52a0\u3002",
              "policy": "Only new dual-Mock and current budgets/balance gate formal. Old pilot2 grade_ok/trial.done gates were withdrawn. Its original failure/cost/data are not modified or regraded.",
              "timing_note": "Only original monotonic files supply durations. Baseline full-call elapsed is not capped; health_elapsed and symptom/recovery elapsed must each be <=300s. No DB/UTC subtraction."}
    try:
        ev = Evidence(Path(str(output) + ".evidence"))
        execute(opts, result, ev)
        result["allow_formal"] = bool(result["checks"]) and all(result["checks"].values()) and not result["reasons"]
    except Exception as problem:
        result["checks"]["verification_completed"] = False
        result["reasons"].append(str(problem) if isinstance(problem, GateError) else "verification could not complete: " + type(problem).__name__)
    finally:
        result["finished_at_utc"] = utc()
        if "ev" in locals():
            result["evidence"] = ev.records
        with os.fdopen(descriptor, "wb") as stream:
            stream.write((json.dumps(result, ensure_ascii=False, indent=2) + "\n").encode("utf-8"))
            stream.flush()
            os.fsync(stream.fileno())
    print(json.dumps({k: result[k] for k in ("run_id", "allow_formal", "checks", "reasons")}, indent=2))
    return 0 if result["allow_formal"] else 2

if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except GateError as problem:
        print("gate refused: " + str(problem), file=sys.stderr)
        raise SystemExit(2)

