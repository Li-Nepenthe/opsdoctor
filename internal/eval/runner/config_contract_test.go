// 本文件：核对两组评测入口的共享预算、S12 地址和评委实际采用的时段价格。

package runner

import (
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

func evalConfigReader(values map[string]string) config.Reader {
	return config.NewReader(func(name string) (string, bool) { v, ok := values[name]; return v, ok })
}

func TestEvalTokenBudget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		values  map[string]string
		want    int
		invalid bool
	}{
		{"未配置", nil, 10_000_000, false},
		{"显式覆盖", map[string]string{"EVAL_TOKEN_BUDGET": " 741 "}, 741, false},
		{"显式空白", map[string]string{"EVAL_TOKEN_BUDGET": " "}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalTokenBudget(evalConfigReader(tc.values))
			if got != tc.want || (err != nil) != tc.invalid {
				t.Fatalf("budget=%d err=%v", got, err)
			}
		})
	}
}

func TestS12OrderURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		values  map[string]string
		want    string
		invalid bool
	}{
		{"缺省", nil, "http://127.0.0.1:8081", false},
		{"显式地址", map[string]string{"CHAOS_ORDER_URL": " http://example.invalid/test "}, "http://example.invalid/test", false},
		{"显式空白", map[string]string{"CHAOS_ORDER_URL": " "}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s12OrderURL(evalConfigReader(tc.values))
			if got != tc.want || (err != nil) != tc.invalid {
				t.Fatalf("address=%q err=%v", got, err)
			}
		})
	}
}

func TestJudgeEffectivePrices(t *testing.T) {
	// 两个固定时刻都是北京时间工作日上午十点，部署指定其中一天为节假日后，它应使用闲时单价。
	monday := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	tuesday := monday.Add(24 * time.Hour)
	night := tuesday.Add(12 * time.Hour)
	defaults := llm.Prices{InputMicros: 2_000_000, CacheHitMicros: 40_000, OutputMicros: 8_000_000}
	halfDefaults := llm.Prices{InputMicros: 1_000_000, CacheHitMicros: 20_000, OutputMicros: 4_000_000}
	overrides := map[string]string{"EVAL_COMPARISON_INPUT_PRICE": "6", "EVAL_COMPARISON_CACHE_HIT_PRICE": "0.12", "EVAL_COMPARISON_OUTPUT_PRICE": "10", "EVAL_DEEPSEEK_HOLIDAYS": "2026-11-09"}
	peak := llm.Prices{InputMicros: 6_000_000, CacheHitMicros: 120_000, OutputMicros: 10_000_000}
	half := llm.Prices{InputMicros: 3_000_000, CacheHitMicros: 60_000, OutputMicros: 5_000_000}
	for _, tc := range []struct {
		name                  string
		values                map[string]string
		base, mon, tue, night llm.Prices
	}{
		{"部署默认", nil, defaults, defaults, defaults, halfDefaults},
		{"部署价格与节假日", overrides, peak, half, peak, half},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prices, err := readJudgeTariff(evalConfigReader(tc.values))
			if err != nil {
				t.Fatal(err)
			}
			got := []llm.Prices{prices.peak, prices.at(monday), prices.at(tuesday), prices.at(night)}
			for i, want := range []llm.Prices{tc.base, tc.mon, tc.tue, tc.night} {
				if got[i] != want {
					t.Fatalf("时段 %d: got=%+v want=%+v", i, got[i], want)
				}
			}
		})
	}
}
