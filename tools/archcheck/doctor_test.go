// 本文件：证明容器不能通过部署配置拿到故障答案或 Docker 控制权。
//
// 主要内容：
//   - TestDoctorMounts：覆盖额外挂载、长格式挂载、整份环境和提权配置。
// 阅读建议：每个用例只加一个越界入口，避免测试因另一条规则失败而漏检。

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDoctorMounts 直接调用 archcheck 使用的检查函数，不另写一份较宽松的测试判断。
func TestDoctorMounts(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		want         bool
	}{
		{"只读接入目录", "", false}, // 正常配置保留最小文件入口。
		{"场景挂载", "    volumes: [../eval/scenarios:/answers:ro]\n", true},
		{"仓库挂载", "    volumes: [..:/repo:ro]\n", true},
		{"长格式socket", "    volumes: [{type: bind, source: /var/run/docker.sock, target: /socket}]\n", true},
		{"整份环境", "    env_file: ../.env\n", true},
		{"容器提权", "    privileged: true\n", true},
		{"实验凭据", "    environment: {CHAOS_MYSQL_DSN: demo}\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 用例重写相应字段，避免依赖 YAML 重复键的处理方式。
			base := "services:\n  doctor:\n"
			if tc.name != "场景挂载" && tc.name != "仓库挂载" && tc.name != "长格式socket" {
				base += "    volumes: ['./doctor/targets:/etc/doctor/targets:ro']\n"
			}
			if tc.name != "实验凭据" {
				base += "    environment: {TARGETS_DIR: /etc/doctor/targets}\n"
			}
			path := filepath.Join(t.TempDir(), "compose.yml")
			if err := os.WriteFile(path, []byte(base+tc.fields), 0600); err != nil {
				t.Fatal(err)
			}
			issues, err := doctorMountProblems(path)
			if err != nil || (len(issues) > 0) != tc.want {
				t.Fatalf("issues=%v err=%v", issues, err)
			}
		})
	}
}
