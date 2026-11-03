// 本文件：在宿主机为阶段9每份诊断保存Collector窗口，不把Docker权限交给医生。
//
// 开始证据先于诊断保存；结束后独占写原始日志、核对容器和日志保留范围，再允许评分。
// dropped_items是失败请求的报告和，不是去重丢失数；重试与最终丢弃分开处理。
//
// 主要内容：beginCollectorWindow保存起点；finish保存原件与计数；checkCollectorSummary拦截异常。

package runner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
)

// 只审计本阶段固定容器；大小与时间上限避免故障时把宿主机内存耗尽。
const (
	collectorContainer      = "opsdoctor-otel-demo-otel-collector"
	collectorMaximumBytes   = 64 << 20
	collectorMaximumLine    = 8 << 20
	collectorCommandTimeout = 30 * time.Second
	collectorClockMargin    = time.Second // 覆盖宿主与Docker时钟的小偏差，不把边界记录遗漏成正常。
)

// collectorCommand是测试替身边界；生产实现只有固定Docker查看命令，不接受shell。
type collectorCommand func(context.Context, ...string) ([]byte, error)

// collectorIdentity只保存核对需要的身份，inspect里的环境和配置不写入证据。
type collectorIdentity struct {
	ID       string    `json:"id"`       // 容器重建后即使名称相同也不同。
	Started  time.Time `json:"started"`  // Collector进程必须在整个诊断窗口前已运行。
	Restarts int       `json:"restarts"` // 名称和ID不变时也要识别进程重启。
	Running  bool      `json:"running"`  // 停止的采集器不能提供完整窗口证明。
}

// collectorWindow的起点文件在任何模型任务创建前独占保存，崩溃恢复不刷新开始时间。
type collectorWindow struct {
	Key      string            `json:"key"`      // 完整槽位身份，不依赖文件名猜测。
	Start    time.Time         `json:"start"`    // 包含任务排队及诊断执行的保守起点。
	Identity collectorIdentity `json:"identity"` // 诊断前身份。
	Anchor   string            `json:"anchor"`   // 起点前最后一条仍保留的Docker日志。
	Prefix   string            `json:"-"`        // 所有证据位于宿主Temp，不挂给医生。
	run      collectorCommand  // 测试可以模拟命令失败、日志轮转和重启。
}

// collectorOutput限制查看命令的输出；达到上限就失败，不把截断内容当作完整证据。
type collectorOutput struct{ bytes.Buffer }

// Write让os/exec把两个输出流送进同一个有界缓冲，原始字节不做重编码。
func (output *collectorOutput) Write(data []byte) (int, error) {
	if len(data) > collectorMaximumBytes-output.Len() {
		return 0, fmt.Errorf("Collector查看输出超过64MiB")
	}
	return output.Buffer.Write(data)
}

// runCollectorCommand始终使用参数数组；stderr也保留，命令失败不会冒充空成功。
func runCollectorCommand(parent context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, collectorCommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", args...)
	var output collectorOutput
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	return output.Bytes(), err
}

// inspectCollector只接受固定容器的最小身份，拒绝空数组与未运行状态。
func inspectCollector(ctx context.Context, run collectorCommand) (collectorIdentity, error) {
	var value collectorIdentity
	raw, err := run(ctx, "inspect", "--format", `{"id":{{json .Id}},"started":{{json .State.StartedAt}},"restarts":{{json .RestartCount}},"running":{{json .State.Running}}}`, collectorContainer)
	if err != nil || json.Unmarshal(raw, &value) != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(value.ID) || value.Started.IsZero() || !value.Running || value.Restarts < 0 {
		return value, fmt.Errorf("不能核对固定Collector运行身份")
	}
	return value, nil
}

// collectorLineTime只认Docker提供的首个RFC3339时间，不把消息内日期当窗口边界。
func collectorLineTime(line string) (time.Time, string, error) {
	stamp, body, ok := strings.Cut(line, " ")
	value, err := time.Parse(time.RFC3339Nano, stamp)
	if !ok || err != nil || body == "" {
		return time.Time{}, "", fmt.Errorf("Collector日志缺少可解析的Docker时间戳")
	}
	return value, body, nil
}

