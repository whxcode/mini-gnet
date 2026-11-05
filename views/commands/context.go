package commands

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// ShowContext 展示当前会话的统计快照，Esc 返回原会话。
func ShowContext(text string) error {
	_, err := tea.NewProgram(newContext(text)).Run()

	return err
}

// contextModel 仅管理详情的折行和滚动，不持有会话或启动后台任务。
type contextModel struct {
	text   string
	width  int
	height int
	offset int
	done   bool
}

// newContext 保存打开时的快照，等待终端尺寸消息。
func newContext(text string) contextModel {
	return contextModel{
		text:   text,
		width:  78,
		height: 24,
	}
}

// Init 统计已经就绪，无需发起请求。
func (m contextModel) Init() tea.Cmd {
	return nil
}

// lines 保留段落并按显示列折行，长会话 ID 和中文说明不会丢失。
func (m contextModel) lines() []string {
	var lines []string
	for _, line := range strings.Split(m.text, "\n") {
		clean := clip(line, len(line)*2+1)
		wrapped := ansi.Hardwrap(clean, max(1, m.width-2), true)
		lines = append(lines, strings.Split(wrapped, "\n")...)
	}

	return lines
}

// Update 处理翻页、鼠标滚动与退出，窗口变化后重新约束偏移。
func (m contextModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.offset -= max(1, m.height-4)
		case "pgdown":
			m.offset += max(1, m.height-4)
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

// View 固定保留标题和返回提示，正文可滚动查看全部精确数值。
func (m contextModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	lines := m.lines()
	end := min(len(lines), m.offset+max(1, m.height-4))
	text := accent.Render(clip("当前会话 · 上下文明细", m.width-2)) + "\n\n"
	text += strings.Join(lines[m.offset:end], "\n")
	text += "\n" + muted.Render(clip("Esc 返回 · ↑↓ / PgUp/PgDown 滚动", m.width-2))

	view := tea.NewView(panel.Width(m.width).Render(text))
	view.MouseMode = tea.MouseModeCellMotion
	return view
}
