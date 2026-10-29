// 本文件：证明评测页面只能读取已保存的结果，并且需要登录。
//
// 主要内容：TestEvaluationEndpoints 检查身份、原始评分和非法批次；evalView
// 用虚构记录替代数据库，不运行模型或实验。
// 阅读建议：评分失败也要原样到达页面，HTTP 层不能把它修成成功。

package httpapi

import (
	"context"
	"encoding/json"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"testing"
)

// evalView 提供固定公开记录，不接触真实医生账本。
type evalView struct{}

// EvalRuns 返回一个批次，供选择框使用。
func (evalView) EvalRuns(context.Context) ([]store.EvalRun, error) {
	return []store.EvalRun{{ID: "test"}}, nil
}

// EvalRun 返回选中批次，不提供修改状态的方法。
func (evalView) EvalRun(context.Context, string) (store.EvalRun, error) {
	return store.EvalRun{ID: "test"}, nil
}

// EvalResults 用脚本文字检查接口仍然传数据，页面只能用 textContent 展示。
func (evalView) EvalResults(context.Context, string) ([]store.EvalResult, error) {
	return []store.EvalResult{{Input: json.RawMessage(`{"private":"large input"}`), Judgement: json.RawMessage(`{"status":"failed","error":"<script>untrusted</script>"}`)}}, nil
}

// TestEvaluationEndpoints 不因结果只读而放开身份边界，也不提供随意改分的 POST。
func TestEvaluationEndpoints(t *testing.T) {
	router, _, login := setupWithOptions(t, func(options *Options) { options.Evaluations = evalView{} })
	// 读取需要登录；相同地址没有写路由，不能修改评分。
	for _, path := range []string{"/api/v1/eval/runs", "/api/v1/eval/runs/test"} {
		if apiCall(router, "GET", path, "", nil).Code != 401 {
			t.Fatal("匿名读取评测")
		}
		if apiCall(router, "GET", path, login, nil).Code != 200 {
			t.Fatal("合法读取失败")
		}
		if apiCall(router, "POST", path, login, []byte(`{}`)).Code != 404 {
			t.Fatal("出现未授权写入口")
		}
	}
	var result struct {
		Data struct {
			Items []store.EvalResult `json:"items"`
		} `json:"data"`
	}
	response := apiCall(router, "GET", "/api/v1/eval/runs/test", login, nil)
	if json.Unmarshal(response.Body.Bytes(), &result) != nil || len(result.Data.Items) != 1 || string(result.Data.Items[0].Input) != "null" {
		t.Fatal("没有按约定去掉重复大输入")
	}
	var grade map[string]any
	if json.Unmarshal(result.Data.Items[0].Judgement, &grade) != nil || grade["status"] != "failed" || grade["error"] != "<script>untrusted</script>" {
		t.Fatal("接口改写了评分")
	}
	if apiCall(router, "GET", "/api/v1/eval/runs/bad!", login, nil).Code != 400 {
		t.Fatal("接受非法批次")
	}
}
