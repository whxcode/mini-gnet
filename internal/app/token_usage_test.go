package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/views/message"
)

// TestContextUsageUsesLatestRequest 确认比例来自最近一轮，而不是历史累计值；未知上限不伪造百分比。
func TestContextUsageUsesLatestRequest(t *testing.T) {
	u := message.UsageSummary{
		TotalTokens: 100_000,
		Reported:    10,
		Last: &message.TokenUsage{
			InputTokens:   1500,
			OutputTokens:  500,
			ContextWindow: 8000,
		},
	}
	if got := tokenContextText(u); got != "上次上下文 ≈25.0%" {
		t.Fatal(got)
	}
	if got := tokenContextCountText(u); got != "2.0k / 8.0k tokens" {
		t.Fatal(got)
	}

	u.Last.ContextWindow = 0
	if strings.Contains(tokenContextText(u), "%") || !strings.Contains(tokenContextCountText(u), "/config") {
		t.Fatal("未知上限不能显示百分比")
	}
	u.Last = nil
	if strings.Contains(tokenContextText(u), "%") {
		t.Fatal("未上报数据不能显示旧的比例")
	}
}

// TestSessionTokenCards 验证两个会话的上下文统计独立，恢复值能进入卡片且不撑坏布局。
func TestSessionTokenCards(t *testing.T) {
	a := terminalTestApp(t)
	first := a.CurrentSession
	if err := first.History.AppendResponse("完成", nil, openai.FinishReasonStop, true, &message.TokenUsage{
		InputTokens:   1500,
		OutputTokens:  500,
		TotalTokens:   2000,
		ContextWindow: 8000,
	}); err != nil {
		t.Fatal(err)
	}

	history, err := session.NewHistory(a.config.Current())
	if err != nil {
		t.Fatal(err)
	}
	a.AttachSession(history)
	if a.CurrentSession.Snapshot().Usage.Reported != 0 || first.Snapshot().Usage.Reported != 1 {
		t.Fatal("两个会话的 token 统计混在一起")
	}

	for _, width := range []int{32, 80, 120} {
		m := newTerminal(a, context.Background())
		m = updateTerminal(m, tea.WindowSizeMsg{Width: width, Height: 40})
		m.session.Draft.SetValue("编辑中")
		view := m.View()
		plain := ansi.Strip(view.Content)
		if !strings.Contains(plain, "25.0%") || !strings.Contains(plain, "尚无用量数据") {
			t.Fatalf("%d 列窗口缺少每个会话的统计：\n%s", width, plain)
		}
		lines := strings.Split(plain, "\n")
		if len(lines) > 40 || view.Cursor == nil || !strings.Contains(lines[view.Cursor.Position.Y], "编辑中") {
			t.Fatal("增加统计后高度或光标不正确")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("%d 列窗口溢出：%s", width, line)
			}
		}
		if width == 80 {
			t.Log("\n" + plain)
		}
	}
}
