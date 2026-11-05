package commands

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestContextMenuLayout 验证中文、长 ID、翻页、缩放窗口与 Esc 返回。
func TestContextMenuLayout(t *testing.T) {
	id := "12345678-1234-1234-1234-123456789abc"
	text := "ID：" + id + "\n" + strings.Repeat("中文上下文说明及使用量统计。\n", 30) + "最后一行"
	for _, width := range []int{24, 48, 100} {
		m := newContext(text)
		next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 16})
		m = next.(contextModel)
		if !strings.Contains(strings.ReplaceAll(strings.Join(m.lines(), ""), " ", ""), id) {
			t.Fatal("长 ID 被裁掉")
		}

		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		m = next.(contextModel)
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "最后一行") || !strings.Contains(view, "Esc 返回") || len(strings.Split(view, "\n")) > 16 {
			t.Fatalf("详情末尾或返回提示不可见：%s", view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("%d 列窗口溢出：%s", width, line)
			}
		}

		next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 80})
		m = next.(contextModel)
		if !strings.Contains(ansi.Strip(m.View().Content), id) {
			t.Fatal("窗口扩大后偏移未复位")
		}
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if cmd == nil || next.(contextModel).View().Content != "" {
			t.Fatal("Esc 未返回")
		}
	}
}