// beginCollectorWindow先核对可保留的锚点；没有证据时在创建付费任务前停止。
func beginCollectorWindow(ctx context.Context, key, existingDiagnosis, root string, now time.Time, run collectorCommand) (*collectorWindow, error) {
	// 槽位名只参与哈希；已有起点必须沿用，不能因进程重启而换成更短的窗口。
	hash := sha256.Sum256([]byte(key))
	prefix := filepath.Join(root, "opsdoctor-phase09", "collector-audit", hex.EncodeToString(hash[:]))
	value := &collectorWindow{Key: key, Start: now.UTC(), Prefix: prefix, run: run}
	if raw, err := os.ReadFile(prefix + ".start.json"); err == nil {
		if len(raw) > 1<<20 || json.Unmarshal(raw, value) != nil || value.Key != key || value.Start.IsZero() || value.Anchor == "" || !value.Identity.Started.Before(value.Start) {
			return nil, fmt.Errorf("Collector原始起点证明损坏，不能刷新窗口")
		}
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("无法读取Collector窗口起点证明")
	}
	if existingDiagnosis != "" {
		return nil, fmt.Errorf("已有诊断缺少原Collector起点证明，不能事后创建正常窗口")
	}
	identity, err := inspectCollector(ctx, run)
	if err != nil || !identity.Started.Before(value.Start) {
		return nil, fmt.Errorf("Collector未在诊断窗口前持续运行：%v", err)
	}
	value.Identity = identity
	// Docker先取tail再按until过滤；只取末行会在新日志晚于起点时误得空集。
	// 固定日志轮转限制保留量，外层仍有64MiB硬上限；从全部保留行选择起点前最近一条。
	raw, err := run(ctx, "logs", "--timestamps", "--until", value.Start.Format(time.RFC3339Nano), "--tail", "all", collectorContainer)
	if err != nil {
		return nil, fmt.Errorf("无法取得Collector起点日志锚点")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("Collector起点前无保留日志，无法证明窗口覆盖")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		line = strings.TrimSuffix(line, "\r")
		stamp, _, parseErr := collectorLineTime(line)
		if parseErr != nil || stamp.After(value.Start) {
			return nil, fmt.Errorf("Collector起点锚点时间无法验证")
		}
		if value.Anchor == "" {
			value.Anchor = line
		} else if prior, _, _ := collectorLineTime(value.Anchor); stamp.After(prior) {
			value.Anchor = line
		}
	}
	if value.Anchor == "" {
		return nil, fmt.Errorf("Collector无保留日志，无法证明窗口覆盖")
	}
	encoded, _ := json.Marshal(value)
	if err = writeCollectorEvidence(prefix+".start.json", encoded); err != nil {
		return nil, err
	}
	return value, nil
}

// writeCollectorEvidence只独占创建；原始文件即使不完整也保留，绝不删掉后重新采一份。
func writeCollectorEvidence(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("Collector证据不能独占保存，原件未覆盖：%w", err)
	}
	_, err = file.Write(raw)
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}

