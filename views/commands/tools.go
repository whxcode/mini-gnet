package commands

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/tools"
)

// ShowTools 打开可滚动的工具说明，退出菜单后继续原会话。
func ShowTools(items []tools.Info, detail bool) error {
	_, err := tea.NewProgram(newTools(items, detail)).Run()

	return err
}

// toolsModel 只保存菜单显示状态，不加载定义或执行工具。
type toolsModel struct {
	cards  []toolCard // 从注册表整理的展示数据。
	width  int        // 菜单总宽度。
	height int        // 终端可用高度。
	offset int        // 从第一行开始的滚动偏移。
	done   bool       // 是否已返回聊天。
}

// newTools 创建工具菜单，收到窗口消息后按真实尺寸排版。
func newTools(items []tools.Info, detail bool) toolsModel {
	return toolsModel{
		cards:  toolCards(items, detail),
		width:  78,
		height: 24,
	}
}

// Init 工具说明已就绪，无需后台请求。
func (m toolsModel) Init() tea.Cmd {
	return nil
}

// lines 将说明折行，滚动时使用显示行而不是原始 JSON 行。
func (m toolsModel) lines() []string {
	if len(m.cards) == 0 {
		return []string{"当前没有已注册工具。"}
	}

	var cards []string
	for _, card := range m.cards {
		cards = append(cards, card.render(m.width-2))
	}

	return strings.Split(strings.Join(cards, "\n\n"), "\n")
}

// Update 处理滚动和退出，Esc、Enter、Ctrl+C 均返回原输入界面。
func (m toolsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(8, msg.Width-1)
		m.height = max(6, msg.Height)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "enter", "ctrl+c":
			m.done = true

			return m, tea.Quit
		case "up", "k":
			m.offset--
		case "down", "j":
			m.offset++
		case "pgup":
			m.offset -= max(1, m.height-5)
		case "pgdown":
			m.offset += max(1, m.height-5)
		case "home", "ctrl+home":
			m.offset = 0
		case "end", "ctrl+end":
			m.offset = len(m.lines())
		}

	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			m.offset -= 3
		}

		if msg.Button == tea.MouseWheelDown {
			m.offset += 3
		}
	}

	m.offset = min(max(0, m.offset), max(0, len(m.lines())-max(1, m.height-4)))

	return m, nil
}

// View 只显示当前页，工具说明再长也不会挤出返回提示。
func (m toolsModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	lines := m.lines()
	end := min(len(lines), m.offset+max(1, m.height-4))
	heading := fmt.Sprintf("工具 · %d 个", len(m.cards))
	text := accent.Render(clip(heading, m.width-2)) + "\n\n" + strings.Join(lines[m.offset:end], "\n")
	footer := fmt.Sprintf("%d–%d / %d 行 · ↑↓ 滚动 · PgUp/PgDown · /tools <名称> 详情 · Esc 返回", m.offset+1, end, len(lines))
	if m.width < 80 {
		footer = fmt.Sprintf("%d/%d · ↑↓ 滚动 · Esc 返回", end, len(lines))
	}
	text += "\n" + muted.Render(clip(footer, m.width-2))
	view := tea.NewView(panel.Width(m.width).Render(text))
	view.MouseMode = tea.MouseModeCellMotion

	return view
}
