// 页面只展示服务端保存的结果，不在浏览器里执行模型建议。
// 外部文字一律交给 textContent，禁止把报告或工具结果拼成 HTML。
"use strict";

const byId = (id) => document.getElementById(id);
const labels = {pending:"等待执行", running:"正在诊断", waiting_approval:"等待人工审批", succeeded:"报告已提交", failed:"执行失败", cancelled:"已取消", budget_exceeded:"预算已用尽"};
const state = {csrf:"", selected:"", source:null, detail:null, steps:new Map(), page:1, loggedIn:false, refreshing:false};

// 元素内容只作为文字写入；模型返回的标签、事件属性和脚本都不会被浏览器解析。
function node(tag, text, className) {
  const value = document.createElement(tag);
  if (text !== undefined) value.textContent = String(text);
  if (className) value.className = className;
  return value;
}
function notify(message) { byId("notice").textContent = message; byId("notice").hidden = !message; }
function money(micros, currency = "CNY") { return (currency === "USD" ? "$" : "¥") + (Number(micros || 0) / 1000000).toFixed(4); }
function finished(status) { return !["pending", "running", "waiting_approval"].includes(status); }
function closeStream() { if (state.source) state.source.close(); state.source = null; }

// JWT 留在 HttpOnly Cookie，脚本只持有会话派生的 CSRF 值；地址从不带凭据。
async function api(path, options = {}) {
  const headers = {"Content-Type":"application/json", ...options.headers};
  if (options.method && options.method !== "GET") headers["X-CSRF-Token"] = state.csrf;
  const response = await fetch("/api/v1" + path, {...options, headers, credentials:"same-origin"});
  const body = await response.json();
  if (!response.ok) {
    if (response.status === 401) showLogin();
    throw new Error(body.message || "请求未完成，请稍后重试");
  }
  return body.data;
}

// 会话失效时同时关闭订阅，防止旧页面继续表现得像已登录。
function showLogin() {
  state.loggedIn = false; state.csrf = ""; closeStream();
  byId("workspace").hidden = true; byId("logout").hidden = true; byId("login-panel").hidden = false;
}

// 登录和刷新页面共用初始化，不把 access_token 写入本地存储。
async function enter() {
  const session = await api("/auth/session"); state.csrf = session.csrf_token;
  const targets = await api("/targets");
  byId("filter").replaceChildren(new Option("全部系统", "")); byId("manual-target").replaceChildren();
  for (const target of targets) {
    byId("filter").add(new Option(target.name, target.name));
    byId("manual-target").add(new Option(target.name, target.name));
  }
  state.loggedIn = true;
  byId("workspace").hidden = false; byId("logout").hidden = false; byId("login-panel").hidden = true;
  notify(""); await refresh();
}

// 刷新合并并发触发，慢请求不会让计时器不断叠加新的请求。
async function refresh() {
  if (!state.loggedIn || state.refreshing) return;
  state.refreshing = true;
  try { await Promise.all([refreshStatus(), refreshList(), refreshAlerts(), refreshApprovals()]); }
  catch (error) { notify(error.message); }
  finally { state.refreshing = false; }
}

// 报告结束后 SSE 会关闭，告警却可能晚到；只刷新告警区，不重读模型审计或重建报告。
async function refreshAlerts() {
  const id = state.selected;
  if (!id || !state.detail || state.detail.id !== id) return;
  const history = await api("/diagnoses/" + encodeURIComponent(id) + "/alerts");
  if (state.selected !== id || state.detail.id !== id) return;
  state.detail.alerts = history.alerts; state.detail.recovery_alerts = history.recovery_alerts;
  renderAlerts();
}

// 后续告警有独立标题和到达时间；即使正文包含 HTML，仍然只显示为文字。
function renderAlerts() {
  const area = byId("alert-history"); if (!area) return;
  const detail = state.detail; area.replaceChildren();
  const initial = node("details", undefined, "step");
  initial.append(node("summary", "初始症状与最新告警"), node("pre", JSON.stringify({input:detail.input, alerts:detail.alerts}, null, 2))); area.append(initial);
  const followups = node("section", undefined, "step"); followups.append(node("h3", "恢复观察期内的后续告警"));
  followups.append(node("p", "这些症状归入原诊断，不会重新执行报告。观察期内若发生了新的独立故障，可以手动发起诊断。", "muted"));
  for (const item of detail.recovery_alerts || []) {
    followups.append(node("p", new Date(item.received_at).toLocaleString() + " · " + item.alert.labels.alertname + " · " + item.alert.status), node("pre", JSON.stringify(item.alert, null, 2)));
  }
  if (!(detail.recovery_alerts || []).length) followups.append(node("p", "尚无后续告警。", "muted"));
  area.append(followups);
}