// finish先保存实际输出，再检查解析、容器身份和锚点；任一项失败都保留unknown证据。
func (window *collectorWindow) finish(ctx context.Context, end time.Time) *report.CollectorAudit {
	audit := &report.CollectorAudit{WindowStart: window.Start, WindowEnd: end.UTC(), ContainerID: window.Identity.ID, Status: "failed", RawPath: window.Prefix + ".log", SignalDroppedItems: map[string]int64{}, SignalDroppingDataEvents: map[string]int64{}}
	// 成功或失败的旧审计都直接复用，不用新窗口覆盖曾经发生的丢弃。
	if raw, err := os.ReadFile(window.Prefix + ".audit.json"); err == nil {
		var prior report.CollectorAudit
		if json.Unmarshal(raw, &prior) == nil && prior.WindowStart.Equal(window.Start) && prior.ContainerID == window.Identity.ID {
			if err = verifyCollectorRaw(&prior); err == nil {
				return &prior
			}
		}
		audit.Error = "Collector原审计或原始文件无法验证"
		return audit
	}
	raw, commandErr := window.run(ctx, "logs", "--timestamps", "--since", window.Start.Format(time.RFC3339Nano), "--until", audit.WindowEnd.Format(time.RFC3339Nano), collectorContainer)
	saveErr := writeCollectorEvidence(audit.RawPath, raw)
	if saveErr != nil {
		// 崩溃留下的部分原件不能覆写；只计算现在磁盘上已有的字节。
		raw, _ = os.ReadFile(audit.RawPath)
	}
	hash := sha256.Sum256(raw)
	audit.RawBytes, audit.SHA256 = int64(len(raw)), hex.EncodeToString(hash[:])
	parseErr := parseCollectorWindow(raw, audit)
	identity, inspectErr := inspectCollector(ctx, window.run)
	if inspectErr == nil && identity != window.Identity {
		inspectErr = fmt.Errorf("Collector在诊断期间重启或被替换")
	}
	anchorTime, _, anchorErr := collectorLineTime(window.Anchor)
	if anchorErr == nil {
		retained, err := window.run(ctx, "logs", "--timestamps", "--since", anchorTime.Format(time.RFC3339Nano), "--until", window.Start.Format(time.RFC3339Nano), collectorContainer)
		if err != nil || !strings.Contains(strings.ReplaceAll(string(retained), "\r\n", "\n"), window.Anchor+"\n") && strings.TrimSuffix(strings.ReplaceAll(string(retained), "\r\n", "\n"), "\n") != window.Anchor {
			anchorErr = fmt.Errorf("Collector起点锚点已不保留，无法排除日志轮转缺口")
		}
	}
	if !audit.WindowEnd.After(audit.WindowStart) {
		parseErr = fmt.Errorf("Collector诊断窗口时间无效")
	}
	if problem := errors.Join(commandErr, saveErr, parseErr, inspectErr, anchorErr); problem != nil {
		audit.Error = problem.Error()
	} else {
		audit.Status = "ok"
	}
	encoded, _ := json.Marshal(audit)
	if err := writeCollectorEvidence(window.Prefix+".audit.json", encoded); err != nil {
		audit.Status, audit.Error = "failed", "Collector审计元数据未能独占保存"
	}
	return audit
}

// parseCollectorWindow只计顶层最终丢弃，不把Will retry或error字符串中的伪事件重复计入。
func parseCollectorWindow(raw []byte, audit *report.CollectorAudit) error {
	if len(raw) == 0 {
		return fmt.Errorf("Collector窗口输出为空，不能证明采集正常")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), collectorMaximumLine)
	stackFunction := regexp.MustCompile(`^[A-Za-z0-9_.@/\-]+\.[^\s]+$`)
	stackSource := regexp.MustCompile(`\.go:[0-9]+$`)
	standardLog := regexp.MustCompile(`^[0-9]{4}/[0-9]{2}/[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2} `)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		stamp, body, err := collectorLineTime(line)
		if err != nil {
			return err
		}
		if stamp.Before(audit.WindowStart) || stamp.After(audit.WindowEnd) {
			return fmt.Errorf("Collector输出包含窗口外时间")
		}
		jsonAt := strings.Index(body, "\t{")
		if jsonAt < 0 {
			// 固定Collector的错误堆栈也逐行带Docker时间，但不是另一个导出失败事件。
			trimmed := strings.TrimSpace(body)
			if stackFunction.MatchString(trimmed) || strings.HasPrefix(body, "\t") && stackSource.MatchString(trimmed) {
				continue
			}
			// 固定版OTLP健康探测也会输出Go标准日志；其中若出现目标错误词则不能忽略。
			if standardLog.MatchString(body) && !strings.Contains(body, "Dropping data") && !strings.Contains(body, "mapper_parsing_exception") {
				continue
			}
			return fmt.Errorf("Collector窗口出现无法解释的日志格式")
		}
		// Zap可能省略caller，也可能带logger名；最后一列始终是消息，不靠固定列号计数。
		parts := strings.Split(body[:jsonAt], "\t")
		if len(parts) < 3 || len(parts) > 5 {
			return fmt.Errorf("Collector结构化日志列数无法确认")
		}
		if _, err = time.Parse(time.RFC3339Nano, parts[0]); err != nil {
			return fmt.Errorf("Collector结构化日志时间无效")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(body[jsonAt+1:]), &fields) != nil {
			return fmt.Errorf("Collector结构化日志JSON无效")
		}
		var message string
		if rawError, exists := fields["error"]; exists && json.Unmarshal(rawError, &message) != nil {
			return fmt.Errorf("Collector错误字段不是可验证的字符串")
		}
		if count := strings.Count(message, "mapper_parsing_exception"); count > 0 {
			audit.MapperErrorLines++
			audit.MapperErrorOccurrences += int64(count)
		}
		message = parts[len(parts)-1]
		if !strings.HasPrefix(message, "Exporting failed.") || !strings.Contains(message, "Dropping data") {
			if _, exists := fields["dropped_items"]; exists {
				return fmt.Errorf("Collector丢弃字段的事件格式无法确认")
			}
			continue
		}
		var signal, component string
		if json.Unmarshal(fields["otelcol.signal"], &signal) != nil || signal == "" || json.Unmarshal(fields["otelcol.component.id"], &component) != nil || component == "" {
			return fmt.Errorf("Collector最终丢弃缺少信号或导出器")
		}
		count, err := strconv.ParseInt(string(fields["dropped_items"]), 10, 64)
		if err != nil || count < 0 {
			return fmt.Errorf("Collector最终丢弃项数无效")
		}
		if count > math.MaxInt64-audit.SignalDroppedItems[signal] || signal == "logs" && (component == "opensearch" || strings.HasPrefix(component, "opensearch/")) && count > math.MaxInt64-audit.LogDroppedItems {
			return fmt.Errorf("Collector最终丢弃报告和溢出")
		}
		audit.DroppingDataEvents++
		audit.SignalDroppingDataEvents[signal]++
		audit.SignalDroppedItems[signal] += count
		if signal == "logs" && (component == "opensearch" || strings.HasPrefix(component, "opensearch/")) {
			audit.LogDroppedItems += count
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Collector窗口日志未完整解析")
	}
	return nil
}

