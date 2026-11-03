package commands

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// ShowTools 打开可滚动的工具说明，退出菜单后继续原会话。
func ShowTools(text string) error {
	_, err := tea.NewProgram(newTools(text)).Run()

	return err
}

// toolsModel 只保存菜单显示状态，不加载定义或执行工具。
type toolsModel struct {
	text   string // 调用方从注册表生成的说明。
	width  int    // 菜单总宽度。
	height int    // 终端可用高度。
	offset int    // 从第一行开始的滚动偏移。
	done   bool   // 是否已返回聊天。
}

// newTools 创建工具菜单，收到窗口消息后按真实尺寸排版。
func newTools(text string) toolsModel {
	return toolsModel{
		text:   text,
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
	return strings.Split(ansi.Hardwrap(m.text, max(1, m.width-2), true), "\n")
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
	text := accent.Render("◆ 已注册工具") + "\n\n" + strings.Join(lines[m.offset:end], "\n")
	text += "\n" + muted.Render(clip("↑↓ / 滚轮 / PgUp/PgDown · Esc / Enter 返回", m.width-2))
	view := tea.NewView(panel.Width(m.width).Render(text))
	view.MouseMode = tea.MouseModeCellMotion

	return view
}