// 页面清楚区分固定脚本与真实模型，预算不足的原因直接采用服务器读数。
async function refreshStatus() {
  const value = await api("/status");
  byId("mode-banner").hidden = value.provider !== "mock";
  byId("mode-banner").textContent = "Mock 模式，不是真实诊断。当前使用固定脚本：" + value.model;
  byId("budget-banner").hidden = !value.budget.blocked;
  byId("budget-banner").textContent = value.budget.reason || "全局费用额度不足，真实诊断已停止启动。";
  byId("cost").replaceChildren(node("strong", money(value.budget.spent_micros,value.budget.currency) + " / " + money(value.budget.limit_micros,value.budget.currency)), node("div", "当前账户结算费用 · 含未知和未结算预留额", "muted"));
  byId("manual-submit").disabled = value.provider !== "mock" && value.budget.blocked;
}

// 列表按系统分页，旧任务仍可访问；标题、系统名都作为纯文字展示。
async function refreshList() {
  const filter = byId("filter").value;
  const result = await api("/diagnoses?page_size=20&page=" + state.page + "&target=" + encodeURIComponent(filter));
  const list = byId("diagnosis-list"); list.replaceChildren();
  for (const item of result.items) {
    const button = node("button", undefined, "diagnosis-item" + (item.id === state.selected ? " selected" : ""));
    button.append(node("strong", item.title), node("small", item.target + " · " + (labels[item.status] || item.status)), node("small", new Date(item.created_at).toLocaleString()));
    button.addEventListener("click", () => selectDiagnosis(item.id).catch(error => notify(error.message)));
    list.append(button);
  }
  if (!result.items.length) list.append(node("p", "这一页还没有诊断记录。", "muted"));
  const pager = node("div", undefined, "section-title");
  const previous = node("button", "上一页", "quiet"); previous.disabled = state.page === 1;
  const next = node("button", "下一页", "quiet"); next.disabled = result.items.length < 20;
  previous.addEventListener("click", () => { state.page--; refresh(); });
  next.addEventListener("click", () => { state.page++; refresh(); });
  pager.append(previous, node("small", "第 " + state.page + " 页"), next); list.append(pager);
}

// 切换任务先关旧订阅。旧请求回来时再次比对 ID，不能把别的任务画到当前详情。
async function selectDiagnosis(id) {
  closeStream(); state.selected = id;
  const detail = await api("/diagnoses/" + encodeURIComponent(id));
  if (state.selected !== id) return;
  state.detail = detail; state.steps = new Map(detail.steps.map(step => [step.seq, step]));
  renderDetail(); refreshApprovals().catch(error => notify(error.message)); refreshList().catch(error => notify(error.message));
  if (!finished(detail.status)) subscribe(id);
}

// EventSource 自动重连时携带 Last-Event-ID；after 只用于首次接续已加载的步骤号。
function subscribe(id) {
  const after = Math.max(0, ...state.steps.keys());
  const source = new EventSource("/api/v1/diagnoses/" + encodeURIComponent(id) + "/events?after=" + after);
  state.source = source;
  source.addEventListener("step", event => {
    if (state.selected !== id || state.source !== source) return;
    try {
      const step = JSON.parse(event.data);
      if (state.steps.has(step.seq)) return;
      state.steps.set(step.seq, step); appendStep(step); updateProgress();
      if (step.kind === "approval") refreshApprovals().catch(error => notify(error.message));
      if (step.kind === "system" && labels[step.status]) {
        state.detail.status = step.status;
        byId("diagnosis-state").textContent = labels[step.status] || step.status;
        // pending 行尚未绑定实际模型；开始执行后重读一次，不能把迁移默认值误标成 Mock。
        if (step.status === "running" && !state.detail.model) { selectDiagnosis(id).catch(error => notify(error.message)); return; }
        if (finished(step.status)) { closeStream(); selectDiagnosis(id).catch(error => notify(error.message)); refresh(); }
      }
    } catch { notify("实时步骤格式异常，请刷新详情。"); }
  });
  source.addEventListener("status", () => { if (state.source === source) { closeStream(); selectDiagnosis(id).catch(error => notify(error.message)); } });
  source.onerror = () => {
    if (state.source === source) byId("connection-state").textContent = "连接暂时中断，正在自动补发缺失步骤……";
  };
  source.onopen = () => { if (state.source === source) byId("connection-state").textContent = "实时连接已建立"; };
}

