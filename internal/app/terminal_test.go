package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/net"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// TestSessionGridFitsTerminal 验证多会话栅格在窄窗口和分页后不溢出，当前卡片与输入光标保持可见。
func TestSessionGridFitsTerminal(t *testing.T) {
	a := terminalTestApp(t)
	for i := 1; i < 12; i++ {
		if err := executeCommandForTest(t, a, "/new"); err != nil {
			t.Fatal(err)
		}
	}
	for _, session := range a.opened {
		session.title = strings.Repeat("测试长标题是否撑开卡片", 5)
		session.hasUnreadResult = true
	}
	for _, width := range []int{32, 80, 120} {
		for _, height := range []int{24, 40} {
			for _, number := range []int{1, 7, 12} {
				if err := executeCommandForTest(t, a, fmt.Sprintf("/ps %d", number)); err != nil {
					t.Fatal(err)
				}
				m := newTerminal(a, context.Background())
				m = updateTerminal(m, tea.WindowSizeMsg{Width: width, Height: height})
				m.session.draft.SetValue("编辑中")
				view := m.View()
				plain := ansi.Strip(view.Content)
				lines := strings.Split(plain, "\n")
				for _, line := range lines {
					if ansi.StringWidth(line) > width {
						t.Fatalf("栅格超出 %d 列：%q", width, line)
					}
				}
				if len(lines) > height || !strings.Contains(plain, fmt.Sprintf("%d ○ 空闲 · 当前", number)) {
					t.Fatalf("%dx%d 会话 %d 的高度或当前卡片错误：\n%s", width, height, number, plain)
				}
				if view.Cursor == nil || view.Cursor.Position.Y >= len(lines) || !strings.Contains(lines[view.Cursor.Position.Y], "编辑中") {
					t.Fatal("栅格改变后输入光标偏移")
				}
			}
		}
	}
}

// TestTerminalGrowsWithContent 验证短对话不会占满屏幕，换行后输入区自然增长且光标位置正确。
func TestTerminalGrowsWithContent(t *testing.T) {
	a := terminalTestApp(t)
	m := newTerminal(a, context.Background())
	m = updateTerminal(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	before := m.View()
	if before.AltScreen || lipgloss.Height(before.Content) >= 15 {
		t.Fatal("短对话仍使用全屏或填充大量空行")
	}
	m = updateTerminal(m, tea.PasteMsg{Content: "第一行\n第二行\n第三行"})
	after := m.View()
	if lipgloss.Height(after.Content) != lipgloss.Height(before.Content)+2 {
		t.Fatal("输入高度没有随内容增长")
	}
	lines := strings.Split(ansi.Strip(after.Content), "\n")
	if after.Cursor == nil || after.Cursor.Position.Y >= len(lines) || !strings.Contains(lines[after.Cursor.Position.Y], "第三行") {
		t.Fatalf("光标未跟随最后一行输入：%+v", after.Cursor)
	}
	m = updateTerminal(m, tea.WindowSizeMsg{Width: 100, Height: 60})
	if lipgloss.Height(m.View().Content) != lipgloss.Height(after.Content) {
		t.Fatal("拉高终端不应增加短对话的空白高度")
	}
}

// TestTerminalLayoutFitsWidth 验证边框和留白不会使窄窗口溢出，中文光标仍位于正在编辑的文字行。
func TestTerminalLayoutFitsWidth(t *testing.T) {
	for _, width := range []int{32, 80, 120} {
		a := terminalTestApp(t)
		m := newTerminal(a, context.Background())
		m = updateTerminal(m, tea.WindowSizeMsg{Width: width, Height: 30})
		a.CurrentSession.updateDisplay(conversation.Event{Role: conversation.Assistant, Text: strings.Repeat("中文内容需要正确折行。", 8)})
		m = updateTerminal(m, tea.PasteMsg{Content: "编辑中"})
		view := m.View()
		lines := strings.Split(ansi.Strip(view.Content), "\n")
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("窗口 %d 列溢出：%q", width, line)
			}
		}
		if len(lines) > 30 || view.Cursor == nil || !strings.Contains(lines[view.Cursor.Position.Y], "编辑中") {
			t.Fatalf("窗口 %d 列的高度或光标错误", width)
		}
	}
}

