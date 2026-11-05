package contextusage

import (
	"strings"
	"testing"

	"github.com/whxcode/mini-gnet/views/message"
)

// TestContextSnapshot 区分最近上下文和累计消耗，覆盖缺失、未知上限和超限。
func TestContextSnapshot(t *testing.T) {
	usage := message.UsageSummary{
		TotalTokens: 100_000,
		Reported:    10,
		Last: &message.TokenUsage{
			InputTokens:   1500,
			OutputTokens:  500,
			ContextWindow: 8000,
			Model:         "test-model",
		},
	}
	if UsedText(usage) != "已用 ≈2.0k tokens" || RemainingText(usage) != "剩余 ≈6.0k tokens" {
		t.Fatal(Summary(usage))
	}
	detail := Detail("session-id", "测试会话", "空闲", usage)
	for _, want := range []string{"session-id", "测试会话", "test-model", "25.0%", "6000 tokens", "100000 tokens"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("详情缺少 %q：%s", want, detail)
		}
	}
	if strings.Contains(Summary(usage), "100.0k") {
		t.Fatal("标题不应混入累计消耗")
	}

	usage.Last.ContextWindow = 0
	if !strings.Contains(RemainingText(usage), "上限未设置") || !strings.Contains(Detail("", "", "", usage), "/config") {
		t.Fatal("未知上限必须解释原因")
	}
	if !strings.Contains(UsedText(usage), "2.0k") {
		t.Fatal("未知上限不应隐藏已用数据")
	}

	usage.Last.ContextWindow = 1000
	if RemainingText(usage) != "剩余 0 · 已超上限" {
		t.Fatal(RemainingText(usage))
	}

	usage.Last = nil
	usage.Missing = 1
	if !strings.Contains(UsedText(usage), "未上报") || RemainingText(usage) != "剩余 --" {
		t.Fatal("未上报时不能沿用旧统计")
	}
	if !strings.Contains(UsedText(message.UsageSummary{}), "尚无数据") {
		t.Fatal("新会话不能显示占用为零")
	}
}

// TestCompactUnits 验证单位切换及临界值的四舍五入。
func TestCompactUnits(t *testing.T) {
	for count, want := range map[int]string{
		0: "0", 999: "999", 1000: "1.0k", 25_500: "25.5k",
		999_949: "999.9k", 999_950: "1.0M", 1_000_000: "1.0M", 2_500_000: "2.5M",
	} {
		if got := Compact(count); got != want {
			t.Errorf("%d: got %s, want %s", count, got, want)
		}
	}
}