// 每次完整读取才重建详情；实时事件只追加一步，不打断用户正在展开阅读的工具结果。
function renderDetail() {
  const detail = state.detail; const area = byId("detail"); area.replaceChildren();
  const head = node("div", undefined, "detail-head"); const title = node("div");
  title.append(node("p", detail.target + " / " + (detail.trigger === "alert" ? "告警触发" : "手动发起"), "eyebrow"), node("h2", detail.title));
  const badge = node("span", labels[detail.status] || detail.status, "pill"); badge.id = "diagnosis-state"; title.append(badge); head.append(title);
  if (!finished(detail.status)) {
    const cancel = node("button", "取消诊断", "danger-button");
    cancel.addEventListener("click", async () => {
      cancel.disabled = true;
      try { await api("/diagnoses/" + detail.id + "/cancel", {method:"POST", body:"{}"}); notify("已请求取消，正在等待在途操作结束。"); }
      catch (error) { notify(error.message); cancel.disabled = false; }
    }); head.append(cancel);
  }
  area.append(head);
  if (detail.model && (detail.provider === "mock" || detail.model.startsWith("mock"))) area.append(node("p", "Mock 模式，不是真实诊断。", "banner"));
  const progress = node("p", "", "metadata"); progress.id = "diagnosis-progress"; area.append(progress); updateProgress();
  if (detail.error) area.append(node("p", detail.error, "banner danger"));
  if (detail.report) renderReport(area, detail.report);
  const connection = node("p", finished(detail.status) ? "诊断已结束，以下为保存的完整过程。" : "正在连接实时步骤……", "muted"); connection.id = "connection-state"; area.append(connection);
  const alerts = node("div"); alerts.id = "alert-history"; area.append(alerts); renderAlerts();
  const approvals = node("div"); approvals.id = "approvals"; area.append(approvals);
  const steps = node("div", undefined, "steps"); steps.id = "steps"; steps.append(node("h3", "诊断步骤")); area.append(steps);
  for (const step of [...state.steps.values()].sort((a,b) => a.seq-b.seq)) appendStep(step);
}

// 报告里的建议仍只显示。只有已保存的审批单才有批准按钮，不能直接执行报告文字。
function renderReport(area, report) {
  const block = node("section", undefined, "report"); block.append(node("h3", "诊断报告"), node("p", report.summary));
  block.append(node("p", "组件：" + report.root_cause.component + " · 置信度：" + Math.round(report.root_cause.confidence * 100) + "%"), node("p", report.root_cause.mechanism));
  const evidence = node("ol", undefined, "evidence");
  for (const item of report.evidence) evidence.append(node("li", "步骤 " + item.step + "：" + item.observation));
  block.append(evidence, node("h3", "建议 · 需人工核对"));
  for (const action of report.suggested_actions) block.append(node("p", action.tool + " · " + JSON.stringify(action.args) + "\n风险：" + action.risk));
  if (report.unknowns) block.append(node("h3", "尚未确认"), node("p", report.unknowns));
  area.append(block);
}

// 步骤里的实际模型与任务初始模型不一致时明确标出来，不能默默混成同一种模型。
function appendStep(step) {
  const block = node("details", undefined, "step");
  const title = "#" + step.seq + " · " + ({llm:"模型调用",tool:"工具结果",approval:"人工审批",system:"状态变化"}[step.kind] || step.kind) + (step.tool_name ? " / " + step.tool_name : "") + (step.kind === "llm" ? " / " + step.model + (state.detail.model && step.model !== state.detail.model ? "（与初始模型不同）" : "") : "") + " · " + step.status + " · " + step.duration_ms + " ms";
  block.append(node("summary", title));
  if (step.kind === "llm") {
    block.append(node("p", "实际模型：" + step.model + " · " + money(step.cost_micros,step.currency)));
    if (state.detail.model && step.model !== state.detail.model) block.append(node("p", "本步模型与诊断初始模型不同", "warning"));
  }
  block.append(node("h3", "发送内容（已脱敏）"), node("pre", JSON.stringify(step.args, null, 2)), node("h3", "返回内容（已脱敏）"));
  let result = step.result; try { result = JSON.stringify(JSON.parse(result), null, 2); } catch { /* 普通错误文字原样显示。 */ }
  block.append(node("pre", result)); byId("steps").append(block);
}