// verifyCollectorRaw在续跑评分前重新核对原件，不让删除或改写证据变成正常样本。
func verifyCollectorRaw(audit *report.CollectorAudit) error {
	if audit == nil || audit.RawPath == "" {
		return fmt.Errorf("缺少Collector原始证据")
	}
	raw, err := os.ReadFile(audit.RawPath)
	if err != nil || len(raw) > collectorMaximumBytes {
		return fmt.Errorf("Collector原始证据不可读")
	}
	hash := sha256.Sum256(raw)
	if int64(len(raw)) != audit.RawBytes || hex.EncodeToString(hash[:]) != audit.SHA256 {
		return fmt.Errorf("Collector原始证据与保存的SHA256不符")
	}
	return nil
}

// checkCollectorSummary只拦阶段9；受影响和未知样本先保存，绝不进入评分或自动补跑。
func checkCollectorSummary(ctx context.Context, value report.Summary) error {
	if ctx.Value(generalizationGuardKey{}) == nil {
		return nil
	}
	if quality := report.CollectionQuality(value); quality != report.QualityNormal {
		return fmt.Errorf("阶段9采集质量%s，摘要和原件已保留，暂停评分及下一槽", quality)
	}
	if value.WriteExecutions > 0 {
		return fmt.Errorf("阶段9存在写操作执行，暂停评分及下一槽")
	}
	return verifyCollectorRaw(value.CollectorAudit)
}

// readCollectorSummary把第一次落盘的质量决定作为续跑依据，取消后的unknown不能被后来成功洗掉。
func readCollectorSummary(path string) (json.RawMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value report.Summary
	if len(raw) > 1<<20 || json.Unmarshal(raw, &value) != nil || value.DiagnosisID == "" || value.CollectorAudit == nil {
		return nil, fmt.Errorf("Collector原诊断摘要损坏，不能重采覆盖")
	}
	return raw, nil
}

// terminalCollectorDetail在取消后短暂等真正终态，不能把仍在取消中的费用快照冒充最终结果。
func terminalCollectorDetail(ctx context.Context, load func(context.Context) (store.Detail, error)) (store.Detail, error) {
	// 只读轮询不创建任务；到期仍返回最后一次详情，摘要会标unknown并保留原始状态。
	for {
		value, err := load(ctx)
		if err != nil {
			return value, err
		}
		if value.Status == "succeeded" || value.Status == "failed" || value.Status == "cancelled" || value.Status == "budget_exceeded" {
			return value, nil
		}
		if err = waitTick(ctx, 100*time.Millisecond); err != nil {
			return value, fmt.Errorf("取消后诊断尚未到终态，已保留当前摘要：%w", err)
		}
	}
}

// cancelUnattachedCollector先取消已创建却未关联成功的任务，再读终态；不能让它在后台继续收费。
func cancelUnattachedCollector(parent context.Context, cancelDiagnosis func(context.Context) error, load func(context.Context) (store.Detail, error)) (store.Detail, error) {
	// 两个预算相互独立，也都不继承已经取消的外层；取消失败仍读取原任务以保留费用证据。
	cancelContext, stopCancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
	cancelErr := cancelDiagnosis(cancelContext)
	stopCancel()
	readContext, stopRead := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer stopRead()
	detail, readErr := terminalCollectorDetail(readContext, load)
	return detail, errors.Join(cancelErr, readErr)
}

