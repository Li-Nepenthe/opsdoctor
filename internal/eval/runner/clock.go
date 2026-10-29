// 本文件：在评测花钱前和执行期间检查医生容器的时钟是否跳变。
//
// 日期突然快进或倒退，不只会让耗时变负，还可能影响告警窗口和数据库租约。
// 所以有单调计时仍不够：实验环境本身的墙钟也必须稳定。
//
// 主要内容：checkClock 检查连续六十个一秒步长；inspectClock 可同时保存原始样本；clockSamples 验证流式样本；
// guardClock 持续重复检查，发现跳变便取消正在等待的诊断。
// 阅读建议：所有 Docker 命令在宿主机评测器执行，不给医生 Docker 权限。

package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const clockSampleCount = 60 // 用户要求连续检查一分钟；只看一次当前日期无法发现来回拨动。

// checkClock 在创建付费任务前完成整段检查，任何缺样或不等于一秒的步长都拦截。
func checkClock(parent context.Context) error {
	return inspectClock(parent, io.Discard)
}

// inspectClock 让独立预检和正式评测使用同一套判断，输出样本方便事后核对。
func inspectClock(parent context.Context, evidence io.Writer) error {
	// 多给十五秒用于 Docker 启动与调度。它不是允许日期跳变的容差。
	ctx, cancel := context.WithTimeout(parent, 75*time.Second)
	defer cancel()
	script := `date +%s; i=0; while [ "$i" -lt 60 ]; do sleep 1; date +%s; i=$((i+1)); done`
	command := exec.CommandContext(ctx, "docker", "compose", "--env-file", ".env", "-f", "deploy/docker-compose.yml", "exec", "-T", "doctor", "sh", "-c", script)
	output, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("无法读取容器时钟，评测暂停")
	}
	if err = command.Start(); err != nil {
		return fmt.Errorf("无法启动容器时钟检查，请检查 Docker，评测暂停")
	}
	// 一行一行检查，发生跳变立即返回，不等满一分钟才通知上层。
	// 取消只结束本机 docker 命令；容器里的只读采样最多自行运行六十秒。
	readErr := clockSamples(io.TeeReader(output, evidence))
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

// clockSamples 只接受秒级整数；错误文字或截断输出不能被当成稳定的时钟。
func clockSamples(input io.Reader) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 128), 128) // date 只应输出十余个字节，拒绝意外的大段输出。
	var previous int64
	count := 0
	for scanner.Scan() {
		current, err := strconv.ParseInt(strings.TrimSpace(scanner.Text()), 10, 64)
		if err != nil || current <= 0 || count > clockSampleCount {
			return fmt.Errorf("容器时钟样本格式或数量不合法，评测暂停")
		}
		if count > 0 && current-previous != 1 {
			return fmt.Errorf("容器时钟第 %d 次步长为 %d 秒，要求 1 秒；评测暂停，请检查 Windows 与 WSL 校时", count, current-previous)
		}
		previous = current
		count++
	}
	if scanner.Err() != nil || count != clockSampleCount+1 {
		return fmt.Errorf("容器时钟样本不完整，评测暂停")
	}
	return nil
}

// clockGuardKey 只标记本包已经启动的守卫，嵌套等待不再启动重复的 Docker 采样。
type clockGuardKey struct{}

// guardClock 每段检查结束后立即开始下一段；发生跳变时保留具体原因传给等待器。
func guardClock(parent context.Context, sample func(context.Context) error) (context.Context, func()) {
	if parent.Value(clockGuardKey{}) != nil {
		return parent, func() {}
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
