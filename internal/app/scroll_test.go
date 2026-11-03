package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// TestHistoryScrollPreservesReading 验证滚轮、翻页、首尾跳转及流式追加不会破坏阅读位置和草稿。
func TestHistoryScrollPreservesReading(t *testing.T) {
	a := terminalTestApp(t)
	var lines []string

	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("line-%03d", i))
	}

	a.CurrentSession.Output.Update(conversation.Event{
		Role: conversation.Tool,
		Text: strings.Join(lines, "\n"),
	})
	m := newTerminal(a, context.Background())
	m = updateTerminal(m, tea.WindowSizeMsg{
		Width:  80,
		Height: 30,
	})
	m = updateTerminal(m, tea.PasteMsg{Content: "保留草稿"})
	cursor := m.session.Draft.Cursor().Position

	// 完整工具输出现在默认折叠；进入详情后验证每一行仍然可以滚动查看。
	m = updateTerminal(m, tea.KeyPressMsg{
		Code: 'o',
		Mod:  tea.ModCtrl,
	})

	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("未启用鼠标滚轮")
	}

	m = updateTerminal(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})

	if m.detailOffset != 3 {
		t.Fatal("滚轮没有向上滚动")
	}

	m = updateTerminal(m, tea.KeyPressMsg{Code: tea.KeyPgUp})

	if m.detailOffset <= 3 {
		t.Fatal("PgUp 没有继续上翻")
	}

	m = updateTerminal(m, tea.KeyPressMsg{
		Code: tea.KeyHome,
		Mod:  tea.ModCtrl,
	})
	_, before := m.render()

	if !strings.Contains(before.Content, "line-000") {
		t.Fatal("Ctrl+Home 没有到达顶部")
	}

	// 新输出会增加末尾行数，但第一行仍应停留在原来的位置。
	a.CurrentSession.Output.Update(conversation.Event{
		Kind: conversation.Delta,
		Role: conversation.Assistant,
		Text: "新增输出\n新的一行",
	})
	m = updateTerminal(m, refreshMsg{})
	_, after := m.render()

	if after.Total-after.Offset != before.Total-before.Offset ||
		!strings.Contains(after.Content, "line-000") {
		t.Fatal("新输出推动了正在阅读的位置")
	}

	// 在顶部反复滚动后，向下一步应立即移动，不积累越界偏移。
	for i := 0; i < 10; i++ {
		m = updateTerminal(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	}

	top := m.detailOffset
	m = updateTerminal(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	if m.detailOffset != top-3 {
		t.Fatal("顶部滚动产生了越界偏移")
	}

	m = updateTerminal(m, tea.KeyPressMsg{
		Code: tea.KeyEnd,
		Mod:  tea.ModCtrl,
	})

	if m.detailOffset != 0 || !strings.Contains(m.View().Content, "新的一行") {
		t.Fatal("Ctrl+End 没有回到最新输出")
	}

	if m.session.Draft.Value() != "保留草稿" || m.session.Draft.Cursor().Position != cursor {
		t.Fatal("滚动改变了输入内容或光标")
	}

	m = updateTerminal(m, tea.PasteMsg{Content: "详情内不能修改草稿"})
	m = updateTerminal(m, tea.KeyPressMsg{Code: tea.KeyEscape})

	if m.details || m.session.ScrollOffset != 0 || m.session.Draft.Value() != "保留草稿" {
		t.Fatal("Esc 没有恢复聊天位置和原草稿")
	}
}
