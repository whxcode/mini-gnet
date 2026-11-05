package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/internal/command"
	"github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/views/message"
)

// TestContextCommandFollowsSession 验证命令跟随当前会话，不跨会话累计或写入模型历史。
func TestContextCommandFollowsSession(t *testing.T) {
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
	second := a.CurrentSession
	cmd, args, err := command.Lookup("/context")
	if err != nil || cmd == nil {
		t.Fatal("/context 未注册", err)
	}

	for _, s := range []*session.Session{first, second, first} {
		a.ActivateSession(s)
		before := len(s.History.ToChatCompletionMessage())
		var output bytes.Buffer
		if err := cmd.RunPipe(context.Background(), a, args, &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), s.History.SessionID) {
			t.Fatal("没有显示当前会话 ID")
		}
		if s == first && !strings.Contains(output.String(), "剩余 ≈6.0k") {
			t.Fatal(output.String())
		}
		if s == second && (!strings.Contains(output.String(), "尚无数据") || strings.Contains(output.String(), "6.0k")) {
			t.Fatal("新会话混入其他会话的统计")
		}
		if len(s.History.ToChatCompletionMessage()) != before {
			t.Fatal("查看统计不应写入模型历史")
		}
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
		if !strings.Contains(plain, "已用 ≈2.0k") || !strings.Contains(plain, "剩余 ≈6.0k") || !strings.Contains(plain, "尚无数据") {
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