// 运行中也按已保存步骤更新计数；不等终态才显示轮数，也不把未完成的请求算进 token。
function updateProgress() {
  const detail = state.detail;
  const steps = [...state.steps.values()];
  const models = steps.filter(step => step.kind === "llm");
  const views = steps.filter(step => step.kind === "tool" && step.tool_name !== "submit_report");
  const total = (field) => models.reduce((sum, step) => sum + Number((step.usage || {})[field] || 0), 0);
  const cost = models.reduce((sum, step) => sum + Number(step.cost_micros || 0), 0);
  byId("diagnosis-progress").textContent = "模型：" + (detail.model || "尚未开始") + " · 提示版本：" + (detail.prompt_version || "—") + " · 已保存 " + models.length + " 轮模型 / " + views.length + " 份工具结果 · 输入 " + total("prompt_tokens") + " / 输出 " + total("completion_tokens") + " token · " + money(cost,detail.currency);
}

// 所有事件处理都收敛到统一 API，表单提交失败时给用户可见反馈。
byId("login-form").addEventListener("submit", async event => {
  event.preventDefault(); const form = new FormData(event.target); const button = event.target.querySelector("button"); button.disabled = true;
  try { const value = await api("/auth/login", {method:"POST", body:JSON.stringify({email:form.get("email"),password:form.get("password")})}); state.csrf=value.csrf_token; event.target.reset(); await enter(); }
  catch (error) { notify(error.message); } finally { button.disabled=false; }
});
byId("manual-form").addEventListener("submit", async event => {
  event.preventDefault(); const form = new FormData(event.target); const button = byId("manual-submit"); button.disabled = true;
  try { const value = await api("/diagnoses", {method:"POST", body:JSON.stringify({target:form.get("target"),symptoms:form.get("symptoms")})}); notify(""); await selectDiagnosis(value.id); await refresh(); }
  catch (error) { notify(error.message); } finally { await refreshStatus().catch(() => {}); }
});
byId("logout").addEventListener("click", async () => { try { await api("/auth/logout", {method:"POST", body:"{}"}); showLogin(); } catch (error) { notify(error.message); } });
byId("filter").addEventListener("change", () => { state.page=1; refresh(); });
byId("refresh").addEventListener("click", refresh);
setInterval(refresh, 10000);
enter().catch(() => showLogin());

// 每张单展示数据库里的确切计划；按钮不携带参数，避免审阅后被另一份值替换。
async function refreshApprovals() {
  const id = state.selected; if (!id || !state.detail || state.detail.id !== id || !byId("approvals")) return;
  const approvals = await api("/diagnoses/" + encodeURIComponent(id) + "/approvals");
  if (state.selected !== id || !byId("approvals")) return;
  const area = byId("approvals"); area.replaceChildren();
  if (!approvals.length) return;
  area.append(node("h3", "人工审批"));
  const names = {pending:"待审批", approved:"已批准，等待执行", executing:"执行中或正在核对断点", rejected:"已拒绝", expired:"已过期", executed:"执行完成", failed:"执行未确认或失败"};
  for (const item of approvals) {
    const card = node("section", undefined, "step");
    card.append(node("h3", item.tool_name + " · " + (names[item.status] || item.status)), node("p", "审批编号：" + item.id), node("pre", item.plan.preview));
    card.append(node("p", "理由：" + item.reason), node("p", "证据步骤：" + item.evidence_steps.join("、")), node("p", "预期效果：" + item.expected_effect), node("p", "风险：" + item.risk));
    card.append(node("h4", "撤销方法（由人执行）"), node("pre", item.plan.rollback), node("p", "过期时间：" + new Date(item.expires_at).toLocaleString()));
    if (item.tool_name === "create_index") card.append(node("p", "取消医生请求不保证 MySQL 已停止建索引；执行不确定时请先核对真实状态。", "warning"));
    if (item.decided_by) card.append(node("p", "决定人：" + item.decided_by + " · " + new Date(item.decided_at).toLocaleString()));
    if (item.result && item.result.summary) card.append(node("pre", JSON.stringify(item.result, null, 2)));
    if (item.status === "pending" && !finished(state.detail.status)) {
      const buttons = node("div", undefined, "section-title");
      for (const [action, label] of [["approve", "批准此具体操作"], ["reject", "拒绝"]]) {
        const button = node("button", label, action === "approve" ? "danger-button" : "quiet");
        button.addEventListener("click", async () => {
          for (const control of buttons.querySelectorAll("button")) control.disabled = true;
          try { await api("/approvals/" + item.id + "/" + action, {method:"POST", body:"{}"}); notify("决定已保存，诊断将继续。"); await refreshApprovals(); }
          catch (error) { notify(error.message); await refreshApprovals(); }
        }); buttons.append(button);
      } card.append(buttons);
    }
    area.append(card);
  }
}