// terminalTestApp 创建只用于界面测试的会话，不请求模型。
func terminalTestApp(t *testing.T) *App {
	t.Helper()
	cfg := config.Config{ChatsDir: t.TempDir(), ProjectDir: t.TempDir()}
	record, err := startSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.attachSession(record, cfg, net.Client{})
	t.Cleanup(func() { a.Close() })
	return a
}

// updateTerminal 将按键或粘贴输入应用到界面模型，用于验证编辑状态。
func updateTerminal(m terminalModel, msg tea.Msg) terminalModel {
	next, _ := m.Update(msg)
	return next.(terminalModel)
}

// TestSessionDraftAndDisplayIsolation 验证快捷切换保留草稿，后台消息不显示在另一个会话里。
func TestSessionDraftAndDisplayIsolation(t *testing.T) {
	a := terminalTestApp(t)
	m := newTerminal(a, context.Background())
	m = updateTerminal(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updateTerminal(m, tea.PasteMsg{Content: "第一份草稿"})
	m = updateTerminal(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	first := a.CurrentSession
	firstCursor := first.draft.Cursor().Position
	m = updateTerminal(m, tea.KeyPressMsg{Code: 'n', Mod: tea.ModAlt})
	if !m.done || m.query != "/new" || first.draft.Value() != "第一份草稿" {
		t.Fatal("快捷新建丢失草稿")
	}
	if err := executeCommandForTest(t, a, m.query); err != nil {
		t.Fatal(err)
	}
	second := a.CurrentSession
	m = newTerminal(a, context.Background())
	m = updateTerminal(m, tea.PasteMsg{Content: "第二份草稿"})
	first.updateDisplay(conversation.Event{Role: conversation.Assistant, Text: "only-first-output"})
	if strings.Contains(ansi.Strip(m.View().Content), "only-first-output") {
		t.Fatal("后台完整输出混入当前会话")
	}
	m = updateTerminal(m, tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	if err := executeCommandForTest(t, a, m.query); err != nil {
		t.Fatal(err)
	}
	if a.CurrentSession != first || first.draft.Value() != "第一份草稿" || first.draft.Cursor().Position != firstCursor {
		t.Fatal("切回未恢复草稿与光标")
	}
	if second.draft.Value() != "第二份草稿" {
		t.Fatal("第二会话草稿被覆盖")
	}
	if !strings.Contains(ansi.Strip(newTerminal(a, context.Background()).View().Content), "only-first-output") {
		t.Fatal("切回没有恢复输出")
	}
}

// TestBusyInputRemainsEditable 验证执行期间可编辑、换行和调用命令，普通重复提交不会丢失草稿。
func TestBusyInputRemainsEditable(t *testing.T) {
	a := terminalTestApp(t)
	s := a.CurrentSession
	// 本测试不创建后台任务，仅设置显示状态；结束前恢复，避免清理时发起取消。
	s.running = true
	defer func() { s.running = false }()
	m := newTerminal(a, context.Background())
	m = updateTerminal(m, tea.PasteMsg{Content: "等待中的问题"})
	m = updateTerminal(m, tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = updateTerminal(m, tea.PasteMsg{Content: "第二行"})
	m = updateTerminal(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.done || !m.fromEditor || s.draft.Value() != "等待中的问题\n第二行" {
		t.Fatal("界面应交出原始文本并保留草稿")
	}
	if _, err := s.prepareTask(context.Background(), m.query, nil); err == nil {
		t.Fatal("主流程允许忙碌会话重复提交")
	}
	m = newTerminal(a, context.Background())
	m = updateTerminal(m, tea.KeyPressMsg{Code: tea.KeyF2})
	if !m.done || m.query != "/sessions" {
		t.Fatal("执行中无法打开会话列表")
	}
	m = newTerminal(a, context.Background())
	m = updateTerminal(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.query != "/stop" {
		t.Fatal("Ctrl+C 没有针对当前任务停止")
	}
}
