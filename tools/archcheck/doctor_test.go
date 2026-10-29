// 本文件：证明容器不能通过部署配置拿到故障答案或 Docker 控制权。
//
// 主要内容：
//   - TestDoctorMounts：覆盖额外挂载、长格式挂载、整份环境和提权配置。
//   - TestOpsInternal：覆盖短格式、长格式端口映射和主机网络的绕过入口。
// 阅读建议：每个用例只加一个越界入口，避免测试因另一条规则失败而漏检。

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDoctorMounts 直接调用 archcheck 使用的检查函数，不另写一份较宽松的测试判断。
func TestDoctorMounts(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		want         bool
	}{
		{"只读接入目录", "", false}, // 正常配置保留最小文件入口。
		{"只读手册", "    volumes: ['./doctor/targets:/etc/doctor/targets:ro','../docs/runbooks:/etc/doctor/runbooks:ro']\n", false}, // 阶段 8 的唯一新增只读资料。
		{"整份文档", "    volumes: ['./doctor/targets:/etc/doctor/targets:ro','../docs:/etc/doctor/runbooks:ro']\n", true},           // 汇报书可能含答案，不能扩大到 docs。
		{"手册可写", "    volumes: ['./doctor/targets:/etc/doctor/targets:ro','../docs/runbooks:/etc/doctor/runbooks:rw']\n", true},  // 医生不能修改冻结的手册。
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
			if !strings.Contains(tc.fields, "volumes:") {
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

// TestOpsInternal 运维端口即使只映射到 localhost 也不符合本项目的“仅内部网络”约定。
func TestOpsInternal(t *testing.T) {
	// 业务端口是正常对外入口；其余写法都试图把内部运维接口带到宿主机。
	for _, test := range []struct {
		name    string
		service map[string]any
		denied  bool
	}{
		{"只发布业务端口", map[string]any{"ports": []any{"127.0.0.1:8080:8080"}}, false},
		{"短格式运维端口", map[string]any{"ports": []any{"127.0.0.1:6064:6064"}}, true},
		{"自动分配宿主端口", map[string]any{"ports": []any{6064}}, true},
		{"长格式运维端口", map[string]any{"ports": []any{map[string]any{"target": 6064, "published": 9000}}}, true},
		{"主机网络", map[string]any{"network_mode": "host"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			issues := opsPortProblems("compose.yml", map[string]map[string]any{"inventory-service": test.service})
			if (len(issues) > 0) != test.denied {
				t.Fatalf("检查结果=%v", issues)
			}
		})
	}
}