// createCollectorTask把提交确认不确定的已知ID也当作可能已执行，先取消再返回原收费摘要。
func createCollectorTask(ctx context.Context, create func(context.Context) (string, error), cancelDiagnosis func(context.Context, string) error, load func(context.Context, string) (store.Detail, error)) (string, store.Detail, error) {
	id, err := create(ctx)
	if err == nil || id == "" {
		return id, store.Detail{}, err
	}
	// tx.Commit可能已在数据库完成却丢失确认；有ID时不能直接退出、留下付费任务。
	detail, stopErr := cancelUnattachedCollector(ctx, func(stop context.Context) error { return cancelDiagnosis(stop, id) }, func(read context.Context) (store.Detail, error) { return load(read, id) })
	return id, detail, errors.Join(err, stopErr)
}

// executeDemoDiagnosis为阶段9加宿主采集观察，诊断输入、模型身份和原评分标准都不改变。
func executeDemoDiagnosis(ctx context.Context, options command, configuration pairedConfiguration, slot store.EvalResult, expected string, repository *store.Store, client *doctorClient, output io.Writer) (store.EvalResult, error) {
	hash := sha256.Sum256([]byte(slot.Key))
	prefix := filepath.Join(os.TempDir(), "opsdoctor-phase09", "collector-audit", hex.EncodeToString(hash[:]))
	if raw, err := readCollectorSummary(prefix + ".summary.json"); err == nil {
		// 崩溃可能发生在摘要文件写完、数据库保存之前；只补保存，不重查或重跑诊断。
		var saved report.Summary
		_ = json.Unmarshal(raw, &saved)
		if slot.DiagnosisID != "" && slot.DiagnosisID != saved.DiagnosisID {
			return slot, fmt.Errorf("Collector摘要诊断身份与槽位不符")
		}
		if err = repository.AttachEvalDiagnosis(ctx, slot.Key, saved.DiagnosisID); err != nil {
			return slot, err
		}
		slot.DiagnosisID, slot.Data = saved.DiagnosisID, raw
		return slot, repository.FinishEvalResult(ctx, slot.Key, raw)
	} else if !errors.Is(err, os.ErrNotExist) {
		return slot, err
	}
	current, err := client.runtime(ctx)
	if err != nil {
		return slot, err
	}
	profile, ok := current.Executions[configuration.Profile]
	if !ok || profile.Model != configuration.Model || profile.Provider != configuration.Provider || profile.Account != configuration.Account || profile.Budget.Currency != configuration.Currency {
		return slot, fmt.Errorf("运行中的执行配置已经改变，暂停评测")
	}
	if profile.Provider != "mock" && (!options.paid || profile.Budget.Blocked || profile.Budget.LimitMicros-profile.Budget.SpentMicros < 300000) {
		return slot, fmt.Errorf("未授权真实调用或账户不能预留单次0.30上限")
	}
	window, startErr := beginCollectorWindow(ctx, slot.Key, slot.DiagnosisID, os.TempDir(), time.Now().Add(-collectorClockMargin), runCollectorCommand)
	if startErr != nil && slot.DiagnosisID == "" {
		return slot, startErr
	}
	id := slot.DiagnosisID
	var detail store.Detail
	var creationErr error
	if id == "" {
		id, detail, creationErr = createCollectorTask(ctx, func(create context.Context) (string, error) {
			return repository.Evaluation(create, store.EvaluationInput{Target: options.target, RunID: options.runID, Key: slot.Key, Group: slot.Group, ExpectedModel: configuration.Model, ExecutionProfile: configuration.Profile, Provider: configuration.Provider, Account: configuration.Account, Currency: configuration.Currency, FaultStartedAt: slot.FaultStartedAt, Symptoms: slot.Input})
		}, func(stop context.Context, knownID string) error { return client.cancelDiagnosis(stop, knownID) }, func(read context.Context, knownID string) (store.Detail, error) {
			return repository.Detail(read, knownID)
		})
		if creationErr != nil && id == "" {
			return slot, creationErr
		}
	}
	slot.DiagnosisID = id
	fmt.Fprintf(output, "位置%d：%s diagnosis=%s；宿主保存Collector窗口\n", slot.Position, slot.Key, id)
	var attachErr error
	var waitErr error
	if creationErr == nil {
		attachErr = repository.AttachEvalDiagnosis(ctx, slot.Key, id)
		if attachErr == nil {
			detail, waitErr = waitDiagnosis(ctx, id, repository, client, output)
		} else {
			// 没有进入waitDiagnosis就没有它的deferred cancel，必须在读取和持久化之前主动取消。
			detail, waitErr = cancelUnattachedCollector(ctx, func(cancel context.Context) error { return client.cancelDiagnosis(cancel, id) }, func(read context.Context) (store.Detail, error) { return repository.Detail(read, id) })
		}
	}
	// 取消后仍留独立时间读取终态；它不能再创建模型任务或重试诊断。
	if detail.ID == "" {
		detailContext, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		detail, err = terminalCollectorDetail(detailContext, func(read context.Context) (store.Detail, error) { return repository.Detail(read, id) })
		stop()
		waitErr = errors.Join(waitErr, err)
	}
	value := report.Summarize(detail, expected)
	if value.DiagnosisID == "" {
		value.DiagnosisID, value.Model, value.Currency = id, configuration.Model, configuration.Currency
	}
	if window == nil {
		window = &collectorWindow{Key: slot.Key, Start: detail.CreatedAt, Prefix: prefix, run: runCollectorCommand}
		if window.Start.IsZero() {
			window.Start = slot.FaultStartedAt
		}
	}
	end := time.Now().UTC().Add(collectorClockMargin)
	// 等到保守终点以后再取日志，避免--until指定未来时刻却立即返回，留下未观察尾部。
	auditContext, stopAudit := context.WithTimeout(context.WithoutCancel(ctx), 3*collectorCommandTimeout)
	boundaryErr := waitTick(auditContext, collectorClockMargin)
	value.CollectorAudit = window.finish(auditContext, end)
	stopAudit()
	if detail.CreatedAt.Before(window.Start) {
		startErr = errors.Join(startErr, fmt.Errorf("诊断早于原采集起点，窗口不能覆盖完整执行"))
	}
	if startErr != nil {
		value.CollectorAudit.Status, value.CollectorAudit.Error = "failed", startErr.Error()
	}
	value.DataQuality = report.QualityNormal
	value.DataQuality = report.CollectionQuality(value)
	// Docker取证用完自己的预算后，持久化仍有新的独立时间，不能继承已取消的上下文。
	saveContext, stopSave := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stopSave()
	var recoveryAttachErr error
	if creationErr != nil || attachErr != nil {
		// 只重试关联这个已取消的原ID，不创建任务；提交其实未完成时失败证据仍在Temp保留。
		recoveryAttachErr = repository.AttachEvalDiagnosis(saveContext, slot.Key, id)
	}
	safetyErr := safetySummary(saveContext, repository, detail, &value)
	value.ReliabilityFailure, err = repository.EvalReliability(saveContext, id)
	problem := errors.Join(creationErr, attachErr, recoveryAttachErr, waitErr, safetyErr, boundaryErr, err)
	if value.WriteExecutions > 0 {
		problem = errors.Join(problem, fmt.Errorf("评测发现写操作执行记录，停止并需要人工检查基线"))
	}
	if problem != nil && value.DataQuality == report.QualityNormal {
		value.DataQuality = report.QualityUnknown
		value.CollectorAudit.Status, value.CollectorAudit.Error = "failed", "诊断等待或完整审计未能确认，不能自动续跑"
	}
	slot.Data, _ = json.Marshal(value)
	// 数据库或取消收尾失败时，宿主仍有独占摘要；原始报告与数据质量不会只存在内存里。
	backupErr := writeCollectorEvidence(window.Prefix+".summary.json", slot.Data)
	if backupErr != nil {
		prior, readErr := os.ReadFile(window.Prefix + ".summary.json")
		if readErr == nil && equalJSON(prior, slot.Data) {
			backupErr = nil
		}
	}
	saveErr := repository.FinishEvalResult(saveContext, slot.Key, slot.Data)
	fmt.Fprintf(output, "%s status=%s data_quality=%s collector_raw=%s sha256=%s\n", slot.Key, value.Status, value.DataQuality, value.CollectorAudit.RawPath, value.CollectorAudit.SHA256)
	return slot, errors.Join(problem, backupErr, saveErr)
}
