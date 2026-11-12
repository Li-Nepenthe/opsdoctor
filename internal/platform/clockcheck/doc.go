// Package clockcheck 比较墙钟与单调计时的增量，区分日期跳变和采样进程调度晚到。
// 先读 Sample 和 Check，再看 Monitor。超过 100 毫秒先记录；回拨超过一秒、前跳超过五秒，或十分钟内第四次超过半秒的修正才暂停。
//
//   - sample.go：采集六十个时间步长并核对两种增量。
//   - policy.go：在跨采样段的十分钟窗口内决定记录或暂停。
package clockcheck
