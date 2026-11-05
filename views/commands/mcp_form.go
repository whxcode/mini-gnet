package commands

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/internal/mcp"
)

// EditMCP 编辑服务副本；Esc 返回 nil，确认后由命令层保存。
func EditMCP(server mcp.Server, problem string) (*mcp.Server, error) {
	model := newMCPForm(server)
	model.problem = problem
	result, err := tea.NewProgram(model).Run()
	if err != nil {
		return nil, err
	}

	return result.(mcpForm).saved, nil
}

// mcpForm 只保存编辑草稿，不持有管理器或文件句柄。
type mcpForm struct {
	fields  []textinput.Model
	focus   int
	width   int
	height  int
	problem string
	saved   *mcp.Server
	done    bool
}

// newMCPForm 使用固定的 HTTP 传输方式，避免呈现尚未实现的选项。
func newMCPForm(server mcp.Server) mcpForm {
	m := mcpForm{width: 78, height: 24}
	for _, value := range []string{server.Name, server.Description, server.URL} {
		field := textinput.New()
		field.Prompt = ""
		field.SetWidth(74)
		field.SetValue(value)
		field.SetVirtualCursor(true)
		m.fields = append(m.fields, field)
	}

	m.fields[0].Focus()
	return m
}

// Init 表单不需要后台操作。
func (m mcpForm) Init() tea.Cmd {
	return nil
}

// Update 校验草稿，确认才返回结果；保存失败可携带草稿重新打开。
func (m mcpForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(8, msg.Width-1)
		m.height = max(5, msg.Height)
		for i := range m.fields {
			// Bubbles 修改宽度不会重新计算横向窗口；保留光标并刷新可见片段。
			position := m.fields[i].Position()
			m.fields[i].SetWidth(max(1, m.width-3))
			m.fields[i].CursorEnd()
			m.fields[i].SetCursor(position)
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.done = true
			return m, tea.Quit
		case "enter", "tab", "down", "shift+tab", "up":
			if msg.String() == "enter" && m.focus == len(m.fields) {
				server, err := mcp.Normalize(mcp.Server{
					Name:        m.fields[0].Value(),
					Description: m.fields[1].Value(),
					URL:         m.fields[2].Value(),
					Transport:   mcp.StreamableHTTP,
				})
				if err != nil {
					m.problem = err.Error()
					return m, nil
				}

				m.saved = &server
				m.done = true
				return m, tea.Quit
			}

			if m.focus < len(m.fields) {
				m.fields[m.focus].Blur()
			}

			step := 1
			if msg.String() == "shift+tab" || msg.String() == "up" {
				step = -1
			}

			m.focus = (m.focus + step + len(m.fields) + 1) % (len(m.fields) + 1)
			if m.focus < len(m.fields) {
				return m, m.fields[m.focus].Focus()
			}

			return m, nil
		}
	}

	if m.focus < len(m.fields) {
		var cmd tea.Cmd
		m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
		return m, cmd
	}

	return m, nil
}

// View 短窗口只显示当前字段，避免保存按钮和 Esc 提示被挤出屏幕。
func (m mcpForm) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	labels := []string{"服务名称", "简介（可留空）", "地址 · streamable_http"}
	lines := []string{accent.Render(clip("◆ 编辑 MCP · 不连接服务", m.width-2))}
	for i, field := range m.fields {
		if m.height < 13 && i != m.focus {
			continue
		}

		label := "  " + labels[i]
		if i == m.focus {
			label = "› " + labels[i]
		}

		lines = append(lines, clip(label, m.width-2), field.View())
	}

	if m.height >= 13 || m.focus == len(m.fields) {
		button := "  保存配置"
		if m.focus == len(m.fields) {
			button = "› 保存配置"
		}

		lines = append(lines, accent.Render(clip(button, m.width-2)))
	}

	if m.problem != "" {
		lines = append(lines, bad.Render(clip(m.problem, m.width-2)))
	}

	lines = append(lines, muted.Render(clip("Esc 返回 · Tab 切换 · Enter 下一项/保存", m.width-2)))
	return tea.NewView(panel.Width(m.width).Render(strings.Join(lines, "\n")))
}
