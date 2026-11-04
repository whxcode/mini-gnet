package commands

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/tools"
)

// TestToolsMenuScrollAndEscape 验证长说明可翻到末尾，窄窗口不溢出，Esc 正确返回。
func TestToolsMenuScrollAndEscape(t *testing.T) {
	m := newTools([]tools.Info{{
		Definition: tools.Definition{
			Name:        "test_tool",
			Description: strings.Repeat("参数说明很长，需要正确折行。\n", 100),
		},
		Launch: tools.LaunchInfo{Note: "最后一行"},
	}}, false)
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

// TestToolsCardsLayout 验证卡片层级、长路径完整性及不同窗口下的边界。
func TestToolsCardsLayout(t *testing.T) {
	path := "/Users/mac/study/mini-gnet/my_tools/web_search/tool.json"
	items := []tools.Info{{
		Definition: tools.Definition{
			Name:        "web_search",
			Description: "联网搜索网页，返回标题、链接和摘要。",
			InputSchema: []byte(`{"properties":{"query":{"type":"string","description":"搜索关键词，支持中文和英文。"}},"required":["query"]}`),
		},
		Launch: tools.LaunchInfo{
			Command:        "node",
			Args:           []string{"main.mjs"},
			ExecutablePath: "/opt/homebrew/bin/node",
			WorkDir:        "/Users/mac/study/mini-gnet/my_tools/web_search",
			ConfigPath:     path,
		},
	}}

	for _, width := range []int{24, 48, 100} {
		m := newTools(items, false)
		next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m = next.(toolsModel)
		content := ansi.Strip(strings.Join(m.lines(), "\n"))

		previous := -1
		for _, label := range []string{"web_search", "简介", "输入参数", "执行命令", "启动参数", "可执行文件路径", "工作目录", "配置文件"} {
			position := strings.Index(content, label)
			if position <= previous {
				t.Fatalf("%d 列窗口的层级不正确，缺少 %s：\n%s", width, label, content)
			}
			previous = position
		}

		// 去掉卡片边框和折行空白后，路径必须完整，没有省略号或丢字。
		compact := strings.NewReplacer("│", "", " ", "", "\n", "").Replace(content)
		if !strings.Contains(compact, path) {
			t.Fatalf("%d 列窗口丢失路径：\n%s", width, content)
		}

		for _, line := range strings.Split(content, "\n") {
			if ansi.StringWidth(line) > width-3 {
				t.Fatalf("%d 列窗口的卡片超宽：%s", width, line)
			}
		}

		if width == 48 {
			t.Log("\n" + content)
		}

		// 在列表末尾扩大窗口时，也必须重新约束滚动位置。
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		m = next.(toolsModel)
		next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 80})
		m = next.(toolsModel)
		if !strings.Contains(ansi.Strip(m.View().Content), path) {
			t.Fatal("窗口放大后无法查看完整路径")
		}
	}
}
