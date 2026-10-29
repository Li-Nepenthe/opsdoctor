// 本文件：在评测花钱前和执行期间检查医生容器的时钟是否跳变。
//
// 日期突然快进或倒退，不只会让耗时变负，还可能影响告警窗口和数据库租约。
// 所以有单调计时仍不够：实验环境本身的墙钟也必须稳定。
//
// 主要内容：checkClock 检查连续六十次读数；inspectClock 保存并校验原始样本；
// guardClock 持续重复检查，达到暂停门槛才取消正在等待的诊断。
// 阅读建议：所有 Docker 命令在宿主机评测器执行，不给医生 Docker 权限。

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/clockcheck"
)

// checkClock 在创建付费任务前完成整段检查；采样延迟不等同于时钟跳变。
func checkClock(parent context.Context) error {
	return inspectClock(parent, io.Discard)
}

// inspectClock 让独立预检和正式评测使用同一套判断，输出样本方便事后核对。
func inspectClock(parent context.Context, evidence io.Writer) error {
	// 多给十五秒用于 Docker 启动与调度。小修正记录，大幅或反复跳变才暂停。
	ctx, cancel := context.WithTimeout(parent, 75*time.Second)
	defer cancel()
	// 复用镜像内已有程序的纯采样入口，不给医生 Docker 权限，也不临时下载工具。
	command := exec.CommandContext(ctx, "docker", "compose", "--env-file", ".env", "-f", "deploy/docker-compose.yml", "exec", "-T", "doctor", "/app/server", "--clock-samples")
	output, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("无法读取容器时钟，评测暂停")
	}
	if err = command.Start(); err != nil {
		return fmt.Errorf("无法启动容器时钟检查，请检查 Docker，评测暂停")
	}
	// 一行一行检查，发生跳变立即返回，不等满一分钟才通知上层。
	// 取消只结束本机 docker 命令；容器里的只读采样最多自行运行七十五秒。
	monitor, ok := parent.Value(clockMonitorKey{}).(*clockcheck.Monitor)
	if !ok {
		monitor = clockcheck.NewMonitor(recordClockEvent)
	}
	readErr := monitor.Check(io.TeeReader(output, evidence))
	if readErr != nil {
		cancel()
	}
	waitErr := command.Wait()
	if readErr != nil {
		return readErr
	}
	if waitErr != nil {
		return fmt.Errorf("容器时钟检查没有完整结束，评测暂停")
	}
	return nil
}

// clockGuardKey 只标记本包已经启动的守卫，嵌套等待不再启动重复的 Docker 采样。
type clockGuardKey struct{}

// clockMonitorKey 让多段一分钟采样共享十分钟历史，嵌套 matrix 也不能把计数清零。
type clockMonitorKey struct{}

// recordClockEvent 保存允许继续的小修正；关闭文件后才返回，进程退出也不会只剩内存。
func recordClockEvent(event clockcheck.Event) error {
	file, err := os.OpenFile(".opsdoctor-clock-events.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(event)
	// 先保存在本机再输出，终端滚动丢失也仍可回填数据台账。
	if err == nil {
		_, err = file.Write(append(encoded, '\n'))
	}
	closed := file.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	fmt.Fprintf(os.Stderr, "校时事件 %s\n", encoded)
	return nil
}

// guardClock 每段检查后继续下一段，共享十分钟历史；暂停时保留具体原因。
func guardClock(parent context.Context, sample func(context.Context) error) (context.Context, func()) {
	if parent.Value(clockGuardKey{}) != nil {
		return parent, func() {}
	}
	if parent.Value(clockMonitorKey{}) == nil {
		parent = context.WithValue(parent, clockMonitorKey{}, clockcheck.NewMonitor(recordClockEvent))
	}
	ctx, cancel := context.WithCancelCause(context.WithValue(parent, clockGuardKey{}, true))
	done := make(chan struct{}) // 检查 goroutine 关闭，调用方结束时等它回收本机进程。
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			if err := sample(ctx); err != nil {
				cancel(err)
				return
			}
		}
	}()
	return ctx, func() { cancel(nil); <-done }
}
