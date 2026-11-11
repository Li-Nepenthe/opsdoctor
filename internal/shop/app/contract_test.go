// 本文件：从三个业务进程共同的运维监听入口核对地址和令牌的读取。

package app

import (
	"testing"
)

// TestOpsEnvironmentContract 同时传空保证装配在任何依赖或监听之前结束，也能揭示将一个变量误改名的情况。
func TestOpsEnvironmentContract(t *testing.T) {
	t.Setenv("OPS_ADDR", "")
	t.Setenv("OPS_TOKEN", "")
	closeOps, err := StartOps(t.Context(), nil, nil, nil, nil)
	if closeOps != nil || err == nil || err.Error() != "环境变量 OPS_ADDR 不能为空\n环境变量 OPS_TOKEN 不能为空" {
		t.Fatal(err)
	}
}
