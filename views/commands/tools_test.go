package commands

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestToolsMenuScrollAndEscape 验证长说明可翻到末尾，窄窗口不溢出，Esc 正确返回。
func TestToolsMenuScrollAndEscape(t *testing.T) {
	m := newTools(strings.Repeat("参数说明很长，需要正确折行。\n", 100) + "最后一行")
	next, _ := m.Update(tea.WindowSizeMsg{
		Width:  32,
		Height: 20,
	})
	m = next.(toolsModel)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(toolsModel)
	view := ansi.Strip(m.View().Content)

	if !strings.Contains(view, "最后一行") || len(strings.Split(view, "\n")) > 20 {
		t.Fatalf("无法查看末尾：%s", view)
	}

	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 32 {
			t.Fatal("菜单超出窗口宽度")
		}
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if !next.(toolsModel).done || cmd == nil || next.(toolsModel).View().Content != "" {
		t.Fatal("Esc 没有返回")
	}
}
